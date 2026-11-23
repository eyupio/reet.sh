import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Private Factory templates: saving a finished outcome's plan, the list and a template's page, using one (the changes
// since the last use, an older version, skills that are no longer approved), what the launch sends, and the line on a
// run that says where it came from, and the crew chosen for a start. The API is answered with fixtures that follow docs/handoffs/factory-templates-design.md.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-templates'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),oid='e'.repeat(32),tid='7'.repeat(32),gone='8'.repeat(32),dropped='9'.repeat(32),started='d'.repeat(32)
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const st=(kind,state,extra={})=>({kind,tool:'claude',model:'',state,requestId:'r',...extra})
const project=(id,name,dir,position)=>({id,host:'a1',machine:'workshop',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position,version:1,createdAt:now,updatedAt:now})
const machines=[{agentId:'a1',host:'workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,inspect:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:[]}]}}]
// The same machine while it also has Codex, for a saved crew that later cannot start.
const withCodex=[{...machines[0],capabilities:{...machines[0].capabilities,acpAgents:[...machines[0].capabilities.acpAgents,{tool:'codex',adapter:'codex-acp',state:'available',options:[]}]}}]
const GAP={pipeline:0,stage:0,id:'library:go-tests',name:'Go tests',revision:'rev1aaaa1111',status:'changed',key:'0:0:library:go-tests'}
const GAP2={pipeline:1,stage:0,id:'builtin:lint',name:'Lint',revision:'rev9zzzz9999',status:'unavailable',key:'1:0:builtin:lint'}
const skill=rev=>({id:'library:go-tests',name:'Go tests',source:'personal',revision:rev})
const stage=(kind,tier,skills=[],brief)=>({kind,tier,skills,...(brief?{brief}:{})})
// Three versions of one template: v2 rewords a criterion, v3 adds a test stage, a criterion and a newer skill revision, and gives review a brief.
const contents={
  1:{name:'CSV export',description:'Billing export: the API, the button and the tests.',objective:'Ship CSV export for billing.',maxReviews:8,hours:24,pipelines:[
    {title:'Export API',prompt:'Build the CSV export endpoint. Raise a pull request. Do not merge.',autoMerge:false,criteria:[{id:'c1',text:'The export downloads as CSV.'}],stages:[stage('code','standard',[skill('rev1aaaa1111')]),stage('review','heavy')]},
    {title:'Billing button',prompt:'Add the export button to the billing page.',autoMerge:true,criteria:[],stages:[stage('code','standard',[{id:'builtin:lint',name:'Lint',source:'built-in',revision:'rev9zzzz9999'}])]}]},
}
contents[2]={...contents[1],pipelines:[{...contents[1].pipelines[0],criteria:[{id:'c1',text:'The export downloads as a CSV file.'}]},contents[1].pipelines[1]]}
contents[3]={...contents[2],pipelines:[{...contents[2].pipelines[0],criteria:[...contents[2].pipelines[0].criteria,{id:'c2',text:'Totals match the ledger.'}],stages:[stage('code','standard',[skill('rev2bbbb2222')]),stage('review','heavy',[],'Read every line of the export.'),stage('test','standard')]},contents[2].pipelines[1]]}
const diffOf=(from,to)=>from===to?{from,to,changed:false,nameChanged:false,descriptionChanged:false,objectiveChanged:false,limitsChanged:false,orderChanged:false,olderNotShown:false,pipelines:[]}:{from,to,changed:true,nameChanged:false,descriptionChanged:false,objectiveChanged:false,limitsChanged:false,orderChanged:false,olderNotShown:false,pipelines:[
  {title:'Export API',change:'changed',promptChanged:false,autoMergeChanged:false,stagesChanged:from<3&&to>=3,stagesBefore:['code','review'],stagesAfter:['code','review','test'],criteria:{added:from<3&&to>=3?['Totals match the ledger.']:[],removed:[],reworded:from<2&&to>=2?[{id:'c1',from:'The export downloads as CSV.',to:'The export downloads as a CSV file.'}]:from>=2&&to<2?[{id:'c1',from:'The export downloads as a CSV file.',to:'The export downloads as CSV.'}]:[]},skills:from<3&&to>=3?[{stage:'code',id:'library:go-tests',name:'Go tests',from:'rev1aaaa1111',to:'rev2bbbb2222'}]:[],stageEdits:from<3&&to>=3?[{stage:'review',field:'brief',from:'',to:'Read every line of the export.'}]:[]},
  {title:'Billing button',change:'same',promptChanged:false,autoMergeChanged:false,stagesChanged:false,stagesBefore:[],stagesAfter:[],criteria:{added:[],removed:[],reworded:[]},skills:[],stageEdits:[]}]}

function fixture(over={}){
  // `older` is a template whose last used version, 1, is no longer kept; `crew` is the saved crew; `codex` puts Codex on the machine until `codexGone`;
  // `other` marks Billing button changed in a way the diff does not describe.
  const box={versions:over.versions??3,lastUsed:over.older?1:'lastUsed' in over?over.lastUsed:2,lastUsedAt:now-2*86400000,older:!!over.older,other:!!over.other,crew:over.crew??null,codex:!!over.codex,codexGone:false,gaps:over.gaps??[],none:!!over.none,saves:[],edits:[],deletes:[],launches:[],diffs:[],reads:[],saveFail:'',editFail:'',deleteFail:'',launchFail:'',gapsAfter:null,fail502:0,name:'CSV export',description:contents[1].description,listed:true}
  const content=v=>({...contents[Math.min(v,3)],name:box.name,description:box.description})
  const view=(v=box.versions)=>({id:tid,version:v,latest:box.versions,versions:Array.from({length:box.versions},(_,n)=>n+1).filter(n=>!box.older||n>1),requestId:'r'.repeat(32),outcomeId:oid,createdAt:now-86400000*3,...(box.lastUsed?{lastUsedVersion:box.lastUsed,lastUsedAt:box.lastUsedAt}:{lastUsedAt:0}),content:content(v),skillGaps:box.gaps})
  const summary=()=>({id:tid,version:box.versions,name:box.name,description:box.description,pipelines:2,createdAt:now-86400000*3,updatedAt:now-3600000,...(box.lastUsed?{lastUsedVersion:box.lastUsed,lastUsedAt:box.lastUsedAt}:{lastUsedAt:0})})
  const done=(key,title)=>({key,title,prompt:'p',state:'done',note:'Finished.',sessionId:'s-'+key,conversationId:key.repeat(32).slice(0,32),sessionState:'ended',updatedAt:now-3600000,stage:1,stages:[st('code','done'),st('review','done')],criteria:[{id:'c1',text:'The export downloads as CSV.',revision:1}],criteriaRevision:1,reviewRevision:1,deliveryAccepted:true})
  const mk=(id,extra={})=>({id,revision:3,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now-3600000,sessions:[done('a','Export API')],...extra})
  // Runs record the template's name at their start; the fifth is older and recorded none.
  const outcomes=[mk(oid),mk('1'.repeat(32),{objective:'Ship CSV export, second run.',templateId:tid,templateVersion:2,templateName:'CSV export',sessions:[done('b','Export API')]}),mk('2'.repeat(32),{objective:'Ship CSV export, third run.',templateId:gone,templateVersion:1,templateName:'Old CSV export',sessions:[done('c','Export API')]}),mk('3'.repeat(32),{objective:'Ship CSV export, fourth run.',templateId:dropped,templateVersion:1,templateName:'Older export',sessions:[done('d','Export API')]}),mk('4'.repeat(32),{objective:'Ship CSV export, fifth run.',templateId:gone,templateVersion:1,sessions:[done('e','Export API')]}),mk('5'.repeat(32),{objective:'Ship CSV export, sixth run.',sessions:[done('f','Export API'),{...done('g','Billing button'),state:'failed',deliveryAccepted:false}]})]
  const err=(status,code,error)=>({status,json:{error,code}})
  const body=r=>r.postDataJSON()
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/gaffer/conversations':{conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]},
    '/api/v2/gaffer/goals':()=>({goals:[],readAt:Date.now()}),
    '/api/v2/gaffer/outcomes':()=>({outcomes}),
    [`/api/v2/gaffer/outcomes/${started}`]:()=>({...mk(started),sessions:[{key:'a',title:'Export API',prompt:'p',state:'following',note:'The code stage is under way.',updatedAt:now}]}),
    '/api/v2/terminal/status':{configured:true,available:true,reason:''},
    '/api/v2/terminal/sessions':{sessions:[]},
    '/api/v2/terminal/machines':()=>({machines:box.codex&&!box.codexGone?withCodex:machines}),
    ...(box.crew?{'/api/v2/gaffer/crew':{crew:box.crew,saved:true,tiers:['light','standard','heavy'],defaults:{}}}:{}),
    '/api/v2/projects':{projects:[]},
    '/api/v2/folder-projects':{projects:[project('p1','billing','/work/billing',0)],savedWaiting:0},
    '/api/v2/settings/yolo':{allowed:false},
    '/api/v2/gaffer/templates':r=>{
      if(r.method()==='POST'){
        const b=body(r);box.saves.push(b)
        if(box.fail502>0){box.fail502--;return{status:502,json:{error:'502 Bad Gateway'}}}
        // The outcome changed after the page read it: Export API failed meanwhile, so the page still offered the save.
        if(box.saveFail==='not_successful')return err(409,'template_not_successful','“Export API” did not finish well, so this plan cannot be saved. Only a plan whose every pipeline finished, or had its delivery accepted, can be saved as a template.')
        if(box.saveFail==='limit')return err(409,'template_limit','This account already has 20 templates. Delete one before saving another.')
        if(box.saveFail==='large')return err(422,'template_too_large','That template is too large to save.')
        if(box.saveFail==='changed'){box.saveFail='';box.versions++;return err(409,'template_changed','That template changed. Reload it before trying again.')}
        if(b.templateId){box.versions++;box.name=b.name;box.description=b.description;return{status:201,json:view(box.versions)}}
        return{status:201,json:{...view(1),versions:[1],latest:1,content:{...content(1),name:b.name}}}
      }
      return{templates:box.none?[]:[summary()]}
    },
    [`/api/v2/gaffer/templates/${tid}`]:r=>{
      const m=r.method()
      if(m==='PUT'){
        const b=body(r);box.edits.push(b)
        if(box.editFail==='changed'){box.editFail='';box.versions++;return err(409,'template_changed','That template changed. Reload it before trying again.')}
        box.versions++;box.name=b.content.name;box.description=b.content.description;return view(box.versions)
      }
      if(m==='DELETE'){
        box.deletes.push(new URL(r.url()).searchParams.get('version'))
        if(box.deleteFail==='changed'){box.deleteFail='';box.versions++;return err(409,'template_changed','That template changed. Reload it before trying again.')}
        box.none=true;return{status:204,json:null}
      }
      box.reads.push('latest');return view()
    },
    // No `from` is the version last used, or the oldest kept when that one has gone.
    [`/api/v2/gaffer/templates/${tid}/diff`]:r=>{const q=new URL(r.url()).searchParams;box.diffs.push(`${q.get('from')??'last'}-${q.get('to')}`);const from=q.get('from')?+q.get('from'):box.older?2:box.lastUsed;const d=diffOf(from,+q.get('to'));if(box.other)d.pipelines[1]={...d.pipelines[1],change:'changed'};return{...d,olderNotShown:!q.get('from')&&box.older}},
    [`/api/v2/gaffer/templates/${tid}/launch`]:r=>{
      const b=body(r);box.launches.push(b)
      if(box.launchFail==='skills'){box.launchFail='';box.gaps=box.gapsAfter;return{status:409,json:{error:'The approved skills changed.',code:'template_skills_changed',gaps:box.gapsAfter}}}
      if(box.fail502>0){box.fail502--;return{status:502,json:{error:'502 Bad Gateway'}}}
      return{status:201,json:{...mk(started),state:'starting',templateId:tid,templateVersion:b.version,sessions:[{key:'a',title:'Export API',prompt:'p',state:'starting',note:'Starting.',updatedAt:now}]}}
    },
    [`/api/v2/gaffer/templates/${gone}`]:{status:404,json:{error:'That template or outcome is not available.'}},
    [`/api/v2/gaffer/templates/${gone}/versions/1`]:{status:404,json:{error:'That template or outcome is not available.'}},
    [`/api/v2/gaffer/templates/${dropped}`]:()=>({...view(),id:dropped,content:{...content(3),name:'Older export'}}),
    [`/api/v2/gaffer/templates/${dropped}/versions/1`]:{status:404,json:{error:'That template or outcome is not available.'}},
  }
  for(let v=1;v<=3;v++)routes[`/api/v2/gaffer/templates/${tid}/versions/${v}`]=()=>{box.reads.push(`v${v}`);return v>box.versions?{status:404,json:{error:'That template or outcome is not available.'}}:view(v)}
  return {box,routes}
}
const open=async(width,theme,over={},extra={})=>{
  const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:1100,theme,touch:width<1000,terminalMachines:machines,routes:f.routes,...extra})
  return {f,w,page:w.page}
}
const noOverflow=async page=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true)
// The dialog's own content must not scroll sideways either.
const dialogFits=async page=>{expect(await page.locator('dialog[open] [data-shell-body]').last().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);expect(await page.locator('dialog[open]').last().evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=document.documentElement.clientWidth})).toBe(true)}
// The dialog's frame must not have been scrolled: its title and Close stay in view.
const titleInView=async dialog=>expect(await dialog.evaluate(el=>{const h=el.querySelector('header'),r=h.getBoundingClientRect(),d=el.getBoundingClientRect();return el.scrollTop===0&&r.top>=d.top-1&&r.bottom<=innerHeight&&r.top>=0})).toBe(true)
// Every control in the open dialog is at least 44 px tall on a phone.
async function targets(page){
  const small=await page.evaluate(()=>[...document.querySelectorAll('dialog[open]:last-of-type :is(button,select,input:not([type=checkbox]):not([type=radio]),textarea,summary,a[href])')].filter(el=>{const r=el.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(el).visibility!=='hidden'&&!(el.matches('a')&&getComputedStyle(el).display==='inline')&&!el.closest('details:not([open]) > :not(summary)')}).map(el=>({name:(el.getAttribute('aria-label')||el.textContent||el.tagName).trim().slice(0,40),h:Math.round(el.getBoundingClientRect().height)})).filter(item=>item.h<44))
  expect(small).toEqual([])
}
// Scrolls the open dialog's own body so a part of it is at the top, for a capture.
const scrollTo=async(page,selector)=>page.locator('dialog[open]').last().locator('[data-shell-body]').evaluate((body,sel)=>{const t=body.querySelector(sel);body.scrollTop+=t.getBoundingClientRect().top-body.getBoundingClientRect().top-8},selector)
const shot=async(page,name,width,theme)=>{await page.mouse.move(0,0);await page.screenshot({path:`${shots}/${name}-${width}-${theme}.png`})}
const focused=page=>page.evaluate(()=>{const el=document.activeElement;return el?(el.getAttribute('aria-label')||el.textContent||el.tagName).trim().replace(/\s+/g,' '):''})
async function tabTo(page,name,max=60){
  const seen=[]
  for(let i=0;i<max;i++){await page.keyboard.press('Tab');const now=await focused(page);seen.push(now);if(now===name)return seen}
  throw new Error(`Tab never reached “${name}”: ${seen.join(' | ')}`)
}
const FINISHED=`${base}/factory?pipeline=${oid}.a`
const templatesDialog=page=>page.getByRole('dialog',{name:/^(Templates|CSV export|Renamed export)$/})
const outcomeDialog=page=>page.getByRole('dialog',{name:'New Factory outcome',exact:true})
const openTemplates=async page=>{await page.goto(base+'/factory');await page.getByRole('button',{name:'Templates',exact:true}).click();return page.locator('dialog[open]').first()}

