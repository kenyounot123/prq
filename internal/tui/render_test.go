package tui

import (
	"context"
	"os"
	"path/filepath"
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
