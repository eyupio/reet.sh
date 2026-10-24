export const MAX_BUNDLE_BYTES = 2 * 1024 * 1024
export const MAX_SUPPORTING_FILES = 100

export type ImportInput = {
  path: string
  size: number
  markdown?: string
  base64?: string
  unread?: boolean
}

export type PreparedImport = {
  key: string
  source: string
  name: string
  description: string
  content: string
  files: Record<string, string>
  problem: string
  selected: boolean
  size: number
  fileCount: number
}

export type SkillGroup = {
  mainIndex: number
  root: string
  indices: number[]
}

type Frontmatter = {
  normalized: string
  yaml: string
  body: string
  fields: Record<string, string>
}

const encoder = new TextEncoder()

export function normalizeImportPath(input: string): string {
  return input.replace(/\\/g, '/').split('/').filter(part => part && part !== '.').join('/')
}

function basename(input: string): string {
  const clean = normalizeImportPath(input)
  const at = clean.lastIndexOf('/')
  return at === -1 ? clean : clean.slice(at + 1)
}

function dirname(input: string): string {
  const clean = normalizeImportPath(input)
  const at = clean.lastIndexOf('/')
  return at === -1 ? '' : clean.slice(0, at)
}

function isMarkdown(path: string): boolean {
  return /\.(md|markdown)$/i.test(path)
}

function isSkillMarkdown(path: string): boolean {
  return /(^|\/)skill\.md$/i.test(normalizeImportPath(path))
}

function depth(path: string): number {
  return normalizeImportPath(path).split('/').length
}

function isAncestor(root: string, path: string): boolean {
  return root === '' || path === root || path.startsWith(root + '/')
}

function pathOf(inputs: ImportInput[], index: number): string {
  return normalizeImportPath(inputs[index].path)
}

function groupByPaths(inputs: ImportInput[]): SkillGroup[] {
  const mains = inputs
    .map((input, index) => ({ index, path: normalizeImportPath(input.path) }))
    .filter(item => isSkillMarkdown(item.path))
    .sort((a, b) => depth(b.path) - depth(a.path) || a.index - b.index)

  const groups: SkillGroup[] = mains.map(item => ({ mainIndex: item.index, root: dirname(item.path), indices: [] }))

  for (let index = 0; index < inputs.length; index++) {
    const path = pathOf(inputs, index)
    const own = groups.find(group => group.mainIndex === index)
    if (own) {
      own.indices.push(index)
      continue
    }
    const target = groups.find(group => isAncestor(group.root, path))
    if (target) target.indices.push(index)
  }

  return groups
}

function groupByLooseMarkdown(inputs: ImportInput[]): SkillGroup[] {
  const groups: SkillGroup[] = []
  for (let index = 0; index < inputs.length; index++) {
    const path = pathOf(inputs, index)
    if (path.includes('/') || !isMarkdown(path) || !inputs[index].markdown) continue
    const frontmatter = parseFrontmatter(inputs[index].markdown ?? '')
    if (!frontmatter || !frontmatter.fields.name || !frontmatter.fields.description?.trim()) continue
    groups.push({ mainIndex: index, root: '', indices: [index] })
  }
  return groups
}

