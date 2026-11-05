<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { ApiError, UnauthorizedError, cancelGafferAnswer, configureGafferConversation, createGafferConversation, gafferTurnEventsURL, getGafferConversation, getGafferModels, getGafferSettings, removeGafferConversation, sendGafferMessage,
    type GafferConversationView, type GafferSettings, type GafferTurn } from './api'
  import { gafferSessionContext } from './gafferContext'
  import { gafferMessageBytes, gafferMessageId, gafferTurnLabel, newerGafferTurn, parseGafferTurn, readGafferReceipt, writeGafferReceipt, type PendingGafferSend } from './gafferChat'
  import GafferAvatar from './terminal/GafferAvatar.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import GafferChatHistory from './GafferChatHistory.svelte'
  import ContextSharing from './ContextSharing.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { shell } from './dialogShell'
  import { canAutoFocus } from './focus'
  import { keepLauncherClear } from './floatingLauncher'
  import { formatWhen } from './time'
  import { toolName } from './agentLaunch'

  let { userId, onerror, timeZone = '', dateFormat = 'locale' }: { userId: number; onerror?: (e: unknown) => void; timeZone?: string; dateFormat?: string } = $props()
  let historyOpen = $state(false)
  let contextOpen = $state(false)
  let open = $state(false), phone = $state(false), expanded = $state(false), mode = $state('session')
  let panel = $state<HTMLDialogElement>(), launcher = $state<HTMLButtonElement>(), composer = $state<HTMLTextAreaElement>(), transcript = $state<HTMLDivElement>()
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

  onMount(() => {
    const mq = matchMedia('(max-width: 559.98px)'); const resize = () => phone = mq.matches
    resize(); mq.addEventListener('change', resize)
    return () => { mq.removeEventListener('change', resize); dispose(); drafts = {}; pendingMemory.clear() }
  })
  $effect(() => { const key = scope, shown = open; untrack(() => { contextOpen=false; if (shown) void load(key); else dispose() }) })
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
      model = conversation?.conversation.model ?? ''; choicesOpen = !conversation && !providerId
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
  async function close() { open = false; confirming = false; historyOpen = false; dispose(); await tick(); launcher?.focus({preventScroll:true}) }
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
  async function applyChoice(event: SubmitEvent) {
    event.preventDefault(); if (!current) return
    const mine = epoch; updating = true; error = ''; notice = ''
    try { const answer = await configureGafferConversation(current.conversation, providerId, model, request?.signal); if (mine === epoch) { view = answer; choicesOpen = false; notice = 'Choice of brains saved.' } }
    catch (e) { if (mine === epoch) error = message(e) }
    finally { if (mine === epoch) updating = false }
  }
  async function contextChoices(){
    if(loading||sending||updating||dirty||unavailable)return
    if(manualModel&&!model.trim()){choicesOpen=true;error='Enter an exact model name first.';return}
    if(!current&&!provider?.enabled){choicesOpen=true;return}
    const mine=epoch;updating=true;error=''
    try{
      if(!current){const v=await createGafferConversation(scope,providerId,model,request?.signal);if(mine!==epoch)return;view=v}
      if(mine===epoch)contextOpen=true
    }catch(e){if(mine===epoch)error=message(e)}finally{if(mine===epoch)updating=false}
  }
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
      const receipt: PendingGafferSend = existing ?? {conversationId:answer.conversation.id,id:gafferMessageId(),revision:answer.conversation.revision,text}
      remember(key, receipt); admitted = true
      const turn = await sendGafferMessage({...answer.conversation,revision:receipt.revision}, receipt.id, text, request?.signal)
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

