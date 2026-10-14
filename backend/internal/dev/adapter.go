package dev
import("context";"database/sql";"encoding/json";"errors";"os";"campus-resource-platform/backend/internal/m1")
type Fixture struct{Tokens map[string]m1.Actor `json:"tokens"`;RoomsData []m1.Room `json:"rooms"`;Emails map[string]string `json:"emails"`}
func Load(path string)(*Fixture,error){raw,e:=os.ReadFile(path);if e!=nil{return nil,e};var f Fixture;e=json.Unmarshal(raw,&f);if e!=nil{return nil,e};if len(f.Tokens)==0||len(f.RoomsData)==0{return nil,errors.New("empty dev fixture")};return &f,nil}
func(f *Fixture)Rooms(context.Context,*sql.Tx)([]m1.Room,error){return f.RoomsData,nil}
func(f *Fixture)Auth(_ context.Context,t string)(m1.Actor,error){a,ok:=f.Tokens[t];if !ok{return m1.Actor{},errors.New("unauthenticated")};return a,nil}
