<script lang="ts">
  // What changed between two versions of a template, in plain lines. It draws nothing when nothing changed.
  import type { TemplateDiff } from '../api'
  import { diffLines } from './templates'

  let { diff, title }: { diff: TemplateDiff; title: string } = $props()
  const lines = $derived(diffLines(diff))
</script>

{#if diff.changed}<section class="changes" aria-label={title}>
  <p class="head"><strong>{title}</strong></p>
  {#if lines.general.length}<ul>{#each lines.general as line (line)}<li><span aria-hidden="true">·</span> {line}</li>{/each}</ul>{/if}
  {#each lines.pipelines as pipeline (pipeline.title)}
    <p class="pipe">{pipeline.title}</p>
    <ul>{#each pipeline.lines as line, n (n)}<li><span aria-hidden="true">·</span> {line}</li>{/each}</ul>
  {/each}
</section>{/if}

<style>
  .changes{display:grid;gap:4px;padding:12px;border:1px solid var(--line);border-radius:12px;background:var(--tint);font-size:14px;line-height:1.5;min-width:0}
  p{margin:0}.pipe{font-weight:600;margin-top:4px;overflow-wrap:anywhere}
  ul{list-style:none;margin:0;padding:0;display:grid;gap:2px;color:var(--ink-soft)}li{overflow-wrap:anywhere}
</style>
