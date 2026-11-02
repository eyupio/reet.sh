<script lang="ts">
  import { onMount } from 'svelte'
  import ChatPane from '../../src/lib/terminal/ChatPane.svelte'
  import GafferAvatar from '../../src/lib/terminal/GafferAvatar.svelte'
  import type { TerminalSession } from '../../src/lib/terminal/api'

  const session = { id: 'session-gaffer-preview001', agentId: 'connector-preview00001', title: 'reet.sh · Gaffer', state: 'running', createdAt: 1, updatedAt: 1, exitCode: null, tool: 'opencode', model: '', effort: '', yolo: false, projectId: null, dir: '/home/dev/reet.sh', structured: true } as unknown as TerminalSession
  type Stream = { emit: (data: unknown) => void; reset: () => void; disconnect: () => void; reconnect: () => void }
  const stream = () => (window as unknown as { __gaffer: Stream }).__gaffer
  let current = $state('Editing'), sequence = 0
  const send = (data: object) => stream().emit({ seq: ++sequence, ...data })
  function scene(name: string) {
    current = name
    stream().reconnect(); stream().reset(); sequence = 0
    send({ kind: 'user', text: 'Polish the Gaffer chat overlay. Keep it clear, warm and properly Yorkshire.' })
    send({ kind: 'status', state: 'working' })
    send({ kind: 'agent', messageId: 'intro', text: 'I’m checking the existing chat layout, then refining the progress panel and its responsive styles.' })
    send({ kind: 'tool', toolCallId: 'read', toolKind: 'read', title: 'Read chat layout', path: '/home/dev/reet.sh/web/src/lib/terminal/ChatPane.svelte', status: name === 'Reading' ? 'in_progress' : 'completed', durationMs: 184, text: 'Read the chat layout and its theme tokens.' })
    if (name === 'Reading') return
    send({ kind: 'tool', toolCallId: 'search', toolKind: 'search', title: 'Find shared theme tokens', status: name === 'Searching' ? 'in_progress' : 'completed', durationMs: 92, text: 'Theme tokens found in app.css.' })
    if (name === 'Searching') return
    if (name === 'Thinking') { send({ kind: 'tool', toolCallId: 'think', toolKind: 'think', title: 'Thinking', status: 'in_progress', text: 'Considering how to keep progress clear at phone widths.' }); return }
    send({ kind: 'tool', toolCallId: 'edit', toolKind: 'edit', path: '/home/dev/reet.sh/web/src/lib/terminal/GafferProgress.svelte', title: 'Refine progress panel', status: name === 'Editing' ? 'in_progress' : 'completed', additions: 24, deletions: 6, durationMs: name === 'Editing' ? 0 : 842, diff: '@@ -1,2 +1,2 @@\n-<p>Working…</p>\n+<GafferProgress {chat} />' })
    if (name === 'Editing') return
    if (name === 'Approval') { send({ kind: 'permission', permissionId: 'preview-approval', toolKind: 'execute', title: 'Run the web checks', command: 'npm run check' }); send({ kind: 'status', state: 'waiting' }); return }
    if (name === 'Question') { send({ kind: 'question', questionId: 'preview-question', question: { message: 'How much Yorkshire should Gaffer bring?', fields: [{ key: 'tone', kind: 'single', required: true, title: 'Gaffer’s voice', options: [{ value: 'warm', label: 'Warm, with a little cheek' }, { value: 'more', label: 'Turn up the Yorkshire' }] }] } }); return }
    const finished = name === 'Finished' || name === 'Ended'
    send({ kind: 'tool', toolCallId: 'checks', toolKind: 'execute', command: 'npm run check', title: 'Run web checks', status: finished ? 'completed' : name === 'Snag' ? 'failed' : 'in_progress', text: finished ? 'All fixture checks passed.' : name === 'Snag' ? 'One fixture check failed.' : '' })
    if (name === 'Reconnecting') { stream().disconnect(); return }
    if (name === 'Error') { send({ kind: 'status', state: 'error', detail: 'The provider’s rate limit was reached. Wait for it to reset or choose another configured model.' }); return }
    if (finished || name === 'Snag') {
      send({ kind: 'agent', messageId: 'done', text: name === 'Snag' ? 'The progress panel was updated. One fixture check still needs attention; review its output before continuing.' : 'The progress panel is ready to review. Gaffer now has a younger face, expressive gestures and a cheekier voice. The recorded web check completed; review the changes before carrying on.' })
      send({ kind: 'status', state: name === 'Ended' ? 'ended' : 'idle' })
    }
  }
  function theme() { document.documentElement.dataset.theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light' }
  onMount(() => { setTimeout(() => scene(new URLSearchParams(location.search).get('scene') || 'Editing'), 80) })
  onMount(() => {
    const listener = (event: Event) => {
      const { kind, body, id } = (event as CustomEvent).detail
      if (kind === 'permission') send({ kind: 'permission_resolved', permissionId: id, allow: !!body.allow })
      if (kind === 'question') send({ kind: 'question_resolved', questionId: id, reason: body.action })
      if (kind === 'cancel') { send({ kind: 'status', state: 'cancelled' }); return }
      if (kind === 'prompt') send({ kind: 'user', text: body.text })
      send({ kind: 'status', state: 'working' })
      send({ kind: 'tool', toolCallId: `demo-${sequence}`, toolKind: 'read', title: 'Scripted demo task', path: '/home/dev/reet.sh/web/src/app.css', status: 'in_progress' })
    }
    document.addEventListener('gaffer-fixture-action', listener)
    return () => document.removeEventListener('gaffer-fixture-action', listener)
  })
</script>

<main>
  <header class="review-head"><a href="/tests/fixtures/gaffer-progress.html">reet<span>.sh</span></a><span class="review-label">Gaffer · design preview</span><button type="button" class="theme" onclick={theme}>Switch theme</button></header>
  <div class="review-layout">
    <aside>
      <div class="meet"><GafferAvatar active /></div>
      <p class="eyebrow">A proper pair of hands.</p>
      <h1>Meet the Gaffer.</h1>
      <p>He minds the brief.<br />The agent gets stuck in.<br />You stay in the know.</p>
      <div class="note">A little Yorkshire.<br />A lot of care.</div>
      <span class="demo-note">Scripted demo activity</span>
    </aside>
    <div class="workspace">
      <div class="session-head"><div><span class="session-mark" aria-hidden="true">g</span><strong>Gaffer</strong><span class="project">reet.sh / web</span></div><span class="session-state"><i></i> Preview</span></div>
      <div class="pane"><ChatPane {session} /></div>
    </div>
  </div>
  <nav class="scenes" aria-label="Preview states"><span>Try a state</span>{#each ['Thinking', 'Reading', 'Searching', 'Editing', 'Running', 'Approval', 'Question', 'Reconnecting', 'Error', 'Snag', 'Finished', 'Ended'] as name}<button type="button" class:chosen={current === name} aria-pressed={current === name} onclick={() => scene(name)}>{name}</button>{/each}</nav>
</main>

<style>
  main { width: min(1240px, 100%); margin: 0 auto; padding: 26px 32px; }
  .review-head { display: flex; align-items: center; gap: 25px; margin-bottom: 32px; }
  .review-head a { color: var(--ink); text-decoration: none; font: 600 25px var(--sans); letter-spacing: -.06em; }.review-head a span { color: var(--accent-text); }
  .review-label { color: var(--muted); font: 11px var(--mono); }
  .theme { margin-left: auto; border: 1px solid var(--line); border-radius: 6px; background: var(--card); padding: 7px 10px; color: var(--muted); font-size: 11px; cursor: pointer; }
  .review-layout { display: grid; grid-template-columns: 250px minmax(0, 1fr); gap: 46px; align-items: center; }
  aside { padding: 12px 0; }.meet { width: 168px; height: 168px; margin-bottom: 27px; }
  .eyebrow { color: var(--accent-text); font-size: 11px; letter-spacing: .06em; }
  h1 { margin: 8px 0 17px; font: 600 35px/1.16 var(--serif); letter-spacing: -.025em; }
  aside > p:not(.eyebrow) { color: var(--muted); font: 14px/1.9 var(--sans); }.note { border-left: 2px solid var(--line-strong); margin-top: 25px; padding-left: 12px; color: var(--ink-soft); font: italic 14px/1.7 var(--serif); }.demo-note { display: block; margin-top: 32px; color: var(--faint); font: 10px var(--mono); }
  .workspace { border: 1px solid var(--line-strong); border-radius: 15px; overflow: hidden; box-shadow: 0 20px 60px #00000020; }
  .session-head { display: flex; justify-content: space-between; align-items: center; padding: 13px 16px; border-bottom: 1px solid var(--line); background: var(--card); }.session-head > div { display: flex; align-items: center; gap: 10px; }.session-head strong { font-size: 13px; }.session-mark { display: grid; place-items: center; width: 24px; height: 24px; background: var(--tint); color: var(--accent-text); border-radius: 6px; font: 600 18px var(--serif); }.project { color: var(--faint); font: 10px var(--mono); }.session-state { display: flex; align-items: center; gap: 6px; color: var(--muted); font-size: 10px; }.session-state i { width: 5px; height: 5px; border-radius: 50%; background: var(--good); }
  .pane { height: 565px; }
  .scenes { display: flex; flex-wrap: wrap; gap: 5px; align-items: center; justify-content: flex-end; padding-top: 22px; }.scenes > span { color: var(--faint); font: 10px var(--mono); margin-right: 8px; }.scenes button { min-height: 34px; border: 1px solid var(--line); border-radius: 6px; background: var(--card); color: var(--muted); padding: 4px 9px; font-size: 11px; cursor: pointer; }.scenes button.chosen { color: var(--accent-text); background: var(--accent-wash); border-color: var(--accent); }
  @media (max-width: 850px) { main { padding: 16px; }.review-layout { grid-template-columns: minmax(0, 1fr); gap: 0; }aside { display: none; }.review-head { margin-bottom: 16px; }.scenes { justify-content: flex-start; }.pane { height: 580px; } }
  @media (max-width: 430px) { main { padding: 8px; }.review-label { display: none; }.project { display: none; }.scenes button, .theme { min-height: 40px; }.pane { height: 590px; } }
</style>
