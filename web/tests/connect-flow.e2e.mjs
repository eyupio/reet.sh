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
 const opener=page.getByRole('article',{name:'new-box',exact:true}).getByRole('button',{name:'Choose skills',exact:true})
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
await browser.close()
