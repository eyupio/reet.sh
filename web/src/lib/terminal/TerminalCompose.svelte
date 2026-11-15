<script lang="ts">
  // The compose box: write (or dictate) a message in an ordinary field, where the phone's autocorrect, suggestions and
  // dictation all work, then send it to the terminal as one paste. Nothing is sent until Send or Insert is pressed.
  import { onMount, tick } from 'svelte'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import Dictate from './Dictate.svelte'
  import { COMPOSE_MAX, appendDictation, composeText } from './compose'

  let { draft = $bindable(''), live, bracketed = false, target, onsend, onclose }: {
    draft?: string
    /** The terminal is connected: a message can be sent. */
    live: boolean
    /** The program in the terminal takes a paste as one block (bracketed paste). A plain shell without it runs each line as it arrives. */
    bracketed?: boolean
    /** What the message goes to, for the sheet's own words: "Claude Code" or "the shell". */
    target: string
    /** `enter` presses Enter after the text (Send); without it the text is left on the prompt (Insert). */
    onsend: (text: string, enter: boolean) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  let field = $state<HTMLTextAreaElement>()
  const text = $derived(composeText(draft))
  const lines = $derived(text ? text.split('\n').length : 0)
  onMount(() => { void tick().then(() => field?.focus()) })

  // Dictation (Dictate.svelte) adds each finished phrase to the draft. The phone keyboard's own microphone works in
  // this field without it; the button is for browsers on a computer and for keyboards with none.
  let dictate = $state<{ stop(): void }>()
  let listening = $state(false), interim = $state(''), said = $state('')

  function send(enter: boolean) {
    if (!text || !live) return
    dictate?.stop()
    onsend(text, enter)
  }
  // Enter is a new line, as in any message field on a phone; Ctrl or Cmd with Enter sends.
  function keys(event: KeyboardEvent) {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) { event.preventDefault(); send(true) }
  }
</script>

<WorkspaceDialog title="Compose" {onclose}>
  <form class="compose" onsubmit={event => { event.preventDefault(); send(true) }}>
    <label for={`${id}-text`}>Message for {target}</label>
    <textarea id={`${id}-text`} bind:this={field} bind:value={draft} rows="5" maxlength={COMPOSE_MAX} onkeydown={keys}
      autocapitalize="sentences" autocomplete="on" spellcheck="true" enterkeyhint="enter"
      placeholder="Write it here, with your keyboard’s autocorrect and dictation. It is sent as one paste."
      aria-describedby={`${id}-hint`}></textarea>
    {#if interim}<p class="interim" aria-hidden="true">{interim}…</p>{/if}
    <p class="compose-hint" id={`${id}-hint`}>
      {#if !live}Reconnect before sending. Your text is kept.
      {:else if lines > 1 && !bracketed}<span class="warn">This terminal takes each line as it arrives, so the first {lines - 1 === 1 ? 'line runs' : `${lines - 1} lines run`} as soon as {lines - 1 === 1 ? 'it is' : 'they are'} pasted.</span> Put it on one line to send it as one message.
      {:else}<strong>Send</strong> pastes this and presses Enter. <strong>Insert</strong> leaves it on the prompt for you to finish.{/if}
    </p>
    <div class="actions">
      <Dictate bind:this={dictate} bind:listening bind:interim bind:problem={said} onphrase={phrase => (draft = appendDictation(draft, phrase))} />
      <span class="spacer"></span>
      <button type="button" class="btn" disabled={!text || !live} onclick={() => send(false)}>Insert</button>
      <button type="submit" class="btn primary" disabled={!text || !live}>Send</button>
    </div>
    <p class="said" role="status">{#if listening}Listening. Your browser sends the audio to its own speech service, not to reet.{:else}{said}{/if}</p>
  </form>
</WorkspaceDialog>

<style>
  .compose { display: grid; gap: 10px; }
  label { font: 600 0.875rem/1.4 var(--sans); color: var(--muted); }
  textarea { width: 100%; box-sizing: border-box; min-height: 132px; max-height: 40dvh; padding: 10px 12px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--bg-sunk); color: var(--ink); font: 16px/1.5 var(--sans); resize: vertical; }
  textarea:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .interim { margin: -4px 2px 0; color: var(--muted); font-size: 0.875rem; font-style: italic; overflow-wrap: anywhere; }
  .compose-hint, .said { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .compose-hint strong { color: var(--ink); font-weight: 600; }
  .warn { color: var(--warn); font-weight: 600; }
  .said:empty { display: none; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .spacer { flex: 1; }
  .actions :global(.btn) { min-height: 44px; }
  @media (max-width: 420px) { .spacer { flex-basis: 100%; height: 0; } .actions > .btn { flex: 1; } .actions :global(.dictate) { width: 100%; } }
</style>
