<script lang="ts">
  import { tick } from 'svelte'
  import TerminalPane, { type TerminalActions } from '../terminal/TerminalPane.svelte'
  import type { ConnectionState } from '../terminal/connection'
  import { SESSIONS_PER_MACHINE, sessionHref, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus, statusLabel } from '../sessionStatus'
  import { middleEllipsis } from '../sessions/middleEllipsis'
  import DeskMenu from './DeskMenu.svelte'
  import { dirLabel, paneAction, paneCopy, paneState, type PaneFailure, type PaneState } from './paneState'
  import { leaves, MAX_PANES, type Pane, type Tab } from './tree'

  // One pane of a desk tab: a one-line header washed in the pane's colour (name, machine, folder, state and a menu) over
  // either the terminal or a card that says what happened and offers one action. A pane never shows a blank terminal.
  // The terminal draws no chrome of its own: the desk draws one toolbar for the focused pane from its `actions`.
  let { tab, pane, session, machine, ready, focused = true, failure, visible = true, hold = false, admitted = true, layout = true, zoomed = false, tabs = [],
    onfocus, onrename, oncolour, onclose, onactions, onsettled, onsplit, onzoom, onswap, onmovetab }: {
    tab: Tab
    pane: Pane
    session: TerminalSession | undefined
    /** The pane's machine; undefined once the machine is no longer enrolled. */
    machine: TerminalMachine | undefined
    /** The machines and sessions have been read at least once, so a missing one means gone, not unknown. */
    ready: boolean
    focused?: boolean
    failure?: PaneFailure
    /** False while another pane is zoomed (or a phone shows another pane): the terminal stays attached, unsized. */
    visible?: boolean
    /** A separator is being dragged: the terminal refits locally and tells its machine the size once, on release. */
    hold?: boolean
    /** The desk lets this pane attach now (attaches start a few at a time). */
    admitted?: boolean
    /** Splits, zoom, swap and moving are offered (not on a phone). */
    layout?: boolean
    zoomed?: boolean
    /** The desk's tabs, for Move to tab. */
    tabs?: Tab[]
    onfocus?: () => void
    onrename: (name: string) => void
    oncolour: (colour: ProjectColour) => void
    onclose: () => void
    /** The terminal's toolbar actions while it is mounted, undefined otherwise. */
    onactions?: (actions: TerminalActions | undefined) => void
    /** The attach this pane was admitted for has an answer (or will not start), so the next pane may start. */
    onsettled?: () => void
    onsplit?: (dir: 'right' | 'down') => void
    onzoom?: () => void
    onswap?: (other: string) => void
    onmovetab?: (tab: string) => void
  } = $props()

  let terminal = $state<TerminalPane>()
  let connection = $state<ConnectionState>('connecting')
  // A new session starts a new attachment.
  $effect(() => { void session?.id; connection = 'connecting' })
  const attach = $derived({ conflict: connection === 'taken_over', down: connection === 'reconnecting' })
  const deskShell = $derived((machine?.capabilities as { deskShell?: boolean } | undefined)?.deskShell)
  const shown = $derived<PaneState>(paneState(pane, session, machine ? { online: machine.ready, deskShell } : undefined, attach, failure))
  const mounted = $derived(!!session && (shown === 'live' || shown === 'reconnecting' || shown === 'elsewhere'))
  const attaching = $derived(mounted && admitted)
  let actions = $state<TerminalActions>()
  $effect(() => { onactions?.(attaching ? actions : undefined) })
  $effect(() => () => onactions?.(undefined))
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
  const copy = $derived(paneCopy(shown, pane, session, host, SESSIONS_PER_MACHINE))
  const first = $derived(paneAction(shown, pane, session)[0])
  const status = $derived(session && mounted ? sessionStatus(session) : undefined)
  const WORDS: Record<PaneState, string> = { live: 'Running', reconnecting: 'Reconnecting', elsewhere: 'Elsewhere', ended: 'Ended', lost: 'Lost', folderGone: 'Folder gone', limit: 'At the limit', machineGone: 'Machine removed', empty: 'Not started', update: 'Update reet' }
  const word = $derived(shown === 'live' && status && !(status === 'working' && !session?.tool) ? statusLabel(status) : WORDS[shown])
  const tone = $derived(shown === 'lost' || shown === 'machineGone' ? 'bad' : shown === 'folderGone' || shown === 'limit' || shown === 'update' ? 'warn' : shown === 'live' ? 'good' : '')

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
  const paneName = (p: Pane) => p.name || 'Untitled'
  function act(kind: string) {
    if (kind === 'takeover') terminal?.takeOver()
    else if (kind === 'retry') terminal?.retryNow()
  }
  const working = (kind: string) => kind === 'takeover' || kind === 'retry'
</script>

