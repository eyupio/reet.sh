<script lang="ts">
  import { CREW_TIERS, defaultTier } from '../../gafferOutcome'
  import type { GafferCrew } from '../../api'
  import type { MachineModels, ModelFavourite } from '../../models'
  import { chatToolName } from '../../terminal/api'
  import { agentOptionLabel, crewEffortChoices, crewModelNote } from '../../crewChoices'
  import GafferModelSelect from '../../GafferModelSelect.svelte'
  import ClineSourceSelect from '../../ClineSourceSelect.svelte'
  import {
    MAX_STAGE_GUIDANCE_BYTES, MAX_STAGE_PROMPT_BYTES, MAX_STAGE_SKILLS, byteLength, canAddSkill, canMove, stageTitle,
    type PathwayAgent, type PathwaySkill, type PathwayStage, type StageProblem,
  } from '../pathways'

  // The selected stage's own fields: its place, name, who does it, skills and prompt. Every change goes up as a patch.
  // On a narrow screen (`sheet`) it is the body of a bottom sheet: the name with ↑ ↓ ✕, then Model, Skills and Prompt tabs.
  let {
    stages, at, shelf, crew, favourites, models, modelsRead, modelsFailed, onMachines, offered, problems, sheet = false,
    onpatch, onmove, onremove, onaddskill, onremoveskill, onpickskill, onclose,
  }: {
    stages: PathwayStage[]; at: number; shelf: PathwaySkill[] | null; crew: GafferCrew | null; favourites: ModelFavourite[]
    models: Record<string, MachineModels | null>; modelsRead: Record<string, boolean>; modelsFailed: Record<string, boolean>
    onMachines: (tool: string) => boolean | null; offered: (tool: string) => string[] | null | undefined
    problems: StageProblem[]; sheet?: boolean
    onpatch: (patch: Partial<PathwayStage>) => void; onmove: (to: number) => void; onremove: () => void
    onaddskill: (id: string) => void; onremoveskill: (id: string) => void; onpickskill?: () => void; onclose?: () => void
  } = $props()

  // A stage may be set before a machine connects, so every agent is listed, as in Gaffer settings' crew.
  const AGENTS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const uid = $props.id()
  const stage = $derived(stages[at])
  const own = $derived(stage.kind === 'custom')
  const title = $derived(stageTitle(stage).trim() || 'Unnamed stage')
  const count = $derived(stages.length)
  const said = (field: StageProblem['field']) => problems.filter(p => p.field === field)
  const why = (to: number) => to < 0 ? 'Already the first stage.' : to >= count ? 'Already the last stage.' : canMove(stages, at, to)
  const earlier = $derived(why(at - 1)), later = $derived(why(at + 1))
  const words = (n: number) => n.toLocaleString('en-GB')

  const who = (tier: 'heavy' | 'standard' | 'light') => { const m = crew?.[tier]; return m?.tool ? [chatToolName(m.tool), m.model].filter(Boolean).join(' · ') : crew ? 'The outcome’s own agent' : '' }
  const tier = $derived(stage.tier ?? defaultTier(stage.kind))
  // Switching to a named model starts from whoever the stage's tier is in the crew, or Claude Code.
  function toNamed() {
    if (stage.agent) return
    const m = crew?.[tier]
    onpatch({ agent: m?.tool ? { tool: m.tool, ...(m.model ? { model: m.model } : {}), ...(m.effort ? { effort: m.effort } : {}) } : { tool: 'claude' } })
  }
  const toTier = (next = defaultTier(stage.kind)) => onpatch({ tier: next })
  function setAgent(next: Partial<PathwayAgent>) {
    const agent: PathwayAgent = { ...stage.agent!, ...next }
    for (const key of ['model', 'effort', 'provider'] as const) if (!agent[key]?.trim()) delete agent[key]
    onpatch({ agent })
  }

  const skills = $derived(stage.skills ?? [])
  const guidance = $derived(skills.reduce((sum, id) => sum + (shelf?.find(s => s.id === id)?.bytes ?? 0), 0))
  const missing = (id: string) => !!shelf && !shelf.some(s => s.id === id)
  const promptBytes = $derived(byteLength(stage.prompt ?? ''))
  // The sheet opens on the part with the stage's first problem, if it has one.
  const partOf = (field: StageProblem['field']) => field === 'skills' ? 'skills' : field === 'name' || field === 'prompt' ? 'prompt' : 'model'
  const firstPart = () => problems[0] ? partOf(problems[0].field) : 'model'
  let part = $state<'model' | 'skills' | 'prompt'>(firstPart())
  const PARTS = [{ id: 'model', label: 'Model' }, { id: 'skills', label: 'Skills' }, { id: 'prompt', label: 'Prompt' }] as const
  function partKeys(event: KeyboardEvent) {
    const i = PARTS.findIndex(p => p.id === part)
    const next = event.key === 'ArrowRight' ? i + 1 : event.key === 'ArrowLeft' ? i - 1 : null
    if (next === null) return
    event.preventDefault()
    part = PARTS[(next + PARTS.length) % PARTS.length].id
    ;(event.currentTarget as HTMLElement).querySelector<HTMLElement>(`#${uid}-${part}`)?.focus()
  }
