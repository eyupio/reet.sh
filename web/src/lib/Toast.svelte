<script lang="ts">
  import { onDestroy, onMount } from 'svelte'

  let { message, actionLabel, onaction, ondismiss, duration = 8000 }: {
    message: string
    actionLabel?: string
    onaction?: () => void | Promise<void>
    ondismiss: () => void
    duration?: number
  } = $props()

  let leaving = $state(false)
  let acting = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let remaining = 0
  let started = 0

  function start() {
    started = Date.now()
    timer = setTimeout(dismiss, remaining)
  }

  function pause() {
    if (!timer) return
    clearTimeout(timer)
    timer = undefined
    remaining = Math.max(0, remaining - (Date.now() - started))
  }

  function resume() {
    if (timer || leaving) return
    if (remaining <= 0) {
      dismiss()
      return
    }
    start()
  }

  function dismiss() {
    if (leaving) return
    leaving = true
    clearTimeout(timer)
    timer = undefined
    setTimeout(ondismiss, 180)
  }

  async function act() {
    if (!onaction || acting) return
    acting = true
    try {
      await onaction()
    } finally {
      acting = false
      dismiss()
    }
  }

  onMount(() => {
    remaining = duration
    start()
  })
  onDestroy(() => clearTimeout(timer))
</script>

<div
  class="toast"
  class:leaving
  role="status"
  aria-live="polite"
  onmouseenter={pause}
  onmouseleave={resume}
  onfocusin={pause}
  onfocusout={resume}
>
  <span class="msg">{message}</span>
  {#if actionLabel && onaction}
    <button type="button" class="action" onclick={act} disabled={acting}>{acting ? 'Working…' : actionLabel}</button>
  {/if}
  <button type="button" class="close" onclick={dismiss} aria-label="Dismiss">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
      <path d="m7 7 10 10M17 7 7 17"></path>
    </svg>
  </button>
</div>

<style>
  .toast {
    position: fixed; left: 50%; bottom: 24px; z-index: 20; transform: translate(-50%, 0);
    display: flex; align-items: center; gap: 10px; width: max-content;
    max-width: min(460px, calc(100vw - 32px)); padding: 10px 10px 10px 14px;
    background: var(--card); border: 1px solid var(--line); border-left: 3px solid var(--accent);
    border-radius: 12px; box-shadow: 0 18px 50px -20px rgb(0 0 0 / 0.4);
  }
  .msg { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .action {
    flex: none; background: none; border: 0; padding: 6px 8px; min-height: 36px;
    color: var(--accent); font-weight: 650; cursor: pointer; border-radius: 8px;
  }
  .action:hover:not(:disabled) { background: var(--tint); }
  .action:disabled { opacity: 0.6; cursor: default; }
  .close {
    flex: none; display: grid; place-items: center; width: 36px; height: 36px; padding: 0;
    background: none; border: 0; border-radius: 8px; color: var(--muted); cursor: pointer;
  }
  .close:hover { color: var(--ink); background: var(--tint); }
  .close svg { width: 16px; height: 16px; }
  @media (pointer: coarse) {
    .action, .close { min-height: 44px; }
    .close { width: 44px; height: 44px; }
  }
  @media (max-width: 560px) { .toast { bottom: 16px; } }
  @media (prefers-reduced-motion: no-preference) {
    .toast { transition: opacity 180ms ease, transform 180ms var(--ease-out); }
    .toast.leaving { opacity: 0; transform: translate(-50%, 8px); }
    @starting-style {
      .toast { opacity: 0; transform: translate(-50%, 8px); }
    }
  }
</style>
