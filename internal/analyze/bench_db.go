package analyze

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/analyze/testdb"
)

// ScaledBenchDB builds a deterministic :memory: DB for query benchmarks.
func ScaledBenchDB(seed int64) (*sql.DB, error) {
	rng := rand.New(rand.NewSource(seed))
	b, err := testdb.New()
	if err != nil {
		return nil, err
	}
	conn := b.Finish()

	indexes := `
		CREATE INDEX IF NOT EXISTS idx_documents_path ON documents(relative_path);
		CREATE INDEX IF NOT EXISTS idx_mentions_symbol ON mentions(symbol_id, role);
		CREATE INDEX IF NOT EXISTS idx_mentions_chunk ON mentions(chunk_id);
		CREATE INDEX IF NOT EXISTS idx_der_symbol ON defn_enclosing_ranges(symbol_id);
		CREATE INDEX IF NOT EXISTS idx_der_document ON defn_enclosing_ranges(document_id);
		CREATE INDEX IF NOT EXISTS idx_chunks_document ON chunks(document_id);
	`
	if _, err := conn.Exec(indexes); err != nil {
		return nil, err
	}

	modules := make([]string, 20)
	for i := range modules {
		modules[i] = fmt.Sprintf("src/module%02d", i)
	}

	var allFiles []string
	for _, module := range modules {
		for j := 0; j < 50; j++ {
			allFiles = append(allFiles, fmt.Sprintf("%s/file%03d.ts", module, j))
		}
	}

	fileToDoc := make(map[string]int)
	docID := 1
	chunkID := 1
	for _, filePath := range allFiles {
		if _, err := conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", docID, filePath); err != nil {
			return nil, err
		}
		if _, err := conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 200)", chunkID, docID); err != nil {
			return nil, err
		}
		fileToDoc[filePath] = docID
		docID++
		chunkID++
	}

	symID := 1
	derID := 1
	fileSymbols := make(map[string][]int)

	insertSymbol := func(filePath, symbol, display string, start, end int) int {
		doc := fileToDoc[filePath]
		chk := doc
		id := symID
		symID++
		if _, err := conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)", id, symbol, display); err != nil {
			panic(err)
		}
		if _, err := conn.Exec(`
			INSERT INTO defn_enclosing_ranges
			(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
			VALUES (?, ?, ?, ?, 0, ?, 0)`,
			derID, doc, id, start, end); err != nil {
			panic(err)
		}
		derID++
		if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)", chk, id); err != nil {
			panic(err)
		}
		fileSymbols[filePath] = append(fileSymbols[filePath], id)
		return id
	}

	for _, filePath := range allFiles {
		fileLabel := filePath[strings.LastIndex(filePath, "/")+1:]
		for i := 0; i < 10; i++ {
			symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/func%d().", filePath, fileLabel, i)
			insertSymbol(filePath, symbol, fmt.Sprintf("func%d", i), i*20, (i+1)*20-1)
		}
		for i := 0; i < 3; i++ {
			className := fmt.Sprintf("Class%d", i)
			classSymbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#", filePath, fileLabel, className)
			insertSymbol(filePath, classSymbol, className, 200+i*30, 200+(i+1)*30-1)
			for j := 0; j < 2; j++ {
				methodSymbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/%s#method%d().", filePath, fileLabel, className, j)
				insertSymbol(filePath, methodSymbol, fmt.Sprintf("method%d", j), 200+i*30+j*10, 200+i*30+(j+1)*10-1)
			}
		}
		for i := 0; i < 2; i++ {
			typeSymbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/Type%d#", filePath, fileLabel, i)
			insertSymbol(filePath, typeSymbol, fmt.Sprintf("Type%d", i), 300+i*10, 300+(i+1)*10-1)
		}
	}

	hubSymIDs := make([]int, 5)
	for i := 0; i < 5; i++ {
		hubFile := allFiles[i]
		fileLabel := hubFile[strings.LastIndex(hubFile, "/")+1:]
		hubSymbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/hubFunc%d().", hubFile, fileLabel, i)
		hubSymIDs[i] = insertSymbol(hubFile, hubSymbol, fmt.Sprintf("hubFunc%d", i), 0, 10)
	}

	for _, filePath := range allFiles {
		chk := fileToDoc[filePath]
		numRefs := 5 + rng.Intn(11)
		if rng.Float64() < 0.3 && len(hubSymIDs) > 0 {
			hub := hubSymIDs[rng.Intn(len(hubSymIDs))]
			if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 0)", chk, hub); err != nil {
				return nil, err
			}
		}
		for n := 0; n < numRefs; n++ {
			refFile := allFiles[rng.Intn(len(allFiles))]
			if refFile == filePath || len(fileSymbols[refFile]) == 0 {
				continue
			}
			refSym := fileSymbols[refFile][rng.Intn(len(fileSymbols[refFile]))]
			if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 0)", chk, refSym); err != nil {
				return nil, err
			}
		}
	}

	numCycles := 5 + rng.Intn(6)
	for c := 0; c < numCycles; c++ {
		cycleSize := 2 + rng.Intn(3)
		cycleFiles := pickSample(rng, allFiles, cycleSize)
		for i, fromFile := range cycleFiles {
			toFile := cycleFiles[(i+1)%len(cycleFiles)]
			if len(fileSymbols[toFile]) == 0 {
				continue
			}
			toSym := fileSymbols[toFile][0]
			fromChunk := fileToDoc[fromFile]
			if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 0)", fromChunk, toSym); err != nil {
				return nil, err
			}
		}
	}

	maxDoc, _ := scalarInt(conn, "SELECT COALESCE(MAX(id), 0) FROM documents")
	maxChunk, _ := scalarInt(conn, "SELECT COALESCE(MAX(id), 0) FROM chunks")
	maxSym, _ := scalarInt(conn, "SELECT COALESCE(MAX(id), 0) FROM global_symbols")
	maxDER, _ := scalarInt(conn, "SELECT COALESCE(MAX(id), 0) FROM defn_enclosing_ranges")
	docID = maxDoc + 1
	chunkID = maxChunk + 1
	symID = maxSym + 1
	derID = maxDER + 1

	for i := 0; i < 120; i++ {
		path := fmt.Sprintf("tests/noise/dead%03d.ts", i)
		if err := insertNoiseDead(conn, &docID, &chunkID, &symID, &derID, path, i); err != nil {
			return nil, err
		}
	}
	for i := 0; i < 80; i++ {
		path := fmt.Sprintf("tests/module_only/mod%03d.ts", i)
		if err := insertModuleOnly(conn, &docID, &chunkID, &symID, &derID, path, i); err != nil {
			return nil, err
		}
	}

	testConsumerPath := "tests/unit/bench_consumer.ts"
	if _, err := conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", docID, testConsumerPath); err != nil {
		return nil, err
	}
	if _, err := conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 50)", chunkID, docID); err != nil {
		return nil, err
	}
	testConsumerChunk := chunkID
	docID++
	chunkID++

	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("src/module00/a_testonly%03d.ts", i)
		if _, err := conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", docID, path); err != nil {
			return nil, err
		}
		if _, err := conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 20)", chunkID, docID); err != nil {
			return nil, err
		}
		fileLabel := fmt.Sprintf("a_testonly%03d.ts", i)
		symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`%s`/benchOnly%d().", path, fileLabel, i)
		if _, err := conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)", symID, symbol, fmt.Sprintf("benchOnly%d", i)); err != nil {
			return nil, err
		}
		if _, err := conn.Exec(`
			INSERT INTO defn_enclosing_ranges
			(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
			VALUES (?, ?, ?, 0, 0, 10, 0)`,
			derID, docID, symID); err != nil {
			return nil, err
		}
		if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)", chunkID, symID); err != nil {
			return nil, err
		}
		if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 0)", testConsumerChunk, symID); err != nil {
			return nil, err
		}
		docID++
		chunkID++
		symID++
		derID++
	}

	return conn, nil
}

