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
