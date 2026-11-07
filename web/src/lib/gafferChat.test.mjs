import test from 'node:test'
import assert from 'node:assert/strict'
import { gafferMessageBytes, gafferMessageId, gafferSendKey, gafferTurnLabel, newerGafferTurn, parseGafferTurn, readGafferReceipt, writeGafferReceipt } from './gafferChat.ts'

test('pending receipt storage separates accounts/scopes and never writes message text', () => {
 const values=new Map();globalThis.sessionStorage={getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)}
 const receipt={conversationId:'a'.repeat(32),id:'b'.repeat(32),revision:7,text:'private message and a secret'}
 writeGafferReceipt(1,'session-a',receipt)
 assert.equal([...values.values()].some(v=>v.includes(receipt.text)),false)
 assert.deepEqual(readGafferReceipt(1,'session-a'),{conversationId:receipt.conversationId,id:receipt.id,revision:7})
 assert.equal(readGafferReceipt(2,'session-a'),null);assert.equal(readGafferReceipt(1,'session-b'),null)
 writeGafferReceipt(1,'session-a',null);assert.equal(readGafferReceipt(1,'session-a'),null)
})
test('malformed and unavailable storage cannot create a send request',()=>{
 globalThis.sessionStorage={getItem:()=>'{bad'};assert.equal(readGafferReceipt(1,''),null)
 globalThis.sessionStorage={getItem:()=>JSON.stringify({conversationId:'wrong',id:'b'.repeat(32),revision:1})};assert.equal(readGafferReceipt(1,''),null)
 globalThis.sessionStorage={getItem:()=>{throw Error('blocked')},setItem:()=>{throw Error('blocked')},removeItem:()=>{throw Error('blocked')}}
 assert.equal(readGafferReceipt(1,''),null);assert.doesNotThrow(()=>writeGafferReceipt(1,'',null))
})
test('Enter sends; Shift or Alt Enter adds a line; a composing IME never sends', () => {
 const key=extra=>({key:'Enter',shiftKey:false,altKey:false,isComposing:false,...extra})
 assert.equal(gafferSendKey(key()),true)
 assert.equal(gafferSendKey(key({shiftKey:true})),false)
 assert.equal(gafferSendKey(key({altKey:true})),false)
 assert.equal(gafferSendKey(key({isComposing:true})),false)
 assert.equal(gafferSendKey(key({key:'a'})),false)
 assert.equal(gafferSendKey(key({key:'Escape'})),false)
})
test('request ids use the server shape and prompts count UTF-8 bytes',()=>{
 const a=gafferMessageId(),b=gafferMessageId();assert.match(a,/^[a-f0-9]{32}$/);assert.notEqual(a,b)
 assert.equal(gafferMessageBytes('☕'.repeat(2667)),8001)
})
test('an older running update cannot reopen a completed reply',()=>{
 const done={id:'a',state:'completed',updatedAt:4};assert.equal(newerGafferTurn(done,{id:'a',state:'running',updatedAt:5}),done)
 assert.equal(newerGafferTurn(done,{id:'a',state:'completed',updatedAt:2}),done)
 assert.equal(gafferTurnLabel({...done,cut:true}),'Reply limit reached')
})
test('stream updates are bound to the expected conversation and receipt',()=>{
 const turn={id:'t',conversationId:'c',prompt:'Hi',answer:'Hello',state:'running',updatedAt:1}
 assert.deepEqual(parseGafferTurn(JSON.stringify(turn),'c','t'),turn)
 assert.throws(()=>parseGafferTurn(JSON.stringify(turn),'other','t'))
 assert.throws(()=>parseGafferTurn(JSON.stringify({...turn,state:'unknown'}),'c','t'))
})
