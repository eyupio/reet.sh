<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { shell } from '../dialogShell'
  let { title, onclose, children, placement='center' }: { placement?:'center'|'drawer';title: string; onclose: () => void; children: Snippet } = $props()
  let dialog = $state<HTMLDialogElement>()
  const id = $props.id()
  onMount(() => { dialog?.showModal(); return () => dialog?.close() })
</script>
<dialog class:drawer={placement==='drawer'} bind:this={dialog} use:shell aria-labelledby={id} oncancel={event => { event.preventDefault(); onclose() }}>
  <header data-shell-head><h2 id={id}>{title}</h2><button class="btn quiet" onclick={onclose} aria-label={`Close ${title}`} data-shell-close>Close ×</button></header>
  <div class="content" data-shell-body>{@render children()}</div>
</dialog>
<style>
  .drawer{position:fixed;inset:76px 16px 16px auto;margin:0;width:min(390px,calc(100vw - 24px));max-height:calc(100dvh - 92px)}@media(max-width:650px){.drawer{inset:64px 8px 8px;width:calc(100vw - 16px);max-height:calc(100dvh - 72px)}}
  dialog{padding:0;border:1px solid var(--line);border-radius:14px;background:var(--card);color:var(--ink);width:min(580px,calc(100vw - 24px));max-height:calc(100dvh - 24px);overflow:auto}dialog::backdrop{background:rgb(20 18 15 / .4)}header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 20px;border-bottom:1px solid var(--line);position:sticky;top:0;background:var(--card);z-index:1}h2{font:500 1.35rem var(--serif);margin:0}.content{padding:18px 20px;display:grid;gap:16px}
</style>
