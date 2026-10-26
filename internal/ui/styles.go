package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// palette wraps the active theme with rendering helpers.
type palette struct {
	config.Theme
	ic        Icons
	Selection string
}

func newPalette(t config.Theme, ic Icons) palette {
	return palette{Theme: t, ic: ic, Selection: blend(t.BorderActive, t.BgSurface, 0.18)}
}

// blend mixes fg over bg with alpha a, emulating CSS rgba() tints in TrueColor.
func blend(fg, bg string, a float64) string {
	parse := func(h string) (r, g, b float64) {
		h = strings.TrimPrefix(h, "#")
		if len(h) != 6 {
			return 0, 0, 0
		}
		v, _ := strconv.ParseUint(h, 16, 32)
		return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)
	}
	fr, fg2, fb := parse(fg)
	br, bg2, bb := parse(bg)
	mix := func(f, b float64) int { return int(b + (f-b)*a + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", mix(fr, br), mix(fg2, bg2), mix(fb, bb))
}

func (p palette) fg(hex, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(s)
}

func (p palette) bold(hex, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Bold(true).Render(s)
}

func (p palette) text(s string) string  { return p.fg(p.TextPrimary, s) }
func (p palette) muted(s string) string { return p.fg(p.TextSecondary, s) }
func (p palette) dim(s string) string   { return p.fg(p.TextMuted, s) }
func (p palette) line(s string) string  { return p.fg(p.BorderMuted, s) }

// badge renders a tinted chip: accent text on a translucent accent background (the design's .tui-badge).
func (p palette) badge(accent, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Background(lipgloss.Color(blend(accent, p.BgSurface, 0.18))).Bold(true).Render(" " + s + " ")
}

// pill renders a solid chip with dark text on an accent background.
func (p palette) pill(bg, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.BgPrimary)).Background(lipgloss.Color(bg)).Bold(true).Render(" " + s + " ")
}

// kbd renders a key cap like the design's .kbd-key.
func (p palette) kbd(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.TextSecondary)).Background(lipgloss.Color(p.BgPrimary)).Bold(true).Render(" " + s + " ")
}

// field renders an inset input-like box on the primary background.
func (p palette) field(s string) string {
	return paint(" "+s+" ", p.BgPrimary, width(s)+2)
}

func (p palette) stateIcon(state string) string {
	switch state {
	case gh.StateSuccess:
		return p.fg(p.Green, p.ic.CheckPass)
	case gh.StateFailure:
		return p.fg(p.Red, p.ic.CheckFail)
	case gh.StatePending:
		return p.fg(p.Yellow, p.ic.Running)
	default:
		return p.dim(p.ic.Pending)
	}
}

// ciBadge renders a CI rollup as a tinted chip with an optional ratio label.
func (p palette) ciBadge(state, label string) string {
	switch state {
	case gh.StateSuccess:
		if label == "" {
			label = "Pass"
		}
		return p.badge(p.Green, p.ic.CheckPass+" "+label)
	case gh.StateFailure:
		if label == "" {
			label = "Fail"
		}
		return p.badge(p.Red, p.ic.CheckFail+" "+label)
	case gh.StatePending:
		if label == "" {
			label = "Run"
		}
		return p.badge(p.Yellow, p.ic.Running+" "+label)
	default:
		return p.badge(p.TextSecondary, p.ic.Pending+" none")
	}
}

func (p palette) prIcon(pr gh.PullRequest) string {
	switch {
	case pr.Merged:
		return p.fg(p.Purple, p.ic.PRMerged)
	case pr.IsDraft:
		return p.fg(p.TextSecondary, p.ic.PRDraft)
	default:
		return p.fg(p.Green, p.ic.PROpen)
	}
}

// reviewBadge mirrors the design's Approved / Changes Req / Review Req chips.
func (p palette) reviewBadge(pr gh.PullRequest) string {
	switch {
	case pr.IsDraft:
		return p.badge(p.TextSecondary, p.ic.PRDraft+" Draft")
	case pr.ReviewDecision == gh.ReviewApproved:
		return p.badge(p.Green, p.ic.CheckPass+" Approved")
	case pr.ReviewDecision == gh.ReviewChangesRequested:
		return p.badge(p.Red, p.ic.CheckFail+" Changes Req")
	default:
		return p.badge(p.Yellow, p.ic.Clock+" Review Req")
	}
}

func (p palette) mergeableBadge(pr gh.PullRequest) string {
	switch {
	case pr.IsDraft:
		return p.badge(p.TextSecondary, "Draft")
	case pr.Mergeable && pr.ReviewDecision != gh.ReviewChangesRequested:
		return p.badge(p.Green, "Mergeable")
	default:
		return p.badge(p.Red, "Blocked")
	}
}

