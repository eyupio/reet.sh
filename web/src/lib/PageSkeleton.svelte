<script lang="ts">
  // The default page skeleton: the shape of a ledger page with a heading, a few cards and a list. The HTML boot shell
  // draws this same shape before the bundle runs, and RouteSkeleton keeps it while a page's code downloads, so a slow
  // route never swaps one layout for another on its way to the real page.
</script>

<div class="boot-page" aria-hidden="true">
  <div class="head"><i class="slim"></i><i class="title"></i></div>
  <div class="cards"><div class="card"></div><div class="card"></div><div class="card"></div></div>
  <div class="rows">
    <div class="row"><i class="slim"></i><i class="slim"></i></div>
    <div class="row"><i class="slim"></i><i class="slim"></i></div>
    <div class="row"><i class="slim"></i><i class="slim"></i></div>
    <div class="row"><i class="slim"></i><i class="slim"></i></div>
  </div>
</div>

<style>
  .boot-page { display: grid; gap: 18px; width: 100%; min-width: 0; }
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
  @media (prefers-reduced-motion: no-preference) { i { background: linear-gradient(90deg, var(--line) 0%, var(--line-soft) 50%, var(--line) 100%); background-size: 200% 100%; animation: page-shimmer 1.8s ease-in-out infinite; } }
  @keyframes page-shimmer { to { background-position: -200% 0; } }
  @media (max-width: 850px) { .cards { grid-template-columns: 1fr; } }
</style>
