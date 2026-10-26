<script lang="ts">
  import type { NeedCard } from '../home'

  // Needs you: newest first, with the count. Nothing that exists today says a session wants the person (an exit already shows
  // in In flight), so the list is empty and says so plainly; the cards arrive with PR G and decision cards with Program 2.
  let { cards }: { cards: NeedCard[] } = $props()
  const id = $props.id()
</script>

<section class="needs" aria-labelledby={`${id}-title`}>
  <header>
    <h2 id={`${id}-title`}>Needs you</h2>
    <span class="count" class:waiting={cards.length > 0}>{cards.length}</span>
  </header>
  {#if cards.length === 0}
    <p class="none">Nothing needs you.</p>
  {:else}
    <ul>
      {#each cards as c (c.sessionId + c.kind)}
        <li class="card"><span class="kind">{c.kind}</span></li>
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
  .none { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--muted); }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .card { padding: 13px 15px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); }
  .kind { font-size: 12px; font-weight: 600; }
  @media (max-width: 1099px) { h2 { font-size: 21px; } }
</style>
