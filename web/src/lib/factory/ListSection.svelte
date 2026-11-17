<script lang="ts">
  // One collapsible block of a list: a single button header (arrow, name, count, one-line summary) and the rows
  // beneath it while open. The page owns the open state and remembers it with sectionMemory. `count` can be words
  // ("2 of 3 connected"). `region` false leaves the block unnamed, for content that is a named region of its own.
  import type { Snippet } from 'svelte'
  let { name, count, summary, open, ontoggle, region = true, children }: { name: string; count: number | string; summary: string; open: boolean; ontoggle: () => void; region?: boolean; children?: Snippet } = $props()
</script>

<svelte:element this={region ? 'section' : 'div'} class="sec" aria-label={region ? name : undefined}>
  <!-- A heading of the page's outline (the page's title is its h1); the button inside it opens and closes the block. -->
  <h2 class="heading"><button type="button" class="head" aria-expanded={open} onclick={ontoggle}>
    <span class="arrow" class:open aria-hidden="true"></span>
    <span class="name">{name}</span>
    <span class="count">· {count}</span>
    <span class="summary">{summary}</span>
  </button></h2>
  {#if open}<div class="body">{@render children?.()}</div>{/if}
</svelte:element>

<style>
  .sec{display:grid;grid-template-columns:minmax(0,1fr);gap:8px}
  .heading{margin:0;font:inherit;min-width:0}
  .head{display:flex;align-items:center;gap:8px;width:100%;min-height:36px;padding:6px 4px;border:0;background:none;font:inherit;font-size:14px;color:var(--ink);cursor:pointer;text-align:left;border-bottom:1px solid var(--line-soft)}
  .head:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .arrow{flex:none;width:0;height:0;border-style:solid;border-color:transparent;border-left-color:var(--muted);border-width:5px 0 5px 7px}
  .arrow.open{border-left-color:transparent;border-top-color:var(--muted);border-width:7px 5px 0 5px}
  .name{flex:none;font-weight:700}
  .count{flex:none;color:var(--muted);font-weight:600}
  .summary{flex:1 1 0;min-width:0;text-align:right;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .body{display:grid;grid-template-columns:minmax(0,1fr);gap:8px}
  @media(pointer:coarse){.head{min-height:44px}}
  /* On a phone the summary takes a line of its own under the name, so a closed section still says what is inside. */
  @media(max-width:599px){.head{flex-wrap:wrap;row-gap:2px}.summary{flex:1 1 100%;padding-left:15px;text-align:left;white-space:normal}.summary:empty{display:none}}
</style>
