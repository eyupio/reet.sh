<script lang="ts">
 import CopyButton from './CopyButton.svelte'
 import { DOCS_URL, REPO_URL, ROADMAP_URL } from './links'
 import ControlRoom from './ControlRoom.svelte'
 import StartFresh from './StartFresh.svelte'
 let {signupEnabled=false,contactEmail='',setupRequired=false}:{signupEnabled?:boolean;contactEmail?:string;setupRequired?:boolean}=$props()
 const installChip='curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh'
 const install=`${installChip}\nreet install`
 const docs=DOCS_URL
 // A visitor from search can try the product before signing in to an account they may not have. The full two-line
 // install stays in the self-host section. The line under the buttons is the first command, ready to copy.
 const chip=installChip
 const inviteHref=$derived(contactEmail?`mailto:${contactEmail}?subject=${encodeURIComponent('reet.sh account request')}&body=${encodeURIComponent('Hi, I would like an account on your reet.sh instance.')}`:'')
</script>
<section class="welcome" aria-labelledby="welcome-title">
 <div class="hero"><p class="eyebrow">Ey up. Your AI control room.</p><h1 id="welcome-title">One command on any machine.<br/><em>Your whole AI setup shows up, ready to control.</em></h1><p class="lede">reet.sh is the control room for your AI agents. Run them on any machine from one place, sync skills and setups across the lot, back it all up, and bring a brand-new, empty machine up to speed with one command.</p>
  <div class="cta-row">
   {#if setupRequired}
    <a class="btn primary" href="/setup">Set up this instance →</a><a class="btn quiet" href="#self-host">Host it yourself</a>
   {:else if signupEnabled}
    <a class="btn primary" href="/signup">Create an account</a><a class="btn" href="/signin">Sign in</a><a class="btn quiet" href="#self-host">Host it yourself</a>
   {:else}
    <a class="btn primary" href="#self-host">Install reet.sh →</a><a class="btn" href="/community">Explore the community</a>{#if inviteHref}<a class="btn" href={inviteHref}>Request an invite</a>{/if}<a class="btn quiet" href="/signin">Sign in</a>
   {/if}
  </div>
  <div class="enrol-chip"><code><span class="prompt">$</span> {chip}</code><CopyButton text={chip} compact label="Copy" ariaLabel="Copy the install command"/></div>
  {#if setupRequired}<p class="invite">Create the owner account, then connect your first machine.</p>{:else if !signupEnabled}<p class="invite">Already have an account? Sign in. Otherwise install reet.sh to run your own.</p>{/if}
  <div class="trust" aria-label="Why reet.sh"><span>Self-hosted</span><i></i><span>Free · AGPL-3.0</span><i></i><span>No telemetry</span></div>
 </div>
 <ControlRoom />
</section>
<section class="tiles" aria-label="What reet.sh does">
 <article><h2>Command</h2><p>Open a terminal on any machine, start an agent in a folder, and pick the session up again later.</p><div class="cap"><code>claude code · hetzner-vps</code><code class="live"><span aria-hidden="true">●</span> session running</code></div></article>
 <article><h2>Sync</h2><p>Send a skill or setup to one machine or the whole fleet. You review the exact plan before anything changes.</p><div class="cap"><code><span class="chip">code-review</span> <span class="chip">→ 3 machines</span></code><code class="live">plan ready · approve to apply</code></div></article>
 <article><h2>Back up</h2><p>Setups and skills are kept on the controller, sealed and verified, so a machine can be put back as it was.</p><div class="cap"><code>setup: web-dev</code><code class="live">✓ sealed · verified 2 h ago</code></div></article>
 <article><h2>Start fresh</h2><p>Enrol an empty machine and it picks up your skills and setups. It ends up like the rest.</p><div class="cap"><code>fresh-vps · 0 skills</code><code class="live">→ 4 skills, 1 setup · in sync</code></div></article>
</section>
<section class="how" aria-label="How reet.sh works">
 <article><span>01</span><h2>Enrol a machine with one command.</h2><p>Run the agent on a laptop, a VPS or a lab box. It dials out to your controller, so there is no port to open, and reports in with the agents, skills and setups it already has.</p></article>
 <article><span>02</span><h2>Command your agents from one place.</h2><p>Open a terminal on any enrolled machine, start an agent in a folder, keep sessions running and pick them up later.</p></article>
 <article><span>03</span><h2>Sync and restore.</h2><p>Push a skill or setup to one machine or all of them, review the plan first, and restore a setup on a machine that has nothing.</p></article>
</section>
<section class="guides" aria-labelledby="guides-title">
 <header><p class="eyebrow">Guides</p><h2 id="guides-title">Start with a job to do.</h2></header>
 <div class="guide-links">
  <a href="/docs/run-claude-code-on-a-vps/"><strong>Run Claude Code on a VPS</strong><span>Open a browser terminal and start the agent already on the machine.</span></a>
  <a href="/docs/sync-skills-between-machines/"><strong>Sync skills between machines</strong><span>Scan, preview the exact plan, approve it, and check the result.</span></a>
  <a href="/docs/self-host-mcp-server/"><strong>Connect an AI client with MCP</strong><span>Give Claude or another client OAuth access to your private shelf.</span></a>
 </div>
</section>
<StartFresh />
<section class="fleet-story" aria-labelledby="fleet-story-title">
 <header><p class="eyebrow">Keep the whole lot in good order</p><h2 id="fleet-story-title">The workspace around your agents.</h2><p>Machines, reusable setups, skills and ongoing work belong together. You decide what moves between them.</p></header>
 <div class="capability-grid">
  <article><h3>Your fleet, in view.</h3><p>Every enrolled machine in one control room: what it has installed, whether it is in sync, and the work already running there.</p></article>
  <article><h3>Setups worth keeping.</h3><p>Save and share named AI setup profiles through reet. Pull a known setup when preparing another machine, with restore and verification in the CLI.</p></article>
  <article><h3>Skills that travel.</h3><p>Keep a central skill library, bring machine discoveries back, and choose a source and destinations for distribution.</p></article>
  <article><h3>Sync, on your terms.</h3><p>Choose one machine or the fleet. Review exact plans and resolve conflicts before applying changes. A machine applies a plan sent from here only when its owner has opted in on that machine.</p></article>
 </div>
 <div class="next-up"><span>On the workbench</span><p>Agent software: OpenCode today, more agents to come. Fleet-wide agent configuration is on the roadmap.</p><a href={ROADMAP_URL} target="_blank" rel="noopener noreferrer">See what’s next →</a></div>
</section>
<section class="ownership" aria-labelledby="ownership-title"><div><p class="eyebrow">Proper control, on your own kit</p><h2 id="ownership-title">Your infrastructure.<br/>Your call.</h2></div><p>Run reet.sh on your own infrastructure. Keep control of accounts, local shell consent, skills, setup profiles and reviewed fleet changes. One application, one SQLite file, with no telemetry.</p></section>
<section class="self-host" id="self-host" aria-labelledby="self-host-title"><div><p class="eyebrow">Make yourself at home</p><h2 id="self-host-title">Give your machines a home.</h2><p>Install reet.sh, choose your deployment, then bring your first machine and its AI toolkit into the workspace.</p><div class="self-links"><a class="btn" href={docs} target="_blank" rel="noopener noreferrer">Installation guide →</a><a href={REPO_URL} target="_blank" rel="noopener noreferrer">Source on GitHub →</a></div></div><div class="install"><header><span>Install and set up</span><CopyButton text={install} label="Copy install command"/></header><pre>{install}</pre></div></section>
<style>
 .fleet-story{padding:42px 0 0}.fleet-story header{max-width:650px}.fleet-story h2{font:500 clamp(1.8rem,3vw,2.6rem)/1.15 var(--serif);letter-spacing:-.03em;margin:0 0 14px}.fleet-story header>p:last-child{color:var(--muted);font-size:.88rem;line-height:1.8;margin:0}.capability-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;margin:26px 0}.capability-grid article{padding:22px;border:1px solid var(--line);border-radius:12px;background:var(--card)}.capability-grid h3{font-size:.9rem;margin:0 0 10px}.capability-grid article>p{font-size:.82rem;line-height:1.8;color:var(--muted);margin:0}.next-up{display:flex;align-items:center;gap:18px;padding:18px 0;border-bottom:1px solid var(--line);font-size:.75rem}.next-up>span{white-space:nowrap;color:var(--accent-text);font-weight:600}.next-up p{color:var(--muted);line-height:1.7;margin:0}.next-up>a{white-space:nowrap;text-decoration:none}@media(max-width:550px){.capability-grid{grid-template-columns:1fr}.next-up{display:grid;gap:8px}.fleet-story{padding-top:30px}}
 .welcome{display:grid;grid-template-columns:1fr 1.05fr;gap:48px;align-items:center;padding:34px 0 40px}.hero{min-width:0}.eyebrow{font-size:.67rem;font-weight:600;letter-spacing:.12em;text-transform:uppercase;color:var(--accent-text);margin:0 0 18px}h1{font:500 clamp(2.5rem,4.5vw,4rem)/1.06 var(--serif);letter-spacing:-.045em;margin:0}h1 em{font-style:normal;color:var(--accent-text)}.lede{font-size:1rem;line-height:1.8;color:var(--muted);max-width:51ch;margin:24px 0}.cta-row{display:flex;flex-wrap:wrap;gap:8px}.cta-row .btn{font-size:.82rem;padding:10px 14px;min-height:40px}.invite{font-size:.8rem;color:var(--muted);margin:15px 0;line-height:1.6}.trust{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-top:26px;color:var(--muted);font-size:.8rem}.trust i{height:3px;width:3px;border-radius:50%;background:var(--line)}
 .enrol-chip{display:flex;align-items:center;justify-content:space-between;gap:10px;max-width:max-content;min-width:0;margin-top:14px;padding:5px 6px 5px 14px;border:1px solid var(--line);border-radius:10px;background:var(--card)}.enrol-chip code{font:.78rem var(--mono);white-space:nowrap;color:var(--ink);min-width:0;overflow-wrap:anywhere}.enrol-chip .prompt{color:var(--accent-text)}
 .tiles{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;padding:6px 0 40px}.tiles article{display:flex;flex-direction:column;min-width:0;padding:20px;border:1px solid var(--line);border-radius:12px;background:var(--card)}.tiles h2{font:500 1.2rem var(--serif);letter-spacing:-.02em;margin:0 0 8px}.tiles p{font-size:.82rem;line-height:1.7;color:var(--muted);margin:0 0 16px}.tiles .cap{display:grid;gap:4px;margin-top:auto;padding-top:12px;border-top:1px dashed var(--line-strong)}.tiles .cap code{font:.75rem/1.5 var(--mono);color:var(--muted);overflow-wrap:anywhere}.tiles .cap .live{color:var(--good)}.tiles .chip{display:inline-block;padding:0 7px;border:1px solid var(--line-strong);border-radius:6px;color:var(--ink)}
 .how{display:grid;grid-template-columns:repeat(3,1fr);gap:28px;padding:35px 0;border-top:1px solid var(--line);border-bottom:1px solid var(--line)}.guides{padding:38px 0;border-bottom:1px solid var(--line)}.guides header{display:flex;align-items:baseline;justify-content:space-between;gap:18px;margin-bottom:18px}.guides .eyebrow{font-size:.7rem}.guides h2{font:500 clamp(1.6rem,3vw,2.4rem)/1.15 var(--serif);letter-spacing:-.03em;margin:0}.guide-links{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px}.guide-links a{display:grid;gap:6px;padding:18px;border:1px solid var(--line);border-radius:12px;background:var(--card);text-decoration:none;color:var(--ink)}.guide-links a:hover{border-color:var(--line-strong)}.guide-links strong{font:600 .95rem var(--sans)}.guide-links span{font-size:.8rem;line-height:1.6;color:var(--muted)}.how article>span{font:.72rem var(--mono);color:var(--accent-text)}.how h2{font:500 1.35rem var(--serif);letter-spacing:-.02em;margin:12px 0 10px}.how p{font-size:.82rem;line-height:1.8;color:var(--muted);margin:0}.ownership{display:grid;grid-template-columns:1fr 1fr;gap:48px;align-items:center;padding:54px 0}.ownership h2,.self-host h2{font:500 clamp(1.7rem,3vw,2.6rem)/1.15 var(--serif);letter-spacing:-.035em;margin:0}.ownership>p,.self-host p:not(.eyebrow){font-size:.88rem;line-height:1.8;color:var(--muted);margin:0}.self-host{display:grid;grid-template-columns:1fr 1fr;gap:36px;padding:32px;border:1px solid var(--line);border-radius:14px;background:var(--card);margin-bottom:25px}.self-host p:not(.eyebrow){margin:14px 0}.self-links{display:flex;align-items:center;gap:14px;flex-wrap:wrap;font-size:.74rem}.self-links .btn{font-size:.75rem}.install{border:1px solid var(--line);border-radius:10px;background:var(--bg);align-self:center;min-width:0}.install>header{display:flex;justify-content:space-between;align-items:center;gap:10px;padding:10px 12px;border-bottom:1px solid var(--line);font-size:.68rem;color:var(--muted)}pre{font:.7rem/1.8 var(--mono);padding:14px;margin:0;overflow-wrap:anywhere;white-space:pre-wrap;word-break:break-word;color:var(--ink)}
 @media(max-width:1000px){.tiles{grid-template-columns:repeat(2,minmax(0,1fr))}}
 @media(max-width:850px){.welcome{grid-template-columns:1fr;gap:30px;padding:20px 0 28px}.lede{max-width:65ch}.how{gap:20px}.guides header{display:block}.guide-links{grid-template-columns:1fr}.ownership{gap:24px}.self-host{grid-template-columns:1fr;padding:24px}}
 @media(max-width:550px){h1{font-size:2.65rem}.lede{font-size:.9rem}.tiles{grid-template-columns:1fr;gap:12px;padding-bottom:30px}.tiles article{padding:18px}.enrol-chip{max-width:100%;padding-left:10px}.enrol-chip code{font-size:.7rem}.how,.ownership{grid-template-columns:1fr;gap:22px}.how article{display:grid;grid-template-columns:25px 1fr;column-gap:10px}.how article>span{grid-row:span 2;margin-top:5px}.how h2{margin:0 0 8px}.how p{grid-column:2}.ownership{padding:34px 0}.ownership .eyebrow{margin-bottom:12px}.self-host{gap:20px;padding:20px}.trust{gap:8px}}
 @media(max-width:700px){.enrol-chip code{white-space:normal}}
</style>
