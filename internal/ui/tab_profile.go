package ui

import (
	"fmt"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The Profile tab mirrors github.com/<login>: a sidebar with the avatar and profile details, and a
// main column with the profile README, pinned repositories, the contribution graph and where the
// year's commits went.

const (
	pinCardH   = 4  // lines per pinned-repository card
	pinCardMin = 30 // narrowest card before the grid drops a column
	profBandH  = 9  // contribution graph band
)

// profileGeom holds the Profile tab's panel sizes, shared by View and mouse hit-testing.
type profileGeom struct {
	sw, rw           int // sidebar and main column widths
	readme, pinned   int // panel heights in the main column; 0 = hidden
	band             int // contribution band height; 0 = hidden
	pinnedY, bandY   int // y offsets of the pinned panel and the band
	gap              int
	cols             int // pinned cards per row; 0 = one line per repository
	heatW, activityW int // band split; activityW 0 = heatmap only
}

func (m Model) profileLayout(w, h int) profileGeom {
	g := profileGeom{sw: clamp(w*28/100, 28, 44)}
	g.rw = w - g.sw - 1
	if h >= 24 {
		g.gap = 1
	}
	avail := h
	if h-profBandH-g.gap >= 7 {
		g.band = profBandH
		avail -= profBandH + g.gap
		heat := heatmapBlockWidth(len(m.data.Weeks()), true) + 4
		g.heatW = g.rw
		if g.rw >= heat+1+30 {
			g.heatW, g.activityW = heat, g.rw-heat-1
		}
	}

	pr := m.data.Profile
	n := len(pr.Pinned)
	readme := len(m.readmeLines(pr.Readme, g.rw-4))
	maxCols := clamp((g.rw-4+1)/(pinCardMin+1), 1, max(min(3, n), 1))
	need := func(cols int) int {
		rows := (n + cols - 1) / cols
		return panelChrome + rows*pinCardH + rows - 1
	}
	// fitCols picks the fewest columns whose card grid fits hgt (taller grids fill the panel), or the
	// one-line list when no grid fits.
	fitCols := func(hgt int) int {
		for cols := 1; cols <= maxCols; cols++ {
			if need(cols) <= hgt {
				return cols
			}
		}
		return 0
	}
	switch {
	case n == 0:
		g.readme = avail // the README panel explains what is missing
	case readme == 0:
		g.pinned = avail
	default:
		minPinned := need(maxCols)
		if minPinned > avail-g.gap-5 {
			minPinned = min(panelChrome+n, avail-g.gap-5)
		}
		if minPinned < panelChrome+1 {
			g.pinned = avail // no room for both: pinned repositories win
			break
		}
		g.readme = clamp(panelChrome+readme, 5, avail-g.gap-minPinned)
		g.pinned = avail - g.readme - g.gap
	}
	if g.pinned > 0 {
		g.cols = fitCols(g.pinned)
	}
	if g.readme > 0 {
		g.pinnedY = g.readme + g.gap
	}
	g.bandY = avail + g.gap
	return g
}

// viewProfile renders Tab [1].
func (m Model) viewProfile(w, h int) string {
	p := m.p
	pr := m.data.Profile
	if pr == nil {
		msg := p.muted("Your profile couldn't be loaded · press ") + p.kbd("R") + p.muted(" to retry")
		if m.loading {
			msg = p.fg(p.Yellow, m.spin()) + p.muted(" loading your profile…")
		}
		return p.panel(p.fg(p.Purple, p.ic.Person)+" PROFILE", "", []string{"", msg}, nil, w, h, false)
	}
	g := m.profileLayout(w, h)
	var main []string
	if g.readme > 0 {
		main = append(main, m.readmePanel(pr, g.rw, g.readme))
	}
	if g.pinned > 0 {
		main = append(main, m.pinnedPanel(pr, g))
	}
	if g.band > 0 {
		band := m.heatmapPanel(g.heatW, g.band)
		if g.activityW > 0 {
			band = hjoin(band, m.activityPanel(pr, g.activityW, g.band))
		}
		main = append(main, band)
	}
	sep := "\n"
	if g.gap > 0 {
		sep = "\n\n"
	}
	return hjoin(m.profileCard(pr, g.sw, h), strings.Join(main, sep))
}

// profileCard is the sidebar: avatar, name, login, status, bio, followers, details, highlights,
// organizations and top languages, in that order of importance (the bottom is cut first).
func (m Model) profileCard(pr *gh.Profile, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome

	name := pr.Name
	if name == "" {
		name = pr.Login
	}
	login := p.muted(pr.Login)
	if pr.Pronouns != "" {
		login += p.dim(" · " + pr.Pronouns)
	}
	follow := p.muted(p.ic.Person+" ") + p.bold(p.TextPrimary, compact(pr.Followers)) + p.muted(" followers · ") +
		p.bold(p.TextPrimary, compact(pr.Following)) + p.muted(" following")
	status := ""
	if pr.StatusMessage != "" {
		e := statusEmojis[pr.StatusEmoji]
		if e != "" {
			e += " "
		}
		status = p.field(p.muted(fit(e+sanitize(pr.StatusMessage), inner-2)))
	}

	var details []string
	detail := func(icon, s string, color string) {
		if s != "" {
			details = append(details, p.muted(icon)+" "+p.fg(color, sanitize(s)))
		}
	}
	detail(p.ic.Org, pr.Company, p.TextPrimary)
	detail(p.ic.Location, pr.Location, p.TextPrimary)
	detail(p.ic.Link, trimScheme(pr.Website), p.Blue)
	if pr.Twitter != "" {
		detail(p.ic.Link, "x.com/"+pr.Twitter, p.Blue)
	}
	for _, s := range pr.Socials {
		detail(p.ic.Link, firstNonEmpty(s.Name, trimScheme(s.URL)), p.Blue)
	}
	if !pr.CreatedAt.IsZero() {
		detail(p.ic.Calendar, "Joined "+pr.CreatedAt.Format("Jan 2006"), p.TextSecondary)
	}

	// Lines beside (compact) or under (large) the avatar, then everything else.
	head := []string{p.bold(p.TextPrimary, name), login}
	var rest []string
	if status != "" {
		rest = append(rest, status)
	}
	for _, l := range wrap(sanitize(pr.Bio), inner, 3) {
		if l != "" {
			rest = append(rest, p.text(l))
		}
	}
	rest = append(rest, "", follow)
	if len(details) > 0 {
		rest = append(append(rest, ""), details...)
	}
	stars := p.fg(p.Yellow, p.ic.Star) + " " + p.bold(p.TextPrimary, compact(pr.StarsEarned)) + p.muted(" stars  ") +
		p.fg(p.Blue, p.ic.RepoPublic) + " " + p.bold(p.TextPrimary, compact(pr.Repos)) + p.muted(" repos")
	if forks := "  " + p.muted(p.ic.Fork) + " " + p.bold(p.TextPrimary, compact(pr.ForksEarned)) + p.muted(" forks"); width(stars+forks) <= inner {
		stars += forks
	}
	rest = append(rest, "", stars)
	essential := len(rest)
	if len(pr.Orgs) > 0 {
		rest = append(rest, "", p.bold(p.TextSecondary, "ORGANIZATIONS"))
		orgs := make([]string, len(pr.Orgs))
		for i, o := range pr.Orgs {
			orgs[i] = "@" + o
		}
		for _, l := range wrap(strings.Join(orgs, "  "), inner, 2) {
			rest = append(rest, p.fg(p.Blue, l))
		}
	}
	if len(pr.Languages) > 0 {
		rest = append(rest, "", p.bold(p.TextSecondary, "TOP LANGUAGES"), RenderLanguageBar(inner, pr.Languages), p.langLegend(pr.Languages, inner))
	}

	// The avatar takes what the text leaves, between 4 and 10 rows (2 pixels per row, so a rows×2
	// cell square). If even the details would be cut, it shrinks first; below 4 rows it moves beside
	// the name.
	rows := clamp(room-1-len(head)-len(rest), 0, 10)
	if rows < 5 {
		rows = clamp(room-1-len(head)-essential, 0, 10)
	}
	rows = min(rows, (inner-2)/2)
	avatar := pr.Avatar
	if avatar == nil {
		avatar = gh.Identicon(pr.Login, 32)
	}
	var body []string
	if rows >= 4 {
		pad := strings.Repeat(" ", (inner-2*rows)/2)
		for _, l := range m.avatarLines(avatar, rows) {
			body = append(body, pad+l)
		}
		body = append(body, "")
		body = append(append(body, head...), rest...)
	} else {
		side := append(head, follow)
		for i, l := range m.avatarLines(avatar, 3) {
			body = append(body, l+"  "+fit(side[i], inner-8))
		}
		// follow is shown beside the avatar; drop its copy (and the blank before it) from the rest.
		var kept []string
		for i, l := range rest {
			if l == follow || (l == "" && i+1 < len(rest) && rest[i+1] == follow) {
				continue
			}
			kept = append(kept, l)
		}
		body = append(body, kept...)
	}
	title := p.fg(p.Purple, p.ic.Person) + " PROFILE"
	link := linkFits(title, p.fg(p.Blue, trimScheme(pr.URL())), w)
	return p.panel(title, link, body, nil, w, h, false)
}

// avatarLines draws the avatar as a circle, rows terminal rows tall and 2·rows cells wide: each
// cell is an upper half block with the top pixel as foreground and the bottom one as background.
func (m Model) avatarLines(px *gh.Pixels, rows int) []string {
	n := 2 * rows
	grid := make([][]string, n)
	r := float64(n)/2 + 0.25
	for y := 0; y < n; y++ {
		grid[y] = make([]string, n)
		for x := 0; x < n; x++ {
			dx, dy := float64(x)+0.5-float64(n)/2, float64(y)+0.5-float64(n)/2
			if dx*dx+dy*dy <= r*r {
				grid[y][x] = sample(px, x, y, n)
			}
		}
	}
	out := make([]string, rows)
	for row := 0; row < rows; row++ {
		var b strings.Builder
		for x := 0; x < n; x++ {
			top, bot := grid[2*row][x], grid[2*row+1][x]
			st := lipgloss.NewStyle()
			switch {
			case top != "" && bot != "":
				b.WriteString(st.Foreground(lipgloss.Color(top)).Background(lipgloss.Color(bot)).Render("▀"))
			case top != "":
				b.WriteString(st.Foreground(lipgloss.Color(top)).Render("▀"))
			case bot != "":
				b.WriteString(st.Foreground(lipgloss.Color(bot)).Render("▄"))
			default:
				b.WriteString(" ")
			}
		}
		out[row] = b.String()
	}
	return out
}

// sample box-filters the source pixels covering cell (x, y) of an n×n grid.
func sample(px *gh.Pixels, x, y, n int) string {
	x0, x1 := x*px.W/n, max((x+1)*px.W/n, x*px.W/n+1)
	y0, y1 := y*px.H/n, max((y+1)*px.H/n, y*px.H/n+1)
	var r, g, b, c int
	for sy := y0; sy < y1 && sy < px.H; sy++ {
		for sx := x0; sx < x1 && sx < px.W; sx++ {
			i := 3 * (sy*px.W + sx)
			if i+2 >= len(px.RGB) {
				continue
			}
			r, g, b, c = r+int(px.RGB[i]), g+int(px.RGB[i+1]), b+int(px.RGB[i+2]), c+1
		}
	}
	if c == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r/c, g/c, b/c)
}

