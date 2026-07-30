# Contributing to PRQ

Thanks for helping improve PRQ. Please read the
[Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Development environment

PRQ v1 is developed and tested on macOS with Go 1.26. Runtime use requires an
authenticated GitHub CLI, but the automated test suite uses local fakes and
does not require GitHub credentials.

Fork and clone the repository, then install dependencies:

```bash
go mod download
```

## Make a change

Keep changes focused and follow the existing package boundaries. Add or update
tests for every behavior change. New dependencies should have a concrete
benefit that reduces application code or risk.

GitHub access is intentionally read-only. Any change to `internal/github`, its
interfaces, GraphQL documents, command arguments, or related tests requires an
explicit review of the read-only query guard. PRQ must never gain a GitHub write
path. GraphQL variables must remain separate from fixed query documents.

## Check your work

Run these commands before opening a pull request:

```bash
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/prq
```

CI also runs the test suite with the race detector:

```bash
go test -race ./...
```

## Open a pull request

Describe the user-visible result and the tests you ran. Call out any change
that touches GitHub access, local storage, subprocess execution, clipboard
content, or URL handling. Keep logs and screenshots free of personal
organizations, repositories, usernames, tokens, and local paths.
