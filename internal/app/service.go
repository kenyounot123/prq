package app

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type PullRequestReader interface {
	ListOpenPullRequests(context.Context, string, []string) ([]PullRequest, error)
}

type SnapshotWriter interface {
	Save(Selection, Snapshot) error
}

type RefreshResult struct {
	Snapshot     Snapshot
	CacheWarning error
}

type Service struct {
	Reader PullRequestReader
	Cache  SnapshotWriter
	Now    func() time.Time
}

func (s Service) Refresh(ctx context.Context, selection Selection) (RefreshResult, error) {
	if s.Reader == nil {
		return RefreshResult{}, fmt.Errorf("refresh pull requests: reader is required")
	}
	if s.Cache == nil {
		return RefreshResult{}, fmt.Errorf("refresh pull requests: cache is required")
	}
	if s.Now == nil {
		return RefreshResult{}, fmt.Errorf("refresh pull requests: clock is required")
	}

	selection, err := NormalizeSelection(selection.Organization, selection.Repositories)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("refresh pull requests: validate selection: %w", err)
	}

	pullRequests, err := s.Reader.ListOpenPullRequests(ctx, selection.Organization, selection.Repositories)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("refresh pull requests: %w", err)
	}

	snapshot := Snapshot{
		PullRequests: slices.Clone(pullRequests),
		FetchedAt:    s.Now().UTC(),
	}
	SortPullRequests(snapshot.PullRequests)

	result := RefreshResult{Snapshot: snapshot}
	if err := s.Cache.Save(selection, snapshot); err != nil {
		result.CacheWarning = fmt.Errorf("save pull request cache: %w", err)
	}
	return result, nil
}
