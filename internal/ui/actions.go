package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	gh "github.com/Tukesh1/ghx/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

// actionMsg reports the outcome of a mutation; after applies the change to the local model so the
// UI updates immediately, before the next refresh confirms it.
type actionMsg struct {
	label string
	err   error
	after func(*Model)
}

// mutate runs an API mutation. In demo mode the change is simulated locally instead.
func (m Model) mutate(label string, call func(context.Context, *gh.Client) error, after func(*Model)) (tea.Model, tea.Cmd) {
	if m.demo() {
		if after != nil {
			after(&m)
		}
		m.notify(statusOK, label)
		return m, nil
	}
	m.busy++
	m.setStatus(statusInfo, "ghx: "+strings.ToLower(label[:1])+label[1:]+"…")
	c := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return actionMsg{label: label, err: call(ctx, c), after: after}
	}
}

func (m Model) handleAction(msg actionMsg) (tea.Model, tea.Cmd) {
	m.busy = max(m.busy-1, 0)
	if msg.err != nil {
		m.notify(statusErr, msg.label+" failed: "+firstLine(msg.err.Error()))
		return m, nil
	}
	if msg.after != nil {
		msg.after(&m)
	}
	m.notify(statusOK, msg.label)
	// GitHub's search index lags mutations by a few seconds; confirm with a refresh shortly after.
	if m.client != nil {
		m.scheduleFetch(4 * time.Second)
	}
	return m, nil
}

// confirm arms a confirm-twice action; it returns true when this press is the confirmation.
func (m *Model) confirm(key, id, prompt string) bool {
	token := key + ":" + id
	if m.pending == token {
		m.pending = ""
		return true
	}
	m.pending = token
	m.setStatus(statusErr, prompt+" — press "+key+" again to confirm, any other key cancels")
	return false
}

func (m Model) approve() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindPR {
		m.setStatus(statusInfo, "ghx: select a pull request to approve (press "+tabKey(tabPRs)+")")
		return m, nil
	}
	pr, ok := m.selectedPR()
	if !ok {
		return m, nil
	}
	if pr.Author == m.data.User.Login {
		m.notify(statusErr, fmt.Sprintf("You can't approve your own PR #%d", pr.Number))
		return m, nil
	}
	return m.mutate(fmt.Sprintf("Approved PR #%d", pr.Number),
		func(ctx context.Context, c *gh.Client) error { return c.Approve(ctx, pr.Repo, pr.Number) },
		func(mm *Model) {
			mm.mutatePR(pr, func(p *gh.PullRequest) {
				p.ReviewDecision = gh.ReviewApproved
				p.NeedsMyReview = false
			})
		})
}

func (m Model) merge() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindPR {
		m.setStatus(statusInfo, "ghx: select a pull request to merge (press "+tabKey(tabPRs)+")")
		return m, nil
	}
	pr, ok := m.selectedPR()
	if !ok {
		return m, nil
	}
	warn := ""
	switch {
	case pr.IsDraft:
		m.notify(statusErr, fmt.Sprintf("PR #%d is a draft", pr.Number))
		return m, nil
	case pr.Conflicting:
		m.notify(statusErr, fmt.Sprintf("PR #%d has merge conflicts", pr.Number))
		return m, nil
	case !pr.MergeReady():
		warn = " (not merge-ready!)"
	}
	verb := strings.ToLower(pr.MergeLabel())
	if !m.confirm("m", pr.Key(), fmt.Sprintf("%s #%d%s", verb, pr.Number, warn)) {
		return m, nil
	}
	own := pr.Author == m.data.User.Login
	return m.mutate(fmt.Sprintf("Merged PR #%d into %s", pr.Number, pr.BaseRef),
		func(ctx context.Context, c *gh.Client) error { return c.Merge(ctx, pr, own) },
		func(mm *Model) {
			mm.mutatePR(pr, func(p *gh.PullRequest) { p.Merged = true })
			mm.removeMerged()
		})
}

// checkout runs `gh pr checkout` in the foreground, but only inside a clone of the PR's repository:
// elsewhere it would fetch the branch into an unrelated repo, so the command is copied instead.
func (m Model) checkout() (tea.Model, tea.Cmd) {
	pr, ok := m.selectedPR()
	if !ok {
		return m, nil
	}
	args := gh.CheckoutCommand(pr.Repo, pr.Number)
	line := strings.Join(args, " ")
	if m.demo() {
		m.notify(statusOK, "demo · would run "+line)
		return m, nil
	}
	dir, _ := os.Getwd()
	inClone := false
	for _, r := range gh.LocalRepos(dir) {
		inClone = inClone || strings.EqualFold(r, pr.Repo)
	}
	if !inClone || !gh.HasGH() {
		reason := "not in a clone of " + pr.Repo
		if !gh.HasGH() {
			reason = "gh CLI not installed"
		}
		if err := gh.CopyToClipboard(line); err == nil {
			m.notify(statusInfo, reason+" · copied: "+line)
		} else {
			m.notify(statusInfo, reason+" · run: "+line)
		}
		return m, nil
	}
	label := fmt.Sprintf("Checked out PR #%d", pr.Number)
	return m, tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		return actionMsg{label: label, err: err}
	})
}

