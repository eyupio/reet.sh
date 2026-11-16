<script lang="ts">
  import BoardIcon from './BoardIcon.svelte'
  import TerminalIcon from '../terminal/TerminalIcon.svelte'
  let { kind }: { kind: 'factory' | 'agent' | 'terminal' } = $props()
</script>

<div class="preview" aria-hidden="true">
  <div class="window">
    {#if kind === 'factory'}
      <div class="caption"><BoardIcon name="board" />Gaffer’s job sheet</div>
      <div class="outcome">Your outcome</div>
      <div class="flow"><span><BoardIcon name="backlog" /></span><i></i><span><TerminalIcon name="commands" /></span><i></i><span><BoardIcon name="finished" /></span></div>
    {:else if kind === 'agent'}
      <div class="caption"><TerminalIcon name="compose" />Agent session</div>
      <div>Your next task</div><div class="line"></div><div class="line short"></div>
    {:else}
      <div class="dots"><i></i><i></i><i></i></div>
      <code>~/project<br /><span>❯</span> _</code>
    {/if}
  </div>
</div>

<style>
  .preview{display:flex;align-items:center;justify-content:center;min-width:0;min-height:136px;padding:20px;background:var(--bg-sunk);color:var(--ink);border-bottom:1px solid var(--line)}
  .window{width:100%;max-width:220px;padding:12px 14px;border:1px solid var(--line);border-radius:9px;background:var(--bg);color:var(--muted);font-size:14px;line-height:1.4;box-shadow:var(--shadow)}
  .caption{display:flex;align-items:center;gap:7px;color:var(--accent-text);margin-bottom:10px;overflow-wrap:anywhere}
  .caption :global(svg){flex:none}.outcome{color:var(--ink)}
  .flow{display:flex;align-items:center;gap:6px;margin-top:10px}.flow span{display:grid;place-items:center;width:27px;height:27px;border:1px solid var(--line);background:var(--card);color:var(--accent-text);border-radius:7px;flex:none}.flow i{height:1px;background:var(--line);flex:1}
  .line{height:4px;border-radius:3px;background:var(--line);margin-top:9px;width:80%}.line.short{width:48%}
  .dots{display:flex;gap:5px;margin-bottom:12px}.dots i{height:5px;width:5px;border-radius:50%;background:var(--muted)}
  code{font:14px/1.8 var(--mono);color:var(--ink);overflow-wrap:anywhere}code span{color:var(--accent-text)}
  @container workbench (max-width:760px){.preview{width:120px;min-height:160px;border-bottom:0;border-right:1px solid var(--line);padding:12px;flex:none}.window{padding:10px}.caption{flex-wrap:wrap}.flow{gap:3px}.flow span{width:20px;height:24px}.flow span :global(svg){width:14px;height:14px}}
  @container workbench (max-width:380px){.preview{width:82px;padding:8px}.window{padding:8px}.caption{gap:4px}.flow{flex-wrap:wrap}.flow i{display:none}}
</style>
