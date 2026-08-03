# Detail Pane Visual Hierarchy Design

Date: 2026-08-02
Status: Approved

## Summary

Make the selected pull request easier to scan by turning the detail pane's flat metadata list into a grouped summary. The pull-request title and repository context receive the strongest hierarchy, status and activity become compact secondary groups, and whitespace separates metadata from the description.

## Layout

The selected pull request renders as:

```text
DETAIL
Pull request title
owner/repository #123  •  OPEN

Review required  •  Checks failing
Created 2026-07-28 15:55 UTC  •  Updated 2026-07-28 15:56 UTC
https://github.com/owner/repository/pull/123

Pull-request description…
```

- The title is the first content line and uses the existing bold accent style.
- Repository, pull-request number, and uppercase `OPEN` or `DRAFT` state share a muted context line.
- Review and checks share one status line, with concise human-readable values.
- Created and updated timestamps share one muted activity line and retain their existing UTC format.
- The URL occupies one muted line.
- Blank lines separate identity, metadata, and the plain-text description.

## Status Styling

The palette remains restrained. Labels and ordinary status values use the normal foreground. `changes requested` review status and `failing` check status use the existing error color. Other review and check states do not introduce additional colors.

Styling is applied only after repository, title, URL, review, and check text has passed through the existing terminal-text sanitizer. Layout separators and ANSI sequences come only from trusted renderer code.

## Responsive Behavior

Every metadata line is truncated to the detail pane's inner width without mutating the stored pull-request data. The summary occupies eight fixed rows including the `DETAIL` heading and separator lines, one fewer than the current metadata block. The detail viewport therefore gains one row in both side-by-side and stacked layouts.

The description remains wrapped plain text inside the existing scrollable viewport. Markdown rendering is explicitly out of scope.

## Scope

This change is limited to detail-pane presentation and its viewport height calculation. It does not change GitHub queries, setup, domain data, caching, sorting, list rows, navigation, copying, opening URLs, or Markdown handling.

## Testing

A focused renderer test will verify title emphasis, muted context/activity/URL lines, compact status grouping, error styling, and blank-line grouping. Existing terminal-sanitization tests will continue to prove that untrusted control sequences cannot reach the terminal. Viewport sizing tests will reflect the one-row increase, and wide, narrow, help, stale-cache, and refresh-error golden views will be regenerated as needed. The complete Go test suite, formatting, vetting, build, and diff checks will run before completion.
