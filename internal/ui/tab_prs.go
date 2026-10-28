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
func (m Model) listPane(kind int, title, chips string, cols []column, rows [][]string, w, h int, empty string) string {
	if m.filter != "" {
		empty = "Nothing matches “" + m.filter + "” · esc clears the filter"
	}
	body := m.p.tableEmpty(cols, rows, m.cursor[kind], w-4, h-panelChrome, true, empty)
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
	{title: "Title · Repository & Branch", width: -3, min: 26},
	{title: "Author", width: 13, drop: 4},
	{title: "Review", width: 15},
	{title: "CI Checks", width: 16, drop: 1},
	{title: "Merge", width: 11, drop: 2},
	{title: "Diff", width: 11, drop: 5},
	{title: "Updated", width: 7, right: true, drop: 3},
}

func (m Model) prRows(prs []gh.PullRequest) [][]string {
	p := m.p
	rows := make([][]string, 0, len(prs))
	for _, pr := range prs {
		author := p.fg(p.Cyan, "@"+pr.Author)
		if pr.Author == m.data.User.Login {
			author = p.muted("you")
		}
		rows = append(rows, []string{
			p.prIcon(pr) + " " + p.bold(p.TextPrimary, fmt.Sprintf("#%d", pr.Number)),
			cell(p.text(pr.Title), m.repoRefMax(pr.Repo, 24)+p.muted(" · "+p.ic.Branch+" "+pr.HeadRef)),
			author,
			p.reviewBadge(pr),
			p.ciBadge(pr.CIState, checksLabel(pr)),
			p.mergeableBadge(pr),
			p.diff(pr.Additions, pr.Deletions),
			p.muted(ago(pr.UpdatedAt)),
		})
	}
	return rows
}

// viewPRs renders Tab [2]: Pull Requests queue + changed files / reviews / checks inspector.
func (m Model) viewPRs(w, h int) string {
	p := m.p
	return m.splitTab(w, h,
		func(lw, h int) string {
			chips := m.chipsFit([]string{fmt.Sprintf("All (%d)", len(m.data.ActionPRs())), "Needs Review", "Ready", "Needs Work"},
				[]string{"All", "Review", "Ready", "Work"},
				[]string{p.Blue, p.Yellow, p.Green, p.Red}, m.pill[kindPR], lw)
			title := titled(p.ic.PROpen+" PULL REQUESTS", p.ic.PROpen+" PULL REQUESTS · REVIEW REQUESTED & AUTHORED", chips, lw)
			return m.listPane(kindPR, title, chips, prCols, m.prRows(m.prs()), lw, h, "No pull requests here · f cycles filters")
		},
		m.prInspector)
}

// mergeState summarises what stands between a PR and the merge button.
func mergeState(pr gh.PullRequest) string {
	switch {
	case pr.IsDraft:
		return "DRAFT"
	case pr.Conflicting:
		return "CONFLICTS"
	case pr.ReviewDecision == gh.ReviewChangesRequested:
		return "CHANGES REQUESTED"
	case pr.CIState == gh.StateFailure:
		return "CI FAILING"
	case pr.MergeReady():
		return "READY TO MERGE"
	case pr.CIState == gh.StatePending:
		return "CI RUNNING"
	case pr.ReviewDecision == gh.ReviewRequired:
		return "REVIEW REQUIRED"
	case !pr.Mergeable:
		return "CHECKING"
	default:
		return "BLOCKED"
	}
}

func (m Model) prInspector(w, h int) string {
	p := m.p
	inner := w - 4
	title := titled("PULL REQUEST", "PULL REQUEST · REVIEWS, FILES & CHECKS", "#00000", w)
	pr, ok := m.selectedPR()
	if !ok {
		return p.panel(title, "", []string{p.muted("no pull request selected")}, nil, w, h, false)
	}
	var b []string
	for _, l := range wrap(pr.Title, inner, 2) {
		b = append(b, p.bold(p.TextPrimary, l))
	}
	b = append(b,
		p.fg(p.Blue, pr.Repo)+p.dim(" · ")+p.fg(p.Cyan, p.ic.Branch+" "+pr.HeadRef)+p.dim(" "+p.ic.Arrow+" ")+p.muted(pr.BaseRef),
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
	stateColor := map[string]string{"READY TO MERGE": p.Green, "CONFLICTS": p.Red, "CHANGES REQUESTED": p.Red, "CI FAILING": p.Red,
		"DRAFT": p.TextSecondary, "CHECKING": p.TextSecondary}[state]
	if stateColor == "" {
		stateColor = p.Yellow
	}
	gate := []string{
		p.bold(stateColor, state),
		p.muted("Files: ") + p.bold(p.TextPrimary, fmt.Sprint(pr.ChangedFiles)) + " (" + p.diff(pr.Additions, pr.Deletions) + ")",
		p.diffBar(pr.Additions, pr.Deletions, clamp((inner-1)/2-4, 4, 24)),
	}
	b = append(b, m.boxPair("Reviewers", reviewers, "Merge Gate & Diff", gate, inner)...)
	b = append(b, "")

	var files []string
	for _, f := range pr.Files {
		d := p.diff(f.Additions, f.Deletions)
		files = append(files, fitMid(p.fg(p.Cyan, p.ic.File+" "+f.Path), inner-3-width(d))+" "+d)
	}
	if len(files) == 0 {
		files = []string{p.muted("file list unavailable")}
	}
	if more := pr.ChangedFiles - len(pr.Files); more > 0 && len(pr.Files) > 0 {
		files = append(files, p.dim(fmt.Sprintf("… %d more", more)))
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

	var actions [][3]string
	if pr.Author != m.data.User.Login && pr.ReviewDecision != gh.ReviewApproved {
		actions = append(actions, [3]string{"a", p.ic.CheckPass + " Approve", p.Green})
	}
	if !pr.IsDraft && !pr.Conflicting {
		color := p.Purple
		if !pr.MergeReady() {
			color = p.TextSecondary
		}
		actions = append(actions, [3]string{"m m", p.ic.PRMerged + " " + pr.MergeLabel(), color})
	}
	actions = append(actions,
		[3]string{"c", p.ic.Branch + " Checkout", p.Blue},
		[3]string{"C", p.ic.Comment + " Comment", p.Blue},
		[3]string{"o", "Open", p.TextSecondary},
	)
	footer := append([]string{p.line(strings.Repeat("─", inner))}, m.buttons(inner, actions...)...)
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
