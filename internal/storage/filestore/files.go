package filestore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// fileSystem is the only way the store touches project files. Paths are
// slash-separated and relative to the project root.
//
// Two implementations exist. diskFS reads and writes the project directly.
// stagedFS keeps every write in memory on top of the disk state, so a command
// can load and validate the would-be project before anything reaches the disk
// (GAP-015, GAP-019).
type fileSystem interface {
	read(rel string) ([]byte, error)
	write(rel string, data []byte) error
	remove(rel string) error
	exists(rel string) (bool, error)
	glob(pattern string) ([]string, error)
}

// ErrBindingsLayerReadOnly refuses every project write while the OpenProject
// bindings layer is loaded, so layered private source refs never reach
// tracked files, whichever command asks.
var ErrBindingsLayerReadOnly = errors.New("the OpenProject bindings layer is read-only: project files cannot be written while it is active")

type readOnlyFS struct {
	fileSystem
}

func (readOnlyFS) write(string, []byte) error { return ErrBindingsLayerReadOnly }

func (readOnlyFS) remove(string) error { return ErrBindingsLayerReadOnly }

type diskFS struct {
	root                string
	commitOperationHook func(completed int, path string) error // test-only failure injection
}

func (d diskFS) abs(rel string) string {
	return filepath.Join(d.root, filepath.FromSlash(rel))
}

func (d diskFS) read(rel string) ([]byte, error) {
	return os.ReadFile(d.abs(rel))
}

func (d diskFS) write(rel string, data []byte) error {
	return writeFileAtomic(d.abs(rel), data, 0o644)
}

func (d diskFS) remove(rel string) error {
	return os.Remove(d.abs(rel))
}

func (d diskFS) exists(rel string) (bool, error) {
	_, err := os.Stat(d.abs(rel))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (d diskFS) glob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(d.abs(pattern))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		rel, err := filepath.Rel(d.root, match)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out, nil
}

// stagedFS overlays pending writes and removals on the disk. Nothing reaches
// the disk until commit.
type stagedFS struct {
	disk    diskFS
	files   map[string][]byte
	removed map[string]bool
}

func newStagedFS(disk diskFS) *stagedFS {
	return &stagedFS{disk: disk, files: map[string][]byte{}, removed: map[string]bool{}}
}

func (s *stagedFS) read(rel string) ([]byte, error) {
	if s.removed[rel] {
		return nil, &fs.PathError{Op: "read", Path: rel, Err: os.ErrNotExist}
	}
	if data, ok := s.files[rel]; ok {
		return append([]byte(nil), data...), nil
	}
	return s.disk.read(rel)
}

func (s *stagedFS) write(rel string, data []byte) error {
	delete(s.removed, rel)
	s.files[rel] = append([]byte(nil), data...)
	return nil
}

func (s *stagedFS) remove(rel string) error {
	if _, ok := s.files[rel]; ok {
		delete(s.files, rel)
		onDisk, err := s.disk.exists(rel)
		if err != nil {
			return err
		}
		if onDisk {
			s.removed[rel] = true
		}
		return nil
	}
	onDisk, err := s.disk.exists(rel)
	if err != nil {
		return err
	}
	if !onDisk || s.removed[rel] {
		return &fs.PathError{Op: "remove", Path: rel, Err: os.ErrNotExist}
	}
	s.removed[rel] = true
	return nil
}

func (s *stagedFS) exists(rel string) (bool, error) {
	if s.removed[rel] {
		return false, nil
	}
	if _, ok := s.files[rel]; ok {
		return true, nil
	}
	return s.disk.exists(rel)
}

func (s *stagedFS) glob(pattern string) ([]string, error) {
	onDisk, err := s.disk.glob(pattern)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, rel := range onDisk {
		if s.removed[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	for rel := range s.files {
		if seen[rel] {
			continue
		}
		ok, err := path.Match(pattern, rel)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out, nil
}

// pending lists every path the stage would change, in commit order.
func (s *stagedFS) pending() []string {
	data, audit := splitAudit(commitOrder(s.files))
	out := make([]string, 0, len(s.files)+len(s.removed))
	out = append(out, data...)
	out = append(out, sortedKeys(s.removed)...)
	return append(out, audit...)
}

// commit writes a durable roll-forward journal before it replaces staged data
// and proposal files, applies removals, and replaces audit files. Each file is
// replaced atomically. If the process stops between files, the next locked
// write or project recover replays the complete validated set from the journal.
// Callers outside the locked CLI must provide equivalent serialization and
// recovery. The returned list holds the paths that did reach the disk in this
// attempt, in commit order.
func (s *stagedFS) commit() ([]string, error) {
	written := []string{}
	data, audit := splitAudit(commitOrder(s.files))
	order := make([]commitJournalOperation, 0, len(s.files)+len(s.removed))
	for _, rel := range data {
		order = append(order, commitJournalOperation{Action: journalActionWrite, Path: rel})
	}
	for _, rel := range sortedKeys(s.removed) {
		order = append(order, commitJournalOperation{Action: journalActionRemove, Path: rel})
	}
	for _, rel := range audit {
		order = append(order, commitJournalOperation{Action: journalActionWrite, Path: rel})
	}
	journal, err := s.disk.createCommitJournal(order, s.files)
	if err != nil {
		return written, err
	}
	for i, operation := range journal.operations {
		if s.disk.commitOperationHook != nil {
			if err := s.disk.commitOperationHook(i, operation.Path); err != nil {
				return written, fmt.Errorf("commit %s: %w", operation.Path, err)
			}
		}
		if err := s.disk.applyCommitOperationWithContents(operation, journal.contents); err != nil {
			return written, err
		}
		written = append(written, operation.Path)
	}
	if err := s.disk.deleteCommitJournal(); err != nil {
		return written, fmt.Errorf("delete completed commit journal: %w", err)
	}
	s.files = map[string][]byte{}
	s.removed = map[string]bool{}
	return written, nil
}

func splitAudit(paths []string) (data, audit []string) {
	for _, rel := range paths {
		if strings.HasPrefix(rel, "audit/") {
			audit = append(audit, rel)
		} else {
			data = append(data, rel)
		}
	}
	return data, audit
}

func commitOrder(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for rel := range files {
		keys = append(keys, rel)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		ai, aj := strings.HasPrefix(keys[i], "audit/"), strings.HasPrefix(keys[j], "audit/")
		if ai != aj {
			return !ai
		}
		return keys[i] < keys[j]
	})
	return keys
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for rel := range m {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	return keys
}
