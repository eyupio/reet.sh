<script lang="ts">
  // One collapsible block of a list: a single button header (arrow, name, count, one-line summary) and the rows
  // beneath it while open. The page owns the open state and remembers it with sectionMemory.
  import type { Snippet } from 'svelte'
  let { name, count, summary, open, ontoggle, children }: { name: string; count: number; summary: string; open: boolean; ontoggle: () => void; children?: Snippet } = $props()
</script>

<section class="sec" aria-label={name}>
  <button type="button" class="head" aria-expanded={open} onclick={ontoggle}>
    <span class="arrow" class:open aria-hidden="true"></span>
    <span class="name">{name}</span>
    <span class="count">· {count}</span>
    <span class="summary">{summary}</span>
  </button>
  {#if open}<div class="body">{@render children?.()}</div>{/if}
</section>

<style>
  .sec{display:grid;gap:8px}
  .head{display:flex;align-items:center;gap:8px;width:100%;min-height:36px;padding:6px 4px;border:0;background:none;font:inherit;font-size:14px;color:var(--ink);cursor:pointer;text-align:left;border-bottom:1px solid var(--line-soft)}
  .head:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .arrow{flex:none;width:0;height:0;border-style:solid;border-color:transparent;border-left-color:var(--muted);border-width:5px 0 5px 7px}
  .arrow.open{border-left-color:transparent;border-top-color:var(--muted);border-width:7px 5px 0 5px}
  .name{flex:none;font-weight:700}
  .count{flex:none;color:var(--muted);font-weight:600}
  .summary{flex:1 1 0;min-width:0;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .body{display:grid;gap:8px}
  @media(pointer:coarse){.head{min-height:44px}}
</style>
