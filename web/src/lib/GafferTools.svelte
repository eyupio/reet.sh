<script lang="ts">
  // Which of reet's own tools Gaffer may use in your chats, tool by tool: Off, Ask me (Gaffer shows the exact call and
  // waits for your click) or Allow. Everything starts Off. The whole choice is saved on each change.
  import { onMount } from 'svelte'
  import { getGafferTools, saveGafferTools, type GafferToolChoice, type GafferToolMode } from './api'

  let tools = $state<GafferToolChoice[]>([]), loading = $state(true), busy = $state(false), error = $state(''), notice = $state('')
  const groups = $derived.by(() => {
    const out: { name: string; tools: GafferToolChoice[] }[] = []
    for (const tool of tools) {
      let group = out.find(item => item.name === tool.group)
      if (!group) { group = { name: tool.group, tools: [] }; out.push(group) }
      group.tools.push(tool)
    }
    return out.sort((a, b) => a.name.localeCompare(b.name))
  })
  const counts = $derived({ ask: tools.filter(tool => tool.mode === 'ask').length, allow: tools.filter(tool => tool.mode === 'allow').length })
  const risky = $derived(tools.filter(tool => tool.mode === 'allow' && (tool.group === 'Factory: merge' || tool.group === 'Factory: start agents' || tool.destructive)))

  async function load() {
    loading = true; error = ''
    try { tools = await getGafferTools() } catch (e) { error = e instanceof Error ? e.message : 'Gaffer’s tools could not be read.' }
    loading = false
  }
  async function save(next: GafferToolChoice[], said: string) {
    if (busy) return
    const before = tools
    tools = next; busy = true; error = ''; notice = ''
    try { tools = await saveGafferTools(Object.fromEntries(next.filter(tool => tool.mode !== 'off').map(tool => [tool.name, tool.mode]))); notice = said }
    catch (e) { tools = before; error = e instanceof Error ? e.message : 'That choice was not saved.' }
    busy = false
  }
  const setOne = (name: string, mode: GafferToolMode) => void save(tools.map(tool => tool.name === name ? { ...tool, mode } : tool), 'Saved.')
  const setAll = (mode: GafferToolMode) => void save(tools.map(tool => ({ ...tool, mode })), `Every tool set to ${label(mode)}.`)
  // Each tool has three choices, drawn as one small control, as connector settings do elsewhere.
  const choices: { mode: GafferToolMode; label: string; mark: string }[] = [{ mode: 'allow', label: 'Allow', mark: '✓' }, { mode: 'ask', label: 'Ask me', mark: '✋' }, { mode: 'off', label: 'Off', mark: '⊘' }]
  let closed = $state<Record<string, boolean>>({})
  const setGroup = (group: string, mode: GafferToolMode) => void save(tools.map(tool => tool.group === group ? { ...tool, mode } : tool), `${group}: all set to ${label(mode)}.`)
  const label = (mode: GafferToolMode) => mode === 'ask' ? 'Ask me' : mode === 'allow' ? 'Allow' : 'Off'
  const groupMode = (items: GafferToolChoice[]): GafferToolMode | '' => items.length && items.every(tool => tool.mode === items[0].mode) ? items[0].mode : ''
  onMount(() => { void load() })
</script>

