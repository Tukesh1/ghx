package ui

import (
	"fmt"
	"strings"
	"time"

	gh "github.com/Tukesh1/ghx/internal/github"
)

var repoCols = []column{
	{title: "Repository", width: -2, min: 22},
	{title: "Vis", width: 9, drop: 5},
	{title: "Lang", width: 12, drop: 4},
	{title: "Branch & Sync", width: 22, drop: 3},
	{title: "★ Stars", width: 8, right: true},
	{title: "Forks", width: 7, right: true, drop: 2},
	{title: "PRs", width: 5, right: true, drop: 6},
	{title: "Issues", width: 6, right: true, drop: 7},
	{title: "CI", width: 8},
	{title: "Release", width: 10, drop: 8},
	{title: "Updated", width: 8, right: true, drop: 1},
}

func (m Model) repoRows(repos []gh.Repo) [][]string {
	p := m.p
	rows := make([][]string, 0, len(repos))
	for _, r := range repos {
		vis := p.badge(p.Green, "public")
		if r.IsPrivate {
			vis = p.badge(p.Yellow, "private")
		}
		lang := p.dim("—")
		if r.Language != "" {
			lang = p.fg(r.LanguageColor, "●") + " " + p.text(r.Language)
		}
		release := p.dim("—")
		if r.Release != "" {
			release = p.badge(p.TextSecondary, r.Release)
		}
		desc := r.Description
		if desc == "" {
			desc = "no description"
		}
		rows = append(rows, []string{
			cell(m.repoIcon(r)+" "+p.bold(p.Blue, shortRepo(r.NameWithOwner, m.data.User.Login)), p.muted(desc)),
			vis,
			lang,
			cell(p.text(p.ic.Branch+" "+r.SyncBranch), m.syncText(r)),
			p.bold(p.Yellow, p.ic.Star+" "+compact(r.Stars)),
			p.text(compact(r.Forks)),
			p.bold(p.Green, fmt.Sprint(r.OpenPRs)),
			p.bold(p.Yellow, fmt.Sprint(r.OpenIssues)),
			p.ciBadge(r.CIState, ""),
			release,
			p.muted(ago(r.PushedAt) + " ago"),
		})
	}
	return rows
}

// viewRepos renders Tab [4]: repository inventory + an analytics inspector, or the full Insights view.
func (m Model) viewRepos(w, h int) string {
	if m.insights {
		return m.viewInsights(w, h)
	}
	p := m.p
	return m.splitTab(w, h,
		func(lw, h int) string {
			pub, priv, div := 0, 0, 0
			for _, r := range m.data.Repos {
				if r.IsPrivate {
					priv++
				} else {
					pub++
				}
				if r.Ahead > 0 || r.Behind > 0 {
					div++
				}
			}
			chips := m.filterChips(
				[]string{fmt.Sprintf("All (%d)", len(m.data.Repos)), fmt.Sprintf("Public (%d)", pub), fmt.Sprintf("Private (%d)", priv), fmt.Sprintf("Diverged (%d)", div)},
				[]string{p.Blue, p.Green, p.Yellow, p.Purple}, m.pill[kindRepo])
			title := titled(p.ic.RepoPublic+" REPOSITORIES", p.ic.RepoPublic+" REPOSITORIES INVENTORY", chips, lw)
			return m.listPane(kindRepo, title, chips, repoCols, m.repoRows(m.repos()), lw, h)
		},
		m.repoInspector)
}

// analyticsFor returns cached insight data for a repo; demo mode synthesizes it on the fly.
func (m Model) analyticsFor(r gh.Repo) (*gh.RepoAnalytics, string) {
	if m.demo() {
		return gh.MockAnalytics(r), ""
	}
	if a, ok := m.analytics[r.NameWithOwner]; ok {
		return a, ""
	}
	if e, ok := m.analyticsErr[r.NameWithOwner]; ok {
		return nil, "analytics unavailable: " + e
	}
	return nil, m.spin() + " loading analytics…"
}

