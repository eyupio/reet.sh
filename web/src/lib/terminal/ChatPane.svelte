<script lang="ts">
 import { onDestroy, onMount, tick, untrack } from 'svelte'
 import { fade, fly } from 'svelte/transition'
 import { applyChatEvent, checkQuestionDraft, endedReason, diffCounts, displayPath, dropQueuedPrompt, editQueuedPrompt, emptyDraft, enqueuePrompt, fieldLabel, formatDuration, initialChat, inlineSegments, isWorking, own, pendingPermissions, pendingQuestions, promoteQueuedPrompt, proseBlocks, resetChat, summarizeAnswers, textSegments, thoughtPreview, toolLabel, toolState, toolTarget, type ChatEvent, type ChatState, type ChatItem, type QueuedPrompt, type QuestionAnswers, type QuestionDraft } from '../chat'
 import { copyText } from '../clipboard'
 import { chatToolName, chatAnswer, chatCancel, chatQuestionAnswer, chatModel, machineModels, chatPrompt, chatURL, getSessionSkills, launchUpload, terminalClosed, type TerminalSession } from './api'
 import { addModelFavourite, listModelFavourites, setModelFavouriteDefault } from '../api'
 import { defaultLabel, type ModelFavourite } from '../models'
 import Toast from '../Toast.svelte'
 import ImageLightbox from './ImageLightbox.svelte'
