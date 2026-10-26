package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minWidth   = 80
	minHeight  = 24
	toastTTL   = 3 * time.Second
	pageMargin = 1
	// navRows are the fixed rows above the KPI strip: header, tab bar and tab rule.
	navRows = 3
)

// Tab indices, matching the 1-5 hotkeys.
const (
	tabOverview = iota
	tabPRs
	tabActions
	tabRepos
	tabIssues
)

// Data kinds; each dedicated tab shows one kind and the Command Center focuses one at a time.
const (
	kindPR = iota
	kindRun
	kindRepo
	kindIssue
)

var (
	tabNames      = []string{"Command Center", "Pull Requests", "Actions CI/CD", "Repositories", "Issues & Bugs"}
	shortTabNames = []string{"Overview", "PRs", "Actions", "Repos", "Issues"}
	// pillCount is the number of filter chips per dedicated tab, cycled with f.
	pillCount = [4]int{4, 4, 4, 4}
)

// spin returns the current loading-spinner frame for the active glyph set.
func (m Model) spin() string {
	frames := m.p.ic.Spinner
	return frames[m.frame%len(frames)]
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusOK
	statusErr
)

type dataMsg struct {
	d   *gh.Dashboard
	err error
}

type actionMsg struct {
	label   string
	res     gh.ActionResult
	refresh bool
	after   func(*Model)
}

type tickMsg time.Time

type analyticsMsg struct {
	repo    string
	a       *gh.RepoAnalytics
	err     error
	retried bool
}

func analyticsCmd(c *gh.Client, repo string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := c.FetchRepoAnalytics(ctx, repo)
		return analyticsMsg{repo: repo, a: a, err: err}
	}
}

// ensureAnalytics starts loading insight data for the repo under the cursor when the Repos tab shows it.
func (m *Model) ensureAnalytics() tea.Cmd {
	if m.tab != tabRepos || m.demo() || m.client == nil {
		return nil
	}
	r, ok := m.selectedRepo()
	if !ok {
		return nil
	}
	name := r.NameWithOwner
	if m.analytics[name] != nil || m.analyticsLoading[name] || m.analyticsErr[name] != "" {
		return nil
	}
	m.analyticsLoading[name] = true
	return analyticsCmd(m.client, name)
}

// Model is the root Bubbletea model for ghx.
type Model struct {
	cfg    *config.Config
	client *gh.Client
	data   *gh.Dashboard
	p      palette

	width, height int
	tab           int
	focus         int
	cursor        [4]int
	pill          [4]int
	quick         int
	notifCursor   int

	search    textinput.Model
	searching bool
	filter    string

	showHelp   bool
	showNotifs bool
	insights   bool

	// Per-repo analytics, loaded lazily when a repo is inspected (maps are shared across model copies).
	analytics        map[string]*gh.RepoAnalytics
	analyticsLoading map[string]bool
	analyticsErr     map[string]string

	loading   bool
	frame     int
	status    string
	kind      statusKind
	toast     string
	toastKind statusKind
	toastAt   time.Time
	pending   string
}

// New builds the root model. client may be nil, in which case demo data is used.
func New(cfg *config.Config, client *gh.Client, demoNote string) Model {
	ti := textinput.New()
	ti.Placeholder = "Filter PRs, repos, branches, workflows, issues…"
	ti.Prompt = ""
	ti.CharLimit = 80
	m := Model{cfg: cfg, client: client, search: ti,
		analytics: map[string]*gh.RepoAnalytics{}, analyticsLoading: map[string]bool{}, analyticsErr: map[string]string{}}
	m.applyTheme()
	if client == nil {
		m.data = gh.MockDashboard()
		m.status = demoNote
		if m.status == "" && cfg.FontHint != "" {
			m.status = "ghx: " + cfg.FontHint
		}
	} else {
		m.loading = true
		m.status = "fetching GitHub dashboard…"
		if cached, err := gh.LoadCache(cfg.Org); err == nil {
			m.data = cached
			m.status = "cached snapshot from " + ago(cached.FetchedAt) + " ago · refreshing…"
		}
	}
	return m
}

func (m *Model) applyTheme() {
	m.p = newPalette(m.cfg.Theme(), iconSet(m.cfg.ASCII))
	t := m.p
	m.search.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary))
	m.search.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted))
	m.search.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(t.BorderActive))
}

// Init starts the first fetch (live mode) and the clock tick.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(time.Second)}
	if m.client != nil {
		cmds = append(cmds, fetchCmd(m.client, m.cfg.Org), tick(120*time.Millisecond))
	}
	return tea.Batch(cmds...)
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchCmd(c *gh.Client, org string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		d, err := c.Fetch(ctx, org)
		return dataMsg{d: d, err: err}
	}
}

func ghCmd(label string, args []string, refresh bool) tea.Cmd {
	return func() tea.Msg {
		return actionMsg{label: label, res: gh.RunGH(args...), refresh: refresh}
	}
}

