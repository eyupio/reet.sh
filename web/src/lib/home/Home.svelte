<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { getSyncState, UnauthorizedError, type Machine, type SyncState } from '../api'
  import { getRun, listProjects, machineHealth, terminalMachines, terminalSessions, TerminalError, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import type { Project, ProjectRun } from '../projects'
  import { attentionCount, fleetCards, reporting, inFlight, needsYou, projectRunning, recentRunIds, sessionProjects, type GroupBy, type HomeSession } from '../home'
  import StartAgentDialog from '../StartAgentDialog.svelte'
  import ProjectDialog from '../ProjectDialog.svelte'
  import FleetStrip from './FleetStrip.svelte'
  import InFlight from './InFlight.svelte'
  import NeedsYou from './NeedsYou.svelte'
  import ProjectRail from './ProjectRail.svelte'

  // Home on today's data. A writer in accounts mode gets the project rail, the fleet strip, In flight, Needs you and a docked
  // Start an agent; everyone else signed in gets the fleet strip alone (R16). Everything comes from the existing loaders and
  // is read again every 15 seconds while the page is in view. A read that fails keeps what was last read and says so. Nothing
  // about Projects, sessions or machines goes into browser storage; the one stored value is the Group by choice.
  let { userId, writer, canAdd, accounts, onsignedout, onattention }: {
    userId: number
    /** A writer in accounts mode: the full Home. Otherwise the read-only Home. */
    writer: boolean
    /** Whether this sign-in can add a machine. */
    canAdd: boolean
    accounts: boolean
    onsignedout: (e: unknown) => void
    /** The Needs you count, for the bell. */
    onattention?: (count: number) => void
  } = $props()

  const POLL_MS = 15_000
  // The Machines page opens Add a machine (or the connect steps) when it arrives with this fragment.
  const machinesAt = (fragment: string) => `/machines#${fragment}`
  const GROUPS: GroupBy[] = ['project', 'machine', 'state']
  // svelte-ignore state_referenced_locally
  const groupKey = `reet.home.groupBy.${userId}`
  function storedGroup(): GroupBy {
    try { const v = localStorage.getItem(groupKey); return GROUPS.includes(v as GroupBy) ? v as GroupBy : 'project' } catch { return 'project' }
  }
  let groupBy = $state<GroupBy>(storedGroup())
  function choose(next: GroupBy) {
    groupBy = next
    try { localStorage.setItem(groupKey, next) } catch { /* the choice lasts for this page only */ }
  }

  let health = $state<MachineHealth[] | undefined>()
  let sessions = $state<HomeSession[] | undefined>()
  let sync = $state<SyncState | undefined>()
  let projects = $state<Project[] | undefined>()
  let runs = $state(new Map<string, ProjectRun>())
  let now = $state(Date.now())
  let problem = $state('')
  let readOnce = false
  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(POLL_MS)])

  const emptySync: SyncState = { environments: [], targets: [], agents: [], jobs: [] }
  const cards = $derived(fleetCards(health ?? [], sessions ?? [], sync ?? emptySync, now))
  const owners = $derived(sessionProjects(projects ?? [], [...runs.values()]))
  const groups = $derived(inFlight(sessions ?? [], owners, health ?? [], groupBy, now))
  const shownMachines = $derived(new Set(groups.flatMap(g => g.sessions.map(s => s.agentId))))
  const offline = $derived(cards.filter(c => !reporting(c) && shownMachines.has(c.agentId)).map(c => ({ agentId: c.agentId, host: c.name })))
  const running = $derived(projectRunning(sessions ?? [], owners))
  const needs = $derived(needsYou(sessions ?? [], now))
  $effect(() => { onattention?.(attentionCount(needs)) })

  // Only a refusal of the credential this sign-in uses means it is signed out: the token rides the sync read, the cookie (an
  // accounts-mode sign-in) every read. The health read is cookie-only, so a single-token or open instance never makes it.
  const refused = (e: unknown, cookie: boolean) => e instanceof UnauthorizedError || (cookie && accounts && e instanceof TerminalError && e.status === 401)
  // Machines from the health report when this sign-in can read it; otherwise (a single token, an open instance, or an account
  // the health read refuses with 403) from the sync agents, connected when the agent reported in the last 90 seconds.
  // svelte-ignore state_referenced_locally
  let healthReadable = accounts
  const fromAgents = (s: SyncState, at: number): MachineHealth[] =>
    (s.agents ?? []).map(a => ({ agentId: a.id, host: a.host, state: a.lastSeen > at - 90_000 ? 'connected' : 'never_connected' }))
  let sessionsFailed = $state(false), projectsFailed = $state(false)

  // Each load has a generation. A poll is skipped while another load runs; an explicit refresh (after a start or a save)
  // supersedes it, and an answer from an older load is dropped so it never overwrites a newer list.
  let loading = false, generation = 0
  async function load(explicit = false) {
    if (life.signal.aborted || (loading && !explicit)) return
    const mine = ++generation
    const stale = () => life.signal.aborted || mine !== generation
    loading = true
    const signOut = () => { life.abort(); onsignedout(new UnauthorizedError()) }
    try {
      const [h, y, s, p] = await Promise.allSettled([
        healthReadable ? machineHealth(bounded()) : Promise.resolve(undefined),
        getSyncState(bounded()),
        writer ? terminalSessions(bounded()) : Promise.resolve(undefined),
        writer ? listProjects(bounded()) : Promise.resolve(undefined),
      ])
      if (stale()) return
      if ([h, y, s, p].some(r => r.status === 'rejected' && refused(r.reason, r !== y))) return signOut()
      const at = Date.now()
      let failed = false
      if (y.status === 'fulfilled') sync = y.value; else failed = true
      if (h.status === 'rejected' && h.reason instanceof TerminalError && h.reason.status === 403 && health === undefined) healthReadable = false
      if (h.status === 'fulfilled' && h.value) health = h.value.machines ?? []
      else if (!healthReadable) { if (y.status === 'fulfilled') health = fromAgents(y.value, at) }
      // A health read that failed for a passing reason keeps the cards as last read.
      else failed = true
      if (writer) {
        if (s.status === 'fulfilled' && s.value) { sessions = s.value; sessionsFailed = false } else { failed = true; sessionsFailed = sessions === undefined }
        if (p.status === 'fulfilled' && p.value) { projects = p.value; projectsFailed = false } else { failed = true; projectsFailed = projects === undefined }
        // The runs whose members carry session ids: a Project's last run from the last day, or one still running. A run that
        // cannot be read keeps what it last said.
        const ids = recentRunIds(projects ?? [], at)
        const read = await Promise.allSettled(ids.map(id => getRun(id, bounded())))
        if (stale()) return
        if (read.some(r => r.status === 'rejected' && refused(r.reason, true))) return signOut()
        const next = new Map<string, ProjectRun>()
        ids.forEach((id, i) => {
          const r = read[i]
          if (r.status === 'fulfilled') next.set(id, r.value)
          else { failed = true; const kept = runs.get(id); if (kept) next.set(id, kept) }
        })
        runs = next
      }
      now = at
      problem = !failed ? '' : readOnce ? "Couldn't refresh just now. Showing what was last read." : "Couldn't read everything just now. Home tries again every 15 seconds."
      readOnce = true
    } finally {
      if (mine === generation) loading = false
    }
  }

  // The dialogs need the terminal machines (with their launch capabilities) and the reported folders.
  let terminals = $state<TerminalMachine[] | undefined>()
  let checking = $state(false)
  let notice = $state<{ text: string; href?: string; bad?: boolean } | null>(null)
  let starting = $state<{ opener: HTMLElement | null } | null>(null)
  let creating = $state<{ opener: HTMLElement | null } | null>(null)
  const folders = $derived<Machine[] | undefined>(sync?.machines)
  // Read on every open, never from an earlier one: a machine's readiness and launch capabilities change while Home stays
  // open, and the polls refresh health, not these. `checking` stops a second open racing the first read.
  async function machinesForDialog(): Promise<boolean> {
    checking = true
    try { terminals = await terminalMachines(bounded()); return true }
    catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return false }
      notice = { text: "Couldn't read your machines just now. Try again in a moment.", bad: true }
      return false
    } finally { checking = false }
  }
  async function openStart(opener: HTMLElement) {
    if (checking) return
    notice = null
    if (await machinesForDialog()) starting = { opener }
  }
  async function openProject(opener: HTMLElement) {
    if (checking) return
    notice = null
    if (await machinesForDialog()) creating = { opener }
  }
  // A start goes straight to the new session, at the address the Machines and Terminals pages use. The row is added first, so
  // Home has it if the browser brings this page back from its cache on Back; the dialog stays Starting… until the page goes.
  function started(session: TerminalSession): 'leaving' {
    sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
    location.assign(`/terminals?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`)
    return 'leaving'
  }
  function saved(p: Project) {
    projects = [p, ...(projects ?? []).filter(x => x.id !== p.id)]
    notice = { text: `${p.name} is saved.`, href: `/projects/${encodeURIComponent(p.id)}` }
    void load(true)
  }

  onMount(() => {
    void load()
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const shown = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', shown)
    // Back from the session the person just started: a page restored from the browser's cache still has the dialog in its
    // Starting… state and lists as they were, so it closes the dialog and reads everything again.
    const restored = async (e: PageTransitionEvent) => {
      if (!e.persisted) return
      const back = starting?.opener
      starting = null
      void load(true)
      // Once the dialog has gone, focus goes back to the button that opened it.
      await tick()
      if (back?.isConnected) back.focus()
    }
    window.addEventListener('pageshow', restored)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', shown); window.removeEventListener('pageshow', restored); life.abort() }
  })
