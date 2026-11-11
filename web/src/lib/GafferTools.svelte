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
  const setGroup = (group: string, mode: GafferToolMode) => void save(tools.map(tool => tool.group === group ? { ...tool, mode } : tool), `${group}: all set to ${label(mode)}.`)
  const label = (mode: GafferToolMode) => mode === 'ask' ? 'Ask me' : mode === 'allow' ? 'Allow' : 'Off'
  const groupMode = (items: GafferToolChoice[]): GafferToolMode | '' => items.every(tool => tool.mode === items[0].mode) ? items[0].mode : ''
  onMount(() => { void load() })
</script>

<section class="tools" aria-label="Tools Gaffer can use">
  <h3>Tools Gaffer can use</h3>
  <p class="note">These are reet’s own tools, the same ones an MCP client gets. In your own chat with Gaffer it can use the ones you switch on, to do what you ask. <strong>Ask me</strong> shows you the exact call in the chat and runs it only when you approve. <strong>Allow</strong> runs it straight away. Gaffer’s goal reviews never get these tools, and in a session’s chat anything that changes something always asks first.</p>
  {#if loading}<p role="status">Reading Gaffer’s tools…</p>
  {:else}
    <p class="summary" role="status">{counts.allow} allowed · {counts.ask} ask first · {tools.length - counts.allow - counts.ask} off{#if notice} · {notice}{/if}</p>
    {#if error}<p class="feedback bad" role="alert">{error}</p>{/if}
    {#if risky.length}<p class="feedback warn" role="status">Allowed without asking: {risky.map(tool => tool.title).join(', ')}. Gaffer can do {risky.length === 1 ? 'this' : 'these'} on your word in a chat with no further click, including starting agents, deleting, or approving a merge. Choose Ask me if you want to see each one first.</p>{/if}
    {#each groups as group (group.name)}<details class="group" open={group.tools.some(tool => tool.mode !== 'off')}>
      <summary><span class="group-name">{group.name}</span><span class="group-count">{group.tools.filter(tool => tool.mode !== 'off').length} of {group.tools.length} on</span></summary>
      <div class="all"><label>Set all in {group.name}<select aria-label={`Set all tools in ${group.name}`} disabled={busy} value={groupMode(group.tools)} onchange={event => { const mode = event.currentTarget.value as GafferToolMode; if (mode) setGroup(group.name, mode) }}><option value="" disabled>Mixed</option><option value="off">Off</option><option value="ask">Ask me</option><option value="allow">Allow</option></select></label></div>
      <ul>{#each group.tools as tool (tool.name)}<li>
        <div class="about"><strong>{tool.title}</strong><code>{tool.name}</code><p>{tool.description}</p></div>
        <select aria-label={`${tool.title}: what Gaffer may do`} disabled={busy} value={tool.mode} onchange={event => setOne(tool.name, event.currentTarget.value as GafferToolMode)}><option value="off">Off</option><option value="ask">Ask me</option><option value="allow">Allow</option></select>
      </li>{/each}</ul>
    </details>{/each}
  {/if}
</section>

<style>
  .tools{display:grid;gap:12px;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--card);min-width:0}h3{margin:0;font:600 1.05rem var(--sans)}.note,.summary{margin:0;color:var(--muted);font-size:.9375rem;line-height:1.5}.note strong{color:var(--ink)}
  .feedback{margin:0;padding:8px 12px;border-radius:8px;border:1px solid var(--line)}.feedback.bad{border-color:var(--bad);color:var(--bad)}.feedback.warn{border-color:var(--accent)}
  .group{border:1px solid var(--line);border-radius:10px;min-width:0}.group summary{display:flex;justify-content:space-between;gap:12px;align-items:center;min-height:44px;padding:0 12px;cursor:pointer;font-weight:600}.group-count{font-weight:400;color:var(--muted);font-size:.875rem}
  .all{padding:8px 12px;border-top:1px solid var(--line)}.all label{display:flex;flex-wrap:wrap;gap:8px;align-items:center;justify-content:space-between;font-size:.875rem;color:var(--muted)}
  ul{list-style:none;margin:0;padding:0}li{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:12px;align-items:start;padding:10px 12px;border-top:1px solid var(--line)}
  .about{display:grid;gap:2px;min-width:0}.about code{font:0.8125rem var(--mono);color:var(--muted);overflow-wrap:anywhere}.about p{margin:0;color:var(--muted);font-size:.875rem;line-height:1.45;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:3;line-clamp:3;overflow:hidden}
  select{min-height:44px;padding:0 10px;border:1px solid var(--line-strong);border-radius:8px;background:var(--bg);color:var(--ink);font:inherit}select:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:560px){li{grid-template-columns:minmax(0,1fr)}select{width:100%}}
</style>
