// A skill found on only one machine reaches the others once it is kept: the review takes reet's copy from the machine it
// was found on, and the skill's row follows the copy and the installs live, on desktop and phone.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const health=(state,over={})=>({state,pending:0,total:2,hosts:[],machines:[],...over})
try {
 for(const width of [390,1280]) {
  const w={rows:[],found:[{name:'improve',copies:[{agentId:'a0',host:'Elitebook',folder:'~/.agents/skills'}],digestsDiffer:false}],posts:[]}
  const fixture=await openWorkspace(browser,{width,height:900,touch:width===390,routes:{
   '/api/v2/skills':req=>skillsPageOf(w.rows,new URL(req.url()).searchParams.get('status')||'active',{newOnMachines:w.found,machines:2}),
   '/api/v2/skills/new':req=>{const body=req.postDataJSON();w.posts.push(body);w.found=[];w.rows=[skillRow('improve',{originKind:'kept',origin:'Elitebook',hasCopy:false,health:health('copying',{hosts:['Elitebook']})})];return {items:['item-improve']}},
  }})
  const {page}=fixture
  // Nothing spreads before the person says so: the skill waits in the banner, not in the list.
  await page.goto(`${base}/skills`)
  const banner=page.getByRole('region',{name:'New on a machine'})
  await expect(banner).toContainText('1 new skill turned up on Elitebook')
  await expect(page.locator('[data-skill]')).toHaveCount(0)
  await banner.getByRole('button',{name:'Review'}).click()
  const dialog=page.getByRole('dialog',{name:'New on Elitebook'})
  await dialog.getByRole('button',{name:'Keep improve'}).click()
  await expect(dialog).toContainText('All sorted.')
  expect(w.posts).toEqual([{names:['improve'],action:'keep'}])
  await dialog.getByRole('button',{name:'Close'}).click()
  // reet copies it in from Elitebook, then the other machine installs it, and the row says so as it goes.
  const row=page.locator('[data-skill="item-improve"]')
  await expect(row.locator('.health')).toHaveText('Copying into reet from Elitebook')
  await expect(banner).toHaveCount(0)
  const refresh=()=>page.evaluate(()=>window.dispatchEvent(new Event('reet-shelf-changed')))
  w.rows=[{...w.rows[0],hasCopy:true,health:health('installing',{pending:1,hosts:['PROD_Snicket']})}]
  await refresh()
  await expect(row.locator('.health')).toHaveText('Installing on 1 of 2')
  w.rows=[{...w.rows[0],health:health('ok')}]
  await refresh()
  await expect(row.locator('.health')).toHaveText('On all 2 machines')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(fixture.errors).toEqual([])
  expect(fixture.state.unexpected).toEqual([])
  await fixture.context.close()
 }
 console.log('Machine-only skills reach every machine once kept: reet copies them in from where they were found, with live install progress on desktop and phone.')
} finally {await browser.close()}
