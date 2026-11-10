import { chromium, expect } from '@playwright/test'
import { owner, openWorkspace } from './helpers/workspace.mjs'

// Choosing a machine's skills has one entry point, with one name: Skills on this machine. The card's button opens the machine's
// skill dialog directly (the menu no longer repeats it), and a link that says ?choose=skills (Home's Skills on this machine) opens it once on arrival and tidies the
// address. Anyone who cannot use the dialog (a viewer) is not sent into a dead end: they keep the plain link, or just land on
// the machine's card. Job review and activity links are diagnostics and are untouched.
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const id=ch=>ch.repeat(32)
const library=[{id:id('1'),name:'code-review',itemType:'skill',status:'in_use',contentState:'complete'}]
const folder={id:'new-box-agents',host:'new-box',agent:'agents',name:'Shared',skills:[],skillCount:0,complete:true,scannedAt:Date.now(),lastSeen:Date.now()}
const sync={environments:[folder],machines:[{host:'new-box',groups:[folder],skillCount:0}],agents:[{id:'agent-new',host:'new-box',paused:false,autoUpdate:false,keepSynced:true,lastSeen:Date.now()}],targets:[],jobs:[],runs:[],updateJobs:[]}
const open=(width,{viewer=false}={})=>openWorkspace(browser,{width,touch:width<600,sync,status:viewer?{user:{...owner,role:'viewer'}}:undefined,routes:{
 '/api/v2/library':{schemaVersion:1,items:library},
 '/api/v2/library/selections/new-box-agents':{status:404,json:{error:'private skill unavailable'}},
}})
const card=page=>page.getByRole('article',{name:'new-box',exact:true})

for(const width of [390,1280]){
 { // The card's button opens the dialog itself, on the same page; the menu does not offer a second way in, and focus goes back to the button.
  const {page,context,errors}=await open(width)
  await page.goto(`${base}/machines`)
  await card(page).getByRole('group').or(card(page)).locator('summary[aria-label="More options for new-box"]').click()
  await expect(page.getByRole('menuitem',{name:'Settings…',exact:true})).toBeVisible()
  await expect(page.getByRole('menuitem',{name:'Choose skills & locations'})).toHaveCount(0)
  await expect(page.getByRole('menuitem',{name:'Skills on this machine'})).toHaveCount(0)
  await page.keyboard.press('Escape')
  await card(page).getByRole('button',{name:'Skills on this machine',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'Skills on new-box',exact:true})
  await expect(dialog).toBeVisible()
  expect(new URL(page.url()).pathname).toBe('/machines')
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(card(page).getByRole('button',{name:'Skills on this machine',exact:true})).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
  console.log(`PASS the card button opens the skill dialog at ${width}px`)
 }
 { // A link with ?choose=skills opens the dialog once on the machine, then the address is tidied and a reload does not reopen it.
  const {page,context,errors}=await open(width)
  await page.goto(`${base}/machines?machine=new-box&choose=skills`)
  const dialog=page.getByRole('dialog',{name:'Skills on new-box',exact:true})
  await expect(dialog).toBeVisible()
  await expect.poll(()=>new URL(page.url()).searchParams.has('choose')).toBe(false)
  expect(new URL(page.url()).searchParams.get('machine')).toBe('new-box')
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await page.reload()
  await expect(card(page)).toBeVisible()
  await expect(dialog).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log(`PASS ?choose=skills opens once at ${width}px`)
 }
}

{ // A viewer has no dialog: the menu item stays the plain link, and ?choose=skills lands on the machine without opening anything.
 const {page,context,errors}=await open(1280,{viewer:true})
 await page.goto(`${base}/machines?machine=new-box&choose=skills`)
 await expect(card(page)).toBeVisible()
 await expect(page.getByRole('dialog',{name:'Skills on new-box',exact:true})).toHaveCount(0)
 await expect.poll(()=>new URL(page.url()).searchParams.has('choose')).toBe(false)
 await card(page).locator('summary[aria-label="More options for new-box"]').click()
 const item=page.getByRole('menuitem',{name:'Choose skills & locations'})
 expect(await item.evaluate(node=>node.tagName)).toBe('A')
 await expect(item).toHaveAttribute('href','/sync?machine=new-box')
 expect(errors).toEqual([])
 await context.close()
 console.log('PASS viewer keeps the plain link and no dialog')
}

{ // An unknown machine in the link opens nothing and breaks nothing.
 const {page,context,errors}=await open(1280)
 await page.goto(`${base}/machines?machine=no-such-box&choose=skills`)
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await expect.poll(()=>new URL(page.url()).searchParams.has('choose')).toBe(false)
 expect(errors).toEqual([])
 await context.close()
 console.log('PASS unknown machine opens nothing')
}
await browser.close()
