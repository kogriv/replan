package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestGitHubLivePaginationParametersAndPullRequestFiltering(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "must-not-be-read")
	var requests []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		assertGitHubHeaders(t, r, "")
		w.Header().Set("X-RateLimit-Remaining", "57")
		switch {
		case r.URL.Path == "/repos/acme/widgets":
			writeJSON(t, w, map[string]any{"name": "widgets", "html_url": "https://github.test/acme/widgets", "owner": map[string]any{"login": "acme"}})
		case r.URL.Path == "/repos/acme/widgets/issues" && r.URL.Query().Get("page") == "2":
			writeJSON(t, w, []map[string]any{{"id": 3, "number": 3, "title": "Second", "state": "closed", "state_reason": "completed", "updated_at": "2026-10-29T12:00:00Z"}})
		case r.URL.Path == "/repos/acme/widgets/issues":
			if r.URL.Query().Get("state") != "all" || r.URL.Query().Get("since") != "2026-10-29T00:00:00Z" || r.URL.Query().Get("per_page") != "100" {
				t.Errorf("issues query = %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", `<`+serverURL(r)+`/repos/acme/widgets/issues?page=2>; rel="next", <`+serverURL(r)+`/repos/acme/widgets/issues?page=2>; rel="last"`)
			writeJSON(t, w, []map[string]any{
				{"id": 1, "number": 1, "title": "First", "state": "open", "updated_at": "2026-10-29T01:00:00Z"},
				{"id": 2, "number": 2, "title": "PR", "state": "open", "updated_at": "2026-10-29T02:00:00Z", "pull_request": map[string]any{"url": "x"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := app.NewGitHubImportService(filestore.New("../../examples/small-team")).Import(context.Background(), app.GitHubImportRequest{
		Repository: "acme/widgets", BaseURL: server.URL, TokenEnvSet: true, UserAgent: "replan/test",
		Since: "2026-10-29", IncludeClosed: true, DryRun: true, RequestTimeoutSeconds: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TaskCandidates) != 2 || result.TaskCandidates[0].SourceID != "acme/widgets#1" || result.TaskCandidates[1].SourceID != "acme/widgets#3" {
		t.Fatalf("task candidates = %#v", result.TaskCandidates)
	}
	if result.RateLimitRemaining == nil || *result.RateLimitRemaining != 57 {
		t.Fatalf("rate remaining = %#v", result.RateLimitRemaining)
	}
	if !diagnosticsHaveCode(result.Warnings, "github_token_missing") || len(requests) != 3 {
		t.Fatalf("requests=%v warnings=%#v", requests, result.Warnings)
	}
}

func TestGitHubLiveRelationsAndClosedIssueSkip(t *testing.T) {
	relationReads := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets":
			writeJSON(t, w, map[string]any{"name": "widgets", "owner": map[string]any{"login": "acme"}})
		case "/repos/acme/widgets/issues":
			if r.URL.Query().Get("state") != "open" {
				t.Errorf("state = %q, want open", r.URL.Query().Get("state"))
			}
			writeJSON(t, w, []map[string]any{
				{"id": 10, "number": 10, "title": "Parent", "state": "open", "updated_at": "2026-10-29T01:00:00Z"},
				{"id": 11, "number": 11, "title": "Child", "state": "open", "updated_at": "2026-10-29T01:00:00Z"},
				{"id": 12, "number": 12, "title": "Closed", "state": "closed", "updated_at": "2026-10-29T01:00:00Z"},
			})
		case "/repos/acme/widgets/issues/10/dependencies/blocked_by":
			relationReads[r.URL.Path]++
			writeJSON(t, w, []map[string]any{{"id": 11, "number": 11, "html_url": "https://github.test/acme/widgets/issues/11"}})
		case "/repos/acme/widgets/issues/10/sub_issues":
			relationReads[r.URL.Path]++
			writeJSON(t, w, []map[string]any{{"id": 11, "number": 11, "html_url": "https://github.test/acme/widgets/issues/11"}})
		case "/repos/acme/widgets/issues/11/dependencies/blocked_by", "/repos/acme/widgets/issues/11/sub_issues":
			relationReads[r.URL.Path]++
			writeJSON(t, w, []any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := app.NewGitHubImportService(filestore.New("../../examples/small-team")).Import(context.Background(), app.GitHubImportRequest{
		Repository: "acme/widgets", BaseURL: server.URL, TokenEnvSet: true, DryRun: true, IncludeRelations: true, RequestTimeoutSeconds: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DependencyCandidates) != 1 {
		t.Fatalf("dependencies = %#v", result.DependencyCandidates)
	}
	if len(result.TaskCandidates) != 2 || result.TaskCandidates[1].ParentRef == nil || result.TaskCandidates[1].ParentRef.ObjectID != "acme/widgets#10" {
		t.Fatalf("task candidates = %#v", result.TaskCandidates)
	}
	for path := range relationReads {
		if strings.Contains(path, "/12/") {
			t.Fatalf("closed issue relation endpoint read: %s", path)
		}
	}
}

func TestGitHubLiveErrorsRateLimitAndRetry(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		headers    map[string]string
		wantCode   string
		wantReads  int
		retries    int
		succeed502 bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantCode: "source_unauthorized", wantReads: 1},
		{name: "not found", status: http.StatusNotFound, wantCode: "source_not_found", wantReads: 1},
		{name: "rate limited", status: http.StatusForbidden, headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1793289600"}, wantCode: "source_rate_limited", wantReads: 1, retries: 2},
		{name: "retry 502", status: http.StatusBadGateway, wantReads: 3, retries: 1, succeed502: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				for key, value := range test.headers {
					w.Header().Set(key, value)
				}
				if test.succeed502 && reads == 2 {
					writeJSON(t, w, map[string]any{"name": "widgets", "owner": map[string]any{"login": "acme"}})
					return
				}
				if test.succeed502 && r.URL.Path == "/repos/acme/widgets/issues" {
					writeJSON(t, w, []any{})
					return
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			result, err := app.NewGitHubImportService(filestore.New("../../examples/small-team")).Import(context.Background(), app.GitHubImportRequest{
				Repository: "acme/widgets", BaseURL: server.URL, TokenEnvSet: true, DryRun: true,
				RequestTimeoutSeconds: 2, RequestRetries: test.retries, RequestRetryDelaySeconds: 0.001,
			})
			if test.succeed502 {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !diagnosticsHaveCode(result.Errors, test.wantCode) {
				t.Fatalf("err=%v errors=%#v", err, result.Errors)
			}
			if reads != test.wantReads {
				t.Fatalf("reads=%d want %d", reads, test.wantReads)
			}
		})
	}
}

func TestGitHubLiveDocumentReplayAcceptAndTokenSafety(t *testing.T) {
	const token = "secret-token-that-must-not-appear"
	t.Setenv("TEST_GITHUB_TOKEN", token)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertGitHubHeaders(t, r, "Bearer "+token)
		switch r.URL.Path {
		case "/repos/example-org/planning-demo":
			writeJSON(t, w, map[string]any{"name": "planning-demo", "html_url": "https://github.test/example-org/planning-demo", "owner": map[string]any{"login": "example-org"}})
		case "/repos/example-org/planning-demo/issues":
			writeJSON(t, w, []map[string]any{{"id": 1000101, "number": 101, "title": "Implement live import", "state": "open", "updated_at": "2026-10-29T01:00:00Z", "labels": []map[string]any{{"name": "in progress"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := copyProjectFixture(t)
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	request := app.GitHubImportRequest{Profile: "github.demo", BaseURL: server.URL, TokenEnv: "TEST_GITHUB_TOKEN", TokenEnvSet: true, DryRun: false, Now: now, RequestTimeoutSeconds: 2}
	live, err := app.NewGitHubImportService(filestore.New(dir)).Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(dir, "imports/github/20261029T150000/raw.json")
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("token appears in raw snapshot")
	}
	var liveDocument app.GitHubImportDocument
	if err := json.Unmarshal(raw, &liveDocument); err != nil {
		t.Fatal(err)
	}
	wantDocument := app.GitHubImportDocument{
		Format:     app.GitHubImportFormatV1,
		Repository: app.GitHubRepository{Owner: "example-org", Name: "planning-demo", HTMLURL: "https://github.test/example-org/planning-demo"},
		Issues: []app.GitHubIssue{{
			ID: 1000101, Number: 101, Title: "Implement live import", State: "open",
			UpdatedAt: "2026-10-29T01:00:00Z", Labels: []app.GitHubLabel{{Name: "in progress"}},
		}},
	}
	if !reflect.DeepEqual(liveDocument, wantDocument) {
		t.Fatalf("live document = %#v, want %#v", liveDocument, wantDocument)
	}
	replay, err := app.NewGitHubImportService(filestore.New(dir)).Import(context.Background(), app.GitHubImportRequest{FromFile: rawPath, Profile: "github.demo", DryRun: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(live.TaskCandidates, replay.TaskCandidates) || !reflect.DeepEqual(live.DependencyCandidates, replay.DependencyCandidates) {
		t.Fatalf("live and replay differ\nlive=%#v\nreplay=%#v", live.TaskCandidates, replay.TaskCandidates)
	}
	accept, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), app.ImportAcceptRequest{SnapshotID: live.Snapshot.ID, DryRun: true, ActorID: "actor.human.operator"})
	if err != nil || len(accept.PlannedWrites) == 0 {
		t.Fatalf("accept err=%v result=%#v", err, accept)
	}
	encoded, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) {
		t.Fatal("token appears in result")
	}
}

func assertGitHubHeaders(t *testing.T, r *http.Request, authorization string) {
	t.Helper()
	if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" || r.Header.Get("User-Agent") == "" || r.Header.Get("Authorization") != authorization {
		t.Errorf("headers = %#v", r.Header)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
