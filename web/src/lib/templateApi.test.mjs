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
