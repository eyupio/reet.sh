<script lang="ts">
  import { onMount } from 'svelte'
  import type { HealthResponse } from './terminal/api'
  import { getMachineHealth as machineHealth } from './api'
  import MachineHealthStrip from './terminal/MachineHealthStrip.svelte'
  import type { Machine, SyncAgent, SyncEnvironment, SyncJob, SyncState } from './api'
  import { controlSyncAgent, forgetEnvironment, forgetMachine, queueSyncJob, startSyncRun, updateSyncAgent } from './api'
  import { agentBuild, agentGroups, agentUpdateTarget, groupLabel, latestRelease, updateAvailable } from './machines'
  import { machineStatus, scanOnlyAgents, supportedAgents, syncLink } from './machineSetup'
  import MachineCommands from './MachineCommands.svelte'
  import ContentObservation from './ContentObservation.svelte'
  import MachineSkillImports from './MachineSkillImports.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let {
    machines,
    snapshot,
    site,
    canWrite,
    terminalAccess=false,
    monitorAccess=false,
    onadd,
    onupdate,
    onerror,
    onnotice,
  }: {
    machines: Machine[]
    snapshot: SyncState
    site: string
    canWrite: boolean
    terminalAccess?:boolean
    monitorAccess?:boolean
    onadd: () => void
    onupdate: (state: SyncState) => void
    onerror: (error: unknown) => void
    onnotice: (message: string) => void
  } = $props()

  type Pending = { title: string; body: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> }
  let commandsFor = $state<Machine | null>(null)
  let confirm = $state<Pending | null>(null)
  let busy = $state('')
  let importSource = $state<{ environmentId: string; name: string } | null>(null)
  let health=$state<HealthResponse>({serverNow:Date.now(),machines:[]}), healthElapsed=$state(0),healthReceived=performance.now()
  onMount(()=>{if(!monitorAccess)return;const abort=new AbortController();let busy=false;async function refreshHealth(){if(busy||document.visibilityState!=='visible')return;busy=true;try{const next=await machineHealth(abort.signal);if(!abort.signal.aborted){health=next;healthReceived=performance.now();healthElapsed=0}}catch{/* inventory remains usable when health is unavailable */}finally{busy=false}}void refreshHealth();const wake=()=>void refreshHealth();document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);const timer=setInterval(wake,15000),clock=setInterval(()=>healthElapsed=performance.now()-healthReceived,1000);return()=>{abort.abort();clearInterval(timer);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}})

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
    return at ? new Date(at).toLocaleString() : 'No report yet'
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
  function latestFor(agent: SyncAgent) {
    return latestRelease(agent, snapshot.releases)
  }
  function hasUpdate(agent: SyncAgent): boolean {
    return updateAvailable(agent, snapshot.releases)
  }
  function lastUpdate(machine: Machine): SyncJob | undefined {
    const agent = agentFor(machine)
    if (!agent) return
    return (snapshot.jobs ?? []).filter((job) => job.agentId === agent.id && job.kind === 'update').sort((a, b) => b.createdAt - a.createdAt)[0]
  }
  function updateJobLabel(job: SyncJob): string {
    const when = new Date(job.createdAt).toLocaleString()
    switch (job.status) {
      case 'queued': return `Update queued ${when}`
      case 'running': return 'Update in progress'
      case 'awaiting-approval': return 'Update waiting for the agent'
      case 'succeeded': return `Updated to ${job.revision || 'latest'} ${when}`
      case 'failed': return `Update failed: ${job.message || 'unknown error'}`
      case 'cancelled': return 'Update cancelled'
      default: return `Update ${job.status}`
    }
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
  function activeRun(machine: Machine) {
    const agent = agentFor(machine)
    return agent ? (snapshot.runs ?? []).find(run => run.agentId === agent.id && ['running', 'review'].includes(run.status)) : undefined
  }
  async function syncNow(machine: Machine, agent: SyncAgent) {
    if (await run(`sync-${agent.id}`, () => startSyncRun(agent.id, savedTargets(machine).map(item => item.target.id)))) {
      onnotice(`Automatic sync started for ${machine.host}. Additive plans apply on their own; removals wait for review.`)
    }
  }
  async function updateAgent(agent: SyncAgent) {
    const version = agentUpdateTarget(agent)
    if (await run(`update-${agent.id}`, () => updateSyncAgent(agent.id, version))) {
      onnotice(`Update to ${version} queued for ${agent.host}. The agent verifies the release, keeps the previous binary, then restarts.`)
    }
  }
  async function toggleAutoUpdate(agent: SyncAgent, enabled: boolean) {
    if (await run(`auto-update-${agent.id}`, () => controlSyncAgent(agent.id, enabled ? 'auto-update-on' : 'auto-update-off'))) {
      onnotice(enabled
        ? `Auto-update enabled for ${agent.host}. The controller will move it to the latest ${agentUpdateTarget(agent)} build automatically.`
        : `Auto-update disabled for ${agent.host}. Manual updates still work.`)
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

<section class="fleet" aria-labelledby="fleet-title">
  <header class="fleet-head">
    <div>
      <p class="eyebrow">Connected workspace</p>
      <h2 id="fleet-title">{machines.length} {machines.length === 1 ? 'machine' : 'machines'}, one clear view.</h2>
      <p>Choose a machine to sync its skills or continue working in its terminal. Expand a card to inspect its agent scopes.</p>
    </div>
    <div class="fleet-controls">{#if canWrite}<button type="button" onclick={() => importSource = {environmentId: '', name: ''}}>Private skill imports</button>{/if}<button class="btn" type="button" onclick={onadd}>Connect a machine</button></div>
  </header>

  <div class="machine-grid">
    {#each machines as machine (machine.host)}
      {@const enrolled = agentFor(machine)}
      {@const groups = agentGroups(machine)}
      {@const supported = supportedAgents(machine)}
      {@const scanOnly = scanOnlyAgents(machine)}
      {@const state = machineStatus(machine)}
      <article class="machine" aria-labelledby={`machine-${machine.host}`}>
        <header class="machine-head">
          <div class="identity">
            <span class="dot {state}" aria-hidden="true"></span>
            <div>
              <h3 id={`machine-${machine.host}`}>{machine.host}</h3>
              {#if machine.aliases?.length}<p class="alias">Also known as {machine.aliases.join(', ')}</p>{:else}<p class="alias">Machine inventory and agent scopes</p>{/if}
            </div>
          </div>
          <span class="badge {state}">{stateLabel(machine)}</span>
        </header>

        <div class="meta">
          <span><strong>{machine.skillCount}</strong> unique {machine.skillCount === 1 ? 'skill' : 'skills'}</span>
          <span><strong>{groups.length}</strong> agent {groups.length === 1 ? 'scope' : 'scopes'}</span>
          <span>Last report <strong>{lastSeen(machine)}</strong></span>
          {#if enrolled}<span class="live"><span aria-hidden="true">●</span> {agentState(enrolled)}</span><span class="build" title={enrolled.commit ? `commit ${enrolled.commit}` : undefined}>reet {agentBuild(enrolled)}</span>{#if hasUpdate(enrolled)}<span class="update-available">Update available</span>{/if}{/if}
          {#if activeRun(machine)}{@const run = activeRun(machine)!}<span class="sync-run">Sync run {run.cursor}/{run.targets.length}{run.status === 'review' ? ' · review needed' : ''}</span>{/if}
        </div>
        <div class="agent-chips" aria-label="Agent scopes">
          {#each supported as group (group.id)}<span class="agent-chip">{groupLabel(group)} <strong>{group.skillCount ?? 0}</strong></span>{/each}
          {#each scanOnly as group (group.id)}<span class="agent-chip muted">{groupLabel(group)}</span>{/each}
        </div>
        {#if enrolled&&health.machines.some(item=>item.agentId===enrolled.id)}<MachineHealthStrip compact health={health.machines.find(item=>item.agentId===enrolled.id)} serverNow={health.serverNow+healthElapsed} />{/if}

        {#if enrolled}
          {@const latest = latestFor(enrolled)}
          {@const update = hasUpdate(enrolled)}
          {@const updateJob = lastUpdate(machine)}
          <div class="release-strip" aria-label="reet updates">
            <div class="release-copy">
              {#if latest?.error}
                <span class="release-error">Release check failed</span>
              {:else if latest}
                <span title={latest.commit ? `commit ${latest.commit}` : undefined}>Latest {latest.label}</span>
              {/if}
              {#if update}<span class="release-badge">Update available</span>{/if}
            </div>
            {#if canWrite}
              <div class="release-controls">
                <label class="auto-update" title="Install new reet builds for this machine automatically">
                  <input type="checkbox" checked={enrolled.autoUpdate} disabled={!!busy} onchange={(event) => toggleAutoUpdate(enrolled, event.currentTarget.checked)} />
                  Auto-update
                </label>
                <button type="button" class="link" disabled={!!busy || enrolled.paused} onclick={() => updateAgent(enrolled)}>{busy === `update-${enrolled.id}` ? 'Updating…' : update ? `Update to ${agentUpdateTarget(enrolled)}` : 'Update reet'}</button>
              </div>
            {:else}
              <span class="auto-update">Auto-update {enrolled.autoUpdate ? 'on' : 'off'}</span>
            {/if}
            {#if updateJob}<span class="release-job" class:bad={updateJob.status === 'failed'} class:good={updateJob.status === 'succeeded'}>{updateJobLabel(updateJob)}</span>{/if}
          </div>
        {/if}

        <div class="actions">
          {#if enrolled && savedTargets(machine).length}
            <button class="btn primary" type="button" disabled={!!busy || enrolled.paused} onclick={() => syncNow(machine, enrolled)}>{busy === `sync-${enrolled.id}` ? 'Starting…' : savedTargets(machine).length > 1 ? 'Sync saved skills' : 'Sync this machine'}</button>
          {:else}
            <a class="btn primary" href={syncLink(machine.host)}>Choose skills to sync</a>
          {/if}
          {#if terminalAccess&&enrolled}<a class="btn" href={`/machines?view=terminal&agent=${encodeURIComponent(enrolled.id)}`}>Open terminal</a>{/if}
          <button class="btn" type="button" onclick={() => (commandsFor = machine)}>Commands</button>
          {#if canWrite}
            <details class="manage">
              <summary>Manage</summary>
              <div class="manage-menu">
                {#if enrolled}
                  <button type="button" disabled={!!busy || enrolled.paused} onclick={() => scan(enrolled)}>{busy === `scan-${enrolled.id}` ? 'Queueing…' : 'Scan now'}</button>
                  {#if savedTargets(machine).length}<button type="button" disabled={!!busy || enrolled.paused} onclick={() => syncNow(machine, enrolled)}>Sync saved skills</button>{/if}
                  <button type="button" disabled={!!busy} onclick={() => updateAgent(enrolled)}>{busy === `update-${enrolled.id}` ? 'Queueing update…' : 'Update reet'} <small>{agentUpdateTarget(enrolled)}</small></button>
                  <button type="button" disabled={!!busy} onclick={() => pause(enrolled)}>{enrolled.paused ? 'Resume sync jobs' : 'Pause sync jobs'}</button>
                {/if}
                <button type="button" class="danger" onclick={() => askForgetMachine(machine)}>Forget machine</button>
                {#if enrolled}<button type="button" onclick={() => askRevoke(enrolled)}>Revoke live access</button>{/if}
              </div>
            </details>
          {/if}
        </div>

        <details class="agent-list">
          <summary>
            <span>Agents on this machine</span>
            <span>{groups.length} {groups.length === 1 ? 'scope' : 'scopes'}</span>
          </summary>
          <ul>
            {#each supported as group (group.id)}
              <li>
                <span class="dot {group.complete && fresh(group) ? 'current' : group.complete ? 'stale' : 'partial'}" aria-hidden="true"></span>
                <div class="agent-copy">
                  <strong>{groupLabel(group)}</strong>
                  <small>{group.skillCount ?? 0} {group.skillCount === 1 ? 'skill' : 'skills'} · {group.complete ? 'Last full report' : 'Incomplete report'} {new Date(group.scannedAt || group.lastSeen).toLocaleString()}</small>
                  <ContentObservation content={group.content} onimport={canWrite && enrolled ? (name) => importSource = { environmentId: group.id, name } : undefined} />
                </div>
                <span class="scope-state">{groupState(group)}</span>
              </li>
            {/each}
            {#each scanOnly as group (group.id)}
              <li class="scan-only">
                <span class="dot empty" aria-hidden="true"></span>
                <div class="agent-copy"><strong>{groupLabel(group)}</strong><small>Scan only. Sync to the shared agents folder or this agent's supported location.</small><ContentObservation content={group.content} onimport={canWrite && enrolled ? (name) => importSource = { environmentId: group.id, name } : undefined} /></div>
                <span class="scope-state">Scan only</span>
              </li>
            {/each}
            {#if groups.length === 0}<li class="empty-row">No agent inventory yet. Run a scan, then refresh.</li>{/if}
          </ul>
          {#if canWrite && groups.length}
            <div class="agent-links">
              <button type="button" class="link" onclick={() => (commandsFor = machine)}>Copy rescan command</button>
              {#if enrolled}<button type="button" class="link" onclick={() => askRevoke(enrolled)}>Revoke live access</button>{/if}
              {#each supported as group (group.id)}
                <button type="button" class="link quiet" onclick={() => askForgetGroup(group, machine)}>Forget {groupLabel(group)}</button>
              {/each}
            </div>
          {/if}
        </details>
      </article>
    {/each}
  </div>
</section>

{#if importSource}<MachineSkillImports {snapshot} source={importSource} onclose={() => importSource = null} />{/if}

{#if commandsFor}
  <MachineCommands
    machine={commandsFor.host}
    {site}
    enrolled={!!agentFor(commandsFor)}
    agentStatus={agentFor(commandsFor) ? `${agentState(agentFor(commandsFor)!)} · reet ${agentBuild(agentFor(commandsFor)!)} · last seen ${agentSeen(agentFor(commandsFor)!)}` : ''}
    onclose={() => (commandsFor = null)}
  />
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
  .fleet { margin: 26px 0; }
  .fleet-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; flex-wrap: wrap; margin-bottom: 16px; }
  .eyebrow { margin: 0 0 4px; color: var(--accent); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  h2 { margin: 0; font: 500 clamp(1.5rem, 2.8vw, 1.85rem)/1.15 var(--serif); letter-spacing: -0.025em; }
  .fleet-controls { display: flex; flex-wrap: wrap; gap: 8px; }
  .fleet-head p:last-child { margin: 6px 0 0; max-width: 62ch; color: var(--muted); font-size: 0.83rem; line-height: 1.55; }
  .machine-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 430px), 1fr)); gap: 14px; }
  .machine { display: grid; gap: 14px; padding: 18px; border: 1px solid var(--line); border-radius: 14px; background: var(--card); box-shadow: var(--shadow); min-width: 0; }
  .machine-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .identity { display: flex; align-items: flex-start; gap: 10px; min-width: 0; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; margin-top: 5px; background: var(--muted); }
  .dot.current { background: var(--good); }
  .dot.stale { background: var(--warn, #b45309); }
  .dot.partial { background: var(--bad); }
  .dot.empty { background: var(--line); }
  h3 { margin: 0; font: 500 1.35rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .alias { margin: 3px 0 0; color: var(--muted); font-size: 0.72rem; overflow-wrap: anywhere; }
  .badge { flex: none; padding: 4px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.68rem; }
  .badge.current { color: var(--good); border-color: color-mix(in srgb, var(--good) 35%, var(--line)); }
  .badge.stale { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 35%, var(--line)); }
  .badge.partial { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); }
  .meta { display: flex; flex-wrap: wrap; gap: 6px 14px; color: var(--muted); font-size: 0.75rem; }
  .meta strong { color: var(--ink); font-weight: 600; }
  .live { color: var(--good); }
  .build { color: var(--ink); }
  .update-available { color: var(--accent); font-weight: 600; }
  .release-strip { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 12px; padding: 9px 11px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); color: var(--muted); font-size: 0.72rem; }
  .release-copy { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; min-width: 0; }
  .release-badge { padding: 2px 7px; border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line)); border-radius: 999px; color: var(--accent); }
  .release-error { color: var(--bad); }
  .release-controls { display: flex; align-items: center; gap: 10px; }
  .auto-update { display: inline-flex; align-items: center; gap: 5px; cursor: pointer; }
  .release-job { flex-basis: 100%; color: var(--muted); }
  .release-job.bad { color: var(--bad); }
  .release-job.good { color: var(--good); }
  .sync-run { color: var(--accent); }
  .agent-chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .agent-chip { padding: 4px 8px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--muted); font-size: 0.69rem; }
  .agent-chip strong { color: var(--ink); font-weight: 600; }
  .agent-chip.muted { opacity: 0.78; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .manage { position: relative; }
  .manage > summary { list-style: none; display: inline-flex; align-items: center; min-height: 42px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; color: var(--muted); font-size: 0.82rem; cursor: pointer; }
  .manage > summary::-webkit-details-marker { display: none; }
  .manage > summary:hover { color: var(--ink); border-color: var(--muted); }
  .manage-menu { position: absolute; right: 0; top: calc(100% + 5px); z-index: 12; display: grid; min-width: 190px; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); box-shadow: 0 12px 28px rgb(0 0 0 / 0.14); }
  .manage-menu button { border: 0; border-radius: 7px; padding: 9px 10px; background: none; color: var(--ink); text-align: left; font: inherit; font-size: 0.8rem; cursor: pointer; }
  .manage-menu button:hover { background: var(--tint); }
  .manage-menu .danger { color: var(--bad); }
  .agent-list { border-top: 1px solid var(--line); padding-top: 12px; }
  .agent-list > summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 34px; cursor: pointer; font-size: 0.8rem; font-weight: 650; }
  .agent-list > summary span:last-child { color: var(--muted); font-weight: 400; }
  .agent-list > summary::after { content: '⌄'; color: var(--muted); font-size: 0.95rem; line-height: 1; transition: transform 160ms ease; }
  .agent-list[open] > summary::after { transform: rotate(180deg); }
  .agent-list ul { list-style: none; display: grid; gap: 0; padding: 0; margin: 8px 0 0; }
  .agent-list li { display: flex; align-items: flex-start; gap: 9px; padding: 10px 0; border-top: 1px solid var(--line); }
  .agent-copy { display: grid; gap: 2px; min-width: 0; flex: 1; }
  .agent-copy strong { font-size: 0.8rem; }
  .agent-copy small { color: var(--muted); font-size: 0.72rem; line-height: 1.45; }
  .scope-state { flex: none; color: var(--muted); font-size: 0.7rem; }
  .scan-only { opacity: 0.82; }
  .empty-row { color: var(--muted); font-size: 0.8rem; }
  .agent-links { display: flex; flex-wrap: wrap; gap: 6px 12px; margin-top: 8px; }
  .link { padding: 0; border: 0; background: none; color: var(--accent); font: inherit; font-size: 0.74rem; text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
  .link.quiet { color: var(--muted); }
    @media (max-width: 560px) {
    .machine { padding: 14px; }
    .actions .btn { flex: 1 1 auto; }
    .manage { flex: 1 1 auto; }
    .manage > summary { justify-content: center; width: 100%; }
    .manage-menu { left: 0; right: auto; width: min(240px, calc(100vw - 64px)); }
  }
</style>
