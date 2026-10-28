package ui

import (
	"fmt"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
)

var issueCols = []column{
	{title: "ID", width: 8},
	{title: "Repository", width: -1, drop: 4, mid: true},
	{title: "Title", width: -3, min: 26},
	{title: "Priority", width: 12},
	{title: "Labels", width: 18, drop: 3},
	{title: "Linked PR", width: 9, drop: 2},
	{title: "Opened By", width: 12, drop: 6},
	{title: "Comments", width: 8, right: true, drop: 5},
	{title: "Age", width: 5, right: true, drop: 1},
}

func (m Model) issueRows(issues []gh.Issue) [][]string {
	p := m.p
	rows := make([][]string, 0, len(issues))
	for _, is := range issues {
		linked := p.dim("—")
		if is.LinkedPR > 0 {
			linked = p.fg(p.Purple, fmt.Sprintf("%s #%d", p.ic.PROpen, is.LinkedPR))
		}
		var labels []string
		for _, l := range is.Labels {
			if !strings.EqualFold(l.Name, is.Priority) {
				labels = append(labels, p.badge(p.TextSecondary, l.Name))
			}
		}
		author := p.dim("—")
		if is.Author != "" {
			author = p.fg(p.Cyan, "@"+is.Author)
		}
		rows = append(rows, []string{
			p.fg(p.Green, p.ic.IssueOpen) + " " + p.bold(p.TextPrimary, fmt.Sprintf("#%d", is.Number)),
			m.repoRef(is.Repo),
			p.text(is.Title),
			p.priorityBadge(is.Priority),
			strings.Join(labels, " "),
			linked,
			author,
			p.muted(fmt.Sprintf("%s %d", p.ic.Comment, is.Comments)),
			p.muted(ago(is.CreatedAt)),
		})
	}
	return rows
}

// viewIssues renders Tab [5]: issues & bug triage + discussion and milestone inspector.
func (m Model) viewIssues(w, h int) string {
	p := m.p
	return m.splitTab(w, h,
		func(lw, h int) string {
			count := map[string]int{}
			for _, is := range m.data.Issues {
				count[is.Priority]++
			}
			chips := m.chipsFit(
				[]string{fmt.Sprintf("All (%d)", len(m.data.Issues)), fmt.Sprintf("P0 (%d)", count["P0"]), fmt.Sprintf("P1 (%d)", count["P1"]), fmt.Sprintf("P2+ (%d)", count["P2"]+count["P3"])},
				[]string{"All", "P0", "P1", "P2+"},
				[]string{p.Blue, p.Red, p.Yellow, p.TextSecondary}, m.pill[kindIssue], lw)
			title := titled(p.ic.IssueOpen+" ISSUES & BUG TRIAGE", p.ic.IssueOpen+" ISSUES, BUG REPORTS & FEATURE REQUESTS", chips, lw)
			return m.listPane(kindIssue, title, chips, issueCols, m.issueRows(m.issues()), lw, h, "No open issues are assigned to you")
		},
		m.issueInspector)
}

func (m Model) findPR(repo string, number int) (gh.PullRequest, bool) {
	for _, pr := range m.data.ActionPRs() {
		if pr.Repo == repo && pr.Number == number {
			return pr, true
		}
	}
	return gh.PullRequest{}, false
}

func (m Model) issueInspector(w, h int) string {
	p := m.p
	inner := w - 4
	title := titled("ISSUE", "ISSUE · DESCRIPTION & MILESTONE", "v0.00 (00%)", w)
	is, ok := m.selectedIssue()
	if !ok {
		return p.panel(title, "", []string{p.muted("no issue selected")}, nil, w, h, false)
	}
	b := []string{p.bold(p.Yellow, fmt.Sprintf("#%d · %s", is.Number, is.Repo))}
	for _, l := range wrap(is.Title, inner, 2) {
		b = append(b, p.bold(p.TextPrimary, l))
	}
	labels := p.priorityBadge(is.Priority)
	for _, l := range is.Labels {
		if !strings.EqualFold(l.Name, is.Priority) {
			labels += " " + p.badge(p.TextSecondary, l.Name)
		}
	}
	opened := "opened " + ago(is.CreatedAt) + " ago"
	if is.Author != "" {
		opened += " by @" + is.Author
	}
	b = append(b, labels+"  "+p.muted(p.ic.Comment+" "+plural(is.Comments, "comment")+" · "+opened), "")

	body := strings.TrimSpace(is.Body)
	if body == "" {
		body = "No description provided."
	}
	var bl []string
	for _, l := range wrap(body, inner-2, 8) {
		bl = append(bl, p.muted(l))
	}
	b = append(b, p.box("Description", bl, inner)...)
	b = append(b, "")

	switch pr, found := m.findPR(is.Repo, is.LinkedPR); {
	case is.LinkedPR == 0:
		b = append(b, p.muted("Linked Pull Request: ")+p.dim("none yet"))
	case found:
		b = append(b,
			p.muted("Linked Pull Request: ")+p.bold(p.Purple, fmt.Sprintf("#%d", pr.Number))+" "+p.text(pr.Title),
			"  "+p.reviewBadge(pr)+" "+p.ciBadge(pr.CIState, checksRatio(pr))+" "+p.diff(pr.Additions, pr.Deletions))
	default:
		b = append(b, p.muted("Linked Pull Request: ")+p.bold(p.Purple, fmt.Sprintf("#%d", is.LinkedPR)))
	}
	b = append(b, "")

	right := p.badge(p.TextSecondary, "no milestone")
	if is.Milestone != "" {
		right = p.badge(p.Green, fmt.Sprintf("%s (%.0f%%)", is.Milestone, is.MilestonePct))
		b = append(b, p.box("Milestone", []string{
			p.fg(p.Purple, p.ic.Milestone+" "+is.Milestone) + p.muted(fmt.Sprintf("  %.0f%% complete", is.MilestonePct)),
			p.progress(is.MilestonePct, clamp(inner-4, 8, 48), p.Purple),
		}, inner)...)
	}

	footer := append([]string{p.line(strings.Repeat("─", inner))}, m.buttons(inner,
		[3]string{"x x", p.ic.IssueClosed + " Close Issue", p.Purple},
		[3]string{"c", p.ic.Comment + " Comment", p.Blue},
		[3]string{"o", "Open", p.TextSecondary},
		[3]string{"y", "Copy Link", p.TextSecondary},
	)...)
	return p.panel(title, right, b, footer, w, h, false)
}