func (m Model) copyClone() (tea.Model, tea.Cmd) {
	var repo string
	if m.activeKind() == kindPinned {
		r, ok := m.selectedPinned()
		if !ok {
			return m, nil
		}
		repo = r.NameWithOwner
	} else {
		r, ok := m.selectedRepo()
		if !ok {
			return m, nil
		}
		repo = r.NameWithOwner
	}
	cmd := gh.CloneCommand(repo)
	if err := gh.CopyToClipboard(cmd); err != nil {
		m.notify(statusInfo, "run: "+cmd)
	} else {
		m.notify(statusOK, "Copied: "+cmd)
	}
	return m, nil
}

// checkoutCloneComment is the context-sensitive c key.
func (m Model) checkoutCloneComment() (tea.Model, tea.Cmd) {
	switch m.activeKind() {
	case kindPR:
		return m.checkout()
	case kindRepo, kindPinned:
		return m.copyClone()
	case kindIssue:
		return m.startComment()
	}
	m.setStatus(statusInfo, "ghx: c checks out PRs, copies clone commands and comments on issues")
	return m, nil
}

func (m Model) rerunOrRefresh() (tea.Model, tea.Cmd) {
	if m.activeKind() == kindRun {
		if run, ok := m.selectedRun(); ok && run.State() == gh.StateFailure {
			return m.mutate(fmt.Sprintf("Re-running failed jobs of %s", run.Name),
				func(ctx context.Context, c *gh.Client) error { return c.RerunFailedJobs(ctx, run.Repo, run.ID) },
				func(mm *Model) {
					for i := range mm.data.Runs {
						if mm.data.Runs[i].ID == run.ID {
							mm.data.Runs[i].Status, mm.data.Runs[i].Conclusion = "queued", ""
							mm.data.Runs[i].StartedAt = time.Now()
						}
					}
				})
		}
	}
	return m.refresh()
}

// refresh reloads the dashboard (and the inspected repository's insights).
func (m Model) refresh() (tea.Model, tea.Cmd) {
	if m.tab == tabRepos && !m.demo() {
		if r, ok := m.selectedRepo(); ok {
			delete(m.analytics, r.NameWithOwner)
			delete(m.analyticsErr, r.NameWithOwner)
		}
	}
	if m.client == nil {
		m.data = gh.MockDashboard()
		m.clampCursors()
		m.notify(statusOK, "Reloaded demo data")
		return m, nil
	}
	if m.loading {
		return m, nil
	}
	return m, m.startFetch("fetching latest GitHub state…")
}

func (m Model) dispatchWorkflow() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindRun {
		m.setStatus(statusInfo, "ghx: w w triggers workflow_dispatch from the Actions view (3)")
		return m, nil
	}
	r, ok := m.selectedRun()
	if !ok {
		return m, nil
	}
	if r.WorkflowID == 0 {
		m.notify(statusErr, "workflow id unknown for "+r.Name)
		return m, nil
	}
	if !m.confirm("w", fmt.Sprint(r.ID), fmt.Sprintf("run %s on %s (%s)", r.Name, r.Branch, shortRepo(r.Repo, m.data.User.Login))) {
		return m, nil
	}
	return m.mutate(fmt.Sprintf("Dispatched %s on %s", r.Name, r.Branch),
		func(ctx context.Context, c *gh.Client) error {
			return c.DispatchWorkflow(ctx, r.Repo, r.WorkflowID, r.Branch)
		}, nil)
}

func (m Model) closeIssue() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindIssue {
		m.setStatus(statusInfo, "ghx: x x closes the selected issue (5)")
		return m, nil
	}
	is, ok := m.selectedIssue()
	if !ok {
		return m, nil
	}
	if !m.confirm("x", fmt.Sprintf("%s#%d", is.Repo, is.Number), fmt.Sprintf("close issue #%d", is.Number)) {
		return m, nil
	}
	return m.mutate(fmt.Sprintf("Closed issue #%d", is.Number),
		func(ctx context.Context, c *gh.Client) error { return c.CloseIssue(ctx, is.Repo, is.Number) },
		func(mm *Model) {
			var keep []gh.Issue
			for _, i := range mm.data.Issues {
				if !(i.Repo == is.Repo && i.Number == is.Number) {
					keep = append(keep, i)
				}
			}
			mm.data.Issues = keep
			mm.clampCursors()
		})
}

func (m Model) markAllRead() (tea.Model, tea.Cmd) {
	if len(m.data.Notifications) == 0 {
		return m, nil
	}
	return m.mutate("Marked all notifications as read",
		func(ctx context.Context, c *gh.Client) error { return c.MarkAllRead(ctx) },
		func(mm *Model) {
			mm.data.Notifications = nil
			mm.notifCursor = 0
		})
}

