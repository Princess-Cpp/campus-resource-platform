package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"strings"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked int
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK('campus_m1_migrate',30)").Scan(&locked); err != nil {
		return err
	}
	if locked != 1 {
		return fmt.Errorf("migration lock unavailable")
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('campus_m1_migrate')")
	_, err = conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS m1_schema_migrations (version VARCHAR(64) PRIMARY KEY, checksum CHAR(64) NOT NULL)")
	if err != nil {
		return err
	}
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := strings.SplitN(entry.Name(), "_", 2)[0]
		raw, e := files.ReadFile(entry.Name())
		if e != nil {
			return e
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(raw))
		var old string
		e = conn.QueryRowContext(ctx, "SELECT checksum FROM m1_schema_migrations WHERE version=?", version).Scan(&old)
		if e == nil {
			if old != sum {
				return fmt.Errorf("migration %s checksum changed", version)
			}
			continue
		}
		if e != sql.ErrNoRows {
			return e
		}
		for _, statement := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, e = conn.ExecContext(ctx, statement); e != nil {
				return e
			}
		}
		if _, e = conn.ExecContext(ctx, "INSERT INTO m1_schema_migrations VALUES (?,?)", version, sum); e != nil {
			return e
		}
	}
	return nil
}
