package app_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestRefreshExistingSelfOpenProjectEntries(t *testing.T) {
	sample := selfOpenProjectSample{}
	sample.Embedded.Elements = []json.RawMessage{
		json.RawMessage(`{"id":9999,"untouched":{"name":"keep"},"subject":"Old title","startDate":"2026-01-01","dueDate":"2026-01-02","tail":true}`),
		json.RawMessage(`{"id":2102,"subject":"Old listed title","startDate":"2026-02-01","dueDate":"2026-02-02","status":"keep"}`),
		json.RawMessage(`{"id":9998,"subject":"Old undated title","startDate":"keep-start","dueDate":"keep-finish","other":7}`),
	}
	tasks := []domain.Task{
		{ID: "task.refresh", Title: "New title", PlannedStart: "2026-03-01", PlannedFinish: "2026-03-02"},
		{ID: "task.listed", Title: "New listed title", PlannedStart: "2026-04-01", PlannedFinish: "2026-04-02"},
		{ID: "task.undated", Title: "New undated title"},
	}
	linkedTasks := map[string]string{
		"task.refresh": "9999",
		"task.listed":  "2102",
		"task.undated": "9998",
	}

	if err := refreshExistingSelfOpenProjectEntries(&sample, tasks, linkedTasks, map[string]struct{}{"2102": {}}); err != nil {
		t.Fatalf("refresh entries: %v", err)
	}

	want := []json.RawMessage{
		json.RawMessage(`{"id":9999,"untouched":{"name":"keep"},"subject":"New title","startDate":"2026-03-01","dueDate":"2026-03-02","tail":true}`),
		json.RawMessage(`{"id":2102,"subject":"New listed title","startDate":"2026-02-01","dueDate":"2026-02-02","status":"keep"}`),
		json.RawMessage(`{"id":9998,"subject":"New undated title","startDate":"keep-start","dueDate":"keep-finish","other":7}`),
	}
	if !reflect.DeepEqual(sample.Embedded.Elements, want) {
		t.Fatalf("refreshed elements differ:\n got: %s\nwant: %s", sample.Embedded.Elements, want)
	}
	for index := range want {
		if !bytes.Equal(sample.Embedded.Elements[index], want[index]) {
			t.Fatalf("element %d changed bytes outside selected values:\n got: %s\nwant: %s", index, sample.Embedded.Elements[index], want[index])
		}
	}
}

func TestCheckSelfOpenProjectLinkedFields(t *testing.T) {
	const updateSuffix = "; update command: " + selfOpenProjectUpdateCmd
	const listedSuffix = "; the update command keeps listed dates, so remove the id from selfOpenProjectDeliberateDateDriftIDs and lower the smoke's review_actions count"
	tests := []struct {
		name          string
		elements      []json.RawMessage
		task          domain.Task
		workPackageID string
		deliberate    map[string]struct{}
		want          []string
	}{
		{
			name:          "stale subject",
			elements:      []json.RawMessage{json.RawMessage(`{"id":9999,"subject":"Stale title","startDate":"2026-03-01","dueDate":"2026-03-02"}`)},
			task:          domain.Task{ID: "task.test", Title: "Canonical title", PlannedStart: "2026-03-01", PlannedFinish: "2026-03-02"},
			workPackageID: "9999",
			want:          []string{`work package 9999 field subject: sample "Stale title"; canonical "Canonical title"` + updateSuffix},
		},
		{
			name:          "stale date",
			elements:      []json.RawMessage{json.RawMessage(`{"id":9999,"subject":"Canonical title","startDate":"2026-02-28","dueDate":"2026-03-02"}`)},
			task:          domain.Task{ID: "task.test", Title: "Canonical title", PlannedStart: "2026-03-01", PlannedFinish: "2026-03-02"},
			workPackageID: "9999",
			want:          []string{`work package 9999 field startDate: sample "2026-02-28"; canonical "2026-03-01"` + updateSuffix},
		},
		{
			name:          "listed entry no longer drifts",
			elements:      []json.RawMessage{json.RawMessage(`{"id":2102,"subject":"Canonical title","startDate":"2026-03-01","dueDate":"2026-03-02"}`)},
			task:          domain.Task{ID: "task.test", Title: "Canonical title", PlannedStart: "2026-03-01", PlannedFinish: "2026-03-02"},
			workPackageID: "2102",
			deliberate:    map[string]struct{}{"2102": {}},
			want: []string{
				`work package 2102 field deliberate date drift: sample startDate="2026-03-01", dueDate="2026-03-02"; canonical planned_start="2026-03-01", planned_finish="2026-03-02"` + listedSuffix,
			},
		},
		{
			name:          "listed entry missing",
			task:          domain.Task{ID: "task.test", Title: "Canonical title", PlannedStart: "2026-03-01", PlannedFinish: "2026-03-02"},
			workPackageID: "2102",
			deliberate:    map[string]struct{}{"2102": {}},
			want: []string{
				`work package 2102 field deliberate date drift: sample <missing>; canonical planned_start="2026-03-01", planned_finish="2026-03-02"` + listedSuffix,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sample := selfOpenProjectSample{}
			sample.Embedded.Elements = test.elements
			got := checkSelfOpenProjectLinkedFields(sample, []domain.Task{test.task}, map[string]string{test.task.ID: test.workPackageID}, test.deliberate)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("problems differ:\n got: %#v\nwant: %#v", got, test.want)
			}
		})
	}
}
