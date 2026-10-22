import assert from "node:assert/strict"
import test from "node:test"
import { createHTTPGateway } from "./transport.ts"
import { ActionSubmission, actionStore } from "./action-submission.ts"
import type { ActionInput, Order } from "./types.ts"
const input: ActionInput = { action: "ASSIGN", expectedVersion: 2, assigneeId: "worker-a", comment: "检查灯管并处理" }
function storage() {
 const values=new Map<string,string>()
 return {getItem:(k:string)=>values.get(k)??null,setItem:(k:string,v:string)=>{values.set(k,v)},removeItem:(k:string)=>{values.delete(k)}}
}
test("action HTTP adapter preserves ID, version, key and dispatch view",async()=>{
 const calls:{path:string;init?:RequestInit}[]=[]
 const api=createHTTPGateway(async<T>(path:string,init?:RequestInit)=>{calls.push({path,init});return {} as T})
 await api.act("id/encoded",input,"same-key");await api.assignees("id/encoded");await api.list({view:"dispatch",page:1,pageSize:20})
 assert.equal(calls[0].path,"/m3/workorders/id%2Fencoded/actions")
 assert.deepEqual(JSON.parse(calls[0].init!.body as string),input)
 assert.deepEqual(calls[0].init?.headers,{"Idempotency-Key":"same-key"})
 assert.equal(calls[1].path,"/m3/workorders/id%2Fencoded/assignees")
 assert.equal(new URLSearchParams(calls[2].path.split("?")[1]).get("view"),"dispatch")
})
test("unknown action keeps its original target, version and key across reconstruction",async()=>{
 const s=storage();const calls:{input:ActionInput;key:string}[]=[]
 const api={act:async(_id:string,q:ActionInput,key:string):Promise<Order>=>{calls.push({input:structuredClone(q),key});if(calls.length===1)throw new Error("reply lost");return {id:"order",version:3} as Order}}
 const first=new ActionSubmission(api,"order",actionStore("actor","order",s),()=>"original")
 await assert.rejects(first.send(input));assert.equal(first.locked,true)
 const restored=new ActionSubmission(api,"order",actionStore("actor","order",s),()=>{throw new Error("new key forbidden")})
 assert.deepEqual(restored.draft,input)
 await assert.rejects(restored.send({...input,assigneeId:"other-worker"}),/原操作/)
 await restored.send(input)
 assert.deepEqual(calls,[{input,key:"original"},{input,key:"original"}]);assert.equal(restored.locked,false)
 assert.equal(new ActionSubmission(api,"order",actionStore("other","order",s)).locked,false)
})
test("double action click sends once; stale-version rejection unlocks fresh selection",async()=>{
 let release!:(o:Order)=>void;let calls=0
 const api={act:async():Promise<Order>=>{calls++;return new Promise(resolve=>{release=resolve})}}
 const controller=new ActionSubmission(api,"order",actionStore("actor","order",storage()),()=>"one")
 const first=controller.send(input);const second=controller.send(input)
 assert.equal(calls,1);release({id:"order",version:3} as Order);await Promise.all([first,second])
 api.act=async()=>{throw Object.assign(new Error("version"),{status:409})}
 await assert.rejects(controller.send(input));assert.equal(controller.locked,false)
})
test("damaged storage and invalid action inputs never reach HTTP",async()=>{
 let calls=0;const api={act:async()=>{calls++;return {} as Order}}
 const broken={getItem:()=>"{broken",setItem(){},removeItem(){}}
 const controller=new ActionSubmission(api,"order",actionStore("actor","order",broken))
 await assert.rejects(controller.send(input),/未确认/)
 const normal=new ActionSubmission(api,"order",actionStore("actor","order",storage()))
 await assert.rejects(normal.send({...input,expectedVersion:0}),/版本/)
 await assert.rejects(normal.send({...input,assigneeId:""}),/维修人员/)
 assert.equal(calls,0)
})
test("malformed success and forbidden replay retain an earlier unknown command",async()=>{
 let attempt=0;const api={act:async():Promise<Order>=>{if(attempt++===0)return undefined as unknown as Order;throw Object.assign(new Error("forbidden"),{status:403})}}
 const controller=new ActionSubmission(api,"order",actionStore("actor","order",storage()))
 await assert.rejects(controller.send(input),/核对/);await assert.rejects(controller.send(input));assert.equal(controller.locked,true)
})
