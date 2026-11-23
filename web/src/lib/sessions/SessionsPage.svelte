<script lang="ts">
  import { projectRepo, projectSource } from '../githubRepo'
  import { onMount, tick } from 'svelte'
  import { onAttentionChanged } from '../attentionSignal'
  import { getProfile, listGafferOutcomes, UnauthorizedError } from '../api'
  import CommandLibrary from '../terminal/CommandLibrary.svelte'
  import ProjectRailHost from '../home/ProjectRailHost.svelte'
  import ProjectWorktrees from '../home/ProjectWorktrees.svelte'
  import { GROUP_CHOICES, needingIds, needsYou, projectWorktrees, railState, sessionOwners, shortAge, stateWord, type GroupBy } from '../home'
  import { offDesks, type DeskRef } from '../home/deskSessions'
  import { sessionStatus } from '../sessionStatus'
  import { PROJECT_COLOURS, projectColourHex } from '../projectColour'
  import { getDesk, listDesks } from '../desks/api'
  import { agentChoicesOf, deskPlaces, sessionColourOf, filterSessions, filtersOn, offlineSentence, rowMeta, searchSessions, sessionGroups, tabFromMenu, type DeskPlace, type MachineName, type SessionFilter, type SessionSort } from '../sessions'
  import { folderProjectList, listProjectGroups, machineHealth, sessionHref, terminalMachines, terminalSessions, TerminalError,
    type FolderProject, type MachineHealth, type ProjectGroup, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import SessionActions from './SessionActions.svelte'
  import { FACTORY_SESSION_END_DEFAULT, factoryMembers, factorySessionEndModeOf, offFinishedFactory, type FactoryMember, type FactorySessionEndMode } from '../gafferOutcome'

  // The Sessions page (/sessions): every session across machines from the account-wide listing, grouped as Home groups them
  // (the Group by choice is Home's), with search, filters and each session's actions menu; the projects come below with
  // Add a project. Read again every 15 seconds while in view. A read that fails keeps the last list and says so. Sessions on
  // a desk are listed here only when they need the person (the desk is where they are seen), as on Home. Only the
  // Group by choice is stored (Home's key); the search and filters live on this page only. On a tablet or phone the projects
  // are the full project rail (the phone rail, moved here from Home): its menus, Select and the state words.
  // The bell and the title count by the app's one rule (App reads it on this page as on every page but Home); this page
  // reads its list again whenever something is answered or marked seen there.
  let { userId, onsignedout }: {
    userId: number
    onsignedout: (e: unknown) => void
  } = $props()

  const POLL_MS = 15_000
  // The Machines page opens Add a machine when it arrives with this fragment.
  const machinesAt = (fragment: string) => `/machines#${fragment}`
  const GROUPS: GroupBy[] = GROUP_CHOICES.map(choice => choice.value)
  // svelte-ignore state_referenced_locally
  const groupKey = `reet.home.groupBy.${userId}`
  function storedGroup(): GroupBy {
    try { const v = localStorage.getItem(groupKey); return GROUPS.includes(v as GroupBy) ? v as GroupBy : 'project' } catch { return 'project' }
  }
  let groupBy = $state<GroupBy>(storedGroup())
  function chooseGroup(next: GroupBy) {
    groupBy = next
    try { localStorage.setItem(groupKey, next) } catch { /* the choice lasts for this page only */ }
  }
  // The custom order and the session colours the sessions panel saved in this browser, offered as a sort and a filter.
  // svelte-ignore state_referenced_locally
  const panelKey = `reet-terminal-layout:${userId}:sessions`
  function savedPanel(): { order: string[]; colors: Record<string, string> } {
    try {
      const saved = JSON.parse(localStorage.getItem(panelKey) || '{}')
      const order = Array.isArray(saved.order) ? saved.order.filter((v: unknown) => typeof v === 'string').slice(0, 2000) : []
      const colors = saved.colors && typeof saved.colors === 'object' && !Array.isArray(saved.colors) ? Object.fromEntries(Object.entries(saved.colors).filter((e): e is [string, string] => typeof e[1] === 'string')) : {}
      return { order, colors }
    } catch { return { order: [], colors: {} } }
  }
  const { order, colors } = savedPanel()

  let sessions = $state<TerminalSession[] | undefined>()
  // The desks, read with the sessions: a session on a desk leaves this page, as it leaves Home's lists, and only one that
  // needs the person stays. A failed read keeps the last list (no problem line): without desks every session shows.
  let desks = $state<DeskRef[] | undefined>()
  // Which sessions are part of a Factory pipeline, read with the sessions so a stage the Factory has finished with can
  // leave the list: its pipeline is the item that says what became of it. A failed read keeps the last map.
  let factory = $state<Map<string, FactoryMember>>(new Map())
  let projects = $state<FolderProject[] | undefined>()
  let projectGroups = $state<ProjectGroup[] | undefined>()
  let terminals = $state<TerminalMachine[] | undefined>()
  let health = $state<MachineHealth[] | undefined>()
  let now = $state(Date.now())
  let checking = $state(false), failed = $state(false), projectsFailed = $state(false), groupsFailed = $state(false), denied = $state(false)
  let query = $state(''), sort = $state<SessionSort>('recent')
  let filter = $state<SessionFilter>({ state: '', agent: '', machine: '', yoloOnly: false, worktreeOnly: false, colour: '' })
  let notice = $state<{ text: string; bad?: boolean } | null>(null)
  let savedWaiting = $state(0)
  // Tablet and phone widths (the spec's breakpoints): the full rail; a desktop keeps the simple cards (Home has the rail).
  const NARROW = '(max-width: 1099px)'
  let narrow = $state(typeof matchMedia === 'function' && matchMedia(NARROW).matches)
  // Select in the rail: there is no launch bar here, so the chosen projects go to Home's in the address (ids only).
  let selecting = $state(false), selected = $state<string[]>([])
  const inRailOrder = (ids: string[]) => (projects ?? []).filter(p => ids.includes(p.id)).map(p => p.id)
  function setSelecting(on: boolean, first?: FolderProject) { selecting = on; selected = on && first ? [first.id] : [] }
  function toggleSelected(p: FolderProject) { selected = inRailOrder(selected.includes(p.id) ? selected.filter(x => x !== p.id) : [...selected, p.id]) }
  $effect(() => { const list = projects; if (list && !selected.every(x => list.some(p => p.id === x))) selected = inRailOrder(selected) })

  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(POLL_MS)])
  const signOut = () => { life.abort(); onsignedout(new UnauthorizedError()) }
  const refused = (e: unknown) => e instanceof TerminalError && e.status === 401

  // Machine names from the terminal machines, then any the health report adds.
  const machines = $derived.by<MachineName[]>(() => {
    const out: MachineName[] = (terminals ?? []).map(m => ({ agentId: m.agentId, host: m.host }))
    for (const m of health ?? []) if (!out.some(x => x.agentId === m.agentId)) out.push({ agentId: m.agentId, host: m.host })
    return out
  })
  // A machine the health report says is not connected is offline; with no report nothing is called offline.
  const offline = $derived(new Set((health ?? []).filter(m => m.state !== 'connected').map(m => m.agentId)))
  // Which desk each session is on, read once as the page opens (and again after Add to a desk): the desks list names the
  // sessions each desk holds, and only the desks holding one are read for the pane and its colour.
  // Only the desks holding a session this page lists are read; a read that finishes after a newer one started is dropped.
  let places = $state<Map<string, DeskPlace>>(new Map())
  let placesRead = 0
  async function loadPlaces() {
    const mine = ++placesRead
    try {
      // Read the desks fresh: an Add to or a move between desks changes what each holds before the page's next poll.
      const list = await listDesks(bounded())
      if (life.signal.aborted || mine !== placesRead) return
      desks = list
      const listedIds = new Set((listed ?? []).map(s => s.id))
      const holding = list.filter(d => !d.template && d.sessions.some(id => listedIds.has(id)))
      const read = await Promise.allSettled(holding.map(d => getDesk(d.id)))
      if (life.signal.aborted || mine !== placesRead) return
      places = deskPlaces(read.flatMap(r => r.status === 'fulfilled' ? [r.value] : []))
    } catch { /* the rows show no desk; nothing else depends on it */ }
  }
  const colourOf = (s: TerminalSession) => sessionColourOf(s, colors, places.get(s.id)?.colour, projects ?? [])
  const colourName = (id: string) => PROJECT_COLOURS.find(c => c.id === id)?.name ?? ''
  const cards = $derived(needsYou(sessions ?? [], now, sessionOwners(sessions ?? [], projects ?? []), machines))
  // By the bell's rule: a session that only finished is not "Needs you".
  const needing = $derived(needingIds(cards))
  // A session on a (non-template) desk belongs to the desk, not this page: it is listed here only when it needs the person.
  // With no desks read (a failed read, or before the first one) every session is listed, as it was before desks.
  const listed = $derived(offFinishedFactory(offDesks(sessions ?? [], desks, needing), factory))

  const shown = $derived(filterSessions(searchSessions(listed, projects ?? [], machines, query), filter, needing, colourOf))
  const groups = $derived(sessionGroups(shown, projects ?? [], machines, groupBy, offline, sort, order))
  const agents = $derived(agentChoicesOf(listed))
  const on = $derived(filtersOn(filter))
  const running = $derived.by(() => { const m = new Map<string, number>(); for (const s of listed) if (s.projectId && (s.state === 'running' || s.state === 'creating')) m.set(s.projectId, (m.get(s.projectId) ?? 0) + 1); return m })
  const worktrees = $derived(projectWorktrees(listed))
  // Whether each machine reports now (the rail's state words); a machine without a report is not called offline.
  const reportingNow = $derived(new Map((health ?? []).map(m => [m.agentId, m.state === 'connected'])))
  // A write's answer from the rail is the list as it now is: it supersedes a read on its way.
  function settle(next: FolderProject[]) { generation++; loading = false; checking = false; projects = next }
  function settleGroups(next: ProjectGroup[]) { generation++; loading = false; checking = false; projectGroups = next }

  let loading = false, generation = 0
  async function load(explicit = false) {
    if (life.signal.aborted || (loading && !explicit)) return
    const mine = ++generation
    loading = true; checking = true
    try {
      const [s, p, t, h, g, d, o] = await Promise.allSettled([terminalSessions(bounded()), folderProjectList(bounded()), terminalMachines(bounded()), machineHealth(bounded()), listProjectGroups(bounded()), listDesks(bounded()), listGafferOutcomes(bounded())])
      if (life.signal.aborted || mine !== generation) return
      if ([s, p, t, h].some(r => r.status === 'rejected' && refused(r.reason))) return signOut()
      // An account that lost terminal access (403) is shown nothing it can no longer open, as the workspace does.
      denied = [s, t].some(r => r.status === 'rejected' && r.reason instanceof TerminalError && r.reason.status === 403)
      if (denied) { sessions = []; terminals = []; failed = false; return }
      let bad = false
      if (s.status === 'fulfilled') sessions = s.value; else bad = true
      if (t.status === 'fulfilled') terminals = t.value; else bad = true
      // The projects keep what was last read; only a first read that fails says so, in their own section.
      if (p.status === 'fulfilled') { projects = p.value.projects; savedWaiting = p.value.savedWaiting ?? 0; projectsFailed = false } else projectsFailed = projects === undefined
      // Without groups the rail shows every project ungrouped; a failed read keeps what it had.
      if (g.status === 'fulfilled') { projectGroups = g.value; groupsFailed = false } else groupsFailed = projectGroups === undefined
      // A health read that fails keeps the last one: the offline sentences stay as last known.
      if (h.status === 'fulfilled') health = h.value.machines ?? []
      // A desk read that fails keeps the last list and says nothing: without desks every session shows, as before desks.
      if (d.status === 'fulfilled') desks = d.value
      // A Factory read that fails keeps the last map and, like the desks, never adds a problem line: with nothing read
      // every session shows. An account without Gaffer is not a failure.
      if (o.status === 'fulfilled') factory = factoryMembers(o.value)
      failed = bad
      now = Date.now()
      // A session already on a desk that starts needing the person enters the list now: read its desk for the chip, pane
      // and colour, as the first read does for one that was already waiting.
      const held = new Set((desks ?? []).flatMap(m => m.template ? [] : m.sessions))
      if ((sessions ?? []).some(s => held.has(s.id) && needing.has(s.id) && !places.has(s.id))) void loadPlaces()
    } finally {
      if (mine === generation) { loading = false; checking = false }
    }
  }

  // The actions menu: right-click on a row, Shift+F10 or the menu key on it, or its \u22ee button. SessionActions draws it,
  // moves between its items, and gives focus back to what opened it.
  let actions = $state<SessionActions>()
  let openActionsFor = $state('')
  const menuKeyOpens = (event: KeyboardEvent) => event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')
  const openMenu = (event: MouseEvent | KeyboardEvent, item: TerminalSession) => void actions?.openMenu(event, item)
  let removeDefault = $state(false)
  let factorySessionEndDefault = $state<FactorySessionEndMode>(FACTORY_SESSION_END_DEFAULT)

  // A project card's menu: right-click or Shift+F10 offers the ways into the project that fit here. Edit and remove stay on
  // Home's rail; Worktrees opens the existing sheet for the folder.
  let projectMenu = $state<{ item: FolderProject; x: number; y: number; opener: HTMLElement } | null>(null)
  let projectMenuEl = $state<HTMLElement>()
  async function openProjectMenu(event: MouseEvent | KeyboardEvent, item: FolderProject) {
    if (event.type === 'contextmenu' && event.shiftKey) return
    event.preventDefault()
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    const x = 'clientX' in event && event.clientX ? event.clientX : box.left, y = 'clientY' in event && event.clientY ? event.clientY : box.bottom
    projectMenu = { item, opener, x: Math.min(Math.max(8, x), Math.max(8, innerWidth - 248)), y: Math.max(8, Math.min(y, innerHeight - 190)) }
    await tick()
    projectMenuEl?.querySelector<HTMLElement>('[role=menuitem]')?.focus()
  }
  function closeProjectMenu(focus = true) {
    const opener = projectMenu?.opener
    projectMenu = null
    if (focus) opener?.focus()
  }
  function projectMenuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); closeProjectMenu(); return }
    if (event.key === 'Tab') { event.preventDefault(); const opener = projectMenu?.opener; closeProjectMenu(false); tabFromMenu(opener, event.shiftKey); return }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = [...projectMenuEl!.querySelectorAll<HTMLElement>('[role=menuitem]')], at = items.indexOf(document.activeElement as HTMLElement)
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
  }
  let worktreesFor = $state<{ project: FolderProject; opener: HTMLElement } | null>(null)
  function openWorktrees() {
    const target = projectMenu
    closeProjectMenu(false)
    if (target) worktreesFor = { project: target.item, opener: target.opener }
  }

  // Add a project goes through the rail's host at every width, so the desktop list offers From GitHub (and follows a
  // clone to its end) as the phone rail and Home do. The host reads the machines again first.
  let projectHost = $state<ProjectRailHost>()
  // Saved commands (owner ruling): the account's command library, without opening a session. Insert needs a live pane,
  // so it is not offered here; the pane's own entry stays.
  let commandsOpen = $state(false), commandsButton = $state<HTMLButtonElement>()
  async function closeCommands() { commandsOpen = false; await tick(); commandsButton?.focus() }

  const projectLabel = (p: FolderProject) => [p.name, p.machine || 'machine not enrolled', projectSource(p), railState(sessions ? running.get(p.id) ?? 0 : null, health?.some(m => m.agentId === p.host) ? !offline.has(p.host) : undefined)?.text].filter(Boolean).join(', ')
  // Nothing to list and nothing to filter: the page's one action is in the empty state, so the header carries no second New.
  const firstRun = $derived(sessions !== undefined && sessions.length === 0 && terminals !== undefined && !denied)
  const clearFilters = () => { filter = { state: '', agent: '', machine: '', yoloOnly: false, worktreeOnly: false, colour: '' } }

  onMount(() => {
    void load()
    void getProfile().then(p => { removeDefault = p.profile.terminalRemoveOnClose ?? false; factorySessionEndDefault = factorySessionEndModeOf(p.profile.factorySessionEndMode) }).catch(() => { /* the safe default keeps history */ })
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const wake = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', wake)
    const query = matchMedia(NARROW), resized = () => { narrow = query.matches }
    query.addEventListener('change', resized)
    const offAttention = onAttentionChanged(() => { void load(true) })
    return () => { offAttention(); clearInterval(timer); document.removeEventListener('visibilitychange', wake); query.removeEventListener('change', resized); life.abort() }
  })
