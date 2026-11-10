<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { ApiError, UnauthorizedError, cancelGafferAnswer, configureGafferConversation, createGafferConversation, gafferTurnEventsURL, getGafferConversation, getGafferModels, getGafferSettings, removeGafferConversation, sendGafferMessage,getWorkerSettings,getGafferGoal,
    type GafferConversationView, type GafferSettings, type GafferTurn } from './api'
  import { gafferSessionContext, gafferChatEntry, GAFFER_GOALS_CHANGED } from './gafferContext'
  import { gafferMessageBytes, gafferMessageId, gafferSendKey, gafferTurnLabel, newerGafferTurn, parseGafferTurn, readGafferReceipt, writeGafferReceipt, type PendingGafferSend } from './gafferChat'
  import GafferCompanionAvatar from './GafferCompanionAvatar.svelte'
  import {readGafferLayout,moveGafferPanel,type GafferPanelSize,type GafferPanelSide} from './gafferPanel'
  import {keepLauncherClear} from './floatingLauncher'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import GafferChatHistory from './GafferChatHistory.svelte'
  import GafferProse from './GafferProse.svelte'
  import WorkerMessaging from './WorkerMessaging.svelte'
 import GafferGoal from './GafferGoal.svelte'
 import { goalPrefill } from './sessions/goalPrefill'
 import { copyText } from './clipboard'
  import GafferSessionPicker from './GafferSessionPicker.svelte'
  import GafferFollowedSessions from './GafferFollowedSessions.svelte'
  import ContextSharing from './ContextSharing.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { shell } from './dialogShell'
  import { canAutoFocus } from './focus'
  import { formatWhen } from './time'
  import { toolName } from './agentLaunch'

  let { userId, onerror, timeZone = '', dateFormat = 'locale' }: { userId: number; onerror?: (e: unknown) => void; timeZone?: string; dateFormat?: string } = $props()
  const contextDescriptionId = $props.id()
  const composeHelpId = `${contextDescriptionId}-keys`
  let historyOpen = $state(false), optionsOpen = $state(false), workerOpen = $state(false), goalOpen=$state(false), relay = $state(false)
  let followedOpen = $state(false)
  let sessionPickerOpen = $state(false)
  let modelButton = $state<HTMLButtonElement>(), historyButton = $state<HTMLButtonElement>(), contextButton = $state<HTMLButtonElement>(), optionsButton = $state<HTMLButtonElement>(), freshButton = $state<HTMLButtonElement>()
  let modelBefore: {providerId:string;model:string;manual:boolean} | undefined
  let contextOpen = $state(false)
  let open = $state(false), phone = $state(false), full = $state(false), mode = $state('session'), keyboardEditing=$state(false)
  // A phone chat carries Gaffer in its status row: the floating circle would hover over the conversation. Remove it
  // from the page rather than merely hiding it, so keyboard and assistive order never reach an invisible duplicate.
  let phoneChat = $state(false)
  // The saved corner is read while the widget is created, so the workspace reserves Gaffer's space in the same frame the
  // launcher can first be seen. Waiting for onMount left a frame where the panel overlapped the workspace.
  const savedGafferLayout=(()=>{try{return readGafferLayout(localStorage.getItem(`reet.gaffer.layout.${userId}`))}catch{return readGafferLayout(null)}})()
  let panelSize=$state<GafferPanelSize>(savedGafferLayout.size),panelSide=$state<GafferPanelSide>(savedGafferLayout.side)
  try{document.documentElement.setAttribute('data-gaffer-anchor',savedGafferLayout.side)}catch{/* no document outside a browser */}

  let layoutRevision=$state(0)
  const modal=$derived(phone||full)
  let panel = $state<HTMLDialogElement>(), launcher = $state<HTMLButtonElement>(), composer = $state<HTMLTextAreaElement>(), transcript = $state<HTMLDivElement>()
  let inlineOpener: HTMLElement | undefined
  let settings = $state<GafferSettings | null>(null), view = $state<GafferConversationView | null>(null), loadedScope = $state<string | null>(null)
  let loading = $state(false), sending = $state(false), updating = $state(false), cancelling = $state(false), confirming = $state(false), freshConfirm = $state(false)
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
  const contextLabel = $derived(current?.context?.needsReview ? 'Review context sharing' : current?.context?.grant && (current.context.grant.status || current.context.grant.history || current.context.grant.text) ? 'Selected context shared' : 'Context off · typed messages only')
  // A fresh chat is only useful when this saved conversation holds something to
  // clear: messages, a running goal or shared context. An empty chat keeps the
  // tool disabled rather than pretending to do something.
  const canStartFresh = $derived(!!current && !loading && !sending && !updating && !pending && (current.turns.length > 0 || !!current.goal || !!(current.context?.grant && (current.context.grant.status || current.context.grant.history || current.context.grant.text))))
  const brainTitle = $derived([provider?.name, model || provider?.model].filter(Boolean).join(' · ') || 'No provider chosen yet')
  const freshTitle = $derived(canStartFresh ? 'Start a fresh chat' : !current ? 'No saved chat to clear yet' : loading || sending || updating || pending ? 'Wait for this chat to settle' : 'Nothing to clear in this chat yet')

  onMount(() => {
    document.documentElement.setAttribute('data-gaffer-anchor',panelSide)
    const entry = (opener: HTMLElement, intent?: 'followed') => {
      inlineOpener = opener
      if (intent === 'followed') followedOpen = true
      else open = true
    }
    gafferChatEntry.set(entry)
    const arrival=new URL(location.href)
    if(arrival.pathname==='/sessions'&&arrival.searchParams.get('gaffer')==='1'){
      open=true;arrival.searchParams.delete('gaffer');history.replaceState(history.state,'',arrival)
    }
    const mq = matchMedia('(max-width:767px), (max-height:500px) and (pointer:coarse) and (orientation:landscape)'); const resize = () => phone = mq.matches
    resize(); mq.addEventListener('change', resize)
    // Give the worker the whole keyboard-visible area while typing. Returning
    // from the keyboard restores the same corner, rather than moving the mascot.
    const vv=window.visualViewport,coarse=matchMedia('(pointer:coarse)')
    let seenWidth=innerWidth,tallest=innerHeight,frame=0
    const measureKeyboard=()=>{frame=0;if(innerWidth!==seenWidth){seenWidth=innerWidth;tallest=0}tallest=Math.max(tallest,innerHeight);const focus=document.activeElement;keyboardEditing=coarse.matches&&!!focus?.closest('.pane,.chat-pane')&&((!!vv&&Math.abs(vv.scale-1)<.01&&vv.height<innerHeight*.85)||innerHeight<tallest*.85);document.documentElement.toggleAttribute('data-gaffer-editing',keyboardEditing)}
    const scheduleKeyboard=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(measureKeyboard)}
    measureKeyboard();window.addEventListener('resize',scheduleKeyboard);vv?.addEventListener('resize',scheduleKeyboard);document.addEventListener('focusin',scheduleKeyboard);document.addEventListener('focusout',scheduleKeyboard)
    return () => { cancelAnimationFrame(frame);window.removeEventListener('resize',scheduleKeyboard);vv?.removeEventListener('resize',scheduleKeyboard);document.removeEventListener('focusin',scheduleKeyboard);document.removeEventListener('focusout',scheduleKeyboard);document.documentElement.removeAttribute('data-gaffer-editing');document.documentElement.removeAttribute('data-gaffer-anchor');gafferChatEntry.update(current => current === entry ? null : current); mq.removeEventListener('change', resize); dispose(); drafts = {}; pendingMemory.clear() }
  })
  $effect(() => {
    if (!phone) { phoneChat = false; return }
    const update = () => { phoneChat = !!document.querySelector('.chat-pane') }
    update()
    const observer = new MutationObserver(update)
    observer.observe(document.body, { childList: true, subtree: true })
    return () => observer.disconnect()
  })
  $effect(() => { const key = scope, shown = open; untrack(() => { contextOpen=false; choicesOpen=false; optionsOpen=false; workerOpen=false;goalOpen=false;followedOpen=false;sessionPickerOpen=false;relay=false; if (shown) void load(key); else dispose() }) })
  $effect(() => {
    if (!open || !panel) return
    const mobile = modal, dialog = panel
    if (dialog.open) dialog.close()
    if (mobile) dialog.showModal(); else dialog.show()
    if (canAutoFocus()) void tick().then(() => { if (dialog.open && !mobile) composer?.focus({preventScroll:true}) })
  })
  function layout(size=panelSize,side=panelSide){panelSize=size;panelSide=side;layoutRevision++;document.documentElement.setAttribute('data-gaffer-anchor',side);try{localStorage.setItem(`reet.gaffer.layout.${userId}`,JSON.stringify({size,side}))}catch{/* optional preference */}}
  // The snap-back tool asks the drag action to return the panel (and any native
  // resize) to the saved corner without changing the saved preset itself.
  function snapPanel(){panel?.dispatchEvent(new Event('gaffer-place'))}
  function startFresh(){if(canStartFresh)freshConfirm=true}
  function phoneShell(node: HTMLDialogElement, mobile: boolean) {
    let handle: ReturnType<typeof shell>
    function update(value: boolean) { handle?.destroy?.(); node.removeAttribute('data-shell'); if (value) handle = shell(node, {sheet:'full'}); else handle = undefined }
    update(mobile)
    return { update, destroy() { handle?.destroy?.() } }
  }
  function dispose() { epoch++; request?.abort(); modelRequest?.abort(); modelEpoch++; stream?.close(); stream = undefined }
  function focusEntry(button?: HTMLElement) {
    (button?.isConnected ? button : inlineOpener?.isConnected ? inlineOpener : launcher)?.focus({preventScroll:true})
  }
  function message(e: unknown): string { if (e instanceof UnauthorizedError) onerror?.(e); return e instanceof Error ? e.message : 'Gaffer could not reach the controller just now.' }
  function remember(key: string, value: PendingGafferSend | null) {
    if (value) pendingMemory.set(key, value); else pendingMemory.delete(key)
    writeGafferReceipt(userId, key, value)
    if (scope === key) pending = value
  }
  async function load(key = scope) {
    dispose(); const mine = epoch, controller = new AbortController(); request = controller
    loading = true; sending = false; updating = false; cancelling = false; confirming = false; freshConfirm = false; error = ''; notice = ''; streamError = ''; loadedScope = null; view = null
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
  async function close() { wantedDraft = null; open = false;full=false; confirming = false; freshConfirm = false; historyOpen = false; choicesOpen = false; optionsOpen = false; workerOpen = false; goalOpen = false; followedOpen = false; relay = false; dispose(); await tick(); (inlineOpener?.isConnected ? inlineOpener : launcher)?.focus({preventScroll:true}) }
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
  // `draft` is a goal the session overview wrote for the person; the editor opens with it and nothing is started.
 let goalDraft=$state(''),wantedDraft=$state<{id:string;text:string}|null>(null),goalSeen=0
 async function goalChoices(draft=''){
 optionsOpen=false
 if(loading||sending||updating||dirty||unavailable||!scope)return
 goalDraft=draft
 const mine=epoch;updating=true;error=''
 try{if(!current){const c=await createGafferConversation(scope,providerId,model,request?.signal);if(mine!==epoch)return;view=c}if(mine===epoch)goalOpen=true}catch(e){if(mine===epoch){error=message(e);if(draft){goalDraft='';void copyText(draft).then(ok=>{if(ok&&mine===epoch)notice='The goal could not be opened here, so it was copied instead.'})}}}finally{if(mine===epoch)updating=false}
 }
 // The overview asks for the follow-up editor for the chat session Gaffer is showing. Answering at once says it is ours; the editor opens
 // as soon as the chat has loaded, which is why this only records the wish.
 $effect(()=>{
  const ask=$goalPrefill,ctx=$gafferSessionContext
  if(!ask||ask.nonce<=goalSeen||!ctx||ask.sessionId!==ctx.id)return
  goalSeen=ask.nonce
  const taken=untrack(()=>!!ctx.structured&&settings?.available!==false&&!!ask.text.trim())
  if(taken)untrack(()=>{mode='session';wantedDraft={id:ctx.id,text:ask.text};open=true})
  ask.answer?.(taken)
 })
 $effect(()=>{
  if(wantedDraft===null||!open||goalOpen||!scope||unavailable||loadedScope!==scope||loading||sending||updating||dirty)return
  // A wish made for another session (Gaffer was moved on before the chat loaded) is dropped, not carried to this one.
  const wish=wantedDraft;wantedDraft=null
  if(wish.id===scope)void untrack(()=>goalChoices(wish.text))
 })
 async function goalChanged(){
 window.dispatchEvent(new Event(GAFFER_GOALS_CHANGED))
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
  // Enter sends and never starts a new line; Shift+Enter keeps the line break.
  // The send guard still decides whether the message is admitted.
  function composeKey(event: KeyboardEvent) {
    if (!gafferSendKey(event)) return
    event.preventDefault()
    void send()
  }
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
  async function removeConversation(fresh = false) {
    if (!current) return
    const mine = epoch, key = scope; updating = true; confirming = false; freshConfirm = false; error = ''; streamError = ''; cancelling = false
    try { await removeGafferConversation(current.conversation, request?.signal); if (mine === epoch) { stream?.close(); stream = undefined; remember(key, null); view = null; drafts[key] = ''; notice = fresh ? 'Fresh chat ready. Ask away.' : 'Conversation removed. Fresh brew, fresh start.'; if (fresh && canAutoFocus()) void tick().then(() => composer?.focus({preventScroll:true})) } }
    catch (e) { if (mine === epoch) error = message(e) }
    finally { if (mine === epoch) updating = false }
  }
</script>

<div class="gaffer-anchor-space" data-gaffer-space aria-hidden="true"></div>
{#if !open&&!keyboardEditing&&!phoneChat}
  <button class="gaffer-launcher" data-side={panelSide} bind:this={launcher} use:keepLauncherClear aria-label="Ask Gaffer" aria-expanded="false" aria-haspopup="dialog" title="Ask Gaffer" onclick={() => open = true}>
    <GafferCompanionAvatar size={56}/>
  </button>
{/if}
{#if open}
  <dialog class="gaffer-chat" data-size={panelSize} data-side={panelSide} data-full={full||undefined} bind:this={panel} use:phoneShell={modal} use:moveGafferPanel={{reset:`${panelSize}:${panelSide}:${full}:${phone}:${layoutRevision}`,modal}} aria-labelledby="gaffer-chat-title" aria-modal={modal ? 'true' : 'false'} oncancel={event => {event.preventDefault(); void close()}} onkeydown={event => { if (event.key === 'Escape') {event.preventDefault(); event.stopPropagation(); void close()} }}>
    <header class="chat-head" data-shell-head data-gaffer-drag>
      <button class="identity" type="button" data-gaffer-move disabled={modal} aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown" aria-label="Move Gaffer panel with arrow keys or by dragging" title={modal ? 'Gaffer moves with the screen on this size' : 'Drag to move · arrow keys nudge · double-click to snap back'}><GafferCompanionAvatar size={32} thinking={!!running}/></button>
      <div class="heading"><h2 id="gaffer-chat-title">Gaffer</h2><p class="scope-title" class:follow-scope-label={!scope} title={`${scopeTitle} · ${brainTitle}`}>{#if scope}{scopeTitle}{:else}<span class="sr-only">General Gaffer chat · </span>Follow a session <span aria-hidden="true">⌄</span>{/if}</p>{#if !scope}<button type="button" class="follow-scope" aria-label="Follow a session" title="Choose a session for Gaffer" onclick={()=>sessionPickerOpen=true}></button>{/if}</div>
      <div class="head-actions" role="group" aria-label="Gaffer panel tools">
        <button class="icon" type="button" bind:this={freshButton} aria-label="New conversation" title={freshTitle} aria-haspopup="dialog" disabled={!canStartFresh} onclick={startFresh}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9" /><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg></button>
        <button class="icon" bind:this={historyButton} aria-label="Conversation history" title="Conversation history" aria-haspopup="dialog" onclick={() => historyOpen = true}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 11a9 9 0 1 1 2.6 7M3 4v7h7M12 7v5l3 2" /></svg></button>
        <button class="icon" bind:this={optionsButton} aria-label="Chat options" title="Chat options" aria-haspopup="dialog" onclick={() => optionsOpen=true}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/></svg></button>
        <button class="icon expand" aria-label={full ? 'Restore panel' : 'Full screen'} aria-pressed={full} title={full ? 'Restore panel' : 'Full screen'} onclick={() => full = !full}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d={full ? 'M4 10h6V4M20 14h-6v6M10 10 3 3m11 11 7 7' : 'M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7'} /></svg></button>
        <button class="icon" data-shell-close aria-label="Close Gaffer" title="Close the panel; keep the current answer running" onclick={() => void close()}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
      </div>
    </header>
    <div class="panel-layout"><div class="panel-sizes" role="group" aria-label="Gaffer panel size">{#each [['compact','Compact'],['comfortable','Default'],['expanded','Expanded']] as [value,label]}<button type="button" aria-pressed={panelSize===value} disabled={full} onclick={()=>layout(value as GafferPanelSize)}>{label}</button>{/each}</div><div class="panel-position" role="group" aria-label="Panel position"><button type="button" aria-label="Place Gaffer on the left" title="Dock Gaffer on the left" aria-pressed={panelSide==='left'} disabled={full} onclick={()=>layout(panelSize,'left')}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2.5" /><path d="M9 3v18" /></svg></button><button type="button" aria-label="Place Gaffer on the right" title="Dock Gaffer on the right" aria-pressed={panelSide==='right'} disabled={full} onclick={()=>layout(panelSize,'right')}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2.5" /><path d="M15 3v18" /></svg></button><button class="snap" type="button" aria-label="Snap Gaffer back to its corner" title="Reset a moved or resized panel" disabled={full} onclick={snapPanel}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 2.6-6.4" /><path d="M3 4v5h5" /></svg></button></div></div>
    {#if unavailable}
      <div class="unavailable" data-shell-body><h3>This session uses a terminal.</h3><p>A session companion needs a supported structured chat. You can still talk to Gaffer in General chat.</p><button class="btn" onclick={() => mode = 'general'}>Open general chat</button></div>
    {:else}
      <div class="conversation-body" data-shell-body>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrolling history must be reachable with a keyboard.) -->
      <div class="transcript" bind:this={transcript} data-shell-body onscroll={() => {if (transcript) stickToBottom = transcript.scrollHeight - transcript.scrollTop - transcript.clientHeight < 90}} tabindex="0" role="region" aria-label="Gaffer conversation messages">
        {#if loading}<p class="empty" role="status">Finding our place…</p>
        {:else if loadedScope === scope && !error && !current?.turns.length}<div class="welcome"><GafferCompanionAvatar size={112} sitting/><div class="welcome-copy"><h3>Ey up. I’m Gaffer.</h3><p>Talk it through, question the plan, or ask for a second pair of eyes.</p><p class="note">{scope?'Choose “Look after this session” to review access and set a goal in one place.':'A brew and a second pair of eyes. Open a working session when you’d like Gaffer to look after its progress.'}</p></div></div>{/if}
        {#if loadedScope === scope}{#each current?.turns ?? [] as turn (turn.id)}
          <article class="turn">
            <div class="user-message"><p class="speaker">{turn.goalId?'Goal review':'You'}</p><p class="message-text">{turn.prompt}</p></div>
            <div class="gaffer-message"><div class="reply-head"><p class="speaker"><GafferCompanionAvatar size={36} thinking={turn.state==='running'} seed={parseInt(turn.id.slice(-6),16)||0}/>Gaffer</p><span class="state" class:bad={turn.state === 'failed'}>{gafferTurnLabel(turn)}</span></div>
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
        <p class="sr-only" id={composeHelpId}>Enter sends your message. Shift plus Enter adds a new line.</p>
        <textarea id="gaffer-message" bind:this={composer} value={drafts[scope] ?? ''} oninput={event => drafts[scope] = event.currentTarget.value} disabled={sending || loading} rows="2" placeholder="Talk it through with Gaffer…" aria-describedby={composeHelpId} title="Enter to send · Shift + Enter for a new line" onkeydown={composeKey}></textarea>
        <div class="compose-actions">
          <div class="compose-tools"><button class="model-control" bind:this={modelButton} type="button" aria-label="Model" aria-haspopup="dialog" title={`Gaffer: ${provider?.name || 'Choose a provider'} · ${model || provider?.model || 'Choose a model'}`} disabled={loading||sending||updating} onclick={openChoices}><span>{model || provider?.model || 'Choose model'}</span><svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m6 9 6 6 6-6"/></svg></button>
          {#if scope&&!unavailable}<button class="context-control" class:shared={current?.goal?.state==='active'} type="button" aria-label={current?.goal?`${current.goal.state==='active'?'Gaffer following':current.goal.state==='needs_user'?'Goal needs you':current.goal.state==='limit'?'Goal limit reached':`Goal ${current.goal.state}`} · ${current.goal.usedReviews}/${current.goal.maxReviews} reviews`:'Look after this session'} title="Look after this session" disabled={loading||sending||updating||dirty} onclick={()=>void goalChoices()}><span class="context-dot" aria-hidden="true"></span><span>{current?.goal?(current.goal.state==='active'?'Following':current.goal.state==='needs_user'?'Needs you':current.goal.state==='limit'?'Limit reached':current.goal.state):'Look after'}</span></button>{:else}          <button class="context-control" bind:this={contextButton} class:attention={current?.context?.needsReview} class:shared={contextLabel==='Selected context shared'} type="button" aria-label="Context" aria-describedby={contextDescriptionId} aria-haspopup="dialog" title={contextLabel} disabled={loading||sending||updating||dirty} onclick={()=>void contextChoices()}><span class="context-dot" aria-hidden="true"></span><span>{current?.context?.needsReview?'Review context':contextLabel==='Selected context shared'?'Context on':'Context off'}</span></button>{/if}<span id={contextDescriptionId} class="sr-only">{contextLabel}</span></div>
          {#if running}<button class="send-control stop" type="button" aria-label="Cancel answer" title="Cancel Gaffer’s answer" disabled={cancelling || sending} onclick={() => void cancelAnswer()}><svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><rect x="5" y="5" width="14" height="14" rx="2"/></svg></button>{:else}<button class="send-control" aria-label="Send message" title="Send message · Enter" disabled={blocked || !drafts[scope]?.trim() || bytes > 8000}><svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19V5m-6 6 6-6 6 6"/></svg></button>{/if}
        </div>
        {#if bytes>7200||dirty}<p class="compose-hint" class:bad={bytes>8000}>{bytes>7200?`${bytes.toLocaleString()} / 8,000 bytes`:'Apply your model choice first'}</p>{/if}
      </form>
    {/if}

  </dialog>
{/if}
{#if historyOpen}<GafferChatHistory {timeZone} {dateFormat} activeSession={$gafferSessionContext} currentScope={scope} onselect={value=>{mode=value;historyOpen=false;open=true}} onerror={message} onclose={()=>{historyOpen=false;void tick().then(()=>focusEntry(open ? historyButton : undefined))}} onremoved={c => { if (current?.conversation.id === c.id || pending?.conversationId === c.id) { remember(scope, null); drafts[scope] = ''; void load() } }} />{/if}
{#if confirming && current}<ConfirmDialog title="Remove this Gaffer conversation?" body="This deletes its messages and stops its current answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Remove conversation" danger oncancel={() => confirming = false} onconfirm={() => void removeConversation()} />{/if}
{#if freshConfirm && current}<ConfirmDialog title="Start a new Gaffer conversation?" body="This clears the saved messages, shared context and any session goal for this chat, and stops its current answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Start fresh chat" danger oncancel={() => {freshConfirm = false; void tick().then(() => freshButton?.focus({preventScroll:true}))}} onconfirm={() => void removeConversation(true)} />{/if}

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
    <p>Your critical buddy, with a brew standing by. Closing the panel keeps the current answer running.</p>
    <a class="btn" href="/account/gaffer">Gaffer settings</a>
    {#if scope&&!unavailable}<button class="btn" type="button" disabled={loading||sending||updating||dirty} onclick={()=>void goalChoices()}>Look after this session</button><details><summary>Advanced session controls</summary><div class="chat-options"><button class="btn" type="button" disabled={loading||sending||updating||dirty} onclick={()=>void workerChoices()}>Worker messages</button><button class="btn" type="button" title={contextLabel} aria-describedby={contextDescriptionId} disabled={loading||sending||updating||dirty} onclick={()=>{optionsOpen=false;void contextChoices()}}>Context sharing</button></div></details>{/if}
    <button class="btn" type="button" onclick={()=>{optionsOpen=false;followedOpen=true}}>Factory floor</button>
    {#if current}<button class="btn remove" aria-label="Remove conversation" disabled={sending||updating||loading} onclick={()=>{optionsOpen=false;confirming=true}}>Remove chat</button>{/if}
  </div>
</WorkspaceDialog>{/if}

{#if sessionPickerOpen}<GafferSessionPicker onclose={()=>{sessionPickerOpen=false;void tick().then(()=>composer?.focus({preventScroll:true}))}} onselect={()=>{mode='session';sessionPickerOpen=false}} />{/if}
{#if goalOpen&&current}<WorkspaceDialog title="Look after this session" onclose={()=>{goalOpen=false;goalDraft='';void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}><GafferGoal id={current.conversation.id} {userId} {timeZone} {dateFormat} draft={goalDraft} onchanged={goalChanged}/></WorkspaceDialog>{/if}
{#if followedOpen}<GafferFollowedSessions {userId} {timeZone} {dateFormat} onerror={message} onchanged={async()=>{if(open&&scope&&current)await goalChanged();else window.dispatchEvent(new Event(GAFFER_GOALS_CHANGED))}} onselect={()=>{mode='session';followedOpen=false;open=true}} onhistory={()=>{followedOpen=false;historyOpen=true}} onclose={()=>{followedOpen=false;void tick().then(()=>focusEntry(open ? optionsButton : undefined))}}/>{/if}
{#if workerOpen&&current}<WorkspaceDialog title="Worker messages" onclose={()=>{workerOpen=false;void tick().then(()=>optionsButton?.focus({preventScroll:true}))}}><WorkerMessaging path={`/api/v2/gaffer/conversations/${encodeURIComponent(current.conversation.id)}/worker`} gaffer {timeZone} {dateFormat} onchanged={workerPermissionSaved} onrequest={()=>{relay=true;workerOpen=false;if(canAutoFocus())void tick().then(()=>composer?.focus({preventScroll:true}))}}/></WorkspaceDialog>{/if}

<style>

  .gaffer-launcher,.gaffer-chat{--gaffer-floor:calc(var(--bottom-chrome,0px) - var(--gaffer-reserve,0px) + 40px)}
  .gaffer-anchor-space{position:fixed;inset:auto 0 0;height:var(--bottom-chrome,0px);pointer-events:none;z-index:-1}
  .gaffer-launcher{position:fixed;z-index:18;right:20px;bottom:var(--launcher-clearance,var(--gaffer-floor));display:flex;width:56px;height:56px;padding:0;border:0;border-radius:50%;background:var(--card);box-shadow:0 5px 22px var(--scrim);cursor:pointer}
  .gaffer-launcher[data-side=left]{right:auto;left:20px}
  .gaffer-chat{box-sizing:border-box;position:fixed;z-index:30;inset:auto 16px var(--gaffer-floor) auto;margin:0;width:min(28rem,calc(100vw - 32px));height:min(42rem,calc(100dvh - var(--gaffer-floor) - 24px));min-width:min(22rem,calc(100vw - 32px));min-height:min(26rem,calc(100dvh - var(--gaffer-floor) - 24px));max-width:calc(100vw - 32px);max-height:calc(100dvh - var(--gaffer-floor) - 24px);padding:0;border:1px solid var(--line-strong);border-radius:12px;background:var(--card);color:var(--ink);box-shadow:0 16px 64px var(--scrim);overflow:hidden;resize:both}
  .gaffer-chat[open]{display:flex;flex-direction:column}.gaffer-chat[data-side=left]{inset:auto auto var(--gaffer-floor) 16px}
  .gaffer-chat[data-size=compact]{width:min(24rem,calc(100vw - 32px));height:min(32rem,calc(100dvh - var(--gaffer-floor) - 24px))}
  .gaffer-chat[data-size=expanded]{width:min(46rem,calc(100vw - 32px));height:min(52rem,calc(100dvh - var(--gaffer-floor) - 24px))}
  .gaffer-chat[data-full]{inset:16px; width:calc(100vw - 32px);height:calc(100dvh - 32px);max-height:calc(100dvh - 32px);resize:none}
  .gaffer-chat::backdrop{background:var(--scrim)}
  .chat-head{display:flex;align-items:center;gap:10px;padding:8px 12px;border-bottom:1px solid var(--line);flex:none;background:linear-gradient(180deg,var(--card-raised),var(--bg-sunk));cursor:grab;touch-action:none;user-select:none;transition:box-shadow .15s ease}
  .chat-head:active{cursor:grabbing}
  :global(.gaffer-chat[data-gaffer-dragging]){box-shadow:0 24px 80px var(--scrim);cursor:grabbing}
  .identity{display:grid;place-items:center;flex:none;width:40px;height:40px;padding:0;border:1px solid var(--line-soft);border-radius:50%;background:var(--card);cursor:grab;touch-action:none;transition:border-color .15s ease,box-shadow .15s ease,transform .15s ease}
  .identity:hover:not(:disabled){border-color:var(--accent);box-shadow:0 0 0 3px var(--tint)}
  .identity:active:not(:disabled){cursor:grabbing;transform:scale(.96)}
  .identity:disabled{cursor:default;border-color:transparent;background:transparent}
  .identity:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .panel-layout{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:4px 10px;border-bottom:1px solid var(--line);background:var(--card);flex:none}
  .panel-sizes,.panel-position{display:flex;align-items:center;gap:3px}
  .panel-layout button{display:grid;place-items:center;min-height:32px;padding:5px 10px;border:1px solid transparent;border-radius:8px;background:transparent;color:var(--muted);font:500 12.5px var(--sans);cursor:pointer;transition:background .15s ease,color .15s ease,border-color .15s ease}
  .panel-layout button:hover:not(:disabled){background:var(--tint);color:var(--ink)}
  .panel-layout button[aria-pressed=true]{color:var(--accent-text);background:var(--tint);border-color:var(--accent)}
  .panel-layout button:disabled{opacity:.45;cursor:default}
  .panel-position button{width:32px;padding:0}
  /* A moved panel marks its snap-back tool, so the way home stays visible. */
  :global(.gaffer-chat[data-gaffer-placed]) .snap{color:var(--accent-text);border-color:var(--accent)}
  .panel-layout svg{display:block}
  .heading{position:relative;flex:1;min-width:0}
  .follow-scope{position:absolute;inset:0;min-height:44px;width:100%;border:0;border-radius:4px;background:transparent;cursor:pointer}
  .follow-scope:focus-visible{outline:2px solid var(--accent-text);outline-offset:2px}
  .scope-title.follow-scope-label{color:var(--accent-text)}
  h2{font:600 18px/1.15 var(--serif);margin:0;letter-spacing:-.01em}
  h3{font:600 23px/1.25 var(--serif);margin:0 0 10px}
  .scope-title{margin:3px 0 0;font-size:13px;line-height:1.3;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .head-actions{display:flex;align-items:center;gap:2px;flex:none}
  .icon{display:grid;place-items:center;width:36px;height:36px;min-height:36px;padding:0;border:1px solid transparent;border-radius:9px;background:transparent;color:var(--muted);cursor:pointer;transition:background .15s ease,color .15s ease,border-color .15s ease}
  .icon:hover:not(:disabled){background:var(--tint);color:var(--ink)}
  .icon:active:not(:disabled){background:var(--accent-wash)}
  .icon:disabled{opacity:.4;cursor:default}
  .icon[aria-pressed=true]{background:var(--tint);border-color:var(--accent);color:var(--accent-text)}
  .head-actions svg{display:block}
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
  .transcript { overflow:auto;overscroll-behavior:contain;flex:1;min-height:80px;padding:22px 20px;scrollbar-gutter:stable; }
  .welcome { padding:16px 4px;display:flex;align-items:flex-start;flex-wrap:wrap;gap:16px 20px; }.welcome-copy{flex:1 1 220px;min-width:0}
  .welcome h3 { margin:0 0 10px; }
  .welcome-copy>p { font-size:15px;line-height:1.6; }
  .welcome .note { margin-top:16px;font-size:14px; }
  .turn+.turn { margin-top:28px; }
  .user-message { background:var(--tint);border:1px solid var(--line-soft);border-radius:12px 12px 4px 12px;padding:12px 14px;margin:0 0 18px 24px; }
  .user-message .speaker { position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0); }
  .speaker { display:flex;align-items:center;gap:8px;margin:0 0 10px;font-size:14px;font-weight:600; }
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
  .request-mode { margin:0 0 6px;display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:14px;color:var(--accent-text); }.worker-receipt { margin:12px 0;padding:10px;border:1px solid var(--line);border-radius:8px; }.worker-receipt summary { cursor:pointer;font-size:14px; }.worker-receipt p { margin-top:8px; }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; border: 0; }
  button:focus-visible, select:focus-visible, input:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer:coarse) { .icon { width:44px;height:44px;min-height:44px; }.identity { width:44px;height:44px; }.panel-layout button { min-height:44px; }.send-control { width:44px;min-height:44px; }.model-control,.context-control,.reply-details summary { min-height:44px; } }
  @media (max-width:767px), (max-height:500px) and (pointer:coarse) and (orientation:landscape) {
    .gaffer-launcher,.gaffer-chat{--gaffer-floor:calc(var(--bottom-chrome,0px) - var(--gaffer-reserve,0px) + max(8px,env(safe-area-inset-bottom,0px)))}
    .gaffer-launcher{right:12px}.gaffer-launcher[data-side=left]{left:12px}
    .gaffer-chat,.gaffer-chat[data-size],.gaffer-chat[data-side],.gaffer-chat[data-full]{inset:calc(var(--shell-top,0px) + 12px) auto auto 12px;width:calc(100vw - 24px);min-width:0;max-width:calc(100vw - 24px);height:calc(var(--shell-height,100dvh) - 24px);min-height:0;max-height:calc(var(--shell-height,100dvh) - 24px);resize:none}.expand,.panel-layout{display:none}
    .chat-head { padding:8px 10px;gap:8px; }.icon { width:40px;height:44px;min-height:44px; }.identity { width:44px;height:44px; }.transcript { padding:20px 16px; }.composer { padding:8px 12px; }
    .btn,.quiet,select,input { min-height:44px; }.model-control,.context-control { min-height:44px; }.send-control { width:44px;min-height:44px; }textarea { font-size:16px; }
  }
  @media (max-width:380px) { .chat-head { padding:8px;gap:6px; }.head-actions { gap:0; }.scope-title { font-size:12px; } }
  @media (max-height:600px) { .chat-head { padding-block:4px; }.composer { padding-block:6px; }textarea { height:48px;min-height:44px;max-height:90px; } }
  :global(.gaffer-chat[data-shell-short]) .composer { margin-top:0;padding:6px 12px; }
  :global(.gaffer-chat[data-shell-short]) textarea { height:44px;min-height:44px;max-height:60px; }
</style>
