# Security Policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's security-advisory feature.
Do not open a public issue. Include the affected version, impact, reproduction
steps, and any relevant logs after removing usernames, organization and
repository names, tokens, and local paths.

Reports of any possible GitHub write path are security issues and should follow
this private process.

## Authentication and data access

PRQ shells out to the user's authenticated `gh`. It does not read, persist, or
log GitHub tokens.

All GraphQL operations are fixed read-only queries. The GitHub command runner
accepts only an authentication status check and approved fixed query documents;
it rejects mutation operations, unrecognized documents, flags, and variables.
User-controlled text is passed only as GraphQL variables.

The user's existing `gh` credential may have write scopes, but PRQ never
exercises those permissions. GraphQL queries use HTTP POST as transport; their
operation type remains `query`, never `mutation`.

PRQ writes only to local configuration, local cache, and the system clipboard.
It may pass a validated `https://github.com` URL to the local browser.
