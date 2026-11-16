<script lang="ts">
  import { onMount } from 'svelte'
  import Skeleton from './Skeleton.svelte'
  import MoveAway from './MoveAway.svelte'
  import { getBackupStatus, getBuildInfo, getFence, getPendingImport, stageImport, cancelPendingImport, type BackupStatus, type RestoreReport, type StagedImport } from './api'
  import { addressLines, addressProblem, cleanAddress, fenceSentence, fenceWatch, isLiveSourceRefusal, moveFeatures, type Fence } from './controllerMove'
  import { bytes } from './terminal/health'

  let { role = 'owner', onerror, timeZone = '', dateFormat = 'locale' }: { role?: string; onerror?: (e: unknown) => void; timeZone?: string; dateFormat?: string } = $props()
  let status = $state<BackupStatus | null>(null)
  let loading = $state(true)
  let error = $state('')
  let saved = $state('')
  let busy = $state('')
  let pendingImport = $state<StagedImport | null>(null)
  let importFile = $state<File | null>(null)
  let importPassword = $state('')
  let importError = $state('')
  // Prefilled with where this page was opened: the address the controller demonstrably answers at.
  let importPublicUrl = $state(typeof window === 'undefined' ? '' : window.location.origin)
  let importAllowLive = $state(false)
  // Set only by the controller's own refusal, so the tick is asked for when it is needed and never offered as a
  // way round a check nobody has run yet.
  let importLiveRefused = $state(false)
  // What this controller can do in a move. Nothing is offered until it has said so itself.
  let features = $state(moveFeatures(undefined))
  let fence = $state<Fence>(fenceWatch.get())
  const isOwner = $derived(role === 'owner')
  const importAddressBad = $derived(addressProblem(importPublicUrl))
  let importDropSessions = $state(false)
  let importDropTokens = $state(false)
  let importOwnerEmail = $state('')
  let importReplace = $state(false)

  const copyItems = $derived(status?.items ?? [])
  onMount(() => {
    if (isOwner) void load()
    return fenceWatch.subscribe(next => (fence = next))
  })

  async function load(quiet = false) {
    if (!quiet) loading = true
    try {
      status = await getBackupStatus(true)
      pendingImport = await getPendingImport()
      features = moveFeatures((await getBuildInfo()).features)
      if (features.fence) fenceWatch.set(await getFence())
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  function reportLines(r: RestoreReport): string[] {
    const roles = Object.entries(r.usersByRole ?? {}).map(([role, n]) => `${n} ${role}`).join(', ')
    const l = [`${r.users} account(s)${roles ? ` (${roles})` : ''}, ${r.shelves} ${r.shelves === 1 ? 'library' : 'libraries'}, ${r.skills} skill(s), ${r.lists} list(s), ${r.tags} tag(s), ${r.environments} environment(s), ${r.machines} connected machine(s) and ${r.destinations} backup destination(s) come across.`,
      r.sessionsEnded ? `${r.sessionsEnded} browser sign-in(s) end.` : `${r.sessionsKept} browser sign-in(s) are kept.`]
    if (r.libraryItems || r.privateBundles) l.splice(1, 0, `${r.libraryItems ?? 0} library item(s) and ${r.privateBundles ?? 0} private content bundle(s) come across.`)
    l.push(r.tokensKept
      ? 'Personal API tokens and MCP connections are kept, so the old controller\'s tokens open this one.'
      : `${r.tokensRevoked} personal API token(s) and ${r.mcpConnectionsRevoked} MCP connection(s) are revoked.`)
    if (r.identityDropped) l.push('The private-connection identity is dropped; this controller makes its own.')
    if (r.safetyCopy) l.push(`The database that is live now was copied to ${r.safetyCopy} first.`)
    return l
  }

  function chooseImport(file: File | null) {
    importFile = file
    // The refusal was about the copy that was chosen; another copy is asked afresh.
    importLiveRefused = false
    importAllowLive = false
    importError = ''
  }

  async function stage() {
    if (!importFile || importAddressBad) return
    busy = 'import'
    importError = ''
    saved = ''
    try {
      pendingImport = await stageImport(importFile, {
        password: importPassword, dropSessions: importDropSessions, dropTokens: importDropTokens, ownerEmail: importOwnerEmail, replace: importReplace,
        publicUrl: features.publicUrl ? cleanAddress(importPublicUrl) : '', allowLiveSource: importLiveRefused && importAllowLive,
      })
      importFile = null
      importPassword = ''
      importLiveRefused = false
      importAllowLive = false
      saved = 'Staged. Restart reet to put it in place.'
    } catch (e) {
      // Said beside the form, with everything typed left as it was: the refusal is usually answered by changing one
      // field and trying again.
      importError = e instanceof Error ? e.message : String(e)
      if (isLiveSourceRefusal(e)) importLiveRefused = true
    } finally {
      busy = ''
    }
  }

  async function discardImport() {
    busy = 'import'
    try {
      await cancelPendingImport()
      pendingImport = null
      saved = 'The staged import was discarded.'
    } catch (e) {
      importError = e instanceof Error ? e.message : String(e)
    } finally {
      busy = ''
    }
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

<section aria-label="Move controller">
  {#if !isOwner}
    <p>Only the instance owner can move this controller.</p>
  {:else if loading}
    <Skeleton label="Loading controller…" shape="rows" count={3} />
  {:else if !status}
    <p class="notice bad" role="alert">{error || 'Could not load this controller.'}</p>
    <button type="button" class="btn" onclick={() => void load()}>Try again</button>
  {:else}
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {#if saved}<p class="notice good" role="status">{saved}</p>{/if}
    {#if fence.fenced}
      <div class="banner fenced" role="status"><strong>This controller is fenced.</strong> {fenceSentence(fence, when)} Signing in, reading, and taking, checking and downloading copies still work.</div>
    {/if}
      {#if features.fence}
        <MoveAway {fence} {features} items={copyItems} enabled={status.enabled} {when} size={bytes}
          onfence={(next) => fenceWatch.set(next)} ontaken={() => void load(true)} />
      {/if}
      {#if isOwner}
        <div class="banner import" role="group" aria-labelledby="import-title">
          <strong id="import-title">Move a controller in</strong>
          <p class="muted">Import a sealed copy from another controller (<code>.reetbackup</code> or the downloaded <code>.zip</code>). It is opened and checked now; it replaces this controller's database only when reet next starts, and the current one is kept beside it.</p>
          {#if importError}<p class="notice bad" role="alert">{importError}</p>{/if}
          {#if pendingImport}
            {@const report = pendingImport.report}
            <p><strong>{report.id}</strong> is staged. Restart reet to put it in place.</p>
            <ul>
              {#each reportLines(report) as line}<li>{line}</li>{/each}
              {#if features.publicUrl}{#each addressLines(report) as line}<li>{line}</li>{/each}{/if}
            </ul>
            {#if report.unreadable.length}
              <p>Sealed under a different backup key, so they cannot be opened here. They were cleared:</p>
              <ul>{#each report.unreadable as u}<li>{u}</li>{/each}</ul>
            {/if}
            {#if report.paused?.length}
              <p>Held back, so this controller does not repeat it by itself. Resume each when you are ready:</p>
              <ul>{#each report.paused as p}<li>{p}</li>{/each}</ul>
            {/if}
            {#if pendingImport.notInCopy.length}
              <details class="not-in-copy">
                <summary>Not in the copy: set these by hand on this machine</summary>
                <dl>
                  {#each pendingImport.notInCopy as item}
                    <dt>{item.names.join(', ')}</dt>
                    <dd>{item.say}</dd>
                  {/each}
                </dl>
              </details>
            {/if}
            <p class="muted">Once it has restarted, machines move to it on their own: the old controller's fence tells each one this address, and the old controller's Move page shows them arrive. Keep the old one fenced and running until they have.</p>
            <div class="formacts"><button type="button" class="btn" onclick={discardImport} disabled={busy === 'import'}>Discard the staged import</button></div>
          {:else}
            <label>
              <span>Copy</span>
              <input type="file" accept=".reetbackup,.sealed,.zip" onchange={(e) => chooseImport((e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
            </label>
            <label>
              <span>Copy password (if it has one)</span>
              <input type="password" autocomplete="off" bind:value={importPassword} />
            </label>
            {#if features.publicUrl}
              <label>
                <span>This controller's address (optional)</span>
                <input type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://reet.example.com" bind:value={importPublicUrl}
                  aria-invalid={importAddressBad ? 'true' : undefined} aria-describedby="import-address-hint" />
              </label>
              <small id="import-address-hint" class:bad={!!importAddressBad}>
                {importAddressBad || 'The addresses saved in the copy (links in mail, the sync library origin) are changed to this one, and MCP clients authorise again here. Clear it to leave them as they are.'}
              </small>
            {/if}
            <label class="keep">
              <input type="checkbox" bind:checked={importDropSessions} />
              End every browser sign-in in the copy
            </label>
            <label class="keep">
              <input type="checkbox" bind:checked={importDropTokens} />
              Revoke every personal API token and MCP connection in the copy
            </label>
            <label class="keep">
              <input type="checkbox" bind:checked={importReplace} />
              Replace what this controller already holds (a copy of the current database is kept first)
            </label>
            {#if importLiveRefused}
              <label class="keep needed">
                <input type="checkbox" bind:checked={importAllowLive} required />
                <span><strong>The old controller is stopped or gone</strong> (needed for this copy). It was not taken from a fenced controller. If the old one is still running, two controllers would work from one set of data; fence it and take a final copy instead.</span>
              </label>
            {/if}
            <label>
              <span>Make sure this owner can sign in (optional)</span>
              <input type="email" autocomplete="off" bind:value={importOwnerEmail} />
            </label>
            <div class="formacts">
              <button type="button" class="btn primary" onclick={stage} disabled={!importFile || busy === 'import' || !!importAddressBad || (importLiveRefused && !importAllowLive)} aria-busy={busy === 'import'}>{busy === 'import' ? 'Checking…' : 'Check and stage'}</button>
            </div>
          {/if}
        </div>
      {/if}
  {/if}
</section>
<style>

  .banner { margin-top: 20px; padding: clamp(16px, 3vw, 24px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); line-height: 1.6; }
  .muted { color: var(--muted); font-size: 0.9rem; }
  .notice { padding: 12px; border: 1px solid var(--line); border-radius: 8px; }
  .bad { color: var(--bad); overflow-wrap: anywhere; }
  .good { color: var(--good); }
  .formacts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: 16px; }
  .banner.import { border-color: var(--line); }
  .banner.fenced { border-color: color-mix(in srgb, var(--warn) 60%, var(--line)); }
  .banner.import > label:not(.keep) { display: grid; gap: 4px; margin-top: 12px; }
  .banner.import > label:not(.keep) > span { color: var(--muted); font-size: 0.875rem; }
  .banner.import input[type='url'], .banner.import input[type='password'], .banner.import input[type='email'] { width: 100%; max-width: 28rem; min-width: 0; }
  .banner.import small { display: block; max-width: 34rem; margin-top: 4px; color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .banner.import .notice { margin: 12px 0; overflow-wrap: anywhere; }
  .banner.import ul { overflow-wrap: anywhere; }
  .keep.needed { padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--warn) 60%, var(--line)); border-radius: 8px; }
  .not-in-copy { margin: 12px 0; }
  .not-in-copy summary { cursor: pointer; font-weight: 600; }
  .not-in-copy dl { margin: 8px 0 0; display: grid; gap: 4px; }
  .not-in-copy dt { font: 0.875rem/1.45 var(--mono); overflow-wrap: anywhere; }
  .not-in-copy dd { margin: 0 0 8px; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .banner.import input[type='file'] { width: 100%; min-width: 0; max-width: 100%; }
  .keep { display: flex; align-items: flex-start; gap: 8px; margin-top: 8px; }
  .keep input { width: 18px; height: 18px; min-height: 0; flex: none; }
</style>
