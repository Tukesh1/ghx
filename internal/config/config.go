package config

import "os"

// Config is the runtime state shared by the UI: chosen theme, glyph mode and org filter.
type Config struct {
	ThemeIndex int
	ASCII      bool
	Org        string
	Demo       bool
	Check      bool
	// FontHint is set when Nerd Font glyphs were auto-disabled, explaining how to enable them.
	FontHint string
}

// New builds a Config from CLI flag values, honouring GHX_ASCII=1 and GHX_THEME.
func New(theme, org string, demo, ascii, check bool) *Config {
	if theme == "" {
		theme = os.Getenv("GHX_THEME")
	}
	if os.Getenv("GHX_ASCII") == "1" {
		ascii = true
	}
	hint := ""
	if !ascii && os.Getenv("GHX_NERD") != "1" {
		if ok, h := NerdFontHint(); !ok {
			ascii, hint = true, h
		}
	}
	return &Config{
		ThemeIndex: ThemeIndex(theme),
		ASCII:      ascii,
		Org:        org,
		Demo:       demo,
		Check:      check,
		FontHint:   hint,
	}
}

// Theme returns the active palette.
func (c *Config) Theme() Theme {
	if c.ThemeIndex < 0 || c.ThemeIndex >= len(Themes) {
		c.ThemeIndex = 0
	}
	return Themes[c.ThemeIndex]
}

// CycleTheme advances to the next palette and returns it.
func (c *Config) CycleTheme() Theme {
	c.ThemeIndex = (c.ThemeIndex + 1) % len(Themes)
	return Themes[c.ThemeIndex]
}

// FontMode names the active glyph set for the header.
func (c *Config) FontMode() string {
	if c.ASCII {
		return "unicode"
	}
	return "nerd"
}
