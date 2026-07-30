# PRQ TUI Design

Date: 2026-07-30
Status: Approved

## Summary

PRQ is a personal, open-source terminal application for reviewing the status of pull requests authored by the current GitHub user. It presents open pull requests from a configured set of repositories in one place, restores context without requiring a browser, and makes selected or bulk PR links easy to copy.

Version 1 is a macOS-only TUI written in Go with Bubble Tea. It reuses the authenticated GitHub CLI, performs read-only GitHub queries, displays cached data immediately, and refreshes in the background. PRQ never modifies GitHub.

## Goals

- Show every open PR authored by the current GitHub user in selected repositories, including drafts.
- Make the dashboard useful immediately by rendering the last successful cache before refreshing.
- Let the user navigate entirely with the keyboard.
- Show a compact description in the list and the full PR description with status metadata in a persistent detail pane.
- Copy the selected PR URL or every displayed PR as Slack-formatted bullets.
- Open the selected PR in the default browser.
- Provide a guided first-run setup and a repeatable `prq setup` flow.
- Keep the codebase approachable for learning Go and accepting open-source contributions.
- Enforce read-only GitHub behavior in the component API, implementation, and tests.

## Non-goals

Version 1 will not include:

- GitHub mutations of any kind, including comments, review requests, labels, assignments, state changes, merges, or edits.
- Slack API or webhook integration.
- Reminder tracking or notification scheduling.
- AI-generated summaries.
- A public JSON interface or general-purpose `list`, `copy`, or `open` subcommands.
- A native menu-bar or SwiftUI application.
- Linux or Windows support.
- GitHub Enterprise Server support.
- Dashboard search or filtering.
- Automatic local-repository discovery.
- Multiple organizations in one configuration.
- Recently merged or closed PRs.
- Reviewer-specific status.
- Rich Markdown rendering of PR descriptions.
- Homebrew distribution.

Local-repository discovery remains a possible later feature. Version 1 will store repositories by canonical `owner/name`, but it will not add speculative discovery abstractions.

## Command Surface

PRQ intentionally has a minimal command surface:

```text
prq
prq setup
prq --help
prq --version
```

- `prq` opens the setup flow when no valid configuration exists; otherwise it opens the dashboard.
- `prq setup` reruns organization and repository selection, writes the new configuration only after successful completion, and then opens the dashboard.
- Unknown arguments return a concise usage error and a nonzero exit status.

The entry point uses a small argument dispatcher. Cobra or another general CLI framework is not required.

## First-run Setup

The setup flow uses terminal forms from the Charm ecosystem and proceeds as follows:

1. Locate `gh` on `PATH`.
2. verify authentication without changing it.
3. Query the current viewer and the organizations visible to that account.
4. Present a selectable single-organization list.
5. Query repositories visible within the selected organization.
6. Present a searchable multi-select repository list.
7. Require at least one repository.
8. Save the configuration atomically.
9. Best-effort remove any cache created for the previous repository selection.
10. Open the dashboard and perform an initial refresh.

PRQ never starts `gh auth login` itself. If `gh` is missing or unauthenticated, setup presents the appropriate install or login command and exits without writing configuration.

Once the configuration write succeeds, setup is committed even if obsolete
cache cleanup fails. Cache snapshots are bound to the normalized selection that
produced them, so an old cache cannot be displayed for the newly committed
selection and can be replaced by the dashboard's next successful refresh.

Only one organization can be selected in version 1. Repository choices are stored as canonical, case-preserving `owner/name` values and deduplicated case-insensitively.

## Dashboard Experience

### Startup and refresh

When the dashboard starts:

1. Load the last successful cache only when its bound selection matches the
   active normalized configuration.
2. Render cached PRs immediately when present.
3. Start a GitHub refresh in the background.
4. Replace the visible model and cache after a successful refresh.
5. Retain cached data and mark it stale after a failed refresh.

