<script lang="ts">
  import './gafferAvatar.css'
  import grid from './gaffer-grid.webp'
  import phosphor from './gaffer-phosphor.webp'
  import gestures from './gaffer-gestures.webp'
  import { GAFFER_GESTURE_MS, gafferAnimations } from '../gafferMotion'
  import type { GafferPhase } from '../gafferProgress'
  import type { GafferPose, GafferReaction } from '../gafferPersonality'

  const id = $props.id()
  let { active = false, paused = false, phase = 'working', pose = 'duty', reaction = '', reactionKey = 0, variant = 'portrait' }: {
    active?: boolean; paused?: boolean; phase?: GafferPhase; pose?: GafferPose; reaction?: GafferReaction; reactionKey?: number
    variant?: 'portrait' | 'bust'
  } = $props()
  // Requests and snags outrank decorative tea breaks and greetings, including in standalone uses.
  const concern = $derived(pose === 'concerned' || ['error', 'cancelled', 'connecting', 'reconnecting'].includes(phase))
  const attention = $derived(!concern && (pose === 'attention' || phase === 'approval' || phase === 'question'))
  const scene = $derived(concern || attention || pose === 'asleep' ? variant : pose === 'tea' || reaction === 'sip' ? 'tea' : reaction === 'tip-cap' ? 'greeting' : phase === 'reading' ? 'reading' : variant)
  const sprite = $derived(scene === 'greeting' || scene === 'reading' || scene === 'tea')
  const row = $derived(scene === 'reading' ? 1 : scene === 'tea' ? 2 : 0)
  const play = $derived(sprite && (scene === 'greeting' || scene === 'tea' || reaction === 'scan'))
</script>

