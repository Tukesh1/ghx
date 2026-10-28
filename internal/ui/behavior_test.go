package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tukesh1/ghx/internal/config"
	gh "github.com/Tukesh1/ghx/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("GHX_NERD", "1")
	t.Setenv("GHX_CONFIG_DIR", t.TempDir())
	t.Setenv("GHX_CACHE_DIR", t.TempDir())
	return config.New("", "", false, false, true)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func sized(m Model) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return next.(Model)
}

func plain(m Model) string { return ansi.Strip(m.View()) }

// A live fetch that fails before any data loaded must explain the error, not pretend with demo data.
func TestFirstFetchFailureShowsErrorScreen(t *testing.T) {
	m := sized(New(testConfig(t), gh.NewTestClient("tok", "http://127.0.0.1:1"), ""))
	next, cmd := m.Update(dataMsg{err: errors.New("GitHub rejected the token (401)")})
	m = next.(Model)
	if m.data != nil {
		t.Fatal("failed first fetch must not substitute demo data")
	}
	if cmd != nil {
		t.Error("dataMsg must not start a second tick loop")
	}
	view := plain(m)
	for _, want := range []string{"Couldn't load your dashboard", "401", "retry now", "demo data"} {
		if !strings.Contains(view, want) {
			t.Errorf("error screen lacks %q", want)
		}
	}
	if time.Until(m.nextFetch) < 4*time.Second {
		t.Errorf("retry scheduled too soon: %v", time.Until(m.nextFetch))
	}
	// d opts into demo data explicitly.
	m = press(t, m, "d")
	if m.data == nil || !m.data.Demo {
		t.Fatal("d should load demo data")
	}
}

// A failed refresh keeps the data on screen and backs off instead of retrying on every tick.
func TestFailedRefreshBacksOff(t *testing.T) {
	m := sized(New(testConfig(t), gh.NewTestClient("tok", "http://127.0.0.1:1"), ""))
	live := gh.MockDashboard()
	live.Demo = false
	next, _ := m.Update(dataMsg{d: live})
	m = next.(Model)
	if m.loading || time.Until(m.nextFetch) < 4*time.Minute {
		t.Fatalf("next refresh should be ~5m away, got %v", time.Until(m.nextFetch))
	}
	m.loading = true
	var gaps []time.Duration
	for i := 0; i < 3; i++ {
		next, _ = m.Update(dataMsg{err: errors.New("HTTP 502")})
		m = next.(Model)
		gaps = append(gaps, time.Until(m.nextFetch).Round(time.Second))
		m.loading = true
	}
	if m.data != live {
		t.Error("data must survive a failed refresh")
	}
	if !(gaps[0] < gaps[1] && gaps[1] < gaps[2]) {
		t.Errorf("retries should back off, got %v", gaps)
	}
	if !strings.Contains(m.status, "refresh failed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestPartialDataIsFlagged(t *testing.T) {
	m := sized(New(testConfig(t), gh.NewTestClient("tok", "http://127.0.0.1:1"), ""))
	live := gh.MockDashboard()
	live.Demo, live.Warnings = false, []string{"issues: GitHub is having trouble (HTTP 502)"}
	next, _ := m.Update(dataMsg{d: live})
	m = next.(Model)
	if view := plain(m); !strings.Contains(view, "1 partial") || !strings.Contains(view, "partial data · issues") {
		t.Error("partial data should be visible in the header and status line")
	}
}

func TestTickIntervalFollowsActivity(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	if m.tickInterval() != time.Second {
		t.Errorf("idle tick = %v", m.tickInterval())
	}
	m.loading = true
	if m.tickInterval() >= time.Second {
		t.Error("spinner should tick fast while loading")
	}
}

func TestDemoMutations(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	// Approve the first review-queue PR from the PRs tab.
	m = press(t, m, "3", "a")
	if pr, _ := m.selectedPR(); pr.ReviewDecision != gh.ReviewApproved {
		t.Errorf("approve: decision = %q", pr.ReviewDecision)
	}
	// Merging needs a second press; another key cancels.
	before := len(m.data.ActionPRs())
	m = press(t, m, "j", "j", "j", "m", "k")
	if len(m.data.ActionPRs()) != before || m.pending != "" {
		t.Fatal("a different key must cancel a pending merge")
	}
	m = press(t, m, "j", "m", "m")
	if len(m.data.ActionPRs()) != before-1 {
		t.Errorf("m m should merge: %d → %d PRs", before, len(m.data.ActionPRs()))
	}
	// Close an issue.
	issues := len(m.data.Issues)
	m = press(t, m, "6", "x", "x")
	if len(m.data.Issues) != issues-1 {
		t.Errorf("x x should close an issue")
	}
}

func TestCommentComposer(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	m = press(t, m, "6", "c")
	if m.composeTo == nil {
		t.Fatal("c on an issue should open the composer")
	}
	is, _ := m.selectedIssue()
	if !strings.Contains(plain(m), "COMMENT") {
		t.Error("composer modal not rendered")
	}
	// Typing q must go into the comment, not quit.
	next, cmd := m.Update(key("q"))
	m = next.(Model)
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q inside the composer quit the app")
		}
	}
	m = press(t, m, "ctrl+s")
	if m.composeTo != nil {
		t.Fatal("ctrl+s should post and close the composer")
	}
	after, _ := m.selectedIssue()
	if after.Comments != is.Comments+1 {
		t.Errorf("comments %d → %d", is.Comments, after.Comments)
	}
}

