<script lang="ts">
 import { onDestroy, onMount, tick } from 'svelte'
 import { applyChatEvent, initialChat, isWorking, pendingPermissions, resetChat, type ChatEvent, type ChatState } from '../chat'
 import { chatAnswer, chatCancel, chatPrompt, chatURL, launchUpload, terminalClosed, type TerminalSession } from './api'
 import { chatPromptParts, formatImageSize, imageFiles, MAX_EMBEDDED_IMAGES, MAX_IMAGES, MAX_IMAGE_TOTAL_BYTES, validateImage } from './upload'

 let { session, uploadsSupported = false, uploadsOff = false, acpImages = false }: { session: TerminalSession; uploadsSupported?: boolean; uploadsOff?: boolean; acpImages?: boolean } = $props()

 const MAX_CHAT_BYTES = 2000
 let chat: ChatState = $state(initialChat())
 let draft = $state(''), feedback = $state(''), sending = $state(false), uploading = $state(false), answering = $state('')
 let images = $state<{ id: number; file: File; preview: string }[]>([]), imageSeq = 0, attachNote = $state('')
 let fileInput: HTMLInputElement | undefined = $state()
 let log: HTMLElement | undefined = $state()
 let source: EventSource | undefined
 let alive = true, nearBottom = true

 const ended = $derived(terminalClosed(session.state) || chat.status.state === 'ended')
 const working = $derived(isWorking(chat))
 const pending = $derived(pendingPermissions(chat))
 const statusWord = $derived(chat.connection === 'reconnecting' ? 'Reconnecting to reet…' : ended ? 'This reet session has ended.' : pending.length ? 'Waiting for your approval.' : working ? 'OpenCode is working…' : chat.status.state === 'error' ? 'OpenCode stopped with an error.' : chat.status.state === 'cancelled' ? 'Stopped.' : 'Ready.')
 const statusDetail = $derived(pending.length ? '' : chat.status.detail)
 const canAttach = $derived(uploadsSupported && !uploadsOff && !ended)
 const imageLimit = $derived(acpImages ? MAX_EMBEDDED_IMAGES : MAX_IMAGES)

 function closeStream() { source?.close(); source = undefined }
 function readReset(event: MessageEvent) {
  let truncated = false
  try { truncated = !!JSON.parse(event.data)?.truncated } catch { /* fixed empty reset */ }
  chat = { ...resetChat(truncated), connection: 'open' }
  void scrollToEnd()
 }
 function readEvent(event: MessageEvent) {
  let parsed: ChatEvent
  try { parsed = JSON.parse(event.data) as ChatEvent } catch { return }
  chat = applyChatEvent(chat, parsed)
  void scrollToEnd()
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
  if ((!text && !images.length) || sending || uploading || working || ended || chat.connection === 'reconnecting') return
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
 async function answer(permissionId: string, allow: boolean) {
  if (answering) return
  answering = permissionId; feedback = ''
  try { await chatAnswer(session.id, permissionId, allow) }
  catch (e) { feedback = e instanceof Error ? e.message : 'That answer was not sent.' }
  finally { answering = '' }
 }
 function keydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault(); void send()
 }
 onMount(() => {
  const stream = new EventSource(chatURL(session.id))
  source = stream
  stream.addEventListener('reset', readReset)
  stream.addEventListener('chat', readEvent)
  stream.onopen = () => { chat = { ...chat, connection: 'open' } }
  stream.onerror = () => { if (alive) chat = { ...chat, connection: 'reconnecting' } }
  return () => { alive = false; closeStream() }
 })
 onDestroy(() => { for (const image of images) URL.revokeObjectURL(image.preview) })
</script>

