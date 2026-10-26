package github

import (
	"sort"
	"time"
)

// CI rollup / check states shared by PRs, repos and runs.
const (
	StateSuccess = "SUCCESS"
	StateFailure = "FAILURE"
	StatePending = "PENDING"
	StateNone    = ""
)

// Review decisions as returned by the GraphQL API.
const (
	ReviewApproved         = "APPROVED"
	ReviewChangesRequested = "CHANGES_REQUESTED"
	ReviewRequired         = "REVIEW_REQUIRED"
)

// User is the authenticated viewer.
type User struct {
	Login string
	Name  string
}

// ChangedFile is one file in a pull request diff.
type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
}

// CheckRun is a single CI check attached to a PR head commit.
type CheckRun struct {
	Name  string
	State string
}

// Reviewer is a requested or completed reviewer on a PR.
type Reviewer struct {
	Login string
	State string
}

// PullRequest is an open PR either awaiting the viewer's review or authored by the viewer.
type PullRequest struct {
	Number         int
	Title          string
	Repo           string
	Author         string
	HeadRef        string
	BaseRef        string
	IsDraft        bool
	Merged         bool
	Mergeable      bool
	ReviewDecision string
	CIState        string
	Additions      int
	Deletions      int
	ChangedFiles   int
	UpdatedAt      time.Time
	NeedsMyReview  bool
	Files          []ChangedFile
	Checks         []CheckRun
	Reviewers      []Reviewer
}

// MergeReady reports whether the PR is approved, green and mergeable.
func (p PullRequest) MergeReady() bool {
	return !p.IsDraft && p.Mergeable && p.ReviewDecision == ReviewApproved && p.CIState == StateSuccess
}

// Step is one step of a workflow job.
type Step struct {
	Number     int
	Name       string
	Status     string
	Conclusion string
	Duration   time.Duration
	Log        []string
}

// Job is one job of a workflow run.
type Job struct {
	ID         int64
	Name       string
	Status     string
	Conclusion string
	Steps      []Step
}

// WorkflowRun is a GitHub Actions run.
type WorkflowRun struct {
	ID         int64
	Name       string
	Repo       string
	Branch     string
	Event      string
	SHA        string
	Status     string
	Conclusion string
	StartedAt  time.Time
	Duration   time.Duration
	Jobs       []Job
	FailureLog []string
}

// State collapses status/conclusion into SUCCESS, FAILURE or PENDING.
func (r WorkflowRun) State() string {
	if r.Status != "completed" {
		return StatePending
	}
	switch r.Conclusion {
	case "success", "skipped", "neutral":
		return StateSuccess
	default:
		return StateFailure
	}
}

// Commit is a commit on a repository's default branch.
type Commit struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

// Repo is an active repository with branch sync and CI status.
type Repo struct {
	NameWithOwner string
	IsPrivate     bool
	Description   string
	Stars         int
	Forks         int
	Language      string
	LanguageColor string
	DefaultBranch string
	SyncBranch    string
	Ahead         int
	Behind        int
	LatestSHA     string
	LatestMessage string
	LatestAuthor  string
	OpenPRs       int
	OpenIssues    int
	CIState       string
	PushedAt      time.Time
	Release       string
	RecentCommits []Commit
	Cadence       []int
}

// Label is an issue label.
type Label struct {
	Name  string
	Color string
}

// Issue is an open issue assigned to the viewer.
type Issue struct {
	Number       int
	Title        string
	Repo         string
	Labels       []Label
	Priority     string
	LinkedPR     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Comments     int
	Body         string
	Milestone    string
	MilestonePct float64
}

// Notification is an unread inbox item.
type Notification struct {
	ID        string
	Reason    string
	Title     string
	Type      string
	Repo      string
	UpdatedAt time.Time
}

// ContributionDay is one cell of the contribution calendar.
type ContributionDay struct {
	Date    time.Time
	Count   int
	Weekday int
}

// LangStat is one segment of the language bar.
type LangStat struct {
	Name    string
	Color   string
	Percent float64
	Bytes   int
}

// Contributor is one of a repository's top committers.
type Contributor struct {
	Login   string
	Commits int
}

// WeekDelta is one week of code frequency: lines added and deleted.
type WeekDelta struct {
	Week time.Time
	Add  int
	Del  int
}

// Referrer is a traffic source for a repository over the last 14 days.
type Referrer struct {
	Source  string
	Count   int
	Uniques int
}

// RepoAnalytics is the deep, per-repository insight data loaded on demand.
type RepoAnalytics struct {
	Repo         string
	Watchers     int
	DiskKB       int
	Releases     int
	Branches     int
	ClosedIssues int
	MergedPRs    int
	ClosedPRs    int
	License      string
	Topics       []string
	Homepage     string
	CreatedAt    time.Time
	IsFork       bool
	IsArchived   bool
	Languages    []LangStat
	CommitWeeks  []int
	WeeksStart   time.Time
	CodeFreq     []WeekDelta
	Contributors []Contributor
	Views        []int
	Clones       []int
	ViewsTotal   int
	ViewsUnique  int
	ClonesTotal  int
	ClonesUnique int
	Referrers    []Referrer
	TrafficOK    bool
	StatsPending bool
	FetchedAt    time.Time
}

// TotalCommits52w sums the weekly commit activity.
func (a *RepoAnalytics) TotalCommits52w() int {
	n := 0
	for _, c := range a.CommitWeeks {
		n += c
	}
	return n
}

// DependabotAlert is an open vulnerable dependency with a patched version available.
type DependabotAlert struct {
	Repo           string
	Package        string
	Ecosystem      string
	CVE            string
	Severity       string
	CurrentVersion string
	PatchedVersion string
}

