import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'

const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
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
  await page.goto(`${base}/sync`)
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
 for (const width of [320,390,1280]) for (const theme of ['light','dark']) {
  const saved=[]
  let selection={targetId:scope.id,itemIds:[],version:''}
  let refuse=false
  let libraryFailure=width===320&&theme==='light'
  const other={...scope,id:'other-machine',host:'Office desktop'}
  const multiple=width===1280&&theme==='dark'
  const ownFolders=multiple ? [scope,{...scope,id:'offline-project',project:'/work/project'}] : [scope]
  const {page,context,errors,state}=await openWorkspace(browser,{width,theme,touch:width<600,sync:{environments:[...ownFolders,other],machines:[{host:scope.host,groups:ownFolders,skillCount:0},{host:other.host,groups:[other],skillCount:0}],agents:[{id:'machine-a',host:scope.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:now-600000},{id:'machine-b',host:other.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:now}],targets:[],jobs:[],runs:[],updateJobs:[]},routes:{
   '/api/v2/library':()=>libraryFailure ? {status:500,json:{error:'Library could not be loaded'}} : {schemaVersion:1,items},
   '/api/v2/library/selections/offline-shared':()=>selection,
   '/api/v2/library/selections':request=>{
    if(refuse)return {status:500,json:{error:'Selection could not be saved'}}
    const body=request.postDataJSON();saved.push(body);selection={...body,version:'saved-version'};return selection
   }
  }})
  await page.goto(`${base}/machines`)
  const opener=page.getByRole('article',{name:scope.host,exact:true}).getByRole('button',{name:'Skills on this machine',exact:true})
  await opener.click()
  const dialog=page.getByRole('dialog',{name:`Skills on ${scope.host}`,exact:true})
  const panel=dialog.getByRole('region',{name:'Skills to sync'})
  if(libraryFailure){
   await expect(panel.getByRole('alert')).toContainText('Library could not be loaded')
   libraryFailure=false
   await panel.getByRole('button',{name:'Try again',exact:true}).click()
  }
  if(multiple){
   // With several folders the first one is chosen to start with; the choice stays a person's to change.
   await expect(panel.getByLabel('Machine folder')).toHaveValue(scope.id)
  }
  await expect(panel).toContainText('Each AI tool on this machine keeps its skills in its own folder. Pick the folder, then tick the skills it should have.')
  await expect(panel.getByLabel('Machine folder')).toHaveValue(scope.id)
  await expect(panel.getByLabel('Machine folder').locator('option')).toHaveCount(multiple ? 3 : 2)
  await expect(panel.getByRole('checkbox',{name:'Skill to test'})).toBeDisabled()
  await panel.getByRole('button',{name:'Select all In use'}).click()
  refuse=true
  await panel.getByRole('button',{name:'Save selection'}).click()
  await expect(panel.getByRole('alert')).toContainText('Selection could not be saved')
  if(process.env.REET_UI_SCREENSHOTS&&width===390&&theme==='dark'){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-library-save-error.png`})}
  await expect(panel.getByRole('status')).toHaveCount(0)
  expect(saved).toEqual([])
  refuse=false
  await panel.getByRole('button',{name:'Save selection'}).click()
  await expect(panel.getByRole('status')).toContainText('Selection saved')
  expect(saved).toEqual([{targetId:scope.id,itemIds:[items[0].id,items[2].id],version:''}])
  expect(new URL(page.url()).pathname).toBe('/machines')
  await expect(panel).not.toContainText('Up to date')
  if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-library-${width}-${theme}.png`})}
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()
  await opener.press('Enter')
  if(multiple)await panel.getByLabel('Machine folder').selectOption(scope.id)
  await expect(panel.getByRole('checkbox',{name:'Waiting capture'})).toBeChecked()
  await panel.getByRole('checkbox',{name:'Waiting capture'}).focus()
  await page.keyboard.press('Space')
  await expect(panel.getByRole('checkbox',{name:'Waiting capture'})).not.toBeChecked()
  await panel.getByLabel('Find a skill').fill('does not exist')
  await expect(panel).toContainText('No skills match your search.')
  await panel.getByLabel('Find a skill').fill('Waiting')
  await expect(panel.getByRole('checkbox')).toHaveCount(1)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  const bounds=await dialog.boundingBox()
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x+bounds.width).toBeLessThanOrEqual(width+1)
  // Reopening in the same task as a native close must keep the new dialog: the old dialog's close event arrives later.
  await opener.evaluate(button=>{document.querySelector('dialog[open]').close();button.click()})
  await expect(dialog).toBeVisible()
  await page.waitForTimeout(100)
  await expect(dialog).toBeVisible()
  expect(errors).toEqual([])
  // No scan, comparison, preview, approval or machine command is needed to save offline intent.
  expect(state.unexpected.filter(path=>/sync\/|terminal\/|skill-import/.test(path))).toEqual([])
  await context.close()
  console.log(`PASS machine-scoped offline selection/retry/focus at ${width}px ${theme}`)
 }
} finally { await browser.close() }
