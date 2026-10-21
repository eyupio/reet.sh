<script lang="ts">
  import { onMount } from 'svelte'
  import {
    createInvite, deleteAdminUser, getAdminStatus, listAdminUsers, resetUserPassword, revokeUserTokens,
    saveAdminSettings, transferOwner, updateAdminUser, type AdminStatus, type UserMeta,
  } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { eventLabel, eventTitle } from './events'
  import { elapsed, isOldEnvironment } from './time'

  let {
    onerror, role = 'owner', accounts = false, currentEmail = '',
  }: { onerror?: (e: unknown) => void; role?: string; accounts?: boolean; currentEmail?: string } = $props()

  const isOwner = $derived(role === 'owner')

  let status = $state<AdminStatus | null>(null)
  let error = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let saved = $state('')
  let form = $state({ refreshEnabled: true, topicsEnabled: true, instanceName: '' })
  let users = $state<UserMeta[]>([])
  let usersError = $state('')
  let inviteEmail = $state('')
  let inviteRole = $state('member')
  let inviteLink = $state('')
  let inviteExpires = $state(0)
  let resetPassword = $state('')
  let resetName = $state('')
  let busyID = $state(0)
  let confirmAction = $state<{ kind: 'delete' | 'transfer'; user: UserMeta } | null>(null)

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

  async function loadUsers() {
    if (!accounts) return
    try {
      users = (await listAdminUsers()).users
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    }
  }

  onMount(() => {
    document.title = 'Admin · reet.sh'
    void load()
    void loadUsers()
    return () => {
      document.title = 'reet.sh – your shelf for AI skills'
    }
  })

  function canManage(user: UserMeta): boolean {
    if (!accounts || user.email === currentEmail || user.role === 'owner') return false
    if (role === 'admin' && user.role === 'admin') return false
    return true
  }

  function roleOptions(user: UserMeta): string[] {
    const options = ['member', 'viewer']
    if (role === 'owner' && user.role !== 'owner') options.push('admin')
    return options
  }

  async function saveRole(user: UserMeta, newRole: string) {
    if (newRole === user.role) return
    busyID = user.id
    try {
      users = (await updateAdminUser({ id: user.id, role: newRole })).users
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

  async function toggleDisabled(user: UserMeta) {
    busyID = user.id
    try {
      users = (await updateAdminUser({ id: user.id, disabled: user.disabledAt === undefined || user.disabledAt === 0 })).users
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

  async function resetPasswordFor(user: UserMeta) {
    busyID = user.id
    resetPassword = ''
    try {
      const r = await resetUserPassword(user.id)
      resetPassword = r.password
      resetName = user.name
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

  async function revokeTokensFor(user: UserMeta) {
    busyID = user.id
    try {
      await revokeUserTokens(user.id)
      await loadUsers()
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

  async function invite() {
    if (!inviteEmail.trim()) return
    busyID = -1
    inviteLink = ''
    try {
      const r = await createInvite(inviteEmail.trim(), inviteRole)
      inviteLink = location.origin + r.url
      inviteExpires = r.expiresAt
      inviteEmail = ''
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

  async function runConfirmed() {
    const action = confirmAction
    confirmAction = null
    if (!action) return
    busyID = action.user.id
    try {
      if (action.kind === 'delete') {
        await deleteAdminUser(action.user.id)
        await loadUsers()
      } else {
        users = (await transferOwner(action.user.id)).users
        location.reload()
        return
      }
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busyID = 0
    }
  }

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

    {#if accounts}
      <section class="block" aria-labelledby="admin-users">
        <h3 id="admin-users">Users</h3>
        {#if usersError}<p class="error" role="alert">{usersError}</p>{/if}
        <div class="invite">
          <label for="invite-email">Invite by email</label>
          <div class="invite-row">
            <input id="invite-email" bind:value={inviteEmail} type="email" placeholder="sam@example.com" />
            <label class="vh" for="invite-role">Invite role</label>
            <select id="invite-role" bind:value={inviteRole}>
              <option value="member">Member</option>
              <option value="viewer">Viewer</option>
              {#if role === 'owner'}<option value="admin">Admin</option>{/if}
            </select>
            <button type="button" class="btn" onclick={invite} disabled={busyID !== 0 || !inviteEmail.trim()}>Create invite</button>
          </div>
          {#if inviteLink}
            <div class="fresh">
              <p>Send this link. It expires {new Date(inviteExpires).toLocaleString()}.</p>
              <div class="tokenline">
                <code>{inviteLink}</code>
                <CopyButton text={inviteLink} />
              </div>
            </div>
          {/if}
        </div>

        {#if resetPassword}
          <div class="fresh">
            <p>Temporary password for {resetName}. Copy it now.</p>
            <div class="tokenline">
              <code>{resetPassword}</code>
              <CopyButton text={resetPassword} />
            </div>
          </div>
        {/if}

        <div class="tablewrap">
          <table>
            <thead><tr><th>User</th><th>Role</th><th>Last active</th><th>Tokens</th><th></th></tr></thead>
            <tbody>
              {#each users as user (user.id)}
                <tr class:disabled={user.disabledAt}>
                  <td>
                    <strong>{user.name}</strong>{#if user.email === currentEmail} <span class="you">you</span>{/if}
                    <small>{user.email}</small>
                  </td>
                  <td>
                    {#if canManage(user) && role === 'owner'}
                      <select value={user.role} onchange={(e) => saveRole(user, e.currentTarget.value)} disabled={busyID === user.id}>
                        {#each roleOptions(user) as option}
                          <option value={option} selected={option === user.role}>{option}</option>
                        {/each}
                      </select>
                    {:else}
                      <span>{user.role}</span>
                    {/if}
                  </td>
                  <td>{user.lastActive ? new Date(user.lastActive).toLocaleString() : 'never'}</td>
                  <td>{user.tokens}</td>
                  <td class="actions">
                    {#if canManage(user)}
                      <button type="button" class="btn quiet" onclick={() => toggleDisabled(user)} disabled={busyID === user.id}>
                        {user.disabledAt ? 'Enable' : 'Disable'}
                      </button>
                      <button type="button" class="btn quiet" onclick={() => resetPasswordFor(user)} disabled={busyID === user.id}>Reset password</button>
                      <button type="button" class="btn quiet" onclick={() => revokeTokensFor(user)} disabled={busyID === user.id}>Revoke tokens</button>
                      {#if role === 'owner'}
                        <button type="button" class="btn quiet" onclick={() => (confirmAction = { kind: 'transfer', user })} disabled={busyID === user.id}>Make owner</button>
                        <button type="button" class="btn quiet danger" onclick={() => (confirmAction = { kind: 'delete', user })} disabled={busyID === user.id}>Delete</button>
                      {/if}
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>
    {/if}

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
    {#if confirmAction}
      <ConfirmDialog
        title={confirmAction.kind === 'delete' ? `Delete ${confirmAction.user.name}?` : `Make ${confirmAction.user.name} the owner?`}
        body={confirmAction.kind === 'delete'
          ? 'Their shelf, tokens and sessions are deleted. Transfer ownership first if this is the owner.'
          : 'You become an admin and they become the owner. You can transfer it back later.'}
        confirmLabel={confirmAction.kind === 'delete' ? 'Delete' : 'Transfer'}
        danger={confirmAction.kind === 'delete'}
        onconfirm={runConfirmed}
        oncancel={() => (confirmAction = null)}
      />
    {/if}
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
  .error { margin: 8px 0 0; color: var(--bad); font-size: 0.85rem; }
  .invite { margin-top: 12px; }
  .invite > label { display: block; margin-bottom: 4px; color: var(--muted); font-size: 0.85rem; }
  .invite-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .invite-row input { flex: 1; min-width: 180px; }
  .fresh { margin-top: 12px; padding: 12px; border: 1px solid var(--accent); border-radius: 10px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.85rem; }
  .tokenline { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
  .tokenline code { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 0.8rem; }
  td small { display: block; color: var(--muted); font-size: 0.75rem; }
  .you { color: var(--muted); font-size: 0.75rem; font-weight: 500; }
  tr.disabled td { opacity: 0.55; }
  td.actions { white-space: nowrap; }
  td.actions .btn { padding-left: 6px; padding-right: 6px; margin-left: 2px; }
  td select { padding: 4px 8px; min-height: 32px; }
  @media (pointer: coarse) { td select { min-height: 44px; } }
  @media (max-width: 560px) {
    h2 { font-size: 1.7rem; }
    .cards { grid-template-columns: 1fr 1fr; }
  }
</style>
