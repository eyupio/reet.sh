import { listSkills, type Skill } from '../api'
import { useSkillPrompt } from '../install'
import { savedPrompts, type TerminalPrompt } from '../terminal/api'

/** The prompts a person saved, plus every skill's ready-to-use text keyed by slug and id. */
export type PromptLibrary = { prompts: TerminalPrompt[]; skills: Record<string, string> }

/** Reads both libraries for one picker opening. A failed skills read still leaves the
 *  person's prompts usable; only a prompt read failure is fatal to the picker. */
export async function readPromptLibrary(signal?: AbortSignal): Promise<PromptLibrary> {
  const promptRead = savedPrompts(signal)
  // A slow shelf must not hold up the prompts themselves. Skill tokens simply stay as written if it cannot answer.
  const shelfRead = Promise.race([
    listSkills({ tab: 'all', q: '', sort: 'name' }).catch((): { skills: Skill[] } => ({ skills: [] })),
    new Promise<{ skills: Skill[] }>(resolve => setTimeout(() => resolve({ skills: [] }), 10000)),
  ])
  const [prompts, shelf] = await Promise.all([promptRead, shelfRead])
  const skills: Record<string, string> = {}
  for (const skill of shelf.skills) {
    const text = useSkillPrompt(skill)
    if (!text) continue
    if (skill.slug) skills[skill.slug] = text
    if (skill.id) skills[skill.id] = text
  }
  return { prompts, skills }
}
