<script lang="ts">
  import { onMount } from 'svelte'
  import { listGafferConversations, removeGafferConversation, type GafferConversation } from './api'
  import { shell } from './dialogShell'
  import { formatWhen } from './time'
  import ConfirmDialog from './ConfirmDialog.svelte'
  let { onclose, onremoved, onerror, timeZone, dateFormat }: { onclose:()=>void; onremoved:(c:GafferConversation)=>void; onerror:(e:unknown)=>string; timeZone:string; dateFormat:string } = $props()
  let dialog: HTMLDialogElement, rows=$state<GafferConversation[]>([]), loading=$state(true), error=$state(''), removing=$state<GafferConversation|null>(null), busy=$state(false)
  const life=new AbortController()
  onMount(()=>{dialog.showModal();void load();return()=>life.abort()})
  async function load(){loading=true;error='';try{const data=await listGafferConversations(life.signal);if(!life.signal.aborted)rows=data.conversations}catch(e){if(!life.signal.aborted)error=onerror(e)}finally{if(!life.signal.aborted)loading=false}}
  async function remove(){const c=removing;if(!c)return;removing=null;busy=true;error='';try{await removeGafferConversation(c,life.signal);if(!life.signal.aborted){rows=rows.filter(r=>r.id!==c.id);onremoved(c)}}catch(e){if(!life.signal.aborted)error=onerror(e)}finally{if(!life.signal.aborted)busy=false}}
</script>
<dialog bind:this={dialog} use:shell={{sheet:'full'}} aria-labelledby="gaffer-history-title" oncancel={event=>{event.preventDefault();onclose()}}>
  <header data-shell-head><div><h2 id="gaffer-history-title">Saved Gaffer chats</h2><p>Your own companion histories, including sessions no longer in the workspace.</p></div><button class="btn" data-shell-close onclick={onclose}>Close saved chats</button></header>
  <div class="body" data-shell-body>
    {#if error}<p class="error" role="alert">{error}</p><button class="btn" onclick={()=>void load()}>Reload saved chats</button>{/if}
    {#if loading}<p role="status">Reading saved chats…</p>{:else if !rows.length}<p>No saved Gaffer chats.</p>{:else}<ul>{#each rows as c (c.id)}<li><div><strong>{c.sessionId ? `Session companion · ${c.sessionId.slice(-8)}` : 'General Gaffer chat'}</strong><p>{formatWhen(c.updatedAt,timeZone,dateFormat)}</p></div><button class="btn remove" disabled={busy} aria-label={`Remove ${c.sessionId ? 'session '+c.sessionId.slice(-8) : 'general'} chat`} onclick={()=>removing=c}>Remove</button></li>{/each}</ul>{/if}
  </div>
</dialog>
{#if removing}<ConfirmDialog title="Remove this saved Gaffer chat?" body="This deletes its messages and stops any current Gaffer answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Remove saved chat" danger onconfirm={()=>void remove()} oncancel={()=>removing=null}/>{/if}
<style>
 dialog{width:min(580px,calc(100vw - 24px));max-height:calc(100dvh - 24px);padding:0;border:1px solid var(--line-strong);border-radius:14px;background:var(--card);color:var(--ink)}
 dialog::backdrop{background:var(--scrim)}
 header{display:flex;align-items:flex-start;gap:16px;padding:22px;border-bottom:1px solid var(--line)}
 h2{margin:0;font:600 26px var(--serif)}
 header p{margin:8px 0 0;color:var(--muted);font-size:14px;line-height:1.5}
 header button{flex:none}
 .body{padding:18px 22px;overflow:auto;max-height:65dvh;font-size:14px}
 ul{list-style:none;padding:0;margin:0}
 li{display:flex;align-items:center;justify-content:space-between;gap:12px;border-bottom:1px solid var(--line);padding:12px 0}
 li p{color:var(--muted);margin:4px 0 0;font-size:14px}
 .remove,.error{color:var(--bad)}
 @media(max-width:559.98px){header{padding:14px;flex-wrap:wrap}h2{font-size:24px}.body{padding:14px;max-height:calc(var(--shell-height,100dvh) - 150px)}.btn{min-height:44px}}
</style>
