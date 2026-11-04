import { chromium, expect } from '@playwright/test'
import { readFile, mkdir } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { active, openWorkspace } from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const A='connector-preview00000001', S='session-preview000000001', now=Date.now()
const bytes=await readFile(new URL('../../docs/images/ui/session-actions-desktop-light.png',import.meta.url))
const id=createHash('sha256').update(bytes).digest('hex')
const picture={id,name:'Session screenshot.png',mime:'image/png',size:bytes.length}
const machine={agentId:A,host:'Preview machine',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalControl:true,terminalEnabled:true,healthEnabled:true,readiness:'ready',acp:true,acpPreviews:true,tools:['opencode']}}
const row={id:S,agentId:A,title:'Review the screenshot',state:'running',createdAt:now,updatedAt:now,tool:'opencode',structured:true,exitCode:null,dir:'/tmp'}
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
try {
 for(const theme of ['light','dark']) for(const width of [1360,390]) {
  const {page,context,errors}=await openWorkspace(browser,{width,height:900,touch:width<600,theme,terminalStatus:active,terminalMachines:[machine],routes:{'/api/v2/terminal/sessions':{sessions:[row]}}})
  let refuse=true,fetches=0
  await page.route(`**/chat/previews/${id}`,route=>{fetches++;return refuse?route.fulfill({status:503,json:{error:'Reconnect the machine to load this image preview.'}}):route.fulfill({contentType:'image/png',body:bytes})})
  await page.addInitScript(()=>{
   window.__blobCounts={made:0,revoked:0}
   const make=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL)
   URL.createObjectURL=blob=>{window.__blobCounts.made++;return make(blob)}
   URL.revokeObjectURL=url=>{window.__blobCounts.revoked++;return revoke(url)}
   class Stream {
    constructor(){this.listeners={};window.__chatStream=this;setTimeout(()=>this.onopen?.({}),0)}
    addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}
    emit(type,data){for(const fn of this.listeners[type]??[])fn({data:JSON.stringify(data)})}
    close(){}
   }
   window.EventSource=Stream
  })
  async function open(){
   await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
   await page.waitForFunction(()=>window.__chatStream)
   await page.evaluate(picture=>{
    window.__chatStream.emit('reset',{truncated:false})
    for(const event of [{seq:1,kind:'tool',toolCallId:'read-shot',toolKind:'read',title:'Read screenshot',status:'completed',text:'Read image successfully.'},{seq:2,kind:'preview',toolCallId:'read-shot',preview:picture},{seq:3,kind:'agent',messageId:'summary',text:'Here is the screenshot for review.'},{seq:4,kind:'status',state:'idle'}])window.__chatStream.emit('chat',event)
   },picture)
  }
  try {
   await open()
   const card=page.getByRole('figure',{name:'Agent image: Session screenshot.png'})
   await expect(card).toBeVisible()
   await expect(card).toContainText('Reconnect the machine')
   refuse=false
   await card.getByRole('button',{name:'Retry preview'}).click()
   const thumbnail=card.getByRole('img',{name:'Session screenshot.png',exact:true})
   await expect(thumbnail).toBeVisible()
   await expect.poll(()=>thumbnail.evaluate(img=>img.naturalWidth)).toBeGreaterThan(100)
   await card.getByRole('button',{name:'Open image preview: Session screenshot.png'}).click()
   const lightbox=page.getByRole('dialog',{name:'Image preview: Session screenshot.png'})
   await expect(lightbox).toBeVisible()
   await expect(lightbox.getByRole('button',{name:'Close preview'})).toBeFocused()
   await page.keyboard.press('Escape')
   await expect(lightbox).toHaveCount(0)
   await expect(card.getByRole('button',{name:'Open image preview: Session screenshot.png'})).toBeFocused()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
   if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/chat-preview-${width}-${theme}.png`,fullPage:true})}
   // Replay after reload still resolves the same captured ID.
   await open();await expect(card.getByRole('img')).toBeVisible()
   await page.evaluate(()=>window.__chatStream.emit('chat',{seq:5,kind:'preview',preview:{name:'Oversized screenshot.png',reason:'too_large'}}))
   await expect(page.getByRole('figure',{name:'Agent image: Oversized screenshot.png'})).toContainText('5 MB preview limit')
   const counts=await page.evaluate(()=>window.__blobCounts);expect(counts.made).toBeGreaterThan(0)
   await page.evaluate(()=>window.__chatStream.emit('reset',{truncated:false}))
   await expect(card).toHaveCount(0)
   await expect.poll(()=>page.evaluate(()=>window.__blobCounts.revoked)).toBeGreaterThanOrEqual(counts.made)
   expect(fetches).toBeGreaterThanOrEqual(3)
   expect(errors).toEqual([])
  } finally {await context.close()}
 }
 console.log('Agent image previews: desktop/phone light/dark, lazy authenticated fetch, offline retry, full-size keyboard viewer, replay, limits and object URL cleanup passed.')
} finally {await browser.close()}
