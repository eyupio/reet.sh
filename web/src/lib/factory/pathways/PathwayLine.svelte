<script lang="ts">
  import { tick } from 'svelte'
  import type { PathwayStage, StageProblem } from '../pathways'
  import StageCard from './StageCard.svelte'

  // The pathway's stages in order, joined by arrows, with a + at the end. Sideways from 700 px wide, top to bottom below.
  // `onmove` answers whether the stage moved, so focus can follow a card moved from the keyboard.
  let { stages, selected, problems, vertical, onselect, onmove, onremove, onaddend }: {
    stages: PathwayStage[]; selected: number; problems: StageProblem[]; vertical: boolean
    onselect: (at: number) => void; onmove: (from: number, to: number) => boolean; onremove: (at: number) => void; onaddend: () => void
  } = $props()

  let list = $state<HTMLElement>()
  const cardAt = (at: number) => list?.querySelector<HTMLElement>(`[data-stage-index="${at}"]`)
  // The selected card is brought into view, in the line's own scroll and the page's.
  $effect(() => { const at = selected; if (at >= 0) void tick().then(() => cardAt(at)?.scrollIntoView({ block: 'nearest', inline: 'nearest' })) })

  async function keyMove(from: number, to: number) {
    if (to < 0 || to >= stages.length || !onmove(from, to)) return
    await tick()
    cardAt(to)?.focus()
  }
  async function keyRemove(at: number) {
    onremove(at)
    await tick()
    ;(cardAt(Math.min(at, stages.length - 1)) ?? list?.querySelector<HTMLElement>('.add'))?.focus()
  }
</script>

<div class="line" class:vertical data-pathway-line bind:this={list}>
  <div class="run">
    {#if stages.length}
      <ol aria-label="Stages">
        {#each stages as stage, at (at)}
          <li>
            {#if at > 0}<span class="join" aria-hidden="true"></span>{/if}
            <StageCard {stage} {at} {vertical} selected={selected === at} problem={problems.find(p => p.at === at)?.text ?? ''}
              onselect={() => onselect(at)} onmove={to => void keyMove(at, to)} onremove={() => void keyRemove(at)} />
          </li>
        {/each}
      </ol>
      <span class="join" aria-hidden="true"></span>
    {:else}
      <div class="empty">
        <p class="lead">No stages yet.</p>
        <p>Add one from the {vertical ? 'list' : 'palette'}, or press +.</p>
      </div>
    {/if}
    <button type="button" class="add" aria-label="Add a stage" title="Add a stage" onclick={onaddend}><span aria-hidden="true">+</span>{#if vertical}<span class="words" aria-hidden="true">Add a stage</span>{/if}</button>
  </div>
</div>

<style>
  /* The line scrolls sideways inside the canvas; the run is centred while it fits and starts at the left once it does not. */
  .line { display: flex; overflow-x: auto; overscroll-behavior-x: contain; padding: 40px 32px; scroll-padding-inline: 32px; }
  .run { display: flex; align-items: center; margin: auto; width: max-content; }
  ol { display: flex; align-items: stretch; margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: center; }
  /* A thin arrow between cards: a rule with a head. */
  .join { position: relative; flex: none; width: 28px; height: 1px; margin: 0 6px; background: var(--line-deep); }
  .join::after { content: ''; position: absolute; right: -1px; top: -4px; border: 4.5px solid transparent; border-right: 0; border-left: 6px solid var(--line-deep); }
  .add {
    display: inline-flex; align-items: center; justify-content: center; gap: 8px; flex: none; width: 48px; height: 48px; padding: 0;
    border: 1.5px dashed var(--line-deep); border-radius: 50%; background: var(--card); color: var(--muted); font: inherit; font-size: 22px; line-height: 1; cursor: pointer;
  }
  .add:hover { border-color: var(--accent); color: var(--accent-text); }
  .add:active { background: var(--tint); }
  .add:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .empty { display: grid; gap: 4px; margin-right: 20px; padding: 18px 22px; border: 1.5px dashed var(--line-strong); border-radius: 12px; background: var(--card); color: var(--muted); text-align: center; }
  .empty p { margin: 0; font-size: 14px; }
  .empty .lead { color: var(--ink); font-weight: 600; font-size: 15px; }

  /* Top to bottom: full-width rows joined by short connectors, ending in a dashed Add a stage. */
  .vertical { display: block; overflow: visible; padding: 16px; }
  .vertical .run { display: block; width: auto; margin: 0; }
  .vertical ol, .vertical li { display: block; }
  .vertical .join { display: block; width: 1px; height: 14px; margin: 0 0 0 30px; }
  .vertical .add { display: flex; }
  .vertical .join::after { display: none; }
  .vertical .empty { margin: 0 0 14px; }
  .vertical .add { width: 100%; height: 52px; border-radius: 12px; font-size: 15px; font-weight: 600; }
  .vertical .add span[aria-hidden]:first-child { font-size: 20px; }
  @media (prefers-reduced-motion: no-preference) { .add { transition: border-color 0.15s, color 0.15s, background-color 0.15s; } }
</style>
