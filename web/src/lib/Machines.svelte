<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import PageHeader from './PageHeader.svelte'
  import { createToken } from './api'
  import { onMount, type Component } from 'svelte'
  let { canWrite, accounts, monitoring=false, compare = false }: { canWrite: boolean; accounts: boolean; monitoring?:boolean; compare?: boolean } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state('')
  const terminals = location.pathname === '/terminals' || new URLSearchParams(location.search).get('view') === 'terminal'
  let TerminalView = $state<Component>()
  onMount(()=>{let disposed=false;if(terminals&&accounts&&canWrite)void import('./terminal/TerminalWorkspace.svelte').then(module=>{if(!disposed)TerminalView=module.default}).catch(()=>{if(!disposed)error='Could not load the terminal workspace. Refresh this page to retry.'});return()=>{disposed=true}})
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
</script>
{#if !terminals}<PageHeader eyebrow="Your control centre" title={compare ? "Sync, on your terms." : "Your machines, connected."} lede={compare ? "Move skills between reet.sh and your machines. Choose a source, select destinations, then review the changes." : "Connect a machine, inspect its skills and agents, and open a terminal to put them to work."} />{/if}
<nav aria-label="Machines"><a class="btn quiet" href="/machines" aria-current={!compare&&!terminals ? 'page' : undefined}>Inventory & connection</a><a class="btn quiet" href="/sync" aria-current={compare ? 'page' : undefined}>Skills & sync</a>{#if accounts&&canWrite}<a class="btn quiet" href="/terminals" aria-current={terminals?'page':undefined}>Terminals</a>{/if}<a class="btn quiet" href="/profiles">Portable setups →</a></nav>
{#if terminals && accounts && canWrite}{#if TerminalView}<TerminalView />{:else}<p role="status">{error||'Loading terminal workspace…'}</p>{/if}{:else}
<EnvironmentSync compact {canWrite} monitorAccess={monitoring} terminalAccess={accounts&&canWrite} {token} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
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
<style>
 nav,form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} a {text-decoration:none} a[aria-current=page] {background:var(--tint);border-color:var(--accent)} label {display:grid;gap:6px;font-size:.85rem} p {font-size:.85rem;color:var(--muted)}
</style>
