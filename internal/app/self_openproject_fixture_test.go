package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

const (
	selfOpenProjectSamplePath   = "../../examples/imports/openproject/replan-self.work-packages.sample.json"
	selfOpenProjectBindingsPath = "../../examples/openproject/private-bindings.replan-self.example.yaml"
	selfOpenProjectUpdateCmd    = "REPLAN_UPDATE_GOLDEN=1 go test ./internal/app -run TestSelfOpenProjectSampleAgreesWithCanonicalTasks"
)

// These work packages deliberately keep their older dates so the smoke test's
// OpenProject date-push dry run continues to exercise exactly 23 review actions.
var selfOpenProjectDeliberateDateDriftIDs = map[string]struct{}{
	"2102": {}, "2103": {}, "2104": {}, "2105": {}, "2108": {},
	"2109": {}, "2110": {}, "2111": {}, "2112": {}, "2113": {},
	"2114": {}, "2115": {}, "2116": {}, "2117": {}, "2118": {},
	"2119": {}, "2120": {}, "2121": {}, "2122": {}, "2136": {},
	"2137": {}, "2138": {}, "2139": {},
}

type selfOpenProjectSample struct {
	Type     string `json:"_type"`
	Total    int    `json:"total"`
	Count    int    `json:"count"`
	Embedded struct {
		Elements    []json.RawMessage `json:"elements"`
		Relations   json.RawMessage   `json:"relations"`
		TimeEntries json.RawMessage   `json:"time_entries"`
	} `json:"_embedded"`
}

type selfOpenProjectWorkPackage struct {
	ID          int                    `json:"id"`
	Subject     string                 `json:"subject"`
	Description selfOpenProjectText    `json:"description"`
	Status      selfOpenProjectName    `json:"status"`
	Assignee    selfOpenProjectName    `json:"assignee"`
	Estimate    string                 `json:"estimatedTime"`
	Spent       string                 `json:"spentTime"`
	StartDate   string                 `json:"startDate"`
	DueDate     string                 `json:"dueDate"`
	Links       selfOpenProjectWPLinks `json:"_links"`
}

type selfOpenProjectText struct {
	Raw string `json:"raw"`
}

type selfOpenProjectName struct {
	Name string `json:"name"`
}

type selfOpenProjectLink struct {
	Href  string `json:"href,omitempty"`
	Title string `json:"title,omitempty"`
}

type selfOpenProjectWPLinks struct {
	Self     selfOpenProjectLink `json:"self"`
	Status   selfOpenProjectLink `json:"status"`
	Assignee selfOpenProjectLink `json:"assignee"`
}

