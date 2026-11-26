<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { ApiError, createGafferPathway, getGafferCrew, getGafferPathway, listGafferPathwaySkills, listModelFavourites, saveGafferPathway, type GafferCrew } from '../../api'
  import { readPromptLibrary } from '../../prompts/library'
  import { allModels, chatToolName, structuredAgent, terminalMachines, type TerminalMachine, type TerminalPrompt } from '../../terminal/api'
  import { reportedEfforts } from '../../terminal/launchOptions'
  import { effortUnion, mergeMachineModels } from '../../crewChoices'
  import type { MachineModels, ModelFavourite } from '../../models'
  import { shell } from '../../dialogShell'
  import ConfirmDialog from '../../ConfirmDialog.svelte'
  import Skeleton from '../../Skeleton.svelte'
  import {
    addSkill, blankPathway, canAddSkill, canInsert, canMove, insertStage, moveStage, newStage, patchStage, pathwayProblems, removeSkill,
    removeStage, sameEdit, sameStages, stageTitle, standardPathway, MAX_STAGE_SKILLS, type Pathway, type PathwaySkill, type PathwayStage,
  } from '../pathways'
  import { defaultTier, stageLabel, tierLabel } from '../../gafferOutcome'
  import { targetAt, type DragHooks, type DragItem, type DragState } from './drag'
  import PathwayPalette, { type ModelChoice, type PaletteTab } from './PathwayPalette.svelte'
  import PathwayLine from './PathwayLine.svelte'
  import StageInspector from './StageInspector.svelte'

  // The pathway editor: a top bar (name, description, on or off, Save), the palette, the line of stages and the selected
  // stage's inspector. It owns the saved pathway and the draft; every edit goes through pathways.ts in the functions
  // below (add, move, remove, patch and the palette's appliers), which a drop calls too. Below 700 px wide the palette and
  // the inspector are bottom sheets, and dragging only moves cards along the line. Focus arrives on the name (on a touch
  // screen, where a field would raise the keyboard, on the heading).
  let { id, start, onclose, onsaved, ontitle }: {
    id: string | null; start: 'blank' | 'standard'; onclose: () => void; onsaved: (p: Pathway) => void; ontitle?: (title: string) => void
  } = $props()

  const life = new AbortController()
  const signal = () => AbortSignal.any([life.signal, AbortSignal.timeout(15_000)])
  const fresh = () => start === 'standard' ? standardPathway() : blankPathway()
  const first = fresh()
  let saved = $state.raw<Pathway>(first)
  let draft = $state.raw<Pathway>(first)
  // An id for each stage on the line, in step with draft.stages, so a moved card keeps its element and slides. They live
  // here rather than on the stages, so they never reach Save or the unsaved-changes test.
  let serial = 0
  const newKeys = (n: number) => Array.from({ length: n }, () => ++serial)
  let keys = $state.raw<number[]>(newKeys(first.stages.length))
  function replace(p: Pathway) { draft = p; keys = newKeys(p.stages.length) }
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
  // Deleted somewhere else: Save makes a new pathway from the edits.
  let deleted = $state(false)
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
      // An address that is not a pathway is gone, as a 404 is: /skills answers with the skills a stage can load.
      if (Array.isArray(p?.stages)) { saved = p; replace(p); phase = 'ready' } else phase = 'gone'
    } catch (e) {
      if (life.signal.aborted) return
      phase = e instanceof ApiError && e.status === 404 ? 'gone' : 'failed'
    }
    void arrive()
  }
  let heading = $state<HTMLElement>()
  async function arrive() {
    await tick()
    const name = phase === 'ready' && matchMedia('(pointer: fine)').matches ? document.querySelector<HTMLElement>('.editor .name') : null
    ;(name ?? heading)?.focus({ preventScroll: true })
  }

  async function loadShelf() {
    shelfFailed = false
    try { shelf = await listGafferPathwaySkills(signal()) } catch { if (!life.signal.aborted) shelfFailed = true }
  }

  onMount(() => {
    if (id === null) void arrive()
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

  // The palette's dock reaches the foot of the window wherever the page is scrolled: as tall as what is in view below its
  // top, and the whole window once it sticks there, less the phone's bottom tab bar where that is showing (it is fixed over
  // the page, so the window's foot is its top). Its list scrolls inside it.
  let dock = $state<HTMLElement>()
  const windowFoot = () => { const tabs = document.querySelector('nav.bottom-tabs'); return tabs && getComputedStyle(tabs).display !== 'none' ? tabs.getBoundingClientRect().top : innerHeight }
  $effect(() => {
    const node = dock, side = node?.parentElement
    if (!node || !side) return
    let frame = 0
    const fit = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(() => { node.style.maxHeight = `${windowFoot() - Math.max(0, side.getBoundingClientRect().top)}px` }) }
    fit()
    addEventListener('scroll', fit, { passive: true })
    addEventListener('resize', fit)
    return () => { cancelAnimationFrame(frame); removeEventListener('scroll', fit); removeEventListener('resize', fit) }
  })

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
    place(made, at)
    return ''
  }
  function place(made: PathwayStage, at: number) {
    edit(insertStage(draft, made, at))
    keys = [...keys.slice(0, at), ++serial, ...keys.slice(at)]
    selected = at
    if (phone) sheet = ''
    say(`${titled(made)} added at place ${at + 1} of ${draft.stages.length}.`)
  }
  function move(from: number, to: number): boolean {
    if (to < 0 || to >= draft.stages.length) return false
    const why = canMove(draft.stages, from, to)
    if (why) { say(why); return false }
    const moving = draft.stages[from]
    edit(moveStage(draft, from, to))
    const rest = keys.filter((_, i) => i !== from)
    keys = [...rest.slice(0, to), keys[from], ...rest.slice(to)]
    selected = to
    say(`${titled(moving)} moved to place ${to + 1} of ${draft.stages.length}`)
    return true
  }
  // Focus goes to the card now in its place, or the one before, or the + when the line is empty, wherever Remove was pressed.
  async function remove(at: number) {
    const gone = draft.stages[at]
    edit(removeStage(draft, at))
    keys = keys.filter((_, i) => i !== at)
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
      const answer = body.id && !deleted ? await saveGafferPathway(body, signal()) : await createGafferPathway({ name: body.name, description: body.description, enabled: body.enabled, stages: body.stages }, signal())
      // Edits made while the save was out stay in the draft, on the answer's id and time.
      saved = answer; conflict = false; deleted = false
      // An answer with the stages on screen keeps the cards (and a drag or focus on one); edits made meanwhile stay.
      if (draft === sending) { if (sameStages(answer.stages, sending.stages)) draft = answer; else replace(answer) }
      else draft = { ...draft, id: answer.id, createdAt: answer.createdAt, updatedAt: answer.updatedAt }
      say('Saved.')
      onsaved(answer)
    } catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : '', field = e instanceof ApiError ? e.field : ''
      const text = e instanceof Error ? e.message : String(e)
      if (e instanceof ApiError && e.status === 404) gone()
      else if (code === 'pathway_changed') { conflict = true; theirs = null; void getGafferPathway(body.id, signal()).then(p => { theirs = p }).catch(() => { /* read again on the choice */ }) }
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
    try { return theirs = await getGafferPathway(draft.id, signal()) } catch (e) {
      if (e instanceof ApiError && e.status === 404) gone()
      else notice = 'The saved pathway could not be read just now. Try again.'
      return null
    }
  }
  function gone() { deleted = true; conflict = false; theirs = null; notice = '' }
  function leaveDeleted() { discard(); onclose() }
  async function loadTheirs() {
    const p = await newer()
    if (!p) return
    saved = p; replace(p); conflict = false; notice = ''
    selected = Math.min(selected, p.stages.length - 1)
  }
  async function keepMine() {
    const p = await newer()
    if (!p) return
    saved = p; draft = { ...draft, updatedAt: p.updatedAt }; conflict = false; notice = ''
    info = 'Your edits are still here. Saving now replaces the version saved elsewhere.'
  }
  function discard() {
    replace(saved); conflict = false; theirs = null; nameError = ''; descriptionError = ''; saveError = ''; info = ''
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

  // Dragging. The palette's items and the cards report a drag here; the editor works out where it would land, draws the
  // ghost, the insertion bar and a sentence saying what a drop does (or why not), and a drop calls the same edits as the buttons.
  let dragging = $state.raw<DragState>(null)
  let ghost = $state.raw<{ node: HTMLElement; dx: number; dy: number; width: number; height: number } | null>(null)
  // Over the line the ghost drops below the cards, so the card or gap it would land on stays in sight: the top it keeps clear of.
  let floor = $state(0)
  let bar = $state.raw<{ left: number; top: number; width: number; height: number } | null>(null)
  let hintAt = $state.raw({ x: 0, y: 0, above: false, side: false })
  let canvas = $state<HTMLElement>()
  let raf = 0
  const lineOf = () => canvas?.querySelector<HTMLElement>('[data-pathway-line]') ?? null
  const slotTo = (from: number, at: number) => at > from ? at - 1 : at
  const skillName = (skillId: string) => shelf?.find(s => s.id === skillId)?.name ?? skillId
  const stageName = (kind: string) => kind === 'custom' ? 'your own stage' : stageLabel(kind)
  const modelName = (choice: ModelChoice) => choice === 'other' ? 'another model' : 'tier' in choice ? `the ${tierLabel(choice.tier)} crew tier` : choice.agent.model || chatToolName(choice.agent.tool)
  function refusal(item: DragItem, at: number): string {
    if (item.type !== 'skill') return ''
    const skill = shelf?.find(s => s.id === item.id)
    return skill ? canAddSkill(draft.stages[at], skill, shelf ?? []) : ''
  }

  const hooks: DragHooks = {
    onstart(s, node) {
      const box = node.getBoundingClientRect()
      const copy = node.cloneNode(true) as HTMLElement
      for (const name of ['id', 'data-stage-index', 'data-palette-item', 'data-drop', 'data-dragging', 'aria-pressed']) copy.removeAttribute(name)
      // On a phone a card is held where it was pressed. Otherwise what is picked up floats below and to the right of the
      // pointer, and below the cards while over the line.
      ghost = { node: copy, dx: phone ? s.x - box.left : -16, dy: phone ? s.y - box.top : -20, width: box.width, height: box.height }
      document.documentElement.style.userSelect = 'none'
      document.documentElement.style.setProperty('-webkit-user-select', 'none')
      document.documentElement.dataset.pathwayDrag = ''
      aim(s.item, s.x, s.y)
      raf = requestAnimationFrame(edges)
    },
    onmove(x, y) { if (dragging) aim(dragging.item, x, y) },
    onend(drop) {
      const was = dragging
      cancelAnimationFrame(raf)
      dragging = null; ghost = null; bar = null; floor = 0
      document.documentElement.style.removeProperty('user-select')
      document.documentElement.style.removeProperty('-webkit-user-select')
      delete document.documentElement.dataset.pathwayDrag
      if (drop && was) land(was)
    },
  }

  // Where the pointer would drop: a slot the order rules allow, or a card (which may refuse, with why).
  function aim(item: DragItem, x: number, y: number) {
    const line = lineOf()
    if (!canvas || !line) return
    const cards = [...line.querySelectorAll<HTMLElement>('[data-stage-index]')].map(el => el.getBoundingClientRect())
    let target = targetAt(item, x, y, cards, canvas.getBoundingClientRect(), phone), why = ''
    if (target?.type === 'slot') {
      why = item.type === 'stage' ? canInsert(draft.stages, newStage(item.kind, draft.stages), target.at) : item.type === 'card' ? canMove(draft.stages, item.from, slotTo(item.from, target.at)) : ''
      if (why) target = null
    } else if (target) why = refusal(item, target.at)
    dragging = { item, x, y, target, why }
    bar = target?.type === 'slot' ? barAt(target.at, cards) : null
    const box = line.getBoundingClientRect(), bottom = Math.max(...cards.map(c => c.bottom))
    floor = !phone && cards.length && x >= box.left && x <= box.right && y >= box.top && y <= bottom ? bottom + 12 : 0
    const below = floor && ghost ? floor + ghost.height + 16 : cards.length ? bottom + 18 : box.top + box.height / 2
    // Where it would not fit in the window it goes above the line, or above the pointer when that is higher (never under
    // it); where there is no room above either, beside the pointer, to its right.
    const tall = document.querySelector('[data-drag-hint]')?.getBoundingClientRect().height || 80
    const out = !phone && cards.length > 0 && below + tall > windowFoot() - 8, lift = Math.min(box.top, y) - 12
    const side = out && lift - tall < 8
    hintAt = side ? { x: x + 20, y, above: false, side } : { x: box.left + box.width / 2, y: out ? lift : below, above: out, side }
  }
  // The insertion bar, in the gap the slot is: between two cards, or just outside the first or last.
  function barAt(at: number, cards: DOMRect[]) {
    const a = cards[at - 1], b = cards[at], ref = b ?? a
    if (!ref) return null
    if (phone) {
      const y = a && b ? (a.bottom + b.top) / 2 : b ? b.top - 7 : a.bottom + 7
      return { left: ref.left, top: y - 1.5, width: ref.width, height: 3 }
    }
    const x = a && b ? (a.right + b.left) / 2 : b ? b.left - 20 : a.right + 20
    return { left: x - 1.5, top: ref.top - 8, width: 3, height: ref.height + 16 }
  }
  // Held near an edge of the line (or, on a phone, of the screen), the line scrolls that way.
  const EDGE = 56
  function edges() {
    const d = dragging, line = lineOf()
    if (!d || !line) return
    const speed = (gap: number, edge: number) => Math.ceil(((edge - Math.max(gap, 0)) / edge) * 14)
    let scroller: Element = line, by = 0
    if (phone) {
      scroller = scrollerOf(line)
      if (d.y < 80) by = -speed(d.y, 80)
      else if (d.y > innerHeight - 80) by = speed(innerHeight - d.y, 80)
    } else {
      const r = line.getBoundingClientRect()
      if (d.y >= r.top && d.y <= r.bottom && d.x >= r.left && d.x <= r.right) {
        if (d.x < r.left + EDGE) by = -speed(d.x - r.left, EDGE)
        else if (d.x > r.right - EDGE) by = speed(r.right - d.x, EDGE)
      }
    }
    if (by) {
      const was = phone ? scroller.scrollTop : scroller.scrollLeft
      if (phone) scroller.scrollTop += by
      else scroller.scrollLeft += by
      if ((phone ? scroller.scrollTop : scroller.scrollLeft) !== was) aim(d.item, d.x, d.y)
    }
    raf = requestAnimationFrame(edges)
  }
  function scrollerOf(node: Element): Element {
    for (let e = node.parentElement; e; e = e.parentElement) {
      const overflow = getComputedStyle(e).overflowY
      if ((overflow === 'auto' || overflow === 'scroll') && e.scrollHeight > e.clientHeight) return e
    }
    return document.scrollingElement ?? document.documentElement
  }

  // A drop: the same edits the buttons make, and the stage is selected.
  function land({ item, target, why }: NonNullable<DragState>) {
    if (!target || why) return
    if (item.type === 'stage') place(newStage(item.kind, draft.stages), target.at)
    else if (item.type === 'card') { const to = slotTo(item.from, target.at); if (to !== item.from) move(item.from, to) }
    else {
      selected = target.at
      if (item.type === 'model') void applyModel(item.choice)
      else if (item.type === 'skill') applySkill(item.id)
      else applyPrompt(item.text)
    }
  }

  // While a model, skill or prompt is dragged: the card under the pointer is ringed with what would change, and every card
  // that cannot take a skill is dimmed with why, in a word.
  const aims = $derived.by(() => {
    const d = dragging
    if (!d || d.item.type === 'stage' || d.item.type === 'card') return []
    const item = d.item, on = d.target?.type === 'card' ? d.target.at : -1
    return draft.stages.map((s, at) => {
      if (refusal(item, at) && item.type === 'skill') {
        const have = s.skills ?? []
        return { take: false, note: have.includes(item.id) ? `has ${skillName(item.id)}` : have.length >= MAX_STAGE_SKILLS ? `${have.length} skills · full` : 'too much guidance' }
      }
      if (at !== on) return null
      const note = item.type === 'skill' ? `+ ${skillName(item.id)}` : item.type === 'model' ? `→ ${modelName(item.choice)}` : '+ prompt'
      return { take: true, note }
    })
  })
  const hint = $derived.by(() => {
    const d = dragging
    if (!d) return ''
    const { item, target, why } = d
    if (item.type === 'stage') return target ? `Drop to add ${stageName(item.kind)} at place ${target.at + 1}.` : why || `Drop on the line to add ${stageName(item.kind)}.`
    if (item.type === 'card') {
      const name = titled(draft.stages[item.from]), to = target ? slotTo(item.from, target.at) : -1
      return !target ? why || `Drop on the line to move ${name}.` : to === item.from ? `Drop here to leave ${name} where it is.` : `Drop to move ${name} to place ${to + 1}.`
    }
    const on = target?.type === 'card' ? target.at : -1, name = on >= 0 ? titled(draft.stages[on]) : ''
    const what = item.type === 'skill' ? skillName(item.id) : item.type === 'model' ? modelName(item.choice) : 'this prompt'
    if (on >= 0 && why) return `${name} cannot take ${what}. ${why}`
    let first: string
    if (item.type === 'skill') first = on >= 0 ? `Drop to give ${name} the skill ${what}.` : `Drop on a stage to give it the skill ${what}.`
    else if (item.type === 'model') first = item.choice === 'other' ? `Drop on a stage to choose another model for it.` : on >= 0 ? `Drop to put ${name} on ${what}.` : `Drop on a stage to put it on ${what}.`
    else {
      first = on >= 0 ? `Drop to add this prompt to ${name}.` : 'Drop on a stage to add this prompt to it.'
    }
    if (item.type !== 'skill') return first
    const others = draft.stages.flatMap((s, at) => at === on || !refusal(item, at) ? [] : [
      (s.skills ?? []).includes(item.id) ? `${titled(s)} already has it.` : (s.skills?.length ?? 0) >= MAX_STAGE_SKILLS ? `${titled(s)} already has ${MAX_STAGE_SKILLS}.` : `${titled(s)} has too much guidance for it.`])
    return [first, ...others].join(' ')
  })
  // The ghost is a copy of what was picked up, put in the drag layer.
  function hold(node: HTMLElement, copy: HTMLElement) { node.replaceChildren(copy) }

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
    drag={phone ? null : hooks} onadd={add} onmodel={choice => void applyModel(choice)} onskill={applySkill} onprompt={applyPrompt} {canskill} />
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
  <h1 class="vh" id={`${uid}-title`} tabindex="-1" bind:this={heading}>Pathway editor</h1>
  {#if phase !== 'ready'}
    <div class="bar"><a class="back" href="/factory/pathways" onclick={back}><span aria-hidden="true">‹</span>&nbsp;Pathways</a></div>
    {#if phase === 'loading'}
      <div class="waiting"><Skeleton label="Reading the pathway…" shape="cards" count={4} height="132px" /></div>
    {:else}
      <div class="failed" role="alert">
        <p><strong>{phase === 'gone' ? 'That pathway is not available.' : 'The pathway could not be read.'}</strong> {phase === 'gone' ? 'It may have been deleted.' : 'Nothing was changed.'}</p>
        {#if phase === 'failed'}<button class="btn" type="button" onclick={loadPathway}>Retry</button>
        {:else}<a class="btn" href="/factory/pathways" onclick={back}>Back to your pathways</a>{/if}
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
    {#if deleted}
      <div class="banner conflict" role="alert">
        <p><strong>This pathway was deleted somewhere else.</strong> Your edits are still on screen and nothing was saved.</p>
        <div class="choices">
          <button type="button" class="btn" onclick={leaveDeleted}>Discard and go to the list</button>
          <button type="button" class="btn primary" disabled={blocked || saving} aria-busy={saving} onclick={save}>Save as a new pathway</button>
        </div>
      </div>
    {/if}
    {#if notice}<p class="banner bad" role="alert"><span class="mark" aria-hidden="true">!</span>{notice}</p>{/if}
    {#if info}<p class="banner" role="status">{info}</p>{/if}

    <div class="body">
      {#if !phone}<aside class="side"><div class="dock" bind:this={dock}>{@render palette()}</div></aside>{/if}
      <div class="main">
        <div class="canvas" bind:this={canvas}>
          <PathwayLine stages={draft.stages} {keys} {selected} drag={hooks} {aims} problems={problems.stages} vertical={phone} onselect={select} onmove={move} onremove={at => void remove(at)} onaddend={() => void focusPalette('stages')} />
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
{#if dragging}
  <div class="drag-layer" aria-hidden="true" inert>
    {#if ghost}
      <div class="ghost" style:width="{ghost.width}px" style:transform="translate({dragging.x - ghost.dx}px, {Math.max(dragging.y - ghost.dy, floor)}px)"><div class="tilt" use:hold={ghost.node}></div></div>
    {/if}
    <!-- Over the ghost, so the gap it will land in is never hidden. -->
    {#if bar}<div class="drop-bar" data-drag-bar style:left="{bar.left}px" style:top="{bar.top}px" style:width="{bar.width}px" style:height="{bar.height}px"></div>{/if}
    {#if hint}<p class="drag-hint" class:phone class:above={hintAt.above} class:side={hintAt.side} data-drag-hint style:left={phone ? undefined : `${hintAt.x}px`} style:top={phone ? undefined : `${hintAt.y}px`} style:max-width={hintAt.side ? `min(560px, ${innerWidth - hintAt.x - 16}px)` : undefined}>{hint}</p>{/if}
  </div>
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
  .back:focus-visible { outline: 2px solid var(--focus); outline-offset: 1px; }
  .names { display: grid; flex: 1 1 320px; min-width: 0; max-width: 560px; gap: 0; }
  .names input {
    width: 100%; min-width: 0; box-sizing: border-box; margin: 0; border: 1px solid transparent; border-radius: 8px; background: transparent; color: var(--ink);
  }
  .names input:hover { border-color: var(--line); }
  .names input:focus-visible { outline: 2px solid var(--focus); outline-offset: 0; background: var(--card); }
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
  .toggle:focus-visible { outline: 2px solid var(--focus); outline-offset: 1px; }
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
  .side { border-right: 1px solid var(--line); background: var(--bg-sunk); }
  /* The palette stays in sight as the page scrolls, and its list scrolls on its own below the tabs and search. */
  .dock { position: sticky; top: 0; display: flex; flex-direction: column; max-height: 100dvh; box-sizing: border-box; padding: 16px; }
  .dock > :global(.palette) { flex: 1 1 auto; min-height: 0; }
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

  @media (pointer: coarse) { .back, .toggle { min-height: 44px; } }
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
  .subbar .description:focus-visible { outline: 2px solid var(--focus); outline-offset: 0; }
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

  /* The drag layer: over everything, never in the way of the pointer. */
  .drag-layer { position: fixed; inset: 0; z-index: 1000; pointer-events: none; }
  .drop-bar { position: fixed; border-radius: 2px; background: var(--accent); box-shadow: 0 0 0 3px var(--accent-wash); }
  .ghost { position: fixed; left: 0; top: 0; }
  .tilt > :global(*) {
    width: 100% !important; margin: 0 !important; opacity: 1 !important; border-color: var(--accent) !important; border-style: solid !important;
    background: var(--card) !important; box-shadow: 0 18px 36px -14px var(--scrim), var(--shadow) !important;
  }
  .drag-hint {
    position: fixed; translate: -50% 0; max-width: min(560px, calc(100vw - 32px)); box-sizing: border-box; margin: 0; padding: 8px 12px; border-radius: 8px;
    background: var(--ink); color: var(--bg); font-size: 14px; line-height: 1.45; text-align: center; box-shadow: 0 12px 30px -12px var(--scrim);
  }
  .drag-hint.above { translate: -50% -100%; }
  .drag-hint.side { translate: 0 -50%; }
  .drag-hint.phone { left: 16px; right: 16px; bottom: calc(16px + var(--bottom-chrome, 0px)); translate: none; max-width: none; }
  :global(html[data-pathway-drag]), :global(html[data-pathway-drag] *) { cursor: grabbing !important; }

  @media (prefers-reduced-motion: no-preference) {
    .tilt { rotate: 2deg; }
    .knob { transition: transform 0.16s var(--ease-out), background-color 0.16s; }
    .sheet[open] { animation: up 220ms var(--ease-out) both; }
    @keyframes up { from { transform: translateY(24px); opacity: 0; } to { transform: none; opacity: 1; } }
  }
</style>
