<script lang="ts">
  import { untrack } from 'svelte'
  import type { NeedCard } from '../home'
  import Approval from '../shell/Approval.svelte'
  import { worktreeStatus } from '../terminal/api'
  import type { GoalNeed } from './homeWaiting'

  // One session waiting for the person, in Home's "Waiting for you" list: the same parts as a Factory item (chip, title,
  // where, one sentence, buttons). `row` is one line on a wide screen, in the columns WaitingItem uses (the stages
  // column stays empty), and a row narrower than ROW_MIN of its own width is drawn as a card. Given a `goal` instead of a
  // `card`, it is a Gaffer goal that needs review on a session: chip Gaffer, Gaffer's words, and Open.
  let { card: given, goal, desk = '', layout, onseen, onanswered }: {
    card?: NeedCard
    goal?: GoalNeed
    /** The name of the desk the session is on: its place then says so, and its link (already) opens the desk. */
    desk?: string
    layout: 'card' | 'row'
    onseen?: (sessionId: string) => void | Promise<void>
    onanswered?: () => void | Promise<void>
  } = $props()

  // A goal is drawn as a card that asks nothing of its own: the parts below read the card, and the goal's own words
  // replace the sentence.
  const card = $derived<NeedCard>(given ?? { kind: 'waiting', reasons: [], label: '', tone: 'warn', sessionId: goal?.sessionId ?? '', agentId: '', title: goal?.title ?? '', where: goal?.where ?? '', href: goal?.href ?? '', at: goal?.since ?? 0, age: '', markSeen: false, worktreeTarget: null })
  const hasQuestion = $derived(!!card.question || card.reasons.includes('question'))
  const hasApproval = $derived(!!card.approval || card.reasons.includes('approval'))
  const asking = $derived(!!card.question && !card.approval)
  const chip = $derived(goal ? 'Gaffer' : card.kind === 'failed' ? 'Failed' : card.kind === 'review' ? 'To review' : card.kind === 'finished' ? 'Finished' : card.kind === 'stopped' ? 'Stopped waiting' : hasApproval ? 'Approval' : hasQuestion ? 'Question' : 'Waiting')
  // Only the kinds that ask something of the person use the accent tone.
  const tone = $derived(card.kind === 'failed' ? 'bad' : goal || hasApproval || hasQuestion ? 'you' : 'plain')

  // For a worktree session that finished, ask the machine once per (session, finish time) how many files changed. The
  // effect depends on that key alone: the target is a fresh object on every poll, so it is read untracked, and the shown
  // count stays while the key is unchanged.
  const worktreeKey = $derived(card.kind === 'review' && card.worktreeTarget ? `${card.sessionId}:${card.at}` : '')
  let changed = $state<number>()
  $effect(() => {
    const key = worktreeKey
    changed = undefined
    const target = key ? untrack(() => card.worktreeTarget) : null
    if (!target) return
    let current = true
    void worktreeStatus(target).then(status => {
      if (current && typeof status?.changed === 'number') changed = status.changed
    }).catch(() => { /* keep the place alone when the machine cannot answer */ })
    return () => { current = false }
  })
  const place = $derived.by(() => {
    if (typeof changed !== 'number') return card.where
    const files = `${changed} ${changed === 1 ? 'file' : 'files'} changed`
    return card.where ? `${files} · ${card.where}` : files
  })
  const where = $derived([place, desk ? `Desk ${desk}` : ''].filter(Boolean).join(' · '))

  const ROW_MIN = 720
  let holderWidth = $state(0)
  const wide = $derived(holderWidth === 0 || holderWidth >= ROW_MIN)
</script>

