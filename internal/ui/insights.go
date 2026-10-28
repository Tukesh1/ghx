package ui

import (
	"fmt"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
)

// viewInsights renders the full-screen Repository Insights dashboard for the selected repo:
// a header with stat tiles, a row of graphs (commit activity, code frequency, traffic) and a
// row of tables (contributors, languages, health & recent commits).
func (m Model) viewInsights(w, h int) string {
	p := m.p
	r, ok := m.selectedRepo()
	if !ok {
		return p.panel("REPOSITORY INSIGHTS", "", []string{p.muted("no repository selected")}, nil, w, h, true)
	}
	a, state := m.analyticsFor(r)

	header := m.insightsHeader(r, a, state, w)
	headerH := strings.Count(header, "\n") + 1
	gap := 0
	if h >= 30 {
		gap = 1
	}
	rest := max(h-headerH-2*gap, 2*panelChrome+2)
	rowA := rest * 55 / 100
	rowB := rest - rowA

	a1 := w * 44 / 100
	a2 := w * 30 / 100
	a3 := w - a1 - a2 - 2
	b1 := w * 32 / 100
	b2 := w * 30 / 100
	b3 := w - b1 - b2 - 2

	graphs := lipgloss.JoinHorizontal(lipgloss.Top,
		m.commitPanel(a, a1, rowA), " ", m.codeFreqPanel(a, a2, rowA), " ", m.trafficPanel(a, a3, rowA))
	tables := lipgloss.JoinHorizontal(lipgloss.Top,
		m.contributorsPanel(a, b1, rowB), " ", m.languagesPanel(r, a, b2, rowB), " ", m.healthPanel(r, a, b3, rowB))
	parts := []string{header}
	for _, band := range []string{graphs, tables} {
		if gap > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, band)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) insightsHeader(r gh.Repo, a *gh.RepoAnalytics, state string, w int) string {
	p := m.p
	inner := w - 4
	desc := r.Description
	if desc == "" {
		desc = "No description."
	}
	meta := []string{}
	if a != nil {
		if a.License != "" {
			meta = append(meta, p.muted("⚖ ")+p.text(a.License))
		}
		meta = append(meta,
			p.muted("created ")+p.text(fmtAge(a.CreatedAt)),
			p.muted("size ")+p.text(fmtSize(a.DiskKB)),
			p.muted("branches ")+p.text(fmt.Sprint(a.Branches)),
		)
	}
	meta = append(meta, p.muted("default ")+p.fg(p.Cyan, p.ic.Branch+" "+r.DefaultBranch), p.muted("pushed ")+p.text(ago(r.PushedAt)+" ago"))
	if r.Language != "" {
		meta = append([]string{p.fg(r.LanguageColor, "●") + " " + p.text(r.Language)}, meta...)
	}
	topics := ""
	if a != nil {
		for _, t := range a.Topics {
			topics += p.badge(p.Blue, t) + " "
		}
	}
	lines := []string{
		spread(p.text(desc), topics, inner),
		strings.Join(meta, p.dim("  ·  ")),
	}
	if a == nil {
		lines = append(lines, p.fg(p.Yellow, state))
	}
	lines = append(lines, "")
	perRow := 8
	if inner < 8*16+7 {
		perRow = 4
	}
	lines = append(lines, p.tileGrid(m.repoTiles(r, a), inner, perRow)...)

	vis := p.badge(p.Green, "public")
	if r.IsPrivate {
		vis = p.badge(p.Yellow, "private")
	}
	title := m.repoIcon(r) + " " + r.NameWithOwner + " · REPOSITORY INSIGHTS"
	right := vis + "  " + p.kbd("j/k") + p.muted(" repo  ") + p.kbd("esc") + p.muted(" back")
	return p.panel(title, right, lines, nil, w, len(lines)+panelChrome, true)
}

