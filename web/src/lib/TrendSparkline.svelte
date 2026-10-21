<script lang="ts">
  let { values, name }: { values: number[]; name: string } = $props()

  const max = $derived(Math.max(...values, 1))
  const width = $derived(values.length * 8 - 2)
  const label = $derived(`Weekly installs for ${name}: ${values.map((v) => v.toLocaleString()).join(', ')}`)
</script>

<svg class="spark" viewBox="0 0 {width} 18" role="img" aria-label={label}>
  {#each values as value, i}
    <rect
      x={i * 8}
      y={18 - Math.max(2, (value / max) * 16)}
      width="5"
      height={Math.max(2, (value / max) * 16)}
      rx="1"
      fill="var(--accent)"
      opacity={i === values.length - 1 ? '1' : '0.32'}
    ></rect>
  {/each}
</svg>

<style>
  .spark { width: 62px; height: 18px; flex: none; vertical-align: middle; }
</style>
