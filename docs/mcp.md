# Connect your shelf to an AI client

reet.sh includes a hosted MCP server in its Go binary. In accounts mode, open
**Account → MCP access**, copy the MCP URL, and add it as a remote connector in
Claude or another MCP client. Choose **OAuth** when the client asks how to sign
in, or leave its authentication choice on automatic. If it asks for a transport,
choose **Streamable HTTP**.

Your client opens reet.sh in a browser. Sign in, check the client name and callback
host, choose read or management permissions, and approve. You return to the client
without copying a secret. The connection appears in Account → MCP access.
OAuth approval enables MCP for your account. You can instead cancel without
granting access. Broader permissions require a new approval in your client.

Remote services need to reach the URL: use a public or otherwise reachable HTTPS
origin for hosted clients. A local desktop client can use a localhost instance.
For a reverse proxy, forward `/mcp`, `/oauth/`, `/.well-known/`, `/api/` and the
web UI to the same instance. Set `REET_PUBLIC_URL=https://reet.example.com` to
pin the canonical origin; it has no path component. When empty, reet.sh follows
the request host and HTTPS proxy header. Configure the proxy to preserve the
public host and replace forwarded headers. Do not strip bearer authorization.

No Node process or separate MCP service is needed for the hosted connection.
The optional [stdio bridge](../mcp/README.md) is available for subprocess-only
clients. OAuth and managed MCP credentials require `REET_AUTH=accounts`.

## Your access controls

Every account, including viewers, has its own **MCP access** section:

| Control | Effect |
| --- | --- |
| Enable / pause all access | Enables or blocks OAuth, bearer credentials and browser tools for your account. |
| Read only | Search shelf metadata and personal notes, tags, inventories, saved sync plans, accessible lists, recent activity and ignored entries. |
| Read & manage | Also add, edit or remove shelf skills and notes, edit tags, create personal lists, add list skills and unignore entries. |
| Pause / resume a client | Blocks or restores that connection without changing its credential. |
| Manage | Rename a connection, reduce its permissions, or change a manual token's expiry. Manual bearer permissions can also be increased within the account's role; reconnect OAuth clients to renew authorization. |
| Replace token | Invalidates a manual bearer's old secret immediately and shows one replacement. OAuth clients reconnect through their own sign-in flow. |
| Revoke | Permanently blocks that connection and its OAuth refresh family. |
| Revoke all connections | Blocks all OAuth and bearer connections; browser access has its own switch. |

The page shows connection type, state, permission, expiry and last use. Secrets
are shown once and stored only as hashes. Expired or revoked credentials cannot
access the server. Account disabling, viewer role changes and administrative
token revocation also apply immediately on subsequent requests. An in-flight
operation may already have completed; check the shelf before retrying a write.

Account and list permissions apply to every operation. A connection cannot read
another person's private shelf. It cannot manage credentials, passwords,
administration, backups or list sharing, and it cannot execute or install skills.
Browser sessions alone cannot authenticate the hosted MCP endpoint. MCP bearer
credentials cannot be used as general REST API tokens or to create new credentials.
Use a signed-in browser to maintain access.

## Manual bearer tokens

For a client that supports bearer headers instead of OAuth, enable access and
expand **Manual connection with a bearer token**. Give the client a recognizable
name, choose permissions and expiry, and create the token. Copy the one-time
secret into the client's bearer-token field, using the same MCP URL:

```http
Authorization: Bearer reet_mcp_…
```

Do not put credentials in a URL or use an ordinary API token at `/mcp`. Manual
credentials expire after the selected 7, 30, 90 or 365 days. Replace and revoke
them independently so losing one client does not disrupt the others.

## Browser WebMCP

Enable **Browser WebMCP** to expose the same tools through
`navigator.modelContext` while a reet.sh page is open and signed in. It is off by
default and uses read-only permissions until you choose **Read & manage, with
confirmation**. Every browser tool that changes data opens an on-page review
dialog; cancelling makes no change. Signing out unregisters the tools. Changing
access settings refreshes the registry; the server rechecks permissions on every
call, including after changes made in another tab.

WebMCP browser support is still evolving. The page detects availability and
explains when the current browser does not expose the API. No polyfill is used,
and OAuth and bearer connections continue to work independently. Browser WebMCP
does not start a remote server or require a bearer token.

## OAuth implementation and limits

The default flow uses OAuth authorization code with mandatory PKCE S256, public
client registration, exact callback matching, resource binding, a signed-in
consent screen and one-use five-minute codes. The server advertises RFC 9728
protected-resource metadata and RFC 8414 authorization-server discovery.
Only `mcp:read` and `mcp:write` scopes are supported.

Access tokens last one hour. Refresh tokens rotate on use; replay of a used
refresh token revokes the connection family. Authorization lasts 30 days unless
the user reduces its expiry or revokes it. Scope changes cannot expand an
already consented OAuth grant. Clients with other OAuth grant types, private
client secrets or client-ID metadata documents must use the supported public
dynamic-registration flow or manual bearer authentication.

The 28 tool definitions are shared with the optional stdio package. Read-only
connections discover eleven tools; management connections discover twenty-eight.
Collections support bounded pages and filters. Responses are capped at 25,000
characters; use smaller pages or more specific filters for large shelves.
`reet_list_skills` has `compact: true` (or `fields`) for large shelves; it
returns only id, name, source, sourceType, stage, liked, bookmarked, tags and
environment_count. A page shortened to fit the limit says so in `truncated` and
`truncated_reason`; follow `next_offset` for the rest.
If a change succeeds but its response exceeds the limit, the error explicitly
says the change succeeded. Read the saved state before making another change;
do not retry the write just to retrieve its response.
Recent activity pages with `before`/`after` event-ID cursors and can be filtered
by `action`, `source` and skill. Changes made through MCP are recorded with
`source: "mcp"` and the client name in the event detail.
Inventories include scan time and completeness: an old or incomplete scan does
not prove what is installed today.

Environment reports can be deleted or merged by ID. Deletes and merges default
to a dry run and need `confirm: true`; a non-empty report is refused unless
`force: true`. Owner settings can prune empty `reet-server` reports older than a
chosen number of days. Local skills can be added to personal lists only; shared
or published lists return `local_skill_not_allowed_in_list`. Ambiguous display
names return `ambiguous_skill_id` with the exact candidate IDs.

## Validation

Go tests exercise hosted MCP discovery/calls, credentials, scope, lifecycle,
OAuth grant binding and rotation, and browser API enforcement. The optional
TypeScript package tests real SDK interoperability with the Go HTTP endpoint
and scoped stdio forwarding. The web UI is type-checked with `npm run check` in
`web/`. Reproducible read-only evaluation tasks live in
[`mcp/evaluations`](../mcp/evaluations/README.md).
