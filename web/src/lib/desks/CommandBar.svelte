<script lang="ts">
  import type { TerminalActions } from '../terminal/TerminalPane.svelte'
  import { browseFolder, commandHistory, hasHiddenChars, savedCommands, type BrowseResult } from '../terminal/api'
  import { pathToken, resolveDir, suggest } from './commandBar'

  // The desk's command bar: one field under the panes that runs a command in the focused pane's shell, with the next
  // actions beside it. Empty, it offers the folders of the shell's own folder to cd into and the commands run lately or
  // saved; as a command is typed it completes it. A real shell does not tell reet what is on its prompt, so the bar owns
  // its own field. A suggestion only fills the field: nothing runs without Enter or Run.
  let { name, actions, session = '', agentId = '', dir = '', shell = false, history = false }: {
    /** The focused pane's name. */
    name: string
    /** The focused pane's terminal, when it has one. */
    actions: TerminalActions | undefined
    /** The pane's session id, for its command history. */
    session?: string
    /** The pane's machine. */
    agentId?: string
    /** The folder the shell is in now. */
    dir?: string
    /** The pane runs a shell, not an agent. */
    shell?: boolean
    /** The machine can read saved shell history. */
    history?: boolean
  } = $props()

  const id = $props.id()
  let input = $state<HTMLInputElement>(), list = $state<HTMLElement>()
  let value = $state(''), active = $state(0), note = $state('')
  export function focus() { input?.focus() }

  const off = $derived(!shell ? (actions ? 'The command bar is for shell panes. Compose writes to an agent.' : 'This pane has no running shell.') : !actions?.live ? 'Waiting for the terminal to connect…' : actions.scrollback ? 'Return to live output to run a command.' : '')

  // Saved commands are the account's; history is the pane's machine's, with what this bar ran since put first (a shell
  // may not have written its latest lines yet).
  let saved = $state<string[]>([]), read = $state<string[]>([]), ran = $state<string[]>([])
  $effect(() => { savedCommands().then(items => (saved = items.map(item => item.command))).catch(() => {}) })
  $effect(() => {
    ran = []; read = []; value = ''; note = ''
    if (!session || !shell || !history) return
    const life = new AbortController()
    commandHistory(session, life.signal).then(snapshot => (read = snapshot.entries.slice(0, 200).map(entry => entry.command))).catch(() => {})
    return () => life.abort()
  })

  // The folder whose folders are offered: the shell's own, or the one the path being typed names. A listing is kept for
  // half a minute, so changing pane or typing costs the machine nothing more.
  const nonce = crypto.randomUUID(), KEEP = 30_000
  const kept = new Map<string, { at: number; result: BrowseResult }>()
  let listing = $state<BrowseResult | null>(null), home = $state('')
  const token = $derived(pathToken(value))
  const wanted = $derived(off || !agentId || !dir || (token?.dir.startsWith('~') && !home) ? '' : resolveDir(dir, home, token?.dir ?? ''))
  $effect(() => {
    const key = `${agentId}\n${wanted}`, have = kept.get(key)
    if (!wanted) { listing = null; return }
    if (have && Date.now() - have.at < KEEP) { listing = have.result; return }
    listing = null
    const life = new AbortController()
    const wait = setTimeout(() => {
      // shortcut: the first page of a folder only, read the next pages when a folder of hundreds needs completing.
      browseFolder(agentId, { path: wanted, session: nonce, hidden: true }, life.signal).then(result => {
        kept.set(key, { at: Date.now(), result })
        if (!life.signal.aborted) { listing = result; home = result.home ?? '' }
      }).catch(() => {})
    }, 200)
    return () => { clearTimeout(wait); life.abort() }
  })

  const items = $derived(off ? [] : suggest(value, { folders: listing?.entries ?? [], parent: !!listing && listing.parent !== listing.path, recent: [...ran, ...read], saved }))
  $effect(() => { void value; active = 0 })
  $effect(() => { list?.children[active]?.scrollIntoView({ block: 'nearest', inline: 'nearest' }) })

  function accept(index: number) {
    const item = items[index]
    if (!item) return
    value = item.text; note = ''
    input?.focus()
  }
  function run(event: SubmitEvent) {
    event.preventDefault()
    const text = value.trim()
    if (!text || off) return
    // What runs must be what the field shows.
    if (hasHiddenChars(text)) { note = "This command has hidden characters, so reet won't run it. Edit it first."; return }
    if (!actions?.send(`${text}\r`)) { note = 'Not sent: the terminal is not connected. Your command is kept.'; return }
    ran = [text, ...ran.filter(command => command !== text)].slice(0, 50)
    value = ''; note = ''
  }
  function keys(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); actions?.focus(); return }
    if (!items.length || event.altKey || event.ctrlKey || event.metaKey) return
    if (event.key === 'Tab' && !event.shiftKey) { event.preventDefault(); accept(active) }
    else if (event.key === 'ArrowDown') { event.preventDefault(); active = (active + 1) % items.length }
    else if (event.key === 'ArrowUp') { event.preventDefault(); active = (active - 1 + items.length) % items.length }
  }
  const WHERE = { folder: 'Folder', recent: 'From this machine’s history', saved: 'Saved command' }
