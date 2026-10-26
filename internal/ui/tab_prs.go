package ui

import (
	"fmt"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
)

// splitRatio is the list-pane share for each dedicated tab (design: 7/5 for PRs & Actions, 8/4 for Repos & Issues).
func splitRatio(tab int) int {
	switch tab {
	case tabRepos:
		return 52
	case tabIssues:
		return 66
	}
	return 58
}

// splitTab joins the list pane and the inspector for a dedicated tab.
func (m Model) splitTab(w, h int, left func(int, int) string, right func(int, int) string) string {
	lw, rw := split(w, splitRatio(m.tab))
	return hjoin(left(lw, h), right(rw, h))
}

// listPane renders a dedicated tab's table inside a titled panel.
func (m Model) listPane(kind int, title, chips string, cols []column, rows [][]string, w, h int) string {
	body := m.p.table(cols, rows, m.cursor[kind], w-4, h-panelChrome, true)
	return m.p.panel(title, chips, body, nil, w, h, true)
}

// buttons renders design-style action buttons (key cap + tinted chip), wrapping onto extra lines.
func (m Model) buttons(w int, items ...[3]string) []string {
	var lines []string
	cur := ""
	for _, it := range items {
		b := m.p.kbd(it[0]) + m.p.badge(it[2], it[1])
		if cur != "" && width(cur)+2+width(b) > w {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += "  "
		}
		cur += b
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// boxPair renders two captioned inset boxes side by side at equal height.
func (m Model) boxPair(capA string, a []string, capB string, b []string, w int) []string {
	n := max(len(a), len(b))
	for len(a) < n {
		a = append(a, "")
	}
	for len(b) < n {
		b = append(b, "")
	}
	lw := (w - 1) / 2
	return sideBySide(m.p.box(capA, a, lw), m.p.box(capB, b, w-lw-1), w)
}

var prCols = []column{
	{title: "PR #", width: 8},
	{title: "Repository", width: -1, drop: 3},
	{title: "Title & Branch", width: -3, min: 26},
	{title: "Author", width: 12, drop: 4},
	{title: "Review", width: 15},
	{title: "CI Checks", width: 16, drop: 1},
	{title: "Mergeable", width: 11, drop: 2},
	{title: "Diff", width: 11, drop: 5},
}

func (m Model) prRows(prs []gh.PullRequest) [][]string {
	p := m.p
	rows := make([][]string, 0, len(prs))
	for _, pr := range prs {
		rows = append(rows, []string{
			p.prIcon(pr) + " " + p.bold(p.TextPrimary, fmt.Sprintf("#%d", pr.Number)),
			p.bold(p.Blue, shortRepo(pr.Repo, m.data.User.Login)),
			cell(p.text(pr.Title), p.muted(p.ic.Branch+" "+pr.HeadRef)),
			p.fg(p.Cyan, pr.Author),
			p.reviewBadge(pr),
			p.ciBadge(pr.CIState, checksLabel(pr)),
			p.mergeableBadge(pr),
			p.diff(pr.Additions, pr.Deletions),
		})
	}
	return rows
}

// viewPRs renders Tab [2]: Pull Requests queue + changed files / reviews / checks inspector.
func (m Model) viewPRs(w, h int) string {
	p := m.p
	return m.splitTab(w, h,
		func(lw, h int) string {
			chips := m.filterChips([]string{fmt.Sprintf("All Open (%d)", len(m.data.ActionPRs())), "Review Req", "Approved", "Changes Req"},
				[]string{p.Blue, p.Yellow, p.Green, p.Red}, m.pill[kindPR])
			title := titled(p.ic.PROpen+" PULL REQUESTS QUEUE", p.ic.PROpen+" PULL REQUESTS QUEUE (AUTHORED, REVIEW REQUESTED & MENTIONED)", chips, lw)
			return m.listPane(kindPR, title, chips, prCols, m.prRows(m.prs()), lw, h)
		},
		m.prInspector)
}

func mergeState(pr gh.PullRequest) string {
	switch {
	case pr.IsDraft:
		return "DRAFT"
	case !pr.Mergeable || pr.ReviewDecision == gh.ReviewChangesRequested:
		return "BLOCKED"
	case pr.CIState == gh.StateFailure:
		return "UNSTABLE"
	case pr.MergeReady():
		return "CLEAN"
	default:
		return "REVIEW_REQUIRED"
	}
}

func (m Model) prInspector(w, h int) string {
	p := m.p
	inner := w - 4
	title := titled("PULL REQUEST INSPECTOR", "PULL REQUEST INSPECTOR (CHANGED FILES, REVIEWS & CHECKS)", "#00000", w)
	pr, ok := m.selectedPR()
	if !ok {
		return p.panel(title, "", []string{p.muted("no pull request selected")}, nil, w, h, false)
	}
	var b []string
	for _, l := range wrap(pr.Title, inner, 2) {
		b = append(b, p.bold(p.TextPrimary, l))
	}
	b = append(b,
		p.fg(p.Blue, pr.Repo)+p.dim(" · ")+p.fg(p.Blue, p.ic.Branch+" "+pr.HeadRef)+p.dim(" "+p.ic.Arrow+" "+pr.BaseRef),
		p.muted("by ")+p.fg(p.Cyan, "@"+pr.Author)+p.muted(" · updated "+ago(pr.UpdatedAt)+" ago"),
		"")

	var reviewers []string
	for _, r := range pr.Reviewers {
		reviewers = append(reviewers, reviewerState(p, r.State)+" "+p.text("@"+r.Login))
	}
	if len(reviewers) == 0 {
		reviewers = []string{p.muted("none requested")}
	}
	state := mergeState(pr)
	stateColor := map[string]string{"CLEAN": p.Green, "BLOCKED": p.Red, "UNSTABLE": p.Yellow, "DRAFT": p.TextSecondary}[state]
	if stateColor == "" {
		stateColor = p.Yellow
	}
	gate := []string{
		p.muted("State: ") + p.bold(stateColor, state),
		p.muted("Files: ") + p.bold(p.TextPrimary, fmt.Sprint(pr.ChangedFiles)) + " (" + p.diff(pr.Additions, pr.Deletions) + ")",
		p.diffBar(pr.Additions, pr.Deletions, clamp((inner-1)/2-4, 4, 24)),
	}
	b = append(b, m.boxPair("Reviewers", reviewers, "Merge Gate & Diff", gate, inner)...)
	b = append(b, "")

	var files []string
	for _, f := range pr.Files {
		files = append(files, spread(p.fg(p.Cyan, p.ic.File+" "+f.Path), p.diff(f.Additions, f.Deletions), inner-2))
	}
	if len(files) == 0 {
		files = []string{p.muted("file list unavailable")}
	}
	b = append(b, p.box("Changed Files", files, inner)...)
	b = append(b, "")

	var checks []string
	for _, c := range pr.Checks {
		checks = append(checks, p.stateIcon(c.State)+" "+p.text(c.Name))
	}
	if len(checks) == 0 {
		checks = []string{p.muted("no checks reported")}
	}
	ratio := checksRatio(pr)
	if ratio == "" {
		ratio = "0/0"
	}
	b = append(b, p.box("CI Check Runs ("+ratio+")", checks, inner)...)

	footer := append([]string{p.line(strings.Repeat("─", inner))}, m.buttons(inner,
		[3]string{"a", p.ic.CheckPass + " Approve PR", p.Green},
		[3]string{"m m", p.ic.PRMerged + " Squash & Merge", p.Purple},
		[3]string{"c", fmt.Sprintf("%s gh pr checkout %d", p.ic.Branch, pr.Number), p.Blue},
		[3]string{"o", "Open", p.TextSecondary},
	)...)
	return p.panel(title, p.badge(p.Purple, fmt.Sprintf("#%d", pr.Number)), b, footer, w, h, false)
}

func reviewerState(p palette, state string) string {
	switch state {
	case "APPROVED":
		return p.fg(p.Green, p.ic.CheckPass)
	case "CHANGES_REQUESTED":
		return p.fg(p.Red, p.ic.CheckFail)
	case "COMMENTED":
		return p.fg(p.Blue, p.ic.Comment)
	default:
		return p.fg(p.Yellow, p.ic.Clock)
	}
}
