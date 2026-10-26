package github

import (
	"reflect"
	"testing"
	"time"
)

func TestFailureExcerptEndsAtErrorAnnotation(t *testing.T) {
	raw := "2026-10-26T10:00:00.0000000Z ##[group]Run go test\n" +
		"2026-10-26T10:00:01.0000000Z === RUN TestX\n" +
		"2026-10-26T10:00:02.0000000Z --- FAIL: TestX (0.00s)\n" +
		"2026-10-26T10:00:03.0000000Z ##[error]Process completed with exit code 1.\n" +
		"2026-10-26T10:00:04.0000000Z Post job cleanup.\n"
	got := FailureExcerpt(raw, 2)
	want := []string{"--- FAIL: TestX (0.00s)", "Process completed with exit code 1."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPriorityFromLabels(t *testing.T) {
	for want, labels := range map[string][]Label{
		"P0": {{Name: "bug"}, {Name: "P0"}},
		"P1": {{Name: "priority: high"}},
		"P2": {{Name: "bug"}},
		"P3": {{Name: "docs"}},
	} {
		if got := PriorityFromLabels(labels); got != want {
			t.Errorf("%v → %s, want %s", labels, got, want)
		}
	}
}

func TestLevel(t *testing.T) {
	for count, want := range map[int]int{0: 0, 1: 1, 3: 1, 4: 2, 8: 3, 13: 4, 99: 4} {
		if got := Level(count); got != want {
			t.Errorf("Level(%d) = %d, want %d", count, got, want)
		}
	}
}

func TestStreakAndWeeks(t *testing.T) {
	d := MockDashboard()
	if got := d.Streak(); got != 47 {
		t.Errorf("mock streak = %d, want 47", got)
	}
	weeks, starts := d.WeeksWithStarts()
	if len(weeks) == 0 || len(weeks) > 53 || len(weeks) != len(starts) {
		t.Fatalf("weeks=%d starts=%d", len(weeks), len(starts))
	}
	for _, s := range starts[1:] {
		if s.Weekday() != time.Sunday {
			t.Fatalf("week start %v is not a Sunday", s)
		}
	}
}

func TestBuildCIHistory(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	runs := []WorkflowRun{
		{Status: "completed", Conclusion: "success", StartedAt: now.Add(-time.Hour), Duration: time.Minute},
		{Status: "completed", Conclusion: "failure", StartedAt: now.Add(-time.Hour), Duration: 3 * time.Minute},
		{Status: "in_progress", StartedAt: now},
		{Status: "completed", Conclusion: "success", StartedAt: now.AddDate(0, 0, -30)},
	}
	days := BuildCIHistory(runs, now)
	if len(days) != 14 {
		t.Fatalf("days = %d", len(days))
	}
	last := days[13]
	if last.Passed != 1 || last.Failed != 1 {
		t.Errorf("today = %+v", last)
	}
}

func TestMockAnalyticsIsDeterministic(t *testing.T) {
	r := MockDashboard().Repos[0]
	a, b := MockAnalytics(r), MockAnalytics(r)
	if !reflect.DeepEqual(a.CommitWeeks, b.CommitWeeks) || len(a.CommitWeeks) != 52 {
		t.Errorf("commit weeks not deterministic or wrong length (%d)", len(a.CommitWeeks))
	}
	if len(a.Contributors) == 0 || len(a.CodeFreq) != 26 || len(a.Views) != 14 {
		t.Errorf("incomplete mock analytics: %d contributors, %d code freq, %d views", len(a.Contributors), len(a.CodeFreq), len(a.Views))
	}
}
