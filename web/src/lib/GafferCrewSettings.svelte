<script lang="ts">
  // The account's crew, in Gaffer settings: who does a heavy, a standard and a light stage, each tier's backup, when a
  // backup takes over, and whether the outcome dialog starts on "Gaffer decides". Every outcome starts from these and can
  // still be changed when it is approved. Model and effort are typed here, since no one machine is in view to list them.
  import { onMount } from 'svelte'
  import { getGafferCrew, saveGafferCrew, type GafferCrew, type GafferCrewTier } from './api'
  import { BALANCE_LOW_PERCENT, BALANCE_MAX_PERCENT, BALANCE_MIN_PERCENT, CREW_TIERS, balancePercent } from './gafferOutcome'
  import { chatToolName } from './terminal/api'

  const AGENTS = ['claude', 'codex', 'opencode', 'cline', 'antigravity']
  type Row = { tool: string; model: string; effort: string }
  type Tier = 'heavy' | 'standard' | 'light'
  const blank = (): Row => ({ tool: '', model: '', effort: '' })
  let first = $state<Record<Tier, Row>>({ heavy: blank(), standard: blank(), light: blank() })
  let spare = $state<Record<Tier, Row>>({ heavy: blank(), standard: blank(), light: blank() })
  // A Cline tier's billing source is chosen on a machine, in the outcome dialog; it is kept as it was saved.
  let sources: Partial<Record<Tier, string>> = {}
  let below = $state<number | string>(BALANCE_LOW_PERCENT), decides = $state(false)
  let loading = $state(true), busy = $state(false), error = $state(''), notice = $state(''), savedAt = $state(0)
  const formId = $props.id()

  function show(crew: GafferCrew) {
    for (const item of CREW_TIERS) {
      const member: GafferCrewTier = crew[item.tier] ?? {}
      first[item.tier] = { tool: member.tool ?? '', model: member.model ?? '', effort: member.effort ?? '' }
      spare[item.tier] = { tool: member.backup?.tool ?? '', model: member.backup?.model ?? '', effort: member.backup?.effort ?? '' }
      sources[item.tier] = member.provider
    }
    below = balancePercent(crew.balanceBelow); decides = !!crew.gafferDecides; savedAt = crew.updatedAt ?? 0
  }
  onMount(async () => {
    try { show((await getGafferCrew()).crew) } catch (e) { error = e instanceof Error ? e.message : 'Your crew could not be read.' }
    loading = false
  })
  const tier = (name: Tier): GafferCrewTier => {
    const one = first[name], two = spare[name]
    if (!one.tool) return {}
    return { tool: one.tool, model: one.model.trim(), effort: one.effort.trim(), ...(one.tool === 'cline' && sources[name] ? { provider: sources[name] } : {}), ...(two.tool ? { backup: { tool: two.tool, model: two.model.trim(), effort: two.effort.trim() } } : {}) }
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

<section class="crew" aria-label="Your Factory crew">
  <h3>Your Factory crew</h3>
  <p class="note">Who does each kind of Factory stage. Gaffer says how demanding a stage is; this is what that means for you. Every outcome starts from here and you can still change it when you approve one.</p>
  {#if loading}<p role="status">Reading your crew…</p>
  {:else}
  <form onsubmit={event => { event.preventDefault(); void save() }}>
    <div class="tiers">
      {#each CREW_TIERS as item (item.tier)}<div class="tier" role="group" aria-label={`${item.label} tier`}>
        <div class="tier-head"><strong>{item.label}</strong><span>{item.does}</span></div>
        <span class="slot">First choice</span>
        <select aria-label={`${item.label} agent`} bind:value={first[item.tier].tool} disabled={busy} onchange={() => { if (!first[item.tier].tool) { first[item.tier] = blank(); spare[item.tier] = blank() } }}><option value="">The outcome’s own agent</option>{#each AGENTS as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
        {#if first[item.tier].tool}
          <input aria-label={`${item.label} model`} placeholder="Agent’s default model" bind:value={first[item.tier].model} disabled={busy} maxlength="200" autocapitalize="off" spellcheck="false"/>
          <input aria-label={`${item.label} effort`} placeholder="Default effort" bind:value={first[item.tier].effort} disabled={busy} maxlength="40" autocapitalize="off" spellcheck="false"/>
          <span class="slot">Backup</span>
          <select aria-label={`${item.label} backup agent`} bind:value={spare[item.tier].tool} disabled={busy} onchange={() => { if (!spare[item.tier].tool) spare[item.tier] = blank() }}><option value="">No backup</option>{#each AGENTS as id (id)}<option value={id}>{chatToolName(id)}</option>{/each}</select>
          {#if spare[item.tier].tool}
            <input aria-label={`${item.label} backup model`} placeholder="Agent’s default model" bind:value={spare[item.tier].model} disabled={busy} maxlength="200" autocapitalize="off" spellcheck="false"/>
            <input aria-label={`${item.label} backup effort`} placeholder="Default effort" bind:value={spare[item.tier].effort} disabled={busy} maxlength="40" autocapitalize="off" spellcheck="false"/>
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
  select,input{width:100%;min-width:0;min-height:44px;font:inherit}
  .row{display:grid;grid-template-columns:minmax(0,auto) minmax(0,1fr);align-items:center;gap:6px 12px;margin:0;padding:12px 0 0;border:0;border-top:1px solid var(--line);min-width:0}.row label,.who-name{font-weight:600}.who{grid-template-columns:1fr}
  .percent{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px}.percent input{width:5.5rem;text-align:right;font-variant-numeric:tabular-nums}
  .choice{display:inline-flex;justify-self:start;border:1px solid var(--line);border-radius:10px;overflow:hidden}.choice label{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 16px;font-weight:600;cursor:pointer;color:var(--muted)}.choice label+label{border-left:1px solid var(--line)}.choice label.on{color:var(--ink);background:var(--tint)}.choice input{width:auto;min-height:0;accent-color:var(--accent)}
  .feedback{margin:0;padding:8px 12px;border-radius:8px;border:1px solid var(--line)}.feedback.bad{border-color:var(--bad);color:var(--bad)}
  .actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px 14px;color:var(--muted);font-size:.875rem}
  @media(max-width:560px){.tier{grid-template-columns:1fr}.slot{line-height:1.4;margin-top:4px}.none{grid-column:1/-1;line-height:1.5}.row{grid-template-columns:1fr}}
</style>
