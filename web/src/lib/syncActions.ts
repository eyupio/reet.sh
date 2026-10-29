import { getSyncState, saveDesired, startSyncRun, syncCandidates, type Skill, type SyncSkill, type SyncState } from './api'
import { fallbackInstallTarget, foldHost, pickSyncCandidate, savedInstallTargets } from './syncOverview'

export class MachineSourceRequired extends Error {}

export type InstallResult = {
  state: SyncState
  host: string
  targets: number
  createdScope: boolean
}

async function candidatesFor(sourceId: string, state: SyncState, cache: Map<string, Promise<SyncSkill[]>>): Promise<SyncSkill[]> {
  const key = sourceId || 'shelf'
  if (!cache.has(key)) {
    cache.set(key, (async () => {
      if (sourceId === 'hosted') return (await syncCandidates(0, 'hosted')).skills
      const list = /^list:(\d+)$/.exec(sourceId)
      if (list) return (await syncCandidates(Number(list[1]), 'shelf')).skills
      if (sourceId && sourceId !== 'shelf') {
        const env = (state.environments ?? []).find(item => item.id === sourceId)
        if (env) return env.skills ?? []
      }
      return (await syncCandidates(0, 'shelf')).skills
    })())
  }
  return cache.get(key)!
}

export async function installSkillOnMachine(skill: Pick<Skill, 'id' | 'slug' | 'name'>, host: string): Promise<InstallResult> {
  const state = await getSyncState()
  const machine = (state.machines ?? []).find(item => foldHost(item.host) === foldHost(host))
  if (!machine) throw new Error(`No machine named ${host} is connected. Refresh Machines and try again.`)
  const agent = (state.agents ?? []).find(item => foldHost(item.host) === foldHost(host))
  if (!agent) throw new Error(`${host} has no reet agent yet. Connect it once, then install from here in one click.`)
  if (agent.paused) throw new Error(`Sync is paused for ${host}. Resume it under Machines → Settings, then try again.`)

  const cache = new Map<string, Promise<SyncSkill[]>>()
  const shelf = await candidatesFor('shelf', state, cache)
  if (!pickSyncCandidate(shelf, skill)) throw new MachineSourceRequired(`${skill.name} needs a copy from a machine that has it.`)

  let next = state
  const targetIds: string[] = []
  for (const destination of savedInstallTargets(machine, state)) {
    const sourceCandidate = pickSyncCandidate(await candidatesFor(destination.candidateKey, state, cache), skill)
    const candidate = sourceCandidate ?? pickSyncCandidate(shelf, skill)!

    if (!destination.skillIds.includes(candidate.id)) {
      next = await saveDesired({
        targetId: destination.targetId,
        sourceId: destination.sourceId,
        listId: destination.listId,
        skillIds: [...destination.skillIds, candidate.id],
        mode: destination.mode,
        revision: destination.revision,
        followSource: !!sourceCandidate && destination.followSource,
      })
    }
    targetIds.push(destination.targetId)
  }

  let createdScope = false
  if (!targetIds.length) {
    const fallback = fallbackInstallTarget(machine, state)
    if (!fallback) throw new Error(`No recent complete agent scan on ${host}. Rescan the machine, then try again.`)
    if ((state.targets ?? []).some(target => target.target.id === fallback.targetId)) {
      throw new Error(`${skill.name} is not in the saved sync source for ${host}. Open Sync to add it to the existing set.`)
    }
    const candidate = pickSyncCandidate(shelf, skill)!
    next = await saveDesired({ targetId: fallback.targetId, sourceId: 'shelf', listId: 0, skillIds: [candidate.id], mode: 'additive', revision: '' })
    targetIds.push(fallback.targetId)
    createdScope = true
  }

  next = await startSyncRun(agent.id, targetIds)
  return { state: next, host: machine.host, targets: targetIds.length, createdScope }
}
