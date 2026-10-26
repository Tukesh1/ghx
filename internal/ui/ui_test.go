package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func init() { lipgloss.SetColorProfile(termenv.TrueColor) }

// Every frame must be exactly the terminal size: one extra column or row scrolls the header away.
func TestEveryFrameFitsTheTerminal(t *testing.T) {
	t.Setenv("GHX_NERD", "1")
	for ti, theme := range config.Themes {
		for _, ascii := range []bool{false, true} {
			cfg := config.New(theme.Name, "", true, ascii, true)
			frames, err := RenderCheck(cfg, gh.MockDashboard())
			if err != nil {
				t.Fatalf("theme %d ascii=%v: %v", ti, ascii, err)
			}
			for name, frame := range frames {
				var w, h int
				fmt.Sscanf(name, "%dx%d", &w, &h)
				lines := strings.Split(frame, "\n")
				if len(lines) != h {
					t.Errorf("%s/%s: %d lines, want %d", theme.Name, name, len(lines), h)
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != w {
						t.Errorf("%s/%s line %d: width %d, want %d", theme.Name, name, i+1, got, w)
						break
					}
					if strings.ContainsAny(ansi.Strip(l), "\t\r") {
						t.Errorf("%s/%s line %d contains a raw tab or carriage return", theme.Name, name, i+1)
					}
				}
			}
		}
	}
}

func TestHostileDataStaysOnOneLine(t *testing.T) {
	t.Setenv("GHX_NERD", "1")
	d := gh.MockDashboard()
	d.ReviewPRs[0].Title = "tabs\tand\nnewlines\r\x07bell"
	d.Repos[0].Description = "multi\nline\tdescription"
	d.Runs[0].FailureLog = []string{"FAIL\tpkg\t0.1s", "\x1b[31mred\x1b[0m"}
	cfg := config.New("", "", true, false, true)
	for tab := range tabNames {
		frame, err := Snapshot(cfg, d, tab, 120, 36, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(strings.Split(frame, "\n")); n != 36 {
			t.Fatalf("tab %d: %d lines, want 36", tab+1, n)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"a\tb":       "a b",
		"a\nb\r":     "a b ",
		"bell\x07":   "bell",
		"\x1b[1mok":  "\x1b[1mok",
		"plain":      "plain",
		"del\x7fete": "delete",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFitPadsAndTruncates(t *testing.T) {
	if got := fit("abc", 5); got != "abc  " {
		t.Errorf("pad: %q", got)
	}
	if got := ansi.StringWidth(fit("abcdefgh", 4)); got != 4 {
		t.Errorf("truncate width = %d", got)
	}
	if got := ansi.StringWidth(fit("\x1b[31mred text here\x1b[0m", 6)); got != 6 {
		t.Errorf("ansi truncate width = %d", got)
	}
}

func TestTableLayoutPrefersSpaciousRows(t *testing.T) {
	if _, mode := tableLayout(5, 0, 20, true); mode != (rowMode{2, 1}) {
		t.Errorf("roomy: got %+v", mode)
	}
	if _, mode := tableLayout(5, 0, 9, true); mode != (rowMode{1, 1}) {
		t.Errorf("medium: got %+v", mode)
	}
	if _, mode := tableLayout(7, 0, 5, true); mode != (rowMode{1, 0}) {
		t.Errorf("tight: got %+v", mode)
	}
	start, _ := tableLayout(20, 15, 5, false)
	if start == 0 {
		t.Errorf("selection beyond the window should scroll, start=%d", start)
	}
}

func TestFuzzy(t *testing.T) {
	for _, tc := range []struct {
		q, hay string
		want   bool
	}{
		{"", "anything", true},
		{"orb", "acme/orbit-engine", true},
		{"oe", "acme/orbit-engine", true},
		{"orbit main", "acme/orbit-engine main", true},
		{"zzz", "acme/orbit-engine", false},
	} {
		if got := fuzzy(tc.q, tc.hay); got != tc.want {
			t.Errorf("fuzzy(%q, %q) = %v", tc.q, tc.hay, got)
		}
	}
}

func TestBlend(t *testing.T) {
	if got := blend("#ffffff", "#000000", 0.5); got != "#808080" {
		t.Errorf("blend = %s", got)
	}
	if got := blend("#58a6ff", "#161b22", 0); got != "#161b22" {
		t.Errorf("alpha 0 = %s", got)
	}
}

func TestDownsample(t *testing.T) {
	got := downsample([]int{1, 1, 1, 1, 1, 1}, 3)
	if len(got) != 3 || got[0]+got[1]+got[2] != 6 {
		t.Errorf("downsample = %v", got)
	}
	if got := downsample([]int{1, 2}, 5); len(got) != 2 {
		t.Errorf("no-op downsample = %v", got)
	}
}
