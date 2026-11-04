<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte'
  import './gafferAvatar.css'
  import grid from './gaffer-grid.webp'
  import phosphor from './gaffer-phosphor.webp'
  import gestures from './gaffer-gestures.webp'
  import expressions from './gaffer-expressions.webp'
  import { GAFFER_TRANSITION_MS, gafferAnimations, gafferGestureMs } from '../gafferMotion'
  import type { GafferPhase } from '../gafferProgress'
  import type { GafferPose, GafferReaction } from '../gafferPersonality'

  const id = $props.id()
  let { active = false, paused = false, phase = 'working', pose = 'duty', reaction = '', reactionKey = 0, variant = 'portrait', pokeKind = '', pokeKey = 0 }: {
    active?: boolean; paused?: boolean; phase?: GafferPhase; pose?: GafferPose; reaction?: GafferReaction; reactionKey?: number
    variant?: 'portrait' | 'bust'
    pokeKind?: 'flinch' | 'bob' | 'sway' | ''
    pokeKey?: number
  } = $props()
  // Requests and snags outrank decorative tea breaks and greetings, including in standalone uses.
  const concern = $derived(pose === 'concerned' || ['error', 'cancelled', 'connecting', 'reconnecting'].includes(phase))
  const attention = $derived(!concern && (pose === 'attention' || phase === 'approval' || phase === 'question'))
  const scene = $derived(concern || attention || pose === 'asleep' ? variant : pose === 'tea' || reaction === 'sip' ? 'tea' : reaction === 'tip-cap' ? 'greeting' : reaction === 'biscuit' || reaction === 'shrug' || reaction === 'huff' ? reaction : phase === 'reading' && (!reaction || reaction === 'scan') ? 'reading' : variant)
  const expressive = $derived(scene === 'biscuit' || scene === 'shrug' || scene === 'huff')
  const sprite = $derived(expressive || scene === 'greeting' || scene === 'reading' || scene === 'tea')
  const row = $derived(scene === 'reading' || scene === 'shrug' ? 1 : scene === 'tea' || scene === 'huff' ? 2 : 0)
  const play = $derived(sprite && (expressive || scene === 'greeting' || scene === 'tea' || reaction === 'scan'))
  const motion = $derived(scene === 'greeting' ? 'tip-cap' : scene === 'tea' ? 'sip' : scene === 'reading' ? 'scan' : expressive ? scene as GafferReaction : reaction)
  const appearance = $derived({ key: `${phase}:${pose}:${reaction}:${reactionKey}:${variant}`, phase, pose, reaction, scene, expressive, sprite, row, play, concern, attention, duration: gafferGestureMs(motion) })
  type Layer = typeof appearance & { entering: boolean; leaving: boolean; from: number; alternate: boolean }
  let layers = $state<Layer[]>([]), mounted = $state(false), reduced = $state(false), hidden = $state(false)
  const still = $derived(paused || !$gafferAnimations || reduced || hidden)
  const sceneNodes = new Map<string, SVGGElement>()
  let transitionTimer: ReturnType<typeof setTimeout> | undefined

  function registerScene(node: SVGGElement, key: string) {
    sceneNodes.set(key, node)
    return { destroy() { if (sceneNodes.get(key) === node) sceneNodes.delete(key) } }
  }
  $effect(() => {
    const next = appearance, instant = still || !mounted || concern || attention
    untrack(() => {
      clearTimeout(transitionTimer)
      if (instant || !layers.length || layers.at(-1)?.key === next.key) {
        layers = [{ ...next, entering: false, leaving: false, from: 1, alternate: false }]
        return
      }
      // Preserve the actual opacity when a second state arrives during a blend; never flash back to full ink.
      const previous = layers.filter(layer => layer.key !== next.key).map(layer => {
        const node = sceneNodes.get(layer.key)
        return { ...layer, entering: false, leaving: true, from: node ? Number(getComputedStyle(node).opacity) : layer.entering ? 0 : 1, alternate: !layer.alternate }
      }).filter(layer => layer.from > .025)
      const keep = new Set([...previous].sort((a, b) => b.from - a.from).slice(0, 3).map(layer => layer.key))
      const returning = sceneNodes.get(next.key)
      layers = [...previous.filter(layer => keep.has(layer.key)), { ...next, entering: true, leaving: false, from: returning ? Number(getComputedStyle(returning).opacity) : 0, alternate: false }]
      transitionTimer = setTimeout(() => { layers = [{ ...next, entering: false, leaving: false, from: 1, alternate: false }] }, GAFFER_TRANSITION_MS)
    })
  })
  onMount(() => {
    const media = matchMedia('(prefers-reduced-motion: reduce)')
    const updateMotion = () => { reduced = media.matches }
    const updateVisibility = () => { hidden = document.hidden }
    updateMotion(); updateVisibility(); mounted = true
    media.addEventListener('change', updateMotion)
    document.addEventListener('visibilitychange', updateVisibility)
    return () => {
      media.removeEventListener('change', updateMotion)
      document.removeEventListener('visibilitychange', updateVisibility)
    }
  })
  onDestroy(() => clearTimeout(transitionTimer))

  // A nudge is answered in whatever scene is already playing: a quick physical acknowledgement on a wrapper around every layer, so
  // the artwork, pose and gesture underneath are never swapped or interrupted. Translations only: a person flinches, nods and glances;
  // they do not tip sideways like a board. The Web Animations API restarts cleanly on a second nudge.
  const POKES: Record<'flinch' | 'bob' | 'sway', { duration: number; easing: string; frames: Keyframe[] }> = {
    flinch: { duration: 620, easing: 'ease-out', frames: [{ transform: 'translateY(0) scale(1)' }, { transform: 'translateY(-1.8px) scale(1.02)', offset: .22 }, { transform: 'translateY(.5px) scale(1)', offset: .55 }, { transform: 'translateY(0) scale(1)' }] },
    bob: { duration: 820, easing: 'ease-in-out', frames: [{ transform: 'translateY(0)' }, { transform: 'translateY(1.6px)', offset: .22 }, { transform: 'translateY(0)', offset: .45 }, { transform: 'translateY(1.1px)', offset: .68 }, { transform: 'translateY(0)' }] },
    sway: { duration: 860, easing: 'ease-in-out', frames: [{ transform: 'translateX(0)' }, { transform: 'translateX(-1.5px)', offset: .3 }, { transform: 'translateX(1.2px)', offset: .62 }, { transform: 'translateX(0)' }] },
  }
  let pokeEl = $state<SVGGElement>()
  let lastPoke = untrack(() => pokeKey)
  $effect(() => {
    const key = pokeKey, kind = pokeKind, el = pokeEl, off = still
    if (!key || key === lastPoke) return
    untrack(() => {
      lastPoke = key
      if (off || !el || !kind) return
      const poke = POKES[kind]
      el.animate(poke.frames, { duration: poke.duration, easing: poke.easing })
    })
  })
