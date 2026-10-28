package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
)

var (
	sparkBlocks = []rune("▁▂▃▄▅▆▇█")
	lowerBlocks = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
)

// RenderCompactHeatmap packs 7 weekdays into 4 terminal rows using the upper half block,
// with the foreground as day 2r and the background as day 2r+1.
func RenderCompactHeatmap(weeks [][]int, palette [5]string) string {
	var rows [4]strings.Builder
	for r := 0; r < 4; r++ {
		for w := 0; w < len(weeks); w++ {
			if len(weeks[w]) < 7 {
				continue
			}
			topLevel := clamp(weeks[w][r*2], 0, 4)
			botLevel := 0
			if r*2+1 < 7 {
				botLevel = clamp(weeks[w][r*2+1], 0, 4)
			}
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(palette[topLevel]))
			if r*2+1 < 7 {
				style = style.Background(lipgloss.Color(palette[botLevel]))
			}
			rows[r].WriteString(style.Render("▀"))
		}
	}
	return strings.Join([]string{rows[0].String(), rows[1].String(), rows[2].String(), rows[3].String()}, "\n")
}

// RenderLanguageBar draws a 1-row segmented bar whose segment lengths follow each language's share.
func RenderLanguageBar(width int, langs []gh.LangStat) string {
	if width <= 0 || len(langs) == 0 {
		return ""
	}
	var bar strings.Builder
	used := 0
	for i, l := range langs {
		chars := max(1, int(math.Round(float64(width)*l.Percent/100.0)))
		if i == len(langs)-1 {
			chars = width - used
		}
		if used+chars > width {
			chars = width - used
		}
		if chars <= 0 {
			break
		}
		used += chars
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color))
		bar.WriteString(style.Render(strings.Repeat("█", chars)))
	}
	return bar.String()
}

// heatmapPalette returns the theme's green ramp; an empty level 0 uses the theme surface so it reads on the inset.
func (p palette) heatmapPalette() [5]string {
	pal := p.Heatmap
	if pal[0] == "" {
		pal[0] = p.BgSurface
	}
	return pal
}

// stackedBars renders a pass (green) / fail (red) stacked bar chart, height rows tall, colw cells
// per day separated by gap cells. Each cell is split into eighths with lower-block glyphs.
func (p palette) stackedBars(days []gh.CIDay, height, colw, gap int) []string {
	peak := 1
	for _, d := range days {
		peak = max(peak, d.Passed+d.Failed)
	}
	units := height * 8
	rows := make([]strings.Builder, height)
	for _, d := range days {
		total := d.Passed + d.Failed
		totalU := int(math.Round(float64(total) / float64(peak) * float64(units)))
		if total > 0 && totalU == 0 {
			totalU = 1
		}
		failU := 0
		if d.Failed > 0 {
			failU = max(1, int(math.Round(float64(d.Failed)/float64(total)*float64(totalU))))
		}
		passU := totalU - failU
		for r := 0; r < height; r++ {
			base := (height - 1 - r) * 8
			g := clamp(passU-base, 0, 8)
			t := clamp(totalU-base, 0, 8)
			var c string
			switch {
			case t == 0 && r == height-1:
				c = p.line("▁")
			case t == 0:
				c = " "
			case g == 8:
				c = p.fg(p.Green, "█")
			case g == 0:
				c = p.fg(p.Red, lowerBlocks[t])
			case t == 8:
				c = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)).Background(lipgloss.Color(p.Red)).Render(lowerBlocks[g])
			default:
				c = p.fg(p.Green, lowerBlocks[g])
			}
			rows[r].WriteString(strings.Repeat(c, colw) + strings.Repeat(" ", gap))
		}
	}
	out := make([]string, height)
	for i := range rows {
		out[i] = rows[i].String()
	}
	return out
}

