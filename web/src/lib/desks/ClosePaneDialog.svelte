<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'

  // Closing a pane (or a tab) whose shell is still running asks what should happen to the shell. End the shell is the
  // default and Enter chooses it; Take off this desk keeps the shell running in Sessions.
  let { name, host, count = 1, removes = false, onend, ontakeoff, oncancel }: {
    name: string
    host: string
    /** How many running shells the close covers (a tab asks once for all of them). */
    count?: number
    /** The person's setting: an ended session leaves Sessions (true) or stays there as ended. */
    removes?: boolean
    onend: () => void
    ontakeoff: () => void
    oncancel: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let confirmButton = $state<HTMLButtonElement>()
  let choice = $state<'end' | 'takeoff'>('end')
  const titleId = $props.id()
  const body = $derived(count > 1 ? `${count} of its shells are still running on ${host}.` : `Its shell is still running on ${host}.`)

  onMount(() => {
    dialog?.showModal()
    confirmButton?.focus()
    return () => dialog?.close()
  })
  function done(run: () => void) { dialog?.close(); run() }
  const confirm = () => done(choice === 'end' ? onend : ontakeoff)
  function keys(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.isComposing) return
    // Enter on Cancel cancels; anywhere else it confirms what is chosen (End the shell unless the other was picked).
    if ((event.target as HTMLElement).closest('[data-cancel]')) return
    event.preventDefault()
    confirm()
  }
  function arrows(event: KeyboardEvent) {
    if (!['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(event.key)) return
    event.preventDefault()
    choice = choice === 'end' ? 'takeoff' : 'end'
    dialog?.querySelector<HTMLElement>(`[data-choice="${choice}"]`)?.focus()
  }
</script>

<dialog bind:this={dialog} class="close-pane" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={event => { event.preventDefault(); done(oncancel) }} onkeydown={keys}>
  <div class="box" data-shell-card>
    <h2 id={titleId}>Close “{name}”?</h2>
    <p>{body}</p>
    <div class="options" role="radiogroup" aria-label="What happens to the shell" tabindex="-1" onkeydown={arrows}>
      <button type="button" class="opt" role="radio" data-choice="end" aria-checked={choice === 'end'} tabindex={choice === 'end' ? 0 : -1} onclick={() => (choice = 'end')}>
        <b>End the shell</b><span>{removes ? "Stops what is running in it. The session leaves Sessions, and this pane's command history is removed." : "Stops what is running in it. The session stays in Sessions as ended, and this pane's command history is removed."}</span>
      </button>
      <button type="button" class="opt" role="radio" data-choice="takeoff" aria-checked={choice === 'takeoff'} tabindex={choice === 'takeoff' ? 0 : -1} onclick={() => (choice = 'takeoff')}>
        <b>Take off this desk</b><span>It keeps running, and stays in Sessions.</span>
      </button>
    </div>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" data-cancel onclick={() => done(oncancel)}>Cancel</button>
      <button type="button" class="btn primary" bind:this={confirmButton} onclick={confirm}>{choice === 'end' ? 'End the shell' : 'Take off this desk'}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { max-width: min(440px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); overflow-wrap: anywhere; }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; text-wrap: balance; }
  p { margin: 8px 0 0; color: var(--muted); }
  .options { display: flex; flex-direction: column; gap: 8px; margin-top: 16px; }
  .opt { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); color: var(--ink); text-align: left; cursor: pointer; min-height: 44px; }
  .opt[aria-checked='true'] { border-color: var(--accent-text); background: var(--tint); }
  .opt b { font-weight: 600; }
  .opt span { color: var(--muted); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
</style>
