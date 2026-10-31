<script lang="ts">
 import { onDestroy, onMount, tick } from 'svelte'
 import { applyChatEvent, diffCounts, displayPath, formatDuration, initialChat, inlineSegments, isWorking, pendingPermissions, proseBlocks, resetChat, textSegments, thoughtPreview, toolLabel, toolState, toolTarget, type ChatEvent, type ChatState } from '../chat'
 import { copyText } from '../clipboard'
 import { chatAnswer, chatCancel, chatModel, machineModels, chatPrompt, chatURL, launchUpload, terminalClosed, type TerminalSession } from './api'
 import { chatPromptParts, formatImageSize, imageFiles, MAX_EMBEDDED_IMAGES, MAX_IMAGES, MAX_IMAGE_TOTAL_BYTES, validateImage } from './upload'

 let { session, uploadsSupported = false, uploadsOff = false, acpImages = false }: { session: TerminalSession; uploadsSupported?: boolean; uploadsOff?: boolean; acpImages?: boolean } = $props()

 let chat: ChatState = $state(initialChat())
 let selectedModel = $state(session.model ?? ''), currentModel = $state(session.model ?? ''), changingModel = $state(false)
 let models = $state<string[]>([]), modelNote = $state('')
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
  changingModel = true; feedback = ''; modelNote = ''
  try { await chatModel(session.id, selectedModel); currentModel = selectedModel; modelNote = `Model changed to ${selectedModel}. Send a message to continue.` }
  catch (e) { feedback = e instanceof Error ? e.message : 'The model could not be changed. Try again.' }
  finally { changingModel = false }
 }
 const MAX_CHAT_BYTES = 2000
 let draft = $state(''), feedback = $state(''), sending = $state(false), uploading = $state(false), answering = $state('')
 let images = $state<{ id: number; file: File; preview: string }[]>([]), imageSeq = 0, attachNote = $state('')
 let fileInput: HTMLInputElement | undefined = $state()
 let log: HTMLElement | undefined = $state()
 let source: EventSource | undefined
 let alive = true, nearBottom = true
 // Events are batched before they reach the reducer: a fast stream, or a continued chat replaying its whole history,
 // arrives as many events in a burst, and applying and drawing each one separately is what used to stall the tab.
 let pendingEvents: ChatEvent[] = []
 let flushTimer: ReturnType<typeof setTimeout> | undefined
 // Tool rows are folded by default; a failure opens so its error is not hidden. The person's toggle wins for that key.
 let expanded = $state<Record<string, boolean>>({})
 let copied = $state('')
 let copyTimer: ReturnType<typeof setTimeout> | undefined

 const ended = $derived(terminalClosed(session.state) || chat.status.state === 'ended')
 const working = $derived(isWorking(chat))
 const pending = $derived(pendingPermissions(chat))
 const statusWord = $derived(chat.connection === 'reconnecting' ? 'Reconnecting to reet…' : ended ? 'This reet session has ended.' : pending.length ? 'Waiting for your approval.' : working ? 'OpenCode is working…' : chat.status.state === 'error' ? 'OpenCode stopped with an error.' : chat.status.state === 'cancelled' ? 'Stopped.' : 'Ready.')
 const statusDetail = $derived(pending.length ? '' : chat.status.detail)
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
 async function send() {
  const text = draft.trim()
  if ((!text && !images.length) || sending || uploading || changingModel || working || ended || chat.connection === 'reconnecting') return
  if (images.length && !canAttach) { feedback = uploadsOff ? 'Image uploads are off on that machine.' : 'That machine does not accept images.'; return }
  if (new TextEncoder().encode(text).length > MAX_CHAT_BYTES) { feedback = 'That message is too long. Keep it under 2,000 bytes.'; return }
  uploading = images.length > 0; sending = true; feedback = ''
  const sendingImages = images.slice()
  try {
   const uploads: { path: string; reference: string }[] = []
   if (sendingImages.length) {
    attachNote = ''
    for (let i = 0; i < sendingImages.length; i++) {
     if (sendingImages.length > 1) attachNote = `Uploading image ${i + 1} of ${sendingImages.length}…`
     uploads.push(await launchUpload(session.agentId, sendingImages[i].file, AbortSignal.timeout(30000)))
    }
   }
   const body = chatPromptParts(text, uploads, acpImages)
   if (new TextEncoder().encode(body.text).length > MAX_CHAT_BYTES) { feedback = 'That message and its image paths are too long. Keep the message under 2,000 bytes.'; return }
   await chatPrompt(session.id, body.text, body.images)
   // Only the images in this message go: one attached while the first send was
   // still settling must stay in the composer.
   const sent = new Set(sendingImages.map(image => image.id))
   for (const image of sendingImages) URL.revokeObjectURL(image.preview)
   draft = ''; images = images.filter(image => !sent.has(image.id)); attachNote = ''; nearBottom = true; await scrollToEnd()
  }
  catch (e) { feedback = e instanceof Error ? e.message : 'That message was not sent.' }
  finally { uploading = false; sending = false }
 }
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
 function keydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault(); void send()
 }
 function isOpen(key: string, failed = false) { return expanded[key] ?? failed }
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
  const stream = new EventSource(chatURL(session.id))
  source = stream
  stream.addEventListener('reset', readReset)
  stream.addEventListener('chat', readEvent)
  stream.onopen = () => { chat = { ...chat, connection: 'open' } }
  stream.onerror = () => { if (alive) chat = { ...chat, connection: 'reconnecting' } }
  return () => { alive = false; life.abort(); pendingEvents = []; stopFlush(); if (copyTimer !== undefined) clearTimeout(copyTimer); closeStream() }
 })
 onDestroy(() => { for (const image of images) URL.revokeObjectURL(image.preview) })
