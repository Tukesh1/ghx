package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() { retryDelay = time.Millisecond }

// fakeGitHub answers the dashboard's GraphQL queries and REST calls with small fixtures.
// failing names GraphQL operations (e.g. "GhxIssues") that answer 502.
func fakeGitHub(t *testing.T, failing ...string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, f := range failing {
			if strings.Contains(req.Query, "query "+f) {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
		}
		switch {
		case strings.Contains(req.Query, "query GhxViewer"):
			_, _ = w.Write([]byte(`{"data":{"viewer":{"login":"mona","name":"Mona","contributionsCollection":{"totalCommitContributions":5,
				"contributionCalendar":{"weeks":[{"contributionDays":[{"contributionCount":3,"date":"2026-10-25","weekday":0},{"contributionCount":1,"date":"2026-10-26","weekday":1}]}]}}}}}`))
		case strings.Contains(req.Query, "query GhxRepos"):
			_, _ = w.Write([]byte(`{"data":{"viewer":{"repositories":{"nodes":[{"nameWithOwner":"mona/app","stargazerCount":7,
				"primaryLanguage":{"name":"Go","color":"#00add8"},"languages":{"totalSize":100,"edges":[{"size":100,"node":{"name":"Go","color":"#00add8"}}]},
				"defaultBranchRef":{"name":"main","target":{"oid":"abcdef1234","messageHeadline":"init","statusCheckRollup":{"state":"SUCCESS"},"history":{"nodes":[]}}}}]}}}}`))
		case strings.Contains(req.Query, "query GhxPRs"):
			_, _ = w.Write([]byte(`{"data":{"search":{"nodes":[{"number":4,"title":"Add thing","mergeable":"MERGEABLE","reviewDecision":null,
				"headRefName":"feat","baseRefName":"main","author":{"login":"mona"},
				"repository":{"nameWithOwner":"mona/app","squashMergeAllowed":false,"mergeCommitAllowed":true},
				"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS","contexts":{"nodes":[{"name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}]}}}}]}},
				{}]}}}`))
		case strings.Contains(req.Query, "query GhxProfile"):
			_, _ = fmt.Fprintf(w, `{"data":{"viewer":{"login":"mona","name":"Mona","bio":"Hi there","createdAt":"2020-01-02T00:00:00Z",
				"avatarUrl":"%s/avatar.png","followers":{"totalCount":3},"following":{"totalCount":1},"status":{"emoji":":rocket:","message":"shipping"},
				"pinnedItems":{"nodes":[]},"allRepos":{"totalCount":4},
				"owned":{"pageInfo":{"hasNextPage":true,"endCursor":"page2"},"nodes":[{"nameWithOwner":"mona/app","stargazerCount":7,"forkCount":2,"primaryLanguage":{"name":"Go","color":"#00add8"}},
					{"nameWithOwner":"mona/secret","isPrivate":true,"stargazerCount":1}]},
				"contributionsCollection":{"commitContributionsByRepository":[{"repository":{"nameWithOwner":"mona/app"},"contributions":{"totalCount":12}},
					{"repository":{"nameWithOwner":"mona/secret","isPrivate":true},"contributions":{"totalCount":40}}]}}}}`, srv.URL)
		case strings.Contains(req.Query, "query GhxOwnedRepos"):
			// The second page of the viewer's repositories (more than 100 of them).
			_, _ = w.Write([]byte(`{"data":{"viewer":{"owned":{"pageInfo":{"hasNextPage":false},
				"nodes":[{"nameWithOwner":"mona/tool","stargazerCount":1,"primaryLanguage":{"name":"Rust","color":"#dea584"}}]}}}}`))
		case strings.Contains(req.Query, "query GhxIssues"):
			_, _ = w.Write([]byte(`{"data":{"search":{"nodes":[{"number":9,"title":"Bug","repository":{"nameWithOwner":"mona/app"},"labels":{"nodes":[{"name":"p1","color":"ff0000"}]}}]}}}`))
		default:
			_, _ = w.Write([]byte(`{"errors":[{"message":"unknown query"}]}`))
		}
	})
	mux.HandleFunc("/repos/mona/app/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"workflow_runs":[{"id":11,"workflow_id":5,"name":"CI","status":"completed","conclusion":"failure","head_branch":"main",
			"run_started_at":"2026-10-26T10:00:00Z","updated_at":"2026-10-26T10:03:00Z"}]}`))
	})
	mux.HandleFunc("/repos/mona/app/actions/runs/11/jobs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jobs":[{"id":21,"name":"test","status":"completed","conclusion":"failure","steps":[]}]}`))
	})
	mux.HandleFunc("/repos/mona/app/actions/jobs/21/logs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("2026-10-26T10:00:01.0000000Z running\n2026-10-26T10:00:02.0000000Z ##[error]boom\n"))
	})
	mux.HandleFunc("/repos/mona/app/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Dependabot alerts are disabled"}`))
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"1","reason":"mention","subject":{"title":"Hi","type":"PullRequest","url":"https://api.github.com/repos/mona/app/pulls/4"},"repository":{"full_name":"mona/app"}}]`))
	})
	mux.HandleFunc("/users/mona/orgs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"login":"public-org"}]`))
	})
	mux.HandleFunc("/repos/mona/mona/readme", func(w http.ResponseWriter, r *http.Request) {
		body := base64.StdEncoding.EncodeToString([]byte("# Hi\n<img src=\"x.gif\">\nHello **world**"))
		_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, body)
	})
	mux.HandleFunc("/avatar.png", func(w http.ResponseWriter, r *http.Request) {
		// Avatars live on another host: the token must never be sent there.
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for i := range img.Pix {
			img.Pix[i] = []byte{255, 0, 0, 255}[i%4]
		}
		img.Set(0, 0, color.RGBA{0, 0, 255, 255})
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		_, _ = w.Write(b.Bytes())
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestFetchAssemblesDashboard(t *testing.T) {
	srv, _ := fakeGitHub(t)
	d, err := NewTestClient("tok", srv.URL).Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if d.User.Login != "mona" || len(d.Repos) != 1 || len(d.Issues) != 1 || len(d.Contributions) != 2 {
		t.Fatalf("unexpected dashboard: user=%q repos=%d issues=%d days=%d", d.User.Login, len(d.Repos), len(d.Issues), len(d.Contributions))
	}
	// The same PR appears in both searches; it must only be listed once, in the review queue.
	if len(d.ReviewPRs) != 1 || len(d.MyPRs) != 0 {
		t.Errorf("review=%d mine=%d, want 1/0 (deduplicated)", len(d.ReviewPRs), len(d.MyPRs))
	}
	pr := d.ReviewPRs[0]
	if pr.MergeMethod != MergeCommit || !pr.MergeReady() {
		t.Errorf("merge method %q ready=%v; want merge commit and ready (no review policy, CI green)", pr.MergeMethod, pr.MergeReady())
	}
	if len(d.Runs) != 1 || d.Runs[0].WorkflowID != 5 || len(d.Runs[0].FailureLog) == 0 {
		t.Errorf("runs not enriched: %+v", d.Runs)
	}
	if len(d.Notifications) != 1 || d.Notifications[0].URL != "https://github.com/mona/app/pull/4" {
		t.Errorf("notification URL = %+v", d.Notifications)
	}
	if len(d.Warnings) != 0 {
		t.Errorf("disabled Dependabot must not warn, got %v", d.Warnings)
	}
}

func TestFetchDegradesWhenASectionFails(t *testing.T) {
	srv, _ := fakeGitHub(t, "GhxIssues")
	d, err := NewTestClient("tok", srv.URL).Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("one failing section must not fail the dashboard: %v", err)
	}
	if len(d.Repos) != 1 || len(d.Issues) != 0 {
		t.Errorf("repos=%d issues=%d", len(d.Repos), len(d.Issues))
	}
	if len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], "issues: ") {
		t.Errorf("warnings = %v", d.Warnings)
	}
}

