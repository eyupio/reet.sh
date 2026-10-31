<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { shell } from '../dialogShell'
  import { UnauthorizedError, listModelFavourites, type Skill } from '../api'
  import { agentChoices } from '../catalogue'
  import { cleanModel, modelSuggestions, notStartedText, refusalText, toolName, validateModel } from '../agentLaunch'
  import { effortChoices, modelOptions, modelStatus, type MachineModels, type ModelFavourite } from '../models'
  import { installPrompt } from '../install'
  import { insertSkillPrompt } from '../skillPrompt'
  import { formatImageSize, imageFiles, imagePrompt, MAX_IMAGES, MAX_IMAGE_TOTAL_BYTES, validateImage } from '../terminal/upload'
  import SlotPicker, { type SlotOption } from '../launch/SlotPicker.svelte'
  import SkillPromptPicker from './SkillPromptPicker.svelte'
  import { initialChoice, startable, toCreate, withAgent, withProject, type LaunchChoice, type LaunchCreate, type LaunchDefaults } from '../launchBar'
  import { createTerminal, launchUpload, machineModels, refusalSentence, structuredAgent, TerminalError, type FolderProject, type TerminalMachine, type TerminalSession } from '../terminal/api'

  // Starts a session straight from the Active column. The interface choice is the launch bar's own: the reet TUI is the default
  // wherever the machine offers it, and the agent's own TUI is one click away. Everything else (defaults, refusals, the one
  // idempotent create) is the launch bar's logic, so both start sessions the same way.
  let { projects, machines, profile, initialProject = '', onstarted, onsignedout, onclose }: {
    projects: FolderProject[]; machines: TerminalMachine[]; profile: LaunchDefaults; initialProject?: string
    onstarted: (session: TerminalSession) => void; onsignedout: (e: unknown) => void; onclose: () => void
  } = $props()

  const id = $props.id()
  const life = new AbortController()
  let dialog = $state<HTMLDialogElement>()
  let busy = $state(false), message = $state('')
  // Kept while a start's outcome is unknown, so Retry asks about the same request and cannot start a second agent.
  let request: (LaunchCreate & { structured: boolean }) | null = null, requestPrompt = ''
  const first = (): LaunchChoice => {
    const options = { machines }
    const start = initialChoice(projects, profile, options)
    const picked = projects.find(p => p.id === initialProject)
    return picked ? withProject(start, picked, profile, options) : start
  }
  let choice = $state<LaunchChoice>(first())
  const project = $derived(projects.find(p => p.id === choice.projectIds[0]) ?? null)
  const machine = $derived(machines.find(m => m.agentId === project?.host))
  const offers = $derived(machine ? agentChoices(machine).filter(o => o.available) : [])
  const hasChat = $derived(!!structuredAgent(machine?.capabilities, choice.agent))
  const chat = $derived(hasChat && choice.chat === true)
  const why = $derived(startable(choice, projects, machines, profile))

  // Model, effort and skills, as in the plan dialog. The model list is read from the machine the project lives on.
  const efforts = $derived(effortChoices(choice.agent))
  const takesEffort = $derived(!chat && efforts.length > 1)
  let favourites = $state<ModelFavourite[]>([])
  let modelRead = $state<{ key: string; machine: MachineModels | null; loading: boolean }>({ key: '', machine: null, loading: false })
  const modelKey = $derived(machine && choice.agent ? `${machine.agentId}\u0000${choice.agent}` : '')
  const modelMachine = $derived(modelRead.key === modelKey ? modelRead.machine : null)
  const modelLine = $derived.by(() => {
    if (!modelKey) return ''
    if (modelRead.key === modelKey && modelRead.loading) return `Reading ${machine?.host ?? 'the machine'}’s models…`
    const info = modelStatus(modelMachine, machine?.host ?? '')
    return [info.sentence, info.age ? `Read ${info.age}.` : ''].filter(Boolean).join(' ')
  })
  let modelOpen = $state(false), effortOpen = $state(false), skillOpen = $state(false), touch = $state(false)
  let modelButton = $state<HTMLButtonElement>(), effortButton = $state<HTMLButtonElement>(), skillButton = $state<HTMLButtonElement>(), promptInput = $state<HTMLTextAreaElement>()
  $effect(() => {
    const key = modelKey, target = machine?.agentId ?? '', agent = choice.agent
    if (!key || !target || !machine?.ready || !machine.capabilities?.models || machine.capabilities.modelsOff) return
    if (untrack(() => modelRead.key === key)) return
    modelRead = { key, machine: null, loading: true }
    machineModels(target, agent, life.signal).then(
      next => { if (!life.signal.aborted && modelRead.key === key) modelRead = { key, machine: next, loading: false } },
      () => { if (!life.signal.aborted && modelRead.key === key) modelRead = { key, machine: null, loading: false } })
  })
  const modelChoices = (): SlotOption[] => modelOptions({
    tool: choice.agent, current: cleanModel(choice.model), machine: modelMachine, favourites, recent: [], suggestions: modelSuggestions(choice.agent),
    selectedModel: cleanModel(choice.model), host: machine?.host ?? '', agentName: toolName(choice.agent),
  })
  const effortOptions = (): SlotOption[] => efforts.map(value => ({ value, label: value || 'Agent default' }))
  function useSkill(skill: Skill) {
    const text = installPrompt(skill)
    if (!text) { message = 'That skill has no source to run it from. Link one on the Skills page.'; return }
    const at = promptInput?.selectionStart ?? choice.prompt.length, end = promptInput?.selectionEnd ?? at
    const inserted = insertSkillPrompt(choice.prompt, text, at, end)
    choice = { ...choice, prompt: inserted.text }
    void tick().then(() => { promptInput?.focus(); promptInput?.setSelectionRange(inserted.caret, inserted.caret) })
  }

  // Images pasted, dropped or chosen before a start live only in this page; each is uploaded to the machine as the session
  // starts and its path joins the opening prompt (the launch bar's way).
  type Attachment = { id: number; file: File; preview: string }
  let attachments = $state<Attachment[]>([]), attachmentSeq = 0
  const imagesReason = $derived.by(() => {
    if (!attachments.length) return ''
    if (!machine?.capabilities) return ''
    if (!machine.capabilities.uploads) return machine.capabilities.uploadsOff ? 'Image uploads are off on this machine.' : 'Update reet on that machine to attach images.'
    return ''
  })
  function addImages(files: File[]) {
    if (busy || !files.length) return
    const accepted: Attachment[] = [], reasons: string[] = []
    let count = attachments.length, bytes = attachments.reduce((t, a) => t + a.file.size, 0)
    for (const file of files) {
      if (count >= MAX_IMAGES) { reasons.push(`Attach at most ${MAX_IMAGES} images.`); break }
      if (bytes + file.size > MAX_IMAGE_TOTAL_BYTES) { reasons.push('Those images are larger than 50 MiB together.'); break }
      const check = validateImage({ type: file.type, size: file.size })
      if (!check.ok) { reasons.push(check.reason === 'empty' ? 'That image is empty.' : check.reason === 'size' ? 'That image is larger than 5 MiB.' : 'Attach a PNG, JPEG, GIF or WebP image.'); continue }
      accepted.push({ id: ++attachmentSeq, file, preview: URL.createObjectURL(file) })
      count++; bytes += file.size
    }
    if (accepted.length) attachments = [...attachments, ...accepted]
    message = reasons.length === 1 ? reasons[0] : reasons.length ? `${reasons.length} images were refused. ${reasons[0]}` : ''
  }
  function removeImage(id: number) {
    const item = attachments.find(x => x.id === id)
    if (item) URL.revokeObjectURL(item.preview)
    attachments = attachments.filter(x => x.id !== id)
    message = ''
  }
  function pasted(event: ClipboardEvent) {
    const files = imageFiles(event.clipboardData?.items)
    if (!files.length) return
    event.preventDefault(); addImages(files)
  }

  function pickProject(value: string) {
    const next = projects.find(p => p.id === value)
    if (next) choice = withProject(choice, next, profile, { machines })
  }
  function pickAgent(value: string) { choice = withAgent(choice, value, project ? { ...project } : null, profile, { machines }) }
  function pickTui(reet: boolean) { const { trust: _t, ...rest } = choice; choice = { ...rest, chat: reet } }

  async function start(event: SubmitEvent) {
    event.preventDefault()
    if (busy || why) { if (why) message = why; return }
    if (imagesReason) { message = imagesReason; return }
    let made: LaunchCreate | null = null
    if (!request || requestPrompt !== choice.prompt) made = toCreate(choice, project, crypto.randomUUID(), crypto.randomUUID().replaceAll('-', ''), profile)
    if (made && attachments.length) {
      // The images go to the machine before the session exists; their paths join the opening prompt.
      busy = true
      const references: string[] = []
      try {
        for (let i = 0; i < attachments.length; i++) {
          message = attachments.length === 1 ? 'Attaching the image…' : `Attaching image ${i + 1} of ${attachments.length}…`
          references.push((await launchUpload(made.agentId, attachments[i].file, life.signal)).reference)
        }
      } catch (e) {
        busy = false
        if (life.signal.aborted) return
        if (e instanceof TerminalError && e.status === 401) { onsignedout(new UnauthorizedError()); return }
        message = `Couldn't attach that image. ${refusalSentence(e)}`
        return
      }
      if (life.signal.aborted) return
      made.launch.prompt = imagePrompt(made.launch.prompt ?? '', references)
    }
    if (made) { request = { ...made, structured: !!made.launch.structured }; requestPrompt = choice.prompt }
    const op = request!
    busy = true; message = `Starting ${toolName(op.launch.tool)}…`
    try {
      const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(op.structured ? 60000 : 15000)]), op.launch)
      if (life.signal.aborted) return
      request = null
      for (const a of attachments) URL.revokeObjectURL(a.preview)
      attachments = []
      const notStarted = notStartedText(session.state)
      if (notStarted) { message = notStarted; return }
      onstarted(session)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status !== 503) {
        request = null
        message = refusalText(e.status, e.code, e.message)
        if (e.status === 401) onsignedout(new UnauthorizedError())
      } else message = "We didn't hear back, so the agent may have started. Check Active before starting another, or press Start to retry the same request."
    } finally { busy = false }
  }
  function close() { if (!busy) { dialog?.close(); onclose() } }
  onMount(() => {
    touch = matchMedia('(pointer: coarse)').matches
    listModelFavourites(life.signal).then(a => { if (!life.signal.aborted) favourites = a.favourites ?? [] }, () => { /* a help only */ })
    dialog?.showModal(); return () => { life.abort(); dialog?.close(); for (const a of attachments) URL.revokeObjectURL(a.preview) } })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`} oncancel={e => { e.preventDefault(); close() }}>
  <form class="sheet" data-shell-card onsubmit={start}>
    <header data-shell-head>
      <div>
        <p class="eyebrow" data-shell-aside>New session</p>
        <h2 id={`${id}-title`}>Start a session</h2>
        <p class="lede">Pick a project and an agent, then start it now.</p>
      </div>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close} disabled={busy}>×</button>
    </header>
    <div class="body" data-shell-body>
      {#if !projects.length}
        <p class="none">Add a project from Home first, then start a session in it.</p>
      {:else}
        <fieldset disabled={busy}>
          <label class="field"><span>Project</span>
            <select aria-label="Project" value={choice.projectIds[0] ?? ''} onchange={e => pickProject(e.currentTarget.value)}>
              {#each projects as p}<option value={p.id}>{p.name} · {machines.find(m => m.agentId === p.host)?.host ?? p.machine}</option>{/each}
            </select></label>
          <label class="field"><span>Agent</span>
            <select aria-label="Agent" value={choice.agent} onchange={e => pickAgent(e.currentTarget.value)}>
              {#each offers as o}<option value={o.agent.id}>{o.agent.name}</option>{/each}
              {#if !offers.some(o => o.agent.id === choice.agent)}<option value={choice.agent}>{toolName(choice.agent)}</option>{/if}
            </select></label>
          <div class="field"><span>Interface</span>
            <div class="mode" role="group" aria-label="Interface">
              <button type="button" class:on={chat} aria-pressed={chat} disabled={!hasChat} title={hasChat ? '' : 'This machine does not offer the reet TUI for that agent.'} onclick={() => pickTui(true)}>reet TUI</button>
              <button type="button" class:on={!chat} aria-pressed={!chat} onclick={() => pickTui(false)}>Agent TUI</button>
            </div>
            <small>{chat ? 'A native chat with approvals, history and images.' : `${toolName(choice.agent)}’s own terminal interface.`}</small>
          </div>
          <div class="settings">
            <div class="field"><span>Model</span>
              <button type="button" class="picker" bind:this={modelButton} aria-label={`Model: ${choice.model || 'Agent default'}`} aria-haspopup="listbox" aria-expanded={modelOpen} onclick={() => modelOpen = !modelOpen}><span class="value">{choice.model || 'Agent default'}</span><span class="chev" aria-hidden="true">▾</span></button></div>
            <div class="field"><span>Effort</span>
              <button type="button" class="picker" bind:this={effortButton} aria-label={`Effort: ${choice.effort || 'Agent default'}`} aria-haspopup="listbox" aria-expanded={effortOpen} disabled={!takesEffort} title={takesEffort ? '' : `${toolName(choice.agent)} chooses its own effort.`} onclick={() => effortOpen = !effortOpen}><span class="value">{takesEffort ? choice.effort || 'Agent default' : 'This agent decides'}</span>{#if takesEffort}<span class="chev" aria-hidden="true">▾</span>{/if}</button></div>
          </div>
          <label class="field"><span>Prompt <small>Optional</small></span>
            <textarea bind:this={promptInput} aria-label="Prompt" rows="4" bind:value={choice.prompt} onpaste={pasted} placeholder="What should the agent do?"></textarea></label>
          <div class="tools"><button type="button" class="skill-add" bind:this={skillButton} aria-haspopup="listbox" aria-expanded={skillOpen} onclick={() => skillOpen = !skillOpen}>＋ Use a skill</button>
            <label class="attach">＋ Attach images<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple onchange={e => { addImages(Array.from(e.currentTarget.files ?? [])); e.currentTarget.value = '' }} /></label></div>
          {#if attachments.length}
            <ul class="attachments" aria-label="Images to attach">
              {#each attachments as item (item.id)}
                <li><img src={item.preview} alt="" /><span>{item.file.name || 'image'} · {formatImageSize(item.file.size)}</span><button type="button" aria-label={`Remove ${item.file.name || 'image'}`} onclick={() => removeImage(item.id)}>×</button></li>
              {/each}
            </ul>
          {/if}
          {#if chat}
            <label class="trust"><input type="checkbox" checked={choice.trust === true} onchange={e => choice = { ...choice, trust: e.currentTarget.checked }} />I trust this folder. OpenCode plugins in it may run.</label>
          {/if}
        </fieldset>
      {/if}
      {#if message || imagesReason || (why && projects.length)}<p class="note" role="status">{message || imagesReason || why}</p>{/if}
    </div>
    <footer data-shell-foot>
      <button type="button" class="btn" onclick={close} disabled={busy}>Cancel</button>
      <button class="btn primary" disabled={busy || !projects.length || !!why || !!imagesReason}>{busy ? 'Starting…' : 'Start'}</button>
    </footer>
  </form>
  {#if modelOpen}
    <SlotPicker label="Model" options={modelChoices()} value={cleanModel(choice.model)} sheet={touch} opener={modelButton ?? null}
      filter="Filter models" status={modelLine}
      other={{ label: 'Other…', field: 'Model name', validate: validateModel }}
      onpick={value => { choice = { ...choice, model: value }; modelOpen = false }} onclose={() => modelOpen = false} />
  {/if}
  {#if effortOpen}
    <SlotPicker label="Effort" options={effortOptions()} value={choice.effort} sheet={touch} opener={effortButton ?? null}
      onpick={value => { choice = { ...choice, effort: value as LaunchChoice['effort'] }; effortOpen = false }} onclose={() => effortOpen = false} />
  {/if}
  {#if skillOpen}
    <SkillPromptPicker opener={skillButton ?? null} sheet={touch} oninsert={useSkill} onclose={() => skillOpen = false} />
  {/if}
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); padding: 0; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card); color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; max-height: 90dvh; }
  header { display: flex; justify-content: space-between; gap: 16px; padding: 22px 24px 16px; border-bottom: 1px solid var(--line); }
  .eyebrow { margin: 0; font-size: 10px; font-weight: 650; letter-spacing: .13em; text-transform: uppercase; color: var(--accent-text); }
  h2 { font: 500 24px/1.2 var(--serif); margin: 6px 0; }
  .lede { margin: 0; color: var(--muted); font-size: 13px; }
  .close { align-self: start; width: 34px; height: 34px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font-size: 22px; cursor: pointer; }
  .body { padding: 18px 24px; overflow-y: auto; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
  .field { display: flex; flex-direction: column; gap: 7px; margin-bottom: 15px; font-size: 13px; font-weight: 600; }
  .field small { font-size: 11px; font-weight: 400; color: var(--muted); }
  select, textarea { width: 100%; box-sizing: border-box; font-size: 13px; font-weight: 400; color: var(--ink); }
  textarea { resize: vertical; min-height: 90px; }
  .mode { display: inline-flex; align-self: flex-start; border: 1px solid var(--line); border-radius: 8px; padding: 3px; background: var(--bg-sunk); }
  .mode button { padding: 8px 14px; border: 1px solid transparent; border-radius: 5px; background: transparent; color: var(--muted); font: 500 12px var(--sans); cursor: pointer; }
  .mode button.on { background: var(--card); color: var(--ink); border-color: var(--line); }
  .mode button:disabled { opacity: .5; cursor: default; }
  .settings { display: grid; grid-template-columns: 1.2fr 1fr; gap: 12px; }
  .picker { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; min-height: 38px; padding: 8px 10px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card); color: var(--ink); font: 400 13px var(--sans); text-align: left; cursor: pointer; }
  .picker:disabled { opacity: .55; cursor: default; }
  .picker .value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .picker .chev { flex: none; color: var(--muted); }
  .tools { margin: -8px 0 14px; display: flex; gap: 14px; flex-wrap: wrap; align-items: center; }
  .attach { position: relative; padding: 7px 0; color: var(--accent-text); font: 600 11px var(--sans); cursor: pointer; }
  .attach input { position: absolute; inset: 0; width: 100%; height: 100%; opacity: 0; cursor: pointer; }
  .attach:focus-within { outline: 2px solid var(--accent); outline-offset: 3px; }
  .attachments { display: flex; flex-wrap: wrap; gap: 6px; margin: 0 0 14px; padding: 0; list-style: none; }
  .attachments li { display: flex; align-items: center; gap: 8px; border: 1px solid var(--line); border-radius: 8px; padding: 4px 6px; font-size: 11px; max-width: 100%; }
  .attachments img { width: 32px; height: 32px; object-fit: cover; border-radius: 4px; }
  .attachments span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .attachments button { border: 0; background: none; color: var(--muted); cursor: pointer; font-size: 18px; min-width: 28px; }
  .skill-add { border: 0; background: none; padding: 7px 0; color: var(--accent-text); font: 600 11px var(--sans); cursor: pointer; }
  .skill-add:hover { text-decoration: underline; }
  .trust { display: flex; align-items: center; gap: 8px; font-size: 12px; }
  .note, .none { font-size: 12px; color: var(--muted); margin: 8px 0 0; }
  footer { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 24px; border-top: 1px solid var(--line); }
  @media (max-width: 650px) { select, textarea, .picker { font-size: 16px; } .settings { grid-template-columns: 1fr; gap: 0; } .skill-add { min-height: 44px; } header, .body, footer { padding-left: 16px; padding-right: 16px; } }
</style>
