<script lang="ts">
  // A small mark for an agent: its initial on a tile of its own colour. reet draws its own marks, not the vendors'
  // logos; the agent is always named beside the mark or in its tooltip, so the mark only has to tell agents apart.
  let { tool = '', size = 16 }: { tool?: string; size?: number } = $props()
  // Tiles take the theme's own colours, so the marks follow light and dark like everything else.
  const MARKS: Record<string, [string, string, string]> = {
    claude: ['C', 'var(--accent)', 'var(--accent-ink)'], codex: ['X', 'var(--ink)', 'var(--bg)'], opencode: ['O', 'var(--muted)', 'var(--bg)'],
    cline: ['L', 'var(--info)', 'var(--bg)'], antigravity: ['A', 'var(--repo)', 'var(--bg)'], copilot: ['P', 'var(--good)', 'var(--bg)'],
  }
  const mark = $derived(MARKS[tool] ?? [(tool || '?').slice(0, 1).toUpperCase(), 'var(--line-strong)', 'var(--ink)'])
</script>

<span class="agent-mark" style={`--size:${size}px;--tile:${mark[1]};--glyph:${mark[2]}`} aria-hidden="true">{mark[0]}</span>

<style>
  .agent-mark { flex: none; display: inline-grid; place-items: center; width: var(--size); height: var(--size); border-radius: calc(var(--size) * .28); background: var(--tile); color: var(--glyph); font: 700 calc(var(--size) * .62)/1 var(--sans); letter-spacing: 0; user-select: none; }
</style>
