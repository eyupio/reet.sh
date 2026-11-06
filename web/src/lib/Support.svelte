<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import { getGroups, listTickets, openTicket, type Group, type Ticket, type TicketStatus } from './api'
  import CollectionPanel from './CollectionPanel.svelte'
  import PageHeader from './PageHeader.svelte'
  import { elapsed } from './time'

  let tickets = $state<Ticket[]>([])
  let groups = $state<Group[]>([])
  let loading = $state(true)
  let busy = $state(false)
  let loadError = $state('')
  let error = $state('')
  let status = $state<'all' | TicketStatus>('all')
  let search = $state('')
  let form = $state({ subject: '', body: '', groupId: 0 })

  const labels: Record<TicketStatus, string> = { open: 'With support', waiting: 'Waiting on you', closed: 'Closed' }
  const searched = $derived(tickets.filter((t) => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean)
    return words.every((word) => `${t.subject} ${t.opener} ${t.groupName ?? ''} ${t.id}`.toLowerCase().includes(word))
  }))
  const counts = $derived({
    all: searched.length,
    open: searched.filter((t) => t.status === 'open').length,
    waiting: searched.filter((t) => t.status === 'waiting').length,
    closed: searched.filter((t) => t.status === 'closed').length,
  })
  const visible = $derived(searched.filter((t) => status === 'all' || t.status === status))
  const countLabel = $derived(loading ? 'Reading tickets…' : loadError ? 'Tickets unavailable' : visible.length === tickets.length ? `${tickets.length} ${tickets.length === 1 ? 'ticket' : 'tickets'}` : `${visible.length} of ${tickets.length} tickets`)

  async function load() {
    loading = true
    loadError = ''
    try {
      const [t, g] = await Promise.all([listTickets(), getGroups()])
      tickets = t.tickets
      groups = g.groups
      error = ''
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  async function open(e: SubmitEvent) {
    e.preventDefault()
    if (busy || !form.subject.trim() || !form.body.trim()) return
    busy = true
    error = ''
    try {
      const r = await openTicket({ subject: form.subject.trim(), body: form.body.trim(), groupId: form.groupId || undefined })
      form = { subject: '', body: '', groupId: 0 }
      await load()
      location.href = `/support/${r.ticket.id}`
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  function clearFilters() {
    search = ''
    status = 'all'
  }

  onMount(() => { void load() })
</script>

<svelte:head><title>Support · reet.sh</title></svelte:head>

<PageHeader eyebrow="Support" title="Tell us what's not working." lede="Open a ticket for yourself or a team you belong to. A team's owner and managers can read and answer its tickets; instance administrators can see every ticket." />

{#if error && !loadError}<p class="notice bad" role="alert">{error}</p>{/if}

<div class="support">
  <section class="card" aria-labelledby="open-title">
    <h2 id="open-title">Open a ticket</h2>
    <form onsubmit={open} aria-busy={busy}>
      <label>Subject
        <input bind:value={form.subject} maxlength="200" placeholder="A short summary of the issue" required />
      </label>
      <label>About
        <select bind:value={form.groupId}>
          <option value={0}>Just me</option>
          {#each groups as g (g.id)}<option value={g.id}>{g.name} · {g.kind === 'organisation' ? 'organisation' : 'team'}</option>{/each}
        </select>
      </label>
      <label>What is happening
        <textarea bind:value={form.body} rows="7" maxlength="20000" placeholder="What happened, what you expected, and what you have tried…" required></textarea>
      </label>
      <button type="submit" class="btn primary" disabled={busy || !form.subject.trim() || !form.body.trim()} aria-busy={busy}>{busy ? 'Opening…' : 'Open a ticket'}</button>
    </form>
  </section>

  <CollectionPanel label="Tickets you can see" count={countLabel}>
    {#snippet actions()}
      <button type="button" class="btn quiet" onclick={load} disabled={loading}>Refresh</button>
    {/snippet}
    {#snippet toolbar()}
      <div class="ticket-toolbar">
        <label class="search">Search tickets
          <input type="search" bind:value={search} placeholder="Subject, opener or team" />
        </label>
        <div class="scopes" role="group" aria-label="Show tickets">
          {#each [['all', 'All'], ['open', 'With support'], ['waiting', 'Waiting on you'], ['closed', 'Closed']] as [key, label]}
            <button type="button" class="scope" aria-pressed={status === key} onclick={() => (status = key as typeof status)}>{label}<small>{counts[key as keyof typeof counts]}</small></button>
          {/each}
        </div>
      </div>
    {/snippet}

    {#if loading}
      <Skeleton label="Reading tickets…" shape="rows" count={5} />
    {:else if loadError}
      <div class="state-block error" role="alert">
        <p>{loadError}</p>
        <button type="button" class="btn" onclick={load}>Try again</button>
      </div>
    {:else if visible.length === 0}
      <div class="state-block">
        <h2>{tickets.length === 0 ? 'No tickets yet.' : 'No ticket matches that search or status.'}</h2>
        <p>{tickets.length === 0 ? 'Open one on the left and it appears here with every reply.' : 'Try another search, or clear the filters.'}</p>
        {#if tickets.length > 0}<button type="button" class="btn" onclick={clearFilters}>Clear filters</button>{/if}
      </div>
    {:else}
      <div class="ticket-list">
        {#each visible as t (t.id)}
          <a class="ticket" href={`/support/${t.id}`}>
            <span class="main">
              <strong>{t.subject}</strong>
              <small>{t.opener}{t.groupName ? ` · ${t.groupName}` : ' · personal'} · {t.messages} {t.messages === 1 ? 'message' : 'messages'} · {elapsed(t.updatedAt)}</small>
            </span>
            <span class="state {t.status}">{labels[t.status]}</span>
            <span class="go" aria-hidden="true"></span>
          </a>
        {/each}
      </div>
    {/if}
  </CollectionPanel>
</div>


<style>
  .support { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr); gap: 20px; margin-top: 24px; align-items: start; }
  .card { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  h2 { margin: 0; font: 500 1.4rem/1.2 var(--serif); letter-spacing: -0.02em; }
  form { display: grid; gap: 12px; margin-top: 16px; }
  label { display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  input, select, textarea { width: 100%; }
  textarea { min-height: 140px; resize: vertical; }

  .ticket-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .search input { width: 100%; }
  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }

  .ticket-list { display: grid; }
  .ticket { display: grid; grid-template-columns: minmax(0, 1fr) auto 8px; align-items: center; gap: 12px; padding: 16px 2px; border-top: 1px solid var(--line-soft); color: inherit; text-decoration: none; }
  .ticket:first-child { border-top: 0; }
  .ticket:hover strong { text-decoration: underline; text-underline-offset: 2px; }
  .main { min-width: 0; display: grid; gap: 3px; }
  .main strong { overflow-wrap: anywhere; }
  .main small { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
  .state { flex: none; padding: 3px 8px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; white-space: nowrap; }
  .state.open { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent-text); }
  .state.waiting { border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); color: var(--warn); }
  .state.closed { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .go { width: 7px; height: 7px; border-top: 2px solid var(--muted); border-right: 2px solid var(--muted); transform: rotate(45deg); }

  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h2 { font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; margin: 0; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  .state-block.error p { color: var(--ink); }

  .notice { margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.875rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }

  @media (max-width: 820px) { .support { grid-template-columns: 1fr; } }
  @media (max-width: 650px) {
    .ticket-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .ticket { grid-template-columns: minmax(0, 1fr) auto; }
    .go { display: none; }
  }
  @media (pointer: coarse) { .scope, .search input { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .scope, .ticket { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