// CIDay aggregates workflow runs for one calendar day.
type CIDay struct {
	Date   time.Time
	Passed int
	Failed int
	P50    time.Duration
}

// ContributionBreakdown splits the year's contributions by type.
type ContributionBreakdown struct {
	Commits int
	Reviews int
	PRs     int
	Issues  int
}

// Dashboard is the full dataset rendered by the UI.
type Dashboard struct {
	User          User
	Breakdown     ContributionBreakdown
	ReviewPRs     []PullRequest
	MyPRs         []PullRequest
	Runs          []WorkflowRun
	Repos         []Repo
	Issues        []Issue
	Notifications []Notification
	Contributions []ContributionDay
	Languages     []LangStat
	Alerts        []DependabotAlert
	CIHistory     []CIDay
	FetchedAt     time.Time
	Demo          bool
}

// ActionPRs returns review-queue PRs followed by the viewer's own PRs.
func (d *Dashboard) ActionPRs() []PullRequest {
	out := make([]PullRequest, 0, len(d.ReviewPRs)+len(d.MyPRs))
	out = append(out, d.ReviewPRs...)
	return append(out, d.MyPRs...)
}

// MergeReadyCount counts the viewer's PRs that can be merged now.
func (d *Dashboard) MergeReadyCount() int {
	n := 0
	for _, p := range d.MyPRs {
		if p.MergeReady() {
			n++
		}
	}
	return n
}

// RunCounts returns failing and running workflow run counts.
func (d *Dashboard) RunCounts() (failing, running int) {
	for _, r := range d.Runs {
		switch r.State() {
		case StateFailure:
			failing++
		case StatePending:
			running++
		}
	}
	return failing, running
}

// BlockerCount counts P0 issues.
func (d *Dashboard) BlockerCount() int {
	n := 0
	for _, i := range d.Issues {
		if i.Priority == "P0" {
			n++
		}
	}
	return n
}

// ContributionsSince sums contributions over the trailing number of days.
func (d *Dashboard) ContributionsSince(days int) int {
	cut := d.lastDay().AddDate(0, 0, -days)
	total := 0
	for _, c := range d.Contributions {
		if c.Date.After(cut) {
			total += c.Count
		}
	}
	return total
}

// TotalContributions sums the whole calendar.
func (d *Dashboard) TotalContributions() int {
	total := 0
	for _, c := range d.Contributions {
		total += c.Count
	}
	return total
}

// Streak returns the number of consecutive days with contributions ending today (or yesterday).
func (d *Dashboard) Streak() int {
	days := d.sortedDays()
	if len(days) == 0 {
		return 0
	}
	i := len(days) - 1
	if days[i].Count == 0 {
		i--
	}
	n := 0
	for ; i >= 0 && days[i].Count > 0; i-- {
		n++
	}
	return n
}

// Weeks groups the calendar into columns of 7 weekday levels (0-4), oldest first.
func (d *Dashboard) Weeks() [][]int {
	weeks, _ := d.WeeksWithStarts()
	return weeks
}

// WeeksWithStarts is Weeks plus the first date of each week column, for month labels.
func (d *Dashboard) WeeksWithStarts() ([][]int, []time.Time) {
	days := d.sortedDays()
	if len(days) == 0 {
		return nil, nil
	}
	var weeks [][]int
	var starts []time.Time
	week := make([]int, 7)
	started := false
	for _, day := range days {
		wd := day.Weekday
		if wd == 0 && started {
			weeks = append(weeks, week)
			week = make([]int, 7)
		}
		if wd == 0 || !started {
			starts = append(starts, day.Date)
		}
		started = true
		week[wd] = Level(day.Count)
	}
	weeks = append(weeks, week)
	if len(weeks) > 53 {
		weeks = weeks[len(weeks)-53:]
		starts = starts[len(starts)-53:]
	}
	return weeks, starts
}

// Level maps a daily contribution count to a 0-4 heatmap intensity.
func Level(count int) int {
	switch {
	case count <= 0:
		return 0
	case count <= 3:
		return 1
	case count <= 7:
		return 2
	case count <= 12:
		return 3
	default:
		return 4
	}
}

func (d *Dashboard) sortedDays() []ContributionDay {
	days := make([]ContributionDay, len(d.Contributions))
	copy(days, d.Contributions)
	sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })
	return days
}

func (d *Dashboard) lastDay() time.Time {
	var last time.Time
	for _, c := range d.Contributions {
		if c.Date.After(last) {
			last = c.Date
		}
	}
	if last.IsZero() {
		return time.Now()
	}
	return last
}

// BuildCIHistory aggregates runs into the trailing 14 calendar days.
func BuildCIHistory(runs []WorkflowRun, now time.Time) []CIDay {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -13)
	days := make([]CIDay, 14)
	durs := make([][]time.Duration, 14)
	for i := range days {
		days[i].Date = start.AddDate(0, 0, i)
	}
	for _, r := range runs {
		if r.Status != "completed" || r.StartedAt.Before(start) {
			continue
		}
		idx := int(r.StartedAt.Sub(start).Hours() / 24)
		if idx < 0 || idx >= 14 {
			continue
		}
		if r.State() == StateSuccess {
			days[idx].Passed++
		} else {
			days[idx].Failed++
		}
		durs[idx] = append(durs[idx], r.Duration)
	}
	for i, ds := range durs {
		if len(ds) == 0 {
			continue
		}
		sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
		days[i].P50 = ds[len(ds)/2]
	}
	return days
}

// MedianDuration returns the P50 duration across all CI history days with runs.
func MedianDuration(days []CIDay) time.Duration {
	var ds []time.Duration
	for _, d := range days {
		if d.P50 > 0 {
			ds = append(ds, d.P50)
		}
	}
	if len(ds) == 0 {
		return 0
	}
	sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
	return ds[len(ds)/2]
}
