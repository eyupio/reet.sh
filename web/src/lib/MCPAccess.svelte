<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import ContextSharing from './ContextSharing.svelte'
  import WorkerMessaging from './WorkerMessaging.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { onMount } from 'svelte'
  import CopyButton from './CopyButton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import { createMCPConnection, getMCPAccess, revokeMCPConnection, rotateMCPConnection, saveMCPAccess, updateMCPConnection, type MCPAccess, type MCPConnection } from './api'

  let { timeZone = '', dateFormat = 'locale' }: { timeZone?: string; dateFormat?: string } = $props()

  let workerClient = $state<{path:string;name:string}|null>(null)
  let contextClient = $state<{path:string;name:string}|null>(null)
  let access = $state<MCPAccess | null>(null)
  let error = $state('')
  let notice = $state('')
  let busy = $state(false)
  let name = $state('Desktop client')
  let scope = $state<'read' | 'write'>('read')
  let days = $state(30)
  let fresh = $state('')
  let freshName = $state('')
  let editing = $state<number | null>(null)
  let editName = $state('')
  let editScope = $state<'read' | 'write'>('read')
  let renewDays = $state(0)
  let confirmation = $state<{ title: string; body: string; label: string; run: () => Promise<void> } | null>(null)
  const browserSupported = typeof navigator !== 'undefined' && 'modelContext' in navigator
  const endpoint = $derived(access?.endpoint ?? `${location.origin}/mcp`)
  let connectionSearch = $state('')
  let connectionKind = $state<'all' | 'oauth' | 'bearer'>('all')

  function announce() { window.dispatchEvent(new Event('reet-mcp-changed')) }
  async function load() {
    try { access = await getMCPAccess(); if (!access.canWrite) { scope = 'read'; editScope = 'read' }; error = '' }
    catch (e) { error = e instanceof Error ? e.message : 'Could not load MCP access.' }
  }
  onMount(() => { void load() })

  async function act(action: () => Promise<void>, message: string) {
    busy = true; error = ''; notice = ''
    try { await action(); await load(); announce(); notice = message }
    catch (e) { error = e instanceof Error ? e.message : 'The change could not be saved.' }
    finally { busy = false }
  }
  function setting(value: Parameters<typeof saveMCPAccess>[0], message: string) {
    void act(async () => { access = await saveMCPAccess(value) }, message)
  }
  async function create(e: SubmitEvent) {
    e.preventDefault()
    await act(async () => {
      fresh = ''
      const result = await createMCPConnection(name.trim(), scope, days)
      fresh = result.token; freshName = result.connection.name
    }, 'Connection created. Copy its token below.')
  }
  function edit(connection: MCPConnection) {
    editing = connection.id; editName = connection.name; editScope = access?.canWrite ? connection.scope : 'read'; renewDays = 0
  }
  function status(connection: MCPConnection): string {
    if (connection.expiresAt <= Date.now()) return 'Expired'
    if (!connection.enabled) return 'Paused'
    return access?.enabled ? 'Active' : 'Access paused'
  }

  const searchedConnections = $derived.by(() => {
    const term = connectionSearch.trim().toLowerCase()
    return (access?.connections ?? []).filter((connection) => {
      if (!term) return true
      const kind = connection.kind === 'oauth' ? 'oauth' : 'bearer token'
      return term.split(/\s+/).every((word) => `${connection.name} ${kind}`.toLowerCase().includes(word))
    })
  })
  const connectionCounts = $derived({
    all: searchedConnections.length,
    oauth: searchedConnections.filter((connection) => connection.kind === 'oauth').length,
    bearer: searchedConnections.filter((connection) => connection.kind === 'bearer').length,
  })
  const visibleConnections = $derived(searchedConnections.filter((connection) => connectionKind === 'all' || connection.kind === connectionKind))
  const connectionCountLabel = $derived(connectionCount(visibleConnections.length, access?.connections.length ?? 0))

  function connectionCount(shown: number, total: number): string {
    if (total === 0) return 'No connections'
    if (shown === total) return `${total} ${total === 1 ? 'connection' : 'connections'}`
    return `${shown} of ${total} connections`
  }

  function clearConnectionFilters() {
    connectionSearch = ''
    connectionKind = 'all'
  }
  function when(value: number): string {
    if (!value) return 'Not used yet'
    const locale = ({ dmy: 'en-GB', mdy: 'en-US', ymd: 'sv-SE' } as Record<string, string>)[dateFormat]
    try { return new Date(value).toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'short', ...(timeZone ? { timeZone } : {}) }) }
    catch { return new Date(value).toLocaleString() }
  }
  async function saveEdit(e: SubmitEvent, connection: MCPConnection) {
    e.preventDefault()
    await act(async () => {
      await updateMCPConnection(connection.id, { name: editName.trim(), scope: editScope, ...(renewDays ? { expiresInDays: renewDays } : {}) })
      editing = null
    }, 'Connection updated.')
  }
  function rotate(connection: MCPConnection) {
    confirmation = {
      title: `Replace the token for ${connection.name}?`,
      body: 'Its current token will stop working immediately. Copy the replacement into your client to reconnect. Permissions and expiry stay the same.', label: 'Replace token',
      run: async () => { fresh = ''; const result = await rotateMCPConnection(connection.id); fresh = result.token; freshName = connection.name },
    }
  }
  function revoke(connection?: MCPConnection) {
    confirmation = {
      title: connection ? `Revoke ${connection.name}?` : 'Revoke all MCP connections?',
      body: connection ? 'This client will lose access. To connect it again, sign in with OAuth or create a new bearer token.' : 'Every OAuth and bearer-token connection will lose access. Browser WebMCP is controlled separately below.',
      label: connection ? 'Revoke connection' : 'Revoke all connections', run: async () => { await revokeMCPConnection(connection?.id); fresh = '' },
    }
  }
  function confirm() {
    const pending = confirmation; confirmation = null
    if (pending) void act(pending.run, 'Access updated.')
  }
