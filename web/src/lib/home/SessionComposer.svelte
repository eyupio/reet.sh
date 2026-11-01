<script lang="ts">
  import { gafferTooltip } from '../gafferTooltip'
  import { onMount, tick, untrack } from 'svelte'
  import { addModelFavourite, listModelFavourites, removeModelFavourite, type Skill } from '../api'
  import { cleanModel, modelSuggestions, toolName, validateDir, validateModel, validatePrompt } from '../agentLaunch'
  import { CATALOGUE } from '../catalogue'
  import { shell } from '../dialogShell'
  import { installPrompt } from '../install'
  import { defaultTitle } from '../launchBar'
  import { effortChoices, favouriteKey, modelOptions, modelStatus, resolveLaunchDefaults, starLabel, type MachineModels, type ModelFavourite } from '../models'
  import { saveDraft } from '../sessionBacklog'
  import { insertSkillPrompt } from '../skillPrompt'
  import { machineModels, refreshMachineModels, structuredAgent, type FolderProject, type TerminalMachine } from '../terminal/api'
  import { cleanDraftOptions, draftOptionRules, ideaProblem } from '../draftOptions'
  import SlotPicker, { type SlotOption } from '../launch/SlotPicker.svelte'
  import type { DraftImage, SessionDraft } from '../sessionBoard'
  import type { LaunchDefaults } from '../launchBar'
  import FolderBrowser from './FolderBrowser.svelte'
  import SkillPromptPicker from './SkillPromptPicker.svelte'

  let { draft = null, projects, machines, profile, initialProject = '', onsaved, onclose, onerror }: {
    draft?: SessionDraft | null; projects: FolderProject[]; machines: TerminalMachine[]; profile: LaunchDefaults
    initialProject?: string; onsaved: (draft: SessionDraft) => void; onclose: () => void; onerror: (e: unknown) => void
  } = $props()
  const id = $props.id()
  let dialog: HTMLDialogElement, promptInput = $state<HTMLTextAreaElement>()
  let title = $state(''), prompt = $state(''), projectId = $state(''), agentId = $state(''), dir = $state('')
  let tool = $state(''), model = $state(''), effort = $state(''), mode = $state<'project' | 'folder'>('project')
  let idea = $state(false), yolo = $state(false), worktree = $state(false), base = $state(''), chat = $state(true), trust = $state(false)
  let images = $state<DraftImage[]>([]), busy = $state(false), reading = $state(false), message = $state('')
  let browse = $state(false), titleTouched = $state(false)
  let modelButton = $state<HTMLButtonElement>(), effortButton = $state<HTMLButtonElement>(), skillButton = $state<HTMLButtonElement>()
  let modelOpen = $state(false), effortOpen = $state(false), skillOpen = $state(false), touch = $state(false)
  let modelKey = $state(''), modelRead = $state<MachineModels | null>(null), modelLoading = $state(false), modelReadError = $state('')
  let favourites = $state<ModelFavourite[]>([]), favouriteNote = $state('')
  let favouriteQueue: Promise<void> = Promise.resolve(), favouriteMutationSeq = 0, favouritesLoadSeq = 0
  const life = new AbortController()
  const enabled = CATALOGUE.filter(a => a.enabled)
  const machine = $derived(machines.find(m => m.agentId === agentId))
  const bytes = $derived(new TextEncoder().encode(prompt).length)
  const modelTargetKey = $derived(agentId && tool ? `${agentId}\u0000${tool}` : '')
  const modelMachine = $derived(modelKey === modelTargetKey ? modelRead : null)
  const efforts = $derived(effortChoices(tool))
  const takesEffort = $derived(efforts.length > 1)
  const project = $derived(mode === 'project' ? projects.find(p => p.id === projectId) : undefined)
  const rules = $derived(draftOptionRules({ tool, isRepo: !!project?.isRepo, canChat: !!structuredAgent(machine?.capabilities, tool), chat }))
  const modelStatusInfo = $derived.by(() => {
    const host = machine?.host ?? ''
    if (!machine) return modelStatus(null, host)
    const blank = (state: MachineModels['state']): MachineModels => ({ tool, state, models: [], more: false, source: '', version: '', fetchedAt: null, checkedAt: null, reason: '', sentence: '' })
    if (!machine.ready) return modelStatus(modelMachine ? { ...modelMachine, state: 'offline', sentence: '' } : blank('offline'), host)
    if (machine.capabilities?.modelsOff) return modelStatus(modelMachine ? { ...modelMachine, state: 'off', sentence: '' } : blank('off'), host)
    if (machine.capabilities && !machine.capabilities.models) return modelStatus(modelMachine ? { ...modelMachine, state: 'update', sentence: '' } : blank('update'), host)
    if (!modelMachine) return modelStatus(null, host)
    return modelStatus(modelMachine, host)
  })
  const modelStatusLine = $derived.by(() => {
    if (!modelTargetKey) return ''
    if (modelLoading) return `Reading ${machine?.host || 'the machine'}’s models…`
    const failure = modelKey === modelTargetKey ? modelReadError : ''
    const ageWord = modelMachine?.state === 'failed' ? 'Checked' : 'Read'
    const line = [failure || modelStatusInfo.sentence, !failure && modelStatusInfo.age ? `${ageWord} ${modelStatusInfo.age}.` : ''].filter(Boolean).join(' ')
    return [line, favouriteNote].filter(Boolean).join(' ')
  })
  const modelStatusAction = $derived.by(() => {
    if (!modelTargetKey || !modelStatusInfo.canRefresh || !machine?.ready || !machine.capabilities?.models || machine.capabilities.modelsOff) return undefined
    return { label: 'Refresh', onclick: () => void refreshModels(), disabled: modelLoading }
  })
  const initial = () => JSON.stringify({ title, prompt, projectId, agentId, dir, tool, model, effort, mode, images, idea, yolo, worktree, base, chat, trust })
  let original = '', confirmClose = $state(false)

  function defaults(project?: FolderProject) {
    const resolved = resolveLaunchDefaults({ project, account: profile, availableAgents: enabled.map(a => a.id) })
    tool = resolved.agent; model = resolved.model; effort = resolved.effort
  }
  function chooseProject() {
    const p = projects.find(p => p.id === projectId)
    if (!p) return
    agentId = p.host; dir = p.dir; browse = false; defaults(p)
    worktree = p.worktreeDefault === 'yes' ? true : p.worktreeDefault === 'no' ? false : profile.worktreesByDefault !== false
    base = ''; trust = false
  }
  function changeMode(next: 'project' | 'folder') {
    mode = next; browse = false
    if (next === 'project') { projectId ||= projects[0]?.id ?? ''; chooseProject() }
    else { projectId = ''; agentId ||= machines[0]?.agentId ?? '' }
  }
  $effect(() => {
    if (!titleTouched) title = defaultTitle(prompt)
  })
  $effect(() => {
    const key = modelTargetKey, target = agentId, agent = tool
    if (!key || !target || !agent || !machine?.ready) { modelLoading = false; return }
    if (untrack(() => modelKey === key && (modelLoading || modelRead))) return
    untrack(() => { void readModels(target, agent, key) })
  })
  onMount(() => {
    touch = matchMedia('(pointer: coarse)').matches
    void loadFavourites()
    if (draft) {
      title = draft.title; prompt = draft.prompt; projectId = draft.projectId; agentId = draft.agentId; dir = draft.dir
      tool = draft.tool; model = draft.model; effort = draft.effort; images = [...(draft.images ?? [])]
      idea = draft.idea === true; yolo = draft.yolo === true; worktree = draft.worktree === true; base = draft.base ?? ''; chat = draft.chat === true; trust = draft.trust === true
      if (!CATALOGUE.find(agent => agent.id === tool)?.effort) effort = ''
      mode = projectId ? 'project' : 'folder'; titleTouched = true
    } else {
      projectId = initialProject || projects[0]?.id || ''; mode = projectId ? 'project' : 'folder'
      if (projectId) chooseProject(); else { agentId = machines[0]?.agentId ?? ''; defaults() }
    }
    original = initial(); dialog.showModal()
    return () => life.abort()
  })

  function failedModels(agent: string, sentence: string, previous: MachineModels | null): MachineModels {
    return previous
      ? { ...previous, state: 'failed', sentence, checkedAt: Date.now() }
      : { tool: agent, state: 'failed', models: [], more: false, source: '', version: '', fetchedAt: null, checkedAt: Date.now(), reason: '', sentence }
  }
  async function readModels(target: string, agent: string, key: string) {
    modelLoading = true
    modelReadError = ''
    try {
      const next = await machineModels(target, agent, life.signal)
      if (life.signal.aborted || modelTargetKey !== key) return
      modelRead = next
      modelKey = key
    } catch {
      if (life.signal.aborted || modelTargetKey !== key) return
      const sentence = `Couldn’t read ${machine?.host || 'the machine'}’s models just now. Favourites and suggestions still work.`
      modelRead = failedModels(agent, sentence, modelRead)
      modelKey = key
      modelReadError = sentence
    } finally {
      if (!life.signal.aborted && modelTargetKey === key) modelLoading = false
    }
  }
  async function refreshModels() {
    const key = modelTargetKey, target = agentId, agent = tool
    if (!key || !target || !agent || modelLoading) return
    const before = modelKey === key ? modelRead : null
    modelLoading = true
    modelReadError = ''
    try {
      const next = await refreshMachineModels(target, agent, life.signal)
      if (life.signal.aborted || modelTargetKey !== key) return
      modelRead = next.state === 'offline' && before ? { ...next, models: before.models, fetchedAt: before.fetchedAt } : next
      modelKey = key
    } catch (e) {
      if (life.signal.aborted || modelTargetKey !== key) return
      const sentence = (e as { status?: number })?.status === 429 ? 'Slow down a moment, then try again. The last list is still here.'
        : (e as { status?: number })?.status === 409 ? 'Another read is already running. Try again in a moment. The last list is still here.'
        : `Couldn’t refresh ${machine?.host || 'the machine'}’s models. The last list is still here.`
      modelRead = before
      modelKey = key
      modelReadError = sentence
    } finally {
      if (!life.signal.aborted && modelTargetKey === key) modelLoading = false
    }
  }
  async function loadFavourites() {
    const seq = ++favouritesLoadSeq
    let timer: ReturnType<typeof setTimeout> | undefined
    try {
      const answer = await Promise.race([
        listModelFavourites(life.signal),
        new Promise<never>((_, no) => { timer = setTimeout(() => no(new Error('timeout')), 10_000) }),
      ])
      if (!life.signal.aborted && seq === favouritesLoadSeq) { favourites = answer.favourites ?? []; favouriteNote = '' }
    } catch {
      if (!life.signal.aborted && seq === favouritesLoadSeq) favouriteNote = 'Couldn’t read your favourite models just now.'
    } finally { clearTimeout(timer) }
  }
  async function toggleStar(value: string, starred: boolean) {
    const agent = tool
    if (!agent || !value) return
    const key = favouriteKey(agent, value)
    const beforeEntry = favourites.find(f => favouriteKey(f.tool, f.model) === key) ?? null
    const mutation = ++favouriteMutationSeq
    favouritesLoadSeq++
    favouriteNote = ''
    favourites = starred ? favourites.filter(f => favouriteKey(f.tool, f.model) !== key) : beforeEntry ? favourites : [...favourites, { tool: agent, model: value, agentDefault: false }]
    favouriteQueue = favouriteQueue.then(async () => {
      try {
        if (starred) await removeModelFavourite(agent, value)
        else {
          const answer = await addModelFavourite(agent, value)
          if (!life.signal.aborted && mutation === favouriteMutationSeq && answer?.favourites) favourites = answer.favourites
        }
      } catch (e) {
        if (life.signal.aborted) return
        favourites = favourites.filter(f => favouriteKey(f.tool, f.model) !== key)
        if (beforeEntry) favourites = [...favourites, beforeEntry]
        favouriteNote = (e as { message?: string })?.message ? (e as Error).message : 'Couldn’t update your favourites just now. Try again.'
      }
    })
    await favouriteQueue
  }
  function modelOptionsFor(): SlotOption[] {
    if (!tool) return []
    return modelOptions({
      tool, current: cleanModel(model), machine: modelMachine, favourites, recent: [], suggestions: modelSuggestions(tool),
      selectedModel: cleanModel(model), host: machine?.host ?? '', agentName: toolName(tool),
    }).map(option => option.value === '' ? option : {
      ...option, starLabel: starLabel(tool, option.value, !!option.starred), onstar: () => { void toggleStar(option.value, !!option.starred) },
    })
  }
  function effortOptionsFor(): SlotOption[] {
    return efforts.map(value => ({ value, label: value || 'Agent default' }))
  }
  function useSkill(skill: Skill) {
    const text = installPrompt(skill)
    if (!text) { message = 'That skill has no source to run it from. Link one on the Skills page.'; return }
    const start = promptInput?.selectionStart ?? prompt.length
    const end = promptInput?.selectionEnd ?? start
    const inserted = insertSkillPrompt(prompt, text, start, end)
    const firstPrompt = prompt.trim() === '' && !titleTouched
    prompt = inserted.text
    if (firstPrompt) { title = skill.name; titleTouched = true }
    void tick().then(() => {
      if (!promptInput) return
      promptInput.focus()
      promptInput.setSelectionRange(inserted.caret, inserted.caret)
    })
  }
  function close() {
    if (busy || reading) return
    if (original !== initial()) { confirmClose = true; void tick().then(() => { const b = dialog?.querySelector<HTMLElement>('.discard'); b?.scrollIntoView({ block: 'nearest' }); b?.querySelector<HTMLElement>('button')?.focus() }); return }
    onclose()
  }
  async function attach(files: File[]) {
    if (reading || busy) return
    reading = true; message = ''
    try {
      const next = [...images]
      for (const file of files) {
        if (next.length >= 4) throw new Error('Keep up to four reference images.')
        if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(file.type) || file.size <= 0 || file.size > 5 << 20) throw new Error('Choose a PNG, JPEG, GIF or WebP image up to 5 MB.')
        const data = await new Promise<string>((resolve, reject) => { const r = new FileReader(); r.onload = () => resolve(String(r.result)); r.onerror = reject; r.readAsDataURL(file) })
        next.push({ name: file.name, data })
        if (next.reduce((sum, im) => sum + (im.data.length - im.data.indexOf(',') - 1) * 3 / 4, 0) > 10 << 20) throw new Error('Keep the attached images under 10 MB together.')
      }
      images = next
    } catch (e) { message = e instanceof Error ? e.message : "Couldn't read that image." }
    finally { reading = false }
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (busy || reading) return
    const why = !agentId ? 'Choose a machine.' : mode === 'project' && !projectId ? 'Choose a project.' : idea ? ideaProblem({ idea, title, prompt }) || (prompt.trim() ? validatePrompt(tool, prompt) : null) || validateModel(model) || (dir ? validateDir(dir) : null) : !prompt.trim() ? 'Describe what the agent should do.' : validatePrompt(tool, prompt) || validateModel(model) || (dir ? validateDir(dir) : null)
    if (why) { message = why; return }
    busy = true; message = ''
    try {
      const saved = await saveDraft({ title: title.trim() || defaultTitle(prompt), prompt, idea, projectId: mode === 'project' ? projectId : '', agentId, dir, tool, model: model.trim(), effort, ...cleanDraftOptions({ tool, isRepo: !!project?.isRepo, canChat: !!structuredAgent(machine?.capabilities, tool), chat, worktree, base, trust, yolo }), images, version: draft?.version ?? 0 }, draft?.id, AbortSignal.timeout(20_000))
      onsaved(saved)
    } catch (e) { onerror(e); message = e instanceof Error ? e.message : "Couldn't save this session. Your draft is still here." }
    finally { busy = false }
  }
