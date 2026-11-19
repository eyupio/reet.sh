// Skills found on a machine that reet did not put there wait as New on a machine: one banner above the tabs names the
// machine and the skills, and Review asks about each with Keep and Archive, plus Keep all. Nothing spreads until the
// person says so; when the machines' copies differ, Keep asks which machine's copy to take.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const copy = (agentId, host) => ({agentId, host, folder:'~/.agents/skills'})
const found = (name, copies = [copy('a1', 'Elitebook')], digestsDiffer = false) => ({name, copies, digestsDiffer})

function routes(w) {
 return {
  '/api/v2/skills': req => skillsPageOf(w.rows, new URL(req.url()).searchParams.get('status') || 'active', {newOnMachines:w.found}),
  '/api/v2/skills/new': req => {
   const b = req.postDataJSON(); w.posts.push(b)
   w.found = w.found.filter(n => !b.names.includes(n.name))
   w.rows = [...w.rows, ...b.names.map(name => skillRow(name, {status:b.action === 'keep' ? 'active' : 'archived', originKind:'kept', origin:'Elitebook', hasCopy:false, health:{state:'copying',pending:0,total:3,hosts:['Elitebook'],machines:[]}}))]
   return {items:b.names.map(n => `item-${n}`)}
  },
 }
}

async function open(found_, {path = '/skills', ...options} = {}) {
 const w = {rows:[skillRow('seo-audit')], found:found_, posts:[]}
 const f = await openWorkspace(browser, {width:1440, height:900, ...options, routes:routes(w)})
 await f.page.goto(`${base}${path}`)
 return {...f, w}
}
const banner = page => page.getByRole('region', {name:'New on a machine'})

