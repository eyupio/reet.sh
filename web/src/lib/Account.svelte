<script lang="ts">
  import NavigationMenu from './NavigationMenu.svelte'
  import { onMount } from 'svelte'
  import ActivitySection from './ActivitySection.svelte'
  import AgentsSettings from './AgentsSettings.svelte'
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

  type Section = 'machines' | 'sync' | 'profile' | 'agents' | 'security' | 'sso' | 'sessions' | 'tokens' | 'mcp' | 'backups' | 'activity'
  // svelte-ignore state_referenced_locally
  let who = $state<User>(user)
  let sso = $state<SsoIdentity | null>(null)
  let ssoUnlinkAsk = $state(false)
  const canWrite = $derived(who.role !== 'viewer')
  const sections: { key: Section; label: string }[] = $derived([
    { key: 'profile', label: 'Profile' },
    { key: 'agents', label: 'Agents' },
    { key: 'security', label: 'Password' },
    ...(sso?.enabled ? [{ key: 'sso' as const, label: 'Single sign-on' }] : []),
    { key: 'sessions', label: 'Sessions' },
    { key: 'mcp', label: 'MCP access' },
    { key: 'machines', label: 'Machines' },
    { key: 'sync', label: 'Compare & sync' },
    ...(canWrite ? [{ key: 'tokens' as const, label: 'API tokens' }] : []),
    { key: 'backups', label: 'Backups' },
    { key: 'activity', label: 'Activity' },
  ])

  const startConnect = location.hash === '#connect-machine'
  let section = $state<Section>(sectionFromHash())
  let profile = $state<Profile>({ theme: 'system', timezone: '', dateFormat: 'locale', shelfView: 'list', cardsExpanded: false, hostedPrefix: 'reet-', updatedAt: 0 })
  let sessions = $state<ProfileSession[]>([])
  let tokens = $state<APIToken[]>([])
  let freshToken = $state('')
  let tokenName = $state('scan token')
  let freshTokenId = $state(0)
  let revokePending = $state<APIToken | null>(null)
  let error = $state('')
  let notice = $state('')
  let busy = $state('')

  // svelte-ignore state_referenced_locally
  let profileForm = $state({ name: user.name, theme: 'system', timezone: '', dateFormat: 'locale', shelfView: 'list' as 'list' | 'board', cardsExpanded: false, terminalRemoveOnClose: false })
  let prefixEnabled = $state(true)
  let prefixValue = $state('reet-')
  let passwordForm = $state({ current: '', password: '', confirm: '' })

  function sectionFromHash(): Section {
    // /activity is the old address for the account's activity section; it keeps working and is rewritten below.
    const fromPath = location.pathname === '/activity' ? 'activity' : location.pathname.split('/')[2] || ''
    const h = location.hash.slice(1) || fromPath || 'profile'
    if (h === 'connect-machine') return 'machines'
    return ['profile', 'agents', 'security', 'sso', 'sessions', 'machines', 'sync', 'tokens', 'mcp', 'backups', 'activity'].includes(h) ? h as Section : 'profile'
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
      profileForm = { name: r.user.name, theme: r.profile.theme, timezone: r.profile.timezone, dateFormat: r.profile.dateFormat, shelfView: r.profile.shelfView, cardsExpanded: r.profile.cardsExpanded, terminalRemoveOnClose: r.profile.terminalRemoveOnClose??false }
      prefixEnabled = r.profile.hostedPrefix !== ''
      prefixValue = r.profile.hostedPrefix || 'reet-'
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
      const { name, ...preferences } = profileForm
      const hostedPrefix = prefixEnabled ? (prefixValue.trim() || 'reet-') : ''
      const r = await saveProfile({ ...(canWrite ? profileForm : preferences), hostedPrefix })
      who = r.user
      profile = r.profile
      prefixEnabled = r.profile.hostedPrefix !== ''
      prefixValue = r.profile.hostedPrefix || 'reet-'
      onuserchange?.(r.user)
      applyTheme(r.profile.theme)
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
    <PageHeader eyebrow="Your account" title={who.name || who.email} titleId="account-title" lede={`${who.email} · ${who.role}`} />
  </div>

  <NavigationMenu label="Account" items={sections.map(s => ({ label: s.label, href: s.key === 'machines' ? '/machines' : s.key === 'sync' ? '/machines?view=compare' : `/account/${s.key}`, current: section === s.key }))} />

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}

  {#if section === 'profile'}
    <section class="block" aria-labelledby="profile-title">
      <h2 id="profile-title">Profile</h2>
      <p class="muted">Make your workspace feel just reet. Set your terminal, theme and date preferences.</p>
      <form onsubmit={save}>
        <div class="grid2">
          <label class="wide"><span>Name</span>
            <input bind:value={profileForm.name} autocomplete="name" maxlength="100" placeholder="Your name" disabled={!canWrite} />
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
          <label class="wide"><span>When stopping a terminal session</span><select aria-label="Terminal history default" bind:value={profileForm.terminalRemoveOnClose}><option value={false}>Keep the closed session in history</option><option value={true}>Remove from history after it ends</option></select><small>This sets the confirmation’s default. You can change it for each session. Running or pending sessions remain visible until the machine confirms closure.</small></label>
          <label><span>Default shelf layout</span>
            <select bind:value={profileForm.shelfView}><option value="list">List</option><option value="board">Board</option></select>
            <small>Switching List/Board on your shelf also saves this preference.</small>
          </label>
          <label><span>Default skill cards</span>
            <select bind:value={profileForm.cardsExpanded}><option value={false}>Collapsed — quick actions and core data</option><option value={true}>Expanded — all details</option></select>
            <small>Individual cards and Expand all / Collapse all can override this during your visit.</small>
          </label>
          <div class="wide field"><span>Hosted skill name prefix</span>
            <div class="prefix-row">
              <span class="prefix-toggle"><input id="hosted-prefix-toggle" type="checkbox" bind:checked={prefixEnabled} /><label for="hosted-prefix-toggle">Prefix new and renamed hosted skills</label></span>
              <input class="prefix-input" bind:value={prefixValue} placeholder="reet-" maxlength="33" disabled={!prefixEnabled} aria-label="Hosted skill name prefix" />
            </div>
            <small>Reet adds this to skills you host so they stand out from imports. Type a custom prefix such as <code>acme-</code>, or turn it off to use the bare skill name. Existing names are not changed automatically.</small>
          </div>
        </div>
        <div class="formacts">
          <button type="submit" class="btn primary" disabled={busy === 'profile'}>{busy === 'profile' ? 'Saving…' : 'Save profile'}</button>
        </div>
      </form>
    </section>
  {:else if section === 'agents'}
    <AgentsSettings {canWrite} onerror={onerror} />
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
      <section class="block" aria-labelledby="tokens-title">
        <h2 id="tokens-title">Personal API tokens</h2>
        <p class="muted">Use a token with <code>reet scan --token …</code> or an API client. A token is shown once, then only its name remains.</p>
        {@render tokenForm()}

        {#if tokens.length}
          <ul class="rows">
            {#each tokens as token (token.id)}
              <li>
                <div><strong>{token.name}</strong><small>created {when(token.createdAt)} · used {when(token.lastUsedAt)}</small></div>
                <button type="button" class="btn quiet danger" onclick={() => (revokePending = token)} disabled={!!busy}>Revoke</button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted">No API tokens yet.</p>
        {/if}
      </section>
    {/if}
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
  .idcard { display: flex; align-items: center; gap: 18px; }
  .avatar {
    display: inline-grid; place-items: center; flex: none; width: 64px; height: 64px;
    border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line)); border-radius: 999px;
    background: color-mix(in srgb, var(--tint) 45%, var(--card)); color: var(--accent-text);
    font: 600 1.3rem/1 var(--sans); letter-spacing: 0.03em;
  }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .blockhead { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .muted { margin: 8px 0 0; color: var(--muted); }
  /* minmax(0, 1fr): a text field is about 20 characters wide by default, which overflowed the card on a 320 px phone. */
  form { display: grid; grid-template-columns: minmax(0, 1fr); gap: 14px; margin-top: 16px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .grid2 label { display: grid; gap: 4px; font-size: 0.84rem; color: var(--muted); }
  .grid2 label.wide, .grid2 .field.wide { grid-column: 1 / -1; }
  .grid2 .field { display: grid; gap: 4px; font-size: 0.84rem; color: var(--muted); }
  .grid2 small { color: var(--muted); font-size: 0.76rem; }
  .formacts { display: flex; justify-content: flex-end; }
  .prefix-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .prefix-toggle { display: flex; align-items: center; gap: 8px; color: var(--ink); font-size: 0.84rem; }
  .prefix-toggle input { width: auto; }
  .prefix-input { flex: 1 1 140px; max-width: 220px; }
  .rows { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 8px; }
  .rows li { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; padding: 10px 0; border-bottom: 1px solid var(--line); }
  .rows small { display: block; color: var(--muted); font-size: 0.78rem; }
  /* Token, session and connection names can be one long word: they wrap rather than widen the card past the screen. */
  .rows li > div { min-width: 0; }
  .rows strong { overflow-wrap: anywhere; }
  .tag { margin-left: 6px; padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.65rem; }
  .fresh { margin-top: 14px; padding: 12px; border: 1px solid var(--accent); border-radius: 10px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.88rem; }
  .createrow { display: flex; gap: 8px; flex-wrap: wrap; }
  .createrow input { flex: 1; min-width: 0; }
  .token-secret { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
  .token-secret code { min-width: 0; overflow-wrap: anywhere; }
  .signout { margin-top: 28px; }
  @media (max-width: 560px) {
    .grid2 { grid-template-columns: 1fr; }
    .idcard { gap: 12px; }
    .avatar { width: 52px; height: 52px; font-size: 1.05rem; }
    /* One line: a wrapping column is as wide as its widest item, so the field ran out of the card. */
    .createrow { flex-direction: column; flex-wrap: nowrap; }

  }
</style>
