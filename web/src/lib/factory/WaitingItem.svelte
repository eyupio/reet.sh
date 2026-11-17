<script lang="ts">
  import type { NeedChoice } from '../gafferOutcome'
  import type { NeedCard } from '../home'
  import Approval from '../shell/Approval.svelte'
  import StagePipeline from './StagePipeline.svelte'
  import type { FactoryItem } from './factoryList'

  // One pipeline in the Factory list: what it is, where it stands and, when it waits on the person, the one thing most
  // likely wanted as a filled button. `compact` is the list beside an open detail: the whole item is one button.
  // The page passes the hash-link `id` (and any other attributes) in `rest`.
  let { item, layout, pipeline, selected = false, busy = false, permission, sessionId = '', usable, onchoose, ondetails, onanswered, ...rest }: {
    item: FactoryItem
    layout: 'card' | 'row' | 'compact'
    pipeline: 'labelled' | 'circles' | 'dots'
    selected?: boolean
    busy?: boolean
    permission?: NonNullable<NeedCard['approval']>
    sessionId?: string
    usable: (choice: NeedChoice) => boolean
    onchoose: (choice: NeedChoice) => void
    ondetails: () => void
    onanswered?: () => void | Promise<void>
    [key: string]: unknown
  } = $props()

  // An archived outcome's pipeline offers nothing but its Details, whatever it was doing when it was put away.
  const filled = $derived.by((): NeedChoice | undefined => {
    if (item.archived) return undefined
    if (item.kind === 'failed') return { id: 'restart', label: 'Try again' }
    // The filled button is the choice marked primary, without its trailing "…": the dialog that choice opens still asks first.
    const primary = item.need?.choices.find(choice => choice.primary && usable(choice))
    return primary && { ...primary, label: primary.label.replace(/…$/, '') }
  })
  // With Gaffer's advice the card says it in the detail's words, and marks the filled button as the detail's bar does.
  const advised = $derived(!item.archived && item.need?.recommend?.by === 'gaffer' ? item.need.recommend : undefined)
  const tone = $derived(item.section === 'waiting' ? 'you' : item.kind === 'running' ? 'good' : 'plain')
  const detailsLabel = $derived(`Details and other actions for ${item.title}`)
</script>

{#snippet head(heading: boolean)}
  {#if heading}<div class="line"><span class="chip {tone}">{item.chip}</span><h3 class="title">{item.title}</h3></div>
  {:else}<span class="line"><span class="chip {tone}">{item.chip}</span><strong class="title">{item.title}</strong></span>{/if}
  <span class="objective">For: {item.objective}</span>
{/snippet}
{#snippet stages()}
  <StagePipeline nodes={item.nodes} at={item.at} size={pipeline} waiting={item.section === 'waiting'} label={`Stages of ${item.title}`} />
{/snippet}

{#if layout === 'compact'}
  <!-- Named by its chip and title: the stages inside would otherwise make a long name. -->
  <button type="button" class="item compact" class:selected aria-pressed={selected} aria-label={`${item.chip}: ${item.title}`} onclick={ondetails} {...rest}>
    {@render head(false)}
    {@render stages()}
  </button>
{:else}
  <article class="item card" {...rest}>
    {@render head(true)}
    {@render stages()}
    {#if advised}<p class="why"><strong>Gaffer recommends: {advised.label.replace(/…$/, '')}.</strong> {advised.why}</p>
    {:else if item.why}<p class="why">{item.why}</p>{/if}
    <div class="buttons">
      {#if item.kind === 'approval' && permission && !item.archived}
        <Approval {sessionId} approval={permission} {onanswered} />
      {:else if filled}
        {#snippet fill()}<button type="button" class="btn primary" disabled={busy} aria-busy={busy ? 'true' : undefined} onclick={() => onchoose(filled)}>{filled.label}{#if advised}<span class="sr-only"> Recommended</span>{/if}</button>{/snippet}
        <!-- Marked as the detail's bar marks it: a caption above the button. -->
        {#if advised}<span class="lead"><span class="rec" aria-hidden="true">Recommended</span>{@render fill()}</span>{:else}{@render fill()}{/if}
      {/if}
      <button type="button" class="btn" aria-label={detailsLabel} onclick={ondetails}><span class="long">Details and other actions</span><span class="short" aria-hidden="true">Details</span></button>
    </div>
  </article>
{/if}

<style>
  .item{display:grid;grid-template-columns:minmax(0,1fr);gap:8px;min-width:0;box-sizing:border-box;padding:14px 16px;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--ink);font:inherit;text-align:left}
  .item.compact{width:100%;cursor:pointer;padding:12px 14px}
  .item.compact:hover{border-color:var(--line-strong)}
  .item.compact.selected{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .item.compact:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .line{display:flex;flex-wrap:wrap;align-items:center;gap:4px 10px;min-width:0}
  .chip{flex:none;padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:600;white-space:nowrap}
  .chip.you{border-color:var(--accent);color:var(--accent-text)}
  .chip.good{border-color:var(--good);color:var(--good)}
  .title{min-width:0;margin:0;font:inherit;font-weight:700;overflow-wrap:anywhere}
  .objective,.why{min-width:0;margin:0;color:var(--muted);font-size:0.875rem;line-height:1.4;overflow-wrap:anywhere}
  .objective{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;overflow:hidden}
  .why{color:var(--ink)}
  .buttons{display:flex;flex-wrap:wrap;align-items:flex-end;gap:8px}
  .buttons .btn{min-height:44px}
  .lead{display:grid;gap:2px;min-width:0}
  .rec{font-size:0.875rem;font-weight:600;color:var(--accent-text);line-height:1.3}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  .short{display:none}
  @media (max-width:560px){.long{display:none}.short{display:inline}}
  @media (pointer: coarse){.item.compact{min-height:44px}}
</style>
