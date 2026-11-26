import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Choosing a pathway when starting work: in the new-outcome dialog's plan and in Add pipelines. "Standard stages" (and no
// pathways at all, or a pathways read that fails) sends exactly the body the dialogs sent before pathways existed.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-pathways-choice'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),oid='e'.repeat(32)
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const machines=[{agentId:'a1',host:'workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,inspect:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:[]}]}}]
const project={id:'p1',host:'a1',machine:'workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:0,version:1,createdAt:now,updatedAt:now}

// Pathways as /api/v2/gaffer/pathways answers them (internal/api/gaffer_pathways.go): two to choose from, one that cannot
// start (shown, but not chosen), and one switched off (offered nowhere).
const st=(kind,extra={})=>({kind,...(kind==='custom'?{}:{tier:'standard'}),...extra})
const pathway=(id,name,stages,extra={})=>({id,name,description:'',enabled:true,stages,createdAt:now-86400000,updatedAt:now-3600000,...extra})
const PATHWAYS=[
 pathway('p1','Quick fix',[st('code'),st('pr'),st('merge')]),
 pathway('p2','Careful release',[st('code'),st('custom',{name:'Security pass',prompt:'Look for secrets.'}),st('review'),st('pr'),st('merge')]),
 pathway('p3','Billing hotfix',[st('code',{skills:['lint-rules']}),st('pr')],{problems:['The skill “lint-rules” on Code is no longer on your shelf. Work cannot start on this pathway until it is replaced.']}),
 pathway('p4','Docs only',[st('custom',{name:'Write docs',prompt:'Write the docs.'}),st('pr')],{enabled:false}),
]
const STANDARD='Standard stages'

// What each dialog sent before pathways existed, for the same steps (captured on the code before this change, request ID left out).
const BASELINE_OUTCOME={"objective":"Ship CSV export for billing.","projectId":"p1","tool":"claude","model":"","provider":"","trust":false,"yolo":false,"autoReport":false,"automation":false,"balanceBelow":20,"autoMerge":false,"maxReviews":12,"hours":24,"confirmed":true,"sessions":[{"title":"Whole export","prompt":"Build the CSV export end to end. Raise a pull request. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"review","tool":"claude","model":"","tier":"heavy"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"babysit","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]},{"title":"Button","prompt":"Add the export button. Raise a pull request. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"review","tool":"claude","model":"","tier":"heavy"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"babysit","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]}]}
const BASELINE_ADD={"revision":3,"confirmed":true,"sessions":[{"title":"Changelog entry","prompt":"Add a changelog entry. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"}]}]}