try {
 // The banner names the machine and the skills; Review asks about each. Keep posts keep, Archive posts archive, and
 // each leaves the review as it is decided.
 if (run('review')) {
  const {context, page, errors, w} = await open([found('paseo-help'), found('cq')])
  await expect(banner(page)).toContainText('2 new skills turned up on Elitebook')
  await expect(banner(page)).toContainText('paseo-help, cq')
  // It sits above the tabs.
  const [b, t] = [await banner(page).boundingBox(), await page.getByRole('tablist', {name:'Skill state'}).boundingBox()]
  expect(b.y + b.height).toBeLessThanOrEqual(t.y)
  await banner(page).getByRole('button', {name:'Review'}).click()
  const dialog = page.getByRole('dialog', {name:'New on Elitebook'})
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('Nothing spreads until you choose.')
  await dialog.getByRole('button', {name:'Keep paseo-help'}).click()
  await expect(dialog.getByRole('button', {name:'Keep paseo-help'})).toHaveCount(0)
  expect(w.posts).toEqual([{names:['paseo-help'], action:'keep'}])
  await expect(dialog.getByRole('status')).toContainText('Kept paseo-help · going on every machine')
  await dialog.getByRole('button', {name:'Archive cq'}).click()
  await expect(dialog.getByRole('button', {name:'Archive cq'})).toHaveCount(0)
  expect(w.posts[1]).toEqual({names:['cq'], action:'archive'})
  await expect(dialog).toContainText('All sorted.')
  await dialog.getByRole('button', {name:'Close'}).click()
  await expect(dialog).toHaveCount(0)
  // The banner goes with the last of them, and the kept skill is on the Active tab.
  await expect(banner(page)).toHaveCount(0)
  await expect(page.locator('[data-skill="item-paseo-help"]')).toBeVisible()
  await expect(page.getByRole('tab', {name:/^Archived/})).toContainText('1')
  expect(new URL(page.url()).pathname).toBe('/skills')
  expect(errors).toEqual([])
  await context.close()
 }

 // Keep all sends both names in one request.
 if (run('keep all')) {
  const {context, page, errors, w} = await open([found('paseo-help'), found('cq')])
  await banner(page).getByRole('button', {name:'Review'}).click()
  const dialog = page.getByRole('dialog', {name:'New on Elitebook'})
  await dialog.getByRole('button', {name:'Keep all'}).click()
  await expect(dialog).toContainText('All sorted.')
  expect(w.posts).toEqual([{names:['paseo-help','cq'], action:'keep'}])
  expect(errors).toEqual([])
  await context.close()
 }

 // Keep all that keeps some and then fails on another says why, and the one it could not keep is still there to decide.
 if (run('keep all error')) {
  const {context, page, errors, w} = await open([found('paseo-help'), found('askme', [copy('a1', 'Elitebook'), copy('a2', 'ThinkPad')], true)])
  await page.route('**/api/v2/skills/new', async route => {
   if (route.request().postDataJSON().names.includes('askme')) return route.fulfill({status:500, json:{error:'ThinkPad did not answer.'}})
   return route.fallback()
  })
  await banner(page).getByRole('button', {name:'Review'}).click()
  const dialog = page.getByRole('dialog', {name:'New on Elitebook and ThinkPad'})
  await dialog.getByRole('button', {name:'Keep all'}).click()
  await expect(dialog.getByRole('button', {name:'Keep paseo-help'})).toHaveCount(0)
  await expect(dialog.getByRole('alert')).toHaveText('ThinkPad did not answer.')
  await expect(dialog.getByRole('button', {name:'Keep askme'})).toBeVisible()
  await expect(dialog.getByRole('status')).toContainText('Kept paseo-help · going on every machine')
  expect(w.posts).toEqual([{names:['paseo-help'], action:'keep'}])
  expect(errors).toEqual([])
  await context.close()
 }

 // When the machines' copies differ, the review asks which machine's copy to keep and Keep sends it.
 if (run('choose copy')) {
  const {context, page, errors, w} = await open([found('askme', [copy('a1', 'Elitebook'), copy('a2', 'ThinkPad')], true), found('cq')])
  await expect(banner(page)).toContainText('2 new skills turned up on Elitebook and ThinkPad')
  await banner(page).getByRole('button', {name:'Review'}).click()
  const dialog = page.getByRole('dialog', {name:'New on Elitebook and ThinkPad'})
  const choice = dialog.getByRole('radiogroup', {name:'Which copy of askme to keep'})
  await expect(choice.getByRole('radio', {name:'Elitebook'})).toBeChecked()
  await choice.getByRole('radio', {name:'ThinkPad'}).check()
  // A skill whose copies agree asks nothing.
  await expect(dialog.getByRole('radiogroup')).toHaveCount(1)
  await dialog.getByRole('button', {name:'Keep askme'}).click()
  await expect(dialog.getByRole('button', {name:'Keep askme'})).toHaveCount(0)
  expect(w.posts).toEqual([{names:['askme'], action:'keep', from:'a2'}])
  expect(errors).toEqual([])
  await context.close()
 }

 // Keep takes a machine's copy (the server picks that machine's first folder), so a machine holding the skill in two
 // folders is one choice, not two radios that share a value and both show as chosen.
 if (run('one choice per machine')) {
  const two = [copy('a1', 'Elitebook'), {...copy('a1', 'Elitebook'), folder:'~/.claude/skills'}, copy('a2', 'ThinkPad')]
  const {context, page, errors, w} = await open([found('askme', two, true)])
  await banner(page).getByRole('button', {name:'Review'}).click()
  const dialog = page.getByRole('dialog', {name:'New on Elitebook and ThinkPad'})
  const choice = dialog.getByRole('radiogroup', {name:'Which copy of askme to keep'})
  await expect(choice.getByRole('radio')).toHaveCount(2)
  await expect(choice.getByRole('radio', {checked:true})).toHaveCount(1)
  await choice.getByRole('radio', {name:/^ThinkPad/}).check()
  await expect(choice.getByRole('radio', {checked:true})).toHaveCount(1)
  await choice.getByRole('radio', {name:/^Elitebook/}).check()
  await dialog.getByRole('button', {name:'Keep askme'}).click()
  await expect(dialog.getByRole('button', {name:'Keep askme'})).toHaveCount(0)
  expect(w.posts).toEqual([{names:['askme'], action:'keep', from:'a1'}])
  expect(errors).toEqual([])
  await context.close()
 }

 // Home's Waiting for you links to /skills?review=new, which opens the review at once.
 if (run('link')) {
  const {context, page, errors} = await open([found('paseo-help'), found('cq')], {path:'/skills?review=new'})
  const dialog = page.getByRole('dialog', {name:'New on Elitebook'})
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button', {name:'Keep paseo-help'})).toBeVisible()
  await dialog.getByRole('button', {name:'Not now'}).click()
  await expect(dialog).toHaveCount(0)
  await expect(banner(page)).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // On a phone, light and dark: the banner and the review fit, with 44 px targets.
 if (run('phone')) {
  for (const theme of ['light', 'dark']) {
   const {context, page, errors} = await open([found('paseo-help'), found('cq')], {width:390, height:844, theme, touch:true})
   await expect(banner(page)).toBeVisible()
   await banner(page).getByRole('button', {name:'Review'}).click()
   const dialog = page.getByRole('dialog', {name:'New on Elitebook'})
   // Measured once the dialog has finished rising into place.
   await dialog.evaluate(el => Promise.all(el.getAnimations({subtree:true}).map(a => a.finished)))
   for (const button of await dialog.getByRole('button').all()) {
    const box = await button.boundingBox()
    expect(box.height, `${await button.textContent()} height (${theme})`).toBeGreaterThanOrEqual(44)
    expect(box.x + box.width, `${await button.textContent()} right edge (${theme})`).toBeLessThanOrEqual(390)
   }
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width (${theme})`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('New on a machine passed: the banner names the machine and the skills above the tabs, Review keeps or archives each, Keep all sends both names, differing copies ask which machine to keep from, ?review=new opens the review, and it fits a phone in light and dark.')
} finally {await browser.close()}
