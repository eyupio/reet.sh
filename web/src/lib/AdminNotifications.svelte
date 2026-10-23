<script lang="ts">
  import { onMount } from 'svelte'
  import { getNotificationDefaults, saveNotificationDefaults, type NotificationDefault } from './api'

  let { role = 'owner', onerror }: { role?: string; onerror?: (e: unknown) => void } = $props()
  const isOwner = $derived(role === 'owner')
  let defaults = $state<NotificationDefault[]>([])
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let notice = $state('')

  const groups = $derived.by(() => {
    const out: { name: string; items: NotificationDefault[] }[] = []
    for (const d of defaults) {
      let g = out.find((x) => x.name === d.group)
      if (!g) { g = { name: d.group, items: [] }; out.push(g) }
      g.items.push(d)
    }
    return out
  })

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
      const r = await saveNotificationDefaults(defaults.filter((d) => !d.locked).map((d) => ({
        kind: d.key, enabled: d.default.enabled, mode: d.default.mode, owner: d.default.owner, admin: d.default.admin,
      })))
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
  {:else if loading}
    <p class="muted" role="status">Loading defaults…</p>
  {:else}
    <div class="head"><span class="muted">Owner and administrator columns apply to staff notifications only.</span><button type="button" class="btn primary" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save defaults'}</button></div>
    {#each groups as g (g.name)}
      <fieldset>
        <legend>{g.name}</legend>
        {#each g.items as d (d.key)}
          <div class="row">
            <div class="what"><strong>{d.label}</strong><small>{d.description}</small></div>
            {#if d.locked}
              <span class="locked">{d.locked}</span>
            {:else}
              <label class="check"><input type="checkbox" checked={d.default.enabled} onchange={(e) => (d.default.enabled = e.currentTarget.checked)} /> On</label>
              <label class="mode">Delivery
                <select value={d.default.mode} disabled={!d.default.enabled} onchange={(e) => (d.default.mode = e.currentTarget.value as 'immediate' | 'none')}>
                  <option value="immediate">Email now</option>
                  <option value="none">Centre only</option>
                </select>
              </label>
              {#if d.audience === 'staff'}
                <label class="check"><input type="checkbox" checked={d.default.owner} disabled={!d.default.enabled} onchange={(e) => (d.default.owner = e.currentTarget.checked)} /> Owner</label>
                <label class="check"><input type="checkbox" checked={d.default.admin} disabled={!d.default.enabled} onchange={(e) => (d.default.admin = e.currentTarget.checked)} /> Admins</label>
              {/if}
            {/if}
          </div>
        {/each}
      </fieldset>
    {/each}
  {/if}
</section>

<style>
  h2 { margin: 0 0 6px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .muted { margin: 4px 0 0; color: var(--muted); font-size: 0.84rem; line-height: 1.55; }
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; margin: 16px 0 0; }
  .head .muted { margin: 0; }
  fieldset { margin: 14px 0 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; min-width: 0; }
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
  .notice { margin: 12px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.85rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
</style>
