<script lang="ts">
  // One line under the header while the controller is fenced. Every write is refused then, and without this a person
  // meets that one failed save at a time, on whatever page they happen to be using.
  //
  // An admin's shell asks for the state when it opens. Anyone else learns it from the first write the controller
  // refuses, which is all they are told anyway. `quiet` keeps the listening and drops the line, for the page that
  // already says it in full.
  import { onMount } from 'svelte'
  import { getBuildInfo, getFence, onRefusal } from '../api'
  import { fenceFromRefusal, fenceSentence, fenceWatch, moveFeatures } from '../controllerMove'
  import { formatWhen } from '../time'

  let { admin = false, quiet = false, timeZone = '', dateFormat = 'locale' }: { admin?: boolean; quiet?: boolean; timeZone?: string; dateFormat?: string } = $props()

  let fence = $state(fenceWatch.get())
  let asked = false

  onMount(() => {
    onRefusal((status, body) => {
      const refused = fenceFromRefusal(status, body)
      if (refused) fenceWatch.set(refused)
    })
    const stop = fenceWatch.subscribe(next => (fence = next))
    return () => { stop(); onRefusal(null) }
  })

  $effect(() => {
    if (!admin || asked) return
    asked = true
    // Asked only of a controller that says it has a fence. One without, or a sign-in that has lapsed, leaves the line off.
    void getBuildInfo()
      .then(build => (moveFeatures(build.features).fence ? getFence() : null))
      .then(next => { if (next) fenceWatch.set(next) })
      .catch(() => {})
  })
</script>

{#if fence.fenced && !quiet}
  <p class="fence-strip" role="status">
    <strong>This controller is fenced.</strong>
    {fenceSentence(fence, ms => formatWhen(ms, timeZone, dateFormat))}
    {#if admin}<a href="/admin/backups">Open Backups</a>{/if}
  </p>
{/if}

<style>
  .fence-strip {
    margin: 0; padding: 8px 16px; border-bottom: 1px solid color-mix(in srgb, var(--warn) 45%, var(--line));
    background: color-mix(in srgb, var(--warn) 10%, var(--bg)); color: var(--ink); font-size: 0.875rem; line-height: 1.45;
    text-align: center; overflow-wrap: anywhere;
  }
  a { margin-left: 4px; white-space: nowrap; }
</style>
