<script lang="ts">
  import { short, type NeedChoice } from '../gafferOutcome'
  import type { NeedCard } from '../home'
  import Approval from '../shell/Approval.svelte'
  import StagePipeline from './StagePipeline.svelte'
  import type { FactoryItem } from './factoryList'

  // One pipeline in the Factory list: what it is, where it stands and, when it waits on the person, the one thing most
  // likely wanted as a filled button. `compact` is the list beside an open detail: the whole item is one button. `row` is
  // one line of a list on a wide screen (chip, text, stages, buttons in columns that line up from row to row); a row
  // narrower than ROW_MIN of its own width is drawn as a card.
  // The page passes the hash-link `id` (and any other attributes) in `rest`.
  // With `detailsHref`, Details is a link to that address (Home); without it, a button that calls `ondetails` (the Factory).
  let { item, layout, pipeline, selected = false, busy = false, permission, sessionId = '', usable, onchoose, ondetails, detailsHref = '', onanswered, ...rest }: {
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
    detailsHref?: string
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
  // A worker that ran out of its plan has two ways on: the filled backup, and any other model. Both are offered here, so
  // neither needs the detail. With no backup the other model is the filled button already.
  const recover = $derived(!item.archived && filled?.id !== 'replace-worker' ? item.need?.choices.find(choice => choice.id === 'replace-worker' && usable(choice)) : undefined)
  const tone = $derived(item.section === 'waiting' ? 'you' : item.kind === 'running' ? 'good' : 'plain')
  const detailsLabel = $derived(`Details and other actions for ${item.title}`)

  const ROW_MIN = 720
  let holderWidth = $state(0)
  // Before the first measurement a row stays a row, so a wide list does not flash as cards.
  const wide = $derived(holderWidth === 0 || holderWidth >= ROW_MIN)
</script>

{#snippet head(heading: boolean)}
  {#if heading}<div class="line"><span class="chip {tone}">{item.chip}</span><h3 class="title">{item.title}</h3></div>
  {:else}<span class="line"><span class="chip {tone}">{item.chip}</span><strong class="title">{item.title}</strong></span>{/if}
  <span class="objective">For: {item.objective}</span>
{/snippet}
{#snippet stages(size: 'labelled' | 'circles' | 'dots' = pipeline)}
  <StagePipeline nodes={item.nodes} at={item.at} {size} waiting={item.section === 'waiting'} label={`Stages of ${item.title}`} />
{/snippet}

{#snippet why()}
  {#if advised}<p class="why"><strong>Gaffer recommends: {advised.label.replace(/…$/, '')}.</strong> {advised.why}</p>
  {:else if item.why}<p class="why">{item.why}</p>{/if}
{/snippet}
{#snippet approval()}
  <Approval {sessionId} approval={permission!} {onanswered} />
{/snippet}
<!-- In a row the approval sits in the text column, where its command has room; the buttons column then holds Details alone. -->
{#snippet buttons(withApproval: boolean = true)}
  {#if item.kind === 'approval' && permission && !item.archived}
    {#if withApproval}{@render approval()}{/if}
  {:else if filled}
    {#snippet fill()}<button type="button" class="btn primary" disabled={busy} aria-busy={busy ? 'true' : undefined} onclick={() => onchoose(filled)}>{filled.label}{#if advised}<span class="sr-only">{' '}Recommended</span>{/if}</button>{/snippet}
    <!-- Marked as the detail's bar marks it: a caption above the button. -->
    {#if advised}<span class="lead"><span class="rec" aria-hidden="true">Recommended</span>{@render fill()}</span>{:else}{@render fill()}{/if}
  {/if}
  <!-- A usage limit has two ways on: the filled one above, and the other with Details on a line of their own under it. -->
  {#if recover}<span class="pair"><button type="button" class="btn" disabled={busy} aria-busy={busy ? 'true' : undefined} onclick={() => onchoose(recover)}>{recover.label}</button>{@render more()}</span>{:else}{@render more()}{/if}
{/snippet}
{#snippet more()}
  {#if detailsHref}<a class="btn" href={detailsHref} aria-label={detailsLabel}><span class="long">Details and other actions</span><span class="short" aria-hidden="true">Details</span></a>
  {:else}<button type="button" class="btn" aria-label={detailsLabel} onclick={ondetails}><span class="long">Details and other actions</span><span class="short" aria-hidden="true">Details</span></button>{/if}
{/snippet}
{#snippet card(size: 'labelled' | 'circles' | 'dots')}
  <article class="item card" {...rest}>
    {@render head(true)}
    {@render stages(size)}
    {@render why()}
    <!-- An approval's command and its Allow and Deny sit above the row of buttons, so Details never squeezes them. -->
    {#if item.kind === 'approval' && permission && !item.archived}{@render approval()}{/if}
    <div class="buttons" data-floating-clearance="avoid">{@render buttons(false)}</div>
  </article>
{/snippet}

{#if layout === 'compact'}
  <!-- Named by its chip and title: the stages inside would otherwise make a long name. -->
  <button type="button" class="item compact" class:selected aria-pressed={selected} aria-label={`${item.chip}: ${item.title}`} onclick={ondetails} {...rest}>
    {@render head(false)}
    {@render stages()}
  </button>
{:else if layout === 'row'}
  <div class="holder" bind:clientWidth={holderWidth}>
    {#if wide}
      <article class="row" {...rest}>
        <span class="chipcell"><span class="chip {tone}">{item.chip}</span></span>
        <div class="text">
          <div class="what"><h3 class="title">{item.title}</h3>{" "}<span class="where" title={item.objective}>· Factory · {short(item.objective)}</span></div>
          {@render why()}
          {#if item.kind === 'approval' && permission && !item.archived}{@render approval()}{/if}
        </div>
        <div class="stagecell">{@render stages('dots')}</div>
        <div class="buttons end" data-floating-clearance="avoid">{@render buttons(false)}</div>
      </article>
    {:else}
      {@render card(pipeline === 'dots' ? 'labelled' : pipeline)}
    {/if}
  </div>
{:else}
  {@render card(pipeline)}
{/if}

<style>
  .item{display:grid;grid-template-columns:minmax(0,1fr);gap:8px;min-width:0;box-sizing:border-box;padding:14px 16px;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--ink);font:inherit;text-align:left}
  .item.compact{width:100%;cursor:pointer;padding:12px 14px}
  .item.compact:hover{border-color:var(--line-strong)}
  .item.compact.selected{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .item.compact:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .holder{container-type:inline-size;min-width:0}
  .row{display:grid;grid-template-columns:var(--col-chip,124px) minmax(0,1fr) var(--col-stages,180px) var(--col-actions,248px);grid-template-areas:'chip text stages actions';column-gap:16px;align-items:start;min-width:0;box-sizing:border-box;padding:14px 16px;color:var(--ink);text-align:left}
  .chipcell{grid-area:chip;align-self:start}
  .text{grid-area:text;display:grid;gap:4px;min-width:0}
  .stagecell{grid-area:stages;min-width:0}
  .row .buttons{grid-area:actions}
  /* Bottom-aligned, so Details sits level with a filled button that has Recommended above it. */
  .buttons.end{justify-content:flex-end;align-items:flex-end}
  .what{min-width:0;overflow-wrap:anywhere;line-height:1.4}
  .what .title{display:inline}
  .where{color:var(--muted);font-size:0.875rem}
  @container (max-width:899px){
    .row{grid-template-columns:var(--col-chip,124px) minmax(0,1fr) var(--col-actions,248px);grid-template-areas:'chip text actions' 'chip stages actions';row-gap:8px}
    .stagecell{max-width:var(--col-stages,180px)}
  }
  .line{display:flex;flex-wrap:wrap;align-items:center;gap:4px 10px;min-width:0}
  .chip{flex:none;padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:600;white-space:nowrap}
  .chip.you{border-color:var(--accent);color:var(--accent-text)}
  .chip.good{border-color:var(--good);color:var(--good)}
  .title{min-width:0;margin:0;font:inherit;font-weight:700;overflow-wrap:anywhere}
  .objective,.why{min-width:0;margin:0;color:var(--muted);font-size:0.875rem;line-height:1.4;overflow-wrap:anywhere}
  .objective{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;overflow:hidden}
  .why{color:var(--ink)}
  .buttons{display:flex;flex-wrap:wrap;align-items:flex-end;gap:8px}
  .buttons .btn{min-height:44px;max-width:100%}
  a.btn{display:inline-flex;align-items:center;text-decoration:none}
  .pair{display:flex;gap:8px;flex:1 0 100%;min-width:0}.pair .btn{flex:1 1 auto;min-width:0}
  .lead{display:grid;gap:2px;min-width:0}
  .rec{font-size:0.875rem;font-weight:600;color:var(--accent-text);line-height:1.3}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  .short{display:none}
  /* A row's buttons column holds the filled button and Details side by side, so Details is its short name there. */
  .row .long{display:none}.row .short{display:inline}
  @media (max-width:560px){.long{display:none}.short{display:inline}}
  @media (pointer: coarse){.item.compact{min-height:44px}}
</style>
