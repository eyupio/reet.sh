<script lang="ts">
  // The compose box: write (or dictate) a message in an ordinary field, where the phone's autocorrect, suggestions and
  // dictation all work, then send it to the terminal as one paste. Nothing is sent until Send or Insert is pressed.
  import { onDestroy, onMount, tick } from 'svelte'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import TerminalIcon from './TerminalIcon.svelte'
  import { COMPOSE_MAX, appendDictation, composeText, dictationProblem, speechRecognition } from './compose'

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

  // Dictation through the browser's own speech recognition, where it has one. The phone keyboard's microphone works in
  // this field without it; this button is for browsers on a computer and for keyboards with none.
  type Recognition = { lang: string; continuous: boolean; interimResults: boolean; start(): void; stop(): void; abort(): void
    onresult: ((event: { resultIndex: number; results: ArrayLike<ArrayLike<{ transcript: string }> & { isFinal: boolean }> }) => void) | null
    onerror: ((event: { error: string }) => void) | null; onend: (() => void) | null }
  const Speech = typeof window === 'undefined' ? undefined : speechRecognition<new () => Recognition>(window as unknown as { SpeechRecognition?: new () => Recognition; webkitSpeechRecognition?: new () => Recognition })
  let recognition: Recognition | undefined
  let listening = $state(false), interim = $state(''), said = $state('')

  function stopListening() { recognition?.stop() }
  function dictate() {
    if (listening) { stopListening(); return }
    if (!Speech) return
    said = ''
    const next = new Speech()
    next.lang = navigator.language || 'en-GB'
    next.continuous = true
    next.interimResults = true
    next.onresult = event => {
      let heard = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i]
        if (result.isFinal) draft = appendDictation(draft, result[0].transcript)
        else heard += result[0].transcript
      }
      interim = heard.trim()
    }
    next.onerror = event => { said = dictationProblem(event.error) }
    next.onend = () => { listening = false; interim = ''; if (recognition === next) recognition = undefined }
    recognition = next
    try { next.start(); listening = true } catch { said = dictationProblem('unknown'); recognition = undefined }
  }
  onDestroy(() => recognition?.abort())

  function send(enter: boolean) {
    if (!text || !live) return
    recognition?.abort()
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
      {#if Speech}
        <button type="button" class="btn dictate" aria-pressed={listening} onclick={dictate}
          title="Dictate with your browser’s speech recognition. Your browser sends the audio to its own speech service, not to reet.">
          <TerminalIcon name="mic" />{listening ? 'Stop dictating' : 'Dictate'}
        </button>
      {/if}
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
  .actions .btn { min-height: 44px; }
  .dictate { display: inline-flex; align-items: center; gap: 6px; }
  .dictate[aria-pressed='true'] { border-color: var(--bad); color: var(--bad); }
  @media (max-width: 420px) { .spacer { flex-basis: 100%; height: 0; } .actions .btn:not(.dictate) { flex: 1; } .dictate { width: 100%; justify-content: center; } }
</style>
