package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestServiceRefreshReaderFailureDoesNotSave(t *testing.T) {
	readerErr := errors.New("GitHub is unavailable")
	cache := &fakeSnapshotWriter{}
	service := Service{
		Reader: fakePullRequestReader{err: readerErr},
		Cache:  cache,
		Now:    func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
	}

	_, err := service.Refresh(context.Background(), Selection{Organization: "acme", Repositories: []string{"acme/api"}})
	if !errors.Is(err, readerErr) {
		t.Fatalf("Refresh() error = %v, want wrapped %v", err, readerErr)
	}
	if len(cache.saved) != 0 {
		t.Fatalf("Save() calls = %d, want 0", len(cache.saved))
	}
}

func TestServiceRefreshSortsTimestampsAndSavesFreshSnapshot(t *testing.T) {
	updated := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	source := []PullRequest{
		{Repository: "acme/web", Number: 3, Draft: true, UpdatedAt: updated},
		{Repository: "acme/api", Number: 2, UpdatedAt: updated.Add(time.Hour)},
		{Repository: "acme/api", Number: 1, UpdatedAt: updated},
	}
	cache := &fakeSnapshotWriter{}
	service := Service{
		Reader: fakePullRequestReader{pullRequests: source},
		Cache:  cache,
		Now: func() time.Time {
			return time.Date(2026, 7, 30, 8, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
		},
	}

	result, err := service.Refresh(context.Background(), Selection{
		Organization: " Acme ",
		Repositories: []string{" Acme/Web ", "acme/api", "ACME/web"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := pullRequestNumbers(result.Snapshot.PullRequests); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("snapshot pull request numbers = %v, want [1 2 3]", got)
	}
	wantFetchedAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if !result.Snapshot.FetchedAt.Equal(wantFetchedAt) || result.Snapshot.FetchedAt.Location() != time.UTC {
		t.Fatalf("snapshot fetched at = %v (%s), want %v (UTC)", result.Snapshot.FetchedAt, result.Snapshot.FetchedAt.Location(), wantFetchedAt)
	}
	if len(cache.saved) != 1 {
		t.Fatalf("Save() calls = %d, want 1", len(cache.saved))
	}
	wantSelection := Selection{
		Organization: "Acme",
		Repositories: []string{"acme/api", "Acme/Web"},
	}
	if !reflect.DeepEqual(cache.saved[0].selection, wantSelection) {
		t.Fatalf("saved selection = %#v, want normalized %#v", cache.saved[0].selection, wantSelection)
	}
	if !reflect.DeepEqual(cache.saved[0].snapshot, result.Snapshot) {
		t.Fatalf("saved snapshot = %#v, want %#v", cache.saved[0].snapshot, result.Snapshot)
	}
}

func TestServiceRefreshReturnsFreshDataWhenCacheSaveFails(t *testing.T) {
	cacheErr := errors.New("disk full")
	service := Service{
		Reader: fakePullRequestReader{pullRequests: []PullRequest{{Repository: "acme/api", Number: 8}}},
		Cache:  &fakeSnapshotWriter{err: cacheErr},
		Now:    func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
	}

	result, err := service.Refresh(context.Background(), Selection{Organization: "acme", Repositories: []string{"acme/api"}})
	if err != nil {
		t.Fatalf("Refresh() error = %v, want nil", err)
	}
	if !errors.Is(result.CacheWarning, cacheErr) {
		t.Fatalf("CacheWarning = %v, want wrapped %v", result.CacheWarning, cacheErr)
	}
	if got := pullRequestNumbers(result.Snapshot.PullRequests); !slices.Equal(got, []int{8}) {
		t.Fatalf("snapshot pull request numbers = %v, want [8]", got)
	}
}

func TestServiceRefreshDoesNotMutateReaderSlice(t *testing.T) {
	updated := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	source := []PullRequest{
		{Repository: "acme/web", Number: 3, Draft: true, UpdatedAt: updated},
		{Repository: "acme/api", Number: 2, UpdatedAt: updated.Add(time.Hour)},
		{Repository: "acme/api", Number: 1, UpdatedAt: updated},
	}
	wantSource := slices.Clone(source)
	service := Service{
		Reader: fakePullRequestReader{pullRequests: source},
		Cache:  &fakeSnapshotWriter{},
		Now:    func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
	}

	if _, err := service.Refresh(context.Background(), Selection{Organization: "acme", Repositories: []string{"acme/api", "acme/web"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source, wantSource) {
		t.Fatalf("reader source = %#v, want %#v", source, wantSource)
	}
}

func TestServiceRefreshRequiresDependencies(t *testing.T) {
	selection := Selection{Organization: "acme", Repositories: []string{"acme/api"}}
	tests := map[string]struct {
		service Service
		want    string
	}{
		"reader": {
			service: Service{Cache: &fakeSnapshotWriter{}, Now: func() time.Time { return time.Now() }},
			want:    "refresh pull requests: reader is required",
		},
		"cache": {
			service: Service{Reader: fakePullRequestReader{}, Now: func() time.Time { return time.Now() }},
			want:    "refresh pull requests: cache is required",
		},
		"clock": {
			service: Service{Reader: fakePullRequestReader{}, Cache: &fakeSnapshotWriter{}},
			want:    "refresh pull requests: clock is required",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := tt.service.Refresh(context.Background(), selection)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Refresh() error = %v, want %q", err, tt.want)
			}
		})
	}
}

type fakePullRequestReader struct {
	pullRequests []PullRequest
	err          error
}

func (r fakePullRequestReader) ListOpenPullRequests(context.Context, string, []string) ([]PullRequest, error) {
	return r.pullRequests, r.err
}

type fakeSnapshotWriter struct {
	saved []savedSnapshot
	err   error
}

type savedSnapshot struct {
	selection Selection
	snapshot  Snapshot
}

func (w *fakeSnapshotWriter) Save(selection Selection, snapshot Snapshot) error {
	w.saved = append(w.saved, savedSnapshot{selection: selection, snapshot: snapshot})
	return w.err
}

func pullRequestNumbers(pullRequests []PullRequest) []int {
	numbers := make([]int, len(pullRequests))
	for i, pullRequest := range pullRequests {
		numbers[i] = pullRequest.Number
	}
	return numbers
}