func (m Model) commitPanel(a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome
	var body []string
	right := ""
	if a != nil && len(a.CommitWeeks) > 0 {
		total := a.TotalCommits52w()
		peak, peakAt := 0, 0
		for i, c := range a.CommitWeeks {
			if c > peak {
				peak, peakAt = c, i
			}
		}
		right = p.fg(p.Green, commaInt(total)+" commits")
		body = m.weeklyChart(a.CommitWeeks, a.WeeksStart, inner, room-1, p.Green)
		body = append(body, p.muted(fmt.Sprintf("avg %.1f/week · peak %d (week of %s) · last 4w %d",
			float64(total)/float64(len(a.CommitWeeks)), peak, a.WeeksStart.AddDate(0, 0, 7*peakAt).Format("Jan 2"), sumLast(a.CommitWeeks, 4))))
	} else {
		body = m.commitActivityBlock(a, inner, room)
	}
	return p.panel(p.fg(p.Green, p.ic.Commit)+" COMMIT ACTIVITY · 52W", right, body, nil, w, h, false)
}

func sumLast(v []int, n int) int {
	t := 0
	for i := max(len(v)-n, 0); i < len(v); i++ {
		t += v[i]
	}
	return t
}

func (m Model) codeFreqPanel(a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome
	title := p.fg(p.Purple, p.ic.File) + " CODE FREQUENCY"
	if a == nil || len(a.CodeFreq) == 0 {
		msg := "code frequency unavailable"
		if a != nil && a.StatsPending {
			msg = "GitHub is computing statistics"
		}
		return p.panel(title, "", []string{p.muted(msg)}, nil, w, h, false)
	}
	// Show as many of the most recent weeks as fit, one column each.
	weeks := a.CodeFreq
	if len(weeks) > inner {
		weeks = weeks[len(weeks)-inner:]
	}
	title += fmt.Sprintf(" · %dW", len(weeks))
	adds := make([]int, len(weeks))
	dels := make([]int, len(weeks))
	ta, td := 0, 0
	for i, d := range weeks {
		adds[i], dels[i] = d.Add, d.Del
		ta += d.Add
		td += d.Del
	}
	colw, gap := columnLayout(len(adds), inner)
	chartRows := max(room-2, 2)
	up := max(chartRows*6/10, 1)
	down := max(chartRows-up, 1)
	body := p.divergingChart(adds, dels, up, down, colw, gap)
	body = append(body, p.fg(p.Green, "+"+compact(ta))+p.dim(" / ")+p.fg(p.Red, "−"+compact(td))+p.muted(fmt.Sprintf("  net %s%s lines", map[bool]string{true: "+", false: "−"}[ta >= td], commaInt(abs(ta-td)))))
	right := p.fg(p.Green, "▇ add") + " " + p.fg(p.Red, "▇ del")
	if width(title)+width(right)+5 > w {
		title = p.fg(p.Purple, p.ic.File) + " CODE FREQUENCY"
	}
	return p.panel(title, right, body, nil, w, h, false)
}

func (m Model) trafficPanel(a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome
	title := p.fg(p.Cyan, p.ic.Eye) + " TRAFFIC · 14D"
	if a == nil || !a.TrafficOK {
		return p.panel(title, "", []string{
			p.muted("Traffic stats need push access"),
			p.muted("to this repository."),
		}, nil, w, h, false)
	}
	chartH := clamp((room-4)/2, 1, 5)
	colw, cgap := columnLayout(len(a.Views), inner-chartAxisWidth(a.Views))
	var body []string
	body = append(body, spread(p.bold(p.TextSecondary, "VIEWS"), p.bold(p.Cyan, commaInt(a.ViewsTotal))+p.muted(fmt.Sprintf(" · %d unique", a.ViewsUnique)), inner))
	body = append(body, p.columnChart(a.Views, chartH, colw, cgap, p.Cyan)...)
	body = append(body, spread(p.bold(p.TextSecondary, "CLONES"), p.bold(p.Purple, commaInt(a.ClonesTotal))+p.muted(fmt.Sprintf(" · %d unique", a.ClonesUnique)), inner))
	body = append(body, p.columnChart(a.Clones, chartH, colw, cgap, p.Purple)...)
	if left := room - len(body) - 1; left >= 3 && len(a.Referrers) > 0 {
		cols := []column{{title: "Referrer", width: -1}, {title: "Views", width: 6, right: true}, {title: "Uniq", width: 5, right: true, drop: 1}}
		var rows [][]string
		for _, ref := range a.Referrers {
			rows = append(rows, []string{p.text(ref.Source), p.bold(p.Cyan, commaInt(ref.Count)), p.muted(fmt.Sprint(ref.Uniques))})
		}
		body = append(body, "")
		body = append(body, p.table(cols, rows, -1, inner, left, false)...)
	}
	return p.panel(title, "", body, nil, w, h, false)
}