The header shows the number of visible PRs, whether a refresh is active, and the time of the last successful refresh. A manual `r` refresh uses the same path. Only one refresh may run at a time.

### Ordering

Every open PR authored by the viewer in a configured repository appears, including drafts.

The order is deterministic:

1. Non-draft PRs.
2. Draft PRs.
3. Within each group, least recently updated first.
4. Ties sort by repository name and then PR number.

This order is also used by the bulk-copy action.

### Compact rows

Each row shows:

- Repository name and PR number.
- PR title.
- Compact description.
- Draft marker when applicable.
- Aggregate review state.
- Aggregate check state.
- Time since the PR was last updated.

The compact description is the first meaningful nonblank line of the PR body. PRQ trims whitespace, removes a leading Markdown heading or list marker, collapses internal whitespace, and uses `No description` when no meaningful line exists. Rendering truncates fields to the available terminal width without mutating the underlying value.

### Detail pane

The selected PR appears in a persistent detail pane containing:

- Repository and PR number.
- Title.
- Open or draft state.
- Created and updated timestamps.
- Aggregate review state.
- Aggregate check state.
- URL.
- Full PR body.

The body is displayed as wrapped plain text. Version 1 preserves Markdown source characters rather than adding a Markdown renderer.

At widths of 100 columns or more, the list and detail pane appear side by side at approximately a 40/60 split. Below 100 columns, the list appears above the detail pane. Layout changes immediately when the terminal is resized.

### Keyboard controls

```text
↑ / ↓   Move the selected PR when the list is focused;
        scroll the PR body when the detail pane is focused
Tab     Move focus between the list and detail pane
o       Open the selected PR in the default browser
c       Copy the selected PR URL
C       Copy all displayed PRs as Slack-formatted bullets
r       Refresh from GitHub
?       Toggle full key help
q       Quit
```

`o`, `c`, `C`, `r`, `?`, and `q` are global. `Enter` has no dashboard action in version 1.

Successful copy and open actions produce a short status-line confirmation. Failures appear in the same status line and do not exit the dashboard.

### Clipboard format

Bulk copy produces one line per PR in dashboard order:

```text
• <https://github.com/acme/api/pull/184|Fix session refresh> — Refreshes expired sessions without forcing a new login.
```

The formatter uses the full PR title as the Slack link label and the untruncated compact description. It escapes Slack-sensitive `&`, `<`, and `>` characters in user-controlled text. A PR with no body uses `No description`.

## Status Mapping

Review and check information is intentionally aggregate.

Review states:

- `approved`
- `changes requested`
- `review required`
- `none`
- `unknown`

GitHub's `APPROVED`, `CHANGES_REQUESTED`, and `REVIEW_REQUIRED` review decisions map directly. An absent decision maps to `none`; an unfamiliar value maps to `unknown`.

Check states:

- `passing`
- `failing`
- `pending`
- `none`
- `unknown`

A successful status rollup maps to `passing`; failure or error maps to `failing`; pending or expected maps to `pending`; an absent rollup maps to `none`; an unfamiliar value maps to `unknown`.

## Architecture

The application is split into small packages with explicit responsibilities:

```text
cmd/prq
  argument dispatch and process exit behavior

internal/app
  pull-request model, refresh orchestration, sorting,
  compact descriptions, status mapping, and Slack formatting

internal/github
  read-only GitHub reader backed by fixed `gh api` queries

internal/store
  configuration and cache loading plus atomic persistence

internal/setup
  first-run and explicit setup flow

internal/tui
  Bubble Tea model, update loop, responsive rendering, and key bindings

internal/platform
  macOS clipboard and browser adapters
```

Presentation packages depend on application interfaces. The application package does not depend on Bubble Tea, terminal rendering, `gh`, or macOS process details.

The initial dependency set is:

