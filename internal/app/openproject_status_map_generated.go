// Code generated from scripts/lib/openproject_status_map.json; DO NOT EDIT.

package app

var openProjectStatusByNormalizedName = map[string]string{
	"new":             "not_started",
	"open":            "not_started",
	"tobescheduled":   "ready",
	"scheduled":       "ready",
	"inspecification": "ready",
	"specified":       "ready",
	"confirmed":       "ready",
	"inprogress":      "in_progress",
	"progress":        "in_progress",
	"development":     "in_progress",
	"developing":      "in_progress",
	"onhold":          "blocked",
	"blocked":         "blocked",
	"developed":       "review_ready",
	"review":          "review_ready",
	"readyforreview":  "review_ready",
	"intesting":       "review_ready",
	"tested":          "review_ready",
	"closed":          "accepted",
	"done":            "accepted",
	"resolved":        "accepted",
	"accepted":        "accepted",
	"rejected":        "cancelled",
	"cancelled":       "cancelled",
	"canceled":        "cancelled",
}