func (m Model) contributorsPanel(a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	right := ""
	if a != nil {
		right = p.muted(fmt.Sprintf("top %d", len(a.Contributors)))
	}
	return p.panel(p.fg(p.Pink, p.ic.Person)+" TOP CONTRIBUTORS", right, m.contributorsTable(a, w-4, h-panelChrome), nil, w, h, false)
}

func (m Model) languagesPanel(r gh.Repo, a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	inner := w - 4
	title := p.fg(p.Pink, p.ic.Code) + " LANGUAGES"
	langs := []gh.LangStat{}
	if a != nil {
		langs = a.Languages
	}
	if len(langs) == 0 && r.Language != "" {
		langs = []gh.LangStat{{Name: r.Language, Color: r.LanguageColor, Percent: 100}}
	}
	if len(langs) == 0 {
		return p.panel(title, "", []string{p.muted("no language data")}, nil, w, h, false)
	}
	body := []string{RenderLanguageBar(inner, langs), ""}
	barW := clamp(inner/3, 6, 18)
	cols := []column{
		{title: "Language", width: -1, min: 10},
		{title: "Share", width: barW, drop: 1},
		{title: "%", width: 5, right: true},
		{title: "Size", width: 8, right: true, drop: 2},
	}
	var rows [][]string
	for _, l := range langs {
		size := "—"
		if l.Bytes > 0 {
			size = fmtSize(l.Bytes / 1024)
		}
		rows = append(rows, []string{
			p.fg(l.Color, "●") + " " + p.text(l.Name),
			p.shareBar(l.Percent/100, barW, l.Color),
			p.bold(p.TextPrimary, fmt.Sprintf("%.1f", l.Percent)),
			p.muted(size),
		})
	}
	body = append(body, p.table(cols, rows, -1, inner, h-panelChrome-2, false)...)
	return p.panel(title, p.muted(plural(len(langs), "language")), body, nil, w, h, false)
}

func (m Model) healthPanel(r gh.Repo, a *gh.RepoAnalytics, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome
	barW := clamp(inner-24, 6, 30)
	var body []string
	if a != nil {
		rate := func(label string, num, den int, color string) string {
			pct := 0.0
			if den > 0 {
				pct = float64(num) * 100 / float64(den)
			}
			return fit(p.muted(label), 14) + p.progress(pct, barW, color) + " " + p.bold(color, fmt.Sprintf("%3.0f%%", pct))
		}
		body = append(body,
			rate("Issues closed", a.ClosedIssues, a.ClosedIssues+r.OpenIssues, p.Green),
			rate("PR merge rate", a.MergedPRs, a.MergedPRs+a.ClosedPRs, p.Purple),
			rate("CI on "+r.DefaultBranch, boolInt(r.CIState == gh.StateSuccess), 1, p.stateColor(r.CIState)),
			"",
		)
	}
	cols := []column{
		{title: "SHA", width: 7},
		{title: "Message", width: -1, min: 12},
		{title: "Author", width: 12, drop: 1},
		{title: "Age", width: 4, right: true, drop: 2},
	}
	commits := r.RecentCommits
	if len(commits) == 0 && r.LatestSHA != "" {
		commits = []gh.Commit{{SHA: r.LatestSHA, Message: r.LatestMessage, Author: r.LatestAuthor, Date: r.PushedAt}}
	}
	var rows [][]string
	for _, c := range commits {
		rows = append(rows, []string{p.fg(p.Yellow, c.SHA), p.text(c.Message), p.fg(p.Cyan, c.Author), p.muted(ago(c.Date))})
	}
	body = append(body, p.table(cols, rows, -1, inner, room-len(body), false)...)
	return p.panel(p.fg(p.Green, p.ic.CheckPass)+" HEALTH & RECENT COMMITS", "", body, nil, w, h, false)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (p palette) stateColor(state string) string {
	switch state {
	case gh.StateSuccess:
		return p.Green
	case gh.StateFailure:
		return p.Red
	case gh.StatePending:
		return p.Yellow
	default:
		return p.TextSecondary
	}
}
