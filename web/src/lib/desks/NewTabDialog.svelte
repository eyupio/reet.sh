<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import FolderBrowser from '../home/FolderBrowser.svelte'
  import { validateFolderInput } from '../folderBrowser'
  import type { TerminalMachine } from '../terminal/api'

  // New tab: a plain shell on a machine, in a folder (typed, or found with the machine's folder browser). It starts on the
  // focused pane's machine and folder. Opening it is the press that starts the shell; nothing starts before. A pane's
  // Choose a folder and Choose a machine use the same fields under their own title (`fixed` keeps the pane's machine).
  let { machines, agentId: startAgent = '', dir: startDir = '', title = 'New tab', aside = 'A plain shell, in the folder you choose.', submit: submitLabel = 'Open tab', busyLabel = 'Opening…', fixed = false, browsing = false, oncreate, onclose }: {
    machines: TerminalMachine[]
    agentId?: string
    dir?: string
    title?: string
    aside?: string
    submit?: string
    busyLabel?: string
    /** The machine is the one given and cannot be changed. */
    fixed?: boolean
    /** The machine's folder browser is open from the start. */
    browsing?: boolean
    /** Starts the shell and adds the tab; a refusal is thrown and shown here. */
    oncreate: (agentId: string, dir: string) => Promise<void>
    onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id(), browseId = crypto.randomUUID()
  const usable = $derived(machines.filter(m => m.capabilities?.terminalEnabled !== false))
  // svelte-ignore state_referenced_locally
  let agentId = $state(usable.some(m => m.agentId === startAgent) ? startAgent : (usable.find(m => m.ready)?.agentId ?? usable[0]?.agentId ?? ''))
  // svelte-ignore state_referenced_locally
  let dir = $state(agentId === startAgent ? startDir : '')
  // svelte-ignore state_referenced_locally
  let browse = $state(browsing), busy = $state(false), error = $state('')
  const machine = $derived(usable.find(m => m.agentId === agentId))
  const canBrowse = $derived(!!machine?.ready && !!machine.capabilities?.browse && !machine.capabilities.browseOff)

  onMount(() => {
    dialog?.showModal()
    return () => dialog?.close()
  })
  function close() { if (busy) return; dialog?.close(); onclose() }
  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (busy || !machine) return
    const folder = dir.trim()
    const problem = folder ? validateFolderInput(folder) : null
    if (problem) { error = problem; return }
    busy = true; error = ''
    try {
      await oncreate(machine.agentId, folder)
      dialog?.close()
      onclose()
    } catch (e) {
      error = e instanceof Error && e.message ? e.message : 'That shell could not be started. Try again.'
    } finally { busy = false }
  }
</script>

<dialog bind:this={dialog} class="new-tab" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={event => { event.preventDefault(); close() }}>
  <form class="box" data-shell-card onsubmit={submit}>
    <h2 id={titleId}>{title}</h2>
    <p data-shell-aside>{aside}</p>
    {#if usable.length === 0}
      <p class="bad" role="alert">No machine can open a terminal yet. Add one on Machines.</p>
    {:else}
      <label class="field"><span>Machine</span>
        <select aria-label="Machine" disabled={fixed} bind:value={agentId} onchange={() => { dir = ''; browse = false }}>
          {#each usable as m (m.agentId)}<option value={m.agentId}>{m.host}{m.ready ? '' : ' · offline'}</option>{/each}
        </select>
      </label>
      <label class="field"><span>Folder <small>Leave empty for the machine’s home folder</small></span>
        <div class="folder"><input aria-label="Folder" bind:value={dir} placeholder="~/project" autocomplete="off" spellcheck="false" /><button type="button" class="btn" disabled={!canBrowse} title={canBrowse ? '' : 'This machine cannot list its folders just now.'} onclick={() => (browse = !browse)}>Browse</button></div>
      </label>
      {#if browse && machine}<FolderBrowser agentId={machine.agentId} host={machine.host} session={browseId} start={dir} chosen={dir} serial={job => job()} onuse={picked => { dir = picked.path; browse = false }} />{/if}
    {/if}
    {#if error}<p class="bad" role="alert">{error}</p>{/if}
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" onclick={close}>Cancel</button>
      <button type="submit" class="btn primary" disabled={!machine || busy} aria-busy={busy}>{busy ? busyLabel : submitLabel}</button>
    </div>
  </form>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); max-width: none; max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { display: flex; flex-direction: column; gap: 12px; padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; }
  p { margin: 0; color: var(--muted); }
  .field { display: flex; flex-direction: column; gap: 6px; font-weight: 600; }
  .field small { font-weight: 400; color: var(--muted); font-size: 14px; }
  .folder { display: flex; gap: 8px; }
  .folder input { flex: 1; font-family: var(--mono); }
  .bad { color: var(--bad); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
</style>
