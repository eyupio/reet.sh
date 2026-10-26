<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import type { Machine, ReleaseInfo, SyncAgent } from './api'
  import { agentBuild, channelBuild } from './machines'

  // A machine's policies live here rather than beside the buttons that act on
  // it, so a card says what is happening and this dialog says how it is set.
  let {
    machine,
    agent,
    releases,
    busy,
    onautoupdate,
    onpause,
    onscan,
    onswitch,
    onrevoke,
    onforget,
    onclose,
  }: {
    machine: Machine
    agent?: SyncAgent
    releases?: ReleaseInfo
    busy: boolean
    onautoupdate: (enabled: boolean) => void
    onpause: () => void
    onscan: () => void
    onswitch: () => void
    onrevoke: () => void
    onforget: () => void
    onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const id = $props.id()
  const onDev = $derived(agent?.version === 'dev')
  const pointer = $derived(channelBuild(releases, onDev ? 'dev' : 'latest'))

  onMount(() => {
    dialog?.showModal()
    return () => {
      if (dialog?.open) dialog.close()
    }
  })
</script>

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby={`${id}-title`}>
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Machine settings</p>
        <h2 id={`${id}-title`}>{machine.host}</h2>
        <p class="lede" data-shell-aside>How this machine keeps itself current, and what the controller may ask of it. A change applies straight away.</p>
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close settings" data-shell-close>×</button>
    </header>

    {#if agent}
      <section aria-labelledby={`${id}-updates`}>
        <h3 id={`${id}-updates`}>Updates</h3>
        <div class="row">
          <div class="copy">
            <strong id={`${id}-auto`}>Automatic updates</strong>
            <small>Install new reet builds on this machine without a review. The agent verifies each build and keeps the previous binary; a build that cannot report back is put back.</small>
          </div>
          <button type="button" role="switch" class="switch" aria-checked={agent.autoUpdate} aria-labelledby={`${id}-auto`} disabled={busy} onclick={() => onautoupdate(!agent.autoUpdate)}>
            <span class="knob" aria-hidden="true"></span>
          </button>
        </div>
        <div class="row">
          <div class="copy">
            <strong>Release channel</strong>
            <small>
              {onDev ? 'On the development channel, which follows main.' : 'On the release channel, which follows tagged builds.'}
              Runs <code>{agentBuild(agent)}</code>{#if pointer}; the channel points at <code>{pointer}</code>{/if}.
            </small>
          </div>
          <button class="btn" type="button" disabled={busy || agent.paused} onclick={onswitch}>Switch channel…</button>
        </div>
      </section>

      <section aria-labelledby={`${id}-sync`}>
        <h3 id={`${id}-sync`}>Sync</h3>
        <div class="row">
          <div class="copy">
            <strong id={`${id}-pause`}>Pause sync jobs</strong>
            <small>While paused, no new sync jobs, scans or updates run on this machine. A job already running may finish.</small>
          </div>
          <button type="button" role="switch" class="switch" aria-checked={agent.paused} aria-labelledby={`${id}-pause`} disabled={busy} onclick={onpause}>
            <span class="knob" aria-hidden="true"></span>
          </button>
        </div>
        <div class="row">
          <div class="copy">
            <strong>Inventory</strong>
            <small>Ask the agent to report its installed skills again. Nothing is installed or removed.</small>
          </div>
          <button class="btn" type="button" disabled={busy || agent.paused} onclick={onscan}>Scan now</button>
        </div>
      </section>
    {/if}

    <section aria-labelledby={`${id}-access`}>
      <h3 id={`${id}-access`}>Access</h3>
      {#if agent}
        <div class="row">
          <div class="copy">
            <strong>Live access</strong>
            <small>Close private access and stop new terminal authorisation and sync jobs. Installed files and running shells stay.</small>
          </div>
          <button class="btn" type="button" disabled={busy} onclick={onrevoke}>Revoke live access…</button>
        </div>
      {/if}
      <div class="row">
        <div class="copy">
          <strong>Forget this machine</strong>
          <small>Remove its inventory reports and saved sync references here. Enrollment stays, and a future scan can report it again.</small>
        </div>
        <button class="btn danger" type="button" disabled={busy} onclick={onforget}>Forget machine…</button>
      </div>
    </section>
  </div>
</dialog>

<style>
  dialog { width: min(640px, calc(100vw - 32px)); max-height: min(88vh, 760px); padding: 0; border: 1px solid var(--line); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px rgb(0 0 0 / 0.28); }
  dialog::backdrop { background: rgb(20 18 16 / 0.45); backdrop-filter: blur(2px); }
  .sheet { display: grid; gap: 18px; padding: clamp(20px, 4vw, 30px); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; overflow-wrap: anywhere; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.86rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  section { display: grid; gap: 0; padding-top: 14px; border-top: 1px solid var(--line); }
  h3 { margin: 0 0 4px; color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.1em; text-transform: uppercase; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 10px 0; }
  .row + .row { border-top: 1px solid color-mix(in srgb, var(--line) 60%, transparent); }
  .copy { display: grid; gap: 2px; min-width: 0; }
  .copy strong { font-size: 0.86rem; }
  .copy small { color: var(--muted); font-size: 0.76rem; line-height: 1.5; }
  .copy code { font-size: 0.74rem; }
  .row .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  .btn.danger { color: var(--bad); }
  .switch { position: relative; flex: none; width: 42px; height: 24px; padding: 0; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); cursor: pointer; }
  .switch[aria-checked='true'] { background: var(--accent); border-color: var(--accent); }
  .switch:disabled { opacity: 0.55; cursor: default; }
  .switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .knob { position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--card); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .switch[aria-checked='true'] .knob { transform: translateX(18px); }
  @media (max-width: 560px) {
    .row { flex-wrap: wrap; }
    .row .btn { flex: 1 1 auto; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .switch { transition: background-color 160ms ease, border-color 160ms ease; }
    .knob { transition: transform 160ms var(--ease-out); }
  }
</style>
