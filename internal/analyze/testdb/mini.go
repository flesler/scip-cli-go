package testdb

import (
	"database/sql"
	"fmt"
)

func ImportSymbol(b *Builder, intoPath string, symID int) {
	_, chunkID := b.ensureFile(intoPath)
	_, _ = b.conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 2)",
		chunkID, symID)
}

// MiniCodebase builds the graph used by Python mini_codebase_db().
func MiniCodebase() (*sql.DB, error) {
	b, err := New()
	if err != nil {
		return nil, err
	}
	foo := b.Define("src/lib.ts", "foo", 0, 10)
	b.Define("src/lib.ts", "Orphan", 0, 10)
	sameFile := b.Define("src/lib.ts", "sameFileHelper", 0, 10)
	b.Reference("src/lib.ts", sameFile)
	bar := b.Define("src/lib.ts", "Bar", 0, 10)
	unused := b.Define("src/dead.ts", "deadFn", 0, 10)
	testOnly := b.Define("src/lib.ts", "testOnlyFn", 0, 10)
	moduleUsed := b.Define("src/lib.ts", "moduleUsed", 0, 10)
	b.Reference("src/lib.ts", moduleUsed)
	b.Define("src/consumer.ts", "message", 0, 10)
	b.Reference("src/consumer.ts", foo)
	b.Reference("src/consumer.ts", bar)
	ImportSymbol(b, "src/consumer.ts", foo)
	ImportSymbol(b, "src/importer.ts", unused)
	b.AddFile("tests/harness.ts")
	b.Reference("tests/harness.ts", testOnly)
	b.Reference("tests/harness.ts", moduleUsed)

	symX := b.Define("src/cycle/a.ts", "alpha", 0, 10)
	symY := b.Define("src/cycle/b.ts", "beta", 0, 10)
	b.Reference("src/cycle/a.ts", symY)
	b.Reference("src/cycle/b.ts", symX)

	stale := b.Define("src/types.ts", "StaleType", 0, 10)
	b.Reference("src/only.ts", stale)

	return b.Finish(), nil
}

func SymbolID(db *sql.DB, displayName string) (int, error) {
	var id int
	err := db.QueryRow("SELECT id FROM global_symbols WHERE display_name = ?", displayName).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("symbol %q: %w", displayName, err)
	}
	return id, nil
}