// Update handles input, data and timer messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.frame++
		if m.loading {
			return m, tick(120 * time.Millisecond)
		}
		if m.client != nil && m.data != nil && !m.data.Demo && time.Since(m.data.FetchedAt) > 5*time.Minute {
			m.loading = true
			m.setStatus(statusInfo, "auto-refreshing…")
			return m, tea.Batch(fetchCmd(m.client, m.cfg.Org), tick(120*time.Millisecond))
		}
		return m, tick(time.Second)

	case dataMsg:
		m.loading = false
		if msg.err != nil {
			if m.data == nil {
				m.data = gh.MockDashboard()
				m.setStatus(statusErr, "live fetch failed ("+msg.err.Error()+") · showing demo data")
			} else {
				m.setStatus(statusErr, "refresh failed: "+msg.err.Error())
			}
			return m, tick(time.Second)
		}
		m.data = msg.d
		m.clampCursors()
		_ = gh.SaveCache(m.data, m.cfg.Org)
		m.setStatus(statusInfo, "")
		return m, tick(time.Second)

	case actionMsg:
		if msg.res.Err != nil {
			out := msg.res.Output
			if out == "" {
				out = msg.res.Err.Error()
			}
			m.notify(statusErr, msg.label+" failed: "+firstLine(out))
			return m, nil
		}
		if msg.after != nil {
			msg.after(&m)
		}
		m.notify(statusOK, msg.label)
		if msg.refresh && m.client != nil && !m.loading {
			m.loading = true
			return m, tea.Batch(fetchCmd(m.client, m.cfg.Org), tick(120*time.Millisecond))
		}
		return m, nil

	case analyticsMsg:
		delete(m.analyticsLoading, msg.repo)
		if msg.err != nil {
			m.analyticsErr[msg.repo] = msg.err.Error()
			return m, nil
		}
		m.analytics[msg.repo] = msg.a
		// GitHub answers 202 while it computes statistics; retry once shortly after.
		if msg.a.StatsPending && !msg.retried && m.client != nil {
			repo := msg.repo
			client := m.client
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				a, err := client.FetchRepoAnalytics(ctx, repo)
				return analyticsMsg{repo: repo, a: a, err: err, retried: true}
			})
		}
		return m, nil

	case tea.MouseMsg:
		next, cmd := m.handleMouse(msg)
		mm := next.(Model)
		return mm, tea.Batch(cmd, mm.ensureAnalytics())

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		mm := next.(Model)
		return mm, tea.Batch(cmd, mm.ensureAnalytics())
	}
	return m, nil
}

func (m *Model) setStatus(k statusKind, s string) {
	m.kind, m.status = k, s
}

// notify sets the status line and pops a toast for action outcomes.
func (m *Model) notify(k statusKind, s string) {
	m.setStatus(k, "ghx: "+s)
	m.toast, m.toastKind, m.toastAt = s, k, time.Now()
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.search.Blur()
			m.search.SetValue("")
			m.filter = ""
			m.cursor = [4]int{}
			return m, nil
		case "enter", "down", "up":
			m.searching = false
			m.search.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		if v := m.search.Value(); v != m.filter {
			m.filter = v
			m.cursor = [4]int{}
		}
		return m, cmd
	}

	if m.showHelp {
		if key == "?" || key == "esc" || key == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	// Confirm-twice actions: any key other than the pending one cancels.
	if m.pending != "" && !strings.HasPrefix(m.pending, key+":") {
		m.pending = ""
		m.setStatus(statusInfo, "cancelled")
	}

	if m.showNotifs {
		switch key {
		case "n", "esc":
			m.showNotifs = false
			return m, nil
		case "j", "down":
			m.move(1)
			return m, nil
		case "k", "up":
			m.move(-1)
			return m, nil
		case "m":
			return m.markAllRead()
		case "q":
			return m, tea.Quit
		}
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5":
		m.tab = int(key[0] - '1')
		m.insights = false
	case "backspace":
		m.insights = false
	case "tab", "l", "right":
		if m.tab == tabOverview {
			m.focus = (m.focus + 1) % 4
		} else if key == "tab" {
			m.tab = m.tab%4 + 1
		}
	case "shift+tab", "h", "left":
		if m.tab == tabOverview {
			m.focus = (m.focus + 3) % 4
		} else if key == "shift+tab" {
			m.tab = (m.tab+2)%4 + 1
		}
	case "/":
		m.searching = true
		m.search.SetValue(m.filter)
		m.search.CursorEnd()
		return m, m.search.Focus()
	case "esc":
		if m.insights {
			m.insights = false
			return m, nil
		}
		m.filter = ""
		m.search.SetValue("")
		m.cursor = [4]int{}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.cursor[m.activeKind()] = 0
	case "G", "end":
		m.cursor[m.activeKind()] = 1 << 30
	case "enter":
		switch m.tab {
		case tabOverview:
			m.openFocused()
		case tabRepos:
			m.insights = !m.insights
		}
	case "f":
		if m.tab == tabOverview {
			if m.focus == kindPR {
				m.quick = (m.quick + 1) % 3
				m.cursor[kindPR] = 0
			}
		} else {
			k := m.activeKind()
			m.pill[k] = (m.pill[k] + 1) % pillCount[k]
			m.cursor[k] = 0
		}
	case "t":
		t := m.cfg.CycleTheme()
		m.applyTheme()
		m.setStatus(statusInfo, "ghx: theme → "+t.Name)
	case "i":
		m.cfg.ASCII = !m.cfg.ASCII
		m.applyTheme()
		m.setStatus(statusInfo, "ghx: glyphs → "+m.cfg.FontMode())
	case "n":
		m.showNotifs = !m.showNotifs
		m.notifCursor = 0
	case "?":
		m.showHelp = true
	case "a":
		return m.approve()
	case "m":
		return m.merge()
	case "c":
		return m.checkoutCloneComment()
	case "r":
		return m.rerunOrRefresh()
	case "w":
		return m.dispatchWorkflow()
	case "x":
		return m.closeIssue()
	case "o":
		return m.openInBrowser()
	case "u", "U":
		idx := 0
		if key == "U" {
			idx = 1
		}
		return m.bumpAlert(idx)
	}
	m.clampCursors()
	return m, nil
}

