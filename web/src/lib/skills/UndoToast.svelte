<script lang="ts">
  // The Skills page's toast: what just changed, with Undo when it can be undone. It stays about 8 seconds, and waits
  // while the pointer or keyboard is on it. One live region holds it, so each new message is read out.
  export type UndoMessage = { id: number; text: string; undo?: () => void }

  let { toast, raised = false, onclose }: {
    toast: UndoMessage | null
    /** The bulk bar is up: the toast sits above it. */
    raised?: boolean
    onclose: () => void
  } = $props()

  const SHOW_MS = 8000
  let timer: ReturnType<typeof setTimeout> | undefined

  function close(id: number, after: number) {
    clearTimeout(timer)
    timer = setTimeout(() => { if (toast?.id === id) onclose() }, after)
  }
  $effect(() => {
    const id = toast?.id
    if (id) close(id, SHOW_MS)
    return () => clearTimeout(timer)
  })
  function hold() { clearTimeout(timer) }
  function release() { if (toast) close(toast.id, SHOW_MS / 2) }
</script>

<div class="toast-region" class:raised role="status" aria-live="polite" onmouseenter={hold} onmouseleave={release} onfocusin={hold} onfocusout={release}>
  {#if toast}
    {#key toast.id}
      <div class="toast">
        <span>{toast.text}</span>
        {#if toast.undo}<button type="button" onclick={toast.undo}>Undo</button>{/if}
      </div>
    {/key}
  {/if}
</div>

<style>
  .toast-region { position: fixed; left: 50%; bottom: calc(28px + var(--bottom-chrome, 0px)); transform: translateX(-50%); z-index: 20; width: max-content; max-width: min(560px, calc(100vw - 24px)); }
  .toast-region.raised { bottom: calc(100px + var(--bottom-chrome, 0px)); }
  .toast {
    display: flex; align-items: center; gap: 14px; padding: 6px 6px 6px 18px; border-radius: 12px;
    background: var(--ink); color: var(--bg); font-size: 0.9375rem; box-shadow: 0 18px 40px -18px var(--scrim);
  }
  .toast span { flex: 1; min-width: 0; overflow-wrap: anywhere; padding: 8px 0; }
  .toast button { flex: none; min-height: 44px; min-width: 44px; border: 0; border-radius: 8px; padding: 0 14px; cursor: pointer; font-weight: 600; background: color-mix(in srgb, var(--bg) 16%, var(--ink)); color: var(--tint); }
  .toast button:hover { background: color-mix(in srgb, var(--bg) 26%, var(--ink)); }
  .toast button:focus-visible { outline-color: var(--tint); }
  @media (prefers-reduced-motion: no-preference) {
    .toast { animation: toast-in 180ms var(--ease-out) both; }
    @keyframes toast-in { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
  }
  @media (max-width: 560px) {
    .toast-region { left: 12px; right: 12px; transform: none; width: auto; max-width: none; bottom: calc(16px + var(--bottom-chrome, 0px)); }
    .toast-region.raised { bottom: calc(150px + var(--bottom-chrome, 0px)); }
  }
</style>
