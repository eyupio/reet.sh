<script lang="ts">
  import { tick } from 'svelte'
  import { UnauthorizedError } from '../api'
  import { copyText } from '../clipboard'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import WorktreeDialog from '../WorktreeDialog.svelte'
  import { offlineSentence, tabFromMenu, worktreeTargetOf } from '../sessions'
  import { archiveTerminal, endTerminal, renameTerminal, sessionHref, terminalClosed, TerminalError, validTerminalTitle,
    type TerminalMachine, type TerminalSession } from '../terminal/api'

  // The actions menu a session offers wherever its row is drawn (Home's In flight and the Sessions page): right-click,
  // Shift+F10 / the menu key, or the row's ⋮ button. Arrow keys move, Escape closes and gives focus back to what opened it.
  // Rename, End (with remove from history) and Remove from history are confirmed; Check and remove worktree uses the
  // existing worktree sheet. A machine whose session server is turned off can't rename or end one.
  let { machines, offline, terminals = [], removeDefault = false, onsignedout, onchanged, onnotice, onopen, onclose }: {
    /** The machines' names, so a menu says where the session runs. */
    machines: { agentId: string; host: string }[]
    /** The agent ids whose machine is not reporting now. */
    offline: ReadonlySet<string>
    /** The machines' terminal records, so a session server turned off disables the actions it would refuse. */
    terminals?: Pick<TerminalMachine, 'agentId' | 'serverEnabled'>[]
    /** The profile's choice for a session that ends: remove it from history too. */
    removeDefault?: boolean
    onsignedout: (e: unknown) => void
    /** The action went through: read the sessions again. */
    onchanged: () => void | Promise<void>
    onnotice: (notice: { text: string; bad?: boolean } | null) => void
    /** The menu opened for this session: the caller marks the control that opened it. */
    onopen?: (item: TerminalSession) => void
    /** The menu closed. */
    onclose?: () => void
  } = $props()

  const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host || agentId
  const live = (s: TerminalSession) => !terminalClosed(s.state)
  const serverOff = (s: TerminalSession) => terminals.find(m => m.agentId === s.agentId)?.serverEnabled === false

  let menu = $state<HTMLElement>()
  let context = $state<{ item: TerminalSession; x: number; y: number } | null>(null)
  let returnFocus = $state<HTMLElement>()
  /** Open the menu at the pointer (a right-click) or beside the control that asked for it. */
  export async function openMenu(event: MouseEvent | KeyboardEvent, item: TerminalSession) {
    // Shift+right-click keeps the browser's own menu, as on the terminal.
    if (event.type === 'contextmenu' && event.shiftKey) return
    event.preventDefault()
    const opener = event.currentTarget as HTMLElement
    returnFocus = opener
    const box = opener.getBoundingClientRect()
    const x = 'clientX' in event && event.clientX ? event.clientX : box.left
    const y = 'clientY' in event && event.clientY ? event.clientY : box.bottom
    context = { item, x: Math.min(Math.max(8, x), Math.max(8, innerWidth - 248)), y: Math.max(8, Math.min(y, innerHeight - 290)) }
    onopen?.(item)
    await tick()
    menu?.querySelector<HTMLElement>('[role=menuitem]:not(:disabled):not([aria-disabled=true])')?.focus()
  }
  export function closeMenu(focus = true) { context = null; onclose?.(); if (focus) returnFocus?.focus() }
  function menuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); closeMenu(); return }
    if (event.key === 'Tab') { event.preventDefault(); closeMenu(false); tabFromMenu(returnFocus, event.shiftKey); return }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = [...menu!.querySelectorAll<HTMLElement>('[role=menuitem]:not(:disabled):not([aria-disabled=true])')], at = items.indexOf(document.activeElement as HTMLElement)
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
  }
  // Rename, End (with the remove-from-history choice) and Remove from history, each confirmed; then the list is read again.
  let action = $state<{ kind: 'rename' | 'end' | 'archive'; item: TerminalSession } | null>(null)
  let rename = $state(''), removeOnEnd = $state(false), busy = $state(false)
  const endKeys = new Map<string, { key: string; remove: boolean }>()
  function begin(kind: 'rename' | 'end' | 'archive') {
    if (!context) return
    const item = context.item
    closeMenu(false)
    rename = item.title
    removeOnEnd = endKeys.get(item.id)?.remove ?? (item.state === 'end_requested' ? item.removeHistory ?? false : removeDefault)
    action = { kind, item }
  }
  function cancel() { action = null; returnFocus?.focus() }
  async function perform() {
    if (!action || busy) return
    const { kind, item } = action
    if (kind === 'rename' && !validTerminalTitle(rename.trim())) { onnotice({ text: 'Use a session name of 1–80 characters without control characters.', bad: true }); return }
    action = null; busy = true; onnotice(null)
    try {
      if (kind === 'rename') await renameTerminal(item.id, rename.trim())
      else if (kind === 'archive') await archiveTerminal(item.id)
      else {
        let op = endKeys.get(item.id)
        if (!op) { op = { key: crypto.randomUUID(), remove: removeOnEnd }; endKeys.set(item.id, op) }
        await endTerminal(item.id, op.key, op.remove)
      }
      await onchanged()
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) return onsignedout(new UnauthorizedError())
      onnotice({ text: e instanceof Error ? e.message : 'The action is unconfirmed. Retry checks the same session.', bad: true })
    } finally { busy = false }
  }
  async function copyBranch() {
    if (!context?.item.worktreeBranch) return
    const branch = context.item.worktreeBranch
    closeMenu()
    onnotice((await copyText(branch)) ? { text: `Copied ${branch}.` } : { text: "Couldn't copy the branch. Select it in the session's details instead.", bad: true })
  }
  // Check and remove worktree: the existing sheet, which reads the worktree's state first and removes nothing unsaved.
  let removing = $state<{ item: TerminalSession; host: string; target: NonNullable<ReturnType<typeof worktreeTargetOf>> } | null>(null)
  function checkWorktree() {
    const item = context?.item, target = item ? worktreeTargetOf(item) : null
    if (!item || !target) return
    closeMenu(false)
    removing = { item, host: hostOf(item.agentId), target }
  }
  const member = $derived(removing ? { position: 0, host: removing.item.agentId, dir: '', tool: removing.item.tool ?? '', model: removing.item.model ?? '', state: 'started' as const, code: '', sentence: '', sessionId: removing.item.id,
    worktreePath: removing.item.worktreePath ?? '', worktreeRepo: removing.target.repo, worktreeBranch: removing.item.worktreeBranch ?? '', worktreeRun: removing.target.run, worktreeProject: removing.target.project, sharesCheckout: false, attempt: 0 } : null)