// openFocused jumps from a Command Center panel to its dedicated tab, keeping the selection.
func (m *Model) openFocused() {
	if m.focus == kindPR {
		if pr, ok := m.selectedPR(); ok {
			m.tab, m.pill[kindPR] = tabPRs, 0
			for i, x := range m.prs() {
				if x.Repo == pr.Repo && x.Number == pr.Number {
					m.cursor[kindPR] = i
				}
			}
			return
		}
	}
	m.tab = m.focus + 1
	m.pill[m.focus] = 0
}

// overviewKPITabs maps each KPI card to the tab it opens.
var overviewKPITabs = []int{tabPRs, tabPRs, tabActions, tabIssues, tabRepos}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		m.move(1)
		return m, nil
	case tea.MouseButtonWheelUp:
		m.move(-1)
		return m, nil
	case tea.MouseButtonLeft:
	default:
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || m.data == nil {
		return m, nil
	}
	w, h := m.width, m.height
	x, y := msg.X, msg.Y
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.showNotifs {
		if x < w-drawerWidth(w) {
			m.showNotifs = false
		} else if idx := (y - m.bodyTop() - 2) / 3; idx >= 0 && idx < len(m.data.Notifications) {
			m.notifCursor = idx
		}
		return m, nil
	}
	if y == 1 {
		for i, r := range m.tabRanges() {
			if x >= r[0] && x < r[1] {
				m.tab = i
			}
		}
		return m, nil
	}
	x -= pageMargin
	w -= 2 * pageMargin
	// The sticky KPI strip is clickable from every tab.
	if kpiH, _ := chromeLayout(h); y >= navRows && y < navRows+kpiH {
		edge := 0
		for i, cw := range kpiWidths(w) {
			if x >= edge && x < edge+cw {
				m.tab, m.insights = overviewKPITabs[i], false
			}
			edge += cw + 1
		}
		return m, nil
	}
	by := y - m.bodyTop()
	bodyH := m.bodyHeight()
	if by < 0 || by >= bodyH {
		return m, nil
	}

	if m.tab != tabOverview {
		if m.insights {
			return m, nil
		}
		lw, _ := split(w, splitRatio(m.tab))
		if x < lw {
			before := m.cursor[m.tab-1]
			m.clickRow(m.tab-1, by, bodyH, false)
			if m.tab == tabRepos && m.cursor[kindRepo] == before && by >= panelChrome {
				m.insights = true
			}
		}
		return m, nil
	}

	g := overviewLayout(w, bodyH)
	switch {
	case by >= g.t1Y() && by < g.t1Y()+g.t1:
		kind := kindPR
		if x > g.lw {
			kind = kindRun
		}
		m.clickRow(kind, by-g.t1Y(), g.t1, true)
	case by >= g.t2Y() && by < g.t2Y()+g.t2:
		kind := kindRepo
		if x > g.lw {
			kind = kindIssue
		}
		m.clickRow(kind, by-g.t2Y(), g.t2, true)
	}
	return m, nil
}

