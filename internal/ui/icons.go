package ui

// Icons holds every glyph ghx renders, as Nerd Font Octicons or plain Unicode fallbacks.
type Icons struct {
	PROpen, PRDraft, PRMerged     string
	RepoPublic, RepoPrivate       string
	CheckPass, CheckFail, Running string
	IssueOpen, IssueClosed        string
	Branch, Commit                string
	Star, Fork, Eye, Bell, Clock  string
	Flame, Shield, Play, Comment  string
	Milestone, Tag, Person, Dot   string
	Skip, Github, Search, Inbox   string
	Ahead, Behind, Arrow, Pending string
	Pulse, File, Zap, Code        string
	Org, Location, Link, Calendar string
	Pin                           string
	Spinner                       []string
}

var nerdIcons = Icons{
	PROpen:      "",
	PRDraft:     "",
	PRMerged:    "",
	RepoPublic:  "",
	RepoPrivate: "",
	CheckPass:   "",
	CheckFail:   "",
	Running:     "",
	IssueOpen:   "",
	IssueClosed: "",
	Branch:      "",
	Commit:      "",
	Star:        "",
	Fork:        "",
	Eye:         "",
	Bell:        "",
	Clock:       "",
	Flame:       "",
	Shield:      "",
	Play:        "",
	Comment:     "",
	Milestone:   "",
	Tag:         "",
	Person:      "",
	Dot:         "●",
	Skip:        "⊘",
	Github:      "",
	Search:      "",
	Inbox:       "",
	Ahead:       "↑",
	Behind:      "↓",
	Arrow:       "→",
	Pending:     "◌",
	Pulse:       "\uf469",
	File:        "\uf40d",
	Zap:         "\uf0e7",
	Code:        "\uf44f",
	Org:         "\uf42b",
	Location:    "\uf450",
	Link:        "\uf44c",
	Calendar:    "\uf455",
	Pin:         "\uf435",
	Spinner:     []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

// unicodeIcons is the fallback set. Every glyph is verified to exist in Menlo (the default VS Code /
// Cursor terminal font on macOS) and in common monospace fonts, so nothing renders as a blank box.
var unicodeIcons = Icons{
	PROpen:      "⇄",
	PRDraft:     "◌",
	PRMerged:    "◆",
	RepoPublic:  "▤",
	RepoPrivate: "▣",
	CheckPass:   "✔",
	CheckFail:   "✖",
	Running:     "↻",
	IssueOpen:   "⊙",
	IssueClosed: "✔",
	Branch:      "↱",
	Commit:      "⊚",
	Star:        "★",
	Fork:        "⇵",
	Eye:         "◉",
	Bell:        "◈",
	Clock:       "◷",
	Flame:       "▲",
	Shield:      "⚠",
	Play:        "▶",
	Comment:     "✎",
	Milestone:   "⚑",
	Tag:         "◇",
	Person:      "@",
	Dot:         "●",
	Skip:        "⊘",
	Github:      "◎",
	Search:      "⌕",
	Inbox:       "✉",
	Ahead:       "↑",
	Behind:      "↓",
	Arrow:       "→",
	Pending:     "◌",
	Pulse:       "▦",
	File:        "▭",
	Zap:         "ϟ",
	Code:        "⌘",
	Org:         "⌂",
	Location:    "⌖",
	Link:        "↗",
	Calendar:    "◫",
	Pin:         "✦",
	Spinner:     []string{"◐", "◓", "◑", "◒"},
}

// iconSet returns the Nerd Font set, or the Unicode fallback when ascii is true.
func iconSet(ascii bool) Icons {
	if ascii {
		return unicodeIcons
	}
	return nerdIcons
}
