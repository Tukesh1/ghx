package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minWidth   = 80
	minHeight  = 24
	toastTTL   = 4 * time.Second
	pageMargin = 1
	// navRows are the fixed rows above the KPI strip: header, tab bar and tab rule.
	navRows = 3
)

// Tab indices, matching the 1-6 hotkeys.
const (
	tabProfile = iota
	tabOverview
	tabPRs
	tabActions
	tabRepos
	tabIssues
)

// Data kinds; each dedicated tab shows one kind and the Command Center focuses one at a time.
// The first four map onto the tabs from tabPRs on; kindPinned is the Profile tab's pinned repositories.
const (
	kindPR = iota
	kindRun
	kindRepo
	kindIssue
	kindPinned
)

// Exported tab indices for --snapshot and tools/screenshot, so callers never hard-code positions.
// TabInsights is the full-screen repository insights view, one past the last tab.
const (
	TabProfile  = tabProfile
	TabOverview = tabOverview
	TabPRs      = tabPRs
	TabActions  = tabActions
	TabRepos    = tabRepos
	TabIssues   = tabIssues
	TabInsights = tabIssues + 1
)

// TabCount is the number of tabs, bound to the 1…TabCount hotkeys.
func TabCount() int { return len(tabNames) }

// tabKey is the hotkey that opens a tab.
func tabKey(tab int) string { return fmt.Sprint(tab + 1) }

// tabFor is the dedicated tab of a data kind.
func tabFor(kind int) int { return kind + tabPRs }

var (
	tabNames      = []string{"Profile", "Command Center", "Pull Requests", "Actions CI/CD", "Repositories", "Issues & Bugs"}
	shortTabNames = []string{"Profile", "Overview", "PRs", "Actions", "Repos", "Issues"}
	// pillCount is the number of filter chips per dedicated tab, cycled with f.
	pillCount = [4]int{4, 4, 4, 4}
	// retryBackoff spaces out automatic retries after a failed fetch.
	retryBackoff = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute}
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

type tickMsg time.Time

type analyticsMsg struct {
	repo    string
	a       *gh.RepoAnalytics
	err     error
	attempt int
}

func analyticsCmd(c *gh.Client, repo string, attempt int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := c.FetchRepoAnalytics(ctx, repo)
		return analyticsMsg{repo: repo, a: a, err: err, attempt: attempt}
	}
}

