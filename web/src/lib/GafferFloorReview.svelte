<script lang="ts">
  // One review for several sessions. Each session still gets its own conversation, access and goal through the same
  // reviewed setup a single session uses; this only spares the person from confirming the same choices N times.
  import { onMount } from 'svelte'
  import { ApiError, createGafferConversation, getGafferConversation, getGafferGoal, getGafferSettings, setupGafferGoal, type GafferProvider } from './api'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { BUILT_IN_GOAL_PROMPTS, goalPromptTitle, readGoalPrompts, type GoalPrompt } from './gafferPrompts'
  import { wholeBetween } from './gafferOutcome'
  import { floorBlocker, floorSummary, readFloorDefaults, writeFloorDefaults, MAX_GOAL_BYTES, type FloorOutcome, type FloorSession } from './gafferFloor'
  import { toolName } from './agentLaunch'

  let { sessions, room, userId = 0, auto = false, onclose, onstarted }: {
    /** The person has pre-approved looking after sessions: start with their usual goal and limits, with no form to fill. */
    auto?: boolean;
    sessions: FloorSession[]; room: number; userId?: number; onclose: () => void; onstarted: () => void | Promise<void>
  } = $props()
  const formId = $props.id()
  const starting = BUILT_IN_GOAL_PROMPTS[0].text
  let provider = $state<GafferProvider | null>(null), loading = $state(true), error = $state('')
  let text = $state(starting), maxReviews = $state(12), hours = $state(24)
  let userPrompts = $state<GoalPrompt[]>([])
  let busy = $state(false), progress = $state(''), outcomes = $state<FloorOutcome[] | null>(null)
  const life = new AbortController()
  const bytes = $derived(new TextEncoder().encode(text).length)
  const blocker = $derived(loading ? '' : floorBlocker({ chosen: sessions.length, room, text, provider: !!provider }))
  const many = $derived(sessions.length !== 1)
  const limitsOk = $derived(wholeBetween(maxReviews, 1, 48) && wholeBetween(hours, 1, 168))

  onMount(() => {
    const saved = readFloorDefaults(userId, starting)
    text = saved.text; maxReviews = saved.maxReviews; hours = saved.hours
    userPrompts = readGoalPrompts(userId)
    void getGafferSettings(life.signal).then(settings => {
      provider = settings.available ? settings.providers.find(item => item.isDefault && item.enabled) ?? null : null
    }).catch(e => { if (!life.signal.aborted) error = e instanceof Error ? e.message : 'Gaffer settings could not be read.' })
      .finally(() => { loading = false; if (auto && provider && !floorBlocker({ chosen: sessions.length, room, text, provider: true }) && limitsOk) void start() })
    return () => life.abort()
  })

  function usePrompt(event: Event) {
    const select = event.currentTarget as HTMLSelectElement
    const chosen = BUILT_IN_GOAL_PROMPTS.find(item => `builtin:${item.id}` === select.value)?.text ?? userPrompts.find(item => `user:${item.id}` === select.value)?.text
    select.value = ''
    if (chosen) text = chosen
  }

  async function startOne(session: FloorSession, providerId: string): Promise<void> {
    let found = await getGafferConversation(session.id, life.signal).catch(e => { if (e instanceof ApiError && e.status === 404) return null; throw e })
    if (!found) found = await createGafferConversation(session.id, providerId, '', life.signal)
    const id = found.conversation.id
    const view = await getGafferGoal(id, life.signal)
    if (view.goal) throw new Error('This session already has a goal. Review it on its card.')
    // The goal view's own reason describes missing grants, which this setup grants; what blocks it here is the provider or the chat.
    if (!(view.setupAllowed ?? view.allowed)) throw new Error('This session needs an enabled provider and a structured worker with live message controls.')
    await setupGafferGoal(id, { ...view.pins, revision: 0, conversationRevision: view.conversationRevision ?? 0, confirmed: true, action: 'start', text: text.trim(), maxReviews, hours }, life.signal)
  }

  async function start() {
    if (busy || blocker || !limitsOk || !provider) return
    busy = true; error = ''
    writeFloorDefaults(userId, { text, maxReviews, hours })
    const done: FloorOutcome[] = []
    // One at a time: the controller admits each goal against the account's limit, and a failure names its session.
    for (const [index, session] of sessions.entries()) {
      if (life.signal.aborted) return
      progress = `Starting ${index + 1} of ${sessions.length}: ${session.title}`
      try { await startOne(session, provider.id); done.push({ id: session.id, title: session.title, ok: true, note: 'Gaffer waits for new worker progress.' }) }
      catch (e) {
        if (life.signal.aborted) return
        done.push({ id: session.id, title: session.title, ok: false, note: e instanceof Error ? e.message : 'Follow-up was not started.' })
      }
    }
    progress = ''; outcomes = done; busy = false
    if (done.some(item => item.ok)) await onstarted()
  }
</script>

