<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, queueSyncJob, resolveSyncSourceConflicts, startSyncRun, updateSyncAgent, type SyncJob, type SyncSourceFix, type SyncState } from './api'
  import { agentOnline, groupLabel } from './machines'
  import { activeJobKindLabel, agoLabel, jobPlanCounts, syncJobStages } from './syncOverview'
  import SyncProblem from './SyncProblem.svelte'
  import { keepLauncherClear } from './floatingLauncher'

  let {
    snapshot = null,
    open = false,
    context = null,
    canWrite = false,
    onupdate,
    onclose,
    onerror,
  }: {
    snapshot?: SyncState | null
    open?: boolean
    context?: { host?: string; title?: string; note?: string } | null
    canWrite?: boolean
    onupdate?: (state: SyncState) => void
    onclose?: () => void
    onerror?: (error: unknown) => void
  } = $props()

  let local = $state<SyncState | null>(null)
  let error = $state('')
  let dismissed = $state(false)
  let expanded = $state(false)
  let now = $state(Date.now())
  let refreshing = false
  let wasLive = $state(false)
  let finishedAt = $state(0)
  let retrying = $state('')
  // Auto fix is two steps: read what would change, then change it once the person agrees.
  let fixing = $state('')
  let fixPlan = $state<{ jobId: string; changes: SyncSourceFix[] } | null>(null)
  let fixNote = $state<Record<string, string>>({})

  const sync = $derived(local ?? snapshot)
  const host = $derived((context?.host ?? '').toLowerCase())
  const liveJobs = $derived((sync?.jobs ?? []).filter(job => job.kind !== 'update'))
  const scopedJobs = $derived(liveJobs.filter(job => !host || hostFor(job.agentId).toLowerCase() === host))
  const activeJobs = $derived(scopedJobs.filter(job => ['queued', 'running', 'awaiting-approval'].includes(job.status)).sort((a, b) => b.updatedAt - a.updatedAt))
  const activeRuns = $derived((sync?.runs ?? []).filter(run => ['running', 'review'].includes(run.status) && (run.status !== 'review' || activeJobs.some(job => job.agentId === run.agentId && run.targets.includes(job.targetId))) && (!host || hostFor(run.agentId).toLowerCase() === host)))
  const run = $derived(activeRuns[0])
  const ordered = $derived([...scopedJobs].sort((a, b) => b.updatedAt - a.updatedAt))
  const latest = $derived(ordered[0])
  const live = $derived(activeJobs.length > 0 || activeRuns.length > 0)
  const waiting = $derived(live && activeJobs.every(job => job.status === 'queued') && !activeRuns.length)
  const recentlyFinished = $derived(finishedAt > 0 && now - finishedAt < 15000)
  const show = $derived((open || live || recentlyFinished) && !dismissed)
  const displayJobs = $derived(activeJobs.length ? activeJobs.slice(0, 2) : latest ? [latest] : [])
  const moreJobs = $derived(Math.max(0, activeJobs.length - displayJobs.length))
  const currentTarget = $derived(run && run.cursor < run.targets.length ? (sync?.targets ?? []).find(target => target.target.id === run.targets[run.cursor]) : undefined)
  const progress = $derived(run ? Math.round((run.cursor / Math.max(1, run.targets.length)) * 100) : 0)
  const hostName = $derived(context?.host || hostForRunOrJob() || '')
  const title = $derived.by(() => {
    if (context?.title && live && !activeJobs.some(job => job.status === 'awaiting-approval')) return context.title
    const suffix = hostName ? ` on ${hostName}` : ''
    if (waiting) return `Queued${suffix}`
    if (run) return `${run.status === 'review' ? 'Review needed' : 'Syncing'}${suffix}`
    if (activeJobs.length) {
      const job = activeJobs[0]
      if (job.status === 'awaiting-approval') return `Review needed${suffix}`
      return `${activeJobKindLabel(job)}${suffix}`
    }
    if (latest?.status === 'failed' || latest?.status === 'rolled_back') return `${kindLabel(latest.kind)} failed${suffix}`
    if (latest?.status === 'succeeded') return `Sync complete${suffix}`
    return 'Sync activity'
  })
  const subtitle = $derived(context?.note && live ? context.note : summaryLine())
  const doneCounts = $derived(latest ? jobPlanCounts(latest) : { installs: 0, removals: 0, total: 0 })

  function hostFor(agentId: string): string {
    return (sync?.agents ?? []).find(agent => agent.id === agentId)?.host ?? ''
  }
  function onlineFor(agentId: string): boolean {
    return agentOnline((sync?.agents ?? []).find(agent => agent.id === agentId))
  }
  function hostForRunOrJob(): string {
    const id = run?.agentId ?? activeJobs[0]?.agentId ?? latest?.agentId
    return id ? hostFor(id) : ''
  }
  function kindLabel(kind: string): string {
    return kind === 'apply' ? 'Apply' : kind === 'scan' ? 'Scan' : kind === 'preview' ? 'Preview' : kind === 'update' ? 'Update' : 'Sync'
  }
  function summaryLine(): string {
    if (waiting) return `${activeJobs.length} ${activeJobs.length === 1 ? 'job' : 'jobs'} waiting for the machine`
    if (run) return `${run.cursor} of ${run.targets.length} scopes complete${hostName ? ` · ${hostName}` : ''}`
    if (activeJobs.length > 1) return `${activeJobs.length} jobs in progress`
    if (activeJobs[0]) return `${scopeOnly(activeJobs[0])} · ${statusText(activeJobs[0])}`
    if (latest) return `${scopeOnly(latest)} · ${agoLabel(latest.updatedAt, now)}`
    return context?.host ?? 'All connected machines'
  }
  function scopeOnly(job: SyncJob): string {
    const target = (sync?.targets ?? []).find(item => item.target.id === job.targetId)
    if (target) return groupLabel(target.target)
    const env = (sync?.environments ?? []).find(item => item.id === job.targetId)
    return env ? groupLabel(env) : 'Install location'
  }
  function targetLabel(job: SyncJob): string {
    const target = (sync?.targets ?? []).find(item => item.target.id === job.targetId)
    if (target) return `${target.target.host} · ${groupLabel(target.target)}`
    const env = (sync?.environments ?? []).find(item => item.id === job.targetId)
    return env ? `${env.host} · ${groupLabel(env)}` : 'Install location'
  }
  function kindTitle(job: SyncJob): string {
    return job.kind === 'apply' ? 'Applying the approved plan' : job.kind === 'preview' ? 'Planning the exact changes' : job.kind === 'scan' ? 'Refreshing the inventory' : 'Automatic sync'
  }
  function statusText(job: SyncJob): string {
    const labels: Record<string, string> = {
      queued: 'Waiting for machine', running: activeJobKindLabel(job), 'awaiting-approval': 'Review needed',
      succeeded: 'Done', failed: 'Failed', cancelled: 'Cancelled',
    }
    return labels[job.status] ?? job.status.replaceAll('-', ' ')
  }
  function tone(job: SyncJob): string {
    if (job.status === 'succeeded') return 'ok'
    if (job.status === 'failed' || job.status === 'rolled_back') return 'bad'
    if (job.status === 'awaiting-approval') return 'warn'
    return 'work'
  }

  async function refresh() {
    if (refreshing) return
    refreshing = true
    try {
      const next = await getSyncState()
      local = next
      error = ''
      onupdate?.(next)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      refreshing = false
    }
  }

  // Toasts share this corner. Publish the dock's actual top edge, including
  // phone navigation, so collapsed and expanded activity both leave room. The
  // dock can also rise above a fixed action bar (see keepLauncherClear), which
  // moves its top edge without changing its size, so a change of style counts too.
  function reserveToastSpace(node: HTMLElement) {
    const root = document.documentElement
    const measure = () => root.style.setProperty('--sync-dock-clearance', `${window.innerHeight - node.getBoundingClientRect().top}px`)
    const observer = new ResizeObserver(measure)
    const moved = new MutationObserver(measure)
    observer.observe(node)
    moved.observe(node, { attributes: true, attributeFilter: ['style'] })
    window.addEventListener('resize', measure)
    measure()
    return { destroy() {
      observer.disconnect()
      moved.disconnect()
      window.removeEventListener('resize', measure)
      root.style.removeProperty('--sync-dock-clearance')
    } }
  }

  function toggle() {
    expanded = !expanded
  }

  function close() {
    dismissed = true
    expanded = false
    onclose?.()
  }

  // The same fix Advanced sync offers: follow the copy already on the machine, then sync again. Nothing on the machine
  // is changed. The first press only reads what would change.
  async function prepareFix(job: SyncJob) {
    if (fixing || !canWrite || !job.targetId) return
    fixing = job.id; fixNote = { ...fixNote, [job.id]: '' }
    try {
      const preview = await resolveSyncSourceConflicts([job.targetId], false)
      if (preview.changes.length) fixPlan = { jobId: job.id, changes: preview.changes }
      else fixNote = { ...fixNote, [job.id]: 'Auto fix cannot settle this one: reet holds this skill as its own copy, so there is no other source to follow. Update reet on the machine and sync again (it then skips only this skill), or remove the copy on the machine to use reet’s.' }
    } catch (e) {
      fixNote = { ...fixNote, [job.id]: e instanceof Error ? e.message : String(e) }
    } finally { fixing = '' }
  }
  async function applyFix(job: SyncJob) {
    if (fixing || !canWrite || !job.targetId) return
    fixing = job.id
    try {
      await resolveSyncSourceConflicts([job.targetId], true)
      const next = onlineFor(job.agentId) ? await startSyncRun(job.agentId, [job.targetId]) : await getSyncState()
      fixPlan = null
      local = next
      error = ''
      onupdate?.(next)
    } catch (e) {
      fixNote = { ...fixNote, [job.id]: e instanceof Error ? e.message : String(e) }
    } finally { fixing = '' }
  }

  // A fresh scan or preview is the only retry offered: an old approved plan is
  // never replayed from the dock.
  async function retry(job: SyncJob) {
    if (retrying || !canWrite) return
    retrying = job.id
    try {
      const next = job.kind === 'update'
        ? await updateSyncAgent(job.agentId, job.revision || 'latest')
        : await queueSyncJob(job.agentId, job.targetId, job.kind === 'scan' ? 'scan' : job.kind === 'apply' || job.kind === 'auto' ? 'auto' : 'preview')
      local = next
      error = ''
      onupdate?.(next)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      retrying = ''
    }
  }

  $effect(() => {
    if (!open) return
    dismissed = false
    void refresh()
  })

  $effect(() => {
    if (live) {
      wasLive = true
      finishedAt = 0
    } else if (wasLive) {
      wasLive = false
      finishedAt = Date.now()
      expanded = false
    }
  })

  onMount(() => {
    if (!snapshot) void refresh()
    const timer = setInterval(() => {
      now = Date.now()
      if (document.visibilityState === 'visible' && (open || live)) void refresh()
    }, 2000)
    return () => clearInterval(timer)
  })
