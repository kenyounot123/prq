# PRQ TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an open-source, macOS-only Go TUI that shows the authenticated user's open pull requests from configured GitHub repositories and provides read-only browse, copy, refresh, and open actions.

**Architecture:** A small domain package owns PR data, normalization, ordering, formatting, and refresh orchestration. Thin adapters handle fixed read-only `gh api graphql` queries, local JSON stores, Huh setup forms, Bubble Tea rendering, and macOS `pbcopy`/`open` commands. The entry point supports only `prq`, `prq setup`, `--help`, and `--version`.

**Tech Stack:** Go 1.26, Bubble Tea v2, Bubbles v2, Lip Gloss v2, Huh v2, GitHub CLI, macOS system commands, Go test, GitHub Actions.

## Global Constraints

- Module path: `github.com/kenyounot123/prq`.
- Target Go version: 1.26; the current machine has Go 1.23.5 and a stale `GOROOT`, so Task 1 repairs the toolchain before project work.
- Target platform: macOS only; Linux, Windows, GitHub Enterprise Server, and Homebrew packaging are out of scope.
- GitHub authentication is owned by `gh`; PRQ never reads, stores, or logs a token.
- GitHub access is read-only. Every GraphQL document is a fixed named `query`; no mutation or generic API method is permitted.
- Invoke processes with `exec.CommandContext` and argument arrays; never construct a shell command from data.
- No Cobra, public JSON interface, AI summary, Slack API, reminders, search, repository discovery, or GitHub writes.
- The dashboard includes open authored PRs from one organization and a repository allowlist, including drafts.
- Sort non-drafts before drafts, then least recently updated, then repository, then PR number.
- Use side-by-side layout at 100 columns or wider and stacked layout below 100 columns.
- Keep the application domain independent of Bubble Tea, `gh`, and macOS commands.
- All automated tests run without GitHub credentials and without external writes.
- Every task ends with formatting, focused tests, the full available test suite, and a commit.

---

## Planned File Structure

```text
go.mod
go.sum
cmd/prq/main.go
cmd/prq/run.go
cmd/prq/run_test.go
internal/app/pull_request.go
internal/app/pull_request_test.go
internal/app/description.go
internal/app/description_test.go
internal/app/slack.go
internal/app/slack_test.go
internal/app/service.go
internal/app/service_test.go
internal/store/paths.go
internal/store/json.go
internal/store/config.go
internal/store/config_test.go
internal/store/cache.go
internal/store/cache_test.go
internal/github/runner.go
internal/github/queries.go
internal/github/client.go
internal/github/client_test.go
internal/github/pull_requests.go
internal/github/pull_requests_test.go
internal/github/testdata/*.json
internal/platform/actions.go
internal/platform/actions_test.go
internal/setup/flow.go
internal/setup/flow_test.go
internal/setup/huh.go
internal/tui/model.go
internal/tui/update.go
internal/tui/render.go
internal/tui/model_test.go
internal/tui/render_test.go
internal/tui/testdata/wide.golden
internal/tui/testdata/narrow.golden
testdata/bin/gh
testdata/bin/pbcopy
testdata/bin/open
LICENSE
README.md
CONTRIBUTING.md
CODE_OF_CONDUCT.md
SECURITY.md
.github/workflows/ci.yml
.github/ISSUE_TEMPLATE/bug_report.yml
.github/ISSUE_TEMPLATE/feature_request.yml
.github/PULL_REQUEST_TEMPLATE.md
docs/assets/prq-wide.svg
docs/assets/prq-narrow.svg
```

Responsibility rules:

- `internal/app` contains pure or interface-driven behavior and imports no other internal package.
- `internal/store`, `internal/github`, and `internal/platform` implement application-facing data and action boundaries.
- `internal/setup` coordinates GitHub discovery and local persistence, with Huh isolated behind `Prompter`.
- `internal/tui` owns only terminal state, commands, and rendering.
- `cmd/prq` constructs concrete dependencies and owns process exit behavior.

---

### Task 1: Go Module and Core Pull Request Model

**Files:**
- Create: `go.mod`
- Create: `internal/app/pull_request.go`
- Create: `internal/app/pull_request_test.go`

**Interfaces:**
- Produces: `app.Selection`, `app.PullRequest`, `app.Snapshot`, `app.ReviewState`, `app.CheckState`, `app.NormalizeSelection`, `app.SortPullRequests`, `app.ReviewStateFromGitHub`, and `app.CheckStateFromGitHub`.

- [ ] **Step 1: Repair and verify the Go 1.26 toolchain**

Run:

```bash
brew install go
unset GOROOT
hash -r
go version
```

Expected: `go version go1.26.x darwin/arm64`. Remove the stale exported `GOROOT=/opt/homebrew/opt/go/libexec` from the shell startup file if `go version` still resolves Go 1.23.5, then open a fresh shell and rerun the check.

- [ ] **Step 2: Initialize the module**

Run:

```bash
go mod init github.com/kenyounot123/prq
go mod edit -go=1.26
```

Expected: `go.mod` declares `module github.com/kenyounot123/prq` and `go 1.26`.

- [ ] **Step 3: Write failing domain tests**

Create table-driven tests covering normalization, duplicate repositories, invalid repository names, draft grouping, stale-first ordering, tie-breaking, and GitHub enum mapping. The central fixtures should be:

