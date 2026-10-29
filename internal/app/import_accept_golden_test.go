package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

// A golden run of every import accept mode: the dry run, the apply, and the
// apply with nothing safe to write. The command result and the bytes of every
// written file, the audit record among them, are held in
// testdata/import-accept/<case>.json (GAP-020).
func TestImportAcceptGoldenEveryMode(t *testing.T) {
	type mode struct {
		name     string
		snapshot func(t *testing.T, dir string, safe bool) string
		set      func(req *app.ImportAcceptRequest)
	}
	modes := []mode{
		{"tasks", func(t *testing.T, dir string, safe bool) string {
			from := "../../examples/imports/openproject/work-packages.sample.json"
			result, err := app.NewOpenProjectImportService(filestore.New(dir)).Import(context.Background(), app.OpenProjectImportRequest{
				FromFile: from, Now: time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			if !safe {
				// Accepting the same snapshot twice leaves nothing to write.
				if _, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), app.ImportAcceptRequest{
					SnapshotID: result.Snapshot.ID, Apply: true, ActorID: "actor.human.operator",
					Now: time.Date(2026, 7, 10, 12, 40, 0, 0, time.UTC),
				}); err != nil {
					t.Fatal(err)
				}
			}
			return result.Snapshot.ID
		}, func(*app.ImportAcceptRequest) {}},
		{"time-entries", func(t *testing.T, dir string, safe bool) string {
			id := writeOpenProjectMappedSnapshotForTimeEntry(t, dir)
			if !safe {
				if _, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), app.ImportAcceptRequest{
					SnapshotID: id, Apply: true, AcceptTimeEntries: true, ActorID: "actor.human.operator",
					Now: time.Date(2026, 7, 23, 17, 0, 0, 0, time.UTC),
				}); err != nil {
					t.Fatal(err)
				}
			}
			return id
		}, func(req *app.ImportAcceptRequest) { req.AcceptTimeEntries = true }},
		{"resource-demands", func(t *testing.T, dir string, safe bool) string {
			status := "safe_to_accept"
			if !safe {
				status = "needs_review"
			}
			return writeOpenProjectMappedSnapshotForResourceDemand(t, dir, status, "task.op.1234", "resource.human.ivan")
		}, func(req *app.ImportAcceptRequest) { req.AcceptResourceDemands = true }},
		{"resource-links", func(t *testing.T, dir string, safe bool) string {
			resource := "resource.human.ivan"
			if !safe {
				resource = ""
			}
			return writeOpenProjectMappedSnapshotForResourceLink(t, dir, resource, "11")
		}, func(req *app.ImportAcceptRequest) { req.AcceptResourceLinks = true }},
		{"tracking-status", func(t *testing.T, dir string, safe bool) string {
			task := "task.op.1234"
			if !safe {
				task = "task.op.missing"
			}
			return writeOpenProjectMappedSnapshotForTrackingStatus(t, dir, "in_progress", task)
		}, func(req *app.ImportAcceptRequest) { req.AcceptTrackingStatus = true }},
		{"dates", func(t *testing.T, dir string, safe bool) string {
			start, finish := "2026-07-08", "2026-07-12"
			if !safe {
				start, finish = "", ""
			}
			return writeOpenProjectMappedSnapshotForDates(t, dir, "task.op.1234", start, finish)
		}, func(req *app.ImportAcceptRequest) { req.AcceptDates = true }},
		{"dependencies", func(t *testing.T, dir string, safe bool) string {
			status := "safe_to_accept"
			if !safe {
				status = "needs_review"
			}
			return writeOpenProjectMappedSnapshotForDependency(t, dir, status, true, "task.op.1234", "task.offline.repair-pump", "88")
		}, func(req *app.ImportAcceptRequest) { req.AcceptDependencies = true }},
	}
	for _, m := range modes {
		for _, run := range []string{"dry-run", "apply", "apply-nothing-safe"} {
			t.Run(m.name+"/"+run, func(t *testing.T) {
				dir := copyProjectFixture(t)
				id := m.snapshot(t, dir, run != "apply-nothing-safe")
				before := snapshotFiles(t, dir)
				req := app.ImportAcceptRequest{
					SnapshotID: id,
					DryRun:     run == "dry-run",
					Apply:      run != "dry-run",
					ActorID:    "actor.human.operator",
					Reason:     "Golden run.",
					Now:        time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
				}
				m.set(&req)
				result, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), req)
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				changed := map[string]string{}
				for rel, content := range snapshotFiles(t, dir) {
					if before[rel] != content {
						changed[rel] = content
					}
				}
				record := map[string]any{"result": result, "error": errText, "changed_files": changed}
				got, err := json.MarshalIndent(record, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				got = []byte(strings.ReplaceAll(string(got), dir, "<project>") + "\n")
				path := filepath.Join("testdata", "import-accept", m.name+"."+run+".json")
				if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, got, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read golden %s: %v (REPLAN_UPDATE_GOLDEN=1 writes it)", path, err)
				}
				if string(got) != string(want) {
					t.Fatalf("result or written files differ from %s", path)
				}
			})
		}
	}
}
