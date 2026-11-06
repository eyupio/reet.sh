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
The optional [stdio bridge](https://github.com/eyupio/reet.sh/blob/main/mcp/README.md) is available for subprocess-only
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
| Context access | Preview and grant retained account counts or selected-session context, separately from shelf permissions. Requires an account with write access. |
| Worker access | Separately allow a management connection to queue instructions for one selected structured session and inspect delivery receipts. |
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

## Share retained reet context

Signed-in accounts with write access can choose **Context access** beside a
connection, or **Browser context access** for WebMCP. Choose account activity
counts or one owned structured session, select categories, preview, then enable
sharing. Existing connections start with no context grants. Read and management
shelf scopes do not grant transcript access. Viewers cannot grant or read this data.

`reet_list_context_access` returns only scopes explicitly granted to that client.
`reet_get_context` reads one exact `session_id`; an empty ID selects granted account
counts. Each client and browser tools have independent grants. Pausing/revoking a
client, pausing account access, disabling browser tools, changing the account role
or archiving the session prevents subsequent reads. The tools cannot grant access,
send instructions, answer approvals or run commands.

Status contains counts or canonical selected-session state, worker type and update
time. Event summaries contain roles/times and tool kinds/states without prose.
Optional message text includes bounded user/agent text, title and goal excerpt;
it can contain code or secrets despite best-effort credential masking. Reasoning,
approval prompts, raw tool arguments/output, files and diffs are always omitted.
At most 16 KiB / 20 projected events are returned, with source timestamps,
retention bounds and truncation/unavailable markers. Reads are limited to 60 per
account per minute. This retained snapshot is not live machine health, current
GitHub checks or the full transcript.

**Stop sharing** revokes one scope. Previously read data cannot be recalled.
Sharing persists until stopped; token replacement retains the same client's grants.
Provider permissions apply to Gaffer conversations separately; an external MCP
client controls its own provider and receives exactly the categories granted here.

The stdio bridge forwards these tools only with a scoped `reet_mcp_` credential.
Legacy general API-token and anonymous stdio clients keep their shelf behaviour
and cannot read context; connect through scoped MCP access instead.

## Send a worker instruction

Choose **Worker access** beside a management connection, or **Browser worker
access** for WebMCP. Select an owned structured session, enable its permission and
save. Each client and browser tools have separate grants, off by default; context
and shelf permissions grant no messaging authority. You can enable at most 100
session scopes per client.

`reet_list_worker_access` lists that client's granted sessions.
`reet_send_worker_message` takes an exact `session_id`, a stable 32-character
lowercase hexadecimal `request_id` and `text` of up to 8,000 UTF-8 bytes. It queues
for the next worker turn through the existing channel. The working model performs
the work under its existing permissions. `reet_get_worker_message` reads the same
request's delivery receipt. Responses contain delivery metadata, not instruction
text or a worker transcript. Matching retries read the original receipt; changing
the text for that ID is rejected. There is no automatic resubmission.

Queued, sending, accepted, blocked, uncertain and removed are distinct outcomes.
Accepted means channel acceptance, not task completion. An uncertain outcome needs
review in the working conversation. A waiting human approval or unavailable
channel blocks admission. Account, controller, client, session and grant checks
run again at delivery. Revoking permission removes unclaimed instructions; work
already claimed or accepted cannot be recalled. Receipts are bounded to 500 per
account; archive finished sessions or remove old Gaffer conversations for cleanup.

These three tools require management access and a signed-in account with write
access. They cannot grant themselves permission, interrupt a worker, answer an
approval or change its settings. The browser offers a separately confirmed
**Stop worker and send** action for a queued receipt. Browser WebMCP also uses its
existing write confirmation. Legacy API-token/anonymous stdio cannot send worker
messages; use a scoped MCP connection. No offline Gaffer follow goal is created by these tools. Goals are activated
separately in the signed-in session companion; client credentials cannot start
background supervision or grant themselves that authority.

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

The 35 tool definitions are shared with the optional stdio package. For accounts
with write access, read-only connections discover thirteen tools and management
connections thirty-five, including the two separately granted context reads and
three management-only worker messaging tools.
Viewer accounts retain eleven shelf-read tools.
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
to a dry run and need `confirm: true`. A delete refuses a report that still has skills
(`environment_has_skills`) or still holds private skill ownership receipts
(`environment_has_receipts`) unless `force: true`; a forced delete forgets the
receipts with the report, and pins always go with it. A merge has no `force`: it
refuses a report that holds private skill pins (`environment_has_pins`) or ownership
receipts (`environment_has_receipts`) with a 409, and its dry run reports the same
refusal. Pins, previews and receipts themselves are not available through MCP. Owner
settings can prune empty `reet-server` reports older than a chosen number of days.
Local skills can be added to personal lists only; shared or published lists return
`local_skill_not_allowed_in_list`. Ambiguous display names return
`ambiguous_skill_id` with the exact candidate IDs.

## Validation

Go tests exercise hosted MCP discovery/calls, credentials, scope, lifecycle,
OAuth grant binding and rotation, and browser API enforcement. The optional
TypeScript package tests real SDK interoperability with the Go HTTP endpoint
and scoped stdio forwarding. The web UI is type-checked with `npm run check` in
`web/`. Reproducible read-only evaluation tasks live in
[`mcp/evaluations`](https://github.com/eyupio/reet.sh/blob/main/mcp/evaluations/README.md).
