import {chromium,expect} from '@playwright/test'
import {mkdir,readFile} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// A work order pasted into New Factory outcome: the controller reads it (POST /api/v2/gaffer/work-orders/check, answered
// here as the server answers it) and the plan review opens filled in. Its crew is a suggestion for this outcome only,
// Launch readiness says what the machine cannot run, and nothing it says turns on YOLO, folder trust or merging.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-work-order'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),oid='e'.repeat(32)
const example=await readFile(new URL('../../internal/gaffer/testdata/workorder_example.md',import.meta.url),'utf8')
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const ADAPTER={claude:'claude-agent-acp',codex:'codex-acp',opencode:'opencode'}
const agent=(tool,state='available')=>({tool,adapter:ADAPTER[tool],state,options:tool==='claude'?[{id:'effort',category:'thought_level',values:['default','low','medium','high','max']}]:[]})
const machine=agents=>({agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,yolo:true,acp:true,acpStartOptions:true,acpAgents:agents}})
const everyAgent=[machine([agent('claude'),agent('codex'),agent('opencode')])],noCodex=[machine([agent('claude'),agent('codex','missing'),agent('opencode')])]
const folder=(id,name)=>({id,host:'a1',machine:'workshop',dir:`/work/${name}`,name,isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:0,version:1,createdAt:now,updatedAt:now})
// The order the server reads from the example (TestWorkOrderExample in internal/gaffer/workorder_test.go).
const order={objective:'Add CSV export to the reports page',project:'reports-api',crew:{heavy:{tool:'claude',model:'opus',effort:'high'},standard:{tool:'codex'}},sessions:[{title:'Export endpoint',prompt:'Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,\nup to the first ### heading)',criteria:['An export of 10,000 rows finishes in under 5 seconds','A reader of another team’s report is refused'],stages:[{kind:'plan'},{kind:'code',tier:'heavy',part:'Streaming writer',brief:'Write rows as they are read, never the whole file in memory.'},{kind:'code',tier:'heavy',part:'Endpoint and auth',brief:'Wire the handler and reuse the report’s read permission.'},{kind:'review',tier:'standard'},{kind:'test'},{kind:'pr'}]}]}
// The saved crew: OpenCode on every tier, with a backup on Heavy that the work order's suggestion leaves out.
const savedCrew={light:{tool:'opencode',model:''},standard:{tool:'opencode',model:''},heavy:{tool:'opencode',model:'',backup:{tool:'claude',model:''}}}

// A running outcome of the person's, to add to and to copy the report of.
const running={id:oid,revision:3,objective:'Add CSV export to the reports page',state:'settled',note:'',agentId:'a1',projectId:'p-reports',projectName:'reports-api',dir:'/work/reports-api',tool:'opencode',model:'',providerId:pid,maxReviews:12,hours:24,trust:false,yolo:false,createdAt:now-3600000,updatedAt:now,
  sessions:[{key:'a',title:'Export API',prompt:'p',state:'following',note:'The code stage is under way.',updatedAt:now,stage:0,sessionState:'running',stages:[{kind:'code',tool:'opencode',model:'',state:'running',requestId:'r'},{kind:'review',tool:'opencode',model:'',state:'pending',requestId:'r'}]}]}
