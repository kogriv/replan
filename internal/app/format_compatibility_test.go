package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
	"gopkg.in/yaml.v3"
)

func TestFormatV1ReleaseFixturesRemainCompatible(t *testing.T) {
	for _, release := range []string{"v0.1.0", "v0.3.0"} {
		t.Run(release, func(t *testing.T) {
			source := filepath.Join("testdata", "format-v1", release)
			assertFrozenFieldsKnown(t, source)
			projectPath := filepath.Join(t.TempDir(), "small-team")
			if err := copyAppDir(source, projectPath); err != nil {
				t.Fatalf("copy frozen fixture: %v", err)
			}

			ctx := context.Background()
			store := filestore.New(projectPath)
			workspace, diagnostics := store.Load(ctx)
			if len(diagnostics) != 0 {
				t.Fatalf("load diagnostics: %#v", diagnostics)
			}
			if workspace.Project.FormatVersion != 1 {
				t.Fatalf("reported format version = %d, want 1", workspace.Project.FormatVersion)
			}

			validation, err := app.NewValidateService(store).Validate(ctx, app.ValidateRequest{ProjectPath: projectPath})
			if err != nil || !validation.Valid || len(validation.Errors) != 0 {
				t.Fatalf("validate: err=%v result=%#v", err, validation)
			}

			workload, err := app.NewWorkloadService(store).Calculate(ctx, app.WorkloadRequest{ProjectPath: projectPath, Week: "2026-W28"})
			if err != nil || len(workload.Errors) != 0 || len(workload.Resources) == 0 {
				t.Fatalf("workload: err=%v result=%#v", err, workload)
			}

			state, err := app.NewReportService(store).Dashboard(ctx, app.DashboardReportRequest{
				ProjectPath: projectPath,
				Week:        "2026-W28",
				GeneratedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
				Command:     "report state",
			})
			if err != nil || len(state.Errors) != 0 || state.ProjectID != workspace.Project.ID {
				t.Fatalf("report state: err=%v result=%#v", err, state)
			}

			migration, err := app.NewProjectService(store).Migrate(ctx, app.ProjectMigrateRequest{
				ActorID:    "actor.human.operator",
				OccurredAt: "2026-07-06T12:00:00+07:00",
				DryRun:     true,
			})
			if err != nil || len(migration.Errors) != 0 {
				t.Fatalf("project migrate dry run: err=%v result=%#v", err, migration)
			}
			if len(migration.Steps) > 1 || (len(migration.Steps) == 1 && migration.Steps[0].Name != "declare_format_version") {
				t.Fatalf("unexpected migration steps: %#v", migration.Steps)
			}

			assertSealsVerify(t, workspace)
		})
	}
}

