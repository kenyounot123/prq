package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

func renderedLines(t *testing.T, model Model) []string {
	t.Helper()
	return strings.Split(ansi.Strip(model.View().Content), "\n")
}

func assertFitsTerminal(t *testing.T, model Model, label string) {
	t.Helper()
	lines := renderedLines(t, model)
	if len(lines) != model.height {
		t.Errorf("%s: render height = %d rows, want %d", label, len(lines), model.height)
	}
	for index, line := range lines {
		if got := ansi.StringWidth(line); got != model.width {
			t.Errorf("%s: line %d width = %d, want %d: %q", label, index+1, got, model.width, line)
		}
	}
}

func assertGolden(t *testing.T, name, raw string) {
	t.Helper()
	normalized := normalizeRender(raw)
	goldenPath := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if normalized != string(want) {
		t.Fatalf("render differs from %s\n--- got ---\n%s\n--- want ---\n%s", name, normalized, want)
	}
}

func TestRenderWideAndNarrowLayouts(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		goldenFile string
		stacked    bool
	}{
		{name: "wide", width: 120, goldenFile: "wide.golden"},
		{name: "narrow", width: 80, goldenFile: "narrow.golden", stacked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := renderTestModel()
			model.checked["PR_web_77"] = true
			model.checked["PR_api_184"] = true
			model = selectPullRequest(t, model, "PR_api_190")
			model = updateModel(t, model, tea.WindowSizeMsg{Width: tt.width, Height: 32})

			view := model.View()
			if !view.AltScreen {
				t.Fatal("View did not enable alternate screen")
			}
			if view.WindowTitle != "prq" {
				t.Fatalf("window title = %q, want prq", view.WindowTitle)
			}
			assertFitsTerminal(t, model, tt.name)

			stripped := ansi.Strip(view.Content)
			for _, required := range []string{
				"● 1 ready", "○ 2 review", "✗ 2 blocked", "◇ 1 draft",
				"2 selected", "synced 8m ago",
				"✗ BLOCKED", "● READY TO MERGE", "○ WAITING ON REVIEW", "▸ ◇ DRAFT",
				"api#201", "web#77", "Cache session lookups",
				"acme/api #190", "DESCRIPTION", "Adds a short lived cache.",
				"↑↓ move", "tab next group", "space select", "c copy 2", "? more",
			} {
				if !strings.Contains(stripped, required) {
					t.Errorf("render missing %q", required)
				}
			}
			if strings.Contains(stripped, "Add invoice filters") {
				t.Error("collapsed draft group lists its pull requests")
			}

			listLine := lineContaining(stripped, "BLOCKED")
			detailLine := lineContaining(stripped, "acme/api #190")
			if tt.stacked && detailLine <= listLine {
				t.Fatalf("narrow detail line %d is not below list line %d", detailLine, listLine)
			}
			if !tt.stacked && detailLine != listLine {
				t.Fatalf("wide detail line %d is not beside list line %d", detailLine, listLine)
			}
			assertGolden(t, tt.goldenFile, view.Content)
		})
	}
}

func TestRenderFitsTerminalAtEverySize(t *testing.T) {
	for _, width := range []int{12, 30, 60, 80, 99, 100, 120, 180} {
		for _, height := range []int{6, 12, 20, 40} {
			for _, help := range []bool{false, true} {
				for _, checked := range []int{0, 1, 4} {
					model := renderTestModel()
					model.showHelp = help
					model.status = statusLine{text: "Copied 3 PRs.", kind: statusSuccess}
					for _, pullRequest := range model.pullRequests[:checked] {
						model.checked[pullRequest.ID] = true
					}
					model.resize(width, height)
					rendered := ansi.Strip(model.View().Content)
					for index, line := range strings.Split(rendered, "\n") {
						if got := ansi.StringWidth(line); got != width {
							t.Fatalf("%dx%d help=%v checked=%d: line %d width = %d: %q", width, height, help, checked, index+1, got, line)
						}
					}
					if got := strings.Count(rendered, "\n") + 1; got < height {
						t.Fatalf("%dx%d help=%v checked=%d: rendered %d rows, want at least %d", width, height, help, checked, got, height)
					}
				}
			}
		}
	}
}

