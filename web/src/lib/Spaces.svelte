<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount, tick } from 'svelte'
  import { getGroups, createGroup, getTeamNotifications, saveTeamNotifications, setGroupMember, updateGroup, type Group, type TeamNotifyView } from './api'
  import {
    GROUP_SCOPES, groupCountLabel, groupKindLabel, groupRoleLabel, groupScopeCounts, matchesGroupScope,
    matchesGroupSearch, type GroupScope,
  } from './spaces'
  import CollectionPanel from './CollectionPanel.svelte'
  import PageHeader from './PageHeader.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite }: { canWrite: boolean } = $props()

  let groups = $state<Group[]>([])
  let name = $state('')
  let kind = $state('team')
  let parentId = $state(0)
  let busy = $state(false)
  let loading = $state(true)
  let error = $state('')
  let loadError = $state('')
  let notice = $state('')
  let creating = $state(false)
  let scope = $state<GroupScope>('all')
  let search = $state('')

  let emails = $state<Record<number, string>>({})
  let roles = $state<Record<number, string>>({})
  let renames = $state<Record<number, string>>({})
  let deleting = $state<Group | null>(null)
  let removing = $state<{ group: Group; email: string } | null>(null)
  let notify = $state<Record<number, TeamNotifyView>>({})
  let extraText = $state<Record<number, string>>({})
  let savingNotify = $state(0)
  let notifyNotice = $state('')

  const searched = $derived(groups.filter((group) => matchesGroupSearch(group, search)))
  const counts = $derived(groupScopeCounts(searched))
  const shown = $derived(searched.filter((group) => matchesGroupScope(group, scope)))
  const countLabel = $derived(groupCountLabel(shown.length, groups.length))
  const countText = $derived(loading ? 'Reading your groups…' : !groups.length && loadError ? 'Groups unavailable' : countLabel)

  async function rename(group: Group, remove = false) {
    busy = true
    error = ''
    deleting = null
    try {
      groups = (await updateGroup(group.id, renames[group.id] || group.name, remove)).groups
      notice = remove ? 'Group deleted. Lists remain private to their owners and individually invited people.' : 'Group renamed.'
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function load() {
    loading = true
    loadError = ''
    try {
      groups = (await getGroups()).groups
      error = ''
      await loadNotify(groups)
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
      error = loadError
    } finally {
      loading = false
    }
  }

  async function loadNotify(list: Group[]) {
    for (const group of list) {
      if (!['owner', 'manager'].includes(group.role)) continue
      try {
        const view = await getTeamNotifications(group.id)
        notify[group.id] = view
        extraText[group.id] = view.notify.extraEmails.join('\n')
      } catch {
        /* A group that cannot be read here simply has no notification form. */
      }
    }
  }

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
  }

  onMount(() => { void load() })

  async function startCreate() {
    creating = true
    await tick()
    document.getElementById('group-name')?.focus()
  }

  async function closeCreate() {
    creating = false
    await tick()
    document.getElementById('create-group-button')?.focus()
  }

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      groups = (await createGroup({ name, kind, parentId: kind === 'team' ? parentId : 0 })).groups
      name = ''
      await loadNotify(groups)
      notice = 'Created. Add people below, then use this group as a list audience.'
      await closeCreate()
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function member(group: Group, email: string, role: string, remove = false) {
    busy = true
    error = ''
    removing = null
    try {
      groups = (await setGroupMember(group.id, email, role, remove)).groups
      emails[group.id] = ''
      await loadNotify(groups)
      notice = remove ? 'Membership removed. Inherited list access has been revoked; separate personal invitations still apply.' : 'Membership saved. Access to lists shared with this group now follows its membership.'
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  function toggleEvent(view: TeamNotifyView, key: string, on: boolean) {
    const chosen = new Set(view.notify.events)
    if (on) chosen.add(key)
    else chosen.delete(key)
    view.notify.events = view.events.filter((event) => chosen.has(event.key)).map((event) => event.key)
  }

  async function saveNotify(group: Group) {
    const view = notify[group.id]
    if (!view) return
    savingNotify = group.id
    error = ''
    try {
      const saved = await saveTeamNotifications(group.id, {
        enabled: view.notify.enabled,
        events: view.notify.events,
        recipients: view.notify.recipients,
        extraEmails: (extraText[group.id] || '').split(/[\s,;]+/).filter(Boolean),
      })
      notify[group.id] = saved
      extraText[group.id] = saved.notify.extraEmails.join('\n')
      notifyNotice = 'Notifications saved.'
    } catch (e) {
      fail(e)
    } finally {
      savingNotify = 0
    }
  }

  function clearFilters() {
    search = ''
    scope = 'all'
  }

  function emptyTitle(): string {
    if (search) return 'Nowt matches just yet.'
    if (scope === 'organisations') return 'No organisations yet.'
    if (scope === 'teams') return 'No teams yet.'
    if (scope === 'managed') return 'No groups you manage.'
    return 'A place for every team.'
  }

  function emptyBody(): string {
    if (search) return 'Try another search or clear your filters.'
    if (scope === 'organisations') return 'Create an organisation to group teams under one name.'
    if (scope === 'teams') return 'Create an independent team, or a team inside one of your organisations.'
    if (scope === 'managed') return 'Groups you own or manage appear here.'
    return 'Create an organisation or an independent team, then add existing accounts. Group membership does not expose anyone’s personal shelf or machine reports.'
  }
</script>

<PageHeader eyebrow="Teams & organisations" title="A proper toolkit. Together." lede="Organisations group your teams; each has explicit membership. Share lists with a whole group, with additional people or individual exclusions when needed.">
  {#snippet actions()}
    {#if canWrite}<button id="create-group-button" class="btn primary" type="button" onclick={startCreate}>Create a group</button>{/if}
  {/snippet}
</PageHeader>

{#if error && !loadError}<p class="notice error" role="alert">{error}</p>{/if}
{#if notice}<p class="notice" role="status">{notice}</p>{/if}
{#if notifyNotice}<p class="notice" role="status">{notifyNotice}</p>{/if}

{#if canWrite && creating}
  <section class="create" id="create-group" aria-labelledby="create-title">
    <div class="create-head">
      <h2 id="create-title">Create a team or organisation</h2>
      <button type="button" class="btn quiet" onclick={closeCreate}>Cancel</button>
    </div>
    <p class="hint">An organisation groups teams under one name. A team can stand on its own or sit inside an organisation.</p>
    <form onsubmit={create}>
      <label>Name
        <input id="group-name" aria-label="Group name" bind:value={name} maxlength="80" required placeholder="e.g. Platform engineering" />
      </label>
      <label>Type
        <select aria-label="Group type" bind:value={kind}>
          <option value="team">Team</option>
          <option value="organisation">Organisation</option>
        </select>
      </label>
      {#if kind === 'team'}
        <label>Organisation
          <select aria-label="Parent organisation" bind:value={parentId}>
            <option value={0}>Independent team</option>
            {#each groups.filter(group => group.kind === 'organisation' && ['owner', 'manager'].includes(group.role)) as group (group.id)}
              <option value={group.id}>{group.name}</option>
            {/each}
          </select>
        </label>
      {/if}
      <button class="btn primary" type="submit" disabled={busy || !name.trim()}>{busy ? 'Creating…' : 'Create group'}</button>
    </form>
  </section>
{/if}

<CollectionPanel label="Your teams and organisations" count={countText}>
  {#snippet toolbar()}
    <div class="spaces-toolbar">
      <label class="search">Search groups
        <input type="search" bind:value={search} placeholder="Group or member" />
      </label>
      <div class="scopes" role="group" aria-label="Show groups">
        {#each GROUP_SCOPES as item (item.id)}
          <button type="button" class="scope" aria-pressed={scope === item.id} onclick={() => (scope = item.id)}>
            {item.label}<small>{counts[item.id]}</small>
          </button>
        {/each}
      </div>
    </div>
  {/snippet}

  {#if loading}
    <Skeleton label="Reading your groups…" shape="rows" count={5} />
  {:else if loadError}
    <div class="state error" role="alert">
      <p>{loadError}</p>
      <button type="button" class="btn" onclick={load}>Try again</button>
    </div>
  {:else if shown.length === 0}
    <div class="state">
      <h2>{emptyTitle()}</h2>
      <p>{emptyBody()}</p>
      <div class="state-actions">
        {#if search || scope !== 'all'}<button type="button" class="btn" onclick={clearFilters}>Clear filters</button>{/if}
        {#if canWrite}<button type="button" class="btn primary" onclick={startCreate}>Create a group</button>{/if}
      </div>
    </div>
  {:else}
    <div class="groups">
      {#each shown as group (group.id)}
        <details class="group">
          <summary>
            <div class="identity">
              <h2 class="group-name">{group.name}</h2>
              <span class="kind">{groupKindLabel(group, groups)}</span>
            </div>
            {#if group.members}<span class="members">{group.members.length} {group.members.length === 1 ? 'member' : 'members'}</span>{/if}
            <span class="role" class:manage={group.role === 'owner' || group.role === 'manager'}>{groupRoleLabel(group.role)}</span>
            <span class="chev" aria-hidden="true"></span>
          </summary>

          <div class="group-body">
            {#if canWrite && ['owner', 'manager'].includes(group.role)}
              <details class="settings">
                <summary>Group settings</summary>
                <form onsubmit={e => { e.preventDefault(); void rename(group) }}>
                  <label>Group name
                    <input aria-label={`Rename ${group.name}`} value={renames[group.id] ?? group.name} oninput={e => renames[group.id] = e.currentTarget.value} maxlength="80" required />
                  </label>
                  <button class="btn" type="submit" disabled={busy}>Rename</button>
                  {#if group.role === 'owner'}<button type="button" class="btn quiet danger" disabled={busy} onclick={() => (deleting = group)}>Delete group</button>{/if}
                </form>
              </details>

              <ul class="members-list">
                {#each group.members ?? [] as person (person.id)}
                  <li>
                    <div>
                      <strong>{person.name}</strong>
                      <small>{person.email} · {person.role}</small>
                    </div>
                    {#if person.id !== group.ownerId && (group.role === 'owner' || person.role !== 'manager')}
                      <div class="member-actions">
                        {#if group.role === 'owner'}
                          <select aria-label={`Group role for ${person.email}`} value={person.role} disabled={busy} onchange={e => void member(group, person.email, e.currentTarget.value)}>
                            <option value="member">Member</option>
                            <option value="manager">Manager</option>
                          </select>
                        {/if}
                        <button type="button" class="btn quiet" disabled={busy} onclick={() => (removing = { group, email: person.email })}>Remove</button>
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>

              <form class="add-form" onsubmit={e => { e.preventDefault(); void member(group, emails[group.id] || '', roles[group.id] || 'member') }}>
                <label>Account email
                  <input aria-label={`Add person to ${group.name}`} type="email" bind:value={emails[group.id]} required placeholder="person@example.com" />
                </label>
                <label>Role
                  <select aria-label={`New role in ${group.name}`} value={roles[group.id] || 'member'} onchange={e => roles[group.id] = e.currentTarget.value}>
                    <option value="member">Member</option>
                    {#if group.role === 'owner'}<option value="manager">Manager</option>{/if}
                  </select>
                </label>
                <button class="btn" type="submit" disabled={busy}>Add person</button>
              </form>
            {:else}
              <p class="hint">Ask this group’s manager to update its membership. Group-shared lists appear under Lists → Teams &amp; organisations.</p>
            {/if}

            {#if canWrite && ['owner', 'manager'].includes(group.role)}
              <details class="notify">
                <summary>Notifications</summary>
                {#if notify[group.id]}
                  {@const view = notify[group.id]}
                  <p class="hint">What this team is emailed about. Every kind is on until a manager chooses otherwise. Messages are written to the instance outbox and sent through its mail server.</p>
                  {#if !view.mailReady}
                    <p class="notice">The instance owner has not set up a mail server yet. These choices are saved, but no team notices are sent until one is set.</p>
                  {/if}
                  <form onsubmit={(e) => { e.preventDefault(); void saveNotify(group) }}>
                    <label class="check"><input type="checkbox" checked={view.notify.enabled} onchange={(e) => (view.notify.enabled = e.currentTarget.checked)} /> Email this team about changes</label>
                    <fieldset class="events" disabled={!view.notify.enabled}>
                      <legend>What to send</legend>
                      {#each view.events as event (event.key)}
                        <label class="check"><input type="checkbox" checked={view.notify.events.includes(event.key)} onchange={(e) => toggleEvent(view, event.key, e.currentTarget.checked)} /><span><strong>{event.label}</strong><small>{event.description}</small></span></label>
                      {/each}
                    </fieldset>
                    <div class="notify-pair">
                      <label>Send to
                        <select value={view.notify.recipients} onchange={(e) => (view.notify.recipients = e.currentTarget.value === 'members' ? 'members' : 'managers')} disabled={!view.notify.enabled}>
                          <option value="managers">The owner and managers</option>
                          <option value="members">Every member</option>
                        </select>
                      </label>
                      <label>Extra addresses
                        <textarea rows="2" placeholder="team-inbox@example.com, one per line" value={extraText[group.id] ?? ''} oninput={(e) => (extraText[group.id] = e.currentTarget.value)} disabled={!view.notify.enabled}></textarea>
                        <small>People without access also receive the notice. They are not given access to anything.</small>
                      </label>
                    </div>
                    <button class="btn" type="submit" disabled={busy || savingNotify === group.id}>{savingNotify === group.id ? 'Saving…' : 'Save notifications'}</button>
                  </form>
                {:else}
                  <p class="hint">Loading this group’s notification settings…</p>
                {/if}
              </details>
            {/if}
          </div>
        </details>
      {/each}
    </div>
  {/if}
</CollectionPanel>

<p class="hint page-hint">Groups organise access within this reet site. They are not separate hosted tenants or an SSO directory. Instance administrators retain administrative access.</p>

{#if removing}
  <ConfirmDialog title="Remove group membership?" body={`${removing.email} will lose access inherited from ${removing.group.name}. Direct invitations to individual lists remain.`} confirmLabel="Remove membership" oncancel={() => (removing = null)} onconfirm={() => { if (removing) void member(removing.group, removing.email, 'member', true) }} />
{/if}
{#if deleting}
  <ConfirmDialog title={`Delete ${deleting.name}?`} body="Its lists and skills remain. Members lose inherited group access; individual list invitations still apply. Delete child teams before deleting their organisation." confirmLabel="Delete group" oncancel={() => (deleting = null)} onconfirm={() => { if (deleting) void rename(deleting, true) }} />
{/if}

<style>
  .notice { margin: 12px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); font-size: 0.875rem; }
  .notice.error { border-color: var(--bad); }
  .hint { color: var(--muted); font-size: 0.875rem; line-height: 1.6; }
  .page-hint { margin: 12px 2px 0; font-size: 0.875rem; }

  .create { margin: 0 0 18px; padding: 18px; border: 1px solid var(--line); border-radius: 14px; background: var(--card); box-shadow: var(--shadow); }
  .create-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .create h2 { margin: 0; font: 500 1.3rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .create .hint { margin: 6px 0 0; }
  .create form, .group-body form { display: flex; flex-wrap: wrap; align-items: end; gap: 12px; margin-top: 16px; }
  label { display: grid; gap: 6px; min-width: 0; flex: 1 1 160px; color: var(--muted); font-size: 0.875rem; }
  input, select { width: 100%; min-width: 0; }

  .spaces-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 260px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }

  .groups { display: grid; }
  .group { border-top: 1px solid var(--line-soft); }
  .group:first-child { border-top: 0; }
  .group > summary { display: grid; grid-template-columns: minmax(0, 1fr) auto auto 10px; align-items: center; gap: 10px 16px; min-height: 64px; padding: 14px 2px; list-style: none; cursor: pointer; }
  .group > summary::-webkit-details-marker { display: none; }
  .identity { min-width: 0; display: grid; gap: 3px; }
  .group-name { display: block; margin: 0; font: 500 1.2rem/1.3 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .kind, .members { color: var(--muted); font-size: 0.875rem; }
  .members { white-space: nowrap; font-variant-numeric: tabular-nums; }
  .role { padding: 3px 8px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; white-space: nowrap; }
  .role.manage { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, var(--line)); }
  .chev { width: 7px; height: 7px; border-right: 2px solid var(--muted); border-bottom: 2px solid var(--muted); transform: rotate(45deg); }
  .group[open] > summary .chev { transform: rotate(-135deg); }

  .group-body { padding: 0 2px 18px; }
  .settings, .notify { margin-top: 12px; border-top: 1px solid var(--line-soft); padding-top: 10px; }
  .settings > summary, .notify > summary { min-height: 34px; cursor: pointer; font-size: 0.875rem; font-weight: 600; }
  .members-list { list-style: none; margin: 8px 0 0; padding: 0; display: grid; }
  .members-list li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 10px 0; border-top: 1px solid var(--line-soft); }
  .members-list li:first-child { border-top: 0; }
  .members-list strong, .members-list small { display: block; overflow-wrap: anywhere; }
  .members-list small { color: var(--muted); font-size: 0.875rem; }
  .member-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .member-actions select { width: auto; }

  .notify form { display: grid; gap: 12px; margin-top: 12px; }
  .check { display: flex; align-items: flex-start; gap: 8px; flex: none; color: var(--ink); font-size: 0.875rem; }
  .check input { width: 18px; height: 18px; min-height: 0; margin-top: 1px; flex: none; }
  .check span { display: grid; gap: 2px; }
  .check small { color: var(--muted); font-weight: 400; line-height: 1.4; }
  fieldset.events { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 16px; border: 1px solid var(--line); border-radius: 8px; padding: 12px; }
  fieldset.events legend { padding: 0 5px; color: var(--muted); font-size: 0.875rem; }
  .notify-pair { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .notify-pair label { display: grid; gap: 5px; color: var(--muted); font-size: 0.875rem; }
  .notify-pair textarea { width: 100%; min-height: 64px; }

  .state { padding: 26px 0 18px; max-width: 62ch; }
  .state h2 { margin: 0; font: 500 1.3rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .state p { margin: 6px 0 0; color: var(--muted); }
  .state-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
  .error p { margin: 0 0 12px; color: var(--ink); }

  @media (prefers-reduced-motion: no-preference) {
    .scope, .group > summary, .chev { transition: background-color 0.15s, border-color 0.15s, color 0.15s, transform 160ms ease; }
  }
  @media (max-width: 650px) {
    .spaces-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .group > summary { grid-template-columns: minmax(0, 1fr) auto 10px; gap: 8px 12px; }
    .members { grid-column: 1; grid-row: 2; }
    .role { grid-column: 2; grid-row: 1; }
    .chev { grid-column: 3; grid-row: 1; }
    fieldset.events, .notify-pair { grid-template-columns: 1fr; }
    .create form, .group-body form { align-items: stretch; }
  }
  @media (pointer: coarse) { .scope, .settings > summary, .notify > summary { min-height: 44px; } }
</style>
