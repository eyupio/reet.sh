<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { getProfile, getSyncState, UnauthorizedError, type SyncState } from '../api'
  import { deleteFolderProject, folderProjectList, getFolderProject, importSavedProjects, machineHealth, refusalSentence, reorderFolderProjects, terminalMachines,
    folderProjectForLegacy, sessionHref, terminalSessions, updateFolderProject, TerminalError, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { attentionCount, fleetCards, reporting, inFlight, needsYou, projectRunning, projectWorktrees, sessionOwners, type GroupBy, type HomeSession } from '../home'
  import { addedLine } from '../folderBrowser'
  import type { LaunchDefaults } from '../launchBar'
  import LaunchBar from '../launch/LaunchBar.svelte'
  import type { ProjectColour } from '../projectColour'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import AddProjectDialog from './AddProjectDialog.svelte'
  import ProjectWorktrees from './ProjectWorktrees.svelte'
  import FleetStrip from './FleetStrip.svelte'
  import InFlight from './InFlight.svelte'
  import NeedsYou from './NeedsYou.svelte'
  import ProjectRail from './ProjectRail.svelte'

  // Home on today's data. A writer in accounts mode gets the project rail, the fleet strip, In flight, Needs you and the docked
  // launch bar; everyone else signed in gets the fleet strip alone (R16). Everything comes from the existing loaders and
  // is read again every 15 seconds while the page is in view. A read that fails keeps what was last read and says so. Nothing
  // about Projects, sessions or machines goes into browser storage; the one stored value is the Group by choice.
  let { userId, writer, canAdd, accounts, moved = false, legacyProject = '', onsignedout, onattention }: {
    userId: number
    /** A writer in accounts mode: the full Home. Otherwise the read-only Home. */
    writer: boolean
    /** Whether this sign-in can add a machine. */
    canAdd: boolean
    accounts: boolean
    /** The page was opened at a retired /projects address: say once, for this page load, where projects live now. */
    moved?: boolean
    /** The saved Project id an old /projects/<id> address named: its folder project is looked up and chosen. */
    legacyProject?: string
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
  let projects = $state<FolderProject[] | undefined>()
  let savedWaiting = $state(0)
  // The project an address chose (/?project=<id>): its card is marked and the launch bar opens on it.
  let chosen = $state(new URLSearchParams(location.search).get('project') ?? '')
  // The account's launch defaults (base branch, worktrees by default), read once; the bar waits for the answer or its failure.
  let profile = $state<LaunchDefaults>({}), profileRead = $state(false)
  let bar = $state<LaunchBar>()
  let now = $state(Date.now())
  let problem = $state('')
  let readOnce = false
  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(POLL_MS)])

  const emptySync: SyncState = { environments: [], targets: [], agents: [], jobs: [] }
  const cards = $derived(fleetCards(health ?? [], sessions ?? [], sync ?? emptySync, now))
  const owners = $derived(sessionOwners(sessions ?? [], projects ?? []))
  const projectColours = $derived(new Map((projects ?? []).map(p => [p.id, p.colour])))
  const groups = $derived(inFlight(sessions ?? [], owners, health ?? [], groupBy, now))
  const shownMachines = $derived(new Set(groups.flatMap(g => g.sessions.map(s => s.agentId))))
  const offline = $derived(cards.filter(c => !reporting(c) && shownMachines.has(c.agentId)).map(c => ({ agentId: c.agentId, host: c.name })))
  const running = $derived(projectRunning(sessions ?? []))
  const worktrees = $derived(projectWorktrees(sessions ?? []))
  const machineReporting = $derived(new Map(cards.map(c => [c.agentId, reporting(c)])))
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
      const [h, y, s, p, t] = await Promise.allSettled([
        healthReadable ? machineHealth(bounded()) : Promise.resolve(undefined),
        getSyncState(bounded()),
        writer ? terminalSessions(bounded()) : Promise.resolve(undefined),
        writer ? folderProjectList(bounded()) : Promise.resolve(undefined),
        // The launch bar's machines: readiness and launch capabilities change while Home stays open, so they are read with the rest.
        writer ? terminalMachines(bounded()) : Promise.resolve(undefined),
      ])
      if (stale()) return
      if ([h, y, s, p, t].some(r => r.status === 'rejected' && refused(r.reason, r !== y))) return signOut()
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
        if (p.status === 'fulfilled' && p.value) { projects = p.value.projects; savedWaiting = p.value.savedWaiting ?? 0; projectsFailed = false; void revealChosen() } else { failed = true; projectsFailed = projects === undefined }
        if (t.status === 'fulfilled' && t.value) terminals = t.value; else failed = true
      }
      now = at
      problem = !failed ? '' : readOnce ? "Couldn't refresh just now. Showing what was last read." : "Couldn't read everything just now. Home tries again every 15 seconds."
      readOnce = true
    } finally {
      if (mine === generation) loading = false
    }
  }

  // Tablet and phone widths put the rail under In flight instead of beside it.
  const NARROW = '(max-width: 1099px)'
  let narrow = $state(typeof matchMedia === 'function' && matchMedia(NARROW).matches)

  // The launch bar and the Add project dialog need the terminal machines (with their readiness and launch capabilities).
  let terminals = $state<TerminalMachine[] | undefined>()
  let checking = $state(false)
  let notice = $state<{ text: string; href?: string; bad?: boolean } | null>(null)
  // The Add project dialog: a new project, or the one being changed (read with its starter prompt).
  let adding = $state<{ opener: HTMLElement | null; project?: FolderProject } | null>(null)
  let removing = $state<{ opener: HTMLElement | null; project: FolderProject } | null>(null)
  let listing = $state<{ opener: HTMLElement | null; project: FolderProject } | null>(null)
  let importing = $state(false)
  // One change at a time: a project with a dismiss or remove on its way, and a move on its way, ignore a second press, so
  // the second never meets the first one's new version as "someone changed this project".
  const pending = new SvelteSet<string>()
  let moving = $state(false)
  // Read again on every open of Add project, never from an earlier one: a machine's browse capability can change while Home
  // stays open. `checking` stops a second open racing the first read.
  async function machinesForDialog(): Promise<boolean> {
    checking = true
    try { terminals = await terminalMachines(bounded()); return true }
    catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return false }
      notice = { text: "Couldn't read your machines just now. Try again in a moment.", bad: true }
      return false
    } finally { checking = false }
  }
  async function openAdd(opener: HTMLElement) {
    if (checking) return
    notice = null
    if (await machinesForDialog()) adding = { opener }
  }
  // Choosing a card (or New session here) puts that project in the launch bar's sentence and moves to the prompt; the rail
  // marks the project the sentence names.
  let launchProject = $state('')
  function chooseProject(project: FolderProject) { bar?.choose(project) }
  // Edit reads the project again with its starter prompt (only the single read carries it), then opens the dialog.
  async function openEdit(project: FolderProject, opener: HTMLElement) {
    if (checking) return
    notice = null
    checking = true
    try {
      const [machines, full] = await Promise.all([terminalMachines(bounded()), getFolderProject(project.id, bounded())])
      terminals = machines
      adding = { opener, project: full }
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
      if (e instanceof TerminalError && e.status === 404) void load(true)
    } finally { checking = false }
  }
  // A write's answer is the list as it now is (or a project in it): it replaces what was read, superseding a poll in flight.
  function settle(next: FolderProject[]) { generation++; loading = false; projects = next }
  function added(p: FolderProject, wasEdit: boolean) {
    settle([...(projects ?? []).filter(x => x.id !== p.id), p].sort((a, b) => a.position - b.position))
    notice = { text: wasEdit ? `Sorted. ${p.name} saved.` : addedLine(p.name) }
  }
  // After a remove the focus goes to the card now in its place (or the one before, or Add); a refused remove gives it back
  // to the control that opened the confirmation.
  async function remove(project: FolderProject, opener: HTMLElement | null) {
    if (pending.has(project.id)) return
    pending.add(project.id)
    notice = null
    const at = (projects ?? []).findIndex(p => p.id === project.id)
    try {
      const answer = await deleteFolderProject(project.id, project.version, bounded())
      settle((projects ?? []).filter(p => p.id !== project.id))
      notice = { text: answer.message }
      await tick()
      const shown = (el: HTMLElement) => el.getClientRects().length > 0
      const cards = [...document.querySelectorAll<HTMLElement>('aside.rail .card, aside.rail .mini')].filter(shown)
      const add = [...document.querySelectorAll<HTMLElement>('aside.rail button[aria-label="Add a project"]')].find(shown)
      ;(cards[Math.min(Math.max(at, 0), cards.length - 1)] ?? add)?.focus()
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
      if (opener?.isConnected) opener.focus()
      void load(true)
    } finally { pending.delete(project.id) }
  }
  // Moving a card sends the whole order with the version of each project as read; a changed list is read again.
  async function move(project: FolderProject, by: -1 | 1) {
    // The order names every project's version as read: a dismiss or remove on its way would make it stale.
    if (moving || pending.size > 0) return
    const list = [...(projects ?? [])]
    const at = list.findIndex(p => p.id === project.id), to = at + by
    if (at < 0 || to < 0 || to >= list.length) return
    ;[list[at], list[to]] = [list[to], list[at]]
    notice = null
    moving = true
    try {
      settle(await reorderFolderProjects(list.map(p => p.id), list.map(p => p.version), bounded()))
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
      void load(true)
    } finally { moving = false }
  }
  // A rail colour is saved on the project, so it follows the person to every browser. The card shows it at once; a
  // refusal puts the project back and says why.
  async function colour(project: FolderProject, choice: ProjectColour) {
    if (pending.has(project.id) || (project.colour || '') === choice) return
    pending.add(project.id)
    notice = null
    const before = projects ?? []
    settle(before.map(p => p.id === project.id ? { ...p, colour: choice } : p))
    try {
      const saved = await updateFolderProject(project.id, { colour: choice }, project.version, bounded())
      settle((projects ?? []).map(p => p.id === saved.id ? saved : p))
    } catch (e) {
      settle(before)
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
      void load(true)
    } finally { pending.delete(project.id) }
  }
  // The migration's note goes once the person has read it: clearing it is the one change made.
  async function dismiss(project: FolderProject) {
    if (pending.has(project.id)) return
    pending.add(project.id)
    try {
      const saved = await updateFolderProject(project.id, { note: '' }, project.version, bounded())
      settle((projects ?? []).map(p => p.id === saved.id ? saved : p))
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
      void load(true)
    } finally { pending.delete(project.id) }
  }
  // The saved Project folders that waited for room come in only when the person asks; the answer is said as it came.
  async function bringIn() {
    if (importing) return
    importing = true; notice = null
    try {
      const answer = await importSavedProjects(bounded())
      settle(answer.projects)
      savedWaiting = answer.savedWaiting
      notice = { text: answer.message }
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      notice = { text: refusalSentence(e), bad: true }
    } finally { importing = false }
  }
  // On a tablet or phone the rail sits below In flight: a project chosen by the address (Jump, an old link) is scrolled into
  // view once and named in the status line, so the person sees where it went.
  let revealed = false
  async function revealChosen() {
    if (revealed || !chosen || !narrow) return
    const p = projects?.find(x => x.id === chosen)
    if (!p) return
    revealed = true
    await tick()
    document.querySelector<HTMLElement>('aside.rail .card[aria-current="true"]')?.scrollIntoView({ block: 'center' })
    notice = { text: `Showing ${p.name} in Projects below.` }
  }
  // An old /projects/<id> address: its folder project, if one was made from it, is chosen and named in the address.
  async function findLegacy(id: string) {
    try {
      const found = await folderProjectForLegacy(id, bounded())
      if (!found || life.signal.aborted) return
      chosen = found
      history.replaceState(history.state, '', `/?project=${encodeURIComponent(found)}`)
      void revealChosen()
    } catch { /* no folder project was made from it: Home stays at / */ }
  }

  // A start goes straight to the new session, at the address the Machines and Terminals pages use. The row is added first, so
  // Home has it if the browser brings this page back from its cache on Back; the bar stays locked until the page goes.
  function started(session: TerminalSession): 'leaving' {
    sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
    location.assign(sessionHref(session))
    return 'leaving'
  }
  // The next page never arrived (the bar let go after a few seconds): say what started, with the way to it.
  function leaveFailed(session: TerminalSession) {
    const host = health?.find(m => m.agentId === session.agentId)?.host ?? terminals?.find(m => m.agentId === session.agentId)?.host ?? session.agentId
    notice = { text: `Started ${session.title} on ${host}.`, href: sessionHref(session) }
    void load(true)
  }

  // The redirect note goes into its live region after the page is drawn, so it is announced.
  let movedShown = $state(false)
  onMount(() => {
    if (moved) void tick().then(() => { movedShown = true })
    void load()
    // The profile read gives up after 10 seconds, so the bar never waits on it for good (it then uses each project's own base).
    let giveUpTimer: ReturnType<typeof setTimeout> | undefined
    if (writer) Promise.race([getProfile(), new Promise<never>((_, no) => { giveUpTimer = setTimeout(() => no(new Error('timeout')), 10_000) })]).then(p => { profile = { defaultBaseBranch: p.profile.defaultBaseBranch, worktreesByDefault: p.profile.worktreesByDefault } }, () => {}).finally(() => { profileRead = true; clearTimeout(giveUpTimer) })
    if (writer && legacyProject) void findLegacy(legacyProject)
    const query = matchMedia(NARROW), resized = () => { narrow = query.matches }
    query.addEventListener('change', resized)
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const shown = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', shown)
    // Back from the session the person just started: a page restored from the browser's cache shows lists as they were, so
    // it reads everything again. (The launch bar unlocks itself.)
    const restored = (e: PageTransitionEvent) => { if (e.persisted) void load(true) }
    window.addEventListener('pageshow', restored)
    return () => { query.removeEventListener('change', resized); clearInterval(timer); document.removeEventListener('visibilitychange', shown); window.removeEventListener('pageshow', restored); life.abort() }
  })
