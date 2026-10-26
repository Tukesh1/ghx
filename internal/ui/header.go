package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Version is the build version shown in the header badge; main sets it from the release ldflags.
var Version = "dev"

func versionBadge() string {
	if Version == "" || Version == "dev" || strings.HasPrefix(Version, "v") {
		return Version
	}
	return "v" + Version
}

type headerItem struct {
	s    string
	prio int
}

// joinFitting joins items with sep, dropping the lowest-priority ones until the result fits w.
func joinFitting(items []headerItem, sep string, w int) string {
	for floor := 0; floor <= 10; floor++ {
		var parts []string
		for _, it := range items {
			if it.prio >= floor && it.s != "" {
				parts = append(parts, it.s)
			}
		}
		out := strings.Join(parts, sep)
		if width(out) <= w || len(parts) <= 1 {
			return out
		}
	}
	return ""
}

// header renders the top bar: brand, @user, context switcher, glyph/theme fields, inbox and keys buttons, clock.
func (m Model) header(w int) string {
	p := m.p
	brand := p.bold(p.TextPrimary, p.ic.Github+" ghx") + " " + p.badge(p.Blue, versionBadge())
	user := p.fg(p.Green, "●") + " " + p.bold(p.TextPrimary, "@"+m.data.User.Login)
	ctx := fmt.Sprintf("Context: All Repositories (%d)", len(m.data.Repos))
	if m.cfg.Org != "" {
		ctx = fmt.Sprintf("Org: %s (%d)", m.cfg.Org, len(m.data.Repos))
	}
	context := p.field(p.muted(p.ic.RepoPublic) + " " + p.bold(p.Blue, ctx) + p.muted("  ⌄"))
	demo := ""
	if m.data.Demo {
		demo = p.badge(p.Yellow, "DEMO")
	}

	unread := len(m.data.Notifications)
	inboxColor := p.Blue
	if unread == 0 {
		inboxColor = p.TextSecondary
	}
	glyphs := "Nerd Octicons"
	if m.cfg.ASCII {
		glyphs = "Unicode"
	}
	// One shared priority list across both sides so the lowest-value items drop first when narrow.
	items := []struct {
		s     string
		prio  int
		right bool
	}{
		{" " + brand, 10, false},
		{user, 9, false},
		{context, 7, false},
		{demo, 5, false},
		{p.field(p.muted("FONT:") + " " + p.text(glyphs) + p.muted(" ⌄")), 2, true},
		{p.field(p.muted("THEME:") + " " + p.fg(p.Cyan, p.Name) + p.muted(" ⌄")), 4, true},
		{p.badge(inboxColor, fmt.Sprintf("%s %d Inbox", p.ic.Bell, unread)), 8, true},
		{p.badge(p.TextSecondary, "? Keys"), 3, true},
		{p.muted(time.Now().Format("15:04 MST")), 1, true},
	}
	var left, right string
	for floor := 0; floor <= 10; floor++ {
		var l, r []string
		for _, it := range items {
			if it.prio < floor || it.s == "" {
				continue
			}
			if it.right {
				r = append(r, it.s)
			} else {
				l = append(l, it.s)
			}
		}
		left, right = strings.Join(l, "   "), strings.Join(r, "  ")+" "
		if width(left)+width(right)+2 <= w {
			break
		}
	}
	return paint(spread(left, right, w), p.BgSurface, w)
}

func (m Model) tabCount(i int) string {
	p := m.p
	switch i {
	case tabPRs:
		return p.badge(p.Green, fmt.Sprint(len(m.data.ReviewPRs)+len(m.data.MyPRs)))
	case tabActions:
		if f, _ := m.data.RunCounts(); f > 0 {
			return p.badge(p.Red, fmt.Sprintf("%d Fail", f))
		}
		return p.badge(p.Green, p.ic.CheckPass)
	case tabRepos:
		return p.badge(p.TextSecondary, fmt.Sprint(len(m.data.Repos)))
	case tabIssues:
		return p.badge(p.Yellow, fmt.Sprint(len(m.data.Issues)))
	}
	return ""
}

func (m Model) tabIcon(i int) string {
	p := m.p
	switch i {
	case tabPRs:
		return p.fg(p.Green, p.ic.PROpen)
	case tabActions:
		return p.fg(p.Cyan, p.ic.Play)
	case tabRepos:
		return p.fg(p.Blue, p.ic.RepoPublic)
	case tabIssues:
		return p.fg(p.Yellow, p.ic.IssueOpen)
	default:
		return p.fg(p.Blue, p.ic.Pulse)
	}
}

// tabSegment renders one tab button: key cap, icon, label and count badge.
func (m Model) tabSegment(i int) string {
	p := m.p
	name := tabNames[i]
	if m.width < 190 {
		name = shortTabNames[i]
	}
	label := p.muted(name)
	if i == m.tab {
		label = lipgloss.NewStyle().Foreground(lipgloss.Color(p.TextPrimary)).Bold(true).Underline(true).Render(name)
	}
	seg := " " + p.kbd(fmt.Sprint(i+1)) + " " + m.tabIcon(i) + " " + label
	if c := m.tabCount(i); c != "" {
		seg += " " + c
	}
	seg += " "
	if i == m.tab {
		return paint(seg, p.BgPrimary, width(seg))
	}
	return seg
}

// tabRanges returns the [start,end) x-range of each tab for mouse hit-testing.
func (m Model) tabRanges() [][2]int {
	ranges := make([][2]int, len(tabNames))
	x := 0
	for i := range tabNames {
		sw := width(m.tabSegment(i))
		ranges[i] = [2]int{x, x + sw}
		x += sw + 1
	}
	return ranges
}

// tabBar renders the workspace tabs plus the fuzzy filter box and Fetch button.
func (m Model) tabBar(w int) string {
	p := m.p
	var segs []string
	for i := range tabNames {
		segs = append(segs, m.tabSegment(i))
	}
	tabs := strings.Join(segs, p.line("│")) + p.line("│")

	fetch := p.badge(p.Green, p.ic.Running+" Fetch")
	if m.loading {
		fetch = p.badge(p.Yellow, m.spin()+" Syncing")
	} else if !m.data.Demo {
		fetch = p.badge(p.Green, p.ic.Running+" Fetch · "+ago(m.data.FetchedAt))
	}
	room := w - width(tabs) - width(fetch) - 4
	right := ""
	if room >= 18 {
		fw := min(room, 52)
		var content string
		switch {
		case m.searching:
			m.search.Width = fw - 5
			content = p.fg(p.Yellow, p.ic.Search) + " " + m.search.View()
		case m.filter != "":
			content = p.fg(p.Yellow, p.ic.Search+" "+m.filter) + p.dim("  esc clears")
		default:
			content = p.muted(p.ic.Search) + " " + p.dim("Filter PRs, repos, branches, workflows, issues… (/)")
		}
		right = paint(" "+fit(content, fw-2)+" ", p.BgPrimary, fw) + " " + fetch + " "
	} else if room >= 0 {
		right = fetch + " "
	}
	return paint(spread(tabs, right, w), p.BgSurface, w)
}

// tabRule is the nav bar's bottom border, with a heavy accent segment under the active tab.
func (m Model) tabRule(w int) string {
	p := m.p
	r := m.tabRanges()[m.tab]
	start, end := clamp(r[0], 0, w), clamp(r[1], 0, w)
	return paint(p.line(strings.Repeat("─", start))+p.fg(p.BorderActive, strings.Repeat("━", end-start))+p.line(strings.Repeat("─", w-end)), p.BgSurface, w)
}