</script>

{#snippet problemLines(list: StageProblem[], id?: string)}
  {#if list.length}<ul class="problems" {id}>{#each list as p (p.text)}<li><span aria-hidden="true">!</span> {p.text}</li>{/each}</ul>{/if}
{/snippet}

{#snippet named(agent: PathwayAgent)}
  {@const effort = crewEffortChoices({ offered: offered(agent.tool), saved: agent.effort ?? '' })}
  <div class="named" data-inspector-model>
    <label class="field"><span class="label">Agent</span>
      <select value={agent.tool} onchange={e => onpatch({ agent: { tool: (e.currentTarget as HTMLSelectElement).value } })}>
        {#each AGENTS as id (id)}<option value={id}>{agentOptionLabel(chatToolName(id), onMachines(id))}</option>{/each}
      </select>
    </label>
    <div class="field"><span class="label" aria-hidden="true">Model</span>
      <GafferModelSelect label="Model" tool={agent.tool} value={agent.model ?? ''} machine={models[agent.tool] ?? null} {favourites} everywhere
        note={crewModelNote({ failed: !!modelsFailed[agent.tool], read: !!modelsRead[agent.tool], listed: !!models[agent.tool]?.models?.length })}
        onchange={model => setAgent({ model })} />
    </div>
    <label class="field"><span class="label">Effort</span>
      <select value={agent.effort ?? ''} disabled={effort.disabled} onchange={e => setAgent({ effort: (e.currentTarget as HTMLSelectElement).value })}>
        {#each effort.options as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
      </select>
    </label>
    {#if agent.tool === 'cline'}
      <div class="field wide"><ClineSourceSelect label="Cline billing source" showLabel={false} star={false} value={agent.provider ?? ''} saved={agent.provider ?? ''} onchange={provider => setAgent({ provider })} /></div>
    {/if}
    {#if effort.unoffered}<p class="note wide">If the agent does not offer this effort, the stage starts on the agent’s default effort.</p>{/if}
  </div>
{/snippet}

{#snippet nameField()}
  {#if own}
    <label class="field"><span class="label">Name</span>
      <input value={stage.name ?? ''} maxlength="80" aria-invalid={said('name').length > 0} aria-describedby={said('name').length ? `${uid}-name` : undefined}
        oninput={e => onpatch({ name: (e.currentTarget as HTMLInputElement).value })} />
    </label>
    {@render problemLines(said('name'), `${uid}-name`)}
  {/if}
{/snippet}

{#snippet promptField()}
  <label class="field"><span class="label">{own ? 'Prompt' : 'Extra instructions'}</span>
    <textarea rows={sheet ? 8 : 5} value={stage.prompt ?? ''} aria-describedby={`${uid}-bytes`} aria-invalid={said('prompt').length > 0}
      placeholder={own ? 'What this stage does, in your words' : 'Anything to add to the built-in wording'}
      oninput={e => onpatch({ prompt: (e.currentTarget as HTMLTextAreaElement).value })}></textarea>
  </label>
  <p class="note" id={`${uid}-bytes`}>{own ? 'This is the stage’s work.' : 'Added after the built-in wording.'} <span class:over={promptBytes > MAX_STAGE_PROMPT_BYTES}>{words(promptBytes)} of {words(MAX_STAGE_PROMPT_BYTES)} bytes.</span></p>
  {@render problemLines(said('prompt'))}
{/snippet}

{#snippet skillsLine()}
  <p class="note">{guidance ? `${words(guidance)} of ${words(MAX_STAGE_GUIDANCE_BYTES)} bytes of guidance.` : 'Skills add their guidance to the stage when it starts.'}</p>
{/snippet}

{#if !sheet}
  <section class="inspector" aria-label="Selected stage">
    <div class="col">
      <div class="head">
        <span class="eyebrow">Stage {at + 1} of {count}</span>
        <div class="tools">
          <button type="button" class="btn quiet" disabled={!!earlier} title={earlier || undefined} onclick={() => onmove(at - 1)}><span aria-hidden="true">‹</span> Earlier</button>
          <button type="button" class="btn quiet" disabled={!!later} title={later || undefined} onclick={() => onmove(at + 1)}>Later <span aria-hidden="true">›</span></button>
          <button type="button" class="btn quiet remove" onclick={onremove}><span aria-hidden="true">✕</span> Remove</button>
        </div>
      </div>
      {@render problemLines(said('order'))}
      {@render nameField()}
      <fieldset class="who">
        <legend class="label">Who does it</legend>
        <div class="pills">
          <label class:on={!stage.agent}><input type="radio" name={`${uid}-who`} checked={!stage.agent} onchange={() => toTier()} />Crew tier</label>
          <label class:on={!!stage.agent}><input type="radio" name={`${uid}-who`} checked={!!stage.agent} onchange={toNamed} />A named model</label>
        </div>
        {#if stage.agent}
          {@render named(stage.agent)}
        {:else}
          <div class="tier">
            <select aria-label="Tier" value={tier} onchange={e => toTier((e.currentTarget as HTMLSelectElement).value as typeof tier)}>
              {#each CREW_TIERS as t (t.tier)}<option value={t.tier}>{t.label}</option>{/each}
            </select>
            {#if who(tier)}<span class="crew">Your crew: <span class="mono">{who(tier)}</span></span>{/if}
          </div>
        {/if}
        {@render problemLines(said('who'))}
        <p class="note">A named model has no stand-in when its plan runs low. A crew tier does.</p>
      </fieldset>
    </div>
    <div class="col">
      <div class="skills">
        <div class="row-head"><span class="label">Skills ({skills.length} of {MAX_STAGE_SKILLS})</span>{#if onpickskill && skills.length < MAX_STAGE_SKILLS}<button type="button" class="link" onclick={onpickskill}>Choose from Skills</button>{/if}</div>
        {#if skills.length}
          <ul class="chips">
            {#each skills as id (id)}
              <li class="chip" class:gone={missing(id)}>{#if missing(id)}<span class="mark" aria-hidden="true">!</span>{/if}<span class="chip-name">{shelf?.find(s => s.id === id)?.name ?? id}</span>
                <button type="button" aria-label={`Remove skill ${id}`} title="Remove" onclick={() => onremoveskill(id)}>✕</button></li>
            {/each}
          </ul>
        {:else}<p class="note">No skills yet.</p>{/if}
        {@render problemLines(said('skills'))}
        {@render skillsLine()}
      </div>
      {@render promptField()}
    </div>
  </section>
{:else}
  <section class="sheet-body" aria-label="Selected stage">
    <div class="sheet-head">
      <h2>{title}</h2>
      <div class="icons">
        <button type="button" class="icon" aria-label="Move earlier" disabled={!!earlier} title={earlier || 'Move earlier'} onclick={() => onmove(at - 1)}>↑</button>
        <button type="button" class="icon" aria-label="Move later" disabled={!!later} title={later || 'Move later'} onclick={() => onmove(at + 1)}>↓</button>
        <button type="button" class="icon remove" aria-label="Remove stage" title="Remove stage" onclick={onremove}>✕</button>
        <button type="button" class="btn quiet close" onclick={onclose}>Close</button>
      </div>
    </div>
    <p class="eyebrow">Stage {at + 1} of {count} · {own ? 'Your own' : 'Built in'}</p>
    {@render problemLines(said('order'))}
    <div class="parts" role="tablist" aria-label="Stage settings" tabindex="-1" onkeydown={partKeys}>
      {#each PARTS as p (p.id)}<button type="button" role="tab" id={`${uid}-${p.id}`} aria-selected={part === p.id} aria-controls={`${uid}-part`} tabindex={part === p.id ? 0 : -1} onclick={() => { part = p.id }}>{p.label}{#if problems.some(q => partOf(q.field) === p.id)}<span class="flag" aria-hidden="true"> !</span>{/if}</button>{/each}
    </div>
    <div class="part" id={`${uid}-part`} role="tabpanel" aria-labelledby={`${uid}-${part}`}>
      {#if part === 'model'}
        {#each CREW_TIERS as t (t.tier)}
          {@const on = !stage.agent && tier === t.tier}
          <button type="button" class="choice" aria-pressed={on} onclick={() => toTier(t.tier)}>
            <span class="name">{t.label} · crew</span><span class="tick" aria-hidden="true">{on ? '✓' : ''}</span>
            {#if who(t.tier)}<span class="sub mono">{who(t.tier)}</span>{/if}
          </button>
        {/each}
        <button type="button" class="choice" aria-pressed={!!stage.agent} onclick={toNamed}><span class="name">A named model</span><span class="tick" aria-hidden="true">{stage.agent ? '✓' : ''}</span></button>
        {#if stage.agent}{@render named(stage.agent)}{/if}
        {@render problemLines(said('who'))}
        <p class="note">A named model has no stand-in when its plan runs low. A crew tier does.</p>
      {:else if part === 'skills'}
        <p class="note">{skills.length} of {MAX_STAGE_SKILLS} skills. {guidance ? `${words(guidance)} of ${words(MAX_STAGE_GUIDANCE_BYTES)} bytes of guidance.` : ''}</p>
        {#each skills.filter(missing) as id (id)}
          <button type="button" class="choice gone" aria-label={`Remove skill ${id}`} onclick={() => onremoveskill(id)}><span class="name"><span aria-hidden="true">! </span>{id}</span><span class="tick" aria-hidden="true">✕</span><span class="sub">No longer available. Press to remove it.</span></button>
        {/each}
        {#if shelf === null}<p class="note">Reading your skills…</p>{/if}
        {#each shelf ?? [] as s (s.id)}
          {@const on = skills.includes(s.id)}
          {@const no = on ? '' : canAddSkill(stage, s, shelf ?? [])}
          <button type="button" class="choice" aria-pressed={on} disabled={!!no} onclick={() => on ? onremoveskill(s.id) : onaddskill(s.id)}>
            <span class="name">{s.name}</span><span class="tick" aria-hidden="true">{on ? '✓' : ''}</span>
            <span class="sub">{no || s.description}</span>
          </button>
        {/each}
        {@render problemLines(said('skills'))}
      {:else}
        {@render nameField()}
        {@render promptField()}
      {/if}
    </div>
  </section>
{/if}

<style>
  .inspector { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 20px 32px; padding: 20px 24px 24px; border-top: 1px solid var(--line); background: var(--card); }
  .col { display: grid; align-content: start; gap: 14px; min-width: 0; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; min-height: 40px; }
  .eyebrow { margin: 0; color: var(--muted); font-size: 14px; font-weight: 600; font-variant-caps: all-small-caps; letter-spacing: 0.06em; }
  .tools { display: flex; gap: 2px; flex-wrap: wrap; }
  .tools .btn { padding: 6px 10px; min-height: 36px; }
  .remove:hover:not(:disabled) { color: var(--bad); }
  .label { color: var(--ink); font-size: 14px; font-weight: 600; }
  .field { display: grid; gap: 6px; min-width: 0; }
  input, select, textarea {
    width: 100%; min-width: 0; box-sizing: border-box; min-height: 40px; padding: 8px 12px; border: 1px solid var(--line-strong); border-radius: 8px;
    background: var(--bg); color: var(--ink); font: inherit; font-size: 15px;
  }
  textarea { resize: vertical; line-height: 1.5; min-height: 110px; }
  input:hover, select:hover, textarea:hover { border-color: var(--muted); }
  input:focus-visible, select:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; border-color: var(--accent); }
  [aria-invalid='true'] { border-color: var(--warn); }
  .note { margin: 0; color: var(--muted); font-size: 14px; line-height: 1.5; }
  .over { color: var(--warn); font-weight: 600; }
  .problems { margin: 0; padding: 0; list-style: none; display: grid; gap: 4px; color: var(--warn); font-size: 14px; line-height: 1.45; }
  .problems span { font-weight: 700; }
  .who { display: grid; gap: 10px; margin: 0; padding: 0; border: 0; min-width: 0; }
  .who legend { padding: 0; margin-bottom: 8px; }
  .pills { display: inline-flex; justify-self: start; padding: 3px; gap: 2px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); }
  .pills label { display: inline-flex; align-items: center; gap: 8px; min-height: 34px; padding: 0 14px; border-radius: 7px; color: var(--muted); font-size: 14px; font-weight: 600; cursor: pointer; }
  .pills label:hover { color: var(--ink); }
  .pills label.on { background: var(--card); color: var(--ink); box-shadow: 0 0 0 1px var(--line-strong); }
  .pills input { width: auto; min-height: 0; margin: 0; accent-color: var(--accent); }
  .pills label:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 1px; }
  .tier { display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; }
  .tier select { width: auto; min-width: 160px; }
  .crew { color: var(--muted); font-size: 14px; }
  .mono { font-family: var(--mono); letter-spacing: -0.02em; color: var(--ink-soft); }
  .named { display: grid; grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.3fr) minmax(0, 0.8fr); gap: 10px; align-items: start; }
  .named .wide { grid-column: 1 / -1; }
  .named :global(.model select), .named :global(.model input), .named :global(.cline-source select) { background: var(--bg); border-color: var(--line-strong); min-height: 40px; padding: 8px 12px; font-size: 15px; }
  .named :global(.unchecked) { font-size: 14px; }
  .skills { display: grid; gap: 10px; }
  .row-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 28px; }
  .link { border: 0; padding: 0; background: none; color: var(--accent-text); font: inherit; font-size: 14px; font-weight: 600; cursor: pointer; }
  .link:hover { text-decoration: underline; }
  .link:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 4px; }
  .chips { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .chip { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; padding: 2px 2px 2px 12px; border: 1px solid var(--line-strong); border-radius: 999px; background: var(--bg); font: 14px/20px var(--mono); letter-spacing: -0.02em; }
  .chip.gone { border-color: var(--warn); color: var(--warn); }
  .chip .mark { font-weight: 700; }
  .chip-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .chip button { display: inline-grid; place-items: center; flex: none; width: 28px; height: 28px; border: 0; border-radius: 50%; background: transparent; color: var(--muted); font: inherit; cursor: pointer; }
  .chip button:hover { background: var(--bg-sunk); color: var(--bad); }
  .chip button:focus-visible { outline: 2px solid var(--accent); outline-offset: 0; }
  @media (max-width: 1100px) { .named { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } .named .field:nth-child(2) { grid-column: 1 / -1; grid-row: 2; } }
  @media (max-width: 900px) { .inspector { grid-template-columns: minmax(0, 1fr); } }

  /* The phone sheet. */
  .sheet-body { display: grid; gap: 12px; padding: 0 16px 20px; }
  .sheet-head { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; gap: 8px; margin: 0 -16px; padding: 2px 16px; background: var(--card); }
  h2 { flex: 1; min-width: 0; margin: 0; font: 600 1.3rem/1.2 var(--serif); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .icons { display: flex; align-items: center; gap: 2px; }
  .icon { display: inline-grid; place-items: center; width: 44px; height: 44px; border: 0; border-radius: 10px; background: transparent; color: var(--ink); font: inherit; font-size: 18px; cursor: pointer; }
  .icon:disabled { color: var(--line-strong); cursor: default; }
  .icon:active:not(:disabled) { background: var(--bg-sunk); }
  .icon.remove { color: var(--bad); }
  .icon:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .close { min-height: 44px; padding: 0 10px; }
  .parts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); }
  .parts:focus { outline: none; }
  .parts [role='tab'] { min-height: 44px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); font: inherit; font-size: 15px; font-weight: 600; cursor: pointer; }
  .parts [role='tab'][aria-selected='true'] { background: var(--card); color: var(--ink); box-shadow: 0 0 0 1px var(--line-strong); }
  .parts [role='tab']:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .flag { color: var(--warn); font-weight: 700; }
  .part { display: grid; gap: 8px; min-width: 0; }
  .choice {
    display: grid; grid-template-columns: minmax(0, 1fr) 24px; align-items: center; gap: 2px 10px; width: 100%; min-height: 52px; box-sizing: border-box; padding: 8px 14px;
    border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--ink); font: inherit; font-size: 15px; text-align: left; cursor: pointer;
  }
  .choice[aria-pressed='true'] { border-color: var(--accent); background: var(--accent-wash); }
  .choice:active:not(:disabled) { background: var(--bg-sunk); }
  .choice:disabled { cursor: default; }
  .choice:disabled .name { color: var(--muted); }
  .choice:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .choice .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .choice .tick { grid-column: 2; grid-row: 1; color: var(--accent-text); font-weight: 700; text-align: center; }
  .choice .sub { grid-column: 1 / -1; color: var(--muted); font-size: 14px; line-height: 1.4; }
  .choice:disabled .sub { color: var(--warn); }
  .choice.gone { border-color: var(--warn); }
  .choice.gone .name, .choice.gone .tick { color: var(--warn); }
  .sheet-body .named { grid-template-columns: minmax(0, 1fr); }
  .sheet-body .named .field:nth-child(2) { grid-column: auto; grid-row: auto; }
  .sheet-body input, .sheet-body select, .sheet-body textarea, .sheet-body .named :global(.model select), .sheet-body .named :global(.model input) { min-height: 44px; font-size: 16px; }
  @media (pointer: coarse) { .tools .btn, .pills label, .tier select, input, select { min-height: 44px; } .chip button { width: 40px; height: 40px; } }
</style>