func TestRenderBeforeFirstWindowSizeDoesNotPanic(t *testing.T) {
	_ = renderTestModel().View()
}

func TestRenderListRowShowsLabelTitleAndReasonColumn(t *testing.T) {
	model := renderTestModel()
	model.resize(120, 32)
	rendered := ansi.Strip(model.View().Content)

	rows := map[string]string{
		"api#201": "checks",
		"web#77":  "changes",
		"api#190": "◌ ci",
		"api#184": "10d",
		"web#85":  "1d",
	}
	for label, want := range rows {
		line := rendered[strings.Index(rendered, label):]
		line = line[:strings.Index(line, "\n")]
		listPart, _, _ := strings.Cut(line, "│")
		if !strings.HasSuffix(strings.TrimRight(listPart, " "), want) {
			t.Errorf("row %q = %q, want right column %q", label, listPart, want)
		}
	}
	if strings.Contains(rendered, "acme/api#201") {
		t.Error("list label includes the owner")
	}
	listPart, _, _ := strings.Cut(rendered[strings.Index(rendered, "api#201"):], "│")
	if !strings.Contains(listPart, "Rotate signing keys") || !strings.Contains(listPart, "…") {
		t.Errorf("long title row = %q, want a truncated title ending in an ellipsis", listPart)
	}
}

func TestRenderSyncStates(t *testing.T) {
	tests := []struct {
		name  string
		model func() Model
		want  string
	}{
		{"synced", renderTestModel, "synced 8m ago"},
		{"refreshing with data", func() Model {
			model := renderTestModel()
			model.refreshing = true
			return model
		}, "refreshing · synced 8m ago"},
		{"stale", func() Model {
			return updateModel(t, renderTestModel(), refreshDoneMsg{err: errors.New("offline")})
		}, "stale · synced 8m ago"},
		{"first sync", func() Model {
			return NewModel(context.Background(), app.Selection{}, nil, nil, testDependencies())
		}, "syncing…"},
		{"first sync failed", func() Model {
			model := NewModel(context.Background(), app.Selection{}, nil, nil, testDependencies())
			return updateModel(t, model, refreshDoneMsg{err: errors.New("offline")})
		}, "never synced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := updateModel(t, tt.model(), tea.WindowSizeMsg{Width: 120, Height: 32})
			header := renderedLines(t, model)[0]
			if !strings.HasSuffix(strings.TrimRight(header, " "), tt.want) {
				t.Fatalf("header = %q, want it to end with %q", header, tt.want)
			}
		})
	}
}

func TestRenderRefreshAndHelpStates(t *testing.T) {
	helpBindings := []string{
		"↑↓ move", "tab next group", "enter expand", "space select", "a select all",
		"esc clear", "c copy url", "C copy all as Slack", "o open", "r refresh", "q quit",
	}
	tests := []struct {
		name       string
		goldenFile string
		width      int
		model      func() Model
		required   []string
	}{
		{
			name:       "stale cached data",
			goldenFile: "stale.golden",
			width:      120,
			model: func() Model {
				return updateModel(t, renderTestModel(), refreshDoneMsg{err: errors.New("network unavailable")})
			},
			required: []string{"stale · synced 8m ago", "api#184", "Refresh failed: network unavailable"},
		},
		{
			name:       "first refresh failure",
			goldenFile: "refresh-error.golden",
			width:      120,
			model: func() Model {
				model := NewModel(
					context.Background(),
					app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
					nil,
					nil,
					testDependencies(),
				)
				return updateModel(t, model, refreshDoneMsg{err: errors.New("network unavailable")})
			},
			required: []string{"never synced", "Refresh failed: network unavailable", "Press r to retry or q to quit."},
		},
		{
			name:       "loading",
			goldenFile: "loading.golden",
			width:      120,
			model: func() Model {
				return NewModel(
					context.Background(),
					app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
					nil,
					nil,
					testDependencies(),
				)
			},
			required: []string{"syncing…", "Loading pull requests…"},
		},
		{
			name:       "empty",
			goldenFile: "empty.golden",
			width:      120,
			model: func() Model {
				model := NewModel(
					context.Background(),
					app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
					snapshotPointer(testSnapshot()),
					nil,
					testDependencies(),
				)
				return updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{Snapshot: testSnapshot()}})
			},
			required: []string{"No open pull requests."},
		},
		{
			name:       "expanded help",
			goldenFile: "help.golden",
			width:      120,
			model: func() Model {
				return updateModel(t, renderTestModel(), tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
			},
			required: append([]string{"? less"}, helpBindings...),
		},
		{
			name:       "expanded help narrow",
			goldenFile: "help-narrow.golden",
			width:      60,
			model: func() Model {
				return updateModel(t, renderTestModel(), tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
			},
			required: append([]string{"? less"}, helpBindings...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := updateModel(t, tt.model(), tea.WindowSizeMsg{Width: tt.width, Height: 32})
			raw := model.View().Content
			stripped := ansi.Strip(raw)
			for _, required := range tt.required {
				if !strings.Contains(stripped, required) {
					t.Errorf("render missing %q", required)
				}
			}
			assertFitsTerminal(t, model, tt.name)
			assertGolden(t, tt.goldenFile, raw)
		})
	}
}

