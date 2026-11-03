<script lang="ts">
  export type IconName = 'chevron' | 'plus' | 'refresh' | 'arrow-right' | 'x' | 'more'
  let { name, size = 16 }: { name: IconName; size?: number } = $props()
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  {#if name === 'more'}
    <circle cx="5" cy="12" r="1"></circle><circle cx="12" cy="12" r="1"></circle><circle cx="19" cy="12" r="1"></circle>
  {:else if name === 'chevron'}
    <path d="m6 9 6 6 6-6"></path>
  {:else if name === 'plus'}
    <path d="M12 5v14M5 12h14"></path>
  {:else if name === 'refresh'}
    <path d="M20 12a8 8 0 1 1-2.34-5.66M20 4v5h-5"></path>
  {:else if name === 'arrow-right'}
    <path d="M4 12h16m-6-6 6 6-6 6"></path>
  {:else}
    <path d="M6 6l12 12M18 6 6 18"></path>
  {/if}
</svg>
