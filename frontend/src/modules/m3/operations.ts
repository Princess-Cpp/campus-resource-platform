export function durationText(seconds:number|null):string {
 if(seconds===null || !Number.isFinite(seconds) || seconds<0)return "暂无样本"
 if(seconds<60)return `${Math.round(seconds)} 秒`
 if(seconds<3600)return `${(seconds/60).toFixed(1)} 分钟`
 return `${(seconds/3600).toFixed(1)} 小时`
}
export function ratioText(ratio:number|null):string{return ratio===null?"暂无样本":`${(ratio*100).toFixed(1)}%`}
export function routingText(state:string):string{
 return ({PENDING:"待核对主管",NO_SUPERVISOR:"未配置辖区主管",DIRECTORY_UNAVAILABLE:"人员目录暂不可用",QUEUED:"主管通知意图已入队，待投递",OBSOLETE:"原阶段已结束，停止补建通知"} as Record<string,string>)[state]??state
}