func TestRenderSelectionTrayShowsCountPullRequestsAndHints(t *testing.T) {
	model := renderTestModel()
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 32})
	if strings.Contains(ansi.Strip(model.View().Content), "esc clear") {
		t.Fatal("tray is visible with nothing selected")
	}

	model.checked["PR_api_184"] = true
	model.checked["PR_api_201"] = true
	lines := renderedLines(t, model)
	tray := lines[len(lines)-2]
	want := "2 selected  api#201 api#184"
	if !strings.Contains(tray, want) {
		t.Fatalf("tray = %q, want checked pull requests in group order after %q", tray, want)
	}
	if !strings.HasSuffix(strings.TrimRight(tray, " "), "c copy  esc clear") {
		t.Fatalf("tray = %q, want hints right-aligned", tray)
	}
	assertFitsTerminal(t, model, "tray")
}

func TestRenderSelectionTrayTruncatesPullRequestsAtNarrowWidth(t *testing.T) {
	var pullRequests []app.PullRequest
	for number := 100; number < 120; number++ {
		pullRequests = append(pullRequests, testPullRequest(fmt.Sprintf("PR_%d", number), "acme/api", number))
	}
	model := groupedTestModel(pullRequests...)
	model.selectAll()
	for _, width := range []int{40, 60, 80} {
		model = updateModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
		lines := renderedLines(t, model)
		tray := lines[len(lines)-2]
		if !strings.Contains(tray, "20 selected") || !strings.Contains(tray, "…") {
			t.Errorf("width %d: tray = %q, want the count and a truncated pull request list", width, tray)
		}
		if !strings.Contains(tray, "esc clear") {
			t.Errorf("width %d: tray = %q lost its hints", width, tray)
		}
		assertFitsTerminal(t, model, fmt.Sprintf("tray width %d", width))
	}
}

func TestRenderTrayShrinksBodyByOneRow(t *testing.T) {
	model := updateModel(t, renderTestModel(), tea.WindowSizeMsg{Width: 120, Height: 32})
	without := model.bodyHeight()
	model.checked["PR_api_184"] = true
	if with := model.bodyHeight(); with != without-1 {
		t.Fatalf("body height with tray = %d, want %d", with, without-1)
	}
}

func TestRenderDetailPaneShowsReviewChecksBranchAgeAndWrappedDescription(t *testing.T) {
	model := renderTestModel()
	model = selectPullRequest(t, model, "PR_api_184")
	lines := model.renderDetailPane(40, 24)
	stripped := ansi.Strip(strings.Join(lines, "\n"))

	for _, want := range []string{
		"acme/api #184",
		"Fix session refresh",
		"review    ✓ approved",
		"checks    ✓ passing",
		"branch    ken/session-refresh",
		"age       opened 10d · updated 1d",
		strings.Repeat("─", 40),
		"DESCRIPTION",
		"Refreshes expired sessions without",
		"forcing a new login, dropping cookies,",
		"## Notes",
		"Preserves Markdown source.",
	} {
		if !strings.Contains(stripped, want) {
			t.Errorf("detail missing %q:\n%s", want, stripped)
		}
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 40 {
			t.Errorf("detail line width = %d: %q", ansi.StringWidth(line), ansi.Strip(line))
		}
	}
}