// `pathways` is what the list read answers: an array, {status, json} for a failure, or null for the helper's own answer (none). `pathwayId` marks the outcome's
// first pipeline as started from that pathway. `refuse` is a sentence the next start is refused with.
function fixture({pathways=PATHWAYS,pathwayId}={}){
 const box={starts:[],adds:[],refuse:''}
 const session={key:'a',title:'Export API',prompt:'p',state:'done',note:'Finished.',sessionId:'s-a',conversationId:'a'.repeat(32),sessionState:'ended',updatedAt:now-3600000,stage:1,stages:[{kind:'code',tool:'claude',model:'',state:'done',requestId:'r'}],...(pathwayId?{pathwayId,pathwayName:'Quick fix'}:{})}
 const outcome={id:oid,revision:3,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now-3600000,sessions:[session],
  report:{text:'Nothing covers the changelog.',at:now,model:'gaffer-model',inputTokens:1,outputTokens:1,usageReported:true,proposals:[{title:'Changelog entry',prompt:'Add a changelog entry. Do not merge.',why:'No pipeline covers it.',stages:['code','pr']}]}}
 const refused=()=>{const error=box.refuse;box.refuse='';return{status:400,json:{error,code:'invalid',field:'pathway'}}}
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':{conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[]},
  '/api/v2/terminal/machines':{machines},
  '/api/v2/folder-projects':{projects:[project],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
  '/api/v2/gaffer/outcomes':r=>{
   if(r.method()!=='POST')return{outcomes:[outcome]}
   const b=r.postDataJSON();box.starts.push(b)
   if(box.refuse)return refused()
   return{status:201,json:{...outcome,id:'d'.repeat(32),state:'starting',report:undefined,sessions:[{key:'a',title:b.sessions[0].title,prompt:'p',state:'starting',note:'Starting.',updatedAt:now}]}}
  },
  [`/api/v2/gaffer/outcomes/${'d'.repeat(32)}`]:()=>({...outcome,id:'d'.repeat(32),state:'running',report:undefined}),
  [`/api/v2/gaffer/outcomes/${oid}/sessions`]:r=>{const b=r.postDataJSON();box.adds.push(b);if(box.refuse)return refused();return{...outcome,revision:4,report:{...outcome.report,proposals:[]}}},
 }
 if(pathways!==null)routes['/api/v2/gaffer/pathways']=Array.isArray(pathways)?{pathways}:pathways
 return {box,routes}
}
const open=async(width,theme,over={})=>{
 const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,terminalMachines:machines,routes:f.routes})
 return {f,w,page:w.page}
}
const shot=async(page,name,width,theme)=>{await page.mouse.move(0,0);await page.waitForTimeout(300);await page.screenshot({path:`${shots}/choice-${name}-${width}-${theme}.png`})}
const noRequestId=({requestId:_,...rest})=>rest
const dialogFits=async page=>expect(await page.locator('dialog[open] [data-shell-body]').last().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
// Scrolls the open dialog's body so `el` is at its top, for a capture.
const bringUp=async el=>el.evaluate(t=>{const body=t.closest('[data-shell-body]');body.scrollTop+=t.getBoundingClientRect().top-body.getBoundingClientRect().top-8})

// The new-outcome dialog, as far as its plan: a goal, the project, one pipeline written by hand.
async function toPlan(page){
 await page.goto(base+'/factory')
 await page.getByRole('button',{name:'New Factory outcome',exact:true}).click()
 const outcome=page.getByRole('dialog',{name:'New Factory outcome',exact:true})
 await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill('Ship CSV export for billing.')
 await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p1')
 await outcome.getByRole('button',{name:'Write the plan myself',exact:true}).click()
 await outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true}).fill('Whole export')
 await outcome.getByRole('textbox',{name:'Pipeline 1 prompt',exact:true}).fill('Build the CSV export end to end. Raise a pull request. Do not merge.')
 await outcome.getByRole('button',{name:'Add a pipeline',exact:true}).click()
 await outcome.getByRole('textbox',{name:'Pipeline 2 title',exact:true}).fill('Button')
 await outcome.getByRole('textbox',{name:'Pipeline 2 prompt',exact:true}).fill('Add the export button. Raise a pull request. Do not merge.')
 return outcome
}
async function startOutcome(page,f,outcome){
 await outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true}).click()
 await expect.poll(()=>f.box.starts.length).toBe(1)
 return noRequestId(f.box.starts[0])
}
async function toAdd(page){
 await page.goto(`${base}/factory?pipeline=${oid}.a`)
 const report=page.locator('details.row').filter({has:page.locator('summary .label').getByText('Gaffer’s report',{exact:true})})
 if(await report.getAttribute('open')===null)await report.locator('summary').click()
 await report.getByRole('button',{name:'Add 1 pipeline',exact:true}).click()
 const add=page.getByRole('dialog',{name:'Add “Changelog entry”?',exact:true})
 await expect(add.getByRole('button',{name:'Add pipeline',exact:true})).toBeEnabled()
 return add
}
async function addOne(page,f,add){
 await add.getByRole('button',{name:'Add pipeline',exact:true}).click()
 await expect.poll(()=>f.box.adds.length).toBe(1)
 return f.box.adds[0]
}
const same=(sent,baseline,what)=>{
 if(baseline===null){console.log(`${what} baseline:`,JSON.stringify(sent));throw new Error(`No baseline for ${what} yet: copy the line above into the suite.`)}
 expect(JSON.stringify(sent)).toBe(JSON.stringify(baseline))
}

