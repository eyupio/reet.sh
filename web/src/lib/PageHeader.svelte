<script lang="ts">
  import type { Snippet } from 'svelte'

  let { eyebrow, title, lede = '', titleId, meta, actions }: {
    eyebrow: string
    title: string
    lede?: string
    titleId?: string
    meta?: Snippet
    actions?: Snippet
  } = $props()
</script>

<header class="page-heading">
  <div class="copy">
    <p class="eyebrow">{eyebrow}</p>
    <h1 id={titleId}>{title}</h1>
    {#if lede}<p class="lede">{lede}</p>{/if}
    {#if meta}<div class="meta">{@render meta()}</div>{/if}
  </div>
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
</header>

<style>
  .page-heading { display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; margin-bottom: 28px; }
  .copy { min-width: 0; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h1 { margin: 10px 0 12px; font: 500 clamp(2rem, 4vw, 3rem)/1.07 var(--serif); letter-spacing: -0.035em; overflow-wrap: anywhere; }
  .lede { margin: 0; color: var(--muted); max-width: 60ch; font-size: 0.88rem; }
  .meta { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; color: var(--muted); font-size: 0.78rem; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; flex: none; }
  @media (max-width: 1000px) {
    .page-heading { align-items: flex-start; flex-direction: column; gap: 10px; margin-bottom: 16px; }
    h1 { margin: 6px 0 8px; font-size: clamp(1.75rem, 4vw, 2.25rem); }
  }
</style>
