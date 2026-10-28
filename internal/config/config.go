package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DefaultRefresh is how often the live dashboard re-fetches in the background.
const DefaultRefresh = 5 * time.Minute

// Config is the runtime state shared by the UI: chosen theme, glyph mode and org filter.
type Config struct {
	ThemeIndex int
	ASCII      bool
	Org        string
	Demo       bool
	Check      bool
	Refresh    time.Duration
	// GlyphsChosen is set once the user picks a glyph set explicitly, so it is remembered.
	GlyphsChosen bool
	// FontHint is set when Nerd Font glyphs were auto-disabled, explaining how to enable them.
	FontHint string
}

// settings is the small JSON file ghx persists between runs (theme and glyph choice).
type settings struct {
	Theme  string `json:"theme,omitempty"`
	Glyphs string `json:"glyphs,omitempty"` // "nerd" or "unicode"
}

// Dir is the ghx settings directory: $GHX_CONFIG_DIR, or ghx/ under the OS user config directory.
func Dir() (string, error) {
	if d := os.Getenv("GHX_CONFIG_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ghx"), nil
}

func loadSettings() settings {
	var s settings
	dir, err := Dir()
	if err != nil {
		return s
	}
	if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// New builds a Config from CLI flag values. Precedence: flags, then GHX_THEME / GHX_ASCII / GHX_NERD,
// then the saved settings, then defaults (with Nerd Font auto-detection for editor terminals).
func New(theme, org string, demo, ascii, check bool) *Config {
	saved := settings{}
	if !check {
		saved = loadSettings()
	}
	if theme == "" {
		theme = os.Getenv("GHX_THEME")
	}
	if theme == "" {
		theme = saved.Theme
	}
	chosen := false
	switch {
	case ascii || os.Getenv("GHX_ASCII") == "1":
		ascii = true
	case os.Getenv("GHX_NERD") == "1":
	case saved.Glyphs != "":
		ascii, chosen = saved.Glyphs == "unicode", true
	}
	hint := ""
	if !ascii && !chosen && os.Getenv("GHX_NERD") != "1" {
		if ok, h := NerdFontHint(); !ok {
			ascii, hint = true, h
		}
	}
	return &Config{
		ThemeIndex:   ThemeIndex(theme),
		ASCII:        ascii,
		Org:          org,
		Demo:         demo,
		Check:        check,
		Refresh:      DefaultRefresh,
		GlyphsChosen: chosen,
		FontHint:     hint,
	}
}

// Save persists the theme (and the glyph set, once chosen explicitly) for the next run.
func (c *Config) Save() error {
	if c.Check {
		return nil
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	s := settings{Theme: Slug(c.Theme().Name)}
	if c.GlyphsChosen {
		s.Glyphs = c.FontMode()
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), append(b, '\n'), 0o644)
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

// FontMode names the active glyph set.
func (c *Config) FontMode() string {
	if c.ASCII {
		return "unicode"
	}
	return "nerd"
}
