<script lang="ts">
  import { onMount } from 'svelte'
  import {
    adminAssignTicket, adminGetTicket, adminListTickets, adminNoteTicket, adminReplyTicket, adminSetTicketStatus,
    listAdminUsers, type Ticket, type TicketMessage, type TicketStatus, type UserMeta,
  } from './api'
  import { elapsed } from './time'

  let { onerror }: { onerror?: (e: unknown) => void } = $props()

  let tickets = $state<Ticket[]>([])
  let counts = $state<Record<TicketStatus, number>>({ open: 0, waiting: 0, closed: 0 })
  let admins = $state<UserMeta[]>([])
  let status = $state<'all' | TicketStatus>('open')
  let search = $state('')
  let selected = $state<Ticket | null>(null)
  let messages = $state<TicketMessage[]>([])
  let loading = $state(true)
  let detailLoading = $state(false)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let body = $state('')
  let mode = $state<'reply' | 'note'>('reply')

  const labels: Record<TicketStatus, string> = { open: 'Open', waiting: 'Waiting on requester', closed: 'Closed' }
  const authorLabel: Record<TicketMessage['authorKind'], string> = { user: 'Requester', team: 'Team manager', admin: 'Support' }
  const visible = $derived(tickets.filter((t) => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean)
    return words.every((word) => `${t.subject} ${t.opener} ${t.groupName ?? 'personal'} ${t.assignee ?? ''} ${t.id}`.toLowerCase().includes(word))
  }))

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  async function loadQueue() {
    try {
      const r = await adminListTickets(status === 'all' ? '' : status)
      tickets = r.tickets
      counts = r.counts
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  async function loadDetail(id: number) {
    detailLoading = true
    try {
      const r = await adminGetTicket(id)
      selected = r.ticket
      messages = r.messages
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      detailLoading = false
    }
  }

  function select(t: Ticket) {
    history.replaceState(null, '', `/admin/support?ticket=${t.id}`)
    void loadDetail(t.id)
  }

  async function refreshDetail() {
    if (selected) await loadDetail(selected.id)
  }

  async function send(status: 'waiting' | 'closed') {
    if (!selected || !body.trim() || busy) return
    busy = true
    error = ''
    notice = ''
    try {
      if (mode === 'note') {
        await adminNoteTicket(selected.id, body.trim())
        notice = 'Internal note added. The requester cannot see it.'
      } else {
        await adminReplyTicket(selected.id, body.trim(), status)
        notice = status === 'closed' ? 'Reply sent and ticket closed.' : 'Reply sent. The ticket waits on the requester.'
      }
      body = ''
      await Promise.all([loadQueue(), refreshDetail()])
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function changeStatus(next: TicketStatus) {
    if (!selected || busy) return
    busy = true
    error = ''
    try {
      const r = await adminSetTicketStatus(selected.id, next)
      selected = r.ticket
      notice = `Ticket is ${labels[next].toLowerCase()}.`
      await loadQueue()
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function assign(next: number) {
    if (!selected || busy) return
    busy = true
    error = ''
    try {
      const r = await adminAssignTicket(selected.id, next)
      selected = r.ticket
      notice = next ? `Assigned to ${r.ticket.assignee}.` : 'Ticket unassigned.'
      await loadQueue()
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  function total() { return counts.open + counts.waiting + counts.closed }

  onMount(() => {
    const id = Number(new URLSearchParams(location.search).get('ticket') ?? 0)
    if (id > 0) status = 'all'
    void loadQueue()
    void listAdminUsers().then((r) => {
      admins = r.users.filter((u) => (u.role === 'owner' || u.role === 'admin') && !u.disabledAt)
    }).catch(fail)
    if (id > 0) void loadDetail(id)
  })
</script>

<section class="support-admin" aria-labelledby="admin-tickets-title">
  <h2 id="admin-tickets-title">Support tickets</h2>
  <p class="muted">Tickets opened by people and by teams. A team's owner and managers answer their own; internal notes stay here.</p>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}

  <div class="layout">
    <section class="queue" aria-labelledby="queue-title">
      <h3 id="queue-title" class="vh">Ticket queue</h3>
      <div class="filters" role="group" aria-label="Ticket status">
        {#each [['open', 'Open'], ['waiting', 'Waiting'], ['closed', 'Closed'], ['all', 'All']] as [key, label]}
          <button type="button" class="btn quiet" aria-pressed={status === key} onclick={() => { status = key as typeof status; void loadQueue() }}>{label}<small>{key === 'all' ? total() : counts[key as TicketStatus]}</small></button>
        {/each}
      </div>
      <label class="search">Search
        <input type="search" bind:value={search} placeholder="Subject, opener, team or assignee" />
      </label>

      {#if loading}
        <p class="muted" role="status">Loading tickets…</p>
      {:else if visible.length === 0}
        <p class="empty">{tickets.length === 0 ? 'No tickets in this view.' : 'No ticket matches that search.'}</p>
      {:else}
        <ul class="tickets">
          {#each visible as t (t.id)}
            <li>
              <button type="button" class:selected={selected?.id === t.id} onclick={() => select(t)}>
                <span class="main">
                  <strong>{t.subject}</strong>
                  <small>#{t.id} · {t.opener}{t.groupName ? ` · ${t.groupName}` : ' · personal'} · {t.messages} {t.messages === 1 ? 'message' : 'messages'} · {elapsed(t.updatedAt)}</small>
                </span>
                <span class="state {t.status}">{labels[t.status]}</span>
                <span class="assignee">{t.assignee || 'Unassigned'}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="detail" aria-labelledby="detail-title">
      {#if detailLoading}
        <p class="muted" role="status">Loading the ticket…</p>
      {:else if !selected}
        <div class="empty-detail"><h3 id="detail-title">Choose a ticket</h3><p>Its conversation, notes and controls appear here.</p></div>
      {:else}
        <header class="detail-head">
          <div>
            <span class="eyebrow">Ticket #{selected.id}</span>
            <h3 id="detail-title">{selected.subject}</h3>
            <p class="meta">{selected.opener}{selected.groupName ? ` · ${selected.groupName}` : ' · personal'} · opened {elapsed(selected.createdAt)}</p>
          </div>
          <span class="state {selected.status}">{labels[selected.status]}</span>
        </header>

        <div class="controls">
          <label>Status
            <select value={selected.status} disabled={busy} onchange={(e) => changeStatus(e.currentTarget.value as TicketStatus)}>
              <option value="open">Open — ours to answer</option>
              <option value="waiting">Waiting on requester</option>
              <option value="closed">Closed</option>
            </select>
          </label>
          <label>Assign to
            <select value={selected.assigneeId ?? 0} disabled={busy} onchange={(e) => assign(Number(e.currentTarget.value))}>
              <option value={0}>Unassigned</option>
              {#each admins as a (a.id)}<option value={a.id}>{a.name || a.email}</option>{/each}
            </select>
          </label>
          <button type="button" class="btn quiet" onclick={refreshDetail} disabled={busy || detailLoading}>Refresh</button>
        </div>

        <ol class="thread" aria-label="Conversation">
          {#each messages as m (m.id)}
            <li class:note={m.internal} class:admin={m.authorKind === 'admin'} class:team={m.authorKind === 'team'}>
              <div class="who"><strong>{m.author || authorLabel[m.authorKind]}</strong><span>{authorLabel[m.authorKind]}</span>{#if m.internal}<span class="tag">Internal note</span>{/if}<time datetime={new Date(m.createdAt).toISOString()}>{elapsed(m.createdAt)}</time></div>
              <p>{m.body}</p>
            </li>
          {/each}
        </ol>

        <div class="composer">
          <div class="modes" role="group" aria-label="Kind of message">
            <button type="button" class="btn quiet" aria-pressed={mode === 'reply'} onclick={() => (mode = 'reply')}>Reply to requester</button>
            <button type="button" class="btn quiet" aria-pressed={mode === 'note'} onclick={() => (mode = 'note')}>Internal note</button>
          </div>
          <form onsubmit={(e) => { e.preventDefault(); void send('waiting') }}>
            <textarea bind:value={body} rows="5" maxlength="20000" placeholder={mode === 'note' ? 'Only instance administrators see this.' : 'Write to the requester…'} aria-label={mode === 'note' ? 'Internal note' : 'Reply'}></textarea>
            <div class="send">
              <span class="hint">{mode === 'note' ? 'The requester never sees notes.' : 'A reply waits on the requester; a closed ticket can be reopened later.'}</span>
              <div class="send-actions">
                {#if mode === 'reply'}<button type="button" class="btn" disabled={busy || !body.trim()} onclick={() => send('closed')}>Send reply and close</button>{/if}
                <button type="submit" class="btn primary" disabled={busy || !body.trim()}>{busy ? 'Sending…' : mode === 'note' ? 'Add note' : 'Send reply'}</button>
              </div>
            </div>
          </form>
        </div>
      {/if}
    </section>
  </div>
</section>

<style>
  .support-admin h2 { margin: 0 0 6px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .muted, .meta, .hint { color: var(--muted); }
  .muted { margin: 4px 0 0; }
  .layout { display: grid; grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.35fr); gap: 18px; margin-top: 18px; align-items: start; }
  .queue, .detail, .composer { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 16px; min-width: 0; }
  .queue { position: sticky; top: 18px; max-height: calc(100vh - 48px); overflow: auto; }
  .filters { display: flex; flex-wrap: wrap; gap: 2px; }
  .filters small { margin-left: 4px; color: var(--muted); }
  .search { margin-top: 12px; display: grid; gap: 5px; color: var(--muted); font-size: 0.8rem; }
  .search input { width: 100%; }
  .tickets { list-style: none; margin: 12px 0 0; padding: 0; }
  .tickets li + li { border-top: 1px solid var(--line); }
  .tickets button { width: 100%; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 10px; padding: 11px 6px; border: 0; background: transparent; text-align: left; cursor: pointer; color: inherit; border-radius: 8px; }
  .tickets button:hover, .tickets button.selected { background: var(--tint); }
  .tickets .main { min-width: 0; display: grid; gap: 3px; }
  .tickets strong { overflow-wrap: anywhere; }
  .tickets small { color: var(--muted); font-size: 0.74rem; overflow-wrap: anywhere; }
  .assignee { grid-column: 1 / -1; color: var(--muted); font-size: 0.72rem; }
  .state { justify-self: end; padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.7rem; white-space: nowrap; }
  .state.open { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent); }
  .state.waiting { border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); color: var(--warn); }
  .state.closed { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .empty, .empty-detail { color: var(--muted); font-size: 0.86rem; padding: 20px 0; text-align: center; }
  .empty-detail h3 { margin: 0 0 6px; font: 500 1.3rem/1.2 var(--serif); color: var(--ink); }
  .empty-detail p { margin: 0; }
  .detail-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .eyebrow { color: var(--muted); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
  .detail h3 { margin: 3px 0 5px; font: 500 1.5rem/1.2 var(--serif); overflow-wrap: anywhere; }
  .meta { margin: 0; font-size: 0.78rem; overflow-wrap: anywhere; }
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: 10px; margin: 16px 0; padding: 12px 0; border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); }
  .controls label { display: grid; gap: 4px; color: var(--muted); font-size: 0.76rem; min-width: 150px; }
  .controls select { width: 100%; }
  .thread { list-style: none; display: grid; gap: 9px; margin: 0 0 16px; padding: 0; }
  .thread li { border: 1px solid var(--line); border-radius: 10px; padding: 12px; background: color-mix(in srgb, var(--card) 90%, var(--bg)); }
  .thread li.admin { border-left: 3px solid var(--accent); }
  .thread li.team { border-left: 3px solid var(--good); }
  .thread li.note { background: color-mix(in srgb, var(--warn) 8%, var(--card)); border-style: dashed; }
  .who { display: flex; flex-wrap: wrap; align-items: baseline; gap: 5px 9px; margin-bottom: 6px; }
  .who strong { font-size: 0.86rem; }
  .who span, .who time { color: var(--muted); font-size: 0.72rem; }
  .tag { padding: 1px 6px; border: 1px solid color-mix(in srgb, var(--warn) 40%, var(--line)); border-radius: 999px; color: var(--warn); }
  .thread p { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.55; font-size: 0.9rem; }
  .composer { margin-top: 0; }
  .modes { display: flex; gap: 4px; margin-bottom: 8px; }
  .composer form { display: grid; gap: 8px; }
  .composer textarea { width: 100%; min-height: 110px; resize: vertical; }
  .send { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .send-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-left: auto; }
  .notice { margin: 12px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.84rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 1050px) {
    .layout { grid-template-columns: 1fr; }
    .queue { position: static; max-height: none; }
  }
</style>
