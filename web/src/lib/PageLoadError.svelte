<script lang="ts">
  const reload = () => location.reload()
</script>

<div class="notice bad" role="alert">
  <h1>This page could not load.</h1>
  <p>Check your connection, then reload to try again.</p>
  <button class="btn" type="button" onclick={reload}>Reload page</button>
</div>
