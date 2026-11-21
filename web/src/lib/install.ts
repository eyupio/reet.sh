import type { Skill } from './api'

export type InstallKind = 'command' | 'prompt'

/**
 * The skills CLI package and version in every command reet copies out. It is the version sync runs on a machine
 * (SkillsCLI in internal/ref/cli.go; a test keeps the two equal), so a pasted command behaves like the one reet
 * ran and tested. How the CLI matches a skill's name has changed between versions.
 */
export const SKILLS_CLI = 'skills@1.7.4'

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

/**
 * What `--skill` is given. The skills CLI matches a skill by its own name, which
 * is not always its folder name ("SVG Logo Designer" in svg-logo-designer), so a
 * differing name is offered as well and the CLI takes whichever the source uses.
 */
export function skillSelector(skill: Pick<Skill, 'name' | 'slug'>): string {
  const name = (skill.name ?? '').trim()
  const plain = name !== '' && name.length <= 200 && !name.startsWith('-') && !name.includes('*') && !/[\u0000-\u001f\u007f]/.test(name)
  if (!plain || name.toLowerCase() === skill.slug.toLowerCase()) return skill.slug
  return `${skill.slug} '${name.replaceAll("'", "'\\''")}'`
}

export function installCommand(skill: Skill): string | null {
  const url = sourceURL(skill)
  if (!url) return null
  return `npx ${SKILLS_CLI} add ${url} --skill ${skillSelector(skill)}`
}

/**
 * What `skills use --skill` is given. It takes one value and matches the skill's own name, so a name that
 * differs from the slug is used in its place: `svg-logo-designer` finds nothing on skills 1.7.4 and
 * `uiux-design-review` finds nothing on any version, while "SVG Logo Designer" and "UI/UX Design Review" do.
 * The value sits in double quotes inside a code span, so a name that could end either is not used.
 */
export function useSelector(skill: Pick<Skill, 'name' | 'slug'>): string {
  const name = (skill.name ?? '').trim()
  const plain = name !== '' && name.length <= 200 && !name.startsWith('-') && !name.includes('*') && !/[\u0000-\u001f\u007f"`$\\]/.test(name)
  return plain && name.toLowerCase() !== skill.slug.toLowerCase() ? name : skill.slug
}

export function installPrompt(skill: Skill): string | null {
  const url = sourceURL(skill)
  if (!url) return null
  return `Run \`npx ${SKILLS_CLI} use "${url}" --skill "${useSelector(skill)}"\` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.`
}

/** Words to paste into a prompt to use a skill. A published skill is fetched from its public reet.sh page, so the
 *  prompt works on any machine, with or without the skill. Any other skill is used by name with its description,
 *  never a link: a sentence of the description that holds one is left out. The Skills page's Copy prompt. */
export function copyPrompt(row: { name: string; description?: string; published?: boolean; publicUrl?: string }): string {
  if (row.published && row.publicUrl && !/[\s"`$\\]/.test(row.publicUrl)) {
    const prompt = installPrompt({ sourceType: 'hosted', url: row.publicUrl, name: row.name, slug: row.name } as Skill)
    if (prompt) return prompt
  }
  const link = /https?:\/\/|www\.|github\.com/i
  const kept = (row.description ?? '').split(/(?<=[.!?])\s+/).filter(sentence => !link.test(sentence)).join(' ')
  const blurb = skillBlurb(kept)
  return blurb ? `Use the ${row.name} skill: ${blurb}` : `Use the ${row.name} skill.`
}

/** Copy prompt for a skill served from a reet.sh public page, in Community and lists: it fetches the skill from that
 *  page, so anyone can use it without installing it. null for a skill from anywhere else. */
export function hostedPrompt(skill: Skill): string | null {
  if (skill.sourceType !== 'hosted' || !skill.url) return null
  const prompt = copyPrompt({ name: skill.slug || skill.name, published: true, publicUrl: skill.url })
  return prompt.startsWith('Run ') ? prompt : null
}

/** The text a session's prompt pickers put in a prompt to use a library skill. reet's copy is on every machine, so the
 *  agent is asked to use it by name, published or not; the skill's original link is never used. */
export function useSkillPrompt(skill: Pick<Skill, 'name' | 'slug' | 'description'>): string | null {
  const name = (skill.name || skill.slug || '').trim()
  if (!name) return null
  return copyPrompt({ name, description: skill.description })
}

const MAX_BLURB = 280

/** A skill's own description as one line: whitespace collapsed, cut at a word with an ellipsis when long, ending in
 *  punctuation. Mirrors skillBlurb in internal/api/terminal_skills.go. */
export function skillBlurb(description: string | undefined): string {
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
