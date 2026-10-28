<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { cardReadiness, terminalMachines, type HealthResponse, type TerminalMachine, type TerminalSession } from './terminal/api'
  import StartAgentDialog from './StartAgentDialog.svelte'
  import { getMachineHealth as machineHealth } from './api'
  import MachineHealthStrip from './terminal/MachineHealthStrip.svelte'
  import type { Machine, SyncAgent, SyncEnvironment, SyncJob, SyncState } from './api'
  import { controlSyncAgent, controlSyncJob, forgetEnvironment, forgetMachine, queueSyncJob, startSyncRun, updateSyncAgent } from './api'
  import { agentBuild, agentGroups, agentUpdateTarget, groupLabel, updateJobFor } from './machines'
  import { machineStatus, scanOnlyAgents, supportedAgents } from './machineSetup'
  import MachineCommands from './MachineCommands.svelte'
  import AgentUpdate from './AgentUpdate.svelte'
  import MachineSettings from './MachineSettings.svelte'
  import UpdateReview from './UpdateReview.svelte'
  import ContentObservation from './ContentObservation.svelte'
  import MachineSkillContent from './MachineSkillContent.svelte'
  import { scopeEligibility } from './contentReview'
  import type { JoinContext } from './machineJoin'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import { agoLabel, machineSyncOverview } from './syncOverview'

  let {
    machines,
    snapshot,
    site,
    canWrite,
    accounts=false,
    terminalAccess=false,
    monitorAccess=false,
    onjoin,
    onimport,
    landing = null,
    onlanded,
    highlight = '',
    onupdate,
    onerror,
    onnotice,
    onsignedout,
  }: {
    machines: Machine[]
    snapshot: SyncState
    site: string
    canWrite: boolean
    accounts?: boolean
    terminalAccess?:boolean
    monitorAccess?:boolean
    /** Opens the Add a machine dialog; absent when the person cannot use it. Given the button, which gets focus back. */
    onjoin?: (opener: HTMLElement | null, context?: JoinContext) => void
    onimport: (source: { environmentId: string; name: string }) => void
    /** A folder the Add a machine dialog chose for the Private skills dialog to open on, with the note saying which. Used once. */
    landing?: { environmentId: string; name?: string; note: string } | null
    /** Called as soon as `landing` has been turned into the dialog, so it cannot open again when this is drawn again. */
    onlanded?: () => void
    /** The machine just brought in by that dialog: its card is marked for a few seconds. */
    highlight?: string
    onupdate: (state: SyncState) => void
    onerror: (error: unknown) => void
    onnotice: (message: string, href?: string) => void
    // The sign-in ran out inside a dialog: the page clears it and shows its sign-in card.
    onsignedout?: (error: unknown) => void
  } = $props()

  type Pending = { title: string; body: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> }
  let commandsFor = $state<Machine | null>(null)
  let confirm = $state<Pending | null>(null)
  let busy = $state('')
  let contentSource = $state<{ environmentId: string; name?: string; opener: HTMLElement | null; note?: string } | null>(null)
  // The settings dialog and the review read the live snapshot by host and id,
  // so a change made inside them shows there at once instead of in a stale copy.
  let settingsHost = $state('')
  let review = $state<{ agentId: string; initial: 'dev' | 'latest' } | null>(null)
  const settingsMachine = $derived(machines.find((machine) => machine.host === settingsHost))
  const reviewAgent = $derived(review ? snapshot.agents?.find((agent) => agent.id === review?.agentId) : undefined)
  let health=$state<HealthResponse>({serverNow:Date.now(),machines:[]}), healthElapsed=$state(0),healthReceived=performance.now()
  onMount(()=>{if(!monitorAccess)return;const abort=new AbortController();let busy=false;async function refreshHealth(){if(busy||document.visibilityState!=='visible')return;busy=true;try{const next=await machineHealth(abort.signal);if(!abort.signal.aborted){health=next;healthReceived=performance.now();healthElapsed=0}}catch{/* inventory remains usable when health is unavailable */}finally{busy=false}}void refreshHealth();const wake=()=>void refreshHealth();document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);const timer=setInterval(wake,15000),clock=setInterval(()=>healthElapsed=performance.now()-healthReceived,1000);return()=>{abort.abort();clearInterval(timer);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}})

  // Terminal readiness for Start an agent, read only for a writer in accounts mode (the people terminalAccess is given to).
  // Undefined until the first read; a failed read keeps what was last known.
  let terminals = $state<TerminalMachine[] | undefined>()
  // The last read failed: said on the card (and the button stays disabled) until a read succeeds. Reads keep the 15-second pace.
  let terminalsFailed = $state(false)
  let startAgent = $state<{ agent: string; opener: HTMLElement | null } | null>(null)
  onMount(() => {
    if (!terminalAccess) return
    const abort = new AbortController()
    let reading = false
    async function read() {
      if (reading || document.visibilityState !== 'visible') return
      reading = true
      try { const next = await terminalMachines(AbortSignal.any([abort.signal, AbortSignal.timeout(10000)])); if (!abort.signal.aborted) { terminals = next; terminalsFailed = false } } catch { if (!abort.signal.aborted) terminalsFailed = true } finally { reading = false }
    }
    void read()
    const wake = () => void read()
    document.addEventListener('visibilitychange', wake); window.addEventListener('online', wake)
    const timer = setInterval(wake, 15000)
    return () => { abort.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake) }
  })
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
  // Private skills (pins) are for a signed-in account that can write; for anyone else the action is not there at all.
  // For the rest, scopeEligibility says plainly when it cannot be used yet, and the reason shows beside a disabled button.
  function privateSkills(group: SyncEnvironment) {
    return canWrite && accounts ? scopeEligibility(group, snapshot.agents, { accounts, canWrite, now: Date.now() }) : null
  }
  function pinOpener(group: SyncEnvironment): ((name: string, opener: HTMLElement) => void) | undefined {
    return privateSkills(group)?.ok ? (name, opener) => { contentSource = { environmentId: group.id, name, opener } } : undefined
  }
  // The landing from the Add a machine dialog becomes the Private skills dialog once, then is handed back as used.
  // The folder was chosen when Use was pressed; it is judged again here, against the snapshot and the clock as they are now.
  $effect(() => {
    const next = landing
    if (!next) return
    untrack(() => {
      const group = snapshot.environments?.find((env) => env.id === next.environmentId)
      if (!group || !privateSkills(group)?.ok) {
        onnotice("Private skills can't open on that folder right now.")
        onlanded?.()
        return
      }
      // When the folder's own button is not drawn (the page is filtered to another machine), focus goes back to Add a machine.
      const opener = document.querySelector<HTMLElement>(`[data-private-skills="${CSS.escape(next.environmentId)}"]`) ?? document.querySelector<HTMLElement>('[data-add-machine]')
      // The folder is shown when the dialog is closed, so focus has somewhere to go back to.
      for (let node = opener?.closest('details'); node; node = node.parentElement?.closest('details')) node.open = true
      contentSource = { environmentId: next.environmentId, name: next.name, opener, note: next.note }
      onlanded?.()
    })
  })
  // The Private skills dialog's way into Add a machine (only for a writer in accounts mode, who is the one `onjoin` is given to):
  // it closes, and the add dialog opens with what it came with, handing focus back to what was pressed if closed.
  function addFromPrivate(source: { opener: HTMLElement | null }): ((context: JoinContext) => void) | undefined {
    if (!onjoin || !accounts || !canWrite) return undefined
    return (context) => {
      const opener = source.opener
      contentSource = null
      onjoin(opener, context)
    }
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
    return new Date(agent.lastSeen).toLocaleString()
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
  async function scan(agent: SyncAgent) {
    if (await run(`scan-${agent.id}`, () => queueSyncJob(agent.id, '', 'scan'))) onnotice('Scan queued. The machine will report shortly.')
  }
  function pause(agent: SyncAgent) {
    return run(`pause-${agent.id}`, () => controlSyncAgent(agent.id, agent.paused ? 'resume' : 'pause'))
  }
  function savedTargets(machine: Machine) {
    return snapshot.targets.filter(item => item.target.host.toLowerCase() === machine.host.toLowerCase())
  }
  async function syncNow(machine: Machine, agent: SyncAgent) {
    if (await run(`sync-${agent.id}`, () => startSyncRun(agent.id, savedTargets(machine).map(item => item.target.id)))) {
      onnotice(`Automatic sync started for ${machine.host}. Additive plans apply on their own; removals wait for review.`)
    }
  }
  function closeMenu(event: MouseEvent) {
    (event.currentTarget as HTMLElement | null)?.closest('details')?.removeAttribute('open')
  }
  function closeMenus() {
    document.querySelectorAll<HTMLDetailsElement>('details.sync-menu[open]').forEach((menu) => menu.removeAttribute('open'))
  }
  function menusKey(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const opener = document.querySelector<HTMLElement>('details.sync-menu[open] > summary')
    if (!opener) return
    closeMenus()
    opener.focus()
  }
  async function previewNow(machine: Machine, agent: SyncAgent) {
    const targets = savedTargets(machine)
    if (!targets.length) return
    if (await run(`preview-${agent.id}`, async () => {
      let next = snapshot
      for (const item of targets) next = await queueSyncJob(agent.id, item.target.id, 'preview')
      return next
    })) onnotice(`Preview queued for ${targets.length} agent ${targets.length === 1 ? 'scope' : 'scopes'} on ${machine.host}. Installed files stay unchanged until you approve.`)
  }
  async function updateAgent(agent: SyncAgent, version: string = agentUpdateTarget(agent)) {
    if (await run(`update-${agent.id}`, () => updateSyncAgent(agent.id, version))) {
      const switching = (version === 'dev') !== (agent.version === 'dev')
      const target = version === 'dev' ? 'the development build' : 'the latest release'
      onnotice(`${switching ? 'Switch' : 'Update'} to ${target} queued for ${agent.host}. The agent verifies the release, keeps the previous binary, then restarts and reports back.`)
    }
  }
  async function cancelUpdate(job: SyncJob) {
    const host = machineOf(job)?.host
    if (await run(`cancel-${job.id}`, () => controlSyncJob(job.id, 'cancel'))) onnotice(`Update cancelled for ${host ?? 'the machine'}. It stays on its current build.`)
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
      body: 'This removes inventory reports and saved sync references. It does not revoke enrollment or terminal access. Installed files and running shells stay; a future scan can report it again.',
      confirmLabel: 'Forget machine',
      danger: true,
      action: async () => { if (await run(`forget-${machine.host}`, () => forgetMachine(machine.host))) onnotice(`Forgot ${machine.host}. Installed files are unchanged.`) },
    }
  }
  function askForgetGroup(group: SyncEnvironment, machine: Machine) {
    confirm = {
      title: `Forget ${groupLabel(group)} on ${machine.host}?`,
      body: 'This removes this agent report and its saved sync reference. Installed files and shelf entries stay. A future scan can report it again.',
      confirmLabel: 'Forget agent scope',
      danger: true,
      action: async () => { if (await run(`forget-${group.id}`, () => forgetEnvironment(group.id))) onnotice(`Forgot ${groupLabel(group)}. Installed files are unchanged.`) },
    }
  }
  function askRevoke(agent: SyncAgent) {
    confirm = {
      title: `Revoke live access for ${agent.host}?`,
      body: 'This closes private access and prevents new terminal authorization and sync jobs. Independent local tmux shells keep running. A running sync operation may finish; installed files remain.',
      confirmLabel: 'Revoke enrollment',
      danger: true,
      action: async () => { if (await run(`revoke-${agent.id}`, () => controlSyncAgent(agent.id, 'revoke'))) onnotice(`${agent.host} is no longer enrolled.`) },
    }
  }
