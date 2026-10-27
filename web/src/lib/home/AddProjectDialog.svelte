<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import { shell } from '../dialogShell'
  import { canAutoFocus } from '../focus'
  import { MAX_PROMPT_BYTES, TOOLS, cleanModel, validateModel, validatePrompt } from '../agentLaunch'
  import { repoMessage, topFolder, validateFolderInput } from '../folderBrowser'
  import { browseFolder, createFolderProject, listBranches, readiness, refusalSentence, updateFolderProject, TerminalError,
    type BrowseRepo, type BrowseResult, type FolderProject, type FolderProjectInput, type TerminalMachine, type WorktreeDefault } from '../terminal/api'
  import FolderBrowser from './FolderBrowser.svelte'

  // Add a project (one folder on one machine), or change one. The folder is typed or picked by browsing the machine's folders
  // live; leaving the typed field asks the machine once whether it exists and is a repository. A machine that cannot browse
  // (offline, an older reet, browsing switched off) still takes a typed path, checked again at launch. One save per press; a
  // change names the version it was read at. The starter prompt lives only in this component while it is open: never in
  // browser storage, the URL or a log.
  let { machines, project, opener = null, onadded, onclose }: {
    machines: TerminalMachine[]
    /** The project to change, read with its starter prompt (the single read); absent adds a new one. */
    project?: FolderProject
    opener?: HTMLElement | null
    /** After a save: the project as the server answered it. The dialog then closes; the page shows the success line. */
    onadded: (project: FolderProject) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  // The Machines page opens Add a machine when it arrives with this fragment (as Home builds the link).
  const machinesAt = (fragment: string) => `/machines#${fragment}`
  // svelte-ignore state_referenced_locally
  const editing = project
  const session = crypto.randomUUID()
  const life = new AbortController()

  const machineFor = (agentId: string) => machines.find(m => m.agentId === agentId)
  /** Why this machine's folders cannot be listed now, or '' when they can. */
  function browseBlock(m: TerminalMachine | undefined): string {
    if (!m) return editing ? 'That machine is no longer on this account.' : 'Choose a machine.'
    const r = readiness(m)
    if (r) return r
    if (m.capabilities?.browseOff) return 'That machine has folder browsing switched off.'
    if (!m.capabilities?.browse) return 'Update reet on that machine to browse its folders.'
    return ''
  }
  // svelte-ignore state_referenced_locally
  let host = $state(editing?.host ?? (machines.find(m => !browseBlock(m)) ?? machines[0])?.agentId ?? '')
  let dir = $state(editing?.dir ?? '')
  let name = $state(editing?.name ?? '')
  let worktreeDefault = $state<WorktreeDefault>(editing?.worktreeDefault ?? 'inherit')
  let starterPrompt = $state(editing?.starterPrompt ?? '')
  let baseBranch = $state(editing?.baseBranch ?? '')
  let defaultTool = $state(editing?.defaultTool ?? '')
  let defaultModel = $state(editing?.defaultModel ?? '')
  // A project read without its prompt (only the single read carries it) keeps its saved prompt as it is.
  const promptKnown = !editing || editing.starterPrompt !== undefined || !editing.hasStarterPrompt
  const machine = $derived(machineFor(host))
  const block = $derived(browseBlock(machine))
  // svelte-ignore state_referenced_locally
  let browsing = $state(!editing && !browseBlock(machineFor(host)))
  // What the machine said about each folder this dialog has seen: whether it is a repository.
  const known = new SvelteMap<string, BrowseRepo>()
  let fieldNote = $state<{ text: string; kind: 'repo' | 'plain' | 'bad'; top?: string } | null>(null)
  let checked = ''
  let checkSeq = 0
  // svelte-ignore state_referenced_locally
  if (editing) { checked = editing.dir; if (editing.isRepo) known.set(editing.dir, 'root') }

  let dialog = $state<HTMLDialogElement>(), folderInput = $state<HTMLInputElement>(), submitButton = $state<HTMLButtonElement>()
  let promptFold = $state<HTMLDetailsElement>(), moreFold = $state<HTMLDetailsElement>()
  let busy = $state(false), message = $state(''), failed = $state(false)
  let branches = $state<string[]>([]), branchNote = $state('')
  // One save: exactly what is sent, kept while its outcome is unknown so Try again sends the same thing.
  let operation = $state<{ input: FolderProjectInput; lost: boolean } | null>(null)

  const folderProblem = $derived(validateFolderInput(dir))
  const ready = $derived(host !== '' && folderProblem === null)
  const promptBytes = $derived(new TextEncoder().encode(starterPrompt).length)
  const chosenRepo = $derived(known.get(dir))

  // Browse requests one at a time for this dialog: the machine answers a second at once with "already listing".
  let queue: Promise<unknown> = Promise.resolve()
  function serial<T>(job: () => Promise<T>): Promise<T> {
    const next = queue.then(job, job)
    queue = next.catch(() => undefined)
    return next
  }

  function noteFor(r: Pick<BrowseResult, 'repo' | 'repoTop' | 'home'>): NonNullable<typeof fieldNote> {
    if (r.repo === 'root') return { text: 'A git repository.', kind: 'repo' }
    return { text: repoMessage(r) ?? '', kind: 'plain', top: topFolder(r) ?? undefined }
  }
  // Leaving the field: one browse of the typed folder, only when it changed and the machine can be asked.
  async function checkFolder() {
    const v = dir
    if (v === '') { fieldNote = null; return }
    const problem = validateFolderInput(v)
    if (problem) { fieldNote = { text: problem, kind: 'bad' }; checked = ''; return }
    if (block || v === checked) return
    checked = v
    const mine = ++checkSeq
    try {
      const r = await serial(() => browseFolder(host, { path: v, session }, life.signal))
      if (mine !== checkSeq || dir !== v || life.signal.aborted) return
      known.set(v, r.repo)
      fieldNote = noteFor(r)
      void readBranches()
    } catch (e) {
      if (mine !== checkSeq || dir !== v || life.signal.aborted) return
      checked = ''
      fieldNote = { text: refusalSentence(e), kind: 'bad' }
    }
  }
  function use(picked: Pick<BrowseResult, 'path' | 'home' | 'repo' | 'repoTop'>) {
    dir = picked.path
    checked = picked.path
    checkSeq++
    known.set(picked.path, picked.repo)
    clearBranches()
    void readBranches()
    fieldNote = noteFor(picked)
    browsing = false
    edited()
    void tick().then(() => folderInput?.focus())
  }
  function machineChanged() {
    known.clear(); checked = ''; checkSeq++; fieldNote = null; clearBranches()
    if (block) browsing = false
    else if (!editing) browsing = true
    edited()
  }
  // The base branch suggestions belong to one folder on one machine: dropped whenever the folder or machine changes, and
  // read while More defaults is open on a repository's top folder (when it opens, or when the folder becomes one). An
  // answer for a folder no longer chosen is dropped.
  let branchesFor = '', branchSeq = 0
  function clearBranches() { branchSeq++; branchesFor = ''; branches = []; branchNote = '' }
  async function readBranches() {
    const key = `${host}\u0000${dir}`
    if (!moreFold?.open || branchesFor === key || !machine?.capabilities?.branches || known.get(dir) !== 'root' || block) return
    const mine = ++branchSeq
    branchesFor = key
    try {
      const r = await listBranches(host, dir, undefined, life.signal)
      if (mine === branchSeq) { branches = r.branches; branchNote = '' }
    } catch (e) {
      if (mine === branchSeq && !life.signal.aborted) { branches = []; branchNote = refusalSentence(e) }
    }
  }

  function shutdown(returnFocus: boolean) {
    life.abort()
    starterPrompt = ''
    operation = null
    dialog?.close()
    if (returnFocus && opener?.isConnected) opener.focus()
  }
  // While a save is in flight the dialog stays: the 15-second limit ends the wait.
  function close() { if (busy) return; shutdown(true); onclose() }
  // A change clears a refusal. After a lost reply the fields are locked, so Try again sends exactly what was sent.
  function edited() { if (!busy && !operation && message) { message = ''; failed = false } }

  function inputFor(): FolderProjectInput {
    const repo = known.get(dir)
    const isRepo = repo === undefined ? undefined : repo === 'root'
    const n = name.trim(), model = cleanModel(defaultModel), branch = baseBranch.trim()
    if (!editing) {
      const input: FolderProjectInput = { host, dir }
      if (isRepo !== undefined) input.isRepo = isRepo
      if (n) input.name = n
      input.worktreeDefault = worktreeDefault
      if (starterPrompt) input.starterPrompt = starterPrompt
      if (branch) input.baseBranch = branch
      if (defaultTool) input.defaultTool = defaultTool
      if (model) input.defaultModel = model
      return input
    }
    // A change sends only what changed: absent keeps, "" clears.
    const input: FolderProjectInput = {}
    if (host !== editing.host) input.host = host
    if (dir !== editing.dir) input.dir = dir
    if (isRepo !== undefined && isRepo !== editing.isRepo) input.isRepo = isRepo
    if (n && n !== editing.name) input.name = n
    if (worktreeDefault !== editing.worktreeDefault) input.worktreeDefault = worktreeDefault
    if (promptKnown && starterPrompt !== (editing.starterPrompt ?? '')) input.starterPrompt = starterPrompt
    if (branch !== editing.baseBranch) input.baseBranch = branch
    if (defaultTool !== editing.defaultTool) input.defaultTool = defaultTool
    if (model !== editing.defaultModel) input.defaultModel = model
    return input
  }

  const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 410, 413, 422, 429].includes(e.status)
  // The field a refusal names, so focus goes there (opening its fold first).
  async function focusField(field: string) {
    const fold = field === 'starterPrompt' ? promptFold : ['baseBranch', 'defaultTool', 'defaultModel'].includes(field) ? moreFold : undefined
    if (fold) fold.open = true
    await tick()
    if (life.signal.aborted) return
    const target = ({ host: 'machine', dir: 'folder', name: 'name', worktreeDefault: 'worktree', starterPrompt: 'prompt', baseBranch: 'branch', defaultTool: 'tool', defaultModel: 'model' } as Record<string, string>)[field]
    const el = target ? dialog?.querySelector<HTMLElement>(`#${CSS.escape(`${id}-${target}`)}`) : null
    ;(el ?? submitButton)?.focus()
  }
  // Control and format characters and the two Unicode line separators, as the server refuses them.
  const NAME_BAD = new RegExp('[\\p{Cc}\\p{Cf}\\u2028\\u2029]', 'u')
  function nameProblem(): string {
    const n = name.trim()
    if (n === '') return ''
    if (Array.from(n).length > 80 || NAME_BAD.test(n)) return 'Give the project a name of 1 to 80 characters, without line breaks or control characters.'
    return ''
  }

  async function save() {
    if (busy || life.signal.aborted || !ready) return
    if (!operation) {
      const invalid: [string, string] | null = nameProblem() ? ['name', nameProblem()]
        : promptKnown && validatePrompt(defaultTool, starterPrompt) ? ['starterPrompt', validatePrompt(defaultTool, starterPrompt)!]
        : validateModel(defaultModel) ? ['defaultModel', validateModel(defaultModel)!] : null
      if (invalid) { failed = true; message = invalid[1]; void focusField(invalid[0]); return }
      const input = inputFor()
      if (editing && Object.keys(input).length === 0) { failed = false; message = 'Nothing has changed.'; return }
      operation = { input, lost: false }
    }
    const op = operation
    busy = true; failed = false
    message = editing ? 'Saving your changes…' : 'Adding the project…'
    try {
      const signal = AbortSignal.any([life.signal, AbortSignal.timeout(15000)])
      const saved = editing ? await updateFolderProject(editing.id, op.input, editing.version, signal) : await createFolderProject(op.input, signal)
      if (life.signal.aborted) return
      const done = onadded
      shutdown(true)
      done(saved)
      onclose()
    } catch (e) {
      if (life.signal.aborted) return
      failed = true
      busy = false
      if (definite(e)) {
        const err = e as TerminalError
        const wasLost = op.lost
        operation = null
        if (wasLost && !editing && err.code === 'project_exists') message = `${err.message} Your earlier request may have gone through: close this and look for it in your projects.`
        else if (wasLost && editing && err.code === 'project_changed') message = 'This project changed since it was read. Your earlier save may have gone through. Close this and open it again to see.'
        else message = err.message
        void focusField(err.field ?? '')
      } else {
        op.lost = true
        message = editing
          ? "We didn't hear back, so your changes may have been saved. Try again: if they were, reet says the project changed."
          : "We didn't hear back, so the project may have been added. Try again: if it was, reet says that folder already has a project."
        void tick().then(() => submitButton?.focus())
      }
    } finally {
      if (!life.signal.aborted) busy = false
    }
  }

  onMount(() => {
    dialog?.showModal()
    if (canAutoFocus()) folderInput?.focus()
    return () => { life.abort(); starterPrompt = '' }
  })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'full' }} class:browsing aria-labelledby={`${id}-title`} oncancel={(event) => { event.preventDefault(); close() }}>
  <header data-shell-head>
    <div class="titles">
      <p class="eyebrow">Projects</p>
      <h2 id={`${id}-title`}>{editing ? `Edit ${editing.name}` : 'Add a project'}</h2>
    </div>
    <button type="button" class="close" onclick={close} disabled={busy} aria-label="Close" data-shell-close>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>
    </button>
  </header>
  <form onsubmit={(event) => { event.preventDefault(); void save() }} oninput={edited}>
    <div class="body" data-shell-body>
      <fieldset disabled={busy || !!operation}>
        <div class="field">
          <label class="label machine-label" for={`${id}-machine`}>Machine</label>
          <div class="machine-row">
            <span class="dot" class:on={machine?.ready} aria-hidden="true"></span>
            <span class="machine-name">{machine?.host ?? (host ? 'A machine no longer on this account' : 'Choose a machine')}{#if machine}{' '}<span class="state">· {machine.ready ? 'online' : 'offline'}</span>{/if}</span>
            <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <select id={`${id}-machine`} bind:value={host} onchange={machineChanged} aria-describedby={block ? `${id}-machine-note` : undefined}>
              {#if host === ''}<option value="" disabled>Choose a machine</option>{/if}
              {#if host && !machine}<option value={host}>A machine no longer on this account</option>{/if}
              {#each machines as m (m.agentId)}<option value={m.agentId}>{m.host} · {m.ready ? 'online' : 'offline'}</option>{/each}
            </select>
          </div>
          {#if block && machine}<p class="machine-note" id={`${id}-machine-note`}>{block}</p>{/if}
          {#if machines.length === 0}<p class="hint">No machine is enrolled yet. <a href={machinesAt('add-machine')}>Add a machine</a> first.</p>{/if}
        </div>

        <div class="field">
          <label class="label" for={`${id}-folder`}>Folder</label>
          <div class="folder-row">
            <input id={`${id}-folder`} bind:this={folderInput} bind:value={dir} onchange={checkFolder} oninput={() => { checkSeq++; fieldNote = null; clearBranches() }}
              placeholder="/path/to/repo" autocomplete="off" autocapitalize="off" spellcheck="false"
              aria-invalid={fieldNote?.kind === 'bad' ? 'true' : undefined} aria-describedby={fieldNote ? `${id}-folder-note` : undefined} />
            <button type="button" class="toggle" aria-expanded={browsing} aria-controls={browsing ? `${id}-browser` : undefined} disabled={!!block}
              onclick={() => { browsing = !browsing }}>{browsing ? 'Hide browser' : 'Browse'}</button>
          </div>
          {#if fieldNote}
            <div class="note-row" id={`${id}-folder-note`}>
              <p class="field-note" class:repo={fieldNote.kind === 'repo'} class:bad={fieldNote.kind === 'bad'}>{fieldNote.text}</p>
              {#if fieldNote.top}<button type="button" class="ghost" onclick={() => fieldNote?.top && use({ path: fieldNote.top, home: '', repo: 'root', repoTop: '' })}>Use the top folder</button>{/if}
            </div>
          {/if}
        </div>

        {#if browsing && !block && machine}
          <div id={`${id}-browser`} class="browser-slot">
            {#key host}
              <FolderBrowser agentId={host} host={machine.host} {session} start={folderProblem === null ? dir : ''} chosen={dir} {serial} onuse={use} />
            {/key}
          </div>
        {/if}

        <div class="pair">
          <div class="field">
            <label class="label" for={`${id}-name`}>Name <span class="optional">optional</span></label>
            <input id={`${id}-name`} bind:value={name} placeholder="Defaults to the folder name" autocomplete="off" maxlength="200" />
          </div>
          <div class="field">
            <label class="label" for={`${id}-worktree`}>Worktree for each session</label>
            <div class="select-wrap">
              <select id={`${id}-worktree`} bind:value={worktreeDefault}>
                <option value="inherit">Use the account default</option>
                <option value="yes">Yes</option>
                <option value="no">No</option>
              </select>
              <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </div>
          </div>
        </div>

        <details class="fold" bind:this={promptFold}>
          <summary>Starter prompt <span class="optional">optional</span></summary>
          <div class="fold-body">
            <label class="label" for={`${id}-prompt`}>Starter prompt</label>
            <textarea id={`${id}-prompt`} bind:value={starterPrompt} rows="3" disabled={!promptKnown} aria-describedby={`${id}-prompt-hint`}></textarea>
            <p class="hint" id={`${id}-prompt-hint`}><span class:over={promptBytes > MAX_PROMPT_BYTES}>{promptBytes.toLocaleString('en-GB')} of {MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes.</span> The launch bar offers it as <strong>Use the starter prompt</strong>; reet never sends it by itself. Secrets don't belong here.</p>
            {#if !promptKnown}<p class="hint">The saved starter prompt wasn't read with this project, so it is kept exactly as it is.</p>{/if}
          </div>
        </details>

        <details class="fold" bind:this={moreFold} ontoggle={readBranches}>
          <summary>More defaults</summary>
          <div class="fold-body three">
            <div class="field">
              <label class="label" for={`${id}-branch`}>Default base branch</label>
              <input id={`${id}-branch`} bind:value={baseBranch} list={`${id}-branches`} placeholder="The repository's default" autocomplete="off" spellcheck="false" maxlength="255" />
              <datalist id={`${id}-branches`}>{#each branches as b (b)}<option value={b}></option>{/each}</datalist>
              {#if branchNote}<p class="hint branch-note">{branchNote}</p>{/if}
            </div>
            <div class="field">
              <label class="label" for={`${id}-tool`}>Default agent</label>
              <div class="select-wrap">
                <select id={`${id}-tool`} bind:value={defaultTool}>
                  <option value="">The launch bar's choice</option>
                  {#each TOOLS as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
                </select>
                <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
              </div>
            </div>
            <div class="field">
              <label class="label" for={`${id}-model`}>Default model</label>
              <input id={`${id}-model`} bind:value={defaultModel} placeholder="Agent default" autocomplete="off" spellcheck="false" maxlength="100" />
            </div>
          </div>
        </details>
      </fieldset>
    </div>
    <footer data-shell-foot>
      <p class="chosen">{#if dir && !folderProblem}<span class="path">{dir}</span>{#if chosenRepo === 'root'}<span class="kind"><span class="vh">{' · '}</span>git repository</span>{/if}{:else}<span class="none">No folder chosen yet</span>{/if}</p>
      <p class="status" class:bad={failed} class:quiet={!message} role="status" aria-live="polite" aria-atomic="true">{message || 'The machine decides whether it can open each folder.'}</p>
      <div class="actions">
        <button type="button" class="cancel" onclick={close} disabled={busy}>Cancel</button>
        <button type="submit" class="submit" bind:this={submitButton} disabled={busy || !ready} aria-busy={busy}>
          {#if busy}{editing ? 'Saving…' : 'Adding…'}{:else if operation?.lost}Try again{:else if editing}Save changes{:else}<span class="wide">Add project</span><span class="narrow">Add this project</span>{/if}
        </button>
      </div>
    </footer>
  </form>
</dialog>

<style>
  dialog { width: min(640px, calc(100vw - 28px)); max-height: calc(100dvh - 56px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 14px; box-shadow: 0 24px 60px var(--scrim); line-height: 1.3; }
  dialog.browsing { height: min(844px, calc(100dvh - 56px)); }
  dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr); }
  dialog::backdrop { background: var(--scrim); }
  header { display: flex; align-items: center; gap: 12px; padding: 20px 24px 12px; }
  .titles { flex-grow: 1; min-width: 0; }
  .eyebrow { margin: 0; font: 600 11px var(--sans); letter-spacing: .08em; text-transform: uppercase; color: var(--accent-text); }
  h2 { margin: 2px 0 0; font: 600 24px/1.25 var(--serif); overflow-wrap: anywhere; }
  .close { flex: none; width: 44px; height: 44px; display: grid; place-items: center; padding: 0; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--ink-soft); cursor: pointer; }
  .close svg { width: 18px; height: 18px; }
  .close:hover:not(:disabled) { border-color: var(--line-strong); color: var(--ink); }
  form { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
  .body { min-height: 0; overflow: auto; padding: 0 24px 16px; }
  .body:focus-visible { outline-offset: -2px; }
  fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: flex; flex-direction: column; gap: 14px; min-height: 100%; }
  dialog.browsing fieldset { height: 100%; }
  .field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .label { color: var(--muted); font-size: 13px; }
  .optional { color: var(--faint); }
  input:not([type=checkbox]), select, textarea { width: 100%; min-width: 0; height: 42px; min-height: 42px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--sans); }
  input::placeholder, textarea::placeholder { color: var(--faint); }
  textarea { height: auto; min-height: 84px; padding: 10px 12px; resize: vertical; line-height: 1.45; }
  .folder-row { display: flex; gap: 8px; }
  .folder-row input { flex-grow: 1; font: 13px var(--mono); }
  .folder-row input::placeholder { color: var(--muted); }
  .toggle { flex: none; height: 42px; padding: 0 16px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--ink); font-weight: 500; cursor: pointer; }
  .toggle:disabled { opacity: .5; cursor: default; }
  .toggle:hover:not(:disabled), .cancel:hover:not(:disabled), .ghost:hover { border-color: var(--line-strong); }
  /* The machine: the drawn row is what shows; the real select lies over it, so it is what is clicked, focused and read. */
  .machine-row { position: relative; display: flex; align-items: center; gap: 10px; height: 42px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .machine-row:focus-within { outline: 2px solid var(--accent); outline-offset: 0; }
  .machine-row:hover { border-color: var(--muted); }
  .machine-row select { position: absolute; inset: 0; height: 100%; opacity: 0; cursor: pointer; font-size: 16px; }
  .dot { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--muted); }
  .dot.on { background: var(--good); }
  .machine-name { flex-grow: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .state { color: var(--muted); }
  .chev { width: 14px; height: 14px; flex: none; color: var(--muted); }
  .select-wrap { position: relative; }
  .select-wrap select { appearance: none; padding-right: 34px; cursor: pointer; }
  .select-wrap .chev { position: absolute; right: 12px; top: 14px; pointer-events: none; }
  .machine-note, .hint, .field-note { margin: 0; font-size: 13px; line-height: 1.45; color: var(--muted); overflow-wrap: anywhere; }
  .hint a { color: var(--accent-text); }
  .note-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .field-note { flex: 1 1 14em; min-width: 0; }
  .field-note.repo { color: var(--repo); }
  .field-note.bad { color: var(--bad); }
  .ghost { flex: none; height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--ink); font-weight: 500; cursor: pointer; }
  .browser-slot { display: flex; flex-direction: column; min-height: 0; flex: none; }
  .pair { display: flex; gap: 12px; }
  .pair > .field { flex: 1; }
  .fold { border-top: 1px solid var(--line-soft); padding-top: 12px; }
  summary { cursor: pointer; color: var(--muted); font-size: 13px; width: fit-content; }
  summary:hover { color: var(--ink); }
  .fold-body { display: flex; flex-direction: column; gap: 6px; margin-top: 10px; }
  .fold-body.three { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
  .over { color: var(--bad); }
  footer { display: flex; align-items: center; gap: 10px; padding: 16px 24px; border-top: 1px solid var(--line-soft); }
  .chosen { display: none; }
  .status { flex-grow: 1; min-width: 0; margin: 0; font-size: 13px; color: var(--ink); overflow-wrap: anywhere; }
  .status.quiet { color: var(--muted); }
  .status.bad { color: var(--bad); }
  .actions { display: flex; gap: 10px; flex: none; }
  .cancel, .submit { height: 44px; border-radius: 8px; cursor: pointer; font: inherit; }
  .cancel { padding: 0 20px; border: 1px solid var(--line); background: transparent; color: var(--ink); font-weight: 500; }
  .submit { padding: 0 24px; border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); font-weight: 600; }
  .submit:hover:not(:disabled) { background: color-mix(in srgb, var(--accent) 90%, var(--ink)); }
  .submit:disabled, .cancel:disabled { opacity: .5; cursor: default; }
  .narrow { display: none; }
  @media (pointer: coarse) { .ghost { height: 44px; } }

  /* Phone: a full-screen sheet (the shell's `full`). The machine, folder, browser and fields scroll; the chosen folder and
     the buttons stay pinned at the foot. Every field is 16 px so focusing it never zooms the page. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    dialog.browsing { height: auto; }
    header, form { background: var(--bg); }
    header { height: 60px; flex: none; padding: 0 8px 0 16px; border-bottom: 1px solid var(--line-soft); }
    .eyebrow { font-size: 10px; letter-spacing: .09em; }
    h2 { margin: 0; font-size: 21px; }
    .close { border: 0; }
    .close svg { width: 20px; height: 20px; }
    .body { padding: 14px 16px 16px; font-size: 15px; }
    fieldset { gap: 12px; }
    dialog.browsing fieldset { height: auto; }
    .machine-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
    .machine-row { height: 48px; padding: 0 14px; border-radius: 10px; background: var(--card); }
    input:not([type=checkbox]), select, textarea { height: 48px; min-height: 48px; border-radius: 10px; background: var(--card); font-size: 16px; }
    textarea { height: auto; min-height: 96px; }
    .folder-row input { font-size: 16px; }
    .toggle { height: 48px; border-radius: 10px; }
    .select-wrap .chev { top: 17px; }
    .pair { flex-direction: column; }
    .fold-body.three { grid-template-columns: minmax(0, 1fr); }
    footer { flex-direction: column; align-items: stretch; gap: 10px; padding: 12px 16px 16px; background: var(--bg-sunk); }
    .chosen { display: flex; align-items: baseline; gap: 8px; margin: 0; font-size: 13px; min-width: 0; }
    .chosen .path { flex-grow: 1; min-width: 0; font-family: var(--mono); color: var(--ink); overflow-wrap: anywhere; }
    .chosen .kind { flex: none; color: var(--repo); }
    .chosen .none { color: var(--muted); }
    .status.quiet { display: none; }
    .actions { gap: 10px; }
    .cancel { width: 110px; height: 48px; padding: 0; border-color: var(--line-strong); border-radius: 10px; }
    .submit { flex: 1; height: 48px; border-radius: 10px; padding: 0 12px; }
    .wide { display: none; }
    .narrow { display: inline; }
  }
</style>
