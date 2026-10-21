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

function sourceURL(skill: Skill): string | null {
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
