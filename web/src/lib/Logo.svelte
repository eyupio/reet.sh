<script lang="ts">
  import horizontalLogo from './logo.svg?raw'
</script>

<span class="logo">{@html horizontalLogo}</span>

<style>
  .logo { display: block; width: 160px; max-width: 100%; }
  .logo :global(svg) { display: block; width: 100%; height: auto; }
</style>
