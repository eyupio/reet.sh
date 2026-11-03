<script lang="ts">
  import GafferAvatar from '../../src/lib/terminal/GafferAvatar.svelte'
  import { gafferAnimations } from '../../src/lib/gafferMotion'
  import type { GafferPhase } from '../../src/lib/gafferProgress'
  import type { GafferPose, GafferReaction } from '../../src/lib/gafferPersonality'
  let phase = $state<GafferPhase>('working'), pose = $state<GafferPose>('duty'), reaction = $state<GafferReaction>(''), key = $state(0), paused = $state(false)
  function select(next: string) {
    phase = next === 'Reading' ? 'reading' : next === 'Error' ? 'error' : next === 'Approval' ? 'approval' : 'working'
    // Intentionally conflicting decorations exercise status precedence in the avatar itself.
    pose = next === 'Tea' || next === 'Error' || next === 'Approval' ? 'tea' : 'duty'
    reaction = next === 'Greeting' || next === 'Error' || next === 'Approval' ? 'tip-cap' : next === 'Reading' ? 'scan' : next === 'Tea' ? 'sip' : ''
    key++
  }
</script>
<main>
  <section aria-label="Grid portrait"><GafferAvatar {phase} {pose} {reaction} {paused} reactionKey={key} /></section>
  <section aria-label="Phosphor bust"><GafferAvatar variant="bust" {phase} {pose} {reaction} {paused} reactionKey={key} /></section>
  <nav aria-label="Artwork states">{#each ['Rest', 'Greeting', 'Reading', 'Tea', 'Error', 'Approval'] as state}<button onclick={() => select(state)}>{state}</button>{/each}</nav>
  <button onclick={() => paused = !paused}>{paused ? 'Resume' : 'Pause'}</button>
  <button onclick={() => gafferAnimations.update(value => !value)}>{$gafferAnimations ? 'Disable animation' : 'Enable animation'}</button>
</main>
<style>
  :global(body) { background: #eee; margin: 20px; font-family: sans-serif; }
  main { display: flex; gap: 20px; flex-wrap: wrap; max-width: 900px; }
  section { width: 240px; height: 240px; }
  nav { flex-basis: 100%; } button { padding: 10px; margin: 3px; }
</style>
