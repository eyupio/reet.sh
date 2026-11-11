// Gaffer's use of reet's own tools: the per-tool choice in Gaffer settings (Off, Ask me, Allow), and the call that
// waits in the chat for the person's approval.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const captures=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-tools'
await mkdir(captures,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),cid='c'.repeat(32),tid='d'.repeat(32),toolId='e'.repeat(32)
const provider={id:pid,revision:1,name:'My OpenCode',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'glm-5',reasoningEffort:'',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
const catalogue=()=>[
 {name:'reet_list_factory',title:'List Factory outcomes',description:'List the Factory’s outcomes with their pipelines.',readOnly:true,destructive:false,group:'Factory: read',mode:'off'},
 {name:'reet_get_factory_outcome',title:'Read one outcome',description:'Read one outcome and its pipelines.',readOnly:true,destructive:false,group:'Factory: read',mode:'off'},
 {name:'reet_stop_outcome',title:'Stop an outcome',description:'Stop an outcome.',readOnly:false,destructive:false,group:'Factory: control',mode:'off'},
 {name:'reet_approve_pipeline_merge',title:'Approve a pipeline’s merge',description:'Record the user’s approval for a pipeline that is Ready to merge.',readOnly:false,destructive:false,group:'Factory: merge',mode:'off'},
]
function fixture(){
 const box={tools:catalogue(),saves:[],answers:[],failSave:false}
 const turn={id:tid,conversationId:cid,requestRevision:1,prompt:'Stop the export outcome.',answer:'I’ve asked to stop it. It waits for your approval.',state:'completed',providerId:pid,providerName:'My OpenCode',model:'glm-5',inputTokens:10,outputTokens:5,usageReported:true,cut:false,startedAt:now,updatedAt:now,finishedAt:now,
  tools:[{id:'f'.repeat(32),name:'reet_list_factory',args:'{}',mode:'allow',state:'ran',result:'{"outcomes":[{"id":"abc"}]}',at:now},{id:toolId,name:'reet_stop_outcome',args:'{"confirm":true,"outcome_id":"abc"}',mode:'ask',state:'pending',at:now}]}
 const view={conversation:{id:cid,sessionId:'',providerId:pid,model:'',revision:2,createdAt:now,updatedAt:now},turns:[turn]}
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/tools':request=>{
   if(request.method()==='GET')return {tools:box.tools}
   const input=request.postDataJSON();box.saves.push(input)
   if(box.failSave)return {status:500,json:{error:'Gaffer’s tools could not be saved.'}}
   box.tools=box.tools.map(tool=>({...tool,mode:input.modes[tool.name]||'off'}));return {tools:box.tools}
  },
  '/api/v2/gaffer/conversations':request=>new URL(request.url()).searchParams.get('all')==='true'?{conversations:[view.conversation]}:view,
  [`/api/v2/gaffer/conversations/${cid}`]:view,
  [`/api/v2/gaffer/conversations/${cid}/turns/${tid}/tools/${toolId}`]:request=>{
   const input=request.postDataJSON();box.answers.push(input)
   turn.tools[1]={...turn.tools[1],state:input.approve?'ran':'declined',...(input.approve?{result:'{"outcome":{"state":"stopped"}}'}:{})};return turn
  },
 }
 return {box,routes,turn}
}

