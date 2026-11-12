import type { Skill } from './api'

export type InstallKind = 'command' | 'prompt'

const KEY = 'reet-install-kind'

export function readInstallKind(): InstallKind {
  try {
    return localStorage.getItem(KEY) === 'prompt' ? 'prompt' : 'command'
  } catch {
    return 'command'
  }
}

export function writeInstallKind(kind: InstallKind) {
  try {
    localStorage.setItem(KEY, kind)
  } catch {
    /* storage unavailable: the choice lasts until reload */
  }
}

export function sourceURL(skill: Skill): string | null {
  if (skill.sourceType === 'hosted') return skill.url
  if (skill.sourceType === 'github') return `https://github.com/${skill.source}`
  if (skill.sourceType === 'well-known') return `https://${skill.source}`
  return null
}

export function installCommand(skill: Skill): string | null {
  const url = sourceURL(skill)
  if (!url) return null
  return `npx skills add ${url} --skill ${skill.slug}`
}

export function installPrompt(skill: Skill): string | null {
  const url = sourceURL(skill)
  if (!url) return null
  return `Run \`npx skills use "${url}" --skill "${skill.slug}"\` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.`
}

/** The text to put in a prompt to use a skill. A skill with an install source is fetched and run; one without a source
 *  (a local copy reet knows about) is already on the machine, so the agent is asked to use it by name. reet is the source
 *  for a sourceless skill. */
export function useSkillPrompt(skill: Skill): string | null {
  const url = sourceURL(skill)
  if (url) return installPrompt(skill)
  const name = (skill.name || skill.slug || '').trim()
  if (!name) return null
  const blurb = skillBlurb(skill.description)
  return blurb ? `Use the ${name} skill: ${blurb}` : `Use the ${name} skill.`
}

const MAX_BLURB = 280

/** A skill's own description as one line: whitespace collapsed, cut at a word with an ellipsis when long, ending in
 *  punctuation. Mirrors skillBlurb in internal/api/terminal_skills.go. */
function skillBlurb(description: string | undefined): string {
  const text = (description || '').replace(/\s+/g, ' ').trim()
  const runes = Array.from(text)
  if (runes.length > MAX_BLURB) {
    let cut = runes.slice(0, MAX_BLURB).join('')
    const i = cut.lastIndexOf(' ')
    if (i > MAX_BLURB / 2) cut = cut.slice(0, i)
    return cut.replace(/[ ,;:.]+$/, '') + '…'
  }
  return text && !/[.!?…]$/.test(text) ? text + '.' : text
}