// repoTiles is the stat-tile set shown for a repository.
func (m Model) repoTiles(r gh.Repo, a *gh.RepoAnalytics) []tile {
	p := m.p
	dash := "—"
	watchers, merged, releases, contributors := dash, dash, dash, dash
	issueSub, prSub, relSub, ctrSub := "open", "open", "published", "top committers"
	if a != nil {
		watchers = compact(a.Watchers)
		merged = compact(a.MergedPRs)
		releases = compact(a.Releases)
		contributors = fmt.Sprint(len(a.Contributors))
		if total := r.OpenIssues + a.ClosedIssues; total > 0 {
			issueSub = fmt.Sprintf("%d%% closed", a.ClosedIssues*100/total)
		}
		if total := a.MergedPRs + a.ClosedPRs; total > 0 {
			prSub = fmt.Sprintf("%d%% merge rate", a.MergedPRs*100/total)
		}
		if r.Release != "" {
			relSub = "latest " + r.Release
		}
		if len(a.Contributors) > 0 {
			ctrSub = "@" + a.Contributors[0].Login
		}
	}
	return []tile{
		{p.ic.Star, "Stars", commaInt(r.Stars), fmt.Sprintf("%s watching", watchers), p.Yellow},
		{p.ic.Fork, "Forks", commaInt(r.Forks), "network", p.Blue},
		{p.ic.IssueOpen, "Issues", fmt.Sprint(r.OpenIssues), issueSub, p.Green},
		{p.ic.PROpen, "Open PRs", fmt.Sprint(r.OpenPRs), prSub, p.Purple},
		{p.ic.PRMerged, "Merged", merged, "PRs all time", p.Purple},
		{p.ic.Tag, "Releases", releases, relSub, p.Cyan},
		{p.ic.Person, "Authors", contributors, ctrSub, p.Pink},
		{p.ic.Eye, "Watchers", watchers, "subscribed", p.TextPrimary},
	}
}

// commitActivityBlock renders the 52-week commit chart sized to w×h (chart rows + month axis).
func (m Model) commitActivityBlock(a *gh.RepoAnalytics, w, h int) []string {
	p := m.p
	if a == nil || len(a.CommitWeeks) == 0 {
		msg := "commit activity unavailable"
		if a != nil && a.StatsPending {
			msg = "GitHub is computing statistics — press r in a moment"
		}
		return []string{p.muted(msg)}
	}
	return m.weeklyChart(a.CommitWeeks, a.WeeksStart, w, h, p.Green)
}

// weeklyChart draws a weekly series as a column chart with a month axis, downsampling to fit w.
func (m Model) weeklyChart(series []int, start time.Time, w, h int, color string) []string {
	p := m.p
	weeks := series
	axisW := chartAxisWidth(weeks) + 1
	if avail := w - axisW; len(weeks) > avail && avail > 0 {
		weeks = downsample(series, avail)
	}
	gap := 0
	if axisW+len(weeks)*2 <= w {
		gap = 1
	}
	chart := p.columnChart(weeks, max(h-1, 1), 1, gap, color)
	perCol := float64(len(series)) / float64(max(len(weeks), 1))
	return append(chart, p.monthAxis(start, len(weeks), 1, gap, chartAxisWidth(weeks), perCol))
}

// contributorsTable renders the top contributors with a share bar.
func (m Model) contributorsTable(a *gh.RepoAnalytics, w, h int) []string {
	p := m.p
	if a == nil || len(a.Contributors) == 0 {
		return []string{p.muted("contributors unavailable")}
	}
	total := 0
	for _, c := range a.Contributors {
		total += c.Commits
	}
	barW := clamp(w/4, 6, 20)
	cols := []column{
		{title: "#", width: 2, right: true},
		{title: "Contributor", width: -1, min: 12},
		{title: "Commits", width: 8, right: true},
		{title: "Share", width: barW, drop: 1},
		{title: "%", width: 4, right: true, drop: 2},
	}
	colors := []string{p.Green, p.Blue, p.Purple, p.Yellow, p.Cyan, p.Pink, p.TextSecondary, p.TextSecondary}
	var rows [][]string
	for i, c := range a.Contributors {
		share := float64(c.Commits) / float64(max(total, 1))
		rows = append(rows, []string{
			p.dim(fmt.Sprint(i + 1)),
			p.fg(p.Cyan, "@"+c.Login),
			p.bold(p.TextPrimary, commaInt(c.Commits)),
			p.shareBar(share, barW, colors[i%len(colors)]),
			p.muted(fmt.Sprintf("%.0f%%", share*100)),
		})
	}
	return p.table(cols, rows, -1, w, h, false)
}