func TestEmptyCommentIsRejected(t *testing.T) {
	m := press(t, sized(New(testConfig(t), nil, "")), "6", "c", "ctrl+s")
	if m.composeTo == nil {
		t.Error("an empty comment must not be posted")
	}
}

func TestInboxScrollsToCursor(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	for i := 0; i < 20; i++ {
		m.data.Notifications = append(m.data.Notifications, gh.Notification{ID: string(rune('a' + i)), Title: "extra notification", Repo: "o/r"})
	}
	m = press(t, m, "n", "G")
	last := m.data.Notifications[len(m.data.Notifications)-1]
	if m.notifCursor != len(m.data.Notifications)-1 {
		t.Fatalf("G should select the last notification, cursor=%d", m.notifCursor)
	}
	start, perPage := m.notifWindow(m.bodyHeight())
	if m.notifCursor < start || m.notifCursor >= start+perPage {
		t.Errorf("cursor %d outside window %d+%d", m.notifCursor, start, perPage)
	}
	if !strings.Contains(plain(m), last.Title) {
		t.Error("selected notification not visible")
	}
	// Opening the selected notification marks it read (removes it).
	n := len(m.data.Notifications)
	m = press(t, m, "enter")
	if len(m.data.Notifications) != n-1 {
		t.Error("enter should open and mark the notification read")
	}
}

func TestOverviewEnterKeepsSelection(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	m = press(t, m, "tab", "j", "j")
	run, _ := m.selectedRun()
	m = press(t, m, "enter")
	if m.tab != tabActions {
		t.Fatalf("enter on the runs panel should open Actions, got tab %d", m.tab)
	}
	if got, _ := m.selectedRun(); got.ID != run.ID {
		t.Errorf("selection lost: %d → %d", run.ID, got.ID)
	}
}

func TestDemoNeverShowsMockAnalyticsForLiveData(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	m.data.Demo = false
	r, _ := m.selectedRepo()
	if a, _ := m.analyticsFor(r); a != nil {
		t.Error("live repositories must never get synthesized analytics")
	}
}

func TestFitMidKeepsBothEnds(t *testing.T) {
	got := ansi.Strip(fitMid("software-data-engineering-eded247b-6f3352526288", 20))
	if ansi.StringWidth(got) != 20 || !strings.HasPrefix(got, "software") || !strings.HasSuffix(got, "526288") {
		t.Errorf("fitMid = %q", got)
	}
}