import SkillPicker from './SkillPicker.svelte'
import { applyInvocation, slashQuery, type SessionSkill, type SessionSkills } from './skills'
 import { loadBinding, loadThumb, makeThumb, saveBinding, saveThumb } from './thumbs'
 import { baseName, chatPromptParts, formatImageSize, imageFiles, splitImageRefs, MAX_EMBEDDED_IMAGES, MAX_IMAGES, MAX_IMAGE_TOTAL_BYTES, validateImage } from './upload'

 let { session, uploadsSupported = false, uploadsOff = false, acpImages = false, onbusy }: { session: TerminalSession; uploadsSupported?: boolean; uploadsOff?: boolean; acpImages?: boolean
  /** Told whenever the chat starts or stops being busy (a running turn, a waiting approval or question), for the header's TUI
   *  switch; undefined until the stream has opened and its replay has been applied, as the chat cannot say before then. */
  onbusy?: (busy: boolean | undefined) => void } = $props()

 let chat: ChatState = $state(initialChat())
 let selectedModel = $state(''), currentModel = $state(''), changingModel = $state(false)
 let modelSession = '', modelValue = ''
 $effect(() => {
  const id = session.id, model = session.model ?? ''
  if (id === modelSession && model === modelValue) return
  modelSession = id; modelValue = model
  selectedModel = model; currentModel = model
 })
 let models = $state<string[]>([]), modelNote = $state('')
 let favourites = $state<ModelFavourite[]>([]), favouriteBusy = $state(false)
 // Confirmations (model changed, default saved) say it once and fade away: the star and the model field already show the result.
 let toast = $state<{ id: number; text: string; bad: boolean } | null>(null), toastSeq = 0
 function flash(text: string, bad = false) {
  toast = { id: ++toastSeq, text, bad }
 }
 const tool = $derived(session.tool || 'opencode')
 const toolName = $derived(chatToolName(tool))
 const agentDefault = $derived(favourites.find(f => f.tool === tool && f.agentDefault)?.model ?? '')
 const selectedIsDefault = $derived(!!selectedModel.trim() && selectedModel.trim() === agentDefault)
 const selectedIsFavourite = $derived(favourites.some(f => f.tool === tool && f.model === selectedModel.trim()))
 const failed = $derived(chat.status.state === 'error')
 const guidance = $derived(/credit|quota|usage limit|billing|balance|insufficient_quota/i.test(chat.status.detail)
  ? 'This model has reached its usage or credit limit. Choose another model below, or add credits with your provider, then send a message to continue.'
  : /rate.?limit|too many requests|429/i.test(chat.status.detail)
  ? 'Wait for the provider’s rate limit to reset, or choose another model below, then send your message again.'
  : /unauthori|api.?key|authentication|401|403/i.test(chat.status.detail)
  ? 'Check this provider’s credentials on the session’s machine, or choose a model from another configured provider below.'
  : 'Check the error details below. You can choose another model or resolve the provider issue, then send a message to continue.')
 async function changeModel() {
  if (!selectedModel || changingModel || working || ended) return
  changingModel = true; feedback = ''; modelNote = ''; if (toast && !toast.bad) toast = null
  try { await chatModel(session.id, selectedModel); currentModel = selectedModel; flash(`Model changed to ${selectedModel}.`) }
  catch (e) { feedback = e instanceof Error ? e.message : 'The model could not be changed. Try again.' }
  finally { changingModel = false }
 }
 async function toggleDefault() {
  const model = selectedModel.trim()
  if (!model || favouriteBusy) return
  const wasDefault = selectedIsDefault
  favouriteBusy = true
  try {
   if (!wasDefault && !selectedIsFavourite) await addModelFavourite(tool, model)
   const answer = await setModelFavouriteDefault(tool, wasDefault ? '' : model)
   if (answer?.favourites) favourites = answer.favourites
   flash(wasDefault ? `Cleared the default for ${tool}.` : `New ${tool} sessions use ${model}.`)
  } catch (e) {
   flash(e instanceof Error ? e.message : 'Couldn’t update the agent default just now. Try again.', true)
  } finally { favouriteBusy = false }
 }
 const MAX_CHAT_BYTES = 2000
 let dropping = $state(false)
 let draft = $state(''), feedback = $state(''), sending = $state(false), uploading = $state(false), answering = $state('')
 type Picture = { id: number; file: File; preview: string }
 let images = $state<{ id: number; file: File; preview: string }[]>([]), imageSeq = 0, attachNote = $state('')
 let fileInput: HTMLInputElement | undefined = $state()
 // The skill picker. It only ever puts an invocation into the draft; nothing is sent until the person presses Send.
 let composerInput: HTMLTextAreaElement | undefined = $state()
 let skillsOpen = $state(false), skillData = $state<SessionSkills | null>(null), skillsLoading = $state(false), skillsError = $state(''), skillQuery = $state('')
 let skillsLife: AbortController | undefined, slashDismissed = false
 async function loadSkills() {
  skillsLife?.abort()
  const life = skillsLife = new AbortController()
  skillsLoading = true; skillsError = ''; skillData = null
  try { const data = await getSessionSkills(session.id, life.signal); if (!life.signal.aborted) skillData = data }
  catch (e) { if (!life.signal.aborted) skillsError = e instanceof Error && e.message ? e.message : 'Skills could not be read. Try again.' }
  finally { if (!life.signal.aborted) skillsLoading = false }
 }
 function openSkills(query = '') {
  if (skillsOff || skillsOpen) return
  skillQuery = query; skillsOpen = true
  void loadSkills()
 }
 async function closeSkills(dismissed = true) {
  skillsLife?.abort(); skillsLife = undefined
  skillsOpen = false; skillsLoading = false
  if (dismissed && slashQuery(draft) !== null) slashDismissed = true
  await tick()
  composerInput?.focus()
  composerInput?.setSelectionRange(draft.length, draft.length)
 }
 function typed(event: Event) {
  const value = (event.currentTarget as HTMLTextAreaElement).value
  const query = slashQuery(value)
  if (query === null) { slashDismissed = false; return }
  if (!slashDismissed) openSkills(query)
 }
 function pickSkill(skill: SessionSkill) {
  if (skillsOff) { feedback = ended ? `${skillsOff} Nothing was inserted.` : chat.connection !== 'open' ? 'Connection changed. Nothing was inserted.' : 'The message box is busy. Nothing was inserted.'; void closeSkills(); return }
  const result = applyInvocation(draft, skill.invocation, MAX_CHAT_BYTES)
  if ('error' in result) { feedback = result.error; void closeSkills(); return }
  feedback = ''; draft = result.draft
  void closeSkills(false)
 }
 // What the person sent: the machine's path for each upload maps to a picture held in this browser, so a message shows the
 // image, not its path. A message from before a reload falls back to a saved thumbnail, then to a filename chip.
 let sentPreviews = $state<Record<string, { url: string; name: string }>>({})
 const sentUrls: string[] = []
 // With embedded images the echo carries no path; the text sent and its uploads wait here for the echo to arrive.
 let echoes: { text: string; refs: string[] }[] = []
 let bound = $state<Record<string, string[]>>({})
 const boundKeys = new Set<string>()
 const IMAGE_ONLY_ECHO = 'Image attached.'
 let lightbox = $state<{ items: { url: string; name: string }[]; index: number } | null>(null)
 function previewFor(ref: string): { url: string; name: string } | null { return sentPreviews[ref] ?? loadThumb(ref) }
 function attachmentsOf(item: { key: string; text: string }): { text: string; refs: string[] } {
  const refs = bound[item.key] ?? loadBinding(session.id, item.key)
  // The broker shows an image-only message as this placeholder; the thumbnails say it better.
  return refs ? { text: item.text.trim() === IMAGE_ONLY_ECHO ? '' : item.text, refs } : splitImageRefs(item.text)
 }
 function openPreview(items: { url: string; name: string }[], index: number) { if (items.length) lightbox = { items, index } }
 $effect(() => {
  for (const item of chat.items) {
   if (item.kind !== 'user' || boundKeys.has(item.key)) continue
   boundKeys.add(item.key)
   const at = echoes.findIndex(echo => echo.text === item.text.trim())
   if (at < 0) continue
   const [echo] = echoes.splice(at, 1)
   bound = { ...bound, [item.key]: echo.refs }
   saveBinding(session.id, item.key, echo.refs)
  }
 })
 // Messages typed while OpenCode works are kept here and sent one at a time when the turn finishes. Nothing is stored.
 let queued = $state<QueuedPrompt<Picture[]>[]>([]), queueSeq = 0
 let queueHalted = $state(false), queueNote = $state(''), dispatching = $state(false)
 // Set after one queued message is sent, until the agent is seen working again: one turn takes one message.
 let awaitingWork = $state(false), awaitingTimer: ReturnType<typeof setTimeout> | undefined
 let log: HTMLElement | undefined = $state()
 let source: EventSource | undefined
 let alive = true, nearBottom = true
 // Events are batched before they reach the reducer: a fast stream, or a continued chat replaying its whole history,
 // arrives as many events in a burst, and applying and drawing each one separately is what used to stall the tab.
 let pendingEvents: ChatEvent[] = []
 let flushTimer: ReturnType<typeof setTimeout> | undefined
 // Whether this chat's word on being busy can be trusted: the controller writes a stream's reset and its replay together, so
 // the chat has replayed once what arrived with the reset has been applied (settle, one batch later). Until then, and again
 // while a dropped stream waits for its next reset, the chat says nothing either way.
 let replayed = $state(false), settleTimer: ReturnType<typeof setTimeout> | undefined
 function stopSettle() { if (settleTimer !== undefined) { clearTimeout(settleTimer); settleTimer = undefined } }
 // Every tool row is folded by default, including a failure; the person's toggle wins for that key.
 let expanded = $state<Record<string, boolean>>({})
 let copied = $state('')
 let copyTimer: ReturnType<typeof setTimeout> | undefined

 const ended = $derived(terminalClosed(session.state) || chat.status.state === 'ended')
 // A dismissed `/` trigger re-arms whenever the draft stops being a leading-slash token, however the draft changed.
 $effect(() => { if (slashQuery(draft) === null) slashDismissed = false })
 const working = $derived(isWorking(chat))
 // One value says whether the message box can take text; the textarea, the Skills button and a pick all use it, so they cannot drift apart.
 const composerBlocked = $derived(sending || uploading || changingModel || working || chat.connection === 'reconnecting')
 const skillsOff = $derived(ended ? 'This reet session has ended.' : chat.connection !== 'open' ? 'Skills can be inserted once reet is connected.' : sending || uploading || changingModel ? 'Skills can be inserted once the message has been sent.' : working ? 'Skills can be inserted when the agent is idle.' : '')
 const pending = $derived(pendingPermissions(chat))
 const asked = $derived(pendingQuestions(chat))
 const statusWord = $derived(chat.connection === 'reconnecting' ? 'Reconnecting to reet…' : ended ? 'This reet session has ended.' : pending.length ? 'Waiting for your approval.' : asked.length ? 'Waiting for your answer.' : working ? `${toolName} is working…` : chat.status.state === 'error' ? `${toolName} stopped with an error.` : chat.status.state === 'cancelled' ? 'Stopped.' : 'Ready.')
 $effect(() => { onbusy?.(replayed ? working || pending.length > 0 || asked.length > 0 : undefined) })
 const statusDetail = $derived(pending.length || asked.length ? '' : chat.status.detail)
 const canAttach = $derived(uploadsSupported && !uploadsOff && !ended)
 const imageLimit = $derived(acpImages ? MAX_EMBEDDED_IMAGES : MAX_IMAGES)

 function closeStream() { source?.close(); source = undefined }
 function stopFlush() {
  if (flushTimer !== undefined) { clearTimeout(flushTimer); flushTimer = undefined }
 }
 function flushEvents() {
  stopFlush()
  if (!pendingEvents.length) return
  const events = pendingEvents
  pendingEvents = []
  let next = chat
  for (const event of events) next = applyChatEvent(next, event)
  if (next === chat) return
  chat = next
  void scrollToEnd()
 }
 function queueEvent(event: ChatEvent) {
  pendingEvents.push(event)
  // A backlog is applied in one pass rather than one render per event.
  if (pendingEvents.length >= 1000) flushEvents()
  else if (flushTimer === undefined) flushTimer = setTimeout(flushEvents, 16)
 }
 function readReset(event: MessageEvent) {
  let truncated = false
  try { truncated = !!JSON.parse(event.data)?.truncated } catch { /* fixed empty reset */ }
  pendingEvents = []; stopFlush()
  chat = { ...resetChat(truncated), connection: 'open' }
  replayed = false; stopSettle()
  settleTimer = setTimeout(() => { settleTimer = undefined; flushEvents(); replayed = true }, 16)
  void scrollToEnd()
 }
 function readEvent(event: MessageEvent) {
  let parsed: ChatEvent
  try { parsed = JSON.parse(event.data) as ChatEvent } catch { return }
  queueEvent(parsed)
 }
 function onScroll() {
  if (!log) return
  nearBottom = log.scrollHeight - log.scrollTop - log.clientHeight < 80
 }
 async function scrollToEnd() {
  if (!nearBottom) return
  await tick()
  if (log) log.scrollTop = log.scrollHeight
 }
 function noteImages(files: File[]) {
  let total = images.reduce((sum, item) => sum + item.file.size, 0)
  let refused = ''
  for (const file of files) {
   const check = validateImage(file)
   if (!check.ok) { refused = check.reason === 'type' ? 'Use PNG, JPEG, GIF or WebP images.' : check.reason === 'size' ? 'Each image must be 5 MB or smaller.' : 'That image is empty.'; continue }
   if (images.length >= imageLimit) { refused = `Attach at most ${imageLimit} images.`; break }
   if (total + file.size > MAX_IMAGE_TOTAL_BYTES) { refused = 'Those images are more than 50 MB together.'; break }
   images = [...images, { id: ++imageSeq, file, preview: URL.createObjectURL(file) }]
   total += file.size
  }
  attachNote = refused
 }
 async function picked(event: Event) {
  const input = event.currentTarget as HTMLInputElement
  noteImages([...(input.files ?? [])])
  input.value = ''
 }
 function pasted(event: ClipboardEvent) {
  if (!canAttach) return
  const files = imageFiles(event.clipboardData?.items)
  if (!files.length) return
  event.preventDefault(); noteImages(files)
 }
 function dropped(event: DragEvent) {
  if (!canAttach) return
  const files = imageFiles(event.dataTransfer?.items)
  if (!files.length) return
  event.preventDefault(); noteImages(files)
 }
 function removeImage(id: number) { const item = images.find(image => image.id === id); if (item) URL.revokeObjectURL(item.preview); images = images.filter(image => image.id !== id); attachNote = '' }
 function messageProblem(text: string): string {
  if (new TextEncoder().encode(text).length > MAX_CHAT_BYTES) return 'That message is too long. Keep it under 2,000 bytes.'
  return ''
 }
 /** Send a message and its pictures now, or queue it while the agent is busy. */
 async function send() {
  const text = draft.trim()
  if ((!text && !images.length) || sending || uploading || changingModel || ended || chat.connection === 'reconnecting') return
  const problem = messageProblem(text)
  if (problem) { feedback = problem; return }
  if (working || queued.length) { queueCurrent(text); return }
  const sendingImages = images.slice()
  const ok = await deliver(text, sendingImages)
  if (!ok) return
  const sent = new Set(sendingImages.map(image => image.id))
  for (const image of sendingImages) sentUrls.push(image.preview)
  draft = ''; images = images.filter(image => !sent.has(image.id)); attachNote = ''; nearBottom = true; await scrollToEnd()
 }
 /** Put what is in the composer at the end of the queue, pictures and all. */
 function queueCurrent(text: string) {
  if (images.length && !canAttach) { feedback = uploadsOff ? 'Image uploads are off on that machine.' : 'That machine does not accept images.'; return }
  const item: QueuedPrompt<Picture[]> = { id: `queued-${++queueSeq}`, text, payload: images.slice() }
  const answer = enqueuePrompt(queued, item)
  if (answer.full) { feedback = 'The queue is full. Send or remove one of the messages first.'; return }
  queued = answer.queue
  draft = ''; images = []; attachNote = ''; feedback = ''; queueHalted = false
  queueNote = 'Queued. It sends when OpenCode finishes this turn.'
  void tick().then(() => composerInput?.focus())
 }
 /** One attempt at sending a message; true when the controller accepted it. */
 async function deliver(text: string, pictures: Picture[]): Promise<boolean> {
  if (pictures.length && !canAttach) { feedback = uploadsOff ? 'Image uploads are off on that machine.' : 'That machine does not accept images.'; return false }
  const problem = messageProblem(text)
  if (problem) { feedback = problem; return false }
  uploading = pictures.length > 0; sending = true; feedback = ''
  try {
   const uploads: { path: string; reference: string }[] = []
   if (pictures.length) {
    attachNote = ''
    for (let i = 0; i < pictures.length; i++) {
     if (pictures.length > 1) attachNote = `Uploading image ${i + 1} of ${pictures.length}…`
     uploads.push(await launchUpload(session.agentId, pictures[i].file, AbortSignal.timeout(30000)))
    }
   }
   const keep: Record<string, { url: string; name: string }> = {}
   uploads.forEach((upload, i) => {
    const image = pictures[i]
    for (const ref of [upload.path, upload.reference]) if (ref) keep[ref] = { url: image.preview, name: image.file.name }
    void makeThumb(image.file).then(data => saveThumb(upload.path || upload.reference, image.file.name, data))
   })
   sentPreviews = { ...sentPreviews, ...keep }
   const body = chatPromptParts(text, uploads, acpImages)
   if (new TextEncoder().encode(body.text).length > MAX_CHAT_BYTES) { feedback = 'That message and its image paths are too long. Keep the message under 2,000 bytes.'; return false }
   const echo = body.images.length ? { text: body.text.trim() || IMAGE_ONLY_ECHO, refs: uploads.map(upload => upload.path || upload.reference) } : null
   if (echo) echoes.push(echo)
   try { await chatPrompt(session.id, body.text, body.images) }
   catch (e) { if (echo) echoes = echoes.filter(item => item !== echo); throw e }
   nearBottom = true
   return true
  }
  catch (e) { feedback = e instanceof Error ? e.message : 'That message was not sent.'; return false }
  finally { uploading = false; sending = false; attachNote = '' }
 }
 /** Send the first queued message once the agent is free; each turn takes one, so a steer lands between turns. */
 async function dispatchQueued() {
  if (dispatching || !queued.length) return
  const item = queued[0]
  dispatching = true
  try {
   const ok = await deliver(item.text, item.payload)
   if (!ok) { queueHalted = true; queueNote = 'The queued message was not sent. Retry, edit or remove it.'; return }
   // deliver() put these previews in sentPreviews, and the history and lightbox read them through previewFor();
   // keep them alive until the pane closes, as the immediate-send path does.
   for (const picture of item.payload) sentUrls.push(picture.preview)
   queued = dropQueuedPrompt(queued, item.id)
   queueNote = queued.length ? 'Sent one queued message. The next waits for this turn.' : 'Queued message sent.'
   awaitingWork = true
   clearTimeout(awaitingTimer)
   awaitingTimer = setTimeout(() => { awaitingWork = false }, 4000)
   nearBottom = true
   await scrollToEnd()
  } finally { dispatching = false }
 }
 function editQueued(id: string) {
  const item = queued.find(entry => entry.id === id)
  if (!item) return
  queued = dropQueuedPrompt(queued, id)
  draft = item.text
  images = [...images, ...item.payload]
  queueHalted = false; queueNote = ''
  void tick().then(() => { composerInput?.focus(); composerInput?.setSelectionRange(draft.length, draft.length) })
 }
 function removeQueued(id: string) {
  const item = queued.find(entry => entry.id === id)
  if (!item) return
  for (const picture of item.payload) URL.revokeObjectURL(picture.preview)
  queued = dropQueuedPrompt(queued, id)
  if (!queued.length) { queueHalted = false; queueNote = '' }
 }
 function retryQueued(id: string) {
  queued = promoteQueuedPrompt(queued, id)
  queueHalted = false; queueNote = 'Trying that message again…'
  void tick().then(() => { if (!working && !sending && !uploading) void dispatchQueued() })
 }
 // A free agent with a queued message sends it. Waiting for an approval or an answer counts as busy: the person's reply
 // comes first. A lost connection waits; a failed send stops the queue until someone retries or changes it.
 // Seeing the turn start releases the next message; the timer is only a fallback if a status never arrives.
 $effect(() => {
  if (!working) return
  untrack(() => { awaitingWork = false; clearTimeout(awaitingTimer) })
 })
 $effect(() => {
  const ready = queued.length > 0 && !queueHalted && !awaitingWork && !working && !pending.length && !asked.length && !ended
   && chat.connection === 'open' && !sending && !uploading && !changingModel && !dispatching
  if (!ready) return
  untrack(() => { void dispatchQueued() })
 })
 async function stop() {
  feedback = ''
  try { await chatCancel(session.id) }
  catch (e) { feedback = e instanceof Error ? e.message : 'That reet session could not be stopped.' }
 }
 async function answer(permissionId: string, allow: boolean, forSession = false) {
  if (answering) return
  answering = permissionId; feedback = ''
  try { await chatAnswer(session.id, permissionId, allow, forSession) }
  catch (e) { feedback = e instanceof Error ? e.message : 'That answer was not sent.' }
  finally { answering = '' }
 }
 // A typed question. The draft is what the person has typed so far; the machine checks every answer again and is the
 // authority, so a refusal from it is shown on the card and the question stays open.
 type QuestionItem = Extract<ChatItem, { kind: 'question' }>
 let drafts = $state<Record<string, QuestionDraft>>({}), questionErrors = $state<Record<string, Record<string, string>>>({})
 let questionNote = $state<Record<string, string>>({}), sentAnswers = $state<Record<string, QuestionAnswers>>({}), asking = $state('')
 function draftOf(item: QuestionItem): QuestionDraft { return drafts[item.questionId] ?? emptyDraft(item.question) }
 function setField(item: QuestionItem, key: string, value: string | string[]) {
  drafts[item.questionId] = { ...draftOf(item), [key]: value }
  if (own(questionErrors[item.questionId], key)) { const { [key]: _fixed, ...rest } = questionErrors[item.questionId]; questionErrors[item.questionId] = rest }
 }
 function toggleChoice(item: QuestionItem, key: string, value: string, on: boolean) {
  const current = own(draftOf(item), key)
  const list = Array.isArray(current) ? current : []
  setField(item, key, on ? [...list.filter(v => v !== value), value] : list.filter(v => v !== value))
 }
 async function respondQuestion(item: QuestionItem, action: 'accept' | 'decline' | 'cancel') {
  if (asking) return
  questionNote[item.questionId] = ''
  let answers: QuestionAnswers | undefined
  if (action === 'accept') {
   const check = checkQuestionDraft(item.question, draftOf(item))
   questionErrors[item.questionId] = check.errors
   if (!check.ok) {
    await tick()
    document.querySelector<HTMLElement>(`[data-question="${CSS.escape(item.questionId)}"] [aria-invalid="true"], [data-invalid="true"]`)?.focus()
    return
   }
   answers = check.answers
  }
  asking = item.questionId
  try { await chatQuestionAnswer(session.id, item.questionId, action, answers); if (answers) sentAnswers[item.questionId] = answers }
  catch (e) { questionNote[item.questionId] = e instanceof Error ? e.message : 'That answer was not sent.' }
  finally { asking = '' }
 }
 function questionDecision(item: QuestionItem): string {
  if (item.decision === 'answered') return 'Answered.'
  if (item.decision === 'declined') return 'Skipped.'
  return item.reason === 'timeout' ? 'Closed: the question timed out.' : 'Closed without an answer.'
 }
 function keydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault(); void send()
 }
 function isOpen(key: string) { return expanded[key] ?? false }
 function toggle(key: string) { expanded[key] = !isOpen(key) }
 function lineCount(text: string) { return text ? text.replace(/\n$/, '').split('\n').length : 0 }
 function diffClass(line: string) {
  if (line.startsWith('+++') || line.startsWith('---')) return 'line meta'
  if (line.startsWith('@@')) return 'line hunk'
  if (line.startsWith('+')) return 'line add'
  if (line.startsWith('-')) return 'line del'
  return 'line'
 }
 async function copyBlock(key: string, text: string) {
  if (await copyText(text)) {
   copied = key
   if (copyTimer !== undefined) clearTimeout(copyTimer)
   copyTimer = setTimeout(() => { copied = '' }, 1500)
  } else feedback = 'Clipboard access was denied. Use your browser’s copy command.'
 }
 const ICON_PATHS: Record<string, string[]> = {
  execute: ['M4 17l6-6-6-6', 'M12 19h8'],
  read: ['M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z', 'M14 2v6h6'],
  edit: ['M12 20h9', 'M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z'],
  delete: ['M3 6h18', 'M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2', 'M19 6l-1 14a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1L5 6'],
  move: ['M5 9l-3 3 3 3', 'M9 5l3-3 3 3', 'M15 19l-3 3-3-3', 'M19 9l3 3-3 3', 'M2 12h20', 'M12 2v20'],
  search: ['M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14z', 'M20 20l-3.5-3.5'],
  fetch: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M3 12h18', 'M12 3c2.5 2.5 4 5.6 4 9s-1.5 6.5-4 9c-2.5-2.5-4-5.6-4-9s1.5-6.5 4-9z'],
  think: ['M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z'],
  switch_mode: ['M4 7h16M4 17h16', 'M9 4l-3 3 3 3', 'M15 14l3 3-3 3'],
  other: ['M14.7 6.3a4 4 0 0 1-5.4 5.4L4 17v3h3l5.3-5.3a4 4 0 0 1 5.4-5.4z'],
 }
 function iconPaths(kind: string) { return ICON_PATHS[kind] ?? ICON_PATHS.other }

 onMount(() => {
  const life = new AbortController()
  void machineModels(session.agentId, session.tool || 'opencode', life.signal).then(result => { models = result.models.map(model => model.id) }).catch(() => { if (!life.signal.aborted) modelNote = 'Model list unavailable. Enter a model ID configured on this machine.' })
  void listModelFavourites(life.signal).then(answer => { if (!life.signal.aborted) favourites = answer.favourites }).catch(() => {})
  const stream = new EventSource(chatURL(session.id))
  source = stream
  stream.addEventListener('reset', readReset)
  stream.addEventListener('chat', readEvent)
  stream.onopen = () => { chat = { ...chat, connection: 'open' } }
  stream.onerror = () => { if (alive) { chat = { ...chat, connection: 'reconnecting' }; stopSettle(); replayed = false } }
  return () => { alive = false; life.abort(); skillsLife?.abort(); pendingEvents = []; stopFlush(); stopSettle(); if (copyTimer !== undefined) clearTimeout(copyTimer); closeStream() }
 })
 onDestroy(() => { clearTimeout(awaitingTimer); for (const image of images) URL.revokeObjectURL(image.preview); for (const url of sentUrls) URL.revokeObjectURL(url); for (const item of queued) for (const image of item.payload) URL.revokeObjectURL(image.preview) })