// The production loader intentionally ignores unknown fields. Frozen fixtures
// also get this strict schema pass so renaming or removing a field they use
// cannot silently turn historical data into an ignored field.
func assertFrozenFieldsKnown(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := frozenYAMLTarget(filepath.ToSlash(rel))
		if target == nil {
			t.Fatalf("frozen YAML file has no strict decoder: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(target); err != nil {
			t.Fatalf("frozen fields are no longer recognized in %s: %v", rel, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk frozen fixture: %v", err)
	}
}

func frozenYAMLTarget(rel string) any {
	switch {
	case rel == ".replan/project.yaml":
		return &struct {
			Project domain.Project `yaml:"project"`
		}{}
	case rel == ".replan/policy.yaml":
		return &struct {
			Policy domain.Policy `yaml:"policy"`
		}{}
	case rel == "canonical/actors.yaml":
		return &struct {
			Actors []domain.Actor `yaml:"actors"`
		}{}
	case rel == "canonical/resources.yaml":
		return &struct {
			Resources []domain.Resource `yaml:"resources"`
		}{}
	case rel == "canonical/resource_pools.yaml":
		return &struct {
			ResourcePools []domain.ResourcePool `yaml:"resource_pools"`
		}{}
	case rel == "canonical/tasks.yaml":
		return &struct {
			Tasks []domain.Task `yaml:"tasks"`
		}{}
	case rel == "canonical/resource_demands.yaml":
		return &struct {
			ResourceDemands []domain.ResourceDemand `yaml:"resource_demands"`
		}{}
	case rel == "canonical/dependencies.yaml":
		return &struct {
			Dependencies []domain.Dependency `yaml:"dependencies"`
		}{}
	case rel == "canonical/deadlines.yaml":
		return &struct {
			Deadlines []any `yaml:"deadlines"`
		}{}
	case rel == "canonical/capacities.yaml":
		return &struct {
			Capacities []domain.Capacity `yaml:"capacities"`
		}{}
	case rel == "canonical/availability.yaml":
		return &struct {
			Availability []domain.Availability `yaml:"availability"`
		}{}
	case rel == "canonical/current_plan.yaml":
		return &struct {
			CurrentPlan domain.CurrentPlan `yaml:"current_plan"`
		}{}
	case strings.HasPrefix(rel, "baselines/"):
		return &struct {
			Baseline domain.Baseline `yaml:"baseline"`
		}{}
	case strings.HasPrefix(rel, "calibrations/"):
		return &struct {
			Calibration domain.Calibration `yaml:"calibration"`
		}{}
	case strings.HasPrefix(rel, "actuals/"):
		return &struct {
			Actuals []domain.Actual `yaml:"actuals"`
		}{}
	case strings.HasPrefix(rel, "evidence/"):
		return &struct {
			Evidence []domain.Evidence `yaml:"evidence"`
		}{}
	case strings.HasPrefix(rel, "proposals/"):
		return &struct {
			Proposal domain.Proposal `yaml:"proposal"`
		}{}
	case strings.HasPrefix(rel, "scenarios/"):
		return &struct {
			Scenario domain.Scenario `yaml:"scenario"`
		}{}
	default:
		return nil
	}
}

func TestUnknownTaskFieldLoadsAndSurvivesCanonicalSplice(t *testing.T) {
	source := filepath.Join("testdata", "format-v1", "v0.3.0")
	projectPath := filepath.Join(t.TempDir(), "small-team")
	if err := copyAppDir(source, projectPath); err != nil {
		t.Fatalf("copy frozen fixture: %v", err)
	}

	tasksPath := filepath.Join(projectPath, "canonical", "tasks.yaml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	const marker = "    future_optional_field: keep-me\n"
	updated := strings.Replace(string(data), "    title: Replace greenhouse pump\n", "    title: Replace greenhouse pump\n"+marker, 1)
	if updated == string(data) {
		t.Fatal("failed to add unknown task field")
	}
	if err := os.WriteFile(tasksPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	store := filestore.New(projectPath)
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 || len(workspace.Tasks) == 0 {
		t.Fatalf("unknown field did not load cleanly: diagnostics=%#v", diagnostics)
	}
	if _, err := store.SetTaskTitle("task.offline.repair-pump", "Replace greenhouse water pump"); err != nil {
		t.Fatalf("splice task title: %v", err)
	}
	after, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), marker) {
		t.Fatalf("canonical splice dropped unknown field:\n%s", after)
	}
}

func assertSealsVerify(t *testing.T, workspace domain.Workspace) {
	t.Helper()
	for _, baseline := range workspace.Baselines {
		if baseline.Seal == nil {
			continue
		}
		if baseline.Seal.Algorithm != "sha256" {
			t.Fatalf("baseline %s has unsupported seal algorithm %q", baseline.ID, baseline.Seal.Algorithm)
		}
		digest, err := domain.BaselineDigest(baseline)
		if err != nil || digest != baseline.Seal.Digest {
			t.Fatalf("baseline %s seal: digest=%q want=%q err=%v", baseline.ID, digest, baseline.Seal.Digest, err)
		}
	}
	for _, calibration := range workspace.Calibrations {
		if calibration.SealSHA256 == "" {
			continue
		}
		digest, err := domain.CalibrationDigest(calibration)
		if err != nil || digest != calibration.SealSHA256 {
			t.Fatalf("calibration %s seal: digest=%q want=%q err=%v", calibration.ID, digest, calibration.SealSHA256, err)
		}
	}
}
