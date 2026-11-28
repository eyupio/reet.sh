import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

// Pasting a work order into the "New Factory outcome" dialog recognises it, checks it with the server, and opens the
// plan review already filled in: nothing is sent to a model. See docs/superpowers/specs/2026-11-28-factory-work-orders-design.md.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-work-order'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),generalId='f'.repeat(32)
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
// The machine runs Claude Code and Codex only: no OpenCode, so a work order that suggests it cannot start here.
const machines=[{agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,acp:true,acpStartOptions:true,acpAgents:[
 {tool:'claude',adapter:'claude-agent-acp',state:'available',options:[{id:'effort',category:'thought_level',values:['default','low','medium','high','max']}]},
 {tool:'codex',adapter:'codex-acp',state:'available'},
]}}]
const folder=(id,name)=>({id,host:'a1',machine:'workshop',dir:`/work/${name}`,name,isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now})
const projects=[folder('p-reports','reports-api')]

// The example work order from the published skill, exactly as a person would paste it.
const goodText=`---
reet: work-order/1
objective: Add CSV export to the reports page
project: reports-api
crew: heavy=claude/opus/high, standard=codex
---

## Pipeline: Export endpoint

stages: plan, code, review, test, pr
tiers: code=heavy, review=standard

Add GET /reports/{id}/export.csv for billing.

### Part: code / Streaming writer
tier: heavy

Write rows as they are read, never the whole file in memory.

### Part: code / Endpoint and auth

Wire the handler and reuse the report's read permission.

### Criteria

- An export of 10,000 rows finishes in under 5 seconds
- A reader of another team's report is refused
`
const goodOrder={objective:'Add CSV export to the reports page',project:'reports-api',crew:{heavy:{tool:'claude',model:'opus',effort:'high'},standard:{tool:'codex'}},sessions:[{
 title:'Export endpoint',prompt:'Add GET /reports/{id}/export.csv for billing.',
 stages:[{kind:'plan'},{kind:'code',part:'Streaming writer',tier:'heavy',brief:'Write rows as they are read, never the whole file in memory.'},{kind:'code',part:'Endpoint and auth',tier:'heavy',brief:"Wire the handler and reuse the report's read permission."},{kind:'review',tier:'standard'},{kind:'test'},{kind:'pr'}],
 criteria:['An export of 10,000 rows finishes in under 5 seconds',"A reader of another team's report is refused"],
}]}
const goodCheck={order:goodOrder,problems:[],projectId:'p-reports',pathwayId:'',notes:['No pathway is named "none"; choose one yourself.']}

// A work order whose crew suggests an agent this machine does not run.
const missingText=`---
reet: work-order/1
objective: Tidy the import job
project: reports-api
crew: standard=opencode
---

## Pipeline: Import tidy

stages: code

Clean up the stray log lines in the importer.
`
const missingOrder={objective:'Tidy the import job',project:'reports-api',crew:{standard:{tool:'opencode'}},sessions:[{title:'Import tidy',prompt:'Clean up the stray log lines in the importer.',stages:[{kind:'code'}]}]}
const missingCheck={order:missingOrder,problems:[],projectId:'p-reports',pathwayId:'',notes:[]}

// A work order with a header key the format does not have: refused before any pipeline is read, so nothing moves on.
const yoloText=`---
reet: work-order/1
objective: Ship it fast
yolo: true
---

## Pipeline: Ship it

stages: code

Ship it.
`
const yoloCheck={order:null,problems:[{line:4,text:'Unknown header key "yolo". Keep to reet, objective, project, crew and continues.'}],projectId:'',pathwayId:'',notes:[]}