</script>

{#snippet inline(text: string)}
 {#each inlineSegments(text) as part, n (n)}{#if part.kind === 'code'}<code>{part.text}</code>{:else if part.kind === 'strong'}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
{/snippet}

<section class="chat-pane" aria-label={`reet TUI in ${session.title}`} ondragover={(event) => { if (canAttach) event.preventDefault() }} ondrop={dropped}>
 <div class="chat-status" role="status" aria-live="polite">
  <span class="status-dot" class:busy={working} class:wait={pending.length > 0} class:down={chat.connection === 'reconnecting'} aria-hidden="true"></span>
  <strong>{statusWord}</strong>
  {#if statusDetail && !failed}<span class="detail">{statusDetail}</span>{/if}
 </div>
 {#if failed}
  <div class="error-panel" role="alert">
   <strong>OpenCode could not continue</strong>
   <p>{guidance}</p>
   {#if statusDetail}<details open><summary>Error details</summary><p>{statusDetail}</p></details>{/if}
  </div>
 {/if}
 {#if chat.truncated}<p class="notice bad">Earlier messages in this reet session are no longer available.</p>{/if}
 {#if feedback}<p class="notice bad" role="alert">{feedback}</p>{/if}
 <div class="log" role="log" bind:this={log} onscroll={onScroll}>
  {#if chat.items.length === 0}
   <div class="empty"><h3>Ask OpenCode to change this project.</h3><p>It asks before it edits a file or runs a command. Allow once for one card, or allow that command or file for the rest of this session.</p></div>
  {/if}
  {#each chat.items as item (item.key)}
   {#if item.kind === 'user'}
    <div class="row user"><div class="bubble user">{item.text}</div></div>
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
     {@const open = isOpen(item.key, item.status === 'failed')}
     <div class="tool" data-kind={item.toolKind || 'other'} class:open class:failed={item.status === 'failed'}>
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
       {#if hasBody}<span class="chevron" class:open aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg></span>{/if}
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
    <div class="approval" class:decided={item.decision} role="group" aria-label="Approval needed">
     <div class="approval-head">
      <span class="ticon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 16H3z" /><path d="M12 9v4M12 16h.01" /></svg></span>
      <span class="kind">{toolLabel(item.toolKind)}</span>
      <strong>{item.title}</strong>
      <span class="state">{item.decision ? 'Answered' : 'Needs you'}</span>
     </div>
     {#if item.path}<p class="approval-path" title={item.path}>{displayPath(item.path, session.dir)}</p>{/if}
     {#if item.command}<div class="block"><span class="block-label">Command</span><pre class="command"><span class="prompt" aria-hidden="true">$</span>{' '}{item.command}</pre></div>{/if}
     {#if item.diff}<div class="block"><span class="block-label">Diff · {approvalCounts.additions} added, {approvalCounts.deletions} removed</span><pre class="diff">{#each item.diff.split('\n') as line, i (i)}<span class={diffClass(line)}>{line || ' '}</span>{/each}</pre></div>{/if}
     {#if item.truncated}<p class="notice bad">This change is too large to review in full here. Deny it and use OpenCode in a terminal if you need to see all of it.</p>{/if}
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
    </div>
   {/if}
  {/each}
 </div>
 {#if ended}
  <p class="composer-note">This reet session has ended. Its live history is not kept after the machine connection closes.</p>
 {:else}
  <div class="composer" onpaste={pasted}>
   <div class="model-controls" data-keeps-focus>
    <label for={`${session.id}-model`}>Model</label>
    <input id={`${session.id}-model`} list={`${session.id}-models`} bind:value={selectedModel} disabled={working || sending || changingModel || chat.connection !== 'open'} placeholder="provider/model" />
    <datalist id={`${session.id}-models`}>{#each models as model}<option value={model}></option>{/each}</datalist>
    <button type="button" class="btn" disabled={working || sending || changingModel || !selectedModel.trim() || selectedModel === currentModel || chat.connection !== 'open'} onclick={() => void changeModel()}>{changingModel ? 'Changing…' : 'Change model'}</button>
   </div>
   {#if modelNote}<p class="notice" role="status">{modelNote}</p>{/if}
   {#if images.length}
    <ul class="attachments" aria-label="Images to attach">
     {#each images as image (image.id)}
      <li><img src={image.preview} alt="" /><span>{image.file.name} · {formatImageSize(image.file.size)}</span><button type="button" aria-label={`Remove ${image.file.name}`} disabled={uploading} onclick={() => removeImage(image.id)}>×</button></li>
     {/each}
    </ul>
   {/if}
   {#if attachNote}<p class="notice" class:bad={/off|does not|at most|more than|Use PNG|Each image|empty/.test(attachNote)}>{attachNote}</p>{/if}
   <label class="vh" for={`${session.id}-chat-prompt`}>Message OpenCode</label>
   <textarea id={`${session.id}-chat-prompt`} class="composer-input" bind:value={draft} rows="2" maxlength="2000" placeholder={working ? 'OpenCode is working…' : 'Ask OpenCode to change this project…'} onkeydown={keydown} disabled={sending || uploading || changingModel || working || chat.connection === 'reconnecting'}></textarea>
   <div class="composer-bar">
    <div class="composer-tools">
     {#if canAttach}<button type="button" class="icon-btn" aria-label="Attach image" title="Attach image" disabled={sending || uploading || changingModel || working || images.length >= imageLimit} onclick={() => fileInput?.click()}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21.4 11.6l-8.5 8.5a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8" /></svg></button>{/if}
     <span class="hint">Enter to send · Shift + Enter for a new line</span>
    </div>
    <div class="composer-actions">
     {#if working}<button type="button" class="btn stop" onclick={() => void stop()}>Stop</button>{/if}
     <button type="button" class="btn primary send" disabled={sending || uploading || changingModel || working || (!draft.trim() && !images.length) || chat.connection === 'reconnecting'} onclick={() => void send()}>{uploading ? 'Uploading…' : sending ? 'Sending…' : 'Send'}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg></button>
    </div>
   </div>
   {#if canAttach}<input class="vh" bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple onchange={picked} />{/if}
  </div>
 {/if}
</section>

<style>
 .error-panel { flex: none; margin: 12px 16px; padding: 16px; border: 2px solid var(--bad); border-radius: 10px; background: color-mix(in srgb, var(--bad) 12%, var(--bg)); color: var(--ink); overflow-wrap: anywhere; max-height: 35vh; overflow: auto; }
 .error-panel > strong { color: var(--bad); font-size: 1rem; }
 .error-panel p { margin: 8px 0; line-height: 1.5; }
 .error-panel summary { cursor: pointer; }
 .model-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
 .model-controls input { flex: 1; min-width: 120px; padding: 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; }
 .model-controls button { min-height: 44px; }
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
 .approval-head strong { font: 500 13px var(--sans); color: var(--ink); }
 .approval-head .state { margin-left: auto; color: var(--warn); }
 .approval.decided .approval-head .state { color: var(--muted); }
 .approval-path { margin: 0; font: 11.5px var(--mono); color: var(--ink-soft); overflow-wrap: anywhere; }
 .approval-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
 .approval-actions .btn { min-height: 36px; padding: 0 13px; font-size: 13px; }
 .decision { margin: 0; font-size: 12px; color: var(--muted); }
 /* Composer: one soft card with the field, attachments and the send controls. */
 .composer { display: flex; flex-direction: column; gap: 6px; margin: 0 14px 12px; padding: 7px 7px 7px 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); box-shadow: var(--shadow); }
 .composer:focus-within { border-color: var(--line-strong); }
 .composer-input { min-height: 42px; max-height: 10rem; padding: 7px 0 4px; border: 0; background: none; color: var(--ink); font-size: 14px; line-height: 1.5; resize: none; }
 .composer-input:focus-visible { outline: none; }
 .composer-bar { display: flex; align-items: center; gap: 8px; min-width: 0; }
 .composer-tools { display: flex; align-items: center; gap: 4px; flex: 1; min-width: 0; }
 .hint { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; color: var(--faint); }
 .icon-btn { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 7px; background: none; color: var(--muted); cursor: pointer; }
 .icon-btn:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
 .icon-btn:disabled { opacity: .5; cursor: default; }
 .icon-btn svg { width: 16px; height: 16px; }
 .composer-actions { display: flex; align-items: center; gap: 6px; flex: none; }
 .composer-actions .btn { min-height: 34px; padding: 0 12px; font-size: 12.5px; }
 .send svg { width: 14px; height: 14px; }
 .stop { color: var(--bad); }
 .attachments { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; }
 .attachments li { display: inline-flex; align-items: center; gap: 6px; max-width: 100%; padding: 4px 5px 4px 9px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg-sunk); font-size: 12px; color: var(--ink-soft); }
 .attachments li img { width: 28px; height: 28px; flex: none; border-radius: 5px; object-fit: cover; }
 .attachments li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
 .attachments button { min-width: 26px; min-height: 26px; border: 0; background: none; color: var(--muted); font-size: 15px; cursor: pointer; }
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
  .composer { margin: 0 10px 10px; }
  .think-preview { font-size: 11.5px; }
 }
 @media (max-width: 650px) {
  .log { padding: 12px 12px 16px; }
  .row.user .bubble { max-width: 92%; }
  .bubble.agent { font-size: 13.5px; }
  .tool-head { min-height: 42px; }
  .composer-actions .btn { min-height: 40px; }
  .icon-btn { width: 36px; height: 36px; }
 }
 @media (pointer: coarse) {
  .tool-head { min-height: 44px; }
  .think-head { min-height: 40px; }
  .approval-actions .btn { min-height: 46px; flex: 1 1 0; }
  .copy { min-height: 32px; padding: 0 10px; font-size: 12px; }
  .attachments button { min-width: 40px; min-height: 40px; }
 }
</style>
