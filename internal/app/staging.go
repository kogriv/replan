package app

import (
	"context"
	"fmt"
)

// StagedWrites is a project view whose writes stay pending until Commit.
// Loading through it returns the would-be project.
type StagedWrites interface {
	WorkspaceLoader
	Pending() []string
	Commit() ([]string, error)
}

// WriteStager is implemented by stores that can stage writes. Commands use it
// to validate the result of their writes before anything reaches the disk, so
// a command never reports applied false after it has written files (GAP-015).
type WriteStager interface {
	StageWrites() StagedWrites
}

// writeSession routes a command's writes either through a stage or, for a
// store that cannot stage, directly to the store.
type writeSession struct {
	target WorkspaceLoader
	staged StagedWrites
}

func beginWrite(loader WorkspaceLoader) writeSession {
	if stager, ok := loader.(WriteStager); ok {
		staged := stager.StageWrites()
		return writeSession{target: staged, staged: staged}
	}
	return writeSession{target: loader}
}

// finish validates the would-be project. When validation fails nothing is
// committed and written is empty. When it passes, the stage is committed and
// written lists the paths that reached the disk; if the commit itself fails
// part way, written lists the files already replaced and a commit_failed
// diagnostic names the cause. Without a stage the writes already happened and
// written is the list the caller collected.
func (w writeSession) finish(ctx context.Context, collected []string) (ValidateResult, []string, error) {
	validation, err := NewValidateService(w.target).Validate(ctx, ValidateRequest{
		ProjectPath:       w.target.ProjectPath(),
		IncludeObjectScan: true,
	})
	if err != nil {
		if w.staged != nil {
			return validation, []string{}, err
		}
		return validation, collected, err
	}
	if w.staged == nil {
		return validation, collected, nil
	}
	written, err := w.staged.Commit()
	if err != nil {
		// Callers surface validation.Errors when finish fails, so the commit
		// failure is reported there. written lists what did reach the disk:
		// a failed commit can be partial, and the envelope must say so.
		validation.Errors = append(validation.Errors, Diagnostic{
			Code:     "commit_failed",
			Message:  fmt.Sprintf("commit stopped after writing %d of the staged files %v: %v", len(written), written, err),
			Severity: "error",
			Target:   Target{Kind: "project", ID: w.target.ProjectPath()},
		})
		return validation, written, err
	}
	return validation, written, nil
}

// ProjectFileCreator creates new project files that the loader reads, such as
// scenario and proposal candidates.
type ProjectFileCreator interface {
	CreateProjectFile(rel string, content []byte) (string, error)
}

// CreateProjectFiles stages the create writes, validates the would-be project
// and commits only when it is valid. Only the "create" action is supported;
// an existing target file is an error.
func CreateProjectFiles(ctx context.Context, loader WorkspaceLoader, writes []PlannedWrite) (ValidateResult, []string, []Diagnostic, error) {
	session := beginWrite(loader)
	creator, ok := session.target.(ProjectFileCreator)
	if !ok {
		diag := Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support creating project files", Severity: "error", Target: Target{Kind: "project", ID: loader.ProjectPath()}}
		return ValidateResult{}, []string{}, []Diagnostic{diag}, ErrValidationFailed
	}
	collected := []string{}
	for _, write := range writes {
		if write.Action != "create" {
			err := fmt.Errorf("unsupported candidate write action %s", write.Action)
			return ValidateResult{}, []string{}, []Diagnostic{{Code: "candidate_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "file", ID: write.Path}}}, err
		}
		rel, err := creator.CreateProjectFile(write.Path, []byte(write.Content))
		if err != nil {
			return ValidateResult{}, []string{}, []Diagnostic{{Code: "candidate_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "file", ID: write.Path}}}, err
		}
		collected = append(collected, rel)
	}
	validation, written, err := session.finish(ctx, collected)
	if err != nil {
		return validation, written, validation.Errors, err
	}
	return validation, written, nil, nil
}
