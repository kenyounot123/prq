package app

import "testing"

func TestStatusGroupOf(t *testing.T) {
	tests := map[string]struct {
		pr   PullRequest
		want StatusGroup
	}{
		"draft": {
			pr:   PullRequest{Draft: true, Review: ReviewApproved, Checks: ChecksPassing},
			want: StatusDraft,
		},
		"draft with failing checks stays a draft": {
			pr:   PullRequest{Draft: true, Review: ReviewChangesRequested, Checks: ChecksFailing},
			want: StatusDraft,
		},
		"failing checks block": {
			pr:   PullRequest{Review: ReviewApproved, Checks: ChecksFailing},
			want: StatusBlocked,
		},
		"changes requested blocks": {
			pr:   PullRequest{Review: ReviewChangesRequested, Checks: ChecksPassing},
			want: StatusBlocked,
		},
		"approved and passing is ready": {
			pr:   PullRequest{Review: ReviewApproved, Checks: ChecksPassing},
			want: StatusReady,
		},
		"no review and no checks is ready": {
			pr:   PullRequest{Review: ReviewNone, Checks: ChecksNone},
			want: StatusReady,
		},
		"approved without checks is ready": {
			pr:   PullRequest{Review: ReviewApproved, Checks: ChecksNone},
			want: StatusReady,
		},
		"review required waits": {
			pr:   PullRequest{Review: ReviewRequired, Checks: ChecksPassing},
			want: StatusWaiting,
		},
		"pending checks wait": {
			pr:   PullRequest{Review: ReviewApproved, Checks: ChecksPending},
			want: StatusWaiting,
		},
		"unknown review waits": {
			pr:   PullRequest{Review: ReviewUnknown, Checks: ChecksPassing},
			want: StatusWaiting,
		},
		"unknown checks wait": {
			pr:   PullRequest{Review: ReviewApproved, Checks: ChecksUnknown},
			want: StatusWaiting,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := StatusGroupOf(tt.pr); got != tt.want {
				t.Fatalf("StatusGroupOf() = %d, want %d", got, tt.want)
			}
		})
	}
}