// ensureAnalytics starts loading insight data for the repo under the cursor when the Repos tab shows it.
func (m *Model) ensureAnalytics() tea.Cmd {
	if m.tab != tabRepos || m.demo() {
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
	return analyticsCmd(m.client, name, 0)
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
	cursor        [5]int
	pill          [4]int
	quick         int
	notifCursor   int

	search    textinput.Model
	searching bool
	filter    string

	showHelp   bool
	showNotifs bool
	insights   bool

	composer  textarea.Model
	composeTo *commentTarget

	// Per-repo analytics, loaded lazily when a repo is inspected (maps are shared across model copies).
	analytics        map[string]*gh.RepoAnalytics
	analyticsLoading map[string]bool
	analyticsErr     map[string]string

	loading   bool
	fetchErr  string    // last fetch error while there is no data to show
	failures  int       // consecutive failed fetches, for backoff
	nextFetch time.Time // when the next automatic fetch is due; zero = none
	busy      int       // mutations in flight
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
	m := Model{cfg: cfg, client: client, search: ti, tab: tabOverview,
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
		m.status = "fetching your GitHub dashboard…"
		if cached, err := gh.LoadCache(cfg.Org); err == nil && cached.User.Login != "" {
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

// Init starts the first fetch (live mode) and the single clock tick that drives the whole UI.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(m.tickInterval())}
	if m.client != nil {
		cmds = append(cmds, fetchCmd(m.client, m.cfg.Org))
	}
	return tea.Batch(cmds...)
}

// tickInterval is fast while something animates (spinner) and once a second otherwise.
// Exactly one tick is ever in flight: only the tickMsg handler schedules the next one.
func (m Model) tickInterval() time.Duration {
	if m.loading || m.busy > 0 || len(m.analyticsLoading) > 0 {
		return 120 * time.Millisecond
	}
	return time.Second
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchCmd(c *gh.Client, org string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		d, err := c.Fetch(ctx, org)
		return dataMsg{d: d, err: err}
	}
}

// startFetch begins a dashboard fetch; callers must check m.loading first.
func (m *Model) startFetch(status string) tea.Cmd {
	m.loading = true
	m.nextFetch = time.Time{}
	if status != "" {
		m.setStatus(statusInfo, "ghx: "+status)
	}
	return fetchCmd(m.client, m.cfg.Org)
}

// scheduleFetch moves the next automatic fetch earlier (never later).
func (m *Model) scheduleFetch(in time.Duration) {
	at := time.Now().Add(in)
	if m.nextFetch.IsZero() || at.Before(m.nextFetch) {
		m.nextFetch = at
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
		var cmds []tea.Cmd
		if m.client != nil && !m.loading && !m.nextFetch.IsZero() && time.Now().After(m.nextFetch) {
			status := "auto-refreshing…"
			if m.failures > 0 {
				status = "retrying…"
			}
			cmds = append(cmds, m.startFetch(status))
		}
		return m, tea.Batch(append(cmds, tick(m.tickInterval()))...)

	case dataMsg:
		return m.handleData(msg)

	case actionMsg:
		return m.handleAction(msg)

	case analyticsMsg:
		delete(m.analyticsLoading, msg.repo)
		if msg.err != nil {
			m.analyticsErr[msg.repo] = msg.err.Error()
			return m, nil
		}
		m.analytics[msg.repo] = msg.a
		// GitHub answers 202 while it computes statistics; retry a few times shortly after.
		if msg.a.StatsPending && msg.attempt < 3 && m.client != nil {
			m.analyticsLoading[msg.repo] = true
			client, repo, next := m.client, msg.repo, msg.attempt+1
			return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return analyticsCmd(client, repo, next)() })
		}
		return m, nil

	case tea.MouseMsg:
		if m.data == nil {
			return m, nil
		}
		next, cmd := m.handleMouse(msg)
		mm := next.(Model)
		return mm, tea.Batch(cmd, mm.ensureAnalytics())

	case tea.KeyMsg:
		if m.data == nil {
			return m.errorScreenKey(msg)
		}
		next, cmd := m.handleKey(msg)
		mm := next.(Model)
		return mm, tea.Batch(cmd, mm.ensureAnalytics())
	}
	if m.composeTo != nil {
		var cmd tea.Cmd
		m.composer, cmd = m.composer.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleData(msg dataMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	if msg.err != nil {
		m.failures++
		wait := retryBackoff[min(m.failures-1, len(retryBackoff)-1)]
		m.nextFetch = time.Now().Add(wait)
		if m.data == nil {
			m.fetchErr = msg.err.Error()
			return m, nil
		}
		m.setStatus(statusErr, fmt.Sprintf("ghx: refresh failed (%s) · retrying in %s · showing data from %s ago", firstLine(msg.err.Error()), wait, ago(m.data.FetchedAt)))
		return m, nil
	}
	m.failures, m.fetchErr = 0, ""
	m.nextFetch = time.Now().Add(m.cfg.Refresh)
	m.data = msg.d
	m.clampCursors()
	_ = gh.SaveCache(m.data, m.cfg.Org)
	if n := len(msg.d.Warnings); n > 0 {
		more := ""
		if n > 1 {
			more = fmt.Sprintf(" (+%d more)", n-1)
		}
		m.setStatus(statusErr, "ghx: partial data · "+msg.d.Warnings[0]+more)
	} else {
		m.setStatus(statusInfo, "")
	}
	return m, nil
}

// errorScreenKey handles keys while live data has never loaded.
func (m Model) errorScreenKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "r", "enter":
		if !m.loading && m.client != nil {
			m.fetchErr = ""
			return m, m.startFetch("")
		}
	case "d":
		m.data = gh.MockDashboard()
		m.nextFetch = time.Time{}
		m.setStatus(statusErr, "ghx: demo data · live fetch failed: "+firstLine(m.fetchErr)+" · press R to retry")
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
	if m.composeTo != nil {
		return m.composerKey(msg)
	}

	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.search.Blur()
			m.search.SetValue("")
			m.filter = ""
			m.cursor = [5]int{}
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
			m.cursor = [5]int{}
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
		case "g", "home":
			m.notifCursor = 0
			return m, nil
		case "G", "end":
			m.notifCursor = max(len(m.data.Notifications)-1, 0)
			return m, nil
		case "enter", "o":
			return m.openNotification()
		case "m":
			return m.markAllRead()
		case "y":
			return m.yank()
		case "q":
			return m, tea.Quit
		}
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if n := int(key[0] - '1'); n < len(tabNames) {
			m.tab, m.insights = n, false
		}
	case "backspace":
		m.insights = false
	case "tab", "l", "right":
		if m.tab == tabOverview {
			m.focus = (m.focus + 1) % 4
		} else if key == "tab" {
			m.tab, m.insights = cycleTab(m.tab, 1), false
		}
	case "shift+tab", "h", "left":
		if m.tab == tabOverview {
			m.focus = (m.focus + 3) % 4
		} else if key == "shift+tab" {
			m.tab, m.insights = cycleTab(m.tab, -1), false
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
		m.cursor = [5]int{}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d", "pgdown":
		m.move(5)
	case "ctrl+u", "pgup":
		m.move(-5)
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
		default:
			return m.openInBrowser()
		}
	case "f":
		if m.tab == tabProfile {
			break
		}
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
		m.saveConfig()
		m.setStatus(statusInfo, "ghx: theme → "+t.Name)
	case "i":
		m.cfg.ASCII = !m.cfg.ASCII
		m.cfg.GlyphsChosen = true
		m.applyTheme()
		m.saveConfig()
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
	case "C":
		return m.startComment()
	case "r":
		return m.rerunOrRefresh()
	case "R":
		return m.refresh()
	case "w":
		return m.dispatchWorkflow()
	case "x":
		return m.closeIssue()
	case "o":
		return m.openInBrowser()
	case "O":
		return m.openProfile()
	case "y":
		return m.yank()
	case "u", "U":
		idx := 0
		if key == "U" {
			idx = 1
		}
		return m.openAlert(idx)
	}
	m.clampCursors()
	return m, nil
}

