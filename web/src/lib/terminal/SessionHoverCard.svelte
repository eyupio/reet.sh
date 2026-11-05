<script lang="ts">
  import type { MachineHealth, TerminalSession } from './api'
  import { age, bytes, fresh, percent, value } from './health'
  import { shortAge, stateWord } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import { projectColourHex } from '../projectColour'
  import { toolName } from '../agentLaunch'

  // The card a session row shows on hover or keyboard focus: what the session is, where it runs, the machine's last
  // resource readings and the way in. It repeats the row's own label rather than replacing it, so it is presentational;
  // the row stays the control. Readings are never shown as live when the report is stale.
  let { session, host, project, colour = '', health, serverNow, href }: {
    session: TerminalSession
    host: string
    project?: string
    colour?: string
    health?: MachineHealth
    serverNow: number
    href: string
  } = $props()

  const word = $derived(stateWord(session))
  const report = $derived(health?.connection?.report)
  const isFresh = $derived(fresh(health, serverNow))
  const cpu = $derived(value(report?.cpuPercent))
  const memory = $derived(percent(report?.memory.totalBytes, report?.memory.availableBytes))
  const disk = $derived(value(report?.disk.availableBytes))
  const diskTotal = $derived(value(report?.disk.totalBytes))
  const diskUsed = $derived(disk != null && diskTotal != null && diskTotal > 0 ? (diskTotal - disk) / diskTotal * 100 : null)
  const observed = $derived(isFresh ? `Updated ${age(health?.connection?.reportAt, serverNow)}` : report ? `Last known · ${age(health?.connection?.reportAt, serverNow)}` : 'No resource readings yet')
  const reading = (n: number | null, suffix = '%') => n == null ? '—' : `${n.toFixed(n < 10 ? 1 : 0)}${suffix}`
</script>

<div class="session-card" role="presentation" aria-hidden="true" style={`--card-colour:${projectColourHex(colour)}`}>
  <header>
    <span class="swatch" class:empty={!projectColourHex(colour) || projectColourHex(colour) === 'transparent'}></span>
    <strong>{session.title}</strong>
    <span class="state" data-status={sessionStatus(session)}>{word.word}</span>
  </header>
  <p class="where">{project ? `${project} · ` : ''}{host}{#if session.worktreeBranch}{' · '}<code>{session.worktreeBranch}</code>{/if}</p>
  <p class="age">{['running', 'creating', 'end_requested'].includes(session.state) ? `Running for ${shortAge(Math.max(0, serverNow - session.createdAt))}` : `Changed ${shortAge(Math.max(0, serverNow - session.updatedAt))} ago`}</p>
  {#if session.tool || session.model}
    <p class="agent">{toolName(session.tool)}{session.model ? ` · ${session.model}` : ''}</p>
  {/if}
  <dl class="resources">
    <div class="cpu"><dt>CPU</dt><dd>{cpu == null ? '—' : reading(cpu)}</dd><span class="meter" aria-hidden="true"><span style:width={`${Math.max(0, Math.min(100, cpu ?? 0))}%`}></span></span></div>
    <div class="memory"><dt>Memory</dt><dd>{reading(memory)}</dd><span class="meter" aria-hidden="true"><span style:width={`${Math.max(0, Math.min(100, memory ?? 0))}%`}></span></span></div>
    <div class="disk"><dt>Disk</dt><dd>{disk == null ? '—' : bytes(disk)}{#if diskUsed != null}<small>{' '}free</small>{/if}</dd><span class="meter" aria-hidden="true"><span style:width={`${Math.max(0, Math.min(100, diskUsed ?? 0))}%`}></span></span></div>
  </dl>
  <footer><span class:stale={!isFresh}>{observed}</span><span class="jump">Open session →</span></footer>
</div>

<style>
  .session-card { width: 320px; max-width: calc(100vw - 16px); padding: 12px 14px; border: 1px solid var(--line-strong); border-left: 3px solid var(--card-colour, transparent); border-radius: 12px; background: var(--card-raised); color: var(--ink); box-shadow: 0 18px 40px -18px var(--scrim); font-size: 0.875rem; line-height: 1.4; }
  header { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .swatch { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--card-colour); }
  .swatch.empty { display: none; }
  header strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.875rem; }
  .state { flex: none; font-size: 0.875rem; padding: 1px 7px; border: 1px solid var(--status-edge); border-radius: 99px; color: var(--status); background: var(--status-wash); font-weight: 600; }
  .where { margin: 6px 0 0; color: var(--muted); overflow-wrap: anywhere; }
  code { font-family: var(--mono); font-size: 0.875rem; }
  .age { margin: 2px 0 0; color: var(--muted); font-size: 0.875rem; }
  .agent { margin: 2px 0 0; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .resources { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin: 10px 0 0; padding-top: 10px; border-top: 1px solid var(--line); }
  .resources > div { display: grid; gap: 3px; min-width: 0; }
  dt { color: var(--muted); font-size: 0.875rem; letter-spacing: .05em; text-transform: uppercase; }
  dd { margin: 0; font: 600 .88rem/1.2 var(--mono); font-variant-numeric: tabular-nums; }
  dd small { font: 400 0.875rem var(--sans); color: var(--muted); }
  .meter { display: block; height: 4px; border-radius: 2px; background: var(--line-soft); overflow: hidden; }
  .meter > span { display: block; height: 100%; border-radius: 2px; background: var(--accent); }
  footer { display: flex; align-items: baseline; gap: 8px; margin-top: 10px; font-size: 0.875rem; color: var(--muted); }
  footer .stale { color: var(--warn); }
  .jump { margin-left: auto; color: var(--accent-text); font-weight: 600; white-space: nowrap; }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
