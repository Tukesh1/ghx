package ui

import (
	"fmt"
	"strings"
)

// commandEcho is the default status line: a gh-style summary of what needs attention.
func (m Model) commandEcho() string {
	org := "all"
	if m.cfg.Org != "" {
		org = m.cfg.Org
	}
	failing, _ := m.data.RunCounts()
	return fmt.Sprintf("gh status --org=%s // %d PRs awaiting your review · %d CI run(s) failing", org, len(m.data.ReviewPRs), failing)
}

// footer renders the bottom status line: mode pill, command echo with blinking cursor, and key caps.
func (m Model) footer(w int) string {
	p := m.p
	mode, modeColor := "INTERACTIVE", p.Blue
	switch {
	case m.searching:
		mode, modeColor = "FILTER", p.Yellow
	case m.pending != "":
		mode, modeColor = "CONFIRM", p.Red
	case m.loading:
		mode, modeColor = "SYNCING", p.Yellow
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
	}
	cursor := " "
	if m.frame%2 == 0 || m.loading {
		cursor = p.fg(p.Green, "█")
	}
	left := " " + p.pill(modeColor, mode) + " " + p.fg(color, msg) + " " + cursor

	hints := [][2]string{{"1-5", "Views"}, {"/", "Filter"}, {"j/k", "Row Nav"}, {"t", "Theme"}, {"i", "Glyphs"}, {"n", "Inbox"}, {"r", "Fetch"}, {"?", "Keys"}}

	budget := w - width(left) - 3
	var parts []string
	used := 0
	for _, h := range hints {
		s := p.kbd(h[0]) + " " + p.muted(h[1])
		if used+width(s)+2 > budget {
			break
		}
		parts = append(parts, s)
		used += width(s) + 2
	}
	return paint(spread(left, strings.Join(parts, "  ")+" ", w), p.BgSurface, w)
}
