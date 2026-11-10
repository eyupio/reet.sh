<script lang="ts">
  // Give Gaffer an outcome. Gaffer proposes the sessions, the person edits and approves the plan, and only then does the
  // controller start each session through the ordinary launch and put it in Gaffer's care. This dialog only watches:
  // the work carries on if it is closed. Nothing starts on the model's say-so: a proposed session is text until Approve.
  import { onMount } from 'svelte'
  import { createGafferOutcome, getGafferOutcome, getGafferSettings, planGafferOutcome, stopGafferOutcome, type GafferOutcome, type GafferProvider } from './api'
  import { chatToolName, listFolderProjects, structuredAgent, terminalMachines, type FolderProject, type TerminalMachine } from './terminal/api'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { readFloorDefaults, writeFloorDefaults } from './gafferFloor'
  import { BUILT_IN_GOAL_PROMPTS } from './gafferPrompts'
  import { MAX_OUTCOME_SESSIONS, cleanPrompt, objectiveBlocker, outcomeSummary, planBlocker, startingProgress, wholeBetween, type PlanSession } from './gafferOutcome'

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
  let model = $state(''), trust = $state(false)
  let busy = $state(false), startError = $state(''), record = $state<GafferOutcome | null>(null), watchError = $state('')
  let counter = 0
  const key = () => `plan-${++counter}`

  const project = $derived(projects.find(item => item.id === projectId) ?? null)
  const machine = $derived(project ? machines.find(item => item.agentId === project.host) ?? null : null)
  const tools = $derived(machine ? CHAT_TOOLS.filter(id => structuredAgent(machine.capabilities, id)) : [])
  const cap = $derived(Math.max(1, Math.min(MAX_OUTCOME_SESSIONS, room)))
  const blocker = $derived(loading ? '' : !provider ? 'Choose a default Gaffer provider in Gaffer settings first.' : objectiveBlocker({ objective, project: !!project, machineReady: !!machine?.ready, agent: !!tool && tools.includes(tool), room }))
  const approveBlocker = $derived(planBlocker(sessions, room))
  const needsTrust = $derived(tool === 'opencode' || tool === 'antigravity')
  const limitsOk = $derived(wholeBetween(maxReviews, 1, 48) && wholeBetween(hours, 1, 168))
  const done = $derived(!!record && record.state !== 'starting')

  $effect(() => { if (tools.length && !tools.includes(tool)) tool = tools[0]; if (!tools.length) tool = '' })
  $effect(() => { if (wanted > cap) wanted = cap })
  $effect(() => { if (!needsTrust) trust = false })

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
  // The controller owns the start from here. This only reads its record until every session has an answer.
  let watching: ReturnType<typeof setInterval> | undefined
  function watch(id: string) {
    clearInterval(watching)
    watching = setInterval(() => {
      if (document.hidden) return
      void getGafferOutcome(id, life.signal).then(next => {
        record = next; watchError = ''
        if (next.state !== 'starting') { clearInterval(watching); void onstarted() }
      }).catch(e => { if (!life.signal.aborted) watchError = e instanceof Error ? e.message : 'The outcome could not be read.' })
    }, 1500)
  }
  async function approve() {
    if (busy || approveBlocker || !limitsOk || !provider || !project || !tool) return
    busy = true; startError = ''
    writeFloorDefaults(userId, { ...readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text), maxReviews, hours })
    try {
      record = await createGafferOutcome({
        objective: objective.trim(), projectId: project.id, tool, model: model.trim(), trust, maxReviews, hours, confirmed: true,
        sessions: sessions.map(item => ({ title: item.title.trim(), prompt: cleanPrompt(item.prompt) })),
      }, life.signal)
      step = 'results'
      watch(record.id)
      await onstarted()
    } catch (e) { if (!life.signal.aborted) startError = e instanceof Error ? e.message : 'The outcome was not saved. Nothing was started.' }
    finally { busy = false }
  }
  async function stop() {
    if (!record || busy) return
    busy = true
    try { record = await stopGafferOutcome(record, life.signal); watchError = '' }
    catch (e) { if (!life.signal.aborted) watchError = e instanceof Error ? e.message : 'The outcome was not stopped.' }
    finally { busy = false }
  }
  onMount(() => () => clearInterval(watching))