func TestRenderDetailPaneWrapsLongTitlesAndStatesWithoutBranchOrBody(t *testing.T) {
	model := renderTestModel()
	model = selectPullRequest(t, model, "PR_web_85")
	stripped := ansi.Strip(strings.Join(model.renderDetailPane(30, 20), "\n"))
	if strings.Contains(stripped, "branch") {
		t.Errorf("detail shows a branch row for an empty branch:\n%s", stripped)
	}
	if !strings.Contains(stripped, "No description") {
		t.Errorf("detail missing empty description text:\n%s", stripped)
	}

	model = selectPullRequest(t, model, "PR_api_201")
	stripped = normalizeRender(strings.Join(model.renderDetailPane(30, 20), "\n"))
	if !strings.Contains(stripped, "Rotate signing keys without\n") || !strings.Contains(stripped, "✗ failing") {
		t.Errorf("detail did not wrap the title or show failing checks:\n%s", stripped)
	}
}

func TestRenderDetailPaneClipsDescriptionToHeight(t *testing.T) {
	model := renderTestModel()
	model = selectPullRequest(t, model, "PR_api_201")
	lines := model.renderDetailPane(40, 14)
	if len(lines) != 14 {
		t.Fatalf("detail rows = %d, want 14", len(lines))
	}
	if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "new keys are published first") {
		t.Fatal("description is not clipped to the pane height")
	}
}

func TestRenderDetailPaneFillsTheBodyHeight(t *testing.T) {
	model := updateModel(t, renderTestModel(), tea.WindowSizeMsg{Width: 120, Height: 60})
	layout := layoutBody(model.contentWidth(), model.bodyHeight())
	if layout.detailHeight != layout.listHeight || layout.detailHeight < 50 {
		t.Fatalf("detail height = %d, list height = %d, want both to fill the body", layout.detailHeight, layout.listHeight)
	}
}

func TestRenderListScrollsToKeepCursorVisible(t *testing.T) {
	var pullRequests []app.PullRequest
	for number := 1; number <= 30; number++ {
		pullRequests = append(pullRequests, testPullRequest(fmt.Sprintf("PR_%02d", number), "acme/api", number))
	}
	model := groupedTestModel(pullRequests...)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})

	for range 20 {
		model = press(t, model, keyDown)
	}
	rendered := ansi.Strip(model.View().Content)
	if !strings.Contains(rendered, "api#21 ") {
		t.Fatalf("cursor row api#21 is not visible:\n%s", rendered)
	}
	if strings.Contains(rendered, "api#1 ") {
		t.Fatalf("list did not scroll:\n%s", rendered)
	}

	for range 19 {
		model = press(t, model, keyUp)
	}
	model = press(t, model, keyUp)
	rendered = ansi.Strip(model.View().Content)
	if !strings.Contains(rendered, "WAITING ON REVIEW") || !strings.Contains(rendered, "api#1 ") {
		t.Fatalf("scrolling back up lost the group header or cursor row:\n%s", rendered)
	}
}

func TestRenderCursorRowHasPurpleBackground(t *testing.T) {
	model := updateModel(t, renderTestModel(), tea.WindowSizeMsg{Width: 120, Height: 32})
	raw := model.View().Content
	if !strings.Contains(raw, "48;2;164;139;255") {
		t.Fatal("cursor row has no #A48BFF background")
	}
}