func (p palette) diff(add, del int) string {
	return p.fg(p.Green, fmt.Sprintf("+%d", add)) + p.dim(" / ") + p.fg(p.Red, fmt.Sprintf("-%d", del))
}

func (p palette) priorityBadge(pr string) string {
	label := priorityLabel(pr)
	switch pr {
	case "P0":
		return p.badge(p.Red, label)
	case "P1":
		return p.badge(p.Yellow, label)
	case "P2":
		return p.badge(p.Blue, label)
	default:
		return p.badge(p.TextSecondary, label)
	}
}

// checksRatio returns "passed/total" for a PR's check runs.
func checksRatio(pr gh.PullRequest) string {
	if len(pr.Checks) == 0 {
		return ""
	}
	ok := 0
	for _, c := range pr.Checks {
		if c.State == gh.StateSuccess {
			ok++
		}
	}
	return fmt.Sprintf("%d/%d", ok, len(pr.Checks))
}

// checksLabel is the design's CI chip text: "6/6 passed", "3/4 running", "2/3 (1 failed)".
func checksLabel(pr gh.PullRequest) string {
	if len(pr.Checks) == 0 {
		return ""
	}
	ok, failed := 0, 0
	for _, c := range pr.Checks {
		switch c.State {
		case gh.StateSuccess:
			ok++
		case gh.StateFailure:
			failed++
		}
	}
	switch {
	case failed > 0:
		return fmt.Sprintf("%d/%d failing", ok, len(pr.Checks))
	case ok < len(pr.Checks):
		return fmt.Sprintf("%d/%d running", ok, len(pr.Checks))
	default:
		return fmt.Sprintf("%d/%d passed", ok, len(pr.Checks))
	}
}

// priorityLabel expands P0..P3 to the design's "P0-Blocker" style.
func priorityLabel(pr string) string {
	switch pr {
	case "P0":
		return "P0-Blocker"
	case "P1":
		return "P1-High"
	case "P2":
		return "P2-Normal"
	default:
		return pr + "-Low"
	}
}

// quickAction is the single best next step for a PR, as a design-style button.
func (p palette) quickAction(pr gh.PullRequest) string {
	switch {
	case pr.NeedsMyReview && pr.ReviewDecision != gh.ReviewApproved:
		return p.badge(p.Green, p.ic.CheckPass+" Approve")
	case pr.MergeReady():
		return p.badge(p.Purple, p.ic.PRMerged+" Merge")
	default:
		return p.badge(p.TextSecondary, "Inspect")
	}
}

// width is the printable cell width of s, ignoring ANSI escapes.
func width(s string) int { return ansi.StringWidth(s) }

// sanitize makes untrusted text (logs, titles, descriptions) safe for one terminal line: tabs
// become spaces and control characters other than ESC are removed. Terminals expand a tab to up to
// 8 columns while width() counts it as 0, so an unsanitized tab overflows the line, wraps, and
// scrolls the header off screen.
func sanitize(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != 0x1b || r == 0x7f }) {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r == 0x1b:
			return r
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, s)
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = sanitize(s)
	if width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if pad := w - width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// fitRight right-aligns s within w cells.
func fitRight(s string, w int) string {
	if width(s) > w {
		return fit(s, w)
	}
	return strings.Repeat(" ", w-width(s)) + s
}

