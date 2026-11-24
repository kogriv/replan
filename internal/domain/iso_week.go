package domain

import (
	"strconv"
	"time"
)

// ISOWeekRange parses a canonical YYYY-Www label and returns inclusive UTC dates.
func ISOWeekRange(label string) (start, end time.Time, ok bool) {
	if len(label) != 8 || label[4:6] != "-W" {
		return time.Time{}, time.Time{}, false
	}
	for _, i := range [...]int{0, 1, 2, 3, 6, 7} {
		if label[i] < '0' || label[i] > '9' {
			return time.Time{}, time.Time{}, false
		}
	}
	year, _ := strconv.Atoi(label[:4])
	week, _ := strconv.Atoi(label[6:])
	_, lastWeek := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	if year == 0 || week < 1 || week > lastWeek {
		return time.Time{}, time.Time{}, false
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	weekday := (int(jan4.Weekday()) + 6) % 7
	start = jan4.AddDate(0, 0, (week-1)*7-weekday)
	return start, start.AddDate(0, 0, 6), true
}
