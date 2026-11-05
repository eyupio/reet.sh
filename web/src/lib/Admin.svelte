<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import NavigationMenu from './NavigationMenu.svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import { ADMIN_USER_SCOPES, adminUserCountLabel, adminUserMatchesSearch, adminUserRoleCounts } from './adminUsers'
  import { teamUsageCountLabel, teamUsageMatchesSearch } from './adminTeam'
  import { groupLabel } from './machines'
  import PageHeader from './PageHeader.svelte'
  import {
    cacheSoftwareMirror, createInvite, deleteAdminUser, getAdminSso, getAdminStatus, getBranchPruneSetting, getRollup, getSoftwareMirror, getYoloSetting, listAdminUsers, resetUserPassword, revokeUserTokens,
    saveAdminSettings, setBranchPruneSetting, setSoftwareMirror, setYoloSetting, transferOwner, updateAdminUser, type AdminStatus, type BranchPruneSetting, type Event as AuditEvent, type Rollup, type RollupSkill, type SoftwareMirrorStatus, type UserMeta, type YoloSetting,
  } from './api'
  import AdminEmail from './AdminEmail.svelte'
  import AdminNotifications from './AdminNotifications.svelte'
  import AdminTickets from './AdminTickets.svelte'
  import Backups from './Backups.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { EVENT_CATEGORIES, eventCategory, eventLabel, eventTitle, type EventCategory } from './events'
  import { elapsed } from './time'
  import { adminRows, type AdminSso } from './sso'
  import { bytes } from './terminal/health'

  let {
    onerror, role = 'owner', accounts = false, currentEmail = '', supportDesk = false,
  }: { onerror?: (e: unknown) => void; role?: string; accounts?: boolean; currentEmail?: string; supportDesk?: boolean } = $props()

  // svelte-ignore state_referenced_locally (Account mode is fixed for this page.)
  const sections = [
    { key: 'overview', label: 'Overview' }, { key: 'environments', label: 'Environments' },
    ...(accounts ? [{ key: 'users', label: 'Users' }, { key: 'team', label: 'Team usage' }] : []),
    ...(supportDesk ? [{ key: 'support', label: 'Support' }] : []), { key: 'email', label: 'Email' }, { key: 'notifications', label: 'Notifications' }, { key: 'backups', label: 'Backups' }, { key: 'settings', label: 'Settings' }, { key: 'activity', label: 'Activity' },
  ]
  const legacy: Record<string, string> = { 'admin-overview': 'overview', 'admin-envs': 'environments', 'admin-users': 'users', 'admin-team': 'team', 'admin-backups': 'backups', 'admin-settings': 'settings', 'admin-events': 'activity' }
  const requested = legacy[location.hash.slice(1)] || location.pathname.split('/')[2] || 'overview'
  const section = sections.some(s => s.key === requested) ? requested : 'overview'
  const sectionTitle = sections.find(s => s.key === section)!.label
  const descriptions: Record<string, string> = {
    overview: 'Keep the place running reet. Check health, jobs and storage.',
    environments: 'Inspect scanned machines and agent reports, with server heartbeats kept separate.',
    users: 'Invite people and manage their access.',
    team: 'See which skills are earning their keep and where shared lists need attention.',
    support: 'Answer support tickets from people and teams, assign them and keep notes.',
    notifications: 'Set what staff and people are told by default, and for which staff roles.',
    email: 'Set the one mail server every message goes through, and watch the outbox.',
    backups: 'Protect the whole instance with encrypted backups.',
    settings: 'Configure background jobs, sharing and usage privacy.',
    activity: 'Recent changes across the instance.',
  }
  const isOwner = $derived(role === 'owner')

  let status = $state<AdminStatus | null>(null)
  const environmentHosts = $derived([...new Set((status?.environments ?? []).filter(env => !env.agent.startsWith('reet')).map(env => env.host))])
  const serverReports = $derived((status?.environments ?? []).filter(env => env.agent.startsWith('reet')).sort((a, b) => b.lastSeen - a.lastSeen))

  let error = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let saved = $state('')
  let form = $state({ refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: '' })
  let yolo = $state<YoloSetting | null>(null)
  let yoloSaving = $state(false)
  let yoloNotice = $state('')
  let branchPrune = $state<BranchPruneSetting | null>(null)
  let branchPruneSaving = $state(false)
  let branchPruneNotice = $state('')
  let mirror = $state<SoftwareMirrorStatus | null>(null)
  let mirrorSaving = $state(false)
  let mirrorCaching = $state(false)
  let mirrorNotice = $state('')
  let users = $state<UserMeta[]>([])
  let usersError = $state('')
  let usersLoading = $state(true)
  let userQuery = $state('')
  let userScope = $state<'all' | 'owner' | 'admin' | 'member' | 'viewer'>('all')
  const searchedUsers = $derived(users.filter(user => adminUserMatchesSearch(user, userQuery)))
  const userCounts = $derived(adminUserRoleCounts(searchedUsers))
  const shownUsers = $derived(searchedUsers.filter(user => userScope === 'all' || user.role === userScope))
  const userCountLabel = $derived(usersLoading ? 'Reading people…' : adminUserCountLabel(shownUsers.length, users.length))
  let ssoStatus = $state<AdminSso | null>(null)
  let inviteEmail = $state('')
  let inviteRole = $state('member')
  let inviteLink = $state('')
  let inviteExpires = $state(0)
  let inviteEmailed = $state(false)
  let resetPassword = $state('')
  let resetNotice = $state('')
  let resetName = $state('')
  let busyID = $state(0)
  let confirmAction = $state<{ kind: 'delete' | 'transfer'; user: UserMeta } | null>(null)
  let rollup = $state<Rollup | null>(null)
  let rollupError = $state('')
  let rollupLoading = $state(false)
  let rollupList = $state(0)
  let rollupTag = $state('')
  let teamQuery = $state('')
  let rollupSort = $state<'adoption' | 'name'>('adoption')
  let activitySearch = $state('')
  let activityScope = $state<'all' | EventCategory>('all')

  async function load() {
    loading = true
    try {
      const r = await getAdminStatus()
      status = r
      form = { ...r.settings }
      error = ''
      try { yolo = await getYoloSetting() } catch { yolo = null }
      try { branchPrune = await getBranchPruneSetting() } catch { branchPrune = null }
      try { mirror = await getSoftwareMirror() } catch { mirror = null }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  async function loadUsers() {
    if (!accounts) return
    usersLoading = true
    try {
      users = (await listAdminUsers()).users
      usersError = ''
    } catch (e) {
      usersError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      usersLoading = false
    }
  }

  // Read-only, and an addition: if it cannot be read the panel is simply not shown.
  async function loadSso() {
    if (!accounts) return
    try {
      ssoStatus = await getAdminSso()
    } catch {
      ssoStatus = null
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
      return teamUsageMatchesSearch(skill, teamQuery)
    })
    return [...rows].sort((a, b) => {
      if (rollupSort === 'name') return a.name.localeCompare(b.name)
      return b.usedBy - a.usedBy || a.name.localeCompare(b.name)
    })
  })

  const teamCount = $derived(teamUsageCountLabel(rollupRows.length, rollup?.skills.length ?? 0))
  const teamCountLabel = $derived(rollupLoading && !rollup ? 'Reading team usage…' : !rollup ? 'Team usage unavailable' : teamCount)
  const rollupSkillByID = $derived(new Map((rollup?.skills ?? []).map((skill) => [skill.id, skill] as const)))
  const unlistedSkills = $derived((rollup?.drift.unlisted ?? []).map((id) => rollupSkillByID.get(id)).filter((skill): skill is RollupSkill => Boolean(skill)))
  const majorityCount = $derived((rollup?.skills ?? []).filter((skill) => (rollup?.teamSize ?? 0) > 0 && skill.usedBy * 2 > (rollup?.teamSize ?? 0)).length)

  function clearTeamFilters() {
    teamQuery = ''
    rollupList = 0
    rollupTag = ''
    rollupSort = 'adoption'
  }

  const activityCategories = new Map(EVENT_CATEGORIES.map((category) => [category.id, category.label]))
  const activitySearched = $derived.by(() => {
    const term = activitySearch.trim().toLowerCase()
    return (status?.events ?? []).filter((event) => {
      if (!term) return true
      const category = activityCategories.get(eventCategory(event)) ?? 'Account'
      const haystack = `${eventLabel(event)} ${event.actor ?? ''} ${event.source} ${event.action} ${event.detail ?? ''} ${category}`.toLowerCase()
      return term.split(/\s+/).every((word) => haystack.includes(word))
    })
  })
  const activityCounts = $derived.by(() => {
    const counts: Record<'all' | EventCategory, number> = { all: activitySearched.length, agents: 0, projects: 0, skills: 0, lists: 0, account: 0, admin: 0 }
    for (const event of activitySearched) counts[eventCategory(event)]++
    return counts
  })
  const activityRows = $derived(activitySearched.filter((event) => activityScope === 'all' || eventCategory(event) === activityScope))
  const activityCountLabel = $derived(activityCount(activityRows.length, status?.events.length ?? 0))

  function activityCount(shown: number, total: number): string {
    if (total === 0) return 'No events'
    if (shown === total) return `${total} ${total === 1 ? 'event' : 'events'}`
    return `${shown} of ${total} events`
  }

  function activityCategoryLabel(event: AuditEvent): string {
    return activityCategories.get(eventCategory(event)) ?? 'Account'
  }

  function clearActivityFilters() {
    activitySearch = ''
    activityScope = 'all'
  }

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
        data.teamSize,
        skill.onShelf,
        skill.environmentLinks,
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
    history.replaceState(null, '', `/admin/${section}`)
    document.title = `${sectionTitle} · Admin · reet.sh`
    void load()
    if (section === 'users') { void loadUsers(); void loadSso() }
    if (section === 'team') void loadRollup()
    return () => {
      document.title = 'reet.sh – your whole AI setup, on every machine'
    }
  })

  function lastActiveLabel(user: UserMeta): string {
    if (!user.lastActive) return 'Never active'
    const age = elapsed(user.lastActive)
    return age === 'just now' ? 'Active just now' : `Active ${age} ago`
  }

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
    resetNotice = ''
    try {
      const r = await resetUserPassword(user.id)
      if (r.emailed) {
        resetNotice = `A reset link is on its way to ${r.email || user.email}.`
      } else {
        resetPassword = r.password || ''
        resetName = user.name
      }
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
      inviteEmailed = Boolean(r.emailed)
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
      saved = 'Sorted. Settings saved.'
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      saving = false
    }
  }

  async function saveBranchPrune(event: Event) {
    if (!branchPrune) return
    const input = event.currentTarget as HTMLInputElement
    const afterWorktree = input.checked
    branchPruneSaving = true
    branchPruneNotice = ''
    error = ''
    try {
      branchPrune = await setBranchPruneSetting(afterWorktree)
      branchPruneNotice = afterWorktree ? 'Merged local branches are pruned after a worktree is removed.' : 'Automatic branch pruning is off.'
    } catch (e) {
      input.checked = branchPrune.afterWorktree
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      branchPruneSaving = false
    }
  }

  async function saveYolo(event: Event) {
    if (!yolo) return
    const input = event.currentTarget as HTMLInputElement
    const allowed = input.checked
    yoloSaving = true
    yoloNotice = ''
    error = ''
    try {
      yolo = await setYoloSetting(allowed)
      yoloNotice = allowed ? 'YOLO is allowed for this account.' : 'YOLO is off for this account.'
    } catch (e) {
      input.checked = yolo.allowed
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      yoloSaving = false
    }
  }

  async function saveMirror(event: Event) {
    if (!mirror) return
    const input = event.currentTarget as HTMLInputElement
    const enabled = input.checked
    mirrorSaving = true
    mirrorNotice = ''
    error = ''
    try {
      mirror = await setSoftwareMirror(enabled)
      mirrorNotice = enabled ? 'Machines may fetch the reviewed artifact from this controller.' : 'The artifact mirror is off.'
    } catch (e) {
      input.checked = mirror.enabled
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      mirrorSaving = false
    }
  }

  async function cacheMirror() {
    if (!mirror || mirrorCaching) return
    mirrorCaching = true
    mirrorNotice = ''
    error = ''
    try {
      mirror = await cacheSoftwareMirror()
      mirrorNotice = 'The reviewed artifact is cached.'
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      mirrorCaching = false
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

{#snippet teamDetails(skill: RollupSkill)}
  {#if rollup?.individual}
    <details class="team-details">
      <summary>{usedMembers(skill).length} {usedMembers(skill).length === 1 ? 'person' : 'people'} · {environmentChips(skill).length} {environmentChips(skill).length === 1 ? 'agent report' : 'agent reports'}</summary>
      {#if usedMembers(skill).length}
        <div class="memberchips">{#each usedMembers(skill) as member (member.id)}<span class="memberchip">{member.name}</span>{/each}</div>
      {:else}<p class="muted">Nobody yet</p>{/if}
      <ul class="report-list">{#each environmentChips(skill) as chip}<li><strong>{chip.member}</strong><span>{chip.env}</span></li>{/each}</ul>
    </details>
  {:else}<span class="muted">Counts only</span>{/if}
{/snippet}

<div class="admin">
  <PageHeader eyebrow="Instance administration" title={sectionTitle} titleId="admin-title" lede={descriptions[section]} />
  <NavigationMenu label="Admin" items={sections.map(s => ({ label: s.label, href: `/admin/${s.key}`, current: section === s.key }))} />

  {#if loading}
    <Skeleton label="Loading the administration page…" shape="cards" count={3} />
  {:else if !status}
    <p class="notice bad" role="alert">{error || 'Could not load the admin status.'}</p>
  {:else}
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {#if section === 'overview'}
    <h2 class="vh">Instance overview</h2>
    <div class="cards" id="admin-overview">
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

    {/if}
    {#if section === 'environments'}
    <section class="block" aria-labelledby="admin-envs">
      <h2 id="admin-envs">Scanned machines</h2>
      <p class="muted">Agent inventories are grouped by machine. Server heartbeats are separate from skill scans.</p>
      {#if environmentHosts.length === 0}<p class="muted">No agent scans yet. Run <code>reet scan --env NAME</code> on a machine.</p>{/if}
      <div class="environment-machines">
        {#each environmentHosts as host (host)}
          <details class="machine-group">
            <summary><strong>{host}</strong><span>{status.environments.filter(env => env.host === host && !env.agent.startsWith('reet')).length} install locations</span></summary>
            <ul>{#each status.environments.filter(env => env.host === host && !env.agent.startsWith('reet')) as env (env.id)}
              <li><div><strong>{groupLabel(env)}</strong><span>{env.skillCount ?? 0} skills · Last seen {when(String(env.lastSeen))}</span></div><code>{env.path || 'No path reported'}</code></li>
            {/each}</ul>
          </details>
        {/each}
      </div>
      {#if serverReports.length}
        <details class="server-reports"><summary>Server heartbeats <span>{serverReports.length} reports</span></summary>
          <p class="muted">These record running servers, not installed skills. Container restarts may create new host IDs.</p>
          <ul>{#each serverReports as env (env.id)}<li><code>{env.host}</code><span>Last seen {when(String(env.lastSeen))}</span></li>{/each}</ul>
        </details>
      {/if}
    </section>
    {/if}

    {#if accounts && section === 'users' && ssoStatus?.enabled}
      <section class="block" aria-labelledby="sso-title">
        <h2 id="sso-title">Single sign-on</h2>
        <p class="muted">How people can sign in through your identity provider. This is set where the server is configured, so it is shown here and not changed here.</p>
        <dl class="ssorows">
          {#each adminRows(ssoStatus) as row (row.label)}
            <div><dt>{row.label}</dt><dd class:bad={row.label === 'Provider' && !ssoStatus.reachable}>{row.value}</dd></div>
          {/each}
        </dl>
      </section>
    {/if}

    {#if accounts && section === 'users'}
      <section class="block" aria-labelledby="admin-invite">
        <h2 id="admin-invite">Invite someone</h2>
        <p class="muted">Send a link yourself, or let the instance email it when a mail server is set.</p>
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
              <p>{inviteEmailed ? `Invitation queued for email. You can also send this link.` : `Send this link.`} It expires {new Date(inviteExpires).toLocaleString()}.</p>
              <div class="tokenline">
                <code>{inviteLink}</code>
                <CopyButton text={inviteLink} />
              </div>
            </div>
          {/if}
        </div>

        {#if resetNotice}
          <div class="fresh"><p>{resetNotice}</p></div>
        {/if}
        {#if resetPassword}
          <div class="fresh">
            <p>Temporary password for {resetName}. Copy it now.</p>
            <div class="tokenline">
              <code>{resetPassword}</code>
              <CopyButton text={resetPassword} />
            </div>
          </div>
        {/if}
      </section>

      {#if usersError}<p class="error" role="alert">{usersError} <button type="button" class="btn quiet" onclick={loadUsers}>Try again</button></p>{/if}
      <CollectionPanel label="Users" count={userCountLabel}>
        {#snippet actions()}
          <button type="button" class="btn quiet" onclick={loadUsers} disabled={usersLoading}>Refresh</button>
        {/snippet}
        {#snippet toolbar()}
          <div class="user-toolbar">
            <label class="user-search">Search people
              <input type="search" bind:value={userQuery} placeholder="Name, email or role" />
            </label>
            <div class="user-scopes" role="group" aria-label="Show people">
              {#each ADMIN_USER_SCOPES as item (item.id)}
                <button type="button" class="scope" aria-pressed={userScope === item.id} onclick={() => (userScope = item.id)}>{item.label}<small>{userCounts[item.id]}</small></button>
              {/each}
            </div>
          </div>
        {/snippet}

        {#if usersLoading}
          <Skeleton label="Reading people…" shape="lines" count={4} />
        {:else if shownUsers.length === 0}
          <div class="state-block">
            <h2>{users.length === 0 ? 'No people yet.' : 'No person matches that search or role.'}</h2>
            <p>{users.length === 0 ? 'Invite someone above and they appear here.' : 'Try another search or show everyone.'}</p>
            {#if users.length > 0}<button type="button" class="btn" onclick={() => { userQuery = ''; userScope = 'all' }}>Clear filters</button>{/if}
          </div>
        {:else}
          <div class="user-list">
            {#each shownUsers as user (user.id)}
              <div class="user-row" class:disabled={user.disabledAt}>
                <div class="user-identity">
                  <strong>{user.name}{#if user.email === currentEmail} <span class="you">you</span>{/if}</strong>
                  <small>{user.email}</small>
                </div>
                <div class="user-meta">
                  {#if !(canManage(user) && role === 'owner')}<span class="role-badge">{user.role}</span>{/if}
                  <span>{user.tokens} {user.tokens === 1 ? 'token' : 'tokens'}</span>
                  <span title={user.lastActive ? new Date(user.lastActive).toLocaleString() : undefined}>{lastActiveLabel(user)}</span>
                  {#if user.disabledAt}<span class="disabled-badge">Disabled</span>{/if}
                </div>
                <div class="user-tools">
                  {#if canManage(user) && role === 'owner'}
                    <select aria-label={`Instance role for ${user.name}`} value={user.role} onchange={(e) => saveRole(user, e.currentTarget.value)} disabled={busyID === user.id}>
                      {#each roleOptions(user) as option}
                        <option value={option} selected={option === user.role}>{option}</option>
                      {/each}
                    </select>
                  {/if}
                  {#if canManage(user)}
                    <details class="user-actions"><summary>Manage {user.name}</summary><div>
                      <button type="button" class="btn quiet" onclick={() => toggleDisabled(user)} disabled={busyID === user.id}>
                        {user.disabledAt ? 'Enable' : 'Disable'}
                      </button>
                      <button type="button" class="btn quiet" onclick={() => resetPasswordFor(user)} disabled={busyID === user.id}>Reset password</button>
                      <button type="button" class="btn quiet" onclick={() => revokeTokensFor(user)} disabled={busyID === user.id}>Revoke tokens</button>
                      {#if role === 'owner'}
                        <button type="button" class="btn quiet" onclick={() => (confirmAction = { kind: 'transfer', user })} disabled={busyID === user.id}>Make owner</button>
                        <button type="button" class="btn quiet danger" onclick={() => (confirmAction = { kind: 'delete', user })} disabled={busyID === user.id}>Delete</button>
                      {/if}
                    </div></details>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </CollectionPanel>

    {/if}

    {#if section === 'team'}
      <section aria-labelledby="admin-team">
        <h2 id="admin-team" class="vh">Skill adoption</h2>
        <CollectionPanel label="Team usage" count={teamCountLabel}>
          {#snippet actions()}
            <button type="button" class="btn quiet" onclick={downloadCSV} disabled={rollupRows.length === 0}>Download CSV</button>
            <button type="button" class="btn quiet" onclick={loadRollup} disabled={rollupLoading}>Refresh</button>
          {/snippet}
          {#snippet toolbar()}
            {#if rollup}
              <div class="team-summary" role="group" aria-label="Team usage summary">
                <span><strong>{rollup.teamSize}</strong> {rollup.teamSize === 1 ? 'person' : 'people'}</span>
                <span><strong>{majorityCount}</strong> used by more than half</span>
                <span><strong>{unlistedSkills.length}</strong> in use, on no list</span>
                <span><strong>{rollup.drift.unused.length}</strong> list skills unused</span>
              </div>
            {/if}
            {#if rollup && !rollup.individual}<p class="privacy-note">Per-person rows are hidden by the owner. This view shows counts only.</p>{/if}
            <div class="team-toolbar">
              <label class="team-search">Search skills
                <input type="search" bind:value={teamQuery} placeholder="Name, source, list or tag" />
              </label>
              <div class="team-filters">
                <label>List<select aria-label="List" bind:value={rollupList}><option value={0}>All lists</option>{#each rollup?.lists ?? [] as list (list.id)}<option value={list.id}>{list.name}</option>{/each}</select></label>
                <label>Tag<select aria-label="Tag" bind:value={rollupTag}><option value="">All tags</option>{#each rollupTags as tag}<option value={tag}>{tag}</option>{/each}</select></label>
                <label>Sort<select aria-label="Sort" bind:value={rollupSort}><option value="adoption">Adoption</option><option value="name">Name</option></select></label>
              </div>
            </div>
          {/snippet}

          {#if rollupError && !rollup}
            <div class="state-block error" role="alert"><p>{rollupError}</p><button type="button" class="btn" onclick={loadRollup}>Try again</button></div>
          {:else if rollupLoading && !rollup}
            <Skeleton label="Reading team usage…" shape="lines" count={4} />
          {:else if !rollup}
            <div class="state-block"><h2>The rollup is not available yet.</h2><p>Refresh to read it again.</p></div>
          {:else}
            {#if rollupError}<p class="error" role="alert">{rollupError}</p>{/if}
            {#if rollupRows.length === 0}
              <div class="state-block">
                <h2>No skill matches those filters.</h2>
                <p>Try another search, or clear the list and tag filters.</p>
                <button type="button" class="btn" onclick={clearTeamFilters}>Clear filters</button>
              </div>
            {:else}
              <div class="usage-list">
                {#each rollupRows as skill (skill.id)}
                  <div class="usage-row">
                    <div class="usage-identity">
                      <strong>{skill.name}</strong>
                      <code>{skill.source || 'Local skill'}</code>
                      {#if skill.lists?.length}<div class="listchips">{#each skill.lists as list (list.id)}<a class="listchip" href={`/lists/${list.id}`}>{list.name}</a>{/each}</div>{/if}
                      {#if skill.tags?.length}<div class="tagchips">{#each skill.tags as tag}<span class="tagchip">{tag}</span>{/each}</div>{/if}
                    </div>
                    <div class="usage-metrics">
                      <div class="adoption"><span class="metric-label">In use</span><strong>{skill.usedBy} / {rollup.teamSize}</strong><span class="bar" aria-hidden="true"><i style={`width:${rollup.teamSize ? Math.round((skill.usedBy / rollup.teamSize) * 100) : 0}%`}></i></span></div>
                      <div><span class="metric-label">Saved by</span><strong>{skill.onShelf}</strong></div>
                      <div><span class="metric-label">Agent reports</span><strong>{skill.environmentLinks}</strong></div>
                    </div>
                    <div class="usage-team">{@render teamDetails(skill)}</div>
                  </div>
                {/each}
              </div>
            {/if}
          {/if}
        </CollectionPanel>

        {#if rollup}
          <details class="additional-usage"><summary>List gaps & people</summary>
          <div class="drift">
            <div class="driftcol">
              <h4>In use, on no list <small>{unlistedSkills.length}</small></h4>
              {#if unlistedSkills.length === 0}
                <p class="muted">Every skill in use is on a list.</p>
              {:else}
                <ul>
                  {#each unlistedSkills as skill (skill.id)}
                    <li><strong>{skill.name}</strong><small>{skill.usedBy} of {rollup.teamSize} use it</small>{#if rollup.individual && usedMembers(skill).length}<span>{usedMembers(skill).map((member) => member.name).join(', ')}</span>{/if}</li>
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
            <h4>People <small>{rollup.teamSize}</small></h4>
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
              <p class="muted">{rollup.teamSize} active people; {rollup.nothingInstalled === 1 ? '1 has' : `${rollup.nothingInstalled} have`} nothing installed.</p>
            {/if}
          </div>
          </details>
        {/if}
      </section>

    {/if}
    {#if section === 'support'}
    <div id="admin-support">
      <AdminTickets {onerror} />
    </div>
    {/if}
    {#if section === 'notifications'}
    <div id="admin-notifications">
      <AdminNotifications {role} {onerror} />
    </div>
    {/if}
    {#if section === 'email'}
    <div id="admin-email">
      <AdminEmail {role} {onerror} />
    </div>
    {/if}
    {#if section === 'backups'}
    <div id="admin-backups">
      <Backups {onerror} {role} admin />
    </div>

    {/if}
    {#if section === 'settings'}
      {#if isOwner}
        <CollectionPanel label="Instance settings" count="6 settings">
          {#snippet actions()}
            <button type="submit" form="admin-settings-form" class="btn primary" disabled={saving} aria-busy={saving}>{saving ? 'Saving…' : 'Save settings'}</button>
          {/snippet}
          {#snippet toolbar()}
            <p class="note">These apply to the whole instance. The owner and administrators can still choose their own notification and approval defaults.</p>
          {/snippet}
          <form id="admin-settings-form" class="instance-settings" onsubmit={save}>
            <fieldset>
              <legend>Background jobs</legend>
              <label class="check"><input type="checkbox" bind:checked={form.refreshEnabled} /> Six-hourly metadata refresh</label>
              <label class="check"><input type="checkbox" bind:checked={form.topicsEnabled} /> Daily skills.sh topic seed</label>
              <label class="field">
                <span>Prune empty reet-server reports older than (days)</span>
                <input type="number" min="0" max="3650" bind:value={form.pruneServerEnvironmentsDays} placeholder="0 = off" />
              </label>
            </fieldset>
            <fieldset>
              <legend>Sharing and identity</legend>
              <label class="check"><input type="checkbox" bind:checked={form.linkListsEnabled} /> Let list owners share read-only links</label>
              <label class="check"><input type="checkbox" bind:checked={form.rollupIndividual} /> Show per-person usage in the team rollup</label>
              <label class="field">
                <span>Instance name</span>
                <input bind:value={form.instanceName} placeholder="Team shelf" autocomplete="off" />
              </label>
            </fieldset>
            {#if saved}<p class="saved" role="status">{saved}</p>{/if}
          </form>
        </CollectionPanel>
      {:else}
        <section class="block" aria-labelledby="admin-settings">
          <h2 id="admin-settings">Instance settings</h2>
          <p class="muted">Only the owner can change these settings.</p>
          <dl class="readonly-settings">
            <div><dt>Six-hourly metadata refresh</dt><dd>{form.refreshEnabled ? 'On' : 'Off'}</dd></div>
            <div><dt>Daily skills.sh topic seed</dt><dd>{form.topicsEnabled ? 'On' : 'Off'}</dd></div>
            <div><dt>Read-only list links</dt><dd>{form.linkListsEnabled ? 'On' : 'Off'}</dd></div>
            <div><dt>Per-person rollup rows</dt><dd>{form.rollupIndividual ? 'On' : 'Off'}</dd></div>
            <div><dt>Empty reet-server reports</dt><dd>{form.pruneServerEnvironmentsDays > 0 ? `Pruned after ${form.pruneServerEnvironmentsDays} days` : 'Kept'}</dd></div>
            <div><dt>Instance name</dt><dd>{form.instanceName || 'Not set'}</dd></div>
          </dl>
        </section>
      {/if}
    {/if}
    {#if section === 'activity'}
    <CollectionPanel label="Recent activity" count={activityCountLabel}>
      {#snippet toolbar()}
        <div class="activity-toolbar">
          <label class="activity-search">Search activity
            <input type="search" bind:value={activitySearch} placeholder="What happened, who or area" />
          </label>
          <div class="activity-scopes" role="group" aria-label="Show activity">
            <button type="button" class="scope" aria-pressed={activityScope === 'all'} onclick={() => (activityScope = 'all')}>All<small>{activityCounts.all}</small></button>
            {#each EVENT_CATEGORIES as category}
              <button type="button" class="scope" aria-pressed={activityScope === category.id} onclick={() => (activityScope = category.id)}>{category.label}<small>{activityCounts[category.id]}</small></button>
            {/each}
          </div>
        </div>
        <p class="activity-note">The newest 100 events are kept here.</p>
      {/snippet}

      {#if status.events.length === 0}
        <div class="state-block">
          <h2>Nothing recorded yet.</h2>
          <p>Changes around this instance appear here.</p>
        </div>
      {:else if activityRows.length === 0}
        <div class="state-block">
          <h2>No event matches that search or area.</h2>
          <p>Try another search, or show every event.</p>
          <button type="button" class="btn" onclick={clearActivityFilters}>Clear filters</button>
        </div>
      {:else}
        <ul class="events">
          {#each activityRows as event (event.id)}
            <li title={eventTitle(event)}>
              <span class="event-main">{eventLabel(event)}</span>
              <span class="event-meta"><span class="event-cat">{activityCategoryLabel(event)}</span><time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time></span>
            </li>
          {/each}
        </ul>
      {/if}
    </CollectionPanel>
    {/if}
    {#if section === 'settings'}
    {#if (role === 'owner' || role === 'admin') && yolo}
    <section class="block" aria-labelledby="admin-yolo">
      <h2 id="admin-yolo">Approvals</h2>
      <p class="muted">Lets people start a session where the agent doesn't ask before acting. Each session still needs its own confirmation.</p>
      <label class="check"><input type="checkbox" checked={yolo.allowed} disabled={yoloSaving} onchange={saveYolo} /> Allow YOLO</label>
      <p class="muted" role="status">{yoloNotice || (yolo.changedBy ? `Last changed by ${yolo.changedBy}.` : 'Off unless an owner or admin allows it.')}</p>
    </section>
    {/if}
    {#if (role === 'owner' || role === 'admin') && branchPrune}
    <section class="block" aria-labelledby="admin-branch-prune">
      <h2 id="admin-branch-prune">Repository maintenance</h2>
      <p class="muted">After a worktree is removed, prune the local branches in its repository whose work is already merged. Only the proven-safe set is deleted: never unmerged work, the current or default branch, or one a worktree has checked out. Remote branches are left alone, and each run is recorded in Activity.</p>
      <label class="check"><input type="checkbox" checked={branchPrune.afterWorktree} disabled={branchPruneSaving} onchange={saveBranchPrune} /> Prune merged branches after a worktree is removed</label>
      <p class="muted" role="status">{branchPruneNotice || (branchPrune.changedBy ? `Last changed by ${branchPrune.changedBy}.` : 'Off unless an owner or admin turns it on.')}</p>
    </section>
    {/if}
    {#if (role === 'owner' || role === 'admin') && mirror}
    <section class="block" aria-labelledby="admin-software-mirror">
      <h2 id="admin-software-mirror">Software artifact mirror</h2>
      <p class="muted">Lets machines that cannot reach registry.npmjs.org download the one reviewed OpenCode artifact from this controller. Every machine still verifies the pinned checksum before it installs anything.</p>
      <label class="check"><input type="checkbox" checked={mirror.enabled} disabled={mirrorSaving || mirrorCaching} onchange={saveMirror} /> Serve the reviewed artifact to machines</label>
      <p class="muted" role="status">{mirrorNotice || (mirror.cached ? `Cached ${mirror.version ?? 'the reviewed version'} (${bytes(mirror.size ?? 0)}).` : 'Not cached yet.')}</p>
      {#if !mirror.cached}
        <button class="btn" type="button" disabled={mirrorCaching || mirrorSaving} onclick={() => void cacheMirror()}>{mirrorCaching ? 'Caching…' : 'Cache the reviewed artifact now'}</button>
      {/if}
    </section>
    {/if}
    {/if}
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
</div>

<style>
  .ssorows { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px 24px; margin: 16px 0 0; }
  .ssorows div { min-width: 0; }
  .ssorows dt { color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .ssorows dd { margin: 2px 0 0; overflow-wrap: anywhere; }
  .ssorows dd.bad { color: var(--bad); }
  .admin { padding-top: 4px; }
  h3, #admin-overview, #admin-backups { scroll-margin-top: 24px; }
  .block h2 { margin: 0 0 12px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 10px; margin-top: 22px; }
  .cards article { padding: 22px; background: var(--card); border: 1px solid var(--line); border-radius: 12px; }
  .cards h3 {
    margin: 0; color: var(--muted); font-size: 0.875rem; font-weight: 700;
    letter-spacing: 0.1em; text-transform: uppercase;
  }
  .cards p { margin: 6px 0 0; font: 500 1.7rem/1.2 var(--serif); }
  .cards p.ok { color: var(--good); }
  .cards p.bad { color: var(--bad); }
  .cards small { display: block; margin-top: 4px; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .stages { display: flex; flex-wrap: wrap; gap: 8px 18px; margin-top: 14px; color: var(--muted); font-size: 0.9rem; }
  .stages strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .block { margin-top: 24px; padding: clamp(16px, 3vw, 24px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); min-width: 0; }
  .muted { margin: 8px 0 0; color: var(--muted); }

  form { display: grid; gap: 10px; margin-top: 10px; max-width: 460px; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 0.9rem; }
  .check input { min-height: 0; width: 18px; height: 18px; accent-color: var(--accent); }
  .field { display: grid; gap: 4px; font-size: 0.9rem; color: var(--muted); }
  .field input { width: 100%; }
  .saved { color: var(--good); font-size: 0.875rem; margin: 0; }
  .note { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .instance-settings { max-width: none; margin-top: 0; gap: 14px; }
  .instance-settings fieldset { margin: 0; padding: 14px; border: 1px solid var(--line); border-radius: 10px; min-width: 0; }
  .instance-settings legend { padding: 0 5px; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .instance-settings .check { min-height: 32px; }
  .instance-settings .field { max-width: 24rem; }
  .readonly-settings { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px 24px; margin: 14px 0 0; }
  .readonly-settings dt { color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .readonly-settings dd { margin: 2px 0 0; overflow-wrap: anywhere; }
  .activity-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .activity-search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .activity-search input { width: 100%; }
  .activity-scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .activity-note { margin: 10px 0 0; color: var(--muted); font-size: 0.875rem; }
  .events { list-style: none; margin: 0; padding: 0; display: grid; }
  .events li { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: start; gap: 12px; padding: 12px 2px; border-top: 1px solid var(--line-soft); color: var(--ink); font-size: 0.875rem; }
  .events li:first-child { border-top: 0; }
  .event-main { min-width: 0; overflow-wrap: anywhere; line-height: 1.45; }
  .event-meta { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 0.875rem; }
  .event-cat { padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; white-space: nowrap; }
  .events time { font-variant-numeric: tabular-nums; white-space: nowrap; }
  .error { margin: 8px 0 0; color: var(--bad); font-size: 0.875rem; }
  .invite { margin-top: 12px; }
  .invite > label { display: block; margin-bottom: 4px; color: var(--muted); font-size: 0.875rem; }
  .invite-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .invite-row input { flex: 1; min-width: 180px; }
  .fresh { margin-top: 12px; padding: 12px; border: 1px solid var(--accent); border-radius: 10px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.875rem; }
  .tokenline { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
  .tokenline code { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 0.875rem; }
  .you { margin-left: 4px; color: var(--muted); font-size: 0.875rem; font-weight: 500; }
  .user-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .user-search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .user-search input { width: 100%; }
  .user-scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .user-list { display: grid; }
  .user-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 18px; align-items: start; padding: 14px 2px; border-top: 1px solid var(--line-soft); }
  .user-row:first-child { border-top: 0; }
  .user-row.disabled { opacity: 0.65; }
  .user-identity { min-width: 0; }
  .user-identity strong { display: block; overflow-wrap: anywhere; }
  .user-identity small { display: block; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .user-meta { grid-column: 1; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; color: var(--muted); font-size: 0.875rem; }
  .role-badge, .disabled-badge { border: 1px solid var(--line); border-radius: 999px; padding: 2px 8px; color: var(--ink); font-size: 0.875rem; }
  .disabled-badge { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); }
  .user-tools { grid-column: 2; grid-row: 1 / span 2; display: flex; flex-direction: column; align-items: flex-end; gap: 6px; }
  .user-tools select { width: auto; }
  .user-actions { min-width: 140px; }
  .user-actions > summary { cursor: pointer; min-height: 44px; padding: 8px 0; font-size: 0.875rem; }
  .user-actions > div { display: grid; gap: 6px; padding-top: 8px; }
  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h2 { font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; margin: 0; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  @media (max-width: 650px) { .user-row { grid-template-columns: 1fr; } .user-tools { grid-column: 1; grid-row: auto; align-items: flex-start; } .user-search { flex-basis: 100%; } .activity-search { flex-basis: 100%; } .events li { grid-template-columns: 1fr; gap: 4px; } .event-meta { justify-content: flex-start; } }
  @media (pointer: coarse) { .scope, .user-search input, .activity-search input, .instance-settings .check { min-height: 44px; } }
  @media (min-width: 900px) { .instance-settings { grid-template-columns: 1fr 1fr; align-items: start; } .instance-settings .saved { grid-column: 1 / -1; } }
  @media (prefers-reduced-motion: no-preference) { .scope, .user-row { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }

  .team-summary { display: flex; flex-wrap: wrap; gap: 6px 16px; margin-bottom: 12px; color: var(--muted); font-size: 0.875rem; }
  .team-summary strong { color: var(--ink); font-variant-numeric: tabular-nums; }
  .privacy-note { margin: 0 0 12px; color: var(--muted); font-size: 0.875rem; }
  .team-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .team-search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .team-search input { width: 100%; }
  .team-filters { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 10px; }
  .team-filters label { display: grid; gap: 4px; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .team-filters select { min-height: 36px; padding: 6px 10px; }
  .usage-list { display: grid; }
  .usage-row { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr); gap: 10px 20px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .usage-row:first-child { border-top: 0; }
  .usage-identity { min-width: 0; }
  .usage-identity strong { display: block; font-size: 0.95rem; overflow-wrap: anywhere; }
  .usage-identity code { display: block; margin-top: 2px; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .usage-identity .listchips { margin-top: 6px; }
  .tagchips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
  .tagchip { padding: 2px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.875rem; }
  .usage-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; align-content: start; }
  .usage-metrics > div { display: grid; gap: 2px; align-content: start; }
  .metric-label { color: var(--muted); font-size: 0.875rem; }
  .usage-metrics strong { font-size: 0.9rem; font-variant-numeric: tabular-nums; }
  .adoption .bar { margin-top: 4px; }
  .usage-team { grid-column: 2; border-top: 1px solid var(--line-soft); padding-top: 6px; }
  .listchips, .memberchips { display: flex; flex-wrap: wrap; gap: 4px; }
  .listchip {
    display: inline-flex; padding: 2px 7px; border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--line));
    border-radius: 6px; background: color-mix(in srgb, var(--tint) 25%, var(--card)); color: var(--accent-text);
    font-size: 0.875rem; font-weight: 600; text-decoration: none;
  }
  .listchip:hover { border-color: var(--accent); text-decoration: underline; text-underline-offset: 2px; }
  .memberchip { padding: 2px 7px; border: 1px solid color-mix(in srgb, var(--good) 30%, var(--line)); border-radius: 999px; color: var(--good); font-size: 0.875rem; }
  .adoption { display: grid; gap: 4px; min-width: 72px; }
  .bar { display: block; height: 4px; min-width: 64px; border-radius: 999px; background: var(--line); overflow: hidden; }
  .bar i { display: block; height: 100%; border-radius: inherit; background: var(--accent); }
  .drift { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; margin-top: 22px; }
  .driftcol h4, .people-summary h4 { margin: 0; font-size: 0.875rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
  .driftcol h4 small, .people-summary h4 small { margin-left: 5px; color: var(--muted); font-weight: 500; }
  .driftcol ul { list-style: none; margin: 8px 0 0; padding: 0; }
  .driftcol li { display: grid; gap: 1px; padding: 7px 0; border-bottom: 1px solid var(--line); font-size: 0.875rem; }
  .driftcol li strong { font-weight: 600; overflow-wrap: anywhere; }
  .driftcol li small { color: var(--muted); }
  .driftcol li span { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .people-summary { margin-top: 22px; }
  .peoplegrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 8px; margin-top: 8px; }
  .peoplegrid article { padding: 9px 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .peoplegrid article.empty-person { border-color: color-mix(in srgb, var(--bad) 32%, var(--line)); }
  .peoplegrid strong { display: block; font-size: 0.875rem; overflow-wrap: anywhere; }
  .peoplegrid small { display: block; color: var(--muted); font-size: 0.875rem; }
  .peoplegrid span { display: block; margin-top: 3px; color: var(--muted); font-size: 0.875rem; }
  @media (max-width: 720px) {
    .drift { grid-template-columns: 1fr; gap: 22px; }
  }
  @media (max-width: 650px) {
    .team-search { flex-basis: 100%; }
    .usage-row { grid-template-columns: 1fr; }
    .usage-team { grid-column: 1; }
  }
  @media (pointer: coarse) { .team-search input, .team-filters select { min-height: 44px; } }
  @media (max-width: 560px) {
    .cards article { padding: 16px; }
    .cards { grid-template-columns: 1fr 1fr; }
    .machine-group li { grid-template-columns: minmax(0, 1fr); gap: 4px; }
  }
  .environment-machines { display: grid; gap: 10px; margin-top: 16px; }
  .machine-group, .server-reports { border: 1px solid var(--line); border-radius: 10px; padding: 12px; min-width: 0; }
  .machine-group summary, .server-reports summary { cursor: pointer; overflow-wrap: anywhere; min-height: 24px; }
  .machine-group summary span, .server-reports summary span { margin-left: 10px; color: var(--muted); font-size: 0.875rem; }
  .machine-group ul, .server-reports ul, .report-list { list-style: none; padding: 0; margin: 10px 0 0; }
  /* minmax(0, 1fr): a plain 1fr column is at least as wide as its longest unbreakable word, so a long scope path widened the page. */
  .machine-group li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; padding: 12px 0; border-top: 1px solid var(--line); }
  .machine-group li strong { overflow-wrap: anywhere; }
  .machine-group li span, .server-reports li span { display: block; color: var(--muted); font-size: 0.875rem; margin-top: 4px; }
  .machine-group code, .server-reports code { overflow-wrap: anywhere; font-size: 0.875rem; }
  .server-reports { margin-top: 20px; }
  .server-reports li { padding: 10px 0; border-top: 1px solid var(--line); }
  .team-details summary, .additional-usage > summary { cursor: pointer; min-height: 36px; font-size: 0.875rem; color: var(--ink); padding: 8px 0; }
  .team-details .memberchips { margin-top: 8px; }
  .report-list li { display: grid; gap: 3px; padding: 8px 0; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
  .report-list strong { font-size: 0.875rem; font-weight: 500; }
  .report-list span { font-size: 0.875rem; color: var(--muted); }
  .additional-usage { border-top: 1px solid var(--line); margin-top: 20px; }
</style>
