<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { listEvents, UnauthorizedError, type Event } from './api'
  import { eventLabel, eventTitle } from './events'
  import { elapsed } from './time'

  // /activity: the account activity list (what was changed lately, newest first), moved here from the account page.
  let { onsignedout }: { onsignedout?: (e: unknown) => void } = $props()
  let events = $state<Event[]>([])
  let loading = $state(true)
  let error = $state('')

  onMount(() => { void load() })
  async function load() {
    try {
      events = (await listEvents(50)).events
      error = ''
    } catch (e) {
      if (e instanceof UnauthorizedError) { onsignedout?.(e); return }
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }
</script>

<PageHeader eyebrow="Your account" title="Activity" lede="What you changed lately, newest first." />
{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
<section class="block" aria-label="Activity list">
  {#if loading}
    <p class="muted" role="status">Loading activity…</p>
  {:else if events.length === 0 && !error}
    <p class="muted">Nothing recorded yet.</p>
  {:else}
    <ul class="events">
      {#each events as event (event.id)}
        <li title={eventTitle(event)}><span>{eventLabel(event)}</span><time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time></li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--bad); border-radius: 10px; background: var(--card); }
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .muted { margin: 0; color: var(--muted); }
  .events { list-style: none; margin: 0; padding: 0; }
  .events li { display: flex; justify-content: space-between; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.84rem; }
  .events li span { min-width: 0; overflow-wrap: anywhere; }
  .events time { flex: none; }
</style>
