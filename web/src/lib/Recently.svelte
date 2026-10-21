<script lang="ts">
  import type { Event } from './api'
  import { eventLabel, eventTitle } from './events'
  import { elapsed } from './time'

  let { events }: { events: Event[] } = $props()
</script>

{#if events.length}
  <section class="recent" aria-label="Recently">
    <h2>Recently</h2>
    <ul>
      {#each events as e (e.id)}
        <li title={eventTitle(e)}>
          <span class="what">{eventLabel(e)}</span>
          <span class="when">{elapsed(e.at)}</span>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .recent { margin-top: 18px; }
  h2 {
    margin: 0 0 6px; color: var(--muted); font-size: 0.68rem; font-weight: 700;
    letter-spacing: 0.12em; text-transform: uppercase;
  }
  ul {
    display: flex; gap: 8px; list-style: none; margin: 0; padding: 0 0 4px;
    overflow-x: auto; scrollbar-width: thin;
  }
  li {
    flex: none; display: inline-flex; align-items: baseline; gap: 6px;
    padding: 4px 10px; border: 1px solid var(--line); border-radius: 999px;
    background: var(--card); color: var(--muted); font-size: 0.8rem;
  }
  .what { color: var(--ink); }
  .when { font-size: 0.73rem; }
</style>
