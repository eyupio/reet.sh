<script lang="ts">
  import { onMount } from 'svelte'
  import { getAdminStatus, saveAdminSettings, type AdminStatus } from './api'
  import { eventLabel, eventTitle } from './events'
  import { elapsed, isOldEnvironment } from './time'

  let { onerror, role = 'owner' }: { onerror?: (e: unknown) => void; role?: string } = $props()

  const isOwner = $derived(role === 'owner')

  let status = $state<AdminStatus | null>(null)
  let error = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let saved = $state('')
  let form = $state({ refreshEnabled: true, topicsEnabled: true, instanceName: '' })

  async function load() {
    loading = true
    try {
      const r = await getAdminStatus()
      status = r
      form = { ...r.settings }
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    document.title = 'Admin · reet.sh'
    void load()
    return () => {
      document.title = 'reet.sh – your shelf for AI skills'
    }
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    saving = true
    saved = ''
    error = ''
    try {
      await saveAdminSettings(form)
      if (status) status = { ...status, settings: { ...form } }
      saved = 'Saved.'
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      saving = false
    }
  }

  function when(value: string): string {
    const n = Number(value)
    return n ? new Date(n).toLocaleString() : 'Never'
  }

  function size(n: number): string {
    if (n < 1024) return `${n} B`
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
    return `${(n / (1024 * 1024)).toFixed(1)} MB`
  }
</script>

<section class="admin" aria-labelledby="admin-title">
  <a class="back" href="/">← Back to the shelf</a>
  <p class="eyebrow">Operator</p>
  <h2 id="admin-title">Admin</h2>

  {#if loading}
    <p class="empty" role="status">Loading…</p>
  {:else if !status}
    <p class="notice bad" role="alert">{error || 'Could not load the admin status.'}</p>
  {:else}
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    <div class="cards">
      <article>
        <h3>Version</h3>
        <p>{status.version}</p>
        <small>{status.commit}</small>
      </article>
      <article>
        <h3>skills.sh</h3>
        <p class:ok={status.skillsSh} class:bad={!status.skillsSh}>{status.skillsSh ? 'Reachable' : 'Not reachable'}</p>
        <small>{status.metaLastError || `Last metadata ${when(status.metaLastOK)}`}</small>
      </article>
      <article>
        <h3>Database</h3>
        <p>{size(status.db.size)}</p>
        <small><code>{status.db.path}</code></small>
      </article>
      <article>
        <h3>Topics</h3>
        <p>{status.topics.lastSynced ? 'Synced' : 'Not yet'}</p>
        <small>{status.topics.lastSynced ? when(status.topics.lastSynced) : status.topics.lastError || 'Waiting for the first seed'}</small>
      </article>
    </div>

    <div class="stages">
      <span><strong>{status.stages.use}</strong> in use</span>
      <span><strong>{status.stages.try}</strong> to test</span>
      <span><strong>{status.stages.shelf}</strong> on the shelf</span>
      <span><strong>{status.counts.liked}</strong> liked</span>
    </div>

    <section class="block" aria-labelledby="admin-envs">
      <h3 id="admin-envs">Environments</h3>
      {#if status.environments.length === 0}
        <p class="muted">No scans yet. Run <code>reet scan --env NAME</code> on a machine.</p>
      {:else}
        <div class="tablewrap">
          <table>
            <thead><tr><th>Environment</th><th>Skills</th><th>Last scan</th><th>Path</th></tr></thead>
            <tbody>
              {#each status.environments as env (env.id)}
                <tr class:old={isOldEnvironment(env.lastSeen)}>
                  <td>{env.name}</td>
                  <td class="num">{env.skillCount ?? 0}</td>
                  <td>{when(String(env.lastSeen))}</td>
                  <td><code>{env.path || '—'}</code></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    <section class="block" aria-labelledby="admin-settings">
      <h3 id="admin-settings">Jobs and instance</h3>
      {#if isOwner}
        <form onsubmit={save}>
          <label class="check"><input type="checkbox" bind:checked={form.refreshEnabled} /> Six-hourly metadata refresh</label>
          <label class="check"><input type="checkbox" bind:checked={form.topicsEnabled} /> Daily skills.sh topic seed</label>
          <label class="field">
            <span>Instance name</span>
            <input bind:value={form.instanceName} placeholder="Team shelf" autocomplete="off" />
          </label>
          <div class="save">
            <button type="submit" class="btn primary" disabled={saving}>{saving ? 'Saving…' : 'Save settings'}</button>
            {#if saved}<span class="saved" role="status">{saved}</span>{/if}
          </div>
        </form>
      {:else}
        <p class="muted">Only the owner can change these settings. Refresh is {form.refreshEnabled ? 'on' : 'off'}; the topic seed is {form.topicsEnabled ? 'on' : 'off'}.</p>
      {/if}
    </section>

    <section class="block" aria-labelledby="admin-events">
      <h3 id="admin-events">Recent activity</h3>
      {#if status.events.length === 0}
        <p class="muted">Nothing recorded yet.</p>
      {:else}
        <ul class="events">
          {#each status.events.slice(0, 20) as event (event.id)}
            <li title={eventTitle(event)}>
              <span>{eventLabel(event)}</span>
              <time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</section>

<style>
  .admin { padding-top: 4px; }
  .back { color: var(--muted); font-size: 0.9rem; text-decoration: none; }
  .back:hover { color: var(--ink); text-decoration: underline; text-underline-offset: 3px; }
  .eyebrow {
    margin: 22px 0 0; color: var(--accent); font-size: 0.72rem; font-weight: 700;
    letter-spacing: 0.12em; text-transform: uppercase;
  }
  h2 { display: block; margin: 6px 0 0; font: 600 2rem/1.1 var(--serif); letter-spacing: -0.02em; }
  .empty { text-align: center; color: var(--muted); font: italic 1.15rem/1.5 var(--serif); padding: 40px 0; }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 10px; margin-top: 22px; }
  .cards article { padding: 14px; background: var(--card); border: 1px solid var(--line); border-radius: 12px; }
  .cards h3, .block h3 {
    margin: 0; color: var(--muted); font-size: 0.68rem; font-weight: 700;
    letter-spacing: 0.1em; text-transform: uppercase;
  }
  .cards p { margin: 6px 0 0; font-size: 1.05rem; font-weight: 650; }
  .cards p.ok { color: var(--good); }
  .cards p.bad { color: var(--bad); }
  .cards small { display: block; margin-top: 4px; color: var(--muted); font-size: 0.78rem; overflow-wrap: anywhere; }
  .stages { display: flex; flex-wrap: wrap; gap: 8px 18px; margin-top: 14px; color: var(--muted); font-size: 0.9rem; }
  .stages strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .block { margin-top: 28px; padding-top: 18px; border-top: 1px solid var(--line); }
  .muted { margin: 8px 0 0; color: var(--muted); }
  .tablewrap { margin-top: 8px; overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th { text-align: left; color: var(--muted); font-weight: 600; padding: 6px 10px 6px 0; border-bottom: 1px solid var(--line); white-space: nowrap; }
  td { padding: 8px 10px 8px 0; border-bottom: 1px solid var(--line); vertical-align: top; }
  td.num { font-variant-numeric: tabular-nums; }
  tr.old td { opacity: 0.55; }
  form { display: grid; gap: 10px; margin-top: 10px; max-width: 460px; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 0.9rem; }
  .check input { min-height: 0; width: 18px; height: 18px; accent-color: var(--accent); }
  .field { display: grid; gap: 4px; font-size: 0.9rem; color: var(--muted); }
  .field input { width: 100%; }
  .save { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .saved { color: var(--good); font-size: 0.85rem; }
  .events { list-style: none; margin: 8px 0 0; padding: 0; }
  .events li { display: flex; justify-content: space-between; gap: 12px; padding: 6px 0; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.85rem; }
  .events li span { min-width: 0; overflow-wrap: anywhere; }
  .events time { flex: none; font-variant-numeric: tabular-nums; }
  @media (max-width: 560px) {
    h2 { font-size: 1.7rem; }
    .cards { grid-template-columns: 1fr 1fr; }
  }
</style>
