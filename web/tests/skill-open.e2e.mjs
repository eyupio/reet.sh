// An opened skill: pressing its name opens where it stands on each machine, its tags and lists, where it came from,
// its notes and what to do with it. Copy prompt never carries the original link; Copy install command is offered only
// once a skill is published; Update from original shows what changed and posts the digest it showed; Use this version
// asks the machine that edited a skill for its copy; Publish and Unpublish say plainly when a skill can't be published.
// A like, a note or a tag reads nothing again, and the page does not read itself again for a change it made.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const passed = []

// When seo-cluster came into the library, and how the opened skill writes that date.
const importedAt = Date.now() - 3600_000
const importedOn = new Date(importedAt).toLocaleDateString('en-GB', {day:'numeric', month:'short'})
const machines = [{host:'Elitebook', state:'ok'}, {host:'ThinkPad', state:'failed', detail:'No write access to ~/.claude/skills'}, {host:'studio', state:'ok'}]
function world() {
 return {
  rows: [
   skillRow('reet-babysit', {originKind:'written', origin:'', yours:true, description:'Watch a pull request until it is ready to merge.'}),
   skillRow('seo-cluster', {description:'Group keywords into topic clusters mapped to pages. Source: https://github.com/demo/toolkit.', origin:'https://github.com/demo/toolkit/seo-cluster',
    tags:[{id:1, name:'seo'}], lists:[{id:7, name:'SEO kit'}], notes:'Run after seo-audit.',
    health:{state:'failed', pending:1, total:3, hosts:['ThinkPad'], machines},
    editedOn:{agentId:'agent-thinkpad', host:'ThinkPad'}, createdAt:importedAt}),
   skillRow('hosted-tips', {originKind:'written', origin:'', yours:true, published:true, publicUrl:'https://reet.example/sam/hosted-tips'}),
   skillRow('improve-animations', {originKind:'kept', origin:'Elitebook', editedOn:{agentId:'agent-elitebook', host:'Elitebook'}, editedOnProblem:'The copy on Elitebook holds a file over 8 MiB, which reet does not copy.'}),
   skillRow('my_skill', {originKind:'kept', origin:'studio'}),
  ],
  posts: [], gets: [],
  original: {changes:[{path:'SKILL.md', kind:'modified'}, {path:'references/terms.md', kind:'added'}], state:'changed', digest:'digest-2', revision:'rev-1', edited:true},
 }
}

function routes(w) {
 const page = status => skillsPageOf(w.rows, status)
 const post = (kind, req) => { const b = req.postDataJSON(); w.posts.push([kind, b]); return b }
 return {
  '/api/v2/skills': req => { w.gets.push(['page', Date.now()]); return page(new URL(req.url()).searchParams.get('status') || 'active') },
  '/api/v2/skills/status': req => {
   const b = post('status', req)
   w.rows = w.rows.map(r => b.ids.includes(r.id) ? {...r, status:b.status} : r)
   return page(b.tab || b.status)
  },
  '/api/v2/skills/item-seo-cluster/original': req => {
   if (req.method() === 'GET') { w.gets.push(['original', Date.now()]); return w.original }
   post('original', req)
   return {updated:true, digest:w.original.digest, edited:w.original.edited}
  },
  '/api/v2/skills/item-seo-cluster/use-version': req => { post('use-version', req); return {status:202, json:{requested:true, id:'r1'}} },
  '/api/v2/skills/item-seo-cluster/publish': req => {
   const b = post('publish', req)
   w.rows = w.rows.map(r => r.id === 'item-seo-cluster' ? {...r, published:b.public, publicUrl:b.public ? 'https://reet.example/sam/seo-cluster' : undefined} : r)
   return {published:b.public, publicUrl:b.public ? 'https://reet.example/sam/seo-cluster' : ''}
  },
  '/api/skill': req => {
   const b = post('skill', req)
   w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, ...('liked' in b ? {liked:b.liked} : {}), ...('notes' in b ? {notes:b.notes} : {})} : x)
   return {id:b.id}
  },
  '/api/skill/tags': req => {
   const b = post('tags', req)
   const r = w.rows.find(x => x.shelfId === b.id)
   const tags = [...r.tags, ...b.add.map((name, i) => ({id:100 + i, name}))]
   w.rows = w.rows.map(x => x === r ? {...x, tags} : x)
   return {id:b.id, tags}
  },
  '/api/v2/lists': {lists:[{id:7, name:'SEO kit'}, {id:8, name:'Writing'}], linkEnabled:false},
  '/api/v2/lists/8/skills': req => { post('list', req); return {id:8, name:'Writing', skills:[]} },
 }
}

