package ui

import (
	"fmt"
	"sort"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// kpi is one summary card; goTo is the tab a click on the card opens.
type kpi struct {
	label  string
	short  string
	value  string
	detail string
	brief  string
	dcolor string
	accent string
	badge  string
	extra  func(w int) string // third line on tall terminals: a fact, bar or sparkline
	alert  bool
	goTo   int
}

func (m Model) kpis() []kpi {
	p := m.p
	d := m.data
	failing, running := d.RunCounts()
	approved := 0
	ciPass, ciFail, ciRun, changes := 0, 0, 0, 0
	for _, pr := range d.MyPRs {
		if pr.ReviewDecision == gh.ReviewApproved {
			approved++
		}
		if pr.ReviewDecision == gh.ReviewChangesRequested {
			changes++
		}
		switch pr.CIState {
		case gh.StateSuccess:
			ciPass++
		case gh.StateFailure:
			ciFail++
		case gh.StatePending:
			ciRun++
		}
	}
	checks := p.badge(p.Green, p.ic.CheckPass+" Checks Pass")
	if ciFail > 0 {
		checks = p.badge(p.Red, p.ic.CheckFail+" Checks Fail")
	}
	ciAccent, ciBadge := p.Red, p.badge(p.Red, "Inspect Logs →")
	if failing == 0 {
		ciAccent, ciBadge = p.Green, p.badge(p.Green, p.ic.CheckPass+" All Green")
	}
	var milestone *gh.Issue
	for i := range d.Issues {
		if d.Issues[i].Milestone != "" {
			milestone = &d.Issues[i]
			break
		}
	}
	msBadge := p.badge(p.Purple, "Triage →")
	if milestone != nil {
		msBadge = p.badge(p.Purple, fmt.Sprintf("%s: %.0f%%", milestone.Milestone, milestone.MilestonePct))
	}

	reviewExtra := func(w int) string {
		if len(d.ReviewPRs) == 0 {
			return firstFit(w, p.fg(p.Green, p.ic.CheckPass+" queue clear"))
		}
		oldest := d.ReviewPRs[0]
		for _, pr := range d.ReviewPRs {
			if pr.UpdatedAt.Before(oldest.UpdatedAt) {
				oldest = pr
			}
		}
		num := p.fg(p.Yellow, fmt.Sprintf("#%d", oldest.Number))
		return firstFit(w,
			p.muted("oldest ")+num+p.muted(" · "+ago(oldest.UpdatedAt)+" · @"+oldest.Author),
			p.muted("oldest ")+num+p.muted(" · "+ago(oldest.UpdatedAt)),
			num+p.muted(" "+ago(oldest.UpdatedAt)))
	}
	prExtra := func(w int) string {
		ci := p.fg(p.Green, fmt.Sprintf("%s%d", p.ic.CheckPass, ciPass)) + " " +
			p.fg(p.Red, fmt.Sprintf("%s%d", p.ic.CheckFail, ciFail)) + " " + p.fg(p.Yellow, fmt.Sprintf("%s%d", p.ic.Running, ciRun))
		return firstFit(w,
			p.muted("CI ")+ci+p.muted(" · ")+p.fg(p.Red, fmt.Sprintf("%d changes req", changes)),
			p.muted("CI ")+ci,
			ci)
	}
	ciExtra := func(w int) string {
		for _, r := range d.Runs {
			if r.State() == gh.StateFailure {
				return firstFit(w,
					p.muted("last fail ")+p.fg(p.Red, r.Name)+p.muted(" · "+shortRepo(r.Repo, d.User.Login)+" · "+ago(r.StartedAt)),
					p.muted("last fail ")+p.fg(p.Red, r.Name)+p.muted(" · "+ago(r.StartedAt)),
					p.fg(p.Red, r.Name)+p.muted(" "+ago(r.StartedAt)))
			}
		}
		return firstFit(w, p.fg(p.Green, p.ic.CheckPass+" every workflow passing"), p.fg(p.Green, p.ic.CheckPass+" all passing"))
	}
	issueExtra := func(w int) string {
		if milestone != nil {
			label := p.fg(p.Purple, milestone.Milestone) + " "
			return label + p.progress(milestone.MilestonePct, max(w-width(label), 4), p.Purple)
		}
		count := map[string]int{}
		for _, is := range d.Issues {
			count[is.Priority]++
		}
		return p.fg(p.Red, fmt.Sprintf("P0 %d", count["P0"])) + p.muted(" · ") + p.fg(p.Yellow, fmt.Sprintf("P1 %d", count["P1"])) + p.muted(fmt.Sprintf(" · P2+ %d", count["P2"]+count["P3"]))
	}
	commitExtra := func(w int) string {
		days := append([]gh.ContributionDay(nil), d.Contributions...)
		sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })
		if len(days) > 30 {
			days = days[len(days)-30:]
		}
		vals := make([]int, len(days))
		for i, c := range days {
			vals[i] = c.Count
		}
		tail := p.muted(fmt.Sprintf(" %.1f/day", float64(d.ContributionsSince(30))/30))
		if n := w - width(tail); len(vals) > n && n > 0 {
			vals = vals[len(vals)-n:]
		}
		return p.sparkline(vals, p.Green) + tail
	}

	return []kpi{
		{"PRs Needing Your Review", "Review Queue", fmt.Sprintf("%d Waiting", len(d.ReviewPRs)), fmt.Sprintf("· %d Approved", approved), fmt.Sprintf("· %d ok", approved), p.TextSecondary, p.Yellow, p.badge(p.Yellow, "Review Queue →"), reviewExtra, len(d.ReviewPRs) > 0, tabPRs},
		{"Your Open PRs", "Your PRs", fmt.Sprintf("%d Active", len(d.MyPRs)), fmt.Sprintf("· %d Merge-Ready", d.MergeReadyCount()), fmt.Sprintf("· %d ready", d.MergeReadyCount()), p.TextSecondary, p.Green, checks, prExtra, false, tabPRs},
		{"Actions CI/CD Status", "Actions CI/CD", fmt.Sprintf("%d Failing", failing), fmt.Sprintf("· %d Running", running), fmt.Sprintf("· %d run", running), p.Cyan, ciAccent, ciBadge, ciExtra, failing > 0, tabActions},
		{"Assigned Issues", "Issues", fmt.Sprintf("%d Open", len(d.Issues)), fmt.Sprintf("· %d P0-Blocker", d.BlockerCount()), fmt.Sprintf("· %d P0", d.BlockerCount()), p.Red, p.TextPrimary, msBadge, issueExtra, d.BlockerCount() > 0, tabIssues},
		{"Recent Commits (30d)", "Commits 30d", fmt.Sprintf("%d Commits", d.ContributionsSince(30)), fmt.Sprintf("· %dd Streak", d.Streak()), fmt.Sprintf("· %dd", d.Streak()), p.Green, p.Blue, p.badge(p.Blue, fmt.Sprintf("%d Active Repos", len(d.Repos))), commitExtra, false, tabRepos},
	}
}

