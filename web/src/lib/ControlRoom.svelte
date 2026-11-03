<script lang="ts">
 import { onDestroy } from 'svelte'
 import { enrollCommands } from './machineSetup'
 import './landingTerminal.css'

 // The illustration is the product's own enrolment for an empty machine, line for line, against an example address.
 const enrol = enrollCommands('https://reet.example.net', 'fresh-vps').split('\n')

 type Stage = 'empty' | 'enrol' | 'fill' | 'sync'
 type Layout = { name: 'wide' | 'compact'; w: number; h: number; nw: number; nh: number; core: { x: number; y: number; r: number }; nodes: { x: number; y: number }[]; side: boolean }
 // The wide drawing sits the machines either side of the controller; the compact one stacks them above and below it, so the
 // words stay at a readable size on a phone instead of shrinking with the picture.
 const layouts: Layout[] = [
  { name: 'wide', w: 520, h: 214, nw: 156, nh: 60, core: { x: 260, y: 107, r: 46 }, nodes: [{ x: 2, y: 8 }, { x: 362, y: 8 }, { x: 2, y: 144 }, { x: 362, y: 144 }], side: true },
  { name: 'compact', w: 320, h: 266, nw: 150, nh: 58, core: { x: 160, y: 133, r: 42 }, nodes: [{ x: 2, y: 2 }, { x: 168, y: 2 }, { x: 2, y: 206 }, { x: 168, y: 206 }], side: false }
 ]
 const machines = [
  { name: 'work-laptop', sub: 'OpenCode · 4 skills', wire: '', label: 'command' },
  { name: 'hetzner-vps', sub: 'Claude Code · running', wire: 'slow', label: 'sync' },
  { name: 'lab-vm-2', sub: 'Codex · 4 skills', wire: 'slow', label: 'backup' },
  { name: 'fresh-vps', sub: '', wire: 'fw', label: 'start fresh' }
 ]
 const TOTAL = 4
 const geometry = layouts.map(l => ({
  ...l,
  items: machines.map((m, i) => {
   const n = l.nodes[i]
   // The wire leaves the machine's inner edge (wide) or its facing edge (compact) and ends on the controller's rim.
   const ax = l.side ? (i % 2 === 0 ? n.x + l.nw : n.x) : n.x + l.nw / 2
   const ay = l.side ? n.y + l.nh / 2 : i < 2 ? n.y + l.nh : n.y
   const dx = l.core.x - ax, dy = l.core.y - ay, len = Math.hypot(dx, dy)
   const ex = l.core.x - (dx / len) * (l.core.r + 2), ey = l.core.y - (dy / len) * (l.core.r + 2)
   return { ...m, x: n.x, y: n.y, wireD: `M${ax} ${ay} L${ex.toFixed(1)} ${ey.toFixed(1)}`, lx: ax + (ex - ax) * 0.55, ly: ay + (ey - ay) * 0.55 + 4 }
  })
 }))

 const reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
 let stage = $state<Stage>(reduced ? 'sync' : 'empty')
 let done = $state(0)
 let shown = $state(reduced ? 4 : 0)
 const count = $derived(stage === 'sync' ? 4 : 3)
 const badge = $derived(stage === 'empty' ? 'waiting' : stage === 'enrol' ? 'enrolling' : stage === 'fill' ? 'restoring' : 'in sync')
 const sub = $derived(stage === 'empty' ? 'Empty, nothing reported' : stage === 'enrol' ? 'Reporting in' : stage === 'fill' ? `Approved · ${done} of ${TOTAL} skills` : 'Claude Code · 4 skills')
 const pct = $derived(done === 0 ? 5 : (done / TOTAL) * 100)

 let timers: ReturnType<typeof setTimeout>[] = []
 const at = (ms: number, fn: () => void) => { timers.push(setTimeout(fn, ms)) }
 const clear = () => { timers.forEach(clearTimeout); timers = [] }
 function play() {
  clear()
  stage = 'empty'; done = 0; shown = 0
  at(500, () => { shown = 1 })
  at(1100, () => { shown = 2 })
  at(2500, () => { shown = 3; stage = 'enrol' })
  at(3100, () => { stage = 'fill'; done = 0 })
  ;[3500, 3850, 4200, 4550].forEach((ms, i) => at(ms, () => { done = i + 1 }))
  at(5100, () => { shown = 4; stage = 'sync' })
 }
 if (!reduced) play()
 onDestroy(clear)
