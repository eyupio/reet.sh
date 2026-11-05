<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import {
    getNotificationPreferences, listNotifications, markNotificationsRead, saveNotificationPreferences,
    type Notification, type NotificationPreference,
  } from './api'
  import CollectionPanel from './CollectionPanel.svelte'
  import PageHeader from './PageHeader.svelte'
  import { elapsed } from './time'

  let { onchange }: { onchange?: () => void } = $props()

  let rows = $state<Notification[]>([])
  let unread = $state(0)
  let prefs = $state<NotificationPreference[]>([])
  let unreadOnly = $state(false)
  let loading = $state(true)
  let saving = $state(false)
  let loadError = $state('')
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
  const countLabel = $derived(loading ? 'Reading notifications…' : loadError ? 'Notifications unavailable' : unreadOnly ? (rows.length === unread ? `${unread} unread` : `${rows.length} of ${unread} unread`) : `${rows.length} ${rows.length === 1 ? 'notification' : 'notifications'}${unread ? ` · ${unread} unread` : ''}`)

  async function load() {
    loading = true
    loadError = ''
    try {
      const [list, p] = await Promise.all([listNotifications(unreadOnly), getNotificationPreferences()])
      rows = list.notifications
      unread = list.unread
      prefs = p.preferences
      error = ''
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  async function loadFeed(only: boolean) {
    loading = true
    loadError = ''
    try {
      const list = await listNotifications(only)
      rows = list.notifications
      unread = list.unread
      error = ''
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  function show(only: boolean) {
    if (unreadOnly === only) return
    unreadOnly = only
    void loadFeed(only)
  }

  async function follow(n: Notification) {
    if (!n.readAt) {
      try {
        const r = await markNotificationsRead([n.id])
        unread = r.unread
        n.readAt = Date.now()
        if (unreadOnly) rows = rows.filter((row) => row.id !== n.id)
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
      await markNotificationsRead()
      onchange?.()
      await loadFeed(unreadOnly)
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
  <CollectionPanel label="Your notifications" count={countLabel}>
    {#snippet actions()}
      <button type="button" class="btn quiet" onclick={markAll} disabled={unread === 0}>Mark all read</button>
      <button type="button" class="btn quiet" onclick={() => loadFeed(unreadOnly)} disabled={loading}>Refresh</button>
    {/snippet}
    {#snippet toolbar()}
      <div class="scopes" role="group" aria-label="Show notifications">
        <button type="button" class="scope" aria-pressed={!unreadOnly} onclick={() => show(false)}>All<small>{rows.length}</small></button>
        <button type="button" class="scope" aria-pressed={unreadOnly} onclick={() => show(true)}>Unread<small>{unread}</small></button>
      </div>
    {/snippet}

    {#if loading}
      <Skeleton label="Reading notifications…" shape="rows" count={5} />
    {:else if loadError}
      <div class="state error" role="alert">
        <p>{loadError}</p>
        <button type="button" class="btn" onclick={() => loadFeed(unreadOnly)}>Try again</button>
      </div>
    {:else if rows.length === 0}
      <div class="state">
        <h2>{unreadOnly ? 'Nothing unread. Sorted.' : 'Nothing yet.'}</h2>
        <p>{unreadOnly ? 'Everything has been read.' : 'Ticket replies, account notices and staff alerts appear here.'}</p>
        {#if unreadOnly}<button type="button" class="btn" onclick={() => show(false)}>Show all</button>{/if}
      </div>
    {:else}
      <div class="feed">
        {#each rows as n (n.id)}
          <button type="button" class="notification" class:unread={!n.readAt} onclick={() => follow(n)}>
            <span class="dot" aria-hidden="true"></span>
            <span class="main">
              <strong>{n.title}</strong>
              {#if n.body}<span class="body">{n.body}</span>{/if}
              <small>{elapsed(n.updatedAt)}{!n.readAt ? ' · unread' : ''}</small>
            </span>
            {#if n.link}<span class="go" aria-hidden="true"></span>{/if}
          </button>
        {/each}
      </div>
    {/if}
  </CollectionPanel>

  <section class="card" aria-labelledby="prefs-title">
    <div class="head">
      <h2 id="prefs-title">What you hear about</h2>
      <button type="button" class="btn" onclick={savePrefs} disabled={saving} aria-busy={saving}>{saving ? 'Saving…' : 'Save choices'}</button>
    </div>
    <p class="muted">Your own choice wins over the instance defaults. Security notices are always sent and cannot be turned off.</p>
    {#if prefs.length === 0 && !loading}
      <p class="empty">No notification kinds are available to this account.</p>
    {:else}
      {#each groups as g (g.name)}
        <fieldset>
          <legend>{g.name}</legend>
          {#each g.items as p (p.key)}
            {#if p.locked}
              <div class="pref locked-row">
                <div class="check static">
                  <span><strong>{p.label}</strong><small>{p.description}</small></span>
                </div>
                <span class="badge">Always on</span>
              </div>
            {:else}
              <div class="pref">
                <label class="check">
                  <input type="checkbox" checked={p.enabled} onchange={(e) => (p.enabled = e.currentTarget.checked)} />
                  <span><strong>{p.label}</strong><small>{p.description}</small></span>
                </label>
                <label class="mode">Delivery
                  <select value={p.mode} disabled={!p.enabled} onchange={(e) => (p.mode = e.currentTarget.value as 'immediate' | 'none')}>
                    <option value="immediate">Email me now</option>
                    <option value="none">Keep it in the centre only</option>
                  </select>
                </label>
                <span class="source">{sourceWord(p.source)}</span>
              </div>
            {/if}
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
  .muted, .empty { color: var(--muted); font-size: 0.875rem; line-height: 1.6; }
  .muted { margin: 8px 0 0; }
  .empty { margin: 18px 0 0; padding: 18px; border: 1px dashed var(--line); border-radius: 10px; text-align: center; font-family: var(--serif); font-size: 1.05rem; }

  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }

  .feed { display: grid; }
  .notification { width: 100%; display: flex; align-items: flex-start; gap: 10px; padding: 14px 2px; border: 0; border-top: 1px solid var(--line-soft); background: transparent; text-align: left; color: inherit; cursor: pointer; }
  .notification:first-child { border-top: 0; }
  .notification:hover { background: color-mix(in srgb, var(--tint) 45%, transparent); }
  .notification.unread { background: color-mix(in srgb, var(--tint) 28%, transparent); }
  .notification.unread:hover { background: color-mix(in srgb, var(--tint) 50%, transparent); }
  .dot { flex: none; width: 7px; height: 7px; margin-top: 7px; border-radius: 50%; background: transparent; border: 1px solid var(--line-strong); }
  .unread .dot { background: var(--accent); border-color: var(--accent); }
  .main { min-width: 0; flex: 1; display: grid; gap: 2px; }
  .main strong { overflow-wrap: anywhere; }
  .unread .main strong { font-weight: 650; }
  .body { color: var(--muted); font-size: 0.875rem; line-height: 1.45; overflow-wrap: anywhere; }
  .main small { color: var(--muted); font-size: 0.875rem; }
  .go { flex: none; width: 7px; height: 7px; margin-top: 7px; border-top: 2px solid var(--muted); border-right: 2px solid var(--muted); transform: rotate(45deg); }

  .state { padding: 24px 0 16px; max-width: 58ch; }
  .state h2 { font-size: 1.25rem; }
  .state p { margin: 6px 0 12px; color: var(--muted); }
  .error p { color: var(--ink); }

  fieldset { margin: 14px 0 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; min-width: 0; }
  legend { padding: 0 5px; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .pref { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 12px; padding: 10px 0; border-top: 1px solid var(--line); align-items: start; }
  .pref:first-of-type { border-top: 0; }
  .check { display: flex; align-items: flex-start; gap: 8px; min-width: 0; font-size: 0.875rem; }
  .check.static { align-items: flex-start; }
  .check input { width: 18px; height: 18px; min-height: 0; margin-top: 1px; flex: none; }
  .check span { display: grid; gap: 2px; }
  .check small { color: var(--muted); font-weight: 400; line-height: 1.4; }
  .badge { align-self: start; border: 1px solid var(--line); border-radius: 999px; padding: 3px 8px; color: var(--muted); font-size: 0.875rem; white-space: nowrap; }
  .mode { display: grid; gap: 4px; color: var(--muted); font-size: 0.875rem; min-width: 160px; }
  .mode select { width: 100%; }
  .source { grid-column: 2; color: var(--muted); font-size: 0.875rem; text-align: right; }
  .notice { margin: 16px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.875rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 900px) { .layout { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .pref { grid-template-columns: 1fr; } .source { grid-column: auto; text-align: left; } }
  @media (pointer: coarse) { .scope { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .scope, .notification { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
