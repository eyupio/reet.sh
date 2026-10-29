<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { applyChatEvent, initialChat, isWorking, pendingPermissions, resetChat, type ChatEvent, type ChatState } from '../chat'
 import { chatAnswer, chatCancel, chatPrompt, chatURL, terminalClosed, type TerminalSession } from './api'

 let { session }: { session: TerminalSession } = $props()

 const MAX_CHAT_BYTES = 2000
 let chat: ChatState = $state(initialChat())
 let draft = $state(''), feedback = $state(''), sending = $state(false), answering = $state('')
 let log: HTMLElement | undefined = $state()
 let source: EventSource | undefined
 let alive = true, nearBottom = true

 const ended = $derived(terminalClosed(session.state) || chat.status.state === 'ended')
 const working = $derived(isWorking(chat))
 const pending = $derived(pendingPermissions(chat))
 const statusWord = $derived(chat.connection === 'reconnecting' ? 'Reconnecting to reet…' : ended ? 'This chat has ended.' : pending.length ? 'Waiting for your approval.' : working ? 'OpenCode is working…' : chat.status.state === 'error' ? 'OpenCode stopped with an error.' : chat.status.state === 'cancelled' ? 'Stopped.' : 'Ready.')
 const statusDetail = $derived(pending.length ? '' : chat.status.detail)

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
 async function send() {
  const text = draft.trim()
  if (!text || sending || working || ended || chat.connection === 'reconnecting') return
  if (new TextEncoder().encode(text).length > MAX_CHAT_BYTES) { feedback = 'That message is too long. Keep it under 2,000 bytes.'; return }
  sending = true; feedback = ''
  try { await chatPrompt(session.id, text); draft = ''; nearBottom = true; await scrollToEnd() }
  catch (e) { feedback = e instanceof Error ? e.message : 'That message was not sent.' }
  finally { sending = false }
 }
 async function stop() {
  feedback = ''
  try { await chatCancel(session.id) }
  catch (e) { feedback = e instanceof Error ? e.message : 'That chat could not be stopped.' }
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
</script>

<section class="chat-pane" aria-label={`Chat with OpenCode in ${session.title}`}>
 <div class="chat-status" role="status" aria-live="polite">
  <span class="dot" class:busy={working} class:wait={pending.length > 0} class:down={chat.connection === 'reconnecting'} aria-hidden="true"></span>
  <strong>{statusWord}</strong>
  {#if statusDetail}<span class="detail">{statusDetail}</span>{/if}
 </div>
 {#if chat.truncated}<p class="notice bad">Earlier messages in this chat are no longer available.</p>{/if}
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
  <p class="composer-note">This chat has ended. Its history is not kept after the machine connection closes.</p>
 {:else}
  <div class="composer">
   <label class="vh" for={`${session.id}-chat-prompt`}>Message OpenCode</label>
   <textarea id={`${session.id}-chat-prompt`} bind:value={draft} rows="2" maxlength="2000" placeholder={working ? 'OpenCode is working…' : 'Ask OpenCode to change this project…'} onkeydown={keydown} disabled={sending || working || chat.connection === 'reconnecting'}></textarea>
   <div class="composer-actions">
    {#if working}<button type="button" class="btn" onclick={() => void stop()}>Stop</button>{/if}
    <button type="button" class="btn primary" disabled={sending || working || !draft.trim() || chat.connection === 'reconnecting'} onclick={() => void send()}>{sending ? 'Sending…' : 'Send'}</button>
   </div>
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
 .composer-note { margin: 0; padding: 12px 16px; border-top: 1px solid var(--line); color: var(--ink-soft); font-size: .8rem; }
 .vh { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
 @media (max-width: 650px) { .log { padding: 12px; } .composer { padding: 8px 12px calc(8px + env(safe-area-inset-bottom)); } .bubble { max-width: 92%; } }
</style>