```go
func TestSortPullRequests(t *testing.T) {
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	prs := []PullRequest{
		{Repository: "acme/zeta", Number: 8, Draft: true, UpdatedAt: t0},
		{Repository: "acme/api", Number: 2, UpdatedAt: t0.Add(2 * time.Hour)},
		{Repository: "acme/api", Number: 1, UpdatedAt: t0},
		{Repository: "acme/web", Number: 3, Draft: true, UpdatedAt: t0.Add(-time.Hour)},
	}

	SortPullRequests(prs)

	got := []int{prs[0].Number, prs[1].Number, prs[2].Number, prs[3].Number}
	want := []int{1, 2, 3, 8}
	if !slices.Equal(got, want) {
		t.Fatalf("numbers = %v, want %v", got, want)
	}
}

func TestNormalizeSelection(t *testing.T) {
	got, err := NormalizeSelection("Acme", []string{"Acme/Web", "acme/api", "ACME/web"})
	if err != nil {
		t.Fatal(err)
	}
	want := Selection{Organization: "Acme", Repositories: []string{"acme/api", "Acme/Web"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeSelection() = %#v, want %#v", got, want)
	}
}
```

Use only the standard library in this first task.

- [ ] **Step 4: Run the domain tests and verify failure**

Run:

```bash
go test ./internal/app -run 'Test(SortPullRequests|NormalizeSelection)' -v
```

Expected: FAIL because the package types and functions do not exist.

- [ ] **Step 5: Implement the domain types and functions**

Use these exact public shapes:

```go
type ReviewState string

const (
	ReviewApproved         ReviewState = "approved"
	ReviewChangesRequested ReviewState = "changes requested"
	ReviewRequired         ReviewState = "review required"
	ReviewNone             ReviewState = "none"
	ReviewUnknown          ReviewState = "unknown"
)

type CheckState string

const (
	ChecksPassing CheckState = "passing"
	ChecksFailing CheckState = "failing"
	ChecksPending CheckState = "pending"
	ChecksNone    CheckState = "none"
	ChecksUnknown CheckState = "unknown"
)

type Selection struct {
	Organization string   `json:"organization"`
	Repositories []string `json:"repositories"`
}

type PullRequest struct {
	ID         string      `json:"id"`
	Repository string      `json:"repository"`
	Number     int         `json:"number"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	URL        string      `json:"url"`
	Draft      bool        `json:"draft"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Review     ReviewState `json:"review"`
	Checks     CheckState  `json:"checks"`
}

type Snapshot struct {
	PullRequests []PullRequest `json:"pull_requests"`
	FetchedAt    time.Time     `json:"fetched_at"`
}
```

`NormalizeSelection` must trim the organization, validate `owner/name`, require the owner to equal the selected organization case-insensitively, deduplicate case-insensitively, sort case-insensitively, and reject an empty repository list. `SortPullRequests` must use `sort.SliceStable` and the four-level order in the spec. Mapping functions must map empty input to `none` and unfamiliar input to `unknown`.

- [ ] **Step 6: Format and verify**

Run:

```bash
gofmt -w internal/app
go test ./internal/app -v
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod internal/app/pull_request.go internal/app/pull_request_test.go
git commit -m "feat: add pull request domain model"
```

---

### Task 2: Compact Descriptions and Slack Formatting

**Files:**
- Create: `internal/app/description.go`
- Create: `internal/app/description_test.go`
- Create: `internal/app/slack.go`
- Create: `internal/app/slack_test.go`

**Interfaces:**
- Consumes: `app.PullRequest`.
- Produces: `app.CompactDescription(body string) string`, `app.EscapeSlack(text string) string`, and `app.FormatSlackList(prs []PullRequest) string`.

- [ ] **Step 1: Write failing compact-description tests**

Cover blank bodies, headings, unordered and numbered list markers, whitespace collapse, CRLF, and a line containing only a Markdown marker:

