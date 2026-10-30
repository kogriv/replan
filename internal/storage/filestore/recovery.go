package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	commitJournalRelativePath = ".replan/commit-pending"
	commitManifestName        = "manifest.json"
	commitJournalVersion      = 1
	journalActionWrite        = "write"
	journalActionRemove       = "remove"
)

// ErrCommitJournalCorrupt means a complete journal cannot safely be replayed.
// Recovery validates the whole journal before changing project files.
var ErrCommitJournalCorrupt = errors.New("commit journal is corrupt")

type commitJournalWrite struct {
	Path   string `json:"path"`
	Blob   string `json:"blob"`
	SHA256 string `json:"sha256"`
}

type commitJournalOperation struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

type commitJournalManifest struct {
	Version  int                      `json:"version"`
	Writes   []commitJournalWrite     `json:"writes"`
	Removals []string                 `json:"removals"`
	Order    []commitJournalOperation `json:"order"`
}

type loadedCommitJournal struct {
	operations []commitJournalOperation
	contents   map[string][]byte
}

// CommitRecovery describes a pending journal and the files in replay order.
type CommitRecovery struct {
	Pending           bool     `json:"pending"`
	Complete          bool     `json:"complete"`
	IncompleteRemoved bool     `json:"incomplete_removed,omitempty"`
	RecoveredFiles    []string `json:"recovered_files"`
}

// InspectCommit reports a pending journal without changing it or project data.
// A complete journal is fully validated, including every blob hash.
func InspectCommit(projectPath string) (CommitRecovery, error) {
	return diskFS{root: projectPath}.recoverCommitJournal(true)
}

// RecoverCommit replays a complete journal, or removes an incomplete
// manifest-less journal. The caller must hold the project write lock.
func RecoverCommit(projectPath string) (CommitRecovery, error) {
	return diskFS{root: projectPath}.recoverCommitJournal(false)
}

func (d diskFS) journalPath(parts ...string) string {
	all := append([]string{d.root, filepath.FromSlash(commitJournalRelativePath)}, parts...)
	return filepath.Join(all...)
}

