<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { getGafferOutcome, listModelFavourites, recoverGafferPipeline, type GafferOutcome, type GafferOutcomeSession } from '../api'
  import { CATALOGUE } from '../catalogue'
  import GafferModelSelect from '../GafferModelSelect.svelte'
  import { stageInSentence } from '../gafferOutcome'
  import type { MachineModels, ModelFavourite } from '../models'
  import { chatToolName, machineModels, structuredAgent, terminalMachines, type TerminalMachine } from '../terminal/api'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'

  let { record, piece, oncancel, onstarted }: {
    record: GafferOutcome
    piece: GafferOutcomeSession
    oncancel: () => void
    onstarted: () => void | Promise<void>
  } = $props()

  const stage = $derived(piece.stages?.[piece.stage ?? 0])
  const stoppedTool = $derived(stage?.tool ?? record.tool)
  const stoppedModel = $derived(stage?.appliedModel || stage?.model || '')
  let tool = $state(untrack(() => stoppedTool)), model = $state('')
  let machine = $state<TerminalMachine | undefined>(), favourites = $state<ModelFavourite[]>([])
  let offered = $state<Record<string, MachineModels | null>>({})
  let busy = $state(false), error = $state('')
  const life = new AbortController()
  const tools = $derived(CATALOGUE.filter(agent =>
    agent.id === stoppedTool || structuredAgent(machine?.capabilities, agent.id) && (!record.yolo || agent.yolo) && (record.trust || agent.id !== 'antigravity')))
  const ready = $derived(!!model.trim() && !(tool === stoppedTool && model.trim() === stoppedModel))

  onMount(() => {
    void terminalMachines(life.signal).then(found => { machine = found.find(host => host.agentId === record.agentId) }).catch(() => {})
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => {})
    return () => life.abort()
  })
  $effect(() => {
    const agent = tool
    if (agent in offered) return
    offered[agent] = null
    void machineModels(record.agentId, agent, life.signal).then(found => { offered[agent] = found }).catch(() => {})
  })

  async function recover(event: SubmitEvent) {
    event.preventDefault()
    if (busy || !ready || !piece.sessionId) return
    busy = true
    error = ''
    try {
      const current = await getGafferOutcome(record.id, life.signal)
      await recoverGafferPipeline(current, piece.key, piece.sessionId, { tool, model: model.trim() }, life.signal)
      await onstarted()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Gaffer could not start the replacement worker.'
    } finally {
      busy = false
    }
  }
</script>

<WorkspaceDialog title="Continue with another model" onclose={() => { if (!busy) oncancel() }}>
  <form onsubmit={recover}>
    <p>Gaffer will resume the {stageInSentence(stage ?? { kind: '' })} stage in the same folder and branch.</p>
    {#if tools.length > 1}
      <label>Agent<select bind:value={tool} disabled={busy} onchange={() => { model = ''; error = '' }}>
        {#each tools as agent (agent.id)}<option value={agent.id}>{chatToolName(agent.id)}</option>{/each}
      </select></label>
    {/if}
    <label>Model<GafferModelSelect label="Replacement model" {tool} bind:value={model} machine={offered[tool] ?? null}
      {favourites} host={machine?.host ?? ''} disabled={busy} onchange={() => error = ''}/></label>
    {#if tool === stoppedTool && model && !ready}<p role="status">Choose a different model.</p>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button class="btn primary" type="submit" disabled={busy || !ready}>{busy ? 'Starting…' : 'Continue stage'}</button>
      <button class="btn quiet" type="button" disabled={busy} onclick={oncancel}>Cancel</button>
    </div>
  </form>
</WorkspaceDialog>

<style>
  form, label { display: grid; gap: 10px; min-width: 0 }
  label { gap: 6px; font-weight: 600 }
  p { margin: 0 }
  .actions { display: flex; flex-wrap: wrap; gap: 8px }
  select { min-height: 44px; width: 100%; font: inherit; font-weight: 400; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; padding: 8px 12px }
  .error { color: var(--bad) }
</style>
