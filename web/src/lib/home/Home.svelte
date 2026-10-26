<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, UnauthorizedError, type Machine, type SyncState } from '../api'
  import { getRun, listProjects, machineHealth, terminalMachines, terminalSessions, TerminalError, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import type { Project, ProjectRun } from '../projects'
  import { attentionCount, fleetCards, inFlight, needsYou, projectRunning, recentRunIds, sessionProjects, type GroupBy, type HomeSession } from '../home'
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
  const offline = $derived(cards.filter(c => c.status !== 'online' && shownMachines.has(c.agentId)).map(c => ({ agentId: c.agentId, host: c.name })))
  const running = $derived(projectRunning(sessions ?? [], owners))
  const needs = $derived(needsYou(sessions ?? [], now))
  $effect(() => { onattention?.(attentionCount(needs)) })

  const signedOut = (e: unknown) => e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)
  // A read-only sign-in may have no health report to read (a single token or an open instance): the sync agents then name
  // the machines, connected when the agent reported in the last 90 seconds, as Control says.
  const fromAgents = (s: SyncState, at: number): MachineHealth[] =>
    (s.agents ?? []).map(a => ({ agentId: a.id, host: a.host, state: !a.paused && a.lastSeen > at - 90_000 ? 'connected' : 'never_connected' }))

  let loading = false
  async function load() {
    if (loading || life.signal.aborted) return
    loading = true
    try {
      const [h, y, s, p] = await Promise.allSettled([
        machineHealth(bounded()),
        getSyncState(bounded()),
        writer ? terminalSessions(bounded()) : Promise.resolve(undefined),
        writer ? listProjects(bounded()) : Promise.resolve(undefined),
      ])
      if (life.signal.aborted) return
      const results = [h, y, s, p]
      const lost = results.find(r => r.status === 'rejected' && signedOut(r.reason))
      if (lost) { life.abort(); onsignedout(new UnauthorizedError()); return }
      const at = Date.now()
      let failed = false
      if (y.status === 'fulfilled') sync = y.value; else failed = true
      if (h.status === 'fulfilled') health = h.value.machines ?? []
      else if (!writer && y.status === 'fulfilled') health = fromAgents(y.value, at)
      else failed = true
      if (writer) {
        if (s.status === 'fulfilled' && s.value) sessions = s.value; else failed = true
        if (p.status === 'fulfilled' && p.value) projects = p.value; else failed = true
        // Each Project's last run, only when it started in the last day: its members carry the session ids. A run that
        // cannot be read keeps what it last said.
        const ids = recentRunIds(projects ?? [], at)
        const read = await Promise.allSettled(ids.map(id => getRun(id, bounded())))
        if (life.signal.aborted) return
        const next = new Map<string, ProjectRun>()
        ids.forEach((id, i) => {
          const r = read[i]
          if (r.status === 'fulfilled') next.set(id, r.value)
          else { failed = true; const kept = runs.get(id); if (kept) next.set(id, kept) }
        })
        if (read.some(r => r.status === 'rejected' && signedOut(r.reason))) { life.abort(); onsignedout(new UnauthorizedError()); return }
        runs = next
      }
      now = at
      problem = !failed ? '' : readOnce ? "Couldn't refresh just now. Showing what was last read." : "Couldn't read everything just now. Home tries again every 15 seconds."
      readOnce = true
    } finally {
      loading = false
    }
  }

  // The dialogs need the terminal machines (with their launch capabilities) and the reported folders.
  let terminals = $state<TerminalMachine[] | undefined>()
  let checking = $state(false)
  let notice = $state<{ text: string; href?: string; bad?: boolean } | null>(null)
  let starting = $state<{ opener: HTMLElement | null } | null>(null)
  let creating = $state<{ opener: HTMLElement | null } | null>(null)
  const folders = $derived<Machine[] | undefined>(sync?.machines)
  async function machinesForDialog(): Promise<boolean> {
    if (terminals) return true
    checking = true
    try { terminals = await terminalMachines(bounded()); return true }
    catch (e) {
      if (signedOut(e)) { life.abort(); onsignedout(new UnauthorizedError()); return false }
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
  function started(session: TerminalSession) {
    sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
    const host = health?.find(m => m.agentId === session.agentId)?.host ?? terminals?.find(m => m.agentId === session.agentId)?.host ?? session.agentId
    notice = { text: `Started ${session.title} on ${host}.`, href: `/terminals?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}` }
    void load()
  }
  function saved(p: Project) {
    projects = [p, ...(projects ?? []).filter(x => x.id !== p.id)]
    notice = { text: `${p.name} is saved.`, href: `/projects/${encodeURIComponent(p.id)}` }
    void load()
  }

  onMount(() => {
    void load()
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const shown = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', shown)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', shown); life.abort() }
  })
</script>

<div class="home" class:writer>
  <h1 class="vh">Home</h1>
  {#if writer}
    <ProjectRail {projects} machines={health} {running} busy={checking} onadd={openProject} />
  {/if}
  <div class="column">
    <p class="problem" role="status" aria-live="polite" hidden={!problem}>{problem}</p>
    <FleetStrip {cards} sessions={writer} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined} />
    {#if writer}
      <p class="notice" class:bad={notice?.bad} role="status" aria-live="polite" hidden={!notice}>{notice?.text}{#if notice?.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>
      <div class="work">
        <NeedsYou cards={needs} />
        <InFlight {groups} {owners} machines={health ?? []} {offline} {now} loaded={sessions !== undefined} {groupBy} onchoose={choose} />
      </div>
      <section class="dock" aria-label="Start a session">
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
  /* The header is 61 px on a desktop, 65 px on a tablet and 57 px on a phone; the column fills the rest of the screen so
     the docked Start an agent sits at its foot and stays there while the page scrolls. */
  .home { --top: 61px; display: grid; grid-template-columns: minmax(0, 1fr); }
  .home.writer { grid-template-columns: 272px minmax(0, 1fr); }
  .column { min-width: 0; display: flex; flex-direction: column; gap: 18px; padding: 20px 24px 0; min-height: calc(100dvh - var(--top) - var(--bottom-chrome, 0px)); }
  .home:not(.writer) .column { padding-bottom: 36px; }
  .problem, .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .problem { border-color: var(--warn); }
  .notice { border-color: var(--good); }
  .notice.bad { border-color: var(--bad); }
  .problem[hidden], .notice[hidden] { display: none; }
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