{#key `${phase}:${pose}:${reaction}:${reactionKey}:${variant}`}
<svg class="gaffer-avatar" class:active class:paused={paused || !$gafferAnimations} data-phase={phase} data-pose={pose} data-reaction={reaction} data-artwork={scene} style:--gaffer-gesture-duration={`${GAFFER_GESTURE_MS}ms`} viewBox="0 0 64 64" fill="none" aria-hidden="true">
  <defs><clipPath id={`${id}-frame`}><rect width="384" height="341.333333" /></clipPath></defs>
  <rect class="screen" width="64" height="64" rx="6" />
  <g class="body">
    <g class="head" class:sprite class:concern class:attention>
      {#if sprite}
        <svg x="0" y="3.555556" width="64" height="56.888889" viewBox="0 0 384 341.333333" overflow="hidden">
          <g clip-path={`url(#${id}-frame)`}>
            <image class="frames" class:play href={gestures} width="1536" height="1024" y={-row * 1024 / 3} />
          </g>
        </svg>
      {:else}
        <image href={scene === 'portrait' ? grid : phosphor} x="2" y="2" width="60" height="60" preserveAspectRatio="xMidYMid meet" />
      {/if}
    </g>
    <!-- The surrounding work card carries the semantic status colour and text. -->
    {#if concern}
      <g class="badge snag"><path d="m53 39 10 20H43z" /><path d="M53 45v6m0 3v1" class="ink" /></g>
    {:else if attention}
      <g class="badge question"><path d="M45 41h17v15H52l-5 4v-4h-2z" /><path class="ink" d="M50 46v-1h7v5h-3v2m0 1v2" /></g>
    {:else if pose === 'asleep'}
      <path class="zzz" d="M50 10h6l-6 6h6M56 3h6l-6 5h6" />
    {:else if !sprite && (phase === 'searching' || phase === 'fetching')}
      <g class="badge magnifier"><circle cx="52" cy="47" r="7" /><path d="m57 53 5 8" /></g>
    {:else if !sprite && phase === 'editing'}
      <g class="badge pencil"><path d="m49 58 9-19 4 2-9 19-5 3z" /><path class="ink" d="m48 63 2-5 3 2" /></g>
    {:else if !sprite && phase === 'running'}
      <g class="badge watch"><path d="M52 40v-3m-3 0h6" /><circle cx="52" cy="51" r="10" /><path class="watch-hand ink" d="M52 44v7l4 3" /></g>
    {:else if !sprite && phase === 'responding'}
      <g class="badge notes"><path d="M44 42h16v20H44z" /><path class="ink" d="M47 47h10m-10 4h10m-10 4h7" /></g>
    {:else if !sprite && phase === 'planning'}
      <path class="thought" d="M49 12h4v4h-4zM56 5h5v5h-5zM49 20h2v2h-2z" />
    {/if}
  </g>
</svg>
{/key}

<style>
  .gaffer-avatar { display: block; width: 100%; height: 100%; overflow: hidden; border-radius: 6px; }
  .screen { fill: var(--gaffer-screen); }
  .body { transform-origin: 32px 52px; }
  .head { transform-origin: 32px 44px; }
  .badge { fill: var(--gaffer-paper); stroke: var(--gaffer-paper); stroke-width: 1.5; stroke-linejoin: round; }
  .badge .ink { fill: none; stroke: var(--gaffer-ink); }
  .magnifier { fill: var(--gaffer-screen); stroke-width: 2.5; }
  .zzz { stroke: var(--gaffer-paper); stroke-width: 1.5; }
  .thought { fill: var(--gaffer-paper); }
  /* Four approved frames play once, then rest. CSS keeps pause and reduced motion effective. */
  .frames.play { animation: gesture-frames var(--gaffer-gesture-duration) steps(1, end); }
  [data-pose='asleep'] .head { transform: translateY(2px) rotate(8deg); }
  .head.attention { transform: rotate(-3deg); }
  .head.concern { transform: rotate(3deg); }
  [data-reaction='ponder'] .head:not(.sprite) { animation: ponder 1.8s ease-in-out; }
  [data-reaction='scan'] .head:not(.sprite) { animation: scan 1.8s ease-in-out; }
  [data-reaction='scribble'] .pencil { transform-origin: 51px 56px; animation: scribble 1.4s ease-in-out; }
  [data-reaction='watch'] .watch-hand { transform-origin: 52px 51px; animation: clock-check 1.5s ease-in-out; }
  [data-reaction='nod'] .head:not(.sprite), [data-reaction='watch'] .head { animation: nod 1.2s ease-in-out; }
  [data-reaction='listen'] .head { animation: listen 1.4s ease-in-out; }
  [data-reaction='shake'] .head { animation: shake 1.1s ease-in-out; }
  [data-reaction='startle'] .body { animation: startle 1.4s ease-out; }
  [data-reaction='stretch'] .head { animation: stretch 2s ease-in-out; }
  .paused * { animation: none !important; }
  @keyframes gesture-frames { 0%, 100% { transform: translateX(0); } 25% { transform: translateX(-384px); } 50% { transform: translateX(-768px); } 75% { transform: translateX(-1152px); } }
  @keyframes ponder { 35%, 65% { transform: rotate(-6deg) translateY(-1px); } }
  @keyframes scan { 25% { transform: translateX(-1px) rotate(-3deg); } 65% { transform: translateX(1px) rotate(3deg); } }
  @keyframes nod { 35%, 65% { transform: translateY(1.5px) rotate(2deg); } }
  @keyframes listen { 40%, 70% { transform: rotate(-7deg); } }
  @keyframes shake { 25%, 65% { transform: rotate(-3deg); } 45% { transform: rotate(4deg); } }
  @keyframes scribble { 20%, 60% { transform: rotate(-8deg); } 40%, 80% { transform: rotate(5deg); } }
  @keyframes clock-check { 60% { transform: rotate(30deg); } }
  @keyframes startle { 20% { transform: translateY(-3px) rotate(-4deg); } 45% { transform: translateY(1px) rotate(2deg); } 70% { transform: translateY(-1px); } }
  @keyframes stretch { 0% { transform: translateY(2px) rotate(8deg); } 35% { transform: translateY(-2px) rotate(-6deg); } 65% { transform: translateY(-1px) rotate(3deg); } }
  @media (prefers-reduced-motion: reduce) { .gaffer-avatar * { animation: none !important; } }
</style>
