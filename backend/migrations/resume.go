package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var alterTable = regexp.MustCompile(`(?is)^\s*ALTER TABLE ([a-z][a-z0-9_]*)\s+`)
var addedColumn = regexp.MustCompile(`(?i)ADD COLUMN ([a-z][a-z0-9_]*)\s+`)
var addedIndex = regexp.MustCompile(`(?i)ADD INDEX ([a-z][a-z0-9_]*)\s*\(`)

// resumeDDL checks an interrupted ADD against MySQL's canonical definition.
// It only changes a connection-local temporary copy. A mismatched definition
// is an error, never evidence that a migration has already succeeded.
func resumeDDL(ctx context.Context, conn *sql.Conn, statement string, original error) error {
	var mysqlErr *mysql.MySQLError
	if !errors.As(original, &mysqlErr) || (mysqlErr.Number != 1060 && mysqlErr.Number != 1061) {
		return original
	}
	table := alterTable.FindStringSubmatch(statement)
	columns, indexes := addedColumn.FindAllStringSubmatch(statement, -1), addedIndex.FindAllStringSubmatch(statement, -1)
	if len(table) != 2 || len(columns)+len(indexes) == 0 {
		return original
	}
	const scratch = "campus_migration_resume"
	if _, err := conn.ExecContext(ctx, "CREATE TEMPORARY TABLE `"+scratch+"` LIKE `"+table[1]+"`"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "DROP TEMPORARY TABLE IF EXISTS `"+scratch+"`")
	for _, index := range indexes {
		_, err := conn.ExecContext(ctx, "ALTER TABLE `"+scratch+"` DROP INDEX `"+index[1]+"`")
		var m *mysql.MySQLError
		if err != nil && !(errors.As(err, &m) && m.Number == 1091) {
			return err
		}
	}
	for _, column := range columns {
		_, err := conn.ExecContext(ctx, "ALTER TABLE `"+scratch+"` DROP COLUMN `"+column[1]+"`")
		var m *mysql.MySQLError
		if err != nil && !(errors.As(err, &m) && m.Number == 1091) {
			return err
		}
	}
	reconstructed := alterTable.ReplaceAllString(statement, "ALTER TABLE `"+scratch+"` ")
	if _, err := conn.ExecContext(ctx, reconstructed); err != nil {
		return err
	}
	definition := func(name string) (string, error) {
		var tableName, ddl string
		err := conn.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&tableName, &ddl)
		if err != nil {
			return "", err
		}
		start := strings.Index(ddl, "(")
		if start < 0 {
			return "", fmt.Errorf("invalid table definition")
		}
		return ddl[start:], nil
	}
	actual, err := definition(table[1])
	if err != nil {
		return err
	}
	expected, err := definition(scratch)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("interrupted migration schema mismatch for %s: %w", table[1], original)
	}
	return nil
}
