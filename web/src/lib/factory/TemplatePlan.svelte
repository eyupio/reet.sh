<script lang="ts">
  // A saved plan, read only: each pipeline with its prompt, stages, criteria and the skills each stage was approved with.
  // A skill that is no longer approved at that revision says so on its stage.
  import type { TemplateContent, TemplateSkillGap } from '../api'
  import { stageLabel, tierLabel } from '../gafferOutcome'
  import { gapWords, limitsWords, mergeWords } from './templates'

  let { content, gaps = [] }: { content: TemplateContent; gaps?: TemplateSkillGap[] } = $props()
  const gapFor = (pipeline: number, stage: number, id: string) => gaps.find(gap => gap.pipeline === pipeline && gap.stage === stage && gap.id === id)
</script>

<p class="limits">{limitsWords(content)}</p>
<ol class="plan" aria-label="The plan">
  {#each content.pipelines as pipeline, p (p)}<li>
    <h3>{pipeline.title}</h3>
    <details><summary>Prompt</summary><p class="prompt">{pipeline.prompt}</p></details>
    <ul class="stages" aria-label={`Stages of ${pipeline.title}`}>
      {#each pipeline.stages as stage, s (s)}<li>
        <span class="name">{stageLabel(stage.kind)}{stage.part ? ` · ${stage.part}` : ''}</span>
        <span class="tier">{tierLabel(stage.tier)}</span>
        {#if stage.brief}<span class="brief">{stage.brief}</span>{/if}
        {#each stage.skills as skill (skill.id)}{@const gap = gapFor(p, s, skill.id)}
          <span class="skill">Skill {skill.name} · revision {skill.revision.slice(0, 8)}</span>
          {#if gap}<span class="gap" class:gone={gap.status === 'unavailable'}><span class="mark" aria-hidden="true">!</span> {gapWords(gap)}</span>{/if}
        {/each}
      </li>{/each}
    </ul>
    {#if pipeline.criteria.length}
      <p class="label">{pipeline.criteria.length === 1 ? 'Criterion' : 'Criteria'}</p>
      <ul class="criteria" aria-label={`Criteria of ${pipeline.title}`}>{#each pipeline.criteria as criterion (criterion.id)}<li><span aria-hidden="true">·</span> {criterion.text}</li>{/each}</ul>
    {:else}<p class="hint">No criteria, so this pipeline has no delivery review.</p>{/if}
    <p class="merge" class:auto={pipeline.autoMerge}>{mergeWords(pipeline.autoMerge)}</p>
  </li>{/each}
</ol>

<style>
  .limits{margin:0;color:var(--muted);font-size:14px;line-height:1.5}
  .plan{list-style:none;margin:0;padding:0;display:grid;gap:12px;font-size:14px;line-height:1.5}
  .plan>li{display:grid;gap:8px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}
  h3{margin:0;font:600 1rem/1.3 var(--sans);overflow-wrap:anywhere}
  summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;color:var(--accent-text);font-weight:600}
  .prompt{margin:0;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--ink-soft)}
  .stages,.criteria{list-style:none;margin:0;padding:0;display:grid;gap:0}.criteria{gap:2px}
  .stages>li{display:grid;grid-template-columns:minmax(6rem,auto) 5rem minmax(0,1fr);gap:2px 10px;align-items:baseline;padding:8px 0;border-top:1px solid var(--line-soft)}
  .name{font-weight:600;overflow-wrap:anywhere}.tier{color:var(--muted)}
  .brief,.skill,.gap{grid-column:1/-1;color:var(--muted);overflow-wrap:anywhere}
  .gap{color:var(--ink)}.gap .mark{font-weight:700;color:var(--warn)}.gap.gone .mark{color:var(--bad)}
  .label{margin:0;font-weight:600}.merge{margin:0;color:var(--muted)}.merge.auto{color:var(--ink);font-weight:600}.hint{margin:0;color:var(--muted)}
  .criteria li{overflow-wrap:anywhere}
  @media(max-width:479px){.stages>li{grid-template-columns:minmax(0,1fr) auto}}
</style>
