import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'

// Stage 5 acceptance in a real browser against the real controller and two real connector processes. It is started by
// TestStage5BrowserAcceptance (internal/api, build tag sync_contract, REET_STAGE5_BROWSER=1), which provides the account
// credentials, the controller's origin and a control channel to run connector cycles and change files on a machine.
// Nothing here is mocked: every state asserted comes from the controller and the machines' own files.
const base=process.env.REET_UI_URL, email=process.env.REET_ACCEPT_EMAIL, password=process.env.REET_ACCEPT_PASSWORD, control=process.env.REET_ACCEPT_CONTROL, viewerEmail=process.env.REET_ACCEPT_VIEWER_EMAIL, memberEmail=process.env.REET_ACCEPT_MEMBER_EMAIL
if(!base||!email||!password||!control||!viewerEmail||!memberEmail)throw new Error('run this through TestStage5BrowserAcceptance')
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
// Every browser signs in through the real sign-in form, so the session is the one the server issues.
const signedIn=async(width=1280,account=email)=>{
 const context=await browser.newContext({viewport:{width,height:900}})
 const page=await context.newPage()
 const errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(`${base}/signin`)
 await page.getByLabel('Email').fill(account)
 await page.getByLabel('Password',{exact:true}).fill(password)
 await page.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(page.getByRole('navigation',{name:'Main navigation'}).or(page.getByRole('link',{name:'Machines'})).first()).toBeVisible()
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
 // 1. Signed in through the real sign-in form: the Library lists every item once, none selected anywhere yet.
 await page.goto(`${base}/library`)
 await expect(page.getByRole('heading',{name:'Library',level:1})).toBeVisible()
 for(const name of Object.values(names)){
  await expect(item(page,name)).toBeVisible()
  await expect(item(page,name).getByLabel(`Status of ${name}`)).toHaveValue('in_use')
  await expect(item(page,name)).toContainText('Not selected on any machine')
 }
 const ownerItems=await page.evaluate(async()=>(await (await fetch('/api/v2/library')).json()).items.map(item=>({id:item.id,name:item.name,version:item.version,status:item.status})))
 expect(ownerItems.length).toBe(3)
 await shot(page,'accept-1-library-before')
 log('signed in through the real form; the Library shows each item once, In use, selected nowhere')

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

 // 10. A machine that is really offline: the page calls it offline once it has been quiet past its window, a selection can still
 // be saved for it, the rows say the update is waiting, and the machine catches up by itself when it comes back.
 await ctl('/rounds',{m:'laptop',n:0,pause:45000})
 await page.goto(`${base}/machines`)
 await expect(page.getByRole('article',{name:'laptop',exact:true})).toContainText(/offline/i)
 ;({dialog,panel}=await openPicker(page,'laptop'))
 await panel.getByRole('button',{name:'Select all In use'}).click()
 await panel.getByRole('button',{name:'Save selection'}).click()
 await expect(panel.getByRole('status').filter({hasText:'Selection saved'})).toBeVisible()
 await expect(stateOf(panel,names.review)).toHaveAttribute('data-item-state','waiting_offline')
 await expect(stateOf(panel,names.review)).toContainText('Offline — update waiting')
 await expect(stateOf(panel,names.api)).toHaveAttribute('data-item-state','waiting_offline')
 await shot(page,'accept-10-offline-waiting',{phone:true})
 await page.keyboard.press('Escape')
 expect(await present('laptop',names.review),'an offline machine received nothing yet').toBe(false)
 await rounds('laptop',8)
 expect(await present('laptop',names.review)).toBe(true)
 expect(await present('laptop',names.api)).toBe(true)
 expect(await present('laptop',names.notes),'an item that is not In use never arrived').toBe(false)
 ;({dialog,panel}=await openPicker(page,'laptop'))
 await expect(stateOf(panel,names.review)).toHaveAttribute('data-item-state','verified')
 await expect(stateOf(panel,names.api)).toHaveAttribute('data-item-state','verified')
 await page.keyboard.press('Escape')
 log('an offline machine kept its saved selection as waiting and caught up by itself on return')

 // 11. A real read-only account in the same instance. The server gives private skill content only to a writer, so this account has no
 // library: the page says so plainly (no raw refusal, no retry, no offer to write), the menu does not list it, and the real API
 // refuses it the owner's items, history and changes. The owner's data is exactly as it was.
 {
  const other=await signedIn(1280,viewerEmail)
  const {page:v}=other
  await v.goto(`${base}/library`)
  await expect(v.getByRole('heading',{name:'The library is for accounts that can write'})).toBeVisible()
  await expect(v.getByRole('button',{name:'Try again'})).toHaveCount(0)
  await expect(v.getByRole('link',{name:'Write a skill'}),'a read-only account was offered authoring').toHaveCount(0)
  for(const owned of ownerItems)await expect(v.getByText(owned.name)).toHaveCount(0)
  await v.waitForTimeout(800) // let the page settle before it is photographed
  await shot(v,'accept-11-read-only-library',{phone:true})
  await v.goto(`${base}/machines`)
  await expect(v.getByRole('article',{name:'workstation',exact:true})).toHaveCount(0)
  await expect(v.getByRole('article',{name:'laptop',exact:true})).toHaveCount(0)
  const probe=await v.evaluate(async(items)=>{
   const call=async(path,init)=>(await fetch(path,init)).status
   const own=items[0]
   return {
    library:await call('/api/v2/library'),
    read:await call(`/api/v2/library/${own.id}`),
    history:await call(`/api/v2/library-history/${own.id}`),
    patch:await call(`/api/v2/library/${own.id}`,{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({status:'disabled',expectedVersion:own.version})}),
    syncAgents:((await (await fetch('/api/v2/sync')).json()).agents||[]).length,
   }
  },ownerItems)
  for(const key of ['library','read','history','patch'])expect([403,404],`the read-only account was not refused: ${JSON.stringify(probe)}`).toContain(probe[key])
  expect(probe.syncAgents,'the read-only account saw the owner’s machines').toBe(0)
  const after=await page.evaluate(async()=>(await (await fetch('/api/v2/library')).json()).items.map(item=>({name:item.name,status:item.status})))
  expect(after.find(item=>item.name===names.review).status).toBe('in_use')
  expect(after.length).toBe(3)
  expect(other.errors).toEqual([])
  await other.context.close()
  log('a real read-only account has no library, is offered no authoring, and is refused the owner’s items, history and changes')
 }

 // 12. A real second writer with nothing yet: the library page shows the empty state with its two next steps, and Machines shows
 // none of the owner's machines.
 {
  const other=await signedIn(1280,memberEmail)
  const {page:m}=other
  await m.goto(`${base}/library`)
  await expect(m.getByRole('heading',{name:'Your library is empty'})).toBeVisible()
  const region=m.getByRole('region',{name:'Your library'})
  await expect(region.getByRole('link',{name:'Connect a machine'})).toBeVisible()
  await expect(region.getByRole('link',{name:'Write a skill'})).toBeVisible()
  for(const owned of ownerItems)await expect(m.getByText(owned.name)).toHaveCount(0)
  await m.waitForTimeout(800)
  await shot(m,'accept-12-empty-library',{phone:true})
  await m.goto(`${base}/machines`)
  await expect(m.getByRole('article',{name:'workstation',exact:true})).toHaveCount(0)
  expect(other.errors).toEqual([])
  await other.context.close()
  log('a real second writer with nothing yet sees the empty library, with its next steps')
 }

 expect(errors).toEqual([])
 await context.close()
} finally { await browser.close() }
