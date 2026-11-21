<script lang="ts">
  // Approving pipelines Gaffer proposed. Each one can be given its own agent, model and effort; left alone it uses the
  // outcome's. Nothing is added or started until the person approves, and an added pipeline never merges by itself.
  import { onMount } from 'svelte'
  import { getGafferCrew, listModelFavourites, type GafferCrew, type GafferOutcome } from '../api'
  import { chatToolName, machineModels, structuredAgent, terminalMachines, type TerminalMachine } from '../terminal/api'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import GafferModelSelect from '../GafferModelSelect.svelte'
  import { CREW_TIERS, DEFAULT_STAGES, STAGES, defaultTier, partsBlocker, stagePlan, stageParts, stageTakesParts, withNewPart, withPart, withoutParts, withReportedEfforts, type Crew, type CrewSpare, type StagePart } from '../gafferOutcome'
  import StageParts from './StageParts.svelte'
  import type { MachineModels, ModelFavourite } from '../models'
  import { START_EFFORTS, reportedEfforts } from '../terminal/launchOptions'

  type Proposal = { title: string; prompt: string; why?: string; stages?: string[] }
  export type AddedPipeline = { title: string; prompt: string; stages: ReturnType<typeof stagePlan> }
  let { record, proposals, busy = false, error = '', oncancel, onconfirm }: {
    record: GafferOutcome; proposals: Proposal[]; busy?: boolean; /** Why the last approval was refused. */ error?: string; oncancel: () => void; onconfirm: (pipelines: AddedPipeline[]) => void
  } = $props()
  const CHAT_TOOLS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  const life = new AbortController()
  let savedCrew = $state<GafferCrew | null>(null), crewLoading = $state(true)
  let machine = $state<TerminalMachine | null>(null), favourites = $state<ModelFavourite[]>([]), offered = $state<Record<string, MachineModels | null>>({}), capabilitiesRead = $state(false)
  // What each added pipeline runs on: one agent for every stage (`main`), or a crew with its own agent, model and
  // effort for the thinking, building and shipping stages. It starts as the outcome's own agent and model.
  type Who = { tool: string; model: string; effort: string }
  let main = $state<Record<string, Who>>({}), crews = $state<Record<string, Crew>>({})
  const known = $derived(!!machine?.capabilities)
  const tools = $derived.by(() => {
    const found = machine ? CHAT_TOOLS.filter(id => structuredAgent(machine!.capabilities, id)) : []
    // When the machine's report is known, only the agents it offers are listed: silence about the outcome's own agent
    // leaves it out rather than bringing it back. When the report is not known, every saved crew agent is offered too.
    const kept = known ? (found.includes(record.tool) ? [record.tool] : []) : [record.tool]
    const saved = known ? [] : CREW_TIERS.flatMap(({ tier }) => {
      const member = savedCrew?.[tier]
      return [member, member?.backup, ...(member?.moreBackups ?? [])].flatMap(choice => choice?.tool ? [choice.tool] : [])
    })
    return [...new Set([...kept, ...found, ...saved])]
  })
  const stageLabel = (kind: string) => STAGES.find(item => item.kind === kind)?.label ?? kind
  const kinds = (item: Proposal) => item.stages?.length ? item.stages : DEFAULT_STAGES
  // An added pipeline starts as the outcome runs: its agent and model, and the effort its own stages on that agent were given.
  const inherited = () => record.sessions.flatMap(piece => piece.stages ?? []).find(stage => stage.tool === record.tool && stage.effort)?.effort ?? ''
  // The outcome's own agent and model, unless this machine does not offer it: then the first it does offer, on its default
  // model and effort.
  const own = (): Who => tools.length && !tools.includes(record.tool)
    ? { tool: tools[0], model: '', effort: '' }
    : { tool: record.tool, model: record.model ?? '', effort: inherited() }
  const choice = (title: string) => main[title] ?? own()
  const savedRole = (title: string, tier: keyof Crew): Crew[keyof Crew] => {
    const member = savedCrew?.[tier], fallback = choice(title)
    return member?.tool && tools.includes(member.tool)
      ? { tool: member.tool, model: member.model ?? '', effort: member.effort, source: member.provider }
      : { ...fallback, source: fallback.tool === record.tool ? record.provider : undefined }
  }
  const crewOf = (title: string): Crew => crews[title] ?? { heavy: savedRole(title, 'heavy'), standard: savedRole(title, 'standard'), light: savedRole(title, 'light') }
  // Each tier's backup comes from the person's saved crew, for the agents this machine can run: an added pipeline is
  // balanced across plans like the outcome's own. It is changed where the crew is, in Gaffer settings.
  let spares = $state<Partial<Record<keyof Crew, CrewSpare[]>>>({})
  // What backs a pipeline's tier up here: the saved crew's backups ('crew', where there are any), none (''), or an
  // agent of the person's own choosing for this pipeline, with its model and effort.
  let picked = $state<Record<string, { tool: string; model: string; effort: string }>>({})
  const crewSpares = (tier: keyof Crew) => (spares[tier] ?? []).filter(one => tools.includes(one.tool))
  const spareKey = (title: string, tier: keyof Crew) => `${title}\n${tier}`
  const spareChoice = (title: string, tier: keyof Crew) => picked[spareKey(title, tier)]?.tool ?? (crewSpares(tier).length ? 'crew' : '')
  const spare = (title: string, tier: keyof Crew): CrewSpare[] => { const own = picked[spareKey(title, tier)]; return !own ? crewSpares(tier) : own.tool && own.tool !== 'crew' ? [own] : own.tool === 'crew' ? crewSpares(tier) : [] }
  const pickSpare = (title: string, tier: keyof Crew, next: Partial<{ tool: string; model: string; effort: string }>) => { const key = spareKey(title, tier); picked[key] = { ...(picked[key] ?? { tool: spareChoice(title, tier), model: '', effort: '' }), ...next } }
  const withSpares = (title: string, tier: keyof Crew, member: Crew[keyof Crew]) => { const list = spare(title, tier); return { ...member, backup: list[0], more: list.slice(1) } }
  const crewSent = (title: string): Crew => { const crew = crewOf(title); return { heavy: withSpares(title, 'heavy', crew.heavy), standard: withSpares(title, 'standard', crew.standard), light: withSpares(title, 'light', crew.light) } }
  // A stage of an added pipeline can be split into parts, as in a new outcome.
  let splits = $state<Record<string, Record<string, StagePart[]>>>({})
  const ratingsOf = (title: string) => ({ parts: splits[title] ?? {} })
  const spareWords = (one: CrewSpare) => `${chatToolName(one.tool)}${one.model.trim() ? ` · ${one.model.trim()}` : ''}${one.effort ? ` · ${one.effort} effort` : ''}`
  const whoWords = (title: string, tier: keyof Crew) => { const one = crewOf(title)[tier]; return `${chatToolName(one.tool)}${one.model.trim() ? ` · ${one.model.trim()}` : ''}${one.effort ? ` · ${one.effort} effort` : ''}` }
  const blocked = $derived(partsBlocker(proposals.map(item => ({ title: item.title, stages: kinds(item), parts: splits[item.title] }))))
  const usedTools = (title: string) => CREW_TIERS.map(item => crewOf(title)[item.tier].tool)
  // What this machine offers is known once it has been read with its capabilities. Until then (or when the read failed,
  // or the outcome's machine is gone) the start levels are offered and every effort is sent as chosen: the controller
  // starts a stage on the agent's default effort by itself when its effort is not offered.
  const efforts = (id: string) => known ? reportedEfforts(id, machine?.capabilities) : ['', ...START_EFFORTS]
  // The levels a select shows: a chosen level is kept in view while nothing is known about it.
  const shown = (id: string, chosen: string | undefined) => !known && chosen && !efforts(id).includes(chosen) ? [...efforts(id), chosen] : efforts(id)
  const effortLabel = (id: string, level: string) => level ? `${level[0].toUpperCase()}${level.slice(1)}` : efforts(id).length < 2 ? 'No effort setting' : 'Default effort'
  // The plan each added pipeline would send, and every agent in it this machine reports it does not offer. Approving is
  // refused while one is missing: its stage could only fail at its start.
  const plans = $derived(proposals.map(item => ({
    title: item.title, prompt: item.prompt,
    stages: withReportedEfforts(stagePlan(kinds(item), crewSent(item.title), true, ratingsOf(item.title)), known ? efforts : null),
  })))
  const unoffered = $derived(known ? [...new Set(plans.flatMap(({ stages }) => stages.flatMap(stage => [stage.tool, stage.backup?.tool, ...(stage.moreBackups ?? []).map(one => one.tool)])))].filter((id): id is string => !!id && !tools.includes(id)) : [])
  const needsTrust = $derived(!record.trust && proposals.some(item => usedTools(item.title).some(id => id === 'opencode')))
  // Antigravity never starts in a folder that was not trusted, and trust is given when an outcome is approved.
  const refused = $derived(!record.trust && proposals.some(item => usedTools(item.title).includes('antigravity')))
  // Setting the pipeline's own agent, model or effort gives it to every role; a role then keeps what it is given.
  function setMain(title: string, next: Who) {
    main[title] = next
    const member = { ...next, source: next.tool === record.tool ? record.provider : undefined }
    crews[title] = { heavy: { ...member }, standard: { ...member }, light: { ...member } }
  }
  function setRole(title: string, role: keyof Crew, next: Partial<Who>) {
    const current = crewOf(title), member = current[role]
    crews[title] = { ...current, [role]: { ...member, ...next, source: next.tool && next.tool !== member.tool ? (next.tool === record.tool ? record.provider : undefined) : member.source } }
  }
  const split = (title: string) => CREW_TIERS.some(item => { const who = crewOf(title)[item.tier], top = choice(title); return who.tool !== top.tool || who.model.trim() !== top.model.trim() || (who.effort ?? '') !== top.effort })

  // Once this machine's agents are known, a level one of them does not report goes back to the agent's default, so what
  // is shown is what will be sent: the controller would start the stage on that default and record why.
  $effect(() => {
    if (!capabilitiesRead || !known) return
    for (const item of proposals) {
      const top = main[item.title]
      if (top && !tools.includes(top.tool)) setMain(item.title, own())
      else if (top?.effort && !efforts(top.tool).includes(top.effort)) main[item.title] = { ...top, effort: '' }
      for (const part of CREW_TIERS) {
        const role = crews[item.title]?.[part.tier]
        if (role && !tools.includes(role.tool)) setRole(item.title, part.tier, own())
        else if (role?.effort && !efforts(role.tool).includes(role.effort)) setRole(item.title, part.tier, { effort: '' })
      }
    }
    for (const [key, one] of Object.entries(picked)) if (one.effort && !efforts(one.tool).includes(one.effort)) picked[key] = { ...one, effort: '' }
  })
  $effect(() => {
    for (const id of new Set(proposals.flatMap(item => usedTools(item.title)))) {
      if (!id || id in offered) continue
      offered[id] = null
      void machineModels(record.agentId, id, life.signal).then(found => { offered[id] = found }).catch(() => { /* suggestions and Other still work */ })
    }
  })
  onMount(() => {
    void getGafferCrew(life.signal).then(answer => { if (!answer.saved) return; savedCrew = answer.crew; for (const item of CREW_TIERS) { const member = answer.crew[item.tier]; const list = [member?.backup, ...(member?.moreBackups ?? [])].flatMap(one => one?.tool ? [{ tool: one.tool, model: one.model ?? '', effort: one.effort ?? '', source: one.provider ?? '' }] : []); if (list.length) spares[item.tier] = list } }).catch(() => {}).finally(() => { crewLoading = false })
    void terminalMachines(life.signal).then(found => { machine = found.find(item => item.agentId === record.agentId) ?? null }).catch(() => { /* the outcome's own agent is still offered */ }).finally(() => { capabilitiesRead = true })
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => { /* a helper only */ })
    return () => life.abort()
  })
  function approve() { onconfirm(plans) }
