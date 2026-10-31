<script lang="ts">
  import { attentionCount, type NeedCard } from '../home'
  import { worktreeStatus } from '../terminal/api'
  import Approval from '../shell/Approval.svelte'

  let { cards, onseen, onanswered }: {
    cards: NeedCard[]
    onseen?: (sessionId: string) => void | Promise<void>
    onanswered?: () => void | Promise<void>
  } = $props()

  const id = $props.id()
  const count = $derived(attentionCount(cards))

  // For a worktree session that finished ("Ready to review"), ask the machine once per (sessionId, at) how many files
  // changed in the worktree (R11).
  let changedByKey = $state<Record<string, number>>({})
  const asked = new Set<string>()
  $effect(() => {
    for (const card of cards) {
      if (card.kind !== 'review' || !card.worktreeTarget) continue
      const key = `${card.sessionId}:${card.at}`
      if (asked.has(key)) continue
      asked.add(key)
      const target = card.worktreeTarget
      void worktreeStatus(target).then(status => {
        if (typeof status?.changed === 'number') {
          changedByKey = { ...changedByKey, [key]: status.changed }
        }
      }).catch(() => { /* keep project · machine alone when the machine cannot answer */ })
    }
  })

  function metaLine(card: NeedCard): string {
    const key = `${card.sessionId}:${card.at}`
    const changed = changedByKey[key]
    if (card.kind === 'review' && typeof changed === 'number') {
      const files = `${changed} ${changed === 1 ? 'file' : 'files'} changed`
      return card.where ? `${files} · ${card.where}` : files
    }
    return card.where
  }
</script>

<section id="needs-you" class="needs" aria-labelledby={`${id}-title`}>
  <header>
    <h2 id={`${id}-title`}>Needs you</h2>
    <span class="count" class:waiting={count > 0}>{count}</span>
  </header>
  {#if count === 0}
    <p class="none">Nothing needs you.</p>
  {:else}
    <ul class="list">
      {#each cards as card (card.sessionId)}
        <li class="card {card.tone}">
          <span class="kind {card.tone}">{card.label} · {card.age}</span>
          <span class="title">{card.title}</span>
          {#if metaLine(card)}<span class="meta">{metaLine(card)}</span>{/if}
          {#if card.approval}<Approval sessionId={card.sessionId} approval={card.approval} {onanswered} />{/if}
          {#if card.question}
            <p class="ask">{card.question.message}{#if card.question.queued > 0}<span class="more"> · {card.question.queued} more waiting</span>{/if}</p>
          {/if}
          <div class="actions">
            <a class="btn open" href={card.href} onclick={() => void onseen?.(card.sessionId)}>{card.question && !card.approval ? 'Answer in chat' : 'Open'}</a>
            {#if card.markSeen}
              <button type="button" class="btn seen" onclick={() => void onseen?.(card.sessionId)}>Mark seen</button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .needs { min-width: 0; display: flex; flex-direction: column; gap: 8px; }
  header { display: flex; align-items: center; gap: 8px; min-height: 32px; }
  h2 { margin: 0; font: 600 20px/1.3 var(--serif); }
  .count { font-size: 12px; font-weight: 600; border: 1px solid var(--line); border-radius: 99px; padding: 1px 9px; color: var(--muted); }
  .count.waiting { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); }
  .none {
    margin: 0; min-height: 133px; padding: 13px 15px; box-sizing: border-box;
    border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--muted);
  }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .card {
    min-height: 133px; padding: 13px 15px; box-sizing: border-box;
    border: 1px solid var(--line); border-radius: 13px; background: var(--card);
    display: flex; flex-direction: column; gap: 6px;
  }
  .card.warn { border-color: color-mix(in srgb, var(--warn) 45%, var(--line)); }
  .card.bad { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .kind { font-size: 12px; font-weight: 600; color: var(--muted); }
  .kind.warn { color: var(--warn); }
  .kind.bad { color: var(--bad); }
  .kind.good { color: var(--good); }
  .title { font-size: 14px; font-weight: 500; color: var(--ink); overflow-wrap: anywhere; }
  .meta { font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
  .ask { margin: 0; font-size: 13px; line-height: 1.45; color: var(--ink); overflow-wrap: anywhere; white-space: pre-wrap; max-height: 6.5em; overflow: hidden; }
  .ask .more { color: var(--muted); }
  .actions { display: flex; align-items: center; gap: 8px; margin-top: 2px; }
  .btn {
    display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 0 12px;
    border-radius: 8px; font: 500 13px/1 var(--sans); text-decoration: none; cursor: pointer;
  }
  .btn.open { border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); }
  .btn.seen { border: 1px solid var(--line-strong, var(--line)); background: transparent; color: var(--ink); }
  .btn.seen:hover { background: var(--card-raised); }

  @media (pointer: coarse), (max-width: 1099px) {
    .none, .card { min-height: 145px; }
    .btn { min-height: 48px; flex: 1 1 0; }
  }
  @media (max-width: 1099px) {
    h2 { font-size: 21px; }
  }
</style>
