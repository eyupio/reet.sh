import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'

// The machine skills dialog follows the live machine data. A machine with a single reported folder needs no choice, so that
// folder is chosen for it and the library is offered at once. A folder that a later scan reports while the dialog is open is
// added to the list without closing it, and never moves the choice already made. (A machine with no reported folder at all is
// not drawn as a card, so a picker is only ever opened after the machine's first scan.)
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const id=ch=>ch.repeat(32)
const library=[{id:id('1'),name:'code-review',itemType:'skill',status:'in_use',contentState:'complete'},{id:id('2'),name:'api-design',itemType:'skill',status:'in_use',contentState:'complete'},{id:id('3'),name:'draft-notes',itemType:'skill',status:'to_test',contentState:'complete'}]
const folder=(n,agent='agents')=>({id:`new-box-${agent}`,host:'new-box',agent,name:agent==='claude'?'Claude Code':'Shared',skills:[],skillCount:0,complete:true,scannedAt:Date.now(),lastSeen:Date.now()})

for(const [width,theme] of [[390,'dark'],[1280,'light']]){
 const world={folders:[folder('shared')]}
 const saved=[]
 const {page,context,errors}=await openWorkspace(browser,{width,theme,touch:width<600,
  sync:{
   get environments(){return world.folders},
   get machines(){return [{host:'new-box',groups:world.folders,skillCount:0}]},
   agents:[{id:'agent-new',host:'new-box',paused:false,autoUpdate:false,keepSynced:true,lastSeen:Date.now()}],
   targets:[],jobs:[],runs:[],updateJobs:[]
  },
  routes:{
   '/api/v2/library':{schemaVersion:1,items:library},
   '/api/v2/library/selections/new-box-agents':{status:404,json:{error:'private skill unavailable'}},
   '/api/v2/library/selections':request=>{const body=request.postDataJSON();saved.push(body);return {...body,version:'v1'}},
  }})
 await page.goto(`${base}/machines`)
 const opener=page.getByRole('article',{name:'new-box',exact:true}).getByRole('button',{name:'Skills on this machine',exact:true})
 await opener.click()
 const dialog=page.getByRole('dialog',{name:'Skills on new-box',exact:true})
 const panel=dialog.getByRole('region',{name:'Skills to sync'})
 const folderSelect=panel.getByLabel('Machine folder')
 // One folder: chosen for the person, library offered, nothing to pick first.
 await expect(folderSelect).toHaveValue('new-box-agents')
 await expect(panel.getByRole('checkbox',{name:'code-review'})).toBeVisible()
 await expect(folderSelect.locator('option')).toHaveCount(2)
 await panel.getByRole('button',{name:'Select all In use'}).click()
 await panel.getByRole('button',{name:'Save selection'}).click()
 await expect(panel.getByRole('status')).toContainText('Selection saved')
 expect(saved).toEqual([{targetId:'new-box-agents',itemIds:[id('1'),id('2')],version:''}])

 // A later scan reports a second folder while the dialog is open: it joins the list, the choice stays, and the page behind is untouched.
 world.folders=[folder('shared'),folder('claude','claude')]
 // The page behind a modal is inert to the pointer, so refresh the way the page's own timer does.
 await page.getByRole('button',{name:'Refresh status'}).evaluate(button=>button.click())
 await expect(folderSelect.locator('option')).toHaveCount(3)
 await expect(folderSelect).toHaveValue('new-box-agents')
 await expect(panel.getByRole('checkbox',{name:'code-review'})).toBeChecked()
 await expect(dialog).toBeVisible()
 if(process.env.REET_UI_SCREENSHOTS&&width===390){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/connect-flow-phone-live-folders.png`})}
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(errors.filter(message=>!/404/.test(message))).toEqual([])
 await context.close()
 console.log(`PASS live folders in the machine skills dialog at ${width}px ${theme}`)
}

// The import step: what the machine reports that the library lacks is offered right in the picker, each skill leading into the
// private import with its source already chosen, saved without a separate review (see the direct-import block below). Skills the library has are not offered, and a skill whose files could
// not be read completely is explained, not offered. Nothing is offered until the machine's content has actually been observed.
const withContent=(state,skills)=>({...folder('shared'),content:{state,skills}})
const seen=(name,changes={})=>({name,complete:true,files:1,bytes:10,...changes})
for(const [width,theme] of [[390,'dark'],[1280,'light']]){
 const world={folders:[withContent('observed',[seen('code-review'),seen('local-helper'),seen('half-copied',{complete:false})])]}
 const {page,context,errors}=await openWorkspace(browser,{width,theme,touch:width<600,
  sync:{
   get environments(){return world.folders},
   get machines(){return [{host:'new-box',groups:world.folders,skillCount:0}]},
   agents:[{id:'agent-new',host:'new-box',paused:false,autoUpdate:false,keepSynced:true,lastSeen:Date.now()}],
   targets:[],jobs:[],runs:[],updateJobs:[]
  },
  routes:{
   '/api/v2/library':{schemaVersion:1,items:library},
   '/api/v2/library/selections/new-box-agents':{status:404,json:{error:'private skill unavailable'}},
  }})
 await page.goto(`${base}/machines`)
 const opener=page.getByRole('article',{name:'new-box',exact:true}).getByRole('button',{name:'Skills on this machine',exact:true})
 await opener.click()
 const dialog=page.getByRole('dialog',{name:'Skills on new-box',exact:true})
 const found=dialog.getByRole('region',{name:'Found on this machine, not in your library'})
 await expect(found).toBeVisible()
 await expect(found.getByRole('button',{name:'Import local-helper privately'})).toBeVisible()
 // In the library already, so not offered; incomplete, so explained and not offered.
 await expect(found).not.toContainText('code-review')
 await expect(found.getByRole('button',{name:/half-copied/})).toHaveCount(0)
 await expect(found).toContainText('1 more skill here cannot be imported yet')
 await expect(found).toContainText('saved without a separate review')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 if(process.env.REET_UI_SCREENSHOTS&&width===390){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await found.scrollIntoViewIfNeeded();await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/connect-flow-phone-import-step.png`})}

 // By keyboard: the picker closes and the existing reviewed import opens with the folder and skill already chosen.
 await found.getByRole('button',{name:'Import local-helper privately'}).focus()
 await page.keyboard.press('Enter')
 await expect(dialog).toHaveCount(0)
 const imports=page.getByRole('dialog',{name:'Bring a machine skill into reet'})
 await expect(imports).toBeVisible()
 await expect(imports.getByLabel('Source install location')).toHaveValue('new-box-agents')
 await expect(imports.getByLabel('Skill folder')).toHaveValue('local-helper')
 // This machine has not negotiated browser import, so nothing can start by itself: the ordinary form stays, and a preview is the only action.
 await expect(imports.getByRole('button',{name:'Request file preview'})).toBeVisible()
 await expect(imports).toContainText('No file contents are uploaded at this step')
 expect(errors).toEqual([])
 await context.close()
 console.log(`PASS import step at ${width}px ${theme}`)
}

