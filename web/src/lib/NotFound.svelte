<script lang="ts">
  import PageHeader from './PageHeader.svelte'
</script>

<PageHeader eyebrow="Not found" title="Page not found" lede="That address does not lead anywhere on this instance. It may have moved, or the link may be mistyped.">
  {#snippet actions()}<a class="btn primary" href="/">Back to Home</a>{/snippet}
</PageHeader>