// clickRow selects the table row under relY inside a panel of height panelH; a second click on the
// selected Command Center row opens its dedicated tab.
func (m *Model) clickRow(kind, relY, panelH int, overview bool) {
	wasFocused := m.focus == kind
	if overview {
		m.focus = kind
	}
	tableH := panelH - panelChrome
	if overview && kind == kindRun {
		if n := m.logBoxLines(tableH); n > 0 {
			tableH -= n
		}
	}
	line := relY - panelChrome
	if line < 0 {
		return
	}
	n := m.listLen(kind)
	start, mode := tableLayout(n, m.cursor[kind], tableH-1, m.kindHasSub(kind, overview))
	if line%mode.step() >= mode.lines {
		return
	}
	idx := start + line/mode.step()
	if idx >= n || line >= tableH-1 {
		return
	}
	if overview && wasFocused && idx == m.cursor[kind] {
		m.openFocused()
		return
	}
	m.cursor[kind] = idx
}

func (m Model) listLen(kind int) int {
	switch kind {
	case kindPR:
		return len(m.prs())
	case kindRun:
		return len(m.runs())
	case kindRepo:
		return len(m.repos())
	default:
		return len(m.issues())
	}
}

// kindHasSub reports whether a kind's table rows carry a second line in the given view.
func (m Model) kindHasSub(kind int, overview bool) bool {
	switch {
	case overview:
		return true
	case kind == kindIssue:
		return false
	default:
		return true
	}
}

// activeKind is the data kind the cursor keys currently drive.
func (m Model) activeKind() int {
	if m.tab == tabOverview {
		return m.focus
	}
	return m.tab - 1
}

func (m *Model) move(delta int) {
	if m.showNotifs && m.data != nil {
		m.notifCursor = clamp(m.notifCursor+delta, 0, len(m.data.Notifications)-1)
		return
	}
	k := m.activeKind()
	m.cursor[k] += delta
	m.clampCursors()
}

func (m *Model) clampCursors() {
	if m.data == nil {
		return
	}
	m.cursor[kindPR] = clamp(m.cursor[kindPR], 0, len(m.prs())-1)
	m.cursor[kindRun] = clamp(m.cursor[kindRun], 0, len(m.runs())-1)
	m.cursor[kindRepo] = clamp(m.cursor[kindRepo], 0, len(m.repos())-1)
	m.cursor[kindIssue] = clamp(m.cursor[kindIssue], 0, len(m.issues())-1)
}

// pillFor returns the filter chip for a kind; chips only apply on dedicated tabs.
func (m Model) pillFor(kind int) int {
	if m.tab == tabOverview {
		return 0
	}
	return m.pill[kind]
}

func (m Model) prs() []gh.PullRequest {
	var out []gh.PullRequest
	for _, pr := range m.data.ActionPRs() {
		if m.tab == tabOverview {
			if (m.quick == 1 && !pr.NeedsMyReview) || (m.quick == 2 && pr.NeedsMyReview) {
				continue
			}
		} else {
			switch m.pill[kindPR] {
			case 1:
				if pr.IsDraft || pr.ReviewDecision == gh.ReviewApproved || pr.ReviewDecision == gh.ReviewChangesRequested {
					continue
				}
			case 2:
				if pr.ReviewDecision != gh.ReviewApproved {
					continue
				}
			case 3:
				if pr.ReviewDecision != gh.ReviewChangesRequested {
					continue
				}
			}
		}
		if fuzzy(m.filter, fmt.Sprintf("#%d %s %s %s %s", pr.Number, pr.Title, pr.Repo, pr.Author, pr.HeadRef)) {
			out = append(out, pr)
		}
	}
	return out
}

func (m Model) runs() []gh.WorkflowRun {
	var out []gh.WorkflowRun
	pill := m.pillFor(kindRun)
	for _, r := range m.data.Runs {
		st := r.State()
		if (pill == 1 && st != gh.StateFailure) || (pill == 2 && st != gh.StatePending) || (pill == 3 && st != gh.StateSuccess) {
			continue
		}
		if fuzzy(m.filter, fmt.Sprintf("%d %s %s %s %s", r.ID, r.Name, r.Repo, r.Branch, r.Event)) {
			out = append(out, r)
		}
	}
	return out
}

func (m Model) repos() []gh.Repo {
	var out []gh.Repo
	pill := m.pillFor(kindRepo)
	for _, r := range m.data.Repos {
		if (pill == 1 && r.IsPrivate) || (pill == 2 && !r.IsPrivate) || (pill == 3 && r.Ahead == 0 && r.Behind == 0) {
			continue
		}
		if fuzzy(m.filter, r.NameWithOwner+" "+r.Language+" "+r.LatestMessage+" "+r.SyncBranch) {
			out = append(out, r)
		}
	}
	return out
}