func TestSelfOpenProjectSampleAgreesWithCanonicalTasks(t *testing.T) {
	store := filestore.New("../../examples/replan-self")
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("load self fixture: %#v", diagnostics)
	}

	raw, err := os.ReadFile(selfOpenProjectSamplePath)
	if err != nil {
		t.Fatalf("read %s: %v", selfOpenProjectSamplePath, err)
	}
	var sample selfOpenProjectSample
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("decode %s: %v", selfOpenProjectSamplePath, err)
	}
	// The update helper must start from the stable format so refreshing selected
	// values and appending missing entries cannot rewrite unrelated JSON.
	if rendered := encodeSelfOpenProjectSample(t, sample); !bytes.Equal(rendered, raw) {
		t.Fatalf("%s is not in the generator's stable format; REPLAN_UPDATE_GOLDEN=1 must not rewrite existing entries", selfOpenProjectSamplePath)
	}

	linkedTasks := openProjectLinkedTasks(workspace.Tasks)
	if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
		updateSelfOpenProjectSample(t, &sample, workspace, linkedTasks)
		raw = encodeSelfOpenProjectSample(t, sample)
		if err := os.WriteFile(selfOpenProjectSamplePath, raw, 0o644); err != nil {
			t.Fatalf("update %s: %v", selfOpenProjectSamplePath, err)
		}
	}

	sampleTaskByWorkPackage := make(map[string]string, len(sample.Embedded.Elements))
	var duplicateWorkPackageIDs []string
	for _, element := range sample.Embedded.Elements {
		var workPackage struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(element, &workPackage); err != nil {
			t.Fatalf("decode work package in %s: %v", selfOpenProjectSamplePath, err)
		}
		workPackageID := strconv.Itoa(workPackage.ID)
		if _, exists := sampleTaskByWorkPackage[workPackageID]; exists {
			duplicateWorkPackageIDs = append(duplicateWorkPackageIDs, workPackageID)
		}
		sampleTaskByWorkPackage[workPackageID] = ""
	}
	for taskID, workPackageID := range linkedTasks {
		if _, exists := sampleTaskByWorkPackage[workPackageID]; exists {
			sampleTaskByWorkPackage[workPackageID] = taskID
		}
	}

	var missingTaskIDs, extraWorkPackageIDs []string
	for taskID, workPackageID := range linkedTasks {
		if sampleTaskByWorkPackage[workPackageID] == "" {
			missingTaskIDs = append(missingTaskIDs, taskID)
		}
	}
	for workPackageID, taskID := range sampleTaskByWorkPackage {
		if taskID == "" {
			extraWorkPackageIDs = append(extraWorkPackageIDs, workPackageID)
		}
	}
	sort.Strings(missingTaskIDs)
	sort.Strings(extraWorkPackageIDs)
	sort.Strings(duplicateWorkPackageIDs)
	elementCount := len(sample.Embedded.Elements)
	if problems := checkSelfOpenProjectLinkedFields(sample, workspace.Tasks, linkedTasks, selfOpenProjectDeliberateDateDriftIDs); len(problems) != 0 {
		t.Fatalf("self OpenProject fixture field agreement failed:\n%s", strings.Join(problems, "\n"))
	}
	if len(missingTaskIDs) != 0 || len(extraWorkPackageIDs) != 0 || len(duplicateWorkPackageIDs) != 0 || sample.Total != elementCount || sample.Count != elementCount {
		t.Fatalf("self OpenProject fixture agreement failed: missing task ids %v; extra work package ids %v; duplicate work package ids %v; total=%d count=%d elements=%d. Add each missing task's work package to %s (%s), remove extras/duplicates, and keep total/count equal to the element count", missingTaskIDs, extraWorkPackageIDs, duplicateWorkPackageIDs, sample.Total, sample.Count, elementCount, selfOpenProjectSamplePath, selfOpenProjectUpdateCmd)
	}
}

