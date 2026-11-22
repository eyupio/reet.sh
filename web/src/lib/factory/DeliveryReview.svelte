<script lang="ts">
  // A pipeline's delivery review: the criteria the person agreed, the evidence for each, the commit that was looked at,
  // and the two decisions, side by side. Reading it asks the pipeline's machine for the folder's commit, which can take
  // several seconds, so it says so. Accepting the delivery is not approving the merge; the card says that wherever it
  // matters. Every write names the revisions and the commit it was shown, and carries a request ID that is kept for a
  // retry after a lost answer, so an interrupted request is recorded once.
  import { onMount, tick, untrack } from 'svelte'
  import { ApiError, addGafferDeliveryEvidence, decideGafferDelivery, getGafferDelivery, setGafferDeliveryCriteria, type DeliveryCriterion, type DeliveryEvidence, type DeliveryReview, type GafferOutcome, type GafferOutcomeSession } from '../api'
  import { changedAfterAccept, cleanCriteria, criteriaBlocker, deliveryAnchor, stageLabel } from '../gafferOutcome'
  import { signalAttentionChanged } from '../attentionSignal'
  import { checkLabel, checkTone, safeInsightURL } from '../sessionInsights'
  import CriteriaEditor from './CriteriaEditor.svelte'
  import { mergesByItself } from './PipelineChoices.svelte'
  import { EDITABLE_STATES, KIND_WORD, STATUS_MARK, STATUS_WORD, acceptConfirm, attemptIds, commitBlocker, commitStateWord, conflictAdvice, conflictWords, decisionLine, earlierLabel, evidenceKinds, mayHaveArrived, nextNow, nextWords, noChecksWords, noCommitWords, notPassedThen, sendBackBlocker, shortCommit, statusCount, statusMap, summaryLine } from './delivery'

  let { record, piece, archived = false, reviewShown = false, when, onchanged }: {
    record: GafferOutcome
    piece: GafferOutcomeSession
    /** An archived outcome is read, never changed. */
    archived?: boolean
    /** The address or a button asked for this review: it is read at once, whatever the pipeline's state. */
    reviewShown?: boolean
    when: (at: number) => string
    /** Something was recorded: the page reads the outcomes again. */
    onchanged?: () => void | Promise<void>
  } = $props()
  const uid = $props.id()
  const anchor = $derived(deliveryAnchor(record.id, piece.key))
  const agreed = $derived.by(() => { const n = piece.criteria?.length ?? 0, d = piece.deliveries?.length ?? 0; return `${n} ${n === 1 ? 'criterion' : 'criteria'} agreed${d ? ` · decided ${d === 1 ? 'once' : `${d} times`} before` : ''}` })

  // A pipeline waiting for its delivery review is read as soon as it is shown. One at any other point has the review
  // closed until it is asked for, so that choosing a pipeline does not ask its machine anything.
  let asked = $state(false)
  const wanted = $derived(piece.state === 'delivery_review' || reviewShown || asked)
  let review = $state<DeliveryReview | null>(null), reading = $state(false), readError = $state('')
  let controller: AbortController | undefined, generation = 0, started = false
  async function load() {
    controller?.abort()
    const mine = ++generation, mind = controller = new AbortController()
    reading = true; readError = ''
    try {
      const next = await getGafferDelivery(record, piece.key, AbortSignal.any([mind.signal, AbortSignal.timeout(45_000)]))
      if (mine === generation) review = next
    } catch (e) {
      if (mine === generation) readError = mind.signal.aborted ? '' : e instanceof Error && e.name !== 'TimeoutError' ? e.message : 'The machine did not answer in time.'
    } finally { if (mine === generation) { reading = false; settle() } }
  }
  onMount(() => () => { generation++; controller?.abort() })
  $effect(() => { if (wanted && !started) { started = true; void untrack(load) } })
  // Something else recorded a change (another reviewer, the controller): the card reads again rather than show an old
  // one. The pipeline's state alone does not warrant a read of the machine (it moves from advancing to ready to merge
  // by itself after an acceptance); only entering or leaving delivery review does. A change that arrives while the card
  // is reading or writing is remembered and looked at when that ends.
  let stale = false
  const behind = () => !!review && ((piece.reviewRevision !== undefined && piece.reviewRevision > review.reviewRevision) || (piece.state === 'delivery_review') !== (review.state === 'delivery_review'))
  function sync() {
    if (!review) return
    if (reading || deciding || saving || addingBusy) { stale = true; return }
    if (behind()) void load()
  }
  function settle() { if (stale && !reading && !deciding && !saving && !addingBusy) { stale = false; sync() } }
  $effect(() => { void piece.reviewRevision; void piece.state; untrack(sync) })

  const decideIds = attemptIds(), evidenceIds = attemptIds()
  // What accepting leads to is worked out from the pipeline as it is now: the merge setting can change while the review is open.
  const next = $derived(review ? nextNow(review.next, mergesByItself(record, piece)) : undefined)
  const count = $derived(review ? statusCount(review) : null)
  const canEdit = $derived(EDITABLE_STATES.includes(piece.state))
  const kinds = $derived(review ? evidenceKinds(review) : ['note' as const])
  const blocker = $derived(review ? commitBlocker(review) : '')
  const edits = $derived(!archived && !!review && canEdit)
  const decides = $derived(!archived && !!review?.open)

  // What a refusal said, kept above the controls it concerns. A conflict is not an error: the review is read again and
  // the person decides on what is now shown, with what they typed still in the fields.
  let notice = $state(''), problem = $state(''), conflict = $state('')
  const clear = () => { notice = ''; problem = ''; conflict = '' }
  const SAFE = 'It may not have arrived. Trying again is safe: it is recorded once.'
  /** Says what a refusal was. Returns whether it was a conflict, after which the review has been read again. */
  async function afterRefusal(e: unknown, ids: ReturnType<typeof attemptIds> | null, fallback: string): Promise<boolean> {
    if (ids && !mayHaveArrived(e)) ids.clear()
    if (e instanceof ApiError && e.status === 409) {
      conflict = `${conflictWords(e.code) || e.message || 'The review changed.'} ${conflictAdvice}`
      await load()
      return true
    }
    const said = e instanceof ApiError ? e.message : e instanceof Error && e.message ? e.message : fallback
    problem = ids && mayHaveArrived(e) ? `${said} ${SAFE}` : said
    return false
  }

  // Accepting asks first; sending back is a form of its own, so what is typed in it survives a conflict.
  let confirming = $state(false), sending = $state(false), deciding = $state(false)
  let notes = $state(''), stage = $state('')
  $effect(() => { if (review?.stages?.length && !review.stages.includes(stage)) stage = review.defaultStage && review.stages.includes(review.defaultStage) ? review.defaultStage : review.stages[review.stages.length - 1] })
  const sendBlocker = $derived(review ? sendBackBlocker(review, notes, stage) : '')
  async function decide(kind: 'accept' | 'send_back') {
    const shown = review
    if (deciding || !shown?.open || archived) return
    const body = { decision: kind, criteriaRevision: shown.criteriaRevision, reviewRevision: shown.reviewRevision, commit: shown.commit, statuses: statusMap(shown), ...(kind === 'send_back' ? { notes: notes.trim(), stage } : {}), confirmed: true as const }
    deciding = true; clear()
    try {
      const answer = await decideGafferDelivery(record, piece.key, { ...body, requestId: decideIds.idFor(JSON.stringify(body)) })
      decideIds.clear()
      review = answer.review; sending = false; notes = ''
      notice = kind === 'accept'
        ? `Delivery accepted${shown.commit ? ` on ${shortCommit(shown.commit)}` : ''}. ${nextWords(next)}`
        : `Sent back to ${stageLabel(stage).toLowerCase()}. The controller is starting that stage again, and the agent is told what failed and your notes.`
      signalAttentionChanged()
      await onchanged?.()
      await tick(); heading?.focus({ preventScroll: true })
    } catch (e) {
      await afterRefusal(e, decideIds, kind === 'accept' ? 'The delivery was not accepted.' : 'The pipeline was not sent back.')
      confirming = false; deciding = false
      // The confirmation is gone: the keyboard goes back to the button that opened it, or to the heading when nothing is left to press.
      await tick()
      const back = kind === 'accept' ? acceptButton : sendToggle
      if (back) back.focus(); else heading?.focus({ preventScroll: true })
    }
    finally { deciding = false; confirming = false; settle() }
  }
  // Accepting asks first, in the card itself rather than over it: the button that confirms stays where it was pressed and
  // waits while the decision is in flight, so a second press lands on nothing.
  let cancelAccept = $state<HTMLButtonElement>(), acceptButton = $state<HTMLButtonElement>(), sendToggle = $state<HTMLButtonElement>()
  async function askToAccept() { clear(); confirming = true; await tick(); cancelAccept?.focus() }
  const escapes = (event: KeyboardEvent) => { if (event.key === 'Escape' && !deciding) { event.stopPropagation(); closeConfirm() } }
  function closeConfirm() { confirming = false; void tick().then(() => acceptButton?.focus()) }

  // Evidence is added against one criterion at a time.
  type Draft = { criterion: string; kind: 'note' | 'check' | 'artefact'; verdict: 'passed' | 'needs_work'; text: string; url: string; name: string }
  let draft = $state<Draft | null>(null), addingBusy = $state(false)
  const evidenceBlocker = (d: Draft): string => {
    if (d.kind === 'note') return d.text.trim() ? '' : 'Write the note.'
    if (d.kind === 'check') return d.name ? '' : 'Choose the GitHub check.'
    return d.url.trim() ? '' : 'Paste the link to the artefact.'
  }
  function startEvidence(item: DeliveryCriterion) { clear(); draft = { criterion: item.id, kind: 'note', verdict: 'passed', text: '', url: '', name: '' } }
  async function addEvidence() {
    const shown = review, d = draft
    if (addingBusy || !shown || !d || evidenceBlocker(d) || archived) return
    const body = { reviewRevision: shown.reviewRevision, criterionId: d.criterion, kind: d.kind, ...(d.kind === 'check' ? { name: d.name } : { verdict: d.verdict, ...(d.text.trim() ? { text: d.text.trim() } : {}), ...(d.kind === 'artefact' ? { url: d.url.trim() } : {}) }) }
    addingBusy = true; clear()
    const forId = d.criterion
    try {
      review = await addGafferDeliveryEvidence(record, piece.key, { ...body, requestId: evidenceIds.idFor(JSON.stringify(body)) })
      evidenceIds.clear(); draft = null
      notice = 'Evidence added.'
      await onchanged?.()
      await tick(); document.getElementById(`${uid}-add-${forId}`)?.focus()
    } catch (e) { await afterRefusal(e, evidenceIds, 'The evidence was not added.') }
    finally { addingBusy = false; settle() }
  }

  // The criteria are edited in place, with the revision that was shown.
  // After a conflict the rows are the server's again, and what the person typed stays on show to copy from.
  let mine = $state<string[]>([])
  let editing = $state(false), saving = $state(false), rows = $state<string[]>([]), rowIds = $state<(string | undefined)[]>([]), editButton = $state<HTMLButtonElement>()
  function startEdit() { if (!review) return; clear(); mine = []; rows = review.criteria.map(item => item.text); rowIds = review.criteria.map(item => item.id); editing = true }
  const editBlocker = $derived(criteriaBlocker(rows))
  async function saveCriteria() {
    const shown = review
    if (saving || !shown || editBlocker || archived) return
    const body = rows.flatMap((text, at) => { const clean = cleanCriteria([text])[0]; return clean ? [{ ...(rowIds[at] ? { id: rowIds[at] } : {}), text: clean }] : [] })
    saving = true; clear()
    try {
      review = await setGafferDeliveryCriteria(record, piece.key, shown.criteriaRevision, body)
      editing = false; mine = []; notice = review.criteria.length ? 'Criteria saved. Evidence for a criterion whose words changed counts again only once it is added afresh.' : 'Criteria saved. This pipeline has none now.'
      await onchanged?.()
      await tick(); (editButton ?? heading)?.focus({ preventScroll: true })
    } catch (e) {
      const typed = rows.map(text => text.trim()).filter(Boolean)
      if (await afterRefusal(e, null, 'The criteria were not saved.') && review) { mine = typed; rows = review.criteria.map(item => item.text); rowIds = review.criteria.map(item => item.id) }
    }
    finally { saving = false; settle() }
  }

  let heading = $state<HTMLElement>(), copied = $state('')
  async function copyCommit() {
    if (!review?.commit) return
    try { await navigator.clipboard.writeText(review.commit); copied = 'Copied.' } catch { copied = 'Could not copy. Select the commit and copy it.' }
    setTimeout(() => { copied = '' }, 3000)
  }
  const link = (url?: string) => safeInsightURL(url)
  const evidenceWho = (item: DeliveryEvidence) => [item.byName ? (item.kind === 'check' ? `added by ${item.byName}` : item.byName) : '', item.at ? when(item.at) : ''].filter(Boolean).join(' · ')
  const verdict = (item: DeliveryEvidence) => item.verdict === 'passed' ? 'Passed' : item.verdict === 'needs_work' ? 'Needs work' : ''
  const pullURL = $derived(link(review?.pull?.url))
  const recorded = $derived([...(review?.decisions ?? [])].reverse())