const report=`---\nreet: work-report/1\noutcome: ${oid}\nobjective: Add CSV export to the reports page\n---\n\n## Pipeline: Export API\n\nstate: following\n`
function fixture({machines=everyAgent,reportsProject=true,withOutcome=false}={}){
  const box={checks:[],approved:[],added:[]}
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/gaffer/crew':{crew:savedCrew,saved:true,tiers:['light','standard','heavy'],defaults:{}},
    '/api/v2/gaffer/pathways':{pathways:[{id:'pw1',name:'Careful review',description:'',enabled:true,stages:[{kind:'code'},{kind:'review'}],createdAt:now,updatedAt:now},{id:'pw2',name:'Old way',description:'',enabled:false,stages:[{kind:'code'}],createdAt:now,updatedAt:now}]},
    '/api/v2/folder-projects':{projects:reportsProject?[folder('p-billing','billing'),folder('p-reports','reports-api')]:[folder('p-billing','billing')],savedWaiting:0},
    '/api/v2/settings/yolo':{allowed:true,changedBy:'owner@example.test'},
    '/api/v2/machines/a1/models':r=>{const tool=new URL(r.url()).searchParams.get('tool');return {tool,state:'listed',models:[{id:'opus',provider:''},{id:`${tool}-model`,provider:''}],more:false,source:'cli',version:'1',fetchedAt:now,checkedAt:now,reason:'',sentence:''}},
    '/api/v2/machines/a1/accounts':{state:'read',sentence:'',planLimits:false,checkedAt:now,accounts:[]},
    '/api/v2/gaffer/goals':()=>({goals:[],readAt:now}),
    '/api/v2/gaffer/conversations':{conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]},
    '/api/v2/terminal/sessions':{sessions:[]},
    '/api/v2/gaffer/work-orders/check':r=>{
      const {text}=r.postDataJSON();box.checks.push(text)
      const empty={order:null,problems:[],projectId:'',pathwayId:'',notes:[]}
      if(text.includes('yolo: true'))return{...empty,problems:[{line:4,text:'The header has no “yolo” key. Use reet, objective, project, crew, pathway or continues.'}]}
      if(text.includes('crew: light=claude'))return{...empty,order:{...order,crew:{light:{tool:'claude'}}},projectId:'p-reports'}
      // The parser clears the objective of a continuation, and refuses crew with it.
      if(text.includes('continues:')){const {crew:_,...rest}=order;return{...empty,order:{...rest,objective:'',project:undefined,continues:oid}}}
      return reportsProject?{...empty,order,projectId:'p-reports'}:{...empty,order,notes:['No project of yours is called “reports-api”. Choose a project when you approve it.']}
    },
    '/api/v2/gaffer/outcomes':r=>{
      if(r.method()!=='POST')return {outcomes:withOutcome?[running]:[]}
      const input=r.postDataJSON();box.approved.push(input)
      return {status:201,json:{id:oid,revision:1,objective:input.objective,state:'settled',note:'',agentId:'a1',projectId:input.projectId,projectName:'reports-api',dir:'/work/reports-api',tool:input.tool,providerId:pid,maxReviews:12,hours:24,sessions:input.sessions.map((s,n)=>({key:'abcde'[n],title:s.title,prompt:s.prompt,state:'following',note:'Started and followed.',updatedAt:now,stage:0,stages:[]})),createdAt:now,updatedAt:now}}
    },
    [`/api/v2/gaffer/outcomes/${oid}`]:()=>box.added.length?{...running,revision:4,sessions:[...running.sessions,{key:'b',title:'Export endpoint',prompt:'p',state:'following',note:'Started and followed.',updatedAt:now,stage:0,stages:[]}]}:running,
    [`/api/v2/gaffer/outcomes/${oid}/sessions`]:r=>{box.added.push(r.postDataJSON());return{...running,revision:4,sessions:[...running.sessions,{key:'b',title:'Export endpoint',prompt:'p',state:'waiting',note:'Waiting its turn.',updatedAt:now,stage:0,stages:[]}]}},
  }
  return {box,routes,machines}
}
async function open(width,theme,over){
  const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,terminalMachines:f.machines,routes:f.routes})
  await w.page.goto(base+'/factory')
  await w.page.getByRole('button',{name:'New Factory outcome',exact:true}).first().click()
  const dialog=w.page.getByRole('dialog',{name:'New Factory outcome',exact:true})
  await expect(dialog.getByRole('combobox',{name:'Project',exact:true})).toBeVisible()
  return {f,w,page:w.page,dialog}
}
const paste=async(dialog,text)=>dialog.getByRole('textbox',{name:'Outcome',exact:true}).fill(text)
const dialogFits=async page=>expect(await page.locator('dialog[open] [data-shell-body]').last().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
// Every control in the open dialog is at least 44 px tall on a phone.
async function targets(page){
  const small=await page.evaluate(()=>[...document.querySelectorAll('dialog[open]:last-of-type :is(button,select,input:not([type=checkbox]):not([type=radio]),textarea,summary,a[href])')].filter(el=>{const r=el.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(el).visibility!=='hidden'&&!(el.matches('a')&&getComputedStyle(el).display==='inline')&&!el.closest('details:not([open]) > :not(summary)')}).map(el=>({name:(el.getAttribute('aria-label')||el.textContent||el.tagName).trim().slice(0,40),h:Math.round(el.getBoundingClientRect().height)})).filter(item=>item.h<44))
  expect(small).toEqual([])
}
const stagesOf=body=>body.sessions[0].stages

try{
  // The four looks: a clean work order opens the plan review filled in, with its crew suggested for this outcome.
  for(const [width,theme] of [[1440,'light'],[1440,'dark'],[390,'light'],[390,'dark']]){
    const {f,w,page,dialog}=await open(width,theme)
    try{
      await paste(dialog,example)
      await expect(dialog.getByText('This is a work order.',{exact:true})).toBeVisible()
      await expect(dialog.getByRole('button',{name:'Ask Gaffer for a plan',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('button',{name:'Write the plan myself',exact:true})).toHaveCount(0)
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.getByRole('list',{name:'Planned sessions',exact:true})).toBeVisible()
      expect(f.box.checks).toEqual([example])
      await expect(dialog.locator('.objective')).toHaveText('OutcomeAdd CSV export to the reports page')
      await expect(dialog.getByRole('combobox',{name:'Project',exact:true})).toHaveValue('p-reports')
      await expect(dialog.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('Export endpoint')
      await expect(dialog.getByRole('textbox',{name:'Pipeline 1 prompt',exact:true})).toHaveValue(order.sessions[0].prompt)
      const chips=dialog.getByRole('group',{name:'Pipeline 1 stages',exact:true})
      for(const [name,on] of [['Plan',true],['Design',false],['Code',true],['Review',true],['Test',true],['PR',true],['Babysit',false],['Merge',false]])await expect(chips.getByRole('button',{name,exact:true})).toHaveAttribute('aria-pressed',String(on))
      const parts=dialog.getByRole('group',{name:'Code parts in pipeline 1',exact:true})
      await expect(parts.getByRole('textbox',{name:'Part 1 name',exact:true})).toHaveValue('Streaming writer')
      await expect(parts.getByRole('textbox',{name:'Part 2 name',exact:true})).toHaveValue('Endpoint and auth')
      await expect(dialog.getByRole('combobox',{name:'Review tier in pipeline 1',exact:true})).toHaveValue('standard')
      // The tier is the work order's, not Gaffer's.
      await expect(dialog.locator('.stage-tier[data-stage=review] .stage-why')).toContainText('Suggested by the work order.')
      await expect(dialog.getByRole('textbox',{name:'Criterion 1 for pipeline 1',exact:true})).toHaveValue(order.sessions[0].criteria[0])
      await expect(dialog.getByRole('textbox',{name:'Criterion 2 for pipeline 1',exact:true})).toHaveValue(order.sessions[0].criteria[1])
      // Heavy and Standard are on the suggestion, Light on the saved crew; each stage line says which.
      await expect(dialog.locator('.crew-row[aria-label="Heavy crew"]')).toContainText('Suggested by the work order')
      await expect(dialog.locator('.crew-row[aria-label="Light crew"]')).not.toContainText('Suggested by the work order')
      await expect(dialog.locator('.stage-tier[data-stage=plan] .ready-line')).toContainText('Claude Code · opus · high effort · Suggested by the work order')
      await expect(dialog.locator('.stage-tier[data-stage=pr] .ready-line')).toContainText('Saved crew')
      const approve=dialog.getByRole('button',{name:'Approve and start 1 pipeline',exact:true})
      await expect(approve).toBeEnabled()
      await page.evaluate(()=>document.documentElement.scrollWidth).then(sw=>expect(sw).toBeLessThanOrEqual(width))
      await dialogFits(page)
      if(width<500)await targets(page)
      await dialog.getByRole('list',{name:'Planned sessions',exact:true}).scrollIntoViewIfNeeded()
      await page.mouse.move(0,0);await page.screenshot({path:`${shots}/plan-${width}-${theme}.png`})
      if(width===1440&&theme==='dark'){
        // The way back to the saved crew: every tier on it again, the Heavy backup with it.
        await dialog.locator('details.crew summary').click()
        await dialog.getByRole('button',{name:'Use my saved crew',exact:true}).click()
        await expect(dialog.getByText('Suggested by the work order',{exact:true})).toHaveCount(0)
        await expect(dialog.getByRole('combobox',{name:'Heavy agent',exact:true})).toHaveValue('opencode')
        await expect(dialog.getByRole('combobox',{name:'Heavy backup agent',exact:true})).toHaveValue('claude')
        await expect(dialog.locator('.stage-tier[data-stage=plan] .ready-line')).toContainText('Saved crew')
        // Back shows the work order as pasted.
        await dialog.getByRole('button',{name:'Back',exact:true}).click()
        await expect(dialog.getByRole('textbox',{name:'Outcome',exact:true})).toHaveValue(example)
      }
      if(width===1440&&theme==='light'){
        await approve.click()
        await expect.poll(()=>f.box.approved.length).toBe(1)
        const body=f.box.approved[0]
        expect(body.objective).toBe(order.objective);expect(body.projectId).toBe('p-reports')
        expect(stagesOf(body).map(s=>[s.kind,s.part??'',s.tier,s.tool,s.model,s.effort??''])).toEqual([
          ['plan','','heavy','claude','opus','high'],['code','Streaming writer','heavy','claude','opus','high'],['code','Endpoint and auth','heavy','claude','opus','high'],
          ['review','','standard','codex','',''],['test','','standard','codex','',''],['pr','','light','opencode','','']])
        // The saved Heavy backup does not ride along with the suggestion.
        expect(stagesOf(body).some(s=>s.backup)).toBe(false)
        expect(body.sessions[0].criteria).toEqual(order.sessions[0].criteria)
        expect(body.yolo).toBe(false);expect(body.trust).toBe(false);expect(body.autoMerge).toBe(false)
        expect(JSON.stringify(body)).not.toMatch(/"(yolo|trust|autoMerge)":true/)
      }
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  console.log('ok - a work order opens the plan review, at 390 px and desktop, light and dark')

  // A second work order starts from the saved crew again: the first one's suggestion goes with it, on screen and in
  // what is sent. Editing the text back to a plain objective drops the work order and its suggestion too.
  {
    const {f,w,dialog}=await open(1440,'light')
    try{
      await paste(dialog,example)
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.locator('.crew-row[aria-label="Heavy crew"]')).toContainText('Suggested by the work order')
      await dialog.getByRole('button',{name:'Back',exact:true}).click()
      await paste(dialog,example.replace('crew: heavy=claude/opus/high, standard=codex','crew: light=claude'))
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.getByRole('list',{name:'Planned sessions',exact:true})).toBeVisible()
      await expect(dialog.locator('.crew-row[aria-label="Heavy crew"]')).not.toContainText('Suggested by the work order')
      await expect(dialog.locator('.crew-row[aria-label="Standard crew"]')).not.toContainText('Suggested by the work order')
      await expect(dialog.locator('.crew-row[aria-label="Light crew"]')).toContainText('Suggested by the work order')
      await expect(dialog.locator('.stage-tier[data-stage=plan] .ready-line')).toContainText('OpenCode · Agent’s default model · Saved crew')
      await dialog.getByRole('button',{name:'Approve and start 1 pipeline',exact:true}).click()
      await expect.poll(()=>f.box.approved.length).toBe(1)
      const stages=stagesOf(f.box.approved[0])
      expect(stages.map(s=>[s.kind,s.tool])).toEqual([['plan','opencode'],['code','opencode'],['code','opencode'],['review','opencode'],['test','opencode'],['pr','claude']])
      // The saved Heavy backup is back with the saved crew.
      expect(stages[0].backup?.tool).toBe('claude')
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
    console.log('ok - a second work order starts from the saved crew')
  }
  {
    const {f,w,dialog}=await open(1440,'light')
    try{
      await paste(dialog,example)
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await dialog.getByRole('button',{name:'Back',exact:true}).click()
      await paste(dialog,'Ship CSV export for billing.')
      await expect(dialog.getByText('This is a work order.',{exact:true})).toHaveCount(0)
      await dialog.getByRole('button',{name:'Write the plan myself',exact:true}).click()
      await expect(dialog.getByRole('list',{name:'Planned sessions',exact:true})).toBeVisible()
      await expect(dialog.getByText('Suggested by the work order',{exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('list',{name:'Notes on the work order',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('')
      await expect(dialog.locator('.stage-tier[data-stage=review] .ready-line')).toContainText('OpenCode · Agent’s default model · Saved crew')
      expect(f.box.checks.length).toBe(1)
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
    console.log('ok - a plain objective drops the work order')
  }
  // A paste over the server's limit is refused here, without sending it.
  {
    const {f,w,dialog}=await open(390,'light')
    try{
      await paste(dialog,example+'\n'+'€'.repeat(22000))
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.getByRole('list',{name:'Problems in the work order',exact:true})).toHaveText('The work order is over 64 KB. Shorten the briefs.')
      expect(f.box.checks).toEqual([])
    }finally{await w.context.close()}
    console.log('ok - an oversized paste is not sent')
  }

  // A suggested agent the machine lacks: its stages cannot start, Approve says why, and a project the server could not
  // match is a note above the pipelines.
  {
    const {w,dialog}=await open(1440,'light',{machines:noCodex,reportsProject:false})
    try{
      await paste(dialog,example)
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.getByRole('list',{name:'Notes on the work order',exact:true})).toContainText('No project of yours is called “reports-api”. Choose a project when you approve it.')
      await expect(dialog.getByRole('combobox',{name:'Project',exact:true})).toHaveValue('p-billing')
      await expect(dialog.locator('.stage-tier[data-stage=review] .ready-line')).toContainText('Cannot start')
      await expect(dialog.getByRole('button',{name:'Approve and start 1 pipeline',exact:true})).toBeDisabled()
      await expect(dialog.locator('p.status')).toContainText('Codex')
      await dialog.getByRole('list',{name:'Notes on the work order',exact:true}).scrollIntoViewIfNeeded()
      await w.page.mouse.move(0,0);await w.page.screenshot({path:`${shots}/notes-1440-light.png`})
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
    console.log('ok - an agent the machine lacks holds Approve')
  }

  // A work order with a problem: each problem with its line, focus on the list, and nothing moves on.
  for(const [width,theme] of [[390,'dark'],[1440,'light']]){
    const {f,w,page,dialog}=await open(width,theme)
    try{
      await paste(dialog,example.replace('project: reports-api\n','yolo: true\n'))
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      const problems=dialog.getByRole('list',{name:'Problems in the work order',exact:true})
      await expect(problems).toHaveText('Line 4: The header has no “yolo” key. Use reet, objective, project, crew, pathway or continues.')
      await expect(problems).toBeFocused()
      await expect(problems).toHaveClass(/warning/)
      await expect(dialog.getByRole('button',{name:'Read the work order',exact:true})).toBeVisible()
      await expect(dialog.getByRole('list',{name:'Planned sessions',exact:true})).toHaveCount(0)
      expect(f.box.approved).toEqual([])
      await page.screenshot({path:`${shots}/problems-${width}-${theme}.png`})
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  console.log('ok - problems are listed by line and nothing moves on')

  // A work order that continues a running outcome opens the same plan review in an adding mode: the outcome's own choices
  // are not offered again, and Approve and add sends the pipelines to the add request with their stages and criteria.
  const continuing=example.replace('objective: Add CSV export to the reports page\n',`continues: ${oid}\n`).replace('project: reports-api\n','').replace('crew: heavy=claude/opus/high, standard=codex\n','')
  for(const [width,theme] of [[1440,'light'],[1440,'dark'],[390,'light'],[390,'dark']]){
    const {f,w,page,dialog}=await open(width,theme,{withOutcome:true})
    try{
      await paste(dialog,continuing)
      await dialog.getByRole('button',{name:'Read the work order',exact:true}).click()
      await expect(dialog.getByRole('heading',{name:'Add to “Add CSV export to the reports page”',exact:true})).toBeVisible()
      await expect(dialog.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('Export endpoint')
      await expect(dialog.getByRole('group',{name:'Code parts in pipeline 1',exact:true})).toBeVisible()
      await expect(dialog.getByRole('textbox',{name:'Criterion 2 for pipeline 1',exact:true})).toHaveValue(order.sessions[0].criteria[1])
      // What the outcome fixed when it started is not offered again.
      await expect(dialog.getByRole('combobox',{name:'Project',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('combobox',{name:'Agent',exact:true})).toHaveCount(0)
      await expect(dialog.locator('details.crew')).toHaveCount(0)
      await expect(dialog.getByRole('button',{name:/^Choose who does what/})).toHaveCount(0)
      await expect(dialog.getByRole('radiogroup',{name:'Approvals',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('radiogroup',{name:'Decisions',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('checkbox')).toHaveCount(0)
      await expect(dialog.getByText('Change limits')).toHaveCount(0)
      await expect(dialog.getByRole('region',{name:'Reply to Gaffer',exact:true})).toHaveCount(0)
      await expect(dialog.getByRole('button',{name:/^Approve and start/})).toHaveCount(0)
      await expect(dialog.getByRole('button',{name:'Attach an image to the outcome'})).toHaveCount(0)
      const approve=dialog.getByRole('button',{name:'Approve and add',exact:true})
      await expect(approve).toBeEnabled()
      await page.evaluate(()=>document.documentElement.scrollWidth).then(sw=>expect(sw).toBeLessThanOrEqual(width))
      await dialogFits(page)
      if(width<500)await targets(page)
      await page.locator('dialog[open] [data-shell-body]').last().evaluate(el=>{el.scrollTop=0})
      await page.mouse.move(0,0);await page.screenshot({path:`${shots}/add-${width}-${theme}.png`})
      if(width===1440&&theme==='light'){
        await approve.click()
        await expect.poll(()=>f.box.added.length).toBe(1)
        const body=f.box.added[0]
        expect(body.revision).toBe(3);expect(body.confirmed).toBe(true);expect('pathway' in body).toBe(false)
        expect(body.sessions.map(s=>s.title)).toEqual(['Export endpoint'])
        expect(body.sessions[0].criteria).toEqual(order.sessions[0].criteria)
        // The same stages a start sends: kinds, parts, briefs and tiers, on the saved crew as the add request uses it.
        expect(body.sessions[0].stages.map(s=>[s.kind,s.part??'',s.tier,s.tool])).toEqual([['plan','','heavy','opencode'],['code','Streaming writer','heavy','opencode'],['code','Endpoint and auth','heavy','opencode'],['review','','standard','opencode'],['test','','standard','opencode'],['pr','','light','opencode']])
        expect(body.sessions[0].stages[1].brief).toBe('Write rows as they are read, never the whole file in memory.')
        expect(JSON.stringify(body)).not.toMatch(/"(yolo|trust|autoMerge)":true/)
        expect(f.box.approved).toEqual([])
        await expect(dialog.getByRole('list',{name:'Outcome sessions',exact:true})).toContainText('Export endpoint')
      }
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  console.log('ok - a continuation adds to the running outcome, at 390 px and desktop, light and dark')

  // Copy for your agent, in the outcome's menu: the work report on the clipboard, or shown to copy by hand when the
  // browser will not let reet copy.
  for(const [width,theme,copies] of [[1440,'light',true],[1440,'dark',false],[390,'light',true],[390,'dark',false]]){
    const f=fixture({withOutcome:true}),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,terminalMachines:f.machines,routes:f.routes})
    const page=w.page,reads=[]
    // The report is plain text, which the shared helper does not answer.
    await page.route(`**/api/v2/gaffer/outcomes/${oid}/work-report`,route=>{reads.push(route.request().method());return route.fulfill({status:200,contentType:'text/plain; charset=utf-8',body:report})})
    if(copies)await w.context.grantPermissions(['clipboard-read','clipboard-write'],{origin:new URL(base).origin})
    else await page.addInitScript(()=>{Object.defineProperty(navigator,'clipboard',{value:{writeText:()=>Promise.reject(new DOMException('Write permission denied.','NotAllowedError'))}});document.execCommand=()=>false})
    try{
      await page.goto(`${base}/factory?pipeline=${oid}.a`)
      const more=page.getByRole('button',{name:'More for this outcome',exact:true})
      await more.click()
      const item=page.getByRole('button',{name:'Copy for your agent',exact:true})
      await expect(item).toBeVisible()
      if(width<500)expect((await item.boundingBox()).height).toBeGreaterThanOrEqual(44)
      await page.mouse.move(0,0);await page.screenshot({path:`${shots}/menu-${width}-${theme}.png`})
      await item.click()
      expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
      if(copies){
        await expect(page.getByText('Copied the work report for “Add CSV export to the reports page”. Paste it to your agent.',{exact:true})).toBeVisible()
        const copied=await page.evaluate(()=>navigator.clipboard.readText())
        expect(copied.startsWith('---\nreet: work-report/1')).toBe(true);expect(copied).toBe(report)
      }else{
        const shown=page.getByRole('dialog',{name:'Work report',exact:true})
        await expect(shown).toContainText('This browser did not let reet copy it.')
        await expect(shown.getByRole('textbox',{name:'Work report',exact:true})).toHaveValue(report)
        await page.screenshot({path:`${shots}/menu-failed-${width}-${theme}.png`})
      }
      expect(reads).toEqual(['GET'])
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  console.log('ok - copy for your agent, and the report to copy by hand when the browser will not')

  // The prompt for an outside agent: the skill's use line for this controller, then what the dialog holds and no more.
  {
    const {w,page,dialog}=await open(1440,'light')
    try{
      await w.context.grantPermissions(['clipboard-read','clipboard-write'],{origin:new URL(base).origin})
      await dialog.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-reports')
      await dialog.getByRole('button',{name:'Copy prompt for an outside agent',exact:true}).click()
      await expect(dialog.getByText('Copied.',{exact:true})).toBeVisible()
      const copied=await page.evaluate(()=>navigator.clipboard.readText())
      expect(copied).toBe(`Run \`npx skills@1.7.4 use "${new URL(base).origin}/eyupio/factory-contract" --skill "factory-contract"\` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.\n\nContext:\n- Project: reports-api\n- Pathways switched on: Careful review\n- Tiers with a saved crew: heavy, standard, light`)
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
    console.log('ok - copy prompt for an outside agent')
  }
}finally{await browser.close()}
