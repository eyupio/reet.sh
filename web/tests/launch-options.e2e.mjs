import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A='connector-options00000001', S='session-options0000000001', D='draft-options00000000001', now=Date.now()
const agent={tool:'codex',adapter:'codex-acp',state:'available',model:true,options:[{id:'reasoning_effort',category:'thought_level',values:['low','high','ultra']}]}
const machine={agentId:A,host:'Options machine',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalControl:true,terminalEnabled:true,healthEnabled:true,readiness:'ready',launch:true,agentsV2:true,models:true,acp:true,acpLiveControls:true,acpStartOptions:true,acpAgentOptions:true,tools:['codex'],acpAgents:[agent]}}
const project={id:'project-options',name:'Options project',host:A,machine:'Options machine',dir:'/tmp',isRepo:false,worktreeDefault:'no',defaultTool:'codex',defaultModel:'provider/one',defaultEffort:'',hasStarterPrompt:false,colour:'blue',position:0,createdAt:now,updatedAt:now,version:1}
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium',args:['--no-sandbox']})
try {
 for(const width of [1360,390]) {
  const sessions=[], requests=[], drafts=[], machines=[{...machine}]
  const made=body=>({id:S,agentId:A,title:body.title,state:'running',createdAt:now,updatedAt:now,tool:'codex',model:body.model,effort:body.effort,structured:true,dir:'/tmp',exitCode:null})
  const {page,context,errors}=await openWorkspace(browser,{width,height:900,touch:width<600,terminalStatus:active,terminalMachines:machines,routes:{
   '/api/v2/terminal/sessions':req=>{if(req.method()==='POST'){const body=req.postDataJSON();requests.push(body);const row=made(body);sessions.splice(0,sessions.length,row);return row}return {sessions}},
   '/api/v2/folder-projects':{projects:[project],savedWaiting:0},
   [`/api/v2/machines/${A}/models`]:{tool:'codex',state:'listed',models:[{id:'provider/one'},{id:'provider/two'}],more:false,source:'',version:'',fetchedAt:now,checkedAt:now,reason:'',sentence:''},
   '/api/v2/session-backlog':req=>{if(req.method()==='POST'){const body=req.postDataJSON(),row={...body,id:D,version:1,imageCount:0,images:[],createdAt:now,updatedAt:now};drafts.push(row);return row}return {drafts}},
   [`/api/v2/session-backlog/${D}`]:req=>{if(req.method()==='PUT')Object.assign(drafts[0],req.postDataJSON());return drafts[0]},
   [`/api/v2/session-backlog/${D}/start`]:()=>{const row=made(drafts[0]);drafts[0].sessionId=row.id;sessions.splice(0,sessions.length,row);return row},
   [`/api/v2/terminal/sessions/${S}/chat/controls`]:{controls:{working:false,waiting:false,compacting:false,configuring:false,steering:false,model:'provider/two',options:[{id:'reasoning_effort',category:'thought_level',current:'ultra',values:['low','high','ultra']}]}},
   [`/api/v2/terminal/sessions/${S}/chat/queue`]:{queue:[]},
  }})
  page.setDefaultTimeout(10000)
  await page.addInitScript(()=>{class Stream{constructor(){this.listeners={};setTimeout(()=>this.onopen?.({}),0)}addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}close(){}}window.EventSource=Stream})
  async function pickModel(dialog){await dialog.getByRole('button',{name:/^Model:/}).click();await page.getByRole('listbox',{name:'Model',exact:true}).getByRole('option',{name:'provider/two',exact:true}).click()}
  async function pickEffort(dialog){await dialog.getByRole('button',{name:/^Effort:/}).click();const list=page.getByRole('listbox',{name:'Effort',exact:true});await expect(list.getByRole('option',{name:'ultra',exact:true})).toBeVisible();await list.getByRole('option',{name:'ultra',exact:true}).click()}
  try {
   // Machine/workspace Start an agent modal.
   await page.goto(`${base}/sessions?agent=${A}`)
   await page.getByRole('button',{name:/New agent session|Start a new agent session/}).first().click()
   const modal=page.getByRole('dialog',{name:'Start an agent',exact:true})
   await modal.getByRole('combobox',{name:'Project',exact:true}).selectOption({label:'No project — start in a folder'})
   await modal.getByRole('combobox',{name:'Agent',exact:true}).selectOption('codex')
   await expect(modal.getByRole('combobox',{name:'Session',exact:true})).toHaveValue('reet')
   await pickModel(modal)
   await modal.getByRole('combobox',{name:'Effort',exact:true}).selectOption('ultra')
   await modal.getByRole('combobox',{name:'Effort',exact:true}).scrollIntoViewIfNeeded()
   if(process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/start-agent-options-${width}.png`,fullPage:true})
   await modal.getByRole('textbox',{name:/Opening prompt/}).fill('Review the selected options')
   await modal.getByRole('button',{name:'Start',exact:true}).click()
   await expect.poll(()=>requests.at(-1)?.effort).toBe('ultra')
   expect(requests.at(-1).model).toBe('provider/two')
   await expect(modal).toHaveCount(0)
   await page.waitForURL(new RegExp(`session=${S}`))

   // Home's New session opens the same Start an agent dialog as everywhere else.
   await page.goto(`${base}/`)
   await page.getByRole('button',{name:'New session',exact:true}).click()
   const start=page.getByRole('dialog',{name:'Start an agent',exact:true})
   await start.getByRole('combobox',{name:'Project',exact:true}).selectOption('project-options')
   await start.getByRole('textbox',{name:/Opening prompt/}).fill('Start with the selected model and effort')
   await pickModel(start)
   await start.getByRole('combobox',{name:'Effort',exact:true}).selectOption('ultra')
   if(process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/start-session-options-${width}.png`,fullPage:true})
   await start.getByRole('button',{name:'Start',exact:true}).click()
   await expect.poll(()=>requests.length).toBe(2)
   expect(requests.at(-1)).toMatchObject({model:'provider/two',effort:'ultra',structured:true})
   await expect(start).toHaveCount(0)
   await page.waitForURL(new RegExp(`session=${S}`))

   // The launch bar's Start a session offers the machine's Gaffer effort too, as a sentence slot or in the Change sheet.
   await page.goto(`${base}/#launch`)
   const bar=page.getByRole('dialog',{name:'Start a session',exact:true})
   await bar.getByPlaceholder(/Describe the job/).fill('Choose effort in the launch bar')
   const wide=width>=650
   if (wide) {
    const thinking=bar.getByRole('button',{name:/^Thinking:/})
    await expect(thinking).toBeVisible()
    await thinking.click()
   } else {
    await expect(bar.locator('.summary-text')).toContainText('thinking')
    await bar.getByRole('button',{name:'Change',exact:true}).last().click()
    const thinking=bar.getByRole('dialog',{name:'Change the launch'}).getByRole('button',{name:/^Thinking:/})
    await expect(thinking).toBeVisible()
    await thinking.click()
   }
   const effortList=page.getByRole('listbox',{name:'Thinking',exact:true})
   await effortList.getByRole('option',{name:'ultra',exact:true}).click()
   if (wide) await expect(bar.getByRole('button',{name:/^Thinking:/})).toHaveAccessibleName(/ultra$/)
   else {
    await expect(bar.getByRole('dialog',{name:'Change the launch'}).getByRole('button',{name:/^Thinking:/})).toHaveAccessibleName(/ultra$/)
    await bar.getByRole('button',{name:'Done',exact:true}).click()
   }
   await bar.getByRole('button',{name:/^Start the session$/}).click()
   await expect.poll(()=>requests.length).toBe(3)
   expect(requests.at(-1)).toMatchObject({model:'provider/one',effort:'ultra',structured:true})
   await page.waitForURL(new RegExp(`session=${S}`))

   // Backlog creation and editing preserve both choices across a reload.
   await page.goto(`${base}/`)
   await page.getByRole('button',{name:'Add session to backlog',exact:true}).click()
   const composer=page.getByRole('dialog',{name:'A little planning. Proper progress.',exact:true})
   await composer.getByLabel('Project',{exact:true}).selectOption('project-options')
   await composer.getByRole('textbox',{name:'Session title',exact:true}).fill('Saved native options')
   await composer.getByRole('textbox',{name:'Prompt',exact:true}).fill('Use the saved model and effort')
   await composer.getByRole('button',{name:'Gaffer',exact:true}).click()
   await pickModel(composer);await pickEffort(composer)
   await composer.getByRole('button',{name:'Add to backlog',exact:true}).click()
   await expect.poll(()=>drafts.length).toBe(1)
   expect(drafts[0]).toMatchObject({model:'provider/two',effort:'ultra',chat:true,tool:'codex',trust:false})
   await expect(composer).toHaveCount(0)
   await page.reload()
   await page.getByRole('button',{name:'Show details for Saved native options',exact:true}).click()
   await page.getByRole('button',{name:'Edit Saved native options',exact:true}).click()
   const edit=page.getByRole('dialog',{name:'Edit session',exact:true})
   await expect(edit.getByRole('button',{name:'Model: provider/two',exact:true})).toBeVisible()
   await expect(edit.getByRole('button',{name:'Effort: ultra',exact:true})).toBeVisible()
   await edit.getByRole('button',{name:'Effort: ultra',exact:true}).scrollIntoViewIfNeeded()
   if(process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/backlog-options-${width}.png`,fullPage:true})
   await edit.getByRole('button',{name:'Cancel',exact:true}).click()
   await expect(edit).toHaveCount(0)
   // An offline capability read must preserve a saved native plan, including its extended effort.
   machines[0]={...machine,ready:false,capabilities:undefined}
   await page.reload()
   await page.getByRole('button',{name:'Show details for Saved native options',exact:true}).click()
   await page.getByRole('button',{name:'Edit Saved native options',exact:true}).click()
   const offline=page.getByRole('dialog',{name:'Edit session',exact:true})
   await expect(offline.getByRole('button',{name:'Gaffer',exact:true})).toHaveAttribute('aria-pressed','true')
   await expect(offline.getByRole('button',{name:'Effort: ultra',exact:true})).toBeVisible()
   await offline.getByRole('textbox',{name:'Prompt',exact:true}).fill('Keep the native settings while offline')
   await offline.getByRole('button',{name:'Save changes',exact:true}).click()
   await expect(offline).toHaveCount(0)
   expect(drafts[0]).toMatchObject({prompt:'Keep the native settings while offline',model:'provider/two',effort:'ultra',chat:true,tool:'codex'})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
   expect(errors).toEqual([])
  } catch(e) {
   console.log((await page.locator('body').innerText()).slice(0,7000));console.log(await page.getByRole('button').allTextContents());throw e
  } finally {await context.close()}
 }
 console.log('Start-agent, start-session and backlog model/effort selection passed on desktop and phone.')
} finally {await browser.close()}
