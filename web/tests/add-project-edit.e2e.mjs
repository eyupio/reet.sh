// The Add project dialog's later fixes (PR B6), on the same component fixture page as add-project.e2e.mjs: a stale edit
// loads the latest version and keeps the person's own changes to use again; branch suggestions come back when the same
// folder is typed again and are read again after a failed read; typing a filter while a folder opens keeps that folder.
// Synthetic API answers; nothing is started on a machine.
import { chromium, expect } from '@playwright/test'
import { HOME, openFixture, refuse, saved } from './helpers/folders.mjs'
import { startHarness } from './helpers/harness.mjs'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const harness = await startHarness()
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const open = (options={}) => openFixture(browser, harness, options)
const dialog = page => page.getByRole('dialog', {name:/^(Add a project|Edit reet)$/})
const folders = page => dialog(page).getByRole('list', {name:/^Folders in /})
const row = (page, name) => folders(page).getByRole('button', {name, exact:true})
const crumbText = page => dialog(page).getByRole('navigation', {name:'Folder path'}).evaluate(el => el.innerText.replace(/\s+/g, ' ').trim())
const folderField = page => dialog(page).getByRole('textbox', {name:'Folder', exact:true})
const suggestions = page => dialog(page).locator('datalist option').evaluateAll(els => els.map(e => e.value))

