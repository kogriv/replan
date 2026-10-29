package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestExportJSONSmallTeamFixture(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.JSON(context.Background(), app.ExportJSONRequest{
		Format: "json",
		Now:    time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("JSON returned error: %v", err)
	}
	if result.Metadata.ID != "export.replan.20260710T120000Z" {
		t.Fatalf("Metadata.ID = %q", result.Metadata.ID)
	}
	if result.Metadata.ProjectPath != "../../examples/small-team" {
		t.Fatalf("ProjectPath = %q", result.Metadata.ProjectPath)
	}
	if result.Workspace.Project.ID != "project.replan-mvp" {
		t.Fatalf("Project.ID = %q", result.Workspace.Project.ID)
	}
	if len(result.Workspace.Tasks) != 2 {
		t.Fatalf("Tasks len = %d, want 2", len(result.Workspace.Tasks))
	}
	if len(result.Workspace.ResourceDemands) != 7 {
		t.Fatalf("ResourceDemands len = %d, want 7", len(result.Workspace.ResourceDemands))
	}
	if len(result.Workspace.Dependencies) != 2 {
		t.Fatalf("Dependencies len = %d, want 2", len(result.Workspace.Dependencies))
	}
	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
}

func TestExportJSONReportsLoadErrors(t *testing.T) {
	service := app.NewExportService(filestore.New(t.TempDir()))

	result, err := service.JSON(context.Background(), app.ExportJSONRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("JSON error = %v, want ErrValidationFailed", err)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected load errors")
	}
}
