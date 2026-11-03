<script lang="ts">
  import './gafferAvatar.css'
  import gafferHead from './gafferHead.png'
  import type { GafferPhase } from '../gafferProgress'
  let { active = false, paused = false, phase = 'working' }: { active?: boolean; paused?: boolean; phase?: GafferPhase } = $props()
  const attention = $derived(phase === 'approval' || phase === 'question')
  const concerned = $derived(['error', 'cancelled', 'reconnecting'].includes(phase))
  const finished = $derived(phase === 'idle' || phase === 'ended')
</script>

<svg class="gaffer-avatar" class:active class:paused class:attention class:concerned class:finished data-phase={phase} viewBox="0 0 64 64" shape-rendering="crispEdges" fill="none" aria-hidden="true">
  <g fill="var(--accent-text)">
    <path class="spark spark-one" d="M1 20h2v2H1zM3 18h1v1H3z" />
    <path class="spark spark-two" d="M61 42h2v2h-2zM60 46h1v1h-1z" />
  </g>
  <g class="body">
    <g class="head">
      <!-- A finer block portrait, with eyelids fitted to its pixel eye shapes. -->
      <svg width="64" height="64" viewBox="190 130 880 1000">
        <image href={gafferHead} width="1254" height="1254" />
        <g class="eyes" fill="var(--gaffer-lid)">
          <g class="eye-left">
            <path d="M399 619h162v101H399z" />
            <path d="M399 667h162v18H399z" fill="var(--gaffer-eye-line)" />
          </g>
          <g class="eye-right">
            <path d="M694 619h162v101H694z" />
            <path d="M694 667h162v18H694z" fill="var(--gaffer-eye-line)" />
          </g>
        </g>
      </svg>
    </g>
  </g>
</svg>

<style>
  .gaffer-avatar { display: block; width: 100%; height: 100%; overflow: visible; }
  .head image { image-rendering: pixelated; }
  .body { transform-origin: 32px 60px; }
  .head { transform-origin: 32px 48px; animation: look-about 7s steps(1, end) infinite; }
  .eye-left, .eye-right { opacity: 0; animation: blink 4.1s steps(1, end) infinite; }
  .spark { opacity: 0; }
  .active .body { animation: bustle 1.6s steps(1, end) infinite; }
  .active .head { animation: nod 2.7s steps(1, end) infinite; }
  .active .eye-right { animation: wink 6.3s steps(1, end) infinite; }
  .active .spark { animation: sparkle 3.2s steps(1, end) infinite; }
  .active .spark-two { animation-delay: 1.6s; }
  [data-phase='planning'] .head { animation: ponder 3s steps(1, end) infinite; }
  [data-phase='searching'] .head, [data-phase='reading'] .head { animation: look-about 2.6s steps(1, end) infinite; }
  [data-phase='responding'] .head { animation: nod 1.4s steps(1, end) infinite; }
  .attention .head { animation: ponder 5s steps(1, end) infinite; }
  .concerned .head { animation: ponder 7s steps(1, end) infinite; }
  .finished .head { animation: pleased 4s steps(1, end) infinite; }
  .finished .eye-right { animation: wink 5s steps(1, end) infinite; }
  .paused * { animation-play-state: paused !important; }
  @keyframes bustle { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-1px); } }
  @keyframes look-about { 0%, 65%, 100% { transform: translateX(0); } 75%, 90% { transform: translateX(1px); } }
  @keyframes nod { 0%, 100% { transform: translateY(0); } 35% { transform: translateY(1px); } 65% { transform: translateY(-1px); } }
  @keyframes ponder { 0%, 100% { transform: translate(0, 0); } 50% { transform: translate(1px, -1px); } }
  @keyframes pleased { 0%, 70%, 100% { transform: translateY(0); } 78%, 92% { transform: translateY(1px); } 85% { transform: translateY(-1px); } }
  @keyframes blink { 0%, 41%, 45%, 100% { opacity: 0; } 42%, 44% { opacity: 1; } }
  @keyframes wink { 0%, 72%, 80%, 100% { opacity: 0; } 75%, 77% { opacity: 1; } }
  @keyframes sparkle { 0%, 65%, 100% { opacity: 0; } 78%, 88% { opacity: .65; } }
  @media (prefers-reduced-motion: reduce) { .gaffer-avatar * { animation: none !important; } }
</style>
