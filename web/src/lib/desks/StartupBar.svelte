<script lang="ts">
  // The start-up command a pane offers once it has been reopened or started here: the command, Run and Dismiss, in a
  // band above the terminal. Nothing runs without the press; Run waits until the terminal is connected.
  let { command, ready, onrun, ondismiss }: {
    command: string
    /** The terminal is connected, so Run can type into it. */
    ready: boolean
    onrun: () => void
    ondismiss: () => void
  } = $props()
</script>

<div class="sbar" role="group" aria-label="Start-up command">
  <span class="lbl">Start-up command</span>
  <code title={command}>{command}</code>
  <button type="button" class="btn primary" disabled={!ready} title={ready ? 'Type it into this shell and press Enter' : 'Waiting for the terminal to connect'} onclick={onrun}>Run</button>
  <button type="button" class="btn" onclick={ondismiss}>Dismiss</button>
</div>

<style>
  .sbar { flex: none; order: -1; display: flex; align-items: center; gap: 8px; min-width: 0; padding: 6px 10px; background: var(--tint); border-bottom: 1px solid var(--line); font-size: 14px; }
  .lbl { flex: none; color: var(--muted); }
  code { flex: 1; min-width: 0; font-family: var(--mono); color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .btn { flex: none; min-height: 30px; padding: 4px 12px; }
  @media (pointer: coarse) { .btn { min-height: 44px; } }
  @media (max-width: 520px) { .lbl { display: none; } }
</style>