try {
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  // Settings: every tool starts off; one tool, then a whole group, is set, and the whole choice is sent each time.
  const f=fixture()
  const w=await openWorkspace(browser,{width,theme,routes:f.routes})
  await w.page.goto(`${base}/account/gaffer`)
  const tools=w.page.getByRole('region',{name:'Tools Gaffer can use',exact:true})
  await expect(tools).toContainText('0 allowed · 0 ask first · 4 off')
  await expect(tools).toContainText('Gaffer’s goal reviews never get these tools')
  const pick=(title,choice)=>tools.getByRole('radiogroup',{name:`${title}: what Gaffer may do`,exact:true}).getByRole('radio',{name:choice,exact:true})
  // One tool: three choices in one small control, the chosen one marked.
  await expect(pick('List Factory outcomes','Off')).toHaveAttribute('aria-checked','true')
  await pick('List Factory outcomes','Allow').click()
  await expect(pick('List Factory outcomes','Allow')).toHaveAttribute('aria-checked','true')
  await expect(tools).toContainText('1 allowed · 0 ask first · 3 off')
  expect(f.box.saves).toEqual([{modes:{reet_list_factory:'allow'}}])
  // A mixed group reads Custom; one choice sets the whole group.
  const readAll=tools.getByLabel('Set all tools in Factory: read',{exact:true})
  await expect(readAll).toHaveValue('')
  await readAll.selectOption('ask')
  await expect(tools).toContainText('0 allowed · 2 ask first · 2 off')
  expect(f.box.saves[1]).toEqual({modes:{reet_list_factory:'ask',reet_get_factory_outcome:'ask'}})
  // One choice sets every tool, and Off for all sends an empty choice.
  const every=tools.getByLabel('Set every tool',{exact:true})
  await expect(every).toHaveValue('')
  await every.selectOption('ask')
  await expect(tools).toContainText('0 allowed · 4 ask first · 0 off');await expect(every).toHaveValue('ask')
  expect(Object.keys(f.box.saves[2].modes).length).toBe(4)
  await every.selectOption('off')
  await expect(tools).toContainText('0 allowed · 0 ask first · 4 off');expect(f.box.saves[3]).toEqual({modes:{}})
  // A tool's description is behind its name, and a group can be folded away.
  await expect(tools.getByText('Read one outcome and its pipelines.')).toBeHidden()
  await tools.getByText('Read one outcome',{exact:true}).click()
  await expect(tools.getByText('Read one outcome and its pipelines.')).toBeVisible()
  await tools.getByRole('button',{name:'Hide the tools in Factory: read',exact:true}).click()
  await expect(pick('List Factory outcomes','Allow')).toHaveCount(0)
  await tools.getByRole('button',{name:'Show the tools in Factory: read',exact:true}).click()
  // Allowing a merge approval without asking is said plainly.
  await pick('Approve a pipeline’s merge','Allow').click()
  await expect(tools.getByRole('status').filter({hasText:'Allowed without asking'})).toContainText('Approve a pipeline’s merge')
  // A save that fails puts the choice back and says so.
  f.box.failSave=true
  await pick('Approve a pipeline’s merge','Off').click()
  await expect(tools.getByRole('alert')).toContainText('could not be saved')
  await expect(pick('Approve a pipeline’s merge','Allow')).toHaveAttribute('aria-checked','true')
  await w.page.screenshot({path:`${captures}/settings-${width}-${theme}.png`,fullPage:true})

  // Chat: a tool that ran shows its result; one that asks waits with its exact arguments and runs on Approve, once.
  await w.page.goto(`${base}/`)
  await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
  const panel=w.page.getByRole('complementary',{name:/Gaffer/}).or(w.page.getByRole('dialog',{name:/Gaffer/})).first()
  await expect(panel).toContainText('Gaffer used');await expect(panel).toContainText('reet_list_factory')
  await expect(panel).toContainText('Gaffer asks to use');await expect(panel).toContainText('"outcome_id": "abc"')
  await expect(panel).toContainText('Nothing has run. Approve runs exactly this call once.')
  expect(f.box.answers).toEqual([])
  await panel.getByRole('button',{name:'Approve',exact:true}).click()
  await expect(panel.getByRole('button',{name:'Approve',exact:true})).toHaveCount(0)
  expect(f.box.answers).toEqual([{approve:true}])
  await expect(panel).not.toContainText('Gaffer asks to use')
  await w.page.screenshot({path:`${captures}/chat-${width}-${theme}.png`})
  expect(w.errors).toEqual([])
  await w.context.close()

  // Declining runs nothing and says so.
  const g=fixture()
  const d=await openWorkspace(browser,{width,theme,routes:g.routes})
  await d.page.goto(`${base}/`)
  await d.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
  const second=d.page.getByRole('complementary',{name:/Gaffer/}).or(d.page.getByRole('dialog',{name:/Gaffer/})).first()
  await second.getByRole('button',{name:'Decline',exact:true}).click()
  await expect(second).toContainText('You declined');expect(g.box.answers).toEqual([{approve:false}])
  expect(d.errors).toEqual([])
  await d.context.close()
 }
 console.log('Gaffer tools: the per-tool choice in settings (one tool, a group, every tool, the merge warning, a failed save) and a call approved once or declined in the chat passed on desktop light and phone dark.')
} finally { await browser.close() }
