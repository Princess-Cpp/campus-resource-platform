export function durationText(seconds:number|null):string {
 if(seconds===null || !Number.isFinite(seconds) || seconds<0)return "暂无样本"
 if(seconds<60)return `${Math.round(seconds)} 秒`
 if(seconds<3600)return `${(seconds/60).toFixed(1)} 分钟`
 return `${(seconds/3600).toFixed(1)} 小时`
}
export function ratioText(ratio:number|null):string{return ratio===null?"暂无样本":`${(ratio*100).toFixed(1)}%`}
export function routingText(state:string):string{
 return ({PENDING:"正在确认负责主管",NO_SUPERVISOR:"待指定区域主管",DIRECTORY_UNAVAILABLE:"暂时无法获取主管信息",QUEUED:"正在通知主管",OBSOLETE:"该阶段已结束，无需继续提醒"} as Record<string,string>)[state]??"通知状态待确认"
}
