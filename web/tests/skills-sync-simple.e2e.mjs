import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const one = skill('review'), two = { ...skill('my-draft'), id:'hosted/reet.example/sam/my-draft', source:'private:bundle:revision:digest', sourceType:'private' }
const scope = { id:'shared', host:'Laptop', agent:'agents', name:'Laptop', skills:[], skillCount:0, complete:true, scannedAt:Date.now(),lastSeen:Date.now() }
const initial = () => ({ environments:[scope],machines:[{host:'Laptop',groups:[scope],skillCount:0}],agents:[{id:'a1',host:'Laptop',version:'test',lastSeen:Date.now(),paused:false,autoUpdate:false,keepSynced:true},{id:'a2',host:'Desktop',lastSeen:Date.now()-300000,paused:false,keepSynced:true},{id:'a3',host:'Build server',lastSeen:Date.now(),paused:true,keepSynced:true}],targets:[{target:scope,sourceId:'library',sourceName:'Shared library',mode:'additive',desired:[one,two],missing:[one,two],extra:[],revision:'r',followSource:true}],jobs:[],runs:[] })
try {
  for (const width of [390,1280]) for (const theme of ['light','dark']) {
    let sync = initial(), refused = false
    const writes = [], collections = []
    const fixture = await openWorkspace(browser,{width,height:width===390?812:960,touch:width===390,theme,skills:[one,two],sync,routes:{
      '/api/v2/sync':()=>sync,
      '/api/v2/sync/candidates':request=>{ collections.push(new URL(request.url()).searchParams.get('collection')); return {skills:[one,two]} },
      '/api/v2/sync/library':request=>{
        writes.push(request.postDataJSON())
        if(refused) return {status:409,json:{error:'The library needs a source conflict resolved.'}}
        sync.jobs=[{id:'new-job',agentId:'a1',targetId:'shared',kind:'apply',status:'running',plan:{},stage:'apply',stages:[{key:'read',label:'Read machine',status:'done'},{key:'plan',label:'Plan changes',status:'done'},{key:'apply',label:'Apply changes',status:'running',detail:'Installing review · 1 of 2'}],createdAt:Date.now(),updatedAt:Date.now()}]
        return sync
      },
    }})
    const {page}=fixture
    await page.goto(`${base}/sync?skill=${encodeURIComponent(two.id)}`)
    await expect(page.getByRole('heading',{name:'Your skills, everywhere.'})).toBeVisible()
    await expect(page.getByRole('button',{name:'Sync all machines',exact:true})).toBeEnabled()
    if(width===390){const button=await page.getByRole('button',{name:'Sync all machines',exact:true}).boundingBox();expect(button.y+button.height).toBeLessThan(812-64)}
    const section=page.getByRole('region',{name:'Your skills, everywhere.'})
    await expect(section.locator('.machine-list').getByText('Desktop',{exact:true})).toBeVisible()
    await expect(section.locator('.machine-list').getByText('Build server',{exact:true})).toBeVisible()
    await expect(section.getByText('Paused',{exact:true})).toBeVisible()
    await section.locator('summary').filter({hasText:'In your library'}).click()
    await expect(section.getByText('my-draft',{exact:true})).toBeVisible()
    await expect(section.getByText('Private · only your machines',{exact:true})).toBeVisible()
    // The page leads with one sentence and one button. Laptop is missing skills, so the sentence says so.
    await expect(section.getByRole('status').filter({hasText:'1 machine needs syncing.'})).toBeVisible()
    // A refused sync says why in words and leaves the button ready to try again.
    refused=true
    await page.getByRole('button',{name:'Sync all machines',exact:true}).click()
    await expect(section.getByRole('alert')).toContainText('The library needs a source conflict resolved.')
    await expect(page.getByRole('button',{name:'Sync all machines',exact:true})).toBeEnabled()
    refused=false
    await page.getByRole('button',{name:'Sync all machines',exact:true}).click()
    await expect(page.getByText('Installing review · 1 of 2')).toBeVisible()
    // No success paragraph to read: the headline says what is happening and why two machines are not part of it.
    const status=section.getByRole('status').filter({hasText:'Syncing your machines'})
    await expect(status).toBeVisible()
    await expect(status).toContainText("Desktop will catch up when it's back online.")
    await expect(status).toContainText('Build server is paused.')
    await expect(section.getByRole('button',{name:'Syncing…',exact:true})).toBeDisabled()
    expect(writes).toEqual([{},{}])
    expect(collections).toContain('library')
    if (process.env.REET_CAPTURE) {
      await mkdir(process.env.REET_CAPTURE,{recursive:true})
      await page.screenshot({path:`${process.env.REET_CAPTURE}/library-sync-${width}-${theme}.png`,fullPage:true})
    }
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
    expect(fixture.errors).toEqual([])
    await fixture.context.close()
  }
  const empty=await openWorkspace(browser,{width:390,routes:{'/api/v2/sync/candidates':{skills:[]}}})
  await empty.page.goto(`${base}/sync`)
  await expect(empty.page.getByText('Start with your first machine.',{exact:true})).toBeVisible()
  await expect(empty.page.getByRole('button',{name:'Sync all machines'})).toHaveCount(0)
  await empty.context.close()
  // The sentence follows the fleet. Nothing behind and a machine off or paused is still "in sync"; with every machine off it only promises
  // to sync later; with every machine paused there is nothing the button could do, so it is not offered.
  const states=[
    ['in sync',state=>{state.targets[0]={...state.targets[0],missing:[],appliedRevision:'r',updatedAt:Date.now()-600000}},'Everything is in sync.',["Desktop will catch up when it's back online.",'Build server is paused.'],true],
    ['all off',state=>{state.agents[0].lastSeen=Date.now()-300000},"Your machines will sync when they're back online.",['Offline right now: Laptop and Desktop.'],true],
    ['all paused',state=>{for(const agent of state.agents)agent.paused=true},'Sync is paused on all your machines.',[],false],
  ]
  for(const [name,change,headline,details,button] of states){
    const world=initial();change(world)
    const fleet=await openWorkspace(browser,{width:1280,sync:world,routes:{'/api/v2/sync/candidates':{skills:[one,two]}}})
    await fleet.page.goto(`${base}/sync`)
    const status=fleet.page.getByRole('region',{name:'Your skills, everywhere.'}).getByRole('status').filter({hasText:headline})
    await expect(status,name).toBeVisible()
    for(const detail of details)await expect(status,name).toContainText(detail)
    await expect(fleet.page.getByRole('button',{name:'Sync all machines',exact:true}),name).toHaveCount(button?1:0)
    // Pressing Sync again when everything is current is only ever a check, so it is not the loud button.
    if(name==='in sync')await expect(fleet.page.getByRole('button',{name:'Sync all machines',exact:true})).not.toHaveClass(/primary/)
    expect(fleet.errors).toEqual([])
    await fleet.context.close()
  }
  // The Sync page is one status sentence, a row per machine and recent activity. Everything a person has to act on
  // is said on that machine's row; the sync-plan controls are one link away, on Advanced sync.
  for (const width of [390,1280]) {
    const world=initial()
    world.targets[0]={...world.targets[0],missing:[],appliedRevision:'r',updatedAt:Date.now()-600000}
    const job=(id,agentId,kind,status,ago,plan={})=>({id,agentId,targetId:kind==='update'?'':'shared',kind,status,revision:'r',hash:'h',plan,message:'',stage:'',stages:[],createdAt:Date.now()-ago,updatedAt:Date.now()-ago})
    // Laptop has a plan waiting for review; Desktop finished an update a few minutes ago.
    world.jobs=[job('review-1','a1','preview','awaiting-approval',60000,{directory:'~/.agents/skills',actions:[{add:false,skill:one}]}),job('update-1','a2','update','succeeded',180000)]
    const plain=await openWorkspace(browser,{width,height:width===390?844:900,touch:width===390,sync:world,routes:{'/api/v2/sync':()=>world,'/api/v2/sync/candidates':{skills:[one,two]},'/api/v2/lists':{lists:[]}}})
    const p=plain.page
    await p.goto(`${base}/sync`)
    await expect(p.getByRole('heading',{level:1})).toHaveCount(1)
    await expect(p.getByRole('heading',{level:1,name:'Sync',exact:true})).toBeVisible()
    expect(await p.title()).toBe('Sync · reet.sh')
    const summary=p.getByRole('region',{name:'Your skills, everywhere.'})
    await expect(summary.getByRole('status')).toContainText('1 machine needs a look. Laptop: 1 planned change needs your approval before applying.')
    // Exactly one status sentence, and none of the headlines the activity block and the plan controls used to add.
    await expect(p.locator('main [role="status"]:visible').filter({hasText:/\S/}),`one status sentence at ${width}px`).toHaveCount(1)
    for(const gone of ['Sync has unresolved changes','Sync is up to date','Nothing needs you.','No skills set to sync','plan needs review','No sync activity yet'])await expect(p.locator('main').getByText(gone,{exact:false}),gone).toHaveCount(0)
    // No plan controls and no second skills picker.
    for(const label of ['Source','Sync mode','List','Machine'])await expect(p.getByLabel(label,{exact:true}),label).toHaveCount(0)
    await expect(p.getByText(/sync plan · revision|Plan revision (not yet )?acknowledged/i)).toHaveCount(0)
    await expect(p.getByRole('heading',{name:'Sync plans',exact:true})).toHaveCount(0)
    await expect(p.getByRole('heading',{name:'Choose what to sync',exact:true})).toHaveCount(0)
    await expect(p.getByRole('button',{name:'Scan all',exact:true})).toHaveCount(0)
    await expect(p.getByRole('region',{name:'Skills to sync'})).toHaveCount(0)
    await expect(p.getByText(/sha256/i)).toHaveCount(0)
    // What needs a person, and what just finished, is on the machine's own row.
    const laptop=summary.locator('.machine-list article',{hasText:'Laptop'})
    await expect(laptop).toContainText('Review needed')
    await expect(laptop).toContainText('1 plan is waiting for your review.')
    await expect(laptop.getByRole('link',{name:'Review changes',exact:true})).toHaveAttribute('href','#sync-jobs')
    await expect(summary.locator('.machine-list article',{hasText:'Desktop'})).toContainText('Update complete · 3 min ago')
    // Recent activity is a log with a plain heading; the review is done there.
    await expect(p.locator('#sync-jobs-title')).toHaveText('Recent activity')
    await laptop.getByRole('link',{name:'Review changes',exact:true}).click()
    await expect(p.locator('#sync-jobs')).toBeInViewport()
    await expect(p.locator('#sync-jobs').getByRole('button',{name:'Apply changes',exact:true})).toBeVisible()
    // One plain link to the plan controls, with a line saying what they are for.
    const advanced=p.getByRole('link',{name:'Advanced sync',exact:true})
    await expect(advanced).toHaveCount(1)
    await expect(advanced).toHaveAttribute('href','/sync/advanced')
    await expect(summary.locator('.advanced')).toContainText('Sync plans that follow a list or keep an exact set of skills, plan revisions and manual scans.')
    expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`no sideways scroll on Sync at ${width}px`).toBe(true)
    await advanced.click()
    // Advanced sync: its own page, with the plan controls as they were, and a way back.
    await expect(p).toHaveURL(`${base}/sync/advanced`)
    await expect(p.getByRole('heading',{level:1})).toHaveCount(1)
    await expect(p.getByRole('heading',{level:1,name:'Advanced sync',exact:true})).toBeVisible()
    expect(await p.title()).toBe('Advanced sync · reet.sh')
    await expect(p.getByText('plans that follow a list or keep an exact set of skills',{exact:false}).first()).toBeVisible()
    await expect(p.getByRole('link',{name:'Back to Sync',exact:true})).toHaveAttribute('href','/sync')
    await expect(p.getByRole('heading',{name:'Your skills, everywhere.'})).toHaveCount(0)
    await expect(p.getByRole('heading',{name:'Choose what to sync',exact:true})).toBeVisible()
    await expect(p.getByLabel('Source',{exact:true})).toHaveValue('shelf')
    await expect(p.getByLabel('Sync mode',{exact:true}).locator('option')).toHaveText(['Additive','Bidirectional review','Exact set'])
    await expect(p.getByRole('heading',{name:'Sync plans',exact:true})).toBeVisible()
    await expect(p.getByRole('button',{name:'Scan all',exact:true})).toBeVisible()
    await expect(p.locator('#sync-jobs-title')).toHaveText('1 plan needs review')
    expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`no sideways scroll on Advanced sync at ${width}px`).toBe(true)
    await p.getByRole('link',{name:'Back to Sync',exact:true}).click()
    await expect(p).toHaveURL(`${base}/sync`)
    // An older link that names a machine for the plan controls lands on them, with the machine chosen.
    await p.goto(`${base}/sync?machine=Laptop`)
    await expect(p).toHaveURL(`${base}/sync/advanced?machine=Laptop`)
    await expect(p.getByLabel('Machine',{exact:true})).toHaveValue('Laptop')
    await expect(p.getByText('Install locations · Laptop',{exact:true})).toBeVisible()
    await p.goto(`${base}/sync#sync-selection`)
    await expect(p).toHaveURL(`${base}/sync/advanced#sync-selection`)
    await expect(p.locator('#sync-selection')).toBeInViewport()
    // A parameter no Sync page reads is no reason to leave the Sync page.
    await p.goto(`${base}/sync?skill=${encodeURIComponent(two.id)}`)
    await expect(p).toHaveURL(`${base}/sync?skill=${encodeURIComponent(two.id)}`)
    await expect(p.getByRole('heading',{level:1,name:'Sync',exact:true})).toBeVisible()
    expect(plain.errors).toEqual([])
    await plain.context.close()
  }
  // A read-only account and an instance without accounts get both pages, without controls that write.
  for (const [name,status] of [['viewer',{user:{id:2,name:'Reader',role:'viewer'}}],['open instance',{mode:'none',user:null}]]) {
    const world=initial()
    const reader=await openWorkspace(browser,{width:1280,status,sync:world,routes:{'/api/v2/sync/candidates':{skills:[one,two]},'/api/v2/lists':{lists:[]}}})
    const p=reader.page
    await p.goto(`${base}/sync`)
    await expect(p.getByRole('heading',{level:1,name:'Sync',exact:true}),name).toBeVisible()
    await expect(p.locator('main [role="status"]:visible').filter({hasText:/\S/}),name).toHaveCount(1)
    await expect(p.getByRole('link',{name:'Advanced sync',exact:true}),name).toHaveCount(1)
    await p.goto(`${base}/sync/advanced?machine=Laptop`)
    await expect(p.getByRole('heading',{level:1,name:'Advanced sync',exact:true}),name).toBeVisible()
    await expect(p.getByLabel('Machine',{exact:true}),name).toHaveValue('Laptop')
    await expect(p.getByRole('heading',{name:'Sync plans',exact:true}),name).toBeVisible()
    if(name==='viewer'){
      await expect(p.getByLabel('Sync mode',{exact:true})).toBeDisabled()
      for(const button of ['Save sync plan (no files change yet)','Clear this sync plan','Scan all','Sync all · preview','Sync now','Add selected to reet.sh'])await expect(p.getByRole('button',{name:button,exact:true}),button).toHaveCount(0)
    }
    await reader.context.close()
  }
  const viewer=await openWorkspace(browser,{width:1280,status:{user:{id:2,name:'Reader',role:'viewer'}},sync:initial(),routes:{'/api/v2/sync/candidates':{skills:[one,two]}}})
  await viewer.page.goto(`${base}/sync`)
  // A read-only account sees the answer but is not offered a button it cannot use.
  await expect(viewer.page.getByRole('status').filter({hasText:'1 machine needs syncing.'})).toBeVisible()
  await expect(viewer.page.getByRole('button',{name:'Sync all machines'})).toHaveCount(0)
  await viewer.context.close()
  console.log('Shared-library sync passed on phone/desktop in light/dark: drafts, offline/paused machines, API failures, empty fleet and viewer access.')
} finally { await browser.close() }
