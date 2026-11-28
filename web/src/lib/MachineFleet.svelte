<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Sentence from './Sentence.svelte'
  import CopyButton from './CopyButton.svelte'
  import { lockedStatus, runningLine, runningOnMachine } from './machineLock'
  import { LOCKED_SENTENCE, cardReadiness, folderProjectList, terminalMachines, terminalSessions, type FolderProject, type HealthResponse, type TerminalMachine, type TerminalSession } from './terminal/api'
  import { openTerminalHref } from './terminal/openTerminal'
  import StartAgentDialog from './StartAgentDialog.svelte'
  import { getMachineHealth as machineHealth } from './api'
  import MachineHealthStrip from './terminal/MachineHealthStrip.svelte'
  import type { Machine, SyncAgent, SyncEnvironment, SyncJob, SyncSkills, SyncState } from './api'
  import { controlSyncAgent, controlSyncJob, forgetEnvironment, forgetMachine, updateSyncAgent } from './api'
  import { agentBuild, agentGroups, agentUpdateState, agentUpdateTarget, groupLabel, machineMatchesSearch, updateJobFor } from './machines'
  import { machineStatus, scanOnlyAgents, supportedAgents } from './machineSetup'
  import MachineCommands from './MachineCommands.svelte'
  import { reenrolCommand } from './machineJoin'
  import AgentUpdate from './AgentUpdate.svelte'
  import MachineSettings from './MachineSettings.svelte'
  import UpdateReview from './UpdateReview.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import Icon from './Icon.svelte'
  import { agoLabel } from './time'
  import { machineSkillStatus } from './sync/machineStatus'
  import { formatWhen } from './time'

  let {
    machines,
    snapshot,
    skills = null,
    site,
    canWrite,
    timeZone = '',
    dateFormat = 'locale',
    accounts=false,
    terminalAccess=false,
    monitorAccess=false,
    onjoin,
    highlight = '',
    onupdate,
    onerror,
    onnotice,
    onsignedout,
  }: {
    machines: Machine[]
    skills?: SyncSkills | null
    snapshot: SyncState
    site: string
    canWrite: boolean
    timeZone?: string
    dateFormat?: string
    accounts?: boolean
    terminalAccess?:boolean
    monitorAccess?:boolean
    /** Opens the Add a machine dialog; absent when the person cannot use it. Given the button, which gets focus back. */
    onjoin?: (opener: HTMLElement | null, name?: string) => void
    /** The machine just brought in by that dialog: its card is marked for a few seconds. */
    highlight?: string
    onupdate: (state: SyncState) => void
    onerror: (error: unknown) => void
    onnotice: (message: string, href?: string, action?: { label: string; run: () => void | Promise<void> }) => void
    // The sign-in ran out inside a dialog: the page clears it and shows its sign-in card.
    onsignedout?: (error: unknown) => void
  } = $props()

  type Pending = { title: string; body: string; details?: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> }
  let commandsFor = $state<Machine | null>(null)
  let fleetQuery = $state('')
  let fleetFilter = $state<'all' | 'attention' | 'working' | 'synced'>('all')
  let confirm = $state<Pending | null>(null)
  let busy = $state('')

  async function showUpdateProgress(agentId: string) {
    await tick()
    const panel = document.querySelector<HTMLElement>(`[data-agent-update="${CSS.escape(agentId)}"]`)
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    panel?.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'nearest' })
  }
  // The settings dialog and the review read the live snapshot by host and id,
  // so a change made inside them shows there at once instead of in a stale copy.
  let settingsHost = $state('')
  let review = $state<{ agentId: string; initial: 'dev' | 'latest' } | null>(null)
  const settingsMachine = $derived(machines.find((machine) => machine.host === settingsHost))
  const toneRank: Record<string, number> = { action: 0, review: 1, working: 2, attention: 3, setup: 4, synced: 5 }
  function fleetGroup(machine: Machine): 'attention' | 'working' | 'synced' {
    const tone = machineSkillStatus(machine, skills).tone
    return tone === 'working' ? 'working' : tone === 'synced' ? 'synced' : 'attention'
  }
  // A just-added machine stays visible even while a search or status filter that does not name it is active.
  const filteredMachines = $derived(machines.filter((machine) =>
    (!fleetQuery.trim() || machineMatchesSearch(machine, fleetQuery) || machine.host === highlight) &&
    (fleetFilter === 'all' || fleetGroup(machine) === fleetFilter)))
  const shownMachines = $derived([...filteredMachines].sort((a, b) =>
    (toneRank[machineSkillStatus(a, skills).tone] ?? 9) - (toneRank[machineSkillStatus(b, skills).tone] ?? 9) ||
    a.host.localeCompare(b.host, undefined, { sensitivity: 'base' })))
  const fleetCounts = $derived({
    attention: machines.filter((machine) => fleetGroup(machine) === 'attention').length,
    working: machines.filter((machine) => fleetGroup(machine) === 'working').length,
    synced: machines.filter((machine) => fleetGroup(machine) === 'synced').length,
  })
  const fleetCount = $derived(fleetQuery.trim() || fleetFilter !== 'all' ? `${shownMachines.length} of ${machines.length} machines` : `${machines.length} ${machines.length === 1 ? 'machine' : 'machines'}`)
  // Machines enrolled with a personal API token. More than one is said once, above the list, and each row only marks itself.
  const legacyCount = $derived(shownMachines.filter((machine) => agentFor(machine)?.legacyToken).length)
  const reviewAgent = $derived(review ? snapshot.agents?.find((agent) => agent.id === review?.agentId) : undefined)
  let health=$state<HealthResponse>({serverNow:Date.now(),machines:[]}), healthElapsed=$state(0),healthReceived=performance.now()
  onMount(()=>{if(!monitorAccess)return;const abort=new AbortController();let busy=false;async function refreshHealth(){if(busy||document.visibilityState!=='visible')return;busy=true;try{const next=await machineHealth(abort.signal);if(!abort.signal.aborted){health=next;healthReceived=performance.now();healthElapsed=0}}catch{/* inventory remains usable when health is unavailable */}finally{busy=false}}void refreshHealth();const wake=()=>void refreshHealth();document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);const timer=setInterval(wake,15000),clock=setInterval(()=>healthElapsed=performance.now()-healthReceived,1000);return()=>{abort.abort();clearInterval(timer);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}})

  // Terminal readiness for Start an agent, read only for a writer in accounts mode (the people terminalAccess is given to).
  // Undefined until the first read; a failed read keeps what was last known.
  let terminals = $state<TerminalMachine[] | undefined>()
  // The last read failed: said on the card (and the button stays disabled) until a read succeeds. Reads keep the 15-second pace.
  let terminalsFailed = $state(false)
  // The live sessions, read beside the machines: a lock does not stop them, so a locked card says how many are running.
  let liveSessions = $state<TerminalSession[]>()
  let startAgent = $state<{ agent: string; opener: HTMLElement | null } | null>(null)
  // The folder projects the machine's Start an agent offers, so it asks for the same project choice as every other entry
  // point. Read when the dialog opens; a failed read leaves only the explicit "No project" folder path.
  let folderProjects = $state<FolderProject[]>([])
  async function beginStart(agent: string, opener: HTMLElement) {
    startAgent = { agent, opener }
    try { folderProjects = (await folderProjectList()).projects } catch { /* the folder path is still available */ }
  }
  onMount(() => {
    if (!terminalAccess) return
    const abort = new AbortController()
    let reading = false
    async function read() {
      if (reading || document.visibilityState !== 'visible') return
      reading = true
      try { const next = await terminalMachines(AbortSignal.any([abort.signal, AbortSignal.timeout(10000)])); if (!abort.signal.aborted) { terminals = next; terminalsFailed = false } if (next.some((item) => item.locked)) { try { const list = await terminalSessions(AbortSignal.any([abort.signal, AbortSignal.timeout(10000)])); if (!abort.signal.aborted) liveSessions = list } catch { /* the count is left out until a read succeeds */ } } } catch { if (!abort.signal.aborted) terminalsFailed = true } finally { reading = false }
    }
    void read()
    const wake = () => void read()
    document.addEventListener('visibilitychange', wake); window.addEventListener('online', wake)
    const timer = setInterval(wake, 15000)
    return () => { abort.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake) }
  })
  const isLocked = (agent?: SyncAgent): boolean => !!agent && (!!agent.locked || !!terminals?.find((item) => item.agentId === agent.id)?.locked)
  const lockedAt = (agent: SyncAgent): number | undefined => agent.lockedAt ?? terminals?.find((item) => item.agentId === agent.id)?.lockedAt
  function agentReadiness(agentId: string): string {
    if (terminalsFailed) return "Couldn't check this machine just now. Try again in a moment."
    if (!terminals) return 'Checking whether this machine is ready for terminals.'
    return cardReadiness(terminals.find((item) => item.agentId === agentId))
  }
  // The dialog stays Starting… until the Terminals page arrives, and lets go by itself if it never does.
  const sessionHref = (session: TerminalSession) => `/sessions?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`
  function agentStarted(session: TerminalSession): 'leaving' {
    location.assign(sessionHref(session))
    return 'leaving'
  }
  // The Terminals page never arrived (the dialog let go after a few seconds): say what started, with the way to it, as Home does.
  function agentLeaveFailed(session: TerminalSession) {
    const host = terminals?.find((m) => m.agentId === session.agentId)?.host ?? session.agentId
    onnotice(`Started ${session.title} on ${host}.`, sessionHref(session))
  }
  // From Start an agent, Not listed? closes it and opens Add a machine, which hands focus back to Start an agent's button.
  function addFromStart(source: { opener: HTMLElement | null }): (() => void) | undefined {
    if (!onjoin || !accounts || !canWrite) return undefined
    return () => onjoin(source.opener)
  }

  function fresh(group: SyncEnvironment): boolean {
    return group.scannedAt > Date.now() - 14 * 86400000
  }
  function agentFor(machine: Machine): SyncAgent | undefined {
    return snapshot.agents?.find((agent) => agent.host.toLowerCase() === machine.host.toLowerCase())
  }
  function lastReport(machine: Machine): number {
    return Math.max(0, ...agentGroups(machine).map((group) => group.scannedAt || group.lastSeen))
  }
  function lastSeen(machine: Machine): string {
    const at = lastReport(machine)
    return at ? agoLabel(at) : 'No report yet'
  }
  function agentTone(agent: SyncAgent): 'online' | 'paused' | 'offline' {
    if (agent.paused) return 'paused'
    return agent.lastSeen > Date.now() - 90000 ? 'online' : 'offline'
  }
  function stateLabel(machine: Machine): string {
    return ({ current: 'Inventory current', stale: 'Scan is out of date', partial: 'Scan needs attention', empty: 'No agent inventory' })[machineStatus(machine)]
  }
  function agentState(agent: SyncAgent): string {
    if (agent.paused) return 'Sync paused'
    return agent.lastSeen > Date.now() - 90000 ? 'Sync agent online' : 'Sync agent offline'
  }
  function agentSeen(agent: SyncAgent): string {
    return formatWhen(agent.lastSeen, timeZone, dateFormat)
  }
  function lastUpdate(machine: Machine): SyncJob | undefined {
    const agent = agentFor(machine)
    if (!agent) return
    return updateJobFor(snapshot.updateJobs ?? snapshot.jobs, agent.id)
  }
  function groupState(group: SyncEnvironment): string {
    if (!group.complete) return 'Incomplete'
    if (!fresh(group)) return 'Out of date'
    return 'Current'
  }
  async function run(key: string, fn: () => Promise<SyncState>): Promise<boolean> {
    if (busy) return false
    busy = key
    try {
      onupdate(await fn())
      return true
    } catch (error) {
      onerror(error)
      return false
    } finally {
      busy = ''
    }
  }
  function pause(agent: SyncAgent) {
    return run(`pause-${agent.id}`, () => controlSyncAgent(agent.id, agent.paused ? 'resume' : 'pause'))
  }
  function closeMenu(event: MouseEvent) {
    (event.currentTarget as HTMLElement | null)?.closest('details')?.removeAttribute('open')
  }
  function closeMenus() {
    document.querySelectorAll<HTMLDetailsElement>('details.sync-menu[open]').forEach((menu) => menu.removeAttribute('open'))
  }
  // A right-click anywhere on a machine card opens the same options menu as its ⋮ button, so the card itself offers
  // Sync now's neighbours, Commands, Settings and Review sync activity.
  function cardContextMenu(event: MouseEvent) {
    if ((event.target as HTMLElement).closest('details.sync-menu, dialog')) return
    const menu = (event.currentTarget as HTMLElement).querySelector<HTMLDetailsElement>('details.sync-menu')
    if (!menu) return
    event.preventDefault()
    closeMenus()
    menu.open = true
    void tick().then(() => menu.querySelector<HTMLElement>('[role=menuitem]:not([disabled])')?.focus())
  }
  function menusKey(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const opener = document.querySelector<HTMLElement>('details.sync-menu[open] > summary')
    if (!opener) return
    closeMenus()
    opener.focus()
  }
  async function updateAgent(agent: SyncAgent, version: string = agentUpdateTarget(agent)) {
    if (await run(`update-${agent.id}`, () => updateSyncAgent(agent.id, version))) {
      const switching = (version === 'dev') !== (agent.version === 'dev')
      const target = version === 'dev' ? 'the development build' : 'the latest release'
      onnotice(`${switching ? 'Switch' : 'Update'} to ${target} queued for ${agent.host}.`, undefined, { label: 'View progress', run: () => showUpdateProgress(agent.id) })
      await showUpdateProgress(agent.id)
    }
  }
  async function cancelUpdate(job: SyncJob) {
    const host = machineOf(job)?.host
    if (await run(`cancel-${job.id}`, () => controlSyncJob(job.id, 'cancel'))) {
      onnotice(`Update cancelled for ${host ?? 'the machine'}. It stays on its current build.`, undefined, { label: 'View progress', run: () => showUpdateProgress(job.agentId) })
    }
  }
  function machineOf(job: SyncJob): Machine | undefined {
    return machines.find(machine => agentFor(machine)?.id === job.agentId)
  }
  function openReview(agent: SyncAgent, initial: 'dev' | 'latest') {
    settingsHost = ''
    review = { agentId: agent.id, initial }
  }
  // The settings close before the confirmation opens, and the selection is
  // what the dialog derives from, so the machine is read before it is cleared.
  function forgetFromSettings() {
    const machine = settingsMachine
    settingsHost = ''
    if (machine) askForgetMachine(machine)
  }
  function revokeFromSettings() {
    const agent = settingsMachine && agentFor(settingsMachine)
    settingsHost = ''
    if (agent) askRevoke(agent)
  }
  function forgetScopeFromSettings(group: SyncEnvironment) {
    const machine = settingsMachine
    settingsHost = ''
    if (machine) askForgetGroup(group, machine)
  }
  function confirmReview(version: string) {
    const agent = reviewAgent
    review = null
    if (agent) void updateAgent(agent, version)
  }
  async function toggleAutoUpdate(agent: SyncAgent, enabled: boolean) {
    if (await run(`auto-update-${agent.id}`, () => controlSyncAgent(agent.id, enabled ? 'auto-update-on' : 'auto-update-off'))) {
      onnotice(enabled
        ? `Automatic updates enabled for ${agent.host}. The controller will move it to the latest ${agentUpdateTarget(agent) === 'dev' ? 'dev' : 'release'} build and verify it before reporting back.`
        : `Automatic updates disabled for ${agent.host}. Manual updates still work.`)
    }
  }
  function askForgetMachine(machine: Machine) {
    confirm = {
      title: `Forget ${machine.host}?`,
      body: 'This removes inventory reports. It does not revoke enrolment or terminal access. Installed files and running shells stay; a future scan can report it again.',
      confirmLabel: 'Forget machine',
      danger: true,
      action: async () => { if (await run(`forget-${machine.host}`, () => forgetMachine(machine.host))) onnotice(`Forgot ${machine.host}. Installed files are unchanged.`) },
    }
  }
  function askForgetGroup(group: SyncEnvironment, machine: Machine) {
    confirm = {
      title: `Forget ${groupLabel(group)} on ${machine.host}?`,
      body: 'This removes this agent report and its saved sync reference. Installed files and library entries stay. A future scan can report it again.',
      confirmLabel: 'Forget install location',
      danger: true,
      action: async () => { if (await run(`forget-${group.id}`, () => forgetEnvironment(group.id))) onnotice(`Forgot ${groupLabel(group)}. Installed files are unchanged.`) },
    }
  }
  function askRevoke(agent: SyncAgent) {
    confirm = {
      title: `Revoke live access for ${agent.host}?`,
      body: 'This closes private access and prevents new terminal authorization and sync jobs. Independent local tmux shells keep running. A running sync operation may finish; installed files remain.',
      confirmLabel: 'Revoke enrolment',
      danger: true,
      action: async () => { if (await run(`revoke-${agent.id}`, () => controlSyncAgent(agent.id, 'revoke'))) onnotice(`${agent.host} is no longer enrolled.`) },
    }
  }
