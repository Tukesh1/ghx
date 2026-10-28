// Command screenshot renders ghx frames from the demo dataset as SVG images for the README.
//
//	go run ./tools/screenshot -out docs/screenshots
//
// Frames come from the same renderer the app uses (ui.Snapshot). Block and box-drawing characters
// are drawn as shapes rather than text, so charts and borders stay crisp in any font.
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/Tukesh1/ghx/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/rivo/uniseg"
)

type shot struct {
	name         string
	tab          int
	theme        string
	w, h         int
	notifs, help bool
}

var shots = []shot{
	{name: "profile", tab: ui.TabProfile, w: 160, h: 46},
	{name: "overview", tab: ui.TabOverview, w: 160, h: 46},
	{name: "pull-requests", tab: ui.TabPRs, w: 140, h: 40},
	{name: "actions", tab: ui.TabActions, w: 140, h: 40},
	{name: "repositories", tab: ui.TabRepos, w: 140, h: 40},
	{name: "insights", tab: ui.TabInsights, w: 160, h: 46},
	{name: "issues", tab: ui.TabIssues, w: 140, h: 40},
	{name: "inbox", tab: ui.TabOverview, w: 140, h: 40, notifs: true},
	{name: "theme-tokyo-night", tab: ui.TabOverview, theme: "tokyo-night", w: 140, h: 40},
	{name: "theme-catppuccin", tab: ui.TabPRs, theme: "catppuccin-mocha", w: 140, h: 40},
	{name: "theme-github-light", tab: ui.TabOverview, theme: "github-light", w: 140, h: 40},
}

