package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

var _ tea.Model = Model{}

func TestNewModelShowsCachedPullRequestsImmediately(t *testing.T) {
	cached := testSnapshot(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_two", "acme/web", 2),
	)

	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		&cached,
		nil,
		testDependencies(),
	)

	if got := len(model.pullRequests); got != 2 {
		t.Fatalf("visible pull requests = %d, want 2", got)
	}
	if got := model.pullRequests[0].ID; got != "PR_one" {
		t.Fatalf("first pull request ID = %q, want PR_one", got)
	}
	if model.loading {
		t.Fatal("cached model is loading, want cached data visible")
	}
}

func TestNewModelWithoutCacheStartsLoading(t *testing.T) {
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		nil,
		nil,
		testDependencies(),
	)

	if !model.loading {
		t.Fatal("model without cache is not loading")
	}
	if len(model.pullRequests) != 0 {
		t.Fatalf("visible pull requests = %d, want 0", len(model.pullRequests))
	}
}

func TestInitRequestsOneRefresh(t *testing.T) {
	selection := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/api"},
	}
	calls := 0
	deps := testDependencies()
	deps.Refresh = func(_ context.Context, got app.Selection) (app.RefreshResult, error) {
		calls++
		if got.Organization != selection.Organization {
			t.Fatalf("refresh organization = %q, want %q", got.Organization, selection.Organization)
		}
		return app.RefreshResult{Snapshot: testSnapshot()}, nil
	}
	model := NewModel(context.Background(), selection, nil, nil, deps)

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init returned nil command")
	}
	if _, ok := cmd().(refreshDoneMsg); !ok {
		t.Fatal("Init command did not return refreshDoneMsg")
	}
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
}

func TestRefreshKeyIsSuppressedWhileInitialRefreshIsActive(t *testing.T) {
	calls := 0
	deps := testDependencies()
	deps.Refresh = func(context.Context, app.Selection) (app.RefreshResult, error) {
		calls++
		return app.RefreshResult{}, nil
	}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		nil,
		nil,
		deps,
	)
	if !model.refreshing {
		t.Fatal("new model is not marked as refreshing")
	}

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	if cmd != nil {
		t.Fatal("second refresh returned a command while refresh active")
	}
	if next.(Model).refreshing != true {
		t.Fatal("suppressed refresh cleared active refresh state")
	}
	if calls != 0 {
		t.Fatalf("refresh calls during Update = %d, want 0", calls)
	}
}

func TestRefreshSuccessPreservesSelectionByPullRequestID(t *testing.T) {
	cached := testSnapshot(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_two", "acme/api", 2),
		testPullRequest("PR_three", "acme/api", 3),
	)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.selected = 1
	refreshed := testSnapshot(
		testPullRequest("PR_two", "acme/api", 2),
		testPullRequest("PR_three", "acme/api", 3),
	)
	refreshed.FetchedAt = cached.FetchedAt.Add(time.Hour)

	nextModel, _ := model.Update(refreshDoneMsg{
		result: app.RefreshResult{Snapshot: refreshed},
	})
	next := nextModel.(Model)

	if got := next.pullRequests[next.selected].ID; got != "PR_two" {
		t.Fatalf("selected pull request ID = %q, want PR_two", got)
	}
	if next.selected != 0 {
		t.Fatalf("selected index = %d, want 0", next.selected)
	}
	if next.refreshing {
		t.Fatal("successful refresh left refreshing active")
	}
	if next.loading {
		t.Fatal("successful refresh left loading active")
	}
}

func TestRefreshSuccessClampsSelectionWhenSelectedPullRequestWasRemoved(t *testing.T) {
	cached := testSnapshot(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_two", "acme/api", 2),
		testPullRequest("PR_three", "acme/api", 3),
	)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.selected = 2

	nextModel, _ := model.Update(refreshDoneMsg{
		result: app.RefreshResult{
			Snapshot: testSnapshot(testPullRequest("PR_one", "acme/api", 1)),
		},
	})
	next := nextModel.(Model)

	if next.selected != 0 {
		t.Fatalf("selected index = %d, want 0", next.selected)
	}
	if got := next.pullRequests[next.selected].ID; got != "PR_one" {
		t.Fatalf("selected pull request ID = %q, want PR_one", got)
	}
}

