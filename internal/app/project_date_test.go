package app

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestProjectDateFallbacks(t *testing.T) {
	project := domain.Project{Timezone: "Asia/Novosibirsk"}
	for _, test := range []struct {
		name, timezone, input, want string
	}{
		{name: "date only", timezone: project.Timezone, input: "2026-11-08", want: "2026-11-08"},
		{name: "invalid timestamp", timezone: project.Timezone, input: "2026-11-08 later", want: "2026-11-08"},
		{name: "invalid zone", timezone: "Mars/Olympus", input: "2026-11-08T23:59:53Z", want: "2026-11-08"},
		{name: "missing zone", input: "2026-11-08T23:59:53Z", want: "2026-11-08"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := projectDate(domain.Project{Timezone: test.timezone}, test.input); got != test.want {
				t.Fatalf("projectDate(%q, %q) = %q, want %q", test.timezone, test.input, got, test.want)
			}
		})
	}
}
