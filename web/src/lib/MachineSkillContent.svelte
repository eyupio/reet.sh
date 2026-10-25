<script lang="ts">
 import { onMount, tick, untrack } from 'svelte'
 import { ApiError, UnauthorizedError, adoptContent, getContentPins, getContentPreview, getContentPreviews, getContentReceipts, releaseContentReceipt, getPrivateBundles, getPrivateRevisions, removeContentPin, saveContentPin, type ContentPin, type ContentPreview, type ContentPreviewCapability, type ContentReceipt, type PrivateBundle, type PrivateRevision, type SyncState } from './api'
 import { CheckWatcher } from './checkWatcher.svelte'
 import { FALLBACK, OWNERSHIP_SENTENCE, SIGNED_OUT, isSignedOut, adoptRefusal, asSeenFromPin, candidatesFor, checkStateLabel, classifyCheck, connectorFor, describeFlowError, madeByAdoption, missingNote, modeLabel, pinChangeNote, receiptRevision, releaseNote, revisionLabel, revisionLabels, scopeEligibility, type PinChoice } from './contentReview'
 import { groupLabel } from './machines'
 import type { JoinContext } from './machineJoin'
 import ConfirmDialog from './ConfirmDialog.svelte'
 import ContentAdoptStep from './ContentAdoptStep.svelte'
 import ContentCheckStep from './ContentCheckStep.svelte'
 import ContentPinStep from './ContentPinStep.svelte'
 import ContentReceipts from './ContentReceipts.svelte'

 // Opened from one agent scope on the Machines page, or from one reported skill in it (source.name). Everything here
 // reads, or records intent in reet; nothing is written to the machine. The Pin step records intent; the Check step asks
 // the machine what it holds (names, sizes and digests only). opener is the
 // element the person pressed to open this, which gets focus back on close (not every browser focuses a button on click).
 // note is a line from whatever opened this on a folder it chose for the person (the Add a machine landing), shown once and
 // moved to with focus on the heading. onaddmachine is given only to a signed-in writer: it closes this and asks for the Add a
 // machine dialog, carrying the skill, the library skill chosen and the agent of this folder.
 let { snapshot, source, canWrite, accounts, opener = null, note = '', onaddmachine, onclose, onsignedout }: { snapshot: SyncState; source: { environmentId: string; name?: string }; canWrite: boolean; accounts: boolean; opener?: HTMLElement | null; note?: string; onaddmachine?: (context: JoinContext) => void; onclose: () => void; onsignedout?: (e: unknown) => void } = $props()
 type Step = 'pin' | 'check'
 const titleId = $props.id()
 let dialog = $state<HTMLDialogElement>()
 let step = $state<Step>('pin')
 let pins = $state<ContentPin[]>([]), bundles = $state<PrivateBundle[]>([])
 // Revisions read so far, by library skill, so a pin is described by when its revision was made and not by an id.
 let revisionLists = $state<Record<string, PrivateRevision[] | undefined>>({})
 let loading = $state(true), loaded = $state(false), loadFailed = $state(false), busy = $state(false)
 let error = $state(''), notice = $state('')
 // The library skill the Pin step has chosen now, kept for the Add a machine context.
 let chosenBundle = $state('')
 let confirming = $state<{ pin: ContentPin; done: () => void } | null>(null)
 // Checks: the controller holds them (rows from the list are summaries, one is read in full before it is shown).
 let selectedPinId = $state('')
 let checksDown = $state(false)
 // Ownership records. They are read apart from the pins and the checks, so a failure here only hides the records.
 let receipts = $state<ContentReceipt[]>([]), receiptsDown = $state(false)
 let releasing = $state<{ receipt: ContentReceipt; done: () => void } | null>(null)
 let recordsView: ContentReceipts | undefined = $state(), titleEl = $state<HTMLElement>()
 let alive = true, generation = 0
 let loadAbort: AbortController | undefined
 // Aborted with the dialog, so a slow revision list cannot outlive it.
 const life = new AbortController()
 const environment = $derived(snapshot.environments.find(e => e.id === source.environmentId))
 const connector = $derived(environment ? connectorFor(environment, snapshot.agents) : undefined)
 const eligibility = $derived(environment ? scopeEligibility(environment, snapshot.agents, { accounts, canWrite, now: Date.now() }) : { ok: false as const, reason: 'The machine no longer reports this folder.' })

 const pinWords = (p: ContentPin) => `revision ${revisionText(p)} (${modeLabel(p.mode)})`
 const watcher: CheckWatcher = new CheckWatcher({
  alive: () => alive,
  inCheck: () => step === 'check',
  selectedPinId: () => selectedPinId,
  selectedPin: () => selectedPin,
  hasPin: id => pins.some(p => p.id === id),
  select: id => { selectedPinId = id },
  get busy() { return busy }, set busy(v) { busy = v },
  get error() { return error }, set error(v) { error = v },
  get checksDown() { return checksDown }, set checksDown(v) { checksDown = v },
  checkReady: () => checkReady,
  pinWords,
  reload: () => load(),
  leave: () => leaveCheck(),
  signedOut: () => markSignedOut(),
  signal: life.signal,
 })

 // The latest read wins: starting one cancels the one before it, and closing the dialog cancels them all.
 async function load(): Promise<boolean> {
  loadAbort?.abort()
  const abort = new AbortController(); loadAbort = abort
  const token = ++generation
  signedOut = false
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
   // Ownership records are independent of pins and checks: their own read, whose failure hides only the records.
   let records: ContentReceipt[] | undefined
   try { records = (await getContentReceipts(abort.signal)).receipts } catch (e) { if (abort.signal.aborted) throw e; records = undefined }
   const arrived = Date.now()
   // The pin rows say when each revision was made, so read those lists before showing them. A list that cannot be read
   // only means that row falls back to the start of its id.
   const ids = [...new Set([...here.map(p => p.bundleId), ...(records ?? []).filter(r => r.environmentId === source.environmentId && !r.damaged && r.bundleId !== '').map(r => r.bundleId)])]
   const lists = await Promise.allSettled(ids.map(id => getPrivateRevisions(id, abort.signal)))
   if (!alive || token !== generation) return false
   const next = { ...revisionLists }
   ids.forEach((id, i) => { const read = lists[i]; if (read.status === 'fulfilled') next[id] = read.value.revisions })
   revisionLists = next; pins = here; bundles = library.bundles
   checksDown = !listed
   receiptsDown = !records; receipts = records ?? []
   if (listed) watcher.applyList(listed, arrived)
   loaded = true; loadFailed = false; error = ''
   // The pin being checked may have gone; the check shown may have been replaced by a newer one.
   if (step === 'check' && !here.some(p => p.id === selectedPinId)) leaveCheck()
   else watcher.syncWatch()
   return true
  } catch (e) {
   if (alive && token === generation) { if (isSignedOut(e)) markSignedOut(); else { error = describeFlowError(e); loadFailed = true } }
   return false
  } finally {
   if (alive && token === generation) loading = false
  }
 }
 const selectedPin = $derived(pins.find(p => p.id === selectedPinId))
 const checkReady: boolean = $derived(watcher.connectorReady && eligibility.ok && !checksDown)
 const CHECKS_DOWN = "Checks aren't available right now."
 const statusFor = (pin: ContentPin): string => {
  // The same overlay the Check step uses, so a check made for an older pin never reads as current here.
  const row = asSeenFromPin(watcher.latestFor(pin), pin) ?? undefined
  if (!row) return checksDown ? 'Last check: Unavailable' : 'Last check: Not checked yet'
  return `Last check: ${checkStateLabel(row.state === 'captured' && row.reasons?.length ? 'superseded' : row.state)}`
 }


 function openCheck(pin: ContentPin) {
  if (busy) return
  selectedPinId = pin.id; step = 'check'
  error = ''; notice = ''
  watcher.opened()
 }
 function leaveCheck() {
  watcher.left()
  step = 'pin'; selectedPinId = ''
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
   error = changed || (e.status === 404 ? missingNote(action) : describeFlowError(e))
   return
  }
  if (!(e instanceof ApiError || e instanceof UnauthorizedError)) {
   await load()
   if (!alive) return
  }
  fail(e)
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

 // Ownership. The scope's records, and the words for a record's revision (a date, never an id).
 const scopeReceipts = $derived(receipts.filter(r => r.environmentId === source.environmentId))
 const receiptRevisionText = (r: ContentReceipt) => receiptRevision(r.revisionId, revisionLists[r.bundleId], bundles.find(b => b.id === r.bundleId)?.head)
 const candidateLabel = (revisionId: string): string => {
  const pin = selectedPin
  const list = pin ? revisionLists[pin.bundleId] : undefined
  return list && pin ? (revisionLabels(list, bundles.find(b => b.id === pin.bundleId)?.head).get(revisionId) ?? '') : ''
 }
 const filesText = (n: number) => `${n} ${n === 1 ? 'file' : 'files'}`
 // Request keys for adoption stay in memory: one per check, review hash and revision, kept after an uncertain failure so a
 // retry is the same request (which the server answers once), dropped once the server has answered, so a new deliberate
 // attempt is a new key.
 const adoptKeys = new Map<string, string>()
 function newAdoptKey(): string {
  const c = globalThis.crypto
  if (typeof c?.randomUUID === 'function') return `adopt-${c.randomUUID()}`
  const bytes = c.getRandomValues(new Uint8Array(16))
  return `adopt-${[...bytes].map(b => b.toString(16).padStart(2, '0')).join('')}`
 }
 const BUSY_NOW = 'Another action is still under way. Try again in a moment.'
 const UNSURE = "Couldn't confirm whether that was recorded. Check the list below, or try again."
 // The records are read again when something may have changed them and the last read failed or was skipped. A failed read is
 // tried again now and then (about six times, never while the tab is hidden) so a record that was made but not listed does not
 // stay hidden; after that the person is told plainly that it could not be confirmed.
 let receiptsTimer: ReturnType<typeof setTimeout> | undefined, refreshedFor = '', receiptTries = 0, receiptsWaiting = false, signedOut = false
 // The adoption whose reply was lost: when its record turns up the message about it is replaced.
 let pendingAdopt: { previewId: string; revisionId: string; slot: string; said: string } | null = null
 // A failure shows as the message, or hands over to the sign-in when the sign-in is what failed.
 function fail(e: unknown) { if (isSignedOut(e)) markSignedOut(); else error = describeFlowError(e) }
 // The sign-in is gone: stop retrying, say so, and hand it to the page, which clears the token and shows its sign-in card.
 function markSignedOut() {
  const first = !signedOut
  signedOut = true; clearTimeout(receiptsTimer); receiptsTimer = undefined; pendingAdopt = null; error = SIGNED_OUT
  if (first) onsignedout?.(new UnauthorizedError())
 }
 function wantReceipts(fresh = true) {
  if (fresh) receiptTries = 0
  clearTimeout(receiptsTimer); receiptsTimer = undefined
  if (!alive || signedOut) return
  receiptsTimer = setTimeout(async () => {
   receiptsTimer = undefined
   if (!alive || signedOut) return
   if (document.hidden) { receiptsWaiting = true; return }
   receiptTries++
   if (await refreshReceipts() || !alive || signedOut) return
   if (receiptTries >= 6) { if (pendingAdopt) error = UNSURE; return }
   wantReceipts(false)
  }, 10000)
 }
 async function syncReceipts() { if (!(await refreshReceipts()) && alive && !signedOut) wantReceipts() }
 // Something that can be lost while it is waited for, such as a busy message, goes when the thing it waited for has finished.
 $effect(() => { if (!busy) untrack(() => { if (error === BUSY_NOW) error = '' }) })
 // The server consumes a check when it records ownership. When the check being watched turns out to have been overtaken by
 // a changed record, the records are read again (once for that check).
 $effect(() => {
  const c = watcher.check
  untrack(() => {
   if (c && c.state === 'superseded' && c.reasons?.includes('receipt_changed') && refreshedFor !== c.id) { refreshedFor = c.id; void syncReceipts() }
  })
 })
 async function refreshReceipts(): Promise<boolean> {
  try {
   const read = await getContentReceipts(life.signal)
   if (!alive) return false
   receipts = read.receipts; receiptsDown = false
   const waiting = pendingAdopt
   if (waiting && read.receipts.some(r => madeByAdoption(r, waiting.previewId, waiting.revisionId))) {
    adoptKeys.delete(waiting.slot); pendingAdopt = null
    // Only the messages about that lost reply go; anything else on screen stays.
    if (error === waiting.said || error === UNSURE) error = ''
    notice = 'That ownership was recorded.'
   }
   return true
  } catch (e) {
   if (isSignedOut(e)) markSignedOut()
   return false
  }
 }
 // Record ownership. Nothing is sent unless the check, read once more right now, is still the one the person saw and is
 // still current and adoptable; the review hash goes exactly as the server gave it.
 async function recordOwnership(revisionId: string, seen: { previewId: string; reviewHash: string }): Promise<void> {
  const pin = selectedPin
  if (!pin) return
  signedOut = false
  // The scope can stop qualifying while the step is open (a connector paused elsewhere): say why, send nothing.
  if (!eligibility.ok) { error = eligibility.reason; return }
  if (busy) { error = BUSY_NOW; return }
  busy = true; error = ''; notice = ''; pendingAdopt = null
  let slot = ''
  let recorded: ContentReceipt | undefined
  try {
   const again = await getContentPreview(seen.previewId, life.signal)
   const at = Date.now()
   if (!alive) return
   watcher.accept(again, at)
   const current = asSeenFromPin(again, pin) ?? again
   const hash = again.reviewHash
   if (!classifyCheck(current, at, Date.now()).canAdopt || typeof hash !== 'string' || hash !== seen.reviewHash || !candidatesFor(again.adoption).some(c => c.revisionId === revisionId)) {
    error = FALLBACK
    // The check moved on under the person, and a record may be why: show the records as they are.
    await syncReceipts()
    return
   }
   slot = `${again.id}:${hash}:${revisionId}`
   let key = adoptKeys.get(slot)
   if (!key) { key = newAdoptKey(); adoptKeys.set(slot, key) }
   try {
    recorded = await adoptContent({ requestKey: key, previewId: again.id, reviewHash: hash, adoptAsRevisionId: revisionId })
    adoptKeys.delete(slot)
   } catch (e) {
    if (!alive) return
    recorded = await adoptFailed(e, again, revisionId, slot)
   }
   if (!alive || !recorded) return
   // The check that was used is consumed; read it so nothing on screen offers it again, then show the records as they are.
   await refreshReceipts()
   try { watcher.accept(await getContentPreview(again.id, life.signal)) } catch { /* the next check replaces it */ }
   if (!alive) return
   notice = `Sorted. Ownership of ${filesText(recorded.fileCount)} in ${recorded.name} is recorded. ${OWNERSHIP_SENTENCE} Nothing on the machine was changed.`
  } catch (e) {
   if (alive) fail(e)
  } finally {
   if (alive) busy = false
  }
  if (alive && recorded && selectedPinId === pin.id) await watcher.startCheck()
 }
 // A failed request. Returns the receipt when a lost reply turns out to have landed.
 async function adoptFailed(e: unknown, again: ContentPreview, revisionId: string, slot: string): Promise<ContentReceipt | undefined> {
  if (e instanceof UnauthorizedError) { markSignedOut(); return undefined }
  if (e instanceof ApiError) {
   // The server answered, so that key is spent. What to say comes from the status and from what the check and the records
   // say now, never from the server's shared refusal text.
   adoptKeys.delete(slot)
   if (e.status === 409 || e.status === 404) {
    await load()
    let reasons: string[] = []
    let settled: ContentPreview | undefined
    try {
     const latest = await getContentPreview(again.id, life.signal)
     const at = Date.now()
     watcher.accept(latest, at)
     settled = asSeenFromPin(latest, selectedPin) ?? latest
     reasons = classifyCheck(settled, at, Date.now()).reasons
    } catch { /* the plain sentence will do */ }
    if (!alive) return undefined
    const adoptableNow = !!settled && classifyCheck(settled, Date.now(), Date.now()).canAdopt
    error = adoptRefusal(e.status, again.name, { owned: scopeReceipts.some(r => r.name === again.name), reasons, adoptable: adoptableNow, records: receipts.length })
   } else {
    error = adoptRefusal(e.status, again.name, { owned: false, reasons: [], adoptable: false, records: receipts.length }) || describeFlowError(e)
   }
   return undefined
  }
  // No reply arrived, so the record may exist: look for it among the records. If it is not there, or they cannot be read,
  // the key is kept and the next press sends the same request.
  error = describeFlowError(e)
  pendingAdopt = { previewId: again.id, revisionId, slot, said: error }
  try {
   const read = await getContentReceipts(life.signal)
   if (!alive) return undefined
   const landed = read.receipts.find(r => madeByAdoption(r, again.id, revisionId))
   receipts = read.receipts; receiptsDown = false
   if (landed) { adoptKeys.delete(slot); pendingAdopt = null; error = ''; return landed }
  } catch (err) {
   if (isSignedOut(err)) { markSignedOut(); return undefined }
   // Keep the key and the message, and keep trying to read the records: the one that may exist must not stay unlisted.
   wantReceipts()
  }
  return undefined
 }
 async function release(r: ContentReceipt): Promise<void> {
  if (busy) { error = BUSY_NOW; return }
  busy = true; notice = ''; error = ''
  try {
   await releaseContentReceipt(r.id, r.version)
   if (!alive) return
   notice = `Ownership released for ${r.name}. That removed the record only: nothing on the machine changed, and its files stay where they are.`
   await load()
  } catch (e) {
   if (!alive) return
   if (e instanceof ApiError && (e.status === 409 || e.status === 404)) {
    const fresh = await load()
    if (!alive) return
    error = (fresh ? releaseNote(r, scopeReceipts.find(x => x.id === r.id), r.name) : '') || (e.status === 404 ? `The ownership record for ${r.name} isn't there any more, so nothing was released.` : 'Nothing was released. Try again.')
   } else {
    // When no reply came the release may still have landed: read again quietly and keep the plain message.
    if (!(e instanceof ApiError || e instanceof UnauthorizedError)) await load()
    if (!alive) return
    fail(e)
   }
  } finally {
   if (alive) busy = false
  }
  if (alive) { await tick(); if (!recordsView?.focus()) titleEl?.focus() }
 }
 function askRelease(r: ContentReceipt): Promise<void> {
  if (busy) return Promise.resolve()
  return new Promise(resolve => { releasing = { receipt: r, done: resolve } })
 }
 function confirmRelease() {
  const asked = releasing
  releasing = null
  if (asked) void release(asked.receipt).finally(asked.done)
 }
 function cancelRelease() {
  const asked = releasing
  releasing = null
  asked?.done()
 }
 const addable = $derived(!!onaddmachine && accounts && canWrite)
 function addMachine() {
  if (!addable || busy) return
  const bundleId = (step === 'check' ? selectedPin?.bundleId : chosenBundle) || undefined
  const context: JoinContext = { skill: source.name, bundleId, agent: environment?.agent }
  onaddmachine?.(context)
 }
 function close() {
  watcher.dispose(); clearTimeout(receiptsTimer)
  dialog?.close()
  if (opener?.isConnected) opener.focus()
  onclose()
 }
 onMount(() => {
  dialog?.showModal(); void load()
  // Arrived from the Add a machine landing: say which folder was chosen, and start at the heading of this dialog.
  // The note goes into the status region only after the region has been on the page for a moment (a region that is filled as it
  // appears may not be announced); the heading takes focus at once.
  let noteTimer: ReturnType<typeof setTimeout> | undefined
  if (note) { titleEl?.focus(); void tick().then(() => { noteTimer = setTimeout(() => { if (alive) notice = note }, 50) }) }
  // Coming back to the tab reads a check that can still change at once, and the connector's readiness again.
  const visible = () => { watcher.visibilityChanged(); if (!document.hidden && receiptsWaiting) { receiptsWaiting = false; wantReceipts(false) } }
  document.addEventListener('visibilitychange', visible)
  return () => { alive = false; clearTimeout(noteTimer); generation++; loadAbort?.abort(); life.abort(); watcher.dispose(); clearTimeout(receiptsTimer); document.removeEventListener('visibilitychange', visible) }
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={titleId}>
 <header>
  <div>
   <p class="eyebrow">{environment ? `${groupLabel(environment)}${environment.project ? '' : ' · Home'}` : 'Your private toolkit'}</p>
   <h2 id={titleId} tabindex="-1" bind:this={titleEl}>Private skills on {environment?.host ?? 'this machine'}</h2>
  </div>
  <button type="button" class="btn" onclick={close} aria-label="Close private skills">Close ×</button>
 </header>
 <div class="body">
  <p class="intro">{step === 'check' ? 'Ask this machine what it holds for the pinned skill. Nothing on it is changed.' : 'Choose which private library revision this folder should have.'}</p>
  {#if addable && step === 'pin'}<p class="add-machine"><button type="button" class="linklike" disabled={busy} onclick={addMachine}>Not listed? Add a machine</button></p>{/if}
  <div class="live" role="status" aria-live="polite">{notice}</div>
  {#if error}<div class="alert" role="alert"><p>{error}</p>{#if loadFailed}<button type="button" class="btn" onclick={() => void load()}>Try again</button>{:else if watcher.watchFailed}<button type="button" class="btn" onclick={() => watcher.retryWatch()}>Try again</button>{/if}</div>{/if}
  {#if loading}<p class="intro" role="status">Loading pins and your library…</p>{/if}
  {#if checksDown && loaded}<div class="alert" role="status"><p>{CHECKS_DOWN}</p><button type="button" class="btn" onclick={() => void load()}>Try again</button></div>{/if}
  {#if loaded}
   {#if step === 'check' && selectedPin}
    <p class="back"><button type="button" class="btn" disabled={busy} onclick={leaveCheck}>Back to pins</button></p>
    <ContentCheckStep
     pin={selectedPin}
     check={watcher.usable && watcher.check ? watcher.check : null}
     readAt={watcher.check ? (watcher.readAts[watcher.check.id] ?? Number.NaN) : Number.NaN}
     ready={checkReady}
     why={eligibility.ok ? (checksDown ? CHECKS_DOWN : '') : eligibility.reason}
     {busy}
     loading={!!watcher.check && !watcher.usable}
     resumeName={watcher.resumeCandidate?.row.name ?? ''}
     revision={revisionText(selectedPin)}
     madeFor={watcher.check && watcher.check.revisionId !== selectedPin.revisionId ? revisionLabel(watcher.check.revisionId, revisionLists[selectedPin.bundleId], bundles.find(b => b.id === selectedPin.bundleId)?.head) : ''}
     oncheck={() => void watcher.startCheck()}
     onresume={() => watcher.resumeCheck()}
     oncancel={() => void watcher.stopCheck()}
     ondismiss={() => void watcher.dismissCheck()}
    >
     {#snippet adopt()}
      {#if watcher.check}<ContentAdoptStep check={watcher.check} {busy} labelFor={candidateLabel} blocked={eligibility.ok ? '' : eligibility.reason} onadopt={recordOwnership} onlapse={() => { if (alive) error = 'That check is out of date. Check again.' }} />{/if}
     {/snippet}
    </ContentCheckStep>
   {:else}
    <ContentPinStep bind:bundleId={chosenBundle} {pins} {bundles} preset={source.name ?? ''} agentId={connector?.id ?? ''} saveBlocked={eligibility.ok ? '' : eligibility.reason} {busy} {revisionLists} {revisionsFor} {statusFor} oncheck={openCheck} onsave={save} onremove={askRemove} />
   {/if}
   <ContentReceipts bind:this={recordsView} receipts={scopeReceipts} down={receiptsDown} {busy} revisionOf={receiptRevisionText} onrelease={askRelease} onretry={() => void load()} />
  {/if}
 </div>
 <footer>
  <small>Nothing on this screen changes files on the machine.</small>
  <button type="button" class="btn primary" onclick={close}>Done</button>
 </footer>
</dialog>

{#if releasing}
 <ConfirmDialog
  title={`Release ownership of ${releasing.receipt.name}?`}
  body="Releasing removes the ownership record only. Nothing on the machine is changed, and the files stay where they are."
  confirmLabel="Release ownership"
  onconfirm={confirmRelease}
  oncancel={cancelRelease}
 />
{/if}

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
 h2:focus { outline: none; }
 h2:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
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
 .add-machine { margin: -6px 0 14px; }
 .linklike { padding: 4px 0; min-height: 32px; font: inherit; font-size: .85rem; color: var(--accent); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 .linklike:disabled { color: var(--muted); cursor: default; }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px; border-top: 1px solid var(--line); background: var(--card); border-radius: 0 0 16px 16px; }
 footer small { margin: 0; }
 @media (max-width: 500px) { header, .body { padding: 16px; } footer { padding: 12px 16px; } h2 { font-size: 1.35rem; } }
</style>