async function open(options = {}) {
 const w = world()
 options.edit?.(w)
 const f = await openWorkspace(browser, {width:1440, height:900, ...options, routes:routes(w)})
 await f.context.grantPermissions(['clipboard-read', 'clipboard-write'], {origin:base})
 await f.page.goto(`${base}/skills`)
 await expect(f.page.locator('[data-skill]').first()).toBeVisible()
 return {...f, w}
}
const row = (page, name) => page.locator(`[data-skill="item-${name}"]`)
const nameButton = (page, name) => row(page, name).getByRole('button', {name, exact:true})
const posts = (w, kind) => w.posts.filter(p => p[0] === kind).map(p => p[1])
async function openRow(page, name) {
 await nameButton(page, name).click()
 await expect(nameButton(page, name)).toHaveAttribute('aria-expanded', 'true')
 return row(page, name).locator('.detail')
}
// Visible text under 14 px inside every element matching selector.
const under14 = (page, selector) => page.evaluate(sel => [...document.querySelectorAll(sel)].flatMap(root => [root, ...root.querySelectorAll('*')])
 .filter(el => el.checkVisibility() && [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim()) && parseFloat(getComputedStyle(el).fontSize) < 14)
 .map(el => `${el.tagName} "${el.textContent.trim().slice(0, 30)}"`), selector)

