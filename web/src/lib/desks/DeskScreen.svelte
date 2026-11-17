<script lang="ts" module>
  // Type in all panes asks once in a page's lifetime; after that the button turns it on at once. Never saved.
  let confirmedAll = false
</script>

<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus } from '../sessionStatus'
  import { createTerminal, endTerminal, listFolderProjects, renameTerminal, SESSIONS_PER_ACCOUNT, SESSIONS_PER_MACHINE, TerminalError, terminalClosed, validTerminalTitle, type FolderProject, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { canContinue, continueLaunch } from '../sessions'
  import { getDesk, patchDesk, reopenPane, StaleDesk, type Desk, type DeskSummary } from './api'
  import { agentName, isRunning, PANE_CARDS, paneState, reopenable, type PaneActionKind, type PaneFailure } from './paneState'
  import { createAutosave, type Autosave, type Change, type SaveState } from './autosave'
  import type { TerminalActions } from '../terminal/TerminalPane.svelte'
  import { isPhone, screenShortSide } from '../terminal/panelLayout'
  import { browserViewport, fittedHeight, pageViewport } from '../terminal/sizing'
  import { shell } from '../dialogShell'
  import { layout as treeLayout, leaves, MAX_PANES, MAX_TABS, neighbour, newId, type Pane, type Split, type Tab } from './tree'
  import { DESK_KEYS, deskKey } from './keys'
  import { pasteNeedsReview } from '../terminal/paste'
  import { sendToAll, stopReason, STOP_CHANGED, type Mode, type Now } from './typeInAll'
  import ClosePaneDialog from './ClosePaneDialog.svelte'
  import CombineTabsDialog from './CombineTabsDialog.svelte'
  import DeskMenu from './DeskMenu.svelte'
  import DeskPane from './DeskPane.svelte'
  import DeskTabs from './DeskTabs.svelte'
  import DeskToolbar from './DeskToolbar.svelte'
  import NewTabDialog from './NewTabDialog.svelte'
  import AddSessionDialog from './AddSessionDialog.svelte'
  import StartAgentDialog from '../StartAgentDialog.svelte'
  import { paneFor, paneName as cleanName, putOnDesk, tabFor } from './addSession'
  import SplitArea from './SplitArea.svelte'

  // One desk: a header (the desk's name as its switcher, the save state, Layout and the desk menu), the tab bar, one
  // terminal toolbar for the focused pane and the active tab's panes in their splits. Every edit goes through autosave at
  // once and is saved after a short pause; a save that lost to another window is merged onto the newer copy. Opening a
  // desk starts nothing: a shell starts only from New tab, a split or a press on a pane's card (Reopen, Start, Reopen
  // all). The desk's keys are Alt+Shift chords, caught
  // before the terminal sees them.
  let { id, desks, sessions, machines, ready, removeOnClose = false, onopen, onlist, onnew, ondesk, onfromtemplate, onrefresh, onsignedout }: {
    id: string
    /** Every desk, for the switcher; read again every 15 seconds, so a newer version of this desk is read in. */
    desks: DeskSummary[]
    sessions: TerminalSession[]
    machines: TerminalMachine[]
    /** Sessions and machines have been read at least once. */
    ready: boolean
    /** The person's setting for a session that ends (Account › Profile): it leaves Sessions, or stays there as ended. End
     *  the shell follows it, as the Sessions End dialog does. */
    removeOnClose?: boolean
    onopen: (id: string) => void
    onlist: () => void
    onnew: () => void
    /** A desk-level action from the menu: the page owns its dialogs and writes. */
    ondesk: (action: 'rename' | 'duplicate' | 'template' | 'delete', desk: Desk) => void
    /** A template opened by its address: make a desk from it. */
    onfromtemplate: (desk: Desk) => void
    /** Read the sessions and machines again (after a shell is started or ended here). */
    onrefresh: () => void
    onsignedout: (e: unknown) => void
  } = $props()

  let desk = $state<Desk | null>(null)
  let saveState = $state<SaveState>('saved')
  let failure = $state('')
  let auto: Autosave | undefined
  const panelId = $props.id()
  const READ_FAILED = 'This desk could not be read just now. Try again in a moment.'

  function start(deskId: string) {
    // Whatever the desk being left still has waiting is sent (and retried) before that copy stops; its late answers are
    // ignored below.
    auto?.retire()
    // Nothing of the desk being left carries over: its dialogs, menus, offers, failures and notices go with it.
    desk = null; failure = ''; saveState = 'saved'; typeAll = null; askAll = false; localTab = readTab(deskId)
    offered = {}; failures = {}; closing = null; choosing = null; menu = null; reopenNotice = ''; notice = ''; reopening = {}
    adding = null; agentStart = null; addingRunning = null; combining = null
    continueIds.clear(); attempts.clear()
    const mine = createAutosave({
      load: () => getDesk(deskId),
      save: (d, expect, keepalive) => patchDesk(d.id, { expect, doc: d.doc }, { keepalive }),
      onState: s => { if (auto === mine) saveState = s },
      // A desk read after the first read failed is shown: the failure goes.
      onDesk: d => { if (auto === mine && d.schema <= 1) { const first = !desk; desk = d; if (failure) { failure = ''; if (first) openAt(d) } } },
      onRefused: m => { if (auto === mine) notice = m },
    })
    auto = mine
    mine.ready.then(() => {
      if (auto !== mine) return
      const loaded = mine.desk()
      // A desk written by a newer reet is neither shown nor overwritten.
      if (loaded.schema > 1) { mine.stop(); desk = null; failure = 'This desk was saved by a newer version of reet, so it is not shown here.'; return }
      desk = loaded
      openAt(loaded)
    }).catch(e => {
      if (auto !== mine) return
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      failure = e instanceof TerminalError && e.status === 404 ? 'That desk is not on this account any more.' : READ_FAILED
    })
  }
  $effect(() => { start(id) })
  /** `/desks/<id>#<pane>` (a session's desk chip in Sessions) opens the desk at that pane. */
  function openAt(loaded: Desk) {
    let want = ''
    try { want = decodeURIComponent(location.hash.slice(1)) } catch { /* a malformed fragment names no pane */ }
    if (!want) return
    history.replaceState(history.state, '', location.pathname + location.search)
    const t = loaded.doc.tabs.find(x => x.panes[want])
    if (!t) return
    showTab(t.id)
    if (t.active !== want) change({ kind: 'active', tab: t.id, pane: want })
  }
  export function reload() { void auto?.refresh().catch(() => { /* the next save, or the next read of the desk list, tries again */ }) }
  // The Desks page reads the desk list every 15 seconds while it is in view: when it lists this desk at a newer version
  // than the one held (another browser changed it), the desk is read again. Queued edits are kept and merged on top.
  $effect(() => {
    const listed = desks.find(d => d.id === id)?.version, held = desk?.version
    if (listed !== undefined && held !== undefined && listed > held) untrack(() => reload())
  })

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
    // An on-screen keyboard that overlays the page leaves the window's height as it was, so the screen is fitted to
    // what is left above it and the terminals keep their last rows and their keys bar in view.
    let fitFrame = 0
    const fit = () => {
      cancelAnimationFrame(fitFrame)
      fitFrame = requestAnimationFrame(() => {
        const reading = browserViewport(), height = fittedHeight(pageViewport.read(reading), reading)
        const root = document.documentElement
        if (height === null) root.style.removeProperty('--desk-fit')
        else {
          root.style.setProperty('--desk-fit', `${height}px`)
          // The browser may have panned the page to bring the caret into view; the fitted page has nothing to pan.
          if (reading.visual && reading.visual.offsetTop > 0) scrollTo(0, 0)
        }
      })
    }
    fit()
    addEventListener('resize', fit)
    visualViewport?.addEventListener('resize', fit)
    visualViewport?.addEventListener('scroll', fit)
    return () => {
      cancelAnimationFrame(fitFrame)
      removeEventListener('resize', fit)
      visualViewport?.removeEventListener('resize', fit)
      visualViewport?.removeEventListener('scroll', fit)
      document.documentElement.style.removeProperty('--desk-fit')
      removeEventListener('resize', measure)
      document.documentElement.removeAttribute('data-desk-theatre')
      removeEventListener('pagehide', leave)
      document.removeEventListener('visibilitychange', visible)
      document.documentElement.removeAttribute('data-desk-screen')
      const last = auto
      auto = undefined
      last?.retire()
    }
  })

  function change(c: Change) {
    if (!auto || !desk) return
    auto.change(c)
    desk = auto.desk()
  }

  const tabs = $derived(desk?.doc.tabs ?? [])
  // The tab shown is this window's own choice (kept for the window in sessionStorage), so switching tabs saves nothing
  // and moves no other window. A window with no choice yet opens on the tab the desk was last worked in.
  let localTab = $state('')
  const tabKey = (deskId: string) => `reet-desk-tab:${deskId}`
  const readTab = (deskId: string) => { try { return sessionStorage.getItem(tabKey(deskId)) ?? '' } catch { return '' } }
  function showTab(tabId: string) {
    localTab = tabId
    try { if (desk) sessionStorage.setItem(tabKey(desk.id), tabId) } catch { /* only a convenience */ }
  }
  const activeTab = $derived(tabs.find(t => t.id === localTab) ?? tabs.find(t => t.id === desk?.doc.active) ?? tabs[0])
  // The tab a window opens on (and the one that takes the place of a tab closed elsewhere) becomes its own choice at
  // once, so work in another window's tab never moves this one.
  $effect(() => { const shown = activeTab?.id; if (desk && shown && shown !== untrack(() => localTab)) showTab(shown) })
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
  // A template opened by its address is shown, but nothing on it starts a shell.
  const template = $derived(!!desk?.template)
  const saveWord = $derived(saveState === 'saving' ? 'Saving…' : saveState === 'unsaved' ? 'Not saved yet' : saveState === 'refused' ? 'last change not saved' : 'saved')
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`

  // Menus in the header: the switcher (the desk's name) and the desk menu (⋮).
  let menu = $state<{ kind: 'switch' | 'desk' | 'layout' | 'add'; x: number; y: number; opener: HTMLElement } | null>(null)
  function openMenu(kind: 'switch' | 'desk' | 'layout', event: MouseEvent) {
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { kind, opener, x: kind === 'switch' ? box.left : box.right - 220, y: box.bottom + 4 }
  }
  function pickAdd(kind: 'shell' | 'agent' | 'running') {
    const opener = menu?.opener
    closeMenu(false)
    if (!opener) return
    if (kind === 'shell') adding = opener
    else if (kind === 'agent') void openAgent(opener)
    else addingRunning = opener
  }
  /** The tab bar's +: New shell, Start an agent and Add a running session. */
  function openAdd(opener: HTMLElement) {
    if (template || tabs.length >= MAX_TABS) return
    const box = opener.getBoundingClientRect()
    menu = { kind: 'add', opener, x: box.left, y: box.bottom + 4 }
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
  function select(tabId: string) { if (tabId !== activeTab?.id && tabs.some(t => t.id === tabId)) showTab(tabId) }
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
  let closing = $state<{ tab: string; pane?: string; name: string; host: string; live: Pane[]; ended: Pane[] } | null>(null)
  let notice = $state('')
  /** Panes whose shell has already ended or been lost. */
  const ended = (p: Pane) => { const s = p.session ? sessionById.get(p.session) : undefined; return !!s && terminalClosed(s.state) }
  /** A pane that leaves the desk with its shell already ended: its machine forgets the pane's own command history, and the
   *  session stays in Sessions as it is. Nothing is said if that fails (the file is the person's own, on their machine). */
  function forget(panes: Pane[]) {
    for (const p of panes) void endTerminal(p.session, crypto.randomUUID(), false, '', true).catch(() => { /* see above */ })
  }
  function askClose(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    const covered = paneId ? [t.panes[paneId]].filter(Boolean) : panesOf(t)
    const running = covered.filter(live), done = covered.filter(ended)
    const name = paneId ? (t.panes[paneId]?.name || t.name) : t.name
    if (running.length === 0) { removeFromDoc(tabId, paneId); forget(done); return }
    closing = { tab: tabId, pane: paneId, name: name || 'Untitled', host: machineById.get(running[0].host)?.host ?? 'its machine', live: running, ended: done }
  }
  function removeFromDoc(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    // The last pane of a tab takes the tab with it.
    if (!paneId || Object.keys(t.panes).length <= 1) {
      // This window was showing it: the tab that takes its place is shown.
      if (activeTab?.id === tabId) { const rest = tabs.filter(x => x.id !== tabId), next = rest[Math.min(tabs.indexOf(t), rest.length - 1)]; if (next) showTab(next.id) }
      change({ kind: 'closeTab', tab: tabId })
    }
    else change({ kind: 'closePane', tab: tabId, pane: paneId })
  }
  async function endAndClose() {
    const c = closing
    closing = null
    if (!c) return
    removeFromDoc(c.tab, c.pane)
    forget(c.ended)
    // The pane is gone for good, so its machine also forgets the pane's own command history (Take off this desk does not).
    // Whether the session leaves Sessions is the person's own setting, as in the Sessions End dialog.
    const results = await Promise.allSettled(c.live.map(p => endTerminal(p.session, crypto.randomUUID(), removeOnClose, '', true)))
    const failed = results.filter(r => r.status === 'rejected')
    if (failed.length) notice = failed.length === 1 ? 'Its shell could not be ended. It is still in Sessions.' : `${failed.length} shells could not be ended. They are still in Sessions.`
    onrefresh()
  }
  function takeOff() {
    const c = closing
    closing = null
    if (!c) return
    removeFromDoc(c.tab, c.pane)
    // Only the running shells stay as they are; a pane of the tab whose shell had ended is gone for good.
    forget(c.ended)
  }

  // Panes.
  function renamePane(t: Tab, p: Pane, name: string) {
    change({ kind: 'renamePane', tab: t.id, pane: p.id, name })
    // Sessions and the desk agree on the name: a pane with a session renames it too; one with none keeps it on the desk.
    const s = p.session ? sessionById.get(p.session) : undefined
    if (!s || !validTerminalTitle(name)) return
    const mine = auto
    renameTerminal(s.id, name).then(onrefresh, e => {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      notice = 'The pane was renamed, but its session could not be. Try again.'
      // The two stay in step: the pane goes back to the session's name, unless it has been renamed again since.
      const now = paneById(p.id)
      if (auto === mine && now && now.pane.name === name && now.pane.session === s.id) change({ kind: 'renamePane', tab: now.tab.id, pane: p.id, name: s.title })
    })
  }

  // New tab: a plain shell, then the tab that holds it.
  let adding = $state<HTMLElement | null>(null)
  const startAgent = $derived(activePane?.host ?? '')
  // The folder the focused pane's shell is in now, else the pane's own, else the folder its shell started in.
  const startDir = $derived.by(() => { const s = activePane?.session ? sessionById.get(activePane.session) : undefined; return s?.liveDir || activePane?.dir || s?.dir || '' })
  // One request id and pane id per start (New tab on a machine and folder, a split of one pane one way), kept until the
  // shell has started: a create that timed out and is pressed again asks about the same start, so it can never start a
  // second shell. A definite refusal (a 4xx) forgets it, so the next press is a new start.
  const attempts = new Map<string, { request: string; pane: string }>()
  function attempt(key: string) {
    // One attempt of each kind: a New tab elsewhere, or a split of another pane or tab, is a new start.
    const kind = key.split('\n')[0]
    for (const k of [...attempts.keys()]) if (k !== key && k.split('\n')[0] === kind) attempts.delete(k)
    let a = attempts.get(key)
    if (!a) { a = { request: crypto.randomUUID(), pane: newId() }; attempts.set(key, a) }
    return a
  }
  const refused = (e: unknown) => e instanceof TerminalError && e.status >= 400 && e.status < 500
  /** The sentence for a new shell that could not start: a pane card's own words for a session limit and a machine that
   *  is not reporting, else the server's. */
  function startProblem(e: unknown, host: string): string {
    const where = machineById.get(host)?.host || 'That machine'
    if (e instanceof TerminalError && atLimit(e)) return limitSentence(limitKind(host), host)
    if (e instanceof TerminalError && e.status === 503) return `${where} is not reporting. Try again once it is back.`
    return e instanceof Error && e.message ? e.message : 'A new shell could not be started just now. Try again.'
  }
  function limitSentence(kind: PaneFailure, host: string): string {
    return PANE_CARDS.limit.heading({ host: machineById.get(host)?.host ?? '', dir: '', agent: '', perMachine: SESSIONS_PER_MACHINE, perAccount: SESSIONS_PER_ACCOUNT, accountLimit: kind === 'accountLimit', canContinue: false })
  }
  async function createTab(agentId: string, dir: string) {
    // The tab goes to the desk the dialog was opened on, even if another desk is shown by the time the shell starts.
    const mine = auto
    const name = cleanName(dir.split('/').filter(Boolean).pop() || machineById.get(agentId)?.host || 'Shell')
    // The session limits are the browser's to check, before anything is sent.
    const limit = limited(agentId)
    if (limit) throw new Error(limitSentence(limit, agentId))
    // The pane's id comes first, so the shell has the pane's own command history from its first command.
    const key = `tab\n${agentId}\n${dir}`, { request, pane: pid } = attempt(key)
    let created: TerminalSession
    try {
      created = await createTerminal(agentId, request, name, 100, 30, AbortSignal.timeout(15000), dir ? { dir, deskPane: pid } : { deskPane: pid })
    } catch (e) {
      if (refused(e)) attempts.delete(key)
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); throw e }
      throw new Error(startProblem(e, agentId))
    }
    attempts.delete(key)
    const pane: Pane = { id: pid, name, colour: '', host: agentId, session: created.id, kind: 'shell', dir: created.dir || dir, dirAt: Date.now(), start: '' }
    const tab: Tab = { id: newId(), name, colour: '', root: { pane: pid }, zoomed: '', active: pid, panes: { [pid]: pane } }
    fresh = [...fresh, created]
    onrefresh()
    if (!mine || auto !== mine) { notice = `The shell “${name}” started, but its desk is no longer open. It is in Sessions.`; return }
    change({ kind: 'addTab', tab })
    // The desk may have filled from another window while the shell started.
    if (!desk?.doc.tabs.some(x => x.id === tab.id)) { notice = `The shell “${name}” started, but there was no room for a tab. It is in Sessions.`; return }
    // Shown in this window, and saved with the tab as the one the desk was last worked in.
    showTab(tab.id)
    change({ kind: 'active', tab: tab.id })
  }
  async function closeNewTab() {
    // A New tab that is closed or cancelled is over: the next one is a new start.
    for (const k of [...attempts.keys()]) if (k.startsWith('tab\n')) attempts.delete(k)
    const opener = adding; adding = null; await tick(); opener?.focus()
  }

  // Start an agent… (the existing dialog, on the focused pane's machine, and its project when the pane is in one): the
  // session it starts is put in a new tab. An agent pane has no pane history, so no pane id is sent with it.
  let agentStart = $state<{ opener: HTMLElement; projects: FolderProject[]; projectId: string } | null>(null)
  async function openAgent(opener: HTMLElement) {
    const projects = await folderProjects() ?? []
    if (!desk || template) return
    const host = startAgent, dir = startDir
    agentStart = { opener, projects, projectId: projects.find(p => p.host === host && !!dir && p.dir === dir)?.id ?? '' }
  }
  function agentStarted(created: TerminalSession) {
    const mine = auto
    fresh = [...fresh.filter(x => x.id !== created.id), created]
    onrefresh()
    const pane = paneFor(created), t = tabFor(pane)
    // A chat is not a terminal, so a pane cannot show it.
    if (created.structured) { notice = `“${pane.name}” started as a chat, which a desk pane can't show. It is in Sessions.`; return }
    if (!mine || !desk) { notice = `“${pane.name}” started, but its desk is no longer open. It is in Sessions.`; return }
    change({ kind: 'addTab', tab: t })
    if (!desk.doc.tabs.some(x => x.id === t.id)) { notice = `“${pane.name}” started, but there was no room for a tab. It is in Sessions.`; return }
    // Shown in this window, and saved with the tab as the one the desk was last worked in.
    showTab(t.id)
    change({ kind: 'active', tab: t.id })
  }

  // Add a running session…: written straight to the saved desk (not through autosave, so a session the controller says
  // is on another desk is refused there, not retried), then read back.
  let addingRunning = $state<HTMLElement | null>(null)
  const onThisDesk = $derived(new Set(tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean)))
  const runningElsewhere = $derived([...sessionById.values()].filter(s => isRunning(s) && !s.structured && !onThisDesk.has(s.id)).sort((a, b) => b.updatedAt - a.updatedAt))
  async function addRunning(s: TerminalSession, from?: { id: string; name: string }) {
    const mine = auto, current = desk
    if (!mine || !current || template) return
    await mine.flush()
    if (auto !== mine) throw new Error('This desk is no longer open.')
    await putOnDesk(s, { id: current.id, name: current.name }, '', from)
    await mine.refresh().catch(() => { /* the next read of the desk list brings it in */ })
    const added = auto === mine ? desk?.doc.tabs.find(t => Object.values(t.panes).some(p => p.session === s.id)) : undefined
    if (added) showTab(added.id)
    onrefresh()
  }

  // The account's folder projects, read afresh each time they are needed (Continue, Start an agent); null when the read
  // failed, so a caller that needs them can say so rather than carry on without.
  async function folderProjects(): Promise<FolderProject[] | null> {
    try { return await listFolderProjects(AbortSignal.timeout(10_000)) } catch (e) {
      if (e instanceof TerminalError && e.status === 401) onsignedout(e)
      return null
    }
  }

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
    // Also when the pane is already its tab's focused one but the desk was last worked in another tab: working here
    // records this tab, so a new window opens on it.
    if (activeTab && activeTab.panes[paneId] && (paneId !== activePaneId || desk?.doc.active !== activeTab.id)) change({ kind: 'active', tab: activeTab.id, pane: paneId })
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
    if (!t || !p || phone || template || splitting) return
    if (ids.length >= MAX_PANES) { notice = 'A tab holds at most nine panes.'; return }
    if (split && !split.roomFor(p.id, dir)) { notice = "There isn't room to split this pane."; return }
    const limit = limited(p.host)
    if (limit) { notice = limitSentence(limit, p.host); return }
    splitting = true
    const mine = auto
    const key = `split\n${t.id}\n${p.id}\n${dir}`
    try {
      const s = p.session ? sessionById.get(p.session) : undefined
      const where = s?.liveDir || p.dir
      const name = cleanName(where.split('/').filter(Boolean).pop() || machineById.get(p.host)?.host || 'Shell')
      // Pane ids are unique across the whole desk; the id comes first so the shell starts with the pane's own history.
      const { request, pane: pid } = attempt(key)
      const created = await createTerminal(p.host, request, name, 100, 30, AbortSignal.timeout(15000), where ? { dir: where, deskPane: pid } : { deskPane: pid })
      attempts.delete(key)
      fresh = [...fresh, created]
      onrefresh()
      if (!mine || auto !== mine) { notice = `The shell “${name}” started, but its desk is no longer open. It is in Sessions.`; return }
      const pane: Pane = { id: pid, name, colour: '', host: p.host, session: created.id, kind: 'shell', dir: created.dir || where, dirAt: Date.now(), start: '' }
      if (t.zoomed) change({ kind: 'zoom', tab: t.id, pane: '' })
      change({ kind: 'addPane', tab: t.id, at: p.id, dir, pane })
      // The pane it was beside may have closed, or the tab filled from another window, while the shell started.
      if (!desk?.doc.tabs.find(x => x.id === t.id)?.panes[pane.id]) { notice = `The shell “${name}” started, but there was no longer room for it on this tab. It is in Sessions.`; return }
      change({ kind: 'active', tab: t.id, pane: pane.id })
    } catch (e) {
      if (refused(e)) attempts.delete(key)
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      notice = startProblem(e, p.host)
    } finally { splitting = false }
  }
  function zoom(paneId = activePaneId) {
    if (!activeTab || phone || ids.length < 2) return
    // Read before the change: `zoomed` follows the desk.
    const was = zoomed
    change({ kind: 'zoom', tab: activeTab.id, pane: was === paneId ? '' : paneId })
    if (was !== paneId) setActive(paneId)
  }
  function moveToTab(from: Tab, paneId: string, toId: string) {
    if (!from.panes[paneId] || !tabs.some(t => t.id === toId)) return
    // One change: the pane reaches the other tab and leaves this one together, or neither (here, or when the change is
    // merged onto a newer copy of the desk). The last pane of a tab takes the tab with it.
    change({ kind: 'movePaneToTab', from: from.id, to: toId, pane: paneId })
    if (desk?.doc.tabs.some(x => x.id === toId && x.panes[paneId])) showTab(toId)
    if (!desk?.doc.tabs.find(t => t.id === toId)?.panes[paneId]) notice = 'That pane could not be moved there.'
  }

  // Reopen: a fresh shell in a pane whose shell ended or was lost, through the desk's reopen route, which binds the new
  // session into the pane and answers with the desk. Pending edits are saved first so the version sent is current; the
  // desk is then read again so autosave holds the newer copy. The session limits are the browser's to check, before
  // anything is sent.
  let reopening = $state<Record<string, boolean>>({})
  /** Why a pane's last try was refused, and the session the pane had then: a pane whose session changes since (another
   *  window reopened it, or this one did) drops it, so a stale reason never outlives the fix. */
  type Failure = { kind?: PaneFailure; message?: string; home?: boolean; resume?: boolean; session: string }
  let failures = $state<Record<string, Failure>>({})
  let reopenNotice = $state('')
  const paneById = (pid: string) => { for (const t of tabs) if (t.panes[pid]) return { tab: t, pane: t.panes[pid] }; return undefined }
  // Every running session, a shell started here and not yet listed included (each counted once: `fresh` and the list
  // are merged by id).
  const runningOn = (host?: string) => [...sessionById.values()].filter(s => isRunning(s) && (!host || s.agentId === host)).length
  const REASONS: Record<PaneFailure, string> = { folderGone: 'folder gone', folderUnavailable: 'folder unavailable', machineLimit: 'at the limit', accountLimit: 'at the limit', offline: 'machine offline', machineGone: 'machine removed', projectsUnread: 'projects unread' }
  const CODES: Record<string, PaneFailure> = { folder_gone: 'folderGone', folder_unavailable: 'folderUnavailable', machine_gone: 'machineGone', machine_offline: 'offline' }
  function setFailure(pid: string, f: Omit<Failure, 'session'> | undefined) {
    const next = { ...failures }
    if (f) next[pid] = { ...f, session: paneById(pid)?.pane.session ?? '' }; else delete next[pid]
    failures = next
  }
  $effect(() => {
    // The tabs are read here, not only inside the filter: with no failure stored the filter reads nothing, and the effect
    // would never run again.
    const current = tabs
    const now = untrack(() => failures)
    const paneIn = (pid: string) => { for (const t of current) if (t.panes[pid]) return t.panes[pid]; return undefined }
    const keep = Object.entries(now).filter(([pid, f]) => { const p = paneIn(pid); return !!p && p.session === f.session })
    if (keep.length !== Object.keys(now).length) failures = Object.fromEntries(keep)
  })
  function setBusy(pid: string, on: boolean) {
    const next = { ...reopening }
    if (on) next[pid] = true; else delete next[pid]
    reopening = next
  }
  /** A refusal at a session limit. The server says the same words for the machine's limit and the account's, so the
   *  sessions this page knows of say which: a full machine, else a full account, else whichever is nearer its limit
   *  (sessions started in another window may not be listed here yet). */
  const atLimit = (e: TerminalError) => e.status === 409 && e.message === 'terminal session limit reached'
  function limitKind(host: string): PaneFailure {
    const onMachine = runningOn(host), all = runningOn()
    if (onMachine >= SESSIONS_PER_MACHINE) return 'machineLimit'
    if (all >= SESSIONS_PER_ACCOUNT) return 'accountLimit'
    return all / SESSIONS_PER_ACCOUNT > onMachine / SESSIONS_PER_MACHINE ? 'accountLimit' : 'machineLimit'
  }
  /** The limits, checked before anything is sent: the reason a new shell may not start, or undefined. */
  function limited(host: string): PaneFailure | undefined {
    if (runningOn() >= SESSIONS_PER_ACCOUNT) return 'accountLimit'
    if (runningOn(host) >= SESSIONS_PER_MACHINE) return 'machineLimit'
    return undefined
  }
  // The start-up command is offered only in a pane this page reopened or started, for that new session: never on load.
  let offered = $state<Record<string, string>>({})
  function offerDone(pid: string) { const next = { ...offered }; delete next[pid]; offered = next }
  /** Reads the desk again until it holds the session the route bound into the pane. Until then the pane stays busy, so
   *  a second press cannot send a second reopen. It gives up after about half a minute; the desk list, read every 15
   *  seconds, then brings the newer desk in. */
  async function adopt(mine: Autosave, pid: string, session: string) {
    for (let wait = 500, tries = 0; tries < 7; wait = Math.min(wait * 2, 8000), tries++) {
      if (auto !== mine) return
      try { await mine.refresh() } catch { /* tried again below */ }
      const p = paneById(pid)?.pane
      if (!p || p.session === session || auto !== mine) return
      await new Promise(resolve => setTimeout(resolve, wait))
    }
  }
  /** Reopens one pane. `home` opens it in the machine's home folder. Answers '' once a shell started, 'live' when the
   *  pane turned out to have one already (nothing started), or the reason it was not reopened. */
  async function reopen(pid: string, opts: { home?: boolean } = {}): Promise<string> {
    const mine = auto, found = paneById(pid)
    if (!mine || !found || reopening[pid]) return 'busy'
    const { pane: p } = found
    const home = opts.home ?? false
    const limit = limited(p.host)
    if (limit) { setFailure(pid, { kind: limit, home }); return REASONS[limit] }
    setBusy(pid, true)
    setFailure(pid, undefined)
    try {
      await mine.flush()
      for (let left = 1; ; left--) {
        try {
          const done = await reopenPane(mine.desk().id, pid, { expect: mine.desk().version, ...(home ? { home: true } : {}) })
          fresh = [...fresh.filter(s => s.id !== done.session.id), done.session]
          offered = { ...offered, [pid]: done.session.id }
          onrefresh()
          await adopt(mine, pid, done.session.id)
          return ''
        } catch (e) {
          if (!(e instanceof StaleDesk) || left <= 0 || auto !== mine) throw e
          // The desk moved on: read it again and try once more with its version.
          await mine.refresh()
        }
      }
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return 'signed out' }
      // Another window reopened it first: take the desk as it is now; nothing was started here.
      if (e instanceof TerminalError && e.code === 'pane_live') { onrefresh(); await mine.refresh().catch(() => {}); return 'live' }
      const kind = e instanceof TerminalError ? (CODES[e.code ?? ''] ?? (atLimit(e) ? limitKind(p.host) : undefined)) : undefined
      if (kind) { setFailure(pid, { kind, home }); return REASONS[kind] }
      const message = e instanceof StaleDesk ? 'This desk changed in another window. Try again.' : e instanceof Error && e.message ? e.message : 'A new shell could not be started just now. Try again.'
      setFailure(pid, { message, home })
      return 'not started'
    } finally { setBusy(pid, false) }
  }

  // Continue: an agent pane whose agent ended, where the agent's own resume is on offer (the Sessions screen's rule and
  // its continue launch). The new session is bound into the pane through autosave.
  const continuable = (p: Pane) => {
    const s = p.session ? sessionById.get(p.session) : undefined
    return !!s && canContinue(s, machineById.get(p.host), [])
  }
  // One request id per pane and the session it continues, kept until the pane holds the new session: a create that timed
  // out and is pressed again asks about the same start, so it can never start a second agent session.
  const continueIds = new Map<string, { from: string; id: string }>()
  async function continuePane(pid: string) {
    const mine = auto, found = paneById(pid)
    if (!mine || !found || reopening[pid]) return
    const s = found.pane.session ? sessionById.get(found.pane.session) : undefined
    if (!s || !continuable(found.pane)) return
    const limit = limited(found.pane.host)
    if (limit) { setFailure(pid, { kind: limit, resume: true }); return }
    setBusy(pid, true)
    setFailure(pid, undefined)
    try {
      // The account's folder projects go with it, so a continued session keeps its project. A session in a project is not
      // continued without them: it would lose its project, silently.
      const projects = await folderProjects()
      if (!projects && s.projectId) { setFailure(pid, { kind: 'projectsUnread', resume: true }); return }
      const launch = continueLaunch(s, projects ?? [])
      if (!launch) return
      let op = continueIds.get(pid)
      if (!op || op.from !== s.id) { op = { from: s.id, id: crypto.randomUUID() }; continueIds.set(pid, op) }
      const title = cleanName(s.title || found.pane.name || agentName(s.tool))
      const created = await createTerminal(s.agentId, op.id, title, 100, 30, AbortSignal.timeout(15000), launch)
      fresh = [...fresh.filter(x => x.id !== created.id), created]
      onrefresh()
      const now = paneById(pid)
      if (auto !== mine || !now) { notice = `“${title}” continued, but its pane is no longer on this desk. It is in Sessions.`; return }
      change({ kind: 'setSession', tab: now.tab.id, pane: pid, session: created.id })
      if (paneById(pid)?.pane.session !== created.id) { notice = `“${title}” continued, but this pane could not hold it. It is in Sessions.`; return }
      continueIds.delete(pid)
      await mine.flush()
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      // A session limit is the limit card, whose Try again continues again.
      if (e instanceof TerminalError && atLimit(e)) { setFailure(pid, { kind: limitKind(found.pane.host), resume: true }); return }
      setFailure(pid, { message: e instanceof Error && e.message ? e.message : 'The session could not be continued. Try again.', resume: true })
    } finally { setBusy(pid, false) }
  }

  /** A press on a pane's card. */
  function paneAct(pid: string, kind: PaneActionKind) {
    // Start for a pane that has no folder opens in the home folder.
    if (kind === 'start') void reopen(pid, { home: !paneById(pid)?.pane.dir })
    else if (kind === 'reopen') void reopen(pid)
    else if (kind === 'reopenHome') void reopen(pid, { home: true })
    else if (kind === 'again' && failures[pid]?.resume) void continuePane(pid)
    else if (kind === 'again') void reopen(pid, { home: failures[pid]?.home })
    else if (kind === 'continue') void continuePane(pid)
    else if (kind === 'chooseFolder' || kind === 'chooseMachine') choosing = { pane: pid, machine: kind === 'chooseMachine' }
  }

  // Choose a folder (on the pane's machine) or a machine and a folder: the pane is given them through autosave, saved,
  // then reopened there. A folder chosen here is under the home rule, which is what the folder browser offers.
  let choosing = $state<{ pane: string; machine: boolean } | null>(null)
  async function choose(pid: string, host: string, dir: string) {
    const mine = auto, found = paneById(pid)
    if (!mine || !found) return
    change({ kind: 'setPlace', tab: found.tab.id, pane: pid, host, dir })
    await mine.flush()
    // The reopen starts the shell where the saved desk says, so the new place must be saved first.
    const saved = mine.desk().doc.tabs.find(t => t.panes[pid])?.panes[pid]
    if (auto !== mine || saveState !== 'saved' || saved?.host !== host || saved.dir !== dir) throw new Error('That folder could not be saved just now. Try again.')
    const why = await reopen(pid)
    if (why && why !== 'live' && failures[pid]?.message) throw new Error(failures[pid].message)
  }

  // Reopen all: every lost or ended shell on the desk, any tab, one at a time (each reopen moves the desk's version on,
  // so reopens side by side would only lose to each other). Only on the press.
  const reopenList = $derived(!ready || desk?.template ? [] : tabs.flatMap(t => panesOf(t)).filter(p => {
    if (failures[p.id]?.kind || reopening[p.id]) return false
    const m = machineById.get(p.host)
    const s = p.session ? sessionById.get(p.session) : undefined
    return reopenable(paneState(p, s, m ? { online: m.ready } : undefined, { conflict: false, down: false }))
  }))
  let reopeningAll = $state(false)
  async function reopenAll() {
    if (reopeningAll) return
    const list = reopenList.map(p => ({ id: p.id, name: p.name || 'Shell' }))
    if (!list.length) return
    reopeningAll = true; reopenNotice = ''
    const failed: string[] = []
    let ok = 0
    try {
      for (const item of list) {
        const why = await reopen(item.id)
        if (why === '') ok++
        else if (why !== 'live' && why !== 'busy') failed.push(`${item.name} (${why})`)
      }
    } finally { reopeningAll = false }
    reopenNotice = failed.length ? `Reopened ${ok}. ${failed.length} could not be reopened: ${failed.join(', ')}.` : `Reopened ${ok}.`
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

  // Type in all panes: what is typed, pasted or sent from Compose in the focused pane also goes to every other pane of
  // the tab that is live, on screen and connected. Per tab and never saved: off on load and on a tab or desk change, and
  // it stops itself whenever the panes receiving change (a pane drops or is taken over, a shell ends, the tab's panes are
  // split, closed, moved or zoomed, here or in another window). Each input checks the receivers again before it goes on.
  let receivable = $state<Record<string, boolean>>({})
  function noteReceivable(pid: string, ok: boolean) { if (!!untrack(() => receivable[pid]) !== ok) receivable = { ...untrack(() => receivable), [pid]: ok } }
  const receivers = $derived(phone || !activeTab ? [] : ids.filter(pid => {
    const p = activeTab.panes[pid]
    return !!p && live(p) && !!receivable[pid] && (!zoomed || zoomed === pid) && !actionsFor[pid]?.scrollback
  }))
  /** The tab's panes and splits without the ratios, and the zoomed pane: a separator drag changes nothing here. */
  const treeShape = (n: Tab['root']): string => !n ? '' : 'split' in n ? `(${n.split} ${treeShape(n.a)} ${treeShape(n.b)})` : n.pane
  const shapeNow = $derived(activeTab ? `${treeShape(activeTab.root)}|${zoomed}` : '')
  let typeAll = $state<(Mode & { desk: string; since: number }) | null>(null)
  let askAll = $state(false), allButton = $state<HTMLButtonElement>(), askCancel = $state<HTMLButtonElement>()
  const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every(x => b.includes(x))
  /** The tab as it is at this moment, for the gate (typeInAll.ts): each pane's terminal is read when the gate asks. */
  function tabNow(): Now {
    const t = activeTab
    return {
      tab: t?.id ?? '', shape: shapeNow, receivers,
      present: pid => !!t?.panes[pid] && ids.includes(pid),
      pane: pid => {
        const a = actionsFor[pid]
        return a ? { canSend: () => a.canSend(), scrollback: () => a.scrollback, bracketed: () => a.bracketed, send: (data, paste) => a.send(data, paste) } : undefined
      },
      name: pid => paneName(t?.panes[pid]),
    }
  }
  const review = (text: string, bracketed: boolean) => pasteNeedsReview(text, { bracketed }).reason
  function stopAll(why = '') {
    const on = typeAll
    if (!on) return
    typeAll = null
    notice = why || stopReason(on, tabNow()) || STOP_CHANGED
  }
  $effect(() => {
    const on = typeAll
    if (!on) return
    const deskId = desk?.id, tabId = activeTab?.id, now = receivers, shape = shapeNow, small = phone
    untrack(() => {
      // Changing tab or desk ends it quietly: the stripe says so.
      if (deskId !== on.desk || tabId !== on.tab || small) typeAll = null
      else if (shape !== on.shape || !sameSet(now, on.set)) stopAll()
    })
  })
  // The question goes with the tab it was asked about, and once there are not two panes to type in.
  let askTab = ''
  $effect(() => { const tabId = activeTab?.id ?? '', n = receivers.length; if (untrack(() => askAll) && (tabId !== askTab || n < 2)) askAll = false })
  function toggleAll() {
    if (typeAll) { typeAll = null; return }
    if (receivers.length < 2) return
    if (confirmedAll) { startAll(); return }
    askTab = activeTab?.id ?? ''
    askAll = true
    void tick().then(() => askCancel?.focus())
  }
  function closeAsk() { askAll = false; allButton?.focus() }
  function startAll() {
    askAll = false
    if (!desk || !activeTab || template || receivers.length < 2) return
    confirmedAll = true
    typeAll = { desk: desk.id, tab: activeTab.id, set: [...receivers], shape: shapeNow, since: performance.now() }
    void focusPane(receivers.includes(activePaneId) ? activePaneId : receivers[0])
  }
  /** Input a pane has just sent to its own shell: from the focused pane, the gate sends it on to the other receivers,
   *  checking each of them as it is now. Input made before the mode was turned on (a paste held for review) stays put. */
  function typeInAll(from: string, data: string, paste: boolean, at: number) {
    const on = typeAll
    if (!on || from !== activePaneId || at < on.since) return
    const out = sendToAll(on, tabNow(), from, data, paste, review)
    if (out.stop) stopAll(out.stop)
  }

  // Presets and the layout menu.
  const PRESETS: { name: 'side' | 'stack' | 'grid' | 'main' | 'even'; label: string }[] = [
    { name: 'side', label: 'Side by side' }, { name: 'stack', label: 'Stacked' }, { name: 'grid', label: 'Grid' }, { name: 'main', label: 'Main and stack' }, { name: 'even', label: 'Even out' },
  ]
  function applyPreset(name: (typeof PRESETS)[number]['name']) {
    closeMenu(true)
    if (activeTab) change({ kind: 'preset', tab: activeTab.id, name })
  }
  // Combining tabs: the Layout menu brings the panes of other tabs into this one (all of them, or the ones ticked in
  // Combine tabs…). A tab is taken whole, and only while its panes fit. On a tab of one pane, which has nothing to
  // arrange, these are all the menu offers.
  const GATHER: { name: 'side' | 'stack' | 'grid'; label: string }[] = [
    { name: 'side', label: 'Show all tabs side by side' }, { name: 'stack', label: 'Show all tabs stacked' }, { name: 'grid', label: 'Show all tabs as a grid' },
  ]
  const otherTabs = $derived(activeTab ? tabs.filter(t => t.id !== activeTab.id && leaves(t.root).length > 0).map(t => ({ id: t.id, name: t.name || 'Untitled', panes: leaves(t.root).length })) : [])
  const gatherable = $derived(otherTabs.some(t => ids.length + t.panes <= MAX_PANES))
  let combining = $state<HTMLElement | null>(null)
  function gather(name: (typeof GATHER)[number]['name'], from?: string[]) {
    const t = activeTab
    if (!t) return
    const wanted = from ?? otherTabs.map(x => x.id)
    change({ kind: 'gather', tab: t.id, name, from })
    const left = wanted.filter(id => desk?.doc.tabs.some(x => x.id === id)).length
    if (left === wanted.length) notice = 'Those tabs could not be brought into this one.'
    else if (left > 0) notice = `${left === 1 ? 'One tab' : `${left} tabs`} did not fit: a tab holds at most ${MAX_PANES} panes.`
  }
  async function closeCombine() { const opener = combining; combining = null; await tick(); opener?.focus() }
  // The Layout button's picture: the tab's panes as a small grid of cells.
  const icon = $derived(zoomed || !activeTab?.root ? [{ x: 0, y: 0, w: 1, h: 1 }] : [...treeLayout(activeTab.root).values()])

  // Theatre view: the top bar and the desk header fold away, leaving the tab bar, the toolbar and the panes.
  let theatre = $state(false)
  $effect(() => { document.documentElement.toggleAttribute('data-desk-theatre', theatre) })

  // The Keys list.
  let keysOpen = $state(false), keysDialog = $state<HTMLDialogElement>()
  $effect(() => { if (keysOpen) keysDialog?.showModal() })

  // The desk's keys, caught in the capture phase so the terminal never sees them. None is a plain shell key.
  // A text field keeps its keys (a rename, a search, the top bar): there Alt+Shift selects by word or types a letter on a
  // Mac layout. xterm's own helper textarea is the terminal, so the chords work there.
  function typingIn(target: EventTarget | null): boolean {
    if (!(target instanceof Element)) return false
    if (target instanceof HTMLTextAreaElement) return !target.classList.contains('xterm-helper-textarea')
    return target instanceof HTMLInputElement || target instanceof HTMLSelectElement || !!target.closest('[contenteditable]:not([contenteditable="false"])')
  }
  function onKey(event: KeyboardEvent) {
    const k = deskKey(event)
    if (!k || !desk || typingIn(event.target)) return
    // Held down, only the focus arrows repeat: a held Alt+Shift+D must not start shell after shell.
    if (event.repeat && !(typeof k === 'string' && k.startsWith('focus'))) { event.preventDefault(); event.stopPropagation(); return }
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
      case 'newTab': if (tabs.length < MAX_TABS && !template) adding = section?.querySelector<HTMLElement>('[aria-label="New tab"]') ?? section ?? null; break
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
    <div class="gone"><p role="alert">{failure}</p><div class="acts">{#if failure === READ_FAILED}<button type="button" class="btn primary" onclick={() => start(id)}>Try again</button>{/if}<button type="button" class="btn" onclick={onlist}>All desks</button></div></div>
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
      {#if reopenList.length >= 2 || reopeningAll}
        <button type="button" class="btn primary" disabled={reopeningAll} aria-busy={reopeningAll || undefined} onclick={() => void reopenAll()}>{reopeningAll ? 'Reopening…' : `Reopen all (${reopenList.length})`}</button>
      {/if}
      {#if !phone && activeTab}
        <button type="button" class="btn lay" aria-haspopup="menu" aria-expanded={menu?.kind === 'layout'} disabled={ids.length < 2 && !gatherable}
          title={ids.length < 2 && !gatherable ? 'Layouts arrange the panes of a tab. Split this tab first.' : undefined} onclick={event => openMenu('layout', event)}>
          <span class="lay-icon" aria-hidden="true">{#each icon as r, i (i)}<i style={`left:${r.x * 100}%;top:${r.y * 100}%;width:${r.w * 100}%;height:${r.h * 100}%`}></i>{/each}</span>Layout
        </button>
        {#if !template}
          {@const needs = !typeAll && receivers.length < 2}
          <button type="button" class="btn all" class:on={!!typeAll} bind:this={allButton} aria-pressed={!!typeAll} aria-expanded={askAll}
            disabled={needs} title={needs ? 'Needs two or more live panes on this tab.' : undefined} aria-describedby={needs ? `${panelId}-all-why` : undefined}
            onclick={toggleAll}>Type in all panes</button>
          {#if needs}<span class="vh" id={`${panelId}-all-why`}>Needs two or more live panes on this tab.</span>{/if}
        {/if}
      {/if}
      {#if askAll}
        <div class="ask" role="dialog" aria-labelledby={`${panelId}-all`} aria-describedby={`${panelId}-all-body`} tabindex="-1"
          onkeydown={event => { if (event.key === 'Escape') { event.preventDefault(); closeAsk() } }}>
          <h2 id={`${panelId}-all`}>Type in all panes?</h2>
          <p id={`${panelId}-all-body`}>Everything you type, paste or send from Compose goes to all {receivers.length} live panes on this tab until you turn it off or change tab.</p>
          <div class="actions">
            <button type="button" class="btn quiet" bind:this={askCancel} onclick={closeAsk}>Cancel</button>
            <button type="button" class="btn primary" onclick={startAll}>Type in all {receivers.length}</button>
          </div>
        </div>
      {/if}
      <button type="button" class="btn icon" aria-label="Desk menu" aria-haspopup="menu" aria-expanded={menu?.kind === 'desk'} onclick={event => openMenu('desk', event)}>⋮</button>
    </header>
    {#if template}
      {@const current = desk}
      <p class="notice tpl" role="status"><span>This is a template. Make a desk from it to start shells.</span> <button type="button" class="btn primary" onclick={() => onfromtemplate(current)}>New desk from this template</button></p>
    {/if}
    {#if saveState === 'stale'}<p class="notice" role="status">This desk was changed in another window. Showing the latest.</p>{/if}
    {#if reopenNotice}<p class="notice done" role="status">{reopenNotice} <button type="button" class="btn quiet" onclick={() => (reopenNotice = '')}>Dismiss</button></p>{/if}
    {#if notice}<p class="notice bad" role="alert">{notice} <button type="button" class="btn quiet" onclick={() => (notice = '')}>Dismiss</button></p>{/if}
    <DeskTabs {tabs} active={activeTab?.id ?? ''} {panelId} {waiting} full={tabs.length >= MAX_TABS} adding={!template} menuOpen={menu?.kind === 'add'}
      onselect={select}
      onrename={(tab, name) => change({ kind: 'renameTab', tab, name })}
      oncolour={(tab, colour) => change({ kind: 'colourTab', tab, colour })}
      onduplicate={duplicateTab}
      onclose={tabId => askClose(tabId)}
      onreorder={ids => change({ kind: 'reorderTabs', ids })}
      onnew={openAdd} />
    {#if typeAll}<p class="stripe" role="status">Typing in {typeAll.set.length} panes on this tab. Turns off when you change tab.</p>{/if}
    {#if phone && activeTab && ids.length > 1}
      <div class="chips" role="group" aria-label="Panes on this tab">
        {#each ids as pid (pid)}
          {@const p = activeTab.panes[pid]}
          <button type="button" class="chip" aria-pressed={pid === activePaneId} style={`--pc:${projectColourHex(p?.colour || activeTab.colour) || 'var(--line-strong)'}`} onclick={() => void focusPane(pid)}>{paneName(p)}</button>
        {/each}
      </div>
    {/if}
    {#if activeTab && activePane}
      <DeskToolbar name={paneName(activePane)} actions={actionsFor[activePaneId]} splits={!phone && !template} zoomed={!!zoomed} {theatre}
        canZoom={ids.length > 1} onsplit={dir => void splitPane(dir)} onzoom={() => zoom()} ontheatre={() => (theatre = !theatre)} />
    {/if}
    <div class="area" id={panelId} role="tabpanel" aria-labelledby={activeTab ? `desk-tab-${activeTab.id}` : undefined}>
      {#if activeTab && activePane}
        {@const t = activeTab}
        <SplitArea bind:this={split} tab={t} focused={activePaneId} {zoomed} single={phone} name={pid => paneName(t.panes[pid])}
          onratio={(path, ratio) => change({ kind: 'ratio', tab: t.id, path, ratio })}
          onmove={(paneId, onto, edge) => change({ kind: 'move', tab: t.id, pane: paneId, onto, edge })}>
          {#snippet pane(pid, view)}
            {@const p = t.panes[pid]}
            {#if p}
              <DeskPane tab={t} pane={p} session={p.session ? sessionById.get(p.session) : undefined} machine={machineById.get(p.host)} {ready}
                failure={failures[pid]?.kind} error={failures[pid]?.message ?? ''} busy={!!reopening[pid]} canContinue={continuable(p)}
                onreopen={kind => paneAct(pid, kind)}
                startup={p.start && p.session && offered[pid] === p.session ? p.start : ''}
                onstartup={() => offerDone(pid)}
                receiving={!!typeAll && typeAll.set.includes(pid)}
                onreceivable={ok => noteReceivable(pid, ok)}
                oninput={(data, paste, at) => typeInAll(pid, data, paste, at)}
                onstartcommand={start => change({ kind: 'start', tab: t.id, pane: pid, start })}
                focused={pid === activePaneId} visible={view.visible} hold={view.hold} admitted={admitted.has(pid)} layout={!phone} {phone} {template} zoomed={zoomed === pid} {tabs}
                onfocus={() => setActive(pid)}
                onactions={a => noteActions(pid, a)}
                onsettled={() => settle(pid)}
                onrename={name => renamePane(t, p, name)}
                oncolour={colour => change({ kind: 'colourPane', tab: t.id, pane: pid, colour })}
                onsplit={dir => { setActive(pid); void splitPane(dir, pid) }}
                onzoom={() => zoom(pid)}
                onswap={other => change({ kind: 'swap', tab: t.id, a: pid, b: other })}
                onmovetab={to => moveToTab(t, pid, to)}
                onclose={() => askClose(t.id, pid)} />
            {/if}
          {/snippet}
        </SplitArea>
      {:else}
        <div class="empty">
          <h2>{tabs.length ? 'This tab is empty.' : 'Nowt on this desk yet.'}</h2>
          <p>A tab holds a shell on one of your machines, in the folder you choose.</p>
          {#if !template}<button type="button" class="btn primary" disabled={tabs.length >= MAX_TABS} onclick={event => (adding = event.currentTarget as HTMLElement)}>New tab</button>{/if}
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
{:else if menu?.kind === 'add' && desk}
  <DeskMenu x={menu.x} y={menu.y} label="New tab" opener={menu.opener} onclose={closeMenu}>
    <button type="button" role="menuitem" aria-haspopup="dialog" onclick={() => pickAdd('shell')}>New shell…</button>
    <button type="button" role="menuitem" aria-haspopup="dialog" onclick={() => pickAdd('agent')}>Start an agent…</button>
    <button type="button" role="menuitem" aria-haspopup="dialog" onclick={() => pickAdd('running')}>Add a running session…</button>
  </DeskMenu>
{:else if menu?.kind === 'layout' && desk}
  <DeskMenu x={menu.x} y={menu.y} label="Layout" opener={menu.opener} onclose={closeMenu}>
    {#if ids.length >= 2}
      {#each PRESETS as item (item.name)}<button type="button" role="menuitem" onclick={() => applyPreset(item.name)}>{item.label}</button>{/each}
    {:else}
      {#each GATHER as item (item.name)}<button type="button" role="menuitem" onclick={() => { closeMenu(true); gather(item.name) }}>{item.label}</button>{/each}
    {/if}
    {#if gatherable}<button type="button" role="menuitem" aria-haspopup="dialog" onclick={() => { const opener = menu?.opener ?? null; closeMenu(false); combining = opener }}>Combine tabs…</button>{/if}
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
      <p>Type in all panes sends what you type, paste or send from Compose to every live pane on the tab. Run, Commands, Prompts, Attach and Interrupt stay with the focused pane.</p>
      <div class="actions" data-shell-foot><button type="button" class="btn primary" onclick={() => keysDialog?.close()}>Done</button></div>
    </div>
  </dialog>
{/if}

{#if combining && activeTab}
  <CombineTabsDialog name={activeTab.name || 'Untitled'} room={MAX_PANES - ids.length} limit={MAX_PANES} tabs={otherTabs}
    oncombine={(from, layout) => { void closeCombine(); gather(layout, from) }} oncancel={() => void closeCombine()} />
{/if}
{#if closing}
  <ClosePaneDialog name={closing.name} host={closing.host} count={closing.live.length} removes={removeOnClose} onend={() => void endAndClose()} ontakeoff={takeOff} oncancel={() => (closing = null)} />
{/if}

{#if adding}
  <NewTabDialog {machines} agentId={startAgent} dir={startDir} oncreate={createTab} onclose={() => void closeNewTab()} />
{/if}

{#if agentStart}
  <StartAgentDialog {machines} snapshot={undefined} projects={agentStart.projects} initialAgent={startAgent} projectId={agentStart.projectId} opener={agentStart.opener}
    onstarted={agentStarted} onsignedout={onsignedout} onclose={() => { const o = agentStart?.opener; agentStart = null; o?.focus() }} />
{/if}

{#if addingRunning}
  <AddSessionDialog sessions={runningElsewhere} {machines} {desks} here={id} onadd={addRunning} onclose={() => { const o = addingRunning; addingRunning = null; o?.focus() }} />
{/if}

{#if choosing}
  {@const c = choosing}
  {@const p = paneById(c.pane)?.pane}
  {#if p}
    <NewTabDialog {machines} agentId={c.machine ? '' : p.host} fixed={!c.machine} browsing={!c.machine}
      title={c.machine ? 'Choose a machine' : 'Choose a folder'} aside={c.machine ? 'A new shell opens for this pane on the machine and in the folder you choose.' : `A new shell opens for this pane in the folder you choose on ${machineById.get(p.host)?.host ?? 'its machine'}.`}
      submit="Reopen" busyLabel="Reopening…"
      oncreate={(agentId, dir) => choose(c.pane, agentId, dir)} onclose={() => (choosing = null)} />
  {/if}
{/if}

<style>
  .desk-screen { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); }
  .dhead { position: relative; flex: none; display: flex; align-items: center; gap: 12px; padding: 8px 14px; border-bottom: 1px solid var(--line-soft); min-height: 52px; }
  .dname { display: flex; align-items: center; gap: 8px; min-height: 40px; min-width: 0; padding: 4px 8px; margin-left: -8px; border: 0; border-radius: 8px; background: none; color: var(--ink); cursor: pointer; }
  .dname:hover { background: var(--line-soft); }
  .dname h1 { margin: 0; font: 600 1.0625rem/1.2 var(--sans); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chev { color: var(--muted); }
  .sw { width: 10px; height: 10px; border-radius: 3px; flex: none; background: var(--swatch); border: 1px solid var(--line-strong); }
  .sub { color: var(--faint); font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .save[data-save='unsaved'] { color: var(--warn); }
  .sp { margin-left: auto; }
  .btn.icon { min-width: 40px; padding: 6px 10px; }
  @media (pointer: coarse) { .dname, .btn.icon, .btn.lay, .btn.all { min-height: 44px; } .btn.icon { min-width: 44px; } }
  .notice { flex: none; margin: 0; padding: 6px 14px; background: color-mix(in srgb, var(--info) 14%, var(--bg)); border-bottom: 1px solid var(--line-soft); color: var(--ink); font-size: 14px; }
  .notice.done { display: flex; align-items: center; gap: 8px; }
  .notice.tpl { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; }
  .notice.bad { background: color-mix(in srgb, var(--bad) 12%, var(--bg)); display: flex; align-items: center; gap: 8px; }
  .area { flex: 1; min-height: 0; display: flex; padding: 0; background: var(--line-soft); }
  /* Type in all panes: amber while on, with a stripe under the tab bar and "Receiving" in each pane it reaches. */
  .btn.all.on { background: color-mix(in srgb, var(--warn) 18%, var(--card)); border-color: var(--warn); color: var(--warn); font-weight: 600; }
  .stripe { flex: none; margin: 0; padding: 4px 14px; background: color-mix(in srgb, var(--warn) 16%, var(--bg)); border-bottom: 1px solid var(--warn); color: var(--warn); font-size: 14px; font-weight: 600; }
  .ask { position: absolute; z-index: 20; top: calc(100% - 4px); right: 14px; width: min(360px, calc(100vw - 32px)); padding: 16px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 12px; box-shadow: var(--shadow); }
  .ask h2 { margin: 0; font: 600 1.125rem/1.2 var(--sans); }
  .ask p { margin: 6px 0 0; color: var(--muted); font-size: 14px; }
  .ask .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
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
  .gone .acts { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
  .loading { color: var(--muted); }
  :global(html[data-desk-screen] #app) { height: var(--desk-fit, 100dvh); min-height: 0; }
  :global(html[data-desk-screen] main#main-content) { min-height: 0; display: flex; flex-direction: column; overflow: hidden; padding-bottom: 0; }
  @media (max-width: 700px) {
    .dhead { padding: 6px 10px; gap: 8px; }
    .sub { display: none; }
    .area { padding: 0; }
  }
</style>