// readmeLines styles the cleaned profile README for a w-wide panel.
func (m Model) readmeLines(text string, w int) []string {
	p := m.p
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = sanitize(l)
		switch {
		case l == "":
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
		case strings.HasPrefix(l, "# "):
			for _, s := range wrap(strings.TrimPrefix(l, "# "), w, 2) {
				out = append(out, p.bold(p.TextPrimary, s))
			}
		case strings.HasPrefix(l, "• "):
			for i, s := range wrap(strings.TrimPrefix(l, "• "), w-2, 0) {
				lead := "  "
				if i == 0 {
					lead = p.fg(p.Blue, "•") + " "
				}
				out = append(out, lead+p.text(s))
			}
		case strings.HasPrefix(l, "  "):
			out = append(out, p.fg(p.Cyan, l))
		default:
			for _, s := range wrap(l, w, 0) {
				out = append(out, p.text(s))
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func (m Model) readmePanel(pr *gh.Profile, w, h int) string {
	p := m.p
	inner := w - 4
	lines := m.readmeLines(pr.Readme, inner)
	title := p.muted(p.ic.File) + " " + pr.Login + p.muted(" / ") + "README.md"
	if len(lines) == 0 {
		hint := "No profile README or pinned repositories yet."
		if len(pr.Pinned) > 0 {
			hint = "No profile README yet."
		}
		lines = []string{p.muted(hint), p.dim(fmt.Sprintf("Create a repository named %s with a README.md to introduce yourself here.", pr.Login))}
		title = p.muted(p.ic.File) + " README"
	}
	if room := h - panelChrome; len(lines) > room && room > 0 {
		lines = append(lines[:room-1], p.dim("… continued on "+trimScheme(pr.URL())+" · press")+p.kbd("O"))
	}
	return p.panel(title, "", lines, nil, w, h, false)
}

// pinnedCardW is the width of one card in a cols-wide grid inside a w-wide panel.
func pinnedCardW(w, cols int) int { return (w - 4 - (cols - 1)) / cols }

// pinnedListStart scrolls the one-line list so the cursor stays visible.
func pinnedListStart(n, cursor, visible int) int {
	return clamp(cursor-visible+1, 0, max(n-visible, 0))
}

func (m Model) pinnedPanel(pr *gh.Profile, g profileGeom) string {
	p := m.p
	w, h := g.rw, g.pinned
	inner := w - 4
	items := m.pinned()
	sel := clamp(m.cursor[kindPinned], 0, len(items)-1)
	title := p.fg(p.Blue, p.ic.Pin) + " PINNED"
	if pr.Popular {
		title = p.fg(p.Blue, p.ic.Star) + " POPULAR REPOSITORIES"
	}
	right := linkFits(title, p.dim("⏎ open · c clone · y copy"), w)
	if m.filter != "" {
		right = p.fg(p.Yellow, fmt.Sprintf("%d of %d", len(items), len(pr.Pinned)))
	}
	var body []string
	switch {
	case len(items) == 0:
		body = []string{p.muted("no pinned repositories match “" + m.filter + "”")}
	case g.cols > 0:
		cw := pinnedCardW(w, g.cols)
		for start := 0; start < len(items); start += g.cols {
			if start > 0 {
				body = append(body, "")
			}
			cards := make([][]string, 0, g.cols)
			for i := start; i < min(start+g.cols, len(items)); i++ {
				cards = append(cards, m.pinnedCard(items[i], cw, i == sel))
			}
			for line := 0; line < pinCardH; line++ {
				parts := make([]string, len(cards))
				for c := range cards {
					parts[c] = cards[c][line]
				}
				body = append(body, strings.Join(parts, " "))
			}
		}
	default:
		visible := h - panelChrome
		start := pinnedListStart(len(items), sel, visible)
		for i := start; i < len(items) && i < start+visible; i++ {
			body = append(body, m.pinnedRow(items[i], inner, i == sel))
		}
	}
	return p.panel(title, right, body, nil, w, h, true)
}

func (m Model) pinnedMeta(r gh.PinnedRepo) string {
	p := m.p
	var parts []string
	if r.Language != "" {
		color := r.LanguageColor
		if color == "" {
			color = p.TextSecondary
		}
		parts = append(parts, p.fg(color, "●")+" "+p.muted(r.Language))
	}
	parts = append(parts, p.muted(p.ic.Star+" "+compact(r.Stars)))
	if r.Forks > 0 {
		parts = append(parts, p.muted(p.ic.Fork+" "+compact(r.Forks)))
	}
	return strings.Join(parts, "  ")
}

// pinnedCard is one repository card, like github.com's: name and visibility, description (or the
// fork's upstream), language, stars and forks.
func (m Model) pinnedCard(r gh.PinnedRepo, w int, selected bool) []string {
	p := m.p
	inner := w - 2
	vis, icon := "Public", p.ic.RepoPublic
	if r.IsPrivate {
		vis, icon = "Private", p.ic.RepoPrivate
	}
	badge := p.badge(p.TextSecondary, vis)
	nameW := inner - width(badge) - 3
	head := spread(p.muted(icon)+" "+m.repoRefMax(r.NameWithOwner, nameW), badge, inner)
	desc := sanitize(r.Description)
	var mid []string
	if r.Parent != "" {
		mid = append(mid, p.dim(fit("Forked from "+r.Parent, inner)))
		if desc != "" {
			mid = append(mid, p.muted(fit(desc, inner)))
		}
	} else {
		for _, l := range wrap(desc, inner, 2) {
			mid = append(mid, p.muted(l))
		}
	}
	for len(mid) < 2 {
		mid = append(mid, "")
	}
	bg := p.BgPrimary
	if selected {
		bg = p.Selection
	}
	lines := []string{head, mid[0], mid[1], m.pinnedMeta(r)}
	for i, l := range lines {
		lines[i] = paint(" "+fit(l, inner)+" ", bg, w)
	}
	return lines
}

// pinnedRow is the one-line form used when cards do not fit.
func (m Model) pinnedRow(r gh.PinnedRepo, w int, selected bool) string {
	p := m.p
	icon := p.ic.RepoPublic
	if r.IsPrivate {
		icon = p.ic.RepoPrivate
	}
	meta := m.pinnedMeta(r)
	left := p.muted(icon) + " " + m.repoRefMax(r.NameWithOwner, max(w/2, 12))
	if d := sanitize(r.Description); d != "" {
		left += "  " + p.dim(d)
	}
	line := spread(left, meta, w)
	if selected {
		return paint(line, p.Selection, w)
	}
	return line
}

// activityPanel shows where the year's commits went, like github.com's contribution activity.
func (m Model) activityPanel(pr *gh.Profile, w, h int) string {
	p := m.p
	inner := w - 4
	room := h - panelChrome
	var body []string
	top := 1
	for _, c := range pr.CommitRepos {
		top = max(top, c.Commits)
	}
	nameW := clamp(inner*2/5, 10, 26)
	countW := 6
	for i, c := range pr.CommitRepos {
		if i >= room-1 {
			break
		}
		bar := p.shareBar(float64(c.Commits)/float64(top), inner-nameW-countW-2, p.Green)
		body = append(body, fit(m.repoRefMax(c.Repo, nameW), nameW)+" "+bar+" "+fitRight(p.text(commaInt(c.Commits)), countW))
	}
	if len(body) == 0 {
		body = append(body, p.muted("no public commits in the last year"))
	}
	for len(body) < room-1 {
		body = append(body, "")
	}
	summary := plural(pr.ReposCreated, "repo") + " created"
	if pr.PrivateContrib > 0 {
		summary += " · " + commaInt(pr.PrivateContrib) + " private contributions"
	}
	body = append(body, p.dim(summary))
	title := p.fg(p.Green, p.ic.Commit) + " COMMITS BY REPOSITORY"
	right := linkFits(title, p.dim("last 12 months"), w)
	if width(title)+5 > w {
		title = p.fg(p.Green, p.ic.Commit) + " COMMITS"
	}
	return p.panel(title, right, body, nil, w, h, false)
}

// clickProfile handles a click inside the Profile body: the sidebar title opens the profile, a
// pinned repository is selected, and a second click on it opens it.
func (m Model) clickProfile(x, y, w, h int) (tea.Model, tea.Cmd) {
	if m.data.Profile == nil {
		return m, nil
	}
	g := m.profileLayout(w, h)
	if x < g.sw {
		if y == 1 {
			return m.openProfile()
		}
		return m, nil
	}
	x -= g.sw + 1 + 2 // main column, then the panel border and padding
	y -= g.pinnedY + 2
	if g.pinned == 0 || x < 0 || y < 0 || y >= g.pinned-panelChrome {
		return m, nil
	}
	items := m.pinned()
	idx := -1
	if g.cols > 0 {
		cw := pinnedCardW(g.rw, g.cols)
		row, line := y/(pinCardH+1), y%(pinCardH+1)
		col := x / (cw + 1)
		if line < pinCardH && x%(cw+1) < cw && col < g.cols {
			idx = row*g.cols + col
		}
	} else {
		idx = pinnedListStart(len(items), m.cursor[kindPinned], g.pinned-panelChrome) + y
	}
	if idx < 0 || idx >= len(items) {
		return m, nil
	}
	if idx == m.cursor[kindPinned] {
		return m.openInBrowser()
	}
	m.cursor[kindPinned] = idx
	return m, nil
}

// pinned lists the profile's pinned repositories that match the filter.
func (m Model) pinned() []gh.PinnedRepo {
	if m.data == nil || m.data.Profile == nil {
		return nil
	}
	var out []gh.PinnedRepo
	for _, r := range m.data.Profile.Pinned {
		if fuzzy(m.filter, r.NameWithOwner+" "+r.Description+" "+r.Language) {
			out = append(out, r)
		}
	}
	return out
}

func (m Model) selectedPinned() (gh.PinnedRepo, bool) {
	l := m.pinned()
	if len(l) == 0 {
		return gh.PinnedRepo{}, false
	}
	return l[clamp(m.cursor[kindPinned], 0, len(l)-1)], true
}

func trimScheme(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(u, "/")
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// statusEmojis maps common GitHub status shortcodes to emoji. Only single-codepoint emoji with a
// default emoji presentation are listed, so every terminal agrees they are two cells wide.
var statusEmojis = map[string]string{
	":house:": "🏠", ":house_with_garden:": "🏡", ":rocket:": "🚀", ":dart:": "🎯", ":palm_tree:": "🌴",
	":computer:": "💻", ":coffee:": "☕", ":zap:": "⚡", ":fire:": "🔥", ":sparkles:": "✨", ":tada:": "🎉",
	":books:": "📚", ":thought_balloon:": "💭", ":speech_balloon:": "💬", ":bulb:": "💡", ":eyes:": "👀",
	":face_with_thermometer:": "🤒", ":red_circle:": "🔴", ":wave:": "👋", ":construction:": "🚧",
	":calendar:": "📆", ":headphones:": "🎧", ":seedling:": "🌱", ":thinking:": "🤔", ":sleeping:": "😴",
	":hammer:": "🔨", ":bug:": "🐛", ":mag:": "🔍", ":memo:": "📝", ":rainbow:": "🌈", ":star:": "⭐",
}