</script>

<div class="home" class:writer>
  <h1 class="vh">Home</h1>
  {#snippet rail()}
    <ProjectRail {projects} failed={projectsFailed} reporting={machineReporting} {running} known={sessions !== undefined} {worktrees} {savedWaiting} chosen={launchProject || chosen} busy={checking} {importing} {pending} {moving}
      onadd={openAdd} onchoose={(project) => chooseProject(project)} onedit={openEdit} onworktrees={(project, opener) => (listing = { project, opener })}
      onremove={(project, opener) => (removing = { project, opener })} onmove={move} ondismiss={dismiss} onimport={bringIn} oncolour={colour} />
  {/snippet}
  <!-- On a desktop the rail is the left column; on a tablet or phone it sits under In flight (the Sessions tab takes it later). -->
  {#if writer && !narrow}{@render rail()}{/if}
  <div class="column">
    <div class="live" role="status" aria-live="polite">{#if movedShown}<p class="notice">{writer ? 'Projects now live in the rail on Home.' : 'Projects are on Home for accounts that can start sessions.'}</p>{/if}{#if problem}<p class="problem">{problem}</p>{/if}</div>
    <FleetStrip {cards} sessions={writer && sessions !== undefined} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined} />
    {#if writer}
      <div class="live" role="status" aria-live="polite">{#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}{#if notice.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>{/if}</div>
      <div class="work">
        <NeedsYou cards={needs} />
        <InFlight {groups} {owners} machines={health ?? []} {offline} {now} loaded={sessions !== undefined} failed={sessionsFailed} {groupBy} onchoose={choose} {projectColours} />
      </div>
      {#if narrow}{@render rail()}{/if}
      <LaunchBar bind:this={bar} {projects} machines={terminals} sessions={sessions} {profile} initialProject={chosen}
        ready={projects !== undefined && (sessions !== undefined || sessionsFailed) && profileRead}
        onproject={(id) => (launchProject = id)} onstarted={started} onleavefailed={leaveFailed} onaddproject={openAdd} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />
    {/if}
  </div>
</div>

{#if adding && terminals}
  {@const wasEdit = !!adding.project}
  <AddProjectDialog machines={terminals} project={adding.project} opener={adding.opener} onadded={(p) => added(p, wasEdit)} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} onclose={() => (adding = null)} />
{/if}
{#if removing}
  {@const p = removing.project}
  <ConfirmDialog title={`Remove ${p.name} from reet?`} body={`It leaves the rail. Its sessions and any worktrees stay on ${p.machine || 'its machine'}, and the folder isn't touched.`}
    confirmLabel="Remove from reet" danger onconfirm={() => { const target = p, opener = removing?.opener ?? null; removing = null; void remove(target, opener) }} oncancel={() => { const o = removing?.opener; removing = null; o?.focus() }} />
{/if}
{#if listing}
  <ProjectWorktrees project={listing.project} {sessions} {sessionsFailed} opener={listing.opener} {onsignedout} onclose={() => (listing = null)} />
{/if}

<style>
  /* The header is 61 px on a desktop, 65 px on a tablet and 57 px on a phone (measured at each breakpoint). These only set the
     column's minimum height, so the docked launch bar starts at the foot of the first screen; if the header changes,
     the cost is a small gap or a little extra scroll, never an overlap. */
  .home { line-height: 1.3; --top: 61px; display: grid; grid-template-columns: minmax(0, 1fr); }
  .home :global(p) { line-height: 1.55; }
  /* The rail is the mock-up's 272 px of cards inside 14 px padding and a 1 px rule: 301 px. */
  .home.writer { grid-template-columns: 301px minmax(0, 1fr); }
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

  @media (max-width: 1239px) { .home.writer { grid-template-columns: 56px minmax(0, 1fr); } }
  /* Tablet: machines, Needs you, In flight, then the project rail, one column (the Sessions tab takes the rail later). */
  @media (max-width: 1099px) {
    .home { font-size: 15px; --top: 65px; }
    .home.writer { grid-template-columns: minmax(0, 1fr); }
    .work { grid-template-columns: minmax(0, 1fr); }
    .work > :global(.needs), .work > :global(.flight) { grid-column: 1; grid-row: auto; }
  }
  /* Phone: one column with 16 px gutters; the launch bar sits above the tab bar. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .home { --top: 57px; }
    .column { padding: 14px 16px 0; gap: 14px; }
  }
</style>
