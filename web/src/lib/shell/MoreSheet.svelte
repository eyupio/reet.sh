<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import type { Tab } from './tabs'
  // More on a tablet or a phone (and on a desktop with no account menu): every page that is not one of the bar's tabs.
  // A bottom sheet on the shared dialog shell; Escape or Close hands focus back to the button that opened it.
  let { items, opener, onclose }: { items: Tab[]; opener?: HTMLElement; onclose: () => void } = $props()
  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()

  onMount(() => {
    dialog?.showModal()
    return () => dialog?.close()
  })
  function close() {
    dialog?.close()
    onclose()
    opener?.focus()
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId}
  oncancel={(event) => { event.preventDefault(); close() }}
  onclick={(event) => { if (event.target === dialog) close() }}>
  <div class="sheet" data-shell-card>
    <header data-shell-head>
      <h2 id={titleId}>More</h2>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    </header>
    <nav aria-labelledby={titleId} data-shell-body>
      {#each items as item (item.href)}
        <a href={item.href} aria-current={item.current ? 'page' : undefined}>{item.label}</a>
      {/each}
    </nav>
  </div>
</dialog>

<style>
  dialog {
    width: min(360px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; overflow: auto;
    background: transparent; color: var(--ink);
  }
  dialog::backdrop { background: color-mix(in srgb, var(--bg) 60%, transparent); }
  .sheet { padding: 8px 8px 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card-raised); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 4px 4px 12px; }
  h2 { margin: 0; font: 600 1.15rem/1.3 var(--serif); }
  .close {
    display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 10px;
    background: none; color: var(--muted); cursor: pointer;
  }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 20px; height: 20px; }
  nav { display: grid; gap: 2px; }
  a {
    display: flex; align-items: center; min-height: 48px; padding: 0 12px; border-radius: 10px;
    color: var(--ink); font-size: 0.95rem; text-decoration: none;
  }
  a:hover { background: var(--tint); }
  a[aria-current='page'] { background: var(--tint); font-weight: 600; box-shadow: inset 3px 0 0 var(--accent-text); }
</style>
