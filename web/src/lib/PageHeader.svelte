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
  .page-heading { display: flex; justify-content: space-between; align-items: center; gap: 24px; margin-bottom: 32px; }
  .copy { min-width: 0; }
  .eyebrow { margin: 0; color: var(--accent-text); font: 500 14px/1.5 var(--mono); letter-spacing: 0.01em; }
  h1 { margin: 12px 0; font: 600 clamp(2.2rem, 4vw, 3rem)/1.1 var(--serif); letter-spacing: -0.03em; overflow-wrap: anywhere; }
  .lede { margin: 0; color: var(--muted); max-width: 64ch; font-size: 14px; line-height: 1.65; }
  .meta { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; flex: none; }
  @media (max-width: 1000px) {
    .page-heading { align-items: flex-start; flex-direction: column; gap: 12px; margin-bottom: 16px; }
    h1 { margin: 8px 0; font-size: clamp(1.9rem, 5vw, 2.5rem); }
  }
</style>
