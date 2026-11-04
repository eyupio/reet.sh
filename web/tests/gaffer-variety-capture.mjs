// Capture the real repertoire in both themes; animated output also requires FFmpeg.
import {chromium,expect} from '@playwright/test'
import {startHarness} from './helpers/harness.mjs'
import {mkdir,mkdtemp,rm} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import {fileURLToPath} from 'node:url'
import {execFileSync} from 'node:child_process'
const output=fileURLToPath(new URL('../../docs/images/ui/',import.meta.url))
const harness=await startHarness();const browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const frames=await mkdtemp(join(tmpdir(),'reet-ambient-frames-'))
try{
 await mkdir(output,{recursive:true})
 const page=await browser.newPage({viewport:{width:1100,height:1800}})
 const errors=[];page.on('pageerror',error=>errors.push(error.message))
 await page.goto(harness.url+'/tests/fixtures/gaffer-artwork.html?theme=dark')
 await page.locator('.repertoire image').evaluateAll(images=>Promise.all(images.map(async el=>{const im=new Image();im.src=el.href.baseVal;await im.decode()})))
 await page.evaluate(()=>{window.__motionClips=[...document.querySelectorAll('.repertoire .gaffer-avatar')].flatMap(el=>el.getAnimations({subtree:true}));for(const a of window.__motionClips){a.pause();a.currentTime=Number(a.effect.getTiming().duration)*.5}})
 await page.locator('.repertoire').screenshot({path:output+'/gaffer-ambient-review-dark.png',animations:'allow'})
 await page.evaluate(()=>document.documentElement.dataset.theme='light')
 await page.locator('.repertoire').screenshot({path:output+'/gaffer-ambient-review-light.png',animations:'allow'})
 await page.evaluate(()=>document.documentElement.dataset.theme='dark')
 for(let frame=0;frame<60;frame++){
  await page.evaluate(time=>{for(const a of window.__motionClips)a.currentTime=time},frame*100)
  await page.locator('.repertoire').screenshot({path:join(frames,String(frame).padStart(3,'0')+'.png'),animations:'allow'})
 }
 execFileSync('ffmpeg',['-hide_banner','-loglevel','error','-y','-framerate','10','-i',join(frames,'%03d.png'),'-filter_complex','[0:v]scale=600:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle','-loop','0',output+'/gaffer-ambient-performances.gif'])
 expect(errors).toEqual([])
 console.log('Recorded the twelve actual performances and both theme review sheets.')
}finally{await browser.close();await harness.close();await rm(frames,{recursive:true,force:true})}