// Direct import: choosing a found skill saves it to the library with no stop for review. The preview and the approval are the same
// requests the review makes, made for the person; a blocked file or an invalid SKILL.md still stops for their decision.
for(const [label,files,valid] of [['clean',[{path:'SKILL.md',bytes:10,digest:'a'.repeat(64),executable:false}],true],['blocked file',[{path:'SKILL.md',bytes:10,digest:'a'.repeat(64),executable:false},{path:'.env',bytes:4,digest:'b'.repeat(64),executable:false,blocked:'credential_like'}],true],['invalid skill',[{path:'SKILL.md',bytes:10,digest:'a'.repeat(64),executable:false}],false]]){
 const world={folders:[withContent('observed',[seen('local-helper')])]}
 const calls={preview:0,approve:[]}
 let current=null
 const request=(state,extra={})=>({id:'imp1',requestKey:'k',agentId:'agent-new',environmentId:'new-box-agents',host:'new-box',agent:'agents',project:'',name:'local-helper',state,bundleId:'',expectedBase:'',createdAt:1,updatedAt:1,expiresAt:Date.now()+600000,previewHash:'h1',excluded:files.filter(f=>f.blocked).map(f=>f.path).slice(0,0),preview:{schemaVersion:1,sourceDigest:'c'.repeat(64),skillValid:valid,files},...extra})
 const {page,context,errors}=await openWorkspace(browser,{width:390,touch:true,
  sync:{get environments(){return world.folders},get machines(){return [{host:'new-box',groups:world.folders,skillCount:0}]},agents:[{id:'agent-new',host:'new-box',paused:false,autoUpdate:false,keepSynced:true,lastSeen:Date.now()}],targets:[],jobs:[],runs:[],updateJobs:[]},
  routes:{
   '/api/v2/library':{schemaVersion:1,items:library},
   '/api/v2/library/selections/new-box-agents':{status:404,json:{error:'private skill unavailable'}},
   '/api/v2/skill-bundles':{bundles:[]},
   '/api/v2/skill-imports':req=>req.method()==='POST'?(calls.preview++,current=request('reviewing'),{json:current}):{json:{requests:current?[current]:[],capabilities:[{agentId:'agent-new',ready:true,observedAt:Date.now()}]}},
   '/api/v2/skill-imports/imp1':()=>({json:current}),
   '/api/v2/skill-imports/imp1/approve':req=>{calls.approve.push(req.postDataJSON());current=request('completed',{receipt:{bundle:{id:'bundle-1',name:'local-helper',head:'rev-1'},revision:{id:'rev-1',manifestDigest:'d'.repeat(64)}}});return {json:current}}
  }})
 await page.goto(`${base}/machines`)
 await page.getByRole('article',{name:'new-box',exact:true}).getByRole('button',{name:'Skills on this machine',exact:true}).click()
 await page.getByRole('dialog',{name:'Skills on new-box',exact:true}).getByRole('button',{name:'Import local-helper privately'}).click()
 const imports=page.getByRole('dialog',{name:'Bring a machine skill into reet'})
 if(label==='clean'){
  await expect(imports.getByText('Private copy saved: local-helper')).toBeVisible()
  expect(calls.preview).toBe(1)
  expect(calls.approve).toEqual([{previewHash:'h1',excluded:[]}])
  await expect(imports).toContainText('straight into your library')
 }else{
  // Stops for a person: the review table is shown, and nothing is approved on their behalf.
  await expect(imports.getByRole('table',{name:'Files on the machine'})).toBeVisible()
  await page.waitForTimeout(800)
  expect(calls.approve).toEqual([])
  await expect(imports).toContainText('Review a skill')
 }
 expect(errors).toEqual([])
 await context.close()
 console.log(`PASS direct import: ${label}`)
}

{ // Nothing is offered while the machine's content has not been observed: an unread scan is not evidence a skill is missing.
 const {page,context,errors}=await openWorkspace(browser,{width:390,touch:true,
  sync:{environments:[withContent('pending',[seen('local-helper')])],machines:[{host:'new-box',groups:[withContent('pending',[seen('local-helper')])],skillCount:0}],agents:[{id:'agent-new',host:'new-box',paused:false,autoUpdate:false,keepSynced:true,lastSeen:Date.now()}],targets:[],jobs:[],runs:[],updateJobs:[]},
  routes:{'/api/v2/library':{schemaVersion:1,items:library},'/api/v2/library/selections/new-box-agents':{status:404,json:{error:'private skill unavailable'}}}})
 await page.goto(`${base}/machines`)
 await page.getByRole('article',{name:'new-box',exact:true}).getByRole('button',{name:'Skills on this machine',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'Skills on new-box',exact:true})
 await expect(dialog.getByRole('button',{name:'Save selection'})).toBeVisible()
 await expect(dialog.getByRole('region',{name:'Found on this machine, not in your library'})).toHaveCount(0)
 expect(errors).toEqual([])
 await context.close()
 console.log('PASS nothing offered before the content is observed')
}
await browser.close()
