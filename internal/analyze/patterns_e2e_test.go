package analyze

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/internal/cache"
	"github.com/flesler/scip-cli-go/internal/indexing"
	"github.com/flesler/scip-cli-go/internal/output"
	"github.com/flesler/scip-cli-go/internal/queries"

	_ "modernc.org/sqlite"
)

const (
	lazyPanelFile        = "src/ui/LazyPanel.ts"
	hookAFile            = "src/hooks/useHookA.ts"
	hookBFile            = "src/hooks/useHookB.ts"
	typeAFile            = "src/types/a.ts"
	typeBFile            = "src/types/b.ts"
	i18nEnFile           = "src/domain/i18n/en.ts"
	i18nIndexFile        = "src/domain/i18n/index.ts"
	fnLazyPanel          = "LazyPanel"
	fnEvictItem          = "evictItem"
	fnSend               = "send"
	classInferenceClient = "InferenceClient"
	fnOrphanWidget       = "OrphanWidget"
	typeBaseStream       = "BaseStream"
	typeLabelFunc        = "LabelFunc"
	typeButtonProps      = "ButtonProps"
	typeOpts             = "Opts"
)

var (
	patternFixtureRoot string
	patternDBPath      string
	patternIndexOK     bool
)

func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	fixtureSrc := filepath.Join(repoRoot, "testdata", "fixtures", "sample-project")
	if _, err := os.Stat(fixtureSrc); err != nil {
		os.Exit(1)
	}
	tmp, err := os.MkdirTemp("", "scip-analyze-patterns-*")
	if err != nil {
		os.Exit(1)
	}
	patternFixtureRoot = tmp
	if err := copyFixtureTree(fixtureSrc, patternFixtureRoot); err != nil {
		os.Exit(1)
	}
	cacheDir := filepath.Join(tmp, "cache")
	os.Setenv("SCIP_CLI_CACHE", cacheDir)
	if err := indexing.Reindex(patternFixtureRoot, true); err == nil {
		patternDBPath = cache.FindDB(patternFixtureRoot)
		patternIndexOK = patternDBPath != ""
	}
	os.Exit(m.Run())
}

func copyFixtureTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func openPatternDB(t *testing.T) *sql.DB {
	t.Helper()
	if !patternIndexOK {
		t.Skip("fixture indexing unavailable")
	}
	db, err := sql.Open("sqlite", patternDBPath)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func linesContain(lines []string, name string) bool {
	for _, line := range lines {
		if strings.Contains(line, name) {
			return true
		}
	}
	return false
}

func TestLazyDefaultExportNotDead(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	dead, err := deadExports(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(dead, fnLazyPanel) {
		t.Fatalf("LazyPanel should not be dead: %v", dead)
	}
	inFile, err := deadInFile(db, lazyPanelFile, 20)
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(inFile, fnLazyPanel) {
		t.Fatalf("LazyPanel should not be dead in file: %v", inFile)
	}
}

func TestObjectAliasExportNotDead(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	dead, err := deadExports(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(dead, fnEvictItem) {
		t.Fatalf("evictItem should not be dead: %v", dead)
	}
}

func TestDefaultObjectExportNotDead(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	dead, err := deadExports(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	same, err := sameFileOnly(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(dead, fnSend) || linesContain(same, fnSend) {
		t.Fatalf("send should not be flagged: dead=%v same=%v", dead, same)
	}
}

func TestTrulyDeadExportFlagged(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	dead, err := deadExports(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContain(dead, fnOrphanWidget) {
		t.Fatalf("OrphanWidget should be dead: %v", dead)
	}
}

func TestStaleTypeSameFileExtendsNotListed(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	stale, err := staleTypes(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(stale, typeBaseStream) {
		t.Fatalf("BaseStream should not be stale: %v", stale)
	}
}

func TestTypeOnlyCycleIgnored(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	cyc, err := cycles(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range cyc {
		if strings.Contains(line, typeAFile) && strings.Contains(line, typeBFile) {
			t.Fatalf("type-only cycle should be ignored: %q", line)
		}
	}
}

func TestBarrelModuleCycleIgnored(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	cyc, err := cycles(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range cyc {
		if strings.Contains(line, i18nEnFile) && strings.Contains(line, i18nIndexFile) {
			t.Fatalf("barrel module cycle should be ignored: %q", line)
		}
	}
}

func TestOptsInHooksNotStale(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	staleA, err := staleTypes(db, 50, CheckOptions{Scope: hookAFile})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(staleA, typeOpts) {
		t.Fatalf("Opts should not be stale in hook A: %v", staleA)
	}
	staleB, err := staleTypes(db, 50, CheckOptions{Scope: hookBFile})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(staleB, typeOpts) {
		t.Fatalf("Opts should not be stale in hook B: %v", staleB)
	}
}

func TestComponentPropsNotStale(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	stale, err := staleTypes(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(stale, typeButtonProps) {
		t.Fatalf("ButtonProps should not be stale: %v", stale)
	}
}

func TestDefaultClassInstanceNotDead(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	dead, err := deadExports(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	unref, err := unreferencedSymbols(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := staleTypes(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(dead, classInferenceClient) || linesContain(unref, classInferenceClient) || linesContain(stale, classInferenceClient) {
		t.Fatalf("InferenceClient should not be flagged")
	}
}

func TestStaleTypeUnionSameFileNotListed(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	stale, err := staleTypes(db, 50, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContain(stale, "LabelFunc") {
		t.Fatalf("LabelFunc should not be stale: %v", stale)
	}
}

func TestParsePrioritiesAliases(t *testing.T) {
	p, err := ParsePriorities("1,h,high,2,m,low,l,3")
	if err != nil {
		t.Fatal(err)
	}
	if !p[PriorityHigh] || !p[PriorityMedium] || !p[PriorityLow] {
		t.Fatalf("expected all priorities: %v", p)
	}
}

func TestParsePrioritiesRejectsUnknown(t *testing.T) {
	if _, err := ParsePriorities("high,foo"); err == nil {
		t.Fatal("expected error for unknown priority")
	}
}

func TestModuleSymbolShowsModuleLabel(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	var sym string
	err := db.QueryRow(`
		SELECT gs.symbol FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON der.symbol_id = gs.id
		JOIN documents d ON der.document_id = d.id
		WHERE d.relative_path = 'src/ui/menuModule.ts' AND gs.symbol LIKE '%/'
		LIMIT 1`).Scan(&sym)
	if err != nil {
		t.Fatal(err)
	}
	if ShortName(sym) != "(module)" {
		t.Fatalf("short_name(%q)=%q", sym, ShortName(sym))
	}
}

func TestOptsAmbiguousRefsWarns(t *testing.T) {
	db := openPatternDB(t)
	defer db.Close()
	syms, err := queries.ResolveSymbol(db, typeOpts, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) < 2 {
		t.Fatalf("expected ambiguous %s, got %d matches", typeOpts, len(syms))
	}
	stderr := capturePatternStderr(func() {
		output.WarnAmbiguousRefs(typeOpts, syms, db)
	})
	if !strings.Contains(stderr, "Ambiguous symbol") || !strings.Contains(stderr, "ext_refs=") {
		t.Fatalf("stderr=%q", stderr)
	}
	if !strings.Contains(stderr, hookAFile) && !strings.Contains(stderr, hookBFile) {
		t.Fatalf("expected hook paths in stderr: %q", stderr)
	}
}

func capturePatternStderr(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String()
}
