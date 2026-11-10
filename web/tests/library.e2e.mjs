import { chromium, expect } from '@playwright/test'
import { owner, openWorkspace } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'

// The minimal Library view: one entry per canonical item, with its status, the machines that
// keep it, the shelf details migration preserved and its saved history. It reads the canonical
// library only; what is installed where is the Machines page's job, and nothing here claims it.
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const now=Date.now()
const id=ch=>ch.repeat(32)
const item=(ch,name,changes={})=>({id:id(ch),name,itemType:'skill',status:'in_use',contentState:'complete',version:3,published:false,aliases:[],...changes})
const initial=()=>[
 item('1','code-review',{aliases:[{kind:'hosted',reference:'code-review'}],metadata:{liked:true,lists:[{id:1,name:'Daily'}],tags:[{id:1,name:'review'}]}}),
 item('2','api-design',{status:'to_test'}),
 item('3','release-notes',{status:'disabled'}),
 item('4','alpine-setup',{status:'on_shelf'}),
 item('5','pending-capture',{contentState:'pending_capture'}),
]
const laptop={id:'laptop-shared',host:'Dev laptop',agent:'agents',name:'Shared',skills:[],skillCount:0,complete:true,scannedAt:now,lastSeen:now}
const desktop={id:'desktop-claude',host:'Office desktop',agent:'claude',name:'Claude Code',skills:[],skillCount:0,complete:true,scannedAt:now,lastSeen:now}
const sync={environments:[laptop,desktop],machines:[{host:laptop.host,groups:[laptop],skillCount:0},{host:desktop.host,groups:[desktop],skillCount:0}],
 agents:[{id:'a1',host:laptop.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:now},{id:'a2',host:desktop.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:now}],targets:[],jobs:[],runs:[],updateJobs:[]}
const selections={[laptop.id]:{targetId:laptop.id,itemIds:[id('1'),id('2')],version:'v'},[desktop.id]:{targetId:desktop.id,itemIds:[id('1')],version:'v'}}
const receipts=[{eventId:'e2',revisionId:id('b'),won:true,base:id('a'),savedAt:Date.UTC(2026,10,9,10,12),orderAt:1,uncertainty:0,clockFallback:false},{eventId:'e1',revisionId:id('a'),won:false,base:'',savedAt:Date.UTC(2026,10,8,9,5),orderAt:0,uncertainty:0,clockFallback:true}]

