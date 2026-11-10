import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'

// Each selected item says what is really happening to it on this machine, from what the
// controller holds, beside the item itself: installed, waiting (offline or not), syncing,
// needs attention (with its reason) and kept in the library only. Nothing is claimed that
// the controller cannot know, such as "Up to date" from a byte comparison.
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const now=Date.now()
const id=ch=>ch.repeat(32)
const library=[
 {id:id('1'),name:'installed-item',itemType:'skill',status:'in_use',contentState:'complete'},
 {id:id('2'),name:'waiting-item',itemType:'skill',status:'in_use',contentState:'complete'},
 {id:id('3'),name:'problem-item',itemType:'skill',status:'in_use',contentState:'complete'},
 {id:id('4'),name:'inactive-item',itemType:'skill',status:'to_test',contentState:'complete'},
 {id:id('5'),name:'unselected-item',itemType:'skill',status:'in_use',contentState:'complete'},
]
const desired=name=>({id:`hosted/x/${name}`,itemId:library.find(item=>item.name===name).id,name,slug:name,source:`private:${id('b')}:${id('c')}:${id('d')}`,sourceType:'private'})
const env={id:'laptop-shared',host:'Dev laptop',agent:'agents',name:'Shared',skills:[],skillCount:0,complete:true,scannedAt:now,lastSeen:now}
const selection={targetId:env.id,itemIds:[id('1'),id('2'),id('3'),id('4')],version:'v1'}
const reason='The local entry is not a managed directory.'
const finished={id:'job-done',agentId:'agent-1',targetId:env.id,kind:'apply',status:'succeeded',revision:'r',hash:'h',message:'Some selected items are waiting; healthy items continue syncing.',createdAt:now-60000,updatedAt:now-30000,plan:{target:env.id,directory:'/home/dev/.agents/skills',revision:'r',actions:[],problems:{'problem-item':reason}}}
const syncState=({online,jobs})=>({environments:[env],machines:[{host:env.host,groups:[env],skillCount:0}],agents:[{id:'agent-1',host:env.host,paused:false,autoUpdate:false,keepSynced:true,lastSeen:online?now:now-3600000}],
 targets:[{target:env,sourceId:'library',sourceName:'Selected library items',revision:'r',updatedAt:now,desired:['installed-item','waiting-item','problem-item'].map(desired),missing:['waiting-item','problem-item'].map(desired),extra:[]}],jobs,runs:[],updateJobs:[]})

async function scenario({name,width,theme,online,jobs,expectations,shot}){
 const {page,context,errors}=await openWorkspace(browser,{width,theme,touch:width<600,sync:syncState({online,jobs}),routes:{
  '/api/v2/library':{schemaVersion:1,items:library},
  [`/api/v2/library/selections/${env.id}`]:selection,
 }})
 await page.goto(`${base}/machines`)
 const opener=page.getByRole('article',{name:env.host,exact:true}).getByRole('button',{name:'Skills on this machine',exact:true})
 await opener.click()
 const dialog=page.getByRole('dialog',{name:`Skills on ${env.host}`,exact:true})
 const panel=dialog.getByRole('region',{name:'Skills to sync'})
 await expect(panel.getByRole('checkbox',{name:'installed-item'})).toBeChecked()
 for(const [item,key,text] of expectations){
  const row=panel.locator('.row',{has:page.getByRole('checkbox',{name:item})})
  const state=row.locator('[data-item-state]')
  await expect(state).toHaveAttribute('data-item-state',key)
  await expect(state).toContainText(text)
  // The state is tied to its checkbox for assistive technology without changing the checkbox's own name.
  const describedBy=await row.getByRole('checkbox').getAttribute('aria-describedby')
  expect(describedBy).toBe(await state.getAttribute('id'))
 }
 // An item that is not selected says nothing about this machine.
 const unselected=panel.locator('.row',{has:page.getByRole('checkbox',{name:'unselected-item'})})
 await expect(unselected.locator('[data-item-state]')).toHaveCount(0)
 await expect(panel).not.toContainText(/up to date/i)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 const bounds=await dialog.boundingBox()
 expect(bounds.x).toBeGreaterThanOrEqual(0)
 expect(bounds.x+bounds.width).toBeLessThanOrEqual(width+1)
 // Keyboard: a row's checkbox is reachable and toggles with Space without leaving the dialog.
 await panel.getByRole('checkbox',{name:'installed-item'}).focus()
 await page.keyboard.press('Space')
 await expect(panel.getByRole('checkbox',{name:'installed-item'})).not.toBeChecked()
 await page.keyboard.press('Space')
 await expect(panel.getByRole('checkbox',{name:'installed-item'})).toBeChecked()
 if(shot&&process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/${shot}.png`})}
 expect(errors).toEqual([])
 await context.close()
 console.log(`PASS ${name} at ${width}px ${theme}`)
}

try {
 for(const width of [320,390,1280]) for(const theme of ['light','dark']){
  // Offline machine: items that have not arrived say so; the reason for a failure sits beside its own item.
  await scenario({name:'offline machine item states',width,theme,online:false,jobs:[finished],shot:width===390&&theme==='dark'?'machine-item-states-phone-offline':width===1280&&theme==='light'?'machine-item-states-desktop-offline':undefined,expectations:[
   ['installed-item','verified','Installed'],
   ['waiting-item','waiting_offline','Offline — update waiting'],
   ['problem-item','attention',reason],
   ['inactive-item','inactive','Kept in the library only'],
  ]})
 }
 // Online and syncing: a running job for this folder shows Syncing on the items still to arrive.
 await scenario({name:'online machine syncing state',width:390,theme:'light',online:true,jobs:[{...finished,id:'job-run',status:'running',updatedAt:now}],shot:'machine-item-states-phone-syncing',expectations:[
  ['waiting-item','syncing','Syncing'],
  ['installed-item','syncing','Syncing'],
 ]})
} finally { await browser.close() }
