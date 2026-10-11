package m1

import (
 "context"
 "database/sql"
 "time"
)
func (s *Service)cancelMaintenance(ctx context.Context,tx *sql.Tx,room,trace string)error{
 rows,err:=tx.QueryContext(ctx,"SELECT "+bookingCols+" FROM m1_bookings WHERE room_id=? AND status IN ('RESERVED','CHECKED_IN') AND end_at>? FOR UPDATE",room,s.Now().UTC());if err!=nil{return err}
 list:=[]Booking{};for rows.Next(){b,e:=scanBooking(rows);if e!=nil{rows.Close();return e};list=append(list,b)};err=rows.Err();rows.Close();if err!=nil{return err}
 for _,b:=range list{b.Status="CANCELLED";b.Reason="MAINTENANCE";if err=s.cancelTx(ctx,tx,b,false,trace);err!=nil{return err}};return nil
}
func (s *Service)cancelTx(ctx context.Context,tx *sql.Tx,b Booking,violation bool,trace string)error{
 if _,err:=tx.ExecContext(ctx,"UPDATE m1_bookings SET status=?,reason=? WHERE id=?",b.Status,b.Reason,b.ID);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"UPDATE m1_sources SET active=0,version=version+1,released_at=? WHERE module='M1' AND business_id=?",s.Now().UTC(),b.ID);err!=nil{return err}
 if violation{if _,err:=tx.ExecContext(ctx,"INSERT IGNORE INTO m1_violations VALUES (?,?,?)",b.ID,b.UserID,s.Now().UTC());err!=nil{return err}}
 if err:=s.notify(ctx,tx,b,"CANCELLED");err!=nil{return err};return s.audit(ctx,tx,"scheduler",b.Reason,b.ID,"M1",trace)
}
// Tick is a deterministic, restart-safe callback for M2's eventual shared runner.
// Room locks serialize it with booking, check-in and resource operations.
func (s *Service)Tick(ctx context.Context)error{
 rows,err:=s.DB.QueryContext(ctx,"SELECT DISTINCT room_id FROM m1_bookings WHERE status IN ('RESERVED','CHECKED_IN')");if err!=nil{return err};rooms:=[]string{};for rows.Next(){var room string;if err=rows.Scan(&room);err!=nil{rows.Close();return err};rooms=append(rooms,room)};err=rows.Err();rows.Close();if err!=nil{return err}
 for _,room:=range rooms{if err=s.tickRoom(ctx,room);err!=nil{return err}};return nil
}
func (s *Service)tickRoom(ctx context.Context,room string)error{
 tx,err:=s.tx(ctx);if err!=nil{return err};defer tx.Rollback();if err=WithRoomLock(ctx,tx,room);err!=nil{return err}
 rows,err:=tx.QueryContext(ctx,"SELECT "+bookingCols+" FROM m1_bookings WHERE room_id=? AND status IN ('RESERVED','CHECKED_IN') FOR UPDATE",room);if err!=nil{return err};list:=[]Booking{};for rows.Next(){b,e:=scanBooking(rows);if e!=nil{rows.Close();return e};list=append(list,b)};err=rows.Err();rows.Close();if err!=nil{return err};now:=s.Now().UTC()
 for _,b:=range list{
  if b.Status=="RESERVED"&&!now.Before(b.StartAt.Add(15*time.Minute)){b.Status="CANCELLED";b.Reason="NO_SHOW";if err=s.cancelTx(ctx,tx,b,true,"tick");err!=nil{return err};continue}
  if b.Status=="CHECKED_IN"&&!now.Before(b.EndAt){_,err=tx.ExecContext(ctx,"UPDATE m1_bookings SET status='FINISHED' WHERE id=?",b.ID);if err!=nil{return err};_,err=tx.ExecContext(ctx,"UPDATE m1_sources SET active=0,released_at=? WHERE module='M1' AND business_id=?",now,b.ID);if err!=nil{return err};if err=s.audit(ctx,tx,"scheduler","FINISHED",b.ID,"M1","tick");err!=nil{return err};continue}
  if b.Status=="RESERVED"{
   if !now.Before(b.StartAt.Add(-15*time.Minute))&&now.Before(b.StartAt){if err=s.notify(ctx,tx,b,"PRE_REMINDER");err!=nil{return err}}
   if !now.Before(b.StartAt.Add(10*time.Minute)){if err=s.notify(ctx,tx,b,"LATE_REMINDER");err!=nil{return err}}
  }
 };return tx.Commit()
}
func (s *Service)Notifications(ctx context.Context,a Actor)([]map[string]any,error){rows,err:=s.DB.QueryContext(ctx,"SELECT event_id,booking_id,kind,status,created_at FROM m1_outbox WHERE user_id=? ORDER BY created_at DESC,event_id DESC LIMIT 100",a.ID);if err!=nil{return nil,err};defer rows.Close();out:=[]map[string]any{};for rows.Next(){var event,booking,kind,status string;var at time.Time;if err=rows.Scan(&event,&booking,&kind,&status,&at);err!=nil{return nil,err};out=append(out,map[string]any{"eventId":event,"bookingId":booking,"kind":kind,"status":status,"createdAt":at})};return out,rows.Err()}
// Deliver is an injectable M3 notification adapter. Logical events are unique;
// external delivery is at least once when a response is lost.
func (s *Service)Deliver(ctx context.Context,send func(context.Context,string,string)error)error{
 rows,err:=s.DB.QueryContext(ctx,`SELECT event_id FROM m1_outbox WHERE status='PENDING' AND (next_attempt IS NULL OR next_attempt<=?) ORDER BY created_at LIMIT 50`,s.Now().UTC());if err!=nil{return err};ids:=[]string{};for rows.Next(){var id string;if err=rows.Scan(&id);err!=nil{rows.Close();return err};ids=append(ids,id)};rows.Close()
 for _,id:=range ids{
  if err=s.deliverOne(ctx,id,send);err!=nil{return err}
 };return nil
}
func (s *Service)deliverOne(ctx context.Context,id string,send func(context.Context,string,string)error)error{
 // Read immutable routing, then acquire room and event locks in shared order.
 var room string;if err:=s.DB.QueryRowContext(ctx,"SELECT b.room_id FROM m1_outbox o JOIN m1_bookings b ON b.id=o.booking_id WHERE event_id=?",id).Scan(&room);err!=nil{return err}
 tx,err:=s.tx(ctx);if err!=nil{return err};defer tx.Rollback();if err=WithRoomLock(ctx,tx,room);err!=nil{return err}
 var user,booking,kind,status string;var attempts int;var next sql.NullTime
 err=tx.QueryRowContext(ctx,"SELECT user_id,booking_id,kind,status,attempts,next_attempt FROM m1_outbox WHERE event_id=? FOR UPDATE",id).Scan(&user,&booking,&kind,&status,&attempts,&next);if err!=nil{return err};now:=s.Now().UTC();if status!="PENDING"||(next.Valid&&next.Time.After(now)){return nil}
 var state string;if err=tx.QueryRowContext(ctx,"SELECT status FROM m1_bookings WHERE id=?",booking).Scan(&state);err!=nil{return err}
 if (kind=="PRE_REMINDER"||kind=="LATE_REMINDER")&&state!="RESERVED"{_,err=tx.ExecContext(ctx,"UPDATE m1_outbox SET status='SUPPRESSED' WHERE event_id=?",id)}else if e:=send(ctx,user,kind+" / "+booking);e!=nil{
  status="PENDING";if attempts+1>=5{status="FAILED"};_,err=tx.ExecContext(ctx,"UPDATE m1_outbox SET attempts=attempts+1,next_attempt=?,status=? WHERE event_id=?",now.Add(time.Minute*time.Duration(1<<attempts)),status,id)
 }else{_,err=tx.ExecContext(ctx,"UPDATE m1_outbox SET status='DELIVERED',delivered_at=?,attempts=attempts+1 WHERE event_id=?",now,id)}
 if err!=nil{return err};return tx.Commit()
}
