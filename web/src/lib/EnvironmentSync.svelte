<script lang="ts">
  import { onMount, tick } from 'svelte'
  import type { Snippet } from 'svelte'
  import { getBuildInfo, getSyncState, mergeMachines, forgetMachine, listSetupProfiles, type SetupProfileSummary, type SyncState, type SyncEnvironment } from './api'
  import CopyButton from './CopyButton.svelte'
  import SyncControl from './SyncControl.svelte'
  import MachineSetup from './MachineSetup.svelte'
  import MachineFleet from './MachineFleet.svelte'
  import AddMachineDialog from './AddMachineDialog.svelte'
  import { sameHost } from './contentReview'
  import type { JoinContext } from './machineJoin'
  import { agentGroups, inventoryMachines } from './machines'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite, accounts=false, terminalAccess=false, monitorAccess=false, token = '', tokenSetup, view = 'machines', startConnect = false, compact = false, onsignedout }: { canWrite: boolean; accounts?: boolean; terminalAccess?:boolean;monitorAccess?:boolean;token?: string; tokenSetup?: Snippet; view?: 'machines' | 'compare'; startConnect?: boolean; compact?: boolean; onsignedout?: (e: unknown) => void } = $props()

  let syncState = $state<SyncState>({ environments: [], targets: [] })
  let machines = $derived(inventoryMachines(syncState.machines ?? []))
  let serverMachines = $derived((syncState.machines ?? []).filter((machine) => agentGroups(machine).length === 0))
  let reportedMachines = $derived(syncState.machines ?? [])
  let profiles = $state<SetupProfileSummary[]>([])
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let lastRefresh = $state(0)
  let serverBuild = $state<{ version: string; commit: string } | null>(null)
  let loadGeneration = 0
  // Whether the machines have been read once (the Add a machine dialog says nothing about a name before that), and how the
  // last read ended, so the dialog's own refresh can tell a failed read from a skipped one.
  let loaded = $state(false)
  let loadFailure: unknown = undefined
  let refreshing = false
  let loadedOnce = false
  let fleetSince = $state(0)
  let maintenanceFrom = $state('')
  let maintenanceTo = $state('')
  let machineChange = $state<{ from: string; to?: string } | null>(null)
  // svelte-ignore state_referenced_locally
  let setupOpen = $state(startConnect)
  let setupHost = $state('')
  // The Add a machine dialog, with what opened it, and the machine it just brought in (highlighted for a few seconds).
  let joining = $state<{ context: JoinContext; opener: HTMLElement | null } | null>(null)
  let justAdded = $state('')
  let justAddedTimer: ReturnType<typeof setTimeout> | undefined
  const initialSetupMode = new URLSearchParams(location.search).get('setup') === 'scratch' ? 'scratch' : 'connect'
  const site = location.origin
  const focusHost = new URLSearchParams(location.search).get('machine') ?? ''
  const focusedMachine = $derived(focusHost ? machines.find(machine => machine.host.toLowerCase() === focusHost.toLowerCase()) : undefined)
  const visibleMachines = $derived(focusedMachine ? [focusedMachine] : machines)
  // Offered to a signed-in account that can write, on the Machines view only (never to a viewer or in a legacy sign-in mode).
  const canAdd = $derived(accounts && canWrite && view === 'machines')
  let now = $state(Date.now())

  function quote(value: string): string { return "'" + value.replaceAll("'", "'\\''") + "'" }
  async function openSetup() {
    setupOpen = true
    await tick()
    document.getElementById('connect-machine')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
  function updateState(next: SyncState) {
    ++loadGeneration
    syncState = next
    loaded = true
    if (inventoryMachines(next.machines ?? []).length === 0) setupOpen = true
  }
  function fresh(env: SyncEnvironment): boolean { return env.scannedAt > now - (env.agent === 'reet-server' ? 5 * 60000 : 14 * 86400000) }
  function when(at: number): string { return at ? new Date(at).toLocaleString() : 'Not reported yet' }
  function heartbeatStatus(env: SyncEnvironment): string {
    if (env.agent === 'reet-server') return fresh(env) ? 'Server reporting' : 'Server not seen recently'
    return fresh(env) ? 'Scan reported' : 'Scan is out of date'
  }
  function refreshedGroups(machine: import('./api').Machine): number {
    return fleetSince ? agentGroups(machine).filter((group) => group.complete && group.scannedAt >= fleetSince).length : 0
  }
  function refreshCommand(host: string): string {
    return `reet scan --server ${quote(site)} --env ${quote(host)} --prune --reconcile`
  }
  function previewMachine(event: SubmitEvent) {
    event.preventDefault()
    if (busy || !maintenanceFrom || !maintenanceTo.trim()) return
    machineChange = { from: maintenanceFrom, to: maintenanceTo.trim() }
  }
  async function changeMachine() {
    if (!machineChange || busy) return
    const change = machineChange
    machineChange = null
    busy = true
    error = ''
    notice = ''
    try {
      syncState = change.to ? await mergeMachines(change.from, change.to) : await forgetMachine(change.from)
      notice = change.to ? `Reet, reports now belong to ${change.to}. Old names remain aliases; future scans update this machine.` : `Forgot ${change.from}. Installed files and shelf entries are unchanged; a future scan can report it again.`
      maintenanceFrom = ''
      maintenanceTo = ''
      await tick()
      document.getElementById('machine-list-title')?.focus()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
  async function load(background = false) {
    if (refreshing || busy) return
    refreshing = true
    const generation = ++loadGeneration
    if (!background) loading = true
    error = ''
    try {
      const next = await getSyncState()
      if (generation !== loadGeneration) return
      syncState = next
      loaded = true
      loadFailure = undefined
      lastRefresh = Date.now()
      if (!loadedOnce && inventoryMachines(next.machines ?? []).length === 0) setupOpen = true
      loadedOnce = true
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      loadFailure = e
    } finally {
      loading = false
      refreshing = false
    }
  }
  // The dialog's read: the same read as the page's, but a failed one is thrown so the dialog can back off or hand over a lost
  // sign-in. A read the page already has under way (or a change in progress) is not repeated; it counts as how that read ended.
  async function refreshForJoin() {
    await load(true)
    if (loadFailure !== undefined) throw loadFailure
  }
  function openJoin(opener: HTMLElement | null) {
    joining = { context: {}, opener }
  }
  // Closing the dialog hands focus to the entry that opened it; if that entry has moved (the machines were read while it was
  // open), to wherever the entry is now.
  function joinClosed() {
    const opener = joining?.opener
    joining = null
    void tick().then(() => {
      if (opener?.isConnected) return
      document.querySelector<HTMLElement>('[data-add-machine]')?.focus()
    })
  }
  // The new machine is reported in and ready. From the Machines page that means showing it; the Private skills landing is the
  // other branch (context.skill) and is added where that dialog opens this one.
  function joinReady(r: { host: string; context: JoinContext }) {
    if (r.context.skill !== undefined) {
      // Not yet wired to a landing: fall back to showing the machine.
    }
    void showMachine(r.host)
  }
  async function showMachine(host: string) {
    const machine = machines.find(m => sameHost(m.host, host))
    if (!machine) return
    justAdded = machine.host
    clearTimeout(justAddedTimer)
    justAddedTimer = setTimeout(() => { justAdded = '' }, 8000)
    await tick()
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    document.getElementById(`machine-${machine.host}`)?.closest('article')?.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'center' })
  }
  function joinStepByStep(name: string) {
    setupHost = name
    void openSetup()
  }
  async function loadProfiles() {
    try {
      profiles = (await listSetupProfiles()).profiles ?? []
    } catch {
      /* the setup card falls back to a link to Setups */
    }
  }
  onMount(() => {
    void getBuildInfo().then((build) => { serverBuild = build }).catch(() => {})
    void load()
    void loadProfiles()
    const timer = setInterval(() => {
      now = Date.now()
      if (document.visibilityState === 'visible') void load(true)
    }, 5000)
    return () => { clearInterval(timer); clearTimeout(justAddedTimer) }
  })
</script>

<section class="sync" aria-labelledby="sync-title">
  <header class="section-head">
    <div>
      <p class="eyebrow">Machines &amp; sync</p>
      <h2 id="sync-title">{view === 'machines' ? 'Inventory' : 'Compare & sync'}</h2>
    </div>
    <div class="head-actions">
      {#if canAdd && !visibleMachines.length}<button class="btn" type="button" data-add-machine onclick={(event) => openJoin(event.currentTarget)}>Add a machine</button>{/if}
      <button class="btn quiet" type="button" onclick={() => load()} disabled={loading || busy}>{loading ? 'Refreshing…' : 'Refresh status'}</button>
    </div>
  </header>
  {#if !compact}<p class="lede">{reportedMachines.length ? 'Your machines are one unit here: agent scopes live inside each machine, and every command is ready to run on that machine.' : 'Bring a machine in, scan what its agents have installed, and sync the skills and setups you want.'}</p>{/if}
  {#if focusedMachine}<p class="hint machine-focus"><a href="/machines">All machines</a> / <strong>{focusedMachine.host}</strong></p>{:else if focusHost && !loading}<p class="hint">No machine named {focusHost} is connected. <a href="/machines">Show all machines</a>.</p>{/if}
  {#if lastRefresh && reportedMachines.length}<p class="hint">Status refreshed {when(lastRefresh)} · updates automatically while this page is open.</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice" role="status">{notice}</p>{/if}

  {#if view === 'compare'}
    {#if reportedMachines.length}
      <SyncControl snapshot={syncState} {canWrite} {view} onupdate={updateState} />
    {:else}
      <MachineSetup {terminalAccess} {site} {machines} {profiles} {token} {tokenSetup} {serverBuild} snapshot={syncState} bind:open={setupOpen} initialMode={initialSetupMode} onrefresh={() => load()} />
    {/if}
  {:else}
    <MachineSetup {terminalAccess} {site} {machines} {profiles} {token} {tokenSetup} {serverBuild} snapshot={syncState} bind:open={setupOpen} bind:host={setupHost} initialMode={initialSetupMode} onrefresh={() => load()} />
    {#if visibleMachines.length}
      <MachineFleet machines={visibleMachines} snapshot={syncState} {site} {canWrite} {accounts} {terminalAccess} {monitorAccess} onadd={openSetup} onjoin={canAdd ? openJoin : undefined} highlight={justAdded} onupdate={updateState} onerror={(e) => { error = e instanceof Error ? e.message : String(e) }} {onsignedout} onnotice={(message) => { notice = message }} />
    {/if}
  {/if}

  {#if view === 'machines' && canWrite && machines.length}
    <details class="advanced-fleet">
      <summary>Advanced machine maintenance</summary>
      <p class="hint">Duplicate reports, fleet refreshes and server-only heartbeats. Most people will not need these.</p>
      <details class="maintenance">
        <summary>Manage machine names &amp; duplicate reports</summary>
        <p class="hint">Hostname and --env names can be reports from one machine. Choose the report to move, then keep an existing name to merge duplicates or enter a new name to rename it. Only combine reports you know are the same machine.</p>
        <form class="machine-form" onsubmit={previewMachine}>
          <label>Report to move<select aria-label="Report to move" bind:value={maintenanceFrom} required><option value="">Choose a machine</option>{#each reportedMachines as machine (machine.host)}<option value={machine.host}>{machine.host}</option>{/each}</select></label>
          <label>Preferred machine name<input aria-label="Preferred machine name" list="machine-names" bind:value={maintenanceTo} maxlength="200" required placeholder="e.g. PROD_Snicket" /></label>
          <datalist id="machine-names">{#each reportedMachines as machine (machine.host)}<option value={machine.host}></option>{/each}</datalist>
          <button class="btn" type="submit" disabled={busy || !maintenanceFrom || !maintenanceTo.trim()}>Review change</button>
        </form>
      </details>
      <details class="maintenance" ontoggle={(event) => { if (event.currentTarget.open && !fleetSince) fleetSince = Date.now() }}>
        <summary>Full fleet inventory refresh</summary>
        <p class="hint">Run one command per physical machine, using the name you want to keep. If both a hostname and a friendly name appear, choose the friendly-name command only, or merge them above first. These commands never install or remove skills.</p>
        <button class="btn quiet" onclick={() => (fleetSince = Date.now())}>Start a new refresh check</button>
        {#each machines as machine (machine.host)}
          <div class="fleet-command">
            <strong>{machine.host}</strong>
            <p class="hint" role="status">{refreshedGroups(machine)} of {agentGroups(machine).length} agent groups refreshed since this check started.</p>
            <div class="command"><pre>{refreshCommand(machine.host)}</pre><CopyButton text={refreshCommand(machine.host)} label={`Copy refresh for ${machine.host}`} /></div>
          </div>
        {/each}
        <p class="hint">For enrolled machines, use Scan all from Control or Sync. For other machines, run the commands above, then refresh status.</p>
      </details>
      {#if serverMachines.length}
        <details class="maintenance">
          <summary>Server-only reports ({serverMachines.length})</summary>
          <p class="hint">These machines have reported a heartbeat but no agent inventory. Container restarts may create new host IDs.</p>
          <ul>{#each serverMachines as machine (machine.host)}{#each machine.groups as heartbeat (heartbeat.id)}<li><strong>{machine.host}</strong><p class="hint">{heartbeatStatus(heartbeat)} · {when(heartbeat.scannedAt || heartbeat.lastSeen)}</p>{#if canWrite}<button class="btn quiet" onclick={() => (machineChange = { from: machine.host })} disabled={busy}>Forget machine {machine.host}</button>{/if}</li>{/each}{/each}</ul>
        </details>
      {/if}
    </details>
  {/if}
</section>

{#if joining && canAdd}
  <AddMachineDialog {site} {machines} snapshot={loaded ? syncState : undefined} {accounts} {canWrite} {token} {tokenSetup} context={joining.context} opener={joining.opener}
    channel={serverBuild?.version === 'dev' ? 'dev' : 'stable'} onrefresh={refreshForJoin} onready={joinReady} onstepbystep={joinStepByStep} {onsignedout} onclose={joinClosed} />
{/if}

{#if machineChange}
  <ConfirmDialog title={machineChange.to ? 'Combine or rename these reports?' : `Forget ${machineChange.from}?`}
    body={machineChange.to ? `Move ${machineChange.from} into ${machineChange.to}. For matching agent/project groups, the most recent report is kept; distinct groups and saved references are preserved. Old names become aliases. Installed files and shelf entries are unchanged. Conflicting saved references block the whole change.` : 'Remove this machine’s reports and target references. Installed files and shelf entries stay. Snapshots used by other machines remain, and a future scan can report this machine again.'}
    confirmLabel={machineChange.to ? 'Save machine identity' : 'Forget machine'} danger={!machineChange.to} onconfirm={changeMachine} oncancel={() => (machineChange = null)} />
{/if}

<style>
  .sync { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 14px; background: var(--card); }
  .head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .eyebrow { margin: 0 0 4px; color: var(--accent); text-transform: uppercase; letter-spacing: .1em; font-size: .7rem; font-weight: 650; }
  h2 { margin: 0; font: 500 1.7rem/1.15 var(--serif); }
  .lede { color: var(--muted); max-width: 72ch; margin: 8px 0 0; line-height: 1.6; }
  .hint { color: var(--muted); font-size: .83rem; margin: 6px 0; overflow-wrap: anywhere; }
  .machine-focus a { color: var(--accent); }
  .machine-focus strong { color: var(--ink); }
  .maintenance { margin: 16px 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .maintenance > summary { cursor: pointer; min-height: 24px; font-weight: 600; }
  .machine-form { display: grid; gap: 12px; margin-top: 16px; }
  .machine-form label { display: grid; gap: 4px; font-size: .85rem; font-weight: 600; }
  .fleet-command { margin-top: 16px; }
  .command { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin: 10px 0; padding: 12px; background: var(--card); border: 1px solid var(--line); border-radius: 8px; }
  pre { flex: 1 1 260px; min-width: 0; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: .78rem/1.6 var(--mono); }
  .advanced-fleet { margin-top: 24px; border-top: 1px solid var(--line); padding-top: 16px; }
  .advanced-fleet > summary { cursor: pointer; font-weight: 650; font-size: .88rem; }
  .error, .notice { padding: 12px; border: 1px solid var(--bad); border-radius: 8px; overflow-wrap: anywhere; }
  .notice { border-color: var(--good); }
  @media (max-width: 560px) {
    .command { align-items: stretch; }
    .command :global(button) { width: 100%; }
  }
</style>
