<script lang="ts">
 import { untrack } from 'svelte'
 import { chatQueue, chatQueueChange, type ChatControls, type ServerQueuedMessage } from './api'

 let { sessionId, controls, active, reloadKey = 0 }: { sessionId: string; controls: ChatControls | null; active: boolean; reloadKey?: number } = $props()
 let queue = $state<ServerQueuedMessage[]>([]), problem = $state(''), busy = $state(false)
 let editing = $state(''), editText = $state(''), confirmed = $state<Record<string, boolean>>({})
 const canSteer = $derived(!!controls?.working && controls.steering && !controls.waiting && !controls.compacting && !controls.configuring)
 $effect(() => {
  const id = sessionId, key = reloadKey
  void key
  const life = new AbortController()
  let timer: ReturnType<typeof setTimeout>
  async function refresh() {
   try { const items = await chatQueue(id, life.signal); if (!life.signal.aborted) { queue = items; problem = '' } }
   catch (e) { if (!life.signal.aborted) problem = e instanceof Error ? e.message : 'The queue could not be loaded.' }
   if (!life.signal.aborted) timer = setTimeout(refresh, 1500)
  }
  untrack(() => { void refresh() })
  return () => { life.abort(); clearTimeout(timer) }
 })
 async function change(item: ServerQueuedMessage, action: string) {
  if (busy) return
  busy = true; problem = ''
  try {
   queue = await chatQueueChange(sessionId, item, action, action === 'edit' ? editText : '', confirmed[item.id] === true)
   editing = ''; confirmed = {}
  } catch (e) { problem = e instanceof Error ? e.message : 'That queued message could not be changed.' }
  finally { busy = false }
 }
</script>

{#if queue.length || problem}
 <section class="server-queue" aria-label="Queued messages">
  <header><strong>Queued <span>{queue.length}</span></strong><small>Saved here. Sends between turns.</small></header>
  {#if problem}<p role="alert">{problem}</p>{/if}
  <ol>{#each queue as item (item.id)}
   <li>
    <p class="text">{item.text || 'Image only'}{#if item.images.length} <small>· {item.images.length} image{item.images.length === 1 ? '' : 's'}</small>{/if}</p>
    {#if item.note}<p class:uncertain={item.state === 'uncertain'} role="status">{item.note}</p>{/if}
    {#if item.state === 'uncertain'}<label><input type="checkbox" bind:checked={confirmed[item.id]} /> I checked the chat. Retrying may send this twice.</label>{/if}
    {#if editing === item.id}
     <label>Edit message<textarea aria-label="Edit queued message" maxlength="2000" bind:value={editText}></textarea></label>
     <button disabled={busy} onclick={() => void change(item, 'edit')}>Save message</button>
     <button disabled={busy} onclick={() => { editing = '' }}>Cancel edit</button>
    {:else if item.state !== 'sending'}
     <div class="actions">
      {#if item.state === 'queued'}
       {#if canSteer && active}<button disabled={busy} onclick={() => void change(item, 'send')}>Send now</button>{/if}
       {#if controls?.working && active}<button disabled={busy || controls.configuring || item.intent === 'stopping'} onclick={() => void change(item, 'stop')}>Stop and send</button>{/if}
      {:else}
       <button disabled={busy || !active || item.state === 'uncertain' && !confirmed[item.id]} onclick={() => void change(item, 'retry')}>Retry</button>
      {/if}
      {#if item.state !== 'uncertain'}<button disabled={busy} onclick={() => { editing = item.id; editText = item.text }}>Edit</button>{/if}
      <button disabled={busy} onclick={() => void change(item, 'remove')}>Remove</button>
     </div>
    {/if}
   </li>
  {/each}</ol>
 </section>
{/if}

<style>
 .server-queue { flex: none; max-height: 15rem; overflow: auto; padding: 10px; margin-bottom: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); font-size: 12px; }
 header, .actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
 header { justify-content: space-between; }
 small { color: var(--muted); }
 ol { margin: 6px 0 0; padding: 0; list-style: none; }
 li { padding: 8px; margin-top: 4px; border-radius: 6px; background: var(--card); }
 p { margin: 0 0 6px; color: var(--muted); }
 .text { color: var(--ink); white-space: pre-wrap; overflow-wrap: anywhere; }
 .uncertain { color: var(--bad); }
 button { min-height: 30px; padding: 3px 8px; border: 1px solid var(--line); border-radius: 5px; background: var(--card); color: var(--ink); cursor: pointer; font: inherit; }
 button:disabled { opacity: .5; cursor: default; }
 label { display: block; margin-bottom: 8px; }
 textarea { display: block; width: 100%; min-height: 4rem; margin-top: 4px; background: var(--bg-sunk); color: var(--ink); border: 1px solid var(--line); border-radius: 5px; font: inherit; }
</style>
