import type { TerminalMachine } from './terminal/api'

type ChatAdapter = { name: string; adapter: string; install: string }

// The adapters Gaffer talks to each agent through, at the versions reet was checked against. Claude Code's adapter brings its own native
// binary. Codex's adapter depends on @openai/codex and runs that bundled copy (reet does not set CODEX_PATH), so installing it brings a Codex CLI of its
// own; the person's own `codex` shows that Codex is on the machine and runs terminal sessions, but not the chat.
const ADAPTERS: Record<string, ChatAdapter> = {
  claude: { name: 'Claude Code', adapter: 'claude-agent-acp', install: 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1' },
  codex: { name: 'Codex', adapter: 'codex-acp', install: 'npm install --global @agentclientprotocol/codex-acp@1.13.1' },
}

/** The agents whose missing chat adapter has an install hint. */
export const HINTED_TOOLS = Object.keys(ADAPTERS)

/** The command that installs the adapter for a tool, or '' when the tool has no hint. */
export function chatAdapterInstall(tool: string): string { return ADAPTERS[tool]?.install ?? '' }

/**
 * What to tell a person whose machine has an agent but not the adapter its Gaffer chat needs, or null when there is nothing to say.
 * It speaks only when the machine itself reports that agent's adapter missing: a machine that reports nothing about it (an older
 * connector), an adapter that is there, and a chat that is not offered to this account (its descriptor reads unverified) all stay
 * silent, so the hint never points at a step that would change nothing.
 */
export function chatAdapterHint(tool: string, machine: TerminalMachine | undefined): string | null {
  const known = ADAPTERS[tool]
  const caps = machine?.capabilities
  if (!known || !machine || !caps?.tools?.includes(tool) || !Array.isArray(caps.acpAgents)) return null
  const entry = caps.acpAgents.find(agent => agent.tool === tool)
  if (entry?.state !== 'missing') return null
  return `${known.name} is installed on ${machine.host}, but reet there cannot find the adapter its Gaffer chat needs (${known.adapter}) on its PATH. If it is not installed, run this on that machine: ${known.install}`
}