</script>

<svelte:window onpointerdown={event => { if (context && !menu?.contains(event.target as Node)) closeMenu(false) }} onblur={() => closeMenu(false)} />

{#if context}
  {@const item = context.item}
  {@const away = offline.has(item.agentId)}
  {@const target = worktreeTargetOf(item)}
  <div class="menu" role="menu" aria-label={`Actions for ${item.title}`} tabindex="-1" bind:this={menu} style={`left:${context.x}px;top:${context.y}px`} onkeydown={menuKey}>
    <p>{item.title}<small>{hostOf(item.agentId)}</small></p>
    <button role="menuitem" disabled={busy || serverOff(item)} onclick={() => begin('rename')}>Rename…</button>
    {#if away}<a role="menuitem" aria-disabled="true" title={offlineSentence(hostOf(item.agentId))}>Open in theatre</a>
    {:else}<a role="menuitem" href={`${sessionHref(item)}&theatre=1`}>Open in theatre</a>{/if}
    {#if item.worktreeBranch}<button role="menuitem" onclick={() => void copyBranch()}>Copy branch</button>{/if}
    {#if live(item)}<button role="menuitem" class="danger" disabled={busy || serverOff(item)} onclick={() => begin('end')}>{item.state === 'end_requested' ? 'Retry end request…' : 'End session…'}</button>
    {:else}<button role="menuitem" disabled={busy} onclick={() => begin('archive')}>Remove from history…</button>{/if}
    {#if target}<button role="menuitem" disabled={live(item) || away} title={live(item) ? 'End the session first: it is working in this worktree.' : away ? offlineSentence(hostOf(item.agentId)) : undefined} onclick={checkWorktree}>Check and remove worktree…</button>{/if}
  </div>
{/if}

{#if action?.kind === 'rename'}
  <ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={() => void perform()} oncancel={cancel}><label class="field">Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>
{:else if action?.kind === 'end'}
  <ConfirmDialog title={`End ${action.item.title}?`} body="This stops the shell and its running jobs. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={() => void perform()} oncancel={cancel}>
    <label class="option"><input type="checkbox" bind:checked={removeOnEnd} disabled={endKeys.has(action.item.id) || action.item.state === 'end_requested'} /><span>Remove from history when this session ends</span></label>
  </ConfirmDialog>
{:else if action?.kind === 'archive'}
  <ConfirmDialog title={`Remove ${action.item.title} from history?`} body="This removes the closed session from your history. Files on the machine are kept. You cannot restore it from the web interface." confirmLabel="Remove from history" onconfirm={() => void perform()} oncancel={cancel} />
{/if}
{#if removing && member}
  <WorktreeDialog {member} host={removing.host} opener={returnFocus ?? null} onremoved={() => void onchanged()} onsignedout={onsignedout} onclose={() => (removing = null)} />
{/if}

<style>
  .menu { position: fixed; z-index: 110; width: 240px; max-height: calc(100dvh - 16px); overflow: auto; display: grid; gap: 2px; padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--card-raised); box-shadow: var(--shadow); }
  .menu p { margin: 5px 8px 8px; font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
  .menu small { display: block; margin-top: 3px; font-size: 12px; font-weight: 400; color: var(--muted); }
  .menu [role=menuitem] { display: block; min-height: 36px; padding: 8px 9px; border: 0; border-radius: 6px; background: none; color: var(--ink); font: 400 14px var(--sans); text-align: left; text-decoration: none; cursor: pointer; }
  .menu [role=menuitem]:hover:not(:disabled):not([aria-disabled=true]), .menu [role=menuitem]:focus-visible { background: var(--tint); outline: none; }
  .menu [role=menuitem]:disabled, .menu [role=menuitem][aria-disabled=true] { opacity: .45; cursor: default; }
  .menu .danger { color: var(--bad); }
  .field { display: grid; gap: 6px; margin-top: 12px; font-size: 14px; }
  .option { display: flex; align-items: flex-start; gap: 9px; margin-top: 14px; font-size: 14px; line-height: 1.5; cursor: pointer; }
  .option input { width: 16px; height: 16px; margin: 3px 0 0; accent-color: var(--accent); }
  @media (pointer: coarse) { .menu [role=menuitem] { min-height: 44px; } }
</style>