function parseScalar(raw: string): string {
  const value = raw.trim()
  if (!value) return ''
  if (value.startsWith('"') && value.endsWith('"')) {
    return value.slice(1, -1).replace(/\\n/g, '\n').replace(/\\"/g, '"').replace(/\\\\/g, '\\')
  }
  if (value.startsWith("'") && value.endsWith("'")) {
    return value.slice(1, -1).replace(/''/g, "'")
  }
  return value.replace(/\s+#.*$/, '').trim()
}

function parseYamlFields(yaml: string): Record<string, string> {
  const lines = yaml.split('\n')
  const fields: Record<string, string> = {}
  for (let index = 0; index < lines.length; index++) {
    const match = /^([A-Za-z0-9_-]+)\s*:\s*(.*)$/.exec(lines[index])
    if (!match) continue
    const key = match[1]
    if (key in fields) continue
    const raw = match[2].trim()
    const block = /^([>|])[+-]?$/.exec(raw)
    if (block) {
      const parts: string[] = []
      let cursor = index + 1
      while (cursor < lines.length && (lines[cursor].trim() === '' || /^\s+/.test(lines[cursor]))) {
        parts.push(lines[cursor].replace(/^\s+/, ''))
        cursor++
      }
      while (parts.length && parts[parts.length - 1] === '') parts.pop()
      fields[key] = block[1] === '|' ? parts.join('\n') : parts.join(' ').replace(/\s+/g, ' ').trim()
      index = cursor - 1
    } else {
      fields[key] = parseScalar(raw)
    }
  }
  return fields
}

export function parseFrontmatter(input: string): Frontmatter | null {
  const normalized = input.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  if (!normalized.startsWith('---\n')) return null
  const close = normalized.indexOf('\n---\n', 4)
  if (close < 0) return null
  const yaml = normalized.slice(4, close)
  return { normalized, yaml, body: normalized.slice(close + 5), fields: parseYamlFields(yaml) }
}

export function sanitizeSkillName(input: string): string {
  return input
    .normalize('NFKD')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
    .replace(/-+$/g, '')
}

export function addReetPrefix(name: string): string {
  if (!name || name.startsWith('reet-')) return name
  const head = name.slice(0, 64 - 'reet-'.length).replace(/-+$/g, '')
  return head ? `reet-${head}` : name
}

export function uniqueSkillName(name: string, taken: Iterable<string>): string {
  const names = new Set(taken)
  const base = sanitizeSkillName(name) || 'skill'
  if (!names.has(base)) return base
  for (let suffix = 2; suffix < 10000; suffix++) {
    const tail = `-${suffix}`
    const head = base.slice(0, 64 - tail.length).replace(/-+$/g, '')
    const candidate = `${head || 'skill'}${tail}`
    if (!names.has(candidate)) return candidate
  }
  return base.slice(0, 59) + '-copy'
}

export function renameSkillContent(content: string, name: string): string {
  const frontmatter = parseFrontmatter(content)
  if (!frontmatter) return content
  const yaml = /^name\s*:/m.test(frontmatter.yaml)
    ? frontmatter.yaml.replace(/^name\s*:.*$/m, `name: ${name}`)
    : `name: ${name}\n${frontmatter.yaml}`
  return `---\n${yaml}\n---\n${frontmatter.body}`
}

export function validSupportingPath(name: string): boolean {
  if (!name || encoder.encode(name).length > 240) return false
  if (name.startsWith('/') || name.includes('\\') || /[\0\r\n?#%]/.test(name)) return false
  if (name.includes('..') || name === '.' || name.startsWith('.well-known/')) return false
  if (name.toLowerCase() === 'skill.md') return false
  return name.split('/').every(part => part && !part.startsWith('.'))
}

function isJunkSupportPath(path: string): boolean {
  return normalizeImportPath(path).split('/').some(part => (part.startsWith('.') && part !== '..') || part === '__MACOSX')
}

function displaySource(inputs: ImportInput[], group: SkillGroup): string {
  const path = pathOf(inputs, group.mainIndex)
  return group.root || basename(path) || 'SKILL.md'
}

function bundleProblem(input: ImportInput, frontmatter: Frontmatter | null, name: string): string {
  if (input.unread || input.markdown === undefined) return 'SKILL.md is larger than 2 MiB or could not be read.'
  if (!frontmatter) return 'SKILL.md must start with YAML frontmatter containing name and description.'
  if (!frontmatter.fields.description?.trim()) return 'Add a description to the SKILL.md frontmatter.'
  if (encoder.encode(frontmatter.fields.description.trim()).length > 1024) return 'The description must be 1024 bytes or fewer.'
  if (!frontmatter.body.trim()) return 'Add instructions below the SKILL.md frontmatter.'
  if (!name) return 'Give the skill a name of lowercase letters, numbers and hyphens.'
  return ''
}

export function prepareImports(inputs: ImportInput[], existingNames: Iterable<string>, prefix: boolean): PreparedImport[] {
  let groups = groupByPaths(inputs)
  if (!groups.length) groups = groupByLooseMarkdown(inputs)
  groups.sort((a, b) => displaySource(inputs, a).localeCompare(displaySource(inputs, b)) || a.mainIndex - b.mainIndex)

  const taken = new Set(existingNames)
  const prepared: PreparedImport[] = []

  for (const group of groups) {
    const main = inputs[group.mainIndex]
    const frontmatter = main.markdown === undefined ? null : parseFrontmatter(main.markdown)
    const fallbackName = group.root ? basename(group.root) : basename(pathOf(inputs, group.mainIndex)).replace(/\.(md|markdown)$/i, '')
    const baseName = sanitizeSkillName(frontmatter?.fields.name?.trim() || fallbackName) || 'skill'
    const prefixed = prefix ? addReetPrefix(baseName) : baseName
    const name = uniqueSkillName(prefixed, taken)
    taken.add(name)
    const description = frontmatter?.fields.description?.trim() || ''
    let problem = bundleProblem(main, frontmatter, name)
    const files: Record<string, string> = {}
    let size = frontmatter ? encoder.encode(renameSkillContent(frontmatter.normalized, name)).length : main.size
    let supporting = 0

    for (const index of group.indices) {
      if (index === group.mainIndex) continue
      const relative = group.root ? pathOf(inputs, index).slice(group.root.length + 1) : pathOf(inputs, index)
      if (isJunkSupportPath(relative)) continue
      if (!validSupportingPath(relative)) {
        problem ||= `Invalid supporting file path “${relative}”.`
        continue
      }
      if (inputs[index].unread || inputs[index].base64 === undefined) {
        problem ||= `Could not read supporting file “${relative}”.`
        continue
      }
      files[relative] = inputs[index].base64 as string
      supporting++
      size += inputs[index].size
    }

    if (supporting > MAX_SUPPORTING_FILES) problem ||= 'A skill can have at most 100 supporting files.'
    if (size > MAX_BUNDLE_BYTES) problem ||= 'A skill bundle must be 2 MiB or smaller.'

    prepared.push({
      key: String(group.mainIndex),
      source: displaySource(inputs, group),
      name,
      description,
      content: frontmatter ? renameSkillContent(frontmatter.normalized, name) : (main.markdown ?? ''),
      files,
      problem,
      selected: problem === '',
      size,
      fileCount: supporting + (main.markdown === undefined ? 0 : 1),
    })
  }

  return prepared
}