func main() {
	out := flag.String("out", "docs/screenshots", "output directory")
	only := flag.String("only", "", "render just this shot")
	version := flag.String("version", "v0.2.0", "version shown in the header badge (the upcoming release)")
	flag.Parse()
	lipgloss.SetColorProfile(termenv.TrueColor)
	ui.Version = *version
	_ = os.Setenv("GHX_NERD", "")
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	for _, s := range shots {
		if *only != "" && s.name != *only {
			continue
		}
		// Unicode glyphs: README viewers do not have a Nerd Font.
		cfg := config.New(s.theme, "", true, true, true)
		frame, err := ui.Snapshot(cfg, nil, gh.MockDashboard(), s.tab, s.w, s.h, s.notifs, s.help)
		if err != nil {
			fail(err)
		}
		path := filepath.Join(*out, s.name+".svg")
		if err := os.WriteFile(path, []byte(render(frame, s.w, s.h, cfg.Theme())), 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "screenshot:", err)
	os.Exit(1)
}

const (
	cellW   = 8.4
	cellH   = 17.0
	fontPx  = 13.6
	padX    = 18.0
	padTop  = 40.0
	padBot  = 18.0
	radius  = 10.0
	fontCSS = `ui-monospace, "SF Mono", "Cascadia Mono", "JetBrains Mono", Menlo, Consolas, "DejaVu Sans Mono", monospace`
)

type style struct {
	fg, bg          string
	bold, underline bool
}

type cell struct {
	ch    string
	style style
}

// parse turns an ANSI frame into a grid of styled cells.
func parse(frame string, w, h int, def style) [][]cell {
	grid := make([][]cell, h)
	for y, line := range strings.SplitN(frame, "\n", h+1) {
		if y >= h {
			break
		}
		row := make([]cell, 0, w)
		st := def
		for len(line) > 0 {
			if strings.HasPrefix(line, "\x1b[") {
				end := strings.IndexFunc(line[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
				if end < 0 {
					break
				}
				if line[2+end] == 'm' {
					st = apply(st, def, line[2:2+end])
				}
				line = line[3+end:]
				continue
			}
			if line[0] == 0x1b {
				line = line[1:]
				continue
			}
			g, rest, width, _ := uniseg.FirstGraphemeClusterInString(line, -1)
			line = rest
			if width == 0 {
				continue
			}
			row = append(row, cell{g, st})
			for i := 1; i < width; i++ {
				row = append(row, cell{"", st})
			}
		}
		for len(row) < w {
			row = append(row, cell{" ", def})
		}
		grid[y] = row[:w]
	}
	for y := range grid {
		if grid[y] == nil {
			grid[y] = make([]cell, w)
			for x := range grid[y] {
				grid[y][x] = cell{" ", def}
			}
		}
	}
	return grid
}

func apply(st, def style, params string) style {
	ps := strings.Split(params, ";")
	if params == "" {
		return def
	}
	for i := 0; i < len(ps); i++ {
		switch ps[i] {
		case "0", "":
			st = def
		case "1":
			st.bold = true
		case "22":
			st.bold = false
		case "4":
			st.underline = true
		case "24":
			st.underline = false
		case "7":
			st.fg, st.bg = st.bg, st.fg
		case "39":
			st.fg = def.fg
		case "49":
			st.bg = def.bg
		case "38", "48":
			if i+4 < len(ps) && ps[i+1] == "2" {
				r, _ := strconv.Atoi(ps[i+2])
				g, _ := strconv.Atoi(ps[i+3])
				b, _ := strconv.Atoi(ps[i+4])
				hex := fmt.Sprintf("#%02x%02x%02x", r, g, b)
				if ps[i] == "38" {
					st.fg = hex
				} else {
					st.bg = hex
				}
				i += 4
			}
		}
	}
	return st
}

func render(frame string, w, h int, t config.Theme) string {
	def := style{fg: t.TextPrimary, bg: t.BgPrimary}
	grid := parse(frame, w, h, def)
	W := padX*2 + float64(w)*cellW
	H := padTop + padBot + float64(h)*cellH
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`+"\n", W, H, W, H)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="%.0f" fill="%s"/>`+"\n", radius, t.BgPrimary)
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="20" r="6" fill="%s"/>`, padX+6+float64(i)*20, c)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="24" fill="%s" font-family='%s' font-size="12" text-anchor="middle">ghx</text>`+"\n", W/2, t.TextSecondary, fontCSS)
	fmt.Fprintf(&b, `<g transform="translate(%.1f %.1f)" font-family='%s' font-size="%.1f">`+"\n", padX, padTop, fontCSS, fontPx)

	// Backgrounds: one rect per horizontal run of the same color.
	for y, row := range grid {
		for x := 0; x < w; {
			bg := row[x].style.bg
			end := x + 1
			for end < w && row[end].style.bg == bg {
				end++
			}
			if bg != t.BgPrimary {
				fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="%s"/>`, float64(x)*cellW, float64(y)*cellH, float64(end-x)*cellW+0.3, cellH+0.3, bg)
			}
			x = end
		}
	}
	b.WriteString("\n")
	// Shapes for block and box-drawing characters, text for everything else.
	for y, row := range grid {
		for x := 0; x < w; {
			c := row[x]
			if shape(&b, c, float64(x)*cellW, float64(y)*cellH) || c.ch == " " || c.ch == "" {
				x++
				continue
			}
			end := x + 1
			for end < w && row[end].style == c.style && row[end].ch != "" && !isShape(row[end].ch) && row[end].ch != " " {
				end++
			}
			var xs []string
			var txt strings.Builder
			for i := x; i < end; i++ {
				xs = append(xs, fmt.Sprintf("%.1f", float64(i)*cellW))
				txt.WriteString(row[i].ch)
			}
			attrs := ""
			if c.style.bold {
				attrs += ` font-weight="700"`
			}
			if c.style.underline {
				attrs += ` text-decoration="underline"`
			}
			fmt.Fprintf(&b, `<text x="%s" y="%.2f" fill="%s"%s>%s</text>`, strings.Join(xs, " "), float64(y)*cellH+cellH*0.76, c.style.fg, attrs, html.EscapeString(txt.String()))
			x = end
		}
		b.WriteString("\n")
	}
	b.WriteString("</g>\n</svg>\n")
	return b.String()
}

var eighths = map[string]int{"▁": 1, "▂": 2, "▃": 3, "▄": 4, "▅": 5, "▆": 6, "▇": 7, "█": 8}
var lefts = map[string]int{"▏": 1, "▎": 2, "▍": 3, "▌": 4, "▋": 5, "▊": 6, "▉": 7}

func isShape(ch string) bool {
	_, a := eighths[ch]
	_, l := lefts[ch]
	return a || l || strings.Contains("▀─━│╭╮╰╯┤■", ch)
}

// shape draws block elements and box-drawing characters as geometry.
func shape(b *strings.Builder, c cell, x, y float64) bool {
	fg := c.style.fg
	rect := func(rx, ry, rw, rh float64) {
		fmt.Fprintf(b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="%s"/>`, rx, ry, rw, rh, fg)
	}
	line := func(x1, y1, x2, y2, sw float64) {
		fmt.Fprintf(b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="%s" stroke-width="%.1f"/>`, x1, y1, x2, y2, fg, sw)
	}
	mx, my := x+cellW/2, y+cellH/2
	if n, ok := eighths[c.ch]; ok {
		hgt := cellH * float64(n) / 8
		rect(x, y+cellH-hgt, cellW+0.3, hgt+0.3)
		return true
	}
	if n, ok := lefts[c.ch]; ok {
		rect(x, y, cellW*float64(n)/8, cellH+0.3)
		return true
	}
	switch c.ch {
	case "▀":
		rect(x, y, cellW+0.3, cellH/2+0.2)
	case "■":
		rect(x+cellW*0.15, y+cellH*0.3, cellW*0.7, cellH*0.45)
	case "─":
		line(x, my, x+cellW+0.3, my, 1)
	case "━":
		line(x, my, x+cellW+0.3, my, 2.4)
	case "│":
		line(mx, y, mx, y+cellH+0.3, 1)
	case "┤":
		line(mx, y, mx, y+cellH+0.3, 1)
		line(x, my, mx, my, 1)
	case "╭":
		fmt.Fprintf(b, `<path d="M%.2f %.2f V%.2f Q%.2f %.2f %.2f %.2f H%.2f" fill="none" stroke="%s"/>`, mx, y+cellH+0.3, my+4, mx, my, mx+4, my, x+cellW+0.3, fg)
	case "╮":
		fmt.Fprintf(b, `<path d="M%.2f %.2f H%.2f Q%.2f %.2f %.2f %.2f V%.2f" fill="none" stroke="%s"/>`, x, my, mx-4, mx, my, mx, my+4, y+cellH+0.3, fg)
	case "╰":
		fmt.Fprintf(b, `<path d="M%.2f %.2f V%.2f Q%.2f %.2f %.2f %.2f H%.2f" fill="none" stroke="%s"/>`, mx, y, my-4, mx, my, mx+4, my, x+cellW+0.3, fg)
	case "╯":
		fmt.Fprintf(b, `<path d="M%.2f %.2f H%.2f Q%.2f %.2f %.2f %.2f V%.2f" fill="none" stroke="%s"/>`, x, my, mx-4, mx, my, mx, my-4, y, fg)
	default:
		return false
	}
	return true
}
