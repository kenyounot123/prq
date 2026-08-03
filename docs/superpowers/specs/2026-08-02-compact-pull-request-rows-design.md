# Compact Pull Request Rows Design

Date: 2026-08-02
Status: Approved

## Summary

Make the pull-request list easier to scan by reducing each entry to two content lines and giving the title stronger visual priority. This change affects only the list pane; the detail pane continues to show the full description and status metadata.

## Row Layout

Each pull request renders as:

```text
› Pull request title
  owner/repository #123  DRAFT

```

- The first line contains the selection marker and pull-request title.
- The selected title and marker use the existing bold accent style.
- The second line contains the repository and pull-request number in the existing muted style.
- Draft pull requests retain `DRAFT` on the metadata line.
- One blank line separates adjacent entries.
- Both content lines retain the existing terminal-text sanitization and width truncation.

The list no longer renders the compact description, aggregate review state, aggregate check state, or relative update age.

## Scope

The detail pane remains unchanged and continues to show repository, number, title, state, timestamps, review state, check state, URL, and full body. Pull-request data fetching, caching, sorting, copying, navigation, and selection behavior also remain unchanged.

## Rendering and Scrolling

The list renderer treats an entry as three rows: two content lines followed by one separator line. Its visible-row calculation uses this height so moving the selection keeps the selected pull request in view. The final visible entry may include a trailing blank line; pane sizing clips or pads it consistently with existing rendering.

## Testing

Renderer tests will establish that list entries include the title and muted repository metadata while omitting description, review, checks, and age. Existing detail assertions will verify that description and status metadata remain available there. Wide, narrow, help, stale-cache, and refresh-error golden views will be updated, and the complete Go test suite plus formatting, vetting, and build checks will run before completion.
