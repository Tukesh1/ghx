package ui

import (
	"fmt"
	"strings"
	"time"

	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
)

// overviewGeom holds the Command Center band heights and column split, shared by View and mouse hit-testing.
type overviewGeom struct {
	t1, t2, t3 int
	gap        int
	lw, rw     int
}

// y offsets of each band within the body (the KPI strip lives in the sticky chrome above it).
func (g overviewGeom) t1Y() int { return 0 }
func (g overviewGeom) t2Y() int { return g.t1 + g.gap }

func overviewLayout(w, h int) overviewGeom {
	g := overviewGeom{}
	if h >= 24 {
		g.gap = 1
	}
	// The telemetry row only appears when both table tiers keep ~7 visible rows each.
	g.t3 = 9
	if h-g.t3-2*g.gap < 20 {
		g.t3 = 0
	}
	mid := h - g.gap
	if g.t3 > 0 {
		mid -= g.t3 + g.gap
	}
	g.t1 = mid * 55 / 100
	g.t2 = mid - g.t1
	g.lw, g.rw = split(w, 58)
	return g
}

// viewOverview renders Tab [1]: Tier 1 PRs/CI, Tier 2 Repos/Issues, Tier 3 telemetry.
func (m Model) viewOverview(w, h int) string {
	g := overviewLayout(w, h)
	bands := []string{
		hjoin(m.prPanel(g.lw, g.t1, m.focus == kindPR), m.runsPanel(g.rw, g.t1, m.focus == kindRun)),
		hjoin(m.reposPanel(g.lw, g.t2, m.focus == kindRepo), m.issuesPanel(g.rw, g.t2, m.focus == kindIssue)),
	}
	if g.t3 > 0 {
		bands = append(bands, m.telemetryStrip(w, g.t3))
	}
	var parts []string
	for i, b := range bands {
		if i > 0 && g.gap > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, b)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// titled picks the long title when it fits next to the right-hand controls, else the short one.
func titled(short, long, right string, w int) string {
	if width(long)+width(right)+5 <= w {
		return long
	}
	return short
}

// filterChips renders title-bar filter buttons; the active one is solid, the rest tinted.
func (m Model) filterChips(names, colors []string, active int) string {
	var out []string
	for i, n := range names {
		if i == active {
			out = append(out, m.p.pill(colors[i], n))
		} else {
			out = append(out, m.p.badge(colors[i], n))
		}
	}
	return strings.Join(out, " ")
}

func (m Model) link(s string) string { return m.p.fg(m.p.Blue, s) }

var ovPRCols = []column{
	{title: "PR", width: 7},
	{title: "Repository & Branch", width: -2, drop: 2, min: 16},
	{title: "Title & Author", width: -3, min: 21},
	{title: "CI Checks", width: 15},
	{title: "Review / Merge", width: 15, drop: 3},
	{title: "Diff & Files", width: 11, drop: 4},
	{title: "Quick Action", width: 11, drop: 1},
}

func (m Model) ovPRRows(prs []gh.PullRequest) [][]string {
	p := m.p
	rows := make([][]string, 0, len(prs))
	for _, pr := range prs {
		by := "by " + pr.Author + " · " + ago(pr.UpdatedAt)
		if pr.NeedsMyReview {
			by += " · " + p.fg(p.Yellow, "your review")
		}
		rows = append(rows, []string{
			p.prIcon(pr) + " " + p.bold(p.TextPrimary, fmt.Sprintf("#%d", pr.Number)),
			cell(p.bold(p.Blue, shortRepo(pr.Repo, m.data.User.Login)), p.muted(p.ic.Branch+" "+pr.HeadRef)),
			cell(p.text(pr.Title), p.muted(by)),
			p.ciBadge(pr.CIState, checksLabel(pr)),
			p.reviewBadge(pr),
			cell(p.diff(pr.Additions, pr.Deletions), p.muted(fmt.Sprintf("%d files", pr.ChangedFiles))),
			p.quickAction(pr),
		})
	}
	return rows
}

func (m Model) prPanel(w, h int, focused bool) string {
	p := m.p
	review := 0
	for _, pr := range m.data.ActionPRs() {
		if pr.NeedsMyReview {
			review++
		}
	}
	chips := m.filterChips(
		[]string{fmt.Sprintf("All (%d)", len(m.data.ActionPRs())), fmt.Sprintf("Needs Review (%d)", review), "Authored (@me)"},
		[]string{p.Blue, p.Yellow, p.Green}, m.quick)
	if width(chips) > w/2 {
		chips = m.filterChips([]string{"All", "Review", "@me"}, []string{p.Blue, p.Yellow, p.Green}, m.quick)
	}
	title := titled(p.ic.PROpen+" PULL REQUESTS REQUIRING ACTION", p.ic.PROpen+" PULL REQUESTS REQUIRING ACTION (REVIEW QUEUE & READY TO MERGE)", chips, w)
	body := p.table(ovPRCols, m.ovPRRows(m.prs()), m.cursor[kindPR], w-4, h-panelChrome, focused)
	return p.panel(title, chips, body, nil, w, h, focused)
}

// runStateBadge is the compact Pass / Fail / Run chip used in run tables.
func (p palette) runStateBadge(r gh.WorkflowRun) string {
	switch r.State() {
	case gh.StateSuccess:
		return p.badge(p.Green, p.ic.CheckPass+" Pass")
	case gh.StateFailure:
		return p.badge(p.Red, p.ic.CheckFail+" Fail")
	default:
		return p.badge(p.Yellow, p.ic.Running+" Run")
	}
}

var ovRunCols = []column{
	{title: "State", width: 8},
	{title: "Workflow & Repo", width: -1, min: 20},
	{title: "Branch / Commit", width: 16, drop: 3},
	{title: "Duration", width: 8, right: true, drop: 1},
	{title: "Action", width: 10, drop: 2},
}

func runDuration(r gh.WorkflowRun) time.Duration {
	if r.Status != "completed" && !r.StartedAt.IsZero() {
		return time.Since(r.StartedAt).Round(time.Second)
	}
	return r.Duration
}

func (m Model) ovRunRows(runs []gh.WorkflowRun) [][]string {
	p := m.p
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		action := p.muted(ago(r.StartedAt) + " ago")
		if r.State() == gh.StateFailure {
			action = p.badge(p.Red, p.ic.Running+" Re-run")
		}
		d := p.text(dur(runDuration(r)))
		if r.State() == gh.StatePending {
			d = p.fg(p.Yellow, dur(runDuration(r)))
		}
		rows = append(rows, []string{
			p.runStateBadge(r),
			cell(p.bold(p.TextPrimary, r.Name), p.fg(p.Blue, shortRepo(r.Repo, m.data.User.Login))),
			cell(p.muted(r.Branch), p.dim(p.ic.Commit+" "+r.SHA)),
			d,
			action,
		})
	}
	return rows
}

