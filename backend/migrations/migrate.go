package migrations

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "embed"
 "fmt"
 "strings"
)

//go:embed 001_m1.sql
var files embed.FS

func Apply(ctx context.Context, db *sql.DB) error {
 conn, err := db.Conn(ctx); if err != nil { return err }; defer conn.Close()
 var locked int
 if err = conn.QueryRowContext(ctx,"SELECT GET_LOCK('campus_m1_migrate',30)").Scan(&locked); err != nil { return err }
 if locked != 1 { return fmt.Errorf("migration lock unavailable") }
 defer conn.ExecContext(context.Background(),"SELECT RELEASE_LOCK('campus_m1_migrate')")
 _,err=conn.ExecContext(ctx,"CREATE TABLE IF NOT EXISTS m1_schema_migrations (version VARCHAR(64) PRIMARY KEY, checksum CHAR(64) NOT NULL)");if err!=nil{return err}
 raw,_:=files.ReadFile("001_m1.sql");sum:=fmt.Sprintf("%x",sha256.Sum256(raw));var old string
 err=conn.QueryRowContext(ctx,"SELECT checksum FROM m1_schema_migrations WHERE version='001'").Scan(&old)
 if err==nil {if old!=sum{return fmt.Errorf("migration checksum changed")};return nil};if err!=sql.ErrNoRows{return err}
 for _,statement:=range strings.Split(string(raw),";"){if strings.TrimSpace(statement)==""{continue};if _,err=conn.ExecContext(ctx,statement);err!=nil{return err}}
 _,err=conn.ExecContext(ctx,"INSERT INTO m1_schema_migrations VALUES ('001',?)",sum);return err
}
