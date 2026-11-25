<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { ApiError, createGafferPathway, getGafferCrew, getGafferPathway, listGafferPathwaySkills, listModelFavourites, saveGafferPathway, type GafferCrew } from '../../api'
  import { readPromptLibrary } from '../../prompts/library'
  import { allModels, structuredAgent, terminalMachines, type TerminalMachine, type TerminalPrompt } from '../../terminal/api'
  import { reportedEfforts } from '../../terminal/launchOptions'
  import { effortUnion, mergeMachineModels } from '../../crewChoices'
  import type { MachineModels, ModelFavourite } from '../../models'
  import { shell } from '../../dialogShell'
  import ConfirmDialog from '../../ConfirmDialog.svelte'
  import Skeleton from '../../Skeleton.svelte'
  import {
    addSkill, blankPathway, canAddSkill, canInsert, canMove, insertStage, moveStage, newStage, patchStage, pathwayProblems, removeSkill,
    removeStage, sameEdit, stageTitle, standardPathway, type Pathway, type PathwaySkill, type PathwayStage,
  } from '../pathways'
  import { defaultTier } from '../../gafferOutcome'
  import PathwayPalette, { type ModelChoice, type PaletteTab } from './PathwayPalette.svelte'
  import PathwayLine from './PathwayLine.svelte'
  import StageInspector from './StageInspector.svelte'

  // The pathway editor: a top bar (name, description, on or off, Save), the palette, the line of stages and the selected
  // stage's inspector. It owns the saved pathway and the draft; every edit goes through pathways.ts in the functions
  // below (add, move, remove, patch and the palette's appliers), which a drag layer calls too. Below 700 px wide the
  // palette and the inspector are bottom sheets.
  let { id, start, onclose, onsaved, ontitle }: {
    id: string | null; start: 'blank' | 'standard'; onclose: () => void; onsaved: (p: Pathway) => void; ontitle?: (title: string) => void
  } = $props()

  const life = new AbortController()
  const signal = () => AbortSignal.any([life.signal, AbortSignal.timeout(15_000)])
  const fresh = () => start === 'standard' ? standardPathway() : blankPathway()
  let saved = $state.raw<Pathway>(fresh())
  let draft = $state.raw<Pathway>(fresh())
  let phase = $state<'loading' | 'ready' | 'failed' | 'gone'>(isNew() ? 'ready' : 'loading')
  function isNew() { return id === null }
  let selected = $state(-1)
  let tab = $state<PaletteTab>('stages')

  // What the palette and inspector offer. Each is read once, in parallel with the pathway; a failed read says so where it shows.
  let shelf = $state.raw<PathwaySkill[] | null>(null), shelfFailed = $state(false)
  let prompts = $state.raw<TerminalPrompt[] | null>(null), promptsFailed = $state(false)
  let favourites = $state.raw<ModelFavourite[]>([])
  let crew = $state.raw<GafferCrew | null>(null)
  let machines = $state.raw<TerminalMachine[] | null>(null), machinesFailed = $state(false)
  let models = $state<Record<string, MachineModels | null>>({}), modelsRead = $state<Record<string, boolean>>({}), modelsFailed = $state<Record<string, boolean>>({})

  let saving = $state(false)
  // A refusal from the server shows under the field it names, or beside Save; anything else is a notice.
  let nameError = $state(''), descriptionError = $state(''), saveError = $state(''), notice = $state(''), info = $state('')
  // Someone else saved this pathway after it was read: their version, once read, and whether the choice is showing.
  let conflict = $state(false), theirs = $state.raw<Pathway | null>(null)
  let asking = $state<'' | 'link' | 'back'>('')
  let said = $state('')
  let phone = $state(false), sheet = $state<'' | 'stage' | 'add'>('')
  const uid = $props.id()

  const changed = $derived(!sameEdit(saved, draft))
  const problems = $derived(pathwayProblems(draft, shelf))
  const blocked = $derived(problems.pathway.length + problems.stages.length > 0)
  const stage = $derived(selected >= 0 ? draft.stages[selected] ?? null : null)
  const here = () => saved.id ? `/factory/pathways/${encodeURIComponent(saved.id)}` : '/factory/pathways/new'

  async function loadPathway() {
    if (id === null) return
    phase = 'loading'
    try {
      const p = await getGafferPathway(id, signal())
      saved = p; draft = p; phase = 'ready'
    } catch (e) {
      if (life.signal.aborted) return
      phase = e instanceof ApiError && e.status === 404 ? 'gone' : 'failed'
    }
  }

  async function loadShelf() {
    shelfFailed = false
    try { shelf = await listGafferPathwaySkills(signal()) } catch { if (!life.signal.aborted) shelfFailed = true }
  }

  onMount(() => {
    void loadPathway()
    void loadShelf()
    void readPromptLibrary(life.signal).then(l => { prompts = l.prompts }).catch(() => { if (!life.signal.aborted) promptsFailed = true })
    void listModelFavourites(life.signal).then(f => { favourites = f.favourites }).catch(() => { /* a helper only */ })
    void getGafferCrew(life.signal).then(a => { crew = a.crew }).catch(() => { /* the tiers are still offered, without who they are */ })
    void terminalMachines(life.signal).then(m => { machines = m }).catch(() => { if (!life.signal.aborted) machinesFailed = true })
    const narrow = matchMedia('(max-width: 699.98px)')
    const fit = () => { phone = narrow.matches; if (!phone) sheet = '' }
    fit()
    narrow.addEventListener('change', fit)
    // Browser back or forward with unsaved changes: the editor's address goes back on before the page reads it (the page
    // waits for every listener), and the person is asked. Discard changes throws the edits away, then goes back.
    const pop = () => {
      if (!changed) return
      history.pushState(null, '', here())
      asking = 'back'
    }
    const unload = (event: BeforeUnloadEvent) => { if (changed) { event.preventDefault(); event.returnValue = '' } }
    addEventListener('popstate', pop)
    addEventListener('beforeunload', unload)
    return () => {
      narrow.removeEventListener('change', fit)
      removeEventListener('popstate', pop)
      removeEventListener('beforeunload', unload)
      life.abort()
    }
  })

  $effect(() => { ontitle?.(`${saved.name.trim() || 'New pathway'} · Pathways · reet.sh`) })

  // Each named agent's models across the account's machines, read once when it is first on a stage (as Gaffer settings' crew).
  const used = $derived([...new Set(draft.stages.map(s => s.agent?.tool).filter((t): t is string => !!t))])
  $effect(() => {
    for (const tool of used) {
      if (tool in models) continue
      models[tool] = null
      void allModels(tool, life.signal).then(a => { models[tool] = mergeMachineModels(tool, a.machines); modelsRead[tool] = true }).catch(() => { if (!life.signal.aborted) modelsFailed[tool] = true })
    }
  })
  const onMachines = (tool: string): boolean | null => machines ? machines.some(m => !!structuredAgent(m.capabilities, tool)) : null
  const offered = (tool: string): string[] | null | undefined => machinesFailed ? null : !machines ? undefined : onMachines(tool) ? effortUnion(machines.map(m => reportedEfforts(tool, m.capabilities))) : null

  // The edits. Each goes through pathways.ts; refusals are said in the live region and returned to the caller.
  // Cleared first, so the same sentence twice is still read out twice.
  const say = (text: string) => { said = ''; void tick().then(() => { said = text }) }
  const titled = (s: PathwayStage) => stageTitle(s).trim() || 'This stage'
  function edit(next: Pathway) { draft = next }
  function add(kind: string): string {
    const made = newStage(kind, draft.stages)
    let at = draft.stages.length
    while (at >= 0 && canInsert(draft.stages, made, at)) at--
    if (at < 0) { const why = canInsert(draft.stages, made, draft.stages.length); say(why); return why }
    edit(insertStage(draft, made, at))
    selected = at
    if (phone) sheet = ''
    say(`${titled(made)} added at place ${at + 1} of ${draft.stages.length}.`)
    return ''
  }
  function move(from: number, to: number): boolean {
    if (to < 0 || to >= draft.stages.length) return false
    const why = canMove(draft.stages, from, to)
    if (why) { say(why); return false }
    const moving = draft.stages[from]
    edit(moveStage(draft, from, to))
    selected = to
    say(`${titled(moving)} moved to place ${to + 1} of ${draft.stages.length}`)
    return true
  }
  // Focus goes to the card now in its place, or the one before, or the + when the line is empty, wherever Remove was pressed.
  async function remove(at: number) {
    const gone = draft.stages[at]
    edit(removeStage(draft, at))
    if (selected === at) { selected = -1; sheet = '' } else if (selected > at) selected--
    say(`${titled(gone)} removed.`)
    await tick()
    const line = document.querySelector('[data-pathway-line]')
    ;(line?.querySelector<HTMLElement>(`[data-stage-index="${Math.min(at, draft.stages.length - 1)}"]`) ?? line?.querySelector<HTMLElement>('.add'))?.focus()
  }
  function patch(p: Partial<PathwayStage>) { if (selected >= 0) edit(patchStage(draft, selected, p)) }
  async function applyModel(choice: ModelChoice) {
    if (!stage) return
    if (choice !== 'other') { patch(choice); return }
    if (!stage.agent) { const m = crew?.[stage.tier ?? defaultTier(stage.kind)]; patch({ agent: { tool: m?.tool || 'claude' } }) }
    if (phone) sheet = 'stage'
    await tick()
    document.querySelector<HTMLElement>('[data-inspector-model] .model select')?.focus()
  }
  function applySkill(skillId: string) { if (stage && !canskill(skillId)) edit(addSkill(draft, selected, skillId)) }
  function applyPrompt(text: string) {
    if (!stage) return
    const had = (stage.prompt ?? '').replace(/\s+$/, '')
    patch({ prompt: had ? `${had}\n\n${text}` : text })
  }
  function canskill(skillId: string): string {
    const skill = shelf?.find(s => s.id === skillId)
    return stage && skill ? canAddSkill(stage, skill, shelf ?? []) : ''
  }
  function select(at: number) { selected = at; if (phone) sheet = 'stage' }
  async function focusPalette(which: PaletteTab) {
    tab = which
    if (phone) { sheet = 'add'; return }
    await tick()
    document.querySelector<HTMLElement>('.side [data-palette-item]:not(:disabled), .side input')?.focus()
  }

  const stageProblems = $derived(problems.stages.filter(p => p.at === selected))
  const problemList = $derived([
    ...problems.pathway.map(text => ({ where: '', text })),
    ...problems.stages.map(p => ({ where: titled(draft.stages[p.at]), text: p.text })),
  ])

  async function save() {
    if (saving || blocked) return
    saving = true; nameError = ''; descriptionError = ''; saveError = ''; notice = ''; info = ''
    const sending = draft
    const body = { ...sending, name: sending.name.trim(), description: sending.description.trim() }
    try {
      const answer = body.id ? await saveGafferPathway(body, signal()) : await createGafferPathway({ name: body.name, description: body.description, enabled: body.enabled, stages: body.stages }, signal())
      // Edits made while the save was out stay in the draft, on the answer's id and time.
      saved = answer; conflict = false
      draft = draft === sending ? answer : { ...draft, id: answer.id, createdAt: answer.createdAt, updatedAt: answer.updatedAt }
      say('Saved.')
      onsaved(answer)
    } catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : '', field = e instanceof ApiError ? e.field : ''
      const text = e instanceof Error ? e.message : String(e)
      if (code === 'pathway_changed') { conflict = true; theirs = null; void getGafferPathway(body.id, signal()).then(p => { theirs = p }).catch(() => { /* read again on the choice */ }) }
      else if (code === 'pathway_name') nameError = 'You already have a pathway with that name.'
      else if (code === 'pathway_limit') notice = text
      else if (field === 'name') nameError = text
      else if (field === 'description') descriptionError = text
      else if (field === 'stages') saveError = text
      else notice = `The pathway was not saved. ${text}`
    } finally { saving = false }
  }
  async function newer(): Promise<Pathway | null> {
    if (theirs) return theirs
    try { return theirs = await getGafferPathway(draft.id, signal()) } catch { notice = 'The saved pathway could not be read just now. Try again.'; return null }
  }
  async function loadTheirs() {
    const p = await newer()
    if (!p) return
    saved = p; draft = p; conflict = false; notice = ''
    selected = Math.min(selected, p.stages.length - 1)
  }
  async function keepMine() {
    const p = await newer()
    if (!p) return
    saved = p; draft = { ...draft, updatedAt: p.updatedAt }; conflict = false; notice = ''
    info = 'Your edits are still here. Saving now replaces the version saved elsewhere.'
  }
  function discard() {
    draft = saved; conflict = false; theirs = null; nameError = ''; descriptionError = ''; saveError = ''; info = ''
    selected = Math.min(selected, saved.stages.length - 1)
  }
  function back(event: MouseEvent) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    if (changed) asking = 'link'
    else onclose()
  }
  function leave() {
    const how = asking
    asking = ''
    discard()
    if (how === 'back') history.back()
    else onclose()
  }

  // A bottom sheet is a modal <dialog>: Escape and a press outside close it.
  function modal(node: HTMLDialogElement) {
    node.showModal()
    const cancel = (event: Event) => { event.preventDefault(); sheet = '' }
    const outside = (event: MouseEvent) => { if (event.target === node) sheet = '' }
    node.addEventListener('cancel', cancel)
    node.addEventListener('click', outside)
    return { destroy() { node.removeEventListener('cancel', cancel); node.removeEventListener('click', outside); if (node.open) node.close() } }
  }
