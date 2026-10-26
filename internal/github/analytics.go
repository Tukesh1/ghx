package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrStatsPending is returned when GitHub answers 202: repository statistics are still being computed.
var ErrStatsPending = errors.New("github is computing repository statistics")

const repoInsightsQuery = `
query GhxRepoInsights($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    watchers { totalCount }
    diskUsage
    licenseInfo { spdxId name }
    repositoryTopics(first: 8) { nodes { topic { name } } }
    homepageUrl
    createdAt
    isFork
    isArchived
    releases { totalCount }
    refs(refPrefix: "refs/heads/") { totalCount }
    closedIssues: issues(states: CLOSED) { totalCount }
    mergedPRs: pullRequests(states: MERGED) { totalCount }
    closedPRs: pullRequests(states: CLOSED) { totalCount }
    languages(first: 8, orderBy: {field: SIZE, direction: DESC}) {
      totalSize
      edges { size node { name color } }
    }
  }
}`

type insightsResponse struct {
	Data struct {
		Repository *struct {
			Watchers    struct{ TotalCount int } `json:"watchers"`
			DiskUsage   int                      `json:"diskUsage"`
			LicenseInfo *struct {
				SpdxID string `json:"spdxId"`
				Name   string `json:"name"`
			} `json:"licenseInfo"`
			RepositoryTopics struct {
				Nodes []struct {
					Topic struct {
						Name string `json:"name"`
					} `json:"topic"`
				} `json:"nodes"`
			} `json:"repositoryTopics"`
			HomepageURL  string                   `json:"homepageUrl"`
			CreatedAt    time.Time                `json:"createdAt"`
			IsFork       bool                     `json:"isFork"`
			IsArchived   bool                     `json:"isArchived"`
			Releases     struct{ TotalCount int } `json:"releases"`
			Refs         struct{ TotalCount int } `json:"refs"`
			ClosedIssues struct{ TotalCount int } `json:"closedIssues"`
			MergedPRs    struct{ TotalCount int } `json:"mergedPRs"`
			ClosedPRs    struct{ TotalCount int } `json:"closedPRs"`
			Languages    struct {
				TotalSize int `json:"totalSize"`
				Edges     []struct {
					Size int `json:"size"`
					Node struct {
						Name  string `json:"name"`
						Color string `json:"color"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"languages"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) fetchInsightsGraphQL(ctx context.Context, repo string, a *RepoAnalytics) error {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return fmt.Errorf("bad repo name %q", repo)
	}
	body, err := json.Marshal(map[string]any{"query": repoInsightsQuery, "variables": map[string]string{"owner": owner, "name": name}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql: HTTP %d", resp.StatusCode)
	}
	var out insightsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	r := out.Data.Repository
	if r == nil {
		if len(out.Errors) > 0 {
			return fmt.Errorf("graphql: %s", out.Errors[0].Message)
		}
		return fmt.Errorf("repository %s not found", repo)
	}
	a.Watchers = r.Watchers.TotalCount
	a.DiskKB = r.DiskUsage
	if r.LicenseInfo != nil {
		a.License = r.LicenseInfo.SpdxID
		if a.License == "" || a.License == "NOASSERTION" {
			a.License = r.LicenseInfo.Name
		}
	}
	for _, t := range r.RepositoryTopics.Nodes {
		a.Topics = append(a.Topics, t.Topic.Name)
	}
	a.Homepage = r.HomepageURL
	a.CreatedAt = r.CreatedAt
	a.IsFork, a.IsArchived = r.IsFork, r.IsArchived
	a.Releases = r.Releases.TotalCount
	a.Branches = r.Refs.TotalCount
	a.ClosedIssues = r.ClosedIssues.TotalCount
	a.MergedPRs = r.MergedPRs.TotalCount
	a.ClosedPRs = r.ClosedPRs.TotalCount
	total := max(r.Languages.TotalSize, 1)
	for _, e := range r.Languages.Edges {
		color := e.Node.Color
		if color == "" {
			color = "#8b949e"
		}
		a.Languages = append(a.Languages, LangStat{Name: e.Node.Name, Color: color, Bytes: e.Size, Percent: float64(e.Size) * 100 / float64(total)})
	}
	return nil
}

// FetchRepoAnalytics loads insight data for one repository: a GraphQL metadata query plus REST
// statistics (commit activity, code frequency, contributors) and traffic, all concurrently.
// Traffic requires push access and statistics may still be computing; both degrade gracefully.
func (c *Client) FetchRepoAnalytics(ctx context.Context, repo string) (*RepoAnalytics, error) {
	a := &RepoAnalytics{Repo: repo, FetchedAt: time.Now()}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		gqlErr  error
		pending bool
	)
	run := func(f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); errors.Is(err, ErrStatsPending) {
				mu.Lock()
				pending = true
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		gqlErr = c.fetchInsightsGraphQL(ctx, repo, a)
	}()
	run(func() error {
		var weeks []struct {
			Total int   `json:"total"`
			Week  int64 `json:"week"`
		}
		if err := c.get(ctx, "/repos/"+repo+"/stats/commit_activity", &weeks); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for i, w := range weeks {
			if i == 0 {
				a.WeeksStart = time.Unix(w.Week, 0)
			}
			a.CommitWeeks = append(a.CommitWeeks, w.Total)
		}
		return nil
	})
	run(func() error {
		var rows [][3]int64
		if err := c.get(ctx, "/repos/"+repo+"/stats/code_frequency", &rows); err != nil {
			return err
		}
		if len(rows) > 26 {
			rows = rows[len(rows)-26:]
		}
		mu.Lock()
		defer mu.Unlock()
		for _, r := range rows {
			a.CodeFreq = append(a.CodeFreq, WeekDelta{Week: time.Unix(r[0], 0), Add: int(r[1]), Del: int(-r[2])})
		}
		return nil
	})
	run(func() error {
		var cs []struct {
			Login         string `json:"login"`
			Contributions int    `json:"contributions"`
		}
		if err := c.get(ctx, "/repos/"+repo+"/contributors?per_page=8", &cs); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for _, x := range cs {
			a.Contributors = append(a.Contributors, Contributor{Login: x.Login, Commits: x.Contributions})
		}
		return nil
	})
	type traffic struct {
		Count   int `json:"count"`
		Uniques int `json:"uniques"`
		Days    []struct {
			Count int `json:"count"`
		} `json:"views"`
		CloneDays []struct {
			Count int `json:"count"`
		} `json:"clones"`
	}
	run(func() error {
		var t traffic
		if err := c.get(ctx, "/repos/"+repo+"/traffic/views", &t); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		a.TrafficOK = true
		a.ViewsTotal, a.ViewsUnique = t.Count, t.Uniques
		for _, d := range t.Days {
			a.Views = append(a.Views, d.Count)
		}
		return nil
	})
	run(func() error {
		var t traffic
		if err := c.get(ctx, "/repos/"+repo+"/traffic/clones", &t); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		a.ClonesTotal, a.ClonesUnique = t.Count, t.Uniques
		for _, d := range t.CloneDays {
			a.Clones = append(a.Clones, d.Count)
		}
		return nil
	})
	run(func() error {
		var refs []struct {
			Referrer string `json:"referrer"`
			Count    int    `json:"count"`
			Uniques  int    `json:"uniques"`
		}
		if err := c.get(ctx, "/repos/"+repo+"/traffic/popular/referrers", &refs); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for _, r := range refs {
			a.Referrers = append(a.Referrers, Referrer{Source: r.Referrer, Count: r.Count, Uniques: r.Uniques})
		}
		return nil
	})
	wg.Wait()
	a.StatsPending = pending
	if gqlErr != nil {
		return nil, gqlErr
	}
	return a, nil
}

func seedFor(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// mockRepoLanguages gives each demo repository a plausible language mix.
var mockRepoLanguages = map[string][]LangStat{
	"acme/orbit-engine":    {{"Rust", "#dea584", 82, 0}, {"Shell", "#89e051", 8, 0}, {"Python", "#3572A5", 6, 0}, {"Dockerfile", "#384d54", 4, 0}},
	"octocat/ghx":          {{"Go", "#00add8", 91, 0}, {"Makefile", "#427819", 5, 0}, {"Shell", "#89e051", 4, 0}},
	"acme/web-console":     {{"TypeScript", "#3178c6", 74, 0}, {"CSS", "#663399", 14, 0}, {"JavaScript", "#f1e05a", 9, 0}, {"HTML", "#e34c26", 3, 0}},
	"acme/ml-pipelines":    {{"Python", "#3572A5", 78, 0}, {"Jupyter Notebook", "#DA5B0B", 15, 0}, {"Shell", "#89e051", 7, 0}},
	"acme/platform-rfcs":   {{"Markdown", "#083fa1", 96, 0}, {"Mermaid", "#ff3670", 4, 0}},
	"octocat/dotfiles":     {{"Lua", "#000080", 61, 0}, {"Shell", "#89e051", 32, 0}, {"Vim Script", "#199f4b", 7, 0}},
	"acme/terraform-infra": {{"HCL", "#844FBA", 88, 0}, {"Shell", "#89e051", 9, 0}, {"Go", "#00add8", 3, 0}},
}

var mockTopics = map[string][]string{
	"acme/orbit-engine":    {"rust", "streaming", "io-uring", "kafka"},
	"octocat/ghx":          {"tui", "github", "cli", "bubbletea"},
	"acme/web-console":     {"nextjs", "react", "admin"},
	"acme/ml-pipelines":    {"mlops", "airflow", "feature-store"},
	"acme/platform-rfcs":   {"adr", "architecture"},
	"octocat/dotfiles":     {"neovim", "zsh", "tmux"},
	"acme/terraform-infra": {"terraform", "aws", "eks"},
}

// MockAnalytics returns deterministic, realistic insight data for a demo repository.
func MockAnalytics(r Repo) *RepoAnalytics {
	rng := rand.New(rand.NewSource(seedFor(r.NameWithOwner)))
	now := time.Now()
	scale := 1.0 + math.Log10(float64(r.Stars+10))
	a := &RepoAnalytics{
		Repo:         r.NameWithOwner,
		Watchers:     r.Stars/12 + rng.Intn(20) + 3,
		DiskKB:       2_000 + rng.Intn(80_000),
		Releases:     rng.Intn(60) + 1,
		Branches:     rng.Intn(40) + 3,
		ClosedIssues: r.OpenIssues*6 + rng.Intn(200),
		MergedPRs:    r.OpenPRs*14 + rng.Intn(300) + 10,
		ClosedPRs:    rng.Intn(40) + 2,
		License:      []string{"MIT", "Apache-2.0", "BSD-3-Clause", "MPL-2.0"}[rng.Intn(4)],
		Topics:       mockTopics[r.NameWithOwner],
		CreatedAt:    now.AddDate(-1-rng.Intn(5), -rng.Intn(11), 0),
		Languages:    mockRepoLanguages[r.NameWithOwner],
		WeeksStart:   now.AddDate(0, 0, -7*52),
		TrafficOK:    !strings.HasPrefix(r.NameWithOwner, "acme/platform"),
		FetchedAt:    now,
	}
	if r.IsPrivate {
		a.License = ""
	}
	if len(a.Languages) == 0 && r.Language != "" {
		a.Languages = []LangStat{{r.Language, r.LanguageColor, 100, 0}}
	}
	for i := range a.Languages {
		a.Languages[i].Bytes = int(a.Languages[i].Percent * float64(a.DiskKB) * 8)
	}
	for w := 0; w < 52; w++ {
		season := 0.55 + 0.45*math.Sin(float64(w)/6.5+float64(rng.Intn(3)))
		v := int(scale * 3 * season * (0.4 + rng.Float64()))
		if w > 44 {
			v += int(scale * 2)
		}
		a.CommitWeeks = append(a.CommitWeeks, v)
	}
	for w := 0; w < 26; w++ {
		add := int(scale * 60 * (0.3 + rng.Float64()*1.4))
		del := int(float64(add) * (0.2 + rng.Float64()*0.7))
		if rng.Float64() < 0.08 {
			add *= 4
		}
		a.CodeFreq = append(a.CodeFreq, WeekDelta{Week: now.AddDate(0, 0, -7*(26-w)), Add: add, Del: del})
	}
	people := []string{"octocat", "kdubois", "hsuzuki", "priya-r", "lchen", "jmartin", "dependabot[bot]", "release-bot"}
	rng.Shuffle(len(people)-1, func(i, j int) { people[i+1], people[j+1] = people[j+1], people[i+1] })
	base := int(scale * 180)
	for i, p := range people[:6] {
		a.Contributors = append(a.Contributors, Contributor{Login: p, Commits: base / (i + 1) * (80 + rng.Intn(40)) / 100})
	}
	sort.Slice(a.Contributors, func(i, j int) bool { return a.Contributors[i].Commits > a.Contributors[j].Commits })
	if a.TrafficOK {
		for d := 0; d < 14; d++ {
			wk := 1.0
			if wd := now.AddDate(0, 0, d-13).Weekday(); wd == time.Saturday || wd == time.Sunday {
				wk = 0.45
			}
			v := int(scale * 22 * wk * (0.6 + rng.Float64()*0.8))
			c := v / (4 + rng.Intn(4))
			a.Views = append(a.Views, v)
			a.Clones = append(a.Clones, c)
			a.ViewsTotal += v
			a.ClonesTotal += c
		}
		a.ViewsUnique = a.ViewsTotal * 3 / 10
		a.ClonesUnique = a.ClonesTotal * 4 / 10
		for i, src := range []string{"github.com", "news.ycombinator.com", "Google", "reddit.com", "t.co"} {
			cnt := a.ViewsTotal / (3 + i*2)
			a.Referrers = append(a.Referrers, Referrer{Source: src, Count: cnt, Uniques: cnt / 3})
		}
	}
	return a
}
