<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, queueSyncJob, updateSyncAgent, type SyncJob, type SyncState } from './api'
  import { agentOnline, groupLabel } from './machines'
  import { activeJobKindLabel, agoLabel, jobPlanCounts, syncJobStages } from './syncOverview'
  import SyncProblem from './SyncProblem.svelte'

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
  let now = $state(Date.now())
  let refreshing = false
  let wasLive = $state(false)
  let finishedAt = $state(0)
  let retrying = $state('')

  const sync = $derived(local ?? snapshot)
  const host = $derived((context?.host ?? '').toLowerCase())
  const liveJobs = $derived((sync?.jobs ?? []).filter(job => job.kind !== 'update'))
  const scopedJobs = $derived(liveJobs.filter(job => !host || hostFor(job.agentId).toLowerCase() === host))
  const activeJobs = $derived(scopedJobs.filter(job => ['queued', 'running', 'awaiting-approval'].includes(job.status)).sort((a, b) => b.updatedAt - a.updatedAt))
  const activeRuns = $derived((sync?.runs ?? []).filter(run => ['running', 'review'].includes(run.status) && (!host || hostFor(run.agentId).toLowerCase() === host)))
  const run = $derived(activeRuns[0])
  const ordered = $derived([...scopedJobs].sort((a, b) => b.updatedAt - a.updatedAt))
  const latest = $derived(ordered[0])
  const live = $derived(activeJobs.length > 0 || activeRuns.length > 0)
  const waiting = $derived(live && activeJobs.every(job => job.status === 'queued') && !activeRuns.length)
  const recentlyFinished = $derived(finishedAt > 0 && now - finishedAt < 15000)
  const show = $derived((open || live || recentlyFinished) && !dismissed)
  const displayJobs = $derived(activeJobs.length ? activeJobs : latest ? [latest] : [])
  const currentTarget = $derived(run && run.cursor < run.targets.length ? (sync?.targets ?? []).find(target => target.target.id === run.targets[run.cursor]) : undefined)
  const progress = $derived(run ? Math.round((run.cursor / Math.max(1, run.targets.length)) * 100) : 0)
  const title = $derived(waiting ? 'Waiting for machine' : context?.title ?? (live ? run ? (run.status === 'review' ? 'Review sync changes' : 'Syncing your machines') : activeJobs[0] ? activeJobs[0].status === 'awaiting-approval' ? 'Review sync changes' : activeJobs[0].status === 'queued' ? 'Waiting for machine' : activeJobKindLabel(activeJobs[0]) : 'Syncing your machines' : latest ? (latest.status === 'succeeded' ? 'Sync complete' : latest.status === 'failed' ? 'Sync needs attention' : 'Sync stopped') : 'Sync activity'))
  const subtitle = $derived(context?.note || hostForRunOrJob() || 'All connected machines')
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
  function targetLabel(job: SyncJob): string {
    const target = (sync?.targets ?? []).find(item => item.target.id === job.targetId)
    if (target) return `${target.target.host} · ${groupLabel(target.target)}`
    const env = (sync?.environments ?? []).find(item => item.id === job.targetId)
    return env ? `${env.host} · ${groupLabel(env)}` : 'Agent scope'
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
    if (job.status === 'failed') return 'bad'
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

  function close() {
    dismissed = true
    onclose?.()
  }

  // A fresh scan or preview is the only retry offered: an old approved plan is
  // never replayed from the dock.
  async function retry(job: SyncJob) {
    if (retrying || !canWrite) return
    retrying = job.id
    try {
      const next = job.kind === 'update'
        ? await updateSyncAgent(job.agentId, job.revision || 'latest')
        : await queueSyncJob(job.agentId, job.targetId, job.kind === 'scan' ? 'scan' : 'preview')
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

{#if show}
  <aside class="sync-dock" class:working={live} aria-label="Sync activity" aria-live="polite">
    <div class="accent" aria-hidden="true"></div>
    <header class="dock-head">
      <div class="dock-id">
        <span class="orb" class:live={live && !waiting} class:bad={!live && latest?.status === 'failed'} aria-hidden="true">
          {#if waiting}
            <span aria-hidden="true">Ⅱ</span>
          {:else if live}
            <span class="spin"></span>
          {:else if latest?.status === 'failed'}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M6 6l12 12M18 6 6 18"></path></svg>
          {:else}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m5 12 4 4L19 6"></path></svg>
          {/if}
        </span>
        <div class="dock-copy">
          <p>Live sync</p>
          <h2>{title}</h2>
          <small>{subtitle}</small>
        </div>
      </div>
      <button type="button" class="dock-close" aria-label="Hide sync activity" onclick={close}>×</button>
    </header>

    {#if run}
      <div class="run" role="group" aria-label={`Sync run, ${run.cursor} of ${run.targets.length} scopes complete`}>
        <div class="run-meta"><span>{run.status === 'review' ? 'Review needed' : 'Applying changes'}</span><strong>{run.cursor} / {run.targets.length}</strong></div>
        <div class="track"><span style:width={`${progress}%`}></span></div>
        {#if currentTarget}<small>Next: {currentTarget.target.host} · {groupLabel(currentTarget.target)}</small>{/if}
      </div>
    {/if}

    <div class="dock-body">
      {#if error}<p class="dock-error" role="alert">{error}</p>{/if}
      {#each displayJobs as job (job.id)}
        {@const stages = syncJobStages(job)}
        {@const counts = jobPlanCounts(job)}
        <article class="job" class:done={job.status === 'succeeded'} class:failed={job.status === 'failed'} aria-busy={['queued', 'running'].includes(job.status)}>
          <div class="job-head">
            <div><strong>{targetLabel(job)}</strong><small>{job.status === 'queued' ? 'Queued · waiting for the machine to pick up this job' : kindTitle(job)}</small></div>
            <span class="job-state {tone(job)}">{statusText(job)}</span>
          </div>
          <ol class="steps">
            {#each stages as stage (stage.key)}
              <li class={stage.state}><span class="step-dot" aria-hidden="true"></span><span>{stage.label}</span></li>
            {/each}
          </ol>
          {#if counts.total}
            <p class="plan-count">{counts.installs} install{counts.installs === 1 ? '' : 's'}{counts.removals ? ` · ${counts.removals} removal${counts.removals === 1 ? '' : 's'}` : ''}</p>
            <ul class="plan-list">
              {#each (job.plan.actions ?? []).slice(0, 5) as action (action.skill.id)}
                <li class:remove={!action.add}><span aria-hidden="true">{action.add ? '+' : '−'}</span>{action.skill.name}</li>
              {/each}
              {#if (job.plan.actions?.length ?? 0) > 5}<li class="more">+{(job.plan.actions?.length ?? 0) - 5} more</li>{/if}
            </ul>
          {:else if job.status === 'succeeded'}
            <p class="plan-count">{doneCounts.total ? '' : 'Everything already matches. No files needed changing.'}</p>
          {/if}
          {#if job.message}
            {#if job.status === 'failed' || job.status === 'rolled_back'}
              <SyncProblem message={job.message} host={hostFor(job.agentId)} site={location.origin} compact online={onlineFor(job.agentId)}
                retrying={retrying === job.id} onretry={canWrite ? () => void retry(job) : undefined} />
            {:else}
              <p class="job-message">{job.message}</p>
            {/if}
          {/if}
          {#if job.status === 'awaiting-approval'}<a class="btn primary review" href="/sync#sync-jobs">Review changes</a>{/if}
        </article>
      {/each}
      {#if !displayJobs.length && !error}<p class="dock-empty">Waiting for the machine to report…</p>{/if}
    </div>

    <footer class="dock-foot">
      <span>{live ? 'Updating live' : !latest && open ? 'Starting sync…' : 'Finished'} · {agoLabel(latest?.updatedAt ?? now, now)}</span>
      <a href="/sync#sync-jobs">Open sync</a>
    </footer>
  </aside>
{/if}

<style>
  .sync-dock {
    position: fixed; z-index: 60; right: 20px; bottom: 20px;
    width: min(420px, calc(100vw - 32px)); max-height: min(72vh, 660px);
    display: grid; grid-template-rows: auto auto minmax(0, 1fr) auto;
    overflow: hidden; border: 1px solid var(--line-strong); border-radius: 16px;
    background: color-mix(in srgb, var(--card-raised) 94%, transparent);
    box-shadow: var(--shadow);
    backdrop-filter: blur(14px); color: var(--ink);
  }
  .accent { height: 3px; background: linear-gradient(90deg, var(--accent), color-mix(in srgb, var(--info) 70%, var(--accent)), var(--accent)); background-size: 200% 100%; }
  .working .accent { animation: glide 2.8s linear infinite; }
  @keyframes glide { to { background-position: -200% 0; } }
  .dock-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 16px 16px 12px; }
  .dock-id { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
  .orb { display: grid; place-items: center; width: 34px; height: 34px; flex: none; border-radius: 50%; border: 1px solid color-mix(in srgb, var(--good) 45%, var(--line)); background: color-mix(in srgb, var(--good) 12%, var(--card)); color: var(--good); }
  .orb.live { border-color: color-mix(in srgb, var(--accent) 55%, var(--line)); background: color-mix(in srgb, var(--accent) 14%, var(--card)); color: var(--accent-text); }
  .orb.bad { border-color: color-mix(in srgb, var(--bad) 50%, var(--line)); background: color-mix(in srgb, var(--bad) 12%, var(--card)); color: var(--bad); }
  .orb svg { width: 16px; height: 16px; }
  .spin { width: 18px; height: 18px; border-radius: 50%; border: 2px solid color-mix(in srgb, var(--accent) 25%, transparent); border-top-color: var(--accent-text); display: block; }
  .working .spin { animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .dock-copy { display: grid; gap: 2px; min-width: 0; }
  .dock-copy p { margin: 0; color: var(--accent-text); font-size: 0.65rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  .dock-copy h2 { margin: 0; font: 500 1.12rem/1.25 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .dock-copy small { color: var(--muted); font-size: 0.74rem; overflow-wrap: anywhere; }
  .dock-close { flex: none; display: grid; place-items: center; width: 36px; height: 36px; padding: 0; border: 1px solid transparent; border-radius: 9px; background: transparent; color: var(--muted); font-size: 1.15rem; line-height: 1; cursor: pointer; }
  .dock-close:hover { color: var(--ink); border-color: var(--line); }
  .run { padding: 0 16px 12px; display: grid; gap: 6px; }
  .run-meta { display: flex; justify-content: space-between; gap: 10px; color: var(--muted); font-size: 0.72rem; }
  .run-meta strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .run small { color: var(--muted); font-size: 0.7rem; overflow-wrap: anywhere; }
  .track { height: 6px; overflow: hidden; border-radius: 999px; background: color-mix(in srgb, var(--line) 70%, transparent); }
  .track span { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, var(--accent), var(--info)); transition: width 0.5s var(--ease-out); }
  .dock-body { display: grid; gap: 10px; padding: 2px 16px 12px; overflow: auto; overscroll-behavior: contain; }
  .job { display: grid; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--bg) 55%, var(--card)); }
  .job.done { border-color: color-mix(in srgb, var(--good) 32%, var(--line)); }
  .job.failed { border-color: color-mix(in srgb, var(--bad) 38%, var(--line)); }
  .job-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  .job-head > div { display: grid; gap: 1px; min-width: 0; }
  .job-head strong { font-size: 0.82rem; overflow-wrap: anywhere; }
  .job-head small { color: var(--muted); font-size: 0.69rem; overflow-wrap: anywhere; }
  .job-state { flex: none; padding: 2px 8px; border-radius: 999px; border: 1px solid var(--line); color: var(--muted); font-size: 0.65rem; font-weight: 600; }
  .job-state.ok { color: var(--good); border-color: color-mix(in srgb, var(--good) 40%, var(--line)); }
  .job-state.bad { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 40%, var(--line)); }
  .job-state.warn { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); }
  .job-state.work { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); }
  .steps { list-style: none; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4px; margin: 0; padding: 0; }
  .steps li { display: grid; gap: 5px; min-width: 0; color: var(--muted); font-size: 0.67rem; }
  .step-dot { display: block; height: 4px; border-radius: 999px; background: var(--line); }
  .steps li.active { color: var(--accent-text); font-weight: 600; }
  .steps li.active .step-dot { background: linear-gradient(90deg, var(--accent), var(--info)); animation: pulse 1.4s ease-in-out infinite; }
  .steps li.done { color: var(--ink-soft); }
  .steps li.done .step-dot { background: var(--good); }
  .steps li.failed { color: var(--bad); }
  .steps li.failed .step-dot { background: var(--bad); }
  .steps li.review { color: var(--warn); }
  .steps li.review .step-dot { background: var(--warn); }
  @keyframes pulse { 50% { opacity: 0.45; } }
  .plan-count { margin: 0; color: var(--muted); font-size: 0.72rem; }
  .plan-list { list-style: none; display: grid; gap: 3px; margin: 0; padding: 0; font-size: 0.73rem; }
  .plan-list li { display: flex; gap: 7px; align-items: baseline; min-width: 0; }
  .plan-list li span { flex: none; width: 10px; color: var(--good); font-weight: 700; }
  .plan-list li.remove span { color: var(--bad); }
  .plan-list li.more { color: var(--muted); }
  .job-message { margin: 0; color: var(--muted); font-size: 0.72rem; line-height: 1.5; overflow-wrap: anywhere; }
  .review { justify-self: start; min-height: 36px; padding: 6px 12px; font-size: 0.76rem; }
  .dock-empty { margin: 0; padding: 8px 0; color: var(--muted); font-size: 0.76rem; }
  .dock-error { margin: 0; padding: 9px 10px; border: 1px solid color-mix(in srgb, var(--bad) 45%, var(--line)); border-radius: 8px; color: var(--bad); font-size: 0.75rem; }
  .dock-foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 16px; border-top: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 35%, var(--card)); color: var(--muted); font-size: 0.69rem; }
  .dock-foot a { color: var(--accent-text); font-weight: 600; text-decoration: none; }
  .dock-foot a:hover { text-decoration: underline; }
  @media (max-width: 650px) {
    .sync-dock { right: 12px; left: 12px; bottom: calc(12px + var(--bottom-chrome, 0px)); width: auto; max-height: min(68vh, 560px); }
  }
  @media (prefers-reduced-motion: reduce) {
    .working .accent, .working .spin, .steps li.active .step-dot { animation: none; }
    .track span { transition: none; }
  }
</style>
