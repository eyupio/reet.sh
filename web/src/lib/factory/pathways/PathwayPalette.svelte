<script lang="ts" module>
  import type { CrewTier } from '../../gafferOutcome'
  import type { PathwayAgent } from '../pathways'
  export type PaletteTab = 'stages' | 'models' | 'skills' | 'prompts'
  /** What a Models item sets on a stage: a crew tier, a named agent and model, or "Other…" (the inspector's picker). */
  export type ModelChoice = { tier: CrewTier } | { agent: PathwayAgent } | 'other'
</script>

<script lang="ts">
  import { CREW_TIERS, STAGES, defaultTier } from '../../gafferOutcome'
  import type { GafferCrew } from '../../api'
  import type { ModelFavourite } from '../../models'
  import type { TerminalPrompt } from '../../terminal/api'
  import { chatToolName } from '../../terminal/api'
  import { MAX_PATHWAY_STAGES, stageTitle, type PathwaySkill, type PathwayStage } from '../pathways'

  // What can go on a pathway, in four tabs. Every item is a button: a stage is added at the end (before Merge), and a
  // model, skill or prompt is applied to the selected stage. Items carry data-palette-item for the drag layer.
  let { tab = $bindable('stages'), stages, stage, shelf, shelfFailed, onretryshelf, prompts, promptsFailed, favourites, crew, onadd, onmodel, onskill, onprompt, canskill }: {
    tab?: PaletteTab; stages: PathwayStage[]; stage: PathwayStage | null
    shelf: PathwaySkill[] | null; shelfFailed: boolean; onretryshelf: () => void; prompts: TerminalPrompt[] | null; promptsFailed: boolean
    favourites: ModelFavourite[]; crew: GafferCrew | null
    onadd: (kind: string) => string; onmodel: (choice: ModelChoice) => void; onskill: (id: string) => void; onprompt: (text: string) => void; canskill: (id: string) => string
  } = $props()

  const TABS: { id: PaletteTab; label: string }[] = [{ id: 'stages', label: 'Stages' }, { id: 'models', label: 'Models' }, { id: 'skills', label: 'Skills' }, { id: 'prompts', label: 'Prompts' }]
  const uid = $props.id()
  let query = $state('')
  // The last refusal from pressing a stage block, cleared by the next press or a change of tab.
  let refused = $state('')
  $effect(() => { void tab; refused = '' })
  const matches = (...texts: string[]) => { const q = query.trim().toLowerCase(); return !q || texts.some(t => t.toLowerCase().includes(q)) }

  const full = $derived(stages.length >= MAX_PATHWAY_STAGES)
  const builtIns = $derived(STAGES.filter(s => matches(s.label)))
  const showOwn = $derived(matches('Your own stage'))
  const who = (tool?: string, model?: string) => tool ? [chatToolName(tool), model].filter(Boolean).join(' · ') : 'The outcome’s own agent'
  const tiers = $derived(CREW_TIERS.filter(t => matches(t.label, 'crew')))
  const favs = $derived(favourites.filter(f => matches(f.model || 'default', chatToolName(f.tool))))
  const skills = $derived((shelf ?? []).filter(s => matches(s.name, s.description)))
  const firstLine = (text: string) => text.split('\n').find(l => l.trim())?.trim() ?? ''
  const library = $derived((prompts ?? []).filter(p => matches(p.title, p.body)))
  const target = $derived(stage ? stageTitle(stage).trim() || 'this stage' : '')

  function press(kind: string) { refused = onadd(kind) }
  function arrows(event: KeyboardEvent) {
    const at = TABS.findIndex(t => t.id === tab)
    const next = event.key === 'ArrowRight' ? at + 1 : event.key === 'ArrowLeft' ? at - 1 : event.key === 'Home' ? 0 : event.key === 'End' ? TABS.length - 1 : null
    if (next === null) return
    event.preventDefault()
    tab = TABS[(next + TABS.length) % TABS.length].id
    ;(event.currentTarget as HTMLElement).querySelector<HTMLElement>(`#${uid}-${tab}`)?.focus()
  }
</script>