func (m Model) issues() []gh.Issue {
	var out []gh.Issue
	pill := m.pillFor(kindIssue)
	for _, is := range m.data.Issues {
		if (pill == 1 && is.Priority != "P0") || (pill == 2 && is.Priority != "P1") || (pill == 3 && is.Priority < "P2") {
			continue
		}
		labels := ""
		for _, l := range is.Labels {
			labels += " " + l.Name
		}
		if fuzzy(m.filter, fmt.Sprintf("#%d %s %s %s", is.Number, is.Title, is.Repo, labels)) {
			out = append(out, is)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

func (m Model) selectedPR() (gh.PullRequest, bool) {
	l := m.prs()
	if len(l) == 0 {
		return gh.PullRequest{}, false
	}
	return l[clamp(m.cursor[kindPR], 0, len(l)-1)], true
}

func (m Model) selectedRun() (gh.WorkflowRun, bool) {
	l := m.runs()
	if len(l) == 0 {
		return gh.WorkflowRun{}, false
	}
	return l[clamp(m.cursor[kindRun], 0, len(l)-1)], true
}

func (m Model) selectedRepo() (gh.Repo, bool) {
	l := m.repos()
	if len(l) == 0 {
		return gh.Repo{}, false
	}
	return l[clamp(m.cursor[kindRepo], 0, len(l)-1)], true
}

func (m Model) selectedIssue() (gh.Issue, bool) {
	l := m.issues()
	if len(l) == 0 {
		return gh.Issue{}, false
	}
	return l[clamp(m.cursor[kindIssue], 0, len(l)-1)], true
}

func (m Model) demo() bool { return m.client == nil || m.data == nil || m.data.Demo }

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
		m.setStatus(statusInfo, "ghx: focus a pull request to approve (tab / 2)")
		return m, nil
	}
	pr, ok := m.selectedPR()
	if !ok {
		return m, nil
	}
	if pr.Author == m.data.User.Login {
		m.notify(statusErr, fmt.Sprintf("can't approve your own PR #%d", pr.Number))
		return m, nil
	}
	args := gh.ApproveArgs(pr.Repo, pr.Number)
	if m.demo() {
		m.mutatePR(pr, func(p *gh.PullRequest) {
			p.ReviewDecision = gh.ReviewApproved
			p.NeedsMyReview = false
		})
		m.notify(statusOK, fmt.Sprintf("Approved PR #%d (%s)", pr.Number, pr.Repo))
		return m, nil
	}
	m.setStatus(statusInfo, "gh "+strings.Join(args, " "))
	return m, ghCmd(fmt.Sprintf("Approved PR #%d (%s)", pr.Number, pr.Repo), args, true)
}

func (m Model) merge() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindPR {
		m.setStatus(statusInfo, "ghx: focus a pull request to merge (tab / 2)")
		return m, nil
	}
	pr, ok := m.selectedPR()
	if !ok {
		return m, nil
	}
	warn := ""
	if !pr.MergeReady() {
		warn = " (not merge-ready!)"
	}
	if !m.confirm("m", fmt.Sprintf("%s#%d", pr.Repo, pr.Number), fmt.Sprintf("squash-merge #%d%s", pr.Number, warn)) {
		return m, nil
	}
	args := gh.MergeArgs(pr.Repo, pr.Number)
	if m.demo() {
		m.mutatePR(pr, func(p *gh.PullRequest) { p.Merged = true })
		m.removeMerged()
		m.notify(statusOK, fmt.Sprintf("Squashed & merged PR #%d into %s", pr.Number, pr.BaseRef))
		return m, nil
	}
	m.setStatus(statusInfo, "gh "+strings.Join(args, " "))
	return m, ghCmd(fmt.Sprintf("Squashed & merged PR #%d", pr.Number), args, true)
}

func (m Model) checkoutCloneComment() (tea.Model, tea.Cmd) {
	switch m.activeKind() {
	case kindPR:
		pr, ok := m.selectedPR()
		if !ok {
			return m, nil
		}
		args := gh.CheckoutArgs(pr.Repo, pr.Number)
		if m.demo() {
			m.notify(statusOK, "demo · would run gh "+strings.Join(args, " "))
			return m, nil
		}
		m.setStatus(statusInfo, "gh "+strings.Join(args, " "))
		return m, ghCmd(fmt.Sprintf("Checked out PR #%d", pr.Number), args, false)
	case kindRepo:
		r, ok := m.selectedRepo()
		if !ok {
			return m, nil
		}
		cmd := gh.CloneCommand(r.NameWithOwner)
		if err := gh.CopyToClipboard(cmd); err != nil {
			m.notify(statusInfo, "run: "+cmd)
		} else {
			m.notify(statusOK, "Copied: "+cmd)
		}
	case kindIssue:
		is, ok := m.selectedIssue()
		if !ok {
			return m, nil
		}
		args := gh.CommentIssueArgs(is.Repo, is.Number)
		if m.demo() {
			m.mutateIssue(is, func(i *gh.Issue) { i.Comments++ })
			m.notify(statusOK, fmt.Sprintf("Added comment to #%d", is.Number))
			return m, nil
		}
		return m, ghCmd(fmt.Sprintf("Opened comment composer for #%d", is.Number), args, false)
	default:
		m.setStatus(statusInfo, "ghx: c checks out PRs, copies repo clones and comments on issues")
	}
	return m, nil
}

