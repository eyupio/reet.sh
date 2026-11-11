<script lang="ts">
  // Give Gaffer an outcome. Gaffer proposes the sessions, the person edits and approves the plan, and only then does this
  // browser start each session through the ordinary launch and put it in Gaffer's care through the reviewed setup.
  // Nothing here starts on the model's say-so: a proposed session is text until the person presses Approve.
  import { onMount } from 'svelte'
  import { ApiError, createGafferConversation, getGafferConversation, getGafferGoal, getGafferSettings, planGafferOutcome, setupGafferGoal, type GafferProvider } from './api'
  import { chatToolName, createTerminal, launchUpload, listFolderProjects, structuredAgent, terminalMachines, type FolderProject, type TerminalMachine } from './terminal/api'
  import { imagePrompt } from './terminal/upload'
  import ImageAttachment from './ImageAttachment.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { readFloorDefaults, writeFloorDefaults } from './gafferFloor'
  import { BUILT_IN_GOAL_PROMPTS } from './gafferPrompts'
  import { MAX_OUTCOME_SESSIONS, objectiveBlocker, outcomeGoal, outcomeSummary, planBlocker, type OutcomeResult, type PlanSession } from './gafferOutcome'

  let { room, userId = 0, onclose, onstarted }: { room: number; userId?: number; onclose: () => void; onstarted: () => void | Promise<void> } = $props()
  const formId = $props.id()
  const CHAT_TOOLS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const life = new AbortController()
  let step = $state<'objective' | 'plan' | 'results'>('objective')
  let loading = $state(true), loadError = $state('')
  let provider = $state<GafferProvider | null>(null), projects = $state<FolderProject[]>([]), machines = $state<TerminalMachine[]>([])
  let objective = $state(''), projectId = $state(''), tool = $state(''), wanted = $state(3)
  let planning = $state(false), planError = $state(''), summary = $state(''), sessions = $state<PlanSession[]>([])
  let maxReviews = $state(12), hours = $state(24)
  let busy = $state(false), results = $state<OutcomeResult[]>([])
  let image = $state<File | null>(null), imageUploading = $state(false), imageError = $state('')
  let counter = 0
  const key = () => `plan-${++counter}`
  function clearImage() { image = null; imageError = '' }

  const project = $derived(projects.find(item => item.id === projectId) ?? null)
  const machine = $derived(project ? machines.find(item => item.agentId === project.host) ?? null : null)
  const tools = $derived(machine ? CHAT_TOOLS.filter(id => structuredAgent(machine.capabilities, id)) : [])
  const cap = $derived(Math.max(1, Math.min(MAX_OUTCOME_SESSIONS, room)))
  const blocker = $derived(loading ? '' : !provider ? 'Choose a default Gaffer provider in Gaffer settings first.' : objectiveBlocker({ objective, project: !!project, machineReady: !!machine?.ready, agent: !!tool && tools.includes(tool), room }))
  const approveBlocker = $derived(planBlocker(sessions, room))
  const done = $derived(results.length > 0 && !busy)
  $effect(() => { void image; imageError = '' })

  $effect(() => { if (tools.length && !tools.includes(tool)) tool = tools[0]; if (!tools.length) tool = '' })
  $effect(() => { if (wanted > cap) wanted = cap })

  onMount(() => {
    const saved = readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text)
    maxReviews = saved.maxReviews; hours = saved.hours
    void Promise.all([getGafferSettings(life.signal), listFolderProjects(life.signal), terminalMachines(life.signal)]).then(([settings, folders, found]) => {
      provider = settings.available ? settings.providers.find(item => item.isDefault && item.enabled) ?? null : null
      projects = folders; machines = found
      if (folders.length === 1) projectId = folders[0].id
    }).catch(e => { if (!life.signal.aborted) loadError = e instanceof Error ? e.message : 'Projects and machines could not be read.' })
      .finally(() => { loading = false })
    return () => life.abort()
  })

  async function ask() {
    if (planning || blocker || !provider || !project) return
    planning = true; planError = ''
    try {
      const plan = await planGafferOutcome(provider, { objective: objective.trim(), project: project.name, sessions: wanted }, life.signal)
      summary = plan.summary
      sessions = plan.sessions.map(item => ({ key: key(), title: item.title, prompt: item.prompt }))
      step = 'plan'
    } catch (e) { if (!life.signal.aborted) planError = e instanceof Error ? e.message : 'Gaffer could not plan this outcome.' }
    finally { planning = false }
  }
  // Without a tool-capable model, or by choice, the person writes the plan; the rest of the flow is the same.
  function writeOwn() {
    if (blocker) return
    summary = ''; sessions = [{ key: key(), title: '', prompt: '' }]; planError = ''; step = 'plan'
  }
  const addSession = () => { sessions = [...sessions, { key: key(), title: '', prompt: '' }] }
  const removeSession = (id: string) => { sessions = sessions.filter(item => item.key !== id) }
  const requestId = () => (globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}-outcome`).replace(/[^A-Za-z0-9_-]/g, '')
  const wait = (ms: number) => new Promise<void>((resolve, reject) => { const timer = setTimeout(resolve, ms); life.signal.addEventListener('abort', () => { clearTimeout(timer); reject(life.signal.reason) }, { once: true }) })
  const mark = (id: string, change: Partial<OutcomeResult>) => { results = results.map(item => item.key === id ? { ...item, ...change } : item) }

  /** Puts a started session in Gaffer's care. A new chat needs a moment before its message channel is live, so this waits for it. */
  async function follow(sessionId: string, title: string): Promise<void> {
    let found = await getGafferConversation(sessionId, life.signal).catch(e => { if (e instanceof ApiError && e.status === 404) return null; throw e })
    if (!found) found = await createGafferConversation(sessionId, provider!.id, '', life.signal)
    const id = found.conversation.id
    for (let attempt = 0; ; attempt++) {
      const view = await getGafferGoal(id, life.signal)
      if (view.goal) throw new Error('This session already has a goal.')
      if (view.setupAllowed ?? view.allowed) {
        await setupGafferGoal(id, { ...view.pins, revision: 0, conversationRevision: view.conversationRevision ?? 0, confirmed: true, action: 'start', text: outcomeGoal(objective, title), maxReviews, hours }, life.signal)
        return
      }
      if (attempt >= 14) throw new Error(view.reason || 'Its chat was not ready for follow-up in time.')
      await wait(2000)
    }
  }

  async function approve() {
    if (busy || approveBlocker || !provider || !project || !tool) return
    busy = true; imageError = ''
    let reference = ''
    if (image) {
      imageUploading = true
      try { reference = (await launchUpload(project.host, image, life.signal)).reference }
      catch (e) {
        if (!life.signal.aborted) imageError = e instanceof Error ? e.message : 'The image could not be uploaded.'
        imageUploading = false; busy = false; return
      }
      finally { imageUploading = false }
    }
    step = 'results'
    writeFloorDefaults(userId, { ...readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text), maxReviews, hours })
    const chosen = sessions.map(item => ({ ...item, title: item.title.trim(), prompt: item.prompt.trim() }))
    results = chosen.map(item => ({ key: item.key, title: item.title, step: 'waiting', note: 'Waiting its turn.' }))
    let any = false
    // One at a time: each launch is the ordinary reviewed create, with its own worktree in a git project, and a failure names its session.
    for (const item of chosen) {
      if (life.signal.aborted) return
      mark(item.key, { step: 'starting', note: 'Starting the session…' })
      let sessionId = ''
      try {
        const prompt = reference ? imagePrompt(item.prompt, [reference]) : item.prompt
        const created = await createTerminal(project.host, requestId(), item.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(60000)]), { tool, dir: project.dir, projectId: project.id, prompt, structured: true })
        sessionId = created.id; any = true
        mark(item.key, { note: 'Started. Putting it in Gaffer’s care…', sessionId, agentId: created.agentId })
        await follow(sessionId, item.title)
        mark(item.key, { step: 'following', note: 'Started and followed. Gaffer waits for its first progress.' })
      } catch (e) {
        if (life.signal.aborted) return
        const reason = e instanceof Error ? e.message : 'The request failed.'
        mark(item.key, sessionId ? { step: 'unfollowed', note: `Started, but not followed: ${reason} It is under Not followed on the floor.` } : { step: 'failed', note: `Not started: ${reason}` })
      }
    }
    clearImage()
    busy = false
    if (any) await onstarted()
  }
  const stepLabel = (item: OutcomeResult) => item.step === 'following' ? 'Following' : item.step === 'unfollowed' ? 'Started, not followed' : item.step === 'failed' ? 'Not started' : item.step === 'starting' ? 'Starting' : 'Waiting'
</script>

<WorkspaceDialog title="Give Gaffer an outcome" {onclose}>
  <div class="outcome">
    {#if step === 'objective'}
      <p>Say what you want done. Gaffer proposes the sessions to start; nothing starts until you approve the plan.</p>
      {#if loading}<p role="status">Reading your projects…</p>
      {:else if loadError}<p class="warning" role="alert">{loadError}</p>
      {:else}
        <form onsubmit={event => { event.preventDefault(); void ask() }}>
          <label for={`${formId}-objective`}>Outcome</label>
          <textarea id={`${formId}-objective`} bind:value={objective} rows="4" maxlength="4000" placeholder="Ship CSV export for billing: an API endpoint, a button on the billing page, and tests." disabled={planning}></textarea>
          <ImageAttachment bind:file={image} label="Attach an image to the outcome" disabled={planning} busy={imageUploading} error={imageError} />
          <div class="pair">
            <label>Project<select aria-label="Project" bind:value={projectId} disabled={planning}>
              <option value="">Choose a project…</option>
              {#each projects as item (item.id)}<option value={item.id}>{item.name}{item.machine ? ` · ${item.machine}` : ''}</option>{/each}
            </select></label>
            <label>Agent<select aria-label="Agent" bind:value={tool} disabled={planning || !tools.length}>
              {#if !tools.length}<option value="">{project ? 'None available' : 'Choose a project first'}</option>{/if}
              {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
            </select></label>
            <label>Sessions<select aria-label="Sessions" bind:value={wanted} disabled={planning}>
              {#each Array.from({ length: cap }, (_, n) => n + 1) as n (n)}<option value={n}>Up to {n}</option>{/each}
            </select></label>
          </div>
          {#if !projects.length}<p class="hint">No projects yet. <a href="/">Add a project on Home</a> so sessions have a folder to work in.</p>{/if}
          {#if project && !project.isRepo}<p class="hint">This folder is not a git repository, so several sessions would share the same files. One session is safest.</p>{/if}
          {#if provider}<p class="hint">Gaffer plans with {provider.name} · {provider.model || 'model not chosen'}. It is sent only what you type here and the project’s name, and may use provider credits.</p>{/if}
          {#if blocker}<p class="warning" role="status">{blocker}{#if !provider}{' '}<a href="/account/gaffer">Open Gaffer settings</a>{/if}</p>{/if}
          {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
          <div class="actions"><button class="btn primary" type="submit" disabled={planning || !!blocker}>{planning ? 'Gaffer is planning…' : 'Ask Gaffer for a plan'}</button><button class="btn quiet" type="button" disabled={planning || !!blocker} onclick={writeOwn}>Write the plan myself</button></div>
        </form>
      {/if}
    {:else if step === 'plan'}
      <p class="objective"><span class="eyebrow">Outcome</span>{objective.trim()}</p>
      {#if summary}<div class="summary"><p class="eyebrow">Gaffer’s plan</p><p>{summary}</p></div>{/if}
      <p class="hint">{project?.name} · {chatToolName(tool)}. Edit anything; each session starts with exactly the prompt shown{project?.isRepo ? ', in its own worktree' : ''}.</p>
      <ImageAttachment bind:file={image} label="Attach an image to the outcome" disabled={busy} busy={imageUploading} error={imageError} />
      <ol class="plan" aria-label="Planned sessions">
        {#each sessions as item, index (item.key)}<li>
          <div class="plan-head"><label for={`${formId}-${item.key}-title`}>Session {index + 1}</label><button class="link" type="button" aria-label={`Remove session ${index + 1}`} onclick={() => removeSession(item.key)}>Remove</button></div>
          <input id={`${formId}-${item.key}-title`} aria-label={`Session ${index + 1} title`} bind:value={item.title} maxlength="80" placeholder="Short title"/>
          <textarea aria-label={`Session ${index + 1} prompt`} bind:value={item.prompt} rows="5" placeholder="What this session should build, how to check it, and to raise a pull request. Do not merge."></textarea>
        </li>{/each}
      </ol>
      {#if sessions.length < cap}<button class="btn quiet add" type="button" onclick={addSession}>Add a session</button>{/if}
      <div class="access" aria-label="Outcome access review"><p>Approving starts {sessions.length === 1 ? 'this session' : `these ${sessions.length} sessions`} on {project?.machine || 'the project’s machine'} with ordinary approvals (no YOLO). Gaffer may then read each one’s recent conversation, including code or logs in it, and queue instructions for its working model. If the provider does not yet allow session data, this enables it. Gaffer cannot merge, run commands or answer approvals.</p></div>
      <details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours per session <span>Change limits</span></summary>
        <div class="pair two"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours}/></label></div>
      </details>
      {#if approveBlocker}<p class="warning" role="status">{approveBlocker}</p>{/if}
      <div class="actions"><button class="btn primary" type="button" disabled={busy || !!approveBlocker || !(maxReviews >= 1 && maxReviews <= 48 && hours >= 1 && hours <= 168)} onclick={() => void approve()}>Approve and start {sessions.length} {sessions.length === 1 ? 'session' : 'sessions'}</button><button class="btn quiet" type="button" onclick={() => step = 'objective'}>Back</button></div>
    {:else}
      <p class="objective"><span class="eyebrow">Outcome</span>{objective.trim()}</p>
      <p role="status" class="result-line">{done ? outcomeSummary(results) : 'Starting the approved sessions. Keep this open until each one is settled.'}</p>
      <ul class="results" aria-label="Outcome sessions">
        {#each results as item (item.key)}<li class:failed={item.step === 'failed' || item.step === 'unfollowed'}>
          <strong>{item.title}</strong><span>{stepLabel(item)} · {item.note}</span>
          {#if item.sessionId && item.agentId}<a href={`/sessions?agent=${encodeURIComponent(item.agentId)}&session=${encodeURIComponent(item.sessionId)}&gaffer=1`}>Open session</a>{/if}
        </li>{/each}
      </ul>
      {#if done}<p class="hint">Each session has its own goal naming this outcome, and its own budget. The floor groups them under the outcome; only you mark a goal complete.</p>
        <div class="actions"><button class="btn primary" type="button" onclick={onclose}>Back to the floor</button></div>{/if}
    {/if}
  </div>
</WorkspaceDialog>

<style>
  .outcome{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted)}.warning{color:var(--bad)}
  form{display:grid;gap:12px;min-width:0}label{font-weight:600;display:grid;gap:6px;min-width:0}
  textarea,input,select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .pair{display:grid;grid-template-columns:2fr 1.4fr 1fr;gap:12px}.pair.two{grid-template-columns:1fr 1fr;padding-top:8px}
  .eyebrow{display:block;font-size:0.875rem;color:var(--muted);margin-bottom:2px}.objective{white-space:pre-wrap}
  .summary,.access{border-left:2px solid var(--line-strong);padding-left:12px}
  .plan{list-style:none;margin:0;padding:0;display:grid;gap:12px}.plan li{display:grid;gap:8px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}
  .plan-head{display:flex;align-items:center;justify-content:space-between;gap:12px}.link{border:0;background:none;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer;min-height:44px;padding:0 4px}
  .add{justify-self:start}
  .limit-editor summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;font-variant-numeric:tabular-nums}.limit-editor summary span{color:var(--accent-text);margin-left:8px}
  .result-line{font-weight:600}
  .results{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.results li{display:grid;gap:2px;padding:10px 14px;min-width:0;overflow-wrap:anywhere}.results li+li{border-top:1px solid var(--line)}.results span{font-size:0.875rem;color:var(--muted)}.results li.failed span{color:var(--bad)}.results a{color:var(--accent-text);font-weight:600;min-height:44px;display:inline-flex;align-items:center;justify-self:start}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,a:focus-visible,summary:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.pair,.pair.two{grid-template-columns:1fr}}
</style>
