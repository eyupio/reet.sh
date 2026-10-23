<script lang="ts">
  import { onMount } from 'svelte'
  import { getGroups, listTickets, openTicket, type Group, type Ticket, type TicketStatus } from './api'
  import PageHeader from './PageHeader.svelte'
  import { elapsed } from './time'

  let tickets = $state<Ticket[]>([])
  let groups = $state<Group[]>([])
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let status = $state<'all' | TicketStatus>('all')
  let search = $state('')
  let form = $state({ subject: '', body: '', groupId: 0 })

  const labels: Record<TicketStatus, string> = { open: 'With support', waiting: 'Waiting on you', closed: 'Closed' }
  const visible = $derived(tickets.filter((t) => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean)
    return (status === 'all' || t.status === status) &&
      words.every((word) => `${t.subject} ${t.opener} ${t.groupName ?? ''} ${t.id}`.toLowerCase().includes(word))
  }))

  async function load() {
    loading = true
    try {
      const [t, g] = await Promise.all([listTickets(), getGroups()])
      tickets = t.tickets
      groups = g.groups
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
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

  onMount(() => { void load() })
</script>

<svelte:head><title>Support · reet.sh</title></svelte:head>

<PageHeader eyebrow="Support" title="Tell us what's not working." lede="Open a ticket for yourself or a team you belong to. A team's owner and managers can read and answer its tickets; instance administrators can see every ticket." />

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}

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
      <button type="submit" class="btn primary" disabled={busy || !form.subject.trim() || !form.body.trim()}>{busy ? 'Opening…' : 'Open a ticket'}</button>
    </form>
  </section>

  <section class="card" aria-labelledby="tickets-title">
    <div class="head">
      <h2 id="tickets-title">Tickets you can see</h2>
      <button type="button" class="btn quiet" onclick={load} disabled={loading}>Refresh</button>
    </div>
    <p class="hint">Your own tickets, and tickets of teams you own or manage.</p>
    <div class="filters" role="group" aria-label="Ticket status">
      {#each [['all', 'All'], ['open', 'With support'], ['waiting', 'Waiting on you'], ['closed', 'Closed']] as [key, label]}
        <button type="button" class="btn quiet" aria-pressed={status === key} onclick={() => (status = key as typeof status)}>{label}<small>{key === 'all' ? tickets.length : tickets.filter((t) => t.status === key).length}</small></button>
      {/each}
    </div>
    <label class="search">Search tickets
      <input type="search" bind:value={search} placeholder="Subject, opener or team" />
    </label>

    {#if loading}
      <p class="hint" role="status">Loading tickets…</p>
    {:else if visible.length === 0}
      <p class="empty">{tickets.length === 0 ? 'No tickets yet. Open one on the left and it appears here with every reply.' : 'No ticket matches that search or status.'}</p>
    {:else}
      <ul class="tickets">
        {#each visible as t (t.id)}
          <li>
            <a href={`/support/${t.id}`}>
              <span class="main">
                <strong>{t.subject}</strong>
                <small>{t.opener}{t.groupName ? ` · ${t.groupName}` : ' · personal'} · {t.messages} {t.messages === 1 ? 'message' : 'messages'} · {elapsed(t.updatedAt)}</small>
              </span>
              <span class="state {t.status}">{labels[t.status]}</span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<style>
  .support { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr); gap: 20px; margin-top: 24px; align-items: start; }
  .card { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  h2 { margin: 0; font: 500 1.4rem/1.2 var(--serif); letter-spacing: -0.02em; }
  form { display: grid; gap: 12px; margin-top: 16px; }
  label { display: grid; gap: 6px; color: var(--muted); font-size: 0.84rem; }
  input, select, textarea { width: 100%; }
  textarea { min-height: 140px; resize: vertical; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .hint, .empty { color: var(--muted); font-size: 0.84rem; line-height: 1.6; }
  .hint { margin: 6px 0 0; }
  .empty { margin: 18px 0 0; padding: 18px; border: 1px dashed var(--line); border-radius: 10px; text-align: center; font-family: var(--serif); font-size: 1.05rem; }
  .filters { display: flex; flex-wrap: wrap; gap: 2px; margin-top: 12px; }
  .filters small { margin-left: 4px; color: var(--muted); }
  .search { margin-top: 12px; }
  .tickets { list-style: none; margin: 14px 0 0; padding: 0; }
  .tickets li + li { border-top: 1px solid var(--line); }
  .tickets a { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; text-decoration: none; color: inherit; }
  .tickets a:hover strong { text-decoration: underline; text-underline-offset: 2px; }
  .main { min-width: 0; display: grid; gap: 3px; }
  .main strong { overflow-wrap: anywhere; }
  .main small { color: var(--muted); font-size: 0.76rem; overflow-wrap: anywhere; }
  .state { flex: none; padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.72rem; }
  .state.open { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent); }
  .state.waiting { border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); color: var(--warn); }
  .state.closed { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .notice { margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.86rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 820px) { .support { grid-template-columns: 1fr; } }
</style>
