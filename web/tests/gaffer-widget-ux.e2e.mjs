import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-widget'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),cid='b'.repeat(32),sid='widget-layout-session'
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
const session={id:sid,agentId:'a1',title:'Improve the release workflow',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':view,
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true}}]},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:{queue:[]},
}
try{
  for(const theme of ['light','dark'])for(const [width,height] of [[1440,1000],[800,1000],[767,1000],[390,1000],[320,1000],[844,390]]){
    const w=await openWorkspace(browser,{width,height,theme,touch:width<1000,routes})
    await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
    try{
      await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
      const launch=w.page.getByRole('button',{name:'Ask Gaffer',exact:true}),chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true})
      await expect(launch).toHaveCount(1)
      const clear=await launch.evaluate(el=>{const a=el.getBoundingClientRect(),workspace=document.querySelector('[aria-label="Remote terminal workspace"]')?.getBoundingClientRect();return {width:a.width,height:a.height,inView:a.bottom<=innerHeight&&a.top>=0,clear:!workspace||a.top>=workspace.bottom-1||a.left>=workspace.right+8||a.right<=workspace.left-8}})
      expect(clear.width).toBe(56);expect(clear.height).toBe(56);expect(clear.inView).toBe(true);expect(clear.clear).toBe(true)
      await launch.click();await expect(chat).toBeVisible()
      await expect(chat.getByRole('button',{name:'Conversation history',exact:true})).toBeVisible()
      await expect(chat.getByRole('button',{name:'Look after this session',exact:true})).toBeVisible()
      await chat.getByRole('textbox',{name:'Message Gaffer',exact:true}).fill('Preserve this draft while I resize')
      if(width>=768&&height>500){
        for(const [name,remWidth,remHeight]of [['Compact',24,32],['Default',28,42],['Expanded',46,52]]){
          await chat.getByRole('button',{name,exact:true}).click()
          const rect=await chat.boundingBox();expect(Math.abs(rect.width-Math.min(remWidth*16,width-32))).toBeLessThan(2);expect(Math.abs(rect.height-Math.min(remHeight*16,936))).toBeLessThan(2)
        }
        await chat.evaluate(el=>{el.style.width='370px';el.style.height='440px'})
        await chat.getByRole('button',{name:'Expanded',exact:true}).click()
        expect(await chat.evaluate(el=>el.style.width)).toBe('')
        await chat.getByRole('button',{name:'Place Gaffer on the left',exact:true}).click();expect((await chat.boundingBox()).x).toBe(16)
        const handle=chat.getByRole('button',{name:'Move Gaffer panel with arrow keys or by dragging',exact:true})
        await handle.focus();await w.page.keyboard.press('ArrowRight');expect((await chat.boundingBox()).x).toBe(26)
        await chat.getByRole('button',{name:'Full screen',exact:true}).click()
        expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(true)
        expect(Math.abs((await chat.boundingBox()).width-(width-32))).toBeLessThan(2)
        await chat.getByRole('button',{name:'Restore panel',exact:true}).click()
        expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(false)
        expect((await chat.boundingBox()).x).toBe(16)
        await chat.getByRole('button',{name:'Default',exact:true}).click()
      }else{expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(true);await expect(chat.getByRole('group',{name:'Gaffer panel size',exact:true})).toBeHidden()}
      const size=await chat.boundingBox();expect(size.x).toBeGreaterThanOrEqual(0);expect(size.y).toBeGreaterThanOrEqual(0);expect(size.x+size.width).toBeLessThanOrEqual(width+1)
      expect(await chat.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      if(width===1440||width===390)await chat.screenshot({path:`${shots}/gaffer-widget-${width}-${theme}.png`})
      await w.page.keyboard.press('Escape');await expect(launch).toBeFocused()
      const parked=await launch.boundingBox();await w.page.mouse.wheel(0,500);await expect.poll(async()=>(await launch.boundingBox()).y).toBeCloseTo(parked.y,0)
      await launch.click();await expect(chat.getByRole('textbox',{name:'Message Gaffer',exact:true})).toHaveValue('Preserve this draft while I resize')
      if(width>=768&&height>500){await expect(chat.getByRole('button',{name:'Default',exact:true})).toHaveAttribute('aria-pressed','true');expect(await chat.evaluate(el=>el.style.width)).toBe('')}
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  console.log('Widget UX: twelve width/theme views, including a landscape phone, stable clear 56 px launcher, mascot, three presets, native resize reset, left/right placement, keyboard move, modal full screen/phone, drafts and focus passed.')
}finally{await browser.close()}