- Go 1.26.
- Bubble Tea v2.
- Bubbles v2 for viewports and help components.
- Lip Gloss v2 for layout and styles.
- Huh v2 for setup forms.
- The Go standard library for argument dispatch, JSON, processes, files, and time.

Dependency versions are pinned in `go.mod` and `go.sum`. New dependencies require a concrete reduction in application code or risk.

## Data Model

The application model for a pull request contains:

- Stable GitHub node ID.
- Repository `owner/name`.
- PR number.
- Title.
- Full body.
- URL.
- Draft flag.
- Created and updated timestamps.
- Aggregate review state.
- Aggregate check state.

The compact description and relative age are derived values rather than persisted domain fields.

## GitHub Data Access

`gh` is a required runtime dependency and owns authentication. PRQ does not read, persist, or log the token.

The GitHub boundary exposes only:

```text
CurrentViewer
ListOrganizations
ListRepositories
ListOpenPullRequests
```

There is no generic GitHub request method in the application interface and no write-oriented method.

The concrete gateway:

- Uses `exec.CommandContext` with an argument array, never a shell command string.
- Pins authentication and API subprocesses to `github.com`, so unrelated
  enterprise-host authentication failures and `GH_HOST` cannot redirect PRQ.
- Uses fixed, package-private GraphQL `query` documents through `gh api graphql`.
- Passes every GraphQL variable with raw `-f` fields, preserving opaque cursor
  bytes and preventing `gh` typed-field file expansion or value conversion.
- Uses cursor pagination until all available pages are read.
- Reads organizations from `viewer.organizations` and repositories from the
  selected organization.
- Searches for open PRs authored by the viewer in the selected organization,
  then filters the result to the repository allowlist.
- Requests only the fields needed by the application.
- Treats partial GraphQL errors as refresh failures rather than silently presenting incomplete data.

GraphQL queries use HTTP POST as their transport, but their operation type is always `query`, never `mutation`. This distinction is documented because the read-only guarantee concerns GitHub behavior, not the transport verb.

The existing `gh` credential may possess write scopes. PRQ's guarantee is that its code never exercises those permissions. A separate lower-privilege token is not required in version 1.

## Read-only Enforcement

Read-only GitHub behavior is a product invariant:

- GitHub interfaces contain no write capability.
- GraphQL documents are fixed constants and begin with a named `query` operation.
- User-controlled text is passed only as GraphQL variables, never interpolated into operation text.
- Tests enumerate every GraphQL document and fail if one contains a mutation operation.
- Integration tests capture every `gh` invocation and assert that it is an approved authentication check or read-only API query.
- Contribution documentation states that GitHub write operations are outside project scope.

PRQ writes only to local config, local cache, and the system clipboard. Opening a URL delegates to the local browser and performs no GitHub API operation.

## Local Storage

PRQ uses Go's platform directory APIs:

- Configuration: `~/Library/Application Support/prq/config.json`
- Cache: `~/Library/Caches/prq/pull-requests.json`

The exact paths are derived with `os.UserConfigDir` and `os.UserCacheDir`; the paths above are their expected macOS locations.

Both files are private implementation formats rather than public compatibility contracts. Each contains an internal version number so incompatible data can be detected. Writes use a temporary file in the destination directory followed by an atomic rename. Files are created with user-only permissions where supported.

Configuration contains the selected organization and repository allowlist.
Cache contains the normalized selection that produced it, the last successful
normalized PR result, and its fetch time. A cache with a different selection is
ignored, including after a manual valid configuration change or an overlapping
setup/refresh process. Neither file contains a GitHub token.

## macOS Actions

The clipboard adapter starts `pbcopy` and writes content to its standard input. It does not construct a shell command.

The browser adapter runs `open` with the selected GitHub URL as a distinct argument. Version 1 accepts only parsed HTTPS URLs on `github.com` and rejects malformed or unexpected URLs.

Both adapters are interfaces at the TUI boundary and use fakes in tests.

## Error Handling

