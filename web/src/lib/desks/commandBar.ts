// What the desk's command bar suggests: folders of the shell's current folder to cd into, and the commands the person
// ran or saved. Pure, so it is tested without a browser. Nothing here runs anything.
import { hasHiddenChars } from '../terminal/api'

export type Suggestion = { text: string; label: string; kind: 'folder' | 'recent' | 'saved' }
export type Folder = { name: string; hidden?: boolean }

/** The path being typed as the last word of a command: the part up to its last slash, and the name begun after it.
 *  Null for the command word itself.
 *  shortcut: a line with a quote or backslash, or a word with $ or a glob, is not completed, upgrade when people ask for it. */
export function pathToken(input: string): { head: string; dir: string; prefix: string } | null {
  const m = /^(\s*\S+\s+(?:.*\s)?)(\S*)$/.exec(input)
  if (!m || /['"\\]/.test(input) || /[$*?`]/.test(m[2])) return null
  const cut = m[2].lastIndexOf('/') + 1
  return { head: m[1], dir: m[2].slice(0, cut), prefix: m[2].slice(cut) }
}

/** The folder a typed path part names, from the shell's folder: absolute, ~ for home, or relative with . and .. */
export function resolveDir(base: string, home: string, dir: string): string {
  const from = dir.startsWith('/') ? '' : dir === '~' || dir.startsWith('~/') ? home : base
  const parts = from.split('/').filter(Boolean)
  for (const part of dir.replace(/^~/, '').split('/')) {
    if (part === '..') parts.pop()
    else if (part && part !== '.') parts.push(part)
  }
  return (from.startsWith('~') ? '' : '/') + parts.join('/')
}

/** A file name as one shell word. */
export const shellWord = (name: string) => /^[\w@%+=:,.-]+$/.test(name) ? name : `'${name.replaceAll("'", "'\\''")}'`

/** Up to `max` suggestions for what is in the bar. Empty: the next actions (cd into a folder here, then recent and saved
 *  commands). Typing: folders that complete the last word, then commands that start with the whole line. A command
 *  with a hidden character (a line break, a control character) is never suggested: the bar runs what it shows. */
export function suggest(input: string, from: { folders: Folder[]; parent?: boolean; recent: string[]; saved: string[] }, max = 12): Suggestion[] {
  const out: Suggestion[] = [], seen = new Set<string>()
  const add = (text: string, label: string, kind: Suggestion['kind']) => {
    if (seen.has(text) || text === input || hasHiddenChars(text)) return
    seen.add(text); out.push({ text, label, kind })
  }
  const typed = input.trimStart(), token = pathToken(input)
  if (!typed) {
    // Half the row at most, so the commands keep a place beside a folder of many folders.
    for (const f of from.folders.filter(f => !f.hidden).slice(0, max / 2)) add(`cd ${shellWord(f.name)}/`, `cd ${f.name}/`, 'folder')
    if (from.parent) add('cd ..', 'cd ..', 'folder')
  } else if (token) {
    const prefix = token.prefix.toLowerCase()
    for (const f of from.folders) if (f.name.toLowerCase().startsWith(prefix) && (!f.hidden || prefix.startsWith('.'))) add(`${token.head}${token.dir}${shellWord(f.name)}/`, `${f.name}/`, 'folder')
  }
  for (const [list, kind] of [[from.recent, 'recent'], [from.saved, 'saved']] as const)
    for (const line of list) { const command = line.trim(); if (command && command.startsWith(typed)) add(command, command, kind) }
  return out.slice(0, max)
}