<section class="tools" aria-label="Tools Gaffer can use">
  <h3>Tools Gaffer can use</h3>
  <p class="note">These are reet’s own tools, the same ones an MCP client gets. In your own chat with Gaffer it can use the ones you switch on, to do what you ask. <strong>Ask me</strong> shows you the exact call in the chat and runs it only when you approve. <strong>Allow</strong> runs it straight away. Gaffer’s goal reviews never get these tools, and in a session’s chat anything that changes something always asks first.</p>
  {#if loading}<p role="status">Reading Gaffer’s tools…</p>
  {:else}
    <div class="every"><p class="summary" role="status">{counts.allow} allowed · {counts.ask} ask first · {tools.length - counts.allow - counts.ask} off{#if notice} · {notice}{/if}</p>
      <label class="set">All tools<select aria-label="Set every tool" disabled={busy} value={groupMode(tools)} onchange={event => { const mode = event.currentTarget.value as GafferToolMode; if (mode) setAll(mode) }}><option value="" disabled>Custom</option><option value="allow">Allow</option><option value="ask">Ask me</option><option value="off">Off</option></select></label></div>
    {#if error}<p class="feedback bad" role="alert">{error}</p>{/if}
    {#if risky.length}<p class="feedback warn" role="status">Allowed without asking: {risky.map(tool => tool.title).join(', ')}. Gaffer can do {risky.length === 1 ? 'this' : 'these'} on your word in a chat with no further click, including starting agents, deleting, or approving a merge. Choose Ask me if you want to see each one first.</p>{/if}
    {#each groups as group (group.name)}<div class="group">
      <div class="group-head">
        <button class="fold" type="button" aria-expanded={!closed[group.name]} aria-label={`${closed[group.name] ? 'Show' : 'Hide'} the tools in ${group.name}`} onclick={() => closed = { ...closed, [group.name]: !closed[group.name] }}><span class="chevron" class:shut={closed[group.name]} aria-hidden="true">⌄</span><span class="group-name">{group.name}</span><span class="group-count">{group.tools.length}</span></button>
        <select aria-label={`Set all tools in ${group.name}`} disabled={busy} value={groupMode(group.tools)} onchange={event => { const mode = event.currentTarget.value as GafferToolMode; if (mode) setGroup(group.name, mode) }}><option value="" disabled>Custom</option><option value="allow">Allow</option><option value="ask">Ask me</option><option value="off">Off</option></select>
      </div>
      {#if !closed[group.name]}<ul>{#each group.tools as tool (tool.name)}<li>
        <details class="about"><summary>{tool.title}</summary><code>{tool.name}</code><p>{tool.description}</p></details>
        <div class="modes" role="radiogroup" aria-label={`${tool.title}: what Gaffer may do`}>
          {#each choices as choice (choice.mode)}<button type="button" role="radio" aria-checked={tool.mode === choice.mode} aria-label={choice.label} title={choice.label} class={choice.mode} disabled={busy} onclick={() => { if (tool.mode !== choice.mode) setOne(tool.name, choice.mode) }}><span aria-hidden="true">{choice.mark}</span></button>{/each}
        </div>
      </li>{/each}</ul>{/if}
    </div>{/each}
    <p class="note legend"><span aria-hidden="true">✓</span> Allow · <span aria-hidden="true">✋</span> Ask me · <span aria-hidden="true">⊘</span> Off. Open a tool’s name to read what it does.</p>
  {/if}
</section>

<style>
  .tools{display:grid;gap:12px;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--card);min-width:0}h3{margin:0;font:600 1.05rem var(--sans)}.note,.summary{margin:0;color:var(--muted);font-size:.9375rem;line-height:1.5}.note strong{color:var(--ink)}
  .every{display:flex;flex-wrap:wrap;gap:8px 16px;align-items:center;justify-content:space-between}.set{display:flex;gap:8px;align-items:center;font-size:.875rem;color:var(--muted)}
  .feedback{margin:0;padding:8px 12px;border-radius:8px;border:1px solid var(--line)}.feedback.bad{border-color:var(--bad);color:var(--bad)}.feedback.warn{border-color:var(--accent)}
  .group{border:1px solid var(--line);border-radius:10px;min-width:0}.group-head{display:flex;gap:12px;align-items:center;justify-content:space-between;padding:6px 12px}
  .fold{display:flex;gap:8px;align-items:center;min-height:44px;min-width:0;padding:0;border:0;background:none;color:var(--ink);font:inherit;font-weight:600;cursor:pointer;text-align:left}.chevron{display:inline-block;transition:transform .15s}.chevron.shut{transform:rotate(-90deg)}
  .group-count{font-weight:400;color:var(--muted);font-size:.875rem;padding:1px 8px;border:1px solid var(--line);border-radius:999px}
  ul{list-style:none;margin:0;padding:0}li{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:12px;align-items:start;padding:6px 12px;border-top:1px solid var(--line)}
  .about{min-width:0}.about summary{min-height:44px;display:flex;align-items:center;cursor:pointer;list-style:none}.about summary::-webkit-details-marker{display:none}.about summary:hover{text-decoration:underline}
  .about code{font:0.875rem var(--mono);color:var(--muted);overflow-wrap:anywhere}.about p{margin:4px 0 8px;color:var(--muted);font-size:.875rem;line-height:1.45}
  .modes{display:inline-flex;border:1px solid var(--line-strong);border-radius:8px;overflow:hidden;margin-top:4px}
  .modes button{width:44px;height:36px;border:0;border-left:1px solid var(--line);background:none;color:var(--muted);font:inherit;font-size:1rem;cursor:pointer}.modes button:first-child{border-left:0}
  .modes button[aria-checked="true"]{background:var(--tint,var(--bg));color:var(--ink);font-weight:700}.modes button.allow[aria-checked="true"]{color:var(--good)}.modes button.ask[aria-checked="true"]{color:var(--warn)}.modes button.off[aria-checked="true"]{color:var(--ink)}
  .modes button:disabled{opacity:.6;cursor:default}
  select{min-height:44px;padding:0 10px;border:1px solid var(--line-strong);border-radius:8px;background:var(--bg);color:var(--ink);font:inherit}
  select:focus-visible,.fold:focus-visible,.about summary:focus-visible,.modes button:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  .legend{font-size:.875rem}
  @media(prefers-reduced-motion:reduce){.chevron{transition:none}}
  @media(pointer:coarse){.modes button{height:44px}}
</style>
