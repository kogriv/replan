package app

import (
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

const calendarDateLayout = "2006-01-02"

// projectDate returns the calendar date of timestamp in the project's time
// zone. Date-only input is already a calendar date and is preserved. Invalid
// timestamps and unavailable project zones retain the timestamp's own date so
// callers keep the historical, permissive fallback behavior.
func projectDate(project domain.Project, timestamp string) string {
	if _, err := time.Parse(calendarDateLayout, timestamp); err == nil {
		return timestamp
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || strings.TrimSpace(project.Timezone) == "" {
		return datePart(timestamp)
	}
	zone, err := time.LoadLocation(project.Timezone)
	if err != nil {
		return datePart(timestamp)
	}
	return parsed.In(zone).Format(calendarDateLayout)
}