// spread places left and right text at opposite ends of a w-cell line, truncating left first.
func spread(left, right string, w int) string {
	if width(right) >= w {
		return fit(left, w)
	}
	gap := w - width(left) - width(right)
	if gap < 1 {
		return fit(left, w-width(right)-1) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// bgSeq returns the escape sequence that sets the background to hex in the current color profile.
func bgSeq(hex string) string {
	seq := lipgloss.ColorProfile().Color(hex).Sequence(true)
	if seq == "" {
		return ""
	}
	return "\x1b[" + seq + "m"
}

// paint fills a line's background, re-applying it after every SGR reset so nested styles keep it.
func paint(line, hex string, w int) string {
	seq := bgSeq(hex)
	line = fit(line, w)
	if seq == "" {
		return line
	}
	return seq + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq) + "\x1b[0m"
}

// cutLeft drops the first n cells of s while preserving escape sequences.
func cutLeft(s string, n int) string {
	var b strings.Builder
	cells := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
			}
			j++
			if j > len(s) {
				j = len(s)
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		if cells >= n {
			b.WriteString(s[i:])
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		cells += uniseg.StringWidth(string(r))
		i += size
		if cells > n {
			b.WriteString(strings.Repeat(" ", cells-n))
		}
	}
	return b.String()
}

// overlay splices box lines over base lines at column x, row y. Base lines must be w cells wide.
func overlay(base, box []string, x, y, w int) []string {
	out := make([]string, len(base))
	copy(out, base)
	for i, line := range box {
		row := y + i
		if row < 0 || row >= len(out) {
			continue
		}
		bw := width(line)
		left := fit(ansi.Truncate(out[row], x, ""), x)
		right := ""
		if x+bw < w {
			right = cutLeft(out[row], x+bw)
		}
		out[row] = left + "\x1b[0m" + line + "\x1b[0m" + right
	}
	return out
}

// panelChrome is the number of rows a panel spends on borders and its title bar.
const panelChrome = 3

// panel draws a w×h card: rounded border, an elevated title bar (title left, right-hand
// controls), a surface-colored body, and optional footer lines pinned to the bottom.
func (p palette) panel(title, right string, body, footer []string, w, h int, focused bool) string {
	if w < 6 || h < panelChrome {
		return ""
	}
	bc := p.BorderMuted
	if focused {
		bc = p.BorderActive
	}
	b := func(s string) string { return p.fg(bc, s) }
	inner := w - 4

	tc := p.TextPrimary
	if focused {
		tc = p.BorderActive
	}
	bar := spread(" "+lipgloss.NewStyle().Foreground(lipgloss.Color(tc)).Bold(true).Render(title), right+" ", w-2)

	lines := make([]string, 0, h)
	lines = append(lines, b("╭"+strings.Repeat("─", w-2)+"╮"))
	lines = append(lines, b("│")+paint(bar, p.BgElevated, w-2)+b("│"))
	room := h - panelChrome
	if len(footer) > room {
		footer = footer[:room]
	}
	bodyRoom := room - len(footer)
	for i := 0; i < bodyRoom; i++ {
		content := ""
		if i < len(body) {
			content = body[i]
		}
		lines = append(lines, b("│")+" "+fit(content, inner)+" "+b("│"))
	}
	for _, f := range footer {
		lines = append(lines, b("│")+" "+fit(f, inner)+" "+b("│"))
	}
	lines = append(lines, b("╰"+strings.Repeat("─", w-2)+"╯"))
	for i := range lines {
		lines[i] = paint(lines[i], p.BgSurface, w)
	}
	return strings.Join(lines, "\n")
}

// inset paints lines as a recessed box on the primary background (the design's inner bg-primary blocks).
func (p palette) inset(lines []string, w int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = paint(" "+fit(l, w-2)+" ", p.BgPrimary, w)
	}
	return out
}

// box renders an inset with an uppercase caption line.
func (p palette) box(caption string, lines []string, w int) []string {
	all := append([]string{p.bold(p.TextSecondary, strings.ToUpper(caption))}, lines...)
	return p.inset(all, w)
}

// sideBySide places two equal-height column blocks next to each other with a 1-cell gap.
func sideBySide(a, b []string, w int) []string {
	lw := (w - 1) / 2
	rw := w - lw - 1
	n := max(len(a), len(b))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		out[i] = fit(l, lw) + " " + fit(r, rw)
	}
	return out
}

// rule renders a labelled horizontal divider.
func (p palette) rule(label string, w int) string {
	head := p.bold(p.TextSecondary, label) + " "
	n := w - width(head)
	if n < 0 {
		n = 0
	}
	return head + p.line(strings.Repeat("─", n))
}

// column describes one table column. width<0 marks a flex column whose share is weighted by -width;
// drop>0 lets it be hidden when narrow (higher drops first).
type column struct {
	title string
	width int
	right bool
	drop  int
	min   int // minimum width for flex columns (default flexMin)
}

// cell joins a primary line and an optional secondary line for two-line table rows.
func cell(main, sub string) string {
	if sub == "" {
		return main
	}
	return main + "\n" + sub
}

func splitCell(s string) (string, string) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// rowMode is how a table row is drawn: lines of content plus an optional hairline separator.
type rowMode struct{ lines, sep int }

func (r rowMode) step() int { return r.lines + r.sep }

// tableLayout picks the roomiest row mode (two-line rows with hairlines, like the design) that fits,
// and the scroll offset that keeps sel visible.
func tableLayout(nrows, sel, visible int, hasSub bool) (start int, mode rowMode) {
	modes := []rowMode{{2, 1}, {1, 1}, {1, 0}}
	if !hasSub {
		modes = []rowMode{{1, 1}, {1, 0}}
	}
	mode = modes[len(modes)-1]
	want := min(nrows, 5)
	for _, md := range modes {
		if (visible+md.sep)/md.step() >= want {
			mode = md
			break
		}
	}
	capacity := max((visible+mode.sep)/mode.step(), 1)
	if sel >= capacity {
		start = sel - capacity + 1
	}
	return start, mode
}

func rowsHaveSub(rows [][]string) bool {
	for _, r := range rows {
		for _, c := range r {
			if strings.Contains(c, "\n") {
				return true
			}
		}
	}
	return false
}

