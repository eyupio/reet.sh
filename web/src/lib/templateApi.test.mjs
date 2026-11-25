import {readFileSync} from 'node:fs'
import {test} from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const compile=file=>ts.transpileModule(readFileSync(new URL(file,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText
const data=code=>`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const api=await import(data(compile('./api.ts').replace("from './inflight'",`from '${data(compile('./inflight.ts'))}'`)))

test('a template the server sent without criteria or skills is read with empty lists',async()=>{
 const fetched=globalThis.fetch
 // What the server sends for a pipeline with no criteria and a stage with no skills: both keys left out.
 globalThis.fetch=async()=>new Response(JSON.stringify({id:'t1',version:1,content:{name:'n',pipelines:[{title:'A',prompt:'p',autoMerge:false,stages:[{kind:'code',tier:'standard'}]}]}}),{status:200,headers:{'Content-Type':'application/json'}})
 try{
  const got=await api.getGafferTemplate('t1')
  assert.deepEqual(got.content.pipelines[0].criteria,[])
  assert.deepEqual(got.content.pipelines[0].stages[0].skills,[])
 }finally{globalThis.fetch=fetched}
})

test('a pathway is saved with the time it was read at, and a start names its pathway',async()=>{
 const fetched=globalThis.fetch
 const calls=[]
 globalThis.fetch=async(url,init)=>{calls.push({url:String(url),init});return new Response(JSON.stringify({id:'p1',problems:[],skills:[]}),{status:200,headers:{'Content-Type':'application/json'}})}
 try{
  await api.saveGafferPathway({id:'p/1',name:'n',description:'',enabled:true,stages:[],createdAt:1,updatedAt:7,problems:['x']})
  assert.match(calls[0].url,/\/api\/v2\/gaffer\/pathways\/p%2F1$/)
  assert.equal(calls[0].init.method,'PUT')
  const sent=JSON.parse(calls[0].init.body)
  assert.equal(sent.updatedAt,7)
  assert.equal('problems' in sent,false)
  await api.addGafferPipelines({id:'o1',revision:2},[{title:'t',prompt:'p'}],undefined,'p1')
  assert.equal(JSON.parse(calls[1].init.body).pathway,'p1')
  await api.addGafferPipelines({id:'o1',revision:2},[{title:'t',prompt:'p',stages:[]}])
  assert.equal('pathway' in JSON.parse(calls[2].init.body),false)
  assert.deepEqual(await api.listGafferPathwaySkills(),[])
  assert.match(calls[3].url,/\/api\/v2\/gaffer\/pathways\/skills$/)
 }finally{globalThis.fetch=fetched}
})
