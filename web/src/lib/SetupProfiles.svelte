<script lang="ts">
  import { onMount } from 'svelte'
  import {
    deleteSetupProfile, getSetupProfile, listSetupProfiles,
    type SetupProfileRecord, type SetupProfileRevision, type SetupProfileSummary,
  } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PageHeader from './PageHeader.svelte'

  let {
    canWrite = true,
    onerror,
  }: { canWrite?: boolean; onerror?: (e: unknown) => void } = $props()

  let profiles = $state<SetupProfileSummary[]>([])
  let selected = $state<SetupProfileRecord | null>(null)
  let loading = $state(true)
  let busy = $state('')
  let error = $state('')
  let deleteFor = $state<SetupProfileSummary | null>(null)

  const latest = $derived(selected?.revisions.at(-1) ?? null)
  const componentCount = $derived.by(() => {
    if (!latest?.preview?.agents) return 0
    return latest.preview.agents.reduce((n, agent) => n + (agent.components?.length ?? 0), 0)
  })
  const omissionCount = $derived.by(() => {
    if (!latest?.preview?.agents) return 0
    return latest.preview.agents.reduce((n, agent) => n + (agent.omitted?.length ?? 0), 0)
  })

  onMount(() => {
    void load()
  })

  async function load() {
    loading = true
    try {
      const result = await listSetupProfiles()
      profiles = result.profiles ?? []
      error = ''
      if (selected && !profiles.some((profile) => profile.name === selected?.name)) selected = null
      if (!selected && profiles.length > 0) await choose(profiles[0].name)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  async function choose(name: string) {
    busy = name
    try {
      selected = await getSetupProfile(name)
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function remove() {
    if (!deleteFor) return
    const name = deleteFor.name
    busy = name
    try {
      await deleteSetupProfile(name)
      deleteFor = null
      selected = null
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  function stamp(value: number): string {
    if (!value) return ''
    return new Date(value).toLocaleString()
  }

  function when(value: string): string {
    if (!value) return ''
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? '' : parsed.toLocaleString()
  }

  function revisionSummary(revision: SetupProfileRevision): string {
    const components = revision.preview?.agents?.reduce((n, agent) => n + (agent.components?.length ?? 0), 0) ?? 0
    return `${components} components`
  }
</script>

<PageHeader
  eyebrow="Agent setups"
  title="Profiles."
  lede="Named setup revisions stored by your account. Push one with reet setup profile push; the snapshot stays on your machine and is fetched separately."
/>

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}

{#if loading}
  <p class="empty" role="status">Loading profiles…</p>
{:else if profiles.length === 0}
  <section class="empty-state">
    <h2>No profiles yet.</h2>
    <p>Save a local profile revision, then push it here:</p>
    <pre><code>reet setup profile save --password-file FILE work SNAPSHOT
reet setup profile push NAME</code></pre>
    <p>Pulling a profile never downloads its snapshot; use <code>reet setup fetch --s3 --profile NAME</code> for that.</p>
  </section>
{:else}
  <div class="profiles">
    <section class="profile-list" aria-label="Setup profiles">
      {#each profiles as profile (profile.name)}
        <button
          type="button"
          class="profile-row"
          class:active={selected?.name === profile.name}
          aria-pressed={selected?.name === profile.name}
          onclick={() => choose(profile.name)}
          disabled={busy !== ''}
        >
          <span class="name">{profile.name}</span>
          <span class="revision">revision {profile.revision}</span>
          <span class="snapshot">{profile.snapshotId}</span>
          <span class="updated">{stamp(profile.updatedAt)}</span>
        </button>
      {/each}
    </section>

    <section class="profile-detail" aria-live="polite">
      {#if !selected}
        <p class="empty">Choose a profile to inspect it.</p>
      {:else if latest}
        <header class="detail-head">
          <div>
            <p class="eyebrow">Profile</p>
            <h2>{selected.name}</h2>
            <p class="muted">Latest revision {latest.revision} of {selected.revisions.length}</p>
          </div>
          {#if canWrite}
            <button type="button" class="btn quiet danger" onclick={() => (deleteFor = profiles.find((profile) => profile.name === selected?.name) ?? null)} disabled={busy !== ''}>Delete</button>
          {/if}
        </header>

        <dl class="facts">
          <div><dt>Snapshot</dt><dd><code>{latest.snapshot_id}</code></dd></div>
          <div><dt>Taken</dt><dd>{when(latest.snapshot_taken_at)}</dd></div>
          <div><dt>Recorded</dt><dd>{when(latest.created_at)}</dd></div>
          <div><dt>Selected</dt><dd>{componentCount} components</dd></div>
          <div><dt>Omitted</dt><dd>{omissionCount} records</dd></div>
          <div><dt>Software evidence</dt><dd>{latest.software?.length ?? 0} agents</dd></div>
        </dl>

        {#if latest.preview?.credentials_included}
          <p class="notice good">Literal credentials were included inside the encrypted snapshot. They are not shown here.</p>
        {/if}

        {#if latest.overrides && ((latest.overrides.exclude_kinds?.length ?? 0) > 0 || (latest.overrides.exclude_sources?.length ?? 0) > 0)}
          <div class="overrides">
            <h3>Overrides</h3>
            {#if latest.overrides.exclude_kinds?.length}
              <p><span class="tag">excluded kinds</span> {latest.overrides.exclude_kinds.join(', ')}</p>
            {/if}
            {#if latest.overrides.exclude_sources?.length}
              <p><span class="tag">excluded sources</span> {latest.overrides.exclude_sources.join(', ')}</p>
            {/if}
          </div>
        {/if}

        <div class="history">
          <h3>Revision history</h3>
          <ol>
            {#each selected.revisions as revision (revision.revision)}
              <li class:current={revision.revision === latest.revision}>
                <span class="rev">r{revision.revision}</span>
                <span>{when(revision.created_at)}</span>
                <span class="muted">{revisionSummary(revision)}</span>
              </li>
            {/each}
          </ol>
        </div>

        <p class="muted small">Use <code>reet setup restore --profile {selected.name}</code> to plan a restore, or add <code>--revision N</code> for an older revision. The snapshot must exist locally or be fetched first.</p>
      {:else}
        <p class="empty">This profile has no revisions.</p>
      {/if}
    </section>
  </div>
{/if}

{#if deleteFor}
  <ConfirmDialog
    title={`Delete ${deleteFor.name}?`}
    body="This removes the server copy of the profile. Local snapshots and local profiles are not deleted."
    confirmLabel="Delete profile"
    danger
    onconfirm={remove}
    oncancel={() => (deleteFor = null)}
  />
{/if}

<style>
  .profiles { display: grid; grid-template-columns: minmax(240px, 0.8fr) minmax(0, 1.4fr); gap: 18px; align-items: start; }
  .profile-list { display: grid; gap: 8px; }
  .profile-row {
    display: grid; grid-template-columns: 1fr auto; gap: 2px 12px; text-align: left; padding: 12px 14px;
    border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--ink); cursor: pointer;
  }
  .profile-row:hover { border-color: var(--muted); }
  .profile-row.active { border-color: var(--accent); background: var(--tint); }
  .profile-row:disabled { opacity: 0.65; cursor: default; }
  .name { font-weight: 650; overflow-wrap: anywhere; }
  .revision { color: var(--accent); font-size: 0.76rem; font-weight: 600; }
  .snapshot, .updated { grid-column: 1 / -1; color: var(--muted); font-size: 0.75rem; overflow-wrap: anywhere; }
  .profile-detail { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  .detail-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 4px 0 6px; font: 500 1.7rem/1.1 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  h3 { margin: 22px 0 8px; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
  .muted { color: var(--muted); overflow-wrap: anywhere; }
  .small { font-size: 0.8rem; }
  .facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 16px; margin: 18px 0 0; }
  .facts div { min-width: 0; }
  dt { color: var(--muted); font-size: 0.72rem; margin-bottom: 2px; }
  dd { margin: 0; overflow-wrap: anywhere; font-size: 0.84rem; }
  code { font-size: 0.8em; }
  .notice { margin-top: 16px; }
  .overrides p { margin: 6px 0; font-size: 0.82rem; overflow-wrap: anywhere; }
  .history ol { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .history li { display: grid; grid-template-columns: auto 1fr auto; gap: 10px; align-items: baseline; padding: 6px 8px; border-radius: 6px; font-size: 0.8rem; }
  .history li.current { background: var(--tint); }
  .rev { color: var(--accent); font-weight: 650; }
  .empty-state { border: 1px dashed var(--line); border-radius: 12px; padding: 24px; }
  .empty-state h2 { margin-top: 0; }
  .empty-state p { color: var(--muted); max-width: 60ch; }
  pre { overflow: auto; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  @media (max-width: 900px) {
    .profiles { grid-template-columns: 1fr; }
    .facts { grid-template-columns: 1fr; }
  }
  @media (max-width: 560px) {
    .profile-detail { padding: 16px; }
    .detail-head { flex-direction: column; }
  }
</style>