</script>

<section class="mcp" aria-labelledby="mcp-title">
  <div class="heading">
    <div><p class="eyebrow">Your account · Connections</p><h2 id="mcp-title">MCP access</h2><p class="muted">Bring your library and explicitly shared reet context into the AI tools you use. You choose what each connection can read.</p></div>
    {#if access}<button type="button" class="btn" disabled={busy} onclick={() => setting({ enabled: !access!.enabled }, access!.enabled ? 'All MCP access is paused.' : 'MCP access is enabled.')}>{access.enabled ? 'Pause all access' : 'Enable access'}</button>{/if}
  </div>
  {#if error}<div class="notice bad" role="alert">{error} <button type="button" class="btn quiet" onclick={load} disabled={busy}>Retry</button></div>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}
  {#if !access && !error}<Skeleton label="Loading your connections…" shape="rows" count={3} />{/if}
  {#if access}
    <div class="statusline"><span class:active={access.enabled} class="dot" aria-hidden="true"></span><strong>{access.enabled ? 'MCP access is on' : 'MCP access is paused'}</strong><span class="muted">{access.enabled ? 'Permissions apply to your account only.' : 'OAuth, bearer tokens and browser tools cannot access your library.'}</span></div>
    <section class="connect-card" aria-labelledby="connect-mcp-title">
      <div class="cardtop"><span class="step">01</span><span class="badge">Recommended · OAuth</span></div>
      <h3 id="connect-mcp-title">Connect with your reet.sh account</h3>
      <p>Add this URL as a remote MCP connector in Claude or another compatible client. Choose Connect, sign in, and review the permissions. No token to copy.</p>
      <div class="endpoint"><code>{endpoint}</code><CopyButton text={endpoint} label="Copy MCP URL" /></div>
      <p class="hint">In clients that offer a transport, choose Streamable HTTP. Your first approval enables access for your account. Connections expire after 30 days and can be removed at any time.</p>
      {#if endpoint.startsWith('http:')}<p class="hint">Hosted AI services need a reachable HTTPS URL. A desktop client can connect directly to a localhost instance.</p>{/if}
    </section>

    <CollectionPanel label="Connected clients" count={connectionCountLabel}>
      {#snippet actions()}
        <button type="button" class="btn quiet" onclick={load} disabled={busy}>Refresh</button>
      {/snippet}
      {#snippet toolbar()}
        <div class="connection-toolbar">
          <label class="connection-search">Search connections
            <input type="search" bind:value={connectionSearch} placeholder="Name or kind" />
          </label>
          <div class="connection-scopes" role="group" aria-label="Show connections">
            <button type="button" class="scope" aria-pressed={connectionKind === 'all'} onclick={() => (connectionKind = 'all')}>All<small>{connectionCounts.all}</small></button>
            <button type="button" class="scope" aria-pressed={connectionKind === 'oauth'} onclick={() => (connectionKind = 'oauth')}>OAuth<small>{connectionCounts.oauth}</small></button>
            <button type="button" class="scope" aria-pressed={connectionKind === 'bearer'} onclick={() => (connectionKind = 'bearer')}>Bearer token<small>{connectionCounts.bearer}</small></button>
          </div>
        </div>
      {/snippet}

      {#if access.connections.length === 0}
        <div class="empty-state"><strong>Your first connection starts with a URL.</strong><p>Copy the MCP URL above into your client. It will appear here after you approve access.</p></div>
      {:else if visibleConnections.length === 0}
        <div class="state-block">
          <h3>No connection matches that search or kind.</h3>
          <p>Try another search, or show every connection.</p>
          <button type="button" class="btn" onclick={clearConnectionFilters}>Clear filters</button>
        </div>
      {:else}
        <ul class="connections">
          {#each visibleConnections as connection (connection.id)}
            <li>
              <div class="connection-head"><div><strong>{connection.name}</strong><span class="kind">{connection.kind === 'oauth' ? 'OAuth' : 'Bearer token'}</span></div><span class="state" class:live={status(connection) === 'Active'}>{status(connection)}</span></div>
              <p class="connection-meta">{connection.scope === 'write' && access.canWrite ? 'Read & manage' : 'Read only'} · Last used: {when(connection.lastUsedAt)}<br />Expires {when(connection.expiresAt)}</p>
              <div class="actions"><button type="button" class="btn quiet" disabled={busy} onclick={() => edit(connection)}>Manage</button><button type="button" class="btn quiet" disabled={busy} onclick={() => void act(async () => { await updateMCPConnection(connection.id, { enabled: !connection.enabled }) }, connection.enabled ? 'Connection paused.' : 'Connection resumed.')}>{connection.enabled ? 'Pause' : 'Resume'}</button>{#if connection.kind === 'bearer'}<button type="button" class="btn quiet" disabled={busy} onclick={() => rotate(connection)}>Replace token</button>{/if}<button type="button" class="btn quiet danger" disabled={busy} onclick={() => revoke(connection)}>Revoke</button></div>
              {#if access.canWrite}<button class="btn quiet context-access" type="button" disabled={busy} onclick={()=>contextClient={path:`/api/v2/mcp/connections/${connection.id}/context`,name:connection.name}}>Context access</button><button class="btn quiet" type="button" disabled={busy} onclick={()=>workerClient={path:`/api/v2/mcp/connections/${connection.id}/worker`,name:connection.name}}>Worker access</button>{/if}
              {#if editing === connection.id}
                <form class="edit-form" onsubmit={(e) => saveEdit(e, connection)}>
                  <label>Name<input bind:value={editName} required maxlength="100" autocomplete="off" /></label>
                  <label>Permission<select aria-label="Connection permission" bind:value={editScope}><option value="read">Read only</option>{#if access.canWrite && (connection.kind === 'bearer' || connection.scope === 'write')}<option value="write">Read & manage</option>{/if}</select></label>
                  {#if connection.kind === 'bearer'}<label>Expiry<select bind:value={renewDays}><option value={0}>Keep current expiry</option><option value={30}>30 days from today</option><option value={90}>90 days from today</option></select></label>{/if}
                  {#if connection.kind === 'oauth'}<p class="hint wide">To request broader OAuth permissions or renew an expired sign-in, reconnect from your AI client.</p>{/if}
                  <div class="actions wide"><button type="submit" class="btn primary" disabled={busy}>Save changes</button><button type="button" class="btn quiet" onclick={() => editing = null}>Cancel</button></div>
                </form>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </CollectionPanel>
    {#if access.connections.length > 0}
      <button type="button" class="btn quiet danger revoke-all" disabled={busy} onclick={() => revoke()}>Revoke all connections</button>
    {/if}

    <section class="panel" aria-labelledby="browser-mcp-title">
      <div class="rowhead"><div><h3 id="browser-mcp-title">Browser WebMCP</h3><p class="muted">Let a browser assistant work with the reet.sh page you have open.</p></div><span class="badge subtle">{browserSupported ? 'Available in this browser' : 'Requires a compatible browser'}</span></div>
      <p class="hint">Uses your signed-in session. Read-only by default; when management is allowed, each change asks for your confirmation on this page. Tools are removed when you sign out.</p>
      <div class="browser-options"><label class="check"><input type="checkbox" checked={access.browserEnabled} disabled={busy || !access.enabled} onchange={(e) => setting({ browserEnabled: e.currentTarget.checked }, e.currentTarget.checked ? 'Browser tools enabled in compatible browsers.' : 'Browser tools disabled.')} />Enable browser tools</label><label>Permission<select aria-label="Browser permission" value={access.browserScope} disabled={busy || !access.enabled || !access.browserEnabled} onchange={(e) => setting({ browserScope: e.currentTarget.value as 'read' | 'write' }, 'Browser permission updated.')}><option value="read">Read only</option>{#if access.canWrite}<option value="write">Read & manage, with confirmation</option>{/if}</select></label></div>
      {#if access.canWrite}<button class="btn quiet" type="button" disabled={busy} onclick={()=>contextClient={path:'/api/v2/mcp/browser/context',name:'Browser WebMCP'}}>Browser context access</button><button class="btn quiet" type="button" disabled={busy} onclick={()=>workerClient={path:'/api/v2/mcp/browser/worker',name:'Browser WebMCP'}}>Browser worker access</button><p class="hint">Session context needs separate explicit grants. Enabling library tools does not share it.</p>{/if}
      {#if !browserSupported}<p class="hint">Your browser does not expose WebMCP yet. The OAuth and bearer-token connections above work independently.</p>{/if}
    </section>

    <details class="panel manual"><summary>Manual connection with a bearer token</summary><p class="muted">For clients that ask for an Authorization header instead of OAuth. Give each client its own token.</p>
      <form onsubmit={create}><div class="fields"><label>Connection name<input bind:value={name} required maxlength="100" autocomplete="off" /></label><label>Permission<select aria-label="Manual connection permission" bind:value={scope}><option value="read">Read only</option>{#if access.canWrite}<option value="write">Read & manage</option>{/if}</select></label><label>Expires in<select bind:value={days}><option value={7}>7 days</option><option value={30}>30 days</option><option value={90}>90 days</option><option value={365}>1 year</option></select></label></div><div class="actions"><button type="submit" class="btn primary" disabled={busy || !access.enabled} aria-busy={busy}>{busy ? 'Saving…' : 'Create bearer token'}</button></div></form>
      {#if !access.enabled}<p class="hint">Enable access above to create a manual connection.</p>{/if}
    </details>
    {#if fresh}<section class="fresh" aria-label="New bearer token" role="status"><div class="rowhead"><h3>{freshName} · token ready</h3><button type="button" class="btn quiet" onclick={() => fresh = ''}>Dismiss</button></div><p>Copy this token now. It is shown only once.</p><div class="endpoint"><code>{fresh}</code><CopyButton text={fresh} label="Copy token" /></div><p class="hint">Use <code>Authorization: Bearer YOUR_TOKEN</code> with the MCP URL above. Replacing this token invalidates the previous one immediately.</p></section>{/if}
  {/if}
</section>
{#if confirmation}<ConfirmDialog title={confirmation.title} body={confirmation.body} confirmLabel={confirmation.label} danger onconfirm={confirm} oncancel={() => confirmation = null} />{/if}

{#if contextClient}<WorkspaceDialog title={`Context shared with ${contextClient.name}`} onclose={()=>contextClient=null}><ContextSharing {timeZone} {dateFormat} path={contextClient.path}/></WorkspaceDialog>{/if}

<style>
  .mcp { margin-top: 28px; }
  .heading,.rowhead,.connection-head { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; flex-wrap:wrap; }
  .connection-head > div { min-width:0; overflow-wrap:anywhere; }
  h2 { margin:4px 0 8px; font:500 2rem/1.15 var(--serif); letter-spacing:-.035em; }
  h3 { margin:0; font:600 1.05rem/1.35 var(--sans); }
  p { line-height:1.6; }
  .muted,.hint,.connection-meta { color:var(--muted); }
  .muted { margin:5px 0 0; }
  .hint { font-size:0.875rem; margin:12px 0 0; }
  .statusline { display:flex; align-items:center; gap:9px; flex-wrap:wrap; font-size:0.875rem; padding:20px 0; }
  .dot { width:8px; height:8px; border-radius:50%; background:var(--muted); }
  .dot.active { background:var(--good); box-shadow:0 0 0 4px color-mix(in srgb,var(--good) 10%,transparent); }
  .connect-card,.panel,.fresh { padding:clamp(16px,3vw,24px); border:1px solid var(--line); border-radius:12px; background:var(--card); margin-bottom:16px; }
  .connect-card { background:var(--card); border:1px solid var(--line); box-shadow:inset 3px 0 0 var(--accent); }
  .connect-card h3 { margin-top:16px; font:500 1.65rem/1.25 var(--serif); letter-spacing:-.025em; }
  .connect-card > p { max-width:75ch; }
  .cardtop { display:flex; justify-content:space-between; align-items:center; gap:12px; }
  .step { font:500 0.875rem/1 var(--sans); color:var(--accent-text); font-variant-numeric:tabular-nums; }
  .badge,.kind,.state { display:inline-block; font-size:0.875rem; border-radius:6px; padding:3px 9px; border:1px solid var(--line); color:var(--muted); }
  .badge { color:var(--accent-text); border-color:color-mix(in srgb,var(--accent) 25%,var(--line)); }
  .subtle { color:var(--muted); border-color:var(--line); }
  .kind { margin-left:8px; }
  .state.live { color:var(--good); }
  .endpoint { display:flex; align-items:center; justify-content:space-between; gap:12px; background:var(--bg); border:1px solid var(--line); border-radius:8px; padding:12px 16px; margin-top:16px; }
  .endpoint code { overflow-wrap:anywhere; min-width:0; font-size:0.875rem; }
  .empty-state { padding:24px 0 8px; }
  .empty-state p { color:var(--muted); margin-bottom:0; }
  .connection-toolbar { display:flex; align-items:flex-end; flex-wrap:wrap; gap:12px 16px; }
  .connection-search { flex:1 1 240px; display:grid; gap:6px; color:var(--muted); font-size:0.875rem; }
  .connection-search input { width:100%; }
  .connection-scopes { display:flex; align-items:center; flex-wrap:wrap; gap:6px; }
  .scope { min-height:36px; padding:5px 10px; border:1px solid var(--line); border-radius:8px; background:transparent; color:var(--muted); font:inherit; font-size:0.875rem; cursor:pointer; }
  .scope:hover { border-color:var(--muted); color:var(--ink); }
  .scope[aria-pressed='true'] { background:var(--tint); border-color:var(--accent); color:var(--ink); font-weight:600; }
  .scope small { margin-left:5px; font-size:0.875rem; font-variant-numeric:tabular-nums; }
  .connections { list-style:none; margin:0; padding:0; }
  .connections li { padding:16px 0; border-top:1px solid var(--line); }
  .connections li:first-child { border-top:0; padding-top:4px; }
  .state-block { padding:24px 0 16px; max-width:58ch; }
  .state-block h3 { margin:0; font:500 1.25rem/1.2 var(--serif); letter-spacing:-.02em; }
  .state-block p { margin:6px 0 12px; color:var(--muted); }
  .connection-meta { font-size:0.875rem; margin:8px 0 12px; line-height:1.7; font-variant-numeric:tabular-nums; }
  .actions { display:flex; gap:8px; flex-wrap:wrap; }
  .danger { color:var(--bad); }
  .revoke-all { margin-top:12px; }
  form { margin-top:18px; }
  .fields,.edit-form { display:grid; grid-template-columns:2fr 1fr 1fr; gap:14px; }
  .edit-form { padding:18px; margin-top:16px; border:1px solid var(--line); border-radius:8px; background:var(--bg); }
  label { display:grid; gap:7px; color:var(--muted); font-size:0.875rem; }
  .wide { grid-column:1/-1; }
  form > .actions { margin-top:16px; }
  .browser-options { display:flex; gap:24px; align-items:center; flex-wrap:wrap; margin-top:20px; }
  .check { display:flex; gap:9px; align-items:center; color:var(--ink); }
  .check input { width:17px; height:17px; accent-color:var(--accent); }
  summary { font-weight:600; cursor:pointer; }
  .fresh { border-color:var(--accent); background:var(--tint); }
  .notice { border:1px solid var(--line); padding:12px; border-radius:8px; }
  .bad { border-color:var(--bad); }
  .good { border-color:var(--good); }
  @media(max-width:640px) { .fields,.edit-form { grid-template-columns:1fr; } .endpoint { align-items:flex-start; } .heading > button { width:100%; } .browser-options > label { width:100%; } }
  @media (pointer:coarse) { .scope, .connection-search input { min-height:44px; } }
</style>

{#if workerClient}<WorkspaceDialog title={`Worker access for ${workerClient.name}`} onclose={()=>workerClient=null}><WorkerMessaging path={workerClient.path} {timeZone} {dateFormat}/></WorkspaceDialog>{/if}
