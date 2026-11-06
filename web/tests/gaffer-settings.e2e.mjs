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
console.log('Gaffer settings: model selection, stale discovery, CRUD, key privacy, failure recovery, permissions, and desktop/phone themes passed.')
