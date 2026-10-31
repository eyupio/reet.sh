<script lang="ts">
  import { onDestroy, onMount } from 'svelte'

  let { message, actionLabel, onaction, ondismiss, duration = 8000, placement = 'center', tone = '' }: {
    message: string
    actionLabel?: string
    onaction?: () => void | Promise<void>
    ondismiss: () => void
    duration?: number
    /** 'right' is the usual corner toast; 'center' is the original bottom-centre one. */
    placement?: 'center' | 'right'
    tone?: '' | 'ok' | 'bad'
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
  class:right={placement === 'right'}
  class:ok={tone === 'ok'}
  class:bad={tone === 'bad'}
  role="status"
  aria-live="polite"
  onmouseenter={pause}
  onmouseleave={resume}
  onfocusin={pause}
  onfocusout={resume}
>
  {#if tone}<span class="mark" aria-hidden="true">{#if tone === 'ok'}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>{:else}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><path d="M12 7v6M12 17h.01" /></svg>{/if}</span>{/if}
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
    position: fixed; left: 50%; bottom: calc(24px + var(--bottom-chrome, 0px)); z-index: 20; transform: translate(-50%, 0);
    display: flex; align-items: center; gap: 10px; width: max-content;
    max-width: min(460px, calc(100vw - 32px)); padding: 10px 10px 10px 14px;
    background: var(--card); border: 1px solid var(--line); border-left: 3px solid var(--accent);
    border-radius: 12px; box-shadow: 0 18px 50px -20px rgb(0 0 0 / 0.4);
  }
  .toast.right { left: auto; right: 24px; transform: none; max-width: min(380px, calc(100vw - 32px)); }
  .toast.ok { border-left-color: var(--good); }
  .toast.bad { border-left-color: var(--bad); }
  .mark { flex: none; display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; color: var(--accent-ink); background: var(--accent); }
  .ok .mark { background: var(--good); }
  .bad .mark { background: var(--bad); }
  .mark svg { width: 13px; height: 13px; }
  .msg { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .action {
    flex: none; background: none; border: 0; padding: 6px 8px; min-height: 36px;
    color: var(--accent-text); font-weight: 650; cursor: pointer; border-radius: 8px;
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
  @media (max-width: 560px) { .toast { bottom: calc(16px + var(--bottom-chrome, 0px)); } .toast.right { right: 16px; } }
  @media (prefers-reduced-motion: no-preference) {
    .toast { transition: opacity 180ms ease, transform 180ms var(--ease-out); }
    .toast.leaving { opacity: 0; transform: translate(-50%, 8px); }
    .toast.right.leaving { transform: translateX(12px); }
    @starting-style {
      .toast.right { opacity: 0; transform: translateX(16px); }
      .toast { opacity: 0; transform: translate(-50%, 8px); }
    }
  }
</style>
