<script lang="ts">
  // The Factory with no pipelines yet: one line and the button. The outcome dialog asks for everything else.
  let { onstart }: { onstart: () => void } = $props()
  let startButton = $state<HTMLButtonElement>()
  export function focus() { startButton?.focus({ preventScroll: true }) }
  const id = $props.id()
</script>

<section class="welcome" aria-labelledby={`${id}-heading`}>
  <h2 id={`${id}-heading`}>No pipelines yet.</h2>
  <button class="btn primary" type="button" bind:this={startButton} onclick={() => onstart()}>Give Gaffer an outcome</button>
</section>

<style>
  .welcome{display:grid;gap:14px;justify-items:center;text-align:center;min-width:0;padding:28px 20px;border:1px dashed var(--line-strong);border-radius:14px;color:var(--ink)}
  h2{margin:0;font:500 1rem/1.4 var(--sans);color:var(--muted);overflow-wrap:anywhere}
  .btn{min-height:44px;max-width:100%}
</style>
