package app

type StatusGroup int

const (
	StatusBlocked StatusGroup = iota
	StatusReady
	StatusWaiting
	StatusDraft
)

func StatusGroups() []StatusGroup {
	return []StatusGroup{StatusBlocked, StatusReady, StatusWaiting, StatusDraft}
}

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
