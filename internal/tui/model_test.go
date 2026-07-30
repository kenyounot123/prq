package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestResizeBreaksOverlongDetailTokenIntoReachableViewportLines(t *testing.T) {
	token := strings.Repeat("x", 120)
	pullRequest := testPullRequest("PR_one", "acme/api", 1)
	pullRequest.Body = token
	cached := testSnapshot(pullRequest)
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)

	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 32})

	contentLines := strings.Split(model.detailViewport.GetContent(), "\n")
	if len(contentLines) < 2 {
		t.Fatalf("wrapped detail lines = %d, want at least 2", len(contentLines))
	}
	for index, line := range contentLines {
		if got := ansi.StringWidth(line); got > model.detailViewport.Width() {
			t.Errorf(
				"detail line %d width = %d, want at most viewport width %d",
				index+1,
				got,
				model.detailViewport.Width(),
			)
		}
	}
	if got := strings.Join(contentLines, ""); got != token {
		t.Fatalf("vertically reachable detail content length = %d, want %d", len(got), len(token))
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

func TestActionCopySelectedURL(t *testing.T) {
	first := testPullRequest("PR_one", "acme/api", 1)
	second := testPullRequest("PR_two", "acme/web", 2)
	copied := ""
	deps := testDependencies()
	deps.Copy = func(_ context.Context, text string) error {
		copied = text
		return nil
	}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		snapshotPointer(testSnapshot(first, second)),
		nil,
		deps,
	)
	model.selected = 1

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
	if cmd == nil {
		t.Fatal("copy key returned nil command")
	}
	if copied != "" {
		t.Fatalf("copy ran during Update with %q", copied)
	}
	next, _ = next.Update(cmd())
	got := next.(Model)

	if copied != second.URL {
		t.Fatalf("copied text = %q, want selected URL %q", copied, second.URL)
	}
	if got.statusMessage != "Copied pull request URL." {
		t.Fatalf("copy status = %q, want success confirmation", got.statusMessage)
	}
}

func TestActionCopyAllUsesVisibleDashboardOrder(t *testing.T) {
	first := testPullRequest("PR_one", "acme/web", 2)
	first.Title = "First visible"
	second := testPullRequest("PR_two", "acme/api", 1)
	second.Title = "Second visible"
	copied := ""
	deps := testDependencies()
	deps.Copy = func(_ context.Context, text string) error {
		copied = text
		return nil
	}
	pullRequests := []app.PullRequest{first, second}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		snapshotPointer(testSnapshot(pullRequests...)),
		nil,
		deps,
	)

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "C", Code: 'C'}))
	if cmd == nil {
		t.Fatal("copy-all key returned nil command")
	}
	next, _ = next.Update(cmd())
	got := next.(Model)

	want := app.FormatSlackList(pullRequests)
	if copied != want {
		t.Fatalf("copied Slack list = %q, want %q", copied, want)
	}
	if got.statusMessage != "Copied 2 pull requests." {
		t.Fatalf("copy-all status = %q, want success confirmation", got.statusMessage)
	}
}

func TestActionOpenSelectedURL(t *testing.T) {
	first := testPullRequest("PR_one", "acme/api", 1)
	second := testPullRequest("PR_two", "acme/web", 2)
	opened := ""
	deps := testDependencies()
	deps.Open = func(_ context.Context, rawURL string) error {
		opened = rawURL
		return nil
	}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}},
		snapshotPointer(testSnapshot(first, second)),
		nil,
		deps,
	)
	model.selected = 1

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	if cmd == nil {
		t.Fatal("open key returned nil command")
	}
	if opened != "" {
		t.Fatalf("open ran during Update with %q", opened)
	}
	next, _ = next.Update(cmd())
	got := next.(Model)

	if opened != second.URL {
		t.Fatalf("opened URL = %q, want selected URL %q", opened, second.URL)
	}
	if got.statusMessage != "Opened pull request." {
		t.Fatalf("open status = %q, want success confirmation", got.statusMessage)
	}
}

