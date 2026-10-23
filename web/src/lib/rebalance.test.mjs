import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
async function helper(name) {
 const { outputText } = ts.transpileModule(readFileSync(new URL(`./${name}.ts`,import.meta.url),'utf8'),{ compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022} })
 return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}
const {setupContents,compareSetups,shellQuote}=await helper('setupProfiles')
const {matchesListScope}=await helper('lists')
const revision={revision:1,preview:{schema_version:1,agents:[{name:'Codex',components:[{kind:'settings',source:'/home/config'}]}],skills:[{name:'alpha',agent_id:'codex',source:'/home/skills/alpha'},{name:'beta',agent_id:'claude',source:'/home/skills/beta'}],memories:[{name:'memory',agent_id:'codex',source:'/home/db'}]}}
test('restore coverage applies global and selected scopes without changing capture metadata',()=>{
 const scoped={...revision,overrides:{exclude_kinds:['memory'],machines:{laptop:{exclude_sources:['/home/skills']}},projects:{'/project':{exclude_kinds:['settings']}}}}
 assert.deepEqual(setupContents(scoped).count,{components:1,skills:2,memories:0})
 const result=setupContents(scoped,'laptop','/project')
 assert.deepEqual(result.count,{components:0,skills:0,memories:0})
 assert.deepEqual(result.captured,{components:1,skills:2,memories:1})
 assert.equal(result.excluded,4)
 assert.equal(revision.preview.skills.length,2)
})
test('revision comparison exposes exclusion effects',()=>{
 const next={...revision,revision:2,overrides:{exclude_kinds:['skills']}}
 const diff=compareSetups(revision,next)
 assert.equal(diff.removed.length,2);assert.equal(diff.exclusionsChanged,true)
 assert.equal(diff.added.length,0)
})
test('all accessible lists include administrative access, sharing remains a separate relationship',()=>{
 const list={ownerId:2,role:'admin',visibility:'private',sharedWithMe:true}
 assert.equal(matchesListScope(list,'all',1),true)
 assert.equal(matchesListScope(list,'people',1),true)
 assert.equal(matchesListScope({...list,sharedWithMe:false},'people',1),false)
 assert.equal(matchesListScope(list,'mine',1),false)
})
test('generated local commands quote shell metacharacters as data',()=>{
 assert.equal(shellQuote("a'b;$(x)"),"'a'\\''b;$(x)'")
})
