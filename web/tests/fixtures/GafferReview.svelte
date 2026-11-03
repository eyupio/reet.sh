<script lang="ts">
  import GafferAvatar from '../../src/lib/terminal/GafferAvatar.svelte'
  import GafferProgress from '../../src/lib/terminal/GafferProgress.svelte'
  import { gafferAnimations } from '../../src/lib/gafferMotion'
  import { applyChatEvent, initialChat, type ChatEvent, type ChatState } from '../../src/lib/chat'
  import type { GafferPhase } from '../../src/lib/gafferProgress'
  import type { GafferPose, GafferReaction } from '../../src/lib/gafferPersonality'

  const scenes: { title: string; label: string; phase: GafferPhase; pose: GafferPose; reaction: GafferReaction; quote: string; note: string }[] = [
    { title: 'On the job', label: 'Reading the files', phase: 'reading', pose: 'duty', reaction: 'scan', quote: 'Read on, OpenCode. I’ve got my eye on the job sheet.', note: 'The approved reading frames: a glance down, a quiet nod, then back to rest.' },
    { title: 'Caught with a brew', label: 'You return · agent still working', phase: 'editing', pose: 'tea', reaction: 'sip', quote: 'Oh! You’re back. This is a strategic brew. OpenCode is still on the job.', note: 'A mug, a startled hop, then straight back to looking managerial.' },
    { title: 'Executive power nap', label: 'You return · session ended', phase: 'ended', pose: 'asleep', reaction: '', quote: 'Wasn’t asleep. Just reading the inside of my cap. OpenCode’s job sheet is below.', note: 'A quick doze, a stretch and an innocent look. The handover stays ready to read.' },
    { title: 'Snag duty', label: 'Session needs attention', phase: 'error', pose: 'concerned', reaction: 'shake', quote: 'Right, OpenCode. Read the error. My “give it a whack” policy is under review.', note: 'A concerned expression and a clear warning. No celebration or nap while there’s a problem.' },
    { title: 'Your call, boss', label: 'Waiting for your approval', phase: 'approval', pose: 'attention', reaction: 'listen', quote: 'Your call, boss. OpenCode is waiting. My cap doesn’t come with signing authority.', note: 'He leans in to listen. Your decision stays centre stage.' },
    { title: 'Ey up', label: 'Click Gaffer · or press Enter', phase: 'working', pose: 'duty', reaction: 'tip-cap', quote: 'That’s a flat cap, not a doorbell. OpenCode, make a note.', note: 'The approved cap-tip frames and a fresh quip. A short cooldown keeps the poking civilised.' },
    { title: 'Biscuit inspection', label: 'A proper quality-control nibble', phase: 'working', pose: 'duty', reaction: 'biscuit', quote: 'That biscuit’s load-bearing. OpenCode, nobody touch the biscuit.', note: 'A considered dunk, a little nibble, and the expression of a man doing essential research.' },
    { title: 'Now then', label: 'Another nudge · another opinion', phase: 'working', pose: 'duty', reaction: 'shrug', quote: 'Twice? Reet, that’s a meeting. OpenCode, take minutes. Very small ones.', note: 'Palms up, shoulders lift, then a slow settle. No rush. He’s paid by the eyebrow.' },
    { title: 'Steady on', label: 'Keep poking · earn more eyebrow', phase: 'working', pose: 'duty', reaction: 'huff', quote: 'I’ve had less supervision from a pigeon with a chip. OpenCode, don’t encourage ’em.', note: 'Arms folded, a deadpan double take, and a knowing look. A quiet spell resets the teasing.' },
  ]
  let selected = $state(0), replay = $state(0), dark = $state(true)
  let livePose = $state<GafferPose>('duty'), liveReaction = $state<GafferReaction>('scan')
  let returnTimer: ReturnType<typeof setTimeout> | undefined
  const scene = $derived(scenes[selected])
  const chat = $derived.by(() => {
    let state: ChatState = { ...initialChat(), connection: 'open' }
    const events: Omit<ChatEvent, 'seq'>[] = [
      { kind: 'user', text: 'Make Gaffer a little more himself.' },
      { kind: 'status', state: 'working' },
      { kind: 'tool', toolCallId: 'read', toolKind: 'read', path: '/project/web/src/lib/terminal/GafferAvatar.svelte', status: 'completed' },
      { kind: 'tool', toolCallId: 'current', toolKind: scene.phase === 'reading' ? 'read' : 'edit', path: '/project/web/src/lib/terminal/GafferProgress.svelte', status: scene.phase === 'ended' ? 'completed' : 'in_progress' },
    ]
    if (scene.phase === 'approval') events.push({ kind: 'permission', permissionId: 'preview', title: 'Run the web checks', command: 'npm run check' })
    if (scene.phase === 'ended') events.push({ kind: 'agent', messageId: 'handover', text: 'The flat cap, contextual gestures and click reactions are ready to review. Animation can be turned off while quotes, live updates and this summary stay visible.' })
    events.push({ kind: 'status', state: scene.phase === 'error' ? 'error' : scene.phase === 'ended' ? 'ended' : scene.phase === 'approval' ? 'waiting' : 'working', detail: scene.phase === 'error' ? 'The provider stopped responding. Review the session details before continuing.' : '' })
    for (let i = 0; i < events.length; i++) state = applyChatEvent(state, { seq: i + 1, ...events[i] })
    return state
  })
  $effect(() => { document.documentElement.dataset.theme = dark ? 'dark' : 'light' })
  function choose(index: number) {
    selected = index; replay++; clearTimeout(returnTimer)
    livePose = scenes[index].pose; liveReaction = scenes[index].reaction
  }
  function react() {
    replay++
    if (scene.pose === 'tea' || scene.pose === 'asleep') {
      livePose = scene.pose; liveReaction = scene.pose === 'tea' ? 'sip' : ''
      clearTimeout(returnTimer)
      returnTimer = setTimeout(() => { livePose = 'duty'; liveReaction = scene.pose === 'tea' ? 'startle' : 'stretch'; replay++ }, 900)
    } else { liveReaction = scene.reaction; replay++ }
  }