func TestActionFailuresAppearInStatusLine(t *testing.T) {
	actionErr := errors.New("helper unavailable")
	pullRequest := testPullRequest("PR_one", "acme/api", 1)
	tests := []struct {
		name       string
		key        tea.Key
		configure  func(*Dependencies)
		wantStatus string
	}{
		{
			name: "copy selected",
			key:  tea.Key{Text: "c", Code: 'c'},
			configure: func(deps *Dependencies) {
				deps.Copy = func(context.Context, string) error { return actionErr }
			},
			wantStatus: "Copy failed: helper unavailable",
		},
		{
			name: "copy all",
			key:  tea.Key{Text: "C", Code: 'C'},
			configure: func(deps *Dependencies) {
				deps.Copy = func(context.Context, string) error { return actionErr }
			},
			wantStatus: "Copy failed: helper unavailable",
		},
		{
			name: "open",
			key:  tea.Key{Text: "o", Code: 'o'},
			configure: func(deps *Dependencies) {
				deps.Open = func(context.Context, string) error { return actionErr }
			},
			wantStatus: "Open failed: helper unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDependencies()
			tt.configure(&deps)
			model := NewModel(
				context.Background(),
				app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
				snapshotPointer(testSnapshot(pullRequest)),
				nil,
				deps,
			)

			next, cmd := model.Update(tea.KeyPressMsg(tt.key))
			if cmd == nil {
				t.Fatal("action key returned nil command")
			}
			next, _ = next.Update(cmd())
			if got := next.(Model).statusMessage; got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

func TestActionKeysAreSafeNoOpsWithoutPullRequests(t *testing.T) {
	calls := 0
	deps := testDependencies()
	deps.Copy = func(context.Context, string) error {
		calls++
		return nil
	}
	deps.Open = func(context.Context, string) error {
		calls++
		return nil
	}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot()),
		nil,
		deps,
	)

	for _, key := range []tea.Key{
		{Text: "c", Code: 'c'},
		{Text: "C", Code: 'C'},
		{Text: "o", Code: 'o'},
	} {
		next, cmd := model.Update(tea.KeyPressMsg(key))
		if cmd != nil {
			t.Fatalf("empty-list action %q returned a command", key.Text)
		}
		model = next.(Model)
		if model.statusMessage != "No pull request selected." {
			t.Fatalf("empty-list action %q status = %q", key.Text, model.statusMessage)
		}
	}
	if calls != 0 {
		t.Fatalf("empty-list actions called dependencies %d times, want 0", calls)
	}
}

func TestRefreshKeyStartsOneRefreshAndSuppressesRepeatedKey(t *testing.T) {
	refreshed := testSnapshot(testPullRequest("PR_new", "acme/api", 2))
	calls := 0
	deps := testDependencies()
	deps.Refresh = func(context.Context, app.Selection) (app.RefreshResult, error) {
		calls++
		return app.RefreshResult{Snapshot: refreshed}, nil
	}
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot(testPullRequest("PR_old", "acme/api", 1))),
		nil,
		deps,
	)
	model = updateModel(t, model, refreshDoneMsg{
		result: app.RefreshResult{Snapshot: testSnapshot(testPullRequest("PR_old", "acme/api", 1))},
	})

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	if cmd == nil {
		t.Fatal("manual refresh returned nil command")
	}
	active := next.(Model)
	if !active.refreshing {
		t.Fatal("manual refresh did not set refreshing state")
	}
	if calls != 0 {
		t.Fatalf("refresh ran during Update %d times, want 0", calls)
	}
	suppressed, duplicateCmd := active.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	if duplicateCmd != nil {
		t.Fatal("repeated manual refresh returned a command")
	}
	if !suppressed.(Model).refreshing {
		t.Fatal("repeated manual refresh cleared refreshing state")
	}

	next, _ = active.Update(cmd())
	got := next.(Model)
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
	if got.refreshing {
		t.Fatal("completed manual refresh remains active")
	}
	if len(got.pullRequests) != 1 || got.pullRequests[0].ID != "PR_new" {
		t.Fatalf("visible pull requests after refresh = %#v, want fresh result", got.pullRequests)
	}
}

func TestRefreshFailurePreservesCachedDataAndMarksItStale(t *testing.T) {
	cached := testSnapshot(testPullRequest("PR_cached", "acme/api", 1))
	refreshErr := errors.New("network unavailable")
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model.width = 120

	next, _ := model.Update(refreshDoneMsg{err: refreshErr})
	got := next.(Model)

	if len(got.pullRequests) != 1 || got.pullRequests[0].ID != "PR_cached" {
		t.Fatalf("cached pull requests changed after failure: %#v", got.pullRequests)
	}
	if !got.fetchedAt.Equal(cached.FetchedAt) {
		t.Fatalf("last success = %v, want %v", got.fetchedAt, cached.FetchedAt)
	}
	if !got.stale {
		t.Fatal("failed cached refresh is not marked stale")
	}
	if status := strings.TrimSpace(ansi.Strip(got.renderStatus())); status != "Refresh failed: network unavailable" {
		t.Fatalf("failure status = %q", status)
	}
}

