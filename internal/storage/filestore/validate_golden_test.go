package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kogriv/replan/internal/app"
)

// The diagnostics of the small-team example, in order, are a golden file:
// the validator split of GAP-020 must not change a single one.
func TestValidateSmallTeamGolden(t *testing.T) {
	result, _ := app.NewValidateService(New(filepath.Join("..", "..", "..", "examples", "small-team"))).Validate(context.Background(), app.ValidateRequest{})
	got, err := json.MarshalIndent(map[string]any{"errors": result.Errors, "warnings": result.Warnings}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "validate-small-team.json")
	if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
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
		t.Fatalf("diagnostics differ from %s", path)
	}
}