func (m Model) saveConfig() {
	if err := m.cfg.Save(); err != nil {
		m.setStatus(statusErr, "ghx: could not save settings: "+err.Error())
	}
}

// openFocused jumps from a Command Center panel to its dedicated tab, keeping the selection.
func (m *Model) openFocused() {
	kind := m.focus
	var key string
	switch kind {
	case kindPR:
		if pr, ok := m.selectedPR(); ok {
			key = pr.Key()
		}
	case kindRun:
		if r, ok := m.selectedRun(); ok {
			key = fmt.Sprint(r.ID)
		}
	case kindRepo:
		if r, ok := m.selectedRepo(); ok {
			key = r.NameWithOwner
		}
	case kindIssue:
		if is, ok := m.selectedIssue(); ok {
			key = fmt.Sprintf("%s#%d", is.Repo, is.Number)
		}
	}
	m.tab, m.pill[kind] = tabFor(kind), 0
	m.cursor[kind] = 0
	for i, k := range m.itemKeys(kind) {
		if k == key {
			m.cursor[kind] = i
		}
	}
}

// itemKeys lists stable identities of the visible items of a kind, in display order.
func (m Model) itemKeys(kind int) []string {
	var keys []string
	switch kind {
	case kindPR:
		for _, x := range m.prs() {
			keys = append(keys, x.Key())
		}
	case kindRun:
		for _, x := range m.runs() {
			keys = append(keys, fmt.Sprint(x.ID))
		}
	case kindRepo:
		for _, x := range m.repos() {
			keys = append(keys, x.NameWithOwner)
		}
	case kindIssue:
		for _, x := range m.issues() {
			keys = append(keys, fmt.Sprintf("%s#%d", x.Repo, x.Number))
		}
	}
	return keys
}