// colGap is the spacing between table columns (the design's 8px cell padding).
const colGap = 2

// table renders an uppercase header strip plus rows windowed around sel, at most h lines.
func (p palette) table(cols []column, rows [][]string, sel, w, h int, focused bool) []string {
	if h <= 0 || w <= 0 {
		return nil
	}
	const flexMin = 14
	active := make([]int, len(cols))
	for i := range cols {
		active[i] = i
	}
	minOf := func(i int) int { return max(cols[i].min, flexMin) }
	need := func(idx []int) int {
		n := 1
		for _, i := range idx {
			if cols[i].width < 0 {
				n += minOf(i) + colGap
			} else {
				n += cols[i].width + colGap
			}
		}
		return n
	}
	for need(active) > w {
		worst, wi := 0, -1
		for k, i := range active {
			if cols[i].drop > worst {
				worst, wi = cols[i].drop, k
			}
		}
		if wi < 0 {
			break
		}
		active = append(active[:wi], active[wi+1:]...)
	}

	widths := make(map[int]int, len(active))
	fixed, flex, weights := 1, 0, 0
	for _, i := range active {
		if cols[i].width < 0 {
			flex++
			weights += -cols[i].width
		} else {
			fixed += cols[i].width + colGap
			widths[i] = cols[i].width
		}
	}
	if flex > 0 {
		rem := max(w-fixed-flex*colGap, flex)
		// Satisfy minimums first, then share what is left by weight.
		spare := rem
		for _, i := range active {
			if cols[i].width < 0 {
				widths[i] = min(minOf(i), rem)
				spare -= widths[i]
			}
		}
		spare = max(spare, 0)
		given, last := 0, -1
		for _, i := range active {
			if cols[i].width < 0 {
				add := spare * -cols[i].width / weights
				widths[i] += add
				given += add
				last = i
			}
		}
		widths[last] += spare - given
	}

	sep := strings.Repeat(" ", colGap)
	fmtCell := func(i int, s string) string {
		if cols[i].right {
			return fitRight(s, widths[i])
		}
		return fit(s, widths[i])
	}
	var hdr []string
	for _, i := range active {
		hdr = append(hdr, fmtCell(i, strings.ToUpper(cols[i].title)))
	}
	out := []string{paint(p.bold(p.TextSecondary, " "+strings.Join(hdr, sep)), p.BgPrimary, w)}

	visible := h - 1
	if visible <= 0 {
		return out
	}
	if len(rows) == 0 {
		return append(out, "", p.muted("  Nothing needs your attention here "+p.ic.CheckPass))
	}
	hair := p.fg(blend(p.BorderMuted, p.BgSurface, 0.55), strings.Repeat("─", w))
	start, mode := tableLayout(len(rows), sel, visible, rowsHaveSub(rows))
	for r := start; r < len(rows) && len(out)-1+mode.lines <= visible; r++ {
		if r > start && mode.sep > 0 {
			out = append(out, hair)
		}
		var mains, subs []string
		for _, i := range active {
			v := ""
			if i < len(rows[r]) {
				v = rows[r][i]
			}
			m, s := splitCell(v)
			if mode.lines == 1 && s != "" {
				m += "  " + s
			}
			mains = append(mains, fmtCell(i, m))
			subs = append(subs, fmtCell(i, s))
		}
		lines := []string{strings.Join(mains, sep)}
		if mode.lines == 2 {
			lines = append(lines, strings.Join(subs, sep))
		}
		for _, l := range lines {
			switch {
			case r == sel && focused:
				out = append(out, paint(p.fg(p.BorderActive, "▎")+l, p.Selection, w))
			case r == sel:
				out = append(out, paint(p.fg(p.BorderMuted, "▎")+l, p.BgElevated, w))
			default:
				out = append(out, fit(" "+l, w))
			}
		}
	}
	return out
}

// wrap word-wraps plain text to w cells and returns at most max lines (0 = unlimited).
func wrap(s string, w, max int) []string {
	if w <= 0 {
		return nil
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			switch {
			case cur == "":
				cur = word
			case width(cur)+1+width(word) <= w:
				cur += " " + word
			default:
				lines = append(lines, cur)
				cur = word
			}
		}
		lines = append(lines, cur)
	}
	if max > 0 && len(lines) > max {
		lines = lines[:max]
		lines[max-1] = fit(lines[max-1], w-1) + "…"
	}
	return lines
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
}

func dur(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func compact(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return commaInt(n)
	}
}

func commaInt(n int) string {
	s := fmt.Sprint(n)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// shortRepo drops the owner when it matches the viewer to save horizontal space.
func shortRepo(repo, login string) string {
	if strings.HasPrefix(repo, login+"/") {
		return strings.TrimPrefix(repo, login+"/")
	}
	return repo
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
