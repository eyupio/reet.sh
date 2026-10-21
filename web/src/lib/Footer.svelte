<script lang="ts">
  import { DEVELOPER_NAME, DEVELOPER_URL } from './links'
</script>

<footer>
  <div class="inner">
    <span class="brand"><span class="name">reet.sh</span><span class="descriptor">Your shelf for AI skills</span></span>
    <!-- The credit leaves the app, so it opens in a new tab and the list the person was on is not lost. -->
    <span class="credit">
      Developed by
      <a href={DEVELOPER_URL} target="_blank" rel="noopener noreferrer">{DEVELOPER_NAME}<span class="vh"> (opens in a new tab)</span></a>
    </span>
  </div>
</footer>

<style>
  /* Same 16px gutter and 760px column as <main>, so the hairline lines up with the content above it. */
  footer { color: var(--muted); font-size: 0.85rem; padding: 0 16px; }
  .inner {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 16px;
    max-width: 728px; margin: 0 auto; padding: 18px 0 40px; border-top: 1px solid var(--line);
  }
  .brand { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
  .name { font-weight: 600; color: var(--ink); }
  .descriptor { letter-spacing: 0.06em; text-transform: uppercase; font-size: 0.75rem; }
  .credit { white-space: nowrap; }
  .credit a { color: var(--muted); font-weight: 500; text-underline-offset: 3px; }
  .credit a:hover, .credit a:focus-visible { color: var(--ink); text-decoration: underline; }
  @media (max-width: 560px) { .descriptor { display: none; } }
</style>
