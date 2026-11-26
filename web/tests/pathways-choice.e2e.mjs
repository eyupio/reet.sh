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
const ADAPTERS={claude:'claude-agent-acp',codex:'codex-acp',opencode:'opencode',antigravity:'agy'}
// The machine, with the agents it offers (Claude Code unless a test says otherwise).
const machineWith=(tools=['claude'],ready=true,yolo=false)=>({agentId:'a1',host:'workshop',ready,serverEnabled:true,capabilities:{protocolVersion:1,...(yolo?{yolo:true}:{}),os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,inspect:true,acpAgents:tools.map(tool=>({tool,adapter:ADAPTERS[tool],state:'available',options:[]}))}})
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
// Pathways whose stages name their own agent, for the readiness and trust checks.
const AUDIT=pathway('p5','Audit',[st('code'),st('custom',{name:'Security audit',prompt:'Look for secrets.',agent:{tool:'codex'}}),st('pr')])
const PLUGINS=pathway('p6','Plugin pass',[st('custom',{name:'Plugin check',prompt:'Check the plugins.',agent:{tool:'opencode'}}),st('pr')])
const GRAVITY=pathway('p7','Gravity pass',[st('custom',{name:'Gravity check',prompt:'Check it.',agent:{tool:'antigravity'}}),st('pr')])
const savedCrew=crew=>({crew:{light:{},standard:{},heavy:{},...crew},saved:true,tiers:['light','standard','heavy'],defaults:{}})

