<script lang="ts">
  import { onMount } from 'svelte'
  import {
    createInvite, deleteAdminUser, getAdminStatus, getRollup, listAdminUsers, resetUserPassword, revokeUserTokens,
    saveAdminSettings, transferOwner, updateAdminUser, type AdminStatus, type Rollup, type RollupSkill, type UserMeta,
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
  let form = $state({ refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, instanceName: '' })
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
  let rollup = $state<Rollup | null>(null)
  let rollupError = $state('')
  let rollupLoading = $state(false)
  let rollupList = $state(0)
  let rollupTag = $state('')
  let rollupSort = $state<'adoption' | 'name'>('adoption')

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

  async function loadRollup() {
    if (!accounts) return
    rollupLoading = true
    try {
      rollup = await getRollup()
      rollupError = ''
    } catch (e) {
      rollupError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      rollupLoading = false
    }
  }

  const rollupTags = $derived.by(() => {
    const names = new Set<string>()
    for (const skill of rollup?.skills ?? []) for (const tag of skill.tags ?? []) names.add(tag)
    return [...names].sort((a, b) => a.localeCompare(b))
  })

  const rollupRows = $derived.by(() => {
    const rows = (rollup?.skills ?? []).filter((skill) => {
      if (rollupList !== 0 && !(skill.lists ?? []).some((list) => list.id === rollupList)) return false
      if (rollupTag && !(skill.tags ?? []).includes(rollupTag)) return false
      return true
    })
    return [...rows].sort((a, b) => {
      if (rollupSort === 'name') return a.name.localeCompare(b.name)
      return b.usedBy - a.usedBy || a.name.localeCompare(b.name)
    })
  })

  const rollupSkillByID = $derived(new Map((rollup?.skills ?? []).map((skill) => [skill.id, skill] as const)))
  const unlistedSkills = $derived((rollup?.drift.unlisted ?? []).map((id) => rollupSkillByID.get(id)).filter((skill): skill is RollupSkill => Boolean(skill)))
  const majorityCount = $derived((rollup?.skills ?? []).filter((skill) => (rollup?.members.length ?? 0) > 0 && skill.usedBy * 2 > (rollup?.members.length ?? 0)).length)
  const nothingInstalledCount = $derived((rollup?.members ?? []).filter((member) => member.installed === 0).length)

  function usedMembers(skill: RollupSkill) {
    return (skill.members ?? []).filter((member) => member.used)
  }

  function environmentChips(skill: RollupSkill) {
    return (skill.members ?? []).flatMap((member) => (member.environments ?? []).map((env) => ({ member: member.name, env })))
  }

  function csvCell(value: string | number): string {
    const text = String(value)
    return /[",\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
  }

  function downloadCSV() {
    const data = rollup
    if (!data) return
    const header = ['skill', 'source', 'lists', 'used_by', 'team_size', 'on_shelf', 'machines', 'used_members', 'environments']
    const rows = rollupRows.map((skill) => {
      const envs = environmentChips(skill).map(({ member, env }) => `${member} on ${env}`).join('; ')
      return [
        skill.name,
        skill.source,
        (skill.lists ?? []).map((list) => list.name).join('; '),
        skill.usedBy,
        data.members.length,
        skill.onShelf,
        skill.environmentLinks,
        data.members.length,
        usedMembers(skill).map((member) => member.name).join('; '),
        envs,
      ]
    })
    const csv = [header, ...rows].map((row) => row.map(csvCell).join(',')).join('\n')
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = 'reet-team-rollup.csv'
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  onMount(() => {
    document.title = 'Admin · reet.sh'
    void load()
    void loadUsers()
    void loadRollup()
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
      await loadRollup()
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

      <section class="block" aria-labelledby="admin-team">
        <h3 id="admin-team">Team rollup</h3>
        <p class="muted">What the team uses, what sits on a list unused, and where a skill is in use outside a list. Active accounts only.</p>
        {#if rollupError}<p class="error" role="alert">{rollupError}</p>{/if}
        {#if rollupLoading && !rollup}
          <p class="muted">Loading rollup…</p>
        {:else if rollup}
          <div class="rollup-summary">
            <span><strong>{rollup.members.length}</strong> people</span>
            <span><strong>{rollup.skills.length}</strong> skills</span>
            <span><strong>{majorityCount}</strong> used by more than half</span>
            <span><strong>{unlistedSkills.length}</strong> in use, on no list</span>
            <span><strong>{rollup.drift.unused.length}</strong> list skills unused</span>
          </div>

          <div class="rollup-filters">
            <label>List
              <select bind:value={rollupList}>
                <option value={0}>All lists</option>
                {#each rollup.lists as list (list.id)}<option value={list.id}>{list.name}</option>{/each}
              </select>
            </label>
            <label>Tag
              <select bind:value={rollupTag}>
                <option value="">All tags</option>
                {#each rollupTags as tag}<option value={tag}>{tag}</option>{/each}
              </select>
            </label>
            <label>Sort
              <select bind:value={rollupSort}>
                <option value="adoption">Adoption</option>
                <option value="name">Name</option>
              </select>
            </label>
            <button type="button" class="btn" onclick={downloadCSV} disabled={rollupRows.length === 0}>Download CSV</button>
          </div>

          {#if !rollup.individual}
            <p class="privacy-note">Per-person rows are hidden by the owner. The table shows counts only.</p>
          {/if}

          {#if rollupRows.length === 0}
            <p class="muted">Nothing matches those filters.</p>
          {:else}
            <div class="tablewrap">
              <table class="rollup-table">
                <thead><tr><th>Skill</th><th>Lists</th><th>In use</th><th>On shelf</th><th>Machines</th><th>Team</th></tr></thead>
                <tbody>
                  {#each rollupRows as skill (skill.id)}
                    <tr>
                      <td class="skillcell"><strong>{skill.name}</strong><small><code>{skill.source}</code></small></td>
                      <td>
                        {#if skill.lists?.length}
                          <div class="listchips">
                            {#each skill.lists as list (list.id)}<a href={`/lists/${list.id}`} class="listchip">{list.name}</a>{/each}
                          </div>
                        {:else}<span class="muted">—</span>{/if}
                      </td>
                      <td class="num">
                        <div class="adoption">
                          <span>{skill.usedBy} / {rollup.members.length}</span>
                          <span class="bar" aria-hidden="true"><i style={`width:${rollup.members.length ? Math.round((skill.usedBy / rollup.members.length) * 100) : 0}%`}></i></span>
                        </div>
                      </td>
                      <td class="num">{skill.onShelf}</td>
                      <td class="num">{skill.environmentLinks}</td>
                      <td class="teamcell">
                        {#if rollup.individual}
                          {#if usedMembers(skill).length}
                            <div class="memberchips">{#each usedMembers(skill) as member (member.id)}<span class="memberchip" title={member.environments?.join(', ') || 'No environment yet'}>{member.name}</span>{/each}</div>
                          {:else}<span class="muted">Nobody yet</span>{/if}
                          {#if environmentChips(skill).length}
                            <div class="envchips">{#each environmentChips(skill) as chip}<span class="envchip">{chip.member.split(' ')[0]} · {chip.env}</span>{/each}</div>
                          {/if}
                        {:else}<span class="muted">Counts only</span>{/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <div class="drift">
            <div class="driftcol">
              <h4>In use, on no list <small>{unlistedSkills.length}</small></h4>
              {#if unlistedSkills.length === 0}
                <p class="muted">Every skill in use is on a list.</p>
              {:else}
                <ul>
                  {#each unlistedSkills as skill (skill.id)}
                    <li><strong>{skill.name}</strong><small>{skill.usedBy} of {rollup.members.length} use it</small>{#if rollup.individual && usedMembers(skill).length}<span>{usedMembers(skill).map((member) => member.name).join(', ')}</span>{/if}</li>
                  {/each}
                </ul>
              {/if}
            </div>
            <div class="driftcol">
              <h4>On a list, nobody uses <small>{rollup.drift.unused.length}</small></h4>
              {#if rollup.drift.unused.length === 0}
                <p class="muted">Every listed skill is in use by someone.</p>
              {:else}
                <ul>
                  {#each rollup.drift.unused as item (`${item.listId}-${item.skillId}`)}
                    <li><strong>{item.name}</strong><small>{item.listName}</small><span>{item.source}</span></li>
                  {/each}
                </ul>
              {/if}
            </div>
          </div>

          <div class="people-summary">
            <h4>People <small>{rollup.members.length}</small></h4>
            {#if rollup.individual}
              <div class="peoplegrid">
                {#each rollup.members as member (member.id)}
                  <article class:empty-person={member.installed === 0}>
                    <strong>{member.name}</strong>
                    <small>{member.role}{#if member.email === currentEmail} · you{/if}</small>
                    <span>{member.used} in use · {member.installed} installed</span>
                  </article>
                {/each}
              </div>
            {:else}
              <p class="muted">{rollup.members.length} active people; {nothingInstalledCount === 1 ? '1 has' : `${nothingInstalledCount} have`} nothing installed.</p>
            {/if}
          </div>
        {:else}
          <p class="muted">The rollup is not available yet.</p>
        {/if}
      </section>

    <section class="block" aria-labelledby="admin-settings">
      <h3 id="admin-settings">Jobs and instance</h3>
      {#if isOwner}
        <form onsubmit={save}>
          <label class="check"><input type="checkbox" bind:checked={form.refreshEnabled} /> Six-hourly metadata refresh</label>
          <label class="check"><input type="checkbox" bind:checked={form.topicsEnabled} /> Daily skills.sh topic seed</label>
          <label class="check"><input type="checkbox" bind:checked={form.linkListsEnabled} /> Let list owners share read-only links</label>
          <label class="check"><input type="checkbox" bind:checked={form.rollupIndividual} /> Show per-person usage in the team rollup</label>
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
        <p class="muted">Only the owner can change these settings. Refresh is {form.refreshEnabled ? 'on' : 'off'}; the topic seed is {form.topicsEnabled ? 'on' : 'off'}; link sharing is {form.linkListsEnabled ? 'on' : 'off'}; per-person rollup rows are {form.rollupIndividual ? 'on' : 'off'}.</p>
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

  .rollup-summary { display: flex; flex-wrap: wrap; gap: 6px 16px; margin-top: 10px; color: var(--muted); font-size: 0.84rem; }
  .rollup-summary strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .rollup-filters { display: flex; flex-wrap: wrap; gap: 10px; align-items: flex-end; margin-top: 14px; }
  .rollup-filters label { display: grid; gap: 4px; color: var(--muted); font-size: 0.76rem; font-weight: 600; }
  .rollup-filters select { min-height: 36px; padding: 6px 10px; }
  .privacy-note { margin: 12px 0 0; color: var(--muted); font-size: 0.8rem; }
  .rollup-table { margin-top: 10px; min-width: 760px; }
  .rollup-table .skillcell { min-width: 170px; }
  .rollup-table .skillcell strong { display: block; overflow-wrap: anywhere; }
  .rollup-table .muted { margin: 0; }
  .listchips, .memberchips, .envchips { display: flex; flex-wrap: wrap; gap: 4px; }
  .listchip {
    display: inline-flex; padding: 2px 7px; border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--line));
    border-radius: 6px; background: color-mix(in srgb, var(--tint) 25%, var(--card)); color: var(--accent);
    font-size: 0.72rem; font-weight: 600; text-decoration: none;
  }
  .listchip:hover { border-color: var(--accent); text-decoration: underline; text-underline-offset: 2px; }
  .memberchip { padding: 2px 7px; border: 1px solid color-mix(in srgb, var(--good) 30%, var(--line)); border-radius: 999px; color: var(--good); font-size: 0.7rem; }
  .envchip { padding: 2px 6px; border: 1px solid var(--line); border-radius: 5px; background: var(--bg); color: var(--muted); font-size: 0.68rem; }
  .envchips { margin-top: 4px; }
  .teamcell { min-width: 190px; max-width: 320px; }
  .adoption { display: grid; gap: 4px; min-width: 72px; }
  .bar { display: block; height: 4px; min-width: 64px; border-radius: 999px; background: var(--line); overflow: hidden; }
  .bar i { display: block; height: 100%; border-radius: inherit; background: var(--accent); }
  .drift { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; margin-top: 22px; }
  .driftcol h4, .people-summary h4 { margin: 0; font-size: 0.78rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
  .driftcol h4 small, .people-summary h4 small { margin-left: 5px; color: var(--muted); font-weight: 500; }
  .driftcol ul { list-style: none; margin: 8px 0 0; padding: 0; }
  .driftcol li { display: grid; gap: 1px; padding: 7px 0; border-bottom: 1px solid var(--line); font-size: 0.82rem; }
  .driftcol li strong { font-weight: 600; overflow-wrap: anywhere; }
  .driftcol li small { color: var(--muted); }
  .driftcol li span { color: var(--muted); font-size: 0.76rem; overflow-wrap: anywhere; }
  .people-summary { margin-top: 22px; }
  .peoplegrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 8px; margin-top: 8px; }
  .peoplegrid article { padding: 9px 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .peoplegrid article.empty-person { border-color: color-mix(in srgb, var(--bad) 32%, var(--line)); }
  .peoplegrid strong { display: block; font-size: 0.84rem; overflow-wrap: anywhere; }
  .peoplegrid small { display: block; color: var(--muted); font-size: 0.72rem; }
  .peoplegrid span { display: block; margin-top: 3px; color: var(--muted); font-size: 0.72rem; }
  @media (max-width: 720px) {
    .drift { grid-template-columns: 1fr; gap: 22px; }
  }
  @media (max-width: 560px) {
    h2 { font-size: 1.7rem; }
    .cards { grid-template-columns: 1fr 1fr; }
    .rollup-filters { display: grid; grid-template-columns: 1fr 1fr; }
    .rollup-filters label:nth-child(3) { grid-column: 1; }
    .rollup-filters .btn { grid-column: 2; align-self: end; }
    .rollup-table { min-width: 700px; }
  }
</style>
