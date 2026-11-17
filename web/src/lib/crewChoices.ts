// The real choices for the account's crew in Gaffer settings. No one machine is in view there, so the models and
// efforts offered are those of every machine on the account, merged; a saved choice no machine offers is kept and named.
import type { MachineModels, ModelEntry } from './models'

/** One machine's cached model list for an agent, as GET /api/v2/models answers it. */
export type ModelsRow = {
  state: string; models?: (Omit<ModelEntry, 'provider'> & { provider?: string })[] | null; more?: boolean; source?: string; version?: string
  fetchedAt?: number | null; checkedAt?: number | null; reason?: string; sentence?: string
}

const providerOf = (id: string): string => { const slash = id.indexOf('/'); return slash < 0 ? '' : id.slice(0, slash) }

/** Every machine's list for one agent as one list, each model once and with its provider, in the order the machines
 *  list them. Null when no machine has read this agent's models. */
export function mergeMachineModels(tool: string, rows: readonly ModelsRow[]): MachineModels | null {
  if (!rows.length) return null
  const seen = new Set<string>()
  const models: ModelEntry[] = []
  for (const one of rows) for (const entry of one.models ?? []) {
    if (!entry?.id || seen.has(entry.id)) continue
    seen.add(entry.id)
    models.push({ id: entry.id, provider: entry.provider ?? providerOf(entry.id) })
  }
  const listed = rows.find(one => one.state === 'listed')
  const lead = listed ?? rows[0]
  return {
    tool, state: (lead.state as MachineModels['state']), models, more: rows.some(one => !!one.more), source: lead.source ?? '', version: lead.version ?? '',
    fetchedAt: lead.fetchedAt ?? null, checkedAt: lead.checkedAt ?? null, reason: lead.reason ?? '', sentence: lead.sentence ?? '',
  }
}

const ORDER = ['low', 'medium', 'high']

/** The levels any machine reports for an agent: low, medium and high first, then any others in the order reported.
 *  The agent's own "default" and the empty choice are not levels. */
export function effortUnion(lists: readonly (readonly string[])[]): string[] {
  const all = new Set<string>()
  for (const list of lists) for (const level of list) if (level && level !== 'default') all.add(level)
  return [...ORDER.filter(level => all.has(level)), ...[...all].filter(level => !ORDER.includes(level))]
}

export const effortLabel = (level: string): string => level ? level[0].toUpperCase() + level.slice(1) : ''

/** An agent in the crew's list. False means no machine on the account can run it as a chat; null, that the machines
 *  could not be read, so nothing is claimed. */
export const agentOptionLabel = (name: string, onMachines: boolean | null): string => onMachines === false ? `${name} (not on your machines)` : name

export type EffortChoice = { value: string; label: string }

/** A tier's effort select. `offered` is what the machines report for the agent, or null when they could not be read.
 *  A saved level no machine offers stays, labelled; `unoffered` asks for the hint that the stage then starts on the
 *  agent's default effort. With nothing reported and nothing saved there is nothing to choose. */
export function crewEffortChoices({ offered, saved }: { offered: readonly string[] | null; saved: string }): { options: EffortChoice[]; disabled: boolean; unoffered: boolean } {
  const level = saved.trim()
  if (offered === null) return { options: [{ value: '', label: 'Default effort' }, ...(level ? [{ value: level, label: effortLabel(level) }] : [])], disabled: false, unoffered: false }
  const unoffered = !!level && !offered.includes(level)
  if (!offered.length && !level) return { options: [{ value: '', label: 'No effort setting' }], disabled: true, unoffered: false }
  return {
    options: [
      { value: '', label: 'Default effort' }, ...offered.map(one => ({ value: one, label: effortLabel(one) })),
      ...(unoffered ? [{ value: level, label: `${effortLabel(level)} (not offered by your machines)` }] : []),
    ],
    disabled: false, unoffered,
  }
}
