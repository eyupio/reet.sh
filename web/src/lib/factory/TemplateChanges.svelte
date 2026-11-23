<script lang="ts">
  // What changed between two versions of a template, in plain lines. It draws nothing when nothing changed.
  // A pipeline with one change says it on its own line; one with several gets its title and the lines under it.
  import type { TemplateDiff } from '../api'
  import { diffLines, OTHER_CHANGE } from './templates'

  let { diff, title, note = '', oncompare }: { diff: TemplateDiff; title: string; note?: string; oncompare?: () => void } = $props()
  const lines = $derived(diffLines(diff))
</script>

{#snippet line(text: string)}{text}{#if text === OTHER_CHANGE && oncompare}{' '}<button class="inline" type="button" onclick={oncompare}>Compare the versions</button>{/if}{/snippet}

{#if diff.changed}<section class="changes" aria-label={title}>
  <h4>{title}</h4>
  {#if note}<p class="note">{note}</p>{/if}
  <ul>
    {#each lines.general as text (text)}<li>{text}</li>{/each}
    {#each lines.pipelines as pipeline (pipeline.title)}
      {#if pipeline.lines.length === 1}<li><strong>{pipeline.title}</strong> · {@render line(pipeline.lines[0])}</li>
      {:else}<li><strong>{pipeline.title}</strong><ul>{#each pipeline.lines as text, n (n)}<li>{@render line(text)}</li>{/each}</ul></li>{/if}
    {/each}
  </ul>
</section>{/if}

<style>
  .changes{display:grid;gap:6px;padding:12px;border:1px solid var(--line);border-radius:12px;background:var(--tint);font-size:14px;line-height:1.5;min-width:0}
  h4{margin:0;font:600 14px/1.5 var(--sans)}p{margin:0}.note{color:var(--ink-soft)}
  ul{list-style:none;margin:0;padding:0;display:grid;gap:2px;color:var(--ink-soft)}li{overflow-wrap:anywhere}li ul{padding-left:12px}strong{color:var(--ink)}
  .inline{border:0;background:none;padding:0;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;text-decoration:underline;min-height:24px}.inline:focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.inline{min-height:44px}}
</style>
