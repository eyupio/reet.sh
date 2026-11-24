<script lang="ts">
  // A server sentence with `commands` in backticks: the commands show in the product's inline code style.
  let { text }: { text: string } = $props()
  const parts = $derived(text.split('`'))
</script>
<span>{#each parts as part, i}{#if i % 2}<code>{part}</code>{:else}{part}{/if}{/each}</span>
<style>
  code { font: 0.875em var(--mono); color: var(--ink); overflow-wrap: anywhere; }
</style>
