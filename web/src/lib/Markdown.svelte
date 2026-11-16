<script lang="ts">
  import { copyText } from './clipboard'
  import { markdownBlocks, type MdBlock, type MdSpan } from './markdown'

  /**
   * An agent's markdown as real elements. `onlink` takes over a link (to ask before leaving reet); without it a link
   * opens in a new tab. `copy` puts a language label and a Copy button on each code block.
   */
  let { text, onlink, copy = false, oncopyfail }: { text: string; onlink?: (href: string) => void; copy?: boolean; oncopyfail?: () => void } = $props()
  const parsed = $derived(markdownBlocks(text))
  let copied = $state.raw<MdBlock | null>(null), copyTimer: ReturnType<typeof setTimeout> | undefined
  async function copyBlock(block: MdBlock & { kind: 'code' }) {
    if (!(await copyText(block.text))) { oncopyfail?.(); return }
    copied = block
    clearTimeout(copyTimer)
    copyTimer = setTimeout(() => { copied = null }, 1500)
  }
  $effect(() => () => clearTimeout(copyTimer))
</script>

{#snippet inline(spans: MdSpan[])}{#each spans as span, n (n)}{#if span.kind === 'text'}{span.text}{:else if span.kind === 'code'}<code>{span.text}</code>{:else if span.kind === 'strong'}<strong>{@render inline(span.spans)}</strong>{:else if span.kind === 'em'}<em>{@render inline(span.spans)}</em>{:else if span.kind === 'del'}<del>{@render inline(span.spans)}</del>{:else if span.kind === 'br'}<br />{:else if span.kind === 'link'}{#if onlink}<button type="button" class="prose-link" title={span.href} onclick={() => onlink(span.href)} onauxclick={(event) => { if (event.button === 1) { event.preventDefault(); onlink(span.href) } }}>{@render inline(span.spans)}</button>{:else}<a class="prose-link" href={span.href} target="_blank" rel="noopener noreferrer">{@render inline(span.spans)}</a>{/if}{/if}{/each}{/snippet}

{#snippet render(list: MdBlock[])}
  {#each list as block, i (i)}
    {#if block.kind === 'p'}<p class="prose">{@render inline(block.spans)}</p>
    {:else if block.kind === 'text'}{@render inline(block.spans)}
    {:else if block.kind === 'h'}<svelte:element this={`h${Math.min(6, block.level + 3)}`} class="prose-h" data-level={block.level}>{@render inline(block.spans)}</svelte:element>
    {:else if block.kind === 'code'}
      <div class="code">
        {#if copy}<div class="code-head"><span>{block.lang || 'code'}</span><button type="button" class="copy" onclick={() => void copyBlock(block)}>{copied === block ? 'Copied' : 'Copy'}</button></div>{/if}
        <pre><code>{block.text}</code></pre>
      </div>
    {:else if block.kind === 'quote'}<blockquote class="prose-quote">{@render render(block.blocks)}</blockquote>
    {:else if block.kind === 'rule'}<hr class="prose-rule" />
    {:else if block.kind === 'list'}
      <svelte:element this={block.ordered ? 'ol' : 'ul'} class="prose-list" start={block.ordered && block.start !== 1 ? block.start : undefined}>
        {#each block.items as item, k (k)}<li class:task={item.checked !== null}>{#if item.checked !== null}<input type="checkbox" checked={item.checked} disabled aria-label={item.checked ? 'Done' : 'Not done'} />{/if}{@render render(item.blocks)}</li>{/each}
      </svelte:element>
    {:else if block.kind === 'table'}
      <div class="prose-table"><table>
        {#if block.head}<thead><tr>{#each block.head as cell, c (c)}<th scope="col" style:text-align={block.align[c]}>{@render inline(cell)}</th>{/each}</tr></thead>{/if}
        <tbody>{#each block.rows as row, r (r)}<tr>{#each row as cell, c (c)}<td style:text-align={block.align[c]}>{@render inline(cell)}</td>{/each}</tr>{/each}</tbody>
      </table></div>
    {/if}
  {/each}
{/snippet}

<div class="md">{@render render(parsed)}</div>

<style>
 /* Spacing follows --md-gap so a roomier surface can loosen the same markup. */
 .md { min-width: 0; overflow-wrap: anywhere; }
 .md > :global(:first-child) { margin-top: 0; }
 .md > :global(:last-child) { margin-bottom: 0; }
 .prose { margin: 0 0 var(--md-gap, 8px); white-space: pre-wrap; }
 .prose-h { margin: calc(var(--md-gap, 8px) + 6px) 0 6px; font: 600 1.02rem/1.35 var(--sans); color: var(--ink); }
 .prose-h[data-level='1'] { font-size: 1.2rem; }
 .prose-h[data-level='2'] { font-size: 1.1rem; }
 .prose-h[data-level='5'], .prose-h[data-level='6'] { font-size: 1rem; color: var(--muted); }
 .prose-list { margin: 0 0 var(--md-gap, 8px); padding-left: 1.35em; }
 .prose-list li { margin: 2px 0; }
 .prose-list li::marker { color: var(--faint); }
 .prose-list li.task { list-style: none; }
 .prose-list li.task > input { width: 1em; margin: 0 0.35em 0 -1.35em; vertical-align: -1px; accent-color: var(--accent); }
 li > .prose-list, li > .prose, li > .code, li > .prose-table, li > .prose-quote { margin: 4px 0; }
 .prose-quote { margin: 0 0 var(--md-gap, 8px); padding: 2px 0 2px 12px; border-left: 2px solid var(--line-strong); color: var(--muted); }
 .prose-quote > :global(:last-child) { margin-bottom: 0; }
 .prose-rule { margin: calc(var(--md-gap, 8px) + 4px) 0; border: 0; border-top: 1px solid var(--line); }
 .prose-table { max-width: 100%; margin: 4px 0 calc(var(--md-gap, 8px) + 2px); overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; }
 table { width: 100%; border-collapse: collapse; }
 th, td { padding: 6px 10px; border-bottom: 1px solid var(--line-soft); text-align: left; vertical-align: top; }
 th { background: var(--bg-sunk); font-weight: 600; color: var(--ink); }
 tbody tr:last-child td { border-bottom: 0; }
 :not(pre) > code { padding: 1px 4px; border: 1px solid var(--line-soft); border-radius: 4px; background: var(--bg-sunk); font: 14px var(--mono); color: var(--ink); }
 strong { font-weight: 600; color: var(--ink); }
 del { color: var(--muted); }
 .prose-link { color: var(--accent-text); text-decoration: underline; text-underline-offset: 2px; }
 button.prose-link { padding: 0; border: 0; background: none; cursor: pointer; font: inherit; text-align: left; }
 .prose-link:hover { color: var(--ink); }
 .prose-link:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; border-radius: 2px; }
 .code { margin: var(--md-gap, 8px) 0; border: 1px solid var(--line); border-radius: 8px; background: var(--bg-sunk); overflow: hidden; }
 .code-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 4px 8px 4px 12px; border-bottom: 1px solid var(--line-soft); font: 600 14px var(--sans); letter-spacing: 0.01em; color: var(--faint); }
 .code pre { margin: 0; padding: 8px 12px; max-height: 24rem; overflow: auto; font: 14px/1.55 var(--mono); color: var(--ink-soft); white-space: pre; overflow-wrap: normal; }
 .code pre code { font: inherit; }
 .copy { padding: 4px 8px; border: 1px solid transparent; border-radius: 6px; background: none; color: var(--faint); font: 500 14px var(--sans); letter-spacing: 0.01em; cursor: pointer; }
 .copy:hover { border-color: var(--line); color: var(--ink); }
 .copy:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 8px; }
 @media (pointer: coarse) { .copy { min-height: 32px; padding: 0 10px; } }
</style>
