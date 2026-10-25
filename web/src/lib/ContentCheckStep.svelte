<script lang="ts">
 import { onMount, tick, type Snippet } from 'svelte'
 import type { ContentPin, ContentPreview } from './api'
 import { APPLY_NOTICE, CONNECTOR_PROBLEM, classifyCheck, FALLBACK, modeLabel, ownershipNote, planCodeText, planVerdict, preservedText, presenceLabel } from './contentReview'

 // Presentational: the shell reads the check, owns every request and every timer, and says whether one is under way.
 // readAt is the browser time the check's response arrived. The step asks classifyCheck with the time of the question
 // (read afresh each second) and shows a result only when that says the check is current: anything older, overtaken or
 // ended shows its reason and Check again, never the plan's verdict. ready says the connector can take a check now (why
 // says what is wrong when it cannot). loading is a check that is known but not yet read in full.
 let { pin, check, readAt, ready, why = '', busy, loading = false, resumeName = '', revision = '', madeFor = '', oncheck, onresume, oncancel, ondismiss, adopt }: {
  pin: ContentPin
  check: ContentPreview | null
  readAt: number
  ready: boolean
  why?: string
  busy: boolean
  loading?: boolean
  // The folder whose check is already running on this machine, when there is one to resume.
  resumeName?: string
  revision?: string
  // The revision this check was made for, when it is not the pinned one.
  madeFor?: string
  oncheck: () => void
  onresume: () => void
  oncancel: () => void
  ondismiss: () => void
  // The Record ownership step, drawn only while the check is current and adoptable (this step decides, once a second).
  adopt?: Snippet
 } = $props()
 const uid = $props.id()
 let headingEl = $state<HTMLElement>()
 // Only here to make the view ask again as time passes; the time itself is read when the question is asked.
 let beat = $state(0)
 const view = $derived.by(() => { void beat; return classifyCheck(check, readAt, Date.now()) })
 const capture = $derived(view.kind === 'current' ? check?.capture : undefined)
 const note = $derived(capture ? ownershipNote(capture.rootOwnership) : '')
 const summary = $derived(capture && check && !note ? planVerdict(check) : undefined)
 const canResume = $derived(resumeName !== '')
 const owned = $derived(new Set((check?.plan?.preconditions?.baseline?.manifest?.files ?? []).map(f => f.path)))
 const found = $derived(capture?.presence === 'present' && !note ? (capture.manifest?.files ?? []) : [])
 const notReady = $derived(why || "The connector on this machine hasn't checked in recently, so it can't take a check yet. This updates by itself when it does.")
 const KINDS: Record<string, string> = { create: 'Would create', update: 'Would update', remove: 'Would remove' }
 const kindText = (kind: string) => (Object.prototype.hasOwnProperty.call(KINDS, kind) ? KINDS[kind] : 'Would change')
 const size = (n: number) => (n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`)
 const progress = $derived(loading ? 'Reading the latest check…' : view.headline)

 // An action swaps the button that was pressed for another, which would leave focus nowhere: put it on the heading.
 let mounted = false
 $effect(() => {
  void view.kind; void check?.id; void canResume; void busy
  if (!mounted) return
  void tick().then(() => {
   const a = document.activeElement
   if (!a || a === document.body || a.tagName === 'DIALOG' || !a.isConnected || (a instanceof HTMLButtonElement && a.disabled)) headingEl?.focus()
  })
 })
 onMount(() => {
  headingEl?.focus(); mounted = true
  const timer = setInterval(() => { beat++ }, 1000)
  return () => clearInterval(timer)
 })
</script>

<section aria-labelledby={`${uid}-h`}>
 <h3 id={`${uid}-h`} tabindex="-1" bind:this={headingEl}>Check this machine</h3>
 <p class="hint">A check reads file names, sizes and digests only. It never reads file contents, and it changes nothing on the machine.</p>
 <p class="which"><strong>{pin.name}</strong>{#if revision} · Revision {revision}{/if} · {modeLabel(pin.mode)}</p>
 <div class="check-live" role="status" aria-live="polite">{progress}</div>

 {#if !loading}
  {#if madeFor && check && view.kind !== 'working'}<p class="hint">This check was made for revision {madeFor}.</p>{/if}
  {#if view.reasons.length}
   <ul class="reasons">{#each view.reasons as reason (reason)}<li>{reason}</li>{/each}</ul>
  {/if}

  <div class="actions">
   {#if canResume}
    <button type="button" class="btn primary" disabled={busy} onclick={onresume}>Resume the check for {resumeName}</button>
   {:else if view.kind === 'working'}
    <button type="button" class="btn" disabled={busy} onclick={oncancel}>Stop this check</button>
   {:else if view.kind === 'none'}
    <button type="button" class="btn primary" disabled={busy || !ready} aria-describedby={ready ? undefined : `${uid}-why`} onclick={oncheck}>Check this machine</button>
   {:else}
    <button type="button" class="btn primary" disabled={busy || !ready} aria-describedby={ready ? undefined : `${uid}-why`} onclick={oncheck}>Check again</button>
   {/if}
   {#if check && view.kind !== 'working' && view.kind !== 'none'}
    <button type="button" class="btn" disabled={busy} onclick={ondismiss}>Dismiss this result</button>
   {/if}
  </div>
  {#if !ready && !canResume && view.kind !== 'working'}<p class="hint" id={`${uid}-why`}>{notReady}</p>{/if}

  {#if capture}
   <div class="result">
    <h4>{presenceLabel(check)}</h4>
    {#if note}<p class="warn">{note}</p>{/if}
    {#if capture.issues?.length}
     <p class="verdict">{CONNECTOR_PROBLEM}</p>
    {/if}
    {#if found.length}
     <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users must be able to scroll this overflow region.) -->
     <div class="scroll" role="region" aria-label="Files found in the folder (scrolls)" tabindex="0">
      <table>
       <caption>Files found in the folder</caption>
       <thead><tr><th scope="col">File</th><th scope="col">Size</th><th scope="col">Ownership</th></tr></thead>
       <tbody>{#each found as f, i (`${i}:${f.path}`)}<tr><th scope="row">{f.path}</th><td>{size(f.bytes)}</td><td>{!check?.plan ? 'Not known' : owned.has(f.path) ? 'Managed by reet' : 'Not managed by reet'}</td></tr>{/each}</tbody>
      </table>
     </div>
    {/if}
    {#if summary}
     {#if summary.kind === 'up_to_date'}
      <p class="verdict">Up to date. The files reet has recorded here match the pinned revision.</p>
     {:else if summary.kind === 'changes'}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users must be able to scroll this overflow region.) -->
      <div class="scroll" role="region" aria-label="What a later apply would change (scrolls)" tabindex="0">
       <table>
        <caption>What a later apply would change</caption>
        <thead><tr><th scope="col">Change</th><th scope="col">File</th></tr></thead>
        <tbody>{#each summary.actions as a, i (`${i}:${a.path}`)}<tr><td>{kindText(a.kind)}</td><th scope="row">{a.path}</th></tr>{/each}</tbody>
       </table>
      </div>
      <p class="hint">{APPLY_NOTICE}</p>
     {:else if summary.kind === 'adoption'}
      <p class="verdict">{planCodeText('adoption_required')}</p>
      {#if view.canAdopt && adopt}{@render adopt()}{/if}
     {:else if summary.kind === 'conflicts'}
      <ul class="issues">{#each summary.conflicts as c, i (i)}<li>{#if c.path}<code>{c.path}</code> {/if}{planCodeText(c.code)}</li>{/each}</ul>
     {:else}
      <p class="verdict">{FALLBACK}</p>
     {/if}
     {#if summary.preserved.length}
      <p class="label">Files left alone</p>
      <ul class="issues">{#each summary.preserved as f, i (`${i}:${f.path}`)}<li><code>{f.path}</code> {preservedText(f.reason)}</li>{/each}</ul>
     {/if}
    {/if}
   </div>
  {/if}
 {/if}
</section>

<style>
 h3 { margin: 0 0 8px; font-size: 1rem; }
 h3:focus { outline: none; }
 h3:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
 h4 { margin: 18px 0 6px; font-size: 1.05rem; }
 .hint, .which { color: var(--muted); font-size: .8rem; line-height: 1.5; margin: 6px 0 12px; }
 .which strong { color: var(--ink); }
 .check-live { min-height: 1.4em; margin: 0 0 10px; font-size: .9rem; }
 .actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 8px; }
 .reasons, .issues { margin: 0 0 12px; padding-left: 18px; font-size: .85rem; line-height: 1.5; overflow-wrap: anywhere; }
 .warn { margin: 0 0 12px; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--bad) 40%, var(--line)); border-radius: 8px; color: var(--bad); font-size: .85rem; }
 .label { margin: 12px 0 4px; font-size: .8rem; font-weight: 600; }
 .verdict { margin: 12px 0; font-size: .9rem; line-height: 1.5; }
 .scroll { max-height: 320px; overflow: auto; margin: 10px 0; border: 1px solid var(--line); border-radius: 8px; }
 table { width: 100%; border-collapse: collapse; font-size: .8rem; }
 caption { text-align: left; padding: 8px 10px; font-weight: 600; }
 th, td { padding: 6px 10px; text-align: left; vertical-align: top; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
 thead th { color: var(--muted); font-weight: 500; }
 tbody th { font-weight: 400; }
 code { overflow-wrap: anywhere; }
</style>
