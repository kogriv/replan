package domain

import "testing"

func TestISOWeekRange(t *testing.T) {
	for _, tc := range []struct{ label, start, end string }{
		{"", "", ""},
		{"2025-W53", "", ""},
		{"2016-W53", "", ""},
		{"2026-W53", "2026-12-28", "2027-01-03"},
		{"2026-W01", "2025-12-29", "2026-01-04"},
		{"2026-W00", "", ""},
		{"2026-W54", "", ""},
		{"0000-W01", "", ""},
		{"+026-W01", "", ""},
		{"2026-W+1", "", ""},
		{"2026-W1", "", ""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			start, end, ok := ISOWeekRange(tc.label)
			if ok != (tc.start != "") {
				t.Fatalf("ok = %v, want %v", ok, tc.start != "")
			}
			if ok && (start.Format("2006-01-02") != tc.start || end.Format("2006-01-02") != tc.end) {
				t.Fatalf("bounds = %v to %v, want %v to %v", start, end, tc.start, tc.end)
			}
		})
	}
}