// failingRun returns the first failing run with a captured log.
func (m Model) failingRun() (gh.WorkflowRun, bool) {
	for _, r := range m.data.Runs {
		if r.State() == gh.StateFailure && len(r.FailureLog) > 0 {
			return r, true
		}
	}
	return gh.WorkflowRun{}, false
}

// keyLogLines picks up to n diagnostic lines (panics, FAIL, errors) from a log, topped up
// with the log tail, preserving original order.
func keyLogLines(log []string, n int) []string {
	keep := map[int]bool{}
	for _, kw := range []string{"panic", "--- fail", "##[error]", "error", "fail"} {
		for i, l := range log {
			if len(keep) < n && strings.Contains(strings.ToLower(l), kw) {
				keep[i] = true
			}
		}
	}
	// Top up from the tail, skipping echoed step inputs such as "env:" or "  KEY: value".
	for i := len(log) - 1; i >= 0 && len(keep) < n; i-- {
		l := log[i]
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasSuffix(strings.TrimSpace(l), ":") {
			continue
		}
		keep[i] = true
	}
	picked := make([]string, 0, n)
	for i, l := range log {
		if keep[i] {
			picked = append(picked, strings.TrimSpace(strings.TrimPrefix(l, "##[error]")))
		}
	}
	return picked
}

