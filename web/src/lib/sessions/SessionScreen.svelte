<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte'
  import { getSyncState, getYoloSetting, type SyncState, type YoloSetting } from '../api'
  import { copyText } from '../clipboard'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import { fleetCards, stateWord, type FleetCard } from '../home'
  import type { RunMember } from '../projects'
  import { headerMeta, offlineSentence, switchModeState, tabFromMenu, worktreeTargetOf } from '../sessions'
  import { toolName } from '../agentLaunch'
  import { CATALOGUE } from '../catalogue'
  import YoloDialog from '../launch/YoloDialog.svelte'
  import { yoloCanStart, type YoloGate } from '../launchBar'
  import { structuredAgent, markSessionSeen, setApprovalMode, setSessionMode, terminalClosed, TerminalError, type ApprovalMode, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import ContextPanel from './ContextPanel.svelte'
  import SwitchModeDialog from './SwitchModeDialog.svelte'

  // The session screen: the strip of open sessions, the session's header (title, agent · model · started, the state in words
  // with its source, YOLO, reet TUI | the agent TUI and the actions menu), the live terminal the workspace hosts here (children),
  // Approvals under it, and the context panel beside it (behind Details under 1100 px and in theatre view). A terminal
  // session shows no steps and no progress bar (R10): only its state word and where that word came from.
  let { session, sessions, machines, health, projects, now, theatre = false, keyboard = false, busy = false, chatBusy,
    onaction, ontheatre, onrefresh, onmode, onsignedout, oncontinue, onterminal, onside, side, showSide = false, children }: {
    session: TerminalSession; sessions: TerminalSession[]
    machines: TerminalMachine[]; health: MachineHealth[]; projects: FolderProject[] | undefined; now: number
    theatre?: boolean; keyboard?: boolean; busy?: boolean
    onaction: (kind: 'rename' | 'end' | 'archive') => void; ontheatre: () => void; onrefresh: () => void
    /** The open chat's own word on whether it is busy (a running turn, a waiting approval or question), when it has one. */
    chatBusy?: boolean
    /** The machine switched this session to the other TUI: the workspace shows it at once and reads the listing again. */
    onmode?: (id: string, to: 'terminal' | 'chat') => void
    onsignedout?: (e: unknown) => void
    /** Present only when this closed session can be continued here. */
    oncontinue?: () => void; onterminal?: () => void; onside?: () => void; side?: Snippet; showSide?: boolean; children: Snippet
  } = $props()

  const id = $props.id()
  const chatView = $derived(!!session.structured)
  const terminalLabel = $derived(session.tool ? `${toolName(session.tool)} TUI` : 'Terminal')

  // The skills line comes from the sync state, read once when the screen opens (undefined while reading, null if it failed).
  let sync = $state<SyncState | null | undefined>(undefined)
  onMount(() => {
    const life = new AbortController()
    void getSyncState(life.signal).then(state => { sync = state }).catch(() => { if (!life.signal.aborted) sync = null })
    return () => life.abort()
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
  // Read once the screen shows a chat (when it opens as one, or when the session switches to the reet TUI).
  let yoloRead = false
  $effect(() => {
    if (!session.structured || yoloRead) return
    yoloRead = true
    void getYoloSetting().then(value => { yoloSetting = value }).catch(() => { yoloSetting = null })
  })

  // The reet TUI | agent TUI switch (switchModeState says where the inactive button goes and why it may not). A switch
  // stops one process on the machine and starts the other, which can take minutes: until the machine answers, both buttons
  // wait and the status line says so. No client deadline is set; the request ends when the controller answers.
  let switching = $state<{ id: string; to: 'terminal' | 'chat' } | null>(null)
  let switchAsk = $state<{ to: 'terminal' | 'chat'; trust?: { detail: string } } | null>(null), switchOpener: HTMLElement | null = null
  const modeSwitching = $derived(switching?.id === session.id)
  const chatIsBusy = $derived(chatBusy ?? (session.attention === 'working' || session.attention === 'waiting'))
  const switchState = $derived(switchModeState(session, machineCaps, chatIsBusy))
  const tuiName = (to: 'terminal' | 'chat') => to === 'chat' ? 'reet TUI' : terminalLabel
  const canSwitch = $derived(switchState.enabled && !modeSwitching && !busy)
  const switchWhy = $derived(modeSwitching && switching ? `Switching to the ${tuiName(switching.to)}…` : switchState.why)
  function openSwitch(opener: HTMLElement | null) {
    if (!canSwitch) return
    switchOpener = opener
    switchAsk = { to: switchState.to }
  }
  function closeSwitch() { switchAsk = null; switchOpener?.focus() }
  async function confirmSwitch(trust: boolean) {
    const ask = switchAsk
    switchAsk = null
    if (!ask || switching) return
    const target = { id: session.id, to: ask.to }, name = tuiName(ask.to)
    switching = target
    notice = { text: `Switching to the ${name}… This can take a few minutes.` }
    switchOpener?.focus()
    try {
      await setSessionMode(target.id, target.to, trust)
      if (session.id === target.id) notice = { text: `Switched to the ${name}.` }
      if (onmode) onmode(target.id, target.to); else onrefresh()
    } catch (e) {
      if (session.id !== target.id) return
      if (e instanceof TerminalError && e.code === 'acp_trust_required' && target.to === 'chat' && !trust) {
        notice = null
        switchAsk = { to: target.to, trust: { detail: e.detail || e.message } }
        return
      }
      notice = { text: e instanceof Error && e.message ? e.message : 'The session could not be switched. Try again in a moment.', bad: true }
      onrefresh()
    } finally { switching = null }
  }

  // Details: the context panel over the terminal under 1100 px (and in theatre view). Escape closes it and focus returns.
  let detailsOpen = $state(false), detailsButton = $state<HTMLButtonElement>()
  // The panel lies over the terminal (a sheet) while Details is shown, i.e. with under 1100 px of room or in theatre view.
  const sheet = () => !!detailsButton && detailsButton.getClientRects().length > 0
  async function toggleDetails() {
    detailsOpen = !detailsOpen
    if (!detailsOpen) return
    // Focus moves into the sheet (its Close details first), so it never stays on a control the sheet could cover.
    await tick()
    document.getElementById(`${id}-context`)?.querySelector<HTMLElement>('.close-details')?.focus()
  }
  function hideDetails() { detailsOpen = false; detailsButton?.focus() }
  function closeDetails(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !detailsOpen) return
    event.preventDefault(); event.stopPropagation()
    hideDetails()
  }
  // While it is a sheet, Tab and Shift+Tab go round its own controls; Escape and Close details give focus back to Details.
  function panelKey(event: KeyboardEvent) {
    if (event.key === 'Escape') return closeDetails(event)
    if (event.key !== 'Tab' || !detailsOpen || !sheet()) return
    const panel = event.currentTarget as HTMLElement
    const items = [...panel.querySelectorAll<HTMLElement>('a[href],button:not(:disabled),[tabindex="0"]')].filter(el => el.getClientRects().length)
    if (!items.length) return
    const at = items.indexOf(document.activeElement as HTMLElement)
    event.preventDefault()
    items[at < 0 ? 0 : (at + (event.shiftKey ? -1 : 1) + items.length) % items.length].focus()
  }

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
<div class="session-screen" class:theatre class:keyboard class:details-open={detailsOpen}>
  <div class="body">
    <header class="head">
      <a class="back" href="/sessions" aria-label="Back to sessions"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg></a>
      <div class="heading">
        <h2 class="title" title={session.title}>{session.title}</h2>
        <p class="sub">{headerMeta(session, session.createdAt, now)}</p>
        <p class="where-phone">{[project, host].filter(Boolean).join(' · ')}</p>
      </div>
      <span class="pill {word.tone}" title={source}><span class="dot" aria-hidden="true"></span>{word.word}{#if away}{' '}<span class="lk">· last known</span>{/if}<span class="vh"> · {word.source}</span></span>
      {#if canSwitchApproval}
        <div class="approvals" role="group" aria-label="Approvals">
          <button type="button" aria-pressed={approvalMode === 'ask'} disabled={busy || modeBusy} onclick={askMode}>Ask me</button>
          <button type="button" aria-pressed={approvalMode === 'yolo'} disabled={busy || modeBusy}
            title={yoloCanStart(switchGates) ? 'Answer every approval for this session' : 'Read why YOLO is not ready here'} onclick={yoloMode}>YOLO</button>
        </div>
      {:else if session.yolo}<span class="badge yolo">YOLO</span>{/if}
      <!-- The pressed button is the TUI shown; the other switches to it (aria-disabled with its reason when it cannot now). -->
      <div class="view" role="group" aria-label="View" aria-busy={modeSwitching}>
        <button type="button" class="chat" aria-pressed={chatView} aria-disabled={chatView ? modeSwitching || undefined : !canSwitch}
          aria-describedby={!chatView && !canSwitch ? `${id}-switch-why` : undefined} title={!chatView && !canSwitch ? switchWhy : undefined}
          onclick={event => { if (!chatView) openSwitch(event.currentTarget) }}>reet TUI</button>
        <button type="button" aria-pressed={!chatView} aria-disabled={chatView ? !canSwitch : modeSwitching || undefined}
          aria-describedby={chatView && !canSwitch ? `${id}-switch-why` : undefined} title={chatView && !canSwitch ? switchWhy : undefined}
          onclick={event => { if (chatView) openSwitch(event.currentTarget) }}>{terminalLabel}</button>
      </div>
      <span class="vh" id={`${id}-switch-why`}>{switchWhy}</span>
      <button type="button" class="details" bind:this={detailsButton} aria-expanded={detailsOpen} aria-controls={`${id}-context`} onclick={() => void toggleDetails()} onkeydown={closeDetails}>Details</button>
      <button type="button" class="more" bind:this={menuButton} aria-label="Session actions" aria-haspopup="menu" aria-expanded={menuAt !== null}
        onclick={event => { if (menuAt) closeMenu(); else void openMenu(event) }} onkeydown={event => { if (menuKeyOpens(event)) void openMenu(event) }}>
        <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="19" cy="12" r="1.6" /></svg>
      </button>
    </header>
    <!-- Always rendered (empty takes no room), so the first message in it is announced. -->
    <div class="live" role="status">{#if notice}<p class:bad={notice.bad}>{notice.text}</p>{/if}</div>
    {#if showSide && side}
      <div class="side-terminal" data-keeps-focus>{@render side()}</div>
    {:else}
      {#key session.id}
        <ContextPanel {session} {host} {status} {project} {skills} id={`${id}-context`} onkeydown={panelKey} onclose={hideDetails} />
      {/key}
    {/if}
    <div class="stage" bind:this={stage}>{@render children()}</div>
  </div>
</div>

{#if menuAt}
  <!-- data-keeps-focus: a reconnecting terminal never takes focus out of the open menu. -->
  <div class="menu" role="menu" aria-label={`Actions for ${session.title}`} tabindex="-1" data-keeps-focus bind:this={menu} style={`left:${menuAt.x}px;top:${menuAt.y}px`} onkeydown={menuKey}>
    {#if oncontinue}<button role="menuitem" disabled={busy} onclick={continueSession}>{session.structured ? 'Continue in reet TUI with history…' : 'Continue session…'}</button>{/if}
    {#if onterminal}<button role="menuitem" disabled={busy} onclick={openTerminal}>Open a terminal here…</button>{/if}
    {#if onside}<button role="menuitem" disabled={busy} onclick={openSideTerminal}>Open a terminal beside this session…</button>{/if}
    {#if live && session.tool === 'opencode'}
      <button role="menuitem" disabled={!canSwitch} title={canSwitch ? undefined : switchWhy} onclick={() => { closeMenu(false); openSwitch(menuButton ?? null) }}>Switch to the {tuiName(switchState.to)}…</button>
    {/if}
    {#if canSwitchApproval}
      <button role="menuitem" disabled={busy || modeBusy} onclick={() => { closeMenu(false); if (approvalMode === 'yolo') askMode(); else yoloMode() }}>{approvalMode === 'yolo' ? 'Switch to Ask me' : 'Turn on YOLO…'}</button>
    {/if}
    <button role="menuitem" disabled={busy || serverOff} onclick={() => act('rename')}>Rename…</button>
    <button role="menuitem" onclick={theatreView}>{theatre ? 'Exit theatre view' : 'Theatre view'}</button>
    {#if session.worktreeBranch}<button role="menuitem" onclick={() => void copyBranch()}>Copy branch</button>{/if}
    {#if live}<button role="menuitem" class="danger" disabled={busy || serverOff} onclick={() => act('end')}>{session.state === 'end_requested' ? 'Retry end request…' : 'End session…'}</button>
    {:else}<button role="menuitem" disabled={busy} onclick={() => act('archive')}>Remove from history…</button>{/if}
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
  .body > :global(.context-panel) { grid-column: 2; grid-row: 1 / -1; }
  .side-terminal { grid-column: 2; grid-row: 1 / -1; width: 340px; min-width: 0; min-height: 0; display: flex; flex-direction: column; border-left: 1px solid var(--line-soft); background: var(--bg-sunk); }
  .side-terminal :global(.pane) { flex: 1; min-height: 0; }
  .side-terminal :global(.pane > header), .side-terminal :global(.pane > footer) { display: none; }
  /* The mock-up's header: 62 px, 28 px in from the sides, the title in Fraunces 21 px over a 13 px line. */
  .head { flex: none; display: flex; align-items: center; gap: 14px; min-height: 62px; padding: 0 28px; box-sizing: border-box; }
  .heading { flex-grow: 1; min-width: 0; }
  /* Back to the Sessions screen: the phone's slim header has it (the workspace toolbar's arrow gives way there). */
  .back { display: none; flex: none; width: 44px; height: 44px; align-items: center; justify-content: center; color: var(--ink-soft); border-radius: 10px; }
  .back svg { width: 22px; height: 22px; }
  .back:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .title { margin: 0; font: 600 21px/1.3 var(--serif); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .sub, .where-phone { margin: 0; font-size: 13px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .where-phone { display: none; }
  .pill { --tone: var(--muted); flex: none; display: inline-flex; align-items: center; gap: 8px; height: 30px; padding: 0 12px; border-radius: 99px; box-sizing: border-box;
    background: color-mix(in srgb, var(--tone) 14%, transparent); border: 1px solid color-mix(in srgb, var(--tone) 45%, transparent); color: var(--tone); font-size: 13px; font-weight: 500; white-space: nowrap; }
  .pill.good { --tone: var(--good); }
  .pill.warn { --tone: var(--warn); }
  .pill.bad { --tone: var(--bad); }
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
  .details { display: none; flex: none; min-height: 44px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 10px; background: none; color: var(--ink); font: 500 14px var(--sans); cursor: pointer; }
  .details[aria-expanded=true] { background: var(--line-soft); }
  .more { flex: none; display: grid; place-items: center; width: 40px; height: 40px; padding: 0; border: 0; border-radius: 9px; background: none; color: var(--muted); cursor: pointer; }
  .more:hover { background: var(--tint); color: var(--ink); }
  .more svg { width: 20px; height: 20px; }
  .live p { margin: 0 28px 8px; font-size: 13px; color: var(--muted); }
  .live p.bad { color: var(--bad); }
  .stage { min-height: 0; display: flex; flex-direction: column; overflow: hidden; border-top: 1px solid var(--line-soft); }
  .menu { position: fixed; z-index: 110; width: 240px; max-height: calc(100dvh - 16px); overflow: auto; display: grid; gap: 2px; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--card-raised); box-shadow: var(--shadow); }
  .menu [role=menuitem] { display: block; min-height: 36px; padding: 8px 9px; border: 0; border-radius: 6px; background: none; color: var(--ink); font: 400 14px var(--sans); text-align: left; cursor: pointer; }
  .menu [role=menuitem]:hover:not(:disabled), .menu [role=menuitem]:focus-visible { background: var(--tint); outline: none; }
  .menu [role=menuitem]:disabled { opacity: .45; cursor: default; }
  .menu .danger { color: var(--bad); }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @media (pointer: coarse) { .menu [role=menuitem] { min-height: 44px; } .more { width: 44px; height: 44px; } .view button, .approvals button { min-height: 44px; } }
  /* Under 1100 px of room (a narrower window, or the sessions panel docked beside it), and in theatre view, the context panel
     waits behind Details and opens over the terminal's edge, so the terminal keeps most of the width. */
  @container session-screen (max-width: 1099px) {
    .details { display: inline-flex; align-items: center; }
    .body > :global(.context-panel) { display: none; }
    /* The sheet shares the terminal's cell, below the header, so it never covers Details or ⋮. */
    .details-open .body > :global(.context-panel) { display: flex; grid-column: 1; grid-row: 3; justify-self: end; position: relative; z-index: 20; width: min(340px, 100%); box-shadow: var(--shadow); }
    .details-open .body > :global(.context-panel .close-details) { display: inline-flex; align-items: center; }
    /* The controls move under the title when the column is too narrow for both on one line. */
    .head { flex-wrap: wrap; row-gap: 6px; padding: 8px 16px; gap: 10px; }
    .heading { flex: 1 1 200px; }
  }
  .theatre .details { display: inline-flex; align-items: center; }
  .theatre .body > :global(.context-panel) { display: none; }
  .theatre.details-open .body > :global(.context-panel) { display: flex; grid-column: 1; grid-row: 3; justify-self: end; position: relative; z-index: 20; width: min(340px, 100%); box-shadow: var(--shadow); }
  .theatre.details-open .body > :global(.context-panel .close-details) { display: inline-flex; align-items: center; }
  /* Phones: one slim header (title, project · machine, the state), as the mock-up's phone session draws it; the tabs,
     the reet TUI | agent TUI switch and the Approvals line give their rows to the terminal. H3 lays out the rest. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .view, .sub { display: none; }
    .where-phone { display: block; font-size: 12px; }
    .back { display: flex; }
    .head { flex-wrap: nowrap; min-height: 52px; padding: 0 4px 0 4px; gap: 6px; }
    .heading { flex: 1 1 0; }
    .title { font: 600 15px/1.3 var(--sans); }
    .pill { height: 28px; padding: 0 10px; gap: 6px; font-size: 12px; }
    .pill .lk { display: none; }
    .badge { font-size: 11px; line-height: 18px; padding: 0 6px; }
    .approvals { display: none; }
    .details { padding: 0 10px; }
  }
  /* A phone held sideways keeps its few rows for the terminal: the pane's own header names the session. H3 lays it out. */
  @media (max-height: 500px) and (pointer: coarse) and (orientation: landscape) { .head { display: none; } }
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