func (m Model) rerunOrRefresh() (tea.Model, tea.Cmd) {
	if m.tab == tabRepos && !m.demo() {
		if r, ok := m.selectedRepo(); ok {
			delete(m.analytics, r.NameWithOwner)
			delete(m.analyticsErr, r.NameWithOwner)
			m.setStatus(statusInfo, "ghx: reloading insights for "+r.NameWithOwner+"…")
		}
	}
	if m.activeKind() == kindRun {
		if run, ok := m.selectedRun(); ok && run.State() == gh.StateFailure {
			args := gh.RerunArgs(run.Repo, run.ID)
			if m.demo() {
				for i := range m.data.Runs {
					if m.data.Runs[i].ID == run.ID {
						m.data.Runs[i].Status, m.data.Runs[i].Conclusion = "in_progress", ""
						m.data.Runs[i].StartedAt = time.Now()
					}
				}
				m.notify(statusOK, fmt.Sprintf("Re-running failed jobs for %s (#%d)", run.Name, run.ID))
				return m, nil
			}
			m.setStatus(statusInfo, "gh "+strings.Join(args, " "))
			return m, ghCmd(fmt.Sprintf("Re-run queued for %s (#%d)", run.Name, run.ID), args, true)
		}
	}
	if m.client == nil {
		m.data = gh.MockDashboard()
		m.clampCursors()
		m.notify(statusOK, "Fetched latest GitHub state (demo)")
		return m, nil
	}
	if m.loading {
		return m, nil
	}
	m.loading = true
	m.setStatus(statusInfo, "gh api graphql // fetching latest GitHub state…")
	return m, tea.Batch(fetchCmd(m.client, m.cfg.Org), tick(120*time.Millisecond))
}

func (m Model) dispatchWorkflow() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindRun {
		m.setStatus(statusInfo, "ghx: w triggers workflow_dispatch from the Actions view (3)")
		return m, nil
	}
	r, ok := m.selectedRun()
	if !ok {
		return m, nil
	}
	if !m.confirm("w", fmt.Sprint(r.ID), fmt.Sprintf("run workflow_dispatch for %s on %s", r.Name, r.Repo)) {
		return m, nil
	}
	args := gh.WorkflowDispatchArgs(r.Repo, r.Name)
	if m.demo() {
		m.notify(statusOK, fmt.Sprintf("Dispatched %s on %s", r.Name, r.Repo))
		return m, nil
	}
	return m, ghCmd(fmt.Sprintf("Dispatched %s on %s", r.Name, r.Repo), args, true)
}

func (m Model) closeIssue() (tea.Model, tea.Cmd) {
	if m.activeKind() != kindIssue {
		m.setStatus(statusInfo, "ghx: x closes the selected issue (5)")
		return m, nil
	}
	is, ok := m.selectedIssue()
	if !ok {
		return m, nil
	}
	if !m.confirm("x", fmt.Sprintf("%s#%d", is.Repo, is.Number), fmt.Sprintf("close issue #%d", is.Number)) {
		return m, nil
	}
	remove := func(mm *Model) {
		var keep []gh.Issue
		for _, i := range mm.data.Issues {
			if !(i.Repo == is.Repo && i.Number == is.Number) {
				keep = append(keep, i)
			}
		}
		mm.data.Issues = keep
		mm.clampCursors()
	}
	label := fmt.Sprintf("Closed issue #%d (%s)", is.Number, is.Repo)
	if m.demo() {
		remove(&m)
		m.notify(statusOK, label)
		return m, nil
	}
	return m, func() tea.Msg {
		return actionMsg{label: label, res: gh.RunGH(gh.CloseIssueArgs(is.Repo, is.Number)...), after: remove}
	}
}

func (m Model) markAllRead() (tea.Model, tea.Cmd) {
	clear := func(mm *Model) {
		mm.data.Notifications = nil
		mm.notifCursor = 0
	}
	if m.demo() {
		clear(&m)
		m.notify(statusOK, "Marked all notifications as read")
		return m, nil
	}
	return m, func() tea.Msg {
		return actionMsg{label: "Marked all notifications as read", res: gh.RunGH(gh.MarkNotificationsReadArgs()...), after: clear}
	}
}