func TestRenderSanitizesUntrustedPullRequestTextWithoutMutatingDomainData(t *testing.T) {
	pullRequest := app.PullRequest{
		ID:         "PR_hostile",
		Repository: "acme/\x1b[31mapi\x1b[0m",
		Number:     9,
		Title:      "Fix \x1b]8;;https://evil.example\x1b\\linked\x1b]8;;\x1b\\ title\rback\bspace\u0085c1",
		Body:       "First \x1b]52;c;c2VjcmV0\x07line\r\nSecond\b line\u0085end",
		Branch:     "feature/\x1b[2Jbranch",
		URL:        "https://github.com/acme/api/pull/9\x1b[2J",
		CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Review:     app.ReviewState("approved\x1b[2J\rnow"),
		Checks:     app.CheckState("passing\x1b]52;c;c3RhdHVz\x07\bnow"),
	}
	original := pullRequest
	model := groupedTestModel(pullRequest)
	model.checked[pullRequest.ID] = true
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 180, Height: 32})

	raw := model.View().Content
	stripped := ansi.Strip(raw)
	for _, want := range []string{
		"api#9",
		"acme/api #9",
		"Fix linked title�back�space�c1",
		"approved�now",
		"passing�now",
		"feature/branch",
		"First line",
		"Second� line�end",
	} {
		if !strings.Contains(stripped, want) {
			t.Errorf("sanitized render missing %q:\n%s", want, stripped)
		}
	}
	for _, forbidden := range []string{"https://evil.example", "c2VjcmV0", "c3RhdHVz", "\x1b[31m", "\x1b[2J", "\r", "\b", "\u0085"} {
		if strings.Contains(stripped, forbidden) {
			t.Errorf("sanitized render contains %q:\n%s", forbidden, stripped)
		}
	}
	for _, forbidden := range []string{"\x1b[2J", "\x1b]8;", "\x1b]52;"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("raw render contains untrusted terminal sequence %q", forbidden)
		}
	}
	if got := model.pullRequests[0]; got != original {
		t.Fatalf("render mutated domain pull request = %#v, want %#v", got, original)
	}
}

func TestRenderSanitizesUntrustedErrorsAndStatusText(t *testing.T) {
	hostile := errors.New("network \x1b[2Jfailed\x1b]52;c;c2VjcmV0\x07\rnow\b\u0085")

	t.Run("first refresh error", func(t *testing.T) {
		model := NewModel(
			context.Background(),
			app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
			nil,
			nil,
			testDependencies(),
		)
		model = updateModel(t, model, refreshDoneMsg{err: hostile})
		model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 32})

		stripped := ansi.Strip(model.View().Content)
		if !strings.Contains(stripped, "Refresh failed: network failed�now��") {
			t.Fatalf("sanitized refresh error missing from render:\n%s", stripped)
		}
		if strings.Contains(stripped, "c2VjcmV0") || strings.Contains(stripped, "\r") || strings.Contains(stripped, "\b") {
			t.Fatalf("refresh error retained unsafe content:\n%s", stripped)
		}
	})

	t.Run("action status", func(t *testing.T) {
		model := renderTestModel()
		model = updateModel(t, model, actionDoneMsg{
			generation: model.generation,
			action:     "Copy",
			err:        hostile,
		})
		model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 32})

		stripped := ansi.Strip(model.View().Content)
		if !strings.Contains(stripped, "Copy failed: network failed�now��") {
			t.Fatalf("sanitized action status missing from render:\n%s", stripped)
		}
		if strings.Contains(stripped, "c2VjcmV0") || strings.Contains(stripped, "\r") || strings.Contains(stripped, "\b") {
			t.Fatalf("action status retained unsafe content:\n%s", stripped)
		}
	})
}

func TestRenderFooterKeepsStatusVisibleAtNarrowWidth(t *testing.T) {
	model := renderTestModel()
	model.status = statusLine{text: "Copied pull request URL.", kind: statusSuccess}
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 24})
	lines := renderedLines(t, model)
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "✓ Copied pull request URL.") || !strings.Contains(footer, "? more") {
		t.Fatalf("footer = %q, want the status and the help key both kept", footer)
	}
}

func TestCompactAge(t *testing.T) {
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		then time.Time
		want string
	}{
		"seconds":      {now.Add(-30 * time.Second), "now"},
		"minutes":      {now.Add(-11 * time.Minute), "11m"},
		"hours":        {now.Add(-2 * time.Hour), "2h"},
		"days":         {now.Add(-49 * time.Hour), "2d"},
		"future":       {now.Add(time.Hour), "now"},
		"zero instant": {time.Time{}, "now"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := compactAge(now, tt.then); got != tt.want {
				t.Fatalf("compactAge() = %q, want %q", got, tt.want)
			}
		})
	}
}
