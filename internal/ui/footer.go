package ui

import (
	"strings"
)

// commandEcho is the default status line: a one-line summary of what needs attention.
func (m Model) commandEcho() string {
	failing, _ := m.data.RunCounts()
	parts := []string{}
	if n := len(m.data.ReviewPRs); n > 0 {
		parts = append(parts, plural(n, "review")+" waiting")
	}
	if n := m.data.MergeReadyCount(); n > 0 {
		parts = append(parts, plural(n, "PR")+" ready to merge")
	}
	if failing > 0 {
		parts = append(parts, plural(failing, "run")+" failing")
	}
	if n := m.data.BlockerCount(); n > 0 {
		parts = append(parts, plural(n, "P0 issue")+" open")
	}
	if len(parts) == 0 {
		return "all clear · nothing needs your attention"
	}
	return strings.Join(parts, " · ")
}

// footerHints are the key caps shown for the current context, most important first.
func (m Model) footerHints() [][2]string {
	common := [][2]string{{"/", "Filter"}, {"n", "Inbox"}, {"R", "Refresh"}, {"t", "Theme"}, {"q", "Quit"}}
	var ctx [][2]string
	switch {
	case m.composeTo != nil:
		return [][2]string{{"ctrl+s", "Post"}, {"esc", "Cancel"}}
	case m.showNotifs:
		return [][2]string{{"⏎", "Open"}, {"m", "Mark all read"}, {"y", "Copy link"}, {"n", "Close"}}
	case m.searching:
		return [][2]string{{"⏎", "Apply"}, {"esc", "Clear"}}
	case m.tab == tabRepos && m.insights:
		ctx = [][2]string{{"j/k", "Repo"}, {"esc", "Back"}, {"o", "Open"}, {"r", "Reload"}}
	case m.activeKind() == kindPR:
		ctx = [][2]string{{"a", "Approve"}, {"m m", "Merge"}, {"c", "Checkout"}, {"C", "Comment"}, {"o", "Open"}}
	case m.activeKind() == kindRun:
		ctx = [][2]string{{"r", "Re-run"}, {"w w", "Dispatch"}, {"o", "Open"}, {"y", "Copy link"}}
	case m.activeKind() == kindRepo:
		ctx = [][2]string{{"⏎", "Insights"}, {"c", "Clone"}, {"o", "Open"}}
	case m.activeKind() == kindIssue:
		ctx = [][2]string{{"c", "Comment"}, {"x x", "Close"}, {"o", "Open"}}
	case m.activeKind() == kindPinned:
		ctx = [][2]string{{"⏎", "Open repo"}, {"O", "Open profile"}, {"c", "Clone"}, {"y", "Copy link"}}
	}
	if m.tab == tabOverview && !m.showNotifs {
		ctx = append([][2]string{{"tab", "Panel"}, {"⏎", "Expand"}}, ctx...)
	}
	return append(append([][2]string{{"?", "Keys"}}, ctx...), common...)
}

// footer renders the bottom status line: mode pill, status message, and context key caps.
func (m Model) footer(w int) string {
	p := m.p
	mode, modeColor := "NORMAL", p.Blue
	switch {
	case m.composeTo != nil:
		mode, modeColor = "COMMENT", p.Purple
	case m.searching:
		mode, modeColor = "FILTER", p.Yellow
	case m.pending != "":
		mode, modeColor = "CONFIRM", p.Red
	case m.busy > 0:
		mode, modeColor = m.spin()+" WORKING", p.Yellow
	case m.loading:
		mode, modeColor = m.spin()+" SYNCING", p.Yellow
	case m.data.Demo:
		mode, modeColor = "DEMO", p.Yellow
	}

	msg, color := m.status, p.Cyan
	switch m.kind {
	case statusOK:
		color = p.Green
	case statusErr:
		color = p.Red
	}
	if msg == "" {
		msg = m.commandEcho()
		color = p.TextSecondary
	}
	left := " " + p.pill(modeColor, mode) + " " + p.fg(color, msg)

	// Hints get at least ~40% of the line; the message is truncated before hints are dropped.
	var parts []string
	used := 0
	budget := max(w-width(left)-3, w*2/5)
	for _, h := range m.footerHints() {
		s := p.kbd(h[0]) + " " + p.muted(h[1])
		if used+width(s)+2 > budget {
			break
		}
		parts = append(parts, s)
		used += width(s) + 2
	}
	return paint(spread(left, strings.Join(parts, "  ")+" ", w), p.BgSurface, w)
}