func TestEmptyRefreshAndFailedFirstRefreshHaveDistinctState(t *testing.T) {
	selection := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/api"},
	}
	emptyModel := NewModel(context.Background(), selection, nil, nil, testDependencies())
	emptyNext, _ := emptyModel.Update(refreshDoneMsg{
		result: app.RefreshResult{Snapshot: testSnapshot()},
	})
	empty := emptyNext.(Model)
	if empty.loading {
		t.Fatal("empty successful refresh is still loading")
	}
	if empty.refreshErr != nil {
		t.Fatalf("empty successful refresh error = %v, want nil", empty.refreshErr)
	}

	refreshErr := errors.New("network unavailable")
	failedModel := NewModel(context.Background(), selection, nil, nil, testDependencies())
	failedNext, _ := failedModel.Update(refreshDoneMsg{err: refreshErr})
	failed := failedNext.(Model)
	if failed.loading {
		t.Fatal("failed first refresh is still loading")
	}
	if !errors.Is(failed.refreshErr, refreshErr) {
		t.Fatalf("failed first refresh error = %v, want %v", failed.refreshErr, refreshErr)
	}
	if failed.stale {
		t.Fatal("failed first refresh is stale without cached data")
	}
}

func TestTabTogglesListAndDetailFocus(t *testing.T) {
	cached := testSnapshot(testPullRequest("PR_one", "acme/api", 1))
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	if model.focus != listFocus {
		t.Fatalf("initial focus = %v, want list focus", model.focus)
	}

	nextModel, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	next := nextModel.(Model)
	if next.focus != detailFocus {
		t.Fatalf("focus after first Tab = %v, want detail focus", next.focus)
	}

	nextModel, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	next = nextModel.(Model)
	if next.focus != listFocus {
		t.Fatalf("focus after second Tab = %v, want list focus", next.focus)
	}
}

