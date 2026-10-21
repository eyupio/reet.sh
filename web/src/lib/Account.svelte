<script lang="ts">
  import { onMount } from 'svelte'
  import Backups from './Backups.svelte'
  import CopyButton from './CopyButton.svelte'
  import EnvironmentSync from './EnvironmentSync.svelte'
  import MCPAccess from './MCPAccess.svelte'
  import PageHeader from './PageHeader.svelte'
  import {
    applyTheme, changePassword, createToken, endOtherProfileSessions, endProfileSession, getProfile, listEvents,
    listProfileSessions, listTokens, revokeToken, saveProfile,
    type APIToken, type Event, type Profile, type ProfileSession, type User,
  } from './api'
  import { eventLabel, eventTitle } from './events'
  import { elapsed } from './time'

  let {
    user, onsignout, onerror, onuserchange,
  }: { user: User; onsignout: () => void; onerror?: (e: unknown) => void; onuserchange?: (u: User) => void } = $props()

  type Section = 'profile' | 'security' | 'sessions' | 'tokens' | 'mcp' | 'backups' | 'activity'
  // svelte-ignore state_referenced_locally
  let who = $state<User>(user)
  const canWrite = $derived(who.role !== 'viewer')
  const sections: { key: Section; label: string }[] = $derived([
    { key: 'profile', label: 'Profile' },
    { key: 'security', label: 'Password' },
    { key: 'sessions', label: 'Sessions' },
    { key: 'mcp', label: 'MCP access' },
    ...(canWrite ? [{ key: 'tokens' as const, label: 'API tokens' }] : []),
    { key: 'backups', label: 'Backups' },
    { key: 'activity', label: 'Activity' },
  ])

  let section = $state<Section>(sectionFromHash())
  let profile = $state<Profile>({ theme: 'system', timezone: '', dateFormat: 'locale', updatedAt: 0 })
  let sessions = $state<ProfileSession[]>([])
  let events = $state<Event[]>([])
  let tokens = $state<APIToken[]>([])
  let freshToken = $state('')
  let tokenName = $state('scan token')
  let error = $state('')
  let notice = $state('')
  let busy = $state('')

  // svelte-ignore state_referenced_locally
  let profileForm = $state({ name: user.name, theme: 'system', timezone: '', dateFormat: 'locale' })
  let passwordForm = $state({ current: '', password: '', confirm: '' })

  function sectionFromHash(): Section {
    const h = location.hash.replace('#', '') as Section
    return ['profile', 'security', 'sessions', 'tokens', 'mcp', 'backups', 'activity'].includes(h) ? h : 'profile'
  }

  function go(s: Section) {
    section = s
    error = ''
    notice = ''
    history.replaceState(null, '', `#${s}`)
    if (s === 'sessions') void loadSessions()
    if (s === 'activity') void loadActivity()
    if (s === 'tokens') void loadTokens()
  }

  onMount(() => {
    void loadProfile()
    if (section === 'sessions') void loadSessions()
    if (section === 'activity') void loadActivity()
    if (section === 'tokens') void loadTokens()
  })

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  async function loadProfile() {
    try {
      const r = await getProfile()
      who = r.user
      profile = r.profile
      profileForm = { name: r.user.name, theme: r.profile.theme, timezone: r.profile.timezone, dateFormat: r.profile.dateFormat }
      applyTheme(r.profile.theme)
      error = ''
    } catch (e) {
      fail(e)
    }
  }

  async function loadSessions() {
    try {
      sessions = (await listProfileSessions()).sessions
      error = ''
    } catch (e) {
      fail(e)
    }
  }

  async function loadActivity() {
    try {
      events = (await listEvents(50)).events
      error = ''
    } catch (e) {
      fail(e)
    }
  }

  async function loadTokens() {
    if (!canWrite) return
    try {
      tokens = (await listTokens()).tokens
      error = ''
    } catch (e) {
      fail(e)
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = 'profile'
    notice = ''
    try {
      const r = await saveProfile(profileForm)
      who = r.user
      profile = r.profile
      onuserchange?.(r.user)
      applyTheme(r.profile.theme)
      notice = 'Profile saved. The theme follows you to every browser you sign in from.'
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
    busy = 'token'
    freshToken = ''
    try {
      const r = await createToken(tokenName.trim() || 'scan token')
      freshToken = r.token
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
    try {
      await revokeToken(id)
      await loadTokens()
    } catch (e) {
      fail(e)
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

<section class="account" aria-labelledby="account-title">
  <div class="idcard">
    <span class="avatar" aria-hidden="true">{initials()}</span>
    <PageHeader eyebrow="Your account" title={who.name || who.email} titleId="account-title" lede={`${who.email} · ${who.role}`} />
  </div>

  <nav class="section-nav" aria-label="Account sections">
    {#each sections as s (s.key)}
      <button type="button" aria-current={section === s.key ? 'page' : undefined} onclick={() => go(s.key)}>{s.label}</button>
    {/each}
  </nav>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}

  {#if section === 'profile'}
    <section class="block" aria-labelledby="profile-title">
      <h2 id="profile-title">Profile</h2>
      <p class="muted">How your name appears to your team and to admins, and how reet shows you dates.</p>
      <form onsubmit={save}>
        <div class="grid2">
          <label class="wide"><span>Name</span>
            <input bind:value={profileForm.name} autocomplete="name" maxlength="100" placeholder="Your name" />
            <small>Shown on lists and in the team rollup. Leave it empty to be shown by your address.</small>
          </label>
          <label><span>Theme</span>
            <select bind:value={profileForm.theme}>
              <option value="system">Match this device</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
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
        <div class="formacts">
          <button type="submit" class="btn primary" disabled={busy === 'profile'}>{busy === 'profile' ? 'Saving…' : 'Save profile'}</button>
        </div>
      </form>
    </section>
  {:else if section === 'security'}
    <section class="block" aria-labelledby="password-title">
      <h2 id="password-title">Password</h2>
      <p class="muted">Changing it signs out every other browser; this one stays signed in.</p>
      <form onsubmit={change}>
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
        <div class="formacts">
          <button type="submit" class="btn primary" disabled={busy === 'password'}>{busy === 'password' ? 'Changing…' : 'Change password'}</button>
        </div>
      </form>
    </section>
  {:else if section === 'sessions'}
    <section class="block" aria-labelledby="sessions-title">
      <div class="blockhead">
        <div>
          <h2 id="sessions-title">Sessions</h2>
          <p class="muted">Every browser signed in to your account.</p>
        </div>
        <button type="button" class="btn" onclick={endOthers} disabled={busy === 'others' || sessions.length < 2}>
          {busy === 'others' ? 'Ending…' : 'Sign out the others'}
        </button>
      </div>
      {#if sessions.length === 0}
        <p class="muted">No sessions listed.</p>
      {:else}
        <ul class="rows">
          {#each sessions as s (s.id)}
            <li>
              <div>
                <strong>{device(s.device)}</strong>
                {#if s.current}<span class="tag">this browser</span>{/if}
                <small>Last seen {when(s.lastSeen)} · signed in {when(s.createdAt)}</small>
              </div>
              {#if !s.current}
                <button type="button" class="btn quiet danger" onclick={() => endSession(s.id)} disabled={busy === `session:${s.id}`}>Sign out</button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {:else if section === 'tokens'}
    {#if !canWrite}
      <p class="muted">Viewer accounts cannot create API tokens.</p>
    {:else}
      <section class="block" aria-labelledby="tokens-title">
        <h2 id="tokens-title">Personal API tokens</h2>
        <p class="muted">Use a token with <code>reet scan --token …</code> or an API client. A token is shown once, then only its name remains.</p>
        <EnvironmentSync {canWrite} token={freshToken} />
        {#if freshToken}
          <div class="fresh" role="status">
            <p>Copy this token now. It will not be shown again.</p>
            <div class="tokenline">
              <code>{freshToken}</code>
              <CopyButton text={freshToken} />
            </div>
          </div>
        {/if}
        {#if tokens.length}
          <ul class="rows">
            {#each tokens as token (token.id)}
              <li>
                <div><strong>{token.name}</strong><small>created {when(token.createdAt)} · used {when(token.lastUsedAt)}</small></div>
                <button type="button" class="btn quiet danger" onclick={() => revoke(token.id)}>Revoke</button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted">No API tokens yet.</p>
        {/if}
        <form onsubmit={makeToken}>
          <label for="token-name">New token name</label>
          <div class="createrow">
            <input id="token-name" bind:value={tokenName} placeholder="scan token" autocomplete="off" />
            <button type="submit" class="btn primary" disabled={busy === 'token'}>{busy === 'token' ? 'Creating…' : 'Create token'}</button>
          </div>
        </form>
      </section>
    {/if}
  {:else if section === 'mcp'}
    <MCPAccess timeZone={profile.timezone} dateFormat={profile.dateFormat} />
  {:else if section === 'backups'}
    <Backups {onerror} role={who.role} admin={false} {canWrite} timeZone={profile.timezone} dateFormat={profile.dateFormat} />
  {:else if section === 'activity'}
    <section class="block" aria-labelledby="activity-title">
      <h2 id="activity-title">Activity</h2>
      <p class="muted">What you changed lately, newest first.</p>
      {#if events.length === 0}
        <p class="muted">Nothing recorded yet.</p>
      {:else}
        <ul class="events">
          {#each events as event (event.id)}
            <li title={eventTitle(event)}><span>{eventLabel(event)}</span><time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time></li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  <section class="block signout">
    <h2>Session</h2>
    <p class="muted">Sign out of your account on this browser.</p>
    <button type="button" class="btn" onclick={onsignout}>Sign out</button>
  </section>
</section>

<style>
  .account { padding-top: 4px; }
  .idcard { display: flex; align-items: center; gap: 18px; }
  .avatar {
    display: inline-grid; place-items: center; flex: none; width: 64px; height: 64px;
    border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line)); border-radius: 999px;
    background: color-mix(in srgb, var(--tint) 45%, var(--card)); color: var(--accent);
    font: 600 1.3rem/1 var(--sans); letter-spacing: 0.03em;
  }
  .section-nav { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 20px; padding-bottom: 16px; border-bottom: 1px solid var(--line); }
  .section-nav button {
    min-height: 40px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 8px;
    background: transparent; color: var(--muted); font-size: 0.82rem; cursor: pointer;
  }
  .section-nav button:hover { background: var(--tint); border-color: var(--accent); color: var(--ink); }
  .section-nav button[aria-current='page'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  h2 { margin: 0 0 10px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .blockhead { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .muted { margin: 8px 0 0; color: var(--muted); }
  form { display: grid; gap: 14px; margin-top: 16px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .grid2 label { display: grid; gap: 4px; font-size: 0.84rem; color: var(--muted); }
  .grid2 label.wide { grid-column: 1 / -1; }
  .grid2 small { color: var(--muted); font-size: 0.76rem; }
  .formacts { display: flex; justify-content: flex-end; }
  .rows { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 8px; }
  .rows li { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; padding: 10px 0; border-bottom: 1px solid var(--line); }
  .rows small { display: block; color: var(--muted); font-size: 0.78rem; }
  .tag { margin-left: 6px; padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.65rem; }
  .fresh { margin-top: 14px; padding: 12px; border: 1px solid var(--accent); border-radius: 10px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.88rem; }
  .tokenline { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
  .tokenline code { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 0.82rem; }
  .createrow { display: flex; gap: 8px; flex-wrap: wrap; }
  .createrow input { flex: 1; min-width: 0; }
  .events { list-style: none; margin: 12px 0 0; padding: 0; }
  .events li { display: flex; justify-content: space-between; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.84rem; }
  .events time { flex: none; }
  .signout { margin-top: 28px; }
  @media (max-width: 560px) {
    .grid2 { grid-template-columns: 1fr; }
    .idcard { gap: 12px; }
    .avatar { width: 52px; height: 52px; font-size: 1.05rem; }
    .createrow { flex-direction: column; }
    .section-nav button { flex: 1 1 auto; }
  }
</style>
