package ui

import (
	"fmt"
	"strings"
	"time"

	gh "github.com/Tukesh1/ghx/internal/github"
)

var runCols = []column{
	{title: "Run ID", width: 12, drop: 4},
	{title: "Workflow", width: -2, min: 14},
	{title: "Repository", width: -2, drop: 3},
	{title: "Trigger", width: 14, drop: 5},
	{title: "Branch / SHA", width: -2, drop: 2},
	{title: "Status", width: 8},
	{title: "Duration", width: 8, right: true, drop: 1},
	{title: "Started", width: 8, right: true, drop: 6},
}

func (m Model) runRows(runs []gh.WorkflowRun) [][]string {
	p := m.p
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, []string{
			p.bold(p.Cyan, fmt.Sprintf("#%d", r.ID)),
			p.bold(p.TextPrimary, r.Name),
			p.fg(p.Blue, shortRepo(r.Repo, m.data.User.Login)),
			p.badge(p.TextSecondary, r.Event),
			cell(p.muted(r.Branch), p.dim(p.ic.Commit+" "+r.SHA)),
			p.runStateBadge(r),
			p.text(dur(runDuration(r))),
			p.muted(ago(r.StartedAt) + " ago"),
		})
	}
	return rows
}

// viewActions renders Tab [3]: workflow runs + job matrix & ANSI step log inspector.
func (m Model) viewActions(w, h int) string {
	p := m.p
	return m.splitTab(w, h,
		func(lw, h int) string {
			chips := m.filterChips([]string{"All", "Failing", "Running", "Passed"}, []string{p.Blue, p.Red, p.Yellow, p.Green}, m.pill[kindRun])
			title := titled(p.ic.Play+" WORKFLOW RUNS", p.ic.Play+" GITHUB ACTIONS WORKFLOW RUNS", chips, lw)
			return m.listPane(kindRun, title, chips, runCols, m.runRows(m.runs()), lw, h)
		},
		m.runInspector)
}

func jobState(status, conclusion string) string {
	return gh.WorkflowRun{Status: status, Conclusion: conclusion}.State()
}

func (m Model) stepIcon(s gh.Step) string {
	p := m.p
	switch {
	case s.Status != "completed":
		return p.fg(p.Yellow, p.ic.Running)
	case s.Conclusion == "skipped":
		return p.dim(p.ic.Skip)
	case s.Conclusion == "success":
		return p.fg(p.Green, p.ic.CheckPass)
	default:
		return p.fg(p.Red, p.ic.CheckFail)
	}
}

// colorLog applies ANSI coloring to a CI log line by content.
func (m Model) colorLog(l string) string {
	p := m.p
	low := strings.ToLower(l)
	switch {
	case strings.Contains(low, "panic") || strings.Contains(l, "FAIL") || strings.Contains(low, "error") || strings.Contains(low, "exit code 1"):
		return p.bold(p.Red, l)
	case strings.Contains(l, "✓") || strings.Contains(l, "PASS") || strings.HasPrefix(low, "ok "):
		return p.fg(p.Green, l)
	case strings.HasPrefix(l, "===") || strings.HasPrefix(l, "goroutine") || strings.HasPrefix(l, " "):
		return p.muted(l)
	case strings.HasPrefix(l, "##["):
		return p.fg(p.Yellow, l)
	default:
		return p.text(l)
	}
}

func (m Model) runInspector(w, h int) string {
	p := m.p
	inner := w - 4
	title := titled("JOB MATRIX & STEP LOGS", "JOB MATRIX & ANSI STEP LOGS", "Run #0000000000", w)
	r, ok := m.selectedRun()
	if !ok {
		return p.panel(title, "", []string{p.muted("no workflow run selected")}, nil, w, h, false)
	}
	b := []string{
		p.bold(p.TextPrimary, r.Name) + p.dim(" · ") + p.fg(p.Blue, r.Repo) + "  " + p.runStateBadge(r),
		p.muted("Branch: ") + p.text(r.Branch) + p.muted(" · SHA: ") + p.fg(p.Yellow, r.SHA),
		p.muted("Trigger: ") + p.text(r.Event) + p.muted(" · Duration: ") + p.text(dur(runDuration(r))) + p.muted(" · started "+ago(r.StartedAt)+" ago"),
		"",
	}

	var matrix []string
	var logStep *gh.Step
	for ji := range r.Jobs {
		j := r.Jobs[ji]
		var total time.Duration
		for _, s := range j.Steps {
			total += s.Duration
		}
		matrix = append(matrix, spread(p.stateIcon(jobState(j.Status, j.Conclusion))+" "+p.bold(p.TextPrimary, j.Name), p.muted(dur(total)), inner-2))
		for si := range j.Steps {
			s := j.Steps[si]
			name := p.text(s.Name)
			switch {
			case s.Conclusion == "skipped":
				name = p.dim(s.Name)
			case s.Status == "completed" && s.Conclusion != "success":
				name = p.bold(p.Red, s.Name)
			}
			left := "  " + m.stepIcon(s) + " " + p.dim(fmt.Sprintf("%2d ", s.Number)) + name
			matrix = append(matrix, spread(left, p.muted(dur(s.Duration)), inner-2))
			if len(s.Log) > 0 && (logStep == nil || s.Conclusion == "failure") {
				logStep = &r.Jobs[ji].Steps[si]
			}
		}
	}
	if len(matrix) == 0 {
		matrix = []string{p.muted("job details unavailable")}
	}
	b = append(b, p.box(fmt.Sprintf("Job Matrix (%d jobs)", len(r.Jobs)), matrix, inner)...)

	logLines := r.FailureLog
	logTitle := "Log"
	if logStep != nil {
		logLines = logStep.Log
		logTitle = "Log · " + logStep.Name
	}
	if len(logLines) > 0 {
		var ll []string
		for i, l := range logLines {
			ll = append(ll, p.dim(fmt.Sprintf("%3d ", i+1))+m.colorLog(l))
		}
		b = append(b, "")
		b = append(b, p.box(logTitle, ll, inner)...)
	}

	rerun := [3]string{"r", p.ic.Running + " Re-run Workflow", p.Green}
	if r.State() == gh.StateFailure {
		rerun[1] = p.ic.Running + " Re-run Failed Jobs"
	}
	footer := append([]string{p.line(strings.Repeat("─", inner))}, m.buttons(inner,
		rerun,
		[3]string{"w w", p.ic.Play + " Run workflow_dispatch", p.Blue},
		[3]string{"o", "Open", p.TextSecondary},
	)...)
	return p.panel(title, p.badge(p.Cyan, fmt.Sprintf("Run #%d", r.ID)), b, footer, w, h, false)
}
