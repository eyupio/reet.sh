<script lang="ts">
  import { gafferTooltip } from './gafferTooltip'
 import { onMount, tick, untrack } from 'svelte'
 import { shell } from './dialogShell'
 import { UnauthorizedError, addModelFavourite, getProfile, getYoloSetting, listModelFavourites, removeModelFavourite, setModelFavouriteDefault, type Machine, type YoloSetting } from './api'
 import { CATALOGUE } from './catalogue'
 import { defaultLabel, effortChoices, favouriteKey, modelOptions, modelStatus, resolveLaunchDefaults, starLabel, type MachineModels, type ModelFavourite } from './models'
 import { createTerminal, getFolderProject, machineModels, readiness, refreshMachineModels, TerminalError, type FolderProject, type LaunchRequest, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { MAX_PROMPT_BYTES, cleanModel, notStartedText, folderChoices, launchSentence, modelSuggestions, preselect, refusalField, refusalText, rekey, titleFor, toolChoices, toolName, validateDir, validateModel, validatePrompt } from './agentLaunch'
 import { chatToolName, structuredAgent } from './terminal/api'
 import SlotPicker, { type SlotOption } from './launch/SlotPicker.svelte'
 import YoloDialog from './launch/YoloDialog.svelte'
 import { yoloCanStart, type YoloGate } from './launchBar'

 // Start an agent CLI in a new terminal session on one machine. One screen, one press. The page names a tool id, a model, a
 // folder and a prompt; the machine decides the program and its flags. Nothing here is kept in browser storage or the URL, and
 // the prompt lives only in this component: it is cleared when the dialog closes.
 // opener is the element the person pressed to open this, which gets focus back on close.
 let { machines, snapshot, initialAgent = '', initialDir = '', projectId = '', opener = null, onstarted, onleavefailed, onaddmachine, onsignedout, onclose }: {
  /** The machines from /api/v2/terminal/machines, with their readiness and launch capabilities. */
  machines: TerminalMachine[]
  /** The machines' reported folders (from the sync state), or undefined while they are unknown: the home folder and a typed path still work. */
  snapshot: Machine[] | undefined
  /** The machine this was opened from, preselected when it is ready. */
  initialAgent?: string
  /** A folder to start in on that machine (a folder project's), filled in when that machine is the one preselected. */
  initialDir?: string
  /** The folder project the folder belongs to: sent with the start while the machine and folder are still that project's,
   *  so the session is listed under it. */
  projectId?: string
  opener?: HTMLElement | null
  /** Hands over the started session. A page that answers 'leaving' is opening it at another address: the dialog then stays in
   *  its Starting… state, with nothing left to press, until the page unloads. */
  onstarted: (session: TerminalSession) => void | 'leaving'
  /** After 'leaving', the next page did not arrive within a few seconds (a 204, the browser's Stop, a page that will not
   *  load). The dialog has closed; the page may say what started. */
  onleavefailed?: (session: TerminalSession) => void
  /** Opens the add-a-machine flow. The dialog closes first. */
  onaddmachine?: () => void
  /** Hands a lost sign-in to the page, as the other dialogs do. Without it the dialog only says so. */
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 const OTHER = '\u0000other', TYPED = '\u0000typed'
 let dialog = $state<HTMLDialogElement>(), machineSelect = $state<HTMLSelectElement>(), modelButton = $state<HTMLButtonElement>()
 let startButton = $state<HTMLButtonElement>()
 // svelte-ignore state_referenced_locally
 const opening = preselect(machines, initialAgent)
 let agentId = $state(opening.agentId)
 // Said once, while the machine chosen for you is the one shown: why it is not the one you came from.
 const switchedNote = $derived(agentId === opening.agentId ? opening.note : '')
 let tool = $state(''), modelChoice = $state(''), modelOther = $state(''), effort = $state(''), folderChoice = $state(''), typedDir = $state(''), prompt = $state(''), structured = $state(false), sessionTouched = $state(false)
 // An agent, model or thinking level the person chose survives a late project/profile/favourite read for the same agent.
 // svelte-ignore state_referenced_locally
 let explicitTool = $state(CATALOGUE.some(agent => agent.id === initialAgent) ? initialAgent : '')
 let explicitModel = $state(false), explicitEffort = $state(false)
 let attempted = $state(false), busy = $state(false), message = $state(''), failed = $state(false)
 // The status line holds an attempt's validation message only until the person edits a field: it never outlives the mistake it named.
 let invalidMessage = false
 // One attempt: the request id and the exact parameters. Kept while its outcome is unknown, so Retry asks about the same start
 // and a lost reply can never start a second agent. Dropped only when the server has definitely refused it.
 type Operation = { agentId: string; host: string; requestId: string; title: string; launch: LaunchRequest & { tool: string } }
 let operation = $state<Operation | null>(null)
 const life = new AbortController()

 // The project's own defaults (read once) and the account's: resolved with the catalogue's agent gate, never as a list read.
 let projectData = $state<FolderProject | null>(null)
 let accountDefaults = $state<{ defaultTool?: string; defaultModel?: string; defaultEffort?: string } | null>(null)
 // Favourites and the chosen machine's list are helpers only: a failed read changes the status line and nothing else.
 let favourites = $state<ModelFavourite[]>([])
 let favouritesLoadSeq = 0
 let favouriteMutationSeq = 0
 let favouriteQueue: Promise<void> = Promise.resolve()
 let favouriteNote = $state('')
 type ModelRead = { key: string; loading: boolean; machine: MachineModels | null; error: string }
 let modelList = $state<ModelRead>({ key: '', loading: false, machine: null, error: '' })
 let touch = $state(false), pickerOpen = $state(false)

 const machine = $derived(machines.find(m => m.agentId === agentId))
 const choices = $derived(toolChoices(machine))
 // While a request is held or its outcome unknown, the tool is the one it asked for: a machine-list refresh can't change it.
 const chosenTool = $derived(operation ? operation.launch.tool : choices.find(c => c.id === tool && c.available)?.id ?? choices.find(c => c.available)?.id ?? '')
 const availableAgents = $derived(choices.filter(c => c.available).map(c => c.id))
 // Explicit agent > project > account > the favourite marked for the agent > the first available agent. A model and an
 // effort only come with the agent they belong to; a project/account model needs no static hint or machine list.
 const resolvedDefaults = $derived.by(() => {
  const base = resolveLaunchDefaults({ explicitAgent: explicitTool, project: projectData, account: accountDefaults, availableAgents })
  const agent = base.agent && availableAgents.includes(base.agent) ? base.agent : availableAgents[0] ?? ''
  const next = agent === '' ? { agent: '', model: '', effort: '' }
   : agent === base.agent ? base
   : resolveLaunchDefaults({ explicitAgent: agent, project: projectData, account: accountDefaults, availableAgents })
  const favourite = favourites.find(f => f.agentDefault && f.tool === next.agent)
  return favourite
   ? resolveLaunchDefaults({ explicitAgent: next.agent, project: projectData, account: accountDefaults, favouriteDefault: favourite, availableAgents })
   : next
 })
 // A new machine or agent re-resolves the model and thinking to the defaults, and starts the folder afresh: no blank select
 // and no earlier choice coming back for the wrong agent.
 let choiceKey = ''
 // The resolved agent (not the raw radio) is what re-resolves: a late project or profile read that names a different
 // agent must move the choice too, not only its model.
 const desiredKey = $derived(agentId + '\u0000' + resolvedDefaults.agent)
 $effect.pre(() => {
  const key = desiredKey
  const defaults = resolvedDefaults
  untrack(() => {
   // Nothing that would change the held request is reset while it is held.
   if (operation) return
   const next = rekey({ key: choiceKey, model: modelChoice, folder: folderChoice, effort }, key, { model: defaults.model, effort: defaults.effort })
   if (next.key === choiceKey) return
   choiceKey = next.key
   explicitModel = false; explicitEffort = false
   modelChoice = next.model; effort = next.effort ?? ''; folderChoice = next.folder; modelOther = ''; typedDir = ''
   tool = defaults.agent
   // The project's folder is the start for its own machine, including after a machine or agent change.
   const projectDir = agentId === initialAgent ? initialDir : ''
   if (projectDir) {
    const listed = folderChoices(machines.find(m => m.agentId === agentId), snapshot).some(f => f.path === projectDir)
    folderChoice = listed ? projectDir : TYPED
    if (!listed) typedDir = projectDir
   }
  })
 })
 // A late project, profile or favourites read fills the defaults only while the person has not chosen that field.
 $effect(() => {
  const key = desiredKey
  const defaults = resolvedDefaults
  if (!machine || operation) return
  untrack(() => {
   if (key !== choiceKey) return
   if (!explicitModel && modelChoice !== OTHER && modelChoice !== defaults.model) modelChoice = defaults.model
   if (!explicitEffort && effort !== defaults.effort) effort = defaults.effort
  })
 })
 // Any model value the picker gives is sent as chosen; a project/account default needs no static hint.
 const model = $derived(modelChoice === OTHER ? cleanModel(modelOther) : cleanModel(modelChoice))
 const effortOptions = $derived(effortChoices(chosenTool))
 const chatCapable = $derived(!!structuredAgent(machine?.capabilities, chosenTool))
 // OpenCode is a chat by default when the machine offers ACP; the checkbox is
 // the explicit terminal escape hatch. A machine or agent change starts over.
 let chatKey = ''
 $effect(() => {
  const key = agentId + '\u0000' + chosenTool
  if (key !== chatKey) { chatKey = key; sessionTouched = false }
  if (!chatCapable) { sessionTouched = false; structured = false; return }
  if (!sessionTouched) structured = true
 })
 const modelLabel = $derived(modelChoice === OTHER ? cleanModel(modelOther) || 'Other…' : modelChoice || (structured ? 'OpenCode’s configured model' : 'Agent default'))
 const modelKey = $derived(machine && chosenTool ? `${machine.agentId}\u0000${chosenTool}` : '')
 const modelMachine = $derived(modelList.key === modelKey ? modelList.machine : null)
 const modelLoading = $derived(!!modelKey && modelList.key === modelKey && modelList.loading)
 const modelReadError = $derived(modelList.key === modelKey ? modelList.error : '')
 const modelCapable = $derived(!!machine?.ready && !!machine?.capabilities?.models && !machine?.capabilities?.modelsOff)
 const blankModels = (state: MachineModels['state']): MachineModels => ({ tool: chosenTool, state, models: [], more: false, source: '', version: '', fetchedAt: null, checkedAt: null, reason: '', sentence: '' })
 const modelStatusInfo = $derived.by(() => {
  const host = machine?.host ?? ''
  if (!machine) return modelStatus(null, host)
  // The machine's current facts win over a cached list: offline, off and old do not look freshly read.
  if (!machine.ready) return modelStatus(modelMachine ? { ...modelMachine, state: 'offline', sentence: '' } : blankModels('offline'), host)
  if (machine.capabilities?.modelsOff) return modelStatus(modelMachine ? { ...modelMachine, state: 'off', sentence: '' } : blankModels('off'), host)
  if (machine.capabilities && !machine.capabilities.models) return modelStatus(modelMachine ? { ...modelMachine, state: 'update', sentence: '' } : blankModels('update'), host)
  if (!modelMachine) return modelStatus(null, host)
  return modelStatus(modelMachine, host)
 })
 const modelStatusLine = $derived.by(() => {
  if (!modelKey) return ''
  if (modelLoading) return `Reading ${machine?.host || 'the machine'}’s models…`
  const ageWord = modelMachine?.state === 'failed' ? 'Checked' : 'Read'
  const line = [modelReadError || modelStatusInfo.sentence, !modelReadError && modelStatusInfo.age ? `${ageWord} ${modelStatusInfo.age}.` : ''].filter(Boolean).join(' ')
  return [line, favouriteNote].filter(Boolean).join(' ')
 })
 const modelStatusAction = $derived.by(() => {
  if (!modelKey || !modelStatusInfo.canRefresh || !modelCapable) return undefined
  return { label: 'Refresh', onclick: () => void refreshModels(), disabled: modelLoading }
 })
 const folders = $derived(folderChoices(machine, snapshot))
 const dir = $derived(folderChoice === TYPED ? typedDir : folders.some(f => f.path === folderChoice) ? folderChoice : '')
 const promptBytes = $derived(new TextEncoder().encode(prompt).length)
 // Why nothing can start on this machine, in the existing plain readiness words first.
 const blocked = $derived.by(() => {
  if (!machine) return machines.some(m => m.ready) ? 'Choose a machine.' : 'No machine is ready for terminals right now.'
  const why = readiness(machine)
  if (why) return why
  if (!machine.capabilities?.launch) return 'Update reet on this machine to start agents.'
  if (!chosenTool) return 'None of the agents reet can start is installed on this machine.'
  return ''
 })
 const modelError = $derived(modelChoice === OTHER ? (cleanModel(modelOther) === '' ? (attempted ? 'Type a model name, or choose Agent default.' : '') : validateModel(modelOther) ?? '') : '')
 const dirError = $derived(folderChoice === TYPED && (attempted || typedDir !== '') ? validateDir(typedDir) ?? '' : '')
 const promptError = $derived(chosenTool ? validatePrompt(chosenTool, prompt) ?? '' : '')
 const locked = $derived(busy || operation !== null)
 // YOLO: the account setting, the confirmation, and whether this start was confirmed. It is never remembered; changing the
 // machine or the agent drops it, because the confirmation named them.
 let yoloSetting = $state<YoloSetting | null>(null), yoloOpen = $state(false), yoloChosen = $state(false)
 let yoloRadio = $state<HTMLInputElement>()
 // Why YOLO cannot even be offered. A machine that refuses it is named here too, since this dialog has one machine in view.
 const yoloWhy = $derived.by(() => {
  if (!yoloSetting) return 'Reading the account setting…'
  if (!yoloSetting.allowed) return 'An owner or admin can allow YOLO for this account in Settings.'
  if (!chosenTool || !CATALOGUE.find(a => a.id === chosenTool)?.yolo) return 'This agent has no option reet can use for that.'
  if (machine && !machine.capabilities?.yolo) return `${machine.host} doesn’t accept YOLO sessions.`
  return ''
 })
 const yoloGateList = $derived.by((): YoloGate[] => {
  const account = yoloSetting?.allowed === true
  const name = machine?.host || 'That machine'
  const agentOk = !!CATALOGUE.find(a => a.id === chosenTool)?.yolo
  const machineOk = !!machine?.capabilities?.yolo
  return [
   { label: 'Your account allows YOLO', ok: account, detail: account ? (yoloSetting?.changedBy ? `set by ${yoloSetting.changedBy}` : 'set by an owner') : yoloSetting ? 'An owner or admin can allow YOLO for this account in Settings.' : 'Reading the account setting…' },
   { label: `${name} accepts YOLO sessions`, ok: machineOk, detail: machineOk ? 'checked a moment ago' : `${name} doesn’t accept YOLO sessions.` },
   { label: 'This agent has an option reet can use', ok: agentOk, detail: agentOk ? 'the option is recorded with the launch' : 'This agent has no option reet can use for that.' },
   { label: 'Works in the folder you chose', ok: true, warning: true, detail: 'Secrets in that folder are readable to the agent. Use a throwaway worktree if unsure.' },
  ]
 })
 $effect(() => { agentId; chosenTool; untrack(() => { yoloChosen = false }) })
 $effect(() => { if (yoloWhy) untrack(() => { yoloChosen = false }) })
 async function loadYolo() { try { yoloSetting = await getYoloSetting() } catch { yoloSetting = null } }
 function openYolo() { if (locked || yoloWhy || yoloChosen) return; yoloOpen = true }
 function cancelYolo() { yoloOpen = false; yoloChosen = false; void focusAfter(() => yoloRadio) }
 function startYolo() { if (!yoloCanStart(yoloGateList)) return; yoloChosen = true; yoloOpen = false; edited() }
 // While a request is held, the line says what was asked for, not what the current machine list would offer.
 const sentence = $derived(operation ? launchSentence(operation.launch.tool, operation.launch.dir ?? '', operation.host, !!operation.launch.prompt, !!operation.launch.structured) : machine && chosenTool ? launchSentence(chosenTool, dir, machine.host, prompt !== '', structured) : '')

 // The machine's list follows the machine and agent. A read failure is a status line: suggestions, Other and Start stay.
 function failedModels(tool: string, sentence: string, previous: MachineModels | null): MachineModels {
  return previous
   ? { ...previous, state: 'failed', sentence, checkedAt: Date.now() }
   : { tool, state: 'failed', models: [], more: false, source: '', version: '', fetchedAt: null, checkedAt: Date.now(), reason: '', sentence }
 }
 async function readModels(agentId: string, tool: string, key: string) {
  modelList = { key, loading: true, machine: modelList.key === key ? modelList.machine : null, error: '' }
  try {
   const next = await machineModels(agentId, tool, life.signal)
   if (life.signal.aborted || modelList.key !== key) return
   modelList = { key, loading: false, machine: next, error: '' }
  } catch (e) {
   if (life.signal.aborted || modelList.key !== key) return
   if (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
   const sentence = `Couldn’t read ${machine?.host || 'the machine'}’s models just now. Favourites and suggestions still work.`
   modelList = { key, loading: false, machine: failedModels(tool, sentence, modelList.machine), error: sentence }
  }
 }
 $effect(() => {
  const key = modelKey, agentId = machine?.agentId ?? '', tool = chosenTool
  if (!key || !agentId || !tool) return
  if (untrack(() => modelList.key === key && (modelList.loading || modelList.machine))) return
  untrack(() => { void readModels(agentId, tool, key) })
 })
 async function refreshModels() {
  const key = modelKey, agentId = machine?.agentId ?? '', tool = chosenTool
  if (!key || !agentId || !tool || modelList.loading) return
  const before = modelList.key === key ? modelList.machine : null
  modelList = { key, loading: true, machine: before, error: '' }
  try {
   const next = await refreshMachineModels(agentId, tool, life.signal)
   if (life.signal.aborted || modelList.key !== key) return
   // An offline answer keeps the last list on screen; the status line says the machine is offline.
   if (next.state === 'offline' && before) modelList = { key, loading: false, machine: { ...next, models: before.models, fetchedAt: before.fetchedAt }, error: '' }
   else modelList = { key, loading: false, machine: next, error: '' }
  } catch (e) {
   if (life.signal.aborted || modelList.key !== key) return
   if (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
   const sentence = e instanceof TerminalError && e.status === 429 ? 'Slow down a moment, then try again. The last list is still here.'
    : e instanceof TerminalError && e.status === 409 ? 'Another read is already running. Try again in a moment. The last list is still here.'
    : `Couldn’t refresh ${machine?.host || 'the machine'}’s models. The last list is still here.`
   modelList = { key, loading: false, machine: before, error: sentence }
  }
 }
 // Favourites are read once; a star changes the local list first and the API in order, restoring only its own row on failure.
 async function loadFavourites() {
  const seq = ++favouritesLoadSeq
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
   const answer = await Promise.race([
    listModelFavourites(life.signal),
    new Promise<never>((_, no) => { timer = setTimeout(() => no(new Error('timeout')), 10_000) }),
   ])
   if (!life.signal.aborted && seq === favouritesLoadSeq) { favourites = answer.favourites ?? []; favouriteNote = '' }
  } catch (e) {
   if (!life.signal.aborted && (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401)) onsignedout?.(new UnauthorizedError())
   else if (!life.signal.aborted && seq === favouritesLoadSeq) favouriteNote = 'Couldn’t read your favourite models just now.'
  } finally { clearTimeout(timer) }
 }
 async function toggleStar(value: string, starred: boolean) {
  const agent = chosenTool
  if (!agent || !value) return
  const key = favouriteKey(agent, value)
  const beforeEntry = favourites.find(f => favouriteKey(f.tool, f.model) === key) ?? null
  const mutation = ++favouriteMutationSeq
  favouritesLoadSeq++
  favouriteNote = ''
  favourites = starred ? favourites.filter(f => favouriteKey(f.tool, f.model) !== key) : beforeEntry ? favourites : [...favourites, { tool: agent, model: value, agentDefault: false }]
  favouriteQueue = favouriteQueue.then(async () => {
   try {
    if (starred) await removeModelFavourite(agent, value)
    else {
      const answer = await addModelFavourite(agent, value)
      if (!life.signal.aborted && mutation === favouriteMutationSeq && answer?.favourites) favourites = answer.favourites
    }
   } catch (e) {
    if (life.signal.aborted) return
    if (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
    favourites = favourites.filter(f => favouriteKey(f.tool, f.model) !== key)
    if (beforeEntry) favourites = [...favourites, beforeEntry]
    favouriteNote = (e as { message?: string })?.message ? (e as Error).message : 'Couldn’t update your favourites just now. Try again.'
   }
  })
  await favouriteQueue
 }
 async function toggleDefault(value: string, isDefault: boolean) {
  const agent = chosenTool
  if (!agent || !value) return
  const key = favouriteKey(agent, value)
  const beforeEntry = favourites.find(f => favouriteKey(f.tool, f.model) === key) ?? null
  const mutation = ++favouriteMutationSeq
  favouritesLoadSeq++
  favouriteNote = ''
  favourites = [...favourites.filter(f => !(isDefault && f.tool === agent && f.agentDefault)), { tool: agent, model: value, agentDefault: !isDefault }]
   .filter((f, i, all) => all.findIndex(x => favouriteKey(x.tool, x.model) === favouriteKey(f.tool, f.model)) === i)
  favouriteQueue = favouriteQueue.then(async () => {
   try {
    if (!isDefault && !beforeEntry) await addModelFavourite(agent, value)
    const answer = await setModelFavouriteDefault(agent, isDefault ? '' : value)
    if (!life.signal.aborted && mutation === favouriteMutationSeq && answer?.favourites) favourites = answer.favourites
   } catch (e) {
    if (life.signal.aborted) return
    if (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
    if (beforeEntry) favourites = [...favourites.filter(f => favouriteKey(f.tool, f.model) !== key), beforeEntry]
    else favourites = favourites.filter(f => favouriteKey(f.tool, f.model) !== key)
    favouriteNote = (e as { message?: string })?.message ? (e as Error).message : 'Couldn’t update the agent default just now. Try again.'
   }
  })
  await favouriteQueue
 }
 function modelOptionsFor(): SlotOption[] {
  if (!chosenTool) return []
  return modelOptions({
   tool: chosenTool, current: model, machine: modelMachine, favourites, recent: [], suggestions: modelSuggestions(chosenTool),
   selectedModel: model, host: machine?.host ?? '', agentName: toolName(chosenTool),
  }).map(o => o.value === '' ? o : {
   ...o, starLabel: starLabel(chosenTool, o.value, !!o.starred), onstar: () => { void toggleStar(o.value, !!o.starred) },
   defaultLabel: defaultLabel(chosenTool, o.value, !!o.isDefault), ondefault: () => { void toggleDefault(o.value, !!o.isDefault) },
  })
 }
 function openPicker() { if (locked || !chosenTool) return; pickerOpen = true }
 function pickModel(value: string) { explicitModel = true; modelChoice = value; modelOther = ''; edited() }
 function selectTool(next: string) { if (locked) return; explicitTool = next; tool = next; edited() }

 // The project is read once for its own defaults; a failure falls back to the account and favourites.
 async function loadProject() {
  if (!projectId) return
  try { projectData = await getFolderProject(projectId, life.signal) }
  catch (e) {
   if (!life.signal.aborted && (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401)) onsignedout?.(new UnauthorizedError())
   else if (!life.signal.aborted) projectData = null
  }
 }
 async function loadAccount() {
  try {
   const answer = await getProfile()
   if (life.signal.aborted) return
   accountDefaults = { defaultTool: answer.profile.defaultTool, defaultModel: answer.profile.defaultModel, defaultEffort: answer.profile.defaultEffort }
  } catch (e) {
   if (!life.signal.aborted && e instanceof UnauthorizedError) onsignedout?.(e)
   else if (!life.signal.aborted) accountDefaults = null
  }
 }

 function shutdown(returnFocus: boolean) {
  life.abort()
  pickerOpen = false
  prompt = ''
  operation = null
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 // While a request is in flight the dialog stays: closing would forget a start that may still happen. The 15-second abort ends the wait.
 function close() { if (busy) return; shutdown(true); onclose() }
 // The hand-off is read before onclose: a parent may derive it from the state that onclose clears.
 function addMachine() { if (busy) return; const add = onaddmachine; shutdown(false); onclose(); add?.() }

 const LOST_TAIL = 'Check your sessions before starting another. If you close this and start again, a second agent may start.'
 const LOST = `We didn't hear back, so the agent may have started. ${LOST_TAIL}`
 // A refusal the server gave for certain: nothing started, so the person may change the fields and start again.
 const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 422].includes(e.status)
 const field = (name: string) => dialog?.querySelector<HTMLElement>(`#${CSS.escape(`${id}-${name}`)}`)
 // Focus after the fieldset and button are enabled again, so it never drops to the page.
 async function focusAfter(target: () => HTMLElement | null | undefined) { await tick(); if (!life.signal.aborted) (target() ?? startButton)?.focus() }
 function focusFor(where: string) {
  if (where === 'machine') return field('machine')
  if (where === 'tool') return dialog?.querySelector<HTMLElement>(`input[name="${CSS.escape(`${id}-tool`)}"]:checked`)
  if (where === 'model') return modelButton
  if (where === 'folder') return folderChoice === TYPED ? field('dir') : field('folder')
  if (where === 'prompt') return field('prompt')
  return startButton
 }
 async function start() {
  if (busy || life.signal.aborted) return
  pickerOpen = false
  attempted = true
  if (!operation) {
   if (blocked || !machine) return
   // The errors are derived, so they already count this attempt; no await here, or a second press could slip past the busy guard.
   const invalid = modelError ? ['model', modelError] : dirError ? ['dir', dirError] : promptError ? ['prompt', promptError] : null
   if (invalid) { failed = true; message = invalid[1]; invalidMessage = true; field(invalid[0])?.focus(); return }
   const launch: LaunchRequest & { tool: string } = { tool: chosenTool }
   if (structured) { launch.structured = true; if (chosenTool === 'opencode') launch.trust = true }
   if (model) launch.model = model
   if (!structured && effort && effortOptions.length > 1) launch.effort = effort as NonNullable<LaunchRequest['effort']>
   if (dir) launch.dir = dir
   if (prompt) launch.prompt = prompt
   if (yoloChosen && !yoloWhy) launch.yolo = true
   if (projectId && machine.agentId === initialAgent && dir === initialDir) launch.projectId = projectId
   operation = { agentId: machine.agentId, host: machine.host, requestId: crypto.randomUUID(), title: titleFor(chosenTool, dir), launch }
  }
  const op = operation
  busy = true; failed = false; invalidMessage = false
  message = `Starting ${toolName(op.launch.tool)} on ${op.host}… this can take a few seconds.`
  try {
   const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(structured ? 60000 : 15000)]), op.launch)
   if (life.signal.aborted) return
   // Answered, but the machine did not start it: nothing is running, so this request is done and the next Start is a new one.
   const notStarted = notStartedText(session.state)
   if (notStarted) {
    operation = null; failed = true; message = notStarted; busy = false
    void focusAfter(() => startButton)
    return
   }
   const started = onstarted
   // The request is done, so nothing of it is kept, and the dialog's own work stops before the page takes the session.
   life.abort(); prompt = ''; operation = null
   if (started(session) === 'leaving') {
    // The page is going to the session. Stay Starting… so a second press, Close or Escape does nothing in the moment before
    // the next page arrives.
    message = `Opening ${session.title}…`
    leave(session)
    return
   }
   shutdown(false)
   onclose()
  } catch (e) {
   if (life.signal.aborted) return
   failed = true
   if (definite(e)) {
    const err = e as TerminalError
    operation = null
    message = refusalText(err.status, err.code, err.message)
    busy = false
    if (err.status === 401 && onsignedout) { const signedOut = onsignedout; shutdown(false); onclose(); signedOut(new UnauthorizedError()); return }
    void focusAfter(() => focusFor(refusalField(err.status, err.code)))
   } else {
    message = e instanceof TerminalError && e.status === 503 ? `${refusalText(503)} ${LOST_TAIL}` : LOST
    busy = false
    void focusAfter(() => startButton)
   }
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 // While the page is leaving: if it never goes, let go after a few seconds and hand the session back, so a modal never stays
 // with no way out; if the browser brings the page back from its cache on Back, close at once. Once the page has gone
 // (pagehide), the timer stops: a page restored later is not a failed start.
 const LEAVE_MS = 5000
 let stopLeaving = () => {}
 function leave(session: TerminalSession) {
  const failed = onleavefailed
  const done = () => { clearTimeout(timer); removeEventListener('pagehide', gone); removeEventListener('pageshow', back); stopLeaving = () => {} }
  const letGo = () => { done(); shutdown(true); onclose(); failed?.(session) }
  const gone = () => clearTimeout(timer)
  const back = (e: PageTransitionEvent) => { if (!e.persisted) return; done(); shutdown(true); onclose() }
  const timer = setTimeout(letGo, LEAVE_MS)
  addEventListener('pagehide', gone)
  addEventListener('pageshow', back)
  stopLeaving = done
 }

 function edited() { if (invalidMessage) { invalidMessage = false; message = ''; failed = false } }

 onMount(() => {
  dialog?.showModal()
  machineSelect?.focus()
  // A phone opens the picker as the shared bottom sheet; a mouse gets the popover.
  const coarse = matchMedia('(pointer: coarse)')
  const readPointer = () => { touch = coarse.matches }
  readPointer()
  coarse.addEventListener?.('change', readPointer)
  // The profile is read even without a project, so a machine card or the workspace gets the account defaults.
  void loadFavourites()
  void loadAccount()
  void loadYolo()
  if (projectId) void loadProject()
  return () => { stopLeaving(); life.abort(); prompt = ''; coarse.removeEventListener?.('change', readPointer) }
 })
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header data-shell-head>
  <div>
   <p class="eyebrow" data-shell-aside>Sessions</p>
   <h2 id={`${id}-title`}>Start an agent</h2>
   <p class="lede" data-shell-aside>Right then: pick a machine, an agent and a folder. It starts in a new terminal session you can come back to.</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={busy} aria-label="Close" data-shell-close>Close ×</button>
 </header>
 <form class="sheet" oninput={edited} onsubmit={(event) => { event.preventDefault(); void start() }}>
  <!-- While everything is locked there is nothing to tab to in here, so the scrolling part takes focus itself and can be read. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="body" data-shell-body tabindex={locked ? 0 : undefined} role={locked ? 'region' : undefined} aria-label={locked ? 'Your choices' : undefined}>
  <fieldset disabled={locked}>
   <div class="field">
    <label for={`${id}-machine`}>Machine</label>
    <select id={`${id}-machine`} bind:this={machineSelect} bind:value={agentId} aria-describedby={[switchedNote ? `${id}-switched` : '', blocked ? `${id}-blocked` : ''].filter(Boolean).join(' ') || undefined}>
     {#if !machine}<option value="" disabled>Choose a machine</option>{/if}
     {#each machines as item (item.agentId)}<option value={item.agentId} disabled={!item.ready}>{item.host}{item.ready ? '' : ' · not ready'}</option>{/each}
    </select>
    {#if switchedNote}<p class="hint" id={`${id}-switched`}>{switchedNote}</p>{/if}
    {#if blocked}<p class="reason" id={`${id}-blocked`}>{blocked}</p>{/if}
    {#if onaddmachine}<button type="button" class="linklike" onclick={addMachine}>Not listed? Add a machine</button>{/if}
   </div>

   <fieldset class="tools">
    <legend>Agent</legend>
    {#each choices as choice (choice.id)}
     <div class="tool" class:off={!choice.available}>
      <input type="radio" id={`${id}-tool-${choice.id}`} name={`${id}-tool`} value={choice.id} checked={choice.id === chosenTool} disabled={!choice.available} onchange={() => selectTool(choice.id)} aria-describedby={choice.available ? undefined : `${id}-why-${choice.id}`} />
      <span><label for={`${id}-tool-${choice.id}`}>{choice.name}</label>{#if !choice.available}<small id={`${id}-why-${choice.id}`} class:vh={!machine?.capabilities?.launch}>{choice.reason}</small>{/if}</span>
     </div>
    {/each}
   </fieldset>

   {#if chosenTool === 'opencode' && machine && !chatCapable}
    <p class="hint">This machine does not currently offer Gaffer for OpenCode. This starts the OpenCode TUI instead.</p>
   {/if}

   {#if chatCapable}
    <div class="field chat-choice">
     <label for={`${id}-session`}>Session</label>
     <button type="button" use:gafferTooltip aria-label="About Gaffer">About Gaffer</button>
     <select use:gafferTooltip={structured} id={`${id}-session`} value={structured ? 'reet' : 'terminal'} onchange={(event) => { structured = (event.currentTarget as HTMLSelectElement).value === 'reet'; sessionTouched = true; edited() }}>
      <option value="reet">Gaffer</option>
      <option value="terminal">{toolName(chosenTool)} TUI</option>
     </select>
     {#if structured}
      {#if chosenTool === 'claude'}
       <p class="hint">In Gaffer, Claude Code asks before it edits a file or runs a command. Only your own Claude settings apply; this folder's repository settings and hooks are ignored. Its live history stays in memory and ends when the machine connection does.</p>
      {:else}
       <p class="hint">In Gaffer, OpenCode asks before it edits a file or runs a command. Any plugins in this folder's <code>.opencode</code> run when it starts, so use a folder you trust. Its live history stays in memory and ends when the machine connection does.</p>
      {/if}
     {:else}
      <p class="hint">Starts the {toolName(chosenTool)} TUI in a terminal session; the agent's own prompts apply.</p>
     {/if}
    </div>
   {/if}

   <div class="field">
    <label for={`${id}-model`}>Model <span class="optional">optional</span></label>
    <button id={`${id}-model`} type="button" class="picker" bind:this={modelButton} disabled={!chosenTool}
      aria-haspopup="dialog" aria-label={`Model: ${modelLabel}`} onclick={openPicker}>
     <span class="value">{modelLabel}</span><span class="chev" aria-hidden="true">⌄</span>
    </button>
    {#if modelChoice === OTHER && modelError}<p class="reason" id={`${id}-model-error`}>{modelError}</p>{/if}
    <p class="hint">Suggestions are hints. The agent says itself if it doesn't know a model.</p>
    {#if structured}<p class="hint">Gaffer uses this model. Leave it at {chatToolName(chosenTool)}’s configured model to keep {chatToolName(chosenTool)}’s own; its thinking comes from {chatToolName(chosenTool)}.</p>{/if}
   </div>

   {#if !structured && effortOptions.length > 1}
    <div class="field">
     <label for={`${id}-effort`}>Thinking</label>
     <select id={`${id}-effort`} bind:value={effort} onchange={() => { explicitEffort = true; edited() }} disabled={!chosenTool}>
      {#each effortOptions as value (value || 'default')}<option value={value}>{value || 'default'}</option>{/each}
     </select>
    </div>
   {/if}

   <div class="field">
    <label for={`${id}-folder`}>Folder</label>
    <select id={`${id}-folder`} bind:value={folderChoice}>
     {#each folders as folder (folder.path)}<option value={folder.path}>{folder.label}</option>{/each}
     <option value={TYPED}>Type a path…</option>
    </select>
    {#if folderChoice === TYPED}
     <label for={`${id}-dir`} class="sub">Folder path</label>
     <input id={`${id}-dir`} bind:value={typedDir} autocomplete="off" spellcheck="false" placeholder="/home/you/work/site" aria-invalid={dirError ? 'true' : undefined} aria-describedby={dirError ? `${id}-dir-error ${id}-dir-hint` : `${id}-dir-hint`} />
     {#if dirError}<p class="reason" id={`${id}-dir-error`}>{dirError}</p>{/if}
     <p class="hint" id={`${id}-dir-hint`}>The machine decides whether it can start there. It must be inside the home folder it was enrolled with.</p>
    {/if}
   </div>

   <div class="field">
    <label for={`${id}-prompt`}>Opening prompt <span class="optional">optional</span></label>
    <textarea id={`${id}-prompt`} bind:value={prompt} rows="3" disabled={!chosenTool} aria-invalid={promptError ? 'true' : undefined} aria-describedby={`${id}-prompt-count${promptError ? ` ${id}-prompt-error` : ''}`}></textarea>
    <p class="hint count" id={`${id}-prompt-count`} class:over={promptBytes > MAX_PROMPT_BYTES}>{promptBytes.toLocaleString('en-GB')} of {MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes. Secrets don't belong here.</p>
    {#if promptError}<p class="reason" id={`${id}-prompt-error`}>{promptError}</p>{/if}
   </div>

   <!-- Approvals: Ask me is the default. YOLO opens a confirmation first and is never remembered. -->
   <div class="field approvals" role="radiogroup" aria-labelledby={`${id}-approvals-label`} aria-describedby={`${id}-approvals-why`}>
    <span class="legend" id={`${id}-approvals-label`}>Approvals</span>
    <div class="approval-choices">
     <label class:on={!yoloChosen}><input type="radio" name={`${id}-approvals`} value="ask" checked={!yoloChosen} disabled={locked} onchange={() => { yoloChosen = false; edited() }} />Ask me</label>
     <label class:on={yoloChosen} class:off={!!yoloWhy} title={yoloWhy || 'Starts only after a confirmation.'}>
      <input type="radio" name={`${id}-approvals`} value="yolo" bind:this={yoloRadio} checked={yoloChosen} disabled={locked || !!yoloWhy} onclick={(event) => { if (!yoloChosen) { event.preventDefault(); openYolo() } }} onchange={(event) => { if (!yoloChosen) { (event.currentTarget as HTMLInputElement).checked = false; openYolo() } }} />YOLO
     </label>
    </div>
    <p class="hint" id={`${id}-approvals-why`}>{yoloWhy || (yoloChosen ? 'This session will not wait for you to approve commands or file changes.' : 'Starts only after a confirmation. Each use is audited.')}</p>
   </div>
  </fieldset>

  {#if sentence}<p class="sentence">{sentence}</p>{/if}
  </div>
  <footer data-shell-foot>
   <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
   <small data-shell-aside>reet.sh never sends a command line. The machine starts the agent itself.</small>
   <button class="btn primary" type="submit" disabled={busy || (!operation && !!blocked)} aria-busy={busy} bind:this={startButton}>{busy ? 'Starting…' : operation ? 'Try the same request again' : 'Start'}</button>
  </footer>
 </form>
 {#if yoloOpen && machine}
  <YoloDialog gates={yoloGateList} tool={chosenTool} {structured} machine={machine.host} oncancel={cancelYolo} onstart={startYolo} />
 {/if}
 {#if pickerOpen}
  <SlotPicker label="Model" options={modelOptionsFor()} value={model} sheet={touch} opener={modelButton ?? null}
   filter="Filter models" status={modelStatusLine} statusAction={modelStatusAction}
   other={{ label: 'Other…', field: 'Model name', validate: validateModel }}
   onpick={pickModel} onclose={() => { pickerOpen = false }} />
 {/if}
</dialog>

<style>
 dialog { width: min(640px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr); }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent-text); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 /* The fields scroll; the footer with the status line and Start sits below them, outside the scrolling part, so it never covers a field. */
 .sheet { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
 .body { display: grid; gap: 16px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: grid; gap: 18px; }
 .field { display: grid; gap: 6px; min-width: 0; }
 label, legend { font-size: .85rem; font-weight: 600; }
 label.sub { font-weight: 500; margin-top: 4px; }
 .optional { color: var(--muted); font-weight: 400; font-size: .78rem; }
 select, input:not([type=radio]), textarea { min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 .picker { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); text-align: left; background: var(--bg); border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
 .picker:disabled { opacity: .55; cursor: default; }
 .picker .value { min-width: 0; overflow-wrap: anywhere; }
 .picker .chev { flex: none; color: var(--muted); }
 /* A phone zooms into a field under 16 px when it is focused; `font: inherit` above would otherwise beat app.css's rule. */
 @media (max-width: 560px) { select, input:not([type=radio]), textarea, .picker { font-size: 16px; } }
 textarea { resize: vertical; min-height: 80px; }
 [aria-invalid='true'] { border-color: var(--bad); }
 .approvals .legend { font-size: .85rem; }
 .approval-choices { display: flex; gap: 8px; flex-wrap: wrap; }
 .approval-choices label { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font-size: .85rem; cursor: pointer; }
 .approval-choices label.on { border-color: var(--accent); background: var(--tint); }
 .approval-choices label.off { opacity: .55; cursor: not-allowed; }
 .approval-choices label:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
 .tools { gap: 6px; }
 .tools legend { margin-bottom: 6px; padding: 0; }
 .tool { display: flex; align-items: flex-start; gap: 10px; padding: 9px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
 .tool:has(input:checked) { border-color: var(--accent); }
 .tool input { flex: none; margin: 3px 0 0; accent-color: var(--accent); }
 .tool span { display: grid; gap: 2px; min-width: 0; }
 .tool label { font-size: .88rem; font-weight: 600; cursor: pointer; }
 .tool small { color: var(--muted); font-size: .78rem; }
 /* Only the name is dimmed; the reason beside it is text to read and keeps its full contrast. */
 .tool.off label { cursor: default; color: var(--muted); font-weight: 500; }
 .hint, small { margin: 0; color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .count.over { color: var(--bad); }
 .reason { margin: 0; color: var(--bad); font-size: .83rem; line-height: 1.5; }
 .chat-choice code { font-size: .9em; }
 .sentence { margin: 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); font-size: .85rem; line-height: 1.5; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 .linklike { justify-self: start; padding: 4px 0; min-height: 32px; font: inherit; font-size: .83rem; color: var(--accent-text); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 /* Always in view below the scrolling fields, so Start is reachable on a short screen. */
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px 22px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } }
</style>