// logBoxLines is how many rows the inline failing-log snippet takes in a runs panel of the given inner height.
func (m Model) logBoxLines(inner int) int {
	if _, ok := m.failingRun(); !ok {
		return 0
	}
	switch {
	case inner >= 12:
		return 4
	case inner >= 8:
		return 3
	case inner >= 5:
		return 2
	default:
		return 0
	}
}

func (m Model) runsPanel(w, h int, focused bool) string {
	p := m.p
	inner := h - panelChrome
	nLog := m.logBoxLines(inner)
	var logBox []string
	if fr, ok := m.failingRun(); ok && nLog > 0 {
		job := ""
		for _, j := range fr.Jobs {
			if j.Conclusion == "failure" {
				job = " · " + j.Name
			}
		}
		head := spread(p.bold(p.Red, fmt.Sprintf("%s FAILING LOG: %s (Run #%d%s)", p.ic.CheckFail, shortRepo(fr.Repo, m.data.User.Login), fr.ID, job)), p.fg(p.Cyan, "Full Log →"), w-6)
		logBox = append(logBox, head)
		for _, l := range keyLogLines(fr.FailureLog, nLog-1) {
			logBox = append(logBox, p.muted(l))
		}
		logBox = p.inset(logBox, w-4)
	}
	body := p.table(ovRunCols, m.ovRunRows(m.runs()), m.cursor[kindRun], w-4, inner-len(logBox), focused)
	for len(body) < inner-len(logBox) {
		body = append(body, "")
	}
	body = append(body, logBox...)
	right := m.link("All Runs →")
	title := titled(p.ic.Play+" LIVE ACTIONS CI/CD", p.ic.Play+" LIVE ACTIONS CI/CD & BRANCH BUILDS", right, w)
	return p.panel(title, right, body, nil, w, h, focused)
}

var ovRepoCols = []column{
	{title: "Repository", width: -1, min: 14},
	{title: "Branch & Sync", width: 18, drop: 2},
	{title: "Latest Commit on Default Branch", width: -1, drop: 1},
	{title: "PRs / Issues", width: 15, drop: 5},
	{title: "CI", width: 8},
	{title: "★ Stars", width: 8, right: true, drop: 4},
	{title: "Pushed", width: 6, right: true, drop: 3},
}

// syncText renders "↑7 ahead ↓2 behind" (or "✓ synced") colored like the design.
func (m Model) syncText(r gh.Repo) string {
	p := m.p
	switch {
	case r.Ahead == 0 && r.Behind == 0:
		return p.muted(p.ic.CheckPass + " synced")
	case r.Behind == 0:
		return p.bold(p.Green, fmt.Sprintf("%s%d ahead", p.ic.Ahead, r.Ahead))
	case r.Ahead == 0:
		return p.bold(p.Yellow, fmt.Sprintf("%s%d behind", p.ic.Behind, r.Behind))
	default:
		return p.bold(p.Green, fmt.Sprintf("%s%d ahead", p.ic.Ahead, r.Ahead)) + " " + p.bold(p.Yellow, fmt.Sprintf("%s%d behind", p.ic.Behind, r.Behind))
	}
}

func (m Model) repoIcon(r gh.Repo) string {
	if r.IsPrivate {
		return m.p.fg(m.p.Yellow, m.p.ic.RepoPrivate)
	}
	return m.p.fg(m.p.Blue, m.p.ic.RepoPublic)
}

