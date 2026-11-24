package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkloadSharedCapacityJSON(t *testing.T) {
	project := copyCmdProjectFixture(t)
	demands := `resource_demands:
  - id: resource_demand.direct
    task_ref: {kind: task, id: task.op.1234}
    resource_ref: {kind: resource, id: resource.human.ivan}
    unit: hours
    status: active
    remaining_amount: 24
    timing: {preferred_start: "2026-07-06", preferred_finish: "2026-07-06"}
  - id: resource_demand.pooled
    task_ref: {kind: task, id: task.op.1234}
    resource_pool_ref: {kind: resource_pool, id: resource_pool.backend_developers}
    unit: hours
    status: active
    remaining_amount: 24
    timing: {preferred_start: "2026-07-06", preferred_finish: "2026-07-06"}
`
	if err := os.WriteFile(filepath.Join(project, "canonical", "resource_demands.yaml"), []byte(demands), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := tempOutputFiles(t)
	if code := run([]string{"workload", "--project", project, "--week", "2026-W28", "--resource", "resource.human.ivan"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	type warning struct {
		Code   string         `json:"code"`
		Detail map[string]any `json:"shared_capacity"`
	}
	var env struct {
		OK       bool      `json:"ok"`
		Warnings []warning `json:"warnings"`
		Result   struct {
			Warnings []warning `json:"warnings"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || !reflect.DeepEqual(env.Warnings, env.Result.Warnings) {
		t.Fatalf("unexpected workload envelope: %#v", env)
	}
	want := map[string]any{
		"pool_ids": []any{"resource_pool.backend_developers"}, "member_ids": []any{"resource.human.ivan"},
		"unit": "hours", "demand": 24.0, "usable_capacity": 8.0, "shortfall": 16.0,
	}
	for _, w := range env.Warnings {
		if w.Code == "shared_capacity_overload" {
			if !reflect.DeepEqual(w.Detail, want) {
				t.Fatalf("shared_capacity = %#v, want %#v", w.Detail, want)
			}
			return
		}
	}
	t.Fatalf("shared capacity warning missing from %#v", env.Warnings)
}
