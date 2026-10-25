<script lang="ts">
 import { onMount, tick } from 'svelte'
 import type { ContentPin, PrivateBundle, PrivateRevision } from './api'
 import { bundleLabel, describeError, groupBundles, MODES, modeLabel, newestFirst, preselectBundle, revisionLabel, revisionLabels, samePin, shortId, type PinChoice } from './contentReview'

 // Presentational: the shell loads the pins and the library, owns every write and says whether one is under way.
 // preset is the skill name the machine reported, when the dialog was opened from that skill's row. agentId is the
 // connector the shell will send the pin through, so a pin made through another one is not "the current pin".
 // saveBlocked is why saving is unavailable right now (empty when it is). revisionLists are the revisions the shell has
 // read, by library skill, so a pin can be described by when its revision was made rather than by an id.
 let { pins, bundles, preset = '', agentId, saveBlocked = '', busy, revisionLists, revisionsFor, statusFor, oncheck, onsave, onremove }: {
  pins: ContentPin[]
  bundles: PrivateBundle[]
  preset?: string
  agentId: string
  saveBlocked?: string
  busy: boolean
  revisionLists: Record<string, PrivateRevision[] | undefined>
  revisionsFor: (bundleId: string) => Promise<PrivateRevision[]>
  // The status line from a pin's latest check, and the way into the Check step for it.
  statusFor?: (pin: ContentPin) => string
  oncheck?: (pin: ContentPin) => void
  onsave: (input: PinChoice) => Promise<void>
  onremove: (pin: ContentPin) => Promise<void>
 } = $props()
 const uid = $props.id()
 let bundleId = $state(''), revisionId = $state(''), mode = $state('additive')
 let revisions = $state<PrivateRevision[]>([]), revisionsLoading = $state(false), revisionError = $state('')
 let librarySelect = $state<HTMLSelectElement>(), pinsHeading = $state<HTMLElement>()
 // Revision lists can arrive out of order; only the answer to the latest choice may be used.
 let alive = true, generation = 0
 const groups = $derived(groupBundles(bundles, preset))
 const bundle = $derived(bundles.find(b => b.id === bundleId))
 // A pin belongs to a scope and a skill name, so choosing a library skill of the same name changes that pin.
 const existing = $derived(bundle ? pins.find(p => p.name === bundle.name) : undefined)
 const unchanged = $derived(samePin(existing, { agentId, bundleId, revisionId, mode }))
 const ready = $derived(!!bundle && revisionId !== '' && !revisionsLoading && revisionError === '' && MODES.some(m => m.value === mode))
 const headOf = (id: string) => bundles.find(b => b.id === id)?.head
 const labels = $derived(revisionLabels(revisions, bundle?.head))
 const labelOf = (pin: ContentPin) => revisionLabel(pin.revisionId, revisionLists[pin.bundleId], headOf(pin.bundleId))

 async function choose(id: string, wantedRevision = '', wantedMode = '') {
  const token = ++generation
  const chosen = bundles.find(b => b.id === id)
  bundleId = id; revisions = []; revisionId = ''; revisionError = ''
  mode = wantedMode || (chosen ? pins.find(p => p.name === chosen.name)?.mode : undefined) || 'additive'
  revisionsLoading = !!chosen
  if (!chosen) return
  try {
   const list = await revisionsFor(id)
   if (!alive || token !== generation) return
   revisions = newestFirst(list, chosen.head)
   revisionId = revisions.some(r => r.id === wantedRevision) ? wantedRevision : (revisions[0]?.id ?? '')
  } catch (e) {
   if (alive && token === generation) revisionError = describeError(e)
  } finally {
   if (alive && token === generation) revisionsLoading = false
  }
 }
 async function change(pin: ContentPin) {
  void choose(pin.bundleId, pin.revisionId, pin.mode)
  await tick()
  librarySelect?.focus()
 }
 async function submit(event: SubmitEvent) {
  event.preventDefault()
  if (busy || !ready || unchanged || saveBlocked) return
  await onsave({ bundleId, revisionId, mode: mode as PinChoice['mode'], expectedVersion: existing?.version ?? '' })
 }
 async function remove(pin: ContentPin) {
  if (busy) return
  await onremove(pin)
  // The pin the person was on has gone, so put focus on the list it was in.
  if (!alive || pins.some(p => p.id === pin.id)) return
  await tick()
  pinsHeading?.focus()
 }
 onMount(() => {
  const id = preselectBundle(bundles, preset)
  if (id) void choose(id)
  return () => { alive = false }
 })
</script>