</script>

<figure class="workspace-preview" aria-label="Illustration: a controller with four machines, and an empty machine enrolled with one command until it matches the rest">
 <div class="preview-top"><span class="preview-brand">reet<span>.sh</span></span><span class="example">Illustration</span></div>
 <div class="room">
  <div class="room-head"><p><b>Control room</b><span class="hub-count">{count} machines enrolled</span></p>{#if !reduced}<button type="button" class="replay" aria-label="Replay the enrolment" onclick={play}><span aria-hidden="true">↻</span> Replay</button>{/if}</div>
  {#each geometry as g}
   <svg class="hubsvg" class:wide={g.name === 'wide'} class:compact={g.name === 'compact'} data-state={stage} viewBox={`0 0 ${g.w} ${g.h}`} role="img" aria-label="Controller connected to four machines: three in sync and an empty one">
    {#each g.items as m}<path class="wire" class:slow={m.wire === 'slow'} class:fw={m.wire === 'fw'} d={m.wireD} />{/each}
    {#each g.items as m, i}
     <g class="node-group">
      <rect class="node" class:fnode={i === 3} x={m.x} y={m.y} width={g.nw} height={g.nh} rx="12" />
      <text x={m.x + 12} y={m.y + 23}>{m.name}</text>
      <text class="b" class:fnode-badge={i === 3} x={m.x + g.nw - 12} y={m.y + 23} text-anchor="end">{i === 3 ? badge : 'in sync'}</text>
      <text class="s" class:fnode-sub={i === 3} x={m.x + 12} y={m.y + 41}>{i === 3 ? sub : m.sub}</text>
      {#if i === 3}<g class="fprog" class:on={stage === 'fill'}><rect class="pbg" x={m.x + 12} y={m.y + g.nh - 9} width={g.nw - 24} height="4" rx="2" /><rect class="pfill" x={m.x + 12} y={m.y + g.nh - 9} width={((g.nw - 24) * pct) / 100} height="4" rx="2" /></g>{/if}
     </g>
    {/each}
    <circle class="core" cx={g.core.x} cy={g.core.y} r={g.core.r} />
    <text class="c" x={g.core.x} y={g.core.y - 3} text-anchor="middle">Controller</text>
    <text class="c" x={g.core.x} y={g.core.y + 14} text-anchor="middle">your machine</text>
    {#each g.items as m}<text class="lab" x={m.lx} y={m.ly} text-anchor="middle">{m.label}</text>{/each}
   </svg>
  {/each}
 </div>
 <div class="preview-shell" aria-hidden="true"><div class="shell-title"><span>›_ fresh-vps</span><small>enrolling with your controller</small></div><div class="shell-content">
  {#each enrol as line, i}<p class="shell-line" class:on={shown > i}><span class="prompt">$</span> {line}</p>{/each}
  <p class="shell-line" class:on={shown > 2}><span class="path">✓</span> fresh-vps reports in <span class="muted">· outbound only, no port to open</span></p>
  <p class="shell-line" class:on={shown > 3}><span class="path">✓</span> 4 skills, 1 setup in sync <span class="muted">· reviewed, then applied</span></p>
  <p class="shell-space">&nbsp;</p><p><span class="prompt">$</span> <span class="cursor"></span></p><div class="continuity"><i></i>One binary. One SQLite file. No telemetry.</div></div></div>
 <figcaption>An empty machine enrolled with one command, then in sync with the rest. Illustration.</figcaption>
</figure>

<style>
 .workspace-preview{margin:0;min-width:0;border:1px solid var(--line);border-radius:14px;overflow:hidden;background:var(--card);box-shadow:var(--shadow);transform:rotate(-1deg)}
 .preview-top{display:flex;justify-content:space-between;align-items:center;gap:8px;border-bottom:1px solid var(--line);padding:12px 16px}
 .preview-brand{font-weight:700;font-size:.85rem;letter-spacing:-.03em}.preview-brand>span{color:var(--accent-text)}
 .example{font-size:.6875rem;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
 .room{background:var(--bg);border-bottom:1px solid var(--line);padding:12px 16px 14px}
 .room-head{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:6px;min-height:30px}
 .room-head p{display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 10px;margin:0;font-size:.75rem}
 .room-head b{font-weight:600}.hub-count{font-size:.6875rem;letter-spacing:.06em;color:var(--muted)}
 .replay{border:1px solid var(--line-strong);background:var(--card);border-radius:8px;padding:5px 10px;font-size:.75rem;color:var(--ink);cursor:pointer;min-height:30px;white-space:nowrap}
 .replay:hover{border-color:var(--muted)}
 .hubsvg{width:100%;height:auto;display:block}
 .hubsvg.compact{display:none;max-width:360px;margin:0 auto}
 .hubsvg .node{fill:var(--card);stroke:var(--line-strong);stroke-width:1.5;transition:stroke .3s}
 .hubsvg .core{fill:var(--accent)}
 .hubsvg text{fill:var(--ink);font:600 12px var(--sans)}
 .hubsvg text.s{fill:var(--muted);font-weight:400;font-size:11px}
 .hubsvg text.c{fill:var(--accent-ink);font-size:12px}
 .hubsvg text.c+text.c{font-weight:400;font-size:11px}
 .hubsvg text.b{fill:var(--good);font-weight:600;font-size:11px}
 .hubsvg text.lab{paint-order:stroke;stroke:var(--bg);stroke-width:4px;stroke-linejoin:round;fill:var(--muted);font-weight:500;font-size:11px}
 .hubsvg .wire{stroke:var(--accent-text);stroke-width:1.6;stroke-dasharray:4 6;fill:none;opacity:.8;animation:flow 1.6s linear infinite}
 .hubsvg .wire.slow{animation-duration:2.6s;opacity:.5}
 .hubsvg .fprog{display:none}.hubsvg .fprog.on{display:inline}
 .hubsvg .pbg{fill:var(--line)}.hubsvg .pfill{fill:var(--accent-text);transition:width .35s ease}
 .hubsvg[data-state="empty"] .fnode{stroke-dasharray:5 4;stroke:var(--muted)}
 .hubsvg[data-state="empty"] .fnode-badge{fill:var(--muted)}
 .hubsvg[data-state="empty"] .fw{opacity:.12;animation:none}
 .hubsvg[data-state="enrol"] .fnode,.hubsvg[data-state="fill"] .fnode{stroke-dasharray:5 4;stroke:var(--accent-text)}
 .hubsvg[data-state="enrol"] .fnode-badge,.hubsvg[data-state="fill"] .fnode-badge{fill:var(--accent-text)}
 .hubsvg[data-state="fill"] .fw{animation-duration:.7s;opacity:1}
 .hubsvg[data-state="sync"] .fnode{stroke:var(--good)}
 @keyframes flow{to{stroke-dashoffset:-20}}
 @media(prefers-reduced-motion:reduce){.hubsvg .wire{animation:none!important}.hubsvg .pfill,.hubsvg .node{transition:none}.shell-line{transition:none!important}}
 .preview-shell{min-width:0;background:var(--shell-bg)}
 .shell-title{padding:12px 14px;display:grid;gap:4px;border-bottom:1px solid var(--shell-line);color:var(--shell-ink);font-size:.6875rem}
 .shell-title small{font:.6875rem var(--mono);color:var(--shell-dim)}
 .shell-content{font:clamp(11px,1vw,12px)/1.7 var(--mono);color:var(--shell-text);padding:16px 16px 14px;position:relative;min-height:0}
 .shell-content p{margin:0 0 6px;white-space:normal;overflow-wrap:anywhere}
 .shell-line{opacity:0;transition:opacity .3s ease}.shell-line.on{opacity:1}
 .path{color:var(--shell-ok)}.muted{color:var(--shell-mute)}.prompt{color:var(--shell-prompt)}
 .shell-content .shell-space{margin:10px 0}
 .cursor{display:inline-block;height:13px;width:2px;background:var(--shell-cursor);vertical-align:middle}
 .continuity{display:flex;align-items:center;gap:5px;font-size:.6875rem;color:var(--shell-foot);margin-top:14px;white-space:normal}
 .continuity i{height:4px;width:4px;border-radius:50%;background:var(--shell-ok);flex:none}
 figcaption{font-size:.6875rem;padding:10px 14px;color:var(--muted);border-top:1px solid var(--line)}
 @media(max-width:850px){.workspace-preview{max-width:650px;transform:none}.shell-content{font-size:12px}}
 @media(max-width:600px){.hubsvg.wide{display:none}.hubsvg.compact{display:block}}
 @media(max-width:550px){.room{padding:12px 10px 12px}.shell-content{padding:14px 10px 12px;font-size:11px}.shell-title{padding:10px}}
</style>