</script>

<WorkspaceDialog title={proposals.length === 1 ? `Add “${proposals[0].title}”?` : `Add ${proposals.length} pipelines?`} onclose={oncancel}>
  <div class="add">
    <p>The controller starts each added pipeline as a chat in this outcome’s project, {record.yolo ? 'on YOLO like the rest of this outcome' : 'with ordinary approvals, never in YOLO'}, and Gaffer looks after it stage by stage. None of them merges by itself: each stops at <strong>Ready to merge</strong> and waits for you.</p>
    {#if (record.images ?? []).length}<p class="hint" role="status">{record.images!.length === 1 ? 'The image attached to this outcome travels' : `All ${record.images!.length} images attached to this outcome travel`} with each new pipeline.</p>{/if}
    <ul class="rows" aria-label="Pipelines to add">
      {#each proposals as item (item.title)}{@const who = choice(item.title)}<li role="group" aria-label={item.title}>
        <div class="what"><strong>{item.title}</strong><span>{kinds(item).map(stageLabel).join(' → ')}</span></div>
        <div class="who">
          <label>Agent<select aria-label={`Agent for ${item.title}`} value={who.tool} disabled={busy} onchange={event => setMain(item.title, { tool: event.currentTarget.value, model: event.currentTarget.value === record.tool ? record.model ?? '' : '', effort: '' })}>
            {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
          </select></label>
          <label>Model<GafferModelSelect label={`Model for ${item.title}`} tool={who.tool} value={who.model} machine={offered[who.tool] ?? null} {favourites} host={machine?.host ?? ''} disabled={busy} onchange={next => setMain(item.title, { ...choice(item.title), model: next })}/></label>
          <label>Effort<select aria-label={`Effort for ${item.title}`} value={who.effort} disabled={busy || efforts(who.tool).length < 2} onchange={event => setMain(item.title, { ...choice(item.title), effort: event.currentTarget.value })}>
            {#each shown(who.tool, who.effort) as level (level)}<option value={level}>{effortLabel(who.tool, level)}</option>{/each}
          </select></label>
        </div>
        <details class="crew"><summary>{split(item.title) ? 'A crew does the stages' : 'One agent does every stage'} <span>Choose who does what</span></summary>
          <div class="crew-rows">
            {#each CREW_TIERS as part (part.tier)}{@const role = crewOf(item.title)[part.tier]}<div class="crew-row" role="group" aria-label={`${part.label} crew for ${item.title}`}>
              <div class="crew-role"><strong>{part.label}</strong><span>{part.does}</span></div>
              <select aria-label={`${part.label} agent for ${item.title}`} value={role.tool} disabled={busy} onchange={event => setRole(item.title, part.tier, { tool: event.currentTarget.value, model: '', effort: '' })}>{#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
              <GafferModelSelect label={`${part.label} model for ${item.title}`} tool={role.tool} value={role.model} machine={offered[role.tool] ?? null} {favourites} host={machine?.host ?? ''} disabled={busy} onchange={next => setRole(item.title, part.tier, { model: next })}/>
              <select aria-label={`${part.label} effort for ${item.title}`} value={role.effort ?? ''} disabled={busy || efforts(role.tool).length < 2} onchange={event => setRole(item.title, part.tier, { effort: event.currentTarget.value })}>{#each shown(role.tool, role.effort) as level (level)}<option value={level}>{effortLabel(role.tool, level)}</option>{/each}</select>
              {#if true}{@const chosen = spareChoice(item.title, part.tier)}{@const own = picked[spareKey(item.title, part.tier)]}
              <div class="crew-spare" role="group" aria-label={`${part.label} backup for ${item.title}`}>
                <span class="spare-name">Backup</span>
                <select aria-label={`${part.label} backup agent for ${item.title}`} value={chosen} disabled={busy} onchange={event => pickSpare(item.title, part.tier, { tool: event.currentTarget.value, model: '', effort: '' })}>
                  {#if crewSpares(part.tier).length}<option value="crew">From your crew: {crewSpares(part.tier).map(spareWords).join(', then ')}</option>{/if}
                  <option value="">No backup</option>
                  {#each tools as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}
                </select>
                {#if own && chosen && chosen !== 'crew'}
                  <GafferModelSelect label={`${part.label} backup model for ${item.title}`} tool={chosen} value={own.model} machine={offered[chosen] ?? null} {favourites} host={machine?.host ?? ''} disabled={busy} onchange={next => pickSpare(item.title, part.tier, { model: next })}/>
                  <select aria-label={`${part.label} backup effort for ${item.title}`} value={own.effort} disabled={busy || efforts(chosen).length < 2} onchange={event => pickSpare(item.title, part.tier, { effort: event.currentTarget.value })}>{#each shown(chosen, own.effort) as level (level)}<option value={level}>{effortLabel(chosen, level)}</option>{/each}</select>
                {/if}
              </div>{/if}
            </div>{/each}
          </div>
          <p class="hint">{kinds(item).length} {kinds(item).length === 1 ? 'stage' : 'stages'}, each in an agent session of its own.</p>
          <div class="splits" role="group" aria-label={`Split stages of ${item.title}`}>
            {#each kinds(item).filter(stageTakesParts) as kind (kind)}{@const parts = stageParts(ratingsOf(item.title), kind)}
              {#if parts.length > 1}<StageParts {kind} where={item.title} {parts} {busy} who={tier => whoWords(item.title, tier)}
                onchange={(at, next) => splits[item.title] = withPart(ratingsOf(item.title), kind, at, next)} onadd={() => splits[item.title] = withNewPart({ parts: splits[item.title], tiers: { [kind]: defaultTier(kind) } }, kind)} onjoin={() => splits[item.title] = withoutParts(ratingsOf(item.title), kind)}/>{/if}
            {/each}
            {#if kinds(item).some(kind => stageTakesParts(kind) && stageParts(ratingsOf(item.title), kind).length < 2)}<p class="split-offer">Split a stage into parts of different work:{#each kinds(item).filter(kind => stageTakesParts(kind) && stageParts(ratingsOf(item.title), kind).length < 2) as kind (kind)}<button class="inline" type="button" disabled={busy} aria-label={`Split ${stageLabel(kind)} of ${item.title} into parts`} onclick={() => splits[item.title] = withNewPart({ parts: splits[item.title], tiers: { [kind]: defaultTier(kind) } }, kind)}>{stageLabel(kind)}</button>{/each}</p>{/if}
          </div>
        </details>
      </li>{/each}
    </ul>
    <p class="hint">Stages use your saved crew where available. Unset tiers use this outcome’s agent and model.</p>
    {#if refused}<p class="warning" role="alert">Google Antigravity starts only in a folder you have trusted, and this outcome was approved without folder trust. Choose another agent here, or give Gaffer a new outcome with folder trust ticked.</p>
    {:else if needsTrust}<p class="warning" role="status">This outcome was approved without folder trust. OpenCode is refused in a folder that has plugins of its own.</p>{/if}
    {#if proposals.some(item => usedTools(item.title).includes('cline'))}<p class="hint">{record.tool === 'cline' && record.provider ? 'Cline stages keep this outcome’s billing source.' : 'Cline stages run on the billing source Cline has saved on that machine. To choose another, give Gaffer a new outcome.'}</p>{/if}
    {#if known && !tools.length}<p class="warning" role="alert">This machine reports no agent that can run a Gaffer chat. Update reet on the machine, or choose another project for a new outcome.</p>
    {:else if unoffered.length}<p class="warning" role="alert">{unoffered.map(chatToolName).join(' and ')} {unoffered.length === 1 ? 'is' : 'are'} not on this machine now. Choose another agent for the pipeline.</p>{/if}
    {#if error}<p class="warning" role="alert">{error}</p>{/if}
    <div class="actions"><button class="btn primary" type="button" disabled={busy || crewLoading || !capabilitiesRead || refused || !!blocked || unoffered.length > 0} onclick={approve}>{proposals.length === 1 ? 'Add pipeline' : 'Add pipelines'}</button><button class="btn quiet" type="button" disabled={busy} onclick={oncancel}>Cancel</button></div>
  </div>
</WorkspaceDialog>

<style>
  .add{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted)}.warning{color:var(--bad)}
  .rows{list-style:none;margin:0;padding:0;display:grid;gap:12px}.rows li{display:grid;gap:10px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}
  .what{display:grid;gap:2px;min-width:0}.what span{font-size:0.875rem;color:var(--muted)}
  .who{display:grid;grid-template-columns:1fr 1.3fr .9fr;gap:10px;align-items:start}label{font-weight:600;display:grid;gap:6px;min-width:0}
  select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;min-height:44px;box-sizing:border-box}
  .crew summary{min-height:44px;display:list-item;align-content:center;cursor:pointer}.crew summary span{color:var(--accent-text);margin-left:8px}
  .crew-spare{grid-column:1/-1;display:grid;grid-template-columns:subgrid;gap:6px 10px;align-items:start}.spare-name{grid-column:1/-1;font-size:0.875rem;font-weight:600;color:var(--muted)}.crew-spare select{min-width:0;width:100%}.crew-spare>select:last-child{grid-column:1/-1}
  .splits{display:grid;gap:8px;margin:8px 0 0}.split-offer{display:flex;flex-wrap:wrap;gap:4px 14px;align-items:center;font-size:0.875rem;color:var(--muted)}.split-offer button{min-height:44px;border:0;background:none;padding:0 2px;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer}.split-offer button:hover{text-decoration:underline}
  .crew-rows{display:grid;gap:10px;padding:6px 0 8px}.crew-row{display:grid;grid-template-columns:1fr 1.3fr .9fr;gap:6px 10px;align-items:start}.crew-role{grid-column:1/-1;display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 8px;min-width:0}.crew-role span{font-size:0.875rem;color:var(--muted)}
  summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,select:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:559px){.who,.crew-row{grid-template-columns:1fr}}
</style>
