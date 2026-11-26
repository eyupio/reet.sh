<script module lang="ts">
  import { stageTitle, type Pathway, type PathwayStage } from '../pathways'
  export { stageLine }
</script>

<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { listGafferPathways } from '../../api'

  // Which pathway new work follows, or the standard stages ('') the dialog lets a person choose themselves. Only enabled
  // pathways are offered; one that cannot start now is shown with why, and cannot be chosen. With none, nothing is shown.
  // A long list shows the first six (or the chosen one and five more), with the rest behind Show all and, past twelve, a
  // search. Edit pathway opens another tab, so while this is open the list is read again whenever the window comes back:
  // a choice no longer offered goes back to Standard stages, and says so.
  let { pathways = $bindable([]), value = $bindable(''), suggested }: { pathways: Pathway[]; value: string; suggested?: { id: string; why: string } } = $props()
  const id = $props.id()
  const FOLDED = 6, SEARCH_FROM = 12
  const offered = $derived(pathways.filter(p => p.enabled))
  let open = $state(false), query = $state(''), said = $state(''), lifted = $state('')
  let search = $state<HTMLInputElement>()
  const chosen = $derived(offered.find(p => p.id === value))
  const searching = $derived(open && offered.length > SEARCH_FROM)
  const rows = $derived.by(() => {
    if (open) {
      const q = searching ? query.trim().toLowerCase() : ''
      return q ? offered.filter(p => p === chosen || p.name.toLowerCase().includes(q) || p.description.toLowerCase().includes(q)) : offered
    }
    const first = offered.slice(0, FOLDED)
    // A pathway lifted into the folded list stays there, so rows do not shift when another is chosen.
    const lead = chosen && !first.includes(chosen) ? chosen : offered.find(p => p.id === lifted)
    return lead && !first.includes(lead) ? [lead, ...offered.slice(0, FOLDED - 1)] : first
  })
  $effect(() => { if (!open && chosen && !offered.slice(0, FOLDED).includes(chosen)) lifted = chosen.id })

  async function unfold() {
    open = !open; query = ''; lifted = ''
    if (open) { await tick(); search?.focus() }
  }

  onMount(() => {
    const life = new AbortController()
    let reads = 0
    const again = () => {
      if (document.visibilityState !== 'visible') return
      const mine = ++reads
      void listGafferPathways(life.signal).then(found => {
        if (mine !== reads) return
        const was = pathways.find(p => p.id === value)
        pathways = found
        const now = found.find(p => p.id === value)
        if (!value || (now?.enabled && !now.problems?.length)) return
        const name = was?.name ?? now?.name ?? 'That pathway'
        said = now?.enabled ? `${name} cannot start work now, so this is back on Standard stages.` : `${name} is no longer on, so this is back on Standard stages.`
        value = ''
      }).catch(() => { /* the list read before stays */ })
    }
    addEventListener('focus', again)
    document.addEventListener('visibilitychange', again)
    return () => { removeEventListener('focus', again); document.removeEventListener('visibilitychange', again); life.abort() }
  })
</script>