// openNotification opens the selected inbox thread in the browser and marks it read.
func (m Model) openNotification() (tea.Model, tea.Cmd) {
	if len(m.data.Notifications) == 0 {
		return m, nil
	}
	n := m.data.Notifications[clamp(m.notifCursor, 0, len(m.data.Notifications)-1)]
	if m.demo() {
		m.notify(statusOK, "demo · would open "+n.URL)
	} else if err := gh.OpenURL(n.URL); err != nil {
		m.notify(statusErr, err.Error())
		return m, nil
	}
	drop := func(mm *Model) {
		var keep []gh.Notification
		for _, x := range mm.data.Notifications {
			if x.ID != n.ID {
				keep = append(keep, x)
			}
		}
		mm.data.Notifications = keep
		mm.notifCursor = clamp(mm.notifCursor, 0, len(keep)-1)
	}
	if m.demo() {
		drop(&m)
		return m, nil
	}
	c := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return actionMsg{label: "Opened " + shortRepo(n.Repo, "") + " · marked read", err: c.MarkThreadRead(ctx, n.ID), after: drop}
	}
}

// selectedURL is the github.com page for whatever the cursor is on.
func (m Model) selectedURL() string {
	switch m.activeKind() {
	case kindPR:
		if pr, ok := m.selectedPR(); ok {
			return pr.URL()
		}
	case kindRun:
		if r, ok := m.selectedRun(); ok {
			return r.URL()
		}
	case kindRepo:
		if r, ok := m.selectedRepo(); ok {
			return r.URL()
		}
	case kindIssue:
		if is, ok := m.selectedIssue(); ok {
			return is.URL()
		}
	case kindPinned:
		if r, ok := m.selectedPinned(); ok {
			return r.URL()
		}
		if m.data.Profile != nil {
			return m.data.Profile.URL()
		}
	}
	return ""
}

// openProfile opens the viewer's github.com profile page.
func (m Model) openProfile() (tea.Model, tea.Cmd) {
	return m.openURL(gh.ProfileURL(m.data.User.Login))
}

func (m Model) openInBrowser() (tea.Model, tea.Cmd) {
	if m.showNotifs {
		return m.openNotification()
	}
	return m.openURL(m.selectedURL())
}

func (m Model) openURL(u string) (tea.Model, tea.Cmd) {
	switch {
	case u == "":
		return m, nil
	case m.demo():
		m.notify(statusOK, "demo · would open "+u)
	default:
		if err := gh.OpenURL(u); err != nil {
			m.notify(statusErr, err.Error())
		} else {
			m.notify(statusOK, "Opened "+strings.TrimPrefix(u, gh.WebURL+"/"))
		}
	}
	return m, nil
}

func (m Model) openAlert(idx int) (tea.Model, tea.Cmd) {
	if idx >= len(m.data.Alerts) {
		m.setStatus(statusInfo, "ghx: no Dependabot alert in that slot")
		return m, nil
	}
	a := m.data.Alerts[idx]
	u := a.URL
	if u == "" {
		u = fmt.Sprintf("%s/%s/security/dependabot", gh.WebURL, a.Repo)
	}
	return m.openURL(u)
}

// yank copies the selected item's URL to the clipboard.
func (m Model) yank() (tea.Model, tea.Cmd) {
	u := m.selectedURL()
	if m.showNotifs && len(m.data.Notifications) > 0 {
		u = m.data.Notifications[clamp(m.notifCursor, 0, len(m.data.Notifications)-1)].URL
	}
	if u == "" {
		return m, nil
	}
	if err := gh.CopyToClipboard(u); err != nil {
		m.notify(statusErr, "clipboard unavailable · "+u)
	} else {
		m.notify(statusOK, "Copied "+u)
	}
	return m, nil
}

func (m *Model) mutatePR(target gh.PullRequest, f func(*gh.PullRequest)) {
	for _, list := range []*[]gh.PullRequest{&m.data.ReviewPRs, &m.data.MyPRs} {
		for i := range *list {
			if (*list)[i].Key() == target.Key() {
				f(&(*list)[i])
			}
		}
	}
}

func (m *Model) mutateIssue(target gh.Issue, f func(*gh.Issue)) {
	for i := range m.data.Issues {
		if m.data.Issues[i].Repo == target.Repo && m.data.Issues[i].Number == target.Number {
			f(&m.data.Issues[i])
		}
	}
}

func (m *Model) removeMerged() {
	keep := func(in []gh.PullRequest) []gh.PullRequest {
		var out []gh.PullRequest
		for _, p := range in {
			if !p.Merged {
				out = append(out, p)
			}
		}
		return out
	}
	m.data.ReviewPRs = keep(m.data.ReviewPRs)
	m.data.MyPRs = keep(m.data.MyPRs)
	m.clampCursors()
}