</script>

<svelte:window onclick={(event) => { if (!(event.target as Element | null)?.closest('.sync-menu')) closeMenus() }} onkeydown={menusKey} />

<section class="fleet" aria-label="Machines">
  {#if machines.length > 1}
    <div class="fleet-tools">
      <label class="fleet-search"><span class="vh">Search machines</span>
        <input type="search" bind:value={fleetQuery} placeholder="Search machines or skills" aria-label="Search machines" />
      </label>
      {#if machines.length > 6}
        <label class="fleet-status">Status
          <select bind:value={fleetFilter}>
            <option value="all">All machines</option>
            <option value="attention">Needs attention ({fleetCounts.attention})</option>
            <option value="working">Working ({fleetCounts.working})</option>
            <option value="synced">Up to date ({fleetCounts.synced})</option>
          </select>
        </label>
      {/if}
      <p class="fleet-count" role="status">{fleetCount}</p>
    </div>
  {/if}
  {#if legacyCount > 1}
    <div class="legacy legacy-note" data-legacy-note>
      <p class="legacy-line"><span class="mark" aria-hidden="true">!</span><span>{legacyCount} machines were enrolled with a personal API token.</span></p>
      <p class="legacy-why">A personal API token can do most of what your account can and never expires.</p>
      <details class="legacy-how">
        <summary>How to change this</summary>
        <p>On each machine run this with a new enrolment code, then revoke the token that machine used, under Account.</p>
        <pre class="legacy-cmd">{reenrolCommand()}</pre>
        <p class="legacy-hint">It keeps the machine's settings and asks for the code.</p>
      </details>
    </div>
  {/if}
  <div class="machine-grid">
    {#each shownMachines as machine, m (machine.host)}
      {@const enrolled = agentFor(machine)}
      {@const groups = agentGroups(machine)}
      {@const supported = supportedAgents(machine)}
      {@const scanOnly = scanOnlyAgents(machine)}
      {@const state = machineStatus(machine)}
      {@const locked = isLocked(enrolled)}
      {@const sync = machineSkillStatus(machine, skills, locked ? 'Skills wait until it is unlocked there.' : '')}
      {@const running = locked && enrolled ? runningLine(runningOnMachine(liveSessions, enrolled.id)) : ''}
      <article class="machine" class:just-added={highlight === machine.host} aria-labelledby={`machine-${encodeURIComponent(machine.host)}`} oncontextmenu={cardContextMenu}>
        <header class="machine-head">
          <div class="identity">
            {#if locked}<span class="mark head-mark" aria-hidden="true">!</span>{:else}<span class="dot {enrolled ? (agentTone(enrolled) === 'online' ? 'current' : agentTone(enrolled)) : 'empty'}" aria-hidden="true"></span>{/if}
            <div>
              <h3 id={`machine-${encodeURIComponent(machine.host)}`} title={machine.host}>{machine.host}</h3>
              {#if highlight === machine.host}<p class="added-note">Just added. Active skills follow automatically</p>{/if}
              {#if machine.aliases?.length}<p class="alias">Also known as {machine.aliases.join(', ')}</p>{/if}
            </div>
          </div>
          <div class="status {sync.tone}">
            <p class="status-line"><span class="sync-pulse" class:live={sync.tone === 'working'} aria-hidden="true"></span><strong>{sync.label}</strong>
              {#if locked && enrolled}<span class="badge locked-badge" title={lockedAt(enrolled) ? formatWhen(lockedAt(enrolled)! * 1000, timeZone, dateFormat) : undefined}>{lockedStatus(lockedAt(enrolled), (at) => agoLabel(at))}</span>{:else if state !== 'current'}<span class="badge {state}">{stateLabel(machine)}</span>{/if}
            </p>
            <p class="status-detail">{sync.detail} <a class="sync-link" href="/sync">View sync status</a></p>
          </div>
          <div class="head-actions">
            {#if terminalAccess && enrolled}
              {#if locked}<button class="btn" type="button" disabled aria-describedby={`locked-${m}`}>Open terminal</button>{:else}<a class="btn" href={openTerminalHref(enrolled.id)}>Open terminal</a>{/if}
              <button class="btn" type="button" disabled={!!agentReadiness(enrolled.id)} aria-describedby={locked ? `locked-${m}` : agentReadiness(enrolled.id) ? `start-agent-${m}` : undefined} onclick={(event) => void beginStart(enrolled.id, event.currentTarget)}>Start an agent</button>
            {/if}
            <details class="sync-menu">
              <summary aria-label={`More options for ${machine.host}`} title="More options"><Icon name="more" size={18} /></summary>
              <div class="sync-menu-panel" role="menu">
                <a role="menuitem" href="/skills">Manage library skills</a>
                <button type="button" role="menuitem" onclick={(event) => { closeMenu(event); commandsFor = machine }}>Commands…</button>
                {#if canWrite}<button type="button" role="menuitem" onclick={(event) => { closeMenu(event); settingsHost = machine.host }}>Settings…</button>{/if}
              </div>
            </details>
          </div>
        </header>
        {#if terminalAccess&&enrolled&&!locked&&agentReadiness(enrolled.id)}<p class="start-reason" id={`start-agent-${m}`}><Sentence text={agentReadiness(enrolled.id)} /></p>{/if}
        {#if enrolled&&health.machines.some(item=>item.agentId===enrolled.id)}<MachineHealthStrip compact health={health.machines.find(item=>item.agentId===enrolled.id)} serverNow={health.serverNow+healthElapsed} />{/if}

        <p class="meta">
          <span><strong>{machine.skillCount}</strong> unique {machine.skillCount === 1 ? 'skill' : 'skills'}</span>
          <span>Last report <strong title={lastReport(machine) ? formatWhen(lastReport(machine), timeZone, dateFormat) : undefined}>{lastSeen(machine)}</strong></span>
          {#if enrolled && !locked}{#if agentTone(enrolled) !== 'paused'}<span class="live {agentTone(enrolled)}">{agentState(enrolled)}</span>{/if}<span class="build" title={enrolled.commit ? `commit ${enrolled.commit}` : undefined}>reet {agentBuild(enrolled)}</span>{/if}
        </p>

        {#snippet updater()}
          {#if enrolled}
            <div data-agent-update={enrolled.id}>
              <AgentUpdate
                agent={enrolled}
                job={lastUpdate(machine)}
                releases={snapshot.releases}
                {canWrite}
                busy={!!busy}
                onreview={() => openReview(enrolled, enrolled.version === 'dev' ? 'dev' : 'latest')}
                oncancel={() => { const update = lastUpdate(machine); if (update) void cancelUpdate(update) }}
              />
            </div>
          {/if}
        {/snippet}
        <!-- An update that is waiting, running or failed is said on the row; "Not checked" and "Up to date" wait in the details. -->
        {#if enrolled && !['current', 'unknown'].includes(agentUpdateState(enrolled, snapshot.releases, lastUpdate(machine)))}{@render updater()}{/if}

        {#if locked && enrolled}
          <div class="legacy locked" data-locked={enrolled.id}>
            <p class="legacy-line" id={`locked-${m}`}><span class="mark" aria-hidden="true">!</span><span><Sentence text={LOCKED_SENTENCE} /></span></p>
            {#if running}<p class="running"><Sentence text={running} /></p>{/if}
            <details class="legacy-how">
              <summary>What this means</summary>
              <p>Locked on the machine: reet cannot start, attach to or change anything there. It still reports its health and inventory.</p>
              <p>To unlock, run <code>reet agent unlock</code> on it. <CopyButton compact text="reet agent unlock" label="Copy unlock command" /></p>
            </details>
          </div>
        {/if}

        {#if enrolled?.legacyToken}
          <div class="legacy" data-legacy-token={enrolled.id}>
            {#if legacyCount === 1}
              <p class="legacy-line"><span class="mark" aria-hidden="true">!</span><span>Enrolled with a personal API token, which can do most of what your account can and never expires.</span></p>
              <details class="legacy-how">
                <summary>How to change this</summary>
                <p>On that machine run this with a new enrolment code, then revoke the token that machine used, under Account.</p>
                <pre class="legacy-cmd">{reenrolCommand()}</pre>
                <p class="legacy-hint">It keeps the machine's settings and asks for the code.</p>
              </details>
            {:else}
              <p class="legacy-line"><span class="mark" aria-hidden="true">!</span><span>Personal token</span></p>
            {/if}
            {#if onjoin && accounts && canWrite}<div><button type="button" class="btn" aria-label={`Enrol again: ${machine.host}`} onclick={(event) => onjoin(event.currentTarget, machine.host)}>Enrol again</button></div>{/if}
          </div>
        {/if}

        <details class="agent-list">
          <summary>
            <span>Agents on this machine</span>
            <span>{groups.length} install {groups.length === 1 ? 'location' : 'locations'}</span>
          </summary>
          {#if enrolled && ['current', 'unknown'].includes(agentUpdateState(enrolled, snapshot.releases, lastUpdate(machine)))}{@render updater()}{/if}
          <ul>
            {#each supported as group, i (group.id)}
              <li>
                <span class="dot {group.complete && fresh(group) ? 'current' : group.complete ? 'stale' : 'partial'}" aria-hidden="true"></span>
                <div class="agent-copy">
                  <strong>{groupLabel(group)}</strong>
                  <small>{group.skillCount ?? 0} {group.skillCount === 1 ? 'skill' : 'skills'} · {group.complete ? 'Last full report' : 'Incomplete report'} <span title={formatWhen(group.scannedAt || group.lastSeen, timeZone, dateFormat)}>{agoLabel(group.scannedAt || group.lastSeen)}</span></small>


                </div>
                <span class="scope-state">{groupState(group)}</span>
              </li>
            {/each}
            {#each scanOnly as group, i (group.id)}
              <li class="scan-only">
                <span class="dot empty" aria-hidden="true"></span>
                <div class="agent-copy"><strong>{groupLabel(group)}</strong><small>Scan only. Library sync manages supported home folders.</small></div>
                <span class="scope-state">Scan only</span>
              </li>
            {/each}
            {#if groups.length === 0}<li class="empty-row">No agent inventory yet. Run a scan, then refresh.</li>{/if}
          </ul>
          {#if canWrite && groups.length}
            <div class="agent-links">
              <button type="button" class="btn quiet" onclick={() => (commandsFor = machine)}>Copy rescan command</button>
            </div>
          {/if}
        </details>
      </article>
    {:else}
      <div class="fleet-empty">
        <p>{fleetQuery.trim() ? `No machine here matches “${fleetQuery}”.` : 'No machine matches that status.'}</p>
        <button type="button" class="btn" onclick={() => { fleetQuery = ''; fleetFilter = 'all' }}>Clear filters</button>
      </div>
    {/each}
  </div>
</section>

{#if startAgent && terminalAccess && terminals}<StartAgentDialog machines={terminals} snapshot={snapshot.machines ?? []} projects={folderProjects} initialAgent={startAgent.agent} opener={startAgent.opener} onstarted={agentStarted} onleavefailed={agentLeaveFailed} onaddmachine={addFromStart(startAgent)} {onsignedout} onclose={() => (startAgent = null)} />{/if}

{#if commandsFor}
  <MachineCommands
    machine={commandsFor.host}
    {site}
    enrolled={!!agentFor(commandsFor)}
    agentStatus={agentFor(commandsFor) ? `${agentState(agentFor(commandsFor)!)} · reet ${agentBuild(agentFor(commandsFor)!)} · last seen ${agentSeen(agentFor(commandsFor)!)}` : ''}
    onclose={() => (commandsFor = null)}
  />
{/if}

{#if settingsMachine}
  {@const enrolled = agentFor(settingsMachine)}
  <MachineSettings
    machine={settingsMachine}
    agent={enrolled}
    terminal={terminals?.find((item) => item.host === settingsMachine.host)}
    releases={snapshot.releases}
    busy={!!busy}
    onautoupdate={(enabled) => { if (enrolled) void toggleAutoUpdate(enrolled, enabled) }}
    onpause={() => { if (enrolled) void pause(enrolled) }}
    onswitch={() => { if (enrolled) openReview(enrolled, enrolled.version === 'dev' ? 'latest' : 'dev') }}
    onrevoke={revokeFromSettings}
    onforget={forgetFromSettings}
    onforgetscope={forgetScopeFromSettings}
    onclose={() => (settingsHost = '')}
  />
{/if}

{#if review && reviewAgent}
  <UpdateReview agent={reviewAgent} releases={snapshot.releases} initial={review.initial} onconfirm={confirmReview} oncancel={() => (review = null)} />
{/if}

{#if confirm}
  <ConfirmDialog
    title={confirm.title}
    body={confirm.body}
    details={confirm.details}
    confirmLabel={confirm.confirmLabel}
    danger={confirm.danger}
    onconfirm={() => { const action = confirm?.action; confirm = null; void action?.() }}
    oncancel={() => (confirm = null)}
  />
{/if}

<style>
  .fleet { margin: 14px 0 0; }
  .fleet-tools { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px 14px; margin-bottom: 10px; }
  .fleet-search { flex: 1 1 240px; display: grid; }
  .fleet-search input { width: 100%; }
  .fleet-count { margin: 0; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .fleet-status { flex: 0 1 auto; display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 0.875rem; }
  .fleet-status select { min-width: 170px; }
  .fleet-empty { grid-column: 1 / -1; padding: 24px; border: 1px dashed var(--line); border-radius: 12px; background: var(--card); }
  .fleet-empty p { margin: 0 0 12px; color: var(--muted); }
  .machine-grid { display: grid; gap: 10px; }
  .added-note { margin: 2px 0 0; color: var(--good); font-size: 0.875rem; font-weight: 600; }
  .machine.just-added { border-color: var(--good); animation: just-added 6s ease-out; }
  @keyframes just-added { from { background: color-mix(in srgb, var(--good) 22%, var(--card)); box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 45%, transparent); } to { background: var(--card); box-shadow: var(--shadow); } }
  @media (prefers-reduced-motion: reduce) { .machine.just-added { animation: none; box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 45%, transparent); } }
  .machine { display: grid; gap: 8px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); min-width: 0; }
  .machine-head { display: grid; grid-template-columns: minmax(0, 13rem) minmax(0, 1fr) auto; align-items: center; gap: 8px 16px; }
  .identity { display: flex; align-items: flex-start; gap: 10px; min-width: 0; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; margin-top: 5px; background: var(--muted); }
  .dot.offline { background: transparent; border: 2px solid var(--muted); }
  .dot.paused { background: var(--warn); }
  .dot.current { background: var(--good); }
  .dot.stale { background: var(--warn, #b45309); }
  .dot.partial { background: var(--bad); }
  .dot.empty { background: var(--line); }
  h3 { margin: 0; font: 500 1.2rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .alias { margin: 3px 0 0; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .badge { flex: none; padding: 4px 8px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .badge.current { color: var(--good); border-color: color-mix(in srgb, var(--good) 35%, var(--line)); }
  .badge.stale { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, var(--line)); }
  .badge.partial { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 14px; margin: 0; padding-left: 19px; color: var(--muted); font-size: 0.875rem; }
  .meta strong { color: var(--ink); font-weight: 600; font-variant-numeric: tabular-nums; }
  .live.online { color: var(--good); }
  .live.paused { color: var(--warn); }
  .live.offline { color: var(--muted); }
  .build { color: var(--ink); }
  /* One status, said once: the label carries the colour only when something needs a person. Healthy rows stay calm. */
  .status { display: grid; gap: 2px; min-width: 0; }
  .status p { margin: 0; overflow-wrap: anywhere; }
  .status-line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; font-size: 0.875rem; }
  .status-detail { color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .sync-link { color: inherit; text-decoration: underline; text-underline-offset: 2px; white-space: nowrap; }
  .sync-link:hover { color: var(--ink); }
  .status.attention .status-line strong { color: var(--bad); }
  .status.review .status-line strong { color: var(--warn); }
  .status.action .status-line strong { color: var(--accent-text); }
  .status.setup .status-line strong { color: var(--muted); }
  .sync-pulse { display: none; width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--accent); }
  .sync-pulse.live { display: inline-block; animation: sync-pulse 1.4s ease-in-out infinite; }
  @keyframes sync-pulse { 50% { opacity: 0.35; transform: scale(0.8); } }
  .head-actions { display: flex; align-items: center; gap: 6px; justify-self: end; }
  .head-actions .btn { min-height: 36px; padding: 6px 12px; font-size: 0.875rem; }
  .sync-menu { position: relative; }
  .sync-menu > summary { list-style: none; display: grid; place-items: center; width: 38px; height: 38px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card); color: var(--muted); cursor: pointer; }
  .sync-menu > summary::-webkit-details-marker { display: none; }
  .sync-menu > summary:hover, .sync-menu[open] > summary { color: var(--ink); border-color: var(--muted); }
  .sync-menu > summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .sync-menu-panel { position: absolute; z-index: 20; right: 0; top: calc(100% + 6px); display: grid; min-width: 214px; padding: 6px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card-raised); box-shadow: var(--shadow); }
  .sync-menu-panel a, .sync-menu-panel button { display: block; width: 100%; padding: 8px 10px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; font-size: 0.875rem; text-align: left; text-decoration: none; cursor: pointer; }
  .sync-menu-panel a:hover, .sync-menu-panel button:hover:not(:disabled) { background: var(--tint); }
  .sync-menu-panel button:disabled { color: var(--muted); cursor: default; }
  .legacy { display: grid; gap: 6px; color: var(--ink); font-size: 0.875rem; line-height: 1.5; }
  .legacy-line { margin: 0; display: flex; gap: 8px; align-items: flex-start; color: var(--warn); font-weight: 500; }
  .legacy .mark { flex: none; width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid currentColor; border-radius: 50%; font-size: 0.875rem; font-weight: 700; }
  /* Locked is a fact, not a warning: the same mark and disclosure as the legacy note, in the quiet colour. */
  .locked .legacy-line, .locked .running { color: var(--muted); }
  .locked .running { margin: 0; }
  .locked code { font: 0.875rem var(--mono); color: var(--ink); }
  .head-mark { flex: none; width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid var(--muted); border-radius: 50%; color: var(--muted); font-size: 0.875rem; font-weight: 700; }
  .badge.locked-badge { color: var(--muted); }
  .legacy-cmd { margin: 0 0 10px; padding: 10px 12px; background: var(--bg); border: 1px solid var(--line); border-radius: 8px; font: 0.875rem/1.6 var(--mono); white-space: pre-wrap; overflow-wrap: anywhere; color: var(--ink); }
  .legacy-hint { margin: -4px 0 10px; color: var(--muted); }
  .legacy-note { margin: 0 0 14px; }
  .legacy-why { margin: 0; color: var(--muted); }
  .legacy-how > summary { cursor: pointer; display: list-item; padding: 6px 0; color: var(--muted); }
  .legacy-how > p { margin: 6px 0 10px; color: var(--muted); overflow-wrap: anywhere; }
  @media (pointer: coarse) { .legacy-how > summary { padding: 12px 0; } .legacy .btn { min-height: 44px; } }
  .start-reason { margin: 0 0 0 19px; color: var(--muted); font-size: 0.875rem; line-height: 1.5; overflow-wrap: anywhere; }
  .agent-list { margin-left: 19px; }
  .agent-list > summary { display: flex; align-items: center; gap: 12px; min-height: 34px; cursor: pointer; font-size: 0.875rem; font-weight: 500; color: var(--muted); }
  .agent-list > summary::after { content: ''; width: 7px; height: 7px; flex: none; margin-right: 4px; border-right: 2px solid var(--muted); border-bottom: 2px solid var(--muted); transform: rotate(45deg); transition: transform 160ms ease; }
  .agent-list[open] > summary::after { transform: rotate(-135deg); }
  .agent-list ul { list-style: none; display: grid; gap: 0; padding: 0; margin: 8px 0 0; }
  .agent-list li { display: flex; align-items: flex-start; gap: 9px; padding: 10px 0; border-top: 1px solid var(--line); }
  .agent-copy { display: grid; gap: 2px; min-width: 0; flex: 1; }
  .agent-copy strong { font-size: 0.875rem; overflow-wrap: anywhere; }
  .agent-copy small { color: var(--muted); font-size: 0.875rem; line-height: 1.45; overflow-wrap: anywhere; }
  .scope-state { flex: none; color: var(--muted); font-size: 0.875rem; }
  .scan-only { opacity: 0.82; }
  .empty-row { color: var(--muted); font-size: 0.875rem; }
  .agent-links { display: flex; flex-wrap: wrap; gap: 6px 12px; margin-top: 8px; }
    @media (max-width: 650px) {
    .fleet-tools { gap: 10px; }
    .fleet-search, .fleet-status { flex-basis: 100%; }
  }
  @media (pointer: coarse) { .fleet-status select { min-height: 44px; } }
  @media (max-width: 860px) {
    .machine-head { grid-template-columns: minmax(0, 1fr); }
    .head-actions { justify-self: stretch; }
    .head-actions .btn { flex: 1 1 auto; }
  }
  @media (max-width: 560px) {
    .machine { padding: 12px; }
    .meta, .start-reason, .agent-list { margin-left: 0; padding-left: 0; }
  }
  /* Touch: every control on a card is a finger-sized (44 px) target. */
  @media (pointer: coarse) {
    .fleet-search input { min-height: 44px; }
    .head-actions .btn { min-height: 44px; }
    .sync-menu > summary { width: 44px; height: 44px; }
    .sync-menu-panel a, .sync-menu-panel button { min-height: 44px; }
    .agent-list > summary { min-height: 44px; }
    .machine :global(.updater .btn) { min-height: 44px; }
    .sync-link { display: inline-block; padding: 12px 0; }
  }
  @media (prefers-reduced-motion: reduce) {
    .sync-pulse.live { animation: none; }
    }
</style>
