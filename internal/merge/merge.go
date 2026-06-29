package merge

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/sourcegraph/scip-cli-go/internal/symbols"
)

func MergeSQLiteIndexes(partPaths []string, outputPath string) error {
	if len(partPaths) == 0 {
		return fmt.Errorf("at least one input is required")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := copyFile(partPaths[0], outputPath); err != nil {
		return fmt.Errorf("copying first part: %w", err)
	}

	db, err := sql.Open("sqlite", outputPath)
	if err != nil {
		return err
	}
	defer db.Close()

	for _, partPath := range partPaths[1:] {
		if err := mergeOneDatabase(db, partPath); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func mergeOneDatabase(dest *sql.DB, partPath string) error {
	if _, err := dest.Exec("ATTACH DATABASE ? AS src", partPath); err != nil {
		return err
	}
	defer dest.Exec("DETACH DATABASE src")

	tx, err := dest.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("CREATE TEMPORARY TABLE doc_map (old_id INTEGER, new_id INTEGER)"); err != nil {
		return err
	}
	if _, err := tx.Exec("CREATE TEMPORARY TABLE symbol_map (old_id INTEGER, new_id INTEGER)"); err != nil {
		return err
	}
	if _, err := tx.Exec("CREATE TEMPORARY TABLE chunk_map (old_id INTEGER, new_id INTEGER)"); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO documents (relative_path)
		SELECT relative_path FROM src.documents
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO doc_map (old_id, new_id)
		SELECT src.id, dest.id
		FROM src.documents src
		JOIN documents dest ON dest.relative_path = src.relative_path
	`); err != nil {
		return err
	}

	exclude := symbols.SQLExcludeVariableSymbols("symbol")
	if _, err := tx.Exec(fmt.Sprintf(`
		INSERT OR IGNORE INTO global_symbols (symbol, display_name, kind)
		SELECT symbol, display_name, kind
		FROM src.global_symbols
		WHERE %s
	`, exclude)); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO symbol_map (old_id, new_id)
		SELECT src.id, dest.id
		FROM src.global_symbols src
		JOIN global_symbols dest ON dest.symbol = src.symbol
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO chunks (document_id, chunk_index, start_line, end_line, occurrences)
		SELECT dm.new_id, src.chunk_index, src.start_line, src.end_line, src.occurrences
		FROM src.chunks src
		JOIN doc_map dm ON dm.old_id = src.document_id
		WHERE NOT EXISTS (
			SELECT 1 FROM chunks
			WHERE document_id = dm.new_id
			AND chunk_index = src.chunk_index
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO chunk_map (old_id, new_id)
		SELECT src.id, dest.id
		FROM src.chunks src
		JOIN doc_map dm ON dm.old_id = src.document_id
		JOIN chunks dest ON dest.document_id = dm.new_id AND dest.chunk_index = src.chunk_index
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role)
		SELECT cm.new_id, sm.new_id, src.role
		FROM src.mentions src
		JOIN chunk_map cm ON cm.old_id = src.chunk_id
		JOIN symbol_map sm ON sm.old_id = src.symbol_id
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO defn_enclosing_ranges (
			document_id, symbol_id, start_line, start_char, end_line, end_char
		)
		SELECT dm.new_id, sm.new_id, src.start_line, src.start_char, src.end_line, src.end_char
		FROM src.defn_enclosing_ranges src
		JOIN doc_map dm ON dm.old_id = src.document_id
		JOIN symbol_map sm ON sm.old_id = src.symbol_id
	`); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	tx2, err := dest.Begin()
	if err != nil {
		return err
	}
	defer tx2.Rollback()

	for _, stmt := range []string{
		"DROP TABLE doc_map",
		"DROP TABLE symbol_map",
		"DROP TABLE chunk_map",
	} {
		if _, err := tx2.Exec(stmt); err != nil {
			return err
		}
	}

	return tx2.Commit()
}
