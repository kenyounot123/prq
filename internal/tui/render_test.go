package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

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
			model = updateModel(t, model, tea.WindowSizeMsg{Width: tt.width, Height: 32})

			view := model.View()
			if !view.AltScreen {
				t.Fatal("View did not enable alternate screen")
			}
			if view.WindowTitle != "prq" {
				t.Fatalf("window title = %q, want prq", view.WindowTitle)
			}

			raw := view.Content
			lines := strings.Split(strings.TrimSuffix(ansi.Strip(raw), "\n"), "\n")
			if len(lines) != 32 {
				t.Errorf("render height = %d rows, want 32; final rows = %q", len(lines), lines[max(0, len(lines)-4):])
			}
			for index, line := range lines {
				if got := ansi.StringWidth(line); got != tt.width {
					t.Errorf("line %d width = %d, want %d", index+1, got, tt.width)
				}
			}
			for _, required := range []string{
				"2 pull requests",
				"refreshing",
				"acme/api #184",
				"Fix session refresh",
				"Refreshes expired sessions",
				"cookies",
				"unsaved work.",
				"review: approved",
				"checks: passing",
				"acme/web #92",
				"DRAFT",
				"review: changes requested",
				"checks: failing",
				"Created: 2026-07-20 12:00 UTC",
				"Updated: 2026-07-29 12:00 UTC",
				"https://github.com/acme/api/pull/184",
				"↑/↓ move",
				"Tab focus",
				"? help",
			} {
				if !strings.Contains(ansi.Strip(raw), required) {
					t.Errorf("render missing %q", required)
				}
			}

			listLine := lineContaining(ansi.Strip(raw), "PULL REQUESTS")
			detailLine := lineContaining(ansi.Strip(raw), "DETAIL")
			if listLine < 0 || detailLine < 0 {
				t.Fatalf("pane headings not found: list=%d detail=%d", listLine, detailLine)
			}
			if tt.stacked && detailLine <= listLine {
				t.Fatalf("narrow detail line %d is not below list line %d", detailLine, listLine)
			}
			if !tt.stacked && detailLine != listLine {
				t.Fatalf("wide detail line %d is not beside list line %d", detailLine, listLine)
			}

			normalized := normalizeRender(raw)
			goldenPath := filepath.Join("testdata", tt.goldenFile)
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
				t.Fatalf("render differs from %s\n--- got ---\n%s\n--- want ---\n%s", tt.goldenFile, normalized, want)
			}
		})
	}
}

func TestRenderRefreshAndHelpStates(t *testing.T) {
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
				model := renderTestModel()
				return updateModel(t, model, refreshDoneMsg{err: errors.New("network unavailable")})
			},
			required: []string{
				"2 pull requests",
				"stale",
				"last updated",
				"acme/api #184",
				"Refresh failed: network unavailable",
			},
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
			required: []string{
				"0 pull requests",
				"error",
				"last updated never",
				"Refresh failed: network unavailable",
				"Press r to retry or q to quit.",
			},
		},
		{
			name:       "expanded help",
			goldenFile: "help.golden",
			width:      120,
			model: func() Model {
				model := renderTestModel()
				return updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
			},
			required: []string{
				"↑/↓ move or scroll",
				"Tab switch focus",
				"o open",
				"c copy URL",
				"C copy all as Slack",
				"r refresh",
				"? compact help",
				"q quit",
			},
		},
		{
			name:       "expanded help narrow",
			goldenFile: "help-narrow.golden",
			width:      80,
			model: func() Model {
				model := renderTestModel()
				return updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
			},
			required: []string{
				"↑/↓ move or scroll",
				"Tab switch focus",
				"o open",
				"c copy URL",
				"C copy all as Slack",
				"r refresh",
				"? compact help",
				"q quit",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.model()
			model = updateModel(t, model, tea.WindowSizeMsg{Width: tt.width, Height: 32})
			raw := model.View().Content
			stripped := ansi.Strip(raw)
			for _, required := range tt.required {
				if !strings.Contains(stripped, required) {
					t.Errorf("render missing %q", required)
				}
			}
			lines := strings.Split(strings.TrimSuffix(stripped, "\n"), "\n")
			if len(lines) != 32 {
				t.Errorf("render height = %d rows, want 32", len(lines))
			}
			for index, line := range lines {
				if got := ansi.StringWidth(line); got != tt.width {
					t.Errorf("line %d width = %d, want %d", index+1, got, tt.width)
				}
			}

			normalized := normalizeRender(raw)
			goldenPath := filepath.Join("testdata", tt.goldenFile)
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
				t.Fatalf("render differs from %s\n--- got ---\n%s\n--- want ---\n%s", tt.goldenFile, normalized, want)
			}
		})
	}
}

