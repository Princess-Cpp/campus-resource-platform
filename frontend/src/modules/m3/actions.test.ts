import assert from "node:assert/strict"
import test from "node:test"
import { createHTTPGateway } from "./transport.ts"
import { ActionSubmission, actionStore } from "./action-submission.ts"
import type { ActionInput, Order } from "./types.ts"
import {durationText,ratioText} from "./operations.ts"
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

test("repair finish and rejection require real comments before HTTP",async()=>{
 let calls=0
 const api={act:async():Promise<Order>=>{calls++;return {id:"order",version:5} as Order}}
 const controller=new ActionSubmission(api,"order",actionStore("actor","order",storage()))
 for(const action of ["FINISH","REJECT_ACCEPTANCE"] as const){await assert.rejects(controller.send({action,expectedVersion:4,comment:"  "}),/维修结果或驳回原因/)}
 await assert.rejects(controller.send({action:"FINISH",expectedVersion:4,comment:"x".repeat(1001)}),/1000/)
 assert.equal(calls,0)
 await controller.send({action:"FINISH",expectedVersion:4,comment:" 已更换灯管并测试照明正常 "})
 assert.equal(calls,1)
})

test("durable pending start is accepted without claiming repair has started",async()=>{
 const s=storage();let calls=0
 const result={id:"order",version:5,state:"ASSIGNED",syncState:"PENDING",operation:{id:"original-op",action:"BLOCK_ROOM_MAINTENANCE",state:"PENDING",attempts:0}} as Order
 const controller=new ActionSubmission({act:async()=>{calls++;return result}},"order",actionStore("actor","order",s))
 assert.deepEqual(await controller.send({action:"START",expectedVersion:4}),result)
 assert.equal(controller.locked,false);assert.equal(calls,1)
 assert.equal(new ActionSubmission({act:async()=>result},"order",actionStore("actor","order",s)).draft,undefined)
})

test("lost approval response retains approval and cannot become rejection",async()=>{
 const s=storage();const calls:{q:ActionInput;key:string}[]=[]
 const api={act:async(_id:string,q:ActionInput,key:string):Promise<Order>=>{calls.push({q,key});if(calls.length===1)throw new Error("lost");return {id:"order",version:8,state:"AWAITING_ACCEPTANCE",syncState:"UNKNOWN"} as Order}}
 const q:ActionInput={action:"APPROVE_ACCEPTANCE",expectedVersion:6,comment:"验收检查合格"}
 await assert.rejects(new ActionSubmission(api,"order",actionStore("actor","order",s),()=>"approval-key").send(q))
 const restored=new ActionSubmission(api,"order",actionStore("actor","order",s))
 await assert.rejects(restored.send({...q,action:"REJECT_ACCEPTANCE"}),/原操作/)
 await restored.send(q)
 assert.equal(calls.length,2);assert.equal(calls[1].key,"approval-key");assert.deepEqual(calls[1].q,q)
})

test("acceptance queue and resource retry stay within M3 HTTP contract",async()=>{
 const calls:{path:string;init?:RequestInit}[]=[]
 const api=createHTTPGateway(async<T>(path:string,init?:RequestInit)=>{calls.push({path,init});return {} as T})
 await api.list({view:"acceptance",page:1,pageSize:20})
 await api.act("order",{action:"RETRY_RESOURCE",expectedVersion:12},"retry-command")
 assert.equal(new URLSearchParams(calls[0].path.split("?")[1]).get("view"),"acceptance")
 assert.equal(calls[1].path,"/m3/workorders/order/actions")
 assert.deepEqual(JSON.parse(calls[1].init!.body as string),{action:"RETRY_RESOURCE",expectedVersion:12})
})

test("rating inputs require an integer score and at most 500 Unicode characters",async()=>{
 let calls=0;const controller=new ActionSubmission({act:async()=>{calls++;return {id:"order",version:9} as Order}},"order",actionStore("reporter","order",storage()))
 for(const score of [undefined,0,6,2.5])await assert.rejects(controller.send({action:"EVALUATE",expectedVersion:8,score}),/整数评分/)
 await assert.rejects(controller.send({action:"EVALUATE",expectedVersion:8,score:5,comment:"字".repeat(501)}),/500/)
 await assert.rejects(controller.send({action:"CANCEL",expectedVersion:8,score:5}),/其他操作/)
 assert.equal(calls,0)
 await controller.send({action:"EVALUATE",expectedVersion:8,score:5,comment:"字".repeat(500)});assert.equal(calls,1)
})

test("lost rating keeps its score and original key across page reconstruction",async()=>{
 const s=storage();const calls:{q:ActionInput;key:string}[]=[]
 const api={act:async(_id:string,q:ActionInput,key:string):Promise<Order>=>{calls.push({q,key});if(calls.length===1)throw new Error("response lost");return {id:"order",version:9} as Order}}
 const q:ActionInput={action:"EVALUATE",expectedVersion:8,score:4,comment:"使用正常"}
 await assert.rejects(new ActionSubmission(api,"order",actionStore("reporter","order",s),()=>"rating-key").send(q))
 const restored=new ActionSubmission(api,"order",actionStore("reporter","order",s))
 await assert.rejects(restored.send({...q,score:5}),/原操作/)
 await restored.send(q);assert.deepEqual(calls,[{q,key:"rating-key"},{q,key:"rating-key"}]);assert.equal(restored.locked,false)
})

test("operations adapter preserves date cohort, pagination and active=false",async()=>{
 const calls:string[]=[];const api=createHTTPGateway(async<T>(path:string)=>{calls.push(path);return {} as T})
 await api.stats({from:"2030-01-01T00:00:00+08:00",to:"2030-01-02T00:00:00+08:00",category:"LIGHTING"})
 await api.escalations({page:2,pageSize:20,active:false})
 const first=new URLSearchParams(calls[0].split("?")[1]);assert.equal(first.get("from"),"2030-01-01T00:00:00+08:00");assert.equal(first.get("category"),"LIGHTING")
 assert.equal(calls[1],"/m3/escalations?page=2&pageSize=20&active=false")
})

test("empty metrics remain distinct from zero-duration and zero-overdue results",()=>{
 assert.equal(durationText(null),"暂无样本");assert.equal(durationText(0),"0 秒");assert.equal(durationText(90),"1.5 分钟");assert.equal(durationText(5400),"1.5 小时")
 assert.equal(ratioText(null),"暂无样本");assert.equal(ratioText(0),"0.0%");assert.equal(ratioText(1/3),"33.3%")
})
