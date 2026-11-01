<script lang="ts">
 // The in-app twin of the boot shell in index.html: while the first auth answer is on its way, the page keeps a shape
 // instead of reporting a bare "Loading…". It is covered by the static shell on a first load and takes over on a later
 // one, so the two share their spacing and the swap is invisible.
</script>

<div class="boot-page" role="status" aria-busy="true" aria-label="Loading reet.sh">
 <span class="vh">Loading reet.sh…</span>
 <div class="head" aria-hidden="true"><i class="slim"></i><i class="title"></i></div>
 <div class="cards" aria-hidden="true"><div class="card"></div><div class="card"></div><div class="card"></div></div>
 <div class="rows" aria-hidden="true">
  <div class="row"><i class="slim"></i><i class="slim"></i></div>
  <div class="row"><i class="slim"></i><i class="slim"></i></div>
  <div class="row"><i class="slim"></i><i class="slim"></i></div>
 </div>
</div>

<style>
 .boot-page { display: grid; gap: 18px; max-width: var(--content-width); margin: 0 auto; padding: 38px 40px 56px; }
 .head { display: grid; gap: 12px; margin-bottom: 8px; }
 i { display: block; background: var(--line); border-radius: 6px; height: 13px; }
 i.slim { height: 10px; }
 .head i.slim { width: 120px; }
 .head i.title { width: 34%; height: 30px; border-radius: 8px; }
 .cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
 .card { height: 150px; border: 1px solid var(--line); border-radius: 14px; background: var(--card); }
 .rows { display: grid; gap: 1px; border: 1px solid var(--line); border-radius: 14px; overflow: hidden; background: var(--line); }
 .row { display: flex; align-items: center; gap: 16px; padding: 18px 20px; background: var(--card); }
 .row i { flex: 1; }
 .row i:first-child { flex: 0 0 32%; }
 @media (prefers-reduced-motion: no-preference) { i, .card, .row { animation: boot-pulse 1.7s ease-in-out infinite; } i:nth-child(2n) { animation-delay: 250ms; } }
 @keyframes boot-pulse { 0%, 100% { opacity: 1; } 50% { opacity: .55; } }
 @media (max-width: 850px) { .boot-page { padding: 26px 24px 44px; } .cards { grid-template-columns: 1fr; } }
 @media (max-width: 550px) { .boot-page { padding: 18px 16px 36px; } }
</style>