<!-- A pathway's stages in order, read only. Stages of the person's own stand out. -->
{#snippet stageLine(name: string, stages: PathwayStage[])}
  <span class="line" role="list" aria-label={`Stages of ${name}`}>
    {#each stages as stage, i (i)}<span class="step" role="listitem"><span class="chip" class:own={stage.kind === 'custom'}>{stageTitle(stage)}</span>{#if i < stages.length - 1}<span class="sep" aria-hidden="true">›</span>{/if}</span>{/each}
  </span>
{/snippet}

{#if offered.length || said}
  <div class="pathways">
    <span class="label" id={`${id}-label`}>Pathway</span>
    {#if searching}<input class="search" type="search" bind:this={search} bind:value={query} aria-label="Search pathways" placeholder="Search pathways" />{/if}
    <div class="rows" role="radiogroup" aria-labelledby={`${id}-label`}>
      {#each rows as p (p.id)}{@const problem = p.problems?.[0]}
        <div class="row" class:on={value === p.id} class:off={!!problem}>
          <label class="pick">
            <input type="radio" name={id} value={p.id} bind:group={value} disabled={!!problem} onchange={() => { said = '' }}/>
            <span class="text">
              <strong>{p.name}</strong>
              {#if suggested?.id === p.id}<span class="why">Gaffer suggests this: {suggested.why}</span>{/if}
              {@render stageLine(p.name, p.stages)}
              {#if problem}<span class="problem"><span aria-hidden="true">!</span> {problem}</span>{/if}
            </span>
          </label>
          <!-- Beside the label, not in it, so it is not read as part of the radio's name. -->
          {#if value === p.id}<a class="edit" href={`/factory/pathways/${encodeURIComponent(p.id)}`} target="_blank" rel="noopener">Edit pathway</a>{/if}
        </div>
      {/each}
      {#if open && !rows.length}<p class="none">No pathway matches “{query.trim()}”.</p>{/if}
      <div class="row" class:on={value === ''}>
        <label class="pick">
          <input type="radio" name={id} value="" bind:group={value} onchange={() => { said = '' }}/>
          <span class="text"><strong>Standard stages</strong><span class="why">Choose the stages, tiers and parts yourself, as now.</span></span>
        </label>
      </div>
    </div>
    {#if offered.length > FOLDED}<button type="button" class="more" aria-expanded={open} onclick={unfold}>{open ? 'Show fewer' : `Show all ${offered.length} pathways`}</button>{/if}
    <p class="said" role="status">{said}</p>
  </div>
{/if}

<style>
  .pathways { display: grid; gap: 6px; min-width: 0; }
  .label { font-size: 14px; color: var(--muted); }
  .search { width: 100%; box-sizing: border-box; font-size: 15px; }
  .rows { display: grid; gap: 8px; min-width: 0; }
  .row { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: start; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); font-weight: 400; min-width: 0; }
  .pick { display: grid; grid-template-columns: 20px minmax(0, 1fr); gap: 10px; align-items: start; padding: 10px 12px; font-weight: 400; cursor: pointer; min-width: 0; }
  .row.on { border-color: var(--accent); background: var(--accent-wash); }
  .row.off .pick { cursor: not-allowed; }
  .row.off strong, .row.off .line { opacity: 0.6; }
  .pick input { width: 18px; height: 18px; min-height: 0; margin: 2px 0 0; padding: 0; accent-color: var(--accent); }
  .row:has(input:focus-visible) { outline: 2px solid var(--focus); outline-offset: 2px; }
  .text { display: grid; gap: 4px; min-width: 0; }
  .text strong { font-weight: 600; color: var(--ink); overflow-wrap: anywhere; }
  .why { font-size: 14px; color: var(--muted); }
  .problem { font-size: 14px; color: var(--warn); overflow-wrap: anywhere; }
  .problem span { font-weight: 700; }
  .line { display: flex; flex-wrap: wrap; align-items: center; row-gap: 6px; min-width: 0; }
  .step { display: flex; align-items: center; }
  .sep { padding: 0 6px; color: var(--faint); }
  .chip { padding: 1px 10px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--ink-soft); font-size: 14px; font-weight: 400; line-height: 20px; white-space: nowrap; }
  .chip.own { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); background: var(--accent-wash); color: var(--accent-text); }
  /* In the chosen row, across from its name; on a touch screen it reaches 44 px without making the row taller. */
  .edit { display: inline-flex; align-items: center; min-height: 24px; margin: 10px 12px 0 0; padding: 0 2px; border-radius: 4px; font-size: 14px; font-weight: 600; white-space: nowrap; }
  .more { justify-self: start; min-height: 36px; padding: 0 2px; border: 0; background: none; color: var(--accent-text); font-size: 14px; font-weight: 600; cursor: pointer; }
  .more:hover, .edit:hover { text-decoration: underline; }
  .none { margin: 0; padding: 4px 2px; color: var(--muted); font-size: 14px; }
  .said { margin: 0; color: var(--ink-soft); font-size: 14px; }
  /* Kept in the page while empty, so what it later says is read out; it takes no room until then. */
  .said:empty { margin-top: -6px; }
  @media (pointer: coarse) {
    .edit { min-height: 44px; margin: 0 12px 0 0; }
    .more { min-height: 44px; }
  }
</style>
