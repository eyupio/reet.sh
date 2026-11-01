<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte'
  import type { FolderProject, ProjectGroup } from '../terminal/api'
  import { railState, worktreesInUse } from '../home'
  import { PROJECT_COLOURS, projectColour, projectColourHex, type ProjectColour } from '../projectColour'

  // The folder projects, in the person's order: one folder on one machine each, with a repository diamond or a folder icon,
  // the machine, the base branch when one is set, the state in words and the worktrees in use. Choosing a card puts that
  // project in the launch bar's sentence (and the card is marked while it is there). Each card's menu (its ⋮ button, a right-click or the
  // menu key): New session here (which also sets the bar's project), Edit, Worktrees, Move up, Move down, Remove from reet
  // and the project's colour. Under 1240 px on a desktop the rail is a 56 px strip of the same
  // cards; on a tablet or phone it is on the Sessions screen (ProjectRailHost, variant 'sessions').
  // Select (the heading's menu, or a long press on a card on touch) turns the cards into checkboxes for Start in several
  // projects: 2 to 12 of them; the strip shows how many are chosen.
  let { projects, groups = undefined, failed = false, reporting, running, known = true, worktrees, savedWaiting = 0, chosen = '', busy, importing = false, pending, moving = false,
    selecting = false, selected = [], onselecting, ontoggle, variant = 'home', selectAction, userId = 0, scope = null, onscope,
    collapsed = false, oncollapsed,
    onadd, onchoose, onnew, onedit, onworktrees, onremove, onmove, ondismiss, onimport, oncolour,
    onarrange, ongroupadd, ongroupedit, ongroupremove, ongroupmove, ongroupcolour, ongroupmovebusy = false, onmovetogroup }: {
    projects: FolderProject[] | undefined
    /** The account's groups, in rail order. */
    groups?: ProjectGroup[] | undefined
    /** The projects could not be read and none were read before. */
    failed?: boolean
    /** Whether each machine (by enrollment id) is reporting now; a machine not in it is not called offline. */
    reporting: Map<string, boolean>
    running: Map<string, number>
    /** False while the sessions have not been read: no card then says how many run, "idle" or worktrees in use. */
    known?: boolean
    worktrees: Map<string, number>
    /** Saved Project folders still waiting for room (offer to bring them in). */
    savedWaiting?: number
    /** The project in the launch bar's sentence (or the one an address chose, `/?project=<id>`), marked. */
    chosen?: string
    busy: boolean
    importing?: boolean
    /** Projects with a change on its way (a dismiss or remove): their buttons wait for it. */
    pending: ReadonlySet<string>
    /** A move is on its way: no other move is offered until it lands. */
    moving?: boolean
    /** Select is on: the cards are checkboxes. */
    selecting?: boolean
    /** The projects chosen in Select (ids). */
    selected?: readonly string[]
    /** Select on (with the card a long press started it from) or off. */
    onselecting?: (on: boolean, first?: FolderProject) => void
    ontoggle?: (project: FolderProject) => void
    /** 'sessions': the phone Sessions screen's heading, "Projects · N" with Add a project, as the mock-up draws it. */
    variant?: 'home' | 'sessions'
    /** What Select leads to where there is no launch bar to take the chosen projects (the Sessions screen). */
    selectAction?: Snippet
    onadd: (opener: HTMLElement) => void
    /** Sets the launch bar's project (a card, or New session here where it cannot start at once). */
    onchoose: (project: FolderProject, opener: HTMLElement) => void
    /** Starts a session in this project at once (the card's +, or New session here). Falls back to `onchoose`. */
    onnew?: (project: FolderProject, opener: HTMLElement) => void
    onedit: (project: FolderProject, opener: HTMLElement) => void
    onworktrees: (project: FolderProject, opener: HTMLElement) => void
    onremove: (project: FolderProject, opener: HTMLElement) => void
    onmove: (project: FolderProject, by: -1 | 1) => void
    ondismiss: (project: FolderProject) => void
    onimport: () => void
    oncolour: (project: FolderProject, colour: ProjectColour) => void
    /** Whose rail: the folded groups are remembered per account. */
    userId?: number
    /** The project or group the board is showing. */
    scope?: { kind: 'project' | 'group'; id: string } | null
    /** A card or group chooses what the board shows (instead of the launch bar's project). */
    onscope?: (scope: { kind: 'project' | 'group'; id: string } | null) => void
    /** The rail is folded to its strip. */
    collapsed?: boolean
    oncollapsed?: (collapsed: boolean) => void
    /** A drag's whole rail order, each project with its version and group. */
    onarrange?: (order: { id: string; version: number; groupId: string }[]) => void
    ongroupadd?: (opener: HTMLElement) => void
    ongroupedit?: (group: ProjectGroup, opener: HTMLElement) => void
    ongroupremove?: (group: ProjectGroup, opener: HTMLElement) => void
    ongroupmove?: (group: ProjectGroup, by: -1 | 1) => void
    ongroupcolour?: (group: ProjectGroup, colour: ProjectColour) => void
    ongroupmovebusy?: boolean
    onmovetogroup?: (project: FolderProject, groupId: string) => void
  } = $props()

  const id = $props.id()
  const machineTag = (p: FolderProject) => p.machine || 'machine not enrolled'
  const stateOf = (p: FolderProject) => railState(known ? running.get(p.id) ?? 0 : null, reporting.get(p.host))
  /** Everything the card shows, in words: name, machine, branch, state and worktrees in use. */
  function label(p: FolderProject): string {
    const wt = worktrees.get(p.id) ?? 0
    const colour = PROJECT_COLOURS.find(c => c.id === projectColour(p.colour))?.name
    return [p.name, colour && colour !== 'No colour' ? colour : '', machineTag(p), p.baseBranch, stateOf(p)?.text, wt ? worktreesInUse(wt) : ''].filter(Boolean).join(', ')
  }
  const initial = (name: string) => Array.from(name.trim())[0]?.toUpperCase() ?? '?'
  /** The most projects one start takes (the controller's limit). */
  const MAX_SELECTED = 12
  const isPicked = (p: FolderProject) => selected.includes(p.id)
  const full_ = $derived(selected.length >= MAX_SELECTED)
  const pickedLine = $derived(selected.length === 0 ? 'Choose two or more projects.' : selected.length === 1 ? '1 chosen. Choose at least one more.'
    : selected.length >= MAX_SELECTED ? `${selected.length} chosen, the most for one start.` : `${selected.length} chosen.`)
  function toggle(p: FolderProject) {
    if (!isPicked(p) && full_) return
    ontoggle?.(p)
  }
  // A long press on a card (touch) starts Select with that card chosen; the click that may follow the press is not a toggle.
  const LONG_PRESS_MS = 500
  let press: { id: string; x: number; y: number; timer: ReturnType<typeof setTimeout> } | null = null
  let swallow = '', swallowUntil = 0, lastPointer = ''
  function pressStart(event: PointerEvent, p: FolderProject) {
    lastPointer = event.pointerType
    if (event.pointerType !== 'touch' || selecting) return
    pressEnd()
    press = { id: p.id, x: event.clientX, y: event.clientY, timer: setTimeout(() => longPress(p), LONG_PRESS_MS) }
  }
  function pressMove(event: PointerEvent) {
    if (press && Math.hypot(event.clientX - press.x, event.clientY - press.y) > 10) pressEnd()
  }
  function pressEnd() { if (press) clearTimeout(press.timer); press = null }
  function longPress(p: FolderProject) {
    pressEnd()
    if (selecting) return
    swallow = p.id; swallowUntil = Date.now() + 1000
    closeMenu(); headMenu = false
    onselecting?.(true, p)
  }
  function cardClick(p: FolderProject, el: HTMLElement) {
    if (swallow === p.id && Date.now() < swallowUntil) { swallow = ''; return }
    if (selecting) toggle(p)
    else chooseCard(p, el)
  }
  // On touch the browser's own long-press menu is the long press: it starts Select instead of opening the card menu.
  function cardContextMenu(event: MouseEvent, p: FolderProject) {
    if (lastPointer === 'touch') { event.preventDefault(); if (!selecting && swallow !== p.id) longPress(p); return }
    if (selecting) { event.preventDefault(); return }
    contextMenu(event, p)
  }

  // The heading's menu: Select projects (or Stop selecting).
  let headMenu = $state(false), headButton = $state<HTMLButtonElement>()
  async function openHeadMenu() {
    if (headMenu) { headMenu = false; return }
    closeMenu(); groupMenu = null
    headMenu = true
    const box = headButton!.getBoundingClientRect()
    menuX = Math.min(Math.max(8, box.right - 212), Math.max(8, innerWidth - 222))
    menuY = Math.min(box.bottom + 4, Math.max(8, innerHeight - 120))
    await tick()
    document.querySelector<HTMLElement>(`#${CSS.escape(`${id}-head-menu`)} [role=menuitem]`)?.focus()
  }
  function headKeys(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); headMenu = false; headButton?.focus() }
    else if (event.key === 'Tab') headMenu = false
  }
  // Done leaves Select; the focus goes to the heading's menu button, where Select was turned on.
  function doneSelecting() {
    onselecting?.(false)
    void tick().then(() => headButton?.focus())
  }
  // The strip's Stop selecting: focus goes to the strip's panel button, which stays where the stop button was.
  function stopFromStrip() {
    onselecting?.(false)
    void tick().then(() => opener?.focus())
  }
  function selectFromMenu() {
    headMenu = false
    onselecting?.(!selecting)
    void tick().then(() => (full?.querySelector<HTMLElement>('[role=checkbox]') ?? headButton)?.focus())
  }
  function groupFromMenu() {
    headMenu = false
    if (headButton) ongroupadd?.(headButton)
  }
  const folders = (n: number) => `${n} ${n === 1 ? 'folder' : 'folders'}`

  // From 1100 to 1239 px the rail is a 56 px strip. Open the project rail shows the full rail over the page as a panel, so
  // every action (card menus, the migration notes, the waiting folders) stays reachable; the button is marked when a note
  // or folders are waiting. While it is open, Tab stays inside it; Escape, its Close button, the toggle or a press outside
  // closes it, and focus goes back to the toggle. It starts where the strip starts on screen, so a scrolled page shows no
  // gap above it.
  let panelOpen = $state(false), panelTop = $state(0)
  let opener = $state<HTMLButtonElement>(), full = $state<HTMLDivElement>(), aside = $state<HTMLElement>()
  const place = () => { if (aside) panelTop = Math.max(0, aside.getBoundingClientRect().top) }
  $effect(() => {
    if (!panelOpen) return
    place()
    window.addEventListener('scroll', place, { passive: true })
    window.addEventListener('resize', place)
    return () => { window.removeEventListener('scroll', place); window.removeEventListener('resize', place) }
  })
  const notes = $derived((projects ?? []).filter(p => p.note).length)
  const waiting = $derived([notes ? `${notes} ${notes === 1 ? 'note' : 'notes'}` : '', savedWaiting > 0 ? `${folders(savedWaiting)} waiting` : ''].filter(Boolean).join(', '))
  const toggleName = $derived(panelOpen ? 'Close the project rail' : `Open the project rail${waiting ? `: ${waiting}` : ''}`)
  async function openPanel() {
    if (panelOpen) { closePanel(true); return }
    panelOpen = true
    await tick()
    ;(full?.querySelector<HTMLElement>('button.add:not(:disabled)') ?? full?.querySelector<HTMLElement>('button:not(:disabled)'))?.focus()
  }
  function closePanel(returnFocus: boolean) {
    panelOpen = false
    if (returnFocus) opener?.focus()
  }
  function panelKeys(event: KeyboardEvent) {
    if (!panelOpen) return
    if (event.key === 'Escape' && !menuFor && !headMenu) { event.preventDefault(); closePanel(true); return }
    if (event.key !== 'Tab' || !full) return
    // Focus stays in the panel: the page drawn under it can't take it.
    const stops = [...full.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], input, select, textarea, [tabindex="0"]')].filter(el => el.getClientRects().length > 0)
    if (stops.length === 0) return
    const first = stops[0], last = stops[stops.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
  }

  // Groups and drag ordering. The groups are always shown in their saved
  // order; a project's place inside a group is its saved rail position, so a
  // drag sends the whole flat order with each project's group.
  const byPosition = <T extends { position: number }>(list: readonly T[]) => [...list].sort((a, b) => a.position - b.position)
  const sortedGroups = $derived(byPosition(groups ?? []))
  const inGroup = (gid: string) => (projects ?? []).filter(p => (p.groupId ?? '') === gid)
  const ungrouped = $derived(inGroup(''))
  // Folded groups are a per-account preference in this browser only.
  // svelte-ignore state_referenced_locally
  const foldedKey = `reet.rail.folded.${userId || 'rail'}`
  let folded = $state<Record<string, boolean>>({})
  const isFolded = (gid: string) => folded[gid] === true
  function toggleFold(gid: string) { folded = { ...folded, [gid]: !isFolded(gid) }; storeFolded() }
  function storeFolded() { try { localStorage.setItem(foldedKey, JSON.stringify(folded)) } catch { /* The rail still folds. */ } }
  // A wide desktop rail can be folded to the 56 px strip; Home makes the column
  // narrower and this rail shows its strip instead of the full cards.
  let wide = $state(typeof matchMedia === 'function' && matchMedia('(min-width: 1240px)').matches)
  function foldRail() { oncollapsed?.(true); void tick().then(() => opener?.focus()) }
  function expandRail() {
    oncollapsed?.(false)
    void tick().then(() => (full?.querySelector<HTMLElement>('.rail-fold, .add') ?? full?.querySelector<HTMLElement>('button'))?.focus())
  }
  let dragId = $state(''), drop = $state<{ groupId: string; index: number } | null>(null)
  function startDrag(event: DragEvent, p: FolderProject) {
    if (selecting) { event.preventDefault(); return }
    dragId = p.id; closeMenu(); groupMenu = null
    event.dataTransfer?.setData('text/plain', p.id)
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }
  function endDrag() { dragId = ''; drop = null }
  function overProject(event: DragEvent, p: FolderProject) {
    if (!dragId || dragId === p.id) return
    event.preventDefault(); event.stopPropagation()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    const gid = p.groupId ?? ''
    const list = inGroup(gid), index = list.findIndex(x => x.id === p.id)
    const box = (event.currentTarget as HTMLElement).getBoundingClientRect()
    drop = { groupId: gid, index: event.clientY < box.top + box.height / 2 ? index : index + 1 }
  }
  function overBlank(event: DragEvent, gid: string) {
    if (!dragId) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    drop = { groupId: gid, index: inGroup(gid).length }
  }
  function applyDrop() {
    const id = dragId, target = drop
    dragId = ''; drop = null
    if (!id || !target) return
    const movingProject = (projects ?? []).find(p => p.id === id)
    if (!movingProject || !onarrange) return
    const order = ['', ...sortedGroups.map(g => g.id)]
    const buckets = new Map(order.map(gid => [gid, inGroup(gid).filter(p => p.id !== id)]))
    const sourceGroup = movingProject.groupId ?? ''
    const sourceIndex = inGroup(sourceGroup).findIndex(p => p.id === id)
    let at = target.index
    if (sourceGroup === target.groupId && sourceIndex >= 0 && sourceIndex < at) at--
    const bucket = buckets.get(target.groupId) ?? buckets.get('')!
    bucket.splice(Math.max(0, Math.min(at, bucket.length)), 0, movingProject)
    const flat = order.flatMap(gid => (buckets.get(gid) ?? []).map(p => ({ id: p.id, version: p.version, groupId: gid })))
    onarrange(flat)
  }
  function chooseGroup(g: ProjectGroup) {
    if (!onscope) return
    onscope(scope?.kind === 'group' && scope.id === g.id ? null : { kind: 'group', id: g.id })
  }
  function scopeOfProject(p: FolderProject) { return scope?.kind === 'project' && scope.id === p.id }
  function scopeOfGroup(g: ProjectGroup) { return scope?.kind === 'group' && scope.id === g.id }

  // The group menu: rename and recolour, move and remove the group itself.
  let groupMenu = $state<string | null>(null), groupMenuButton: HTMLElement | null = null
  let groupMenuX = $state(0), groupMenuY = $state(0)
  async function openGroupMenu(g: ProjectGroup, button: HTMLElement) {
    if (groupMenu === g.id) { groupMenu = null; return }
    closeMenu(); groupMenu = g.id; groupMenuButton = button
    const box = button.getBoundingClientRect()
    groupMenuX = Math.min(Math.max(8, box.right - 212), Math.max(8, innerWidth - 222))
    groupMenuY = Math.min(Math.max(8, box.bottom + 4), Math.max(8, innerHeight - 320))
    await tick()
    document.querySelector<HTMLElement>(`#${CSS.escape(`${id}-group-menu-${g.id}`)} [role=menuitem]:not(:disabled)`)?.focus()
  }
  function closeGroupMenu(returnFocus = false) { groupMenu = null; if (returnFocus) groupMenuButton?.focus() }

  // One menu open at a time. Arrow keys move between its items, Escape closes it and gives focus back to its button.
  let menuFor = $state<string | null>(null)
  let menuButton: HTMLElement | null = null
  let menuX = $state(0), menuY = $state(0)
  async function openMenu(p: FolderProject, button: HTMLElement, point?: { x: number; y: number }) {
    if (menuFor === p.id && !point) { closeMenu(); return }
    groupMenu = null
    menuFor = p.id; menuButton = button
    const box = button.getBoundingClientRect()
    const at = point ?? { x: Math.max(8, box.right - 212), y: box.bottom + 4 }
    menuX = Math.min(Math.max(8, at.x), Math.max(8, innerWidth - 222))
    menuY = Math.min(Math.max(8, at.y), Math.max(8, innerHeight - 320))
    await tick()
    document.querySelector<HTMLElement>(`#${CSS.escape(`${id}-menu-${p.id}`)} [role=menuitem]:not(:disabled), #${CSS.escape(`${id}-menu-${p.id}`)} [role=menuitemradio]:not(:disabled)`)?.focus()
  }
  // Right-click opens the same menu where the pointer is, so the card itself offers New session, colour and edit.
  function contextMenu(event: MouseEvent, p: FolderProject) { event.preventDefault(); void openMenu(p, event.currentTarget as HTMLElement, { x: event.clientX, y: event.clientY }) }
  function closeMenu(returnFocus = false) {
    menuFor = null
    if (returnFocus) menuButton?.focus()
  }
  function menuKeys(event: KeyboardEvent) {
    const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role=menuitem]:not(:disabled), [role=menuitemradio]:not(:disabled)')]
    const at = items.indexOf(document.activeElement as HTMLElement)
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(true) }
    else if (event.key === 'ArrowDown') { event.preventDefault(); items[(at + 1) % items.length]?.focus() }
    else if (event.key === 'ArrowUp') { event.preventDefault(); items[(at - 1 + items.length) % items.length]?.focus() }
    else if (event.key === 'Home') { event.preventDefault(); items[0]?.focus() }
    else if (event.key === 'End') { event.preventDefault(); items.at(-1)?.focus() }
    else if (event.key === 'Tab') closeMenu()
  }
  // A choice closes the menu first; a dialog it opens gives focus back to the card's menu button.
  function pick(action: (opener: HTMLElement) => void) {
    const opener = menuButton
    closeMenu()
    if (opener) action(opener)
  }
  // Choosing a project narrows the board to it (Home); a page with no board
  // gives the choice to the launch bar's sentence (the Sessions screen).
  function chooseCard(p: FolderProject, el: HTMLElement) {
    if (panelOpen) closePanel(false)
    if (onscope) { onscope(scopeOfProject(p) ? null : { kind: 'project', id: p.id }); return }
    onchoose(p, el)
  }
  // A card's + and New session here start at once; a page without a launch bar (the Sessions rail) navigates instead.
  function startCard(p: FolderProject, el: HTMLElement) {
    if (panelOpen) closePanel(false)
    if (onnew) onnew(p, el)
    else onchoose(p, el)
  }
  function outside(event: PointerEvent) {
    const target = event.target as HTMLElement
    if (menuFor && !target.closest('.menu, .more')) closeMenu()
    if (groupMenu && !target.closest('.menu, .group-more')) closeGroupMenu()
    if (headMenu && !target.closest('.menu, .head-more')) headMenu = false
    // The card and heading menus are drawn outside the rail's box, but a press on one is a press inside the panel.
    if (panelOpen && !target.closest('aside.rail') && !target.closest(`[id^="${CSS.escape(`${id}-menu-`)}"], #${CSS.escape(`${id}-head-menu`)}`) && !target.closest('dialog')) closePanel(false)
  }

  onMount(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(foldedKey) || '{}')
      if (saved && typeof saved === 'object') folded = saved
    } catch { /* Folded groups start open. */ }
    const query = matchMedia('(min-width: 1240px)'), resized = () => { wide = query.matches }
    query.addEventListener('change', resized)
    return () => query.removeEventListener('change', resized)
  })
