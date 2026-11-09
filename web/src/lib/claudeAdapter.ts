import type { TerminalMachine } from './terminal/api'

/** The adapter Gaffer talks to Claude Code through, at the version reet was checked against. */
export const CLAUDE_ADAPTER_INSTALL = 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1'

/**
 * What to tell a person whose machine has Claude Code but not the adapter its Gaffer chat needs, or null when there is
 * nothing to say. It speaks only when the machine itself reports the adapter missing: a machine that reports nothing about
 * it (an older connector), an adapter that is there, and a chat that is not offered to this account (its descriptor reads
 * unverified) all stay silent, so the hint never points at a step that would change nothing.
 */
export function claudeAdapterHint(machine: TerminalMachine | undefined): string | null {
  const caps = machine?.capabilities
  if (!machine || !caps?.tools?.includes('claude') || !Array.isArray(caps.acpAgents)) return null
  const entry = caps.acpAgents.find(agent => agent.tool === 'claude')
  if (entry?.state !== 'missing') return null
  return `Claude Code is installed on ${machine.host}, but the adapter Gaffer needs for it is not. To chat with it here, run this on that machine: ${CLAUDE_ADAPTER_INSTALL}`
}
