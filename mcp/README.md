# reet.sh MCP tools and optional stdio bridge

The default connection is the hosted **Streamable HTTP + OAuth** endpoint in the
Go binary. Users copy their MCP URL from **Account → MCP access**, sign in through
their client, and manage connections in the web UI. Manual scoped bearer tokens
and browser WebMCP are also supported. Start with [MCP setup](../docs/mcp.md).

This package defines the shared tool contract and provides an optional local
stdio bridge for clients that need a subprocess. With a scoped MCP token, it
forwards to the hosted endpoint, preserving permissions and lifecycle controls.
It does not open SQLite or run shell/install commands.

Requires Node.js 22 or newer and a running reet.sh instance. Go 1.26 or newer is
needed **only for the integration tests**, which compile the real API fixture.

## Optional stdio bridge

From this directory:

```sh
npm ci
npm run build
node dist/index.js --help
```

Add this entry to your MCP client's server configuration, replacing the absolute
path, URL and token. The client launches the process and manages its lifetime:

```json
{
  "mcpServers": {
    "reet": {
      "command": "node",
      "args": ["/absolute/path/to/reet.sh/mcp/dist/index.js"],
      "env": {
        "REET_URL": "http://127.0.0.1:8080",
        "REET_API_TOKEN": "your-MCP-bearer-token"
      }
    }
  }
}
```

In accounts mode, enable access and create a bearer token under **Account → MCP
access**. These `reet_mcp_…` credentials use the hosted server and its permissions.
For compatibility, a general personal API token from **Account → API tokens**
still uses the original REST bridge; that credential has the account's general
API privileges and is separate from MCP connection controls. The bootstrap
`REET_TOKEN` cannot access shelf APIs in accounts mode. In legacy token mode,
use the instance token; for `REET_AUTH=none`, omit the token.

`REET_URL` defaults to `http://127.0.0.1:8080`. A path prefix such as
`https://example.com/reet` is supported. Configure the final URL: redirects are
rejected to keep the bearer token bound to one destination. Use HTTPS across
untrusted networks. Store the client configuration privately; tokens never
belong in prompts, source control or URL query parameters.

This optional process uses stdio; its stdout contains MCP messages only. The
hosted `/mcp` endpoint is built into the Go binary and container image. Rebuild
this package after pulling bridge changes.

## Tools

All names start with `reet_`. Tool descriptions and schemas include field limits,
defaults, mutation hints and return information. Successful calls return
`structuredContent: {data: ...}` plus text. `response_format` selects `json`
(default) or a Markdown JSON code block containing the same data.

| Tools | Behavior |
| --- | --- |
| `list_skills`, `get_skill` | Search the personal shelf by text, stage, flag, tags, environment or machine; read a skill's flags, notes and metadata. |
| `add_skill`, `update_skill`, `remove_skill` | Capture references and edit personal flags/notes; remove entries, optionally ignoring future scanner imports. |
| `list_tags`, `tag_skill` | Discover tag IDs; add names or remove IDs on a skill. |
| `list_environments`, `get_environment` | Read scan summaries and paginated inventories, including scan time and completeness. |
| `list_sync_targets` | Inspect saved desired/missing/extra plans without applying changes. |
| `list_lists`, `get_list` | Find accessible lists and read skills, list notes and permissions. |
| `create_list`, `add_list_skill` | Create a personal list or add a reference with a shared list note. |
| `list_events` | Read the most recent activity, optionally for one skill. |
| `list_ignored`, `unignore_skill` | Inspect scanner exclusions and allow future imports. |

The account role and list permissions are enforced by reet.sh for every request.
Use the web UI for sharing/audience changes, administration, backups and tokens.
Use `reet scan` and `reet sync` locally for filesystem discovery and confirmed
installs/removals. Scan completeness and timestamps matter: an incomplete or old
inventory cannot prove that a skill is absent today.

Stages derive from flags: `use` means used, `try` means bookmarked but not used,
and `shelf` means neither. `liked` is independent. Updates preserve unspecified
fields; personal notes and shared list notes are separate. Removing a shelf entry
discards its personal notes and flags. Adding an existing reference can replace
explicitly supplied fields and removes its ignored status.

## Examples

Try these requests in your MCP client:

- “Find the skills I saved to test, ordered by popularity, and show their notes.”
- “Find my Backend tag, then show liked skills with that tag on my laptop.”
- “Read my laptop and desktop inventories and the saved sync plan; explain missing skills and any incomplete scans.”
- “Add `anthropics/skills/mcp-builder` to my shelf with a note to review its transport examples.”
- “Move that skill to To test while keeping its existing notes and liked flag.”
- “Create a personal list called Agent tooling and add mcp-builder with a list note about evaluation coverage.”

## Limits and errors

Collection tools accept `limit` (1–100, default 20) and `offset`. Follow
`next_offset` until `has_more` is false. Pages also include `total_count` and
`count`. `list_events` supports only the latest 100 events and has no historical
cursor. Shelf `counts` describe all flags in the account; the page's
`total_count` describes the filtered results.

Most upstream collection APIs do not support pagination. The bridge fetches
their filtered response, bounds it to **8 MiB**, and paginates locally. It is
therefore intended for personal and team shelves rather than unbounded exports.
Responses are capped at **25,000 characters**. Pages shrink to fit and still
advance the cursor. Individual oversized records fail with a clear message;
use a narrower filter or shorten notes in the web UI. Nested sync-plan inventories
are returned intact and may hit this cap; inspect environments separately in
that case. Concurrent shelf changes can move records between offset pages.

Requests time out after 30 seconds. Errors explain how to resolve authentication,
permission, missing-resource, rate-limit, connectivity and JSON response problems.
Raw server error bodies and credential values are not returned. Check server
logs for validation details hidden by a generic HTTP 400 response. A failed write
request may have reached the server before a connection failure: inspect the
current state before retrying a create operation.

## Verification and evaluations

After changing tool definitions, run `npm run export-tools` and commit the
generated `internal/api/mcp_tools.json`. The Go server embeds that contract;
`npm test` checks that it matches SDK discovery. Hosted HTTP, browser WebMCP and
stdio share the same schemas. The Go dispatcher validates arguments and calls
existing API handlers with the authenticated user's identity.

```sh
npm test
```

Tests use the SDK client over actual stdio and a disposable real Go API backed
by temporary SQLite. They cover discovery, strict inputs, filters, pagination,
shelf and list mutations, scan metadata, auth errors, redirects, timeout and size
limits. No running instance or credentials are needed. The fixture never fetches
external metadata. Go may download module dependencies on the first run.
`REET_GO` can select an explicit Go executable.

The frozen fixture also supports the ten read-only tasks in
[`evaluations/shelf.xml`](evaluations/shelf.xml). Their answers refer only to that
fixture, not to a user's changing shelf. See
[`evaluations/README.md`](evaluations/README.md) for the evaluation workflow.

For manual inspection after building:

```sh
npx @modelcontextprotocol/inspector node dist/index.js
```

Set `REET_URL` and `REET_API_TOKEN` in the inspector's server environment. The
protocol tests run without an LLM API key; model-driven evaluation is a separate,
optional check and has not been run as part of these tests.
