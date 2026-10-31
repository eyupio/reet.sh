<script lang="ts">
  import { onMount } from 'svelte'
  import { getSoftwareMirror, type SoftwareMirrorStatus } from './api'
  import { shell } from './dialogShell'
  import {
    abandonSoftwareBatch,
    activeSoftwareBatch,
    applyMachineSoftware,
    createSoftwareBatch,
    fleetSoftwarePlans,
    retrySoftwareBatchMember,
    skipSoftwareBatchMember,
    softwareBatch,
    type FleetSoftwareRow,
    type SoftwareBatch,
  } from './terminal/api'
  import {
    batchCanAbandon,
    batchMemberAction,
    batchMemberLabel,
    batchMemberNote,
    batchStateLabel,
    fleetSoftwareCanApply,
    fleetSoftwareLabel,
    fleetSoftwareWarning,
  } from './software'

  let { onclose }: { onclose: () => void } = $props()
  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let confirmDialog = $state<HTMLDialogElement>()
  let batchDialog = $state<HTMLDialogElement>()
  let rows = $state<FleetSoftwareRow[]>([])
  let loading = $state(true)
  let error = $state('')
  let applying = $state('')
  let applyError = $state<Record<string, string>>({})
  let notice = $state<Record<string, string>>({})
  let confirm = $state<FleetSoftwareRow | null>(null)
  let selection = $state<string[]>([])
  let batch = $state<SoftwareBatch | null>(null)
  let batchBusy = $state(false)
  let batchError = $state('')
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let mirror = $state<SoftwareMirrorStatus | null>(null)

  const selectedRows = $derived(
    selection
      .map((agentId) => rows.find((row) => row.agentId === agentId))
      .filter((row): row is FleetSoftwareRow => !!row),
  )
  const batchActive = $derived(!!batch && batch.state !== 'done' && batch.state !== 'abandoned')

  async function load() {
    loading = true
    error = ''
    try {
      rows = await fleetSoftwarePlans()
      selection = selection.filter((agentId) => rows.some((row) => row.agentId === agentId && fleetSoftwareCanApply(row)))
      try { mirror = await getSoftwareMirror() } catch { mirror = null }
      // The dialog unmounts when closed; a batch keeps running without it.
      if (!batch) {
        try {
          batch = await activeSoftwareBatch()
          queuePoll()
        } catch { /* the batch panel just stays hidden */ }
      }
    } catch (caught) {
      error = caught instanceof Error ? caught.message : "Couldn't read the fleet's software plans."
    } finally {
      loading = false
    }
  }

  function toggleSelection(agentId: string) {
    if (batchActive) return
    selection = selection.includes(agentId) ? selection.filter((id) => id !== agentId) : [...selection, agentId]
  }

  function moveSelection(index: number, delta: number) {
    const to = index + delta
    if (to < 0 || to >= selection.length) return
    const next = [...selection]
    ;[next[index], next[to]] = [next[to], next[index]]
    selection = next
  }

  function openBatchConfirm() {
    if (!selection.length) return
    batchError = ''
    batchDialog?.showModal()
  }

  async function startBatch() {
    if (batchBusy || !selection.length) return
    batchBusy = true
    batchError = ''
    try {
      batch = await createSoftwareBatch(crypto.randomUUID(), 'opencode', selection)
      selection = []
      batchDialog?.close()
      await load()
      queuePoll()
    } catch (caught) {
      batchError = caught instanceof Error ? caught.message : 'That batch did not start.'
    } finally {
      batchBusy = false
    }
  }

  function queuePoll() {
    clearTimeout(pollTimer)
    if (!batch || (batch.state !== 'pending' && batch.state !== 'running')) return
    pollTimer = setTimeout(() => {
      if (document.visibilityState === 'visible') void pollBatch()
    }, 3000)
  }

  async function pollBatch() {
    if (!batch) return
    const batchId = batch.id
    try {
      const next = await softwareBatch(batchId)
      if (!batch || batch.id !== batchId) return
      batch = next
    } catch (caught) {
      batchError = caught instanceof Error ? caught.message : "Couldn't read the batch."
    }
    if (batch && (batch.state === 'pending' || batch.state === 'running')) queuePoll()
    else if (batch && (batch.state === 'done' || batch.state === 'abandoned')) await load()
  }

  async function retryMember(position: number) {
    if (!batch || batchBusy) return
    batchBusy = true
    batchError = ''
    const batchId = batch.id
    try {
      const next = await retrySoftwareBatchMember(batchId, position)
      if (batch?.id === batchId) {
        batch = next
        queuePoll()
      }
    } catch (caught) {
      batchError = caught instanceof Error ? caught.message : "That machine couldn't be retried."
    } finally {
      batchBusy = false
    }
  }

  async function skipMember(position: number) {
    if (!batch || batchBusy) return
    batchBusy = true
    batchError = ''
    const batchId = batch.id
    try {
      const next = await skipSoftwareBatchMember(batchId, position)
      if (batch?.id === batchId) {
        batch = next
        queuePoll()
      }
    } catch (caught) {
      batchError = caught instanceof Error ? caught.message : "That machine couldn't be skipped."
    } finally {
      batchBusy = false
    }
  }

  async function abandon() {
    if (!batch || batchBusy || !batchCanAbandon(batch)) return
    batchBusy = true
    batchError = ''
    try {
      batch = await abandonSoftwareBatch(batch.id)
      await load()
    } catch (caught) {
      batchError = caught instanceof Error ? caught.message : "That batch couldn't be abandoned."
    } finally {
      batchBusy = false
    }
  }

  function clearBatch() {
    batch = null
    batchError = ''
    selection = []
    void load()
  }

  function wake() {
    if (document.visibilityState === 'visible' && batch && (batch.state === 'pending' || batch.state === 'running')) void pollBatch()
  }

  function openApply(row: FleetSoftwareRow) {
    if (!row.action || !fleetSoftwareCanApply(row)) return
    applyError[row.agentId] = ''
    confirm = row
    confirmDialog?.showModal()
  }

  async function applyConfirm() {
    if (!confirm || applying || !fleetSoftwareCanApply(confirm)) return
    const row = confirm
    const action = row.action
    if (!action) return
    applying = row.agentId
    applyError[row.agentId] = ''
    try {
      const answer = await applyMachineSoftware(row.agentId, 'opencode', action)
      notice[row.agentId] = `${answer.result.action === 'install' ? 'Installed' : 'Updated'} OpenCode ${answer.result.version} on ${row.host}.`
      confirmDialog?.close()
      await load()
    } catch (caught) {
      applyError[row.agentId] = caught instanceof Error ? caught.message : "That install didn't finish."
    } finally {
      applying = ''
    }
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

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby={`${id}-title`}>
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Software</p>
        <h2 id={`${id}-title`}>Fleet software</h2>
        <p class="lede" data-shell-aside>A plan for each machine, from stored evidence. Choose machines to apply in one ordered batch, or apply one on its own. Rollback and remove stay in a machine's Settings.</p>
        {#if mirror}<p class="hint mirror-line">Artifact mirror: {mirror.enabled ? `on${mirror.cached ? ', cached' : ', not cached yet'}` : 'off'}. Owners and admins change it in Admin.</p>{/if}
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close fleet software" data-shell-close>×</button>
    </header>

    {#if loading}
      <p role="status">Reading the stored plans…</p>
    {:else if error}
      <p class="error" role="alert">{error}</p>
      <button class="btn" type="button" onclick={() => void load()}>Try again</button>
    {:else}
      {#if batch}
        <section class="batch" aria-live="polite" aria-label="Batch progress">
          <div class="batch-head">
            <div>
              <strong>Batch</strong>
              <small class="label">{batchStateLabel(batch.state)}</small>
            </div>
            {#if batch.state === 'done' || batch.state === 'abandoned'}
              <button class="btn" type="button" onclick={clearBatch}>Start another batch</button>
            {/if}
          </div>
          <ol class="batch-members">
            {#each batch.members as member (member.position)}
              <li data-state={member.state}>
                <span class="pos">{member.position + 1}</span>
                <div class="copy">
                  <strong>{member.host || member.agentId}</strong>
                  {#if batchMemberAction(member)}<small>{batchMemberAction(member)}</small>{/if}
                  <small class="label">{batchMemberLabel(member)}</small>
                  {#if batchMemberNote(member)}<small class="warn">{batchMemberNote(member)}</small>{/if}
                </div>
                {#if batch.state === 'stopped' && member.state === 'failed'}
                  <div class="batch-actions">
                    <button class="btn" type="button" disabled={batchBusy} onclick={() => void retryMember(member.position)}>Retry</button>
                    <button class="btn" type="button" disabled={batchBusy} onclick={() => void skipMember(member.position)}>Skip and continue</button>
                  </div>
                {/if}
              </li>
            {/each}
          </ol>
          {#if batch.state === 'stopped'}
            <div class="batch-foot">
              <p class="hint">Retry the failed machine, skip it and continue, or abandon the batch.</p>
              <button class="btn" type="button" disabled={batchBusy || !batchCanAbandon(batch)} onclick={() => void abandon()}>Abandon batch</button>
            </div>
          {/if}
          {#if batchError}<p class="error" role="alert">{batchError}</p>{/if}
        </section>
      {/if}

      {#if !batchActive && selection.length > 0}
        <section class="selection" aria-label="Batch order">
          <div class="batch-head">
            <strong>Batch order</strong>
            <small class="hint">One machine at a time, in this order.</small>
          </div>
          <ol class="ordered">
            {#each selectedRows as row, index (row.agentId)}
              <li>
                <span class="pos">{index + 1}</span>
                <div class="copy">
                  <strong>{row.host}</strong>
                  <small>{fleetSoftwareLabel(row)}</small>
                </div>
                <div class="order-actions">
                  <button class="icon small" type="button" aria-label={`Move ${row.host} earlier`} disabled={index === 0} onclick={() => moveSelection(index, -1)}>↑</button>
                  <button class="icon small" type="button" aria-label={`Move ${row.host} later`} disabled={index === selection.length - 1} onclick={() => moveSelection(index, 1)}>↓</button>
                  <button class="btn quiet" type="button" onclick={() => toggleSelection(row.agentId)}>Remove</button>
                </div>
              </li>
            {/each}
          </ol>
          <div class="batch-foot">
            <p class="hint">{selection.length} {selection.length === 1 ? 'machine' : 'machines'} reviewed. reet stops at the first failure.</p>
            <button class="btn primary" type="button" onclick={openBatchConfirm}>Review batch ({selection.length})</button>
          </div>
        </section>
      {/if}

      {#if rows.length === 0}
        <p class="hint">No enrolled machines.</p>
      {:else}
        <ul class="rows">
          {#each rows as row (row.agentId)}
            {@const warning = fleetSoftwareWarning(row)}
            {@const canApply = fleetSoftwareCanApply(row)}
            <li data-machine={row.host} data-state={row.state}>
              {#if !batchActive && canApply}
                <label class="pick">
                  <input
                    type="checkbox"
                    checked={selection.includes(row.agentId)}
                    onchange={() => toggleSelection(row.agentId)}
                    aria-label={`Include ${row.host} in a batch`}
                  />
                </label>
              {/if}
              <div class="copy">
                <strong>{row.host}{#if row.version}<code> {row.version}</code>{/if}</strong>
                <small class="label">{fleetSoftwareLabel(row)}</small>
                <small>{row.sentence}</small>
                {#if warning}<small class="warn">{warning}</small>{/if}
                {#if notice[row.agentId]}<small class="notice-line">{notice[row.agentId]}</small>{/if}
                {#if applyError[row.agentId]}<small class="error">{applyError[row.agentId]}</small>{/if}
                {#if row.source}<small class="source">{row.source}</small>{/if}
              </div>
              {#if row.action && (row.state === 'install' || row.state === 'update')}
                <button
                  class="btn apply"
                  type="button"
                  disabled={!canApply || applying === row.agentId || batchActive}
                  onclick={() => openApply(row)}
                  aria-label={`Apply ${row.action} plan on ${row.host}`}
                >
                  {applying === row.agentId ? 'Applying…' : 'Apply plan'}
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      <footer>
        <p class="hint">{batchActive ? 'A batch is in progress. Finish or abandon it before starting another.' : "Choose machines to batch, or apply one on its own. Rollback and remove live in a machine's Settings."}</p>
        <button class="btn" type="button" onclick={() => void load()} disabled={loading}>Refresh</button>
      </footer>
    {/if}
  </div>
</dialog>

<dialog bind:this={confirmDialog} class="confirm" aria-labelledby={`${id}-confirm`} onclose={() => { confirm = null }}>
  <div class="confirm-body">
    <h2 id={`${id}-confirm`}>Apply this {confirm?.action}?</h2>
    {#if confirm}
      <p>{confirm.sentence}</p>
      <dl>
        <dt>Machine</dt><dd>{confirm.host}</dd>
        <dt>Version</dt><dd>{confirm.target || confirm.version}</dd>
        <dt>Source</dt><dd><code>{confirm.source}</code></dd>
      </dl>
      {#if fleetSoftwareWarning(confirm)}<p class="warn">{fleetSoftwareWarning(confirm)}</p>{/if}
      <p class="hint">reet downloads the artifact, verifies its recorded checksum, and only then makes it current. Only this machine's plan runs.</p>
      {#if applyError[confirm.agentId]}<p class="error">{applyError[confirm.agentId]}</p>{/if}
    {/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => confirmDialog?.close()} disabled={!!applying}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void applyConfirm()} disabled={!!applying || !confirm || !fleetSoftwareCanApply(confirm)}>{applying ? 'Applying…' : 'Apply now'}</button>
    </div>
  </div>
</dialog>

<dialog bind:this={batchDialog} class="confirm" aria-labelledby={`${id}-batch`} onclose={() => { batchError = '' }}>
  <div class="confirm-body">
    <h2 id={`${id}-batch`}>Apply this batch?</h2>
    <p>{selection.length} {selection.length === 1 ? 'machine' : 'machines'} will be applied in this order, one at a time. reet stops at the first failure.</p>
    <ol class="ordered compact">
      {#each selectedRows as row, index (row.agentId)}
        <li>
          <span class="pos">{index + 1}</span>
          <span class="copy"><strong>{row.host}</strong><small>{fleetSoftwareLabel(row)}</small>{#if row.source}<code>{row.source}</code>{/if}</span>
        </li>
      {/each}
    </ol>
    <p class="hint">Each machine re-checks its own installation before it downloads anything. A machine whose installation changed since this review refuses and stops the batch.</p>
    {#if batchError}<p class="error">{batchError}</p>{/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => batchDialog?.close()} disabled={batchBusy}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void startBatch()} disabled={batchBusy || selection.length === 0}>{batchBusy ? 'Starting…' : 'Start batch'}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { width: min(720px, calc(100vw - 32px)); max-height: min(88vh, 820px); padding: 0; border: 1px solid var(--line); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px color-mix(in srgb, var(--ink) 28%, transparent); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); backdrop-filter: blur(2px); }
  .sheet { display: grid; gap: 18px; padding: clamp(20px, 4vw, 30px); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.86rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  .icon.small { width: 32px; height: 32px; font-size: 1rem; }
  .hint { margin: 0; color: var(--muted); font-size: 0.8rem; line-height: 1.5; }
  .mirror-line { margin-top: 8px; }
  .error { margin: 0; color: var(--bad); font-size: 0.86rem; }
  .rows, .batch-members, .ordered { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .rows li, .batch-members li, .ordered li { display: flex; align-items: flex-start; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .copy { display: grid; gap: 3px; min-width: 0; flex: 1 1 auto; }
  .copy strong { font-size: 0.9rem; }
  .copy code { font-size: 0.8rem; }
  .copy small { color: var(--muted); font-size: 0.78rem; line-height: 1.5; overflow-wrap: anywhere; }
  .copy .label { color: var(--ink); font-weight: 600; }
  .copy .warn { color: var(--warn); }
  .copy .notice-line { color: var(--good); }
  .copy .source { font: 0.72rem/1.4 var(--mono, monospace); }
  .apply { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  .pick { display: flex; align-items: center; padding-top: 2px; }
  .pick input { width: 18px; height: 18px; accent-color: var(--accent); cursor: pointer; }
  .batch, .selection { display: grid; gap: 10px; padding: 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--accent) 5%, var(--bg)); }
  .batch-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .batch-head strong { font-size: 0.92rem; }
  .batch-head .label { color: var(--accent-text); font-weight: 600; margin-left: 8px; font-size: 0.8rem; }
  .pos { flex: none; width: 22px; height: 22px; display: grid; place-items: center; border-radius: 50%; background: var(--card); border: 1px solid var(--line); color: var(--muted); font-size: 0.72rem; font-variant-numeric: tabular-nums; }
  .batch-members li[data-state='applied'] .label { color: var(--good); }
  .batch-members li[data-state='failed'] { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .batch-members li[data-state='excluded'] { opacity: 0.8; }
  .order-actions, .batch-actions { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; flex: none; }
  .batch-actions .btn { min-height: 32px; padding: 6px 10px; font-size: 0.78rem; }
  .batch-actions .btn + .btn { margin-left: 0; }
  .batch-foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .batch-foot .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding-top: 14px; border-top: 1px solid var(--line); }
  footer .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  .confirm { width: min(520px, calc(100vw - 32px)); border-radius: 16px; }
  .confirm-body { display: grid; gap: 12px; padding: clamp(20px, 4vw, 28px); }
  .confirm h2 { margin: 0; font: 500 clamp(1.3rem, 3vw, 1.7rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  .confirm p { margin: 0; color: var(--ink); font-size: 0.88rem; line-height: 1.55; }
  .confirm .warn { color: var(--warn); }
  .confirm .hint { color: var(--muted); font-size: 0.8rem; }
  .confirm dl { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; font-size: 0.82rem; }
  .confirm dt { color: var(--muted); }
  .confirm dd { margin: 0; overflow-wrap: anywhere; }
  .ordered.compact { gap: 4px; }
  .ordered.compact li { padding: 6px 8px; align-items: center; }
  .ordered.compact .copy { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
  .confirm-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
  .confirm-actions .btn { min-height: 38px; padding: 8px 14px; }
  .confirm-actions .primary { border-color: var(--accent); background: var(--accent); color: var(--accent-ink); }
  @media (max-width: 560px) {
    .rows li, .batch-members li, .ordered li { padding: 10px 12px; flex-wrap: wrap; }
    .apply { width: 100%; }
    .order-actions, .batch-actions { width: 100%; }
  }
  @media (pointer: coarse) {
    footer .btn, .apply, .batch-actions .btn { min-height: 44px; }
  }
</style>