<WorkspaceDialog title="Look after these sessions" {onclose}>
  <div class="review">
    {#if outcomes}
      <p role="status" class="summary">{floorSummary(outcomes)}</p>
      <ul class="outcomes" aria-label="Follow-up results">
        {#each outcomes as item (item.id)}<li class:failed={!item.ok}><strong>{item.title}</strong><span>{item.ok ? 'Following' : 'Not started'} · {item.note}</span></li>{/each}
      </ul>
      <p class="hint">Each session has its own goal, budget and conversation. Pause, change or stop any of them from its card.</p>
      <div class="actions"><button class="btn primary" type="button" onclick={onclose}>Back to the floor</button></div>
    {:else}
      <p>One review for {sessions.length} {many ? 'sessions' : 'session'}. Each gets its own goal, budget and Gaffer conversation; the working model in each keeps doing the coding and PR work under its own approvals.</p>
      <ul class="chosen" aria-label="Sessions to look after">
        {#each sessions as item (item.id)}<li><strong>{item.title}</strong><span>{toolName(item.tool ?? '') || 'Agent'} · {item.dir || 'Directory not reported'}</span></li>{/each}
      </ul>
      {#if loading}<p role="status">Reading Gaffer settings…</p>
      {:else if provider}<p class="hint">Gaffer: {provider.name} · {provider.model || 'Model not chosen'}. A session that already has a Gaffer chat keeps the provider and model chosen there.</p>
      {:else if !error}<p class="warning">No default Gaffer provider is enabled. <a href="/account/gaffer">Open Gaffer settings</a></p>{/if}
      <div class="access" aria-label="Session access review"><p>Starting lets Gaffer read each listed session’s recent conversation, including any code or logs it contains, and queue instructions for its working model. If the provider does not yet allow session data, this enables it for that provider. Sessions not listed here are not shared.</p></div>
      <form onsubmit={event => { event.preventDefault(); void start() }}>
        <div class="goal-head"><label for={`${formId}-goal`}>Goal for {many ? 'each session' : 'this session'}</label>
          <select aria-label="Use a prompt" disabled={busy} onchange={usePrompt}>
            <option value="">Use a prompt…</option>
            <optgroup label="reet prompts">{#each BUILT_IN_GOAL_PROMPTS as prompt (prompt.id)}<option value={`builtin:${prompt.id}`}>{prompt.title}</option>{/each}</optgroup>
            {#if userPrompts.length}<optgroup label="Your prompts">{#each userPrompts as prompt (prompt.id)}<option value={`user:${prompt.id}`}>{goalPromptTitle(prompt.text, 60)}</option>{/each}</optgroup>{/if}
          </select>
        </div>
        <textarea id={`${formId}-goal`} bind:value={text} rows="4" maxlength="2000" disabled={busy}></textarea>
        <details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours per session <span>Change limits</span></summary>
          <div class="limits"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews} required disabled={busy}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours} required disabled={busy}/></label></div>
        </details>
        <p class="hint">Up to {maxReviews * sessions.length} Gaffer reviews in all, and reviews may use provider credits. This goal and these limits are kept in this browser as your starting point next time.</p>
        {#if bytes > MAX_GOAL_BYTES}<p class="warning">Keep the goal within 2,000 UTF-8 bytes.</p>{/if}
        {#if blocker}<p class="warning" role="status">{blocker}</p>{:else if !limitsOk}<p class="warning" role="status">Choose a whole number of reviews from 1 to 48 and hours from 1 to 168.</p>{/if}
        {#if error}<p class="warning" role="alert">{error}</p>{/if}
        {#if progress}<p role="status">{progress}</p>{/if}
        <div class="actions"><button class="btn primary" type="submit" disabled={busy || loading || !!blocker || !limitsOk}>{busy ? 'Starting…' : `Start follow-up for ${sessions.length} ${many ? 'sessions' : 'session'}`}</button><button class="btn quiet" type="button" disabled={busy} onclick={onclose}>Cancel</button></div>
      </form>
    {/if}
  </div>
</WorkspaceDialog>

<style>
  .review{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted);font-variant-numeric:tabular-nums}.warning{color:var(--bad)}.summary{font-weight:600}
  ul{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}li{display:grid;gap:2px;padding:10px 14px;min-width:0;overflow-wrap:anywhere}li+li{border-top:1px solid var(--line)}li span{font-size:0.875rem;color:var(--muted)}li.failed span{color:var(--bad)}
  .access{border-left:2px solid var(--line-strong);padding-left:12px}
  form{display:grid;gap:12px;min-width:0}.goal-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}label{font-weight:600}
  textarea,input,select{font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;max-width:100%;min-height:44px;box-sizing:border-box}textarea{width:100%;resize:vertical}
  .limit-editor summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;font-variant-numeric:tabular-nums}.limit-editor summary span{color:var(--accent-text);margin-left:8px}
  .limits{display:grid;grid-template-columns:1fr 1fr;gap:12px;padding-top:8px}.limits label{display:grid;gap:6px}.limits input{width:100%}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,a:focus-visible,summary:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.limits{grid-template-columns:1fr}}
</style>
