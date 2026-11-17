<script lang="ts">
  import type { TrackNode } from '../gafferOutcome'
  import { stageCaption } from './factoryList'

  // A pipeline's stages as a row of circles joined by a line: done ones filled with a tick, the stage in hand ringed
  // (or filled in the accent colour when it waits on the person), the rest to come. Three sizes share one look.
  let { nodes, at, size, waiting = false, selected, onselect, label }: {
    nodes: TrackNode[]
    at: number
    size: 'labelled' | 'circles' | 'dots'
    waiting?: boolean
    selected?: number
    onselect?: (index: number) => void
    label: string
  } = $props()

  // The stage in hand: the one at `at`, unless it is done or has failed. Its look, its words and aria-current all ask this.
  const inHand = (node: TrackNode, index: number): boolean => index === at && node.state !== 'done' && node.state !== 'failed'
  const look = (node: TrackNode, index: number): TrackNode['state'] | 'here' | 'waiting' => inHand(node, index) ? (waiting ? 'waiting' : 'here') : node.state
  const symbol = (node: TrackNode, index: number): string => node.moved ? '»' : node.state === 'done' ? '✓' : node.state === 'failed' ? '×' : inHand(node, index) && waiting ? '!' : String(index + 1)
  const said = (node: TrackNode, index: number): string => node.moved ? 'moved on by you, not handed over by Gaffer' : node.state === 'done' ? 'done' : node.state === 'failed' ? 'stopped' : inHand(node, index) ? (waiting ? 'waiting for you' : 'in hand') : 'to come'
  const caption = $derived(size === 'labelled' ? '' : stageCaption(nodes, at, size))
  // A labelled line too long for its box scrolls sideways inside it, never the page, and starts with the stage in hand in view.
  let box = $state<HTMLDivElement>()
  $effect(() => {
    if (size !== 'labelled' || !box || at < 0) return
    const here = box.querySelectorAll('li')[at]
    if (here && box.scrollWidth > box.clientWidth) box.scrollLeft = Math.max(0, here.offsetLeft - (box.clientWidth - here.offsetWidth) / 2)
  })
</script>

{#if nodes.length}
  <div class="stages {size}" bind:this={box}>
    <ol aria-label={label} style:grid-template-columns={size === 'labelled' ? `repeat(${nodes.length}, minmax(min-content, 1fr))` : size === 'dots' ? undefined : `repeat(${nodes.length}, minmax(0, 1fr))`}>
      {#each nodes as node, index (node.key)}
        {#snippet mark()}<span class="dot" class:moved={node.moved} aria-hidden="true">{size === 'dots' ? '' : symbol(node, index)}</span>{/snippet}
        {#snippet words()}{#if size === 'labelled'}<span class="name">{node.label}</span>{/if}<span class="sr-only">{size === 'labelled' ? '' : `${node.label}: `}{said(node, index)}</span>{/snippet}
        <li class={look(node, index)} class:chosen={selected === index} aria-current={inHand(node, index) ? 'step' : undefined}>
          {#if onselect && size === 'labelled'}
            <button type="button" aria-pressed={selected === index} onclick={() => onselect(index)}>{@render mark()}{@render words()}</button>
          {:else}
            <span class="cell">{@render mark()}{@render words()}</span>
          {/if}
        </li>
      {/each}
    </ol>
    {#if caption}<p class="caption">{caption}</p>{/if}
  </div>
{/if}

<style>
  .stages{position:relative;min-width:0}
  /* Named stages keep each word whole: a line wider than its box scrolls inside the box. */
  .labelled{overflow-x:auto;overflow-y:hidden;scrollbar-width:thin;padding:2px 2px 4px;margin:-2px -2px -4px}
  /* A stage is at least as wide as its longest word: its button and name keep their automatic minimum width (a zero
     minimum or a percentage width would let a column shrink below a word and break it), and the grid stretches them. */
  .labelled li{min-width:60px;display:grid}
  .labelled .cell,.labelled button,.labelled .name{min-width:auto}
  .labelled button{width:auto;justify-self:stretch}
  .sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
  ol{list-style:none;margin:0;padding:2px 0 0;display:grid;gap:0;min-width:0}
  li{position:relative;min-width:0;color:var(--muted);font-size:0.875rem;text-align:center}
  /* The line runs from the middle of the circle before to the middle of this one, behind both. */
  li+li::before{content:"";position:absolute;top:calc(var(--size) / 2 - 1px);right:50%;width:100%;height:2px;background:var(--line);z-index:0}
  li.done+li::before{background:var(--good)}
  .cell,button{position:relative;z-index:1;display:grid;justify-items:center;align-content:start;gap:4px;width:100%;min-width:0;padding:0 4px 2px;box-sizing:border-box;border:0;background:none;color:inherit;font:inherit;text-align:center}
  button{cursor:pointer;border-radius:8px}
  button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .name{min-width:0;max-width:100%;overflow-wrap:break-word;text-align:center}
  .dot{position:relative;z-index:1;display:inline-grid;place-items:center;width:var(--size);height:var(--size);box-sizing:border-box;border-radius:50%;border:2px solid var(--line-strong);background:var(--card);font-size:0.875rem;font-weight:600;font-variant-numeric:tabular-nums}
  .labelled{--size:28px}
  .circles{--size:24px}
  .dots{--size:12px}
  /* Dots run from the left edge to the right edge, under the item's title and caption: the first dot starts the line
     and every later one ends its own stretch of it. */
  .dots ol{display:flex}
  .dots li{flex:1 1 0}.dots li:first-child{flex:0 0 var(--size)}
  .dots li+li::before{left:calc(var(--size) / -2);right:calc(var(--size) / 2);width:auto}
  .dots .cell{justify-items:end;padding:0 0 2px}
  li.done{color:var(--ink)}li.done .dot{background:var(--good);border-color:var(--good);color:var(--bg)}li.done .dot.moved{background:var(--card);color:var(--good)}
  li.here,li.running{color:var(--ink);font-weight:600}li.here .dot,li.running .dot{border-color:var(--good);box-shadow:0 0 0 2px var(--good);color:var(--good)}
  li.waiting{color:var(--ink);font-weight:600}li.waiting .dot{border-color:var(--accent);background:var(--accent);color:var(--accent-ink)}
  li.failed{color:var(--bad)}li.failed .dot{border-color:var(--bad);color:var(--bad)}
  li.chosen .name{text-decoration:underline;text-underline-offset:3px}
  .caption{margin:6px 0 0;color:var(--muted);font-size:0.875rem;line-height:1.35;overflow-wrap:anywhere}
  @media (pointer: coarse){button{min-height:44px}}
</style>