async function open(width,{theme='light',library=initial,libraryAnswer,viewer=false,patch,history=receipts}={}){
 const routes={
  '/api/v2/library':libraryAnswer||(()=>({schemaVersion:1,items:library()})),
  [`/api/v2/library/selections/${laptop.id}`]:selections[laptop.id],
  [`/api/v2/library/selections/${desktop.id}`]:selections[desktop.id],
 }
 for(const entry of initial()){
  routes[`/api/v2/library/${entry.id}`]=request=>patch?patch(request,entry):{json:{...entry}}
  routes[`/api/v2/library-history/${entry.id}`]=()=>history
 }
 return openWorkspace(browser,{width,theme,touch:width<600,sync,routes,status:viewer?{user:{...owner,role:'viewer'}}:undefined})
}
const row=(page,name)=>page.locator('.item',{has:page.getByRole('heading',{name,exact:true})})
const clean=async(page,errors,context,width)=>{
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(errors).toEqual([])
 await context.close()
}
const shot=async(page,name)=>{if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/${name}.png`})}}

try {
 for(const width of [320,390,1280]) for(const theme of ['light','dark']){
  const {page,context,errors}=await open(width,{theme})
  await page.goto(`${base}/library`)
  await expect(page.getByRole('heading',{name:'Library',level:1})).toBeVisible()
  await expect(page).toHaveTitle(/Library · reet\.sh/)
  await expect(page.locator('.item')).toHaveCount(5)
  await expect(page.getByRole('status').filter({hasText:'5 items'})).toBeVisible()
  // Counts per status, over the whole library.
  const chips=page.getByRole('group',{name:'Filter by status'})
  await expect(chips.getByRole('button',{name:/^All 5$/})).toHaveAttribute('aria-pressed','true')
  await expect(chips.getByRole('button',{name:/^In use 2$/})).toBeVisible()
  await expect(chips.getByRole('button',{name:/^Disabled 1$/})).toBeVisible()
  // Library details that migration preserved, and which machines keep each item (saved intent, not installation).
  const review=row(page,'code-review')
  await expect(review).toContainText('Written here · private · Liked · On list Daily · Tagged review')
  await expect(review).toContainText('Selected on Dev laptop, Office desktop')
  await expect(review.getByRole('link',{name:'Choose machines on the Machines page'})).toHaveAttribute('href','/machines')
  await expect(row(page,'api-design')).toContainText('Selected on Dev laptop, not installed while to test')
  await expect(row(page,'release-notes')).toContainText('Not selected on any machine')
  await expect(row(page,'pending-capture')).toContainText('Waiting for complete content')
  await expect(review).not.toContainText(/up to date|installed on/i)
  await expect(review.getByLabel('Status of code-review')).toHaveValue('in_use')
  if(width===390&&theme==='dark')await shot(page,'library-page-phone-dark')
  if(width===1280&&theme==='light')await shot(page,'library-page-desktop-light')
  await clean(page,errors,context,width)
  console.log(`PASS populated library at ${width}px ${theme}`)
 }

 { // Search and status filter combine, and inactive items stay findable.
  const {page,context,errors}=await open(390)
  await page.goto(`${base}/library`)
  await page.getByLabel('Find an item').fill('al')
  await expect(page.locator('.item')).toHaveCount(1)
  await expect(page.getByRole('status').filter({hasText:'1 of 5 items'})).toBeVisible()
  await page.getByRole('button',{name:/^Disabled/}).click()
  await expect(page.getByRole('button',{name:/^Disabled/})).toHaveAttribute('aria-pressed','true')
  await expect(page.getByText('No items match.')).toBeVisible()
  await page.getByLabel('Find an item').fill('')
  await expect(page.locator('.item')).toHaveCount(1)
  await expect(row(page,'release-notes')).toBeVisible()
  await page.getByRole('button',{name:/^All/}).click()
  await expect(page.locator('.item')).toHaveCount(5)
  await clean(page,errors,context,390)
  console.log('PASS search and status filter')
 }

 { // Changing a status sends the version it was read at, updates the row and keeps its machines.
  const bodies=[]
  const {page,context,errors}=await open(390,{patch:(request,entry)=>{bodies.push(request.postDataJSON());return {json:{...entry,status:'disabled',version:4}}}})
  await page.goto(`${base}/library`)
  await row(page,'code-review').getByLabel('Status of code-review').selectOption('disabled')
  await expect(row(page,'code-review')).toContainText('Selected on Dev laptop, Office desktop, not installed while disabled')
  expect(bodies).toEqual([{status:'disabled',expectedVersion:3}])
  await expect(row(page,'code-review').getByLabel('Status of code-review')).toHaveValue('disabled')
  await clean(page,errors,context,390)
  console.log('PASS status change')
 }

 { // A stale change is refused by the server: say so beside the item, put the select back, refresh.
  let reads=0
  const {page,context,errors}=await open(390,{libraryAnswer:()=>{reads++;return {schemaVersion:1,items:initial()}},patch:()=>({status:409,json:{error:'The library item or location changed. Refresh before retrying.'}})})
  await page.goto(`${base}/library`)
  await row(page,'api-design').getByLabel('Status of api-design').selectOption('in_use')
  await expect(row(page,'api-design').getByRole('alert')).toContainText('changed somewhere else')
  await expect(row(page,'api-design').getByLabel('Status of api-design')).toHaveValue('to_test')
  await expect.poll(()=>reads).toBeGreaterThan(1)
  await clean(page,errors,context,390)
  console.log('PASS stale status change')
 }

 { // History is read on demand, says whether each save became current, and closes again.
  const {page,context,errors}=await open(1280)
  await page.goto(`${base}/library`)
  const entry=row(page,'code-review')
  const toggle=entry.getByRole('button',{name:'History'})
  await expect(toggle).toHaveAttribute('aria-expanded','false')
  await toggle.focus(); await page.keyboard.press('Enter')
  await expect(toggle).toHaveAttribute('aria-expanded','true')
  const region=entry.getByRole('region',{name:'History of code-review'})
  await expect(region).toContainText('9 Nov 2026')
  await expect(region).toContainText('became the current version')
  await expect(region).toContainText('kept as another version (time estimated)')
  await shot(page,'library-page-history-desktop')
  await page.keyboard.press('Enter')
  await expect(region).toHaveCount(0)
  await clean(page,errors,context,1280)
  console.log('PASS item history')
 }

 { // An item with no recorded saves says so plainly.
  const {page,context,errors}=await open(390,{history:[]})
  await page.goto(`${base}/library`)
  await row(page,'api-design').getByRole('button',{name:'History'}).click()
  await expect(row(page,'api-design')).toContainText('No saves are recorded for this item yet')
  await clean(page,errors,context,390)
  console.log('PASS empty history')
 }

 { // Empty library: plain next steps, not an empty list.
  const {page,context,errors}=await open(390,{library:()=>[]})
  await page.goto(`${base}/library`)
  await expect(page.getByRole('heading',{name:'Your library is empty'})).toBeVisible()
  await expect(page.getByRole('link',{name:'Connect a machine'})).toHaveAttribute('href','/machines')
  await expect(page.getByRole('link',{name:'Write a skill'})).toHaveAttribute('href','/hosting')
  await expect(page.getByLabel('Find an item')).toHaveCount(0)
  await shot(page,'library-page-empty-phone')
  await clean(page,errors,context,390)
  console.log('PASS empty library')
 }

 { // A failed read shows the reason and recovers on retry.
  let fail=true
  const {page,context,errors}=await open(390,{libraryAnswer:()=>fail?{status:500,json:{error:'The library could not be read'}}:{schemaVersion:1,items:initial()}})
  await page.goto(`${base}/library`)
  await expect(page.getByRole('alert')).toContainText('The library could not be read')
  fail=false
  await page.getByRole('button',{name:'Try again'}).click()
  await expect(page.locator('.item')).toHaveCount(5)
  await page.evaluate(()=>0)
  expect(errors.filter(message=>!/500/.test(message))).toEqual([])
  await context.close()
  console.log('PASS read failure and retry')
 }

 { // A failure reading machine selections never hides the library; it is said once and can be retried.
  const {page,context,errors}=await open(390)
  await page.route('**/api/v2/library/selections/**',route=>route.fulfill({status:500,json:{error:'boom'}}))
  await page.goto(`${base}/library`)
  await expect(page.locator('.item')).toHaveCount(5)
  await expect(page.getByText('Machine selections could not be read just now')).toBeVisible()
  await expect(page.getByText('Not selected on any machine')).toHaveCount(0)
  await context.close()
  console.log('PASS machine selection failure')
 }

 { // A read-only account has no library: the server gives private skill content only to a writer, so the page says so plainly
  // instead of asking and showing a refusal, never offers to write or retry, and the menu does not list it.
  const asked=[]
  const {page,context,errors}=await open(390,{viewer:true})
  page.on('request',request=>{if(new URL(request.url()).pathname.startsWith('/api/v2/library'))asked.push(request.url())})
  await page.goto(`${base}/library`)
  await expect(page.getByRole('heading',{name:'The library is for accounts that can write'})).toBeVisible()
  await expect(page.getByText('Ask the owner of this instance for write access')).toBeVisible()
  await expect(page.getByRole('button',{name:'Try again'})).toHaveCount(0)
  await expect(page.getByLabel('Find an item')).toHaveCount(0)
  await expect(page.getByRole('link',{name:'Write a skill'})).toHaveCount(0)
  expect(asked,'a read-only account asked for the library').toEqual([])
  await page.getByRole('button',{name:'More'}).first().click()
  await expect(page.getByRole('dialog',{name:'More'}).getByRole('link',{name:'Library'})).toHaveCount(0)
  await clean(page,errors,context,390)
  console.log('PASS read-only account')
 }

 { // Keyboard: the filter chips and status selects work without a pointer.
  const {page,context,errors}=await open(1280)
  await page.goto(`${base}/library`)
  await page.getByRole('button',{name:/^To test/}).focus()
  await page.keyboard.press('Space')
  await expect(page.locator('.item')).toHaveCount(1)
  await expect(page.getByRole('button',{name:/^To test/})).toHaveAttribute('aria-pressed','true')
  await clean(page,errors,context,1280)
  console.log('PASS keyboard')
 }
} finally { await browser.close() }
