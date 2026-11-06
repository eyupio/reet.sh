<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { ApiError, UnauthorizedError, cancelGafferAnswer, configureGafferConversation, createGafferConversation, gafferTurnEventsURL, getGafferConversation, getGafferModels, getGafferSettings, removeGafferConversation, sendGafferMessage,getWorkerSettings,getGafferGoal,
    type GafferConversationView, type GafferSettings, type GafferTurn } from './api'
  import { gafferSessionContext, gafferChatEntry, gafferChatDock } from './gafferContext'
  import { gafferMessageBytes, gafferMessageId, gafferTurnLabel, newerGafferTurn, parseGafferTurn, readGafferReceipt, writeGafferReceipt, type PendingGafferSend } from './gafferChat'
  import GafferAvatar from './terminal/GafferAvatar.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import GafferChatHistory from './GafferChatHistory.svelte'
  import GafferProse from './GafferProse.svelte'
  import WorkerMessaging from './WorkerMessaging.svelte'
 import GafferGoal from './GafferGoal.svelte'
  import GafferFollowedSessions from './GafferFollowedSessions.svelte'
  import ContextSharing from './ContextSharing.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { shell } from './dialogShell'
  import { canAutoFocus } from './focus'
  import { keepLauncherClear, alignToConversation } from './floatingLauncher'
  import { formatWhen } from './time'
  import { toolName } from './agentLaunch'

  let { userId, onerror, timeZone = '', dateFormat = 'locale' }: { userId: number; onerror?: (e: unknown) => void; timeZone?: string; dateFormat?: string } = $props()
  const contextDescriptionId = $props.id()
  let historyOpen = $state(false), optionsOpen = $state(false), workerOpen = $state(false), goalOpen=$state(false), relay = $state(false)
  let followedOpen = $state(false)
  let modelButton = $state<HTMLButtonElement>(), historyButton = $state<HTMLButtonElement>(), contextButton = $state<HTMLButtonElement>(), optionsButton = $state<HTMLButtonElement>()
  let modelBefore: {providerId:string;model:string;manual:boolean} | undefined
  let contextOpen = $state(false)
  let open = $state(false), phone = $state(false), expanded = $state(false), mode = $state('session')
  let panel = $state<HTMLDialogElement>(), launcher = $state<HTMLButtonElement>(), composer = $state<HTMLTextAreaElement>(), transcript = $state<HTMLDivElement>()
  let inlineOpener: HTMLElement | undefined
  let settings = $state<GafferSettings | null>(null), view = $state<GafferConversationView | null>(null), loadedScope = $state<string | null>(null)
  let loading = $state(false), sending = $state(false), updating = $state(false), cancelling = $state(false), confirming = $state(false)
  let error = $state(''), notice = $state(''), streamError = $state(''), choicesOpen = $state(false)
  let providerId = $state(''), model = $state(''), models = $state<string[]>([]), fetchingModels = $state(false), modelsError = $state(''), manualModel = $state(false)
  let drafts = $state<Record<string,string>>({}), pending = $state<PendingGafferSend | null>(null)
  const pendingMemory = new Map<string,PendingGafferSend>()
  let request: AbortController | undefined, modelRequest: AbortController | undefined, stream: EventSource | undefined, epoch = 0, modelEpoch = 0, stickToBottom = true
  const scope = $derived(mode === 'session' && $gafferSessionContext ? $gafferSessionContext.id : '')
  const scopeTitle = $derived(scope ? $gafferSessionContext?.title || 'This session' : 'General Gaffer chat')
  const unavailable = $derived(!!scope && !$gafferSessionContext?.structured)
  const current = $derived(loadedScope === scope ? view : null)
  const running = $derived(current?.turns.find(t => t.state === 'running'))
  const provider = $derived(settings?.providers.find(p => p.id === providerId))
  const dirty = $derived(!!current && (current.conversation.providerId !== providerId || current.conversation.model !== model))
  const bytes = $derived(gafferMessageBytes(drafts[scope] ?? ''))
  const blocked = $derived(loadedScope !== scope || loading || sending || updating || !!running || !!pending || unavailable || !!streamError || !settings?.available || !provider?.enabled || dirty || !!current?.context?.needsReview || (manualModel && !model.trim()))
  const last = $derived(current?.turns.at(-1))
  const contextLabel = $derived(current?.context?.needsReview ? 'Review context sharing' : current?.context?.grant && (current.context.grant.status || current.context.grant.history || current.context.grant.text) ? 'Selected context shared' : 'Context off · typed messages only')

  onMount(() => {
    const entry = (opener: HTMLElement) => { inlineOpener = opener; open = true }
    gafferChatEntry.set(entry)
    const arrival=new URL(location.href)
    if(arrival.pathname==='/sessions'&&arrival.searchParams.get('gaffer')==='1'){
      open=true;arrival.searchParams.delete('gaffer');history.replaceState(history.state,'',arrival)
    }
    const mq = matchMedia('(max-width: 559.98px)'); const resize = () => phone = mq.matches
    resize(); mq.addEventListener('change', resize)
    return () => { gafferChatEntry.update(current => current === entry ? null : current); mq.removeEventListener('change', resize); dispose(); drafts = {}; pendingMemory.clear() }
  })
  $effect(() => { const key = scope, shown = open; untrack(() => { contextOpen=false; choicesOpen=false; optionsOpen=false; workerOpen=false;goalOpen=false;followedOpen=false;relay=false; if (shown) void load(key); else dispose() }) })
  $effect(() => {
    if (!open || !panel) return
    const mobile = phone, dialog = panel
    if (dialog.open) dialog.close()
    if (mobile) dialog.showModal(); else dialog.show()
    if (canAutoFocus()) void tick().then(() => { if (dialog.open && !mobile) composer?.focus({preventScroll:true}) })
  })
  function phoneShell(node: HTMLDialogElement, mobile: boolean) {
    let handle: ReturnType<typeof shell>
    function update(value: boolean) { handle?.destroy?.(); node.removeAttribute('data-shell'); if (value) handle = shell(node, {sheet:'full'}); else handle = undefined }
    update(mobile)
    return { update, destroy() { handle?.destroy?.() } }
  }
  function dispose() { epoch++; request?.abort(); modelRequest?.abort(); modelEpoch++; stream?.close(); stream = undefined }
  function message(e: unknown): string { if (e instanceof UnauthorizedError) onerror?.(e); return e instanceof Error ? e.message : 'Gaffer could not reach the controller just now.' }
  function remember(key: string, value: PendingGafferSend | null) {
    if (value) pendingMemory.set(key, value); else pendingMemory.delete(key)
    writeGafferReceipt(userId, key, value)
    if (scope === key) pending = value
  }
  async function load(key = scope) {
    dispose(); const mine = epoch, controller = new AbortController(); request = controller
    loading = true; sending = false; updating = false; cancelling = false; confirming = false; error = ''; notice = ''; streamError = ''; loadedScope = null; view = null
    models = []; modelsError = ''; fetchingModels = false; manualModel = false
    pending = pendingMemory.get(key) ?? readGafferReceipt(userId, key)
    if (unavailable) { loading = false; loadedScope = key; return }
    try {
      const [configuration, conversation] = await Promise.all([
        getGafferSettings(controller.signal),
        getGafferConversation(key, controller.signal).catch(e => { if (e instanceof ApiError && e.status === 404) return null; throw e }),
      ])
      if (mine !== epoch || controller.signal.aborted) return
      settings = configuration; view = conversation; loadedScope = key
      providerId = conversation?.conversation.providerId ?? configuration.providers.find(p => p.isDefault)?.id ?? ''
      model = conversation?.conversation.model ?? ''; choicesOpen = false
      if (pending) {
        const seen = conversation?.turns.find(t => t.id === pending?.id && t.conversationId === pending?.conversationId)
        if (seen) { if (!pending.text || drafts[key] === pending.text) drafts[key] = ''; remember(key, null) }
        else if (!conversation || conversation.conversation.id !== pending.conversationId) remember(key, null)
      }
      stickToBottom = true; scrollEnd()
      const active = conversation?.turns.find(t => t.state === 'running'); if (active) observe(active, mine)
    } catch (e) { if (mine === epoch && !controller.signal.aborted) error = message(e) }
    finally { if (mine === epoch) { loading = false; if (canAutoFocus() && panel?.contains(document.activeElement)) void tick().then(() => { if (mine === epoch && open && panel?.contains(document.activeElement)) composer?.focus({preventScroll:true}) }) } }
  }
  function scrollEnd() { if (stickToBottom) void tick().then(() => transcript?.scrollTo({top: transcript.scrollHeight})) }
  function observe(turn: GafferTurn, mine = epoch) {
    stream?.close(); streamError = ''
    const source = new EventSource(gafferTurnEventsURL(turn.conversationId, turn.id), {withCredentials:true}); stream = source
    source.addEventListener('turn', event => {
      if (mine !== epoch || stream !== source || !open || !view || view.conversation.id !== turn.conversationId) return
      try {
        const next = parseGafferTurn((event as MessageEvent).data, turn.conversationId, turn.id)
        view = {...view, turns: view.turns.map(t => t.id === next.id ? newerGafferTurn(t, next) : t)}; scrollEnd()
        if (next.state !== 'running') {
          source.close(); stream = undefined; cancelling = false
          // A fresh revision is required before another send or model change.
          updating = true
          void getGafferConversation(scope, request?.signal).then(answer => {
            if (mine === epoch) view = answer
          }).catch(e => { if (mine === epoch) streamError = message(e) }).finally(() => { if (mine === epoch) updating = false })
        }
      } catch (e) { source.close(); if (mine === epoch) { stream = undefined; streamError = message(e) } }
    })
    source.onerror = () => {
      source.close()
      if (mine === epoch && stream === source && open) { stream = undefined; streamError = 'The reply connection paused. Reconnect to read Gaffer’s saved answer.' }
    }
  }
  async function close() { open = false; confirming = false; historyOpen = false; choicesOpen = false; optionsOpen = false; workerOpen = false; goalOpen = false; followedOpen = false; relay = false; dispose(); await tick(); (inlineOpener?.isConnected ? inlineOpener : $gafferChatDock ?? launcher)?.focus({preventScroll:true}) }
  function chooseProvider(value: string) { providerId = value; model = ''; manualModel = false; invalidateModels() }
  function invalidateModels() { modelEpoch++; modelRequest?.abort(); fetchingModels = false; models = []; modelsError = '' }
  async function fetchModels() {
    if (!provider) return
    const p = provider; modelRequest?.abort(); const controller = new AbortController(); modelRequest = controller
    const mine = ++modelEpoch; fetchingModels = true; modelsError = ''
    try {
      const answer = await getGafferModels({id:p.id,revision:p.revision,name:p.name,kind:p.kind,baseUrl:p.baseUrl,model:p.model,reasoningEffort:p.reasoningEffort,enabled:p.enabled,localOnly:p.localOnly,allowStatus:p.allowStatus,allowSessions:p.allowSessions,allowCode:p.allowCode}, AbortSignal.any([controller.signal, AbortSignal.timeout(25_000)]))
      if (mine === modelEpoch && !controller.signal.aborted) { models = answer.models; if (!models.length) modelsError = 'No models were listed. You can enter an exact name.' }
    } catch (e) { if (mine === modelEpoch && !controller.signal.aborted) modelsError = message(e) }
    finally { if (mine === modelEpoch) fetchingModels = false }
  }
  function openChoices() {
    modelBefore = {providerId, model, manual:manualModel}; choicesOpen = true
  }
  function closeChoices() {
    if (modelBefore) { providerId=modelBefore.providerId; model=modelBefore.model; manualModel=modelBefore.manual }
    choicesOpen=false; invalidateModels(); void tick().then(()=>modelButton?.focus({preventScroll:true}))
  }
  async function applyChoice(event: SubmitEvent) {
    event.preventDefault()
    if (!provider?.enabled || (manualModel && !model.trim())) return
    const mine = epoch; updating = true; error = ''; notice = ''
    try {
      if (current) {
        const answer = await configureGafferConversation(current.conversation, providerId, model, request?.signal)
        if (mine !== epoch) return
        view=answer; providerId=answer.conversation.providerId; model=answer.conversation.model
      }
      if (mine === epoch) { choicesOpen=false;relay=false; invalidateModels(); notice='Choice of brains saved.'; void tick().then(()=>modelButton?.focus({preventScroll:true})) }
    } catch (e) { if (mine === epoch) error = message(e) }
    finally { if (mine === epoch) updating = false }
  }
  async function contextChoices(){
    if(loading||sending||updating||dirty||unavailable)return
    if(manualModel&&!model.trim()){openChoices();error='Enter an exact model name first.';return}
    if(!current&&!provider?.enabled){openChoices();return}
    const mine=epoch;updating=true;error=''
    try{
      if(!current){const v=await createGafferConversation(scope,providerId,model,request?.signal);if(mine!==epoch)return;view=v}
      if(mine===epoch)contextOpen=true
    }catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
  }
  async function workerChoices(){
    optionsOpen=false
    if(loading||sending||updating||dirty||unavailable||!scope)return
    const mine=epoch;updating=true;error=''
    try{if(!current){const c=await createGafferConversation(scope,providerId,model,request?.signal);if(mine!==epoch)return;view=c}if(mine===epoch)workerOpen=true}catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
  }
  async function workerPermissionSaved(){
    relay=false;const mine=epoch,key=scope;updating=true
    try{const fresh=await getGafferConversation(key,request?.signal);if(mine===epoch)view=fresh}catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
  }
  $effect(()=>{
    if(!open||!current?.messages?.some(m=>m.state==='queued'||m.state==='sending'))return
    const id=current.conversation.id,key=scope,mine=epoch
    const timer=setInterval(()=>{if(loading||sending||updating)return;void getWorkerSettings(`/api/v2/gaffer/conversations/${encodeURIComponent(id)}/worker`,request?.signal).then(next=>{if(mine===epoch&&view?.conversation.id===id)view={...view,messages:next.messages}}).catch(e=>{if(mine===epoch)streamError=message(e)})},3000)
    return()=>clearInterval(timer)
  })
  async function goalChoices(){
 optionsOpen=false
 if(loading||sending||updating||dirty||unavailable||!scope)return
 const mine=epoch;updating=true;error=''
 try{if(!current){const c=await createGafferConversation(scope,providerId,model,request?.signal);if(mine!==epoch)return;view=c}if(mine===epoch)goalOpen=true}catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
 }
 async function goalChanged(){
 const mine=epoch,key=scope;updating=true
 try{const fresh=await getGafferConversation(key,request?.signal);if(mine!==epoch)return;view=fresh;const live=fresh.turns.find(t=>t.state==='running');if(live&&!stream)observe(live,mine)}catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
 }
 $effect(()=>{
 if(!open||current?.goal?.state!=='active')return
 const id=current.conversation.id,mine=epoch
 let reading=false
 const timer=setInterval(()=>{if(reading||loading||sending||updating)return;reading=true;void getGafferGoal(id,request?.signal).then(async next=>{if(mine!==epoch||view?.conversation.id!==id)return;const previous=view.goal;view={...view,goal:next.goal??undefined};if(previous?.activeTurnId!==next.goal?.activeTurnId||previous?.usedReviews!==next.goal?.usedReviews||previous?.summary!==next.goal?.summary)await goalChanged()}).catch(e=>{if(mine===epoch)error=message(e)}).finally(()=>reading=false)},4000)
 return()=>clearInterval(timer)
 })
  async function contextSaved(){contextOpen=false;await load(scope);notice='Context sharing updated. The working agent keeps running.'}
  async function send(existing?: PendingGafferSend) {
    if (!existing && (blocked || !drafts[scope]?.trim() || bytes > 8000)) return
    const mine = epoch, key = scope, text = existing?.text ?? drafts[key] ?? ''
    sending = true; error = ''; notice = ''
    let admitted = false, accepted = false
    try {
      let answer = current
      if (!answer) answer = await createGafferConversation(key, providerId, model, request?.signal)
      if (mine !== epoch) return
      view = answer
      const receipt: PendingGafferSend = existing ?? {conversationId:answer.conversation.id,id:gafferMessageId(),revision:answer.conversation.revision,text,...(relay?{relay:true}:{})}
      remember(key, receipt); admitted = true;relay=false
      const turn = await sendGafferMessage({...answer.conversation,revision:receipt.revision}, receipt.id, text, request?.signal,receipt.relay??false)
      if (mine !== epoch) return
      if (turn.id !== receipt.id || turn.conversationId !== receipt.conversationId) throw new Error('The message receipt did not match. Check delivery before sending again.')
      accepted = true
      remember(key, null); if (drafts[key] === text) drafts[key] = ''; stickToBottom = true
      view = {...answer, turns:[...answer.turns.filter(t => t.id !== turn.id),turn]}; scrollEnd()
      if (turn.state === 'running') observe(turn, mine)
      else { updating = true; const fresh = await getGafferConversation(key, request?.signal); if (mine === epoch) { view = fresh; updating = false } }
    } catch (e) {
      if (mine !== epoch) return
      // Gateway/server failures cannot prove the controller rejected a send.
      // Keep its original receipt until a read confirms the saved outcome.
      if (admitted && !existing && e instanceof ApiError && [400,401,403,404,409,413,422,429].includes(e.status)) remember(key, null)
      if (accepted) streamError = 'Reload the saved conversation before sending another message.'
      error = message(e)
    } finally { if (mine === epoch) { sending = false; updating = false } }
  }
  async function cancelAnswer() {
    if (!current || !running) return
    const mine = epoch; cancelling = true; error = ''
    try { await cancelGafferAnswer(current.conversation, running.id, request?.signal); if (mine === epoch) notice = 'Cancellation requested. Waiting for the saved outcome.' }
    catch (e) { if (mine === epoch) { cancelling = false; error = message(e) } }
  }
  async function removeConversation() {
    if (!current) return
    const mine = epoch, key = scope; updating = true; confirming = false; error = ''
    try { await removeGafferConversation(current.conversation, request?.signal); if (mine === epoch) { stream?.close(); remember(key, null); view = null; drafts[key] = ''; notice = 'Conversation removed. Fresh brew, fresh start.' } }
    catch (e) { if (mine === epoch) error = message(e) }
    finally { if (mine === epoch) updating = false }
  }