func (m Model) openInBrowser() (tea.Model, tea.Cmd) {
	var args []string
	if m.showNotifs && len(m.data.Notifications) > 0 {
		n := m.data.Notifications[clamp(m.notifCursor, 0, len(m.data.Notifications)-1)]
		args = []string{"browse", "--repo", n.Repo}
	} else {
		switch m.activeKind() {
		case kindPR:
			if pr, ok := m.selectedPR(); ok {
				args = []string{"pr", "view", fmt.Sprint(pr.Number), "--repo", pr.Repo, "--web"}
			}
		case kindRun:
			if r, ok := m.selectedRun(); ok {
				args = []string{"run", "view", fmt.Sprint(r.ID), "--repo", r.Repo, "--web"}
			}
		case kindRepo:
			if r, ok := m.selectedRepo(); ok {
				args = []string{"browse", "--repo", r.NameWithOwner}
			}
		case kindIssue:
			if is, ok := m.selectedIssue(); ok {
				args = []string{"issue", "view", fmt.Sprint(is.Number), "--repo", is.Repo, "--web"}
			}
		}
	}
	if args == nil {
		return m, nil
	}
	if m.demo() {
		m.notify(statusOK, "demo · would run gh "+strings.Join(args, " "))
		return m, nil
	}
	return m, ghCmd("Opened in browser", args, false)
}

func (m Model) bumpAlert(idx int) (tea.Model, tea.Cmd) {
	if idx >= len(m.data.Alerts) {
		m.setStatus(statusInfo, "ghx: no Dependabot alert in that slot")
		return m, nil
	}
	a := m.data.Alerts[idx]
	url := fmt.Sprintf("https://github.com/%s/security/dependabot", a.Repo)
	if m.demo() {
		m.notify(statusOK, fmt.Sprintf("Bump %s → %s (%s)", a.Package, a.PatchedVersion, a.CVE))
		return m, nil
	}
	if err := gh.OpenURL(url); err != nil {
		m.notify(statusErr, "open failed: "+err.Error())
	} else {
		m.notify(statusOK, fmt.Sprintf("Opened Dependabot for %s → bump %s to %s", a.Repo, a.Package, a.PatchedVersion))
	}
	return m, nil
}

