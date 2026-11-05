<script lang="ts">
  import { gafferTooltip } from '../gafferTooltip'
  import { onMount, tick, type Snippet } from 'svelte'
  import { getSyncState, getYoloSetting, type SyncState, type YoloSetting } from '../api'
  import { copyText } from '../clipboard'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import { fleetCards, stateWord, type FleetCard } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import type { RunMember } from '../projects'
  import { headerMeta, offlineSentence, switchModeState, switchWhy, tabFromMenu, worktreeTargetOf } from '../sessions'
  import { toolName } from '../agentLaunch'
  import { CATALOGUE } from '../catalogue'
  import YoloDialog from '../launch/YoloDialog.svelte'
  import { yoloCanStart, type YoloGate } from '../launchBar'
  import { structuredAgent, markSessionSeen, setApprovalMode, setSessionMode, terminalArchivable, terminalClosed, TerminalError, type ApprovalMode, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import ContextPanel from './ContextPanel.svelte'
  import type { ChatState } from '../chat'
  import SwitchModeDialog from './SwitchModeDialog.svelte'
  import TerminalIcon from '../terminal/TerminalIcon.svelte'
  import { PHONE_SCREEN } from '../terminal/sizing'
  import { CONTEXT_DEFAULT, CONTEXT_MIN, clampContextWidth, contextKeyWidth, contextLayoutRecord, readContextLayout, widestContext } from './contextLayout'

  // The session screen: identity and persistent layout actions above a wrapping toolbar for interface, approvals
  // and terminal creation. The menu contains management actions only, the live terminal the workspace hosts here (children),
  // Approvals under it, and the context panel beside it (behind Details under 1100 px and in theatre view). A terminal
  // session shows no steps and no progress bar (R10): only its state word and where that word came from.
  let { session, sessions, machines, health, projects, now, theatre = false, keyboard = false, busy = false, chatBusy, insightChat,
    onaction, ontheatre, onrefresh, onmode, onsignedout, oncontinue, onterminal, onside, side, showSide = false, contextKey = 'reet-session-context-panel', onresizing, children }: {
    session: TerminalSession; sessions: TerminalSession[]
    machines: TerminalMachine[]; health: MachineHealth[]; projects: FolderProject[] | undefined; now: number
    theatre?: boolean; keyboard?: boolean; busy?: boolean
    /** Where the right panel's width and open state are saved, scoped to this person. */
    contextKey?: string
    /** Whether the right panel's separator is being dragged, so the terminal holds its size until it ends. */
    onresizing?: (active: boolean) => void
    onaction: (kind: 'rename' | 'end' | 'archive') => void; ontheatre: () => void; onrefresh: () => void
    /** The open chat's own word on whether it is busy (a running turn, a waiting approval or question), once its stream has
     *  replayed; until then (undefined) the row's attention stands in for it. */
    chatBusy?: boolean; insightChat?: ChatState
    /** The machine switched this session to the other TUI: the workspace shows it at once and reads the listing again. */
    onmode?: (id: string, to: 'terminal' | 'chat') => void
    onsignedout?: (e: unknown) => void
    /** Present only when this closed session can be continued here. */
    oncontinue?: () => void; onterminal?: () => void; onside?: () => void; side?: Snippet; showSide?: boolean; children: Snippet
  } = $props()

  const id = $props.id()
  const chatView = $derived(!!session.structured)
  const terminalLabel = $derived(session.tool ? `${toolName(session.tool)} TUI` : 'Terminal')

  // A phone opens a Gaffer chat in focus mode: the narrative and composer take the screen, and the site chrome, workspace
  // toolbar and secondary session controls step aside. The choice lasts for this browser; Exit focus in the header or the
  // actions menu brings everything back.
  let phone = $state(typeof matchMedia === 'function' && matchMedia(PHONE_SCREEN).matches)
  // svelte-ignore state_referenced_locally (the panel key is fixed for this screen, and a browser preference needs it at once)
  const focusKey = `${contextKey}:focus`
  function savedFocus(): boolean | null {
    try { const saved = localStorage.getItem(focusKey); return saved === '1' ? true : saved === '0' ? false : null } catch { return null }
  }
  let focusWanted = $state<boolean | null>(savedFocus())
  const focusOn = $derived(phone && chatView && (focusWanted ?? true))

  // The skills line comes from the sync state, read once when the screen opens (undefined while reading, null if it failed).
  let sync = $state<SyncState | null | undefined>(undefined)
  onMount(() => {
    const life = new AbortController()
    void getSyncState(life.signal).then(state => { sync = state }).catch(() => { if (!life.signal.aborted) sync = null })
    return () => life.abort()
  })

  // The phone rules include a short touch screen held sideways, so the mode follows the same query the layout does.
  onMount(() => {
    const mq = matchMedia(PHONE_SCREEN)
    const update = () => { phone = mq.matches }
    mq.addEventListener('change', update)
    return () => mq.removeEventListener('change', update)
  })

  // The whole page knows when the session is in focus, so the shell can drop its chrome and the workspace can recompute its
  // height. Only one session screen is mounted at a time; leaving the screen restores everything.
  $effect(() => {
    const root = document.documentElement
    root.toggleAttribute('data-session-focus', focusOn)
    return () => root.removeAttribute('data-session-focus')
  })

  let focusButton = $state<HTMLButtonElement>()
  async function toggleFocus() {
    const next = !focusOn
    focusWanted = next
    menuAt = null
    try { localStorage.setItem(focusKey, next ? '1' : '0') } catch { /* optional browser preference */ }
    if (next) sheetOpen = false
    await tick()
    if (next) screen?.querySelector<HTMLTextAreaElement>('.composer-input')?.focus({ preventScroll: true })
    else focusButton?.focus()
  }

  // The panel's saved width and open state, and the room the screen actually gives it (the container query's 1100 px
  // is measured on this element, so the docked decision matches what CSS used to decide).
  onMount(() => {
    let saved = ''
    try { saved = localStorage.getItem(contextKey) ?? '' } catch { /* optional browser preference */ }
    const layout = readContextLayout(saved)
    contextWidth = layout.width ?? CONTEXT_DEFAULT
    dockedOpen = layout.open ?? true
    layoutLoaded = true
    if (!screen) return
    const measure = () => { screenWidth = screen!.clientWidth }
    const observer = new ResizeObserver(measure)
    observer.observe(screen); measure()
    return () => observer.disconnect()
  })

  // Opening a session marks it seen on the controller so its Needs you card clears across devices.
  let lastMarkedId = ''
  $effect(() => {
    const sid = session.id
    if (!sid || (sid === lastMarkedId && !session.changedSinceSeen && session.attention !== 'waiting')) return
    lastMarkedId = sid
    if (session.changedSinceSeen || session.attention === 'waiting') {
      void markSessionSeen(sid).catch(() => {})
    }
  })

  const host = $derived(machines.find(m => m.agentId === session.agentId)?.host || health.find(m => m.agentId === session.agentId)?.host || 'Unavailable machine')
  const reported = $derived(health.find(m => m.agentId === session.agentId))
  // The fleet strip's own figures for this machine: its reporting state and its skills line.
  const card = $derived.by<FleetCard | undefined>(() => {
    const entry: MachineHealth = reported ?? { agentId: session.agentId, host, state: '' }
    return sync ? fleetCards([entry], sessions, sync, now)[0] : reported ? fleetCards([entry], sessions, { environments: [], targets: [] }, now)[0] : undefined
  })
  const status = $derived(reported ? card?.status : undefined)
  const skills = $derived(sync === undefined ? undefined : sync === null ? null : card?.skills ?? null)
  const away = $derived(status === 'offline')
  const project = $derived(projects?.find(p => p.id === session.projectId)?.name ?? '')
  // The pane's connection, read from its data-connection-state: while it is down the pill says so instead of the session's
  // state ("Running" would claim a live terminal the person cannot reach).
  let stage = $state<HTMLElement>(), connection = $state('')
  $effect(() => {
    if (!stage) return
    const read = () => { connection = stage?.querySelector('.pane')?.getAttribute('data-connection-state') ?? '' }
    read()
    const watch = new MutationObserver(read)
    watch.observe(stage, { subtree: true, childList: true, attributes: true, attributeFilter: ['data-connection-state'] })
    return () => watch.disconnect()
  })
  const DOWN: Record<string, string> = { reconnecting: 'Reconnecting', detached: 'Disconnected', taken_over: 'Open elsewhere', lost: 'Shell unavailable', access_denied: 'Access denied' }
  const sessionWord = $derived(stateWord(session))
  const word = $derived(!terminalClosed(session.state) && DOWN[connection] ? { word: DOWN[connection], tone: 'warn' as const, source: "from this browser's connection" } : sessionWord)
  const statusKind = $derived(!terminalClosed(session.state) && DOWN[connection] ? (connection === 'reconnecting' ? 'starting' : 'failed') : sessionStatus(session))
  const source = $derived(word.source.charAt(0).toUpperCase() + word.source.slice(1))
  const target = $derived(worktreeTargetOf(session))
  const serverOff = $derived(machines.find(m => m.agentId === session.agentId)?.serverEnabled === false)
  const live = $derived(!terminalClosed(session.state))

  // Approvals: a structured chat can switch between asking for every card and YOLO, which answers each one itself.
  // The account setting, the machine's acceptance and the agent's reviewed option are the same gates as a launch.
  let approvalMode = $state<ApprovalMode>('ask'), modeFor = ''
  let yoloSetting = $state<YoloSetting | null | undefined>(undefined)
  let yoloOpen = $state(false), modeBusy = $state(false)
  const machineCaps = $derived(machines.find(m => m.agentId === session.agentId)?.capabilities)
  const canSwitchApproval = $derived(!!session.structured && live && !!structuredAgent(machineCaps, session.tool)?.approval)
  const agentTakesYolo = $derived(!!session.tool && !!CATALOGUE.find(agent => agent.id === session.tool)?.yolo)
  const switchGates = $derived<YoloGate[]>([
    { label: 'Your account allows YOLO', ok: yoloSetting?.allowed === true,
      detail: yoloSetting?.allowed === true ? (yoloSetting.changedBy ? `set by ${yoloSetting.changedBy}` : 'set by an owner') : yoloSetting === null ? 'Couldn’t read the account setting. Reload the page and try again.' : yoloSetting ? 'An owner or admin can allow YOLO for this account in Settings.' : 'Reading the account setting…' },
    { label: `${host} accepts YOLO sessions`, ok: !!machineCaps?.yolo,
      detail: machineCaps?.yolo ? 'checked a moment ago' : `${host} doesn't accept YOLO sessions.` },
    { label: 'This agent has an option reet can use', ok: agentTakesYolo,
      detail: agentTakesYolo ? 'the option is recorded for this session' : 'This agent has no option reet can use for that.' },
    { label: 'This chat can switch while it runs', ok: !!structuredAgent(machineCaps, session.tool)?.approval,
      detail: structuredAgent(machineCaps, session.tool)?.approval ? 'update reet on the machine if this is ever refused' : 'Update reet on that machine to switch a live chat.' },
  ])
  $effect(() => {
    if (session.id !== modeFor) { modeFor = session.id; approvalMode = session.yolo ? 'yolo' : 'ask' }
  })
  // Read once the screen shows a chat (when it opens as one, or when the session switches to Gaffer); an answer that
  // arrives after the screen has gone is dropped.
  let yoloRead = false
  const yoloLife = new AbortController()
  onMount(() => () => yoloLife.abort())
  $effect(() => {
    if (!session.structured || yoloRead) return
    yoloRead = true
    void getYoloSetting().then(value => { if (!yoloLife.signal.aborted) yoloSetting = value }).catch(() => { if (!yoloLife.signal.aborted) yoloSetting = null })
  })

  // Gaffer | agent TUI switch (switchModeState says where the inactive button goes and why it may not). A switch
  // stops one process on the machine and starts the other, which can take minutes: until the machine answers, both buttons
  // wait and the status line says so. No client deadline is set; the request ends when the controller answers. Each session
  // has its own switch in flight (keyed by its id), so one held on the machine never holds or drops another's.
  let switching = $state<Record<string, 'terminal' | 'chat'>>({})
  let switchAsk = $state<{ to: 'terminal' | 'chat'; trust?: { detail: string } } | null>(null), switchOpener: HTMLElement | null = null
  const modeSwitching = $derived(!!switching[session.id])
  const chatIsBusy = $derived(chatBusy ?? (session.attention === 'working' || session.attention === 'waiting'))
  const switchState = $derived(switchModeState(session, machineCaps, chatIsBusy))
  const tuiName = (to: 'terminal' | 'chat') => to === 'chat' ? 'Gaffer' : terminalLabel
  const canSwitch = $derived(switchState.enabled && !modeSwitching && !busy)
  const whyNot = $derived(switchWhy(switchState, switching[session.id] ? tuiName(switching[session.id]) : undefined, busy))
  function openSwitch(opener: HTMLElement | null) {
    if (!canSwitch) return
    switchOpener = opener
    switchAsk = { to: switchState.to }
  }
  function closeSwitch() { switchAsk = null; switchOpener?.focus() }
  async function confirmSwitch(trust: boolean) {
    const ask = switchAsk
    switchAsk = null
    if (!ask || switching[session.id]) return
    const target = { id: session.id, to: ask.to }, name = tuiName(ask.to)
    switching[target.id] = target.to
    notice = { text: `Switching to ${ask.to === 'chat' ? name : `the ${name}`}… This can take a few minutes.` }
    switchOpener?.focus()
    try {
      await setSessionMode(target.id, target.to, trust)
      if (session.id === target.id) notice = { text: `Switched to ${ask.to === 'chat' ? name : `the ${name}`}.` }
      if (onmode) onmode(target.id, target.to); else onrefresh()
    } catch (e) {
      // Away from it, the session's sentence is not shown here, but its row is read again so it is not stale.
      if (session.id !== target.id) { onrefresh(); return }
      if (e instanceof TerminalError && e.code === 'acp_trust_required' && target.to === 'chat' && !trust) {
        notice = null
        switchAsk = { to: target.to, trust: { detail: e.detail || e.message } }
        return
      }
      notice = { text: e instanceof Error && e.message ? e.message : 'The session could not be switched. Try again in a moment.', bad: true }
      onrefresh()
    } finally { delete switching[target.id] }
  }

  // The context panel is a docked column beside the terminal from 1100 px (never in theatre view); below that, and in
  // theatre view, Details opens it as a sheet over the terminal's edge. Docked it can also be collapsed and resized, and
  // its open state and width are remembered in this browser for the person.
  let screen = $state<HTMLElement>(), screenWidth = $state(0)
  let dockedOpen = $state(true), sheetOpen = $state(false)
  let contextWidth = $state(CONTEXT_DEFAULT), bodyWidth = $state(0), layoutLoaded = false
  let detailsButton = $state<HTMLButtonElement>()
  const docked = $derived(!theatre && screenWidth > 1099)
  const hasSide = $derived(showSide && !!side)
  const contextOpen = $derived(docked ? dockedOpen : sheetOpen)
  // Focus keeps the conversation alone on the screen, so a Details sheet opened before it goes away.
  $effect(() => { if (focusOn) sheetOpen = false })
  // The separator's 8 px track sits between the terminal and the panel; the panel takes at most 40 % of what is left.
  const CONTEXT_TRACK = 8
  const contextRoom = $derived(Math.max(0, bodyWidth - CONTEXT_TRACK))
  const shownContextWidth = $derived(clampContextWidth(contextWidth, contextRoom))
  const contextMax = $derived(widestContext(contextRoom))
  function saveContext() {
    if (!layoutLoaded) return
    try { localStorage.setItem(contextKey, JSON.stringify(contextLayoutRecord(dockedOpen, contextWidth))) } catch { /* optional browser preference */ }
  }
  async function toggleDetails() {
    // A terminal opened beside the session takes the panel's place; Details has nothing to toggle until it closes.
    if (hasSide) return
    if (docked) { dockedOpen = !dockedOpen; saveContext(); return }
    sheetOpen = !sheetOpen
    if (!sheetOpen) return
    // Focus moves into the sheet (its Close details first), so it never stays on a control the sheet could cover.
    await tick()
    document.getElementById(`${id}-context`)?.querySelector<HTMLElement>('.close-details')?.focus()
  }
  // The panel's own collapse control, and the sheet's Close details: the panel goes and focus returns to Details.
  async function collapseDetails() {
    if (docked) dockedOpen = false; else sheetOpen = false
    saveContext()
    await tick()
    detailsButton?.focus()
  }
  // Leaving the panel for the terminal only dismisses a sheet; a docked panel stays where the person put it.
  function closeSheet() { if (!docked) sheetOpen = false }
  function closeDetails(event: KeyboardEvent) {
    if (event.key !== 'Escape' || docked || !sheetOpen) return
    event.preventDefault(); event.stopPropagation()
    void collapseDetails()
  }
  // While it is a sheet, Tab and Shift+Tab go round its own controls; Escape and Close details give focus back to Details.
  function panelKey(event: KeyboardEvent) {
    if (event.key === 'Escape') return closeDetails(event)
    if (event.key !== 'Tab' || !sheetOpen || docked) return
    const panel = event.currentTarget as HTMLElement
    const items = [...panel.querySelectorAll<HTMLElement>('a[href],button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),summary,[tabindex]')].filter(el => {
      if (el.tabIndex < 0 || !el.getClientRects().length || el.closest('[inert]')) return false
      const closed = el.closest('details:not([open])')
      return !closed || (el.tagName === 'SUMMARY' && el.parentElement === closed && !closed.parentElement?.closest('details:not([open])'))
    })
    if (!items.length) return
    const at = items.indexOf(document.activeElement as HTMLElement)
    event.preventDefault()
    items[at < 0 ? 0 : (at + (event.shiftKey ? -1 : 1) + items.length) % items.length].focus()
  }

  // Dragging the right separator: the pointer is captured, so leaving the handle keeps the drag. Moving left widens the
  // panel. The terminal holds its size and refits once when the drag ends, exactly as it does for the sessions panel.
  let contextDrag: { x: number; width: number } | null = null, contextResizing = $state(false)
  function contextSeparatorDown(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    event.preventDefault()
    const handle = event.currentTarget as HTMLElement
    handle.setPointerCapture(event.pointerId); handle.focus({ preventScroll: true })
    contextDrag = { x: event.clientX, width: shownContextWidth }
    contextResizing = true; onresizing?.(true)
  }
  function contextSeparatorMove(event: PointerEvent) {
    if (!contextDrag || !(event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)) return
    contextWidth = clampContextWidth(contextDrag.width - (event.clientX - contextDrag.x), contextRoom)
  }
  function contextSeparatorEnd() {
    if (!contextDrag) return
    contextDrag = null; contextResizing = false; onresizing?.(false); saveContext()
  }
  function contextSeparatorKey(event: KeyboardEvent) {
    const next = contextKeyWidth(event.key, shownContextWidth, contextRoom)
    if (next === null) return
    event.preventDefault(); contextWidth = next; saveContext()
  }
  function contextSeparatorReset() { contextWidth = CONTEXT_DEFAULT; saveContext() }

  // The actions menu: its ⋮ button, or Shift+F10 / the menu key on it. Arrow keys move, Escape closes and gives focus back.
  let menu = $state<HTMLElement>(), menuButton = $state<HTMLButtonElement>()
  let menuAt = $state<{ x: number; y: number } | null>(null)
  async function openMenu(event: MouseEvent | KeyboardEvent) {
    event.preventDefault()
    const box = menuButton!.getBoundingClientRect()
    menuAt = { x: Math.max(8, Math.min(box.right - 240, innerWidth - 248)), y: Math.max(8, Math.min(box.bottom + 4, innerHeight - 290)) }
    await tick()
    menu?.querySelector<HTMLElement>('[role=menuitem]:not(:disabled)')?.focus()
  }
  function closeMenu(focus = true) { menuAt = null; if (focus) menuButton?.focus() }
  function menuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(); return }
    if (event.key === 'Tab') { event.preventDefault(); closeMenu(false); tabFromMenu(menuButton, event.shiftKey); return }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = [...menu!.querySelectorAll<HTMLElement>('[role=menuitem]:not(:disabled)')], at = items.indexOf(document.activeElement as HTMLElement)
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
  }
  const menuKeyOpens = (event: KeyboardEvent) => event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')
  function act(kind: 'rename' | 'end' | 'archive') { closeMenu(false); onaction(kind) }
  function theatreView() { closeMenu(false); ontheatre() }
  function continueSession() { if (!oncontinue) return; closeMenu(false); oncontinue() }
  function openTerminal() { if (!onterminal) return; closeMenu(false); onterminal() }
  function openSideTerminal() { if (!onside) return; closeMenu(false); onside() }

  let notice = $state<{ text: string; bad?: boolean } | null>(null)
  async function setMode(next: ApprovalMode) {
    if (modeBusy || next === approvalMode) return
    modeBusy = true; notice = null
    try {
      await setApprovalMode(session.id, next)
      approvalMode = next
      onrefresh()
    } catch (e) {
      notice = { text: e instanceof Error && e.message ? e.message : 'That approval mode was not changed.', bad: true }
    } finally { modeBusy = false }
  }
  function askMode() { if (approvalMode === 'yolo') void setMode('ask') }
  function yoloMode() {
    if (approvalMode === 'yolo' || modeBusy) return
    yoloOpen = true
  }
  function turnOnYolo() {
    if (!yoloCanStart(switchGates)) return
    yoloOpen = false
    void setMode('yolo')
  }
  async function copyBranch() {
    const branch = session.worktreeBranch
    closeMenu()
    if (!branch) return
    notice = (await copyText(branch)) ? { text: `Copied ${branch}.` } : { text: "Couldn't copy the branch. Select it in Where it's working instead.", bad: true }
  }
  // Check and remove worktree: the existing sheet, which reads the worktree's state first and removes nothing unsaved.
  let removing = $state<RunMember | null>(null)
  function checkWorktree() {
    const t = target
    if (!t) return
    closeMenu(false)
    removing = { position: 0, host: session.agentId, dir: '', tool: session.tool ?? '', model: session.model ?? '', state: 'started', code: '', sentence: '', sessionId: session.id,
      worktreePath: session.worktreePath ?? '', worktreeRepo: t.repo, worktreeBranch: session.worktreeBranch ?? '', worktreeRun: t.run, worktreeProject: t.project, sharesCheckout: false, attempt: 0 }
  }
  // A new session's screen starts with its own words: the last one's notice does not carry over.
  let shownId = ''
  $effect(() => { if (session.id !== shownId) { shownId = session.id; notice = null } })