func TestArrowKeysMoveSelectionInListFocus(t *testing.T) {
	cached := testSnapshot(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_two", "acme/api", 2),
		testPullRequest("PR_three", "acme/api", 3),
	)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)

	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if model.selected != 1 {
		t.Fatalf("selected index after Down = %d, want 1", model.selected)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if model.selected != 2 {
		t.Fatalf("selected index past list end = %d, want 2", model.selected)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if model.selected != 1 {
		t.Fatalf("selected index after Up = %d, want 1", model.selected)
	}
}

func TestArrowKeysScrollViewportInDetailFocus(t *testing.T) {
	pullRequest := testPullRequest("PR_one", "acme/api", 1)
	pullRequest.Body = strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")
	cached := testSnapshot(pullRequest)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.detailViewport.SetWidth(40)
	model.detailViewport.SetHeight(3)
	model.focus = detailFocus

	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if got := model.detailViewport.YOffset(); got != 1 {
		t.Fatalf("detail viewport offset after Down = %d, want 1", got)
	}
	if model.selected != 0 {
		t.Fatalf("selected index in detail focus = %d, want 0", model.selected)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if got := model.detailViewport.YOffset(); got != 0 {
		t.Fatalf("detail viewport offset after Up = %d, want 0", got)
	}
}

func TestMovingListSelectionReplacesDetailAndResetsScroll(t *testing.T) {
	first := testPullRequest("PR_one", "acme/api", 1)
	first.Body = "first line\nsecond line\nthird line"
	second := testPullRequest("PR_two", "acme/api", 2)
	second.Body = "replacement detail"
	cached := testSnapshot(first, second)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.detailViewport.SetWidth(40)
	model.detailViewport.SetHeight(1)
	model.detailViewport.ScrollDown(1)
	if model.detailViewport.YOffset() == 0 {
		t.Fatal("test setup did not scroll initial detail")
	}

	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	if got := model.detailViewport.GetContent(); got != second.Body {
		t.Fatalf("detail content = %q, want %q", got, second.Body)
	}
	if got := model.detailViewport.YOffset(); got != 0 {
		t.Fatalf("detail viewport offset after selection = %d, want 0", got)
	}
}

func TestRefreshReplacesSelectedDetailAndResetsScroll(t *testing.T) {
	selected := testPullRequest("PR_one", "acme/api", 1)
	selected.Body = "old first\nold second\nold third"
	cached := testSnapshot(selected)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.detailViewport.SetWidth(40)
	model.detailViewport.SetHeight(1)
	model.detailViewport.ScrollDown(1)
	refreshed := selected
	refreshed.Body = "new detail"

	model = updateModel(t, model, refreshDoneMsg{
		result: app.RefreshResult{Snapshot: testSnapshot(refreshed)},
	})

	if got := model.detailViewport.GetContent(); got != refreshed.Body {
		t.Fatalf("detail content after refresh = %q, want %q", got, refreshed.Body)
	}
	if got := model.detailViewport.YOffset(); got != 0 {
		t.Fatalf("detail viewport offset after refresh = %d, want 0", got)
	}
}

func TestEnterLeavesModelUnchangedAndReturnsNoCommand(t *testing.T) {
	cached := testSnapshot(testPullRequest("PR_one", "acme/api", 1))
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		Dependencies{},
	)
	before := model

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if cmd != nil {
		t.Fatal("Enter returned a command")
	}
	got := next.(Model)
	if !reflect.DeepEqual(got.selection, before.selection) ||
		!reflect.DeepEqual(got.pullRequests, before.pullRequests) ||
		!got.fetchedAt.Equal(before.fetchedAt) ||
		got.loading != before.loading ||
		got.refreshing != before.refreshing ||
		got.stale != before.stale ||
		got.refreshErr != before.refreshErr ||
		got.selected != before.selected ||
		got.focus != before.focus ||
		got.detailViewport.GetContent() != before.detailViewport.GetContent() ||
		got.detailViewport.YOffset() != before.detailViewport.YOffset() {
		t.Fatal("Enter changed model state")
	}
}

func TestWindowResizeUpdatesDimensionsAndDetailViewport(t *testing.T) {
	cached := testSnapshot(testPullRequest("PR_one", "acme/api", 1))
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)

	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 32})
	if model.width != 120 || model.height != 32 {
		t.Fatalf("wide dimensions = %dx%d, want 120x32", model.width, model.height)
	}
	if got := model.detailViewport.Width(); got != 68 {
		t.Fatalf("wide detail viewport width = %d, want 68", got)
	}
	if got := model.detailViewport.Height(); got != 18 {
		t.Fatalf("wide detail viewport height = %d, want 18", got)
	}

	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 32})
	if model.width != 80 || model.height != 32 {
		t.Fatalf("narrow dimensions = %dx%d, want 80x32", model.width, model.height)
	}
	if got := model.detailViewport.Width(); got != 76 {
		t.Fatalf("narrow detail viewport width = %d, want 76", got)
	}
	if got := model.detailViewport.Height(); got != 4 {
		t.Fatalf("narrow detail viewport height = %d, want 4", got)
	}
}

func TestNewModelTracksCacheWarningAndStartsWithCompactHelp(t *testing.T) {
	cacheWarning := errors.New("cache is corrupt")
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		nil,
		cacheWarning,
		testDependencies(),
	)

	if !errors.Is(model.cacheWarning, cacheWarning) {
		t.Fatalf("cache warning = %v, want %v", model.cacheWarning, cacheWarning)
	}
	if model.showHelp {
		t.Fatal("new model shows expanded help")
	}
	if model.statusMessage != "" {
		t.Fatalf("new model status = %q, want empty", model.statusMessage)
	}
}

func updateModel(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := model.Update(msg)
	return next.(Model)
}

func testDependencies() Dependencies {
	return Dependencies{
		Refresh: func(context.Context, app.Selection) (app.RefreshResult, error) {
			return app.RefreshResult{}, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
		},
	}
}

func testSnapshot(pullRequests ...app.PullRequest) app.Snapshot {
	return app.Snapshot{
		PullRequests: pullRequests,
		FetchedAt:    time.Date(2026, 7, 30, 15, 0, 0, 0, time.UTC),
	}
}

func testPullRequest(id, repository string, number int) app.PullRequest {
	return app.PullRequest{
		ID:         id,
		Repository: repository,
		Number:     number,
		Title:      "Pull request " + id,
		Body:       "Body for " + id,
		URL:        "https://github.com/" + repository + "/pull/1",
		CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Review:     app.ReviewRequired,
		Checks:     app.ChecksPending,
	}
}