// firstFit returns the first candidate that fits w cells untruncated (the last one is truncated if none do).
func firstFit(w int, candidates ...string) string {
	for _, c := range candidates {
		if width(c) <= w {
			return c
		}
	}
	return fit(candidates[len(candidates)-1], w)
}

// kpiWidths splits w into 5 card widths separated by 1-cell gutters.
func kpiWidths(w int) []int {
	cw := (w - 4) / 5
	return []int{cw, cw, cw, cw, w - 4 - cw*4}
}

// kpiStrip renders the 5 sticky action cards. h is 5 (label, value + badge, extra), 4 or 3.
func (m Model) kpiStrip(w, h int) string {
	var parts []string
	for i, c := range m.kpis() {
		if i > 0 {
			parts = append(parts, strings.Repeat(" \n", h-1)+" ")
		}
		parts = append(parts, m.kpiCard(c, kpiWidths(w)[i], h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) kpiCard(c kpi, w, h int) string {
	p := m.p
	border := p.BorderMuted
	if c.alert {
		border = c.accent
	}
	b := func(s string) string { return p.fg(border, s) }
	inner := w - 4
	labelText := c.label
	if width(labelText) > inner {
		labelText = c.short
	}
	label := p.bold(p.TextSecondary, strings.ToUpper(labelText))
	// Value row: the big number, its detail, and the call-to-action badge right-aligned,
	// dropping the least important piece until it fits without truncation.
	big := p.bold(c.accent, c.value)
	value := big
	for _, cand := range []string{
		spread(big+" "+p.fg(c.dcolor, c.detail), c.badge, inner),
		spread(big+" "+p.fg(c.dcolor, c.brief), c.badge, inner),
		spread(big, c.badge, inner),
		big + " " + p.fg(c.dcolor, c.detail),
		big + " " + p.fg(c.dcolor, c.brief),
	} {
		if !strings.Contains(ansi.Strip(cand), "…") && width(cand) <= inner {
			value = cand
			break
		}
	}
	row := func(s string) string { return b("│") + " " + fit(s, inner) + " " + b("│") }
	lines := []string{b("╭" + strings.Repeat("─", w-2) + "╮")}
	if h >= 4 {
		lines = append(lines, row(label))
	}
	lines = append(lines, row(value))
	if h >= 5 && c.extra != nil {
		lines = append(lines, row(c.extra(inner)))
	}
	lines = append(lines, b("╰"+strings.Repeat("─", w-2)+"╯"))
	for i := range lines {
		lines[i] = paint(lines[i], p.BgSurface, w)
	}
	return strings.Join(lines, "\n")
}
