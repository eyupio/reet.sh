<script lang="ts">
  import { untrack } from 'svelte'
  import { formatWhen } from './time'
  import { getContextSettings,previewContext,saveContext,type ContextFlags,type ContextGrant,type ContextState,type ContextSnapshot } from './api'
  let { path,gaffer=false,onchanged,timeZone='',dateFormat='locale' }: {path:string;gaffer?:boolean;onchanged?:()=>void|Promise<void>;timeZone?:string;dateFormat?:string}=$props()
  let view=$state<ContextState|null>(null),flags=$state<ContextFlags>({status:false,history:false,text:false})
  let sessions=$state<{id:string;title:string}[]>([]),scopes=$state<ContextGrant[]>([]),selected=$state('')
  let loading=$state(false),busy=$state(false),error=$state(''),notice=$state(''),preview=$state<ContextSnapshot|null>(null)
  let request:AbortController|undefined,epoch=0
  const any=$derived(flags.status||flags.history||flags.text)
  const url=$derived(gaffer?path:`${path}?sessionId=${encodeURIComponent(selected)}`)
  $effect(()=>{const next=path;untrack(()=>{selected='';void load(next)});return()=>{epoch++;request?.abort()}})
  function fail(e:unknown){return e instanceof Error?e.message:'Could not read context sharing.'}
  async function load(target=url){
    request?.abort();const controller=new AbortController();request=controller;const mine=++epoch
    loading=true;error='';notice='';preview=null;view=null
    try{const v=await getContextSettings(target,controller.signal);if(mine!==epoch)return
      view=v.context;sessions=v.sessions??sessions;scopes=v.scopes??scopes
      const g=v.context.grant,a=v.context.allowed;flags={status:g.status&&a.status,history:g.history&&a.history,text:g.text&&a.text}
    }catch(e){if(mine===epoch&&!controller.signal.aborted)error=fail(e)}finally{if(mine===epoch)loading=false}
  }
  function change(key:keyof ContextFlags,value:boolean){flags={...flags,[key]:value};if(!flags.history)flags.text=false;preview=null;notice=''}
  async function inspect(){
    const mine=epoch;busy=true;error=''
    try{const v=await previewContext(url,flags,request?.signal);if(mine===epoch)preview=v.context}catch(e){if(mine===epoch)error=fail(e)}finally{if(mine===epoch)busy=false}
  }
  async function save(stop=false){
    if(!view||(!stop&&any&&!preview))return
    const mine=epoch;busy=true;error=''
    try{
      const result=await saveContext(url,view,stop?{status:false,history:false,text:false}:flags,request?.signal)
      if(mine!==epoch)return
      view=result.context;notice=stop?'Sharing stopped.':'Sharing saved. Reet, no guessing what goes where.'
      if(stop){flags={status:false,history:false,text:false};preview=null}
      if(!gaffer){const v=await getContextSettings(url,request?.signal);if(mine===epoch){scopes=v.scopes??[];view=v.context}}
      if(mine===epoch)await onchanged?.()
    }catch(e){if(mine===epoch)error=fail(e)}finally{if(mine===epoch)busy=false}
  }
  const when=(at:number)=>at?formatWhen(at,timeZone,dateFormat):'Not reported'
</script>

