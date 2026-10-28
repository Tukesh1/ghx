package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type restRun struct {
	ID           int64     `json:"id"`
	WorkflowID   int64     `json:"workflow_id"`
	Path         string    `json:"path"`
	HTMLURL      string    `json:"html_url"`
	RunAttempt   int       `json:"run_attempt"`
	Name         string    `json:"name"`
	Event        string    `json:"event"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	RunStartedAt time.Time `json:"run_started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type restJobs struct {
	Jobs []struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Steps      []struct {
			Name        string     `json:"name"`
			Status      string     `json:"status"`
			Conclusion  string     `json:"conclusion"`
			Number      int        `json:"number"`
			StartedAt   *time.Time `json:"started_at"`
			CompletedAt *time.Time `json:"completed_at"`
		} `json:"steps"`
	} `json:"jobs"`
}

type restAlert struct {
	HTMLURL    string `json:"html_url"`
	Dependency struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
	} `json:"dependency"`
	SecurityAdvisory struct {
		CVEID    string `json:"cve_id"`
		GHSAID   string `json:"ghsa_id"`
		Severity string `json:"severity"`
	} `json:"security_advisory"`
	SecurityVulnerability struct {
		VulnerableVersionRange string `json:"vulnerable_version_range"`
		FirstPatchedVersion    *struct {
			Identifier string `json:"identifier"`
		} `json:"first_patched_version"`
	} `json:"security_vulnerability"`
}

type restNotification struct {
	ID        string    `json:"id"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
	Subject   struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		URL   string `json:"url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// FetchRuns lists recent workflow runs for a repository.
func (c *Client) FetchRuns(ctx context.Context, repo string, perPage int) ([]WorkflowRun, error) {
	var out struct {
		WorkflowRuns []restRun `json:"workflow_runs"`
	}
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/actions/runs?per_page=%d", repo, perPage), &out); err != nil {
		return nil, err
	}
	runs := make([]WorkflowRun, 0, len(out.WorkflowRuns))
	for _, r := range out.WorkflowRuns {
		end := r.UpdatedAt
		if r.Status != "completed" {
			end = time.Now()
		}
		runs = append(runs, WorkflowRun{
			ID:           r.ID,
			WorkflowID:   r.WorkflowID,
			WorkflowPath: r.Path,
			HTMLURL:      r.HTMLURL,
			Attempt:      r.RunAttempt,
			Name:         r.Name,
			Repo:         repo,
			Branch:       r.HeadBranch,
			Event:        r.Event,
			SHA:          shortSHA(r.HeadSHA),
			Status:       r.Status,
			Conclusion:   r.Conclusion,
			StartedAt:    r.RunStartedAt,
			Duration:     max(end.Sub(r.RunStartedAt).Round(time.Second), 0),
		})
	}
	return runs, nil
}