func TestSelfOpenProjectBindingsAgreeWithCanonicalTasks(t *testing.T) {
	store := filestore.New("../../examples/replan-self")
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("load self fixture: %#v", diagnostics)
	}
	raw, err := os.ReadFile(selfOpenProjectBindingsPath)
	if err != nil {
		t.Fatalf("read %s: %v", selfOpenProjectBindingsPath, err)
	}
	var document struct {
		Bindings struct {
			Tasks []struct {
				TaskID    string `yaml:"replan_task_id"`
				SourceRef struct {
					ObjectID string `yaml:"object_id"`
				} `yaml:"source_ref"`
			} `yaml:"tasks"`
		} `yaml:"openproject_private_bindings"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode %s: %v", selfOpenProjectBindingsPath, err)
	}

	linkedTasks := openProjectLinkedTasks(workspace.Tasks)
	bound := make(map[string]string, len(document.Bindings.Tasks))
	var problems []string
	for _, binding := range document.Bindings.Tasks {
		if _, exists := bound[binding.TaskID]; exists {
			problems = append(problems, "duplicate binding for "+binding.TaskID)
		}
		bound[binding.TaskID] = binding.SourceRef.ObjectID
		want, linked := linkedTasks[binding.TaskID]
		switch {
		case !linked:
			problems = append(problems, "extra binding for "+binding.TaskID)
		case want != binding.SourceRef.ObjectID:
			problems = append(problems, fmt.Sprintf("%s bound to %s, canonical source ref %s", binding.TaskID, binding.SourceRef.ObjectID, want))
		}
	}
	for taskID, workPackageID := range linkedTasks {
		if _, exists := bound[taskID]; !exists {
			problems = append(problems, fmt.Sprintf("missing binding for %s (work package %s)", taskID, workPackageID))
		}
	}
	sort.Strings(problems)
	if len(problems) != 0 {
		t.Fatalf("self OpenProject bindings agreement failed: %s. Add a task binding with the canonical sanitized source ref to %s for each missing task and remove extras", strings.Join(problems, "; "), selfOpenProjectBindingsPath)
	}
}

func openProjectLinkedTasks(tasks []domain.Task) map[string]string {
	result := make(map[string]string)
	for _, task := range tasks {
		for _, ref := range task.SourceRefs {
			if ref.Source == "openproject" && ref.ObjectType == "work_package" {
				result[task.ID] = ref.ObjectID
				break
			}
		}
	}
	return result
}

func updateSelfOpenProjectSample(t *testing.T, sample *selfOpenProjectSample, workspace domain.Workspace, linkedTasks map[string]string) {
	t.Helper()
	if err := refreshExistingSelfOpenProjectEntries(sample, workspace.Tasks, linkedTasks, selfOpenProjectDeliberateDateDriftIDs); err != nil {
		t.Fatalf("refresh existing work packages: %v", err)
	}
	existing := make(map[string]bool, len(sample.Embedded.Elements))
	var assigneeTemplate selfOpenProjectWorkPackage
	for index, element := range sample.Embedded.Elements {
		var workPackage selfOpenProjectWorkPackage
		if err := json.Unmarshal(element, &workPackage); err != nil {
			t.Fatalf("decode existing work package: %v", err)
		}
		existing[strconv.Itoa(workPackage.ID)] = true
		if index == 0 {
			assigneeTemplate = workPackage
		}
	}
	demandsByTask := make(map[string][]domain.ResourceDemand)
	for _, demand := range workspace.ResourceDemands {
		demandsByTask[demand.TaskRef.ID] = append(demandsByTask[demand.TaskRef.ID], demand)
	}
	resourcesByID := make(map[string]domain.Resource)
	for _, resource := range workspace.Resources {
		resourcesByID[resource.ID] = resource
	}

	for _, task := range workspace.Tasks {
		workPackageID, linked := linkedTasks[task.ID]
		if !linked || existing[workPackageID] {
			continue
		}
		id, err := strconv.Atoi(workPackageID)
		if err != nil || id <= 0 {
			t.Fatalf("cannot generate work package for %s: OpenProject source id %q is not a positive integer", task.ID, workPackageID)
		}
		assignee := resourcesByID[task.AssigneeHint.ID].Name
		if assignee == "" {
			t.Fatalf("cannot generate work package for %s: assignee %q has no canonical resource name", task.ID, task.AssigneeHint.ID)
		}
		if assignee != assigneeTemplate.Assignee.Name || assigneeTemplate.Links.Assignee.Href == "" {
			t.Fatalf("cannot generate work package for %s: assignee %q is not represented by the sample's sanitized assignee", task.ID, task.AssigneeHint.ID)
		}
		status, ok := map[string]string{
			"not_started": "New", "ready": "To be scheduled", "in_progress": "In progress",
			"blocked": "On hold", "review_ready": "Developed", "accepted": "Closed", "cancelled": "Rejected",
		}[task.TrackingStatus]
		if !ok {
			t.Fatalf("cannot generate work package for %s: tracking status %q has no OpenProject fixture mapping", task.ID, task.TrackingStatus)
		}
		estimatedHours := 0.0
		for _, demand := range demandsByTask[task.ID] {
			if demand.Unit == "hours" {
				estimatedHours += demand.PlannedAmount
			}
		}
		workPackage := selfOpenProjectWorkPackage{
			ID: id, Subject: task.Title, Description: selfOpenProjectText{Raw: task.Description},
			Status: selfOpenProjectName{Name: status}, Assignee: selfOpenProjectName{Name: assignee},
			Estimate: isoHours(estimatedHours), Spent: "PT0H", StartDate: task.PlannedStart, DueDate: task.PlannedFinish,
			Links: selfOpenProjectWPLinks{
				Self:     selfOpenProjectLink{Href: "/api/v3/work_packages/" + workPackageID},
				Status:   selfOpenProjectLink{Title: status},
				Assignee: assigneeTemplate.Links.Assignee,
			},
		}
		element := encodeJSON(t, workPackage)
		sample.Embedded.Elements = append(sample.Embedded.Elements, json.RawMessage(strings.TrimSpace(string(element))))
		existing[workPackageID] = true
	}
	sample.Total = len(sample.Embedded.Elements)
	sample.Count = len(sample.Embedded.Elements)
}

func refreshExistingSelfOpenProjectEntries(sample *selfOpenProjectSample, tasks []domain.Task, linkedTasks map[string]string, deliberateDateDriftIDs map[string]struct{}) error {
	tasksByID := make(map[string]domain.Task, len(tasks))
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	taskIDsByWorkPackage := make(map[string]string, len(linkedTasks))
	for taskID, workPackageID := range linkedTasks {
		taskIDsByWorkPackage[workPackageID] = taskID
	}

	for index, element := range sample.Embedded.Elements {
		var workPackage struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(element, &workPackage); err != nil {
			return fmt.Errorf("decode existing work package: %w", err)
		}
		workPackageID := strconv.Itoa(workPackage.ID)
		taskID, linked := taskIDsByWorkPackage[workPackageID]
		if !linked {
			continue
		}
		task := tasksByID[taskID]
		refreshed, err := replaceJSONStringField(element, "subject", task.Title)
		if err != nil {
			return fmt.Errorf("refresh work package %s subject: %w", workPackageID, err)
		}
		if _, deliberate := deliberateDateDriftIDs[workPackageID]; !deliberate {
			if task.PlannedStart != "" {
				refreshed, err = replaceJSONStringField(refreshed, "startDate", task.PlannedStart)
				if err != nil {
					return fmt.Errorf("refresh work package %s startDate: %w", workPackageID, err)
				}
			}
			if task.PlannedFinish != "" {
				refreshed, err = replaceJSONStringField(refreshed, "dueDate", task.PlannedFinish)
				if err != nil {
					return fmt.Errorf("refresh work package %s dueDate: %w", workPackageID, err)
				}
			}
		}
		sample.Embedded.Elements[index] = refreshed
	}
	return nil
}

func replaceJSONStringField(raw json.RawMessage, field, value string) (json.RawMessage, error) {
	pattern, err := regexp.Compile(`("` + regexp.QuoteMeta(field) + `"\s*:\s*)"(?:\\.|[^"\\])*"`)
	if err != nil {
		return nil, fmt.Errorf("compile field matcher: %w", err)
	}
	match := pattern.FindAllSubmatchIndex(raw, -1)
	if len(match) != 1 {
		return nil, fmt.Errorf("expected one %q string field, found %d", field, len(match))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %q: %w", field, err)
	}
	prefixEnd := match[0][3]
	fieldEnd := match[0][1]
	result := make([]byte, 0, len(raw)-fieldEnd+prefixEnd+len(encoded))
	result = append(result, raw[:prefixEnd]...)
	result = append(result, encoded...)
	result = append(result, raw[fieldEnd:]...)
	return json.RawMessage(result), nil
}

