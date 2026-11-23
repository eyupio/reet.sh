<script lang="ts">
  // A saved plan, read only: each pipeline with its prompt, stages, criteria and the skills each stage was approved with.
  // A skill that is no longer approved at that revision says so on its stage. Before a start (`lines`) each pipeline is
  // one line with its card behind it, open where it changed since the version compared (`diff`) or has a skill gap,
  // and what changed is marked where it is.
  import type { TemplateContent, TemplateDiff, TemplateSkillGap } from '../api'
  import { stageLabel, tierLabel } from '../gafferOutcome'
  import { diffStage, gapWords, limitsWords, mergeWords, pipelineLine } from './templates'

  let { content, gaps = [], diff = null, lines = false, idPrefix = '' }: { content: TemplateContent; gaps?: TemplateSkillGap[]; diff?: TemplateDiff | null; lines?: boolean; idPrefix?: string } = $props()
  const gapFor = (pipeline: number, stage: number, id: string) => gaps.find(gap => gap.pipeline === pipeline && gap.stage === stage && gap.id === id)
  const changeOf = (title: string) => diff?.changed ? diff.pipelines.find(p => p.title === title && p.change !== 'same') : undefined
  const criterionMark = (title: string, c: { id: string; text: string }) => { const p = changeOf(title); return p?.criteria.reworded.some(r => r.id === c.id) ? 'Reworded' : p?.criteria.added.includes(c.text) ? 'Added' : '' }
  const skillMark = (title: string, stage: { kind: string; part?: string }, skill: { id: string; revision: string }) => { const s = changeOf(title)?.skills.find(one => one.id === skill.id && one.to === skill.revision && one.stage === diffStage(stage)); return !s ? '' : s.from ? 'Revision changed' : 'Added' }
</script>

{#snippet card(pipeline: TemplateContent['pipelines'][number], p: number)}
    <details><summary>Prompt</summary><p class="prompt">{pipeline.prompt}</p></details>
    <ul class="stages" aria-label={`Stages of ${pipeline.title}`}>
      {#each pipeline.stages as stage, s (s)}<li>
        <span class="name">{stageLabel(stage.kind)}{stage.part ? ` · ${stage.part}` : ''}</span>
        <span class="tier">{tierLabel(stage.tier)}</span>
        {#if stage.brief}<span class="brief">{stage.brief}</span>{/if}
        {#each stage.skills as skill (skill.id)}{@const gap = gapFor(p, s, skill.id)}{@const mark = skillMark(pipeline.title, stage, skill)}
          <span class="skill">{skill.name} · revision {skill.revision.slice(0, 8)}{#if mark}{' '}<span class="badge">{mark}</span>{/if}</span>
          {#if gap}<span class="gap" class:gone={gap.status === 'unavailable'} id={idPrefix ? `${idPrefix}gap-${gap.key}` : undefined}><span class="mark" aria-hidden="true">!</span> {gapWords(gap)}</span>{/if}
        {/each}
      </li>{/each}
    </ul>
    {#if pipeline.criteria.length}
      <p class="label">{pipeline.criteria.length === 1 ? 'Criterion' : 'Criteria'}</p>
      <ul class="criteria" aria-label={`Criteria of ${pipeline.title}`}>{#each pipeline.criteria as criterion (criterion.id)}{@const mark = criterionMark(pipeline.title, criterion)}<li><span aria-hidden="true">·</span> {criterion.text}{#if mark}{' '}<span class="badge">{mark}</span>{/if}</li>{/each}</ul>
    {:else}<p class="hint">No criteria, so this pipeline has no delivery review.</p>{/if}
    <p class="merge" class:auto={pipeline.autoMerge}>{mergeWords(pipeline.autoMerge)}</p>
{/snippet}

<p class="limits">{limitsWords(content)}</p>
<ol class="plan" aria-label="The plan">
  {#each content.pipelines as pipeline, p (p)}<li>
    {#if lines}<details class="pipeline" open={!!changeOf(pipeline.title) || gaps.some(gap => gap.pipeline === p)}><summary>{pipelineLine(pipeline)}</summary><div class="body">{@render card(pipeline, p)}</div></details>
    {:else}<h3>{pipeline.title}</h3>{@render card(pipeline, p)}{/if}
  </li>{/each}
</ol>

<style>
  .limits{margin:0;color:var(--muted);font-size:14px;line-height:1.5}
  .plan{list-style:none;margin:0;padding:0;display:grid;gap:12px;font-size:14px;line-height:1.5}
  .plan>li,.body{display:grid;gap:8px;min-width:0}.plan>li{padding:12px;border:1px solid var(--line);border-radius:12px}
  h3{margin:0;font:600 1rem/1.3 var(--sans);overflow-wrap:anywhere}
  summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;color:var(--accent-text);font-weight:600}
  .pipeline>summary{color:var(--ink);overflow-wrap:anywhere}.pipeline[open]>summary{margin-bottom:4px}
  summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .prompt{margin:0;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--ink-soft)}
  .stages,.criteria{list-style:none;margin:0;padding:0;display:grid;gap:0}.criteria{gap:2px}
  .stages>li{display:grid;grid-template-columns:minmax(6rem,auto) 5rem minmax(0,1fr);gap:2px 10px;align-items:baseline;padding:8px 0;border-top:1px solid var(--line-soft)}
  .name{font-weight:600;overflow-wrap:anywhere}.tier{color:var(--muted)}
  .brief,.skill,.gap{grid-column:1/-1;color:var(--muted);overflow-wrap:anywhere}
  .gap{color:var(--ink)}.gap .mark{font-weight:700;color:var(--warn)}.gap.gone .mark{color:var(--bad)}
  .badge{padding:0 6px;color:var(--ink)}
  .label{margin:0;font-weight:600}.merge{margin:0;color:var(--muted)}.merge.auto{color:var(--ink);font-weight:600}.hint{margin:0;color:var(--muted)}
  .criteria li{overflow-wrap:anywhere}
  @media(max-width:479px){.stages>li{grid-template-columns:minmax(0,1fr) auto}}
</style>