func (d diskFS) createCommitJournal(order []commitJournalOperation, files map[string][]byte) (loadedCommitJournal, error) {
	journalPath := d.journalPath()
	if _, err := os.Stat(journalPath); err == nil {
		return loadedCommitJournal{}, fmt.Errorf("commit journal %s already exists; recover it before writing", journalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return loadedCommitJournal{}, fmt.Errorf("stat commit journal %s: %w", journalPath, err)
	}
	blobDir := d.journalPath("blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		return loadedCommitJournal{}, fmt.Errorf("create commit journal: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(journalPath)
		}
	}()
	if err := syncDirectory(filepath.Join(d.root, ".replan")); err != nil {
		return loadedCommitJournal{}, fmt.Errorf("sync commit journal directory: %w", err)
	}

	manifest := commitJournalManifest{Version: commitJournalVersion, Writes: []commitJournalWrite{}, Removals: []string{}, Order: order}
	contents := make(map[string][]byte, len(files))
	writeIndex := 0
	for _, operation := range order {
		if err := validateCommitPath(operation.Path); err != nil {
			return loadedCommitJournal{}, err
		}
		switch operation.Action {
		case journalActionWrite:
			content, ok := files[operation.Path]
			if !ok {
				return loadedCommitJournal{}, fmt.Errorf("commit journal has no staged content for %s", operation.Path)
			}
			blob := filepath.ToSlash(filepath.Join("blobs", fmt.Sprintf("%06d", writeIndex)))
			writeIndex++
			sum := sha256.Sum256(content)
			if err := writeSyncedFile(d.journalPath(filepath.FromSlash(blob)), content, 0o600); err != nil {
				return loadedCommitJournal{}, fmt.Errorf("write commit journal blob for %s: %w", operation.Path, err)
			}
			manifest.Writes = append(manifest.Writes, commitJournalWrite{Path: operation.Path, Blob: blob, SHA256: hex.EncodeToString(sum[:])})
			contents[operation.Path] = append([]byte(nil), content...)
		case journalActionRemove:
			manifest.Removals = append(manifest.Removals, operation.Path)
		default:
			return loadedCommitJournal{}, fmt.Errorf("unsupported commit operation %q", operation.Action)
		}
	}
	if err := syncDirectory(blobDir); err != nil {
		return loadedCommitJournal{}, fmt.Errorf("sync commit journal blobs: %w", err)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return loadedCommitJournal{}, fmt.Errorf("encode commit journal manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := writeFileAtomicSynced(d.journalPath(commitManifestName), manifestData, 0o600); err != nil {
		return loadedCommitJournal{}, fmt.Errorf("write commit journal manifest: %w", err)
	}
	cleanup = false
	return loadedCommitJournal{operations: order, contents: contents}, nil
}

func (d diskFS) recoverCommitJournal(inspectOnly bool) (CommitRecovery, error) {
	result := CommitRecovery{RecoveredFiles: []string{}}
	info, err := os.Stat(d.journalPath())
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("stat commit journal: %w", err)
	}
	if !info.IsDir() {
		return result, corruptJournalError("%s is not a directory", d.journalPath())
	}
	result.Pending = true
	if _, err := os.Stat(d.journalPath(commitManifestName)); errors.Is(err, os.ErrNotExist) {
		if inspectOnly {
			return result, nil
		}
		if err := os.RemoveAll(d.journalPath()); err != nil {
			return result, fmt.Errorf("remove incomplete commit journal: %w", err)
		}
		if err := syncDirectory(filepath.Join(d.root, ".replan")); err != nil {
			return result, fmt.Errorf("sync commit journal removal: %w", err)
		}
		result.IncompleteRemoved = true
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("stat commit journal manifest: %w", err)
	}

	journal, err := d.loadCommitJournal()
	if err != nil {
		return result, err
	}
	result.Complete = true
	for _, operation := range journal.operations {
		result.RecoveredFiles = append(result.RecoveredFiles, operation.Path)
	}
	if inspectOnly {
		return result, nil
	}
	for _, operation := range journal.operations {
		if err := d.applyCommitOperationWithContents(operation, journal.contents); err != nil {
			return result, err
		}
	}
	if err := d.deleteCommitJournal(); err != nil {
		return result, fmt.Errorf("delete recovered commit journal: %w", err)
	}
	return result, nil
}

