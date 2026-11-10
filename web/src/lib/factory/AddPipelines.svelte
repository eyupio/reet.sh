<script lang="ts">
  // Approving pipelines Gaffer proposed. Each one can be given its own agent, model and effort; left alone it uses the
  // outcome's. Nothing is added or started until the person approves, and an added pipeline never merges by itself.
  import { onMount } from 'svelte'
  import { listModelFavourites, type GafferOutcome } from '../api'
  import { chatToolName, machineModels, structuredAgent, terminalMachines, type TerminalMachine } from '../terminal/api'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import GafferModelSelect from '../GafferModelSelect.svelte'
  import { DEFAULT_STAGES, STAGES } from '../gafferOutcome'
  import { effortChoices, type MachineModels, type ModelFavourite } from '../models'

  type Proposal = { title: string; prompt: string; why?: string; stages?: string[] }
  export type AddedPipeline = { title: string; prompt: string; stages: { kind: string; tool?: string; model?: string; effort?: string }[] }
  let { record, proposals, busy = false, oncancel, onconfirm }: {
    record: GafferOutcome; proposals: Proposal[]; busy?: boolean; oncancel: () => void; onconfirm: (pipelines: AddedPipeline[]) => void
  } = $props()
  const CHAT_TOOLS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const life = new AbortController()
  let machine = $state<TerminalMachine | null>(null), favourites = $state<ModelFavourite[]>([]), offered = $state<Record<string, MachineModels | null>>({})
  // What each added pipeline runs on. It starts as the outcome's own agent and model.
  let crew = $state<Record<string, { tool: string; model: string; effort: string }>>({})
  const tools = $derived.by(() => {
    const found = machine ? CHAT_TOOLS.filter(id => structuredAgent(machine!.capabilities, id)) : []
    return found.includes(record.tool) ? found : [record.tool, ...found]
  })
  const stageLabel = (kind: string) => STAGES.find(item => item.kind === kind)?.label ?? kind
  const kinds = (item: Proposal) => item.stages?.length ? item.stages : DEFAULT_STAGES
  const choice = (title: string) => crew[title] ?? { tool: record.tool, model: record.model ?? '', effort: '' }
  const needsTrust = $derived(!record.trust && proposals.some(item => ['opencode', 'antigravity'].includes(choice(item.title).tool)))

  $effect(() => { for (const item of proposals) if (!crew[item.title]) crew[item.title] = { tool: record.tool, model: record.model ?? '', effort: '' } })
  $effect(() => {
    for (const id of new Set(proposals.map(item => choice(item.title).tool))) {
      if (!id || id in offered) continue
      offered[id] = null
      void machineModels(record.agentId, id, life.signal).then(found => { offered[id] = found }).catch(() => { /* suggestions and Other still work */ })
    }
  })
  onMount(() => {
    void terminalMachines(life.signal).then(found => { machine = found.find(item => item.agentId === record.agentId) ?? null }).catch(() => { /* the outcome's own agent is still offered */ })
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => { /* a helper only */ })
    return () => life.abort()
  })
  function approve() {
    onconfirm(proposals.map(item => {
      const who = choice(item.title), model = who.model.trim()
      // Left as the outcome's own agent and model, the stages name no one and the controller fills them in.
      const own = who.tool === record.tool && model === (record.model ?? '') && !who.effort
      return { title: item.title, prompt: item.prompt, stages: kinds(item).map(kind => own ? { kind } : { kind, tool: who.tool, model, ...(who.effort ? { effort: who.effort } : {}) }) }
    }))
  }
</script>

<WorkspaceDialog title={proposals.length === 1 ? `Add “${proposals[0].title}”?` : `Add ${proposals.length} pipelines?`} onclose={oncancel}>
  <div class="add">
    <p>The controller starts each added pipeline as a chat in this outcome’s project, {record.yolo ? 'on YOLO like the rest of this outcome' : 'with ordinary approvals, never in YOLO'}, and Gaffer follows it stage by stage. None of them merges by itself: each stops at <strong>Ready to merge</strong> and waits for you.</p>
    <ul class="rows" aria-label="Pipelines to add">
      {#each proposals as item (item.title)}{@const who = choice(item.title)}<li role="group" aria-label={item.title}>
        <div class="what"><strong>{item.title}</strong><span>{kinds(item).map(stageLabel).join(' → ')}</span></div>
        <div class="who">
          <label>Agent<select aria-label={`Agent for ${item.title}`} value={who.tool} disabled={busy} onchange={event => { crew[item.title] = { tool: event.currentTarget.value, model: event.currentTarget.value === record.tool ? record.model ?? '' : '', effort: '' } }}>
            {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
          </select></label>
          <label>Model<GafferModelSelect label={`Model for ${item.title}`} tool={who.tool} value={who.model} machine={offered[who.tool] ?? null} {favourites} host={machine?.host ?? ''} disabled={busy} onchange={next => { crew[item.title] = { ...choice(item.title), model: next } }}/></label>
          <label>Effort<select aria-label={`Effort for ${item.title}`} value={who.effort} disabled={busy || effortChoices(who.tool).length < 2} onchange={event => { crew[item.title] = { ...choice(item.title), effort: event.currentTarget.value } }}>
            {#each effortChoices(who.tool) as level (level)}<option value={level}>{level ? `${level[0].toUpperCase()}${level.slice(1)}` : effortChoices(who.tool).length < 2 ? 'No effort setting' : 'Default effort'}</option>{/each}
          </select></label>
        </div>
      </li>{/each}
    </ul>
    <p class="hint">Each pipeline starts on this outcome’s agent and model. Change any of them for the job: a stronger model for a hard one, a cheaper one for a chore.</p>
    {#if needsTrust}<p class="warning" role="status">This outcome was not approved to run a folder’s plugins. OpenCode or Google Antigravity is refused in a folder that has any.</p>{/if}
    <div class="actions"><button class="btn primary" type="button" disabled={busy} onclick={approve}>{proposals.length === 1 ? 'Add pipeline' : 'Add pipelines'}</button><button class="btn quiet" type="button" disabled={busy} onclick={oncancel}>Cancel</button></div>
  </div>
</WorkspaceDialog>

<style>
  .add{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted)}.warning{color:var(--bad)}
  .rows{list-style:none;margin:0;padding:0;display:grid;gap:12px}.rows li{display:grid;gap:10px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}
  .what{display:grid;gap:2px;min-width:0}.what span{font-size:0.875rem;color:var(--muted)}
  .who{display:grid;grid-template-columns:1fr 1.3fr .9fr;gap:10px;align-items:start}label{font-weight:600;display:grid;gap:6px;min-width:0}
  select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;min-height:44px;box-sizing:border-box}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,select:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.who{grid-template-columns:1fr}}
</style>
