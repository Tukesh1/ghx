package ui

import (
	"fmt"
	"strings"
)

// drawerWidth is the notification drawer width for a terminal of width w.
func drawerWidth(w int) int { return clamp(w*42/100, 46, 66) }

// notificationsDrawer renders the unread inbox as a right-hand drawer of w×h cells.
func (m Model) notificationsDrawer(w, h int) []string {
	p := m.p
	inner := w - 4
	var body []string
	if len(m.data.Notifications) == 0 {
		body = append(body, "", p.fg(p.Green, p.ic.CheckPass+" Inbox zero — nothing unread."))
	}
	for i, n := range m.data.Notifications {
		icon, color := p.ic.Bell, p.Blue
		switch n.Type {
		case "PullRequest":
			icon, color = p.ic.PROpen, p.Green
		case "Issue":
			icon, color = p.ic.IssueOpen, p.Yellow
		case "CheckSuite":
			icon, color = p.ic.CheckFail, p.Red
		case "RepositoryVulnerabilityAlert":
			icon, color = p.ic.Shield, p.Red
		case "Release":
			icon, color = p.ic.Tag, p.Cyan
		}
		reason := strings.ReplaceAll(n.Reason, "_", " ")
		lines := []string{
			spread(p.fg(color, icon)+" "+p.bold(p.Cyan, reason+" · "+shortRepo(n.Repo, m.data.User.Login)), p.muted(ago(n.UpdatedAt)), inner-2),
			p.text(n.Title),
		}
		if i == m.notifCursor {
			for j := range lines {
				lines[j] = paint(p.fg(p.BorderActive, "▎")+fit(lines[j], inner-1), p.Selection, inner)
			}
		} else {
			lines = p.inset(lines, inner)
		}
		body = append(body, lines...)
		body = append(body, "")
	}
	footer := []string{
		p.line(strings.Repeat("─", inner)),
		spread(p.kbd("m")+" "+p.fg(p.Green, p.ic.CheckPass+" Mark all as read"), p.muted("Press ")+p.kbd("n")+p.muted(" to toggle"), inner),
	}
	title := fmt.Sprintf("%s UNREAD GITHUB NOTIFICATIONS INBOX", p.ic.Bell)
	if width(title)+8 > w {
		title = p.ic.Bell + " INBOX"
	}
	lines := strings.Split(p.panel(title, p.bold(p.Red, "✕"), body, footer, w, h, true), "\n")
	return lines
}

// helpModal renders the keyboard shortcuts reference as a two-column grid of key cells.
func (m Model) helpModal() []string {
	p := m.p
	keys := [][2]string{
		{"Switch Views", "1 .. 5"}, {"Filter / Search", "/"},
		{"Navigate Rows", "j / k"}, {"Top / Bottom", "g / G"},
		{"Focus Panel", "tab"}, {"Inspect Row", "enter"},
		{"Cycle Filter Chips", "f"}, {"Approve PR", "a"},
		{"Squash & Merge", "m m"}, {"Checkout / Clone", "c"},
		{"Re-run / Fetch", "r"}, {"Workflow Dispatch", "w w"},
		{"Close Issue", "x x"}, {"Comment on Issue", "c"},
		{"Repo Insights", "enter"}, {"Back from Insights", "esc"},
		{"Open in Browser", "o"}, {"Dependabot Bump", "u / U"},
		{"Cycle Theme", "t"}, {"Glyph Mode", "i"},
		{"Notifications", "n"}, {"Mark Inbox Read", "m"},
		{"Clear / Close", "esc"}, {"Quit", "q"},
	}
	w := 70
	inner := w - 4
	cw := (inner - 1) / 2
	var body []string
	for i := 0; i < len(keys); i += 2 {
		left := paint(spread(" "+p.text(keys[i][0]), p.kbd(keys[i][1])+" ", cw), p.BgPrimary, cw)
		right := ""
		if i+1 < len(keys) {
			right = paint(spread(" "+p.text(keys[i+1][0]), p.kbd(keys[i+1][1])+" ", inner-cw-1), p.BgPrimary, inner-cw-1)
		}
		body = append(body, left+" "+right)
	}
	footer := []string{"", p.muted("Theme: ") + p.fg(p.Cyan, p.Name) + p.muted(" · mouse: click tabs, cards & rows · wheel scrolls")}
	return strings.Split(p.panel("⌨  GHX KEYBOARD SHORTCUTS", p.bold(p.Red, "✕ [Esc]"), body, footer, w, len(body)+len(footer)+panelChrome, true), "\n")
}
