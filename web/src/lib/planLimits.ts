// What is left of an agent's plan, for the places that show it beside the work: a chat's status strip and a Factory
// pipeline's card. The figures are the ones the machine's settings show (Machines > Settings > Agents); this only
// fetches them once for everything that asks, and keeps them for a couple of minutes.
import type { MachineAccounts } from './agentAccounts'
import { machineAccounts } from './terminal/api'

const KEEP = 2 * 60_000
const held = new Map<string, { at: number; answer: Promise<MachineAccounts | null> }>()

/** A machine's sign-ins and plan limits as the controller has them, or null where it has none to give. The controller
 *  answers what it kept while that is under five minutes old, and otherwise asks an online machine to read once. */
export function machinePlans(agentId: string): Promise<MachineAccounts | null> {
  const now = Date.now(), have = held.get(agentId)
  if (have && now - have.at < KEEP) return have.answer
  const answer = machineAccounts(agentId).catch(() => null)
  held.set(agentId, { at: now, answer })
  return answer
}

/** How often a page that stays open looks again. */
export const PLAN_REFRESH = KEEP
