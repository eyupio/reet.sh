<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { UnauthorizedError } from './api'
 import { deleteProject, TerminalError, type DeletedProject } from './terminal/api'
 import { projectRefusalText, type Project } from './projects'

 // Delete a Project. Its runs are kept, and nothing on any machine is touched: worktrees its runs made stay where they are.
 // The delete names the version that was read, so a Project someone changed meanwhile is not deleted unseen.
 let { project, opener = null, ondeleted, onreload, onsignedout, onclose }: {
  project: Project
  opener?: HTMLElement | null
  ondeleted: (result: DeletedProject) => void
  onreload?: () => void
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 let dialog = $state<HTMLDialogElement>(), cancelButton = $state<HTMLButtonElement>(), deleteButton = $state<HTMLButtonElement>()
 let busy = $state(false), message = $state(''), failed = $state(false), changed = $state(false), lost = $state(false)
 const life = new AbortController()

 function shutdown(returnFocus: boolean) {
  life.abort()
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 function close() { if (busy) return; shutdown(true); onclose() }
 function reload() { if (busy) return; const again = onreload; shutdown(false); onclose(); again?.() }

 async function remove() {
  if (busy || life.signal.aborted) return
  busy = true; failed = false; changed = false
  message = 'Deleting the Project…'
  try {
   const result = await deleteProject({ id: project.id, version: project.version }, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
   if (life.signal.aborted) return
   const done = ondeleted
   shutdown(false)
   done(result)
   onclose()
  } catch (e) {
   if (life.signal.aborted) return
   failed = true
   busy = false
   if (e instanceof TerminalError && e.status === 401 && onsignedout) { const hand = onsignedout; shutdown(false); onclose(); hand(new UnauthorizedError()); return }
   if (e instanceof TerminalError && e.status === 404 && lost) message = 'That Project is gone. Your earlier delete went through.'
   else if (e instanceof TerminalError && e.status !== 502 && e.status !== 503 && e.status !== 504) message = projectRefusalText(e.status, e.code, e.message)
   else { lost = true; message = "We didn't hear back, so the Project may have been deleted. Try again: if it was, it says so." }
   changed = e instanceof TerminalError && e.code === 'project_changed' && !!onreload
   await tick()
   if (!life.signal.aborted) (changed ? dialog?.querySelector<HTMLElement>(`#${CSS.escape(`${id}-reload`)}`) : deleteButton)?.focus()
  } finally {
   if (!life.signal.aborted) busy = false
  }
 }

 onMount(() => {
  dialog?.showModal()
  cancelButton?.focus()
  return () => life.abort()
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`} aria-describedby={`${id}-body`}>
 <h2 id={`${id}-title`}>Delete {project.name}?</h2>
 <div id={`${id}-body`}>
  <p>The Project and its saved defaults go. Its runs are kept, and nothing on any machine is touched.</p>
  {#if project.useWorktrees || project.lastRun}<p>Worktrees its runs made, if still there, stay on their machines. Remove them from a run first if you want them gone.</p>{/if}
 </div>
 <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
 <div class="buttons">
  <button type="button" class="btn" bind:this={cancelButton} onclick={close} disabled={busy}>Keep it</button>
  {#if changed}<button type="button" class="btn" id={`${id}-reload`} onclick={reload}>Reload the Project</button>{/if}
  <button type="button" class="btn primary" bind:this={deleteButton} onclick={remove} disabled={busy} aria-busy={busy}>{busy ? 'Deleting…' : 'Delete Project'}</button>
 </div>
</dialog>

<style>
 dialog { width: min(480px, calc(100vw - 28px)); padding: 22px; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); overflow-wrap: anywhere; }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 h2 { margin: 0 0 10px; font: 500 1.4rem/1.25 var(--serif); }
 p { margin: 0 0 10px; font-size: .9rem; line-height: 1.5; }
 .status:empty { display: none; }
 .status.bad { color: var(--bad); }
 .buttons { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: 8px; }
 @media (max-width: 500px) { .buttons .btn { width: 100%; } }
</style>
