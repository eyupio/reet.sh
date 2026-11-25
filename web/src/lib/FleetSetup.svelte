<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import Icon from './Icon.svelte'
  import { onMount, tick } from 'svelte'
  import { shell } from './dialogShell'
  import { formatWhen } from './time'
  import {
    activeFleetSetupBatch,
    createFleetSetupBatch,
    fleetSetupBatch,
    readFleetSetup,
    steerFleetSetupBatch,
    terminalMachines,
    type FleetSetupBatch,
    type FleetSetupMachine,
    type FleetSetupMember,
    type FleetSetupReview,
    type TerminalMachine,
  } from './terminal/api'
  import {
    anyWidens,
    changingFiles,
    confirmToSend,
    confirmed,
    copyButton,
    diffCounts,
    fleetFileLine,
    machineExcluded,
    machineStateLabel,
    readyMachines,
    setupBatchCanAbandon,
    setupBatchSummary,
    setupErrorMessage,
    setupItemName,
    setupMemberCanUndo,
    setupMemberLabel,
    setupMemberNote,
    sizeLabel,
    widenWarning,
    NEVER_COPIED,
    KEPT_ONLY_WHILE_COPYING,
    fileChip,
    widenEntries,
    confirmLabel,
  } from './fleetSetup'

  let { onclose, timeZone = '', dateFormat = 'locale' }: { onclose: () => void; timeZone?: string; dateFormat?: string } = $props()
  const id = $props.id()
  const LAST = 'reet-fleet-setup-last'
  let dialog = $state<HTMLDialogElement>()
  let confirmDialog = $state<HTMLDialogElement>()
  let undoDialog = $state<HTMLDialogElement>()
  let sourceSelect = $state<HTMLSelectElement>()
  let wordInput = $state<HTMLInputElement>()
  let headline = $state<HTMLElement>()
  let loading = $state(true)
  let error = $state('')
  let machines = $state<TerminalMachine[]>([])
  let source = $state('')
  let review = $state<FleetSetupReview | null>(null)
  let reading = $state(false)
  let readError = $state('')
  let selection = $state<string[]>([])
  let batch = $state<FleetSetupBatch | null>(null)
  let busy = $state(false)
  let batchError = $state('')
  let typedWord = $state('')
  let confirming = $state<string[]>([])
  let undoing = $state<FleetSetupMember | null>(null)
  let undoError = $state('')
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let focused = false

  const when = (at: number) => formatWhen(at, timeZone, dateFormat)
  const byId = (agentId: string) => review?.machines.find((machine) => machine.agentId === agentId)
  const chosen = $derived(selection.map(byId).filter((machine): machine is FleetSetupMachine => !!machine))
  const confirmMachines = $derived(confirming.map(byId).filter((machine): machine is FleetSetupMachine => !!machine))
  const needsWord = $derived(anyWidens(confirmMachines))
  const ready = $derived(readyMachines(review?.machines ?? []))
  const batchActive = $derived(!!batch && batch.state !== 'done' && batch.state !== 'abandoned')
  const summary = $derived(batch ? setupBatchSummary(batch) : null)
  const widening = $derived(confirmMachines.filter((machine) => machine.widens))

  // A button that did its work is gone or stale, so keyboard focus goes to the sentence that says what happened.
  async function focusHeadline() {
    await tick()
    headline?.focus()
  }

  function remember(next: FleetSetupBatch | null) {
    try { next ? localStorage.setItem(LAST, next.id) : localStorage.removeItem(LAST) } catch { /* the last copy just isn't remembered */ }
  }

  async function load() {
    loading = true
    error = ''
    try {
      machines = await terminalMachines()
      if (!machines.some((machine) => machine.agentId === source)) source = (machines.find((machine) => machine.ready) ?? machines[0])?.agentId ?? ''
      // The dialog unmounts when closed; a copy keeps running without it. A finished one stays reachable so it can be undone.
      if (!batch) {
        try { batch = await activeFleetSetupBatch() } catch { /* the progress panel just stays hidden */ }
        if (!batch) {
          let last = ''
          try { last = localStorage.getItem(LAST) ?? '' } catch { /* nothing remembered */ }
          if (last) try { batch = await fleetSetupBatch(last) } catch { remember(null) }
        }
        queuePoll()
      }
    } catch (caught) {
      error = setupErrorMessage(caught, "Couldn't read your machines.")
    } finally {
      loading = false
    }
    if (!focused) {
      focused = true
      await tick()
      if (!batch) sourceSelect?.focus()
    }
  }

  async function read() {
    if (reading || !source) return
    reading = true
    readError = ''
    try {
      review = await readFleetSetup(source)
      selection = readyMachines(review.machines).map((machine) => machine.agentId)
    } catch (caught) {
      review = null
      selection = []
      readError = setupErrorMessage(caught, "Couldn't read that machine's setup.")
    } finally {
      reading = false
    }
  }

  function toggle(agentId: string) {
    selection = selection.includes(agentId) ? selection.filter((other) => other !== agentId) : [...selection, agentId]
  }

  function move(index: number, delta: number) {
    const to = index + delta
    if (to < 0 || to >= selection.length) return
    const next = [...selection]
    ;[next[index], next[to]] = [next[to], next[index]]
    selection = next
  }

  async function ask() {
    if (!selection.length) return
    batchError = ''
    typedWord = ''
    confirming = [...selection]
    confirmDialog?.showModal()
    // The typed word is the one thing to do first when it is needed; otherwise the dialog opens on Cancel.
    if (anyWidens(chosen)) { await tick(); wordInput?.focus() }
  }

  async function start() {
    if (busy || !review || !confirming.length || !confirmed(typedWord, needsWord)) return
    busy = true
    batchError = ''
    try {
      batch = await createFleetSetupBatch(crypto.randomUUID(), review.readId, confirming, confirmToSend(typedWord, needsWord))
      remember(batch)
      review = null
      selection = []
      confirmDialog?.close()
      queuePoll()
      void focusHeadline()
    } catch (caught) {
      batchError = setupErrorMessage(caught, 'That copy did not start.')
      if ((caught as { code?: string })?.code === 'setup_read_expired') {
        // The review is gone, so the question has nothing left to ask.
        readError = batchError
        review = null
        selection = []
        confirmDialog?.close()
      }
    } finally {
      busy = false
    }
  }

  function queuePoll() {
    clearTimeout(pollTimer)
    if (!batch || (batch.state !== 'pending' && batch.state !== 'running')) return
    pollTimer = setTimeout(() => { if (document.visibilityState === 'visible') void poll() }, 3000)
  }

  async function poll() {
    if (!batch) return
    const batchId = batch.id
    try {
      const next = await fleetSetupBatch(batchId)
      if (batch?.id !== batchId) return
      batch = next
    } catch (caught) {
      batchError = setupErrorMessage(caught, "Couldn't read the copy.")
    }
    queuePoll()
  }

  // Retry, skip and abandon share one shape: ask, take the batch the controller answers, and keep following it.
  async function steer(step: 'abandon' | `members/${number}/${'retry' | 'skip'}`, fallback: string) {
    if (!batch || busy) return
    busy = true
    batchError = ''
    const batchId = batch.id
    try {
      const next = await steerFleetSetupBatch(batchId, step)
      if (batch?.id === batchId) { batch = next; queuePoll() }
      void focusHeadline()
    } catch (caught) {
      batchError = setupErrorMessage(caught, fallback)
    } finally {
      busy = false
    }
  }

  function askUndo(member: FleetSetupMember) {
    undoError = ''
    undoing = member
    undoDialog?.showModal()
  }

  async function undo() {
    if (!batch || !undoing || busy) return
    busy = true
    undoError = ''
    const batchId = batch.id
    try {
      const next = await steerFleetSetupBatch(batchId, `members/${undoing.position}/undo`)
      if (batch?.id === batchId) batch = next
      undoDialog?.close()
      void focusHeadline()
    } catch (caught) {
      undoError = setupErrorMessage(caught, "That copy couldn't be undone.")
      const code = (caught as { code?: string })?.code
      // Someone undid it already: show the batch as it is now.
      if (code === 'setup_undo_done' || code === 'setup_undo_unavailable') try { batch = await fleetSetupBatch(batchId) } catch { /* the message stands */ }
    } finally {
      busy = false
    }
  }

  function another() {
    remember(null)
    batch = null
    batchError = ''
    review = null
    void load().then(() => sourceSelect?.focus())
  }

  function wake() {
    if (document.visibilityState === 'visible' && batch && (batch.state === 'pending' || batch.state === 'running')) void poll()
  }

  onMount(() => {
    dialog?.showModal()
    void load()
    document.addEventListener('visibilitychange', wake)
    return () => {
      clearTimeout(pollTimer)
      document.removeEventListener('visibilitychange', wake)
      if (dialog?.open) dialog.close()
    }
  })