func scalarInt(db *sql.DB, query string) (int, error) {
	var v int
	err := db.QueryRow(query).Scan(&v)
	return v, err
}

func pickSample(rng *rand.Rand, items []string, n int) []string {
	if n >= len(items) {
		return append([]string(nil), items...)
	}
	idx := rng.Perm(len(items))[:n]
	out := make([]string, n)
	for i, j := range idx {
		out[i] = items[j]
	}
	return out
}

func insertNoiseDead(conn *sql.DB, docID, chunkID, symID, derID *int, path string, i int) error {
	if _, err := conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", *docID, path); err != nil {
		return err
	}
	if _, err := conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 600)", *chunkID, *docID); err != nil {
		return err
	}
	symbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`dead%03d.ts`/noiseFunc().", path, i)
	if _, err := conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)", *symID, symbol, "noiseFunc"); err != nil {
		return err
	}
	if _, err := conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, 0, 0, 500, 0)`,
		*derID, *docID, *symID); err != nil {
		return err
	}
	if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)", *chunkID, *symID); err != nil {
		return err
	}
	*docID++
	*chunkID++
	*symID++
	*derID++
	return nil
}

func insertModuleOnly(conn *sql.DB, docID, chunkID, symID, derID *int, path string, i int) error {
	if _, err := conn.Exec("INSERT INTO documents (id, relative_path) VALUES (?, ?)", *docID, path); err != nil {
		return err
	}
	if _, err := conn.Exec("INSERT INTO chunks (id, document_id, start_line, end_line) VALUES (?, ?, 0, 10)", *chunkID, *docID); err != nil {
		return err
	}
	moduleSymbol := fmt.Sprintf("scip-typescript npm test 1.0 %s/`mod%03d.ts`/", path, i)
	if _, err := conn.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)", *symID, moduleSymbol, "mod"); err != nil {
		return err
	}
	if _, err := conn.Exec(`
		INSERT INTO defn_enclosing_ranges
		(id, document_id, symbol_id, start_line, start_char, end_line, end_char)
		VALUES (?, ?, ?, 0, 0, 1, 0)`,
		*derID, *docID, *symID); err != nil {
		return err
	}
	if _, err := conn.Exec("INSERT OR IGNORE INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, 1)", *chunkID, *symID); err != nil {
		return err
	}
	*docID++
	*chunkID++
	*symID++
	*derID++
	return nil
}