try{
  for(const [width,theme] of [[1280,'light'],[390,'dark']]){
    const wide=width>=1100
    // 1. Save a finished outcome's plan: what is kept and what is not, a new template or the next version, and every refusal in words.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(FINISHED)
        await page.getByRole('button',{name:'Save as template…',exact:true}).click()
        const dialog=page.getByRole('dialog',{name:'Save as template',exact:true})
        await expect(dialog).toContainText('Keeps the plan: each pipeline’s prompt, stages and criteria, and the skills approved for it.')
        await expect(dialog).toContainText('It does not keep the machine, folder, agents or sign-ins.')
        await expect(dialog).toContainText('Templates are private to you.')
        await expect(dialog.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('Ship CSV export for billing')
        await expect(dialog.getByRole('combobox',{name:'Save as',exact:true})).toHaveValue('')
        await shot(page,'save-dialog',width,theme)
        if(!wide)await targets(page)
        await dialog.getByRole('textbox',{name:'Name',exact:true}).fill('CSV export, new')
        await dialog.getByRole('textbox',{name:/^Description/}).fill('The whole billing export.')
        // A refusal for a plan that did not succeed is shown in the server's words, naming the pipeline, with the way on; what was typed stays.
        f.box.saveFail='not_successful'
        await dialog.getByRole('button',{name:'Save template',exact:true}).click()
        await expect(dialog.getByRole('alert')).toHaveText('“Export API” did not finish well, so this plan cannot be saved. Only a plan whose every pipeline finished, or had its delivery accepted, can be saved as a template. Nothing was saved. Close this dialog to go back to the outcome.')
        await expect(dialog.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('CSV export, new')
        await shot(page,'save-refused',width,theme)
        f.box.saveFail='limit'
        await dialog.getByRole('button',{name:'Save template',exact:true}).click()
        await expect(dialog.getByRole('alert')).toContainText('already has 20 templates')
        f.box.saveFail='large'
        await dialog.getByRole('button',{name:'Save template',exact:true}).click()
        await expect(dialog.getByRole('alert')).toContainText('too large')
        // A lost answer is tried again with the same request ID, so it is saved once.
        f.box.saveFail='';f.box.fail502=1
        await dialog.getByRole('button',{name:'Save template',exact:true}).click()
        await expect(dialog.getByRole('alert')).toContainText('Trying again is safe')
        await dialog.getByRole('button',{name:'Save template',exact:true}).click()
        await expect(page.getByRole('status').filter({hasText:'Saved as template “CSV export, new”, version 1.'})).toBeVisible()
        await expect(page.getByRole('status').filter({hasText:'Saved as template'}).getByRole('button',{name:'Open it in Templates',exact:true})).toBeVisible()
        await shot(page,'saved-notice',width,theme)
        const last=f.box.saves.at(-1),lost=f.box.saves.at(-2)
        expect(last.requestId).toMatch(/^[0-9a-f]{32}$/)
        expect(last.requestId).toBe(lost.requestId)
        expect(last).toEqual({requestId:last.requestId,outcomeId:oid,name:'CSV export, new',description:'The whole billing export.'})
        // A refusal is a finished request: the next try is a new one.
        expect(f.box.saves.slice(0,3).map(s=>s.requestId).every((id,n,all)=>all.indexOf(id)===n||n===0)).toBe(true)
        // As the next version of a template that is there: the base is the version shown, and a newer one saved meanwhile is said plainly.
        await page.getByRole('button',{name:'Save as template…',exact:true}).click()
        const again=page.getByRole('dialog',{name:'Save as template',exact:true})
        // Choosing a template to follow fills only what has not been typed in.
        await again.getByRole('textbox',{name:/^Description/}).fill('Typed first.')
        await again.getByRole('combobox',{name:'Save as',exact:true}).selectOption(tid)
        await expect(again.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('CSV export')
        await expect(again.getByRole('textbox',{name:/^Description/})).toHaveValue('Typed first.')
        await again.getByRole('combobox',{name:'Save as',exact:true}).selectOption('')
        await expect(again.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('Ship CSV export for billing')
        await again.getByRole('textbox',{name:'Name',exact:true}).fill('Mine')
        await again.getByRole('combobox',{name:'Save as',exact:true}).selectOption(tid)
        await expect(again.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('Mine')
        await again.getByRole('textbox',{name:'Name',exact:true}).fill('CSV export')
        await expect(again.getByRole('button',{name:'Save as version 4',exact:true})).toBeVisible()
        await shot(page,'save-as-version',width,theme)
        f.box.saveFail='changed'
        await again.getByRole('button',{name:'Save as version 4',exact:true}).click()
        await expect(again.getByRole('status')).toContainText('Version 4 was saved elsewhere. This now saves as version 5.')
        // Information, not an error: the neutral notice, not the warning colour.
        await expect(again.getByRole('status')).toHaveClass(/\bnotice\b/)
        await expect(again.getByRole('status')).not.toHaveClass(/\bwarning\b/)
        await expect(again.getByRole('button',{name:'Save as version 5',exact:true})).toBeVisible()
        await shot(page,'save-changed',width,theme)
        await again.getByRole('button',{name:'Save as version 5',exact:true}).click()
        await expect.poll(()=>f.box.saves.length).toBeGreaterThan(6)
        const sent=f.box.saves.at(-1)
        expect(sent).toMatchObject({outcomeId:oid,templateId:tid,baseVersion:4,name:'CSV export'})
        // The notice opens the template it names.
        await page.getByRole('button',{name:'Open it in Templates',exact:true}).click()
        await expect(templatesDialog(page).getByRole('heading',{name:'CSV export',level:3})).toBeVisible()
        await noOverflow(page)
      }finally{await w.context.close()}
    }
    // 2. The list, a template's page, a new version by renaming, comparing versions, and deleting.
    {
      const {f,w,page}=await open(width,theme)
      try{
        const dialog=await openTemplates(page)
        const row=dialog.getByRole('listitem').filter({hasText:'CSV export'})
        await expect(row).toContainText('Billing export: the API, the button and the tests.')
        await expect(row).toContainText('Version 3 · 2 pipelines · changed')
        await expect(row).toContainText(/Last used .+ \(version 2\)/)
        await expect(row).toContainText('Changed since you last used it (version 2 → 3)')
        await shot(page,'list',width,theme)
        if(!wide)await targets(page)
        await row.getByRole('button',{name:'CSV export',exact:true}).click()
        await expect(dialog.getByRole('heading',{name:'CSV export',level:3})).toBeFocused()
        // The name is said once: the dialog is titled Templates, and the page is headed by the template.
        await expect(dialog.getByRole('heading',{name:'CSV export'})).toHaveCount(1)
        await expect(dialog.getByText('Up to 8 reviews and 24 hours for each session.')).toBeVisible()
        await expect(dialog.getByRole('list',{name:'The plan'}).getByRole('listitem').filter({hasText:'Export API'}).first()).toContainText('Stops at Ready to merge.')
        await expect(dialog.getByRole('list',{name:'The plan'}).getByRole('listitem').filter({hasText:'Billing button'}).first()).toContainText('Merges by itself once checks pass.')
        await expect(dialog.getByRole('list',{name:'Stages of Export API'})).toContainText('Code')
        await expect(dialog.getByRole('list',{name:'Stages of Export API'})).toContainText('Standard')
        await expect(dialog.getByRole('list',{name:'Stages of Export API'})).toContainText('Go tests · revision rev2bbbb')
        await expect(dialog.getByRole('list',{name:'Stages of Export API'})).not.toContainText('Skill Go tests')
        await expect(dialog.locator('.head')).toContainText('Changed since you last used it (version 2 → 3)')
        await expect(dialog.getByRole('list',{name:'Criteria of Export API'})).toContainText('The export downloads as a CSV file.')
        await expect(dialog.getByRole('list',{name:'Criteria of Billing button'})).toHaveCount(0)
        await expect(dialog.getByText('No criteria, so this pipeline has no delivery review.')).toBeVisible()
        // The plan is read, not edited.
        await expect(dialog.getByRole('textbox',{name:/Pipeline 1/})).toHaveCount(0)
        await expect(dialog).toContainText('To change the plan, start from it, adjust, and save the finished outcome as a new version.')
        await shot(page,'template-page',width,theme)
        await scrollTo(page,'.limits');await shot(page,'template-page-plan',width,theme)
        await dialog.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=0})
        if(!wide)await targets(page)
        // Any two versions can be compared.
        await expect(dialog.getByRole('region',{name:'Compare versions'})).toContainText('Version 2 to version 3')
        await expect(dialog.getByRole('region',{name:'Version 2 to version 3'})).toContainText('Stages were Code, Review; now Code, Review, Test.')
        await expect(dialog.getByRole('region',{name:'Version 2 to version 3'})).toContainText('Criterion added: “Totals match the ledger.”')
        await expect(dialog.getByRole('region',{name:'Version 2 to version 3'})).toContainText('“Go tests” in Code moved from revision rev1aaaa to rev2bbbb')
        await expect(dialog.getByRole('region',{name:'Version 2 to version 3'})).toContainText('Review brief added: “Read every line of the export.”')
        await dialog.getByRole('combobox',{name:'From',exact:true}).selectOption('1')
        await expect.poll(()=>f.box.diffs.at(-1)).toBe('1-3')
        await expect(dialog.getByRole('region',{name:'Version 1 to version 3'})).toContainText('Criterion reworded: “The export downloads as CSV.” to “The export downloads as a CSV file.”')
        // The same version cannot be picked twice: it is not offered on the other side.
        await expect(dialog.getByRole('combobox',{name:'To',exact:true}).locator('option[value="1"]')).toHaveJSProperty('disabled',true)
        await expect(dialog.getByRole('combobox',{name:'From',exact:true}).locator('option[value="3"]')).toHaveJSProperty('disabled',true)
        await scrollTo(page,'section.compare');await shot(page,'compare-same-not-offered',width,theme)
        await dialog.getByRole('combobox',{name:'From',exact:true}).selectOption('2');await dialog.getByRole('combobox',{name:'To',exact:true}).selectOption('3')
        await expect.poll(()=>f.box.diffs.at(-1)).toBe('2-3')
        await scrollTo(page,'section.compare');await shot(page,'compare',width,theme)
        await dialog.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=0})
        // An older version is read as it was, and says it is not the latest.
        // Changing the version leaves the focus on the list of versions, and says so in the page's one status line.
        const statuses=await dialog.locator('[role=status]').count()
        await dialog.getByRole('combobox',{name:'Version',exact:true}).focus()
        await dialog.getByRole('combobox',{name:'Version',exact:true}).selectOption('1')
        await expect(dialog.getByRole('status').filter({hasText:'Showing version 1'})).toHaveCount(1)
        await expect(dialog.getByRole('combobox',{name:'Version',exact:true})).toBeFocused()
        expect(await dialog.locator('[role=status]').count()).toBe(statuses)
        // A version change is not a save: it is said in the meta line, never in the banner for save and delete results.
        await expect(dialog.locator('.head .meta')).toContainText('Showing version 1')
        await expect(dialog.locator('p.notice')).toHaveText('')
        await shot(page,'template-older-version',width,theme)
        await expect(dialog.getByText('Version 1 (the latest is 3)')).toBeVisible()
        await expect(dialog.getByRole('button',{name:'Use version 1',exact:true})).toBeVisible()
        await expect(dialog.getByRole('button',{name:'Rename or change the description',exact:true})).toHaveCount(0)
        // With the keyboard, arrows move through the list and only the version chosen with Enter is read; the focus comes back to the list.
        if(wide){
          const reads=f.box.reads.length
          await dialog.getByRole('combobox',{name:'Version',exact:true}).focus()
          await page.keyboard.press('ArrowUp');await page.keyboard.press('ArrowUp');await page.keyboard.press('ArrowUp')
          expect(f.box.reads.slice(reads)).toEqual([])
          await page.keyboard.press('Enter')
          await expect(dialog.locator('.head .meta')).toContainText('Showing version 3')
          expect(f.box.reads.slice(reads)).toEqual(['v3'])
          await expect(dialog.getByRole('combobox',{name:'Version',exact:true})).toBeFocused()
        }else await dialog.getByRole('combobox',{name:'Version',exact:true}).selectOption('3')
        // Renaming saves the same plan as the next version, with the base version it was shown.
        await dialog.getByRole('button',{name:'Rename or change the description',exact:true}).click()
        await expect(dialog.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('CSV export')
        await shot(page,'rename',width,theme)
        f.box.editFail='changed'
        await dialog.getByRole('textbox',{name:'Name',exact:true}).fill('Renamed export')
        await dialog.getByRole('button',{name:'Save as version 4',exact:true}).click()
        await expect(dialog.getByRole('alert')).toContainText('Version 4 was saved elsewhere. This now saves as version 5.')
        // What was typed stays, and the base is now the newer version.
        await expect(dialog.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('Renamed export')
        await dialog.getByRole('button',{name:'Save as version 5',exact:true}).click()
        const saved=templatesDialog(page).getByRole('status').filter({hasText:'Saved as version 5.'})
        await expect(saved).toBeVisible()
        // The form is gone, so the focus is on the notice, which is in view inside the dialog with its title.
        await expect(saved).toBeFocused()
        expect(await saved.evaluate(el=>{const r=el.getBoundingClientRect(),b=el.closest('[data-shell-body]').getBoundingClientRect();return r.top>=b.top&&r.bottom<=b.bottom})).toBe(true)
        await titleInView(page.locator('dialog[open]').last())
        await expect(dialog.getByRole('heading',{name:'Renamed export',level:3})).toBeVisible()
        expect(f.box.edits.map(e=>e.baseVersion)).toEqual([3,4])
        expect(f.box.edits[1].requestId).toMatch(/^[0-9a-f]{32}$/)
        expect(f.box.edits[1].requestId).not.toBe(f.box.edits[0].requestId)
        expect(f.box.edits[1].content).toMatchObject({name:'Renamed export',objective:'Ship CSV export for billing.'})
        expect(f.box.edits[1].content.pipelines).toHaveLength(2)
        await shot(page,'renamed',width,theme)
        // Delete says what it does to runs already started, and names the latest version it was shown.
        f.box.deleteFail='changed'
        await dialog.getByRole('button',{name:'Delete template…',exact:true}).click()
        const ask=page.getByRole('dialog',{name:'Delete “Renamed export”?',exact:true})
        await expect(ask).toContainText('Outcomes already started from it keep their own inputs and are not touched.')
        await expect(ask).toContainText('5 versions')
        await page.waitForTimeout(500);await shot(page,'delete-confirm',width,theme)
        // The destructive button wears the danger colour, not the primary accent.
        const paint=await ask.getByRole('button',{name:'Delete template',exact:true}).evaluate(el=>{const t=document.createElement('i');t.style.color='var(--bad)';el.after(t);const bad=getComputedStyle(t).color;t.remove();return {button:getComputedStyle(el).backgroundColor,bad}})
        expect(paint.button).toBe(paint.bad)
        await ask.getByRole('button',{name:'Delete template',exact:true}).click()
        await expect(dialog.getByRole('status').filter({hasText:'Version 6 was saved elsewhere, so nothing was deleted.'})).toBeVisible()
        await dialog.getByRole('button',{name:'Delete template…',exact:true}).click()
        await page.getByRole('dialog',{name:/^Delete “/}).getByRole('button',{name:'Delete template',exact:true}).click()
        await expect.poll(()=>f.box.deletes.length).toBe(2)
        expect(f.box.deletes.at(-1)).toBe('6')
        await expect(dialog.getByText('No templates yet.')).toBeVisible()
        await expect(dialog.getByRole('status').filter({hasText:'Deleted “Renamed export”. Outcomes already started from it were not touched.'})).toBeVisible()
        await expect(dialog).toContainText('Open a finished outcome’s pipeline and choose Save as template… in the bar at the foot of its detail.')
        await shot(page,'empty',width,theme)
        await noOverflow(page);await dialogFits(page)
      }finally{await w.context.close()}
    }
    // 3. Use at the latest version: the changes since the last use come first, the plan is read only, and the launch carries the choices.
    {
      const {f,w,page}=await open(width,theme)
      try{
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
        const outcome=outcomeDialog(page)
        await expect(outcome.getByRole('region',{name:'Changed since version 2, the one you last used'})).toContainText('Stages were Code, Review; now Code, Review, Test.')
        expect(f.box.diffs).toEqual(['last-3'])
        // Above the plan.
        const order=await outcome.evaluate(el=>{const a=el.querySelector('[aria-label^="Changed since"]'),b=el.querySelector('[aria-label="The plan"]');return !!(a.compareDocumentPosition(b)&Node.DOCUMENT_POSITION_FOLLOWING)})
        expect(order).toBe(true)
        await expect(outcome.getByRole('list',{name:'Stages of Export API'})).toContainText('Test')
        await expect(outcome.getByRole('textbox',{name:/Pipeline 1/})).toHaveCount(0)
        // The objective is labelled as on any plan, the limits are said once, and each pipeline says what it does at the end.
        await expect(outcome.locator('.objective')).toContainText('Outcome')
        await expect(outcome.getByText('Up to 8 reviews and 24 hours for each session.')).toHaveCount(1)
        await expect(outcome.getByRole('list',{name:'The plan'}).getByRole('listitem').filter({hasText:'Billing button'}).first()).toContainText('Merges by itself once checks pass.')
        // Which pipelines merge by themselves is one line beside Start, not part of the grey access paragraph.
        await expect(outcome.locator('.access')).toContainText('Gaffer cannot run commands or answer approvals.')
        await expect(outcome.locator('.access')).not.toContainText('Gaffer cannot merge')
        await expect(outcome.locator('.access')).not.toContainText('merges by itself')
        await expect(outcome.locator('p.merging')).toHaveText('“Billing button” merges by itself once checks pass.')
        expect((await outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true}).getAttribute('aria-describedby')).split(' ')).toContain(await outcome.locator('p.merging').getAttribute('id'))
        // Each pipeline is one line; the card is open for the one that changed since the version last used.
        const pipelines=outcome.locator('details.pipeline')
        await expect(pipelines.nth(0).locator(':scope>summary')).toHaveText('Export API · Code, Review, Test · 2 criteria · stops at Ready to merge')
        await expect(pipelines.nth(1).locator(':scope>summary')).toHaveText('Billing button · Code · 0 criteria · merges by itself')
        await expect(pipelines.nth(0)).toHaveAttribute('open','')
        await expect(pipelines.nth(1)).not.toHaveAttribute('open','')
        // What changed is marked where it is in the plan, and the panel above still sums it up.
        await expect(outcome.getByRole('region',{name:'Changed since version 2, the one you last used'})).toContainText('Review brief added: “Read every line of the export.”')
        await expect(outcome.getByRole('list',{name:'Criteria of Export API'}).getByRole('listitem').filter({hasText:'Totals match the ledger.'})).toContainText('Added')
        await expect(outcome.getByRole('list',{name:'Criteria of Export API'}).getByRole('listitem').filter({hasText:'The export downloads as a CSV file.'}).locator('.badge')).toHaveCount(0)
        await expect(outcome.getByRole('list',{name:'Stages of Export API'})).toContainText('Go tests · revision rev2bbbb Revision changed')
        await expect(outcome.getByRole('button',{name:'Decline proposal',exact:true})).toHaveCount(0)
        await expect(outcome.getByText('Launch readiness on workshop.')).toBeVisible()
        await expect(outcome.getByRole('combobox',{name:'Project',exact:true})).toHaveValue('p1')
        await expect(outcome.getByRole('option',{name:'A repository from GitHub…'})).toHaveCount(0)
        await shot(page,'use-latest',width,theme)
        await scrollTo(page,'.limits');await shot(page,'use-latest-plan-merge-limits',width,theme)
        await scrollTo(page,'.criteria');await shot(page,'use-latest-marks',width,theme)
        await scrollTo(page,'.access');await shot(page,'use-latest-access',width,theme)
        await outcome.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=0})
        await outcome.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=el.scrollHeight});await shot(page,'use-latest-end',width,theme)
        await outcome.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=0})
        if(!wide)await targets(page)
        // The crew is chosen here as for any plan: the editor and the effort are there.
        await expect(outcome.locator('details.crew summary')).toContainText('Choose who does what')
        await expect(outcome.getByRole('combobox',{name:'Effort',exact:true})).toBeVisible()
        await outcome.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
        await outcome.getByRole('checkbox',{name:/Have Gaffer report across this outcome/}).check()
        const start=outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true})
        await expect(start).toBeEnabled()
        await start.click()
        await expect.poll(()=>f.box.launches.length).toBe(1)
        const sent=f.box.launches[0]
        expect(sent.requestId).toMatch(/^[0-9a-f]{32}$/)
        expect(sent).toEqual({requestId:sent.requestId,version:3,projectId:'p1',tool:'claude',model:'',provider:'',trust:false,yolo:false,autoReport:true,automation:true,balanceBelow:20,crew:{light:{tool:'claude'},standard:{tool:'claude'},heavy:{tool:'claude'}},confirmed:true})
        // The plan is the template's: no objective, sessions or limits travel with the launch, and the approval route was not used.
        await expect(outcome.getByRole('status').filter({hasText:/Starting|under way|started/}).first()).toBeVisible()
        await noOverflow(page);await dialogFits(page)
      }finally{await w.context.close()}
    }
    // 4. An older version is used as it was, with the changes since the last use read against it.
    {
      const {f,w,page}=await open(width,theme)
      try{
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'CSV export',exact:true}).click()
        await dialog.getByRole('combobox',{name:'Version',exact:true}).selectOption('1')
        await dialog.getByRole('button',{name:'Use version 1',exact:true}).click()
        const outcome=outcomeDialog(page)
        await expect(outcome.getByRole('combobox',{name:'Template version',exact:true})).toHaveValue('1')
        await expect(outcome.getByRole('list',{name:'Criteria of Export API'})).toContainText('The export downloads as CSV.')
        await expect(outcome.getByRole('list',{name:'Stages of Export API'})).not.toContainText('Test')
        await expect(outcome.getByRole('region',{name:'Differs from version 2, the one you last used'})).toBeVisible()
        expect(f.box.diffs.at(-1)).toBe('last-1')
        await expect(outcome.getByRole('list',{name:'Criteria of Export API'}).getByRole('listitem').filter({hasText:'The export downloads as CSV.'})).toContainText('Reworded')
        await shot(page,'use-older',width,theme)
        // Choosing the version last used says nothing changed, so nothing is drawn.
        await outcome.getByRole('combobox',{name:'Template version',exact:true}).selectOption('2')
        await expect.poll(()=>f.box.reads.at(-1)).toBe('v2')
        await expect(outcome.getByRole('region',{name:/^Changed since/})).toHaveCount(0)
        // With the keyboard, arrows move through the list and only the version chosen with Enter is read; the focus comes back to the list.
        if(width>=1100){
          const reads=f.box.reads.length
          await outcome.getByRole('combobox',{name:'Template version',exact:true}).focus()
          await page.keyboard.press('ArrowDown');await page.keyboard.press('ArrowDown')
          expect(f.box.reads.slice(reads)).toEqual([])
          await page.keyboard.press('Enter')
          await expect(outcome.getByRole('list',{name:'Criteria of Export API'})).toContainText('The export downloads as CSV.')
          expect(f.box.reads.slice(reads)).toEqual(['v1'])
          await expect(outcome.getByRole('combobox',{name:'Template version',exact:true})).toBeFocused()
        }else await outcome.getByRole('combobox',{name:'Template version',exact:true}).selectOption('1')
        await outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true}).click()
        await expect.poll(()=>f.box.launches.length).toBe(1)
        expect(f.box.launches[0].version).toBe(1)
        expect(f.box.launches[0]).not.toHaveProperty('skippedSkills')
      }finally{await w.context.close()}
    }
    // 5. A template never used shows no changes, and asks for none. It is reached from the first step too, without losing what was typed.
    {
      const {f,w,page}=await open(width,theme,{lastUsed:0})
      try{
        await page.goto(base+'/factory')
        await page.getByRole('button',{name:'New Factory outcome',exact:true}).first().click()
        const outcome=outcomeDialog(page)
        await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill('Something typed first.')
        await outcome.getByRole('button',{name:'Start from a template',exact:true}).click()
        const picker=page.getByRole('dialog',{name:'Templates',exact:true})
        await expect(picker.getByText('Not used yet')).toBeVisible()
        await shot(page,'start-from-template',width,theme)
        await picker.getByRole('button',{name:'Use CSV export',exact:true}).click()
        await expect(outcome.getByText('CSV export',{exact:true}).first()).toBeVisible()
        await expect(outcome.getByRole('region',{name:/^Changed since/})).toHaveCount(0)
        expect(f.box.diffs).toEqual([])
        // Nothing changed and nothing is missing, so every pipeline is one line, its card closed.
        await expect(outcome.locator('details.pipeline[open]')).toHaveCount(0)
        await expect(outcome.locator('details.pipeline')).toHaveCount(2)
        await expect(outcome.locator('p.merging')).toHaveText('“Billing button” merges by itself once checks pass.')
        await shot(page,'use-first',width,theme)
        // Back returns to the first step with the typed words still there.
        await outcome.getByRole('button',{name:'Back',exact:true}).click()
        await expect(outcome.getByRole('textbox',{name:'Outcome',exact:true})).toHaveValue('Something typed first.')
      }finally{await w.context.close()}
    }
    // 6. Skills no longer approved: said per stage, the start waits for the person's word, and only those keys are sent; a change while looking is read again.
    {
      const {f,w,page}=await open(width,theme,{gaps:[GAP]})
      try{
        f.box.gapsAfter=[GAP,GAP2]
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
        const outcome=outcomeDialog(page)
        await expect(outcome.getByRole('list',{name:'Stages of Export API'})).toContainText('“Go tests” has a newer approved revision. This template keeps the older one, so the stage starts without it.')
        const start=outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true})
        await expect(start).toBeDisabled()
        await expect(outcome.getByText('Tick “Start without these skills” to go on, or choose another version.')).toBeVisible()
        // The tick box sits directly under the plan whose gaps it concerns, and is described by them; the line by Start still names it.
        const near=await outcome.evaluate(el=>{const m=[...el.querySelectorAll('label.trust')].find(l=>/Start without these skills/.test(l.textContent)).getBoundingClientRect(),n=el.querySelector('[aria-label="The plan"]').getBoundingClientRect();return {gap:m.top-n.bottom}})
        expect(near.gap).toBeGreaterThanOrEqual(0);expect(near.gap).toBeLessThan(40)
        await expect(outcome.getByRole('checkbox',{name:/Start without these skills/})).toHaveAccessibleDescription(/“Go tests” has a newer approved revision\. This template keeps the older one, so the stage starts without it\./)
        await expect(outcome.locator('.status')).toHaveText('Tick “Start without these skills” to go on, or choose another version.')
        await outcome.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
        await shot(page,'skill-gap',width,theme)
        await scrollTo(page,'.gap');await shot(page,'skill-gap-marks',width,theme)
        await outcome.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=0})
        await outcome.locator('[data-shell-body]').evaluate(el=>{el.scrollTop=el.scrollHeight});await shot(page,'skill-gap-end',width,theme)
        await outcome.getByRole('checkbox',{name:/Start without these skills/}).check()
        await expect(start).toBeEnabled()
        f.box.launchFail='skills'
        const reads=f.box.reads.length
        await start.click()
        // The other choices are kept, the list is the new one, and the person has to agree again.
        await expect(outcome.getByRole('alert')).toContainText('The approved skills changed while you were looking. The plan above now shows the current list.')
        await expect(outcome.getByRole('list',{name:'Stages of Billing button'})).toContainText('“Lint” is not available now (archived, removed or switched off), so the stage starts without it.')
        // The pipeline with the new gap opens, and the tick box is described by both gaps.
        await expect(outcome.locator('details.pipeline').nth(1)).toHaveAttribute('open','')
        await expect(outcome.getByRole('checkbox',{name:/Start without these skills/})).toHaveAccessibleDescription(/Go tests.+Lint/)
        await expect(outcome.getByRole('radio',{name:'Gaffer decides',exact:true})).toBeChecked()
        await expect(outcome.getByRole('checkbox',{name:/Start without these skills/})).not.toBeChecked()
        await expect(start).toBeDisabled()
        // The refusal carried the gaps as they are now, so the template was not read again.
        expect(f.box.reads.length).toBe(reads)
        await shot(page,'skills-changed',width,theme)
        await outcome.getByRole('checkbox',{name:/Start without these skills/}).check()
        await start.click()
        await expect.poll(()=>f.box.launches.length).toBe(2)
        expect(f.box.launches[0]).toMatchObject({skippedSkills:['0:0:library:go-tests']})
        expect(f.box.launches[1]).toMatchObject({skippedSkills:['0:0:library:go-tests','1:0:builtin:lint'],automation:true})
        expect(f.box.launches[1].requestId).not.toBe(f.box.launches[0].requestId)
        // The template's own page tells of the gap too.
      }finally{await w.context.close()}
    }
    // 7. A launch lost in the post is sent again with the same request ID.
    {
      const {f,w,page}=await open(width,theme)
      try{
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
        const outcome=outcomeDialog(page)
        f.box.fail502=1
        await outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true}).click()
        await expect(outcome.getByRole('alert')).toContainText('Trying again is safe: it starts once.')
        await outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true}).click()
        await expect.poll(()=>f.box.launches.length).toBe(2)
        expect(f.box.launches[1].requestId).toBe(f.box.launches[0].requestId)
      }finally{await w.context.close()}
    }
    // 8. Each run says which template and version it came from, and says so plainly when that is gone.
    {
      const {w,page}=await open(width,theme)
      try{
        await page.goto(`${base}/factory?pipeline=${'1'.repeat(32)}.b`)
        const line=page.getByText(/^From template/).first()
        await expect(line).toContainText('From template “CSV export”, version 2.')
        await shot(page,'run-from-template',width,theme)
        // Saving it again follows the template it came from, while that template is kept.
        await page.getByRole('button',{name:'Save as template…',exact:true}).click()
        const save=page.getByRole('dialog',{name:'Save as template',exact:true})
        await expect(save.getByRole('combobox',{name:'Save as',exact:true})).toHaveValue(tid)
        await expect(save).toContainText('Started from version 2.')
        await expect(save.getByRole('textbox',{name:'Name',exact:true})).toHaveValue('CSV export')
        await expect(save.getByRole('button',{name:'Save as version 4',exact:true})).toBeVisible()
        await shot(page,'save-from-template',width,theme)
        await save.getByRole('button',{name:'Cancel',exact:true}).click()
        await line.getByRole('button',{name:'CSV export',exact:true}).click()
        await expect(templatesDialog(page).getByRole('heading',{name:'CSV export',level:3})).toBeVisible()
        await page.keyboard.press('Escape')
        await page.goto(`${base}/factory?pipeline=${'2'.repeat(32)}.c`)
        // The name the run recorded at its start outlives the template.
        await expect(page.getByText('From template “Old CSV export”, version 1. That template has since been deleted.')).toBeVisible()
        await shot(page,'run-template-deleted',width,theme)
        await page.goto(`${base}/factory?pipeline=${'4'.repeat(32)}.e`)
        await expect(page.getByText('From a template that has since been deleted, version 1.')).toBeVisible()
        await page.goto(`${base}/factory?pipeline=${'3'.repeat(32)}.d`)
        await expect(page.getByText('From template “Older export”, version 1, which is no longer kept.')).toBeVisible()
        await page.goto(FINISHED)
        await expect(page.getByText(/^From template/)).toHaveCount(0)
        // A finished outcome the server would refuse (a pipeline failed) is not offered as a template; one it would keep is.
        await expect(page.getByRole('button',{name:'Save as template…',exact:true})).toBeVisible()
        await page.goto(`${base}/factory?pipeline=${'5'.repeat(32)}.f`)
        await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
        await expect(page.getByRole('button',{name:'Archive outcome',exact:true})).toBeVisible()
        await expect(page.getByRole('button',{name:'Save as template…',exact:true})).toHaveCount(0)
        await noOverflow(page)
      }finally{await w.context.close()}
    }
    // 9. The crew is the person's for this start, as for any plan: a tier that cannot start is fixed from the readiness
    // panel with Choose who does what, and the launch sends the crew that is shown.
    {
      const {f,w,page}=await open(width,theme,{codex:true,crew:{light:{},standard:{},heavy:{tool:'codex',model:'gpt-x'}}})
      try{
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
        const outcome=outcomeDialog(page),causes=outcome.locator('section.readiness li')
        const start=outcome.getByRole('button',{name:'Start 2 pipelines from this template',exact:true})
        await expect(outcome.getByText('Launch readiness on workshop.')).toBeVisible()
        await expect(start).toBeEnabled()
        // Codex leaves the machine; the plan step reads the machines again when it is entered.
        f.box.codexGone=true
        await outcome.getByRole('button',{name:'Back',exact:true}).click()
        await outcome.getByRole('button',{name:'Start from a template',exact:true}).click()
        await page.getByRole('dialog',{name:'Templates',exact:true}).getByRole('button',{name:'Use CSV export',exact:true}).click()
        await expect(causes.first()).toHaveText(/^✕ Cannot start · Heavy crew, used by Review\. Codex is not on workshop now\. Choose who does what\s?for the Heavy crew$/)
        await expect(start).toBeDisabled()
        await causes.first().scrollIntoViewIfNeeded()
        await shot(page,'crew-blocked',width,theme)
        await causes.first().getByRole('button',{name:'Choose who does what for the Heavy crew',exact:true}).click()
        await expect(outcome.locator('details.crew')).toHaveAttribute('open','')
        await titleInView(page.locator('dialog[open]').last())
        const heavy=outcome.getByRole('combobox',{name:'Heavy agent',exact:true})
        await expect(heavy).toBeFocused()
        await titleInView(page.locator('dialog[open]').last())
        await shot(page,'crew-title-in-view',width,theme)
        await heavy.selectOption('claude')
        await expect(start).toBeEnabled()
        await shot(page,'crew-fixed',width,theme)
        await start.click()
        await expect.poll(()=>f.box.launches.length).toBe(1)
        expect(f.box.launches[0].crew).toEqual({light:{tool:'claude'},standard:{tool:'claude'},heavy:{tool:'claude'}})
        await noOverflow(page);await dialogFits(page)
      }finally{await w.context.close()}
    }
    // 10. The version last used is no longer kept: the changes start at the oldest kept, and say older ones are not shown.
    {
      const {f,w,page}=await open(width,theme,{older:true})
      try{
        const dialog=await openTemplates(page)
        await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
        const outcome=outcomeDialog(page)
        const changes=outcome.getByRole('region',{name:'Changed since version 2, the oldest kept'})
        await expect(changes).toContainText('Version 1, the one you last used, is no longer kept, so changes before version 2 are not shown.')
        await expect(changes).toContainText('Stages were Code, Review; now Code, Review, Test.')
        expect(f.box.diffs).toEqual(['last-3'])
        await shot(page,'use-older-dropped',width,theme)
      }finally{await w.context.close()}
    }
  }
  // 11. Keyboard: the Templates button, a template and Use are all reached with Tab, and Escape puts the focus back.
  {
    const {w,page}=await open(1280,'light')
    try{
      await page.goto(base+'/factory')
      await page.getByRole('heading',{name:'Factory',level:1}).waitFor()
      await tabTo(page,'Templates')
      await page.keyboard.press('Enter')
      const dialog=page.getByRole('dialog',{name:'Templates',exact:true})
      await expect(dialog).toBeVisible()
      await tabTo(page,'CSV export')
      await tabTo(page,'Use CSV export')
      await page.keyboard.press('Enter')
      await expect(outcomeDialog(page)).toBeVisible()
      await expect(outcomeDialog(page).getByRole('list',{name:'Stages of Export API'})).toBeVisible()
      await tabTo(page,'Start 2 pipelines from this template',160)
    }finally{await w.context.close()}
  }
  // 11b. A template named on the address opens on it, and the address is cleared once the template is used.
  {
    const {w,page}=await open(1280,'light')
    try{
      await page.goto(`${base}/factory?template=${tid}`)
      await page.getByRole('dialog',{name:'Templates',exact:true}).getByRole('button',{name:'Use version 3',exact:true}).click()
      await expect(outcomeDialog(page)).toBeVisible()
      expect(page.url()).not.toContain('template=')
    }finally{await w.context.close()}
  }
  // 12. A phone (390 and 360 wide): the templates, a template's page and the launch fit the width, with 44 px targets and no sideways scroll.
  for(const phone of [390,360]){
    const {w,page}=await open(phone,'light',{gaps:[GAP]})
    try{
      const dialog=await openTemplates(page)
      await noOverflow(page);await dialogFits(page);await targets(page)
      await dialog.getByRole('button',{name:'CSV export',exact:true}).click()
      await noOverflow(page);await dialogFits(page);await targets(page)
      await dialog.getByRole('button',{name:'Use version 3',exact:true}).click()
      const outcome=outcomeDialog(page)
      await expect(outcome.getByRole('list',{name:'Stages of Export API'})).toBeVisible()
      await noOverflow(page);await dialogFits(page)
      const wide=await page.evaluate(()=>[...document.querySelectorAll('dialog[open]:last-of-type *')].filter(el=>el.getBoundingClientRect().right>document.documentElement.clientWidth+1).length)
      expect(wide).toBe(0)
      await targets(page)
    }finally{await w.context.close()}
  }
  // 13. A pipeline marked changed in a way the diff does not describe still says so, with a way to compare the versions.
  for(const [width,theme] of [[1280,'light'],[390,'dark']]){
    const {w,page}=await open(width,theme,{other:true})
    try{
      const dialog=await openTemplates(page)
      await dialog.getByRole('button',{name:'Use CSV export',exact:true}).click()
      const changes=outcomeDialog(page).getByRole('region',{name:'Changed since version 2, the one you last used'})
      await expect(changes.getByRole('listitem').filter({hasText:'Billing button'})).toHaveText('Billing button · Something else in this pipeline changed. Compare the versions')
      await expect(outcomeDialog(page).locator('details.pipeline').nth(1)).toHaveAttribute('open','')
      await shot(page,'changes-other',width,theme)
      await changes.getByRole('button',{name:'Compare the versions',exact:true}).click()
      const page2=page.getByRole('dialog',{name:'Templates',exact:true})
      await expect(page2.getByRole('heading',{name:'CSV export',level:3})).toBeVisible()
      await expect(page2.getByRole('region',{name:'Compare versions'})).toBeVisible()
    }finally{await w.context.close()}
  }
  console.log('Factory templates: save from a finished outcome, the list and a template’s page, rename and delete, use with the changes since the last use, an older version, skill gaps, the launch request and its crew, and the run’s template line pass.')
}finally{await browser.close()}