// FetchJobs returns the jobs and steps of a workflow run.
func (c *Client) FetchJobs(ctx context.Context, repo string, runID int64) ([]Job, error) {
	var out restJobs
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/jobs", repo, runID), &out); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(out.Jobs))
	for _, j := range out.Jobs {
		job := Job{ID: j.ID, Name: j.Name, Status: j.Status, Conclusion: j.Conclusion}
		for _, s := range j.Steps {
			st := Step{Number: s.Number, Name: s.Name, Status: s.Status, Conclusion: s.Conclusion}
			if s.StartedAt != nil && s.CompletedAt != nil {
				st.Duration = s.CompletedAt.Sub(*s.StartedAt).Round(time.Second)
			}
			job.Steps = append(job.Steps, st)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// FetchJobLogTail downloads a job log and returns the lines around the first failure.
func (c *Client) FetchJobLogTail(ctx context.Context, repo string, jobID int64, n int) ([]string, error) {
	var raw string
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", repo, jobID), &raw); err != nil {
		return nil, err
	}
	return FailureExcerpt(raw, n), nil
}

var (
	logTimestamp = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z `)
	logANSI      = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
)

// FailureExcerpt cleans a raw Actions log and returns up to n lines ending at the first error,
// starting no earlier than the step that failed. Timestamps, ANSI color codes, ##[group] markers and
// the "with:"/"env:" blocks the runner echoes for every step are dropped so the excerpt shows what
// actually went wrong.
func FailureExcerpt(raw string, n int) []string {
	var lines []string
	var stepStarts []int // index of each step's "Run …" header line
	inEcho := false
	for _, l := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		l = logANSI.ReplaceAllString(logTimestamp.ReplaceAllString(l, ""), "")
		trimmed := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "##[group]Run "):
			stepStarts = append(stepStarts, len(lines))
			lines = append(lines, "▶ "+strings.TrimPrefix(l, "##[group]"))
			inEcho = false
			continue
		case trimmed == "", strings.HasPrefix(l, "##[group]"), strings.HasPrefix(l, "##[endgroup]"):
			continue
		case trimmed == "with:" || trimmed == "env:":
			inEcho = true
			continue
		case inEcho && strings.HasPrefix(l, "  "):
			continue
		}
		inEcho = false
		lines = append(lines, l)
	}
	// Prefer GitHub's explicit ##[error] annotation; fall back to common failure markers.
	end := len(lines)
	found := false
	for _, markers := range [][]string{{"##[error]"}, {"panic:", "--- fail", "error:", "error[", "failed"}} {
		for i, l := range lines {
			low := strings.ToLower(l)
			for _, mk := range markers {
				if strings.Contains(low, mk) {
					end, found = i+1, true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	start := max(end-n, 0)
	for _, s := range stepStarts {
		if s < end && s > start {
			start = s
		}
	}
	out := make([]string, 0, end-start)
	for _, l := range lines[start:end] {
		out = append(out, strings.TrimPrefix(l, "##[error]"))
	}
	return out
}

// FetchDependabot lists open Dependabot alerts that have a patched version.
func (c *Client) FetchDependabot(ctx context.Context, repo string) ([]DependabotAlert, error) {
	var out []restAlert
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/dependabot/alerts?state=open&per_page=5", repo), &out); err != nil {
		return nil, err
	}
	var alerts []DependabotAlert
	for _, a := range out {
		if a.SecurityVulnerability.FirstPatchedVersion == nil {
			continue
		}
		cve := a.SecurityAdvisory.CVEID
		if cve == "" {
			cve = a.SecurityAdvisory.GHSAID
		}
		alerts = append(alerts, DependabotAlert{
			Repo:           repo,
			Package:        a.Dependency.Package.Name,
			Ecosystem:      a.Dependency.Package.Ecosystem,
			CVE:            cve,
			Severity:       a.SecurityAdvisory.Severity,
			CurrentVersion: a.SecurityVulnerability.VulnerableVersionRange,
			PatchedVersion: a.SecurityVulnerability.FirstPatchedVersion.Identifier,
			URL:            a.HTMLURL,
		})
	}
	return alerts, nil
}

// FetchNotifications lists unread notifications.
func (c *Client) FetchNotifications(ctx context.Context) ([]Notification, error) {
	var out []restNotification
	if err := c.get(ctx, "/notifications?all=false&per_page=30", &out); err != nil {
		return nil, err
	}
	ns := make([]Notification, 0, len(out))
	for _, n := range out {
		ns = append(ns, Notification{
			ID: n.ID, Reason: n.Reason, Title: n.Subject.Title, Type: n.Subject.Type, Repo: n.Repository.FullName,
			URL: NotificationURL(n.Subject.URL, n.Subject.Type, n.Repository.FullName), UpdatedAt: n.UpdatedAt,
		})
	}
	return ns, nil
}

var apiSubject = regexp.MustCompile(`^https://api\.github\.com/repos/([^/]+/[^/]+)/(pulls|issues|commits|discussions)/([^/?#]+)$`)

// NotificationURL turns a notification subject's API URL into the github.com page a person opens.
func NotificationURL(apiURL, typ, repo string) string {
	if m := apiSubject.FindStringSubmatch(apiURL); m != nil {
		kind := map[string]string{"pulls": "pull", "issues": "issues", "commits": "commit", "discussions": "discussions"}[m[2]]
		return fmt.Sprintf("%s/%s/%s/%s", WebURL, m[1], kind, m[3])
	}
	base := WebURL + "/" + repo
	switch typ {
	case "Release":
		return base + "/releases"
	case "CheckSuite", "WorkflowRun":
		return base + "/actions"
	case "Discussion":
		return base + "/discussions"
	case "RepositoryVulnerabilityAlert", "RepositoryDependabotAlertsThread":
		return base + "/security/dependabot"
	}
	return base
}

// FetchCompare returns how far head is ahead/behind base.
func (c *Client) FetchCompare(ctx context.Context, repo, base, head string) (ahead, behind int, err error) {
	var out struct {
		AheadBy  int `json:"ahead_by"`
		BehindBy int `json:"behind_by"`
	}
	path := fmt.Sprintf("/repos/%s/compare/%s...%s?per_page=1", repo, url.PathEscape(base), url.PathEscape(head))
	if err := c.get(ctx, path, &out); err != nil {
		return 0, 0, err
	}
	return out.AheadBy, out.BehindBy, nil
}

// Fetch loads the full live dashboard: concurrent GraphQL sections, then concurrent REST enrichment
// (workflow runs, Dependabot alerts, notifications, branch sync) and finally job details and failure logs.
func (c *Client) Fetch(ctx context.Context, org string) (*Dashboard, error) {
	d, err := c.FetchGraphQL(ctx, org)
	if err != nil {
		return nil, err
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		allRuns []WorkflowRun
	)
	warn := func(section string, err error) {
		if err == nil || IsNotFound(err) || ctx.Err() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		msg := section + ": " + err.Error()
		for _, w := range d.Warnings {
			if w == msg {
				return
			}
		}
		d.Warnings = append(d.Warnings, msg)
	}
	var active []string
	for _, r := range d.Repos {
		if !r.IsArchived {
			active = append(active, r.NameWithOwner)
		}
	}
	for i, repo := range active {
		if i >= 6 {
			break
		}
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			runs, err := c.FetchRuns(ctx, repo, 20)
			warn("workflow runs", err)
			mu.Lock()
			allRuns = append(allRuns, runs...)
			mu.Unlock()
		}(repo)
		if i >= 4 {
			continue
		}
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			// Dependabot alerts need admin or security-manager access; 403/404 just means "not yours".
			alerts, err := c.FetchDependabot(ctx, repo)
			if err != nil {
				return
			}
			mu.Lock()
			d.Alerts = append(d.Alerts, alerts...)
			mu.Unlock()
		}(repo)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ns, err := c.FetchNotifications(ctx)
		warn("notifications", err)
		mu.Lock()
		d.Notifications = ns
		mu.Unlock()
	}()
	branchFor := map[string]string{}
	for _, p := range d.MyPRs {
		if _, ok := branchFor[p.Repo]; !ok && !p.CrossRepo {
			branchFor[p.Repo] = p.HeadRef
		}
	}
	for i := range d.Repos {
		head, ok := branchFor[d.Repos[i].NameWithOwner]
		if !ok || d.Repos[i].DefaultBranch == "" {
			continue
		}
		wg.Add(1)
		go func(r *Repo, head string) {
			defer wg.Done()
			ahead, behind, err := c.FetchCompare(ctx, r.NameWithOwner, r.DefaultBranch, head)
			if err != nil {
				return
			}
			mu.Lock()
			r.SyncBranch, r.Ahead, r.Behind = head, ahead, behind
			mu.Unlock()
		}(&d.Repos[i], head)
	}
	wg.Wait()

	sort.Slice(d.Alerts, func(i, j int) bool { return severityRank(d.Alerts[i].Severity) < severityRank(d.Alerts[j].Severity) })
	d.CIHistory = BuildCIHistory(allRuns, time.Now())
	sort.Slice(allRuns, func(i, j int) bool { return allRuns[i].StartedAt.After(allRuns[j].StartedAt) })
	if len(allRuns) > 15 {
		allRuns = allRuns[:15]
	}
	d.Runs = allRuns

	// Job logs are large downloads, so only the 3 most recent failing runs get an excerpt.
	logBudget := 3
	for i := range d.Runs {
		wantLog := d.Runs[i].State() == StateFailure && logBudget > 0
		if wantLog {
			logBudget--
		}
		wg.Add(1)
		go func(r *WorkflowRun, wantLog bool) {
			defer wg.Done()
			jobs, err := c.FetchJobs(ctx, r.Repo, r.ID)
			if err != nil {
				return
			}
			r.Jobs = jobs
			if !wantLog {
				return
			}
			for _, j := range jobs {
				if j.Conclusion == "failure" {
					if lines, err := c.FetchJobLogTail(ctx, r.Repo, j.ID, 14); err == nil {
						r.FailureLog = lines
					}
					return
				}
			}
		}(&d.Runs[i], wantLog)
	}
	wg.Wait()
	sort.Strings(d.Warnings)
	return d, nil
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium", "moderate":
		return 2
	default:
		return 3
	}
}

