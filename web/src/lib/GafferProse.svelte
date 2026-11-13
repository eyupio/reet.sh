<script lang="ts">
  import { inlineSegments, proseBlocks, textSegments } from './chat'
  let { text }: {text:string} = $props()
</script>

{#snippet inline(value:string)}
  {#each inlineSegments(value) as part}
    {#if part.kind==='strong'}<strong>{part.text}</strong>{:else if part.kind==='code'}<code>{part.text}</code>{:else if part.kind==='link'}<a href={part.href} target="_blank" rel="noopener noreferrer">{part.text}</a>{:else}{part.text}{/if}
  {/each}
{/snippet}
<div class="gaffer-prose">
  {#each textSegments(text) as segment}
    {#if segment.kind==='code'}<pre><code>{segment.text}</code></pre>
    {:else}
      {#each proseBlocks(segment.text) as block}
        {#if block.kind==='h'}<h4>{@render inline(block.text)}</h4>
        {:else if block.kind==='ul'}<ul>{#each block.items as item}<li>{@render inline(item)}</li>{/each}</ul>
        {:else if block.kind==='ol'}<ol>{#each block.items as item}<li>{@render inline(item)}</li>{/each}</ol>
        {:else if block.kind==='quote'}<blockquote>{@render inline(block.text)}</blockquote>
        {:else}<p>{@render inline(block.text)}</p>{/if}
      {/each}
    {/if}
  {/each}
</div>
<style>
 .gaffer-prose { font-size:15px;line-height:1.7;overflow-wrap:anywhere;min-width:0; }
 p { margin:0 0 14px;white-space:pre-wrap; }p:last-child { margin-bottom:0; }
 h4 { font:600 18px/1.4 var(--sans);margin:18px 0 10px; }h4:first-child { margin-top:0; }
 ul,ol { margin:10px 0 16px;padding-left:22px; }li { margin:6px 0;white-space:pre-wrap; }
 blockquote { margin:14px 0;padding:0 0 0 14px;border-left:2px solid var(--accent);color:var(--muted);white-space:pre-wrap; }
 code { font:14px/1.6 var(--mono);padding:2px 4px;border-radius:4px;background:var(--bg-sunk); }
 pre { max-width:100%;overflow:auto;margin:14px 0;padding:12px;border:1px solid var(--line);border-radius:9px;background:var(--bg-sunk); }pre code { padding:0;white-space:pre;overflow-wrap:normal; }
 a { color:var(--accent-text);text-decoration:underline;text-underline-offset:3px; }
</style>