func TestRenderSanitizesUntrustedPullRequestTextWithoutMutatingDomainData(t *testing.T) {
	pullRequest := app.PullRequest{
		ID:         "PR_hostile",
		Repository: "acme/\x1b[31mapi\x1b[0m",
		Number:     9,
		Title:      "Fix \x1b]8;;https://evil.example\x1b\\linked\x1b]8;;\x1b\\ title\rback\bspace\u0085c1",
		Body:       "First \x1b]52;c;c2VjcmV0\x07line\r\nSecond\b line\u0085end",
		URL:        "https://github.com/acme/api/pull/9\x1b[2J",
		CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Review:     app.ReviewState("approved\x1b[2J\rnow"),
		Checks:     app.CheckState("passing\x1b]52;c;c3RhdHVz\x07\bnow"),
	}
	original := pullRequest
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot(pullRequest)),
		nil,
		testDependencies(),
	)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 180, Height: 32})

	raw := model.View().Content
	stripped := ansi.Strip(raw)
	for _, want := range []string{
		"acme/api #9",
		"Fix linked title�back�space�c1",
		"https://github.com/acme/api/pull/9",
		"First line",
		"Second� line�end",
		"Review: approved�now",
		"Checks: passing�now",
	} {
		if !strings.Contains(stripped, want) {
			t.Errorf("sanitized render missing %q:\n%s", want, stripped)
		}
	}
	for _, forbidden := range []string{
		"https://evil.example",
		"c2VjcmV0",
		"\x1b[31m",
		"\x1b[2J",
		"\r",
		"\b",
		"\u0085",
	} {
		if strings.Contains(stripped, forbidden) {
			t.Errorf("sanitized render contains %q:\n%s", forbidden, stripped)
		}
	}
	for _, forbidden := range []string{
		"\x1b[2J",
		"\x1b]8;",
		"\x1b]52;",
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("raw render contains untrusted terminal sequence %q", forbidden)
		}
	}
	if !strings.Contains(model.detailViewport.GetContent(), "\n") {
		t.Fatalf("sanitized detail body lost intended newline: %q", model.detailViewport.GetContent())
	}
	if !reflect.DeepEqual(model.pullRequests[0], original) {
		t.Fatalf("render mutated domain pull request = %#v, want %#v", model.pullRequests[0], original)
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

func renderTestModel() Model {
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	first := app.PullRequest{
		ID:         "PR_api_184",
		Repository: "acme/api",
		Number:     184,
		Title:      "Fix session refresh",
		Body:       "Refreshes expired sessions without forcing a new login, dropping cookies, or losing unsaved work.\n\n## Notes\nPreserves Markdown source.",
		URL:        "https://github.com/acme/api/pull/184",
		CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Review:     app.ReviewApproved,
		Checks:     app.ChecksPassing,
	}
	second := app.PullRequest{
		ID:         "PR_web_92",
		Repository: "acme/web",
		Number:     92,
		Title:      "Add invoice filters",
		Body:       "",
		URL:        "https://github.com/acme/web/pull/92",
		Draft:      true,
		CreatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 30, 14, 0, 0, 0, time.UTC),
		Review:     app.ReviewChangesRequested,
		Checks:     app.ChecksFailing,
	}
	cached := app.Snapshot{
		PullRequests: []app.PullRequest{first, second},
		FetchedAt:    now.Add(-time.Hour),
	}
	deps := testDependencies()
	deps.Now = func() time.Time { return now }
	return NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		&cached,
		nil,
		deps,
	)
}

var relativeAgePattern = regexp.MustCompile(`\b(?:now|\d+[mhd] ago)\b`)

func normalizeRender(render string) string {
	render = ansi.Strip(render)
	render = relativeAgePattern.ReplaceAllString(render, "<age>")
	lines := strings.Split(render, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func lineContaining(render, needle string) int {
	for index, line := range strings.Split(render, "\n") {
		if strings.Contains(line, needle) {
			return index
		}
	}
	return -1
}
