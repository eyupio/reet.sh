<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import PageHeader from './PageHeader.svelte'
  import ProjectsBar from './ProjectsBar.svelte'
  import { createToken } from './api'
  import TerminalReadiness from './TerminalReadiness.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import TerminalCommandReference from './terminal/TerminalCommandReference.svelte'
  import { machineSetUp, terminalControllerStatus, terminalMachines, type TerminalControllerStatus, type TerminalMachine } from './terminal/api'
  import { describeError } from './contentReview'
  import { onMount, type Component } from 'svelte'
  let { canWrite, accounts, monitoring=false, compare = false, userId=0, onsignedout }: { canWrite: boolean; accounts: boolean; monitoring?:boolean; compare?: boolean;userId?:number; onsignedout?: (e: unknown) => void } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state('')
  // A session address (?session=, ?agent= or view=terminal) opens the workspace. A bare /sessions reaches this page only for
  // someone without Sessions (a viewer, a token or an open instance), who gets the Machines page itself.
  const query = new URLSearchParams(location.search)
  const terminals = location.pathname === '/sessions' && (query.has('session') || query.has('agent')) || query.get('view') === 'terminal'
  // svelte-ignore state_referenced_locally
  const focus = compare ? '' : (new URLSearchParams(location.search).get('machine') ?? '')
  let TerminalView = $state<Component<{userId?:number}>>()
  onMount(()=>{let disposed=false;if(terminals&&accounts&&canWrite)void import('./terminal/TerminalWorkspace.svelte').then(module=>{if(!disposed)TerminalView=module.default}).catch(()=>{if(!disposed)error='Could not load the terminal workspace. Refresh this page to retry.'});return()=>{disposed=true}})
  // The first-terminal checklist (from Control, which is folded into Home and Machines, R4): what stands between a writer and a
  // working terminal, read once when the page opens. Hidden once a machine has done its part on a server that allows terminals.
  // Machines only: Sync keeps its own single way forward (empty-states.e2e), so the checklist is not read there.
  let readinessState = $state<{ controller: TerminalControllerStatus | null; machines: TerminalMachine[] } | null>(null), helpOpen = $state(false)
  const setUpCount = $derived(readinessState ? readinessState.machines.filter(machineSetUp).length : 0)
  const showSetup = $derived(!!readinessState && (readinessState.machines.length === 0 || (readinessState.controller !== null && !readinessState.controller.available) || setUpCount === 0))
  onMount(() => {
    if (terminals || compare || !accounts || !canWrite) return
    let gone = false
    const signal = AbortSignal.timeout(10000)
    void Promise.all([terminalMachines(signal), terminalControllerStatus(signal).catch(() => null)]).then(([machines, controller]) => { if (!gone) readinessState = { controller, machines } }).catch(() => { /* the checklist is left out when it cannot be read */ })
    return () => { gone = true }
  })
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = describeError(e) }
    finally { busy = false }
  }
</script>
{#if !terminals && accounts && canWrite}<ProjectsBar {canWrite} {accounts} {onsignedout} />{/if}
{#if !terminals}<PageHeader eyebrow="Machines" title={compare ? "Sync, on your terms." : focus ? `${focus} details` : "Your machines, connected."} lede={compare ? "Move skills between reet.sh and your machines. Choose a source, select destinations, then review the changes." : focus ? "Inspect this machine's agent scopes, health and saved skill sets, or continue in its terminal." : "Connect a machine, inspect its skills and agents, and open a terminal to put them to work."} />{/if}
{#if !terminals}<nav aria-label="Machines"><a class="btn quiet" href="/machines" aria-current={!compare&&!terminals ? 'page' : undefined}>Inventory & connection</a><a class="btn quiet" href="/sync" aria-current={compare ? 'page' : undefined}>Sync</a>{#if accounts&&canWrite}<a class="btn quiet" href="/sessions">Sessions</a>{/if}<a class="btn quiet" href="/profiles">Setups →</a></nav>{/if}
{#if showSetup && readinessState}<TerminalReadiness controller={readinessState.controller} machines={readinessState.machines.length} setUp={setUpCount} ready={readinessState.machines.filter(m => m.ready).length} {canWrite} onhelp={() => (helpOpen = true)} />{/if}
{#if helpOpen}<WorkspaceDialog title="Terminal connection setup" onclose={() => (helpOpen = false)}><p class="help-copy">Allow terminal access on each machine, then open it from your workspace. A browser disconnect leaves its shells running.</p><TerminalCommandReference /><p class="help-copy">On the machine, install tmux 3.2 or newer, then run:</p><pre>reet agent terminal setup</pre><a class="btn" href="/machines#connect-machine">Connect or finish setting up a machine →</a></WorkspaceDialog>{/if}
{#if terminals && accounts && canWrite}{#if TerminalView}<TerminalView {userId} />{:else}<p role="status">{error||'Loading terminal workspace…'}</p>{/if}{:else}
<EnvironmentSync compact {canWrite} {accounts} monitorAccess={monitoring} terminalAccess={accounts&&canWrite} {token} {onsignedout} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
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
 .help-copy{font-size:.82rem;line-height:1.7;color:var(--muted);margin:0} pre{padding:12px;border:1px solid var(--line);border-radius:6px;background:var(--bg);font:.78rem/1.7 var(--mono);overflow:auto}
 nav,form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} a {text-decoration:none} a[aria-current=page] {background:var(--tint);border-color:var(--accent)} label {display:grid;gap:6px;font-size:.85rem} p {font-size:.85rem;color:var(--muted)}
</style>
