package app

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestLatestCorrectionComparesInstantsAcrossOffsets(t *testing.T) {
	amount := func(value float64) *float64 { return &value }
	actuals := []domain.Actual{
		{ID: "actual.a", Type: "work_consumed", Amount: 0.33},
		// 10:00 in UTC+07 is 03:00 UTC: earlier than the 05:00 UTC one below,
		// although its text sorts later.
		{ID: "actual.fix-2", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.a"}, CorrectedAmount: amount(0.2), OccurredAt: "2026-10-30T10:00:00+07:00"},
		{ID: "actual.fix-1", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.a"}, CorrectedAmount: amount(0.1), OccurredAt: "2026-10-30T05:00:00Z"},
	}
	effective := EffectiveActuals(actuals)
	if len(effective) != 1 || effective[0].Amount != 0.1 {
		t.Fatalf("effective = %+v, want actual.a at 0.1", effective)
	}
}

func TestEffectiveStatusAuditsSortsByOccurrenceInstant(t *testing.T) {
	records := []domain.AuditRecord{
		{ID: "audit.finish", Operation: "actual_add_status", OccurredAt: "2026-07-10T10:00:00Z", NewStatus: "accepted"},
		{ID: "audit.start", Operation: "actual_add_status", OccurredAt: "2026-07-05T10:00:00+07:00", NewStatus: "in_progress"},
		{ID: "audit.middle", Operation: "actual_add_status", OccurredAt: "2026-07-06T10:00:00Z", NewStatus: "review_ready"},
	}
	effective := EffectiveStatusAudits(records)
	if len(effective) != 3 || effective[0].ID != "audit.start" || effective[1].ID != "audit.middle" || effective[2].ID != "audit.finish" {
		t.Fatalf("effective status order=%#v", effective)
	}
}

func TestEffectiveStatusAuditsOrdersHistoricalAppendByInstantThenID(t *testing.T) {
	records := []domain.AuditRecord{
		{ID: "audit.new", Operation: "actual_add_status", OccurredAt: "2026-11-06T12:00:00+07:00", NewStatus: "accepted"},
		{ID: "audit.old", Operation: "actual_add_status", OccurredAt: "2026-11-05T12:00:00+07:00", NewStatus: "review_ready"},
		{ID: "audit.same-b", Operation: "actual_add_status", OccurredAt: "2026-11-07T05:00:00Z", NewStatus: "blocked"},
		{ID: "audit.same-a", Operation: "actual_add_status", OccurredAt: "2026-11-07T12:00:00+07:00", NewStatus: "in_progress"},
	}
	effective := EffectiveStatusAudits(records)
	want := []string{"audit.old", "audit.new", "audit.same-a", "audit.same-b"}
	for index, id := range want {
		if effective[index].ID != id {
			t.Fatalf("effective ids = %#v, want %#v", []string{effective[0].ID, effective[1].ID, effective[2].ID, effective[3].ID}, want)
		}
	}
}

func TestWaitTerminalStatus(t *testing.T) {
	for _, status := range []string{"accepted", "done", "closed", "cancelled"} {
		if !waitTerminalStatus(status) {
			t.Errorf("%s is not terminal", status)
		}
	}
	if waitTerminalStatus("review_ready") {
		t.Error("review_ready is terminal")
	}
}
