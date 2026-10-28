<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import PageHeader from './PageHeader.svelte'
  import ProjectsBar from './ProjectsBar.svelte'
  import { createToken } from './api'
  import { describeError } from './contentReview'
  import { onMount, type Component } from 'svelte'
  let { canWrite, accounts, monitoring=false, compare = false, userId=0, onsignedout }: { canWrite: boolean; accounts: boolean; monitoring?:boolean; compare?: boolean;userId?:number; onsignedout?: (e: unknown) => void } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state('')
  // A session address (?session=, ?agent= or view=terminal) opens the workspace. A bare /sessions reaches this page only for
  // someone without Sessions (a viewer, a token or an open instance), who gets the Machines page itself.
  const query = new URLSearchParams(location.search)
  // Without Sessions (not a writer in accounts mode) a session address shows the Machines page with its header too.
  // svelte-ignore state_referenced_locally
  const terminals = (location.pathname === '/sessions' && (query.has('session') || query.has('agent')) || query.get('view') === 'terminal') && accounts && canWrite
  // svelte-ignore state_referenced_locally
  const focus = compare ? '' : (new URLSearchParams(location.search).get('machine') ?? '')
  let TerminalView = $state<Component<{userId?:number}>>()
  onMount(()=>{let disposed=false;if(terminals&&accounts&&canWrite)void import('./terminal/TerminalWorkspace.svelte').then(module=>{if(!disposed)TerminalView=module.default}).catch(()=>{if(!disposed)error='Could not load the terminal workspace. Refresh this page to retry.'});return()=>{disposed=true}})
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = describeError(e) }
    finally { busy = false }
  }
</script>
{#if !terminals}<PageHeader eyebrow={focus ? 'Machine' : 'Machines'} title={compare ? "Sync, on your terms." : focus || "Your machines, connected."} lede={compare ? "Move skills between reet.sh and your machines. Choose a source, select destinations, then review the changes." : focus ? "Inspect this machine's agent scopes, health and saved skill sets, or continue in its terminal." : "Connect a machine, inspect its skills and agents, and open a terminal to put them to work."} />{/if}
{#if !terminals}<nav aria-label="Machines"><a class="btn quiet" href="/machines" aria-current={!compare&&!terminals ? 'page' : undefined}>Inventory &amp; connection</a><a class="btn quiet" href="/sync" aria-current={compare ? 'page' : undefined}>Sync</a></nav>{/if}
{#if terminals && accounts && canWrite}{#if TerminalView}<TerminalView {userId} />{:else}<p role="status">{error||'Loading terminal workspace…'}</p>{/if}{:else}
<EnvironmentSync {canWrite} {accounts} monitorAccess={monitoring} terminalAccess={accounts&&canWrite} {token} {onsignedout} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
  {#snippet tokenSetup()}
    {#if accounts && canWrite}
      {#if !token}
        <form onsubmit={event => { event.preventDefault(); void connectToken() }}><label>Token name <input bind:value={name} maxlength="100" required autocomplete="off" /></label><button class="btn" disabled={busy}>{busy ? 'Creating…' : 'Create machine token'}</button></form>
      {:else}<p role="status">Token created. Copy the one-time token below before leaving this page. Revoke it under Account → API tokens.</p>{/if}
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {:else}<p>Use the token configured for this instance when the CLI asks. Instances without authentication do not require one.</p>{/if}
  {/snippet}
</EnvironmentSync>
{/if}
{#if !terminals && accounts && canWrite}
  <details class="projects-drawer">
    <summary>Projects</summary>
    <ProjectsBar {canWrite} {accounts} {onsignedout} />
  </details>
{/if}
<style>
 nav,form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} a {text-decoration:none} a[aria-current=page] {background:var(--tint);border-color:var(--accent)} label {display:grid;gap:6px;font-size:.85rem} p {font-size:.85rem;color:var(--muted)}
 .projects-drawer {margin:26px 0 0;border-top:1px solid var(--line);padding-top:14px}
 .projects-drawer > summary {cursor:pointer;color:var(--muted);font-size:.82rem;font-weight:650;min-height:32px}
 @media (max-width:560px) { nav {display:grid;grid-template-columns:repeat(2,minmax(0,1fr));align-items:stretch} nav a {justify-content:center} }
</style>