</script>

<section class="delivery" id={anchor} aria-labelledby={`${uid}-title`}>
  <header class="top">
    <h3 id={`${uid}-title`} tabindex="-1" bind:this={heading}>Delivery review</h3>
    {#if review}<span class="state" data-open={review.open}>{review.open ? 'Waiting for your decision' : review.accepted ? 'Accepted' : 'No decision waiting'}</span>{/if}
  </header>

  {#if !wanted}
    <p class="hint">{agreed}. Reading the review asks the pipeline’s machine for its folder’s commit.</p>
    <button class="btn" type="button" aria-expanded="false" onclick={() => { asked = true }}>Show the delivery review</button>
  {:else if !review && reading}
    <p role="status" class="loading"><span class="pulse" aria-hidden="true"></span>Reading the pipeline’s folder on its machine. This can take up to 15 seconds. Nothing is decided by reading.</p>
  {:else if !review}
    <p role="alert" class="warning">The delivery review could not be read{readError ? `: ${readError}` : '.'}</p>
    <div class="actions"><button class="btn" type="button" onclick={() => void load()}>Retry</button></div>
  {:else}
    {#if changedAfterAccept(piece.note) && piece.state === 'delivery_review'}<p class="conflict" role="note">{piece.note} Your acceptance and any merge approval you gave were withdrawn.</p>{/if}
    <p class="lead">Accepting the delivery says the work meets the criteria below. It does <strong>not</strong> approve the GitHub merge: {next === 'merge' ? 'this pipeline merges by itself once its checks pass, as you set it to' : next === 'done' ? 'this pipeline has no merge stage' : 'Approve the merge stays its own action'}.</p>

    <dl class="facts" aria-label="What was reviewed">
      <div class="wide"><dt>Reviewed commit</dt><dd>
        {#if review.commit}<code title={review.commit}>{shortCommit(review.commit)}</code>
          <button class="inline" type="button" aria-label={`Copy the full commit ID ${review.commit}`} onclick={() => void copyCommit()}>Copy</button><span role="status" class="copied">{copied}</span>
        {:else}<span>{noCommitWords(review.commitState)}</span>{/if}
        <span class="sub" class:bad={review.commitState !== 'verified' && review.commitState !== 'none'}>{commitStateWord(review.commitState)}{#if review.commitNote && review.commitState !== 'verified'}: {review.commitNote}{/if}</span>
        <!-- The machine coming back online, or a new commit, changes nothing the page is told about: the person asks again. -->
        <button class="inline" type="button" disabled={reading || deciding} onclick={() => void load()}>{reading ? 'Reading…' : 'Look again'}</button>
        {#if readError}<span class="sub bad" role="alert">The review could not be read again: {readError}</span>{/if}
      </dd></div>
      {#if review.branch}<div><dt>Branch</dt><dd><code>{review.branch}</code></dd></div>{/if}
      {#if review.remote}<div><dt>Repository</dt><dd>{review.remote}</dd></div>{/if}
      {#if review.pull}<div class="wide"><dt>Pull request</dt><dd>
        {#if pullURL}<a href={pullURL} target="_blank" rel="noopener noreferrer">#{review.pull.number}</a>{:else}#{review.pull.number}{/if} · {review.pull.state.toLowerCase()}
        {#if review.pull.atHead === false}<span class="sub warn" role="note">Its head{review.pull.headCommit ? ` (${shortCommit(review.pull.headCommit)})` : ''} is not the reviewed commit{review.commit ? ` (${shortCommit(review.commit)})` : ''}, so its checks do not count until they do.</span>{/if}
      </dd></div>{/if}
    </dl>
    {#if blocker && review.open}<p class="warning" role="note">{blocker}</p>{/if}

    {#if reading}<p role="status" class="loading"><span class="pulse" aria-hidden="true"></span>Reading the review again…</p>{/if}
    {#if conflict}<p class="conflict" role="alert">{conflict}</p>{/if}
    {#if problem}<p class="warning" role="alert">{problem}</p>{/if}
    {#if notice}<p class="done" role="status">{notice}</p>{/if}

    <div class="listhead"><h4>Criteria</h4><p class="hint">{count ? summaryLine(count) : ''}</p></div>
    {#if review.criteria.length}
      <ol class="criteria" aria-label="Acceptance criteria">
        {#each review.criteria as item, index (item.id)}
          <li class="criterion" data-status={item.status}>
            <div class="said"><span class="mark" data-status={item.status}><span class="glyph" aria-hidden="true">{STATUS_MARK[item.status]}</span><span class="word">{STATUS_WORD[item.status]}</span></span>
              <p class="text"><span class="sr-only">Criterion {index + 1}: </span>{item.text}</p></div>
            {#if item.evidence?.length}
              <ul class="evidence" aria-label={`Evidence for criterion ${index + 1}`}>
                {#each item.evidence as ev (ev.id)}{@const earlier = earlierLabel(ev, review)}{@const url = link(ev.url)}
                  <li class:historical={ev.historical} class:unverified={ev.unverified}>
                    <p class="ev-head"><strong>{KIND_WORD[ev.kind] ?? ev.kind}</strong>
                      {#if ev.kind === 'check'}<span>{ev.name}</span>{#if ev.state}<span class="tone" data-tone={checkTone(ev.state)}>{checkLabel(ev.state)}</span>{/if}
                      {:else if verdict(ev)}<span class="verdict" data-verdict={ev.verdict}>{verdict(ev)}</span>{/if}
                      {#if earlier}<span class="tag">{earlier}</span>{/if}{#if ev.unverified}<span class="tag">Could not be verified</span>{/if}</p>
                    {#if ev.text}<p class="ev-text">{ev.text}</p>{/if}
                    {#if url}<p class="ev-link"><a href={url} target="_blank" rel="noopener noreferrer">{ev.kind === 'artefact' && ev.name ? ev.name : ev.kind === 'check' ? 'Open the check' : 'Open the link'}</a></p>{/if}
                    {#if evidenceWho(ev) || ev.commit}<p class="ev-who">{evidenceWho(ev)}{#if ev.commit} · on <code title={ev.commit}>{shortCommit(ev.commit)}</code>{/if}</p>{/if}
                  </li>
                {/each}
              </ul>
            {:else}<p class="hint none">No evidence yet.</p>{/if}

            {#if decides}
              {#if draft?.criterion === item.id}{@const d = draft}
                <form class="add" aria-label={`Add evidence for criterion ${index + 1}`} onsubmit={event => { event.preventDefault(); void addEvidence() }}>
                  <label>What kind<select bind:value={d.kind} disabled={addingBusy || kinds.length < 2}><option value="note">Reviewer note</option>{#if kinds.includes('check')}<option value="check">GitHub check</option>{/if}{#if kinds.includes('artefact')}<option value="artefact">Artefact link</option>{/if}</select></label>
                  {#if d.kind === 'check'}
                    <label>Check<select bind:value={d.name} disabled={addingBusy || !(review.checks ?? []).length}><option value="">Choose a check</option>{#each review.checks ?? [] as check, at (at)}<option value={check.name}>{check.name} · {checkLabel(check.state)}</option>{/each}</select></label>
                    {#if !(review.checks ?? []).length}<p class="hint">{noChecksWords(review)}</p>{/if}
                  {:else}
                    <label>Verdict<select bind:value={d.verdict} disabled={addingBusy}><option value="passed">Passed</option><option value="needs_work">Needs work</option></select></label>
                    {#if d.kind === 'artefact'}<label>Link inside the repository<input type="url" bind:value={d.url} disabled={addingBusy} placeholder="https://github.com/…"/></label>{/if}
                    <label>{d.kind === 'note' ? 'Note' : 'What it shows (optional)'}<textarea bind:value={d.text} rows="3" disabled={addingBusy}></textarea></label>
                  {/if}
                  <div class="actions"><button class="btn primary" type="submit" disabled={addingBusy || !!evidenceBlocker(d)}>{addingBusy ? 'Adding…' : 'Add evidence'}</button><button class="btn quiet" type="button" disabled={addingBusy} onclick={() => { draft = null; void tick().then(() => document.getElementById(`${uid}-add-${item.id}`)?.focus()) }}>Cancel</button></div>
                  {#if evidenceBlocker(d)}<p class="hint">{evidenceBlocker(d)}</p>{/if}
                </form>
              {:else}<button class="btn quiet" id={`${uid}-add-${item.id}`} type="button" aria-label={`Add evidence for criterion ${index + 1}`} disabled={deciding || reading} onclick={() => startEvidence(item)}>Add evidence</button>{/if}
            {/if}
          </li>
        {/each}
      </ol>
    {:else}<p class="hint">No criteria are agreed for this pipeline.</p>{/if}

    {#if decides}
      <div class="decide" role="group" aria-label="Your decision on this delivery">
        <div class="buttons">
          <button class="btn primary" type="button" bind:this={acceptButton} aria-expanded={confirming} disabled={deciding || reading || !!blocker || saving || addingBusy} onclick={() => void askToAccept()}>Accept delivery</button>
          <button class="btn" type="button" bind:this={sendToggle} aria-expanded={sending} aria-controls={`${uid}-back`} disabled={deciding || reading || !!blocker || !(review.stages ?? []).length} onclick={() => { clear(); confirming = false; sending = !sending }}>Send back</button>
        </div>
        {#if !(review.stages ?? []).length}<p class="hint">Send back is not offered: this pipeline has no stage before its merge to do again.</p>{/if}
        <p class="hint">{nextWords(next)}</p>
        {#if confirming}{@const ask = acceptConfirm({ ...review, next })}
          <div class="confirm" role="group" aria-labelledby={`${uid}-ask`}>
            <h4 id={`${uid}-ask`}>{ask.title}</h4>
            <p>{ask.body}</p>
            <div class="actions"><button class="btn quiet" type="button" bind:this={cancelAccept} disabled={deciding} onkeydown={escapes} onclick={closeConfirm}>Cancel</button><button class="btn primary" type="button" disabled={deciding || reading} onkeydown={escapes} onclick={() => void decide('accept')}>{deciding ? 'Accepting…' : ask.confirmLabel}</button></div>
          </div>
        {/if}
        {#if sending}
          <form class="back" id={`${uid}-back`} aria-label="Send this delivery back" onsubmit={event => { event.preventDefault(); if (!sendBlocker) void decide('send_back') }}>
            <label>Stage to do again<select bind:value={stage} disabled={deciding}>{#each review.stages ?? [] as kind (kind)}<option value={kind}>{stageLabel(kind)}</option>{/each}</select></label>
            <label>Notes for the agent<textarea bind:value={notes} rows="4" disabled={deciding} placeholder="The export leaks other tenants’ rows."></textarea></label>
            <p class="hint">The agent is also told which criteria need work or are unverified. That stage and every stage after it is done again in a fresh session, in the same folder and branch.</p>
            {#if sendBlocker}<p class="hint" role="status">{sendBlocker}</p>{/if}
            <div class="actions"><button class="btn primary" type="submit" disabled={deciding || reading || !!sendBlocker}>{deciding ? 'Sending back…' : 'Send back'}</button><button class="btn quiet" type="button" disabled={deciding} onclick={() => { sending = false; void tick().then(() => sendToggle?.focus()) }}>Cancel</button></div>
          </form>
        {/if}
      </div>
    {:else if review.open === false}
      <div class="closed">
        {#if review.accepted}<p>{review.acceptedCurrent ? `The delivery was accepted on the current commit${review.commit ? ` (${shortCommit(review.commit)})` : ''}.` : `The delivery was accepted on ${shortCommit(review.acceptedCommit) || 'an earlier commit'}, which is not the current commit${review.commit ? ` (${shortCommit(review.commit)})` : ''}.`}</p>
          {#if piece.state === 'ready_to_merge'}<p class="hint">Accepting the delivery did not approve the merge. This pipeline waits at Ready to merge: Approve the merge is its own action on this page.</p>{/if}
        {:else}<p class="hint">This pipeline is not waiting for a delivery decision now.</p>{/if}
      </div>
    {/if}

    {#if recorded.length}
      <div class="listhead"><h4>Decisions</h4></div>
      <ol class="decisions" aria-label="Recorded decisions, newest first">
        {#each recorded as d (`${d.requestId ?? ''}${d.at}`)}
          <li><p><strong>{decisionLine({ ...d, stage: d.stage ? stageLabel(d.stage) : '' })}</strong>{#if d.at} · <time datetime={new Date(d.at).toISOString()}>{when(d.at)}</time>{/if}</p>
            {#if d.notes}<p class="ev-text">{d.notes}</p>{/if}
            {#if notPassedThen(d)}<p class="hint">Not passed then: {notPassedThen(d) === 1 ? '1 criterion' : `${notPassedThen(d)} criteria`}.</p>{/if}</li>
        {/each}
      </ol>
    {/if}

    {#if edits}
      {#if editing}
        {#if mine.length}<div class="conflict" role="note"><p>What you typed, to copy from:</p><ul class="mine">{#each mine as text, at (at)}<li>{text}</li>{/each}</ul></div>{/if}
        <form class="edit" aria-label="Edit the acceptance criteria" onsubmit={event => { event.preventDefault(); void saveCriteria() }}>
          <CriteriaEditor bind:items={rows} bind:ids={rowIds} who={piece.title} disabled={saving} explain={false}/>
          <p class="hint">A criterion whose words you change keeps its place, but its evidence is then from earlier criteria and has to be added again.{#if piece.state === 'ready_to_merge'} Changing the words withdraws your acceptance and the merge approval.{/if}</p>
          {#if editBlocker}<p class="warning" role="status">{editBlocker}</p>{/if}
          <div class="actions"><button class="btn primary" type="submit" disabled={saving || !!editBlocker}>{saving ? 'Saving…' : 'Save criteria'}</button><button class="btn quiet" type="button" disabled={saving} onclick={() => { editing = false; void tick().then(() => editButton?.focus()) }}>Cancel</button></div>
        </form>
      {:else}{#if mine.length}<div class="conflict" role="note"><p>What you typed, to copy from:</p><ul class="mine">{#each mine as text, at (at)}<li>{text}</li>{/each}</ul></div>{/if}<div class="actions"><button class="btn quiet" type="button" bind:this={editButton} disabled={deciding || reading} onclick={startEdit}>Edit criteria</button></div>{/if}
    {/if}
  {/if}
</section>


<style>
  .delivery{display:grid;gap:12px;padding:14px 16px;border:1px solid var(--accent);border-radius:10px;background:var(--card);min-width:0;font-size:14px;line-height:1.5}
  p{margin:0;overflow-wrap:anywhere}
  .hint{color:var(--muted);font-size:0.875rem}.warning{color:var(--bad)}
  .top{display:flex;flex-wrap:wrap;align-items:center;gap:4px 12px}
  h3{margin:0;font:600 1.1rem var(--serif)}h3:focus{outline:none}h3:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  h4{margin:0;font-size:0.95rem}
  .state{padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:600}
  .state[data-open=true]{border-color:var(--accent);color:var(--accent-text)}
  .lead{padding:8px 12px;border-left:2px solid var(--accent);background:var(--tint)}
  .loading{display:flex;align-items:center;gap:10px;color:var(--muted)}
  .pulse{flex:none;width:10px;height:10px;border-radius:50%;background:var(--accent)}
  @media (prefers-reduced-motion:no-preference){.pulse{animation:beat 1.2s ease-in-out infinite}@keyframes beat{50%{opacity:.25}}}
  .facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,200px),1fr));gap:8px 24px;margin:0}
  .facts>div{display:grid;gap:0;min-width:0}.facts .wide{grid-column:1/-1}
  dt{font-size:0.875rem;color:var(--muted)}dd{margin:0;overflow-wrap:anywhere;min-width:0}
  code{font:0.875rem var(--mono);overflow-wrap:anywhere}
  .sub{display:block;font-size:0.875rem;color:var(--muted)}.sub.bad{color:var(--bad);font-weight:600}.sub.warn{color:var(--warn);font-weight:600}
  .copied{margin-left:6px;font-size:0.875rem;color:var(--muted)}
  .inline{min-height:44px;padding:0 8px;border:0;background:none;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer}
  a{color:var(--accent-text);font-weight:600}
  .conflict{padding:8px 12px;border:1px solid var(--warn);border-radius:8px;color:var(--ink);background:var(--tint)}
  .done{color:var(--good);font-weight:600}
  .listhead{display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 12px}
  .criteria,.evidence,.decisions{list-style:none;margin:0;padding:0;display:grid;gap:10px;min-width:0}
  .criterion{display:grid;gap:8px;padding:10px 12px;border:1px solid var(--line);border-radius:10px;min-width:0}
  .criterion[data-status=needs_work]{border-color:var(--bad)}
  .said{display:flex;flex-wrap:wrap;align-items:flex-start;gap:6px 12px;min-width:0}
  .mark{display:inline-flex;align-items:center;gap:6px;flex:none;padding:2px 10px 2px 4px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:700;white-space:nowrap}
  .glyph{display:inline-grid;place-items:center;width:22px;height:22px;border-radius:50%;font-size:0.875rem;line-height:1;background:var(--line-soft)}
  .mark[data-status=passed]{border-color:var(--good);color:var(--good)}.mark[data-status=passed] .glyph{background:var(--good);color:var(--bg)}
  .mark[data-status=needs_work]{border-color:var(--bad);color:var(--bad)}.mark[data-status=needs_work] .glyph{border-radius:3px;background:var(--bad);color:var(--bg)}
  .mark[data-status=unverified]{border-color:var(--warn);color:var(--warn);border-style:dashed}.mark[data-status=unverified] .glyph{background:none;border:1.5px dashed var(--warn)}
  .text{flex:1 1 220px;min-width:0;font-weight:600}
  .evidence{padding-left:12px;border-left:2px solid var(--line-strong);gap:8px}
  .evidence li{display:grid;gap:2px;min-width:0}
  .evidence li.historical{opacity:.7;border-left:2px dashed var(--line-strong);padding-left:8px;margin-left:-10px}
  .ev-head{display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 8px}
  .verdict{font-weight:600}.verdict[data-verdict=passed]{color:var(--good)}.verdict[data-verdict=needs_work]{color:var(--bad)}
  .tone{font-weight:600;color:var(--muted)}.tone[data-tone=good]{color:var(--good)}.tone[data-tone=warn]{color:var(--warn)}.tone[data-tone=bad]{color:var(--bad)}
  .tag{padding:0 8px;border:1px solid var(--line-strong);border-radius:999px;font-size:0.875rem;color:var(--muted)}
  li.unverified .tag{border-color:var(--warn);color:var(--warn)}
  .ev-text{white-space:pre-wrap}.ev-who{font-size:0.875rem;color:var(--muted)}
  .none{padding-left:2px}
  .add,.back,.edit,.confirm{display:grid;gap:10px;padding:10px 12px;border:1px solid var(--line);border-radius:10px;background:var(--bg);min-width:0}
  label{font-weight:600;display:grid;gap:4px;min-width:0}
  textarea,select,input{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .decide{display:grid;gap:8px;padding:12px;border:1px solid var(--line-strong);border-radius:10px}
  .buttons,.actions{display:flex;flex-wrap:wrap;gap:8px}
  .btn{min-height:44px}
  .mine{margin:4px 0 0;padding-left:18px;overflow-wrap:anywhere}
  .decisions li{padding:8px 12px;border-left:2px solid var(--line-strong);display:grid;gap:2px}
  .closed{display:grid;gap:4px}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  button:focus-visible,a:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media (max-width:559px){.buttons .btn{flex:1 1 100%}.delivery{padding:12px}}
</style>
