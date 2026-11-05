<script lang="ts">
 // Two cards of identical layout, one on top of the other. The slider only decides how much of the "after" card is drawn.
 let position = $state(50)
 const valueText = $derived(position <= 0 ? 'Showing the machine before it is enrolled' : position >= 100 ? 'Showing the machine after it is enrolled' : `${position}% shows the machine after it is enrolled`)
</script>

<section class="fresh" aria-labelledby="fresh-title">
 <div class="fresh-copy">
  <p class="eyebrow">A new machine</p>
  <h2 id="fresh-title">Start with nothing. End with everything.</h2>
  <p>Got a new laptop, a fresh VPS or a lab box with nothing on it? Enrol it with one command. Your skills and setups arrive, you review the plan, and it ends up like the rest.</p>
 </div>
 <div class="stage">
  <div class="compare" role="group" aria-label="The machine fresh-vps, before and after it is enrolled" data-position={position} style={`--p:${position}%`}>
   <div class="card before">
    <header><b>fresh-vps</b><span class="pill">not enrolled</span></header>
    <dl>
     <div class="row"><dt>Skills</dt><dd>0</dd></div>
     <div class="row"><dt>Setups</dt><dd>0</dd></div>
     <div class="row"><dt>Agents reporting</dt><dd>none yet</dd></div>
    </dl>
   </div>
   <div class="card after">
    <header><b>fresh-vps</b><span class="pill good">in sync</span></header>
    <dl>
     <div class="row"><dt>Skills</dt><dd>4</dd></div>
     <div class="row"><dt>Setups</dt><dd>1 · web-dev</dd></div>
     <div class="row"><dt>Agents reporting</dt><dd>Claude Code</dd></div>
    </dl>
   </div>
   <input type="range" min="0" max="100" step="1" bind:value={position} aria-label="Compare fresh-vps before and after it is enrolled" aria-valuetext={valueText} />
   <div class="divider" aria-hidden="true"><span class="handle">‹ ›</span></div>
  </div>
  <p class="caps" aria-hidden="true"><span>After</span><span>Before</span></p>
 </div>
</section>

<style>
 .fresh{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.05fr);gap:48px;align-items:center;padding:54px 0;border-bottom:1px solid var(--line)}
 .eyebrow{font-size:0.875rem;font-weight:600;letter-spacing:.12em;text-transform:uppercase;color:var(--accent-text);margin:0 0 18px}
 h2{font:500 clamp(1.7rem,3vw,2.6rem)/1.15 var(--serif);letter-spacing:-.035em;margin:0 0 14px}
 .fresh-copy>p:last-child{font-size:.88rem;line-height:1.8;color:var(--muted);margin:0;max-width:48ch}
 .stage{min-width:0}
 .compare{position:relative;max-width:460px;margin:0 auto;border-radius:14px;overflow:hidden;border:1px solid var(--line);background:var(--card);box-shadow:var(--shadow)}
 .card{background:var(--card);padding:16px}
 .card header{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:12px}
 .card header b{font:600 .95rem var(--mono)}
 .pill{font-size:0.875rem;font-weight:600;padding:3px 10px;border-radius:999px;border:1px dashed var(--muted);color:var(--muted);white-space:nowrap}
 .pill.good{border:1px solid var(--good);color:var(--good)}
 dl{margin:0;display:grid;gap:8px}
 .row{display:flex;justify-content:space-between;align-items:center;gap:12px;padding:12px 14px;border:1px dashed var(--line-strong);border-radius:10px;background:var(--bg)}
 .after .row{border:1px solid var(--line);background:var(--card)}
 dt{font-size:0.875rem;color:var(--muted)}
 dd{margin:0;font:600 0.875rem var(--mono);text-align:right;overflow-wrap:anywhere}
 .after{position:absolute;inset:0;clip-path:inset(0 calc(100% - var(--p)) 0 0)}
 /* The native slider covers the card so a drag or a tap anywhere moves it; it draws nothing, the divider and handle do. */
 input[type='range']{position:absolute;inset:0;z-index:3;width:100%;height:100%;margin:0;padding:0;opacity:0;cursor:ew-resize;touch-action:pan-y;-webkit-appearance:none;appearance:none;background:transparent;border:0;min-height:0}
 input[type='range']::-webkit-slider-thumb{-webkit-appearance:none;width:1px;height:100%}
 input[type='range']::-moz-range-thumb{width:1px;height:100%;border:0}
 .divider{position:absolute;top:0;bottom:0;left:var(--p);width:2px;margin-left:-1px;background:var(--accent-text);z-index:2;pointer-events:none}
 .handle{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);display:grid;place-items:center;min-width:44px;height:30px;border-radius:999px;background:var(--accent);color:var(--accent-ink);font-size:0.875rem;font-weight:700;letter-spacing:.05em;box-shadow:var(--shadow)}
 .compare:has(input:focus-visible) .handle{outline:2px solid var(--ink);outline-offset:2px}
 .caps{display:flex;justify-content:space-between;max-width:460px;margin:10px auto 0;font-size:0.875rem;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
 @media(max-width:850px){.fresh{grid-template-columns:1fr;gap:28px;padding:40px 0}}
 @media(max-width:550px){.fresh{padding:32px 0}.fresh-copy .eyebrow{margin-bottom:12px}.card{padding:14px 12px}.row{padding:11px 12px}}
</style>
