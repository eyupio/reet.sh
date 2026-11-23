<script lang="ts">
  // The parts one stage is split into, as the person edits them: each a name, a brief and a tier, done in this order
  // by an agent of its own. Used wherever a pipeline's stages are chosen, so a split reads and works the same there.
  import { CREW_TIERS, MAX_STAGE_PARTS, stageLabel, type StagePart } from '../gafferOutcome'
  import LaunchReadiness from './LaunchReadiness.svelte'
  import type { StageReadiness } from './launchReadiness'

  let { kind, where, parts, busy = false, who, ready, onchange, onadd, onjoin }: {
    kind: string; /** Which pipeline this is, in words, for the controls' names: "pipeline 2" or a pipeline's title. */ where: string
    parts: StagePart[]; busy?: boolean
    /** The agent, model and effort a tier means here, in words. */ who: (tier: StagePart['tier']) => string
    /** What a tier will run on, before approval; drawn under each part. */ ready?: (tier: StagePart['tier']) => StageReadiness
    /** One part changed, or was removed when `next` is null. */ onchange: (index: number, next: Partial<StagePart> | null) => void
    onadd: () => void; /** Do the stage as one again. */ onjoin: () => void
  } = $props()
  const name = $derived(stageLabel(kind))
</script>

<div class="split" role="group" aria-label={`${name} parts in ${where}`} data-stage={kind}>
  <p class="head"><span class="stage">{name}</span><span class="count">Split into {parts.length} parts, done in this order, each by its own agent.</span><button class="link" type="button" disabled={busy} aria-label={`Do ${name} in ${where} as one stage`} onclick={onjoin}>Do as one stage</button></p>
  {#each parts as part, at (at)}<div class="part" role="group" aria-label={`Part ${at + 1} of ${name} in ${where}`}>
    <span class="no" aria-hidden="true">{at + 1}</span>
    <input aria-label={`Part ${at + 1} name`} placeholder="Name this part" value={part.title} maxlength="60" disabled={busy} oninput={event => onchange(at, { title: event.currentTarget.value })}/>
    <select aria-label={`Part ${at + 1} tier`} value={part.tier} disabled={busy} onchange={event => onchange(at, { tier: event.currentTarget.value as StagePart['tier'] })}>{#each CREW_TIERS as choice (choice.tier)}<option value={choice.tier}>{choice.label}</option>{/each}</select>
    <button class="link remove" type="button" disabled={busy} aria-label={`Remove part ${at + 1} of ${name} in ${where}`} onclick={() => onchange(at, null)}>Remove</button>
    <textarea aria-label={`Part ${at + 1} brief`} placeholder="What this part does. Its agent sees only this and the handover from the part before." rows="2" maxlength="1000" value={part.brief} disabled={busy} oninput={event => onchange(at, { brief: event.currentTarget.value })}></textarea>
    {#if ready}<LaunchReadiness ready={ready(part.tier)} label={`part ${at + 1} of ${name} in ${where}`}/>
      {#if part.why}<span class="who"><span class="why"><strong>Gaffer’s assessment.</strong> {part.why}</span></span>{/if}
    {:else}<span class="who">{who(part.tier)}{#if part.why}<span class="why"><strong>Gaffer’s assessment.</strong> {part.why}</span>{/if}</span>{/if}
  </div>{/each}
  {#if parts.length < MAX_STAGE_PARTS}<button class="link add" type="button" disabled={busy} aria-label={`Add a part to ${name} in ${where}`} onclick={onadd}>Add a part</button>{/if}
</div>

<style>
  .split{display:grid;gap:8px;padding:8px;border:1px solid var(--line);border-radius:8px;background:var(--tint);min-width:0}
  .head{margin:0;display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 12px}.stage{font-weight:600}.count{color:var(--muted);font-size:0.875rem;flex:1 1 12rem}
  .part{display:grid;grid-template-columns:1.75rem minmax(0,1fr) 8rem auto;gap:6px 10px;align-items:center;padding:8px;border:1px solid var(--line-soft);border-radius:8px;background:var(--card)}
  .no{width:1.75rem;height:1.75rem;border-radius:999px;display:grid;place-items:center;font-weight:700;font-size:0.875rem;border:1px solid var(--line);color:var(--muted)}
  input,select{min-height:44px;min-width:0;width:100%;font:inherit}textarea{grid-column:2/-1;min-width:0;width:100%;resize:vertical;font:inherit}
  .part :global(.ready-line),.part :global(.notes){grid-column:2/-1}
  .who{grid-column:2/-1;font-size:0.875rem;color:var(--muted);overflow-wrap:anywhere}.why{display:block}
  .link{border:0;background:none;padding:0;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:44px}.link:hover{text-decoration:underline}.link:disabled{opacity:.6;cursor:default}
  .add{justify-self:start}
  @media(max-width:479px){.part{grid-template-columns:1.75rem minmax(0,1fr)}select,textarea,.who,.part :global(.ready-line),.part :global(.notes),.remove{grid-column:2}.remove{order:5;justify-self:start}}
</style>
