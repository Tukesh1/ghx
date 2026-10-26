package config

import "testing"

func TestThemeIndex(t *testing.T) {
	for name, want := range map[string]int{
		"":                 0,
		"github-dark":      0,
		"Tokyo Night":      1,
		"catppuccin":       2,
		"catppuccin-mocha": 2,
		"DRACULA":          3,
		"unknown":          0,
	} {
		if got := ThemeIndex(name); got != want {
			t.Errorf("ThemeIndex(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestCycleThemeWraps(t *testing.T) {
	t.Setenv("GHX_NERD", "1")
	c := New("dracula", "", true, false, false)
	if got := c.CycleTheme().Name; got != Themes[0].Name {
		t.Errorf("cycle from last = %s", got)
	}
}

func TestVSCodeWithoutNerdFontFallsBack(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "")
	t.Setenv("GHX_NERD", "")
	t.Setenv("GHX_ASCII", "")
	c := New("", "", true, false, false)
	if !c.ASCII || c.FontHint == "" {
		t.Errorf("expected Unicode fallback with a hint, got ascii=%v hint=%q", c.ASCII, c.FontHint)
	}
}
