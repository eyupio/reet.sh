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
  const viewer=await openWorkspace(browser,{width:1280,status:{user:{id:2,name:'Reader',role:'viewer'}},sync:initial(),routes:{'/api/v2/sync/candidates':{skills:[one,two]}}})
  await viewer.page.goto(`${base}/sync`)
  // A read-only account sees the answer but is not offered a button it cannot use.
  await expect(viewer.page.getByRole('status').filter({hasText:'1 machine needs syncing.'})).toBeVisible()
  await expect(viewer.page.getByRole('button',{name:'Sync all machines'})).toHaveCount(0)
  await viewer.context.close()
  console.log('Shared-library sync passed on phone/desktop in light/dark: drafts, offline/paused machines, API failures, empty fleet and viewer access.')
} finally { await browser.close() }
