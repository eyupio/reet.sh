// What waits for the person, by one rule for every page: Home's Waiting for you, the bell, the tab title. It takes what
// the pages read (sessions, the Factory's outcomes, Gaffer's goals) and gives the entries; each page only draws them.
import type { GafferGoalOverview, GafferOutcome } from '../api'
import type { MachineHealth, TerminalSession } from '../terminal/api'
import { approvalOf, foldGafferGoals, needsYou, type NeedCard, type SessionOwner } from '../home'
import { factoryItems, type FactoryItem } from '../factory/factoryList'
import { homeWaiting, type GoalNeed, type WaitingEntry } from './homeWaiting'
import { deskOf, type DeskLookup } from './deskSessions'

export type WhatWaits = { entries: WaitingEntry[]; items: FactoryItem[]; cards: NeedCard[] }

const sessionHref = (agentId: string, id: string) => `/sessions?agent=${encodeURIComponent(agentId)}&session=${encodeURIComponent(id)}`

/** `outcomes` may include archived ones (they are left out); `goals` undefined means not read. A session on a desk
 *  (`desks`) is counted as any other, marked with its desk and opening on it; a pipeline's session stays its item. */
export function whatWaits(
  sessions: TerminalSession[],
  now: number,
  owners: ReadonlyMap<string, SessionOwner>,
  machines: readonly Pick<MachineHealth, 'agentId' | 'host'>[],
  outcomes: GafferOutcome[],
  goals: GafferGoalOverview[] | undefined,
  desks: DeskLookup = {},
): WhatWaits {
  // A goal on a session that already has a card is that card's second reason.
  const cards = foldGafferGoals(needsYou(sessions, now, owners, machines), goals).cards
  const floor = outcomes.filter(record => !record.archivedAt)
  const byConversation = new Map((goals ?? []).map(row => [row.goal.conversationId, row]))
  const held = new Map(sessions.flatMap(s => s.pendingPermission?.permissionId ? [[s.id, s.pendingPermission] as const] : []))
  const items = factoryItems(floor, byConversation, new Set(held.keys()))
  // A pipeline's own goals (each stage's conversation, and the ones it retired) are its item, never a goal entry; an
  // archived pipeline's goals are not entries either.
  const piped = new Set(outcomes.flatMap(record => record.sessions.flatMap(piece => [piece.conversationId, ...(piece.stages ?? []).map(stage => stage.conversationId), ...(piece.retired ?? [])])).filter((id): id is string => !!id))
  const host = (agentId: string) => machines.find(m => m.agentId === agentId)?.host ?? ''
  const needs: GoalNeed[] = (goals ?? []).flatMap(row => {
    const id = row.session?.id || row.goal.sessionId
    if (!row.attention || !id || piped.has(row.goal.conversationId)) return []
    const agentId = row.session?.agentId ?? sessions.find(s => s.id === id)?.agentId ?? ''
    return [{
      id: row.goal.conversationId, sessionId: id, title: row.session?.title || sessions.find(s => s.id === id)?.title || 'A session',
      where: [owners.get(id)?.name, host(agentId)].filter(Boolean).join(' · '), href: sessionHref(agentId, id),
      reason: row.reason ?? '', since: row.goal.updatedAt,
    }]
  })
  const pipelineSessions = new Set(items.map(item => item.sessionId).filter(Boolean))
  const on = (sessionId: string) => pipelineSessions.has(sessionId) ? undefined : deskOf(desks.desks, sessionId, desks.panes)
  const entries = homeWaiting(items, cards, needs).map((entry): WaitingEntry => {
    if (entry.source === 'session') { const desk = on(entry.card.sessionId); return desk ? { ...entry, desk, card: { ...entry.card, href: desk.href } } : entry }
    if (entry.source === 'gaffer') { const desk = on(entry.goal.sessionId); return desk ? { ...entry, desk, goal: { ...entry.goal, href: desk.href } } : entry }
    if (entry.item.kind !== 'approval') return entry
    const pending = held.get(entry.item.sessionId)
    return pending ? { ...entry, approval: approvalOf(pending) } : entry
  })
  return { entries, items, cards }
}
