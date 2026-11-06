<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import { ADMIN_MAIL_SCOPES, adminMailCountLabel, adminMailMatchesSearch, adminMailStateCounts, filterAdminMail, type AdminMailScope } from './adminMail'
  import {
    checkMailSettings, getMailDNS, getMailSettings, listMailOutbox, resendMail, saveMailSettings, sendTestMail,
    type MailCheck, type MailDNS, type MailOutbox, type MailSettingsView,
  } from './api'

  let { role = 'owner', onerror }: { role?: string; onerror?: (e: unknown) => void } = $props()
  const isOwner = $derived(role === 'owner')

  type Form = {
    host: string; port: number; tls: string; user: string; from: string; fromName: string;
    replyTo: string; baseUrl: string; dkimSelector: string
  }
  let settings = $state<MailSettingsView | null>(null)
  let outbox = $state<MailOutbox | null>(null)
  let form = $state<Form>({ host: '', port: 587, tls: 'starttls', user: '', from: '', fromName: 'reet.sh', replyTo: '', baseUrl: '', dkimSelector: '' })
  let password = $state('')
  let passwordAction = $state<'keep' | 'replace' | 'clear'>('keep')
  let scope = $state<AdminMailScope>('all')
  let search = $state('')
  let refreshing = $state(false)
  let loading = $state(true)
  let saving = $state(false)
  let notice = $state('')
  let error = $state('')
  let transcript = $state<string[]>([])
  let checkMessage = $state('')
  let checkOK = $state<boolean | null>(null)
  let dns = $state<MailDNS | null>(null)

  function fromSettings(s: MailSettingsView): Form {
    return {
      host: s.host, port: s.port || 587, tls: s.tls || 'starttls', user: s.user,
      from: s.from, fromName: s.fromName || 'reet.sh', replyTo: s.replyTo,
      baseUrl: s.baseUrl || s.baseUrlInUse, dkimSelector: s.dkimSelector,
    }
  }

  async function load() {
    loading = true
    try {
      const r = await getMailSettings()
      settings = r.settings
      form = fromSettings(r.settings)
      await loadOutbox()
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  async function loadOutbox() {
    outbox = await listMailOutbox()
  }

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    saving = true
    notice = ''
    error = ''
    try {
      const r = await saveMailSettings({ ...form, password, passwordAction })
      settings = r.settings
      form = fromSettings(r.settings)
      password = ''
      passwordAction = 'keep'
      notice = r.done
      await loadOutbox()
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  function useResult(r: MailCheck) {
    transcript = r.transcript
    checkMessage = r.message
    checkOK = r.ok
  }

  async function check() {
    saving = true
    notice = ''
    error = ''
    try {
      useResult(await checkMailSettings({ ...form, password, passwordAction }))
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  async function test() {
    saving = true
    notice = ''
    error = ''
    try {
      useResult(await sendTestMail())
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  async function lookupDNS() {
    saving = true
    error = ''
    try {
      dns = await getMailDNS(form.from, form.dkimSelector)
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  async function resend(id: string) {
    saving = true
    notice = ''
    error = ''
    try {
      const r = await resendMail(id)
      notice = r.done
      await loadOutbox()
    } catch (e) {
      fail(e)
    } finally {
      saving = false
    }
  }

  const allMessages = $derived(outbox?.messages ?? [])
  const searched = $derived(allMessages.filter((m) => adminMailMatchesSearch(m, search)))
  const counts = $derived(adminMailStateCounts(searched))
  const messages = $derived(filterAdminMail(allMessages, scope, search))
  const countLabel = $derived(outbox ? adminMailCountLabel(messages.length, allMessages.length) : 'Reading the outbox…')
  const stateLabels: Record<string, string> = { queued: 'Queued', sending: 'Sending', held: 'Held', delivered: 'Delivered', failed: 'Failed', bounced: 'Bounced' }
  function stateLabel(state: string) {
    return stateLabels[state] ?? state
  }
  async function refreshOutbox() {
    refreshing = true
    error = ''
    try {
      await loadOutbox()
    } catch (e) {
      fail(e)
    } finally {
      refreshing = false
    }
  }
  function clearFilters() {
    search = ''
    scope = 'all'
  }
  function when(ms?: number) {
    return ms ? new Date(ms).toLocaleString() : '—'
  }
  function dnsLine(label: string, rec: { state: string; record?: string; advice: string }) {
    return `${label}: ${rec.state}${rec.record ? ` — ${rec.record}` : ''}\n${rec.advice}`
  }

  onMount(() => { void load() })
</script>

<section class="email" aria-label="Email">
  <p class="muted">One mail server for the whole instance. Every message is written to the outbox first, so a slow or broken server costs a retry, not an email or a hanging page.</p>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}
  {#if loading}
    <Skeleton label="Loading email settings…" count={4} />
  {:else if settings}
    <div class="summary">
      <span class="badge">{settings.mode === 'smtp' ? 'Sending' : settings.mode === 'outbox' ? 'Local outbox' : 'Held'}</span>
      <p>{settings.summary}</p>
      {#if settings.problem}<p class="problem" role="alert">{settings.problem}</p>{/if}
      {#if settings.source === 'database' && settings.updatedAt}
        <small>Last saved {when(settings.updatedAt)}{settings.updatedBy ? ` by ${settings.updatedBy}` : ''}.</small>
      {/if}
    </div>

    {#if isOwner}
      <form onsubmit={save} aria-busy={saving}>
        <div class="pair">
          <label class="wide"><span>Mail server</span><input bind:value={form.host} placeholder="smtp.example.com" autocomplete="off" /></label>
          <label><span>Port</span><input type="number" min="1" max="65535" bind:value={form.port} /></label>
          <label><span>Security</span>
            <select bind:value={form.tls}>
              <option value="starttls">STARTTLS</option>
              <option value="implicit">Implicit TLS (465)</option>
              <option value="none">None (local only)</option>
            </select>
          </label>
        </div>
        <div class="pair">
          <label><span>Username</span><input bind:value={form.user} autocomplete="off" placeholder="Empty sends without signing in" /></label>
          <label><span>Password</span>
            <select bind:value={passwordAction}>
              <option value="keep">{settings.passwordSet ? 'Keep the saved password' : 'No password'}</option>
              <option value="replace">Replace the password</option>
              <option value="clear">Clear the password</option>
            </select>
          </label>
        </div>
        {#if passwordAction === 'replace'}
          <label class="wide"><span>New password</span><input type="password" bind:value={password} autocomplete="new-password" /><small>Sealed with this instance's key. It is never shown again.</small></label>
        {/if}
        <div class="pair">
          <label class="wide"><span>From address</span><input type="email" bind:value={form.from} placeholder="hello@example.com" autocomplete="off" /><small>A bare address. The name has its own field.</small></label>
          <label><span>From name</span><input bind:value={form.fromName} placeholder="reet.sh" autocomplete="off" /></label>
          <label><span>Reply-to</span><input type="email" bind:value={form.replyTo} placeholder="Optional" autocomplete="off" /></label>
        </div>
        <div class="pair">
          <label class="wide"><span>Links start with</span><input type="url" bind:value={form.baseUrl} placeholder={settings.baseUrlInUse} autocomplete="off" /><small>Usually this instance's public address. Leave empty to use REET_PUBLIC_URL or the address the page was opened at.</small></label>
          <label><span>DKIM selector</span><input bind:value={form.dkimSelector} placeholder="default" autocomplete="off" /></label>
        </div>
        <div class="actions">
          <button type="submit" class="btn primary" disabled={saving} aria-busy={saving}>{saving ? 'Saving…' : 'Save settings'}</button>
          <button type="button" class="btn" disabled={saving} onclick={check}>Check connection</button>
          <button type="button" class="btn" disabled={saving} onclick={test}>Send test email</button>
          <button type="button" class="btn" disabled={saving} onclick={lookupDNS}>Check DNS</button>
        </div>
      </form>
    {:else}
      <p class="muted">Only the owner can change the mail server.</p>
    {/if}

    {#if checkMessage}
      <div class:bad={checkOK === false} class:good={checkOK === true} class="result" role="status">
        <p>{checkMessage}</p>
        {#if transcript.length}
          <details><summary>What the server said</summary><pre>{transcript.join('\n')}</pre></details>
        {/if}
      </div>
    {/if}

    {#if dns}
      <div class="result">
        <h3>DNS for {dns.domain}</h3>
        <pre>{dnsLine('SPF', dns.spf)}

{dnsLine('DKIM', dns.dkim)}

{dnsLine('DMARC', dns.dmarc)}

{dnsLine('BIMI', dns.bimi)}</pre>
      </div>
    {/if}

    <CollectionPanel label="Outbox" count={countLabel}>
      {#snippet actions()}
        <button type="button" class="btn quiet" onclick={refreshOutbox} disabled={refreshing || loading}>Refresh</button>
      {/snippet}
      {#snippet toolbar()}
        <div class="outbox-toolbar">
          <label class="search">Search outbox
            <input type="search" bind:value={search} placeholder="Subject, recipient or template" />
          </label>
          <div class="scopes" role="group" aria-label="Show messages">
            {#each ADMIN_MAIL_SCOPES as item}
              <button type="button" class="scope" aria-pressed={scope === item.key} onclick={() => (scope = item.key)}>{item.label}<small>{counts[item.key]}</small></button>
            {/each}
          </div>
        </div>
      {/snippet}

      {#if !outbox}
        <div class="state-block error" role="alert">
          <h3>The outbox could not be read.</h3>
          <p>Try again without leaving this page.</p>
          <button type="button" class="btn" onclick={refreshOutbox}>Try again</button>
        </div>
      {:else if messages.length === 0}
        <div class="state-block">
          <h3>{allMessages.length === 0 ? 'No messages yet.' : 'No message matches that search or state.'}</h3>
          <p>{allMessages.length === 0 ? 'Invitations, sign-in links, password resets and notices appear here before they are sent.' : 'Try another search, or show every state.'}</p>
          {#if allMessages.length > 0}<button type="button" class="btn" onclick={clearFilters}>Clear filters</button>{/if}
        </div>
      {:else}
        <ul class="messages">
          {#each messages as m (m.id)}
            <li class:failing={m.state === 'failed' || m.state === 'bounced'}>
              <div class="message-main">
                <strong>{m.subject || m.template}</strong>
                <small>{m.template}{m.secret ? ' · sign-in link' : ''} · to {m.to}</small>
                {#if m.lastError}<small class="bad">{m.lastError}</small>{/if}
              </div>
              <div class="message-meta">
                <span class="state {m.state}">{stateLabel(m.state)}</span>
                <small>{when(m.createdAt)}</small>
                {#if m.attempts}<small>{m.attempts} {m.attempts === 1 ? 'try' : 'tries'}</small>{/if}
                {#if m.nextAttemptAt && m.state === 'queued'}<small>next {when(m.nextAttemptAt)}</small>{/if}
              </div>
              <div class="message-action">
                {#if m.resendable && isOwner}<button type="button" class="btn quiet" disabled={saving} onclick={() => resend(m.id)}>Resend</button>{/if}
              </div>
            </li>
          {/each}
        </ul>
      {/if}
      {#if outbox?.warning}<p class="notice" role="status">{outbox.warning}</p>{/if}
    </CollectionPanel>
  {/if}
</section>

<style>
  .muted { margin: 4px 0 0; color: var(--muted); }
  .summary { margin: 16px 0; padding: 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .summary .badge { display: inline-block; padding: 2px 8px; border: 1px solid var(--line); border-radius: 6px; font-size: 0.875rem; }
  .summary p { margin: 8px 0 0; overflow-wrap: anywhere; }
  .summary small { display: block; margin-top: 6px; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .problem, .result.bad p { color: var(--bad); }
  form { display: grid; gap: 12px; margin: 16px 0; }
  .pair { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
  label { display: grid; gap: 5px; color: var(--muted); font-size: 0.875rem; min-width: 0; }
  label.wide { grid-column: span 2; }
  label small { color: var(--muted); font-size: 0.875rem; line-height: 1.4; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .result { margin: 16px 0; padding: 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .result.good { border-color: var(--good); }
  .result.bad { border-color: var(--bad); }
  .result h3 { margin: 0 0 8px; font-size: 0.9rem; }
  pre { margin: 10px 0 0; padding: 12px; overflow: auto; border-radius: 8px; background: color-mix(in srgb, var(--tint) 18%, var(--card)); font-size: 0.875rem; line-height: 1.5; }
  .outbox-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 240px; }
  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .messages { list-style: none; margin: 0; padding: 0; display: grid; }
  .messages > li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: start; gap: 12px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .messages > li:first-child { border-top: 0; }
  .messages > li.failing { background: color-mix(in srgb, var(--bad) 4%, transparent); }
  .message-main { display: grid; gap: 3px; min-width: 0; }
  .message-main strong { overflow-wrap: anywhere; }
  .message-main small { color: var(--muted); font-size: 0.875rem; line-height: 1.4; overflow-wrap: anywhere; }
  .message-meta { display: grid; gap: 3px; justify-items: start; min-width: 0; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .message-meta small { font-size: 0.875rem; overflow-wrap: anywhere; }
  .message-action { justify-self: end; }
  .state { display: inline-block; padding: 2px 8px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; white-space: nowrap; }
  .state.queued, .state.sending { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent-text); }
  .state.delivered { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .state.held { border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); color: var(--warn); }
  .state.failed, .state.bounced { border-color: color-mix(in srgb, var(--bad) 42%, var(--line)); color: var(--bad); }
  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h3 { margin: 0; font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  .state-block.error p { color: var(--ink); }
  .bad { color: var(--bad) !important; }
  .notice { margin: 14px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.875rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 720px) {
    .pair { grid-template-columns: 1fr; }
    label.wide { grid-column: auto; }
    .outbox-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .messages > li { grid-template-columns: minmax(0, 1fr) auto; }
    .message-meta { grid-column: 1; grid-row: 2; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 10px; }
    .message-action { grid-column: 2; grid-row: 1 / span 2; align-self: center; }
  }
  @media (pointer: coarse) { .scope, .search input, .message-action .btn { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .scope { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
