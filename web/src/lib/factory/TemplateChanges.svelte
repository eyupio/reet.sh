<script lang="ts">
  // What changed between two versions of a template, in plain lines. It draws nothing when nothing changed.
  // A pipeline with one change says it on its own line; one with several gets its title and the lines under it.
  import type { TemplateDiff } from '../api'
  import { diffLines } from './templates'

  let { diff, title, note = '' }: { diff: TemplateDiff; title: string; note?: string } = $props()
  const lines = $derived(diffLines(diff))
</script>

{#if diff.changed}<section class="changes" aria-label={title}>
  <h4>{title}</h4>
  {#if note}<p class="note">{note}</p>{/if}
  <ul>
    {#each lines.general as line (line)}<li>{line}</li>{/each}
    {#each lines.pipelines as pipeline (pipeline.title)}
      {#if pipeline.lines.length === 1}<li><strong>{pipeline.title}</strong> · {pipeline.lines[0]}</li>
      {:else}<li><strong>{pipeline.title}</strong><ul>{#each pipeline.lines as line, n (n)}<li>{line}</li>{/each}</ul></li>{/if}
    {/each}
  </ul>
</section>{/if}

<style>
  .changes{display:grid;gap:6px;padding:12px;border:1px solid var(--line);border-radius:12px;background:var(--tint);font-size:14px;line-height:1.5;min-width:0}
  h4{margin:0;font:600 14px/1.5 var(--sans)}p{margin:0}.note{color:var(--ink-soft)}
  ul{list-style:none;margin:0;padding:0;display:grid;gap:2px;color:var(--ink-soft)}li{overflow-wrap:anywhere}li ul{padding-left:12px}strong{color:var(--ink)}
</style>
