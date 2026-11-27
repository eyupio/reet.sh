import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-skill-buddy'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const unslop={id:'builtin:unslop',name:'Unslop',description:'Polish existing UX and UI while keeping the product character.',source:'builtin',revision:'reet-adaptation-1',reason:'Matches unslop, ux and ui in this task.',stage:'design',stages:['design','code','review'],instructions:'Inspect the UI. Preserve its character. Improve spacing, colour, icons, copy, type and layout. Check mobile, dark mode and keyboard access. Adapted from Unslop by Damir Sokolovsky, CC BY 4.0.'}
const review={...unslop,id:'builtin:review',name:'Review the real behaviour',stage:'review',revision:'1',instructions:'Trace changed behaviour and report concrete findings.'}
const records=Array.from({length:12},(_,i)=>({scope:i<6?`factory:outcome:a:${i}`:`session:s${i}`,sessionId:`s${i}`,stage:['design','code','review','test'][i%4],skill:{...(i%3?unslop:review),state:i%4===0?'suggested':i%3===0?'prepared':i%2===0?'selected':'drafted'},at:now-i*3600000}))
const status={version:'dev',commit:'fixture',db:{path:'/data/reet.db',size:1024},counts:{all:0},environments:[],events:[],skillsSh:true,topics:{},settings:{refreshEnabled:true,topicsEnabled:false,linkListsEnabled:false,rollupIndividual:false}}
const machine={agentId:'a1',host:'workshop',ready:true,capabilities:{terminalEnabled:true,launch:true,acp:true,tools:['opencode']}}
const stages=['code','design','review','test'].map((kind,i)=>({kind,tool:'opencode',state:i===0?'running':'pending',requestId:`request-${i}`}))
const outcome={id:'e'.repeat(32),revision:1,objective:'Polish the project settings UX',state:'settled',projectId:'p1',projectName:'reet.sh',agentId:'a1',dir:'/work/reet',tool:'opencode',maxReviews:12,hours:24,createdAt:now,updatedAt:now,sessions:[{key:'a',title:'Settings interface',prompt:'Unslop the settings UX and UI',state:'following',note:'The code stage is in progress.',sessionId:'s1',stage:0,stages,updatedAt:now}]}

try{
 for(const [width,theme]of[[1440,'light'],[390,'dark']]){
  const choices=[];let system={skills:[unslop,review,...Array.from({length:23},(_,i)=>({...review,id:`builtin:guide-${i}`,name:`Stage guidance ${i+1}`}))],defaults:[unslop,review],version:1}
  const w=await openWorkspace(browser,{width,height:1000,theme,touch:width<500,terminalStatus:active,terminalMachines:[machine],routes:{
   '/api/v2/skill-buddy/insights':{records,limit:500,retentionDays:90,sessionUsage:{s1:{reported:true,inputTokens:1000,outputTokens:200,cachedTokens:500}}},
   '/api/admin/status':status,
   '/api/v2/admin/system-skills':r=>{if(r.method()==='PUT'){const body=r.postDataJSON();choices.push(body);system={...system,version:system.version+1,skills:system.skills.map(s=>s.id===body.skill.id?body.skill:s)}}return system},
   '/api/v2/gaffer/outcomes':{outcomes:[outcome]},
   '/api/v2/gaffer/providers':{providers:[],presets:[],available:true,canUseLocal:true,maxProviders:12},
   // Unslop starts removed from the stage: it must still be listed and selectable again.
   '/api/v2/skill-buddy/recommendations':r=>{const body=r.postDataJSON();return{skills:[{...unslop,instructions:undefined,state:choices.some(c=>c.action==='selected')?'selected':'dismissed'}],stage:stages[body.stage??0].kind,editable:(body.stage??0)>0,scope:`factory:outcome:a:${body.stage??0}`}},
   '/api/v2/skill-buddy/choose':r=>{const body=r.postDataJSON();choices.push(body);return{text:unslop.instructions,skill:unslop}},
  }})
  await w.page.goto(base+'/skill-insights')
  await expect(w.page.getByRole('heading',{name:'Skill insights',exact:true})).toBeVisible()
  await expect(w.page.getByRole('heading',{name:'Skills you reach for'})).toBeVisible()
  await expect(w.page.locator('.receipt')).toHaveCount(12)
  await expect(w.page.locator('.usage-number')).toHaveText('1,200 reported input/output tokens across 1 linked session')
  await expect(w.page.locator('.evidence')).toContainText('500 cached tokens reported separately.')
  await w.page.getByRole('button',{name:'Factory stages',exact:true}).click()
  await expect(w.page.locator('.receipt')).toHaveCount(6)
  expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await w.page.screenshot({path:`${shots}/insights-${width}-${theme}.png`,fullPage:true})

  await w.page.goto(base+'/admin/system-skills')
  await expect(w.page.getByRole('heading',{name:'System skills',exact:true})).toBeVisible()
  await w.page.getByRole('button',{name:/Unslop.*Polish/}).click()
  await expect(w.page.getByLabel('Instructions',{exact:true})).toHaveValue(unslop.instructions)
  if(width<500)await expect(w.page.getByRole('heading',{name:'Edit system skill',exact:true})).toBeInViewport()
  await w.page.getByLabel('Name',{exact:true}).fill('Unslop for our interfaces')
  await w.page.getByRole('button',{name:'Save skill',exact:true}).click()
  await expect(w.page.getByText(/System skill saved\./)).toBeVisible()
  if(width<500)await expect(w.page.getByText(/System skill saved\./)).toBeInViewport()
  expect(choices.at(-1).skill.id).toBe('builtin:unslop')
  expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await w.page.screenshot({path:`${shots}/admin-${width}-${theme}.png`,fullPage:true})

  // The shared pipeline link opens its detail on desktop and phone.
  await w.page.goto(`${base}/factory?pipeline=${outcome.id}.a`)
  const design=w.page.locator('.detail .stages button').nth(1)
  await expect(design).toBeVisible();await design.click()
  const buddy=w.page.locator('[data-skill-buddy]')
  await expect(buddy.getByRole('heading',{name:'Skills for design'})).toBeVisible()
  await buddy.getByRole('button',{name:'Review skill',exact:true}).click()
  await expect(buddy.locator('pre')).toContainText('CC BY 4.0')
  await buddy.getByRole('button',{name:'Use for this stage'}).click()
  await expect(buddy.getByRole('status')).toContainText('Selected')
  expect(choices.at(-1)).toMatchObject({outcomeId:outcome.id,pipeline:'a',stage:1,skillId:'builtin:unslop',action:'selected',revision:'reet-adaptation-1'})
  expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await w.page.screenshot({path:`${shots}/factory-${width}-${theme}.png`,fullPage:true})
  expect(w.errors).toEqual([])
  await w.context.close()
 }
 console.log('Skill insights, system skill administration and per-stage Unslop selection passed at desktop and phone widths.')
}finally{await browser.close()}
