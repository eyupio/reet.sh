<script lang="ts">
  import GafferAvatar from '../../src/lib/terminal/GafferAvatar.svelte'
  import { gafferAnimations } from '../../src/lib/gafferMotion'
  import type { GafferPhase } from '../../src/lib/gafferProgress'
  import type { GafferPose, GafferReaction } from '../../src/lib/gafferPersonality'
  let phase = $state<GafferPhase>('working'), pose = $state<GafferPose>('duty'), reaction = $state<GafferReaction>(''), key = $state(0), paused = $state(false)
  const repertoire: { label: string; reaction: GafferReaction; phase: GafferPhase; pose: GafferPose }[] = [
    { label: 'Ey up', reaction: 'tip-cap', phase: 'working', pose: 'duty' },
    { label: 'On the job', reaction: 'scan', phase: 'reading', pose: 'duty' },
    { label: 'Strategic brew', reaction: 'sip', phase: 'working', pose: 'tea' },
    { label: 'Biscuit inspection', reaction: 'biscuit', phase: 'working', pose: 'duty' },
    { label: 'Now then', reaction: 'shrug', phase: 'working', pose: 'duty' },
    { label: 'Steady on', reaction: 'huff', phase: 'working', pose: 'duty' },
    { label: 'Head scratch', reaction: 'scratch-head', phase: 'working', pose: 'duty' },
    { label: 'Chin rub', reaction: 'scratch-chin', phase: 'working', pose: 'duty' },
    { label: 'Ear scratch', reaction: 'scratch-ear', phase: 'working', pose: 'duty' },
    { label: 'Look around', reaction: 'look-around', phase: 'working', pose: 'duty' },
    { label: 'Nose twitch', reaction: 'nose-twitch', phase: 'working', pose: 'duty' },
    { label: 'Rest head', reaction: 'rest-head', phase: 'idle', pose: 'duty' },
  ]
  function select(next: string) {
    phase = next === 'Rest head' ? 'idle' : next.startsWith('Reading') ? 'reading' : next === 'Error' ? 'error' : next === 'Approval' ? 'approval' : 'working'
    // Intentionally conflicting decorations exercise status precedence in the avatar itself.
    pose = next === 'Tea' || next === 'Error' || next === 'Approval' ? 'tea' : 'duty'
    reaction = repertoire.find(item => item.label === next)?.reaction ?? (next === 'Greeting' || next === 'Error' || next === 'Approval' ? 'tip-cap' : next === 'Reading' ? 'scan' : next === 'Reading nod' ? 'nod' : next === 'Reading ponder' ? 'ponder' : next === 'Tea' ? 'sip' : next === 'Biscuit' ? 'biscuit' : next === 'Shrug' ? 'shrug' : next === 'Steady on' ? 'huff' : '')
    key++
  }
</script>
<main>
  <section aria-label="Grid portrait"><GafferAvatar {phase} {pose} {reaction} {paused} reactionKey={key} /></section>
  <section aria-label="Phosphor bust"><GafferAvatar variant="bust" {phase} {pose} {reaction} {paused} reactionKey={key} /></section>
  <nav aria-label="Artwork states">{#each ['Rest', 'Greeting', 'Reading', 'Reading nod', 'Reading ponder', 'Tea', 'Biscuit', 'Shrug', 'Steady on', 'Head scratch', 'Chin rub', 'Ear scratch', 'Look around', 'Nose twitch', 'Rest head', 'Error', 'Approval'] as state}<button onclick={() => select(state)}>{state}</button>{/each}</nav>
  <button onclick={() => paused = !paused}>{paused ? 'Resume' : 'Pause'}</button>
  <button onclick={() => gafferAnimations.update(value => !value)}>{$gafferAnimations ? 'Disable animation' : 'Enable animation'}</button>
  <div class="repertoire" aria-label="Gaffer’s motion repertoire">
    {#each repertoire as item}<article><div><GafferAvatar variant="bust" phase={item.phase} pose={item.pose} reaction={item.reaction} reactionKey={key} {paused} /></div><h2>{item.label}</h2></article>{/each}
  </div>
</main>
<style>
  :global(body) { background: var(--bg); color: var(--ink); margin: 20px; font-family: sans-serif; }
  main { display: flex; gap: 20px; flex-wrap: wrap; max-width: 900px; }
  section { width: 240px; height: 240px; }
  nav { flex-basis: 100%; } button { padding: 10px; margin: 3px; }
  .repertoire { flex-basis: 100%; display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; padding: 18px; background: var(--card); color: var(--ink); }
  article { border: 1px solid var(--line); padding: 10px; } article>div { width: 100%; aspect-ratio: 1; } h2 { text-align: center; font: 12px monospace; }
  @media (max-width: 640px) { .repertoire { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