</script>

{#snippet fileList(files: FleetSetupMachine['files'])}
  <ul class="files">
    {#each files as file (file.agent + file.kind + file.name)}
      {@const lines = (file.added?.length ?? 0) + (file.removed?.length ?? 0)}
      <li data-file-state={file.state}>
        <span class="chip">{fileChip(file)}</span>
        <div class="copy">
          <span>{fleetFileLine(file)}</span>
          {#if lines}
            <details class="diff" open={lines <= 6}>
              <summary>Changed lines · {diffCounts(file)}</summary>
              {#if file.removed?.length}
                <p class="diff-title">Removed</p>
                <ul class="del">{#each file.removed as line, index (index)}<li><span aria-hidden="true">−</span> {line || '(blank line)'}</li>{/each}</ul>
              {/if}
              {#if file.added?.length}
                <p class="diff-title">Added</p>
                <ul class="add">{#each file.added as line, index (index)}<li><span aria-hidden="true">+</span> {line || '(blank line)'}</li>{/each}</ul>
              {/if}
              {#if file.truncated}<p class="hint">More changed lines are not shown.</p>{/if}
            </details>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
{/snippet}

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby={`${id}-title`}>
  <div class="sheet" data-shell-body>
    <div class="stack">
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Setups</p>
        <h2 id={`${id}-title`}>Copy a setup to other machines</h2>
        <p class="lede" data-shell-aside>Copy one machine's agent instructions and settings to your others, once you have reviewed what each would change. Every copy can be undone.</p>
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close copy a setup" data-shell-close><Icon name="x" size={18} /></button>
    </header>

    {#if loading}
      <Skeleton label="Reading your machines…" shape="rows" count={3} />
    {:else if error}
      <p class="error" role="alert">{error}</p>
      <button class="btn" type="button" onclick={() => void load()}>Try again</button>
    {:else}
      {#if batch && summary}
        <div class="overview" data-state={summary.state}>
          <span class="overview-dot" aria-hidden="true"></span>
          <div class="overview-text">
            <strong role="status" aria-live="polite" tabindex="-1" bind:this={headline}>{summary.headline}</strong>
            {#if summary.detail}<span>{summary.detail}</span>{/if}
            <span>From {batch.sourceHost || 'the source machine'}, read {when(batch.readAt)}.</span>
          </div>
          {#if !batchActive}<button class="btn" type="button" onclick={another}>Start another copy</button>{/if}
        </div>

        <section class="batch" aria-label="Copy progress">
          <ol class="members">
            {#each batch.members as member (member.position)}
              {@const note = setupMemberNote(member)}
              <li data-state={member.state} data-undone={member.undoneAt ? '' : undefined} data-machine={member.host}>
                <span class="pos">{member.position + 1}</span>
                <div class="copy">
                  <strong>{member.host || member.agentId}</strong>
                  <small class="label">{#if member.state === 'applied' && !member.undoneAt}<span class="mark" aria-hidden="true">✓</span>{:else if member.state === 'failed'}<span class="mark" aria-hidden="true">✕</span>{:else if member.undoneAt}<span class="mark" aria-hidden="true">✓</span>{/if}{setupMemberLabel(member)}</small>
                  {#if member.undoneAt}<small>Put back as it was.</small>{/if}
                  {#if note}<small class:warn={member.state === 'failed'}>{note}</small>{/if}
                  {#if member.state !== 'excluded' && !member.undoneAt && changingFiles(member.files).length}
                    <small>{changingFiles(member.files).map((file) => fleetFileLine(file)).join(' ')}</small>
                  {/if}
                </div>
                {#if batch.state === 'stopped' && member.state === 'failed'}
                  <div class="actions">
                    <button class="btn" type="button" disabled={busy} onclick={() => void steer(`members/${member.position}/retry`, "That machine couldn't be retried.")}>Retry</button>
                    <button class="btn" type="button" disabled={busy} onclick={() => void steer(`members/${member.position}/skip`, "That machine couldn't be skipped.")}>Skip and continue</button>
                  </div>
                {:else if setupMemberCanUndo(member)}
                  <div class="actions">
                    <button class="btn" type="button" disabled={busy} aria-label={`Undo copy on ${member.host || member.agentId}`} onclick={() => askUndo(member)}>Undo copy</button>
                  </div>
                {/if}
              </li>
            {/each}
          </ol>
          {#if batch.state === 'stopped'}
            <div class="foot end">
              <button class="btn" type="button" disabled={busy || !setupBatchCanAbandon(batch)} onclick={() => void steer('abandon', "That copy couldn't be abandoned.")}>Abandon copy</button>
            </div>
          {/if}
          {#if batchError}<p class="error" role="alert">{batchError}</p>{/if}
        </section>
      {:else}
        <section class="source" aria-label="Source machine">
          {#if machines.length < 2}
            <p class="hint">Connect at least two machines to copy a setup between them.</p>
          {:else}
            <div class="source-row">
              <div class="field"><label for={`${id}-source`}>Copy from</label>
                <select id={`${id}-source`} bind:this={sourceSelect} bind:value={source} disabled={reading} onchange={() => { review = null; selection = []; readError = '' }}>
                  {#each machines as machine (machine.agentId)}<option value={machine.agentId}>{machine.host}{machine.ready ? '' : ' (not ready)'}</option>{/each}
                </select>
              </div>
              <button class="btn" class:primary={!review} type="button" disabled={reading || !source} aria-busy={reading} onclick={() => void read()}>{reading ? 'Reading…' : review ? 'Read again' : 'Read setup'}</button>
            </div>
            {#if !review}<p class="hint">Reads its global instruction files and portable settings for Claude Code, Codex, OpenCode and Pi. Nothing is written yet. {NEVER_COPIED} {KEPT_ONLY_WHILE_COPYING}</p>{/if}
          {/if}
          {#if readError}<p class="error" role="alert">{readError}</p>{/if}
        </section>

        {#if review}
          <section class="review" aria-label="What would be copied">
            <h3>From {review.source.host}, read {when(review.source.readAt)}</h3>
            <ul class="items">
              {#each review.items as item (item.agent + item.kind + item.name)}
                <li><span>{setupItemName(item)}</span><small>{sizeLabel(item.bytes)}</small></li>
              {/each}
            </ul>
            <p class="never">{NEVER_COPIED}</p>
            <p class="hint">Settings are copied field by field and a machine's other settings stay. An instruction file's text is replaced. {KEPT_ONLY_WHILE_COPYING} Read again after {when(review.expiresAt)}.</p>
          </section>

          <section class="targets" aria-label="Other machines">
            <h3>Other machines · {review.machines.length}</h3>
            {#if review.machines.length === 0}
              <p class="hint">There are no other machines to copy to.</p>
            {:else}
              <ul class="rows">
                {#each review.machines as machine (machine.agentId)}
                  <li data-machine={machine.host} data-state={machine.state}>
                    <div class="row-head">
                      {#if machine.state === 'ready'}
                        <input id={`${id}-pick-${machine.agentId}`} type="checkbox" checked={selection.includes(machine.agentId)} onchange={() => toggle(machine.agentId)} aria-describedby={`${id}-state-${machine.agentId}${machine.widens ? ` ${id}-widen-${machine.agentId}` : ''}`} />
                        <label for={`${id}-pick-${machine.agentId}`}>{machine.host}</label>
                      {:else}
                        <strong>{machine.host}</strong>
                      {/if}
                      <small class="state" id={`${id}-state-${machine.agentId}`}>{machineStateLabel(machine)}</small>
                    </div>
                    {#if machine.state === 'ready'}
                      {#if machine.widens}
                        <p class="widen" role="note" id={`${id}-widen-${machine.agentId}`}><span aria-hidden="true">!</span> <span>{widenWarning(machine.host, machine.files)}</span></p>
                      {/if}
                      {@render fileList(machine.files)}
                    {:else}
                      <p class="why">{machineExcluded(machine)}</p>
                      {#if machine.state === 'unchanged' && machine.files.length}{@render fileList(machine.files)}{/if}
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          {#if chosen.length > 1}
            <section class="selection" aria-label="Copy order">
              <div class="head">
                <strong>Copy order</strong>
                <small class="hint">One machine at a time, in this order. reet stops at the first failure.</small>
              </div>
              <ol class="ordered">
                {#each chosen as machine, index (machine.agentId)}
                  <li>
                    <span class="pos">{index + 1}</span>
                    <div class="copy"><strong>{machine.host}</strong>{#if machine.widens}<small class="warn"><span aria-hidden="true">!</span> Lets agents do more without asking</small>{/if}</div>
                    <div class="order-actions">
                      <button class="icon small" type="button" aria-label={`Move ${machine.host} earlier`} disabled={index === 0} onclick={() => move(index, -1)}>↑</button>
                      <button class="icon small" type="button" aria-label={`Move ${machine.host} later`} disabled={index === chosen.length - 1} onclick={() => move(index, 1)}>↓</button>
                      <button class="btn quiet" type="button" onclick={() => toggle(machine.agentId)} aria-label={`Leave out ${machine.host}`}>Leave out</button>
                    </div>
                  </li>
                {/each}
              </ol>
            </section>
          {/if}
        {/if}
      {/if}
    {/if}
    </div>
    {#if review && !batch && !loading && !error && ready.length > 0}
      <div class="bar" data-shell-foot>
        <p class="hint">{chosen.length === 0 ? 'Tick at least one machine above.' : "A machine that isn't connected when its turn comes is skipped."}</p>
        {#if chosen.length > 0}<button class="btn primary" type="button" onclick={() => void ask()}>{copyButton(chosen.length)}</button>{/if}
      </div>
    {/if}
  </div>
</dialog>

<dialog bind:this={confirmDialog} class="confirm" aria-labelledby={`${id}-confirm`} onclose={() => { batchError = ''; confirming = []; typedWord = '' }}>
  <div class="confirm-body">
    <h2 id={`${id}-confirm`}>{copyButton(confirming.length)}?</h2>
    {#if review}
      <p>reet will copy the setup it read from {review.source.host} ({when(review.source.readAt)}) to {confirming.length === 1 ? 'this machine' : 'these machines'}, in this order, one at a time. A machine that isn't connected is skipped. reet stops at the first failure.</p>
    {/if}
    {#if widening.length}
      <div class="widen block" role="note" id={`${id}-widen`}>
        <p><span aria-hidden="true">!</span> <strong>This copy lets agents do more without asking.</strong></p>
        <ul>
          {#each widening as machine (machine.agentId)}
            <li>
              <strong>{machine.host}</strong>
              {#each widenEntries(machine.files) as entry (entry.file)}
                <span>{entry.file}: {entry.fields.join(', ')}</span>
                {#each entry.added as line, index (index)}<code>+ {line}</code>{/each}
              {/each}
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    <ol class="ordered compact">
      {#each confirmMachines as machine, index (machine.agentId)}
        <li>
          <span class="pos">{index + 1}</span>
          <span class="copy">
            <strong>{machine.host}</strong>
            {#each changingFiles(machine.files) as file (file.agent + file.kind + file.name)}<small>{fleetFileLine(file)}</small>{/each}
          </span>
        </li>
      {/each}
    </ol>
    <p class="never">{NEVER_COPIED}</p>
    <p class="hint">Each machine keeps a rollback copy of every file it changes, so a copy can be undone afterwards. A machine whose files changed since this review refuses and stops the copy. {KEPT_ONLY_WHILE_COPYING}</p>
    {#if needsWord}
      <label class="typed-confirm"><span>{confirmLabel(confirmMachines)}</span>
        <input bind:this={wordInput} aria-describedby={`${id}-widen`} bind:value={typedWord} autocomplete="off" autocapitalize="off" spellcheck="false" onkeydown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void start() } }} />
      </label>
    {/if}
    {#if batchError}<p class="error" role="alert">{batchError}</p>{/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => confirmDialog?.close()} disabled={busy}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void start()} disabled={busy || !confirming.length || !confirmed(typedWord, needsWord)}>{busy ? 'Starting…' : copyButton(confirming.length)}</button>
    </div>
  </div>
</dialog>

<dialog bind:this={undoDialog} class="confirm" aria-labelledby={`${id}-undo`} onclose={() => { undoing = null; undoError = '' }}>
  <div class="confirm-body">
    <h2 id={`${id}-undo`}>Undo the copy on {undoing?.host}?</h2>
    <p>reet puts back the files it changed on {undoing?.host} from the rollback copy that machine kept. If a file has changed since the copy, reet leaves everything as it is and says so. A copy can be undone once.</p>
    {#if undoError}<p class="error" role="alert">{undoError}</p>{/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => undoDialog?.close()} disabled={busy}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void undo()} disabled={busy || !undoing}>{busy ? 'Undoing…' : 'Undo copy'}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { width: min(720px, calc(100vw - 32px)); max-height: min(88vh, 820px); padding: 0; border: 1px solid var(--line); border-radius: 16px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px color-mix(in srgb, var(--ink) 28%, transparent); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); backdrop-filter: blur(2px); }
  .stack { display: grid; gap: 18px; }
  .sheet { --pad: clamp(20px, 4vw, 30px); padding: var(--pad); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.875rem; font-weight: 600; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; }
  h3 { margin: 0; font-size: 0.95rem; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.875rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover:not(:disabled) { color: var(--ink); border-color: var(--muted); }
  .icon:disabled { opacity: 0.4; cursor: default; }
  .icon.small { width: 32px; height: 32px; font-size: 1rem; }
  .hint { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .error { margin: 0; color: var(--bad); font-size: 0.875rem; }
  section { display: grid; gap: 12px; }
  .source-row { display: flex; align-items: flex-end; gap: 12px; flex-wrap: wrap; }
  .field { display: grid; gap: 6px; flex: 1 1 220px; min-width: 0; font-size: 0.875rem; font-weight: 600; }
  .field select { height: 40px; padding: 0 10px; border: 1px solid var(--line-deep); border-radius: 8px; background: var(--bg); color: var(--ink); font: inherit; font-weight: 400; }
  .source-row .btn { flex: none; min-height: 40px; }
  .items, .rows, .files, .members, .ordered { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .items li { display: flex; justify-content: space-between; gap: 12px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); font-size: 0.875rem; }
  .items small { color: var(--muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .rows > li, .members > li, .ordered > li { display: grid; gap: 8px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .rows > li[data-state='excluded'], .rows > li[data-state='unchanged'] { opacity: 0.85; }
  .row-head { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .row-head input { width: 18px; height: 18px; flex: none; accent-color: var(--accent); cursor: pointer; }
  .row-head label, .row-head strong { flex: 1; min-width: 0; font-size: 0.9rem; font-weight: 600; cursor: pointer; overflow-wrap: anywhere; }
  .row-head strong { cursor: default; }
  .state { flex: none; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .rows > li[data-state='ready'] .state { color: var(--accent-text); }
  .why { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .widen { display: flex; gap: 10px; margin: 0; padding: 10px 12px; border: 1px solid var(--warn); border-radius: 8px; color: var(--ink); font-size: 0.875rem; line-height: 1.5; }
  .widen > span:first-child { flex: none; color: var(--warn); font-weight: 700; }
  .widen.block { display: grid; gap: 6px; padding: 12px 14px; }
  .widen.block p { margin: 0; }
  .widen.block p > span { color: var(--warn); font-weight: 700; }
  .widen.block ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .widen.block li { display: grid; gap: 2px; }
  .widen.block code { font: 0.875rem/1.45 var(--mono, monospace); overflow-wrap: anywhere; white-space: pre-wrap; }
  .never { margin: 0; color: var(--ink); font-size: 0.875rem; line-height: 1.5; font-weight: 600; }
  .confirm p.never { font-size: 0.875rem; font-weight: 600; }
  .bar { position: sticky; bottom: 0; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin: 0 calc(-1 * var(--pad)) calc(-1 * var(--pad)); padding: 12px var(--pad) max(12px, env(safe-area-inset-bottom)); border-top: 1px solid var(--line); background: var(--card); }
  .bar .btn { min-height: 40px; }
  .overview[data-state='undone'] .overview-dot { background: var(--muted); }
  .overview[data-state='undone'] .overview-text strong { color: var(--muted); }
  .overview-text strong:focus-visible { outline-offset: 4px; }
  .files > li { display: flex; align-items: flex-start; gap: 10px; }
  .chip { flex: none; width: 92px; box-sizing: border-box; padding: 2px 8px; border: 1px solid var(--line-deep); border-radius: 6px; color: var(--muted); font-size: 0.875rem; text-align: center; }
  .files > li[data-file-state='create'] .chip, .files > li[data-file-state='merge'] .chip { color: var(--ink); }
  .files > li[data-file-state='blocked'] .chip { border-color: var(--warn); color: var(--warn); }
  .copy { display: grid; gap: 3px; min-width: 0; flex: 1 1 auto; font-size: 0.875rem; line-height: 1.5; overflow-wrap: anywhere; }
  .copy strong { font-size: 0.9rem; }
  .copy small { color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .copy .label { color: var(--ink); font-weight: 600; }
  .copy .warn, .warn { color: var(--warn); }
  .diff { margin-top: 4px; }
  .diff summary { display: inline-flex; align-items: center; min-height: 32px; color: var(--muted); font-size: 0.875rem; font-weight: 600; cursor: pointer; }
  .diff-title { margin: 6px 0 2px; color: var(--muted); font-size: 0.875rem; }
  .diff ul { list-style: none; margin: 0; padding: 8px 10px; display: grid; gap: 2px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); font: 0.875rem/1.45 var(--mono, monospace); white-space: pre-wrap; }
  .diff .add li > span { color: var(--good); }
  .diff .del li > span { color: var(--bad); }
  .selection, .batch { padding: 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--accent) 5%, var(--bg)); }
  .head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .ordered > li, .members > li { display: flex; align-items: flex-start; gap: 12px; }
  .pos { flex: none; width: 22px; height: 22px; display: grid; place-items: center; border-radius: 6px; background: var(--card); border: 1px solid var(--line); color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .members > li[data-state='applied'] .label { color: var(--good); }
  .members > li[data-undone] .label { color: var(--muted); }
  .mark { margin-right: 6px; }
  .members > li[data-state='failed'] { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .members > li[data-state='failed'] .label { color: var(--bad); }
  .members > li[data-state='excluded'], .members > li[data-state='skipped'] { opacity: 0.85; }
  .order-actions, .actions { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; flex: none; }
  .actions .btn, .foot .btn { min-height: 36px; padding: 7px 12px; font-size: 0.875rem; }
  .foot.end { justify-content: flex-end; }
  .foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .overview { display: flex; align-items: center; gap: 16px; padding: 18px 20px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  .overview-dot { flex: none; width: 12px; height: 12px; border-radius: 50%; background: var(--muted); }
  .overview[data-state='working'] .overview-dot { background: var(--accent-text); animation: pulse 1.4s ease-in-out infinite; }
  .overview[data-state='finished'] .overview-dot { background: var(--good); }
  .overview[data-state='stopped'] .overview-dot { background: var(--warn); }
  @keyframes pulse { 50% { opacity: .35; } }
  @media (prefers-reduced-motion: reduce) { .overview[data-state='working'] .overview-dot { animation: none; } }
  .overview-text { flex: 1; min-width: 0; display: grid; gap: 4px; overflow-wrap: anywhere; }
  .overview-text strong { font: 500 clamp(1.15rem, 2.6vw, 1.4rem)/1.25 var(--serif); }
  .overview-text span { color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .overview .btn { flex: none; min-height: 40px; }
  .confirm { width: min(520px, calc(100vw - 32px)); border-radius: 16px; max-height: min(88vh, 820px); }
  .confirm-body { display: grid; gap: 12px; padding: clamp(20px, 4vw, 28px); overflow: auto; max-height: inherit; }
  .confirm h2 { margin: 0; font: 500 clamp(1.3rem, 3vw, 1.7rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  .confirm p { margin: 0; color: var(--ink); font-size: 0.88rem; line-height: 1.55; }
  .confirm .hint { color: var(--muted); font-size: 0.875rem; }
  .confirm .error { color: var(--bad); font-size: 0.875rem; }
  .ordered.compact { gap: 4px; }
  .ordered.compact > li { padding: 8px 10px; align-items: flex-start; }
  .confirm-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
  .confirm-actions .btn { min-height: 40px; padding: 8px 14px; }
  .typed-confirm { display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .typed-confirm input { height: 40px; padding: 0 10px; border: 1px solid var(--line-deep); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--mono, monospace); }
  @media (max-width: 560px) {
    .overview { flex-wrap: wrap; padding: 14px; gap: 12px; }
    .overview-text { flex-basis: calc(100% - 28px); }
    .overview .btn { width: 100%; }
    /* The full-screen sheet scrolls as a whole, so the pinned bar sticks to it rather than to an inner scroller. */
    .sheet { overflow: visible; max-height: none; }
    .source-row .btn { width: 100%; }
    .bar .btn { width: 100%; }
    /* The number stays beside the name; the buttons take their own line below. */
    .members > li, .ordered:not(.compact) > li { display: grid; grid-template-columns: auto 1fr; }
    .order-actions, .actions { grid-column: 1 / -1; width: 100%; }
    .files > li { flex-direction: column; gap: 4px; }
    .chip { width: auto; align-self: flex-start; }
  }
  @media (pointer: coarse) {
    .actions .btn, .foot .btn, .bar .btn, .btn.quiet, .icon, .source-row .btn, .overview .btn, .confirm-actions .btn, .field select, .typed-confirm input { min-height: 44px; }
    .icon, .icon.small { width: 44px; height: 44px; }
    .btn.quiet { min-width: 44px; }
    .row-head label { min-height: 44px; display: flex; align-items: center; }
    .row-head input { width: 22px; height: 22px; }
    .diff summary { min-height: 44px; }
  }
</style>
