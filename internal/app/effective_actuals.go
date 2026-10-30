package app

import (
	"time"

	"github.com/kogriv/replan/internal/domain"
)

// EffectiveActuals is the single consumption projection for append-only
// actuals. Correction records never become consumption themselves. The latest
// correction (by occurred_at, then id) replaces the original amount; a zero
// corrected amount voids the original from the returned slice.
func EffectiveActuals(actuals []domain.Actual) []domain.Actual {
	latest := LatestActualCorrections(actuals)
	result := make([]domain.Actual, 0, len(actuals))
	for _, actual := range actuals {
		if actual.Type == "correction" {
			continue
		}
		if correction, ok := latest[actual.ID]; ok && correction.CorrectedAmount != nil {
			if *correction.CorrectedAmount == 0 {
				continue
			}
			actual.Amount = *correction.CorrectedAmount
			actual.CorrectedAmount = correction.CorrectedAmount
		}
		result = append(result, actual)
	}
	return result
}

// LatestActualCorrections returns the superseding correction for each original.
func LatestActualCorrections(actuals []domain.Actual) map[string]domain.Actual {
	latest := map[string]domain.Actual{}
	for _, actual := range actuals {
		if actual.Type != "correction" || actual.CorrectsRef.ID == "" {
			continue
		}
		previous, ok := latest[actual.CorrectsRef.ID]
		if !ok || correctionAfter(actual, previous) {
			latest[actual.CorrectsRef.ID] = actual
		}
	}
	return latest
}

// correctionAfter orders corrections by their instant, so offsets do not
// matter, then by id; unparsable times fall back to text order.
func correctionAfter(a, b domain.Actual) bool {
	at, aErr := time.Parse(time.RFC3339, a.OccurredAt)
	bt, bErr := time.Parse(time.RFC3339, b.OccurredAt)
	if aErr == nil && bErr == nil && !at.Equal(bt) {
		return at.After(bt)
	}
	if aErr != nil || bErr != nil {
		if a.OccurredAt != b.OccurredAt {
			return a.OccurredAt > b.OccurredAt
		}
	}
	return a.ID > b.ID
}