{#snippet sentence()}
  {#if goal}<p class="said">Gaffer’s goal needs you.{#if goal.reason}{' '}{goal.reason}{/if}</p>
  {:else}
  {#if card.approval}<Approval sessionId={card.sessionId} approval={card.approval} {onanswered} />
  {/if}
  {#if card.question}<p class="said">{card.question.message}{#if card.question.queued > 0}<span class="more"> · {card.question.queued} more waiting</span>{/if}</p>
  {:else if !card.approval}<p class="said">{card.label} · {card.age}</p>{/if}
  {#if card.reasons.includes('gaffer')}<p class="note">Gaffer’s goal also needs you</p>{/if}
  {/if}
{/snippet}
{#snippet buttons()}
  {@const filled = !!goal || card.kind !== 'waiting' || asking}
  <a class="btn" class:primary={filled} href={card.href} aria-label={asking ? `Answer in chat: ${card.title}` : `Open ${card.title}`} onclick={() => void onseen?.(card.sessionId)}>{asking ? 'Answer in chat' : 'Open'}</a>
  {#if card.markSeen}<button type="button" class="btn" aria-label={`Mark ${card.title} as seen`} onclick={() => void onseen?.(card.sessionId)}>Mark seen</button>{/if}
{/snippet}

{#if layout === 'row'}
  <div class="holder" bind:clientWidth={holderWidth}>
    {#if wide}
      <article class="row">
        <span class="chipcell"><span class="chip {tone}">{chip}</span></span>
        <div class="text">
          <div class="what"><h3 class="title">{card.title}</h3>{" "}<span class="where">· Session{where ? ` · ${where}` : ''}</span></div>
          {@render sentence()}
        </div>
        <div class="buttons end" data-floating-clearance="avoid">{@render buttons()}</div>
      </article>
    {:else}
      <article class="item card">
        <div class="line"><span class="chip {tone}">{chip}</span><h3 class="title">{card.title}</h3></div>
        <span class="where">Session{where ? ` · ${where}` : ''}</span>
        {@render sentence()}
        <div class="buttons" data-floating-clearance="avoid">{@render buttons()}</div>
      </article>
    {/if}
  </div>
{:else}
  <article class="item card">
    <div class="line"><span class="chip {tone}">{chip}</span><h3 class="title">{card.title}</h3></div>
    <span class="where">Session{where ? ` · ${where}` : ''}</span>
    {@render sentence()}
    <div class="buttons" data-floating-clearance="avoid">{@render buttons()}</div>
  </article>
{/if}

<style>
  .item{display:grid;grid-template-columns:minmax(0,1fr);gap:8px;min-width:0;box-sizing:border-box;padding:14px 16px;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--ink);font:inherit;text-align:left}
  .holder{container-type:inline-size;min-width:0}
  .row{display:grid;grid-template-columns:var(--col-chip,124px) minmax(0,1fr) var(--col-stages,180px) var(--col-actions,248px);grid-template-areas:'chip text stages actions';column-gap:16px;align-items:start;min-width:0;box-sizing:border-box;padding:14px 16px;color:var(--ink);text-align:left}
  .chipcell{grid-area:chip;align-self:start}
  .text{grid-area:text;display:grid;gap:4px;min-width:0}
  .row .buttons{grid-area:actions}
  .buttons.end{justify-content:flex-end;align-items:flex-start}
  @container (max-width:899px){
    .row{grid-template-columns:var(--col-chip,124px) minmax(0,1fr) var(--col-actions,248px);grid-template-areas:'chip text actions'}
  }
  .line{display:flex;flex-wrap:wrap;align-items:center;gap:4px 10px;min-width:0}
  .chip{flex:none;padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:600;white-space:nowrap}
  .chip.you{border-color:var(--accent);color:var(--accent-text)}
  .chip.bad{border-color:var(--bad);color:var(--bad)}
  .title{min-width:0;margin:0;font:inherit;font-weight:700;overflow-wrap:anywhere}
  .what{min-width:0;overflow-wrap:anywhere;line-height:1.4}
  .what .title{display:inline}
  .where,.note{min-width:0;margin:0;color:var(--muted);font-size:0.875rem;line-height:1.4;overflow-wrap:anywhere}
  .said{min-width:0;margin:0;color:var(--ink);font-size:0.875rem;line-height:1.4;overflow-wrap:anywhere;white-space:pre-wrap;max-height:6.5em;overflow:hidden}
  .said .more{color:var(--muted)}
  .buttons{display:flex;flex-wrap:wrap;align-items:flex-end;gap:8px}
  .buttons .btn{min-height:44px;text-decoration:none}
</style>
