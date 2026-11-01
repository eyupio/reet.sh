<script lang="ts">
  import { gafferTooltip } from '../gafferTooltip'
  import { onMount, tick } from 'svelte'
  import { shell } from '../dialogShell'

  // Switch a running OpenCode session between Gaffer and the OpenCode TUI: what happens on the machine, what is not
  // kept, and (to Gaffer, when the machine answers that the folder has OpenCode plugins) the folder-trust confirmation.
  let {
    to,
    agentTui,
    machine,
    trust,
    onconfirm,
    oncancel,
  }: {
    to: 'terminal' | 'chat'
    /** The agent's own TUI, as the header names it ("OpenCode TUI"). */
    agentTui: string
    machine: string
    /** Set when the machine refused for want of trust: its sentence naming the plugin files. */
    trust?: { detail: string }
    onconfirm: (trust: boolean) => void
    oncancel: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let keep = $state<HTMLButtonElement>(), trustBox = $state<HTMLInputElement>()
  let trusted = $state(false)
  const titleId = $props.id()
  const target = $derived(to === 'chat' ? 'Gaffer' : agentTui)
  const from = $derived(to === 'chat' ? agentTui : 'Gaffer')

  onMount(() => {
    dialog?.showModal()
    keep?.focus()
    return () => dialog?.close()
  })
  // The trust question arrives after the first answer: focus goes to it, so it is the next thing read.
  $effect(() => { if (trust) { trusted = false; void tick().then(() => trustBox?.focus()) } })
  function cancel() { dialog?.close(); oncancel() }
  function confirm() { if (trust && !trusted) return; dialog?.close(); onconfirm(!!trust && trusted) }
</script>

<dialog bind:this={dialog} role="alertdialog" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} aria-describedby={`${titleId}-body`}
  oncancel={(event) => { event.preventDefault(); cancel() }}>
  <div class="box" data-shell-card>
    <h2 id={titleId}>Switch to {to === 'chat' ? target : `the ${target}`}?</h2>
    <p id={`${titleId}-body`}>reet stops {to === 'chat' ? `the ${from}` : from} and starts {to === 'chat' ? target : `the ${target}`} on {machine}, on the same OpenCode conversation in the same folder.</p>
    <ul class="terms">
      <li>One process is stopped and another started: anything the agent was doing in {to === 'chat' ? `the ${from}` : from} is stopped with it.</li>
      {#if to === 'chat'}
        <li>Terminal scrollback is not kept. Gaffer rebuilds the conversation from OpenCode's history.</li>
        <li>A Gaffer session ends if reet on the machine restarts; the OpenCode TUI keeps running through that.</li>
      {:else}
        <li>Terminal scrollback is not kept: the OpenCode TUI starts on a fresh screen with the conversation loaded.</li>
        <li>You can switch back to Gaffer from this session's header.</li>
      {/if}
      <li>It can take a few minutes. The session's header says when it is done.</li>
    </ul>
    {#if trust}
      <div class="trust" role="group" aria-labelledby={`${titleId}-trust`}>
        <p class="warn" id={`${titleId}-trust`}><span class="mark" aria-hidden="true">!</span>This folder has OpenCode plugins. They run when Gaffer opens the session.</p>
        <p class="detail">{trust.detail}</p>
        <label><input type="checkbox" bind:this={trustBox} bind:checked={trusted} />I trust this folder. OpenCode plugins in it may run.</label>
      </div>
    {/if}
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" bind:this={keep} onclick={cancel}>Cancel</button>
      <button use:gafferTooltip={to === 'chat'} type="button" class="btn primary" disabled={!!trust && !trusted} onclick={confirm}>Switch to {to === 'chat' ? target : `the ${target}`}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { max-width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); backdrop-filter: blur(2px); }
  .box { padding: 28px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius); box-shadow: var(--shadow-dock); overflow-wrap: anywhere; }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; text-wrap: balance; }
  p { margin: 10px 0 0; color: var(--muted); }
  .terms { margin: 16px 0 0; padding: 12px 0 0 20px; border-top: 1px solid var(--line); display: grid; gap: 8px; color: var(--muted); font-size: 0.86rem; list-style: disc; }
  .trust { margin-top: 16px; padding: 12px 14px; border: 1px solid var(--warn); border-radius: 10px; background: color-mix(in srgb, var(--warn) 8%, transparent); }
  .trust .warn { margin: 0; display: flex; gap: 8px; align-items: flex-start; color: var(--ink); font-weight: 500; }
  .trust .mark { flex: none; width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid var(--warn); border-radius: 50%; color: var(--warn); font-size: 0.8rem; font-weight: 700; }
  .trust .detail { margin: 6px 0 0 28px; font-size: 0.86rem; }
  .trust label { display: flex; align-items: flex-start; gap: 9px; margin: 12px 0 0; font-size: 0.86rem; line-height: 1.4; color: var(--ink); cursor: pointer; }
  .trust input { flex: none; width: 16px; height: 16px; min-height: 0; margin: 2px 0 0; padding: 0; accent-color: var(--accent); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; flex-wrap: wrap; }
  @media (pointer: coarse) { .trust label { min-height: 44px; align-items: center; } .trust input { margin-top: 0; width: 20px; height: 20px; } }
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
