<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, type SyncState } from './api'
  import { inventoryMachines } from './machines'
  import PageHeader from './PageHeader.svelte'
  import MachineFleet from './MachineFleet.svelte'
  import FleetActions from './FleetActions.svelte'
  import ActiveSessions from './terminal/ActiveSessions.svelte'
  let { canWrite, terminalAccess, monitorAccess }: { canWrite: boolean; terminalAccess: boolean; monitorAccess: boolean } = $props()
  let snapshot = $state<SyncState>({ environments: [], targets: [] })
  let loading = $state(true), error = $state(''), notice = $state(''), received = $state(0), now = $state(Date.now())
  let refreshing = false, disposed = false, generation = 0
  const machines = $derived(inventoryMachines(snapshot.machines ?? []))
  const online = $derived((snapshot.agents ?? []).filter(agent => !agent.paused && agent.lastSeen > now - 90000).length)
  const skills = $derived(new Set(snapshot.environments.flatMap(env => env.skills ?? []).map(skill => skill.id)).size)
  const waiting = $derived((snapshot.jobs ?? []).filter(job => job.status === 'awaiting-approval'))
  const failed = $derived((snapshot.jobs ?? []).filter(job => job.status === 'failed'))
  function update(next: SyncState) { ++generation; snapshot = next; received = Date.now() }
  async function refresh() {
    if (refreshing) return
    refreshing = true
    const current = ++generation
    try { const next = await getSyncState(); if (!disposed && generation === current) { snapshot = next; received = Date.now(); error = '' } }
    catch (e) { if (!disposed && generation === current) error = e instanceof Error ? e.message : String(e) }
    finally { if (!disposed) loading = false; refreshing = false }
  }
  onMount(() => {
    void refresh()
    const timer = setInterval(() => { now = Date.now(); if (document.visibilityState === 'visible') void refresh() }, 5000)
    return () => { disposed = true; clearInterval(timer) }
  })
