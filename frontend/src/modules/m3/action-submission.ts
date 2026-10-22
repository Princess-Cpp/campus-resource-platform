import type { ActionInput, ActionGateway, Order } from "./types.ts"

type Pending = { key: string; input: ActionInput }
export interface ActionStore { load(): Pending | undefined; save(value: Pending): void; clear(): void }
function normalize(input: ActionInput): ActionInput {
 if (!["ASSIGN","ACCEPT_ASSIGNMENT","CANCEL","START","FINISH","REJECT_ACCEPTANCE","APPROVE_ACCEPTANCE","RETRY_RESOURCE"].includes(input.action) || !Number.isSafeInteger(input.expectedVersion) || input.expectedVersion<1) throw new Error("操作或版本无效，请刷新工单")
 if (input.action==="ASSIGN" && !input.assigneeId) throw new Error("请选择维修人员")
 if (input.action!=="ASSIGN" && input.assigneeId) throw new Error("此操作不接受维修人员参数")
 const comment=(input.comment??"").trim()
 if (Array.from(comment).length>1000 || (["ASSIGN","FINISH","REJECT_ACCEPTANCE"].includes(input.action) && !comment)) throw new Error("请填写 1–1000 字的处理建议、维修结果或驳回原因")
 return {action:input.action,expectedVersion:input.expectedVersion,...(input.assigneeId?{assigneeId:input.assigneeId}:{}),comment}
}
export function actionStore(actor: string,id: string,storage: Pick<Storage,"getItem"|"setItem"|"removeItem">): ActionStore {
 if (!actor || !id) throw new Error("无法识别当前用户或工单")
 const name="campus.m3.action."+encodeURIComponent(actor)+"."+encodeURIComponent(id)
 return {
  load(){
   const raw=storage.getItem(name);if(!raw)return undefined
   const value:unknown=JSON.parse(raw)
   if(!value || typeof value!=="object" || !("key" in value) || typeof value.key!=="string" || !/^[A-Za-z0-9._:-]{1,128}$/.test(value.key) || !("input" in value) || !value.input || typeof value.input!=="object") throw new Error("未确认操作损坏")
   const q=value.input as ActionInput
   if(typeof q.action!=="string" || (q.comment!==undefined && typeof q.comment!=="string") || (q.assigneeId!==undefined && typeof q.assigneeId!=="string")) throw new Error("未确认操作不完整")
   return {key:value.key,input:normalize(q)}
  },
  save(value){storage.setItem(name,JSON.stringify(value))},clear(){storage.removeItem(name)},
 }
}
export class ActionSubmission {
 private pending?:Pending
 private inFlight?:Promise<Order>
 private uncertain=false
 readonly problem?:Error
 private api:Pick<ActionGateway,"act">
 private id:string
 private store:ActionStore
 private key:()=>string
 constructor(api:Pick<ActionGateway,"act">,id:string,store:ActionStore,key:()=>string=()=>crypto.randomUUID()) {
  this.api=api;this.id=id;this.store=store;this.key=key
  try{this.pending=store.load();this.uncertain=!!this.pending}catch{this.problem=new Error("无法读取未确认操作，请保留页面并联系维护人员核对")}
 }
 get locked(){return !!this.pending}
 get draft(){return this.pending?structuredClone(this.pending.input):undefined}
 send(input:ActionInput):Promise<Order> {
  if(this.problem)return Promise.reject(this.problem)
  let q:ActionInput;try{q=normalize(input)}catch(e){return Promise.reject(e)}
  if(this.pending && JSON.stringify(this.pending.input)!==JSON.stringify(q))return Promise.reject(new Error("原操作结果尚待核对，请重试原操作"))
  if(this.inFlight)return this.inFlight
  if(!this.pending){
   const p={input:q,key:this.key()}
   try{this.store.save(p)}catch{return Promise.reject(new Error("无法保存未确认操作，尚未发送请求"))}
   this.pending=p
  }
  const p=this.pending
  this.inFlight=this.api.act(this.id,structuredClone(p.input),p.key).then(result=>{
   if(!result || result.id!==this.id || !Number.isSafeInteger(result.version) || result.version<=p.input.expectedVersion)throw new Error("响应不完整，请核对原操作")
   this.store.clear();this.pending=undefined;this.uncertain=false;return result
  }).catch(error=>{
   const status=error && typeof error==="object" && "status" in error?Number(error.status):0
   if(!this.uncertain && status>=400 && status<500 && status!==408 && status!==429){this.store.clear();this.pending=undefined}else this.uncertain=true
   throw error
  }).finally(()=>{this.inFlight=undefined})
  return this.inFlight
 }
}
