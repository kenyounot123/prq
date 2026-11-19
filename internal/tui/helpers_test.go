package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

func selectedID(model Model) string {
	pullRequest, ok := model.selectedPullRequest()
	if !ok {
		return ""
	}
	return pullRequest.ID
}

// selectPullRequest puts the cursor on a pull request through the row list, so
// tests do not depend on where headers fall.
func selectPullRequest(t *testing.T, model Model, id string) Model {
	t.Helper()
	for index, row := range model.rows() {
		if !row.isHeader() && model.pullRequests[row.pullRequest].ID == id {
			model.selected = index
			return model
		}
	}
	t.Fatalf("pull request %q is not on a visible row", id)
	return model
}

func pullRequestByID(t *testing.T, model Model, id string) app.PullRequest {
	t.Helper()
	for _, pullRequest := range model.pullRequests {
		if pullRequest.ID == id {
			return pullRequest
		}
	}
	t.Fatalf("no pull request %q", id)
	return app.PullRequest{}
}

func footerText(model Model) string {
	return ansi.Strip(strings.Join(model.footerLines(), "\n"))
}

func lineContaining(render, needle string) int {
	for index, line := range strings.Split(render, "\n") {
		if strings.Contains(line, needle) {
			return index
		}
	}
	return -1
}

func normalizeRender(render string) string {
	lines := strings.Split(ansi.Strip(render), "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// renderTestModel holds one pull request per interesting case: two blocked, one
// ready, two waiting (one without a description) and a collapsed draft.
func renderTestModel() Model {
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	pullRequests := []app.PullRequest{
		{
			ID:         "PR_api_201",
			Repository: "acme/api",
			Number:     201,
			Title:      "Rotate signing keys without invalidating live sessions or the refresh token cache",
			Body:       "Rotates keys on a schedule.\n\n- old keys stay valid for 24h\n- new keys are published first",
			Branch:     "ken/rotate-keys",
			URL:        "https://github.com/acme/api/pull/201",
			CreatedAt:  now.Add(-3 * time.Hour),
			UpdatedAt:  now.Add(-20 * time.Minute),
			Review:     app.ReviewRequired,
			Checks:     app.ChecksFailing,
		},
		{
			ID:         "PR_web_77",
			Repository: "acme/web",
			Number:     77,
			Title:      "Migrate billing webhooks",
			Body:       "Moves the handlers to the new queue.",
			URL:        "https://github.com/acme/web/pull/77",
			CreatedAt:  now.Add(-50 * time.Hour),
			UpdatedAt:  now.Add(-2 * time.Hour),
			Review:     app.ReviewChangesRequested,
			Checks:     app.ChecksPassing,
		},
		{
			ID:         "PR_api_184",
			Repository: "acme/api",
			Number:     184,
			Title:      "Fix session refresh",
			Body:       "Refreshes expired sessions without forcing a new login, dropping cookies, or losing unsaved work.\n\n## Notes\nPreserves Markdown source.",
			Branch:     "ken/session-refresh",
			URL:        "https://github.com/acme/api/pull/184",
			CreatedAt:  now.Add(-10 * 24 * time.Hour),
			UpdatedAt:  now.Add(-24 * time.Hour),
			Review:     app.ReviewApproved,
			Checks:     app.ChecksPassing,
		},
		{
			ID:         "PR_api_190",
			Repository: "acme/api",
			Number:     190,
			Title:      "Cache session lookups",
			Body:       "Adds a short lived cache.",
			Branch:     "ken/session-cache",
			URL:        "https://github.com/acme/api/pull/190",
			CreatedAt:  now.Add(-5 * time.Hour),
			UpdatedAt:  now.Add(-4 * time.Hour),
			Review:     app.ReviewRequired,
			Checks:     app.ChecksPending,
		},
		{
			ID:         "PR_web_85",
			Repository: "acme/web",
			Number:     85,
			Title:      "Trim bundle size",
			URL:        "https://github.com/acme/web/pull/85",
			CreatedAt:  now.Add(-26 * time.Hour),
			UpdatedAt:  now.Add(-26 * time.Hour),
			Review:     app.ReviewRequired,
			Checks:     app.ChecksPassing,
		},
		{
			ID:         "PR_web_92",
			Repository: "acme/web",
			Number:     92,
			Title:      "Add invoice filters",
			URL:        "https://github.com/acme/web/pull/92",
			Draft:      true,
			CreatedAt:  now.Add(-5 * 24 * time.Hour),
			UpdatedAt:  now.Add(-2 * time.Hour),
			Review:     app.ReviewChangesRequested,
			Checks:     app.ChecksFailing,
		},
	}
	cached := app.Snapshot{PullRequests: pullRequests, FetchedAt: now.Add(-8 * time.Minute)}
	deps := testDependencies()
	deps.Now = func() time.Time { return now }
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		&cached,
		nil,
		deps,
	)
	model.refreshing = false
	return model
}

func (m *Model) selectAll() {
	for _, pullRequest := range m.pullRequests {
		m.checked[pullRequest.ID] = true
	}
}