{#if !open}
  <button class="gaffer-launcher" use:keepLauncherClear bind:this={launcher} aria-label="Ask Gaffer" aria-haspopup="dialog" onclick={() => open = true}>
    <span class="launcher-portrait" aria-hidden="true"><GafferAvatar phase="idle" pose="tea" /></span><span>Ask Gaffer</span>
  </button>
{/if}
{#if open}
  <dialog class="gaffer-chat" class:expanded bind:this={panel} use:phoneShell={phone} aria-labelledby="gaffer-chat-title" aria-modal={phone ? 'true' : 'false'} oncancel={event => {event.preventDefault(); void close()}} onkeydown={event => { if (event.key === 'Escape') {event.preventDefault(); event.stopPropagation(); void close()} }}>
    <header class="chat-head" data-shell-head>
      <span class="portrait" aria-hidden="true"><GafferAvatar active={!!running} phase={running ? 'working' : last?.state === 'failed' ? 'error' : 'idle'} pose={running ? 'duty' : 'tea'} reaction={running ? 'scan' : ''} /></span>
      <div><p class="eyebrow">Your critical buddy</p><h2 id="gaffer-chat-title">Gaffer</h2><p class="brew">{running ? 'Brains ticking. Brew standing by.' : 'Pull up a chair.'}</p></div>
      <div class="head-actions"><button class="icon expand" aria-label={expanded ? 'Compact Gaffer panel' : 'Expand Gaffer panel'} onclick={() => expanded = !expanded}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d={expanded ? 'M4 10h6V4M20 14h-6v6M10 10 3 3m11 11 7 7' : 'M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7'} /></svg></button><button class="icon" data-shell-close aria-label="Close Gaffer" title="Close the panel; keep the current answer running" onclick={() => void close()}>×</button></div>
    </header>
    <div class="scope-bar">
      <label for="gaffer-chat-scope">Conversation</label>
      <select id="gaffer-chat-scope" value={mode === 'session' && $gafferSessionContext ? 'session' : 'general'} onchange={event => mode = event.currentTarget.value}>
        <option value="general">General Gaffer chat</option>
        {#if $gafferSessionContext}<option value="session">This session · {$gafferSessionContext.title}</option>{/if}
      </select>
      {#if scope}<p class="scope-title">{scopeTitle}</p>{/if}
      {#if scope && $gafferSessionContext}<p class="worker">Working agent: {toolName($gafferSessionContext.tool) || 'Agent'} · {$gafferSessionContext.model || 'Model not reported'}</p>{/if}
    </div>
    {#if unavailable}
      <div class="unavailable" data-shell-body><h3>This session uses a terminal.</h3><p>A session companion needs a supported structured chat. You can still talk to Gaffer in General chat.</p><button class="btn" onclick={() => mode = 'general'}>Open general chat</button></div>
    {:else}
      <div class="conversation-body" data-shell-body>
      <div class="identity"><span>{loading ? 'Reading Gaffer’s choice of brains…' : `Gaffer: ${provider?.name || 'Choose a provider'} · ${model || provider?.model || 'Choose a model'}`}</span><button class="quiet" aria-expanded={choicesOpen} onclick={() => choicesOpen = !choicesOpen}>Model</button></div>
      <div class="context-strip"><span class:attention={current?.context?.needsReview}>{current?.context?.needsReview?'Review context sharing':current?.context?.grant && (current.context.grant.status||current.context.grant.history||current.context.grant.text)?'Selected context shared':'Context off · typed messages only'}</span><button class="quiet" type="button" disabled={loading||sending||updating||dirty} onclick={()=>void contextChoices()}>Context</button></div>
      {#if choicesOpen && settings}
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
            {#if current}<button class="btn" disabled={!dirty || !provider?.enabled || (manualModel && !model.trim())}>Apply choice</button>{:else}<p class="note">Used when you send your first message.</p>{/if}
          </fieldset>
          <a href="/account/gaffer">Manage providers and keys →</a>
        </form>
      {/if}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrolling history must be reachable with a keyboard.) -->
      <div class="transcript" bind:this={transcript} data-shell-body onscroll={() => {if (transcript) stickToBottom = transcript.scrollHeight - transcript.scrollTop - transcript.clientHeight < 90}} tabindex="0" role="region" aria-label="Gaffer conversation messages">
        {#if loading}<p class="empty" role="status">Finding our place…</p>
        {:else if loadedScope === scope && !error && !current?.turns.length}<div class="welcome"><span class="welcome-mark" aria-hidden="true">“</span><h3>Reet. What’s on your mind?</h3><p>Talk it through, question the plan, or ask for a second pair of eyes.</p><p class="note">I can use what you tell me here. Live reet status, session history and follow-up tools are coming next.</p></div>{/if}
        {#if loadedScope === scope}{#each current?.turns ?? [] as turn (turn.id)}
          <article class="turn">
            <div class="user-message"><p class="speaker">You</p><p class="message-text">{turn.prompt}</p></div>
            <div class="gaffer-message"><div class="reply-head"><p class="speaker">Gaffer</p><span class="state" class:bad={turn.state === 'failed'}>{gafferTurnLabel(turn)}</span></div>
              {#if turn.answer}<p class="message-text">{turn.answer}</p>{:else if turn.state === 'running'}<p class="thinking" role="status">Putting the thinking cap on…</p>{/if}
              {#if turn.error}<p class="turn-error" role="status">{turn.error}</p>{/if}
              {#if turn.cut}<p class="turn-error">This reply reached its output limit and may be incomplete.</p>{/if}
              <div class="reply-foot"><span title={formatWhen(turn.startedAt, timeZone, dateFormat)}>{turn.providerName} · {turn.model}</span>{#if turn.answer}<CopyButton text={turn.answer} label="Copy Gaffer reply" />{/if}</div>
              {#if turn.state !== 'running'}<p class="usage">{turn.usageReported ? `${turn.inputTokens} input · ${turn.outputTokens} output tokens` : 'Usage not reported'}</p>{/if}
              {#if turn.context}<p class="usage">Retained context · read {formatWhen(turn.context.readAt,timeZone,dateFormat)}{turn.context.lastSeq?` · through event #${turn.context.lastSeq}`:''}{turn.context.truncated?' · trimmed or incomplete':''}</p>{/if}
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
        <label class="sr-only" for="gaffer-message">Message Gaffer</label>
        <textarea id="gaffer-message" bind:this={composer} value={drafts[scope] ?? ''} oninput={event => drafts[scope] = event.currentTarget.value} disabled={sending || loading} rows="3" placeholder="Talk it through with Gaffer…" onkeydown={event => {if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.isComposing) {event.preventDefault(); void send()}}}></textarea>
        <div class="compose-actions"><span class:bad={bytes > 8000}>{bytes > 7200 ? `${bytes.toLocaleString()} / 8,000 bytes` : dirty ? 'Apply your model choice first' : 'Ctrl / ⌘ Enter to send'}</span>{#if running}<button class="btn" type="button" disabled={cancelling || sending} onclick={() => void cancelAnswer()}>{cancelling ? 'Cancelling…' : 'Cancel answer'}</button>{:else}<button class="btn primary" disabled={blocked || !drafts[scope]?.trim() || bytes > 8000}>{sending ? 'Sending…' : 'Send message'}</button>{/if}</div>
      </form>
    {/if}
    <footer class="chat-foot"><p>Context is shared only as selected. Worker instructions and offline follow-up are coming next. Closing keeps the current answer running.</p><div><a href="/account/gaffer">Settings</a><button class="quiet" onclick={() => historyOpen = true}>Saved chats</button>{#if current}<button class="quiet remove" aria-label="Remove conversation" disabled={sending || updating || loading} onclick={() => confirming = true}>Remove chat</button>{/if}</div></footer>
  </dialog>
{/if}
{#if historyOpen}<GafferChatHistory {timeZone} {dateFormat} onerror={message} onclose={() => historyOpen = false} onremoved={c => { if (current?.conversation.id === c.id || pending?.conversationId === c.id) { remember(scope, null); drafts[scope] = ''; void load() } }} />{/if}
{#if confirming && current}<ConfirmDialog title="Remove this Gaffer conversation?" body="This deletes its messages and stops its current answer. Your working agent keeps running. Provider credits already used cannot be recovered." confirmLabel="Remove conversation" danger oncancel={() => confirming = false} onconfirm={() => void removeConversation()} />{/if}

{#if contextOpen && current}<WorkspaceDialog title="What Gaffer can read" onclose={()=>contextOpen=false}><ContextSharing {timeZone} {dateFormat} path={`/api/v2/gaffer/conversations/${encodeURIComponent(current.conversation.id)}/context`} gaffer onchanged={contextSaved}/></WorkspaceDialog>{/if}

<style>
  .gaffer-launcher { position: fixed; z-index: 18; right: 24px; bottom: var(--launcher-clearance, calc(var(--bottom-chrome, 0px) + 20px)); min-height: 52px; display: flex; align-items: center; gap: 8px; padding: 4px 16px 4px 8px; border: 1px solid var(--line-strong); border-radius: 999px; background: var(--card-raised); color: var(--ink); box-shadow: 0 5px 22px var(--scrim); font: 600 14px var(--sans); cursor: pointer; }
  .launcher-portrait { width: 38px; height: 38px; }
  .gaffer-launcher:hover { border-color: var(--accent); }
  .gaffer-chat { position: fixed; z-index: 30; inset: auto 20px calc(var(--bottom-chrome, 0px) + 18px) auto; margin: 0; width: 450px; max-width: calc(100vw - 32px); height: min(770px, calc(100dvh - var(--bottom-chrome, 0px) - 48px)); max-height: calc(100dvh - 32px); padding: 0; border: 1px solid var(--line-strong); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 16px 64px var(--scrim); overflow: hidden; }
  .gaffer-chat[open] { display: flex; flex-direction: column; }
  .gaffer-chat.expanded { width: 660px; }
  .gaffer-chat::backdrop { background: var(--scrim); }
  .chat-head { display: flex; align-items: center; gap: 12px; padding: 16px 18px; background: linear-gradient(110deg,var(--tint),var(--card)); border-bottom: 1px solid var(--line); flex: none; }
  .portrait { width: 52px; height: 52px; flex: none; }
  .eyebrow { font: 400 14px var(--mono); color: var(--muted); margin: 0 0 3px; }
  h2 { font: 600 28px/1.1 var(--serif); margin: 0; }
  h3 { font: 600 22px/1.2 var(--serif); margin: 0 0 10px; }
  .brew { margin: 4px 0 0; color: var(--accent-text); font-size: 14px; }
  .head-actions { margin-left: auto; display: flex; }
  .icon { min-width: 40px; min-height: 44px; background: transparent; color: var(--ink); border: 0; border-radius: 8px; font-size: 25px; cursor: pointer; }
  .icon:hover, .quiet:hover { background: var(--tint); }
  .scope-bar { flex: none; padding: 12px 18px 10px; border-bottom: 1px solid var(--line); }
  .scope-bar label { font-size: 14px; color: var(--muted); display: block; margin-bottom: 6px; }
  select, input, textarea { width: 100%; box-sizing: border-box; min-height: 42px; border: 1px solid var(--line-strong); border-radius: 9px; background: var(--bg); color: var(--ink); padding: 9px 10px; font: 400 14px/1.45 var(--sans); min-width: 0; }
  .scope-title { margin: 6px 0 0; font-weight: 600; font-size: 14px; overflow-wrap: anywhere; }
  .worker { margin: 3px 0 0; color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .conversation-body { display: flex; flex-direction: column; flex: 1; min-height: 0; overflow: auto; overscroll-behavior: contain; }
  .context-strip { display:flex;align-items:center;justify-content:space-between;gap:8px;padding:2px 18px;border-bottom:1px solid var(--line-soft);font-size:14px;color:var(--muted);flex:none }.attention { color:var(--bad) }
  .identity { display: flex; align-items: center; gap: 8px; padding: 6px 18px; border-bottom: 1px solid var(--line-soft); font-size: 14px; color: var(--muted); flex: none; }
  .identity span { min-width: 0; overflow-wrap: anywhere; flex: 1; }
  .quiet { border: 0; background: transparent; color: var(--accent-text); padding: 8px; min-height: 36px; font: 600 14px var(--sans); cursor: pointer; border-radius: 7px; }
  .choices { flex: 0 1 auto; overflow: auto; padding: 12px 18px; border-bottom: 1px solid var(--line); background: var(--bg-sunk); max-height: 52%; }
  fieldset { border: 0; padding: 0; margin: 0 0 8px; display: grid; gap: 8px; min-width: 0; }
  .choices label { font-size: 14px; display: grid; gap: 6px; }
  .model-head { display: flex; align-items: center; justify-content: space-between; }
  .choices a { font-size: 14px; }
  .note { font-size: 14px; color: var(--muted); line-height: 1.5; margin: 4px 0; }
  .transcript { overflow: auto; overscroll-behavior: contain; flex: 1 1 180px; min-height: 80px; padding: 18px; }
  .welcome { padding: 12px 4px; }
  .welcome-mark { font: 72px/.7 var(--serif); color: var(--accent-text); display: block; margin: 18px 0 10px; }
  .welcome > p { font-size: 14px; line-height: 1.6; }
  .welcome .note { margin-top: 16px; }
  .turn + .turn { margin-top: 24px; }
  .user-message { background: var(--tint); border: 1px solid var(--line); border-radius: 12px 12px 3px 12px; padding: 12px 14px; margin: 0 0 16px 24px; }
  .speaker { margin: 0 0 8px; font-size: 14px; font-weight: 600; }
  .message-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.65; }
  .reply-head { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 6px; }
  .state { color: var(--muted); font-size: 14px; }
  .reply-foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 12px; color: var(--muted); font-size: 14px; }
  .reply-foot span { overflow-wrap: anywhere; }
  .usage { font-size: 14px; color: var(--muted); margin: 4px 0 0; }
  .turn-error { border-left: 2px solid var(--accent); padding-left: 10px; font-size: 14px; color: var(--muted); line-height: 1.5; }
  .thinking { font-size: 14px; color: var(--muted); }
  .feedback { flex: none; margin: 8px 18px; padding: 10px 12px; border: 1px solid var(--line-strong); border-radius: 9px; background: var(--bg-sunk); font-size: 14px; overflow-wrap: anywhere; }
  .feedback p { margin: 0 0 6px; line-height: 1.4; }
  .bad { color: var(--bad); }
  .notice { font-size: 14px; margin: 6px 18px; color: var(--accent-text); }
  .setup, .unavailable { padding: 18px; font-size: 14px; line-height: 1.5; }
  .unavailable { flex: 1; overflow: auto; }
  .composer { padding: 12px 18px; border-top: 1px solid var(--line); flex: none; }
  textarea { resize: vertical; max-height: 150px; min-height: 68px; }
  .compose-actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 8px; }
  .compose-actions > span { font-size: 14px; color: var(--muted); }
  .compose-actions > .bad { color: var(--bad); }
  .compose-actions .btn { flex: none; min-height: 42px; }
  .chat-foot { padding: 8px 18px 10px; background: var(--bg-sunk); border-top: 1px solid var(--line); flex: none; }
  .chat-foot p { font-size: 14px; color: var(--muted); line-height: 1.4; margin: 0; }
  .chat-foot > div { display: flex; flex-wrap: wrap; gap: 2px 4px; align-items: center; justify-content: space-between; font-size: 14px; }
  .remove { color: var(--bad); }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; border: 0; }
  button:focus-visible, select:focus-visible, input:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (max-width: 559.98px) {
    .gaffer-launcher { right: 12px; }
    .gaffer-chat { inset: 0; width: 100%; max-width: 100%; height: 100dvh; border-radius: 0; }
    .expand { display: none; }
    .chat-head { padding: 10px 14px; }
    .scope-bar, .identity { padding-inline: 14px; }
    .transcript { padding: 14px; }
    .composer { padding: 10px 14px; }
    .chat-foot { padding-inline: 14px; }
    .choices { padding: 10px 14px; }
    .btn, .quiet, select, input { min-height: 44px; }
    .compose-actions .btn { min-height: 44px; }
    textarea { font-size: 16px; }
  }
  @media (max-height: 600px) {
    .brew, .eyebrow, .chat-foot > p, .scope-title { display: none; }
    .chat-head { padding-block: 4px; }
    .portrait { width: 38px; height: 38px; }
    .scope-bar { padding-block: 4px; }
    .composer { padding-block: 6px; }
    textarea { min-height: 44px; max-height: 90px; }
  }
  :global(.gaffer-chat[data-shell-short]) .eyebrow,
  :global(.gaffer-chat[data-shell-short]) .brew,
  :global(.gaffer-chat[data-shell-short]) .scope-title,
  :global(.gaffer-chat[data-shell-short]) .worker,
  :global(.gaffer-chat[data-shell-short]) .chat-foot { display: none; }
  :global(.gaffer-chat[data-shell-short]) .scope-bar label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); }
  :global(.gaffer-chat[data-shell-short]) .portrait { width: 36px; height: 36px; }
  :global(.gaffer-chat[data-shell-short]) .scope-bar { padding-block: 4px; }
  :global(.gaffer-chat[data-shell-short]) .composer { margin-top: 0; padding: 6px 12px; }
  :global(.gaffer-chat[data-shell-short]) textarea { height: 44px; min-height: 44px; max-height: 60px; }
</style>
