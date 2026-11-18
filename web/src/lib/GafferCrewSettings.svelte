<script lang="ts">
  // The account's crew, in Gaffer settings: who does a heavy, a standard and a light stage, each tier's backup, when a
  // backup takes over, and whether the outcome dialog starts on "Gaffer decides". Every outcome starts from these and can
  // still be changed when it is approved. No one machine is in view here, so the models and efforts offered are those of
  // every machine on the account; a saved choice none of them offers is kept and said so. Nothing here blocks saving.
  import { onMount } from 'svelte'
  import { getGafferCrew, listModelFavourites, saveGafferCrew, type GafferCrew, type GafferCrewTier } from './api'
  import { BALANCE_LOW_PERCENT, BALANCE_MAX_PERCENT, BALANCE_MIN_PERCENT, CREW_TIERS, MAX_BACKUPS, balancePercent } from './gafferOutcome'
  import { allModels, chatToolName, structuredAgent, terminalMachines, type TerminalMachine } from './terminal/api'
  import { reportedEfforts } from './terminal/launchOptions'
  import { agentOptionLabel, crewEffortChoices, crewModelNote, effortUnion, mergeMachineModels } from './crewChoices'
  import type { MachineModels, ModelFavourite } from './models'
  import GafferModelSelect from './GafferModelSelect.svelte'
  import ClineSourceSelect from './ClineSourceSelect.svelte'

  // A crew may be set before a machine connects, so every agent is listed; one no machine runs says so.
  const AGENTS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  // `source` is the billing source a Cline row starts on; empty is Cline's own. `key` tells further backups apart, so a
  // row keeps its own fields when one above it is removed; it is never saved.
  type Row = { tool: string; model: string; effort: string; source?: string; key?: number }
  let nextKey = 0
  type Tier = 'heavy' | 'standard' | 'light'
  const blank = (): Row => ({ tool: '', model: '', effort: '' })
  let first = $state<Record<Tier, Row>>({ heavy: blank(), standard: blank(), light: blank() })
  let spare = $state<Record<Tier, Row>>({ heavy: blank(), standard: blank(), light: blank() })
  // Further backups of a tier, tried in order after the first.
  let others = $state<Record<Tier, Row[]>>({ heavy: [], standard: [], light: [] })
  let below = $state<number | string>(BALANCE_LOW_PERCENT), decides = $state(false)
  let loading = $state(true), busy = $state(false), error = $state(''), notice = $state(''), savedAt = $state(0)
  const formId = $props.id()
  const life = new AbortController()

  // The account's machines, read once: null until they are read. Each agent's models across them, read once per agent
  // when it is first in the crew, from what the controller kept (no machine is asked). A failed read leaves the agent's
  // default, favourites, suggestions and Other…
  let machines = $state<TerminalMachine[] | null>(null), machinesFailed = $state(false)
  let models = $state<Record<string, MachineModels | null>>({}), modelsRead = $state<Record<string, boolean>>({}), modelsFailed = $state<Record<string, boolean>>({})
  let favourites = $state<ModelFavourite[]>([])

  function show(crew: GafferCrew) {
    for (const item of CREW_TIERS) {
      const member: GafferCrewTier = crew[item.tier] ?? {}
      first[item.tier] = { tool: member.tool ?? '', model: member.model ?? '', effort: member.effort ?? '', source: member.provider ?? '' }
      spare[item.tier] = { tool: member.backup?.tool ?? '', model: member.backup?.model ?? '', effort: member.backup?.effort ?? '', source: member.backup?.provider ?? '' }
      others[item.tier] = (member.moreBackups ?? []).map(one => ({ tool: one.tool, model: one.model ?? '', effort: one.effort ?? '', source: one.provider ?? '', key: nextKey++ }))
    }
    below = balancePercent(crew.balanceBelow); decides = !!crew.gafferDecides; savedAt = crew.updatedAt ?? 0
  }
  onMount(() => {
    void getGafferCrew(life.signal).then(answer => show(answer.crew)).catch(e => { if (!life.signal.aborted) error = e instanceof Error ? e.message : 'Your crew could not be read.' }).finally(() => { loading = false })
    void terminalMachines(life.signal).then(found => { machines = found }).catch(() => { if (!life.signal.aborted) machinesFailed = true })
    void listModelFavourites(life.signal).then(found => { favourites = found.favourites }).catch(() => { /* a helper only */ })
    return () => life.abort()
  })
  const used = $derived([...new Set(CREW_TIERS.flatMap(item => [first[item.tier].tool, spare[item.tier].tool, ...others[item.tier].map(row => row.tool)]).filter(Boolean))])
  $effect(() => {
    for (const id of used) {
      if (id in models) continue
      models[id] = null
      void allModels(id, life.signal).then(answer => { models[id] = mergeMachineModels(id, answer.machines); modelsRead[id] = true }).catch(() => { if (!life.signal.aborted) modelsFailed[id] = true })
    }
  })
  // Whether any machine runs the agent as a chat, which every stage is; null while the machines are unknown.
  const onMachines = (id: string): boolean | null => machines ? machines.some(one => !!structuredAgent(one.capabilities, id)) : null
  // The levels the machines report for the agent: undefined while they are being read, null when nothing is known for
  // it (the read failed, or no machine runs it, the account having none included), so the start levels are offered.
  const offeredEfforts = (id: string): string[] | null | undefined => machinesFailed ? null : !machines ? undefined : onMachines(id) ? effortUnion(machines.map(one => reportedEfforts(id, one.capabilities))) : null
  const failedLine = $derived(machinesFailed && used.some(id => modelsFailed[id])
    ? 'Your machines and their models could not be read: efforts are not checked against them, and Other… takes a model’s name. You can still save your crew.'
    : machinesFailed ? 'Your machines could not be read, so efforts are not checked against them. You can still save your crew.'
    : used.some(id => modelsFailed[id]) ? 'Your machines’ model lists could not be read: choose Other… to type a model’s name. You can still save your crew.' : '')
  // Another agent's model, effort or billing source means nothing to this one.
  const cleared = (tool: string): Row => ({ tool, model: '', effort: '', source: '' })
  // An id for a row's descriptions, from the row's label ("Heavy backup 2").
  const idOf = (label: string, what: string) => `${formId}-${label.replace(/\W+/g, '-')}-${what}`

  // A row as it is saved: a Cline row carries its billing source when one was chosen.
  const sent = (one: Row) => ({ tool: one.tool, model: one.model.trim(), effort: one.effort.trim(), ...(one.tool === 'cline' && one.source?.trim() ? { provider: one.source.trim() } : {}) })
  const tier = (name: Tier): GafferCrewTier => {
    const one = first[name], two = spare[name], rest = others[name].filter(row => row.tool)
    if (!one.tool) return {}
    return { ...sent(one), ...(two.tool ? { backup: sent(two), ...(rest.length ? { moreBackups: rest.map(sent) } : {}) } : {}) }
  }
  async function save() {
    if (busy) return
    busy = true; error = ''; notice = ''
    try {
      show((await saveGafferCrew({ heavy: tier('heavy'), standard: tier('standard'), light: tier('light'), balanceBelow: balancePercent(below), gafferDecides: decides })).crew)
      notice = 'Saved. New outcomes start from this crew.'
    } catch (e) { error = e instanceof Error ? e.message : 'Your crew was not saved.' }
    busy = false
  }
