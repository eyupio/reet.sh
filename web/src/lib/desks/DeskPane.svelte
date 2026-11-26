<script lang="ts">
  import { tick, type Snippet } from 'svelte'
  import TerminalPane, { type TerminalActions } from '../terminal/TerminalPane.svelte'
  import type { ConnectionState } from '../terminal/connection'
  import { hasHiddenChars, SESSIONS_PER_ACCOUNT, SESSIONS_PER_MACHINE, sessionHref, stripHiddenChars, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { HELD_SENTENCE, STOPPED_LINE, heldWork, sessionStatus, statusLabel, stoppedWaiting } from '../sessionStatus'
  import { middleEllipsis } from '../sessions/middleEllipsis'
  import DeskMenu from './DeskMenu.svelte'
  import { agentName, dirLabel, isRunning, paneCard, paneNote, paneState, startupInput, type PaneActionKind, type PaneFailure, type PaneState } from './paneState'
  import PaneStateCard from './PaneStateCard.svelte'
  import StartupBar from './StartupBar.svelte'
  import { shell } from '../dialogShell'
  import { leaves, MAX_PANES, MAX_TABS, type Pane, type Tab } from './tree'

  // One pane of a desk tab: a one-line header washed in the pane's colour (name, machine, folder, state and a menu) over
  // either the terminal or a card that says what happened and offers its actions (PaneStateCard). A pane never shows a
  // blank terminal. Reopening is the desk's to do (it owns the desk's version); the pane only asks.
  // The terminal draws no chrome of its own: the desk draws one toolbar for the focused pane from its `actions`.
  let { tab, pane, session, machine, ready, focused = true, failure, busy = false, error = '', canContinue = false, visible = true, hold = false, admitted = true, layout = true, zoomed = false, tabs = [], startup = '', receiving = false, template = false, phone = false,
    onfocus, onrename, oncolour, onclose, onreopen, onactions, onsettled, onsplit, onzoom, onswap, onmovetab, onmovetonewtab, onstartcommand, onstartup, onreceivable, oninput, bar }: {
    tab: Tab
    pane: Pane
    session: TerminalSession | undefined
    /** The pane's machine; undefined once the machine is no longer enrolled. */
    machine: TerminalMachine | undefined
    /** The machines and sessions have been read at least once, so a missing one means gone, not unknown. */
    ready: boolean
    focused?: boolean
    failure?: PaneFailure
    /** A reopen, start or continue of this pane is in flight. */
    busy?: boolean
    /** The last refusal, in the route's words, when it has no state of its own. */
    error?: string
    /** The agent's own resume is on offer (the Sessions screen's `canContinue`). */
    canContinue?: boolean
    /** False while another pane is zoomed (or a phone shows another pane): the terminal stays attached, unsized. */
    visible?: boolean
    /** A separator is being dragged: the terminal refits locally and tells its machine the size once, on release. */
    hold?: boolean
    /** The desk lets this pane attach now (attaches start a few at a time). */
    admitted?: boolean
    /** Splits, zoom, swap and moving are offered (not on a phone). */
    layout?: boolean
    /** The desk is a template: nothing starts a shell from it, so the card offers no Start or Reopen and there is no Split. */
    template?: boolean
    /** A phone shows this one pane edge to edge, without the machine's name in its header. */
    phone?: boolean
    zoomed?: boolean
    /** The desk's tabs, for Move to tab. */
    tabs?: Tab[]
    onfocus?: () => void
    onrename: (name: string) => void
    oncolour: (colour: ProjectColour) => void
    onclose: () => void
    /** A press on a card's action that starts a shell or changes where it starts: reopen, start, try again, continue,
     *  choose a folder or a machine. */
    onreopen?: (kind: PaneActionKind) => void
    /** The terminal's toolbar actions while it is mounted, undefined otherwise. */
    onactions?: (actions: TerminalActions | undefined) => void
    /** The attach this pane was admitted for has an answer (or will not start), so the next pane may start. */
    onsettled?: () => void
    onsplit?: (dir: 'right' | 'down') => void
    onzoom?: () => void
    onswap?: (other: string) => void
    onmovetab?: (tab: string) => void
    onmovetonewtab?: () => void
    /** The start-up command to offer above the terminal: set only after this pane was reopened or started here. */
    startup?: string
    /** Saves the pane's start-up command ('' removes it). */
    onstartcommand?: (start: string) => void
    /** The offer is done with: `ran` once Run typed it, false for Dismiss. */
    onstartup?: (ran: boolean) => void
    /** Type in all panes is on and this pane is one of those receiving. */
    receiving?: boolean
    /** Whether this pane could receive now: its terminal is mounted, on screen and connected. */
    onreceivable?: (ok: boolean) => void
    /** Typed keys, a paste or Compose's text, once this pane's terminal has sent them (TerminalPane's `oninput`). */
    oninput?: (data: string, paste: boolean, at: number) => void
    /** The desk's command bar, drawn inside the pane's terminal area when this is the focused pane. */
    bar?: Snippet
  } = $props()

  let terminal = $state<TerminalPane>()
  let connection = $state<ConnectionState>('connecting')
  // A new session starts a new attachment. Only its id counts: a fresh read of the same session is not a new attachment.
  const sessionId = $derived(session?.id)
  $effect(() => { void sessionId; connection = 'connecting' })
  const attach = $derived({ conflict: connection === 'taken_over', down: connection === 'reconnecting' })
  const deskShell = $derived((machine?.capabilities as { deskShell?: boolean } | undefined)?.deskShell)
  const shown = $derived<PaneState>(paneState(pane, session, machine ? { online: machine.ready, deskShell } : undefined, attach, failure))
  const mounted = $derived(!!pane.session && isRunning(session) && (shown === 'live' || shown === 'reconnecting' || shown === 'elsewhere'))
  const attaching = $derived(mounted && admitted)
  let actions = $state<TerminalActions>()
  $effect(() => { onactions?.(attaching ? actions : undefined) })
  $effect(() => () => onactions?.(undefined))
  const receivable = $derived(attaching && shown === 'live' && connection === 'live' && visible)
  $effect(() => { onreceivable?.(receivable) })
  $effect(() => () => onreceivable?.(false))
  // The attach slot is given back once the attachment has an answer, or at once when there is nothing to attach.
  let settled = false
  $effect(() => { if (admitted && !settled && (!mounted || connection !== 'connecting')) { settled = true; onsettled?.() } })
  // The focused pane takes the keyboard once it is live, unless the person is busy somewhere else on the page.
  $effect(() => {
    if (!focused || connection !== 'live' || !visible) return
    const at = document.activeElement
    if (!at || at === document.body || section?.contains(at)) actions?.focus()
  })
  let section = $state<HTMLElement>()
  // The folder the shell is in now, as its machine last reported it; the folder the pane was opened in until then.
  const folder = $derived(session?.liveDir || pane.dir)
  const host = $derived(machine?.host ?? '')
  const name = $derived(pane.name || session?.title || dirLabel(folder).split('/').pop() || 'Shell')
  const colour = $derived(pane.colour || tab.colour)
  const dir = $derived(dirLabel(pane.dir))
  // A template's panes are never started: its card keeps only what does not start a shell.
  const STARTS = new Set<PaneActionKind>(['start', 'reopen', 'reopenHome', 'again', 'continue', 'chooseFolder', 'chooseMachine', 'retry', 'takeover'])
  const drawn = $derived(paneCard(shown, { host, dir, agent: agentName(session?.tool), perMachine: SESSIONS_PER_MACHINE, perAccount: SESSIONS_PER_ACCOUNT, accountLimit: failure === 'accountLimit', canContinue }))
  const card = $derived(drawn && template ? { ...drawn, actions: drawn.actions.filter(a => !STARTS.has(a.kind)) } : drawn)
  const note = $derived(paneNote(pane, shown, machine ? { deskShell } : undefined))
  const status = $derived(session && mounted ? sessionStatus(session) : undefined)
  const WORDS: Record<PaneState, string> = { live: 'Running', reconnecting: 'Reconnecting', elsewhere: 'Elsewhere', ended: 'Ended', lost: 'Lost', agentEnded: 'Ended', agentLost: 'Lost', folderGone: 'Folder gone', folderUnavailable: 'Folder unavailable', limit: 'At the limit', machineGone: 'Machine removed', projectsUnread: 'Not continued', empty: 'Not started' }
  const held = $derived(!!session && shown === 'live' && heldWork(session))
  const stopped = $derived(!!session && shown === 'live' && stoppedWaiting(session))
  const word = $derived(shown === 'live' && status && !(status === 'working' && !session?.tool) ? statusLabel(status, held, stopped) : WORDS[shown])
  const tone = $derived(shown === 'lost' || shown === 'agentLost' || shown === 'machineGone' ? 'bad' : shown === 'folderGone' || shown === 'folderUnavailable' || shown === 'limit' || shown === 'projectsUnread' ? 'warn' : shown === 'live' ? 'good' : '')

  // Rename in place, from the menu or the terminal's own Manage menu.
  let editing = $state(false), draft = $state(''), input = $state<HTMLInputElement>()
  async function startRename() { draft = name; editing = true; await tick(); input?.select() }
  function finishRename(save: boolean) {
    if (!editing) return
    editing = false
    const next = draft.trim()
    if (save && next && next !== name && [...next].length <= 80) onrename(next)
  }

  // The pane menu, and its two lists: the panes to swap with and the tabs to move to.
  let menu = $state<{ x: number; y: number; opener: HTMLElement; list?: 'swap' | 'move' } | null>(null)
  function openMenu(event: MouseEvent) {
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { x: box.right - 220, y: box.bottom + 4, opener }
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }
  function openList(list: 'swap' | 'move') { if (menu) menu = { ...menu, list } }
  const swappable = $derived(leaves(tab.root).filter(id => id !== pane.id && tab.panes[id]).map(id => tab.panes[id]))
  // A pane can move to another tab that has a pane to split and room for one more.
  const movable = $derived(tabs.filter(t => t.id !== tab.id && leaves(t.root).length > 0 && leaves(t.root).length < MAX_PANES))
  // A pane can have a new tab of its own when its tab holds another pane and there is room for one more tab.
  const canOwnTab = $derived(leaves(tab.root).length > 1 && tabs.length < MAX_TABS)
  const paneName = (p: Pane) => p.name || 'Untitled'
  // Run types the command and Enter once, through the terminal's own input, and only into a connected terminal.
  // A command with a hidden character (a line break, a control or format character) is never typed: what is run must be
  // what the bar shows.
  function runStartup() { const keys = startupInput(startup); if (keys && terminal?.type(keys)) onstartup?.(true) }

  // The start-up command dialog: one field, Save and Remove.
  const MAX_START = 2000
  let startOpen = $state(false), startDraft = $state(''), startError = $state(''), startDialog = $state<HTMLDialogElement>(), startInput = $state<HTMLInputElement>()
  const startId = $props.id()
  async function openStart() { startDraft = pane.start; startError = ''; startOpen = true; await tick(); startDialog?.showModal(); startInput?.select() }
  function closeStart() { startDialog?.close(); startOpen = false; section?.querySelector<HTMLElement>('.more')?.focus() }
  function saveStart(event: SubmitEvent) {
    event.preventDefault()
    const next = startDraft.trim()
    if ([...next].length > MAX_START) { startError = `Keep it to ${MAX_START} characters.`; return }
    if (hasHiddenChars(next)) { startError = "This command has hidden characters, so reet won't run it. Edit it first."; return }
    if (next !== pane.start) onstartcommand?.(next)
    closeStart()
  }
  function removeStart() { if (pane.start) onstartcommand?.(''); closeStart() }

  function act(kind: PaneActionKind) {
    if (kind === 'takeover') terminal?.takeOver()
    else if (kind === 'retry' && mounted) terminal?.retryNow()
    // A machine that did not answer a reopen: Retry now asks again.
    else if (kind === 'retry') onreopen?.('again')
    else if (kind === 'close') onclose()
    else onreopen?.(kind)
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions (a press anywhere in a pane focuses it, after any button in it has acted; the terminal and its controls own the keyboard) -->
<section class="desk-pane" class:focused class:phone data-pane={pane.id} data-state={shown} bind:this={section} aria-current={focused ? 'true' : undefined} style={`--pc:${colour ? projectColourHex(colour) : 'transparent'}`} aria-label={`${name} on ${host || 'a removed machine'}`} onclick={() => onfocus?.()}>
  <header class="ph" class:tinted={!!colour} draggable={layout && !editing && swappable.length > 0 ? 'true' : undefined} data-drag-pane={pane.id}>
    {#if editing}
      <input class="rename" aria-label="Pane name" maxlength="80" bind:this={input} bind:value={draft} oninput={() => { const clean = stripHiddenChars(draft); if (clean !== draft) draft = clean }} onkeydown={event => { if (event.key === 'Enter') { event.preventDefault(); finishRename(true) } else if (event.key === 'Escape') { event.preventDefault(); finishRename(false) } }} onblur={() => finishRename(true)} />
    {:else}
      <span class="nm" title={name}>{name}</span>
    {/if}
    {#if host && !phone}<span class="host">{host}</span>{/if}
    {#if folder}<span class="dir" title={folder}>{middleEllipsis(dirLabel(folder), 48)}</span>{/if}
    <span class="st">{#if receiving}<span class="rcv">Receiving</span>{/if}<span class={`word ${tone}`} data-status={status} data-held={held ? '' : undefined}>{#if held}<span class="status-ring" aria-hidden="true"></span>{/if}{word}{#if held}<span class="vh">. {HELD_SENTENCE}</span>{:else if stopped}<span class="vh">. {STOPPED_LINE}</span>{/if}</span>
      <button type="button" class="more" aria-label={`Actions for ${name}`} aria-haspopup="menu" aria-expanded={!!menu} onclick={openMenu}>⋮</button>
    </span>
  </header>
  <div class="body">
    {#if !ready}
      <p class="waiting" role="status">Looking for this pane’s shell…</p>
    {:else if mounted && session && !admitted}
      <p class="waiting" role="status">Connecting…</p>
    {:else if mounted && session}
      <!-- A pane given another session gets a terminal of its own: nothing of the last session's connection carries over. -->
      {#key session.id}
        <TerminalPane bind:this={terminal} bind:actions chrome={false} {session} {host} user={machine?.capabilities?.osUser ?? ''} {visible} holdSize={hold}
          historySupported={machine?.capabilities?.commandHistory ?? false} uploadsSupported={machine?.capabilities?.uploads ?? false} uploadsOff={machine?.capabilities?.uploadsOff ?? false}
          onconnection={next => (connection = next)} {oninput} />
      {/key}
      {#if startup}<StartupBar command={startup} ready={connection === 'live'} onrun={runStartup} ondismiss={() => onstartup?.(false)} />{/if}
      {#if note}<p class="note">{note}</p>{/if}
      {#if card}<PaneStateCard {card} {dir} overlay onaction={act} />{/if}
    {:else if card}
      <PaneStateCard {card} {dir} {busy} {error} onaction={act} />
    {/if}
    {#if bar}{@render bar()}{/if}
  </div>
</section>

{#if menu?.list === 'swap'}
  {#key 'swap'}
    <DeskMenu x={menu.x} y={menu.y} label={`Swap ${name} with`} opener={menu.opener} onclose={closeMenu}>
      {#each swappable as other (other.id)}<button type="button" role="menuitem" onclick={() => { closeMenu(true); onswap?.(other.id) }}>{paneName(other)}</button>{/each}
    </DeskMenu>
  {/key}
{:else if menu?.list === 'move'}
  {#key 'move'}
    <DeskMenu x={menu.x} y={menu.y} label={`Move ${name} to tab`} opener={menu.opener} onclose={closeMenu}>
      {#if canOwnTab}<button type="button" role="menuitem" onclick={() => { closeMenu(false); onmovetonewtab?.() }}>New tab</button>{/if}
      {#each movable as t (t.id)}<button type="button" role="menuitem" onclick={() => { closeMenu(false); onmovetab?.(t.id) }}>{t.name || 'Untitled'}</button>{/each}
    </DeskMenu>
  {/key}
{:else if menu}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${name}`} opener={menu.opener} onclose={closeMenu} colour={pane.colour} oncolour={c => { closeMenu(true); oncolour(c) }}>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); void startRename() }}>Rename</button>
    {#if layout}
      {#if !template}
        <button type="button" role="menuitem" onclick={() => { closeMenu(false); onsplit?.('right') }}>Split right</button>
        <button type="button" role="menuitem" onclick={() => { closeMenu(false); onsplit?.('down') }}>Split down</button>
      {/if}
      <button type="button" role="menuitem" disabled={swappable.length === 0} onclick={() => { closeMenu(true); onzoom?.() }}>{zoomed ? 'Show all panes' : 'Zoom'}</button>
      <button type="button" role="menuitem" aria-haspopup="menu" disabled={swappable.length === 0} onclick={() => openList('swap')}>Swap with…</button>
      <button type="button" role="menuitem" aria-haspopup="menu" disabled={movable.length === 0 && !canOwnTab} onclick={() => openList('move')}>Move to tab…</button>
    {/if}
    <button type="button" role="menuitem" aria-haspopup="dialog" onclick={() => { closeMenu(false); void openStart() }}>Start-up command…</button>
    {#if session}<a role="menuitem" href={sessionHref(session)}>Open in Sessions</a>{/if}
    <button type="button" role="menuitem" class="danger" onclick={() => { closeMenu(false); onclose() }}>Close</button>
  </DeskMenu>
{/if}

{#if startOpen}
  <dialog bind:this={startDialog} class="startup" use:shell={{ sheet: 'bottom' }} aria-labelledby={startId} oncancel={event => { event.preventDefault(); closeStart() }}>
    <form class="box" data-shell-card onsubmit={saveStart}>
      <h2 id={startId}>Start-up command</h2>
      <label class="field"><span>Command to offer when this pane is reopened</span>
        <input bind:this={startInput} bind:value={startDraft} maxlength={MAX_START} spellcheck="false" autocomplete="off" aria-invalid={startError ? 'true' : undefined} /></label>
      <p class="hint">After a reopen it is shown above the terminal with Run. reet never runs it on its own.</p>
      {#if startError}<p class="bad" role="alert">{startError}</p>{/if}
      <div class="actions" data-shell-foot>
        {#if pane.start}<button type="button" class="btn quiet" onclick={removeStart}>Remove</button>{/if}
        <span class="gap"></span>
        <button type="button" class="btn quiet" onclick={closeStart}>Cancel</button>
        <button type="submit" class="btn primary">Save</button>
      </div>
    </form>
  </dialog>
{/if}

<style>
  .startup { width: min(480px, calc(100vw - 32px)); max-width: none; padding: 0; border: 0; background: transparent; color: var(--ink); }
  .startup::backdrop { background: var(--scrim); }
  .startup .box { display: flex; flex-direction: column; gap: 12px; padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); }
  .startup h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .startup .field { display: flex; flex-direction: column; gap: 6px; font-weight: 600; }
  .startup input { font-family: var(--mono); }
  .startup .hint { margin: 0; color: var(--muted); font-size: 14px; }
  .startup .bad { margin: 0; color: var(--bad); }
  .startup .actions { display: flex; gap: 8px; margin-top: 8px; }
  .startup .gap { flex: 1; }
  .desk-pane { flex: 1; min-width: 0; min-height: 0; display: flex; flex-direction: column; background: var(--bg-sunk); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
  .desk-pane.focused { border-color: var(--accent-text); box-shadow: 0 0 0 1px var(--accent-text); }
  /* A phone shows one pane at a time, edge to edge: no frame, no focus ring. */
  .desk-pane.phone, .desk-pane.phone.focused { border: 0; border-radius: 0; box-shadow: none; }
  /* The header carries the pane's colour as a wash (the mock-up's tinted bar); with no colour it is the plain card. */
  .ph[draggable='true'] { cursor: grab; }
  .ph { flex: none; display: flex; align-items: center; gap: 10px; min-height: 34px; padding: 0 4px 0 12px; font-size: 14px; color: var(--muted); white-space: nowrap; overflow: hidden; background: var(--card); border-bottom: 1px solid var(--line-soft); }
  .ph.tinted { background: color-mix(in srgb, var(--pc) 24%, var(--card)); border-bottom-color: color-mix(in srgb, var(--pc) 55%, var(--line)); }
  .nm { color: var(--ink-soft); font-weight: 600; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .focused .nm { color: var(--ink); }
  .host { flex: none; }
  .dir { font-family: var(--mono); overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .st { margin-left: auto; display: flex; align-items: center; gap: 4px; flex: none; }
  .word.good { color: var(--good); } .word.bad { color: var(--bad); } .word.warn { color: var(--warn); }
  .word[data-status='waiting'] { color: var(--status-waiting); }
  /* A live session's word is said in its status colour, as every status pill is. */
  .word[data-status] { color: var(--status-text); }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .word .status-ring { margin-right: 5px; vertical-align: -1px; }
  .rcv { color: var(--warn); font-weight: 600; margin-right: 6px; }
  .more { width: 32px; height: 32px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); font-size: 16px; cursor: pointer; }
  .more:hover { color: var(--ink); background: color-mix(in srgb, var(--ink) 8%, transparent); }
  .rename { min-height: 28px; padding: 2px 8px; font-weight: 600; width: min(260px, 50%); }
  .body { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .body > :global(.pane) { flex: 1; border: 0; border-radius: 0; }
  /* Below the last whole row the screen shows its own background, not xterm's default black viewport. */
  .body :global(.xterm .xterm-viewport) { background-color: transparent; }
  .waiting { margin: auto; color: var(--muted); }
  /* An older machine: one line under the header, the terminal still in reach. */
  .note { flex: none; order: -1; margin: 0; padding: 4px 12px; font-size: 14px; color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, var(--card)); border-bottom: 1px solid var(--line-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  @media (pointer: coarse) { .more { width: 44px; height: 44px; } }
</style>