</script>

{#snippet inline(text: string)}
 {#each inlineSegments(text) as part, n (n)}{#if part.kind === 'code'}<code>{part.text}</code>{:else if part.kind === 'strong'}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
{/snippet}

<section class="chat-pane" aria-label={`reet TUI in ${session.title}`} ondragover={(event) => { if (canAttach) { event.preventDefault(); dropping = true } }} ondragleave={(event) => { if (event.currentTarget === event.target || !(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node)) dropping = false }} ondrop={(event) => { dropping = false; dropped(event) }}>
 <div class="chat-status" role="status" aria-live="polite">
  <span class="status-dot" class:busy={working} class:wait={pending.length > 0} class:down={chat.connection === 'reconnecting'} aria-hidden="true"></span>
  <strong>{statusWord}</strong>
  {#if statusDetail && !failed}<span class="detail">{statusDetail}</span>{/if}
 </div>
 {#if failed}
  <div class="error-panel" role="alert">
   <strong>{toolName} could not continue</strong>
   <p>{guidance}</p>
   {#if statusDetail}<details open><summary>Error details</summary><p>{statusDetail}</p></details>{/if}
  </div>
 {/if}
 {#if chat.truncated}<p class="notice bad">Earlier messages in this reet session are no longer available.</p>{/if}
 {#if feedback}<p class="notice bad" role="alert">{feedback}</p>{/if}
 <div class="log" role="log" bind:this={log} onscroll={onScroll}>
  {#if chat.items.length === 0}
   <div class="empty"><h3>Ask {toolName} to change this project.</h3><p>It asks before it edits a file or runs a command. Allow once for one card, or allow that command or file for the rest of this session.</p></div>
  {/if}
  {#each chat.items as item (item.key)}
   {#if item.kind === 'user'}
    {@const att = attachmentsOf(item)}
    {@const shown = att.refs.map(ref => ({ ref, preview: previewFor(ref) }))}
    {@const gallery = shown.flatMap(entry => entry.preview ? [entry.preview] : [])}
    <div class="row user"><div class="bubble user" class:has-images={shown.length > 0}>
     {#if shown.length}
      <div class="thumbs" class:single={shown.length === 1}>
       {#each shown as entry (entry.ref)}
        {#if entry.preview}
         <button type="button" class="thumb" aria-label={`Preview ${entry.preview.name}`} title="Click to enlarge" onclick={() => openPreview(gallery, gallery.indexOf(entry.preview!))}><img src={entry.preview.url} alt={entry.preview.name} loading="lazy" /><span class="zoom" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" /></svg></span></button>
        {:else}
         <span class="thumb ghost" title={entry.ref}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2.5" /><circle cx="9" cy="10" r="1.6" /><path d="M21 16l-5-5-8 9" /></svg><span>{baseName(entry.ref) || 'Image'}</span></span>
        {/if}
       {/each}
      </div>
     {/if}
     {#if att.text}<div class="said">{att.text}</div>{/if}
    </div></div>
   {:else if item.kind === 'agent'}
    <div class="row agent">
     <div class="bubble agent">
      {#each textSegments(item.text) as segment, i (i)}
       {#if segment.kind === 'code'}
        <div class="code">
         <div class="code-head">
          <span>{segment.lang || 'code'}</span>
          <button type="button" class="copy" onclick={() => void copyBlock(`${item.key}-${i}`, segment.text)}>{copied === `${item.key}-${i}` ? 'Copied' : 'Copy'}</button>
         </div>
         <pre>{segment.text}</pre>
        </div>
       {:else if segment.text.trim()}
        {#each proseBlocks(segment.text) as block, j (j)}
         {#if block.kind === 'h'}
          {#if block.level === 1}<h4 class="prose-h">{@render inline(block.text)}</h4>
          {:else if block.level === 2}<h5 class="prose-h">{@render inline(block.text)}</h5>
          {:else}<h6 class="prose-h">{@render inline(block.text)}</h6>{/if}
         {:else if block.kind === 'ul'}
          <ul class="prose-list">{#each block.items as item, k (k)}<li>{@render inline(item)}</li>{/each}</ul>
         {:else if block.kind === 'ol'}
          <ol class="prose-list">{#each block.items as item, k (k)}<li>{@render inline(item)}</li>{/each}</ol>
         {:else if block.kind === 'quote'}
          <blockquote class="prose-quote">{@render inline(block.text)}</blockquote>
         {:else}
          <p class="prose">{@render inline(block.text)}</p>
         {/if}
        {/each}
       {/if}
      {/each}
     </div>
    </div>
   {:else if item.kind === 'notice'}
    <p class="notice" class:bad={item.bad}>{item.text}</p>
   {:else if item.kind === 'tool'}
    {#if item.toolKind === 'think'}
     {@const think = toolState(item.status)}
     {@const preview = thoughtPreview(item.text)}
     <div class="think-row">
      <svelte:element this={item.text ? 'button' : 'div'} type={item.text ? 'button' : undefined} role={item.text ? 'button' : undefined} class="think-head"
        aria-expanded={item.text ? isOpen(item.key) : undefined} onclick={item.text ? () => toggle(item.key) : undefined}>
       <span class="spark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">{#each iconPaths('think') as d}<path d={d} />{/each}</svg></span>
       <span class="think-label">Thinking</span>
       <span class="think-preview">{preview || 'Reasoning about the next step…'}</span>
       <span class="state {think.tone}">{think.word}</span>
       {#if item.text}<span class="chevron" class:open={isOpen(item.key)} aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg></span>{/if}
      </svelte:element>
      {#if item.text && isOpen(item.key)}<pre class="think-body">{item.text}</pre>{/if}
     </div>
    {:else}
     {@const state = toolState(item.status)}
     {@const target = toolTarget(item, session.dir)}
     {@const counts = item.additions || item.deletions || !item.diff ? { additions: item.additions, deletions: item.deletions } : diffCounts(item.diff)}
     {@const lines = lineCount(item.text)}
     {@const hasBody = !!(item.command || item.diff || item.text)}
     {@const open = isOpen(item.key)}
     <div class="tool" data-kind={item.toolKind || 'other'} class:open={open} class:failed={item.status === 'failed'}>
      <svelte:element this={hasBody ? 'button' : 'div'} type={hasBody ? 'button' : undefined} role={hasBody ? 'button' : undefined} class="tool-head"
        aria-expanded={hasBody ? open : undefined} onclick={hasBody ? () => toggle(item.key) : undefined}>
       <span class="dot {state.tone}" aria-hidden="true"></span>
       <span class="ticon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">{#each iconPaths(item.toolKind) as d}<path d={d} />{/each}</svg></span>
       <span class="kind">{toolLabel(item.toolKind)}</span>
       <span class="target">{#if item.toolKind === 'execute'}<span class="prompt" aria-hidden="true">$</span>{' '}{/if}{target}</span>
       <span class="meta">
        {#if counts.additions || counts.deletions}<span class="add">+{counts.additions}</span><span class="del">−{counts.deletions}</span>{/if}
        {#if lines > 1}<span class="lines">{lines} {item.toolKind === 'search' ? (lines === 1 ? 'match' : 'matches') : 'lines'}</span>{/if}
        {#if item.durationMs > 0}<span class="time">{formatDuration(item.durationMs)}</span>{/if}
       </span>
       <span class="state {state.tone}">{state.word}</span>
       {#if hasBody}<span class="chevron" class:open={open} aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg></span>{/if}
      </svelte:element>
      {#if hasBody && open}
       <div class="tool-body">
        {#if item.toolKind === 'execute' && item.command && item.command !== item.title}
         <div class="block"><span class="block-label">Command</span><pre class="command"><span class="prompt" aria-hidden="true">$</span>{' '}{item.command}</pre></div>
        {/if}
        {#if item.diff}
         <div class="block"><span class="block-label">{item.status === 'failed' ? 'Diff' : `Diff · ${counts.additions} added, ${counts.deletions} removed`}</span>
          <pre class="diff">{#each item.diff.split('\n') as line, i (i)}<span class={diffClass(line)}>{line || ' '}</span>{/each}</pre></div>
        {:else if item.text}
         <div class="block"><span class="block-label">{item.status === 'failed' ? 'Error' : 'Output'}</span><pre>{item.text}</pre></div>
        {:else if item.status === 'in_progress' || item.status === 'pending'}
         <p class="placeholder">Running…</p>
        {:else}
         <p class="placeholder">No output.</p>
        {/if}
        {#if item.locations.length}
         <p class="where">{#each item.locations as location, i (i)}<span class="path" title={location}>{displayPath(location, session.dir)}</span>{#if i < item.locations.length - 1}{' · '}{/if}{/each}</p>
        {/if}
       </div>
      {/if}
     </div>
    {/if}
   {:else if item.kind === 'permission'}
    {@const approvalCounts = item.diff ? diffCounts(item.diff) : { additions: 0, deletions: 0 }}
    {@const decided = !!item.decision}
    {@const open = decided ? isOpen(item.key) : true}
    <div class="approval" class:decided={decided} class:open={decided && open} role="group" aria-label="Approval needed">
     <svelte:element this={decided ? 'button' : 'div'} type={decided ? 'button' : undefined} role={decided ? 'button' : undefined} class="approval-head"
       aria-expanded={decided ? open : undefined} onclick={decided ? () => toggle(item.key) : undefined}>
      <span class="ticon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 16H3z" /><path d="M12 9v4M12 16h.01" /></svg></span>
      <span class="kind">{toolLabel(item.toolKind)}</span>
      <strong>{item.title}</strong>
      <span class="state">{item.decision ? 'Answered' : 'Needs you'}</span>
      {#if decided}<span class="chevron" class:open={open} aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg></span>{/if}
     </svelte:element>
     {#if open}
      {#if item.path}<p class="approval-path" title={item.path}>{displayPath(item.path, session.dir)}</p>{/if}
      {#if item.command}<div class="block"><span class="block-label">Command</span><pre class="command"><span class="prompt" aria-hidden="true">$</span>{' '}{item.command}</pre></div>{/if}
      {#if item.diff}<div class="block"><span class="block-label">Diff · {approvalCounts.additions} added, {approvalCounts.deletions} removed</span><pre class="diff">{#each item.diff.split('\n') as line, i (i)}<span class={diffClass(line)}>{line || ' '}</span>{/each}</pre></div>{/if}
      {#if item.truncated}<p class="notice bad">This change is too large to review in full here. Deny it and use {toolName} in a terminal if you need to see all of it.</p>{/if}
      {#if item.decision}
       <p class="decision">{item.decision === 'allowed' ? 'Allowed once.' : item.decision === 'denied' ? 'Denied.' : item.reason === 'timeout' ? 'Closed: the approval timed out.' : 'Closed without an answer.'}</p>
      {:else}
       <div class="approval-actions">
        <button type="button" class="btn" disabled={!!answering} onclick={() => void answer(item.permissionId, false)}>Deny</button>
        <button type="button" class="btn primary" disabled={!!answering || item.truncated} onclick={() => void answer(item.permissionId, true)}>Allow once</button>
        {#if (item.command || item.path) && !item.truncated}
         <button type="button" class="btn" disabled={!!answering} onclick={() => void answer(item.permissionId, true, true)}>{item.command ? 'Allow this command for this session' : 'Allow this file for this session'}</button>
        {/if}
       </div>
      {/if}
     {/if}
    </div>
   {:else if item.kind === 'question'}
    {@const draft = draftOf(item)}
    {@const errors = questionErrors[item.questionId]}
    {@const busy = asking === item.questionId}
    <form class="approval question" class:decided={item.decision} role="group" aria-labelledby={`${item.key}-message`} data-question={item.questionId} novalidate
     onsubmit={event => { event.preventDefault(); void respondQuestion(item, 'accept') }}>
     <div class="approval-head">
      <span class="ticon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9" /><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1 .9-1 1.7M12 17h.01" /></svg></span>
      <span class="kind">Question</span>
      <span class="state">{item.decision ? 'Answered' : 'Needs you'}</span>
     </div>
     <p class="question-message" id={`${item.key}-message`}>{item.question.message}</p>
     {#if item.decision}
      <p class="decision">{questionDecision(item)}</p>
      {#if item.decision === 'answered' && sentAnswers[item.questionId]}
       <dl class="question-summary">
        {#each summarizeAnswers(item.question, sentAnswers[item.questionId]) as line (line.key)}
         <dt>{line.label}</dt><dd>{line.text}</dd>
        {/each}
       </dl>
      {/if}
     {:else}
      {#each item.question.fields as field (field.key)}
       {@const id = `${item.key}-${field.key}`}
       {@const invalid = !!own(errors, field.key)}
       {@const describedBy = [field.description ? `${id}-help` : '', invalid ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined}
       {#if field.kind === 'text'}
        <div class="qfield">
         <label class="qlegend" for={id}>{fieldLabel(field)}{#if !field.required}{' '}<span class="qopt">(optional)</span>{/if}</label>
         {#if field.description}<p class="qhelp" id={`${id}-help`}>{field.description}</p>{/if}
         <textarea {id} rows="2" value={typeof own(draft, field.key) === 'string' ? own(draft, field.key) : ''} disabled={busy}
          aria-invalid={invalid ? 'true' : undefined} aria-describedby={describedBy} aria-required={field.required ? 'true' : undefined}
          oninput={event => setField(item, field.key, (event.currentTarget as HTMLTextAreaElement).value)}></textarea>
         {#if invalid}<p class="qerror" id={`${id}-error`}>{own(errors, field.key)}</p>{/if}
        </div>
       {:else}
        <fieldset class="qfield" tabindex="-1" data-invalid={invalid ? 'true' : undefined} aria-describedby={describedBy} disabled={busy}>
         <legend class="qlegend">{fieldLabel(field)}{#if !field.required}{' '}<span class="qopt">(optional)</span>{/if}</legend>
         {#if field.description}<p class="qhelp" id={`${id}-help`}>{field.description}</p>{/if}
         {#if field.kind === 'multi' && (field.maxItems ?? 0) > 0 && (field.maxItems ?? 0) < (field.options?.length ?? 0)}<p class="qhelp">Choose up to {field.maxItems}.</p>{/if}
         {#each field.options ?? [] as option (option.value)}
          <label class="qoption">
           <input type={field.kind === 'single' ? 'radio' : 'checkbox'} name={id} value={option.value}
            checked={field.kind === 'single' ? own(draft, field.key) === option.value : Array.isArray(own(draft, field.key)) && (own(draft, field.key) as string[]).includes(option.value)}
            onchange={event => field.kind === 'single' ? setField(item, field.key, option.value) : toggleChoice(item, field.key, option.value, (event.currentTarget as HTMLInputElement).checked)} />
           <span><span class="qlabel">{option.label}</span>{#if option.description}<span class="qdesc">{option.description}</span>{/if}</span>
          </label>
         {/each}
         {#if invalid}<p class="qerror" id={`${id}-error`}>{own(errors, field.key)}</p>{/if}
        </fieldset>
       {/if}
      {/each}
      {#if questionNote[item.questionId]}<p class="notice bad" role="alert">{questionNote[item.questionId]}</p>{/if}
      <div class="approval-actions">
       <button type="button" class="btn" disabled={!!asking} title="Stops what the agent was doing" onclick={() => void respondQuestion(item, 'cancel')}>Cancel question</button>
       <button type="button" class="btn" disabled={!!asking} title="Let the agent carry on without an answer" onclick={() => void respondQuestion(item, 'decline')}>Skip</button>
       <button type="submit" class="btn primary" disabled={!!asking}>{busy ? 'Sending…' : 'Send answer'}</button>
      </div>
     {/if}
    </form>
   {/if}
  {/each}
 </div>
 {#if ended}
  <p class="composer-note">{endedReason(session.state, toolName)}{#if queued.length} {queued.length} queued {queued.length === 1 ? 'message was' : 'messages were'} not sent.{/if}</p>
 {:else}
  <div class="composer" onpaste={pasted} class:dropping={dropping}>
   {#if queued.length}
    <section class="queue" aria-label="Queued messages">
     <header class="queue-head">
      <span class="queue-title"><span class="queue-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" /></svg></span>Queued <b>{queued.length}</b></span>
      <span class="queue-said" role="status">{queueNote || (working ? 'Sent when this turn finishes.' : 'Waiting to send…')}</span>
     </header>
     <ol class="queue-list">
      {#each queued as item, i (item.id)}
       <li class:first={i === 0}>
        <span class="queue-text" title={item.text}>{item.text || 'Image only'}</span>
        {#if item.payload.length}<span class="queue-attach" title={`${item.payload.length} images`}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M21.4 11.6l-8.5 8.5a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8" /></svg>{item.payload.length}</span>{/if}
        <span class="queue-actions">
         {#if queueHalted && i === 0}<button type="button" class="retry" onclick={() => retryQueued(item.id)}>Retry</button>{/if}
         <button type="button" onclick={() => editQueued(item.id)} aria-label={`Edit queued message: ${item.text.slice(0, 60) || 'image only'}`}>Edit</button>
         <button type="button" class="remove" onclick={() => removeQueued(item.id)} aria-label={`Remove queued message: ${item.text.slice(0, 60) || 'image only'}`}>Remove</button>
        </span>
       </li>
      {/each}
     </ol>
    </section>
   {/if}
   {#if modelNote}<p class="notice" role="status">{modelNote}</p>{/if}
   {#if images.length}
    <ul class="attachments" aria-label="Images to attach">
     {#each images as image, i (image.id)}
      <li in:fly={{ y: 8, duration: 160 }}>
       <button type="button" class="tile" aria-label={`Preview ${image.file.name}`} title={`${image.file.name} · ${formatImageSize(image.file.size)}`} onclick={() => openPreview(images.map(item => ({ url: item.preview, name: item.file.name })), i)}><img src={image.preview} alt="" /></button>
       <span class="tile-name">{image.file.name} · {formatImageSize(image.file.size)}</span>
       <button type="button" class="tile-remove" aria-label={`Remove ${image.file.name}`} disabled={uploading} onclick={() => removeImage(image.id)}>×</button>
      </li>
     {/each}
    </ul>
   {/if}
   {#if attachNote}<p class="notice" class:bad={/off|does not|at most|more than|Use PNG|Each image|empty/.test(attachNote)}>{attachNote}</p>{/if}
   <label class="vh" for={`${session.id}-chat-prompt`}>Message {toolName}</label>
   <div class="field" class:busy={working}>
    <span class="field-prompt" aria-hidden="true">›</span>
    <textarea id={`${session.id}-chat-prompt`} class="composer-input" bind:this={composerInput} bind:value={draft} oninput={typed} rows="2" maxlength="2000" placeholder={working || queued.length ? 'Queue a message to steer the next turn…' : `Ask ${toolName} to change this project…`} onkeydown={keydown} disabled={sending || uploading || changingModel || chat.connection === 'reconnecting'}></textarea>
    <div class="field-actions">
     {#if working}<button type="button" class="btn stop" onclick={() => void stop()}><svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><rect x="6" y="6" width="12" height="12" rx="2" /></svg>Stop</button>{/if}
     <button type="button" class="btn primary send" disabled={sending || uploading || changingModel || (!draft.trim() && !images.length) || chat.connection === 'reconnecting'} onclick={() => void send()}>{uploading ? 'Uploading…' : sending ? 'Sending…' : working || queued.length ? 'Queue' : 'Send'}{#if working || queued.length}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" /></svg>{:else}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg>{/if}</button>
    </div>
   </div>
   <div class="composer-bar">
    <div class="composer-tools">
     <button type="button" class="skills-btn" aria-label="Skills" aria-haspopup="dialog" title={skillsOff || 'Insert a skill into your message'} disabled={!!skillsOff} onclick={() => openSkills()}>+ Skills</button>
     {#if canAttach}<button type="button" class="icon-btn" aria-label="Attach image" title="Attach image · or paste or drop one" disabled={sending || uploading || changingModel || working || images.length >= imageLimit} onclick={() => fileInput?.click()}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21.4 11.6l-8.5 8.5a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8" /></svg></button>{/if}
     <div class="model-controls" data-keeps-focus>
      <label for={`${session.id}-model`}>Model</label>
      <input id={`${session.id}-model`} data-combo-list={`${session.id}-models`} bind:value={selectedModel} disabled={working || sending || changingModel || chat.connection !== 'open'} placeholder="provider/model" spellcheck="false" autocomplete="off" />
      <datalist id={`${session.id}-models`}>{#each models as model}<option value={model}></option>{/each}</datalist>
      {#if selectedModel.trim() && selectedModel !== currentModel}
       <button type="button" class="btn change" transition:fade={{ duration: 120 }} disabled={working || sending || changingModel || chat.connection !== 'open'} onclick={() => void changeModel()}>{changingModel ? 'Changing…' : 'Change model'}</button>
      {/if}
      <button type="button" class="star" class:on={selectedIsDefault} disabled={!selectedModel.trim() || favouriteBusy || chat.connection !== 'open'} aria-pressed={selectedIsDefault} aria-label={defaultLabel(tool, selectedModel.trim(), selectedIsDefault)} title={selectedIsDefault ? `Default for new ${tool} sessions. Click to clear.` : `Make this the default for new ${tool} sessions`} onclick={() => void toggleDefault()}>
       <svg viewBox="0 0 24 24" fill={selectedIsDefault ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9 6.8 19.7l1-5.9L3.5 9.7l5.9-.8z" /></svg>
       <span class="star-word">{favouriteBusy ? 'Saving…' : selectedIsDefault ? 'Default' : 'Make default'}</span>
      </button>
     </div>
    </div>
    <span class="hint">Enter to send · Shift + Enter for a new line</span>
   </div>
   {#if skillsOpen}<SkillPicker data={skillData} loading={skillsLoading} error={skillsError} tool={toolName} initialQuery={skillQuery} sessionId={session.id} onrefresh={() => void loadSkills()} onpick={pickSkill} onclose={() => void closeSkills()} onretry={() => void loadSkills()} />{/if}
   {#if canAttach}<input class="vh" bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple onchange={picked} />{/if}
  </div>
 {/if}
 {#if toast}{#key toast.id}<Toast message={toast.text} placement="right" tone={toast.bad ? 'bad' : 'ok'} duration={toast.bad ? 7000 : 3600} ondismiss={() => (toast = null)} />{/key}{/if}
 {#if lightbox}<ImageLightbox items={lightbox.items} index={lightbox.index} onclose={() => (lightbox = null)} />{/if}
</section>

<style>
 .error-panel { flex: none; margin: 12px 16px; padding: 16px; border: 2px solid var(--bad); border-radius: 10px; background: color-mix(in srgb, var(--bad) 12%, var(--bg)); color: var(--ink); overflow-wrap: anywhere; max-height: 35vh; overflow: auto; }
 .error-panel > strong { color: var(--bad); font-size: 1rem; }
 .error-panel p { margin: 8px 0; line-height: 1.5; }
 .error-panel summary { cursor: pointer; }
 /* The model is a quiet control in the composer's toolbar: a labelled field, a star for the default, and Change model only once the value differs. */
 .model-controls { display: flex; align-items: center; gap: 6px; min-width: 0; flex: 1 1 auto; max-width: 30rem; }
 .model-controls label { flex: none; font: 600 10px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--faint); }
 .model-controls input { flex: 1 1 8rem; min-width: 0; height: 30px; padding: 0 9px; border: 1px solid var(--line-soft); border-radius: 8px; background: var(--bg-sunk); color: var(--ink-soft); font: 12px var(--mono); text-overflow: ellipsis; transition: border-color .15s, background .15s; }
 .model-controls input:hover:not(:disabled) { border-color: var(--line); }
 .model-controls input:focus-visible { outline: none; border-color: var(--accent-text); background: var(--card); color: var(--ink); }
 .model-controls input:disabled { opacity: .6; }
 .model-controls .change { flex: none; min-height: 30px; padding: 0 10px; font-size: 12px; }
 .star { display: inline-flex; align-items: center; gap: 5px; flex: none; height: 30px; padding: 0 9px; border: 1px solid transparent; border-radius: 8px; background: none; color: var(--muted); font: 500 12px var(--sans); cursor: pointer; transition: color .15s, background .15s, border-color .15s; }
 .star svg { width: 15px; height: 15px; }
 .star:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
 .star.on { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, transparent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
 .star:disabled { opacity: .5; cursor: default; }
 .star:focus-visible, .tile:focus-visible, .thumb:focus-visible, .tile-remove:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
 .chat-pane { display: flex; flex-direction: column; min-height: 0; height: 100%; background: var(--bg); container: chat-pane / inline-size; }
 .chat-status { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 9px 18px; border-bottom: 1px solid var(--line-soft); font-size: 12px; color: var(--muted); background: var(--bg); }
 .chat-status strong { font-weight: 500; color: var(--ink); }
 .chat-status .detail { min-width: 0; color: var(--muted); overflow-wrap: anywhere; }
 .status-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--line-strong); flex: none; }
 .status-dot.busy { background: var(--accent-text); animation: pulse 1.6s ease-in-out infinite; }
 .status-dot.wait { background: var(--warn); }
 .status-dot.down { background: var(--bad); }
 @keyframes pulse { 50% { opacity: .35; } }
 @media (prefers-reduced-motion: reduce) { .status-dot.busy { animation: none; } }
 .log { flex: 1; min-height: 0; overflow: auto; padding: 14px clamp(12px, 2.4vw, 24px) 22px; display: flex; flex-direction: column; gap: 6px; scroll-behavior: smooth; }
 @media (prefers-reduced-motion: reduce) { .log { scroll-behavior: auto; } }
 /* Rows keep their natural height: the log scrolls, it never squashes a tool card to fit. */
 .log > * { flex: none; }
 .empty { margin: auto; max-width: 40ch; text-align: center; color: var(--muted); }
 .empty h3 { margin: 0 0 6px; font: 600 17px/1.3 var(--serif); color: var(--ink); }
 .empty p { margin: 0; font-size: 12.5px; line-height: 1.55; }
 /* Messages: the person on the right in a warm bubble, the agent as prose at a readable measure. */
 .row { display: flex; min-width: 0; }
 .row.user { justify-content: flex-end; margin: 12px 0 2px; }
 .row.agent { justify-content: flex-start; margin: 4px 0 2px; }
 .bubble { min-width: 0; max-width: min(78ch, 100%); }
 .bubble.user { padding: 8px 12px; border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line)); border-radius: 14px 14px 4px 14px; background: color-mix(in srgb, var(--accent) 13%, var(--card)); color: var(--ink); font-size: 13.5px; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
 .bubble.agent { max-width: min(84ch, 100%); padding: 2px 2px 4px; color: var(--ink); font-size: 13.5px; line-height: 1.65; }
 .prose { margin: 0 0 8px; white-space: pre-wrap; overflow-wrap: anywhere; }
 .bubble.agent > :last-child { margin-bottom: 0; }
 .prose-h { margin: 14px 0 6px; font: 600 1.02rem/1.35 var(--sans); color: var(--ink); }
 .prose-h:first-child { margin-top: 2px; }
 .prose-list { margin: 0 0 8px; padding-left: 1.35em; }
 .prose-list li { margin: 2px 0; }
 .prose-list li::marker { color: var(--faint); }
 .prose-quote { margin: 0 0 8px; padding: 2px 0 2px 11px; border-left: 2px solid var(--line-strong); color: var(--muted); font-style: italic; white-space: pre-wrap; overflow-wrap: anywhere; }
 .prose code, .prose-list code, .prose-quote code { padding: 1px 4px; border: 1px solid var(--line-soft); border-radius: 4px; background: var(--bg-sunk); font: 11.5px var(--mono); color: var(--ink); }
 .prose strong, .prose-list strong { font-weight: 600; color: var(--ink); }
 .code { margin: 9px 0; border: 1px solid var(--line); border-radius: 9px; background: var(--bg-sunk); overflow: hidden; }
 .code-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 4px 8px 4px 11px; border-bottom: 1px solid var(--line-soft); font: 600 10px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--faint); }
 .code pre { margin: 0; padding: 10px 12px; max-height: 24rem; overflow: auto; font: 12px/1.55 var(--mono); color: var(--ink-soft); white-space: pre; }
 .copy { padding: 3px 7px; border: 1px solid transparent; border-radius: 5px; background: none; color: var(--faint); font: 500 10px var(--sans); letter-spacing: .04em; cursor: pointer; }
 .copy:hover { border-color: var(--line); color: var(--ink); }
 .notice { margin: 2px 0; padding: 7px 10px; border-left: 2px solid var(--line-strong); border-radius: 0 7px 7px 0; background: var(--bg-sunk); color: var(--ink-soft); font-size: 12px; }
 .notice.bad { border-left-color: var(--bad); color: var(--bad); }
 /* A tool: one line at rest, with the detail folded under it. */
 .tool { margin: 1px 0; border: 1px solid var(--line-soft); border-radius: 9px; background: var(--card); overflow: hidden; }
 .tool:hover { border-color: var(--line); background: var(--card-raised); }
 .tool.open { background: var(--card); border-color: var(--line); }
 .tool.failed { border-color: color-mix(in srgb, var(--bad) 42%, var(--line)); }
 .tool-head { display: grid; grid-template-columns: 8px 16px auto minmax(0, 1fr) auto auto 14px; align-items: center; gap: 9px; width: 100%; min-height: 38px; padding: 6px 10px; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; }
 div.tool-head { cursor: default; }
 .tool-head:focus-visible, .think-head:focus-visible, .copy:focus-visible, .icon-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 7px; }
 .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--line-strong); flex: none; }
 .dot.good { background: var(--good); }
 .dot.warn { background: var(--warn); }
 .dot.bad { background: var(--bad); }
 .dot.muted { background: var(--faint); }
 .dot.busy { background: var(--accent-text); animation: pulse 1.4s ease-in-out infinite; }
 .ticon { display: grid; place-items: center; color: var(--muted); }
 .ticon svg { width: 15px; height: 15px; }
 .kind { font: 600 10px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--muted); white-space: nowrap; }
 .target { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 12px/1.4 var(--mono); color: var(--ink-soft); }
 .prompt { color: var(--faint); }
 .meta { display: inline-flex; align-items: center; gap: 7px; font: 11px var(--mono); color: var(--faint); font-variant-numeric: tabular-nums; white-space: nowrap; }
 .meta .add { color: var(--good); }
 .meta .del { color: var(--bad); }
 .state { font: 500 11px var(--sans); color: var(--muted); white-space: nowrap; }
 .state.good { color: var(--good); }
 .state.warn { color: var(--warn); }
 .state.bad { color: var(--bad); }
 .state.busy { color: var(--accent-text); }
 .chevron { display: grid; place-items: center; color: var(--faint); transition: transform .15s var(--ease-out); }
 .chevron svg { width: 13px; height: 13px; }
 .chevron.open { transform: rotate(180deg); }
 @media (prefers-reduced-motion: reduce) { .chevron { transition: none; } }
 .tool-body { display: flex; flex-direction: column; gap: 10px; padding: 10px 12px 12px; border-top: 1px solid var(--line-soft); background: var(--bg-sunk); }
 .block { min-width: 0; }
 .block-label { display: block; margin-bottom: 5px; font: 600 9.5px var(--sans); letter-spacing: .1em; text-transform: uppercase; color: var(--faint); }
 .tool-body pre, .approval pre { margin: 0; padding: 8px 10px; max-height: 18rem; overflow: auto; border: 1px solid var(--line-soft); border-radius: 7px; background: var(--bg); color: var(--ink-soft); font: 11.5px/1.5 var(--mono); white-space: pre-wrap; overflow-wrap: anywhere; }
 .tool-body pre.diff, .approval pre.diff { white-space: pre; overflow-x: auto; }
 .diff .line { display: block; min-height: 1.5em; }
 .diff .line.add { color: var(--good); }
 .diff .line.del { color: var(--bad); }
 .diff .line.hunk { color: var(--info); }
 .diff .line.meta { color: var(--faint); }
 .where { margin: 0; font: 11px/1.5 var(--mono); color: var(--faint); overflow-wrap: anywhere; }
 .placeholder { margin: 0; font-size: 12px; font-style: italic; color: var(--muted); }
 /* Thinking is a quiet aside, not a tool call: no card, no border, one line until opened. */
 .think-row { margin: 0; }
 .think-head { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 30px; padding: 4px 10px; border: 0; border-radius: 7px; background: none; color: var(--muted); text-align: left; cursor: pointer; }
 .think-head:hover { background: color-mix(in srgb, var(--ink) 4%, transparent); }
 .spark { display: grid; place-items: center; color: var(--faint); }
 .spark svg { width: 13px; height: 13px; }
 .think-label { flex: none; font: 500 12px var(--sans); color: var(--muted); }
 .think-preview { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: italic 12px var(--sans); color: var(--faint); }
 .think-body { margin: 0 10px 8px 31px; padding: 8px 10px; max-height: 12rem; overflow: auto; border-left: 2px solid var(--line); color: var(--ink-soft); font: italic 12px/1.6 var(--sans); white-space: pre-wrap; overflow-wrap: anywhere; }
 /* An approval is the one card that asks for something, so it carries the warning tone. */
 .approval { display: flex; flex-direction: column; gap: 9px; margin: 6px 0; padding: 11px 12px; border: 1px solid color-mix(in srgb, var(--warn) 38%, var(--line)); border-left: 3px solid var(--warn); border-radius: 10px; background: color-mix(in srgb, var(--warn) 5%, var(--card)); }
 .approval.decided { border-color: var(--line); border-left-color: var(--line-strong); background: var(--card); }
 .approval-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
 button.approval-head { width: 100%; padding: 0; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; }
 button.approval-head:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 5px; }
 .approval-head strong { font: 500 13px var(--sans); color: var(--ink); }
 .approval-head .state { margin-left: auto; color: var(--warn); }
 .approval.decided .approval-head .state { color: var(--muted); }
 .approval-path { margin: 0; font: 11.5px var(--mono); color: var(--ink-soft); overflow-wrap: anywhere; }
 .approval-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
 .approval-actions .btn { min-height: 36px; padding: 0 13px; font-size: 13px; }
 .decision { margin: 0; font-size: 12px; color: var(--muted); }
 .question-message { margin: 0; font-size: 13.5px; line-height: 1.5; color: var(--ink); white-space: pre-wrap; overflow-wrap: anywhere; }
 .qfield { display: flex; flex-direction: column; gap: 6px; min-width: 0; margin: 0; padding: 0; border: 0; }
 .qfield:focus { outline: none; }
 .qlegend { padding: 0; font: 500 12.5px var(--sans); color: var(--ink); }
 .qopt { font-weight: 400; color: var(--muted); }
 .qhelp { margin: 0; font-size: 12px; color: var(--muted); }
 .qoption { display: flex; align-items: flex-start; gap: 9px; min-height: 32px; padding: 4px 2px; cursor: pointer; }
 .qoption input { flex: none; margin: 3px 0 0; accent-color: var(--accent, currentColor); }
 .qlabel { display: block; font-size: 13px; color: var(--ink); overflow-wrap: anywhere; }
 .qdesc { display: block; margin-top: 1px; font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
 .qfield textarea { width: 100%; box-sizing: border-box; padding: 8px 10px; border: 1px solid var(--line); border-radius: 7px; background: var(--bg); color: var(--ink); font: 13px/1.45 var(--sans); resize: vertical; }
 .qfield textarea[aria-invalid='true'] { border-color: var(--bad); }
 fieldset.qfield[data-invalid='true'] { padding: 8px 10px; border: 1px solid var(--bad); border-radius: 7px; }
 .qerror { margin: 0; font-size: 12px; color: var(--bad); }
 .question-summary { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0; font-size: 12.5px; }
 .question-summary dt { color: var(--muted); }
 .question-summary dd { margin: 0; color: var(--ink); overflow-wrap: anywhere; }
/* Queued messages: a quiet, compact list above the composer while OpenCode works. Nothing here starts work by itself
    until the current turn has finished, so a message steers the next turn rather than interrupting this one. */
 .queue { display: flex; flex-direction: column; gap: 7px; margin: 2px 0 4px; padding: 9px 10px 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); }
 .queue-head { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
 .queue-title { display: inline-flex; align-items: center; gap: 6px; flex: none; font: 600 11px var(--sans); letter-spacing: .04em; color: var(--ink-soft); }
 .queue-title b { display: inline-grid; place-items: center; min-width: 17px; height: 17px; padding: 0 4px; border-radius: 99px; background: var(--card); border: 1px solid var(--line); font: 600 10px var(--mono); color: var(--muted); box-sizing: border-box; }
 .queue-icon { display: grid; place-items: center; color: var(--accent-text); }
 .queue-icon svg { width: 13px; height: 13px; }
 .queue-said { min-width: 0; margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; color: var(--muted); text-align: right; }
 .queue-list { display: flex; flex-direction: column; gap: 3px; list-style: none; margin: 0; padding: 0; }
 .queue-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 7px; min-height: 30px; padding: 4px 5px 4px 9px; border: 1px solid transparent; border-radius: 7px; background: var(--card); }
 .queue-list li.first { border-color: color-mix(in srgb, var(--accent) 26%, var(--line)); }
 .queue-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12.5px; color: var(--ink); }
 .queue-attach { display: inline-flex; align-items: center; gap: 3px; flex: none; font: 11px var(--mono); color: var(--muted); }
 .queue-attach svg { width: 12px; height: 12px; }
 .queue-actions { display: inline-flex; align-items: center; gap: 2px; flex: none; }
 .queue-actions button { min-height: 26px; padding: 0 7px; border: 0; border-radius: 5px; background: none; color: var(--muted); font: 500 11px var(--sans); cursor: pointer; }
 .queue-actions button:hover { background: var(--tint); color: var(--ink); }
 .queue-actions .retry { color: var(--accent-text); font-weight: 600; }
 .queue-actions .remove:hover { color: var(--bad); }
 .queue-actions button:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  /* Composer: a raised card holding one clearly bounded input well, then a toolbar. The well is the only place that looks typeable. */
 .composer { position: relative; display: flex; flex-direction: column; gap: 8px; margin: 0 14px 12px; padding: 10px; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card-raised, var(--card)); box-shadow: var(--shadow-dock, var(--shadow)); transition: border-color .15s, box-shadow .15s; }
 .composer.dropping { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 28%, transparent); }
 .field { display: flex; align-items: flex-end; gap: 8px; padding: 8px 8px 8px 12px; border: 1.5px solid var(--line-strong); border-radius: 12px; background: var(--bg-sunk); transition: border-color .15s, box-shadow .15s, background .15s; }
 .field:hover { border-color: color-mix(in srgb, var(--ink) 30%, var(--line-strong)); }
 .field:focus-within { border-color: var(--accent); background: var(--bg); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 22%, transparent); }
 .field.busy { border-style: dashed; }
 .field-prompt { align-self: flex-start; padding-top: 7px; font: 600 18px/1.3 var(--mono); color: var(--accent-text); user-select: none; }
 .composer-input { flex: 1; min-width: 0; min-height: 44px; max-height: 12rem; padding: 7px 0 4px; border: 0; background: none; color: var(--ink); font-size: 14.5px; line-height: 1.5; resize: none; field-sizing: content; }
 .composer-input::placeholder { color: var(--muted); }
 .composer-input:focus-visible { outline: none; }
 .field-actions { display: flex; align-items: center; gap: 6px; flex: none; }
 .field-actions .btn { min-height: 36px; padding: 0 14px; font-size: 13px; display: inline-flex; align-items: center; gap: 6px; }
 .composer-bar { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 0 2px; }
 .composer-tools { display: flex; align-items: center; gap: 6px; flex: 1; min-width: 0; }
 .hint { flex: none; font-size: 11px; color: var(--faint); white-space: nowrap; }
 .icon-btn { display: grid; place-items: center; flex: none; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--muted); cursor: pointer; }
 .icon-btn:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
 .skills-btn { flex: none; height: 32px; padding: 0 10px; border: 0; border-radius: 8px; background: none; color: var(--muted); font: 600 12px var(--sans); cursor: pointer; white-space: nowrap; }
 .skills-btn:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
 .skills-btn:disabled { opacity: .5; cursor: default; }
 .icon-btn:disabled { opacity: .5; cursor: default; }
 .icon-btn svg { width: 17px; height: 17px; }
 .send svg, .stop svg { width: 14px; height: 14px; }
 .stop { color: var(--bad); }
 .attachments { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 10px; }
 .attachments li { position: relative; display: flex; flex-direction: column; gap: 4px; width: 84px; }
 .tile { display: block; width: 84px; height: 84px; padding: 0; border: 1px solid var(--line-strong); border-radius: 10px; overflow: hidden; background: var(--bg-sunk); cursor: zoom-in; transition: transform .15s var(--ease-out), border-color .15s; }
 .tile:hover { border-color: var(--accent); transform: translateY(-1px); }
 .tile img { display: block; width: 100%; height: 100%; object-fit: cover; }
 .tile-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10.5px; color: var(--muted); }
 .tile-remove { position: absolute; top: -6px; right: -6px; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--card-raised, var(--card)); color: var(--ink); font-size: 14px; line-height: 1; cursor: pointer; box-shadow: var(--shadow); }
 .tile-remove:hover:not(:disabled) { background: var(--bad); border-color: var(--bad); color: var(--accent-ink); }
 .tile-remove:disabled { opacity: .5; cursor: default; }
 /* Images in a sent message: thumbnails that open larger. */
 .bubble.user.has-images { display: flex; flex-direction: column; gap: 8px; padding: 6px; }
 .bubble.user.has-images .said { padding: 2px 6px 4px; }
 .thumbs { display: flex; flex-wrap: wrap; gap: 6px; justify-content: flex-end; }
 .thumb { position: relative; display: block; width: 112px; height: 112px; padding: 0; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; background: var(--bg-sunk); cursor: zoom-in; transition: transform .15s var(--ease-out), box-shadow .15s; }
 .thumbs.single .thumb { width: 240px; max-width: 100%; height: auto; max-height: 240px; }
 .thumb img { display: block; width: 100%; height: 100%; object-fit: cover; }
 .thumbs.single .thumb img { height: auto; max-height: 240px; object-fit: contain; }
 .thumb:hover { transform: scale(1.015); box-shadow: 0 4px 14px color-mix(in srgb, var(--ink) 18%, transparent); }
 .thumb .zoom { position: absolute; right: 6px; bottom: 6px; display: grid; place-items: center; width: 24px; height: 24px; border-radius: 6px; background: color-mix(in srgb, var(--ink) 72%, transparent); color: var(--bg); opacity: 0; transition: opacity .15s; }
 .thumb:hover .zoom, .thumb:focus-visible .zoom { opacity: 1; }
 .thumb .zoom svg { width: 13px; height: 13px; }
 .thumb.ghost { display: inline-flex; align-items: center; gap: 7px; width: auto; height: auto; max-width: 100%; padding: 7px 10px; cursor: default; color: var(--muted); font: 12px var(--mono); }
 .thumb.ghost:hover { transform: none; box-shadow: none; }
 .thumb.ghost svg { width: 16px; height: 16px; flex: none; }
 .thumb.ghost span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
 @media (prefers-reduced-motion: reduce) { .tile, .thumb { transition: none; } .tile:hover, .thumb:hover { transform: none; } }
 .composer-note { margin: 0 14px 12px; padding: 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); color: var(--muted); font-size: 12px; }
 .vh { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
 /* Narrow panes keep the target and state; line counts and durations give their room first. */
 @container chat-pane (max-width: 760px) {
  .meta .lines, .meta .time { display: none; }
 }
 @container chat-pane (max-width: 540px) {
  .tool-head { grid-template-columns: 8px 16px auto minmax(0, 1fr) auto 14px; gap: 7px; padding: 6px 8px; }
  .meta { display: none; }
  .hint { display: none; }
  .composer { margin: 0 8px 8px; padding: 8px; }
  .model-controls label { display: none; }
  .star-word { display: none; }
  .think-preview { font-size: 11.5px; }
 }
 @media (max-width: 650px) {
  .log { padding: 12px 12px 16px; }
  .row.user .bubble { max-width: 92%; }
  .bubble.agent { font-size: 13.5px; }
  .tool-head { min-height: 42px; }
  .field-actions .btn { min-height: 40px; }
  .icon-btn { width: 36px; height: 36px; }
  .skills-btn { height: 36px; }
  .composer-bar { flex-wrap: wrap; }
 }
 @media (pointer: coarse) {
  .tool-head { min-height: 44px; }
  .skills-btn, .icon-btn { min-height: 44px; min-width: 44px; }
  .queue-list li { min-height: 44px; }
  .queue-actions button { min-height: 36px; padding: 0 9px; }
  .think-head { min-height: 40px; }
  .approval-actions .btn { min-height: 46px; flex: 1 1 0; }
  .qoption { min-height: 44px; align-items: center; }
  .copy { min-height: 32px; padding: 0 10px; font-size: 12px; }
  .tile-remove { width: 30px; height: 30px; top: -8px; right: -8px; }
  .star { height: 36px; }
  .model-controls input { height: 36px; }
 }
</style>
