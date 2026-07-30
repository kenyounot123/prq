## Summary

Describe the user-visible result and why the change is needed.

## Verification

List the commands and manual checks you ran.

## Read-only and privacy review

- [ ] I added or updated tests for every behavior change.
- [ ] I ran `gofmt -w .`, `go vet ./...`, `go test ./...`, and
      `go build ./cmd/prq`.
- [ ] I explicitly reviewed the read-only query guard if this changes GitHub
      interfaces, queries, command arguments, or tests.
- [ ] This change does not introduce a GitHub write path.
- [ ] Logs, fixtures, and screenshots contain no personal organizations,
      repositories, usernames, tokens, or local paths.

## Additional notes

Call out changes to GitHub access, local storage, subprocess execution,
clipboard content, or URL handling.
