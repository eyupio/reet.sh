<script lang="ts">
  import './gafferAvatar.css'
  let { active = false, paused = false }: { active?: boolean; paused?: boolean } = $props()
</script>

<svg class="gaffer-avatar" class:active class:paused viewBox="0 0 112 112" fill="none" aria-hidden="true">
  <circle cx="56" cy="56" r="54" fill="var(--bg-sunk)" />
  <circle class="halo" cx="56" cy="56" r="51" stroke="var(--accent-text)" stroke-opacity=".25" stroke-dasharray="2 6" />
  <path d="M18 105c1-24 14-34 38-34s37 10 38 34" fill="var(--gaffer-coat)" />
  <path d="m42 74 14 13 14-13-3 31H45z" fill="var(--gaffer-shirt)" />
  <path d="m36 77 6-4 14 14-12 11zM76 77l-6-4-14 14 12 11z" fill="var(--gaffer-collar)" stroke="var(--gaffer-coat-seam)" stroke-width="1.5" stroke-linejoin="round" />
  <path d="M56 89v16" stroke="var(--gaffer-button)" stroke-width="2" />
  <g class="head">
    <ellipse cx="30" cy="55" rx="6" ry="8" fill="var(--gaffer-ear)" />
    <ellipse cx="80" cy="55" rx="6" ry="8" fill="var(--gaffer-ear)" />
    <path d="M31 36c0-13 48-13 48 0v21c0 17-9 26-24 26S31 74 31 57z" fill="var(--gaffer-skin)" />
    <path d="M34 58c0 12 5 22 21 22 10 0 18-4 22-13-3 11-10 16-22 16-15 0-24-9-24-26V41l4 1z" fill="var(--gaffer-skin-shade)" />
    <path d="M29 42c-2-17 5-27 23-27 16 0 28 9 31 23L65 45z" fill="var(--gaffer-cap)" stroke="var(--gaffer-outline)" stroke-width="2" />
    <path d="M31 32c14-9 31-7 47 1M53 16c7 4 12 11 12 20M41 19c6 4 9 9 10 15" stroke="var(--gaffer-cap-line)" stroke-width="1.5" stroke-linecap="round" />
    <path class="brim" d="M26 39c10-6 35-8 52-2l10 5c1 4-14 6-27 5l-33-2c-4 0-5-4-2-6z" fill="var(--gaffer-cap-brim)" stroke="var(--gaffer-outline)" stroke-width="2" />
    <path d="M29 40c15-4 35-4 48 0" stroke="var(--gaffer-cap-stitch)" stroke-opacity=".7" stroke-width="1.5" stroke-linecap="round" />
    <path d="m37 49 9-1m17 0 8 1" stroke="var(--gaffer-brow)" stroke-width="2.5" stroke-linecap="round" />
    <g class="eyes" fill="var(--gaffer-eye)">
      <ellipse cx="42" cy="56" rx="2" ry="2.6" /><ellipse cx="67" cy="56" rx="2" ry="2.6" />
    </g>
    <g stroke="var(--gaffer-glasses)" stroke-width="1.6">
      <rect x="33" y="50" width="17" height="13" rx="5" /><rect x="59" y="50" width="17" height="13" rx="5" /><path d="M50 55c3-2 6-2 9 0M30 52l3 2m43 0 4-2" />
    </g>
    <path d="M53 55 50 65c2 2 5 2 8 0" fill="var(--gaffer-skin-shade)" stroke="var(--gaffer-skin-line)" stroke-width="1.3" stroke-linecap="round" />
    <path d="M54 67c-6-5-12 0-16 4 6 3 12 3 17-1 5 4 12 4 18 0-5-4-11-8-19-3z" fill="var(--gaffer-moustache)" />
    <path d="M50 75c3 2 7 2 10-1" stroke="var(--gaffer-smile)" stroke-width="1.8" stroke-linecap="round" />
  </g>
  <g class="clipboard">
    <rect x="68" y="79" width="23" height="28" rx="3" transform="rotate(8 68 79)" fill="var(--gaffer-board)" stroke="var(--gaffer-outline)" stroke-width="1.5" />
    <path d="m71 84 15 2-3 17-15-2z" fill="var(--gaffer-paper)" />
    <path d="m73 90 7 1m-8 4 7 1" stroke="var(--gaffer-writing)" stroke-width="1.5" stroke-linecap="round" />
    <rect x="75" y="78" width="10" height="5" rx="2" transform="rotate(8 75 78)" fill="var(--gaffer-clip)" />
  </g>
  <g class="pencil">
    <path d="m88 84-9 12" stroke="var(--gaffer-pencil)" stroke-width="3" stroke-linecap="round" />
    <path d="m79 96-1 4 4-2" fill="var(--gaffer-eye)" />
    <path d="M94 99c-3-6-6-8-9-6s-1 6 2 8" fill="var(--gaffer-skin)" stroke="var(--gaffer-skin-line)" stroke-width="1.2" stroke-linecap="round" />
  </g>
</svg>

<style>
  .gaffer-avatar { display: block; width: 100%; height: 100%; overflow: visible; }
  .head { transform-origin: 56px 76px; }
  .eyes { transform-box: fill-box; transform-origin: center; }
  .pencil { transform-origin: 92px 100px; }
  .active .head { animation: nod 5s ease-in-out infinite; }
  .active .eyes { animation: blink 5.7s infinite; }
  .active .pencil { animation: jot 2s ease-in-out infinite; }
  .active .halo { animation: halo 16s linear infinite; transform-origin: 56px 56px; }
  .paused * { animation-play-state: paused !important; }
  @keyframes nod { 0%, 35%, 65%, 100% { transform: rotate(0); } 44%, 55% { transform: rotate(-4deg) translateY(1px); } }
  @keyframes blink { 0%, 43%, 47%, 100% { transform: scaleY(1); } 45% { transform: scaleY(.12); } }
  @keyframes jot { 0%, 55%, 100% { transform: rotate(0); } 65%, 85% { transform: rotate(-7deg); } 75%, 95% { transform: rotate(3deg); } }
  @keyframes halo { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .active .head, .active .eyes, .active .pencil, .active .halo { animation: none; } }
</style>