</script>

<svg class="gaffer-avatar" class:active class:paused={still} data-phase={phase} data-pose={pose} data-reaction={reaction} data-artwork={scene} style:--gaffer-transition-duration={`${GAFFER_TRANSITION_MS}ms`} viewBox="0 0 64 64" fill="none" aria-hidden="true">
  <defs>
    <clipPath id={`${id}-frame`}><rect width="384" height="341.333333" /></clipPath>
    <filter id={`${id}-ink`} color-interpolation-filters="sRGB">
      <!-- Extract the luminous drawing as alpha, dropping the baked-in screen and its dark scanline noise. -->
      <feColorMatrix in="SourceGraphic" type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  .2445 .8225 .083 0 -.1" result="drawing" />
      <feFlood flood-color="currentColor" result="ink" />
      <feComposite in="ink" in2="drawing" operator="in" />
    </filter>
    <!-- The artwork is a lit face on a dark screen. On a light card the same drawing in dark ink would read as a negative, so the
         light theme inks the *features* instead: close the drawing over its grid lines to find the figure, then keep what the
         figure is not (outline, grid, eyes, mouth). A light face with dark line-work, as dark mode reads. The sprite sheet is
         drawn at its own pixel size and the flat portrait is stretched, hence one radius for each. -->
    {#each [['s', 6], ['f', 1.6]] as [kind, radius] (kind)}
      <filter id={`${id}-paper-${kind}`} color-interpolation-filters="sRGB" x="-5%" y="-5%" width="110%" height="110%">
        <feColorMatrix in="SourceGraphic" type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  .2445 .8225 .083 0 -.1" result="soft" />
        <feComponentTransfer in="soft" result="drawing"><feFuncA type="linear" slope="4" intercept="-1.1" /></feComponentTransfer>
        <feMorphology in="drawing" operator="dilate" {radius} result="grown" />
        <feMorphology in="grown" operator="erode" {radius} result="figure" />
        <feComposite in="figure" in2="drawing" operator="out" result="linework" />
        <feFlood flood-color="currentColor" result="ink" />
        <feComposite in="ink" in2="linework" operator="in" />
      </filter>
    {/each}
  </defs>
  <g class="poke" bind:this={pokeEl} data-poke={pokeKind || undefined}>
  {#each layers as layer (layer.key)}
  <g class="scene" class:entering={layer.entering} class:leaving={layer.leaving} class:alternate={layer.alternate} class:performing={layer.play} data-reaction={layer.reaction} data-pose={layer.pose} data-artwork={layer.scene} style:--gaffer-fade-from={layer.from} style:--gaffer-gesture-duration={`${layer.duration}ms`} use:registerScene={layer.key}>
  <g class="body">
    <g class="head" class:sprite={layer.sprite} class:concern={layer.concern} class:attention={layer.attention}>
      {#if layer.sprite}
        <svg x={layer.expressive ? -9 : -7.6} y="2" width="76.8" height="68.266667" viewBox="0 0 384 341.333333" overflow="hidden">
          <g clip-path={`url(#${id}-frame)`}>
            <image class="frames" class:play={layer.play} href={layer.expressive ? expressions : gestures} style:--gaffer-ink-filter={`url(#${id}-ink)`} style:--gaffer-paper-filter={`url(#${id}-paper-s)`} width="1536" height="1024" y={-layer.row * 1024 / 3} />
          </g>
        </svg>
      {:else}
        <image href={layer.scene === 'portrait' ? grid : phosphor} style:--gaffer-ink-filter={`url(#${id}-ink)`} style:--gaffer-paper-filter={`url(#${id}-paper-f)`} x={layer.scene === 'portrait' ? 2 : 6} y={layer.scene === 'portrait' ? 2 : 6} width={layer.scene === 'portrait' ? 60 : 52} height={layer.scene === 'portrait' ? 60 : 52} preserveAspectRatio="xMidYMid meet" />
      {/if}
    </g>
    <!-- The surrounding work card carries the semantic status colour and text. -->
    {#if layer.concern}
      <g class="badge snag"><path d="m53 39 10 20H43z" /><path d="M53 45v6m0 3v1" class="ink" /></g>
    {:else if layer.attention}
      <g class="badge question"><path d="M45 41h17v15H52l-5 4v-4h-2z" /><path class="ink" d="M50 46v-1h7v5h-3v2m0 1v2" /></g>
    {:else if layer.pose === 'asleep'}
      <path class="zzz" d="M50 10h6l-6 6h6M56 3h6l-6 5h6" />
    {:else if !layer.sprite && (layer.phase === 'searching' || layer.phase === 'fetching')}
      <g class="badge magnifier"><circle cx="52" cy="47" r="7" /><path d="m57 53 5 8" /></g>
    {:else if !layer.sprite && layer.phase === 'editing'}
      <g class="badge pencil"><path d="m49 58 9-19 4 2-9 19-5 3z" /><path class="ink" d="m48 63 2-5 3 2" /></g>
    {:else if !layer.sprite && layer.phase === 'running'}
      <g class="badge watch"><path d="M52 40v-3m-3 0h6" /><circle cx="52" cy="51" r="10" /><path class="watch-hand ink" d="M52 44v7l4 3" /></g>
    {:else if !layer.sprite && layer.phase === 'responding'}
      <g class="badge notes"><path d="M44 42h16v20H44z" /><path class="ink" d="M47 47h10m-10 4h10m-10 4h7" /></g>
    {:else if !layer.sprite && layer.phase === 'planning'}
      <path class="thought" d="M49 12h4v4h-4zM56 5h5v5h-5zM49 20h2v2h-2z" />
    {/if}
  </g>
  </g>
  {/each}
  </g>
</svg>

<style>
  .gaffer-avatar { display: block; width: 100%; height: 100%; overflow: visible; isolation: isolate; }
  .poke { transform-origin: 32px 52px; }
  .scene { mix-blend-mode: plus-lighter; }
  .scene.entering { animation: scene-in var(--gaffer-transition-duration) ease-out; }
  .scene.leaving { animation: scene-out var(--gaffer-transition-duration) ease-out forwards; }
  /* Restart only the fade when interrupted, keeping the existing pose and body animation. */
  .scene.leaving.alternate { animation-name: scene-out-alternate; }
  .body { transform-origin: 32px 52px; }
  .head { transform-origin: 32px 44px; }
  .badge { fill: var(--gaffer-paper); stroke: var(--gaffer-paper); stroke-width: 1.5; stroke-linejoin: round; }
  .badge .ink { fill: none; stroke: var(--gaffer-ink); }
  .magnifier { fill: var(--gaffer-screen); stroke-width: 2.5; }
  .zzz { stroke: var(--gaffer-paper); stroke-width: 1.5; }
  .thought { fill: var(--gaffer-paper); }
  /* Anticipation, action, a held expression, then settling; the last frame is a still resting pose. */
  .frames { transform: translateX(-1152px); }
  .frames.play { animation: gesture-frames var(--gaffer-gesture-duration) steps(1, end); }
  .scene.performing .body { animation: bust-motion var(--gaffer-gesture-duration) cubic-bezier(.37, 0, .25, 1); }
  /* The sprite is the whole bust, shoulders included, so nothing here rotates it: a person nods, leans and glances, they do not
     tip over like a rigid board. Sleep is a slump. */
  .scene[data-pose='asleep'] .head { transform: translateY(3px); }
  .scene[data-reaction='ponder'] .head:not(.sprite) { animation: ponder var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='scan'] .head:not(.sprite) { animation: scan var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='scribble'] .pencil { transform-origin: 51px 56px; animation: scribble var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='watch'] .watch-hand { transform-origin: 52px 51px; animation: clock-check var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='nod'] .head:not(.sprite), .scene[data-reaction='watch'] .head { animation: nod var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='listen'] .head { animation: listen var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='shake'] .head { animation: shake var(--gaffer-gesture-duration) ease-in-out; }
  .scene[data-reaction='startle'] .body { animation: startle var(--gaffer-gesture-duration) ease-out; }
  .scene[data-reaction='stretch'] .head { animation: stretch var(--gaffer-gesture-duration) ease-in-out; }
  .paused * { animation: none !important; }
  @keyframes scene-in { from { opacity: var(--gaffer-fade-from); } to { opacity: 1; } }
  @keyframes scene-out { from { opacity: var(--gaffer-fade-from); } to { opacity: 0; } }
  @keyframes scene-out-alternate { from { opacity: var(--gaffer-fade-from); } to { opacity: 0; } }
  @keyframes gesture-frames { 0%, 14% { transform: translateX(0); } 20% { transform: translateX(-384px); } 44% { transform: translateX(-768px); } 72%, 100% { transform: translateX(-1152px); } }
  @keyframes bust-motion { 0%, 100% { transform: none; } 12% { transform: translateY(.4px); } 38% { transform: translateY(-.4px); } 68% { transform: translateY(.3px); } 88% { transform: none; } }
  @keyframes ponder { 35%, 65% { transform: translate(-.8px, -1px); } }
  @keyframes scan { 25% { transform: translateX(-1px); } 65% { transform: translateX(1px); } }
  @keyframes nod { 35%, 65% { transform: translateY(1.5px); } }
  @keyframes listen { 40%, 70% { transform: translate(-1.2px, -.6px); } }
  @keyframes shake { 20%, 60% { transform: translateX(-1.2px); } 40%, 80% { transform: translateX(1.2px); } }
  @keyframes scribble { 20%, 60% { transform: rotate(-8deg); } 40%, 80% { transform: rotate(5deg); } }
  @keyframes clock-check { 60% { transform: rotate(30deg); } }
  @keyframes startle { 20% { transform: translateY(-3px); } 45% { transform: translateY(1px); } 70% { transform: translateY(-1px); } }
  @keyframes stretch { 0% { transform: translateY(3px); } 35% { transform: translateY(-2.5px); } 65% { transform: translateY(-1px); } }
  @media (prefers-reduced-motion: reduce) { .gaffer-avatar * { animation: none !important; } }
</style>
