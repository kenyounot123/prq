package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kenyounot123/prq/internal/app"
)

func TestCacheStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.json")
	s := CacheStore{Path: path}
	selection := validSelection()
	want := validSnapshot()
	if err := s.Save(selection, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(selection)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestCacheStoreLoadClassifiesMissingFile(t *testing.T) {
	_, err := (CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}).Load(validSelection())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
}

func TestCacheStoreLoadClassifiesMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(path, []byte("{not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (CacheStore{Path: path}).Load(validSelection())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestCacheStoreLoadRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	writeJSONFixture(t, path, map[string]any{"version": 99})

	_, err := (CacheStore{Path: path}).Load(validSelection())
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("Load() error = %v, want ErrVersion", err)
	}
}

func TestCacheStoreLoadRejectsInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*app.Snapshot)
	}{
		{"zero fetched time", func(snapshot *app.Snapshot) { snapshot.FetchedAt = time.Time{} }},
		{"missing id", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].ID = "" }},
		{"missing repository", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].Repository = "" }},
		{"zero number", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].Number = 0 }},
		{"negative number", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].Number = -1 }},
		{"missing title", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].Title = "" }},
		{"missing url", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].URL = "" }},
		{"zero created time", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].CreatedAt = time.Time{} }},
		{"zero updated time", func(snapshot *app.Snapshot) { snapshot.PullRequests[0].UpdatedAt = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.json")
			snapshot := validSnapshot()
			tt.mutate(&snapshot)
			writeJSONFixture(t, path, map[string]any{
				"version":       2,
				"organization":  "acme",
				"repositories":  []string{"acme/api"},
				"pull_requests": snapshot.PullRequests,
				"fetched_at":    snapshot.FetchedAt,
			})

			_, err := (CacheStore{Path: path}).Load(validSelection())
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Load() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCacheStoreLoadRejectsCacheFromManualConfigurationChange(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}
	if err := store.Save(validSelection(), validSnapshot()); err != nil {
		t.Fatal(err)
	}

	manualSelection := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/web"},
	}
	_, err := store.Load(manualSelection)
	if !errors.Is(err, ErrSelectionMismatch) {
		t.Fatalf("Load() error = %v, want ErrSelectionMismatch", err)
	}
}

func TestCacheStoreLoadRejectsPullRequestOutsideBoundSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	snapshot := validSnapshot()
	writeJSONFixture(t, path, map[string]any{
		"version":       2,
		"organization":  "acme",
		"repositories":  []string{"acme/web"},
		"pull_requests": snapshot.PullRequests,
		"fetched_at":    snapshot.FetchedAt,
	})

	_, err := (CacheStore{Path: path}).Load(app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/web"},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid for out-of-selection pull request", err)
	}
}

func TestCacheStoreLoadMatchesNormalizedSelectionCaseInsensitively(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}
	if err := store.Save(
		app.Selection{Organization: " Acme ", Repositories: []string{" Acme/Web ", "acme/API"}},
		validSnapshot(),
	); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load(app.Selection{
		Organization: "acme",
		Repositories: []string{"ACME/api", "ACME/WEB"},
	})
	if err != nil {
		t.Fatalf("Load() error = %v, want matching normalized selection", err)
	}
}

func TestCacheStoreRejectsStaleConcurrentWriterAtSelectionBoundary(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}
	currentSelection := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/web"},
	}
	currentSnapshot := validSnapshot()
	currentSnapshot.PullRequests[0].Repository = "acme/web"
	if err := store.Save(currentSelection, currentSnapshot); err != nil {
		t.Fatal(err)
	}

	// An older dashboard refresh can finish after setup committed the new
	// selection. Its atomic write may win, but its identity must not.
	if err := store.Save(validSelection(), validSnapshot()); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load(currentSelection)
	if !errors.Is(err, ErrSelectionMismatch) {
		t.Fatalf("Load() error = %v, want ErrSelectionMismatch for stale writer", err)
	}
}

func TestCacheStoreClearIsIdempotent(t *testing.T) {
	s := CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear missing cache: %v", err)
	}
	if err := s.Save(validSelection(), validSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear existing cache: %v", err)
	}
	if _, err := os.Stat(s.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache stat error = %v, want not exist", err)
	}
}

func validSelection() app.Selection {
	return app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/api"},
	}
}

func validSnapshot() app.Snapshot {
	createdAt := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	return app.Snapshot{
		FetchedAt: createdAt.Add(time.Hour),
		PullRequests: []app.PullRequest{{
			ID:         "PR_kwDO123",
			Repository: "acme/api",
			Number:     42,
			Title:      "Add stores",
			URL:        "https://github.com/acme/api/pull/42",
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt.Add(30 * time.Minute),
		}},
	}
}