func (m Model) ovRepoRows(repos []gh.Repo) [][]string {
	p := m.p
	rows := make([][]string, 0, len(repos))
	for _, r := range repos {
		meta := ""
		if r.Language != "" {
			meta = p.fg(r.LanguageColor, "●") + " " + p.muted(r.Language)
		}
		if r.Release != "" {
			meta += p.muted(" · " + r.Release)
		}
		rows = append(rows, []string{
			cell(m.repoIcon(r)+" "+p.bold(p.Blue, shortRepo(r.NameWithOwner, m.data.User.Login)), meta),
			cell(p.text(p.ic.Branch+" "+r.SyncBranch), m.syncText(r)),
			p.dim(p.ic.Commit) + " " + p.fg(p.Yellow, r.LatestSHA) + " " + p.text(r.LatestMessage),
			p.badge(p.Green, fmt.Sprintf("%d PRs", r.OpenPRs)) + " " + p.badge(p.Yellow, fmt.Sprintf("%d Iss", r.OpenIssues)),
			p.ciBadge(r.CIState, ""),
			p.bold(p.TextPrimary, p.ic.Star+" "+compact(r.Stars)),
			p.muted(ago(r.PushedAt)),
		})
	}
	return rows
}

func (m Model) reposPanel(w, h int, focused bool) string {
	p := m.p
	right := m.link(fmt.Sprintf("Inspect All %d Repos →", len(m.data.Repos)))
	title := titled(p.ic.RepoPublic+" ACTIVE REPOSITORIES", p.ic.RepoPublic+" ACTIVE REPOSITORIES (BRANCH STATUS, LATEST COMMIT & OPEN WORK)", right, w)
	body := p.table(ovRepoCols, m.ovRepoRows(m.repos()), m.cursor[kindRepo], w-4, h-panelChrome, focused)
	return p.panel(title, right, body, nil, w, h, focused)
}

var ovIssueCols = []column{
	{title: "Issue", width: 7},
	{title: "Title & Repo", width: -1, min: 22},
	{title: "Labels & Priority", width: 12},
	{title: "Linked PR", width: 9, drop: 1},
	{title: "Updated", width: 7, right: true, drop: 2},
}

func (m Model) ovIssueRows(issues []gh.Issue) [][]string {
	p := m.p
	rows := make([][]string, 0, len(issues))
	for _, is := range issues {
		linked := p.dim("—")
		if is.LinkedPR > 0 {
			linked = p.fg(p.Purple, fmt.Sprintf("PR #%d", is.LinkedPR))
		}
		rows = append(rows, []string{
			p.bold(p.Yellow, fmt.Sprintf("%s #%d", p.ic.IssueOpen, is.Number)),
			cell(p.text(is.Title), p.fg(p.Blue, shortRepo(is.Repo, m.data.User.Login))),
			p.priorityBadge(is.Priority),
			linked,
			p.muted(ago(is.UpdatedAt) + " ago"),
		})
	}
	return rows
}

func (m Model) issuesPanel(w, h int, focused bool) string {
	p := m.p
	right := m.link(fmt.Sprintf("All %d Issues →", len(m.data.Issues)))
	title := titled(p.ic.IssueOpen+" ASSIGNED ISSUES (@ME)", p.ic.IssueOpen+" ASSIGNED ISSUES & TRIAGE QUEUE (@ME)", right, w)
	body := p.table(ovIssueCols, m.ovIssueRows(m.issues()), m.cursor[kindIssue], w-4, h-panelChrome, focused)
	return p.panel(title, right, body, nil, w, h, focused)
}