</script>

<svelte:document onpointerdown={outside} />

{#snippet projectCard(p: FolderProject)}
  {@const state = stateOf(p)}
  {@const wt = worktrees.get(p.id) ?? 0}
  {@const picked = selecting && isPicked(p)}
  {@const gid = p.groupId ?? ''}
  {@const list = inGroup(gid)}
  {@const index = list.findIndex(x => x.id === p.id)}
  <li class="item" class:selecting class:dragging={dragId === p.id}
    class:over-top={!!drop && drop.groupId === gid && drop.index === index && dragId !== p.id}
    class:over-bottom={!!drop && drop.groupId === gid && drop.index === index + 1 && dragId !== p.id}
    style={`--project-colour:${projectColourHex(p.colour)}`}
    ondragover={(e) => overProject(e, p)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
    <button type="button" class="card" class:chosen={!selecting && chosen === p.id} class:scoped={!selecting && scopeOfProject(p)} class:picked
      draggable={!selecting && !!onarrange}
      role={selecting ? 'checkbox' : undefined} aria-checked={selecting ? picked : undefined} aria-disabled={selecting && !picked && full_ ? 'true' : undefined}
      aria-current={!selecting && (scopeOfProject(p) || chosen === p.id) ? 'true' : undefined} aria-label={label(p)}
      onclick={(e) => cardClick(p, e.currentTarget)} oncontextmenu={(e) => cardContextMenu(e, p)}
      onpointerdown={(e) => pressStart(e, p)} onpointermove={pressMove} onpointerup={pressEnd} onpointercancel={pressEnd} onpointerleave={pressEnd}
      ondragstart={(e) => startDrag(e, p)} ondragend={endDrag}
      onkeydown={(e) => { if (!selecting && (e.key === 'ContextMenu' || e.shiftKey && e.key === 'F10')) void openMenu(p, e.currentTarget) }}>
      <span class="top">
        {#if selecting}<span class="tick" aria-hidden="true">{#if picked}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>{/if}</span>{/if}
        {#if !selecting && onarrange}<span class="grip" aria-hidden="true"><svg viewBox="0 0 12 16" fill="currentColor"><circle cx="3" cy="3.5" r="1.1"/><circle cx="9" cy="3.5" r="1.1"/><circle cx="3" cy="8" r="1.1"/><circle cx="9" cy="8" r="1.1"/><circle cx="3" cy="12.5" r="1.1"/><circle cx="9" cy="12.5" r="1.1"/></svg></span>{/if}
        {#if projectColourHex(p.colour) !== 'transparent'}<span class="identity" aria-hidden="true"></span>{/if}
        {#if p.isRepo}
          <svg class="icon repo" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg>
        {:else}
          <svg class="icon folder" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 6.5A1.5 1.5 0 014.5 5H9l2 2h8.5A1.5 1.5 0 0121 8.5v9a1.5 1.5 0 01-1.5 1.5h-15A1.5 1.5 0 013 17.5z" /></svg>
        {/if}
        <span class="name">{p.name}</span>
        <span class="tag">{machineTag(p)}</span>
      </span>
      <span class="meta">
        {#if p.baseBranch}<span class="branch">{p.baseBranch}</span>{/if}
        {#if state}<span class="state {state.tone}">{state.text}</span>{/if}
        {#if wt}<span class="wt">{worktreesInUse(wt)}</span>{/if}
      </span>
    </button>
    {#if !selecting}
      {#if onnew}<button type="button" class="start" aria-label={`New session in ${p.name}`} title="Start a session in this project" onclick={(e) => startCard(p, e.currentTarget)}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>
      </button>{/if}
      <button type="button" class="more" aria-label={`More for ${p.name}`} aria-haspopup="menu" aria-expanded={menuFor === p.id}
        aria-controls={menuFor === p.id ? `${id}-menu-${p.id}` : undefined} onclick={(e) => openMenu(p, e.currentTarget)}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
      </button>
    {/if}
    {#if p.note}
      <div class="note" role="note">
        <p>Moved from your saved Projects. The starter prompts of {p.note} weren't kept here; their old runs are in Activity.</p>
        <button type="button" class="ghost" aria-label={`Dismiss the note on ${p.name}`} disabled={pending.has(p.id)} onclick={() => ondismiss(p)}>Dismiss</button>
      </div>
    {/if}
  </li>
{/snippet}

<aside class="rail" class:open={panelOpen} class:sessions={variant === 'sessions'} class:collapsed bind:this={aside} aria-labelledby={`${id}-title`}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="full" id={`${id}-full`} bind:this={full} onkeydown={panelKeys} style:top={panelOpen ? `${panelTop}px` : undefined} style:height={panelOpen ? `calc(100dvh - ${panelTop}px)` : undefined}>
    <header>
      {#if variant === 'home'}
        <button type="button" class="rail-fold" aria-label="Fold the project rail" title="Fold the rail" onclick={foldRail}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 5l-7 7 7 7" /></svg>
        </button>
      {/if}
      <h2 id={`${id}-title`}>{variant === 'sessions' && projects ? `Projects · ${projects.length}` : 'Projects'}</h2>
      {#if panelOpen}<button type="button" class="close-panel" aria-label="Close the project rail" onclick={() => closePanel(true)}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18" /></svg></button>{/if}
      {#if ongroupadd || (projects && projects.length > 1)}
        <button type="button" class="head-more" bind:this={headButton} aria-label="Projects menu" aria-haspopup="menu" aria-expanded={headMenu}
          aria-controls={headMenu ? `${id}-head-menu` : undefined} onclick={openHeadMenu}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
        </button>
      {/if}
      <button type="button" class="add" aria-label="Add a project" disabled={busy} aria-busy={busy} onclick={(e) => onadd(e.currentTarget)}>
        {#if variant === 'sessions'}Add a project{:else}<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>Add{/if}
      </button>
    </header>
    {#if selecting}
      <div class="picking">
        <p role="status" aria-live="polite">{pickedLine}</p>
        <button type="button" class="ghost" aria-label="Done selecting" onclick={doneSelecting}>Done</button>
      </div>
      {@render selectAction?.()}
    {/if}
    {#if savedWaiting > 0}
      <div class="waiting">
        <p>{folders(savedWaiting)} from your saved Projects {savedWaiting === 1 ? "isn't" : "aren't"} in the rail yet.</p>
        <button type="button" class="ghost" disabled={importing} aria-busy={importing} onclick={onimport}>Add the waiting folders ({savedWaiting})</button>
      </div>
    {/if}
    {#if !projects && failed}
      <p class="none">Couldn't read your projects just now.</p>
    {:else if !projects}
      <p class="none">Reading your projects…</p>
    {:else if projects.length === 0 && sortedGroups.length === 0}
      <p class="none">Add a project to start an agent in one of your folders.</p>
    {:else}
      <!-- Ungrouped projects come first; groups follow in their own order. -->
      <div class="group-block ungrouped" class:captioned={sortedGroups.length > 0}
        ondragover={(e) => overBlank(e, '')} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
        {#if sortedGroups.length > 0 && ungrouped.length > 0}<p class="group-caption">Ungrouped</p>{/if}
        <ul class="items">{#each ungrouped as p (p.id)}{@render projectCard(p)}{/each}</ul>
        {#if sortedGroups.length > 0 && ungrouped.length === 0}
          <div class="group-empty" class:active={!!dragId && drop?.groupId === ''}
            ondragover={(e) => overBlank(e, '')} ondrop={(e) => { e.preventDefault(); applyDrop() }}>{dragId ? 'Drop here to ungroup' : 'Projects outside a group live here.'}</div>
        {/if}
      </div>
      {#if sortedGroups.length}
        <div class="groups">
          {#each sortedGroups as g (g.id)}
            {@const items = inGroup(g.id)}
            <section class="group-block" class:folded={isFolded(g.id)} data-group-id={g.id}
              class:drop-group={!!dragId && drop?.groupId === g.id}
              ondragover={(e) => overBlank(e, g.id)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
              <div class="group-head">
                <button type="button" class="group-fold" aria-expanded={!isFolded(g.id)} aria-label={`${isFolded(g.id) ? 'Open' : 'Fold'} ${g.name}`} onclick={() => toggleFold(g.id)}>
                  <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M9 6l6 6-6 6" /></svg>
                </button>
                <button type="button" class="group-name" class:scoped={scopeOfGroup(g)} aria-current={scopeOfGroup(g) ? 'true' : undefined}
                  onclick={() => chooseGroup(g)} title={`Show sessions in ${g.name}`}>
                  <span class="group-dot" style={`--swatch:${projectColourHex(g.colour)}`} aria-hidden="true"></span>
                  <span class="group-label">{g.name}</span>
                  <span class="group-count">{items.length}</span>
                </button>
                <button type="button" class="group-more" aria-label={`More for group ${g.name}`} aria-haspopup="menu" aria-expanded={groupMenu === g.id}
                  aria-controls={groupMenu === g.id ? `${id}-group-menu-${g.id}` : undefined} onclick={(e) => void openGroupMenu(g, e.currentTarget)}>
                  <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
                </button>
              </div>
              {#if !isFolded(g.id)}
                {#if items.length}
                  <ul class="items">{#each items as p (p.id)}{@render projectCard(p)}{/each}</ul>
                {:else}
                  <div class="group-empty" class:active={!!dragId && drop?.groupId === g.id}
                    ondragover={(e) => overBlank(e, g.id)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>{dragId ? 'Drop here to add to this group' : 'Drag projects here to group them.'}</div>
                {/if}
              {/if}
            </section>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
  <div class="strip">
    <button type="button" class="add-icon" aria-label="Add a project" disabled={busy} aria-busy={busy} onclick={(e) => onadd(e.currentTarget)}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>
    </button>
    <button type="button" class="manage" bind:this={opener} aria-label={collapsed ? 'Expand the project rail' : toggleName} title={collapsed ? 'Expand the project rail' : toggleName}
      aria-expanded={collapsed || panelOpen} aria-controls={`${id}-full`} onclick={() => {
        if (collapsed && wide) { expandRail(); return }
        openPanel()
      }}>
      {#if collapsed}
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7" /></svg>
      {:else}
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16" /></svg>
      {/if}
      {#if waiting}<i class="marker" aria-hidden="true"></i>{/if}
    </button>
    {#if selecting}
      <p class="picked-count" title={pickedLine}><span class="vh">{'Chosen: '}</span>{selected.length}</p>
      <!-- Select can start here (a long press on touch), so it can end here too. -->
      <button type="button" class="add-icon stop" aria-label="Stop selecting" title="Stop selecting" onclick={stopFromStrip}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    {/if}
    {#each projects ?? [] as p (p.id)}
      {@const wt = worktrees.get(p.id) ?? 0}
      {@const picked = selecting && isPicked(p)}
      <button type="button" class="mini" class:chosen={!selecting && (scopeOfProject(p) || chosen === p.id)} class:picked aria-current={!selecting && (scopeOfProject(p) || chosen === p.id) ? 'true' : undefined}
        role={selecting ? 'checkbox' : undefined} aria-checked={selecting ? picked : undefined} aria-disabled={selecting && !picked && full_ ? 'true' : undefined}
        aria-label={label(p)} title={label(p)} style={`--project-colour:${projectColourHex(p.colour)}`}
        onclick={(e) => { if (swallow === p.id && Date.now() < swallowUntil) { swallow = ''; return } if (selecting) toggle(p); else if (onscope) onscope(scopeOfProject(p) ? null : { kind: 'project', id: p.id }); else onchoose(p, e.currentTarget) }}
        oncontextmenu={(e) => cardContextMenu(e, p)} onpointerdown={(e) => pressStart(e, p)} onpointermove={pressMove} onpointerup={pressEnd} onpointercancel={pressEnd} onpointerleave={pressEnd}>
        <span class="mini-initial" aria-hidden="true">{initial(p.name)}</span>
        {#if p.baseBranch}<span class="mini-branch" aria-hidden="true">{p.baseBranch}</span>{/if}
        {#if p.isRepo}<svg class="badge" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg>{/if}
        {#if (running.get(p.id) ?? 0) > 0}<i class="live" aria-hidden="true"></i>{/if}
        {#if wt}<span class="mini-wt" aria-hidden="true">{wt}</span>{/if}
      </button>
    {/each}
  </div>
</aside>

{#if headMenu}
  <!-- svelte-ignore a11y_interactive_supports_focus -->
  <div class="menu" id={`${id}-head-menu`} style:left={`${menuX}px`} style:top={`${menuY}px`} role="menu" aria-label="Projects" tabindex="-1" onkeydown={headKeys}>
    {#if ongroupadd}<button type="button" role="menuitem" onclick={groupFromMenu}>New group</button>{/if}
    <button type="button" role="menuitem" onclick={selectFromMenu}>{selecting ? 'Stop selecting' : 'Select projects'}</button>
  </div>
{/if}

{#if groupMenu}
  {@const group = sortedGroups.find(g => g.id === groupMenu)}
  {#if group}
    {@const index = sortedGroups.findIndex(g => g.id === group.id)}
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="menu" id={`${id}-group-menu-${group.id}`} style:left={`${groupMenuX}px`} style:top={`${groupMenuY}px`} role="menu" aria-label={group.name} tabindex="-1"
      onkeydown={(e) => menuKeys(e)}>
      <button type="button" role="menuitem" onclick={() => { const target = group; const button = groupMenuButton; closeGroupMenu(); if (button) ongroupedit?.(target, button) }}>Rename group</button>
      <button type="button" role="menuitem" disabled={ongroupmovebusy || index === 0} onclick={() => { const target = group; closeGroupMenu(true); ongroupmove?.(target, -1) }}>Move group up</button>
      <button type="button" role="menuitem" disabled={ongroupmovebusy || index === sortedGroups.length - 1} onclick={() => { const target = group; closeGroupMenu(true); ongroupmove?.(target, 1) }}>Move group down</button>
      <span class="colour-label">Group colour</span>
      <div class="colours">{#each PROJECT_COLOURS as choice}<button type="button" role="menuitemradio" aria-checked={projectColour(group.colour) === choice.id} aria-label={choice.name} title={choice.name} style={`--swatch:${choice.hex}`} onclick={() => { const target = group; ongroupcolour?.(target, choice.id); closeGroupMenu(true) }}>{choice.id === '' ? '×' : ''}</button>{/each}</div>
      <button type="button" role="menuitem" class="danger" onclick={() => { const target = group; const button = groupMenuButton; closeGroupMenu(); if (button) ongroupremove?.(target, button) }}>Remove group</button>
    </div>
  {/if}
{/if}

{#if menuFor}
  {@const project = (projects ?? []).find(item => item.id === menuFor)}
  {#if project}
    {@const gid = project.groupId ?? ''}
    {@const index = inGroup(gid).findIndex(item => item.id === project.id)}
    {@const count = inGroup(gid).length}
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="menu" id={`${id}-menu-${project.id}`} style:left={`${menuX}px`} style:top={`${menuY}px`} role="menu" aria-label={project.name} tabindex="-1" onkeydown={menuKeys}>
      <button type="button" role="menuitem" onclick={() => { const target = project; pick(o => startCard(target, o)) }}>New session here</button>
      <button type="button" role="menuitem" onclick={() => { const target = project; pick(o => onedit(target, o)) }}>Edit</button>
      <button type="button" role="menuitem" onclick={() => { const target = project; pick(o => onworktrees(target, o)) }}>Worktrees</button>
      <button type="button" role="menuitem" disabled={moving || pending.size > 0 || index === 0} onclick={() => { const target = project; closeMenu(true); onmove(target, -1) }}>Move up</button>
      <button type="button" role="menuitem" disabled={moving || pending.size > 0 || index === count - 1} onclick={() => { const target = project; closeMenu(true); onmove(target, 1) }}>Move down</button>
      {#if onmovetogroup}
        <span class="colour-label">Move to group</span>
        <div class="group-picks">
          {#each sortedGroups as g (g.id)}
            <button type="button" role="menuitemradio" aria-checked={gid === g.id} disabled={pending.has(project.id)} onclick={() => { const target = project; closeMenu(true); onmovetogroup?.(target, g.id) }}>
              <span class="pick-dot" style={`--swatch:${projectColourHex(g.colour)}`} aria-hidden="true"></span>{g.name}
            </button>
          {/each}
          <button type="button" role="menuitemradio" aria-checked={gid === ''} disabled={pending.has(project.id)} onclick={() => { const target = project; closeMenu(true); onmovetogroup?.(target, '') }}>
            <span class="pick-dot none" aria-hidden="true"></span>No group
          </button>
        </div>
      {/if}
      <button type="button" role="menuitem" class="danger" disabled={pending.has(project.id)} onclick={() => { const target = project; pick(o => onremove(target, o)) }}>Remove from reet</button>
      <span class="colour-label">Project colour</span>
      <div class="colours">{#each PROJECT_COLOURS as choice}<button type="button" role="menuitemradio" aria-checked={projectColour(project.colour) === choice.id} aria-label={choice.name} title={choice.name} style={`--swatch:${choice.hex}`} onclick={() => oncolour(project, choice.id)}>{choice.id === '' ? '×' : ''}</button>{/each}</div>
    </div>
  {/if}
{/if}


<style>
  /* The mock-up's rail: 272 px of content inside 14 px padding and a 1 px rule (301 px in all). */
  .rail { border-right: 1px solid var(--line-soft); align-self: stretch; min-width: 0; line-height: 1.3; }
  .full { position: sticky; top: 0; max-height: 100dvh; overflow-y: auto; padding: 18px 14px; display: flex; flex-direction: column; gap: 12px; min-height: calc(100dvh - 61px); box-sizing: border-box; }
  header { display: flex; align-items: center; }
  h2 { flex-grow: 1; margin: 0; font: 600 11px/1.3 var(--sans); letter-spacing: 0.09em; text-transform: uppercase; color: var(--muted); }
  .add { display: flex; align-items: center; gap: 6px; height: 30px; padding: 0 10px; border: 1px solid var(--line-mid); border-radius: 8px; background: transparent; color: var(--ink); font-size: 13px; font-weight: 500; cursor: pointer; }
  .add svg, .add-icon svg { width: 14px; height: 14px; }
  .add:hover:not(:disabled), .add-icon:hover:not(:disabled) { border-color: var(--muted); }
  .add:disabled, .add-icon:disabled { opacity: 0.5; cursor: default; }
  .rail-fold { display: grid; place-items: center; width: 24px; height: 24px; margin: 0 6px 0 0; padding: 0; border: 0; border-radius: 6px; background: transparent; color: var(--muted); cursor: pointer; flex: none; }
  .rail-fold:hover { background: var(--card); color: var(--ink); }
  .rail-fold svg { width: 15px; height: 15px; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
  .items { gap: 8px; }
  .groups { display: flex; flex-direction: column; gap: 14px; }
  .group-block { display: flex; flex-direction: column; gap: 7px; min-width: 0; border-radius: 10px; }
  .group-block.captioned { gap: 6px; }
  .group-caption { margin: 0 3px; font: 600 10px/1.3 var(--sans); letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }
  .group-head { display: flex; align-items: center; gap: 2px; padding: 3px 3px 3px 1px; border-radius: 8px; transition: background .15s; }
  .group-head:hover { background: var(--line-soft); }
  .group-fold { flex: none; display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 5px; background: transparent; color: var(--muted); cursor: pointer; }
  .group-fold:hover { color: var(--ink); }
  .group-fold svg { width: 12px; height: 12px; transition: transform .16s var(--ease-out, ease); }
  .folded .group-fold svg { transform: rotate(-90deg); }
  .group-name { flex: 1; display: flex; align-items: center; gap: 7px; min-width: 0; padding: 4px 2px; border: 0; border-radius: 6px; background: none; color: var(--ink-soft); font: 600 12px/1.2 var(--sans); text-align: left; cursor: pointer; }
  .group-name:hover { color: var(--ink); }
  .group-name.scoped { color: var(--accent-text); }
  .group-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--swatch); box-shadow: inset 0 0 0 1px var(--line-deep); }
  .group-label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .group-count { flex: none; margin-left: 1px; padding: 0 6px; border: 1px solid var(--line); border-radius: 99px; background: var(--bg-sunk); color: var(--muted); font: 500 10px/16px var(--sans); }
  .group-more { flex: none; display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 5px; background: transparent; color: var(--muted); cursor: pointer; opacity: 0; }
  .group-head:hover .group-more, .group-more:focus-visible, .group-more[aria-expanded='true'] { opacity: 1; }
  .group-more svg { width: 14px; height: 14px; }
  .group-block > .items { margin-left: 10px; padding-left: 7px; border-left: 2px solid var(--line-soft); }
  .group-empty { margin-left: 10px; padding: 13px 10px; border: 1px dashed var(--line-mid); border-radius: 9px; color: var(--muted); font-size: 12px; text-align: center; transition: border-color .15s, background .15s, color .15s; }
  .group-empty.active { border-color: var(--accent); color: var(--accent-text); background: var(--accent-wash); }
  .group-block.drop-group > .group-head { background: var(--accent-wash); }
  /* A drag shows a line where the project will land. */
  .item { --drop-line: var(--accent); }
  .item::before { content: ''; position: absolute; left: 3px; right: 3px; height: 2px; border-radius: 2px; background: var(--drop-line); opacity: 0; pointer-events: none; }
  .item.over-top::before { opacity: 1; top: -5px; }
  .item.over-bottom::before { opacity: 1; bottom: -5px; }
  .item.dragging { opacity: .45; }
  .card[draggable='true'] { cursor: grab; }
  .item.dragging .card { cursor: grabbing; }
  .item { position: relative; display: flex; flex-direction: column; gap: 6px; }
  .card { display: flex; flex-direction: column; gap: 6px; width: 100%; padding: 10px 12px; border: 1px solid var(--line); border-radius: 11px; background: var(--card); color: var(--ink); font: inherit; text-align: left; cursor: pointer; }
  .card:hover { border-color: var(--line-strong); }
  .card.chosen, .card.scoped { border-color: var(--accent); }
  .card.scoped { box-shadow: inset 0 0 0 1px var(--accent); }
  .top { display: flex; align-items: center; gap: 8px; min-width: 0; padding-right: 58px; }
  .icon { width: 14px; height: 14px; flex: none; }
  .grip { flex: none; display: grid; place-items: center; width: 9px; height: 14px; margin-left: -2px; color: var(--faint); opacity: 0; transition: opacity .15s; }
  .grip svg { width: 8px; height: 12px; }
  .item:hover .grip, .card:focus-visible .grip { opacity: 1; }
  .icon.repo { fill: var(--repo); }
  .icon.folder { color: var(--muted); }
  .name { flex-grow: 1; min-width: 0; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tag { flex: none; max-width: 45%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; color: var(--muted); border: 1px solid var(--line-mid); border-radius: 99px; padding: 1px 8px; }
  .meta { display: flex; align-items: center; gap: 10px; min-width: 0; font-size: 12px; color: var(--muted); }
  .branch { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--mono); }
  .state { flex: none; }
  .state.good { color: var(--good); }
  .state.warn { color: var(--warn); }
  .wt { margin-left: auto; flex: none; white-space: nowrap; }
  /* The card's menu button shows on hover or keyboard focus with a mouse, and always on touch. */
  .more, .start { position: absolute; top: 6px; display: grid; place-items: center; width: 26px; height: 26px; padding: 0; border: 1px solid var(--line); border-radius: 7px; background: var(--card-raised); color: var(--ink-soft); cursor: pointer; opacity: 0; }
  .more { right: 6px; }
  /* The + is always in reach: it is the one press that starts a session in this project. */
  .start { right: 38px; opacity: 1; }
  .more svg, .start svg { width: 15px; height: 15px; }
  .item:hover .more, .more:focus-visible, .start:focus-visible, .more[aria-expanded='true'] { opacity: 1; }
  .menu { position: fixed; z-index: 30; min-width: 198px; display: flex; flex-direction: column; padding: 4px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised); box-shadow: 0 12px 30px var(--scrim); }
  .menu button { min-height: 34px; padding: 0 12px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; font-size: 13px; text-align: left; cursor: pointer; }
  .menu button:hover:not(:disabled), .menu button:focus-visible { background: var(--line-soft); }
  .menu button:disabled { color: var(--faint); cursor: default; }
  .menu .danger { color: var(--bad); }
  .identity { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--project-colour); }
  .colour-label { display: block; padding: 5px 12px 1px; color: var(--muted); font-size: 11px; }
  .colours { display: flex; gap: 7px; padding: 4px 12px 6px; }
  .menu .colours button { flex: none; width: 20px; height: 20px; min-height: 0; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--swatch); color: var(--ink); font-size: 11px; line-height: 1; }
  .menu .colours button[aria-checked='true'] { box-shadow: 0 0 0 2px var(--accent); }
  .menu .colours button:first-child { color: var(--muted); }
  .group-picks { display: flex; flex-direction: column; gap: 1px; max-height: 168px; overflow-y: auto; padding: 2px 6px 5px; }
  .menu .group-picks button { display: flex; align-items: center; gap: 7px; min-height: 30px; padding: 0 6px; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .menu .group-picks button[aria-checked='true'] { color: var(--accent-text); background: var(--accent-wash); }
  .pick-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--swatch); box-shadow: inset 0 0 0 1px var(--line-deep); }
  .pick-dot.none { background: transparent; }
  .note, .waiting { display: flex; flex-direction: column; gap: 8px; padding: 10px 12px; border: 1px solid var(--line-strong); border-radius: 11px; background: var(--card); font-size: 13px; }
  .note p, .waiting p { margin: 0; color: var(--ink-soft); overflow-wrap: anywhere; line-height: 1.45; }
  .ghost { align-self: flex-start; min-height: 32px; padding: 4px 12px; border: 1px solid var(--line-mid); border-radius: 8px; background: transparent; color: var(--ink); font: inherit; font-size: 13px; font-weight: 500; cursor: pointer; text-align: left; }
  .ghost:hover:not(:disabled) { border-color: var(--muted); }
  .ghost:disabled { opacity: 0.5; cursor: default; }
  .none { margin: 0; color: var(--muted); font-size: 13px; }
  .strip { display: none; }
  /* Select: the cards are checkboxes, with a tick box where the menu button was. */
  .head-more { display: grid; place-items: center; width: 30px; height: 30px; margin-right: 8px; padding: 0; border: 1px solid var(--line-mid); border-radius: 8px; background: transparent; color: var(--ink-soft); cursor: pointer; }
  .head-more svg { width: 16px; height: 16px; }
  .head-more[aria-expanded='true'] { border-color: var(--muted); }
  .picking { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .picking p { margin: 0; font-size: 13px; color: var(--ink-soft); }
  .tick { flex: none; display: grid; place-items: center; width: 16px; height: 16px; border: 1.5px solid var(--line-deep); border-radius: 4px; color: var(--accent-ink); }
  .tick svg { width: 12px; height: 12px; }
  .card.picked { border-color: var(--accent); }
  .card.picked .tick { border-color: var(--accent); background: var(--accent); }
  .card[aria-disabled='true'] { opacity: 0.55; cursor: default; }
  .item.selecting .card { min-height: 44px; }
  /* A long press on a card starts Select: the browser's own text selection and link callout stay out of its way. */
  @media (pointer: coarse) { .card, .mini { -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; } }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  .close-panel { display: grid; place-items: center; width: 30px; height: 30px; margin-right: 8px; padding: 0; border: 1px solid var(--line-mid); border-radius: 8px; background: transparent; color: var(--ink-soft); cursor: pointer; }
  .close-panel svg { width: 14px; height: 14px; }

  @media (pointer: coarse) {
    .close-panel, .head-more { width: 44px; height: 44px; }
    .add { height: 44px; }
    .card { padding-right: 96px; }
    .more, .start { opacity: 1; top: 50%; transform: translateY(-50%); width: 44px; height: 44px; border: 0; background: transparent; }
    .more { right: 4px; }
    .start { right: 48px; }
    .item:has(.note) .more, .item:has(.note) .start { top: 4px; transform: none; }
    .menu { top: 52px; }
    .menu button { min-height: 44px; }
    .ghost { min-height: 44px; }
  }
  /* 1100 to 1239 px: a 56 px strip with the same cards as buttons, each named in full. */
  @media (min-width: 1100px) and (max-width: 1239px) {
    .full { display: none; }
    .strip { position: sticky; top: 0; display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 18px 0; min-height: calc(100dvh - 61px); box-sizing: border-box; }
    .add-icon, .manage, .mini { position: relative; display: grid; place-items: center; width: 40px; min-height: 40px; padding: 0; border: 1px solid var(--line-mid); border-radius: 10px; background: var(--card); color: var(--ink); font: inherit; font-weight: 600; cursor: pointer; }
    .add-icon, .manage { background: transparent; }
    .manage svg { width: 16px; height: 16px; }
    .manage[aria-expanded='true'] { border-color: var(--accent); }
    .marker { position: absolute; top: -3px; right: -3px; width: 9px; height: 9px; border-radius: 50%; background: var(--accent-text); border: 2px solid var(--bg); }
    /* Each project: its initial, then its base branch (mono, cut short; the whole is in the button's name), the repository
       diamond, a running dot and the worktrees in use as a small count. */
    .mini { width: 44px; padding: 5px 0 4px; gap: 1px; align-content: center; }
    .mini-initial { font-size: 14px; line-height: 1.2; }
    .mini-branch { max-width: 40px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 400 10px/1.2 var(--mono); color: var(--muted); }
    .mini.chosen, .mini.picked { border-color: var(--accent); }
    .mini.picked { box-shadow: inset 0 0 0 1px var(--accent); }
    .mini[aria-disabled='true'] { opacity: 0.55; }
    /* Select's count under the toggle: how many are chosen. */
    .picked-count { margin: 0; min-width: 28px; padding: 1px 6px; border-radius: 99px; background: var(--accent); color: var(--accent-ink); font: 600 12px/18px var(--sans); text-align: center; }
    .badge { position: absolute; top: 3px; left: 3px; width: 8px; height: 8px; fill: var(--repo); }
    .live { position: absolute; top: 4px; right: 4px; width: 7px; height: 7px; border-radius: 50%; background: var(--good); }
    .mini-wt { position: absolute; bottom: -5px; right: -5px; min-width: 15px; height: 15px; padding: 0 3px; box-sizing: border-box; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card-raised); color: var(--ink-soft); font: 500 10px/13px var(--sans); text-align: center; }
    /* The full rail, opened over the page from the strip. */
    .rail.open .full { display: flex; position: fixed; z-index: 20; top: 0; left: 0; width: 301px; height: 100dvh; max-height: none; min-height: 0; background: var(--bg); border-right: 1px solid var(--line-soft); box-shadow: 12px 0 40px var(--scrim); }
  }
  @media (pointer: coarse) and (min-width: 1100px) and (max-width: 1239px) { .add-icon, .manage { width: 44px; min-height: 44px; } .mini { min-height: 44px; } }
  /* A wide desktop rail folded to its 56 px strip: the same strip the
     1100-1239 px layout uses, with the toggle opening it again. */
  @media (min-width: 1240px) {
    .rail.collapsed { border-right-color: var(--line-soft); }
    .rail.collapsed .full { display: none; }
    .rail.collapsed .strip { position: sticky; top: 0; display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 18px 0; min-height: calc(100dvh - 61px); box-sizing: border-box; }
    .rail.collapsed .add-icon, .rail.collapsed .manage, .rail.collapsed .mini { position: relative; display: grid; place-items: center; width: 40px; min-height: 40px; padding: 0; border: 1px solid var(--line-mid); border-radius: 10px; background: var(--card); color: var(--ink); font: inherit; font-weight: 600; cursor: pointer; }
    .rail.collapsed .add-icon, .rail.collapsed .manage { background: transparent; }
    .rail.collapsed .manage svg { width: 16px; height: 16px; }
    .rail.collapsed .manage[aria-expanded='true'] { border-color: var(--accent); }
    .rail.collapsed .marker { position: absolute; top: -3px; right: -3px; width: 9px; height: 9px; border-radius: 50%; background: var(--accent-text); border: 2px solid var(--bg); }
    .rail.collapsed .mini { width: 44px; padding: 5px 0 4px; gap: 1px; align-content: center; }
    .rail.collapsed .mini-initial { font-size: 14px; line-height: 1.2; }
    .rail.collapsed .mini-branch { max-width: 40px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 400 10px/1.2 var(--mono); color: var(--muted); }
    .rail.collapsed .mini.chosen, .rail.collapsed .mini.picked { border-color: var(--accent); }
    .rail.collapsed .mini.picked { box-shadow: inset 0 0 0 1px var(--accent); }
    .rail.collapsed .mini[aria-disabled='true'] { opacity: 0.55; }
    .rail.collapsed .picked-count { margin: 0; min-width: 28px; padding: 1px 6px; border-radius: 99px; background: var(--accent); color: var(--accent-ink); font: 600 12px/18px var(--sans); text-align: center; }
    .rail.collapsed .badge { position: absolute; top: 3px; left: 3px; width: 8px; height: 8px; fill: var(--repo); }
    .rail.collapsed .live { position: absolute; top: 4px; right: 4px; width: 7px; height: 7px; border-radius: 50%; background: var(--good); }
    .rail.collapsed .mini-wt { position: absolute; bottom: -5px; right: -5px; min-width: 15px; height: 15px; padding: 0 3px; box-sizing: border-box; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card-raised); color: var(--ink-soft); font: 500 10px/13px var(--sans); text-align: center; }
  }
  @media (max-width: 1239px) { .rail-fold { display: none; } }
  /* Tablet and phone: the rail is a section on the Sessions screen, with the same cards. */
  @media (max-width: 1099px) {
    .rail { border-right: 0; }
    .full { position: static; max-height: none; min-height: 0; overflow: visible; padding: 0; }
    h2 { font: 600 21px/1.3 var(--serif); letter-spacing: 0; text-transform: none; color: var(--ink); }
    /* The phone Sessions screen draws the heading as "Projects · N" over the cards, with a 44 px Add a project. */
    .rail.sessions h2 { font: 600 11px/1.3 var(--sans); letter-spacing: 0.09em; text-transform: uppercase; color: var(--muted); }
    .rail.sessions .add { height: 44px; padding: 0 14px; border-color: var(--line-strong); border-radius: 10px; font-size: 15px; }
  }
</style>
