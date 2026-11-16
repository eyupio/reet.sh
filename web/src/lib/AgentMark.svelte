<script lang="ts">
  // A small mark for an agent: its initial on a tile of its own colour. reet draws its own marks, not the vendors'
  // logos; the agent is always named beside the mark or in its tooltip, so the mark only has to tell agents apart.
  let { tool = '', size = 16 }: { tool?: string; size?: number } = $props()
  const MARKS: Record<string, [string, string, string]> = {
    claude: ['C', '#c8643c', '#fff7f2'], codex: ['X', '#2b2f2a', '#f4f5ef'], opencode: ['O', '#5a6472', '#f3f5f8'],
    cline: ['L', '#6d4bc2', '#f6f2ff'], antigravity: ['A', '#2f6fd0', '#f1f6ff'], copilot: ['P', '#3a7d44', '#f0f8f1'],
  }
  const mark = $derived(MARKS[tool] ?? [(tool || '?').slice(0, 1).toUpperCase(), '#6b7263', '#f5f6f0'])
</script>

<span class="agent-mark" style={`--size:${size}px;--tile:${mark[1]};--glyph:${mark[2]}`} aria-hidden="true">{mark[0]}</span>

<style>
  .agent-mark { flex: none; display: inline-grid; place-items: center; width: var(--size); height: var(--size); border-radius: calc(var(--size) * .28); background: var(--tile); color: var(--glyph); font: 700 calc(var(--size) * .62)/1 var(--sans); letter-spacing: 0; user-select: none; }
</style>
