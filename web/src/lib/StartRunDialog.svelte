<script lang="ts">
 import { onMount, tick, untrack } from 'svelte'
 import { shell } from './dialogShell'
 import { UnauthorizedError } from './api'
 import { startRun, TerminalError, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { STARTED_MEANS, confirmSentence, planRun, projectRefusalText, type Project, type ProjectRun } from './projects'

 // The confirmation sheet for Start all: which repositories will start, which cannot and why, and one press to start the
 // chosen ones. One run per press: the request id is made once and resent unchanged after a lost reply, so the server
 // answers with the same run instead of starting a second one.
 let { project, machines, sessions, opener = null, onstarted, onchanged, onaddmachine, onsignedout, onclose }: {
  project: Project
  machines: TerminalMachine[]
  /** The account's sessions now: the 8-per-machine and 32-per-account limits count them. */
  sessions: TerminalSession[]
  opener?: HTMLElement | null
  onstarted: (run: ProjectRun) => void
  /** The Project changed since this sheet was made, so nothing started. The dialog closes first; the page reads it again. */
  onchanged?: () => void
  /** Opens the add-a-machine flow. The dialog closes first. */
  onaddmachine?: () => void
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 let dialog = $state<HTMLDialogElement>(), startButton = $state<HTMLButtonElement>()
 // The plan is made once, when the sheet opens: what it lists is what was confirmed, even if a machine changes meanwhile.
 // The server and the machine check everything again.
 const plan = untrack(() => planRun(project, machines, undefined, sessions))
 const hostOf = (agentId: string) => untrack(() => machines.find(m => m.agentId === agentId)?.host) ?? 'A machine no longer on this account'
 let chosen = $state<number[]>(plan.start.map(s => s.position))
 let busy = $state(false), message = $state(''), failed = $state(false)
 // The version the sheet was made from goes with the request: a Project changed since never starts from this sheet.
 type Operation = { requestId: string; positions: number[]; version: number }
 let operation = $state<Operation | null>(null)
 const life = new AbortController()
 const locked = $derived(busy || operation !== null)
 const count = $derived(operation ? operation.positions.length : chosen.length)
 const sentence = $derived(confirmSentence(count, project.hasSharedPrompt, project.useWorktrees))
 // svelte-ignore state_referenced_locally
 const rows = project.members.map(member => {
  const start = plan.start.find(s => s.position === member.position)
  const skip = plan.skipped.find(s => s.position === member.position)
  return { member, start, skip }
 })

 function toggle(position: number, on: boolean) {
  if (locked) return
  chosen = on ? [...new Set([...chosen, position])].sort((a, b) => a - b) : chosen.filter(p => p !== position)
  if (!failed) message = ''
 }

 function shutdown(returnFocus: boolean) {
  life.abort()
  operation = null
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 function close() { if (busy) return; shutdown(true); onclose() }
 function addMachine() { if (busy) return; const add = onaddmachine; shutdown(false); onclose(); add?.() }

 const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 413, 422].includes(e.status)
 async function focusAfter(target: () => HTMLElement | null | undefined) { await tick(); if (!life.signal.aborted) (target() ?? startButton)?.focus() }

 async function start() {
  if (busy || life.signal.aborted) return
  if (!operation) {
   if (chosen.length === 0) { failed = true; message = 'Choose at least one repository to start.'; return }
   operation = { requestId: crypto.randomUUID(), positions: [...chosen], version: project.version }
  }
  const op = operation
  busy = true; failed = false
  message = `Starting ${op.positions.length === 1 ? 'one repository' : `${op.positions.length} repositories`}…`
  try {
   const run = await startRun({ id: project.id, version: op.version }, op.positions, op.requestId, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
   if (life.signal.aborted) return
   const started = onstarted
   shutdown(false)
   started(run)
   onclose()
  } catch (e) {
   if (life.signal.aborted) return
   failed = true
   if (definite(e)) {
    const err = e as TerminalError
    operation = null
    busy = false
    if (err.status === 401 && onsignedout) { const signedOut = onsignedout; shutdown(false); onclose(); signedOut(new UnauthorizedError()); return }
    if (err.code === 'project_changed' && onchanged) { const changed = onchanged; shutdown(false); onclose(); changed(); return }
    message = projectRefusalText(err.status, err.code, err.message)
   } else {
    // The same request id again can only ever give this one run.
    message = "We didn't hear back, so the run may have started. Try the same request again: it can't start a second run."
    busy = false
   }
   void focusAfter(() => startButton)
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 onMount(() => {
  dialog?.showModal()
  ;(dialog?.querySelector<HTMLElement>('input[type=checkbox]:not(:disabled)') ?? startButton)?.focus()
  return () => life.abort()
 })
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`} aria-describedby={`${id}-sentence`}>
 <header data-shell-head>
  <div>
   <p class="eyebrow" data-shell-aside>{project.name}</p>
   <h2 id={`${id}-title`}>Start all</h2>
   <p class="lede" data-shell-aside>Here's what'll start. Untick any you'd rather leave for now.</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={busy} aria-label="Close" data-shell-close>Close ×</button>
 </header>
 <form class="sheet" onsubmit={(event) => { event.preventDefault(); void start() }}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="body" data-shell-body tabindex={locked ? 0 : undefined} role={locked ? 'region' : undefined} aria-label={locked ? 'What will start' : undefined}>
   <fieldset disabled={locked}>
    <legend class="vh">Repositories to start</legend>
    <ul>
     {#each rows as row (row.member.position)}
      {@const box = `${id}-m-${row.member.position}`}
      <li class:off={!row.start}>
       <input type="checkbox" id={box} checked={!!row.start && (operation ? operation.positions : chosen).includes(row.member.position)} disabled={!row.start} onchange={(e) => toggle(row.member.position, e.currentTarget.checked)} aria-describedby={`${box}-what`} />
       <span>
        <label for={box}><span class="host">{hostOf(row.member.host)}</span> <span class="dir">{row.member.dir}</span></label>
        {#if row.start}
         <small id={`${box}-what`}>{row.start.toolName} · {row.start.model || 'Agent default'} · {row.start.worktree ? 'new worktree and branch' : 'in the saved folder'}</small>
        {:else if row.skip}
         <small id={`${box}-what`} class="why">Won't start: {row.skip.reason}</small>
         {#if row.skip.code === 'not_enrolled' && onaddmachine}<button type="button" class="linklike" onclick={addMachine}>Add a machine</button>{/if}
        {/if}
       </span>
      </li>
     {/each}
    </ul>
   </fieldset>
   {#if project.members.length === 0}<p class="hint">This Project has no repositories yet. Change the Project to add some.</p>{/if}
   {#if !project.useWorktrees && count > 0}<p class="warn">This Project makes no worktrees. Each agent works in the saved folder itself, so any other session there shares that checkout and its files.</p>{/if}
   <p class="sentence" id={`${id}-sentence`}>{sentence}</p>
   <p class="hint">{STARTED_MEANS}</p>
  </div>
  <footer data-shell-foot>
   <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
   <small data-shell-aside>reet.sh never sends a command line. Each machine starts its agent itself.</small>
   <button class="btn primary" type="submit" disabled={busy || (!operation && chosen.length === 0)} aria-busy={busy} bind:this={startButton}>{busy ? 'Starting…' : operation ? 'Try the same request again' : 'Start'}</button>
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
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; overflow-wrap: anywhere; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 .sheet { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
 .body { display: grid; gap: 14px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 /* The dialog clips anything outside it and the footer covers the bottom, so the ring is drawn inside the region. */
 .body:focus-visible { outline-offset: -2px; }
 fieldset { margin: 0; padding: 0; border: 0; min-width: 0; }
 ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
 li { display: flex; gap: 10px; align-items: flex-start; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); min-width: 0; }
 li:has(input:checked) { border-color: var(--accent); }
 li input { flex: none; margin: 3px 0 0; width: 18px; height: 18px; accent-color: var(--accent); }
 li > span { display: grid; gap: 3px; min-width: 0; }
 label { font-size: .88rem; cursor: pointer; }
 .host { font-weight: 600; }
 .dir { font-family: var(--mono); font-size: .8rem; }
 li.off label { cursor: default; }
 li.off .host { color: var(--muted); }
 small, .hint { margin: 0; color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .why { color: var(--ink); }
 .warn { margin: 0; padding: 10px 12px; border: 1px solid var(--warn); border-radius: 8px; font-size: .85rem; line-height: 1.5; }
 .sentence { margin: 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); font-size: .85rem; line-height: 1.5; }
 .linklike { justify-self: start; padding: 4px 0; min-height: 32px; font: inherit; font-size: .83rem; color: var(--accent); background: none; border: 0; text-decoration: underline; cursor: pointer; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px 22px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } }
</style>
