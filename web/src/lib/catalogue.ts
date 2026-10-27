import type { TerminalMachine } from './terminal/api'
import { modelSuggestions } from './agentLaunch'

export type CatalogueAgent = { id: string; name: string; models: string[]; effort: boolean; enabled: boolean; disabledReason?: string }
// Display suggestions only: the connector owns executable names and arguments.
export const CATALOGUE: CatalogueAgent[] = [
  { id: 'claude', name: 'Claude Code', models: modelSuggestions('claude'), effort: false, enabled: true },
  { id: 'codex', name: 'Codex', models: modelSuggestions('codex'), effort: false, enabled: true },
  { id: 'opencode', name: 'OpenCode', models: modelSuggestions('opencode'), effort: false, enabled: true },
  { id: 'gemini', name: 'Gemini CLI', models: modelSuggestions('gemini'), effort: false, enabled: true },
  { id: 'pi', name: 'Pi', models: [], effort: false, enabled: false, disabledReason: "reet doesn't start this agent yet." },
  { id: 'cursor', name: 'Cursor Agent', models: [], effort: false, enabled: false, disabledReason: "reet doesn't start this agent yet." },
  { id: 'cline', name: 'Cline', models: [], effort: true, enabled: true },
]
const ORIGINAL = new Set(['claude', 'codex', 'opencode', 'gemini'])
export function agentChoices(machine: TerminalMachine): { agent: CatalogueAgent; available: boolean; reason?: string }[] {
  return CATALOGUE.map(agent => {
    if (!agent.enabled) return { agent, available: false, reason: agent.disabledReason ?? "reet doesn't start this agent yet." }
    if (!machine.capabilities?.launch || (!machine.capabilities.agentsV2 && !ORIGINAL.has(agent.id))) return { agent, available: false, reason: 'Update reet on that machine' }
    if (!machine.capabilities.tools?.includes(agent.id)) return { agent, available: false, reason: `Not installed on ${machine.host}` }
    return { agent, available: true }
  }).sort((a, b) => Number(b.available) - Number(a.available) || Number(b.agent.enabled) - Number(a.agent.enabled))
}
