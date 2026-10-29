import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, machineWith } from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
try {
 for(const width of [390,1280]) {
  const local={...skill('improve'),id:'local/improve',source:'',sourceType:'local',url:'',used:true}
  const source=machineWith('Elitebook',[local]),dest=machineWith('PROD_Snicket',[])
  const inventory={environments:[...source.environments,...dest.environments],machines:[...source.machines,...dest.machines]}
  const sync={...inventory,agents:['Elitebook','PROD_Snicket'].map((host,i)=>({id:`a${i}`,host,lastSeen:Date.now(),paused:false,keepSynced:true})),targets:[],jobs:[],runs:[]}
  let request=null
  const writes=[]
  const fixture=await openWorkspace(browser,{width,height:900,touch:width===390,skills:[local],sync,inventory,routes:{
   '/api/v2/sync/candidates':{skills:[local]},
   '/api/ignored':{skills:[]},
   '/api/v2/skill-imports':()=>({requests:request?[request]:[],capabilities:[]}),
   '/api/v2/sync/from-machine':req=>{const body=req.postDataJSON();writes.push(body);request={id:'copy1',syncSkillId:local.id,destinationId:dest.environments[0].id,environmentId:source.environments[0].id,host:'Elitebook',agent:'agents',name:'improve',state:'previewing',createdAt:Date.now(),updatedAt:Date.now()};return request},
   '/api/v2/skill-imports/copy1':()=>request,
  }})
  const {page}=fixture
  await page.goto(`${base}/skills`)
  await expect(page.getByText('Available from your machines',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'Link source to sync',exact:true})).not.toBeVisible()
  await page.getByRole('button',{name:/Install improve on PROD_Snicket/}).click()
  const dialog=page.getByRole('dialog',{name:'Sync improve',exact:true})
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('heading',{name:'Reading improve from Elitebook'})).toBeVisible()
  expect(writes).toHaveLength(1)
  expect(writes[0]).toMatchObject({skillId:'local/improve',destination:'PROD_Snicket',sourceId:source.environments[0].id})
  request={...request,state:'completed',syncRunId:'transfer_copy1'}
  sync.runs=[{id:'transfer_copy1',status:'running',targets:[request.destinationId]}]
  sync.jobs=[{id:'install1',targetId:request.destinationId,status:'running',message:'Installing improve · 1 of 1',createdAt:Date.now()}]
  await expect(dialog.getByRole('heading',{name:'Installing on PROD_Snicket'})).toBeVisible({timeout:7000})
  await expect(dialog.getByRole('status')).toHaveText('Installing improve · 1 of 1')
  sync.runs[0].status='completed'
  await expect(dialog.getByRole('heading',{name:'improve is on PROD_Snicket'})).toBeVisible({timeout:7000})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(fixture.errors).toEqual([])
  expect(fixture.state.unexpected).toEqual([])
  await fixture.context.close()
 }
 console.log('Machine-only skills can sync through a private copy, with live transfer/install progress on desktop and phone.')
} finally {await browser.close()}
