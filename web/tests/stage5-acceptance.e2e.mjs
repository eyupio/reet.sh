import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'

// Stage 5 acceptance in a real browser against the real controller and two real connector processes. It is started by
// TestStage5BrowserAcceptance (internal/api, build tag sync_contract, REET_STAGE5_BROWSER=1), which provides the signed-in
// session, the controller's origin and a control channel to run connector cycles and change files on a machine.
// Nothing here is mocked: every state asserted comes from the controller and the machines' own files.
const base=process.env.REET_UI_URL, session=process.env.REET_ACCEPT_SESSION, control=process.env.REET_ACCEPT_CONTROL
if(!base||!session||!control)throw new Error('run this through TestStage5BrowserAcceptance')
const shots=process.env.REET_UI_SCREENSHOTS
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const names={review:'reet-review-helper',api:'reet-api-design',notes:'reet-release-notes'}
const ctl=async(path,query={})=>{
 const response=await fetch(`${control}${path}?${new URLSearchParams(query)}`,{method:'POST'})
 const text=await response.text()
 if(!response.ok)throw new Error(`${path} failed: ${text}`)
 return text
}
const present=async(machine,name)=>(await ctl('/present',{m:machine,name}))==='true'
const rounds=(machines,n=8,pause=0)=>ctl('/rounds',{m:machines,n,pause})
const log=message=>console.log(`PASS ${message}`)
const signedIn=async(width=1280)=>{
 const context=await browser.newContext({viewport:{width,height:900}})
 await context.addCookies([{name:'reet_session',value:session,url:base}])
 const page=await context.newPage()
 const errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 return {context,page,errors}
}
const shot=async(page,name,{phone=false}={})=>{
 if(!shots)return
 await mkdir(shots,{recursive:true})
 if(phone){const size=page.viewportSize();await page.setViewportSize({width:390,height:812});await page.screenshot({path:`${shots}/${name}-phone.png`});await page.setViewportSize(size)}
 await page.screenshot({path:`${shots}/${name}-desktop.png`})
}
const item=(page,name)=>page.locator('.item',{has:page.getByRole('heading',{name,exact:true})})
const openPicker=async(page,machine)=>{
 await page.goto(`${base}/machines`)
 const card=page.getByRole('article',{name:machine,exact:true})
 await card.getByRole('button',{name:'Choose skills',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:`Skills on ${machine}`,exact:true})
 const panel=dialog.getByRole('region',{name:'Skills to sync'})
 await expect(panel.getByLabel('Machine folder')).not.toHaveValue('')
 return {dialog,panel}
}
const stateOf=(panel,name)=>panel.locator('.row',{has:panel.page().getByRole('checkbox',{name})}).locator('[data-item-state]')
let {context,page,errors}=await signedIn()

