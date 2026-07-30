package store

import (
	"fmt"
	"os"
	"strings"

	"github.com/kenyounot123/prq/internal/app"
)

const cacheVersion = 2

type CacheStore struct {
	Path string
}

type cacheFile struct {
	Version int `json:"version"`
	app.Selection
	app.Snapshot
}

func (s CacheStore) Load(selection app.Selection) (app.Snapshot, error) {
	var stored cacheFile
	if err := readJSON(s.Path, &stored); err != nil {
		return app.Snapshot{}, err
	}
	if stored.Version != cacheVersion {
		return app.Snapshot{}, fmt.Errorf("load cache %s: version %d: %w", s.Path, stored.Version, ErrVersion)
	}
	storedSelection, err := app.NormalizeSelection(stored.Organization, stored.Repositories)
	if err != nil {
		return app.Snapshot{}, fmt.Errorf("load cache %s: %w: invalid cached selection: %v", s.Path, ErrInvalid, err)
	}
	expectedSelection, err := app.NormalizeSelection(selection.Organization, selection.Repositories)
	if err != nil {
		return app.Snapshot{}, fmt.Errorf("load cache %s: %w: invalid configured selection: %v", s.Path, ErrInvalid, err)
	}
	if !sameSelection(storedSelection, expectedSelection) {
		return app.Snapshot{}, fmt.Errorf("load cache %s: %w", s.Path, ErrSelectionMismatch)
	}
	if err := validateSnapshot(stored.Snapshot, storedSelection); err != nil {
		return app.Snapshot{}, fmt.Errorf("load cache %s: %w: %v", s.Path, ErrInvalid, err)
	}
	return stored.Snapshot, nil
}

func (s CacheStore) Save(selection app.Selection, snapshot app.Snapshot) error {
	normalized, err := app.NormalizeSelection(selection.Organization, selection.Repositories)
	if err != nil {
		return fmt.Errorf("save cache %s: %w: invalid selection: %v", s.Path, ErrInvalid, err)
	}
	if err := validateSnapshot(snapshot, normalized); err != nil {
		return fmt.Errorf("save cache %s: %w: %v", s.Path, ErrInvalid, err)
	}
	if err := writeJSONAtomic(s.Path, cacheFile{
		Version:   cacheVersion,
		Selection: normalized,
		Snapshot:  snapshot,
	}); err != nil {
		return fmt.Errorf("save cache %s: %w", s.Path, err)
	}
	return nil
}

func (s CacheStore) Clear() error {
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear cache %s: %w", s.Path, err)
	}
	return nil
}

func validateSnapshot(snapshot app.Snapshot, selection app.Selection) error {
	if snapshot.FetchedAt.IsZero() {
		return fmt.Errorf("fetched at is required")
	}
	for _, pullRequest := range snapshot.PullRequests {
		switch {
		case pullRequest.ID == "":
			return fmt.Errorf("pull request ID is required")
		case pullRequest.Repository == "":
			return fmt.Errorf("pull request repository is required")
		case !selectionContainsRepository(selection, pullRequest.Repository):
			return fmt.Errorf("pull request repository %q is outside the cached selection", pullRequest.Repository)
		case pullRequest.Number <= 0:
			return fmt.Errorf("pull request number is required")
		case pullRequest.Title == "":
			return fmt.Errorf("pull request title is required")
		case pullRequest.URL == "":
			return fmt.Errorf("pull request URL is required")
		case pullRequest.CreatedAt.IsZero():
			return fmt.Errorf("pull request created at is required")
		case pullRequest.UpdatedAt.IsZero():
			return fmt.Errorf("pull request updated at is required")
		}
	}
	return nil
}

func sameSelection(left, right app.Selection) bool {
	if !strings.EqualFold(left.Organization, right.Organization) ||
		len(left.Repositories) != len(right.Repositories) {
		return false
	}
	for index := range left.Repositories {
		if !strings.EqualFold(left.Repositories[index], right.Repositories[index]) {
			return false
		}
	}
	return true
}

func selectionContainsRepository(selection app.Selection, repository string) bool {
	for _, candidate := range selection.Repositories {
		if strings.EqualFold(candidate, repository) {
			return true
		}
	}
	return false
}