try {
 // Pressing the name opens the detail: the machines with their states, the tags and lists, where it came from, the
 // notes and what to do with it. Edit goes to Write a skill: a skill you wrote by its name, any other by its id.
 if (run('open')) {
  const {context, page, errors} = await open()
  await expect(nameButton(page, 'seo-cluster')).toHaveAttribute('aria-expanded', 'false')
  const detail = await openRow(page, 'seo-cluster')
  await expect(detail).toBeVisible()
  await expect(nameButton(page, 'seo-cluster')).toHaveAttribute('aria-controls', await detail.getAttribute('id'))
  const list = detail.getByRole('list', {name:'Machines for seo-cluster'})
  await expect(list.getByRole('listitem')).toHaveCount(3)
  await expect(list.getByRole('listitem').nth(1)).toContainText('ThinkPad')
  await expect(list.getByRole('listitem').nth(1)).toContainText('Failed')
  await expect(list.getByRole('listitem').nth(1)).toContainText('No write access to ~/.claude/skills')
  await expect(detail.getByRole('list', {name:'Tags of seo-cluster'})).toContainText('seo')
  await expect(detail.getByRole('list', {name:'Lists with seo-cluster'})).toContainText('SEO kit')
  await expect(detail.getByRole('heading', {name:'Imported from'})).toBeVisible()
  await expect(detail.getByRole('link', {name:'github.com/demo/toolkit/seo-cluster'})).toHaveAttribute('href', 'https://github.com/demo/toolkit/seo-cluster')
  await expect(detail.getByText('For reference. reet installs its own copy.')).toBeVisible()
  // The date it was imported sits beside the link, as in the mock.
  await expect(detail.locator('.origin')).toHaveText(`github.com/demo/toolkit/seo-cluster · ${importedOn}`)
  await expect(detail.getByRole('button', {name:'Update from original'})).toBeVisible()
  await expect(detail.getByText('Run after seo-audit.')).toBeVisible()
  await expect(detail.getByRole('link', {name:'Edit seo-cluster'})).toHaveAttribute('href', '/hosting?item=item-seo-cluster')
  // A skill kept from a machine names the machine, with no Update from original.
  const kept = await openRow(page, 'improve-animations')
  await expect(nameButton(page, 'seo-cluster')).toHaveAttribute('aria-expanded', 'false')
  await expect(kept.getByRole('heading', {name:'Kept from'})).toBeVisible()
  await expect(kept.getByText('Elitebook', {exact:true})).toBeVisible()
  await expect(kept.getByText('For reference. reet installs its own copy.')).toBeVisible()
  await expect(kept.getByRole('button', {name:'Update from original'})).toHaveCount(0)
  // A skill you wrote is edited by its name, and comes from nowhere else.
  const written = await openRow(page, 'reet-babysit')
  await expect(written.getByRole('link', {name:'Edit reet-babysit'})).toHaveAttribute('href', '/hosting?edit=reet-babysit')
  await expect(written.getByRole('heading', {name:/Imported from|Kept from/})).toHaveCount(0)
  // The keyboard opens and closes it too.
  await nameButton(page, 'reet-babysit').focus()
  await page.keyboard.press('Enter')
  await expect(nameButton(page, 'reet-babysit')).toHaveAttribute('aria-expanded', 'false')
  await page.keyboard.press('Space')
  await expect(nameButton(page, 'reet-babysit')).toHaveAttribute('aria-expanded', 'true')
  expect(errors).toEqual([])
  await context.close()
  passed.push('open')
 }

 // Copy prompt names the skill and its description and never its original link; a private skill has no Copy install
 // command, a published one does.
 if (run('copy')) {
  const {context, page, errors} = await open()
  const detail = await openRow(page, 'seo-cluster')
  await detail.getByRole('button', {name:'Copy prompt for seo-cluster'}).click()
  const prompt = await page.evaluate(() => navigator.clipboard.readText())
  expect(prompt).toBe('Use the seo-cluster skill: Group keywords into topic clusters mapped to pages.')
  expect(prompt).not.toContain('github.com')
  await expect(detail.getByRole('button', {name:/Copy install command/})).toHaveCount(0)
  await expect(detail.getByText("Private. Copy install command appears once it's published.")).toBeVisible()
  const published = await openRow(page, 'hosted-tips')
  await published.getByRole('button', {name:'Copy install command for hosted-tips'}).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toContain('https://reet.example/sam/hosted-tips')
  expect(errors).toEqual([])
  await context.close()
  passed.push('copy')
 }

 // Update from original shows the changed paths and, for a skill edited since it was imported, says the edits go and
 // stay in history. Confirm posts the digest and revision it showed. Escape closes it and focus comes back.
 if (run('original')) {
  const {context, page, errors, w} = await open()
  const detail = await openRow(page, 'seo-cluster')
  const button = detail.getByRole('button', {name:'Update from original'})
  await button.click()
  const dialog = page.getByRole('dialog', {name:'Update seo-cluster from its original?'})
  await expect(dialog).toBeVisible()
  const changes = dialog.getByRole('list', {name:'What changed'})
  await expect(changes.getByRole('listitem')).toHaveCount(2)
  await expect(changes).toContainText('SKILL.md')
  await expect(changes).toContainText('references/terms.md')
  await expect(dialog.getByText("Your edits will be replaced; they stay in this skill's history.")).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(button).toBeFocused()
  expect(posts(w, 'original')).toEqual([])
  // Without edits there is no warning.
  w.original = {...w.original, edited:false}
  await button.click()
  await expect(dialog.getByRole('list', {name:'What changed'}).getByRole('listitem')).toHaveCount(2)
  await expect(dialog.getByText("Your edits will be replaced; they stay in this skill's history.")).toHaveCount(0)
  await dialog.getByRole('button', {name:'Update from original'}).click()
  await expect(dialog).toHaveCount(0)
  expect(posts(w, 'original')).toEqual([{digest:'digest-2', revision:'rev-1'}])
  await expect(page.getByRole('status').filter({hasText:'Updated seo-cluster from its original'})).toBeVisible()
  // Nothing changed at the original: nothing to confirm.
  w.original = {...w.original, changes:[], state:'equal'}
  await button.click()
  await expect(dialog.getByText('Nothing has changed at the original since reet took its copy.')).toBeVisible()
  await expect(dialog.getByRole('button', {name:'Update from original'})).toHaveCount(0)
  await dialog.getByRole('button', {name:'Close'}).click()
  expect(posts(w, 'original')).toHaveLength(1)
  // reet has no copy yet: every file of the original is new, and the confirmation names no revision.
  w.original = {changes:[{path:'SKILL.md', kind:'added'}], state:'changed', digest:'digest-3', revision:'', edited:false}
  await button.click()
  await expect(dialog.getByText('reet has no copy of seo-cluster yet, so every file is new.')).toBeVisible()
  await expect(dialog.getByRole('list', {name:'What changed'}).getByRole('listitem')).toHaveText([/SKILL\.md\s*New/])
  await dialog.getByRole('button', {name:'Update from original'}).click()
  await expect(dialog).toHaveCount(0)
  expect(posts(w, 'original')[1]).toEqual({digest:'digest-3', revision:''})
  expect(errors).toEqual([])
  await context.close()
  passed.push('original')
 }

 // A machine's edit was put back: the opened skill says so, and Use this version posts that machine's agent id. When the
 // backup can't be used, the reason shows in place of the button.
 if (run('use version')) {
  const {context, page, errors, w} = await open()
  const detail = await openRow(page, 'seo-cluster')
  await expect(detail.getByText("seo-cluster was edited on ThinkPad; reet's copy put back.")).toBeVisible()
  await detail.getByRole('button', {name:'Use this version'}).click()
  await expect.poll(() => posts(w, 'use-version')).toEqual([{agentId:'agent-thinkpad'}])
  await expect(page.getByRole('status').filter({hasText:'Asked ThinkPad for its version of seo-cluster'})).toBeVisible()
  const kept = await openRow(page, 'improve-animations')
  await expect(kept.getByText("improve-animations was edited on Elitebook; reet's copy put back.")).toBeVisible()
  await expect(kept.getByText('The copy on Elitebook holds a file over 8 MiB, which reet does not copy.')).toBeVisible()
  await expect(kept.getByRole('button', {name:'Use this version'})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  passed.push('use version')
 }

 // Publish asks first, then posts {public:true}; the install command follows. Unpublish posts {public:false}. A name a
 // public address can't take says so in plain words instead of offering Publish.
 if (run('publish')) {
  const {context, page, errors, w} = await open()
  const detail = await openRow(page, 'seo-cluster')
  await detail.getByRole('button', {name:'Publish seo-cluster'}).click()
  const ask = page.getByRole('dialog', {name:'Publish seo-cluster?'})
  await expect(ask).toContainText('anyone')
  await ask.getByRole('button', {name:'Publish', exact:true}).click()
  await expect.poll(() => posts(w, 'publish')).toEqual([{public:true}])
  await expect(detail.getByRole('button', {name:'Copy install command for seo-cluster'})).toBeVisible()
  await detail.getByRole('button', {name:'Unpublish seo-cluster'}).click()
  await expect.poll(() => posts(w, 'publish')).toEqual([{public:true}, {public:false}])
  await expect(detail.getByRole('button', {name:/Copy install command/})).toHaveCount(0)
  // The server's reason shows beside the button when it refuses.
  await page.route('**/api/v2/skills/item-seo-cluster/publish', route => route.fulfill({status:409, json:{code:'not-publishable', error:'Choose a publishing username in Write a skill first; it is the first part of every public address.'}}))
  await detail.getByRole('button', {name:'Publish seo-cluster'}).click()
  await page.getByRole('dialog', {name:'Publish seo-cluster?'}).getByRole('button', {name:'Publish', exact:true}).click()
  await expect(detail.getByRole('alert')).toHaveText('Choose a publishing username in Write a skill first; it is the first part of every public address.')
  const odd = await openRow(page, 'my_skill')
  await expect(odd.getByRole('button', {name:/^Publish/})).toHaveCount(0)
  await expect(odd.getByText('reet.sh can only publish a skill whose name uses lowercase letters, numbers and single hyphens, so my_skill stays private.')).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
  passed.push('publish')
 }

 // A published skill whose page can't follow its latest version says so beside Unpublish, from the row and straight
 // after an Update from original that left the page behind.
 if (run('public problem')) {
  const {context, page, errors, w} = await open({edit: w => {
   w.rows = w.rows.map(r => r.name === 'hosted-tips' ? {...r, publicProblem:'The public page still shows the version before: hosted-tips can\'t be published yet: add instructions below the SKILL.md frontmatter.'} : r.name === 'seo-cluster' ? {...r, published:true, publicUrl:'https://reet.example/sam/seo-cluster'} : r)
  }})
  const tips = await openRow(page, 'hosted-tips')
  await expect(tips.getByText("The public page still shows the version before: hosted-tips can't be published yet: add instructions below the SKILL.md frontmatter.")).toBeVisible()
  const detail = await openRow(page, 'seo-cluster')
  await expect(detail.getByText(/The public page still shows/)).toHaveCount(0)
  await page.route('**/api/v2/skills/item-seo-cluster/original', route => route.request().method() === 'GET'
   ? route.fulfill({json:w.original})
   : route.fulfill({json:{updated:true, digest:'digest-2', edited:false, publicProblem:'The public page still shows the version before: seo-cluster can\'t be published yet: the SKILL.md name must match the skill name in the URL.'}}))
  await detail.getByRole('button', {name:'Update from original'}).click()
  await page.getByRole('dialog', {name:'Update seo-cluster from its original?'}).getByRole('button', {name:'Update from original'}).click()
  await expect(detail.getByText("The public page still shows the version before: seo-cluster can't be published yet: the SKILL.md name must match the skill name in the URL.")).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
  passed.push('public problem')
 }

 // Tags and Lists each have Add. A tag or a list changes nothing the rest of the app lists, so nothing is read again.
 if (run('tags and lists')) {
  const {context, page, errors, w} = await open()
  const detail = await openRow(page, 'seo-cluster')
  await detail.getByRole('button', {name:'Add tag'}).click()
  const field = detail.getByRole('combobox', {name:'New tag for seo-cluster'})
  await expect(field).toBeFocused()
  await field.fill('content')
  await field.press('Enter')
  await expect(detail.getByRole('list', {name:'Tags of seo-cluster'})).toContainText('content')
  expect(posts(w, 'tags')).toEqual([{id:'demo/toolkit/seo-cluster', add:['content'], remove:[]}])
  await expect(detail.getByRole('button', {name:'Add tag'})).toBeFocused()
  await detail.getByRole('button', {name:'Add to list'}).click()
  const pick = detail.getByRole('combobox', {name:'List for seo-cluster'})
  await expect(pick.getByRole('option')).toHaveText(['Writing'])
  await detail.getByRole('button', {name:'Add', exact:true}).click()
  await expect(detail.getByRole('list', {name:'Lists with seo-cluster'})).toContainText('Writing')
  expect(posts(w, 'list')).toEqual([{id:'demo/toolkit/seo-cluster'}])
  expect(errors).toEqual([])
  await context.close()
  passed.push('tags and lists')
 }

 // A like, a note or a tag changes nothing Jump or another page lists: the app reads nothing again. An Archive does, so
 // the app reads its library again, but the Skills page does not read itself again for its own change.
 if (run('reads')) {
  const {context, page, errors, w} = await open()
  const shelfReads = []
  page.on('request', req => { if (new URL(req.url()).pathname === '/api/skills' && req.method() === 'GET') shelfReads.push(Date.now()) })
  // The app reads its library once as it opens; count from after that.
  await page.waitForTimeout(1500)
  shelfReads.length = 0
  const pageReads = () => w.gets.filter(g => g[0] === 'page').length
  let before = pageReads()
  await row(page, 'seo-cluster').getByRole('button', {name:'Like seo-cluster'}).click()
  await expect.poll(() => posts(w, 'skill')).toHaveLength(1)
  const detail = await openRow(page, 'seo-cluster')
  await detail.getByRole('button', {name:'Edit note'}).click()
  await detail.getByRole('textbox', {name:'Notes on seo-cluster'}).fill('Keep clusters small.')
  await detail.getByRole('button', {name:'Save note'}).click()
  await expect.poll(() => posts(w, 'skill')).toHaveLength(2)
  await page.waitForTimeout(1500)
  expect(pageReads() - before, 'a like and a note read the page again').toBe(0)
  expect(shelfReads, 'a like and a note make the app read the library again').toEqual([])
  before = pageReads()
  await row(page, 'reet-babysit').getByRole('button', {name:'Archive reet-babysit'}).click()
  await expect.poll(() => posts(w, 'status')).toHaveLength(1)
  await expect.poll(() => shelfReads.length, {message:'an Archive tells the app'}).toBeGreaterThan(0)
  await page.waitForTimeout(1500)
  // The Archive's own answer is the tab in view; the other tab is read once. Its own event reads nothing more.
  expect(pageReads() - before).toBe(1)
  expect(errors).toEqual([])
  await context.close()
  passed.push('reads')
 }

 // Edit on a skill you did not write opens it in Write a skill by its library item: its SKILL.md as reet holds it, saved
 // as the next revision against the one it was read at. No publishing username is needed to edit.
 if (run('write a skill')) {
  const {context, page, errors, w} = await open()
  const md = '---\nname: seo-cluster\ndescription: Group keywords.\n---\nCluster them.\n'
  const b64 = text => Buffer.from(text).toString('base64')
  let saved = null
  await page.route('**/api/v2/skills/item-seo-cluster/content', route => {
   if (route.request().method() === 'GET') return route.fulfill({json:{id:'item-seo-cluster', name:'seo-cluster', revision:'rev-1', published:true, files:[{path:'SKILL.md', data:b64(md), executable:false}, {path:'scripts/run.sh', data:b64('echo hi\n'), executable:true}]}})
   saved = route.request().postDataJSON()
   return route.fulfill({json:{revision:'rev-2', publicProblem:'The public page still shows the version before: it needs a description.'}})
  })
  const detail = await openRow(page, 'seo-cluster')
  await detail.getByRole('link', {name:'Edit seo-cluster'}).click()
  await expect(page).toHaveURL(/\/hosting\?item=item-seo-cluster$/)
  await expect(page.getByRole('heading', {name:'Edit seo-cluster'})).toBeVisible()
  const box = page.getByRole('textbox', {name:'SKILL.md'})
  await expect(box).toHaveValue(md)
  await expect(page.getByText('scripts/run.sh')).toBeVisible()
  await box.fill(md + 'Keep clusters small.\n')
  await page.getByRole('button', {name:'Save', exact:true}).click()
  // The save is a success; the public page that could not follow is a warning of its own, never inside the success.
  const good = page.locator('.notice.good')
  await expect(good).toHaveText("Sorted. Saved seo-cluster; it goes on every machine, and the version before stays in its history.")
  const warn = page.locator('.item-editor .notice.warn').filter({hasText:'The public page still shows the version before: it needs a description.'})
  await expect(warn).toBeVisible()
  // Each notice wears its tone: green for done, amber for a warning, red for a refusal, in the app's own colours.
  const tone = (sel, token) => page.evaluate(([sel, token]) => {
   const probe = document.createElement('span'); probe.style.color = `var(${token})`; document.body.append(probe)
   const want = getComputedStyle(probe).color; probe.remove()
   return getComputedStyle(document.querySelector(sel)).borderLeftColor === want
  }, [sel, token])
  expect(await tone('.notice.good', '--good'), 'a done notice is green').toBe(true)
  expect(await tone('.item-editor .notice.warn', '--warn'), 'a warning is amber').toBe(true)
  expect(saved.revision).toBe('rev-1')
  expect(saved.files.map(f => [f.path, Buffer.from(f.data, 'base64').toString(), f.executable])).toEqual([['SKILL.md', md + 'Keep clusters small.\n', false], ['scripts/run.sh', 'echo hi\n', true]])
  // A changed name in the front matter would not rename the skill: it says so.
  const warning = page.getByText('SKILL.md says name: seo-groups, but this skill is seo-cluster.')
  await expect(warning).toHaveCount(0)
  await box.fill(md.replace('name: seo-cluster', 'name: seo-groups'))
  await expect(warning).toBeVisible()
  await expect(page.getByRole('status').filter({hasText:'Saving keeps it seo-cluster on every machine'})).toBeVisible()
  // A published skill's page stops following it while the names differ.
  await expect(page.getByRole('status').filter({hasText:'Saving keeps it seo-cluster on every machine'})).toContainText('Its public page stops following it until the names match.')
  expect(errors).toEqual([])
  await context.close()
  // On a phone, light and dark: nothing under 14 px, 44 px targets, no sideways scroll.
  for (const theme of ['light', 'dark']) {
   const f = await openWorkspace(browser, {width:390, height:844, theme, touch:true, routes:routes(world())})
   await f.page.route('**/api/v2/skills/item-seo-cluster/content', route => route.fulfill({json:{id:'item-seo-cluster', name:'seo-cluster', revision:'rev-1', files:[{path:'SKILL.md', data:b64(md.replace('name: seo-cluster', 'name: seo-groups')), executable:false}, {path:'references/a-rather-long-file-name-for-a-phone.md', data:b64('x'), executable:false}]}}))
   await f.page.goto(`${base}/hosting?item=item-seo-cluster`)
   await expect(f.page.getByRole('textbox', {name:'SKILL.md'})).toBeVisible()
   await expect(f.page.getByText('SKILL.md says name: seo-groups, but this skill is seo-cluster.')).toBeVisible()
   expect(await under14(f.page, '.item-editor'), `Write a skill text under 14 px at 390px ${theme}`).toEqual([])
   for (const el of await f.page.locator('.item-editor').locator('button, a').all()) {
    expect((await el.boundingBox()).height, `${await el.textContent()} at 390px ${theme}`).toBeGreaterThanOrEqual(44)
   }
   expect(await f.page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `no sideways scroll at 390px ${theme}`).toBe(true)
   expect(f.errors).toEqual([])
   await f.context.close()
  }
  passed.push('write a skill')
 }

 // At each width, light and dark: nothing under 14 px, every target at least 44 px, no sideways scroll.
 if (run('layout')) {
  for (const width of [1440, 1024, 390]) for (const theme of ['light', 'dark']) {
   const {context, page, errors} = await open({width, height:900, theme, touch:width === 390})
   const detail = await openRow(page, 'seo-cluster')
   await expect(detail.getByRole('button', {name:'Use this version'})).toBeVisible()
   expect(await under14(page, '[data-skill="item-seo-cluster"] .detail'), `text under 14 px at ${width}px ${theme}`).toEqual([])
   for (const el of await detail.locator('button, a, select, input').all()) {
    if (!(await el.isVisible())) continue
    const box = await el.boundingBox()
    expect(box.height, `${await el.textContent()} at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
   }
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `no sideways scroll at ${width}px ${theme}`).toBe(true)
   await detail.getByRole('button', {name:'Update from original'}).click()
   const dialog = page.getByRole('dialog', {name:'Update seo-cluster from its original?'})
   await expect(dialog.getByRole('list', {name:'What changed'})).toBeVisible()
   // Measured once the dialog has finished growing in.
   await page.waitForTimeout(300)
   expect(await under14(page, 'dialog[open]'), `dialog text under 14 px at ${width}px ${theme}`).toEqual([])
   for (const el of await dialog.getByRole('button').all()) expect((await el.boundingBox()).height, `${await el.textContent()} in the dialog at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
   expect(errors).toEqual([])
   await context.close()
  }
  passed.push('layout')
 }
 console.log(`Opened skill passed: ${passed.join(', ')}`)
} finally {
 await browser.close()
}
