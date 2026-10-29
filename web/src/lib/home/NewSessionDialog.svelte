<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { readiness, type FolderProject, type TerminalMachine } from '../terminal/api'

  // A machine's + or its menu's New session asks what to start there: one of the folder projects on that machine, which
  // starts that project's default agent, or a plain terminal session. The project choice is handed straight to Home's
  // launch bar (which starts it), so this dialog never starts anything itself.
  let { host, agentId, machine, machinesLoaded = true, projects, loaded = true, onproject, onterminal, onclose }: {
    host: string
    /** The machine's enrollment id: the folder projects on it carry it as their host. */
    agentId: string
    /** The machine's terminal entry, for its readiness; undefined while it has not been read. */
    machine?: TerminalMachine
    /** False while the terminal machines have not been read. */
    machinesLoaded?: boolean
    projects: FolderProject[]
    /** False while the projects have not been read. */
    loaded?: boolean
    onproject: (project: FolderProject) => void
    onterminal: () => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  const why = $derived(machine ? readiness(machine) : machinesLoaded ? 'This machine has not connected for terminals yet.' : 'Reading this machine…')
  const onMachine = $derived(projects.filter(project => project.host === agentId))

  onMount(() => {
    dialog?.showModal()
    ;(dialog?.querySelector<HTMLButtonElement>('button.choice:not(:disabled)') ?? dialog?.querySelector<HTMLButtonElement>('button.cancel'))?.focus()
    return () => dialog?.close()
  })
  function close() { dialog?.close(); onclose() }
  function choose(action: () => void) { dialog?.close(); action() }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`}
  oncancel={(event) => { event.preventDefault(); close() }}>
  <div class="sheet" data-shell-card>
    <header data-shell-head>
      <div>
        <p class="eyebrow" data-shell-aside>New session</p>
        <h2 id={`${id}-title`}>Start on {host}</h2>
        <p class="lede">Choose a project on this machine, or open a terminal session.</p>
      </div>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    </header>
    <div class="body" data-shell-body>
      {#if why}<p class="reason" role="status">{why}</p>{/if}
      {#if !loaded}
        <p class="none">Reading your projects…</p>
      {:else if onMachine.length === 0}
        <p class="none">No projects are on this machine yet. Open a terminal session, or add a project first.</p>
      {:else}
        <ul class="projects" aria-label={`Projects on ${host}`}>
          {#each onMachine as project (project.id)}
            <li>
              <button type="button" class="choice" disabled={!!why} onclick={() => choose(() => onproject(project))}>
                {#if project.isRepo}
                  <svg class="icon repo" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg>
                {:else}
                  <svg class="icon folder" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 6.5A1.5 1.5 0 014.5 5H9l2 2h8.5A1.5 1.5 0 0121 8.5v9a1.5 1.5 0 01-1.5 1.5h-15A1.5 1.5 0 013 17.5z" /></svg>
                {/if}
                <span class="copy"><b>{project.name}</b><small>{project.dir}</small></span>
                {#if project.baseBranch}<span class="branch">{project.baseBranch}</span>{/if}
                <span class="chev" aria-hidden="true">›</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <button type="button" class="choice terminal" disabled={!!why} onclick={() => choose(onterminal)}>
        <svg class="icon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M7 9l3 3-3 3M13 15h4" /></svg>
        <span class="copy"><b>Terminal session</b><small>A shell in this machine's home folder, with no agent.</small></span>
        <span class="chev" aria-hidden="true">›</span>
      </button>
    </div>
    <footer data-shell-foot>
      <button type="button" class="cancel" onclick={close}>Cancel</button>
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; overflow: auto; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; min-height: 0; padding: 8px 8px 12px; border: 1px solid var(--line); border-radius: 16px; background: var(--card-raised); }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 12px 8px 8px 16px; }
  header > div { min-width: 0; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 11px; font-weight: 650; letter-spacing: 0.1em; text-transform: uppercase; }
  h2 { margin: 3px 0 0; font: 600 1.4rem/1.25 var(--serif); overflow-wrap: anywhere; }
  .lede { margin: 6px 0 0; color: var(--muted); font-size: 13px; line-height: 1.5; }
  .close { flex: none; display: grid; place-items: center; width: 40px; height: 40px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--muted); cursor: pointer; }
  .close svg { width: 19px; height: 19px; }
  .body { display: flex; flex-direction: column; gap: 8px; padding: 4px; overflow: auto; }
  .projects { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .choice { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 56px; padding: 8px 12px; border: 0; border-radius: 10px; background: none; color: var(--ink); font: inherit; text-align: left; cursor: pointer; }
  .choice:hover:not(:disabled), .choice:focus-visible { background: var(--line-soft); }
  .choice:disabled { color: var(--faint); cursor: default; }
  .terminal { border-top: 1px solid var(--line-soft); border-radius: 0 0 10px 10px; margin-top: 2px; }
  .icon { flex: none; width: 16px; height: 16px; }
  .icon.repo { fill: var(--repo); }
  .icon.folder { color: var(--muted); }
  .copy { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
  .copy b { font-weight: 600; overflow-wrap: anywhere; }
  .copy small { color: var(--muted); font: 11px/1.4 var(--mono); overflow-wrap: anywhere; }
  .branch { flex: none; max-width: 30%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 11px/1.4 var(--mono); color: var(--muted); }
  .chev { flex: none; color: var(--muted); font-size: 20px; }
  .none, .reason { margin: 0; padding: 8px 12px; font-size: 13px; line-height: 1.5; color: var(--muted); }
  .reason { color: var(--warn); }
  footer { display: flex; justify-content: flex-end; padding: 8px 4px 0; }
  .cancel { min-height: 40px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 10px; background: transparent; color: var(--ink); font: inherit; cursor: pointer; }
  @media (max-width: 560px) { header { padding: 10px 4px 6px 12px; } .choice { min-height: 60px; } }
</style>