// sparkline renders values as a single row of block characters.
func (p palette) sparkline(values []int, color string) string {
	peak := 1
	for _, v := range values {
		peak = max(peak, v)
	}
	var b strings.Builder
	for _, v := range values {
		if v <= 0 {
			b.WriteString(p.line("▁"))
			continue
		}
		idx := clamp(int(math.Ceil(float64(v)/float64(peak)*8))-1, 0, 7)
		b.WriteString(p.fg(color, string(sparkBlocks[idx])))
	}
	return b.String()
}

// progress renders a w-cell progress bar for pct in [0,100].
func (p palette) progress(pct float64, w int, color string) string {
	if w <= 0 {
		return ""
	}
	filled := clamp(int(math.Round(pct/100*float64(w))), 0, w)
	return p.fg(color, strings.Repeat("━", filled)) + p.line(strings.Repeat("━", w-filled))
}

// diffBar renders a proportional green/red additions-deletions bar.
func (p palette) diffBar(add, del, w int) string {
	total := add + del
	if total == 0 || w <= 0 {
		return p.line(strings.Repeat("■", max(w, 0)))
	}
	g := clamp(int(math.Round(float64(add)/float64(total)*float64(w))), 0, w)
	return p.fg(p.Green, strings.Repeat("■", g)) + p.fg(p.Red, strings.Repeat("■", w-g))
}

// langLegend renders "● Rust 38%" items until w cells are used.
func (p palette) langLegend(langs []gh.LangStat, w int) string {
	var out []string
	used := 0
	for _, l := range langs {
		item := p.fg(l.Color, "●") + " " + p.muted(fmt.Sprintf("%s %.0f%%", shortLang(l.Name), l.Percent))
		if used+width(item)+2 > w && len(out) > 0 {
			break
		}
		out = append(out, item)
		used += width(item) + 2
	}
	return strings.Join(out, "  ")
}

var langAbbrev = map[string]string{
	"TypeScript": "TS", "JavaScript": "JS", "Python": "Py", "Markdown": "MD",
	"Dockerfile": "Docker", "Jupyter Notebook": "Jupyter", "Objective-C": "ObjC",
}

func shortLang(name string) string {
	if s, ok := langAbbrev[name]; ok {
		return s
	}
	return name
}

// downsampleMax keeps the largest value of each bucket, so a narrowed chart's peak still matches
// the per-week numbers quoted next to it.
func downsampleMax(values []int, n int) []int {
	if n <= 0 || len(values) <= n {
		return values
	}
	out := make([]int, n)
	for i, v := range values {
		out[i*n/len(values)] = max(out[i*n/len(values)], v)
	}
	return out
}

// columnChart renders values as vertical bars height rows tall with a y-axis gutter showing the peak.
// Each value gets colw cells plus gap cells. The returned lines are all the same width.
func (p palette) columnChart(values []int, height, colw, gap int, color string) []string {
	peak := 1
	for _, v := range values {
		peak = max(peak, v)
	}
	axisW := len(compact(peak)) + 1
	units := height * 8
	rows := make([]strings.Builder, height)
	for _, v := range values {
		u := int(math.Round(float64(v) / float64(peak) * float64(units)))
		if v > 0 && u == 0 {
			u = 1
		}
		for r := 0; r < height; r++ {
			f := clamp(u-(height-1-r)*8, 0, 8)
			var c string
			switch {
			case f == 0 && r == height-1:
				c = p.line("▁")
			case f == 0:
				c = " "
			default:
				c = p.fg(color, lowerBlocks[f])
			}
			rows[r].WriteString(strings.Repeat(c, colw) + strings.Repeat(" ", gap))
		}
	}
	out := make([]string, height)
	for r := range rows {
		label := ""
		switch r {
		case 0:
			label = compact(peak)
		case height - 1:
			label = "0"
		}
		out[r] = p.dim(fitRight(label, axisW-1)) + p.line("┤") + rows[r].String()
	}
	return out
}

// chartAxisWidth is the y-axis gutter columnChart adds for a series.
func chartAxisWidth(values []int) int {
	peak := 1
	for _, v := range values {
		peak = max(peak, v)
	}
	return len(compact(peak)) + 1
}