func TestFetchFailsWithoutViewer(t *testing.T) {
	srv, calls := fakeGitHub(t, "GhxViewer")
	start := time.Now()
	_, err := NewTestClient("tok", srv.URL).Fetch(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v", err)
	}
	// 6 queries plus the profile's second page of repositories, and the viewer query is retried twice more.
	if n := atomic.LoadInt32(calls); n != 9 {
		t.Errorf("graphql calls = %d, want 9", n)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("retries took too long")
	}
}

func TestUnauthorizedIsExplained(t *testing.T) {
	srv, _ := fakeGitHub(t)
	_, err := NewTestClient("wrong", srv.URL).Fetch(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "gh auth refresh") {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitIsExplained(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1900000000")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	err := NewTestClient("tok", srv.URL).get(context.Background(), "/user", nil)
	var ae *APIError
	if !strings.Contains(err.Error(), "rate limit") || !asAPIError(err, &ae) || ae.Reset.IsZero() {
		t.Fatalf("err = %v", err)
	}
	if IsNotFound(err) {
		t.Error("rate limit must not be treated as not-found")
	}
}

func asAPIError(err error, target **APIError) bool {
	ae, ok := err.(*APIError)
	*target = ae
	return ok
}

func TestMutationsAreNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if err := NewTestClient("tok", srv.URL).Approve(context.Background(), "o/r", 1); err == nil {
		t.Fatal("want error")
	}
	if calls != 1 {
		t.Errorf("approve sent %d times; a mutation must never be retried", calls)
	}
}

func TestMergeUsesRepoMethodAndDeletesOwnBranch(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = append(got, r.Method+" "+r.URL.EscapedPath()+" "+body["merge_method"])
	}))
	defer srv.Close()
	pr := PullRequest{Repo: "o/r", Number: 3, HeadRef: "feat/x", MergeMethod: MergeRebase}
	if err := NewTestClient("tok", srv.URL).Merge(context.Background(), pr, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT /repos/o/r/pulls/3/merge rebase", "DELETE /repos/o/r/git/refs/heads/feat%2Fx "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %q, want %q", got, want)
	}
	got = nil
	pr.CrossRepo = true
	_ = NewTestClient("tok", srv.URL).Merge(context.Background(), pr, true)
	if len(got) != 1 {
		t.Errorf("fork branches must not be deleted: %q", got)
	}
}