// ---- Mutations ------------------------------------------------------------------------------
// Mutations call the REST API directly with the same token, so they work without the gh CLI.

// Approve submits an approving review.
func (c *Client) Approve(ctx context.Context, repo string, number int) error {
	return c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number), map[string]string{"event": "APPROVE"}, nil)
}

// Merge merges a PR with its repository's preferred method and, when deleteBranch is set and the
// head branch lives in the same repository, deletes the branch afterwards.
func (c *Client) Merge(ctx context.Context, pr PullRequest, deleteBranch bool) error {
	method := pr.MergeMethod
	if method == "" {
		method = MergeSquash
	}
	body := map[string]string{"merge_method": method}
	if method == MergeSquash {
		body["commit_title"] = fmt.Sprintf("%s (#%d)", pr.Title, pr.Number)
	}
	if err := c.rest(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/pulls/%d/merge", pr.Repo, pr.Number), body, nil); err != nil {
		return err
	}
	if deleteBranch && !pr.CrossRepo && pr.HeadRef != "" {
		// 422 means the branch is already gone (e.g. "automatically delete head branches" is on).
		_ = c.rest(ctx, http.MethodDelete, fmt.Sprintf("/repos/%s/git/refs/heads/%s", pr.Repo, url.PathEscape(pr.HeadRef)), nil, nil)
	}
	return nil
}

