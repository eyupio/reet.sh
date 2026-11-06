<script lang="ts">
  import { onMount } from 'svelte'
  import { listGafferConversations, removeGafferConversation, type GafferConversation } from './api'
  import { shell } from './dialogShell'
  import { terminalSessions, type TerminalSession } from './terminal/api'
  import type { GafferSessionContext } from './gafferContext'
  import { formatWhen } from './time'
  import ConfirmDialog from './ConfirmDialog.svelte'
  let { onclose, onremoved, onerror, timeZone, dateFormat, activeSession, currentScope, onselect }: {activeSession:GafferSessionContext|null;currentScope:string;onselect:(mode:'general'|'session')=>void; onclose:()=>void; onremoved:(c:GafferConversation)=>void; onerror:(e:unknown)=>string; timeZone:string; dateFormat:string } = $props()
  let dialog: HTMLDialogElement, rows=$state<GafferConversation[]>([]), loading=$state(true), error=$state(''), removing=$state<GafferConversation|null>(null), busy=$state(false)
  let sessions=$state<TerminalSession[]>([]), sessionsRead=$state(false)
  const life=new AbortController()
  onMount(()=>{dialog.showModal();void load();return()=>life.abort()})
  async function load(){loading=true;error='';try{const [data,available]=await Promise.all([listGafferConversations(life.signal),terminalSessions(life.signal).catch(()=>null)]);if(!life.signal.aborted){rows=data.conversations;sessions=available??[];sessionsRead=available!==null}}catch(e){if(!life.signal.aborted)error=onerror(e)}finally{if(!life.signal.aborted)loading=false}}
  function openSession(event:MouseEvent, session:TerminalSession) {
    if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return
    // The mounted workspace can switch tabs without losing in-memory drafts.
    // Else the ordinary link opens that workspace and reopens the companion.
    const request=new CustomEvent('reet:show-session',{detail:session.id,cancelable:true})
    if(!window.dispatchEvent(request)){event.preventDefault();onselect('session')}
    else onclose()
  }
  async function remove(){const c=removing;if(!c)return;removing=null;busy=true;error='';try{await removeGafferConversation(c,life.signal);if(!life.signal.aborted){rows=rows.filter(r=>r.id!==c.id);onremoved(c)}}catch(e){if(!life.signal.aborted)error=onerror(e)}finally{if(!life.signal.aborted)busy=false}}
</script>
<dialog bind:this={dialog} use:shell={{sheet:'full'}} aria-labelledby="gaffer-history-title" oncancel={event=>{event.preventDefault();onclose()}}>
  <header data-shell-head><div><h2 id="gaffer-history-title">Conversation history</h2><p>Pick up a natter, or switch between general chat and this session.</p></div><button class="btn" data-shell-close onclick={onclose}>Close history</button></header>
  <div class="body" data-shell-body>
    <nav aria-label="Choose Gaffer conversation" class="scope-choices">
      <button class="scope-choice" class:current={!currentScope} aria-current={!currentScope?'true':undefined} onclick={()=>onselect('general')}><strong>General Gaffer chat</strong><span>A separate conversation, outside your working sessions</span></button>
      {#if activeSession}<button class="scope-choice" class:current={currentScope===activeSession.id} aria-current={currentScope===activeSession.id?'true':undefined} onclick={()=>onselect('session')}><strong>This session</strong><span>{activeSession.title}</span></button>{/if}
    </nav>
    <h3>Saved chats</h3>
    {#if error}<p class="error" role="alert">{error}</p><button class="btn" onclick={()=>void load()}>Reload saved chats</button>{/if}
    {#if loading}<p role="status">Reading saved chats…</p>{:else if !rows.length}<p>No saved Gaffer chats.</p>{:else}<ul>{#each rows as c (c.id)}{@const session=sessions.find(s=>s.id===c.sessionId&&s.structured)}<li><div class="saved-chat">
      {#if !c.sessionId || c.sessionId===activeSession?.id}<button class="open-chat" aria-label={!c.sessionId?'Open general Gaffer chat':'Open this session’s Gaffer chat'} onclick={()=>onselect(c.sessionId?'session':'general')}><strong>{c.sessionId?activeSession?.title:'General Gaffer chat'}</strong></button>
      {:else if session}<a class="open-chat" href={`/sessions?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}&gaffer=1`} onclick={event=>openSession(event,session)}><strong>{session.title||'Session companion'}</strong></a>
      {:else}<strong>Session companion · {c.sessionId.slice(-8)}</strong><p>{sessionsRead?'Session unavailable. Its saved chat can be removed here.':'Session availability was not read. Open its workspace to read this chat.'}</p>{/if}
      <p>{formatWhen(c.updatedAt,timeZone,dateFormat)}</p></div><button class="btn remove" disabled={busy} aria-label={`Remove ${c.sessionId ? 'session '+c.sessionId.slice(-8) : 'general'} chat`} onclick={()=>removing=c}>Remove</button></li>{/each}</ul>{/if}
  </div>
</dialog>
{#if removing}<ConfirmDialog title="Remove this saved Gaffer chat?" body="This deletes its messages and stops any current Gaffer answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Remove saved chat" danger onconfirm={()=>void remove()} oncancel={()=>removing=null}/>{/if}
<style>
 dialog{width:min(580px,calc(100vw - 24px));max-height:calc(100dvh - 24px);padding:0;border:1px solid var(--line-strong);border-radius:12px;background:var(--card);color:var(--ink)}
 dialog::backdrop{background:var(--scrim)}
 header{display:flex;align-items:flex-start;gap:16px;padding:20px;border-bottom:1px solid var(--line)}
 h2{margin:0;font:600 26px var(--serif)}
 header p{margin:8px 0 0;color:var(--muted);font-size:14px;line-height:1.5}
 header button{flex:none}
 .body{padding:16px 20px;overflow:auto;max-height:65dvh;font-size:14px}
 ul{list-style:none;padding:0;margin:0}
 .scope-choices{display:grid;gap:8px;margin-bottom:24px}.scope-choice{display:grid;gap:5px;text-align:left;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--bg);color:var(--ink);font:inherit;min-height:44px;cursor:pointer}.scope-choice span{color:var(--muted);font-size:14px;overflow-wrap:anywhere}.scope-choice.current{border-color:var(--accent);background:var(--tint)}h3{font:600 14px var(--sans);color:var(--muted);margin:0 0 8px}.saved-chat{min-width:0}.saved-chat strong{overflow-wrap:anywhere}.open-chat{display:block;text-align:left;background:none;border:0;padding:0;color:var(--ink);font:inherit;cursor:pointer;min-height:36px;text-decoration:underline;text-underline-offset:3px}button:focus-visible,a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
 li{display:flex;align-items:center;justify-content:space-between;gap:12px;border-bottom:1px solid var(--line);padding:12px 0}
 li p{color:var(--muted);margin:4px 0 0;font-size:14px;font-variant-numeric:tabular-nums}
 .remove,.error{color:var(--bad)}
 @media(max-width:559.98px){header{padding:16px;flex-wrap:wrap}h2{font-size:24px}.body{padding:16px;max-height:calc(var(--shell-height,100dvh) - 150px)}.btn{min-height:44px}}
</style>
