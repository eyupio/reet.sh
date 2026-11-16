<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import type { TerminalMachine, TerminalSession } from '../terminal/api'
  import type { DeskSummary } from './api'
  import { deskHolding } from './addSession'

  // Add a running session… (the desk's + menu): a session that is already running, put in a new tab on this desk. One on
  // another desk is offered as Move here: it is taken off that desk's pane first. Sessions already on this desk are not
  // listed; nothing is started.
  let { sessions, machines, desks, here, onadd, onclose }: {
    /** The running sessions not on this desk. */
    sessions: TerminalSession[]
    machines: TerminalMachine[]
    /** Every desk, to say which one a session is on. */
    desks: DeskSummary[]
    /** This desk's id. */
    here: string
    /** Adds it (moving it from `from` first); a refusal is thrown and shown here. */
    onadd: (session: TerminalSession, from?: { id: string; name: string }) => Promise<void>
    onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
  // svelte-ignore state_referenced_locally
  let chosen = $state(sessions[0]?.id ?? ''), busy = $state(false), error = $state('')
  const hostOf = (s: TerminalSession) => machines.find(m => m.agentId === s.agentId)?.host ?? 'a machine'
  const onOther = (s: TerminalSession) => { const d = deskHolding(desks, s.id); return d && d.id !== here ? d : undefined }
  const pick = $derived(sessions.find(s => s.id === chosen))
  const from = $derived(pick ? onOther(pick) : undefined)

  onMount(() => {
    dialog?.showModal()
    return () => dialog?.close()
  })
  function close() { if (busy) return; dialog?.close(); onclose() }
  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (busy || !pick) return
    busy = true; error = ''
    try {
      await onadd(pick, from ? { id: from.id, name: from.name } : undefined)
      dialog?.close()
      onclose()
    } catch (e) {
      error = e instanceof Error && e.message ? e.message : 'That session could not be added just now. Try again.'
    } finally { busy = false }
  }
</script>

<dialog bind:this={dialog} class="add-session" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={event => { event.preventDefault(); close() }}>
  <form class="box" data-shell-card onsubmit={submit}>
    <h2 id={titleId}>Add a running session</h2>
    <p data-shell-aside>It opens in a new tab here and keeps running as it is.</p>
    {#if sessions.length === 0}
      <p>No other session is running just now.</p>
    {:else}
      <fieldset class="list">
        <legend>Session</legend>
        {#each sessions as s (s.id)}
          {@const other = onOther(s)}
          <label class="row"><input type="radio" name="add-session" value={s.id} bind:group={chosen} />
            <span class="txt"><span class="title">{s.title}</span><small>{hostOf(s)}{#if other}{` · on “${other.name}”`}{/if}</small></span></label>
        {/each}
      </fieldset>
      {#if from}<p class="move">It is on “{from.name}” now. Moving it takes it off that desk; the pane there stays, empty.</p>{/if}
    {/if}
    {#if error}<p class="bad" role="alert">{error}</p>{/if}
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" onclick={close}>Cancel</button>
      <button type="submit" class="btn primary" disabled={busy || !pick} aria-busy={busy}>{busy ? (from ? 'Moving…' : 'Adding…') : from ? 'Move here' : 'Add'}</button>
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
  .list { margin: 0; padding: 0; border: 0; display: flex; flex-direction: column; gap: 2px; max-height: 50dvh; overflow-y: auto; }
  .list legend { padding: 0 0 6px; font-weight: 600; }
  .row { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 4px 8px; border-radius: 8px; cursor: pointer; }
  .row:hover { background: var(--tint); }
  .row input { width: 16px; height: 16px; margin: 0; flex: none; accent-color: var(--accent); }
  .txt { display: flex; flex-direction: column; min-width: 0; }
  .title { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .txt small { color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .bad { color: var(--bad); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
</style>
