// What a template's pages say, kept as plain functions so the wording can be tested without a page.
import type { GafferCrewTier, TemplateDiff, TemplateSkillGap } from '../api'
import { stageLabel, tierLabel } from '../gafferOutcome'

const short = (revision: string) => revision.slice(0, 8)
const quoted = (text: string) => `“${text}”`
// The server names a stage of a part as "code: API".
const stageName = (label: string) => { const [kind, ...part] = label.split(': '); return [stageLabel(kind), ...part].join(': ') }
const list = (kinds: string[]) => kinds.map(stageName).join(', ') || 'none'
/** A stage as the server's diff names it: the kind, with its part when it has one. */
export const diffStage = (stage: { kind: string; part?: string }) => (stage.part ? `${stage.kind}: ${stage.part}` : stage.kind)
/** The line said for a pipeline that changed in a way no other line describes. */
export const OTHER_CHANGE = 'Something else in this pipeline changed.'

function editLine(e: TemplateDiff['pipelines'][number]['stageEdits'][number]): string {
  const stage = stageName(e.stage)
  if (e.field === 'tier') return `${stage} tier changed from ${tierLabel(e.from)} to ${tierLabel(e.to)}.`
  // Worded as the criterion lines are: the quoted text ends the line.
  if (!e.from) return `${stage} ${e.field} added: ${quoted(e.to)}`
  if (!e.to) return `${stage} ${e.field} removed: ${quoted(e.from)}`
  return `${stage} ${e.field} changed: ${quoted(e.from)} to ${quoted(e.to)}`
}

/** The version to show changes since: the one last used, unless that is the one about to be used, or nothing was used yet. */
export const changesSince = (lastUsed: number | undefined, version: number): number => (lastUsed && lastUsed !== version ? lastUsed : 0)
/** Whether the latest version is newer than the one last used. A template never used has nothing to compare with. */
export const newerThanLastUsed = (t: { latest: number; lastUsedVersion?: number }): boolean => !!t.lastUsedVersion && t.latest > t.lastUsedVersion
/** Exactly the gaps shown, in the form a launch sends. */
export const skippedKeys = (gaps: readonly Pick<TemplateSkillGap, 'key'>[]): string[] => gaps.map(gap => gap.key)

export function gapWords(gap: Pick<TemplateSkillGap, 'name' | 'status'>): string {
  return gap.status === 'changed'
    ? `${quoted(gap.name)} has a newer approved revision. This template keeps the older one, so the stage starts without it.`
    : `${quoted(gap.name)} is not available now (archived, removed or switched off), so the stage starts without it.`
}

/** What changed between two versions, as lines for a person: a few general ones, then each pipeline that moved. */
export function diffLines(diff: TemplateDiff): { general: string[]; pipelines: { title: string; lines: string[] }[] } {
  const general = [
    ...(diff.nameChanged ? ['The name changed.'] : []), ...(diff.descriptionChanged ? ['The description changed.'] : []),
    ...(diff.objectiveChanged ? ['The objective changed.'] : []), ...(diff.limitsChanged ? ['The review limits changed.'] : []),
    ...(diff.orderChanged ? ['The pipelines run in another order.'] : []),
  ]
  const pipelines = diff.pipelines.filter(p => p.change !== 'same').map(p => {
    if (p.change === 'added') return { title: p.title, lines: ['Added.'] }
    if (p.change === 'removed') return { title: p.title, lines: ['Removed.'] }
    const lines: string[] = []
    if (p.promptChanged) lines.push('The prompt changed.')
    if (p.autoMergeChanged) lines.push('The merge setting changed.')
    if (p.stagesChanged && list(p.stagesBefore) !== list(p.stagesAfter)) lines.push(`Stages were ${list(p.stagesBefore)}; now ${list(p.stagesAfter)}.`)
    lines.push(...p.stageEdits.map(editLine))
    for (const text of p.criteria.added) lines.push(`Criterion added: ${quoted(text)}`)
    for (const text of p.criteria.removed) lines.push(`Criterion removed: ${quoted(text)}`)
    for (const one of p.criteria.reworded) lines.push(`Criterion reworded: ${quoted(one.from)} to ${quoted(one.to)}`)
    for (const s of p.skills) lines.push(!s.from ? `${quoted(s.name)} added to ${stageName(s.stage)} at revision ${short(s.to)}.` : !s.to ? `${quoted(s.name)} removed from ${stageName(s.stage)}.` : `${quoted(s.name)} in ${stageName(s.stage)} moved from revision ${short(s.from)} to ${short(s.to)}.`)
    // A pipeline marked changed never shows its title with nothing under it.
    return { title: p.title, lines: lines.length ? lines : [OTHER_CHANGE] }
  })
  return { general, pipelines }
}

