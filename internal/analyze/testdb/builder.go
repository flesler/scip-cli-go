package testdb

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

const analyzeSchema = `
CREATE TABLE documents (
    id INTEGER PRIMARY KEY,
    relative_path TEXT NOT NULL UNIQUE
);
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    document_id INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL
);
CREATE TABLE global_symbols (
    id INTEGER PRIMARY KEY,
    symbol TEXT NOT NULL UNIQUE,
    display_name TEXT,
    kind INTEGER
);
CREATE TABLE mentions (
    chunk_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    role INTEGER NOT NULL,
    PRIMARY KEY (chunk_id, symbol_id, role)
);
CREATE TABLE defn_enclosing_ranges (
    id INTEGER PRIMARY KEY,
    document_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    start_char INTEGER NOT NULL DEFAULT 0,
    end_line INTEGER NOT NULL,
    end_char INTEGER NOT NULL DEFAULT 0
);
`

type Builder struct {
	conn    *sql.DB
	nextDoc int
	nextChk int
	nextSym int
	nextDER int
}

func New() (*Builder, error) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(analyzeSchema); err != nil {
		conn.Close()
		return nil, err
	}
	return &Builder{conn: conn, nextDoc: 1, nextChk: 1, nextSym: 1, nextDER: 1}, nil
}

func (b *Builder) Finish() *sql.DB {
	return b.conn
}

func (b *Builder) AddFile(path string) (int, int) {
	docID := b.nextDoc
	b.nextDoc++
	chunkID := b.nextChk
	b.nextChk++
	_, _ = b.conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", docID, path)
	_, _ = b.conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 200)",
		chunkID, docID)
	return docID, chunkID
}

func (b *Builder) Define(path, name string, start, end int) int {
	docID, chunkID := b.ensureFile(path)
	symID := b.nextSym
	b.nextSym++
	fileLabel := path[strings.LastIndex(path, "/")+1:]
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s().", path, fileLabel, name)
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' && !strings.HasSuffix(name, "()") {
		symbol = fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#", path, fileLabel, name)
	}
	_, _ = b.conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)",
		symID, symbol, name)
	derID := b.nextDER
	b.nextDER++
	_, _ = b.conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, ?, 0, ?, 0)`,
		derID, docID, symID, start, end)
	_, _ = b.conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)",
		chunkID, symID)
	return symID
}

func (b *Builder) DefineType(path, name string, start, end int) int {
	docID, chunkID := b.ensureFile(path)
	symID := b.nextSym
	b.nextSym++
	fileLabel := path[strings.LastIndex(path, "/")+1:]
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#", path, fileLabel, name)
	_, _ = b.conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)",
		symID, symbol, name)
	derID := b.nextDER
	b.nextDER++
	_, _ = b.conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, ?, 0, ?, 0)`,
		derID, docID, symID, start, end)
	_, _ = b.conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)",
		chunkID, symID)
	return symID
}

func (b *Builder) DefineModule(path string) int {
	docID, _ := b.ensureFile(path)
	symID := b.nextSym
	b.nextSym++
	fileLabel := path[strings.LastIndex(path, "/")+1:]
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/", path, fileLabel)
	_, _ = b.conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)",
		symID, symbol, fileLabel)
	derID := b.nextDER
	b.nextDER++
	_, _ = b.conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, 0, 0, 200, 0)`,
		derID, docID, symID)
	return symID
}

func (b *Builder) Reference(fromPath string, toSymID int) {
	_, chunkID := b.ensureFile(fromPath)
	_, _ = b.conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 0)",
		chunkID, toSymID)
}

func (b *Builder) Method(path, className, methodName string, start, end int) int {
	docID, chunkID := b.ensureFile(path)
	symID := b.nextSym
	b.nextSym++
	fileLabel := path[strings.LastIndex(path, "/")+1:]
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#%s().", path, fileLabel, className, methodName)
	_, _ = b.conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)",
		symID, symbol, methodName)
	derID := b.nextDER
	b.nextDER++
	_, _ = b.conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, ?, 0, ?, 0)`,
		derID, docID, symID, start, end)
	_, _ = b.conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)",
		chunkID, symID)
	return symID
}

func (b *Builder) TypeLiteralField(path, parentName, fieldName string, literalIndex int) int {
	_, _ = b.ensureFile(path)
	symID := b.nextSym
	b.nextSym++
	fileLabel := path[strings.LastIndex(path, "/")+1:]
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#typeLiteral%d:%s.",
		path, fileLabel, parentName, literalIndex, fieldName)
	_, _ = b.conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)",
		symID, symbol, fieldName)
	return symID
}

func (b *Builder) ensureFile(path string) (int, int) {
	var docID int
	err := b.conn.QueryRow("SELECT id FROM documents WHERE relative_path = ?", path).Scan(&docID)
	if err == nil {
		var chunkID int
		_ = b.conn.QueryRow("SELECT id FROM chunks WHERE document_id = ? LIMIT 1", docID).Scan(&chunkID)
		return docID, chunkID
	}
	return b.AddFile(path)
}
