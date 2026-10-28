package ui

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/Tukesh1/ghx/internal/github"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// commentTarget is the issue or pull request the composer posts to.
type commentTarget struct {
	repo   string
	number int
	title  string
	isPR   bool
}

func newComposer(p palette) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Write a comment… Markdown is supported."
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 65536
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color(p.BgPrimary))
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(p.TextPrimary)).Background(lipgloss.Color(p.BgPrimary))
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(p.TextMuted)).Background(lipgloss.Color(p.BgPrimary))
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color(p.BgPrimary))
	ta.BlurredStyle = ta.FocusedStyle
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(p.BorderActive))
	return ta
}

// startComment opens the composer for the selected issue, or the selected PR on PR views.
func (m Model) startComment() (tea.Model, tea.Cmd) {
	var t commentTarget
	switch m.activeKind() {
	case kindIssue:
		is, ok := m.selectedIssue()
		if !ok {
			return m, nil
		}
		t = commentTarget{repo: is.Repo, number: is.Number, title: is.Title}
	case kindPR:
		pr, ok := m.selectedPR()
		if !ok {
			return m, nil
		}
		t = commentTarget{repo: pr.Repo, number: pr.Number, title: pr.Title, isPR: true}
	default:
		m.setStatus(statusInfo, "ghx: select an issue or pull request to comment on")
		return m, nil
	}
	m.composer = newComposer(m.p)
	m.composeTo = &t
	return m, m.composer.Focus()
}

func (m Model) composerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if strings.TrimSpace(m.composer.Value()) != "" && m.pending != "esc:compose" {
			m.pending = "esc:compose"
			m.setStatus(statusErr, "discard this comment? press esc again")
			return m, nil
		}
		m.pending = ""
		m.composeTo = nil
		m.setStatus(statusInfo, "")
		return m, nil
	case "ctrl+s":
		body := strings.TrimSpace(m.composer.Value())
		if body == "" {
			m.setStatus(statusErr, "ghx: the comment is empty")
			return m, nil
		}
		t := *m.composeTo
		m.composeTo = nil
		m.pending = ""
		return m.mutate(fmt.Sprintf("Commented on #%d", t.number),
			func(ctx context.Context, c *gh.Client) error { return c.Comment(ctx, t.repo, t.number, body) },
			func(mm *Model) {
				if !t.isPR {
					mm.mutateIssue(gh.Issue{Repo: t.repo, Number: t.number}, func(i *gh.Issue) { i.Comments++ })
				}
			})
	}
	m.pending = ""
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

// composerModal renders the comment box sized for a w×h body.
func (m Model) composerModal(w, h int) []string {
	p := m.p
	t := m.composeTo
	mw := clamp(w-8, 50, 96)
	mh := clamp(h-4, 10, 18)
	inner := mw - 4
	m.composer.SetWidth(inner - 2)
	m.composer.SetHeight(max(mh-panelChrome-5, 3))

	kind, icon := "issue", p.fg(p.Green, p.ic.IssueOpen)
	if t.isPR {
		kind, icon = "pull request", p.fg(p.Green, p.ic.PROpen)
	}
	body := []string{
		icon + " " + p.bold(p.TextPrimary, fmt.Sprintf("#%d", t.number)) + " " + p.text(t.title),
		p.muted(t.repo + " · new comment on this " + kind),
		"",
	}
	for _, l := range strings.Split(m.composer.View(), "\n") {
		body = append(body, paint(" "+l, p.BgPrimary, inner))
	}
	footer := []string{
		p.line(strings.Repeat("─", inner)),
		spread(p.kbd("ctrl+s")+p.muted(" post comment")+"   "+p.kbd("esc")+p.muted(" cancel"), p.dim(fmt.Sprintf("%d chars", len(m.composer.Value()))), inner),
	}
	return strings.Split(p.panel(p.ic.Comment+" COMMENT", p.badge(p.Blue, shortRepo(t.repo, m.data.User.Login)), body, footer, mw, mh, true), "\n")
}
