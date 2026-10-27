<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { listEvents, UnauthorizedError, type Event } from './api'
  import { eventLabel, eventTitle } from './events'
  import { elapsed } from './time'
  import { listProjects, projectRuns, terminalMachines, terminalSessions, TerminalError, type TerminalMachine, type TerminalSession } from './terminal/api'
  import { runSummary, type ProjectRun } from './projects'
  import RunView from './RunView.svelte'

  // /activity: the account activity list (what was changed lately, newest first), moved here from the account page. For a
  // writer it also lists the runs of the saved Projects, kept read-only since folder projects replaced them: each opens
  // its run, whose worktrees can still be checked and removed.
  let { accounts = true, writer = false, onsignedout }: { accounts?: boolean; writer?: boolean; onsignedout?: (e: unknown) => void } = $props()
  let events = $state<Event[]>([])
  let loading = $state(true)
  let error = $state('')

  let runs = $state<ProjectRun[] | undefined>()
  let runsFailed = $state(false)
  // How many saved Projects the account has: null until read (or when the read failed).
  let savedCount = $state<number | null>(null)
  // Saved Projects whose runs could not be read: the others are still listed.
  let unread = $state<string[]>([])
  let machines = $state<TerminalMachine[] | undefined>()
  let sessions = $state<TerminalSession[] | undefined>()
  let sessionsFailed = $state(false)
  let open = $state<string | null>(null)
  const life = new AbortController()

  onMount(() => {
    void load()
    if (writer) void loadRuns()
    return () => life.abort()
  })
  async function load() {
    try {
      events = (await listEvents(50)).events
      error = ''
    } catch (e) {
      if (e instanceof UnauthorizedError) { onsignedout?.(e); return }
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }
  const signedOut = (e: unknown) => e instanceof TerminalError && e.status === 401
  // Every saved Project's runs (the server keeps the newest 20 of each), newest first, read one Project at a time so one
  // that fails leaves the others listed. The machines and sessions are for the run view: names, and whether a run's
  // session has ended (only then is its worktree offered for removal).
  async function loadRuns() {
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
    } else { runsFailed = true; runs = [] }
    if (m.status === 'fulfilled') machines = m.value
    if (s.status === 'fulfilled') sessions = s.value; else sessionsFailed = true
  }
  const started = (at: number) => new Date(at).toLocaleString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
</script>

<PageHeader eyebrow={accounts ? 'Your account' : 'This workspace'} title="Activity" lede="What you changed lately, newest first." />
{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
<section class="block" aria-label="Activity list">
  {#if loading}
    <p class="muted" role="status">Loading activity…</p>
  {:else if events.length === 0 && !error}
    <p class="muted">Nothing recorded yet.</p>
  {:else}
    <ul class="events">
      {#each events as event (event.id)}
        <li title={eventTitle(event)}><span>{eventLabel(event)}</span><time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time></li>
      {/each}
    </ul>
  {/if}
</section>

<!-- Only an account that had saved Projects sees their runs; nothing is said about a retired feature otherwise. -->
{#if writer && (savedCount === null || savedCount > 0)}
  <section class="block runs" aria-labelledby="runs-title">
    <h2 id="runs-title">Runs from Projects</h2>
    <p class="muted">Runs of your saved Projects, kept to read: the newest 20 runs of each saved Project. Their worktrees can still be checked and removed here.</p>
    {#each unread as name (name)}<p class="bad" role="alert">Couldn't read the runs of {name} just now.</p>{/each}
    {#if runsFailed}
      <p class="bad" role="alert">Couldn't read the runs just now.</p>
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
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--bad); border-radius: 10px; background: var(--card); }
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .muted { margin: 0; color: var(--muted); }
  .bad { margin: 0; color: var(--bad); }
  .events { list-style: none; margin: 0; padding: 0; }
  .events li { display: flex; justify-content: space-between; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.84rem; }
  .events li span { min-width: 0; overflow-wrap: anywhere; }
  .events time { flex: none; }
  .runs { display: grid; gap: 12px; }
  .runs h2 { margin: 0; font: 600 20px/1.3 var(--serif); }
  .run-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .run-list > li { border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
  .run-head { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 4px 12px; width: 100%; min-height: 44px; padding: 10px 14px; border: 0; background: transparent; color: var(--ink); font: inherit; font-size: 0.88rem; text-align: left; cursor: pointer; }
  .run-head .what { min-width: 0; overflow-wrap: anywhere; }
  .run-body { padding: 0 14px 14px; }
</style>
