<script module lang="ts">
  import { stageTitle, type Pathway, type PathwayStage } from '../pathways'
  export { stageLine }
</script>

<script lang="ts">
  // Which pathway new work follows, or the standard stages ('') the dialog lets a person choose themselves. Only enabled
  // pathways are offered; one that cannot start now is shown with why, and cannot be chosen. With none, nothing is shown.
  let { pathways, value = $bindable(''), suggested }: { pathways: Pathway[]; value: string; suggested?: { id: string; why: string } } = $props()
  const id = $props.id()
  const offered = $derived(pathways.filter(p => p.enabled))
</script>

<!-- A pathway's stages in order, read only. Stages of the person's own stand out. -->
{#snippet stageLine(name: string, stages: PathwayStage[])}
  <span class="line" role="list" aria-label={`Stages of ${name}`}>
    {#each stages as stage, i (i)}<span class="step" role="listitem"><span class="chip" class:own={stage.kind === 'custom'}>{stageTitle(stage)}</span>{#if i < stages.length - 1}<span class="sep" aria-hidden="true">›</span>{/if}</span>{/each}
  </span>
{/snippet}

{#if offered.length}
  <div class="pathways">
    <span class="label" id={`${id}-label`}>Pathway</span>
    <div class="rows" role="radiogroup" aria-labelledby={`${id}-label`}>
      {#each offered as p (p.id)}{@const problem = p.problems?.[0]}
        <label class="row" class:on={value === p.id} class:off={!!problem}>
          <input type="radio" name={id} value={p.id} bind:group={value} disabled={!!problem}/>
          <span class="text">
            <strong>{p.name}</strong>
            {#if suggested?.id === p.id}<span class="why">Gaffer suggests this: {suggested.why}</span>{/if}
            {@render stageLine(p.name, p.stages)}
            {#if problem}<span class="problem"><span aria-hidden="true">!</span> {problem}</span>{/if}
          </span>
        </label>
      {/each}
      <label class="row" class:on={value === ''}>
        <input type="radio" name={id} value="" bind:group={value}/>
        <span class="text"><strong>Standard stages</strong><span class="why">Choose the stages, tiers and parts yourself, as now.</span></span>
      </label>
    </div>
    {#if value}<a class="edit" href={`/factory/pathways/${encodeURIComponent(value)}`} target="_blank" rel="noopener">Edit pathway</a>{/if}
  </div>
{/if}

<style>
  .pathways { display: grid; gap: 6px; min-width: 0; }
  .label { font-size: 14px; color: var(--muted); }
  .rows { display: grid; gap: 8px; min-width: 0; }
  .row { display: grid; grid-template-columns: 20px minmax(0, 1fr); gap: 10px; align-items: start; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); font-weight: 400; cursor: pointer; min-width: 0; }
  .row.on { border-color: var(--accent); background: var(--accent-wash); }
  .row.off { cursor: not-allowed; }
  .row.off strong, .row.off .line { opacity: 0.6; }
  .row input { width: 18px; height: 18px; min-height: 0; margin: 2px 0 0; padding: 0; accent-color: var(--accent); }
  .row:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
  .text { display: grid; gap: 4px; min-width: 0; }
  .text strong { font-weight: 600; color: var(--ink); overflow-wrap: anywhere; }
  .why { font-size: 14px; color: var(--muted); }
  .problem { font-size: 14px; color: var(--warn); overflow-wrap: anywhere; }
  .problem span { font-weight: 700; }
  .line { display: flex; flex-wrap: wrap; align-items: center; row-gap: 6px; min-width: 0; }
  .step { display: flex; align-items: center; }
  .sep { padding: 0 6px; color: var(--faint); }
  .chip { padding: 1px 10px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--ink-soft); font-size: 14px; font-weight: 400; line-height: 20px; white-space: nowrap; }
  .chip.own { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); background: var(--accent-wash); color: var(--accent-text); }
  .edit { justify-self: start; display: inline-flex; align-items: center; min-height: 44px; font-size: 14px; font-weight: 600; }
  .edit:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
