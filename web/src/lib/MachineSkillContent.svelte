<script lang="ts">
 import { onMount } from 'svelte'
 import { ApiError, UnauthorizedError, dismissContentPreview, getContentPins, getContentPreview, getContentPreviews, getPrivateBundles, getPrivateRevisions, removeContentPin, requestContentPreview, saveContentPin, stopContentPreview, type ContentPin, type ContentPreview, type ContentPreviewCapability, type PrivateBundle, type PrivateRevision, type SyncState } from './api'
 import { asSeenFromPin, checkStateLabel, connectorFor, describeError, missingNote, modeLabel, pinChangeNote, pollDelay, revisionLabel, scopeEligibility, type PinChoice } from './contentReview'
 import { groupLabel } from './machines'
 import ConfirmDialog from './ConfirmDialog.svelte'
 import ContentCheckStep from './ContentCheckStep.svelte'
 import ContentPinStep from './ContentPinStep.svelte'

 // Opened from one agent scope on the Machines page, or from one reported skill in it (source.name). Everything here
 // reads, or records intent in reet; nothing is written to the machine. The Pin step records intent; the Check step asks
 // the machine what it holds (names, sizes and digests only). opener is the
 // element the person pressed to open this, which gets focus back on close (not every browser focuses a button on click).
 let { snapshot, source, canWrite, accounts, opener = null, onclose }: { snapshot: SyncState; source: { environmentId: string; name?: string }; canWrite: boolean; accounts: boolean; opener?: HTMLElement | null; onclose: () => void } = $props()
 type Step = 'pin' | 'check'
 const titleId = $props.id()
 let dialog = $state<HTMLDialogElement>()
 let step = $state<Step>('pin')
 let pins = $state<ContentPin[]>([]), bundles = $state<PrivateBundle[]>([])
 // Revisions read so far, by library skill, so a pin is described by when its revision was made and not by an id.
 let revisionLists = $state<Record<string, PrivateRevision[] | undefined>>({})
 let loading = $state(true), loaded = $state(false), loadFailed = $state(false), busy = $state(false)
 let error = $state(''), notice = $state('')
 let confirming = $state<{ pin: ContentPin; done: () => void } | null>(null)
 // Checks. Rows from the list are summaries (no capture or plan); one is read in full before it is shown. readAts is the
 // browser time each response arrived, which is what freshness is judged from, never the server's timestamps.
 let selectedPinId = $state('')
 let previews = $state<ContentPreview[]>([]), capabilities = $state<ContentPreviewCapability[]>([])
 let readAts = $state<Record<string, number>>({})
 let resumeCandidate = $state<{ row: ContentPreview; at: number } | null>(null)
 let watchFailed = $state(false), checksDown = $state(false)
 let alive = true, generation = 0
 let loadAbort: AbortController | undefined
 // Aborted with the dialog, so a slow revision list cannot outlive it.
 const life = new AbortController()
 const environment = $derived(snapshot.environments.find(e => e.id === source.environmentId))
 const connector = $derived(environment ? connectorFor(environment, snapshot.agents) : undefined)
 const eligibility = $derived(environment ? scopeEligibility(environment, snapshot.agents, { accounts, canWrite, now: Date.now() }) : { ok: false as const, reason: 'The machine no longer reports this folder.' })

 // The latest read wins: starting one cancels the one before it, and closing the dialog cancels them all.
 async function load(): Promise<boolean> {
  loadAbort?.abort()
  const abort = new AbortController(); loadAbort = abort
  const token = ++generation
  // A first read that is being tried again shows that it is working; the old failure stays until something succeeds.
  if (!loaded) loading = true
  try {
   const [saved, library] = await Promise.all([getContentPins(abort.signal), getPrivateBundles(abort.signal)])
   const here = saved.targets.filter(p => p.environmentId === source.environmentId)
   // No check can exist before a pin does, so the checks are read only once this scope has one.
   // Checks are a separate read: when it fails the pins and the library are still usable, and only checking is off.
   let listed: { previews: ContentPreview[]; capabilities: ContentPreviewCapability[] } | undefined = { previews: [], capabilities: [] }
   if (here.length > 0) {
    try { listed = await getContentPreviews(abort.signal) } catch (e) { if (abort.signal.aborted) throw e; listed = undefined }
   }
   const arrived = Date.now()
   // The pin rows say when each revision was made, so read those lists before showing them. A list that cannot be read
   // only means that row falls back to the start of its id.
   const ids = [...new Set(here.map(p => p.bundleId))]
   const lists = await Promise.allSettled(ids.map(id => getPrivateRevisions(id, abort.signal)))
   if (!alive || token !== generation) return false
   const next = { ...revisionLists }
   ids.forEach((id, i) => { const read = lists[i]; if (read.status === 'fulfilled') next[id] = read.value.revisions })
   revisionLists = next; pins = here; bundles = library.bundles
   checksDown = !listed
   if (listed) applyList(listed, arrived)
   loaded = true; loadFailed = false; error = ''
   // The pin being checked may have gone; the check shown may have been replaced by a newer one.
   if (step === 'check' && !here.some(p => p.id === selectedPinId)) leaveCheck()
   else syncWatch()
   return true
  } catch (e) {
   if (alive && token === generation) { error = describeError(e); loadFailed = true }
   return false
  } finally {
   if (alive && token === generation) loading = false
  }
 }
 // The list is a summary: a row that has not changed keeps the full read already held for it, otherwise the summary
 // replaces it and is read in full before it is shown. Every row's arrival time is recorded.
 function applyList(listed: { previews: ContentPreview[]; capabilities: ContentPreviewCapability[] }, arrived: number) {
  const held = new Map(previews.map(p => [p.id, p]))
  previews = listed.previews.map(row => {
   const old = held.get(row.id)
   return old?.capture && !row.capture && old.state === row.state && old.updatedAt === row.updatedAt && (old.reasons?.length ?? 0) === (row.reasons?.length ?? 0) ? old : row
  })
  capabilities = listed.capabilities
  const stamps = { ...readAts }
  for (const row of listed.previews) stamps[row.id] = arrived
  readAts = stamps
 }
 function accept(p: ContentPreview, at = Date.now()) {
  previews = previews.some(x => x.id === p.id) ? previews.map(x => (x.id === p.id ? p : x)) : [...previews, p]
  readAts = { ...readAts, [p.id]: at }
 }
 const latestFor = (pin: ContentPin): ContentPreview | undefined => previews.filter(p => p.targetId === pin.id).reduce<ContentPreview | undefined>((best, p) => (!best || p.createdAt > best.createdAt ? p : best), undefined)
 const selectedPin = $derived(pins.find(p => p.id === selectedPinId))
 // A check belongs to one version of the pin: once the pin has moved on it is overtaken, whatever the server last said.
 const check = $derived(selectedPin ? (asSeenFromPin(latestFor(selectedPin), selectedPin) ?? undefined) : undefined)
 // A summary row for a captured check carries no capture, so it is not usable until it has been read in full.
 const usable = $derived(!!check && !(check.state === 'captured' && !check.capture))
 const connectorReady = $derived(!!selectedPin && capabilities.find(c => c.agentId === selectedPin.agentId)?.ready === true)
 const checkReady = $derived(connectorReady && eligibility.ok && !checksDown)
 const CHECKS_DOWN = "Checks aren't available right now."
 const statusFor = (pin: ContentPin): string => {
  const row = latestFor(pin)
  if (!row) return 'Last check: Not checked yet'
  return `Last check: ${checkStateLabel(row.state === 'captured' && row.reasons?.length ? 'superseded' : row.state)}`
 }

 // Reading a check again and again while it can still change. One watcher at a time, fenced by the dialog's lifetime, the
 // selected pin and a generation, and cancelled with an AbortController; it stops on a final state and while the tab is
 // hidden, and the tab coming back reads at once.
 let watchId = '', watchGen = 0, watchWants = false, watchBusy = false
 let watchTimer: ReturnType<typeof setTimeout> | undefined, watchAbort: AbortController | undefined, watchRead: (() => Promise<void>) | undefined
 function stopWatching() {
  clearTimeout(watchTimer); watchTimer = undefined
  watchAbort?.abort(); watchAbort = undefined
  watchGen++; watchId = ''; watchWants = false; watchBusy = false; watchRead = undefined
 }
 function watch(id: string, immediate: boolean) {
  stopWatching()
  const gen = watchGen, pinId = selectedPinId
  const abort = new AbortController()
  watchAbort = abort; watchId = id
  const fenced = () => !alive || gen !== watchGen || pinId !== selectedPinId || step !== 'check'
  const schedule = (state: string) => {
   if (fenced()) return
   watchWants = pollDelay(state, false) !== null
   const delay = pollDelay(state, document.hidden)
   if (delay !== null) watchTimer = setTimeout(() => void read(), delay)
  }
  const read = async () => {
   watchTimer = undefined
   if (fenced() || watchBusy || document.hidden) return
   watchBusy = true
   let state = previews.find(p => p.id === id)?.state ?? ''
   try {
    const p = await getContentPreview(id, abort.signal)
    if (fenced()) return
    accept(p); state = asSeenFromPin(p, selectedPin)?.state ?? p.state
    if (watchFailed) { watchFailed = false; error = '' }
   } catch (e) {
    if (fenced() || abort.signal.aborted) return
    if (e instanceof ApiError && e.status === 404) { watchBusy = false; await load(); return }
    error = describeError(e); watchFailed = true
   } finally {
    if (gen === watchGen) watchBusy = false
   }
   schedule(state)
  }
  watchRead = read
  watchWants = true
  if (immediate) void read()
  else schedule(previews.find(p => p.id === id)?.state ?? '')
 }
 // Keep the watcher matched to what is on screen: nothing for a final check, a read at once for one not yet shown in full.
 function syncWatch() {
  const row = check
  if (!alive || step !== 'check' || !selectedPin || !row) { stopWatching(); return }
  if (!usable) { watch(row.id, true); return }
  if (pollDelay(row.state, false) === null) { stopWatching(); return }
  if (watchId !== row.id) watch(row.id, true)
 }
 // While the connector has not recently reported in, the list is read now and then so the button comes back by itself.
 let capTimer: ReturnType<typeof setTimeout> | undefined
 function scheduleCaps() {
  clearTimeout(capTimer); capTimer = undefined
  if (!alive || step !== 'check' || connectorReady || document.hidden) return
  capTimer = setTimeout(async () => {
   capTimer = undefined
   if (!alive || step !== 'check') return
   try {
    const listed = await getContentPreviews(life.signal)
    if (alive) capabilities = listed.capabilities
   } catch { /* the next try will do */ }
   scheduleCaps()
  }, 10000)
 }
 function openCheck(pin: ContentPin) {
  if (busy) return
  selectedPinId = pin.id; step = 'check'
  error = ''; notice = ''; resumeCandidate = null; watchFailed = false
  syncWatch(); scheduleCaps()
 }
 function leaveCheck() {
  stopWatching(); clearTimeout(capTimer); capTimer = undefined
  step = 'pin'; selectedPinId = ''; resumeCandidate = null; watchFailed = false
 }
 function retryWatch() {
  error = ''; watchFailed = false
  if (check) watch(check.id, true)
 }

 // Request keys stay in memory: one per pin version, kept after an uncertain create failure so a retry is the same
 // request, and dropped once the server has answered, so Check again is a new one.
 const requestKeys = new Map<string, string>()
 function newKey(): string {
  const c = globalThis.crypto
  if (typeof c?.randomUUID === 'function') return `chk-${c.randomUUID()}`
  const bytes = c.getRandomValues(new Uint8Array(16))
  return `chk-${[...bytes].map(b => b.toString(16).padStart(2, '0')).join('')}`
 }
 const slotOf = (pin: ContentPin) => `${pin.id}:${pin.version}`
 const pinWords = (p: ContentPin) => `revision ${revisionText(p)} (${modeLabel(p.mode)})`

 async function startCheck(): Promise<void> {
  const pin = selectedPin
  if (busy || !pin || !checkReady) return
  busy = true; error = ''; notice = ''; resumeCandidate = null
  const pinId = pin.id
  let slot = ''
  try {
   // The pin is read again first, so a check is never made for a pin that is not the one on screen.
   const fresh = (await getContentPins(life.signal)).targets.find(p => p.id === pinId)
   if (!alive || selectedPinId !== pinId) return
   if (!fresh || fresh.version !== pin.version) {
    await load()
    if (!alive) return
    const now = pins.find(p => p.id === pinId)
    if (!now) leaveCheck()
    error = !now ? `The pin for ${pin.name} was removed elsewhere, so no check was made.` : `The pin for ${pin.name} changed elsewhere, so no check was made. It is now ${pinWords(now)}. Check again to check it as it is now.`
    return
   }
   slot = slotOf(fresh)
   let key = requestKeys.get(slot)
   if (!key) { key = newKey(); requestKeys.set(slot, key) }
   const made = await requestContentPreview({ requestKey: key, targetId: fresh.id, expectedTargetVersion: fresh.version })
   requestKeys.delete(slot)
   if (!alive || selectedPinId !== pinId) return
   accept(made)
   watch(made.id, false)
  } catch (e) {
   if (alive && selectedPinId === pinId) await createFailed(e, pin, slot)
  } finally {
   if (alive) busy = false
  }
 }
 async function createFailed(e: unknown, pin: ContentPin, slot: string) {
  error = describeError(e)
  if (e instanceof ApiError) {
   // The server answered, so that key is spent. Every refusal to make a check is a 409, whether the machine is busy, the
   // pin moved or the connector has not reported in; only an active check on this connector makes it a busy answer.
   requestKeys.delete(slot)
   if (e.status === 409) await whenRefused(pin)
   return
  }
  if (e instanceof UnauthorizedError || slot === '') return
  // No reply arrived, so the check may exist: look for it by its key. If it cannot be found or the list cannot be read,
  // the key is kept and the next press sends the same request.
  const key = requestKeys.get(slot)
  try {
   const listed = await getContentPreviews(life.signal)
   const arrived = Date.now()
   if (!alive || selectedPinId !== pin.id) return
   const made = key ? listed.previews.find(p => p.requestKey === key) : undefined
   applyList(listed, arrived)
   if (made) { requestKeys.delete(slot); error = ''; syncWatch() }
  } catch { /* keep the key and the message */ }
 }
 async function whenRefused(pin: ContentPin) {
  try {
   const listed = await getContentPreviews(life.signal)
   const arrived = Date.now()
   if (!alive || selectedPinId !== pin.id) return
   capabilities = listed.capabilities
   const active = listed.previews.find(p => (p.state === 'queued' || p.state === 'capturing') && p.agentId === pin.agentId)
   if (active && pins.some(p => p.id === active.targetId)) { resumeCandidate = { row: active, at: arrived }; error = `This machine is already checking ${active.name}.`; return }
   if (active) error = `${error.replace(/[.!?]?$/, '.')} A check for ${active.name} in another folder is running on this machine.`
  } catch { /* the message already says what the server said */ }
  // Read everything again in case the pin moved, but keep what the server said.
  const said = error
  await load()
  if (alive && !error) error = said
 }
 function resumeCheck() {
  const found = resumeCandidate
  if (busy || !found) return
  resumeCandidate = null; error = ''
  selectedPinId = found.row.targetId
  accept(found.row, found.at)
  watch(found.row.id, true)
 }
 async function stopCheck(): Promise<void> {
  const current = check
  if (busy || !current) return
  busy = true; error = ''
  try {
   const stopped = await stopContentPreview(current.id)
   if (!alive) return
   accept(stopped); syncWatch()
  } catch (e) {
   if (alive) { error = describeError(e); await load() }
  } finally {
   if (alive) busy = false
  }
 }
 async function dismissCheck(): Promise<void> {
  const current = check
  if (busy || !current) return
  busy = true; error = ''
  try {
   await dismissContentPreview(current.id)
   if (!alive) return
   previews = previews.filter(p => p.id !== current.id)
   syncWatch()
  } catch (e) {
   if (alive) { error = describeError(e); await load() }
  } finally {
   if (alive) busy = false
  }
 }

 const revisionsFor = async (bundleId: string): Promise<PrivateRevision[]> => {
  const list = (await getPrivateRevisions(bundleId, life.signal)).revisions
  revisionLists = { ...revisionLists, [bundleId]: list }
  return list
 }
 const revisionText = (p: ContentPin) => revisionLabel(p.revisionId, revisionLists[p.bundleId], bundles.find(b => b.id === p.bundleId)?.head)

 // A refusal because the pin is not as the person last saw it: read it again and say what changed. When no reply came
 // at all the write may still have landed, so read again quietly too and keep the plain message.
 async function refused(e: unknown, name: string, before: ContentPin | undefined, action: 'save' | 'remove') {
  if (e instanceof ApiError && (e.status === 409 || e.status === 404)) {
   const fresh = await load()
   if (!alive) return
   const changed = fresh ? pinChangeNote(before, pins.find(p => p.name === name), name, action, revisionText) : ''
   error = changed || (e.status === 404 ? missingNote(action) : describeError(e))
   return
  }
  if (!(e instanceof ApiError || e instanceof UnauthorizedError)) {
   await load()
   if (!alive) return
  }
  error = describeError(e)
 }
 async function save(choice: PinChoice): Promise<void> {
  if (busy) return
  busy = true; notice = ''; error = ''
  const name = bundles.find(b => b.id === choice.bundleId)?.name ?? ''
  const before = pins.find(p => p.name === name)
  try {
   if (!environment || !connector || !eligibility.ok) { error = eligibility.ok ? 'Choose a running connector on this machine first.' : eligibility.reason; return }
   const saved = await saveContentPin({ environmentId: environment.id, agentId: connector.id, ...choice })
   if (!alive) return
   await load()
   if (!alive) return
   notice = `Sorted. ${saved.name} is pinned at revision ${revisionText(saved)}. Nothing was written to the machine.`
  } catch (e) {
   if (alive) await refused(e, name, before, 'save')
  } finally {
   if (alive) busy = false
  }
 }
 async function remove(pin: ContentPin): Promise<void> {
  if (busy) return
  busy = true; notice = ''; error = ''
  try {
   await removeContentPin(pin.id, pin.version)
   if (!alive) return
   notice = `Pin removed for ${pin.name}. That removed the intent only: nothing on the machine changed, and any ownership record for it stays as it is.`
   await load()
  } catch (e) {
   if (alive) await refused(e, pin.name, pin, 'remove')
  } finally {
   if (alive) busy = false
  }
 }
 // The step waits on this: it settles when the person cancels, or once the removal has finished.
 function askRemove(pin: ContentPin): Promise<void> {
  if (busy) return Promise.resolve()
  return new Promise(resolve => { confirming = { pin, done: resolve } })
 }
 // Read the question before clearing it: the dialog is gone once it is cleared.
 function confirmRemove() {
  const asked = confirming
  confirming = null
  if (asked) void remove(asked.pin).finally(asked.done)
 }
 function cancelRemove() {
  const asked = confirming
  confirming = null
  asked?.done()
 }
 function close() {
  stopWatching(); clearTimeout(capTimer)
  dialog?.close()
  if (opener?.isConnected) opener.focus()
  onclose()
 }
 onMount(() => {
  dialog?.showModal(); void load()
  // Coming back to the tab reads a check that can still change at once, and the connector's readiness again.
  const visible = () => {
   if (document.hidden) return
   scheduleCaps()
   if (watchWants && !watchBusy && watchRead) { clearTimeout(watchTimer); watchTimer = undefined; void watchRead() }
  }
  document.addEventListener('visibilitychange', visible)
  return () => { alive = false; generation++; loadAbort?.abort(); life.abort(); stopWatching(); clearTimeout(capTimer); document.removeEventListener('visibilitychange', visible) }
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={titleId}>
 <header>
  <div>
   <p class="eyebrow">{environment ? `${groupLabel(environment)}${environment.project ? '' : ' · Home'}` : 'Your private toolkit'}</p>
   <h2 id={titleId}>Private skills on {environment?.host ?? 'this machine'}</h2>
  </div>
  <button type="button" class="btn" onclick={close} aria-label="Close private skills">Close ×</button>
 </header>
 <div class="body">
  <p class="intro">{step === 'check' ? 'Ask this machine what it holds for the pinned skill. Nothing on it is changed.' : 'Choose which private library revision this folder should have.'}</p>
  <div class="live" role="status" aria-live="polite">{notice}</div>
  {#if error}<div class="alert" role="alert"><p>{error}</p>{#if loadFailed}<button type="button" class="btn" onclick={() => void load()}>Try again</button>{:else if watchFailed}<button type="button" class="btn" onclick={retryWatch}>Try again</button>{/if}</div>{/if}
  {#if loading}<p class="intro" role="status">Loading pins and your library…</p>{/if}
  {#if checksDown && loaded}<div class="alert" role="status"><p>{CHECKS_DOWN}</p><button type="button" class="btn" onclick={() => void load()}>Try again</button></div>{/if}
  {#if loaded}
   {#if step === 'check' && selectedPin}
    <p class="back"><button type="button" class="btn" disabled={busy} onclick={leaveCheck}>Back to pins</button></p>
    <ContentCheckStep
     pin={selectedPin}
     check={usable && check ? check : null}
     readAt={check ? (readAts[check.id] ?? Number.NaN) : Number.NaN}
     ready={checkReady}
     why={eligibility.ok ? (checksDown ? CHECKS_DOWN : '') : eligibility.reason}
     {busy}
     loading={!!check && !usable}
     resumeName={resumeCandidate?.row.name ?? ''}
     revision={revisionText(selectedPin)}
     madeFor={check && check.revisionId !== selectedPin.revisionId ? revisionLabel(check.revisionId, revisionLists[selectedPin.bundleId], bundles.find(b => b.id === selectedPin.bundleId)?.head) : ''}
     oncheck={() => void startCheck()}
     onresume={resumeCheck}
     oncancel={() => void stopCheck()}
     ondismiss={() => void dismissCheck()}
    />
   {:else}
    <ContentPinStep {pins} {bundles} preset={source.name ?? ''} agentId={connector?.id ?? ''} saveBlocked={eligibility.ok ? '' : eligibility.reason} {busy} {revisionLists} {revisionsFor} {statusFor} oncheck={openCheck} onsave={save} onremove={askRemove} />
   {/if}
  {/if}
 </div>
 <footer>
  <small>Nothing on this screen changes files on the machine.</small>
  <button type="button" class="btn primary" onclick={close}>Done</button>
 </footer>
</dialog>

{#if confirming}
 <ConfirmDialog
  title={`Remove the pin for ${confirming.pin.name}?`}
  body="Removing the pin removes intent only. It changes nothing on the machine, and any ownership record for it stays as it is."
  confirmLabel="Remove pin"
  onconfirm={confirmRemove}
  oncancel={cancelRemove}
 />
{/if}

<style>
 dialog { width: min(880px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr) auto; height: min(900px, calc(100dvh - 28px)); }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); background: var(--card); border-radius: 16px 16px 0 0; }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; overflow-wrap: anywhere; }
 header .btn { flex: none; white-space: nowrap; }
 .body { padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; }
 .intro, small { color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .intro { margin: 0 0 14px; }
 .live { min-height: 0; margin: 0; font-size: .85rem; color: var(--good); }
 .live:not(:empty) { margin: 0 0 14px; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--good) 35%, var(--line)); border-radius: 8px; }
 .alert { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; margin: 0 0 14px; padding: 10px 12px; color: var(--bad); border: 1px solid color-mix(in srgb, var(--bad) 40%, var(--line)); border-radius: 8px; font-size: .85rem; }
 .alert p { margin: 0; overflow-wrap: anywhere; }
 .back { margin: 0 0 14px; }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px; border-top: 1px solid var(--line); background: var(--card); border-radius: 0 0 16px 16px; }
 footer small { margin: 0; }
 @media (max-width: 500px) { header, .body { padding: 16px; } footer { padding: 12px 16px; } h2 { font-size: 1.35rem; } }
</style>