try {
 // 1. Signed in with a real session: the Library lists every item once, none selected anywhere yet.
 await page.goto(`${base}/library`)
 await expect(page.getByRole('heading',{name:'Library',level:1})).toBeVisible()
 for(const name of Object.values(names)){
  await expect(item(page,name)).toBeVisible()
  await expect(item(page,name).getByLabel(`Status of ${name}`)).toHaveValue('in_use')
  await expect(item(page,name)).toContainText('Not selected on any machine')
 }
 await shot(page,'accept-1-library-before')
 log('signed in; the Library shows each item once, In use, selected nowhere')

 // 2. Both machines are connected. Choose skills on one: select, save, and the rows say they are waiting, not done.
 await page.goto(`${base}/machines`)
 await expect(page.getByRole('article',{name:'workstation',exact:true})).toBeVisible()
 await expect(page.getByRole('article',{name:'laptop',exact:true})).toBeVisible()
 let {dialog,panel}=await openPicker(page,'workstation')
 await panel.getByRole('button',{name:'Select all In use'}).click()
 await panel.getByRole('button',{name:'Save selection'}).click()
 await expect(panel.getByRole('status').filter({hasText:'Selection saved'})).toBeVisible()
 for(const name of Object.values(names)){
  const key=await stateOf(panel,name).getAttribute('data-item-state')
  expect(['waiting','syncing'],`${name} was ${key} right after saving`).toContain(key)
 }
 await expect(panel).not.toContainText(/up to date/i)
 log('selection saved from the machine card; rows say waiting or syncing, never done')

 // 3. The connector syncs by itself (no browser action): real files appear, and the rows now say Installed from the machine's own report.
 await page.keyboard.press('Escape')
 await rounds('workstation',8)
 for(const name of Object.values(names))expect(await present('workstation',name),`${name} on workstation`).toBe(true)
 expect(await present('laptop',names.review),'the unselected machine received nothing').toBe(false)
 ;({dialog,panel}=await openPicker(page,'workstation'))
 for(const name of Object.values(names))await expect(stateOf(panel,name)).toHaveAttribute('data-item-state','verified')
 await shot(page,'accept-3-installed',{phone:true})
 log('files installed on the selected machine only; rows read Installed from its report')

 // 4. The Library says which machine keeps each item.
 await page.goto(`${base}/library`)
 for(const name of Object.values(names))await expect(item(page,name)).toContainText('Selected on workstation')
 log('the Library shows the machine that keeps each item')

 // 5. Status removal from the Library: Disabled removes the managed copy, keeps the library entry and the selection, notifies nobody.
 await item(page,names.review).getByLabel(`Status of ${names.review}`).selectOption('disabled')
 await expect(item(page,names.review)).toContainText('not installed while disabled')
 await rounds('workstation',10)
 expect(await present('workstation',names.review)).toBe(false)
 expect(await ctl('/backups',{m:'workstation',name:names.review}),'the removed copy is recoverable').toBe('1')
 expect(await present('workstation',names.api)).toBe(true)
 ;({dialog,panel}=await openPicker(page,'workstation'))
 await expect(stateOf(panel,names.review)).toHaveAttribute('data-item-state','inactive')
 await expect(stateOf(panel,names.review)).toContainText('Kept in the library only')
 await expect(panel.getByRole('checkbox',{name:names.review})).toBeChecked()
 await expect(stateOf(panel,names.api)).toHaveAttribute('data-item-state','verified')
 await shot(page,'accept-5-status-removal',{phone:true})
 const afterRemoval=await page.evaluate(async()=>(await (await fetch('/api/v2/notifications')).json()).notifications.length)
 expect(afterRemoval,'an authorised removal produced a notification').toBe(0)
 log('Disabled removed the managed copy, kept the entry and selection, and notified nobody')

 // 6. In use again restores the latest revision to the retained selection.
 await page.keyboard.press('Escape')
 await page.goto(`${base}/library`)
 await item(page,names.review).getByLabel(`Status of ${names.review}`).selectOption('in_use')
 await rounds('workstation',10)
 expect(await present('workstation',names.review)).toBe(true)
 log('returning to In use restored the copy with no reselection')

 // 7. A real partial failure: one item's local entry becomes a plain file. It says why beside itself; the others carry on.
 // The controller retries an item at most once a minute (a deliberate back-off), so wait that window out, as a person would.
 await ctl('/breakdir',{m:'workstation',name:names.api})
 await rounds('workstation',2)
 await rounds('workstation',8,62000)
 ;({dialog,panel}=await openPicker(page,'workstation'))
 await expect(stateOf(panel,names.api)).toHaveAttribute('data-item-state','attention')
 await expect(stateOf(panel,names.api)).toContainText('The managed directory cannot be verified; its local files are retained.')
 await expect(stateOf(panel,names.review)).toHaveAttribute('data-item-state','verified')
 await expect(stateOf(panel,names.notes)).toHaveAttribute('data-item-state','verified')
 await shot(page,'accept-7-partial-failure',{phone:true})
 log('a failing item explains itself beside itself while healthy items stay installed')
 await page.keyboard.press('Escape')
 // With the blocker gone the folder is simply missing, which is believed only once it has persisted past the debounce window.
 await ctl('/healdir',{m:'workstation',name:names.api})
 await rounds('workstation',2)
 await rounds('workstation',8,500)
 expect(await present('workstation',names.api)).toBe(true)
 ;({dialog,panel}=await openPicker(page,'workstation'))
 await expect(stateOf(panel,names.api)).toHaveAttribute('data-item-state','verified')
 log('clearing the blocker lets the item sync without any reselection')
 await page.keyboard.press('Escape')

 // 8. Close the browser entirely. The world carries on without it, and a new browser sees the durable result.
 await context.close()
 await ctl('/used',{name:names.notes,value:'false'})
 await rounds('workstation',10)
 expect(await present('workstation',names.notes),'sync continued with no browser open').toBe(false)
 ;({context,page,errors}=await signedIn())
 await page.goto(`${base}/library`)
 // Turned off through the legacy "used" flag while no browser was open: it maps to To test, the authoritative status.
 await expect(item(page,names.notes).getByLabel(`Status of ${names.notes}`)).toHaveValue('to_test')
 await expect(item(page,names.notes)).toContainText('not installed while to test')
 ;({dialog,panel}=await openPicker(page,'workstation'))
 await expect(stateOf(panel,names.notes)).toHaveAttribute('data-item-state','inactive')
 await expect(panel.getByRole('checkbox',{name:names.notes})).toBeChecked()
 await page.keyboard.press('Escape')
 log('closing the browser did not stop sync; reopening shows the durable state')

 // 9. A local deletion is restored and announced exactly once, from the controller's own record.
 await ctl('/deletefile',{m:'workstation',name:names.review,file:'reference.txt'})
 await rounds('workstation',2)
 await rounds('workstation',8,500)
 await page.goto(`${base}/notifications`)
 const restored=page.getByText(`Restored ${names.review} on workstation after it was deleted locally.`)
 await expect(restored).toHaveCount(1)
 await shot(page,'accept-9-restored-notification',{phone:true})
 await rounds('workstation',6)
 await page.reload()
 await expect(restored).toHaveCount(1)
 log('a deleted file was restored and announced exactly once')

 expect(errors).toEqual([])
 await context.close()
} finally { await browser.close() }