<section class="sharing" aria-label="Context sharing">
  <p class="lead">{gaffer?'Choose what Gaffer may read for this conversation.':'Choose exactly what this connection may read. Shelf permissions stay separate.'}</p>
  <p class="hint">Reads use the controller’s retained snapshot. Live machine health, current PR checks, files, reasoning, approval prompts and raw tool input/output are excluded.</p>
  {#if !gaffer}
    <label class="scope-label">Scope<select aria-label="Context scope" value={selected} disabled={loading||busy} onchange={e=>{selected=e.currentTarget.value;void load()}}>
      <option value="">Account activity counts</option>
      {#each sessions as session}<option value={session.id}>{session.title||session.id}</option>{/each}
      {#each scopes.filter(g=>g.sessionId&&!sessions.some(s=>s.id===g.sessionId)) as grant}<option value={grant.sessionId}>Session {grant.sessionId}</option>{/each}
    </select></label>
    {#if scopes.length}<details class="saved"><summary>{scopes.length} shared {scopes.length===1?'scope':'scopes'}</summary><ul>{#each scopes as grant}<li><button class="scope-link" type="button" disabled={busy||loading} onclick={()=>{selected=grant.sessionId;void load()}}>{grant.sessionId?(sessions.find(s=>s.id===grant.sessionId)?.title||grant.sessionId):'Account activity counts'}</button></li>{/each}</ul></details>{/if}
  {/if}
  {#if loading}<p role="status">Reading sharing permissions…</p>{:else if view}
    {#if gaffer}<p>Destination: <strong>{view.providerName||'Provider unavailable'}</strong> · {view.model||'Model unavailable'}</p>{/if}
    {#if gaffer&&!view.allowed.status&&!view.allowed.history}<p class="hint">Enable the matching data permissions in Account → Gaffer AI before sharing.</p>{/if}
    {#if view.reason}<p class="review" role="status">{view.reason}</p>{/if}
    <fieldset disabled={busy}>
      <legend>Shared data</legend>
      <label class="choice"><input type="checkbox" aria-label="Share status" checked={flags.status} disabled={!view.allowed.status} onchange={e=>change('status',e.currentTarget.checked)} /><span><strong>Last-reported status</strong><small>{view.grant.sessionId?'This session’s state, worker type and update time.':'Counts of your machines and unarchived sessions, with the last report time.'}</small></span></label>
      {#if view.grant.sessionId}
        <label class="choice"><input type="checkbox" aria-label="Share event summaries" checked={flags.history} disabled={!view.allowed.history} onchange={e=>change('history',e.currentTarget.checked)} /><span><strong>Conversation event summaries</strong><small>Recent roles, times and tool states. Message text is excluded.</small></span></label>
        <label class="choice"><input type="checkbox" aria-label="Share message text and code" checked={flags.text} disabled={!view.allowed.text||!flags.history} onchange={e=>change('text',e.currentTarget.checked)} /><span><strong>Message text and code</strong><small>Bounded user/agent text, the goal excerpt and title. Text can contain code or secrets; automatic masking is limited.</small></span></label>
      {/if}
    </fieldset>
    {#if gaffer}<p class="hint">The provider’s data permissions also apply. A provider/model change needs a fresh review. Changing sharing stops the current Gaffer answer; the working agent keeps running.</p>{/if}
    <p class="hint">Sharing continues for future reads until you stop it. Previously shared data cannot be recalled. Read access does not allow worker instructions or offline supervision.</p>
    <div class="actions"><button class="btn" type="button" disabled={busy||!any} onclick={inspect}>{busy?'Working…':'Preview shared data'}</button><button class="btn primary" type="button" disabled={busy||(any&&!preview)} onclick={()=>save()}>{any?'Enable selected sharing':'Save with sharing off'}</button>{#if view.grant.status||view.grant.history||view.grant.text}<button class="btn quiet danger" type="button" disabled={busy} onclick={()=>save(true)}>Stop sharing</button>{/if}</div>
    {#if preview}
      <section class="preview" aria-label="Shared context preview"><h3>What will be shared</h3><p class="hint">Controller-retained data · read {when(preview.readAt)}. The next read can contain newer permitted events.</p>
        {#if preview.fleet}<p>{preview.fleet.machines} machines · {Object.entries(preview.fleet.sessions).map(([state,count])=>`${count} ${state}`).join(' · ')||'No reported sessions'}</p><p class="hint">Last session report: {when(preview.fleet.lastReportedAt)}</p>{/if}
        {#if preview.session}<p>{preview.session.title||'Selected session'}{preview.session.state?` · ${preview.session.state}`:''}{preview.session.worker?` · ${preview.session.worker}`:''}</p>{#if preview.session.goalExcerpt}<blockquote>{preview.session.goalExcerpt}{preview.session.goalTruncated?'… (excerpt)':''}</blockquote>{/if}{/if}
        {#if preview.history}<p class="hint">{preview.history.unavailable?'No retained conversation events are available.':`${preview.history.events.length} projected ${preview.history.events.length===1?'event':'events'} from ${preview.history.retainedCount} retained records.`} {preview.history.truncated?'History is trimmed or some fields/events are omitted.':''}</p><ol>{#each preview.history.events as event}<li><small>#{event.seq} · {event.kind} · {when(event.at)}{event.state?` · ${event.state}`:''}{event.toolKind?` · ${event.toolKind} ${event.status??''}`:''}</small>{#if event.text}<p class="text">{event.text}</p>{/if}{#if event.truncated}<small>Text excerpt</small>{/if}</li>{/each}</ol>{/if}
        <details><summary>Full shared data</summary><pre>{JSON.stringify(preview,null,2)}</pre></details>
      </section>
    {/if}
  {/if}
  {#if error}<p class="error" role="alert">{error}</p><button class="btn" type="button" disabled={busy} onclick={()=>load()}>Reload sharing permissions</button>{/if}
  {#if notice}<p class="notice" role="status">{notice}</p>{/if}
</section>

<style>
  .sharing { display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5 }
  p { margin:0 }.lead { color:var(--ink) }.hint,small { color:var(--muted);font-size:14px }
  fieldset { display:grid;gap:12px;border:1px solid var(--line);border-radius:12px;padding:14px;min-width:0 }legend { padding:0 5px;font-weight:600 }
  .choice { display:flex;align-items:flex-start;gap:10px;cursor:pointer }.choice input { flex:none;width:20px;height:20px;margin:3px 0;accent-color:var(--accent) }.choice span { display:grid;gap:3px;min-width:0 }.choice:has(:disabled) { cursor:default }
  .scope-label { display:grid;gap:6px;font-weight:600 }select { min-height:44px;border:1px solid var(--line-strong);border-radius:8px;padding:8px;background:var(--bg);color:var(--ink);font:inherit;min-width:0;max-width:100% }
  .actions { display:flex;flex-wrap:wrap;gap:8px }.actions .btn { min-height:44px }.danger,.error { color:var(--bad) }.review { padding:10px 12px;border-left:3px solid var(--accent);background:var(--tint) }.notice { color:var(--accent-text) }
  .preview { display:grid;gap:10px;padding:14px;border:1px solid var(--line);border-radius:12px;background:var(--bg-sunk);min-width:0 }h3 { font:600 20px var(--serif);margin:0 }blockquote { margin:0;padding:8px 12px;border-left:2px solid var(--accent);white-space:pre-wrap;overflow-wrap:anywhere }
  ol { display:grid;gap:14px;padding:0;list-style:none;margin:0;max-height:260px;overflow:auto }.text { white-space:pre-wrap;overflow-wrap:anywhere;margin-top:5px }pre { max-height:240px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font-size:14px }summary { cursor:pointer;min-height:32px;align-content:center }.saved ul { list-style:none;padding:0 }.scope-link { background:none;border:0;color:var(--accent-text);font:inherit;text-align:left;min-height:40px;overflow-wrap:anywhere;cursor:pointer }
</style>
