import type { SetupProfileRevision } from './api'

export function setupContents(revision: SetupProfileRevision | null | undefined, machine = '', project = '') {
  const preview = revision?.preview, overrides = revision?.overrides
  const scopes = [overrides, overrides?.machines?.[machine], overrides?.projects?.[project]]
  const kinds = new Set(scopes.flatMap(scope => scope?.exclude_kinds ?? []))
  const sources = new Set(scopes.flatMap(scope => scope?.exclude_sources ?? []))
  const excluded = (kind: string, source: string, folder = false) => kinds.has(kind) || sources.has(source) || (folder && sources.has(source.replace(/[\\/][^\\/]+$/, '')))
  const agents = (preview?.agents ?? []).map(agent => ({ ...agent, components: (agent.components ?? []).filter(component => !excluded(component.kind, component.source)) }))
  const skills = (preview?.skills ?? []).filter(skill => !excluded('skills', skill.source, true))
  const memories = (preview?.memories ?? []).filter(memory => !excluded('memory', memory.source))
  const count = { components: agents.reduce((n, agent) => n + agent.components.length, 0), skills: skills.length, memories: memories.length }
  const captured = { components: (preview?.agents ?? []).reduce((n, agent) => n + (agent.components?.length ?? 0), 0), skills: preview?.skills?.length ?? 0, memories: preview?.memories?.length ?? 0 }
  return { agents, skills, memories, count, captured, kinds: [...kinds], excluded: captured.components + captured.skills + captured.memories - count.components - count.skills - count.memories }
}

export function compareSetups(before: SetupProfileRevision | undefined, after: SetupProfileRevision, machine = '', project = '') {
  function entries(revision: SetupProfileRevision | undefined) {
    const contents = setupContents(revision, machine, project)
    return new Map([
      ...contents.agents.flatMap(agent => agent.components.map(component => [`${agent.name}: ${component.kind} · ${component.source}`, JSON.stringify(component)] as const)),
      ...contents.skills.map(skill => [`Skill: ${skill.name} · ${skill.agent_id} · ${skill.source}`, JSON.stringify(skill)] as const),
      ...contents.memories.map(memory => [`Memory: ${memory.name} · ${memory.agent_id} · ${memory.source}`, JSON.stringify(memory)] as const),
    ])
  }
  const left = entries(before), right = entries(after)
  return { added: [...right.keys()].filter(key => !left.has(key)), removed: [...left.keys()].filter(key => !right.has(key)), changed: [...right.keys()].filter(key => left.has(key) && left.get(key) !== right.get(key)), exclusionsChanged: JSON.stringify(before?.overrides ?? {}) !== JSON.stringify(after.overrides ?? {}) }
}

export function shellQuote(value: string): string { return "'" + value.replaceAll("'", "'\\''") + "'" }
