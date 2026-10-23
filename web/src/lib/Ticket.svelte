<script lang="ts">
  import { onMount } from 'svelte'
  import { closeTicket, getTicket, replyTicket, type Ticket, type TicketMessage, type TicketStatus } from './api'
  import { elapsed } from './time'

  let { id }: { id: number } = $props()
  let ticket = $state<Ticket | null>(null)
  let messages = $state<TicketMessage[]>([])
  let body = $state('')
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')

  const labels: Record<TicketStatus, string> = { open: 'With support', waiting: 'Waiting on you', closed: 'Closed' }
  const authorLabel: Record<TicketMessage['authorKind'], string> = { user: 'Requester', team: 'Team manager', admin: 'Support' }

  async function load() {
    try {
      const r = await getTicket(id)
      ticket = r.ticket
      messages = r.messages
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function send(e: SubmitEvent) {
    e.preventDefault()
    if (!body.trim() || busy) return
    busy = true
    error = ''
    notice = ''
    try {
      const r = await replyTicket(id, body.trim())
      ticket = r.ticket
      body = ''
      notice = 'Reply sent.'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  async function close() {
    if (!ticket || busy) return
    busy = true
    error = ''
    try {
      const r = await closeTicket(id)
      ticket = r.ticket
      notice = 'Ticket closed. A reply reopens it.'
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  onMount(() => { void load() })
</script>

<svelte:head><title>{ticket ? `${ticket.subject} · Support` : 'Support ticket'} · reet.sh</title></svelte:head>

<a class="back" href="/support">← All tickets</a>

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if notice}<p class="notice good" role="status">{notice}</p>{/if}

{#if !ticket && !error}
  <p class="hint" role="status">Loading the ticket…</p>
{:else if ticket}
  <header class="head">
    <div>
      <span class="eyebrow">Support ticket #{ticket.id}</span>
      <h1>{ticket.subject}</h1>
      <p class="meta">{ticket.opener}{ticket.groupName ? ` · ${ticket.groupName}` : ' · personal'} · opened {elapsed(ticket.createdAt)}{ticket.assignee ? ` · held by ${ticket.assignee}` : ''}</p>
    </div>
    <span class="state {ticket.status}">{labels[ticket.status]}</span>
  </header>

  <section class="thread" aria-label="Conversation">
    {#each messages as m (m.id)}
      <article class:hidden={m.internal} class:admin={m.authorKind === 'admin'} class:team={m.authorKind === 'team'}>
        <div class="who"><strong>{m.author}</strong><span>{authorLabel[m.authorKind]}</span><time datetime={new Date(m.createdAt).toISOString()}>{elapsed(m.createdAt)}</time></div>
        <p>{m.body}</p>
      </article>
    {/each}
  </section>

  <section class="card" aria-labelledby="reply-title">
    <h2 id="reply-title">{ticket.status === 'closed' ? 'Reply and reopen' : 'Reply'}</h2>
    <form onsubmit={send} aria-busy={busy}>
      <label class="vh" for="ticket-reply">Reply</label>
      <textarea id="ticket-reply" bind:value={body} rows="5" maxlength="20000" placeholder="Add what you have tried, or answer the question…" required></textarea>
      <div class="actions">
        <button type="submit" class="btn primary" disabled={busy || !body.trim()}>{busy ? 'Sending…' : ticket.status === 'closed' ? 'Reply and reopen' : 'Send reply'}</button>
        {#if ticket.status !== 'closed'}<button type="button" class="btn quiet" onclick={close} disabled={busy}>Close ticket</button>{/if}
      </div>
    </form>
  </section>
{/if}

<style>
  .back { display: inline-flex; min-height: 44px; align-items: center; color: var(--muted); font-size: 0.84rem; text-decoration: none; }
  .back:hover { color: var(--ink); text-decoration: underline; text-underline-offset: 2px; }
  .head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin: 8px 0 20px; }
  .eyebrow { color: var(--muted); font-size: 0.72rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
  h1 { margin: 4px 0 6px; font: 500 clamp(1.8rem, 3vw, 2.6rem)/1.15 var(--serif); letter-spacing: -0.03em; overflow-wrap: anywhere; }
  .meta { margin: 0; color: var(--muted); font-size: 0.82rem; overflow-wrap: anywhere; }
  .state { flex: none; padding: 4px 9px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.74rem; }
  .state.open { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent); }
  .state.waiting { border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); color: var(--warn); }
  .state.closed { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .thread { display: grid; gap: 10px; margin: 0 0 20px; }
  .thread article { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 16px; }
  .thread article.admin { border-left: 3px solid var(--accent); }
  .thread article.team { border-left: 3px solid var(--good); }
  .thread article.hidden { display: none; }
  .who { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 10px; margin-bottom: 8px; }
  .who strong { font-size: 0.9rem; }
  .who span, .who time { color: var(--muted); font-size: 0.74rem; }
  .thread p { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.6; }
  .card { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 18px; }
  h2 { margin: 0; font: 500 1.3rem/1.2 var(--serif); }
  form { display: grid; gap: 10px; margin-top: 12px; }
  textarea { width: 100%; min-height: 120px; resize: vertical; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .hint { color: var(--muted); font-size: 0.86rem; }
  .notice { margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.86rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
</style>
