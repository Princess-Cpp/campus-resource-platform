package m1

import (
 "context"
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "time"
)
func ID()string{b:=make([]byte,16);if _,err:=rand.Read(b);err!=nil{panic(err)};return hex.EncodeToString(b)}
func hash(v any)string{b,_:=json.Marshal(v);h:=sha256.Sum256(b);return hex.EncodeToString(h[:])}
func (s *Service)tx(ctx context.Context)(*sql.Tx,error){return s.DB.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelReadCommitted})}
func WithRoomLock(ctx context.Context,tx *sql.Tx,id string)error{
 if _,err:=tx.ExecContext(ctx,"INSERT IGNORE INTO m1_room_locks VALUES (?)",id);err!=nil{return err}
 var got string;return tx.QueryRowContext(ctx,"SELECT room_id FROM m1_room_locks WHERE room_id=? FOR UPDATE",id).Scan(&got)
}
func (s *Service)room(ctx context.Context,tx *sql.Tx,id string)(Room,error){rooms,err:=s.Catalog.Rooms(ctx,tx);if err!=nil{return Room{},err};for _,r:=range rooms{if r.ID==id{return r,nil}};return Room{},bad("NOT_FOUND",404)}
func (s *Service)interval(start,end time.Time,max int)error{
 a,b:=start.In(s.Zone),end.In(s.Zone)
 if a.Minute()!=0||a.Second()!=0||a.Nanosecond()!=0||b.Minute()!=0||b.Second()!=0||b.Nanosecond()!=0||!end.After(start)||end.Sub(start)>time.Duration(max)*time.Hour||a.Format("2006-01-02")!=b.Add(-time.Nanosecond).Format("2006-01-02"){return bad("INVALID_INPUT",400)};return nil
}
func (s *Service)access(r Room,a Actor,start,end time.Time)error{
 if !r.Active{return bad("RESOURCE_UNAVAILABLE",409)}
 if r.Department!=""&&r.Department!=a.Department&&a.Service==""{return bad("FORBIDDEN",403)}
 if start.In(s.Zone).Hour()<r.OpenHour || end.In(s.Zone).Hour()>r.CloseHour || (end.In(s.Zone).Hour()==0 && r.CloseHour!=24){return bad("CLOSED",409)};return nil
}
func (s *Service)conflict(ctx context.Context,tx *sql.Tx,room,seat string,start,end time.Time)(bool,error){var n int
 err:=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM m1_sources WHERE room_id=? AND active=1 AND (kind='MAINTENANCE' OR (start_at<? AND end_at>? AND (seat_id='' OR ?='' OR seat_id=?)))`,room,end.UTC(),start.UTC(),seat,seat).Scan(&n);return n>0,err
}
func (s *Service)audit(ctx context.Context,tx *sql.Tx,actor,action,business,module,trace string)error{_,err:=tx.ExecContext(ctx,"INSERT INTO m1_audit VALUES (?,?,?,?,?,?,?)",ID(),actor,action,business,module,trace,s.Now().UTC());return err}
func (s *Service)notify(ctx context.Context,tx *sql.Tx,b Booking,kind string)error{_,err:=tx.ExecContext(ctx,"INSERT IGNORE INTO m1_outbox(event_id,user_id,booking_id,kind,created_at) VALUES (?,?,?,?,?)",b.ID+":"+kind,b.UserID,b.ID,kind,s.Now().UTC());return err}
const bookingCols="id,user_id,room_id,seat_id,room_name,seat_label,start_at,end_at,status,reason"
func scanBooking(row interface{Scan(...any)error})(b Booking,err error){err=row.Scan(&b.ID,&b.UserID,&b.RoomID,&b.SeatID,&b.RoomName,&b.SeatLabel,&b.StartAt,&b.EndAt,&b.Status,&b.Reason);return}
func (s *Service)Book(ctx context.Context,a Actor,q BookRequest,key,trace string)(Booking,error){
 if key==""||len(key)>128{return Booking{},bad("INVALID_IDEMPOTENCY_KEY",400)}
 tx,err:=s.tx(ctx);if err!=nil{return Booking{},err};defer tx.Rollback()
 if err=WithRoomLock(ctx,tx,q.RoomID);err!=nil{return Booking{},err}
 var oldHash string;b,err:=scanBooking(tx.QueryRowContext(ctx,"SELECT "+bookingCols+" FROM m1_bookings WHERE user_id=? AND idem_key=?",a.ID,key))
 if err==nil{err=tx.QueryRowContext(ctx,"SELECT request_hash FROM m1_bookings WHERE id=?",b.ID).Scan(&oldHash);if err!=nil{return b,err};if oldHash!=hash(q){return b,bad("IDEMPOTENCY_CONFLICT",409)};return b,nil};if err!=sql.ErrNoRows{return b,err}
 var max int;if err=tx.QueryRowContext(ctx,"SELECT max_hours FROM m1_config WHERE id=1").Scan(&max);err!=nil{return b,err}
 if err=s.interval(q.StartAt,q.EndAt,max);err!=nil{return b,err};if !q.StartAt.After(s.Now()){return b,bad("INVALID_INPUT",400)}
 r,err:=s.room(ctx,tx,q.RoomID);if err!=nil{return b,err};if err=s.access(r,a,q.StartAt,q.EndAt);err!=nil{return b,err}
 var label string;for _,seat:=range r.Seats{if seat.ID==q.SeatID&&seat.Active{label=seat.Label}};if label==""{return b,bad("NOT_FOUND",404)}
 clash,err:=s.conflict(ctx,tx,q.RoomID,q.SeatID,q.StartAt,q.EndAt);if err!=nil{return b,err};if clash{return b,bad("RESOURCE_CONFLICT",409)}
 b=Booking{ID(),a.ID,r.ID,q.SeatID,r.Name,label,q.StartAt.UTC(),q.EndAt.UTC(),"RESERVED",""}
 _,err=tx.ExecContext(ctx,`INSERT INTO m1_bookings(id,user_id,room_id,seat_id,room_name,seat_label,start_at,end_at,status,created_at,idem_key,request_hash) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,b.ID,b.UserID,b.RoomID,b.SeatID,b.RoomName,b.SeatLabel,b.StartAt,b.EndAt,b.Status,s.Now().UTC(),key,hash(q));if err!=nil{return b,err}
 _,err=tx.ExecContext(ctx,"INSERT INTO m1_sources VALUES ('M1',?,?,?,1,1,'SEAT',?,?,?,NULL)",b.ID,r.ID,b.SeatID,b.StartAt,b.EndAt,s.Now().UTC());if err!=nil{return b,err}
 if err=s.audit(ctx,tx,a.ID,"BOOK",b.ID,"M1",trace);err!=nil{return b,err};if err=s.notify(ctx,tx,b,"CREATED");err!=nil{return b,err};return b,tx.Commit()
}
func (s *Service)History(ctx context.Context,a Actor,page,size int)([]Booking,error){rows,err:=s.DB.QueryContext(ctx,"SELECT "+bookingCols+" FROM m1_bookings WHERE user_id=? AND hidden_at IS NULL ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?",a.ID,size,(page-1)*size);if err!=nil{return nil,err};defer rows.Close();out:=[]Booking{};for rows.Next(){b,e:=scanBooking(rows);if e!=nil{return nil,e};out=append(out,b)};return out,rows.Err()}
// Hide removes a finished record from the owner's history only; the row, audit
// and violation records stay. No-show records remain visible for appeals.
func (s *Service)Hide(ctx context.Context,a Actor,id,trace string)error{
 tx,err:=s.tx(ctx);if err!=nil{return err};defer tx.Rollback()
 var status,reason string;var hidden sql.NullTime
 err=tx.QueryRowContext(ctx,"SELECT status,reason,hidden_at FROM m1_bookings WHERE id=? AND user_id=? FOR UPDATE",id,a.ID).Scan(&status,&reason,&hidden);if err==sql.ErrNoRows{return bad("NOT_FOUND",404)};if err!=nil{return err}
 if hidden.Valid{return nil}
 if status!="FINISHED"&&(status!="CANCELLED"||reason=="NO_SHOW"){return bad("INVALID_STATE",409)}
 if _,err=tx.ExecContext(ctx,"UPDATE m1_bookings SET hidden_at=? WHERE id=?",s.Now().UTC(),id);err!=nil{return err}
 if err=s.audit(ctx,tx,a.ID,"HIDE",id,"M1",trace);err!=nil{return err};return tx.Commit()
}
func (s *Service)Code(room string,day time.Time)string{h:=hmac.New(sha256.New,s.CodeSecret);h.Write([]byte(room+"|"+day.In(s.Zone).Format("2006-01-02")));return fmt.Sprintf("%X",h.Sum(nil)[:4])}
func (s *Service)Change(ctx context.Context,a Actor,id,action,code,trace string)(Booking,error){
 var room string;if err:=s.DB.QueryRowContext(ctx,"SELECT room_id FROM m1_bookings WHERE id=? AND user_id=?",id,a.ID).Scan(&room);err!=nil{if err==sql.ErrNoRows{return Booking{},bad("NOT_FOUND",404)};return Booking{},err}
 tx,err:=s.tx(ctx);if err!=nil{return Booking{},err};defer tx.Rollback();if err=WithRoomLock(ctx,tx,room);err!=nil{return Booking{},err}
 b,err:=scanBooking(tx.QueryRowContext(ctx,"SELECT "+bookingCols+" FROM m1_bookings WHERE id=? AND user_id=? FOR UPDATE",id,a.ID));if err!=nil{return b,err};now:=s.Now().UTC()
 if action=="CHECK_IN"{
  if b.Status=="CHECKED_IN"{return b,nil};if b.Status!="RESERVED"||now.Before(b.StartAt.Add(-15*time.Minute))||!now.Before(b.StartAt.Add(15*time.Minute)){return b,bad("INVALID_STATE",409)}
  if !hmac.Equal([]byte(code),[]byte(s.Code(room,now))){return b,bad("INVALID_CODE",400)}
  b.Status="CHECKED_IN";_,err=tx.ExecContext(ctx,"UPDATE m1_bookings SET status=?,checked_in_at=? WHERE id=?",b.Status,now,id)
 }else{
  if b.Status=="CANCELLED"{return b,nil};if b.Status!="RESERVED"||!now.Before(b.StartAt){return b,bad("INVALID_STATE",409)};b.Status="CANCELLED";b.Reason="USER_CANCELLED"
  _,err=tx.ExecContext(ctx,"UPDATE m1_bookings SET status=?,reason=? WHERE id=?",b.Status,b.Reason,id);if err==nil{_,err=tx.ExecContext(ctx,"UPDATE m1_sources SET active=0,version=version+1,released_at=? WHERE module='M1' AND business_id=?",now,id)}
 };if err!=nil{return b,err};if err=s.audit(ctx,tx,a.ID,action,id,"M1",trace);err!=nil{return b,err};return b,tx.Commit()
}