</script>

{#snippet privateSkillsAction(group: SyncEnvironment, reasonId: string)}
  {@const entry = privateSkills(group)}
  {#if entry}
    <div class="private-skills">
      <button type="button" class="btn" disabled={!entry.ok} aria-label={`Private skills for ${groupLabel(group)}`} data-private-skills={group.id} aria-describedby={entry.ok ? undefined : reasonId} onclick={(event) => (contentSource = { environmentId: group.id, opener: event.currentTarget })}>Private skills</button>
      {#if !entry.ok}<small id={reasonId}>{entry.reason}</small>{/if}
    </div>
  {/if}
{/snippet}

<svelte:window onclick={(event) => { if (!(event.target as Element | null)?.closest('.sync-menu')) closeMenus() }} onkeydown={menusKey} />

<section class="fleet" aria-label="Machines">
  <div class="machine-grid">
    {#each machines as machine, m (machine.host)}
      {@const enrolled = agentFor(machine)}
      {@const groups = agentGroups(machine)}
      {@const supported = supportedAgents(machine)}
      {@const scanOnly = scanOnlyAgents(machine)}
      {@const state = machineStatus(machine)}
      {@const sync = machineSyncOverview(machine, snapshot)}
      <article class="machine" class:just-added={highlight === machine.host} aria-labelledby={`machine-${machine.host}`}>
        <header class="machine-head">
          <div class="identity">
            <span class="dot" class:current={!!enrolled && enrolled.lastSeen > Date.now() - 90000} class:offline={!!enrolled && enrolled.lastSeen <= Date.now() - 90000} aria-hidden="true"></span>
            <div>
              <h3 id={`machine-${machine.host}`} title={machine.host}>{machine.host}</h3>
              {#if highlight === machine.host}<p class="added-note">Just added</p>{/if}
              {#if machine.aliases?.length}<p class="alias">Also known as {machine.aliases.join(', ')}</p>{:else}<p class="alias">Machine inventory and agent scopes</p>{/if}
            </div>
          </div>
          <span class="badge {state}">{stateLabel(machine)}</span>
        </header>

        <div class="sync-band {sync.tone}">
          <div class="sync-copy">
            <span class="sync-pulse" class:live={sync.tone === 'working'} aria-hidden="true"></span>
            <div><strong>{sync.label}</strong><small>{sync.detail}</small></div>
          </div>
          <div class="sync-cta">
            {#if sync.canSync}
              <button class="btn primary sync-now" type="button" disabled={!!busy || !!enrolled?.paused} onclick={(event) => { closeMenu(event); if (enrolled) void syncNow(machine, enrolled) }}>{busy === `sync-${enrolled?.id}` ? 'Starting…' : 'Sync now'}</button>
            {:else if sync.tone === 'working'}
              <a class="btn sync-now" href="/sync#sync-jobs">View progress</a>
            {:else}
              <a class="btn sync-now" href={sync.href}>{sync.tone === 'setup' ? 'Set up sync' : 'Open sync'}</a>
            {/if}
            <details class="sync-menu">
              <summary aria-label={`More options for ${machine.host}`} title="More options"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="5" cy="12" r="1"></circle><circle cx="12" cy="12" r="1"></circle><circle cx="19" cy="12" r="1"></circle></svg></summary>
              <div class="sync-menu-panel" role="menu">
                <a role="menuitem" href={`/sync?machine=${encodeURIComponent(machine.host)}`}>Choose skills &amp; scopes</a>
                {#if enrolled && savedTargets(machine).length}<button type="button" role="menuitem" disabled={!!busy || enrolled.paused || sync.tone === 'working'} onclick={(event) => { closeMenu(event); void previewNow(machine, enrolled) }}>Preview changes only</button>{/if}
                {#if enrolled}<button type="button" role="menuitem" disabled={!!busy} onclick={(event) => { closeMenu(event); void scan(enrolled) }}>Rescan machine</button>{/if}
                <button type="button" role="menuitem" onclick={(event) => { closeMenu(event); commandsFor = machine }}>Commands…</button>
                {#if canWrite}<button type="button" role="menuitem" onclick={(event) => { closeMenu(event); settingsHost = machine.host }}>Settings…</button>{/if}
                <a role="menuitem" href="/sync#sync-jobs">Review sync activity</a>
              </div>
            </details>
          </div>
          {#if sync.progress}
            <div class="sync-track" aria-hidden="true"><span style:width={`${Math.round((sync.progress.done / Math.max(1, sync.progress.total)) * 100)}%`}></span></div>
          {/if}
        </div>
        {#if enrolled&&health.machines.some(item=>item.agentId===enrolled.id)}<MachineHealthStrip compact health={health.machines.find(item=>item.agentId===enrolled.id)} serverNow={health.serverNow+healthElapsed} />{/if}

        <div class="meta">
          <span><strong>{machine.skillCount}</strong> unique {machine.skillCount === 1 ? 'skill' : 'skills'}</span>
          <span><strong>{groups.length}</strong> agent {groups.length === 1 ? 'scope' : 'scopes'}</span>
          <span>Last report <strong title={lastReport(machine) ? new Date(lastReport(machine)).toLocaleString() : undefined}>{lastSeen(machine)}</strong></span>
          {#if enrolled}<span class="live {agentTone(enrolled)}"><span aria-hidden="true">●</span> {agentState(enrolled)}</span><span class="build" title={enrolled.commit ? `commit ${enrolled.commit}` : undefined}>reet {agentBuild(enrolled)}</span>{/if}
        </div>
        <div class="agent-chips" aria-label="Agent scopes">
          {#each supported as group (group.id)}<span class="agent-chip">{groupLabel(group)} <strong>{group.skillCount ?? 0}</strong></span>{/each}
          {#each scanOnly as group (group.id)}<span class="agent-chip muted">{groupLabel(group)}</span>{/each}
        </div>

        {#if enrolled}
          <AgentUpdate
            agent={enrolled}
            job={lastUpdate(machine)}
            releases={snapshot.releases}
            {canWrite}
            busy={!!busy}
            onreview={() => openReview(enrolled, enrolled.version === 'dev' ? 'dev' : 'latest')}
            oncancel={() => { const update = lastUpdate(machine); if (update) void cancelUpdate(update) }}
          />
        {/if}

        {#if terminalAccess && enrolled}
          <div class="actions">
            <a class="btn" class:primary={sync.tone === 'synced' || sync.tone === 'setup'} href={`/machines?view=terminal&agent=${encodeURIComponent(enrolled.id)}`}>Open terminal</a>
            <button class="btn" type="button" disabled={!!agentReadiness(enrolled.id)} aria-describedby={agentReadiness(enrolled.id) ? `start-agent-${m}` : undefined} onclick={(event) => (startAgent = { agent: enrolled.id, opener: event.currentTarget })}>Start an agent</button>
          </div>
        {/if}
        {#if terminalAccess&&enrolled&&agentReadiness(enrolled.id)}<p class="start-reason" id={`start-agent-${m}`}>{agentReadiness(enrolled.id)}</p>{/if}

        <details class="agent-list">
          <summary>
            <span>Agents on this machine</span>
            <span>{groups.length} {groups.length === 1 ? 'scope' : 'scopes'}</span>
          </summary>
          <ul>
            {#each supported as group, i (group.id)}
              <li>
                <span class="dot {group.complete && fresh(group) ? 'current' : group.complete ? 'stale' : 'partial'}" aria-hidden="true"></span>
                <div class="agent-copy">
                  <strong>{groupLabel(group)}</strong>
                  <small>{group.skillCount ?? 0} {group.skillCount === 1 ? 'skill' : 'skills'} · {group.complete ? 'Last full report' : 'Incomplete report'} {new Date(group.scannedAt || group.lastSeen).toLocaleString()}</small>
                  {@render privateSkillsAction(group, `private-skills-${m}-${i}`)}
                  <ContentObservation content={group.content} onimport={canWrite && enrolled ? (name) => onimport({ environmentId: group.id, name }) : undefined} onpin={pinOpener(group)} />
                </div>
                <span class="scope-state">{groupState(group)}</span>
              </li>
            {/each}
            {#each scanOnly as group, i (group.id)}
              <li class="scan-only">
                <span class="dot empty" aria-hidden="true"></span>
                <div class="agent-copy"><strong>{groupLabel(group)}</strong><small>Scan only. Sync to the shared agents folder or this agent's supported location.</small>{@render privateSkillsAction(group, `private-skills-${m}-scan-${i}`)}<ContentObservation content={group.content} onimport={canWrite && enrolled ? (name) => onimport({ environmentId: group.id, name }) : undefined} onpin={pinOpener(group)} /></div>
                <span class="scope-state">Scan only</span>
              </li>
            {/each}
            {#if groups.length === 0}<li class="empty-row">No agent inventory yet. Run a scan, then refresh.</li>{/if}
          </ul>
          {#if canWrite && groups.length}
            <div class="agent-links">
              <button type="button" class="btn quiet" onclick={() => (commandsFor = machine)}>Copy rescan command</button>
            </div>
            <details class="danger-zone">
              <summary>Forget a report…</summary>
              <div class="danger-actions">
                {#each supported as group (group.id)}
                  <button type="button" class="btn danger" onclick={() => askForgetGroup(group, machine)}>Forget {groupLabel(group)}…</button>
                {/each}
              </div>
              <p>Revoke live access and forget the whole machine in Settings.</p>
            </details>
          {/if}
        </details>
      </article>
    {/each}
  </div>
</section>

{#if startAgent && terminalAccess && terminals}<StartAgentDialog machines={terminals} snapshot={snapshot.machines ?? []} initialAgent={startAgent.agent} opener={startAgent.opener} onstarted={agentStarted} onleavefailed={agentLeaveFailed} onaddmachine={addFromStart(startAgent)} {onsignedout} onclose={() => (startAgent = null)} />{/if}

{#if contentSource}<MachineSkillContent {snapshot} source={contentSource} opener={contentSource.opener} note={contentSource.note ?? ''} onaddmachine={addFromPrivate(contentSource)} {canWrite} {accounts} {onsignedout} onclose={() => contentSource = null} />{/if}

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
    releases={snapshot.releases}
    busy={!!busy}
    onautoupdate={(enabled) => { if (enrolled) void toggleAutoUpdate(enrolled, enabled) }}
    onpause={() => { if (enrolled) void pause(enrolled) }}
    onscan={() => { if (enrolled) void scan(enrolled) }}
    onswitch={() => { if (enrolled) openReview(enrolled, enrolled.version === 'dev' ? 'latest' : 'dev') }}
    onrevoke={revokeFromSettings}
    onforget={forgetFromSettings}
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
    confirmLabel={confirm.confirmLabel}
    danger={confirm.danger}
    onconfirm={() => { const action = confirm?.action; confirm = null; void action?.() }}
    oncancel={() => (confirm = null)}
  />
{/if}

<style>
  .fleet { margin: 14px 0 0; }
  .machine-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 430px), 1fr)); gap: 14px; }
  .added-note { margin: 2px 0 0; color: var(--good); font-size: .78rem; font-weight: 650; }
  .machine.just-added { border-color: var(--good); animation: just-added 6s ease-out; }
  @keyframes just-added { from { background: color-mix(in srgb, var(--good) 22%, var(--card)); box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 45%, transparent); } to { background: var(--card); box-shadow: var(--shadow); } }
  @media (prefers-reduced-motion: reduce) { .machine.just-added { animation: none; box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 45%, transparent); } }
  .machine { display: grid; gap: 14px; padding: 18px; border: 1px solid var(--line); border-radius: 14px; background: var(--card); box-shadow: var(--shadow); min-width: 0; }
  .machine-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .identity { display: flex; align-items: flex-start; gap: 10px; min-width: 0; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; margin-top: 5px; background: var(--muted); }
  .dot.offline { background: transparent; border: 2px solid var(--muted); }
  .dot.current { background: var(--good); }
  .dot.stale { background: var(--warn, #b45309); }
  .dot.partial { background: var(--bad); }
  .dot.empty { background: var(--line); }
  h3 { margin: 0; font: 500 1.35rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .alias { margin: 3px 0 0; color: var(--muted); font-size: 0.72rem; overflow-wrap: anywhere; }
  .badge { flex: none; padding: 4px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.68rem; }
  .badge.current { color: var(--good); border-color: color-mix(in srgb, var(--good) 35%, var(--line)); }
  .badge.stale { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, var(--line)); }
  .badge.partial { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); }
  .meta { display: flex; flex-wrap: wrap; gap: 6px 14px; color: var(--muted); font-size: 0.75rem; }
  .meta strong { color: var(--ink); font-weight: 600; }
  .live.online { color: var(--good); }
  .live.paused { color: var(--warn); }
  .live.offline { color: var(--muted); }
  .build { color: var(--ink); }
  .sync-band { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: color-mix(in srgb, var(--bg) 52%, var(--card)); }
  .sync-copy { display: flex; align-items: flex-start; gap: 9px; min-width: 0; flex: 1 1 220px; }
  .sync-copy > div { display: grid; gap: 1px; min-width: 0; }
  .sync-copy strong { font-size: 0.8rem; overflow-wrap: anywhere; }
  .sync-copy small { color: var(--muted); font-size: 0.72rem; line-height: 1.45; overflow-wrap: anywhere; }
  .sync-pulse { width: 9px; height: 9px; border-radius: 50%; flex: none; margin-top: 5px; background: var(--muted); }
  .sync-band.action { border-color: color-mix(in srgb, var(--accent) 32%, var(--line)); }
  .sync-band.action .sync-pulse { background: var(--accent); }
  .sync-band.review { border-color: color-mix(in srgb, var(--warn) 32%, var(--line)); }
  .sync-band.review .sync-pulse { background: var(--warn); }
  .sync-band.attention { border-color: color-mix(in srgb, var(--bad) 30%, var(--line)); }
  .sync-band.attention .sync-pulse { background: var(--bad); }
  .sync-band.synced .sync-pulse { background: var(--good); }
  .sync-band.setup .sync-pulse { background: var(--line-strong); }
  .sync-pulse.live { background: var(--accent); animation: sync-pulse 1.4s ease-in-out infinite; }
  @keyframes sync-pulse { 50% { opacity: 0.35; transform: scale(0.8); } }
  .sync-cta { display: flex; align-items: center; gap: 6px; flex: none; }
  .sync-now { min-height: 36px; padding: 6px 12px; font-size: 0.78rem; }
  .sync-menu { position: relative; }
  .sync-menu > summary { list-style: none; display: grid; place-items: center; width: 38px; height: 38px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card); color: var(--muted); cursor: pointer; }
  .sync-menu > summary::-webkit-details-marker { display: none; }
  .sync-menu > summary:hover, .sync-menu[open] > summary { color: var(--ink); border-color: var(--muted); }
  .sync-menu > summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .sync-menu > summary svg { width: 18px; height: 18px; }
  .sync-menu-panel { position: absolute; z-index: 20; right: 0; top: calc(100% + 6px); display: grid; min-width: 214px; padding: 6px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised); box-shadow: var(--shadow); }
  .sync-menu-panel a, .sync-menu-panel button { display: block; width: 100%; padding: 8px 10px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; font-size: 0.78rem; text-align: left; text-decoration: none; cursor: pointer; }
  .sync-menu-panel a:hover, .sync-menu-panel button:hover:not(:disabled) { background: var(--tint); }
  .sync-menu-panel button:disabled { color: var(--muted); cursor: default; }
  .sync-track { flex-basis: 100%; height: 4px; overflow: hidden; border-radius: 999px; background: var(--line); }
  .sync-track span { display: block; height: 100%; background: linear-gradient(90deg, var(--accent), var(--info)); transition: width 0.4s var(--ease-out); }
  .agent-chips { display: flex; flex-wrap: wrap; gap: 6px; }
  /* A scope's label can carry a long folder path with no spaces: let it break anywhere so it never widens the page. */
  .agent-chip { min-width: 0; max-width: 100%; padding: 4px 8px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--muted); font-size: 0.69rem; overflow-wrap: anywhere; }
  .agent-chip strong { color: var(--ink); font-weight: 600; }
  .agent-chip.muted { opacity: 0.78; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .start-reason { margin: -6px 0 0; color: var(--muted); font-size: 0.75rem; line-height: 1.5; overflow-wrap: anywhere; }
  .agent-list { border-top: 1px solid var(--line); padding-top: 12px; }
  .agent-list > summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 34px; cursor: pointer; font-size: 0.8rem; font-weight: 650; }
  .agent-list > summary span:last-child { color: var(--muted); font-weight: 400; }
  .agent-list > summary::after { content: ''; width: 7px; height: 7px; flex: none; margin-right: 4px; border-right: 2px solid var(--muted); border-bottom: 2px solid var(--muted); transform: rotate(45deg); transition: transform 160ms ease; }
  .agent-list[open] > summary::after { transform: rotate(-135deg); }
  .agent-list ul { list-style: none; display: grid; gap: 0; padding: 0; margin: 8px 0 0; }
  .agent-list li { display: flex; align-items: flex-start; gap: 9px; padding: 10px 0; border-top: 1px solid var(--line); }
  .agent-copy { display: grid; gap: 2px; min-width: 0; flex: 1; }
  .agent-copy strong { font-size: 0.8rem; overflow-wrap: anywhere; }
  .agent-copy small { color: var(--muted); font-size: 0.72rem; line-height: 1.45; overflow-wrap: anywhere; }
  .private-skills { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; margin: 4px 0 2px; }
  .private-skills .btn { min-height: 34px; padding: 5px 12px; font-size: 0.78rem; }
  .private-skills small { flex: 1 1 200px; min-width: 0; }
  .scope-state { flex: none; color: var(--muted); font-size: 0.7rem; }
  .scan-only { opacity: 0.82; }
  .empty-row { color: var(--muted); font-size: 0.8rem; }
  .agent-links { display: flex; flex-wrap: wrap; gap: 6px 12px; margin-top: 8px; }
  .danger-zone { margin-top: 8px; border-top: 1px solid var(--line); padding-top: 8px; }
  .danger-zone > summary { cursor: pointer; color: var(--muted); font-size: 0.76rem; min-height: 32px; }
  .danger-actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 6px 0; }
  .danger-zone p { margin: 0; color: var(--muted); font-size: 0.74rem; }
    @media (max-width: 560px) {
    .machine { padding: 14px; }
    .machine-head { flex-direction: column; align-items: flex-start; }
    .actions .btn { flex: 1 1 auto; }
    .sync-cta { width: 100%; }
    .sync-cta .btn { flex: 1 1 auto; }
  }
  /* Touch: every control on a card is a finger-sized (44 px) target. */
  @media (pointer: coarse) {
    .sync-now { min-height: 44px; }
    .sync-menu > summary { width: 44px; height: 44px; }
    .sync-menu-panel a, .sync-menu-panel button { min-height: 44px; }
    .agent-list > summary { min-height: 44px; }
    .machine :global(.updater .btn) { min-height: 44px; }
  }
  @media (prefers-reduced-motion: reduce) {
    .sync-pulse.live { animation: none; }
    .sync-track span { transition: none; }
  }
</style>
