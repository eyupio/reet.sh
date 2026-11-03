<script lang="ts">
  import './gafferAvatar.css'
  import gafferHead from './gafferHead.png'
  import { gafferAnimations } from '../gafferMotion'
  import type { GafferPhase } from '../gafferProgress'
  import type { GafferPose, GafferReaction } from '../gafferPersonality'
  const capId = $props.id()
  let { active = false, paused = false, phase = 'working', pose = 'duty', reaction = '', reactionKey = 0 }: {
    active?: boolean; paused?: boolean; phase?: GafferPhase; pose?: GafferPose; reaction?: GafferReaction; reactionKey?: number
  } = $props()
</script>

{#key `${phase}:${pose}:${reaction}:${reactionKey}`}
<svg class="gaffer-avatar" class:active class:paused={paused || !$gafferAnimations} data-phase={phase} data-pose={pose} data-reaction={reaction} viewBox="0 0 64 64" shape-rendering="crispEdges" fill="none" aria-hidden="true">
  <g class="body">
    <g class="head">
      <svg width="64" height="64" overflow="visible" viewBox="190 130 880 1000">
        <image href={gafferHead} width="1254" height="1254" />
        <g class="eyes" fill="var(--gaffer-lid)">
          <g class="eye-left"><path d="M399 619h162v101H399z" /><path d="M399 667h162v18H399z" fill="var(--gaffer-eye-line)" /></g>
          <g class="eye-right"><path d="M694 619h162v101H694z" /><path d="M694 667h162v18H694z" fill="var(--gaffer-eye-line)" /></g>
        </g>
        {#if pose === 'concerned'}
          <path d="M477 790h325v100H477z" fill="var(--gaffer-lid)" />
          <path d="M505 843h220v23H505zM725 866h40v23h-40z" fill="var(--gaffer-eye-line)" />
        {/if}
        <!-- Front-facing cloth crown and centred peak above the brow; the whole cap tips with the head. -->
        <g class="flat-cap">
          <defs>
            <pattern id={`${capId}-tweed`} width="80" height="60" patternUnits="userSpaceOnUse">
              <path d="M0 5h8v8h8v8h8v8h8v8h8v-8h8v-8h8v-8h8v-8h8" stroke="var(--gaffer-stitch)" stroke-width="7" opacity=".28" />
              <path d="M0 39h8v8h8v8h8M56 55h8v-8h8v-8h8" stroke="var(--gaffer-cap-edge)" stroke-width="7" opacity=".18" />
            </pattern>
          </defs>
          <path d="M225 420v-70h20v-50h30v-45h40v-40h50v-30h70v-25h80v-15h220v15h80v25h70v30h50v40h40v45h30v50h20v70h-20v20h-80v10H825v-10H425v10H325v-10h-80v-20z" fill="var(--gaffer-cap-edge)" />
          <path d="M250 410v-55h20v-50h30v-45h40v-35h50v-25h60v-25h70v-15h210v15h70v25h60v25h50v35h40v45h30v50h20v55h-20v15h-65v10h-90v-10H425v10h-90v-10h-65v-15z" fill="var(--gaffer-cap)" />
          <path d="M270 345v-40h30v-45h40v-35h50v-25h60v-25h70v-15h70v20h-55v25h-45v30h-40v40h-40v40h-30v45h-15v35h-95z" fill="var(--gaffer-tweed)" opacity=".65" />
          <path d="M650 165h80v15h70v25h60v25h50v35h40v45h30v50h20v50h-20v15h-65v10h-90v-10h-35v-40h-25v-45h-30v-45h-30v-40h-30v-40h-25z" fill="var(--gaffer-cap-shadow)" opacity=".65" />
          <path d="M250 410v-55h20v-50h30v-45h40v-35h50v-25h60v-25h70v-15h210v15h70v25h60v25h50v35h40v45h30v50h20v55h-20v15h-65v10h-90v-10H425v10h-90v-10h-65v-15z" fill={`url(#${capId}-tweed)`} />
          <path d="M625 165v255" stroke="var(--gaffer-seam)" stroke-width="10" opacity=".6" />
          <path d="M635 185v220" stroke="var(--gaffer-stitch)" stroke-width="6" opacity=".2" />
          <path d="M270 410h65v10h90v-5h400v5h90v-10h65v15h-65v10h-90v-5H425v5h-90v-10h-65z" fill="var(--gaffer-brim)" />
          <g class="cap-peak">
            <path d="M355 425h540v10h30v20h-20v20h-50v15h-80v10H475v-10h-80v-15h-50v-20h-20v-20h30z" fill="var(--gaffer-cap-edge)" />
            <path d="M375 435h500v10h30v10h-20v10h-50v10h-80v10H495v-10h-80v-10h-50v-10h-20v-10h30z" fill="var(--gaffer-brim-light)" />
            <path d="M365 455h50v10h80v10h260v-10h80v-10h50v10h-30v15h-80v10H475v-10h-80v-15h-30z" fill="var(--gaffer-brim)" />
            <path d="M415 450h420v8H415z" fill="var(--gaffer-stitch)" opacity=".4" />
            <path d="M600 425h50v20h-50z" fill="var(--gaffer-cap)" />
          </g>
        </g>
      </svg>
    </g>
    {#if pose === 'tea'}
      <g class="mug" stroke="var(--gaffer-prop-ink)" stroke-width="1.5">
        <path d="M49 46h8v3h-8z" fill="var(--gaffer-tea)" /><path d="M57 48h4v6h-4" />
        <path d="M46 47h11v12H46z" fill="var(--gaffer-paper)" /><path d="M49 50h5v5h-5z" fill="var(--gaffer-mug-mark)" stroke="none" />
        <path d="M43 61h17" /><path d="M49 43v-3h2v-3M54 44v-4" stroke="var(--gaffer-steam)" />
      </g>
    {:else if pose === 'asleep'}
      <path class="zzz" d="M51 8h6l-6 6h6M56 1h6l-6 5h6" stroke="var(--accent-text)" stroke-width="1.5" />
    {:else if pose === 'concerned'}
      <g class="snag"><path d="m53 39 10 19H43z" fill="var(--gaffer-warning)" stroke="var(--gaffer-prop-edge)" /><path d="M53 45v6m0 2v2" stroke="var(--gaffer-eye-line)" stroke-width="2" /></g>
    {:else if pose === 'attention'}
      <g class="question"><path d="M46 42h16v15H52l-5 4v-4h-1z" fill="var(--gaffer-paper)" stroke="var(--gaffer-prop-edge)" /><path d="M51 47v-2h6v4h-3v3m0 1v2" stroke="var(--gaffer-prop-edge)" stroke-width="1.5" /></g>
    {:else if phase === 'reading'}
      <g class="glasses" stroke="var(--gaffer-prop-edge)" stroke-width="1.5"><path d="M13 32h14v9H13zM34 32h14v9H34zM27 35h7M9 33h4M48 33h4" /></g>
      <path d="M43 49h18v13H43z" fill="var(--gaffer-paper)" stroke="var(--gaffer-prop-edge)" /><path d="M47 53h10m-10 4h8" stroke="var(--gaffer-tweed)" />
    {:else if phase === 'searching' || phase === 'fetching'}
      <g class="magnifier" stroke="var(--gaffer-prop-edge)" stroke-width="2.5"><circle cx="52" cy="47" r="7" fill="var(--gaffer-glass)" fill-opacity=".8" /><path d="m56 53 6 8" /></g>
    {:else if phase === 'editing'}
      <g class="pencil"><path d="m49 58 9-19 4 2-9 19-5 3z" fill="var(--gaffer-pencil)" stroke="var(--gaffer-prop-edge)" /><path d="m58 39 1-3 4 2-1 3" fill="var(--gaffer-eraser)" /><path d="m48 63 2-5 3 2z" fill="var(--gaffer-prop-ink)" /></g>
    {:else if phase === 'running'}
      <g class="watch"><path d="M52 40v-3m-3 0h6" stroke="var(--gaffer-prop-edge)" stroke-width="2" /><circle cx="52" cy="51" r="10" fill="var(--gaffer-paper)" stroke="var(--gaffer-prop-edge)" stroke-width="2" /><path class="watch-hand" d="M52 44v7l4 3" stroke="var(--gaffer-prop-edge)" stroke-width="1.5" /></g>
    {:else if phase === 'responding'}
      <g class="notes"><path d="M44 42h16v20H44z" fill="var(--gaffer-paper)" stroke="var(--gaffer-prop-edge)" /><path d="M47 47h10m-10 4h10m-10 4h7" stroke="var(--gaffer-tweed)" /></g>
    {:else if phase === 'planning'}
      <g class="thought" fill="var(--accent-text)"><path d="M49 12h4v4h-4zM56 5h5v5h-5zM49 20h2v2h-2z" /></g>
    {/if}
  </g>
</svg>
{/key}

<style>
  .gaffer-avatar { display: block; width: 100%; height: 100%; overflow: visible; }
  .head image { image-rendering: pixelated; }
  .body { transform-origin: 32px 55px; }
  .head { transform-origin: 32px 48px; }
  .flat-cap { transform-origin: 360px 420px; }
  /* Only a brief blink loops. Gestures play once, followed by a long, irregular rest. */
  .eye-left, .eye-right { opacity: 0; animation: blink 12.7s steps(1, end) infinite; }
  [data-pose='asleep'] .head { transform: translateY(2px) rotate(8deg); }
  [data-pose='asleep'] .eye-left, [data-pose='asleep'] .eye-right { opacity: 1; animation: none; }
  [data-pose='tea'] .head { transform: rotate(-4deg); }
  [data-pose='attention'] .head { transform: rotate(-3deg); }
  [data-pose='concerned'] .head { transform: rotate(3deg); }
  [data-reaction='ponder'] .head { animation: ponder 1.8s ease-in-out; }
  [data-reaction='scan'] .head { animation: scan 1.8s ease-in-out; }
  [data-reaction='scribble'] .pencil { transform-origin: 51px 56px; animation: scribble 1.4s ease-in-out; }
  [data-reaction='watch'] .watch-hand { transform-origin: 52px 51px; animation: clock-check 1.5s ease-in-out; }
  [data-reaction='watch'] .head { animation: nod 1.5s ease-in-out; }
  [data-reaction='nod'] .head { animation: nod 1.2s ease-in-out; }
  [data-reaction='listen'] .head { animation: listen 1.4s ease-in-out; }
  [data-reaction='shake'] .head { animation: shake 1.1s ease-in-out; }
  [data-reaction='tip-cap'] .flat-cap { animation: cap-tip 1.3s ease-in-out; }
  [data-reaction='wink'] .eye-right { animation: wink 1s steps(1, end); }
  [data-reaction='sip'] .mug { animation: sip 1.6s ease-in-out; }
  [data-reaction='startle'] .body { animation: startle 1.4s ease-out; }
  [data-reaction='stretch'] .head { animation: stretch 2s ease-in-out; }
  .paused * { animation: none !important; }
  @keyframes blink { 0%, 95%, 97%, 100% { opacity: 0; } 96% { opacity: 1; } }
  @keyframes wink { 0%, 25%, 65%, 100% { opacity: 0; } 30%, 60% { opacity: 1; } }
  @keyframes ponder { 35%, 65% { transform: rotate(-6deg) translateY(-1px); } }
  @keyframes scan { 25% { transform: translateX(-1px) rotate(-3deg); } 65% { transform: translateX(1px) rotate(3deg); } }
  @keyframes nod { 35%, 65% { transform: translateY(1.5px) rotate(2deg); } }
  @keyframes listen { 40%, 70% { transform: rotate(-7deg); } }
  @keyframes shake { 25%, 65% { transform: rotate(-3deg); } 45% { transform: rotate(4deg); } }
  @keyframes cap-tip { 35%, 65% { transform: translateY(-65px) rotate(-9deg); } }
  @keyframes scribble { 20%, 60% { transform: rotate(-8deg); } 40%, 80% { transform: rotate(5deg); } }
  @keyframes clock-check { 60% { transform: rotate(30deg); } }
  @keyframes sip { 40%, 65% { transform: translate(-5px, -8px) rotate(-8deg); } }
  @keyframes startle { 20% { transform: translateY(-3px) rotate(-4deg); } 45% { transform: translateY(1px) rotate(2deg); } 70% { transform: translateY(-1px); } }
  @keyframes stretch { 0% { transform: translateY(2px) rotate(8deg); } 35% { transform: translateY(-2px) rotate(-6deg); } 65% { transform: translateY(-1px) rotate(3deg); } }
  @media (prefers-reduced-motion: reduce) { .gaffer-avatar * { animation: none !important; } }
</style>
