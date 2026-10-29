package reports

import "testing"

func TestFormatChangeValueShowsStructuredValuesAsJSON(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{nil, ""},
		{"2026-08-17", "2026-08-17"},
		{1.25, "1.25"},
		{map[string]any{"type": "unavailable", "date": "2026-07-23", "resource_ref": map[string]any{"kind": "resource", "id": "resource.human.maintainer"}}, `{"date":"2026-07-23","resource_ref":{"id":"resource.human.maintainer","kind":"resource"},"type":"unavailable"}`},
	}
	for _, test := range tests {
		if got := formatChangeValue(test.value); got != test.want {
			t.Fatalf("formatChangeValue(%#v) = %q, want %q", test.value, got, test.want)
		}
	}
}
