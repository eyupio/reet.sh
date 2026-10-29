<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte'
  import { getSyncState, type SyncState } from '../api'
  import { copyText } from '../clipboard'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import { fleetCards, stateWord, type FleetCard } from '../home'
  import type { RunMember } from '../projects'
  import { headerMeta, offlineSentence, tabFromMenu, worktreeTargetOf } from '../sessions'
  import { terminalClosed, type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import ContextPanel from './ContextPanel.svelte'

  // The session screen: the strip of open sessions, the session's header (title, agent · model · started, the state in words
  // with its source, YOLO, Chat | Terminal and the actions menu), the live terminal the workspace hosts here (children),
  // Approvals under it, and the context panel beside it (behind Details under 1100 px and in theatre view). A terminal
  // session shows no steps and no progress bar (R10): only its state word and where that word came from.
  let { session, sessions, machines, health, projects, now, theatre = false, keyboard = false, busy = false,
    onaction, ontheatre, onrefresh, onsignedout, oncontinue, children }: {
    session: TerminalSession; sessions: TerminalSession[]
    machines: TerminalMachine[]; health: MachineHealth[]; projects: FolderProject[] | undefined; now: number
    theatre?: boolean; keyboard?: boolean; busy?: boolean
    onaction: (kind: 'rename' | 'end' | 'archive') => void; ontheatre: () => void; onrefresh: () => void
    onsignedout?: (e: unknown) => void
    /** Present only when this closed session can be continued here. */
    oncontinue?: () => void; children: Snippet
  } = $props()

  const id = $props.id()
  const chatView = $derived(!!session.structured)
  const CHAT_WHY = 'Chat is for a structured OpenCode session.'
  const TERMINAL_WHY = 'A chat session has no terminal.'

  // The skills line comes from the sync state, read once when the screen opens (undefined while reading, null if it failed).
  let sync = $state<SyncState | null | undefined>(undefined)
  onMount(() => {
    const life = new AbortController()
    void getSyncState(life.signal).then(state => { sync = state }).catch(() => { if (!life.signal.aborted) sync = null })
    return () => life.abort()
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

  let notice = $state<{ text: string; bad?: boolean } | null>(null)
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
      {#if session.yolo}<span class="badge yolo">YOLO</span>{/if}
      <div class="view" role="group" aria-label="View">
        <button type="button" class="chat" aria-pressed={chatView} aria-disabled={!chatView} aria-describedby={chatView ? undefined : `${id}-chat-why`} title={chatView ? undefined : CHAT_WHY}>Chat</button>
        <button type="button" aria-pressed={!chatView} aria-disabled={chatView} aria-describedby={chatView ? `${id}-terminal-why` : undefined} title={chatView ? TERMINAL_WHY : undefined}>Terminal</button>
      </div>
      <span class="vh" id={`${id}-chat-why`}>{CHAT_WHY}</span>
      {#if chatView}<span class="vh" id={`${id}-terminal-why`}>{TERMINAL_WHY}</span>{/if}
      <button type="button" class="details" bind:this={detailsButton} aria-expanded={detailsOpen} aria-controls={`${id}-context`} onclick={() => void toggleDetails()} onkeydown={closeDetails}>Details</button>
      <button type="button" class="more" bind:this={menuButton} aria-label="Session actions" aria-haspopup="menu" aria-expanded={menuAt !== null}
        onclick={event => { if (menuAt) closeMenu(); else void openMenu(event) }} onkeydown={event => { if (menuKeyOpens(event)) void openMenu(event) }}>
        <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="19" cy="12" r="1.6" /></svg>
      </button>
    </header>
    <!-- Always rendered (empty takes no room), so the first message in it is announced. -->
    <div class="live" role="status">{#if notice}<p class:bad={notice.bad}>{notice.text}</p>{/if}</div>
    {#key session.id}
      <ContextPanel {session} {host} {status} {project} {skills} id={`${id}-context`} onkeydown={panelKey} onclose={hideDetails} />
    {/key}
    <div class="stage" bind:this={stage}>{@render children()}</div>
  </div>
</div>

{#if menuAt}
  <!-- data-keeps-focus: a reconnecting terminal never takes focus out of the open menu. -->
  <div class="menu" role="menu" aria-label={`Actions for ${session.title}`} tabindex="-1" data-keeps-focus bind:this={menu} style={`left:${menuAt.x}px;top:${menuAt.y}px`} onkeydown={menuKey}>
    {#if oncontinue}<button role="menuitem" disabled={busy} onclick={continueSession}>{session.structured ? 'Continue chat with history…' : 'Continue session…'}</button>{/if}
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

<style>
  .session-screen { flex: 1; min-height: 0; min-width: 0; display: flex; flex-direction: column; background: var(--bg); container: session-screen / inline-size; }
  .body { flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-rows: auto auto minmax(0, 1fr); }
  .head { grid-column: 1; grid-row: 1; }
  .live { grid-column: 1; grid-row: 2; }
  .stage { grid-column: 1; grid-row: 3; }
  .body > :global(.context-panel) { grid-column: 2; grid-row: 1 / -1; }
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
  .view { flex: none; display: flex; border: 1px solid var(--line); border-radius: 9px; overflow: hidden; }
  .view button { padding: 7px 16px; border: 0; background: none; color: var(--muted); font: 400 14px var(--sans); cursor: pointer; }
  .view button[aria-pressed=true] { background: var(--line-soft); color: var(--ink); font-weight: 500; cursor: default; }
  .view button[aria-disabled=true] { cursor: not-allowed; }
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
  @media (pointer: coarse) { .menu [role=menuitem] { min-height: 44px; } .more { width: 44px; height: 44px; } .view button { min-height: 44px; } }
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
     the Chat | Terminal switch and the Approvals line give their rows to the terminal. H3 lays out the rest. */
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