try{
 // 1. No pathways (the read answers 404, as in a fixture that does not know them, or fails): the dialogs have no choice
 //    and send what they sent before.
 for(const pathways of [null,{status:404,json:{error:'Not found'}},{status:502,json:{error:'502 Bad Gateway'}},[PATHWAYS[3]]]){
  const {f,w,page}=await open(1280,'light',{pathways})
  try{
   const outcome=await toPlan(page)
   await expect(outcome.getByRole('radiogroup',{name:'Pathway',exact:true})).toHaveCount(0)
   await expect(outcome.getByText('Pathway',{exact:true})).toHaveCount(0)
   same(await startOutcome(page,f,outcome),BASELINE_OUTCOME,'outcome')
   const add=await toAdd(page)
   await expect(add.getByRole('radiogroup',{name:'Pathway',exact:true})).toHaveCount(0)
   same(await addOne(page,f,add),BASELINE_ADD,'add')
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }

 for(const [width,theme] of [[1280,'light'],[1280,'dark'],[390,'light'],[390,'dark']]){
  // 2. The new-outcome dialog: the enabled pathways, one that cannot start shown with why, and Standard stages chosen.
  {
   const {f,w,page}=await open(width,theme)
   try{
    const outcome=await toPlan(page)
    const group=outcome.getByRole('radiogroup',{name:'Pathway',exact:true})
    const radios=group.getByRole('radio')
    await expect(radios).toHaveCount(4)
    await expect(group.getByRole('radio',{name:/^Quick fix/})).toBeEnabled()
    await expect(group.getByRole('radio',{name:/^Careful release/})).toBeEnabled()
    await expect(group.getByRole('radio',{name:/^Billing hotfix/})).toBeDisabled()
    await expect(group).toContainText('The skill “lint-rules” on Code is no longer on your shelf.')
    await expect(group.getByText('Docs only')).toHaveCount(0)
    await expect(group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
    await expect(group).toContainText('Choose the stages, tiers and parts yourself, as now.')
    await expect(outcome.getByRole('link',{name:'Edit pathway',exact:true})).toHaveCount(0)
    await expect(outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})).toBeVisible()
    await dialogFits(page)
    await bringUp(group.locator('..'));await shot(page,'outcome-standard',width,theme)

    // Choosing a pathway: the stage ticks, tiers and crew give way to its stage line and one sentence; everything else stays.
    await group.getByRole('radio',{name:/^Careful release/}).check()
    await expect(outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})).toHaveCount(0)
    await expect(outcome.getByRole('group',{name:'Pipeline 1 stage tiers',exact:true})).toHaveCount(0)
    await expect(outcome.getByText('Each pipeline follows this pathway. Its stages, models, skills and prompts come from it.',{exact:true})).toHaveCount(2)
    await expect(outcome.getByRole('list',{name:'Stages of Careful release'})).toHaveCount(3)
    await expect(outcome.getByRole('list',{name:'Stages of Careful release'}).nth(1)).toHaveText(/Code.*Security pass.*Review.*PR.*Merge/s)
    const edit=outcome.getByRole('link',{name:'Edit pathway',exact:true})
    await expect(edit).toHaveAttribute('href','/factory/pathways/p2')
    await expect(edit).toHaveAttribute('target','_blank')
    await expect(outcome.getByRole('textbox',{name:'Pipeline 1 prompt',exact:true})).toBeVisible()
    await dialogFits(page)
    await bringUp(group.locator('..'));await shot(page,'outcome-pathway',width,theme)
    // Arrow keys move the choice, as radios do.
    await group.getByRole('radio',{name:/^Careful release/}).focus()
    await page.keyboard.press('ArrowUp')
    await expect(group.getByRole('radio',{name:/^Quick fix/})).toBeChecked()
    await page.keyboard.press('ArrowDown')
    await expect(group.getByRole('radio',{name:/^Careful release/})).toBeChecked()

    if(width===1280&&theme==='light'){
     // A refusal about the pathway is said where a start error is.
     f.box.refuse='The Careful release pathway is switched off. Switch it on to use it.'
     await outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true}).click()
     await expect(outcome.getByRole('alert')).toHaveText('The Careful release pathway is switched off. Switch it on to use it.')
     const sent=noRequestId(f.box.starts[0])
     expect(sent.pathway).toBe('p2')
     for(const s of sent.sessions)expect(s).not.toHaveProperty('stages')
     expect(sent.sessions).toEqual([{title:'Whole export',prompt:'Build the CSV export end to end. Raise a pull request. Do not merge.'},{title:'Button',prompt:'Add the export button. Raise a pull request. Do not merge.'}])
     // Back to Standard stages: the body is the one sent before pathways existed.
     await group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)}).check()
     await expect(outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})).toBeVisible()
     f.box.starts.length=0
     same(await startOutcome(page,f,outcome),BASELINE_OUTCOME,'outcome')
    }
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // 3. Add pipelines starts on the pathway the outcome's first pipeline used, and sends it top-level.
  {
   const {f,w,page}=await open(width,theme,{pathwayId:'p1'})
   try{
    const add=await toAdd(page)
    const group=add.getByRole('radiogroup',{name:'Pathway',exact:true})
    await expect(group.getByRole('radio',{name:/^Quick fix/})).toBeChecked()
    await expect(add.getByRole('combobox',{name:'Agent for Changelog entry',exact:true})).toHaveCount(0)
    await expect(add.getByText('Each pipeline follows this pathway. Its stages, models, skills and prompts come from it.',{exact:true})).toBeVisible()
    await dialogFits(page)
    await shot(page,'add-pathway',width,theme)
    if(width===1280&&theme==='light'){
     const sent=await addOne(page,f,add)
     expect(sent).toEqual({revision:3,confirmed:true,pathway:'p1',sessions:[{title:'Changelog entry',prompt:'Add a changelog entry. Do not merge.'}]})
    }
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // 4. Add pipelines for an outcome started on standard stages: Standard stages is chosen and the body is as before.
  {
   const {f,w,page}=await open(width,theme)
   try{
    const add=await toAdd(page)
    const group=add.getByRole('radiogroup',{name:'Pathway',exact:true})
    await expect(group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
    await expect(add.getByRole('combobox',{name:'Agent for Changelog entry',exact:true})).toBeVisible()
    await dialogFits(page)
    await shot(page,'add-standard',width,theme)
    if(width===1280&&theme==='light')same(await addOne(page,f,add),BASELINE_ADD,'add')
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
 }
 // 5. A pathway the outcome used that has since been switched off is not preselected.
 {
  const {f,w,page}=await open(1280,'light',{pathwayId:'p4'})
  try{
   const add=await toAdd(page)
   await expect(add.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
   same(await addOne(page,f,add),BASELINE_ADD,'add')
  } finally { await w.context.close() }
 }
 console.log('pathways-choice: ok')
} finally { await browser.close() }