// telemetryStrip renders Tier 3: heatmap (42%), CI chart (33%), languages + Dependabot (25%).
func (m Model) telemetryStrip(w, h int) string {
	// The heatmap panel is sized to show all 53 weeks (plus weekday labels when there is room)
	// rather than a fixed share, so it is never cut off.
	weeks := len(m.data.Weeks())
	a := clamp(heatmapBlockWidth(weeks, true)+4, w*40/100, w*50/100)
	if heatmapBlockWidth(weeks, true)+4 > a {
		a = clamp(heatmapBlockWidth(weeks, false)+4, w*40/100, w*50/100)
	}
	rest := w - a - 2
	b := rest * 57 / 100
	c := rest - b
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.heatmapPanel(a, h), " ", m.ciPanel(b, h), " ", m.langPanel(c, h))
}

// heatmapLabelW is the width of the "Mon " weekday gutter.
const heatmapLabelW = 4

// heatmapBlockWidth is the inset width needed for n weeks, with or without weekday labels.
func heatmapBlockWidth(n int, labels bool) int {
	w := n + 2
	if labels {
		w += heatmapLabelW
	}
	return w
}

func (m Model) heatmapPanel(w, h int) string {
	p := m.p
	inner := w - 4
	weeks, starts := m.data.WeeksWithStarts()
	labels := heatmapBlockWidth(len(weeks), true) <= inner
	if !labels && len(weeks) > inner-2 && inner > 2 {
		cut := len(weeks) - (inner - 2)
		weeks, starts = weeks[cut:], starts[cut:]
	}
	pal := p.heatmapPalette()
	grid := strings.Split(RenderCompactHeatmap(weeks, pal), "\n")

	// Month labels above the grid, GitHub style.
	months := []rune(strings.Repeat(" ", len(weeks)))
	next := 0
	for i, st := range starts {
		if i == 0 || st.Month() != starts[i-1].Month() {
			name := []rune(st.Format("Jan"))
			if i >= next && i+len(name) <= len(months) {
				copy(months[i:], name)
				next = i + len(name) + 1
			}
		}
	}
	block := []string{p.dim(string(months))}
	block = append(block, grid...)
	if labels {
		day := []string{"Mon ", "Wed ", "Fri ", "    "}
		block[0] = strings.Repeat(" ", heatmapLabelW) + block[0]
		for i := range grid {
			block[i+1] = p.dim(day[i]) + block[i+1]
		}
	}
	bw := heatmapBlockWidth(len(weeks), labels)
	pad := strings.Repeat(" ", max((inner-bw)/2, 0))
	var body []string
	for _, l := range p.inset(block, bw) {
		body = append(body, pad+l)
	}

	bd := m.data.Breakdown
	total := max(bd.Commits+bd.Reviews+bd.PRs+bd.Issues, 1)
	pct := func(n int) string { return fmt.Sprintf("%d%%", n*100/total) }
	mix := p.muted("Commits ") + p.bold(p.Green, pct(bd.Commits)) + "  " +
		p.muted("Reviews ") + p.bold(p.Blue, pct(bd.Reviews)) + "  " +
		p.muted("PRs ") + p.bold(p.Purple, pct(bd.PRs)) + "  " +
		p.muted("Issues ") + p.bold(p.Yellow, pct(bd.Issues))
	var legend string
	for _, c := range pal {
		legend += p.fg(c, "■")
	}
	legend = p.dim("Less ") + legend + p.dim(" More")
	if width(mix)+width(legend)+2 > inner {
		mix = p.muted("Commits ") + p.bold(p.Green, pct(bd.Commits)) + "  " + p.muted("Reviews ") + p.bold(p.Blue, pct(bd.Reviews))
	}
	body = append(body, spread(mix, legend, inner))

	days := max(len(m.data.Contributions), 1)
	right := p.fg(p.Green, fmt.Sprintf("%dd streak · %.1f/day avg", m.data.Streak(), float64(m.data.TotalContributions())/float64(days)))
	title := p.fg(p.Green, "■") + " " + fmt.Sprintf("CONTRIBUTION ACTIVITY (%s IN 52W)", commaInt(m.data.TotalContributions()))
	if width(title)+width(right)+5 > w {
		title = p.fg(p.Green, "■") + " " + fmt.Sprintf("CONTRIBUTIONS (%s)", commaInt(m.data.TotalContributions()))
		right = p.fg(p.Green, fmt.Sprintf("%dd streak", m.data.Streak()))
	}
	return p.panel(title, right, body, nil, w, h, false)
}

