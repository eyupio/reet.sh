import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const scope={id:'offline-shared',host:'Offline laptop',agent:'agents',name:'Shared',skills:[],skillCount:0,complete:false,scannedAt:0,lastSeen:0}
const items=[{id:'a'.repeat(32),name:'selected'.repeat(10).slice(0,64),itemType:'skill',status:'in_use',contentState:'complete'},{id:'b'.repeat(32),name:'Skill to test',itemType:'skill',status:'to_test',contentState:'complete'},{id:'c'.repeat(32),name:'Waiting capture',itemType:'skill',status:'in_use',contentState:'pending_capture'}]
try {
 for (const width of [320,390,1280]) {
  const saved=[]
  const {page,context,errors}=await openWorkspace(browser,{width,touch:width<600,sync:{environments:[scope],machines:[{host:scope.host,groups:[scope],skillCount:0}],agents:[{id:'machine-a',host:scope.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:now-600000}],targets:[],jobs:[],runs:[],updateJobs:[]},routes:{
   '/api/v2/library':{schemaVersion:1,items},
   '/api/v2/library/selections/offline-shared':{status:404,json:{error:'private skill unavailable'}},
   '/api/v2/library/selections':request=>{const body=request.postDataJSON();saved.push(body);return {...body,version:'saved-version'}}
  }})
  await page.goto('http://127.0.0.1:4173/sync')
  const panel=page.getByRole('region',{name:'Skills to sync'})
  await expect(panel).toBeVisible()
  await panel.getByLabel('Machine folder').selectOption(scope.id)
  await expect(panel.getByRole('button',{name:'Save selection'})).toBeVisible()
  await expect(panel.getByRole('checkbox',{name:'Skill to test'})).toBeDisabled()
  await panel.getByRole('button',{name:'Select all In use'}).click()
  await panel.getByRole('button',{name:'Save selection'}).click()
  await expect(panel.getByRole('status')).toContainText('Selection saved')
  expect(saved).toEqual([{targetId:scope.id,itemIds:[items[0].id,items[2].id],version:''}])
  // Intent includes a pending capture; it must not require a complete scan.
  await expect(panel.getByRole('checkbox',{name:'Waiting capture'})).toBeChecked()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
  console.log(`PASS selected library offline intent at ${width}px`)
 }
} finally { await browser.close() }
