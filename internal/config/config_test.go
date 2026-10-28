package config

import (
	"testing"
	"time"
)

// isolate points settings at a temporary directory and clears glyph overrides.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("GHX_CONFIG_DIR", t.TempDir())
	t.Setenv("GHX_THEME", "")
	t.Setenv("GHX_ASCII", "")
	t.Setenv("GHX_NERD", "1")
}

func TestThemeIndex(t *testing.T) {
	for name, want := range map[string]int{
		"":                 0,
		"github-dark":      0,
		"Tokyo Night":      1,
		"catppuccin":       2,
		"catppuccin-mocha": 2,
		"DRACULA":          3,
		"github-light":     4,
		"unknown":          0,
	} {
		if got := ThemeIndex(name); got != want {
			t.Errorf("ThemeIndex(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestCycleThemeWraps(t *testing.T) {
	isolate(t)
	c := New(Names()[len(Themes)-1], "", true, false, false)
	if got := c.CycleTheme().Name; got != Themes[0].Name {
		t.Errorf("cycle from last = %s", got)
	}
}

func TestEveryThemeIsComplete(t *testing.T) {
	for _, th := range Themes {
		for name, v := range map[string]string{"BgPrimary": th.BgPrimary, "BgSurface": th.BgSurface, "Green": th.Green, "Red": th.Red, "Heatmap[4]": th.Heatmap[4]} {
			if len(v) != 7 || v[0] != '#' {
				t.Errorf("%s: %s = %q", th.Name, name, v)
			}
		}
	}
}

func TestSettingsPersist(t *testing.T) {
	isolate(t)
	c := New("", "", false, false, false)
	c.CycleTheme()
	c.CycleTheme()
	c.ASCII, c.GlyphsChosen = true, true
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GHX_NERD", "")
	again := New("", "", false, false, false)
	if again.ThemeIndex != 2 || !again.ASCII {
		t.Errorf("reloaded theme=%d ascii=%v, want 2/true", again.ThemeIndex, again.ASCII)
	}
	if again.Refresh != 5*time.Minute {
		t.Errorf("refresh = %v", again.Refresh)
	}
	// Flags beat saved settings, and --check never reads or writes them.
	if New("dracula", "", false, false, false).ThemeIndex != 3 {
		t.Error("--theme must override the saved theme")
	}
	if New("", "", false, false, true).ThemeIndex != 0 {
		t.Error("check mode must ignore saved settings")
	}
}

func TestVSCodeWithoutNerdFontFallsBack(t *testing.T) {
	isolate(t)
	t.Setenv("GHX_NERD", "")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "")
	c := New("", "", true, false, false)
	if !c.ASCII || c.FontHint == "" {
		t.Errorf("expected Unicode fallback with a hint, got ascii=%v hint=%q", c.ASCII, c.FontHint)
	}
}