<section aria-labelledby={`${uid}-pins`}>
 <h3 id={`${uid}-pins`} tabindex="-1" bind:this={pinsHeading}>Pinned in this folder</h3>
 {#if pins.length === 0}
  <p class="empty">Nothing is pinned to this folder yet.</p>
 {:else}
  <ul class="pins" role="list" aria-labelledby={`${uid}-pins`}>
   {#each pins as pin (pin.id)}
    {@const head = headOf(pin.bundleId)}
    <li>
     <div class="info">
      <strong>{pin.name}</strong>
      <span>Revision {labelOf(pin)}{#if head && pin.revisionId !== head}<span class="where">· Not the newest in the library</span>{/if}</span>
      <span>{modeLabel(pin.mode)}</span>
      {#if statusFor}<span>{statusFor(pin)}</span>{/if}
     </div>
     <div class="row-actions">
      {#if oncheck}<button type="button" class="btn" disabled={busy} aria-label={`Open check for ${pin.name}`} onclick={() => oncheck(pin)}>Check</button>{/if}
      <button type="button" class="btn" disabled={busy} aria-label={`Change pin for ${pin.name}`} onclick={() => void change(pin)}>Change pin</button>
      <button type="button" class="btn" disabled={busy} aria-label={`Remove pin for ${pin.name}`} onclick={() => void remove(pin)}>Remove pin</button>
     </div>
    </li>
   {/each}
  </ul>
 {/if}
</section>

<section aria-labelledby={`${uid}-form`}>
 <h3 id={`${uid}-form`}>Pin a library revision</h3>
 <p class="hint">A pin is intent only: it records which library revision this folder should have. It installs nothing and writes nothing to the machine.</p>
 {#if bundles.length === 0}
  <p class="empty">Your library has no private skills yet. Bring one in with Private skill imports, then pin it here.</p>
 {:else}
  {#if preset && groups.matching.length === 0}<p class="hint">Your library has no private skill named {preset} yet.</p>{/if}
  <form onsubmit={submit}>
   <div class="field">
    <label for={`${uid}-library`}>Library skill</label>
    <select id={`${uid}-library`} bind:this={librarySelect} value={bundleId} onchange={(event) => void choose(event.currentTarget.value)}>
     <option value="">Choose a library skill</option>
     {#if groups.matching.length}
      <optgroup label="Matches the skill reported on this machine">{#each groups.matching as b (b.id)}<option value={b.id}>{bundleLabel(b, bundles)}</option>{/each}</optgroup>
      {#if groups.others.length}<optgroup label="Other library skills">{#each groups.others as b (b.id)}<option value={b.id}>{bundleLabel(b, bundles)}</option>{/each}</optgroup>{/if}
     {:else}
      {#each groups.others as b (b.id)}<option value={b.id}>{bundleLabel(b, bundles)}</option>{/each}
     {/if}
    </select>
   </div>
   <div class="field">
    <label for={`${uid}-revision`}>Revision</label>
    <select id={`${uid}-revision`} value={revisionId} disabled={revisions.length === 0} onchange={(event) => { revisionId = event.currentTarget.value }}>
     {#if revisions.length === 0}<option value="">No revision to choose yet</option>{/if}
     {#each revisions as r (r.id)}<option value={r.id}>{labels.get(r.id) ?? shortId(r.id)}</option>{/each}
    </select>
    {#if revisionsLoading}<small role="status">Loading revisions…</small>{/if}
    {#if revisionError}<div class="inline"><small role="alert" class="error">{revisionError}</small><button type="button" class="btn" onclick={() => void choose(bundleId, '', mode)}>Try again</button></div>{/if}
    {#if bundle && !revisionsLoading && !revisionError && revisions.length > 1}<small>The newest revision is chosen first. Older ones are there if you want to roll back.</small>{/if}
   </div>
   <fieldset>
    <legend>Mode</legend>
    {#each MODES as m (m.value)}
     <label class="choice"><input type="radio" name={`${uid}-mode`} value={m.value} checked={mode === m.value} onchange={() => { mode = m.value }} /><span>{m.label}</span></label>
    {/each}
   </fieldset>
   {#if existing && bundle}
    <p class="hint">{#if unchanged}This is the current pin for {bundle.name}.{:else}This folder already has a pin for {bundle.name}: revision {labelOf(existing)}, {modeLabel(existing.mode)}. Saving changes it.{/if}{#if existing.bundleId !== bundleId} It points at a different library skill with the same name.{/if}{#if agentId && existing.agentId !== agentId} It was made through a different connector; saving moves it to this machine's running one.{/if}</p>
   {/if}
   {#if saveBlocked}<p class="hint">Pins can't be saved right now. {saveBlocked} You can still review and remove pins.</p>{/if}
   <div class="form-actions"><button type="submit" class="btn primary" disabled={busy || !ready || unchanged || !!saveBlocked}>Save pin</button></div>
  </form>
 {/if}
</section>

<style>
 section { margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--line); }
 section:first-child { margin-top: 0; padding-top: 0; border-top: 0; }
 h3 { margin: 0 0 8px; font-size: 1rem; }
 h3:focus { outline: none; }
 h3:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
 .hint, .empty, small { color: var(--muted); font-size: .8rem; line-height: 1.5; }
 .hint, .empty { margin: 6px 0 12px; }
 small { display: block; }
 .error { color: var(--bad); overflow-wrap: anywhere; }
 .pins { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
 .pins li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 14px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; }
 .info { display: grid; gap: 2px; min-width: 0; flex: 1 1 220px; font-size: .8rem; overflow-wrap: anywhere; }
 .info strong { font-size: .9rem; }
 .info span { color: var(--muted); }
 .where { margin-left: .4em; }
 .row-actions { display: flex; flex-wrap: wrap; gap: 8px; }
 form { display: grid; gap: 16px; }
 .field { display: grid; gap: 5px; min-width: 0; }
 label, legend { font-size: .82rem; font-weight: 600; }
 select { width: 100%; min-width: 0; }
 fieldset { display: grid; gap: 8px; margin: 0; padding: 0; border: 0; min-width: 0; }
 legend { padding: 0; margin-bottom: 6px; }
 .choice { display: flex; align-items: flex-start; gap: 9px; font-weight: 400; line-height: 1.45; }
 .choice input { flex: none; width: 16px; height: 16px; margin-top: 2px; }
 .inline { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
 .form-actions { display: flex; flex-wrap: wrap; gap: 8px; }
 @media (max-width: 500px) { .pins li { padding: 10px 12px; } .row-actions { width: 100%; } .row-actions .btn { flex: 1 1 auto; } }
</style>