func TestNotificationURL(t *testing.T) {
	for _, tc := range []struct{ api, typ, want string }{
		{"https://api.github.com/repos/o/r/pulls/12", "PullRequest", "https://github.com/o/r/pull/12"},
		{"https://api.github.com/repos/o/r/issues/7", "Issue", "https://github.com/o/r/issues/7"},
		{"https://api.github.com/repos/o/r/commits/abc123", "Commit", "https://github.com/o/r/commit/abc123"},
		{"https://api.github.com/repos/o/r/releases/99", "Release", "https://github.com/o/r/releases"},
		{"", "CheckSuite", "https://github.com/o/r/actions"},
		{"", "Unknown", "https://github.com/o/r"},
	} {
		if got := NotificationURL(tc.api, tc.typ, "o/r"); got != tc.want {
			t.Errorf("NotificationURL(%q) = %q, want %q", tc.api, got, tc.want)
		}
	}
}

func TestFailureExcerptSkipsStepEcho(t *testing.T) {
	raw := "2026-10-26T10:00:00.0000000Z ##[group]Run some/action@v1\n" +
		"2026-10-26T10:00:00.0000000Z with:\n" +
		"2026-10-26T10:00:00.0000000Z   TOKEN: ***\n" +
		"2026-10-26T10:00:00.0000000Z env:\n" +
		"2026-10-26T10:00:00.0000000Z   CI: true\n" +
		"2026-10-26T10:00:00.0000000Z ##[endgroup]\n" +
		"2026-10-26T10:00:01.0000000Z \x1b[36mcompiling\x1b[0m\n" +
		"2026-10-26T10:00:02.0000000Z ##[error]Couldn't find the marker\n"
	got := FailureExcerpt(raw, 5)
	want := []string{"▶ Run some/action@v1", "compiling", "Couldn't find the marker"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFailureExcerptStartsAtFailingStep(t *testing.T) {
	raw := "##[group]Run actions/checkout@v4\n" + strings.Repeat("git fetch noise\n", 20) + "##[endgroup]\n" +
		"##[group]Run make test\n##[endgroup]\nok pkg/a\nFAIL pkg/b\n##[error]Process completed with exit code 2.\n"
	got := FailureExcerpt(raw, 14)
	if len(got) != 4 || got[0] != "▶ Run make test" || got[3] != "Process completed with exit code 2." {
		t.Errorf("excerpt = %q", got)
	}
}

func TestMergeReady(t *testing.T) {
	base := PullRequest{Mergeable: true, CIState: StateSuccess}
	for name, tc := range map[string]struct {
		mut  func(*PullRequest)
		want bool
	}{
		"no review policy":  {func(p *PullRequest) {}, true},
		"approved":          {func(p *PullRequest) { p.ReviewDecision = ReviewApproved }, true},
		"review required":   {func(p *PullRequest) { p.ReviewDecision = ReviewRequired }, false},
		"changes requested": {func(p *PullRequest) { p.ReviewDecision = ReviewChangesRequested }, false},
		"draft":             {func(p *PullRequest) { p.IsDraft = true }, false},
		"conflicting":       {func(p *PullRequest) { p.Mergeable = false }, false},
		"ci failing":        {func(p *PullRequest) { p.CIState = StateFailure }, false},
		"no ci":             {func(p *PullRequest) { p.CIState = StateNone }, true},
	} {
		pr := base
		tc.mut(&pr)
		if got := pr.MergeReady(); got != tc.want {
			t.Errorf("%s: MergeReady = %v, want %v", name, got, tc.want)
		}
	}
}

func TestLocalReposParsesRemotes(t *testing.T) {
	for line, want := range map[string]string{
		"origin\tgit@github.com:Tukesh1/ghx.git (fetch)":          "Tukesh1/ghx",
		"origin\thttps://github.com/Tukesh1/ghx (fetch)":          "Tukesh1/ghx",
		"upstream\thttps://github.com/charm/bubbletea.git (push)": "charm/bubbletea",
	} {
		m := remoteRepo.FindStringSubmatch(line)
		if m == nil || m[1] != want {
			t.Errorf("%q → %v, want %s", line, m, want)
		}
	}
}

func TestFetchProfile(t *testing.T) {
	srv, _ := fakeGitHub(t)
	d, err := NewTestClient("tok", srv.URL).Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	p := d.Profile
	if p == nil {
		t.Fatalf("no profile; warnings %v", d.Warnings)
	}
	if p.Login != "mona" || p.Followers != 3 || p.Repos != 4 || p.StatusEmoji != ":rocket:" || p.CreatedAt.Year() != 2020 {
		t.Errorf("profile fields: %+v", p)
	}
	// Nothing pinned: like github.com, fall back to the most-starred public repositories.
	if !p.Popular || len(p.Pinned) != 2 || p.Pinned[0].NameWithOwner != "mona/app" || p.Pinned[0].Language != "Go" {
		t.Errorf("popular repositories = %+v", p.Pinned)
	}
	// Stars and languages count every page of repositories, not just the first 100.
	if p.StarsEarned != 9 || p.ForksEarned != 2 || len(p.Languages) != 2 || p.Languages[0].Percent != 50 {
		t.Errorf("stars %d forks %d languages %+v", p.StarsEarned, p.ForksEarned, p.Languages)
	}
	// Private repositories and private org memberships never appear: github.com hides them from visitors.
	if len(p.CommitRepos) != 1 || p.CommitRepos[0].Commits != 12 {
		t.Errorf("commit repos = %+v", p.CommitRepos)
	}
	if len(p.Orgs) != 1 || p.Orgs[0] != "public-org" {
		t.Errorf("orgs = %v, want public memberships only", p.Orgs)
	}
	if p.Readme != "# Hi\n\nHello world" {
		t.Errorf("readme = %q", p.Readme)
	}
	if p.Avatar == nil || p.Avatar.W != 32 {
		t.Fatalf("avatar = %+v (it must be fetched without the token)", p.Avatar)
	}
	if c, _ := p.Avatar.At(31, 31); c != "#ff0000" {
		t.Errorf("avatar pixel = %s", c)
	}
}

func TestCleanReadme(t *testing.T) {
	in := `<!-- <p align="center"><img src="banner.gif" /></p> -->
<!--START_SECTION:waka-->
<!--END_SECTION:waka-->
<h1 align="center">Hi, I'm Mona</h1>
<p align="center">
I'm building [orbit](https://github.com/acme/orbit) &amp; more.
</p>

<h3 align="center">Languages and Tools</h3>
<p align="center"><img src="https://skillicons.dev/icons?i=go,rust" height="50"/></p>

## Now
* **Learning** Zig
- ` + "`ghx`" + ` [![badge](https://img.shields.io/x)](https://x)
[ref]: https://example.com
`
	want := "# Hi, I'm Mona\n\nI'm building orbit & more.\n\n# Now\n• Learning Zig\n• ghx"
	if got := CleanReadme(in); got != want {
		t.Errorf("CleanReadme:\n%s\nwant:\n%s", got, want)
	}
}
