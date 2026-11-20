import {readFileSync} from 'node:fs'
import {test} from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const compile=file=>ts.transpileModule(readFileSync(new URL(file,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText
const data=code=>`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const {sameHost,describeError,isSignedOut,SIGNED_OUT}=await import(data(compile('./contentReview.ts')))
const inflight=data(compile('./inflight.ts'))
const api=await import(data(compile('./api.ts').replace("from './inflight'",`from '${inflight}'`)))
test('machine identity folds ASCII as SQLite NOCASE does, preserving different Unicode names',()=>{
 assert.ok(sameHost('LAPTOP','laptop'))
 assert.ok(!sameHost('Ë','ë'))
 assert.ok(!sameHost('K','k'))
 assert.ok(!sameHost('laptop','laptop '))
})
test('readable failures keep the server message without exposing browser errors or stacks',()=>{
 assert.equal(describeError(new Error('This skill changed. Refresh and try again.')),'This skill changed. Refresh and try again.')
 assert.equal(describeError(new TypeError('Failed to fetch')),"Couldn't reach reet. Check your connection and try again.")
 for(const error of [new SyntaxError('Unexpected token <'),new TypeError('bad code'),'500 Internal Server Error','Error: bad\nat file.js:1:2','x'.repeat(301),null])assert.equal(describeError(error),'Something went wrong. Try again.')
 assert.ok(isSignedOut({name:'UnauthorizedError'}));assert.ok(!isSignedOut(new Error('401')))
 assert.match(SIGNED_OUT,/Sign in again/)
})
test('retained API writes preserve refusal status and server text, while 401 hands over sign-in',async()=>{
 let status=409
 globalThis.fetch=async()=>new Response(JSON.stringify({error:status===401?'sign in again':'This revision changed.'}),{status})
 await assert.rejects(api.saveSkillContent('item','old',[]),e=>e instanceof api.ApiError&&e.status===409&&describeError(e)==='This revision changed.')
 status=401
 await assert.rejects(api.getPrivateBundles(),e=>e instanceof api.UnauthorizedError&&!(e instanceof api.ApiError)&&isSignedOut(e))
})
