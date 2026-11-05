<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import LaunchBar from '../launch/LaunchBar.svelte'
  import type { LaunchDefaults } from '../launchBar'
  import type { FolderProject, TerminalMachine, TerminalSession } from '../terminal/api'

  // Home's launch bar in a modal. The bar used to sit docked at the foot of the page; opening it here keeps Home's
  // other starts (the rail's Select and an address that names projects) on the same modal footing as the rest of the app.
  let { projects, projectsFailed = false, machines, sessions, profile, ready, initialProject = '', selection = [], onproject, onstarted, onleavefailed, onsignedout,
    onaddproject, onrun, onrundone, onclose }: {
    projects: FolderProject[] | undefined
    projectsFailed?: boolean
    machines: TerminalMachine[] | undefined
    sessions: TerminalSession[] | undefined
    profile: LaunchDefaults
    ready: boolean
    initialProject?: string
    selection?: string[]
    onproject?: (id: string) => void
    onstarted: (session: TerminalSession) => void | 'leaving'
    onleavefailed?: (session: TerminalSession) => void
    onsignedout?: (e: unknown) => void
    onaddproject?: (opener: HTMLElement) => void
    onrun?: () => void
    onrundone?: () => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  // A start under way cannot be cancelled by tearing the bar down: the server may already be starting it, and reopening
  // would send a new request. Close, Escape and a backdrop press wait, as the Start an agent dialog does.
  let working = $state(false)

  function close() {
    if (working) return
    dialog?.close()
    onclose()
  }

  onMount(() => {
    dialog?.showModal()
    return () => dialog?.close()
  })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`}
  oncancel={(event) => { event.preventDefault(); close() }} onclick={(event) => { if (event.target === dialog) close() }} aria-busy={working}>
  <div class="sheet" data-shell-card>
    <header data-shell-head>
      <div>
        <p class="eyebrow" data-shell-aside>New session</p>
        <h2 id={`${id}-title`}>Start a session</h2>
      </div>
      <button type="button" class="close" data-shell-close aria-label="Close" disabled={working} onclick={close}>×</button>
    </header>
    <div class="body" data-shell-body>
      <LaunchBar {projects} {projectsFailed} {machines} {sessions} {profile} {ready} {initialProject} {selection} {onproject} {onstarted} {onleavefailed} {onsignedout}
        {onaddproject} {onrun} {onrundone} onworkchange={(next) => (working = next)} />
    </div>
  </div>
</dialog>

<style>
  dialog { width: min(760px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 32px); border: 1px solid var(--line); border-radius: 16px; background: var(--card-raised); box-shadow: var(--shadow-dock, 0 18px 50px var(--scrim)); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 8px 12px 18px; border-bottom: 1px solid var(--line-soft); }
  .eyebrow { margin: 0; font-size: 14px; font-weight: 650; letter-spacing: .13em; text-transform: uppercase; color: var(--accent-text); }
  h2 { margin: 2px 0 0; font: 500 22px/1.2 var(--serif); letter-spacing: -.02em; }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--muted); font-size: 22px; line-height: 1; cursor: pointer; }
  .close:hover { background: var(--line-soft); color: var(--ink); }
  .body { min-height: 0; overflow-y: auto; padding: 0 14px 14px; }
  .body :global(.launch) { position: static; margin: 0; box-shadow: none; border-color: transparent; }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    dialog { width: 100vw; max-height: 100dvh; }
    .sheet { max-height: 100dvh; border-radius: 16px 16px 0 0; }
  }
</style>
