<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus } from '../sessionStatus'
  import { createTerminal, endTerminal, renameTerminal, TerminalError, terminalClosed, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { getDesk, patchDesk, type Desk, type DeskSummary } from './api'
  import { createAutosave, type Autosave, type Change, type SaveState } from './autosave'
  import type { TerminalActions } from '../terminal/TerminalPane.svelte'
  import { isPhone, screenShortSide } from '../terminal/panelLayout'
  import { shell } from '../dialogShell'
  import { layout as treeLayout, leaves, MAX_PANES, MAX_TABS, neighbour, newId, type Pane, type Split, type Tab } from './tree'
  import { DESK_KEYS, deskKey } from './keys'
  import ClosePaneDialog from './ClosePaneDialog.svelte'
  import DeskMenu from './DeskMenu.svelte'
  import DeskPane from './DeskPane.svelte'
  import DeskTabs from './DeskTabs.svelte'
  import DeskToolbar from './DeskToolbar.svelte'
  import NewTabDialog from './NewTabDialog.svelte'
  import SplitArea from './SplitArea.svelte'

  // One desk: a header (the desk's name as its switcher, the save state, Layout and the desk menu), the tab bar, one
  // terminal toolbar for the focused pane and the active tab's panes in their splits. Every edit goes through autosave at
  // once and is saved after a short pause; a save that lost to another window is merged onto the newer copy. Opening a
  // desk starts nothing: a shell starts only from New tab or a split. The desk's keys are Alt+Shift chords, caught
  // before the terminal sees them.
  let { id, desks, sessions, machines, ready, onopen, onlist, onnew, ondesk, onrefresh, onsignedout }: {
    id: string
    /** Every desk, for the switcher. */
    desks: DeskSummary[]
    sessions: TerminalSession[]
    machines: TerminalMachine[]
    /** Sessions and machines have been read at least once. */
    ready: boolean
    onopen: (id: string) => void
    onlist: () => void
    onnew: () => void
    /** A desk-level action from the menu: the page owns its dialogs and writes. */
    ondesk: (action: 'rename' | 'duplicate' | 'template' | 'delete', desk: Desk) => void
    /** Read the sessions and machines again (after a shell is started or ended here). */
    onrefresh: () => void
    onsignedout: (e: unknown) => void
  } = $props()

  let desk = $state<Desk | null>(null)
  let saveState = $state<SaveState>('saved')
  let failure = $state('')
  let auto: Autosave | undefined
  const panelId = $props.id()

  function start(deskId: string) {
    // Whatever the desk being left still has waiting is sent before that copy stops; its late answers are ignored below.
    const old = auto
    void old?.flush().finally(() => old.stop())
    desk = null; failure = ''; saveState = 'saved'
    const mine = createAutosave({
      load: () => getDesk(deskId),
      save: (d, expect, keepalive) => patchDesk(d.id, { expect, doc: d.doc }, { keepalive }),
      onState: s => { if (auto === mine) saveState = s },
      onDesk: d => { if (auto === mine) desk = d },
    })
    auto = mine
    mine.ready.then(() => {
      if (auto !== mine) return
      const loaded = mine.desk()
      // A desk written by a newer reet is neither shown nor overwritten.
      if (loaded.schema > 1) { mine.stop(); desk = null; failure = 'This desk was saved by a newer version of reet, so it is not shown here.'; return }
      desk = loaded
    }).catch(e => {
      if (auto !== mine) return
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      failure = e instanceof TerminalError && e.status === 404 ? 'That desk is not on this account any more.' : 'This desk could not be read just now. Try again in a moment.'
    })
  }
  $effect(() => { start(id) })
  export function reload() { void auto?.refresh().catch(() => { /* the next save or poll tries again */ }) }

  onMount(() => {
    // A phone shows one pane at a time (the terminal workspace's own phone rule).
    const measure = () => { phone = isPhone(innerWidth, screenShortSide(screen, innerWidth, innerHeight)) }
    measure()
    addEventListener('resize', measure)
    // Whatever is waiting is sent as the page goes away.
    const leave = () => { void auto?.flush(true) }
    addEventListener('pagehide', leave)
    const visible = () => { if (document.visibilityState === 'hidden') leave(); else reload() }
    document.addEventListener('visibilitychange', visible)
    // The desk screen takes the window's height to the pixel, as the session workspace does.
    document.documentElement.toggleAttribute('data-desk-screen', true)
    return () => {
      removeEventListener('resize', measure)
      document.documentElement.removeAttribute('data-desk-theatre')
      removeEventListener('pagehide', leave)
      document.removeEventListener('visibilitychange', visible)
      document.documentElement.removeAttribute('data-desk-screen')
      const last = auto
      auto = undefined
      void last?.flush().finally(() => last.stop())
    }
  })

  function change(c: Change) {
    if (!auto || !desk) return
    auto.change(c)
    desk = auto.desk()
  }

  const tabs = $derived(desk?.doc.tabs ?? [])
  const activeTab = $derived(tabs.find(t => t.id === desk?.doc.active) ?? tabs[0])
  const activePaneId = $derived(activeTab ? (activeTab.panes[activeTab.active] ? activeTab.active : leaves(activeTab.root)[0] ?? '') : '')
  const activePane = $derived(activeTab && activePaneId ? activeTab.panes[activePaneId] : undefined)
  // A shell started here (New tab, a split) counts as listed until the next read of the sessions lists it.
  let fresh = $state<TerminalSession[]>([])
  const sessionById = $derived(new Map([...fresh, ...sessions].map(s => [s.id, s])))
  $effect(() => { const listed = new Set(sessions.map(s => s.id)); if (untrack(() => fresh).some(s => listed.has(s.id))) fresh = untrack(() => fresh).filter(s => !listed.has(s.id)) })
  const machineById = $derived(new Map(machines.map(m => [m.agentId, m])))
  const live = (p: Pane) => { const s = p.session ? sessionById.get(p.session) : undefined; return !!s && !terminalClosed(s.state) }
  const panesOf = (t: Tab) => leaves(t.root).map(pid => t.panes[pid]).filter((p): p is Pane => !!p)
  const waiting = $derived(new Set(tabs.filter(t => panesOf(t).some(p => { const s = p.session ? sessionById.get(p.session) : undefined; return !!s && sessionStatus(s) === 'waiting' })).map(t => t.id)))
  const hosts = $derived(new Set(tabs.flatMap(t => Object.values(t.panes).map(p => p.host)).filter(Boolean)))
  const others = $derived(desks.filter(d => !d.template && d.id !== id))
  const saveWord = $derived(saveState === 'saving' ? 'Saving…' : saveState === 'unsaved' ? 'Not saved yet' : 'saved')
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`

  // Menus in the header: the switcher (the desk's name) and the desk menu (⋮).
  let menu = $state<{ kind: 'switch' | 'desk' | 'layout'; x: number; y: number; opener: HTMLElement } | null>(null)
  function openMenu(kind: 'switch' | 'desk' | 'layout', event: MouseEvent) {
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { kind, opener, x: kind === 'switch' ? box.left : box.right - 220, y: box.bottom + 4 }
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }

  async function colourDesk(colour: ProjectColour) {
    closeMenu(true)
    const current = desk
    if (!current || current.colour === colour) return
    try {
      await patchDesk(current.id, { expect: current.version, colour })
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      // A stale write is retried once on the newer copy.
      try { const fresh = await getDesk(current.id); await patchDesk(current.id, { expect: fresh.version, colour }) } catch { notice = 'That colour could not be saved. Try again.'; return }
    }
    reload()
  }

  // Tabs.
  function select(tabId: string) { if (tabId !== desk?.doc.active) change({ kind: 'active', tab: tabId }) }
  function duplicateTab(tabId: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t || tabs.length >= MAX_TABS) return
    // A copy holds the same machines, folders, names and colours; a session is in one pane only, so its panes start empty.
    const ids = new Map(Object.keys(t.panes).map(pid => [pid, newId()]))
    const copyNode = (n: Tab['root']): Tab['root'] => !n ? null : 'split' in n ? { split: n.split, ratio: n.ratio, a: copyNode(n.a)!, b: copyNode(n.b)! } : { pane: ids.get(n.pane)! }
    const panes: Record<string, Pane> = {}
    for (const [pid, p] of Object.entries(t.panes)) panes[ids.get(pid)!] = { ...p, id: ids.get(pid)!, session: '' }
    const name = [...`${t.name} copy`].slice(0, 80).join('')
    const copy: Tab = { id: newId(), name, colour: t.colour, root: copyNode(t.root), zoomed: '', active: ids.get(t.active) ?? '', panes }
    change({ kind: 'addTab', tab: copy, index: tabs.indexOf(t) + 1 })
  }

  // Closing: a pane or tab with a running shell asks first; one with nothing running closes at once.
  let closing = $state<{ tab: string; pane?: string; name: string; host: string; live: Pane[] } | null>(null)
  let notice = $state('')
  function askClose(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    const covered = paneId ? [t.panes[paneId]].filter(Boolean) : panesOf(t)
    const running = covered.filter(live)
    const name = paneId ? (t.panes[paneId]?.name || t.name) : t.name
    if (running.length === 0) { removeFromDoc(tabId, paneId); return }
    closing = { tab: tabId, pane: paneId, name: name || 'Untitled', host: machineById.get(running[0].host)?.host ?? 'its machine', live: running }
  }
  function removeFromDoc(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    // The last pane of a tab takes the tab with it.
    if (!paneId || Object.keys(t.panes).length <= 1) change({ kind: 'closeTab', tab: tabId })
    else change({ kind: 'closePane', tab: tabId, pane: paneId })
  }
  async function endAndClose() {
    const c = closing
    closing = null
    if (!c) return
    removeFromDoc(c.tab, c.pane)
    const results = await Promise.allSettled(c.live.map(p => endTerminal(p.session, crypto.randomUUID(), true)))
    const failed = results.filter(r => r.status === 'rejected')
    if (failed.length) notice = failed.length === 1 ? 'Its shell could not be ended. It is still in Sessions.' : `${failed.length} shells could not be ended. They are still in Sessions.`
    onrefresh()
  }
  function takeOff() {
    const c = closing
    closing = null
    if (c) removeFromDoc(c.tab, c.pane)
  }

  // Panes.
  function renamePane(t: Tab, p: Pane, name: string) {
    change({ kind: 'renamePane', tab: t.id, pane: p.id, name })
    // Sessions and the desk agree on the name.
    if (live(p)) renameTerminal(p.session, name).then(onrefresh, () => { /* the desk keeps its name; Sessions keeps the old one */ })
  }
  async function endPane(p: Pane) {
    if (!p.session) return
    try { await endTerminal(p.session, crypto.randomUUID(), false) } catch (e) { notice = e instanceof Error && e.message ? e.message : 'That shell could not be ended. Try again.' }
    onrefresh()
  }

  // New tab: a plain shell, then the tab that holds it.
  let adding = $state<HTMLElement | null>(null)
  const startAgent = $derived(activePane?.host ?? '')
  const startDir = $derived.by(() => { const s = activePane?.session ? sessionById.get(activePane.session) : undefined; return s?.dir || activePane?.dir || '' })
  async function createTab(agentId: string, dir: string) {
    // The tab goes to the desk the dialog was opened on, even if another desk is shown by the time the shell starts.
    const mine = auto
    const name = [...(dir.split('/').filter(Boolean).pop() || machineById.get(agentId)?.host || 'Shell')].slice(0, 80).join('')
    const created = await createTerminal(agentId, crypto.randomUUID(), name, 100, 30, AbortSignal.timeout(15000), dir ? { dir } : undefined)
    const pid = newId()
    const pane: Pane = { id: pid, name, colour: '', host: agentId, session: created.id, kind: 'shell', dir: created.dir || dir, dirAt: Date.now(), start: '' }
    const tab: Tab = { id: newId(), name, colour: '', root: { pane: pid }, zoomed: '', active: pid, panes: { [pid]: pane } }
    fresh = [...fresh, created]
    onrefresh()
    if (!mine || auto !== mine) { notice = `The shell “${name}” started, but its desk is no longer open. It is in Sessions.`; return }
    change({ kind: 'addTab', tab })
    change({ kind: 'active', tab: tab.id })
  }
  async function closeNewTab() { const opener = adding; adding = null; await tick(); opener?.focus() }

  // The split area. A phone shows one pane at a time and offers no splits, presets or zoom; the saved tree is untouched.
  let phone = $state(false)
  let split = $state<SplitArea>()
  let section = $state<HTMLElement>()
  const ids = $derived(activeTab ? leaves(activeTab.root) : [])
  const zoomed = $derived(activeTab && !phone && ids.includes(activeTab.zoomed) && ids.length > 1 ? activeTab.zoomed : '')
  const paneName = (p: Pane | undefined) => p?.name || (p?.session ? sessionById.get(p.session)?.title : '') || 'Shell'
  // Each mounted terminal's toolbar actions, so the one toolbar acts on the focused pane.
  let actionsFor = $state<Record<string, TerminalActions | undefined>>({})
  function noteActions(paneId: string, a: TerminalActions | undefined) { if (actionsFor[paneId] !== a) actionsFor[paneId] = a }

  function setActive(paneId: string) {
    if (activeTab && activeTab.panes[paneId] && paneId !== activePaneId) change({ kind: 'active', tab: activeTab.id, pane: paneId })
  }
  /** Focus moves to a pane: it becomes the tab's active pane and its terminal (or, with none, its menu) takes the keyboard. */
  async function focusPane(paneId: string) {
    setActive(paneId)
    await tick()
    const a = actionsFor[paneId]
    if (a) a.focus()
    else section?.querySelector<HTMLElement>(`[data-pane="${CSS.escape(paneId)}"] .more`)?.focus()
  }

  // Split: a new plain shell on the focused pane's machine, in the folder its shell is in now, beside it.
  let splitting = false
  async function splitPane(dir: Split, paneId = activePaneId) {
    const t = activeTab, p = t?.panes[paneId]
    if (!t || !p || phone || splitting) return
    if (ids.length >= MAX_PANES) { notice = 'A tab holds at most nine panes.'; return }
    if (split && !split.roomFor(p.id, dir)) { notice = "There isn't room to split this pane."; return }
    splitting = true
    const mine = auto
    try {
      const s = p.session ? sessionById.get(p.session) : undefined
      const where = s?.liveDir || p.dir
      const name = [...(where.split('/').filter(Boolean).pop() || machineById.get(p.host)?.host || 'Shell')].slice(0, 80).join('')
      const created = await createTerminal(p.host, crypto.randomUUID(), name, 100, 30, AbortSignal.timeout(15000), where ? { dir: where } : undefined)
      fresh = [...fresh, created]
      onrefresh()
      if (!mine || auto !== mine) { notice = `The shell “${name}” started, but its desk is no longer open. It is in Sessions.`; return }
      // Pane ids are unique across the whole desk.
      const pane: Pane = { id: newId(), name, colour: '', host: p.host, session: created.id, kind: 'shell', dir: created.dir || where, dirAt: Date.now(), start: '' }
      if (t.zoomed) change({ kind: 'zoom', tab: t.id, pane: '' })
      change({ kind: 'addPane', tab: t.id, at: p.id, dir, pane })
      change({ kind: 'active', tab: t.id, pane: pane.id })
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      notice = e instanceof Error && e.message ? e.message : 'A new shell could not be started just now. Try again.'
    } finally { splitting = false }
  }
  function zoom(paneId = activePaneId) {
    if (!activeTab || phone || ids.length < 2) return
    change({ kind: 'zoom', tab: activeTab.id, pane: zoomed === paneId ? '' : paneId })
    if (zoomed !== paneId) setActive(paneId)
  }
  function moveToTab(from: Tab, paneId: string, toId: string) {
    const p = from.panes[paneId], to = tabs.find(t => t.id === toId)
    if (!p || !to) return
    const at = to.panes[to.active] ? to.active : leaves(to.root)[0]
    if (!at) return
    change({ kind: 'addPane', tab: to.id, at, dir: 'right', pane: p })
    // Only once it is in the other tab does it leave this one; the last pane of a tab takes the tab with it.
    if (!desk?.doc.tabs.find(t => t.id === to.id)?.panes[paneId]) { notice = 'That pane could not be moved there.'; return }
    if (leaves(from.root).length <= 1) change({ kind: 'closeTab', tab: from.id })
    else change({ kind: 'closePane', tab: from.id, pane: paneId })
    change({ kind: 'active', tab: to.id, pane: paneId })
  }

  // Attaches for a tab start at most three at a time, the focused pane first; each pane's attach then keeps its own
  // backoff. A slot is given back once the attach has an answer, or after a while whatever happens.
  const ATTACHING = 3, SLOT_MS = 10_000
  let admitted = $state<ReadonlySet<string>>(new Set())
  let attaching = new Set<string>(), queueTab = ''
  const wanted = $derived(activeTab ? [activePaneId, ...ids.filter(x => x !== activePaneId)].filter(x => { const p = activeTab.panes[x]; return !!p && live(p) }) : [])
  function pump(list: string[]) {
    let next: Set<string> | null = null
    for (const pid of list) {
      if (attaching.size >= ATTACHING) break
      if (admitted.has(pid) || next?.has(pid)) continue
      next ??= new Set(admitted)
      next.add(pid); attaching.add(pid)
      setTimeout(() => settle(pid), SLOT_MS)
    }
    if (next) admitted = next
  }
  function settle(pid: string) { if (attaching.delete(pid)) pump(untrack(() => wanted)) }
  $effect(() => {
    const tabId = activeTab?.id ?? '', list = wanted
    untrack(() => {
      if (tabId !== queueTab) { queueTab = tabId; attaching = new Set(); admitted = new Set() }
      pump(list)
    })
  })

  // Presets and the layout menu.
  const PRESETS: { name: 'side' | 'stack' | 'grid' | 'main' | 'even'; label: string }[] = [
    { name: 'side', label: 'Side by side' }, { name: 'stack', label: 'Stacked' }, { name: 'grid', label: 'Grid' }, { name: 'main', label: 'Main and stack' }, { name: 'even', label: 'Even out' },
  ]
  function applyPreset(name: (typeof PRESETS)[number]['name']) {
    closeMenu(true)
    if (activeTab) change({ kind: 'preset', tab: activeTab.id, name })
  }
  // The Layout button's picture: the tab's panes as a small grid of cells.
  const icon = $derived(zoomed || !activeTab?.root ? [{ x: 0, y: 0, w: 1, h: 1 }] : [...treeLayout(activeTab.root).values()])

  // Theatre view: the top bar and the desk header fold away, leaving the tab bar, the toolbar and the panes.
  let theatre = $state(false)
  $effect(() => { document.documentElement.toggleAttribute('data-desk-theatre', theatre) })

  // The Keys list.
  let keysOpen = $state(false), keysDialog = $state<HTMLDialogElement>()
  $effect(() => { if (keysOpen) keysDialog?.showModal() })

  // The desk's keys, caught in the capture phase so the terminal never sees them. None is a plain shell key.
  function onKey(event: KeyboardEvent) {
    const k = deskKey(event)
    if (!k || !desk || event.repeat && typeof k !== 'string') return
    if (closing || adding || keysOpen || document.querySelector('dialog[open]')) return
    event.preventDefault()
    event.stopPropagation()
    if (typeof k === 'object') { const t = tabs[k.tab - 1]; if (t) select(t.id); return }
    const at = activeTab ? tabs.indexOf(activeTab) : -1
    switch (k) {
      case 'splitRight': void splitPane('right'); break
      case 'splitDown': void splitPane('down'); break
      case 'zoom': zoom(); break
      case 'closePane': if (activeTab && activePaneId) askClose(activeTab.id, activePaneId); break
      case 'newTab': if (tabs.length < MAX_TABS) adding = section?.querySelector<HTMLElement>('[aria-label="New tab"]') ?? section ?? null; break
      case 'prevTab': if (tabs.length > 1) select(tabs[(at - 1 + tabs.length) % tabs.length].id); break
      case 'nextTab': if (tabs.length > 1) select(tabs[(at + 1) % tabs.length].id); break
      default: {
        if (!activeTab?.root || !activePaneId || zoomed) return
        const dir = k === 'focusLeft' ? 'left' : k === 'focusRight' ? 'right' : k === 'focusUp' ? 'up' : 'down'
        const order = phone ? ids : null
        const to = order ? order[(order.indexOf(activePaneId) + (dir === 'left' || dir === 'up' ? -1 : 1) + order.length) % order.length] : neighbour(activeTab.root, activePaneId, dir)
        if (to && to !== activePaneId) void focusPane(to)
      }
    }
  }
</script>

<svelte:window onkeydowncapture={onKey} />

<section class="desk-screen" class:theatre bind:this={section} aria-label={desk ? `Desk ${desk.name}` : 'Desk'}>
  {#if failure}
    <div class="gone"><p role="alert">{failure}</p><button type="button" class="btn" onclick={onlist}>All desks</button></div>
  {:else if !desk}
    <div class="dhead"><span class="loading" role="status">Opening the desk…</span></div>
  {:else}
    <header class="dhead" data-keeps-focus>
      <button type="button" class="dname" aria-haspopup="menu" aria-expanded={menu?.kind === 'switch'} aria-label={`${desk.name}, switch desk`} onclick={event => openMenu('switch', event)}>
        <span class="sw" style={`--swatch:${projectColourHex(desk.colour)}`} aria-hidden="true"></span>
        <h1>{desk.name}</h1><span class="chev" aria-hidden="true">▾</span>
      </button>
      <span class="sub">{plural(tabs.length, 'tab')} · {plural(hosts.size, 'machine')} · <span class="save" data-save={saveState} role="status">{saveWord}</span></span>
      <span class="sp"></span>
      {#if !phone && activeTab}
        <button type="button" class="btn lay" aria-haspopup="menu" aria-expanded={menu?.kind === 'layout'} disabled={ids.length < 2} onclick={event => openMenu('layout', event)}>
          <span class="lay-icon" aria-hidden="true">{#each icon as r, i (i)}<i style={`left:${r.x * 100}%;top:${r.y * 100}%;width:${r.w * 100}%;height:${r.h * 100}%`}></i>{/each}</span>Layout
        </button>
      {/if}
      <button type="button" class="btn icon" aria-label="Desk menu" aria-haspopup="menu" aria-expanded={menu?.kind === 'desk'} onclick={event => openMenu('desk', event)}>⋮</button>
    </header>
    {#if saveState === 'stale'}<p class="notice" role="status">This desk was changed in another window. Showing the latest.</p>{/if}
    {#if notice}<p class="notice bad" role="alert">{notice} <button type="button" class="btn quiet" onclick={() => (notice = '')}>Dismiss</button></p>{/if}
    <DeskTabs {tabs} active={activeTab?.id ?? ''} {panelId} {waiting} full={tabs.length >= MAX_TABS}
      onselect={select}
      onrename={(tab, name) => change({ kind: 'renameTab', tab, name })}
      oncolour={(tab, colour) => change({ kind: 'colourTab', tab, colour })}
      onduplicate={duplicateTab}
      onclose={tabId => askClose(tabId)}
      onreorder={ids => change({ kind: 'reorderTabs', ids })}
      onnew={opener => (adding = opener)} />
    {#if phone && activeTab && ids.length > 1}
      <div class="chips" role="group" aria-label="Panes on this tab">
        {#each ids as pid (pid)}
          {@const p = activeTab.panes[pid]}
          <button type="button" class="chip" aria-pressed={pid === activePaneId} style={`--pc:${projectColourHex(p?.colour || activeTab.colour) || 'var(--line-strong)'}`} onclick={() => void focusPane(pid)}>{paneName(p)}</button>
        {/each}
      </div>
    {/if}
    {#if activeTab && activePane}
      <DeskToolbar name={paneName(activePane)} actions={actionsFor[activePaneId]} splits={!phone} zoomed={!!zoomed} {theatre}
        onsplit={dir => void splitPane(dir)} onzoom={() => zoom()} ontheatre={() => (theatre = !theatre)} />
    {/if}
    <div class="area" id={panelId} role="tabpanel" aria-labelledby={activeTab ? `desk-tab-${activeTab.id}` : undefined}>
      {#if activeTab && activePane}
        {@const t = activeTab}
        <SplitArea bind:this={split} tab={t} focused={activePaneId} {zoomed} single={phone}
          onratio={(path, ratio) => change({ kind: 'ratio', tab: t.id, path, ratio })}
          onmove={(paneId, onto, edge) => change({ kind: 'move', tab: t.id, pane: paneId, onto, edge })}>
          {#snippet pane(pid, view)}
            {@const p = t.panes[pid]}
            {#if p}
              <DeskPane tab={t} pane={p} session={p.session ? sessionById.get(p.session) : undefined} machine={machineById.get(p.host)} {ready}
                focused={pid === activePaneId} visible={view.visible} hold={view.hold} admitted={admitted.has(pid)} layout={!phone} zoomed={zoomed === pid} {tabs}
                onfocus={() => setActive(pid)}
                onactions={a => noteActions(pid, a)}
                onsettled={() => settle(pid)}
                onrename={name => renamePane(t, p, name)}
                oncolour={colour => change({ kind: 'colourPane', tab: t.id, pane: pid, colour })}
                onsplit={dir => { setActive(pid); void splitPane(dir, pid) }}
                onzoom={() => zoom(pid)}
                onswap={other => change({ kind: 'swap', tab: t.id, a: pid, b: other })}
                onmovetab={to => moveToTab(t, pid, to)}
                onclose={() => askClose(t.id, pid)}
                onend={() => void endPane(p)} />
            {/if}
          {/snippet}
        </SplitArea>
      {:else}
        <div class="empty">
          <h2>{tabs.length ? 'This tab is empty.' : 'Nowt on this desk yet.'}</h2>
          <p>A tab holds a shell on one of your machines, in the folder you choose.</p>
          <button type="button" class="btn primary" disabled={tabs.length >= MAX_TABS} onclick={event => (adding = event.currentTarget as HTMLElement)}>New tab</button>
        </div>
      {/if}
    </div>
  {/if}
</section>

{#if menu?.kind === 'switch' && desk}
  <DeskMenu x={menu.x} y={menu.y} label="Switch desk" opener={menu.opener} onclose={closeMenu}>
    {#each others as d (d.id)}
      <button type="button" role="menuitem" onclick={() => { closeMenu(false); onopen(d.id) }}><span class="swatch" style={`--swatch:${projectColourHex(d.colour)}`}></span>{d.name}</button>
    {/each}
    {#if others.length}<hr />{/if}
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); onlist() }}>All desks</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); onnew() }}>New desk</button>
  </DeskMenu>
{:else if menu?.kind === 'layout' && desk}
  <DeskMenu x={menu.x} y={menu.y} label="Layout" opener={menu.opener} onclose={closeMenu}>
    {#each PRESETS as item (item.name)}<button type="button" role="menuitem" onclick={() => applyPreset(item.name)}>{item.label}</button>{/each}
  </DeskMenu>
{:else if menu?.kind === 'desk' && desk}
  {@const current = desk}
  <DeskMenu x={menu.x} y={menu.y} label="Desk menu" opener={menu.opener} onclose={closeMenu} colour={current.colour} oncolour={c => void colourDesk(c)}>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('rename', current) }}>Rename</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('duplicate', current) }}>Duplicate</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('template', current) }}>Save as template</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); keysOpen = true }}>Keys</button>
    <button type="button" role="menuitem" class="danger" onclick={() => { closeMenu(false); ondesk('delete', current) }}>Delete</button>
  </DeskMenu>
{/if}

{#if keysOpen}
  <dialog bind:this={keysDialog} class="keys" use:shell={{ sheet: 'bottom' }} aria-labelledby={`${panelId}-keys`} onclose={() => (keysOpen = false)}>
    <div class="box" data-shell-card>
      <h2 id={`${panelId}-keys`}>Keys</h2>
      <p>They work anywhere on a desk, in a terminal too. None of them is a key your shell uses.</p>
      <dl>{#each DESK_KEYS as item (item.keys)}<dt><kbd>{item.keys}</kbd></dt><dd>{item.label}</dd>{/each}</dl>
      <div class="actions" data-shell-foot><button type="button" class="btn primary" onclick={() => keysDialog?.close()}>Done</button></div>
    </div>
  </dialog>
{/if}

{#if closing}
  <ClosePaneDialog name={closing.name} host={closing.host} count={closing.live.length} onend={() => void endAndClose()} ontakeoff={takeOff} oncancel={() => (closing = null)} />
{/if}

{#if adding}
  <NewTabDialog {machines} agentId={startAgent} dir={startDir} oncreate={createTab} onclose={() => void closeNewTab()} />
{/if}

<style>
  .desk-screen { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); }
  .dhead { flex: none; display: flex; align-items: center; gap: 12px; padding: 8px 14px; border-bottom: 1px solid var(--line-soft); min-height: 52px; }
  .dname { display: flex; align-items: center; gap: 8px; min-height: 40px; min-width: 0; padding: 4px 8px; margin-left: -8px; border: 0; border-radius: 8px; background: none; color: var(--ink); cursor: pointer; }
  .dname:hover { background: var(--line-soft); }
  .dname h1 { margin: 0; font: 600 1.0625rem/1.2 var(--sans); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chev { color: var(--muted); }
  .sw { width: 10px; height: 10px; border-radius: 3px; flex: none; background: var(--swatch); border: 1px solid var(--line-strong); }
  .sub { color: var(--faint); font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .save[data-save='unsaved'] { color: var(--warn); }
  .sp { margin-left: auto; }
  .btn.icon { min-width: 40px; padding: 6px 10px; }
  @media (pointer: coarse) { .dname, .btn.icon { min-height: 44px; } .btn.icon { min-width: 44px; } }
  .notice { flex: none; margin: 0; padding: 6px 14px; background: color-mix(in srgb, var(--info) 14%, var(--bg)); border-bottom: 1px solid var(--line-soft); color: var(--ink); font-size: 14px; }
  .notice.bad { background: color-mix(in srgb, var(--bad) 12%, var(--bg)); display: flex; align-items: center; gap: 8px; }
  .area { flex: 1; min-height: 0; display: flex; padding: 0; background: var(--line-soft); }
  .btn.lay { display: inline-flex; align-items: center; gap: 7px; }
  .lay-icon { position: relative; width: 16px; height: 12px; flex: none; }
  .lay-icon i { position: absolute; box-sizing: border-box; border: 0.75px solid var(--card); border-radius: 1.5px; background: currentColor; opacity: 0.85; }
  /* The phone's pane switcher: the pane names as chips under the tab bar. */
  .chips { flex: none; display: flex; gap: 6px; padding: 6px 10px; overflow-x: auto; border-bottom: 1px solid var(--line-soft); scrollbar-width: none; }
  .chip { flex: none; display: flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 12px; border: 1px solid var(--line); border-radius: 999px; background: var(--card); color: var(--muted); font: inherit; font-size: 14px; white-space: nowrap; cursor: pointer; }
  .chip::before { content: ''; width: 7px; height: 7px; border-radius: 50%; background: var(--pc); flex: none; }
  .chip[aria-pressed='true'] { border-color: var(--pc); color: var(--ink); background: color-mix(in srgb, var(--pc) 20%, var(--card)); }
  /* Theatre: the top bar (App's .site-header) and the desk header fold away; the tab bar, toolbar and panes stay. */
  .theatre .dhead { display: none; }
  :global(html[data-desk-theatre] .site-header) { display: none; }
  .keys { max-width: min(460px, calc(100vw - 32px)); padding: 0; border: 0; background: transparent; color: var(--ink); }
  .keys::backdrop { background: var(--scrim); }
  .keys .box { padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); }
  .keys h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); }
  .keys p { margin: 8px 0 0; color: var(--muted); }
  .keys dl { display: grid; grid-template-columns: auto 1fr; gap: 8px 16px; margin: 16px 0 0; }
  .keys dt, .keys dd { margin: 0; }
  .keys kbd { font: 14px var(--mono); color: var(--ink-soft); white-space: nowrap; }
  .keys .actions { display: flex; justify-content: flex-end; margin-top: 20px; }
  .empty { margin: auto; display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; padding: 24px; }
  .empty h2 { margin: 0; font: 600 1.375rem/1.2 var(--serif); }
  .empty p { margin: 0; color: var(--muted); max-width: 44ch; }
  .gone { margin: 48px auto; display: flex; flex-direction: column; align-items: center; gap: 12px; }
  .gone p { margin: 0; color: var(--muted); }
  .loading { color: var(--muted); }
  :global(html[data-desk-screen] #app) { height: 100dvh; min-height: 0; }
  :global(html[data-desk-screen] main#main-content) { min-height: 0; display: flex; flex-direction: column; overflow: hidden; padding-bottom: 0; }
  @media (max-width: 700px) {
    .dhead { padding: 6px 10px; gap: 8px; }
    .sub { display: none; }
    .area { padding: 0; }
  }
</style>
