<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import {
    backupDownloadUrl, createBackupRemote, deleteBackup, deleteBackupRemote, deleteBackupRemoteCopy, getBackupStatus,
    listBackupRemoteCopies, pruneBackupCopies, runBackupNow, restoreUserBackup, saveBackupSettings, shipBackupCopies,
    testBackupRemote, testBackupRemoteDraft, updateBackupRemote, verifyBackup, getBuildInfo, getFence,
    type BackupCheck, type BackupCompression, type BackupDestination, type BackupItem, type BackupRemote,
    type BackupRemoteInput, type BackupStatus,
  } from './api'
  import CopyButton from './CopyButton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CollectionPanel from './CollectionPanel.svelte'
  import {
    fenceSentence, fenceWatch, moveFeatures,
    type Fence,
  } from './controllerMove'
  import {
    BACKUP_COPY_SCOPES, BACKUP_RUN_SCOPES, backupCopyCountLabel, backupCopyMatchesSearch, backupCopyScopeCounts,
    backupRemoteReady, backupRunLocation, backupRunCountLabel, backupRunMatchesSearch, backupRunScopeCounts, filterBackupCopies, filterBackupRuns,
    type BackupCopyScope, type BackupRunScope,
  } from './adminBackups'

  let {
    onerror, role = 'owner', admin = false, canWrite = true, timeZone = '', dateFormat = 'locale',
  }: { onerror?: (e: unknown) => void; role?: string; admin?: boolean; canWrite?: boolean; timeZone?: string; dateFormat?: string } = $props()

  const canEdit = $derived(admin ? role === 'owner' : canWrite)

  let status = $state<BackupStatus | null>(null)
  let loading = $state(true)
  let error = $state('')
  let saved = $state('')
  let busy = $state('')
  let form = $state({
    every: '24h', keep: '48', destination: 'both' as BackupDestination,
    compression: 'gzip' as BackupCompression, password: '', removePassword: false,
  })
  let checks = $state<Record<string, BackupCheck>>({})
  let verified = $state<Record<string, { ok: boolean; problems: string[] }>>({})
  let openCopies = $state<string | null>(null)
  let copies = $state<BackupItem[]>([])
  let copiesError = $state('')
  let copiesLoading = $state(false)
  let formOpen = $state(false)
  let editing = $state<BackupRemote | null>(null)
  let deleteFor = $state<BackupItem | null>(null)
  let deleteRemoteFor = $state<BackupRemote | null>(null)
  let restoreFor = $state<BackupItem | null>(null)
  let fence = $state<Fence>(fenceWatch.get())
  const isOwner = $derived(admin && role === 'owner')
  let copySearch = $state('')
  let copyScope = $state<BackupCopyScope>('all')
  let runSearch = $state('')
  let runScope = $state<BackupRunScope>('all')

  type Section = 'overview' | 'copies' | 'destinations' | 'schedule' | 'runs'
  let section = $state<Section>('overview')
  const sections = $derived.by(() => {
    const list: { key: Section; label: string; count?: number }[] = [
      { key: 'overview', label: 'Overview', count: status?.findings.filter(f => f.severity !== 'info').length },
      { key: 'copies', label: 'Copies', count: status?.items.length },
      { key: 'destinations', label: 'Destinations', count: status?.remotes.length },
      { key: 'schedule', label: 'Schedule' },
      { key: 'runs', label: 'Runs', count: status?.runs.length },
    ]
    return list
  })
  const current = $derived<Section>(sections.some((s) => s.key === section) ? section : 'overview')

  function tabKeys(e: KeyboardEvent) {
    const keys = sections.map((s) => s.key)
    const i = keys.indexOf(current)
    let n = i
    if (e.key === 'ArrowRight') n = (i + 1) % keys.length
    else if (e.key === 'ArrowLeft') n = (i - 1 + keys.length) % keys.length
    else if (e.key === 'Home') n = 0
    else if (e.key === 'End') n = keys.length - 1
    else return
    e.preventDefault()
    section = keys[n]
    document.getElementById(`backups-tab-${keys[n]}`)?.focus()
  }

  let remoteError = $state('')
  let remoteCheck = $state<BackupCheck | null>(null)

  const emptyRemote = () => ({
    name: '', endpoint: '', region: '', bucket: '', prefix: 'reet/', accessKey: '', secretKey: '',
    pathStyle: 'path' as 'auto' | 'path' | 'virtual', keep: String(admin ? 90 : 14), enabled: true,
  })
  let remoteForm = $state(emptyRemote())

  onMount(() => {
    void load()
    // A write refused elsewhere on the page, or the fence raised below, reaches the banner by the same road.
    return fenceWatch.subscribe((next) => (fence = next))
  })

  async function load(quiet = false) {
    if (!quiet) loading = true
    try {
      status = await getBackupStatus(admin)
      if (admin) {
        const features = moveFeatures((await getBuildInfo().catch(() => null))?.features)
        // An admin may read the fence; anyone else is refused, which says nothing either way and changes nothing.
        if (features.fence) await getFence().then((f) => fenceWatch.set(f)).catch(() => {})
      }
      const every = status.settings.everySeconds
      form = {
        every: everyText(every), keep: String(status.settings.keep), destination: status.settings.destination || 'both',
        compression: status.settings.compression || 'gzip', password: '', removePassword: false,
      }
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
  const scopeWord = $derived(admin ? 'the instance' : 'your library')

  async function saveSettings(e: SubmitEvent) {
    e.preventDefault()
    if (!canEdit) return
    if (everyBad || keepBad) {
      error = 'Fix the schedule or retention before saving.'
      return
    }
    if (!form.removePassword && form.password !== '' && form.password.trim().length < 12) {
      error = 'A copy password needs at least 12 characters, or leave it empty.'
      return
    }
    busy = 'settings'
    saved = ''
    error = ''
    try {
      const password = form.removePassword ? '' : form.password.trim() ? form.password : undefined
      await saveBackupSettings(admin, everySeconds(form.every), Number(form.keep), form.destination, form.compression, password)
      saved = 'Saved. It applies to the next copy.'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  function protectionText(): string {
    const compression = status?.settings.compression === 'gzip' ? 'gzip' : 'no compression'
    return `${status?.settings.hasPassword ? 'Password' : 'Instance key'} + ${compression}`
  }

  function destinationText(destination: BackupDestination): string {
    if (destination === 'local') return 'This machine only'
    if (destination === 'remote') return 'Off-site only'
    return 'This machine and off-site'
  }

  async function runNow() {
    busy = 'run'
    saved = ''
    try {
      const item = await runBackupNow(admin)
      saved = `Took ${item.id}. ${item.local ? (item.remotes.length ? 'Stored on this machine and off-site.' : 'Stored on this machine.') : 'Stored off-site; no local copy retained.'}`
      await load()
      if (status?.settings.destination === 'remote' && item.local && !item.remotes.length) {
        saved = ''
        error = 'The off-site upload failed. A recovery copy was kept on this machine. Check Destinations and retry the upload.'
      }
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
      let result = await verifyBackup(admin, item.id)
      if (!result.ok && result.problems.join(' ').toLowerCase().includes('password')) {
        const password = prompt('This copy is protected with a password. Enter it to verify the copy:')
        if (password) result = await verifyBackup(admin, item.id, password)
      }
      verified = { ...verified, [item.id]: result }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function removeItem(item: BackupItem) {
    deleteFor = null
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
    remoteError = ''
    remoteCheck = null
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

  function remoteInput(): BackupRemoteInput {
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
    if (editing) input.originalName = editing.name
    if (remoteForm.secretKey.trim()) input.secretKey = remoteForm.secretKey.trim()
    return input
  }

  const remoteNeedsSecret = $derived(
    !!editing && remoteForm.endpoint.trim().replace(/\/$/, '').toLowerCase() !== editing.endpoint.replace(/\/$/, '').toLowerCase() && !remoteForm.secretKey.trim(),
  )
  function remoteProblem(): string {
    if (!remoteForm.name.trim()) return 'Give the destination a name.'
    if (!remoteForm.endpoint.trim()) return 'Give the endpoint URL.'
    if (!remoteForm.bucket.trim()) return 'Name the bucket.'
    if (!remoteForm.accessKey.trim()) return 'Enter the access key id.'
    if (!editing && !remoteForm.secretKey.trim()) return 'Enter the secret access key.'
    if (remoteNeedsSecret) return 'The stored secret is for the old endpoint. Enter it again to use it with the new one.'
    return ''
  }

  async function testDraft() {
    const problem = remoteProblem()
    if (problem) {
      remoteError = problem
      return
    }
    remoteError = ''
    remoteCheck = null
    busy = 'remote-test'
    try {
      remoteCheck = await testBackupRemoteDraft(admin, remoteInput())
    } catch (e) {
      remoteError = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function saveRemote(e: SubmitEvent) {
    e.preventDefault()
    const problem = remoteProblem()
    if (problem) {
      remoteError = problem
      return
    }
    remoteError = ''
    busy = 'remote'
    try {
      if (editing) await updateBackupRemote(admin, editing.name, remoteInput())
      else await createBackupRemote(admin, remoteInput())
      formOpen = false
      editing = null
      remoteCheck = null
      saved = 'Destination saved.'
      await load()
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e)
      remoteError = message
      error = message
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function removeRemote(r: BackupRemote) {
    deleteRemoteFor = null
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

  async function copyOffsite() {
    busy = 'offsite'
    saved = ''
    try {
      const out = await shipBackupCopies(admin)
      const failed = Object.keys(out.errors).length
      saved = out.sent.length
        ? `Copied ${out.sent.length} ${out.sent.length === 1 ? 'copy' : 'copies'} off-site.${failed ? ` ${failed} destination(s) refused: ${Object.values(out.errors).join('; ')}` : ''}`
        : failed
          ? `Nothing was copied. ${Object.values(out.errors).join('; ')}`
          : 'Every destination already holds every copy.'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function applyRetention() {
    if (!confirm('Apply retention now? This deletes copies older than the counts set here and at each destination.')) return
    busy = 'prune'
    saved = ''
    try {
      const out = await pruneBackupCopies(admin)
      const remotes = out.remotes ?? []
      const remote = remotes.reduce((n, r) => n + (r.removed?.length ?? 0), 0)
      const total = (out.removed?.length ?? 0) + remote
      const failed = remotes.filter((r) => r.error)
      saved = `Removed ${total} ${total === 1 ? 'copy' : 'copies'}.${failed.length ? ` ${failed.map((r) => `${r.name}: ${r.error}`).join('; ')}` : ''}`
      await load()
      if (openCopies) await loadCopies(openCopies, true)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function removeRemoteCopy(name: string, item: BackupItem) {
    if (!confirm(`Remove ${item.id} from ${name}? What this machine holds is left alone.`)) return
    busy = `remote-copy:${item.id}`
    try {
      await deleteBackupRemoteCopy(admin, name, item.id)
      await loadCopies(name, true)
      await load()
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
    await loadCopies(name)
  }

  // Only the latest request may fill the list: one destination's copies must not show under another.
  let copiesSeq = 0
  async function loadCopies(name: string, quiet = false) {
    const seq = ++copiesSeq
    if (!quiet) {
      copiesLoading = true
      copiesError = ''
    }
    try {
      const items = (await listBackupRemoteCopies(admin, name)).items
      if (seq === copiesSeq && openCopies === name) copies = items
    } catch (e) {
      if (quiet) throw e
      if (seq !== copiesSeq) return
      copies = []
      copiesError = e instanceof Error ? e.message : String(e)
    } finally {
      if (seq === copiesSeq) copiesLoading = false
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

  const copyItems = $derived(status?.items ?? [])
  const locations = $derived(backupCopyScopeCounts(copyItems))
  const readyRemotes = $derived(status?.remotes.filter(backupRemoteReady) ?? [])
  const attention = $derived(status?.findings.filter(f => f.severity !== 'info') ?? [])
  const information = $derived(status?.findings.filter(f => f.severity === 'info') ?? [])
  const latestCopy = $derived(copyItems.reduce<BackupItem | null>((latest, item) => !latest || item.takenAt > latest.takenAt ? item : latest, null))
  const searchedCopies = $derived(copyItems.filter((item) => backupCopyMatchesSearch(item, copySearch, verified)))
  const copyCounts = $derived(backupCopyScopeCounts(searchedCopies, verified))
  const shownCopies = $derived(filterBackupCopies(copyItems, copyScope, copySearch, verified))
  const copyCountLabel = $derived(backupCopyCountLabel(shownCopies.length, copyItems.length))
  const runItems = $derived(status?.runs ?? [])
  const searchedRuns = $derived(runItems.filter((run) => backupRunMatchesSearch(run, runSearch)))
  const runCounts = $derived(backupRunScopeCounts(searchedRuns))
  const shownRuns = $derived(filterBackupRuns(runItems, runScope, runSearch))
  const runCountLabel = $derived(backupRunCountLabel(shownRuns.length, runItems.length))

  function clearCopyFilters() {
    copySearch = ''
    copyScope = 'all'
  }

  function clearRunFilters() {
    runSearch = ''
    runScope = 'all'
  }
</script>

<section class="backups" aria-label="Backups">
  <div class="head">
    <div>
      <p class="lede">
        {admin
          ? 'Encrypted copies of the whole database. See where each copy is stored and manage your backup policy.'
          : 'Encrypted copies of your library. Choose local storage, an off-site destination, or both.'}
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
    <Skeleton label="Loading backups…" shape="rows" count={4} />
  {:else if !status}
    <p class="notice bad" role="alert">{error || 'Could not load the backups.'}</p>
  {:else}
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {#if saved}<p class="notice good" role="status">{saved}</p>{/if}

    {#if admin && fence.fenced}
      <div class="banner fenced" role="status">
        <strong>This controller is fenced.</strong>
        {fenceSentence(fence, when)}
        Signing in, reading, and taking, checking and downloading copies still work.
        {#if isOwner}<a href="/admin/move">Open Move to manage the fence.</a>{:else}Only the owner can lift the fence.{/if}
      </div>
    {/if}

    {#if !status.enabled}
      <div class="banner" role="alert">
        <strong>Nothing is being backed up.</strong> The backup key is missing or unusable. Set
        <code>REET_BACKUP_KEY</code> to a value from <code>reet backup keygen</code>, keep a copy away from this
        machine, and restart reet.
      </div>
    {/if}

    <div class="subtabs" role="tablist" aria-label="Backup sections" tabindex="-1" onkeydown={tabKeys}>
      {#each sections as s (s.key)}
        <button type="button" role="tab" class="subtab" id={`backups-tab-${s.key}`} aria-selected={current === s.key}
          aria-controls="backups-panel" tabindex={current === s.key ? 0 : -1} onclick={() => (section = s.key)}>
          {s.label}{#if s.count}<small>{s.count}</small>{/if}
        </button>
      {/each}
    </div>

    <div role="tabpanel" id="backups-panel" aria-labelledby={`backups-tab-${current}`}>
    {#if current === 'overview'}
    <section class="health" class:attention={!status.enabled || attention.length > 0 || !!status.unreachable?.length || status.settings.lastState === 'failed'} aria-label="Backup status">
      <div>
        <span class="eyebrow">Backup status</span>
        <h3>{!status.enabled ? 'Backups are unavailable' : attention.length || status.unreachable?.length || status.settings.lastState === 'failed'
          ? 'Backups need attention'
          : locations.remote ? 'Off-site copies available' : copyItems.length ? 'Copies available on this machine' : 'Ready for your first backup'}</h3>
        <p>{destinationText(status.settings.destination)} · {status.settings.everySeconds ? `Every ${everyText(status.settings.everySeconds)}` : 'Manual backups only'}</p>
      </div>
      <button type="button" class="btn" onclick={() => (section = 'schedule')}>Manage schedule</button>
    </section>

    <div class="cards" aria-label="Stored copies">
      <article>
        <h3>Off-site copies</h3>
        <p>{locations.remote}</p>
        <small>Across {readyRemotes.length} ready {readyRemotes.length === 1 ? 'destination' : 'destinations'}</small>
        <button type="button" class="text-action" onclick={() => { copyScope = 'remote'; copySearch = ''; section = 'copies' }}>View off-site copies →</button>
      </article>
      <article>
        <h3>On this machine</h3>
        <p>{locations.local}</p>
        <small>{status.settings.destination === 'remote' ? 'Successful off-site uploads leave no local copy.' : `Local retention: ${status.settings.keep ? `newest ${status.settings.keep}` : 'keep every copy'}`}</small>
        <button type="button" class="text-action" onclick={() => { copyScope = 'local'; copySearch = ''; section = 'copies' }}>View local copies →</button>
      </article>
      <article>
        <h3>Latest copy</h3>
        <p class="latest">{latestCopy ? when(latestCopy.takenAt) : 'No copies yet'}</p>
        <small>{copyItems.length} unique {copyItems.length === 1 ? 'copy' : 'copies'} in total · {locations.local && locations.remote ? 'A copy can be in both locations.' : 'Includes all listed locations.'}</small>
        <button type="button" class="text-action" onclick={() => (section = 'runs')}>View run history →</button>
      </article>
    </div>

    {#if attention.length || status.unreachable?.length || status.settings.lastState === 'failed'}
      <section class="block attention-block" aria-labelledby="findings-title">
        <div class="blockhead"><h3 id="findings-title">Needs attention</h3><button type="button" class="btn" onclick={() => (section = 'destinations')}>Check destinations</button></div>
        <ul class="findings">
          {#each attention as f, i (f.code + i)}
            <li class={f.severity}><span class="sev">{f.severity}</span><span>{f.message}</span></li>
          {/each}
          {#if status.unreachable?.length}<li class="error"><span class="sev">error</span><span>Could not list {status.unreachable.join(', ')}. Off-site counts may be incomplete; check the destination connection.</span></li>{/if}
          {#if status.settings.lastState === 'failed' && status.settings.lastError}<li class="error"><span class="sev">error</span><span>Last run: {status.settings.lastError}</span></li>{/if}
        </ul>
      </section>
    {/if}

    <div class="overview-grid">
      <section class="block" aria-labelledby="policy-title">
        <div class="blockhead"><h3 id="policy-title">Backup policy</h3><button type="button" class="text-action" onclick={() => (section = 'schedule')}>Edit policy →</button></div>
        <dl class="policy">
          <div><dt>Storage</dt><dd>{destinationText(status.settings.destination)}</dd></div>
          <div><dt>Next run</dt><dd>{status.settings.everySeconds ? when(status.settings.nextAt) : 'Manual only'}</dd></div>
          <div><dt>Protection</dt><dd>{protectionText()}</dd></div>
          {#if status.settings.destination !== 'remote'}<div><dt>Local retention</dt><dd>{status.settings.keep ? `Newest ${status.settings.keep} copies` : 'Every copy'}</dd></div>{/if}
        </dl>
        {#if status.settings.destination === 'remote'}<p class="policy-note">Copies are removed from this machine after a successful off-site upload. If all uploads fail, a recovery copy stays here. Changing this setting does not remove older local copies.</p>{/if}
      </section>
      <section class="block" aria-labelledby="destinations-summary-title">
        <div class="blockhead"><h3 id="destinations-summary-title">Off-site destinations</h3><button type="button" class="text-action" onclick={() => (section = 'destinations')}>Manage →</button></div>
        {#if status.remotes.length}
          <ul class="destination-summary">
            {#each status.remotes as r (r.name)}
              <li><div><strong>{r.name}</strong><small>{r.disabled ? 'Uploads paused' : r.lastUploadAt ? `Last upload ${when(r.lastUploadAt)}` : 'No successful upload recorded'}</small></div><span class="tag" class:good={backupRemoteReady(r) && !r.lastError} class:bad={!r.disabled && (!!r.problem || !!r.lastError)}>{r.disabled ? 'Disabled' : r.lastError ? 'Upload failed' : backupRemoteReady(r) ? 'Ready' : 'Needs setup'}</span></li>
            {/each}
          </ul>
        {:else}<p class="empty">Add an S3-compatible destination to keep copies off this machine.</p>{/if}
      </section>
    </div>
    <details class="block recovery">
      <summary>Encryption and recovery</summary>
      <p class="muted">{status.settings.hasPassword ? 'New copies open with your stored copy password. Keep it somewhere safe.' : 'New copies open with the instance backup key. Keep a copy of that key away from this machine.'}</p>
      {#if admin}<p class="muted">Key fingerprint: <code>{status.key.fingerprint || '—'}</code>{#if status.key.path} · <code>{status.key.path}</code>{/if}</p>{/if}
      {#if information.length}<ul class="findings">{#each information as f, i (f.code + i)}<li><span>{f.message}</span></li>{/each}</ul>{/if}
    </details>
    {:else if current === 'copies'}
      {#if status.unreachable?.length}<p class="notice bad" role="alert">Could not list {status.unreachable.join(', ')}. Off-site counts may be incomplete.</p>{/if}
      {#if restoreFor}
        <div class="banner" role="note">
          {#if admin}
            <strong>Restore {restoreFor.id}?</strong> Stop reet, then on its machine run:
            <div class="cmdline"><code>reet backup restore --yes {restoreFor.id}</code><CopyButton text={`reet backup restore --yes ${restoreFor.id}`} compact /></div>
            The database in place is moved aside, not deleted.
          {:else}
            <strong>Restore {restoreFor.id}?</strong> Matching skills have their saved flags, notes and tags replaced. Skills added since this copy remain in your library. Lists, teams and account settings are not restored.
            Copies taken since then are not deleted.
            <div class="formacts">
              <button type="button" class="btn" onclick={() => (restoreFor = null)}>Cancel</button>
              <button type="button" class="btn primary" onclick={async () => {
                if (!restoreFor) return
                busy = 'restore'
                try {
                  let r
                  try {
                    r = await restoreUserBackup(restoreFor!.id)
                  } catch (e) {
                    const message = e instanceof Error ? e.message : String(e)
                    if (!message.toLowerCase().includes('password')) throw e
                    const password = prompt('This copy is protected with a password. Enter it to restore your library:')
                    if (!password) throw e
                    r = await restoreUserBackup(restoreFor!.id, password)
                  }
                  saved = `Restored ${r.restored} skills from ${restoreFor!.id}.`
                  restoreFor = null
                  await load()
                } catch (e) {
                  error = e instanceof Error ? e.message : String(e)
                  onerror?.(e)
                } finally {
                  busy = ''
                }
              }} disabled={busy === 'restore'}>{busy === 'restore' ? 'Restoring…' : 'Restore my library'}</button>
            </div>
          {/if}
          {#if admin}<div class="formacts"><button type="button" class="btn" onclick={() => (restoreFor = null)}>Close</button></div>{/if}
        </div>
      {/if}

    <CollectionPanel label="Backup copies" count={copyCountLabel}>
      {#snippet toolbar()}
        <div class="copy-toolbar">
          <label class="search">Search copies
            <input type="search" bind:value={copySearch} placeholder="Copy, destination or problem" />
          </label>
          <div class="scopes" role="group" aria-label="Show copies">
            {#each BACKUP_COPY_SCOPES as item}
              <button type="button" class="scope" aria-pressed={copyScope === item.key} onclick={() => (copyScope = item.key)}>{item.label}<small>{copyCounts[item.key]}</small></button>
            {/each}
          </div>
        </div>
      {/snippet}

      {#if copyItems.length === 0}
        <div class="state-block">
          <h3>No copy has been taken yet.</h3>
          <p>Take one now, or wait for the schedule.</p>
        </div>
      {:else if shownCopies.length === 0}
        <div class="state-block">
          <h3>No copy matches that search or filter.</h3>
          <p>Try another search, or show every copy.</p>
          <button type="button" class="btn" onclick={clearCopyFilters}>Clear filters</button>
        </div>
      {:else}
        <ul class="copy-list">
          {#each shownCopies as item (item.id)}
            <li>
              <div class="copy-main">
                <span class="copy-id"><code>{item.id}</code>{#if admin && item.fenced}<span class="tag">taken fenced</span>{/if}{#if verified[item.id]}<span class="tag" class:bad={!verified[item.id].ok}>{verified[item.id].ok ? 'verified' : 'damaged'}</span>{/if}</span>
                <small>{when(item.takenAt)} · {bytes(item.bytes)}{item.version ? ` · ${item.version}` : ''}</small>
                {#if verified[item.id] && !verified[item.id].ok}<small class="bad">{verified[item.id].problems.join(' ')}</small>{/if}
              </div>
              <div class="copy-where">
                {#if item.local}<span class="tag">this machine</span>{/if}
                {#each item.remotes as remote (remote)}<span class="tag">{remote}</span>{/each}
              </div>
              <div class="copy-actions">
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
                  <button type="button" class="btn quiet danger" onclick={() => (deleteFor = item)} disabled={busy === `delete:${item.id}`}>Delete</button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </CollectionPanel>

      {#if admin}
        <p class="muted">An instance restore replaces the live database, so it runs with the server stopped: <code>reet backup restore --yes &lt;id&gt;</code>.</p>
      {/if}
    {:else if current === 'destinations'}
    <section class="block" aria-labelledby="destinations-title">
      <div class="blockhead">
        <div>
          <h3 id="destinations-title">Off-site destinations</h3>
          <p class="muted">Any S3-compatible service: AWS S3, Cloudflare R2, Backblaze B2, MinIO, Garage. The secret is sealed in the database.</p>
        </div>
        <div class="actions">{#if canEdit}
          {#if status.remotes.length > 0 && status.settings.destination !== 'local'}
            <button type="button" class="btn" onclick={copyOffsite} disabled={busy === 'offsite'}>
              {busy === 'offsite' ? 'Copying…' : 'Copy offsite now'}
            </button>
            <button type="button" class="btn" onclick={applyRetention} disabled={busy === 'prune'}>
              {busy === 'prune' ? 'Applying…' : 'Apply retention now'}
            </button>
          {/if}
          <button type="button" class="btn" onclick={() => editRemote()}>Add destination</button>
        {/if}</div>
      </div>

      {#if status.remotes.length === 0}
        <p class="empty">
          No destination is configured.
          {admin ? 'Add one, or set REET_BACKUP_S3_* in the environment.' : 'Add one to copy your library off this machine.'}
        </p>
      {:else}
        <ul class="remotes">
          {#each status.remotes as r (r.name)}
            <li class:failing={!!r.lastError}>
              <div class="rwho">
                <div class="destination-name"><strong>{r.name}</strong><span class="tag" class:good={backupRemoteReady(r) && !r.lastError}>{r.disabled ? 'Disabled' : r.lastError ? 'Upload failed' : backupRemoteReady(r) ? 'Ready' : 'Needs setup'}</span></div>
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
                    <button type="button" class="btn quiet danger" onclick={() => (deleteRemoteFor = r)} disabled={busy === `remote:${r.name}`}>Remove</button>
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
                        <li>
                          <code>{c.id}</code><span>{when(c.takenAt)} · {bytes(c.bytes)}</span>
                          {#if canEdit}
                            <button type="button" class="btn quiet danger" onclick={() => removeRemoteCopy(r.name, c)}
                              disabled={busy === `remote-copy:${c.id}`}>Remove</button>
                          {/if}
                        </li>
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
          <p class="muted">Test the destination here before saving it. The test writes a small object under the prefix, reads it back and deletes it again.</p>
          <div class="grid2">
            <label><span>Name</span><input bind:value={remoteForm.name} placeholder="offsite" autocomplete="off" required /></label>
            <label><span>Endpoint</span><input bind:value={remoteForm.endpoint} placeholder="https://s3.eu-west-2.amazonaws.com" autocomplete="off" required /></label>
            <label><span>Region</span><input bind:value={remoteForm.region} placeholder="eu-west-2" autocomplete="off" /></label>
            <label><span>Bucket</span><input bind:value={remoteForm.bucket} placeholder="my-reet-backups" autocomplete="off" required /></label>
            <label><span>Prefix</span><input bind:value={remoteForm.prefix} placeholder="reet/" autocomplete="off" /></label>
            <label><span>Path style</span>
              <select bind:value={remoteForm.pathStyle}>
                <option value="path">Path (MinIO, R2, B2)</option>
                <option value="auto">Choose from the endpoint</option>
                <option value="virtual">Virtual-hosted (AWS)</option>
              </select>
            </label>
            <label><span>Access key id</span><input bind:value={remoteForm.accessKey} autocomplete="off" required /></label>
            <label><span>Secret access key</span><input type="password" bind:value={remoteForm.secretKey} autocomplete="new-password" placeholder={editing ? 'leave blank to keep the stored secret' : ''} aria-invalid={remoteNeedsSecret || undefined} />
              <small class:bad={remoteNeedsSecret}>{remoteNeedsSecret ? 'The endpoint changed; enter the secret again.' : editing ? 'Stored already. Type a new one to replace it.' : 'Required. Sealed with the backup key and never shown again.'}</small>
            </label>
            <label><span>Keep</span><input bind:value={remoteForm.keep} inputmode="numeric" /></label>
            <label class="check"><input type="checkbox" bind:checked={remoteForm.enabled} /> Send copies here</label>
          </div>
          {#if remoteCheck}
            <div class="check" aria-live="polite">
              <p class:good={remoteCheck.ok} class:bad={!remoteCheck.ok}>
                {remoteCheck.ok ? 'The destination can be written, read and cleaned up.' : remoteCheck.error}
              </p>
              <ul>
                {#each remoteCheck.steps as st (st.step)}
                  <li><strong>{st.step}</strong> {st.ok ? 'ok' : 'failed'} — {st.detail}</li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if remoteError}<p class="notice bad" role="alert">{remoteError}</p>{/if}
          <div class="formacts">
            <button type="button" class="btn" onclick={() => { formOpen = false; editing = null; remoteError = ''; remoteCheck = null }}>Cancel</button>
            <button type="button" class="btn" onclick={testDraft} disabled={busy === 'remote-test' || busy === 'remote'}>
              {busy === 'remote-test' ? 'Testing…' : 'Test'}
            </button>
            <button type="submit" class="btn primary" disabled={busy === 'remote' || busy === 'remote-test'}>{busy === 'remote' ? 'Saving…' : 'Save destination'}</button>
          </div>
        </form>
      {/if}
    </section>
    {:else if current === 'schedule'}
    <section class="block" aria-labelledby="schedule-title">
      <h3 id="schedule-title">Schedule and retention</h3>
      <p class="muted">Choose when backups run, where they are stored, and how they are protected.</p>
      <form class="settings" onsubmit={saveSettings}>
        <fieldset><legend>When and where</legend><div class="settings-grid">
        <label>
          <span>Every</span>
          <input bind:value={form.every} placeholder="24h" disabled={!canEdit} aria-invalid={everyBad || undefined} />
          <small class:bad={everyBad}>{everyBad ? 'Use 30m, 1h, 24h or 7d; 0 is off.' : 'The shortest schedule is a minute.'}</small>
        </label>
        <label>
          <span>Where copies go</span>
          <select bind:value={form.destination} disabled={!canEdit}>
            <option value="both">This machine and off-site</option>
            <option value="local">This machine only</option>
            <option value="remote">Off-site only</option>
          </select>
          <small class:bad={form.destination === 'remote' && readyRemotes.length === 0}>
            {form.destination === 'local'
              ? 'Lost with this machine. Not recommended.'
              : form.destination === 'remote'
                ? (readyRemotes.length > 0
                  ? 'Removed locally after a successful upload. Kept locally only if all uploads fail.'
                  : 'Add and test a destination in Destinations before running a backup.')
                : 'Every copy is on this machine and at each enabled destination.'}
          </small>
        </label>
        <label>
          <span>Local copies to keep</span>
          <input bind:value={form.keep} inputmode="numeric" disabled={!canEdit} aria-invalid={keepBad || undefined} />
          <small class:bad={keepBad}>{keepBad ? 'A whole number, 0 or more.' : form.destination === 'remote' ? 'Applies to existing local copies and recovery copies after failed uploads. 0 keeps all.' : '0 keeps every copy on this machine.'}</small>
        </label>
        </div><p class="policy-note">Off-site retention is set separately on each destination.</p></fieldset>
        <fieldset><legend>Encryption and compression</legend><div class="settings-grid">
        <label>
          <span>Compression</span>
          <select bind:value={form.compression} disabled={!canEdit}>
            <option value="gzip">gzip</option>
            <option value="none">No compression</option>
          </select>
          <small>Compressed before the copy is sealed.</small>
        </label>
        <label>
          <span>Copy password</span>
          <input type="password" bind:value={form.password} autocomplete="new-password" disabled={!canEdit}
            placeholder={status.settings.hasPassword ? 'stored; leave blank to keep' : 'optional, at least 12 characters'} />
          <small class:bad={form.password !== '' && form.password.trim().length < 12}>
            {form.password !== '' && form.password.trim().length < 12
              ? 'At least 12 characters, or leave it empty.'
              : status.settings.hasPassword
                ? 'A password is stored. With one, a copy opens with that password instead of the instance key.'
                : 'Optional. With one, a copy opens with that password alone.'}
          </small>
        </label>
        {#if status.settings.hasPassword && canEdit}
          <label class="check">
            <input type="checkbox" bind:checked={form.removePassword} />
            Remove the stored password
          </label>
        {/if}
        </div></fieldset>
        <div class="settings-footer">
        {#if canEdit}
          <button type="submit" class="btn primary" disabled={busy === 'settings' || everyBad || keepBad}>
            {busy === 'settings' ? 'Saving…' : 'Save backup policy'}
          </button>
        {:else}
          <p class="muted">{admin ? 'Only the instance owner can change the schedule.' : 'Read-only accounts can see backups but not change them.'}</p>
        {/if}
        </div>
      </form>
    </section>
    {:else if current === 'runs'}
    <CollectionPanel label="Recent runs" count={runCountLabel}>
      {#snippet toolbar()}
        <div class="run-toolbar">
          <label class="search">Search runs
            <input type="search" bind:value={runSearch} placeholder="Trigger, outcome, destination or error" />
          </label>
          <div class="scopes" role="group" aria-label="Show runs">
            {#each BACKUP_RUN_SCOPES as item}
              <button type="button" class="scope" aria-pressed={runScope === item.key} onclick={() => (runScope = item.key)}>{item.label}<small>{runCounts[item.key]}</small></button>
            {/each}
          </div>
        </div>
      {/snippet}

      {#if runItems.length === 0}
        <div class="state-block">
          <h3>Nothing has run yet.</h3>
          <p>A scheduled or manual copy appears here with its outcome.</p>
        </div>
      {:else if shownRuns.length === 0}
        <div class="state-block">
          <h3>No run matches that search or outcome.</h3>
          <p>Try another search, or show every run.</p>
          <button type="button" class="btn" onclick={clearRunFilters}>Clear filters</button>
        </div>
      {:else}
        <ul class="run-list">
          {#each shownRuns as run (run.id)}
            <li>
              <div class="run-main">
                <strong>{run.trigger}</strong>
                <small>{when(run.startedAt)}</small>
                {#if run.error}<small class="bad">{run.error}</small>{/if}
              </div>
              <div class="run-meta">
                <span class="state {run.state}">{run.state}</span>
                <small>{bytes(run.bytes)}</small>
                <small>{backupRunLocation(run)}</small>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </CollectionPanel>

    {/if}
    </div>
  {/if}
</section>

{#if deleteFor}
  <ConfirmDialog title="Delete this local backup?" body={`Delete ${deleteFor.id} from this machine? Copies at destinations are left alone.`} confirmLabel="Delete backup" danger oncancel={() => (deleteFor = null)} onconfirm={() => { if (deleteFor) void removeItem(deleteFor) }} />
{/if}
{#if deleteRemoteFor}
  <ConfirmDialog title="Remove this destination?" body={`Remove ${deleteRemoteFor.name}? Existing copies in its bucket are left alone.`} confirmLabel="Remove destination" danger oncancel={() => (deleteRemoteFor = null)} onconfirm={() => { if (deleteRemoteFor) void removeRemote(deleteRemoteFor) }} />
{/if}

<style>
  .health { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; margin-top: 24px; padding: 24px; border: 1px solid color-mix(in srgb, var(--good) 40%, var(--line)); border-radius: 12px; background: color-mix(in srgb, var(--good) 5%, var(--card)); }
  .health.attention { border-color: color-mix(in srgb, var(--warn) 50%, var(--line)); background: var(--card); }
  .eyebrow { color: var(--muted); font-size: 0.875rem; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; }
  .health h3 { font: 500 clamp(1.3rem, 2vw, 1.7rem)/1.25 var(--serif); margin: 6px 0; }
  .health p { color: var(--muted); font-size: 0.9rem; margin: 0; }
  .text-action { display: inline-block; padding: 4px 0; border: 0; background: transparent; color: var(--accent-text); font: inherit; font-size: 0.875rem; text-align: left; cursor: pointer; }
  .text-action:hover { text-decoration: underline; text-underline-offset: 3px; }
  .cards .text-action { margin-top: 12px; }
  .cards p.latest { font-size: 1.1rem; line-height: 1.5; }
  .overview-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
  .overview-grid .block { min-width: 0; }
  .policy { display: grid; gap: 12px; margin: 20px 0 0; font-size: 0.9rem; }
  .policy > div { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 4px 16px; }
  .policy dt { color: var(--muted); }
  .policy dd { margin: 0; overflow-wrap: anywhere; }
  .policy-note { margin: 16px 0 0; color: var(--muted); font-size: 0.875rem; line-height: 1.55; }
  .destination-summary { list-style: none; margin: 12px 0 0; padding: 0; }
  .destination-summary li { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: start; gap: 8px; padding: 12px 0; border-bottom: 1px solid var(--line-soft); }
  .destination-summary small { display: block; margin-top: 4px; color: var(--muted); font-size: 0.875rem; }
  .recovery summary { cursor: pointer; font-weight: 600; }
  .recovery code { overflow-wrap: anywhere; }
  .destination-name { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  @media (max-width: 760px) { .overview-grid { grid-template-columns: 1fr; gap: 0; } }
  @media (pointer: coarse) { .text-action { min-height: 44px; } }
  .backups { padding-top: 4px; }
  .head { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .lede { margin: 0; max-width: 62ch; color: var(--muted); }
  .actions { display: flex; gap: 8px; flex-wrap: wrap; }
  .notice, .banner { margin: 16px 0 0; padding: 12px 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .notice.good { border-color: var(--good); }
  .notice.bad, .banner { border-color: var(--bad); }
  .banner { line-height: 1.6; }
  .cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-top: 24px; }
  .cards article { padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .cards h3, .block h3 { margin: 0; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .block h3 { font-size: 1rem; letter-spacing: -0.01em; color: var(--ink); font-family: var(--serif); font-weight: 500; }
  .cards p { margin: 6px 0 0; font: 500 1.35rem/1.2 var(--serif); overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
  .cards small { display: block; margin-top: 4px; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
  .subtabs { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 24px; border-bottom: 1px solid var(--line); }
  .subtab { min-height: 40px; padding: 8px 14px; margin-bottom: -1px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--muted); font: inherit; font-size: 0.95rem; cursor: pointer; }
  .subtab:hover { color: var(--ink); }
  .subtab[aria-selected='true'] { border-bottom-color: var(--accent); color: var(--ink); font-weight: 600; }
  .subtab small { margin-left: 6px; padding: 0 6px; border: 1px solid var(--line); border-radius: 999px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }

  .block { margin-top: 24px; padding: clamp(16px, 3vw, 24px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .blockhead { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .muted { margin: 6px 0 0; color: var(--muted); font-size: 0.88rem; }
  .empty { margin: 16px 0 0; color: var(--muted); }
  .findings { list-style: none; margin: 12px 0 0; padding: 0; display: grid; gap: 8px; }
  .findings li { display: flex; gap: 8px; align-items: flex-start; font-size: 0.875rem; }
  .sev { flex: none; padding: 1px 7px; border: 1px solid var(--line); border-radius: 6px; font-size: 0.875rem; }
  .findings li.error .sev { border-color: var(--bad); color: var(--bad); }
  .findings li.warning .sev { border-color: var(--heart); color: var(--heart); }
  .settings { display: grid; gap: 24px; margin-top: 24px; max-width: 960px; }
  .settings fieldset { margin: 0; padding: 0; border: 0; min-width: 0; }
  .settings legend { padding: 0; margin-bottom: 14px; font-weight: 600; font-size: 0.95rem; }
  .settings-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
  .settings label { display: grid; gap: 6px; align-content: start; font-size: 0.875rem; color: var(--muted); min-width: 0; }
  .settings input, .settings select { width: 100%; min-width: 0; }
  .settings small { display: block; color: var(--muted); line-height: 1.5; }
  .settings label.check { display: flex; align-items: center; gap: 10px; }
  .settings .check input { width: 18px; height: 18px; flex: none; }
  .settings-footer { border-top: 1px solid var(--line); padding-top: 20px; }
  .bad { color: var(--bad) !important; overflow-wrap: anywhere; }
  .remotes { list-style: none; margin: 16px 0 0; padding: 0; display: grid; gap: 12px; }
  .remotes > li { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--line); border-radius: 8px; }
  .remotes > li.failing { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .rwho { display: grid; gap: 2px; }
  .rwho small { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
  .racts { display: flex; flex-wrap: wrap; gap: 4px; }
  .tag { display: inline-flex; margin-left: 6px; padding: 1px 7px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; }
  .tag.bad { border-color: var(--bad); color: var(--bad); }
  .check, .copies { margin-top: 4px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); font-size: 0.875rem; }
  .check p { margin: 0; }
  .check ul, .copies ul { list-style: none; margin: 6px 0 0; padding: 0; display: grid; gap: 4px; }
  .copies li { display: flex; flex-wrap: wrap; gap: 8px; justify-content: space-between; }
  .copies code { font-size: 0.875rem; overflow-wrap: anywhere; }
  .copies span { color: var(--muted); font-variant-numeric: tabular-nums; }
  .good { color: var(--good); }
  .remoteform { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
  .remoteform h4 { margin: 0 0 10px; font: 500 1.15rem/1.2 var(--serif); }
  .grid2 { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 12px; }
  .grid2 label { display: grid; gap: 4px; font-size: 0.875rem; color: var(--muted); }
  .grid2 label.check { display: flex; align-items: center; gap: 8px; align-self: end; }
  .grid2 label.check input { width: 18px; height: 18px; min-height: 0; }
  .formacts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: 12px; }
  .copy-toolbar, .run-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 240px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .search input { width: 100%; }
  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .copy-list, .run-list { list-style: none; margin: 0; padding: 0; display: grid; }
  .copy-list > li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: start; gap: 12px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .copy-list > li:first-child, .run-list > li:first-child { border-top: 0; }
  .copy-main, .run-main { display: grid; gap: 3px; min-width: 0; }
  .copy-id { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
  .copy-id code { font-size: 0.875rem; overflow-wrap: anywhere; }
  .copy-main small, .run-main small { color: var(--muted); font-size: 0.875rem; line-height: 1.4; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
  .copy-where { display: flex; flex-wrap: wrap; gap: 4px; max-width: 16rem; }
  .copy-where .tag { margin-left: 0; }
  .copy-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px; }
  .copy-actions .btn { padding: 4px 8px; min-height: 32px; }
  .run-list > li { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: start; gap: 12px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .run-main strong { overflow-wrap: anywhere; }
  .run-meta { display: grid; justify-items: end; gap: 3px; color: var(--muted); font-size: 0.875rem; text-align: right; font-variant-numeric: tabular-nums; }
  .run-meta small { overflow-wrap: anywhere; }
  .state { display: inline-block; padding: 2px 8px; border: 1px solid var(--line); border-radius: 6px; font-size: 0.875rem; text-transform: capitalize; }
  .state.succeeded { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); color: var(--good); }
  .state.failed { border-color: color-mix(in srgb, var(--bad) 42%, var(--line)); color: var(--bad); }
  .state.running { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); color: var(--accent-text); }
  .state-block { padding: 24px 0 16px; max-width: 58ch; }
  .state-block h3 { margin: 0; font: 500 1.25rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .state-block p { margin: 6px 0 12px; color: var(--muted); }
  .banner.fenced { border-color: color-mix(in srgb, var(--warn) 60%, var(--line)); }
  .cmdline { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .cmdline code { padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); overflow-wrap: anywhere; }
  @media (max-width: 900px) { .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 560px) {
    .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
    .cards article { padding: 16px; min-width: 0; }
    .cards article:last-child { grid-column: 1 / -1; }
    .health { padding: 16px; }
    .health .btn { display: none; }
    .settings-grid { grid-template-columns: 1fr; }
    .settings input { width: 100%; }
    .settings-footer .btn { width: 100%; }
    .actions, .actions .btn { width: 100%; }
    .copy-toolbar, .run-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .copy-list > li { grid-template-columns: 1fr; }
    .copy-where, .copy-actions { grid-column: 1; grid-row: auto; }
    .copy-actions { justify-content: flex-start; }
    .run-meta { justify-items: start; text-align: left; }
  }
  @media (pointer: coarse) { .scope, .subtab, .search input, .copy-actions .btn { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .scope { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