func checkSelfOpenProjectLinkedFields(sample selfOpenProjectSample, tasks []domain.Task, linkedTasks map[string]string, deliberateDateDriftIDs map[string]struct{}) []string {
	tasksByID := make(map[string]domain.Task, len(tasks))
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	canonicalByWorkPackage := make(map[string]domain.Task, len(linkedTasks))
	for taskID, workPackageID := range linkedTasks {
		canonicalByWorkPackage[workPackageID] = tasksByID[taskID]
	}
	sampleByWorkPackage := make(map[string]selfOpenProjectWorkPackage, len(sample.Embedded.Elements))
	for _, element := range sample.Embedded.Elements {
		var workPackage selfOpenProjectWorkPackage
		if err := json.Unmarshal(element, &workPackage); err != nil {
			return []string{fmt.Sprintf("decode work package fields: %v; update command: %s", err, selfOpenProjectUpdateCmd)}
		}
		sampleByWorkPackage[strconv.Itoa(workPackage.ID)] = workPackage
	}

	var problems []string
	for workPackageID, task := range canonicalByWorkPackage {
		workPackage, exists := sampleByWorkPackage[workPackageID]
		if !exists {
			continue
		}
		if workPackage.Subject != task.Title {
			problems = append(problems, selfOpenProjectFieldProblem(workPackageID, "subject", workPackage.Subject, task.Title))
		}
		if _, deliberate := deliberateDateDriftIDs[workPackageID]; deliberate {
			continue
		}
		if task.PlannedStart != "" && workPackage.StartDate != task.PlannedStart {
			problems = append(problems, selfOpenProjectFieldProblem(workPackageID, "startDate", workPackage.StartDate, task.PlannedStart))
		}
		if task.PlannedFinish != "" && workPackage.DueDate != task.PlannedFinish {
			problems = append(problems, selfOpenProjectFieldProblem(workPackageID, "dueDate", workPackage.DueDate, task.PlannedFinish))
		}
	}
	for workPackageID := range deliberateDateDriftIDs {
		task, linked := canonicalByWorkPackage[workPackageID]
		workPackage, exists := sampleByWorkPackage[workPackageID]
		sampleDates := "<missing>"
		canonicalDates := "<no linked canonical task>"
		if linked {
			canonicalDates = fmt.Sprintf("planned_start=%q, planned_finish=%q", task.PlannedStart, task.PlannedFinish)
		}
		if exists {
			sampleDates = fmt.Sprintf("startDate=%q, dueDate=%q", workPackage.StartDate, workPackage.DueDate)
		}
		if !exists || !linked || !selfOpenProjectDatesDiffer(workPackage, task) {
			problems = append(problems, fmt.Sprintf("work package %s field deliberate date drift: sample %s; canonical %s; the update command keeps listed dates, so remove the id from selfOpenProjectDeliberateDateDriftIDs and lower the smoke's review_actions count", workPackageID, sampleDates, canonicalDates))
		}
	}
	sort.Strings(problems)
	return problems
}

func selfOpenProjectDatesDiffer(workPackage selfOpenProjectWorkPackage, task domain.Task) bool {
	return task.PlannedStart != "" && workPackage.StartDate != task.PlannedStart ||
		task.PlannedFinish != "" && workPackage.DueDate != task.PlannedFinish
}

func selfOpenProjectFieldProblem(workPackageID, field, sampleValue, canonicalValue string) string {
	return fmt.Sprintf("work package %s field %s: sample %q; canonical %q; update command: %s", workPackageID, field, sampleValue, canonicalValue, selfOpenProjectUpdateCmd)
}

func isoHours(hours float64) string {
	if hours == float64(int64(hours)) {
		return fmt.Sprintf("PT%dH", int64(hours))
	}
	return "PT" + strconv.FormatFloat(hours, 'f', -1, 64) + "H"
}

func encodeSelfOpenProjectSample(t *testing.T, sample selfOpenProjectSample) []byte {
	t.Helper()
	return encodeJSON(t, sample)
}

func encodeJSON(t *testing.T, value any) []byte {
	t.Helper()
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("encode generated OpenProject sample: %v", err)
	}
	return output.Bytes()
}
