<script lang="ts">
 import { onMount, tick, untrack } from 'svelte'
 import { shell } from './dialogShell'
 import { UnauthorizedError, type Machine } from './api'
 import { createTerminal, readiness, TerminalError, type LaunchRequest, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { MAX_PROMPT_BYTES, cleanModel, notStartedText, folderChoices, launchSentence, modelSuggestions, preselect, refusalField, refusalText, rekey, titleFor, toolChoices, toolName, validateDir, validateModel, validatePrompt } from './agentLaunch'

 // Start an agent CLI in a new terminal session on one machine. One screen, one press. The page names a tool id, a model, a
 // folder and a prompt; the machine decides the program and its flags. Nothing here is kept in browser storage or the URL, and
 // the prompt lives only in this component: it is cleared when the dialog closes.
 // opener is the element the person pressed to open this, which gets focus back on close.
 let { machines, snapshot, initialAgent = '', opener = null, onstarted, onaddmachine, onsignedout, onclose }: {
  /** The machines from /api/v2/terminal/machines, with their readiness and launch capabilities. */
  machines: TerminalMachine[]
  /** The machines' reported folders (from the sync state), or undefined while they are unknown: the home folder and a typed path still work. */
  snapshot: Machine[] | undefined
  /** The machine this was opened from, preselected when it is ready. */
  initialAgent?: string
  opener?: HTMLElement | null
  onstarted: (session: TerminalSession) => void
  /** Opens the add-a-machine flow. The dialog closes first. */
  onaddmachine?: () => void
  /** Hands a lost sign-in to the page, as the other dialogs do. Without it the dialog only says so. */
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 const OTHER = '\u0000other', TYPED = '\u0000typed'
 let dialog = $state<HTMLDialogElement>(), machineSelect = $state<HTMLSelectElement>()
 let startButton = $state<HTMLButtonElement>()
 // svelte-ignore state_referenced_locally
 const opening = preselect(machines, initialAgent)
 let agentId = $state(opening.agentId)
 // Said once, while the machine chosen for you is the one shown: why it is not the one you came from.
 const switchedNote = $derived(agentId === opening.agentId ? opening.note : '')
 let tool = $state(''), modelChoice = $state(''), modelOther = $state(''), folderChoice = $state(''), typedDir = $state(''), prompt = $state('')
 let attempted = $state(false), busy = $state(false), message = $state(''), failed = $state(false)
 // The status line holds an attempt's validation message only until the person edits a field: it never outlives the mistake it named.
 let invalidMessage = false
 // One attempt: the request id and the exact parameters. Kept while its outcome is unknown, so Retry asks about the same start
 // and a lost reply can never start a second agent. Dropped only when the server has definitely refused it.
 type Operation = { agentId: string; host: string; requestId: string; title: string; launch: LaunchRequest & { tool: string } }
 let operation = $state<Operation | null>(null)
 const life = new AbortController()

 const machine = $derived(machines.find(m => m.agentId === agentId))
 const choices = $derived(toolChoices(machine?.capabilities))
 // While a request is held or its outcome unknown, the tool is the one it asked for: a machine-list refresh can't change it.
 const chosenTool = $derived(operation ? operation.launch.tool : choices.find(c => c.id === tool && c.available)?.id ?? choices.find(c => c.available)?.id ?? '')
 // A new machine or agent starts the model and folder afresh: no blank select and no earlier choice coming back.
 let choiceKey = ''
 $effect.pre(() => {
  const key = agentId + '\u0000' + chosenTool
  untrack(() => {
   // Nothing that would change the held request is reset while it is held.
   if (operation) return
   const next = rekey({ key: choiceKey, model: modelChoice, folder: folderChoice }, key)
   if (next.key === choiceKey) return
   choiceKey = next.key; modelChoice = next.model; folderChoice = next.folder; modelOther = ''; typedDir = ''
  })
 })
 const suggestions = $derived(chosenTool ? modelSuggestions(chosenTool) : [])
 const model = $derived(modelChoice === OTHER ? cleanModel(modelOther) : suggestions.includes(modelChoice) ? modelChoice : '')
 const folders = $derived(folderChoices(machine, snapshot))
 const dir = $derived(folderChoice === TYPED ? typedDir : folders.some(f => f.path === folderChoice) ? folderChoice : '')
 const promptBytes = $derived(new TextEncoder().encode(prompt).length)
 // Why nothing can start on this machine, in the existing plain readiness words first.
 const blocked = $derived.by(() => {
  if (!machine) return machines.some(m => m.ready) ? 'Choose a machine.' : 'No machine is ready for terminals right now.'
  const why = readiness(machine)
  if (why) return why
  if (!machine.capabilities?.launch) return 'Update reet on this machine to start agents.'
  if (!chosenTool) return 'None of the agents reet can start is installed on this machine.'
  return ''
 })
 const modelError = $derived(modelChoice === OTHER ? (cleanModel(modelOther) === '' ? (attempted ? 'Type a model name, or choose Agent default.' : '') : validateModel(modelOther) ?? '') : '')
 const dirError = $derived(folderChoice === TYPED && (attempted || typedDir !== '') ? validateDir(typedDir) ?? '' : '')
 const promptError = $derived(chosenTool ? validatePrompt(chosenTool, prompt) ?? '' : '')
 const locked = $derived(busy || operation !== null)
 // While a request is held, the line says what was asked for, not what the current machine list would offer.
 const sentence = $derived(operation ? launchSentence(operation.launch.tool, operation.launch.dir ?? '', operation.host, !!operation.launch.prompt) : machine && chosenTool ? launchSentence(chosenTool, dir, machine.host, prompt !== '') : '')

 function shutdown(returnFocus: boolean) {
  life.abort()
  prompt = ''
  operation = null
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 // While a request is in flight the dialog stays: closing would forget a start that may still happen. The 15-second abort ends the wait.
 function close() { if (busy) return; shutdown(true); onclose() }
 // The hand-off is read before onclose: a parent may derive it from the state that onclose clears.
 function addMachine() { if (busy) return; const add = onaddmachine; shutdown(false); onclose(); add?.() }

 const LOST_TAIL = 'Check your sessions before starting another. If you close this and start again, a second agent may start.'
 const LOST = `We didn't hear back, so the agent may have started. ${LOST_TAIL}`
 // A refusal the server gave for certain: nothing started, so the person may change the fields and start again.
 const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 422].includes(e.status)
 const field = (name: string) => dialog?.querySelector<HTMLElement>(`#${CSS.escape(`${id}-${name}`)}`)
 // Focus after the fieldset and button are enabled again, so it never drops to the page.
 async function focusAfter(target: () => HTMLElement | null | undefined) { await tick(); if (!life.signal.aborted) (target() ?? startButton)?.focus() }
 function focusFor(where: string) {
  if (where === 'machine') return field('machine')
  if (where === 'tool') return dialog?.querySelector<HTMLElement>(`input[name="${CSS.escape(`${id}-tool`)}"]:checked`)
  if (where === 'folder') return folderChoice === TYPED ? field('dir') : field('folder')
  if (where === 'prompt') return field('prompt')
  return startButton
 }
 async function start() {
  if (busy || life.signal.aborted) return
  attempted = true
  if (!operation) {
   if (blocked || !machine) return
   // The errors are derived, so they already count this attempt; no await here, or a second press could slip past the busy guard.
   const invalid = modelError ? ['model-other', modelError] : dirError ? ['dir', dirError] : promptError ? ['prompt', promptError] : null
   if (invalid) { failed = true; message = invalid[1]; invalidMessage = true; field(invalid[0])?.focus(); return }
   const launch: LaunchRequest & { tool: string } = { tool: chosenTool }
   if (model) launch.model = model
   if (dir) launch.dir = dir
   if (prompt) launch.prompt = prompt
   operation = { agentId: machine.agentId, host: machine.host, requestId: crypto.randomUUID(), title: titleFor(chosenTool, dir), launch }
  }
  const op = operation
  busy = true; failed = false; invalidMessage = false
  message = `Starting ${toolName(op.launch.tool)} on ${op.host}… this can take a few seconds.`
  try {
   const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]), op.launch)
   if (life.signal.aborted) return
   // Answered, but the machine did not start it: nothing is running, so this request is done and the next Start is a new one.
   const notStarted = notStartedText(session.state)
   if (notStarted) {
    operation = null; failed = true; message = notStarted; busy = false
    void focusAfter(() => startButton)
    return
   }
   const started = onstarted
   shutdown(false)
   started(session)
   onclose()
  } catch (e) {
   if (life.signal.aborted) return
   failed = true
   if (definite(e)) {
    const err = e as TerminalError
    operation = null
    message = refusalText(err.status, err.code, err.message)
    busy = false
    if (err.status === 401 && onsignedout) { const signedOut = onsignedout; shutdown(false); onclose(); signedOut(new UnauthorizedError()); return }
    void focusAfter(() => focusFor(refusalField(err.status, err.code)))
   } else {
    message = e instanceof TerminalError && e.status === 503 ? `${refusalText(503)} ${LOST_TAIL}` : LOST
    busy = false
    void focusAfter(() => startButton)
   }
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 function edited() { if (invalidMessage) { invalidMessage = false; message = ''; failed = false } }

 onMount(() => {
  dialog?.showModal()
  machineSelect?.focus()
  return () => { life.abort(); prompt = '' }
 })
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header data-shell-head>
  <div>
   <p class="eyebrow" data-shell-aside>Terminals</p>
   <h2 id={`${id}-title`}>Start an agent</h2>
   <p class="lede" data-shell-aside>Right then: pick a machine, an agent and a folder. It starts in a new terminal session you can come back to.</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={busy} aria-label="Close" data-shell-close>Close ×</button>
 </header>
 <form class="sheet" oninput={edited} onsubmit={(event) => { event.preventDefault(); void start() }}>
  <!-- While everything is locked there is nothing to tab to in here, so the scrolling part takes focus itself and can be read. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="body" data-shell-body tabindex={locked ? 0 : undefined} role={locked ? 'region' : undefined} aria-label={locked ? 'Your choices' : undefined}>
  <fieldset disabled={locked}>
   <div class="field">
    <label for={`${id}-machine`}>Machine</label>
    <select id={`${id}-machine`} bind:this={machineSelect} bind:value={agentId} aria-describedby={[switchedNote ? `${id}-switched` : '', blocked ? `${id}-blocked` : ''].filter(Boolean).join(' ') || undefined}>
     {#if !machine}<option value="" disabled>Choose a machine</option>{/if}
     {#each machines as item (item.agentId)}<option value={item.agentId} disabled={!item.ready}>{item.host}{item.ready ? '' : ' · not ready'}</option>{/each}
    </select>
    {#if switchedNote}<p class="hint" id={`${id}-switched`}>{switchedNote}</p>{/if}
    {#if blocked}<p class="reason" id={`${id}-blocked`}>{blocked}</p>{/if}
    {#if onaddmachine}<button type="button" class="linklike" onclick={addMachine}>Not listed? Add a machine</button>{/if}
   </div>

   <fieldset class="tools">
    <legend>Agent</legend>
    {#each choices as choice (choice.id)}
     <div class="tool" class:off={!choice.available}>
      <input type="radio" id={`${id}-tool-${choice.id}`} name={`${id}-tool`} value={choice.id} checked={choice.id === chosenTool} disabled={!choice.available} onchange={() => (tool = choice.id)} aria-describedby={choice.available ? undefined : `${id}-why-${choice.id}`} />
      <span><label for={`${id}-tool-${choice.id}`}>{choice.name}</label>{#if !choice.available}<small id={`${id}-why-${choice.id}`} class:vh={!machine?.capabilities?.launch}>{choice.reason}</small>{/if}</span>
     </div>
    {/each}
   </fieldset>

   <div class="field">
    <label for={`${id}-model`}>Model <span class="optional">optional</span></label>
    <select id={`${id}-model`} bind:value={modelChoice} disabled={!chosenTool}>
     <option value="">Agent default</option>
     {#each suggestions as name (name)}<option value={name}>{name}</option>{/each}
     <option value={OTHER}>Other…</option>
    </select>
    {#if modelChoice === OTHER}
     <label for={`${id}-model-other`} class="sub">Model name</label>
     <input id={`${id}-model-other`} bind:value={modelOther} autocomplete="off" spellcheck="false" maxlength="100" aria-invalid={modelError ? 'true' : undefined} aria-describedby={modelError ? `${id}-model-error` : undefined} />
     {#if modelError}<p class="reason" id={`${id}-model-error`}>{modelError}</p>{/if}
    {/if}
    <p class="hint">Suggestions are hints. The agent says itself if it doesn't know a model.</p>
   </div>

   <div class="field">
    <label for={`${id}-folder`}>Folder</label>
    <select id={`${id}-folder`} bind:value={folderChoice}>
     {#each folders as folder (folder.path)}<option value={folder.path}>{folder.label}</option>{/each}
     <option value={TYPED}>Type a path…</option>
    </select>
    {#if folderChoice === TYPED}
     <label for={`${id}-dir`} class="sub">Folder path</label>
     <input id={`${id}-dir`} bind:value={typedDir} autocomplete="off" spellcheck="false" placeholder="/home/you/work/site" aria-invalid={dirError ? 'true' : undefined} aria-describedby={dirError ? `${id}-dir-error ${id}-dir-hint` : `${id}-dir-hint`} />
     {#if dirError}<p class="reason" id={`${id}-dir-error`}>{dirError}</p>{/if}
     <p class="hint" id={`${id}-dir-hint`}>The machine decides whether it can start there. It must be inside the home folder it was enrolled with.</p>
    {/if}
   </div>

   <div class="field">
    <label for={`${id}-prompt`}>Opening prompt <span class="optional">optional</span></label>
    <textarea id={`${id}-prompt`} bind:value={prompt} rows="3" disabled={!chosenTool} aria-invalid={promptError ? 'true' : undefined} aria-describedby={`${id}-prompt-count${promptError ? ` ${id}-prompt-error` : ''}`}></textarea>
    <p class="hint count" id={`${id}-prompt-count`} class:over={promptBytes > MAX_PROMPT_BYTES}>{promptBytes.toLocaleString('en-GB')} of {MAX_PROMPT_BYTES.toLocaleString('en-GB')} bytes. Secrets don't belong here.</p>
    {#if promptError}<p class="reason" id={`${id}-prompt-error`}>{promptError}</p>{/if}
   </div>
  </fieldset>

  {#if sentence}<p class="sentence">{sentence}</p>{/if}
  </div>
  <footer data-shell-foot>
   <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
   <small data-shell-aside>reet.sh never sends a command line. The machine starts the agent itself.</small>
   <button class="btn primary" type="submit" disabled={busy || (!operation && !!blocked)} aria-busy={busy} bind:this={startButton}>{busy ? 'Starting…' : operation ? 'Try the same request again' : 'Start'}</button>
  </footer>
 </form>
</dialog>

<style>
 dialog { width: min(640px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr); }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 /* The fields scroll; the footer with the status line and Start sits below them, outside the scrolling part, so it never covers a field. */
 .sheet { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
 .body { display: grid; gap: 16px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: grid; gap: 18px; }
 .field { display: grid; gap: 6px; min-width: 0; }
 label, legend { font-size: .85rem; font-weight: 600; }
 label.sub { font-weight: 500; margin-top: 4px; }
 .optional { color: var(--muted); font-weight: 400; font-size: .78rem; }
 select, input:not([type=radio]), textarea { min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 textarea { resize: vertical; min-height: 80px; }
 [aria-invalid='true'] { border-color: var(--bad); }
 .tools { gap: 6px; }
 .tools legend { margin-bottom: 6px; padding: 0; }
 .tool { display: flex; align-items: flex-start; gap: 10px; padding: 9px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
 .tool:has(input:checked) { border-color: var(--accent); }
 .tool input { flex: none; margin: 3px 0 0; accent-color: var(--accent); }
 .tool span { display: grid; gap: 2px; min-width: 0; }
 .tool label { font-size: .88rem; font-weight: 600; cursor: pointer; }
 .tool small { color: var(--muted); font-size: .78rem; }
 /* Only the name is dimmed; the reason beside it is text to read and keeps its full contrast. */
 .tool.off label { cursor: default; color: var(--muted); font-weight: 500; }
 .hint, small { margin: 0; color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .count.over { color: var(--bad); }
 .reason { margin: 0; color: var(--bad); font-size: .83rem; line-height: 1.5; }
 .sentence { margin: 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); font-size: .85rem; line-height: 1.5; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 .linklike { justify-self: start; padding: 4px 0; min-height: 32px; font: inherit; font-size: .83rem; color: var(--accent); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 /* Always in view below the scrolling fields, so Start is reachable on a short screen. */
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px 22px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } }
</style>
