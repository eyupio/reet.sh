<script lang="ts">
  import TerminalIcon from '../terminal/TerminalIcon.svelte'
  import type { TerminalActions } from '../terminal/TerminalPane.svelte'

  // The one terminal toolbar of a desk tab: it names the focused pane and acts on it, so nine panes do not carry nine
  // toolbars. The pane's own TerminalPane (chrome={false}) does the work and opens its own dialogs; a pane with no
  // terminal (a state card) leaves the terminal's buttons off. Split right, Split down, Zoom and Theatre sit at the end.
  let { name, actions, splits = true, canZoom = true, zoomed = false, theatre = false, onsplit, onzoom, ontheatre }: {
    /** The focused pane's name. */
    name: string
    /** The focused pane's terminal, when it has one. */
    actions: TerminalActions | undefined
    /** Splits and zoom are offered (not on a phone). */
    splits?: boolean
    /** There is more than one pane to zoom between. */
    canZoom?: boolean
    zoomed?: boolean
    theatre?: boolean
    onsplit: (dir: 'right' | 'down') => void
    onzoom: () => void
    ontheatre: () => void
  } = $props()

  const a = $derived(actions)
  const off = $derived(!a)
  const reading = $derived(!!a?.scrollback)
</script>

<div class="desk-toolbar" role="toolbar" aria-label={`Terminal tools for ${name}`}>
  <b class="who" title={name}>{name}</b>
  <div class="tools">
    <button type="button" disabled={off} onclick={() => a?.earlierOutput()} aria-pressed={reading} aria-label="Earlier output" title="Scroll earlier terminal output · Shift+PageUp"><TerminalIcon name="history" /><span class="tl">Earlier output</span></button>
    <button type="button" disabled={off || reading} onclick={() => a?.compose()} aria-haspopup="dialog" aria-label="Compose" title="Write or dictate a message, then send it as one paste"><TerminalIcon name="compose" /><span class="tl">Compose</span></button>
    <button type="button" disabled={off} onclick={() => a?.search()} aria-expanded={!!a?.searching} aria-label="Search" title="Search terminal output"><TerminalIcon name="search" /><span class="tl">Search</span></button>
    <button type="button" disabled={off} onclick={() => a?.copy()} aria-label="Copy selection" title="Copy selected output"><TerminalIcon name="copy" /><span class="tl">Copy</span></button>
    <button type="button" disabled={off || reading} onclick={() => a?.copyScreen()} aria-label="Copy screen" title="Copy the visible screen"><TerminalIcon name="screen" /><span class="tl wide">Screen</span></button>
    <button type="button" disabled={off || reading} onclick={() => a?.selectText()} aria-label="Select text" title="Select terminal text"><TerminalIcon name="select" /><span class="tl wide">Select text</span></button>
    <button type="button" disabled={off || !a?.live || reading} onclick={() => a?.paste()} aria-label="Paste" title="Paste clipboard"><TerminalIcon name="paste" /><span class="tl">Paste</span></button>
    {#if a?.uploadsSupported || a?.uploadsOff}<button type="button" disabled={a.uploadsOff} onclick={() => a?.attach()} aria-label="Attach image" title={a.uploadsOff ? 'Image uploads are off on this machine.' : 'Attach a PNG, JPEG, GIF or WebP image'}><TerminalIcon name="image" /><span class="tl">Attach</span></button>{/if}
    <span class="div" aria-hidden="true"></span>
    <button type="button" disabled={off || !a?.live || reading} onclick={() => a?.interrupt()} aria-label="Interrupt shell" title="Interrupt · Ctrl+C"><TerminalIcon name="interrupt" /><span class="tl wide">Interrupt</span></button>
    <button type="button" disabled={off} onclick={() => a?.commands()} aria-label="Saved commands" title="Your saved commands"><TerminalIcon name="commands" /><span class="tl">Commands</span></button>
    <button type="button" disabled={off} onclick={event => a?.prompts(event.currentTarget)} aria-haspopup="dialog" aria-label="Prompts" title="Insert one of your saved prompts"><span>Prompts</span></button>
    {#if a?.skills}<button type="button" disabled={!!a.skillsOff} onclick={() => a?.openSkills()} aria-haspopup="dialog" aria-label="Skills" title={a.skillsOff || 'Insert a skill into the agent'}><span>Skills</span></button>{/if}
    <button type="button" disabled={off} onclick={() => a?.history()} aria-label="Recent history" title="Find saved shell commands or read terminal output"><TerminalIcon name="history" /><span class="tl">History</span></button>
    <button type="button" disabled={off} onclick={() => a?.appearance()} aria-label="Appearance" title="Theme, text size and cursor"><TerminalIcon name="appearance" /><span class="tl wide">Appearance</span></button>
    <span class="space"></span>
    {#if splits}
      <button type="button" onclick={() => onsplit('right')} aria-label="Split right" title="Split right · Alt+Shift+D"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.5 2.5h13v11h-13zM8 2.5v11" /></svg><span class="tl">Split right</span></button>
      <button type="button" onclick={() => onsplit('down')} aria-label="Split down" title="Split down · Alt+Shift+E"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.5 2.5h13v11h-13zM1.5 8h13" /></svg><span class="tl">Split down</span></button>
      <button type="button" disabled={!canZoom} onclick={onzoom} aria-pressed={zoomed} aria-label={zoomed ? 'Show all panes' : 'Zoom'} title={zoomed ? 'Show all panes · Alt+Shift+Z' : 'Zoom this pane · Alt+Shift+Z'}><TerminalIcon name={zoomed ? 'contract' : 'expand'} /><span class="tl">{zoomed ? 'Show all' : 'Zoom'}</span></button>
    {/if}
    <button type="button" onclick={ontheatre} aria-pressed={theatre} aria-label={theatre ? 'Exit theatre view' : 'Theatre view'} title={theatre ? 'Show the top bar and desk header' : 'Wide terminal · theatre view'}><TerminalIcon name="theatre" /></button>
  </div>
</div>

<style>
  .desk-toolbar { flex: none; display: flex; align-items: center; gap: 10px; min-width: 0; padding: 3px 8px 3px 14px; border-bottom: 1px solid var(--line-soft); background: var(--bg); font-size: 14px; }
  .who { flex: 0 1 auto; max-width: 22ch; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink-soft); font-weight: 600; }
  .tools { flex: 1; min-width: 0; display: flex; align-items: center; gap: 2px; overflow-x: auto; scrollbar-width: thin; }
  button { flex: none; display: flex; align-items: center; gap: 6px; min-height: 32px; padding: 5px 8px; border: 1px solid transparent; border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 14px; white-space: nowrap; cursor: pointer; }
  button:hover:not(:disabled) { color: var(--ink); background: var(--card); border-color: var(--line); }
  button:disabled { opacity: 0.45; cursor: default; }
  button[aria-pressed='true'] { color: var(--accent-text); border-color: var(--accent); }
  svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.35; stroke-linejoin: round; }
  .div { width: 1px; height: 16px; margin: 0 4px; flex: none; background: var(--line); }
  .space { flex: 1; min-width: 8px; }
  /* Labels go first, then nothing else changes: every button keeps its name for a screen reader through aria-label. */
  @media (max-width: 1599px) { .wide { display: none; } }
  @media (max-width: 1280px) { .tl { display: none; } button { padding: 6px; min-width: 32px; justify-content: center; } }
  /* On a tablet the tools wrap onto a second line rather than hide their last controls (Zoom, Theatre) off the end; a
     phone keeps one line that scrolls, to leave its height to the terminal. */
  @media (min-width: 701px) and (max-width: 1280px) { .tools { flex-wrap: wrap; row-gap: 2px; } }
  @media (pointer: coarse) { button { min-width: 44px; min-height: 44px; } }
  @media (max-width: 700px) { .desk-toolbar { padding: 0 4px 0 10px; gap: 6px; } .space { display: none; } }
</style>