</script>

{#snippet palette()}
  <PathwayPalette bind:tab stages={draft.stages} {stage} {shelf} {shelfFailed} onretryshelf={loadShelf} {prompts} {promptsFailed} {favourites} {crew}
    onadd={add} onmodel={choice => void applyModel(choice)} onskill={applySkill} onprompt={applyPrompt} {canskill} />
{/snippet}

{#snippet onoff()}
  <button type="button" role="switch" class="toggle" aria-checked={draft.enabled} aria-label="Offered when starting work" title={draft.enabled ? 'On: offered when you or Gaffer start work' : 'Off: not offered to you or Gaffer'}
    onclick={() => edit({ ...draft, enabled: !draft.enabled })}>
    <span class="switch" aria-hidden="true"><span class="knob"></span></span><span class="state" aria-hidden="true">{draft.enabled ? 'On' : 'Off'}</span>
  </button>
{/snippet}

{#snippet description()}
  <input class="description" aria-label="Description" placeholder="Describe when to use this pathway" value={draft.description}
    aria-invalid={!!descriptionError} aria-describedby={descriptionError ? `${uid}-description` : undefined}
    oninput={e => { edit({ ...draft, description: (e.currentTarget as HTMLInputElement).value }); descriptionError = '' }} />
  {#if descriptionError}<p class="field-error" id={`${uid}-description`}>{descriptionError}</p>{/if}
{/snippet}

{#snippet status()}
  <span class="saved-state" class:dirty={changed}>{saving ? 'Saving…' : !draft.id ? 'Not saved yet' : changed ? 'Unsaved changes' : '✓ Saved'}</span>
  {#if changed && !saving}<button type="button" class="btn quiet discard" onclick={discard}>Discard</button>{/if}
{/snippet}

<section class="editor" class:phone aria-labelledby={`${uid}-title`}>
  <h1 class="vh" id={`${uid}-title`}>Pathway editor</h1>
  {#if phase !== 'ready'}
    <div class="bar"><a class="back" href="/factory/pathways" onclick={back}><span aria-hidden="true">‹</span>&nbsp;Pathways</a></div>
    {#if phase === 'loading'}
      <div class="waiting"><Skeleton label="Reading the pathway…" shape="cards" count={4} height="132px" /></div>
    {:else}
      <div class="failed" role="alert">
        <p><strong>{phase === 'gone' ? 'That pathway is not available.' : 'The pathway could not be read.'}</strong> {phase === 'gone' ? 'It may have been deleted.' : 'Nothing was changed.'}</p>
        {#if phase === 'failed'}<button class="btn" type="button" onclick={loadPathway}>Retry</button>{/if}
      </div>
    {/if}
  {:else}
    <header class="bar">
      <a class="back" href="/factory/pathways" onclick={back} aria-label={phone ? 'Pathways' : undefined}><span aria-hidden="true">‹</span>{#if !phone}&nbsp;Pathways{/if}</a>
      <div class="names">
        <input class="name" aria-label="Pathway name" placeholder="Name this pathway" value={draft.name}
          aria-invalid={!!nameError} aria-describedby={nameError ? `${uid}-name` : undefined}
          oninput={e => { edit({ ...draft, name: (e.currentTarget as HTMLInputElement).value }); nameError = '' }} />
        {#if nameError}<p class="field-error" id={`${uid}-name`}>{nameError}</p>{/if}
        {#if !phone}{@render description()}{/if}
      </div>
      <div class="actions">
        {#if !phone}{@render onoff()}<span class="rule" aria-hidden="true"></span>{@render status()}{/if}
        <button type="button" class="btn primary save" disabled={blocked || saving || (!changed && !!draft.id)} aria-busy={saving}
          aria-describedby={blocked ? `${uid}-why` : undefined} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
      </div>
    </header>
    {#if phone}
      <div class="subbar">
        {@render description()}
        <div class="row">{@render onoff()}<span class="grow"></span>{@render status()}</div>
      </div>
    {/if}

    {#if blocked}
      <ul class="why" id={`${uid}-why`} aria-label="Why Save is off">
        {#each problemList as p, n (n)}<li><span class="mark" aria-hidden="true">!</span>{#if p.where}<span class="where">{p.where}</span>{/if}<span>{p.text}</span></li>{/each}
      </ul>
    {/if}
    {#if saveError}<p class="banner bad" role="alert"><span class="mark" aria-hidden="true">!</span>{saveError}</p>{/if}
    {#if conflict}
      <div class="banner conflict" role="alert">
        <p><strong>“{saved.name}” was changed somewhere else</strong> after you opened it. Your edits are still on screen and nothing was saved.</p>
        <div class="choices">
          <button type="button" class="btn" onclick={loadTheirs}>Load the saved one</button>
          <button type="button" class="btn primary" onclick={keepMine}>Keep editing</button>
        </div>
      </div>
    {/if}
    {#if notice}<p class="banner bad" role="alert"><span class="mark" aria-hidden="true">!</span>{notice}</p>{/if}
    {#if info}<p class="banner" role="status">{info}</p>{/if}

    <div class="body">
      {#if !phone}<aside class="side">{@render palette()}</aside>{/if}
      <div class="main">
        <div class="canvas">
          <PathwayLine stages={draft.stages} {selected} problems={problems.stages} vertical={phone} onselect={select} onmove={move} onremove={at => void remove(at)} onaddend={() => void focusPalette('stages')} />
        </div>
        {#if !phone}
          {#if stage}
            <StageInspector stages={draft.stages} at={selected} {shelf} {crew} {favourites} {models} {modelsRead} {modelsFailed} {onMachines} {offered} problems={stageProblems}
              onpatch={patch} onmove={to => move(selected, to)} onremove={() => void remove(selected)} onaddskill={applySkill} onremoveskill={skillId => edit(removeSkill(draft, selected, skillId))}
              onpickskill={() => void focusPalette('skills')} />
          {:else}
            <p class="pick">Select a stage to set its model, skills and prompt.</p>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
  <p class="vh" aria-live="polite" data-pathway-live>{said}</p>
</section>

{#if phone && sheet === 'stage' && stage}
  <dialog class="sheet" use:shell={{ sheet: 'bottom' }} use:modal aria-label={stageTitle(stage).trim() || 'Unnamed stage'}>
    <div class="handle" aria-hidden="true"></div>
    <p class="vh" aria-live="polite">{said}</p>
    <StageInspector sheet stages={draft.stages} at={selected} {shelf} {shelfFailed} onretryshelf={loadShelf} {crew} {favourites} {models} {modelsRead} {modelsFailed} {onMachines} {offered} problems={stageProblems}
      onpatch={patch} onmove={to => move(selected, to)} onremove={() => void remove(selected)} onaddskill={applySkill} onremoveskill={skillId => edit(removeSkill(draft, selected, skillId))}
      onclose={() => { sheet = '' }} />
  </dialog>
{/if}
{#if phone && sheet === 'add'}
  <dialog class="sheet" use:shell={{ sheet: 'bottom' }} use:modal aria-labelledby={`${uid}-add`}>
    <div class="handle" aria-hidden="true"></div>
    <div class="sheet-head"><h2 id={`${uid}-add`}>Add a stage</h2><button type="button" class="btn quiet" onclick={() => { sheet = '' }}>Close</button></div>
    <p class="vh" aria-live="polite">{said}</p>
    <div class="sheet-palette">{@render palette()}</div>
  </dialog>
{/if}
{#if asking}
  <ConfirmDialog title="Leave without saving?" body="Your changes to this pathway are not saved. If you leave now, they are lost."
    confirmLabel="Discard changes" cancelLabel="Keep editing" danger onconfirm={leave} oncancel={() => { asking = '' }} />
{/if}

<style>
  .editor { display: flex; flex-direction: column; width: 100%; min-height: calc(100dvh - 64px); box-sizing: border-box; padding-bottom: var(--bottom-chrome, 0px); }
  .vh { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }

  /* The top bar: back, the name over its description, on or off, then the save state, Discard and Save at the right. */
  .bar { display: flex; align-items: center; gap: 12px 20px; padding: 14px 24px; border-bottom: 1px solid var(--line); background: var(--bg); }
  .back { display: inline-flex; align-items: center; flex: none; min-height: 40px; padding: 0 6px; margin-left: -6px; border-radius: 8px; color: var(--muted); text-decoration: none; font-weight: 600; font-size: 15px; }
  .back:hover { color: var(--ink); }
  .back:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .names { display: grid; flex: 1 1 320px; min-width: 0; max-width: 560px; gap: 0; }
  .names input {
    width: 100%; min-width: 0; box-sizing: border-box; margin: 0; border: 1px solid transparent; border-radius: 8px; background: transparent; color: var(--ink);
  }
  .names input:hover { border-color: var(--line); }
  .names input:focus-visible { outline: 2px solid var(--accent); outline-offset: 0; background: var(--card); }
  .name { padding: 2px 8px; margin-left: -9px !important; font: 600 26px/1.25 var(--serif) !important; letter-spacing: -0.02em; text-overflow: ellipsis; }
  .name::placeholder { color: var(--faint); }
  .description { padding: 2px 8px; margin-left: -9px !important; color: var(--muted) !important; font: 15px/1.5 var(--sans); text-overflow: ellipsis; }
  .description::placeholder { color: var(--faint); }
  [aria-invalid='true'] { border-color: var(--warn) !important; }
  .field-error { margin: 2px 0 4px; color: var(--warn); font-size: 14px; }
  .actions { display: flex; align-items: center; gap: 4px 10px; margin-left: auto; flex: none; }
  .rule { width: 1px; height: 24px; margin: 0 6px; background: var(--line); }
  .saved-state { color: var(--muted); font-size: 14px; white-space: nowrap; }
  .saved-state.dirty { color: var(--ink-soft); }
  .discard { padding: 8px 10px; }
  .save { min-width: 88px; }

  /* The on or off switch, as on the list. */
  .toggle { display: inline-flex; align-items: center; gap: 8px; flex: none; min-height: 40px; padding: 0 8px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font: inherit; font-size: 14px; font-weight: 600; cursor: pointer; }
  .toggle:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .toggle:hover .switch { border-color: var(--muted); }
  .toggle[aria-checked='true']:hover .switch { border-color: var(--accent); }
  .switch { position: relative; flex: none; width: 42px; height: 24px; box-sizing: border-box; border: 1px solid var(--line); border-radius: 12px; background: var(--bg-sunk); }
  .toggle[aria-checked='true'] .switch { background: var(--accent); border-color: var(--accent); }
  .knob { position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--card); box-shadow: var(--shadow); }
  .toggle[aria-checked='false'] .knob { background: var(--line-strong); box-shadow: none; }
  .toggle[aria-checked='true'] .knob { transform: translateX(18px); }
  .toggle .state { min-width: 24px; text-align: left; }

  /* Why Save is off, and what the server said. */
  .why { display: flex; flex-wrap: wrap; gap: 6px 20px; margin: 0; padding: 10px 24px; list-style: none; border-bottom: 1px solid var(--line); background: var(--card); color: var(--ink-soft); font-size: 14px; line-height: 1.45; }
  .why li { min-width: 0; }
  .mark { color: var(--warn); font-weight: 700; margin-right: 6px; }
  .where { color: var(--ink); font-weight: 600; white-space: nowrap; }
  .where::after { content: ' · '; color: var(--muted); font-weight: 400; white-space: pre; }
  .banner { margin: 12px 24px 0; padding: 12px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--ink-soft); font-size: 14px; line-height: 1.5; }
  .banner.bad { border-color: var(--bad); }
  .conflict { display: flex; align-items: center; justify-content: space-between; gap: 12px 20px; flex-wrap: wrap; border-color: var(--warn); }
  .conflict p { flex: 1 1 360px; margin: 0; }
  .conflict strong { color: var(--ink); }
  .choices { display: flex; gap: 8px; flex-wrap: wrap; }

  /* The palette at the left; the canvas and the inspector at the right. */
  .body { display: grid; grid-template-columns: 264px minmax(0, 1fr); flex: 1; min-height: 0; margin-top: 0; }
  .banner + .body, .why + .body { margin-top: 0; }
  .side { padding: 16px; border-right: 1px solid var(--line); background: var(--bg-sunk); }
  .main { display: flex; flex-direction: column; min-width: 0; }
  .canvas {
    display: flex; align-items: center; flex: 1; min-height: 300px;
    background-color: var(--bg); background-image: radial-gradient(circle, var(--line-strong) 1px, transparent 1.4px); background-size: 20px 20px; background-position: 10px 10px;
  }
  .canvas > :global(.line) { flex: 1; min-width: 0; }
  .pick { margin: 0; padding: 28px 24px; border-top: 1px solid var(--line); background: var(--card); color: var(--muted); text-align: center; font-size: 15px; }
  .waiting { padding: 24px; }
  .failed { display: flex; align-items: center; justify-content: space-between; gap: 12px 16px; flex-wrap: wrap; margin: 24px; padding: 14px 16px; border: 1px solid var(--bad); border-radius: var(--radius); background: var(--card); }
  .failed p { flex: 1 1 280px; margin: 0; color: var(--muted); }
  .failed strong { color: var(--bad); font-weight: 600; }

  @media (max-width: 1000px) { .bar { flex-wrap: wrap; } .names { flex-basis: 100%; max-width: none; order: 3; } .actions { order: 2; } }

  /* A phone: a compact bar (back, name, Save), the description and switch under it, and the line top to bottom. */
  .phone .bar { flex-wrap: nowrap; gap: 4px; padding: 8px 12px 8px 8px; }
  .phone .back { justify-content: center; width: 44px; min-height: 44px; margin: 0; padding: 0; font-size: 26px; font-weight: 400; }
  .phone .names { order: 0; flex: 1 1 auto; }
  .phone .name { font-size: 21px !important; margin-left: 0 !important; }
  .phone .actions { order: 0; }
  .phone .save { min-height: 44px; min-width: 72px; }
  .subbar { display: grid; gap: 4px; padding: 6px 16px 10px; border-bottom: 1px solid var(--line); }
  .subbar .description { margin-left: -9px; width: calc(100% + 9px); min-height: 44px; border: 1px solid transparent; border-radius: 8px; background: transparent; font-size: 16px; }
  .subbar .description:focus-visible { outline: 2px solid var(--accent); outline-offset: 0; }
  .subbar .row { display: flex; align-items: center; gap: 8px; }
  .subbar .toggle { min-height: 44px; margin-left: -8px; }
  .subbar .discard { min-height: 44px; }
  .grow { flex: 1; }
  .phone .why { padding: 10px 16px; flex-direction: column; }
  .phone .banner { margin: 12px 16px 0; }
  .phone .body { display: block; }
  .phone .canvas { display: block; min-height: 0; background-image: none; }

  /* The bottom sheets. */
  .sheet {
    position: fixed; inset: auto 0 0 0; width: 100%; max-width: none; max-height: 86dvh; margin: 0; padding: 0 0 env(safe-area-inset-bottom, 0px); box-sizing: border-box;
    overflow-y: auto; overscroll-behavior: contain; border: 1px solid var(--line); border-width: 1px 0 0; border-radius: 16px 16px 0 0; background: var(--card); color: var(--ink); box-shadow: var(--shadow-dock);
  }
  /* Above the shared shell's phone rule, which would let the sheet fill the screen. */
  .sheet.sheet.sheet[open] { max-height: 88dvh; }
  .sheet::backdrop { background: var(--scrim); }
  .handle { width: 40px; height: 4px; margin: 8px auto 6px; border-radius: 2px; background: var(--line-strong); }
  .sheet-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 0 12px 0 16px; }
  .sheet-head h2 { margin: 0; font: 600 1.3rem/1.2 var(--serif); letter-spacing: -0.01em; }
  .sheet-head .btn { min-height: 44px; }
  .sheet-palette { padding: 8px 16px 20px; }

  @media (prefers-reduced-motion: no-preference) {
    .knob { transition: transform 0.16s var(--ease-out), background-color 0.16s; }
    .sheet[open] { animation: up 220ms var(--ease-out) both; }
    @keyframes up { from { transform: translateY(24px); opacity: 0; } to { transform: none; opacity: 1; } }
  }
</style>