// CloseIssue closes an issue as completed.
func (c *Client) CloseIssue(ctx context.Context, repo string, number int) error {
	return c.rest(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", repo, number), map[string]string{"state": "closed", "state_reason": "completed"}, nil)
}

// Comment posts a comment on an issue or pull request.
func (c *Client) Comment(ctx context.Context, repo string, number int, body string) error {
	return c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number), map[string]string{"body": body}, nil)
}

// RerunFailedJobs re-runs only the failed jobs of a workflow run.
func (c *Client) RerunFailedJobs(ctx context.Context, repo string, runID int64) error {
	return c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/actions/runs/%d/rerun-failed-jobs", repo, runID), nil, nil)
}

// DispatchWorkflow triggers a workflow_dispatch event for a workflow on ref.
func (c *Client) DispatchWorkflow(ctx context.Context, repo string, workflowID int64, ref string) error {
	return c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/actions/workflows/%d/dispatches", repo, workflowID), map[string]string{"ref": ref}, nil)
}

// MarkThreadRead marks one notification thread as read.
func (c *Client) MarkThreadRead(ctx context.Context, id string) error {
	return c.rest(ctx, http.MethodPatch, "/notifications/threads/"+url.PathEscape(id), nil, nil)
}

// MarkAllRead marks every notification as read.
func (c *Client) MarkAllRead(ctx context.Context) error {
	return c.rest(ctx, http.MethodPut, "/notifications", map[string]bool{"read": true}, nil)
}

// ---- Local helpers --------------------------------------------------------------------------

var remoteRepo = regexp.MustCompile(`github\.com[:/]([^/\s]+/[^/\s]+?)(?:\.git)?/?(?:\s|$)`)

// LocalRepos lists the GitHub repositories (owner/name) the git checkout in dir points at.
func LocalRepos(dir string) []string {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var repos []string
	for _, line := range strings.Split(string(out), "\n") {
		if m := remoteRepo.FindStringSubmatch(line); m != nil && !seen[strings.ToLower(m[1])] {
			seen[strings.ToLower(m[1])] = true
			repos = append(repos, m[1])
		}
	}
	return repos
}

// CheckoutCommand is the gh command that checks a PR out locally.
func CheckoutCommand(repo string, number int) []string {
	return []string{"gh", "pr", "checkout", fmt.Sprint(number), "--repo", repo}
}

// CloneCommand returns the shell command to clone a repository.
func CloneCommand(repo string) string {
	return "gh repo clone " + repo
}

// HasGH reports whether the GitHub CLI is installed.
func HasGH() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// OpenURL opens a URL in the default browser.
func OpenURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		for _, opener := range []string{"xdg-open", "wslview", "sensible-browser"} {
			if _, err := exec.LookPath(opener); err == nil {
				cmd = exec.Command(opener, u)
				break
			}
		}
	}
	if cmd == nil {
		return fmt.Errorf("no browser opener found · %s", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// CopyToClipboard copies text using pbcopy, wl-copy, xclip, xsel or clip.exe, whichever exists.
func CopyToClipboard(text string) error {
	candidates := [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"clip.exe"}}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return fmt.Errorf("no clipboard tool found")
}
