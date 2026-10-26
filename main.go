package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/Tukesh1/ghx/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// Set at release time via -ldflags "-X main.version=… -X main.commit=… -X main.date=…".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// versionString reports the release version, falling back to module info for `go install` builds.
func versionString() string {
	v, c := version, commit
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && c == "" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	out := "ghx " + v
	if c != "" {
		out += " (" + c
		if date != "" {
			out += ", " + date
		}
		out += ")"
	}
	return out
}

const usage = `ghx — an action-first GitHub dashboard for your terminal

Usage:
  ghx [flags]

Examples:
  ghx                      live dashboard (uses ` + "`gh auth token`" + ` or $GITHUB_TOKEN)
  ghx --demo               offline demo with sample data
  ghx --org my-company     scope PRs, issues and repos to one organization
  ghx --theme tokyo-night  github-dark | tokyo-night | catppuccin-mocha | dracula

Environment:
  GITHUB_TOKEN / GH_TOKEN  token to use instead of the gh CLI
  GHX_THEME                default theme
  GHX_NERD=1 / GHX_ASCII=1 force Nerd Font Octicons / plain Unicode glyphs

Press ? inside ghx for all keyboard shortcuts.

Flags:
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
	demo := flag.Bool("demo", false, "run with the offline demo dataset")
	org := flag.String("org", "", "scope PRs, issues and repos to a GitHub org")
	theme := flag.String("theme", "", "github-dark | tokyo-night | catppuccin-mocha | dracula")
	ascii := flag.Bool("ascii", false, "use Unicode glyphs instead of Nerd Font Octicons (or GHX_ASCII=1)")
	check := flag.Bool("check", false, "render every tab at 80x24, 120x36 and 160x48, verify, and exit")
	snapshot := flag.Bool("snapshot", false, "print a single rendered frame and exit (see --tab, --size)")
	tab := flag.Int("tab", 1, "tab to render with --snapshot (1-5, or 6 for repository insights)")
	size := flag.String("size", "120x36", "terminal size for --snapshot, WxH")
	modals := flag.Bool("modals", false, "with --snapshot, also show the inbox drawer and help modal")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(versionString())
		return
	}

	cfg := config.New(*theme, *org, *demo, *ascii, *check)

	var client *gh.Client
	note := ""
	if !cfg.Demo {
		token, err := gh.Token()
		if err != nil {
			cfg.Demo = true
			note = "no GitHub auth found (gh auth login or GITHUB_TOKEN) · showing demo data"
		} else {
			client = gh.NewClient(token)
		}
	}

	if *check || *snapshot {
		lipgloss.SetColorProfile(termenv.TrueColor)
		data := loadOnce(client, cfg.Org)
		if *snapshot {
			os.Exit(runSnapshot(cfg, data, *tab, *size, *modals))
		}
		os.Exit(runCheck(cfg, data))
	}

	p := tea.NewProgram(ui.New(cfg, client, note), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghx:", err)
		os.Exit(1)
	}
}

// loadOnce fetches live data synchronously, falling back to the demo dataset.
func loadOnce(client *gh.Client, org string) *gh.Dashboard {
	if client == nil {
		return gh.MockDashboard()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	d, err := client.Fetch(ctx, org)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghx: live fetch failed, using demo data:", err)
		return gh.MockDashboard()
	}
	_ = gh.SaveCache(d, org)
	return d
}

func runSnapshot(cfg *config.Config, d *gh.Dashboard, tab int, size string, modals bool) int {
	var w, h int
	if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
		fmt.Fprintln(os.Stderr, "ghx: --size must look like 120x36")
		return 2
	}
	frame, err := ui.Snapshot(cfg, d, tab-1, w, h, modals, modals)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghx:", err)
		return 1
	}
	fmt.Println(frame)
	return 0
}

func runCheck(cfg *config.Config, d *gh.Dashboard) int {
	frames, err := ui.RenderCheck(cfg, d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✖ render failed:", err)
		return 1
	}
	keys := make([]string, 0, len(frames))
	for k := range frames {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	failed := 0
	for _, k := range keys {
		var w, h int
		fmt.Sscanf(k, "%dx%d", &w, &h)
		lines := strings.Split(frames[k], "\n")
		problem := ""
		switch {
		case !strings.Contains(frames[k], "\x1b["):
			problem = "no ANSI styling"
		case len(lines) != h:
			problem = fmt.Sprintf("%d lines, want %d", len(lines), h)
		default:
			for i, l := range lines {
				if lw := ansi.StringWidth(l); lw != w {
					problem = fmt.Sprintf("line %d is %d cells wide, want %d", i+1, lw, w)
					break
				}
			}
		}
		if problem != "" {
			failed++
			fmt.Printf("✖ %-32s %s\n", k, problem)
			continue
		}
		fmt.Printf("✔ %-32s %6d bytes\n", k, len(frames[k]))
	}
	if failed > 0 {
		fmt.Printf("\n%d of %d frames failed\n", failed, len(frames))
		return 1
	}
	fmt.Printf("\nall %d frames rendered cleanly (themes: %d, mode: %s)\n", len(frames), len(config.Themes), map[bool]string{true: "demo", false: "live"}[d.Demo])
	return 0
}
