// Chrome geometry against the approved Home drawings. Synthetic API answers only.
// Run separately: this adds no fixture to terminal-browser's CI budget.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
import { mkdirSync, writeFileSync } from 'node:fs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const out = process.env.REET_FIDELITY_OUTPUT
if (out) mkdirSync(out, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const readings = []
try {
 for (const theme of ['dark', 'light']) for (const [width, height] of [[1440,900],[1239,900],[1100,900],[834,1112],[651,1112],[390,844],[320,844]]) {
  const now=Date.now()
  const caps = {protocolVersion:1, os:'linux', osVersion:'6.1', terminalEnabled:true, terminalControl:true, launch:true, tools:['claude','codex'], readiness:'ready'}
  const machines=['studio','server','desk'].map((host,i)=>({agentId:`a${i+1}`,host,ready:true,serverEnabled:true,capabilities:{...caps,os:i===2?'darwin':'linux',osVersion:i===2?'15':'6.1'}}))
  const projects=['reet','site','notes','runner','docs'].map((name,i)=>({id:`p${i}`,host:`a${i%3+1}`,machine:machines[i%3].host,dir:`/home/dev/${name}`,name,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position:i,createdAt:1,updatedAt:1,version:1}))
  const titles=['Refactor the store layer','Add folder browse','Speed up the runner pool','Review code','Docs pass']
  const tools=['cursor','cline','claude','codex','']
  const sessions=titles.map((title,i)=>({id:`s${i}`,agentId:`a${i%3+1}`,projectId:`p${i}`,title,state:i===4?'exited':'running',tool:tools[i],createdAt:now-600000,updatedAt:now-60000,exitCode:i===4?0:null}))
  const {context,page,errors} = await openWorkspace(browser, {width,height,theme,touch:width<1100,
   terminalStatus:{configured:true,available:true,reason:''},terminalMachines:machines,
   routes:{
    '/api/v2/folder-projects':{projects,savedWaiting:0},
    '/api/v2/terminal/sessions':{sessions},
    '/api/v2/machines/health':{serverNow:now,machines:machines.map((m,i)=>({...m,state:'connected',connection:{connected:true,reportFresh:true,report:{cpuPercent:{value:[38,71,6][i]},scope:'host'}}}))},
   }})
  try {
   await page.goto(base+'/')
   // Home's primary view is the session board: its Active stage holds the running sessions.
   await expect(page.getByRole('heading',{name:'Active',exact:true})).toBeVisible()
   await expect(page.locator('.stage.active .session-card').first()).toBeVisible()
   // The fleet strip lives in the collapsed Your machines section; open it to read the machine drawings.
   const machinesSection = page.locator('details.home-detail').filter({hasText:'Your machines'})
   await machinesSection.locator('summary').click()
   await page.evaluate(()=>document.fonts.ready)
   if(width===1440) await expect(page.locator('.fleet .os').first()).toHaveText('Linux 6.1')
   const values = await page.evaluate(()=>{
    const box = selector => {const e=document.querySelector(selector),r=e.getBoundingClientRect(),s=getComputedStyle(e);return {x:r.x,y:r.y,width:r.width,height:r.height,gap:s.gap,fontSize:s.fontSize,fontWeight:s.fontWeight,lineHeight:s.lineHeight,background:s.backgroundColor,border:s.borderWidth,padding:s.padding}}
    return {shell:box('.shell-bar'),logo:box('.shell-bar .logo'),avatar:box('.account .avatar'),jump:box('.jump'),home:box('div.home'),bell:box('.bell-button'),bottom:box('.bottom-tabs'),scrollWidth:document.documentElement.scrollWidth,navOverflow:(n=>n?n.scrollWidth-n.clientWidth:0)(document.querySelector('.shell-bar > .top-tabs')),viewport:innerWidth,footer:!!document.querySelector('body > footer, main > footer'),headerTargets:[...document.querySelectorAll('.shell-bar > .home, .shell-bar > .top-tabs, .shell-bar > .jump, .shell-bar > .search-button, .shell-bar > .bell, .shell-bar > .account')].filter(e=>getComputedStyle(e).display!=='none').map(e=>({name:e.className,left:e.getBoundingClientRect().left,right:e.getBoundingClientRect().right}))}
   })
   readings.push({theme,width,height,...values})
   expect(values.scrollWidth,`${theme} ${width}: horizontal scroll`).toBeLessThanOrEqual(width)
   expect(values.navOverflow,`${width}: the tabs fit inside their own box, so none runs under Jump or the bell`).toBeLessThanOrEqual(0)
   for(let i=0;i<values.headerTargets.length;i++) {
    const target=values.headerTargets[i]
    expect(target.left,`${width}: ${target.name} starts in viewport`).toBeGreaterThanOrEqual(0)
    expect(target.right,`${width}: ${target.name} ends in viewport`).toBeLessThanOrEqual(width)
    if(i) expect(target.left,`${width}: header controls never overlap`).toBeGreaterThanOrEqual(values.headerTargets[i-1].right)
   }
   expect(values.footer, 'Home leaves the dock at the bottom without a site footer').toBe(false)
   expect(values.logo.width).toBeCloseTo(120,0)
   expect(values.avatar.width).toBe(36)
   expect(values.avatar.height).toBe(36)
   if(width===1440) {
    expect(values.shell.gap).toBe('28px')
    expect(values.jump.width).toBe(326)
    expect(values.shell.lineHeight).toBe('18.2px')
   }
   if(width===834) expect(values.shell.gap).toBe('20px')
   if(width<651) {
    expect(values.shell.height).toBe(56)
    expect(values.bell.border).toBe('0px')
    expect(values.bottom.height).toBe(64)
    for(const element of await page.locator('.shell-bar button:visible, .shell-bar summary:visible, .bottom-tabs a, .bottom-tabs button').all()) {
     const r=await element.boundingBox()
     expect(r.width,'phone target width').toBeGreaterThanOrEqual(44)
     expect(r.height,'phone target height').toBeGreaterThanOrEqual(44)
    }
   }
   expect(errors).toEqual([])
   if(out) await page.screenshot({path:`${out}/home-${theme}-${width}.png`,fullPage:true})
  } finally { await context.close() }
 }
} finally {
 if(out) writeFileSync(`${out}/metrics.json`,JSON.stringify(readings,null,2)+'\n')
 await browser.close()
}
console.log('PASS shell fidelity: 1440 / 1239 / 1100 / 834 / 651 / 390 / 320, dark and light')