</script>
<PageHeader eyebrow="Ey up. Everything in hand." title="Your machines. One control centre." lede="Keep skills on reet.sh, sync them across your machines, and open a terminal wherever the work is.">
  {#snippet actions()}<a class="btn" href="/skills">Skill library</a>{#if canWrite}<a class="btn primary" href="/machines#connect-machine">Connect a machine</a>{/if}{/snippet}
</PageHeader>
{#if terminalAccess}<ActiveSessions />{/if}
<nav class="workflow" aria-label="Control workflow"><a href="/skills"><span>01</span><strong>Maintain skills</strong><small>Your central library</small></a><b aria-hidden="true">↔</b><a href="/sync"><span>02</span><strong>Choose &amp; sync</strong><small>Reet ↔ machine ↔ machine</small></a><b aria-hidden="true">→</b><a href={terminalAccess ? '/terminals' : '/machines'}><span>03</span><strong>Put them to work</strong><small>{terminalAccess ? 'Open a remote terminal' : 'Inspect your machines'}</small></a></nav>
{#if error}<div class="notice bad" role="alert">{error} <button class="btn" onclick={() => void refresh()}>Retry</button></div>{/if}
{#if notice}<p role="status">{notice}</p>{/if}
{#if loading}<p role="status">Loading your control centre…</p>{:else if received}
  <div class="metrics" aria-label="Fleet overview"><a href="/machines"><strong>{machines.length}</strong><span>Machines with inventory</span><small>{online} enrolled agents reporting recently</small></a><a href="/skills"><strong>{skills}</strong><span>Skills reported</span><small>Unique across machine inventories</small></a><a href="/sync"><strong>{snapshot.targets.length}</strong><span>Saved sync scopes</span><small>Each has its own desired skills</small></a><a href="/sync#sync-jobs" class:attention={waiting.length > 0}><strong>{waiting.length}</strong><span>Plans awaiting approval</span><small>{failed.length ? `${failed.length} failed jobs to inspect` : 'You decide what gets applied'}</small></a></div>
  {#if waiting.length || failed.length}<aside class="attention-panel"><div><strong>{waiting.length ? `${waiting.length} sync plans need your review` : `${failed.length} sync jobs need attention`}</strong><p>Inspect each machine’s proposed changes and resolve conflicts before applying.</p></div><a class="btn" href="/sync#sync-jobs">Review sync activity →</a></aside>{/if}
  <FleetActions {snapshot} {canWrite} onupdate={update} />
  {#if machines.length}
    <MachineFleet {machines} {snapshot} site={location.origin} {canWrite} {terminalAccess} {monitorAccess} onadd={() => location.assign('/machines#connect-machine')} onupdate={update} onerror={e => error = e instanceof Error ? e.message : String(e)} onnotice={message => notice = message} />
  {:else}<section class="first-machine"><p class="eyebrow">Start with one machine. Grow from there.</p><h2>Bring your toolkit together.</h2><p>Connect your first machine to discover its skills. Add the ones you want to reet.sh, then choose what your next machine receives.</p>{#if canWrite}<a class="btn primary" href="/machines#connect-machine">Connect your first machine</a>{/if}<a class="btn" href="/skills">Explore your skill library</a></section>{/if}
  <div class="secondary"><a href="/hosting"><strong>Create your own skills ↗</strong><span>Maintain instructions and supporting files on reet.sh.</span></a><a href="/profiles"><strong>Portable agent setups ↗</strong><span>Capture and restore supported configuration files.</span></a><div><strong>Agent maintenance <span class="badge">Planned</span></strong><span>Install, update and remove agent software across machines. Current controls manage skills and terminals.</span></div></div>
  <p class="updated">Last received {new Date(received).toLocaleTimeString()}. Inventories describe scans; they do not establish file or version equality.</p>
{/if}
<style>
  .workflow{display:grid;grid-template-columns:1fr auto 1fr auto 1fr;align-items:center;gap:12px;border:1px solid var(--line);border-radius:14px;background:var(--card);padding:20px}.workflow a{display:grid;grid-template-columns:auto 1fr;gap:2px 12px;color:var(--ink);text-decoration:none;padding:6px;border-radius:6px}.workflow a:hover{background:var(--tint)}.workflow a>span{grid-row:span 2;align-self:center;color:var(--accent);font: .75rem var(--mono);padding:8px;border:1px solid var(--line);border-radius:50%}.workflow strong{font-size:.85rem}.workflow small,.workflow b{color:var(--muted)}.workflow small{font-size:.72rem}.metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:1px;border:1px solid var(--line);border-radius:12px;overflow:hidden;background:var(--line);margin:24px 0}.metrics a{display:grid;gap:4px;padding:22px 18px;background:var(--card);color:var(--ink);text-decoration:none}.metrics a:hover{background:var(--tint)}.metrics strong{font:500 2.2rem var(--serif)}.metrics span{font-size:.8rem;font-weight:600}.metrics small{font-size:.7rem;color:var(--muted)}.metrics .attention strong{color:var(--accent)}.attention-panel{display:flex;justify-content:space-between;align-items:center;gap:16px;flex-wrap:wrap;border-left:3px solid var(--accent);padding:16px 20px;background:var(--tint);border-radius:8px}.attention-panel p{font-size:.8rem;margin:4px 0;color:var(--muted)}.first-machine{padding:40px 24px;text-align:center;border:1px dashed var(--line);border-radius:14px;background:var(--card)}h2{font:500 1.9rem var(--serif);margin:8px 0 12px}.first-machine p:not(.eyebrow){max-width:65ch;margin:0 auto 20px;color:var(--muted);font-size:.9rem}.first-machine .btn{margin:4px}.eyebrow{color:var(--accent);font-size:.65rem;text-transform:uppercase;letter-spacing:.12em}.secondary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px;margin-top:28px;border-top:1px solid var(--line);padding-top:24px}.secondary>a,.secondary>div{display:grid;align-content:start;gap:8px;color:var(--ink);text-decoration:none}.secondary strong{font-size:.82rem}.secondary span:not(.badge){font-size:.76rem;line-height:1.6;color:var(--muted)}.secondary a:hover strong{color:var(--accent)}.secondary .badge{font-weight:400;padding:2px 7px;font-size:.62rem}.updated{font-size:.7rem;color:var(--muted);margin-top:24px}.notice{padding:12px;border:1px solid var(--bad);border-radius:8px;margin:20px 0;overflow-wrap:anywhere}.bad{color:var(--bad)}
  @media(max-width:800px){.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.workflow{gap:4px;padding:12px}.workflow a{grid-template-columns:1fr}.workflow a>span{display:none}.secondary{grid-template-columns:1fr}.workflow small{font-size:.65rem}}
  @media(max-width:460px){.workflow{grid-template-columns:1fr}.workflow b{display:none}.workflow a{grid-template-columns:auto 1fr;padding:10px}.workflow a>span{display:block}.metrics a{padding:16px 12px}.metrics strong{font-size:1.8rem}}
</style>
