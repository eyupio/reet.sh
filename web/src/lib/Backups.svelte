<script lang="ts">
  import { onMount } from 'svelte'
  import {
    backupDownloadUrl, createBackupRemote, deleteBackup, deleteBackupRemote, getBackupStatus, listBackupRemoteCopies,
    runBackupNow, restoreUserBackup, saveBackupSettings, testBackupRemote, updateBackupRemote, verifyBackup,
    type BackupCheck, type BackupItem, type BackupRemote, type BackupRemoteInput, type BackupStatus,
  } from './api'
  import CopyButton from './CopyButton.svelte'

  let {
    onerror, role = 'owner', admin = false, canWrite = true, timeZone = '', dateFormat = 'locale',
  }: { onerror?: (e: unknown) => void; role?: string; admin?: boolean; canWrite?: boolean; timeZone?: string; dateFormat?: string } = $props()

  const canEdit = $derived(admin ? role === 'owner' : canWrite)

  let status = $state<BackupStatus | null>(null)
  let loading = $state(true)
  let error = $state('')
  let saved = $state('')
  let busy = $state('')
  let form = $state({ every: '24h', keep: '48' })
  let checks = $state<Record<string, BackupCheck>>({})
  let verified = $state<Record<string, { ok: boolean; problems: string[] }>>({})
  let openCopies = $state<string | null>(null)
  let copies = $state<BackupItem[]>([])
  let copiesError = $state('')
  let copiesLoading = $state(false)
  let formOpen = $state(false)
  let editing = $state<BackupRemote | null>(null)
  let restoreFor = $state<BackupItem | null>(null)

  const emptyRemote = () => ({
    name: '', endpoint: '', region: '', bucket: '', prefix: 'reet/', accessKey: '', secretKey: '',
    pathStyle: 'path' as 'auto' | 'path' | 'virtual', keep: String(admin ? 90 : 14), enabled: true,
  })
  let remoteForm = $state(emptyRemote())

  onMount(() => {
    void load()
  })

  async function load() {
    loading = true
    try {
      status = await getBackupStatus(admin)
      const every = status.settings.everySeconds
      form = { every: everyText(every), keep: String(status.settings.keep) }
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  function everyText(seconds: number): string {
    if (!seconds) return '0'
    if (seconds % 86400 === 0) return `${seconds / 86400}d`
    if (seconds % 3600 === 0) return `${seconds / 3600}h`
    if (seconds % 60 === 0) return `${seconds / 60}m`
    return `${seconds}s`
  }

  function everySeconds(text: string): number {
    const t = text.trim().toLowerCase()
    if (t === '' || t === '0') return 0
    const m = t.match(/^(\d+(?:\.\d+)?)\s*([smhd])$/)
    if (!m) return -1
    const n = Number(m[1])
    const unit = { s: 1, m: 60, h: 3600, d: 86400 }[m[2] as 's' | 'm' | 'h' | 'd']
    return Math.round(n * unit)
  }

  const everyBad = $derived(everySeconds(form.every) < 0 || (everySeconds(form.every) > 0 && everySeconds(form.every) < 60))
  const keepBad = $derived(!/^\d+$/.test(form.keep) || Number(form.keep) > 100000)
  const scopeWord = $derived(admin ? 'the instance' : 'your shelf')

  async function saveSettings(e: SubmitEvent) {
    e.preventDefault()
    if (everyBad || keepBad || !canEdit) return
    busy = 'settings'
    saved = ''
    try {
      await saveBackupSettings(admin, everySeconds(form.every), Number(form.keep))
      saved = 'Saved. It applies to the next scheduled copy.'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function runNow() {
    busy = 'run'
    saved = ''
    try {
      const item = await runBackupNow(admin)
      saved = `Took ${item.id}.`
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function verify(item: BackupItem) {
    busy = `verify:${item.id}`
    try {
      verified = { ...verified, [item.id]: await verifyBackup(admin, item.id) }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function removeItem(item: BackupItem) {
    if (!confirm(`Delete ${item.id} from this machine? Copies at destinations are left alone.`)) return
    busy = `delete:${item.id}`
    try {
      await deleteBackup(admin, item.id)
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  function editRemote(r?: BackupRemote) {
    editing = r ?? null
    if (r) {
      remoteForm = {
        name: r.name, endpoint: r.endpoint, region: r.region, bucket: r.bucket, prefix: r.prefix,
        accessKey: r.accessKey, secretKey: '', pathStyle: r.pathStyle === null ? 'auto' : r.pathStyle ? 'path' : 'virtual',
        keep: String(r.keep), enabled: !r.disabled,
      }
    } else {
      remoteForm = emptyRemote()
    }
    formOpen = true
  }

  async function saveRemote(e: SubmitEvent) {
    e.preventDefault()
    if (!remoteForm.name.trim() || !remoteForm.endpoint.trim() || !remoteForm.bucket.trim() || !remoteForm.accessKey.trim()) return
    if (!editing && !remoteForm.secretKey.trim()) {
      error = 'Enter the secret access key for a new destination.'
      return
    }
    const input: BackupRemoteInput = {
      name: remoteForm.name.trim(),
      endpoint: remoteForm.endpoint.trim(),
      region: remoteForm.region.trim(),
      bucket: remoteForm.bucket.trim(),
      prefix: remoteForm.prefix.trim(),
      accessKey: remoteForm.accessKey.trim(),
      pathStyle: remoteForm.pathStyle === 'auto' ? null : remoteForm.pathStyle === 'path',
      keep: Number(remoteForm.keep),
      enabled: remoteForm.enabled,
    }
    if (remoteForm.secretKey.trim()) input.secretKey = remoteForm.secretKey.trim()
    busy = 'remote'
    try {
      if (editing) await updateBackupRemote(admin, editing.name, input)
      else await createBackupRemote(admin, input)
      formOpen = false
      editing = null
      saved = 'Destination saved.'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function removeRemote(r: BackupRemote) {
    if (!confirm(`Remove ${r.name}? What its bucket already holds is left alone.`)) return
    busy = `remote:${r.name}`
    try {
      await deleteBackupRemote(admin, r.name)
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function testRemote(r: BackupRemote) {
    busy = `test:${r.name}`
    try {
      checks = { ...checks, [r.name]: await testBackupRemote(admin, r.name) }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function showCopies(name: string) {
    if (openCopies === name) {
      openCopies = null
      return
    }
    openCopies = name
    copiesLoading = true
    copiesError = ''
    try {
      copies = (await listBackupRemoteCopies(admin, name)).items
    } catch (e) {
      copies = []
      copiesError = e instanceof Error ? e.message : String(e)
    } finally {
      copiesLoading = false
    }
  }

  function bytes(n: number): string {
    if (!n) return '—'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let i = 0
    let v = n
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024
      i++
    }
    return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
  }

  function when(ms?: number): string {
    if (!ms) return 'never'
    const opts: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' }
    if (timeZone) opts.timeZone = timeZone
    try {
      const d = new Date(ms)
      if (dateFormat === 'dmy') return d.toLocaleString('en-GB', opts)
      if (dateFormat === 'mdy') return d.toLocaleString('en-US', opts)
      if (dateFormat === 'ymd') return d.toLocaleString('sv-SE', opts)
      return d.toLocaleString(undefined, opts)
    } catch {
      return new Date(ms).toLocaleString()
    }
  }
</script>

<section class="backups" aria-labelledby="backups-title">
  <div class="head">
    <div>
      <p class="eyebrow">{admin ? 'Instance administration' : 'Your account'}</p>
      <h2 id="backups-title">Backups</h2>
      <p class="lede">
        {admin
          ? 'Sealed copies of the whole database, kept on this machine and shipped to any S3-compatible destination.'
          : 'Sealed copies of your own shelf, on this machine and at your own off-site destination.'}
      </p>
    </div>
    <div class="actions">
      {#if canEdit}
        <button type="button" class="btn primary" onclick={runNow} disabled={busy === 'run' || !status?.enabled}>
          {busy === 'run' ? 'Taking a copy…' : `Back up ${scopeWord} now`}
        </button>
      {/if}
    </div>
  </div>

  {#if loading}
    <p class="muted" role="status">Loading…</p>
  {:else if !status}
    <p class="notice bad" role="alert">{error || 'Could not load the backups.'}</p>
  {:else}
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {#if saved}<p class="notice good" role="status">{saved}</p>{/if}

    {#if !status.enabled}
      <div class="banner" role="alert">
        <strong>Nothing is being backed up.</strong> The backup key is missing or unusable. Set
        <code>REET_BACKUP_KEY</code> to a value from <code>reet backup keygen</code>, keep a copy away from this
        machine, and restart reet.
      </div>
    {/if}

    <div class="cards">
      {#if admin}
        <article>
          <h3>Key</h3>
          <p>{status.key.fingerprint || '—'}</p>
          <small>
            {status.key.source === 'environment' ? 'From the environment' : 'Generated beside the database'}
            {#if status.key.path}· <code>{status.key.path}</code>{/if}
          </small>
        </article>
      {:else}
        <article>
          <h3>Sealed</h3>
          <p>AES-256-GCM</p>
          <small>Every copy is encrypted with the instance's backup key before it is written or sent.</small>
        </article>
      {/if}
      <article>
        <h3>Schedule</h3>
        <p>{everyText(status.settings.everySeconds) === '0' ? 'Manual only' : `Every ${everyText(status.settings.everySeconds)}`}</p>
        <small>Next {when(status.settings.nextAt)}</small>
      </article>
      <article>
        <h3>Keep</h3>
        <p>{status.settings.keep === 0 ? 'Every copy' : `Newest ${status.settings.keep}`}</p>
        <small>{status.items.length} {status.items.length === 1 ? 'copy' : 'copies'} on this machine</small>
      </article>
      <article>
        <h3>Off-site</h3>
        <p>{status.remotes.filter((r) => !r.disabled && !r.problem).length || 'None'}</p>
        <small>{status.remotes.length ? 'Every copy is sealed before it leaves' : 'Copies are on this machine only'}</small>
      </article>
    </div>

    {#if status.findings.length}
      <section class="block" aria-labelledby="findings-title">
        <h3 id="findings-title">Needs attention</h3>
        <ul class="findings">
          {#each status.findings as f, i (f.code + i)}
            <li class={f.severity}>
              <span class="sev">{f.severity}</span>
              <span>{f.message}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="block" aria-labelledby="schedule-title">
      <h3 id="schedule-title">Schedule and retention</h3>
      <p class="muted">How often a copy is taken and how many this machine keeps. 0 turns the schedule off; 0 copies kept means every one.</p>
      <form class="settings" onsubmit={saveSettings}>
        <label>
          <span>Every</span>
          <input bind:value={form.every} placeholder="24h" disabled={!canEdit} aria-invalid={everyBad || undefined} />
          <small class:bad={everyBad}>{everyBad ? 'Use 30m, 1h, 24h or 7d; 0 is off.' : 'The shortest schedule is a minute.'}</small>
        </label>
        <label>
          <span>Keep</span>
          <input bind:value={form.keep} inputmode="numeric" disabled={!canEdit} aria-invalid={keepBad || undefined} />
          <small class:bad={keepBad}>{keepBad ? 'A whole number, 0 or more.' : '0 keeps every copy on this machine.'}</small>
        </label>
        {#if canEdit}
          <button type="submit" class="btn primary" disabled={busy === 'settings' || everyBad || keepBad}>
            {busy === 'settings' ? 'Saving…' : 'Save'}
          </button>
        {:else}
          <p class="muted">{admin ? 'Only the instance owner can change the schedule.' : 'Read-only accounts can see backups but not change them.'}</p>
        {/if}
      </form>
    </section>

    <section class="block" aria-labelledby="destinations-title">
      <div class="blockhead">
        <div>
          <h3 id="destinations-title">Off-site destinations</h3>
          <p class="muted">Any S3-compatible service: AWS S3, Cloudflare R2, Backblaze B2, MinIO, Garage. The secret is sealed in the database.</p>
        </div>
        {#if canEdit}
          <button type="button" class="btn" onclick={() => editRemote()}>Add destination</button>
        {/if}
      </div>

      {#if status.remotes.length === 0}
        <p class="empty">
          No destination is configured.
          {admin ? 'Add one, or set REET_BACKUP_S3_* in the environment.' : 'Add one to copy your shelf off this machine.'}
        </p>
      {:else}
        <ul class="remotes">
          {#each status.remotes as r (r.name)}
            <li class:failing={!!r.lastError}>
              <div class="rwho">
                <strong>{r.name}</strong>
                {#if r.source === 'environment'}<span class="tag">from the environment</span>{/if}
                {#if r.disabled}<span class="tag">disabled</span>{/if}
                {#if r.problem}<span class="tag bad">{r.problem}</span>{/if}
                <small>{r.where}</small>
                <small>Keep {r.keep === 0 ? 'every copy' : `the newest ${r.keep}`}</small>
                {#if r.lastUploadAt && !r.lastError}<small>Last copy {when(r.lastUploadAt)}</small>{/if}
                {#if r.lastError}<small class="bad">Last copy refused: {r.lastError}</small>{/if}
              </div>
              <div class="racts">
                {#if canEdit}
                  <button type="button" class="btn quiet" onclick={() => testRemote(r)} disabled={busy === `test:${r.name}` || !!r.problem}>
                    {busy === `test:${r.name}` ? 'Testing…' : 'Test'}
                  </button>
                  {#if r.source === 'database'}
                    <button type="button" class="btn quiet" onclick={() => editRemote(r)}>Edit</button>
                    <button type="button" class="btn quiet danger" onclick={() => removeRemote(r)} disabled={busy === `remote:${r.name}`}>Remove</button>
                  {/if}
                {/if}
                <button type="button" class="btn quiet" onclick={() => showCopies(r.name)} disabled={!!r.problem}>
                  {openCopies === r.name ? 'Hide copies' : 'Copies'}
                </button>
              </div>
              {#if checks[r.name]}
                <div class="check" aria-live="polite">
                  <p class:good={checks[r.name].ok} class:bad={!checks[r.name].ok}>
                    {checks[r.name].ok ? 'The destination can be written, read and cleaned up.' : checks[r.name].error}
                  </p>
                  <ul>
                    {#each checks[r.name].steps as st (st.step)}
                      <li><strong>{st.step}</strong> {st.ok ? 'ok' : 'failed'} — {st.detail}</li>
                    {/each}
                  </ul>
                </div>
              {/if}
              {#if openCopies === r.name}
                <div class="copies">
                  {#if copiesLoading}
                    <p class="muted">Listing…</p>
                  {:else if copiesError}
                    <p class="bad">{copiesError}</p>
                  {:else if copies.length === 0}
                    <p class="muted">It holds no copy yet.</p>
                  {:else}
                    <ul>
                      {#each copies as c (c.id)}
                        <li><code>{c.id}</code><span>{when(c.takenAt)} · {bytes(c.bytes)}</span></li>
                      {/each}
                    </ul>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if formOpen}
        <form class="remoteform" onsubmit={saveRemote}>
          <h4>{editing ? `Edit ${editing.name}` : 'Add a destination'}</h4>
          <div class="grid2">
            <label><span>Name</span><input bind:value={remoteForm.name} placeholder="offsite" autocomplete="off" /></label>
            <label><span>Endpoint</span><input bind:value={remoteForm.endpoint} placeholder="https://s3.eu-west-2.amazonaws.com" autocomplete="off" /></label>
            <label><span>Region</span><input bind:value={remoteForm.region} placeholder="eu-west-2" autocomplete="off" /></label>
            <label><span>Bucket</span><input bind:value={remoteForm.bucket} placeholder="my-reet-backups" autocomplete="off" /></label>
            <label><span>Prefix</span><input bind:value={remoteForm.prefix} placeholder="reet/" autocomplete="off" /></label>
            <label><span>Path style</span>
              <select bind:value={remoteForm.pathStyle}>
                <option value="path">Path (MinIO, R2, B2)</option>
                <option value="auto">Choose from the endpoint</option>
                <option value="virtual">Virtual-hosted (AWS)</option>
              </select>
            </label>
            <label><span>Access key id</span><input bind:value={remoteForm.accessKey} autocomplete="off" /></label>
            <label><span>Secret access key</span><input type="password" bind:value={remoteForm.secretKey} autocomplete="new-password" placeholder={editing ? 'leave blank to keep the stored secret' : ''} /></label>
            <label><span>Keep</span><input bind:value={remoteForm.keep} inputmode="numeric" /></label>
            <label class="check"><input type="checkbox" bind:checked={remoteForm.enabled} /> Send copies here</label>
          </div>
          <div class="formacts">
            <button type="button" class="btn" onclick={() => { formOpen = false; editing = null }}>Cancel</button>
            <button type="submit" class="btn primary" disabled={busy === 'remote'}>{busy === 'remote' ? 'Saving…' : 'Save destination'}</button>
          </div>
        </form>
      {/if}
    </section>

    <section class="block" aria-labelledby="copies-title">
      <h3 id="copies-title">Copies</h3>
      {#if status.items.length === 0}
        <p class="empty">No copy has been taken yet.</p>
      {:else}
        <div class="tablewrap">
          <table>
            <thead><tr><th>Backup</th><th>Taken</th><th>Size</th><th>Where</th><th></th></tr></thead>
            <tbody>
              {#each status.items as item (item.id)}
                <tr>
                  <td><code>{item.id}</code>{#if verified[item.id]}<span class="tag" class:bad={!verified[item.id].ok}>{verified[item.id].ok ? 'verified' : 'damaged'}</span>{/if}</td>
                  <td>{when(item.takenAt)}</td>
                  <td class="num">{bytes(item.bytes)}</td>
                  <td>
                    <span class="where">
                      {#if item.local}<span class="tag">this machine</span>{/if}
                      {#each item.remotes as r (r)}<span class="tag">{r}</span>{/each}
                    </span>
                  </td>
                  <td class="rowacts">
                    <button type="button" class="btn quiet" onclick={() => verify(item)} disabled={busy === `verify:${item.id}`}>
                      {busy === `verify:${item.id}` ? 'Verifying…' : 'Verify'}
                    </button>
                    <a class="btn quiet" href={backupDownloadUrl(admin, item.id)} download>Download</a>
                    {#if !admin && canEdit}
                      <button type="button" class="btn quiet" onclick={() => (restoreFor = item)}>Restore</button>
                    {/if}
                    {#if admin && item.local && canEdit}
                      <button type="button" class="btn quiet" onclick={() => (restoreFor = item)}>Restore…</button>
                    {/if}
                    {#if item.local && canEdit}
                      <button type="button" class="btn quiet danger" onclick={() => removeItem(item)} disabled={busy === `delete:${item.id}`}>Delete</button>
                    {/if}
                  </td>
                </tr>
                {#if verified[item.id] && !verified[item.id].ok}
                  <tr><td colspan="5" class="bad">{verified[item.id].problems.join(' ')}</td></tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#if admin}
        <p class="muted">An instance restore replaces the live database, so it runs with the server stopped: <code>reet backup restore --yes &lt;id&gt;</code>.</p>
      {/if}
      {#if restoreFor}
        <div class="banner" role="note">
          {#if admin}
            <strong>Restore {restoreFor.id}?</strong> Stop reet, then on its machine run:
            <div class="cmdline"><code>reet backup restore --yes {restoreFor.id}</code><CopyButton text={`reet backup restore --yes ${restoreFor.id}`} compact /></div>
            The database in place is moved aside, not deleted.
          {:else}
            <strong>Restore {restoreFor.id}?</strong> Your shelf goes back to what it held when that copy was taken.
            Copies taken since then are not deleted.
            <div class="formacts">
              <button type="button" class="btn" onclick={() => (restoreFor = null)}>Cancel</button>
              <button type="button" class="btn primary" onclick={async () => {
                if (!restoreFor) return
                busy = 'restore'
                try {
                  const r = await restoreUserBackup(restoreFor!.id)
                  saved = `Restored ${r.restored} skills from ${restoreFor!.id}.`
                  restoreFor = null
                  await load()
                } catch (e) {
                  error = e instanceof Error ? e.message : String(e)
                  onerror?.(e)
                } finally {
                  busy = ''
                }
              }} disabled={busy === 'restore'}>{busy === 'restore' ? 'Restoring…' : 'Restore my shelf'}</button>
            </div>
          {/if}
          {#if admin}<div class="formacts"><button type="button" class="btn" onclick={() => (restoreFor = null)}>Close</button></div>{/if}
        </div>
      {/if}
    </section>

    <section class="block" aria-labelledby="runs-title">
      <h3 id="runs-title">Recent runs</h3>
      {#if status.runs.length === 0}
        <p class="empty">Nothing has run yet.</p>
      {:else}
        <div class="tablewrap">
          <table>
            <thead><tr><th>Started</th><th>By</th><th>Outcome</th><th>Size</th><th>Off-site</th></tr></thead>
            <tbody>
              {#each status.runs as run (run.id)}
                <tr>
                  <td>{when(run.startedAt)}</td>
                  <td>{run.trigger}</td>
                  <td class:bad={run.state === 'failed'}>{run.state}</td>
                  <td class="num">{bytes(run.bytes)}</td>
                  <td class="offsite">{run.offsite || '—'}</td>
                </tr>
                {#if run.error}<tr><td colspan="5" class="bad">{run.error}</td></tr>{/if}
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  {/if}
</section>

<style>
  .backups { padding-top: 4px; }
  .head { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  h2 { margin: 8px 0 0; font: 500 2rem/1.15 var(--serif); letter-spacing: -0.03em; }
  .lede { margin: 8px 0 0; max-width: 62ch; color: var(--muted); }
  .actions { display: flex; gap: 8px; flex-wrap: wrap; }
  .notice, .banner { margin: 16px 0 0; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.good { border-color: var(--good); }
  .notice.bad, .banner { border-color: var(--bad); }
  .banner { line-height: 1.6; }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; margin-top: 22px; }
  .cards article { padding: 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .cards h3, .block h3 { margin: 0; color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.1em; text-transform: uppercase; }
  .block h3 { font-size: 1rem; letter-spacing: -0.01em; text-transform: none; color: var(--ink); font-family: var(--serif); font-weight: 500; }
  .cards p { margin: 6px 0 0; font: 500 1.35rem/1.2 var(--serif); overflow-wrap: anywhere; }
  .cards small { display: block; margin-top: 4px; color: var(--muted); font-size: 0.76rem; overflow-wrap: anywhere; }
  .block { margin-top: 22px; padding: clamp(16px, 3vw, 24px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .blockhead { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .muted { margin: 6px 0 0; color: var(--muted); font-size: 0.88rem; }
  .empty { margin: 14px 0 0; color: var(--muted); font-style: italic; }
  .findings { list-style: none; margin: 12px 0 0; padding: 0; display: grid; gap: 8px; }
  .findings li { display: flex; gap: 8px; align-items: flex-start; font-size: 0.86rem; }
  .sev { flex: none; padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.06em; }
  .findings li.error .sev { border-color: var(--bad); color: var(--bad); }
  .findings li.warning .sev { border-color: var(--heart); color: var(--heart); }
  .settings { display: flex; flex-wrap: wrap; gap: 14px; align-items: flex-end; margin-top: 14px; }
  .settings label { display: grid; gap: 4px; font-size: 0.8rem; color: var(--muted); }
  .settings input { width: 9rem; }
  .settings small { display: block; max-width: 18rem; color: var(--muted); }
  .bad { color: var(--bad) !important; overflow-wrap: anywhere; }
  .remotes { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 10px; }
  .remotes > li { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; }
  .remotes > li.failing { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .rwho { display: grid; gap: 2px; }
  .rwho small { color: var(--muted); font-size: 0.78rem; overflow-wrap: anywhere; }
  .racts { display: flex; flex-wrap: wrap; gap: 4px; }
  .tag { display: inline-flex; margin-left: 6px; padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.65rem; }
  .tag.bad { border-color: var(--bad); color: var(--bad); }
  .check, .copies { margin-top: 4px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); font-size: 0.8rem; }
  .check p { margin: 0; }
  .check ul, .copies ul { list-style: none; margin: 6px 0 0; padding: 0; display: grid; gap: 4px; }
  .copies li { display: flex; flex-wrap: wrap; gap: 8px; justify-content: space-between; }
  .copies code { font-size: 0.74rem; overflow-wrap: anywhere; }
  .copies span { color: var(--muted); }
  .good { color: var(--good); }
  .remoteform { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
  .remoteform h4 { margin: 0 0 10px; font: 500 1.15rem/1.2 var(--serif); }
  .grid2 { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 10px; }
  .grid2 label { display: grid; gap: 4px; font-size: 0.76rem; color: var(--muted); }
  .grid2 label.check { display: flex; align-items: center; gap: 8px; align-self: end; }
  .grid2 label.check input { width: 18px; height: 18px; min-height: 0; }
  .formacts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: 12px; }
  .tablewrap { margin-top: 12px; overflow-x: auto; }
  table { width: 100%; min-width: 640px; border-collapse: collapse; font-size: 0.82rem; }
  th { text-align: left; padding: 10px 8px; border-bottom: 1px solid var(--line); color: var(--muted); font-weight: 600; white-space: nowrap; }
  td { padding: 12px 8px; border-bottom: 1px solid var(--line); vertical-align: top; }
  td.num { font-variant-numeric: tabular-nums; white-space: nowrap; }
  td code { font-size: 0.75rem; overflow-wrap: anywhere; }
  td.rowacts { white-space: nowrap; text-align: right; }
  td.rowacts .btn { padding: 4px 8px; min-height: 32px; }
  .where { display: flex; flex-wrap: wrap; gap: 2px; }
  .offsite { font-family: var(--mono); font-size: 0.7rem; max-width: 22rem; overflow-wrap: anywhere; }
  .cmdline { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .cmdline code { padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); overflow-wrap: anywhere; }
  @media (max-width: 560px) {
    h2 { font-size: 1.6rem; }
    .settings { display: grid; grid-template-columns: 1fr 1fr; }
    .settings input { width: 100%; }
    .settings .btn { grid-column: 1 / -1; }
    .actions, .actions .btn { width: 100%; }
  }
</style>