</script>

<WorkspaceDialog title="Give Gaffer an outcome" {onclose}>
  <div class="outcome">
    <ol class="steps" aria-label="Steps">
      {#each [['objective','Outcome'],['plan','Plan'],['results','Start']] as [id,name],index (id)}<li aria-current={step===id?'step':undefined} class:done={['objective','plan','results'].indexOf(step)>index}><span class="n">{index+1}</span>{name}</li>{/each}
    </ol>
    {#if step === 'objective'}
      <p>Say what you want done. Gaffer proposes the sessions to start; nothing starts until you approve the plan.</p>
      {#if loading}<p role="status">Reading your projects…</p>
      {:else if loadError}<p class="warning" role="alert">{loadError}</p>
      {:else}
        <form onsubmit={event => { event.preventDefault(); void ask() }}>
          <label for={`${formId}-objective`}>Outcome</label>
          <textarea id={`${formId}-objective`} bind:value={objective} rows="4" maxlength="4000" placeholder="Ship CSV export for billing: an API endpoint, a button on the billing page, and tests." disabled={planning}></textarea>
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
          <details class="limit-editor"><summary>Agent’s default model <span>Choose a model</span></summary>
            <label class="model">Model<input aria-label="Model" bind:value={model} maxlength="200" placeholder="Leave empty for the agent’s default" disabled={planning}/></label>
            <p class="hint">Use the name this agent accepts, as in its model picker. Every session of the outcome uses it.</p>
          </details>
          {#if !projects.length}<p class="hint">No projects yet. <a href="/">Add a project on Home</a> so sessions have a folder to work in.</p>{/if}
          {#if project && !project.isRepo}<p class="hint">This folder is not a git repository, so several sessions would share the same files. One session is safest.</p>{/if}
          {#if provider}<p class="hint">Gaffer plans with {provider.name} · {provider.model || 'model not chosen'}. It is sent only what you type here and the project’s name, and may use provider credits.</p>{/if}
          {#if blocker}<p class={provider&&!objective.trim()?'hint':'warning'} role="status">{blocker}{#if !provider}{' '}<a href="/account/gaffer">Open Gaffer settings</a>{/if}</p>{/if}
          {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
          <div class="actions"><button class="btn primary" type="submit" disabled={planning || !!blocker}>{planning ? 'Gaffer is planning…' : 'Ask Gaffer for a plan'}</button><button class="btn quiet" type="button" disabled={planning || !!blocker} onclick={writeOwn}>Write the plan myself</button></div>
        </form>
      {/if}
    {:else if step === 'plan'}
      <p class="objective"><span class="eyebrow">Outcome</span>{objective.trim()}</p>
      {#if summary}<div class="summary"><p class="eyebrow">Gaffer’s plan</p><p>{summary}</p></div>{/if}
      <p class="hint">{project?.name} · {chatToolName(tool)}. Edit anything; each session starts with exactly the prompt shown{project?.isRepo ? ', in its own worktree' : ''}.</p>
      <ol class="plan" aria-label="Planned sessions">
        {#each sessions as item, index (item.key)}<li>
          <div class="plan-head"><label for={`${formId}-${item.key}-title`}>Session {index + 1}</label><button class="link" type="button" aria-label={`Remove session ${index + 1}`} onclick={() => removeSession(item.key)}>Remove</button></div>
          <input id={`${formId}-${item.key}-title`} aria-label={`Session ${index + 1} title`} bind:value={item.title} maxlength="80" placeholder="Short title"/>
          <textarea aria-label={`Session ${index + 1} prompt`} bind:value={item.prompt} rows="5" placeholder="What this session should build, how to check it, and to raise a pull request. Do not merge."></textarea>
        </li>{/each}
      </ol>
      {#if sessions.length < cap}<button class="btn quiet add" type="button" onclick={addSession}>Add a session</button>{/if}
      <div class="access" aria-label="Outcome access review"><p>Approving has the controller start {sessions.length === 1 ? 'this session' : `these ${sessions.length} sessions`} on {project?.machine || 'the project’s machine'} with ordinary approvals (no YOLO), one after another, whether or not this page stays open. Gaffer may then read each one’s recent conversation, including code or logs in it, and queue instructions for its working model. If the provider does not yet allow session data, this enables it. Gaffer cannot merge, run commands or answer approvals.</p></div>
      <details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours per session <span>Change limits</span></summary>
        <div class="pair two"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours}/></label></div>
      </details>
      {#if needsTrust}<label class="trust"><input type="checkbox" bind:checked={trust} disabled={busy}/><span>I trust this folder: {chatToolName(tool)} may run the plugins and customisations it finds in {project?.dir || 'it'}. Leave this off and a folder that has any is refused.</span></label>{/if}
      {#if approveBlocker}<p class="warning" role="status">{approveBlocker}</p>
      {:else if !limitsOk}<p class="warning" role="status">Choose a whole number of reviews from 1 to 48 and hours from 1 to 168.</p>{/if}
      {#if startError}<p class="warning" role="alert">{startError}</p>{/if}
      <div class="actions"><button class="btn primary" type="button" disabled={busy || !!approveBlocker || !limitsOk} onclick={() => void approve()}>{busy ? 'Saving the plan…' : `Approve and start ${sessions.length} ${sessions.length === 1 ? 'session' : 'sessions'}`}</button><button class="btn quiet" type="button" disabled={busy} onclick={() => step = 'objective'}>Back</button></div>
    {:else if record}
      <p class="objective"><span class="eyebrow">Outcome</span>{record.objective}</p>
      <p role="status" class="result-line">{done ? outcomeSummary(record.sessions) : startingProgress(record.sessions)}</p>
      {#if record.state === 'stopped' && record.note}<p class="hint">{record.note}</p>{/if}
      {#if watchError}<p class="warning" role="alert">{watchError}</p>{/if}
      <ul class="results" aria-label="Outcome sessions">
        {#each record.sessions as item (item.key)}<li class:failed={item.state === 'failed' || item.state === 'unfollowed' || item.state === 'cancelled'}>
          <strong>{item.title}</strong><span>{item.note}</span>
          {#if item.sessionId}<a href={`/sessions?agent=${encodeURIComponent(record.agentId)}&session=${encodeURIComponent(item.sessionId)}&gaffer=1`}>Open session</a>{/if}
        </li>{/each}
      </ul>
      {#if done}<p class="hint">Each session has its own goal naming this outcome, and its own budget. The floor groups them under the outcome; only you mark a goal complete.</p>
        <div class="actions"><button class="btn primary" type="button" onclick={onclose}>Back to the floor</button></div>
      {:else}<div class="actions"><button class="btn" type="button" onclick={onclose}>Close and let it run</button><button class="btn quiet" type="button" disabled={busy} onclick={() => void stop()}>Stop starting sessions</button></div>{/if}
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
  .steps{list-style:none;display:flex;gap:6px 18px;flex-wrap:wrap;margin:0;padding:0 0 12px;border-bottom:1px solid var(--line);color:var(--muted);font-size:0.875rem}.steps li{display:flex;align-items:center;gap:8px}.steps .n{display:inline-grid;place-items:center;width:22px;height:22px;border-radius:50%;border:1px solid var(--line-strong);font-variant-numeric:tabular-nums;font-size:0.8125rem}.steps li[aria-current]{color:var(--ink);font-weight:600}.steps li[aria-current] .n,.steps li.done .n{background:var(--accent);border-color:var(--accent);color:var(--accent-ink)}
  .result-line{font-weight:600}.model{padding-top:8px}.trust{display:flex;align-items:flex-start;gap:10px;font-weight:400;min-height:44px}.trust input{width:20px;height:20px;min-height:0;margin:2px 0 0;flex:none;padding:0;accent-color:var(--accent)}
  .results{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.results li{display:grid;gap:2px;padding:10px 14px;min-width:0;overflow-wrap:anywhere}.results li+li{border-top:1px solid var(--line)}.results span{font-size:0.875rem;color:var(--muted)}.results li.failed span{color:var(--bad)}.results a{color:var(--accent-text);font-weight:600;min-height:44px;display:inline-flex;align-items:center;justify-self:start}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,a:focus-visible,summary:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.pair,.pair.two{grid-template-columns:1fr}}
</style>
