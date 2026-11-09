<script lang="ts">
  import { gafferTooltip } from './gafferTooltip'
 import { onMount, tick, untrack } from 'svelte'
 import { shell } from './dialogShell'
 import { UnauthorizedError, addModelFavourite, getProfile, getYoloSetting, listModelFavourites, removeModelFavourite, saveProfile, setModelFavouriteDefault, type Machine, type Skill, type YoloSetting } from './api'
 import { useSkillPrompt } from './install'
 import { insertSkillPrompt } from './skillPrompt'
 import SkillPromptPicker from './home/SkillPromptPicker.svelte'
 import PromptPicker from './prompts/PromptPicker.svelte'
 import InlinePromptMenu from './prompts/InlinePromptMenu.svelte'
 import { readPromptLibrary, type PromptLibrary } from './prompts/library'
 import { promptTrigger, replacePromptTrigger } from './terminal/prompts'
 import type { TerminalPrompt } from './terminal/api'
 import { CATALOGUE } from './catalogue'
 import { launchEfforts, effortHelp } from './terminal/launchOptions'
 import { claudeAdapterHint, CLAUDE_ADAPTER_INSTALL } from './claudeAdapter'
 import { KNOWN_SOURCES, resolveStartSource, sourceLabel } from './clineSource'
 import { defaultLabel, favouriteKey, modelOptions, modelStatus, resolveLaunchDefaults, starLabel, type MachineModels, type ModelFavourite } from './models'
 import { createTerminal, getFolderProject, launchUpload, machineModels, readiness, refreshMachineModels, refusalSentence, TerminalError, type FolderProject, type LaunchRequest, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { formatImageSize, imageFiles, imagePrompt, MAX_IMAGES, MAX_IMAGE_TOTAL_BYTES, validateImage } from './terminal/upload'
 import { rememberUpload } from './terminal/thumbs'
 import { MAX_PROMPT_BYTES, cleanModel, notStartedText, folderChoices, launchSentence, modelSuggestions, preselect, refusalField, refusalText, rekey, promptTitle, titleOriginFor, titleFor, toolChoices, toolName, validateDir, validateModel, validatePrompt } from './agentLaunch'
 import { projectSlug } from './launchBar'
 import { chatNeedsTrust, chatToolName, structuredAgent } from './terminal/api'
 import SlotPicker, { type SlotOption } from './launch/SlotPicker.svelte'
 import YoloDialog from './launch/YoloDialog.svelte'
 import { yoloCanStart, type YoloGate } from './launchBar'

 // Start an agent CLI in a new terminal session on one machine. One screen, one press. The page names a tool id, a model, a
 // folder and a prompt; the machine decides the program and its flags. Nothing here is kept in browser storage or the URL, and
 // the prompt lives only in this component: it is cleared when the dialog closes.
 // opener is the element the person pressed to open this, which gets focus back on close.
 let { machines, snapshot, projects = [], initialAgent = '', projectId = '', opener = null, onstarted, onleavefailed, onaddmachine, onsignedout, onclose }: {
  /** The machines from /api/v2/terminal/machines, with their readiness and launch capabilities. */
  machines: TerminalMachine[]
  /** The machines' reported folders (from the sync state), or undefined while they are unknown: the home folder and a typed path still work. */
  snapshot: Machine[] | undefined
  /** The folder projects the person can choose. A project fills in its machine and supplies the folder, so the folder
   *  fields only appear under the explicit "No project" choice. */
  projects?: FolderProject[]
  /** The machine an action named, preselected as given (even when it is not ready, so its own sentence shows). Empty when
   *  the dialog was opened without naming a machine: the choice is then the person's. */
  initialAgent?: string
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
 const OTHER = '\u0000other', TYPED = '\u0000typed', NO_PROJECT = '\u0000folder'
 let dialog = $state<HTMLDialogElement>(), projectSelect = $state<HTMLSelectElement>(), machineSelect = $state<HTMLSelectElement>(), modelButton = $state<HTMLButtonElement>(), effortSelect = $state<HTMLSelectElement>()
 let startButton = $state<HTMLButtonElement>()
 // The project is a required choice. An action that starts from a project opens with it already chosen; every other
 // opening starts on the placeholder, and "No project" is an explicit choice that reveals the folder fields.
 // svelte-ignore state_referenced_locally
 const openingProject = projectId ? projects.find(p => p.id === projectId) : undefined
 // svelte-ignore state_referenced_locally
 let projectSel = $state(projectId)
 const pickedProject = $derived(projects.find(p => p.id === projectSel))
 // svelte-ignore state_referenced_locally
 const opening = preselect(machines, openingProject?.host || initialAgent || '')
 let agentId = $state(opening.agentId)
 const usingProject = $derived(!!pickedProject)
 const folderVisible = $derived(projectSel === NO_PROJECT)
 let tool = $state(''), modelChoice = $state(''), modelOther = $state(''), effort = $state(''), folderChoice = $state(''), typedDir = $state(''), prompt = $state(''), structured = $state(false), sessionTouched = $state(false)
 // Images pasted or picked before a start live only in this dialog; each is uploaded to the machine as the
 // session starts and its path joins the opening prompt.
 type Attachment = { id: number; file: File; preview: string }
 let attachments = $state<Attachment[]>([]), attachmentSeq = 0
 // An agent, model or thinking level the person chose survives a late project/profile/favourite read for the same agent.
 // svelte-ignore state_referenced_locally
 let explicitTool = $state(CATALOGUE.some(agent => agent.id === initialAgent) ? initialAgent : '')
 let explicitModel = $state(false), explicitEffort = $state(false)
 // The Cline billing source: the person's own choice (an empty one is Cline's own), kept apart from the saved default.
 let sourceChoice = $state(''), explicitSource = $state(false), sourceBusy = $state(false), sourceNote = $state('')
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
 let accountDefaults = $state<{ defaultTool?: string; defaultModel?: string; defaultEffort?: string; defaultClineSource?: string; defaultBaseBranch?: string; worktreesByDefault?: boolean } | null>(null)
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
 // A Cline chat on a machine that negotiated acp-provider-v1 can start on a chosen billing source. The default model follows
 // the saved default source, so the chosen source feeds the defaults below.
 const sourceShown = $derived(chosenTool === 'cline' && structured && !!machine?.capabilities?.acpProvider)
 // A source reet has no label for still applies: a chat's star saves whatever id Cline reports, and this dialog cannot know
 // what the machine's Cline offers, so a saved id is shown as given. Cline's own refusal at launch is the one that says no.
 const sourceOptions = $derived.by(() => {
  const saved = accountDefaults?.defaultClineSource ?? ''
  return saved && !KNOWN_SOURCES.some(choice => choice.id === saved) ? [...KNOWN_SOURCES, { id: saved, label: sourceLabel(saved) }] : [...KNOWN_SOURCES]
 })
 const startSource = $derived(resolveStartSource({ explicit: explicitSource ? sourceChoice : undefined, saved: accountDefaults?.defaultClineSource, offered: sourceOptions.map(choice => choice.id) }))
 const chosenSource = $derived(startSource.source)
 const sourceIsDefault = $derived(!!chosenSource && chosenSource === (accountDefaults?.defaultClineSource ?? ''))
 // Explicit agent > project > account > the favourite marked for the agent > the first available agent. A model and an
 // effort only come with the agent they belong to; a project/account model needs no static hint or machine list.
 const resolvedDefaults = $derived.by(() => {
  const chosen = sourceShown ? chosenSource : undefined
  const base = resolveLaunchDefaults({ explicitAgent: explicitTool, project: projectData, account: accountDefaults, availableAgents, chosenSource: chosen })
  const agent = base.agent && availableAgents.includes(base.agent) ? base.agent : availableAgents[0] ?? ''
  const next = agent === '' ? { agent: '', model: '', effort: '' }
   : agent === base.agent ? base
   : resolveLaunchDefaults({ explicitAgent: agent, project: projectData, account: accountDefaults, availableAgents, chosenSource: chosen })
  const favourite = favourites.find(f => f.agentDefault && f.tool === next.agent)
  return favourite
   ? resolveLaunchDefaults({ explicitAgent: next.agent, project: projectData, account: accountDefaults, favouriteDefault: favourite, availableAgents, chosenSource: chosen })
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
   explicitModel = false; explicitEffort = false; explicitSource = false; sourceChoice = ''; sourceNote = ''
   modelChoice = next.model; effort = next.effort ?? ''; folderChoice = next.folder; modelOther = ''; typedDir = ''
   tool = defaults.agent
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
 const effortOptions = $derived(launchEfforts(chosenTool, machine?.capabilities, structured))
 const effortError = $derived(effort && !effortOptions.includes(effort) ? 'Choose an effort offered by this machine, or Agent default.' : '')
 const effortHint = $derived(effortHelp(chosenTool, machine?.capabilities, structured))
 // A chat takes an effort only when this agent's descriptor offers it. Cline's terminal thinking choice cannot be sent to
 // its ACP session, so switching to Gaffer clears it rather than blocking Start with an option the chat does not have.
 $effect(() => {
  if (structured && effort && !effortOptions.includes(effort)) effort = ''
 })
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
 const modelLabel = $derived(modelChoice === OTHER ? cleanModel(modelOther) || 'Other…' : modelChoice || (structured ? `${chatToolName(chosenTool)}’s configured model` : 'Agent default'))
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
 const adHocDir = $derived(folderChoice === TYPED ? typedDir : folders.some(f => f.path === folderChoice) ? folderChoice : '')
 const dir = $derived(usingProject ? pickedProject!.dir : folderVisible ? adHocDir : '')
 const promptBytes = $derived(new TextEncoder().encode(prompt).length)
 // Why nothing can start, in the order the choices are made: the project first, then the machine it runs on.
 const projectBlocked = $derived(!projectSel ? 'Choose a project, or choose No project to start in a folder.' : projectSel !== NO_PROJECT && !pickedProject ? 'Choose a project.' : '')
 const blocked = $derived.by(() => {
  if (!machine) return machines.some(m => m.ready) ? 'Choose a machine.' : 'No machine is ready for terminals right now.'
  const why = readiness(machine)
  if (why) return why
  if (!machine.capabilities?.launch) return 'Update reet on this machine to start agents.'
  if (!chosenTool) return 'None of the agents reet can start is installed on this machine.'
  return ''
 })
 const modelError = $derived(modelChoice === OTHER ? (cleanModel(modelOther) === '' ? (attempted ? 'Type a model name, or choose Agent default.' : '') : validateModel(modelOther) ?? '') : '')
 const dirError = $derived(folderVisible && folderChoice === TYPED && (attempted || typedDir !== '') ? validateDir(typedDir) ?? '' : '')
 const promptError = $derived(chosenTool ? validatePrompt(chosenTool, prompt) ?? '' : '')
 const locked = $derived(busy || operation !== null)
 // Why attached images cannot be sent: the machine has to accept uploads. A machine with none listed is an older reet.
 const imageReason = $derived.by(() => {
  if (!attachments.length) return ''
  if (!machine?.capabilities) return ''
  if (!machine.capabilities.uploads) return machine.capabilities.uploadsOff ? 'Image uploads are off on this machine.' : 'Update reet on that machine to attach images.'
  return ''
 })
 function addImages(files: File[]) {
  if (locked || !files.length) return
  const accepted: Attachment[] = [], reasons: string[] = []
  let count = attachments.length, bytes = attachments.reduce((total, item) => total + item.file.size, 0)
  for (const file of files) {
   if (count >= MAX_IMAGES) { reasons.push(`Attach at most ${MAX_IMAGES} images.`); break }
   if (bytes + file.size > MAX_IMAGE_TOTAL_BYTES) { reasons.push('Those images are larger than 50 MiB together.'); break }
   const check = validateImage({ type: file.type, size: file.size })
   if (!check.ok) { reasons.push(check.reason === 'empty' ? 'That image is empty.' : check.reason === 'size' ? 'That image is larger than 5 MiB.' : 'Attach a PNG, JPEG, GIF or WebP image.'); continue }
   accepted.push({ id: ++attachmentSeq, file, preview: URL.createObjectURL(file) })
   count++; bytes += file.size
  }
  if (accepted.length) attachments = [...attachments, ...accepted]
  message = reasons.length === 1 ? reasons[0] : reasons.length ? `${reasons.length} images were refused. ${reasons[0]}` : ''
 }
 function removeImage(id: number) {
  const item = attachments.find(a => a.id === id)
  if (item) URL.revokeObjectURL(item.preview)
  attachments = attachments.filter(a => a.id !== id)
  message = ''
 }
 function pasted(event: ClipboardEvent) {
  const files = imageFiles(event.clipboardData?.items)
  if (!files.length) return
  event.preventDefault(); addImages(files)
 }
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
 // Choosing a project moves to its machine and borrows its defaults. "No project" is an explicit choice that keeps the
 // machine as it is and reveals the folder fields. A machine change leaves a project that belongs elsewhere, so the
 // project is chosen again rather than silently starting on the wrong one.
 const projectReady = (item: FolderProject) => machines.some(m => m.agentId === item.host && m.ready)
 function pickProject(next: string) {
  if (locked) return
  projectSel = next
  folderChoice = ''; typedDir = ''
  if (next === NO_PROJECT) { void loadProject(''); edited(); return }
  const item = projects.find(p => p.id === next)
  void loadProject(next)
  if (!item) { edited(); return }
  if (!projectReady(item)) { projectSel = ''; void loadProject(''); edited(); return }
  agentId = item.host
  edited()
 }
 function pickMachine(next: string) {
  if (locked) return
  agentId = next
  if (pickedProject && pickedProject.host !== next) {
   projectSel = ''
   projectReadSeq++
   projectData = null
  }
  edited()
 }
 let skillOpen = $state(false), skillButton = $state<HTMLButtonElement>(), promptInput = $state<HTMLTextAreaElement>()
 let promptOpen = $state(false), promptButton = $state<HTMLButtonElement>()
 function useSkill(skill: Skill) {
  const text = useSkillPrompt(skill)
  if (!text) { failed = true; message = 'That skill has no name to use it by.'; invalidMessage = true; return }
  const at = promptInput?.selectionStart ?? prompt.length, end = promptInput?.selectionEnd ?? at
  const inserted = insertSkillPrompt(prompt, text, at, end)
  prompt = inserted.text
  edited()
  void tick().then(() => { promptInput?.focus(); promptInput?.setSelectionRange(inserted.caret, inserted.caret) })
 }
 // The optional agent, model and effort a prompt suggests. Preselected only where the person has
 // not chosen those fields here, and only for the agent the suggestion was written for.
 async function applyPromptSuggestions(item: TerminalPrompt, unresolved: string[]) {
  // A model or effort the person chose also belongs to their current agent.
  // Switching it implicitly would let the rekey erase those explicit choices.
  if (item.tool && !explicitTool && !explicitModel && !explicitEffort && availableAgents.includes(item.tool)) {
   explicitTool = item.tool
   // The agent rekey resets its dependent defaults. Apply template defaults
   // after that reset, using the resolved agent rather than the previous one.
   await tick()
  }
  if (locked) return
  const agent = chosenTool
  if (item.model && !explicitModel && (!item.tool || item.tool === agent)) { explicitModel = true; modelChoice = item.model; modelOther = '' }
  if (item.effort && !explicitEffort && (!item.tool || item.tool === agent) && effortOptions.includes(item.effort)) { explicitEffort = true; effort = item.effort }
  if (unresolved.length) {
   failed = false
   message = `Inserted “${item.title}”. ${unresolved.join(', ')} ${unresolved.length === 1 ? 'was' : 'were'} not known here, so ${unresolved.length === 1 ? 'it stays' : 'they stay'} as written.`
  }
 }
 // A saved prompt fills the opening prompt as editable text. Nothing is sent.
 function usePrompt(result: { prompt: TerminalPrompt; text: string; unresolved: string[] }) {
  const { prompt: item, text, unresolved } = result
  const at = promptInput?.selectionStart ?? prompt.length, end = promptInput?.selectionEnd ?? at
  const inserted = insertSkillPrompt(prompt, text, at, end)
  if (new TextEncoder().encode(inserted.text).length > MAX_PROMPT_BYTES) {
   failed = true; invalidMessage = true
   message = `That prompt is too long for this field. Keep it under ${MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes.`
   return
  }
  prompt = inserted.text
  edited(); void applyPromptSuggestions(item, unresolved)
  void tick().then(() => { promptInput?.focus(); promptInput?.setSelectionRange(inserted.caret, inserted.caret) })
 }
 // Typing a backslash token in the opening prompt opens the same prompts inline.
 let promptMenu = $state<{ start: number; query: string } | null>(null)
 let promptLibrary = $state<PromptLibrary | null>(null), promptLoading = $state(false), promptReadError = $state('')
 let promptSuppressed = false, promptLife: AbortController | undefined
 let promptMenuEl = $state<InlinePromptMenu>()
 const promptContext = $derived({ project: pickedProject?.name, branch: pickedProject?.baseBranch || '', folder: pickedProject?.dir || typedDir || '', sessionTitle: promptTitle(prompt) })
 async function loadPromptLibrary() {
  if (promptLibrary || promptLoading) return
  promptLife?.abort()
  const life = promptLife = new AbortController()
  promptLoading = true; promptReadError = ''
  try { promptLibrary = await readPromptLibrary(life.signal) }
  catch (e) { if (!life.signal.aborted) promptReadError = e instanceof Error && e.message ? e.message : 'Prompts could not be read. Try again.' }
  finally { if (!life.signal.aborted) promptLoading = false }
 }
 function openPromptMenu(trigger: { start: number; query: string }) {
  if (locked || !chosenTool) return
  promptMenu = trigger
  void loadPromptLibrary()
 }
 function closePromptMenu(dismissed = true) {
  promptMenu = null
  if (dismissed) promptSuppressed = true
 }
 function typedPrompt(event: Event) {
  const input = event.currentTarget as HTMLTextAreaElement
  prompt = input.value
  const trigger = promptTrigger(input.value, input.selectionStart ?? input.value.length)
  if (!trigger) { promptSuppressed = false; promptMenu = null; return }
  if (!promptSuppressed) openPromptMenu(trigger)
 }
 function pickPrompt(result: { prompt: TerminalPrompt; text: string; unresolved: string[] }) {
  if (!promptMenu) return
  const out = replacePromptTrigger(prompt, promptMenu, result.text, MAX_PROMPT_BYTES)
  if ('error' in out) { failed = true; message = out.error; return }
  prompt = out.draft; promptSuppressed = true; promptMenu = null
  edited(); void applyPromptSuggestions(result.prompt, result.unresolved)
  void tick().then(() => { promptInput?.focus(); promptInput?.setSelectionRange(out.caret, out.caret) })
 }
 function promptKeys(event: KeyboardEvent) {
  if (!promptMenu) return
  if (event.key === 'ArrowDown') { event.preventDefault(); promptMenuEl?.move(1); return }
  if (event.key === 'ArrowUp') { event.preventDefault(); promptMenuEl?.move(-1); return }
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closePromptMenu(true); return }
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); promptMenuEl?.choose(); return }
  if (event.key === 'Tab') closePromptMenu(false)
 }
 function selectTool(next: string) { if (locked) return; explicitTool = next; tool = next; edited() }

 // The project is read once for its own defaults; a failure falls back to the account and favourites.
 let projectReadSeq = 0
 async function loadProject(id: string) {
  // The newest selection owns projectData: an earlier read that finishes late, or after "No project", is ignored.
  const seq = ++projectReadSeq
  projectData = null
  if (!id) return
  try {
   const next = await getFolderProject(id, life.signal)
   if (seq === projectReadSeq) projectData = next
  } catch (e) {
   if (seq !== projectReadSeq) return
   if (!life.signal.aborted && (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401)) onsignedout?.(new UnauthorizedError())
   else if (!life.signal.aborted) projectData = null
  }
 }
 async function loadAccount() {
  try {
   const answer = await getProfile()
   if (life.signal.aborted) return
   accountDefaults = { defaultTool: answer.profile.defaultTool, defaultModel: answer.profile.defaultModel, defaultEffort: answer.profile.defaultEffort, defaultClineSource: answer.profile.defaultClineSource, defaultBaseBranch: answer.profile.defaultBaseBranch, worktreesByDefault: answer.profile.worktreesByDefault }
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
  for (const item of attachments) URL.revokeObjectURL(item.preview)
  attachments = []
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
  if (where === 'project') return projectSelect
  if (where === 'machine') return field('machine')
  if (where === 'tool') return field('tool')
  if (where === 'model') return modelButton
  if (where === 'effort') return effortSelect
  if (where === 'folder') return folderChoice === TYPED ? field('dir') : field('folder')
  if (where === 'prompt') return field('prompt')
  return startButton
 }
 // The star saves the chosen source as the account default, or clears it when it already is.
 async function toggleSourceDefault() {
  if (!chosenSource || sourceBusy) return
  const wasDefault = sourceIsDefault
  sourceBusy = true; sourceNote = ''
  try {
   const answer = await saveProfile({ defaultClineSource: wasDefault ? '' : chosenSource })
   if (!life.signal.aborted) accountDefaults = { ...accountDefaults, defaultClineSource: answer.profile.defaultClineSource ?? '' }
  } catch (e) {
   if (life.signal.aborted) return
   if (e instanceof UnauthorizedError) onsignedout?.(e)
   else sourceNote = e instanceof Error && e.message ? e.message : 'Couldn’t save the default source just now. Try again.'
  } finally { sourceBusy = false }
 }
 async function start() {
  if (busy || life.signal.aborted) return
  pickerOpen = false
  attempted = true
  if (!operation) {
   if (projectBlocked || blocked || !machine) return
   // The errors are derived, so they already count this attempt; no await here, or a second press could slip past the busy guard.
   const invalid = modelError ? ['model', modelError] : effortError ? ['effort', effortError] : dirError ? ['dir', dirError] : promptError ? ['prompt', promptError] : null
   if (invalid) { failed = true; message = invalid[1]; invalidMessage = true; field(invalid[0])?.focus(); return }
   if (imageReason) { failed = true; field('prompt')?.focus(); return }
   const launch: LaunchRequest & { tool: string } = { tool: chosenTool }
   if (structured) { launch.structured = true; if (chatNeedsTrust(chosenTool)) launch.trust = true }
   if (model) launch.model = model
   if (effort) launch.effort = effort as NonNullable<LaunchRequest['effort']>
   if (sourceShown && chosenSource) launch.provider = chosenSource
   if (dir) launch.dir = dir
   if (prompt) launch.prompt = prompt
   if (yoloChosen && !yoloWhy) launch.yolo = true
   const chosenProject = pickedProject
   const onProject = !!chosenProject && machine.agentId === chosenProject.host && dir === chosenProject.dir
   if (onProject) launch.projectId = chosenProject.id
   // A chosen project's own defaults apply however the dialog was opened: a repository that asks for a worktree (or
   // inherits the account's default) starts in one, from its own base branch, never in the shared checkout.
   if (onProject && !structured && chosenProject?.isRepo) {
    const worktree = chosenProject.worktreeDefault === 'yes' ? true : chosenProject.worktreeDefault === 'no' ? false : accountDefaults?.worktreesByDefault !== false
    if (worktree && machine.capabilities?.worktree) {
     launch.worktree = crypto.randomUUID().replaceAll('-', '')
     launch.project = projectSlug(chosenProject.name)
     const base = chosenProject.baseBranch || accountDefaults?.defaultBaseBranch || ''
     if (base) launch.base = base
    }
   }
   const taskTitle = promptTitle(prompt) || titleFor(chosenTool, dir)
   // Who named the session comes from the typed task, never from the image paths appended to it below.
   launch.titleOrigin = titleOriginFor(prompt)
   if (attachments.length) {
    // The images go to the machine before the session exists; their paths join the opening prompt.
    busy = true
    const references: string[] = []
    try {
     for (let i = 0; i < attachments.length; i++) {
      message = attachments.length === 1 ? 'Attaching the image…' : `Attaching image ${i + 1} of ${attachments.length}…`
      const uploaded = await launchUpload(machine.agentId, attachments[i].file, life.signal)
      rememberUpload(attachments[i].file, attachments[i].file.name, uploaded.path, uploaded.reference)
      references.push(uploaded.reference)
     }
    } catch (e) {
     busy = false
     if (life.signal.aborted) return
     if (e instanceof UnauthorizedError || e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
     failed = true
     message = `Couldn't attach that image. ${refusalSentence(e)}`
     void focusAfter(() => startButton)
     return
    }
    if (life.signal.aborted) return
    launch.prompt = imagePrompt(launch.prompt ?? '', references)
    if (new TextEncoder().encode(launch.prompt).length > MAX_PROMPT_BYTES) {
     // The machine rejects a prompt over the limit, so catch the paths' extra bytes before it is asked to start.
     busy = false; failed = true; invalidMessage = true
     message = `The image paths push the opening prompt past ${MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes. Shorten the prompt or remove an image.`
     void focusAfter(() => field('prompt'))
     return
    }
   }
   operation = { agentId: machine.agentId, host: machine.host, requestId: crypto.randomUUID(), title: taskTitle, launch }
  }
  const op = operation
  busy = true; failed = false; invalidMessage = false
  message = `Starting ${toolName(op.launch.tool)} on ${op.host}… this can take a few seconds.`
  try {
   const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(structured ? 60000 : 15000)]), op.launch)
   if (life.signal.aborted) return
   for (const item of attachments) URL.revokeObjectURL(item.preview)
   attachments = []
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
  projectSelect?.focus()
  // A phone opens the picker as the shared bottom sheet; a mouse gets the popover.
  const coarse = matchMedia('(pointer: coarse)')
  const readPointer = () => { touch = coarse.matches }
  readPointer()
  coarse.addEventListener?.('change', readPointer)
  // The profile is read even without a project, so a machine card or the workspace gets the account defaults.
  void loadFavourites()
  void loadAccount()
  void loadYolo()
  if (projectId) void loadProject(projectId)
  return () => { stopLeaving(); life.abort(); prompt = ''; for (const item of attachments) URL.revokeObjectURL(item.preview); coarse.removeEventListener?.('change', readPointer) }
 })
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header data-shell-head>
  <div>
   <p class="eyebrow" data-shell-aside>Sessions</p>
   <h2 id={`${id}-title`}>Start an agent</h2>
   <p class="lede" data-shell-aside>Right then: pick a project and a machine, then an agent. It starts in a new terminal session you can come back to.</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={busy} aria-label="Close" data-shell-close>Close <svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
 </header>
 <form class="sheet" oninput={edited} onsubmit={(event) => { event.preventDefault(); void start() }}>
  <!-- While everything is locked there is nothing to tab to in here, so the scrolling part takes focus itself and can be read. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="body" data-shell-body tabindex={locked ? 0 : undefined} role={locked ? 'region' : undefined} aria-label={locked ? 'Your choices' : undefined}>
  <fieldset disabled={locked}>
   <div class="field">
    <label for={`${id}-project`}>Project</label>
    <select id={`${id}-project`} bind:this={projectSelect} value={projectSel} aria-required="true" aria-describedby={[projectBlocked ? `${id}-project-error` : '', usingProject ? `${id}-project-dir` : ''].filter(Boolean).join(' ') || undefined} onchange={(event) => pickProject((event.currentTarget as HTMLSelectElement).value)}>
     <option value="" disabled>Choose a project</option>
     {#each projects as item (item.id)}<option value={item.id} disabled={item.id !== projectSel && !projectReady(item)}>{item.name}{item.machine ? ` · ${item.machine}` : ''}{projectReady(item) ? '' : ' · machine not ready'}</option>{/each}
     <option value={NO_PROJECT}>No project — start in a folder</option>
    </select>
    {#if projectBlocked}
     <p class="reason" id={`${id}-project-error`}>{projectBlocked}</p>
    {:else if usingProject}
     <p class="hint" id={`${id}-project-dir`}>Starts in {pickedProject!.dir}.</p>
    {/if}
   </div>

   <div class="field">
    <label for={`${id}-machine`}>Machine</label>
    <select id={`${id}-machine`} bind:this={machineSelect} value={agentId} aria-required="true" aria-describedby={blocked ? `${id}-blocked` : undefined} onchange={(event) => pickMachine((event.currentTarget as HTMLSelectElement).value)}>
     {#if !machine}<option value="" disabled>Choose a machine</option>{/if}
     {#each machines as item (item.agentId)}<option value={item.agentId} disabled={!item.ready}>{item.host}{item.ready ? '' : ' · not ready'}</option>{/each}
    </select>
    {#if blocked}<p class="reason" id={`${id}-blocked`}>{blocked}</p>{/if}
    {#if onaddmachine}<button type="button" class="linklike" onclick={addMachine}>Not listed? Add a machine</button>{/if}
   </div>

   {#if folderVisible}
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
   {/if}

   <div class="field">
    <label for={`${id}-tool`}>Agent</label>
    <select id={`${id}-tool`} value={chosenTool} disabled={!choices.some(c => c.available)} onchange={(event) => selectTool((event.currentTarget as HTMLSelectElement).value)}>
     {#if !chosenTool}<option value="" disabled>No agent available</option>{/if}
     {#each choices as choice (choice.id)}<option value={choice.id} disabled={!choice.available}>{choice.name}{choice.available ? '' : ` · ${choice.reason}`}</option>{/each}
    </select>
   </div>

   {#if chosenTool === 'opencode' && machine && !chatCapable}
    <p class="hint">This machine does not currently offer Gaffer for OpenCode. This starts the OpenCode TUI instead.</p>
   {/if}

   {#if chosenTool === 'claude'}
    {@const adapterHint = claudeAdapterHint(machine)}
    {#if adapterHint}<p class="hint claude-adapter">{adapterHint.split(CLAUDE_ADAPTER_INSTALL)[0]}<code>{CLAUDE_ADAPTER_INSTALL}</code></p>{/if}
   {/if}

   {#if chatCapable}
    <div class="field chat-choice">
     <label for={`${id}-session`}>Session</label>
     <select use:gafferTooltip={structured} id={`${id}-session`} value={structured ? 'reet' : 'terminal'} onchange={(event) => { structured = (event.currentTarget as HTMLSelectElement).value === 'reet'; sessionTouched = true; edited() }}>
      <option value="reet">Gaffer</option>
      <option value="terminal">{toolName(chosenTool)} TUI</option>
     </select>
     {#if structured}
      {#if chosenTool === 'claude'}
       <p class="hint">In Gaffer, Claude Code asks before it edits a file or runs a command. Only your own Claude settings apply; this folder's repository settings and hooks are ignored. Its transcript is kept with the session.</p>
      {:else if chosenTool === 'codex'}
       <p class="hint">In Gaffer, Codex starts in the mode where it runs commands inside its sandbox and asks before one leaves it. You can switch it to let Codex decide for itself, in which case you may not be asked. Its transcript is kept with the session.</p>
      {:else if chosenTool === 'cline'}
       <p class="hint">In Gaffer, Cline asks before it reads a file, edits one or runs a command. Its own settings and hooks apply, and its transcript is kept with the session, which can be restored after reet restarts on the machine.</p>
      {:else if chosenTool === 'antigravity'}
       <p class="hint">In Gaffer, Google Antigravity (AGY) starts in Default mode with its configured permission rules. Approval requests appear here. Start only in a folder you trust: AGY workspace hooks and customizations may run. Its transcript is kept with the session.</p>
      {:else}
       <p class="hint">In Gaffer, OpenCode asks before it edits a file or runs a command. Any plugins in this folder's <code>.opencode</code> run when it starts, so use a folder you trust. Its transcript is kept with the session, which can be restored after reet restarts on the machine.</p>
      {/if}
     {:else}
      <p class="hint">Starts the {toolName(chosenTool)} TUI in a terminal session; the agent's own prompts apply.</p>
     {/if}
    </div>
   {/if}

   {#if sourceShown}
    <div class="field">
     <label for={`${id}-source`}>Source</label>
     <div class="source-row">
      <select id={`${id}-source`} value={chosenSource} disabled={locked} onchange={event => { sourceChoice = event.currentTarget.value; explicitSource = true; sourceNote = ''; edited() }}>
       <option value="">Cline’s own choice</option>
       {#each sourceOptions as choice (choice.id)}<option value={choice.id}>{choice.label}</option>{/each}
      </select>
      <button type="button" class="star" class:on={sourceIsDefault} disabled={locked || !chosenSource || sourceBusy} aria-pressed={sourceIsDefault} aria-label={sourceIsDefault ? `${sourceLabel(chosenSource)} is the default source. Clear it` : chosenSource ? `Make ${sourceLabel(chosenSource)} the default source` : 'Make a source the default'} title={sourceIsDefault ? 'New Cline sessions start on this source. Click to clear.' : 'Make this the source new Cline sessions start on'} onclick={() => void toggleSourceDefault()}>
       <svg viewBox="0 0 24 24" width="15" height="15" fill={sourceIsDefault ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9 6.8 19.7l1-5.9L3.5 9.7l5.9-.8z" /></svg>
       <span>{sourceBusy ? 'Saving…' : sourceIsDefault ? 'Default' : 'Make default'}</span>
      </button>
     </div>
     {#if startSource.note}<p class="hint">{startSource.note}</p>{/if}
     {#if sourceNote}<p class="reason" role="alert">{sourceNote}</p>{/if}
    </div>
   {/if}

   <div class="field">
    <label for={`${id}-model`}>Model <span class="optional">optional</span></label>
    <button id={`${id}-model`} type="button" class="picker" bind:this={modelButton} disabled={!chosenTool}
      aria-haspopup="dialog" aria-label={`Model: ${modelLabel}`} onclick={openPicker}>
     <span class="value">{modelLabel}</span><svg class="chev" aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6" /></svg>
    </button>
    {#if modelChoice === OTHER && modelError}<p class="reason" id={`${id}-model-error`}>{modelError}</p>{/if}
    {#if structured}<p class="hint">Gaffer uses this model. Leave it at {chatToolName(chosenTool)}’s configured model to keep {chatToolName(chosenTool)}’s own; choose effort below when this machine offers it.</p>{/if}
   </div>

   {#if structured || effortOptions.length > 1 || effort}
    <div class="field">
     <label for={`${id}-effort`}>{structured ? 'Effort' : 'Thinking'}</label>
     <select id={`${id}-effort`} bind:this={effortSelect} bind:value={effort} onchange={() => { explicitEffort = true; edited() }} disabled={!chosenTool || effortOptions.length < 2 && !effort}>
      {#if effortError}<option value={effort} disabled>{effort} — unavailable</option>{/if}
      {#each effortOptions as value (value || 'default')}<option value={value}>{value || 'Agent default'}</option>{/each}
     </select>
     {#if effortError}<p class="reason" role="alert">{effortError}</p>{/if}
     {#if structured}<p class="hint">{effortHint}</p>{/if}
    </div>
   {/if}

   <div class="field">
    <div class="label-row">
     <label for={`${id}-prompt`}>Opening prompt <span class="optional">optional</span></label>
     <span class="prompt-links">
      <button type="button" class="linklike skill-add" bind:this={promptButton} disabled={!chosenTool || locked} aria-haspopup="dialog" aria-expanded={promptOpen} onclick={() => { closePromptMenu(false); promptOpen = true }}>Use a prompt</button>
      <button type="button" class="linklike skill-add" bind:this={skillButton} disabled={!chosenTool} aria-haspopup="listbox" aria-expanded={skillOpen} onclick={() => skillOpen = !skillOpen}>Use a skill</button>
     </span>
    </div>
    <textarea id={`${id}-prompt`} bind:this={promptInput} bind:value={prompt} rows="3" disabled={!chosenTool} oninput={typedPrompt} onkeydown={promptKeys} onpaste={pasted} onblur={() => closePromptMenu(false)} aria-invalid={promptError ? 'true' : undefined} aria-describedby={`${id}-prompt-count${promptError ? ` ${id}-prompt-error` : ''}`}></textarea>
    {#if promptMenu}
     <InlinePromptMenu inline bind:this={promptMenuEl} prompts={promptLibrary?.prompts ?? []} skills={promptLibrary?.skills ?? {}} query={promptMenu.query} context={promptContext} loading={promptLoading} error={promptReadError} oninsert={pickPrompt} onclose={() => closePromptMenu(false)} />
    {/if}
    <div class="prompt-tools">
     <label class="attach">Attach images<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple disabled={locked || !chosenTool} onchange={(event) => { addImages(Array.from(event.currentTarget.files ?? [])); event.currentTarget.value = '' }} /></label>
    </div>
    {#if attachments.length}
     <ul class="attachments" aria-label="Images to attach">
      {#each attachments as item (item.id)}
       <li><img src={item.preview} alt="" /><span>{item.file.name || 'image'} · {formatImageSize(item.file.size)}</span><button type="button" aria-label={`Remove ${item.file.name || 'image'}`} disabled={locked} onclick={() => removeImage(item.id)}><svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg></button></li>
      {/each}
     </ul>
    {/if}
    {#if imageReason}<p class="reason" id={`${id}-image-error`}>{imageReason}</p>{/if}
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
   <button class="btn primary" type="submit" disabled={busy || (!operation && (!!projectBlocked || !!blocked || !!imageReason))} aria-busy={busy} bind:this={startButton}>{busy ? 'Starting…' : operation ? 'Try the same request again' : 'Start'}</button>
  </footer>
 </form>
 {#if yoloOpen && machine}
  <YoloDialog gates={yoloGateList} tool={chosenTool} {structured} machine={machine.host} oncancel={cancelYolo} onstart={startYolo} />
 {/if}
 {#if skillOpen}
  <SkillPromptPicker opener={skillButton ?? null} sheet={touch} oninsert={useSkill} onclose={() => { skillOpen = false }} />
 {/if}
 {#if promptOpen}
  <PromptPicker opener={promptButton ?? null} sheet={touch}
   context={{ project: pickedProject?.name, branch: pickedProject?.baseBranch || '', folder: pickedProject?.dir || typedDir || '', sessionTitle: promptTitle(prompt) }}
   oninsert={usePrompt} onclose={() => { promptOpen = false; void tick().then(() => promptInput?.focus()) }} />
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
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 20px 24px; border-bottom: 1px solid var(--line); }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.875rem; font-weight: 600; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; display: inline-flex; align-items: center; gap: 6px; }
 /* The fields scroll; the footer with the status line and Start sits below them, outside the scrolling part, so it never covers a field. */
 .sheet { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
 .body { display: grid; gap: 16px; padding: 20px 24px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: grid; gap: 16px; }
 .field { display: grid; gap: 6px; min-width: 0; }
 .source-row { display: flex; align-items: center; gap: 8px; min-width: 0; }
 .source-row select { flex: 1 1 auto; }
 .star { display: inline-flex; align-items: center; gap: 4px; flex: none; min-height: 42px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
 .star:hover:not(:disabled) { color: var(--ink); background: var(--tint); }
 .star.on { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, transparent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
 .star:disabled { opacity: .5; cursor: default; }
 .star:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
 label { font-size: 0.875rem; font-weight: 600; }
 .label-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
 .prompt-links { display: flex; align-items: center; gap: 12px; flex: none; }
 .label-row .skill-add { min-height: 28px; padding: 0; }
 label.sub { font-weight: 500; margin-top: 4px; }
 .optional { color: var(--muted); font-weight: 400; font-size: 0.875rem; }
 select, input:not([type=radio]), textarea { min-width: 0; width: 100%; min-height: 42px; padding: 8px 12px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 .picker { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; width: 100%; min-height: 42px; padding: 8px 12px; font: inherit; color: var(--ink); text-align: left; background: var(--bg); border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
 .picker:disabled { opacity: .55; cursor: default; }
 .picker .value { min-width: 0; overflow-wrap: anywhere; }
 .picker .chev { flex: none; color: var(--muted); }
 /* A phone zooms into a field under 16 px when it is focused; `font: inherit` above would otherwise beat app.css's rule. */
 @media (max-width: 560px) { select, input:not([type=radio]), textarea, .picker { font-size: 16px; } }
 textarea { resize: vertical; min-height: 80px; }
 [aria-invalid='true'] { border-color: var(--bad); }
 .approvals .legend { font-size: 0.875rem; }
 .approval-choices { display: flex; gap: 8px; flex-wrap: wrap; }
 .approval-choices label { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font-size: 0.875rem; cursor: pointer; }
 .approval-choices label.on { border-color: var(--accent); background: var(--tint); }
 .approval-choices label.off { opacity: .55; cursor: not-allowed; }
 .approval-choices label:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
 .hint, small { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
 .prompt-tools { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
 .attach { position: relative; align-self: start; padding: 4px 0; color: var(--accent-text); font-size: 0.875rem; font-weight: 600; cursor: pointer; }
 .attach input { position: absolute; inset: 0; width: 100%; height: 100%; opacity: 0; cursor: pointer; }
 .attach:focus-within { outline: 2px solid var(--accent); outline-offset: 3px; }
 .attach:has(input:disabled) { opacity: .55; cursor: default; }
 .attachments { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
 .attachments li { display: flex; align-items: center; gap: 8px; max-width: 100%; padding: 4px 6px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.875rem; }
 .attachments img { width: 32px; height: 32px; object-fit: cover; border-radius: 4px; }
 .attachments span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
 .attachments button { min-width: 28px; display: inline-grid; place-items: center; border: 0; background: none; color: var(--muted); cursor: pointer; font-size: 18px; }
 .attachments button:disabled { cursor: default; opacity: .55; }
 .count { font-variant-numeric: tabular-nums; }
 .count.over { color: var(--bad); }
 .reason { margin: 0; color: var(--bad); font-size: 0.875rem; line-height: 1.5; }
 .chat-choice code { font-size: max(0.875rem, .9em); }
 .sentence { margin: 0; padding: 12px 16px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); font-size: 0.875rem; line-height: 1.5; }
 .status { flex: 1 0 100%; margin: 0; font-size: 0.875rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 .linklike { justify-self: start; padding: 4px 0; min-height: 32px; font: inherit; font-size: 0.875rem; color: var(--accent-text); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 /* Always in view below the scrolling fields, so Start is reachable on a short screen. */
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; padding: 16px 24px 20px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } }
</style>
