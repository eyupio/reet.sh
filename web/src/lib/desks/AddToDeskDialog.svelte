<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { TerminalError, type TerminalSession } from '../terminal/api'
  import { getDesk, listDesks, type Desk, type DeskSummary } from './api'
  import { deskHolding, putOnDesk, roomIn } from './addSession'
  import { MAX_TABS } from './tree'

  // Add to a desk…: a running session from its menu (Sessions and Home) into a new pane on one of the person's desks, in
  // a tab they choose or a new one. The desks are read when the dialog opens, never before. A session already on a desk
  // is offered as Move here: it is taken off its old pane, then added here.
  let { session, onclose, ondone, onsignedout }: {
    session: TerminalSession
    onclose: () => void
    /** It was added (or moved): the sentence to show. */
    ondone: (text: string) => void
    onsignedout: (e: unknown) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
  let desks = $state<DeskSummary[] | null>(null), readFailed = $state(false)
  let deskId = $state(''), desk = $state<Desk | null>(null), tabId = $state(''), deskFailed = $state(false)
  let busy = $state(false), error = $state('')
  const choices = $derived((desks ?? []).filter(d => !d.template))
  const holder = $derived(desks ? deskHolding(desks, session.id) : undefined)
  const moving = $derived(!!holder)
  const loadedTabs = $derived(desk && desk.id === deskId ? desk.doc.tabs : [])

  const signedOut = (e: unknown) => { if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return true } return false }
  onMount(() => {
    dialog?.showModal()
    void listDesks().then(list => {
      desks = list.filter(d => !((d as { schema?: number }).schema! > 1))
      const first = choices.find(d => d.id !== holder?.id) ?? choices[0]
      if (first) pick(first.id)
    }).catch(e => { if (!signedOut(e)) readFailed = true })
    return () => dialog?.close()
  })
  function pick(id: string) {
    deskId = id; desk = null; tabId = ''; deskFailed = false; error = ''
    void getDesk(id).then(d => {
      if (deskId !== id) return
      desk = d
      const active = d.doc.tabs.find(t => t.id === d.doc.active)
      tabId = active && roomIn(active) ? active.id : d.doc.tabs.find(roomIn)?.id ?? ''
    }).catch(e => { if (!signedOut(e) && deskId === id) deskFailed = true })
  }
  function close() { if (busy) return; dialog?.close(); onclose() }
  async function submit(event: SubmitEvent) {
    event.preventDefault()
    const to = choices.find(d => d.id === deskId)
    if (busy || !to || !desk) return
    busy = true; error = ''
    try {
      await putOnDesk(session, to, tabId, holder)
      ondone(holder ? `Sorted. “${session.title}” moved to “${to.name}”.` : `Sorted. “${session.title}” is on “${to.name}”.`)
      dialog?.close()
      onclose()
    } catch (e) {
      if (signedOut(e)) return
      error = e instanceof Error && e.message ? e.message : 'That session could not be added just now. Try again.'
    } finally { busy = false }
  }
</script>

<dialog bind:this={dialog} class="add-desk" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={event => { event.preventDefault(); close() }}>
  <form class="box" data-shell-card onsubmit={submit}>
    <h2 id={titleId}>Add to a desk</h2>
    <p data-shell-aside>“{session.title}” opens in a new pane. It keeps running as it is.</p>
    {#if readFailed}
      <p class="bad" role="alert">Your desks could not be read just now. Try again in a moment.</p>
    {:else if desks === null}
      <p role="status">Reading your desks…</p>
    {:else if choices.length === 0}
      <p>You have no desks yet. Make one on Desks first.</p>
    {:else}
      {#if holder}<p class="move">It is on “{holder.name}” now. Moving it takes it off that desk; the pane there stays, empty.</p>{/if}
      <label class="field"><span>Desk</span>
        <select aria-label="Desk" value={deskId} onchange={event => pick(event.currentTarget.value)}>
          {#each choices as d (d.id)}<option value={d.id}>{d.name}{d.id === holder?.id ? ' (it is on this one)' : ''}</option>{/each}
        </select>
      </label>
      {#if deskFailed}
        <p class="bad" role="alert">That desk could not be read just now. Try again in a moment.</p>
      {:else if !desk || desk.id !== deskId}
        <p role="status">Reading the desk…</p>
      {:else}
        <fieldset class="tabs">
          <legend>Tab</legend>
          {#each loadedTabs as t (t.id)}
            {@const room = roomIn(t)}
            <label class="tab" class:off={!room}><input type="radio" name="add-desk-tab" value={t.id} bind:group={tabId} disabled={!room} />
              <span>{t.name || 'Untitled'}{#if !room}<small>{` · ${Object.keys(t.panes).length ? 'full' : 'empty'}`}</small>{/if}</span></label>
          {/each}
          <label class="tab" class:off={loadedTabs.length >= MAX_TABS}><input type="radio" name="add-desk-tab" value="" bind:group={tabId} disabled={loadedTabs.length >= MAX_TABS} /><span>A new tab</span></label>
        </fieldset>
      {/if}
    {/if}
    {#if error}<p class="bad" role="alert">{error}</p>{/if}
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" onclick={close}>Cancel</button>
      <button type="submit" class="btn primary" disabled={busy || !desk || desk.id !== deskId || (!tabId && loadedTabs.length >= MAX_TABS)} aria-busy={busy}>{busy ? (moving ? 'Moving…' : 'Adding…') : moving ? 'Move here' : 'Add'}</button>
    </div>
  </form>
</dialog>

<style>
  dialog { width: min(480px, calc(100vw - 32px)); max-width: none; max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { display: flex; flex-direction: column; gap: 12px; padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; }
  p { margin: 0; color: var(--muted); overflow-wrap: anywhere; }
  .move { color: var(--ink); padding: 8px 10px; border-radius: 8px; background: var(--tint); }
  .field { display: flex; flex-direction: column; gap: 6px; font-weight: 600; }
  .tabs { margin: 0; padding: 0; border: 0; display: flex; flex-direction: column; gap: 2px; }
  .tabs legend { padding: 0 0 6px; font-weight: 600; }
  .tab { display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 8px; border-radius: 8px; cursor: pointer; overflow-wrap: anywhere; }
  .tab:hover:not(.off) { background: var(--tint); }
  .tab.off { color: var(--faint); cursor: default; }
  .tab input { width: 16px; height: 16px; margin: 0; flex: none; accent-color: var(--accent); }
  .tab small { color: var(--muted); font-size: 14px; }
  .bad { color: var(--bad); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
  @media (pointer: coarse) { .tab { min-height: 44px; } }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
</style>
