<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { toolName } from '../agentLaunch'
  import { yoloCanStart, type YoloGate } from '../launchBar'

  let {
    gates,
    tool,
    structured = false,
    machine,
    oncancel,
    onstart,
  }: {
    gates: YoloGate[]
    tool: string
    structured?: boolean
    machine: string
    oncancel: () => void
    onstart: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let keep = $state<HTMLButtonElement>()
  const titleId = $props.id()
  const canStart = $derived(yoloCanStart(gates))
  const sentence = $derived(structured
    ? `The agent will not wait for you. reet says yes, this once, to every permission request it makes on ${machine}, including running commands and changing files.`
    : `The agent will not ask before running commands and changing files on ${machine}. reet passes ${toolName(tool)}'s own skip-the-prompts option and records the launch.`)

  onMount(() => {
    dialog?.showModal()
    keep?.focus()
    return () => dialog?.close()
  })
  function cancel() { dialog?.close(); oncancel() }
  function start() { if (!canStart) return; dialog?.close(); onstart() }
</script>

<dialog bind:this={dialog} role="alertdialog" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} aria-describedby={`${titleId}-body`}
  oncancel={(event) => { event.preventDefault(); cancel() }}>
  <div class="box" data-shell-card>
    <p class="warn" aria-hidden="true">!</p>
    <h2 id={titleId}>Start this session on YOLO?</h2>
    <p id={`${titleId}-body`}>{sentence}</p>
    <ul class="gates">
      {#each gates as gate (gate.label)}
        <li class:bad={!gate.ok && !gate.warning} class:warn={gate.warning}>
          <span class="mark" aria-hidden="true">{gate.warning ? '!' : gate.ok ? '✓' : '×'}</span>
          <span><b>{gate.label}</b><small>{gate.detail}</small></span>
        </li>
      {/each}
    </ul>
    <ul class="terms">
      <li>{structured ? 'Every request reet answers is written to the audit log, one line each.' : 'The launch is written to the audit log.'}</li>
      <li>The session wears a YOLO badge wherever it appears, for everyone who can see it.</li>
      <li>It can't be switched on part-way through. To stop it, end the session.</li>
      <li>Secrets in that folder are readable to the agent. Use a throwaway worktree if unsure.</li>
    </ul>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" bind:this={keep} onclick={cancel}>Keep asking me</button>
      <button type="button" class="btn primary" disabled={!canStart} onclick={start}>Start with YOLO</button>
    </div>
  </div>
</dialog>

<style>
  dialog { max-width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); backdrop-filter: blur(2px); }
  .box { padding: 28px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius); box-shadow: var(--shadow-dock); overflow-wrap: anywhere; }
  .warn { margin: 0 0 8px; width: 28px; height: 28px; display: grid; place-items: center; border: 1px solid var(--warn); border-radius: 50%; color: var(--warn); font-weight: 700; }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; text-wrap: balance; }
  p { margin: 10px 0 0; color: var(--muted); }
  .gates, .terms { margin: 16px 0 0; padding: 0; list-style: none; display: grid; gap: 8px; }
  .gates li { display: grid; grid-template-columns: 20px minmax(0, 1fr); gap: 8px; align-items: start; color: var(--ink); }
  .gates li.bad { color: var(--bad); }
  .gates li.warn { color: var(--warn); }
  .gates small { display: block; color: var(--muted); font-size: 0.78rem; }
  .mark { font-weight: 700; text-align: center; }
  .terms { padding-top: 12px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.86rem; list-style: disc; padding-left: 20px; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 14px; }
    .actions { flex-direction: column; }
    .actions button { width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
