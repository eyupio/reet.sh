// The Add project dialog and its folder browser (PR B5), on the component fixture page (tests/fixtures/add-project.html),
// which mounts the dialog alone; Home's rail opens the same dialog (home.e2e.mjs "Add opens the folder browser"). Synthetic API answers: the browse route answers from a small tree under
// /home/dev on studio; desk is offline. Nothing is started on a machine.
import { chromium, expect } from '@playwright/test'
import { HOME, openFixture, refuse } from './helpers/folders.mjs'
import { startHarness } from './helpers/harness.mjs'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const harness = await startHarness()
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const open = (options={}) => openFixture(browser, harness, options)
const dialog = page => page.getByRole('dialog', {name:/^(Add a project|Edit reet)$/})
const addButton = page => page.getByRole('button', {name:'Add a project', exact:true})
const folders = page => dialog(page).getByRole('list', {name:/^Folders in /})
const row = (page, name) => folders(page).getByRole('button', {name, exact:true})
const crumbs = page => dialog(page).getByRole('navigation', {name:'Folder path'})
const crumbText = page => crumbs(page).evaluate(el => el.innerText.replace(/\s+/g, ' ').trim())
const folderField = page => dialog(page).getByRole('textbox', {name:'Folder', exact:true})
const lastBrowse = box => box.browses.at(-1)
async function openDialog(page) {
 await addButton(page).click()
 await expect(dialog(page)).toBeVisible()
 // The browser opens at the home on a machine that can browse.
 await expect(folders(page)).toBeVisible()
}

