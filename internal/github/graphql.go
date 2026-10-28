package github

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// The dashboard is loaded with several small GraphQL queries run concurrently rather than one big
// batch: GitHub aborts queries that take longer than ~10s server-side (HTTP 502), and a combined
// query over active repositories with commit history routinely gets close to that limit. Small
// queries are faster in parallel and let one failing section degrade instead of the whole screen.

const viewerQuery = `
query GhxViewer {
  viewer {
    login
    name
    contributionsCollection {
      totalCommitContributions
      totalPullRequestContributions
      totalPullRequestReviewContributions
      totalIssueContributions
      contributionCalendar {
        weeks { contributionDays { contributionCount date weekday } }
      }
    }
  }
}`

const repoFields = `
fragment RepoFields on Repository {
  nameWithOwner
  description
  isPrivate
  isArchived
  stargazerCount
  forkCount
  pushedAt
  primaryLanguage { name color }
  languages(first: 5, orderBy: {field: SIZE, direction: DESC}) {
    totalSize
    edges { size node { name color } }
  }
  issues(states: OPEN) { totalCount }
  pullRequests(states: OPEN) { totalCount }
  releases(first: 1, orderBy: {field: CREATED_AT, direction: DESC}) { nodes { tagName } }
  defaultBranchRef {
    name
    target {
      ... on Commit {
        oid
        messageHeadline
        committedDate
        author { name user { login } }
        statusCheckRollup { state }
        history(first: 30, since: $since) {
          nodes { oid messageHeadline committedDate author { name user { login } } }
        }
      }
    }
  }
}`

const viewerReposQuery = `
query GhxRepos($since: GitTimestamp!) {
  viewer {
    repositories(first: 15, orderBy: {field: PUSHED_AT, direction: DESC}, affiliations: [OWNER, ORGANIZATION_MEMBER, COLLABORATOR]) {
      nodes { ...RepoFields }
    }
  }
}` + repoFields

// ownerReposQuery scopes repositories to one org (or user) for --org, instead of filtering the viewer's
// 15 most recent repositories, which may contain none of the org's.
const ownerReposQuery = `
query GhxOwnerRepos($owner: String!, $since: GitTimestamp!) {
  repositoryOwner(login: $owner) {
    repositories(first: 15, orderBy: {field: PUSHED_AT, direction: DESC}) {
      nodes { ...RepoFields }
    }
  }
}` + repoFields

const prSearchQuery = `
query GhxPRs($q: String!) {
  search(query: $q, type: ISSUE, first: 20) {
    nodes { ...PRFields }
  }
}

fragment PRFields on PullRequest {
  number
  title
  isDraft
  isCrossRepository
  mergeable
  reviewDecision
  additions
  deletions
  changedFiles
  updatedAt
  headRefName
  baseRefName
  author { login }
  repository { nameWithOwner squashMergeAllowed mergeCommitAllowed rebaseMergeAllowed }
  files(first: 8) { nodes { path additions deletions } }
  latestReviews(first: 6) { nodes { author { login } state } }
  reviewRequests(first: 6) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
  commits(last: 1) {
    nodes {
      commit {
        statusCheckRollup {
          state
          contexts(first: 12) {
            totalCount
            checkRunCountsByState { state count }
            statusContextCountsByState { state count }
            nodes {
              ... on CheckRun { name status conclusion }
              ... on StatusContext { context state }
            }
          }
        }
      }
    }
  }
}`

const issueSearchQuery = `
query GhxIssues($q: String!) {
  search(query: $q, type: ISSUE, first: 20) {
    nodes {
      ... on Issue {
        number
        title
        bodyText
        createdAt
        updatedAt
        comments { totalCount }
        repository { nameWithOwner }
        author { login }
        labels(first: 6) { nodes { name color } }
        milestone { title progressPercentage }
        closedByPullRequestsReferences(first: 1) { nodes { number } }
      }
    }
  }
}`

