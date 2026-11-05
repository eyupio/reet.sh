// Finding git repositories on a machine: the folders a browse page says are repository tops, and the folders worth
// opening next. Pure: no DOM, no fetch. The machine's own browse answer decides what is a repository; nothing here reads
// a folder, and nothing claims a repository is ready to work in.

import type { BrowseResult } from './terminal/api'

export type DiscoveredRepo = { host: string; dir: string; name: string }
export type ScanFolder = { path: string; depth: number }

/** Folders that never hold the person's own repositories, or whose contents would flood a scan. */
const SKIP = new Set([
  'node_modules', 'vendor', 'dist', 'build', 'out', 'target', 'bin', 'obj', 'coverage',
  'library', 'caches', 'cache', 'tmp', 'temp', 'logs', 'downloads', 'movies', 'music', 'pictures',
  'go', 'pkg', 'snap', 'var', 'usr', 'etc', 'opt', 'srv', 'mnt', 'media',
])

/** Names people usually keep repositories under; scanned before the rest of a folder. */
const PRIORITY = ['code', 'src', 'projects', 'project', 'work', 'workspace', 'dev', 'repos', 'repo', 'git', 'github',
  'documents', 'desktop', 'sites', 'apps', 'app', 'clients', 'client', 'personal', 'experiments']

const lower = (s: string) => s.trim().toLowerCase()

/** Whether a folder's name is one the scan leaves alone. */
export function skipFolder(name: string): boolean {
  const n = lower(name)
  return n === '' || n.startsWith('.') || SKIP.has(n)
}

/** A folder inside `parent`, by the name the machine listed. */
export function joinFolder(parent: string, name: string): string {
  const p = String(parent ?? '')
  if (p === '') return `/${name}`
  return p.endsWith('/') ? `${p}${name}` : `${p}/${name}`
}

/** The repository tops in one browse page, each with the machine that listed it. */
export function reposFrom(result: Pick<BrowseResult, 'path' | 'entries'>, host: string): DiscoveredRepo[] {
  const out: DiscoveredRepo[] = []
  for (const entry of result.entries ?? []) {
    if (!entry.dir || !entry.repoRoot) continue
    out.push({ host, dir: joinFolder(result.path, entry.name), name: entry.name })
  }
  return out
}

/** The folders in one browse page worth opening next: ordinary folders, not repository tops, not names the scan skips. */
export function nextFolders(result: Pick<BrowseResult, 'path' | 'entries'>, depth: number, maxDepth: number, seen: ReadonlySet<string>): ScanFolder[] {
  if (depth >= maxDepth) return []
  const out: ScanFolder[] = []
  for (const entry of result.entries ?? []) {
    if (!entry.dir || entry.repoRoot || skipFolder(entry.name)) continue
    const path = joinFolder(result.path, entry.name)
    if (seen.has(path)) continue
    out.push({ path, depth: depth + 1 })
  }
  return out
}

/** The scan's order: folders below a usual repository parent first, then shallow folders, then by path. */
export function scanOrder(a: ScanFolder, b: ScanFolder): number {
  const rank = (f: ScanFolder) => {
    const parts = f.path.split('/').filter(Boolean).map(lower)
    const at = parts.reduce((best, part) => {
      const found = PRIORITY.indexOf(part)
      return found >= 0 && (best < 0 || found < best) ? found : best
    }, -1)
    return at < 0 ? PRIORITY.length : at
  }
  const byRank = rank(a) - rank(b)
  if (byRank !== 0) return byRank
  if (a.depth !== b.depth) return a.depth - b.depth
  return a.path.localeCompare(b.path)
}

/** One repository once, by machine and folder, in path order. */
export function mergeRepos(existing: readonly DiscoveredRepo[], found: readonly DiscoveredRepo[]): DiscoveredRepo[] {
  const key = (r: DiscoveredRepo) => `${r.host}\u0000${r.dir}`
  const all = new Map<string, DiscoveredRepo>()
  for (const r of [...existing, ...found]) all.set(key(r), r)
  return [...all.values()].sort((a, b) => a.host.localeCompare(b.host) || a.dir.localeCompare(b.dir))
}

/** The identity a discovered repository and a saved project share: its machine and folder. */
export const repoKey = (r: { host: string; dir: string }): string => `${r.host}\u0000${r.dir}`

/** The folders already saved as projects, as repoKey values. */
export function savedKeys(projects: readonly { host: string; dir: string }[]): Set<string> {
  return new Set(projects.map(repoKey))
}