func (m Model) ciPanel(w, h int) string {
	p := m.p
	inner := w - 4
	days := m.data.CIHistory
	title := p.fg(p.Yellow, p.ic.Zap) + " 14D CI PASS RATE & DURATION"
	if len(days) == 0 {
		return p.panel(title, "", []string{p.muted("no workflow history")}, nil, w, h, false)
	}
	per := max(inner/len(days), 1)
	gap := 1
	if per < 2 {
		gap = 0
	}
	colw := clamp(per-gap, 1, 3)
	chartH := clamp(h-panelChrome-1, 1, 6)
	body := p.stackedBars(days, chartH, colw, gap)

	var axis strings.Builder
	for i, d := range days {
		label := d.Date.Weekday().String()[:1]
		if i == len(days)-1 {
			label = "•"
		}
		axis.WriteString(fit(label, colw+gap))
	}
	body = append(body, p.dim(axis.String()))

	passed, failed := 0, 0
	for _, d := range days {
		passed += d.Passed
		failed += d.Failed
	}
	rate := 0.0
	if passed+failed > 0 {
		rate = float64(passed) * 100 / float64(passed+failed)
	}
	color := p.Green
	if rate < 90 {
		color = p.Yellow
	}
	if rate < 75 {
		color = p.Red
	}
	right := p.fg(color, fmt.Sprintf("%.1f%% Pass · P50 %s", rate, dur(gh.MedianDuration(days))))
	if width(right)+width(title)+5 > w {
		title = p.fg(p.Yellow, p.ic.Zap) + " 14D CI"
	}
	return p.panel(title, right, body, nil, w, h, false)
}

func (m Model) langPanel(w, h int) string {
	p := m.p
	inner := w - 4
	body := []string{RenderLanguageBar(inner, m.data.Languages), p.langLegend(m.data.Languages, inner), p.line(strings.Repeat("─", inner))}
	right := p.badge(p.Green, p.ic.Shield+" No CVEs")
	if n := len(m.data.Alerts); n > 0 {
		right = p.badge(p.Red, fmt.Sprintf("%d CVE Alerts", n))
	}
	keys := []string{"u", "U"}
	var rows []string
	for i, a := range m.data.Alerts {
		if i >= 2 {
			break
		}
		sev, color := "MOD", p.Yellow
		switch strings.ToLower(a.Severity) {
		case "critical":
			sev, color = "CRIT", p.Red
		case "high":
			sev, color = "HIGH", p.Yellow
		}
		left := p.bold(color, sev) + " " + p.text(shortPkg(a.Package))
		bump := p.kbd(keys[i]) + p.badge(p.Green, "Bump "+a.PatchedVersion)
		if width(left)+width(bump)+1 > inner-2 {
			bump = p.badge(p.Green, keys[i]+" "+p.ic.Ahead+a.PatchedVersion)
		}
		rows = append(rows, spread(left, bump, inner-2))
	}
	if len(rows) == 0 {
		rows = append(rows, p.fg(p.Green, p.ic.Shield+" no open Dependabot alerts"))
	}
	body = append(body, p.inset(rows, inner)...)
	title := p.fg(p.Pink, p.ic.Code) + " LANGUAGES & DEPENDABOT"
	if width(title)+width(right)+5 > w {
		title = p.fg(p.Pink, p.ic.Code) + " STACK"
	}
	return p.panel(title, right, body, nil, w, h, false)
}

// shortPkg trims long module paths (golang.org/x/net → x/net) for the compact CVE rows.
func shortPkg(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return name
}
