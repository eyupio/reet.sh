# Connect an AI client to your shelf with MCP

reet.sh includes a hosted MCP server in its Go binary. A client such as Claude can read your skill shelf, search it, manage notes and tags, and work with the lists you can already access, without a separate MCP service or a Node process.

This guide connects one client with OAuth. It also covers manual bearer tokens and the proxy details that matter when the controller is behind HTTPS.

## What you need

- A reet.sh instance in `REET_AUTH=accounts` mode. OAuth and managed MCP credentials are not available in token or no-auth mode.
- An account with a role that can read the shelf. Viewers can use read-only access; writers and admins can grant management access.
- A client that supports remote MCP servers over Streamable HTTP and OAuth. Local desktop clients can use a localhost controller; hosted clients need a reachable HTTPS origin.

## 1. Open MCP access

Sign in to reet.sh and open **Account → MCP access**.

The page shows the MCP URL for this instance, usually `https://reet.example.com/mcp`, and the access controls for your account. Keep **Enable access** on for the connections you want.

If the page is not there, check that the instance is in accounts mode. MCP is a per-account feature, not an instance-wide switch.

## 2. Add the remote connector

In your AI client, add a remote MCP server with the URL from the page. The exact labels vary by client, but the choices are:

- **Authentication:** OAuth, or automatic if the client offers it.
- **Transport:** Streamable HTTP.
- **URL:** the MCP URL shown on the page.

Do not paste a password or an API token into the URL. OAuth is the intended path for a hosted client.

## 3. Approve the connection

Your client opens reet.sh in a browser. Sign in, check the client name and callback host, choose **Read only** or **Read & manage**, and approve.

You return to the client without copying a secret. The connection appears under **Account → MCP access** with its type, state, permission, expiry and last use.

- **Read only** can search shelf metadata and personal notes, tags, inventories, saved sync plans, lists you can access, recent activity and ignored entries.
- **Read & manage** can also add, edit and remove shelf skills and notes, edit tags, create personal lists, add list skills and unignore entries.

Broader permissions require a new approval in the client. A connection cannot read another person's private shelf, manage credentials, passwords, administration, backups or list sharing, or execute and install skills.

## 4. Check it works

Ask the client to list your skills, find a duplicate, or search for a tag. A useful first read is the compact shelf summary rather than every field.

If the client shows an empty shelf, check the account and permission on the MCP access page before changing the URL. If it shows an authentication error, remove the connection in the client and approve it again.

## Manual bearer tokens

For a client that supports bearer headers instead of OAuth, enable access and expand **Manual connection with a bearer token**. Give it a recognisable name, choose permissions and an expiry, and create the token.

Use the same MCP URL and send the secret in the header:

```http
Authorization: Bearer reet_mcp_…
```

Manual credentials expire after 7, 30, 90 or 365 days. Replace and revoke them independently. An ordinary API token is not an MCP credential and will not work at `/mcp`.

## Pause, replace and revoke

- **Pause / resume a client** blocks or restores one connection without changing its credential.
- **Manage** renames a connection, reduces permissions, or changes a manual token's expiry.
- **Replace token** invalidates a manual token immediately and shows one replacement.
- **Revoke** permanently blocks that connection and its OAuth refresh family.
- **Revoke all connections** blocks every OAuth and bearer connection. Browser access has its own switch.

Disabling an account, changing a viewer role or revoking a token administratively also applies on later requests. An in-flight operation may already have completed; check the shelf before retrying a write.

## Behind a reverse proxy

Forward `/mcp`, `/oauth/`, `/.well-known/`, `/api/` and the web UI to the same instance. Set `REET_PUBLIC_URL=https://reet.example.com` to pin the canonical origin; it must not contain a path.

Configure the proxy to preserve the public host and replace forwarded headers. Do not strip the bearer authorization header. A local desktop client can use a localhost instance without a public origin.

## Troubleshooting

- **401 or a refused connection.** The client is not sending a valid OAuth or bearer credential, or the connection was paused or revoked. Check the MCP access page, then reconnect.
- **404 on `/mcp`.** The instance is not in accounts mode, the path is not forwarded to reet, or a proxy is rewriting it.
- **The browser approval opens the wrong host.** Check `REET_PUBLIC_URL` and the proxy's forwarded host headers.
- **A write is refused.** The connection has read-only permission, the account cannot write, or the list does not allow it.

## Limits

- The MCP server reads and manages the shelf. It does not install or execute skills.
- It cannot manage credentials, passwords, administration, backups or list sharing.
- A browser session alone cannot authenticate the hosted MCP endpoint.
- OAuth consent is per client. Reducing permissions or revoking access does not delete the skills already on a machine.

For the complete access table and client notes, read [Connect your shelf to an AI client](mcp.md).
