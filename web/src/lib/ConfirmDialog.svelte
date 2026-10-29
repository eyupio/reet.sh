<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import type { Snippet } from 'svelte'

  let {
    title,
    body,
    details,
    confirmLabel = 'Confirm',
    secondaryLabel,
    danger = false,
    onconfirm,
    onsecondary,
    oncancel,
    children,
  }: {
    title: string
    body: string
    details?: string
    confirmLabel?: string
    secondaryLabel?: string
    danger?: boolean
    onconfirm: () => void
    onsecondary?: () => void
    oncancel: () => void
    children?: Snippet
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
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

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={(e) => { e.preventDefault(); cancel() }}>
  <div class="box" data-shell-card>
    <h2 id={titleId}>{title}</h2>
    <p>{body}</p>
    {#if details}<textarea class="details" readonly rows="8" aria-label="Proposed changes" value={details}></textarea>{/if}
    {#if children}{@render children()}{/if}
    <div class="actions" data-shell-foot>
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
    max-width: min(440px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0;
    background: transparent; color: var(--ink);
  }
  dialog::backdrop { background: rgb(20 18 15 / 0.45); backdrop-filter: blur(2px); }
  .box {
    padding: 28px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius);
    box-shadow: 0 28px 80px -24px rgb(0 0 0 / 0.45); overflow-wrap: anywhere;
  }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; text-wrap: balance; }
  p { margin: 10px 0 0; color: var(--muted); overflow-wrap: anywhere; }
  .details { width:100%; box-sizing:border-box; margin:16px 0 0; padding:12px; max-height:240px; overflow:auto; resize:vertical; white-space:pre-wrap; overflow-wrap:anywhere; font:.76rem/1.5 ui-monospace,monospace; border:1px solid var(--line); border-radius:8px; background:var(--bg); color:var(--ink); }
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