func (m Model) repoInspector(w, h int) string {
	p := m.p
	inner := w - 4
	r, ok := m.selectedRepo()
	if !ok {
		return p.panel("REPOSITORY ANALYTICS", "", []string{p.muted("no repository selected")}, nil, w, h, false)
	}
	a, state := m.analyticsFor(r)

	vis := p.badge(p.Green, "public")
	if r.IsPrivate {
		vis = p.badge(p.Yellow, "private")
	}
	head := m.repoIcon(r) + " " + p.bold(p.TextPrimary, r.NameWithOwner) + " " + vis
	if a != nil && a.IsArchived {
		head += " " + p.badge(p.Yellow, "archived")
	}
	if a != nil && a.IsFork {
		head += " " + p.badge(p.TextSecondary, "fork")
	}
	b := []string{head}
	if r.Description != "" {
		for _, l := range wrap(r.Description, inner, 2) {
			b = append(b, p.muted(l))
		}
	}
	if a != nil {
		var meta []string
		for _, t := range a.Topics {
			meta = append(meta, p.badge(p.Blue, t))
		}
		if a.License != "" {
			meta = append(meta, p.badge(p.TextSecondary, "⚖ "+a.License))
		}
		if len(meta) > 0 {
			b = append(b, fit(strings.Join(meta, " "), inner))
		}
	} else {
		b = append(b, p.fg(p.Yellow, state))
	}
	b = append(b, "")

	perRow := 4
	if inner < 4*15+3 {
		perRow = 3
	}
	tiles := m.repoTiles(r, a)
	if perRow == 3 {
		tiles = append(tiles[:5], tiles[6])
	}
	b = append(b, p.tileGrid(tiles, inner, perRow)...)
	b = append(b, "")

	commits := 0
	if a != nil {
		commits = a.TotalCommits52w()
	}
	b = append(b, spread(p.bold(p.TextSecondary, "COMMIT ACTIVITY · 52 WEEKS"), p.fg(p.Green, fmt.Sprintf("%s commits", commaInt(commits))), inner))
	b = append(b, m.commitActivityBlock(a, inner, 5)...)
	b = append(b, "")
	b = append(b, spread(p.bold(p.TextSecondary, "TOP CONTRIBUTORS"), p.muted(r.DefaultBranch+" · "+m.syncTextPlain(r)), inner))
	b = append(b, m.contributorsTable(a, inner, 7)...)

	footer := append([]string{p.line(strings.Repeat("─", inner))}, m.buttons(inner,
		[3]string{"⏎", p.ic.Pulse + " Full Insights", p.Green},
		[3]string{"c", "gh repo clone " + shortRepo(r.NameWithOwner, ""), p.Blue},
		[3]string{"o", "Open", p.TextSecondary},
	)...)
	return p.panel(titled("REPOSITORY ANALYTICS", "REPOSITORY ANALYTICS & COMMIT ACTIVITY", "#", w), p.fg(p.Blue, shortRepo(r.NameWithOwner, m.data.User.Login)), b, footer, w, h, false)
}

// syncTextPlain is syncText without color, for compact captions.
func (m Model) syncTextPlain(r gh.Repo) string {
	switch {
	case r.Ahead == 0 && r.Behind == 0:
		return "in sync"
	default:
		return fmt.Sprintf("%s ↑%d ↓%d", r.SyncBranch, r.Ahead, r.Behind)
	}
}

// fmtSize renders a KB count as KB/MB/GB.
func fmtSize(kb int) string {
	switch {
	case kb >= 1<<20:
		return fmt.Sprintf("%.1f GB", float64(kb)/float64(1<<20))
	case kb >= 1<<10:
		return fmt.Sprintf("%.1f MB", float64(kb)/float64(1<<10))
	default:
		return fmt.Sprintf("%d KB", kb)
	}
}

func fmtAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	years := time.Since(t).Hours() / 24 / 365
	if years >= 1 {
		return fmt.Sprintf("%.1fy ago", years)
	}
	return ago(t) + " ago"
}
