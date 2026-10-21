<script lang="ts">
  import { onMount } from 'svelte'

  let {
    title,
    body,
    confirmLabel = 'Confirm',
    secondaryLabel,
    danger = false,
    onconfirm,
    onsecondary,
    oncancel,
  }: {
    title: string
    body: string
    confirmLabel?: string
    secondaryLabel?: string
    danger?: boolean
    onconfirm: () => void
    onsecondary?: () => void
    oncancel: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let cancelBtn = $state<HTMLButtonElement>()

  onMount(() => {
    dialog?.showModal()
    cancelBtn?.focus()
    return () => dialog?.close()
  })

  function cancel() {
    dialog?.close()
    oncancel()
  }

  function confirm() {
    dialog?.close()
    onconfirm()
  }

  function secondary() {
    dialog?.close()
    onsecondary?.()
  }
</script>

<dialog bind:this={dialog} aria-labelledby="confirm-title" oncancel={(e) => { e.preventDefault(); cancel() }}>
  <div class="box">
    <h2 id="confirm-title">{title}</h2>
    <p>{body}</p>
    <div class="actions">
      <button type="button" class="btn quiet" bind:this={cancelBtn} onclick={cancel}>Cancel</button>
      {#if secondaryLabel && onsecondary}
        <button type="button" class="btn quiet secondary" onclick={secondary}>{secondaryLabel}</button>
      {/if}
      <button type="button" class="btn primary" class:danger onclick={confirm}>{confirmLabel}</button>
    </div>
  </div>
</dialog>

<style>
  dialog {
    max-width: min(440px, calc(100vw - 32px)); padding: 0; border: 0;
    background: transparent; color: var(--ink);
  }
  dialog::backdrop { background: rgb(20 18 15 / 0.45); backdrop-filter: blur(2px); }
  .box {
    padding: 24px; background: var(--card); border: 1px solid var(--line); border-radius: 16px;
    box-shadow: 0 28px 80px -24px rgb(0 0 0 / 0.45);
  }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; }
  p { margin: 10px 0 0; color: var(--muted); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; flex-wrap: wrap; }
  .danger { background: var(--bad); border-color: var(--bad); color: var(--bg); }
  .secondary { color: var(--bad); }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 14px; }
    .actions { flex-direction: column; }
    .actions button { flex: none; width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: scale(0.97); }
      to { opacity: 1; transform: scale(1); }
    }
  }
</style>
