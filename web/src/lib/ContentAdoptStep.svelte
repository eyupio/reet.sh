<script lang="ts">
 import { onMount, untrack } from 'svelte'
 import { getAdoptionBreakdown, type AdoptionBreakdown, type ContentPreview } from './api'
 import { BREAKDOWN_LABELS, tableMatches, OWNERSHIP_SENTENCE, breakdownNote, candidateCounts, candidatesFor, claimSummary, groupBreakdown, preselect } from './contentReview'
 import ConfirmDialog from './ConfirmDialog.svelte'

 // Shown by the Check step only while the check is current and adoptable, so it is mounted exactly then: it is gone the
 // moment the check goes stale, is overtaken or needs reading again. Presentational apart from the one metadata read of
 // the file breakdown. Nothing is chosen for the person unless the library holds exactly one matching revision, and even
 // then the breakdown and the confirmation still follow. onadopt is told which preview and review hash the person saw;
 // the shell reads the check once more before it sends anything.
 let { check, busy, blocked = '', labelFor, onadopt, onlapse }: {
  check: ContentPreview
  busy: boolean
  // Why recording is unavailable right now (empty when it is): the scope no longer qualifies.
  blocked?: string
  // A revision in words (the date it was made); empty when it is not in a list that has been read.
  labelFor: (revisionId: string) => string
  onadopt: (revisionId: string, seen: { previewId: string; reviewHash: string }) => Promise<void>
  // The step went away while its confirmation was open (the check lapsed): the shell says so.
  onlapse?: () => void
 } = $props()
 const uid = $props.id()
 const candidates = $derived(candidatesFor(check.adoption))
 let chosen = $state(''), breakdown = $state<AdoptionBreakdown | null>(null), loadingBreakdown = $state(false), breakdownError = $state('')
 let asking = $state(false), sending = false
 // What the table on screen was read for. The hash that is sent is this one, never whatever the check holds now.
 let loadedFor = $state<{ previewId: string; reviewHash: string; revisionId: string } | null>(null)
 let generation = 0, abort: AbortController | undefined, lastKey = ''
 // What the breakdown on screen was read for: another read of the same check changes nothing, a different review hash does.
 const key = $derived(`${check.id}:${check.reviewHash ?? ''}`)
 const rows = $derived(breakdown && breakdown.revisionId === chosen ? groupBreakdown(breakdown) : [])
 const summary = $derived(breakdown && breakdown.revisionId === chosen ? claimSummary(breakdown) : undefined)
 const loaded = $derived(!!summary && !loadingBreakdown && breakdownError === '')
 const canRecord = $derived(loaded && !!summary && summary.claimed > 0 && chosen !== '' && !busy && blocked === '' && tableMatches(loadedFor, check, chosen))
 const nameOf = (id: string, i: number) => labelFor(id) || `Another revision (${i + 1})`
 const chosenName = $derived(nameOf(chosen, Math.max(0, candidates.findIndex(c => c.revisionId === chosen))))
 const size = (n: number) => (n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`)

 // The latest choice wins: a slow answer for an earlier one is dropped, and the earlier request is abandoned.
 async function loadBreakdown() {
  abort?.abort()
  const mine = ++generation
  breakdown = null; loadedFor = null; breakdownError = ''
  if (!chosen) { loadingBreakdown = false; return }
  const revision = chosen
  const forKey = { previewId: check.id, reviewHash: check.reviewHash ?? '', revisionId: revision }
  const own = new AbortController(); abort = own
  loadingBreakdown = true
  try {
   const read = await getAdoptionBreakdown(check.id, revision, own.signal)
   if (mine !== generation) return
   if (read.revisionId !== revision) { breakdownError = "This revision can't be compared with the folder any more. Check again."; return }
   breakdown = read; loadedFor = forKey
  } catch (e) {
   if (mine === generation && !own.signal.aborted) breakdownError = breakdownNote(e)
  } finally {
   if (mine === generation) loadingBreakdown = false
  }
 }
 function choose(id: string) {
  if (id === chosen) return
  asking = false; chosen = id
  void loadBreakdown()
 }
 // When the check is read again with another review hash (or for the first time), the table is read again for the same
 // choice and any open confirmation is closed, so what is confirmed is what is on screen.
 $effect(() => {
  const k = key
  untrack(() => {
   if (k === lastKey) return
   const first = lastKey === ''
   lastKey = k; asking = false
   if (first) chosen = preselect(check.adoption)
   else if (chosen && !candidates.some(c => c.revisionId === chosen)) chosen = ''
   void loadBreakdown()
  })
 })
 onMount(() => () => { abort?.abort(); generation++; if (asking && !sending) onlapse?.() })

 async function confirmed() {
  asking = false
  const read = loadedFor
  if (sending || !chosen || !read || read.revisionId !== chosen || read.reviewHash === '') return
  sending = true
  try { await onadopt(read.revisionId, { previewId: read.previewId, reviewHash: read.reviewHash }) } finally { sending = false }
 }
</script>

<section class="adopt" aria-labelledby={`${uid}-h`}>
 <h4 id={`${uid}-h`}>Record ownership</h4>
 <p class="hint">Recording ownership says which of these files are identical to a revision in your library. It changes nothing on the machine.</p>
 {#if candidates.length === 0}
  <p class="verdict">No revision of this library skill matches the files in this folder, so there is nothing to record.</p>
 {:else}
  <fieldset>
   <legend>Library revision to record ownership against</legend>
   {#each candidates as c, i (c.revisionId)}
    <label class="choice">
     <input type="radio" name={`${uid}-rev`} value={c.revisionId} checked={chosen === c.revisionId} disabled={busy} onchange={() => choose(c.revisionId)} />
     <span><strong>{nameOf(c.revisionId, i)}</strong><small>{candidateCounts(c)}</small></span>
    </label>
   {/each}
  </fieldset>
 {/if}

 <div class="breakdown" role="status" aria-live="polite">
  {#if !chosen && candidates.length > 0}Choose a revision to see which files would be recorded.
  {:else if loadingBreakdown}Reading the file list for this revision…
  {:else if breakdownError}{breakdownError}
  {:else if summary}{summary.text}{/if}
 </div>
 {#if breakdownError}<p><button type="button" class="btn" onclick={() => void loadBreakdown()}>Try again</button></p>{/if}

 {#if rows.length}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users must be able to scroll this overflow region.) -->
  <div class="scroll" role="region" aria-label="Files this revision would claim (scrolls)" tabindex="0">
   <table>
    <caption>What recording ownership would do to each file</caption>
    <thead><tr><th scope="col">File</th><th scope="col">Result</th><th scope="col">Size</th></tr></thead>
    <tbody>{#each rows as r, i (`${i}:${r.path}`)}<tr><th scope="row">{r.path}</th><td>{BREAKDOWN_LABELS[r.state]}</td><td>{size(r.bytes)}</td></tr>{/each}</tbody>
   </table>
  </div>
  <p class="hint">Files that are not claimed stay yours; reet leaves them alone.</p>
 {/if}

 <div class="actions">
  <button type="button" class="btn primary" disabled={!canRecord} aria-describedby={blocked ? `${uid}-why` : undefined} onclick={() => { asking = true }}>Record ownership</button>
 </div>
 {#if blocked}<p class="hint" id={`${uid}-why`}>{blocked}</p>{/if}
</section>

{#if asking && chosen}
 <ConfirmDialog
  title={`Record ownership of ${check.name}?`}
  body={`${summary ? summary.text : ''} The revision is ${chosenName}. ${OWNERSHIP_SENTENCE}`.trim()}
  confirmLabel="Confirm and record"
  onconfirm={() => void confirmed()}
  oncancel={() => { asking = false }}
 >
  <p class="note">Nothing on the machine is changed.</p>
  <p class="note">Files that are not listed as claimed stay yours.</p>
  <p class="note">You can release this record later.</p>
 </ConfirmDialog>
{/if}

<style>
 .adopt { margin: 18px 0 0; padding: 14px 0 0; border-top: 1px solid var(--line); }
 h4 { margin: 0 0 6px; font-size: 1.05rem; }
 .hint { color: var(--muted); font-size: .8rem; line-height: 1.5; margin: 6px 0 12px; }
 .verdict { margin: 12px 0; font-size: .9rem; line-height: 1.5; }
 fieldset { margin: 0 0 12px; padding: 0; border: 0; min-width: 0; }
 legend { padding: 0; margin: 0 0 6px; font-size: .8rem; font-weight: 600; }
 .choice { display: flex; gap: 10px; align-items: flex-start; padding: 8px 10px; margin: 0 0 6px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
 .choice span { display: grid; gap: 2px; min-width: 0; font-size: .85rem; overflow-wrap: anywhere; }
 .choice small { color: var(--muted); font-size: .75rem; }
 .breakdown { min-height: 1.4em; margin: 0 0 8px; font-size: .85rem; line-height: 1.5; overflow-wrap: anywhere; }
 .scroll { max-height: 320px; overflow: auto; margin: 10px 0; border: 1px solid var(--line); border-radius: 8px; }
 table { width: 100%; border-collapse: collapse; font-size: .8rem; }
 caption { text-align: left; padding: 8px 10px; font-weight: 600; }
 th, td { padding: 6px 10px; text-align: left; vertical-align: top; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
 thead th { color: var(--muted); font-weight: 500; }
 tbody th { font-weight: 400; }
 .actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0 0; }
 .note { margin: 8px 0 0; color: var(--ink); font-size: .9rem; }
</style>
