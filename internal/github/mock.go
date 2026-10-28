package github

import (
	"math"
	"math/rand"
	"time"
)

// MockDashboard returns a deterministic, realistic dataset for `ghx --demo` and offline use.
func MockDashboard() *Dashboard {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	h := time.Hour
	day := 24 * time.Hour

	d := &Dashboard{
		User:      User{Login: "octocat", Name: "Mona Lisa Octocat"},
		Breakdown: ContributionBreakdown{Commits: 2612, Reviews: 576, PRs: 423, Issues: 231},
		FetchedAt: now,
		Demo:      true,
	}

	d.ReviewPRs = []PullRequest{
		{
			Number: 1842, Title: "feat(scheduler): lock-free work-stealing run queue", Repo: "acme/orbit-engine",
			Author: "kdubois", HeadRef: "feat/work-stealing", BaseRef: "main", Mergeable: true, MergeMethod: MergeSquash,
			ReviewDecision: ReviewRequired, CIState: StateSuccess, Additions: 612, Deletions: 148, ChangedFiles: 9,
			UpdatedAt: ago(25 * time.Minute), NeedsMyReview: true,
			Files: []ChangedFile{
				{"src/sched/runqueue.rs", 284, 61}, {"src/sched/worker.rs", 142, 39}, {"src/sched/mod.rs", 48, 12},
				{"benches/sched_bench.rs", 96, 0}, {"src/sync/atomic_deque.rs", 31, 22}, {"Cargo.toml", 2, 1},
				{"docs/scheduler.md", 9, 13},
			},
			Checks: []CheckRun{
				{"build (ubuntu-latest)", StateSuccess}, {"build (macos-14)", StateSuccess}, {"clippy", StateSuccess},
				{"miri", StateSuccess}, {"bench-regression", StateSuccess},
			},
			Reviewers: []Reviewer{{"octocat", "PENDING"}, {"hsuzuki", "COMMENTED"}},
		},
		{
			Number: 977, Title: "fix(auth): refresh OIDC token before expiry window", Repo: "acme/web-console",
			Author: "priya-r", HeadRef: "fix/oidc-refresh", BaseRef: "main", Mergeable: true, MergeMethod: MergeSquash,
			ReviewDecision: ReviewRequired, CIState: StatePending, Additions: 88, Deletions: 31, ChangedFiles: 4,
			UpdatedAt: ago(2 * h), NeedsMyReview: true,
			Files: []ChangedFile{
				{"src/auth/session.ts", 54, 19}, {"src/auth/oidc.ts", 21, 9}, {"src/auth/__tests__/session.test.ts", 12, 0}, {"CHANGELOG.md", 1, 3},
			},
			Checks: []CheckRun{
				{"lint", StateSuccess}, {"typecheck", StateSuccess}, {"unit (node 22)", StatePending}, {"e2e (playwright)", StatePending},
			},
			Reviewers: []Reviewer{{"octocat", "PENDING"}, {"web-platform", "PENDING"}},
		},
		{
			Number: 311, Title: "chore(deps): bump golang.org/x/net to v0.33.0", Repo: "octocat/ghx",
			Author: "dependabot[bot]", HeadRef: "dependabot/go_modules/x-net", BaseRef: "main", Mergeable: true, MergeMethod: MergeSquash,
			ReviewDecision: ReviewRequired, CIState: StateFailure, Additions: 6, Deletions: 6, ChangedFiles: 2,
			UpdatedAt: ago(5 * h), NeedsMyReview: true,
			Files:     []ChangedFile{{"go.mod", 2, 2}, {"go.sum", 4, 4}},
			Checks:    []CheckRun{{"build", StateSuccess}, {"test (race)", StateFailure}, {"golangci-lint", StateSuccess}},
			Reviewers: []Reviewer{{"octocat", "PENDING"}},
		},
	}

	d.MyPRs = []PullRequest{
		{
			Number: 1838, Title: "perf(ingest): zero-copy protobuf decode path", Repo: "acme/orbit-engine",
			Author: "octocat", HeadRef: "perf/zero-copy", BaseRef: "main", Mergeable: true, MergeMethod: MergeSquash,
			ReviewDecision: ReviewApproved, CIState: StateSuccess, Additions: 341, Deletions: 207, ChangedFiles: 11,
			UpdatedAt: ago(48 * time.Minute),
			Files: []ChangedFile{
				{"src/ingest/decode.rs", 188, 141}, {"src/ingest/buffer.rs", 72, 30}, {"src/proto/mod.rs", 41, 22},
				{"benches/ingest.rs", 38, 0}, {"Cargo.lock", 2, 14},
			},
			Checks:    []CheckRun{{"build (ubuntu-latest)", StateSuccess}, {"build (macos-14)", StateSuccess}, {"clippy", StateSuccess}, {"miri", StateSuccess}},
			Reviewers: []Reviewer{{"kdubois", "APPROVED"}, {"hsuzuki", "APPROVED"}},
		},
		{
			Number: 305, Title: "feat(ui): half-block contribution heatmap", Repo: "octocat/ghx",
			Author: "octocat", HeadRef: "feat/heatmap", BaseRef: "main", Mergeable: true, MergeMethod: MergeSquash,
			ReviewDecision: ReviewApproved, CIState: StateSuccess, Additions: 164, Deletions: 22, ChangedFiles: 3,
			UpdatedAt: ago(3 * h),
			Files:     []ChangedFile{{"internal/ui/charts.go", 131, 8}, {"internal/ui/tab_overview.go", 29, 12}, {"README.md", 4, 2}},
			Checks:    []CheckRun{{"build", StateSuccess}, {"test (race)", StateSuccess}, {"golangci-lint", StateSuccess}},
			Reviewers: []Reviewer{{"priya-r", "APPROVED"}},
		},
		{
			Number: 982, Title: "refactor(api): migrate billing client to fetch + zod", Repo: "acme/web-console",
			Author: "octocat", HeadRef: "refactor/billing-fetch", BaseRef: "main", Mergeable: false, MergeMethod: MergeSquash,
			ReviewDecision: ReviewChangesRequested, CIState: StateSuccess, Additions: 503, Deletions: 690, ChangedFiles: 17,
			UpdatedAt: ago(26 * h),
			Files: []ChangedFile{
				{"src/api/billing.ts", 211, 380}, {"src/api/schema.ts", 144, 0}, {"src/api/http.ts", 61, 122},
				{"src/pages/billing/index.tsx", 49, 88}, {"package.json", 3, 5},
			},
			Checks:    []CheckRun{{"lint", StateSuccess}, {"typecheck", StateSuccess}, {"unit (node 22)", StateSuccess}, {"e2e (playwright)", StateSuccess}},
			Reviewers: []Reviewer{{"priya-r", "CHANGES_REQUESTED"}, {"web-platform", "PENDING"}},
		},
		{
			Number: 64, Title: "docs: architecture decision record for event bus", Repo: "acme/platform-rfcs",
			Author: "octocat", HeadRef: "adr/event-bus", BaseRef: "main", IsDraft: true, Mergeable: true, MergeMethod: MergeCommit,
			ReviewDecision: ReviewRequired, CIState: StateNone, Additions: 212, Deletions: 0, ChangedFiles: 1,
			UpdatedAt: ago(3 * day),
			Files:     []ChangedFile{{"adr/0042-event-bus.md", 212, 0}},
		},
	}

	failLog := []string{
		"=== RUN   TestCompactHeatmap/narrow_terminal",
		"--- FAIL: TestCompactHeatmap/narrow_terminal (0.00s)",
		"panic: runtime error: index out of range [52] with length 52",
		"goroutine 18 [running]:",
		"github.com/octocat/ghx/internal/ui.RenderCompactHeatmap(...)",
		"\tinternal/ui/charts.go:41 +0x1b4",
		"FAIL\tgithub.com/octocat/ghx/internal/ui\t0.412s",
		"##[error]Process completed with exit code 1.",
	}
	d.Runs = []WorkflowRun{
		{
			ID: 9918273641, WorkflowID: 101, WorkflowPath: ".github/workflows/ci.yml", Name: "CI", Repo: "octocat/ghx", Branch: "dependabot/go_modules/x-net", Event: "pull_request",
			SHA: "4be1f0c", Status: "completed", Conclusion: "failure", StartedAt: ago(5 * h), Duration: 3*time.Minute + 12*time.Second,
			FailureLog: failLog,
			Jobs: []Job{
				{ID: 1, Name: "build", Status: "completed", Conclusion: "success", Steps: []Step{
					{1, "Set up job", "completed", "success", 2 * time.Second, nil},
					{2, "actions/checkout@v4", "completed", "success", 3 * time.Second, nil},
					{3, "actions/setup-go@v5", "completed", "success", 11 * time.Second, nil},
					{4, "go build ./...", "completed", "success", 38 * time.Second, nil},
				}},
				{ID: 2, Name: "test (race)", Status: "completed", Conclusion: "failure", Steps: []Step{
					{1, "Set up job", "completed", "success", 2 * time.Second, nil},
					{2, "actions/checkout@v4", "completed", "success", 3 * time.Second, nil},
					{3, "actions/setup-go@v5", "completed", "success", 12 * time.Second, nil},
					{4, "go test -race ./...", "completed", "failure", 1*time.Minute + 47*time.Second, failLog},
					{5, "Upload coverage", "completed", "skipped", 0, nil},
				}},
			},
		},
		{
			ID: 9918270012, WorkflowID: 202, WorkflowPath: ".github/workflows/e2e.yml", Name: "e2e", Repo: "acme/web-console", Branch: "fix/oidc-refresh", Event: "pull_request",
			SHA: "a91c3d2", Status: "in_progress", StartedAt: ago(6 * time.Minute), Duration: 6*time.Minute + 4*time.Second,
			Jobs: []Job{
				{ID: 3, Name: "playwright (chromium)", Status: "in_progress", Steps: []Step{
					{1, "Set up job", "completed", "success", 2 * time.Second, nil},
					{2, "actions/checkout@v4", "completed", "success", 2 * time.Second, nil},
					{3, "pnpm install --frozen-lockfile", "completed", "success", 41 * time.Second, nil},
					{4, "pnpm playwright test --shard 1/3", "in_progress", "", 0, []string{
						"Running 148 tests using 4 workers",
						"  ✓  auth/login.spec.ts:12:3 › sign in with SSO (2.1s)",
						"  ✓  auth/session.spec.ts:44:3 › refreshes token before expiry (1.4s)",
						"  ·  billing/invoices.spec.ts:8:3 › lists invoices",
					}},
				}},
			},
		},
		{
			ID: 9918265520, WorkflowID: 303, WorkflowPath: ".github/workflows/release.yml", Name: "Release", Repo: "acme/orbit-engine", Branch: "main", Event: "push",
			SHA: "e03b7a9", Status: "completed", Conclusion: "success", StartedAt: ago(70 * time.Minute), Duration: 8*time.Minute + 51*time.Second,
			Jobs: []Job{
				{ID: 4, Name: "publish crates", Status: "completed", Conclusion: "success", Steps: []Step{
					{1, "Set up job", "completed", "success", 2 * time.Second, nil},
					{2, "cargo build --release", "completed", "success", 5*time.Minute + 2*time.Second, nil},
					{3, "cargo publish", "completed", "success", 48 * time.Second, nil},
					{4, "Create GitHub release v2.14.0", "completed", "success", 4 * time.Second, nil},
				}},
			},
		},
		{
			ID: 9918260007, WorkflowID: 304, WorkflowPath: ".github/workflows/ci.yml", Name: "CI", Repo: "acme/orbit-engine", Branch: "feat/work-stealing", Event: "pull_request",
			SHA: "77d10fe", Status: "completed", Conclusion: "success", StartedAt: ago(30 * time.Minute), Duration: 11*time.Minute + 3*time.Second,
			Jobs: []Job{
				{ID: 5, Name: "test matrix", Status: "completed", Conclusion: "success", Steps: []Step{
					{1, "Set up job", "completed", "success", 2 * time.Second, nil},
					{2, "cargo test --all-features", "completed", "success", 7*time.Minute + 30*time.Second, nil},
					{3, "cargo miri test", "completed", "success", 3*time.Minute + 1*time.Second, nil},
				}},
			},
		},
		{
			ID: 9918251199, WorkflowID: 205, WorkflowPath: ".github/workflows/preview.yml", Name: "Deploy preview", Repo: "acme/web-console", Branch: "refactor/billing-fetch", Event: "pull_request",
			SHA: "c4f8e21", Status: "completed", Conclusion: "success", StartedAt: ago(26 * h), Duration: 2*time.Minute + 37*time.Second,
			Jobs: []Job{
				{ID: 6, Name: "vercel deploy", Status: "completed", Conclusion: "success", Steps: []Step{
					{1, "Set up job", "completed", "success", 1 * time.Second, nil},
					{2, "vercel build", "completed", "success", 1*time.Minute + 58*time.Second, nil},
					{3, "vercel deploy --prebuilt", "completed", "success", 31 * time.Second, nil},
				}},
			},
		},
	}

	cadence := func(seed int64, base float64) []int {
		r := rand.New(rand.NewSource(seed))
		out := make([]int, 30)
		for i := range out {
			v := base*(0.6+0.8*r.Float64()) + 1.5*math.Sin(float64(i)/3)
			if i%7 == 5 || i%7 == 6 {
				v *= 0.3
			}
			if v < 0 {
				v = 0
			}
			out[i] = int(v)
		}
		return out
	}
	d.Repos = []Repo{
		{
			NameWithOwner: "acme/orbit-engine", Description: "High-throughput event ingestion engine written in Rust",
			Stars: 4812, Forks: 311, Language: "Rust", LanguageColor: "#dea584", DefaultBranch: "main", SyncBranch: "perf/zero-copy",
			Ahead: 7, Behind: 2, LatestSHA: "e03b7a9", LatestMessage: "release: v2.14.0", LatestAuthor: "release-bot",
			OpenPRs: 23, OpenIssues: 61, CIState: StateSuccess, PushedAt: ago(70 * time.Minute), Release: "v2.14.0", Cadence: cadence(1, 6),
			RecentCommits: []Commit{
				{"e03b7a9", "release: v2.14.0", "release-bot", ago(70 * time.Minute)},
				{"9a0c4de", "fix(ingest): handle partial frames on reconnect", "octocat", ago(4 * h)},
				{"51be7f2", "perf(sched): avoid false sharing on worker counters", "kdubois", ago(9 * h)},
				{"0d3e1a8", "test: property tests for frame decoder", "hsuzuki", ago(20 * h)},
			},
		},
		{
			NameWithOwner: "octocat/ghx", Description: "Action-first GitHub dashboard for your terminal",
			Stars: 1290, Forks: 64, Language: "Go", LanguageColor: "#00add8", DefaultBranch: "main", SyncBranch: "feat/heatmap",
			Ahead: 3, Behind: 0, LatestSHA: "b7c2e19", LatestMessage: "feat(ui): theme cycling with t", LatestAuthor: "octocat",
			OpenPRs: 4, OpenIssues: 12, CIState: StateFailure, PushedAt: ago(3 * h), Release: "v0.9.2", Cadence: cadence(2, 4),
			RecentCommits: []Commit{
				{"b7c2e19", "feat(ui): theme cycling with t", "octocat", ago(3 * h)},
				{"4be1f0c", "chore(deps): bump golang.org/x/net", "dependabot[bot]", ago(5 * h)},
				{"c19d0aa", "fix(api): paginate workflow runs", "octocat", ago(27 * h)},
			},
		},
		{
			NameWithOwner: "acme/web-console", IsPrivate: true, Description: "Customer-facing admin console (Next.js)",
			Stars: 0, Forks: 0, Language: "TypeScript", LanguageColor: "#3178c6", DefaultBranch: "main", SyncBranch: "refactor/billing-fetch",
			Ahead: 12, Behind: 9, LatestSHA: "f2a61bc", LatestMessage: "feat(billing): usage-based invoice preview", LatestAuthor: "priya-r",
			OpenPRs: 17, OpenIssues: 38, CIState: StatePending, PushedAt: ago(6 * time.Minute), Cadence: cadence(3, 8),
			RecentCommits: []Commit{
				{"f2a61bc", "feat(billing): usage-based invoice preview", "priya-r", ago(40 * time.Minute)},
				{"77ae0c1", "fix(a11y): focus trap in dialog", "jmartin", ago(5 * h)},
			},
		},
		{
			NameWithOwner: "acme/ml-pipelines", IsPrivate: true, Description: "Feature store and training DAGs",
			Language: "Python", LanguageColor: "#3572A5", DefaultBranch: "main", SyncBranch: "main",
			LatestSHA: "3d9e0b4", LatestMessage: "feat(dag): nightly embeddings backfill", LatestAuthor: "lchen",
			OpenPRs: 6, OpenIssues: 14, CIState: StateSuccess, PushedAt: ago(19 * h), Cadence: cadence(4, 3),
			RecentCommits: []Commit{{"3d9e0b4", "feat(dag): nightly embeddings backfill", "lchen", ago(19 * h)}},
		},
		{
			NameWithOwner: "acme/platform-rfcs", Description: "Design docs and ADRs for the platform team",
			Stars: 87, Forks: 9, Language: "Markdown", LanguageColor: "#083fa1", DefaultBranch: "main", SyncBranch: "adr/event-bus",
			Ahead: 1, Behind: 4, LatestSHA: "8e4f77a", LatestMessage: "rfc-0041: multi-region failover", LatestAuthor: "hsuzuki",
			OpenPRs: 3, OpenIssues: 2, CIState: StateNone, PushedAt: ago(2 * day), Cadence: cadence(5, 1),
		},
		{
			NameWithOwner: "octocat/dotfiles", Description: "zsh, neovim and tmux config",
			Stars: 233, Forks: 18, Language: "Lua", LanguageColor: "#000080", DefaultBranch: "main", SyncBranch: "main",
			LatestSHA: "aa01c3e", LatestMessage: "nvim: switch to blink.cmp", LatestAuthor: "octocat",
			CIState: StateSuccess, PushedAt: ago(4 * day), Cadence: cadence(6, 1),
		},
		{
			NameWithOwner: "acme/terraform-infra", IsPrivate: true, Description: "AWS + Cloudflare infrastructure as code",
			Language: "HCL", LanguageColor: "#844FBA", DefaultBranch: "main", SyncBranch: "main", Behind: 0,
			LatestSHA: "6b2d8f0", LatestMessage: "feat(eks): bump node group to 1.31", LatestAuthor: "ops-bot",
			OpenPRs: 2, OpenIssues: 5, CIState: StateSuccess, PushedAt: ago(6 * day), Cadence: cadence(7, 2),
		},
	}

	d.Issues = []Issue{
		{
			Number: 1851, Title: "Ingest stalls under backpressure after reconnect storm", Repo: "acme/orbit-engine",
			Labels: []Label{{"P0", "b60205"}, {"bug", "d73a4a"}}, Priority: "P0", LinkedPR: 1838,
			CreatedAt: ago(2 * day), UpdatedAt: ago(1 * h), Comments: 14, Milestone: "v2.15", MilestonePct: 62,
			Body: "When 200+ producers reconnect within 1s the bounded channel fills and the decoder never drains. p99 latency spikes to 40s and the node is evicted from the ring.",
		},
		{
			Number: 990, Title: "SSO users logged out every 60 minutes", Repo: "acme/web-console",
			Labels: []Label{{"P0", "b60205"}, {"auth", "5319e7"}}, Priority: "P0", LinkedPR: 977,
			CreatedAt: ago(3 * day), UpdatedAt: ago(2 * h), Comments: 9, Milestone: "Sprint 44", MilestonePct: 71,
			Body: "OIDC refresh is only attempted after the access token expires, so the first request after expiry 401s and the session is dropped.",
		},
		{
			Number: 318, Title: "Heatmap panics on terminals narrower than 60 cols", Repo: "octocat/ghx",
			Labels: []Label{{"P1", "d93f0b"}, {"bug", "d73a4a"}}, Priority: "P1", LinkedPR: 305,
			CreatedAt: ago(1 * day), UpdatedAt: ago(5 * h), Comments: 3, Milestone: "v1.0", MilestonePct: 80,
			Body: "Resizing to 58 columns triggers index out of range in RenderCompactHeatmap. Guard week slicing to available width.",
		},
		{
			Number: 1799, Title: "Expose scheduler queue depth as Prometheus gauge", Repo: "acme/orbit-engine",
			Labels: []Label{{"P2", "fbca04"}, {"observability", "0e8a16"}}, Priority: "P2",
			CreatedAt: ago(9 * day), UpdatedAt: ago(2 * day), Comments: 4, Milestone: "v2.15", MilestonePct: 62,
			Body: "Operators need per-worker queue depth to tune steal thresholds.",
		},
		{
			Number: 142, Title: "Backfill job retries forever on schema mismatch", Repo: "acme/ml-pipelines",
			Labels: []Label{{"P1", "d93f0b"}}, Priority: "P1",
			CreatedAt: ago(12 * day), UpdatedAt: ago(3 * day), Comments: 6,
			Body: "Retries should be capped and the DAG should alert when the upstream schema version changes.",
		},
		{
			Number: 52, Title: "Document cross-region RPO/RTO targets", Repo: "acme/platform-rfcs",
			Labels: []Label{{"P3", "c5def5"}, {"docs", "0075ca"}}, Priority: "P3",
			CreatedAt: ago(21 * day), UpdatedAt: ago(8 * day), Comments: 1,
			Body: "Add a table of recovery objectives per tier to rfc-0041.",
		},
	}

	note := func(id, reason, title, typ, repo, url string, at time.Time) Notification {
		return Notification{ID: id, Reason: reason, Title: title, Type: typ, Repo: repo, URL: WebURL + "/" + repo + url, UpdatedAt: at}
	}
	d.Notifications = []Notification{
		note("n1", "review_requested", "feat(scheduler): lock-free work-stealing run queue", "PullRequest", "acme/orbit-engine", "/pull/1842", ago(25*time.Minute)),
		note("n3", "mention", "@octocat can you confirm the backpressure repro?", "Issue", "acme/orbit-engine", "/issues/1851", ago(1*h)),
		note("n2", "ci_activity", "CI workflow run failed for dependabot/go_modules/x-net", "CheckSuite", "octocat/ghx", "/actions", ago(5*h)),
		note("n5", "security_alert", "Critical: golang.org/x/net HTTP/2 rapid reset", "RepositoryVulnerabilityAlert", "octocat/ghx", "/security/dependabot", ago(6*h)),
		note("n6", "comment", "fix(auth): refresh OIDC token before expiry window", "PullRequest", "acme/web-console", "/pull/977", ago(8*h)),
		note("n7", "subscribed", "v2.14.0", "Release", "acme/orbit-engine", "/releases", ago(20*h)),
		note("n4", "assign", "SSO users logged out every 60 minutes", "Issue", "acme/web-console", "/issues/990", ago(3*day)),
	}

	d.Languages = []LangStat{
		{Name: "Rust", Color: "#dea584", Percent: 38}, {Name: "TypeScript", Color: "#3178c6", Percent: 29}, {Name: "Go", Color: "#00add8", Percent: 19}, {Name: "Python", Color: "#3572A5", Percent: 14},
	}

	d.Alerts = []DependabotAlert{
		{Repo: "octocat/ghx", Package: "golang.org/x/net", Ecosystem: "go", CVE: "CVE-2023-44487", Severity: "critical", CurrentVersion: "0.17.0", PatchedVersion: "0.33.0", URL: WebURL + "/octocat/ghx/security/dependabot/3"},
		{Repo: "acme/web-console", Package: "next", Ecosystem: "npm", CVE: "CVE-2025-29927", Severity: "high", CurrentVersion: "14.2.3", PatchedVersion: "14.2.25", URL: WebURL + "/acme/web-console/security/dependabot/12"},
	}

	r := rand.New(rand.NewSource(42))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	start := today.AddDate(0, 0, -364)
	for start.Weekday() != time.Sunday {
		start = start.AddDate(0, 0, -1)
	}
	for t := start; !t.After(today); t = t.AddDate(0, 0, 1) {
		days := int(math.Round(today.Sub(t).Hours() / 24)) // round: DST days are 23h or 25h long
		weekday := int(t.Weekday())
		var count int
		switch {
		case days < 47:
			count = 1 + r.Intn(14)
		case days == 47:
			// A rest day right before the streak keeps it exactly 47 days on any date or timezone.
			count = 0
		case weekday == 0 || weekday == 6:
			if r.Float64() < 0.35 {
				count = r.Intn(5)
			}
		default:
			season := 0.5 + 0.5*math.Sin(float64(days)/28)
			if r.Float64() < 0.82 {
				count = int(season*12*r.Float64()) + r.Intn(3)
			}
		}
		d.Contributions = append(d.Contributions, ContributionDay{Date: t, Count: count, Weekday: weekday})
	}

	pattern := [][2]int{{4, 0}, {6, 1}, {3, 0}, {5, 0}, {2, 0}, {1, 0}, {7, 2}, {5, 0}, {6, 0}, {4, 1}, {2, 0}, {6, 0}, {5, 0}, {4, 1}}
	p50s := []int{312, 298, 341, 276, 305, 290, 388, 322, 301, 295, 280, 330, 315, 296}
	for i := 0; i < 14; i++ {
		d.CIHistory = append(d.CIHistory, CIDay{
			Date:   today.AddDate(0, 0, i-13),
			Passed: pattern[i][0],
			Failed: pattern[i][1],
			P50:    time.Duration(p50s[i]) * time.Second,
		})
	}
	d.Profile = mockProfile()
	return d
}