</script>

{#snippet agents()}{#each AGENTS as id (id)}<option value={id}>{agentOptionLabel(chatToolName(id), onMachines(id))}</option>{/each}{/snippet}
{#snippet fields(row: Row, label: string)}
  {@const effort = crewEffortChoices({ offered: offeredEfforts(row.tool), saved: row.effort })}
  {@const why = effort.unoffered ? idOf(label, 'effort-hint') : effort.disabled ? idOf(label, 'effort-none') : undefined}
  <GafferModelSelect label={`${label} model`} tool={row.tool} value={row.model} onchange={next => { row.model = next }} machine={models[row.tool] ?? null} {favourites} everywhere disabled={busy}
    note={crewModelNote({ failed: !!modelsFailed[row.tool], read: !!modelsRead[row.tool], listed: !!models[row.tool]?.models?.length })}/>
  <span class="effort">
    <select aria-label={`${label} effort`} aria-describedby={why} value={row.effort} disabled={busy || effort.disabled} onchange={event => { row.effort = (event.currentTarget as HTMLSelectElement).value }}>
      {#each effort.options as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
    </select>
    {#if effort.unoffered}<span class="caveat" id={idOf(label, 'effort-hint')}>If the agent does not offer this effort, the stage starts on the agent’s default effort.</span>{/if}
    {#if effort.disabled}<span class="sr-only" id={idOf(label, 'effort-none')}>{chatToolName(row.tool)} has no effort setting</span>{/if}
  </span>
  {#if row.tool === 'cline'}<div class="source"><ClineSourceSelect label={`${label} Cline billing source`} showLabel={false} star={false} value={row.source ?? ''} saved={row.source ?? ''} disabled={busy} onchange={next => { row.source = next }} /></div>{/if}
{/snippet}

<section class="crew" aria-label="Your Factory crew">
  <h3>Your Factory crew</h3>
  <p class="note">Who does each kind of Factory stage. Gaffer says how demanding a stage is; this is what that means for you. Every outcome starts from here and you can still change it when you approve one.</p>
  {#if loading}<p role="status">Reading your crew…</p>
  {:else}
  <form onsubmit={event => { event.preventDefault(); void save() }}>
    <div class="live" role="status">{#if failedLine}<p class="hint warn">{failedLine}</p>{/if}</div>
    <div class="tiers">
      {#each CREW_TIERS as item (item.tier)}<div class="tier" role="group" aria-label={`${item.label} tier`}>
        <div class="tier-head"><strong>{item.label}</strong><span>{item.does}</span></div>
        <span class="slot">First choice</span>
        <select aria-label={`${item.label} agent`} bind:value={first[item.tier].tool} disabled={busy} onchange={() => { if (!first[item.tier].tool) { first[item.tier] = blank(); spare[item.tier] = blank(); others[item.tier] = [] } else first[item.tier] = cleared(first[item.tier].tool) }}><option value="">The outcome’s own agent</option>{@render agents()}</select>
        {#if first[item.tier].tool}
          {@render fields(first[item.tier], item.label)}
          <span class="slot">Backup</span>
          <select aria-label={`${item.label} backup agent`} bind:value={spare[item.tier].tool} disabled={busy} onchange={() => { if (!spare[item.tier].tool) { spare[item.tier] = blank(); others[item.tier] = [] } else spare[item.tier] = cleared(spare[item.tier].tool) }}><option value="">No backup</option>{@render agents()}</select>
          {#if spare[item.tier].tool}
            {@render fields(spare[item.tier], `${item.label} backup`)}
            {#each others[item.tier] as extra, n (extra.key)}
              <span class="slot">Then</span>
              <select aria-label={`${item.label} backup ${n + 2} agent`} bind:value={extra.tool} disabled={busy} onchange={() => { if (!extra.tool) others[item.tier] = others[item.tier].filter(row => row.key !== extra.key); else others[item.tier][n] = { ...cleared(extra.tool), key: extra.key } }}><option value="">Remove this backup</option>{@render agents()}</select>
              {@render fields(extra, `${item.label} backup ${n + 2}`)}
            {/each}
            {#if others[item.tier].length < MAX_BACKUPS - 1}<button class="link more" type="button" disabled={busy} aria-label={`Add another backup for ${item.label}`} onclick={() => others[item.tier] = [...others[item.tier], { tool: AGENTS.find(id => id !== spare[item.tier].tool) ?? AGENTS[0], model: '', effort: '', key: nextKey++ }]}>Add another backup</button>{/if}
          {:else}<span class="none">Stages stay on the first choice.</span>{/if}
        {:else}<span class="none">Stages of this tier run on whichever agent the outcome is given.</span>{/if}
      </div>{/each}
    </div>
    <div class="row" role="group" aria-label="When a backup takes over">
      <label for={`${formId}-below`}>Switch to a backup below</label>
      <span class="percent"><input id={`${formId}-below`} type="number" inputmode="numeric" min={BALANCE_MIN_PERCENT} max={BALANCE_MAX_PERCENT} step="5" bind:value={below} disabled={busy} aria-describedby={`${formId}-below-unit`} onblur={() => below = balancePercent(below)}/><span id={`${formId}-below-unit`}>% of the first choice’s plan left</span></span>
    </div>
    <p class="hint">A stage starts on its tier’s backup when the first choice’s plan has under this much left in its tightest window and the backup’s has more. It needs plan limits to be on for the machine.</p>
    <div class="row who" role="group" aria-label="Decisions, when a stage stops"><span class="who-name">Decisions, when a stage stops</span>
      <div class="choice" role="radiogroup" aria-label="Who decides by default">
        <label class:on={!decides}><input type="radio" name={`${formId}-who`} checked={!decides} disabled={busy} onchange={() => decides = false}/>Ask me</label>
        <label class:on={decides}><input type="radio" name={`${formId}-who`} checked={decides} disabled={busy} onchange={() => decides = true}/>Gaffer decides</label>
      </div>
    </div>
    <p class="hint">This is only where the outcome dialog starts. You still choose for each outcome when you approve it, and an outcome started any other way, such as over MCP, is never given to Gaffer by this.</p>
    {#if error}<p class="feedback bad" role="alert">{error}</p>{/if}
    <div class="actions"><button class="btn primary" type="submit" disabled={busy}>{busy ? 'Saving…' : 'Save crew'}</button><span role="status">{notice || (savedAt ? `Last saved ${new Date(savedAt).toLocaleString('en-GB', { dateStyle: 'medium', timeStyle: 'short' })}.` : 'Not saved yet: outcomes use their own agent for every stage.')}</span></div>
  </form>{/if}
</section>

<style>
  .crew{display:grid;gap:12px;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--card);min-width:0}h3{margin:0;font:600 1.05rem var(--sans)}
  .note,.hint{margin:0;color:var(--muted);font-size:.9375rem;line-height:1.5}form{display:grid;gap:12px;min-width:0}
  .tiers{display:grid;gap:0}.tier{display:grid;grid-template-columns:6rem minmax(0,1fr) minmax(0,1.25fr) minmax(0,1fr);gap:8px 10px;align-items:start;padding:12px 0;border-top:1px solid var(--line)}.tier:first-child{border-top:0;padding-top:0}
  .tier-head{grid-column:1/-1;display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 10px}.tier-head span{color:var(--muted);font-size:.875rem}
  .slot{line-height:44px;font-size:.875rem;font-weight:600;color:var(--muted)}.none{grid-column:3/-1;line-height:44px;font-size:.875rem;color:var(--muted)}
  select,input{width:100%;min-width:0;min-height:44px;font:inherit}.source{grid-column:2/-1}.effort{display:grid;gap:6px;min-width:0}.caveat{font-size:.875rem;line-height:1.5;color:var(--muted)}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap;border:0}
  /* The failed-read line's live region is always there, so a screen reader hears the line when it comes; empty, it takes no room. */
  .live:empty{position:absolute}
  /* The model and source selects are components of their own: they take this form's field colours so a row reads as one. */
  .tier :global(.model select),.tier :global(.model input),.tier :global(.cline-source select){background:var(--card);border-color:var(--line-strong);padding:8px 12px}.tier :global(.model select:hover),.tier :global(.model input:hover),.tier :global(.cline-source select:hover){border-color:var(--muted)}.tier :global(.model select:focus),.tier :global(.model input:focus),.tier :global(.cline-source select:focus){border-color:var(--accent)}.hint.warn{color:var(--warn)}
  .link{border:0;background:none;padding:0;font:inherit;font-size:.875rem;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:44px}.link:hover{text-decoration:underline}.more{grid-column:2/-1;justify-self:start}
  .row{display:grid;grid-template-columns:minmax(0,auto) minmax(0,1fr);align-items:center;gap:6px 12px;margin:0;padding:12px 0 0;border:0;border-top:1px solid var(--line);min-width:0}.row label,.who-name{font-weight:600}.who{grid-template-columns:1fr}
  .percent{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px}.percent input{width:5.5rem;text-align:right;font-variant-numeric:tabular-nums}
  .choice{display:inline-flex;justify-self:start;border:1px solid var(--line);border-radius:10px;overflow:hidden}.choice label{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 16px;font-weight:600;cursor:pointer;color:var(--muted)}.choice label+label{border-left:1px solid var(--line)}.choice label.on{color:var(--ink);background:var(--tint)}.choice input{width:auto;min-height:0;accent-color:var(--accent)}
  .feedback{margin:0;padding:8px 12px;border-radius:8px;border:1px solid var(--line)}.feedback.bad{border-color:var(--bad);color:var(--bad)}
  .actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px 14px;color:var(--muted);font-size:.875rem}
  @media(max-width:560px){.tier{grid-template-columns:1fr}.more,.source{grid-column:1/-1}.slot{line-height:1.4;margin-top:4px}.none{grid-column:1/-1;line-height:1.5}.row{grid-template-columns:1fr}}
</style>
