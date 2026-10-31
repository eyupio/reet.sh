<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { CATALOGUE } from '../catalogue'
  import { defaultTitle } from '../launchBar'
  import { modelSuggestions, validateDir, validateModel, validatePrompt } from '../agentLaunch'
  import { resolveLaunchDefaults } from '../models'
  import { machineModels, type FolderProject, type TerminalMachine } from '../terminal/api'
  import { saveDraft } from '../sessionBacklog'
  import type { DraftImage, SessionDraft } from '../sessionBoard'
  import type { LaunchDefaults } from '../launchBar'
  import FolderBrowser from './FolderBrowser.svelte'

  let { draft = null, projects, machines, profile, initialProject = '', onsaved, onclose, onerror }: {
    draft?: SessionDraft | null; projects: FolderProject[]; machines: TerminalMachine[]; profile: LaunchDefaults
    initialProject?: string; onsaved: (draft: SessionDraft) => void; onclose: () => void; onerror: (e: unknown) => void
  } = $props()
  const id = $props.id()
  let dialog: HTMLDialogElement
  let title = $state(''), prompt = $state(''), projectId = $state(''), agentId = $state(''), dir = $state('')
  let tool = $state(''), model = $state(''), effort = $state(''), mode = $state<'project' | 'folder'>('project')
  let images = $state<DraftImage[]>([]), busy = $state(false), reading = $state(false), message = $state('')
  let browse = $state(false), suggestions = $state<string[]>([])
  let titleTouched = $state(false)
  const enabled = CATALOGUE.filter(a => a.enabled)
  const machine = $derived(machines.find(m => m.agentId === agentId))
  const bytes = $derived(new TextEncoder().encode(prompt).length)
  const initial = () => JSON.stringify({ title, prompt, projectId, agentId, dir, tool, model, effort, mode, images })
  let original = '', confirmClose = $state(false)

  function defaults(project?: FolderProject) {
    const resolved = resolveLaunchDefaults({ project, account: profile, availableAgents: enabled.map(a => a.id) })
    tool = resolved.agent; model = resolved.model; effort = resolved.effort
  }
  function chooseProject() {
    const p = projects.find(p => p.id === projectId)
    if (!p) return
    agentId = p.host; dir = p.dir; browse = false; defaults(p)
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
    const agent = tool, target = agentId
    suggestions = modelSuggestions(agent)
    let current = true
    if (target && agent && machine?.ready) machineModels(target, agent).then(result => {
      if (current) suggestions = [...new Set([...result.models.map(m => m.id), ...modelSuggestions(agent)])]
    }, () => {})
    return () => { current = false }
  })
  onMount(() => {
    if (draft) {
      title = draft.title; prompt = draft.prompt; projectId = draft.projectId; agentId = draft.agentId; dir = draft.dir
      tool = draft.tool; model = draft.model; effort = draft.effort; images = [...(draft.images ?? [])]
      mode = projectId ? 'project' : 'folder'; titleTouched = true
    } else {
      projectId = initialProject || projects[0]?.id || ''; mode = projectId ? 'project' : 'folder'
      if (projectId) chooseProject(); else { agentId = machines[0]?.agentId ?? ''; defaults() }
    }
    original = initial(); dialog.showModal()
  })
  function close() {
    if (busy || reading) return
    if (original !== initial()) { confirmClose = true; return }
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
    const why = !agentId ? 'Choose a machine.' : mode === 'project' && !projectId ? 'Choose a project.' : !prompt.trim() ? 'Describe what the agent should do.' : validatePrompt(tool, prompt) || validateModel(model) || (dir ? validateDir(dir) : null)
    if (why) { message = why; return }
    busy = true; message = ''
    try {
      const saved = await saveDraft({ title: title.trim() || defaultTitle(prompt), prompt, projectId: mode === 'project' ? projectId : '', agentId, dir, tool, model: model.trim(), effort, images, version: draft?.version ?? 0 }, draft?.id, AbortSignal.timeout(20_000))
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
        <div class="discard" role="alert"><strong>Discard your unsaved changes?</strong><div><button type="button" class="btn" onclick={() => confirmClose = false}>Keep editing</button><button type="button" class="btn" onclick={onclose}>Discard changes</button></div></div>
      {/if}
      <fieldset disabled={busy || reading}>
        <label class="field"><span>Session title <small>Optional · suggested from your prompt</small></span><input aria-label="Session title" bind:value={title} oninput={() => titleTouched = true} maxlength="80" placeholder="A short name for this piece of work" /></label>
        <label class="field"><span>Prompt <em>Required</em></span><textarea aria-label="Prompt" bind:value={prompt} rows="6" required placeholder="What should the agent do? Include the context, the outcome you want, and anything it should keep in mind." onpaste={e => { const files = Array.from(e.clipboardData?.files ?? []); if (files.length) { e.preventDefault(); void attach(files) } }}></textarea></label>
        <div class="prompt-foot"><label class="attach">＋ Attach images<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple disabled={reading} onchange={e => { void attach(Array.from(e.currentTarget.files ?? [])); e.currentTarget.value = '' }} /></label><span class:over={bytes > 2000}>{bytes.toLocaleString()} / 2,000 bytes</span></div>
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
        <label class="field"><span>Model</span><input aria-label="Model" bind:value={model} list={`${id}-models`} placeholder="Agent default" maxlength="100" /><datalist id={`${id}-models`}>{#each suggestions as suggestion}<option value={suggestion}></option>{/each}</datalist></label>
        <label class="field"><span>Effort</span><select aria-label="Effort" bind:value={effort} disabled={!CATALOGUE.find(a => a.id === tool)?.effort}><option value="">Agent default</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option></select></label></div>
        <p class="hint">{machine && !machine.ready ? 'This machine is offline. You can save now and start when it reconnects.' : 'Saving to the backlog keeps everything ready for later.'} Uses the agent’s normal approval settings.</p>
      </fieldset>
      {#if message}<p class="error" role="alert">{message}</p>{/if}
    </div>
    <footer data-shell-foot><span>{reading ? 'Reading images…' : 'Your prompt and images are saved to your account.'}</span><button type="button" class="btn" onclick={close} disabled={busy || reading}>Cancel</button><button class="btn primary" disabled={busy || reading || bytes > 2000 || !machines.length}>{busy ? 'Saving…' : draft ? 'Save changes' : 'Add to backlog'}</button></footer>
  </form>
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
  .prompt-foot { display: flex; justify-content: space-between; align-items: center; margin-top: -10px; color: var(--muted); font-size: 11px; }
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
  .hint { font-size: 12px; color: var(--muted); margin: 0; line-height: 1.6; }
  footer { display: flex; align-items: center; gap: 8px; padding: 18px 28px; border-top: 1px solid var(--line); background: var(--bg); }
  footer > span { flex: 1; font-size: 11px; color: var(--muted); }
  .error, .discard { border: 1px solid var(--warn); padding: 12px; border-radius: 8px; color: var(--warn); font-size: 13px; }
  .discard { margin-bottom: 18px; }.discard div { display: flex; gap: 8px; margin-top: 10px; }
  .over { color: var(--bad); }
  @media (max-width: 650px) { input, textarea, select { font-size: 16px; } header, .body, footer { padding: 18px; } h2 { font-size: 23px; } .settings { grid-template-columns: 1fr; gap: 0; } .field small { display: block; margin: 2px 0 0; } footer > span { display: none; } footer { justify-content: flex-end; } }
</style>
