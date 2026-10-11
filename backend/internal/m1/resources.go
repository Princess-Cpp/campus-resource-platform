package m1

import (
 "context"
 "database/sql"
 "encoding/json"
 "time"
)

type Available struct {Room Room `json:"room"`;Seat Seat `json:"seat"`}
func (s *Service)Search(ctx context.Context,a Actor,start,end time.Time,area string,outlet bool)([]Available,error){
 tx,err:=s.tx(ctx);if err!=nil{return nil,err};defer tx.Rollback();var max int
 if err=tx.QueryRowContext(ctx,"SELECT max_hours FROM m1_config WHERE id=1").Scan(&max);err!=nil{return nil,err};if err=s.interval(start,end,max);err!=nil{return nil,err};if !start.After(s.Now()){return nil,bad("INVALID_INPUT",400)}
 rooms,err:=s.Catalog.Rooms(ctx,tx);if err!=nil{return nil,err};out:=[]Available{}
 for _,r:=range rooms{if s.access(r,a,start,end)!=nil||(area!=""&&r.Area!=area){continue};for _,seat:=range r.Seats{if !seat.Active||(outlet&&!seat.Outlet){continue};clash,e:=s.conflict(ctx,tx,r.ID,seat.ID,start,end);if e!=nil{return nil,e};if !clash{out=append(out,Available{r,seat})}}};return out,nil
}
func (s *Service)Operate(ctx context.Context,a Actor,q Operation,key,trace string)(Receipt,error){
 if a.Service!=q.SourceModule || (a.Service!="M2"&&a.Service!="M3"){return Receipt{},bad("FORBIDDEN",403)}
 if key==""||len(key)>128||q.SourceVersion<1||len(q.Reason)>500{return Receipt{},bad("INVALID_INPUT",400)}
 if (q.Action=="HOLD_ROOM_INTERVAL"&&a.Service!="M2")||(q.Action=="BLOCK_ROOM_MAINTENANCE"&&a.Service!="M3"){return Receipt{},bad("FORBIDDEN",403)}
 if q.Action!="HOLD_ROOM_INTERVAL"&&q.Action!="BLOCK_ROOM_MAINTENANCE"&&q.Action!="RELEASE_SOURCE"{return Receipt{},bad("INVALID_INPUT",400)}
 tx,err:=s.tx(ctx);if err!=nil{return Receipt{},err};defer tx.Rollback();if err=WithRoomLock(ctx,tx,q.RoomID);err!=nil{return Receipt{},err}
 var saved,oldHash,oldID string
 err=tx.QueryRowContext(ctx,"SELECT id,request_hash,response FROM m1_operations WHERE module=? AND idem_key=?",a.Service,key).Scan(&oldID,&oldHash,&saved)
 if err==nil{if oldHash!=hash(q)||oldID!=q.OperationID{return Receipt{},bad("IDEMPOTENCY_CONFLICT",409)};var receipt Receipt;err=json.Unmarshal([]byte(saved),&receipt);return receipt,err};if err!=sql.ErrNoRows{return Receipt{},err}
 var version int64;var oldRoom string
 err=tx.QueryRowContext(ctx,"SELECT version,room_id FROM m1_sources WHERE module=? AND business_id=? FOR UPDATE",q.SourceModule,q.SourceBusinessID).Scan(&version,&oldRoom)
 if err!=nil&&err!=sql.ErrNoRows{return Receipt{},err}
 receipt:=Receipt{q.OperationID,"CONFIRMED",q.SourceModule,q.SourceBusinessID,q.SourceVersion,s.Now().UTC(),""}
 fail:=func(code string){receipt.OperationStatus="FAILED";receipt.ErrorCode=code}
 if err==nil&&(q.SourceVersion<=version||oldRoom!=q.RoomID){fail("VERSION_CONFLICT")}
 if receipt.ErrorCode==""&&q.Action!="RELEASE_SOURCE"{
  r,e:=s.room(ctx,tx,q.RoomID);if e!=nil{return receipt,e};if !r.Active{fail("RESOURCE_UNAVAILABLE")}
  if q.Action=="HOLD_ROOM_INTERVAL"{
   if q.StartAt==nil||q.EndAt==nil||q.DeadlineAt==nil{return receipt,bad("INVALID_INPUT",400)}
   if e=s.interval(*q.StartAt,*q.EndAt,24);e!=nil{return receipt,e}
   if !q.StartAt.After(s.Now())||!s.Now().Before(*q.DeadlineAt){fail("DEADLINE_EXPIRED")}
   if e=s.access(r,a,*q.StartAt,*q.EndAt);e!=nil{fail("RESOURCE_UNAVAILABLE")}
   clash,e:=s.conflict(ctx,tx,r.ID,"",*q.StartAt,*q.EndAt);if e!=nil{return receipt,e};if clash{fail("RESOURCE_CONFLICT")}
  }
 }
 if receipt.ErrorCode==""{
  kind:="MAINTENANCE";active:=true;var start,end any
  if q.Action=="HOLD_ROOM_INTERVAL"{kind="ROOM";start=q.StartAt.UTC();end=q.EndAt.UTC()}
  var released any;if q.Action=="RELEASE_SOURCE"{active=false;kind="RELEASED";released=s.Now().UTC()}
  _,err=tx.ExecContext(ctx,`INSERT INTO m1_sources VALUES (?,?,?,'',?,?, ?,?,?,?,?) ON DUPLICATE KEY UPDATE version=VALUES(version),active=VALUES(active),released_at=VALUES(released_at)`,q.SourceModule,q.SourceBusinessID,q.RoomID,q.SourceVersion,active,kind,start,end,s.Now().UTC(),released);if err!=nil{return receipt,err}
  if q.Action=="BLOCK_ROOM_MAINTENANCE"{if err=s.cancelMaintenance(ctx,tx,q.RoomID,trace);err!=nil{return receipt,err}}
  if err=s.audit(ctx,tx,a.Service,q.Action,q.SourceBusinessID,q.SourceModule,trace);err!=nil{return receipt,err}
 }
 raw,_:=json.Marshal(receipt);_,err=tx.ExecContext(ctx,"INSERT INTO m1_operations VALUES (?,?,?,?,?)",q.OperationID,a.Service,key,hash(q),string(raw));if err!=nil{return receipt,err};return receipt,tx.Commit()
}
func (s *Service)Receipt(ctx context.Context,a Actor,id string)(Receipt,error){var raw string;err:=s.DB.QueryRowContext(ctx,"SELECT response FROM m1_operations WHERE id=? AND module=?",id,a.Service).Scan(&raw);if err==sql.ErrNoRows{return Receipt{},bad("NOT_FOUND",404)};if err!=nil{return Receipt{},err};var r Receipt;err=json.Unmarshal([]byte(raw),&r);return r,err}
func (s *Service)Sources(ctx context.Context,a Actor)([]map[string]any,error){rows,err:=s.DB.QueryContext(ctx,"SELECT business_id,room_id,version,active,kind FROM m1_sources WHERE module=? ORDER BY business_id",a.Service);if err!=nil{return nil,err};defer rows.Close();out:=[]map[string]any{};for rows.Next(){var id,room,kind string;var v int64;var active bool;if err=rows.Scan(&id,&room,&v,&active,&kind);err!=nil{return nil,err};out=append(out,map[string]any{"sourceModule":a.Service,"sourceBusinessId":id,"roomId":room,"sourceVersion":v,"active":active,"kind":kind})};return out,rows.Err()}
