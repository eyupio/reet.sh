<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  let { title, onclose, children }: { title: string; onclose: () => void; children: Snippet } = $props()
  let dialog = $state<HTMLDialogElement>()
  const id = $props.id()
  onMount(() => { dialog?.showModal(); return () => dialog?.close() })
</script>
<dialog bind:this={dialog} aria-labelledby={id} oncancel={event => { event.preventDefault(); onclose() }}>
  <header><h2 id={id}>{title}</h2><button class="btn quiet" onclick={onclose} aria-label={`Close ${title}`}>Close ×</button></header>
  <div class="content">{@render children()}</div>
</dialog>
<style>
  dialog{padding:0;border:1px solid var(--line);border-radius:14px;background:var(--card);color:var(--ink);width:min(580px,calc(100vw - 24px));max-height:calc(100dvh - 24px);overflow:auto}dialog::backdrop{background:rgb(20 18 15 / .4)}header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 20px;border-bottom:1px solid var(--line);position:sticky;top:0;background:var(--card);z-index:1}h2{font:500 1.35rem var(--serif);margin:0}.content{padding:18px 20px;display:grid;gap:16px}
</style>