func TestOptionalSegmentsDropFirst(t *testing.T) {
	cols := []column{{title: "Repo", width: 12}}
	rows := [][]string{{optional("acme-corp/") + "engine"}}
	out := m0().p.table(cols, rows, -1, 20, 5, false)
	if line := ansi.Strip(out[1]); !strings.Contains(line, "engine") || strings.Contains(line, "acme") {
		t.Errorf("narrow cell should drop the optional owner: %q", line)
	}
	wide := m0().p.table([]column{{title: "Repo", width: 20}}, rows, -1, 30, 5, false)
	if line := ansi.Strip(wide[1]); !strings.Contains(line, "acme-corp/engine") {
		t.Errorf("wide cell should keep the owner: %q", line)
	}
}

func TestPlural(t *testing.T) {
	for in, want := range map[int]string{0: "0 repos", 1: "1 repo", 2: "2 repos"} {
		if got := plural(in, "repo"); got != want {
			t.Errorf("plural(%d) = %q", in, got)
		}
	}
	if got := plural(3, "entry"); got != "3 entries" {
		t.Errorf("plural(entry) = %q", got)
	}
}

func m0() Model {
	return New(config.New("", "", true, false, true), nil, "")
}

func TestProfileTab(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	if m.tab != tabOverview {
		t.Fatalf("ghx should start on the Command Center, got tab %d", m.tab)
	}
	m = press(t, m, "1")
	view := plain(m)
	for _, want := range []string{"Mona Lisa Octocat", "README.md", "Hi, I'm Mona", "PINNED", "orbit-engine", "COMMITS BY REPOSITORY", "Joined Jan 2011"} {
		if !strings.Contains(view, want) {
			t.Errorf("profile lacks %q", want)
		}
	}
	m = press(t, m, "j", "enter")
	if !strings.Contains(m.status, "would open https://github.com/octocat/ghx") {
		t.Errorf("enter should open the second pinned repository, status %q", m.status)
	}
	m = press(t, m, "O")
	if !strings.Contains(m.status, "would open https://github.com/octocat") {
		t.Errorf("O should open the profile, status %q", m.status)
	}
	// tab cycles the dedicated tabs and the profile, skipping the Command Center (where tab moves panels).
	m = press(t, m, "tab")
	if m.tab != tabPRs {
		t.Errorf("tab from Profile → %d, want PRs", m.tab)
	}
	m = press(t, m, "tab", "tab", "tab", "tab")
	if m.tab != tabProfile {
		t.Errorf("tab should wrap from Issues to Profile, got %d", m.tab)
	}
}

func TestProfileClickSelectsPinnedCard(t *testing.T) {
	m := press(t, sized(New(testConfig(t), nil, "")), "1")
	g := m.profileLayout(m.width-2*pageMargin, m.bodyHeight())
	if g.cols < 2 {
		t.Fatalf("expected a card grid at 140x40, got %d columns", g.cols)
	}
	x := pageMargin + g.sw + 1 + 2 + pinnedCardW(g.rw, g.cols) + 1 + 3 // inside the second card
	y := m.bodyTop() + g.pinnedY + 2
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.cursor[kindPinned] != 1 {
		t.Errorf("click selected pinned %d, want 1", m.cursor[kindPinned])
	}
}

func TestProfileUnavailable(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	m.data.Profile = nil
	m = press(t, m, "1", "j", "enter", "c", "y")
	if !strings.Contains(plain(m), "Your profile couldn't be loaded") {
		t.Error("a missing profile should say so")
	}
}

// Messages and help derive tab numbers from the tab list instead of hard-coding them.
func TestTabNumbersAreNotHardCoded(t *testing.T) {
	m := sized(New(testConfig(t), nil, ""))
	m = press(t, m, tabKey(tabIssues), "a")
	if want := "press " + tabKey(tabPRs); !strings.Contains(m.status, want) {
		t.Errorf("approve hint %q should say %q", m.status, want)
	}
	if got := helpKeys[0][1]; got != "1–"+tabKey(len(tabNames)-1) {
		t.Errorf("help lists views as %q", got)
	}
	if m = press(t, m, "9"); m.tab != tabIssues {
		t.Errorf("a key past the last tab must be ignored, got tab %d", m.tab)
	}
}
