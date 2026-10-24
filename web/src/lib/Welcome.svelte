<script lang="ts">
  import CopyButton from './CopyButton.svelte'

  let { signupEnabled = false, contactEmail = '', setupRequired = false }: { signupEnabled?: boolean; contactEmail?: string; setupRequired?: boolean } = $props()

  const install = 'git clone https://github.com/eyupio/reet.sh && cd reet.sh && sh scripts/init-env.sh && docker compose up -d'
  const docs = 'https://github.com/eyupio/reet.sh#quick-start'
  const inviteHref = $derived(contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent('reet.sh account request')}&body=${encodeURIComponent('Hi, I would like an account on your reet.sh instance.')}`
    : '')
</script>

<section class="welcome" aria-labelledby="welcome-title">
  <div class="hero">
    <p class="eyebrow">Ey up. Your machines, together.</p>
    <h1 id="welcome-title">Your machines.<br />Your skills.<br /><span>One control centre.</span></h1>
    <p class="lede">Connect your machines to a self-hosted workspace. Maintain skills here, review changes in both directions, and use installed agents through a remote terminal. Capture from <a href="https://www.skills.sh" target="_blank" rel="noopener noreferrer">skills.sh</a> or host your own skills, then choose which machines receive them.</p>
    <div class="cta-row">
      {#if setupRequired}
        <a class="btn primary" href="/setup">Set up this instance</a>
        <a class="btn" href="#self-host">Self-host it</a>
      {:else}
        <a class="btn primary" href="/signin">Sign in</a>
        {#if signupEnabled}
          <a class="btn" href="/signup">Create an account</a>
        {:else}
          <a class="btn" href="#self-host">Self-host it</a>
        {/if}
      {/if}
    </div>
    {#if setupRequired}
      <p class="invite">This instance has no owner yet. Setup takes about a minute.</p>
    {:else if !signupEnabled}
      <p class="invite">
        {#if inviteHref}<a href={inviteHref}>Request an invite</a> from the instance owner.{:else}Ask your instance admin for an invitation.{/if}
      </p>
    {/if}
    <ul class="trust" aria-label="Why reet.sh">
      <li>Self-hosted</li>
      <li>No telemetry</li>
      <li>One SQLite file</li>
      <li>AGPL-3.0</li>
    </ul>
  </div>

  <figure class="shot">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="/shelf-dark.png" />
      <img src="/shelf-light.png" alt="The reet.sh shelf: skills grouped into On the shelf, To test and In use, with install counts and quick actions." />
    </picture>
    <figcaption>One shelf, three stages. Keep a find, give it a go, then keep what earns its place.</figcaption>
  </figure>
</section>

<section class="how" aria-label="How your workspace works">
  <div>
    <span>01</span>
    <h2>Connect your machines</h2>
    <p>Enroll a machine and report what its agents have installed. Keep an eye on inventories and private connection health.</p>
  </div>
  <div>
    <span>02</span>
    <h2>Choose and sync</h2>
    <p>Maintain a central skill library. Send a selection to one machine or all, bring skills back, and review installs and removals.</p>
  </div>
  <div>
    <span>03</span>
    <h2>Open a terminal</h2>
    <p>Enable terminal access on a machine, then use its installed AI agents from your browser. Sessions stay on the host when you disconnect.</p>
  </div>
</section>

<section class="features" aria-labelledby="features-title">
  <div class="feature-head">
    <p class="eyebrow">More than a bookmark list</p>
    <h2 id="features-title">A workspace of your own. Room for the fleet.</h2>
    <p>Start with your own finds. Add lists, teams, hosted skills, agent sync and sealed backups when you need them. Every part is built in and self-hosted.</p>
  </div>
  <div class="feature-grid">
    <article>
      <span>01</span>
      <h3>Collect and triage</h3>
      <p>Save a find from skills.sh, an install command, a URL or your own hosted page. Move it through On the shelf, To test and In use. Add tags, notes and bookmarklets.</p>
    </article>
    <article>
      <span>02</span>
      <h3>Lists and teams</h3>
      <p>Keep a list to yourself, share it with a few people, a whole team or your organisation, publish to the community, or hand out an unlisted link. Editors and per-list notes included.</p>
    </article>
    <article>
      <span>03</span>
      <h3>Agent sync</h3>
      <p>See what Claude Code, Cursor, Codex, OpenCode and the skills CLI have installed. Compare machines, find what is missing and sync it across your agents.</p>
    </article>
    <article>
      <span>04</span>
      <h3>Host and connect</h3>
      <p>Publish skills under a permanent username. Connect agents over MCP with OAuth, bearer tokens or WebMCP, with read or write scopes you control.</p>
    </article>
    <article>
      <span>05</span>
      <h3>Portable AI setups</h3>
      <p>Capture agent settings, skills and supported memory as a setup. Review coverage before restoring it on another machine, or keep a reference snapshot.</p>
    </article>
    <article>
      <span>06</span>
      <h3>Sealed backups</h3>
      <p>Back up your own shelf or the whole instance. Sealed copies stay local or go to any S3-compatible destination, with schedules, retention and restore.</p>
    </article>
  </div>
</section>

<section class="selfhost" id="self-host" aria-labelledby="selfhost-title">
  <div class="selfhost-copy">
    <p class="eyebrow">Yours to keep</p>
    <h2 id="selfhost-title">One binary. One SQLite file. No telemetry.</h2>
    <p>Run it on your laptop first, then a home server or your team's infrastructure when you need. Lists, teams, agent sync, MCP, setups and sealed backups come with it. Nothing is sent to skills.sh except read-only page fetches for names and descriptions.</p>
    <p class="links"><a href={docs} target="_blank" rel="noopener noreferrer">Quick start and docs<span class="vh"> (opens in a new tab)</span></a><a href="https://github.com/eyupio/reet.sh" target="_blank" rel="noopener noreferrer">GitHub<span class="vh"> (opens in a new tab)</span></a></p>
  </div>
  <div class="command">
    <code>{install}</code>
    <CopyButton text={install} label="Copy install command" />
  </div>
</section>

<style>
  .welcome { display: grid; grid-template-columns: minmax(0, 0.92fr) minmax(0, 1.08fr); gap: clamp(24px, 4vw, 48px); align-items: center; margin: 8px 0 12px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h1 { margin: 14px 0 0; font: 500 clamp(2.4rem, 4.6vw, 3.8rem)/1 var(--serif); letter-spacing: -0.045em; }
  h1 span { color: var(--accent); }
  .lede { margin: 20px 0 0; max-width: 48ch; color: var(--muted); font-size: 1rem; line-height: 1.6; }
  .lede a { color: var(--accent); }
  .cta-row { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 26px; }
  .cta-row .btn { min-height: 46px; padding: 10px 18px; }
  .invite { margin: 14px 0 0; color: var(--muted); font-size: 0.85rem; }
  .invite a { color: var(--accent); }
  .trust { list-style: none; display: flex; flex-wrap: wrap; gap: 6px; padding: 0; margin: 28px 0 0; }
  .trust li { padding: 5px 9px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.73rem; }
  .shot { margin: 0; min-width: 0; }
  .shot picture { display: block; overflow: hidden; max-height: 470px; border: 1px solid var(--line); border-radius: 16px; background: var(--card); box-shadow: var(--shadow); }
  .shot img { display: block; width: 100%; height: auto; object-position: top; }
  .shot figcaption { margin-top: 10px; color: var(--muted); font-size: 0.78rem; }

  .how { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin: 34px 0 0; }
  .how > div { padding: 20px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .how span { color: var(--accent); font: 0.72rem var(--mono); }
  .how h2 { margin: 8px 0 6px; font: 500 1.35rem/1.2 var(--serif); }
  .how p { margin: 0; color: var(--muted); font-size: 0.85rem; line-height: 1.55; }

  .features { margin: 34px 0 0; }
  .feature-head { max-width: 760px; }
  .feature-head h2 { margin: 8px 0 8px; font: 500 clamp(1.6rem, 2.8vw, 2.2rem)/1.12 var(--serif); letter-spacing: -0.03em; }
  .feature-head p { margin: 0; color: var(--muted); font-size: 0.9rem; line-height: 1.6; }
  .feature-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin-top: 20px; }
  .feature-grid article { padding: 20px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .feature-grid span { color: var(--accent); font: 0.7rem var(--mono); }
  .feature-grid h3 { margin: 8px 0 6px; font: 500 1.25rem/1.2 var(--serif); }
  .feature-grid p { margin: 0; color: var(--muted); font-size: 0.82rem; line-height: 1.55; }

  .selfhost { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr); gap: 24px; align-items: center; margin: 34px 0 0; padding: clamp(22px, 3vw, 30px); border: 1px solid var(--line); border-radius: 16px; background: color-mix(in srgb, var(--tint) 24%, var(--card)); }
  .selfhost h2 { margin: 8px 0 10px; font: 500 clamp(1.5rem, 2.6vw, 2rem)/1.15 var(--serif); letter-spacing: -0.025em; }
  .selfhost p { margin: 0; color: var(--muted); font-size: 0.88rem; max-width: 56ch; }
  .links { display: flex; flex-wrap: wrap; gap: 16px; margin-top: 14px !important; }
  .links a { color: var(--accent); font-weight: 600; }
  .command { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
  .command code { font-size: 0.8rem; line-height: 1.55; overflow-wrap: anywhere; }
  @media (max-width: 900px) {
    .welcome { grid-template-columns: 1fr; }
    .shot { order: 2; }
    .shot picture { max-height: 360px; }
    .how { grid-template-columns: 1fr; }
    .feature-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .selfhost { grid-template-columns: 1fr; }
  }
  @media (max-width: 560px) {
    .welcome { margin-top: 18px; }
    h1 { font-size: clamp(2.3rem, 12vw, 3.15rem); }
    .shot picture { max-height: 260px; }
    .cta-row { flex-direction: column; align-items: stretch; }
    .cta-row .btn { flex: 1 1 100%; width: 100%; }
    .feature-grid { grid-template-columns: 1fr; }
    .command { padding: 12px; }
  }
</style>