// monthAxis labels weekly columns with month names where the month changes.
func (p palette) monthAxis(start time.Time, cols, colw, gap, indent int, weeksPerCol float64) string {
	cells := []rune(strings.Repeat(" ", cols*(colw+gap)))
	last := -1
	nextFree := 0
	for i := 0; i < cols; i++ {
		d := start.AddDate(0, 0, int(7*weeksPerCol*float64(i)))
		if int(d.Month()) != last {
			last = int(d.Month())
			pos := i * (colw + gap)
			name := d.Format("Jan")
			if pos >= nextFree && pos+len(name) <= len(cells) {
				copy(cells[pos:], []rune(name))
				nextFree = pos + len(name) + 1
			}
		}
	}
	return strings.Repeat(" ", indent) + p.dim(string(cells))
}

// divergingChart draws additions as green bars rising above a baseline and deletions as red bars
// hanging below it, up rows above and down rows below.
func (p palette) divergingChart(adds, dels []int, up, down, colw, gap int) []string {
	peak := 1
	for i := range adds {
		peak = max(peak, adds[i], dels[i])
	}
	out := make([]string, 0, up+down+1)
	for r := 0; r < up; r++ {
		var b strings.Builder
		for _, a := range adds {
			u := int(math.Round(float64(a) / float64(peak) * float64(up*8)))
			f := clamp(u-(up-1-r)*8, 0, 8)
			c := " "
			if f > 0 {
				c = p.fg(p.Green, lowerBlocks[f])
			}
			b.WriteString(strings.Repeat(c, colw) + strings.Repeat(" ", gap))
		}
		out = append(out, b.String())
	}
	out = append(out, p.line(strings.Repeat("─", len(adds)*(colw+gap))))
	for r := 0; r < down; r++ {
		var b strings.Builder
		for _, d := range dels {
			u := int(math.Round(float64(d) / float64(peak) * float64(down*2)))
			f := clamp(u-r*2, 0, 2)
			c := " "
			switch f {
			case 2:
				c = p.fg(p.Red, "█")
			case 1:
				c = p.fg(p.Red, "▀")
			}
			b.WriteString(strings.Repeat(c, colw) + strings.Repeat(" ", gap))
		}
		out = append(out, b.String())
	}
	return out
}

// shareBar renders a w-cell bar filled to share (0-1) in color, with the remainder as a faint track.
func (p palette) shareBar(share float64, w int, color string) string {
	if w <= 0 {
		return ""
	}
	full := share * float64(w)
	n := int(full)
	frac := int((full - float64(n)) * 8)
	bar := strings.Repeat("█", clamp(n, 0, w))
	rest := w - clamp(n, 0, w)
	if rest > 0 && frac > 0 {
		bar += []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}[frac]
		rest--
	}
	return p.fg(color, bar) + p.fg(blend(p.BorderMuted, p.BgSurface, 0.6), strings.Repeat("▁", max(rest, 0)))
}

// statTile renders a 3-line inset tile: label, big value, and a muted sub-line.
func (p palette) statTile(icon, label, value, sub, color string, w int) []string {
	return p.inset([]string{
		p.fg(color, icon) + " " + p.bold(p.TextSecondary, strings.ToUpper(label)),
		p.bold(color, value),
		p.muted(sub),
	}, w)
}

type tile struct{ icon, label, value, sub, color string }

// tileGrid lays tiles out perRow across w cells with 1-cell gutters and a blank line between rows.
func (p palette) tileGrid(tiles []tile, w, perRow int) []string {
	perRow = max(perRow, 1)
	tw := (w - (perRow - 1)) / perRow
	var out []string
	for i := 0; i < len(tiles); i += perRow {
		if i > 0 {
			out = append(out, "")
		}
		rows := make([]string, 3)
		for j := i; j < i+perRow && j < len(tiles); j++ {
			t := tiles[j]
			cell := p.statTile(t.icon, t.label, t.value, t.sub, t.color, tw)
			for k := range rows {
				if j > i {
					rows[k] += " "
				}
				rows[k] += cell[k]
			}
		}
		out = append(out, rows...)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