</script>

<div class="sessions-page">
  <header class="page-head">
    <a class="back" href="/" aria-label="Back to Home"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg></a>
    <h1>Sessions</h1>
    <button type="button" class="commands" aria-label="Saved commands" bind:this={commandsButton} onclick={() => (commandsOpen = true)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 7l4 5-4 5M12 17h7" /></svg><span class="wide">Saved commands</span><span class="narrow">Commands</span></button>
    {#if !firstRun}<a class="new" href="/#launch" aria-label="New session">New</a>{/if}
  </header>

  <div class="tools">
    <label class="search">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
      <input type="search" bind:value={query} placeholder="Search sessions" aria-label="Search sessions" autocomplete="off" />
    </label>
    <div class="group-by" role="radiogroup" aria-label="Group by">
      {#each GROUP_CHOICES as { value, label } (value)}
        <label class:on={groupBy === value}><input type="radio" name="sessions-group" {value} checked={groupBy === value} onchange={() => chooseGroup(value)} />{label}</label>
      {/each}
    </div>
    <details class="filters">
      <summary><span>Filters</span>{#if on}<span class="on-count"> · {on} on</span>{/if}</summary>
      <div class="filter-grid">
        <label>State<select aria-label="State" bind:value={filter.state}><option value="">Any state</option><option value="needs">Needs you</option><option value="running">Running</option><option value="finished">Finished</option><option value="ended">Ended</option></select></label>
        <label>Agent<select aria-label="Agent" bind:value={filter.agent}><option value="">Any agent</option>{#each agents as a (a.value)}<option value={a.value}>{a.label}</option>{/each}</select></label>
        <label>Machine<select aria-label="Machine" bind:value={filter.machine}><option value="">Any machine</option>{#each machines as m (m.agentId)}<option value={m.agentId}>{m.host}</option>{/each}</select></label>
        <label>Colour<select aria-label="Colour" bind:value={filter.colour}><option value="">Any colour</option>{#each PROJECT_COLOURS.filter(c => c.id) as c (c.id)}<option value={c.id}>{c.name}</option>{/each}<option value="none">No colour</option></select></label>
        <label>Sort<select aria-label="Sort" bind:value={sort}><option value="recent">Recently changed</option><option value="name">Name</option><option value="custom">Custom order</option></select></label>
        <label class="check"><input type="checkbox" bind:checked={filter.yoloOnly} />YOLO only</label>
        <label class="check"><input type="checkbox" bind:checked={filter.worktreeOnly} />Worktree only</label>
        <button type="button" class="clear" disabled={on === 0} onclick={clearFilters}>Clear filters</button>
      </div>
    </details>
  </div>

  <!-- One reserved line for the notice and "Checking…", so the rows below never move when a read starts or ends (a tap
       aimed at a row lands on it). -->
  <div class="status-line">
    <div class="live" role="status" aria-live="polite">
      {#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}</p>{/if}
    </div>
    <!-- Not a live region: a read every 15 seconds would be announced each time. -->
    {#if checking}<p class="checking">Checking…</p>{/if}
  </div>
  {#if failed}<p class="problem" role="alert"><span>{sessions === undefined ? "Couldn't read your sessions just now." : "Couldn't refresh. Showing what we last saw."}</span> <button type="button" class="again" onclick={() => void load(true)}>Try again</button></p>{/if}

  {#if denied}
    <p class="none" role="alert">Your account no longer has terminal access. Sign in again or ask the owner to check your permissions. Existing sessions were not ended.</p>
  {:else if terminals !== undefined && terminals.length === 0 && (sessions ?? []).length === 0}
    <div class="first-machine"><h2>Connect your first machine</h2><p>Enroll a machine and enable terminal access locally to start sessions here.</p><a class="btn primary" href={machinesAt('add-machine')}>Connect a machine</a></div>
  {:else if sessions !== undefined}
    {#if listed.length === 0}
      {#if (sessions ?? []).length === 0}<div class="first-machine"><h2>No sessions yet</h2><p>Start an agent or a terminal on one of your machines.</p><a class="btn primary" href="/#launch" aria-label="New session">Start a session</a></div>
      {:else}<p class="none">Every session here is on a desk. Open Desks to see them.</p>{/if}
    {:else if groups.length === 0}
      <p class="none">{filter.state === 'needs' ? 'Nothing marks a session as needing you yet, so none is listed here.' : on > 0 && !query.trim() ? 'No sessions match these filters.' : 'No sessions match.'}</p>
    {:else}
      <div class="groups">
        {#each groups as g (g.key)}
          <section class="group" aria-label={g.label}>
            <h2>
              {#if g.project}<svg class="mark" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l10 10-10 10L2 12z" /></svg>{/if}
              <span class="name">{g.label}</span><span class="count"><span class="vh">, </span>{g.count}<span class="vh"> {g.count === 1 ? 'session' : 'sessions'}</span></span>
              {#if g.machine}<span class="machine-pill" title={g.machine}>{g.machine}</span>{/if}
            </h2>
            {#each g.offline as host (host)}<p class="offline">{offlineSentence(host)}</p>{/each}
            <ul class="card">
              {#each g.sessions as s (s.id)}
                {@const word = stateWord(s)}
                {@const colour = colourOf(s)}
                <li class="session-row" class:coloured={!!colour} data-status={sessionStatus(s)} style={colour ? `--session-colour:${projectColourHex(colour)}` : undefined}>
                  <a class="open" href={sessionHref(s)} oncontextmenu={event => openMenu(event, s)} onkeydown={event => { if (menuKeyOpens(event)) openMenu(event, s) }}>
                    {#if sessionStatus(s) === 'finished'}<span class="status-tick" aria-hidden="true"></span>{:else}<span class="dot" aria-hidden="true"></span>{/if}
                    <span class="main">
                      <span class="line"><span class="title" title={s.title}>{s.title}</span>{#if s.yolo}<span class="badge yolo">YOLO</span>{/if}{#if s.worktreePath}<span class="badge wt">worktree</span>{/if}{#if colour}<span class="badge colour-name"><span class="swatch" aria-hidden="true"></span>{colourName(colour)}</span>{/if}</span>
                      <span class="meta" title={rowMeta(s)}>{rowMeta(s)}</span>
                    </span>
                    <span class="word" title={word.source}>{word.word}{#if offline.has(s.agentId)}{' '}<span class="lk">· last known</span>{/if}</span>
                    <span class="age"><span class="vh">changed </span>{shortAge(Math.max(0, now - s.updatedAt))}<span class="vh"> ago</span></span>
                  </a>
                  {#if places.get(s.id)}
                    {@const place = places.get(s.id)!}
                    <a class="desk-chip" href={`/desks/${encodeURIComponent(place.desk)}#${encodeURIComponent(place.pane)}`} title={`On the desk ${place.name}`} aria-label={`On the desk ${place.name}`}><span class="desk-icon" aria-hidden="true"></span><span class="desk-name">{place.name}</span></a>
                  {/if}
                  <button type="button" class="more" aria-label={`Actions for ${s.title}`} aria-haspopup="menu" aria-expanded={openActionsFor === s.id} onclick={event => openMenu(event, s)} oncontextmenu={event => openMenu(event, s)} onkeydown={event => { if (menuKeyOpens(event)) openMenu(event, s) }}>
                    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="12" cy="5" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="12" cy="19" r="1.6" /></svg>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {/if}
  {/if}

  <!-- On a phone or tablet the rail: the projects with their menus, Select and state words. A card (or New session here) opens
       Home's launch bar on that project; Select hands the chosen projects to it. On a desktop the host shows no rail and holds
       only the dialogs, which the list below opens. -->
  <div class={narrow ? 'projects rail-host' : 'rail-dialogs'}>
    <ProjectRailHost bind:this={projectHost} rail={narrow} variant="sessions" {projects} groups={projectGroups} failed={projectsFailed} {savedWaiting} {sessions} sessionsFailed={failed && sessions === undefined} reporting={reportingNow}
      {running} known={sessions !== undefined} {worktrees} {selecting} {selected} onselecting={setSelecting} ontoggle={toggleSelected}
      onchoose={(p) => location.assign(`/?project=${encodeURIComponent(p.id)}`)} signal={bounded} onsettle={settle} onsettlegroups={settleGroups} onwaiting={(n) => (savedWaiting = n)}
      onmachines={(m) => (terminals = m)} onnotice={(n) => (notice = n)} onreload={() => void load(true)} onsignedout={signOut}>
      {#snippet selectAction()}
        {#if selected.length >= 2}<a class="start-chosen" href={`/?projects=${selected.map(encodeURIComponent).join(',')}`}>Start in these on Home</a>{/if}
      {/snippet}
    </ProjectRailHost>
  </div>
  {#if !narrow}
    <section class="projects" aria-labelledby="sessions-projects">
      <div class="projects-head">
        <h2 id="sessions-projects">{projects ? `Projects · ${projects.length}` : 'Projects'}</h2>
        <button type="button" class="add" onclick={event => void projectHost?.openAdd(event.currentTarget)}>Add a project</button>
      </div>
      {#if projectsFailed}<p class="none">Couldn't read your projects just now.</p>
      {:else if projects !== undefined && projects.length === 0}<p class="none">Add a project to start an agent in one of your folders.</p>
      {:else if projects}
        <ul class="project-list">
          {#each projects as p (p.id)}
            <li><a class="project-card" href={`/?project=${encodeURIComponent(p.id)}`} aria-label={projectLabel(p)} style={`--project-colour:${projectColourHex(p.colour)}`}
              oncontextmenu={event => void openProjectMenu(event, p)} onkeydown={event => { if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) void openProjectMenu(event, p) }}>
              <span class="stripe" aria-hidden="true"></span><span class="pname" title={p.name}>{p.name}</span><span class="pmachine" title={[p.machine || 'machine not enrolled', projectRepo(p) ? `github.com/${projectRepo(p)}` : ''].filter(Boolean).join(' · ')}>{p.machine || 'machine not enrolled'}{#if projectRepo(p)} · {projectRepo(p)}{/if}</span>
            </a></li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

<svelte:window onpointerdown={event => { if (projectMenu && !projectMenuEl?.contains(event.target as Node)) closeProjectMenu(false) }} />

{#if projectMenu}
  {@const item = projectMenu.item}
  <div class="menu" role="menu" aria-label={`Actions for ${item.name}`} tabindex="-1" bind:this={projectMenuEl} style={`left:${projectMenu.x}px;top:${projectMenu.y}px`} onkeydown={projectMenuKey}>
    <p>{item.name}<small>{item.machine || 'machine not enrolled'}</small></p>
    <a role="menuitem" href={`/?project=${encodeURIComponent(item.id)}`}>Open on Home</a>
    <a role="menuitem" href={`/?project=${encodeURIComponent(item.id)}#launch`}>New session here</a>
    <button role="menuitem" onclick={openWorktrees}>Worktrees…</button>
  </div>
{/if}
<SessionActions bind:this={actions} machines={machines} {offline} {terminals} {removeDefault} {factorySessionEndDefault}
  onsignedout={signOut} onchanged={() => load(true)} ondesks={() => void loadPlaces()} onnotice={(n) => (notice = n)}
  onopen={(item) => (openActionsFor = item.id)} onclose={() => (openActionsFor = '')} />
{#if commandsOpen}<CommandLibrary onclose={() => void closeCommands()} />{/if}
{#if worktreesFor}
  <ProjectWorktrees project={worktreesFor.project} {sessions} sessionsFailed={failed && sessions === undefined} opener={worktreesFor.opener} onsignedout={signOut} onclose={() => (worktreesFor = null)} />
{/if}

<style>
  .sessions-page { max-width: 960px; margin: 0 auto; display: flex; flex-direction: column; gap: 14px; min-width: 0; }
  .page-head { display: flex; align-items: center; gap: 12px; min-height: 56px; }
  .back { display: none; width: 44px; height: 44px; margin-left: -10px; align-items: center; justify-content: center; color: var(--ink-soft); }
  .back svg { width: 22px; height: 22px; }
  h1 { margin: 0; flex-grow: 1; min-width: 0; font: 600 28px/1.2 var(--serif); }
  .new { display: inline-flex; align-items: center; min-height: 44px; padding: 0 14px; border-radius: 10px; background: var(--accent); color: var(--accent-ink); font-weight: 600; text-decoration: none; }
  .new:hover { filter: brightness(1.08); }
  .commands { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 12px; border: 1px solid var(--line-strong); border-radius: 10px; background: none; color: var(--ink); font: 500 14px var(--sans); cursor: pointer; }
  .commands:hover { background: var(--tint); }
  .commands svg { width: 16px; height: 16px; }
  .commands .narrow { display: none; }
  .tools { display: flex; flex-direction: column; gap: 10px; }
  .search { display: flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--muted); }
  .search:focus-within { border-color: var(--accent); }
  .search svg { width: 16px; height: 16px; flex: none; }
  .search input { flex: 1; min-width: 0; min-height: 42px; padding: 0; border: 0; outline: 0; background: none; color: var(--ink); font: 400 16px var(--sans); box-shadow: none; }
  .group-by { display: flex; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; font-size: 14px; }
  .group-by label { position: relative; flex: 1; display: flex; align-items: center; justify-content: center; min-height: 44px; color: var(--muted); cursor: pointer; }
  .group-by label.on { background: var(--line-soft); color: var(--ink); font-weight: 600; }
  .group-by label:has(:focus-visible) { outline: 2px solid var(--accent); outline-offset: -2px; }
  .group-by input { position: absolute; inset: 0; opacity: 0; margin: 0; cursor: pointer; }
  .filters { border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .filters summary { display: flex; align-items: center; min-height: 44px; padding: 0 12px; font-size: 14px; color: var(--ink-soft); cursor: pointer; }
  .on-count { color: var(--accent-text); font-weight: 600; font-variant-numeric: tabular-nums; white-space: pre; }
  .filter-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; padding: 4px 12px 12px; align-items: end; }
  .filter-grid label { display: grid; gap: 4px; font-size: 14px; color: var(--muted); min-width: 0; }
  .filter-grid select { width: 100%; min-width: 0; min-height: 44px; font-size: 16px; }
  .filter-grid .check { display: flex; align-items: center; gap: 8px; min-height: 44px; font-size: 14px; color: var(--ink); cursor: pointer; }
  .filter-grid .check input { width: 18px; height: 18px; margin: 0; accent-color: var(--accent); }
  .clear { min-height: 44px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: none; color: var(--ink); font: inherit; font-size: 14px; cursor: pointer; }
  .clear:disabled { opacity: .45; cursor: default; }
  .live p, .problem { margin: 0; font-size: 14px; color: var(--muted); }
  .status-line { display: flex; align-items: baseline; gap: 12px; min-height: 18px; margin: -6px 0; }
  .status-line .live { flex: 1; min-width: 0; }
  .checking { margin: 0 0 0 auto; flex: none; font-size: 14px; line-height: 18px; color: var(--muted); }
  .notice.bad, .problem { color: var(--bad); }
  .again { min-height: 44px; padding: 0 6px; border: 0; background: none; color: inherit; font: inherit; text-decoration: underline; cursor: pointer; }
  .none { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--muted); }
  .first-machine { padding: 24px 16px; border: 1px dashed var(--line); border-radius: 12px; background: var(--card); text-align: center; }
  .first-machine h2 { margin: 0 0 8px; font: 600 21px/1.3 var(--serif); }
  .first-machine p { margin: 0 0 16px; color: var(--muted); }
  .groups { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
  .group, .projects, .tools { min-width: 0; }
  .group h2 { display: flex; align-items: center; gap: 8px; margin: 0; padding: 0 2px 6px; font: 600 15px/1.3 var(--sans); }
  .group h2 .name { min-width: 0; overflow-wrap: anywhere; }
  .group h2 .count { color: var(--muted); font-weight: 500; font-size: 14px; font-variant-numeric: tabular-nums; flex-grow: 1; }
  .mark { width: 14px; height: 14px; flex: none; fill: var(--repo); }
  .machine-pill { flex: 0 1 auto; min-width: 0; max-width: 45%; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; padding: 1px 8px; border: 1px solid var(--line-strong); border-radius: 6px; font-size: 14px; font-weight: 400; color: var(--muted); }
  .offline { margin: 0 2px 6px; font-size: 14px; color: var(--warn); }
  .card { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 12px; background: var(--card); overflow: hidden; }
  .session-row { display: flex; align-items: stretch; border-top: 1px solid var(--line-soft); }
  .session-row:first-child { border-top: 0; }
  /* A colour from the sessions panel (or the project's) is identity, never state: a strip on the row's edge, and its name. */
  .session-row { border-left: 3px solid transparent; }
  .session-row.coloured { border-left-color: var(--session-colour); }
  .colour-name { display: inline-flex; align-items: center; gap: 4px; }
  .swatch { width: 7px; height: 7px; border-radius: 2px; background: var(--session-colour); }
  .session-row:hover { background: var(--card-raised); }
  .open { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; min-height: 52px; padding: 6px 4px 6px 14px; color: var(--ink); text-decoration: none; }
  .open:focus-visible, .more:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--status); }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .line { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .title { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 15px; }
  .badge { flex: none; padding: 0 6px; border: 1px solid var(--line-strong); border-radius: 6px; font-size: 14px; line-height: 17px; color: var(--muted); }
  .badge.yolo { border-color: var(--warn); color: var(--warn); font-weight: 600; }
  .meta { font-size: 14px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .word { flex: none; max-width: 34%; font-size: 14px; font-weight: 600; color: var(--status); border: 1px solid var(--status-edge); background: var(--status-wash); border-radius: 6px; padding: 1px 8px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .session-row[data-status='waiting'], .session-row[data-status='working'], .session-row[data-status='starting'], .session-row[data-status='failed'] { background: color-mix(in srgb, var(--status) 5%, transparent); box-shadow: inset 3px 0 var(--status); }
  .session-row[data-status='waiting'] .title, .session-row[data-status='working'] .title, .session-row[data-status='failed'] .title { color: var(--status); }
  .age { flex: none; min-width: 28px; text-align: right; font-size: 14px; font-variant-numeric: tabular-nums; color: var(--muted); }
  /* The desk a session is on: a chip after the row's words, opening the desk at that pane. */
  .desk-chip { flex: none; align-self: center; display: inline-flex; align-items: center; gap: 6px; max-width: 160px; min-height: 30px; padding: 0 10px; margin-left: 4px; border: 1px solid var(--line-strong); border-radius: 999px; color: var(--ink-soft); font-size: 14px; text-decoration: none; }
  .desk-chip:hover { background: var(--tint); color: var(--ink); }
  .desk-chip:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .desk-icon { width: 10px; height: 8px; flex: none; border: 1.5px solid currentColor; border-radius: 2px; background: linear-gradient(currentColor, currentColor) 50% 0 / 1.5px 100% no-repeat; }
  .desk-name { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  @media (pointer: coarse) { .desk-chip { min-height: 44px; } }
  .more { flex: none; width: 44px; display: grid; place-items: center; padding: 0; border: 0; background: none; color: var(--muted); cursor: pointer; }
  .more:hover { color: var(--accent-text); background: var(--tint); }
  .more svg { width: 18px; height: 18px; }
  .rail-dialogs { display: contents; }
  .projects { display: flex; flex-direction: column; gap: 10px; padding-top: 4px; }
  .projects-head { display: flex; align-items: center; gap: 10px; }
  .start-chosen { display: inline-flex; align-items: center; justify-content: center; min-height: 48px; padding: 0 16px; border-radius: 8px; background: var(--accent); color: var(--accent-ink); font-weight: 600; text-decoration: none; }
  .projects-head h2 { flex-grow: 1; margin: 0; font: 600 14px var(--sans); letter-spacing: 0.01em; color: var(--muted); }
  .add { min-height: 44px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 8px; background: none; color: var(--ink); font: 500 15px var(--sans); cursor: pointer; }
  .add:hover { background: var(--tint); }
  .project-list { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr)); gap: 8px; }
  .project-card { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--ink); text-decoration: none; min-width: 0; }
  .project-card:hover { background: var(--card-raised); }
  .stripe { width: 4px; align-self: stretch; border-radius: 2px; background: var(--project-colour, transparent); border: 1px solid var(--line-soft); flex: none; }
  .pname { flex: 1; min-width: 0; font-weight: 600; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .pmachine { flex: 0 1 auto; min-width: 0; max-width: 50%; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 14px; color: var(--muted); }
  .menu { position: fixed; z-index: 110; width: 240px; max-height: calc(100dvh - 16px); overflow: auto; display: grid; gap: 2px; padding: 6px; border: 1px solid var(--line); border-radius: 8px; background: var(--card-raised); box-shadow: var(--shadow); }
  .menu p { margin: 5px 8px 8px; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
  .menu small { display: block; margin-top: 3px; font-size: 14px; font-weight: 400; color: var(--muted); }
  .menu [role=menuitem] { display: block; min-height: 36px; padding: 8px 9px; border: 0; border-radius: 6px; background: none; color: var(--ink); font: 400 14px var(--sans); text-align: left; text-decoration: none; cursor: pointer; }
  .menu [role=menuitem]:hover, .menu [role=menuitem]:focus-visible { background: var(--tint); outline: none; }
  .menu [role=menuitem]:disabled { opacity: .45; cursor: default; }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @media (pointer: coarse) { .menu [role=menuitem] { min-height: 44px; } }
  /* Tablet and phone: the mock-up's phone Sessions screen (the tablet uses it at its own width). */
  @media (max-width: 1099px) {
    .back { display: flex; }
    h1 { font-size: 21px; }
    .filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .sessions-page { gap: 12px; }
    .filter-grid { grid-template-columns: minmax(0, 1fr); }
    /* A long word ("Working in the background") wraps rather than losing its end on a phone. */
    .word { max-width: 45%; font-size: 14px; line-height: 1.3; white-space: normal; text-align: center; }
    /* The mock-up's phone row: dot, title, state word and age. The agent line and the worktree badge are left out to keep the
       title readable (YOLO stays); "last known" is still read out, and the group says the machine is offline. */
    .meta, .badge.wt { display: none; }
    .desk-chip { max-width: 96px; padding: 0 8px; }
    .lk { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    /* The colour's name is still read out; the strip shows it. */
    .colour-name { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .commands .wide { display: none; }
    .commands .narrow { display: inline; }
    .commands { padding: 0 10px; }
  }
  /* The narrowest phones: Saved commands keeps its icon (and its name for assistive tech) so the header fits. */
  @media (max-width: 374px) {
    .commands { width: 44px; padding: 0; justify-content: center; }
    .commands .narrow { display: none; }
  }
</style>