<section class="palette" aria-label="Palette">
  <div class="tabs" role="tablist" aria-label="Palette" tabindex="-1" onkeydown={arrows}>
    {#each TABS as t (t.id)}
      <button type="button" role="tab" id={`${uid}-${t.id}`} aria-selected={tab === t.id} aria-controls={`${uid}-panel`} tabindex={tab === t.id ? 0 : -1} onclick={() => { tab = t.id }}>{t.label}</button>
    {/each}
  </div>
  <input class="search" type="search" aria-label={`Search ${TABS.find(t => t.id === tab)?.label.toLowerCase()}`} placeholder="Search" bind:value={query} />
  <div class="panel" id={`${uid}-panel`} role="tabpanel" aria-labelledby={`${uid}-${tab}`}>
    {#if tab !== 'stages'}
      <p class="aim" class:none={!stage}>{stage ? `Press to apply to ${target}.` : 'Select a stage first.'}</p>
    {/if}

    {#if tab === 'stages'}
      {#if builtIns.length}<p class="group">Built in</p>{/if}
      {#each builtIns as s (s.kind)}
        {@const used = stages.some(one => one.kind === s.kind)}
        <button type="button" class="item" class:used data-palette-item={`stage:${s.kind}`} disabled={used || full} title={full && !used ? `A pathway can have up to ${MAX_PATHWAY_STAGES} stages.` : undefined} onclick={() => press(s.kind)}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name">{s.label}</span>{#if used}<span class="aside">in use</span>{/if}
        </button>
      {/each}
      {#if showOwn}
        <p class="group">Yours</p>
        <button type="button" class="item own" data-palette-item="stage:custom" disabled={full} onclick={() => press('custom')}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name">Your own stage</span>
          {#if full}<span class="why">A pathway can have up to {MAX_PATHWAY_STAGES} stages.</span>{/if}
        </button>
      {/if}
      {#if !builtIns.length && !showOwn}<p class="quiet">No stage matches “{query.trim()}”.</p>{/if}
      <p class="status" role="status">{refused}</p>
      <p class="hint">Press a stage to add it at the end.</p>

    {:else if tab === 'models'}
      {#if tiers.length}<p class="group">Crew tiers</p>{/if}
      {#each tiers as t (t.tier)}
        {@const on = !!stage && !stage.agent && (stage.tier ?? defaultTier(stage.kind)) === t.tier}
        <button type="button" class="item two" data-palette-item={`tier:${t.tier}`} disabled={!stage} onclick={() => onmodel({ tier: t.tier })}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name">{t.label} · crew</span>{#if on}<span class="aside tick">✓</span>{/if}
          <span class="sub">{crew ? who(crew[t.tier]?.tool, crew[t.tier]?.model) : 'Your crew'}</span>
        </button>
      {/each}
      {#if favs.length}<p class="group">Favourites</p>{/if}
      {#each favs as f (`${f.tool}/${f.model}`)}
        {@const on = !!stage?.agent && stage.agent.tool === f.tool && (stage.agent.model ?? '') === f.model}
        <button type="button" class="item two" data-palette-item={`model:${f.tool}/${f.model}`} disabled={!stage} onclick={() => onmodel({ agent: { tool: f.tool, ...(f.model ? { model: f.model } : {}) } })}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name mono">{f.model || 'Agent’s default model'}</span>{#if on}<span class="aside tick">✓</span>{/if}
          <span class="sub">{chatToolName(f.tool)}</span>
        </button>
      {/each}
      <button type="button" class="item" data-palette-item="model:other" disabled={!stage} onclick={() => onmodel('other')}><span class="grip" aria-hidden="true">⠿</span><span class="name">Other…</span></button>

    {:else if tab === 'skills'}
      {#if shelf === null && shelfFailed}<p class="quiet failed">Your skills could not be read. <button type="button" class="retry" onclick={onretryshelf}>Retry</button></p>
      {:else if shelf === null}<p class="quiet">Reading your skills…</p>
      {:else if !shelf.length}<p class="quiet">No skills a stage can load yet. Make a skill Active on the Skills page.</p>
      {:else if !skills.length}<p class="quiet">No skill matches “{query.trim()}”.</p>{/if}
      {#each skills as s (s.id)}
        {@const why = stage ? canskill(s.id) : ''}
        <button type="button" class="item two" data-palette-item={`skill:${s.id}`} disabled={!stage || !!why} onclick={() => onskill(s.id)}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name">{s.name}</span>{#if stage?.skills?.includes(s.id)}<span class="aside tick">✓</span>{/if}
          <span class="sub">{s.description}</span>
          {#if why && !stage?.skills?.includes(s.id)}<span class="why">{why}</span>{/if}
        </button>
      {/each}
      <p class="hint">Select a stage, then press a skill to add it. Up to 2 a stage.</p>

    {:else}
      {#if prompts === null}<p class="quiet">{promptsFailed ? 'Your prompts could not be read.' : 'Reading your prompts…'}</p>
      {:else if !prompts.length}<p class="quiet">No saved prompts yet. Save one on the Prompts page.</p>
      {:else if !library.length}<p class="quiet">No prompt matches “{query.trim()}”.</p>{/if}
      {#each library as p (p.id)}
        <button type="button" class="item two" data-palette-item={`prompt:${p.id}`} disabled={!stage} onclick={() => onprompt(p.body)}>
          <span class="grip" aria-hidden="true">⠿</span><span class="name">{p.title}</span>
          <span class="sub">{firstLine(p.body)}</span>
        </button>
      {/each}
      <p class="hint">A prompt is copied into the stage, after anything already there.</p>
    {/if}
  </div>
</section>

<style>
  .palette { display: grid; align-content: start; gap: 12px; min-width: 0; }
  .tabs { display: grid; grid-template-columns: repeat(4, auto); gap: 0; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .tabs:focus { outline: none; }
  [role='tab'] { min-height: 34px; padding: 0 6px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); font: inherit; font-size: 14px; font-weight: 600; cursor: pointer; }
  [role='tab']:hover { color: var(--ink); }
  [role='tab'][aria-selected='true'] { background: var(--tint); color: var(--accent-text); }
  [role='tab']:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .search { width: 100%; box-sizing: border-box; min-height: 40px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; }
  .search:hover { border-color: var(--line-strong); }
  .search:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; border-color: var(--accent); }
  .panel { display: grid; gap: 6px; min-width: 0; }
  .group { margin: 8px 0 2px; color: var(--muted); font-size: 14px; font-variant-caps: all-small-caps; letter-spacing: 0.06em; font-weight: 600; }
  .group:first-child { margin-top: 0; }
  .aim { margin: 0 0 4px; padding: 8px 10px; border-radius: 8px; background: var(--card); color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .aim.none { color: var(--ink-soft); border: 1px dashed var(--line-strong); background: transparent; }
  .item {
    display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; column-gap: 10px; width: 100%; min-height: 40px; box-sizing: border-box;
    padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; text-align: left; cursor: pointer;
  }
  .item.two { row-gap: 2px; }
  .item:hover:not(:disabled) { border-color: var(--line-strong); background: var(--card-raised); }
  .item:active:not(:disabled) { border-color: var(--muted); }
  .item:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .item:disabled { cursor: default; }
  .item:disabled .name, .item:disabled .grip { color: var(--muted); }
  .item.used { border-style: dashed; background: transparent; }
  .item.own { border-color: var(--accent); }
  .item.own:hover:not(:disabled) { border-color: var(--accent); background: var(--accent-wash); }
  .grip { color: var(--faint); grid-row: 1; }
  .name { font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .name.mono { font: 600 14px var(--mono); letter-spacing: -0.02em; }
  .aside { color: var(--muted); font-size: 14px; }
  .tick { color: var(--accent-text); font-weight: 700; }
  .sub, .why { grid-column: 2 / -1; color: var(--muted); font-size: 14px; line-height: 1.4; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .why { color: var(--warn); }
  .retry { padding: 0; border: 0; background: none; color: var(--accent-text); font: inherit; font-weight: 600; cursor: pointer; }
  .retry:hover { text-decoration: underline; }
  .retry:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 4px; }
  @media (pointer: coarse) { .retry { min-height: 44px; } }
  .quiet { margin: 4px 0; color: var(--muted); font-size: 14px; }
  .status { margin: 0; color: var(--warn); font-size: 14px; }
  .hint { margin: 10px 0 0; color: var(--muted); font-size: 14px; line-height: 1.5; }
  @media (pointer: coarse) { .item, [role='tab'], .search { min-height: 44px; } }
</style>