function fixture(){
 const box={checks:[],approved:[]}
 const general={conversation:{id:generalId,sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/crew':{crew:{light:{},standard:{},heavy:{}},saved:false,tiers:['light','standard','heavy'],defaults:{}},
  '/api/v2/folder-projects':{projects,savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:true,changedBy:'owner@example.test'},
  '/api/v2/profile/model-favourites':{version:1,favourites:[]},
  '/api/v2/machines/a1/models':request=>{const tool=new URL(request.url()).searchParams.get('tool');return {tool,state:'listed',models:[],more:false,source:'cli',version:'1',fetchedAt:now,checkedAt:now,reason:'',sentence:''}},
  '/api/v2/gaffer/goals':{goals:[],readAt:now},
  '/api/v2/gaffer/conversations':request=>new URL(request.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
  '/api/v2/terminal/sessions':{sessions:[]},
  '/api/v2/gaffer/pathways':{pathways:[]},
  '/api/v2/gaffer/work-orders/check':request=>{
   const {text}=request.postDataJSON();box.checks.push(text)
   if(text===yoloText)return yoloCheck
   if(text===missingText)return missingCheck
   return goodCheck
  },
  '/api/v2/gaffer/outcomes':request=>{
   if(request.method()!=='POST')return {outcomes:[]}
   const input=request.postDataJSON();box.approved.push(input)
   return {status:201,json:{id:'e'.repeat(32),revision:1,objective:input.objective,state:'starting',note:'',agentId:'a1',projectId:input.projectId,projectName:'reports-api',dir:'/work/reports-api',tool:input.tool,model:input.model,providerId:pid,images:[],maxReviews:input.maxReviews,hours:input.hours,autoMerge:input.autoMerge,sessions:input.sessions.map((s,n)=>({key:'abcde'[n],title:s.title,prompt:s.prompt,requestId:String(n).repeat(32),state:'waiting',note:'Waiting its turn.',updatedAt:now,stage:0,stages:(s.stages??[]).map(stage=>({...stage,state:'pending',requestId:'r'}))})),createdAt:now,updatedAt:now}}
  },
 }
 return {box,routes}
}
async function openOutcome(w){
 await w.page.goto(base+'/factory')
 await w.page.getByRole('button',{name:'New Factory outcome',exact:true}).click()
 return w.page.getByRole('dialog',{name:'New Factory outcome',exact:true})
}

try {
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,terminalMachines:machines,routes:f.routes})
  try {
   const outcome=await openOutcome(w)
   const objective=outcome.getByRole('textbox',{name:'Outcome',exact:true})
   // An ordinary objective keeps the ordinary buttons: nothing about a work order is said.
   await objective.fill('Ship CSV export for billing.')
   await expect(outcome.getByText('This is a work order.',{exact:false})).toHaveCount(0)
   await expect(outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true})).toBeVisible()
   // Pasting the work order is recognised at once: the primary button changes and "Write the plan myself" goes.
   await objective.fill(goodText)
   await expect(outcome.getByText('This is a work order.',{exact:false})).toBeVisible()
   const read=outcome.getByRole('button',{name:'Read the work order',exact:true})
   await expect(read).toBeVisible();await expect(read).toBeEnabled()
   await expect(outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true})).toHaveCount(0)
   await expect(outcome.getByRole('button',{name:'Write the plan myself',exact:true})).toHaveCount(0)
   await outcome.screenshot({path:`${shots}/work-order-objective-${width}-${theme}.png`})
   await read.click()
   // The check asked the server, nothing else: the plan step opens pre-filled from its answer.
   expect(f.box.checks).toEqual([goodText])
   const steps=outcome.getByRole('list',{name:'Steps',exact:true})
   await expect(steps.locator('[aria-current="step"]')).toHaveText('2Plan')
   await expect(outcome.getByText('No pathway is named "none"; choose one yourself.',{exact:true})).toBeVisible()
   const plan=outcome.getByRole('list',{name:'Planned sessions',exact:true})
   await expect(plan.locator(':scope > li')).toHaveCount(1)
   await expect(outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('Export endpoint')
   const tiers=outcome.getByRole('group',{name:'Pipeline 1 stage tiers',exact:true})
   // Two code parts, both heavy; review on standard, as the work order's tiers line asked.
   const split=tiers.getByRole('group',{name:'Code parts in pipeline 1',exact:true})
   await expect(split.getByRole('group',{name:/^Part \d of Code in pipeline 1$/})).toHaveCount(2)
   await expect(split).toContainText('Suggested by the work order')
   await expect(tiers.locator('[data-stage=review]').getByRole('combobox',{name:'Review tier in pipeline 1',exact:true})).toHaveValue('standard')
   await expect(tiers.locator('[data-stage=review]')).toContainText('Suggested by the work order')
   // Both acceptance criteria travel with the pipeline.
   await expect(outcome.getByRole('textbox',{name:'Criterion 1 for pipeline 1',exact:true})).toHaveValue('An export of 10,000 rows finishes in under 5 seconds')
   await expect(outcome.getByRole('textbox',{name:'Criterion 2 for pipeline 1',exact:true})).toHaveValue("A reader of another team's report is refused")
   await outcome.screenshot({path:`${shots}/work-order-plan-${width}-${theme}.png`})
   expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
   // The crew editor can restore the saved crew (empty here) over what the work order suggested.
   await outcome.getByText('Choose who does what',{exact:true}).click()
   const useSaved=outcome.getByRole('button',{name:'Use my saved crew',exact:true})
   await expect(useSaved).toBeVisible()
   await useSaved.click()
   await expect(outcome.getByRole('combobox',{name:'Heavy agent',exact:true})).toHaveValue('claude')
   await expect(tiers.locator('[data-stage=review]')).not.toContainText('Suggested by the work order')
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // A suggested agent this machine does not run cannot start, and Approve says why.
 {
  const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
  try {
   const outcome=await openOutcome(w)
   await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(missingText)
   await outcome.getByRole('button',{name:'Read the work order',exact:true}).click()
   const tiers=outcome.getByRole('group',{name:'Pipeline 1 stage tiers',exact:true}).locator('[data-stage=code]')
   await expect(tiers).toContainText('Cannot start')
   await expect(tiers).toContainText('Suggested by the work order')
   const approve=outcome.getByRole('button',{name:/^Approve and start/})
   await expect(approve).toBeDisabled()
   await expect(outcome.getByText('is not on',{exact:false}).first()).toBeVisible()
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // A work order the format refuses never leaves the first step, and says which line and why.
 {
  const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
  try {
   const outcome=await openOutcome(w)
   await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(yoloText)
   await outcome.getByRole('button',{name:'Read the work order',exact:true}).click()
   await expect(outcome.getByText(/^Line 4: /,{exact:false})).toBeVisible()
   const steps=outcome.getByRole('list',{name:'Steps',exact:true})
   await expect(steps.locator('[aria-current="step"]')).toHaveText('1Outcome')
   await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true})).toHaveCount(0)
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // Approve posts the suggested agent for the code stage, and sets no yolo, trust or auto-merge.
 {
  const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
  try {
   const outcome=await openOutcome(w)
   await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(goodText)
   await outcome.getByRole('button',{name:'Read the work order',exact:true}).click()
   const approve=outcome.getByRole('button',{name:/^Approve and start/})
   await expect(approve).toBeEnabled()
   await approve.click()
   await expect(outcome.getByRole('status')).toHaveText(/^Starting session \d of 1\./)
   expect(f.box.approved).toHaveLength(1)
   const sent=f.box.approved[0]
   expect(sent.yolo).toBe(false);expect(sent.trust).toBe(false);expect(sent.autoMerge).toBe(false)
   const codeStages=sent.sessions[0].stages.filter(stage=>stage.kind==='code')
   expect(codeStages).toHaveLength(2)
   expect(codeStages.every(stage=>stage.tool==='claude')).toBe(true)
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 console.log('work-order.e2e.mjs: all assertions passed')
} finally { await browser.close() }
