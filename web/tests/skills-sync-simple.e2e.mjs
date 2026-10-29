import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now()
const one = skill('review'), two = skill('testing')
const scope = { id:'shared', host:'Elitebook', agent:'agents', name:'Elitebook', skills:[], skillCount:0, complete:true, scannedAt:now,lastSeen:now }
const initial = () => ({ environments:[scope],machines:[{host:'Elitebook',groups:[scope],skillCount:0}],agents:[{id:'a1',host:'Elitebook',version:'test',lastSeen:Date.now(),paused:false,autoUpdate:false,keepSynced:false}],targets:[],jobs:[],runs:[] })
try {
 for (const width of [390,1280]) {
  let sync = initial()
  const writes=[]
  let failToggle=false
  const fixture=await openWorkspace(browser,{width,height:900,touch:width===390,skills:[one,two],sync,routes:{
   '/api/v2/sync':()=>sync,
   '/api/v2/sync/candidates':{skills:[one,two]},
   '/api/v2/sync/agents':request=>{const body=request.postDataJSON();if(failToggle)return {status:409,json:{error:'Could not save machine setting'}};writes.push(body);sync.agents[0].keepSynced=body.action==='keep-sync-on';return sync},
   '/api/v2/sync/desired':request=>{const body=request.postDataJSON();writes.push(body);sync.targets=[{target:scope,sourceId:'shelf',sourceName:'Shelf',mode:body.mode,desired:body.skillIds.map(id=>[one,two].find(skill=>skill.id===id)),missing:[one],extra:[],revision:'new',followSource:body.followSource}];return sync},
   '/api/v2/sync/run':request=>{writes.push(request.postDataJSON());sync.jobs=[{id:'new-job',agentId:'a1',targetId:'shared',kind:'auto',status:'running',plan:{},stage:'apply',stages:[{key:'read',label:'Read machine',status:'done'},{key:'plan',label:'Plan changes',status:'done'},{key:'apply',label:'Apply changes',status:'running',detail:'Installing review · 1 of 2'}],createdAt:Date.now(),updatedAt:Date.now()}];return sync},
  }})
  const {page}=fixture
  await page.goto(`${base}/sync`)
  await expect(page.getByRole('heading',{name:'Choose once. Keep in sync.'})).toBeVisible()
  await expect(page.getByRole('heading',{name:'Choose what to sync',exact:true})).not.toBeVisible()
  await page.getByRole('switch',{name:'Keep Elitebook synced'}).check()
  await expect(page.getByRole('switch',{name:'Keep Elitebook synced'})).toBeChecked()
  failToggle=true
  await page.getByRole('switch',{name:'Keep Elitebook synced'}).click()
  await expect(page.getByText('Could not save machine setting')).toBeVisible()
  await expect(page.getByRole('switch',{name:'Keep Elitebook synced'})).toBeChecked()
  failToggle=false
  if (process.env.REET_CAPTURE) await page.screenshot({path:`/workspace/work/sync-${width}.png`,fullPage:true})
  await page.getByRole('button',{name:'Sync now',exact:true}).click()
  await expect(page.getByText('Installing review · 1 of 2')).toBeVisible()
  const selection=writes.find(body=>body.targetId==='shared')
  expect(selection.skillIds).toEqual([one.id,two.id]);expect(selection.followSource).toBe(true);expect(selection.mode).toBe('additive')
  expect(writes.some(body=>body.agentId==='a1'&&body.targetIds[0]==='shared')).toBe(true)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(fixture.errors).toEqual([])
  await fixture.context.close()
 }
 // Grouped apply failures must chain scopes; exclusive auto jobs cannot be
 // independently queued while an earlier one waits on the same machine.
 const groupedState=initial()
 const second={...scope,id:'claude',agent:'claude'}
 groupedState.environments.push(second);groupedState.machines[0].groups.push(second)
 groupedState.targets=[scope,second].map(target=>({target,sourceId:'shelf',sourceName:'Shelf',desired:[one],missing:[one],extra:[],mode:'additive',revision:'r'}))
 groupedState.jobs=[scope,second].map(target=>({id:`failed-${target.id}`,agentId:'a1',targetId:target.id,kind:'apply',status:'failed',revision:'r',hash:'old',plan:{},message:'approved plan no longer matches local files or desired revision; preview again',createdAt:now-10000,updatedAt:now-9000}))
 const chained=[]
 const grouped=await openWorkspace(browser,{width:390,height:844,touch:true,sync:groupedState,routes:{
  '/api/v2/sync/candidates':{skills:[one]},
  '/api/v2/sync/jobs':()=>{throw new Error('Grouped auto retries must use a run')},
  '/api/v2/sync/run':request=>{chained.push(request.postDataJSON());return groupedState},
 }})
 await grouped.page.goto(`${base}/sync`)
 await grouped.page.getByRole('button',{name:'Fix and try again',exact:true}).click()
 await expect(grouped.page.getByText('Fresh sync queued for 2 jobs on Elitebook. They appear at the top of Sync activity.')).toBeVisible()
 expect(chained).toEqual([{agentId:'a1',targetIds:['shared','claude']}])
 expect(grouped.errors).toEqual([])
 await grouped.context.close()
 let hosted={slug:'reet-review',description:'Reviews code',content:'---\nname: reet-review\ndescription: Reviews code\n---\nReview carefully.',files:{'guide.md':'keep this'},published:false,createdAt:now,updatedAt:now}
 const shelf=await openWorkspace(browser,{width:390,height:844,touch:true,skills:[one],routes:{
  '/api/v2/hosting':()=>({username:'sam',prefix:'reet-',skills:[hosted]}),
  '/api/v2/hosting/skills/reet-review':request=>{hosted=request.postDataJSON();return hosted},
 }})
 await shelf.page.goto(`${base}/skills`)
 await expect(shelf.page.getByRole('button',{name:'Copy prompt for review',exact:true})).toBeVisible()
 await expect(shelf.page.getByRole('heading',{name:/Skills you create/})).toBeVisible()
 await shelf.page.getByRole('button',{name:'Publish & share'}).click()
 await expect(shelf.page.getByRole('button',{name:'Copy share link for reet-review'})).toBeVisible()
 expect(hosted.files).toEqual({'guide.md':'keep this'});expect(hosted.published).toBe(true)
 await shelf.page.getByRole('switch',{name:'Make reet-review public'}).uncheck()
 await expect(shelf.page.getByText('Private draft',{exact:true})).toBeVisible()
 expect(hosted.published).toBe(false)
 expect(await shelf.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 if (process.env.REET_CAPTURE) await shelf.page.screenshot({path:'/workspace/work/skills-phone.png',fullPage:true})
 expect(shelf.errors).toEqual([])
 await shelf.context.close()
 console.log('Simplified sync, persistent machine toggle, runtime progress, collapsed prompt and hosted sharing pass on desktop/phone.')
} finally { await browser.close() }