// overviewKPITabs maps each KPI card to the tab it opens.
var overviewKPITabs = []int{tabPRs, tabPRs, tabActions, tabIssues, tabRepos}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.composeTo != nil {
		return m, nil
	}
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
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	w, h := m.width, m.height
	x, y := msg.X, msg.Y
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.showNotifs {
		if x < w-drawerWidth(w)-1 {
			m.showNotifs = false
		} else if idx := m.notifAt(y - m.bodyTop()); idx >= 0 {
			if idx == m.notifCursor {
				return m.openNotification()
			}
			m.notifCursor = idx
		}
		return m, nil
	}
	switch y {
	case 0:
		return m.clickHeader(x)
	case 1:
		for i, r := range m.tabRanges() {
			if x >= r[0] && x < r[1] {
				m.tab, m.insights = i, false
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

	if m.tab == tabProfile {
		return m.clickProfile(x, by, w, bodyH)
	}
	if m.tab != tabOverview {
		if m.insights {
			return m, nil
		}
		lw, _ := split(w, splitRatio(m.tab))
		if x < lw {
			kind := m.activeKind()
			before := m.cursor[kind]
			if m.clickRow(kind, by, bodyH, false) && m.cursor[kind] == before {
				if m.tab == tabRepos {
					m.insights = true
					return m, nil
				}
				return m.openInBrowser()
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

// clickHeader makes the header's theme, glyph, inbox and keys controls clickable.
func (m Model) clickHeader(x int) (tea.Model, tea.Cmd) {
	plain := ansi.Strip(m.header(m.width))
	at := func(label string) bool {
		i := strings.Index(plain, label)
		if i < 0 {
			return false
		}
		start := ansi.StringWidth(plain[:i])
		return x >= start-2 && x < start+ansi.StringWidth(label)+8
	}
	switch {
	case at("THEME"):
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	case at("GLYPHS"):
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	case at("Inbox"):
		m.showNotifs, m.notifCursor = true, 0
	case at("Keys"):
		m.showHelp = true
	}
	return m, nil
}

// clickRow selects the table row under relY inside a panel of height panelH and reports whether a
// row was hit. A second click on the selected Command Center row opens its dedicated tab.
func (m *Model) clickRow(kind, relY, panelH int, overview bool) bool {
	wasFocused := m.focus == kind
	if overview {
		m.focus = kind
	}
	tableH := panelH - panelChrome
	if overview && kind == kindRun {
		tableH -= m.logBoxLines(tableH)
	}
	line := relY - panelChrome
	if line < 0 {
		return false
	}
	n := m.listLen(kind)
	start, mode := tableLayout(n, m.cursor[kind], tableH-1, rowsHaveSub(m.rowsFor(kind, overview)))
	if line%mode.step() >= mode.lines {
		return false
	}
	idx := start + line/mode.step()
	if idx >= n || line >= tableH-1 {
		return false
	}
	if overview && wasFocused && idx == m.cursor[kind] {
		m.openFocused()
		return true
	}
	m.cursor[kind] = idx
	return true
}

func (m Model) listLen(kind int) int {
	switch kind {
	case kindPR:
		return len(m.prs())
	case kindRun:
		return len(m.runs())
	case kindRepo:
		return len(m.repos())
	case kindPinned:
		return len(m.pinned())
	default:
		return len(m.issues())
	}
}

// rowsFor builds the table rows a kind renders in the Command Center or on its dedicated tab.
func (m Model) rowsFor(kind int, overview bool) [][]string {
	switch {
	case kind == kindPR && overview:
		return m.ovPRRows(m.prs())
	case kind == kindPR:
		return m.prRows(m.prs())
	case kind == kindRun && overview:
		return m.ovRunRows(m.runs())
	case kind == kindRun:
		return m.runRows(m.runs())
	case kind == kindRepo && overview:
		return m.ovRepoRows(m.repos())
	case kind == kindRepo:
		return m.repoRows(m.repos())
	case overview:
		return m.ovIssueRows(m.issues())
	default:
		return m.issueRows(m.issues())
	}
}

// activeKind is the data kind the cursor keys currently drive.
func (m Model) activeKind() int {
	switch m.tab {
	case tabOverview:
		return m.focus
	case tabProfile:
		return kindPinned
	}
	return m.tab - tabPRs
}

// cycleTab steps through the tabs with tab / shift+tab. The Command Center is skipped: there, tab
// moves between its panels instead.
func cycleTab(tab, delta int) int {
	for {
		tab = (tab + delta + len(tabNames)) % len(tabNames)
		if tab != tabOverview {
			return tab
		}
	}
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
	m.cursor[kindPinned] = clamp(m.cursor[kindPinned], 0, len(m.pinned())-1)
	m.notifCursor = clamp(m.notifCursor, 0, len(m.data.Notifications)-1)
}

// pillFor returns the filter chip for a kind; chips only apply on dedicated tabs.
func (m Model) pillFor(kind int) int {
	if m.tab == tabOverview {
		return 0
	}
	return m.pill[kind]
}

// needsReview reports whether a PR waits on a review: requested from the viewer or required by the repo.
func needsReview(pr gh.PullRequest) bool {
	return !pr.IsDraft && (pr.NeedsMyReview || pr.ReviewDecision == gh.ReviewRequired)
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
				if !needsReview(pr) || pr.ReviewDecision == gh.ReviewApproved {
					continue
				}
			case 2:
				if !pr.MergeReady() {
					continue
				}
			case 3:
				if pr.ReviewDecision != gh.ReviewChangesRequested && pr.CIState != gh.StateFailure && !pr.Conflicting {
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
		if fuzzy(m.filter, r.NameWithOwner+" "+r.Language+" "+r.LatestMessage+" "+r.SyncBranch+" "+r.Description) {
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

// demo reports whether the model shows offline sample data, so actions are simulated.
func (m Model) demo() bool { return m.client == nil || m.data == nil || m.data.Demo }

// fuzzy reports whether every whitespace-separated term of query is a subsequence of hay.
func fuzzy(query, hay string) bool {
	if query == "" {
		return true
	}
	hay = strings.ToLower(hay)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		want := []rune(term)
		i := 0
		for _, r := range hay {
			if i < len(want) && r == want[i] {
				i++
			}
		}
		if i < len(want) {
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

// View renders the whole screen, painted with the theme background.
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
	dimmed := func() {
		for i := range body {
			body[i] = m.p.dim(ansi.Strip(body[i]))
		}
	}
	switch {
	case m.composeTo != nil:
		dimmed()
		box := m.composerModal(w, bodyH)
		body = overlay(body, box, max((w-width(box[0]))/2, 0), clamp((bodyH-len(box))/2, 0, bodyH), w)
	case m.showHelp:
		dimmed()
		box := m.helpModal(w, bodyH)
		body = overlay(body, box, max((w-width(box[0]))/2, 0), clamp((bodyH-len(box))/2, 0, bodyH), w)
	case m.showNotifs:
		dw := drawerWidth(w)
		body = overlay(body, m.notificationsDrawer(dw, bodyH), w-dw-1, 0, w)
	}
	if m.toast != "" && time.Since(m.toastAt) < toastTTL && !m.showHelp && m.composeTo == nil {
		color, icon := m.p.Green, m.p.ic.CheckPass
		switch m.toastKind {
		case statusErr:
			color, icon = m.p.Red, "!"
		case statusInfo:
			color, icon = m.p.Blue, "i"
		}
		t := m.p.pill(color, icon+" "+m.toast)
		if tw := width(t); tw > w-4 {
			t = m.p.pill(color, fit(icon+" "+m.toast, w-6))
		}
		body = overlay(body, []string{t}, max(w-width(t)-2, 0), bodyH-1, w)
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
	case tabProfile:
		return m.viewProfile(w, h)
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

var logo = []string{
	" ██████╗ ██╗  ██╗██╗  ██╗",
	"██╔════╝ ██║  ██║╚██╗██╔╝",
	"██║  ███╗███████║ ╚███╔╝ ",
	"██║   ██║██╔══██║ ██╔██╗ ",
	"╚██████╔╝██║  ██║██╔╝ ██╗",
	" ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝",
}

// splash is the startup screen while the first fetch runs, and the error screen if it fails.
func (m Model) splash(w, h int) string {
	p := m.p
	colors := []string{p.Blue, p.Blue, p.Cyan, p.Cyan, p.Purple, p.Purple}
	var b strings.Builder
	for i, l := range logo {
		b.WriteString(p.bold(colors[i], l) + "\n")
	}
	b.WriteString("\n" + p.muted("Your GitHub universe, in your terminal.") + "\n\n")
	switch {
	case m.fetchErr != "" && !m.loading:
		msgW := clamp(w-8, 30, 72)
		b.WriteString(p.bold(p.Red, p.ic.CheckFail+" Couldn't load your dashboard") + "\n")
		for _, l := range wrap(sanitize(m.fetchErr), msgW, 3) {
			b.WriteString(p.muted(l) + "\n")
		}
		b.WriteString("\n")
		if !m.nextFetch.IsZero() {
			b.WriteString(p.dim(fmt.Sprintf("retrying in %s", time.Until(m.nextFetch).Round(time.Second))) + "\n\n")
		}
		b.WriteString(p.kbd("r") + p.muted(" retry now   ") + p.kbd("d") + p.muted(" explore demo data   ") + p.kbd("q") + p.muted(" quit"))
	default:
		b.WriteString(p.fg(p.Yellow, m.spin()) + " " + p.text(strings.TrimPrefix(m.status, "ghx: ")))
	}
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
	render := func(name string, sz [2]int, setup func(*Model)) error {
		m := checkModel(cfg, nil, d)
		m.width, m.height = sz[0], sz[1]
		setup(&m)
		frame, err := safeView(m)
		if err != nil {
			return fmt.Errorf("%s at %dx%d: %w", name, sz[0], sz[1], err)
		}
		out[fmt.Sprintf("%dx%d/%s", sz[0], sz[1], name)] = frame
		return nil
	}
	for _, sz := range sizes {
		for tab := range tabNames {
			if err := render(fmt.Sprintf("%d-%s", tab+1, tabNames[tab]), sz, func(m *Model) { m.tab = tab }); err != nil {
				return out, err
			}
		}
		steps := map[string]func(*Model){
			"5-insights":      func(m *Model) { m.tab, m.insights = tabRepos, true },
			"1-no-profile":    func(m *Model) { d := *m.data; d.Profile = nil; m.data = &d; m.tab = tabProfile },
			"modal-inbox":     func(m *Model) { m.showNotifs = true },
			"modal-help":      func(m *Model) { m.showHelp = true },
			"modal-comment":   func(m *Model) { m.tab = tabIssues; mm, _ := m.startComment(); *m = mm.(Model) },
			"inbox-scrolled":  func(m *Model) { m.showNotifs, m.notifCursor = true, len(m.data.Notifications)-1 },
			"empty":           func(m *Model) { m.data = emptyDashboard(m.data) },
			"1-empty-profile": func(m *Model) { m.data = emptyDashboard(m.data); m.tab = tabProfile },
		}
		for name, setup := range steps {
			if err := render(name, sz, setup); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// emptyDashboard is a brand-new account: no PRs, runs, issues or notifications.
func emptyDashboard(d *gh.Dashboard) *gh.Dashboard {
	var prof *gh.Profile
	if d.Profile != nil {
		prof = &gh.Profile{Login: d.Profile.Login, Avatar: d.Profile.Avatar, CreatedAt: d.Profile.CreatedAt}
	}
	return &gh.Dashboard{User: d.User, Profile: prof, Repos: d.Repos[:1], Contributions: d.Contributions, FetchedAt: d.FetchedAt, Demo: d.Demo}
}

// Snapshot renders a single frame for the given tab and size. With a live client, the repository
// views also load real insights for the first repository.
func Snapshot(cfg *config.Config, client *gh.Client, d *gh.Dashboard, tab, w, h int, notifs, help bool) (string, error) {
	m := checkModel(cfg, client, d)
	m.width, m.height = w, h
	if tab == TabInsights {
		tab, m.insights = tabRepos, true
	}
	m.tab = clamp(tab, 0, len(tabNames)-1)
	m.showNotifs, m.showHelp = notifs, help
	if m.tab == tabRepos && !m.demo() {
		if r, ok := m.selectedRepo(); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			// GitHub computes statistics on first request (HTTP 202); give it a few tries.
			for attempt := 0; attempt < 4; attempt++ {
				a, err := client.FetchRepoAnalytics(ctx, r.NameWithOwner)
				if err != nil {
					m.analyticsErr[r.NameWithOwner] = err.Error()
					break
				}
				m.analytics[r.NameWithOwner] = a
				if !a.StatsPending {
					break
				}
				time.Sleep(3 * time.Second)
			}
		}
	}
	return safeView(m)
}

// checkModel builds an offline model around pre-fetched data for --check / --snapshot.
func checkModel(cfg *config.Config, client *gh.Client, d *gh.Dashboard) Model {
	m := New(cfg, nil, "")
	m.client, m.data = client, d
	m.status = ""
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
