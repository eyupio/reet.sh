<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { toolName, notStartedText, validatePrompt } from '../agentLaunch'
  import { agentChoices } from '../catalogue'
  import { getYoloSetting, type YoloSetting } from '../api'
  import { yoloGates } from '../launchBar'
  import YoloDialog from '../launch/YoloDialog.svelte'
  import BaseConfirm from './BaseConfirm.svelte'
  import { baseQuestion, repoProjectAt } from '../baseGate'
  import { startProblem, type StartPlace, type StartTarget } from '../draftOptions'
  import { shortAge, stateWord } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import { listDrafts, getDraft, deleteDraft, startDraft } from '../sessionBacklog'
  import { boardItems, dropIntent, emptyFilters, filterBoard, filterCount, sortBoard, STAGES, type BoardView, type DraftImage, type SessionDraft } from '../sessionBoard'
  import { launchUpload, readiness, sessionHref, TerminalError, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { imagePrompt } from '../terminal/upload'
  import { rememberUpload } from '../terminal/thumbs'
  import type { LaunchDefaults } from '../launchBar'
  import SessionComposer from './SessionComposer.svelte'
  import BoardIcon from './BoardIcon.svelte'
  import DraftImagePreview from './DraftImagePreview.svelte'
  import ImageLightbox from '../terminal/ImageLightbox.svelte'
  import SessionActions from '../sessions/SessionActions.svelte'
  import { factoryBadge, type FactoryMember } from '../gafferOutcome'

  let { embedded = false, factory = new Map(), userId, sessions, sessionsFailed = false, projects, machines, health, profile, now, initialProject = '', removeDefault = false,
    scopeIds = undefined, scopeKind = undefined, scopeName = '', onclearscope,
    onchanged, onsignedout, onstart }: {
    embedded?: boolean
    /** Sessions that are part of a Factory pipeline, by session id. */
    factory?: Map<string, FactoryMember>
    userId: number; sessions: TerminalSession[] | undefined; sessionsFailed?: boolean; projects: FolderProject[]; machines: TerminalMachine[]
    health: MachineHealth[]; profile: LaunchDefaults; now: number; initialProject?: string; removeDefault?: boolean
    /** The projects the board is narrowed to; undefined shows every project. */
    scopeIds?: string[]; scopeKind?: 'project' | 'group'; scopeName?: string; onclearscope?: () => void
    onchanged: () => void | Promise<void>; onsignedout: () => void; /** Opens the standard Start an agent dialog for a session that starts now. */ onstart: (opener: HTMLElement) => void
  } = $props()
  const id = $props.id()
  const life = new AbortController()
  const signal = () => AbortSignal.any([life.signal, AbortSignal.timeout(30_000)])
  let drafts = $state<SessionDraft[] | undefined>(), problem = $state(''), notice = $state('')
  let view = $state<BoardView>('board'), sort = $state('newest'), compact = $state(true)
  let filters = $state(emptyFilters()), filtersOpen = $state(false)
  // Cards start as a scan-friendly summary of their core details; only the card's own prompt or branch needs opening.
  let expanded = $state<string[]>([])
  const PAGE_SIZE = 5
  let visible = $state<Record<string, number>>({backlog: PAGE_SIZE, active: PAGE_SIZE, finished: PAGE_SIZE})
  const windowKey = $derived(JSON.stringify({filters, sort, scopeIds, view}))
  $effect(() => { void windowKey; visible = {backlog: PAGE_SIZE, active: PAGE_SIZE, finished: PAGE_SIZE} })
  // `start` opens an idea to be refined and started, rather than just edited.
  let composer = $state<{ draft: SessionDraft | null; opener: HTMLElement; start?: boolean } | null>(null)
  let removing = $state<SessionDraft | null>(null), deleteBusy = $state(false)
  let working = $state(''), phase = $state(''), loadingId = $state('')
  // Dragging a backlog card onto Active starts it. The source element is kept so a refined idea can return focus to it.
  let dragDraft = $state(''), dragOpener: HTMLElement | undefined, dropActive = $state(false)
  // YOLO is confirmed here, at Start, never when the item is saved or remembered by the browser.
  let yoloAsk = $state<SessionDraft | null>(null), yoloSetting = $state<YoloSetting | null>(null)
  // Where a plan in a saved repository would start, asked of the machine at Start as the two start dialogs ask it. The
  // opener is kept so Cancel gives the card's Start the keyboard back.
  let baseAsk = $state<{ draft: SessionDraft; say: string; host: string; confirmed: number } | null>(null)
  let baseOpener: HTMLElement | undefined
  let actions = $state<SessionActions>(), filterButton = $state<HTMLButtonElement>(), filterPanel = $state<HTMLDivElement>(), deleteDialog = $state<HTMLDialogElement>()
  let generation = 0
  let activeRequests = new Set<string>()
  // A reference-image preview opens on hover or focus of a backlog card's image badge. Large image bodies are only
  // fetched on that first look, then kept for the visit. The cache is keyed by the draft's version, so changing its
  // attachments can never show the previous ones.
  let imagePreview = $state<{ draft: SessionDraft; anchor: HTMLElement } | null>(null)
  let imageCache = $state<Record<string, DraftImage[]>>({})
  let imageLoading = $state<Record<string, boolean>>({})
  let imageErrors = $state<Record<string, string>>({})
  let lightbox = $state<{ items: { url: string; name: string }[]; index: number } | null>(null)
  let previewShowTimer: ReturnType<typeof setTimeout> | undefined, previewHideTimer: ReturnType<typeof setTimeout> | undefined
  const imageRequests = new Map<string, Promise<DraftImage[] | undefined>>()
  const machineNames = $derived(new Map(machines.map(m => [m.agentId, m.host])))
  const projectNames = $derived(new Map(projects.map(p => [p.id, p.name])))
  const all = $derived(boardItems(drafts ?? [], sessions ?? []))
  // The rail's choice narrows the board before the filters do.
  const scoped = $derived(scopeIds === undefined ? all : all.filter(item => scopeIds.includes(item.projectId || '')))
  const filtered = $derived(sortBoard(filterBoard(scoped, filters, { machines: machineNames, projects: projectNames }), sort))
  const count = $derived(filterCount(filters))
  const hasFilters = $derived(count > 0 || !!filters.query.trim())
  const columns = $derived(STAGES.filter(stage => view === 'board' || stage.id === view))
  const counts = $derived(Object.fromEntries(STAGES.map(stage => [stage.id, filtered.filter(item => item.stage === stage.id).length])))
  const filterGroups = $derived([
    { key: 'machines' as const, label: 'Machine', options: [...new Set([...machines.map(m => m.agentId), ...all.map(i => i.agentId)])].map(value => ({ value, label: machineNames.get(value) || value })) },
    { key: 'projects' as const, label: 'Project', options: [...new Set([...projects.map(p => p.id), ...all.map(i => i.projectId || 'none')])].map(value => ({ value, label: value === 'none' ? 'No project' : projectNames.get(value) || 'Removed project' })) },
    { key: 'agents' as const, label: 'Agent', options: [...new Set(all.map(i => i.tool || 'shell'))].map(value => ({ value, label: value === 'shell' ? 'Shell' : toolName(value) })) },
    { key: 'models' as const, label: 'Model', options: [...new Set(all.map(i => i.model || 'default'))].map(value => ({ value, label: value === 'default' ? 'Agent default' : value })) },
  ])
  const chips = $derived(filterGroups.flatMap(group => filters[group.key].map(value => ({ key: group.key, value, label: group.options.find(o => o.value === value)?.label ?? value }))))
  const offline = $derived(new Set(machines.filter(m => !m.ready).map(m => m.agentId)))

  function error(e: unknown) {
    if (e instanceof TerminalError && e.status === 401) { onsignedout(); return }
  }
  async function load() {
    const mine = ++generation
    try {
      const next = await listDrafts(signal())
      if (life.signal.aborted || mine !== generation) return
      drafts = next; problem = ''; pruneImages(next)
    } catch (e) {
      if (life.signal.aborted || mine !== generation) return
      error(e); problem = drafts ? "Couldn't refresh the backlog. Showing your last saved items." : "Couldn't read the backlog just now."
    }
  }
  async function showItems(stage: string, amount: number, opener: HTMLButtonElement) {
    const panel = opener.closest('.list-window')
    visible[stage] = amount
    await tick()
    if (!opener.isConnected) panel?.querySelector('button')?.focus()
  }
  function preferences() {
    try { localStorage.setItem(`reet.sessionBoard.${userId}`, JSON.stringify({ view, sort, compact })) } catch { /* The view still works without storage. */ }
  }
  const imageKey = (draft: Pick<SessionDraft, 'id' | 'version'>) => `${draft.id}:${draft.version}`
  // A draft that was edited or started no longer needs the images of its earlier versions.
  function pruneImages(drafts: SessionDraft[]) {
    const live = new Set(drafts.map(imageKey))
    const keep = <T,>(record: Record<string, T>) => Object.fromEntries(Object.entries(record).filter(([key]) => live.has(key)))
    imageCache = keep(imageCache); imageErrors = keep(imageErrors); imageLoading = keep(imageLoading)
  }
  function showImagePreview(draft: SessionDraft, el: HTMLElement, delay = true) {
    clearTimeout(previewShowTimer); clearTimeout(previewHideTimer)
    const open = () => { imagePreview = { draft, anchor: el }; void loadDraftImages(draft) }
    if (delay) previewShowTimer = setTimeout(open, 130); else open()
  }
  function keepImagePreview() { clearTimeout(previewHideTimer); clearTimeout(previewShowTimer) }
  function hideImagePreview(delay = 0) {
    clearTimeout(previewShowTimer); clearTimeout(previewHideTimer)
    if (delay) previewHideTimer = setTimeout(() => { imagePreview = null }, delay); else imagePreview = null
  }
  function loadDraftImages(draft: SessionDraft): Promise<DraftImage[] | undefined> {
    const key = imageKey(draft)
    if (imageCache[key]) return Promise.resolve(imageCache[key])
    const pending = imageRequests.get(key)
    if (pending) return pending
    const request = (async () => {
      imageLoading = { ...imageLoading, [key]: true }
      const errors = { ...imageErrors }; delete errors[key]; imageErrors = errors
      try {
        const full = await getDraft(draft.id, signal())
        if (life.signal.aborted) return undefined
        const images = full.images ?? []
        imageCache = { ...imageCache, [key]: images }
        return images
      } catch (e) {
        if (!life.signal.aborted) { error(e); imageErrors = { ...imageErrors, [key]: "Couldn't read those images." } }
        return undefined
      } finally {
        if (!life.signal.aborted) { const next = { ...imageLoading }; delete next[key]; imageLoading = next }
      }
    })()
    imageRequests.set(key, request)
    void request.finally(() => { imageRequests.delete(key) })
    return request
  }
  async function openImageLightbox(draft: SessionDraft, index = 0) {
    hideImagePreview()
    const images = imageCache[imageKey(draft)] ?? await loadDraftImages(draft)
    if (images?.length) lightbox = { items: images.map(image => ({ url: image.data, name: image.name })), index }
  }
  function setView(next: BoardView) { view = next; preferences() }
  function toggleDetails(key: string) {
    expanded = expanded.includes(key) ? expanded.filter(k => k !== key) : [...expanded, key]
  }
  function toggle(key: 'machines' | 'projects' | 'agents' | 'models', value: string) {
    filters = { ...filters, [key]: filters[key].includes(value) ? filters[key].filter(v => v !== value) : [...filters[key], value] }
  }
  function clear() { filters = emptyFilters() }
  // Reset from the scoped banner: back to every project and every session.
  function resetScope() { clear(); onclearscope?.() }
  function closeFilters(focus = false) { filtersOpen = false; if (focus) filterButton?.focus() }
  function outside(e: PointerEvent) {
    if (filtersOpen && !filterPanel?.contains(e.target as Node) && !filterButton?.contains(e.target as Node)) closeFilters()
  }
  function keydown(e: KeyboardEvent) { if (e.key === 'Escape' && imagePreview) { e.preventDefault(); hideImagePreview(); return } if (e.key === 'Escape' && filtersOpen) { e.preventDefault(); closeFilters(true) } }
  async function edit(draft: SessionDraft, opener: HTMLElement, start = false) {
    if (loadingId || working) return
    loadingId = draft.id; notice = ''
    try { const full = await getDraft(draft.id, signal()); if (!life.signal.aborted) composer = { draft: full, opener, start } }
    catch (e) { error(e); notice = e instanceof Error ? e.message : "Couldn't open that item." }
    finally { loadingId = '' }
  }
  function dragStart(event: DragEvent, draft: SessionDraft) {
    if (draft.starting || working || loadingId) { event.preventDefault(); return }
    dragDraft = draft.id; dragOpener = event.currentTarget as HTMLElement
    if (event.dataTransfer) { event.dataTransfer.setData('application/x-reet-backlog', draft.id); event.dataTransfer.effectAllowed = 'move' }
  }
  function dragEnd() { dragDraft = ''; dropActive = false }
  function activeOver(event: DragEvent) {
    if (!dragDraft) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    dropActive = true
  }
  function activeLeave(event: DragEvent) {
    if (!dragDraft) return
    if (!(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node | null)) dropActive = false
  }
  function activeDrop(event: DragEvent) {
    if (!dragDraft) return
    event.preventDefault()
    const id = event.dataTransfer?.getData('application/x-reet-backlog') || dragDraft
    const draft = (drafts ?? []).find(d => d.id === id)
    dragEnd()
    if (draft) dropToActive(draft)
  }
  // Dropping says what the card's Start button would do, in the same words, so the two routes cannot disagree.
  function dropToActive(draft: SessionDraft) {
    const intent = dropIntent(draft, startReason(draft))
    if (intent.kind === 'blocked') { notice = intent.reason; return }
    if (intent.kind === 'refine') { void edit(draft, dragOpener ?? document.body, true); return }
    baseOpener = dragOpener
    void activate(draft)
  }
  function closeComposer() { const opener = composer?.opener; composer = null; void tick().then(() => opener?.focus()) }
  function saved(draft: SessionDraft) {
    const start = composer?.start
    generation++; drafts = [...(drafts ?? []).filter(d => d.id !== draft.id), { ...draft, images: undefined }]; pruneImages(drafts)
    closeComposer()
    // A refined idea is now a normal backlog item, so it starts exactly like one.
    if (start) { void activate(draft); return }
    notice = 'Sorted. Session saved to your backlog.'; void load()
  }
  // What the machine and project lists say about where an item would run; the shared rules turn it into a reason.
  function startTarget(place: StartPlace): StartTarget {
    const machine = machines.find(m => m.agentId === place.agentId)
    return {
      readiness: readiness(machine),
      agent: machine && agentChoices(machine).find(c => c.agent.id === place.tool),
      uploads: !!machine?.capabilities?.uploads && !machine.capabilities.uploadsOff,
      effort: !!(place.chat ? machine?.capabilities?.acpStartOptions : machine?.capabilities?.agentsV2),
      projectCurrent: projects.some(p => p.id === place.projectId && p.host === place.agentId && p.dir === place.dir),
    }
  }
  const startCheck = (place: StartPlace) => startProblem(place, startTarget(place))
  // An idea is never blocked on the card: Start opens it to be refined, and the same rules check the refined item there.
  const startReason = (draft: SessionDraft) => draft.idea ? null : startCheck(draft)
  const yoloChoice = (d: SessionDraft) => ({ agent: d.tool, model: '', projectIds: d.projectId ? [d.projectId] : [], homeMachine: d.agentId, branch: '', worktree: d.worktree === true && !d.chat, effort: '' as const, prompt: '', approvals: 'yolo' as const })
  const yoloGateList = $derived(yoloAsk ? yoloGates(yoloChoice(yoloAsk), yoloSetting, projects, machines) : [])
  async function askYolo(item: SessionDraft) {
    yoloSetting = null; yoloAsk = item
    try { const next = await getYoloSetting(); if (!life.signal.aborted) yoloSetting = next } catch { /* The dialog keeps its reading line and Start stays off. */ }
  }
  function cancelYolo() { yoloAsk = null; notice = 'Not started. The item is still in your backlog.' }
  // The confirmation is for the version the dialog showed; a draft edited since is confirmed again.
  function confirmYolo() { const item = yoloAsk; yoloAsk = null; if (item) void activate(item, item.version) }
  function cancelBase() {
    baseAsk = null; notice = 'Not started. The item is still in your backlog.'
    void tick().then(() => { if (baseOpener?.isConnected) baseOpener.focus() })
  }
  // The yes is for the version that was shown; a plan edited since is asked about again.
  function confirmBase() { const ask = baseAsk; baseAsk = null; if (ask) void activate(ask.draft, ask.confirmed, ask.draft.version) }
  async function activate(item: SessionDraft, confirmedVersion = 0, basedVersion = 0) {
    if (activeRequests.has(item.id)) return
    const why = startReason(item); if (why) { notice = why; return }
    activeRequests.add(item.id); working = item.id; notice = ''; phase = 'Preparing…'
    try {
      const draft = await getDraft(item.id, signal())
      // Decide on the draft as the server has it now, not the board's last refresh. A frozen retry carries the request
      // already made, so it is not asked again.
      if (draft.yolo && !draft.preparedPrompt && draft.version !== confirmedVersion) { void askYolo(draft); return }
      if (!draft.preparedPrompt && draft.version !== basedVersion) {
        // Where it will start, before the images and before the controller is asked to start anything. With no name
        // for the machine the sentence says "that machine": an enrolment id would mean nothing to anyone.
        const host = machineNames.get(draft.agentId) || ''
        phase = 'Checking where this starts…'
        const say = await baseQuestion(draft.agentId, host, { tool: draft.tool, dir: draft.dir, base: draft.base },
          repoProjectAt(projects, draft.agentId, draft.dir), signal())
        if (life.signal.aborted) return
        if (say) { baseAsk = { draft, say, host, confirmed: confirmedVersion }; return }
      }
      let prompt = draft.preparedPrompt || draft.prompt
      if (!draft.preparedPrompt) {
        const refs: string[] = []
        for (const [index, image] of (draft.images ?? []).entries()) {
          phase = `Sending image ${index + 1} of ${draft.images!.length}…`
          const comma = image.data.indexOf(','); const type = image.data.slice(5, image.data.indexOf(';'))
          const bytes = Uint8Array.from(atob(image.data.slice(comma + 1)), c => c.charCodeAt(0))
          const blob = new Blob([bytes], { type })
          const uploaded = await launchUpload(draft.agentId, blob, signal())
          rememberUpload(blob, image.name, uploaded.path, uploaded.reference)
          refs.push(uploaded.reference)
        }
        prompt = imagePrompt(draft.prompt, refs)
        const invalid = validatePrompt(draft.tool, prompt); if (invalid) throw new Error(invalid)
      }
      phase = 'Starting agent…'
      const session = await startDraft(draft, prompt, signal())
      const notStarted = notStartedText(session.state)
      notice = notStarted || `Started “${draft.title}”. You’ll find it in Active.`
      await Promise.all([load(), onchanged()])
    } catch (e) {
      if (!life.signal.aborted) { error(e); notice = e instanceof Error ? e.message : 'No reply yet. Retry uses the same session request.'; await load() }
    } finally { activeRequests.delete(item.id); working = ''; phase = '' }
  }
  async function askDelete(draft: SessionDraft) { removing = draft; await tick(); deleteDialog?.showModal() }
  async function remove() {
    if (!removing || deleteBusy) return
    const draft = removing; deleteBusy = true
    try {
      await deleteDraft(draft, signal()); generation++; drafts = (drafts ?? []).filter(d => d.id !== draft.id); pruneImages(drafts)
      removing = null; notice = 'Backlog item deleted.'
    } catch (e) { error(e); notice = e instanceof Error ? e.message : "Couldn't delete that item."; removing = null; await load() }
    finally { deleteBusy = false }
  }
  onMount(() => {
    try {
      const p = JSON.parse(localStorage.getItem(`reet.sessionBoard.${userId}`) || '{}')
      if (['board', 'backlog', 'active', 'finished'].includes(p.view)) view = p.view
      if (['newest', 'oldest', 'title'].includes(p.sort)) sort = p.sort
      if (typeof p.compact === 'boolean') compact = p.compact
    } catch { /* Default to the full board. */ }
    void load()
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, 15_000)
    const refresh = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', refresh)
    return () => { life.abort(); clearInterval(timer); clearTimeout(previewShowTimer); clearTimeout(previewHideTimer); document.removeEventListener('visibilitychange', refresh) }
  })
