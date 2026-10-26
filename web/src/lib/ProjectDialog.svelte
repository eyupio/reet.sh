<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { UnauthorizedError, type Machine } from './api'
 import { saveProject, TerminalError, type TerminalMachine } from './terminal/api'
 import { MAX_PROMPT_BYTES, TOOLS, cleanModel, folderChoices, modelSuggestions, toolName, validateDir, validateModel, validatePrompt } from './agentLaunch'
 import { MAX_MEMBERS, projectInput, projectRefusalText, validateMember, validateMembers, validateProjectName, type MemberInput, type Project, type ProjectInput } from './projects'

 // Create a Project, or change one. One screen; one save per press. A save names the version it was read at, so a second
 // editor's change is never undone silently. Nothing here is kept in browser storage or the URL, and the shared prompt lives
 // only in this component while it is open.
 let { project = null, machines, snapshot, otherNames = [], opener = null, onsaved, onreload, onsignedout, onclose }: {
  /** The Project to change, read with its shared prompt; null makes a new one. */
  project?: Project | null
  machines: TerminalMachine[]
  /** The machines' reported folders, or undefined while unknown: a typed path still works. */
  snapshot: Machine[] | undefined
  /** The account's other Project names, compared ignoring case. The server checks again. */
  otherNames?: string[]
  opener?: HTMLElement | null
  /** The save reply leaves the shared prompt out; `sent` says what was saved, so the page can keep it. */
  onsaved: (project: Project, sent: ProjectInput) => void
  /** Reads the Project again after someone else changed it. The dialog closes first. */
  onreload?: () => void
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 const TYPED = '\u0000typed'
 type Row = { key: number; host: string; folder: string; typedDir: string; tool: string; model: string }
 // svelte-ignore state_referenced_locally
 const editing = project
 let dialog = $state<HTMLDialogElement>(), nameInput = $state<HTMLInputElement>(), saveButton = $state<HTMLButtonElement>()
 let name = $state(editing?.name ?? '')
 let defaultTool = $state(editing?.defaultTool || 'claude')
 let defaultModel = $state(editing?.defaultModel ?? '')
 let sharedPrompt = $state(editing?.sharedPrompt ?? '')
 // A Project read without its prompt (only the single read carries it) keeps its saved prompt untouched: the box is locked.
 const promptKnown = !editing || editing.sharedPrompt !== undefined
 let useWorktrees = $state(editing ? editing.useWorktrees : true)
 let nextKey = 0
 // svelte-ignore state_referenced_locally
 let rows = $state<Row[]>((editing?.members ?? []).map(m => ({ key: nextKey++, host: m.host, folder: m.dir, typedDir: '', tool: m.tool, model: m.model })))
 // Repositories saved on a machine that has since left the account stay acceptable as they are; the server agrees.
 const saved = new Set((editing?.members ?? []).map(m => `${m.host}\u0000${m.dir}`))
 let attempted = $state(false), busy = $state(false), message = $state(''), failed = $state(false), changedElsewhere = $state(false)
 let invalidMessage = false
 // One save: exactly what is sent. Kept while its outcome is unknown, so the retry sends the same thing.
 type Operation = { input: ProjectInput; current?: { id: string; version: number }; lost: boolean }
 let operation = $state<Operation | null>(null)
 const life = new AbortController()
 const locked = $derived(busy || operation !== null)

 const dirOf = (r: Row): string => (r.folder === TYPED ? r.typedDir : r.folder)
 const machineFor = (host: string) => machines.find(m => m.agentId === host)
 function folderOptions(r: Row): { path: string; label: string }[] {
  const reported = folderChoices(machineFor(r.host), snapshot).filter(f => f.path !== '')
  // A saved folder the machine has not reported (or not yet) is still offered, as itself.
  if (r.folder && r.folder !== TYPED && !reported.some(f => f.path === r.folder)) reported.unshift({ path: r.folder, label: r.folder })
  return reported
 }
 const memberInput = (r: Row): MemberInput => ({ host: r.host, dir: dirOf(r), tool: r.tool, model: cleanModel(r.model) })
 function memberError(r: Row, n: number): string {
  const m = memberInput(r)
  const known = saved.has(`${m.host}\u0000${m.dir}`) && !machineFor(m.host) ? [...machines, { agentId: m.host, host: m.host, ready: false }] : machines
  const problem = validateMember(m, known)
  return problem ? `Repository ${n}: ${problem}` : ''
 }
 const nameError = $derived(attempted || name !== '' ? validateProjectName(name, otherNames) ?? '' : '')
 const modelError = $derived(validateModel(defaultModel) ?? '')
 const promptBytes = $derived(new TextEncoder().encode(sharedPrompt).length)
 // The prompt goes to every repository, so it must suit every agent the Project starts.
 const promptError = $derived.by(() => {
  if (sharedPrompt === '') return ''
  const tools = new Set([defaultTool, ...rows.map(r => r.tool).filter(Boolean)])
  for (const tool of tools) { const problem = validatePrompt(tool, sharedPrompt); if (problem) return problem }
  return ''
 })
 const rowErrors = $derived(rows.map((r, i) => (attempted || dirOf(r) !== '' ? memberError(r, i + 1) : '')))
 const listError = $derived(validateMembers(rows.map(memberInput)) ?? '')

 function addRow() {
  if (locked || rows.length >= MAX_MEMBERS) return
  const first = machines.find(m => m.ready) ?? machines[0]
  rows.push({ key: nextKey++, host: first?.agentId ?? '', folder: '', typedDir: '', tool: '', model: '' })
  edited()
  void tick().then(() => field(`host-${rows.length - 1}`)?.focus())
 }
 function removeRow(index: number) {
  if (locked) return
  rows.splice(index, 1)
  edited()
  void tick().then(() => (field(`host-${Math.min(index, rows.length - 1)}`) ?? addButton)?.focus())
 }
 let addButton = $state<HTMLButtonElement>()

 function shutdown(returnFocus: boolean) {
  life.abort()
  sharedPrompt = ''
  operation = null
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 // While a save is in flight the dialog stays: the 15-second limit ends the wait.
 function close() { if (busy) return; shutdown(true); onclose() }
 function reload() { if (busy) return; const again = onreload; shutdown(false); onclose(); again?.() }

 const field = (name: string) => dialog?.querySelector<HTMLElement>(`#${CSS.escape(`${id}-${name}`)}`)
 async function focusAfter(target: () => HTMLElement | null | undefined) { await tick(); if (!life.signal.aborted) (target() ?? saveButton)?.focus() }
 const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 413, 422].includes(e.status)
 // Which field a server refusal names ("members.2.dir"), so focus goes there.
 function focusForField(fieldName: string): HTMLElement | null | undefined {
  const m = /^members\.(\d+)\.(host|dir|tool|model)$/.exec(fieldName)
  if (m) { const r = rows[Number(m[1])]; return field(m[2] === 'dir' ? (r?.folder === TYPED ? `dir-${m[1]}` : `folder-${m[1]}`) : `${m[2]}-${m[1]}`) }
  return field(({ name: 'name', defaultTool: 'tool', defaultModel: 'model', sharedPrompt: 'prompt' } as Record<string, string>)[fieldName] ?? '')
 }

 async function save() {
  if (busy || life.signal.aborted) return
  attempted = true
  if (!operation) {
   // Derived errors already count this attempt. No await before busy is set, so a second press cannot slip past the guard.
   const nameProblem = validateProjectName(name, otherNames)
   const rowIndex = rows.findIndex((r, i) => memberError(r, i + 1) !== '')
   const invalid: [string, string] | null = nameProblem ? ['name', nameProblem]
    : modelError ? ['model', modelError]
    : promptError ? ['prompt', promptError]
    : rowIndex >= 0 ? [rows[rowIndex].host === '' ? `host-${rowIndex}` : rows[rowIndex].folder === TYPED ? `dir-${rowIndex}` : `folder-${rowIndex}`, memberError(rows[rowIndex], rowIndex + 1)]
    : listError ? ['add', listError] : null
   if (invalid) { failed = true; message = invalid[1]; invalidMessage = true; (field(invalid[0]) ?? addButton)?.focus(); return }
   const input = projectInput(editing, { name, defaultTool, defaultModel: cleanModel(defaultModel), sharedPrompt, useWorktrees, members: rows.map(memberInput) })
   operation = { input, current: editing ? { id: editing.id, version: editing.version } : undefined, lost: false }
  }
  const op = operation
  busy = true; failed = false; invalidMessage = false; changedElsewhere = false
  message = editing ? 'Saving your changes…' : 'Saving the Project…'
  try {
   const result = await saveProject(op.input, op.current, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
   if (life.signal.aborted) return
   const done = onsaved
   shutdown(true)
   done(result, op.input)
   onclose()
  } catch (e) {
   if (life.signal.aborted) return
   failed = true
   if (definite(e)) {
    const err = e as TerminalError
    const wasLost = op.lost
    operation = null
    busy = false
    if (err.status === 401 && onsignedout) { const signedOut = onsignedout; shutdown(false); onclose(); signedOut(new UnauthorizedError()); return }
    // A resent save whose first try went through answers as if someone else had changed it, or as if the name were taken.
    if (wasLost && err.code === 'project_changed') message = 'This Project changed since it was read. Your earlier save may have gone through. Reload it to see.'
    else if (wasLost && !editing && err.code === 'name_taken') message = 'A Project with that name is already saved. Your earlier save may have gone through. Close this and check your Projects.'
    else if (wasLost && !editing && err.code === 'project_limit') message = 'This account now has 50 Projects, the most it can have. Your earlier save may have gone through. Close this and check your Projects.'
    else message = projectRefusalText(err.status, err.code, err.message)
    changedElsewhere = err.code === 'project_changed' && !!onreload
    const fieldName = err.field ?? ''
    void focusAfter(() => (changedElsewhere ? field('reload') : err.code === 'name_taken' ? field('name') : focusForField(fieldName)))
   } else {
    op.lost = true
    message = editing
     ? "We didn't hear back, so your changes may have been saved. Try the same request again: if they were, it says the Project changed."
     : "We didn't hear back, so the Project may have been saved. Try the same request again, or close this and check your Projects."
    busy = false
    void focusAfter(() => saveButton)
   }
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 function edited() { if (invalidMessage) { invalidMessage = false; message = ''; failed = false } }

 onMount(() => {
  dialog?.showModal()
  nameInput?.focus()
  return () => { life.abort(); sharedPrompt = '' }
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header>
  <div>
   <p class="eyebrow">Projects</p>
   <h2 id={`${id}-title`}>{editing ? `Change ${editing.name}` : 'New project'}</h2>
   <p class="lede">{editing ? 'Change the defaults or the repositories. Runs already started keep what they used.' : "Name it, choose the repositories, and set what each agent starts with. You can start them all from the Project's page."}</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={busy} aria-label="Close">Close ×</button>
 </header>
 <form class="sheet" oninput={edited} onchange={edited} onsubmit={(event) => { event.preventDefault(); void save() }}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="body" tabindex={locked ? 0 : undefined} role={locked ? 'region' : undefined} aria-label={locked ? 'Your choices' : undefined}>
  <fieldset disabled={locked}>
   <div class="field">
    <label for={`${id}-name`}>Name</label>
    <input id={`${id}-name`} bind:this={nameInput} bind:value={name} autocomplete="off" maxlength="200" aria-invalid={nameError ? 'true' : undefined} aria-describedby={nameError ? `${id}-name-error` : undefined} />
    {#if nameError}<p class="reason" id={`${id}-name-error`}>{nameError}</p>{/if}
   </div>

   <div class="pair">
    <div class="field">
     <label for={`${id}-tool`}>Default agent</label>
     <select id={`${id}-tool`} bind:value={defaultTool}>
      {#each TOOLS as tool (tool.id)}<option value={tool.id}>{tool.name}</option>{/each}
     </select>
    </div>
    <div class="field">
     <label for={`${id}-model`}>Default model <span class="optional">optional</span></label>
     <input id={`${id}-model`} bind:value={defaultModel} list={`${id}-models-${defaultTool}`} placeholder="Agent default" autocomplete="off" spellcheck="false" maxlength="100" aria-invalid={modelError ? 'true' : undefined} aria-describedby={modelError ? `${id}-model-error` : `${id}-model-hint`} />
     {#if modelError}<p class="reason" id={`${id}-model-error`}>{modelError}</p>{:else}<p class="hint" id={`${id}-model-hint`}>Suggestions are hints. The agent says itself if it doesn't know a model.</p>{/if}
    </div>
   </div>
   {#each TOOLS as tool (tool.id)}<datalist id={`${id}-models-${tool.id}`}>{#each modelSuggestions(tool.id) as m (m)}<option value={m}></option>{/each}</datalist>{/each}

   <div class="field">
    <label for={`${id}-prompt`}>Shared prompt <span class="optional">optional</span></label>
    <textarea id={`${id}-prompt`} bind:value={sharedPrompt} rows="3" disabled={!promptKnown} aria-invalid={promptError ? 'true' : undefined} aria-describedby={`${id}-prompt-count ${id}-prompt-where${promptError ? ` ${id}-prompt-error` : ''}`}></textarea>
    <p class="hint count" id={`${id}-prompt-count`} class:over={promptBytes > MAX_PROMPT_BYTES}>{promptBytes.toLocaleString('en-GB')} of {MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes. Secrets don't belong here.</p>
    <p class="hint" id={`${id}-prompt-where`}>Sent as each repository's opening prompt. It is kept with the Project and with each session record, and is visible to other users of each machine in its process list.</p>
    {#if !promptKnown}<p class="hint">The saved prompt wasn't read with this Project, so it is kept exactly as it is.</p>{/if}
    {#if promptError}<p class="reason" id={`${id}-prompt-error`}>{promptError}</p>{/if}
   </div>

   <div class="check">
    <input type="checkbox" id={`${id}-worktrees`} bind:checked={useWorktrees} aria-describedby={`${id}-worktrees-hint`} />
    <span>
     <label for={`${id}-worktrees`}>Give each run its own git worktree</label>
     <small id={`${id}-worktrees-hint`}>{useWorktrees ? 'Each repository gets a new folder under ~/.reet/worktrees and a new branch, so agents never share a checkout. reet never removes a worktree by itself.' : 'Agents start in the saved folders themselves, so any other session there shares that checkout.'}</small>
    </span>
   </div>

   <fieldset class="members">
    <legend>Repositories <span class="optional">{rows.length} of {MAX_MEMBERS}</span></legend>
    {#if rows.length === 0}<p class="hint">No repositories yet. Add one for each folder an agent should work in.</p>{/if}
    <ol>
     {#each rows as row, i (row.key)}
      {@const options = folderOptions(row)}
      {@const error = rowErrors[i]}
      <li class="member" aria-label={`Repository ${i + 1}`}>
       <div class="member-head"><strong>Repository {i + 1}</strong><button type="button" class="btn quiet small" onclick={() => removeRow(i)} aria-label={`Remove repository ${i + 1} from this Project`}>Remove</button></div>
       <div class="pair">
        <div class="field">
         <label for={`${id}-host-${i}`}>Machine</label>
         <select id={`${id}-host-${i}`} bind:value={row.host} onchange={() => { if (row.folder !== TYPED) row.folder = '' }}>
          {#if row.host === ''}<option value="" disabled>Choose a machine</option>{/if}
          {#if row.host && !machineFor(row.host)}<option value={row.host}>A machine no longer on this account</option>{/if}
          {#each machines as m (m.agentId)}<option value={m.agentId}>{m.host}</option>{/each}
         </select>
        </div>
        <div class="field">
         <label for={`${id}-folder-${i}`}>Folder</label>
         <select id={`${id}-folder-${i}`} bind:value={row.folder}>
          {#if row.folder === ''}<option value="" disabled>Choose a folder</option>{/if}
          {#each options as f (f.path)}<option value={f.path}>{f.label}</option>{/each}
          <option value={TYPED}>Type a path…</option>
         </select>
        </div>
       </div>
       {#if row.folder === TYPED}
        <div class="field">
         <label for={`${id}-dir-${i}`}>Folder path</label>
         <input id={`${id}-dir-${i}`} bind:value={row.typedDir} autocomplete="off" spellcheck="false" placeholder="/home/you/work/site" aria-invalid={row.typedDir !== '' && validateDir(row.typedDir) ? 'true' : undefined} aria-describedby={`${id}-dir-hint-${i}`} />
         <p class="hint" id={`${id}-dir-hint-${i}`}>The machine decides whether it can start there. It must be inside the home folder it was enrolled with{useWorktrees ? ', and be the top of a git repository' : ''}.</p>
        </div>
       {/if}
       <div class="pair">
        <div class="field">
         <label for={`${id}-tool-${i}`}>Agent</label>
         <select id={`${id}-tool-${i}`} bind:value={row.tool}>
          <option value="">Project default ({toolName(defaultTool)})</option>
          {#each TOOLS as tool (tool.id)}<option value={tool.id}>{tool.name}</option>{/each}
         </select>
        </div>
        <div class="field">
         <label for={`${id}-model-${i}`}>Model <span class="optional">optional</span></label>
         <input id={`${id}-model-${i}`} bind:value={row.model} list={`${id}-models-${row.tool || defaultTool}`} placeholder={row.tool && row.tool !== defaultTool ? 'Agent default' : defaultModel ? `Project default (${defaultModel})` : 'Agent default'} autocomplete="off" spellcheck="false" maxlength="100" aria-invalid={validateModel(row.model) ? 'true' : undefined} />
        </div>
       </div>
       {#if error}<p class="reason">{error}</p>{/if}
      </li>
     {/each}
    </ol>
    {#if listError && attempted}<p class="reason">{listError}</p>{/if}
    <button type="button" class="btn" id={`${id}-add`} bind:this={addButton} onclick={addRow} disabled={rows.length >= MAX_MEMBERS}>Add a repository</button>
    {#if machines.length === 0}<p class="hint">No machine is enrolled yet. <a href="/machines#add-machine">Add a machine</a> first.</p>{/if}
   </fieldset>
  </fieldset>
  </div>
  <footer>
   <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
   {#if changedElsewhere}<button type="button" class="btn" id={`${id}-reload`} onclick={reload} disabled={busy}>Reload the Project</button>{/if}
   <button class="btn primary" type="submit" disabled={busy} aria-busy={busy} bind:this={saveButton}>{busy ? 'Saving…' : operation ? 'Try the same request again' : editing ? 'Save changes' : 'Save Project'}</button>
  </footer>
 </form>
</dialog>

<style>
 dialog { width: min(720px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr); }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 .sheet { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
 .body { display: grid; gap: 16px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: grid; gap: 18px; }
 .field { display: grid; gap: 6px; min-width: 0; align-content: start; }
 .pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
 label, legend { font-size: .85rem; font-weight: 600; }
 .optional { color: var(--muted); font-weight: 400; font-size: .78rem; }
 select, input:not([type=checkbox]), textarea { min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 textarea { resize: vertical; min-height: 80px; }
 [aria-invalid='true'] { border-color: var(--bad); }
 .check { display: flex; gap: 10px; align-items: flex-start; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
 .check input { flex: none; margin: 3px 0 0; width: 18px; height: 18px; accent-color: var(--accent); }
 .check span { display: grid; gap: 2px; min-width: 0; }
 .members { gap: 10px; }
 .members legend { padding: 0; margin-bottom: 4px; }
 ol { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
 .member { display: grid; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
 .member select, .member input { background: var(--card); }
 .member-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: .85rem; }
 .btn.small { min-height: 36px; padding: 6px 10px; font-size: .82rem; }
 .members > .btn { justify-self: start; }
 .hint, small { margin: 0; color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .hint a { color: var(--accent); }
 .count.over { color: var(--bad); }
 .reason { margin: 0; color: var(--bad); font-size: .83rem; line-height: 1.5; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 10px 12px; padding: 14px 22px 22px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 @media (max-width: 560px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } .pair { grid-template-columns: minmax(0, 1fr); } footer .btn { width: 100%; } }
</style>
