<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import PageHeader from './PageHeader.svelte'
  import { createToken } from './api'
  let { canWrite, accounts, compare = false }: { canWrite: boolean; accounts: boolean; compare?: boolean } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state('')
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
</script>
<PageHeader eyebrow="Your workspace" title="Machines." lede="See what every AI agent has installed, compare machines and review changes before applying them." />
<nav aria-label="Machines"><a class="btn quiet" href="/machines" aria-current={!compare ? 'page' : undefined}>Inventory & connection</a><a class="btn quiet" href="/machines?view=compare" aria-current={compare ? 'page' : undefined}>Compare & sync</a><a class="btn quiet" href="/profiles">Portable setups →</a></nav>
<EnvironmentSync compact {canWrite} {token} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
  {#snippet tokenSetup()}
    {#if accounts && canWrite}
      {#if !token}
        <form onsubmit={event => { event.preventDefault(); void connectToken() }}><label>Token name <input bind:value={name} maxlength="100" required autocomplete="off" /></label><button class="btn" disabled={busy}>{busy ? 'Creating…' : 'Create machine token'}</button></form>
      {:else}<p role="status">Token created. Copy the one-time token below before leaving this page. Revoke it under Account → API tokens.</p>{/if}
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {:else}<p>Use the token configured for this instance when the CLI asks. Instances without authentication do not require one.</p>{/if}
  {/snippet}
</EnvironmentSync>
<style>
 nav,form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} a {text-decoration:none} a[aria-current=page] {background:var(--tint);border-color:var(--accent)} label {display:grid;gap:6px;font-size:.85rem} p {font-size:.85rem;color:var(--muted)}
</style>
