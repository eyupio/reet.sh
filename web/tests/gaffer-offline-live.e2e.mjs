// No API routing/interception: all requests reach the real controller test binary.
import {chromium,expect,request} from '@playwright/test'
import {mkdir} from 'node:fs/promises'

const base=process.env.REET_UI_URL
if(!base)throw new Error('REET_UI_URL must identify the isolated GM6 test controller')
const mode=process.env.REET_GAFFER_TEST_MODE||'native'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-offline'
await mkdir(shots,{recursive:true})
const driver=await request.newContext({baseURL:base,extraHTTPHeaders:{Origin:base}})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
let context,page,passed=false
const errors=[]
async function api(path,method='GET',data){
  const response=await driver.fetch(path,{method,data})
  if(!response.ok())throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`)
  return response.json()
}
const command=(action,extra={})=>api('/_gaffer_test/control','POST',{action,...extra})
const state=()=>api('/_gaffer_test/state')
async function openBrowser(){
  context=await browser.newContext({viewport:{width:mode==='docker'?390:1440,height:1000},storageState:await driver.storageState(),hasTouch:mode==='docker',isMobile:mode==='docker',colorScheme:mode==='docker'?'dark':'light'})
  page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
}
const chat=()=>page.getByRole('dialog',{name:'Gaffer',exact:true})
const goal=()=>page.getByRole('dialog',{name:'Look after this session',exact:true})
const conversations={}
let workers
async function session(marker){
  const s=workers[marker].session
  await page.goto(`${base}/sessions?agent=${s.agentId}&session=${s.id}`)
  await page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
  await expect(chat().locator('.scope-title')).toHaveText(s.title)
  await expect(chat().getByRole('textbox',{name:'Message Gaffer',exact:true})).toBeVisible()
}
async function option(name){
  await chat().getByRole('button',{name:'Chat options',exact:true}).click()
  await page.getByRole('dialog',{name:'Chat options',exact:true}).getByRole('button',{name,exact:true}).click()
}
async function identify(marker){
  const view=await api(`/api/v2/gaffer/conversations?sessionId=${workers[marker].session.id}`)
  conversations[marker]=view.conversation.id
}
async function start(marker,budget){
  await option('Look after this session')
  await goal().getByText('Change limits',{exact:true}).click()
  await expect(goal().getByLabel('Review budget',{exact:true})).toHaveValue('12')
  await expect(goal().getByLabel('Follow for (hours)',{exact:true})).toHaveValue('24')
  await goal().getByLabel('Review budget',{exact:true}).fill(String(budget))
  await goal().getByLabel('Agreed goal',{exact:true}).fill(`${marker} ensure a PR is raised and its checks and conflicts are addressed. Bring decisions back to me. Do not merge.`)
  await identify(marker)
  await goal().getByRole('button',{name:'Start follow-up',exact:true}).click()
  await expect(goal().locator('.saved-goal')).toContainText(marker)
}
const goalData=async marker=>(await api(`/api/v2/gaffer/conversations/${conversations[marker]}/goal`)).goal
const receipt=async marker=>(await api(`/api/v2/gaffer/conversations/${conversations[marker]}`)).messages?.at(0)
async function waitGoal(marker,stateName){await expect.poll(async()=>(await goalData(marker))?.state,{timeout:90000}).toBe(stateName)}
async function resume(){
  await goal().getByRole('button',{name:'Resume after review',exact:true}).click()
  await page.getByRole('dialog',{name:'Resume after checking the working conversation?',exact:true}).getByRole('button',{name:'Resume follow-up',exact:true}).click()
  await expect(goal().getByRole('heading',{name:'Looking after this session',exact:true})).toBeVisible()
}
try{
  await api('/api/v2/auth/sign-in','POST',{email:'gm6@example.test',password:'synthetic offline flow password'})
  const fixture=await state();workers=fixture.workers
  const provider={name:'Synthetic Gaffer',kind:'openai_compatible',baseUrl:fixture.providerURL,model:'gaffer-synthetic',enabled:true,makeDefault:true,localOnly:true,allowSessions:false,allowCode:false,apiKey:'synthetic-gm6-key'}
  const listed=await api('/api/v2/gaffer/providers/models','POST',provider)
  expect(listed.models).toContain('gaffer-synthetic')
  await api('/api/v2/gaffer/providers','POST',provider)
  await openBrowser();await session('ALPHA')
  await option('Look after this session')
  await expect(goal().getByLabel('Agreed goal',{exact:true})).toBeVisible()
  await expect(goal().getByText(/This also enables selected-session data/)).toBeVisible()
  if(mode==='native')await goal().screenshot({path:`${shots}/gaffer-session-setup-${mode}.png`})
  await page.keyboard.press('Escape')
  expect((await state()).workers.ALPHA.calls).toBe(0)
  await command('mode',{marker:'ALPHA',mode:'queue'})
  await start('ALPHA',2)
  const savedProvider=(await api('/api/v2/gaffer/providers')).providers[0]
  expect(savedProvider.allowSessions).toBe(true);expect(savedProvider.allowCode).toBe(true);expect(savedProvider.allowStatus).toBe(false)
  await expect.poll(async()=>(await receipt('ALPHA'))?.state,{timeout:20000}).toBe('queued')
  await expect.poll(async()=>(await goalData('ALPHA')).activeTurnId,{timeout:10000}).toBeFalsy()
  expect((await state()).workers.ALPHA.prompts).toHaveLength(0)
  await context.close();context=null
  await command('restart')
  expect((await state()).workers.ALPHA.calls).toBe(2)
  // No browser, queue-read or user send wakes delivery after restart.
  await command('progress',{marker:'ALPHA'})
  await expect.poll(async()=>(await state()).workers.ALPHA.prompts.length,{timeout:10000}).toBe(1)
  expect((await state()).workers.ALPHA.calls).toBe(2)
  console.log(`${mode}: browser-free queued delivery survived a closed/reopened database and controller restart, exactly once`)

  await openBrowser();await session('BRAVO')
  await command('mode',{marker:'BRAVO',waiting:true})
  await start('BRAVO',1);await waitGoal('BRAVO','needs_user')
  expect((await state()).workers.BRAVO.calls).toBe(0)
  expect((await state()).workers.BRAVO.prompts).toHaveLength(0)
  const permissionGoal=await goalData('BRAVO');expect(permissionGoal.usedReviews).toBe(0)
  await page.keyboard.press('Escape')

  await session('ALPHA');await option('Look after this session')
  await goal().getByRole('button',{name:'Pause follow-up',exact:true}).click()
  await expect(goal().getByRole('heading',{name:'Paused',exact:true})).toBeVisible()
  await command('progress',{marker:'ALPHA'})
  await expect(goal().getByText('1 / 2 reviews used',{exact:false})).toBeVisible()
  expect((await state()).workers.ALPHA.calls).toBe(2)
  await resume()
  expect((await goalData('ALPHA')).usedReviews).toBe(1)
  await command('progress',{marker:'ALPHA'})
  await expect.poll(async()=>(await state()).workers.ALPHA.prompts.length,{timeout:30000}).toBe(2)
  await waitGoal('ALPHA','limit')
  const limited=await goalData('ALPHA');expect(limited.usedReviews).toBe(2);expect(limited.maxReviews).toBe(2)
  await command('progress',{marker:'ALPHA'})
  expect((await state()).workers.ALPHA.calls).toBe(4)
  await page.keyboard.press('Escape')

  await session('BRAVO');await option('Look after this session');await resume()
  await command('progress',{marker:'BRAVO'})
  await expect.poll(async()=>(await state()).workers.BRAVO.prompts.length,{timeout:30000}).toBe(1)
  await waitGoal('BRAVO','limit')
  await goal().getByRole('button',{name:'Mark goal complete',exact:true}).click()
  await page.getByRole('dialog',{name:'Mark this goal complete?',exact:true}).getByRole('button',{name:'Mark complete',exact:true}).click()
  await expect(goal().getByRole('heading',{name:'You marked this complete',exact:true})).toBeVisible()
  await page.keyboard.press('Escape')
  console.log(`${mode}: independent goals, worker approval boundary, explicit resume, preserved budgets, hard limits and user completion passed`)

  // Advanced revocation remains available after the single reviewed setup.
  await session('ALPHA');await option('Look after this session')
  await goal().getByRole('button',{name:'Edit goal or start a new budget',exact:true}).click()
  await goal().getByLabel('Agreed goal',{exact:true}).fill('ALPHA review the new simulated check; ask the worker to fix it. Do not merge.')
  await command('mode',{marker:'ALPHA',mode:'queue'})
  await goal().getByRole('button',{name:'Start new goal / budget',exact:true}).click()
  await page.getByRole('dialog',{name:'Start a new goal and budget?',exact:true}).getByRole('button',{name:'Start new goal',exact:true}).click()
  await expect.poll(async()=>(await receipt('ALPHA'))?.state,{timeout:30000}).toBe('queued')
  await expect.poll(async()=>(await goalData('ALPHA')).activeTurnId,{timeout:10000}).toBeFalsy()
  const workerPermission=await api(`/api/v2/gaffer/conversations/${conversations.ALPHA}/worker`)
  await api(`/api/v2/gaffer/conversations/${conversations.ALPHA}/worker`,'PUT',{enabled:false,revision:workerPermission.worker.grant.revision,providerId:workerPermission.worker.providerId,providerRevision:workerPermission.worker.providerRevision,model:workerPermission.worker.model})
  await command('progress',{marker:'ALPHA'})
  await expect.poll(async()=>(await receipt('ALPHA'))?.state,{timeout:10000}).toBe('removed')
  await waitGoal('ALPHA','needs_user')
  expect((await state()).workers.ALPHA.prompts).toHaveLength(2)
  expect((await state()).workers.ALPHA.calls).toBe(6)
  await page.keyboard.press('Escape')
  console.log(`${mode}: revoking this session's worker grant removed a queued goal instruction before delivery`)

  await session('CHARLIE');await command('mode',{marker:'CHARLIE',mode:'uncertain'})
  await start('CHARLIE',2)
  await expect.poll(async()=>(await receipt('CHARLIE'))?.state,{timeout:20000}).toBe('uncertain')
  await waitGoal('CHARLIE','needs_user')
  await context.close();context=null
  const before=(await state()).workers.CHARLIE
  await command('restart')
  expect((await receipt('CHARLIE')).state).toBe('uncertain')
  const after=(await state()).workers.CHARLIE
  expect(after.prompts).toHaveLength(1);expect(after.calls).toBe(before.calls)
  await openBrowser();await session('CHARLIE');await option('Factory floor')
  const overview=page.getByRole('dialog',{name:'Factory floor',exact:true})
  await expect(overview.getByRole('heading',{name:'CHARLIE release',exact:true})).toBeVisible()
  await expect(overview.getByText('Delivery uncertain',{exact:true}).first()).toBeVisible()
  await overview.screenshot({path:`${shots}/gaffer-offline-${mode}.png`})
  const final=await state()
  expect(final.forbidden).toBe(0)
  expect(final.workers.ALPHA.prompts.every(text=>text.includes('ALPHA')&&!text.includes('BRAVO'))).toBe(true)
  expect(final.workers.BRAVO.prompts.every(text=>text.includes('BRAVO')&&!text.includes('ALPHA'))).toBe(true)
  expect(final.workers.CHARLIE.prompts).toHaveLength(1)
  expect(errors).toEqual([])
  passed=true
  console.log(`${mode}: ambiguous worker handoff retained through restart and never retried; real followed-session overview and scoped model context passed`)
}finally{
  await context?.close();await browser.close()
  try{await command('finish',{ok:passed})}catch(error){console.error('Test fixture stopped before the final acknowledgement:',error.message.split('\n')[0])}finally{await driver.dispose()}
}