try {
 // Someone else saved the project first. The edit says so, loads the latest version on request, and lists what this
 // person changed, so they can use it again on top of the latest one; the next save names the latest version.
 if (run('stale edit loads the latest and keeps my changes')) {
  let version = 3
  const patches = []
  const routes = {'/api/v2/folder-projects/fp1': request => {
   if (request.method() === 'GET') return {status:200, json:version === 3 ? saved({starterPrompt:'Read the README first'}) : saved({version:4, defaultModel:'opus', worktreeDefault:'no', starterPrompt:'Read the README first'})}
   const body = request.postDataJSON(), match = request.headers()['if-match']
   patches.push({body, match})
   if (match !== `"${version}"`) return refuse(409, 'project_changed', 'Someone changed this project. Reload it, then try again.')
   return saved({version:5, name:body.name ?? 'reet', defaultModel:body.defaultModel ?? 'opus', worktreeDefault:'no'})
  }}
  const {context,page,errors} = await open({query:'?edit=fp1', routes})
  await page.getByRole('button', {name:'Edit', exact:true}).click()
  const d = dialog(page)
  await expect(d).toBeVisible()
  await d.getByRole('textbox', {name:/^Name/}).fill('reet two')
  await d.locator('summary', {hasText:'More defaults'}).click()
  await d.getByRole('textbox', {name:'Default model'}).fill('sonnet')
  version = 4
  await d.getByRole('button', {name:'Save changes'}).click()
  await expect(d.getByRole('status')).toHaveText('Someone changed this project. Reload it, then try again.')
  // The stale version is never sent again: Save waits for the latest version.
  await expect(d.getByRole('button', {name:'Save changes'})).toBeDisabled()
  const reload = d.getByRole('button', {name:'Load the latest version'})
  await expect(reload).toBeFocused()
  await reload.click()
  await expect(d.getByRole('textbox', {name:/^Name/})).toHaveValue('reet')
  await expect(d.getByRole('textbox', {name:'Default model'})).toHaveValue('opus')
  await expect(d.getByRole('combobox', {name:'Worktree for each session'})).toHaveValue('no')
  const mine = d.getByRole('region', {name:'Your changes'})
  await expect(mine).toContainText('Name: reet two')
  await expect(mine).toContainText('Default model: sonnet')
  await expect(mine).not.toContainText('Worktree')
  await mine.getByRole('button', {name:'Use my changes again'}).click()
  await expect(d.getByRole('textbox', {name:/^Name/})).toHaveValue('reet two')
  await expect(d.getByRole('textbox', {name:'Default model'})).toHaveValue('sonnet')
  await expect(d.getByRole('combobox', {name:'Worktree for each session'})).toHaveValue('no', {timeout:1000})
  await d.getByRole('button', {name:'Save changes'}).click()
  await expect(page.getByRole('status').filter({hasText:'Sorted.'})).toHaveText('Sorted. reet two saved.')
  expect(patches.map(p => p.match)).toEqual(['"3"', '"4"'])
  expect(patches[1].body).toEqual({name:'reet two', defaultModel:'sonnet'})
  expect(errors).toEqual([])
  await context.close()
 }

 // Use fills the folder and reads its branches; typing it away and back to the same folder shows the same note and
 // suggestions again, without asking the machine a second time.
 if (run('retyping the same folder keeps its branch suggestions')) {
  const {context,page,errors,box} = await open()
  await page.getByRole('button', {name:'Add a project', exact:true}).click()
  const d = dialog(page)
  await d.locator('summary', {hasText:'More defaults'}).click()
  await row(page, 'reet, git repository').click()
  await expect.poll(() => crumbText(page)).toBe('~ / reet')
  await d.getByRole('button', {name:'Use this folder'}).click()
  await expect.poll(() => suggestions(page)).toEqual(['main', 'dev'])
  const browses = box.browses.length
  await folderField(page).fill(`${HOME}/reetx`)
  await expect.poll(() => suggestions(page)).toEqual([])
  await folderField(page).fill(`${HOME}/reet`)
  await folderField(page).press('Tab')
  await expect(d.locator('.field-note')).toHaveText('A git repository.')
  await expect.poll(() => suggestions(page)).toEqual(['main', 'dev'])
  expect(box.browses.length).toBe(browses)
  expect(errors).toEqual([])
  await context.close()
 }

 // A branch read that failed is tried again the next time More defaults opens.
 if (run('a failed branch read is tried again when More defaults reopens')) {
  const {context,page,errors,box} = await open()
  await page.getByRole('button', {name:'Add a project', exact:true}).click()
  const d = dialog(page)
  await row(page, 'reet, git repository').click()
  await expect.poll(() => crumbText(page)).toBe('~ / reet')
  await d.getByRole('button', {name:'Use this folder'}).click()
  box.branchFail = refuse(422, 'git_failed', "Git couldn't list the branches there.")
  const more = d.locator('summary', {hasText:'More defaults'})
  await more.click()
  await expect(d.locator('.branch-note')).toHaveText("Git couldn't list the branches there.")
  await more.click()
  await more.click()
  await expect.poll(() => suggestions(page)).toEqual(['main', 'dev'])
  await expect(d.locator('.branch-note')).toHaveCount(0)
  expect(box.branchReads).toEqual([`${HOME}/reet`, `${HOME}/reet`])
  expect(errors).toEqual([])
  await context.close()
 }

 // Typing in the filter while a folder is opening filters that folder; it never goes back to the one being left.
 if (run('typing a filter while a folder opens keeps that folder')) {
  const gates = {}
  const {context,page,errors,box} = await open({gates})
  await page.getByRole('button', {name:'Add a project', exact:true}).click()
  const d = dialog(page)
  const filter = d.getByRole('textbox', {name:'Filter folders'})
  // Each way round: the folder answers before the filter's pause (300 ms) ends, and after it. A filter request queues
  // behind the folder's (one browse at a time), so the second case waits out the pause itself.
  for (const late of [false, true]) {
   await expect(row(page, 'reet, git repository')).toBeVisible()
   let release
   gates['/api/v2/machines/a1/browse'] = new Promise(r => { release = r })
   await row(page, 'reet, git repository').click()
   await filter.fill('do')
   if (late) await page.waitForTimeout(600)
   delete gates['/api/v2/machines/a1/browse']
   release()
   await expect.poll(() => crumbText(page), `late ${late}`).toBe('~ / reet')
   await expect.poll(() => box.browses.at(-1).searchParams.get('filter'), `late ${late}`).toBe('do')
   expect(box.browses.at(-1).searchParams.get('path')).toBe(`${HOME}/reet`)
   await expect(folders(page).getByRole('listitem')).toHaveCount(1)
   await expect(row(page, 'docs')).toBeVisible()
   // The person is still typing: the folder's answer does not take the focus.
   await expect(filter, `late ${late}`).toBeFocused()
   await filter.fill('')
   await dialog(page).getByRole('navigation', {name:'Folder path'}).getByRole('button', {name:'~'}).click()
   await expect.poll(() => crumbText(page)).toBe('~')
  }
  expect(errors).toEqual([])
  await context.close()
 }

 console.log('Add project, later fixes: a stale edit loads the latest version and keeps my changes to use again, retyping a folder keeps its branch suggestions, a failed branch read is tried again, and a filter typed while a folder opens keeps that folder.')
} finally {
 await browser.close()
 await harness.close()
}