</script>

<form class="cbar" aria-label={`Command bar for ${name}`} onsubmit={run}>
  <label class="field" class:off={!!off}>
    <span class="mark" aria-hidden="true">›</span>
    <input bind:this={input} bind:value role="combobox" aria-label={`Command to run in ${name}`} aria-autocomplete="list" aria-expanded={items.length > 0} aria-controls={`${id}-list`}
      aria-activedescendant={items.length ? `${id}-${active}` : undefined} aria-describedby={note ? `${id}-note` : undefined}
      disabled={!!off} placeholder={off || `Run a command in ${name}`} maxlength="2000" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" enterkeyhint="go" onkeydown={keys} />
  </label>
  <div class="opts" role="listbox" id={`${id}-list`} aria-label="Suggestions" bind:this={list}>
    {#each items as item, i (item.text)}
      <!-- svelte-ignore a11y_click_events_have_key_events (the field's own keys choose a suggestion: arrows, then Tab) -->
      <div class="opt" role="option" tabindex="-1" id={`${id}-${i}`} aria-selected={i === active} title={`${WHERE[item.kind]}: ${item.text}`} onpointerdown={event => event.preventDefault()} onclick={() => accept(i)}><span>{item.label}</span>{#if i === active}<kbd aria-hidden="true">Tab</kbd>{/if}</div>
    {/each}
  </div>
  <button type="submit" class="run" disabled={!!off || !value.trim()} title="Type it into this shell and press Enter">Run</button>
  {#if note}<p class="note" id={`${id}-note`} role="alert">{note}</p>{/if}
</form>

<style>
  /* One line beside the panes' foot, the toolbar's twin: the field keeps its width and the suggestions take the rest,
     so nothing moves as they come and go. */
  .cbar { flex: none; display: flex; align-items: center; gap: 8px; min-width: 0; padding: 4px 8px 4px 14px; border-top: 1px solid var(--line-soft); background: var(--bg); font-size: 14px; }
  .field { flex: 0 1 46ch; min-width: 20ch; display: flex; align-items: center; gap: 8px; min-height: 32px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg-sunk); cursor: text; }
  .field:focus-within { border-color: var(--accent-text); outline: 2px solid color-mix(in srgb, var(--accent-text) 28%, transparent); outline-offset: 1px; }
  .field.off { background: transparent; cursor: default; }
  .mark { flex: none; color: var(--accent-text); font: 700 14px var(--mono); }
  .off .mark { color: var(--faint); }
  .field input { flex: 1; min-width: 0; width: 100%; min-height: 30px; padding: 0; border: 0; background: transparent; color: var(--ink); font: 14px var(--mono); caret-color: var(--accent-text); text-overflow: ellipsis; }
  .field input:focus, .field input:focus-visible { outline: 0; border: 0; box-shadow: none; }
  .field input::placeholder { color: var(--muted); font-family: var(--sans); opacity: 1; }
  .field input::selection { background: color-mix(in srgb, var(--accent-text) 32%, transparent); }
  .opts { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; overflow-x: auto; scrollbar-width: none; scroll-padding-inline: 8px; }
  .opt { flex: none; display: flex; align-items: center; gap: 8px; max-width: 34ch; min-height: 28px; padding: 0 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--card); color: var(--ink-soft); font: 14px var(--mono); white-space: nowrap; cursor: pointer; }
  .opt span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .opt:hover { border-color: var(--line-strong); color: var(--ink); }
  /* The suggestion Tab takes is marked only while the field has the keyboard. */
  .cbar:focus-within .opt[aria-selected='true'] { border-color: var(--accent-text); background: var(--tint); color: var(--ink); }
  kbd { display: none; flex: none; padding: 1px 5px; border: 1px solid var(--line-strong); border-radius: 4px; background: var(--bg); color: var(--muted); font: 14px/1.2 var(--sans); }
  .cbar:focus-within kbd { display: inline; }
  .run { flex: none; min-height: 32px; padding: 5px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; cursor: pointer; }
  .run:hover:not(:disabled) { border-color: var(--accent-text); background: var(--tint); }
  .run:disabled { opacity: 0.45; cursor: default; }
  .note { flex: 1 0 100%; margin: 0; padding: 0 2px 2px; color: var(--bad); }
  .cbar:has(.note) { flex-wrap: wrap; }
  /* A touch screen: 44 px targets, 16 px in the field so the browser does not zoom to it, and no key to hint at. */
  @media (pointer: coarse) { .field, .opt, .run { min-height: 44px; } .field input { font-size: 16px; } .cbar:focus-within kbd { display: none; } }
  /* A phone: the suggestions take a line of their own above the field. */
  @media (max-width: 700px) {
    .cbar { flex-wrap: wrap; padding: 4px 8px; gap: 6px; }
    .opts { order: -1; flex: 1 0 100%; }
    .opts:empty { display: none; }
    .field { flex: 1; }
  }
</style>
