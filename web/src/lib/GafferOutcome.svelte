<script lang="ts">
  import Sentence from './Sentence.svelte'
  // Gaffer proposes the sessions, the person edits and approves the plan, and only then does the
  // controller start each session through the ordinary launch and put it in Gaffer's care. This dialog only watches:
  // the work carries on if it is closed. Nothing starts on the model's say-so: a proposed session is text until Approve.
  import { onMount, tick, untrack } from 'svelte'
  import { ApiError, createGafferOutcome, diffGafferTemplate, getGafferCrew, getGafferTemplate, launchGafferTemplate, type Template, type TemplateDiff, type TemplateLaunchInput, type GafferOutcomeInput, saveGafferCrew, type GafferCrew, getGafferOutcome, getGafferSettings, getProfile, getYoloSetting, listGafferPathways, listModelFavourites, planGafferOutcome, type GafferOutcomePlan, type Pathway, type GafferPlanPhase, stopGafferOutcome, type GafferOutcome, type GafferProvider, type YoloSetting } from './api'
  import { chatToolName, launchUpload, listFolderProjects, machineAccounts, machineModels, structuredAgent, terminalMachines, type FolderProject, type TerminalMachine } from './terminal/api'
  import { imageFiles } from './terminal/upload'
  import ImageAttachment from './ImageAttachment.svelte'
  import { MAX_ATTACHMENTS, attachmentMessage, attachmentsFromPaste, type AttachmentReason } from './imageAttachment'
  import GafferModelSelect from './GafferModelSelect.svelte'
  import ClineSourceSelect from './ClineSourceSelect.svelte'
  import { resolveStartSource, sourceOptions } from './clineSource'
  import GafferProse from './GafferProse.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import YoloDialog from './launch/YoloDialog.svelte'
  import { startsFromLatest, yoloCanStart, type YoloGate } from './launchBar'
  import LatestCodeChoice, { type LatestChoice } from './launch/LatestCodeChoice.svelte'
  import { CATALOGUE } from './catalogue'
  import type { MachineModels, ModelFavourite } from './models'
  import { reportedEfforts } from './terminal/launchOptions'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import { readFloorDefaults, writeFloorDefaults } from './gafferFloor'
  import { BUILT_IN_GOAL_PROMPTS } from './gafferPrompts'
  import { CREW_TIERS, DEFAULT_STAGES, defaultTier, MAX_OUTCOME_REPLY_BYTES, MAX_OUTCOME_SESSIONS, STAGES, carryCriteria, cleanCriteria, cleanPrompt, objectiveBlocker, orderStages, outcomeSummary, planBlocker, previousPlan, replyBlocker, stagePlan, stageLabel, stageTier, tierLabel, planRatings, withTier, startingProgress, wholeBetween, withReportedEfforts, type Crew, type PlanSession, type PlanTurn, BALANCE_LOW_PERCENT, BALANCE_MIN_PERCENT, BALANCE_MAX_PERCENT, balancePercent, stageParts, withPart, withoutParts, withNewPart, partsBlocker, stageTakesParts, MAX_BACKUPS, savedCrewRows } from './gafferOutcome'
  import StageParts from './factory/StageParts.svelte'
  import LaunchReadiness from './factory/LaunchReadiness.svelte'
  import TemplateChanges from './factory/TemplateChanges.svelte'
  import TemplatePlan from './factory/TemplatePlan.svelte'
  import Templates from './factory/Templates.svelte'
  import PathwayChoice, { stageLine } from './factory/pathways/PathwayChoice.svelte'
  import { stageTitle } from './factory/pathways'
  import { attemptIds, mayHaveArrived } from './factory/delivery'
  import { changesSince, changesWords, conflictWords, mergeLine, refusedGaps, skippedKeys, tierCrew } from './factory/templates'
  import { reveal } from './dialogShell'
  import { inWords, launchReadiness, missingCrewLine, launchSummary, type ModelsRead } from './factory/launchReadiness'
  import type { MachineAccounts } from './agentAccounts'
  import CriteriaEditor from './factory/CriteriaEditor.svelte'
  import { gafferAutoMerge } from './gafferMotion'
  import RepoField from './home/RepoField.svelte'
  import { cloneForStart } from './cloneForStart'
  import { cloneFolder, repoProject, parseGitHubRepo } from './githubRepo'

  let { room, userId = 0, projectId: wantedProject = '', initialObjective = '', template: initialTemplate, timeZone = '', dateFormat = 'locale', onclose, onstarted }: { room: number; userId?: number; timeZone?: string; dateFormat?: string;
    /** A template to start from: the dialog opens on its plan, read only, with the launch choices still open. */
    template?: { id: string; version: number };
    /** The project the page had chosen when the dialog opened; it starts selected and can be changed. */
    projectId?: string; initialObjective?: string; onclose: () => void; onstarted: () => void | Promise<void> } = $props()
  const formId = $props.id()
  const CHAT_TOOLS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const life = new AbortController()
  let step = $state<'objective' | 'plan' | 'results'>(untrack(() => initialTemplate ? 'plan' : 'objective'))
  // Starting from a template: its plan is read only, and everything that belongs to a launch is chosen here. The skills
  // the person agreed to start without are exactly the gaps they were shown.
  let template = $state<{ id: string; version: number } | null>(untrack(() => initialTemplate ?? null))
  let tpl = $state<Template | null>(null), tplDiff = $state<TemplateDiff | null>(null), tplReading = $state(false), tplError = $state(''), skipOk = $state(false), picking = $state(false), pickingId = $state('')
  const launchIds = attemptIds(), createIds = attemptIds()
  const gaps = $derived(tpl?.skillGaps ?? [])
  // Only the newest read is shown: the version list stays usable while one is read, so the focus stays on it.
  let reads = 0
  async function readTemplate(id: string, version = 0) {
    const asked = ++reads
    tplReading = true; tplError = ''
    try {
      const next = await getGafferTemplate(id, version, life.signal)
      // Compared from the version last used, or the oldest kept when that one has gone: the server chooses.
      const diff = changesSince(next.lastUsedVersion, next.version) ? await diffGafferTemplate(id, 0, next.version, life.signal).catch(() => null) : null
      if (asked !== reads) return
      tplDiff = diff
      tpl = next; template = { id, version: next.version }; skipOk = false
      sessions = next.content.pipelines.map(item => ({ key: key(), title: item.title, prompt: item.prompt, stages: orderStages([...new Set(item.stages.map(stage => stage.kind))]), criteria: item.criteria.map(one => one.text), ...planRatings(item.stages) }))
      step = 'plan'
    } catch (e) { if (!life.signal.aborted && asked === reads) tplError = e instanceof ApiError && e.status === 404 ? 'That template or version is not kept any more.' : e instanceof Error ? e.message : 'The template could not be read.' }
    finally { if (asked === reads) tplReading = false }
  }
  function leaveTemplate() { template = null; tpl = null; tplDiff = null; tplError = ''; sessions = []; skipOk = false; startError = ''; step = 'objective' }
  let loading = $state(true), loadError = $state('')
  let provider = $state<GafferProvider | null>(null), projects = $state<FolderProject[]>([]), machines = $state<TerminalMachine[]>([])
  let objective = $state(untrack(() => initialObjective)), projectId = $state(''), tool = $state(''), wanted = $state(3)
  // Where a plan has got to, as the controller reports it, and how long it has been. A failed plan keeps its step, so the
  // list shows which one failed.
  const PLAN_STEPS: GafferPlanPhase[] = ['sending', 'writing', 'checking']
  let planPhase = $state<GafferPlanPhase | ''>(''), planSeconds = $state(0), planStop: AbortController | undefined, planDone = () => {}
  function planBegin(): AbortSignal {
    planPhase = 'sending'; planSeconds = 0
    planStop = new AbortController()
    const tick = setInterval(() => { planSeconds++ }, 1000)
    planDone = () => clearInterval(tick)
    const signal = AbortSignal.any([life.signal, planStop.signal])
    signal.addEventListener('abort', planDone, { once: true })
    return signal
  }
  const stopPlanning = () => { planStop?.abort(); planPhase = '' }
  const planMark = (at: number) => { const now = PLAN_STEPS.indexOf(planPhase as GafferPlanPhase); return at < now ? '✓' : at > now ? '·' : planning ? '…' : '✕' }
  const planStepText = (step: GafferPlanPhase) => step === 'sending' ? `${planPhase === 'sending' ? 'Sending' : 'Sent'} to ${provider?.name ?? 'the provider'}` : step === 'writing' ? `${provider?.model || 'The model'} is writing the plan` : 'Checking the plan'
  const clock = (seconds: number) => `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
  let planning = $state(false), planError = $state(''), summary = $state(''), sessions = $state<PlanSession[]>([])
  // The reply a person sends to a proposal, and the exchange it belongs to. A declined proposal starts nothing.
  let reply = $state(''), planThread = $state<PlanTurn[]>([]), declined = $state(false), declining = $state(false)
  const replyProblem = $derived(replyBlocker(reply))
  let maxReviews = $state(12), hours = $state(24)
  // The pathway every pipeline follows, or '' for the stages chosen here. A failed read offers none, and the dialog is as it was.
  let pathways = $state<Pathway[]>([]), pathwayId = $state('')
  const pathway = $derived(pathways.find(p => p.id === pathwayId) ?? null)
  // The pathway Gaffer's latest plan suggests, said on its row while it is offered. A plan chooses it only while the person
  // has not chosen a pathway (or Standard stages) themselves in this dialog.
  let suggestion = $state<GafferOutcomePlan['pathway']>(), pathwayTouched = $state(false)
  const suggested = $derived.by(() => {
    const p = pathways.find(p => p.id === suggestion?.id && p.enabled && !p.problems?.length)
    return p && { id: p.id, why: suggestion?.why ?? '' }
  })
  function followPlan(plan?: GafferOutcomePlan) {
    suggestion = plan?.pathway
    if (!pathwayTouched) pathwayId = suggested?.id ?? ''
  }
  const touchPathway = (e: Event) => { if (e.target instanceof HTMLInputElement && e.target.type === 'radio') pathwayTouched = true }
  let trust = $state(false)
  // The model every stage uses unless a crew role is given its own. Empty is the agent's default.
  let model = $state(''), effort = $state('')
  // The Cline billing source: the account's saved default, and the person's own choice for this run.
  let savedSource = $state(''), explicitSource = $state(false), source = $state('')
  let crewSourceExplicit = $state<Record<string, boolean>>({ heavy: false, standard: false, light: false })
  // What each agent offers on the project's machine, read once per agent. A failed read leaves favourites, suggestions and Other.
  let offerFailed = $state<Record<string, boolean>>({})
  let offered = $state<Record<string, MachineModels | null>>({}), favourites = $state<ModelFavourite[]>([])
  // YOLO: the account setting, and whether the person confirmed it for this outcome. It is never remembered.
  let yoloSetting = $state<YoloSetting | null>(null), yolo = $state(false), yoloOpen = $state(false)
  // The crew: which agent and model does the thinking, the building and the shipping stages of every pipeline.
  let crew = $state<Crew>({ heavy: { tool: '', model: '' }, standard: { tool: '', model: '' }, light: { tool: '', model: '' } })
  // Each tier's backup: who does its stages when the first choice's plan is running low. An empty agent is no backup.
  type Spare = { tool: string; model: string; effort: string; source?: string }
  let backups = $state<Record<'heavy' | 'standard' | 'light', Spare>>({ heavy: { tool: '', model: '', effort: '' }, standard: { tool: '', model: '', effort: '' }, light: { tool: '', model: '', effort: '' } })
  // Further backups of a tier, tried in order after the first: up to MAX_BACKUPS in all.
  let more = $state<Record<'heavy' | 'standard' | 'light', Spare[]>>({ heavy: [], standard: [], light: [] })
  // The share of the first choice's plan under which a backup takes over. The person's to set; saved with the crew.
  let balanceBelow = $state<number | string>(BALANCE_LOW_PERCENT)
  const anyBackup = $derived(CREW_TIERS.some(item => !!backups[item.tier].tool))
  const crewSent = $derived<Crew>({ heavy: { ...crew.heavy, backup: backups.heavy, more: more.heavy }, standard: { ...crew.standard, backup: backups.standard, more: more.standard }, light: { ...crew.light, backup: backups.light, more: more.light } })
  let autoMerge = $state(false)
  // Gaffer reports across the outcome by itself when a pipeline finishes or stops. Each report is one model request.
  let autoReport = $state(false)
  // Who takes the decisions when a stage stops: the person, or Gaffer for them. The person's unless they say so.
  let automation = $state(false)
  let busy = $state(false), startError = $state(''), record = $state<GafferOutcome | null>(null), watchError = $state('')
  let images = $state<File[]>([]), imageUploading = $state(false), imageError = $state('')
  let uploaded = $state<{ file: File; host: string; path: string }[]>([])
  let counter = 0
  const key = () => `plan-${++counter}`
  function clearImages() { images = []; uploaded = []; imageError = '' }
  // Every image on the clipboard is added at once, and another paste adds to them.
  function addImages(found: File[], rejected: AttachmentReason[] = []) {
    const room = Math.max(0, MAX_ATTACHMENTS - images.length)
    const accepted = found.slice(0, room)
    if (accepted.length) images = [...images, ...accepted]
    const over = found.length > accepted.length
    imageError = rejected.length ? attachmentMessage(rejected[0]) : over ? `Attach up to ${MAX_ATTACHMENTS} images.` : ''
  }
  // A pasted screenshot becomes part of the outcome's images, exactly as if it had been picked. Text pastes are left alone.
  function pasted(event: ClipboardEvent) {
    if (planning || busy) return
    const data = event.clipboardData
    const files = data?.files.length ? data.files : imageFiles(data?.items)
    const found = attachmentsFromPaste(files)
    if (!found) return
    event.preventDefault()
    addImages(found.files, found.rejected)
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
  // The person's answer for this outcome's first worktrees, or null to use the project's setting, then the account's.
  let latestOverride = $state<boolean | null>(null)
  let accountLatest = $state(false)
  const machine = $derived(project ? machines.find(item => item.agentId === project.host) ?? null : github ? cloners.find(item => item.agentId === ghHost) ?? null : null)
  const ghKnown = $derived(github && machine && ghParsed ? repoProject(projects, { host: machine.agentId, repo: `${ghParsed.owner}/${ghParsed.repo}` }) : undefined)
  const tools = $derived(machine ? CHAT_TOOLS.filter(id => structuredAgent(machine.capabilities, id)) : [])
  const cap = $derived(Math.max(1, Math.min(MAX_OUTCOME_SESSIONS, room)))
  const githubBlocker = $derived(!github ? '' : !cloners.length ? 'No machine here can clone from GitHub yet. It needs a current reet, git and the GitHub CLI.' : !machine ? 'Choose the machine to clone it on.' : !ghParsed ? 'Choose a repository, or type owner/name.' : '')
  const blocker = $derived(loading ? '' : !provider ? 'Choose a default Gaffer provider in Gaffer settings first.' : (objective.trim() && githubBlocker) || objectiveBlocker({ objective: tpl?.content.objective ?? objective, project: !!project || github, machineReady: !!machine?.ready, agent: !!tool && tools.includes(tool), room }))
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
  const spareTools = $derived(CREW_TIERS.flatMap(({ tier }) => [backups[tier], ...more[tier]].map(one => one.tool).filter(Boolean)))
  // Google Antigravity never starts in a folder that was not trusted, so approving a crew with it and no trust would only fail later.
  const sourceOffered = $derived(sourceOptions({ saved: savedSource }).map(item => item.id))
  const startSource = $derived(resolveStartSource({ explicit: explicitSource ? source : undefined, saved: savedSource, offered: sourceOffered }))
  const chosenSource = $derived(startSource.source)
  const sourceShown = $derived(tool === 'cline' && machine?.capabilities?.acpProvider === true)
  // The agents the work runs on (runTools, below).
  const needsTrust = $derived.by(() => runTools.some(id => id === 'opencode' || id === 'antigravity'))
  const trustNames = $derived.by(() => [...new Set(runTools.filter(id => id === 'opencode' || id === 'antigravity'))].map(chatToolName).join(' and '))
  const limitsOk = $derived(wholeBetween(maxReviews, 1, 48) && wholeBetween(hours, 1, 168))
  const done = $derived(!!record && record.state !== 'starting')

  // A model or effort chosen for one agent means nothing to another: when the machine changes the agent, they go with it.
  $effect(() => { if (tools.length && !tools.includes(tool)) { tool = tools[0]; model = ''; effort = '' } })
  $effect(() => { if (wanted > cap) wanted = cap })
  // The crew the person saved for their account is where the dialog starts, for the tiers this machine can run. It is
  // applied once, when the machine's agents are known; after that the crew here is theirs to change for this outcome.
  let savedCrew = $state<GafferCrew | null>(null), crewMachine = '', crewSaving = $state(false), crewNote = $state('')
  // Whether Gaffer settings has a saved crew, and what it holds, is read once when the dialog opens: the summary below
  // shows the saved agent and model for each tier before the person submits, without a second copy of the data.
  let crewRead = $state<'reading' | 'saved' | 'empty' | 'failed'>('reading')
  const savedSettings = $derived(savedCrewRows(savedCrew))
  const unavailableCrew = $derived(savedSettings.filter(item => tools.length > 0 && !tools.includes(item.tool)))
  let chosenRoles = $state<Partial<Record<keyof Crew, boolean>>>({})
  const followsOutcome = (role: keyof Crew) => !chosenRoles[role]
  // The tiers whose first choice the person changed here, so each can be put back as the dialog started it.
  let editedRoles = $state<Partial<Record<keyof Crew, boolean>>>({})
  const savedHere = (tier: keyof Crew) => { const member = savedCrew?.[tier]; return !!member?.tool && tools.includes(member.tool) }
  // A tier's first choice as the dialog starts it: the saved crew's where this machine can run it, else the outcome's own agent.
  function startTier(tier: keyof Crew) {
    const member = savedCrew?.[tier], useSaved = savedHere(tier)
    chosenRoles[tier] = useSaved; editedRoles[tier] = false; droppedEffort[tier] = ''
    crew[tier] = useSaved
      ? { tool: member!.tool!, model: member!.model ?? '', effort: member!.effort ?? '', source: member!.provider ?? '' }
      : { tool, model, effort, source: chosenSource }
    crewSourceExplicit[tier] = useSaved && !!member!.provider
  }
  // The saved default for who decides is where this dialog starts; the person still chooses for this outcome.
  onMount(() => { void getGafferCrew(life.signal).then(answer => { if (life.signal.aborted) return; if (answer.saved) { savedCrew = answer.crew; crewRead = 'saved'; if (answer.crew.gafferDecides) automation = true } else crewRead = 'empty' }).catch(() => { if (!life.signal.aborted) crewRead = 'failed' }) })
  $effect(() => {
    const host = machine?.agentId, available = tools, saved = savedCrew
    if (!host || !available.includes(tool) || crewRead === 'reading' || crewMachine === host) return
    crewMachine = host
    untrack(() => {
      for (const { tier } of CREW_TIERS) {
        const member = saved?.[tier]
        startTier(tier)
        const spares = [member?.backup, ...(member?.moreBackups ?? [])].flatMap(one => one?.tool && available.includes(one.tool) ? [{ tool: one.tool, model: one.model ?? '', effort: one.effort ?? '', source: one.provider ?? '' }] : [])
        backups[tier] = spares[0] ?? { tool: '', model: '', effort: '' }
        more[tier] = spares.slice(1, MAX_BACKUPS)
      }
      if (saved?.balanceBelow) balanceBelow = balancePercent(saved.balanceBelow)
      crewNote = CREW_TIERS.some(({ tier }) => chosenRoles[tier]) ? 'Started from your saved crew.' : ''
    })
  })
  // A tier's backups as the saved crew holds them: the first, then any further ones, each with its Cline source.
  const sentSpare = (one: Spare) => ({ tool: one.tool, model: one.model.trim(), effort: one.effort, ...(one.tool === 'cline' && one.source ? { provider: one.source } : {}) })
  function spareFields(tier: 'light' | 'standard' | 'heavy') {
    const list = [backups[tier], ...more[tier]].filter(one => one.tool).map(sentSpare)
    return list.length ? { backup: list[0], ...(list.length > 1 ? { moreBackups: list.slice(1) } : {}) } : {}
  }
  async function rememberCrew() {
    crewSaving = true; crewNote = ''
    try {
      const member = (tier: 'light' | 'standard' | 'heavy') => ({ tool: crew[tier].tool, model: crew[tier].model.trim(), effort: crew[tier].effort ?? '', ...(crew[tier].tool === 'cline' && crew[tier].source ? { provider: crew[tier].source } : {}), ...spareFields(tier) })
      // What was just saved is what is on screen: there is nothing to apply again.
      savedCrew = (await saveGafferCrew({ light: member('light'), standard: member('standard'), heavy: member('heavy'), balanceBelow: balancePercent(balanceBelow), gafferDecides: !!savedCrew?.gafferDecides })).crew
      crewNote = 'Saved. New outcomes start from this crew.'
    } catch (e) { crewNote = e instanceof Error ? e.message : 'The crew could not be saved.' }
    finally { crewSaving = false }
  }
  // Keep saved and explicitly edited tiers separate from the outcome's fallback.
  $effect(() => {
    const fallback = { tool, model, effort, source: chosenSource }
    for (const { tier } of CREW_TIERS) if (followsOutcome(tier)) crew[tier] = { ...fallback }
  })
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
  $effect(() => {
    const agentId = machine?.agentId
    if (!agentId) return
    for (const id of new Set([tool, ...crewTools, ...spareTools, ...runTools])) {
      const slot = `${agentId}:${id}`
      if (!id || slot in offered) continue
      offered[slot] = null
      void machineModels(agentId, id, life.signal).then(found => { offered[slot] = found }).catch(() => { offerFailed[slot] = true /* suggestions and Other still work */ })
    }
  })
  // Every session here is a chat, so the efforts are the agent's own thinking levels as this machine reports them.
  // A level this machine does not report is never carried: the controller starts the stage on the agent's default effort.
  const efforts = (id: string) => reportedEfforts(id, machine?.capabilities)
  // The effort a crew role asked for (saved, or typed) and the machine does not report: it is cleared above, and kept here so the plan can say so.
  let droppedEffort = $state<Record<string, string>>({})
  const edit = (tier: 'heavy' | 'standard' | 'light') => { chosenRoles[tier] = true; editedRoles[tier] = true; droppedEffort[tier] = '' }
  // A chosen level the agent no longer reports (another project's machine, or a crew role's own agent) goes back to the
  // agent's default, so what is shown is what the outcome will start with.
  // Nothing is cleared while the machine's capabilities are not known: a guess would only lose the person's choice.
  $effect(() => {
    if (!machine?.capabilities) return
    if (effort && !efforts(tool).includes(effort)) effort = ''
    for (const item of CREW_TIERS) {
      const role = crew[item.tier]
      if (role.effort && !efforts(role.tool).includes(role.effort)) { droppedEffort[item.tier] = role.effort; crew[item.tier] = { ...role, effort: '' } }
    }
  })
  const offeredFor = (id: string) => machine ? offered[`${machine.agentId}:${id}`] ?? null : null
  // What each tier will run on this machine, before the person approves: the saved crew's entry, what the machine reports
  // and its sign-ins (read once per machine when the plan opens; a failed read is a state, not an error).
  // The machine list is read again when the plan opens: the person may have taken a while, and a machine that went away
  // or lost an agent in the meantime must stop Approve now, not after the controller has refused each stage. While a
  // stage cannot start it is read every ten seconds, so a machine that comes back clears the block; no reads otherwise.
  const readMachines = () => void terminalMachines(life.signal).then(found => { if (!life.signal.aborted) machines = found }).catch(() => { /* the list read before stands */ })
  $effect(() => { if (step === 'plan') readMachines() })
  $effect(() => {
    if (step !== 'plan' || !launch.blocked) return
    const timer = setInterval(readMachines, 10_000)
    return () => clearInterval(timer)
  })
  let accounts = $state<Record<string, MachineAccounts | null>>({})
  const askedAccounts = new Set<string>()
  // A machine that is busy, or a request that failed, is asked once more a moment later, and not again.
  function askAccounts(id: string, again: boolean) {
    const retry = () => { if (again) setTimeout(() => { if (!life.signal.aborted) askAccounts(id, false) }, 3000) }
    void machineAccounts(id, life.signal).then(found => { accounts[id] = found; if (found.state === 'busy') retry() }).catch(() => { if (!life.signal.aborted) { accounts[id] = null; retry() } })
  }
  $effect(() => {
    const id = machine?.agentId
    if (step !== 'plan' || !id || askedAccounts.has(id)) return
    askedAccounts.add(id)
    askAccounts(id, true)
  })
  const savedFor = (tier: 'heavy' | 'standard' | 'light') => crewRead === 'failed' || crewRead === 'reading' ? undefined : crewRead === 'saved' && savedCrew?.[tier]?.tool ? savedCrew[tier] : null
  const modelsFor = (id: string): ModelsRead => { const slot = machine ? `${machine.agentId}:${id}` : ''; return !slot ? null : offered[slot] ?? (offerFailed[slot] ? null : 'reading') }
  // A tier as the payload will carry it: the same dedupe and the same effort strip, so a row never lists a backup or an effort that is not sent.
  const sentFor = (tier: 'heavy' | 'standard' | 'light') => withReportedEfforts(stagePlan(['code'], crewSent, machine?.capabilities?.acpProvider === true, { tiers: { code: tier } }), machine?.capabilities ? efforts : null)[0]
  const tierReady = $derived.by(() => {
    const now = Date.now()
    return Object.fromEntries(CREW_TIERS.map(({ tier }) => {
      const one = sentFor(tier), asked = one.effort || (chosenRoles[tier] ? droppedEffort[tier] : '') || ''
      const spares = [one.backup, ...(one.moreBackups ?? [])].flatMap(spare => spare ? [{ tool: spare.tool, model: spare.model ?? '', effort: spare.effort ?? '' }] : [])
      return [tier, launchReadiness({ machine, tools, efforts, models: modelsFor, accounts: machine ? accounts[machine.agentId] : undefined, now, member: { tool: one.tool, model: one.model, effort: asked }, spares, saved: savedFor(tier), edited: !!chosenRoles[tier] })]
    })) as Record<'heavy' | 'standard' | 'light', ReturnType<typeof launchReadiness>>
  })
  // With a pathway, what each of its stages runs on, as the controller resolves it (outcomePieces): the stage's own agent
  // with no backups, else its tier's saved crew member with that member's backups, whether or not this machine has them,
  // else (a tier not saved) the outcome's own agent. The crew edited here is not sent.
  const pathwayRuns = $derived(!pathway ? [] : pathway.stages.map(stage => {
    if (stage.agent) return { stage, tier: null, saved: false, member: { tool: stage.agent.tool, model: stage.agent.model ?? '', effort: stage.agent.effort ?? '' }, spares: [] }
    const tier = (stage.tier ?? defaultTier(stage.kind)) as keyof Crew, member = savedCrew?.[tier]
    if (!member?.tool) return { stage, tier, saved: false, member: { tool, model, effort }, spares: [] }
    const spares = [member.backup, ...(member.moreBackups ?? [])].flatMap(one => one?.tool ? [{ tool: one.tool, model: one.model ?? '', effort: one.effort ?? '' }] : [])
    return { stage, tier, saved: true, member: { tool: member.tool, model: member.model ?? '', effort: member.effort ?? '' }, spares }
  }))
  // The stages each pipeline sends under standard stages; the start's body is built from these.
  const sentStages = $derived(sessions.map(item => withReportedEfforts(stagePlan(item.stages ?? DEFAULT_STAGES, crewSent, machine?.capabilities?.acpProvider === true, item), machine?.capabilities ? efforts : null)))
  const agentsOf = (plan: { tool: string; backup?: { tool: string }; moreBackups?: { tool: string }[] }[]) => plan.flatMap(one => [one.tool, one.backup?.tool ?? '', ...(one.moreBackups ?? []).map(spare => spare.tool)]).filter(Boolean)
  // Every agent the start can use, as the controller counts them for folder trust and YOLO: the outcome's own agent, and the
  // first choice and backups of each stage it sends, so a tier no stage uses counts for nothing. A template's stages run on
  // the crew sent for their tiers.
  const runTools = $derived([...new Set([tool, ...(pathway ? pathwayRuns.flatMap(run => [run.member.tool, ...run.spares.map(one => one.tool)])
    : template && tpl ? [...new Set(tpl.content.pipelines.flatMap(item => item.stages.map(stage => stage.tier)))].flatMap(tier => CREW_TIERS.some(one => one.tier === tier) ? agentsOf([sentFor(tier as 'heavy' | 'standard' | 'light')]) : [])
    : sentStages.flatMap(agentsOf))])])
  const pathwayLaunch = $derived.by(() => {
    if (!pathway) return null
    const now = Date.now(), held = machine ? accounts[machine.agentId] : undefined
    const summary = launchSummary(pathwayRuns.map(run => ({ stage: stageTitle(run.stage), crew: run.tier ? tierLabel(run.tier) : chatToolName(run.member.tool),
      readiness: launchReadiness({ machine, tools, efforts, models: modelsFor, accounts: held, now, member: run.member, spares: run.spares, saved: run.tier ? savedFor(run.tier) : undefined, edited: run.saved || !run.tier }) })))
    // There is no crew to change here: what holds a stage is fixed where the stage is, in the pathway, the saved crew or on the machine.
    const first = summary.causes.find(cause => cause.level === 'blocked')
    const away = first && !first.machine ? pathwayRuns.find(run => run.saved && first.stages.includes(stageTitle(run.stage)) && !tools.includes(run.member.tool)) : undefined
    const blocker = !first ? '' : first.machine ? first.text : away ? missingCrewLine(tierLabel(away.tier!), chatToolName(away.member.tool), pathwayRuns.filter(run => run.tier === away.tier && run.saved).map(run => stageTitle(run.stage)), machine?.host || 'this machine')
      : `${first.text} This pathway’s ${inWords(first.stages)} ${first.stages.length === 1 ? 'stage runs' : 'stages run'} on it. Install it there, or change the stage in the pathway.`
    return { ...summary, blocker }
  })
  // Every stage, and every part of a split stage, with the readiness of the tier it is on.
  const launch = $derived(pathwayLaunch ?? launchSummary(sessions.flatMap(item => (item.stages ?? []).flatMap(kind => {
    const split = stageParts(item, kind), stage = stageLabel(kind)
    return split.length > 1 ? split.map(part => ({ stage, crew: tierLabel(part.tier), readiness: tierReady[part.tier] })) : [{ stage, crew: tierLabel(stageTier(kind, item).tier), readiness: tierReady[stageTier(kind, item).tier] }]
  }))))
  const approveBlocker = $derived(crewRead === 'reading' ? 'Reading your saved Factory crew…' : (template && blocker) || (pathway ? planBlocker(sessions.map(item => ({ ...item, stages: undefined })), room) : planBlocker(sessions, room) || partsBlocker(sessions)) || launch.blocker || (template && gaps.length && !skipOk ? 'Tick “Start without these skills” to go on, or choose another version.' : '') || (runTools.includes('antigravity') && !trust ? `Google Antigravity starts only in a folder you trust. Tick the folder trust box below, or ${pathway ? 'change the stage in the pathway' : 'choose another agent'}.` : ''))
  // A machine that is only locked is waiting, not failing: the hold is said quietly.
  const waitOnly = $derived(!!approveBlocker && approveBlocker === launch.blocker && launch.waiting === launch.blocked)
  // The status by Approve is one element for good, so that a blocker clearing is announced as well as one appearing.
  let blockerSeen = $state(false)
  $effect(() => { if (step !== 'plan') blockerSeen = false; else if (approveBlocker) blockerSeen = true })
  let crewEditor = $state<HTMLDetailsElement>()
  // Opens the crew editor and puts the person on the first affected tier's agent.
  async function chooseWho(crews: string[]) {
    if (!crewEditor) return
    crewEditor.open = true
    await tick()
    const agent = crewEditor.querySelector<HTMLSelectElement>(`select[aria-label="${crews[0]} agent"]`)
    // Scrolls the dialog's own body only: scrollIntoView would move the dialog frame too and cut off its title.
    const frame = agent?.closest('dialog')
    if (agent && frame) reveal(agent, frame)
    agent?.focus({ preventScroll: true })
  }
  const yoloGates = $derived.by((): YoloGate[] => {
    const account = yoloSetting?.allowed === true
    const host = machine?.host || 'This machine'
    const without = [...new Set(runTools.filter(id => id && !CATALOGUE.find(agent => agent.id === id)?.yolo))].map(chatToolName)
    return [
      { label: 'Your account allows YOLO', ok: account, detail: account ? (yoloSetting?.changedBy ? `set by ${yoloSetting.changedBy}` : 'set by an owner') : yoloSetting ? 'An owner or admin can allow YOLO for this account in Settings.' : 'Reading the account setting…' },
      { label: `${host} accepts YOLO sessions`, ok: !!machine?.capabilities?.yolo, detail: machine?.capabilities?.yolo ? 'reported by the machine' : `${host} doesn’t accept YOLO sessions.` },
      { label: pathway ? 'Every agent this pathway uses has a YOLO option' : 'Every agent this start can use has a YOLO option', ok: !without.length, detail: without.length ? `${without.join(' and ')} has no option reet can use for that.` : runTools.filter(Boolean).length ? [...new Set(runTools.filter(Boolean))].map(chatToolName).join(', ') : 'No agent chosen' },
    ]
  })
  const launchesInWorktree = $derived(!!project?.isRepo && !!machine?.capabilities?.worktree)
  const latestChoice = $derived<LatestChoice>(latestOverride === null ? (startsFromLatest(project, { pullLatestByDefault: accountLatest }) ? 'github' : 'machine') : latestOverride ? 'github' : 'machine')
  const yoloWhy = $derived(yoloGates.find(gate => !gate.ok)?.detail ?? '')
  // The template's pipelines that merge by themselves, named beside Start rather than in the access paragraph.
  const merging = $derived(template && tpl ? mergeLine(tpl.content.pipelines.filter(item => item.autoMerge).map(item => item.title), yolo && !yoloWhy) : '')
  $effect(() => { if (yoloWhy) yolo = false })
  $effect(() => { void projectId; latestOverride = null })

  onMount(() => {
    const saved = readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text)
    maxReviews = saved.maxReviews; hours = saved.hours
    autoMerge = $gafferAutoMerge
    void getYoloSetting().then(found => { yoloSetting = found }).catch(() => { yoloSetting = { allowed: false } })
    void getProfile().then(answer => { if (!life.signal.aborted) { savedSource = answer.profile.defaultClineSource ?? ''; accountLatest = answer.profile.pullLatestByDefault === true } }).catch(() => { /* Cline’s own choice */ })
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => { /* a helper only */ })
    // A plan that came back before the list is followed once the list is here, unless the person has chosen since.
    void listGafferPathways(life.signal).then(found => { pathways = found; if (!pathwayTouched && !pathwayId) pathwayId = suggested?.id ?? '' }).catch(() => { /* no pathways to offer */ })
    void Promise.all([getGafferSettings(life.signal), listFolderProjects(life.signal), terminalMachines(life.signal)]).then(([settings, folders, found]) => {
      provider = settings.available ? settings.providers.find(item => item.isDefault && item.enabled) ?? null : null
      projects = folders; machines = found
      if (folders.some(item => item.id === wantedProject)) projectId = wantedProject
      else if (folders.length === 1) projectId = folders[0].id
    }).catch(e => { if (!life.signal.aborted) loadError = e instanceof Error ? e.message : 'Projects and machines could not be read.' })
      .finally(() => { loading = false })
    if (initialTemplate) void readTemplate(initialTemplate.id, initialTemplate.version)
    return () => life.abort()
  })

  // Choosing the main source applies it to every stage that follows the main agent, unless a crew role has its own.
  function pickSource(next: string) {
    source = next; explicitSource = true
    for (const item of CREW_TIERS) {
      if (!followsOutcome(item.tier) || crew[item.tier].tool !== tool) continue
      crew[item.tier] = { ...crew[item.tier], source: next }
      crewSourceExplicit[item.tier] = true
    }
  }
  function pickCrewSource(role: keyof Crew, next: string) {
    chosenRoles[role] = true; editedRoles[role] = true
    crew[role] = { ...crew[role], source: next }
    crewSourceExplicit[role] = true
  }

  async function ask() {
    if (planning || cloning || blocker || !provider) return
    if (!(await ensureProject()) || !project) return
    planning = true; planError = ''; reply = ''; planThread = []; declined = false
    const signal = planBegin()
    try {
      const plan = await planGafferOutcome(provider, { objective: objective.trim(), project: project.name, sessions: wanted }, signal, phase => { planPhase = phase })
      summary = plan.summary
      sessions = plan.sessions.map(item => ({ key: key(), title: item.title, prompt: item.prompt, stages: item.stages?.length ? orderStages([...new Set(item.stages.map(stage => stage.kind))]) : [...DEFAULT_STAGES], ...planRatings(item.stages) }))
      followPlan(plan)
      step = 'plan'; planPhase = ''
    } catch (e) { if (!signal.aborted) planError = e instanceof Error ? e.message : 'Gaffer could not plan this outcome.' }
    finally { planning = false; planDone() }
  }
  // Without a tool-capable model, or by choice, the person writes the plan; the rest of the flow is the same.
  async function writeOwn() {
    if (cloning || blocker) return
    if (!(await ensureProject())) return
    summary = ''; sessions = [{ key: key(), title: '', prompt: '', stages: [...DEFAULT_STAGES] }]; planError = ''; reply = ''; planThread = []; declined = false; followPlan(); step = 'plan'
  }
  // A reply is sent back to Gaffer with the proposal it answers, and the revised plan replaces the rows. The person's
  // words stay in the review as part of the exchange; nothing is saved or started until the existing approve action.
  async function sendReply() {
    if (planning || !provider || !project || replyBlocker(reply)) return
    const text = reply.trim()
    const thread: PlanTurn[] = planThread.length ? planThread : summary ? [{ from: 'gaffer', text: summary }] : []
    planThread = [...thread, { from: 'you', text }]
    planning = true; planError = ''; reply = ''
    const signal = planBegin()
    try {
      const plan = await planGafferOutcome(provider, { objective: objective.trim(), project: project.name, sessions: wanted, reply: text, previous: previousPlan(summary, sessions) }, signal, phase => { planPhase = phase })
      summary = plan.summary
      // Criteria the person typed are theirs, not Gaffer's: they stay with the pipeline of the same title, else the one in the same place.
      const typed = sessions.map(old => ({ title: old.title.trim(), criteria: old.criteria }))
      sessions = plan.sessions.map((item, at) => ({ key: key(), title: item.title, prompt: item.prompt, stages: item.stages?.length ? orderStages([...new Set(item.stages.map(stage => stage.kind))]) : [...DEFAULT_STAGES], ...planRatings(item.stages), ...carryCriteria(typed, item.title, at) }))
      planThread = [...planThread, { from: 'gaffer', text: plan.summary }]
      // A reply's plan rarely repeats the pathway (the model is not told which it suggested): naming none changes nothing.
      if (plan.pathway) followPlan(plan)
      declined = false; planPhase = ''
    } catch (e) {
      planThread = planThread.slice(0, -1)
      reply = text
      if (!signal.aborted) planError = e instanceof Error ? e.message : 'Gaffer could not answer your reply.'
    } finally { planning = false; planDone() }
  }
  const addSession = () => { sessions = [...sessions, { key: key(), title: '', prompt: '', stages: [...DEFAULT_STAGES] }] }
  const setTier = (item: PlanSession, kind: string, tier: string) => { const next = withTier(item, kind, tier); item.tiers = next.tiers; item.whys = next.whys }
  const toggleStage = (item: PlanSession, kind: string) => { const now = item.stages ?? []; item.stages = now.includes(kind) ? now.filter(k => k !== kind) : orderStages([...now, kind]) }
  const hasMerge = $derived(pathway ? pathway.stages.some(stage => stage.kind === 'merge') : sessions.some(item => item.stages?.includes('merge')))
  const removeSession = (id: string) => { sessions = sessions.filter(item => item.key !== id) }
  // The controller owns the start from here. This only reads its record until every session has an answer.
  let watching: ReturnType<typeof setInterval> | undefined, polling = false
  function watch(id: string) {
    clearInterval(watching)
    watching = setInterval(() => {
      // One read at a time: a slow older answer must not land after a newer one and put the dialog back to Starting.
      if (document.hidden || polling) return
      polling = true
      void getGafferOutcome(id, life.signal).then(next => {
        record = next; watchError = ''
        if (next.state !== 'starting') { clearInterval(watching); void onstarted() }
      }).catch(e => { if (!life.signal.aborted) watchError = e instanceof Error ? e.message : 'The outcome could not be read.' }).finally(() => { polling = false })
    }, 1500)
  }
  // Uploads every selected image by itself. One that fails keeps the others:
  // what uploaded travels with the outcome, and the rest is named on the result.
  async function uploadImages(): Promise<{ paths: string[]; failed: string[] } | null> {
    const host = project?.host
    if (!host) return null
    const results: { file: File; host: string; path: string }[] = []
    const failed: string[] = []
    for (const file of images) {
      // A path only means something on the machine it was uploaded to: after a
      // change of project the image is uploaded again rather than reused.
      const known = uploaded.find(item => item.file === file && item.host === host)
      if (known) { results.push(known); continue }
      imageUploading = true
      try {
        const next = await launchUpload(host, file, life.signal)
        const item = { file, host, path: next.path }
        uploaded = [...uploaded, item]
        results.push(item)
      } catch (e) {
        if (life.signal.aborted) return null
        failed.push(file.name)
      } finally { imageUploading = false }
    }
    if (images.length && !results.length) {
      imageError = images.length === 1 ? 'The image could not be uploaded.' : 'The images could not be uploaded.'
      return null
    }
    return { paths: results.map(item => item.path), failed }
  }
  async function approve() {
    if (busy || approveBlocker || !limitsOk || !provider || !project || !tool) return
    busy = true; startError = ''; imageError = ''
    const upload = await uploadImages()
    if (upload === null) { busy = false; return }
    writeFloorDefaults(userId, { ...readFloorDefaults(userId, BUILT_IN_GOAL_PROMPTS[0].text), maxReviews, hours })
    const body: Omit<GafferOutcomeInput, 'requestId'> = {
      objective: objective.trim(), ...(pathway ? { pathway: pathway.id } : {}), projectId: project.id, tool, model: model.trim(), provider: sourceShown ? chosenSource : '', trust: trust && needsTrust, yolo: yolo && !yoloWhy, autoReport, automation, balanceBelow: balancePercent(balanceBelow), autoMerge: autoMerge && hasMerge, maxReviews, hours, confirmed: true, ...(upload.paths.length ? { images: upload.paths } : {}),
      sessions: sessions.map((item, at) => ({ title: item.title.trim(), prompt: cleanPrompt(item.prompt), ...(cleanCriteria(item.criteria).length ? { criteria: cleanCriteria(item.criteria) } : {}), ...(pathway ? {} : { stages: sentStages[at] }) })),
      ...(latestOverride === null ? {} : { pullLatest: latestOverride ? 'yes' as const : 'no' as const }),
    }
    try {
      record = await createGafferOutcome({ ...body, requestId: createIds.idFor(JSON.stringify(body)) }, life.signal)
      createIds.clear(); step = 'results'
      const failed = upload.failed.length ? `${upload.failed.join(', ')} could not be uploaded; ${upload.paths.length === 1 ? 'the other image was' : `the other ${upload.paths.length} images were`} attached.` : ''
      clearImages()
      imageError = failed
      watch(record.id)
      await onstarted()
    } catch (e) {
      if (life.signal.aborted) return
      if (!mayHaveArrived(e)) createIds.clear()
      startError = e instanceof Error ? e.message + (mayHaveArrived(e) ? ' It may not have arrived. Trying again is safe: it starts once.' : '') : 'The outcome was not saved. Nothing was started.'
    } finally { busy = false }
  }
  async function startFromTemplate() {
    const one = tpl
    if (busy || approveBlocker || !provider || !project || !tool || !one || (gaps.length && !skipOk)) return
    busy = true; startError = ''
    const skipped = skippedKeys(gaps)
    // The crew is sent tier by tier exactly as the readiness above reads it, so what is shown is what starts.
    const crewChosen = { light: tierCrew(sentFor('light')), standard: tierCrew(sentFor('standard')), heavy: tierCrew(sentFor('heavy')) }
    const body: Omit<TemplateLaunchInput, 'requestId'> = { version: one.version, projectId: project.id, tool, model: model.trim(), provider: sourceShown ? chosenSource : '', trust: trust && needsTrust, yolo: yolo && !yoloWhy, autoReport, automation, balanceBelow: balancePercent(balanceBelow), crew: crewChosen, confirmed: true, ...(skipped.length ? { skippedSkills: skipped } : {}), ...(latestOverride === null ? {} : { pullLatest: latestOverride ? 'yes' as const : 'no' as const }) }
    try {
      record = await launchGafferTemplate(one.id, { ...body, requestId: launchIds.idFor(JSON.stringify(body)) }, life.signal)
      launchIds.clear(); step = 'results'; watch(record.id); await onstarted()
    } catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : ''
      if (!mayHaveArrived(e)) launchIds.clear()
      // The skills changed since they were shown: the refusal carries them as they are now. Every other choice stays.
      if (code === 'template_skills_changed') {
        const now = refusedGaps(e instanceof ApiError ? e.body : undefined)
        if (now && tpl) { tpl = { ...tpl, skillGaps: now }; skipOk = false } else await readTemplate(one.id, one.version)
        startError = 'The approved skills changed while you were looking. The plan above now shows the current list. Check it and start again.'
      }
      else startError = e instanceof Error && e.message ? conflictWords(code, e.message) + (mayHaveArrived(e) ? ' It may not have arrived. Trying again is safe: it starts once.' : '') : 'The outcome was not started. Nothing was started.'
    } finally { busy = false }
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

<WorkspaceDialog title="New Factory outcome" {onclose}>
  <div class="outcome">
    <ol class="steps" aria-label="Steps">
      {#each [['objective', template ? 'Template' : 'Outcome'],['plan','Plan'],['results','Start']] as [id,name],index (id)}<li aria-current={step===id?'step':undefined} class:done={['objective','plan','results'].indexOf(step)>index}><span class="n">{index+1}</span>{name}</li>{/each}
    </ol>
    {#if step === 'objective'}
      <p>Describe the result you want. Gaffer can draft the plan for your approval. Once you start it, track the work in Factory.</p>
      <p><button class="link" type="button" onclick={() => { pickingId = ''; picking = true }}>Start from a template</button></p>
      {#if loading}<p role="status">Reading your projects…</p>
      {:else if loadError}<p class="warning" role="alert">{loadError}</p>
      {:else}
        <form onsubmit={event => { event.preventDefault(); void ask() }}>
          <label for={`${formId}-objective`}>Outcome</label>
          <textarea id={`${formId}-objective`} bind:value={objective} rows="4" maxlength="4000" placeholder="Ship CSV export for billing: an API endpoint, a button on the billing page, and tests." disabled={planning} onpaste={pasted}></textarea>
          <ImageAttachment bind:files={images} max={MAX_ATTACHMENTS} label="Attach an image to the outcome" disabled={planning} busy={imageUploading} error={imageError} />
          {@render where()}
          {@render savedCrewBlock()}
          {@render crewDetails()}
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
          {@render planProgress()}
          {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
          <div class="actions"><button class="btn primary" type="submit" disabled={planning || cloning || !!blocker}>{cloning ? 'Cloning…' : planning ? 'Gaffer is planning…' : 'Ask Gaffer for a plan'}</button><button class="btn quiet" type="button" disabled={planning || cloning || !!blocker} onclick={() => void writeOwn()}>Write the plan myself</button></div>
        </form>
      {/if}
    {:else if step === 'plan' && template}
      {#if !tpl}
        {#if tplError}<p class="warning" role="alert">{tplError}</p><div class="actions"><button class="btn" type="button" onclick={() => template && void readTemplate(template.id, template.version)}>Try again</button><button class="btn quiet" type="button" onclick={leaveTemplate}>Back</button></div>
        {:else}<p role="status">Reading the template…</p>{/if}
      {:else}
        <p><strong>{tpl.content.name}</strong>, version {tpl.version}{tpl.versions.length > 1 ? ` of ${tpl.latest}` : ''}. The plan is as you saved it. Choose where it runs, then start.</p>
        <p class="objective"><span class="eyebrow">Outcome</span>{tpl.content.objective}</p>
        {#if tpl.versions.length > 1}<label>Template version<select aria-label="Template version" value={tpl.version} disabled={busy} onchange={event => void readTemplate(tpl!.id, Number(event.currentTarget.value))}>
          {#each [...tpl.versions].reverse() as n (n)}<option value={n}>Version {n}{n === tpl.latest ? ' (latest)' : ''}{n === tpl.lastUsedVersion ? ' (last used)' : ''}</option>{/each}
        </select></label>{/if}
        {#if tplError}<p class="warning" role="alert">{tplError}</p>{/if}
        {#if tplDiff}{@const said = changesWords(tplDiff, tpl.lastUsedVersion)}<TemplateChanges diff={tplDiff} title={said.title} note={said.note} oncompare={() => { pickingId = tpl!.id; picking = true }}/>{/if}
        <TemplatePlan content={tpl.content} {gaps} diff={tplDiff} lines idPrefix={`${formId}-`}/>
        {#if gaps.length}<label class="trust consent"><input type="checkbox" bind:checked={skipOk} disabled={busy} aria-describedby={gaps.map(gap => `${formId}-gap-${gap.key}`).join(' ')}/><span><strong>Start without these skills.</strong> {gaps.length === 1 ? 'The skill' : `The ${gaps.length} skills`} marked in the plan above {gaps.length === 1 ? 'is' : 'are'} left out of {gaps.length === 1 ? 'its stage' : 'their stages'}. Nothing else changes.</span></label>{/if}
        {@render where()}
        {@render savedCrewBlock()}
        {@render crewDetails()}
        {@render readinessPanel()}
        {@render launchChoices()}
        {#if merging}<p class="merging" id={`${formId}-merging`}>{merging}</p>{/if}
        <p class="status" class:warning={!!approveBlocker && !waitOnly} class:hint={!approveBlocker || waitOnly} id={`${formId}-blocker`} role="status"><Sentence text={approveBlocker || (blockerSeen ? 'Nothing is holding the plan now.' : '')} /></p>
        {#if startError}<p class="warning" role="alert">{startError}</p>{/if}
        <div class="actions"><button class="btn primary" type="button" disabled={busy || tplReading || !!approveBlocker} aria-describedby={[merging ? `${formId}-merging` : '', approveBlocker ? `${formId}-blocker` : ''].filter(Boolean).join(' ') || undefined} onclick={() => void startFromTemplate()}>{busy ? 'Starting…' : `Start ${sessions.length} ${sessions.length === 1 ? 'pipeline' : 'pipelines'} from this template`}</button><button class="btn quiet" type="button" disabled={busy} onclick={leaveTemplate}>Back</button></div>
      {/if}
    {:else if step === 'plan'}
      <p class="objective"><span class="eyebrow">Outcome</span>{objective.trim()}</p>
      <!-- A change of radio, or a click on one (the checked suggestion included), is the person's own choice; Tab past is not. -->
      {#if !declined}<div class="pathway-choice" onclick={touchPathway} onchange={touchPathway} role="presentation"><PathwayChoice bind:pathways bind:value={pathwayId} {suggested}/></div>{/if}
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
      <p class="hint">{project?.name}. Each row below is a pipeline: edit its brief{pathway ? '' : ', and tick the stages it runs through'}. The first stage starts with the prompt shown{project?.isRepo ? ', in its own worktree' : ''}; later stages take over in the same folder.</p>
      <ImageAttachment bind:files={images} max={MAX_ATTACHMENTS} label="Attach an image to the outcome" disabled={busy} busy={imageUploading} error={imageError} />
      {#if images.length}<p class="hint" role="status">{images.length === 1 ? 'The attached image travels' : `All ${images.length} attached images travel`} with every pipeline this outcome starts.</p>{/if}
      {#if !pathway}{@render crewDetails()}{/if}
      {@render readinessPanel()}
      <ol class="plan" aria-label="Planned sessions">
        {#each sessions as item, index (item.key)}<li>
          <div class="plan-head"><label for={`${formId}-${item.key}-title`}>Pipeline {index + 1}</label><button class="link" type="button" aria-label={`Remove pipeline ${index + 1}`} onclick={() => removeSession(item.key)}>Remove</button></div>
          <input id={`${formId}-${item.key}-title`} aria-label={`Pipeline ${index + 1} title`} bind:value={item.title} maxlength="80" placeholder="Short title"/>
          <textarea aria-label={`Pipeline ${index + 1} prompt`} bind:value={item.prompt} rows="5" placeholder="What this pipeline should build, how to check it, and to raise a pull request. Do not merge." onpaste={pasted}></textarea>
          {#if pathway}
          {@render stageLine(pathway.name, pathway.stages)}
          <p class="hint">Each pipeline follows this pathway. Its stages, models, skills and prompts come from it.</p>
          {:else}
          <div class="stage-pick" role="group" aria-label={`Pipeline ${index + 1} stages`}>
            {#each STAGES as stage (stage.kind)}<button type="button" class="stage-chip" aria-pressed={item.stages?.includes(stage.kind) ?? false} onclick={() => toggleStage(item, stage.kind)}>{stage.label}</button>{/each}
          </div>
          {#if item.stages?.length}<div class="stage-tiers" role="group" aria-label={`Pipeline ${index + 1} stage tiers`}>
            {#each item.stages as kind (kind)}{@const split = stageParts(item, kind)}{#if split.length > 1}
            <StageParts {kind} where={`pipeline ${index + 1}`} parts={split} {busy} ready={tier => tierReady[tier]} who={tier => { const one = crew[tier]; return `${one.tool ? chatToolName(one.tool) : 'No agent'}${one.model.trim() ? ` · ${one.model.trim()}` : ' · Agent’s default model'}${one.effort ? ` · ${one.effort} effort` : ''}` }}
              onchange={(at, next) => item.parts = withPart(item, kind, at, next)} onadd={() => item.parts = withNewPart(item, kind)} onjoin={() => item.parts = withoutParts(item, kind)}/>
            {:else}{@const rated = stageTier(kind, item)}{@const member = crew[rated.tier]}
            <div class="stage-tier" class:changed={rated.changed} data-stage={kind}>
              <span class="stage-name">{stageLabel(kind)}</span>
              <select aria-label={`${stageLabel(kind)} tier in pipeline ${index + 1}`} value={rated.tier} onchange={event => setTier(item, kind, event.currentTarget.value)}>{#each CREW_TIERS as choice (choice.tier)}<option value={choice.tier}>{choice.label}</option>{/each}</select>
              <LaunchReadiness ready={tierReady[rated.tier]} label={`${stageLabel(kind)} in pipeline ${index + 1}`}/>
              {#if stageTakesParts(kind)}<button class="link stage-splitter" type="button" disabled={busy} aria-label={`Split ${stageLabel(kind)} in pipeline ${index + 1} into parts`} onclick={() => item.parts = withNewPart(item, kind)}>Split into parts</button>{/if}
              {#if rated.changed}<p class="stage-why"><strong>{rated.by === 'gaffer' ? 'Gaffer’s assessment' : 'Your choice'}.</strong> {rated.why || `The default for ${stageLabel(kind).toLowerCase()} is ${tierLabel(rated.standard)}.`} <button class="link" type="button" aria-label={`Put ${stageLabel(kind)} in pipeline ${index + 1} back on ${tierLabel(rated.standard)}`} onclick={() => setTier(item, kind, rated.standard)}>Move back to {tierLabel(rated.standard)} tier</button></p>{/if}
            </div>{/if}{/each}
          </div>{/if}
          {#if true}{@const count = (item.stages ?? []).reduce((sum, kind) => sum + Math.max(1, stageParts(item, kind).length), 0)}{@const splits = (item.stages ?? []).filter(kind => stageParts(item, kind).length > 1).length}<p class="hint">{item.stages?.length ?? 0} {item.stages?.length === 1 ? 'stage' : 'stages'}{splits ? `, ${splits} split into parts: ${count} agent sessions in all, one after another.` : ', each in an agent session of its own.'}</p>{/if}
          {/if}
          <CriteriaEditor bind:items={item.criteria} who={`pipeline ${index + 1}`} disabled={busy}/>
        </li>{/each}
      </ol>
      {#if sessions.length < cap}<button class="btn quiet add" type="button" onclick={addSession}>Add a pipeline</button>{/if}
      {@render launchChoices()}
      <p class="status" class:warning={!!approveBlocker && !waitOnly} class:hint={!approveBlocker || waitOnly} id={`${formId}-blocker`} role="status"><Sentence text={approveBlocker || (blockerSeen ? 'Nothing is holding the plan now.' : '')} /></p>
      {#if !approveBlocker && !limitsOk}<p class="warning" role="status">Choose a whole number of reviews from 1 to 48 and hours from 1 to 168.</p>{/if}
      {#if startError}<p class="warning" role="alert">{startError}</p>{/if}
      {/if}
      <section class="reply" aria-label="Reply to Gaffer">
        <label for={`${formId}-reply`}>Reply to Gaffer</label>
        <textarea id={`${formId}-reply`} aria-label="Reply to Gaffer" bind:value={reply} rows="3" placeholder="Not that — keep the export in one session and put its tests with it." disabled={planning}></textarea>
        <p class="hint">Gaffer reads this as an answer to the proposal above. Use it when the split or the scope is wrong; the rows are still yours to edit. Up to {MAX_OUTCOME_REPLY_BYTES.toLocaleString('en-GB')} bytes, sent only when you choose.</p>
        {#if replyProblem && reply.trim()}<p class="warning" role="status">{replyProblem}</p>{/if}
        <div class="actions"><button class="btn" type="button" disabled={planning || !!replyProblem} onclick={() => void sendReply()}>{planning ? 'Gaffer is reading your reply…' : 'Send reply'}</button></div>
        {@render planProgress()}
        {#if planError}<p class="warning" role="alert">{planError}</p>{/if}
      </section>
      <div class="actions">
        {#if declined}<button class="btn" type="button" onclick={() => step = 'objective'}>Back to the outcome</button>
        {:else}<button class="btn primary" type="button" disabled={busy || planning || !!approveBlocker || !limitsOk} aria-describedby={approveBlocker ? `${formId}-blocker` : undefined} onclick={() => void approve()}>{busy ? 'Saving the plan…' : `Approve and start ${sessions.length} ${sessions.length === 1 ? 'pipeline' : 'pipelines'}`}</button><button class="btn quiet" type="button" disabled={busy || planning} onclick={() => declining = true}>Decline proposal</button><button class="btn quiet" type="button" disabled={busy || planning} onclick={() => step = 'objective'}>Back</button>{/if}
      </div>
    {:else if record}
      <p class="objective"><span class="eyebrow">Outcome</span>{record.objective}</p>
      <p role="status" class="result-line">{done ? outcomeSummary(record.sessions) : startingProgress(record.sessions)}</p>
      {#if (record.images ?? []).length}<ul class="outcome-images" aria-label="Images attached to the outcome">
        {#each record.images ?? [] as path (path)}<li title={path}>{path.split('/').filter(Boolean).at(-1) || path}</li>{/each}
      </ul>{/if}
      {#if imageError}<p class="warning" role="alert">{imageError}</p>{/if}
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
{#if picking}<Templates openId={pickingId} {timeZone} {dateFormat} onuse={(id, version) => { picking = false; pickingId = ''; void readTemplate(id, version) }} onclose={() => { picking = false; pickingId = '' }}/>{/if}
{#if yoloOpen && machine}<YoloDialog gates={yoloGates} tool={tool} structured machine={machine.host} enabling oncancel={() => { yoloOpen = false }} onstart={() => { if (yoloCanStart(yoloGates)) yolo = true; yoloOpen = false }}/>{/if}
{#if declining}<ConfirmDialog title="Decline this proposal?" body="Nothing has started. This plan will not be approved or started, and stays here so you can tell Gaffer what you want instead or go back to the outcome." confirmLabel="Decline proposal" oncancel={() => declining = false} onconfirm={() => { declined = true; declining = false }}/>{/if}

<!-- Where the work runs and which agent starts it. A template's launch has no session count, and no repository to clone first. -->
{#snippet planProgress()}
  {#if planPhase}
    <div class="plan-progress">
      <ol role="status" aria-label="Gaffer’s progress on the plan">
        {#each PLAN_STEPS as item, at (item)}
          {@const mark = planMark(at)}
          <li class:done={mark === '✓'} class:failed={mark === '✕'} aria-current={mark === '…' ? 'step' : undefined}><span class="mark" aria-hidden="true">{mark}</span><span>{planStepText(item)}{#if mark === '✓'}<span class="sr-only">, done</span>{:else if mark === '✕'}<span class="sr-only">, failed</span>{/if}</span></li>
        {/each}
      </ol>
      {#if planning}<div class="plan-foot"><span>{clock(planSeconds)} · at most 1:30</span><button class="btn quiet" type="button" onclick={stopPlanning}>Stop</button></div>{/if}
    </div>
  {/if}
{/snippet}
{#snippet where()}
          <div class="pair">
            <label>Project<select aria-label="Project" bind:value={projectId} disabled={planning || cloning}>
              <option value="">Choose a project…</option>
              {#each projects as item (item.id)}<option value={item.id}>{item.name}{item.machine ? ` · ${item.machine}` : ''}</option>{/each}
              {#if !template}<option value={GITHUB}>A repository from GitHub…</option>{/if}
            </select></label>
            {#if github}<label>Machine<select aria-label="Machine" bind:value={ghHost} disabled={planning || cloning || !cloners.length}>
              {#if !ghHost}<option value="" disabled>{cloners.length ? 'Choose a machine…' : 'None can clone'}</option>{/if}
              {#each cloners as item (item.agentId)}<option value={item.agentId}>{item.host}</option>{/each}
            </select></label>{/if}
            <label>Agent<select aria-label="Agent" bind:value={tool} disabled={planning || !tools.length} onchange={() => { model = ''; effort = ''; source = ''; explicitSource = false }}>
              {#if !tools.length}<option value="">{project ? 'None available' : 'Choose a project first'}</option>{/if}
              {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
            </select></label>
            <label>Model<GafferModelSelect label="Model" {tool} bind:value={model} machine={offeredFor(tool)} {favourites} host={machine?.host ?? ''} disabled={planning || !tool}/></label>
            <label>Effort<select aria-label="Effort" bind:value={effort} disabled={planning || efforts(tool).length < 2}>
              {#each efforts(tool) as level (level)}<option value={level}>{level ? level[0].toUpperCase() + level.slice(1) : efforts(tool).length < 2 ? 'Not offered by this agent' : 'Agent’s default effort'}</option>{/each}
            </select></label>
            {#if !template}<label>Sessions<select aria-label="Sessions" bind:value={wanted} disabled={planning}>
              {#each Array.from({ length: cap }, (_, n) => n + 1) as n (n)}<option value={n}>Up to {n}</option>{/each}
            </select></label>{/if}
            {#if sourceShown}<div class="source-field"><ClineSourceSelect label="Source" value={chosenSource} saved={savedSource} offered={sourceOffered} note={startSource.note} disabled={planning} onchange={pickSource} onsaved={next => savedSource = next} /></div>{/if}
          </div>
          {#if launchesInWorktree && machine && project}
            <div class="latest-field">
              <LatestCodeChoice agentId={machine.agentId} host={machine.host} dir={project.dir} base={project.baseBranch}
                value={latestChoice} disabled={planning} onchange={(choice) => { latestOverride = choice === 'github'; planError = '' }} />
            </div>
          {/if}
{/snippet}
<!-- What the person saved in Gaffer settings: the crew editor starts from it. -->
{#snippet savedCrewBlock()}
          {#if crewRead !== 'reading'}
            <section class="saved-crew" aria-label="Your Gaffer settings">
              <h3>Your Gaffer settings</h3>
              {#if savedSettings.length}
                <dl class="saved-crew-list">
                  {#each savedSettings as row (row.tier)}<div>
                    <dt>{row.label}</dt>
                    <dd><span class="saved-part">Agent</span> {chatToolName(row.tool)}<span class="saved-sep" aria-hidden="true">·</span><span class="saved-part">Model</span> {row.model || 'Agent’s default model'}</dd>
                  </div>{/each}
                </dl>
                <p class="hint">The agent and model saved for each tier in <a href="/account/gaffer">Gaffer settings</a>. The crew below starts from these; a saved agent this machine cannot run falls back to the outcome’s own agent. You can change them for this outcome below.</p>
                {#if unavailableCrew.length}<p class="hint" role="status">{unavailableCrew.map(item => `${item.label} uses the outcome’s agent because ${chatToolName(item.tool)} is unavailable on this machine.`).join(' ')}</p>{/if}
              {:else if crewRead === 'failed'}
                <p class="hint">Your Gaffer settings could not be read. Each stage will use the agent and model you choose below. <a href="/account/gaffer">Open Gaffer settings</a>.</p>
              {:else}
                <p class="hint">No crew is saved in <a href="/account/gaffer">Gaffer settings</a> yet. Each stage will use the agent and model you choose below.</p>
              {/if}
            </section>
          {/if}
{/snippet}
<!-- What each stage would run on, before the person starts. A blocker here holds the start, for a template as for a plan, and opens the crew editor. -->
{#snippet readinessPanel()}
      {#if launch.ready + launch.look + launch.blocked + launch.reading}<section class="readiness" aria-label="Launch readiness">
        <p class="launch" class:look={(launch.look > 0 && !launch.blocked) || (launch.blocked > 0 && launch.waiting === launch.blocked)} class:stop={launch.blocked > launch.waiting}><strong>Launch readiness{machine ? ` on ${machine.host}` : ''}.</strong> <span aria-hidden="true">{launch.blocked > launch.waiting ? '✕' : launch.waiting ? '!' : launch.look ? '!' : launch.reading ? '…' : '✓'}</span> {launch.text}</p>
        {#if launch.causes.length}<ul>{#each launch.causes as cause (cause.text)}<li class={cause.waiting ? 'waiting' : cause.level}>
          <span class="mark" aria-hidden="true">{cause.waiting ? '!' : cause.level === 'blocked' ? '✕' : cause.level === 'look' ? '!' : '·'}</span>
          <span class="text"><strong>{cause.waiting ? 'Waiting to be unlocked' : cause.level === 'blocked' ? 'Cannot start' : cause.level === 'look' ? 'Look at' : 'Note'}</strong> · {cause.machine ? 'Every stage' : pathway ? `Used by ${inWords(cause.stages)}` : `${inWords(cause.crews)} ${cause.crews.length === 1 ? 'crew' : 'crews'}, used by ${inWords(cause.stages)}`}. <Sentence text={cause.text} /></span>
          {#if cause.level !== 'note' && !cause.machine && !pathway}<button class="btn" type="button" onclick={() => void chooseWho(cause.crews)}>Choose who does what<span class="sr-only"> for the {inWords(cause.crews)} {cause.crews.length === 1 ? 'crew' : 'crews'}</span></button>{/if}
        </li>{/each}</ul>{/if}
      </section>{/if}
{/snippet}
<!-- How the work is run: access, limits, approvals, decisions, reporting, merge and trust. -->
{#snippet launchChoices()}
      <div class="access" aria-label="Outcome access review"><p>{template ? 'Starting' : 'Approving'} has the controller start {sessions.length === 1 ? 'this pipeline' : `these ${sessions.length} pipelines`} on {project?.machine || 'the project’s machine'} {yolo ? 'on YOLO' : 'with ordinary approvals (no YOLO)'}, one after another, whether or not this page stays open.{#if yolo}{' '}<strong>On YOLO the agents do not wait for you before running commands or changing files, and that includes what Gaffer asks them to do.</strong>{/if}{' '} Gaffer may then read each one’s recent conversation, including code or logs in it, and queue instructions for its working model. If the provider does not yet allow session data, this enables it. {merging ? 'Gaffer cannot run commands or answer approvals.' : 'Gaffer cannot merge, run commands or answer approvals.'}</p></div>
      {#if !template}<details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours per session <span>Change limits</span></summary>
        <div class="pair two"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours}/></label></div>
      </details>{/if}
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
      {#if hasMerge && !template}<label class="trust"><input type="checkbox" bind:checked={autoMerge} disabled={busy}/><span>Merge by itself once every check passes, no review thread is open and there are no conflicts. Left off, each pipeline stops at <strong>Ready to merge</strong> and waits for you.{#if yolo && !yoloWhy}{' '}<strong>On YOLO a pipeline still stops there</strong> until you confirm on the Factory page that it may merge by itself.{/if}</span></label>{/if}
      {#if needsTrust}<label class="trust"><input type="checkbox" bind:checked={trust} disabled={busy}/><span>I trust this folder: {trustNames} may run the plugins and customisations it finds in {project?.dir || 'it'}. {runTools.includes('antigravity') ? 'Google Antigravity needs this to start at all; OpenCode needs it only in a folder that has plugins.' : 'Leave this off and a folder that has any is refused.'}</span></label>{/if}
{/snippet}

<!-- An agent that has left the machine stays in its box, said as missing, so the box is never blank. -->
{#snippet awayAgent(id: string)}{#if id && !tools.includes(id)}<option value={id} disabled>{chatToolName(id)}{tools.length ? ' (missing)' : ''}</option>{/if}{/snippet}
<!-- On the plan step too. Frozen while the plan is being saved: approve() awaits the image uploads before it builds what it sends. -->
<!-- The share of a plan under which a backup takes over. -->
{#snippet balanceField()}
            <div class="balance" role="group" aria-label="When a backup takes over">
              <label for={`${formId}-balance`}>Switch to a backup below</label>
              <span class="percent"><input id={`${formId}-balance`} type="number" inputmode="numeric" min={BALANCE_MIN_PERCENT} max={BALANCE_MAX_PERCENT} step="5" bind:value={balanceBelow} disabled={planning || busy} aria-describedby={`${formId}-balance-unit`} onblur={() => balanceBelow = balancePercent(balanceBelow)}/><span id={`${formId}-balance-unit`}>% of the first choice’s plan left</span></span>
            </div>
{/snippet}
{#snippet crewDetails()}
          <details class="limit-editor crew" bind:this={crewEditor}><summary>{crewTools.every(id => id === tool) && CREW_TIERS.every(item => crew[item.tier].model.trim() === model.trim() && (crew[item.tier].effort ?? '') === effort) ? 'One agent does every stage' : 'A crew does the stages'} <span>Choose who does what</span></summary>
            <p class="hint">Right tool for the job. Each stage has a tier: Heavy for careful judgement, Standard for ordinary building, Light for routine work. Say here which agent, model and effort each tier means. Gaffer’s plan puts a stage on another tier where the work calls for it, and you can change any stage below. Every stage runs in a session of its own.</p>
            <div class="crew-rows">
              {#each CREW_TIERS as item (item.tier)}<div class="crew-row" role="group" aria-label={`${item.label} crew`}>
                <div class="crew-role"><strong>{item.label}</strong><span>{item.does}</span>{#if editedRoles[item.tier]}<button class="link crew-reset" type="button" disabled={planning || busy} aria-label={`Put the ${item.label} crew back to ${savedHere(item.tier) ? 'your saved crew' : 'the outcome’s agent'}`} onclick={() => startTier(item.tier)}>{savedHere(item.tier) ? 'Back to your saved crew' : 'Back to the outcome’s agent'}</button>{/if}</div>
                <span class="crew-slot">First choice</span>
                <select aria-label={`${item.label} agent`} bind:value={crew[item.tier].tool} disabled={planning || busy || !tools.length} onchange={() => { edit(item.tier); const same = crew[item.tier].tool === tool; crew[item.tier].model = same ? model : ''; crew[item.tier].effort = same ? effort : ''; crew[item.tier].source = same ? chosenSource : ''; crewSourceExplicit[item.tier] = false }}>{@render awayAgent(crew[item.tier].tool)}{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
                <GafferModelSelect label={`${item.label} model`} tool={crew[item.tier].tool} bind:value={crew[item.tier].model} machine={offeredFor(crew[item.tier].tool)} {favourites} host={machine?.host ?? ''} disabled={planning || busy} onchange={() => edit(item.tier)}/>
                <select aria-label={`${item.label} effort`} bind:value={crew[item.tier].effort} onchange={() => edit(item.tier)} disabled={planning || busy || efforts(crew[item.tier].tool).length < 2}>{#each efforts(crew[item.tier].tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)} effort` : efforts(crew[item.tier].tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}</select>
                {#if crew[item.tier].tool === 'cline' && machine?.capabilities?.acpProvider}<div class="crew-source"><ClineSourceSelect label={`${item.label} source`} value={crew[item.tier].source ?? ''} saved={crew[item.tier].source || savedSource} offered={sourceOffered} disabled={planning || busy} onchange={next => pickCrewSource(item.tier, next)} onsaved={next => savedSource = next} /></div>{/if}
                <div class="crew-backup" role="group" aria-label={`${item.label} backup`}>
                  <span class="crew-slot">Backup</span>
                  <select aria-label={`${item.label} backup agent`} bind:value={backups[item.tier].tool} disabled={planning || busy || !tools.length} onchange={() => { backups[item.tier].model = ''; backups[item.tier].effort = ''; backups[item.tier].source = ''; if (!backups[item.tier].tool) more[item.tier] = [] }}><option value="">No backup</option>{@render awayAgent(backups[item.tier].tool)}{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
                  {#if backups[item.tier].tool}
                    <GafferModelSelect label={`${item.label} backup model`} tool={backups[item.tier].tool} bind:value={backups[item.tier].model} machine={offeredFor(backups[item.tier].tool)} {favourites} host={machine?.host ?? ''} disabled={planning || busy}/>
                    <select aria-label={`${item.label} backup effort`} bind:value={backups[item.tier].effort} disabled={planning || busy || efforts(backups[item.tier].tool).length < 2}>{#each efforts(backups[item.tier].tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)} effort` : efforts(backups[item.tier].tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}</select>
                    {#if backups[item.tier].tool === 'cline' && machine?.capabilities?.acpProvider}<div class="crew-source"><ClineSourceSelect label={`${item.label} backup source`} value={backups[item.tier].source ?? ''} saved={backups[item.tier].source || savedSource} offered={sourceOffered} disabled={planning || busy} onchange={next => backups[item.tier].source = next} onsaved={next => savedSource = next} /></div>{/if}
                    {#each more[item.tier] as extra, n (n)}
                      <span class="crew-slot">Then</span>
                      <select aria-label={`${item.label} backup ${n + 2} agent`} bind:value={extra.tool} disabled={planning || busy} onchange={() => { if (!extra.tool) more[item.tier] = more[item.tier].filter((_, at) => at !== n); else { extra.model = ''; extra.effort = ''; extra.source = '' } }}><option value="">Remove this backup</option>{@render awayAgent(extra.tool)}{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
                      <GafferModelSelect label={`${item.label} backup ${n + 2} model`} tool={extra.tool} bind:value={extra.model} machine={offeredFor(extra.tool)} {favourites} host={machine?.host ?? ''} disabled={planning || busy}/>
                      <select aria-label={`${item.label} backup ${n + 2} effort`} bind:value={extra.effort} disabled={planning || busy || efforts(extra.tool).length < 2}>{#each efforts(extra.tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)} effort` : efforts(extra.tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}</select>
                      {#if extra.tool === 'cline' && machine?.capabilities?.acpProvider}<div class="crew-source"><ClineSourceSelect label={`${item.label} backup ${n + 2} source`} value={extra.source ?? ''} saved={extra.source || savedSource} offered={sourceOffered} disabled={planning || busy} onchange={next => extra.source = next} onsaved={next => savedSource = next} /></div>{/if}
                    {/each}
                    {#if more[item.tier].length < MAX_BACKUPS - 1}<button class="link crew-more" type="button" disabled={planning || busy} aria-label={`Add another backup for ${item.label}`} onclick={() => more[item.tier] = [...more[item.tier], { tool: tools.find(id => id !== backups[item.tier].tool) ?? tools[0] ?? '', model: '', effort: '' }]}>Add another backup</button>{/if}
                  {:else}<span class="crew-none">Stages stay on the first choice.</span>{/if}
                </div>
              </div>{/each}
            </div>
            {@render balanceField()}
            <p class="hint">{anyBackup ? `A stage starts on its tier’s backup when the first choice’s plan has under ${balancePercent(balanceBelow)}% left in its tightest window and the backup’s has more. The stage says when that happened and why.` : 'No tier has a backup yet, so every stage runs on its first choice.'} Balancing needs the machine to be able to read the agent’s plan; without figures every stage stays on its first choice.</p>
            <p class="crew-save"><button class="link" type="button" disabled={planning || busy || crewSaving || !tools.length} onclick={() => void rememberCrew()}>{crewSaving ? 'Saving…' : 'Save as my crew'}</button> <span role="status">{crewNote}</span></p>
          </details>
{/snippet}

<style>
  .outcome{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted)}.warning{color:var(--bad)}
  form{display:grid;gap:12px;min-width:0}label{font-weight:600;display:grid;gap:6px;min-width:0}
  textarea,input,select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .pathway-choice{display:contents}
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
  .crew-more{grid-column:2/-1;justify-self:start;min-height:44px;font-size:0.875rem}.crew-reset{margin-left:auto}
  .saved-crew{display:grid;gap:8px;padding:12px;border:1px solid var(--line);border-radius:10px;background:var(--bg)}
  .saved-crew h3{margin:0;font:600 14px var(--sans)}
  .saved-crew-list{margin:0;display:grid;gap:4px}
  .saved-crew-list>div{display:grid;grid-template-columns:5.5rem minmax(0,1fr);gap:2px 10px;align-items:baseline}
  .saved-crew-list dt{font-weight:600}
  .saved-crew-list dd{margin:0;min-width:0;overflow-wrap:anywhere;color:var(--ink-soft)}
  .saved-part{font-size:0.875rem;color:var(--muted)}
  .saved-sep{margin:0 6px;color:var(--muted)}
  @media(max-width:479px){.saved-crew-list>div{grid-template-columns:4.5rem minmax(0,1fr)}}
  .launch{color:var(--ink-soft)}.launch.look span{color:var(--warn)}.launch.stop span{color:var(--bad)}
  /* Each cause is a row like a stage line: a fixed-width mark, then the text, with its button after it (under it on narrow widths). */
  .readiness{display:grid;gap:6px;min-width:0}.readiness ul{list-style:none;margin:0;padding:0;display:grid;gap:6px}
  .readiness li{display:grid;grid-template-columns:1.5em minmax(0,1fr) auto;gap:2px 10px;align-items:start;min-width:0;font-size:0.875rem;color:var(--muted)}
  .readiness .mark{font-weight:700}.readiness li.blocked .mark{color:var(--bad)}.readiness li.look .mark{color:var(--warn)}.readiness li.waiting .mark,.readiness li.waiting strong{color:var(--muted)}.readiness strong{font-weight:600}.readiness li.blocked strong{color:var(--bad)}
  .readiness li .btn{align-self:center}
  .status:empty{position:absolute;width:0;height:0;overflow:hidden}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap;border:0}
  @media(max-width:559px){.readiness li{grid-template-columns:1.5em minmax(0,1fr)}.readiness li .btn{grid-column:2;justify-self:start}}.stage-splitter{font-size:0.875rem;white-space:nowrap}
  .crew-rows{display:grid;gap:10px;padding-top:6px}.crew-row{display:grid;grid-template-columns:5.5rem minmax(0,1fr) minmax(0,1.25fr) minmax(0,1.05fr);gap:8px 10px;align-items:start}.crew-row+.crew-row{padding-top:12px;border-top:1px solid var(--line)}.crew-source{grid-column:2/-1}.crew-role{grid-column:1/-1;display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 8px;min-width:0}.crew-role span{font-size:0.875rem;color:var(--muted);font-weight:400}
  .stage-pick{display:flex;flex-wrap:wrap;gap:6px}.stage-chip{font:inherit;font-size:0.875rem;min-height:36px;padding:4px 12px;border-radius:999px;border:1px solid var(--line);background:var(--bg);color:var(--muted);cursor:pointer}.stage-chip[aria-pressed=true]{background:var(--tint);border-color:var(--accent);color:var(--ink);font-weight:600}
  /* Each chosen stage with its tier and who that means. One the plan or the person moved off its default is marked, with why. */
  .stage-tiers{margin:8px 0 0;display:grid;gap:6px}.stage-tier{display:grid;grid-template-columns:5.5rem 8rem minmax(0,1fr) auto;align-items:center;gap:2px 10px;padding:4px 8px;border:1px solid var(--line-soft);border-radius:8px}
  .stage-tier.changed{border-color:color-mix(in srgb,var(--accent) 45%,var(--line));background:var(--accent-wash)}.stage-name{font-weight:600}.stage-tiers select{min-height:36px;font:inherit;font-size:0.875rem}
  .stage-tier :global(.ready-line){grid-column:1/-1}.stage-splitter{grid-column:3/-1;grid-row:1;justify-self:end}.stage-why{grid-column:1/-1;margin:2px 0 0;font-size:0.875rem;color:var(--ink-soft)}
  .crew-save{margin:12px 0 0;display:flex;flex-wrap:wrap;align-items:center;gap:4px 12px;font-size:0.875rem;color:var(--muted)}
  @media(max-width:479px){.crew-row{grid-template-columns:1fr}.crew-slot{line-height:1.4;margin-top:4px}.crew-none,.crew-source{grid-column:1/-1}.stage-tier{grid-template-columns:5rem minmax(0,1fr)}.stage-splitter{grid-column:1/-1;grid-row:auto;justify-self:start;min-height:44px}}@media(pointer:coarse){.stage-chip{min-height:44px}.stage-tiers select{min-height:44px}}
  .approvals{border:0;margin:0;padding:0;display:grid;gap:6px;min-width:0}.approvals legend{font-weight:600;padding:0;margin-bottom:6px}
  .choice{display:inline-flex;border:1px solid var(--line);border-radius:10px;overflow:hidden;justify-self:start}.choice label{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 16px;font-weight:600;cursor:pointer;color:var(--muted)}.choice label+label{border-left:1px solid var(--line)}.choice label.on{background:var(--tint);color:var(--ink)}.choice label.off{cursor:not-allowed;opacity:.6}.choice input{width:18px;height:18px;min-height:0;margin:0;padding:0;accent-color:var(--accent)}
  .hint.warn{color:var(--warn)}
  .result-line{font-weight:600}.merging{margin:0;font-weight:600;color:var(--ink);overflow-wrap:anywhere}.trust{display:flex;align-items:flex-start;gap:10px;font-weight:400;min-height:44px}.trust input{width:20px;height:20px;min-height:0;margin:2px 0 0;flex:none;padding:0;accent-color:var(--accent)}
  .outcome-images{list-style:none;margin:0;padding:0;display:flex;flex-wrap:wrap;gap:6px}.outcome-images li{padding:4px 10px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:0.875rem;overflow-wrap:anywhere}.results{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.results li{display:grid;gap:2px;padding:10px 14px;min-width:0;overflow-wrap:anywhere}.results li+li{border-top:1px solid var(--line)}.results span{font-size:0.875rem;color:var(--muted)}.results li.failed span{color:var(--bad)}.results a{color:var(--accent-text);font-weight:600;min-height:44px;display:inline-flex;align-items:center;justify-self:start}
  .plan-progress{display:grid;gap:10px;padding:12px 14px;border:1px solid var(--line);border-radius:12px}.plan-progress ol{list-style:none;margin:0;padding:0;display:grid;gap:6px}.plan-progress li{display:grid;grid-template-columns:20px minmax(0,1fr);gap:8px;color:var(--muted);overflow-wrap:anywhere}.plan-progress li.done{color:var(--ink)}.plan-progress li[aria-current]{color:var(--ink);font-weight:600}.plan-progress li.failed{color:var(--bad);font-weight:600}.plan-progress .mark{text-align:center}.plan-progress li[aria-current] .mark{animation:plan-wait 1.2s ease-in-out infinite}.plan-foot{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;color:var(--muted);font-variant-numeric:tabular-nums}
  @keyframes plan-wait{50%{opacity:.35}}@media(prefers-reduced-motion:reduce){.plan-progress li[aria-current] .mark{animation:none}}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,a:focus-visible,summary:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.pair,.pair.two{grid-template-columns:1fr}}
</style>
