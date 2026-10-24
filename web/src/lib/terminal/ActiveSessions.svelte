<script lang="ts">
  import {getProfile} from '../api'
  import TerminalCardActions from './TerminalCardActions.svelte'
  import { onMount } from 'svelte'
  import { terminalMachines, terminalSessions, TerminalError, type TerminalMachine, type TerminalSession } from './api'
  let sessions = $state<TerminalSession[]>([]), machines = $state<TerminalMachine[]>([])
  let loading = $state(true), busy = $state(false), error = $state(''), checked = $state(0), now = $state(performance.now())
  let generation=0,removeDefault=$state(false)
  let disposed = false, request: AbortController | undefined
  const active = $derived(sessions.filter(item => ['creating','running','end_requested'].includes(item.state)))
  const stale = $derived(!!error || checked > 0 && now - checked > 45000)
  function host(agent: string) { return machines.find(item => item.agentId === agent)?.host || 'Unavailable machine' }
  async function refresh() {
    if (busy || disposed) return
    busy = true;const observation=++generation
    const abort = new AbortController(); request = abort
    const timeout = setTimeout(() => abort.abort(), 10000)
    try {
      const [nextSessions, nextMachines] = await Promise.all([terminalSessions(abort.signal), terminalMachines(abort.signal)])
      if (disposed||observation!==generation) return
      sessions = nextSessions; machines = nextMachines; error = ''; checked = performance.now(); now = checked
    } catch (e) {
      if (disposed||observation!==generation) return
      if (e instanceof TerminalError && [401,403].includes(e.status)) { sessions = []; machines = []; checked = 0 }
      error = 'Session status is unavailable. Refresh to check your running shells.'
    } finally { clearTimeout(timeout); if (!disposed&&observation===generation) { loading = false; busy = false } }
  }
  function changed(){generation++;request?.abort();busy=false;void refresh()}
  onMount(() => {
    void getProfile().then(data=>{if(!disposed)removeDefault=data.profile.terminalRemoveOnClose??false}).catch(()=>{})
    void refresh()
    const wake = () => { now = performance.now(); if (document.visibilityState === 'visible') void refresh() }
    const poll = setInterval(wake, 15000), clock = setInterval(() => now = performance.now(), 1000)
    document.addEventListener('visibilitychange', wake); window.addEventListener('online', wake)
    return () => { disposed = true; request?.abort(); clearInterval(poll); clearInterval(clock); document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake) }
  })
</script>
<section class="active-sessions" id="active-sessions" aria-labelledby="active-sessions-title" aria-busy={loading}>
  <header><div><p class="eyebrow">Pick up where you left off</p><h2 id="active-sessions-title">{loading ? 'Checking your sessions…' : checked ? `${active.length} active ${active.length === 1 ? 'session' : 'sessions'}` : 'Your terminal sessions'}{#if stale && checked}<span class="badge">Last known</span>{/if}</h2></div><div class="actions"><button class="btn quiet" disabled={busy} onclick={() => void refresh()} aria-label="Refresh active sessions">Refresh</button><a class="btn primary" href="/terminals">Open workspace →</a></div></header>
  {#if error}<p class="status" role="status">{error}{#if checked} Showing the last received session list.{/if}</p>{/if}
  {#if active.length}<ul aria-label="Active terminal sessions">{#each active as item (item.id)}
    {@const machine = machines.find(machine => machine.agentId === item.agentId)}
    <li><TerminalCardActions machine={machine||{agentId:item.agentId,host:host(item.agentId),ready:false}} session={item} disabled={stale||machine?.serverEnabled===false} {removeDefault} ondefault={value=>removeDefault=value} onrefresh={changed}>{#snippet children(open,controls)}
    <div class="session-card" role="group" aria-label={`${item.title} on ${host(item.agentId)}`} oncontextmenu={open}><a href={`/terminals?agent=${encodeURIComponent(item.agentId)}&session=${encodeURIComponent(item.id)}`} aria-label={`Continue ${item.title} on ${host(item.agentId)}`}><span class="session-title"><span class="dot" class:pending={item.state !== 'running' || stale} aria-hidden="true"></span><strong>{item.title}</strong><span aria-hidden="true">↗</span></span><span class="host">{host(item.agentId)}</span><span class="state">{item.state === 'running' ? 'Running shell' : item.state === 'creating' ? 'Starting shell' : 'End requested'} · {stale ? 'Last known' : item.state === 'end_requested' ? 'Awaiting confirmation' : item.state === 'creating' ? 'Waiting for host' : machine?.ready ? 'Ready to reconnect' : 'Needs connection or setup'}</span></a><div class="session-actions">{@render controls()}</div></div>{/snippet}</TerminalCardActions></li>
  {/each}</ul>{:else if !loading && checked && !error}<p class="status">No active shells. Open the workspace to start a terminal; closing your browser keeps it running.</p>{/if}
</section>
<style>
  .active-sessions{margin:0 0 24px;border:1px solid color-mix(in srgb,var(--accent) 35%,var(--line));border-radius:14px;background:var(--card);overflow:hidden}header{padding:18px 20px;display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}.eyebrow{margin:0 0 4px;color:var(--accent);font-size:.65rem;letter-spacing:.1em;text-transform:uppercase}h2{display:flex;align-items:center;gap:10px;margin:0;font:500 1.5rem var(--serif)}.actions{display:flex;gap:6px}.actions .btn{font-size:.75rem;padding:7px 10px;min-height:36px}ul{list-style:none;display:flex;gap:10px;overflow-x:auto;scroll-snap-type:x proximity;padding:0 20px 18px;margin:0}.session-card{display:flex;align-items:center;gap:4px;border:1px solid var(--line);border-radius:10px;padding:8px;min-width:0;height:100%;width:100%}li{flex:0 0 290px;min-width:0;scroll-snap-align:start}.session-card>a{display:grid;gap:6px;flex:1;min-width:0;border:1px solid transparent;border-radius:6px;padding:6px;color:var(--ink);text-decoration:none}.session-card>a:hover{border-color:var(--accent);background:var(--tint)}.session-title{display:flex;align-items:center;gap:8px;min-width:0}.session-title strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:.85rem}.session-title>span:last-child{margin-left:auto;color:var(--accent)}.dot{height:7px;width:7px;flex:none;border-radius:50%;background:var(--good)}.dot.pending{background:var(--warn)}.host{font:.74rem var(--mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.state{font-size:.69rem;color:var(--muted)}.status{padding:0 20px 16px;margin:0;color:var(--muted);font-size:.78rem}.badge{font:500 .65rem var(--sans);padding:3px 7px}@media(max-width:600px){header{padding:14px;gap:10px}.actions{margin-left:auto}h2{font-size:1.3rem}ul{padding:0 14px 14px}li{flex-basis:280px}.session-actions{flex:none}.status{padding:0 14px 14px}.actions .btn{min-height:36px}}
</style>
