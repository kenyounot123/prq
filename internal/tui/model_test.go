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

func TestArrowKeysMoveHighlight(t *testing.T) {
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
		!reflect.DeepEqual(got.checked, before.checked) {
		t.Fatal("Enter changed model state")
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
	model.checked[first.ID] = true

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

func TestRefreshOutcomeWinsOverEarlierActionCompletionInBothMessageOrders(t *testing.T) {
	tests := []struct {
		name       string
		completion tea.Msg
		wantStatus string
	}{
		{
			name:       "failure",
			completion: refreshDoneMsg{err: errors.New("network unavailable")},
			wantStatus: "Refresh failed: network unavailable",
		},
		{
			name: "cache warning",
			completion: refreshDoneMsg{
				result: app.RefreshResult{
					Snapshot:     testSnapshot(testPullRequest("PR_fresh", "acme/api", 2)),
					CacheWarning: errors.New("disk full"),
				},
			},
			wantStatus: "Warning: disk full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newModel := func() Model {
				deps := testDependencies()
				deps.Copy = func(context.Context, string) error { return nil }
				model := NewModel(
					context.Background(),
					app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
					snapshotPointer(testSnapshot(testPullRequest("PR_cached", "acme/api", 1))),
					nil,
					deps,
				)
				model.width = 120
				return model
			}
			startCopy := func(t *testing.T, model Model) (Model, tea.Msg) {
				t.Helper()
				next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
				if cmd == nil {
					t.Fatal("copy key returned nil command")
				}
				return next.(Model), cmd()
			}
			assertRefreshStatus := func(t *testing.T, model Model) {
				t.Helper()
				status := strings.TrimSpace(ansi.Strip(model.renderStatus()))
				if status != tt.wantStatus {
					t.Fatalf("status = %q, want refresh outcome %q", status, tt.wantStatus)
				}
			}

			t.Run("action completion arrives first", func(t *testing.T) {
				started, actionMsg := startCopy(t, newModel())
				actionFirst := updateModel(t, started, actionMsg)
				refreshLast := updateModel(t, actionFirst, tt.completion)
				assertRefreshStatus(t, refreshLast)
			})

			t.Run("refresh completion arrives first", func(t *testing.T) {
				started, actionMsg := startCopy(t, newModel())
				refreshFirst := updateModel(t, started, tt.completion)
				actionLast := updateModel(t, refreshFirst, actionMsg)
				assertRefreshStatus(t, actionLast)

				afterRefresh, currentActionMsg := startCopy(t, refreshFirst)
				currentAction := updateModel(t, afterRefresh, currentActionMsg)
				if currentAction.statusMessage != "Copied pull request URL." {
					t.Fatalf(
						"action started after refresh status = %q, want normal confirmation",
						currentAction.statusMessage,
					)
				}
			})
		})
	}
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

func TestCheckboxesAreIndependentOfHighlight(t *testing.T) {
	model := renderTestModel()
	space := tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "})
	model = updateModel(t, model, space)
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if !model.checked["PR_api_184"] || model.checked["PR_web_92"] || model.selected != 1 {
		t.Fatalf("moving highlight changed checkboxes: %#v, highlight %d", model.checked, model.selected)
	}
	if detail := ansi.Strip(model.renderDetailPane(80, 12)); !strings.Contains(detail, "Add invoice filters") || strings.Contains(detail, "Fix session refresh") {
		t.Fatalf("detail does not follow highlight: %s", detail)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	model = updateModel(t, model, space)
	if len(model.checked) != 0 {
		t.Fatalf("second Space did not uncheck: %#v", model.checked)
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	if len(model.checked) != 2 {
		t.Fatalf("select all checked %d PRs, want 2", len(model.checked))
	}
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if len(model.checked) != 0 {
		t.Fatalf("Esc did not clear checkboxes: %#v", model.checked)
	}
}

func TestCheckedCopyUsesDashboardOrderAndKeepsSelection(t *testing.T) {
	model := renderTestModel()
	third := testPullRequest("PR_three", "acme/api", 3)
	model.pullRequests = append(model.pullRequests, third)
	space := tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "})
	model.selected = 2
	model = updateModel(t, model, space)
	model.selected = 0
	model = updateModel(t, model, space)
	model.selected = 1 // The highlighted PR is not checked.
	copied := ""
	model.deps.Copy = func(_ context.Context, text string) error { copied = text; return nil }
	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
	if cmd == nil {
		t.Fatal("checked copy returned no command")
	}
	next, _ = next.Update(cmd())
	model = next.(Model)
	want := app.FormatSlackList([]app.PullRequest{model.pullRequests[0], third})
	if copied != want {
		t.Fatalf("copied %q, want %q", copied, want)
	}
	if model.statusMessage != "Copied 2 PRs." || len(model.checked) != 2 {
		t.Fatalf("copy result: status %q, checkboxes %#v", model.statusMessage, model.checked)
	}
	model.selected = 0
	model = updateModel(t, model, space)
	next, cmd = model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
	if cmd == nil {
		t.Fatal("single checked copy returned no command")
	}
	next, _ = next.Update(cmd())
	if copied != app.FormatSlackList([]app.PullRequest{third}) || next.(Model).statusMessage != "Copied 1 PR." {
		t.Fatalf("single checked copy = %q, status %q", copied, next.(Model).statusMessage)
	}
}

func TestRefreshPreservesCheckboxesByIDAndDropsRemovedPRs(t *testing.T) {
	model := renderTestModel()
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	model = updateModel(t, model, refreshDoneMsg{err: errors.New("offline")})
	if len(model.checked) != 2 {
		t.Fatal("failed refresh cleared checkboxes")
	}
	remaining := model.pullRequests[0]
	remaining.Title = "Updated title"
	fresh := testPullRequest("PR_new", "acme/api", 3)
	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{Snapshot: testSnapshot(fresh, remaining)}})
	if len(model.checked) != 1 || !model.checked[remaining.ID] || model.selected != 1 {
		t.Fatalf("refresh lost selection: checked %#v, highlight %d", model.checked, model.selected)
	}
	if detail := ansi.Strip(model.renderDetailPane(80, 12)); !strings.Contains(detail, remaining.Title) {
		t.Fatalf("refresh did not update detail: %s", detail)
	}
	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{Snapshot: testSnapshot()}})
	if len(model.checked) != 0 {
		t.Fatal("empty refresh retained removed PRs")
	}
	for _, key := range []tea.Key{{Code: tea.KeySpace, Text: " "}, {Code: 'a', Text: "a"}, {Code: tea.KeyEscape}} {
		model = updateModel(t, model, tea.KeyPressMsg(key))
		if len(model.checked) != 0 {
			t.Fatal("empty list acquired checkboxes")
		}
	}
}
