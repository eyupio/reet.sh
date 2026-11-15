<script lang="ts">
  // Dictation through the browser's own speech recognition, where it has one. Each finished phrase is handed to the
  // field that owns the button; nothing is sent by speaking. The browser sends the audio to its own speech service,
  // not to reet, and the button's title says so. A browser with no speech recognition draws nothing.
  import { onDestroy } from 'svelte'
  import TerminalIcon from './TerminalIcon.svelte'
  import { dictationProblem, speechRecognition } from './compose'

  let { icon = false, disabled = false, listening = $bindable(false), interim = $bindable(''), problem = $bindable(''), onphrase }: {
    /** An icon-only button for a row of tools; otherwise a labelled one. */
    icon?: boolean
    disabled?: boolean
    /** True while the browser is listening. */
    listening?: boolean
    /** Words still being heard, not yet a finished phrase. */
    interim?: string
    /** Why dictation stopped, when there is something to do about it. */
    problem?: string
    onphrase: (text: string) => void
  } = $props()

  type Recognition = { lang: string; continuous: boolean; interimResults: boolean; start(): void; stop(): void; abort(): void
    onresult: ((event: { resultIndex: number; results: ArrayLike<ArrayLike<{ transcript: string }> & { isFinal: boolean }> }) => void) | null
    onerror: ((event: { error: string }) => void) | null; onend: (() => void) | null }
  const Speech = typeof window === 'undefined' ? undefined : speechRecognition<new () => Recognition>(window as unknown as { SpeechRecognition?: new () => Recognition; webkitSpeechRecognition?: new () => Recognition })
  let recognition: Recognition | undefined

  /** Stops listening at once, discarding what was still being heard: the field's text is about to be sent or dropped. */
  export function stop() { recognition?.abort() }

  function toggle() {
    if (listening) { recognition?.stop(); return }
    if (!Speech) return
    problem = ''
    const next = new Speech()
    next.lang = navigator.language || 'en-GB'
    next.continuous = true
    next.interimResults = true
    next.onresult = event => {
      let heard = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i]
        if (result.isFinal) onphrase(result[0].transcript)
        else heard += result[0].transcript
      }
      interim = heard.trim()
    }
    next.onerror = event => { problem = dictationProblem(event.error) }
    next.onend = () => { listening = false; interim = ''; if (recognition === next) recognition = undefined }
    recognition = next
    try { next.start(); listening = true } catch { problem = dictationProblem('unknown'); recognition = undefined }
  }
  onDestroy(() => recognition?.abort())
  const title = 'Dictate with your browser’s speech recognition. Your browser sends the audio to its own speech service, not to reet.'
</script>

{#if Speech}
  <button type="button" class={icon ? 'dictate icon' : 'btn dictate'} aria-pressed={listening} aria-label={icon ? (listening ? 'Stop dictating' : 'Dictate') : undefined} {disabled} {title} onclick={toggle}>
    <TerminalIcon name="mic" />{#if !icon}{listening ? 'Stop dictating' : 'Dictate'}{/if}
  </button>
{/if}

<style>
  .dictate { display: inline-flex; align-items: center; justify-content: center; gap: 6px; }
  .dictate[aria-pressed='true'] { border-color: var(--bad); color: var(--bad); }
  .dictate.icon { flex: none; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--muted); cursor: pointer; }
  .dictate.icon:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
  .dictate.icon:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .dictate.icon:disabled { opacity: .5; cursor: default; }
  .dictate.icon[aria-pressed='true'] { background: color-mix(in srgb, var(--bad) 14%, transparent); }
  @media (pointer: coarse) { .dictate.icon { width: 44px; height: 44px; } }
</style>
