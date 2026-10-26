package config

import "strings"

// Theme is a 24-bit TrueColor palette used by every ghx view.
type Theme struct {
	Name          string
	BgPrimary     string
	BgSurface     string
	BgElevated    string
	BorderMuted   string
	BorderActive  string
	TextPrimary   string
	TextSecondary string
	TextMuted     string
	Green         string
	Blue          string
	Purple        string
	Yellow        string
	Red           string
	Cyan          string
	Pink          string
}

// Themes lists the palettes in the order the `t` key cycles through them.
var Themes = []Theme{
	{
		Name:          "GitHub Dark",
		BgPrimary:     "#0d1117",
		BgSurface:     "#161b22",
		BgElevated:    "#1c2128",
		BorderMuted:   "#30363d",
		BorderActive:  "#58a6ff",
		TextPrimary:   "#e6edf3",
		TextSecondary: "#8b949e",
		TextMuted:     "#484f58",
		Green:         "#3fb950",
		Blue:          "#58a6ff",
		Purple:        "#a371f7",
		Yellow:        "#d29922",
		Red:           "#f85149",
		Cyan:          "#39c5cf",
		Pink:          "#db61a2",
	},
	{
		Name:          "Tokyo Night",
		BgPrimary:     "#1a1b26",
		BgSurface:     "#24283b",
		BgElevated:    "#292e42",
		BorderMuted:   "#3b4261",
		BorderActive:  "#7aa2f7",
		TextPrimary:   "#c0caf5",
		TextSecondary: "#7982a9",
		TextMuted:     "#565f89",
		Green:         "#9ece6a",
		Blue:          "#7aa2f7",
		Purple:        "#bb9af7",
		Yellow:        "#e0af68",
		Red:           "#f7768e",
		Cyan:          "#7dcfff",
		Pink:          "#ff007c",
	},
	{
		Name:          "Catppuccin Mocha",
		BgPrimary:     "#1e1e2e",
		BgSurface:     "#181825",
		BgElevated:    "#313244",
		BorderMuted:   "#45475a",
		BorderActive:  "#89b4fa",
		TextPrimary:   "#cdd6f4",
		TextSecondary: "#a6adc8",
		TextMuted:     "#6c7086",
		Green:         "#a6e3a1",
		Blue:          "#89b4fa",
		Purple:        "#cba6f7",
		Yellow:        "#f9e2af",
		Red:           "#f38ba8",
		Cyan:          "#94e2d5",
		Pink:          "#f5c2e7",
	},
	{
		Name:          "Dracula",
		BgPrimary:     "#282a36",
		BgSurface:     "#1e1f29",
		BgElevated:    "#343746",
		BorderMuted:   "#44475a",
		BorderActive:  "#8be9fd",
		TextPrimary:   "#f8f8f2",
		TextSecondary: "#9580ff",
		TextMuted:     "#6272a4",
		Green:         "#50fa7b",
		Blue:          "#8be9fd",
		Purple:        "#bd93f9",
		Yellow:        "#f1fa8c",
		Red:           "#ff5555",
		Cyan:          "#8be9fd",
		Pink:          "#ff79c6",
	},
}

// HeatmapPalette holds the 5 GitHub contribution intensity levels (0 → 13+ commits).
var HeatmapPalette = [5]string{"#161b22", "#0e4429", "#006d32", "#26a641", "#39d353"}

// ThemeIndex resolves a theme by display name or slug (e.g. "tokyo-night"); unknown names map to 0.
func ThemeIndex(name string) int {
	key := slug(name)
	for i, t := range Themes {
		s := slug(t.Name)
		if s == key || strings.HasPrefix(s, key) && key != "" {
			return i
		}
	}
	return 0
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return strings.ReplaceAll(s, "_", "-")
}