<section class="desk-pane" class:focused data-pane={pane.id} data-state={shown} bind:this={section} aria-current={focused ? 'true' : undefined} style={`--pc:${colour ? projectColourHex(colour) : 'transparent'}`} aria-label={`${name} on ${host || 'a removed machine'}`} onpointerdown={() => onfocus?.()} onfocusin={() => onfocus?.()}>
  <header class="ph" class:tinted={!!colour} draggable={layout && !editing && swappable.length > 0 ? 'true' : undefined} data-drag-pane={pane.id}>
    {#if editing}
      <input class="rename" aria-label="Pane name" maxlength="80" bind:this={input} bind:value={draft} onkeydown={event => { if (event.key === 'Enter') { event.preventDefault(); finishRename(true) } else if (event.key === 'Escape') { event.preventDefault(); finishRename(false) } }} onblur={() => finishRename(true)} />
    {:else}
      <span class="nm" title={name}>{name}</span>
    {/if}
    {#if host}<span class="host">{host}</span>{/if}
    {#if folder}<span class="dir" title={folder}>{middleEllipsis(dirLabel(folder), 48)}</span>{/if}
    <span class="st"><span class={`word ${tone}`} data-status={status}>{word}</span>
      <button type="button" class="more" aria-label={`Actions for ${name}`} aria-haspopup="menu" aria-expanded={!!menu} onclick={openMenu}>⋮</button>
    </span>
  </header>
  <div class="body">
    {#if !ready}
      <p class="waiting" role="status">Looking for this pane’s shell…</p>
    {:else if mounted && session && !admitted}
      <p class="waiting" role="status">Connecting…</p>
    {:else if mounted && session}
      <TerminalPane bind:this={terminal} bind:actions chrome={false} {session} {host} user={machine?.capabilities?.osUser ?? ''} {visible} holdSize={hold}
        historySupported={machine?.capabilities?.commandHistory ?? false} uploadsSupported={machine?.capabilities?.uploads ?? false} uploadsOff={machine?.capabilities?.uploadsOff ?? false}
        onconnection={next => (connection = next)} />
      {#if shown !== 'live'}
        <div class="sbar" role="status"><span><b>{copy.title}</b> {copy.body}</span>{#if first}<button type="button" class="btn primary" onclick={() => act(first.kind)}>{first.label}</button>{/if}</div>
      {/if}
    {:else}
      <div class="state" role="status">
        <span class="tag">{WORDS[shown]}</span>
        <h3>{copy.title}</h3>
        {#if copy.body}<p>{copy.body}</p>{/if}
        {#if first}<div class="row"><button type="button" class="btn primary" disabled={!working(first.kind)} title={working(first.kind) ? undefined : 'Coming in a later step'} onclick={() => act(first.kind)}>{first.label}</button></div>{/if}
      </div>
    {/if}
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
      {#each movable as t (t.id)}<button type="button" role="menuitem" onclick={() => { closeMenu(false); onmovetab?.(t.id) }}>{t.name || 'Untitled'}</button>{/each}
    </DeskMenu>
  {/key}
{:else if menu}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${name}`} opener={menu.opener} onclose={closeMenu} colour={pane.colour} oncolour={c => { closeMenu(true); oncolour(c) }}>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); void startRename() }}>Rename</button>
    {#if layout}
      <button type="button" role="menuitem" onclick={() => { closeMenu(false); onsplit?.('right') }}>Split right</button>
      <button type="button" role="menuitem" onclick={() => { closeMenu(false); onsplit?.('down') }}>Split down</button>
      <button type="button" role="menuitem" disabled={swappable.length === 0} onclick={() => { closeMenu(true); onzoom?.() }}>{zoomed ? 'Show all panes' : 'Zoom'}</button>
      <button type="button" role="menuitem" aria-haspopup="menu" disabled={swappable.length === 0} onclick={() => openList('swap')}>Swap with…</button>
      <button type="button" role="menuitem" aria-haspopup="menu" disabled={movable.length === 0} onclick={() => openList('move')}>Move to tab…</button>
    {/if}
    {#if session}<a role="menuitem" href={sessionHref(session)}>Open in Sessions</a>{/if}
    <button type="button" role="menuitem" class="danger" onclick={() => { closeMenu(false); onclose() }}>Close</button>
  </DeskMenu>
{/if}

<style>
  .desk-pane { flex: 1; min-width: 0; min-height: 0; display: flex; flex-direction: column; background: var(--bg-sunk); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
  .desk-pane.focused { border-color: var(--accent-text); box-shadow: 0 0 0 1px var(--accent-text); }
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
  .more { width: 32px; height: 32px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); font-size: 16px; cursor: pointer; }
  .more:hover { color: var(--ink); background: color-mix(in srgb, var(--ink) 8%, transparent); }
  .rename { min-height: 28px; padding: 2px 8px; font-weight: 600; width: min(260px, 50%); }
  .body { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .body > :global(.pane) { flex: 1; border: 0; border-radius: 0; }
  /* Below the last whole row the screen shows its own background, not xterm's default black viewport. */
  .body :global(.xterm .xterm-viewport) { background-color: transparent; }
  .waiting { margin: auto; color: var(--muted); }
  .state { flex: 1; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 8px; padding: 16px 20px; overflow: auto; }
  .state h3 { margin: 0; font-size: 1rem; font-weight: 600; overflow-wrap: anywhere; }
  .state p { margin: 0; color: var(--muted); max-width: 52ch; }
  .tag { font-size: 14px; letter-spacing: 0.04em; text-transform: uppercase; color: var(--faint); }
  .row { display: flex; gap: 8px; flex-wrap: wrap; }
  .sbar { position: absolute; left: 0; right: 0; top: 0; z-index: 5; display: flex; align-items: center; gap: 12px; padding: 8px 12px; background: var(--tint); border-bottom: 1px solid var(--line); font-size: 14px; }
  .sbar span { flex: 1; min-width: 0; color: var(--muted); }
  .sbar b { color: var(--ink); font-weight: 600; }
  @media (pointer: coarse) { .more { width: 44px; height: 44px; } }
</style>
