import type { TerminalMachine } from './terminal/api'

export type CatalogueAgent = { id: string; name: string; models: string[]; effort: boolean; enabled: boolean; resume?: boolean; yolo?: boolean; disabledReason?: string }
// The one table of tool names and model hints (agentLaunch.ts reads it; it must not import agentLaunch back). Display suggestions only: the connector owns executable names and arguments.
export const CATALOGUE: CatalogueAgent[] = [
  { id: 'claude', name: 'Claude Code', models: ['sonnet', 'opus', 'haiku'], effort: false, enabled: true },
  { id: 'codex', name: 'Codex', models: ['gpt-5-codex'], effort: false, enabled: true, yolo: true },
  { id: 'opencode', name: 'OpenCode', models: [], effort: false, enabled: true, resume: true, yolo: true },
  { id: 'gemini', name: 'Gemini CLI', models: ['gemini-2.5-pro', 'gemini-2.5-flash'], effort: false, enabled: true },
  { id: 'pi', name: 'Pi', models: [], effort: false, enabled: false, disabledReason: "reet doesn't start this agent yet." },
  { id: 'cursor', name: 'Cursor Agent', models: [], effort: false, enabled: false, disabledReason: "reet doesn't start this agent yet." },
  { id: 'cline', name: 'Cline', models: [], effort: true, enabled: true, yolo: true },
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
