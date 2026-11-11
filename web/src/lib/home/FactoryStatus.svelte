<script lang="ts">
  // Home's view of the Factory: every pipeline as one line, with what it is part of, where it stands and why. Each line
  // opens that pipeline's card on the Factory page. It reads nothing itself.
  import type { FactoryRow } from '../gafferOutcome'
  let { rows }: { rows: FactoryRow[] } = $props()
  let all = $state(false)
  const shown = $derived(all ? rows : rows.slice(0, 5))
</script>

<ul class="rows" aria-label="Factory pipelines">
  {#each shown as row (row.id)}<li><a href={row.href} aria-label={`${row.title}: ${row.label}. Open it on the Factory page`}>
    <span class="badge {row.tone}">{row.label}</span>
    <span class="what"><strong>{row.title}</strong>{#if row.stage}<span class="stage">{row.stage}</span>{/if}</span>
    <span class="for">For: {row.objective}</span>
    {#if row.why}<span class="why">{row.why}</span>{/if}
  </a></li>{/each}
</ul>
{#if rows.length > 5}<button type="button" class="more" aria-expanded={all} onclick={() => all = !all}>{all ? 'Show fewer' : `Show all ${rows.length}`}</button>{/if}

<style>
  .rows{list-style:none;margin:0;padding:0;display:grid;gap:8px}
  a{display:grid;grid-template-columns:auto minmax(0,1fr);column-gap:12px;row-gap:2px;align-items:center;padding:10px 12px;min-height:44px;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--ink);text-decoration:none}
  a:hover{border-color:var(--line-strong)}a:focus-visible,.more:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .badge{grid-row:1;align-self:start;padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:.8125rem;font-weight:600;white-space:nowrap}
  .badge.you{border-color:var(--accent);color:var(--accent-text)}.badge.bad{border-color:var(--bad);color:var(--bad)}.badge.good{color:var(--muted)}
  .what{display:flex;flex-wrap:wrap;gap:4px 10px;align-items:baseline;min-width:0}.what strong{overflow-wrap:anywhere}.stage{color:var(--muted);font-size:.875rem;white-space:nowrap}
  .for,.why{grid-column:2;color:var(--muted);font-size:.875rem;line-height:1.4;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:1;line-clamp:1;overflow:hidden;min-width:0}
  .why{color:var(--ink);-webkit-line-clamp:2;line-clamp:2}
  .more{margin-top:8px;min-height:44px;padding:0 12px;border:1px solid var(--line);border-radius:8px;background:none;color:var(--ink);font:inherit;cursor:pointer}
  @media(max-width:560px){a{grid-template-columns:minmax(0,1fr)}.badge{justify-self:start}.for,.why{grid-column:1}}
</style>
