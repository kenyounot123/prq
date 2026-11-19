package app

// StatusGroup says what the viewer can do about a pull request. The constants
// are in the order the dashboard displays the groups.
type StatusGroup int

const (
	StatusBlocked StatusGroup = iota
	StatusReady
	StatusWaiting
	StatusDraft
)

// StatusGroups returns every group in display order.
func StatusGroups() []StatusGroup {
	return []StatusGroup{StatusBlocked, StatusReady, StatusWaiting, StatusDraft}
}

// StatusGroupOf checks the rules in order, so a draft with failing checks is a
// draft and a pull request with unresolved states waits on review.
func StatusGroupOf(pr PullRequest) StatusGroup {
	switch {
	case pr.Draft:
		return StatusDraft
	case pr.Checks == ChecksFailing || pr.Review == ReviewChangesRequested:
		return StatusBlocked
	case (pr.Review == ReviewApproved || pr.Review == ReviewNone) &&
		(pr.Checks == ChecksPassing || pr.Checks == ChecksNone):
		return StatusReady
	default:
		return StatusWaiting
	}
}