type gqlAuthor struct {
	Name string `json:"name"`
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (a gqlAuthor) display() string {
	if a.User != nil && a.User.Login != "" {
		return a.User.Login
	}
	return a.Name
}

type gqlLogin struct {
	Login string `json:"login"`
}

type gqlPR struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	IsDraft           bool      `json:"isDraft"`
	IsCrossRepository bool      `json:"isCrossRepository"`
	Mergeable         string    `json:"mergeable"`
	ReviewDecision    string    `json:"reviewDecision"`
	Additions         int       `json:"additions"`
	Deletions         int       `json:"deletions"`
	ChangedFiles      int       `json:"changedFiles"`
	UpdatedAt         time.Time `json:"updatedAt"`
	HeadRefName       string    `json:"headRefName"`
	BaseRefName       string    `json:"baseRefName"`
	Author            *gqlLogin `json:"author"`
	Repository        struct {
		NameWithOwner      string `json:"nameWithOwner"`
		SquashMergeAllowed bool   `json:"squashMergeAllowed"`
		MergeCommitAllowed bool   `json:"mergeCommitAllowed"`
		RebaseMergeAllowed bool   `json:"rebaseMergeAllowed"`
	} `json:"repository"`
	Files *struct {
		Nodes []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"nodes"`
	} `json:"files"`
	LatestReviews struct {
		Nodes []struct {
			Author *gqlLogin `json:"author"`
			State  string    `json:"state"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						TotalCount   int        `json:"totalCount"`
						RunCounts    []gqlCount `json:"checkRunCountsByState"`
						StatusCounts []gqlCount `json:"statusContextCountsByState"`
						Nodes        []struct {
							Name       string `json:"name"`
							Status     string `json:"status"`
							Conclusion string `json:"conclusion"`
							Context    string `json:"context"`
							State      string `json:"state"`
						} `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type gqlCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type gqlCommit struct {
	OID             string    `json:"oid"`
	MessageHeadline string    `json:"messageHeadline"`
	CommittedDate   time.Time `json:"committedDate"`
	Author          gqlAuthor `json:"author"`
}

type gqlRepo struct {
	NameWithOwner   string    `json:"nameWithOwner"`
	Description     string    `json:"description"`
	IsPrivate       bool      `json:"isPrivate"`
	IsArchived      bool      `json:"isArchived"`
	StargazerCount  int       `json:"stargazerCount"`
	ForkCount       int       `json:"forkCount"`
	PushedAt        time.Time `json:"pushedAt"`
	PrimaryLanguage *struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"primaryLanguage"`
	Languages struct {
		TotalSize int `json:"totalSize"`
		Edges     []struct {
			Size int `json:"size"`
			Node struct {
				Name  string `json:"name"`
				Color string `json:"color"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"languages"`
	Issues       struct{ TotalCount int } `json:"issues"`
	PullRequests struct{ TotalCount int } `json:"pullRequests"`
	Releases     struct {
		Nodes []struct {
			TagName string `json:"tagName"`
		} `json:"nodes"`
	} `json:"releases"`
	DefaultBranchRef *struct {
		Name   string `json:"name"`
		Target struct {
			gqlCommit
			StatusCheckRollup *struct {
				State string `json:"state"`
			} `json:"statusCheckRollup"`
			History struct {
				Nodes []gqlCommit `json:"nodes"`
			} `json:"history"`
		} `json:"target"`
	} `json:"defaultBranchRef"`
}

type gqlIssue struct {
	Number     int                      `json:"number"`
	Title      string                   `json:"title"`
	BodyText   string                   `json:"bodyText"`
	CreatedAt  time.Time                `json:"createdAt"`
	UpdatedAt  time.Time                `json:"updatedAt"`
	Comments   struct{ TotalCount int } `json:"comments"`
	Author     *gqlLogin                `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Labels struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
	Milestone *struct {
		Title              string  `json:"title"`
		ProgressPercentage float64 `json:"progressPercentage"`
	} `json:"milestone"`
	ClosedBy struct {
		Nodes []struct {
			Number int `json:"number"`
		} `json:"nodes"`
	} `json:"closedByPullRequestsReferences"`
}

type gqlViewer struct {
	Viewer struct {
		Login                   string `json:"login"`
		Name                    string `json:"name"`
		ContributionsCollection struct {
			TotalCommitContributions            int `json:"totalCommitContributions"`
			TotalPullRequestContributions       int `json:"totalPullRequestContributions"`
			TotalPullRequestReviewContributions int `json:"totalPullRequestReviewContributions"`
			TotalIssueContributions             int `json:"totalIssueContributions"`
			ContributionCalendar                struct {
				Weeks []struct {
					ContributionDays []struct {
						ContributionCount int    `json:"contributionCount"`
						Date              string `json:"date"`
						Weekday           int    `json:"weekday"`
					} `json:"contributionDays"`
				} `json:"weeks"`
			} `json:"contributionCalendar"`
		} `json:"contributionsCollection"`
	} `json:"viewer"`
}

type gqlRepoConn struct {
	Repositories struct {
		Nodes []gqlRepo `json:"nodes"`
	} `json:"repositories"`
}

type gqlSearch[T any] struct {
	Search struct {
		Nodes []T `json:"nodes"`
	} `json:"search"`
}

// searchScope appends an org: qualifier to a search query.
func searchScope(q, org string) string {
	if org != "" {
		q += " org:" + org
	}
	return q
}

// FetchGraphQL loads the GraphQL half of the dashboard. The viewer query must succeed; the repository,
// pull request, issue and profile sections fail independently and are reported in Dashboard.Warnings.
func (c *Client) FetchGraphQL(ctx context.Context, org string) (*Dashboard, error) {
	since := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	var (
		wg               sync.WaitGroup
		mu               sync.Mutex
		viewer           gqlViewer
		viewerErr        error
		repos            []gqlRepo
		reviewPRs, myPRs gqlSearch[gqlPR]
		issues           gqlSearch[gqlIssue]
		profile          *Profile
		warnings         []string
	)
	section := func(name string, f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != nil {
				mu.Lock()
				warnings = append(warnings, name+": "+err.Error())
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		viewerErr = c.graphQL(ctx, viewerQuery, nil, &viewer)
	}()
	section("repositories", func() error {
		if org != "" {
			var out struct {
				RepositoryOwner *gqlRepoConn `json:"repositoryOwner"`
			}
			if err := c.graphQL(ctx, ownerReposQuery, map[string]any{"owner": org, "since": since}, &out); err != nil {
				return err
			}
			if out.RepositoryOwner == nil {
				return &APIError{Status: 404, Message: "no organization or user named " + org}
			}
			repos = out.RepositoryOwner.Repositories.Nodes
			return nil
		}
		var out struct {
			Viewer gqlRepoConn `json:"viewer"`
		}
		err := c.graphQL(ctx, viewerReposQuery, map[string]any{"since": since}, &out)
		repos = out.Viewer.Repositories.Nodes
		return err
	})
	section("review requests", func() error {
		return c.graphQL(ctx, prSearchQuery, map[string]any{"q": searchScope("is:pr is:open review-requested:@me archived:false sort:updated-desc", org)}, &reviewPRs)
	})
	section("your pull requests", func() error {
		return c.graphQL(ctx, prSearchQuery, map[string]any{"q": searchScope("is:pr is:open author:@me archived:false sort:updated-desc", org)}, &myPRs)
	})
	section("issues", func() error {
		return c.graphQL(ctx, issueSearchQuery, map[string]any{"q": searchScope("is:issue is:open assignee:@me archived:false sort:updated-desc", org)}, &issues)
	})
	section("profile", func() error {
		var err error
		profile, err = c.FetchProfile(ctx)
		return err
	})
	wg.Wait()
	if viewerErr != nil {
		return nil, viewerErr
	}

	v := viewer.Viewer
	d := &Dashboard{User: User{Login: v.Login, Name: v.Name}, Profile: profile, FetchedAt: time.Now()}
	sort.Strings(warnings)
	d.Warnings = warnings
	cc := v.ContributionsCollection
	d.Breakdown = ContributionBreakdown{
		Commits: cc.TotalCommitContributions,
		Reviews: cc.TotalPullRequestReviewContributions,
		PRs:     cc.TotalPullRequestContributions,
		Issues:  cc.TotalIssueContributions,
	}
	for _, w := range cc.ContributionCalendar.Weeks {
		for _, day := range w.ContributionDays {
			t, err := time.Parse("2006-01-02", day.Date)
			if err != nil {
				continue
			}
			d.Contributions = append(d.Contributions, ContributionDay{Date: t, Count: day.ContributionCount, Weekday: day.Weekday})
		}
	}

	seen := map[string]bool{}
	for _, n := range reviewPRs.Search.Nodes {
		if n.Number == 0 {
			continue
		}
		pr := mapPR(n)
		pr.NeedsMyReview = true
		seen[pr.Key()] = true
		d.ReviewPRs = append(d.ReviewPRs, pr)
	}
	for _, n := range myPRs.Search.Nodes {
		if n.Number == 0 {
			continue
		}
		if pr := mapPR(n); !seen[pr.Key()] {
			d.MyPRs = append(d.MyPRs, pr)
		}
	}

	langSize := map[string]int{}
	langColor := map[string]string{}
	total := 0
	for _, r := range repos {
		if r.NameWithOwner == "" {
			continue
		}
		d.Repos = append(d.Repos, mapRepo(r))
		for _, e := range r.Languages.Edges {
			langSize[e.Node.Name] += e.Size
			langColor[e.Node.Name] = e.Node.Color
			total += e.Size
		}
	}
	d.Languages = buildLanguages(langSize, langColor, total)

	for _, n := range issues.Search.Nodes {
		if n.Number == 0 {
			continue
		}
		d.Issues = append(d.Issues, mapIssue(n))
	}
	return d, nil
}

func mapPR(n gqlPR) PullRequest {
	pr := PullRequest{
		Number:         n.Number,
		Title:          n.Title,
		Repo:           n.Repository.NameWithOwner,
		HeadRef:        n.HeadRefName,
		BaseRef:        n.BaseRefName,
		IsDraft:        n.IsDraft,
		CrossRepo:      n.IsCrossRepository,
		Mergeable:      n.Mergeable == "MERGEABLE",
		Conflicting:    n.Mergeable == "CONFLICTING",
		ReviewDecision: n.ReviewDecision,
		Additions:      n.Additions,
		Deletions:      n.Deletions,
		ChangedFiles:   n.ChangedFiles,
		UpdatedAt:      n.UpdatedAt,
	}
	switch r := n.Repository; {
	case r.SquashMergeAllowed:
		pr.MergeMethod = MergeSquash
	case r.MergeCommitAllowed:
		pr.MergeMethod = MergeCommit
	case r.RebaseMergeAllowed:
		pr.MergeMethod = MergeRebase
	}
	if n.Author != nil {
		pr.Author = n.Author.Login
	}
	if n.Files != nil {
		for _, f := range n.Files.Nodes {
			pr.Files = append(pr.Files, ChangedFile{Path: f.Path, Additions: f.Additions, Deletions: f.Deletions})
		}
	}
	for _, r := range n.LatestReviews.Nodes {
		if r.Author != nil {
			pr.Reviewers = append(pr.Reviewers, Reviewer{Login: r.Author.Login, State: r.State})
		}
	}
	for _, r := range n.ReviewRequests.Nodes {
		name := r.RequestedReviewer.Login
		if name == "" {
			name = r.RequestedReviewer.Name
		}
		if name != "" {
			pr.Reviewers = append(pr.Reviewers, Reviewer{Login: name, State: "PENDING"})
		}
	}
	if len(n.Commits.Nodes) > 0 {
		if roll := n.Commits.Nodes[0].Commit.StatusCheckRollup; roll != nil {
			pr.CIState = normalizeRollup(roll.State)
			for _, c := range roll.Contexts.Nodes {
				if c.Name != "" {
					pr.Checks = append(pr.Checks, CheckRun{Name: c.Name, State: checkState(c.Status, c.Conclusion)})
				} else if c.Context != "" {
					pr.Checks = append(pr.Checks, CheckRun{Name: c.Context, State: normalizeRollup(c.State)})
				}
			}
			// Failing checks first, so the inspector shows what is red even when there are many.
			sort.SliceStable(pr.Checks, func(i, j int) bool { return checkRank(pr.Checks[i].State) < checkRank(pr.Checks[j].State) })
			// The node list is capped; the per-state counts cover every check on the commit.
			for _, c := range append(roll.Contexts.RunCounts, roll.Contexts.StatusCounts...) {
				switch normalizeRollup(c.State) {
				case StateSuccess:
					pr.ChecksPassed += c.Count
				case StateFailure:
					pr.ChecksFailed += c.Count
				default:
					pr.ChecksPending += c.Count
				}
			}
			pr.ChecksTotal = roll.Contexts.TotalCount
		}
	}
	return pr
}

func mapRepo(r gqlRepo) Repo {
	repo := Repo{
		NameWithOwner: r.NameWithOwner,
		Description:   r.Description,
		IsPrivate:     r.IsPrivate,
		IsArchived:    r.IsArchived,
		Stars:         r.StargazerCount,
		Forks:         r.ForkCount,
		PushedAt:      r.PushedAt,
		OpenIssues:    r.Issues.TotalCount,
		OpenPRs:       r.PullRequests.TotalCount,
		Cadence:       make([]int, 30),
	}
	if r.PrimaryLanguage != nil {
		repo.Language = r.PrimaryLanguage.Name
		repo.LanguageColor = r.PrimaryLanguage.Color
	}
	if repo.LanguageColor == "" && repo.Language != "" {
		repo.LanguageColor = "#8b949e"
	}
	if len(r.Releases.Nodes) > 0 {
		repo.Release = r.Releases.Nodes[0].TagName
	}
	if b := r.DefaultBranchRef; b != nil {
		repo.DefaultBranch = b.Name
		repo.SyncBranch = b.Name
		repo.LatestSHA = shortSHA(b.Target.OID)
		repo.LatestMessage = b.Target.MessageHeadline
		repo.LatestAuthor = b.Target.Author.display()
		if b.Target.StatusCheckRollup != nil {
			repo.CIState = normalizeRollup(b.Target.StatusCheckRollup.State)
		}
		now := time.Now()
		for i, c := range b.Target.History.Nodes {
			if i < 6 {
				repo.RecentCommits = append(repo.RecentCommits, Commit{SHA: shortSHA(c.OID), Message: c.MessageHeadline, Author: c.Author.display(), Date: c.CommittedDate})
			}
			age := int(now.Sub(c.CommittedDate).Hours() / 24)
			if age >= 0 && age < 30 {
				repo.Cadence[29-age]++
			}
		}
	}
	return repo
}

func mapIssue(n gqlIssue) Issue {
	is := Issue{
		Number:    n.Number,
		Title:     n.Title,
		Repo:      n.Repository.NameWithOwner,
		Labels:    n.Labels.Nodes,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
		Comments:  n.Comments.TotalCount,
		Body:      n.BodyText,
	}
	if n.Author != nil {
		is.Author = n.Author.Login
	}
	is.Priority = PriorityFromLabels(is.Labels)
	if n.Milestone != nil {
		is.Milestone = n.Milestone.Title
		is.MilestonePct = n.Milestone.ProgressPercentage
	}
	if len(n.ClosedBy.Nodes) > 0 {
		is.LinkedPR = n.ClosedBy.Nodes[0].Number
	}
	return is
}

// PriorityFromLabels derives P0-P3 from common label conventions.
func PriorityFromLabels(labels []Label) string {
	best := ""
	for _, l := range labels {
		n := strings.ToLower(l.Name)
		p := ""
		switch {
		case strings.Contains(n, "p0"), strings.Contains(n, "critical"), strings.Contains(n, "blocker"), strings.Contains(n, "urgent"):
			p = "P0"
		case strings.Contains(n, "p1"), strings.Contains(n, "high"):
			p = "P1"
		case strings.Contains(n, "p2"), strings.Contains(n, "medium"), strings.Contains(n, "bug"):
			p = "P2"
		case strings.Contains(n, "p3"), strings.Contains(n, "low"):
			p = "P3"
		}
		if p != "" && (best == "" || p < best) {
			best = p
		}
	}
	if best == "" {
		return "P3"
	}
	return best
}

func buildLanguages(size map[string]int, color map[string]string, total int) []LangStat {
	if total == 0 {
		return nil
	}
	var langs []LangStat
	for name, s := range size {
		c := color[name]
		if c == "" {
			c = "#8b949e"
		}
		langs = append(langs, LangStat{Name: name, Color: c, Percent: float64(s) * 100 / float64(total), Bytes: s})
	}
	sort.Slice(langs, func(i, j int) bool {
		if langs[i].Percent != langs[j].Percent {
			return langs[i].Percent > langs[j].Percent
		}
		return langs[i].Name < langs[j].Name
	})
	if len(langs) > 5 {
		other := 0.0
		for _, l := range langs[4:] {
			other += l.Percent
		}
		langs = append(langs[:4], LangStat{Name: "Other", Color: "#8b949e", Percent: other})
	}
	return langs
}

func checkRank(state string) int {
	switch state {
	case StateFailure:
		return 0
	case StatePending:
		return 1
	default:
		return 2
	}
}

// normalizeRollup maps rollup, status-context and check-run states onto SUCCESS / FAILURE / PENDING.
func normalizeRollup(s string) string {
	switch strings.ToUpper(s) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return StateSuccess
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
		return StateFailure
	case "PENDING", "EXPECTED", "IN_PROGRESS", "QUEUED", "WAITING", "REQUESTED", "COMPLETED":
		return StatePending
	default:
		return StateNone
	}
}

func checkState(status, conclusion string) string {
	if !strings.EqualFold(status, "COMPLETED") {
		return StatePending
	}
	switch strings.ToUpper(conclusion) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return StateSuccess
	default:
		return StateFailure
	}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
