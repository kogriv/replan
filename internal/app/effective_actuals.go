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
		if correction, ok := latest[actual.ID]; ok {
			if correction.CorrectedAmount != nil {
				if *correction.CorrectedAmount == 0 {
					continue
				}
				actual.Amount = *correction.CorrectedAmount
				actual.CorrectedAmount = correction.CorrectedAmount
			}
			if correction.CorrectedStatusDate != "" {
				actual.StatusDate = correction.CorrectedStatusDate
				actual.CorrectedStatusDate = correction.CorrectedStatusDate
			}
		}
		result = append(result, actual)
	}
	return result
}

// EffectiveStatusAudits returns status-change records with the latest
// append-only status-date correction projected onto each original record.
// Correction audit records are audit evidence, not additional status events.
func EffectiveStatusAudits(records []domain.AuditRecord) []domain.AuditRecord {
	latest := LatestStatusCorrections(records)
	result := make([]domain.AuditRecord, 0, len(records))
	for _, record := range records {
		if record.Operation != "actual_add_status" {
			continue
		}
		if correction, ok := latest[record.ID]; ok {
			record.StatusDate = correction.StatusDate
		}
		result = append(result, record)
	}
	return result
}

// LatestStatusCorrections returns the latest correction for each status audit.
func LatestStatusCorrections(records []domain.AuditRecord) map[string]domain.AuditRecord {
	latest := map[string]domain.AuditRecord{}
	for _, record := range records {
		if record.Operation != "actual_correct_status" || record.CorrectsRef == nil || record.CorrectsRef.ID == "" {
			continue
		}
		previous, ok := latest[record.CorrectsRef.ID]
		if !ok || auditCorrectionAfter(record, previous) {
			latest[record.CorrectsRef.ID] = record
		}
	}
	return latest
}

func auditCorrectionAfter(a, b domain.AuditRecord) bool {
	at, aErr := time.Parse(time.RFC3339, a.OccurredAt)
	bt, bErr := time.Parse(time.RFC3339, b.OccurredAt)
	if aErr == nil && bErr == nil && !at.Equal(bt) {
		return at.After(bt)
	}
	if a.OccurredAt != b.OccurredAt {
		return a.OccurredAt > b.OccurredAt
	}
	return a.ID > b.ID
}

// LatestActualCorrections returns the superseding correction for each original.
func LatestActualCorrections(actuals []domain.Actual) map[string]domain.Actual {
	latestAmount := map[string]domain.Actual{}
	latestDate := map[string]domain.Actual{}
	for _, actual := range actuals {
		if actual.Type != "correction" || actual.CorrectsRef.ID == "" {
			continue
		}
		if actual.CorrectedAmount != nil {
			previous, ok := latestAmount[actual.CorrectsRef.ID]
			if !ok || correctionAfter(actual, previous) {
				latestAmount[actual.CorrectsRef.ID] = actual
			}
		}
		if actual.CorrectedStatusDate != "" {
			previous, ok := latestDate[actual.CorrectsRef.ID]
			if !ok || correctionAfter(actual, previous) {
				latestDate[actual.CorrectsRef.ID] = actual
			}
		}
	}
	latest := make(map[string]domain.Actual, len(latestAmount)+len(latestDate))
	for id, correction := range latestAmount {
		latest[id] = correction
	}
	for id, correction := range latestDate {
		merged := latest[id]
		if merged.ID == "" {
			merged = correction
		} else {
			merged.CorrectedStatusDate = correction.CorrectedStatusDate
		}
		latest[id] = merged
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
