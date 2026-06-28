package sqlhelp

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

var debugLogger *log.Logger

func init() {
	if os.Getenv("SCIP_CLI_DEBUG") != "" {
		debugLogger = log.New(os.Stderr, "", 0)
	}
}

func DebugExecute(db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	if debugLogger != nil {
		truncated := query
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		debugLogger.Printf("SQL: %s | params: %v", strings.TrimSpace(truncated), args)
	}
	return db.Query(query, args...)
}

func DebugExecuteOne(db *sql.DB, query string, args ...interface{}) *sql.Row {
	if debugLogger != nil {
		truncated := query
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		debugLogger.Printf("SQL: %s | params: %v", strings.TrimSpace(truncated), args)
	}
	return db.QueryRow(query, args...)
}

func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func ConfigureReadConnection(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA query_only = ON",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -64000",
		"PRAGMA mmap_size = 268435456",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}
	return nil
}