</script>

<svelte:window onpointerdown={outside} onkeydown={keydown} />

<section class="session-board" class:embedded aria-labelledby={`${id}-heading`}>
  <header class="heading" class:embedded>
    <h2 class:vh={embedded} id={`${id}-heading`}>Your work, in good order.</h2>
    {#if !embedded}<button class="btn primary new-session" aria-label="New session" onclick={e => onstart(e.currentTarget)}><BoardIcon name="plus" />New session</button>{/if}
  </header>
  <div class="viewbar">
    <div class="views" role="group" aria-label="Session view">
      <button class:selected={view === 'board'} aria-pressed={view === 'board'} onclick={() => setView('board')}><BoardIcon name="board" />Board</button>
      <span class="divider"></span>
      {#each STAGES as stage}<button class:selected={view === stage.id} aria-pressed={view === stage.id} onclick={() => setView(stage.id)}><BoardIcon name={stage.id} /><span>{stage.label}</span><span class="view-count">{counts[stage.id] ?? 0}</span></button>{/each}
    </div>
    <div class="display"><label><span class="vh">Sort sessions</span><select bind:value={sort} onchange={preferences}><option value="newest">Recently updated</option><option value="oldest">Oldest first</option><option value="title">Title A–Z</option></select></label><button class="density" class:on={compact} aria-label="Compact cards" aria-pressed={compact} onclick={() => { compact = !compact; preferences() }}><span></span><span></span><span></span></button></div>
  </div>
  {#if scopeIds !== undefined}
    <div class="scope" role="status" aria-live="polite">
      <span class="scope-mark"><BoardIcon name={scopeKind === 'group' ? 'folder' : 'board'} size={15} /></span>
      <p><span class="scope-kind">{scopeKind === 'group' ? 'Group' : 'Project'}</span><strong>{scopeName}</strong><span class="scope-count">{scoped.length} {scoped.length === 1 ? 'session' : 'sessions'}</span></p>
      <button type="button" class="scope-reset" onclick={resetScope} title="Show every project and clear filters">
        <BoardIcon name="close" size={12} />Show everything
      </button>
    </div>
  {/if}
  <div class="filterbar">
    <label class="search"><BoardIcon name="search" /><input type="search" bind:value={filters.query} placeholder="Find a session, prompt, folder…" aria-label="Search sessions" />{#if filters.query}<button aria-label="Clear search" onclick={() => filters.query = ''}><BoardIcon name="close" size={13}/></button>{/if}</label>
    <div class="filter-anchor">
      <button class="filter-button" class:applied={count > 0} bind:this={filterButton} aria-expanded={filtersOpen} aria-controls={`${id}-filters`} onclick={() => filtersOpen = !filtersOpen}><BoardIcon name="filter" />Filters{#if count}<span>{count}</span>{/if}</button>
      {#if filtersOpen}
        <div class="filter-panel" id={`${id}-filters`} bind:this={filterPanel}>
          <div class="filter-head"><strong>Make it your workspace</strong><button aria-label="Close filters" onclick={() => closeFilters(true)}><BoardIcon name="close" /></button></div>
          <p>Choose several in a group. Combine groups to narrow down.</p>
          <div class="filter-groups">{#each filterGroups as group}<fieldset><legend>{group.label}</legend>{#if !group.options.length}<span class="no-options">No sessions yet</span>{/if}{#each group.options as option}<label><input type="checkbox" aria-label={option.label} checked={filters[group.key].includes(option.value)} onchange={() => toggle(group.key, option.value)} /><span>{option.label}</span><small aria-hidden="true">{all.filter(item => (group.key === 'machines' ? item.agentId : group.key === 'projects' ? item.projectId || 'none' : group.key === 'agents' ? item.tool || 'shell' : item.model || 'default') === option.value).length}</small></label>{/each}</fieldset>{/each}</div>
          <div class="extra-filters"><label><input type="checkbox" bind:checked={filters.attention} />Needs attention</label><label><input type="checkbox" bind:checked={filters.images} />Has reference images</label></div>
          <div class="filter-foot"><button onclick={clear}>Reset filters</button><button class="btn primary" onclick={() => closeFilters(true)}>Show {filtered.length} sessions</button></div>
        </div>
      {/if}
    </div>
    <span class="result-count">{hasFilters || scopeIds !== undefined ? `${filtered.length} of ${scoped.length}` : all.length} sessions</span>
  </div>
  {#if hasFilters}<div class="chips">{#each chips as chip}<button onclick={() => toggle(chip.key, chip.value)}>{chip.label}<BoardIcon name="close" size={11} /></button>{/each}{#if filters.attention}<button onclick={() => filters.attention = false}>Needs attention<BoardIcon name="close" size={11}/></button>{/if}{#if filters.images}<button onclick={() => filters.images = false}>Has images<BoardIcon name="close" size={11}/></button>{/if}<button class="clear" onclick={clear}>Clear all</button></div>{/if}
  <div class="live" role="status">{#if notice}<p class="notice">{notice}</p>{/if}</div>
  {#if problem}<p class="problem" role="alert">{problem} <button onclick={() => void load()}>Try again</button></p>{/if}
  <div class="board" class:focused={view !== 'board'} class:compact>
    {#each columns as stage}
      {@const items = filtered.filter(item => item.stage === stage.id)}
      <section class="stage {stage.id}" class:drop-target={stage.id === 'active' && !!dragDraft} class:drop-ready={stage.id === 'active' && dropActive} aria-labelledby={`${id}-${stage.id}`} ondragover={stage.id === 'active' ? activeOver : undefined} ondragleave={stage.id === 'active' ? activeLeave : undefined} ondrop={stage.id === 'active' ? activeDrop : undefined}>
        <header class="stage-head"><span class="stage-mark"><BoardIcon name={stage.id} size={15}/></span><h3 id={`${id}-${stage.id}`}>{stage.label}</h3><span class="stage-count">{items.length}</span>{#if stage.id === 'backlog'}<button aria-label="Add session to backlog" onclick={e => composer = { draft: null, opener: e.currentTarget }}><BoardIcon name="plus" /></button>{:else if stage.id === 'active'}<button aria-label="Start a session" title="Start a session" onclick={e => onstart(e.currentTarget)}><BoardIcon name="plus" /></button>{/if}</header>
        <p class="stage-hint">{stage.id === 'active' && dropActive ? 'Drop to start it on its machine.' : stage.hint}</p>
        <div class="cards" aria-busy={stage.id === 'backlog' && drafts === undefined && !problem || stage.id !== 'backlog' && sessions === undefined && !sessionsFailed}>
          {#if stage.id !== 'backlog' && sessions === undefined && sessionsFailed}
            <div class="empty"><span class="empty-icon"><BoardIcon name="clock" size={24}/></span><strong>Couldn’t read your sessions.</strong><p>Try again to see what’s active and finished.</p><button onclick={() => void onchanged()}>Try again</button></div>
          {:else if stage.id === 'backlog' && drafts === undefined && !problem || stage.id !== 'backlog' && sessions === undefined}
            <div class="empty"><span class="empty-icon"><BoardIcon name="clock" size={24}/></span><strong>Reading your sessions…</strong></div>
          {:else if !items.length}
            <div class="empty"><span class="empty-icon"><BoardIcon name={hasFilters ? 'search' : stage.id} size={26}/></span><strong>{hasFilters ? 'Nowt matches just yet.' : scopeIds !== undefined ? `Nothing in ${scopeName} here yet.` : stage.id === 'backlog' ? 'Make room for your next idea.' : stage.id === 'active' ? 'Ready when you are.' : 'Good work will land here.'}</strong><p>{hasFilters ? 'Try another search or loosen your filters.' : scopeIds !== undefined ? 'Choose Show everything to see every project again.' : stage.id === 'backlog' ? 'Save a clear prompt, pick an agent, and leave it ready for later.' : stage.id === 'active' ? 'Start one now, or from your backlog.' : 'Sessions move here when they end, with their history close at hand.'}</p>{#if hasFilters}<button onclick={clear}>Clear filters</button>{:else if scopeIds !== undefined}<button onclick={resetScope}>Show everything</button>{:else if stage.id === 'backlog'}<button onclick={e => composer = { draft: null, opener: e.currentTarget }}><BoardIcon name="plus" size={14}/>Plan a session</button>{:else if stage.id === 'active'}<button onclick={e => onstart(e.currentTarget)}><BoardIcon name="plus" size={14}/>Start a session</button>{/if}</div>
          {:else}
            {#each items.slice(0, visible[stage.id]) as item, index (item.key)}
              {@const draft = item.draft}
              {@const session = item.session}
              {@const project = projects.find(p => p.id === item.projectId)}
              {@const word = session ? stateWord(session) : null}
              {@const reason = draft ? startReason(draft) : null}
              {@const detailText = draft ? draft.prompt || (draft.idea ? 'Just an idea so far.' : '') : session?.promptExcerpt || ''}
              {@const hasDetails = true}
              {@const detailOpen = expanded.includes(item.key)}
              {@const detailId = `${id}-${stage.id}-${index}-details`}
              <article class="session-card" class:expanded={detailOpen} class:retrying={draft?.starting} class:attention={session?.attention === 'waiting'} class:starting={working === draft?.id} class:dragging={!!draft && dragDraft === draft.id} data-status={session ? sessionStatus(session) : undefined} draggable={draft ? !draft.starting && !working && !loadingId : undefined} ondragstart={draft ? event => dragStart(event, draft) : undefined} ondragend={draft ? dragEnd : undefined}>
                <div class="card-top"><span class="project-label" title={project?.name ?? item.dir}><BoardIcon name="folder" size={13}/>{project?.name || (item.projectId ? 'Removed project' : item.dir.split('/').filter(Boolean).at(-1) || 'Home folder')}</span>{#if draft?.idea}<span class="idea-badge">Idea</span>{/if}{#if session && factory.get(session.id)}{@const member = factory.get(session.id)!}<a class="factory-badge" href={member.key ? `/factory#pipeline-${member.outcomeId}-${member.key}` : '/factory'} title={`Part of a Factory pipeline: ${member.pipeline}. Outcome: ${member.objective}`}>{factoryBadge(member)}</a>{/if}{#if draft?.imageCount}<button type="button" class="image-count" aria-label={`Preview ${draft.imageCount} reference ${draft.imageCount === 1 ? 'image' : 'images'}`} aria-haspopup="dialog" onpointerenter={event => showImagePreview(draft, event.currentTarget)} onpointerleave={() => hideImagePreview(160)} onfocus={event => showImagePreview(draft, event.currentTarget, false)} onblur={() => hideImagePreview()} onclick={() => void openImageLightbox(draft)}><BoardIcon name="image" size={13}/>{draft.imageCount}</button>{/if}</div><div class="card-tools">{#if hasDetails}<button type="button" class="disclosure" aria-expanded={detailOpen} aria-controls={detailId} aria-label={`${detailOpen ? 'Hide' : 'Show'} details for ${item.title}`} title={detailOpen ? 'Hide details' : 'Show details'} onclick={() => toggleDetails(item.key)}><span class="disclosure-text" aria-hidden="true">{detailOpen ? 'Hide' : 'Details'}</span><BoardIcon name="chevron" size={13}/></button>{/if}{#if session}<button class="more" aria-label={`Actions for ${item.title}`} onclick={e => void actions?.openMenu(e, session)}><BoardIcon name="more"/></button>{/if}</div>
                <h4>{#if session}<a href={sessionHref(session)}>{item.title}</a>{:else}<button onclick={e => draft && void edit(draft, e.currentTarget)} disabled={!!loadingId || !!draft?.starting}>{item.title}</button>{/if}</h4>
                {#if hasDetails}<div class="details" class:open={detailOpen} id={detailId} inert={!detailOpen}><div class="details-inner">{#if draft}<p class="prompt-preview">{detailText}</p>{:else}<p class="prompt-preview">{detailText}{session?.promptMore ? '…' : ''}</p>{#if session?.worktreeBranch}<p class="branch" title={session.worktreeBranch}>↳ {session.worktreeBranch}</p>{/if}{/if}</div></div>{/if}
                <div class="item-metadata" inert={compact && !detailOpen}><div class="agent-meta"><span class="agent-icon">{item.tool ? toolName(item.tool).slice(0, 1) : '›'}</span><strong>{item.tool ? toolName(item.tool) : 'Shell'}</strong><span class="model" title={item.model || 'Agent default'}>{item.model || 'Agent default'}</span>{#if item.effort}<span class="effort">{item.effort}</span>{/if}</div>
                <div class="machine-line"><BoardIcon name="machine" size={13}/><span>{machineNames.get(item.agentId) || health.find(m => m.agentId === item.agentId)?.host || item.agentId}</span>{#if offline.has(item.agentId)}<span class="offline-label">Offline{session ? ' · last known' : ''}</span>{/if}</div>
                </div>
                <footer class="card-foot">
                  {#if draft}
                    <span class="age" title={new Date(item.createdAt).toLocaleString()}>Saved {shortAge(Math.max(0, now - item.createdAt))} ago</span>
                    <div class="card-actions">{#if !draft.starting}<button class="edit" disabled={!!working || !!loadingId} onclick={e => void edit(draft, e.currentTarget)} aria-label={`Edit ${draft.title}`}>Edit</button><button class="delete" disabled={!!working || !!loadingId} aria-label={`Delete ${draft.title}`} onclick={() => void askDelete(draft)}>Delete</button>{/if}<button class="start" disabled={!!reason || !!working} title={reason ?? (draft.idea ? 'Refine this idea, then start it' : 'Start this session on its machine')} onclick={e => { baseOpener = e.currentTarget; if (draft.idea) void edit(draft, e.currentTarget, true); else void activate(draft) }}>{working === draft.id ? phase : draft.starting ? 'Retry start' : 'Start'}<BoardIcon name="arrow" size={13}/></button></div>
                  {:else if session && word}
                    <span class="status" title={word.source}>{#if sessionStatus(session) === 'finished'}<span class="status-tick" aria-hidden="true"></span>{:else}<i></i>{/if}{word.word}{offline.has(item.agentId) ? ' · last known' : ''}</span><span class="age session-age" title={new Date(item.updatedAt).toLocaleString()}>{shortAge(Math.max(0, now - item.updatedAt))} ago</span><a class="open-session" href={sessionHref(session)}>{stage.id === 'finished' ? 'Review' : 'Open'}<BoardIcon name="arrow" size={13}/></a>
                  {/if}
                </footer>
                {#if draft && reason}<p class="start-note">{reason}</p>{:else if draft?.starting}<p class="start-note">Start was requested. Retry checks the same request.</p>{/if}
              </article>
            {/each}
          {/if}
        </div>
        {#if items.length > PAGE_SIZE}<div class="list-window">
          <span>Showing {Math.min(visible[stage.id], items.length)} of {items.length}</span>
          {#if visible[stage.id] < items.length}<button type="button" onclick={event => void showItems(stage.id, visible[stage.id] + PAGE_SIZE, event.currentTarget)}>Show {Math.min(PAGE_SIZE, items.length - visible[stage.id])} more</button><button type="button" onclick={event => void showItems(stage.id, items.length, event.currentTarget)}>View all {items.length}</button>
          {:else}<button type="button" onclick={async event => { const section = event.currentTarget.closest('.stage'); visible[stage.id] = PAGE_SIZE; expanded = []; await tick(); section?.querySelector<HTMLButtonElement>('.list-window button')?.focus() }}>Show fewer</button>{/if}
        </div>{/if}
      </section>
    {/each}
  </div>
  <div class="board-foot"><span><BoardIcon name="finished" size={13}/>Saved to your account. Ready across your machines.</span><a href="/sessions">All session history<BoardIcon name="arrow" size={13}/></a></div>
</section>
{#if baseAsk}<BaseConfirm sentence={baseAsk.say} host={baseAsk.host} oncancel={cancelBase} onstart={confirmBase} />{/if}
{#if yoloAsk}<YoloDialog gates={yoloGateList} tool={yoloAsk.tool} structured={yoloAsk.chat === true} machine={machineNames.get(yoloAsk.agentId) || 'the machine'} oncancel={cancelYolo} onstart={confirmYolo} />{/if}
{#if composer}<SessionComposer draft={composer.draft} startNow={composer.start === true} {startCheck} {projects} {machines} {profile} {initialProject} onsaved={saved} onclose={closeComposer} onerror={error}/>{/if}
{#if removing}<dialog class="delete-dialog" bind:this={deleteDialog} aria-labelledby={`${id}-delete`} oncancel={e => { if (deleteBusy) e.preventDefault(); else removing = null }}><h2 id={`${id}-delete`}>Delete this backlog item?</h2><p>“{removing.title}” and its saved images will be removed.</p><div><button class="btn" disabled={deleteBusy} onclick={() => removing = null}>Keep session</button><button class="btn primary" disabled={deleteBusy} aria-busy={deleteBusy} onclick={() => void remove()}>{deleteBusy ? 'Deleting…' : 'Delete item'}</button></div></dialog>{/if}
{#if imagePreview}
  {@const previewDraft = imagePreview.draft}
  {#key previewDraft.id}
    <DraftImagePreview images={imageCache[imageKey(previewDraft)] ?? []} count={previewDraft.imageCount} loading={!!imageLoading[imageKey(previewDraft)]} error={imageErrors[imageKey(previewDraft)] ?? ''} anchor={imagePreview.anchor} onenter={keepImagePreview} onleave={() => hideImagePreview(160)} />
  {/key}
{/if}
{#if lightbox}<ImageLightbox items={lightbox.items} index={lightbox.index} onclose={() => lightbox = null} />{/if}
<SessionActions bind:this={actions} machines={health} {offline} terminals={machines} {removeDefault} {onsignedout} {onchanged} onnotice={n => notice = n?.text ?? ''}/>

<style>
  .heading.embedded { min-height: 0; margin: 0; padding: 0; }

  .session-board { min-width: 0; }
  .heading { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin: 0 0 16px; }
  h2 { font: 500 clamp(22px, 2vw, 28px)/1.15 var(--serif); letter-spacing: -.035em; margin: 0; }
  .new-session { flex-shrink: 0; font-size: 14px; box-shadow: 0 2px 4px color-mix(in srgb, var(--ink) 6%, transparent); }
  .viewbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; border-bottom: 1px solid var(--line); padding-bottom: 12px; }
  .views { display: flex; align-items: center; gap: 4px; }
  .views button { display: flex; align-items: center; gap: 8px; border: 1px solid transparent; background: transparent; color: var(--muted); border-radius: 8px; padding: 8px 12px; cursor: pointer; font: 500 14px var(--sans); min-height: 36px; }
  .views button.selected { color: var(--ink); background: var(--card); border-color: var(--line); box-shadow: 0 1px 3px color-mix(in srgb, var(--ink) 3%, transparent); }
  .views button.selected :global(svg) { color: var(--accent-text); }
  .views button:hover { background: var(--card); }.view-count { font-size: 14px; font-variant-numeric: tabular-nums; background: var(--bg-sunk); color: var(--muted); min-width: 18px; text-align: center; border-radius: 4px; padding: 1px 4px; }
  .divider { height: 16px; width: 1px; background: var(--line); margin: 0 6px; }
  .display { display: flex; align-items: center; gap: 8px; }
  .display select { background: transparent; min-height: 34px; padding: 4px 20px 4px 8px; border-color: transparent; color: var(--muted); font-size: 14px; cursor: pointer; }
  .density { display: flex; flex-direction: column; justify-content: center; gap: 3px; width: 32px; height: 32px; border: 1px solid var(--line); background: transparent; border-radius: 6px; cursor: pointer; }
  .density span { height: 1px; width: 14px; margin: 0 auto; background: var(--muted); }.density.on { background: var(--card); }
  .scope { display: flex; align-items: center; gap: 12px; margin-top: 12px; padding: 8px 12px; border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line)); border-radius: 8px; background: var(--accent-wash); }
  .scope-mark { flex: none; display: grid; place-items: center; width: 28px; height: 28px; border-radius: 8px; background: var(--card); color: var(--accent-text); box-shadow: 0 1px 2px color-mix(in srgb, var(--ink) 7%, transparent); }
  .scope p { display: flex; align-items: baseline; gap: 8px; min-width: 0; margin: 0; font-size: 14px; }
  .scope-kind { flex: none; font-size: 14px; font-weight: 600; color: var(--accent-text); }
  .scope strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
  .scope-count { flex: none; color: var(--muted); font-variant-numeric: tabular-nums; }
  .scope-reset { display: flex; align-items: center; gap: 6px; margin-left: auto; flex: none; min-height: 30px; padding: 4px 8px; border: 1px solid var(--line-mid); border-radius: 6px; background: var(--card); color: var(--ink-soft); font: 500 14px var(--sans); cursor: pointer; }
  .scope-reset:hover { border-color: var(--muted); color: var(--ink); }
  .filterbar { display: flex; align-items: center; gap: 8px; padding: 12px 0; }
  .search { display: flex; align-items: center; gap: 8px; color: var(--muted); flex: 1; max-width: 360px; border: 1px solid var(--line); background: var(--card); border-radius: 8px; padding: 0 12px; }
  .search input { border: 0; min-height: 35px; padding: 8px 0; width: 100%; outline: 0; font-size: 14px; background: transparent; }
  .search:focus-within { outline: 2px solid var(--accent); outline-offset: 2px; }.search button, .filter-head button { border: 0; padding: 4px; background: transparent; color: var(--muted); cursor: pointer; }
  .search input::-webkit-search-cancel-button { display: none; }
  .filter-anchor { position: relative; }.filter-button { display: flex; align-items: center; gap: 8px; min-height: 36px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--muted); font: 500 14px var(--sans); padding: 6px 12px; cursor: pointer; }
  .filter-button.applied { border-color: var(--accent-text); color: var(--accent-text); }.filter-button > span { background: var(--accent-wash); padding: 0 6px; border-radius: 4px; font-variant-numeric: tabular-nums; }
  .result-count { margin-left: auto; font-size: 14px; font-variant-numeric: tabular-nums; color: var(--muted); white-space: nowrap; }
  .filter-panel { position: absolute; top: calc(100% + 8px); right: 0; width: min(540px, calc(100vw - 48px)); border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card); box-shadow: 0 12px 50px color-mix(in srgb, var(--ink) 12%, transparent); padding: 16px; z-index: 25; max-height: min(580px, 72dvh); overflow-y: auto; }
  .filter-head { display: flex; align-items: center; justify-content: space-between; font-size: 14px; }.filter-panel > p { font-size: 14px; color: var(--muted); margin: 4px 0 16px; }
  .filter-groups { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }.filter-groups fieldset { border: 0; padding: 0; margin: 0; min-width: 0; max-height: 165px; overflow-y: auto; }
  legend { font-size: 14px; font-weight: 600; color: var(--muted); padding: 0; margin-bottom: 8px; }
  .filter-groups label { display: flex; align-items: center; gap: 8px; font-size: 14px; padding: 4px 0; cursor: pointer; }.filter-groups label span { overflow-wrap: anywhere; }.filter-groups small { margin-left: auto; color: var(--muted); font-size: 14px; font-variant-numeric: tabular-nums; }
  .extra-filters { display: flex; gap: 16px; flex-wrap: wrap; border-top: 1px solid var(--line); margin-top: 16px; padding-top: 16px; font-size: 14px; }.extra-filters label { display: flex; align-items: center; gap: 8px; }
  .filter-foot { display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); margin-top: 16px; padding-top: 16px; }.filter-foot > button:first-child { border: 0; background: none; font-size: 14px; color: var(--muted); cursor: pointer; }.filter-foot .btn { font-size: 14px; }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; padding: 0 0 16px; }.chips button { display: flex; align-items: center; gap: 8px; font-size: 14px; color: var(--ink-soft); background: var(--card); border: 1px solid var(--line); padding: 4px 8px; border-radius: 6px; cursor: pointer; }.chips .clear { background: none; border-color: transparent; color: var(--muted); }
  .board { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; align-items: start; }
  .board.focused { grid-template-columns: minmax(0, 1fr); }.stage { min-width: 0; border: 1px solid var(--line); border-radius: 12px; background: var(--bg-sunk); padding: 16px 12px 12px; min-height: 345px; }
  .stage.drop-target { border-style: dashed; border-color: color-mix(in srgb, var(--accent) 45%, var(--line)); }
  .stage.drop-ready { border-style: solid; border-color: var(--accent); background: var(--accent-wash); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 14%, transparent); }
  .stage.drop-ready .stage-hint { color: var(--accent-text); font-weight: 550; }
  .stage-head { display: flex; align-items: center; gap: 8px; padding: 0 4px; }.stage-mark { color: var(--muted); display: flex; }.active .stage-mark { color: var(--accent-text); }.finished .stage-mark { color: var(--good); }
  h3 { font-size: 14px; font-weight: 600; margin: 0; }.stage-count { font-size: 14px; font-variant-numeric: tabular-nums; color: var(--muted); border: 1px solid var(--line); border-radius: 4px; padding: 1px 6px; min-width: 18px; text-align: center; box-sizing: border-box; }
  .stage-head > button { margin-left: auto; border: 0; background: transparent; color: var(--muted); display: flex; align-items: center; justify-content: center; cursor: pointer; width: 24px; height: 24px; border-radius: 4px; }.stage-head > button:hover { background: var(--card); }
  .stage-hint { font-size: 14px; color: var(--muted); margin: 6px 4px 16px; }
  .cards { display: grid; gap: 8px; }.focused .cards { grid-template-columns: repeat(auto-fill, minmax(290px, 1fr)); }
  .session-card { position: relative; background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 12px; box-shadow: 0 2px 3px color-mix(in srgb, var(--ink) 1%, transparent); min-width: 0; transition: border-color .16s, box-shadow .16s; }.session-card:hover { border-color: var(--line-strong); box-shadow: 0 3px 9px color-mix(in srgb, var(--ink) 3%, transparent); }
  .session-card[draggable='true'] { cursor: grab; }.session-card.dragging { opacity: .55; cursor: grabbing; }
  .session-card[data-status] { box-shadow: inset 3px 0 var(--status), 0 2px 3px color-mix(in srgb, var(--ink) 1%, transparent); }
  .session-card[data-status='working'], .session-card[data-status='waiting'], .session-card[data-status='starting'], .session-card[data-status='failed'] { background: var(--card); border-color: var(--line); }.session-card.attention { border-color: var(--line); }.session-card.starting { border-color: var(--accent-text); }
  .card-top { display: flex; align-items: center; gap: 8px; min-height: 44px; color: var(--muted); font-size: 14px; }.project-label { display: flex; align-items: center; gap: 6px; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.project-label :global(svg) { flex-shrink: 0; }.image-count { display: flex; align-items: center; gap: 4px; flex: none; min-height: 22px; padding: 1px 6px; margin: -1px -3px; border: 1px solid transparent; border-radius: 6px; background: transparent; color: var(--muted); font: inherit; font-variant-numeric: tabular-nums; cursor: pointer; transition: color .15s, background-color .15s, border-color .15s; }.image-count:hover, .image-count:focus-visible { color: var(--ink); background: var(--bg-sunk); border-color: var(--line); }.image-count :global(svg) { flex: none; }
  .card-top { padding-right: 88px; }
  .card-tools { position: absolute; right: 8px; top: 8px; display: flex; align-items: center; gap: 1px; margin-left: auto; }
  .disclosure { display: inline-flex; align-items: center; gap: 4px; border: 0; background: transparent; color: var(--muted); margin: -4px -3px -4px 0; padding: 4px 6px; cursor: pointer; border-radius: 4px; font: 500 14px var(--sans); min-height: 24px; }.disclosure:hover { background: var(--bg-sunk); color: var(--ink-soft); }.disclosure:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }.disclosure :global(svg) { transition: transform .18s ease; }.disclosure[aria-expanded='true'] :global(svg) { transform: rotate(180deg); }
  .more { border: 0; background: transparent; color: var(--muted); margin: -4px -5px -4px 0; padding: 4px; cursor: pointer; display: flex; border-radius: 4px; }.more:hover { background: var(--bg-sunk); }
  .details { display: grid; grid-template-rows: 0fr; opacity: 0; transition: grid-template-rows .2s ease, opacity .16s ease; }.details.open { grid-template-rows: 1fr; opacity: 1; }.details-inner { min-height: 0; overflow: hidden; }
  h4 { font: 600 14px/1.45 var(--sans); margin: 8px 0; overflow-wrap: anywhere; }h4 a, h4 button { color: var(--ink); text-decoration: none; }h4 button { border: 0; padding: 0; background: none; text-align: left; font: inherit; cursor: pointer; }h4 a:hover, h4 button:hover { color: var(--accent-text); }h4 button:disabled { cursor: default; opacity: 1; }
  .prompt-preview { display: -webkit-box; -webkit-line-clamp: 6; line-clamp: 6; -webkit-box-orient: vertical; overflow: hidden; font-size: 14px; color: var(--muted); margin: 0 0 12px; overflow-wrap: anywhere; white-space: pre-line; }
  .agent-meta { display: flex; align-items: center; gap: 6px; font-size: 14px; min-width: 0; }.agent-icon { background: var(--bg-sunk); border: 1px solid var(--line); border-radius: 4px; width: 18px; height: 18px; display: inline-flex; align-items: center; justify-content: center; font-size: 14px; font-weight: 600; flex-shrink: 0; }.agent-meta strong { font-weight: 550; white-space: nowrap; }.model { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding-left: 6px; margin-left: 2px; border-left: 1px solid var(--line); }.effort { color: var(--muted); background: var(--bg-sunk); border-radius: 4px; padding: 1px 4px; margin-left: auto; }
  .machine-line { display: flex; align-items: center; gap: 6px; color: var(--muted); font-size: 14px; margin: 8px 0 12px; min-width: 0; }.machine-line > span:first-of-type { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.offline-label { font-size: 14px; color: var(--warn); margin-left: auto; flex-shrink: 0; }
  .card-foot { display: flex; align-items: center; gap: 6px; border-top: 1px solid var(--line-soft); padding-top: 8px; justify-content: space-between; }.age { color: var(--muted); font-size: 14px; }.card-actions { display: flex; align-items: center; gap: 8px; margin-left: auto; }
  .card-actions button { font: 500 14px var(--sans); padding: 4px 0; background: none; border: 0; color: var(--muted); cursor: pointer; }.card-actions .delete { font-size: 14px; }.card-actions .delete:hover { color: var(--bad); }.card-actions .start { display: flex; align-items: center; gap: 6px; background: var(--accent-wash); color: var(--accent-text); border-radius: 6px; padding: 4px 8px; }.card-actions .start:disabled { color: var(--muted); background: var(--bg-sunk); cursor: default; }.card-actions .start:not(:disabled):hover { background: var(--tint); }
  .status { display: inline-flex; align-items: center; gap: 6px; font-size: 14px; font-weight: 600; color: var(--status); border: 1px solid var(--status-edge); background: var(--status-wash); border-radius: 6px; padding: 2px 8px; }.status i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
  .open-session { display: flex; align-items: center; gap: 6px; font-size: 14px; font-weight: 500; text-decoration: none; color: var(--muted); }.open-session:hover { color: var(--accent-text); }
  .factory-badge { font-size: 14px; font-weight: 600; color: var(--accent-text); border: 1px solid var(--accent); border-radius: 6px; padding: 1px 8px; text-decoration: none; white-space: nowrap; }
  .factory-badge:hover { background: var(--tint); }
  .factory-badge:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .idea-badge { font-size: 14px; font-weight: 600; color: var(--accent-text); border: 1px solid var(--line-strong); border-radius: 6px; padding: 1px 8px; }
  .start-note { font-size: 14px; color: var(--warn); border-top: 1px solid var(--line-soft); margin: 8px 0 0; padding-top: 8px; line-height: 1.5; }.branch { font: 14px/1.5 var(--mono); color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin: 0 0 12px; }
  .compact .stage-hint { display: none; }
  .compact .stage-head { margin-bottom: 8px; }
  .compact .cards, .compact.focused .cards { grid-template-columns: minmax(0, 1fr); gap: 0; }
  .compact .session-card { position: relative; padding: 8px 10px; border-width: 0 0 1px; border-radius: 0; background: transparent; }
  .compact .session-card:not(.expanded) { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; column-gap: 8px; }
  .compact .session-card:not(.expanded) h4 { grid-column: 1 / 3; grid-row: 1; padding-right: 0; margin: 0; min-height: 24px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .compact .session-card:not(.expanded) h4 a, .compact .session-card:not(.expanded) h4 button { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .compact .session-card:not(.expanded) .card-top { grid-column: 1; grid-row: 2; min-height: 24px; padding-right: 0; }
  .compact .session-card:not(.expanded) .card-tools { position: static; grid-column: 3; grid-row: 1 / 3; align-self: center; }
  .compact .session-card:not(.expanded) .card-foot { grid-column: 2; grid-row: 2; flex-wrap: wrap; max-width: 150px; border: 0; padding: 0; gap: 8px; }
  .compact .session-card:not(.expanded) .item-metadata, .compact .session-card:not(.expanded) .details { display: none; }
  .compact .session-card:not(.expanded) .card-actions .edit, .compact .session-card:not(.expanded) .card-actions .delete, .compact .session-card:not(.expanded) .open-session { display: none; }
  .compact .disclosure-text { display: none; }
  .compact .disclosure, .compact .more { margin: 0; min-width: 44px; min-height: 44px; justify-content: center; }
  .compact .session-card.expanded { border: 1px solid var(--line); border-radius: 6px; margin-block: 4px; }
  .compact .session-card:not(.expanded):has(.card-actions) .card-top { padding-right: 0; }
  .session-age { white-space: nowrap; }
  .list-window { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; padding: 10px 4px 0; font-size: 14px; color: var(--muted); }
  .list-window button { min-height: 44px; padding: 4px 0; border: 0; background: transparent; color: var(--ink); text-decoration: underline; cursor: pointer; font: inherit; }
  .list-window button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .compact .card-actions button { min-height: 44px; }
  .compact .session-card:not(.expanded):not(.retrying) .card-actions { display: none; }
  .compact .session-card:not(.expanded).retrying .card-foot { grid-row: 1 / 3; }
  .compact .session-card:not(.expanded).retrying h4 { grid-column: 1; }
  .compact .session-card:not(.expanded).retrying .card-foot > .age { display: none; }
  .compact .session-card:not(.expanded) .more { display: none; }
  .compact .session-card.expanded .card-top { padding-right: 88px; }
  .compact .session-card.expanded .card-tools { top: 4px; right: 4px; }
  .compact .session-card:not(.expanded) .status { font-size: 14px; }
  .compact .session-card:not(.expanded) .session-age { font-size: 14px; }
  .empty { text-align: center; padding: 36px 12px; color: var(--muted); grid-column: 1/-1; }.empty-icon { width: 48px; height: 48px; display: flex; align-items: center; justify-content: center; background: var(--card); border: 1px solid var(--line); border-radius: 12px; margin: 0 auto 16px; }.empty strong { font-size: 14px; color: var(--ink-soft); font-weight: 550; display: block; }.empty p { font-size: 14px; margin: 8px auto 16px; max-width: 240px; }.empty button { display: inline-flex; align-items: center; gap: 6px; color: var(--accent-text); font-size: 14px; border: 0; background: transparent; cursor: pointer; }
  .board-foot { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: 16px; font-size: 14px; color: var(--muted); }.board-foot span, .board-foot a { display: flex; gap: 6px; align-items: center; }.board-foot a { text-decoration: none; color: var(--muted); }.board-foot a:hover { color: var(--accent-text); }
  .live:empty { display: none; }.notice, .problem { padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); margin: 0 0 16px; font-size: 14px; }.problem { border-color: var(--warn); }.problem button { border: 0; background: none; color: var(--accent-text); cursor: pointer; text-decoration: underline; }.no-options { font-size: 14px; color: var(--muted); }
  .delete-dialog { padding: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--ink); max-width: min(430px, calc(100vw - 48px)); }.delete-dialog h2 { font-size: 24px; }.delete-dialog p { font-size: 14px; margin: 16px 0 24px; overflow-wrap: anywhere; }.delete-dialog > div { display: flex; justify-content: flex-end; gap: 8px; }.delete-dialog::backdrop { background: var(--scrim); }
  @media (min-width: 1800px) { .session-card { padding: 16px; }h4 { font-size: 15px; }.prompt-preview { font-size: 14px; }.agent-meta, .machine-line, .age, .status { font-size: 14px; } }
  @media (max-width: 1400px) { .board { gap: 12px; }.stage { padding: 12px 8px 8px; }.session-card { padding: 12px; }.card-foot { flex-wrap: wrap; }.card-actions { gap: 8px; } }
  @media (max-width: 850px) { .board { grid-template-columns: minmax(0, 1fr); gap: 16px; }.cards { grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); }.stage { min-height: 0; }.stage-hint { font-size: 14px; }.stage-head h3 { font-size: 14px; }.session-card h4 { font-size: 14px; }.agent-meta, .machine-line, .card-actions button, .status, .open-session, .age { font-size: 14px; }.card-actions button, .open-session { min-height: 30px; }.filter-panel { right: 0; width: min(480px, calc(100vw - 54px)); }.display select { max-width: 120px; }.viewbar { flex-wrap: wrap; }.views { flex: 1; }.display { margin-left: auto; } }
  .embedded .stage:not(.drop-ready) { background: transparent; border-radius: 8px; }
  .embedded .session-card { box-shadow: none; }
  .embedded .session-card[data-status='waiting'], .embedded .session-card[data-status='failed'] { box-shadow: inset 2px 0 var(--status); }
  .embedded .status { padding: 2px 0; border: 0; background: transparent; }
  .embedded .stage-count, .embedded .view-count { border: 0; background: transparent; }
  @media (max-width: 650px) { .disclosure-text { display: none; }.disclosure { margin-right: -2px; }.scope { flex-wrap: wrap; padding: 8px 12px; } .scope p { flex-wrap: wrap; row-gap: 2px; } .scope-reset { margin-left: 0; width: 100%; justify-content: center; min-height: 40px; } .heading { margin: 0 0 16px; gap: 12px; align-items: center; }h2 { font-size: 22px; }.new-session { font-size: 14px; }.views { gap: 1px; justify-content: space-between; width: 100%; overflow-x: auto; scrollbar-width: none; }.views::-webkit-scrollbar { display: none; }.views button { flex: 0 0 auto; padding: 8px 6px; font-size: 14px; gap: 6px; min-height: 42px; }.views button :global(svg) { width: 14px; }.divider { margin: 0 2px; }.view-count { font-size: 14px; min-width: 14px; padding: 1px 2px; }.viewbar { gap: 8px; padding-bottom: 8px; }.display { width: 100%; justify-content: space-between; }.display select { max-width: none; padding-left: 0; font-size: 16px; }.filterbar { gap: 8px; padding: 12px 0; flex-wrap: wrap; }.search { max-width: none; min-width: 0; }.search input { min-height: 42px; font-size: 16px; }.filter-button { min-height: 44px; }.result-count { display: none; }.filter-panel { position: fixed; top: 15dvh; left: 16px; right: 16px; width: auto; max-height: 70dvh; box-sizing: border-box; }.filter-groups { gap: 16px; }.filter-groups label { padding: 8px 0; }.board-foot { align-items: start; font-size: 14px; }.board-foot span { max-width: 180px; }.board-foot span :global(svg) { flex-shrink: 0; }.focused .cards, .cards { grid-template-columns: minmax(0, 1fr); }.card-foot { flex-wrap: nowrap; }.empty { padding: 24px 12px; }.prompt-preview { font-size: 14px; } }
  @media (max-width: 380px) { .views button:not(:first-child) :global(svg) { display: none; } }
  @media (pointer: coarse) { .disclosure { min-height: 40px; padding: 8px; } }
  @media (prefers-reduced-motion: reduce) { .session-card, .details, .disclosure :global(svg) { transition: none; } }
</style>