</script>

<svelte:head><title>Gaffer · Design review</title></svelte:head>
<main>
  <header><a href="#top">reet<span>.sh</span></a><span class="review-tag">Gaffer / design review</span><button class="theme" onclick={() => dark = !dark}>{dark ? 'Light' : 'Dark'} theme</button></header>
  <section class="intro" id="top"><div><p class="eyebrow">MORE YORKSHIRE. MORE EYEBROW.</p><h1>Now then, boss.<br />You rang. Again.</h1><p class="lede">Six unhurried little performances, a biscuit with structural concerns, and more opinions every time you give him a nudge.</p></div><div class="intro-art"><figure><div class="intro-avatar"><GafferAvatar phase="idle" paused={!$gafferAnimations} /></div><figcaption>02 / GRID</figcaption></figure><figure><div class="intro-avatar"><GafferAvatar variant="bust" phase="idle" paused={!$gafferAnimations} /></div><figcaption>03 / PHOSPHOR</figcaption></figure></div></section>
  <div class="design-rules"><span><i></i> Brief gestures, long rests</span><span>Reacts to the actual job</span><span>Always leaves the facts in view</span></div>
  <section class="gallery" aria-label="Gaffer mock states">
    {#each scenes as item, index}
      <button class="scene" class:selected={index === selected} onclick={() => choose(index)} aria-pressed={index === selected}>
        <div class="scene-top"><span class="number">0{index + 1}</span><div class="mini-avatar"><GafferAvatar variant="bust" phase={item.phase} pose={item.pose} reaction={item.reaction} reactionKey={index === selected ? replay : 0} paused={!$gafferAnimations} /></div><span class="preview-label">{item.label}</span></div>
        <h2>{item.title}</h2><p class="quip">“{item.quote}”</p><p class="annotation">{item.note}</p>
      </button>
    {/each}
  </section>
  <section class="try-it">
    <div class="try-heading"><div><p class="eyebrow">GIVE HIM A NUDGE</p><h2>{scene.title}</h2><p>Pick a scene above. Click his cap below to try the reaction.</p></div><button class="big-avatar" aria-label="Replay Gaffer reaction" onclick={react}><GafferAvatar variant="bust" phase={scene.phase} pose={livePose} reaction={liveReaction} reactionKey={replay} paused={!$gafferAnimations} /></button></div>
    <div class="live-card"><GafferProgress {chat} agent="OpenCode" dir="/project" ended={scene.phase === 'ended'} /></div>
  </section>
  <section class="preference"><div><p class="eyebrow">ACCOUNT → PROFILE → APPEARANCE</p><h2>Keep the Gaffer. Quiet the cap.</h2><p>Quotes, live work updates and executive summaries stay on.</p></div><label><span>Gaffer animation</span><select aria-label="Gaffer animation" bind:value={$gafferAnimations}><option value={true}>On — occasional gestures and reactions</option><option value={false}>Off — keep Gaffer still</option></select><small>Your device’s reduced-motion setting is always respected.</small></label></section>
  <footer>Local interactive mock · scripted activity · no agent session is started</footer>
</main>

<style>
  :global(body) { margin: 0; background: var(--bg); }
  main { max-width: 1180px; margin: auto; padding: 28px 42px 24px; color: var(--ink); }
  header { display: flex; align-items: center; gap: 24px; padding-bottom: 25px; border-bottom: 1px solid var(--line); }
  header a { color: var(--ink); text-decoration: none; font: 700 27px var(--sans); letter-spacing: -.05em; } header a span { color: var(--accent-text); }
  .review-tag { color: var(--muted); font: 11px var(--mono); } .theme { margin-left: auto; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--ink-soft); padding: 9px 14px; cursor: pointer; font: 11px var(--sans); }
  .intro-art { display: flex; gap: 18px; flex: none; }.intro-art figure { margin: 0; }.intro-art .intro-avatar { width: 120px; height: 120px; margin: 0; }.intro-art figcaption { text-align: center; color: var(--muted); font: 9px var(--mono); margin-top: 10px; }
  .intro { display: flex; justify-content: space-between; align-items: center; padding: 34px 0 24px; gap: 40px; }.eyebrow { margin: 0 0 12px; font: 10px var(--mono); letter-spacing: .12em; color: var(--accent-text); }h1 { font: 500 46px/1.1 var(--serif); margin: 0 0 17px; letter-spacing: -.025em; }.lede { font: 14px/1.7 var(--sans); color: var(--muted); max-width: 510px; margin: 0; }.intro-avatar { width: 140px; height: 140px; margin-right: 35px; }
  .design-rules { display: flex; gap: 28px; padding: 16px 0 22px; color: var(--ink-soft); font: 11px var(--sans); }.design-rules span { display: flex; gap: 8px; align-items: center; }.design-rules i { width: 6px; height: 6px; border-radius: 50%; background: var(--accent-text); }
  .gallery { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }.scene { text-align: left; border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 17px 18px 15px; color: var(--ink); cursor: pointer; }.scene.selected { border-color: var(--accent-text); background: color-mix(in srgb, var(--accent-text) 4%, var(--card)); }.scene:hover { border-color: var(--muted); }.scene:focus-visible,.big-avatar:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 3px; }.scene-top { display: flex; align-items: center; gap: 10px; min-height: 68px; }.mini-avatar { width: 65px; height: 65px; flex: none; }.number { align-self: flex-start; color: var(--faint); font: 10px var(--mono); }.preview-label { font: 10px/1.5 var(--mono); color: var(--muted); max-width: 130px; }.scene h2 { font: 500 21px var(--serif); margin: 11px 0 9px; }.quip { font: 12px/1.6 var(--sans); min-height: 58px; margin: 0; color: var(--ink-soft); }.annotation { border-top: 1px solid var(--line-soft); padding-top: 11px; margin: 12px 0 0; font: 10px/1.6 var(--sans); color: var(--muted); min-height: 44px; }
  .try-it { margin-top: 28px; border: 1px solid var(--line); border-radius: 12px; padding: 20px 20px 8px; background: var(--bg-sunk); }.try-heading { display: flex; justify-content: space-between; align-items: center; margin-bottom: 17px; }.try-heading h2,.preference h2 { font: 500 24px var(--serif); margin: 0 0 8px; }.try-heading p:not(.eyebrow),.preference p:not(.eyebrow) { margin: 0; color: var(--muted); font: 11px/1.6 var(--sans); }.big-avatar { width: 84px; height: 84px; padding: 0; border: 0; background: none; border-radius: 8px; cursor: pointer; margin-right: 14px; }.live-card :global(.gaffer-progress) { margin: 0 0 10px; }
  .preference { display: flex; justify-content: space-between; align-items: center; gap: 30px; padding: 24px 0; margin-top: 12px; border-bottom: 1px solid var(--line); }.preference label { display: grid; gap: 7px; font: 11px var(--sans); min-width: 330px; }.preference select { padding: 10px; background: var(--card); color: var(--ink); border: 1px solid var(--line); border-radius: 7px; font: 11px var(--sans); }.preference small { font-size: 9px; color: var(--muted); }footer { color: var(--faint); font: 10px var(--mono); padding-top: 18px; }
  @media(max-width: 760px) { main { padding: 20px; }.gallery { grid-template-columns: repeat(2,minmax(0,1fr)); }h1 { font-size: 35px; }.intro-avatar { width: 95px; height: 95px; margin: 0; }.preference { flex-direction: column; align-items: stretch; }.design-rules { gap: 15px; flex-wrap: wrap; }.review-tag { display: none; } }
  @media(max-width: 460px) { .gallery { grid-template-columns: minmax(0,1fr); }.intro { gap: 15px; }h1 { font-size: 30px; }.intro-avatar { width: 72px; flex: none; }.preference label { min-width: 0; }.annotation,.quip { min-height: 0; } }
  @media(max-width: 760px) { .intro { flex-wrap: wrap; }.intro-art .intro-avatar { width: 95px; height: 95px; } }
</style>
