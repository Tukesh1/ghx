package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// editorSettingsDirs are the per-user settings folders of VS Code-based editors, whose integrated
// terminals all report TERM_PROGRAM=vscode.
var editorSettingsDirs = []string{"Code", "Code - Insiders", "Cursor", "Windsurf", "VSCodium"}

var terminalFontRe = regexp.MustCompile(`"terminal\.integrated\.fontFamily"\s*:\s*"([^"]*)"`)

// NerdFontHint reports whether the current terminal is likely rendering a Nerd Font. Only the
// VS Code family is checked (their terminals default to a font without Octicon glyphs); other
// terminals are assumed to be configured by the user. The returned hint explains how to fix it.
func NerdFontHint() (ok bool, hint string) {
	if os.Getenv("TERM_PROGRAM") != "vscode" {
		return true, ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true, ""
	}
	roots := []string{
		filepath.Join(home, "Library", "Application Support"),
		filepath.Join(home, ".config"),
		os.Getenv("APPDATA"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, dir := range editorSettingsDirs {
			b, err := os.ReadFile(filepath.Join(root, dir, "User", "settings.json"))
			if err != nil {
				continue
			}
			if mt := terminalFontRe.FindSubmatch(b); mt != nil {
				font := strings.ToLower(string(mt[1]))
				if strings.Contains(font, "nerd") || strings.Contains(font, " nf") {
					return true, ""
				}
			}
		}
	}
	return false, `GitHub Octicons need a Nerd Font: set "terminal.integrated.fontFamily" to your Nerd Font (or GHX_NERD=1)`
}