</script>

{#snippet orbMark()}
  <span class="orb" class:live={live && !waiting} class:bad={!live && (latest?.status === 'failed' || latest?.status === 'rolled_back')} aria-hidden="true">
    {#if waiting}
      <span aria-hidden="true">Ⅱ</span>
    {:else if live}
      <span class="spin"></span>
    {:else if latest?.status === 'failed' || latest?.status === 'rolled_back'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M6 6l12 12M18 6 6 18"></path></svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m5 12 4 4L19 6"></path></svg>
    {/if}
  </span>
{/snippet}

{#if show}
  <aside use:reserveToastSpace use:keepLauncherClear class="sync-dock" class:working={live} class:expanded aria-label="Sync activity" aria-live="polite">
    <div class="accent" aria-hidden="true"></div>
    {#if expanded}
      <header class="dock-head">
        <div class="dock-id">
          {@render orbMark()}
          <div class="dock-copy">
            <p>{live ? 'Live sync' : 'Sync activity'}</p>
            <h2>{title}</h2>
            <small>{subtitle}</small>
          </div>
        </div>
        <div class="dock-actions">
          <button type="button" class="dock-control" aria-label="Minimise sync activity" aria-expanded="true" onclick={toggle}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="m6 15 6-6 6 6"></path></svg>
          </button>
          <button type="button" class="dock-control" aria-label="Hide sync activity" onclick={close}>×</button>
        </div>
      </header>

      {#if run}
        <div class="run" role="group" aria-label={`Sync run, ${run.cursor} of ${run.targets.length} scopes complete`}>
          <div class="run-meta"><span>{run.status === 'review' ? 'Review needed' : 'Applying changes'}</span><strong>{run.cursor} / {run.targets.length}</strong></div>
          <div class="track" role="progressbar" aria-label="Scopes synced" aria-valuemin={0} aria-valuemax={run.targets.length} aria-valuenow={run.cursor}><span style:width={`${progress}%`}></span></div>
          {#if currentTarget}<small>Next: {currentTarget.target.host} · {groupLabel(currentTarget.target)}</small>{/if}
        </div>
      {/if}

      <div class="dock-body">
        {#if error}<p class="dock-error" role="alert">{error}</p>{/if}
        {#each displayJobs as job (job.id)}
          {@const stages = syncJobStages(job)}
          {@const counts = jobPlanCounts(job)}
          <article class="job" class:done={job.status === 'succeeded'} class:failed={job.status === 'failed' || job.status === 'rolled_back'} aria-busy={['queued', 'running'].includes(job.status)}>
            <div class="job-head">
              <div><strong>{targetLabel(job)}</strong><small>{job.status === 'queued' ? 'Queued · waiting for the machine to pick up this job' : kindTitle(job)}</small></div>
              <span class="job-state {tone(job)}">{statusText(job)}</span>
            </div>
            <ol class="steps">
              {#each stages as stage (stage.key)}
                <li class={stage.state}><span class="step-dot" aria-hidden="true"></span><span>{stage.label}</span></li>
              {/each}
            </ol>
            {#if job.status === 'running' && job.stages?.some(stage => stage.status === 'running')}<p class="job-message" role="status">{job.stages.find(stage => stage.status === 'running')?.detail}</p>{/if}
            {#if counts.total}
              <p class="plan-count">{counts.installs} install{counts.installs === 1 ? '' : 's'}{counts.removals ? ` · ${counts.removals} removal${counts.removals === 1 ? '' : 's'}` : ''}</p>
              <ul class="plan-list">
                {#each (job.plan.actions ?? []).slice(0, 5) as action (action.skill.id)}
                  <li class:remove={!action.add}><span aria-hidden="true">{action.add ? '+' : '−'}</span>{action.skill.name}</li>
                {/each}
                {#if (job.plan.actions?.length ?? 0) > 5}<li class="more">+{(job.plan.actions?.length ?? 0) - 5} more</li>{/if}
              </ul>
            {:else if job.status === 'succeeded'}
              <p class="plan-count">{doneCounts.total || Object.keys(job.plan?.problems ?? {}).length ? '' : 'Everything already matches. No files needed changing.'}</p>
            {/if}
            <!-- A skill sync left alone is said here with its reason, so a finished sync never hides what it skipped. -->
            {#if Object.keys(job.plan?.problems ?? {}).length}
              <ul class="left-alone" aria-label="Skills left alone">
                {#each Object.entries(job.plan?.problems ?? {}) as [slug, reason] (slug)}<li><strong>{slug}</strong> was left alone. {reason}</li>{/each}
              </ul>
            {/if}
            {#if job.message}
              {#if job.status === 'failed' || job.status === 'rolled_back'}
                <SyncProblem message={job.message} host={hostFor(job.agentId)} site={location.origin} compact online={onlineFor(job.agentId)}
                  retrying={retrying === job.id} onretry={canWrite ? () => void retry(job) : undefined}
                  fixing={fixing === job.id} onfix={canWrite && job.targetId && fixPlan?.jobId !== job.id ? () => void prepareFix(job) : undefined} />
                {#if fixPlan?.jobId === job.id}
                  <div class="fix-confirm" role="group" aria-label="Confirm auto fix">
                    <p>Keep the copy of {fixPlan.changes.map(change => change.name).join(', ')} already on {hostFor(job.agentId)} and sync again? Nothing on the machine is changed.</p>
                    <div class="fix-actions">
                      <button class="btn primary" type="button" disabled={fixing === job.id} aria-busy={fixing === job.id} onclick={() => void applyFix(job)}>{fixing === job.id ? 'Fixing…' : 'Auto fix & sync'}</button>
                      <button class="btn quiet" type="button" disabled={fixing === job.id} onclick={() => (fixPlan = null)}>Cancel</button>
                    </div>
                  </div>
                {/if}
                {#if fixNote[job.id]}<p class="job-message" role="status">{fixNote[job.id]}</p>{/if}
              {:else}
                <p class="job-message">{job.message}</p>
              {/if}
            {/if}
            {#if job.status === 'awaiting-approval'}<a class="btn primary review" href="/sync#sync-jobs">Review changes</a>{/if}
          </article>
        {/each}
        {#if !displayJobs.length && !error}<p class="dock-empty">Waiting for the machine to report…</p>{/if}
        {#if moreJobs}<a class="more-jobs" href="/sync#sync-jobs">+{moreJobs} more {moreJobs === 1 ? 'job' : 'jobs'}</a>{/if}
      </div>

      <footer class="dock-foot">
        <span>{live ? 'Updating live' : !latest && open ? 'Starting sync…' : 'Finished'} · {agoLabel(latest?.updatedAt ?? now, now)}</span>
        <a href="/sync#sync-jobs">View all activity</a>
      </footer>
    {:else}
      <div class="collapsed">
        <button type="button" class="dock-main" aria-expanded="false" aria-label={`${title}. ${subtitle}. Open sync activity.`} onclick={toggle}>
          {@render orbMark()}
          <span class="compact-copy"><strong>{title}</strong><small>{subtitle}</small></span>
          <svg class="chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="m6 9 6 6 6-6"></path></svg>
        </button>
        <button type="button" class="dock-control close" aria-label="Hide sync activity" onclick={close}>×</button>
      </div>
    {/if}
  </aside>
{/if}

<style>
  .sync-dock {
    position: fixed; z-index: 60; right: 20px; bottom: var(--launcher-clearance, calc(20px + var(--bottom-chrome, 0px)));
    width: min(420px, calc(100vw - 32px));
    overflow: hidden; border: 1px solid var(--line-strong); border-radius: 16px;
    background: color-mix(in srgb, var(--card-raised) 94%, transparent);
    box-shadow: var(--shadow);
    backdrop-filter: blur(14px); color: var(--ink);
  }
  .sync-dock.expanded { display: grid; grid-template-rows: auto auto minmax(0, 1fr) auto; max-height: min(72vh, 660px); }
  .accent { height: 3px; background: var(--accent); }
  .working .accent { animation: glide 2.8s linear infinite; }
  @keyframes glide { to { background-position: -200% 0; } }
  .collapsed { display: flex; align-items: center; gap: 2px; padding: 6px 6px 6px 8px; }
  .dock-main { display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0; min-height: 52px; padding: 6px; border: 1px solid transparent; border-radius: 12px; background: transparent; color: inherit; text-align: left; cursor: pointer; }
  .dock-main:hover { border-color: var(--line); background: color-mix(in srgb, var(--bg) 40%, transparent); }
  .compact-copy { display: grid; gap: 1px; min-width: 0; flex: 1; }
  .compact-copy strong { font-size: 0.875rem; line-height: 1.3; overflow-wrap: anywhere; }
  .compact-copy small { color: var(--muted); font-size: 0.875rem; line-height: 1.35; overflow-wrap: anywhere; }
  .chevron { width: 18px; height: 18px; flex: none; color: var(--muted); }
  .dock-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; padding: 16px 12px 12px 16px; }
  .dock-id { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
  .dock-actions { display: flex; gap: 2px; flex: none; }
  .orb { display: grid; place-items: center; width: 34px; height: 34px; flex: none; border-radius: 50%; border: 1px solid color-mix(in srgb, var(--good) 45%, var(--line)); background: color-mix(in srgb, var(--good) 12%, var(--card)); color: var(--good); }
  .orb.live { border-color: color-mix(in srgb, var(--accent) 55%, var(--line)); background: color-mix(in srgb, var(--accent) 14%, var(--card)); color: var(--accent-text); }
  .orb.bad { border-color: color-mix(in srgb, var(--bad) 50%, var(--line)); background: color-mix(in srgb, var(--bad) 12%, var(--card)); color: var(--bad); }
  .orb svg { width: 16px; height: 16px; }
  .spin { width: 18px; height: 18px; border-radius: 50%; border: 2px solid color-mix(in srgb, var(--accent) 25%, transparent); border-top-color: var(--accent-text); display: block; }
  .working .spin { animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .dock-copy { display: grid; gap: 2px; min-width: 0; }
  .dock-copy p { margin: 0; color: var(--accent-text); font-size: 0.875rem; font-weight: 600; }
  .dock-copy h2 { margin: 0; font: 500 1.12rem/1.25 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .dock-copy small { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .dock-control { flex: none; display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 1px solid transparent; border-radius: 9px; background: transparent; color: var(--muted); font-size: 1.15rem; line-height: 1; cursor: pointer; }
  .dock-control svg { width: 18px; height: 18px; }
  .dock-control:hover { color: var(--ink); border-color: var(--line); }
  .dock-control.close { width: 44px; height: 44px; }
  .run { padding: 0 16px 12px; display: grid; gap: 6px; }
  .run-meta { display: flex; justify-content: space-between; gap: 10px; color: var(--muted); font-size: 0.875rem; }
  .run-meta strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .run small { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .track { height: 6px; overflow: hidden; border-radius: 4px; background: color-mix(in srgb, var(--line) 70%, transparent); }
  .track span { display: block; height: 100%; border-radius: inherit; background: var(--accent); transition: width 0.5s var(--ease-out); }
  .dock-body { display: grid; gap: 10px; padding: 2px 16px 12px; overflow: auto; overscroll-behavior: contain; }
  .job { display: grid; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--bg) 55%, var(--card)); }
  .job.done { border-color: color-mix(in srgb, var(--good) 32%, var(--line)); }
  .job.failed { border-color: color-mix(in srgb, var(--bad) 38%, var(--line)); }
  .job-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  .job-head > div { display: grid; gap: 1px; min-width: 0; }
  .job-head strong { font-size: 0.875rem; overflow-wrap: anywhere; }
  .job-head small { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .job-state { flex: none; padding: 2px 8px; border-radius: 6px; border: 1px solid var(--line); color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .job-state.ok { color: var(--good); border-color: color-mix(in srgb, var(--good) 40%, var(--line)); }
  .job-state.bad { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 40%, var(--line)); }
  .job-state.warn { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); }
  .job-state.work { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); }
  .steps { list-style: none; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4px; margin: 0; padding: 0; }
  .steps li { display: grid; gap: 5px; min-width: 0; color: var(--muted); font-size: 0.875rem; }
  .step-dot { display: block; height: 4px; border-radius: 4px; background: var(--line); }
  .steps li.active { color: var(--accent-text); font-weight: 600; }
  .steps li.active .step-dot { background: var(--accent); animation: pulse 1.4s ease-in-out infinite; }
  .steps li.done { color: var(--ink-soft); }
  .steps li.done .step-dot { background: var(--good); }
  .steps li.failed { color: var(--bad); }
  .steps li.failed .step-dot { background: var(--bad); }
  .steps li.review { color: var(--warn); }
  .steps li.review .step-dot { background: var(--warn); }
  @keyframes pulse { 50% { opacity: 0.45; } }
  .plan-count { margin: 0; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .plan-list { list-style: none; display: grid; gap: 3px; margin: 0; padding: 0; font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .plan-list li { display: flex; gap: 7px; align-items: baseline; min-width: 0; }
  .plan-list li span { flex: none; width: 10px; color: var(--good); font-weight: 600; }
  .plan-list li.remove span { color: var(--bad); }
  .plan-list li.more { color: var(--muted); }
  .job-message { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; overflow-wrap: anywhere; }
  .review { justify-self: start; min-height: 36px; padding: 6px 12px; font-size: 0.875rem; }
  .more-jobs { color: var(--accent-text); font-size: 0.875rem; font-weight: 600; font-variant-numeric: tabular-nums; text-decoration: none; }
  .more-jobs:hover { text-decoration: underline; }
  .dock-empty { margin: 0; padding: 8px 0; color: var(--muted); font-size: 0.875rem; }
  .dock-error { margin: 0; padding: 9px 10px; border: 1px solid color-mix(in srgb, var(--bad) 45%, var(--line)); border-radius: 8px; color: var(--bad); font-size: 0.875rem; }
  .dock-foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 16px; border-top: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 35%, var(--card)); color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .dock-foot a { color: var(--accent-text); font-weight: 600; text-decoration: none; }
  .dock-foot a:hover { text-decoration: underline; }
  @media (max-width: 650px) {
    .sync-dock { right: 12px; left: 12px; bottom: var(--launcher-clearance, calc(12px + var(--bottom-chrome, 0px))); width: auto; }
    .sync-dock.expanded { max-height: min(58dvh, 520px); }
  }
  @media (prefers-reduced-motion: reduce) {
    .working .accent, .working .spin, .steps li.active .step-dot { animation: none; }
    .track span { transition: none; }
  }
  .fix-confirm { display: grid; gap: 8px; margin-top: 8px; }
  .fix-confirm p { margin: 0; }
  .fix-actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .fix-actions .btn { min-height: 44px; }
  .left-alone { list-style: none; margin: 8px 0 0; padding: 0; display: grid; gap: 4px; color: var(--warn); }
</style>