func TestRefreshFailureTreatsEmptyCachedSnapshotAsStaleData(t *testing.T) {
	cached := testSnapshot()
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		&cached,
		nil,
		testDependencies(),
	)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 32})

	next, _ := model.Update(refreshDoneMsg{err: errors.New("network unavailable")})
	got := next.(Model)
	rendered := ansi.Strip(got.View().Content)

	if !got.stale {
		t.Fatal("failed refresh after empty cached snapshot is not marked stale")
	}
	if !got.fetchedAt.Equal(cached.FetchedAt) {
		t.Fatalf("last success = %v, want %v", got.fetchedAt, cached.FetchedAt)
	}
	if !strings.Contains(rendered, "No open pull requests.") {
		t.Fatalf("stale empty render does not preserve successful empty result:\n%s", rendered)
	}
	if strings.Contains(rendered, "Press r to retry or q to quit.") {
		t.Fatalf("stale empty render shows first-refresh error state:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Refresh failed: network unavailable") {
		t.Fatalf("stale empty render does not show refresh failure:\n%s", rendered)
	}
}

func TestRefreshCompletionSupersedesEarlierActionStatus(t *testing.T) {
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot(testPullRequest("PR_cached", "acme/api", 1))),
		nil,
		testDependencies(),
	)
	model.statusMessage = "Copied pull request URL."

	t.Run("failure", func(t *testing.T) {
		next, _ := model.Update(refreshDoneMsg{err: errors.New("network unavailable")})
		got := next.(Model)
		if got.statusMessage != "" {
			t.Fatalf("status after refresh failure = %q, want transient action status cleared", got.statusMessage)
		}
	})

	t.Run("cache warning", func(t *testing.T) {
		next, _ := model.Update(refreshDoneMsg{
			result: app.RefreshResult{
				Snapshot:     testSnapshot(testPullRequest("PR_fresh", "acme/api", 2)),
				CacheWarning: errors.New("disk full"),
			},
		})
		got := next.(Model)
		if got.statusMessage != "" {
			t.Fatalf("status after refresh warning = %q, want transient action status cleared", got.statusMessage)
		}
	})
}

func TestRefreshSuccessReplacesCacheWarningWithCurrentWarning(t *testing.T) {
	oldWarning := errors.New("old cache unreadable")
	newWarning := errors.New("disk full")
	fresh := testSnapshot(testPullRequest("PR_fresh", "acme/api", 2))
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		nil,
		oldWarning,
		testDependencies(),
	)
	model.width = 120

	next, _ := model.Update(refreshDoneMsg{
		result: app.RefreshResult{Snapshot: fresh, CacheWarning: newWarning},
	})
	got := next.(Model)

	if len(got.pullRequests) != 1 || got.pullRequests[0].ID != "PR_fresh" {
		t.Fatalf("fresh pull requests = %#v", got.pullRequests)
	}
	if !errors.Is(got.cacheWarning, newWarning) {
		t.Fatalf("cache warning = %v, want %v", got.cacheWarning, newWarning)
	}
	if status := strings.TrimSpace(ansi.Strip(got.renderStatus())); status != "Warning: disk full" {
		t.Fatalf("warning status = %q", status)
	}
}

func TestHelpKeyTogglesExpandedHelp(t *testing.T) {
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot()),
		nil,
		testDependencies(),
	)
	model.width = 120

	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if !model.showHelp {
		t.Fatal("help key did not expand help")
	}
	if footer := ansi.Strip(model.renderFooter()); !strings.Contains(footer, "copy all as Slack") {
		t.Fatalf("expanded footer = %q", footer)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if model.showHelp {
		t.Fatal("second help key did not restore compact help")
	}
}

func TestQuitKeyReturnsTeaQuit(t *testing.T) {
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot()),
		nil,
		testDependencies(),
	)

	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("quit key returned nil command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit command returned %T, want tea.QuitMsg", cmd())
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

func snapshotPointer(snapshot app.Snapshot) *app.Snapshot {
	return &snapshot
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
