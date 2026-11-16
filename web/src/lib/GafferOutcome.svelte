<script lang="ts">
  // Give Gaffer an outcome. Gaffer proposes the sessions, the person edits and approves the plan, and only then does the
  // controller start each session through the ordinary launch and put it in Gaffer's care. This dialog only watches:
  // the work carries on if it is closed. Nothing starts on the model's say-so: a proposed session is text until Approve.
  import { onMount, untrack } from 'svelte'
  import { createGafferOutcome, getGafferCrew, saveGafferCrew, type GafferCrew, getGafferOutcome, getGafferSettings, getProfile, getYoloSetting, listModelFavourites, planGafferOutcome, stopGafferOutcome, type GafferOutcome, type GafferProvider, type YoloSetting } from './api'
  import { chatToolName, launchUpload, listFolderProjects, machineModels, structuredAgent, terminalMachines, type FolderProject, type TerminalMachine } from './terminal/api'
  import { imageFiles, imagePrompt } from './terminal/upload'
  import ImageAttachment from './ImageAttachment.svelte'
  import { attachmentFromPaste, attachmentMessage } from './imageAttachment'
  import GafferModelSelect from './GafferModelSelect.svelte'
  import ClineSourceSelect from './ClineSourceSelect.svelte'
  import { KNOWN_SOURCES, resolveStartSource } from './clineSource'
  import GafferProse from './GafferProse.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import YoloDialog from './launch/YoloDialog.svelte'
  import { yoloCanStart, type YoloGate } from './launchBar'
  import { CATALOGUE } from './catalogue'
  import type { MachineModels, ModelFavourite } from './models'
  import { reportedEfforts } from './terminal/launchOptions'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { readFloorDefaults, writeFloorDefaults } from './gafferFloor'
  import { BUILT_IN_GOAL_PROMPTS } from './gafferPrompts'
  import { CREW_TIERS, DEFAULT_STAGES, MAX_OUTCOME_REPLY_BYTES, MAX_OUTCOME_SESSIONS, STAGES, cleanPrompt, objectiveBlocker, orderStages, outcomeSummary, planBlocker, previousPlan, replyBlocker, stagePlan, stageLabel, stageTier, tierLabel, planRatings, withTier, startingProgress, wholeBetween, withReportedEfforts, type Crew, type PlanSession, type PlanTurn, BALANCE_LOW_PERCENT, BALANCE_MIN_PERCENT, BALANCE_MAX_PERCENT, balancePercent, stageParts, withPart, withoutParts, type StagePart } from './gafferOutcome'
  import { gafferAutoMerge } from './gafferMotion'
  import RepoField from './home/RepoField.svelte'
  import { cloneForStart } from './cloneForStart'
  import { cloneFolder, repoProject, parseGitHubRepo } from './githubRepo'

  let { room, userId = 0, projectId: wantedProject = '', initialObjective = '', onclose, onstarted }: { room: number; userId?: number;
    /** The project the page had chosen when the dialog opened; it starts selected and can be changed. */
    projectId?: string; initialObjective?: string; onclose: () => void; onstarted: () => void | Promise<void> } = $props()
  const formId = $props.id()
  const CHAT_TOOLS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const life = new AbortController()
  let step = $state<'objective' | 'plan' | 'results'>('objective')
  let loading = $state(true), loadError = $state('')
  let provider = $state<GafferProvider | null>(null), projects = $state<FolderProject[]>([]), machines = $state<TerminalMachine[]>([])
  let objective = $state(untrack(() => initialObjective)), projectId = $state(''), tool = $state(''), wanted = $state(3)
  let planning = $state(false), planError = $state(''), summary = $state(''), sessions = $state<PlanSession[]>([])
  // The reply a person sends to a proposal, and the exchange it belongs to. A declined proposal starts nothing.
  let reply = $state(''), planThread = $state<PlanTurn[]>([]), declined = $state(false), declining = $state(false)
  const replyProblem = $derived(replyBlocker(reply))
  let maxReviews = $state(12), hours = $state(24)
  let trust = $state(false)
  // The model every stage uses unless a crew role is given its own. Empty is the agent's default.
  let model = $state(''), effort = $state('')
  // The Cline billing source: the account's saved default, and the person's own choice for this run.
  let savedSource = $state(''), explicitSource = $state(false), source = $state('')
  let crewSourceExplicit = $state<Record<string, boolean>>({ heavy: false, standard: false, light: false })
  // What each agent offers on the project's machine, read once per agent. A failed read leaves favourites, suggestions and Other.
  let offered = $state<Record<string, MachineModels | null>>({}), favourites = $state<ModelFavourite[]>([])
  // YOLO: the account setting, and whether the person confirmed it for this outcome. It is never remembered.
  let yoloSetting = $state<YoloSetting | null>(null), yolo = $state(false), yoloOpen = $state(false)
  // The crew: which agent and model does the thinking, the building and the shipping stages of every pipeline.
  let crew = $state<Crew>({ heavy: { tool: '', model: '' }, standard: { tool: '', model: '' }, light: { tool: '', model: '' } })
  // Each tier's backup: who does its stages when the first choice's plan is running low. An empty agent is no backup.
  let backups = $state<Record<'heavy' | 'standard' | 'light', { tool: string; model: string; effort: string }>>({ heavy: { tool: '', model: '', effort: '' }, standard: { tool: '', model: '', effort: '' }, light: { tool: '', model: '', effort: '' } })
  // The share of the first choice's plan under which a backup takes over. The person's to set; saved with the crew.
  let balanceBelow = $state<number | string>(BALANCE_LOW_PERCENT)
  const anyBackup = $derived(CREW_TIERS.some(item => !!backups[item.tier].tool))
  const crewSent = $derived<Crew>({ heavy: { ...crew.heavy, backup: backups.heavy }, standard: { ...crew.standard, backup: backups.standard }, light: { ...crew.light, backup: backups.light } })
  let autoMerge = $state(false)
  // Gaffer reports across the outcome by itself when a pipeline finishes or stops. Each report is one model request.
  let autoReport = $state(false)
  // Who takes the decisions when a stage stops: the person, or Gaffer for them. The person's unless they say so.
  let automation = $state(false)
  let busy = $state(false), startError = $state(''), record = $state<GafferOutcome | null>(null), watchError = $state('')
  let image = $state<File | null>(null), imageUploading = $state(false), imageError = $state('')
  let counter = 0
  const key = () => `plan-${++counter}`
  function clearImage() { image = null; imageError = '' }
  $effect(() => { void image; imageError = '' })
  // A pasted screenshot becomes the outcome's image, exactly as if it had been picked. Text pastes are left alone.
  function pasted(event: ClipboardEvent) {
    if (planning || busy) return
    const data = event.clipboardData
    const files = data?.files.length ? data.files : imageFiles(data?.items)
    const found = attachmentFromPaste(files)
    if (!found) return
    event.preventDefault()
    if (!found.ok) { imageError = attachmentMessage(found.reason); return }
    image = found.file
    imageError = ''
  }

  // A repository From GitHub instead of a saved project: the person names it and a machine that can clone it, and it
  // becomes a project (cloned there, or the one already there) before Gaffer plans, so the rest is an ordinary outcome.
  const GITHUB = '\u0000github'
  const github = $derived(projectId === GITHUB)
  let ghHost = $state(''), ghRepo = $state(''), ghLeft = $state(false), cloning = $state(false), cloneLine = $state('')
  const cloners = $derived(machines.filter(item => item.ready && item.capabilities?.repoClone))
  const ghParsed = $derived(parseGitHubRepo(ghRepo))
  const ghProblem = $derived(github && ghLeft && ghRepo.trim() !== '' && !ghParsed ? 'Write the repository as owner/name, or paste its github.com link.' : '')
  $effect(() => { if (github && !cloners.some(item => item.agentId === ghHost)) ghHost = cloners.length === 1 ? cloners[0].agentId : '' })
  const project = $derived(projects.find(item => item.id === projectId) ?? null)
  const machine = $derived(project ? machines.find(item => item.agentId === project.host) ?? null : github ? cloners.find(item => item.agentId === ghHost) ?? null : null)
  const ghKnown = $derived(github && machine && ghParsed ? repoProject(projects, { host: machine.agentId, repo: `${ghParsed.owner}/${ghParsed.repo}` }) : undefined)
  const tools = $derived(machine ? CHAT_TOOLS.filter(id => structuredAgent(machine.capabilities, id)) : [])
  const cap = $derived(Math.max(1, Math.min(MAX_OUTCOME_SESSIONS, room)))
  const githubBlocker = $derived(!github ? '' : !cloners.length ? 'No machine here can clone from GitHub yet. It needs a current reet, git and the GitHub CLI.' : !machine ? 'Choose the machine to clone it on.' : !ghParsed ? 'Choose a repository, or type owner/name.' : '')
  const blocker = $derived(loading ? '' : !provider ? 'Choose a default Gaffer provider in Gaffer settings first.' : (objective.trim() && githubBlocker) || objectiveBlocker({ objective, project: !!project || github, machineReady: !!machine?.ready, agent: !!tool && tools.includes(tool), room }))
  /** The project the outcome works in: the one chosen, or the repository From GitHub made into one. False when it could not be. */
  async function ensureProject(): Promise<boolean> {
    if (!github) return !!project
    if (!machine || !ghParsed) return false
    cloning = true; planError = ''
    cloneLine = `Getting ${ghParsed.owner}/${ghParsed.repo} ready on ${machine.host}…`
    const ready = await cloneForStart(machine.agentId, machine.host, ghRepo, projects, life.signal, { onwaiting: line => { if (!life.signal.aborted) cloneLine = line.replace('the agent starts', 'Gaffer plans') } })
    cloning = false; cloneLine = ''
    if (life.signal.aborted) return false
    if ('signedout' in ready) { planError = 'You were signed out. Sign in again, then try again.'; return false }
    if ('sentence' in ready) { planError = ready.sentence; return false }
    if (!projects.some(item => item.id === ready.project.id)) projects = [...projects, ready.project]
    projectId = ready.project.id
    return true
  }
  const crewTools = $derived(CREW_TIERS.map(item => crew[item.tier].tool))
  // Google Antigravity never starts in a folder that was not trusted, so approving a crew with it and no trust would only fail later.
  const approveBlocker = $derived(planBlocker(sessions, room) || (crewTools.includes('antigravity') && !trust ? 'Google Antigravity starts only in a folder you trust. Tick the folder trust box below, or choose another agent.' : ''))
  const sourceOffered = $derived.by(() => {
    const ids = KNOWN_SOURCES.map(item => item.id)
    if (savedSource && !ids.includes(savedSource)) ids.push(savedSource)
    return ids
  })
  const startSource = $derived(resolveStartSource({ explicit: explicitSource ? source : undefined, saved: savedSource, offered: sourceOffered }))
  const chosenSource = $derived(startSource.source)
  const sourceShown = $derived(tool === 'cline' && machine?.capabilities?.acpProvider === true)
  const needsTrust = $derived(crewTools.some(id => id === 'opencode' || id === 'antigravity'))
  const trustNames = $derived([...new Set(crewTools.filter(id => id === 'opencode' || id === 'antigravity'))].map(chatToolName).join(' and '))
  const limitsOk = $derived(wholeBetween(maxReviews, 1, 48) && wholeBetween(hours, 1, 168))
  const done = $derived(!!record && record.state !== 'starting')

  // A model or effort chosen for one agent means nothing to another: when the machine changes the agent, they go with it.
  $effect(() => { if (tools.length && !tools.includes(tool)) { tool = tools[0]; model = ''; effort = '' } if (!tools.length) tool = '' })
  $effect(() => { if (wanted > cap) wanted = cap })
  // The crew the person saved for their account is where the dialog starts, for the tiers this machine can run. It is
  // applied once, when the machine's agents are known; after that the crew here is theirs to change for this outcome.
  let savedCrew = $state<GafferCrew | null>(null), crewApplied = false, crewSaving = $state(false), crewNote = $state('')
  // The saved default for who decides is where this dialog starts; the person still chooses for this outcome.
  onMount(() => { void getGafferCrew().then(answer => { if (answer.saved) { savedCrew = answer.crew; if (answer.crew.gafferDecides) automation = true } }).catch(() => {}) })
  $effect(() => {
    if (crewApplied || !savedCrew || !tools.length || !tool) return
    crewApplied = true
    const saved = savedCrew
    untrack(() => {
      let used = 0
      for (const item of CREW_TIERS) {
        const member = saved[item.tier]
        if (!member?.tool || !tools.includes(member.tool)) continue
        crew[item.tier] = { tool: member.tool, model: member.model ?? '', effort: member.effort ?? '', source: member.provider ?? '' }
        if (member.provider) crewSourceExplicit[item.tier] = true
        // A saved backup this machine cannot run is left out, like a saved first choice.
        if (member.backup?.tool && tools.includes(member.backup.tool)) backups[item.tier] = { tool: member.backup.tool, model: member.backup.model ?? '', effort: member.backup.effort ?? '' }
        used++
      }
      if (saved.balanceBelow) balanceBelow = balancePercent(saved.balanceBelow)
      if (used) crewNote = 'Started from your saved crew.'
    })
  })
  async function rememberCrew() {
    crewSaving = true; crewNote = ''
    try {
      const member = (tier: 'light' | 'standard' | 'heavy') => ({ tool: crew[tier].tool, model: crew[tier].model.trim(), effort: crew[tier].effort ?? '', ...(crew[tier].tool === 'cline' && crew[tier].source ? { provider: crew[tier].source } : {}), ...(backups[tier].tool ? { backup: { tool: backups[tier].tool, model: backups[tier].model.trim(), effort: backups[tier].effort } } : {}) })
      // What was just saved is what is on screen: there is nothing to apply again.
      crewApplied = true
      savedCrew = (await saveGafferCrew({ light: member('light'), standard: member('standard'), heavy: member('heavy'), balanceBelow: balancePercent(balanceBelow), gafferDecides: !!savedCrew?.gafferDecides })).crew
      crewNote = 'Saved. New outcomes start from this crew.'
    } catch (e) { crewNote = e instanceof Error ? e.message : 'The crew could not be saved.' }
    finally { crewSaving = false }
  }
  // A crew role follows the main agent until the person gives it one of its own that this machine can run.
  $effect(() => { for (const item of CREW_TIERS) if (!tools.includes(crew[item.tier].tool) && crew[item.tier].tool !== tool) crew[item.tier] = { ...crew[item.tier], tool, model: '', effort: '' } })
  // A new agent starts on the saved source; a source picked for the old agent is not carried to the new one.
  let lastTool = ''
  $effect(() => {
    const next = tool
    untrack(() => {
      if (lastTool && next !== lastTool) { explicitSource = false; source = '' }
      lastTool = next
    })
  })
  // A Cline selection opens on the saved default until the person picks another source for this run.
  $effect(() => {
    const saved = savedSource, currentTool = tool, currentExplicit = explicitSource, currentSource = source, currentTools = crewTools
    untrack(() => {
      if (currentTool === 'cline' && !currentExplicit && currentSource !== saved) source = saved
      for (const item of CREW_TIERS) {
        if (!currentTools.includes('cline')) continue
        const role = crew[item.tier]
        if (role.tool === 'cline' && !crewSourceExplicit[item.tier] && (role.source ?? '') !== saved) role.source = saved
      }
    })
  })
  $effect(() => { if (!needsTrust) trust = false })
  $effect(() => {
    const agentId = machine?.agentId
    if (!agentId) return
    for (const id of new Set([tool, ...crewTools])) {
      const slot = `${agentId}:${id}`
      if (!id || slot in offered) continue
      offered[slot] = null
      void machineModels(agentId, id, life.signal).then(found => { offered[slot] = found }).catch(() => { /* suggestions and Other still work */ })
    }
  })
  // Every session here is a chat, so the efforts are the agent's own thinking levels as this machine reports them.
  // A level this machine does not report is never carried: an unattended stage the agent refuses fails the whole launch.
  const efforts = (id: string) => reportedEfforts(id, machine?.capabilities)
  // A chosen level the agent no longer reports (another project's machine, or a crew role's own agent) goes back to the
  // agent's default, so what is shown is what the outcome will start with.
  $effect(() => {
    if (effort && !efforts(tool).includes(effort)) effort = ''
    for (const item of CREW_TIERS) {
      const role = crew[item.tier]
      if (role.effort && !efforts(role.tool).includes(role.effort)) crew[item.tier] = { ...role, effort: '' }
    }
  })
  const offeredFor = (id: string) => machine ? offered[`${machine.agentId}:${id}`] ?? null : null
  const yoloGates = $derived.by((): YoloGate[] => {
    const account = yoloSetting?.allowed === true
    const host = machine?.host || 'This machine'
    const without = [...new Set(crewTools.filter(id => id && !CATALOGUE.find(agent => agent.id === id)?.yolo))].map(chatToolName)
    return [
      { label: 'Your account allows YOLO', ok: account, detail: account ? (yoloSetting?.changedBy ? `set by ${yoloSetting.changedBy}` : 'set by an owner') : yoloSetting ? 'An owner or admin can allow YOLO for this account in Settings.' : 'Reading the account setting…' },
      { label: `${host} accepts YOLO sessions`, ok: !!machine?.capabilities?.yolo, detail: machine?.capabilities?.yolo ? 'reported by the machine' : `${host} doesn’t accept YOLO sessions.` },
      { label: 'Every agent in the crew has a YOLO option', ok: !without.length, detail: without.length ? `${without.join(' and ')} has no option reet can use for that.` : crewTools.filter(Boolean).length ? [...new Set(crewTools.filter(Boolean))].map(chatToolName).join(', ') : 'No agent chosen' },
    ]
  })
  const yoloWhy = $derived(yoloGates.find(gate => !gate.ok)?.detail ?? '')
  $effect(() => { if (yoloWhy) yolo = false })

  onMount(() => {
    const saved = readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text)
    maxReviews = saved.maxReviews; hours = saved.hours
    autoMerge = $gafferAutoMerge
    void getYoloSetting().then(found => { yoloSetting = found }).catch(() => { yoloSetting = { allowed: false } })
    void getProfile().then(answer => { if (!life.signal.aborted) savedSource = answer.profile.defaultClineSource ?? '' }).catch(() => { /* Cline’s own choice */ })
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => { /* a helper only */ })
    void Promise.all([getGafferSettings(life.signal), listFolderProjects(life.signal), terminalMachines(life.signal)]).then(([settings, folders, found]) => {
      provider = settings.available ? settings.providers.find(item => item.isDefault && item.enabled) ?? null : null
      projects = folders; machines = found
      if (folders.some(item => item.id === wantedProject)) projectId = wantedProject
      else if (folders.length === 1) projectId = folders[0].id
    }).catch(e => { if (!life.signal.aborted) loadError = e instanceof Error ? e.message : 'Projects and machines could not be read.' })
      .finally(() => { loading = false })
    return () => life.abort()
  })

  // Choosing the main source applies it to every stage that follows the main agent, unless a crew role has its own.
  function pickSource(next: string) {
    source = next; explicitSource = true
    for (const item of CREW_TIERS) {
      if (crew[item.tier].tool !== tool) continue
      crew[item.tier] = { ...crew[item.tier], source: next }
      crewSourceExplicit[item.tier] = true
    }
  }
  function pickCrewSource(role: keyof Crew, next: string) {
    crew[role] = { ...crew[role], source: next }
    crewSourceExplicit[role] = true
  }

  async function ask() {
    if (planning || cloning || blocker || !provider) return
    if (!(await ensureProject()) || !project) return
    planning = true; planError = ''; reply = ''; planThread = []; declined = false
    try {
      const plan = await planGafferOutcome(provider, { objective: objective.trim(), project: project.name, sessions: wanted }, life.signal)
      summary = plan.summary
      sessions = plan.sessions.map(item => ({ key: key(), title: item.title, prompt: item.prompt, stages: item.stages?.length ? orderStages([...new Set(item.stages.map(stage => stage.kind))]) : [...DEFAULT_STAGES], ...planRatings(item.stages) }))
      step = 'plan'
    } catch (e) { if (!life.signal.aborted) planError = e instanceof Error ? e.message : 'Gaffer could not plan this outcome.' }
    finally { planning = false }
  }
  // Without a tool-capable model, or by choice, the person writes the plan; the rest of the flow is the same.
  async function writeOwn() {
    if (cloning || blocker) return
    if (!(await ensureProject())) return
    summary = ''; sessions = [{ key: key(), title: '', prompt: '', stages: [...DEFAULT_STAGES] }]; planError = ''; reply = ''; planThread = []; declined = false; step = 'plan'
  }
  // A reply is sent back to Gaffer with the proposal it answers, and the revised plan replaces the rows. The person's
  // words stay in the review as part of the exchange; nothing is saved or started until the existing approve action.
  async function sendReply() {
    if (planning || !provider || !project || replyBlocker(reply)) return
    const text = reply.trim()
    const thread: PlanTurn[] = planThread.length ? planThread : summary ? [{ from: 'gaffer', text: summary }] : []
    planThread = [...thread, { from: 'you', text }]
    planning = true; planError = ''; reply = ''
    try {
      const plan = await planGafferOutcome(provider, { objective: objective.trim(), project: project.name, sessions: wanted, reply: text, previous: previousPlan(summary, sessions) }, life.signal)
      summary = plan.summary
      sessions = plan.sessions.map(item => ({ key: key(), title: item.title, prompt: item.prompt, stages: item.stages?.length ? orderStages([...new Set(item.stages.map(stage => stage.kind))]) : [...DEFAULT_STAGES], ...planRatings(item.stages) }))
      planThread = [...planThread, { from: 'gaffer', text: plan.summary }]
      declined = false
    } catch (e) {
      planThread = planThread.slice(0, -1)
      reply = text
      if (!life.signal.aborted) planError = e instanceof Error ? e.message : 'Gaffer could not answer your reply.'
    } finally { planning = false }
  }
  const addSession = () => { sessions = [...sessions, { key: key(), title: '', prompt: '', stages: [...DEFAULT_STAGES] }] }
  const setTier = (item: PlanSession, kind: string, tier: string) => { const next = withTier(item, kind, tier); item.tiers = next.tiers; item.whys = next.whys }
  const toggleStage = (item: PlanSession, kind: string) => { const now = item.stages ?? []; item.stages = now.includes(kind) ? now.filter(k => k !== kind) : orderStages([...now, kind]) }
  const hasMerge = $derived(sessions.some(item => item.stages?.includes('merge')))
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
    busy = true; startError = ''; imageError = ''
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
    writeFloorDefaults(userId, { ...readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text), maxReviews, hours })
    try {
      record = await createGafferOutcome({
        objective: objective.trim(), projectId: project.id, tool, model: model.trim(), provider: sourceShown ? chosenSource : '', trust, yolo: yolo && !yoloWhy, autoReport, automation, balanceBelow: balancePercent(balanceBelow), autoMerge: autoMerge && hasMerge, maxReviews, hours, confirmed: true,
        sessions: sessions.map(item => ({ title: item.title.trim(), prompt: reference ? imagePrompt(cleanPrompt(item.prompt), [reference]) : cleanPrompt(item.prompt), stages: withReportedEfforts(stagePlan(item.stages ?? DEFAULT_STAGES, crewSent, machine?.capabilities?.acpProvider === true, item), efforts).map(stage => stage.backup?.effort && !efforts(stage.backup.tool).includes(stage.backup.effort) ? { ...stage, backup: { tool: stage.backup.tool, ...(stage.backup.model ? { model: stage.backup.model } : {}) } } : stage) })),
      }, life.signal)
      step = 'results'; clearImage()
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
          <textarea id={`${formId}-objective`} bind:value={objective} rows="4" maxlength="4000" placeholder="Ship CSV export for billing: an API endpoint, a button on the billing page, and tests." disabled={planning} onpaste={pasted}></textarea>
          <ImageAttachment bind:file={image} label="Attach an image to the outcome" disabled={planning} busy={imageUploading} error={imageError} />
          <div class="pair">
            <label>Project<select aria-label="Project" bind:value={projectId} disabled={planning || cloning}>
              <option value="">Choose a project…</option>
              {#each projects as item (item.id)}<option value={item.id}>{item.name}{item.machine ? ` · ${item.machine}` : ''}</option>{/each}
              <option value={GITHUB}>A repository from GitHub…</option>
            </select></label>
            {#if github}<label>Machine<select aria-label="Machine" bind:value={ghHost} disabled={planning || cloning || !cloners.length}>
              {#if !ghHost}<option value="" disabled>{cloners.length ? 'Choose a machine…' : 'None can clone'}</option>{/if}
              {#each cloners as item (item.agentId)}<option value={item.agentId}>{item.host}</option>{/each}
            </select></label>{/if}
            <label>Agent<select aria-label="Agent" bind:value={tool} disabled={planning || !tools.length} onchange={() => { model = ''; effort = ''; source = ''; explicitSource = false; crewSourceExplicit = { heavy: false, standard: false, light: false }; for (const item of CREW_TIERS) crew[item.tier] = { tool, model: '', effort: '', source: '' } }}>
              {#if !tools.length}<option value="">{project ? 'None available' : 'Choose a project first'}</option>{/if}
              {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
            </select></label>
            <label>Model<GafferModelSelect label="Model" {tool} bind:value={model} machine={offeredFor(tool)} {favourites} host={machine?.host ?? ''} disabled={planning || !tool} onchange={next => { for (const item of CREW_TIERS) if (crew[item.tier].tool === tool) crew[item.tier].model = next }}/></label>
            <label>Effort<select aria-label="Effort" bind:value={effort} disabled={planning || efforts(tool).length < 2} onchange={() => { for (const item of CREW_TIERS) if (crew[item.tier].tool === tool) crew[item.tier].effort = effort }}>
              {#each efforts(tool) as level (level)}<option value={level}>{level ? level[0].toUpperCase() + level.slice(1) : efforts(tool).length < 2 ? 'Not offered by this agent' : 'Agent’s default effort'}</option>{/each}
            </select></label>
            <label>Sessions<select aria-label="Sessions" bind:value={wanted} disabled={planning}>
              {#each Array.from({ length: cap }, (_, n) => n + 1) as n (n)}<option value={n}>Up to {n}</option>{/each}
            </select></label>
            {#if sourceShown}<div class="source-field"><ClineSourceSelect label="Source" value={chosenSource} saved={savedSource} offered={sourceOffered} note={startSource.note} disabled={planning} onchange={pickSource} onsaved={next => savedSource = next} /></div>{/if}
          </div>
          <details class="limit-editor crew"><summary>{crewTools.every(id => id === tool) && CREW_TIERS.every(item => crew[item.tier].model.trim() === model.trim() && (crew[item.tier].effort ?? '') === effort) ? 'One agent does every stage' : 'A crew does the stages'} <span>Choose who does what</span></summary>
            <p class="hint">Right tool for the job. Each stage has a tier: Heavy for careful judgement, Standard for ordinary building, Light for routine work. Say here which agent, model and effort each tier means. Gaffer’s plan puts a stage on another tier where the work calls for it, and you can change any stage below. Every stage runs in a session of its own.</p>
            <div class="crew-rows">
              {#each CREW_TIERS as item (item.tier)}<div class="crew-row" role="group" aria-label={`${item.label} crew`}>
                <div class="crew-role"><strong>{item.label}</strong><span>{item.does}</span></div>
                <span class="crew-slot">First choice</span>
                <select aria-label={`${item.label} agent`} bind:value={crew[item.tier].tool} disabled={planning || !tools.length} onchange={() => { const same = crew[item.tier].tool === tool; crew[item.tier].model = same ? model : ''; crew[item.tier].effort = same ? effort : ''; crew[item.tier].source = same ? chosenSource : ''; crewSourceExplicit[item.tier] = false }}>{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
                <GafferModelSelect label={`${item.label} model`} tool={crew[item.tier].tool} bind:value={crew[item.tier].model} machine={offeredFor(crew[item.tier].tool)} {favourites} host={machine?.host ?? ''} disabled={planning}/>
                <select aria-label={`${item.label} effort`} bind:value={crew[item.tier].effort} disabled={planning || efforts(crew[item.tier].tool).length < 2}>{#each efforts(crew[item.tier].tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)} effort` : efforts(crew[item.tier].tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}</select>
                {#if crew[item.tier].tool === 'cline' && machine?.capabilities?.acpProvider}<div class="crew-source"><ClineSourceSelect label={`${item.label} source`} value={crew[item.tier].source ?? ''} saved={savedSource} offered={sourceOffered} disabled={planning} onchange={next => pickCrewSource(item.tier, next)} onsaved={next => savedSource = next} /></div>{/if}
                <div class="crew-backup" role="group" aria-label={`${item.label} backup`}>
                  <span class="crew-slot">Backup</span>
                  <select aria-label={`${item.label} backup agent`} bind:value={backups[item.tier].tool} disabled={planning || !tools.length} onchange={() => { backups[item.tier].model = ''; backups[item.tier].effort = '' }}><option value="">No backup</option>{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
                  {#if backups[item.tier].tool}
                    <GafferModelSelect label={`${item.label} backup model`} tool={backups[item.tier].tool} bind:value={backups[item.tier].model} machine={offeredFor(backups[item.tier].tool)} {favourites} host={machine?.host ?? ''} disabled={planning}/>
                    <select aria-label={`${item.label} backup effort`} bind:value={backups[item.tier].effort} disabled={planning || efforts(backups[item.tier].tool).length < 2}>{#each efforts(backups[item.tier].tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)} effort` : efforts(backups[item.tier].tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}</select>
                  {:else}<span class="crew-none">Stages stay on the first choice.</span>{/if}
                </div>
              </div>{/each}
            </div>
            <div class="balance" role="group" aria-label="When a backup takes over">
              <label for={`${formId}-balance`}>Switch to a backup below</label>
              <span class="percent"><input id={`${formId}-balance`} type="number" inputmode="numeric" min={BALANCE_MIN_PERCENT} max={BALANCE_MAX_PERCENT} step="5" bind:value={balanceBelow} disabled={planning} aria-describedby={`${formId}-balance-unit`} onblur={() => balanceBelow = balancePercent(balanceBelow)}/><span id={`${formId}-balance-unit`}>% of the first choice’s plan left</span></span>
            </div>
            <p class="hint">{anyBackup ? `A stage starts on its tier’s backup when the first choice’s plan has under ${balancePercent(balanceBelow)}% left in its tightest window and the backup’s has more. The stage says when that happened and why.` : 'No tier has a backup yet, so every stage runs on its first choice.'} Balancing needs plan limits to be on for the machine; without figures every stage stays on its first choice.</p>
            <p class="crew-save"><button class="link" type="button" disabled={planning || crewSaving || !tools.length} onclick={() => void rememberCrew()}>{crewSaving ? 'Saving…' : 'Save as my crew'}</button> <span role="status">{crewNote}</span></p>
          </details>
          {#if github}
            <div class="repo-field">
              <label for={`${formId}-gh-repo`}>Repository</label>
              <RepoField id={`${formId}-gh-repo`} machine={machine ?? undefined} bind:value={ghRepo} problem={ghProblem} disabled={planning || cloning} onleave={() => (ghLeft = true)} onsignedout={() => (planError = 'You were signed out. Sign in again, then try again.')} />
              {#if ghKnown}<p class="hint">Works in your project {ghKnown.name}, {ghKnown.dir}.</p>
              {:else if ghParsed && machine}<p class="hint">reet clones it to {cloneFolder(ghParsed)} on {machine.host} before Gaffer plans. Once the outcome is done reet removes the clone, unless that would lose work.</p>{/if}
            </div>
          {/if}
          {#if cloneLine}<p class="hint" role="status">{cloneLine}</p>{/if}
          {#if !projects.length && !github}<p class="hint">No projects yet. Choose <strong>A repository from GitHub…</strong> above, or <a href="/">add a project on Home</a>, so sessions have a folder to work in.</p>{/if}
          {#if project && !project.isRepo}<p class="hint">This folder is not a git repository, so several sessions would share the same files. One session is safest.</p>{/if}
          {#if provider}<p class="hint">Gaffer plans with {provider.name} · {provider.model || 'model not chosen'}. It is sent only what you type here and the project’s name, and may use provider credits.</p>{/if}
          {#if blocker}<p class={provider&&!objective.trim()?'hint':'warning'} role="status">{blocker}{#if !provider}{' '}<a href="/account/gaffer">Open Gaffer settings</a>{/if}</p>{/if}
          {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
          <div class="actions"><button class="btn primary" type="submit" disabled={planning || cloning || !!blocker}>{cloning ? 'Cloning…' : planning ? 'Gaffer is planning…' : 'Ask Gaffer for a plan'}</button><button class="btn quiet" type="button" disabled={planning || cloning || !!blocker} onclick={() => void writeOwn()}>Write the plan myself</button></div>
        </form>
      {/if}
    {:else if step === 'plan'}
      <p class="objective"><span class="eyebrow">Outcome</span>{objective.trim()}</p>
      {#if planThread.length}
        <section class="talk" aria-label="Conversation with Gaffer">
          {#each planThread as turn, index (index)}
            <div class="said {turn.from}"><p class="eyebrow">{turn.from === 'you' ? 'You' : 'Gaffer'}</p>{#if turn.from === 'you'}<p>{turn.text}</p>{:else}<GafferProse text={turn.text}/>{/if}</div>
          {/each}
        </section>
      {:else if summary}<div class="summary"><p class="eyebrow">Gaffer’s plan</p><p>{summary}</p></div>{/if}
      {#if declined}
        <p class="declined" role="status">Proposal declined. Nothing was started. Reply below to tell Gaffer what you want instead, or go back to the outcome.</p>
      {:else}
      <p class="hint">{project?.name}. Each row below is a pipeline: edit its brief, and tick the stages it runs through. The first stage starts with the prompt shown{project?.isRepo ? ', in its own worktree' : ''}; later stages take over in the same folder.</p>
      <ImageAttachment bind:file={image} label="Attach an image to the outcome" disabled={busy} busy={imageUploading} error={imageError} />
      <ol class="plan" aria-label="Planned sessions">
        {#each sessions as item, index (item.key)}<li>
          <div class="plan-head"><label for={`${formId}-${item.key}-title`}>Pipeline {index + 1}</label><button class="link" type="button" aria-label={`Remove pipeline ${index + 1}`} onclick={() => removeSession(item.key)}>Remove</button></div>
          <input id={`${formId}-${item.key}-title`} aria-label={`Pipeline ${index + 1} title`} bind:value={item.title} maxlength="80" placeholder="Short title"/>
          <textarea aria-label={`Pipeline ${index + 1} prompt`} bind:value={item.prompt} rows="5" placeholder="What this pipeline should build, how to check it, and to raise a pull request. Do not merge." onpaste={pasted}></textarea>
          <div class="stage-pick" role="group" aria-label={`Pipeline ${index + 1} stages`}>
            {#each STAGES as stage (stage.kind)}<button type="button" class="stage-chip" aria-pressed={item.stages?.includes(stage.kind) ?? false} onclick={() => toggleStage(item, stage.kind)}>{stage.label}</button>{/each}
          </div>
          {#if item.stages?.length}<div class="stage-tiers" role="group" aria-label={`Pipeline ${index + 1} stage tiers`}>
            {#each item.stages as kind (kind)}{@const split = stageParts(item, kind)}{#if split.length > 1}
            <div class="stage-split" role="group" aria-label={`${stageLabel(kind)} parts in pipeline ${index + 1}`} data-stage={kind}>
              <p class="split-head"><span class="stage-name">{stageLabel(kind)}</span><span class="split-count">Split into {split.length} parts, done in this order, each by its own agent.</span><button class="link" type="button" disabled={busy} aria-label={`Do ${stageLabel(kind)} in pipeline ${index + 1} as one stage`} onclick={() => item.parts = withoutParts(item, kind)}>Do as one stage</button></p>
              {#each split as part, at (at)}{@const who = crew[part.tier]}<div class="split-part" role="group" aria-label={`Part ${at + 1} of ${stageLabel(kind)} in pipeline ${index + 1}`}>
                <span class="part-no" aria-hidden="true">{at + 1}</span>
                <input aria-label={`Part ${at + 1} name`} value={part.title} maxlength="60" disabled={busy} oninput={event => item.parts = withPart(item, kind, at, { title: event.currentTarget.value })}/>
                <select aria-label={`Part ${at + 1} tier`} value={part.tier} disabled={busy} onchange={event => item.parts = withPart(item, kind, at, { tier: event.currentTarget.value as StagePart['tier'] })}>{#each CREW_TIERS as choice (choice.tier)}<option value={choice.tier}>{choice.label}</option>{/each}</select>
                <button class="link part-remove" type="button" disabled={busy} aria-label={`Remove part ${at + 1} of ${stageLabel(kind)} in pipeline ${index + 1}`} onclick={() => item.parts = withPart(item, kind, at, null)}>Remove</button>
                <textarea aria-label={`Part ${at + 1} brief`} rows="2" maxlength="1000" value={part.brief} disabled={busy} oninput={event => item.parts = withPart(item, kind, at, { brief: event.currentTarget.value })}></textarea>
                <span class="stage-who">{who.tool ? chatToolName(who.tool) : 'No agent'}{who.model.trim() ? ` · ${who.model.trim()}` : ''}{who.effort ? ` · ${who.effort} effort` : ''}{#if part.why}<span class="stage-spare"><strong>Gaffer’s assessment.</strong> {part.why}</span>{/if}</span>
              </div>{/each}
            </div>
            {:else}{@const rated = stageTier(kind, item)}{@const member = crew[rated.tier]}
            <div class="stage-tier" class:changed={rated.changed} data-stage={kind}>
              <span class="stage-name">{stageLabel(kind)}</span>
              <select aria-label={`${stageLabel(kind)} tier in pipeline ${index + 1}`} value={rated.tier} onchange={event => setTier(item, kind, event.currentTarget.value)}>{#each CREW_TIERS as choice (choice.tier)}<option value={choice.tier}>{choice.label}</option>{/each}</select>
              <span class="stage-who">{member.tool ? chatToolName(member.tool) : 'No agent'}{member.model.trim() ? ` · ${member.model.trim()}` : ''}{member.effort ? ` · ${member.effort} effort` : ''}{#if backups[rated.tier].tool}<span class="stage-spare">Backup: {chatToolName(backups[rated.tier].tool)}{backups[rated.tier].model.trim() ? ` · ${backups[rated.tier].model.trim()}` : ''}{backups[rated.tier].effort ? ` · ${backups[rated.tier].effort} effort` : ''}</span>{/if}</span>
              {#if rated.changed}<p class="stage-why"><strong>{rated.by === 'gaffer' ? 'Gaffer’s assessment' : 'Your choice'}.</strong> {rated.why || `The default for ${stageLabel(kind).toLowerCase()} is ${tierLabel(rated.standard)}.`} <button class="link" type="button" aria-label={`Put ${stageLabel(kind)} in pipeline ${index + 1} back on ${tierLabel(rated.standard)}`} onclick={() => setTier(item, kind, rated.standard)}>Use {tierLabel(rated.standard)}</button></p>{/if}
            </div>{/if}{/each}
          </div>{/if}
          {#if true}{@const count = (item.stages ?? []).reduce((sum, kind) => sum + Math.max(1, stageParts(item, kind).length), 0)}{@const splits = (item.stages ?? []).filter(kind => stageParts(item, kind).length > 1).length}<p class="hint">{item.stages?.length ?? 0} {item.stages?.length === 1 ? 'stage' : 'stages'}{splits ? `, ${splits} split into parts: ${count} agent sessions in all, one after another.` : ', each in an agent session of its own.'}</p>{/if}
        </li>{/each}
      </ol>
      {#if sessions.length < cap}<button class="btn quiet add" type="button" onclick={addSession}>Add a pipeline</button>{/if}
      <div class="access" aria-label="Outcome access review"><p>Approving has the controller start {sessions.length === 1 ? 'this pipeline' : `these ${sessions.length} pipelines`} on {project?.machine || 'the project’s machine'} {yolo ? 'on YOLO' : 'with ordinary approvals (no YOLO)'}, one after another, whether or not this page stays open.{#if yolo}{' '}<strong>On YOLO the agents do not wait for you before running commands or changing files, and that includes what Gaffer asks them to do.</strong>{/if}{' '} Gaffer may then read each one’s recent conversation, including code or logs in it, and queue instructions for its working model. If the provider does not yet allow session data, this enables it. Gaffer cannot merge, run commands or answer approvals.</p></div>
      <details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours per session <span>Change limits</span></summary>
        <div class="pair two"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours}/></label></div>
      </details>
      <fieldset class="approvals"><legend>Approvals</legend>
        <div class="choice" role="radiogroup" aria-label="Approvals">
          <label class:on={!yolo}><input type="radio" name={`${formId}-approvals`} checked={!yolo} disabled={busy} onchange={() => yolo = false}/>Ask me</label>
          <label class:on={yolo} class:off={!!yoloWhy}><input type="radio" name={`${formId}-approvals`} checked={yolo} disabled={busy || !!yoloWhy} onclick={event => { if (!yolo) { event.preventDefault(); yoloOpen = true } }}/>YOLO</label>
        </div>
        <p class="hint" class:warn={yolo}>{yoloWhy || (yolo ? 'Every session of this outcome starts on YOLO: no agent waits for you to approve commands or file changes. Each start is checked and audited.' : 'Agents ask before running commands. A pipeline waits whenever one is asking, and carries on once you answer. YOLO needs a confirmation.')}</p>
      </fieldset>
      <fieldset class="approvals"><legend>Decisions</legend>
        <div class="choice" role="radiogroup" aria-label="Decisions">
          <label class:on={!automation}><input type="radio" name={`${formId}-decisions`} checked={!automation} disabled={busy} onchange={() => automation = false}/>Ask me</label>
          <label class:on={automation}><input type="radio" name={`${formId}-decisions`} checked={automation} disabled={busy} onchange={() => automation = true}/>Gaffer decides</label>
        </div>
        <p class="hint">{automation ? 'When a stage stops, Gaffer chooses what happens next (review again, tell the agent, move on, a new budget or a new session) and the Factory records what it chose and why, at most three times for a stage. It also answers an agent’s multiple-choice question. Each decision is one request to Gaffer’s model. Merges, approvals and questions that need your own words still come to you. You can take the decisions back on the outcome’s card.' : 'When a stage stops, the Factory shows you the choices with a recommendation first, and waits for you.'}</p>
      </fieldset>
      <label class="trust"><input type="checkbox" bind:checked={autoReport} disabled={busy}/><span>Have Gaffer report across this outcome by itself when a pipeline finishes or stops, and propose what is missing. Each report is one request to Gaffer’s model, at most six for the outcome; nothing it proposes starts without you.</span></label>
      {#if hasMerge}<label class="trust"><input type="checkbox" bind:checked={autoMerge} disabled={busy}/><span>Merge by itself once every check passes, no review thread is open and there are no conflicts. Left off, each pipeline stops at <strong>Ready to merge</strong> and waits for you.{#if yolo && !yoloWhy}{' '}<strong>On YOLO a pipeline still stops there</strong> until you confirm on the Factory page that it may merge by itself.{/if}</span></label>{/if}
      {#if needsTrust}<label class="trust"><input type="checkbox" bind:checked={trust} disabled={busy}/><span>I trust this folder: {trustNames} may run the plugins and customisations it finds in {project?.dir || 'it'}. {crewTools.includes('antigravity') ? 'Google Antigravity needs this to start at all; OpenCode needs it only in a folder that has plugins.' : 'Leave this off and a folder that has any is refused.'}</span></label>{/if}
      {#if approveBlocker}<p class="warning" role="status">{approveBlocker}</p>
      {:else if !limitsOk}<p class="warning" role="status">Choose a whole number of reviews from 1 to 48 and hours from 1 to 168.</p>{/if}
      {#if startError}<p class="warning" role="alert">{startError}</p>{/if}
      {/if}
      <section class="reply" aria-label="Reply to Gaffer">
        <label for={`${formId}-reply`}>Reply to Gaffer</label>
        <textarea id={`${formId}-reply`} aria-label="Reply to Gaffer" bind:value={reply} rows="3" placeholder="Not that — keep the export in one session and put its tests with it." disabled={planning}></textarea>
        <p class="hint">Gaffer reads this as an answer to the proposal above. Use it when the split or the scope is wrong; the rows are still yours to edit. Up to {MAX_OUTCOME_REPLY_BYTES.toLocaleString('en-GB')} bytes, sent only when you choose.</p>
        {#if replyProblem && reply.trim()}<p class="warning" role="status">{replyProblem}</p>{/if}
        <div class="actions"><button class="btn" type="button" disabled={planning || !!replyProblem} onclick={() => void sendReply()}>{planning ? 'Gaffer is reading your reply…' : 'Send reply'}</button></div>
        {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
      </section>
      <div class="actions">
        {#if declined}<button class="btn" type="button" onclick={() => step = 'objective'}>Back to the outcome</button>
        {:else}<button class="btn primary" type="button" disabled={busy || planning || !!approveBlocker || !limitsOk} onclick={() => void approve()}>{busy ? 'Saving the plan…' : `Approve and start ${sessions.length} ${sessions.length === 1 ? 'pipeline' : 'pipelines'}`}</button><button class="btn quiet" type="button" disabled={busy || planning} onclick={() => declining = true}>Decline proposal</button><button class="btn quiet" type="button" disabled={busy || planning} onclick={() => step = 'objective'}>Back</button>{/if}
      </div>
    {:else if record}
      <p class="objective"><span class="eyebrow">Outcome</span>{record.objective}</p>
      <p role="status" class="result-line">{done ? outcomeSummary(record.sessions) : startingProgress(record.sessions)}</p>
      {#if record.state === 'stopped' && record.note}<p class="hint">{record.note}</p>{/if}
      {#if watchError}<p class="warning" role="alert">{watchError}</p>{/if}
      <ul class="results" aria-label="Outcome sessions">
        {#each record.sessions as item (item.key)}<li class:failed={item.state === 'failed' || item.state === 'unfollowed' || item.state === 'cancelled'}>
          <strong>{item.title}</strong><span>{item.note}</span>
          {#if item.sessionId}<a href={`/sessions?agent=${encodeURIComponent(record.agentId)}&session=${encodeURIComponent(item.sessionId)}`}>Open session</a>{/if}
        </li>{/each}
      </ul>
      {#if done}<p class="hint">Each session has its own goal naming this outcome, and its own budget. The floor groups them under the outcome; only you mark a goal complete.</p>
        <div class="actions"><button class="btn primary" type="button" onclick={onclose}>Back to the Factory</button></div>
      {:else}<div class="actions"><button class="btn" type="button" onclick={onclose}>Close and let it run</button><button class="btn quiet" type="button" disabled={busy} onclick={() => void stop()}>Stop starting sessions</button></div>{/if}
    {/if}
  </div>
</WorkspaceDialog>
{#if yoloOpen && machine}<YoloDialog gates={yoloGates} tool={tool} structured machine={machine.host} enabling oncancel={() => { yoloOpen = false }} onstart={() => { if (yoloCanStart(yoloGates)) yolo = true; yoloOpen = false }}/>{/if}
{#if declining}<ConfirmDialog title="Decline this proposal?" body="Nothing has started. This plan will not be approved or started, and stays here so you can tell Gaffer what you want instead or go back to the outcome." confirmLabel="Decline proposal" oncancel={() => declining = false} onconfirm={() => { declined = true; declining = false }}/>{/if}

<style>
  .outcome{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted)}.warning{color:var(--bad)}
  form{display:grid;gap:12px;min-width:0}label{font-weight:600;display:grid;gap:6px;min-width:0}
  textarea,input,select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .repo-field{display:grid;gap:6px;min-width:0}
  .pair{display:grid;grid-template-columns:1fr 1fr;gap:12px}.pair.two{grid-template-columns:1fr 1fr;padding-top:8px}.source-field{min-width:0}.crew-source{grid-column:1/-1;min-width:0}
  .eyebrow{display:block;font-size:0.875rem;color:var(--muted);margin-bottom:2px}.objective{white-space:pre-wrap}
  .summary,.access{border-left:2px solid var(--line-strong);padding-left:12px}
  .talk{display:grid;gap:12px;padding:12px;border:1px solid var(--line);border-radius:12px;background:var(--bg)}
  .said{display:grid;gap:2px;min-width:0}.said.you p:last-child{white-space:pre-wrap}.said.gaffer{padding-left:12px;border-left:2px solid var(--line-strong)}
  .declined{padding:10px 14px;border:1px solid var(--line);border-radius:10px;background:var(--tint);font-weight:600}
  .reply{display:grid;gap:8px;border-top:1px solid var(--line);padding-top:14px}
  .plan{list-style:none;margin:0;padding:0;display:grid;gap:12px}.plan li{display:grid;gap:8px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}
  .plan-head{display:flex;align-items:center;justify-content:space-between;gap:12px}.link{border:0;background:none;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer;min-height:44px;padding:0 4px}
  .add{justify-self:start}
  .limit-editor summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;font-variant-numeric:tabular-nums}.limit-editor summary span{color:var(--accent-text);margin-left:8px}
  .steps{list-style:none;display:flex;gap:6px 18px;flex-wrap:wrap;margin:0;padding:0 0 12px;border-bottom:1px solid var(--line);color:var(--muted);font-size:0.875rem}.steps li{display:flex;align-items:center;gap:8px}.steps .n{display:inline-grid;place-items:center;width:22px;height:22px;border-radius:50%;border:1px solid var(--line-strong);font-variant-numeric:tabular-nums;font-size:0.875rem}.steps li[aria-current]{color:var(--ink);font-weight:600}.steps li[aria-current] .n,.steps li.done .n{background:var(--accent);border-color:var(--accent);color:var(--accent-ink)}
  .crew-backup{grid-column:1/-1;display:grid;grid-template-columns:subgrid;gap:6px 10px;align-items:start}.crew-slot{align-self:start;line-height:44px;font-size:0.875rem;font-weight:600;color:var(--muted)}.crew-none{grid-column:3/-1;align-self:start;line-height:44px;font-size:0.875rem;color:var(--muted)}.crew-row select{width:100%;min-width:0}
  .balance{display:grid;grid-template-columns:minmax(0,auto) minmax(0,1fr);align-items:center;gap:6px 10px;margin-top:14px;padding-top:14px;border-top:1px solid var(--line)}.balance label{font-weight:600;line-height:1.3}.percent{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px;color:var(--ink)}.percent input{width:5.5rem;min-height:44px;text-align:right;font-variant-numeric:tabular-nums}.balance+.hint{margin-top:8px}@media(max-width:479px){.balance{grid-template-columns:1fr}}
  .stage-spare{display:block;color:var(--muted)}
  .stage-split{display:grid;gap:8px;padding:8px;border:1px solid var(--line);border-radius:8px;background:var(--tint)}.split-head{margin:0;display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 12px}.split-count{color:var(--muted);font-size:0.875rem;flex:1 1 12rem}
  .split-part{display:grid;grid-template-columns:1.75rem minmax(0,1fr) 8rem auto;gap:6px 10px;align-items:center;padding:8px;border:1px solid var(--line-soft);border-radius:8px;background:var(--card)}.part-no{width:1.75rem;height:1.75rem;border-radius:999px;display:grid;place-items:center;font-weight:700;font-size:0.875rem;border:1px solid var(--line);color:var(--muted)}.split-part input,.split-part select{min-height:44px;min-width:0;width:100%}.split-part textarea{grid-column:2/-1;min-width:0;width:100%;resize:vertical}.split-part .stage-who{grid-column:2/-1}.part-remove{min-height:44px}
  @media(max-width:479px){.split-part{grid-template-columns:1.75rem minmax(0,1fr)}.split-part select,.split-part textarea,.split-part .stage-who,.part-remove{grid-column:2}.part-remove{order:5;justify-self:start}}
  .crew-rows{display:grid;gap:10px;padding-top:6px}.crew-row{display:grid;grid-template-columns:5.5rem minmax(0,1fr) minmax(0,1.25fr) minmax(0,1.05fr);gap:8px 10px;align-items:start}.crew-row+.crew-row{padding-top:12px;border-top:1px solid var(--line)}.crew-source{grid-column:2/-1}.crew-role{grid-column:1/-1;display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 8px;min-width:0}.crew-role span{font-size:0.875rem;color:var(--muted);font-weight:400}
  .stage-pick{display:flex;flex-wrap:wrap;gap:6px}.stage-chip{font:inherit;font-size:0.875rem;min-height:36px;padding:4px 12px;border-radius:999px;border:1px solid var(--line);background:var(--bg);color:var(--muted);cursor:pointer}.stage-chip[aria-pressed=true]{background:var(--tint);border-color:var(--accent);color:var(--ink);font-weight:600}
  /* Each chosen stage with its tier and who that means. One the plan or the person moved off its default is marked, with why. */
  .stage-tiers{margin:8px 0 0;display:grid;gap:6px}.stage-tier{display:grid;grid-template-columns:5.5rem 8rem minmax(0,1fr);align-items:center;gap:2px 10px;padding:4px 8px;border:1px solid var(--line-soft);border-radius:8px}
  .stage-tier.changed{border-color:color-mix(in srgb,var(--accent) 45%,var(--line));background:var(--accent-wash)}.stage-name{font-weight:600}.stage-tiers select{min-height:36px;font:inherit;font-size:0.875rem}
  .stage-who{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font-size:0.875rem}.stage-why{grid-column:1/-1;margin:2px 0 0;font-size:0.875rem;color:var(--ink-soft)}
  .crew-save{margin:12px 0 0;display:flex;flex-wrap:wrap;align-items:center;gap:4px 12px;font-size:0.875rem;color:var(--muted)}
  @media(max-width:479px){.crew-row{grid-template-columns:1fr}.crew-slot{line-height:1.4;margin-top:4px}.crew-none,.crew-source{grid-column:1/-1}.stage-tier{grid-template-columns:5rem minmax(0,1fr)}.stage-who{grid-column:1/-1}}@media(pointer:coarse){.stage-chip{min-height:44px}.stage-tiers select{min-height:44px}}
  .approvals{border:0;margin:0;padding:0;display:grid;gap:6px;min-width:0}.approvals legend{font-weight:600;padding:0;margin-bottom:6px}
  .choice{display:inline-flex;border:1px solid var(--line);border-radius:10px;overflow:hidden;justify-self:start}.choice label{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 16px;font-weight:600;cursor:pointer;color:var(--muted)}.choice label+label{border-left:1px solid var(--line)}.choice label.on{background:var(--tint);color:var(--ink)}.choice label.off{cursor:not-allowed;opacity:.6}.choice input{width:18px;height:18px;min-height:0;margin:0;padding:0;accent-color:var(--accent)}
  .hint.warn{color:var(--warn)}
  .result-line{font-weight:600}.trust{display:flex;align-items:flex-start;gap:10px;font-weight:400;min-height:44px}.trust input{width:20px;height:20px;min-height:0;margin:2px 0 0;flex:none;padding:0;accent-color:var(--accent)}
  .results{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.results li{display:grid;gap:2px;padding:10px 14px;min-width:0;overflow-wrap:anywhere}.results li+li{border-top:1px solid var(--line)}.results span{font-size:0.875rem;color:var(--muted)}.results li.failed span{color:var(--bad)}.results a{color:var(--accent-text);font-weight:600;min-height:44px;display:inline-flex;align-items:center;justify-self:start}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,a:focus-visible,summary:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.pair,.pair.two{grid-template-columns:1fr}}
</style>
