<script lang="ts">
  import type { MachineHealth } from '../terminal/api'
  import { hostLabel, type Project } from '../projects'

  // The saved Projects, read-only: name, the machines their folders are on and how many of their sessions run now. Choosing
  // one opens it; Add opens the existing New project dialog. Folder browsing arrives with PR A and PR B. Under 1240 px the
  // rail is a 56 px strip of the same links.
  let { projects, machines, running, busy, onadd }: {
    projects: Project[] | undefined
    /** For the machine names only; until they are read a member shows its machine id. */
    machines: MachineHealth[] | undefined
    running: Map<string, number>
    busy: boolean
    onadd: (opener: HTMLElement) => void
  } = $props()

  const id = $props.id()
  const hosts = (p: Project) => [...new Set(p.members.map(m => hostLabel(m.host, machines)))]
  const href = (p: Project) => `/projects/${encodeURIComponent(p.id)}`
  const initial = (name: string) => Array.from(name.trim())[0]?.toUpperCase() ?? '?'
</script>

<aside class="rail" aria-labelledby={`${id}-title`}>
  <div class="full">
    <header>
      <h2 id={`${id}-title`}>Projects</h2>
      <button type="button" class="add" aria-label="Add a project" disabled={busy} aria-busy={busy} onclick={(e) => onadd(e.currentTarget)}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>Add
      </button>
    </header>
    {#if !projects}
      <p class="none" role="status">Reading your Projects…</p>
    {:else if projects.length === 0}
      <p class="none">No Projects yet. Add one to start agents in its folders together.</p>
    {:else}
      <ul>
        {#each projects as p (p.id)}
          {@const n = running.get(p.id) ?? 0}
          <li>
            <a class="card" href={href(p)}>
              <span class="top">
                <svg class="diamond" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 2l10 10-10 10L2 12z" /></svg>
                <span class="name">{p.name}</span>
              </span>
              <span class="meta">
                {#each hosts(p) as h (h)}<span class="tag">{h}</span>{/each}
                <span class="state" class:on={n > 0}>{n > 0 ? `${n} running` : 'idle'}</span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
    <p class="foot">These are your saved Projects. Browsing a machine's folders comes in a later release.</p>
  </div>
  <div class="strip">
    <button type="button" class="add-icon" aria-label="Add a project" disabled={busy} aria-busy={busy} onclick={(e) => onadd(e.currentTarget)}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>
    </button>
    {#each projects ?? [] as p (p.id)}
      <a class="mini" href={href(p)} aria-label={p.name} title={p.name}><span aria-hidden="true">{initial(p.name)}</span>{#if (running.get(p.id) ?? 0) > 0}<i class="live" aria-hidden="true"></i>{/if}</a>
    {/each}
  </div>
</aside>

<style>
  .rail { border-right: 1px solid var(--line-soft); align-self: start; position: sticky; top: 0; max-height: 100dvh; overflow-y: auto; }
  .full { padding: 18px 14px; display: flex; flex-direction: column; gap: 12px; min-height: calc(100dvh - 61px); }
  header { display: flex; align-items: center; }
  h2 { flex-grow: 1; margin: 0; font: 600 11px/1.4 var(--sans); letter-spacing: 0.09em; text-transform: uppercase; color: var(--muted); }
  .add { display: flex; align-items: center; gap: 6px; height: 30px; padding: 0 10px; border: 1px solid var(--line-strong); border-radius: 8px; background: transparent; color: var(--ink); font-size: 13px; font-weight: 500; cursor: pointer; }
  .add svg, .add-icon svg { width: 14px; height: 14px; }
  .add:hover:not(:disabled), .add-icon:hover:not(:disabled) { border-color: var(--muted); }
  .add:disabled, .add-icon:disabled { opacity: 0.5; cursor: default; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .card { display: flex; flex-direction: column; gap: 6px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 11px; background: var(--card); color: var(--ink); text-decoration: none; }
  .card:hover { border-color: var(--line-strong); }
  .top { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .diamond { width: 14px; height: 14px; flex: none; fill: var(--repo); }
  .name { font-weight: 600; min-width: 0; overflow-wrap: anywhere; }
  .meta { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; font-size: 12px; color: var(--muted); }
  .tag { border: 1px solid var(--line-strong); border-radius: 99px; padding: 0 8px; font-size: 11px; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .state { margin-left: auto; }
  .state.on { color: var(--good); }
  .none { margin: 0; color: var(--muted); font-size: 13px; }
  .foot { margin: auto 0 0; padding: 10px 12px; border: 1px dashed var(--line-strong); border-radius: 11px; color: var(--muted); font-size: 13px; }
  .strip { display: none; }

  @media (max-width: 1239px) {
    .full { display: none; }
    .strip { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 18px 0; }
    .add-icon, .mini { position: relative; display: grid; place-items: center; width: 40px; height: 40px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card); color: var(--ink); text-decoration: none; font-weight: 600; cursor: pointer; }
    .add-icon { background: transparent; }
    .live { position: absolute; top: 4px; right: 4px; width: 7px; height: 7px; border-radius: 50%; background: var(--good); }
  }
  @media (pointer: coarse) and (max-width: 1239px) { .add-icon, .mini { width: 44px; height: 44px; } }
  @media (pointer: coarse) { .add { height: 44px; } }
</style>
