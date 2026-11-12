package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

func TestInterruptedMigrationResumesAndRejectsMismatch(t *testing.T) {
	dsn := os.Getenv("MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("MIGRATION_TEST_DSN not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "campus_migration_check" {
		t.Fatal("isolated campus_migration_check required")
	}
	ctx := context.Background()
	clean := func() {
		rows, err := db.Query("SHOW TABLES")
		if err != nil {
			t.Fatal(err)
		}
		var tables []string
		for rows.Next() {
			var table string
			if err = rows.Scan(&table); err != nil {
				t.Fatal(err)
			}
			tables = append(tables, table)
		}
		rows.Close()
		for _, table := range tables {
			if _, err = db.Exec("DROP TABLE `" + table + "`"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Simulate death after each DDL-containing migration, before recording version.
	for _, version := range []string{"003", "004", "007", "010"} {
		t.Run(version, func(t *testing.T) {
			clean()
			if err := Apply(ctx, db); err != nil {
				t.Fatal(err)
			}
			// Existing versions above the failed version stay, emulating a lost final marker.
			if _, err = db.Exec("DELETE FROM m1_schema_migrations WHERE version=?", version); err != nil {
				t.Fatal(err)
			}
			if err := Apply(ctx, db); err != nil {
				t.Fatalf("resume %s: %v", version, err)
			}
			if err := Apply(ctx, db); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("partial007", func(t *testing.T) {
		clean()
		entries, _ := files.ReadDir(".")
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			raw, _ := files.ReadFile(entry.Name())
			version := strings.SplitN(entry.Name(), "_", 2)[0]
			if version > "007" {
				break
			}
			for _, statement := range strings.Split(string(raw), ";") {
				if strings.TrimSpace(statement) == "" {
					continue
				}
				if _, err = db.Exec(statement); err != nil {
					t.Fatal(err)
				}
				if version == "007" {
					break
				}
			}
		}
		if err := Apply(ctx, db); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		if _, err = db.Exec("DELETE FROM m1_schema_migrations WHERE version='010'"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("ALTER TABLE m3_outbox MODIFY COLUMN lease_token VARCHAR(8) COLLATE utf8mb4_bin NOT NULL DEFAULT ''"); err != nil {
			t.Fatal(err)
		}
		if err := Apply(ctx, db); err == nil || !strings.Contains(err.Error(), "schema mismatch") {
			t.Fatalf("mismatch accepted: %v", err)
		}
		var n int
		db.QueryRow("SELECT COUNT(*) FROM m1_schema_migrations WHERE version='010'").Scan(&n)
		if n != 0 {
			t.Fatal("mismatched migration marked complete")
		}
	})
}
