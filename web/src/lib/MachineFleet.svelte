<script lang="ts">
  import { onMount } from 'svelte'
  import type { HealthResponse } from './terminal/api'
  import { getMachineHealth as machineHealth } from './api'
  import MachineHealthStrip from './terminal/MachineHealthStrip.svelte'
  import type { Machine, SyncAgent, SyncEnvironment, SyncState } from './api'
  import { controlSyncAgent, forgetEnvironment, forgetMachine, queueSyncJob } from './api'
  import { agentGroups, groupLabel } from './machines'
  import { machineStatus, scanOnlyAgents, supportedAgents, syncLink } from './machineSetup'
  import MachineCommands from './MachineCommands.svelte'
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
      <p class="eyebrow">Your machines</p>
      <h2 id="fleet-title">{machines.length} {machines.length === 1 ? 'machine' : 'machines'}, one clear view.</h2>
      <p>Each machine is one unit. Its agent scopes live inside it, so you do not have to think in folders.</p>
    </div>
    <button class="btn" type="button" onclick={onadd}>Add a machine</button>
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
          {#if enrolled}<span class="live"><span aria-hidden="true">●</span> {agentState(enrolled)} · {enrolled.version}</span>{/if}
        </div>
        <div class="agent-chips" aria-label="Agent scopes">
          {#each supported as group (group.id)}<span class="agent-chip">{groupLabel(group)} <strong>{group.skillCount ?? 0}</strong></span>{/each}
          {#each scanOnly as group (group.id)}<span class="agent-chip muted">{groupLabel(group)}</span>{/each}
        </div>
        {#if enrolled&&health.machines.some(item=>item.agentId===enrolled.id)}<MachineHealthStrip compact health={health.machines.find(item=>item.agentId===enrolled.id)} serverNow={health.serverNow+healthElapsed} />{/if}

        <div class="actions">
          <a class="btn primary" href={syncLink(machine.host)}>Sync this machine</a>
          {#if terminalAccess&&enrolled}<a class="btn" href={`/machines?view=terminal&agent=${encodeURIComponent(enrolled.id)}`}>Open terminal</a>{/if}
          <button class="btn" type="button" onclick={() => (commandsFor = machine)}>Commands</button>
          {#if canWrite}
            <details class="manage">
              <summary>Manage</summary>
              <div class="manage-menu">
                {#if enrolled}
                  <button type="button" disabled={!!busy || enrolled.paused} onclick={() => scan(enrolled)}>{busy === `scan-${enrolled.id}` ? 'Queueing…' : 'Scan now'}</button>
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
                </div>
                <span class="scope-state">{groupState(group)}</span>
              </li>
            {/each}
            {#each scanOnly as group (group.id)}
              <li class="scan-only">
                <span class="dot empty" aria-hidden="true"></span>
                <div class="agent-copy"><strong>{groupLabel(group)}</strong><small>Scan only. Sync to the shared agents folder or this agent's supported location.</small></div>
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

{#if commandsFor}
  <MachineCommands
    machine={commandsFor.host}
    {site}
    enrolled={!!agentFor(commandsFor)}
    agentStatus={agentFor(commandsFor) ? `${agentState(agentFor(commandsFor)!)} · last seen ${agentSeen(agentFor(commandsFor)!)}` : ''}
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
