<script lang="ts">
 import { onMount } from 'svelte'
 import type { Machine } from './api'
 import { createTerminal, readiness, TerminalError, type LaunchRequest, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { MAX_PROMPT_BYTES, cleanModel, folderChoices, launchSentence, modelSuggestions, refusalText, titleFor, toolChoices, toolName, validateDir, validateModel, validatePrompt } from './agentLaunch'

 // Start an agent CLI in a new terminal session on one machine. One screen, one press. The page names a tool id, a model, a
 // folder and a prompt; the machine decides the program and its flags. Nothing here is kept in browser storage or the URL, and
 // the prompt lives only in this component: it is cleared when the dialog closes.
 // opener is the element the person pressed to open this, which gets focus back on close.
 let { machines, snapshot, initialAgent = '', opener = null, onstarted, onaddmachine, onclose }: {
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
  onclose: () => void
 } = $props()

 const id = $props.id()
 const OTHER = '\u0000other', TYPED = '\u0000typed'
 let dialog = $state<HTMLDialogElement>(), machineSelect = $state<HTMLSelectElement>()
 // svelte-ignore state_referenced_locally
 let agentId = $state(machines.find(m => m.agentId === initialAgent && m.ready)?.agentId ?? machines.find(m => m.ready)?.agentId ?? '')
 let tool = $state(''), modelChoice = $state(''), modelOther = $state(''), folderChoice = $state(''), typedDir = $state(''), prompt = $state('')
 let attempted = $state(false), busy = $state(false), message = $state(''), failed = $state(false)
 // One attempt: the request id and the exact parameters. Kept while its outcome is unknown, so Retry asks about the same start
 // and a lost reply can never start a second agent. Dropped only when the server has definitely refused it.
 type Operation = { agentId: string; host: string; requestId: string; title: string; launch: LaunchRequest }
 let operation = $state<Operation | null>(null)
 const life = new AbortController()

 const machine = $derived(machines.find(m => m.agentId === agentId))
 const choices = $derived(toolChoices(machine?.capabilities))
 const chosenTool = $derived(choices.find(c => c.id === tool && c.available)?.id ?? choices.find(c => c.available)?.id ?? '')
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
 const sentence = $derived(machine && chosenTool ? launchSentence(chosenTool, dir, machine.host, prompt !== '') : '')

 function shutdown(returnFocus: boolean) {
  life.abort()
  prompt = ''
  operation = null
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 function close() { shutdown(true); onclose() }
 // The hand-off is read before onclose: a parent may derive it from the state that onclose clears.
 function addMachine() { const add = onaddmachine; shutdown(false); onclose(); add?.() }

 // A refusal the server gave for certain: nothing started, so the person may change the fields and start again.
 const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 422].includes(e.status)
 async function start() {
  if (busy || life.signal.aborted) return
  attempted = true
  if (!operation) {
   if (blocked || modelError || dirError || promptError || !machine) return
   if (modelChoice === OTHER && validateModel(modelOther) !== null) return
   const launch: LaunchRequest = { tool: chosenTool }
   if (model) launch.model = model
   if (dir) launch.dir = dir
   if (prompt) launch.prompt = prompt
   operation = { agentId: machine.agentId, host: machine.host, requestId: crypto.randomUUID(), title: titleFor(chosenTool, dir), launch }
  }
  const op = operation
  busy = true; failed = false
  message = `Starting ${toolName(op.launch.tool)} on ${op.host}…`
  try {
   const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]), op.launch)
   if (life.signal.aborted) return
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
   } else if (e instanceof TerminalError && e.status === 503) {
    message = `${refusalText(503)} Retry asks about the same start, so it never starts a second agent.`
   } else {
    message = 'The start is unconfirmed. Retry asks about the same start, so it never starts a second agent.'
   }
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 onMount(() => {
  dialog?.showModal()
  machineSelect?.focus()
  return () => { life.abort(); prompt = '' }
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header>
  <div>
   <p class="eyebrow">Terminals</p>
   <h2 id={`${id}-title`}>Start an agent</h2>
   <p class="lede">Right then: pick a machine, an agent and a folder. It starts in a new terminal session you can come back to.</p>
  </div>
  <button type="button" class="btn" onclick={close} aria-label="Close">Close ×</button>
 </header>
 <form class="body" onsubmit={(event) => { event.preventDefault(); void start() }}>
  <fieldset disabled={locked}>
   <div class="field">
    <label for={`${id}-machine`}>Machine</label>
    <select id={`${id}-machine`} bind:this={machineSelect} bind:value={agentId} aria-describedby={blocked ? `${id}-blocked` : undefined}>
     {#if !machine}<option value="" disabled>Choose a machine</option>{/if}
     {#each machines as item (item.agentId)}<option value={item.agentId} disabled={!item.ready}>{item.host}{item.ready ? '' : ' · not ready'}</option>{/each}
    </select>
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
  {#if operation && failed}<p class="hint">The fields stay as they were so Retry asks about the same start. If it did start, it appears in your sessions; close this to start afresh.</p>{/if}
  <footer>
   <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
   <small>reet.sh never sends a command line. The machine starts the agent itself.</small>
   <button class="btn primary" type="submit" disabled={busy || (!operation && !!blocked)} aria-busy={busy}>{busy ? 'Starting…' : operation ? 'Retry' : 'Start'}</button>
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
 .body { display: grid; gap: 16px; padding: 22px 22px 0; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
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
 .tool.off { opacity: .75; }
 .tool.off label { cursor: default; }
 .hint, small { margin: 0; color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .count.over { color: var(--bad); }
 .reason { margin: 0; color: var(--bad); font-size: .83rem; line-height: 1.5; }
 .sentence { margin: 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); font-size: .85rem; line-height: 1.5; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 .linklike { justify-self: start; padding: 4px 0; min-height: 32px; font: inherit; font-size: .83rem; color: var(--accent); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 /* Kept in view at the foot of the scrolling form, so Start is always reachable on a short screen. */
 footer { position: sticky; bottom: 0; display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 0 22px; border-top: 1px solid var(--line); background: var(--card); }
 .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px 16px 0; } footer { padding-bottom: 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } }
</style>
