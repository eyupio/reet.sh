<script lang="ts">
  // The one loading affordance for content. A spinner belongs on a button that is doing something; a page or a section
  // that is waiting for data keeps the shape of what is coming, so nothing jumps when it arrives.
  //
  // `shape` is that shape: `lines` for prose and short facts, `rows` for a list, `cards` for a grid of cards. `label` is
  // read out once by a screen reader, in place of the bars it cannot see.
  let {
    label = 'Loading…',
    shape = 'lines',
    count = 3,
    height,
    class: className = '',
  }: {
    label?: string
    shape?: 'lines' | 'rows' | 'cards'
    /** How many lines, rows or cards. */
    count?: number
    /** For `cards`: the card's height, so the skeleton is as tall as the real thing. */
    height?: string
    class?: string
  } = $props()
  const items = $derived(Array.from({ length: Math.max(1, count) }, (_, i) => i))
</script>

<div class="skeleton {shape} {className}" role="status" aria-busy="true" aria-label={label}>
  <span class="vh">{label}</span>
  {#if shape === 'cards'}
    <div class="grid" aria-hidden="true">
      {#each items as i (i)}<div class="card" data-skeleton style:height={height}><i class="title"></i><i></i><i class="short"></i></div>{/each}
    </div>
  {:else if shape === 'rows'}
    <div class="list" aria-hidden="true">
      {#each items as i (i)}<div class="row" data-skeleton><i class="lead"></i><i></i><i class="tail"></i></div>{/each}
    </div>
  {:else}
    <div class="lines" aria-hidden="true">
      {#each items as i (i)}<i data-skeleton class:short={i === items.length - 1 && items.length > 1}></i>{/each}
    </div>
  {/if}
</div>

<style>
  .skeleton { display: block; }
  i { display: block; height: 12px; border-radius: 6px; background: var(--line); }
  .lines { display: grid; gap: 10px; }
  .lines i.short { width: 60%; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(240px, 100%), 1fr)); gap: 14px; }
  .card { display: grid; align-content: start; gap: 14px; min-height: 112px; padding: 20px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .card i.title { width: 40%; height: 18px; }
  .card i.short { width: 70%; }
  .list { display: grid; gap: 1px; overflow: hidden; border: 1px solid var(--line); border-radius: var(--radius); background: var(--line); }
  .row { display: flex; align-items: center; gap: 16px; padding: 18px 20px; background: var(--card); }
  .row i { flex: 1; height: 10px; }
  .row i.lead { flex: 0 0 30%; height: 13px; }
  .row i.tail { flex: 0 0 56px; }
  @media (prefers-reduced-motion: no-preference) {
    i { background: linear-gradient(90deg, var(--line) 0%, var(--line-soft) 50%, var(--line) 100%); background-size: 200% 100%; animation: skeleton-shimmer 1.8s ease-in-out infinite; }
    @keyframes skeleton-shimmer { to { background-position: -200% 0; } }
  }
</style>
