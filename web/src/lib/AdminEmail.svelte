<script lang="ts">
  import { onMount } from 'svelte'
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
  let filter = $state('')
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

  const messages = $derived((outbox?.messages ?? []).filter((m) => !filter || m.state === filter))
  function when(ms?: number) {
    return ms ? new Date(ms).toLocaleString() : '—'
  }
  function count(state: string) {
    return outbox?.counts?.[state] ?? 0
  }
  function dnsLine(label: string, rec: { state: string; record?: string; advice: string }) {
    return `${label}: ${rec.state}${rec.record ? ` — ${rec.record}` : ''}\n${rec.advice}`
  }

  onMount(() => { void load() })
</script>

<section class="email" aria-labelledby="email-title">
  <h2 id="email-title">Email</h2>
  <p class="muted">One mail server for the whole instance. Every message is written to the outbox first, so a slow or broken server costs a retry, not an email or a hanging page.</p>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}
  {#if loading}
    <p class="muted" role="status">Loading email settings…</p>
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
          <button type="submit" class="btn primary" disabled={saving}>{saving ? 'Saving…' : 'Save settings'}</button>
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

    <section class="outbox" aria-labelledby="outbox-title">
      <div class="outbox-head">
        <h3 id="outbox-title">Outbox</h3>
        <div class="filters" role="group" aria-label="Filter outbox">
          {#each [['', 'All'], ['queued', 'Queued'], ['held', 'Held'], ['delivered', 'Delivered'], ['failed', 'Failed'], ['bounced', 'Bounced']] as [key, label]}
            <button type="button" class="btn quiet" aria-pressed={filter === key} onclick={() => (filter = key)}>{label} <small>{key ? count(key) : Object.values(outbox?.counts ?? {}).reduce((a, b) => a + b, 0)}</small></button>
          {/each}
        </div>
      </div>
      {#if !messages.length}
        <p class="muted">Nothing here yet.</p>
      {:else}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need to scroll the wide table.) -->
        <div class="tablewrap" tabindex="0" role="region" aria-label="Email outbox, scroll horizontally for more">
          <table>
            <thead><tr><th>Message</th><th>To</th><th>State</th><th>Created</th><th></th></tr></thead>
            <tbody>
              {#each messages as m (m.id)}
                <tr>
                  <td><strong>{m.subject || m.template}</strong><small>{m.template}{m.secret ? ' · sign-in link' : ''}</small></td>
                  <td class="to">{m.to}</td>
                  <td>
                    <span class="state">{m.state}</span>
                    {#if m.attempts}<small>{m.attempts} {m.attempts === 1 ? 'try' : 'tries'}</small>{/if}
                    {#if m.nextAttemptAt && m.state === 'queued'}<small>next {when(m.nextAttemptAt)}</small>{/if}
                    {#if m.lastError}<small class="problem">{m.lastError}</small>{/if}
                  </td>
                  <td>{when(m.createdAt)}</td>
                  <td>{#if m.resendable && isOwner}<button type="button" class="btn quiet" disabled={saving} onclick={() => resend(m.id)}>Resend</button>{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#if outbox?.warning}<p class="notice" role="status">{outbox.warning}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .email h2 { margin: 0 0 8px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .muted { margin: 4px 0 0; color: var(--muted); }
  .summary { margin: 16px 0; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .summary p { margin: 8px 0 0; overflow-wrap: anywhere; }
  .summary small { display: block; margin-top: 6px; color: var(--muted); font-size: 0.78rem; }
  .problem, .result.bad p { color: var(--bad); }
  form { display: grid; gap: 12px; margin: 18px 0; }
  .pair { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
  label { display: grid; gap: 5px; color: var(--muted); font-size: 0.82rem; min-width: 0; }
  label.wide { grid-column: span 2; }
  label small { color: var(--muted); font-size: 0.74rem; line-height: 1.4; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .result { margin: 14px 0; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .result.good { border-color: var(--good); }
  .result.bad { border-color: var(--bad); }
  .result h3 { margin: 0 0 8px; font-size: 0.9rem; }
  pre { margin: 10px 0 0; padding: 12px; overflow: auto; border-radius: 8px; background: color-mix(in srgb, var(--tint) 18%, var(--card)); font-size: 0.76rem; line-height: 1.5; }
  .outbox { margin-top: 24px; border-top: 1px solid var(--line); padding-top: 18px; }
  .outbox-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .outbox h3 { margin: 0; font: 500 1.2rem/1.2 var(--serif); }
  .filters { display: flex; flex-wrap: wrap; gap: 2px; }
  .filters small { color: var(--muted); margin-left: 3px; }
  .tablewrap { margin-top: 10px; overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 0.84rem; }
  th { text-align: left; color: var(--muted); font-weight: 600; padding: 10px; border-bottom: 1px solid var(--line); white-space: nowrap; }
  td { padding: 12px 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
  td small { display: block; color: var(--muted); font-size: 0.75rem; margin-top: 3px; overflow-wrap: anywhere; }
  td.to { overflow-wrap: anywhere; min-width: 150px; }
  .state { display: inline-block; padding: 2px 8px; border: 1px solid var(--line); border-radius: 999px; font-size: 0.74rem; }
  .notice { margin: 14px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.84rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  @media (max-width: 720px) {
    .pair { grid-template-columns: 1fr; }
    label.wide { grid-column: auto; }
  }
</style>
