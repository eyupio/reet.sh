<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { getProfile, UnauthorizedError } from '../api'
  import { copyText } from '../clipboard'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import CommandLibrary from '../terminal/CommandLibrary.svelte'
  import AddProjectDialog from '../home/AddProjectDialog.svelte'
  import ProjectRailHost from '../home/ProjectRailHost.svelte'
  import { addedLine } from '../folderBrowser'
  import { projectWorktrees, railState, shortAge, stateWord, type GroupBy } from '../home'
  import { PROJECT_COLOURS, projectColourHex } from '../projectColour'
  import type { RunMember } from '../projects'
  import { agentChoicesOf, sessionColourOf, filterSessions, filtersOn, offlineSentence, rowMeta, searchSessions, sessionGroups, worktreeTargetOf, type MachineName, type SessionFilter, type SessionSort } from '../sessions'
  import { archiveTerminal, endTerminal, folderProjectList, machineHealth, renameTerminal, sessionHref, terminalClosed, terminalMachines, terminalSessions, TerminalError, validTerminalTitle,
    type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'

  // The Sessions page (/sessions): every session across machines from the account-wide listing, grouped as Home groups them
  // (the Group by choice is Home's), with search, filters and each session's actions menu; the projects come below with
  // Add a project. Read again every 15 seconds while in view. A read that fails keeps the last list and says so. Only the
  // Group by choice is stored (Home's key); the search and filters live on this page only. On a tablet or phone the projects
  // are the full project rail (the phone rail, moved here from Home): its menus, Select and the state words.
  let { userId, onsignedout }: { userId: number; onsignedout: (e: unknown) => void } = $props()

  const POLL_MS = 15_000
  // The Machines page opens Add a machine when it arrives with this fragment.
  const machinesAt = (fragment: string) => `/machines#${fragment}`
  const GROUPS: GroupBy[] = ['project', 'machine', 'state']
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
  let projects = $state<FolderProject[] | undefined>()
  let terminals = $state<TerminalMachine[] | undefined>()
  let health = $state<MachineHealth[] | undefined>()
  let now = $state(Date.now())
  let checking = $state(false), failed = $state(false), projectsFailed = $state(false), denied = $state(false)
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
  const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host || agentId
  const colourOf = (s: TerminalSession) => sessionColourOf(s, colors, projects ?? [])
  const colourName = (id: string) => PROJECT_COLOURS.find(c => c.id === id)?.name ?? ''
  const shown = $derived(filterSessions(searchSessions(sessions ?? [], projects ?? [], machines, query), filter, new Set(), colourOf))
  const groups = $derived(sessionGroups(shown, projects ?? [], machines, groupBy, offline, sort, order))
  const agents = $derived(agentChoicesOf(sessions ?? []))
  const on = $derived(filtersOn(filter))
  const running = $derived.by(() => { const m = new Map<string, number>(); for (const s of sessions ?? []) if (s.projectId && (s.state === 'running' || s.state === 'creating')) m.set(s.projectId, (m.get(s.projectId) ?? 0) + 1); return m })
  const worktrees = $derived(projectWorktrees(sessions ?? []))
  // Whether each machine reports now (the rail's state words); a machine without a report is not called offline.
  const reportingNow = $derived(new Map((health ?? []).map(m => [m.agentId, m.state === 'connected'])))
  // A write's answer from the rail is the list as it now is: it supersedes a read on its way.
  function settle(next: FolderProject[]) { generation++; loading = false; checking = false; projects = next }

  let loading = false, generation = 0
  async function load(explicit = false) {
    if (life.signal.aborted || (loading && !explicit)) return
    const mine = ++generation
    loading = true; checking = true
    try {
      const [s, p, t, h] = await Promise.allSettled([terminalSessions(bounded()), folderProjectList(bounded()), terminalMachines(bounded()), machineHealth(bounded())])
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
      // A health read that fails keeps the last one: the offline sentences stay as last known.
      if (h.status === 'fulfilled') health = h.value.machines ?? []
      failed = bad
      now = Date.now()
    } finally {
      if (mine === generation) { loading = false; checking = false }
    }
  }

  // The actions menu: right-click on a row, Shift+F10 or the menu key on it, or its ⋮ button. Arrow keys move, Escape closes
  // and gives focus back to what opened it.
  let menu = $state<HTMLElement>()
  let context = $state<{ item: TerminalSession; x: number; y: number } | null>(null)
  let returnFocus = $state<HTMLElement>()
  async function openMenu(event: MouseEvent | KeyboardEvent, item: TerminalSession) {
    // Shift+right-click keeps the browser's own menu, as on the terminal.
    if (event.type === 'contextmenu' && event.shiftKey) return
    event.preventDefault()
    const opener = event.currentTarget as HTMLElement
    // Focus goes back to what opened the menu: the row's link or its ⋮ button.
    returnFocus = opener
    const box = opener.getBoundingClientRect()
    const x = 'clientX' in event && event.clientX ? event.clientX : box.left, y = 'clientY' in event && event.clientY ? event.clientY : box.bottom
    context = { item, x: Math.min(Math.max(8, x), Math.max(8, innerWidth - 248)), y: Math.max(8, Math.min(y, innerHeight - 290)) }
    await tick()
    menu?.querySelector<HTMLElement>('[role=menuitem]:not(:disabled):not([aria-disabled=true])')?.focus()
  }
  function closeMenu(focus = true) { context = null; if (focus) returnFocus?.focus() }
  function menuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); closeMenu(); return }
    if (event.key === 'Tab') { closeMenu(false); return }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = [...menu!.querySelectorAll<HTMLElement>('[role=menuitem]:not(:disabled):not([aria-disabled=true])')], at = items.indexOf(document.activeElement as HTMLElement)
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
  }
  const menuKeyOpens = (event: KeyboardEvent) => event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')
  const live = (s: TerminalSession) => !terminalClosed(s.state)
  const serverOff = (s: TerminalSession) => terminals?.find(m => m.agentId === s.agentId)?.serverEnabled === false

  // Rename, End (with the remove-from-history choice) and Remove from history, each confirmed; then the list is read again.
  let action = $state<{ kind: 'rename' | 'end' | 'archive'; item: TerminalSession } | null>(null)
  let rename = $state(''), removeOnEnd = $state(false), removeDefault = false, busy = $state(false)
  const endKeys = new Map<string, { key: string; remove: boolean }>()
  function begin(kind: 'rename' | 'end' | 'archive') {
    if (!context) return
    const item = context.item
    closeMenu(false)
    rename = item.title
    removeOnEnd = endKeys.get(item.id)?.remove ?? (item.state === 'end_requested' ? item.removeHistory ?? false : removeDefault)
    action = { kind, item }
  }
  function cancel() { action = null; returnFocus?.focus() }
  async function perform() {
    if (!action || busy) return
    const { kind, item } = action
    if (kind === 'rename' && !validTerminalTitle(rename.trim())) { notice = { text: 'Use a session name of 1–80 characters without control characters.', bad: true }; return }
    action = null; busy = true; notice = null
    try {
      if (kind === 'rename') await renameTerminal(item.id, rename.trim())
      else if (kind === 'archive') { await archiveTerminal(item.id); sessions = (sessions ?? []).filter(s => s.id !== item.id) }
      else {
        let op = endKeys.get(item.id)
        if (!op) { op = { key: crypto.randomUUID(), remove: removeOnEnd }; endKeys.set(item.id, op) }
        await endTerminal(item.id, op.key, op.remove)
      }
      await load(true)
    } catch (e) {
      if (refused(e)) return signOut()
      notice = { text: e instanceof Error ? e.message : 'The action is unconfirmed. Retry checks the same session.', bad: true }
    } finally { busy = false }
  }
  async function copyBranch() {
    if (!context?.item.worktreeBranch) return
    const branch = context.item.worktreeBranch
    closeMenu()
    notice = (await copyText(branch)) ? { text: `Copied ${branch}.` } : { text: "Couldn't copy the branch. Select it in the session's details instead.", bad: true }
  }
  // Check and remove worktree: the existing sheet, which reads the worktree's state first and removes nothing unsaved.
  let removing = $state<{ member: RunMember; host: string } | null>(null)
  function checkWorktree() {
    const item = context?.item, target = item ? worktreeTargetOf(item) : null
    if (!item || !target) return
    closeMenu(false)
    removing = { host: hostOf(item.agentId), member: { position: 0, host: item.agentId, dir: '', tool: item.tool ?? '', model: item.model ?? '', state: 'started', code: '', sentence: '', sessionId: item.id,
      worktreePath: item.worktreePath ?? '', worktreeRepo: target.repo, worktreeBranch: item.worktreeBranch ?? '', worktreeRun: target.run, worktreeProject: target.project, sharesCheckout: false, attempt: 0 } }
  }

  // Add a project: the machines are read again first, as on Home.
  let adding = $state<{ opener: HTMLElement } | null>(null), addBusy = $state(false)
  // Saved commands (owner ruling): the account's command library, without opening a session. Insert needs a live pane,
  // so it is not offered here; the pane's own entry stays.
  let commandsOpen = $state(false), commandsButton = $state<HTMLButtonElement>()
  async function closeCommands() { commandsOpen = false; await tick(); commandsButton?.focus() }
  async function openAdd(opener: HTMLElement) {
    if (addBusy) return
    addBusy = true; notice = null
    try { terminals = await terminalMachines(bounded()); adding = { opener } }
    catch (e) { if (refused(e)) return signOut(); notice = { text: "Couldn't read your machines just now. Try again in a moment.", bad: true } }
    finally { addBusy = false }
  }
  function added(p: FolderProject) { notice = { text: addedLine(p.name) }; void load(true) }

  const projectLabel = (p: FolderProject) => [p.name, p.machine || 'machine not enrolled', railState(sessions ? running.get(p.id) ?? 0 : null, health?.some(m => m.agentId === p.host) ? !offline.has(p.host) : undefined)?.text].filter(Boolean).join(', ')
  const clearFilters = () => { filter = { state: '', agent: '', machine: '', yoloOnly: false, worktreeOnly: false, colour: '' } }

  onMount(() => {
    void load()
    void getProfile().then(p => { removeDefault = p.profile.terminalRemoveOnClose ?? false }).catch(() => { /* the safe default keeps history */ })
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const wake = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', wake)
    const query = matchMedia(NARROW), resized = () => { narrow = query.matches }
    query.addEventListener('change', resized)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', wake); query.removeEventListener('change', resized); life.abort() }
  })
