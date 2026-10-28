import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const page=await browser.newPage({viewport:{width:1360,height:1000}})
const now=Date.now(),errors=[],writes=[]
page.on('pageerror',e=>errors.push(e.message))
let viewer=false,active=null,bundles=[],sequence=0,loseApproval=true
const skill={id:'demo/repo/demo',name:'demo',slug:'demo',source:'demo/repo',sourceType:'github',description:'Test skill',notes:'',used:true}
const scope=(id,host)=>({id,host,name:host,agent:'agents',project:'',complete:true,scannedAt:now,lastSeen:now,skills:[skill],skillCount:1,content:{state:'observed',skills:[{name:'demo',complete:true,files:3,bytes:92,digest:'d'.repeat(64)}]}})
const envs=[scope('scope-one','workstation'),scope('scope-old','old-connector')]
const snapshot={environments:envs,machines:envs.map(e=>({host:e.host,groups:[e],skillCount:1})),agents:envs.map((e,i)=>({id:`agent-${i}`,host:e.host,version:'dev',paused:false,lastSeen:now})),targets:[],jobs:[]}
const preview={schemaVersion:1,sourceDigest:'d'.repeat(64),skillValid:true,files:[{path:'SKILL.md',bytes:45,digest:'a'.repeat(64),executable:false},{path:'.env',bytes:32,digest:'b'.repeat(64),executable:false,blocked:'credential_path'},{path:'run.sh',bytes:15,digest:'c'.repeat(64),executable:true}]}
await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname,method=req.method(),body=method==='GET'?null:req.postDataJSON()
 let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Operator',email:'operator@example.test',role:viewer?'viewer':'owner'}}
 else if(path==='/api/v2/profile')data={profile:{theme:'system',shelfView:'list'}}
 else if(path==='/api/skills')data={skills:[skill],counts:{all:1,used:1}}
 else if(path==='/api/environments')data={environments:envs,machines:snapshot.machines}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data=snapshot
 else if(path==='/api/v2/machines/health')data={serverNow:now,machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines')data={machines:[]}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'dev',commit:'fixture'}
 else if(path==='/api/v2/skill-bundles')data={bundles}
 else if(path.match(/^\/api\/v2\/skill-bundles\/[^/]+\/revisions$/))data={revisions:[{id:'revision-first',createdAt:now,manifestDigest:'f'.repeat(64),origin:{kind:'import'}}]}
 else if(path==='/api/v2/skill-imports'&&method==='GET')data={requests:active?[{...active,preview:undefined}]:[],capabilities:[{agentId:'agent-0',ready:true,observedAt:now},{agentId:'agent-1',ready:false,observedAt:0}]}
 else if(path==='/api/v2/skill-imports'&&method==='POST'){
  writes.push({path,body});active={...body,id:`request-${++sequence}`,host:'workstation',agent:'agents',project:'',state:'queued',createdAt:now,updatedAt:now,expiresAt:now+600000,excluded:[]};data=active
 }
 else if(path===`/api/v2/skill-imports/${active?.id}/cancel`){ writes.push({path,body});if(active.state!=='completed')active={...active,state:'cancelled'};data=active }
 else if(path===`/api/v2/skill-imports/${active?.id}/approve`){
  writes.push({path,body});expect(body).toEqual({previewHash:'preview-hash',excluded:['.env']});active={...active,state:'approved',excluded:body.excluded,digest:'f'.repeat(64)}
  if(loseApproval){loseApproval=false;await route.abort('failed');return};data=active
 }
 else if(path===`/api/v2/skill-imports/${active?.id}`){
  if(method==='DELETE'){writes.push({path,method});active=null;data={ok:true}}else data=active
 }
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
try{
 await page.goto(`${base}/machines`)
 await page.getByRole('button',{name:'Import a machine skill…',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await expect(dialog.getByRole('heading',{name:'Bring a machine skill into reet'})).toBeVisible()
 expect(writes).toEqual([])
 await page.getByLabel('Source scope',{exact:true}).selectOption('scope-old')
 await page.getByLabel('Skill folder',{exact:true}).selectOption('demo')
 await expect(page.getByRole('button',{name:'Request file preview'})).toBeDisabled()
 await expect(dialog).toContainText('has not recently negotiated browser import')
 await page.getByLabel('Source scope',{exact:true}).selectOption('scope-one')
 await page.getByLabel('Skill folder',{exact:true}).selectOption('demo')
 await page.getByRole('button',{name:'Request file preview'}).click()
 await expect(dialog).toContainText('Waiting for the machine')
 expect(writes).toHaveLength(1)
 expect(writes[0].body).toMatchObject({agentId:'agent-0',environmentId:'scope-one',name:'demo',bundleId:'',expectedBase:''})
 expect(writes[0].body.files).toBeUndefined()
 active={...active,state:'reviewing',preview,previewHash:'preview-hash'}
 await dialog.getByRole('button',{name:'Refresh status'}).click()
 await expect(page.getByLabel('Include SKILL.md')).toBeDisabled()
 await expect(page.getByRole('button',{name:'Import 3 files privately'})).toBeDisabled()
 await page.getByLabel('Include .env',{exact:true}).uncheck()
 await dialog.getByRole('button',{name:'Refresh status'}).click()
 await expect(page.getByLabel('Include .env',{exact:true})).not.toBeChecked()
 await expect(page.getByRole('button',{name:'Import 2 files privately'})).toBeEnabled()
 await layout()
 await page.setViewportSize({width:390,height:900});await page.emulateMedia({colorScheme:'dark'});await layout()
 if(process.env.REET_IMPORT_SCREENSHOT)await page.screenshot({path:process.env.REET_IMPORT_SCREENSHOT,fullPage:false})
 await page.setViewportSize({width:1360,height:1000});await page.emulateMedia({colorScheme:'light'})
 await page.getByRole('button',{name:'Import 2 files privately'}).click()
 await expect.poll(()=>writes.length).toBe(2)
 await dialog.getByRole('button',{name:'Refresh status'}).click()
 await expect(dialog).toContainText('Waiting for the machine to recheck')
 expect(writes).toHaveLength(2)
 bundles=[{id:'bundle-first',name:'demo',head:'revision-first',createdAt:now}]
 active={...active,state:'completed',receipt:{bundle:bundles[0],revision:{id:'revision-first',manifestDigest:'f'.repeat(64)}}}
 await dialog.getByRole('button',{name:'Refresh status'}).click()
 await expect(dialog).toContainText('Private copy saved: demo')
 await dialog.getByRole('button',{name:'demo',exact:true}).click()
 await expect(dialog.getByRole('listitem').filter({hasText:'revision-first'}).last()).toBeVisible()
 for(const width of [390,320]){await page.setViewportSize({width,height:900});await page.emulateMedia({colorScheme:'dark'});await layout();await expect(page.getByRole('button',{name:'Close skill imports'})).toBeVisible()}
 await page.getByRole('button',{name:'Dismiss import history'}).click()
 expect(bundles).toHaveLength(1)
 // Completion can win a cancellation race. Preserve its receipt rather than
 // reporting cancellation or silently deleting the completed history.
 await dialog.getByRole('button',{name:'Request file preview'}).click()
 await expect(dialog.getByRole('button',{name:'Cancel import'})).toBeVisible()
 const second={id:'bundle-second',name:'demo',head:'revision-second',createdAt:now};bundles.push(second)
 active={...active,state:'completed',receipt:{bundle:second,revision:{id:'revision-second',manifestDigest:'e'.repeat(64)}}}
 await dialog.getByRole('button',{name:'Cancel import'}).click()
 await expect(dialog).toContainText('revision-second')
 await expect(dialog).toContainText('Saved privately in reet.')
 expect(writes.at(-1).path).toMatch(/\/cancel$/)
 await dialog.getByRole('button',{name:'Dismiss import history'}).click()
 expect(bundles).toHaveLength(2)
 await page.getByRole('button',{name:'Close skill imports'}).click()
 viewer=true;await page.reload()
 await expect(page.getByRole('button',{name:'Import a machine skill…',exact:true})).toHaveCount(0)
 expect(errors).toEqual([])
 console.log('Machine import UI passed: metadata-only preview, old connectors, explicit exclusions/approval, uncertain-response reconciliation, private receipts/history, responsive layouts and viewer isolation.')
}finally{await browser.close()}