/** What a refusal of a template write means for the person; where the server's own sentence says it best, that is kept.
 *  `latest` is the version read again after a template_changed refusal, so the words can say which one was saved elsewhere. */
export function conflictWords(code: string, said: string, latest = 0, deleting = false): string {
  if (code === 'template_changed') {
    if (!latest) return 'Another tab or session saved a newer version of this template. Nothing was changed. Try again.'
    return deleting ? `Version ${latest} was saved elsewhere, so nothing was deleted. Delete again if you still want to.` : `Version ${latest} was saved elsewhere. This now saves as version ${latest + 1}.`
  }
  if (code === 'template_request_reused') return 'That request was already used for something else. Try again.'
  return said
}

/** What a template's pipeline does at the end: the setting it was saved with. */
export const mergeWords = (autoMerge: boolean): string => (autoMerge ? 'Merges by itself once checks pass.' : 'Stops at Ready to merge.')
/** The review limits every session of a run from the template gets. */
export const limitsWords = (c: { maxReviews: number; hours: number }): string => `Up to ${c.maxReviews} ${c.maxReviews === 1 ? 'review' : 'reviews'} and ${c.hours} ${c.hours === 1 ? 'hour' : 'hours'} for each session.`
/** The line beside Start naming the template's pipelines that merge by themselves; empty when none does. */
export function mergeLine(auto: string[], yoloHolds: boolean): string {
  if (!auto.length) return ''
  const names = auto.length === 1 ? quoted(auto[0]) : `${auto.slice(0, -1).map(quoted).join(', ')} and ${quoted(auto.at(-1)!)}`
  return `${names} ${auto.length === 1 ? 'merges by itself' : 'merge by themselves'} once checks pass.${yoloHolds ? ' On YOLO they still stop at Ready to merge until you confirm on the Factory page that they may merge by themselves.' : ''}`
}

/** One line for a pipeline of a template: its title, stages, criteria and how it ends. */
export function pipelineLine(p: { title: string; autoMerge: boolean; stages: { kind: string }[]; criteria: unknown[] }): string {
  const criteria = `${p.criteria.length} ${p.criteria.length === 1 ? 'criterion' : 'criteria'}`
  return [p.title, list(p.stages.map(stage => stage.kind)), criteria, p.autoMerge ? 'merges by itself' : 'stops at Ready to merge'].join(' · ')
}

/** Whether the server would save this outcome as a template: it has started, and every pipeline finished, or had its delivery accepted and did not then fail. */
export function canSaveAsTemplate(o: { state: string; sessions: { state: string; stages?: unknown[]; deliveryAccepted?: boolean }[] }): boolean {
  return o.state !== 'starting' && o.sessions.length > 0 && o.sessions.every(p => {
    const ended = p.state === 'failed' || p.state === 'unfollowed' || p.state === 'cancelled'
    return (p.stages?.length ?? 0) > 0 && (p.state === 'done' || (!!p.deliveryAccepted && !ended))
  })
}

export const usedWords = (lastUsed: number | undefined, at: number | undefined, when: (at: number) => string): string =>
  !lastUsed ? 'Not used yet' : at ? `Last used ${when(at)} (version ${lastUsed})` : `Last used at version ${lastUsed}`

/** The heading of the changes shown before a start: since the version last used, or how an older one differs from it. A note when the version last used is no longer kept. */
export function changesWords(diff: Pick<TemplateDiff, 'from' | 'to' | 'olderNotShown'>, lastUsed: number | undefined): { title: string; note: string } {
  if (diff.to < diff.from) return { title: `Differs from version ${diff.from}, the one you last used`, note: '' }
  if (!diff.olderNotShown) return { title: `Changed since version ${diff.from}, the one you last used`, note: '' }
  return { title: `Changed since version ${diff.from}, the oldest kept`, note: `Version ${lastUsed}, the one you last used, is no longer kept, so changes before version ${diff.from} are not shown.` }
}

/** The gaps a template_skills_changed refusal carries, or null when it carries none and the template must be read again. */
export function refusedGaps(body: { gaps?: unknown } | undefined): TemplateSkillGap[] | null {
  return Array.isArray(body?.gaps) ? (body.gaps as TemplateSkillGap[]) : null
}

/** One tier of the crew a launch sends: what a stage on that tier is sent with, without the stage's own fields. */
export function tierCrew(stage: GafferCrewTier & { kind: string }): GafferCrewTier {
  const { tool, model, effort, provider, backup, moreBackups } = stage
  return { tool, ...(model ? { model } : {}), ...(effort ? { effort } : {}), ...(provider ? { provider } : {}), ...(backup ? { backup } : {}), ...(moreBackups ? { moreBackups } : {}) }
}