```go
func TestCompactDescription(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"blank":        {" \n\t", "No description"},
		"heading":      {"\n## Refresh sessions\nMore detail", "Refresh sessions"},
		"bullet":       {"- Avoid repeated requests", "Avoid repeated requests"},
		"numbered":     {"1. Add cache\n2. Add tests", "Add cache"},
		"collapse":     {"Keeps   the\tcontext  visible", "Keeps the context visible"},
		"empty marker": {"-\nActual description", "Actual description"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := CompactDescription(tt.body); got != tt.want {
				t.Fatalf("CompactDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Write failing Slack-format tests**

```go
func TestFormatSlackList(t *testing.T) {
	prs := []PullRequest{
		{
			URL:   "https://github.com/acme/api/pull/184",
			Title: "Fix <sessions> & cookies",
			Body:  "Refreshes expired sessions.",
		},
		{
			URL:   "https://github.com/acme/web/pull/92",
			Title: "Invoice filters",
			Body:  "",
		},
	}
	want := "• <https://github.com/acme/api/pull/184|Fix &lt;sessions&gt; &amp; cookies> — Refreshes expired sessions.\n" +
		"• <https://github.com/acme/web/pull/92|Invoice filters> — No description"
	if got := FormatSlackList(prs); got != want {
		t.Fatalf("FormatSlackList() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
go test ./internal/app -run 'Test(CompactDescription|FormatSlackList)' -v
```

Expected: FAIL with undefined functions.

- [ ] **Step 4: Implement the minimal pure functions**

`CompactDescription` must split on lines, trim each line, remove one leading heading/list marker with a compiled regular expression, collapse whitespace with `strings.Fields`, and return the first nonempty result. `EscapeSlack` must replace `&` before `<` and `>` to prevent double escaping. `FormatSlackList` must preserve input order and must not sort or mutate its argument.

- [ ] **Step 5: Format, verify, and commit**

Run:

```bash
gofmt -w internal/app
go test ./internal/app -v
go test ./...
git add internal/app/description.go internal/app/description_test.go internal/app/slack.go internal/app/slack_test.go
git commit -m "feat: add pull request descriptions and Slack formatting"
```

Expected: all tests PASS and the commit succeeds.

---

### Task 3: Versioned Atomic Configuration and Cache Stores

**Files:**
- Create: `internal/store/paths.go`
- Create: `internal/store/json.go`
- Create: `internal/store/config.go`
- Create: `internal/store/config_test.go`
- Create: `internal/store/cache.go`
- Create: `internal/store/cache_test.go`

**Interfaces:**
- Consumes: `app.Selection` and `app.Snapshot`.
- Produces: `store.Paths`, `store.DefaultPaths`, `store.ConfigStore.Load/Save`, `store.CacheStore.Load/Save/Clear`, `store.ErrNotFound`, `store.ErrInvalid`, and `store.ErrVersion`.

- [ ] **Step 1: Write failing path and configuration tests**

Use temporary directories and injected `Paths`, never the user's real directories:

```go
func TestConfigStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	s := ConfigStore{Path: path}
	want := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/api", "acme/web"},
	}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
```

Add explicit tests for missing files, malformed JSON, version `99`, invalid selections, and preservation of the old valid file when the temporary write fails.

- [ ] **Step 2: Write failing cache tests**

Test snapshot round-trip, missing cache, malformed cache, version mismatch, and idempotent `Clear`:

```go
func TestCacheStoreClearIsIdempotent(t *testing.T) {
	s := CacheStore{Path: filepath.Join(t.TempDir(), "cache.json")}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear missing cache: %v", err)
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
go test ./internal/store -v
```

Expected: FAIL because the store package does not exist.

- [ ] **Step 4: Implement paths and atomic JSON writing**

Use:

```go
type Paths struct {
	Config string
	Cache  string
}

func DefaultPaths() (Paths, error)

var (
	ErrNotFound = errors.New("store file not found")
	ErrInvalid  = errors.New("store data is invalid")
	ErrVersion  = errors.New("store version is unsupported")
)
```

`DefaultPaths` must call `os.UserConfigDir` and `os.UserCacheDir`, returning:

```text
<config root>/prq/config.json
<cache root>/prq/pull-requests.json
```

`writeJSONAtomic` must create the parent with `0700`, create a temporary file in the same directory, set `0600`, encode JSON plus a trailing newline, `Sync`, close, and rename. Every error path must close and remove the temporary file.

- [ ] **Step 5: Implement versioned stores**

Use internal envelopes:

```go
type configFile struct {
	Version int `json:"version"`
	app.Selection
}

type cacheFile struct {
	Version int `json:"version"`
	app.Snapshot
}
```

Set both current versions to `1`. `ConfigStore.Load` must run `app.NormalizeSelection` after decoding. `CacheStore.Load` must reject a zero `FetchedAt` and any PR missing ID, repository, number, title, URL, or timestamps. Wrap errors with the path and preserve `errors.Is` for the sentinel class.

- [ ] **Step 6: Format, verify, and commit**

Run:

```bash
gofmt -w internal/store
go test ./internal/store -v
go test ./...
git add internal/store
git commit -m "feat: add versioned config and cache stores"
```

Expected: PASS.

---

### Task 4: Read-only GitHub Runner and Discovery Queries

**Files:**
- Create: `internal/github/runner.go`
- Create: `internal/github/queries.go`
- Create: `internal/github/client.go`
- Create: `internal/github/client_test.go`
- Create: `internal/github/testdata/viewer.json`
- Create: `internal/github/testdata/organizations-page-1.json`
- Create: `internal/github/testdata/organizations-page-2.json`
- Create: `internal/github/testdata/repositories.json`

**Interfaces:**
- Produces: `github.Runner`, `github.ExecRunner`, `github.NewExecRunner() (*ExecRunner, error)`, `github.Client`, `github.NewClient(Runner) *Client`, `Client.CheckAuth`, `Client.CurrentViewer`, `Client.ListOrganizations`, and `Client.ListRepositories`.

- [ ] **Step 1: Write the fake runner and failing discovery tests**

Define the seam expected by all GitHub tests:

```go
type fakeRunner struct {
	outputs [][]byte
	errs    []error
	calls   [][]string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, slices.Clone(args))
	i := len(f.calls) - 1
	return f.outputs[i], f.errs[i]
}
```

Tests must prove:

- `CheckAuth` invokes exactly `gh auth status`.
- `CurrentViewer` returns the fixture login.
- Organizations paginate and sort case-insensitively.
- Repositories paginate, return canonical `nameWithOwner`, and sort case-insensitively.
- A GraphQL response with a nonempty `errors` array fails with operation context.
- Runner errors never include the full response body.

- [ ] **Step 2: Add the query invariant test before implementations**

```go
func TestAllGraphQLDocumentsAreReadOnly(t *testing.T) {
	for name, document := range allQueries {
		normalized := strings.TrimSpace(document)
		if !strings.HasPrefix(normalized, "query ") {
			t.Errorf("%s must start with a named query", name)
		}
		if regexp.MustCompile(`(?i)\bmutation\b`).MatchString(normalized) {
			t.Errorf("%s contains a mutation operation", name)
		}
	}
}
```

This test is permanent and must enumerate viewer, organizations, repositories, and pull-request queries.

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
go test ./internal/github -run 'Test(CheckAuth|CurrentViewer|ListOrganizations|ListRepositories|AllGraphQL)' -v
```

Expected: FAIL because the package implementation does not exist.

- [ ] **Step 4: Implement the process runner**

Use:

```go
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type ExecRunner struct {
	Path string
}

type Client struct {
	runner Runner
}

func NewClient(runner Runner) *Client
```

`NewExecRunner` must use `exec.LookPath("gh")`. `ExecRunner.Run` must call `exec.CommandContext(ctx, r.Path, args...)`, capture stdout and stderr separately, and return a redacted error containing only the operation and trimmed stderr. It must never invoke a shell.

- [ ] **Step 5: Implement fixed discovery queries and cursor pagination**

Declare package-private named query constants for:

```graphql
query CurrentViewer {
  viewer { login }
}
```

```graphql
query ViewerOrganizations($cursor: String) {
  viewer {
    organizations(first: 100, after: $cursor) {
      nodes { login }
      pageInfo { hasNextPage endCursor }
    }
  }
}
```

```graphql
query OrganizationRepositories($organization: String!, $cursor: String) {
  organization(login: $organization) {
    repositories(first: 100, after: $cursor, orderBy: {field: NAME, direction: ASC}) {
      nodes { nameWithOwner }
      pageInfo { hasNextPage endCursor }
    }
  }
}
```

Each call must construct arguments without a shell:

```go
args := []string{
	"api",
	"graphql",
	"-f", "query=" + organizationRepositoriesQuery,
	"-F", "organization=" + organization,
}
if cursor != "" {
	args = append(args, "-F", "cursor="+cursor)
}
```

Omit the cursor argument on the first page. Stop only when `hasNextPage` is false; fail if it is true but `endCursor` is empty.

Expose these exact methods:

```go
func (c *Client) CheckAuth(ctx context.Context) error
func (c *Client) CurrentViewer(ctx context.Context) (string, error)
func (c *Client) ListOrganizations(ctx context.Context) ([]string, error)
func (c *Client) ListRepositories(ctx context.Context, organization string) ([]string, error)
```

- [ ] **Step 6: Format, verify, and commit**

Run:

```bash
gofmt -w internal/github
go test ./internal/github -v
go test ./...
git add internal/github
git commit -m "feat: add read-only GitHub discovery client"
```

Expected: PASS.

---

### Task 5: Authored Pull Request Query and Status Mapping

**Files:**
- Modify: `internal/github/queries.go`
- Create: `internal/github/pull_requests.go`
- Create: `internal/github/pull_requests_test.go`
- Create: `internal/github/testdata/pull-requests-page-1.json`
- Create: `internal/github/testdata/pull-requests-page-2.json`
- Create: `internal/github/testdata/pull-requests-partial-error.json`

**Interfaces:**
- Consumes: `github.Runner`, `github.Client`, `app.PullRequest`, and app status mapping.
- Produces: `Client.ListOpenPullRequests(ctx context.Context, organization string, repositories []string) ([]app.PullRequest, error)`.

- [ ] **Step 1: Write failing PR decoding and pagination tests**

Fixtures must include:

- A non-draft PR with `APPROVED` and `SUCCESS`.
- A draft PR with a null review decision and null status rollup.
- A PR with `CHANGES_REQUESTED` and `FAILURE`.
- A PR in an unconfigured repository that must be filtered out.
- A second page.
- Unknown review/check enum strings.
- A response with both `data` and GraphQL `errors`, which must fail.

The main assertion should verify the complete normalized `app.PullRequest`, not just its count.

- [ ] **Step 2: Write failing argument-safety tests**

Assert that:

- The search string is passed as separate arguments: `"-F", "search="+searchQuery`.
- The GraphQL document remains byte-for-byte equal to the fixed constant.
- Repository names never appear inside the GraphQL document.
- Only `api graphql`, the fixed query, and declared variables are passed to `gh`.

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
go test ./internal/github -run 'TestListOpenPullRequests' -v
```

Expected: FAIL because `ListOpenPullRequests` does not exist.

- [ ] **Step 4: Add the fixed PR query**

Use this field set:

```graphql
query OpenPullRequests($search: String!, $cursor: String) {
  search(query: $search, type: ISSUE, first: 100, after: $cursor) {
    nodes {
      ... on PullRequest {
        id
        number
        title
        body
        url
        isDraft
        createdAt
        updatedAt
        reviewDecision
        statusCheckRollup { state }
        repository { nameWithOwner }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Add it to `allQueries` so the read-only invariant test covers it.

- [ ] **Step 5: Implement pagination, filtering, and mapping**

`ListOpenPullRequests` must:

1. Validate the selection with `app.NormalizeSelection`.
2. Call `CurrentViewer`.
3. Build `is:pr is:open author:<viewer> org:<organization>` as a GraphQL variable.
4. Paginate until `hasNextPage` is false.
5. Reject partial GraphQL errors.
6. Filter by a lowercased repository allowlist.
7. Parse RFC3339 timestamps.
8. Map null status fields to `none` and unknown fields to `unknown`.
9. Return unsorted normalized PRs; sorting belongs to `internal/app`.

- [ ] **Step 6: Verify the permanent mutation guard still passes**

Run:

```bash
gofmt -w internal/github
go test ./internal/github -run 'Test(ListOpenPullRequests|AllGraphQLDocumentsAreReadOnly)' -v
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/github
git commit -m "feat: fetch authored open pull requests"
```

---

### Task 6: Refresh Service and Cache Semantics

**Files:**
- Create: `internal/app/service.go`
- Create: `internal/app/service_test.go`

**Interfaces:**
- Consumes: `app.Selection`, `app.PullRequest`, and `app.Snapshot`.
- Produces: `app.PullRequestReader`, `app.SnapshotWriter`, `app.RefreshResult`, `app.Service`, and `Service.Refresh`.

- [ ] **Step 1: Write failing refresh tests**

Use these interfaces and fakes in the tests:

```go
type PullRequestReader interface {
	ListOpenPullRequests(context.Context, string, []string) ([]PullRequest, error)
}

type SnapshotWriter interface {
	Save(Snapshot) error
}

type RefreshResult struct {
	Snapshot     Snapshot
	CacheWarning error
}
```

Test:

- Reader failure returns an error and never calls `Save`.
- Success sorts a copy, timestamps it with the injected clock, and saves it.
- Cache-save failure returns fresh data plus `CacheWarning`, not a refresh failure.
- The service does not mutate the reader's source slice.

- [ ] **Step 2: Run and verify failure**

Run:

```bash
go test ./internal/app -run TestServiceRefresh -v
```

Expected: FAIL with undefined service types.

- [ ] **Step 3: Implement the service**

Use:

```go
type Service struct {
	Reader PullRequestReader
	Cache  SnapshotWriter
	Now    func() time.Time
}

func (s Service) Refresh(ctx context.Context, selection Selection) (RefreshResult, error)
```

Validate nonnil dependencies, clone the returned slice, sort it, use UTC for `FetchedAt`, and wrap reader/cache errors with `refresh pull requests` or `save pull request cache`.

- [ ] **Step 4: Format, verify, and commit**

Run:

```bash
gofmt -w internal/app
go test ./internal/app -v
go test ./...
git add internal/app/service.go internal/app/service_test.go
git commit -m "feat: add pull request refresh service"
```

Expected: PASS.

---

### Task 7: macOS Clipboard and Browser Actions

**Files:**
- Create: `internal/platform/actions.go`
- Create: `internal/platform/actions_test.go`

**Interfaces:**
- Produces: `platform.Executor`, `platform.SystemExecutor`, `platform.Clipboard.Copy(ctx, text) error`, and `platform.Browser.Open(ctx, rawURL) error`.

- [ ] **Step 1: Write failing adapter tests**

Use a recording executor:

```go
type call struct {
	name  string
	args  []string
	stdin string
}

type fakeExecutor struct {
	calls []call
	err   error
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string, stdin string) error {
	f.calls = append(f.calls, call{name: name, args: slices.Clone(args), stdin: stdin})
	return f.err
}
```

Assert:

- Clipboard calls `pbcopy` with no args and exact stdin.
- Browser calls `open` with one distinct URL argument.
- Browser rejects HTTP, malformed URLs, non-`github.com` hosts, credentials, and fragments before invoking the executor.
- Executor failures preserve operation context.
- A non-macOS runtime returns an unsupported-platform error. Inject a `goos` string into adapters so this can be tested on macOS.

- [ ] **Step 2: Run and verify failure**

Run:

```bash
go test ./internal/platform -v
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement action interfaces and the system executor**

Use:

```go
type Executor interface {
	Run(ctx context.Context, name string, args []string, stdin string) error
}

type Clipboard struct {
	Exec Executor
	GOOS string
}

type Browser struct {
	Exec Executor
	GOOS string
}
```

`SystemExecutor.Run` must use `exec.CommandContext`, `strings.NewReader(stdin)`, and separate stdout/stderr buffers. The browser validator must require scheme `https`, hostname exactly `github.com`, no user info, no fragment, and a path beginning with `/`.

- [ ] **Step 4: Format, verify, and commit**

Run:

```bash
gofmt -w internal/platform
go test ./internal/platform -v
go test ./...
git add internal/platform
git commit -m "feat: add macOS clipboard and browser actions"
```

Expected: PASS.

---

### Task 8: Setup Orchestration and Huh Prompts

**Files:**
- Create: `internal/setup/flow.go`
- Create: `internal/setup/flow_test.go`
- Create: `internal/setup/huh.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: GitHub discovery methods, `app.Selection`, `store.ConfigStore.Save`, and `store.CacheStore.Clear`.
- Produces: `setup.GitHub`, `setup.ConfigWriter`, `setup.CacheInvalidator`, `setup.Prompter`, `setup.Flow.Run`, and `setup.HuhPrompter`.

- [ ] **Step 1: Add Huh v2**

Run:

```bash
go get charm.land/huh/v2
go mod tidy
```

Expected: Huh v2 and its pinned transitive dependencies appear in `go.mod` and `go.sum`.

- [ ] **Step 2: Write failing flow tests**

Define:

```go
type GitHub interface {
	CheckAuth(context.Context) error
	ListOrganizations(context.Context) ([]string, error)
	ListRepositories(context.Context, string) ([]string, error)
}

type ConfigWriter interface {
	Save(app.Selection) error
}

type CacheInvalidator interface {
	Clear() error
}

type Prompter interface {
	SelectOrganization(context.Context, []string) (string, error)
	SelectRepositories(context.Context, string, []string) ([]string, error)
}
```

Tests must assert exact call order:

```text
CheckAuth → ListOrganizations → SelectOrganization →
ListRepositories → SelectRepositories → Save → Clear
```

Also test empty organizations, empty repositories, prompt cancellation, invalid prompt output, save failure, and the rule that cache clear cannot happen before a successful save.

- [ ] **Step 3: Run and verify failure**

Run:

```bash
go test ./internal/setup -v
```

Expected: FAIL because setup types do not exist.

- [ ] **Step 4: Implement `Flow.Run`**

Use:

```go
type Flow struct {
	GitHub GitHub
	Config ConfigWriter
	Cache  CacheInvalidator
	Prompt Prompter
}

func (f Flow) Run(ctx context.Context) (app.Selection, error)
```

Wrap every stage with a concise operation name. Return the saved normalized selection only after `Clear` succeeds. Never invoke `gh auth login`.

- [ ] **Step 5: Implement Huh prompts**

`HuhPrompter` must use:

```go
huh.NewSelect[string]().
	Title("Choose a GitHub organization").
	Options(huh.NewOptions(organizations...)...).
	Value(&selected)
```

and:

```go
huh.NewMultiSelect[string]().
	Title("Choose repositories").
	Description("Press / to filter; space toggles a repository").
	Options(huh.NewOptions(repositories...)...).
	Validate(func(values []string) error {
		if len(values) == 0 {
			return errors.New("select at least one repository")
		}
		return nil
	}).
	Value(&selected)
```

Run each form with `RunWithContext(ctx)`.

- [ ] **Step 6: Format, verify, and commit**

Run:

```bash
gofmt -w internal/setup
go test ./internal/setup -v
go test ./...
git add go.mod go.sum internal/setup
git commit -m "feat: add guided repository setup"
```

Expected: PASS.

---

### Task 9: Bubble Tea Dashboard Model and Responsive Rendering

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/update.go`
- Create: `internal/tui/render.go`
- Create: `internal/tui/model_test.go`
- Create: `internal/tui/render_test.go`
- Create: `internal/tui/testdata/wide.golden`
- Create: `internal/tui/testdata/narrow.golden`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `app.Selection`, `app.Snapshot`, `app.RefreshResult`, `app.CompactDescription`, and Bubble Tea messages.
- Produces: `tui.Dependencies`, `tui.Model`, `tui.NewModel`, and a Bubble Tea v2 `tea.Model`.

- [ ] **Step 1: Add the pinned TUI libraries**

Run:

```bash
go get charm.land/bubbletea/v2@v2.0.6
go get charm.land/bubbles/v2@v2.1.0
go get charm.land/lipgloss/v2
go mod tidy
```

Expected: the v2 Charm modules appear in `go.mod` and `go.sum`.

- [ ] **Step 2: Write failing model-state tests**

Use:

```go
type Dependencies struct {
	Refresh func(context.Context, app.Selection) (app.RefreshResult, error)
	Copy    func(context.Context, string) error
	Open    func(context.Context, string) error
	Now     func() time.Time
}

func NewModel(
	ctx context.Context,
	selection app.Selection,
	cached *app.Snapshot,
	cacheWarning error,
	deps Dependencies,
) Model
```

Tests must assert:

- Cached PRs are visible immediately.
- No cache starts in a loading state.
- `Init` requests one refresh.
- A second refresh is suppressed while one is active.
- Selection is clamped after a refresh removes items.
- Empty and failed-first-refresh states are distinct.
- `Tab` toggles list/detail focus.
- Up/down selects rows in list focus and scrolls the viewport in detail focus.
- `Enter` leaves state unchanged and returns no command.

Construct key messages as:

```go
tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
```

- [ ] **Step 3: Write failing wide and narrow render tests**

Create a fixed snapshot and clock. Send:

```go
tea.WindowSizeMsg{Width: 120, Height: 32}
```

and:

```go
tea.WindowSizeMsg{Width: 80, Height: 32}
```

Normalize ANSI escape codes and replace relative ages with fixed values before comparing complete output to `wide.golden` and `narrow.golden`. Assert separately that wide output places list/detail on the same row and narrow output places detail below the list.

- [ ] **Step 4: Run and verify failure**

Run:

```bash
go test ./internal/tui -v
```

Expected: FAIL because the TUI package does not exist.

- [ ] **Step 5: Implement model state and refresh messages**

Use package-private messages:

```go
type refreshDoneMsg struct {
	result app.RefreshResult
	err    error
}

type actionDoneMsg struct {
	action string
	err    error
}
```

The model must track context, selection, PRs, fetched time, stale state, refreshing state, refresh error, selected index, focus, dimensions, help state, status message, and a Bubbles viewport for the detail body. `Init` returns the refresh command.

- [ ] **Step 6: Implement deterministic responsive rendering**

Use:

- `wideLayoutMin = 100`.
- A 40/60 split after borders and padding.
- Header with PR count, refreshing/stale state, and last successful refresh.
- Rows containing repository/number, title, compact description, draft, review, checks, and relative age.
- Detail metadata plus full body wrapped to the pane width.
- Stacked list/detail panes below 100 columns.
- Footer with compact keys; `?` toggles full help.

`View` must return:

```go
view := tea.NewView(m.render())
view.AltScreen = true
view.WindowTitle = "prq"
return view
```

- [ ] **Step 7: Generate and review golden files**

Add an `UPDATE_GOLDEN=1` branch in render tests that writes the normalized actual render to `testdata/*.golden`. Run:

```bash
UPDATE_GOLDEN=1 go test ./internal/tui -run TestRender -v
go test ./internal/tui -run TestRender -v
```

Inspect both golden files and confirm all required metadata and key hints are visible at their target sizes.

- [ ] **Step 8: Format, verify, and commit**

Run:

```bash
gofmt -w internal/tui
go test ./internal/tui -v
go test ./...
git add go.mod go.sum internal/tui
git commit -m "feat: add responsive pull request dashboard"
```

Expected: PASS.

---

### Task 10: Dashboard Copy, Open, Refresh, Help, and Error Actions

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/update.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/model_test.go`
- Modify: `internal/tui/render_test.go`

**Interfaces:**
- Consumes: `tui.Dependencies.Copy/Open/Refresh`, `app.FormatSlackList`, and selected `app.PullRequest`.
- Produces: complete global key behavior for `o`, `c`, `C`, `r`, `?`, and `q`.

- [ ] **Step 1: Write failing action tests**

Test commands by calling the returned `tea.Cmd` and feeding its message back into `Update`:

```go
next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
if cmd == nil {
	t.Fatal("copy key returned nil command")
}
msg := cmd()
next, _ = next.Update(msg)
```

Assert:

- `c` copies only the selected URL.
- `C` copies `app.FormatSlackList` for all visible PRs in dashboard order.
- `o` opens only the selected URL.
- Empty lists make `c`, `C`, and `o` no-ops with a status explanation.
- Success and failure produce correct status-line text.
- `r` starts one refresh and ignores repeated `r` while active.
- Failed refresh with cache retains PRs, marks stale, and shows last success.
- Failed refresh without cache shows retry/quit error state.
- Cache-warning success shows fresh PRs plus a warning.
- `?` toggles help.
- `q` returns `tea.Quit`.

- [ ] **Step 2: Run and verify failure**

Run:

```bash
go test ./internal/tui -run 'Test(Action|Refresh|Help|Quit)' -v
```

Expected: FAIL because the actions are not wired.

- [ ] **Step 3: Implement global actions**

Keep side effects inside Bubble Tea commands. Never call copy/open/refresh directly during `Update`. Use `context.Context` from the model and return `actionDoneMsg` or `refreshDoneMsg`.

Uppercase copy must be matched with `msg.String() == "C"`; lowercase copy remains `"c"`. Refresh success replaces PRs and fetched time, clears stale/error state, and updates the detail viewport. Refresh failure preserves existing data.

- [ ] **Step 4: Update golden render files**

Add golden cases for stale cached data, first-refresh failure, and expanded help. Regenerate and inspect:

```bash
UPDATE_GOLDEN=1 go test ./internal/tui -run TestRender -v
go test ./internal/tui -v
```

Expected: PASS.

- [ ] **Step 5: Format, verify, and commit**

Run:

```bash
gofmt -w internal/tui
go test ./internal/tui -v
go test ./...
git add internal/tui
git commit -m "feat: add dashboard actions and refresh states"
```

Expected: PASS.

---

### Task 11: Command Routing and End-to-end Dependency Wiring

**Files:**
- Create: `cmd/prq/main.go`
- Create: `cmd/prq/run.go`
- Create: `cmd/prq/run_test.go`
- Create: `testdata/bin/gh`
- Create: `testdata/bin/pbcopy`
- Create: `testdata/bin/open`

**Interfaces:**
- Consumes: every concrete package built in Tasks 1-10.
- Produces: the `prq` executable and its four supported command forms.

- [ ] **Step 1: Write failing routing tests**

Separate routing from construction:

```go
type command int

const (
	commandDashboard command = iota
	commandSetup
	commandHelp
	commandVersion
)

func parseCommand(args []string) (command, error)
```

Test:

```text
[]                 → dashboard
["setup"]          → setup
["--help"], ["-h"] → help
["--version"]      → version
anything else      → concise error
```

- [ ] **Step 2: Write failing orchestration tests**

Inject:

```go
type runtimeDeps struct {
	loadConfig  func() (app.Selection, error)
	loadCache   func() (app.Snapshot, error)
	runSetup    func(context.Context) (app.Selection, error)
	runDashboard func(context.Context, app.Selection, *app.Snapshot, error) error
}
```

Drive it through:

```go
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int

func runWithDeps(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps runtimeDeps,
) int
```

Assert:

- Missing config on `prq` runs setup, then dashboard.
- Invalid config reports its path and directs the user to `prq setup`.
- Explicit setup always runs setup, then dashboard.
- Valid config loads cache before dashboard.
- Missing cache passes nil without warning.
- Corrupt cache passes nil plus a warning so dashboard refreshes.
- Help/version write only to stdout.
- Usage and operational errors write to stderr and return nonzero.

- [ ] **Step 3: Run and verify failure**

Run:

```bash
go test ./cmd/prq -v
```

Expected: FAIL because command routing does not exist.

- [ ] **Step 4: Implement routing and composition**

`main.go` must only:

```go
func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
```

Declare `var version = "dev"` in `run.go`; `--version` prints `prq dev`. This variable is the sole future linker-flag injection point for tagged releases.

`run.go` constructs:

1. `store.DefaultPaths`.
2. `store.ConfigStore` and `store.CacheStore`.
3. `github.NewExecRunner` and `github.NewClient`.
4. `setup.Flow` with `setup.HuhPrompter`.
5. `app.Service`.
6. `platform.SystemExecutor`, `platform.Clipboard`, and `platform.Browser`.
7. `tui.Model` and `tea.NewProgram`.

Handle SIGINT through Bubble Tea; do not add a separate terminal raw-mode layer.
Translate a missing `gh` binary to `GitHub CLI not found; install it with: brew install gh`, and an authentication failure to `GitHub authentication required; run: gh auth login`.

- [ ] **Step 5: Add deterministic fake executables**

The fake `gh` script must switch on its arguments and emit checked-in fixture JSON for auth, viewer, organization, repository, and PR queries. It must exit `64` for any unrecognized command and exit `65` if its stdin or arguments contain `mutation`.

The fake `pbcopy` script writes stdin to `$PRQ_TEST_CAPTURE_DIR/clipboard.txt`. The fake `open` script writes one argument per line to `$PRQ_TEST_CAPTURE_DIR/open.txt`. Each script starts with `#!/bin/sh`, uses `set -eu`, and is executable.

- [ ] **Step 6: Add process-level smoke tests**

Build the binary and run help/version against the executable:

```bash
go build -o "$TMPDIR/prq-test" ./cmd/prq
"$TMPDIR/prq-test" --help
"$TMPDIR/prq-test" --version
```

Add a Go test that prepends the fake-bin directory to `PATH`, redirects config/cache roots to temporary directories through injected paths, and confirms no external command outside the allowlist runs.

- [ ] **Step 7: Format, verify, and commit**

Run:

```bash
gofmt -w cmd/prq
go test ./cmd/prq -v
go test ./...
go vet ./...
go build ./cmd/prq
git add cmd/prq testdata/bin
git commit -m "feat: wire the prq command"
```

Expected: PASS.

---

### Task 12: Open-source Documentation, Policies, CI, and Screenshots

**Files:**
- Create: `LICENSE`
- Create: `README.md`
- Create: `CONTRIBUTING.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `SECURITY.md`
- Create: `.github/workflows/ci.yml`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `.github/ISSUE_TEMPLATE/feature_request.yml`
- Create: `.github/PULL_REQUEST_TEMPLATE.md`
- Create: `docs/assets/prq-wide.svg`
- Create: `docs/assets/prq-narrow.svg`
- Modify: `docs/superpowers/specs/2026-07-30-prq-tui-design.md`

**Interfaces:**
- Consumes: final commands, keys, requirements, golden renders, and read-only guarantees.
- Produces: contributor-ready repository documentation and automated macOS checks.

- [ ] **Step 1: Add the MIT license and policy documents**

Use the standard MIT text with copyright:

```text
Copyright (c) 2026 kenyounot123
```

Use Contributor Covenant 2.1 for `CODE_OF_CONDUCT.md`. `SECURITY.md` must state:

- PRQ shells out to the user's authenticated `gh`.
- PRQ does not store tokens.
- All GraphQL operations are fixed read-only queries.
- Reports of any possible GitHub write path are security issues.
- Users should report privately through GitHub's security-advisory feature, not a public issue.

- [ ] **Step 2: Write the README**

Include these exact sections:

```text
What PRQ does
Status: macOS-only v1
Prerequisites: Go 1.26 and authenticated gh
Install: go install github.com/kenyounot123/prq/cmd/prq@latest
First run
Keyboard controls
Data and privacy
Read-only GitHub guarantee
Development
Contributing
License
```

Document both local storage locations and the Slack bulk-copy format. State all v1 exclusions without promising dates.

- [ ] **Step 3: Add contributor guidance and templates**

`CONTRIBUTING.md` must give exact commands:

```bash
go mod download
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/prq
```

Require tests for behavior changes and explicit review of the read-only query guard for GitHub changes. Issue templates must request macOS version, terminal, `prq --version`, `gh --version`, reproduction steps, expected behavior, and redacted logs.

- [ ] **Step 4: Generate screenshots from actual golden renders**

Install the documentation-only renderer:

```bash
brew install charmbracelet/tap/freeze
```

Generate images from the reviewed render fixtures:

```bash
freeze internal/tui/testdata/wide.golden -o docs/assets/prq-wide.svg
freeze internal/tui/testdata/narrow.golden -o docs/assets/prq-narrow.svg
```

Open both SVGs and verify that text is legible and no personal organization, repository, username, token, or local path appears. Embed both images in the README.

- [ ] **Step 5: Add macOS CI**

Create one workflow named `CI` triggered by pushes and pull requests. Use `macos-latest`, `actions/checkout`, and `actions/setup-go` with Go `1.26.x`. Run:

```yaml
- run: test -z "$(gofmt -l .)"
- run: go vet ./...
- run: go test -race ./...
- run: go build ./cmd/prq
```

Grant only:

```yaml
permissions:
  contents: read
```

Do not provide GitHub, Slack, or other secrets.

- [ ] **Step 6: Mark the design spec approved**

Change:

```text
Status: Awaiting written-spec review
```

to:

```text
Status: Approved
```

- [ ] **Step 7: Verify docs and commit**

Run:

```bash
rg -n 'TB[D]|TO[D]O|FIX[M]E|your-username|example[.]com' README.md CONTRIBUTING.md SECURITY.md .github docs
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build ./cmd/prq
git diff --check
```

Expected: the placeholder scan returns no matches, and all checks PASS.

Commit:

```bash
git add LICENSE README.md CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md .github docs/assets docs/superpowers/specs/2026-07-30-prq-tui-design.md
git commit -m "docs: prepare prq for open source contributions"
```

---

### Task 13: Final Acceptance Verification

**Files:**
- Modify only files required to correct a verification failure; do not add features.

**Interfaces:**
- Consumes: the complete application.
- Produces: evidence that every acceptance criterion is satisfied.

- [ ] **Step 1: Run the complete automated verification**

Run:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build -trimpath -o /tmp/prq ./cmd/prq
/tmp/prq --help
/tmp/prq --version
git diff --check
```

Expected: every command exits zero.

- [ ] **Step 2: Audit read-only GitHub access**

Run:

```bash
rg -n 'mutation|gh (pr|issue|repo).*?(edit|create|close|merge)|api .*--method|api .*-X' --glob '*.go' .
go test ./internal/github -run TestAllGraphQLDocumentsAreReadOnly -v
```

Expected: the only `mutation` matches are the invariant test and explanatory comments; no write-oriented `gh` command or HTTP method appears; the guard test passes.

- [ ] **Step 3: Run a manual macOS smoke test**

With the user's real authenticated `gh` session:

```bash
go run ./cmd/prq setup
```

Verify:

1. Organization and searchable repository selection work.
2. The dashboard opens after setup.
3. Cached data appears before a later refresh.
4. Wide and narrow layouts switch at 100 columns.
5. List navigation and detail scrolling work.
6. `c` copies one URL.
7. `C` copies Slack-formatted lines in display order.
8. `o` opens the selected `https://github.com/...` PR.
9. `r`, `?`, and `q` behave as documented.
10. No GitHub content changes.

- [ ] **Step 4: Inspect local persistence and repository cleanliness**

Run:

```bash
stat -f '%Sp %N' "$HOME/Library/Application Support/prq/config.json"
stat -f '%Sp %N' "$HOME/Library/Caches/prq/pull-requests.json"
git status --short
```

Expected: both local files are user-readable/writable only, neither is tracked, and the worktree is clean.

- [ ] **Step 5: Commit only if verification required a correction**

If a correction was required, rerun Steps 1-4 and then commit the minimal fix:

```bash
git add -u
git commit -m "fix: satisfy final prq verification"
```

If no correction was required, do not create an empty commit.