func (m *Model) mutatePR(target gh.PullRequest, f func(*gh.PullRequest)) {
	for _, list := range []*[]gh.PullRequest{&m.data.ReviewPRs, &m.data.MyPRs} {
		for i := range *list {
			if (*list)[i].Repo == target.Repo && (*list)[i].Number == target.Number {
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

// fuzzy reports whether every whitespace-separated term of query is a subsequence of hay.
func fuzzy(query, hay string) bool {
	if query == "" {
		return true
	}
	hay = strings.ToLower(hay)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		i := 0
		for _, r := range hay {
			if i < len(term) && r == rune(term[i]) {
				i++
			}
		}
		if i < len(term) {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// View renders the whole screen, painted with the theme background.
// chromeLayout sizes the sticky KPI strip and the gap under it for a terminal of height h.
func chromeLayout(h int) (kpiH, gap int) {
	switch {
	case h >= 44:
		return 5, 1
	case h >= 32:
		return 4, 1
	default:
		return 3, 0
	}
}

// bodyTop is the first row of the scrolling body: everything above it (header, tabs, KPI strip) is sticky.
func (m Model) bodyTop() int {
	k, g := chromeLayout(m.height)
	return navRows + k + g
}

// bodyHeight is the number of rows available to the active view.
func (m Model) bodyHeight() int { return m.height - m.bodyTop() - 1 }

func (m Model) View() string {
	w, h := m.width, m.height
	if w == 0 || h == 0 {
		return ""
	}
	if m.data == nil {
		return m.splash(w, h)
	}
	if w < minWidth || h < minHeight {
		msg := m.p.bold(m.p.Yellow, "ghx needs at least 80×24") + "\n" + m.p.muted(fmt.Sprintf("current terminal: %d×%d", w, h))
		return m.fullscreen(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg), w, h)
	}

	// The body gets a 1-cell page margin on each side, like the design's padded viewport.
	bodyH := m.bodyHeight()
	body := strings.Split(m.renderBody(w-2*pageMargin, bodyH), "\n")
	for len(body) < bodyH {
		body = append(body, "")
	}
	body = body[:bodyH]
	for i := range body {
		body[i] = fit(strings.Repeat(" ", pageMargin)+body[i], w)
	}
	if m.showHelp {
		for i := range body {
			body[i] = m.p.dim(ansi.Strip(body[i]))
		}
		box := m.helpModal()
		bw := width(box[0])
		body = overlay(body, box, max((w-bw)/2, 0), clamp((bodyH-len(box))/2, 0, bodyH), w)
	}
	if m.showNotifs {
		dw := drawerWidth(w)
		body = overlay(body, m.notificationsDrawer(dw, bodyH), w-dw-1, 0, w)
	}
	if m.toast != "" && time.Since(m.toastAt) < toastTTL && !m.showHelp {
		color, icon := m.p.Green, m.p.ic.CheckPass
		if m.toastKind == statusErr {
			color, icon = m.p.Red, "!"
		}
		t := m.p.pill(color, icon+" "+m.toast)
		if tw := width(t); tw < w-4 {
			body = overlay(body, []string{t}, w-tw-2, bodyH-1, w)
		}
	}

	lines := make([]string, 0, h)
	lines = append(lines, m.header(w), m.tabBar(w), m.tabRule(w))
	kpiH, gap := chromeLayout(h)
	margin := strings.Repeat(" ", pageMargin)
	for _, l := range strings.Split(m.kpiStrip(w-2*pageMargin, kpiH), "\n") {
		lines = append(lines, fit(margin+l, w))
	}
	for i := 0; i < gap; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, body...)
	lines = append(lines, m.footer(w))
	return m.fullscreen(strings.Join(lines, "\n"), w, h)
}

func (m Model) fullscreen(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	lines = lines[:h]
	for i := range lines {
		lines[i] = paint(lines[i], m.p.BgPrimary, w)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderBody(w, h int) string {
	switch m.tab {
	case tabPRs:
		return m.viewPRs(w, h)
	case tabActions:
		return m.viewActions(w, h)
	case tabRepos:
		return m.viewRepos(w, h)
	case tabIssues:
		return m.viewIssues(w, h)
	default:
		return m.viewOverview(w, h)
	}
}

func (m Model) splash(w, h int) string {
	logo := []string{
		" ██████╗ ██╗  ██╗██╗  ██╗",
		"██╔════╝ ██║  ██║╚██╗██╔╝",
		"██║  ███╗███████║ ╚███╔╝ ",
		"██║   ██║██╔══██║ ██╔██╗ ",
		"╚██████╔╝██║  ██║██╔╝ ██╗",
		" ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝",
	}
	colors := []string{m.p.Blue, m.p.Blue, m.p.Cyan, m.p.Cyan, m.p.Purple, m.p.Purple}
	var b strings.Builder
	for i, l := range logo {
		b.WriteString(m.p.bold(colors[i], l) + "\n")
	}
	b.WriteString("\n" + m.p.muted("Your GitHub universe, in your terminal.") + "\n\n")
	b.WriteString(m.p.fg(m.p.Yellow, m.spin()) + " " + m.p.text(m.status))
	return m.fullscreen(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, b.String()), w, h)
}

// split returns left/right widths for a ratio, leaving a 1-cell gutter.
func split(w int, pct int) (int, int) {
	l := w * pct / 100
	return l, w - l - 1
}

func hjoin(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// RenderCheck renders every tab at several terminal sizes and returns one frame per tab/size.
func RenderCheck(cfg *config.Config, d *gh.Dashboard) (map[string]string, error) {
	sizes := [][2]int{{80, 24}, {120, 36}, {160, 48}}
	out := map[string]string{}
	for _, sz := range sizes {
		for tab := range tabNames {
			m := checkModel(cfg, d)
			m.width, m.height = sz[0], sz[1]
			m.tab = tab
			frame, err := safeView(m)
			if err != nil {
				return out, fmt.Errorf("tab %d (%s) at %dx%d: %w", tab+1, tabNames[tab], sz[0], sz[1], err)
			}
			out[fmt.Sprintf("%dx%d/%d-%s", sz[0], sz[1], tab+1, tabNames[tab])] = frame
		}
		{
			m := checkModel(cfg, d)
			m.width, m.height = sz[0], sz[1]
			m.tab, m.insights = tabRepos, true
			frame, err := safeView(m)
			if err != nil {
				return out, fmt.Errorf("insights at %dx%d: %w", sz[0], sz[1], err)
			}
			out[fmt.Sprintf("%dx%d/4-insights", sz[0], sz[1])] = frame
		}
		for _, modal := range []string{"inbox", "help"} {
			m := checkModel(cfg, d)
			m.width, m.height = sz[0], sz[1]
			m.showNotifs, m.showHelp = modal == "inbox", modal == "help"
			frame, err := safeView(m)
			if err != nil {
				return out, fmt.Errorf("%s modal at %dx%d: %w", modal, sz[0], sz[1], err)
			}
			out[fmt.Sprintf("%dx%d/modal-%s", sz[0], sz[1], modal)] = frame
		}
	}
	return out, nil
}

// Snapshot renders a single frame for the given tab and size.
func Snapshot(cfg *config.Config, d *gh.Dashboard, tab, w, h int, notifs, help bool) (string, error) {
	m := checkModel(cfg, d)
	m.width, m.height = w, h
	if tab == len(tabNames) {
		tab, m.insights = tabRepos, true
	}
	m.tab = clamp(tab, 0, len(tabNames)-1)
	m.showNotifs, m.showHelp = notifs, help
	return safeView(m)
}

// checkModel builds an offline model around pre-fetched data for --check / --snapshot.
func checkModel(cfg *config.Config, d *gh.Dashboard) Model {
	m := New(cfg, nil, "")
	m.data = d
	return m
}

func safeView(m Model) (frame string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	frame = m.View()
	if strings.TrimSpace(frame) == "" {
		return "", fmt.Errorf("empty frame")
	}
	return frame, nil
}
