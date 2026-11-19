// A skill's row says in one line where it stands on the machines, toned, and offers Retry only where retrying helps.
// Its actions are Archive or Restore beside Delete; the name opens the row onto each machine. On a phone the row keeps
// 44 px targets and the page its width.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
async function open(rows, options = {}) {
 const f = await openWorkspace(browser, {width:1440, ...options, routes:{'/api/v2/skills':req=>skillsPageOf(rows,new URL(req.url()).searchParams.get('status')||'active')}})
 await f.page.goto(`${base}/skills`)
 return f
}
const h = (state, over = {}) => ({state, pending:0, total:3, hosts:[], machines:[], ...over})

// Every health state, the line it reads, its tone, and whether it offers Retry.
const states = [
 ['ok', h('ok'), 'On all 3 machines', 'ok', false],
 ['installing', h('installing', {pending:2}), 'Installing on 2 of 3', 'work', false],
 ['removing', h('removing', {pending:1, hosts:['ThinkPad']}), 'Coming off ThinkPad', 'work', false],
 ['failed', h('failed', {pending:1, hosts:['ThinkPad'], reason:'no write access'}), 'Failed on ThinkPad', 'bad', true],
 ['offline', h('offline', {pending:1, hosts:['studio']}), "Removing from studio when it's back online", 'work', false],
 ['blocked', h('blocked', {pending:1, hosts:['ThinkPad']}), "Can't write skills on ThinkPad", 'bad', false],
 ['copying', h('copying', {hosts:['Elitebook']}), 'Copying into reet from Elitebook', 'work', false],
 ['copy-failed', h('copy-failed', {hosts:['Elitebook'], reason:'over 8 MiB'}), "Couldn't copy into reet: over 8 MiB. It stays on Elitebook until you delete it.", 'bad', false],
 ['getting', h('getting', {hosts:['Elitebook']}), 'Getting a copy from Elitebook', 'work', false],
 ['fetch-failed', h('fetch-failed'), "Couldn't fetch the files", 'bad', true],
 ['needs-update', h('needs-update', {hosts:['ThinkPad']}), 'Update reet on ThinkPad to keep its skills in sync', 'bad', false],
 ['off', h('off'), 'Off every machine', 'off', false],
]

try {
 // Each state reads its own line, in its own tone, with Retry only after a failure that a retry can clear.
 if (run('health')) {
  const rows = states.map(([state, health]) => skillRow(`skill-${state}`, {health}))
  const {context, page, errors} = await open(rows, {height:1800})
  for (const [state, , text, tone, retry] of states) {
   const row = page.locator(`[data-skill="item-skill-${state}"]`)
   await expect(row.locator('.health'), state).toHaveText(text)
   await expect(row.locator('.health'), state).toHaveAttribute('data-tone', tone)
   await expect(row.getByRole('button', {name:`Retry skill-${state}`}), state).toHaveCount(retry ? 1 : 0)
  }
  // A healthy line is not drawn in the warning colours.
  const colours = await page.evaluate(() => {
   const of = id => getComputedStyle(document.querySelector(`[data-skill="${id}"] .health`)).color
   return {ok:of('item-skill-ok'), bad:of('item-skill-failed')}
  })
  expect(colours.ok).not.toBe(colours.bad)
  expect(errors).toEqual([])
  await context.close()
 }

 // An Active row offers Archive and Delete, an Archived one Restore and Delete, each named for its skill.
 if (run('actions')) {
  const {context, page, errors} = await open([skillRow('code-review'), skillRow('old-notes', {status:'archived', health:h('off')})])
  const active = page.locator('[data-skill="item-code-review"]')
  await expect(active.locator('[data-act]')).toHaveCount(2)
  await expect(active.locator('[data-act="archive"]')).toHaveAccessibleName('Archive code-review')
  await expect(active.locator('[data-act="delete"]')).toHaveAccessibleName('Delete code-review')
  await page.getByRole('tab', {name:/^Archived/}).click()
  const archived = page.locator('[data-skill="item-old-notes"]')
  await expect(archived.locator('[data-act="restore"]')).toHaveAccessibleName('Restore old-notes')
  await expect(archived.locator('[data-act="delete"]')).toHaveAccessibleName('Delete old-notes')
  expect(errors).toEqual([])
  await context.close()
 }

 // Pressing the name opens the row onto each machine and says so; pressing it again closes it.
 if (run('open')) {
  const machines = [{host:'Elitebook', state:'ok'}, {host:'ThinkPad', state:'failed', detail:'no write access'}, {host:'studio', state:'installing'}]
  const {context, page, errors} = await open([skillRow('seo-cluster', {health:h('failed', {pending:2, hosts:['ThinkPad'], machines})})])
  const name = page.getByRole('button', {name:'seo-cluster', exact:true})
  await expect(name).toHaveAttribute('aria-expanded', 'false')
  await name.click()
  await expect(name).toHaveAttribute('aria-expanded', 'true')
  const list = page.getByRole('list', {name:'Machines for seo-cluster'})
  await expect(list.getByRole('listitem')).toHaveText(['ElitebookInstalled', 'ThinkPadFailed: no write access', 'studioInstalling'])
  await name.click()
  await expect(list).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // On a phone, a long name and a long health line wrap inside the row: every button is a 44 px target and nothing
 // pushes the page wider than the screen.
 if (run('phone')) {
  const long = `seo-${'keyword-cluster-'.repeat(5)}tool`
  const rows = [skillRow(long, {health:h('copy-failed', {hosts:['Elitebook-workstation-2'], reason:'the folder holds more than 256 files'})}), skillRow('cq', {health:h('failed', {pending:1, hosts:['ThinkPad']})})]
  for (const width of [390, 320]) {
   const {context, page, errors} = await open(rows, {width, height:812, touch:true})
   await expect(page.locator('[data-skill]')).toHaveCount(2)
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
   for (const button of await page.locator('[data-skill] button').all()) {
    const box = await button.boundingBox()
    const label = await button.evaluate(el => el.getAttribute('aria-label') || el.textContent.trim())
    expect(box.height, `${label} height at ${width}px`).toBeGreaterThanOrEqual(44)
    expect(box.width, `${label} width at ${width}px`).toBeGreaterThanOrEqual(44)
    expect(box.x + box.width, `${label} right edge at ${width}px`).toBeLessThanOrEqual(width)
   }
   await page.getByRole('button', {name:long, exact:true}).click()
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `opened document width at ${width}px`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('Skill card: each health state reads its own line in its own tone with Retry only after a failure a retry clears, an Active row offers Archive and Delete and an Archived one Restore and Delete, the name opens the row onto each machine, and a phone keeps 44 px targets and its width.')
} finally {await browser.close()}
