package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	graphqlURL = "https://api.github.com/graphql"
	restURL    = "https://api.github.com"
)

// Client talks to the GitHub GraphQL and REST APIs with a bearer token.
type Client struct {
	token string
	http  *http.Client
}

// NewClient returns a Client with a sane request timeout.
func NewClient(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

const dashboardQuery = `
query GhxDashboardBatch($orgQuery: String!, $prReviewQuery: String!, $issueQuery: String!, $since: GitTimestamp!) {
  viewer {
    login
    name
    contributionsCollection {
      totalCommitContributions
      totalPullRequestContributions
      totalPullRequestReviewContributions
      totalIssueContributions
      contributionCalendar {
        totalContributions
        weeks { contributionDays { contributionCount date weekday color } }
      }
    }
    repositories(first: 15, orderBy: {field: PUSHED_AT, direction: DESC}, affiliations: [OWNER, ORGANIZATION_MEMBER, COLLABORATOR]) {
      nodes {
        nameWithOwner
        description
        isPrivate
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
      }
    }
  }
  reviewRequestedPRs: search(query: $prReviewQuery, type: ISSUE, first: 15) {
    nodes { ...PRFields }
  }
  myOpenPRs: search(query: $orgQuery, type: ISSUE, first: 15) {
    nodes { ...PRFields }
  }
  assignedIssues: search(query: $issueQuery, type: ISSUE, first: 15) {
    nodes {
      ... on Issue {
        number
        title
        state
        bodyText
        createdAt
        updatedAt
        comments { totalCount }
        repository { nameWithOwner }
        labels(first: 4) { nodes { name color } }
        milestone { title progressPercentage }
        closedByPullRequestsReferences(first: 1) { nodes { number } }
      }
    }
  }
}

fragment PRFields on PullRequest {
  number
  title
  isDraft
  mergeable
  reviewDecision
  additions
  deletions
  changedFiles
  updatedAt
  headRefName
  baseRefName
  author { login }
  repository { nameWithOwner }
  files(first: 8) { nodes { path additions deletions } }
  latestReviews(first: 6) { nodes { author { login } state } }
  reviewRequests(first: 6) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
  commits(last: 1) {
    nodes {
      commit {
        statusCheckRollup {
          state
          contexts(first: 10) {
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

type gqlPR struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	IsDraft        bool      `json:"isDraft"`
	Mergeable      string    `json:"mergeable"`
	ReviewDecision string    `json:"reviewDecision"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changedFiles"`
	UpdatedAt      time.Time `json:"updatedAt"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Author         *struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
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
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
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
						Nodes []struct {
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

type gqlResponse struct {
	Data struct {
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
			Repositories struct {
				Nodes []gqlRepo `json:"nodes"`
			} `json:"repositories"`
		} `json:"viewer"`
		ReviewRequestedPRs struct {
			Nodes []gqlPR `json:"nodes"`
		} `json:"reviewRequestedPRs"`
		MyOpenPRs struct {
			Nodes []gqlPR `json:"nodes"`
		} `json:"myOpenPRs"`
		AssignedIssues struct {
			Nodes []gqlIssue `json:"nodes"`
		} `json:"assignedIssues"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// FetchGraphQL runs the single batched dashboard query, optionally scoped to an org.
func (c *Client) FetchGraphQL(ctx context.Context, org string) (*Dashboard, error) {
	scope := ""
	if org != "" {
		scope = " org:" + org
	}
	since := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	vars := map[string]string{
		"orgQuery":      "is:pr is:open author:@me sort:updated-desc" + scope,
		"prReviewQuery": "is:pr is:open review-requested:@me sort:updated-desc" + scope,
		"issueQuery":    "is:issue is:open assignee:@me sort:updated-desc" + scope,
		"since":         since,
	}
	body, err := json.Marshal(map[string]any{"query": dashboardQuery, "variables": vars})
	if err != nil {
		return nil, err
	}
	// GitHub occasionally answers heavy batched queries with a 502/504; retry those once.
	var out *gqlResponse
	for attempt := 0; attempt < 2; attempt++ {
		var retry bool
		out, retry, err = c.postGraphQL(ctx, body)
		if err == nil || !retry {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return mapDashboard(out, org), nil
}

func (c *Client) postGraphQL(ctx context.Context, body []byte) (out *gqlResponse, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode >= 500, fmt.Errorf("graphql: HTTP %d", resp.StatusCode)
	}
	out = &gqlResponse{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, false, fmt.Errorf("graphql decode: %w", err)
	}
	if len(out.Errors) > 0 && out.Data.Viewer.Login == "" {
		return nil, false, fmt.Errorf("graphql: %s", out.Errors[0].Message)
	}
	return out, false, nil
}

func mapDashboard(out *gqlResponse, org string) *Dashboard {
	v := out.Data.Viewer
	d := &Dashboard{User: User{Login: v.Login, Name: v.Name}, FetchedAt: time.Now()}
	cc := v.ContributionsCollection
	d.Breakdown = ContributionBreakdown{
		Commits: cc.TotalCommitContributions,
		Reviews: cc.TotalPullRequestReviewContributions,
		PRs:     cc.TotalPullRequestContributions,
		Issues:  cc.TotalIssueContributions,
	}

	for _, w := range v.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range w.ContributionDays {
			t, err := time.Parse("2006-01-02", day.Date)
			if err != nil {
				continue
			}
			d.Contributions = append(d.Contributions, ContributionDay{Date: t, Count: day.ContributionCount, Weekday: day.Weekday})
		}
	}

	for _, n := range out.Data.ReviewRequestedPRs.Nodes {
		if n.Number == 0 {
			continue
		}
		pr := mapPR(n)
		pr.NeedsMyReview = true
		d.ReviewPRs = append(d.ReviewPRs, pr)
	}
	for _, n := range out.Data.MyOpenPRs.Nodes {
		if n.Number == 0 {
			continue
		}
		d.MyPRs = append(d.MyPRs, mapPR(n))
	}

	langSize := map[string]int{}
	langColor := map[string]string{}
	total := 0
	for _, r := range v.Repositories.Nodes {
		if org != "" && !strings.EqualFold(strings.SplitN(r.NameWithOwner, "/", 2)[0], org) {
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

	for _, n := range out.Data.AssignedIssues.Nodes {
		if n.Number == 0 {
			continue
		}
		d.Issues = append(d.Issues, mapIssue(n))
	}
	return d
}

func mapPR(n gqlPR) PullRequest {
	pr := PullRequest{
		Number:         n.Number,
		Title:          n.Title,
		Repo:           n.Repository.NameWithOwner,
		HeadRef:        n.HeadRefName,
		BaseRef:        n.BaseRefName,
		IsDraft:        n.IsDraft,
		Mergeable:      n.Mergeable == "MERGEABLE",
		ReviewDecision: n.ReviewDecision,
		Additions:      n.Additions,
		Deletions:      n.Deletions,
		ChangedFiles:   n.ChangedFiles,
		UpdatedAt:      n.UpdatedAt,
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
		}
	}
	return pr
}

func mapRepo(r gqlRepo) Repo {
	repo := Repo{
		NameWithOwner: r.NameWithOwner,
		Description:   r.Description,
		IsPrivate:     r.IsPrivate,
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
		langs = append(langs, LangStat{Name: name, Color: c, Percent: float64(s) * 100 / float64(total)})
	}
	sort.Slice(langs, func(i, j int) bool { return langs[i].Percent > langs[j].Percent })
	if len(langs) > 5 {
		other := 0.0
		for _, l := range langs[4:] {
			other += l.Percent
		}
		langs = append(langs[:4], LangStat{Name: "Other", Color: "#8b949e", Percent: other})
	}
	return langs
}

func normalizeRollup(s string) string {
	switch strings.ToUpper(s) {
	case "SUCCESS":
		return StateSuccess
	case "FAILURE", "ERROR":
		return StateFailure
	case "PENDING", "EXPECTED":
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
