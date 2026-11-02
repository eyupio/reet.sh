<script lang="ts">
  import type { Snippet } from 'svelte'

  // One bounded surface for a collection of records: a count, the page's own controls,
  // an optional filter toolbar, and the records themselves. The page keeps its heading;
  // this panel carries the working part of it.
  let { label, count, actions, toolbar, children }: {
    label: string
    count: string
    actions?: Snippet
    toolbar?: Snippet
    children: Snippet
  } = $props()
</script>

<section class="collection" aria-label={label}>
  <div class="head">
    <p class="count" role="status">{count}</p>
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </div>
  {#if toolbar}<div class="toolbar">{@render toolbar()}</div>{/if}
  <div class="body">{@render children()}</div>
</section>

<style>
  .collection { border: 1px solid var(--line); border-radius: 14px; background: var(--card); box-shadow: var(--shadow); }
  .head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px 16px; padding: 14px 18px; border-bottom: 1px solid var(--line-soft); }
  .count { margin: 0; color: var(--muted); font-size: 0.82rem; font-variant-numeric: tabular-nums; }
  .actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .toolbar { padding: 12px 18px; border-bottom: 1px solid var(--line-soft); }
  .body { padding: 0 18px 18px; }
  @media (max-width: 650px) {
    .head { padding: 12px 14px; }
    .toolbar { padding: 0 14px 12px; }
    .body { padding: 0 14px 14px; }
  }
</style>
