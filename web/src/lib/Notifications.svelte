<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getNotificationPreferences, listNotifications, markNotificationsRead, saveNotificationPreferences,
    type Notification, type NotificationPreference,
  } from './api'
  import PageHeader from './PageHeader.svelte'
  import { elapsed } from './time'

  let { onchange }: { onchange?: () => void } = $props()

  let rows = $state<Notification[]>([])
  let unread = $state(0)
  let prefs = $state<NotificationPreference[]>([])
  let unreadOnly = $state(false)
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let notice = $state('')

  const groups = $derived.by(() => {
    const out: { name: string; items: NotificationPreference[] }[] = []
    for (const p of prefs) {
      let g = out.find((x) => x.name === p.group)
      if (!g) { g = { name: p.group, items: [] }; out.push(g) }
      g.items.push(p)
    }
    return out
  })

  async function load() {
    loading = true
    try {
      const [list, p] = await Promise.all([listNotifications(unreadOnly), getNotificationPreferences()])
      rows = list.notifications
      unread = list.unread
      prefs = p.preferences
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  async function follow(n: Notification) {
    if (!n.readAt) {
      try {
        const r = await markNotificationsRead([n.id])
        unread = r.unread
        onchange?.()
      } catch (e) {
        error = e instanceof Error ? e.message : String(e)
        return
      }
    }
    if (n.link) location.href = n.link
  }

  async function markAll() {
    try {
      const r = await markNotificationsRead()
      unread = r.unread
      onchange?.()
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function savePrefs() {
    saving = true
    notice = ''
    error = ''
    try {
      const r = await saveNotificationPreferences(prefs.filter((p) => !p.locked).map((p) => ({ key: p.key, enabled: p.enabled, mode: p.mode })))
      prefs = r.preferences
      notice = r.done
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      saving = false
    }
  }

  function sourceWord(source: string) {
    switch (source) {
      case 'you': return 'Your choice'
      case 'platform': return 'The owner’s default'
      case 'role': return 'Your staff role’s default'
      case 'locked': return 'Always on'
      default: return 'The default'
    }
  }

  onMount(() => { void load() })
</script>

<svelte:head><title>Notifications · reet.sh</title></svelte:head>

<PageHeader eyebrow="Notifications" title="What's happened." lede="Ticket replies, account security notices and the staff events you chose to receive. Everything is also kept here, so nothing depends on email alone." />

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if notice}<p class="notice good" role="status">{notice}</p>{/if}

<div class="layout">
  <section class="card" aria-labelledby="centre-title">
    <div class="head">
      <h2 id="centre-title">Your notifications <small>{unread} unread</small></h2>
      <div class="actions">
        <button type="button" class="btn quiet" aria-pressed={unreadOnly} onclick={() => { unreadOnly = !unreadOnly; void load() }}>Unread only</button>
        <button type="button" class="btn quiet" onclick={markAll} disabled={unread === 0}>Mark all read</button>
        <button type="button" class="btn quiet" onclick={load} disabled={loading}>Refresh</button>
      </div>
    </div>
    {#if loading}
      <p class="muted" role="status">Loading notifications…</p>
    {:else if rows.length === 0}
      <p class="empty">{unreadOnly ? 'Nothing unread. Sorted.' : 'Nothing yet. Ticket replies, account notices and staff alerts appear here.'}</p>
    {:else}
      <ul class="list">
        {#each rows as n (n.id)}
          <li class:unread={!n.readAt}>
            <button type="button" onclick={() => follow(n)}>
              <span class="dot" aria-hidden="true"></span>
              <span class="main">
                <strong>{n.title}</strong>
                {#if n.body}<span class="body">{n.body}</span>{/if}
                <small>{elapsed(n.updatedAt)}{!n.readAt ? ' · unread' : ''}</small>
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card" aria-labelledby="prefs-title">
    <div class="head">
      <h2 id="prefs-title">What you hear about</h2>
      <button type="button" class="btn" onclick={savePrefs} disabled={saving}>{saving ? 'Saving…' : 'Save choices'}</button>
    </div>
    <p class="muted">Your own choice wins over the instance defaults. Security notices are always sent and cannot be turned off.</p>
    {#if prefs.length === 0 && !loading}
      <p class="empty">No notification kinds are available to this account.</p>
    {:else}
      {#each groups as g (g.name)}
        <fieldset>
          <legend>{g.name}</legend>
          {#each g.items as p (p.key)}
            <div class="pref">
              <label class="check">
                <input type="checkbox" checked={p.locked ? true : p.enabled} disabled={Boolean(p.locked)} onchange={(e) => (p.enabled = e.currentTarget.checked)} />
                <span><strong>{p.label}</strong><small>{p.description}</small></span>
              </label>
              {#if p.locked}
                <span class="locked">{p.locked}</span>
              {:else}
                <label class="mode">Delivery
                  <select value={p.mode} disabled={!p.enabled} onchange={(e) => (p.mode = e.currentTarget.value as 'immediate' | 'none')}>
                    <option value="immediate">Email me now</option>
                    <option value="none">Keep it in the centre only</option>
                  </select>
                </label>
              {/if}
              <span class="source">{sourceWord(p.source)}</span>
            </div>
          {/each}
        </fieldset>
      {/each}
    {/if}
  </section>
</div>

<style>
  .layout { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr); gap: 20px; margin-top: 24px; align-items: start; }
  .card { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  h2 { margin: 0; font: 500 1.4rem/1.2 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  h2 small { margin-left: 6px; color: var(--muted); font: 500 0.75rem/1.2 var(--sans); letter-spacing: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: 2px; }
  .muted, .empty { color: var(--muted); font-size: 0.84rem; line-height: 1.6; }
  .muted { margin: 8px 0 0; }
  .empty { margin: 18px 0 0; padding: 18px; border: 1px dashed var(--line); border-radius: 10px; text-align: center; font-family: var(--serif); font-size: 1.05rem; }
  .list { list-style: none; margin: 14px 0 0; padding: 0; }
  .list li + li { border-top: 1px solid var(--line); }
  .list button { width: 100%; display: flex; align-items: flex-start; gap: 10px; padding: 12px 4px; border: 0; background: transparent; text-align: left; color: inherit; cursor: pointer; border-radius: 8px; }
  .list button:hover { background: var(--tint); }
  .dot { flex: none; width: 7px; height: 7px; margin-top: 7px; border-radius: 50%; background: transparent; border: 1px solid var(--line); }
  .unread .dot { background: var(--accent); border-color: var(--accent); }
  .main { min-width: 0; display: grid; gap: 2px; }
  .main strong { overflow-wrap: anywhere; }
  .body { color: var(--muted); font-size: 0.84rem; line-height: 1.45; overflow-wrap: anywhere; }
  .main small { color: var(--muted); font-size: 0.73rem; }
  fieldset { margin: 14px 0 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; min-width: 0; }
  legend { padding: 0 5px; color: var(--muted); font-size: 0.78rem; font-weight: 600; }
  .pref { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 12px; padding: 10px 0; border-top: 1px solid var(--line); align-items: start; }
  .pref:first-of-type { border-top: 0; }
  .check { display: flex; align-items: flex-start; gap: 8px; min-width: 0; font-size: 0.84rem; }
  .check input { width: 18px; height: 18px; min-height: 0; margin-top: 1px; flex: none; }
  .check span { display: grid; gap: 2px; }
  .check small { color: var(--muted); font-weight: 400; line-height: 1.4; }
  .mode { display: grid; gap: 4px; color: var(--muted); font-size: 0.75rem; min-width: 160px; }
  .mode select { width: 100%; }
  .locked { grid-column: 1 / -1; color: var(--muted); font-size: 0.76rem; line-height: 1.45; }
  .source { grid-column: 2; color: var(--muted); font-size: 0.7rem; text-align: right; }
  .notice { margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.86rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 900px) { .layout { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .pref { grid-template-columns: 1fr; } .source { grid-column: auto; text-align: left; } }
</style>
