<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import { notificationDefaultCountLabel, notificationDefaultGroups, notificationDefaultPayload } from './adminNotifications'
  import { getNotificationDefaults, saveNotificationDefaults, type NotificationDefault } from './api'

  let { role = 'owner', onerror }: { role?: string; onerror?: (e: unknown) => void } = $props()
  const isOwner = $derived(role === 'owner')
  let defaults = $state<NotificationDefault[]>([])
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let notice = $state('')

  const groups = $derived(notificationDefaultGroups(defaults))
  const countLabel = $derived(notificationDefaultCountLabel(defaults.length, loading))

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  async function load() {
    if (!isOwner) { loading = false; return }
    loading = true
    try {
      defaults = (await getNotificationDefaults()).defaults
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  async function save() {
    saving = true
    notice = ''
    error = ''
    try {
      const r = await saveNotificationDefaults(notificationDefaultPayload(defaults))
      defaults = r.defaults
      notice = r.done
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  onMount(() => { void load() })
</script>

<section class="notify-admin" aria-labelledby="notify-defaults-title">
  <h2 id="notify-defaults-title">Notification defaults</h2>
  <p class="muted">What everybody receives until they choose otherwise. Each person can override these under Notifications; the owner and administrators can also choose their own.</p>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}
  {#if !isOwner}
    <p class="muted">Only the owner can change the instance defaults.</p>
  {:else}
    <CollectionPanel label="Notification defaults" count={countLabel}>
      {#snippet actions()}
        <button type="button" class="btn primary" onclick={save} disabled={loading || saving || defaults.length === 0} aria-busy={saving}>{saving ? 'Saving…' : 'Save defaults'}</button>
      {/snippet}
      {#snippet toolbar()}
        <p class="note">Owner and administrator columns apply to staff notifications only.</p>
      {/snippet}

      {#if loading}
        <Skeleton label="Loading the defaults…" shape="rows" count={4} />
      {:else if defaults.length === 0}
        <div class="state-block" class:error={!!error}>
          <h3>{error ? 'The defaults could not be read.' : 'No notification kinds are available.'}</h3>
          <p>{error ? 'Try again without leaving this page.' : 'Kinds appear here when the server offers them.'}</p>
          {#if error}<button type="button" class="btn" onclick={load}>Try again</button>{/if}
        </div>
      {:else}
        {#each groups as group (group.name)}
          <fieldset>
            <legend>{group.name}</legend>
            {#each group.items as item (item.key)}
              <div class="row">
                <div class="what"><strong>{item.label}</strong><small>{item.description}</small></div>
                {#if item.locked}
                  <span class="locked">{item.locked}</span>
                {:else}
                  <label class="check"><input type="checkbox" checked={item.default.enabled} onchange={(e) => (item.default.enabled = e.currentTarget.checked)} /> On</label>
                  <label class="mode">Delivery
                    <select value={item.default.mode} disabled={!item.default.enabled} onchange={(e) => (item.default.mode = e.currentTarget.value as 'immediate' | 'none')}>
                      <option value="immediate">Email now</option>
                      <option value="none">Centre only</option>
                    </select>
                  </label>
                  {#if item.audience === 'staff'}
                    <label class="check"><input type="checkbox" checked={item.default.owner} disabled={!item.default.enabled} onchange={(e) => (item.default.owner = e.currentTarget.checked)} /> Owner</label>
                    <label class="check"><input type="checkbox" checked={item.default.admin} disabled={!item.default.enabled} onchange={(e) => (item.default.admin = e.currentTarget.checked)} /> Admins</label>
                  {/if}
                {/if}
              </div>
            {/each}
          </fieldset>
        {/each}
      {/if}
    </CollectionPanel>
  {/if}
</section>

<style>
  h2 { margin: 0 0 6px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .muted { margin: 4px 0 0; color: var(--muted); font-size: 0.84rem; line-height: 1.55; }
  .note { margin: 0; color: var(--muted); font-size: 0.8rem; line-height: 1.45; }
  fieldset { margin: 14px 0 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; min-width: 0; }
  fieldset:first-of-type { margin-top: 0; }
  legend { padding: 0 5px; color: var(--muted); font-size: 0.78rem; font-weight: 600; }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; padding: 10px 0; border-top: 1px solid var(--line); }
  .row:first-of-type { border-top: 0; }
  .what { flex: 1 1 220px; min-width: 0; display: grid; gap: 2px; }
  .what strong { font-size: 0.86rem; overflow-wrap: anywhere; }
  .what small { color: var(--muted); font-size: 0.75rem; line-height: 1.4; }
  .check { display: inline-flex; align-items: center; gap: 6px; color: var(--muted); font-size: 0.8rem; }
  .check input { width: 17px; height: 17px; min-height: 0; }
  .mode { display: grid; gap: 3px; color: var(--muted); font-size: 0.74rem; min-width: 150px; }
  .mode select { width: 100%; }
  .locked { flex: 1 1 100%; color: var(--muted); font-size: 0.76rem; line-height: 1.45; }
  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h3 { margin: 0; font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  .state-block.error p { color: var(--ink); }
  .notice { margin: 12px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.85rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 560px) {
    .row { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 12px; }
    .what, .mode, .locked { grid-column: 1 / -1; }
  }
  @media (pointer: coarse) {
    .check { min-height: 44px; }
    .check input { width: 20px; height: 20px; }
    .mode select { min-height: 44px; }
  }
</style>
