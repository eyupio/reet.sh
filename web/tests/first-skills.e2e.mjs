// The first Skills screen has one obvious thing to do, and hosting says what it is before it asks for a permanent username.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Empty shelf: adding a skill leads. Nothing to sync yet, creating is secondary, and the same link is not offered twice.
 if (run('empty')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width})
   await page.goto(`${base}/skills`)
   await expect(page.getByRole('heading',{name:'Put the first skill on your shelf.'})).toBeVisible()
   await expect(page.getByRole('link',{name:'Sync skills'}), `Sync skills at ${width}px`).toHaveCount(0)
   const create = page.getByRole('link',{name:'Create a skill'})
   await expect(create).toBeVisible()
   await expect(create).not.toHaveClass(/primary/)
   await expect(page.getByRole('link',{name:'Host your own skill'})).toHaveCount(0)
   const filled = await page.locator('main .btn.primary').allInnerTexts()
   expect(filled.map(t=>t.trim()), `filled buttons at ${width}px`).toEqual(['Add to shelf','Add your first skill'])
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Once there is something on the shelf, syncing is offered, and creating stays secondary.
 if (run('populated')) {
  const {context,page} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('link',{name:'Sync skills'})).toHaveAttribute('href','/sync')
  await expect(page.getByRole('link',{name:'Create a skill'})).not.toHaveClass(/primary/)
  await context.close()
 }

 // On an empty shelf the tools are one quiet button, not a section with its own heading, and restore and import stay reachable.
 // With skills on the shelf the section is unchanged.
 if (run('tools')) {
  const empty = await open({width:1440})
  await empty.page.goto(`${base}/skills`)
  await expect(empty.page.getByRole('heading',{name:'Put the first skill on your shelf.'})).toBeVisible()
  await expect(empty.page.getByRole('heading',{name:'Shelf tools'})).toHaveCount(0)
  await empty.page.getByRole('button',{name:'Import or restore a backup'}).click()
  await expect(empty.page.getByRole('heading',{name:'Import what you have installed'})).toBeVisible()
  await expect(empty.page.getByRole('button',{name:'Restore from JSON'})).toBeVisible()
  await expect(empty.page.getByRole('heading',{name:'Keep a good find'}), 'the start card already offers the bookmarklets').toHaveCount(0)
  await empty.context.close()

  const full = await open({width:1440,skills:[skill('code-review')]})
  await full.page.goto(`${base}/skills`)
  await expect(full.page.getByRole('heading',{name:'Shelf tools'})).toBeVisible()
  await expect(full.page.getByRole('button',{name:'Import or restore a backup'})).toHaveCount(0)
  await full.page.getByRole('button',{name:/Show bookmarklets and tools/}).click()
  await expect(full.page.getByRole('button',{name:'Restore from JSON'})).toBeVisible()
  await expect(full.page.getByRole('heading',{name:'Keep a good find'})).toBeVisible()
  await full.context.close()
 }

 // While the shelf loads it shows placeholder cards rather than a bare "Loading…" line, and they give way to the skills.
 if (run('loading')) {
  let release
  const gate = new Promise(resolve=>{release=resolve})
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')],gates:{'/api/skills':gate}})
  await page.goto(`${base}/skills`)
  const loading = page.getByRole('status',{name:'Loading your shelf'})
  await expect(loading).toBeVisible()
  await expect(loading).toHaveAttribute('aria-busy','true')
  await expect(loading.locator('[data-skeleton]')).toHaveCount(3)
  await expect(page.getByText('Loading…',{exact:true})).toHaveCount(0)
  release()
  await expect(page.getByRole('heading',{name:'code-review'})).toBeVisible()
  await expect(loading).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Hosting: what it gives you comes first, then the username form.
 if (run('hosting')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/hosting`)
  const gives = page.getByRole('heading',{name:'What hosting gives you'})
  const claim = page.getByRole('heading',{name:'Choose your publishing username'})
  await expect(gives).toBeVisible()
  await expect(claim).toBeVisible()
  await expect(page.locator('main').getByText(`npx skills add ${base}/your-username/skill-name`)).toBeVisible()
  const [a,b] = [await gives.boundingBox(), await claim.boundingBox()]
  expect(a.y, 'the explanation sits above the username form').toBeLessThan(b.y)
  await expect(page.getByText(/This username is permanent/)).toBeVisible()
  await context.close()
 }
 console.log('First skills: add leads on an empty shelf, quiet tools on an empty shelf, a skeleton while it loads, sync appears with content, hosting explains itself before the username.')
} finally {await browser.close()}
