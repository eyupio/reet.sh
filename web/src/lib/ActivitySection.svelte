<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, listEvents, UnauthorizedError, type Event } from './api'
  import { eventCategory, eventLabel, eventRemedy, eventTitle, EVENT_CATEGORIES, type EventCategory } from './events'
  import { elapsed } from './time'
  import { listProjects, projectRuns, terminalMachines, terminalSessions, TerminalError, type TerminalMachine, type TerminalSession } from './terminal/api'
  import { runSummary, type ProjectRun } from './projects'
  import RunView from './RunView.svelte'

  // The account's activity: what changed lately, newest first, grouped by day and filterable by area. A writer also sees the
  // runs of the retired saved Projects, kept read-only; each opens its run, whose worktrees can still be checked and removed.
  // A failed read retries itself twice, then says what happened and offers a button; the connection coming back retries too.
  let { writer = false, onsignedout, timeZone = '', dateFormat = '', flat = false }: {
    writer?: boolean
    onsignedout?: (e: unknown) => void
    timeZone?: string
    dateFormat?: string
    /** No top margin: the page's own heading already holds the section away from what is above it. */
    flat?: boolean
  } = $props()

  const PAGE = 50
  const AUTO_RETRIES = 2
  const DAY = 86_400_000

  let events = $state<Event[]>([])
  let loading = $state(true)
  let retrying = $state(false)
  let error = $state('')
  let hasMore = $state(false)
  let nextBefore = $state<number | null>(null)
  let loadingMore = $state(false)
  let moreError = $state('')
  let category = $state<EventCategory | 'all'>('all')
  let autoTries = 0

  let runs = $state<ProjectRun[] | undefined>()
  let runsFailed = $state(false)
  let runsError = $state('')
  let runsLoading = $state(false)
  let savedCount = $state<number | null>(null)
  let unread = $state<string[]>([])
  let machines = $state<TerminalMachine[] | undefined>()
  let sessions = $state<TerminalSession[] | undefined>()
  let sessionsFailed = $state(false)
  let open = $state<string | null>(null)
  const life = new AbortController()
  let dead = false
  let retryTimer: ReturnType<typeof setTimeout> | undefined

  const counts = $derived.by(() => {
    const seen: Record<string, number> = { all: events.length }
    for (const e of events) {
      const key = eventCategory(e)
      seen[key] = (seen[key] ?? 0) + 1
    }
    return seen
  })
  const filters = $derived(EVENT_CATEGORIES.filter(c => (counts[c.id] ?? 0) > 0))
  const visible = $derived(category === 'all' ? events : events.filter(e => eventCategory(e) === category))
  const groups = $derived.by(() => {
    const days = new Map<string, Event[]>()
    for (const e of visible) {
      const key = dayKey(e.at)
      const list = days.get(key)
      if (list) list.push(e)
      else days.set(key, [e])
    }
    return [...days].map(([key, list]) => ({ key, label: dayLabel(list[0].at), events: list }))
  })
  const categoryLabel = (id: EventCategory) => EVENT_CATEGORIES.find(c => c.id === id)?.label ?? 'Account'

  onMount(() => {
    void load()
    if (writer) void loadRuns()
    return () => {
      dead = true
      life.abort()
      if (retryTimer) clearTimeout(retryTimer)
    }
  })

  function signedOut(e: unknown) {
    return e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)
  }
  function transient(e: unknown) {
    if (e instanceof ApiError) return e.status === 408 || e.status === 429 || e.status >= 500
    if (typeof navigator !== 'undefined' && !navigator.onLine) return true
    return e instanceof TypeError
  }
  function guidance(e: unknown) {
    if (typeof navigator !== 'undefined' && !navigator.onLine) return 'You are offline. Activity will come back when the connection does.'
    if (e instanceof ApiError) {
      if (e.status === 403) return 'Your account cannot read this activity. Ask an owner to check your role.'
      if (e.status === 429) return 'The server is busy. Give it a moment, then try again.'
      if (e.status >= 500) return 'The server could not read your activity. This is usually temporary; try again.'
      return e.message
    }
    if (e instanceof TypeError) return 'The connection dropped before the activity arrived. Try again.'
    return e instanceof Error ? e.message : String(e)
  }

  function clearRetry() {
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = undefined
  }

  // `manual` starts over: a click on Retry tries twice again, where an automatic retry keeps its budget.
  async function load(manual = false) {
    if (manual) autoTries = 0
    clearRetry()
    loading = events.length === 0
    retrying = true
    error = ''
    try {
      const page = await listEvents(PAGE)
      if (dead) return
      apply(page.events, page)
      autoTries = 0
      retrying = false
    } catch (e) {
      if (dead) return
      if (signedOut(e)) {
        retrying = false
        onsignedout?.(e)
        return
      }
      if (!manual && autoTries < AUTO_RETRIES && transient(e)) {
        autoTries++
        error = `${guidance(e)} Retrying automatically…`
        retryTimer = setTimeout(() => void load(), autoTries === 1 ? 1200 : 3000)
        return
      }
      error = guidance(e)
      retrying = false
    } finally {
      loading = false
    }
  }

  function apply(list: Event[], page: { has_more?: boolean; next_before?: number | null }, append = false) {
    const seen = new Set(append ? events.map(e => e.id) : [])
    events = append ? [...events, ...list.filter(e => !seen.has(e.id))] : list
    hasMore = page.has_more ?? false
    nextBefore = page.next_before ?? (events.length ? events[events.length - 1].id : null)
  }

  async function more() {
    if (loadingMore || nextBefore === null) return
    loadingMore = true
    moreError = ''
    try {
      const page = await listEvents(PAGE, undefined, { before: nextBefore })
      if (dead) return
      apply(page.events, page, true)
    } catch (e) {
      if (signedOut(e)) { onsignedout?.(e); return }
      moreError = guidance(e)
    } finally {
      loadingMore = false
    }
  }

  const started = (at: number) => new Date(at).toLocaleString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  const ago = (at: number) => {
    const age = elapsed(at)
    return age === 'just now' ? age : `${age} ago`
  }

  function dayKey(ts: number) {
    const date = new Date(ts)
    try {
      const parts = new Intl.DateTimeFormat('en-GB', { timeZone: timeZone || undefined, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(date)
      const part = (type: string) => parts.find(p => p.type === type)?.value ?? ''
      return `${part('year')}-${part('month')}-${part('day')}`
    } catch {
      return date.toISOString().slice(0, 10)
    }
  }
  function dayLabel(ts: number) {
    const key = dayKey(ts)
    if (key === dayKey(Date.now())) return 'Today'
    if (key === dayKey(Date.now() - DAY)) return 'Yesterday'
    const sameYear = key.slice(0, 4) === dayKey(Date.now()).slice(0, 4)
    const locale = dateFormat === 'dmy' ? 'en-GB' : dateFormat === 'mdy' ? 'en-US' : dateFormat === 'ymd' ? 'sv-SE' : undefined
    const options: Intl.DateTimeFormatOptions = { weekday: 'long', day: 'numeric', month: 'long', ...(sameYear ? {} : { year: 'numeric' }), ...(timeZone ? { timeZone } : {}) }
    try {
      return new Date(ts).toLocaleDateString(locale, options)
    } catch {
      return new Date(ts).toLocaleDateString(undefined, options)
    }
  }
  function clock(ts: number) {
    try {
      return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', ...(timeZone ? { timeZone } : {}) }).format(new Date(ts))
    } catch {
      return new Date(ts).toLocaleTimeString()
    }
  }

  async function loadRuns() {
    runsLoading = true
    runsError = ''
    try {
      const [list, m, s] = await Promise.allSettled([listProjects(life.signal), terminalMachines(life.signal), terminalSessions(life.signal)])
      if (life.signal.aborted) return
      if ([list, m, s].some(r => r.status === 'rejected' && signedOut(r.reason))) { onsignedout?.(new UnauthorizedError()); return }
      if (list.status === 'fulfilled') {
        savedCount = list.value.length
        const read = await Promise.allSettled(list.value.map(p => projectRuns(p.id, life.signal)))
        if (life.signal.aborted) return
        if (read.some(r => r.status === 'rejected' && signedOut(r.reason))) { onsignedout?.(new UnauthorizedError()); return }
        unread = list.value.filter((_, i) => read[i].status === 'rejected').map(p => p.name)
        runs = read.flatMap(r => r.status === 'fulfilled' ? r.value : []).sort((a, b) => b.startedAt - a.startedAt)
        runsFailed = false
      } else {
        runsFailed = true
        runs = []
        runsError = guidance(list.reason)
      }
      if (m.status === 'fulfilled') machines = m.value
      if (s.status === 'fulfilled') sessions = s.value
      sessionsFailed = s.status !== 'fulfilled'
    } finally {
      runsLoading = false
    }
  }
</script>

<svelte:window ononline={() => { if (error) void load(true); if (runsFailed) void loadRuns() }} />

<section class="activity" class:flat aria-labelledby="activity-title">
  <div class="head">
    <div>
      <h2 id="activity-title">Activity</h2>
      <p class="lede">What you changed lately, newest first.</p>
    </div>
    <div class="head-actions">
      <span class="total" aria-live="polite">{#if loading && events.length === 0}Reading…{:else}{events.length}{hasMore ? '+' : ''} {events.length === 1 ? 'change' : 'changes'}{/if}</span>
      <button type="button" class="btn quiet" onclick={() => void load(true)} disabled={loading || retrying}>Refresh</button>
    </div>
  </div>

  {#if error}
    <div class="notice bad" role="alert">
      <p>{error}</p>
      <button type="button" class="btn" onclick={() => void load(true)} disabled={loading || retrying}>{retrying ? 'Trying again…' : 'Try again'}</button>
    </div>
  {/if}

  {#if filters.length > 1}
    <div class="filters" role="group" aria-label="Filter activity">
      <button type="button" aria-pressed={category === 'all'} onclick={() => (category = 'all')}>All<small>{counts.all}</small></button>
      {#each filters as filter (filter.id)}
        <button type="button" aria-pressed={category === filter.id} onclick={() => (category = filter.id)}>{filter.label}<small>{counts[filter.id]}</small></button>
      {/each}
    </div>
  {/if}

  {#if loading && events.length === 0}
    <p class="muted" role="status">Reading your activity…</p>
  {:else if events.length === 0}
    {#if !error}
      <div class="empty">
        <p>Nothing recorded yet.</p>
        <p class="muted">Changes you make to machines, projects, skills and your account appear here as they happen.</p>
      </div>
    {/if}
  {:else if visible.length === 0}
    <p class="muted">No {categoryLabel(category as EventCategory).toLowerCase()} activity yet.</p>
  {:else}
    {#each groups as group (group.key)}
      <section class="day" aria-labelledby={`day-${group.key}`}>
        <h3 id={`day-${group.key}`}>{group.label}<span>{group.events.length}</span></h3>
        <ul class="events">
          {#each group.events as e (e.id)}
            {@const remedy = eventRemedy(e)}
            <li>
              <span class="dot" class:agents={eventCategory(e) === 'agents'} class:projects={eventCategory(e) === 'projects'} class:skills={eventCategory(e) === 'skills'} class:lists={eventCategory(e) === 'lists'} class:admin={eventCategory(e) === 'admin'} aria-hidden="true"></span>
              <div class="body">
                <p class="label">{eventLabel(e)}</p>
                <p class="meta">
                  <span class="cat">{categoryLabel(eventCategory(e))}</span>
                  {#if e.actor}<span class="actor">by {e.actor}</span>{/if}
                  <time datetime={new Date(e.at).toISOString()} title={eventTitle(e)}>{clock(e.at)} · {ago(e.at)}</time>
                </p>
                {#if remedy}<p class="remedy"><a href={remedy.href}>{remedy.label}</a></p>{/if}
              </div>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
    {#if moreError}
      <div class="notice bad" role="alert">
        <p>{moreError}</p>
        <button type="button" class="btn" onclick={() => void more()} disabled={loadingMore}>{loadingMore ? 'Trying again…' : 'Try again'}</button>
      </div>
    {:else if hasMore}
      <button type="button" class="btn load" onclick={() => void more()} disabled={loadingMore}>{loadingMore ? 'Reading older changes…' : 'Load older changes'}</button>
    {/if}
  {/if}
</section>

<!-- Only an account that had saved Projects sees their runs; nothing is said about a retired feature otherwise. -->
{#if writer && (savedCount === null || savedCount > 0)}
  <section class="runs" aria-labelledby="runs-title">
    <div class="head">
      <div>
        <h2 id="runs-title">Runs from Projects</h2>
        <p class="lede">The newest 20 runs of each saved Project, kept to read. Their worktrees can still be checked and removed here.</p>
      </div>
      {#if unread.length > 0}
        <button type="button" class="btn quiet" onclick={() => void loadRuns()} disabled={runsLoading}>{runsLoading ? 'Trying again…' : 'Try again'}</button>
      {/if}
    </div>
    {#each unread as name (name)}<p class="bad" role="alert">Couldn't read the runs of {name} just now. The others are still listed.</p>{/each}
    {#if runsFailed}
      <div class="notice bad" role="alert">
        <p>{runsError || "The server couldn't read the runs just now."}</p>
        <button type="button" class="btn" onclick={() => void loadRuns()} disabled={runsLoading}>{runsLoading ? 'Trying again…' : 'Try again'}</button>
      </div>
    {:else if !runs}
      <p class="muted" role="status">Reading the runs…</p>
    {:else if runs.length === 0}
      <p class="muted">No runs were recorded.</p>
    {:else}
      <ul class="run-list">
        {#each runs as run (run.id)}
          <li>
            <button type="button" class="run-head" aria-expanded={open === run.id} aria-controls={`run-${run.id}`} onclick={() => (open = open === run.id ? null : run.id)}>
              <span class="what"><strong>{run.projectName}</strong> <span class="muted">· started {started(run.startedAt)}</span></span>
              <span class="muted">{runSummary(run)}</span>
            </button>
            {#if open === run.id}
              <div id={`run-${run.id}`} class="run-body"><RunView {run} {machines} {sessions} {sessionsFailed} {onsignedout} /></div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .activity, .runs { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .activity.flat { margin-top: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .head h2 { margin: 0; font: 600 20px/1.3 var(--serif); }
  .head-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .lede { margin: 6px 0 0; color: var(--muted); font-size: 0.86rem; max-width: 62ch; }
  .total { color: var(--muted); font-size: 0.78rem; font-variant-numeric: tabular-nums; }
  .notice { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--bad); border-radius: 10px; background: var(--bg); }
  .notice p { margin: 0; color: var(--bad); font-size: 0.86rem; }
  .muted { margin: 12px 0 0; color: var(--muted); }
  .bad { margin: 12px 0 0; color: var(--bad); font-size: 0.86rem; }
  .empty { margin-top: 16px; padding: 18px; border: 1px dashed var(--line); border-radius: 10px; background: var(--bg); }
  .empty p { margin: 0; }
  .empty .muted { margin-top: 6px; font-size: 0.84rem; }

  .filters { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 16px; padding-bottom: 14px; border-bottom: 1px solid var(--line); }
  .filters button {
    display: inline-flex; align-items: center; gap: 6px; min-height: 34px; padding: 5px 11px;
    border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--muted);
    font: inherit; font-size: 0.8rem; cursor: pointer;
  }
  .filters button:hover { color: var(--ink); border-color: var(--muted); }
  .filters button[aria-pressed='true'] { border-color: var(--accent); background: var(--tint); color: var(--ink); font-weight: 600; }
  .filters small { color: var(--muted); font-size: 0.72rem; font-variant-numeric: tabular-nums; }

  .day { margin-top: 18px; }
  .day h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 4px; color: var(--muted); font: 600 0.72rem/1.4 var(--sans); letter-spacing: 0.1em; text-transform: uppercase; }
  .day h3 span { font-weight: 500; letter-spacing: 0; text-transform: none; }
  .events { list-style: none; margin: 0; padding: 0; }
  .events li { display: flex; gap: 10px; padding: 9px 0; border-bottom: 1px solid var(--line); }
  .events li:last-child { border-bottom: 0; }
  .dot { flex: none; width: 8px; height: 8px; margin-top: 6px; border-radius: 999px; background: var(--muted); }
  .dot.agents { background: var(--accent); }
  .dot.projects { background: var(--repo); }
  .dot.skills { background: var(--good); }
  .dot.lists { background: var(--info); }
  .dot.admin { background: var(--warn); }
  .body { min-width: 0; flex: 1; }
  .label { margin: 0; font-size: 0.88rem; overflow-wrap: anywhere; }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin: 3px 0 0; color: var(--muted); font-size: 0.74rem; }
  .cat { padding: 0 6px; border: 1px solid var(--line); border-radius: 999px; font-size: 0.66rem; letter-spacing: 0.03em; }
  .meta time { margin-left: auto; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .remedy { margin: 4px 0 0; font-size: 0.78rem; }
  .remedy a { color: var(--accent-text); }
  .load { margin-top: 16px; }

  .runs { display: grid; gap: 12px; }
  .run-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .run-list > li { border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
  .run-head { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 4px 12px; width: 100%; min-height: 44px; padding: 10px 14px; border: 0; background: transparent; color: var(--ink); font: inherit; font-size: 0.88rem; text-align: left; cursor: pointer; }
  .run-head .what { min-width: 0; overflow-wrap: anywhere; }
  .run-body { padding: 0 14px 14px; }
  @media (max-width: 560px) {
    .meta time { margin-left: 0; }
    .head-actions { width: 100%; justify-content: space-between; }
  }
</style>
