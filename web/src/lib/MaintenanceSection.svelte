<script lang="ts">
  import { onMount } from 'svelte'
  import { cleanWorktrees, maintenanceState, scanWorktrees, setWorktreeMaintenance, TerminalError, type MaintenanceState, type SweepItem } from './terminal/api'
  import { cleanable, itemKey, needsReview, sweepReason } from './maintenance'
  import { elapsed } from './time'
  import WorktreeDialog from './WorktreeDialog.svelte'
  import type { RunMember } from './projects'

  // Machine maintenance. Today: the worktrees of finished sessions. A sweep lists them with what the machine found; Clean
  // removes the ones with nothing unsaved. Worktrees with unsaved work are never removed here: Review opens the worktree's own
  // dialog, which warns about the changes and whether a pull request exists. The schedule is a safety net with the same rule.
  let { admin = false, onsignedout }: { admin?: boolean; onsignedout?: (e: unknown) => void } = $props()

  let settings = $state<MaintenanceState | null>(null)
  let items = $state<SweepItem[] | null>(null)
  let more = $state(false)
  let picked = $state<Record<string, boolean>>({})
  let busy = $state<'' | 'loading' | 'scanning' | 'cleaning' | 'saving'>('loading')
  let message = $state(''), failed = $state(false)
  let reviewing = $state<{ item: SweepItem; opener: HTMLElement | null } | null>(null)
  const life = new AbortController()

  const safe = $derived(items ? cleanable(items) : [])
  const review = $derived(items ? needsReview(items) : [])
  const other = $derived(items ? items.filter(i => i.status === 'unreachable') : [])
  const chosen = $derived(safe.filter(i => picked[itemKey(i)]))
  const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

  function fail(e: unknown, fallback: string) {
    if (life.signal.aborted) return
    if (e instanceof TerminalError && e.status === 401 && onsignedout) return onsignedout(e)
    failed = true
    message = e instanceof Error && e.message ? e.message : fallback
  }

  onMount(() => {
    maintenanceState(life.signal).then(s => { settings = s }).catch(e => fail(e, "Couldn't read the maintenance settings.")).finally(() => { busy = '' })
    return () => life.abort()
  })

  async function scan() {
    if (busy) return
    busy = 'scanning'; failed = false; message = 'Asking your machines about finished sessions’ worktrees…'
    try {
      const out = await scanWorktrees(settings?.worktrees.minAgeDays ?? 7, life.signal)
      items = out.items; more = out.more
      picked = Object.fromEntries(cleanable(out.items).map(i => [itemKey(i), true]))
      message = out.items.length === 0 ? 'Nothing to clean up.' : `Found ${plural(out.items.length, 'worktree', 'worktrees')}.`
    } catch (e) { fail(e, "Couldn't scan just now.") } finally { if (!life.signal.aborted) busy = '' }
  }

  async function clean() {
    if (busy || chosen.length === 0) return
    busy = 'cleaning'; failed = false; message = `Removing ${plural(chosen.length, 'worktree', 'worktrees')}…`
    try {
      const out = await cleanWorktrees(chosen.slice(0, 50), life.signal)
      const done = new Map(out.items.map(i => [itemKey(i), i]))
      const removed = out.items.filter(i => i.removed).length
      items = (items ?? []).filter(i => !done.get(itemKey(i))?.removed).map(i => done.get(itemKey(i)) ?? i)
      message = `Removed ${plural(removed, 'worktree', 'worktrees')}. Their branches are kept.${removed < out.items.length ? ` ${out.items.length - removed} could not be removed and are listed below.` : ''}`
      failed = removed < out.items.length
    } catch (e) { fail(e, "Couldn't clean up just now.") } finally { if (!life.signal.aborted) busy = '' }
  }

  async function save(change: Parameters<typeof setWorktreeMaintenance>[0]) {
    if (busy) return
    busy = 'saving'; failed = false
    try { settings = await setWorktreeMaintenance(change, life.signal); message = 'Saved.' } catch (e) { fail(e, "Couldn't save that.") } finally { if (!life.signal.aborted) busy = '' }
  }

  const memberOf = (i: SweepItem): RunMember => ({ position: 0, host: i.agentId, dir: '', tool: '', model: '', state: 'started', code: '', sentence: '', sessionId: i.sessionId,
    worktreePath: i.path, worktreeRepo: i.repo, worktreeBranch: i.branch, worktreeRun: i.run, worktreeProject: i.project, sharesCheckout: false, attempt: 0 })
  const age = (i: SweepItem) => i.endedAt ? `ended ${elapsed(i.endedAt)}` : ''
</script>

