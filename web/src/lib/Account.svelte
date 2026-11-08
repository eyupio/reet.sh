<script lang="ts">
  import { gafferAnimations, gafferCompact } from './gafferMotion'
  import { onMount } from 'svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import Skeleton from './Skeleton.svelte'
  import ActivitySection from './ActivitySection.svelte'
  import MaintenanceSection from './MaintenanceSection.svelte'
  import AgentsSettings from './AgentsSettings.svelte'
  import GafferSettings from './GafferSettings.svelte'
  import Backups from './Backups.svelte'
  import CopyButton from './CopyButton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import EnvironmentSync from './EnvironmentSync.svelte'
  import MCPAccess from './MCPAccess.svelte'
  import PageHeader from './PageHeader.svelte'
  import {
    applyTheme, changePassword, createToken, endOtherProfileSessions, endProfileSession, getProfile,
    getSsoIdentity, listProfileSessions, listTokens, revokeToken, saveProfile, THEME_EVENT, unlinkSsoIdentity,
    type APIToken, type Profile, type ProfileSession, type User,
  } from './api'
  import { linkedLine, ssoLinkHref, type SsoIdentity } from './sso'

  let {
    user, onsignout, onerror, onuserchange, writer,
  }: { user: User; onsignout: () => void; onerror?: (e: unknown) => void; onuserchange?: (u: User) => void; writer?: boolean } = $props()

  type Section = 'machines' | 'sync' | 'profile' | 'agents' | 'gaffer' | 'security' | 'sso' | 'sessions' | 'tokens' | 'mcp' | 'backups' | 'maintenance' | 'activity'
  // svelte-ignore state_referenced_locally
  let who = $state<User>(user)
  let sso = $state<SsoIdentity | null>(null)
  let ssoUnlinkAsk = $state(false)
  const canWrite = $derived(who.role !== 'viewer')
  const sections: { key: Section; label: string }[] = $derived([
    { key: 'profile', label: 'Profile' },
    { key: 'agents', label: 'Agents' },
    ...(canWrite ? [{ key: 'gaffer' as const, label: 'Gaffer AI' }] : []),
    { key: 'security', label: 'Password' },
    ...(sso?.enabled ? [{ key: 'sso' as const, label: 'Single sign-on' }] : []),
    { key: 'sessions', label: 'Sessions' },
    ...(canWrite ? [{ key: 'tokens' as const, label: 'API tokens' }] : []),
    { key: 'mcp', label: 'MCP access' },
    { key: 'backups', label: 'Backups' },
    ...(canWrite ? [{ key: 'maintenance' as const, label: 'Maintenance' }] : []),
    { key: 'activity', label: 'Activity' },
    { key: 'machines', label: 'Machines' },
    { key: 'sync', label: 'Compare & sync' },
  ])

  const startConnect = location.hash === '#connect-machine'
  let section = $state<Section>(sectionFromHash())
  let profile = $state<Profile>({ theme: 'system', timezone: '', dateFormat: 'locale', shelfView: 'list', cardsExpanded: false, hostedPrefix: 'reet-', updatedAt: 0 })
  let sessions = $state<ProfileSession[]>([])
  let sessionsLoading = $state(false)
  let sessionSearch = $state('')
  let sessionScope = $state<'all' | 'current' | 'other'>('all')
  let tokens = $state<APIToken[]>([])
  let tokensLoading = $state(false)
  let tokenSearch = $state('')
  let tokenScope = $state<'all' | 'used' | 'never'>('all')
  let freshToken = $state('')
  let tokenName = $state('scan token')
  let freshTokenId = $state(0)
  let revokePending = $state<APIToken | null>(null)
  let error = $state('')
  let notice = $state('')
  let busy = $state('')

  // svelte-ignore state_referenced_locally
  let profileForm = $state({ name: user.name, theme: 'system', timezone: '', dateFormat: 'locale', shelfView: 'list' as 'list' | 'board', cardsExpanded: false, terminalRemoveOnClose: false, gafferAnimations: true, gafferCompact: true })
  let prefixEnabled = $state(true)
  let prefixValue = $state('reet-')
  let passwordForm = $state({ current: '', password: '', confirm: '' })

  function sectionFromHash(): Section {
    // /activity is the old address for the account's activity section; it keeps working and is rewritten below.
    const fromPath = location.pathname === '/activity' ? 'activity' : location.pathname.split('/')[2] || ''
    const h = location.hash.slice(1) || fromPath || 'profile'
    if (h === 'connect-machine') return 'machines'
    return ['profile', 'agents', 'gaffer', 'security', 'sso', 'sessions', 'machines', 'sync', 'tokens', 'mcp', 'backups', 'maintenance', 'activity'].includes(h) ? h as Section : 'profile'
  }

  onMount(() => {
    history.replaceState(null, '', `/account/${section}`)
    document.title = `${sections.find(s => s.key === section)?.label ?? 'Account'} · Account · reet.sh`
    void loadProfile()
    void loadSso()
    if (section === 'sessions') void loadSessions()
    if (section === 'tokens') void loadTokens()
    // The account menu can change the theme while this form is open; saving another preference must not put the old one back.
    const followTheme = (e: unknown) => { profileForm.theme = (e as CustomEvent<string>).detail }
    window.addEventListener(THEME_EVENT, followTheme)
    return () => window.removeEventListener(THEME_EVENT, followTheme)
  })

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  // Single sign-on is an addition: an instance without it answers {enabled: false}, and a failure here hides the section
  // rather than the page.
  async function loadSso() {
    try {
      sso = await getSsoIdentity()
    } catch {
      sso = null
    }
  }

  async function unlinkSso() {
    ssoUnlinkAsk = false
    busy = 'sso'
    error = ''
    try {
      await unlinkSsoIdentity()
      await loadSso()
      notice = 'Your identity is unlinked.'
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = ''
    }
  }

  async function loadProfile() {
    try {
      const r = await getProfile()
      who = r.user
      profile = r.profile
      profileForm = { name: r.user.name, theme: r.profile.theme, timezone: r.profile.timezone, dateFormat: r.profile.dateFormat || 'locale', shelfView: r.profile.shelfView, cardsExpanded: r.profile.cardsExpanded, terminalRemoveOnClose: r.profile.terminalRemoveOnClose??false, gafferAnimations: r.profile.gafferAnimations ?? true, gafferCompact: r.profile.gafferCompact ?? true }
      prefixEnabled = r.profile.hostedPrefix !== ''
      prefixValue = r.profile.hostedPrefix || 'reet-'
      applyTheme(r.profile.theme)
      gafferAnimations.set(r.profile.gafferAnimations ?? true); gafferCompact.set(r.profile.gafferCompact ?? true)
      error = ''
    } catch (e) {
      fail(e)
    }
  }

  async function loadSessions() {
    sessionsLoading = true
    try {
      sessions = (await listProfileSessions()).sessions
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      sessionsLoading = false
    }
  }

  async function loadTokens() {
    if (!canWrite) return
    tokensLoading = true
    try {
      tokens = (await listTokens()).tokens
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      tokensLoading = false
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = 'profile'
    notice = ''
    try {
      const { name, ...preferences } = profileForm
      const hostedPrefix = prefixEnabled ? (prefixValue.trim() || 'reet-') : ''
      const r = await saveProfile({ ...(canWrite ? profileForm : preferences), hostedPrefix })
      who = r.user
      profile = r.profile
      prefixEnabled = r.profile.hostedPrefix !== ''
      prefixValue = r.profile.hostedPrefix || 'reet-'
      onuserchange?.(r.user)
      applyTheme(r.profile.theme)
      gafferAnimations.set(r.profile.gafferAnimations ?? true); gafferCompact.set(r.profile.gafferCompact ?? true)
      notice = 'Reet, defaults saved. Your preferences follow you to every browser you sign in from.'
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  async function change(e: SubmitEvent) {
    e.preventDefault()
    notice = ''
    if (passwordForm.password !== passwordForm.confirm) {
      error = 'The new passwords do not match.'
      return
    }
    busy = 'password'
    try {
      await changePassword(passwordForm.current, passwordForm.password)
      passwordForm = { current: '', password: '', confirm: '' }
      notice = 'Password changed. Every other session was signed out.'
      error = ''
      await loadSessions()
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  async function endSession(id: string) {
    busy = `session:${id}`
    try {
      await endProfileSession(id)
      await loadSessions()
      notice = 'That session is signed out.'
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  async function endOthers() {
    busy = 'others'
    try {
      const r = await endOtherProfileSessions()
      await loadSessions()
      notice = r.ended === 1 ? 'One other session is signed out.' : `${r.ended} other sessions are signed out.`
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  async function makeToken(e: SubmitEvent) {
    e.preventDefault()
    if (busy) return
    busy = 'token'
    freshToken = ''
    try {
      const r = await createToken(tokenName.trim() || 'scan token')
      freshToken = r.token
      freshTokenId = r.id
      tokenName = 'scan token'
      await loadTokens()
      notice = ''
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  async function revoke(id: number) {
    revokePending = null
    if (busy) return
    busy = 'revoke'
    try {
      await revokeToken(id)
      if (id === freshTokenId) { freshToken = ''; freshTokenId = 0 }
      await loadTokens()
    } catch (e) {
      fail(e)
    } finally {
      busy = ''
    }
  }

  function initials(): string {
    const words = (who.name || who.email.split('@')[0] || '?').split(/[^\p{L}\p{N}]+/u).filter(Boolean)
    return ((words[0]?.[0] ?? '') + (words.length > 1 ? (words[words.length - 1]?.[0] ?? '') : '')).toUpperCase() || '?'
  }

  function when(ms?: number): string {
    if (!ms) return 'never'
    const opts: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' }
    const tz = profile.timezone || profileForm.timezone
    if (tz) opts.timeZone = tz
    try {
      const d = new Date(ms)
      if (profile.dateFormat === 'dmy' || profileForm.dateFormat === 'dmy') return d.toLocaleString('en-GB', opts)
      if (profile.dateFormat === 'mdy' || profileForm.dateFormat === 'mdy') return d.toLocaleString('en-US', opts)
      if (profile.dateFormat === 'ymd' || profileForm.dateFormat === 'ymd') return d.toLocaleString('sv-SE', opts)
      return d.toLocaleString(undefined, opts)
    } catch {
      return new Date(ms).toLocaleString()
    }
  }

  function device(ua: string): string {
    if (!ua) return 'A browser signed in before sessions were described'
    const browser = ua.match(/(Firefox|Edg|Chrome|Safari)\/[\d.]+/)
    const os = ua.match(/\((?:[^)]*?;\s*)?(Windows|Macintosh|Linux|Android|iPhone|iPad)[^)]*\)/)
    return `${browser?.[1] ?? 'Browser'} on ${os?.[1] ?? 'an unknown device'}`
  }

  const searchedSessions = $derived.by(() => {
    const term = sessionSearch.trim().toLowerCase()
    return sessions.filter((session) => {
      if (!term) return true
      const haystack = `${device(session.device)} ${session.current ? 'this browser current' : 'other browser'}`.toLowerCase()
      return term.split(/\s+/).every((word) => haystack.includes(word))
    })
  })
  const sessionCounts = $derived({
    all: searchedSessions.length,
    current: searchedSessions.filter((session) => session.current).length,
    other: searchedSessions.filter((session) => !session.current).length,
  })
  const visibleSessions = $derived(searchedSessions.filter((session) => sessionScope === 'all' || (sessionScope === 'current' ? session.current : !session.current)))
  const sessionCountLabel = $derived(sessionCount(visibleSessions.length, sessions.length))

  function sessionCount(shown: number, total: number): string {
    if (total === 0) return 'No sessions'
    if (shown === total) return `${total} ${total === 1 ? 'session' : 'sessions'}`
    return `${shown} of ${total} sessions`
  }

  function clearSessionFilters() {
    sessionSearch = ''
    sessionScope = 'all'
  }

  const searchedTokens = $derived.by(() => {
    const term = tokenSearch.trim().toLowerCase()
    return tokens.filter((token) => {
      if (!term) return true
      return term.split(/\s+/).every((word) => token.name.toLowerCase().includes(word))
    })
  })
  const tokenCounts = $derived({
    all: searchedTokens.length,
    used: searchedTokens.filter((token) => token.lastUsedAt).length,
    never: searchedTokens.filter((token) => !token.lastUsedAt).length,
  })
  const visibleTokens = $derived(searchedTokens.filter((token) => tokenScope === 'all' || (tokenScope === 'used' ? Boolean(token.lastUsedAt) : !token.lastUsedAt)))
  const tokenCountLabel = $derived(tokenCount(visibleTokens.length, tokens.length))

  function tokenCount(shown: number, total: number): string {
    if (total === 0) return 'No tokens'
    if (shown === total) return `${total} ${total === 1 ? 'token' : 'tokens'}`
    return `${shown} of ${total} tokens`
  }

  function clearTokenFilters() {
    tokenSearch = ''
    tokenScope = 'all'
  }

  function sampleDate(): string {
    const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short', year: 'numeric' }
    if (profileForm.timezone) opts.timeZone = profileForm.timezone
    const d = new Date()
    try {
      if (profileForm.dateFormat === 'dmy') return d.toLocaleDateString('en-GB', opts)
      if (profileForm.dateFormat === 'mdy') return d.toLocaleDateString('en-US', opts)
      if (profileForm.dateFormat === 'ymd') return d.toLocaleDateString('sv-SE', opts)
      return d.toLocaleDateString(undefined, opts)
    } catch {
      return d.toLocaleDateString(undefined, opts)
    }
  }
</script>

{#snippet tokenForm()}
  {#if canWrite}
    <form onsubmit={makeToken} aria-busy={busy === 'token'}>
      <label for="token-name">Token name</label>
      <div class="createrow"><input id="token-name" bind:value={tokenName} placeholder="e.g. work laptop" autocomplete="off" required disabled={!!busy} /><button type="submit" class="btn primary" disabled={!!busy}>{busy === 'token' ? 'Creating…' : 'Create token'}</button></div>
    </form>
    {#if freshToken}<div class="fresh" role="status"><p>Reet, token created. Copy it now; it disappears when you leave this page.</p><div class="token-secret"><code>{freshToken}</code><CopyButton text={freshToken} label="Copy token" /></div></div>{/if}
  {/if}
{/snippet}

<section class="account" aria-labelledby="account-title">
  <div class="idcard">
    <span class="avatar" aria-hidden="true">{initials()}</span>
    <PageHeader group="Account" title={who.name || who.email} titleId="account-title" lede={`${who.email} · ${who.role}`} sectionsLabel="Account" sections={sections.map(s => ({ label: s.label, href: s.key === 'machines' ? '/machines' : s.key === 'sync' ? '/machines?view=compare' : `/account/${s.key}`, current: section === s.key }))} />
  </div>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}

  {#if section === 'profile'}
    <CollectionPanel label="Your preferences" count="10 preferences">
      {#snippet actions()}
        <button type="submit" form="profile-form" class="btn primary" disabled={busy === 'profile'}>{busy === 'profile' ? 'Saving…' : 'Save profile'}</button>
      {/snippet}
      {#snippet toolbar()}
        <p class="note">Make your workspace feel just reet. These preferences follow you to every browser you sign in from.</p>
      {/snippet}
      <form id="profile-form" class="profile-form" onsubmit={save}>
        <fieldset>
          <legend>Identity</legend>
          <div class="grid2">
            <label class="wide"><span>Name</span>
              <input bind:value={profileForm.name} autocomplete="name" maxlength="100" placeholder="Your name" disabled={!canWrite} />
              <small>Shown on lists and in the team rollup. Leave it empty to be shown by your address.</small>
            </label>
          </div>
        </fieldset>
        <fieldset>
          <legend>Appearance</legend>
          <div class="grid2">
            <label><span>Theme</span>
              <select bind:value={profileForm.theme}>
                <option value="system">Match this device</option>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
              </select>
            </label>
            <label><span>Gaffer animation</span>
              <select aria-label="Gaffer animation" bind:value={profileForm.gafferAnimations}>
                <option value={true}>On — occasional gestures and reactions</option>
                <option value={false}>Off — keep Gaffer still</option>
              </select>
              <small>Gaffer, quirky quotes, live updates and executive summaries stay visible. Your device’s reduced-motion setting is always respected.</small>
            </label>
            <label><span>Gaffer progress card</span>
              <select aria-label="Gaffer progress card" bind:value={profileForm.gafferCompact}>
                <option value={true}>One line — status and Gaffer’s quip</option>
                <option value={false}>Full card — portrait, handover and job sheet</option>
              </select>
              <small>Sets how each card starts. Every card has its own button to switch, and Gaffer keeps his Yorkshire narrative either way.</small>
            </label>
            <label><span>Time zone</span>
              <input bind:value={profileForm.timezone} placeholder="Europe/London" autocomplete="off" />
              <small>An IANA name. Empty follows this device.</small>
            </label>
            <label><span>Dates</span>
              <select bind:value={profileForm.dateFormat}>
                <option value="locale">This device's order</option>
                <option value="dmy">Day, month, year</option>
                <option value="mdy">Month, day, year</option>
                <option value="ymd">Year, month, day</option>
              </select>
              <small>Today reads {sampleDate()}.</small>
            </label>
          </div>
        </fieldset>
        <fieldset>
          <legend>Shelf and terminal</legend>
          <div class="grid2">
            <label class="wide"><span>When stopping a terminal session</span>
              <select aria-label="Terminal history default" bind:value={profileForm.terminalRemoveOnClose}><option value={false}>Keep the closed session in history</option><option value={true}>Remove from history after it ends</option></select>
              <small>This sets the confirmation’s default. You can change it for each session. Running or pending sessions remain visible until the machine confirms closure.</small>
            </label>
            <label><span>Default shelf layout</span>
              <select bind:value={profileForm.shelfView}><option value="list">List</option><option value="board">Board</option></select>
              <small>Switching List/Board on your shelf also saves this preference.</small>
            </label>
            <label><span>Default skill cards</span>
              <select bind:value={profileForm.cardsExpanded}><option value={false}>Collapsed — quick actions and core data</option><option value={true}>Expanded — all details</option></select>
              <small>Individual cards and Expand all / Collapse all can override this during your visit.</small>
            </label>
          </div>
        </fieldset>
        <fieldset>
          <legend>Publishing</legend>
          <div class="grid2">
            <div class="wide field"><span>Your skill name prefix</span>
              <div class="prefix-row">
                <span class="prefix-toggle"><input id="hosted-prefix-toggle" type="checkbox" bind:checked={prefixEnabled} /><label for="hosted-prefix-toggle">Prefix new and renamed skills</label></span>
                <input class="prefix-input" bind:value={prefixValue} placeholder="reet-" maxlength="33" disabled={!prefixEnabled} aria-label="Your skill name prefix" />
              </div>
              <small>Reet adds this to skills you host so they stand out from imports. Type a custom prefix such as <code>acme-</code>, or turn it off to use the bare skill name. Existing names are not changed automatically.</small>
            </div>
          </div>
        </fieldset>
      </form>
    </CollectionPanel>
  {:else if section === 'agents'}
    <AgentsSettings {canWrite} onerror={onerror} />
  {:else if section === 'security'}
    <CollectionPanel label="Password" count="3 fields">
      {#snippet actions()}
        <button type="submit" form="password-form" class="btn primary" disabled={busy === 'password'}>{busy === 'password' ? 'Changing…' : 'Change password'}</button>
      {/snippet}
      {#snippet toolbar()}
        <p class="note">Changing it signs out every other browser; this one stays signed in.</p>
      {/snippet}
      <form id="password-form" class="password-form" onsubmit={change}>
        <div class="grid2">
          <label class="wide"><span>Current password</span>
            <input type="password" bind:value={passwordForm.current} autocomplete="current-password" />
          </label>
          <label><span>New password</span>
            <input type="password" bind:value={passwordForm.password} autocomplete="new-password" minlength="8" />
            <small>At least 8 characters.</small>
          </label>
          <label><span>New password again</span>
            <input type="password" bind:value={passwordForm.confirm} autocomplete="new-password" minlength="8" />
          </label>
        </div>
      </form>
    </CollectionPanel>
  {:else if section === 'sso'}
    <section class="block" aria-labelledby="sso-title">
      <h2 id="sso-title">Single sign-on</h2>
      {#if !sso?.enabled}
        <p class="muted">Single sign-on is not set up on this instance.</p>
      {:else if sso.linked}
        <p class="muted">{linkedLine(sso, ms => new Date(ms).toLocaleDateString(undefined, { dateStyle: 'medium' }))}</p>
        <p class="muted">You can sign in here with your identity provider{sso.hasPassword ? ' or with your password' : ''}.</p>
        {#if !sso.canUnlink}
          <p class="muted">Set a password before unlinking, or ask an admin: unlinking now would leave you no way to sign in.</p>
        {/if}
        <div class="formacts">
          <button type="button" class="btn" disabled={!sso.canUnlink || busy === 'sso'} onclick={() => (ssoUnlinkAsk = true)}>Unlink</button>
        </div>
      {:else}
        <p class="muted">Link your identity provider to this account so you can sign in with it. You will be sent there to confirm, and brought back here.</p>
        <div class="formacts">
          <a class="btn primary" href={ssoLinkHref()}>Link your identity</a>
        </div>
      {/if}
    </section>
  {:else if section === 'sessions'}
    <CollectionPanel label="Your sessions" count={sessionCountLabel}>
      {#snippet actions()}
        <button type="button" class="btn quiet" onclick={loadSessions} disabled={sessionsLoading}>Refresh</button>
        <button type="button" class="btn" onclick={endOthers} disabled={busy === 'others' || !sessions.some((session) => !session.current)}>
          {busy === 'others' ? 'Ending…' : 'Sign out the others'}
        </button>
      {/snippet}
      {#snippet toolbar()}
        <div class="session-toolbar">
          <label class="session-search">Search sessions
            <input type="search" bind:value={sessionSearch} placeholder="Browser or device" />
          </label>
          <div class="session-scopes" role="group" aria-label="Show sessions">
            <button type="button" class="scope" aria-pressed={sessionScope === 'all'} onclick={() => (sessionScope = 'all')}>All<small>{sessionCounts.all}</small></button>
            <button type="button" class="scope" aria-pressed={sessionScope === 'current'} onclick={() => (sessionScope = 'current')}>This browser<small>{sessionCounts.current}</small></button>
            <button type="button" class="scope" aria-pressed={sessionScope === 'other'} onclick={() => (sessionScope = 'other')}>Other browsers<small>{sessionCounts.other}</small></button>
          </div>
        </div>
      {/snippet}

      {#if sessionsLoading}
        <Skeleton label="Reading sessions…" shape="lines" count={3} />
      {:else if sessions.length === 0}
        <div class="state-block">
          <h2>No sessions listed.</h2>
          <p>Browsers signed in to your account appear here.</p>
        </div>
      {:else if visibleSessions.length === 0}
        <div class="state-block">
          <h2>No session matches that search or filter.</h2>
          <p>Try another search, or show every browser.</p>
          <button type="button" class="btn" onclick={clearSessionFilters}>Clear filters</button>
        </div>
      {:else}
        <ul class="session-list">
          {#each visibleSessions as session (session.id)}
            <li>
              <div class="session-main">
                <strong>{device(session.device)}</strong>
                {#if session.current}<span class="tag">this browser</span>{/if}
                <small>Last seen {when(session.lastSeen)} · signed in {when(session.createdAt)}</small>
              </div>
              {#if !session.current}
                <div class="session-action">
                  <button type="button" class="btn quiet danger" onclick={() => endSession(session.id)} disabled={busy === `session:${session.id}`}>Sign out</button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </CollectionPanel>
  {:else if section === 'maintenance'}
    <MaintenanceSection admin={who.role === 'owner' || who.role === 'admin'} onsignedout={onerror} />
  {:else if section === 'activity'}
    <ActivitySection writer={writer ?? canWrite} onsignedout={onerror} timeZone={profile.timezone} dateFormat={profile.dateFormat} />
  {:else if section === 'machines'}
    <EnvironmentSync {canWrite} accounts token={freshToken} {startConnect} onsignedout={onerror} view="machines">
      {#snippet tokenSetup()}{@render tokenForm()}{/snippet}
    </EnvironmentSync>
  {:else if section === 'sync'}
    <EnvironmentSync {canWrite} view="compare" />
  {:else if section === 'tokens'}
    {#if !canWrite}
      <p class="muted">Viewer accounts cannot create API tokens.</p>
    {:else}
      <section class="block" aria-labelledby="create-token-title">
        <h2 id="create-token-title">Create a token</h2>
        <p class="muted">Use a token with <code>reet scan --token …</code> or an API client. A token is shown once, then only its name remains.</p>
        {@render tokenForm()}
      </section>

      <CollectionPanel label="Personal API tokens" count={tokenCountLabel}>
        {#snippet actions()}
          <button type="button" class="btn quiet" onclick={loadTokens} disabled={tokensLoading}>Refresh</button>
        {/snippet}
        {#snippet toolbar()}
          <div class="token-toolbar">
            <label class="token-search">Search tokens
              <input type="search" bind:value={tokenSearch} placeholder="Token name" />
            </label>
            <div class="token-scopes" role="group" aria-label="Show tokens">
              <button type="button" class="scope" aria-pressed={tokenScope === 'all'} onclick={() => (tokenScope = 'all')}>All<small>{tokenCounts.all}</small></button>
              <button type="button" class="scope" aria-pressed={tokenScope === 'used'} onclick={() => (tokenScope = 'used')}>Used<small>{tokenCounts.used}</small></button>
              <button type="button" class="scope" aria-pressed={tokenScope === 'never'} onclick={() => (tokenScope = 'never')}>Never used<small>{tokenCounts.never}</small></button>
            </div>
          </div>
        {/snippet}

        {#if tokensLoading}
          <Skeleton label="Reading tokens…" shape="lines" count={3} />
        {:else if tokens.length === 0}
          <div class="state-block">
            <h2>No API tokens yet.</h2>
            <p>Tokens you create for scanners and API clients appear here.</p>
          </div>
        {:else if visibleTokens.length === 0}
          <div class="state-block">
            <h2>No token matches that search or filter.</h2>
            <p>Try another search, or show every token.</p>
            <button type="button" class="btn" onclick={clearTokenFilters}>Clear filters</button>
          </div>
        {:else}
          <ul class="token-list">
            {#each visibleTokens as token (token.id)}
              <li>
                <div class="token-main">
                  <strong>{token.name}</strong>
                  <small>created {when(token.createdAt)} · used {when(token.lastUsedAt)}</small>
                </div>
                <div class="token-action">
                  <button type="button" class="btn quiet danger" onclick={() => (revokePending = token)} disabled={!!busy}>Revoke</button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </CollectionPanel>
    {/if}
  {:else if section === 'gaffer'}
    {#if canWrite}<GafferSettings {onerror} />{:else}<p class="notice">This account is read-only, so it cannot configure Gaffer providers.</p>{/if}
  {:else if section === 'mcp'}
    <MCPAccess timeZone={profile.timezone} dateFormat={profile.dateFormat} />
  {:else if section === 'backups'}
    <Backups {onerror} role={who.role} admin={false} {canWrite} timeZone={profile.timezone} dateFormat={profile.dateFormat} />
  {/if}

  {#if section === 'sessions'}
  <section class="block signout">
    <h2>Session</h2>
    <p class="muted">Sign out of your account on this browser.</p>
    <button type="button" class="btn" onclick={onsignout}>Sign out</button>
  </section>
  {/if}
</section>

{#if ssoUnlinkAsk}
  <ConfirmDialog title="Unlink your identity?" body="You will no longer be able to sign in with your identity provider. Sign in with your password instead, or link it again from this page." confirmLabel="Unlink" danger onconfirm={() => void unlinkSso()} oncancel={() => (ssoUnlinkAsk = false)} />
{/if}

{#if revokePending}
  <ConfirmDialog title={`Revoke ${revokePending.name}?`} body="Scanners and API clients using this token will lose access immediately. You can create a replacement, but this token cannot be restored." confirmLabel="Revoke token" danger onconfirm={() => { if (revokePending) void revoke(revokePending.id) }} oncancel={() => (revokePending = null)} />
{/if}

<style>
  .account { padding-top: 4px; }
  /* The avatar sits beside the title and lede only: the header's section row spans the full width below it. */
  .idcard { position: relative; }
  .idcard :global(.page-heading) { padding-left: 80px; min-height: 64px; }
  .avatar {
    position: absolute; top: 0; left: 0;
    display: inline-grid; place-items: center; width: 64px; height: 64px;
    border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line)); border-radius: 50%;
    background: color-mix(in srgb, var(--tint) 45%, var(--card)); color: var(--accent-text);
    font: 600 1.3rem/1 var(--sans); letter-spacing: 0.03em;
  }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  .block { margin-top: 24px; padding: clamp(16px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .muted { margin: 8px 0 0; color: var(--muted); }
  .note { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .profile-form { margin-top: 0; gap: 16px; }
  .profile-form fieldset { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 8px; min-width: 0; }
  .profile-form legend { padding: 0 5px; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .profile-form .grid2 { gap: 12px; }
  .password-form { margin-top: 0; }
  /* minmax(0, 1fr): a text field is about 20 characters wide by default, which overflowed the card on a 320 px phone. */
  form { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; margin-top: 16px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; align-items: start; gap: 16px; }
  .grid2 label { display: grid; align-content: start; gap: 4px; font-size: 0.875rem; color: var(--muted); }
  .grid2 label.wide, .grid2 .field.wide { grid-column: 1 / -1; }
  .grid2 .field { display: grid; align-content: start; gap: 4px; font-size: 0.875rem; color: var(--muted); }
  .grid2 small { color: var(--muted); font-size: 0.875rem; }
  .formacts { display: flex; justify-content: flex-end; }
  .prefix-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .prefix-toggle { display: flex; align-items: center; gap: 8px; color: var(--ink); font-size: 0.875rem; }
  .prefix-toggle input { width: auto; }
  .prefix-input { flex: 1 1 140px; max-width: 220px; }
  .session-toolbar, .token-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .session-search, .token-search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .session-search input, .token-search input { width: 100%; }
  .session-scopes, .token-scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .session-list, .token-list { list-style: none; margin: 0; padding: 0; display: grid; }
  .session-list > li, .token-list > li { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: start; gap: 12px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .session-list > li:first-child, .token-list > li:first-child { border-top: 0; }
  .session-main, .token-main { min-width: 0; }
  .session-main strong, .token-main strong { overflow-wrap: anywhere; }
  .session-main small, .token-main small { display: block; margin-top: 3px; color: var(--muted); font-size: 0.875rem; line-height: 1.4; font-variant-numeric: tabular-nums; }
  .session-action, .token-action { justify-self: end; }
  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h2 { margin: 0; font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  /* Token, session and connection names can be one long word: they wrap rather than widen the card past the screen. */
  .tag { margin-left: 6px; padding: 1px 7px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; }
  .fresh { margin-top: 16px; padding: 12px; border: 1px solid var(--accent); border-radius: 8px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.88rem; }
  .createrow { display: flex; gap: 8px; flex-wrap: wrap; }
  .createrow input { flex: 1; min-width: 0; }
  .token-secret { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .token-secret code { min-width: 0; overflow-wrap: anywhere; }
  .signout { margin-top: 28px; }
  @media (max-width: 560px) {
    .grid2 { grid-template-columns: 1fr; }
    .idcard :global(.page-heading) { padding-left: 64px; min-height: 52px; }
    .avatar { width: 52px; height: 52px; font-size: 1.05rem; }
    .session-search, .token-search { flex-basis: 100%; }
    .session-list > li, .token-list > li { grid-template-columns: 1fr; }
    .session-action, .token-action { justify-self: start; }
    /* One line: a wrapping column is as wide as its widest item, so the field ran out of the card. */
    .createrow { flex-direction: column; flex-wrap: nowrap; }

  }
  @media (pointer: coarse) { .scope, .session-search input, .token-search input, .session-action .btn, .token-action .btn { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .scope { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
