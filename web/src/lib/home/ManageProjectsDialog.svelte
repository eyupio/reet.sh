<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { shell } from '../dialogShell'
  import { canAutoFocus } from '../focus'
  import { UnauthorizedError } from '../api'
  import { PROJECT_COLOURS, projectColour, projectColourHex, type ProjectColour } from '../projectColour'
  import { mergeRepos, nextFolders, reposFrom, repoKey, savedKeys, scanOrder, type DiscoveredRepo, type ScanFolder } from '../projectDiscovery'
  import { browseFolder, createFolderProject, readiness, refusalSentence, setFolderProjectGroup, TerminalError,
    type FolderProject, type ProjectGroup, type TerminalMachine } from '../terminal/api'

  // The one place to arrange projects: every project in its group, drag or move, recolour, rename and remove; a
  // repository finder for each machine that can browse its folders; and the ways into Add and Edit. The page owns the
  // read lists and every write's answer (`onsettle`), so the dialog never keeps a second copy.
  let { projects, groups = [], machines = [], signal, opener = null,
    onadd, onedit, onremove, oncolour, onrename, onmovetogroup, onarrange,
    ongroupadd, ongroupedit, ongroupremove, ongroupcolour, ongroupsarrange,
    onsettle, onnotice, onreload, onsignedout, onclose }: {
    projects: FolderProject[] | undefined
    groups?: ProjectGroup[]
    machines?: TerminalMachine[]
    signal: () => AbortSignal
    opener?: HTMLElement | null
    onadd: (opener: HTMLElement) => void
    onedit: (project: FolderProject, opener: HTMLElement) => void
    onremove: (project: FolderProject, opener: HTMLElement) => void
    oncolour: (project: FolderProject, colour: ProjectColour) => void
    onrename: (project: FolderProject, name: string) => void
    onmovetogroup: (project: FolderProject, groupId: string) => void
    onarrange?: (order: { id: string; version: number; groupId: string }[]) => void
    ongroupadd: (opener: HTMLElement) => void
    ongroupedit: (group: ProjectGroup, opener: HTMLElement) => void
    ongroupremove: (group: ProjectGroup, opener: HTMLElement) => void
    ongroupcolour: (group: ProjectGroup, colour: ProjectColour) => void
    ongroupsarrange: (ids: string[], versions: number[]) => void
    onsettle: (projects: FolderProject[]) => void
    onnotice: (notice: { text: string; bad?: boolean } | null) => void
    onreload: () => void
    onsignedout: () => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  const MAX_SCAN_REQUESTS = 18, MAX_SCAN_DEPTH = 3, MAX_ADDED_AT_ONCE = 12

  let dialog = $state<HTMLDialogElement>(), searchInput = $state<HTMLInputElement>()
  let tab = $state<'projects' | 'discover'>('projects')
  let q = $state('')

  const list = $derived(projects ?? [])
  const sortedGroups = $derived([...groups].sort((a, b) => a.position - b.position))
  const inGroup = (gid: string) => list.filter(p => (p.groupId ?? '') === gid)
  const ungrouped = $derived(inGroup(''))
  const groupName = (gid: string) => sortedGroups.find(g => g.id === gid)?.name ?? ''
  const term = $derived(q.trim().toLowerCase())
  const matches = (p: FolderProject) => !term || [p.name, p.machine, p.dir, groupName(p.groupId ?? '')].some(v => (v ?? '').toLowerCase().includes(term))
  const shownIn = (gid: string) => inGroup(gid).filter(matches)
  const shownUngrouped = $derived(ungrouped.filter(matches))
  const visibleGroups = $derived(sortedGroups.filter(g => !term || shownIn(g.id).length > 0))
  const shownCount = $derived(list.filter(matches).length)
  const filtering = $derived(term.length > 0)

  // One drag at a time: a repository moving between groups or a group changing its place.
  type Drag = { kind: 'repo' | 'group'; id: string }
  type Drop = { kind: 'repo'; groupId: string; index: number } | { kind: 'group'; groupId: string } | { kind: 'groups'; index: number }
  let drag = $state<Drag | null>(null), drop = $state<Drop | null>(null)

  function startRepoDrag(event: DragEvent, p: FolderProject) {
    if (filtering) { event.preventDefault(); return }
    drag = { kind: 'repo', id: p.id }
    event.dataTransfer?.setData('text/plain', `repo:${p.id}`)
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }
  function startGroupDrag(event: DragEvent, g: ProjectGroup) {
    if (filtering) { event.preventDefault(); return }
    drag = { kind: 'group', id: g.id }
    event.dataTransfer?.setData('text/plain', `group:${g.id}`)
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }
  function endDrag() { drag = null; drop = null }
  function overRepo(event: DragEvent, p: FolderProject) {
    if (drag?.kind !== 'repo' || drag.id === p.id) return
    event.preventDefault(); event.stopPropagation()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    const gid = p.groupId ?? ''
    const items = inGroup(gid), index = items.findIndex(x => x.id === p.id)
    const box = (event.currentTarget as HTMLElement).getBoundingClientRect()
    drop = { kind: 'repo', groupId: gid, index: event.clientY > box.top + box.height / 2 ? index + 1 : index }
  }
  function overGroup(event: DragEvent, gid: string) {
    if (!drag) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    if (drag.kind === 'repo') drop = { kind: 'group', groupId: gid }
    else {
      const at = sortedGroups.findIndex(g => g.id === gid)
      if (at >= 0) drop = { kind: 'groups', index: at }
    }
  }
  function overGroupHead(event: DragEvent, g: ProjectGroup) {
    if (!drag) return
    event.preventDefault(); event.stopPropagation()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    if (drag.kind === 'group') {
      if (drag.id !== g.id) drop = { kind: 'groups', index: sortedGroups.findIndex(x => x.id === g.id) }
    } else drop = { kind: 'group', groupId: g.id }
  }
  function applyDrop() {
    const moving = drag, target = drop
    drag = null; drop = null
    if (!moving || !target) return
    if (moving.kind === 'repo' && target.kind === 'repo') reorderRepo(moving.id, target.groupId, target.index)
    else if (moving.kind === 'repo' && target.kind === 'group') reorderRepo(moving.id, target.groupId, inGroup(target.groupId).length)
    else if (moving.kind === 'group' && target.kind === 'groups') reorderGroup(moving.id, target.index)
  }

  function reorderRepo(projectId: string, targetGroup: string, index: number) {
    if (!onarrange) return
    const moving = list.find(p => p.id === projectId)
    if (!moving) return
    const order = ['', ...sortedGroups.map(g => g.id)]
    const buckets = new Map(order.map(gid => [gid, inGroup(gid).filter(p => p.id !== projectId)]))
    const sourceGroup = moving.groupId ?? ''
    const sourceIndex = inGroup(sourceGroup).findIndex(p => p.id === projectId)
    let at = index
    if (sourceGroup === targetGroup && sourceIndex >= 0 && sourceIndex < at) at--
    const bucket = buckets.get(targetGroup) ?? buckets.get('')!
    bucket.splice(Math.max(0, Math.min(at, bucket.length)), 0, moving)
    const flat = order.flatMap(gid => (buckets.get(gid) ?? []).map(p => ({ id: p.id, version: p.version, groupId: gid })))
    onarrange(flat)
  }
  function reorderGroup(groupId: string, index: number) {
    const from = sortedGroups.findIndex(g => g.id === groupId)
    if (from < 0) return
    if (from === index) return
    const next = [...sortedGroups]
    const [group] = next.splice(from, 1)
    // Dropped on a group: before it when dragged up, after it when dragged down.
    const at = Math.max(0, Math.min(index, next.length))
    next.splice(at, 0, group)
    ongroupsarrange(next.map(g => g.id), next.map(g => g.version))
  }
  function moveInGroup(p: FolderProject, by: -1 | 1) {
    const gid = p.groupId ?? ''
    const items = inGroup(gid), at = items.findIndex(x => x.id === p.id), to = at + by
    if (at < 0 || to < 0 || to >= items.length) return
    // Dropping after the neighbour means inserting at its index + 2: the list is read again with this project removed.
    reorderRepo(p.id, gid, by < 0 ? at - 1 : at + 2)
  }
  function moveGroup(group: ProjectGroup, by: -1 | 1) {
    const from = sortedGroups.findIndex(g => g.id === group.id), to = from + by
    if (from < 0 || to < 0 || to >= sortedGroups.length) return
    const next = [...sortedGroups]
    ;[next[from], next[to]] = [next[to], next[from]]
    ongroupsarrange(next.map(g => g.id), next.map(g => g.version))
  }

  // The card menus close themselves before their choice runs, so a dialog opened from one never sits under an open menu.
  // One menu open at a time: a press elsewhere or Escape closes an open card menu before it closes the dialog.
  function outside(event: PointerEvent) {
    const target = event.target as HTMLElement
    for (const details of dialog?.querySelectorAll<HTMLDetailsElement>('details.pop[open]') ?? []) {
      if (!details.contains(target)) details.open = false
    }
  }
  function dialogKeys(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const open = [...(dialog?.querySelectorAll<HTMLDetailsElement>('details.pop[open]') ?? [])]
    if (open.length === 0) return
    event.preventDefault(); event.stopPropagation()
    for (const details of open) details.open = false
  }
  function closePop(event: Event) {
    const details = (event.currentTarget as HTMLElement).closest('details')
    if (details) details.open = false
  }
  function pickPop(event: Event, run: () => void) { closePop(event); run() }

  let renaming = $state<string | null>(null), renameText = $state(''), renameInput = $state<HTMLInputElement>()
  async function beginRename(p: FolderProject) {
    renaming = p.id; renameText = p.name
    await tick()
    renameInput?.focus(); renameInput?.select()
  }
  function saveRename(p: FolderProject) {
    const name = renameText.trim()
    renaming = null
    if (name && name !== p.name) onrename(p, name)
  }

  // The repository finder: one machine at a time, a bounded browse of its home and the folders people keep work in.
  const browserMachines = $derived(machines.filter(m => browseBlock(m) === ''))
  function browseBlock(machine: TerminalMachine | undefined): string {
    if (!machine) return 'Choose a machine.'
    const why = readiness(machine)
    if (why) return why
    if (machine.capabilities?.browseOff) return 'Folder browsing is switched off on that machine.'
    if (!machine.capabilities?.browse) return 'Update reet on that machine to browse its folders.'
    return ''
  }
  let scanHost = $state('')
  let scanning = $state(false), scanRequests = $state(0), found = $state<DiscoveredRepo[]>([]), scanError = $state(''), scannedHost = $state('')
  const selectedRepos = new SvelteSet<string>()
  let addTargetGroup = $state(''), adding = $state(false), addError = $state('')
  const sessionId = crypto.randomUUID()
  const saved = $derived(savedKeys(list))
  const pickable = $derived(found.filter(r => !saved.has(repoKey(r))))
  const selectedFound = $derived(found.filter(r => selectedRepos.has(repoKey(r)) && !saved.has(repoKey(r))))
  const allPicked = $derived(pickable.length > 0 && selectedFound.length === pickable.length)

  $effect(() => {
    if (!scanHost && browserMachines.length > 0) scanHost = browserMachines[0].agentId
  })

  async function discover() {
    if (!scanHost || scanning) return
    const host = scanHost
    scanning = true; scanError = ''; addError = ''; found = []; scanRequests = 0; scannedHost = host
    selectedRepos.clear()
    const seen = new Set<string>()
    let queue: ScanFolder[] = [{ path: '', depth: 0 }]
    let all: DiscoveredRepo[] = []
    let requests = 0
    try {
      while (queue.length > 0 && requests < MAX_SCAN_REQUESTS) {
        queue.sort(scanOrder)
        const next = queue.shift()!
        if (seen.has(next.path)) continue
        seen.add(next.path)
        // A folder can hold more entries than one browse page lists: follow its cursor while the scan has budget left.
        let after: string | undefined
        while (requests < MAX_SCAN_REQUESTS) {
          const page = await browseFolder(host, { path: next.path, after, session: sessionId }, signal())
          requests++
          scanRequests = requests
          all = mergeRepos(all, reposFrom(page, host))
          found = all
          queue = [...queue, ...nextFolders(page, next.depth, MAX_SCAN_DEPTH, seen)]
          const last = page.entries.at(-1)?.name
          if (!page.more || !last || last === after) break
          after = last
        }
      }
    } catch (e) {
      if (refused(e)) { onsignedout(); return }
      scanError = refusalSentence(e)
    } finally { scanning = false }
  }
  function toggleRepo(repo: DiscoveredRepo) {
    const key = repoKey(repo)
    if (selectedRepos.has(key)) selectedRepos.delete(key)
    else selectedRepos.add(key)
  }
  function toggleAll() {
    if (allPicked) selectedRepos.clear()
    else for (const repo of pickable) selectedRepos.add(repoKey(repo))
  }
  async function addFound() {
    if (adding || selectedFound.length === 0) return
    adding = true; addError = ''
    const created: FolderProject[] = []
    try {
      for (const repo of selectedFound.slice(0, MAX_ADDED_AT_ONCE)) {
        // No name: the server names the project after its folder and suffixes a clash, so two machines with a `site`
        // both come in, and a folder name longer than the project-name limit is still accepted.
        let project = await createFolderProject({ host: repo.host, dir: repo.dir, isRepo: true }, signal())
        if (addTargetGroup) {
          await setFolderProjectGroup(project.id, addTargetGroup, signal())
          project = { ...project, groupId: addTargetGroup }
        }
        created.push(project)
        selectedRepos.delete(repoKey(repo))
      }
      const ids = new Set(created.map(p => p.id))
      onsettle([...list.filter(p => !ids.has(p.id)), ...created].sort((a, b) => a.position - b.position || a.createdAt - b.createdAt))
      const where = addTargetGroup ? ` to ${groupName(addTargetGroup) || 'the group'}` : ''
      onnotice({ text: created.length === 1 ? `Sorted. ${created[0].name} added${where}.` : `Sorted. ${created.length} repositories added${where}.` })
    } catch (e) {
      if (created.length > 0) {
        const ids = new Set(created.map(p => p.id))
        onsettle([...list.filter(p => !ids.has(p.id)), ...created].sort((a, b) => a.position - b.position || a.createdAt - b.createdAt))
      }
      if (refused(e)) { onsignedout(); return }
      addError = refusalSentence(e)
      onreload()
    } finally { adding = false }
  }

  const refused = (e: unknown) => e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)
  function openDiscover(groupId = '') {
    addTargetGroup = groupId
    tab = 'discover'
    if (!scannedHost && !scanning && pickable.length === 0) void discover()
  }

  function tabKeys(event: KeyboardEvent) {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    tab = tab === 'projects' ? 'discover' : 'projects'
    void tick().then(() => document.querySelector<HTMLElement>(`#${CSS.escape(`${id}-tab-${tab}`)}`)?.focus())
  }

  onMount(() => {
    dialog?.showModal()
    if (canAutoFocus()) searchInput?.focus()
    return () => dialog?.close()
  })
  function close() { dialog?.close(); onclose(); opener?.focus() }
  function done() { close() }

  // The button that opened a menu is the right place for a stacked dialog's focus to return to.
  function headFor(event: Event): HTMLElement {
    const details = (event.currentTarget as HTMLElement)?.closest('details')
    return (details?.querySelector('summary') as HTMLElement) ?? (event.currentTarget as HTMLElement)
  }
  function openFromButton(event: Event, run: () => void) {
    closePop(event)
    run()
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'full' }} aria-labelledby={`${id}-title`} onpointerdown={outside} onkeydown={dialogKeys} oncancel={(e) => { e.preventDefault(); if (!adding) close() }}>
  <div class="box" data-shell-card>
    <header data-shell-head>
      <div class="head-copy">
        <p class="eyebrow">Projects</p>
        <h2 id={`${id}-title`}>Manage projects</h2>
        <p class="lede" data-shell-aside>Group, colour and rename the folders your sessions start in. Nothing here changes a folder on a machine.</p>
      </div>
      <button type="button" class="close" aria-label="Close manage projects" onclick={close}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    </header>

    <div class="tabs" role="tablist" aria-label="Manage projects" tabindex="-1" onkeydown={tabKeys}>
      <button type="button" id={`${id}-tab-projects`} role="tab" aria-selected={tab === 'projects'} aria-controls={`${id}-panel-projects`} tabindex={tab === 'projects' ? 0 : -1}
        onclick={() => (tab = 'projects')}>Projects <span>{list.length}</span></button>
      <button type="button" id={`${id}-tab-discover`} role="tab" aria-selected={tab === 'discover'} aria-controls={`${id}-panel-discover`} tabindex={tab === 'discover' ? 0 : -1}
        onclick={() => openDiscover()}>Find repositories</button>
    </div>

    {#if tab === 'projects'}
      <div class="body" id={`${id}-panel-projects`} role="tabpanel" aria-labelledby={`${id}-tab-projects`} data-shell-body>
        <div class="toolbar">
          <label class="search">
            <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="11" cy="11" r="6.5" /><path d="M16 16l4.5 4.5" /></svg>
            <span class="vh">Search projects</span>
            <input bind:this={searchInput} bind:value={q} type="search" placeholder="Search by name, machine or folder" autocomplete="off" />
          </label>
          <div class="tools">
            <button type="button" class="btn quiet" onclick={(e) => ongroupadd(e.currentTarget)}>New group</button>
            <button type="button" class="btn primary" onclick={(e) => onadd(e.currentTarget)}>
              <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>
              Add project
            </button>
          </div>
        </div>

        {#if !projects}
          <div class="empty"><p>Reading your projects…</p></div>
        {:else if list.length === 0 && sortedGroups.length === 0}
          <div class="empty">
            <span class="empty-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 6.5A1.5 1.5 0 014.5 5H9l2 2h8.5A1.5 1.5 0 0121 8.5v9a1.5 1.5 0 01-1.5 1.5h-15A1.5 1.5 0 013 17.5z" /></svg></span>
            <h3>No projects yet</h3>
            <p>Add a folder on one of your machines, or find the repositories already there.</p>
            <div class="empty-actions">
              <button type="button" class="btn primary" onclick={(e) => onadd(e.currentTarget)}>Add a project</button>
              <button type="button" class="btn" onclick={() => openDiscover()}>Find repositories</button>
            </div>
          </div>
        {:else}
          {#if filtering && shownCount === 0}
            <div class="empty"><p>No project matches “{q.trim()}”.</p><button type="button" class="btn quiet" onclick={() => (q = '')}>Clear search</button></div>
          {/if}

          <section class="group ungrouped" role="group" aria-label="Ungrouped projects" class:drop-active={drop?.kind === 'group' && drop.groupId === ''}
            ondragover={(e) => overGroup(e, '')} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
            <header class="group-head">
              <span class="group-dot none" aria-hidden="true"></span>
              <h3>{visibleGroups.length > 0 ? 'Ungrouped' : 'All projects'}</h3>
              <span class="count">{shownUngrouped.length}</span>
              <div class="group-tools">
                <button type="button" class="ghost" onclick={(e) => openFromButton(e, () => openDiscover(''))}>Find repositories</button>
              </div>
            </header>
            {#if shownUngrouped.length > 0}
              <ul class="repos">
                {#each shownUngrouped as p, index (p.id)}
                  {@render repoCard(p, '', index)}
                {/each}
              </ul>
            {:else}
              <div class="drop-hint" class:active={drop?.kind === 'group' && drop.groupId === ''}>{drag ? 'Drop to leave the group' : filtering ? 'No match here.' : 'Projects outside a group live here.'}</div>
            {/if}
          </section>

          {#each visibleGroups as g, groupIndex (g.id)}
            {@const items = shownIn(g.id)}
            <section class="group" role="group" aria-label={g.name} class:drop-active={drop?.kind === 'group' && drop.groupId === g.id}
              class:group-over={drop?.kind === 'groups' && drop.index === groupIndex && drag?.kind === 'group' && drag.id !== g.id}
              ondragover={(e) => overGroup(e, g.id)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
              <header class="group-head" role="group" aria-label={`${g.name} group`} ondragover={(e) => overGroupHead(e, g)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
                <span class="grip group-grip" title={filtering ? 'Clear the search to arrange groups' : 'Drag to move this group'} aria-hidden="true"
                  draggable={!filtering} ondragstart={(e) => startGroupDrag(e, g)} ondragend={endDrag}>
                  <svg viewBox="0 0 12 16" fill="currentColor"><circle cx="3" cy="3.5" r="1.1"/><circle cx="9" cy="3.5" r="1.1"/><circle cx="3" cy="8" r="1.1"/><circle cx="9" cy="8" r="1.1"/><circle cx="3" cy="12.5" r="1.1"/><circle cx="9" cy="12.5" r="1.1"/></svg>
                </span>
                <span class="group-dot" style={`--swatch:${projectColourHex(g.colour)}`} aria-hidden="true"></span>
                <h3>{g.name}</h3>
                <span class="count">{items.length}</span>
                <div class="group-tools">
                  <button type="button" class="ghost" onclick={(e) => openFromButton(e, () => openDiscover(g.id))}>Add repos</button>
                  <details class="pop">
                    <summary class="icon-btn" aria-label={`More for group ${g.name}`} title={`More for ${g.name}`}>
                      <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
                    </summary>
                    <div class="pop-menu" role="menu" aria-label={g.name}>
                      <button type="button" role="menuitem" onclick={(e) => pickPop(e, () => ongroupedit(g, headFor(e)))}>Rename group</button>
                      <button type="button" role="menuitem" disabled={groupIndex === 0} onclick={(e) => pickPop(e, () => moveGroup(g, -1))}>Move group up</button>
                      <button type="button" role="menuitem" disabled={groupIndex === visibleGroups.length - 1} onclick={(e) => pickPop(e, () => moveGroup(g, 1))}>Move group down</button>
                      <p class="pop-label">Group colour</p>
                      <div class="swatches" role="group" aria-label="Group colour">
                        {#each PROJECT_COLOURS as choice}
                          <button type="button" role="menuitemradio" aria-checked={projectColour(g.colour) === choice.id} aria-label={choice.name} title={choice.name}
                            style={`--swatch:${choice.hex}`} class:on={projectColour(g.colour) === choice.id}
                            onclick={(e) => pickPop(e, () => ongroupcolour(g, choice.id))}>{choice.id === '' ? '×' : ''}</button>
                        {/each}
                      </div>
                      <button type="button" role="menuitem" class="danger" onclick={(e) => pickPop(e, () => ongroupremove(g, headFor(e)))}>Remove group</button>
                    </div>
                  </details>
                </div>
              </header>
              {#if items.length > 0}
                <ul class="repos">
                  {#each inGroup(g.id) as p, index (p.id)}
                    {#if matches(p)}{@render repoCard(p, g.id, index)}{/if}
                  {/each}
                </ul>
              {:else}
                <div class="drop-hint" class:active={drop?.kind === 'group' && drop.groupId === g.id}>{drag ? 'Drop to add to this group' : filtering ? 'No match here.' : 'Drag projects here, or find repositories.'}</div>
              {/if}
            </section>
          {/each}
        {/if}
      </div>
    {:else}
      <div class="body" id={`${id}-panel-discover`} role="tabpanel" aria-labelledby={`${id}-tab-discover`} data-shell-body>
        <section class="discover-head">
          <div>
            <h3>Find repositories on a machine</h3>
            <p>reet looks through the folders people keep work in, on a machine you choose. It only lists folders the machine already lets reet browse.</p>
          </div>
          {#if browserMachines.length > 1}
            <label class="machine-pick">
              <span>Machine</span>
              <select bind:value={scanHost} disabled={scanning} onchange={() => { found = []; scannedHost = ''; scanError = ''; selectedRepos.clear() }}>
                {#each browserMachines as m (m.agentId)}<option value={m.agentId}>{m.host || m.agentId}</option>{/each}
              </select>
            </label>
          {/if}
        </section>

        {#if browserMachines.length === 0}
          <div class="empty">
            <span class="empty-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 5.5h16v13H4zM8 9.5h8M8 13.5h5" /></svg></span>
            <h3>No machine can browse its folders</h3>
            <p>{machines.length === 0 ? 'No machines are connected right now.' : browseBlock(machines[0])}</p>
          </div>
        {:else}
          <div class="scan-bar">
            <div class="scan-where">
              <span class="machine-name">{browserMachines.find(m => m.agentId === scanHost)?.host || scanHost}</span>
              {#if scanning}<span role="status" aria-live="polite" class="scanning">Scanning… {scanRequests} of {MAX_SCAN_REQUESTS} folders</span>
              {:else if scannedHost && !scanError}<span role="status" aria-live="polite">{pickable.length} {pickable.length === 1 ? 'repository' : 'repositories'} found</span>{/if}
            </div>
            <button type="button" class="btn primary" disabled={scanning || !scanHost} aria-busy={scanning} onclick={() => void discover()}>
              {scanning ? 'Scanning…' : scannedHost ? 'Scan again' : 'Scan for repositories'}
            </button>
          </div>

          {#if scanError}<p class="problem" role="alert">{scanError}</p>{/if}

          {#if scanning && found.length === 0}
            <div class="scan-skeleton" aria-hidden="true"><span></span><span></span><span></span></div>
          {:else if pickable.length > 0}
            <div class="found-bar">
              <label class="check-all">
                <input type="checkbox" checked={allPicked} onchange={toggleAll} />
                <span>Select all</span>
              </label>
              <label class="target-group">
                <span>Add to</span>
                <select bind:value={addTargetGroup}>
                  <option value="">No group</option>
                  {#each sortedGroups as g (g.id)}<option value={g.id}>{g.name}</option>{/each}
                </select>
              </label>
              <button type="button" class="btn primary" disabled={adding || selectedFound.length === 0} aria-busy={adding} onclick={() => void addFound()}>
                {adding ? 'Adding…' : selectedFound.length === 0 ? 'Choose repositories' : `Add ${selectedFound.length} ${selectedFound.length === 1 ? 'repository' : 'repositories'}`}
              </button>
            </div>
            {#if addError}<p class="problem" role="alert">{addError}</p>{/if}
            <ul class="found">
              {#each pickable as repo (repoKey(repo))}
                {@const key = repoKey(repo)}
                <li>
                  <label class="found-row" class:on={selectedRepos.has(key)}>
                    <input type="checkbox" checked={selectedRepos.has(key)} onchange={() => toggleRepo(repo)} />
                    <span class="repo-ico" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg></span>
                    <span class="found-copy">
                      <strong>{repo.name}</strong>
                      <span>{repo.dir}</span>
                    </span>
                    <span class="found-host">{browserMachines.find(m => m.agentId === repo.host)?.host || repo.host}</span>
                  </label>
                </li>
              {/each}
            </ul>
          {:else if scannedHost && !scanning}
            <div class="empty">
              <span class="empty-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><circle cx="12" cy="12" r="8" /><path d="M8.5 12h7M12 8.5v7" /></svg></span>
              <h3>{found.length > 0 ? 'Everything found is in reet' : 'No repositories found'}</h3>
              <p>{found.length > 0 ? 'Every repository the scan found is already a project. Scan again after you add more folders.' : 'Try another machine, or add the folder by hand.'}</p>
              <button type="button" class="btn" onclick={(e) => onadd(e.currentTarget)}>Add a project by hand</button>
            </div>
          {:else}
            <div class="empty"><p>Choose a machine and scan its folders for git repositories.</p></div>
          {/if}
        {/if}
      </div>
    {/if}

    <footer data-shell-foot>
      <p>{list.length} {list.length === 1 ? 'project' : 'projects'} · {sortedGroups.length} {sortedGroups.length === 1 ? 'group' : 'groups'}{#if filtering} · {shownCount} shown{/if}</p>
      <div class="foot-actions">
        {#if tab === 'projects'}<button type="button" class="btn quiet" onclick={() => openDiscover()}>Find repositories</button>{/if}
        <button type="button" class="btn" onclick={done}>Done</button>
      </div>
    </footer>
  </div>
</dialog>

{#snippet repoCard(p: FolderProject, gid: string, index: number)}
  <li class="repo" class:dragging={drag?.kind === 'repo' && drag.id === p.id}
    class:over-top={drop?.kind === 'repo' && drop.groupId === gid && drop.index === index && drag?.id !== p.id}
    class:over-bottom={drop?.kind === 'repo' && drop.groupId === gid && drop.index === index + 1 && drag?.id !== p.id}
    ondragover={(e) => overRepo(e, p)} ondrop={(e) => { e.preventDefault(); applyDrop() }}>
    <span class="grip" title={filtering ? 'Clear the search to arrange projects' : `Drag ${p.name}`} aria-hidden="true"
      draggable={!filtering && !!onarrange} ondragstart={(e) => startRepoDrag(e, p)} ondragend={endDrag}>
      <svg viewBox="0 0 12 16" fill="currentColor"><circle cx="3" cy="3.5" r="1.1"/><circle cx="9" cy="3.5" r="1.1"/><circle cx="3" cy="8" r="1.1"/><circle cx="9" cy="8" r="1.1"/><circle cx="3" cy="12.5" r="1.1"/><circle cx="9" cy="12.5" r="1.1"/></svg>
    </span>
    <span class="repo-swatch" style={`--swatch:${projectColourHex(p.colour)}`} aria-hidden="true"></span>
    {#if p.isRepo}
      <span class="repo-ico" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg></span>
    {:else}
      <span class="repo-ico folder" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 6.5A1.5 1.5 0 014.5 5H9l2 2h8.5A1.5 1.5 0 0121 8.5v9a1.5 1.5 0 01-1.5 1.5h-15A1.5 1.5 0 013 17.5z" /></svg></span>
    {/if}
    <div class="repo-copy">
      {#if renaming === p.id}
        <input bind:this={renameInput} bind:value={renameText} class="rename" maxlength="80" aria-label={`New name for ${p.name}`}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); saveRename(p) } else if (e.key === 'Escape') { e.preventDefault(); renaming = null } }}
          onblur={() => saveRename(p)} />
      {:else}
        <strong>{p.name}</strong>
      {/if}
      <span class="repo-meta">{p.machine || 'Machine not enrolled'}{#if p.baseBranch} · <code>{p.baseBranch}</code>{/if}<span class="dir">{p.dir}</span></span>
    </div>
    <label class="group-pick">
      <span class="vh">Group for {p.name}</span>
      <select value={p.groupId ?? ''} onchange={(e) => onmovetogroup(p, e.currentTarget.value)}>
        <option value="">No group</option>
        {#each sortedGroups as g (g.id)}<option value={g.id}>{g.name}</option>{/each}
      </select>
    </label>
    <details class="pop">
      <summary class="icon-btn" aria-label={`More for ${p.name}`} title={`More for ${p.name}`}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
      </summary>
      <div class="pop-menu" role="menu" aria-label={p.name}>
        <button type="button" role="menuitem" onclick={(e) => pickPop(e, () => void beginRename(p))}>Rename project</button>
        <button type="button" role="menuitem" onclick={(e) => pickPop(e, () => onedit(p, headFor(e)))}>Folder settings…</button>
        {#if onarrange}
          <button type="button" role="menuitem" disabled={index === 0} onclick={(e) => pickPop(e, () => moveInGroup(p, -1))}>Move up</button>
          <button type="button" role="menuitem" disabled={index === inGroup(gid).length - 1} onclick={(e) => pickPop(e, () => moveInGroup(p, 1))}>Move down</button>
        {/if}
        <p class="pop-label">Project colour</p>
        <div class="swatches" role="group" aria-label="Project colour">
          {#each PROJECT_COLOURS as choice}
            <button type="button" role="menuitemradio" aria-checked={projectColour(p.colour) === choice.id} aria-label={choice.name} title={choice.name}
              style={`--swatch:${choice.hex}`} class:on={projectColour(p.colour) === choice.id}
              onclick={(e) => pickPop(e, () => oncolour(p, choice.id))}>{choice.id === '' ? '×' : ''}</button>
          {/each}
        </div>
        {#if sortedGroups.length > 0}
          <p class="pop-label">Move to group</p>
          {#each sortedGroups as g (g.id)}
            <button type="button" role="menuitemradio" aria-checked={(p.groupId ?? '') === g.id} onclick={(e) => pickPop(e, () => onmovetogroup(p, g.id))}>
              <span class="pick-dot" style={`--swatch:${projectColourHex(g.colour)}`} aria-hidden="true"></span>{g.name}
            </button>
          {/each}
          <button type="button" role="menuitemradio" aria-checked={!p.groupId} onclick={(e) => pickPop(e, () => onmovetogroup(p, ''))}><span class="pick-dot none" aria-hidden="true"></span>No group</button>
        {/if}
        <button type="button" role="menuitem" class="danger" onclick={(e) => pickPop(e, () => onremove(p, headFor(e)))}>Remove from reet</button>
      </div>
    </details>
  </li>
{/snippet}

<style>
  dialog { width: min(1000px, calc(100vw - 32px)); max-width: none; height: min(780px, calc(100dvh - 32px)); max-height: none; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); backdrop-filter: blur(3px); }
  .box { display: flex; flex-direction: column; width: 100%; height: 100%; min-height: 0; overflow: hidden; background: var(--card); border: 1px solid var(--line); border-radius: 18px; box-shadow: 0 32px 90px -30px var(--scrim); }

  header { display: flex; align-items: flex-start; gap: 16px; padding: 20px 22px 14px; border-bottom: 1px solid var(--line-soft); }
  .head-copy { min-width: 0; flex: 1; }
  .eyebrow { margin: 0 0 6px; font: 600 14px/1.2 var(--sans); letter-spacing: .11em; text-transform: uppercase; color: var(--accent-text); }
  h2 { margin: 0; font: 600 clamp(22px, 2.4vw, 28px)/1.12 var(--serif); letter-spacing: -.025em; }
  .lede { margin: 6px 0 0; max-width: 64ch; color: var(--muted); font-size: 14px; line-height: 1.5; }
  .close { flex: none; display: grid; place-items: center; width: 36px; height: 36px; padding: 0; border: 1px solid var(--line-mid); border-radius: 10px; background: transparent; color: var(--ink-soft); cursor: pointer; }
  .close:hover { border-color: var(--muted); color: var(--ink); }
  .close svg { width: 16px; height: 16px; }

  .tabs { display: flex; gap: 4px; padding: 12px 22px 0; border-bottom: 1px solid var(--line-soft); }
  .tabs button { position: relative; display: inline-flex; align-items: center; gap: 7px; min-height: 42px; padding: 0 14px 2px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--muted); font-size: 14px; font-weight: 600; cursor: pointer; }
  .tabs button:hover { color: var(--ink); }
  .tabs button[aria-selected='true'] { color: var(--ink); border-bottom-color: var(--accent); }
  .tabs button span { padding: 1px 7px; border: 1px solid var(--line); border-radius: 99px; background: var(--bg-sunk); color: var(--muted); font: 500 14px/16px var(--sans); }

  .body { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 16px 22px 22px; display: flex; flex-direction: column; gap: 12px; }

  .toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .search { position: relative; flex: 1 1 260px; display: flex; align-items: center; }
  .search svg { position: absolute; left: 12px; width: 16px; height: 16px; color: var(--muted); pointer-events: none; }
  .search input { width: 100%; padding-left: 36px; min-height: 42px; background: var(--bg); }
  .tools { display: flex; align-items: center; gap: 8px; }
  .tools .btn { min-height: 40px; padding: 0 12px; font-size: 14px; }
  .tools svg { width: 15px; height: 15px; }

  .group { position: relative; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--card-raised) 55%, transparent); transition: border-color .15s, background .15s; }
  .group.ungrouped { background: transparent; border-style: dashed; }
  .group.drop-active { border-color: var(--accent); background: var(--accent-wash); }
  .group.group-over::before { content: ''; position: absolute; left: 10px; right: 10px; top: -7px; height: 2px; border-radius: 2px; background: var(--accent); }
  .group-head { display: flex; align-items: center; gap: 8px; padding: 9px 10px; min-height: 52px; }
  .group-head h3 { flex: 1; min-width: 0; margin: 0; font: 600 14px/1.25 var(--sans); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .group-dot { flex: none; width: 10px; height: 10px; border-radius: 50%; background: var(--swatch, var(--line-mid)); box-shadow: inset 0 0 0 1px var(--line-deep); }
  .group-dot.none { background: transparent; box-shadow: inset 0 0 0 1.5px var(--line-strong); }
  .count { flex: none; padding: 0 7px; border: 1px solid var(--line); border-radius: 99px; color: var(--muted); font: 500 14px/17px var(--sans); }
  .group-tools { display: flex; align-items: center; gap: 4px; margin-left: auto; }
  .ghost { min-height: 32px; padding: 0 9px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); font-size: 14px; font-weight: 600; cursor: pointer; white-space: nowrap; }
  .ghost:hover { color: var(--ink); background: var(--line-soft); }
  .grip { flex: none; display: grid; place-items: center; width: 14px; height: 22px; color: var(--faint); cursor: grab; opacity: .45; transition: opacity .15s; }
  .grip:hover, .repo:hover .grip, .group-head:hover .grip { opacity: 1; }
  .grip:active { cursor: grabbing; }
  .grip svg { width: 8px; height: 14px; }
  .group-grip { margin-left: 2px; }

  .repos { list-style: none; margin: 0; padding: 0 8px 8px; display: flex; flex-direction: column; gap: 6px; }
  .repo { position: relative; display: flex; align-items: center; gap: 9px; min-height: 54px; padding: 7px 9px; border: 1px solid var(--line-soft); border-radius: 10px; background: var(--bg); }
  .repo:hover { border-color: var(--line-mid); }
  .repo.dragging { opacity: .42; }
  .repo::before { content: ''; position: absolute; left: 8px; right: 8px; height: 2px; border-radius: 2px; background: var(--accent); opacity: 0; pointer-events: none; }
  .repo.over-top::before { opacity: 1; top: -5px; }
  .repo.over-bottom::before { opacity: 1; bottom: -5px; }
  .repo-swatch { flex: none; width: 8px; height: 26px; border-radius: 99px; background: var(--swatch); }
  .repo-ico { flex: none; display: grid; place-items: center; width: 22px; height: 22px; color: var(--muted); }
  .repo-ico svg { width: 15px; height: 15px; fill: var(--repo); }
  .repo-ico.folder svg { fill: none; color: var(--muted); }
  .repo-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .repo-copy strong { font-size: 14px; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .repo-meta { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--muted); font-size: 14px; white-space: nowrap; }
  .repo-meta code { font: 14px var(--mono); color: var(--ink-soft); }
  .repo-meta .dir { min-width: 0; overflow: hidden; text-overflow: ellipsis; font-family: var(--mono); opacity: .8; }
  .repo-meta .dir::before { content: '· '; }
  .rename { width: 100%; min-height: 30px !important; padding: 3px 8px !important; font-size: 14px !important; border-radius: 7px !important; background: var(--card) !important; }

  .group-pick select { max-width: 150px; min-height: 34px; padding: 4px 26px 4px 9px; font-size: 14px; border-radius: 8px; background: var(--card); color: var(--ink-soft); }
  .pop { position: relative; flex: none; }
  .pop > summary { list-style: none; }
  .pop > summary::-webkit-details-marker { display: none; }
  .icon-btn { display: grid; place-items: center; width: 34px; height: 34px; padding: 0; border: 0; border-radius: 8px; background: transparent; color: var(--muted); cursor: pointer; }
  .icon-btn:hover, .pop[open] .icon-btn { background: var(--line-soft); color: var(--ink); }
  .icon-btn svg { width: 16px; height: 16px; }
  .pop-menu { position: absolute; z-index: 20; right: 0; top: calc(100% + 5px); min-width: 220px; max-height: min(420px, 60dvh); overflow-y: auto; padding: 5px; border: 1px solid var(--line-strong); border-radius: 11px; background: var(--card-raised); box-shadow: 0 16px 44px -12px var(--scrim); }
  .pop-menu button { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 34px; padding: 0 10px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font-size: 14px; text-align: left; cursor: pointer; }
  .pop-menu button:hover:not(:disabled), .pop-menu button:focus-visible { background: var(--line-soft); }
  .pop-menu button:disabled { color: var(--faint); cursor: default; }
  .pop-menu button[aria-checked='true'] { color: var(--accent-text); }
  .pop-menu .danger { color: var(--bad); }
  .pop-label { margin: 8px 10px 3px; color: var(--muted); font-size: 14px; font-weight: 650; letter-spacing: .09em; text-transform: uppercase; }
  .swatches { display: flex; gap: 7px; padding: 2px 10px 7px; }
  .swatches button { flex: none; width: 22px; height: 22px; min-height: 0; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--swatch); color: var(--muted); font-size: 14px; line-height: 1; cursor: pointer; }
  .swatches button.on { box-shadow: 0 0 0 2px var(--card-raised), 0 0 0 4px var(--accent); }
  .pick-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--swatch); box-shadow: inset 0 0 0 1px var(--line-deep); }
  .pick-dot.none { background: transparent; }

  .drop-hint { margin: 0 8px 8px; padding: 16px 12px; border: 1px dashed var(--line-mid); border-radius: 9px; color: var(--muted); font-size: 14px; text-align: center; }
  .drop-hint.active { border-color: var(--accent); background: var(--accent-wash); color: var(--accent-text); }
  .empty { display: flex; flex-direction: column; align-items: center; gap: 8px; margin: auto; padding: 40px 16px; text-align: center; color: var(--muted); }
  .empty h3 { margin: 0; color: var(--ink); font: 600 18px/1.2 var(--serif); letter-spacing: -.01em; }
  .empty p { margin: 0; max-width: 46ch; font-size: 14px; line-height: 1.5; }
  .empty-mark { display: grid; place-items: center; width: 48px; height: 48px; border: 1px solid var(--line); border-radius: 14px; background: var(--bg-sunk); color: var(--muted); }
  .empty-mark svg { width: 22px; height: 22px; }
  .empty-actions { display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap; justify-content: center; }

  .discover-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; flex-wrap: wrap; }
  .discover-head h3 { margin: 0; font: 600 18px/1.2 var(--serif); letter-spacing: -.01em; }
  .discover-head p { margin: 5px 0 0; max-width: 62ch; color: var(--muted); font-size: 14px; line-height: 1.5; }
  .machine-pick { display: flex; flex-direction: column; gap: 4px; font-size: 14px; color: var(--muted); }
  .machine-pick select { min-height: 38px; padding: 4px 10px; }
  .scan-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding: 11px 13px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg-sunk); }
  .scan-where { display: flex; align-items: center; gap: 10px; min-width: 0; flex-wrap: wrap; }
  .machine-name { font-weight: 650; font-size: 14px; }
  .scanning { color: var(--muted); font-size: 14px; }
  .problem { margin: 0; padding: 9px 12px; border: 1px solid var(--bad); border-radius: 9px; background: var(--card); color: var(--ink); font-size: 14px; }
  .scan-skeleton { display: flex; flex-direction: column; gap: 8px; padding: 4px 0; }
  .scan-skeleton span { height: 50px; border-radius: 10px; background: linear-gradient(90deg, var(--line-soft), var(--card-raised), var(--line-soft)); background-size: 200% 100%; }
  .found-bar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .check-all { display: flex; align-items: center; gap: 7px; font-size: 14px; color: var(--ink-soft); cursor: pointer; }
  .check-all input, .found-row input { width: 17px; height: 17px; accent-color: var(--accent); }
  .target-group { display: flex; align-items: center; gap: 7px; margin-left: auto; font-size: 14px; color: var(--muted); }
  .target-group select { min-height: 36px; padding: 4px 8px; font-size: 14px; }
  .found { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 5px; }
  .found-row { display: flex; align-items: center; gap: 10px; padding: 9px 11px; border: 1px solid var(--line-soft); border-radius: 10px; background: var(--bg); cursor: pointer; }
  .found-row:hover { border-color: var(--line-mid); }
  .found-row.on { border-color: var(--accent); background: var(--accent-wash); }
  .found-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .found-copy strong { font-size: 14px; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .found-copy span { color: var(--muted); font: 14px var(--mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .found-host { flex: none; padding: 1px 8px; border: 1px solid var(--line); border-radius: 99px; color: var(--muted); font-size: 14px; }

  footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 22px; border-top: 1px solid var(--line-soft); background: var(--card); }
  footer p { margin: 0; color: var(--muted); font-size: 14px; }
  .foot-actions { display: flex; align-items: center; gap: 8px; }
  .foot-actions .btn { min-height: 38px; padding: 0 14px; font-size: 14px; }

  @media (max-width: 760px) {
    dialog { width: calc(100vw - 16px); height: calc(100dvh - 16px); }
    .box { border-radius: 16px; }
    header { padding: 16px 16px 12px; }
    .lede { display: none; }
    .tabs { padding: 9px 16px 0; }
    .body { padding: 13px 16px 18px; }
    .group-head { flex-wrap: nowrap; }
    .group-tools { margin-left: auto; width: auto; }
    .repo { flex-wrap: wrap; padding: 9px 10px; }
    .repo-copy { flex-basis: calc(100% - 100px); }
    .group-pick { order: 4; margin-left: 23px; flex: 1 1 auto; }
    .group-pick select { max-width: none; width: 100%; }
    .pop { order: 5; margin-left: 8px; }
    footer { padding: 10px 16px; }
    .foot-actions .btn.quiet { display: none; }
    .found-bar .btn { width: 100%; }
    .target-group { margin-left: 0; }
  }
  @media (max-width: 380px) {
    .group-head { flex-wrap: wrap; }
    .group-tools { margin-left: 0; width: 100%; justify-content: flex-end; }
  }
  @media (pointer: coarse) {
    .icon-btn { width: 44px; height: 44px; }
    .group-pick select, .tabs button { min-height: 44px; }
    .pop-menu button { min-height: 44px; font-size: 14px; }
    .swatches button { width: 30px; height: 30px; }
    .repo { min-height: 60px; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: manage-rise 200ms var(--ease-out) both; }
    @keyframes manage-rise { from { opacity: 0; transform: translateY(8px) scale(.99); } to { opacity: 1; transform: none; } }
    .scan-skeleton span { animation: manage-sheen 1.2s linear infinite; }
    @keyframes manage-sheen { to { background-position: -200% 0; } }
  }
</style>
