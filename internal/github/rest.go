package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

type restRun struct {
	ID           int64     `json:"id"`
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
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return ErrStatsPending
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	if s, ok := v.(*string); ok {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return err
		}
		*s = string(b)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(v)
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
			ID:         r.ID,
			Name:       r.Name,
			Repo:       repo,
			Branch:     r.HeadBranch,
			Event:      r.Event,
			SHA:        shortSHA(r.HeadSHA),
			Status:     r.Status,
			Conclusion: r.Conclusion,
			StartedAt:  r.RunStartedAt,
			Duration:   end.Sub(r.RunStartedAt).Round(time.Second),
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

// FailureExcerpt strips log timestamps and returns n lines ending at the first error marker.
func FailureExcerpt(raw string, n int) []string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		if len(l) > 29 && l[4] == '-' && l[10] == 'T' {
			if sp := strings.IndexByte(l, ' '); sp > 0 && sp < 35 {
				l = l[sp+1:]
			}
		}
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "##[group]") || strings.HasPrefix(l, "##[endgroup]") {
			continue
		}
		lines = append(lines, l)
	}
	// Prefer GitHub's explicit ##[error] annotation; fall back to common failure markers.
	end := len(lines)
	found := false
	for _, markers := range [][]string{{"##[error]"}, {"panic:", "--- fail", "error:"}} {
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
	start := end - n
	if start < 0 {
		start = 0
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
		})
	}
	return alerts, nil
}

// FetchNotifications lists unread notifications.
func (c *Client) FetchNotifications(ctx context.Context) ([]Notification, error) {
	var out []restNotification
	if err := c.get(ctx, "/notifications?all=false&per_page=15", &out); err != nil {
		return nil, err
	}
	ns := make([]Notification, 0, len(out))
	for _, n := range out {
		ns = append(ns, Notification{ID: n.ID, Reason: n.Reason, Title: n.Subject.Title, Type: n.Subject.Type, Repo: n.Repository.FullName, UpdatedAt: n.UpdatedAt})
	}
	return ns, nil
}

// FetchCompare returns how far head is ahead/behind base.
func (c *Client) FetchCompare(ctx context.Context, repo, base, head string) (ahead, behind int, err error) {
	var out struct {
		AheadBy  int `json:"ahead_by"`
		BehindBy int `json:"behind_by"`
	}
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repo, base, head), &out); err != nil {
		return 0, 0, err
	}
	return out.AheadBy, out.BehindBy, nil
}

// Fetch loads the full live dashboard: one GraphQL batch plus concurrent REST enrichment.
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
	ciRepos := d.Repos
	if len(ciRepos) > 5 {
		ciRepos = ciRepos[:5]
	}
	for _, r := range ciRepos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			runs, err := c.FetchRuns(ctx, repo, 20)
			if err != nil {
				return
			}
			mu.Lock()
			allRuns = append(allRuns, runs...)
			mu.Unlock()
		}(r.NameWithOwner)
	}
	alertRepos := d.Repos
	if len(alertRepos) > 4 {
		alertRepos = alertRepos[:4]
	}
	for _, r := range alertRepos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			alerts, err := c.FetchDependabot(ctx, repo)
			if err != nil {
				return
			}
			mu.Lock()
			d.Alerts = append(d.Alerts, alerts...)
			mu.Unlock()
		}(r.NameWithOwner)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ns, err := c.FetchNotifications(ctx); err == nil {
			mu.Lock()
			d.Notifications = ns
			mu.Unlock()
		}
	}()
	branchFor := map[string]string{}
	for _, p := range d.MyPRs {
		if _, ok := branchFor[p.Repo]; !ok {
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

	d.CIHistory = BuildCIHistory(allRuns, time.Now())
	sort.Slice(allRuns, func(i, j int) bool { return allRuns[i].StartedAt.After(allRuns[j].StartedAt) })
	if len(allRuns) > 12 {
		allRuns = allRuns[:12]
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
					if lines, err := c.FetchJobLogTail(ctx, r.Repo, j.ID, 12); err == nil {
						r.FailureLog = lines
					}
					return
				}
			}
		}(&d.Runs[i], wantLog)
	}
	wg.Wait()
	return d, nil
}

// ActionResult reports the outcome of a gh CLI mutation.
type ActionResult struct {
	Command string
	Output  string
	Err     error
}

// RunGH executes a gh subcommand and captures its combined output.
func RunGH(args ...string) ActionResult {
	if _, err := exec.LookPath("gh"); err != nil {
		return ActionResult{Command: "gh " + strings.Join(args, " "), Err: fmt.Errorf("GitHub CLI (gh) not found — install it from https://cli.github.com and run `gh auth login`")}
	}
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	return ActionResult{Command: "gh " + strings.Join(args, " "), Output: strings.TrimSpace(string(out)), Err: err}
}

// ApproveArgs builds `gh pr review --approve` arguments.
func ApproveArgs(repo string, number int) []string {
	return []string{"pr", "review", fmt.Sprint(number), "--repo", repo, "--approve"}
}

// MergeArgs builds `gh pr merge --squash` arguments.
func MergeArgs(repo string, number int) []string {
	return []string{"pr", "merge", fmt.Sprint(number), "--repo", repo, "--squash", "--delete-branch"}
}

// CheckoutArgs builds `gh pr checkout` arguments.
func CheckoutArgs(repo string, number int) []string {
	return []string{"pr", "checkout", fmt.Sprint(number), "--repo", repo}
}

// RerunArgs builds `gh run rerun --failed` arguments.
func RerunArgs(repo string, runID int64) []string {
	return []string{"run", "rerun", fmt.Sprint(runID), "--repo", repo, "--failed"}
}

// WorkflowDispatchArgs builds `gh workflow run` arguments for a workflow_dispatch trigger.
func WorkflowDispatchArgs(repo, workflow string) []string {
	return []string{"workflow", "run", workflow, "--repo", repo}
}

// CloseIssueArgs builds `gh issue close` arguments.
func CloseIssueArgs(repo string, number int) []string {
	return []string{"issue", "close", fmt.Sprint(number), "--repo", repo}
}

// CommentIssueArgs opens the issue comment composer in the browser.
func CommentIssueArgs(repo string, number int) []string {
	return []string{"issue", "comment", fmt.Sprint(number), "--repo", repo, "--web"}
}

// MarkNotificationsReadArgs marks every notification as read via the REST API.
func MarkNotificationsReadArgs() []string {
	return []string{"api", "--method", "PUT", "/notifications", "-F", "read=true"}
}

// CloneCommand returns the shell command to clone a repository.
func CloneCommand(repo string) string {
	return "gh repo clone " + repo
}

// OpenURL opens a URL in the default browser.
func OpenURL(url string) error {
	for _, opener := range []string{"open", "xdg-open", "wslview"} {
		if _, err := exec.LookPath(opener); err == nil {
			return exec.Command(opener, url).Start()
		}
	}
	return fmt.Errorf("no browser opener found")
}

// CopyToClipboard copies text using pbcopy, wl-copy, xclip or clip.exe, whichever exists.
func CopyToClipboard(text string) error {
	candidates := [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"clip.exe"}}
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