func mockProfile() *Profile {
	return &Profile{
		Login: "octocat", Name: "Mona Lisa Octocat", Pronouns: "she/her",
		Bio:     "Building developer tools that stay out of your way. Schedulers, CI pipelines and terminal UIs.",
		Company: "@acme", Location: "San Francisco, CA", Website: "https://octocat.dev", Twitter: "octocat",
		StatusEmoji: ":rocket:", StatusMessage: "Shipping orbit-engine v2",
		Avatar:    Identicon("octocat", 32),
		CreatedAt: time.Date(2011, 1, 25, 18, 44, 36, 0, time.UTC),
		Followers: 12483, Following: 9, Starred: 1204, Repos: 64, StarsEarned: 48211, ForksEarned: 5122,
		Orgs:    []string{"acme", "orbit-labs", "open-tooling"},
		Socials: []Social{{Provider: "MASTODON", Name: "@octocat@hachyderm.io", URL: "https://hachyderm.io/@octocat"}},
		Pinned: []PinnedRepo{
			{NameWithOwner: "acme/orbit-engine", Description: "High-throughput event ingestion engine written in Rust", Language: "Rust", LanguageColor: "#dea584", Stars: 18234, Forks: 1311},
			{NameWithOwner: "octocat/ghx", Description: "Action-first GitHub dashboard for your terminal", Language: "Go", LanguageColor: "#00ADD8", Stars: 9876, Forks: 402},
			{NameWithOwner: "octocat/terminal-charts", Description: "Sparklines, heatmaps and bar charts for Bubble Tea apps", Language: "Go", LanguageColor: "#00ADD8", Stars: 4120, Forks: 233},
			{NameWithOwner: "acme/platform-rfcs", Description: "Design docs and ADRs for the platform team", Language: "Markdown", LanguageColor: "#083fa1", Stars: 812, Forks: 97},
			{NameWithOwner: "octocat/linguist-lite", Description: "Fast language detection for monorepos", Language: "Ruby", LanguageColor: "#701516", Stars: 655, Forks: 41, Parent: "acme/linguist"},
			{NameWithOwner: "octocat/dotfiles", Description: "zsh, neovim and tmux config", Language: "Shell", LanguageColor: "#89e051", Stars: 1530, Forks: 388},
		},
		Languages: []LangStat{
			{Name: "Go", Color: "#00ADD8", Percent: 34}, {Name: "Rust", Color: "#dea584", Percent: 22},
			{Name: "TypeScript", Color: "#3178c6", Percent: 18}, {Name: "Shell", Color: "#89e051", Percent: 9},
			{Name: "Other", Color: "#8b949e", Percent: 17},
		},
		CommitRepos: []RepoContribution{
			{Repo: "acme/orbit-engine", Commits: 1203}, {Repo: "octocat/ghx", Commits: 642}, {Repo: "acme/web-console", Commits: 388},
			{Repo: "octocat/dotfiles", Commits: 211}, {Repo: "acme/terraform-infra", Commits: 97},
		},
		ReposCreated: 14, PrivateContrib: 318,
		Readme: CleanReadme(`### Hi, I'm Mona 👋

I build developer tools: **orbit-engine**, a work-stealing event runtime, and [ghx](https://github.com/octocat/ghx), the GitHub dashboard you are looking at.

#### Currently
- 🔭 Working on orbit-engine v2 — lock-free run queues
- 🌱 Learning Zig and formal verification with TLA+
- 💬 Ask me about schedulers, CI pipelines and terminal UIs
- ⚡ Fun fact: my first commit was a typo fix

<p align="center"><img src="https://github-readme-stats.vercel.app/api?username=octocat" /></p>`),
	}
}
