package ui

import (
	"fmt"
	"strings"
)

// drawerWidth is the notification drawer width for a terminal of width w.
func drawerWidth(w int) int { return clamp(w*42/100, 46, 66) }

// notifRows is the height of one inbox entry: two text lines and a spacer.
const notifRows = 3

// notifWindow returns the first visible inbox entry and how many fit in a drawer of height h.
func (m Model) notifWindow(h int) (start, perPage int) {
	room := h - panelChrome - 2 // title chrome, footer rule and hint
	perPage = max((room+1)/notifRows, 1)
	if m.notifCursor >= perPage {
		start = m.notifCursor - perPage + 1
	}
	return start, perPage
}

// notifAt maps a row inside the body to the inbox entry drawn there, or -1.
func (m Model) notifAt(relY int) int {
	start, perPage := m.notifWindow(m.bodyHeight())
	line := relY - 2 // top border + title bar
	if line < 0 || line%notifRows == notifRows-1 || line/notifRows >= perPage {
		return -1
	}
	idx := start + line/notifRows
	if idx >= len(m.data.Notifications) {
		return -1
	}
	return idx
}

// notificationsDrawer renders the unread inbox as a right-hand drawer of w×h cells.
func (m Model) notificationsDrawer(w, h int) []string {
	p := m.p
	inner := w - 4
	ns := m.data.Notifications
	var body []string
	if len(ns) == 0 {
		body = append(body, "", p.fg(p.Green, p.ic.CheckPass+" Inbox zero — nothing unread."))
	}
	start, perPage := m.notifWindow(h)
	for i := start; i < len(ns) && i < start+perPage; i++ {
		n := ns[i]
		icon, color := p.ic.Bell, p.Blue
		switch n.Type {
		case "PullRequest":
			icon, color = p.ic.PROpen, p.Green
		case "Issue":
			icon, color = p.ic.IssueOpen, p.Yellow
		case "CheckSuite", "WorkflowRun":
			icon, color = p.ic.CheckFail, p.Red
		case "RepositoryVulnerabilityAlert", "RepositoryDependabotAlertsThread":
			icon, color = p.ic.Shield, p.Red
		case "Release":
			icon, color = p.ic.Tag, p.Cyan
		case "Discussion":
			icon, color = p.ic.Comment, p.Purple
		}
		reason := strings.ReplaceAll(n.Reason, "_", " ")
		lines := []string{
			spread(p.fg(color, icon)+" "+p.bold(p.Cyan, reason)+p.muted(" · "+shortRepo(n.Repo, m.data.User.Login)), p.muted(ago(n.UpdatedAt)), inner-2),
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
		firstFit(inner,
			p.kbd("⏎")+p.muted(" open & mark read  ")+p.kbd("m")+p.muted(" mark all read  ")+p.kbd("y")+p.muted(" copy link  ")+p.kbd("n")+p.muted(" close"),
			p.kbd("⏎")+p.muted(" open  ")+p.kbd("m")+p.muted(" all read  ")+p.kbd("n")+p.muted(" close")),
	}
	title := p.ic.Bell + " NOTIFICATIONS"
	right := p.badge(p.Blue, fmt.Sprintf("%d unread", len(ns)))
	if len(ns) > perPage {
		right = p.muted(fmt.Sprintf("%d/%d ", m.notifCursor+1, len(ns))) + right
	}
	return strings.Split(p.panel(title, right, body, footer, w, h, true), "\n")
}

// helpKeys lists every shortcut as {action, key}, grouped by context.
var helpKeys = [][2]string{
	{"Switch views", "1–" + fmt.Sprint(len(tabNames))}, {"Move selection", "j k"}, {"Page down / up", "^d ^u"}, {"Top / bottom", "g G"},
	{"Focus panel", "tab h l"}, {"Inspect / open", "enter"}, {"Fuzzy filter", "/"}, {"Cycle filter chips", "f"},
	{"Back / clear filter", "esc"}, {"Approve PR", "a"}, {"Merge PR", "m m"}, {"Checkout PR", "c"},
	{"Comment on PR/issue", "C"}, {"Re-run failed jobs", "r"}, {"Run workflow", "w w"}, {"Copy clone command", "c"},
	{"Close issue", "x x"}, {"Comment on issue", "c"}, {"Open in browser", "o"}, {"Open your profile", "O"}, {"Copy link", "y"},
	{"Refresh", "R"}, {"Notifications", "n"}, {"Dependabot alert", "u U"}, {"Cycle theme", "t"},
	{"Nerd / Unicode glyphs", "i"}, {"Quit", "q"},
}

// helpModal renders the keyboard shortcuts as a grid sized for a w×h body.
func (m Model) helpModal(w, h int) []string {
	p := m.p
	cols := 2
	if w >= 112 {
		cols = 3
	}
	cw := 32
	mw := min(cols*(cw+1)-1+4, w-4)
	inner := mw - 4
	cw = (inner - (cols - 1)) / cols
	rows := (len(helpKeys) + cols - 1) / cols
	var body []string
	for r := 0; r < rows; r++ {
		var cells []string
		for c := 0; c < cols; c++ {
			i := c*rows + r
			if i >= len(helpKeys) {
				cells = append(cells, strings.Repeat(" ", cw))
				continue
			}
			k := helpKeys[i]
			cells = append(cells, paint(spread(" "+p.text(k[0]), p.kbd(k[1])+" ", cw), p.BgPrimary, cw))
		}
		body = append(body, strings.Join(cells, " "))
	}
	var footer []string
	if rows+panelChrome+2 <= h {
		footer = []string{"", p.muted("Mouse: click tabs, cards, rows and header controls · wheel scrolls · ") + p.fg(p.Cyan, p.Name)}
	}
	mh := min(rows+len(footer)+panelChrome, h)
	return strings.Split(p.panel("⌨  KEYBOARD SHORTCUTS", p.kbd("?")+p.muted(" or ")+p.kbd("esc")+p.muted(" close"), body, footer, mw, mh, true), "\n")
}