</script>

<svelte:window onpointerdown={event => { if (context && !menu?.contains(event.target as Node)) closeMenu(false) }} onblur={() => closeMenu(false)} />

<div class="sessions-page">
  <header class="page-head">
    <a class="back" href="/" aria-label="Back to Home"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg></a>
    <h1>Sessions</h1>
    <button type="button" class="commands" aria-label="Saved commands" bind:this={commandsButton} onclick={() => (commandsOpen = true)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 7l4 5-4 5M12 17h7" /></svg><span class="wide">Saved commands</span><span class="narrow">Commands</span></button>
    <a class="new" href="/" aria-label="New session">New</a>
  </header>

  <div class="tools">
    <label class="search">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
      <input type="search" bind:value={query} placeholder="Search sessions" aria-label="Search sessions" autocomplete="off" />
    </label>
    <div class="group-by" role="radiogroup" aria-label="Group by">
      {#each [['project', 'Project'], ['machine', 'Machine'], ['state', 'State']] as [value, label] (value)}
        <label class:on={groupBy === value}><input type="radio" name="sessions-group" {value} checked={groupBy === value} onchange={() => chooseGroup(value as GroupBy)} />{label}</label>
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

  <div class="live" role="status" aria-live="polite">
    {#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}</p>{/if}
  </div>
  <!-- Not a live region: a read every 15 seconds would be announced each time. -->
  {#if checking}<p class="quiet">Checking…</p>{/if}
  {#if failed}<p class="problem" role="alert"><span>{sessions === undefined ? "Couldn't read your sessions just now." : "Couldn't refresh. Showing what we last saw."}</span> <button type="button" class="again" onclick={() => void load(true)}>Try again</button></p>{/if}

  {#if denied}
    <p class="none" role="alert">Your account no longer has terminal access. Sign in again or ask the owner to check your permissions. Existing sessions were not ended.</p>
  {:else if terminals !== undefined && terminals.length === 0 && (sessions ?? []).length === 0}
    <div class="first-machine"><h2>Connect your first machine</h2><p>Enroll a machine and enable terminal access locally to start sessions here.</p><a class="btn primary" href={machinesAt('add-machine')}>Connect a machine</a></div>
  {:else if sessions !== undefined}
    {#if sessions.length === 0}
      <p class="none">No sessions yet. Start one from Home.</p>
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
                <li class="session-row" class:coloured={!!colour} style={colour ? `--session-colour:${projectColourHex(colour)}` : undefined}>
                  <a class="open" href={sessionHref(s)} oncontextmenu={event => void openMenu(event, s)} onkeydown={event => { if (menuKeyOpens(event)) void openMenu(event, s) }}>
                    <span class="dot {word.tone}" aria-hidden="true"></span>
                    <span class="main">
                      <span class="line"><span class="title" title={s.title}>{s.title}</span>{#if s.yolo}<span class="badge yolo">YOLO</span>{/if}{#if s.worktreePath}<span class="badge wt">worktree</span>{/if}{#if colour}<span class="badge colour-name"><span class="swatch" aria-hidden="true"></span>{colourName(colour)}</span>{/if}</span>
                      <span class="meta" title={rowMeta(s)}>{rowMeta(s)}</span>
                    </span>
                    <span class="word {word.tone}" title={word.source}>{word.word}{#if offline.has(s.agentId)}{' '}<span class="lk">· last known</span>{/if}</span>
                    <span class="age"><span class="vh">changed </span>{shortAge(Math.max(0, now - s.updatedAt))}<span class="vh"> ago</span></span>
                  </a>
                  <button type="button" class="more" aria-label={`Actions for ${s.title}`} aria-haspopup="menu" aria-expanded={context?.item.id === s.id} onclick={event => void openMenu(event, s)} oncontextmenu={event => void openMenu(event, s)} onkeydown={event => { if (menuKeyOpens(event)) void openMenu(event, s) }}>
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

  {#if narrow}
    <!-- The phone rail: the projects with their menus, Select and state words. A card (or New session here) opens Home's launch
         bar on that project; Select hands the chosen projects to it. -->
    <div class="projects rail-host">
      <ProjectRailHost variant="sessions" {projects} failed={projectsFailed} {savedWaiting} {sessions} sessionsFailed={failed && sessions === undefined} reporting={reportingNow}
        {running} known={sessions !== undefined} {worktrees} {selecting} {selected} onselecting={setSelecting} ontoggle={toggleSelected}
        onchoose={(p) => location.assign(`/?project=${encodeURIComponent(p.id)}`)} signal={bounded} onsettle={settle} onwaiting={(n) => (savedWaiting = n)}
        onmachines={(m) => (terminals = m)} onnotice={(n) => (notice = n)} onreload={() => void load(true)} onsignedout={signOut}>
        {#snippet selectAction()}
          {#if selected.length >= 2}<a class="start-chosen" href={`/?projects=${selected.map(encodeURIComponent).join(',')}`}>Start in these on Home</a>{/if}
        {/snippet}
      </ProjectRailHost>
    </div>
  {:else}
    <section class="projects" aria-labelledby="sessions-projects">
      <div class="projects-head">
        <h2 id="sessions-projects">Projects · {projects?.length ?? 0}</h2>
        <button type="button" class="add" disabled={addBusy} onclick={event => void openAdd(event.currentTarget)}>Add a project</button>
      </div>
      {#if projectsFailed}<p class="none">Couldn't read your projects just now.</p>
      {:else if projects !== undefined && projects.length === 0}<p class="none">Add a project to start an agent in one of your folders.</p>
      {:else if projects}
        <ul class="project-list">
          {#each projects as p (p.id)}
            <li><a class="project-card" href={`/?project=${encodeURIComponent(p.id)}`} aria-label={projectLabel(p)} style={`--project-colour:${projectColourHex(p.colour)}`}>
              <span class="stripe" aria-hidden="true"></span><span class="pname" title={p.name}>{p.name}</span><span class="pmachine" title={p.machine || 'machine not enrolled'}>{p.machine || 'machine not enrolled'}</span>
            </a></li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

{#if context}
  {@const item = context.item}
  {@const away = offline.has(item.agentId)}
  {@const target = worktreeTargetOf(item)}
  <div class="menu" role="menu" aria-label={`Actions for ${item.title}`} tabindex="-1" bind:this={menu} style={`left:${context.x}px;top:${context.y}px`} onkeydown={menuKey}>
    <p>{item.title}<small>{hostOf(item.agentId)}</small></p>
    <button role="menuitem" disabled={busy || serverOff(item)} onclick={() => begin('rename')}>Rename…</button>
    {#if away}<a role="menuitem" aria-disabled="true" title={offlineSentence(hostOf(item.agentId))}>Open in theatre</a>
    {:else}<a role="menuitem" href={`${sessionHref(item)}&theatre=1`}>Open in theatre</a>{/if}
    {#if item.worktreeBranch}<button role="menuitem" onclick={() => void copyBranch()}>Copy branch</button>{/if}
    {#if live(item)}<button role="menuitem" class="danger" disabled={busy || serverOff(item)} onclick={() => begin('end')}>{item.state === 'end_requested' ? 'Retry end request…' : 'End session…'}</button>
    {:else}<button role="menuitem" disabled={busy} onclick={() => begin('archive')}>Remove from history…</button>{/if}
    {#if target}<button role="menuitem" disabled={live(item) || away} title={live(item) ? 'End the session first: it is working in this worktree.' : away ? offlineSentence(hostOf(item.agentId)) : undefined} onclick={checkWorktree}>Check and remove worktree…</button>{/if}
  </div>
{/if}

{#if action?.kind === 'rename'}
  <ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={() => void perform()} oncancel={cancel}><label class="field">Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>
{:else if action?.kind === 'end'}
  <ConfirmDialog title={`End ${action.item.title}?`} body="This stops the shell and its running jobs. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={() => void perform()} oncancel={cancel}>
    <label class="option"><input type="checkbox" bind:checked={removeOnEnd} disabled={endKeys.has(action.item.id) || action.item.state === 'end_requested'} /><span>Remove from history when this session ends</span></label>
  </ConfirmDialog>
{:else if action?.kind === 'archive'}
  <ConfirmDialog title={`Remove ${action.item.title} from history?`} body="This removes the closed session from your history. Files on the machine are kept. You cannot restore it from the web interface." confirmLabel="Remove from history" onconfirm={() => void perform()} oncancel={cancel} />
{/if}
{#if removing}
  <WorktreeDialog member={removing.member} host={removing.host} opener={returnFocus ?? null} onremoved={() => void load(true)} onsignedout={signOut} onclose={() => (removing = null)} />
{/if}
{#if commandsOpen}<CommandLibrary onclose={() => void closeCommands()} />{/if}
{#if adding && terminals}
  <AddProjectDialog machines={terminals} opener={adding.opener} onadded={added} onsignedout={signOut} onclose={() => (adding = null)} />
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
  .filters { border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .filters summary { display: flex; align-items: center; min-height: 44px; padding: 0 12px; font-size: 14px; color: var(--ink-soft); cursor: pointer; }
  .on-count { color: var(--accent-text); font-weight: 600; white-space: pre; }
  .filter-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; padding: 4px 12px 12px; align-items: end; }
  .filter-grid label { display: grid; gap: 4px; font-size: 12px; color: var(--muted); min-width: 0; }
  .filter-grid select { width: 100%; min-width: 0; min-height: 44px; font-size: 16px; }
  .filter-grid .check { display: flex; align-items: center; gap: 8px; min-height: 44px; font-size: 14px; color: var(--ink); cursor: pointer; }
  .filter-grid .check input { width: 18px; height: 18px; margin: 0; accent-color: var(--accent); }
  .clear { min-height: 44px; padding: 0 12px; border: 1px solid var(--line); border-radius: 10px; background: none; color: var(--ink); font: inherit; font-size: 14px; cursor: pointer; }
  .clear:disabled { opacity: .45; cursor: default; }
  .live p, .problem { margin: 0; font-size: 13px; color: var(--muted); }
  .live:empty { display: none; }
  .notice.bad, .problem { color: var(--bad); }
  .again { min-height: 44px; padding: 0 6px; border: 0; background: none; color: inherit; font: inherit; text-decoration: underline; cursor: pointer; }
  .none { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--muted); }
  .first-machine { padding: 24px 16px; border: 1px dashed var(--line); border-radius: 13px; background: var(--card); text-align: center; }
  .first-machine h2 { margin: 0 0 8px; font: 600 21px/1.3 var(--serif); }
  .first-machine p { margin: 0 0 16px; color: var(--muted); }
  .groups { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
  .group, .projects, .tools { min-width: 0; }
  .group h2 { display: flex; align-items: center; gap: 8px; margin: 0; padding: 0 2px 6px; font: 600 15px/1.3 var(--sans); }
  .group h2 .name { min-width: 0; overflow-wrap: anywhere; }
  .group h2 .count { color: var(--muted); font-weight: 500; font-size: 13px; flex-grow: 1; }
  .mark { width: 14px; height: 14px; flex: none; fill: var(--repo); }
  .machine-pill { flex: 0 1 auto; min-width: 0; max-width: 45%; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; padding: 1px 8px; border: 1px solid var(--line-strong); border-radius: 99px; font-size: 12px; font-weight: 400; color: var(--muted); }
  .offline { margin: 0 2px 6px; font-size: 13px; color: var(--warn); }
  .card { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 13px; background: var(--card); overflow: hidden; }
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
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--muted); }
  .dot.good { background: var(--good); }
  .dot.warn { background: var(--warn); }
  .dot.bad { background: var(--bad); }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .line { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .title { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 15px; }
  .badge { flex: none; padding: 0 6px; border: 1px solid var(--line-strong); border-radius: 99px; font-size: 11px; line-height: 17px; color: var(--muted); }
  .badge.yolo { border-color: var(--warn); color: var(--warn); font-weight: 600; }
  .meta { font-size: 12px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .word { flex: none; max-width: 34%; font-size: 13px; font-weight: 500; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .word.good { color: var(--good); }
  .word.warn { color: var(--warn); }
  .word.bad { color: var(--bad); }
  .age { flex: none; min-width: 28px; text-align: right; font-size: 12px; color: var(--muted); }
  .more { flex: none; width: 44px; display: grid; place-items: center; padding: 0; border: 0; background: none; color: var(--muted); cursor: pointer; }
  .more:hover { color: var(--accent-text); background: var(--tint); }
  .more svg { width: 18px; height: 18px; }
  .projects { display: flex; flex-direction: column; gap: 10px; padding-top: 4px; }
  .projects-head { display: flex; align-items: center; gap: 10px; }
  .start-chosen { display: inline-flex; align-items: center; justify-content: center; min-height: 48px; padding: 0 16px; border-radius: 10px; background: var(--accent); color: var(--accent-ink); font-weight: 600; text-decoration: none; }
  .projects-head h2 { flex-grow: 1; margin: 0; font: 600 11px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--muted); }
  .add { min-height: 44px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 10px; background: none; color: var(--ink); font: 500 15px var(--sans); cursor: pointer; }
  .add:hover { background: var(--tint); }
  .project-list { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr)); gap: 8px; }
  .project-card { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 11px; background: var(--card); color: var(--ink); text-decoration: none; min-width: 0; }
  .project-card:hover { background: var(--card-raised); }
  .stripe { width: 4px; align-self: stretch; border-radius: 2px; background: var(--project-colour, transparent); border: 1px solid var(--line-soft); flex: none; }
  .pname { flex: 1; min-width: 0; font-weight: 600; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .pmachine { flex: 0 1 auto; min-width: 0; max-width: 50%; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 12px; color: var(--muted); }
  .menu { position: fixed; z-index: 110; width: 240px; max-height: calc(100dvh - 16px); overflow: auto; display: grid; gap: 2px; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--card-raised); box-shadow: var(--shadow); }
  .menu p { margin: 5px 8px 8px; font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
  .menu small { display: block; margin-top: 3px; font-size: 12px; font-weight: 400; color: var(--muted); }
  .menu [role=menuitem] { display: block; min-height: 36px; padding: 8px 9px; border: 0; border-radius: 6px; background: none; color: var(--ink); font: 400 14px var(--sans); text-align: left; text-decoration: none; cursor: pointer; }
  .menu [role=menuitem]:hover:not(:disabled):not([aria-disabled=true]), .menu [role=menuitem]:focus-visible { background: var(--tint); outline: none; }
  .menu [role=menuitem]:disabled, .menu [role=menuitem][aria-disabled=true] { opacity: .45; cursor: default; }
  .menu .danger { color: var(--bad); }
  .field { display: grid; gap: 6px; margin-top: 12px; font-size: 14px; }
  .option { display: flex; align-items: flex-start; gap: 9px; margin-top: 14px; font-size: 14px; line-height: 1.5; cursor: pointer; }
  .option input { width: 16px; height: 16px; margin: 3px 0 0; accent-color: var(--accent); }
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
    .word { font-size: 12px; }
    /* The mock-up's phone row: dot, title, state word and age. The agent line and the worktree badge are left out to keep the
       title readable (YOLO stays); "last known" is still read out, and the group says the machine is offline. */
    .meta, .badge.wt { display: none; }
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