</script>

<dialog bind:this={dialog} use:shell aria-labelledby={`${id}-heading`} oncancel={e => { e.preventDefault(); close() }}>
  <form onsubmit={save}>
    <header data-shell-head>
      <div><span class="eyebrow">PLAN YOUR NEXT MOVE</span><h2 id={`${id}-heading`}>{draft ? 'Edit session' : 'A little planning. Proper progress.'}</h2><p data-shell-aside>Give your agent a clear brief. Start it when you’re ready.</p></div>
      <button type="button" class="close" data-shell-close aria-label="Close session composer" onclick={close} disabled={busy || reading}>×</button>
    </header>
    <div class="body" data-shell-body>
      {#if confirmClose}
        <div class="discard" role="alert" style="position: sticky; top: 0; z-index: 3"><strong>Discard your unsaved changes?</strong><div><button type="button" class="btn" onclick={() => confirmClose = false}>Keep editing</button><button type="button" class="btn" onclick={onclose}>Discard changes</button></div></div>
      {/if}
      <fieldset disabled={busy || reading}>
        <label class="idea-toggle"><input type="checkbox" bind:checked={idea} /> <span><strong>Just capture an idea</strong><small>A placeholder so you don’t forget. It can’t be started until you add a prompt and switch this off.</small></span></label>
        <label class="field"><span>Session title <small>{idea ? 'Required if there’s no note' : 'Optional · suggested from your prompt'}</small></span><input aria-label="Session title" bind:value={title} oninput={() => titleTouched = true} maxlength="80" placeholder="A short name for this piece of work" /></label>
        <label class="field"><span>{idea ? 'Note' : 'Prompt'} <em>{idea ? 'Optional' : 'Required'}</em></span><textarea bind:this={promptInput} aria-label="Prompt" bind:value={prompt} rows="6" required={!idea} placeholder={idea ? 'A rough note is plenty. You can turn it into a full prompt later.' : 'What should the agent do? Include the context, the outcome you want, and anything it should keep in mind.'} onpaste={e => { const files = Array.from(e.clipboardData?.files ?? []); if (files.length) { e.preventDefault(); void attach(files) } }}></textarea></label>
        <div class="prompt-foot"><div class="prompt-actions"><label class="attach">＋ Attach images<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple disabled={reading} onchange={e => { void attach(Array.from(e.currentTarget.files ?? [])); e.currentTarget.value = '' }} /></label><button type="button" class="skill-add" bind:this={skillButton} aria-haspopup="listbox" aria-expanded={skillOpen} onclick={() => skillOpen = !skillOpen}>＋ Use a skill</button></div><span class:over={bytes > 2000}>{bytes.toLocaleString()} / 2,000 bytes</span></div>
        {#if images.length}<div class="images">{#each images as image, i}<div class="image"><img src={image.data} alt={image.name} /><span>{image.name}</span><button type="button" aria-label={`Remove ${image.name}`} onclick={() => images = images.filter((_, index) => index !== i)}>×</button></div>{/each}</div>{/if}
        <div class="section-title">Where should it work?</div>
        <div class="mode" role="group" aria-label="Session location"><button type="button" class:on={mode === 'project'} onclick={() => changeMode('project')}>In a project</button><button type="button" class:on={mode === 'folder'} onclick={() => changeMode('folder')}>Machine & folder</button></div>
        {#if mode === 'project'}
          <label class="field"><span>Project</span><select aria-label="Project" bind:value={projectId} onchange={chooseProject} required><option value="" disabled>Choose a project</option>{#each projects as project}<option value={project.id}>{project.name} · {machines.find(m => m.agentId === project.host)?.host ?? project.machine}</option>{/each}</select></label>
          {#if !projects.length}<p class="hint">Add a project from Home, or choose Machine & folder.</p>{/if}
          {#if projectId}<p class="location">{machine?.host ?? agentId} <span> / </span><code>{dir}</code></p>{/if}
        {:else}
          <label class="field"><span>Machine</span><select aria-label="Machine" bind:value={agentId} required onchange={() => { dir = ''; browse = false }}><option value="" disabled>Choose a machine</option>{#each machines as m}<option value={m.agentId}>{m.host}{m.ready ? '' : ' · offline'}</option>{/each}</select></label>
          <label class="field"><span>Folder <small>Leave empty for the machine’s home folder</small></span><div class="folder"><input aria-label="Folder" bind:value={dir} placeholder="/home/you/project" /><button type="button" class="btn" disabled={!machine?.ready || !machine.capabilities?.browse || machine.capabilities.browseOff} onclick={() => browse = !browse}>Browse</button></div></label>
          {#if browse && machine}<FolderBrowser agentId={machine.agentId} host={machine.host} session={id} start={dir} chosen={dir} serial={job => job()} onuse={picked => { dir = picked.path; browse = false }} />{/if}
        {/if}
        <div class="section-title">Who’s on the job?</div>
        <div class="settings"><label class="field"><span>Agent</span><select aria-label="Agent" bind:value={tool} onchange={() => { model = ''; effort = '' }}>{#each enabled as agent}<option value={agent.id}>{agent.name}</option>{/each}</select></label>
        <div class="field"><span>Model</span><button type="button" class="picker" bind:this={modelButton} aria-label={`Model: ${model || 'Agent default'}`} aria-haspopup="listbox" aria-expanded={modelOpen} disabled={busy || reading || !tool} onclick={() => modelOpen = !modelOpen}><span class="value">{model || 'Agent default'}</span><span class="chev" aria-hidden="true">▾</span></button></div>
        <div class="field"><span>Effort</span><button type="button" class="picker" bind:this={effortButton} aria-label={`Effort: ${effort || 'Agent default'}`} aria-haspopup="listbox" aria-expanded={effortOpen} disabled={busy || reading || !takesEffort} title={takesEffort ? '' : `${toolName(tool)} chooses its own effort.`} onclick={() => effortOpen = !effortOpen}><span class="value">{takesEffort ? effort || 'Agent default' : 'This agent decides'}</span>{#if takesEffort}<span class="chev" aria-hidden="true">▾</span>{/if}</button></div></div>
        {#if !takesEffort}<p class="hint effort-hint">{toolName(tool)} chooses its own effort level.</p>{/if}
        <div class="section-title">How should it run?</div>
        <div class="options">
          <div class="field"><span>Approvals</span><div class="mode" role="group" aria-label="Approvals"><button type="button" class:on={!yolo} aria-pressed={!yolo} onclick={() => yolo = false}>Ask me</button><button type="button" class:on={yolo} aria-pressed={yolo} onclick={() => yolo = true}>YOLO</button></div></div>
          {#if rules.showChat}<div class="field"><span>Opens in</span><div class="mode" role="group" aria-label="Opens in"><button use:gafferTooltip type="button" class:on={chat} aria-pressed={chat} onclick={() => chat = true}>Gaffer</button><button type="button" class:on={!chat} aria-pressed={!chat} onclick={() => chat = false}>{toolName(tool)} TUI</button></div></div>{/if}
          {#if rules.showWorktree}<label class="check"><input type="checkbox" bind:checked={worktree} /> Start in a fresh worktree</label>
            {#if worktree}<label class="field"><span>Base branch <small>Optional · leave empty for the default</small></span><input aria-label="Base branch" bind:value={base} placeholder="main" maxlength="200" /></label>{/if}{/if}
          {#if rules.showTrust}<label class="check"><input type="checkbox" bind:checked={trust} /> I trust this folder; OpenCode plugins in <code>.opencode</code> may run.</label>{/if}
        </div>
        {#if yolo}<p class="hint yolo-hint" role="note">YOLO is saved as a plan only. You’ll be asked to confirm, with the account and machine checks, when you start it.</p>{/if}
        <p class="hint">{machine && !machine.ready ? 'This machine is offline. You can save now and start when it reconnects.' : 'Saving to the backlog keeps everything ready for later.'}</p>
      </fieldset>
      {#if message}<p class="error" role="alert">{message}</p>{/if}
    </div>
    <footer data-shell-foot><span>{reading ? 'Reading images…' : 'Your prompt and images are saved to your account.'}</span><button type="button" class="btn" onclick={close} disabled={busy || reading}>Cancel</button><button class="btn primary" disabled={busy || reading || bytes > 2000 || !machines.length}>{busy ? 'Saving…' : draft ? 'Save changes' : idea ? 'Save idea' : 'Add to backlog'}</button></footer>
  </form>
  {#if modelOpen}
    <SlotPicker label="Model" options={modelOptionsFor()} value={model} sheet={touch} opener={modelButton ?? null}
      filter="Filter models" status={modelStatusLine} statusAction={modelStatusAction}
      other={{ label: 'Other…', field: 'Model name', validate: validateModel }}
      onpick={value => { model = value; modelOpen = false }} onclose={() => modelOpen = false} />
  {/if}
  {#if effortOpen}
    <SlotPicker label="Effort" options={effortOptionsFor()} value={effort} sheet={touch} opener={effortButton ?? null}
      onpick={value => { effort = value; effortOpen = false }} onclose={() => effortOpen = false} />
  {/if}
  {#if skillOpen}
    <SkillPromptPicker opener={skillButton ?? null} sheet={touch} oninsert={useSkill} onclose={() => skillOpen = false} />
  {/if}
</dialog>

<style>
  dialog { width: min(760px, calc(100vw - 32px)); padding: 0; border: 1px solid var(--line-strong); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 24px 100px color-mix(in srgb, var(--ink) 18%, transparent); }
  dialog::backdrop { background: var(--scrim); backdrop-filter: blur(3px); }
  form { display: flex; flex-direction: column; max-height: 90dvh; }
  header { display: flex; justify-content: space-between; gap: 20px; padding: 26px 28px 20px; border-bottom: 1px solid var(--line); }
  .eyebrow { font-size: 10px; font-weight: 650; letter-spacing: .13em; color: var(--accent-text); }
  h2 { font: 500 27px/1.2 var(--serif); margin: 8px 0; letter-spacing: -.025em; }
  header p { margin: 0; color: var(--muted); font-size: 13px; }
  .close { align-self: start; width: 36px; height: 36px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font-size: 24px; cursor: pointer; }
  .body { padding: 22px 28px; overflow-y: auto; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
  .field { display: flex; flex-direction: column; gap: 8px; margin-bottom: 17px; font-size: 13px; font-weight: 600; min-width: 0; }
  .field small { font-size: 11px; font-weight: 400; color: var(--muted); margin-left: 6px; }
  .field em { font-size: 10px; font-style: normal; font-weight: 400; color: var(--muted); margin-left: 8px; }
  input, textarea, select { width: 100%; box-sizing: border-box; font-size: 13px; font-weight: 400; color: var(--ink); }
  textarea { resize: vertical; min-height: 130px; line-height: 1.65; }
  .prompt-foot { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: -10px; color: var(--muted); font-size: 11px; }
  .prompt-actions { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; min-width: 0; }
  .skill-add { border: 0; background: none; padding: 7px 0; color: var(--accent-text); font: 600 11px var(--sans); cursor: pointer; }
  .skill-add:hover { text-decoration: underline; }
  .skill-add:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .attach { position: relative; padding: 7px 0; color: var(--accent-text); font-weight: 600; cursor: pointer; }
  .attach input { position: absolute; width: 100%; height: 100%; inset: 0; opacity: 0; cursor: pointer; }
  .attach:focus-within { outline: 2px solid var(--accent); outline-offset: 3px; }
  .images { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; }
  .image { display: flex; align-items: center; gap: 8px; border: 1px solid var(--line); border-radius: 8px; padding: 5px; font-size: 11px; max-width: 220px; }
  .image img { width: 38px; height: 38px; object-fit: cover; border-radius: 4px; }
  .image span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .image button { border: 0; background: none; color: var(--muted); cursor: pointer; font-size: 20px; min-width: 30px; }
  .section-title { margin: 24px 0 12px; padding-top: 20px; border-top: 1px solid var(--line); font-size: 13px; font-weight: 650; }
  .mode { display: inline-flex; border: 1px solid var(--line); border-radius: 8px; padding: 3px; background: var(--bg-sunk); margin-bottom: 16px; }
  .mode button { padding: 8px 14px; border: 1px solid transparent; border-radius: 5px; background: transparent; color: var(--muted); font: 500 12px var(--sans); cursor: pointer; }
  .mode button.on { background: var(--card); color: var(--ink); border-color: var(--line); box-shadow: 0 1px 3px color-mix(in srgb, var(--ink) 6%, transparent); }
  .location { font-size: 12px; color: var(--muted); overflow-wrap: anywhere; margin-top: -8px; }
  .location span { color: var(--line-strong); padding: 0 6px; }
  .folder { display: flex; gap: 8px; }
  .folder input { flex: 1; min-width: 0; }
  .settings { display: grid; grid-template-columns: 1fr 1.2fr 1fr; gap: 12px; }
  .picker { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; min-height: 42px; padding: 8px 10px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card); color: var(--ink); font: 400 13px var(--sans); text-align: left; cursor: pointer; }
  .picker:disabled { opacity: .55; cursor: default; }
  .picker .value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .picker .chev { flex: none; color: var(--muted); }
  .hint { font-size: 12px; color: var(--muted); margin: 0; line-height: 1.6; }
  .idea-toggle { display: flex; align-items: flex-start; gap: 10px; margin-bottom: 18px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg-sunk); font-size: 13px; cursor: pointer; }
  .idea-toggle input { width: auto; margin-top: 3px; }
  .idea-toggle strong { display: block; font-weight: 650; }
  .idea-toggle small { display: block; margin-top: 2px; font-size: 11px; font-weight: 400; color: var(--muted); }
  .effort-hint { margin: -9px 0 12px; }
  .options { display: grid; gap: 4px; }
  .options .mode { margin-bottom: 0; }
  .check { display: flex; align-items: flex-start; gap: 8px; margin: 0 0 14px; font-size: 13px; }
  .check input { width: auto; margin-top: 2px; }
  .yolo-hint { color: var(--warn); margin-bottom: 8px; }
  footer { display: flex; align-items: center; gap: 8px; padding: 18px 28px; border-top: 1px solid var(--line); background: var(--bg); }
  footer > span { flex: 1; font-size: 11px; color: var(--muted); }
  .error, .discard { border: 1px solid var(--warn); padding: 12px; border-radius: 8px; color: var(--warn); font-size: 13px; }
  .discard { margin-bottom: 18px; background: var(--card); }.discard div { display: flex; gap: 8px; margin-top: 10px; }
  .over { color: var(--bad); }
  @media (max-width: 650px) { input, textarea, select, .picker { font-size: 16px; } header, .body, footer { padding: 18px; } h2 { font-size: 23px; } .settings { grid-template-columns: 1fr; gap: 0; } .field small { display: block; margin: 2px 0 0; } footer > span { display: none; } footer { justify-content: flex-end; } .skill-add { min-height: 44px; } }
</style>
