import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const captures = process.env.REET_UI_SCREENSHOTS || '../work/gaffer/settings-screenshots'
await mkdir(captures, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now()
const presets = [
  { id:'openai', name:'OpenAI', kind:'openai', baseUrl:'https://api.openai.com/v1', help:'Use an OpenAI API key.', localOnly:false },
  { id:'anthropic', name:'Anthropic', kind:'anthropic', baseUrl:'https://api.anthropic.com', help:'Use an Anthropic key.', localOnly:false },
  { id:'ollama-cloud', name:'Ollama Cloud', kind:'openai_compatible', baseUrl:'https://ollama.com/v1', help:'Use an Ollama Cloud key.', localOnly:false },
  { id:'opencode-zen', name:'OpenCode Zen', kind:'openai_compatible', baseUrl:'https://opencode.ai/zen/v1', help:'Choose a supported model.', localOnly:false },
  { id:'ollama-local', name:'Local Ollama', kind:'openai_compatible', baseUrl:'http://localhost:11434/v1', help:'Localhost means the controller.', localOnly:true },
]
function state(seed = false) {
  const box = { providers: seed ? [
    { id:'a'.repeat(32),revision:1,name:'My OpenCode',kind:'openai_compatible',baseUrl:'https://opencode.ai/zen/v1',model:'glm-5',reasoningEffort:'low',enabled:true,isDefault:true,localOnly:false,allowStatus:true,allowSessions:true,allowCode:false,hasKey:true,createdAt:now,updatedAt:now,lastCheck:{at:now,ok:true,model:'glm-5',latencyMs:482,usageReported:true} },
    { id:'b'.repeat(32),revision:1,name:'Workshop Ollama',kind:'openai_compatible',baseUrl:'http://localhost:11434/v1',model:'qwen3:14b',reasoningEffort:'',enabled:false,isDefault:false,localOnly:true,allowStatus:true,allowSessions:false,allowCode:false,hasKey:false,createdAt:now,updatedAt:now },
  ] : [], saves:[], models:[], failModels:false, failSave:false, gate:null }
  const answer = () => ({ providers:box.providers, presets, available:true, canUseLocal:true, maxProviders:12 })
  const routes = {
    '/api/v2/gaffer/providers': request => {
      if (request.method() === 'GET') return answer()
      const input = request.postDataJSON(); box.saves.push(input)
      if (box.failSave) return { status:409,json:{error:'This provider changed; reload it.'} }
      const { apiKey,clearKey,makeDefault,...record } = input
      box.providers.push({...record,id:'c'.repeat(32),revision:1,isDefault:!!makeDefault,hasKey:!!apiKey,createdAt:now,updatedAt:now})
      return answer()
    },
    '/api/v2/gaffer/providers/models': async request => {
      box.models.push(request.postDataJSON())
      if (box.gate) await box.gate
      if (box.failModels) return {status:502,json:{error:'This endpoint does not provide a model list.'}}
      return {models:['model-alpha','model-beta','model-zeta']}
    },
  }
  for (const id of ['a'.repeat(32),'b'.repeat(32),'c'.repeat(32)]) {
    routes[`/api/v2/gaffer/providers/${id}`] = request => {
      if (request.method() === 'DELETE') { box.providers = box.providers.filter(p => p.id !== id); return answer() }
      const input=request.postDataJSON(); const {apiKey,clearKey,...record}=input
      box.providers=box.providers.map(p=>p.id===id?{...p,...record,revision:p.revision+1,hasKey:clearKey?false:apiKey?true:p.hasKey}:p)
      return answer()
    }
    for (const action of ['test','default','enabled']) routes[`/api/v2/gaffer/providers/${id}/${action}`] = request => {
      const input=request.postDataJSON()
      box.providers=box.providers.map(p=>action==='default'?{...p,isDefault:p.id===id}:p.id!==id?p:action==='test'?{...p,lastCheck:{at:now,ok:true,model:p.model,latencyMs:148,usageReported:false}}:{...p,enabled:input.enabled,revision:p.revision+1})
      return answer()
    }
  }
  return {box,routes}
}

// The crew's choices come from the account's machines: Claude Code on both (their efforts differ), OpenCode on one with
// no effort setting, Codex on neither. Model lists are what the controller kept for each machine.
const effortOf=values=>[{id:'effort',category:'thought_level',values:['default',...values]}]
const crewMachine=(agentId,host,agents)=>({agentId,host,ready:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,healthEnabled:false,readiness:'ready',acp:true,acpStartOptions:true,acpAgents:agents}})
const crewMachines=[
  crewMachine('m1','studio',[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:effortOf(['high','low','max'])},{tool:'opencode',adapter:'opencode',state:'available'}]),
  crewMachine('m2','laptop',[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:effortOf(['medium','high'])}]),
]
const kept=(agentId,host,ids)=>({agentId,host,state:'listed',models:ids.map(id=>({id,provider:id.includes('/')?id.slice(0,id.indexOf('/')):''})),more:false,source:'list',version:'1',fetchedAt:now,checkedAt:now,reason:'',sentence:''})
const crewModels={claude:[kept('m1','studio',['model-strong','model-quick']),kept('m2','laptop',['model-quick','model-deep'])],opencode:[kept('m1','studio',['zen/model-a','zen/model-b'])]}
// Heavy is saved with a backup on OpenCode and a further backup on a model no machine lists, at an effort none offers.
const savedCrew={heavy:{tool:'claude',model:'model-strong',effort:'high',backup:{tool:'opencode',model:'',effort:''},moreBackups:[{tool:'claude',model:'retired-model',effort:'ultra'}]},standard:{},light:{},balanceBelow:20,gafferDecides:false,updatedAt:now}
function crewRoutes(box,{failMachines=false,failModels=false,machines=crewMachines}={}){
  return {
    // Read by the rest of the page: the providers, Gaffer's own tools and the sign-in identity. Answered so nothing is
    // left unmocked.
    '/api/v2/gaffer/providers':{providers:[],presets,available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/gaffer/tools':{tools:[]},
    '/api/v2/auth/oidc/identity':{enabled:false},
    '/api/v2/gaffer/crew':request=>{if(request.method()==='PUT'){box.saves.push(request.postDataJSON());return {crew:{...box.saves.at(-1),updatedAt:now},saved:true,tiers:[],defaults:{}}}return {crew:box.crew,saved:true,tiers:[],defaults:{}}},
    '/api/v2/terminal/machines':()=>failMachines?{status:500,json:{error:'machines unavailable'}}:{machines},
    '/api/v2/models':request=>{const tool=new URL(request.url()).searchParams.get('tool');box.modelReads.push(tool);return failModels?{status:500,json:{error:'models unavailable'}}:{machines:crewModels[tool]??[]}},
  }
}
const crewShots=process.env.REET_CREW_SCREENSHOTS||captures
const docImages=process.env.REET_DOC_IMAGES||''
// The tab bar and the Gaffer button float over the page: they are left out of a capture of the crew alone.
const clean={style:'.bottom-tabs,.gaffer-launcher{visibility:hidden}'}

try {
  const fixture=state()
  const {context,page,errors}=await openWorkspace(browser,{width:1440,routes:fixture.routes})
  await page.goto(`${base}/account/gaffer`)
  await expect(page.getByRole('heading',{name:'Gaffer AI',exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'Nowt on the list yet.'})).toBeVisible()
  await page.getByRole('button',{name:'Add provider',exact:true}).click()
  const form=page.getByRole('dialog',{name:'Add a provider'})
  await expect(form).toBeVisible()
  await form.getByLabel('API key',{exact:true}).fill('synthetic-browser-key')
  await form.getByRole('button',{name:'Fetch models'}).click()
  await expect(form.getByRole('status')).toContainText('Read 3 models')
  await form.getByLabel('Filter provider models').fill('beta')
  await expect(form.getByLabel('Model',{exact:true}).getByRole('option',{name:'model-alpha',exact:true})).toHaveCount(0)
  await form.getByLabel('Model',{exact:true}).selectOption('model-beta')
  await form.getByRole('button',{name:'Save provider'}).click()
  await expect(form).toHaveCount(0)
  await expect(page.getByText('model-beta',{exact:true})).toBeVisible()
  expect(fixture.box.saves[0].apiKey).toBe('synthetic-browser-key')
  expect(fixture.box.saves[0].allowSessions).toBe(false)
  await expect(page.locator('body')).not.toContainText('synthetic-browser-key')
  await page.getByRole('button',{name:'Test connection',exact:true}).click()
  await expect(page.getByText('Connection answered',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'Edit',exact:true}).click()
  const edit=page.getByRole('dialog',{name:'Edit provider'})
  await expect(edit.getByLabel('API key',{exact:true})).toHaveValue('')
  await expect(edit.getByLabel('API key',{exact:true})).toHaveAttribute('placeholder','Saved key kept when left blank')
  await edit.getByRole('button',{name:'Cancel',exact:true}).click()
  await page.getByRole('button',{name:'Remove',exact:true}).click()
  await page.getByRole('button',{name:'Remove provider',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Nowt on the list yet.'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()

  // A failed discovery still permits an exact model name, and Escape clears the key.
  const failed=state(); failed.box.failModels=true
  const other=await openWorkspace(browser,{width:1440,routes:failed.routes})
  await other.page.goto(`${base}/account/gaffer`)
  await other.page.getByRole('button',{name:'Add provider',exact:true}).click()
  const manual=other.page.getByRole('dialog')
  await manual.getByRole('button',{name:'Fetch models'}).click()
  await expect(manual.getByRole('status')).toContainText('does not provide a model list')
  await manual.getByLabel('Model',{exact:true}).selectOption('__manual__')
  await manual.getByLabel('Exact model name').fill('my-private-model')
  await manual.getByLabel('API key',{exact:true}).fill('synthetic-key-to-clear')
  failed.box.failSave=true
  await manual.getByRole('button',{name:'Save provider'}).click()
  await expect(manual.getByRole('alert')).toContainText('This provider changed')
  await expect(manual.getByLabel('Exact model name')).toHaveValue('my-private-model')
  await expect(manual.getByLabel('API key',{exact:true})).toHaveValue('synthetic-key-to-clear')
  await other.page.keyboard.press('Escape')
  await other.page.getByRole('button',{name:'Add provider',exact:true}).click()
  await expect(other.page.getByLabel('API key',{exact:true})).toHaveValue('')
  expect(other.errors).toEqual([])
  await other.context.close()

  // Changing the address invalidates discovery, even if the previous answer arrives later.
  const delayed=state()
  let release
  delayed.box.gate=new Promise(resolve=>{release=resolve})
  const stale=await openWorkspace(browser,{width:1440,routes:delayed.routes})
  await stale.page.goto(`${base}/account/gaffer`)
  await stale.page.getByRole('button',{name:'Add provider',exact:true}).click()
  const pending=stale.page.getByRole('dialog')
  await pending.getByRole('button',{name:'Fetch models'}).click()
  await expect.poll(()=>delayed.box.models.length).toBe(1)
  await pending.getByLabel('API address').fill('https://different.example/v1')
  release()
  await expect(pending.getByRole('button',{name:'Fetch models'})).toBeEnabled()
  await expect(pending.getByLabel('Model',{exact:true}).getByRole('option',{name:'model-alpha',exact:true})).toHaveCount(0)
  await pending.getByText('Advanced provider permissions',{exact:true}).click()
  await pending.getByLabel('Followed-session conversations and tool results').check()
  await pending.getByLabel('Code and logs from followed sessions').check()
  await pending.getByLabel('Followed-session conversations and tool results').uncheck()
  await expect(pending.getByLabel('Code and logs from followed sessions')).not.toBeChecked()
  await expect(pending.getByLabel('Code and logs from followed sessions')).toBeDisabled()
  expect(stale.errors).toEqual([])
  await stale.context.close()

  const reader={id:2,name:'Reader',email:'reader@example.test',role:'viewer'}
  const viewer=await openWorkspace(browser,{width:390,status:{user:reader},routes:{'/api/v2/auth/oidc/identity':{identity:null},'/api/v2/profile':{user:reader,profile:{theme:'light',timezone:'UTC',shelfView:'list',hostedPrefix:''},accounts:true,sessionCount:1}}})
  await viewer.page.goto(`${base}/account/gaffer`)
  await expect(viewer.page.getByText('This account is read-only, so it cannot configure Gaffer providers.')).toBeVisible()
  await expect(viewer.page.getByRole('button',{name:'Add provider',exact:true})).toHaveCount(0)
  expect(viewer.state.unexpected).toEqual([])
  await viewer.context.close()

  // The crew's model, effort and agent are chosen from what the account's machines report, not typed.
  {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width:1440,height:1400,routes:crewRoutes(box)})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true}),heavy=crew.getByRole('group',{name:'Heavy tier',exact:true})
      const field=name=>crew.getByRole('combobox',{name,exact:true})
      await expect(field('Heavy agent')).toHaveValue('claude')
      // Agents: every one is listed, and one no machine runs says so.
      await expect(field('Heavy agent').getByRole('option')).toHaveText(['The outcome’s own agent','Claude Code','Codex (not on your machines)','OpenCode','Cline (not on your machines)','Google Antigravity (AGY) (not on your machines)'])
      // Models: every machine's list merged, each model once, with the agent's default and Other…
      await expect(field('Heavy model')).toHaveValue('model-strong')
      await expect(field('Heavy model').locator('optgroup[label="On your machines"] option')).toHaveText(['model-strong','model-quick','model-deep'])
      await expect(field('Heavy model').getByRole('option').first()).toHaveText('Agent’s default model')
      await expect(field('Heavy model').getByRole('option').last()).toHaveText('Other…')
      // Efforts: every machine's levels for the agent, low to high first.
      await expect(field('Heavy effort')).toHaveValue('high')
      await expect(field('Heavy effort').getByRole('option')).toHaveText(['Default effort','Low','Medium','High','Max'])
      // OpenCode reports no effort setting: nothing to choose.
      await expect(field('Heavy backup effort')).toBeDisabled()
      await expect(field('Heavy backup effort')).toHaveValue('')
      await expect(field('Heavy backup effort').getByRole('option')).toHaveText(['No effort setting'])
      await expect(field('Heavy backup model').locator('optgroup[label="On your machines · zen"] option')).toHaveText(['zen/model-a','zen/model-b'])
      // A saved model no machine lists, and an effort none offers, are kept and said so.
      await expect(field('Heavy backup 2 model')).toHaveValue('retired-model')
      await expect(heavy).toContainText('None of your machines lists this model. If the agent does not offer it, the stage stops when it starts.')
      await expect(field('Heavy backup 2 effort')).toHaveValue('ultra')
      await expect(field('Heavy backup 2 effort').getByRole('option',{name:'Ultra (not offered by your machines)',exact:true})).toHaveCount(1)
      await expect(heavy).toContainText('If the agent does not offer this effort, the stage starts on the nearest lower level the agent offers, or on its default effort.')
      // The hint, the model's note and why an effort cannot be chosen are each tied to their select.
      await expect(field('Heavy backup 2 effort')).toHaveAccessibleDescription('If the agent does not offer this effort, the stage starts on the nearest lower level the agent offers, or on its default effort.')
      await expect(field('Heavy backup 2 model')).toHaveAccessibleDescription('None of your machines lists this model. If the agent does not offer it, the stage stops when it starts.')
      await expect(field('Heavy backup effort')).toHaveAccessibleDescription('OpenCode has no effort setting')
      // Each agent's models were read once, and only for agents in the crew.
      expect([...box.modelReads].sort()).toEqual(['claude','opencode'])
      expect(await crew.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      for (const name of ['Heavy agent','Heavy model','Heavy effort','Heavy backup effort']) expect((await field(name).boundingBox()).height,name).toBeGreaterThanOrEqual(44)
      // The model and effort selects line up with the agent select on their row.
      const tops=await Promise.all(['Heavy agent','Heavy model','Heavy effort'].map(async name=>(await field(name).boundingBox()).y))
      expect(Math.max(...tops)-Math.min(...tops)).toBeLessThanOrEqual(1)
      await crew.screenshot({path:`${crewShots}/crew-choices-1440-light.png`,...clean})
      // Changing a row's agent clears its model and effort; choosing an agent not yet read reads its models once.
      const light=crew.getByRole('group',{name:'Light tier',exact:true})
      await field('Light agent').selectOption('claude')
      await field('Light model').selectOption('model-deep');await field('Light effort').selectOption('medium')
      await field('Light agent').selectOption('opencode')
      await expect(field('Light model')).toHaveValue('');await expect(field('Light effort')).toHaveValue('');await expect(field('Light effort')).toBeDisabled()
      await field('Light agent').selectOption('codex')
      await expect.poll(()=>box.modelReads.filter(tool=>tool==='codex').length).toBe(1)
      // No machine runs Codex, so nothing is known of its efforts: the start levels are offered, with the hint.
      await expect(field('Light effort')).toBeEnabled()
      await expect(field('Light effort').getByRole('option')).toHaveText(['Default effort','Low','Medium','High'])
      await expect(field('Light effort')).toHaveAccessibleDescription('If the agent does not offer this effort, the stage starts on the nearest lower level the agent offers, or on its default effort.')
      await field('Light model').selectOption({label:'Other…'});await light.getByRole('textbox',{name:'Light model name',exact:true}).fill('model-typed')
      // No machine has listed Codex's models, so the typed name is not checked against any.
      await expect(light).toContainText('Your machines have not listed this agent’s models, so this name is not checked.')
      await expect(light).not.toContainText('None of your machines lists this model.')
      // Choose from the selects: Standard on Claude Code at low effort on a model one machine lists.
      await field('Standard agent').selectOption('claude');await field('Standard model').selectOption('model-quick');await field('Standard effort').selectOption('low')
      await field('Heavy effort').selectOption('max')
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      // What is sent is exactly what was chosen, the kept model and effort included.
      expect(box.saves).toEqual([{heavy:{tool:'claude',model:'model-strong',effort:'max',backup:{tool:'opencode',model:'',effort:''},moreBackups:[{tool:'claude',model:'retired-model',effort:'ultra'}]},standard:{tool:'claude',model:'model-quick',effort:'low'},light:{tool:'codex',model:'model-typed',effort:''},balanceBelow:20,gafferDecides:false}])
      expect([...new Set(box.modelReads)].sort()).toEqual(['claude','codex','opencode']);expect(box.modelReads.length).toBe(3)
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // Opening the saved crew and saving it with nothing touched sends exactly what was loaded.
  {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width:1440,height:1400,routes:crewRoutes(box)})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true})
      await expect(crew.getByRole('combobox',{name:'Heavy backup 2 model',exact:true})).toHaveValue('retired-model')
      await expect.poll(()=>box.modelReads.length).toBe(2)
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      const {updatedAt,...loaded}=savedCrew
      expect(box.saves).toEqual([loaded])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // A crew saved while reet knew a different Cline provider id keeps it: the source is shown as Cline reported it,
  // selected, and saving the crew untouched writes the same id back.
  {
    const legacy={heavy:{tool:'cline',model:'',effort:'',provider:'legacy-provider'},standard:{},light:{},balanceBelow:20,gafferDecides:false,updatedAt:now}
    const box={crew:structuredClone(legacy),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width:1440,height:1400,routes:crewRoutes(box)})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true})
      const source=crew.getByRole('combobox',{name:'Heavy Cline billing source',exact:true})
      await expect(source).toHaveValue('legacy-provider')
      await expect(source.locator('option')).toHaveText(['Cline’s own choice','Usage-Billing','ClinePass','legacy-provider'])
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      const {updatedAt,...loaded}=legacy
      expect(box.saves).toEqual([loaded])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // Further backups keep their own fields when one above them is removed, and Other… on a model no list has keeps it in
  // the box to be corrected. The note under such a model waits until the agent's models are read.
  {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    let release
    const w=await openWorkspace(browser,{width:1440,height:1400,routes:crewRoutes(box),gates:{'/api/v2/models':new Promise(done=>{release=done})}})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true}),heavy=crew.getByRole('group',{name:'Heavy tier',exact:true})
      const field=name=>crew.getByRole('combobox',{name,exact:true})
      await expect(field('Heavy backup 2 model')).toHaveValue('retired-model')
      // The models are still being read: nothing is said about the saved model yet.
      await expect(heavy).not.toContainText('None of your machines lists this model.')
      await expect(field('Heavy backup 2 model')).not.toHaveAttribute('aria-describedby',/./)
      release()
      await expect(heavy).toContainText('None of your machines lists this model. If the agent does not offer it, the stage stops when it starts.')
      await heavy.getByRole('button',{name:'Add another backup for Heavy',exact:true}).click()
      await field('Heavy backup 3 model').selectOption('model-quick')
      // Other… on the kept model fills the box with it, at the server's limit of 100 bytes.
      await field('Heavy backup 2 model').selectOption({label:'Other…'})
      const typed=heavy.getByRole('textbox',{name:'Heavy backup 2 model name',exact:true})
      await expect(typed).toHaveValue('retired-model');await expect(typed).toHaveAttribute('maxlength','100')
      await typed.fill('retired-model-2')
      // Removing that backup leaves the one below it as it was: its own model, not Other…
      await field('Heavy backup 2 agent').selectOption('')
      await expect(field('Heavy backup 2 model')).toHaveValue('model-quick')
      await expect(heavy.getByRole('textbox',{name:/model name$/})).toHaveCount(0)
      await expect(field('Heavy backup 3 agent')).toHaveCount(0)
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      expect(box.saves[0].heavy.moreBackups).toEqual([{tool:'claude',model:'model-quick',effort:''}])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // An account with no machines can still set its crew: every agent's efforts are the start levels, with the hint.
  {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width:1440,height:1400,routes:crewRoutes(box,{machines:[]})})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true})
      const field=name=>crew.getByRole('combobox',{name,exact:true})
      await expect(field('Heavy agent').getByRole('option',{name:'Claude Code (not on your machines)',exact:true})).toHaveCount(1)
      for (const name of ['Heavy effort','Heavy backup effort']) {
        await expect(field(name)).toBeEnabled()
        await expect(field(name).getByRole('option')).toHaveText(['Default effort','Low','Medium','High'])
      }
      await expect(field('Heavy effort')).toHaveValue('high')
      await field('Heavy backup effort').selectOption('medium')
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      expect(box.saves[0].heavy.backup).toEqual({tool:'opencode',model:'',effort:'medium'})
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // The machines and their models cannot be read: the form says so in one line and the crew can still be saved.
  {
    const box={crew:{heavy:{tool:'claude',model:'',effort:'high'},standard:{},light:{},balanceBelow:20,gafferDecides:false,updatedAt:now},saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width:1440,routes:crewRoutes(box,{failMachines:true,failModels:true})})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true})
      const field=name=>crew.getByRole('combobox',{name,exact:true})
      await expect(crew.getByRole('status').filter({hasText:'could not be read'})).toHaveText('Your machines and their models could not be read: efforts are not checked against them, and Other… takes a model’s name. You can still save your crew.')
      // Nothing is claimed about agents when the machines are not known.
      await expect(field('Heavy agent').getByRole('option',{name:/not on your machines/})).toHaveCount(0)
      await expect(field('Heavy effort').getByRole('option')).toHaveText(['Default effort','Low','Medium','High'])
      await expect(field('Heavy effort')).toHaveValue('high')
      await expect(field('Heavy model').getByRole('option').first()).toHaveText('Agent’s default model')
      await field('Heavy model').selectOption({label:'Other…'});await crew.getByRole('textbox',{name:'Heavy model name',exact:true}).fill('model-typed')
      await expect(crew).toContainText('Your machines’ models could not be read. If the agent does not offer this model, the stage stops when it starts.')
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status').filter({hasText:'Saved.'})).toHaveText('Saved. New outcomes start from this crew.')
      expect(box.saves).toEqual([{heavy:{tool:'claude',model:'model-typed',effort:'high'},standard:{},light:{},balanceBelow:20,gafferDecides:false}])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // The crew in dark and on a phone, where each row stacks.
  for (const [width,theme] of [[1440,'dark'],[390,'dark'],[390,'light']]) {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width,height:width<500?844:1400,theme,touch:width<650,routes:crewRoutes(box)})
    try {
      const page=w.page
      await page.goto(`${base}/account/gaffer`)
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true})
      const field=name=>crew.getByRole('combobox',{name,exact:true})
      await expect(field('Heavy model')).toHaveValue('model-strong')
      await expect(field('Heavy backup effort')).toHaveText('No effort setting')
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${width} ${theme} page width`).toBe(true)
      expect(await crew.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      if (width<500) {
        // Rows stack: the agent, model and effort of a row are one above the other, each the row's full width.
        const [agent,model,effort]=await Promise.all(['Heavy agent','Heavy model','Heavy effort'].map(name=>field(name).boundingBox()))
        expect(model.y).toBeGreaterThan(agent.y+agent.height-1);expect(effort.y).toBeGreaterThan(model.y+model.height-1)
        expect(Math.abs(model.width-agent.width)).toBeLessThanOrEqual(1)
        for (const box of [agent,model,effort]) expect(box.height).toBeGreaterThanOrEqual(44)
      }
      await crew.screenshot({path:`${crewShots}/crew-choices-${width}-${theme}.png`,...clean})
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // The captures for the docs, at twice the pixels.
  if (docImages) for (const [width,theme] of [[1440,'light'],[390,'dark']]) {
    const box={crew:structuredClone(savedCrew),saves:[],modelReads:[]}
    const w=await openWorkspace(browser,{width,height:width<500?844:1400,theme,touch:width<650,scale:2,routes:crewRoutes(box)})
    try {
      await w.page.goto(`${base}/account/gaffer`)
      const crew=w.page.getByRole('region',{name:'Your Factory crew',exact:true})
      await expect(crew.getByRole('combobox',{name:'Heavy model',exact:true})).toHaveValue('model-strong')
      await crew.screenshot({path:`${docImages}/crew-choices-${width}-${theme}.png`,...clean})
    } finally { await w.context.close() }
  }

  for (const width of [1440,390,320]) for (const theme of ['light','dark']) {
    const fixture=state(true)
    const view=await openWorkspace(browser,{width,theme,touch:width<650,routes:fixture.routes})
    await view.page.goto(`${base}/account/gaffer`)
    await expect(view.page.getByRole('heading',{name:'My OpenCode',exact:true})).toBeVisible()
    await expect(view.page.getByRole('button',{name:'Add provider',exact:true})).toBeEnabled()
    expect(await view.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${width} ${theme} page width`).toBe(true)
    await view.page.screenshot({path:`${captures}/gaffer-settings-${width}-${theme}.png`,fullPage:true})
    await view.page.getByRole('button',{name:'Add provider',exact:true}).click()
    const dialog=view.page.getByRole('dialog',{name:'Add a provider'})
    await expect(dialog).toBeVisible()
    await dialog.getByLabel('Provider',{exact:true}).selectOption('ollama-local')
    await expect(dialog.getByLabel('Local network only')).toBeChecked()
    await dialog.getByLabel('Model',{exact:true}).selectOption('__manual__')
    await dialog.getByLabel('Exact model name').fill('qwen3:14b')
    const box=await dialog.boundingBox()
    expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x+box.width).toBeLessThanOrEqual(width)
    if (width<650) {
      const field=await dialog.getByLabel('Name',{exact:true}).boundingBox()
      expect(field.height).toBeGreaterThanOrEqual(44)
    }
    await dialog.evaluate(el=>{el.scrollTop=0})
    await view.page.screenshot({path:`${captures}/gaffer-provider-form-${width}-${theme}.png`})
    await view.page.keyboard.press('Escape')
    expect(view.errors).toEqual([])
    await view.context.close()
  }
} finally { await browser.close() }
console.log('Gaffer settings: the crew chosen from the machines’ reports, model selection, stale discovery, CRUD, key privacy, failure recovery, permissions, and desktop/phone themes passed.')