</script>

<svelte:window onpointerdown={event => { if (menuAt && !menu?.contains(event.target as Node) && !menuButton?.contains(event.target as Node)) closeMenu(false) }} />

<!-- One grid: the header and status line, then the context panel, then the terminal, so the tab order is header → panel →
     terminal while the panel shows beside the terminal (right column) or over its edge (as a sheet below the header). -->
<div class="session-screen" class:theatre class:keyboard class:focus={focusOn} class:resizing={contextResizing}
  bind:this={screen} style={`--context-width:${shownContextWidth}px`}>
  <div class="body" class:context-docked={docked && contextOpen && !hasSide} class:context-sheet={!docked && contextOpen && !hasSide} bind:clientWidth={bodyWidth}>
    <header class="head">
      <div class="identity-row">
      <a class="back" href="/sessions" aria-label="Back to sessions"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg></a>
      <div class="heading">
        <h2 class="title" title={session.title}>{session.title}</h2>
        <p class="sub">{headerMeta(session, session.createdAt, now)}</p>
        <p class="where-phone">{[word.word, project, host].filter(Boolean).join(' · ')}</p>
      </div>
      <span class="pill" data-status={statusKind} title={source}><span class="dot" aria-hidden="true"></span>{word.word}{#if away}{' '}<span class="lk">· last known</span>{/if}<span class="vh"> · {word.source}</span></span>
      <div class="header-actions">
        <button type="button" class="focus-button" bind:this={focusButton} aria-pressed={focusOn}
          aria-label={focusOn ? 'Exit focus mode' : 'Focus mode'} title={focusOn ? 'Show the full session screen' : 'Focus on the conversation and composer'}
          onclick={() => void toggleFocus()}>
          <TerminalIcon name={focusOn ? 'contract' : 'expand'} /><span class="wide-label">{focusOn ? 'Exit focus' : 'Focus'}</span><span class="compact-label">{focusOn ? 'Exit' : 'Focus'}</span>
        </button>
        <button type="button" class="theatre-button" aria-pressed={theatre} aria-label={theatre ? 'Exit theatre view' : 'Theatre view'} title={theatre ? 'Restore the session panels' : 'Hide the panels and focus on this session'} onclick={theatreView}>
          <TerminalIcon name="theatre" /><span class="wide-label">{theatre ? 'Exit theatre' : 'Theatre view'}</span><span class="compact-label">{theatre ? 'Exit' : 'Theatre'}</span>
        </button>
        <button type="button" class="details" bind:this={detailsButton} aria-label="Details" aria-expanded={hasSide ? false : contextOpen} aria-controls={`${id}-context`} title={docked ? (dockedOpen ? 'Collapse session workspace' : 'Show session workspace') : 'Session details'} onclick={() => void toggleDetails()} onkeydown={closeDetails}><TerminalIcon name="panel" /><span>Details</span></button>
        <button type="button" class="more" bind:this={menuButton} aria-label="Session actions" aria-haspopup="menu" aria-expanded={menuAt !== null}
          onclick={event => { if (menuAt) closeMenu(); else void openMenu(event) }} onkeydown={event => { if (menuKeyOpens(event)) void openMenu(event) }}>
          <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="19" cy="12" r="1.6" /></svg><span>More</span>
        </button>
      </div>
      </div>
      <div class="session-tools" role="group" aria-label="Session controls" data-keeps-focus>
      {#if canSwitchApproval}
        <div class="approvals" role="group" aria-label="Approvals">
          <button type="button" aria-pressed={approvalMode === 'ask'} disabled={busy || modeBusy} onclick={askMode}>Ask me</button>
          <button type="button" aria-pressed={approvalMode === 'yolo'} disabled={busy || modeBusy}
            title={yoloCanStart(switchGates) ? 'Answer every approval for this session' : 'Read why YOLO is not ready here'} onclick={yoloMode}>YOLO</button>
        </div>
      {:else if session.yolo}<span class="badge yolo">YOLO</span>{/if}
      <!-- The pressed button is the TUI shown; the other switches to it (aria-disabled with its reason when it cannot now). -->
      <div class="view" role="group" aria-label="View" aria-busy={modeSwitching}>
        <button use:gafferTooltip type="button" class="chat" aria-pressed={chatView} aria-disabled={chatView ? modeSwitching || undefined : !canSwitch}
          aria-describedby={!chatView && !canSwitch ? `${id}-switch-why` : undefined} title={!chatView && !canSwitch ? whyNot : undefined}
          onclick={event => { if (!chatView) openSwitch(event.currentTarget) }}>Gaffer</button>
        <button type="button" aria-pressed={!chatView} aria-disabled={chatView ? !canSwitch : modeSwitching || undefined}
          aria-describedby={chatView && !canSwitch ? `${id}-switch-why` : undefined} title={chatView && !canSwitch ? whyNot : undefined}
          onclick={event => { if (chatView) openSwitch(event.currentTarget) }}>{terminalLabel}</button>
      </div>
        <span class="vh" id={`${id}-switch-why`}>{whyNot}</span>
        {#if live && onterminal}<button type="button" class="tool-action" disabled={busy || serverOff} onclick={openTerminal}><span aria-hidden="true">›_</span> Terminal here</button>{/if}
        {#if live && onside}<button type="button" class="tool-action" disabled={busy || serverOff || showSide} onclick={openSideTerminal}><TerminalIcon name="panel" />{showSide ? 'Terminal open beside' : 'Terminal beside'}</button>{/if}
        {#if oncontinue}<button type="button" class="tool-action" disabled={busy} onclick={continueSession}>{session.structured ? 'Continue with history' : 'Continue session'}</button>{/if}
      </div>
    </header>
    <!-- Always rendered (empty takes no room), so the first message in it is announced. -->
    <div class="live" role="status">{#if notice}<p class:bad={notice.bad}>{notice.text}</p>{/if}</div>
    {#if showSide && side}
      <div class="side-terminal" data-keeps-focus>{@render side()}</div>
    {:else}
      {#key session.id}
        <ContextPanel {session} {host} {status} {project} {skills} {now} chat={insightChat} supported={!!machineCaps?.inspect}
          docked={docked}
          onterminal={() => { closeSheet(); void tick().then(() => stage?.querySelector<HTMLElement>('.xterm-helper-textarea, textarea.composer-input')?.focus()) }}
          id={`${id}-context`} onkeydown={panelKey} onclose={() => void collapseDetails()} />
      {/key}
      {#if docked && contextOpen}
        <!-- The right panel's separator, the same control as the sessions panel's: drag (pointer capture), Left widens,
             Right narrows, Home the narrowest, End the widest, double-click 360 px. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div class="panel-separator context-separator" role="separator" tabindex="0" aria-label="Resize session workspace" aria-controls={`${id}-context`} aria-orientation="vertical"
          aria-valuemin={CONTEXT_MIN} aria-valuemax={contextMax} aria-valuenow={shownContextWidth} aria-valuetext={`${shownContextWidth} pixels`}
          title="Drag to resize · double-click resets" onpointerdown={contextSeparatorDown} onpointermove={contextSeparatorMove} onpointerup={contextSeparatorEnd}
          onpointercancel={contextSeparatorEnd} onlostpointercapture={contextSeparatorEnd} onkeydown={contextSeparatorKey} ondblclick={contextSeparatorReset}></div>
      {/if}
    {/if}
    <div class="stage" bind:this={stage}>{@render children()}</div>
  </div>
</div>

{#if menuAt}
  <!-- data-keeps-focus: a reconnecting terminal never takes focus out of the open menu. -->
  <div class="menu" role="menu" aria-label={`Actions for ${session.title}`} tabindex="-1" data-keeps-focus bind:this={menu} style={`left:${menuAt.x}px;top:${menuAt.y}px`} onkeydown={menuKey}>
    <button role="menuitem" disabled={busy || serverOff} onclick={() => act('rename')}>Rename…</button>
    {#if phone && chatView}<button role="menuitem" onclick={() => void toggleFocus()}>{focusOn ? 'Exit focus mode' : 'Focus mode'}</button>{/if}
    {#if session.worktreeBranch}<button role="menuitem" onclick={() => void copyBranch()}>Copy branch</button>{/if}
    {#if live}<button role="menuitem" class="danger" disabled={busy || serverOff} onclick={() => act('end')}>{session.state === 'end_requested' ? 'Retry end request…' : 'End session…'}</button>
    {/if}
    {#if terminalArchivable(session.state)}<button role="menuitem" disabled={busy} onclick={() => act('archive')}>Remove from history…</button>{/if}
    {#if target}<button role="menuitem" disabled={live || away} title={live ? 'End the session first: it is working in this worktree.' : away ? offlineSentence(host) : undefined} onclick={checkWorktree}>Check and remove worktree…</button>{/if}
  </div>
{/if}
{#if removing}
  <WorktreeDialog member={removing} {host} opener={menuButton ?? null} onremoved={onrefresh} {onsignedout} onclose={() => (removing = null)} />
{/if}
{#if switchAsk}
  <SwitchModeDialog to={switchAsk.to} agentTui={terminalLabel} machine={host} trust={switchAsk.trust}
    onconfirm={trust => void confirmSwitch(trust)} oncancel={closeSwitch} />
{/if}
{#if yoloOpen}
  <YoloDialog gates={switchGates} tool={session.tool ?? ''} structured machine={host} switching
    onstart={turnOnYolo} oncancel={() => (yoloOpen = false)} />
{/if}

<style>
  .session-screen { flex: 1; min-height: 0; min-width: 0; display: flex; flex-direction: column; background: var(--bg); container: session-screen / inline-size; }
  .body { flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-rows: auto auto minmax(0, 1fr); }
  .head { grid-column: 1; grid-row: 1; }
  .live { grid-column: 1; grid-row: 2; }
  .stage { grid-column: 1; grid-row: 3; }
  /* The context panel is hidden until JS says it is docked or shown as a sheet; either way it keeps the same cell it
     always had beside the terminal's right edge. */
  .body > :global(.context-panel) { display: none; grid-column: 2; grid-row: 1 / -1; min-width: 0; }
  .body.context-docked { grid-template-columns: minmax(0, 1fr) 8px var(--context-width); }
  .body.context-docked > :global(.context-panel) { display: flex; grid-column: 3; }
  .body.context-sheet > :global(.context-panel) { display: flex; grid-column: 1; grid-row: 3; justify-self: end; position: relative; z-index: 20; width: min(340px, 100%); box-shadow: var(--shadow); }
  .body.context-sheet > :global(.context-panel .close-details) { display: inline-flex; align-items: center; }
  /* The separator's 8 px track plus 8 px either side, the same control and 24 px hit area as the sessions panel's. */
  .context-separator { grid-column: 2; grid-row: 1 / -1; }
  .panel-separator { position: relative; z-index: 3; margin: 0 -8px; display: flex; align-items: center; justify-content: center; cursor: col-resize; touch-action: none; outline: 0; }
  .panel-separator::after { content: ''; width: 4px; height: 40px; border-radius: 2px; background: var(--line); }
  /* The chat column's scrollbar sits flush against this track, so the hit area must not overlap it: extend only into the panel. */
  .context-separator { margin: 0 -8px 0 0; }
  .panel-separator:hover::after, .session-screen.resizing .panel-separator::after { background: var(--accent); height: 64px; }
  .panel-separator:focus-visible::after { background: var(--accent); height: 64px; outline: 2px solid var(--accent); outline-offset: 3px; }
  .session-screen.resizing { cursor: col-resize; user-select: none; }
  .side-terminal { grid-column: 2; grid-row: 1 / -1; width: 340px; min-width: 0; min-height: 0; display: flex; flex-direction: column; border-left: 1px solid var(--line-soft); background: var(--bg-sunk); }
  .side-terminal :global(.pane) { flex: 1; min-height: 0; }
  .side-terminal :global(.pane > header), .side-terminal :global(.pane > footer) { display: none; }
  /* Identity stays separate from controls, so long session names never push frequent actions into a menu. */
  .head { min-width: 0; padding: 0 20px 8px; box-sizing: border-box; }
  .identity-row { display: flex; align-items: center; gap: 12px; min-height: 58px; min-width: 0; }
  .header-actions { display: flex; align-items: center; gap: 6px; flex: none; }
  .session-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; min-width: 0; }
  .tool-action, .theatre-button, .focus-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; min-height: 36px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: none; color: var(--ink-soft); font: 500 12px var(--sans); cursor: pointer; }
  .tool-action:hover:not(:disabled), .theatre-button:hover, .focus-button:hover { background: var(--tint); color: var(--ink); }
  .tool-action:disabled { opacity: .5; cursor: default; }
  .focus-button { display: none; }
  .compact-label { display: none; }
  .theatre-button[aria-pressed=true] { border-color: var(--accent-text); background: var(--line-soft); color: var(--ink); }
  .tool-action:focus-visible, .theatre-button:focus-visible, .focus-button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .heading { flex-grow: 1; min-width: 0; }
  /* Back to the Sessions screen: the phone's slim header has it (the workspace toolbar's arrow gives way there). */
  .back { display: none; flex: none; width: 44px; height: 44px; align-items: center; justify-content: center; color: var(--ink-soft); border-radius: 10px; }
  .back svg { width: 22px; height: 22px; }
  .back:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .title { margin: 0; font: 600 21px/1.3 var(--serif); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .sub, .where-phone { margin: 0; font-size: 13px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .where-phone { display: none; }
  .pill { flex: none; display: inline-flex; align-items: center; gap: 8px; height: 30px; padding: 0 12px; border-radius: 99px; box-sizing: border-box;
    background: var(--status-wash); border: 1px solid var(--status-edge); color: var(--status); font-size: 13px; font-weight: 600; white-space: nowrap; }
  .pill .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; }
  .badge { flex: none; padding: 0 8px; border: 1px solid var(--line-strong); border-radius: 99px; font-size: 12px; line-height: 22px; color: var(--muted); }
  .badge.yolo { border-color: var(--warn); color: var(--warn); font-weight: 600; }
  .approvals { flex: none; display: flex; border: 1px solid var(--line); border-radius: 9px; overflow: hidden; }
  .approvals button { padding: 7px 12px; border: 0; background: none; color: var(--muted); font: 400 13px var(--sans); cursor: pointer; }
  .approvals button[aria-pressed=true] { color: var(--ink); font-weight: 600; }
  .approvals button[aria-pressed=true]:first-child { background: var(--line-soft); }
  .approvals button[aria-pressed=true]:last-child { background: color-mix(in srgb, var(--warn) 16%, transparent); color: var(--warn); }
  .approvals button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .approvals button:disabled { opacity: .5; cursor: default; }
  .view { flex: none; display: flex; border: 1px solid var(--line); border-radius: 9px; overflow: hidden; }
  .view button { padding: 7px 16px; border: 0; background: none; color: var(--muted); font: 400 14px var(--sans); cursor: pointer; }
  .view button[aria-pressed=true] { background: var(--line-soft); color: var(--ink); font-weight: 500; cursor: default; }
  .view button[aria-disabled=true] { cursor: not-allowed; }
  .view[aria-busy=true] button { cursor: progress; }
  .view button:focus-visible, .details:focus-visible, .more:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .details { display: inline-flex; align-items: center; justify-content: center; gap: 6px; flex: none; min-height: 44px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 10px; background: none; color: var(--ink); font: 500 14px var(--sans); cursor: pointer; }
  .details[aria-expanded=true] { background: var(--line-soft); }
  .more { flex: none; display: inline-flex; align-items: center; justify-content: center; gap: 4px; min-width: 40px; min-height: 40px; padding: 0; border: 0; border-radius: 9px; background: none; color: var(--muted); cursor: pointer; }
  .more:hover { background: var(--tint); color: var(--ink); }
  .more svg { width: 20px; height: 20px; }
  .more span { font: 500 12px var(--sans); }
  .live p { margin: 0 28px 8px; font-size: 13px; color: var(--muted); }
  .live p.bad { color: var(--bad); }
  .stage { min-height: 0; display: flex; flex-direction: column; overflow: hidden; border-top: 1px solid var(--line-soft); }
  .menu { position: fixed; z-index: 110; width: 240px; max-height: calc(100dvh - 16px); overflow: auto; display: grid; gap: 2px; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--card-raised); box-shadow: var(--shadow); }
  .menu [role=menuitem] { display: block; min-height: 36px; padding: 8px 9px; border: 0; border-radius: 6px; background: none; color: var(--ink); font: 400 14px var(--sans); text-align: left; cursor: pointer; }
  .menu [role=menuitem]:hover:not(:disabled), .menu [role=menuitem]:focus-visible { background: var(--tint); outline: none; }
  .menu [role=menuitem]:disabled { opacity: .45; cursor: default; }
  .menu .danger { color: var(--bad); }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @media (pointer: coarse) { .menu [role=menuitem] { min-height: 44px; } .more, .tool-action, .theatre-button { min-width: 44px; min-height: 44px; } .view button, .approvals button { min-height: 44px; } }
  @container session-screen (max-width: 1099px) {
    .head { padding: 0 12px 8px; }
    .heading { flex: 1 1 0; }
  }
  @media (pointer: coarse) { .context-separator::before { content: ''; position: absolute; top: 50%; left: calc(50% - 4px); width: 44px; height: 88px; transform: translateY(-50%); } }
  /* Phones retain every control. Layout actions use 44 px icons; the toolbar wraps instead of hiding choices. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .sub, .identity-row > .pill { display: none; }
    .where-phone { display: block; font-size: 12px; }
    .back { display: flex; }
    .head { padding: 0 6px 6px; }
    .identity-row { min-height: 52px; gap: 4px; }
    .header-actions { gap: 2px; }
    .header-actions .theatre-button, .header-actions .details, .header-actions .more, .header-actions .focus-button { width: 44px; min-width: 44px; min-height: 44px; padding: 0; }
    .header-actions .focus-button { display: inline-flex; }
    .header-actions button { flex-direction: column; gap: 1px; }
    .header-actions button span { display: block; font: 500 10px/12px var(--sans); }
    .header-actions button .wide-label { display: none; }
    .header-actions :global(svg) { width: 19px; height: 19px; }
    .session-tools { gap: 6px; }
    .view button { padding: 6px 10px; font-size: 12px; min-height: 44px; }
    .approvals button { padding: 6px 10px; font-size: 12px; min-height: 44px; }
    .tool-action { min-height: 44px; padding: 0 10px; }
    .heading { flex: 1 1 0; }
    .title { font: 600 15px/1.3 var(--sans); }
    .pill { height: 28px; padding: 0 10px; gap: 6px; font-size: 12px; }
    .pill .lk { display: none; }
    .badge { font-size: 11px; line-height: 18px; padding: 0 6px; }

    .details { padding: 0 10px; }
  }
  /* Super focus on a phone: one slim identity row, then the conversation and composer with nothing competing for the room.
     The site chrome and workspace toolbar are hidden through data-session-focus and the workspace's own rule. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .session-screen.focus .head { padding: 0 6px; padding-top: env(safe-area-inset-top, 0px); }
    .session-screen.focus .identity-row { min-height: 50px; gap: 6px; }
    .session-screen.focus .heading { min-width: 0; max-width: none; flex: 1 1 auto; }
    .session-screen.focus .title { font: 600 16px/1.25 var(--sans); }
    .session-screen.focus .sub, .session-screen.focus .where-phone { display: none; }
    .session-screen.focus .identity-row > .pill { display: inline-flex; }
    .session-screen.focus .session-tools { display: none; }
    .session-screen.focus .header-actions { gap: 0; }
    .session-screen.focus .header-actions .theatre-button,
    .session-screen.focus .header-actions .details,
    .session-screen.focus .header-actions .more { display: none; }
    .session-screen.focus .header-actions .focus-button { width: 44px; min-width: 44px; min-height: 44px; }
    .session-screen.focus .live p { margin: 0 8px 6px; }
    .session-screen.focus .stage { border-top: 0; }
    /* The narrative is the screen: larger prose, a tighter log, and the composer's secondary tools wait behind Exit focus. */
    .session-screen.focus :global(.chat-status) { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
    .session-screen.focus :global(.log) { padding: 12px 14px 16px; gap: 10px; }
    .session-screen.focus :global(.bubble.agent) { font-size: 15px; line-height: 1.68; }
    .session-screen.focus :global(.row.user) { margin-top: 14px; }
    .session-screen.focus :global(.row.user .bubble) { max-width: 90%; }
    .session-screen.focus :global(.composer) { margin: 0 8px max(8px, env(safe-area-inset-bottom, 0px)); padding: 8px; }
    .session-screen.focus :global(.hint) { display: none; }
    .session-screen.focus :global(.tool-head) { min-height: 44px; }
  }
  /* In landscape keep the controls reachable and trim secondary metadata. */
  @media (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .head { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; padding: 0 4px 4px; }
    .identity-row { flex: 1 1 190px; min-height: 44px; }
    .heading { max-width: 100px; }
    .where-phone { display: none; }
    .session-tools { flex: 0 1 auto; }
  }
  /* With the phone keyboard up the site's chrome steps aside (keyboardChrome) and the slim header stays, as the mock-up's phone
     session draws it; the status line gives its row to the terminal. */
  .keyboard .live { display: none; }
  /* The slim header names the session and its state, so the pane's own header gives its row to the terminal while typing. */
  @media (max-width: 650px) {
    /* Not while the connection is down: the header holds Retry now, Reconnect and Continue here. */
    .keyboard .stage :global(.pane:not(.maximised)[data-connection-state=live] > header) { display: none; }
    /* The header keeps its 44 px targets but drops its spare 8 px while typing, so the terminal keeps its keyboard-open rows. */
    .keyboard .head { min-height: 44px; }
  }
</style>
