<script lang="ts">
  import { factoryBadge, type FactoryMember } from '../gafferOutcome'
  import SessionDecision from '../factory/SessionDecision.svelte'
  import { gafferChatEntry, gafferSessionContext } from '../gafferContext'
  import GafferCompanionAvatar from '../GafferCompanionAvatar.svelte'
  import { gafferTooltip } from '../gafferTooltip'
  import { onMount, tick, type Snippet } from 'svelte'
  import { loadMachineState, getYoloSetting, type SyncState, type YoloSetting } from '../api'
  import { copyText } from '../clipboard'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import { fleetCards, stateWord, type FleetCard } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import type { RunMember } from '../projects'
  import { headerMeta, offlineSentence, switchModeState, switchWhy, worktreeTargetOf } from '../sessions'
  import { toolName } from '../agentLaunch'
  import { CATALOGUE } from '../catalogue'
  import YoloDialog from '../launch/YoloDialog.svelte'
  import { yoloSwitchTitle } from '../launch/yoloWording'
  import { yoloCanStart, type YoloGate } from '../launchBar'
  import { signalAttentionChanged } from '../attentionSignal'
  import { attentionRuleOf } from '../attention'
  import { structuredAgent, markSessionSeen, setApprovalMode, setSessionMode, terminalArchivable, terminalClosed, TerminalError, type ApprovalMode, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import ContextPanel from './ContextPanel.svelte'
  import type { ChatState } from '../chat'
  import type { ChatProgressState } from '../gafferProgress'
  import SwitchModeDialog from './SwitchModeDialog.svelte'
  import TerminalIcon from '../terminal/TerminalIcon.svelte'
  import PanelResizer from '../shell/PanelResizer.svelte'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import SessionViewSwitch from './SessionViewSwitch.svelte'
  import TerminalQuestionCard from '../terminal/TerminalQuestionCard.svelte'
  import { PHONE_SCREEN } from '../terminal/sizing'
  import { CONTEXT_DEFAULT, CONTEXT_MIN, clampContextWidth, contextLayoutRecord, readContextLayout, widestContext } from './contextLayout'
  import { SWITCH_LIVE_CHAT_HINT } from '../chatAgents'

  // The session screen: identity above a wrapping toolbar for interface, approvals, theatre, management and terminal
  // actions. The live terminal is the workspace's (children), and the context panel sits beside it, toggled from the
  // workspace toolbar like the sessions panel. A terminal session shows no steps and no progress bar (R10): only its
  // state word and where that word came from.
  let { factory = undefined, session, sessions, machines, health, projects, now, theatre = false, keyboard = false, busy = false, chatBusy, insightChat, insightProgress,
    onaction, ontheatre, onrefresh, onmode, onsignedout, oncontinue, onterminal, onside, side, showSide = false, contextKey = 'reet-session-context-panel', onresizing, children,
    contextOpen = $bindable(false), contextId, oncontextclosed, contextControls, view, nativeAvailable = false, nativePending = false, nativeReason = '', onview }: {
    /** Set when this session is part of a Factory pipeline. */
    factory?: FactoryMember
    view?: 'chat' | 'terminal'; nativeAvailable?: boolean; nativePending?: boolean; nativeReason?: string; onview?: (view: 'chat' | 'terminal') => void
    session: TerminalSession; sessions: TerminalSession[]
    machines: TerminalMachine[]; health: MachineHealth[]; projects: FolderProject[] | undefined; now: number
    theatre?: boolean; keyboard?: boolean; busy?: boolean
    /** Where the right panel's width and open state are saved, scoped to this person. */
    contextKey?: string
    /** Whether the right panel is open, shared with the workspace toolbar's Details toggle. */
    contextOpen?: boolean
    /** The element id the toolbar's Details toggle controls. */
    contextId?: string
    /** The workspace's Details toggle stays beside the session identity. */
    contextControls?: Snippet
    /** Where focus goes when the panel is closed from inside it. */
    oncontextclosed?: () => void
    /** Whether the right panel's separator is being dragged, so the terminal holds its size until it ends. */
    onresizing?: (active: boolean) => void
    onaction: (kind: 'rename' | 'end' | 'archive') => void; ontheatre: () => void; onrefresh: () => void
    /** The open chat's own word on whether it is busy (a running turn, a waiting approval or question), once its stream has
     *  replayed; until then (undefined) the row's attention stands in for it. */
    chatBusy?: boolean; insightChat?: ChatState; insightProgress?: ChatProgressState
    /** The machine switched this session to the other TUI: the workspace shows it at once and reads the listing again. */
    onmode?: (id: string, to: 'terminal' | 'chat') => void
    onsignedout?: (e: unknown) => void
    /** Present only when this closed session can be continued here. */
    oncontinue?: () => void; onterminal?: () => void; onside?: () => void; side?: Snippet; showSide?: boolean; children: Snippet
  } = $props()

  $effect(() => {
    const current = { id: session.id, title: session.title, structured: !!session.structured, model: session.model ?? '', tool: session.tool ?? '' }
    gafferSessionContext.set(current)
    return () => gafferSessionContext.update(value => value === current ? null : value)
  })

  const id = $props.id()
  const chatView = $derived(!!session.structured)
  const terminalLabel = $derived(session.tool ? `${toolName(session.tool)} TUI` : 'Terminal')

  // A phone opens a session in focus mode: the conversation and composer, or the terminal and its keys, take the screen,
  // and the site chrome, workspace toolbar and secondary session controls step aside. The choice lasts for this browser;
  // Exit focus in the header brings everything back.
  let phone = $state(typeof matchMedia === 'function' && matchMedia(PHONE_SCREEN).matches)
  // svelte-ignore state_referenced_locally (the panel key is fixed for this screen, and a browser preference needs it at once)
  const focusKey = `${contextKey}:focus`
  function savedFocus(): boolean | null {
    try { const saved = localStorage.getItem(focusKey); return saved === '1' ? true : saved === '0' ? false : null } catch { return null }
  }
  let focusWanted = $state<boolean | null>(savedFocus())
  const focusOn = $derived(phone && (focusWanted ?? true))
  // The terminal is what the stage shows: a terminal session, or a chat session switched to its agent's own TUI.
  const terminalShowing = $derived(view ? view === 'terminal' : !chatView)

  // The skills line comes from the sync state, read once when the screen opens (undefined while reading, null if it failed).
  let sync = $state<SyncState | null | undefined>(undefined)
  onMount(() => {
    const life = new AbortController()
    void loadMachineState(life.signal).then(state => { sync = state }).catch(() => { if (!life.signal.aborted) sync = null })
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
    try { localStorage.setItem(focusKey, next ? '1' : '0') } catch { /* optional browser preference */ }
    if (next) { sheetOpen = false; saveContext() }
    await tick()
    // A chat's composer takes the caret; a terminal is left alone, since focusing it would raise the keyboard over it.
    if (next && !terminalShowing) screen?.querySelector<HTMLTextAreaElement>('.composer-input')?.focus({ preventScroll: true })
    else focusButton?.focus()
  }

  // The panel's saved width and open state, and the room the screen actually gives it (the container query's 1100 px
  // is measured on this element, so the docked decision matches what CSS used to decide).
  onMount(() => {
    // The saved width was read above, before the first paint, so the waiting skeleton and the panel agree.
    if (!screen) return
    const measure = () => { screenWidth = screen!.clientWidth }
    const observer = new ResizeObserver(measure)
    observer.observe(screen); measure()
    return () => observer.disconnect()
  })

  // Opening a session marks it seen on the controller so its Needs you card clears across devices.
  let lastMarkedId = ''
  // The session the surfaces were last told about. It is cleared only once the session is seen as seen, so one unseen
  // stretch signals once: a session the server keeps calling unseen (a bell that keeps ringing, a shell still printing)
  // is re-marked on each refresh but must not signal each time, or every refresh would trigger the next.
  let signalledFor = ''
  $effect(() => {
    const sid = session.id
    if (!session.changedSinceSeen && signalledFor === sid) signalledFor = ''
    if (!sid || (sid === lastMarkedId && !session.changedSinceSeen && session.attention !== 'waiting')) return
    lastMarkedId = sid
    if (session.changedSinceSeen || session.attention === 'waiting') {
      // Read before the await: only clearing something the surfaces were counting changes what they show (a busy shell is
      // unseen again whenever its terminal moves but is never counted). A failed mark signals nothing.
      const wasCounted = !!session.changedSinceSeen && attentionRuleOf(session, Date.now()) !== null
      void markSessionSeen(sid).then(() => {
        if (wasCounted && signalledFor !== sid) { signalledFor = sid; signalAttentionChanged() }
      }, () => {})
    }
  })

  const host = $derived(machines.find(m => m.agentId === session.agentId)?.host || health.find(m => m.agentId === session.agentId)?.host || 'Unavailable machine')
  const reported = $derived(health.find(m => m.agentId === session.agentId))
  // The fleet strip's own figures for this machine: its reporting state and its skills line.
  const card = $derived.by<FleetCard | undefined>(() => {
    const entry: MachineHealth = reported ?? { agentId: session.agentId, host, state: '' }
    return sync ? fleetCards([entry], sessions, sync, now)[0] : reported ? fleetCards([entry], sessions, { environments: [] }, now)[0] : undefined
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
    const read = () => { connection = stage?.querySelector('[data-session-presentation]:not([hidden]) .pane, :scope > .pane')?.getAttribute('data-connection-state') ?? '' }
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

  // Approvals: a structured chat can switch between asking for every card and YOLO, which answers each one itself (for Codex, only in its ask-before-leaving-the-sandbox mode).
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
      detail: structuredAgent(machineCaps, session.tool)?.approval ? 'update reet on the machine if this is ever refused' : SWITCH_LIVE_CHAT_HINT(session.tool ?? '') },
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

  // Legacy process-changing interface switch; the header's Chat / Terminal control only changes the shared view.
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
  // Read before the first paint: the waiting skeleton draws the panel at this width, not at a default that moves later.
  const initialContext = (() => { try { return readContextLayout(localStorage.getItem(contextKey) ?? '') } catch { return readContextLayout(null) } })()
  let contextWidth = $state(initialContext.width ?? CONTEXT_DEFAULT), bodyWidth = $state(0), layoutLoaded = $state(true)
  // Docked and sheet have their own open state: a docked panel starts closed until asked for, a sheet only when asked for.
  let dockedOpen = $state(initialContext.open ?? false), sheetOpen = $state(false)
  try { localStorage.setItem('reet.workspace.context', JSON.stringify(contextLayoutRecord(initialContext.open ?? false, initialContext.width ?? CONTEXT_DEFAULT))) } catch { /* optional browser preference */ }
  const docked = $derived(!theatre && screenWidth > 1099)
  const hasSide = $derived(showSide && !!side)
  const effectiveOpen = $derived(docked ? dockedOpen : sheetOpen)
  // The workspace toolbar's toggle shows this state; it never writes it, so the two open states stay separate.
  $effect(() => { contextOpen = effectiveOpen })
  const panelId = $derived(contextId ?? `${id}-context`)
  // Focus keeps the conversation alone on the screen, so a Details sheet opened before it goes away.
  $effect(() => { if (focusOn) sheetOpen = false })
  // The separator's 8 px track sits between the terminal and the panel; the panel takes at most 40 % of what is left.
  const CONTEXT_TRACK = 8
  const contextRoom = $derived(Math.max(0, bodyWidth - CONTEXT_TRACK))
  const shownContextWidth = $derived(clampContextWidth(contextWidth, contextRoom))
  const contextMax = $derived(widestContext(contextRoom))
  function saveContext() {
    if (!layoutLoaded) return
    const record = JSON.stringify(contextLayoutRecord(dockedOpen, contextWidth))
    try { localStorage.setItem(contextKey, record); localStorage.setItem('reet.workspace.context', record) } catch { /* optional browser preference */ }
  }
  // The workspace toolbar calls this through its bound component instance.
  export function toggleContext() {
    // A terminal opened beside the session takes the panel's place; Details has nothing to toggle until it closes.
    if (hasSide) return
    if (docked) dockedOpen = !dockedOpen; else sheetOpen = !sheetOpen
    saveContext()
    if (docked || !sheetOpen) return
    // Focus moves into the sheet (its Close details first), so it never stays on a control the sheet could cover.
    void tick().then(() => document.getElementById(panelId)?.querySelector<HTMLElement>('.close-details')?.focus())
  }
  // The panel's own collapse control, and the sheet's Close details: the panel goes and focus returns to Details.
  async function collapseDetails() {
    if (docked) dockedOpen = false; else sheetOpen = false
    saveContext()
    await tick()
    oncontextclosed?.()
  }
  // Leaving the panel for the terminal only dismisses a sheet; a docked panel stays where the person put it.
  function closeSheet() { if (!docked) { sheetOpen = false; saveContext() } }
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

  // Dragging the right separator: the shared PanelResizer captures the pointer, so leaving the handle keeps the drag, and
  // moving left widens this panel. The terminal holds its size and refits once when the drag ends.
  let contextResizing = $state(false)
  function setContextWidth(width: number) { contextWidth = clampContextWidth(width, contextRoom) }
  function commitContextWidth(width: number) { contextWidth = clampContextWidth(width, contextRoom); saveContext() }
  function setContextResizing(active: boolean) { contextResizing = active; onresizing?.(active) }

  // Infrequent session controls share one sheet. Close it before another confirmation opens, keeping focus on the
  // persistent header button rather than a control that is about to unmount.
  let actionsOpen = $state(false), actionsButton = $state<HTMLButtonElement>()
  function closeActions() { actionsOpen = false; void tick().then(() => actionsButton?.focus()) }
  function leaveActions() { actionsOpen = false; actionsButton?.focus() }

  function act(kind: 'rename' | 'end' | 'archive') { leaveActions(); onaction(kind) }
  function theatreView() {
    actionsOpen = false
    ontheatre()
    void tick().then(() => actionsButton?.focus())
  }
  function continueSession() { if (!oncontinue) return; leaveActions(); oncontinue() }
  // Takes the person to the terminal below the card (the plain shell's own input).
  function focusTerminal() { void tick().then(() => stage?.querySelector<HTMLElement>('.xterm-helper-textarea, textarea.composer-input')?.focus()) }
  function openTerminal() { if (!onterminal) return; leaveActions(); onterminal() }
  function openSideTerminal() { if (!onside) return; leaveActions(); onside() }

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
  function askMode() { if (approvalMode === 'yolo') { leaveActions(); void setMode('ask') } }
  function yoloMode() {
    if (approvalMode === 'yolo' || modeBusy) return
    leaveActions()
    yoloOpen = true
  }
  function turnOnYolo() {
    if (!yoloCanStart(switchGates)) return
    yoloOpen = false
    void setMode('yolo')
  }
  async function copyBranch() {
    const branch = session.worktreeBranch
    if (!branch) return
    leaveActions()
    notice = (await copyText(branch)) ? { text: `Copied ${branch}.` } : { text: "Couldn't copy the branch. Select it in Where it's working instead.", bad: true }
  }
  // Check and remove worktree: the existing sheet, which reads the worktree's state first and removes nothing unsaved.
  let removing = $state<RunMember | null>(null)
  function checkWorktree() {
    const t = target
    if (!t) return
    leaveActions()
    removing = { position: 0, host: session.agentId, dir: '', tool: session.tool ?? '', model: session.model ?? '', state: 'started', code: '', sentence: '', sessionId: session.id,
      worktreePath: session.worktreePath ?? '', worktreeRepo: t.repo, worktreeBranch: session.worktreeBranch ?? '', worktreeRun: t.run, worktreeProject: t.project, sharesCheckout: false, attempt: 0 }
  }
  // A new session's screen starts with its own words: the last one's notice does not carry over.
  let shownId = ''
  $effect(() => { if (session.id !== shownId) { shownId = session.id; notice = null; actionsOpen = false } })
</script>

<!-- One grid: the header and status line, then the context panel, then the terminal, so the tab order is header → panel →
     terminal while the panel shows beside the terminal (right column) or over its edge (as a sheet below the header). -->
<div class="session-screen" class:theatre class:keyboard class:focus={focusOn} class:terminal-view={terminalShowing} class:resizing={contextResizing}
  bind:this={screen} style={`--context-width:${shownContextWidth}px`}>
  <div class="body" class:context-docked={docked && effectiveOpen && !hasSide} class:context-sheet={!theatre && !docked && effectiveOpen && !hasSide} bind:clientWidth={bodyWidth}>
    <header class="head">
      <div class="identity-row">
      <a class="back" href="/sessions" aria-label="Back to sessions"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg></a>
      <div class="heading">
        <h2 class="title" title={session.title}>{session.title}</h2>
        {#if factory}<a class="factory-mark" href={factory.key ? `/factory#pipeline-${factory.outcomeId}-${factory.key}` : '/factory'} title={`Part of a Factory pipeline: ${factory.pipeline}. Outcome: ${factory.objective}`}>{factoryBadge(factory)}</a>{/if}
        <p class="sub">{headerMeta(view === 'terminal' && chatView ? { ...session, model: '' } : session, session.createdAt, now)}</p>
        <p class="location">{[project, host].filter(Boolean).join(' · ')}</p>
        <p class="where-phone">{[word.word, project, host].filter(Boolean).join(' · ')}</p>
      </div>
      <span class="pill" data-status={statusKind} title={source}>{#if statusKind === 'finished'}<span class="status-tick" aria-hidden="true"></span>{:else}<span class="dot" aria-hidden="true"></span>{/if}{word.word}{#if away}{' '}<span class="lk">· last known</span>{/if}<span class="vh"> · {word.source}</span></span>
      <div class="header-actions">
        {#if onview}<SessionViewSwitch view={view ?? (chatView ? 'chat' : 'terminal')} chat={chatView} terminal={!chatView || nativeAvailable} pending={nativePending} reason={nativeReason && chatView && switchState.enabled ? `${nativeReason} Click to restart it in the ${terminalLabel}.` : nativeReason} {onview} onunavailable={text => { if (chatView && !nativePending && switchState.enabled && switchState.to === 'terminal') openSwitch(document.activeElement as HTMLElement | null); else notice = { text } }} />{/if}
        {@render contextControls?.()}
        {#if (terminalShowing || focusOn) && $gafferChatEntry}<button type="button" class="gaffer-entry" aria-label="Ask Gaffer" title="Ask Gaffer about this session" onclick={event => $gafferChatEntry?.(event.currentTarget)}><GafferCompanionAvatar size={24} /></button>{/if}
        <button type="button" class="actions-button" data-keeps-focus bind:this={actionsButton} aria-label="Session actions" aria-haspopup="dialog" aria-expanded={actionsOpen}
          title="Session actions" onclick={() => actionsOpen = true}><span aria-hidden="true">···</span></button>
        <button type="button" class="focus-button" bind:this={focusButton} aria-pressed={focusOn}
          aria-label={focusOn ? 'Exit focus mode' : 'Focus mode'} title={focusOn ? 'Show the full session screen' : terminalShowing ? 'Give the screen to the terminal' : 'Focus on the conversation and composer'}
          onclick={() => void toggleFocus()}>
          <TerminalIcon name={focusOn ? 'contract' : 'expand'} /><span class="wide-label">{focusOn ? 'Exit focus' : 'Focus'}</span><span class="compact-label">{focusOn ? 'Exit' : 'Focus'}</span>
        </button>
      </div>
      </div>
      {#if actionsOpen}
      <WorkspaceDialog title="Session actions" subtitle={`${session.title} · ${host}`} onclose={closeActions}>
      <div class="session-tools" role="group" aria-label="Session controls" data-keeps-focus>
        {#if canSwitchApproval}
          <section class="panel-section">
            <div class="section-heading">
              <h3>Approvals</h3>
              <p>{approvalMode === 'yolo' ? 'YOLO answers every tool call without asking you first.' : 'Ask me reviews each tool call before the agent can run it.'}</p>
            </div>
            <div class="approvals" role="group" aria-label="Approvals">
              <button type="button" aria-pressed={approvalMode === 'ask'} disabled={busy || modeBusy} onclick={askMode}>Ask me</button>
              <button type="button" aria-pressed={approvalMode === 'yolo'} disabled={busy || modeBusy}
                title={yoloCanStart(switchGates) ? yoloSwitchTitle(session.tool ?? '') : 'Read why YOLO is not ready here'} onclick={yoloMode}>YOLO</button>
            </div>
          </section>
        {:else if session.yolo}
          <section class="panel-section">
            <div class="section-heading"><h3>Approvals</h3><p>This session answers tool calls without asking.</p></div>
            <span class="badge yolo">YOLO</span>
          </section>
        {/if}

        <section class="panel-section">
          <div class="section-heading">
            <h3>Interface</h3>
            <p>Switching restarts the agent process. When the header offers Chat or Terminal, that changes the view without a restart.</p>
          </div>
          <div class="control-row">
            {#if canSwitch || modeSwitching}
              <label class="tui-picker">
                <span class="vh">Session interface</span>
                <select use:gafferTooltip aria-label="Session interface" value={chatView ? 'chat' : 'terminal'} aria-disabled={modeSwitching || undefined}
                  title={modeSwitching ? whyNot : 'Choose Gaffer or the agent TUI'}
                  onchange={event => { const pick = event.currentTarget.value, current = chatView ? 'chat' : 'terminal'; event.currentTarget.value = current; if (pick !== current && canSwitch) openSwitch(event.currentTarget) }}>
                  <option value="chat">Gaffer</option>
                  <option value="terminal">{terminalLabel}</option>
                </select>
              </label>
            {:else}
              <span class="tui-pill" use:gafferTooltip={chatView} title={whyNot} aria-label={`${chatView ? 'Gaffer' : terminalLabel}. ${whyNot}`}>{chatView ? 'Gaffer' : terminalLabel}</span>
            {/if}
            <span class="vh" id={`${id}-switch-why`}>{whyNot}</span>
            <button type="button" class="theatre-button" aria-pressed={theatre} aria-label={theatre ? 'Exit theatre view' : 'Theatre view'} title={theatre ? 'Restore the session panels' : 'Hide the panels and focus on this session'} onclick={theatreView}>
              <TerminalIcon name="theatre" />{theatre ? 'Exit theatre view' : 'Theatre view'}
            </button>
          </div>
        </section>

        <section class="panel-section">
          <div class="section-heading">
            <h3>Manage session</h3>
            <p>Rename this session, copy its branch, or work in another terminal beside it.</p>
          </div>
          <div class="action-grid">
            <button type="button" class="tool-action action-tile" disabled={busy || serverOff} onclick={() => act('rename')}><TerminalIcon name="compose" />Rename…</button>
            {#if session.worktreeBranch}<button type="button" class="tool-action action-tile secondary-action" onclick={() => void copyBranch()}><TerminalIcon name="copy" />Copy branch</button>{/if}
            {#if live && onterminal}<button type="button" class="tool-action action-tile" aria-label="Terminal here" disabled={busy || serverOff} onclick={openTerminal}><TerminalIcon name="screen" /><span class="here-label">Terminal here</span></button>{/if}
            {#if live && onside}<button type="button" class="tool-action action-tile secondary-action" disabled={busy || serverOff || showSide} onclick={openSideTerminal}><TerminalIcon name="panel" />{showSide ? 'Terminal open beside' : 'Terminal beside'}</button>{/if}
            {#if target}<button type="button" class="tool-action action-tile secondary-action" disabled={live || away} title={live ? 'End the session first: it is working in this worktree.' : away ? offlineSentence(host) : undefined} onclick={checkWorktree}><TerminalIcon name="delete" />Check and remove worktree…</button>{/if}
            {#if oncontinue}<button type="button" class="tool-action action-tile" disabled={busy} onclick={continueSession}><TerminalIcon name="history" />{session.structured ? 'Continue with history' : 'Continue session'}</button>{/if}
          </div>
        </section>

        {#if live || terminalArchivable(session.state)}
          <section class="panel-section danger-zone">
            <div class="section-heading">
              <h3>End session</h3>
              <p>This stops the shell and its running jobs. The worktree and its branch stay on the machine.</p>
            </div>
            <div class="control-row">
              {#if terminalArchivable(session.state)}<button type="button" class="tool-action" disabled={busy} onclick={() => act('archive')}><TerminalIcon name="history" />Remove from history…</button>{/if}
              {#if live}<button type="button" class="tool-action danger-solid" disabled={busy || serverOff} onclick={() => act('end')}>{session.state === 'end_requested' ? 'Retry end request…' : 'End session…'}</button>{/if}
            </div>
          </section>
        {/if}
      </div>
      </WorkspaceDialog>
      {/if}
    </header>
    <!-- A Factory pipeline's decision is taken where the work is: in the session doing its stage in hand. -->
    <div class="below-head">
    <!-- A terminal agent's open dialog sits right under the header, above the terminal, so it is the first thing seen. -->
    {#if session.terminalQuestion && !session.structured}<div class="question"><TerminalQuestionCard {session} question={session.terminalQuestion} title={session.title} machine={host} locked={away ? 'The machine is not connected. Try again when it is back.' : ''} onopen={focusTerminal} onreread={onrefresh} /></div>{/if}
    {#if factory?.key && factory.current}<SessionDecision member={factory} sessionId={session.id}/>{/if}
    <!-- Always rendered (empty takes no room), so the first message in it is announced. -->
    <div class="live" role="status">{#if notice}<p class:bad={notice.bad}>{notice.text}</p>{/if}</div>
    </div>
    {#if showSide && side}
      <div class="side-terminal" data-keeps-focus>{@render side()}</div>
    {:else}
      {#key session.id}
        <ContextPanel {session} {host} {status} {project} {skills} {now} chat={insightChat} chatProgress={insightProgress} supported={!!machineCaps?.inspect}
          docked={docked}
          onterminal={() => { closeSheet(); void tick().then(() => stage?.querySelector<HTMLElement>('.xterm-helper-textarea, textarea.composer-input')?.focus()) }}
          id={panelId} onkeydown={panelKey} onclose={() => void collapseDetails()} />
      {/key}
      {#if docked && effectiveOpen}
        <!-- The right panel's separator, the same control as every other panel: drag (pointer capture), Left widens,
             Right narrows, Home the narrowest, End the widest, double-click 360 px. -->
        <PanelResizer value={shownContextWidth} min={CONTEXT_MIN} max={contextMax} resetValue={CONTEXT_DEFAULT}
          label="Resize session workspace" controls={panelId} side="right" mode="inline"
          position="grid-column: 2; grid-row: 1 / -1; margin: 0 -8px 0 0" extraClass="panel-separator context-separator"
          onchange={setContextWidth} oncommit={commitContextWidth} onresizing={setContextResizing} />
      {/if}
    {/if}
    <div class="stage" bind:this={stage}>{@render children()}</div>
  </div>
</div>

{#if removing}
  <WorktreeDialog member={removing} {host} opener={null} onremoved={onrefresh} {onsignedout} onclose={() => (removing = null)} />
{/if}
{#if switchAsk}
  <SwitchModeDialog to={switchAsk.to} agentTui={terminalLabel} machine={host} trust={switchAsk.trust} sandboxOff={session.tool === 'codex' && !!session.yolo}
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
  .below-head { grid-column: 1; grid-row: 2; min-width: 0; min-height: 0; }
  .question { padding: 8px 20px 0; }
  /* A tall dialog never squeezes the terminal out: the holder caps at 40% of the height and scrolls. */
  .below-head:has(.question) { max-height: 40dvh; overflow-y: auto; overscroll-behavior: contain; }
  /* The long dialog is the thing to read: the holder allows it more of a phone's height. */
  @media (max-width: 767px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) { .below-head:has(.question) { max-height: 70dvh; } }
  /* A long Factory decision sits above the conversation. On a phone (or a sideways one) it keeps a bounded height and
     scrolls itself, so its own choices stay reachable instead of being clipped by the fixed-height workspace. */
  @media (max-width: 767px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .below-head { max-height: 55dvh; overflow-y: auto; overscroll-behavior: contain; }
    /* While a long decision scrolls, its choices stay at the foot of the block: the next step is always in reach. */
    .below-head :global(.choices) { position: sticky; bottom: 0; z-index: 1; background: var(--card); box-shadow: 0 -10px 14px -10px var(--scrim); }
  }
  .stage { grid-column: 1; grid-row: 3; }
  /* The context panel is hidden until JS says it is docked or shown as a sheet; either way it keeps the same cell it
     always had beside the terminal's right edge. */
  .body > :global(.context-panel) { display: none; grid-column: 2; grid-row: 1 / -1; min-width: 0; }
  .body.context-docked { grid-template-columns: minmax(0, 1fr) 8px var(--context-width); }
  .body.context-docked > :global(.context-panel) { display: flex; grid-column: 3; }
  .body.context-sheet > :global(.context-panel) { display: flex; grid-column: 1; grid-row: 3; justify-self: end; position: relative; z-index: 20; width: min(340px, 100%); box-shadow: var(--shadow); }
  .body.context-sheet > :global(.context-panel .close-details) { display: inline-flex; align-items: center; }
  .session-screen.resizing { cursor: col-resize; user-select: none; }
  .side-terminal { grid-column: 2; grid-row: 1 / -1; width: 340px; min-width: 0; min-height: 0; display: flex; flex-direction: column; border-left: 1px solid var(--line-soft); background: var(--bg-sunk); }
  .side-terminal :global(.pane) { flex: 1; min-height: 0; }
  .side-terminal :global(.pane > header), .side-terminal :global(.pane > footer) { display: none; }
  /* Identity stays separate from controls, so long session names never push frequent actions into a menu. */
  .head { min-width: 0; padding: 0 20px 8px; box-sizing: border-box; }
  .identity-row { display: flex; align-items: center; gap: 12px; min-height: 88px; min-width: 0; }
  .header-actions { display: flex; align-items: center; gap: 6px; flex: none; }
  /* The session actions sheet: grouped sections, one visual language for every control, and destructive work kept apart. */
  .session-tools { display: grid; gap: 18px; min-width: 0; }
  .panel-section { display: grid; gap: 10px; padding-top: 18px; border-top: 1px solid var(--line-soft); min-width: 0; }
  .panel-section:first-child { padding-top: 0; border-top: 0; }
  .section-heading { display: grid; gap: 2px; min-width: 0; }
  .section-heading h3 { margin: 0; font: 600 14px/1.3 var(--sans); color: var(--ink); }
  .section-heading p { margin: 0; font-size: 14px; line-height: 1.5; color: var(--muted); text-wrap: pretty; }
  .control-row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
  .action-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 8px; }
  .action-tile { justify-content: flex-start; width: 100%; text-align: left; }
  .actions-button { display: inline-grid; place-items: center; min-width: 36px; min-height: 36px; border: 1px solid transparent; border-radius: 7px; background: none; color: var(--muted); font: 600 20px/1 var(--sans); cursor: pointer; }
  @media (pointer: coarse) { .header-actions :global(button) { min-width: 44px; min-height: 44px; } }
  .actions-button:hover { background: var(--line-soft); color: var(--ink); }
  .actions-button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .tool-action, .theatre-button, .focus-button { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 40px; padding: 0 12px; border: 1px solid var(--line); border-radius: 9px; background: none; color: var(--ink-soft); font: 500 14px var(--sans); cursor: pointer; }
  .tool-action :global(svg), .theatre-button :global(svg) { flex: none; }
  .tool-action:hover:not(:disabled), .theatre-button:hover:not(:disabled), .focus-button:hover { background: var(--tint); color: var(--ink); }
  .tool-action:disabled, .theatre-button:disabled { opacity: .5; cursor: default; }
  .focus-button { display: none; }
  /* A phone or tablet terminal has no corner for the floating Ask Gaffer circle (it would sit on the agent's prompt), so the header carries it. */
  .gaffer-entry { display: none; align-items: center; justify-content: center; width: 44px; height: 44px; flex: none; padding: 0; border: 1px solid transparent; border-radius: 50%; background: none; cursor: pointer; }
  .gaffer-entry:hover { background: var(--tint); }
  .gaffer-entry:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (max-width: 767px), (pointer: coarse) { .gaffer-entry { display: inline-flex; } }
  .compact-label { display: none; }
  .danger-zone { padding: 14px; border: 1px solid color-mix(in srgb, var(--bad) 35%, var(--line)); border-radius: 12px; background: color-mix(in srgb, var(--bad) 5%, transparent); }
  .danger-zone .section-heading h3 { color: var(--bad); }
  .danger-solid { background: var(--bad); border-color: var(--bad); color: var(--bg); font-weight: 600; }
  .danger-solid:hover:not(:disabled) { background: color-mix(in srgb, var(--bad) 88%, var(--ink)); border-color: color-mix(in srgb, var(--bad) 88%, var(--ink)); color: var(--bg); }
  .theatre-button[aria-pressed=true] { border-color: var(--accent-text); background: var(--line-soft); color: var(--ink); }
  .tool-action:focus-visible, .theatre-button:focus-visible, .focus-button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .heading { flex-grow: 1; min-width: 0; }
  /* Back to the Sessions screen: the phone's slim header has it (the workspace toolbar's arrow gives way there). */
  .back { display: none; flex: none; width: 44px; height: 44px; align-items: center; justify-content: center; color: var(--ink-soft); border-radius: 8px; }
  .back svg { width: 22px; height: 22px; }
  .back:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .title { margin: 0; font: 500 20px/1.3 var(--sans); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .location { margin: 3px 0 0; color: var(--muted); font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sub, .where-phone { margin: 0; font-size: 14px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .where-phone { display: none; }
  .pill { flex: none; display: inline-flex; align-items: center; gap: 8px; height: 30px; padding: 0 12px; border-radius: 6px; box-sizing: border-box;
    background: none; border: 0; color: var(--muted); font-size: 14px; font-weight: 400; white-space: nowrap; }
  .pill .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--status); }
  .badge { flex: none; padding: 0 8px; border: 1px solid var(--line-strong); border-radius: 6px; font-size: 14px; line-height: 22px; color: var(--muted); }
  .badge.yolo { border-color: var(--warn); color: var(--warn); font-weight: 600; }
  .approvals { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); }
  .approvals button { min-height: 38px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--muted); font: 500 14px var(--sans); cursor: pointer; }
  .approvals button[aria-pressed=true] { background: var(--card-raised); color: var(--ink); font-weight: 600; box-shadow: 0 1px 2px color-mix(in srgb, var(--ink) 16%, transparent); }
  .approvals button[aria-pressed=true]:last-child { background: color-mix(in srgb, var(--warn) 20%, var(--card-raised)); color: var(--warn); box-shadow: none; }
  .approvals button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .approvals button:disabled { opacity: .5; cursor: default; }
  .tui-picker { flex: none; display: inline-flex; align-items: center; min-width: 0; }
  .tui-picker select { min-height: 40px; padding: 0 32px 0 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); color: var(--ink-soft); font: 500 14px var(--sans); cursor: pointer; }
  .tui-picker select:disabled, .tui-picker select[aria-disabled=true] { opacity: .55; cursor: progress; }
  .tui-picker select:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .tui-pill { flex: none; display: inline-flex; align-items: center; min-height: 40px; padding: 0 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--line-soft); color: var(--muted); font: 500 14px var(--sans); }
  .live p { margin: 0 28px 8px; font-size: 14px; color: var(--muted); }
  .live p.bad { color: var(--bad); }
  .stage { min-height: 0; display: flex; flex-direction: column; overflow: hidden; border-top: 1px solid var(--line-soft); }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @media (pointer: coarse) { .tool-action, .theatre-button { min-width: 44px; min-height: 44px; } .tui-picker select, .approvals button { min-height: 44px; } }
  /* A narrow sheet stacks the action tiles and lets each control fill the width, so nothing is squeezed into a column
     too small for its name. */
  @media (max-width: 520px) {
    .action-grid { grid-template-columns: minmax(0, 1fr); }
    .control-row { align-items: stretch; }
    .control-row .tui-picker, .control-row .tui-picker select, .control-row .tui-pill, .control-row .theatre-button { flex: 1 1 100%; width: 100%; }
  }
  @container session-screen (max-width: 1099px) {
    .head { padding: 0 12px 8px; }
    .heading { flex: 1 1 0; }
  }
  /* A narrow stage that is not a phone (a tablet with the sessions panel docked): the name keeps the first row to itself
     and its state, and the controls take the next, so a session is never called "Refacto…" beside empty room. */
  @container session-screen (max-width: 640px) {
    .identity-row { flex-wrap: wrap; min-height: 64px; }
    .header-actions { width: 100%; }
  }
  /* Phones retain every control. Layout actions use 44 px icons; the toolbar wraps instead of hiding choices. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .sub, .location, .identity-row > .pill { display: none; }
    .where-phone { display: block; font-size: 14px; }
    .back { display: flex; }
    .head { padding: 0 6px 4px; }
    .identity-row { min-height: 44px; gap: 4px; flex-wrap: wrap; padding-bottom: 2px; }
    .header-actions { width: 100%; justify-content: flex-end; }
    .header-actions :global(.view-switch) { margin-right: auto; }
    .header-actions { gap: 2px; }
    .header-actions .actions-button { min-width: 44px; min-height: 44px; }
    .header-actions .focus-button { width: 44px; min-width: 44px; min-height: 44px; padding: 0; display: inline-flex; }
    .header-actions button { flex-direction: column; gap: 1px; }
    .header-actions button span { display: block; font: 500 14px/12px var(--sans); }
    .header-actions button .wide-label { display: none; }
    .header-actions :global(svg) { width: 19px; height: 19px; }
    .session-tools { gap: 16px; }
    .tui-picker select { min-height: 44px; font-size: 14px; }
    .approvals button { padding: 6px 10px; font-size: 14px; min-height: 44px; }
    .tool-action { min-height: 44px; padding: 0 10px; }
    .heading { flex: 1 1 0; }
    .title { font: 600 15px/1.3 var(--sans); }
    .pill { height: 28px; padding: 0 10px; gap: 6px; font-size: 14px; }
    .pill .lk { display: none; }
    .badge { font-size: 14px; line-height: 18px; padding: 0 6px; }
  }
  @media (max-width: 380px) {
    .header-actions :global(.view-switch svg) { display: none; }
  }
  /* Super focus on a phone: one slim identity row, then the conversation and composer with nothing competing for the room.
     The site chrome and workspace toolbar are hidden through data-session-focus and the workspace's own rule. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .session-screen.focus .head { padding: 0 6px; padding-top: env(safe-area-inset-top, 0px); }
    .session-screen.focus .identity-row { min-height: 44px; gap: 6px; }
    .session-screen.focus .heading { min-width: 0; max-width: none; flex: 1 1 100px; }
    .session-screen.focus .title { font: 600 16px/1.25 var(--sans); }
    .session-screen.focus .sub, .session-screen.focus .where-phone { display: none; }
    .session-screen.focus .identity-row > .pill { display: inline-flex; }
    /* A chat's session actions wait behind Exit focus. A terminal keeps them: its pane header, where End and Rename
       also live, has stepped aside. */
    .session-screen.focus:not(.terminal-view) .actions-button { display: none; }
    .session-screen.focus .header-actions { gap: 0; }
    .session-screen.focus .header-actions .focus-button { width: 44px; min-width: 44px; min-height: 44px; }
    /* A chat in focus has one header row: Back, the name, the state, Ask Gaffer and Exit focus. The view switch and
       Details wait behind Exit focus, as the session's actions do. A terminal keeps its switch: it is the way back. */
    .session-screen.focus:not(.terminal-view) .header-actions :global(.view-switch),
    .session-screen.focus:not(.terminal-view) .header-actions :global(.details-toggle) { display: none; }
    .session-screen.focus:not(.terminal-view) .identity-row { flex-wrap: nowrap; }
    .session-screen.focus:not(.terminal-view) .header-actions { width: auto; flex: none; }
    /* The agent's state is said once, in the strip above the composer, which also carries the context left. The line
       under the header says the same while the chat is simply working or idle, so it gives up its row then; it stays
       for a reconnect, a failure or a waiting approval, and is always there for a screen reader. Ask Gaffer, which
       sat on that line, is in the header. */
    .session-screen.focus :global(.chat-status.plain) { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; border: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .session-screen.focus :global(.chat-status .chat-actions) { display: none; }
    /* The composer keeps its message field with Stop and Queue. Prompts, Skills, an image, dictation, effort, model
       and source wait behind Exit focus. */
    .session-screen.focus :global(.composer-bar) { display: none; }
    /* The queue is one line that opens (ChatQueue). */
    .session-screen.focus :global(.server-queue) { padding: 0; margin-bottom: 6px; }
    .session-screen.focus :global(.server-queue .queue-summary) { display: flex; }
    .session-screen.focus :global(.server-queue:not(.open) > header),
    .session-screen.focus :global(.server-queue:not(.open) > ol),
    .session-screen.focus :global(.server-queue:not(.open) > [role=alert]) { display: none; }
    .session-screen.focus :global(.server-queue.open) { padding: 0 10px 10px; }
    .session-screen.focus :global(.server-queue.open .queue-summary) { margin: 0 -10px; width: calc(100% + 20px); }
    .session-screen.focus :global(.server-queue.open > header) { display: none; }
    .session-screen.focus .live p { margin: 0 8px 6px; }
    .session-screen.focus .stage { border-top: 0; }
    /* A terminal in focus: the slim header names the session and its state, so the pane's own header and status footer give
       their rows to the screen. Not while the connection is down: they hold Retry now, Reconnect and the reason. */
    .session-screen.focus .stage :global(.pane:not(.maximised)[data-connection-state=live] > header),
    .session-screen.focus .stage :global(.pane:not(.maximised)[data-connection-state=live] > footer) { display: none; }
    /* The narrative is the screen: larger prose, a tighter log, and the composer's secondary tools wait behind Exit focus. */
    /* Keep live status and the nearby Gaffer entry visible in focus mode too. */
    .session-screen.focus :global(.log) { padding: 12px 14px 16px; gap: 10px; }
    .session-screen.focus :global(.bubble.agent) { font-size: 15px; line-height: 1.68; }
    .session-screen.focus :global(.row.user) { margin-top: 14px; }
    .session-screen.focus :global(.row.user .bubble) { max-width: 90%; }
    /* The workspace already stops above the home indicator (TerminalWorkspace's measure). */
    .session-screen.focus :global(.composer) { margin: 0 8px 8px; padding: 8px; }
    .session-screen.focus :global(.hint) { display: none; }
    .session-screen.focus :global(.tool-head) { min-height: 44px; }
  }
  /* A very short viewport (a phone with the keyboard up, or held sideways): keep the controls on one scrolling row so
     the terminal keeps the room it needs. */
  @media (max-height: 500px) {
    .identity-row { min-height: 54px; }
  }
  /* In landscape keep the controls reachable and trim secondary metadata. */
  @media (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .head { display: flex; flex-wrap: nowrap; align-items: center; gap: 4px 8px; padding: 0 4px 2px; }
    .identity-row { flex: 1 1 190px; min-width: 0; min-height: 44px; flex-wrap: nowrap; padding-bottom: 0; }
    .header-actions { width: auto; margin-left: auto; }
    .heading { max-width: 100px; min-width: 0; }
    .where-phone { display: none; }
  }
  /* With the phone keyboard up the site's chrome steps aside (keyboardChrome) and the slim header stays, as the mock-up's phone
     session draws it; the status line gives its row to the terminal. */
  .keyboard .live { display: none; }
  /* In focus on a phone the header's second row (the view switch, Details, Exit focus) waits too: one row names the
     session, and the rest of the screen above the keyboard is the conversation or the terminal. */
  @media (max-width: 650px) { .session-screen.focus.keyboard .header-actions { display: none; } }
  /* The slim header names the session and its state, so the pane's own header gives its row to the terminal while typing. */
  @media (max-width: 650px) {
    /* Not while the connection is down: the header holds Retry now, Reconnect and Continue here. */
    .keyboard .stage :global(.pane:not(.maximised)[data-connection-state=live] > header) { display: none; }
    /* The header keeps its 44 px targets but drops its spare 8 px while typing, so the terminal keeps its keyboard-open rows. */
    .keyboard .head { min-height: 44px; }
  }
  /* A touch tablet with its keyboard up: the same idea at tablet sizes. The header keeps the name, state and controls on
     one slim row, and the pane's own header and status footer give their rows to the terminal. */
  @media (pointer: coarse) and (min-width: 651px) and (min-height: 501px) {
    .keyboard .stage :global(.pane:not(.maximised)[data-connection-state=live] > header),
    .keyboard .stage :global(.pane:not(.maximised)[data-connection-state=live] > footer) { display: none; }
    .keyboard .identity-row { min-height: 52px; flex-wrap: nowrap; }
    .keyboard .header-actions { width: auto; }
    .keyboard .sub, .keyboard .location { display: none; }
    .keyboard .head { padding-bottom: 2px; }
  }
  .factory-mark { justify-self: start; align-self: start; width: fit-content; font-size: 14px; font-weight: 600; color: var(--accent-text); border: 1px solid var(--accent); border-radius: 6px; padding: 1px 8px; text-decoration: none; white-space: nowrap; }
  .factory-mark:hover { background: var(--tint); }
  .factory-mark:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