// What each dialog sent before pathways existed, for the same steps (captured on the code before this change, request ID left out).
const BASELINE_OUTCOME={"objective":"Ship CSV export for billing.","projectId":"p1","tool":"claude","model":"","provider":"","trust":false,"yolo":false,"autoReport":false,"automation":false,"balanceBelow":20,"autoMerge":false,"maxReviews":12,"hours":24,"confirmed":true,"sessions":[{"title":"Whole export","prompt":"Build the CSV export end to end. Raise a pull request. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"review","tool":"claude","model":"","tier":"heavy"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"babysit","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]},{"title":"Button","prompt":"Add the export button. Raise a pull request. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"review","tool":"claude","model":"","tier":"heavy"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"babysit","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]}]}
// The same, with the edits under Standard stages of section 7 (captured on the code before pathways existed).
const BASELINE_EDITED={"objective":"Ship CSV export for billing.","projectId":"p1","tool":"claude","model":"","provider":"","trust":false,"yolo":false,"autoReport":false,"automation":false,"balanceBelow":20,"autoMerge":false,"maxReviews":12,"hours":24,"confirmed":true,"sessions":[{"title":"Whole export","prompt":"Build the CSV export end to end. Raise a pull request. Do not merge.","stages":[{"kind":"plan","tool":"claude","model":"","tier":"heavy"},{"kind":"code","tool":"claude","model":"","tier":"standard","part":"Endpoint","brief":"Add the endpoint."},{"kind":"code","tool":"claude","model":"","tier":"standard","part":"Tests","brief":"Test the endpoint."},{"kind":"review","tool":"claude","model":"","tier":"heavy"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]},{"title":"Button","prompt":"Add the export button. Raise a pull request. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"review","tool":"claude","model":"","tier":"light"},{"kind":"test","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"},{"kind":"babysit","tool":"claude","model":"","tier":"light"},{"kind":"merge","tool":"claude","model":"","tier":"light"}]}]}
const BASELINE_ADD={"revision":3,"confirmed":true,"sessions":[{"title":"Changelog entry","prompt":"Add a changelog entry. Do not merge.","stages":[{"kind":"code","tool":"claude","model":"","tier":"standard"},{"kind":"pr","tool":"claude","model":"","tier":"light"}]}]}

// `pathways` is what the list read answers: an array, {status, json} for a failure, or null for the helper's own answer (none). `pathwayId` marks the outcome's
// first pipeline as started from that pathway. `refuse` is a sentence the next start is refused with.
// `tools` are the agents the machine offers, `crew` the saved crew's tiers and `accounts` the machine's sign-ins. `yolo` lets the
// account and the machine use YOLO. `box.pathways` can be changed to answer the next read differently.
function fixture({pathways=PATHWAYS,pathwayId,tools,crew,accounts,yolo=false}={}){
 const box={starts:[],adds:[],refuse:'',machines:[machineWith(tools,true,yolo)],pathways}
 const session={key:'a',title:'Export API',prompt:'p',state:'done',note:'Finished.',sessionId:'s-a',conversationId:'a'.repeat(32),sessionState:'ended',updatedAt:now-3600000,stage:1,stages:[{kind:'code',tool:'claude',model:'',state:'done',requestId:'r'}],...(pathwayId?{pathwayId,pathwayName:'Quick fix'}:{})}
 const outcome={id:oid,revision:3,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now-3600000,sessions:[session],
  report:{text:'Nothing covers the changelog.',at:now,model:'gaffer-model',inputTokens:1,outputTokens:1,usageReported:true,proposals:[{title:'Changelog entry',prompt:'Add a changelog entry. Do not merge.',why:'No pipeline covers it.',stages:['code','pr']}]}}
 const refused=()=>{const error=box.refuse;box.refuse='';return{status:400,json:{error,code:'invalid',field:'pathway'}}}
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':{conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[]},
  '/api/v2/terminal/machines':()=>({machines:box.machines}),
  '/api/v2/folder-projects':{projects:[project],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:yolo},
  '/api/v2/gaffer/outcomes':r=>{
   if(r.method()!=='POST')return{outcomes:[outcome]}
   const b=r.postDataJSON();box.starts.push(b)
   if(box.refuse)return refused()
   return{status:201,json:{...outcome,id:'d'.repeat(32),state:'starting',report:undefined,sessions:[{key:'a',title:b.sessions[0].title,prompt:'p',state:'starting',note:'Starting.',updatedAt:now}]}}
  },
  [`/api/v2/gaffer/outcomes/${'d'.repeat(32)}`]:()=>({...outcome,id:'d'.repeat(32),state:'running',report:undefined}),
  [`/api/v2/gaffer/outcomes/${oid}/sessions`]:r=>{const b=r.postDataJSON();box.adds.push(b);if(box.refuse)return refused();return{...outcome,revision:4,report:{...outcome.report,proposals:[]}}},
 }
 if(crew)routes['/api/v2/gaffer/crew']=savedCrew(crew)
 if(accounts)routes['/api/v2/machines/a1/accounts']={state:'read',sentence:'',planLimits:false,checkedAt:now,accounts}
 if(pathways!==null)routes['/api/v2/gaffer/pathways']=Array.isArray(pathways)?()=>({pathways:box.pathways}):pathways
 return {box,routes}
}
const open=async(width,theme,over={})=>{
 const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,terminalMachines:f.box.machines,routes:f.routes})
 return {f,w,page:w.page}
}
const shot=async(page,name,width,theme)=>{await page.mouse.move(0,0);await page.waitForTimeout(300);await page.screenshot({path:`${shots}/choice-${name}-${width}-${theme}.png`})}
const noRequestId=({requestId:_,...rest})=>rest
const dialogFits=async page=>expect(await page.locator('dialog[open] [data-shell-body]').last().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
// Scrolls the open dialog's body so `el` is at its top, for a capture.
const bringUp=async el=>el.evaluate(t=>{const body=t.closest('[data-shell-body]');body.scrollTop+=t.getBoundingClientRect().top-body.getBoundingClientRect().top-8})

// The new-outcome dialog, as far as its plan: a goal, the project, one pipeline written by hand.
// `before` runs once the goal is written, just before the plan opens.
async function toPlan(page,before){
 await page.goto(base+'/factory')
 await page.getByRole('button',{name:'New Factory outcome',exact:true}).click()
 const outcome=page.getByRole('dialog',{name:'New Factory outcome',exact:true})
 await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill('Ship CSV export for billing.')
 await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p1')
 if(before)await before()
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
 await expect(add).toBeVisible()
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
    // It sits in the chosen pathway's own row.
    expect(await edit.evaluate(a=>a.closest('.row')?.querySelector('input:checked')?.value)).toBe('p2')
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
 // 6. Nor one that has a problem now.
 {
  const {f,w,page}=await open(1280,'light',{pathwayId:'p3'})
  try{
   const add=await toAdd(page)
   await expect(add.getByRole('radio',{name:/^Billing hotfix/})).toBeDisabled()
   await expect(add.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
   same(await addOne(page,f,add),BASELINE_ADD,'add')
  } finally { await w.context.close() }
 }
 // 7. Edits under Standard stages (a stage unticked and one ticked, a tier changed, a stage split into parts) survive a
 //    trip to a pathway and back, and send what the same edits send with no pathways at all.
 {
  const edit=async outcome=>{
   const one=outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})
   await one.getByRole('button',{name:'Babysit',exact:true}).click()
   await one.getByRole('button',{name:'Plan',exact:true}).click()
   await outcome.getByRole('combobox',{name:'Review tier in pipeline 2',exact:true}).selectOption('light')
   await outcome.getByRole('button',{name:'Split Code in pipeline 1 into parts',exact:true}).click()
   const split=outcome.getByRole('group',{name:'Code parts in pipeline 1',exact:true})
   for(const [n,name,brief] of [[1,'Endpoint','Add the endpoint.'],[2,'Tests','Test the endpoint.']]){
    const part=split.getByRole('group',{name:`Part ${n} of Code in pipeline 1`,exact:true})
    await part.getByRole('textbox',{name:`Part ${n} name`,exact:true}).fill(name)
    await part.getByRole('textbox',{name:`Part ${n} brief`,exact:true}).fill(brief)
   }
  }
  const sent=[]
  for(const pathways of [null,PATHWAYS]){
   const {f,w,page}=await open(1280,'light',{pathways})
   try{
    const outcome=await toPlan(page)
    await edit(outcome)
    if(pathways){
     const group=outcome.getByRole('radiogroup',{name:'Pathway',exact:true})
     await group.getByRole('radio',{name:/^Quick fix/}).check()
     await expect(outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})).toHaveCount(0)
     await group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)}).check()
    }
    sent.push(await startOutcome(page,f,outcome))
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  same(sent[0],BASELINE_EDITED,'edited')
  expect(JSON.stringify(sent[1])).toBe(JSON.stringify(sent[0]))
 }
 // 8. With a pathway, Approve waits for what its own stages need, and only for that.
 {
  const approve=outcome=>outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true})
  const status=outcome=>outcome.locator('p.status')
  const readiness=outcome=>outcome.getByRole('region',{name:'Launch readiness',exact:true})
  const choose=(outcome,name)=>outcome.getByRole('radiogroup',{name:'Pathway',exact:true}).getByRole('radio',{name:new RegExp(`^${name}`)}).check()
  // (a) The machine is not connected: every stage is held, with the machine's own sentence.
  {
   const {f,w,page}=await open(1280,'light')
   try{
    const outcome=await toPlan(page,()=>{f.box.machines=[machineWith(['claude'],false)]})
    await choose(outcome,'Quick fix')
    await expect(status(outcome)).toHaveText('The machine is reconnecting. Existing shells will be kept.')
    await expect(approve(outcome)).toBeDisabled()
    await expect(readiness(outcome)).toContainText('Every stage')
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // (b) A tier stage runs on the saved crew's agent for its tier: a missing sign-in there is said, for that stage, with
  //     no crew to change. As under Standard stages, it is a warning: a sign-in the machine cannot see may still work.
  {
   const {f,w,page}=await open(1280,'light',{tools:['claude','codex'],crew:{standard:{tool:'codex'}},accounts:[{tool:'codex',state:'none',limitsState:'unsupported'}]})
   try{
    const outcome=await toPlan(page)
    await choose(outcome,'Quick fix')
    const cause=readiness(outcome).getByRole('listitem').filter({hasText:'No Codex sign-in was found on workshop.'})
    await expect(cause).toContainText('Used by Code, PR and Merge.')
    await expect(cause.getByRole('button')).toHaveCount(0)
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // (c) A stage that names its own agent needs that agent on the machine; the sentence says which stage and what to do.
  {
   const {f,w,page}=await open(1280,'light',{pathways:[...PATHWAYS,AUDIT]})
   try{
    const outcome=await toPlan(page)
    await choose(outcome,'Audit')
    await expect(status(outcome)).toHaveText('Codex is not on workshop now. This pathway’s Security audit stage runs on it. Install it there, or change the stage in the pathway.')
    await expect(approve(outcome)).toBeDisabled()
    await expect(outcome.getByRole('link',{name:'Edit pathway',exact:true})).toHaveAttribute('href','/factory/pathways/p5')
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // A crew agent the pathway never uses holds nothing: Google Antigravity on the Heavy tier holds Standard stages
  // (Review is Heavy) until trust is ticked, but not Quick fix, which has no Heavy stage.
  {
   const {f,w,page}=await open(1280,'light',{tools:['claude','antigravity'],crew:{heavy:{tool:'antigravity'}}})
   try{
    const outcome=await toPlan(page)
    await expect(status(outcome)).toContainText('Google Antigravity starts only in a folder you trust.')
    await expect(approve(outcome)).toBeDisabled()
    await choose(outcome,'Quick fix')
    await expect(approve(outcome)).toBeEnabled()
    await expect(outcome.getByRole('checkbox',{name:/^I trust this folder/})).toHaveCount(0)
    await approve(outcome).click()
    await expect.poll(()=>f.box.starts.length).toBe(1)
    expect(f.box.starts[0].pathway).toBe('p1')
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // A stage on an agent that needs folder trust shows the trust box, and Antigravity holds the start until it is ticked.
  {
   const {f,w,page}=await open(1280,'light',{pathways:[...PATHWAYS,PLUGINS,GRAVITY],tools:['claude','opencode','antigravity']})
   try{
    const outcome=await toPlan(page)
    await expect(outcome.getByRole('checkbox',{name:/^I trust this folder/})).toHaveCount(0)
    await choose(outcome,'Plugin pass')
    await expect(outcome.getByRole('checkbox',{name:/^I trust this folder: OpenCode may run/})).toBeVisible()
    await expect(approve(outcome)).toBeEnabled()
    await choose(outcome,'Gravity pass')
    await expect(status(outcome)).toHaveText('Google Antigravity starts only in a folder you trust. Tick the folder trust box below, or change the stage in the pathway.')
    await outcome.getByRole('checkbox',{name:/^I trust this folder/}).check()
    await expect(approve(outcome)).toBeEnabled()
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // Add pipelines: a stage's own agent that is not on the machine, and Antigravity on an outcome approved without trust,
  // hold the add; a crew agent the pathway never uses does not.
  {
   const {f,w,page}=await open(1280,'light',{pathways:[...PATHWAYS,AUDIT,GRAVITY],tools:['claude','antigravity'],crew:{heavy:{tool:'antigravity'}}})
   try{
    const add=await toAdd(page)
    const button=add.getByRole('button',{name:'Add pipeline',exact:true})
    const group=add.getByRole('radiogroup',{name:'Pathway',exact:true})
    await group.getByRole('radio',{name:/^Audit/}).check()
    await expect(add.getByRole('alert')).toHaveText('This pathway’s Security audit stage runs on Codex, which is not on workshop now. Install it there, or change the stage in the pathway.')
    await expect(button).toBeDisabled()
    await group.getByRole('radio',{name:/^Gravity pass/}).check()
    await expect(add.getByRole('alert')).toContainText('This pathway’s Gravity check stage runs on it: change that stage in the pathway, or choose standard stages.')
    await expect(button).toBeDisabled()
    await group.getByRole('radio',{name:/^Quick fix/}).check()
    await expect(add.getByRole('alert')).toHaveCount(0)
    await expect(button).toBeEnabled()
    expect((await addOne(page,f,add)).pathway).toBe('p1')
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
 }
 // 9. Many pathways: the first six, then Standard stages; the rest behind "Show all", with a search once there are more
 //    than twelve rows. A chosen pathway further down stays in sight when the list is folded again.
 const MANY=Array.from({length:30},(_,n)=>pathway(`m${n+1}`,`Line ${n+1}`,[st('code'),st('pr')]))
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  const {f,w,page}=await open(width,theme,{pathways:MANY})
  try{
   const outcome=await toPlan(page)
   const group=outcome.getByRole('radiogroup',{name:'Pathway',exact:true})
   const names=()=>group.getByRole('radio').evaluateAll(els=>els.map(el=>el.closest('label').querySelector('strong').textContent))
   await expect.poll(names).toEqual(['Line 1','Line 2','Line 3','Line 4','Line 5','Line 6',STANDARD])
   await expect(group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
   await expect(outcome.getByRole('searchbox')).toHaveCount(0)
   const all=outcome.getByRole('button',{name:'Show all 30 pathways',exact:true})
   await dialogFits(page)
   await bringUp(group.locator('..'));await shot(page,'many-folded',width,theme)
   await all.click()
   await expect(group.getByRole('radio')).toHaveCount(31)
   await expect(group.getByRole('radio').last()).toHaveAccessibleName(new RegExp(`^${STANDARD}`))
   const search=outcome.getByRole('searchbox',{name:'Search pathways',exact:true})
   await expect(search).toBeFocused()
   await search.fill('Line 2')
   await expect.poll(names).toEqual(['Line 2',...Array.from({length:10},(_,n)=>`Line ${20+n}`),STANDARD])
   await group.getByRole('radio',{name:/^Line 27/}).check()
   await search.fill('')
   await dialogFits(page)
   await bringUp(group.locator('..'));await shot(page,'many-open',width,theme)
   await outcome.getByRole('button',{name:'Show fewer',exact:true}).click()
   await expect.poll(names).toEqual(['Line 27','Line 1','Line 2','Line 3','Line 4','Line 5',STANDARD])
   await expect(group.getByRole('radio',{name:/^Line 27/})).toBeChecked()
   await group.getByRole('radio',{name:/^Line 27/}).focus()
   await page.keyboard.press('ArrowDown')
   await expect(group.locator('label:has(input:checked) strong')).toHaveText('Line 1')
   if(width===1280)expect((await startOutcome(page,f,outcome)).pathway).toBe('m1')
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // 12. The folded list holds still: a chosen pathway lifted into it stays where it is when another row is chosen, until
 //     Show all or the dialog is reopened. Edit pathway is not part of the chosen radio's name, is the next Tab stop, and
 //     reaches 44 px on a touch screen while staying inside the chosen row.
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  const {w,page}=await open(width,theme,{pathways:MANY})
  try{
   const outcome=await toPlan(page)
   const group=outcome.getByRole('radiogroup',{name:'Pathway',exact:true})
   const names=()=>group.getByRole('radio').evaluateAll(els=>els.map(el=>el.closest('label').querySelector('strong').textContent))
   await outcome.getByRole('button',{name:'Show all 30 pathways',exact:true}).click()
   await group.getByRole('radio',{name:/^Line 27/}).check()
   await outcome.getByRole('button',{name:'Show fewer',exact:true}).click()
   await expect.poll(names).toEqual(['Line 27','Line 1','Line 2','Line 3','Line 4','Line 5',STANDARD])
   await group.getByRole('radio',{name:/^Line 3/}).check()
   expect(await names()).toEqual(['Line 27','Line 1','Line 2','Line 3','Line 4','Line 5',STANDARD])
   await group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)}).check()
   expect(await names()).toEqual(['Line 27','Line 1','Line 2','Line 3','Line 4','Line 5',STANDARD])
   await outcome.getByRole('button',{name:'Show all 30 pathways',exact:true}).click()
   await outcome.getByRole('button',{name:'Show fewer',exact:true}).click()
   await expect.poll(names).toEqual(['Line 1','Line 2','Line 3','Line 4','Line 5','Line 6',STANDARD])
   // Edit pathway.
   const radio=group.getByRole('radio',{name:/^Line 4/})
   await radio.check()
   await expect(radio).toHaveAccessibleName(/^Line 4/)
   expect(await radio.evaluate(el=>el.labels[0].textContent)).not.toContain('Edit pathway')
   const edit=group.getByRole('link',{name:'Edit pathway',exact:true})
   await expect(edit).toHaveCount(1)
   expect(await edit.evaluate(a=>!a.closest('label'))).toBe(true)
   await radio.focus()
   await page.keyboard.press('Tab')
   await expect(edit).toBeFocused()
   const [r,e]=[await radio.evaluate(el=>el.closest('.row').getBoundingClientRect().toJSON()),await edit.boundingBox()]
   expect(e.x>=r.x&&e.x+e.width<=r.x+r.width&&e.y>=r.y&&e.y+e.height<=r.y+r.height,'Edit pathway sits inside the chosen row').toBe(true)
   if(width<1000)expect(e.height).toBeGreaterThanOrEqual(44)
   await dialogFits(page)
   await bringUp(group.locator('..'));await shot(page,'edit-link',width,theme)
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // 10. A pathway switched off or changed in another tab (Edit pathway opens one) is read again when the window comes back:
 //     a choice no longer offered goes back to Standard stages and says so, and the body is the one sent before pathways.
 {
  const {f,w,page}=await open(1280,'light')
  try{
   const outcome=await toPlan(page)
   const group=outcome.getByRole('radiogroup',{name:'Pathway',exact:true})
   await group.getByRole('radio',{name:/^Careful release/}).check()
   f.box.pathways=PATHWAYS.map(p=>p.id==='p2'?{...p,enabled:false}:p.id==='p1'?{...p,stages:[st('code'),st('review'),st('pr'),st('merge')]}:p)
   await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
   await expect(group.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
   await expect(outcome.getByRole('status').filter({hasText:'Careful release is no longer on, so this is back on Standard stages.'})).toBeVisible()
   await expect(group.getByRole('radio',{name:/^Careful release/})).toHaveCount(0)
   await expect(group.getByRole('list',{name:'Stages of Quick fix'})).toHaveText(/Code.*Review.*PR.*Merge/s)
   await expect(outcome.getByRole('group',{name:'Pipeline 1 stages',exact:true})).toBeVisible()
   same(await startOutcome(page,f,outcome),BASELINE_OUTCOME,'outcome')
   // Add pipelines, on a page that comes back into view.
   f.box.pathways=PATHWAYS
   const {f:f2,w:w2,page:page2}=await open(1280,'light',{pathwayId:'p1'})
   try{
    const add=await toAdd(page2)
    await expect(add.getByRole('radio',{name:/^Quick fix/})).toBeChecked()
    f2.box.pathways=PATHWAYS.filter(p=>p.id!=='p1')
    await page2.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
    await expect(add.getByRole('radio',{name:new RegExp(`^${STANDARD}`)})).toBeChecked()
    await expect(add.getByRole('status').filter({hasText:'Quick fix is no longer on, so this is back on Standard stages.'})).toBeVisible()
    same(await addOne(page2,f2,add),BASELINE_ADD,'add')
    expect(w2.errors).toEqual([])
   } finally { await w2.context.close() }
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // 11. A folder-trust tick made under Standard stages survives a trip to a pathway that needs none; the YOLO check names
 //     the pathway's agents.
 {
  const {f,w,page}=await open(1280,'light',{tools:['claude','opencode'],crew:{heavy:{tool:'opencode'}},yolo:true})
  try{
   const outcome=await toPlan(page)
   const trust=outcome.getByRole('checkbox',{name:/^I trust this folder/})
   await trust.check()
   await outcome.getByRole('radiogroup',{name:'Pathway',exact:true}).getByRole('radio',{name:/^Quick fix/}).check()
   await expect(trust).toHaveCount(0)
   await outcome.getByRole('radio',{name:'YOLO',exact:true}).click()
   await expect(page.getByText('Every agent this pathway uses has a YOLO option',{exact:true})).toBeVisible()
   await page.getByRole('button',{name:'Keep asking me',exact:true}).click()
   await outcome.getByRole('radiogroup',{name:'Pathway',exact:true}).getByRole('radio',{name:new RegExp(`^${STANDARD}`)}).check()
   await expect(trust).toBeChecked()
   expect(w.errors).toEqual([])
  } finally { await w.context.close() }
 }
 // 13. A pathway's tier stage runs on the saved crew's agent for that tier, as the controller resolves it, even when the
 //     machine does not have it (the outcome's agent is used only for a tier that is not saved), and on that tier's
 //     backups too: they count for folder trust.
 {
  const approve=outcome=>outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true})
  const choose=(scope,name)=>scope.getByRole('radiogroup',{name:'Pathway',exact:true}).getByRole('radio',{name:new RegExp(`^${name}`)}).check()
  const MISSING='Your Standard crew is Codex, which is not on workshop. This pathway’s Code, PR and Merge stages run on it. Install or sign in to Codex there, change your saved crew in Gaffer settings, or change the stage in the pathway.'
  // (a) A saved Standard crew on an agent the machine does not have holds the start, naming the stages and the tier.
  {
   const {f,w,page}=await open(1280,'light',{crew:{standard:{tool:'codex'}}})
   try{
    const outcome=await toPlan(page)
    await choose(outcome,'Quick fix')
    await expect(outcome.locator('p.status')).toHaveText(MISSING)
    await expect(approve(outcome)).toBeDisabled()
    await expect(outcome.getByRole('region',{name:'Launch readiness',exact:true}).getByRole('listitem').filter({hasText:'Codex is not on workshop now.'})).toContainText('Used by Code, PR and Merge.')
    await expect(outcome.getByRole('link',{name:'Edit pathway',exact:true})).toHaveAttribute('href','/factory/pathways/p1')
    // Standard stages keep the crew editor's fallback to the outcome's agent.
    await choose(outcome,STANDARD)
    await expect(approve(outcome)).toBeEnabled()
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // (b) A saved tier whose backup is Google Antigravity, on a first choice that needs no trust: the trust box shows, Approve
  //     waits for it, and the start carries the trust.
  {
   const {f,w,page}=await open(1280,'light',{tools:['claude','antigravity'],crew:{standard:{tool:'claude',backup:{tool:'antigravity'}}}})
   try{
    const outcome=await toPlan(page)
    await choose(outcome,'Quick fix')
    const trust=outcome.getByRole('checkbox',{name:/^I trust this folder: Google Antigravity/})
    await expect(trust).toBeVisible()
    await expect(outcome.locator('p.status')).toHaveText('Google Antigravity starts only in a folder you trust. Tick the folder trust box below, or change the stage in the pathway.')
    await expect(approve(outcome)).toBeDisabled()
    await trust.check()
    await approve(outcome).click()
    await expect.poll(()=>f.box.starts.length).toBe(1)
    expect(f.box.starts[0].pathway).toBe('p1')
    expect(f.box.starts[0].trust).toBe(true)
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // (c) Add pipelines: the same saved crew agent that is not on the machine holds the add, with the same sentence.
  {
   const {f,w,page}=await open(1280,'light',{crew:{standard:{tool:'codex'}}})
   try{
    const add=await toAdd(page)
    await choose(add,'Quick fix')
    await expect(add.getByRole('alert')).toHaveText(MISSING)
    await expect(add.getByRole('button',{name:'Add pipeline',exact:true})).toBeDisabled()
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
  // (d) Add pipelines on an outcome approved without trust: a tier backup on Google Antigravity is refused, as the controller would.
  {
   const {f,w,page}=await open(1280,'light',{tools:['claude','antigravity'],crew:{standard:{tool:'claude',backup:{tool:'antigravity'}}}})
   try{
    const add=await toAdd(page)
    await choose(add,'Quick fix')
    await expect(add.getByRole('alert')).toHaveText('Google Antigravity starts only in a folder you have trusted, and this outcome was approved without folder trust. This pathway’s Code, PR and Merge stages can run on it from your saved Standard crew: change that crew in Gaffer settings, change the stage in the pathway, or choose standard stages.')
    await expect(add.getByRole('button',{name:'Add pipeline',exact:true})).toBeDisabled()
    expect(w.errors).toEqual([])
   } finally { await w.context.close() }
  }
 }
 console.log('pathways-choice: ok')
} finally { await browser.close() }