func (d diskFS) loadCommitJournal() (loadedCommitJournal, error) {
	data, err := os.ReadFile(d.journalPath(commitManifestName))
	if err != nil {
		return loadedCommitJournal{}, corruptJournalError("read manifest: %v", err)
	}
	var manifest commitJournalManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return loadedCommitJournal{}, corruptJournalError("parse manifest: %v", err)
	}
	if manifest.Version != commitJournalVersion {
		return loadedCommitJournal{}, corruptJournalError("manifest version is %d, want %d", manifest.Version, commitJournalVersion)
	}
	writes := make(map[string]commitJournalWrite, len(manifest.Writes))
	contents := make(map[string][]byte, len(manifest.Writes))
	for _, write := range manifest.Writes {
		if err := validateCommitPath(write.Path); err != nil {
			return loadedCommitJournal{}, corruptJournalError("invalid write path %q: %v", write.Path, err)
		}
		if _, duplicate := writes[write.Path]; duplicate {
			return loadedCommitJournal{}, corruptJournalError("duplicate write path %s", write.Path)
		}
		if !validBlobPath(write.Blob) {
			return loadedCommitJournal{}, corruptJournalError("invalid blob path %q for %s", write.Blob, write.Path)
		}
		blob, err := os.ReadFile(d.journalPath(filepath.FromSlash(write.Blob)))
		if err != nil {
			return loadedCommitJournal{}, corruptJournalError("read blob for %s: %v", write.Path, err)
		}
		sum := sha256.Sum256(blob)
		if !strings.EqualFold(write.SHA256, hex.EncodeToString(sum[:])) {
			return loadedCommitJournal{}, corruptJournalError("blob hash does not match for %s", write.Path)
		}
		writes[write.Path] = write
		contents[write.Path] = blob
	}
	removals := make(map[string]bool, len(manifest.Removals))
	for _, rel := range manifest.Removals {
		if err := validateCommitPath(rel); err != nil {
			return loadedCommitJournal{}, corruptJournalError("invalid removal path %q: %v", rel, err)
		}
		if removals[rel] {
			return loadedCommitJournal{}, corruptJournalError("duplicate removal path %s", rel)
		}
		removals[rel] = true
	}
	seenWrites, seenRemovals := map[string]bool{}, map[string]bool{}
	phase := 0
	for _, operation := range manifest.Order {
		if err := validateCommitPath(operation.Path); err != nil {
			return loadedCommitJournal{}, corruptJournalError("invalid ordered path %q: %v", operation.Path, err)
		}
		switch operation.Action {
		case journalActionWrite:
			if _, ok := writes[operation.Path]; !ok || seenWrites[operation.Path] {
				return loadedCommitJournal{}, corruptJournalError("order has missing or duplicate write %s", operation.Path)
			}
			operationPhase := 0
			if strings.HasPrefix(operation.Path, "audit/") {
				operationPhase = 2
			}
			if operationPhase < phase {
				return loadedCommitJournal{}, corruptJournalError("order is not data, then removals, then audit")
			}
			phase = operationPhase
			seenWrites[operation.Path] = true
		case journalActionRemove:
			if !removals[operation.Path] || seenRemovals[operation.Path] {
				return loadedCommitJournal{}, corruptJournalError("order has missing or duplicate removal %s", operation.Path)
			}
			if phase > 1 {
				return loadedCommitJournal{}, corruptJournalError("order is not data, then removals, then audit")
			}
			phase = 1
			seenRemovals[operation.Path] = true
		default:
			return loadedCommitJournal{}, corruptJournalError("order has unsupported action %q", operation.Action)
		}
	}
	if len(seenWrites) != len(writes) || len(seenRemovals) != len(removals) {
		return loadedCommitJournal{}, corruptJournalError("order does not list every write and removal")
	}
	return loadedCommitJournal{operations: manifest.Order, contents: contents}, nil
}

func (d diskFS) applyCommitOperationWithContents(operation commitJournalOperation, contents map[string][]byte) error {
	switch operation.Action {
	case journalActionWrite:
		if err := d.write(operation.Path, contents[operation.Path]); err != nil {
			return fmt.Errorf("commit %s: %w", operation.Path, err)
		}
	case journalActionRemove:
		if err := d.remove(operation.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("commit removal of %s: %w", operation.Path, err)
		}
	default:
		return fmt.Errorf("commit %s: unsupported operation %q", operation.Path, operation.Action)
	}
	return nil
}

func (d diskFS) deleteCommitJournal() error {
	manifestPath := d.journalPath(commitManifestName)
	if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove manifest: %w", err)
	}
	if err := syncDirectory(d.journalPath()); err != nil {
		return fmt.Errorf("sync manifest removal: %w", err)
	}
	if err := os.RemoveAll(d.journalPath()); err != nil {
		return fmt.Errorf("remove journal directory: %w", err)
	}
	return syncDirectory(filepath.Join(d.root, ".replan"))
}

func validateCommitPath(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || filepath.ToSlash(filepath.Clean(rel)) != rel || rel == "." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("path must be a clean relative project path")
	}
	if rel == commitJournalRelativePath || strings.HasPrefix(rel, commitJournalRelativePath+"/") {
		return fmt.Errorf("path points inside the commit journal")
	}
	return nil
}

func validBlobPath(rel string) bool {
	return rel != "" && !filepath.IsAbs(rel) && filepath.ToSlash(filepath.Clean(rel)) == rel && strings.HasPrefix(rel, "blobs/") && !strings.Contains(rel, "../")
}

func corruptJournalError(format string, args ...any) error {
	return fmt.Errorf("%w: %s; inspect .replan/commit-pending/", ErrCommitJournalCorrupt, fmt.Sprintf(format, args...))
}

func writeSyncedFile(path string, data []byte, perm fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeFileAtomicSynced(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest.json.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		if runtime.GOOS == "windows" {
			return nil
		}
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return nil
}