</script>

{#if !open && !$gafferChatDock}
  <button class="gaffer-launcher" use:keepLauncherClear bind:this={launcher} aria-label="Ask Gaffer" aria-haspopup="dialog" onclick={() => open = true}>
    <span class="launcher-portrait" aria-hidden="true"><GafferAvatar phase="idle" pose="tea" /></span><span>Ask Gaffer</span>
  </button>
{/if}
{#if open}
  <dialog class="gaffer-chat" class:expanded bind:this={panel} use:phoneShell={phone} use:alignToConversation={$gafferChatDock} aria-labelledby="gaffer-chat-title" aria-modal={phone ? 'true' : 'false'} oncancel={event => {event.preventDefault(); void close()}} onkeydown={event => { if (event.key === 'Escape') {event.preventDefault(); event.stopPropagation(); void close()} }}>
    <header class="chat-head" data-shell-head>
      <span class="portrait" aria-hidden="true"><GafferAvatar active={!!running} phase={running ? 'working' : last?.state === 'failed' ? 'error' : 'idle'} pose={running ? 'duty' : 'tea'} reaction={running ? 'scan' : ''} /></span>
      <div class="heading"><h2 id="gaffer-chat-title">Gaffer</h2><p class="scope-title" title={scopeTitle}>{scopeTitle}</p></div>
      <div class="head-actions">
        <button class="icon" bind:this={historyButton} aria-label="Conversation history" title="Conversation history" aria-haspopup="dialog" onclick={() => historyOpen = true}><svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 11a9 9 0 1 1 2.6 7M3 4v7h7M12 7v5l3 2" /></svg></button>
        <button class="icon" bind:this={optionsButton} aria-label="Chat options" title="Chat options" aria-haspopup="dialog" onclick={() => optionsOpen=true}><svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/></svg></button>
        <button class="icon expand" aria-label={expanded ? 'Compact Gaffer panel' : 'Expand Gaffer panel'} title={expanded ? 'Compact panel' : 'Expand panel'} onclick={() => expanded = !expanded}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d={expanded ? 'M4 10h6V4M20 14h-6v6M10 10 3 3m11 11 7 7' : 'M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7'} /></svg></button>
        <button class="icon" data-shell-close aria-label="Close Gaffer" title="Close the panel; keep the current answer running" onclick={() => void close()}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
      </div>
    </header>
    {#if unavailable}
      <div class="unavailable" data-shell-body><h3>This session uses a terminal.</h3><p>A session companion needs a supported structured chat. You can still talk to Gaffer in General chat.</p><button class="btn" onclick={() => mode = 'general'}>Open general chat</button></div>
    {:else}
      <div class="conversation-body" data-shell-body>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrolling history must be reachable with a keyboard.) -->
      <div class="transcript" bind:this={transcript} data-shell-body onscroll={() => {if (transcript) stickToBottom = transcript.scrollHeight - transcript.scrollTop - transcript.clientHeight < 90}} tabindex="0" role="region" aria-label="Gaffer conversation messages">
        {#if loading}<p class="empty" role="status">Finding our place…</p>
        {:else if loadedScope === scope && !error && !current?.turns.length}<div class="welcome"><span class="welcome-mark" aria-hidden="true">“</span><h3>Reet. What’s on your mind?</h3><p>Talk it through, question the plan, or ask for a second pair of eyes.</p><p class="note">Choose Context to share retained reet or session data. In a structured session, use Chat options → Worker messages to relay an instruction. Use Session follow-up to save a bounded goal while you’re away.</p></div>{/if}
        {#if loadedScope === scope}{#each current?.turns ?? [] as turn (turn.id)}
          <article class="turn">
            <div class="user-message"><p class="speaker">{turn.goalId?'Goal review':'You'}</p><p class="message-text">{turn.prompt}</p></div>
            <div class="gaffer-message"><div class="reply-head"><p class="speaker">Gaffer</p><span class="state" class:bad={turn.state === 'failed'}>{gafferTurnLabel(turn)}</span></div>
              {#if turn.answer}<GafferProse text={turn.answer} />{:else if turn.state === 'running'}<p class="thinking" role="status">Putting the thinking cap on…</p>{/if}
              {#if turn.error}<p class="turn-error" role="status">{turn.error}</p>{/if}
              {#if turn.cut}<p class="turn-error">This reply reached its output limit and may be incomplete.</p>{/if}
              {#if turn.workerGrantRevision&&turn.state!=='running'&&!updating&&!loading&&!streamError&&!current?.messages?.some(m=>m.turnId===turn.id)}<p class="note">No worker instruction was queued for this request.</p>{/if}
              {#each current?.messages?.filter(m=>m.turnId===turn.id)??[] as instruction (instruction.id)}<details class="worker-receipt"><summary>Worker instruction · {instruction.state==='sent'?'accepted':instruction.state}</summary><p class="message-text">{instruction.text}</p><p class="note">{instruction.note||'Acceptance is not completion. Read Worker messages for the current delivery state.'}</p><button class="quiet" type="button" onclick={()=>void workerChoices()}>Worker messages</button></details>{/each}
              <div class="reply-foot">{#if turn.answer}<CopyButton text={turn.answer} label="Copy Gaffer reply" />{/if}
                <details class="reply-details"><summary>Reply details</summary><p>{turn.providerName} · {turn.model}</p><p>{formatWhen(turn.startedAt,timeZone,dateFormat)}</p>
                  {#if turn.state !== 'running'}<p>{turn.usageReported ? `${turn.inputTokens} input · ${turn.outputTokens} output tokens` : 'Usage not reported'}</p>{/if}
                  {#if turn.context}<p>Retained context · read {formatWhen(turn.context.readAt,timeZone,dateFormat)}{turn.context.lastSeq?` · through event #${turn.context.lastSeq}`:''}{turn.context.truncated?' · trimmed or incomplete':''}</p>{/if}
                </details>
              </div>
            </div>
          </article>
        {/each}{/if}
      </div>
      {#if current?.goal}<button class="goal-status quiet" type="button" onclick={()=>void goalChoices()}>{current.goal.state==='active'?'Gaffer following':current.goal.state==='needs_user'?'Goal needs you':current.goal.state==='limit'?'Goal limit reached':`Goal ${current.goal.state}`} · {current.goal.usedReviews}/{current.goal.maxReviews} reviews</button>{/if}
      {#if error}<div class="feedback bad" role="alert"><p>{error}</p><button class="quiet" onclick={() => void load()}>Reload conversation</button></div>{/if}
      {#if streamError}<div class="feedback" role="alert"><p>{streamError}</p><button class="btn" onclick={() => void load()}>Reconnect reply</button></div>{/if}
      {#if notice}<p class="notice" role="status">{notice}</p>{/if}
      {#if pending && !sending}<div class="feedback uncertain" role="alert"><strong>Message delivery is unconfirmed.</strong><p>Check the saved conversation before sending another message. Gaffer will not resend it automatically.</p><button class="btn" onclick={() => void load()}>Check delivery</button>{#if pending.text}<button class="quiet" onclick={() => void send(pending!)}>Send same message safely</button>{/if}</div>{/if}
      {#if settings && !settings.providers.length}<div class="setup"><p>Nowt configured yet. Choose Gaffer’s brains first.</p><a class="btn" href="/account/gaffer">Set up Gaffer</a></div>
      {:else if settings && (!settings.available || !provider?.enabled)}<p class="feedback" role="status">{!settings.available ? 'Provider-key storage is unavailable. Check Gaffer settings.' : 'Choose an enabled Gaffer provider to continue.'} <a href="/account/gaffer">Gaffer settings →</a></p>{/if}
      </div>
      <form class="composer" data-shell-foot onsubmit={event => {event.preventDefault(); void send()}}>
        {#if relay}<p class="request-mode" role="status">One worker message enabled for this request.<button class="quiet" type="button" onclick={()=>relay=false}>Cancel</button></p>{/if}
        <label class="sr-only" for="gaffer-message">Message Gaffer</label>
        <textarea id="gaffer-message" bind:this={composer} value={drafts[scope] ?? ''} oninput={event => drafts[scope] = event.currentTarget.value} disabled={sending || loading} rows="2" placeholder="Talk it through with Gaffer…" onkeydown={event => {if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.isComposing) {event.preventDefault(); void send()}}}></textarea>
        <div class="compose-actions">
          <div class="compose-tools"><button class="model-control" bind:this={modelButton} type="button" aria-label="Model" aria-haspopup="dialog" title={`Gaffer: ${provider?.name || 'Choose a provider'} · ${model || provider?.model || 'Choose a model'}`} disabled={loading||sending||updating} onclick={openChoices}><span>{model || provider?.model || 'Choose model'}</span><svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m6 9 6 6 6-6"/></svg></button>
          <button class="context-control" bind:this={contextButton} class:attention={current?.context?.needsReview} class:shared={contextLabel==='Selected context shared'} type="button" aria-label="Context" aria-describedby={contextDescriptionId} aria-haspopup="dialog" title={contextLabel} disabled={loading||sending||updating||dirty} onclick={()=>void contextChoices()}><span class="context-dot" aria-hidden="true"></span><span>{current?.context?.needsReview?'Review context':contextLabel==='Selected context shared'?'Context on':'Context off'}</span></button><span id={contextDescriptionId} class="sr-only">{contextLabel}</span></div>
          {#if running}<button class="send-control stop" type="button" aria-label="Cancel answer" title="Cancel Gaffer’s answer" disabled={cancelling || sending} onclick={() => void cancelAnswer()}><svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><rect x="5" y="5" width="14" height="14" rx="2"/></svg></button>{:else}<button class="send-control" aria-label="Send message" title="Send message · Ctrl / ⌘ Enter" disabled={blocked || !drafts[scope]?.trim() || bytes > 8000}><svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19V5m-6 6 6-6 6 6"/></svg></button>{/if}
        </div>
        {#if bytes>7200||dirty}<p class="compose-hint" class:bad={bytes>8000}>{bytes>7200?`${bytes.toLocaleString()} / 8,000 bytes`:'Apply your model choice first'}</p>{/if}
      </form>
    {/if}

  </dialog>
{/if}
{#if historyOpen}<GafferChatHistory {timeZone} {dateFormat} activeSession={$gafferSessionContext} currentScope={scope} onselect={value=>{mode=value;historyOpen=false}} onerror={message} onclose={()=>{historyOpen=false;void tick().then(()=>historyButton?.focus({preventScroll:true}))}} onremoved={c => { if (current?.conversation.id === c.id || pending?.conversationId === c.id) { remember(scope, null); drafts[scope] = ''; void load() } }} />{/if}
{#if confirming && current}<ConfirmDialog title="Remove this Gaffer conversation?" body="This deletes its messages and stops its current answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Remove conversation" danger oncancel={() => confirming = false} onconfirm={() => void removeConversation()} />{/if}

{#if contextOpen && current}<WorkspaceDialog title="What Gaffer can read" onclose={()=>{contextOpen=false;void tick().then(()=>contextButton?.focus({preventScroll:true}))}}><ContextSharing {timeZone} {dateFormat} path={`/api/v2/gaffer/conversations/${encodeURIComponent(current.conversation.id)}/context`} gaffer onchanged={contextSaved}/></WorkspaceDialog>{/if}

{#if choicesOpen && settings}<WorkspaceDialog title="Gaffer model" onclose={closeChoices}>
        <form class="choices" onsubmit={applyChoice}>
          <fieldset disabled={!!running || sending || updating || !!pending}>
            <label>Gaffer provider<select aria-label="Gaffer provider" value={providerId} onchange={event => chooseProvider(event.currentTarget.value)}><option value="">Choose a provider</option>{#if providerId && !settings.providers.some(p => p.id === providerId)}<option value={providerId}>Unavailable provider</option>{/if}{#each settings.providers as p}<option value={p.id} disabled={!p.enabled}>{p.name}{p.enabled ? '' : ' · paused'}</option>{/each}</select></label>
            <div class="model-head"><label for="gaffer-chat-model">Gaffer model</label><button class="quiet" type="button" disabled={!provider?.enabled || fetchingModels} onclick={() => void fetchModels()}>{fetchingModels ? 'Reading…' : 'Fetch models'}</button></div>
            <select id="gaffer-chat-model" value={manualModel ? '__manual__' : model} onchange={event => {manualModel = event.currentTarget.value === '__manual__'; if (!manualModel) model = event.currentTarget.value}}>
              <option value="">Provider default{provider ? ` · ${provider.model}` : ''}</option>
              {#if model && !models.includes(model)}<option value={model}>{model} · current choice</option>{/if}{#each models as id}<option value={id}>{id}</option>{/each}<option value="__manual__">Enter a model name…</option>
            </select>
            {#if manualModel}<label>Exact Gaffer model<input value={model} oninput={event => model = event.currentTarget.value} required maxlength="256" autocomplete="off" /></label>{/if}
            {#if modelsError}<p class="bad" role="status">{modelsError}</p>{/if}
            <p class="note">Your next message shares this Gaffer conversation’s history with the chosen provider. Working-agent settings stay separate.</p>
            <div class="model-actions"><button class="btn" type="button" onclick={closeChoices}>Cancel</button><button class="btn primary" disabled={!!running || sending || updating || !!pending || (current && !dirty) || !provider?.enabled || (manualModel && !model.trim())}>{updating ? 'Saving…' : current ? 'Apply choice' : 'Use model'}</button></div>
          </fieldset>
          <a href="/account/gaffer">Manage providers and keys →</a>
        </form>
</WorkspaceDialog>{/if}
{#if optionsOpen}<WorkspaceDialog title="Chat options" onclose={()=>{optionsOpen=false;void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}>
  <div class="chat-options"><p><strong>{scopeTitle}</strong></p>
    {#if scope && $gafferSessionContext}<p>Working agent: {toolName($gafferSessionContext.tool)||'Agent'} · {$gafferSessionContext.model||'Model not reported'}</p>{/if}
    <p>Gaffer: {provider?.name||'Choose a provider'} · {model||provider?.model||'Choose a model'}</p>
    <p>Your critical buddy, with a brew standing by. Closing the panel keeps the current answer running. In a structured session, Worker messages lets you enable one instruction for your next request. Use Session follow-up to save a bounded goal while you’re away.</p>
    <a class="btn" href="/account/gaffer">Gaffer settings</a>
    {#if scope&&!unavailable}<button class="btn" type="button" disabled={loading||sending||updating||dirty} onclick={()=>void workerChoices()}>Worker messages</button><button class="btn" type="button" disabled={loading||sending||updating||dirty} onclick={()=>void goalChoices()}>Session follow-up</button>{/if}
    <button class="btn" type="button" onclick={()=>{optionsOpen=false;followedOpen=true}}>Followed sessions</button>
    {#if current}<button class="btn remove" aria-label="Remove conversation" disabled={sending||updating||loading} onclick={()=>{optionsOpen=false;confirming=true}}>Remove chat</button>{/if}
  </div>
</WorkspaceDialog>{/if}

{#if goalOpen&&current}<WorkspaceDialog title="Look after this session" onclose={()=>{goalOpen=false;void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}><GafferGoal id={current.conversation.id} {timeZone} {dateFormat} onchanged={goalChanged}/></WorkspaceDialog>{/if}
{#if followedOpen}<GafferFollowedSessions {timeZone} {dateFormat} onerror={message} onchanged={async()=>{if(current)await goalChanged()}} onselect={()=>{mode='session';followedOpen=false}} onhistory={()=>{followedOpen=false;historyOpen=true}} onclose={()=>{followedOpen=false;void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}/>{/if}
{#if workerOpen&&current}<WorkspaceDialog title="Worker messages" onclose={()=>{workerOpen=false;void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}><WorkerMessaging path={`/api/v2/gaffer/conversations/${encodeURIComponent(current.conversation.id)}/worker`} gaffer {timeZone} {dateFormat} onchanged={workerPermissionSaved} onrequest={()=>{relay=true;workerOpen=false;if(canAutoFocus())void tick().then(()=>composer?.focus({preventScroll:true}))}}/></WorkspaceDialog>{/if}

<style>
  .gaffer-launcher { position: fixed; z-index: 18; right: 24px; bottom: var(--launcher-clearance, calc(var(--bottom-chrome, 0px) + 20px)); min-height: 52px; display: flex; align-items: center; gap: 8px; padding: 4px 16px 4px 8px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card-raised); color: var(--ink); box-shadow: 0 5px 22px var(--scrim); font: 600 14px var(--sans); cursor: pointer; }
  .launcher-portrait { width: 38px; height: 38px; }
  .gaffer-launcher:hover { border-color: var(--accent); }
  .gaffer-chat { position: fixed; z-index: 30; inset: auto var(--gaffer-right, 20px) calc(var(--bottom-chrome, 0px) + 18px) auto; margin: 0; width: 450px; max-width: calc(100vw - 32px); height: min(770px, calc(100dvh - var(--bottom-chrome, 0px) - 48px)); max-height: calc(100dvh - 32px); padding: 0; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card); color: var(--ink); box-shadow: 0 16px 64px var(--scrim); overflow: hidden; }
  .gaffer-chat[open] { display: flex; flex-direction: column; }
  .gaffer-chat.expanded { width: 660px; }
  .gaffer-chat::backdrop { background: var(--scrim); }
  .chat-head { display:flex;align-items:center;gap:10px;padding:10px 14px;border-bottom:1px solid var(--line);flex:none;background:var(--bg-sunk); }
  .portrait { width:36px;height:36px;flex:none; }
  .heading { flex:1;min-width:0; }
  h2 { font:600 23px/1.1 var(--serif);margin:0; }
  h3 { font:600 23px/1.25 var(--serif);margin:0 0 10px; }
  .scope-title { margin:3px 0 0;font-size:14px;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap; }
  .head-actions { display:flex;flex:none;gap:0; }
  .icon { width:36px;min-height:40px;display:grid;place-items:center;background:transparent;color:var(--muted);border:0;border-radius:8px;font-size:24px;cursor:pointer; }
  .icon:hover,.quiet:hover,.model-control:hover,.context-control:hover { background:var(--tint);color:var(--ink); }
  select,input,textarea { width:100%;box-sizing:border-box;min-height:42px;border:1px solid var(--line-strong);border-radius:8px;background:var(--bg);color:var(--ink);padding:8px 10px;font:400 14px/1.45 var(--sans);min-width:0; }
  .conversation-body { display:flex;flex-direction:column;flex:1;min-height:0;overflow:auto;overscroll-behavior:contain; }
  .quiet { border:0;background:transparent;color:var(--accent-text);padding:8px;min-height:36px;font:600 14px var(--sans);cursor:pointer;border-radius:6px; }
  .choices { display:grid;gap:16px;min-width:0; }
  fieldset { border:0;padding:0;margin:0;display:grid;gap:12px;min-width:0; }
  .choices label { font-size:14px;display:grid;gap:6px; }
  .model-head,.model-actions { display:flex;align-items:center;justify-content:space-between;gap:10px; }
  .choices a { font-size:14px; }
  .note { font-size:14px;color:var(--muted);line-height:1.5;margin:4px 0; }
  .transcript { overflow:auto;overscroll-behavior:contain;flex:1;min-height:80px;padding:20px;scrollbar-gutter:stable; }
  .welcome { padding:24px 4px;max-width:34ch; }
  .welcome-mark { font:64px/.7 var(--serif);color:var(--accent-text);display:block;margin:18px 0 10px; }
  .welcome>p { font-size:15px;line-height:1.6; }
  .welcome .note { margin-top:16px;font-size:14px; }
  .turn+.turn { margin-top:28px; }
  .user-message { background:var(--tint);border:1px solid var(--line-soft);border-radius:12px 12px 4px 12px;padding:12px 14px;margin:0 0 18px 24px; }
  .user-message .speaker { position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0); }
  .speaker { margin:0 0 10px;font-size:14px;font-weight:600; }
  .message-text { margin:0;white-space:pre-wrap;overflow-wrap:anywhere;font-size:15px;line-height:1.65; }
  .reply-head { display:flex;flex-wrap:wrap;justify-content:space-between;gap:6px; }
  .state { color:var(--muted);font-size:14px;border-radius:6px; }
  .reply-foot { display:flex;align-items:flex-start;gap:12px;margin-top:10px;color:var(--muted);font-size:14px;font-variant-numeric:tabular-nums; }
  .reply-details { min-width:0;flex:1; }
  .reply-details summary { cursor:pointer;min-height:32px;align-content:center; }
  .reply-details p { overflow-wrap:anywhere;line-height:1.5;margin:4px 0; }
  .turn-error { border-left:2px solid var(--accent);padding-left:10px;font-size:14px;color:var(--muted);line-height:1.5; }
  .thinking { font-size:14px;color:var(--muted); }
  .feedback { flex:none;margin:8px 16px;padding:10px 12px;border:1px solid var(--line-strong);border-radius:8px;background:var(--bg-sunk);font-size:14px;overflow-wrap:anywhere; }
  .feedback p { margin:0 0 6px;line-height:1.4; }
  .bad,.attention { color:var(--bad); }
  .notice { font-size:14px;margin:6px 16px;color:var(--accent-text); }
  .setup,.unavailable { padding:16px;font-size:14px;line-height:1.5; }
  .unavailable { flex:1;overflow:auto; }
  .composer { padding:10px 14px 12px;border-top:1px solid var(--line);flex:none;background:var(--card); }
  textarea { resize:vertical;height:72px;max-height:150px;min-height:54px;background:var(--bg-sunk);border-color:var(--line);border-radius:12px; }
  .compose-actions { display:flex;align-items:center;justify-content:space-between;gap:8px;margin-top:6px; }
  .compose-tools { display:flex;align-items:center;gap:4px;flex:1;min-width:0; }
  .model-control,.context-control { display:flex;align-items:center;gap:6px;min-height:36px;min-width:0;padding:6px 8px;border:0;background:transparent;color:var(--muted);font:500 14px var(--sans);border-radius:8px;cursor:pointer; }
  .model-control { flex:0 1 auto;max-width:58%; }
  .model-control span { overflow:hidden;text-overflow:ellipsis;white-space:nowrap; }
  .model-control svg { flex:none; }
  .context-control { flex:none;white-space:nowrap; }
  .context-dot { width:6px;height:6px;border-radius:50%;background:var(--muted);flex:none; }
  .shared .context-dot { background:var(--good); }.attention .context-dot { background:var(--bad); }
  .send-control { display:grid;place-items:center;width:40px;min-height:40px;flex:none;border:0;border-radius:50%;background:var(--accent);color:var(--accent-ink);cursor:pointer; }
  .send-control:disabled { opacity:.45;cursor:default; }.send-control.stop { background:var(--ink);color:var(--card); }
  .compose-hint { font-size:14px;margin:4px 8px 0;color:var(--muted);font-variant-numeric:tabular-nums; }.compose-hint.bad { color:var(--bad); }
  .chat-options { display:grid;gap:14px;font-size:14px;line-height:1.5; }.chat-options p { margin:0;overflow-wrap:anywhere; }.chat-options .btn { text-align:center; }.remove { color:var(--bad); }
  .goal-status { flex:none;text-align:left;margin:0 16px;min-height:32px;font-size:14px;font-variant-numeric:tabular-nums; }
  .request-mode { margin:0 0 6px;display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:14px;color:var(--accent-text); }.worker-receipt { margin:12px 0;padding:10px;border:1px solid var(--line);border-radius:8px; }.worker-receipt summary { cursor:pointer;font-size:14px; }.worker-receipt p { margin-top:8px; }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; border: 0; }
  button:focus-visible, select:focus-visible, input:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer:coarse) { .icon { width:44px;min-height:44px; }.send-control { width:44px;min-height:44px; }.model-control,.context-control,.reply-details summary { min-height:44px; } }
  @media (max-width:559.98px) {
    .gaffer-launcher { right:12px; }.gaffer-chat { inset:0;width:100%;max-width:100%;height:100dvh;border-radius:0; }.expand { display:none; }
    .chat-head { padding:8px 10px;gap:8px; }.icon { width:40px;min-height:44px; }.transcript { padding:20px 16px; }.composer { padding:8px 12px; }
    .btn,.quiet,select,input { min-height:44px; }.model-control,.context-control { min-height:44px; }.send-control { width:44px;min-height:44px; }textarea { font-size:16px; }
  }
  @media (max-height:600px) { .chat-head { padding-block:4px; }.composer { padding-block:6px; }textarea { height:48px;min-height:44px;max-height:90px; } }
  :global(.gaffer-chat[data-shell-short]) .composer { margin-top:0;padding:6px 12px; }
  :global(.gaffer-chat[data-shell-short]) textarea { height:44px;min-height:44px;max-height:60px; }
</style>