try {
 if (run('browse, enter, Use this folder')) {
  const {context,page,errors,box,state} = await open()
  await openDialog(page)
  const d = dialog(page)
  await expect(d.getByRole('combobox', {name:'Machine'})).toHaveValue('a1')
  await expect(d.locator('.machine-row')).toContainText('studio')
  await expect(d.locator('.machine-row')).toContainText('· online')
  await expect(d.getByRole('button', {name:'Hide browser'})).toHaveAttribute('aria-expanded', 'true')
  expect(box.browses.length).toBe(1)
  const first = lastBrowse(box)
  expect(first.searchParams.has('path')).toBe(false)
  expect(first.searchParams.get('session')).toMatch(/^[A-Za-z0-9_-]{16,128}$/)
  await expect.poll(() => crumbText(page)).toBe('~')
  await expect(d.locator('.count')).toHaveText('7 folders')
  // Hidden folders are left out until asked for; repositories get the diamond, bold names and the REPO badge.
  await expect(folders(page).getByRole('listitem')).toHaveCount(7)
  await expect(row(page, '.config')).toHaveCount(0)
  await expect(row(page, 'commands.txt')).toHaveCount(0)
  const reet = row(page, 'reet, git repository')
  await expect(reet).toContainText('REPO')
  await expect(reet.locator('.name')).toHaveCSS('font-weight', '600')
  await expect(row(page, 'archive')).not.toContainText('REPO')
  await expect(row(page, 'archive').locator('.name')).toHaveCSS('font-weight', '400')
  // A name with a space and a # goes back to the machine exactly as listed.
  await row(page, 'my work #1').click()
  await expect.poll(() => lastBrowse(box).searchParams.get('path')).toBe(`${HOME}/my work #1`)
  await expect.poll(() => crumbText(page)).toBe('~ / my work #1')
  await expect(d.locator('.here')).toContainText(`${HOME}/my work #1`)
  // Names that are not ASCII go back as they were listed too.
  await row(page, 'café').click()
  await row(page, '日本語').click()
  await expect.poll(() => lastBrowse(box).searchParams.get('path')).toBe(`${HOME}/my work #1/café/日本語`)
  await expect.poll(() => crumbText(page)).toBe('~ / my work #1 / café / 日本語')
  await expect(d.locator('.here')).toHaveText(`${HOME}/my work #1/café/日本語`)
  // Back to the home by its crumb; the crumbs never go above it.
  await crumbs(page).getByRole('button', {name:'~'}).click()
  await expect.poll(() => crumbText(page)).toBe('~')
  await row(page, 'reet, git repository').click()
  await expect.poll(() => crumbText(page)).toBe('~ / reet')
  await expect(d.locator('.here')).toHaveText(`${HOME}/reet is a git repository`)
  expect(new Set(box.browses.map(u => u.searchParams.get('session'))).size).toBe(1)
  await d.getByRole('button', {name:'Use this folder'}).click()
  await expect(folderField(page)).toHaveValue(`${HOME}/reet`)
  await expect(folders(page)).toBeHidden()
  await expect(d.getByRole('button', {name:'Browse'})).toHaveAttribute('aria-expanded', 'false')
  await expect(d).not.toContainText(/healthy|verified/i)
  await d.getByRole('button', {name:'Add project', exact:true}).click()
  await expect(d).toBeHidden()
  expect(box.writes).toHaveLength(1)
  expect(box.writes[0].method).toBe('POST')
  expect(box.writes[0].body).toEqual({host:'a1', dir:`${HOME}/reet`, isRepo:true, worktreeDefault:'inherit'})
  await expect(page.getByRole('status')).toHaveText('Sorted. reet added.')
  await expect(addButton(page)).toBeFocused()
  expect(errors).toEqual([])
  expect(state.unexpected).toEqual([])
  await context.close()
 }

 if (run('paging with Show more')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  await row(page, 'big').click()
  await expect(dialog(page).locator('.count')).toHaveText('200+ folders')
  await expect(folders(page).getByRole('listitem')).toHaveCount(200)
  // A continuation belongs to the query that listed it: while a new filter waits to be sent there is no Show more, so no
  // page is asked for with one query's continuation and another's filter.
  const filterBox = dialog(page).getByRole('textbox', {name:'Filter folders'})
  await filterBox.fill('f2')
  expect(await dialog(page).getByRole('button', {name:'Show more'}).count()).toBe(0)
  await expect(folders(page).getByRole('listitem')).toHaveCount(50)
  await filterBox.fill('')
  await expect(folders(page).getByRole('listitem')).toHaveCount(200)
  await dialog(page).getByRole('button', {name:'Show more'}).click()
  await expect(folders(page).getByRole('listitem')).toHaveCount(250)
  expect(lastBrowse(box).searchParams.get('after')).toBe('f199')
  expect(lastBrowse(box).searchParams.get('path')).toBe(`${HOME}/big`)
  await expect(dialog(page).locator('.count')).toHaveText('250 folders')
  await expect(dialog(page).getByRole('button', {name:'Show more'})).toHaveCount(0)
  // Focus moves to the first folder of the new page, not back to the top.
  await expect(row(page, 'f200')).toBeFocused()
  expect(box.browses.filter(u => u.searchParams.has('after')).map(u => u.searchParams.get('filter'))).toEqual([null])
  expect(box.browses.filter(u => u.searchParams.has('after') && u.searchParams.get('filter'))).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('hidden toggle and filter')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  await dialog(page).getByRole('checkbox', {name:'Show hidden folders'}).check()
  await expect(row(page, '.config')).toBeVisible()
  expect(lastBrowse(box).searchParams.get('hidden')).toBe('true')
  await expect(dialog(page).locator('.count')).toHaveText('8 folders')
  const before = box.browses.length
  // The filter is the machine's: one request once typing pauses, not one per key.
  await dialog(page).getByRole('textbox', {name:'Filter folders'}).pressSequentially('re', {delay:30})
  await expect(folders(page).getByRole('listitem')).toHaveCount(1)
  expect(box.browses.length).toBe(before + 1)
  expect(lastBrowse(box).searchParams.get('filter')).toBe('re')
  expect(lastBrowse(box).searchParams.get('hidden')).toBe('true')
  await expect(dialog(page).locator('.count')).toHaveText('1 folder')
  await dialog(page).getByRole('textbox', {name:'Filter folders'}).fill('zzz')
  await expect(dialog(page).getByText('No folders match.')).toBeVisible()
  await dialog(page).getByRole('textbox', {name:'Filter folders'}).fill('')
  await expect(folders(page).getByRole('listitem')).toHaveCount(8)
  await dialog(page).getByRole('checkbox', {name:'Show hidden folders'}).uncheck()
  await expect(row(page, '.config')).toHaveCount(0)
  expect(lastBrowse(box).searchParams.has('hidden')).toBe(false)
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('not a repository message')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  await row(page, 'archive').click()
  await expect(d.locator('.repo-note')).toHaveText("This folder isn't a git repository, so sessions there can't use worktrees.")
  // Allowed all the same.
  await d.getByRole('button', {name:'Use this folder'}).click()
  await expect(folderField(page)).toHaveValue(`${HOME}/archive`)
  await expect(d.locator('.field-note')).toHaveText("This folder isn't a git repository, so sessions there can't use worktrees.")
  // A typed folder is checked with one browse when the field is left, never while typing.
  const before = box.browses.length
  await folderField(page).fill('/etc/x')
  expect(box.browses.length).toBe(before)
  await folderField(page).blur()
  await expect(d.locator('.field-note')).toHaveText('That folder is outside the home folder reet was set up with.')
  expect(box.browses.length).toBe(before + 1)
  expect(lastBrowse(box).searchParams.get('path')).toBe('/etc/x')
  await folderField(page).fill(`${HOME}/nope`)
  await folderField(page).blur()
  await expect(d.locator('.field-note')).toHaveText("That folder isn't on studio.")
  await folderField(page).fill(`${HOME}/blog`)
  await folderField(page).blur()
  await expect(d.locator('.field-note')).toHaveText('A git repository.')
  // Leaving it again unchanged asks nothing more.
  const after = box.browses.length
  await folderField(page).focus(); await folderField(page).blur()
  expect(box.browses.length).toBe(after)
  await folderField(page).fill('work/reet')
  await folderField(page).blur()
  await expect(d.locator('.field-note')).toHaveText('Use a full path that starts with /.')
  await expect(d.getByRole('button', {name:'Add project', exact:true})).toBeDisabled()
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('inside a repository button')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  await row(page, 'reet, git repository').click()
  await row(page, 'web').click()
  await expect.poll(() => crumbText(page)).toBe('~ / reet / web')
  await expect(d.locator('.repo-note')).toHaveText('This folder is inside the repository at ~/reet. Use the top folder for worktrees.')
  await d.getByRole('button', {name:'Use the top folder'}).click()
  await expect(folderField(page)).toHaveValue(`${HOME}/reet`)
  await expect(folders(page)).toBeHidden()
  await d.getByRole('button', {name:'Add project', exact:true}).click()
  await expect(d).toBeHidden()
  expect(box.writes[0].body).toMatchObject({host:'a1', dir:`${HOME}/reet`, isRepo:true})
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('offline machine adds by typed path')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  const before = box.browses.length
  await d.getByRole('combobox', {name:'Machine'}).selectOption('a2')
  await expect(d.locator('.machine-row')).toContainText('desk')
  await expect(d.locator('.machine-row')).toContainText('· offline')
  await expect(d.locator('.machine-note')).toHaveText('The machine is reconnecting. Existing shells will be kept.')
  await expect(d.getByRole('button', {name:'Browse'})).toBeDisabled()
  await expect(folders(page)).toBeHidden()
  await folderField(page).fill(`${HOME}/notes`)
  await folderField(page).blur()
  await d.getByRole('button', {name:'Add project', exact:true}).click()
  await expect(d).toBeHidden()
  // An offline machine is not asked, by the browser or by leaving the field.
  expect(box.browses.length).toBe(before)
  expect(box.writes[0].body).toEqual({host:'a2', dir:`${HOME}/notes`, worktreeDefault:'inherit'})
  await expect(page.getByRole('status')).toHaveText('Sorted. notes added.')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('offline mid-browse keeps the list')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  await folderField(page).fill(`${HOME}/typed`)
  await d.getByRole('textbox', {name:'Filter folders'}).focus()
  await expect(d.locator('.field-note')).toHaveText("That folder isn't on studio.")
  box.fail = refuse(503, 'machine_unavailable', 'That machine is reconnecting. Try again in a moment.')
  await row(page, 'reet, git repository').click()
  await expect(d.locator('.browse-error')).toContainText('That machine is reconnecting. Try again in a moment.')
  // The breadcrumb, list and typed path stay as they were.
  await expect.poll(() => crumbText(page)).toBe('~')
  await expect(folders(page).getByRole('listitem')).toHaveCount(7)
  await expect(folderField(page)).toHaveValue(`${HOME}/typed`)
  await expect(d.getByRole('button', {name:'Add project', exact:true})).toBeEnabled()
  await d.getByRole('button', {name:'Try again'}).click()
  await expect.poll(() => crumbText(page)).toBe('~ / reet')
  expect(lastBrowse(box).searchParams.get('path')).toBe(`${HOME}/reet`)
  await expect(d.locator('.browse-error')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('rate limited sentence')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  box.fail = refuse(429, 'browse_rate_limited', 'Slow down a moment, then try again.')
  await row(page, 'archive').click()
  await expect(d.locator('.browse-error')).toContainText('Slow down a moment, then try again.')
  await expect(folders(page).getByRole('listitem')).toHaveCount(7)
  await expect.poll(() => crumbText(page)).toBe('~')
  await d.getByRole('button', {name:'Try again'}).click()
  await expect.poll(() => crumbText(page)).toBe('~ / archive')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('stale edit keeps input')) {
  const {context,page,errors,box} = await open({query:'?edit=fp1'})
  await page.getByRole('button', {name:'Edit', exact:true}).click()
  const d = dialog(page)
  await expect(d).toBeVisible()
  await expect(d.getByRole('heading', {name:'Edit reet'})).toBeVisible()
  await expect(folderField(page)).toHaveValue(`${HOME}/reet`)
  // Editing opens with the browser closed: the folder is already chosen.
  await expect(folders(page)).toBeHidden()
  const name = d.getByRole('textbox', {name:/^Name/})
  await name.fill('reet main')
  await d.locator('summary', {hasText:'Starter prompt'}).click()
  const prompt = d.getByRole('textbox', {name:'Starter prompt'})
  await expect(prompt).toHaveValue('Read the README first')
  await prompt.fill('Read the README and the plan first')
  await d.getByRole('button', {name:'Save changes'}).click()
  await expect(d.locator('.status')).toHaveText('Someone changed this project. Reload it, then try again.')
  await expect(d).toBeVisible()
  await expect(name).toHaveValue('reet main')
  await expect(prompt).toHaveValue('Read the README and the plan first')
  expect(box.writes).toHaveLength(1)
  expect(box.writes[0].method).toBe('PATCH')
  expect(box.writes[0].headers['if-match']).toBe('"3"')
  expect(box.writes[0].body).toEqual({name:'reet main', starterPrompt:'Read the README and the plan first'})
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('phone full-screen sheet with pinned buttons')) {
  const {context,page,errors} = await open({width:390, height:844, touch:true})
  await openDialog(page)
  const d = dialog(page)
  const box = await d.boundingBox()
  expect([box.x, box.y, box.width, box.height].map(Math.round)).toEqual([0, 0, 390, 844])
  const foot = d.locator('[data-shell-foot]')
  await expect(foot.locator('.chosen')).toHaveText('No folder chosen yet')
  await row(page, 'reet, git repository').click()
  await d.getByRole('button', {name:'Use this folder'}).click()
  await expect(foot.locator('.chosen')).toHaveText(`${HOME}/reet · git repository`)
  await d.getByRole('button', {name:'Browse'}).click()
  await expect(folders(page)).toBeVisible()
  const pinned = async () => { const f = await foot.boundingBox(); return Math.round(f.y + f.height) }
  expect(await pinned()).toBe(844)
  // Scrolling the folders leaves the chosen folder and the buttons where they are.
  await d.locator('[data-shell-body]').evaluate(el => { el.scrollTop = el.scrollHeight })
  expect(await pinned()).toBe(844)
  const cancel = await foot.getByRole('button', {name:'Cancel'}).boundingBox()
  const add = await foot.getByRole('button', {name:'Add this project'}).boundingBox()
  expect([Math.round(cancel.width), Math.round(cancel.height), Math.round(add.height)]).toEqual([110, 48, 48])
  expect(add.y + add.height).toBeLessThanOrEqual(844)
  for (const size of await d.locator('input:not([type=checkbox]),select,textarea').evaluateAll(els => els.filter(e => e.getBoundingClientRect().width > 0).map(e => getComputedStyle(e).fontSize))) expect(size).toBe('16px')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // A touch tablet (834 px): the dialog's fields, buttons and the folds' headings are 44 px targets, as on a phone (48 px
 // there); with a mouse the drawing's 42 px fields stay.
 if (run('touch tablet targets')) {
  for (const touch of [true, false]) {
   const {context,page,errors} = await open({width:834, height:1112, touch})
   await openDialog(page)
   const d = dialog(page)
   await row(page, 'reet, git repository').click()
   await d.getByRole('button', {name:'Use this folder'}).click()
   const small = await d.evaluate(root => [...root.querySelectorAll('button, select, input:not([type=checkbox]):not([type=radio]), summary')]
    .filter(el => { const r = el.getBoundingClientRect(); return r.width > 0 && el.checkVisibility({visibilityProperty:true}) })
    .map(el => { const target = el.matches('select') && el.closest('.machine-row') ? el.closest('.machine-row') : el, r = target.getBoundingClientRect(); return [(el.getAttribute('aria-label') || el.textContent || el.name || el.tagName).trim().slice(0, 30), Math.round(r.height)] })
    .filter(([, h]) => h < 44))
   if (touch) expect(small, 'touch targets under 44 px').toEqual([])
   else expect(small.length, 'a mouse keeps the 42 px fields').toBeGreaterThan(0)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 if (run('focus returns to Add on close')) {
  const {context,page,errors} = await open()
  for (const how of ['Escape', 'Cancel', 'Close']) {
   await openDialog(page)
   if (how === 'Escape') await page.keyboard.press('Escape')
   else await dialog(page).getByRole('button', {name:how, exact:true}).click()
   await expect(dialog(page)).toBeHidden()
   await expect(addButton(page)).toBeFocused()
  }
  expect(errors).toEqual([])
  await context.close()
 }
 // Keyboard only: a folder that fails to list leaves focus on Try again, and an empty folder on Use this folder, never on
 // the page behind the dialog.
 if (run('keyboard focus after a failed browse and an empty folder')) {
  const gates = {}
  const {context,page,errors,box} = await open({gates})
  await openDialog(page)
  const d = dialog(page)
  box.fail = refuse(429, 'browse_rate_limited', 'Slow down a moment, then try again.')
  // While the request is held, the row pressed keeps focus (marked busy, never disabled out from under it).
  let release
  gates['/api/v2/machines/a1/browse'] = new Promise(r => { release = r })
  await row(page, 'reet, git repository').focus()
  await page.keyboard.press('Enter')
  await expect(row(page, 'reet, git repository')).toHaveAttribute('aria-disabled', 'true')
  await expect(row(page, 'reet, git repository')).toBeFocused()
  delete gates['/api/v2/machines/a1/browse']
  release()
  await expect(d.locator('.browse-error p')).toHaveText('Slow down a moment, then try again.')
  await expect(d.getByRole('button', {name:'Try again'})).toBeFocused()
  await page.keyboard.press('Enter')
  await expect.poll(() => crumbText(page)).toBe('~ / reet')
  await expect(row(page, 'docs')).toBeFocused()
  await crumbs(page).getByRole('button', {name:'~'}).focus()
  await page.keyboard.press('Enter')
  await expect(row(page, 'archive')).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(d.locator('.empty')).toHaveText('No folders here.')
  await expect(d.getByRole('button', {name:'Use this folder'})).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // Each refusal from the machine or the controller is shown as it was sent, with Try again, and the list stays.
 if (run('machine refusals keep their sentence')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  for (const [status, code, sentence] of [
   [422, 'denied', "reet can't read that folder."],
   [422, 'unreadable', "reet can't read that folder."],
   [409, 'browse_busy', 'That machine is already listing a folder. Try again in a moment.'],
   [409, 'browse_off', 'That machine has folder browsing switched off.'],
   [409, 'browse_unsupported', 'Update reet on that machine to browse its folders.'],
  ]) {
   box.fail = refuse(status, code, sentence)
   await row(page, 'scratch').click()
   await expect(d.locator('.browse-error p')).toHaveText(sentence)
   await expect(d.locator('.browse-error')).toContainText('Try again')
   await expect(folders(page).getByRole('listitem')).toHaveCount(7)
   await expect.poll(() => crumbText(page)).toBe('~')
  }
  expect(errors).toEqual([])
  await context.close()
 }

 // The base branch suggestions are the chosen folder's own: read when More defaults is open on a repository's top folder,
 // read again when the folder changes, none for a folder that is not a top, and a refusal said as sent.
 if (run('branch suggestions follow the folder')) {
  const {context,page,errors,box} = await open()
  await openDialog(page)
  const d = dialog(page)
  const options = () => d.locator(`datalist option`).evaluateAll(els => els.map(e => e.value))
  const note = d.locator('.branch-note')
  const browseTo = async name => {
   await d.getByRole('button', {name:'Browse'}).click()
   await crumbs(page).getByRole('button', {name:'~'}).click()
   await row(page, name).click()
   await expect.poll(() => crumbText(page)).toBe(`~ / ${name.replace(', git repository', '')}`)
   await d.getByRole('button', {name:'Use this folder'}).click()
  }
  await row(page, 'reet, git repository').click()
  await d.getByRole('button', {name:'Use this folder'}).click()
  // Read when More defaults opens on a repository's top folder.
  await d.locator('summary', {hasText:'More defaults'}).click()
  await expect.poll(options).toEqual(['main', 'dev'])
  // A folder that is not a repository's top is not asked about, and the last folder's branches go.
  await browseTo('archive')
  await expect.poll(options).toEqual([])
  // With More defaults open, another repository chosen in the browser brings its own.
  await browseTo('blog, git repository')
  await expect.poll(options).toEqual(['trunk'])
  // Typing another folder drops them too.
  const reads = box.branchReads.length
  await folderField(page).fill(`${HOME}/archive`)
  await folderField(page).blur()
  await expect(d.locator('.field-note')).toHaveText("This folder isn't a git repository, so sessions there can't use worktrees.")
  await expect.poll(options).toEqual([])
  expect(box.branchReads.length).toBe(reads)
  // A repository without branches: none offered, nothing invented.
  await folderField(page).fill(`${HOME}/site`)
  await folderField(page).blur()
  await expect.poll(() => box.branchReads.at(-1)).toBe(`${HOME}/site`)
  await expect.poll(options).toEqual([])
  await expect(note).toHaveCount(0)
  // A refusal is said as the machine sent it.
  box.branchFail = refuse(422, 'git_failed', "Git couldn't list the branches there.")
  await folderField(page).fill(`${HOME}/reet`)
  await folderField(page).blur()
  await expect(note).toHaveText("Git couldn't list the branches there.")
  await expect.poll(options).toEqual([])
  expect(box.branchReads).toEqual([`${HOME}/reet`, `${HOME}/blog`, `${HOME}/site`, `${HOME}/reet`])
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Add project: browsing, paging, hidden and filter, repository notes, offline and refusals, edit, phone sheet, focus, keyboard focus and branch suggestions checked.')
} finally { await browser.close(); await harness.close() }
