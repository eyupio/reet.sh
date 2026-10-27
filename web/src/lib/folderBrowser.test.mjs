import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// folderBrowser.ts imports agentLaunch.ts (for the folder path rule); both are compiled here and the import is pointed at the
// compiled copy, so the test runs the real modules without a bundler.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const launch = asURL(compile('./agentLaunch.ts'))
const f = await import(asURL(compile('./folderBrowser.ts').replace("from './agentLaunch'", `from '${launch}'`)))
const { breadcrumbs, repoMessage, topFolder, countLabel, validateFolderInput, homeRelative, addedLine, childPath } = f

const HOME = '/home/me'
const result = (over = {}) => ({ path: HOME, home: HOME, parent: '', repo: 'none', repoTop: '', entries: [], more: false, partial: false, scanned: 0, omitted: 0, ...over })
const FORBIDDEN = /\b(healthy|verified)\b/i

test('breadcrumb of a path with spaces and #', () => {
  const path = '/home/me/my work/#1 "draft"/-x'
  assert.deepEqual(breadcrumbs(path, HOME), [
    { label: '~', path: HOME },
    { label: 'my work', path: '/home/me/my work' },
    { label: '#1 "draft"', path: '/home/me/my work/#1 "draft"' },
    { label: '-x', path: '/home/me/my work/#1 "draft"/-x' },
  ])
  // Each crumb's path is a real path, never URL-encoded: it goes back to the machine as it is.
  for (const c of breadcrumbs(path, HOME)) assert.ok(!c.path.includes('%'), c.path)
  // A trailing slash on either side changes nothing.
  assert.deepEqual(breadcrumbs('/home/me/my work/', '/home/me/'), breadcrumbs('/home/me/my work', HOME))
  // Joining an entry's name onto the folder keeps it as it is too.
  assert.equal(childPath('/home/me/my work', '#1 "draft"'), '/home/me/my work/#1 "draft"')
  assert.equal(childPath('/', 'srv'), '/srv')
})

test('never above home', () => {
  assert.deepEqual(breadcrumbs(HOME, HOME), [{ label: '~', path: HOME }])
  // A path that is not inside the home (a sibling that shares its prefix, the parent, the root) shows the home alone: no
  // crumb ever leads above it.
  for (const outside of ['/home', '/', '/home/me2/x', '/etc/passwd', ''])
    assert.deepEqual(breadcrumbs(outside, HOME), [{ label: '~', path: HOME }], outside)
  // `..` never climbs out either: the crumbs follow the names as given and stay under the home.
  for (const c of breadcrumbs('/home/me/a/../..', HOME)) assert.ok(c.path === HOME || c.path.startsWith(HOME + '/'), c.path)
  // A home at the root still works.
  assert.deepEqual(breadcrumbs('/srv/app', '/'), [{ label: '~', path: '/' }, { label: 'srv', path: '/srv' }, { label: 'app', path: '/srv/app' }])
  assert.equal(homeRelative('/home/me/work/reet', HOME), '~/work/reet')
  assert.equal(homeRelative(HOME, HOME), '~')
  assert.equal(homeRelative('/home/me2', HOME), '/home/me2')
})

test('inside a repository offers the top folder', () => {
  const inside = result({ path: '/home/me/work/reet/web', repo: 'inside', repoTop: '/home/me/work/reet' })
  assert.equal(repoMessage(inside), 'This folder is inside the repository at ~/work/reet. Use the top folder for worktrees.')
  assert.equal(topFolder(inside), '/home/me/work/reet')
  assert.equal(repoMessage(result({ repo: 'none' })), "This folder isn't a git repository, so sessions there can't use worktrees.")
  assert.equal(topFolder(result({ repo: 'none' })), null)
  assert.equal(repoMessage(result({ path: '/home/me/work/reet', repo: 'root' })), null)
  assert.equal(topFolder(result({ repo: 'root' })), null)
  // A top folder outside the home (which the machine never sends) is not offered.
  assert.equal(topFolder(result({ path: '/home/me/x', repo: 'inside', repoTop: '/home' })), null)
  assert.equal(topFolder(result({ path: '/home/me/x', repo: 'inside', repoTop: '' })), null)
  for (const r of ['none', 'inside', 'root']) assert.doesNotMatch(repoMessage(result({ repo: r, repoTop: '/home/me/a' })) ?? '', FORBIDDEN)
})

test('count labels', () => {
  assert.equal(countLabel(14, false), '14 folders')
  assert.equal(countLabel(200, true), '200+ folders')
  assert.equal(countLabel(1, false), '1 folder')
  assert.equal(countLabel(0, false), '0 folders')
  assert.equal(countLabel(400, true), '400+ folders')
})

test('folder input and the success line', () => {
  assert.equal(validateFolderInput('/home/me/work/reet'), null)
  assert.equal(validateFolderInput('/home/me/a folder with spaces/#1'), null)
  assert.match(validateFolderInput('') ?? '', /folder/i)
  assert.equal(validateFolderInput('work/reet'), 'Use a full path that starts with /.')
  assert.equal(validateFolderInput('~/work'), 'Use a full path that starts with /.')
  assert.match(validateFolderInput('/a\nb') ?? '', /control or invisible/)
  assert.match(validateFolderInput('/' + 'a'.repeat(4096)) ?? '', /too long/)
  assert.equal(addedLine('reet'), 'Sorted. reet added.')
})
