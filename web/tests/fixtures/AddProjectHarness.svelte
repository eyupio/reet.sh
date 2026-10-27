<script lang="ts">
  import AddProjectDialog from '../../src/lib/home/AddProjectDialog.svelte'
  import { addedLine } from '../../src/lib/folderBrowser'
  import { getFolderProject, terminalMachines, type FolderProject, type TerminalMachine } from '../../src/lib/terminal/api'

  // Stands in for Home's rail, so the dialog can be tested alone: an Add button that opens it the way the rail does (machines
  // read on each open), an Edit button for ?edit=<id> (the project read with its starter prompt, as the rail's menu does),
  // and the page's status line for the success sentence, in the rail's words.
  const edit = new URLSearchParams(location.search).get('edit')
  let open = $state<{ opener: HTMLElement; machines: TerminalMachine[]; project?: FolderProject } | null>(null)
  let status = $state('')

  async function add(opener: HTMLElement) { status = ''; open = { opener, machines: await terminalMachines() } }
  async function change(opener: HTMLElement) {
    status = ''
    const [machines, project] = await Promise.all([terminalMachines(), getFolderProject(edit!)])
    open = { opener, machines, project }
  }
</script>

<main>
  <h1>Projects</h1>
  <button type="button" class="btn" aria-label="Add a project" onclick={(e) => add(e.currentTarget)}>Add</button>
  {#if edit}<button type="button" class="btn" onclick={(e) => change(e.currentTarget)}>Edit</button>{/if}
  <p role="status" aria-live="polite">{status}</p>
</main>

{#if open}
  <AddProjectDialog machines={open.machines} project={open.project} opener={open.opener}
    onadded={(p) => (status = open?.project ? `Sorted. ${p.name} saved.` : addedLine(p.name))} onclose={() => (open = null)} />
{/if}

<style>
  main { padding: 24px; display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
  h1 { margin: 0; font: 600 20px var(--serif); flex-basis: 100%; }
</style>
