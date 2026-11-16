<script lang="ts">
  import { hasHiddenChars } from '../terminal/api'

  // The start-up command a pane offers once it has been reopened or started here: the command, Run and Dismiss, in a
  // band above the terminal. Nothing runs without the press; Run waits until the terminal is connected. The whole command
  // is shown, wrapped, so what Run types is what is on screen; a command with a hidden character is never offered to run.
  let { command, ready, onrun, ondismiss }: {
    command: string
    /** The terminal is connected, so Run can type into it. */
    ready: boolean
    onrun: () => void
    ondismiss: () => void
  } = $props()

  const hidden = $derived(hasHiddenChars(command))
</script>

<div class="sbar" role="group" aria-label="Start-up command">
  <span class="lbl">Start-up command</span>
  <code>{command}</code>
  <span class="acts">
    <button type="button" class="btn primary" disabled={!ready || hidden} title={hidden ? undefined : ready ? 'Type it into this shell and press Enter' : 'Waiting for the terminal to connect'} onclick={() => { if (!hidden) onrun() }}>Run</button>
    <button type="button" class="btn" onclick={ondismiss}>Dismiss</button>
  </span>
  {#if hidden}<p class="bad" role="alert">This command has hidden characters, so reet won't run it. Edit it first.</p>{/if}
</div>

<style>
  .sbar { flex: none; order: -1; display: flex; flex-wrap: wrap; align-items: flex-start; gap: 6px 8px; min-width: 0; padding: 6px 10px; background: var(--tint); border-bottom: 1px solid var(--line); font-size: 14px; }
  .lbl { flex: none; color: var(--muted); line-height: 30px; }
  /* The whole command, wrapped; a long one scrolls inside the bar rather than being cut off. */
  code { flex: 1 1 12ch; min-width: 0; max-height: 7.5em; overflow-y: auto; padding: 5px 0; font-family: var(--mono); color: var(--ink); line-height: 1.45; white-space: pre-wrap; overflow-wrap: anywhere; }
  .acts { flex: none; display: flex; gap: 8px; }
  .btn { flex: none; min-height: 30px; padding: 4px 12px; }
  .bad { flex: 1 0 100%; margin: 0; color: var(--bad); }
  @media (pointer: coarse) { .btn { min-height: 44px; } .lbl { line-height: 44px; } code { padding: 12px 0; } }
  @media (max-width: 520px) { .lbl { display: none; } }
</style>
