package store

import (
	"fmt"
	"os"

	"github.com/kenyounot123/prq/internal/app"
)

const cacheVersion = 1

type CacheStore struct {
	Path string
}

type cacheFile struct {
	Version int `json:"version"`
	app.Snapshot
}

func (s CacheStore) Load() (app.Snapshot, error) {
	var stored cacheFile
	if err := readJSON(s.Path, &stored); err != nil {
		return app.Snapshot{}, err
	}
	if stored.Version != cacheVersion {
		return app.Snapshot{}, fmt.Errorf("load cache %s: version %d: %w", s.Path, stored.Version, ErrVersion)
	}
	if err := validateSnapshot(stored.Snapshot); err != nil {
		return app.Snapshot{}, fmt.Errorf("load cache %s: %w: %v", s.Path, ErrInvalid, err)
	}
	return stored.Snapshot, nil
}

func (s CacheStore) Save(snapshot app.Snapshot) error {
	if err := writeJSONAtomic(s.Path, cacheFile{Version: cacheVersion, Snapshot: snapshot}); err != nil {
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

func validateSnapshot(snapshot app.Snapshot) error {
	if snapshot.FetchedAt.IsZero() {
		return fmt.Errorf("fetched at is required")
	}
	for _, pullRequest := range snapshot.PullRequests {
		switch {
		case pullRequest.ID == "":
			return fmt.Errorf("pull request ID is required")
		case pullRequest.Repository == "":
			return fmt.Errorf("pull request repository is required")
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