<section class="maint" aria-labelledby="maint-title">
  <h2 id="maint-title">Maintenance</h2>
  <p class="lede">Housekeeping for your machines. It removes only what you have already saved, and keeps every branch.</p>

  <div class="card">
    <h3>Worktrees</h3>
    <p>Each session can work in its own copy of a repository. They add up. Clean up the ones whose sessions have finished.</p>
    <div class="row">
      <button type="button" class="btn primary" onclick={scan} disabled={!!busy}>{busy === 'scanning' ? 'Scanning…' : items ? 'Scan again' : 'Scan for worktrees'}</button>
      {#if safe.length}<button type="button" class="btn danger" onclick={clean} disabled={!!busy || chosen.length === 0}>{busy === 'cleaning' ? 'Removing…' : `Clean ${chosen.length} selected`}</button>{/if}
    </div>
    <p class="status" class:bad={failed} role="status" aria-live="polite">{message}</p>

    {#if items}
      {#if safe.length}
        <h4>Safe to remove</h4>
        <ul class="list">{#each safe as i (itemKey(i))}
          <li><label><input type="checkbox" bind:checked={picked[itemKey(i)]} disabled={!!busy} />
            <span><b>{i.title || i.branch}</b> <small>{i.host} · {i.project} · {age(i)}</small><small class="mono">{i.path}</small>
            <small>{sweepReason(i)}{i.unpushed ? ` ${plural(i.unpushed, 'commit stays', 'commits stay')} on the branch.` : ''}</small></span></label></li>
        {/each}</ul>
      {/if}
      {#if review.length}
        <h4>Needs your review</h4>
        <ul class="list">{#each review as i (itemKey(i))}
          <li class="review"><span><b>{i.title || i.branch}</b> <small>{i.host} · {i.project} · {age(i)}</small><small class="mono">{i.path}</small><small class="bad">{sweepReason(i)}</small></span>
            <button type="button" class="btn" onclick={e => (reviewing = { item: i, opener: e.currentTarget as HTMLElement })}>Review…</button></li>
        {/each}</ul>
      {/if}
      {#if other.length}<p class="hint">{plural(other.length, 'worktree was', 'worktrees were')} skipped because {other.length === 1 ? 'its machine' : 'their machines'} couldn't be asked. Scan again when {other.length === 1 ? 'it is' : 'they are'} online.</p>{/if}
      {#if more}<p class="hint">There are more. Clean these, then scan again.</p>{/if}
    {/if}
  </div>

  <div class="card">
    <h3>Scheduled clean-up</h3>
    <p>A safety net. On a schedule, reet removes the worktrees of finished sessions that hold nothing unsaved. Anything with uncommitted changes, unreferenced commits or other doubts is left for you to review. Every removal is in Activity.</p>
    {#if settings}
      <label class="line"><input type="checkbox" checked={settings.worktrees.enabled} disabled={!admin || !!busy} onchange={e => void save({ enabled: e.currentTarget.checked })} /> <span>Clean up worktrees automatically</span></label>
      <div class="line">
        <label>Every <select disabled={!admin || !!busy} value={String(settings.worktrees.intervalHours)} onchange={e => void save({ intervalHours: Number(e.currentTarget.value) })}>
          {#each [[6, '6 hours'], [24, 'day'], [168, 'week']] as [h, label] (h)}<option value={String(h)}>{label}</option>{/each}</select></label>
        <label>for sessions finished over <select disabled={!admin || !!busy} value={String(settings.worktrees.minAgeDays)} onchange={e => void save({ minAgeDays: Number(e.currentTarget.value) })}>
          {#each [[0, 'any time'], [1, '1 day'], [7, '7 days'], [30, '30 days']] as [d, label] (d)}<option value={String(d)}>{label} ago</option>{/each}</select></label>
      </div>
      {#if !admin}<p class="hint">Only an owner or admin can change the schedule.</p>{/if}
      {#if settings.lastRun}<p class="hint">Last run {elapsed(settings.lastRun.at)}: removed {settings.lastRun.removed}, {settings.lastRun.review} to review{settings.lastRun.unreachable ? `, ${settings.lastRun.unreachable} on machines that were offline` : ''}.</p>
      {:else if settings.worktrees.enabled}<p class="hint">It hasn't run yet.</p>{/if}
    {/if}
  </div>
</section>

{#if reviewing}
  <WorktreeDialog member={memberOf(reviewing.item)} host={reviewing.item.host} opener={reviewing.opener} onremoved={() => { const k = itemKey(reviewing!.item); items = (items ?? []).filter(i => itemKey(i) !== k) }} onsignedout={onsignedout} onclose={() => (reviewing = null)} />
{/if}

<style>
  .maint { display: grid; gap: 16px; }
  h2 { margin: 0; font: 500 1.5rem/1.2 var(--serif); }
  h3 { margin: 0 0 6px; font-size: 1.05rem; }
  h4 { margin: 14px 0 6px; font-size: .9rem; color: var(--muted); }
  .lede, .card p { color: var(--muted); font-size: .88rem; line-height: 1.5; margin: 0 0 10px; }
  .card { padding: 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .row, .line { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; margin: 8px 0; }
  .line select { margin: 0 4px; }
  .status { min-height: 1.4em; }
  .bad { color: var(--bad); }
  .hint { font-size: .8rem; }
  .list { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .list li { padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .list label { display: flex; gap: 10px; align-items: flex-start; cursor: pointer; }
  .list input { margin-top: 3px; }
  .review { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
  .list span { display: grid; gap: 2px; min-width: 0; overflow-wrap: anywhere; }
  .list small { color: var(--muted); font-size: .78rem; }
  .list small.bad { color: var(--bad); }
  .mono { font-family: var(--mono); }
</style>
