<script lang="ts">
  import ChatPane from '../../src/lib/terminal/ChatPane.svelte'
  import type { TerminalSession } from '../../src/lib/terminal/api'

  // The chat pane on its own with buttons that raise sample questions, so the card can be tried by hand on a phone or with a screen
  // reader. The answers it sends are only listed under the pane.
  const tool = new URLSearchParams(location.search).get('tool') === 'claude' ? 'claude' : 'opencode'
  const session = { id: 'session-fixture0000000001', agentId: 'connector-fixture0000001', title: 'Fixture chat', state: 'running', createdAt: 1, updatedAt: 1, exitCode: null, tool, model: '', effort: '', yolo: false, projectId: null, dir: '/home/dev/site', structured: true } as unknown as TerminalSession
  const emit = (type: string, data: unknown) => (window as unknown as { __emit: (t: string, d: unknown) => void }).__emit(type, data)

  let seq = $state(0), n = $state(0)
  let log = $state<string[]>([])
  document.addEventListener('fixture-answer', (e) => { const d = (e as CustomEvent).detail; log = [...log, `${d.id}: ${JSON.stringify(d.body)}`] })

  const forms: Record<string, (id: string) => unknown> = {
    choice: (id) => ({ message: 'Which approach should I take?', fields: [
      { key: 'approach', title: 'Approach', kind: 'single', required: true, options: [{ value: 'a', label: 'Approach A', description: 'The quick one' }, { value: 'b', label: 'Approach B' }] },
      { key: 'note', title: 'Anything else', kind: 'text', maxLength: 200 }] }),
    several: (id) => ({ message: 'Which checks should I run?', fields: [
      { key: 'checks', title: 'Checks to run', kind: 'multi', maxItems: 2, options: [{ value: 'x', label: 'Lint' }, { value: 'y', label: 'Unit tests' }, { value: 'z', label: 'Build' }] }] }),
    long: (id) => ({ message: 'This is a deliberately long question so that the card has to wrap on a narrow screen and keep every control inside the viewport while it does so, including the buttons below it?', fields: [
      { key: 'reply', title: 'Your reply, in as much detail as you would like to give', kind: 'text', required: true, maxLength: 500 }] }),
  }
  function ask(kind: string) {
    n += 1
    const id = `q${n}`
    emit('chat', { seq: ++seq, kind: 'question', questionId: id, toolCallId: `t${n}`, question: forms[kind](id) })
  }
  function start() { emit('reset', { truncated: false }); emit('chat', { seq: ++seq, kind: 'status', state: 'working' }) }
  function resolve(reason: string) { emit('chat', { seq: ++seq, kind: 'question_resolved', questionId: `q${n}`, reason }) }
  function timeout() { emit('chat', { seq: ++seq, kind: 'question_cancelled', questionId: `q${n}`, reason: 'timeout' }) }
</script>

<main>
  <h1>Question card fixture</h1>
  <p class="lead">The real chat pane. Nothing is sent anywhere. Raise a question, then answer it as you would in the app.</p>
  <div class="controls" role="group" aria-label="Fixture controls">
    <button type="button" class="btn" onclick={start}>Start a turn</button>
    <button type="button" class="btn" onclick={() => ask('choice')}>Ask: choose one</button>
    <button type="button" class="btn" onclick={() => ask('several')}>Ask: choose several</button>
    <button type="button" class="btn" onclick={() => ask('long')}>Ask: long text</button>
    <button type="button" class="btn" onclick={() => resolve('accept')}>Agent saw the answer</button>
    <button type="button" class="btn" onclick={timeout}>Time the question out</button>
  </div>
  <section class="pane" aria-label="Chat"><ChatPane {session} /></section>
  <section aria-label="Answers sent">
    <h2>Answers sent</h2>
    <ul data-testid="answers">{#each log as line}<li>{line}</li>{:else}<li>None yet.</li>{/each}</ul>
  </section>
</main>

<style>
  main { max-width: 760px; margin: 0 auto; padding: 16px; display: grid; gap: 12px; }
  .controls { display: flex; flex-wrap: wrap; gap: 8px; }
  .pane { min-height: 420px; border: 1px solid var(--line); border-radius: 12px; overflow: hidden; display: grid; }
  h1 { font-size: 1.2rem; margin: 0; } h2 { font-size: 1rem; margin: 0 0 4px; } .lead { margin: 0; color: var(--muted); }
</style>
