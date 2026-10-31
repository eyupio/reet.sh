<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { toolName, notStartedText, validatePrompt } from '../agentLaunch'
  import { agentChoices } from '../catalogue'
  import { shortAge, stateWord } from '../home'
  import { listDrafts, getDraft, deleteDraft, startDraft } from '../sessionBacklog'
  import { boardItems, emptyFilters, filterBoard, filterCount, sortBoard, STAGES, type BoardView, type SessionDraft } from '../sessionBoard'
  import { launchUpload, readiness, sessionHref, TerminalError, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { imagePrompt } from '../terminal/upload'
  import type { LaunchDefaults } from '../launchBar'
  import SessionComposer from './SessionComposer.svelte'
  import StartSessionDialog from './StartSessionDialog.svelte'
  import BoardIcon from './BoardIcon.svelte'
  import SessionActions from '../sessions/SessionActions.svelte'

  let { userId, sessions, sessionsFailed = false, projects, machines, health, profile, now, initialProject = '', removeDefault = false, onchanged, onsignedout, onstarted }: {
    userId: number; sessions: TerminalSession[] | undefined; sessionsFailed?: boolean; projects: FolderProject[]; machines: TerminalMachine[]
    health: MachineHealth[]; profile: LaunchDefaults; now: number; initialProject?: string; removeDefault?: boolean
    onchanged: () => void | Promise<void>; onsignedout: () => void; onstarted?: (session: TerminalSession) => void
  } = $props()
  const id = $props.id()
  const life = new AbortController()
  const signal = () => AbortSignal.any([life.signal, AbortSignal.timeout(30_000)])
  let drafts = $state<SessionDraft[] | undefined>(), problem = $state(''), notice = $state('')
  let view = $state<BoardView>('board'), sort = $state('newest'), compact = $state(false)
  let filters = $state(emptyFilters()), filtersOpen = $state(false)
  let composer = $state<{ draft: SessionDraft | null; opener: HTMLElement } | null>(null)
  let starting = $state<{ opener: HTMLElement } | null>(null)
  let removing = $state<SessionDraft | null>(null), deleteBusy = $state(false)
  let working = $state(''), phase = $state(''), loadingId = $state('')
  let actions = $state<SessionActions>(), filterButton = $state<HTMLButtonElement>(), filterPanel = $state<HTMLDivElement>(), deleteDialog = $state<HTMLDialogElement>()
  let generation = 0
  let activeRequests = new Set<string>()
  const machineNames = $derived(new Map(machines.map(m => [m.agentId, m.host])))
  const projectNames = $derived(new Map(projects.map(p => [p.id, p.name])))
  const all = $derived(boardItems(drafts ?? [], sessions ?? []))
  const filtered = $derived(sortBoard(filterBoard(all, filters, { machines: machineNames, projects: projectNames }), sort))
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
      drafts = next; problem = ''
    } catch (e) {
      if (life.signal.aborted || mine !== generation) return
      error(e); problem = drafts ? "Couldn't refresh the backlog. Showing your last saved items." : "Couldn't read the backlog just now."
    }
  }
  function preferences() {
    try { localStorage.setItem(`reet.sessionBoard.${userId}`, JSON.stringify({ view, sort, compact })) } catch { /* The view still works without storage. */ }
  }
  function setView(next: BoardView) { view = next; preferences() }
  function toggle(key: 'machines' | 'projects' | 'agents' | 'models', value: string) {
    filters = { ...filters, [key]: filters[key].includes(value) ? filters[key].filter(v => v !== value) : [...filters[key], value] }
  }
  function clear() { filters = emptyFilters() }
  function closeFilters(focus = false) { filtersOpen = false; if (focus) filterButton?.focus() }
  function outside(e: PointerEvent) {
    if (filtersOpen && !filterPanel?.contains(e.target as Node) && !filterButton?.contains(e.target as Node)) closeFilters()
  }
  function keydown(e: KeyboardEvent) { if (e.key === 'Escape' && filtersOpen) { e.preventDefault(); closeFilters(true) } }
  async function edit(draft: SessionDraft, opener: HTMLElement) {
    if (loadingId || working) return
    loadingId = draft.id; notice = ''
    try { const full = await getDraft(draft.id, signal()); if (!life.signal.aborted) composer = { draft: full, opener } }
    catch (e) { error(e); notice = e instanceof Error ? e.message : "Couldn't open that item." }
    finally { loadingId = '' }
  }
  function closeStart() { const opener = starting?.opener; starting = null; void tick().then(() => opener?.focus()) }
  function startedHere(session: TerminalSession) { starting = null; if (onstarted) onstarted(session); else { notice = `Started “${session.title}”. You’ll find it in Active.`; void onchanged() } }
  function closeComposer() { const opener = composer?.opener; composer = null; void tick().then(() => opener?.focus()) }
  function saved(draft: SessionDraft) {
    generation++; drafts = [...(drafts ?? []).filter(d => d.id !== draft.id), { ...draft, images: undefined }]
    closeComposer(); notice = 'Sorted. Session saved to your backlog.'; void load()
  }
  function startReason(draft: SessionDraft): string | null {
    const machine = machines.find(m => m.agentId === draft.agentId)
    const why = readiness(machine)
    if (why) return why
    const choice = machine && agentChoices(machine).find(c => c.agent.id === draft.tool)
    if (!choice?.available) return choice?.reason ?? 'This agent is unavailable on that machine.'
    if (draft.imageCount && (!machine?.capabilities?.uploads || machine.capabilities.uploadsOff)) return 'Image uploads are unavailable on this machine.'
    if (draft.effort && !machine?.capabilities?.agentsV2) return 'Update reet on this machine to choose effort.'
    if (draft.projectId && !projects.some(p => p.id === draft.projectId && p.host === draft.agentId && p.dir === draft.dir)) return 'The project changed or was removed. Edit this backlog item first.'
    return null
  }
  async function activate(item: SessionDraft) {
    if (activeRequests.has(item.id)) return
    const why = startReason(item); if (why) { notice = why; return }
    activeRequests.add(item.id); working = item.id; notice = ''; phase = 'Preparing…'
    try {
      const draft = await getDraft(item.id, signal())
      let prompt = draft.preparedPrompt || draft.prompt
      if (!draft.preparedPrompt) {
        const refs: string[] = []
        for (const [index, image] of (draft.images ?? []).entries()) {
          phase = `Sending image ${index + 1} of ${draft.images!.length}…`
          const comma = image.data.indexOf(','); const type = image.data.slice(5, image.data.indexOf(';'))
          const bytes = Uint8Array.from(atob(image.data.slice(comma + 1)), c => c.charCodeAt(0))
          const uploaded = await launchUpload(draft.agentId, new Blob([bytes], { type }), signal()); refs.push(uploaded.reference)
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
      await deleteDraft(draft, signal()); generation++; drafts = (drafts ?? []).filter(d => d.id !== draft.id)
      removing = null; notice = 'Backlog item deleted.'
    } catch (e) { error(e); notice = e instanceof Error ? e.message : "Couldn't delete that item."; removing = null; await load() }
    finally { deleteBusy = false }
  }
  onMount(() => {
    try {
      const p = JSON.parse(localStorage.getItem(`reet.sessionBoard.${userId}`) || '{}')
      if (['board', 'backlog', 'active', 'finished'].includes(p.view)) view = p.view
      if (['newest', 'oldest', 'title'].includes(p.sort)) sort = p.sort
      compact = p.compact === true
    } catch { /* Default to the full board. */ }
    void load()
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, 15_000)
    const refresh = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', refresh)
    return () => { life.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', refresh) }
  })
</script>

<svelte:window onpointerdown={outside} onkeydown={keydown} />

<section class="session-board" aria-labelledby={`${id}-heading`}>
  <header class="heading">
    <div><div class="eyebrow">YOUR WORK, IN GOOD ORDER</div><h2 id={`${id}-heading`}>A reet good day’s work.</h2><p>Plan it. Start it. See it through. <span>One place for every session.</span></p></div>
    <button class="btn primary new-session" aria-label="New session" onclick={e => composer = { draft: null, opener: e.currentTarget }}><BoardIcon name="plus" />New session</button>
  </header>
  <div class="viewbar">
    <div class="views" role="group" aria-label="Session view">
      <button class:selected={view === 'board'} aria-pressed={view === 'board'} onclick={() => setView('board')}><BoardIcon name="board" />Board</button>
      <span class="divider"></span>
      {#each STAGES as stage}<button class:selected={view === stage.id} aria-pressed={view === stage.id} onclick={() => setView(stage.id)}><BoardIcon name={stage.id} /><span>{stage.label}</span><span class="view-count">{counts[stage.id] ?? 0}</span></button>{/each}
    </div>
    <div class="display"><label><span class="vh">Sort sessions</span><select bind:value={sort} onchange={preferences}><option value="newest">Recently updated</option><option value="oldest">Oldest first</option><option value="title">Title A–Z</option></select></label><button class="density" class:on={compact} aria-label="Compact cards" aria-pressed={compact} onclick={() => { compact = !compact; preferences() }}><span></span><span></span><span></span></button></div>
  </div>
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
    <span class="result-count">{hasFilters ? `${filtered.length} of ${all.length}` : all.length} sessions</span>
  </div>
  {#if hasFilters}<div class="chips">{#each chips as chip}<button onclick={() => toggle(chip.key, chip.value)}>{chip.label}<BoardIcon name="close" size={11} /></button>{/each}{#if filters.attention}<button onclick={() => filters.attention = false}>Needs attention<BoardIcon name="close" size={11}/></button>{/if}{#if filters.images}<button onclick={() => filters.images = false}>Has images<BoardIcon name="close" size={11}/></button>{/if}<button class="clear" onclick={clear}>Clear all</button></div>{/if}
  <div class="live" role="status">{#if notice}<p class="notice">{notice}</p>{/if}</div>
  {#if problem}<p class="problem" role="alert">{problem} <button onclick={() => void load()}>Try again</button></p>{/if}
  <div class="board" class:focused={view !== 'board'} class:compact>
    {#each columns as stage}
      {@const items = filtered.filter(item => item.stage === stage.id)}
      <section class="stage {stage.id}" aria-labelledby={`${id}-${stage.id}`}>
        <header class="stage-head"><span class="stage-mark"><BoardIcon name={stage.id} size={15}/></span><h3 id={`${id}-${stage.id}`}>{stage.label}</h3><span class="stage-count">{items.length}</span>{#if stage.id === 'backlog'}<button aria-label="Add session to backlog" onclick={e => composer = { draft: null, opener: e.currentTarget }}><BoardIcon name="plus" /></button>{:else if stage.id === 'active'}<button aria-label="Start a session" title="Start a session" onclick={e => starting = { opener: e.currentTarget }}><BoardIcon name="plus" /></button>{/if}</header>
        <p class="stage-hint">{stage.hint}</p>
        <div class="cards" aria-busy={stage.id === 'backlog' && drafts === undefined && !problem || stage.id !== 'backlog' && sessions === undefined && !sessionsFailed}>
          {#if stage.id !== 'backlog' && sessions === undefined && sessionsFailed}
            <div class="empty"><span class="empty-icon"><BoardIcon name="clock" size={24}/></span><strong>Couldn’t read your sessions.</strong><p>Try again to see what’s active and finished.</p><button onclick={() => void onchanged()}>Try again</button></div>
          {:else if stage.id === 'backlog' && drafts === undefined && !problem || stage.id !== 'backlog' && sessions === undefined}
            <div class="empty"><span class="empty-icon"><BoardIcon name="clock" size={24}/></span><strong>Reading your sessions…</strong></div>
          {:else if !items.length}
            <div class="empty"><span class="empty-icon"><BoardIcon name={hasFilters ? 'search' : stage.id} size={26}/></span><strong>{hasFilters ? 'Nowt matches just yet.' : stage.id === 'backlog' ? 'Make room for your next idea.' : stage.id === 'active' ? 'Ready when you are.' : 'Good work will land here.'}</strong><p>{hasFilters ? 'Try another search or loosen your filters.' : stage.id === 'backlog' ? 'Save a clear prompt, pick an agent, and leave it ready for later.' : stage.id === 'active' ? 'Start one now, or from your backlog.' : 'Sessions move here when they end, with their history close at hand.'}</p>{#if hasFilters}<button onclick={clear}>Clear filters</button>{:else if stage.id === 'backlog'}<button onclick={e => composer = { draft: null, opener: e.currentTarget }}><BoardIcon name="plus" size={14}/>Plan a session</button>{:else if stage.id === 'active'}<button onclick={e => starting = { opener: e.currentTarget }}><BoardIcon name="plus" size={14}/>Start a session</button>{/if}</div>
          {:else}
            {#each items as item (item.key)}
              {@const draft = item.draft}
              {@const session = item.session}
              {@const project = projects.find(p => p.id === item.projectId)}
              {@const word = session ? stateWord(session) : null}
              {@const reason = draft ? startReason(draft) : null}
              <article class="session-card" class:attention={session?.attention === 'waiting'} class:starting={working === draft?.id}>
                <div class="card-top"><span class="project-label" title={project?.name ?? item.dir}><BoardIcon name="folder" size={13}/>{project?.name || (item.projectId ? 'Removed project' : item.dir.split('/').filter(Boolean).at(-1) || 'Home folder')}</span>{#if draft?.imageCount}<span class="image-count" title={`${draft.imageCount} reference images`}><BoardIcon name="image" size={13}/>{draft.imageCount}</span>{/if}{#if session}<button class="more" aria-label={`Actions for ${item.title}`} onclick={e => void actions?.openMenu(e, session)}><BoardIcon name="more"/></button>{/if}</div>
                <h4>{#if session}<a href={sessionHref(session)}>{item.title}</a>{:else}<button onclick={e => draft && void edit(draft, e.currentTarget)} disabled={!!loadingId || !!draft?.starting}>{item.title}</button>{/if}</h4>
                {#if draft}<p class="prompt-preview">{draft.prompt}</p>{:else if session?.worktreeBranch}<p class="branch" title={session.worktreeBranch}>↳ {session.worktreeBranch}</p>{/if}
                <div class="agent-meta"><span class="agent-icon">{item.tool ? toolName(item.tool).slice(0, 1) : '›'}</span><strong>{item.tool ? toolName(item.tool) : 'Shell'}</strong><span class="model" title={item.model || 'Agent default'}>{item.model || 'Agent default'}</span>{#if item.effort}<span class="effort">{item.effort}</span>{/if}</div>
                <div class="machine-line"><BoardIcon name="machine" size={13}/><span>{machineNames.get(item.agentId) || health.find(m => m.agentId === item.agentId)?.host || item.agentId}</span>{#if offline.has(item.agentId)}<span class="offline-label">Offline{session ? ' · last known' : ''}</span>{/if}</div>
                <footer class="card-foot">
                  {#if draft}
                    <span class="age" title={new Date(item.createdAt).toLocaleString()}>Saved {shortAge(Math.max(0, now - item.createdAt))} ago</span>
                    <div class="card-actions">{#if !draft.starting}<button class="edit" disabled={!!working || !!loadingId} onclick={e => void edit(draft, e.currentTarget)} aria-label={`Edit ${draft.title}`}>Edit</button><button class="delete" disabled={!!working || !!loadingId} aria-label={`Delete ${draft.title}`} onclick={() => void askDelete(draft)}>Delete</button>{/if}<button class="start" disabled={!!reason || !!working} title={reason ?? 'Start this session on its machine'} onclick={() => void activate(draft)}>{working === draft.id ? phase : draft.starting ? 'Retry start' : 'Start'}<BoardIcon name="arrow" size={13}/></button></div>
                  {:else if session && word}
                    <span class="status {word.tone}" title={word.source}><i></i>{word.word}</span><a class="open-session" href={sessionHref(session)}>{stage.id === 'finished' ? 'Review' : 'Open'}<BoardIcon name="arrow" size={13}/></a>
                  {/if}
                </footer>
                {#if draft && reason}<p class="start-note">{reason}</p>{:else if draft?.starting}<p class="start-note">Start was requested. Retry checks the same request.</p>{/if}
              </article>
            {/each}
          {/if}
        </div>
      </section>
    {/each}
  </div>
  <div class="board-foot"><span><BoardIcon name="finished" size={13}/>Saved to your account. Ready across your machines.</span><a href="/sessions">All session history<BoardIcon name="arrow" size={13}/></a></div>
</section>
{#if starting}<StartSessionDialog {projects} {machines} {profile} {initialProject} onstarted={startedHere} {onsignedout} onclose={closeStart}/>{/if}
{#if composer}<SessionComposer draft={composer.draft} {projects} {machines} {profile} {initialProject} onsaved={saved} onclose={closeComposer} onerror={error}/>{/if}
{#if removing}<dialog class="delete-dialog" bind:this={deleteDialog} aria-labelledby={`${id}-delete`} oncancel={e => { if (deleteBusy) e.preventDefault(); else removing = null }}><h2 id={`${id}-delete`}>Delete this backlog item?</h2><p>“{removing.title}” and its saved images will be removed.</p><div><button class="btn" disabled={deleteBusy} onclick={() => removing = null}>Keep session</button><button class="btn primary" disabled={deleteBusy} onclick={() => void remove()}>{deleteBusy ? 'Deleting…' : 'Delete item'}</button></div></dialog>{/if}
<SessionActions bind:this={actions} machines={health} {offline} terminals={machines} {removeDefault} {onsignedout} {onchanged} onnotice={n => notice = n?.text ?? ''}/>

<style>
  .session-board { min-width: 0; }
  .heading { display: flex; align-items: center; justify-content: space-between; gap: 22px; margin: 5px 0 27px; }
  .eyebrow { font-size: 9px; font-weight: 650; letter-spacing: .14em; color: var(--accent-text); margin-bottom: 9px; }
  h2 { font: 500 clamp(25px, 2.6vw, 36px)/1.15 var(--serif); letter-spacing: -.035em; margin: 0 0 9px; }
  .heading p { margin: 0; color: var(--muted); font-size: 12px; }
  .new-session { flex-shrink: 0; font-size: 12px; box-shadow: 0 2px 4px color-mix(in srgb, var(--ink) 6%, transparent); }
  .viewbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; border-bottom: 1px solid var(--line); padding-bottom: 14px; }
  .views { display: flex; align-items: center; gap: 3px; }
  .views button { display: flex; align-items: center; gap: 7px; border: 1px solid transparent; background: transparent; color: var(--muted); border-radius: 7px; padding: 8px 10px; cursor: pointer; font: 500 12px var(--sans); min-height: 36px; }
  .views button.selected { color: var(--ink); background: var(--card); border-color: var(--line); box-shadow: 0 1px 3px color-mix(in srgb, var(--ink) 3%, transparent); }
  .views button.selected :global(svg) { color: var(--accent-text); }
  .views button:hover { background: var(--card); }.view-count { font-size: 10px; background: var(--bg-sunk); color: var(--muted); min-width: 18px; text-align: center; border-radius: 4px; padding: 1px 3px; }
  .divider { height: 17px; width: 1px; background: var(--line); margin: 0 6px; }
  .display { display: flex; align-items: center; gap: 8px; }
  .display select { background: transparent; min-height: 34px; padding: 5px 20px 5px 7px; border-color: transparent; color: var(--muted); font-size: 11px; cursor: pointer; }
  .density { display: flex; flex-direction: column; justify-content: center; gap: 3px; width: 32px; height: 32px; border: 1px solid var(--line); background: transparent; border-radius: 6px; cursor: pointer; }
  .density span { height: 1px; width: 14px; margin: 0 auto; background: var(--muted); }.density.on { background: var(--card); }
  .filterbar { display: flex; align-items: center; gap: 10px; padding: 17px 0; }
  .search { display: flex; align-items: center; gap: 8px; color: var(--muted); flex: 1; max-width: 360px; border: 1px solid var(--line); background: var(--card); border-radius: 7px; padding: 0 10px; }
  .search input { border: 0; min-height: 35px; padding: 7px 0; width: 100%; outline: 0; font-size: 12px; background: transparent; }
  .search:focus-within { outline: 2px solid var(--accent); outline-offset: 2px; }.search button, .filter-head button { border: 0; padding: 4px; background: transparent; color: var(--muted); cursor: pointer; }
  .search input::-webkit-search-cancel-button { display: none; }
  .filter-anchor { position: relative; }.filter-button { display: flex; align-items: center; gap: 7px; min-height: 36px; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--muted); font: 500 12px var(--sans); padding: 6px 11px; cursor: pointer; }
  .filter-button.applied { border-color: var(--accent-text); color: var(--accent-text); }.filter-button > span { background: var(--accent-wash); padding: 0 5px; border-radius: 4px; }
  .result-count { margin-left: auto; font-size: 11px; color: var(--muted); white-space: nowrap; }
  .filter-panel { position: absolute; top: calc(100% + 8px); right: 0; width: min(540px, calc(100vw - 48px)); border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card); box-shadow: 0 12px 50px color-mix(in srgb, var(--ink) 12%, transparent); padding: 18px; z-index: 25; max-height: min(580px, 72dvh); overflow-y: auto; }
  .filter-head { display: flex; align-items: center; justify-content: space-between; font-size: 13px; }.filter-panel > p { font-size: 11px; color: var(--muted); margin: 4px 0 18px; }
  .filter-groups { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }.filter-groups fieldset { border: 0; padding: 0; margin: 0; min-width: 0; max-height: 165px; overflow-y: auto; }
  legend { font-size: 10px; text-transform: uppercase; letter-spacing: .08em; font-weight: 650; color: var(--muted); padding: 0; margin-bottom: 8px; }
  .filter-groups label { display: flex; align-items: center; gap: 7px; font-size: 12px; padding: 5px 0; cursor: pointer; }.filter-groups label span { overflow-wrap: anywhere; }.filter-groups small { margin-left: auto; color: var(--muted); font-size: 10px; }
  .extra-filters { display: flex; gap: 18px; flex-wrap: wrap; border-top: 1px solid var(--line); margin-top: 18px; padding-top: 14px; font-size: 12px; }.extra-filters label { display: flex; align-items: center; gap: 7px; }
  .filter-foot { display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); margin-top: 15px; padding-top: 14px; }.filter-foot > button:first-child { border: 0; background: none; font-size: 12px; color: var(--muted); cursor: pointer; }.filter-foot .btn { font-size: 12px; }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; padding: 0 0 14px; }.chips button { display: flex; align-items: center; gap: 7px; font-size: 10px; color: var(--ink-soft); background: var(--card); border: 1px solid var(--line); padding: 5px 8px; border-radius: 5px; cursor: pointer; }.chips .clear { background: none; border-color: transparent; color: var(--muted); }
  .board { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; align-items: start; }
  .board.focused { grid-template-columns: minmax(0, 1fr); }.stage { min-width: 0; border: 1px solid var(--line); border-radius: 11px; background: var(--bg-sunk); padding: 14px 10px 10px; min-height: 345px; }
  .stage-head { display: flex; align-items: center; gap: 7px; padding: 0 4px; }.stage-mark { color: var(--muted); display: flex; }.active .stage-mark { color: var(--accent-text); }.finished .stage-mark { color: var(--good); }
  h3 { font-size: 12px; font-weight: 650; margin: 0; }.stage-count { font-size: 10px; color: var(--muted); border: 1px solid var(--line); border-radius: 4px; padding: 1px 5px; min-width: 18px; text-align: center; box-sizing: border-box; }
  .stage-head > button { margin-left: auto; border: 0; background: transparent; color: var(--muted); display: flex; align-items: center; justify-content: center; cursor: pointer; width: 24px; height: 24px; border-radius: 4px; }.stage-head > button:hover { background: var(--card); }
  .stage-hint { font-size: 10px; color: var(--muted); margin: 6px 4px 15px; }
  .cards { display: grid; gap: 10px; }.focused .cards { grid-template-columns: repeat(auto-fill, minmax(290px, 1fr)); }
  .session-card { background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 13px; box-shadow: 0 2px 3px color-mix(in srgb, var(--ink) 1%, transparent); min-width: 0; transition: border-color .16s, box-shadow .16s; }.session-card:hover { border-color: var(--line-strong); box-shadow: 0 3px 9px color-mix(in srgb, var(--ink) 3%, transparent); }.session-card.attention { border-color: color-mix(in srgb, var(--warn) 45%, var(--line)); }.session-card.starting { border-color: var(--accent-text); }
  .card-top { display: flex; align-items: center; gap: 7px; min-height: 18px; color: var(--muted); font-size: 10px; }.project-label { display: flex; align-items: center; gap: 5px; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.project-label :global(svg) { flex-shrink: 0; }.image-count { display: flex; align-items: center; gap: 3px; margin-left: auto; }
  .more { border: 0; background: transparent; color: var(--muted); margin: -4px -5px -4px auto; padding: 5px; cursor: pointer; display: flex; border-radius: 4px; }.more:hover { background: var(--bg-sunk); }
  h4 { font: 600 14px/1.45 var(--sans); margin: 10px 0; overflow-wrap: anywhere; }h4 a, h4 button { color: var(--ink); text-decoration: none; }h4 button { border: 0; padding: 0; background: none; text-align: left; font: inherit; cursor: pointer; }h4 a:hover, h4 button:hover { color: var(--accent-text); }h4 button:disabled { cursor: default; opacity: 1; }
  .prompt-preview { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: 12px; color: var(--muted); margin: -3px 0 15px; overflow-wrap: anywhere; white-space: pre-line; }
  .agent-meta { display: flex; align-items: center; gap: 5px; font-size: 10px; min-width: 0; }.agent-icon { background: var(--bg-sunk); border: 1px solid var(--line); border-radius: 4px; width: 18px; height: 18px; display: inline-flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 600; flex-shrink: 0; }.agent-meta strong { font-weight: 550; white-space: nowrap; }.model { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding-left: 6px; margin-left: 2px; border-left: 1px solid var(--line); }.effort { color: var(--muted); background: var(--bg-sunk); border-radius: 3px; padding: 1px 4px; margin-left: auto; }
  .machine-line { display: flex; align-items: center; gap: 5px; color: var(--muted); font-size: 11px; margin: 9px 0 13px; min-width: 0; }.machine-line > span:first-of-type { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.offline-label { font-size: 9px; color: var(--warn); margin-left: auto; flex-shrink: 0; }
  .card-foot { display: flex; align-items: center; gap: 6px; border-top: 1px solid var(--line-soft); padding-top: 10px; justify-content: space-between; }.age { color: var(--muted); font-size: 10px; }.card-actions { display: flex; align-items: center; gap: 7px; margin-left: auto; }
  .card-actions button { font: 500 10px var(--sans); padding: 4px 0; background: none; border: 0; color: var(--muted); cursor: pointer; }.card-actions .delete { font-size: 9px; }.card-actions .delete:hover { color: var(--bad); }.card-actions .start { display: flex; align-items: center; gap: 5px; background: var(--accent-wash); color: var(--accent-text); border-radius: 5px; padding: 5px 7px; }.card-actions .start:disabled { color: var(--muted); background: var(--bg-sunk); cursor: default; }.card-actions .start:not(:disabled):hover { background: var(--tint); }
  .status { display: flex; align-items: center; gap: 5px; font-size: 10px; color: var(--muted); }.status i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }.status.good { color: var(--good); }.status.warn { color: var(--warn); }.status.bad { color: var(--bad); }
  .open-session { display: flex; align-items: center; gap: 5px; font-size: 10px; font-weight: 500; text-decoration: none; color: var(--muted); }.open-session:hover { color: var(--accent-text); }
  .start-note { font-size: 10px; color: var(--warn); border-top: 1px solid var(--line-soft); margin: 10px 0 0; padding-top: 8px; line-height: 1.5; }.branch { font: 10px/1.5 var(--mono); color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .compact .prompt-preview, .compact .stage-hint, .compact .branch { display: none; }.compact .session-card { padding: 10px 12px; }.compact .stage-head { margin-bottom: 12px; }
  .empty { text-align: center; padding: 36px 12px; color: var(--muted); grid-column: 1/-1; }.empty-icon { width: 50px; height: 50px; display: flex; align-items: center; justify-content: center; background: var(--card); border: 1px solid var(--line); border-radius: 14px; margin: 0 auto 17px; }.empty strong { font-size: 12px; color: var(--ink-soft); font-weight: 550; display: block; }.empty p { font-size: 11px; margin: 8px auto 14px; max-width: 240px; }.empty button { display: inline-flex; align-items: center; gap: 5px; color: var(--accent-text); font-size: 11px; border: 0; background: transparent; cursor: pointer; }
  .board-foot { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: 15px; font-size: 10px; color: var(--muted); }.board-foot span, .board-foot a { display: flex; gap: 5px; align-items: center; }.board-foot a { text-decoration: none; color: var(--muted); }.board-foot a:hover { color: var(--accent-text); }
  .live:empty { display: none; }.notice, .problem { padding: 10px 13px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); margin: 0 0 14px; font-size: 12px; }.problem { border-color: var(--warn); }.problem button { border: 0; background: none; color: var(--accent-text); cursor: pointer; text-decoration: underline; }.no-options { font-size: 11px; color: var(--muted); }
  .delete-dialog { padding: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--ink); max-width: min(430px, calc(100vw - 48px)); }.delete-dialog h2 { font-size: 24px; }.delete-dialog p { font-size: 13px; margin: 14px 0 22px; overflow-wrap: anywhere; }.delete-dialog > div { display: flex; justify-content: flex-end; gap: 8px; }.delete-dialog::backdrop { background: var(--scrim); }
  @media (min-width: 1800px) { .session-card { padding: 17px; }h4 { font-size: 15px; }.prompt-preview { font-size: 12px; }.agent-meta, .machine-line, .age, .status { font-size: 11px; } }
  @media (max-width: 1400px) { .board { gap: 10px; }.stage { padding: 12px 8px 8px; }.session-card { padding: 11px; }.card-foot { flex-wrap: wrap; }.card-actions { gap: 9px; }.heading p span { display: none; } }
  @media (max-width: 850px) { .board { grid-template-columns: minmax(0, 1fr); gap: 18px; }.cards { grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); }.stage { min-height: 0; }.stage-hint { font-size: 11px; }.stage-head h3 { font-size: 14px; }.session-card h4 { font-size: 14px; }.agent-meta, .machine-line, .card-actions button, .status, .open-session, .age { font-size: 11px; }.card-actions button, .open-session { min-height: 30px; }.filter-panel { right: 0; width: min(480px, calc(100vw - 54px)); }.display select { max-width: 120px; }.viewbar { flex-wrap: wrap; }.views { flex: 1; }.display { margin-left: auto; } }
  @media (max-width: 650px) { .heading { margin: 4px 0 24px; gap: 12px; align-items: start; }h2 { font-size: 27px; max-width: 220px; }.eyebrow { font-size: 8px; }.heading p { font-size: 11px; max-width: 190px; }.new-session { font-size: 0; width: 40px; height: 40px; padding: 0; margin-top: 23px; }.new-session :global(svg) { width: 20px; height: 20px; }.new-session::after { content: 'New'; font-size: 10px; }.new-session { width: auto; padding: 0 11px; }.views { gap: 1px; justify-content: space-between; width: 100%; }.views button { padding: 9px 6px; font-size: 11px; gap: 5px; min-height: 42px; }.views button :global(svg) { width: 14px; }.divider { margin: 0 2px; }.view-count { font-size: 9px; min-width: 14px; padding: 1px 2px; }.viewbar { gap: 9px; padding-bottom: 10px; }.display { width: 100%; justify-content: space-between; }.display select { max-width: none; padding-left: 0; font-size: 16px; }.filterbar { gap: 8px; padding: 13px 0; flex-wrap: wrap; }.search { max-width: none; min-width: 0; }.search input { min-height: 42px; font-size: 16px; }.filter-button { min-height: 44px; }.result-count { display: none; }.filter-panel { position: fixed; top: 15dvh; left: 16px; right: 16px; width: auto; max-height: 70dvh; box-sizing: border-box; }.filter-groups { gap: 16px; }.filter-groups label { padding: 9px 0; }.board-foot { align-items: start; font-size: 9px; }.board-foot span { max-width: 180px; }.board-foot span :global(svg) { flex-shrink: 0; }.focused .cards, .cards { grid-template-columns: minmax(0, 1fr); }.card-foot { flex-wrap: nowrap; }.empty { padding: 25px 12px; }.prompt-preview { font-size: 12px; } }
  @media (max-width: 380px) { .views button:not(:first-child) :global(svg) { display: none; } }
  @media (prefers-reduced-motion: reduce) { .session-card { transition: none; } }
</style>
