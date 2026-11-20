// The retained machine page reads reconcile reports, never the removed engine. Includes legacy links and failed reads.
import {chromium, expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace, noWork} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const group={id:'laptop-agents',host:'Laptop',agent:'agents',name:'Laptop',skills:[],skillCount:0,complete:true,scannedAt:now,lastSeen:now,os:'linux',arch:'amd64',version:'v1.0.0'}
const agent={id:'agent-laptop',host:'Laptop',version:'v1.0.0',lastSeen:now,paused:false,autoUpdate:false}
const sync={environments:[group],machines:[{host:'Laptop',groups:[group],skillCount:0}],agents:[agent],targets:[],jobs:[],updateJobs:[],releases:{latest:{version:'v2.0.0'}}}
const health=state=>({active:2,work:noWork,activity:[],machines:[{agentId:agent.id,host:agent.host,state,paused:false,online:true,protocol:2,waiting:[],failed:[],lastSeen:now}]})
try {
 for(const width of [1440,390])for(const theme of ['light','dark']) {
  let report=health('installing')
  const writes=[]
  const f=await openWorkspace(browser,{width,theme,touch:width===390,sync,inventory:{environments:sync.environments,machines:sync.machines},routes:{
   '/api/v2/sync/skills':()=>report,
   '/api/v2/sync/agents':req=>{writes.push(req.postDataJSON());return {...sync,agents:[{...agent,paused:true}]}},
  }})
  const {page,context,errors,state}=f
  await page.goto(`${base}/machines`)
  const card=page.getByRole('article',{name:'Laptop',exact:true})
  await expect(card).toContainText('Installing skills…')
  await expect(card.getByRole('link',{name:'View sync status',exact:true})).toHaveAttribute('href','/sync')
  await expect(card.getByRole('button',{name:/Skills on this machine|Sync now|Preview|Private skills|Add a skill/})).toHaveCount(0)
  await expect(card.getByRole('button',{name:/Review update/})).toBeVisible()
  const menu=card.locator('details.sync-menu')
  await menu.locator('summary').click()
  await menu.getByRole('menuitem',{name:'Settings…'}).click()
  const settings=page.getByRole('dialog')
  await expect(settings.getByRole('switch',{name:'Pause sync',exact:true})).toBeVisible()
  await settings.getByRole('switch',{name:'Pause sync',exact:true}).click()
  expect(writes).toEqual([{id:agent.id,action:'pause'}])
  await expect(settings.getByRole('switch',{name:'Pause sync',exact:true})).toHaveAttribute('aria-checked','true')
  await page.keyboard.press('Escape')
  // A failed current read cannot fall back to the healthy inventory as proof of sync.
  report={status:503,json:{error:'Report unavailable'}}
  await page.getByRole('button',{name:'Refresh status',exact:true}).click()
  await expect(card).toContainText('Skill sync not confirmed')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machines-library-${width}-${theme}.png`,fullPage:true})}
  expect(errors).toEqual([]);expect(state.unexpected).toEqual([])
  await context.close()
 }
 const f=await openWorkspace(browser,{width:1440,routes:{'/api/v2/sync/skills':health('ok')}})
 for(const route of ['/sync/advanced?remove=a#sync-jobs','/account/sync','/machines?view=compare&list=7']) {
  await f.page.goto(`${base}${route}`)
  await expect(f.page).toHaveURL(`${base}/sync`)
  await expect(f.page.getByRole('link',{name:'Advanced',exact:true})).toHaveCount(0)
  await expect(f.page.locator('#sync-selection')).toHaveCount(0)
 }
 expect(f.errors).toEqual([]);expect(f.state.unexpected).toEqual([])
 await f.context.close()
 console.log('Library machine flow passed: reconcile health, failed reads, pause, updates, removed controls, legacy redirects and layout.')
}finally{await browser.close()}
