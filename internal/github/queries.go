package github

const currentViewerQuery = `query CurrentViewer {
  viewer { login }
}`

const viewerOrganizationsQuery = `query ViewerOrganizations($cursor: String) {
  viewer {
    organizations(first: 100, after: $cursor) {
      nodes { login }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const organizationRepositoriesQuery = `query OrganizationRepositories($organization: String!, $cursor: String) {
  organization(login: $organization) {
    repositories(first: 100, after: $cursor, orderBy: {field: NAME, direction: ASC}) {
      nodes { nameWithOwner }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const openPullRequestsQuery = `query OpenPullRequests($search: String!, $cursor: String) {
  search(query: $search, type: ISSUE, first: 100, after: $cursor) {
    nodes {
      ... on PullRequest {
        id
        number
        title
        body
        url
        isDraft
        headRefName
        createdAt
        updatedAt
        reviewDecision
        statusCheckRollup { state }
        repository { nameWithOwner }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

var allQueries = map[string]string{
	"current viewer":            currentViewerQuery,
	"open pull requests":        openPullRequestsQuery,
	"organization repositories": organizationRepositoriesQuery,
	"viewer organizations":      viewerOrganizationsQuery,
}