<section class="chat-pane" aria-label={`reet TUI in ${session.title}`} ondragover={(event) => { if (canAttach) event.preventDefault() }} ondrop={dropped}>
 <div class="chat-status" role="status" aria-live="polite">
  <span class="dot" class:busy={working} class:wait={pending.length > 0} class:down={chat.connection === 'reconnecting'} aria-hidden="true"></span>
  <strong>{statusWord}</strong>
  {#if statusDetail}<span class="detail">{statusDetail}</span>{/if}
 </div>
 {#if chat.truncated}<p class="notice bad">Earlier messages in this reet session are no longer available.</p>{/if}
 {#if feedback}<p class="notice bad" role="alert">{feedback}</p>{/if}
 <div class="log" role="log" bind:this={log} onscroll={onScroll}>
  {#if chat.items.length === 0}
   <div class="empty"><h3>Ask OpenCode to change this project.</h3><p>It asks before it edits a file or runs a command. Approvals are one use only.</p></div>
  {/if}
  {#each chat.items as item (item.key)}
   {#if item.kind === 'user'}
    <div class="bubble user">{item.text}</div>
   {:else if item.kind === 'agent'}
    <div class="bubble agent">{item.text}</div>
   {:else if item.kind === 'notice'}
    <p class="notice" class:bad={item.bad}>{item.text}</p>
   {:else if item.kind === 'tool'}
    <div class="tool">
     <div class="tool-head"><span class="kind">{item.toolKind || 'tool'}</span><strong>{item.title || 'Tool call'}</strong><span class="state">{item.status === 'in_progress' ? 'working' : item.status || 'pending'}</span></div>
     {#if item.text}<pre>{item.text}</pre>{/if}
     {#if item.locations.length}<p class="where">{item.locations.join(' · ')}</p>{/if}
    </div>
   {:else if item.kind === 'permission'}
    <div class="approval" class:decided={item.decision}>
     <div class="approval-head"><span class="kind">{item.toolKind || 'tool'}</span><strong>{item.title}</strong></div>
     {#if item.path}<p class="where">{item.path}</p>{/if}
     {#if item.command}<pre class="command">{item.command}</pre>{/if}
     {#if item.diff}<pre class="diff">{item.diff}</pre>{/if}
     {#if item.truncated}<p class="notice bad">This change is too large to review in full here. Deny it and use OpenCode in a terminal if you need to see all of it.</p>{/if}
     {#if item.decision}
      <p class="decision">{item.decision === 'allowed' ? 'Allowed once.' : item.decision === 'denied' ? 'Denied.' : item.reason === 'timeout' ? 'Closed: the approval timed out.' : 'Closed without an answer.'}</p>
     {:else}
      <div class="approval-actions">
       <button type="button" class="btn primary" disabled={!!answering || item.truncated} onclick={() => void answer(item.permissionId, true)}>Allow once</button>
       <button type="button" class="btn" disabled={!!answering} onclick={() => void answer(item.permissionId, false)}>Deny</button>
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
   {#if images.length}
    <ul class="attachments" aria-label="Images to attach">
     {#each images as image (image.id)}
      <li><img src={image.preview} alt="" /><span>{image.file.name} · {formatImageSize(image.file.size)}</span><button type="button" aria-label={`Remove ${image.file.name}`} disabled={uploading} onclick={() => removeImage(image.id)}>×</button></li>
     {/each}
    </ul>
   {/if}
   {#if attachNote}<p class="notice" class:bad={/off|does not|at most|more than|Use PNG|Each image|empty/.test(attachNote)}>{attachNote}</p>{/if}
   <label class="vh" for={`${session.id}-chat-prompt`}>Message OpenCode</label>
   <textarea id={`${session.id}-chat-prompt`} bind:value={draft} rows="2" maxlength="2000" placeholder={working ? 'OpenCode is working…' : 'Ask OpenCode to change this project…'} onkeydown={keydown} disabled={sending || uploading || working || chat.connection === 'reconnecting'}></textarea>
   <div class="composer-actions">
    {#if canAttach}<button type="button" class="btn" disabled={sending || uploading || working || images.length >= imageLimit} onclick={() => fileInput?.click()}>Attach image</button>{/if}
    {#if working}<button type="button" class="btn" onclick={() => void stop()}>Stop</button>{/if}
    <button type="button" class="btn primary" disabled={sending || uploading || working || (!draft.trim() && !images.length) || chat.connection === 'reconnecting'} onclick={() => void send()}>{uploading ? 'Uploading…' : sending ? 'Sending…' : 'Send'}</button>
   </div>
   {#if canAttach}<input class="vh" bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple onchange={picked} />{/if}
  </div>
 {/if}
</section>

<style>
 .chat-pane { display: flex; flex-direction: column; min-height: 0; height: 100%; background: var(--bg); }
 .chat-status { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 8px 16px; border-bottom: 1px solid var(--line); font-size: .8rem; color: var(--ink-soft); }
 .chat-status strong { font-weight: 600; color: var(--ink); }
 .chat-status .detail { color: var(--ink-soft); overflow-wrap: anywhere; }
 .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--line); flex: none; }
 .dot.busy { background: var(--accent); animation: pulse 1.6s ease-in-out infinite; }
 .dot.wait { background: var(--warn); }
 .dot.down { background: var(--bad); }
 @keyframes pulse { 50% { opacity: .35; } }
 @media (prefers-reduced-motion: reduce) { .dot.busy { animation: none; } }
 .log { flex: 1; min-height: 0; overflow: auto; padding: 16px; display: flex; flex-direction: column; gap: 10px; scroll-behavior: smooth; }
 @media (prefers-reduced-motion: reduce) { .log { scroll-behavior: auto; } }
 .empty { margin: auto; max-width: 38ch; text-align: center; color: var(--ink-soft); }
 .empty h3 { margin: 0 0 6px; font-size: 1rem; color: var(--ink); }
 .empty p { margin: 0; font-size: .85rem; line-height: 1.45; }
 .bubble { max-width: min(72ch, 88%); padding: 9px 12px; border-radius: 12px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: .9rem; line-height: 1.5; }
 .bubble.user { align-self: flex-end; background: var(--accent); color: var(--accent-ink); border-bottom-right-radius: 4px; }
 .bubble.agent { align-self: flex-start; background: var(--card); border: 1px solid var(--line); border-bottom-left-radius: 4px; }
 .notice { margin: 0; padding: 7px 10px; border-left: 3px solid var(--line); color: var(--ink-soft); font-size: .8rem; }
 .notice.bad { border-color: var(--bad); color: var(--bad); }
 .tool, .approval { border: 1px solid var(--line); border-radius: 10px; background: var(--card); padding: 9px 12px; font-size: .82rem; }
 .tool-head, .approval-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
 .kind { font-size: .68rem; text-transform: uppercase; letter-spacing: .06em; color: var(--ink-soft); }
 .state { margin-left: auto; color: var(--ink-soft); font-size: .72rem; }
 .where { margin: 4px 0 0; color: var(--ink-soft); font-size: .76rem; overflow-wrap: anywhere; }
 pre { margin: 7px 0 0; padding: 8px; max-height: 16rem; overflow: auto; background: var(--bg); border: 1px solid var(--line); border-radius: 7px; font-family: var(--mono, ui-monospace, monospace); font-size: .76rem; line-height: 1.45; white-space: pre-wrap; overflow-wrap: anywhere; }
 pre.diff { max-height: 20rem; }
 .approval { border-left: 3px solid var(--warn); }
 .approval.decided { border-left-color: var(--line); }
 .approval-actions, .composer-actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 8px; flex-wrap: wrap; }
 .approval-actions .btn, .composer-actions .btn { min-height: 44px; }
 .decision { margin: 8px 0 0; color: var(--ink-soft); font-size: .8rem; }
 .composer { border-top: 1px solid var(--line); padding: 10px 16px calc(10px + env(safe-area-inset-bottom)); display: flex; flex-direction: column; gap: 8px; background: var(--bg); }
 .composer textarea { width: 100%; resize: vertical; min-height: 48px; max-height: 10rem; padding: 10px 12px; border: 1px solid var(--line); border-radius: 9px; font: inherit; font-size: 1rem; line-height: 1.4; color: var(--ink); background: var(--card); }
 .composer textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
 .attachments { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; }
 .attachments li { display: inline-flex; align-items: center; gap: 6px; max-width: 100%; padding: 4px 6px 4px 4px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); font-size: .76rem; color: var(--ink-soft); }
 .attachments li img { width: 28px; height: 28px; flex: none; border-radius: 5px; object-fit: cover; }
 .attachments li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
 .attachments button { min-width: 30px; min-height: 30px; border: 0; background: none; color: var(--ink-soft); font-size: 1rem; cursor: pointer; }
 .composer-note { margin: 0; padding: 12px 16px; border-top: 1px solid var(--line); color: var(--ink-soft); font-size: .8rem; }
 .vh { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
 @media (max-width: 650px) { .log { padding: 12px; } .composer { padding: 8px 12px calc(8px + env(safe-area-inset-bottom)); } .bubble { max-width: 92%; } }
 @media (pointer: coarse) { .attachments button { min-width: 44px; min-height: 44px; } }
</style>
