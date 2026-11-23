<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import Icon from '../Icon.svelte'
  import { shell } from '../dialogShell'
  let { title, subtitle = '', onclose, children, placement='center', wide=false }: { placement?:'center'|'drawer';wide?:boolean;title: string; subtitle?: string; onclose: () => void; children: Snippet } = $props()
  let dialog = $state<HTMLDialogElement>()
  const id = $props.id()
  onMount(() => { dialog?.showModal(); return () => dialog?.close() })
</script>
<dialog class:drawer={placement==='drawer'} class:wide bind:this={dialog} use:shell aria-labelledby={id} oncancel={event => { event.preventDefault(); onclose() }}>
  <header data-shell-head><div class="heading"><h2 id={id}>{title}</h2>{#if subtitle}<p class="subtitle" data-shell-aside title={subtitle}>{subtitle}</p>{/if}</div><button class="btn quiet close" onclick={onclose} aria-label={`Close ${title}`} data-shell-close>Close <Icon name="x" size={14} /></button></header>
  <div class="content" data-shell-body>{@render children()}</div>
</dialog>
<style>
  .drawer { position:fixed; inset:76px calc(16px + env(safe-area-inset-right,0px)) 16px auto; margin:0; width:min(390px,calc(100vw - 24px)); max-height:calc(100dvh - 92px); }
  @media (max-width:650px) { .drawer { inset:64px 8px 8px; width:calc(100vw - 16px); max-height:calc(100dvh - 72px); } }
  /* The frame itself never scrolls: only .content does. Chromium gives a modal dialog overflow:auto by UA style, which
     would add a second scrollbar beside the content's own. */
  dialog { padding:0; border:1px solid var(--line); border-radius:14px; background:var(--card); color:var(--ink); width:min(580px,calc(100vw - 24px)); max-height:calc(100dvh - 24px); overflow:hidden; }
  /* minmax(0,1fr): a long unwrapped line inside cannot widen the column past the dialog's own width. */
  dialog[open] { display:grid; grid-template-rows:auto minmax(0,1fr); grid-template-columns:minmax(0,1fr); }
  dialog.wide { width:min(820px,calc(100vw - 24px)); }
  dialog::backdrop { background:rgb(20 18 15 / .4); }
  header { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; padding:18px 20px 14px; border-bottom:1px solid var(--line); background:var(--card); }
  .heading { min-width:0; }
  h2 { font:500 1.35rem/1.2 var(--serif); margin:0; overflow-wrap:anywhere; }
  .subtitle { margin:5px 0 0; color:var(--muted); font-size:.875rem; line-height:1.4; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .close { flex:none; white-space:nowrap; }
  .content { min-height:0; padding:20px; overflow-y:auto; overscroll-behavior:contain; display:grid; gap:20px; align-content:start; }
</style>