- Missing `gh`: show a concise error with installation guidance.
- Unauthenticated `gh`: show `gh auth login` guidance.
- Missing configuration: start setup automatically.
- Invalid configuration: report the exact path and direct the user to `prq setup`; do not overwrite it.
- Corrupt cache: ignore it, show a warning, and attempt a fresh refresh.
- Cached data plus refresh failure: preserve the cache, label it stale, show the last successful refresh time, and permit retry.
- No cache plus refresh failure: show a focused error screen with retry and quit actions.
- Unknown API enum: retain the PR and map the field to `unknown`.
- Clipboard or browser failure: retain the current screen and show a status-line error.
- Interruption: restore the terminal before exiting.

Errors returned by lower layers retain their cause and add operation context without exposing credentials or full API payloads.

## Testing Strategy

### Core tests

Table-driven unit tests cover:

- Draft grouping and deterministic ordering.
- Compact-description extraction.
- Review and check status mapping, including unknown values.
- Slack escaping and bulk-copy formatting.
- Repository filtering and deduplication.

### GitHub gateway tests

Saved response fixtures cover pagination, empty results, partial errors, missing optional fields, and unfamiliar enum values. Tests inspect all query documents and subprocess arguments to enforce read-only access.

### Storage tests

Temporary directories cover missing files, version mismatches, malformed JSON, permissions, atomic replacement, and preservation of a valid config after failed setup.

### TUI tests

Update-loop and render tests cover:

- Cached-first loading.
- Background refresh success and failure.
- Prevention of concurrent refreshes.
- Selection and detail scrolling.
- Focus changes.
- Every global key.
- Empty and error states.
- Wide side-by-side and narrow stacked layouts.
- Terminal resize events.

Rendering tests normalize color codes and volatile timestamps before snapshot comparison.

### Integration tests

Fake `gh`, `pbcopy`, and `open` executables capture argument arrays and standard input. Integration tests exercise first-run setup, normal startup, copy actions, open actions, and refresh failure without a GitHub account or external writes.

## Open-source Project Requirements

The repository will include:

- MIT `LICENSE`.
- `README.md` covering prerequisites, installation, setup, keyboard controls, privacy, read-only behavior, and screenshots.
- `CONTRIBUTING.md` with the local development and test workflow.
- `CODE_OF_CONDUCT.md`.
- `SECURITY.md` explaining the authentication and read-only threat model.
- Issue and pull-request templates.
- Representative, anonymized GitHub response fixtures.
- A macOS GitHub Actions workflow running `gofmt` checks, `go vet`, `go test`, and a release build.

No username, organization, repository, token, cache, or machine-specific path is committed. Contributors can run the complete automated test suite without GitHub authentication.

## Acceptance Criteria

Version 1 is complete when:

1. A user with authenticated `gh` can run `prq`, select one organization and multiple repositories, and reach the dashboard.
2. Subsequent launches render the last successful cache before starting a background refresh.
3. Every open PR authored by the viewer in the configured repositories, including drafts, is displayed.
4. Ordering, compact descriptions, aggregate review state, and aggregate check state match this specification.
5. Wide and narrow terminal layouts behave as specified.
6. Every documented key works, including selected copy, bulk Slack copy, browser open, refresh, focus, help, and quit.
7. Refresh failures preserve usable cached data and clearly identify staleness.
8. No application path can issue a GitHub mutation or other write operation.
9. Automated tests pass without external credentials or external writes.
10. The repository contains the approved open-source documentation and MIT license.

## References

- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [Huh](https://github.com/charmbracelet/huh)
- [GitHub CLI](https://cli.github.com/)
- [GitHub GraphQL pull request reference](https://docs.github.com/en/graphql/reference/pulls)
- [GitHub GraphQL pagination guide](https://docs.github.com/en/graphql/guides/using-pagination-in-the-graphql-api)
- [Go user config and cache directories](https://pkg.go.dev/os)
