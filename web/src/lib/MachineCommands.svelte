<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import CopyButton from './CopyButton.svelte'
  import { enrollCommands, scanCommand, syncLink, upgradeCommands } from './machineSetup'

  let {
    machine,
    site,
    enrolled = false,
    agentStatus = '',
    onclose,
  }: {
    machine: string
    site: string
    enrolled?: boolean
    agentStatus?: string
    onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let tab = $state<'scan' | 'live' | 'upgrade'>('scan')
  const tabs = $derived([
    {
      id: 'scan' as const,
      label: 'Rescan',
      title: 'Refresh what this machine has installed',
      commands: scanCommand(site, machine),
      note: 'Run this on the machine. It reads the agent skill folders and updates the inventory here. It never installs or removes skills.',
    },
    {
      id: 'live' as const,
      label: 'Live agent',
      title: enrolled ? 'Restart this machine\'s live agent' : 'Enroll this machine for live status and remote jobs',
      commands: enrolled ? 'reet agent restart\nreet agent status' : enrollCommands(site, machine),
      note: enrolled
        ? 'Restart the agent in the background, then check its connection. Your terminal prompt stays free. Enrollment, credentials and running terminal sessions are preserved. Use reet agent stop to stop the connection.'
        : 'Enroll once, then start the background agent. You can close your terminal; it keeps reporting and processes changes you approve here. The CLI asks for a personal token the first time.',
    },
    {
      id: 'upgrade' as const,
      label: 'Upgrade',
      title: 'Update or diagnose the reet CLI',
      commands: upgradeCommands('stable'),
      note: 'Run these on the machine. Updating the CLI does not restart an unmanaged server.',
    },
  ])
  const active = $derived(tabs.find((item) => item.id === tab) ?? tabs[0])

  onMount(() => {
    dialog?.showModal()
    return () => {
      if (dialog?.open) dialog.close()
    }
  })
</script>

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby="machine-commands-title">
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Machine commands</p>
        <h2 id="machine-commands-title">{machine}</h2>
        <p class="lede" data-shell-aside>Copy a command, run it on that machine, and come back to refresh the status.</p>
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close machine commands" data-shell-close>×</button>
    </header>

    {#if enrolled}
      <p class="status"><span aria-hidden="true">●</span> {agentStatus || 'Live agent enrolled'}</p>
    {/if}

    <div class="layout">
      <div class="tabs" aria-label="Command type" role="tablist">
        {#each tabs as item}
          <button type="button" role="tab" aria-selected={tab === item.id} class:active={tab === item.id} onclick={() => (tab = item.id)}>{item.label}</button>
        {/each}
      </div>
      <div class="command-panel" role="tabpanel" aria-label={active.label}>
        <h3>{active.title}</h3>
        <p>{active.note}</p>
        <div class="command">
          <pre>{active.commands}</pre>
          <CopyButton text={active.commands} label={`Copy ${active.label.toLowerCase()} commands`} />
        </div>
        {#if active.id === 'live' && !enrolled}
          <p class="hint">Add <code>--allow-apply</code> only on a machine you trust. Removed skills are backed up beside the skill folder.</p>
        {/if}
      </div>
    </div>

    <footer>
      <a class="btn" href={syncLink(machine)}>Open compare &amp; sync →</a>
      <a class="btn quiet" href="/machines?setup=scratch#connect-machine">Set up from scratch →</a>
      <button class="btn quiet" type="button" onclick={() => dialog?.close()}>Done</button>
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(760px, calc(100vw - 32px)); max-height: min(88vh, 780px); padding: 0; border: 1px solid var(--line); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px rgb(0 0 0 / 0.28); }
  dialog::backdrop { background: rgb(20 18 16 / 0.45); backdrop-filter: blur(2px); }
  .sheet { display: grid; gap: 18px; padding: clamp(20px, 4vw, 30px); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; overflow-wrap: anywhere; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.86rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  .status { margin: 0; padding: 9px 12px; border: 1px solid color-mix(in srgb, var(--good) 45%, var(--line)); border-radius: 999px; color: var(--good); background: color-mix(in srgb, var(--good) 8%, var(--card)); font-size: 0.8rem; justify-self: start; }
  .layout { display: grid; grid-template-columns: 150px minmax(0, 1fr); gap: 18px; align-items: start; }
  .tabs { display: grid; gap: 4px; }
  .tabs button { text-align: left; min-height: 40px; padding: 8px 10px; border: 1px solid transparent; border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 0.84rem; cursor: pointer; }
  .tabs button:hover { color: var(--ink); background: var(--bg); }
  .tabs button.active { color: var(--ink); background: var(--tint); border-color: var(--line); font-weight: 650; }
  .command-panel { min-width: 0; }
  h3 { margin: 0; font-size: 1rem; }
  .command-panel p { margin: 6px 0 12px; color: var(--muted); font-size: 0.83rem; line-height: 1.55; }
  .command { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
  pre { max-width: 100%; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.78rem/1.65 var(--mono); }
  .hint { color: var(--muted); font-size: 0.78rem; }
  footer { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding-top: 14px; border-top: 1px solid var(--line); }
  footer .btn:last-child { margin-left: auto; }
  @media (max-width: 640px) {
    .layout { grid-template-columns: 1fr; }
    .tabs { display: flex; overflow-x: auto; gap: 4px; }
    .tabs button { white-space: nowrap; }
    footer .btn { flex: 1 1 auto; }
    footer .btn:last-child { margin-left: 0; }
  }
</style>
