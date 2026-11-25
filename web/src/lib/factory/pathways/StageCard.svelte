<script lang="ts">
  import { stageTitle, stageWho, type PathwayStage } from '../pathways'
  import { draggable, type DragHooks } from './drag'

  // One stage on the line: its name, where it comes from, who does it, its skills and prompt, and its first problem.
  // With focus on it, Alt and an arrow along the line moves it and Delete or Backspace removes it. It can be dragged along
  // the line; while a model, skill or prompt is dragged, `aim` says whether it would take the drop and what would change.
  let { stage, at, selected, problem, vertical, drag, aim, onselect, onmove, onremove }: {
    stage: PathwayStage; at: number; selected: boolean; problem: string; vertical: boolean
    drag: DragHooks | null; aim: { take: boolean; note: string } | null
    onselect: () => void; onmove: (to: number) => void; onremove: () => void
  } = $props()

  const own = $derived(stage.kind === 'custom')
  const title = $derived(stageTitle(stage).trim())
  const skills = $derived(stage.skills?.length ?? 0)
  const summary = $derived.by(() => {
    const prompt = !!stage.prompt?.trim()
    if (stage.kind === 'merge' && !skills && !prompt) return 'Always last'
    return [skills ? `${skills} skill${skills === 1 ? '' : 's'}` : 'No skills', prompt ? 'prompt ✓' : ''].filter(Boolean).join(' · ')
  })

  function keys(event: KeyboardEvent) {
    const [back, on] = vertical ? ['ArrowUp', 'ArrowDown'] : ['ArrowLeft', 'ArrowRight']
    if (event.altKey && (event.key === back || event.key === on)) {
      event.preventDefault()
      onmove(at + (event.key === back ? -1 : 1))
    } else if (event.key === 'Delete' || event.key === 'Backspace') {
      event.preventDefault()
      onremove()
    }
  }
</script>

<button type="button" class="card" class:vertical class:bad={!!problem} data-stage-index={at} data-drop={aim ? aim.take ? 'take' : 'refuse' : undefined} aria-pressed={selected}
  onclick={onselect} onkeydown={keys} use:draggable={drag && { item: () => ({ type: 'card', from: at }), ...drag }}>
  <span class="grip" aria-hidden="true">⠿</span>
  <span class="title" class:unnamed={!title}>{title || 'Unnamed stage'}</span>
  <span class="origin">{own ? 'Your own' : stage.kind === 'merge' ? 'Built in · waits for you' : 'Built in'}</span>
  <span class="who">{stageWho(stage)}</span>
  <span class="summary">{#if aim && !aim.take}{aim.note}{:else}{aim ? `${summary} · ` : summary}{#if aim}<span class="preview">{aim.note}</span>{/if}{/if}</span>
  {#if problem}<span class="problem" title={problem}><span class="mark" aria-hidden="true">!</span>{problem}</span>{/if}
</button>

<style>
  .card {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'title grip' 'origin origin' 'who who' 'summary summary' 'problem problem';
    align-content: start; align-self: stretch; gap: 4px; width: 176px; min-height: 132px; box-sizing: border-box; padding: 12px 12px 12px 14px;
    border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--ink);
    font: inherit; text-align: left; cursor: pointer; box-shadow: var(--shadow);
  }
  .card:hover { border-color: var(--line-strong); }
  .card:active { background: var(--card-raised); border-color: var(--muted); }
  .card:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .card[aria-pressed='true'] { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-wash), var(--shadow); }
  .card.bad:not([aria-pressed='true']) { border-color: var(--warn); }
  .grip { grid-area: grip; color: var(--faint); font-size: 14px; line-height: 22px; cursor: grab; }
  /* Picked up: a dashed outline where it was. Under a dragged model, skill or prompt: a ring when it takes the drop, dimmed when it cannot. */
  .card:global([data-dragging]) { opacity: 0.45; border-style: dashed; box-shadow: none; }
  /* While something is dragged only the drop target is ringed; the selected card rests like the others. */
  :global(html[data-pathway-drag]) .card[aria-pressed='true']:not([data-drop='take']) { border-color: var(--line); box-shadow: var(--shadow); }
  .card.card[data-drop='take'] { border-color: var(--accent); box-shadow: 0 0 0 4px var(--accent-wash), var(--shadow); }
  .card[data-drop='refuse'] { opacity: 0.5; }
  .preview { color: var(--accent-text); font-weight: 600; }
  .title { grid-area: title; font-weight: 600; font-size: 15px; line-height: 22px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .title.unnamed { color: var(--muted); font-weight: 400; font-style: italic; }
  .origin { grid-area: origin; color: var(--muted); font-size: 14px; line-height: 20px; }
  .who {
    grid-area: who; justify-self: start; max-width: 100%; box-sizing: border-box; margin: 4px 0 2px; padding: 2px 8px; border-radius: 6px;
    background: var(--bg-sunk); color: var(--ink-soft); font: 14px/20px var(--mono); letter-spacing: -0.03em; overflow-wrap: anywhere;
  }
  .summary { grid-area: summary; color: var(--muted); font-size: 14px; line-height: 20px; }
  .problem { grid-area: problem; color: var(--warn); font-size: 14px; line-height: 20px; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .mark { font-weight: 700; margin-right: 6px; }

  /* Top to bottom on a narrow screen: a full-width row, grip at the left, name over who does it, the summary at the right. */
  .card.vertical {
    width: 100%; min-height: 64px; grid-template-columns: auto minmax(0, 1fr) auto; grid-template-areas: 'grip title summary' 'grip who summary' 'grip problem problem';
    column-gap: 12px; row-gap: 2px; align-items: center; padding: 10px 14px;
  }
  .card.vertical .origin { display: none; }
  .card.vertical .grip { align-self: center; }
  .card.vertical .who { margin: 0; justify-self: start; }
  .card.vertical .summary { text-align: right; white-space: nowrap; }
  @media (prefers-reduced-motion: no-preference) {
    .card { transition: border-color 0.15s, box-shadow 0.15s, background-color 0.15s, opacity 0.15s; }
  }
</style>