</script>

<div class="home" class:writer>
  <h1 class="vh">Home</h1>
  {#if writer}
    <ProjectRail {projects} failed={projectsFailed} machines={health} {running} busy={checking} onadd={openProject} />
  {/if}
  <div class="column">
    <div class="live" role="status" aria-live="polite">{#if problem}<p class="problem">{problem}</p>{/if}</div>
    <FleetStrip {cards} sessions={writer && sessions !== undefined} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined} />
    {#if writer}
      <div class="live" role="status" aria-live="polite">{#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}{#if notice.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>{/if}</div>
      <div class="work">
        <NeedsYou cards={needs} />
        <InFlight {groups} {owners} machines={health ?? []} {offline} {now} loaded={sessions !== undefined} failed={sessionsFailed} {groupBy} onchoose={choose} />
      </div>
      <section class="dock" aria-label="Start an agent">
        <p>Start an agent in a new terminal session on one of your machines.</p>
        <button type="button" class="btn primary start" disabled={checking} aria-busy={checking} onclick={(e) => openStart(e.currentTarget)}>Start an agent</button>
      </section>
    {/if}
  </div>
</div>

{#if starting && terminals}
  <StartAgentDialog machines={terminals} snapshot={folders} opener={starting.opener} onstarted={started} onaddmachine={() => location.assign(machinesAt('add-machine'))} {onsignedout} onclose={() => (starting = null)} />
{/if}
{#if creating && terminals}
  <ProjectDialog machines={terminals} snapshot={folders} otherNames={(projects ?? []).map(p => p.name)} opener={creating.opener} onsaved={saved} {onsignedout} onclose={() => (creating = null)} />
{/if}

<style>
  /* The header is 61 px on a desktop, 65 px on a tablet and 57 px on a phone (measured at each breakpoint). These only set the
     column's minimum height, so the docked Start an agent starts at the foot of the first screen; if the header changes,
     the cost is a small gap or a little extra scroll, never an overlap. */
  .home { --top: 61px; display: grid; grid-template-columns: minmax(0, 1fr); }
  .home.writer { grid-template-columns: 272px minmax(0, 1fr); }
  .column { min-width: 0; display: flex; flex-direction: column; gap: 18px; padding: 20px 24px 0; min-height: calc(100dvh - var(--top) - var(--bottom-chrome, 0px)); }
  .home:not(.writer) .column { padding-bottom: 36px; }
  .problem, .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .problem { border-color: var(--warn); }
  .notice { border-color: var(--good); }
  .notice.bad { border-color: var(--bad); }
  /* The live regions are always present, so a sentence put into one is announced; empty, they take no room. */
  .live:empty { position: absolute; width: 1px; height: 1px; overflow: hidden; }
  .work { display: grid; grid-template-columns: minmax(0, 1fr) 380px; gap: 18px; align-items: start; }
  .work > :global(.needs) { grid-column: 2; grid-row: 1; }
  .work > :global(.flight) { grid-column: 1; grid-row: 1; }
  .dock {
    position: sticky; bottom: calc(var(--bottom-chrome, 0px) + 18px); z-index: 2; margin: auto 0 18px; display: flex; align-items: center; gap: 12px;
    padding: 12px 12px 12px 18px; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card-raised);
    box-shadow: 0 -8px 30px -12px color-mix(in srgb, var(--bg) 80%, transparent);
  }
  .dock p { flex-grow: 1; margin: 0; color: var(--muted); }
  .start { flex: none; min-height: 40px; padding: 0 18px; }

  @media (max-width: 1239px) { .home.writer { grid-template-columns: 56px minmax(0, 1fr); } }
  /* Tablet: no rail (it is reached from Sessions); machines, Needs you, then In flight, one column. */
  @media (max-width: 1099px) {
    .home { --top: 65px; }
    .home.writer { grid-template-columns: minmax(0, 1fr); }
    .home.writer > :global(.rail) { display: none; }
    .work { grid-template-columns: minmax(0, 1fr); }
    .work > :global(.needs), .work > :global(.flight) { grid-column: 1; grid-row: auto; }
    .start { min-height: 44px; padding: 0 22px; }
  }
  @media (pointer: coarse) { .start { min-height: 44px; } }
  /* Phone: one column with 16 px gutters; the docked button is a full-width 48 px primary action above the tab bar. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .home { --top: 57px; }
    .column { padding: 14px 16px 0; gap: 14px; }
    .dock { margin: auto -4px 8px; padding: 8px; bottom: calc(var(--bottom-chrome, 0px) + 8px); }
    .dock p { display: none; }
    .start { flex: 1; min-height: 48px; border-radius: 10px; }
  }
</style>
