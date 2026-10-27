import type { BrowseResult } from './terminal/api'
import { validateDir } from './agentLaunch'

// What the Add project dialog's folder browser shows about a machine's folders: the breadcrumb, the repository sentences
// and the folder count. Pure: no DOM, no fetch. Paths are the machine's own, passed back exactly as it listed them (never
// URL-encoded here; browseURL encodes the query). Nothing here says a folder or machine is in good order: the machine
// decides whether it can open each folder, at browse and again at launch.

export type Crumb = { label: string; path: string }

// A path without its trailing slashes ("/" stays "/").
const trim = (p: string): string => (p.length > 1 ? p.replace(/\/+$/, '') || '/' : p)

/** The part of `path` below `home`, or null when `path` is not the home or inside it (compared a whole name at a time). */
function below(path: string, home: string): string | null {
  const p = trim(String(path ?? '')), h = trim(String(home ?? ''))
  if (!h.startsWith('/') || !p.startsWith('/')) return null
  if (p === h) return ''
  const prefix = h === '/' ? '/' : `${h}/`
  return p.startsWith(prefix) ? p.slice(prefix.length) : null
}

/** A folder inside `dir`, by the name the machine listed. */
export function childPath(dir: string, name: string): string {
  const d = trim(dir)
  return d === '/' ? `/${name}` : `${d}/${name}`
}

/** The crumbs from the home (shown as `~`) down to `path`. The first crumb is always the home and none is above it: a path
 *  outside the home (which the machine never sends) shows the home alone. `..` is not resolved, and a crumb never climbs. */
export function breadcrumbs(path: string, home: string): Crumb[] {
  const h = trim(String(home ?? ''))
  const out: Crumb[] = [{ label: '~', path: h }]
  const rest = below(path, h)
  if (!rest) return out
  let at = h
  for (const name of rest.split('/')) {
    if (name === '' || name === '.' || name === '..') break
    at = childPath(at, name)
    out.push({ label: name, path: at })
  }
  return out
}

/** `path` as the person knows it from the home: `~`, `~/work/reet`, or the full path when it is outside the home. */
export function homeRelative(path: string, home: string): string {
  const rest = below(path, home)
  return rest === null ? path : rest === '' ? '~' : `~/${rest}`
}

/** The spec's two sentences about the folder shown: one when it is not a repository, one when it is inside one but not its
 *  top. Null for a repository's top folder. */
export function repoMessage(result: Pick<BrowseResult, 'repo' | 'repoTop' | 'home'>): string | null {
  if (result.repo === 'root') return null
  if (result.repo === 'inside' && result.repoTop) return `This folder is inside the repository at ${homeRelative(result.repoTop, result.home)}. Use the top folder for worktrees.`
  return "This folder isn't a git repository, so sessions there can't use worktrees."
}

/** The repository's top folder to offer instead, when the folder shown is inside one; never one outside the home. */
export function topFolder(result: Pick<BrowseResult, 'repo' | 'repoTop' | 'home'>): string | null {
  if (result.repo !== 'inside' || !result.repoTop) return null
  return below(result.repoTop, result.home) === null ? null : trim(result.repoTop)
}

/** "14 folders", "1 folder", "200+ folders" while more pages wait. */
export function countLabel(n: number, more: boolean): string {
  return more ? `${n}+ folders` : `${n} ${n === 1 ? 'folder' : 'folders'}`
}

/** What is wrong with a typed folder path, or null. Only its shape: whether it exists is the machine's to say. */
export function validateFolderInput(path: string): string | null {
  if (String(path ?? '') === '') return 'Choose a folder: type its full path or browse to it.'
  return validateDir(path)
}

/** The status line after a project is added. */
export const addedLine = (name: string): string => `Sorted. ${name} added.`
