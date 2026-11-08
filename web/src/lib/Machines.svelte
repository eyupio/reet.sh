<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import WorkspaceSkeleton from './terminal/WorkspaceSkeleton.svelte'
  import PageHeader from './PageHeader.svelte'
  import type { SectionItem } from './SectionNav.svelte'
  import ProjectsBar from './ProjectsBar.svelte'
  import FleetSoftware from './FleetSoftware.svelte'
  import { createToken } from './api'
  import { describeError } from './contentReview'
  import { onMount, type Component } from 'svelte'
  let { canWrite, accounts, monitoring=false, compare = false, userId=0, timeZone = '', dateFormat = 'locale', onsignedout, ontitle }: { canWrite: boolean; accounts: boolean; monitoring?:boolean; compare?: boolean;userId?:number; timeZone?: string; dateFormat?: string; onsignedout?: (e: unknown) => void; ontitle?: (title: string) => void } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state(''), softwareOpen = $state(false)
  // A session address (?session=, ?agent= or view=terminal) opens the workspace. A bare /sessions reaches this page only for
  // someone without Sessions (a viewer, a token or an open instance), who gets the Machines page itself.
  const query = new URLSearchParams(location.search)
  // Without Sessions (not a writer in accounts mode) a session address shows the Machines page with its header too.
  // svelte-ignore state_referenced_locally
  const terminals = (location.pathname === '/sessions' && query.get('view') !== 'list' || query.get('view') === 'terminal') && accounts && canWrite
  // svelte-ignore state_referenced_locally
  const focus = compare ? '' : (new URLSearchParams(location.search).get('machine') ?? '')
  const workspaceContext = query.get('theatre') !== '1' && !!(query.get('session') || query.get('agent'))
  let TerminalView = $state<Component<{userId?:number; ontitle?: (title: string) => void}>>()
  onMount(()=>{let disposed=false;if(terminals&&accounts&&canWrite)void import('./terminal/TerminalWorkspace.svelte').then(module=>{if(!disposed)TerminalView=module.default}).catch(()=>{if(!disposed)error='Could not load the terminal workspace. Refresh this page to retry.'});return()=>{disposed=true}})
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = describeError(e) }
    finally { busy = false }
  }
</script>
{#if !terminals}
  {@const sections: SectionItem[] = [
    { label: 'Inventory & connection', href: '/machines', current: !compare },
    { label: 'Sync', href: '/sync', current: compare },
    ...(accounts && canWrite ? [{ label: 'Software', action: () => (softwareOpen = true) }] : [])
  ]}
  <PageHeader group="Machines" groupHref={!compare && focus ? '/machines' : undefined} title={compare ? "Sync" : focus || "Machines"} lede={compare ? "One shared skill library, kept up to date on every machine." : focus ? "Inspect this machine's install locations, health and sync plans, or continue in its terminal." : "Connect your machines. Reet keeps your skills in sync and your agents ready to work."} {sections} sectionsLabel="Machines" />
{/if}
{#if terminals && accounts && canWrite}{#if TerminalView}<TerminalView {userId} {ontitle} />{:else if error}<p class="notice bad" role="alert">{error}</p>{:else}
  <WorkspaceSkeleton label="Loading the terminal workspace" context={workspaceContext} />
  {/if}{:else}
<EnvironmentSync {canWrite} {accounts} {timeZone} {dateFormat} monitorAccess={monitoring} terminalAccess={accounts&&canWrite} {token} {onsignedout} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
  {#snippet tokenSetup()}
    {#if accounts && canWrite}
      {#if !token}
        <form onsubmit={event => { event.preventDefault(); void connectToken() }}><label>Token name <input bind:value={name} maxlength="100" required autocomplete="off" /></label><button class="btn" disabled={busy} aria-busy={busy}>{busy ? 'Creating…' : 'Create machine token'}</button></form>
      {:else}<p role="status">Token created. Copy the one-time token below before leaving this page. Revoke it under Account → API tokens.</p>{/if}
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {:else}<p>Use the token configured for this instance when the CLI asks. Instances without authentication do not require one.</p>{/if}
  {/snippet}
</EnvironmentSync>
{/if}
{#if softwareOpen}<FleetSoftware onclose={() => (softwareOpen = false)} />{/if}
{#if !terminals && !compare && accounts && canWrite}
  <details class="projects-drawer">
    <summary>Projects</summary>
    <ProjectsBar {canWrite} {accounts} {onsignedout} />
  </details>
{/if}
<style>
 form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} label {display:grid;gap:6px;font-size:0.875rem} p {font-size:0.875rem;color:var(--muted)}
 .projects-drawer {margin:26px 0 0;border-top:1px solid var(--line);padding-top:14px}
 .projects-drawer > summary {cursor:pointer;color:var(--muted);font-size:0.875rem;font-weight:600;min-height:32px}
</style>
