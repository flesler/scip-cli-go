package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/scip-cli-go/internal/clierr"
	"github.com/sourcegraph/scip-cli-go/internal/commands"
	"github.com/sourcegraph/scip-cli-go/internal/paths"
	"github.com/sourcegraph/scip-cli-go/internal/project"
	"github.com/sourcegraph/scip-cli-go/internal/symbols"
)

const version = "2.3.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if os.Args[1] == "--version" || os.Args[1] == "-version" {
		exe, _ := os.Executable()
		fmt.Printf("scip-cli-go %s (%s)\n", version, exe)
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "refs":
		err = runRefs(args)
	case "code":
		err = runCode(args)
	case "search":
		err = runSearch(args)
	case "symbols":
		err = runSymbols(args)
	case "rdeps":
		err = runRdeps(args)
	case "deps":
		err = runDeps(args)
	case "members":
		err = runMembers(args)
	case "skill":
		err = runSkill(args)
	case "analyze":
		err = runAnalyze(args)
	case "reindex":
		err = runReindex(args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		clierr.Fatal(err)
	}
}

func pathScope(flagPath string) string {
	if flagPath == "" {
		return ""
	}
	root, ok := project.FindProjectRoot("")
	if !ok {
		return ""
	}
	scope, err := paths.NormalizePathScope(flagPath, root)
	if err != nil {
		clierr.Fatal(err)
	}
	return scope
}

func parseKind(s string) (*symbols.SymbolKind, error) {
	return symbols.ParseFilterableKind(s)
}

func runRefs(argv []string) error {
	fs := newFlagSet("refs")
	limit := fs.Int("limit", 10, "max reference lines")
	pathFlag := fs.String("path", "", "limit to file or directory")
	pathsOnly := fs.Bool("paths-only", false, "print unique file paths only")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	syms := fs.Args()
	if len(syms) == 0 {
		return fmt.Errorf("refs requires at least one symbol name")
	}
	return commands.RefsMain(map[string]interface{}{
		"symbol":     syms,
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
		"paths_only": *pathsOnly,
	})
}

func runCode(argv []string) error {
	fs := newFlagSet("code")
	limit := fs.Int("limit", 10, "max matching symbols")
	pathFlag := fs.String("path", "", "limit to file or directory")
	kindStr := fs.String("kind", "", "filter by kind")
	maxLines := fs.Int("max-lines", -1, "max source lines per definition body")
	maxLinesSet := false
	fs.trackInt("max-lines", maxLines, &maxLinesSet)
	full := fs.Bool("full", false, "show full definition")
	offset := fs.Int("offset", 0, "skip first N lines of definition")
	snippet := fs.Bool("snippet", false, "show only first line")
	lineNumbers := fs.Bool("line-numbers", false, "prefix lines with numbers")
	fs.BoolVar(lineNumbers, "n", false, "prefix lines with numbers")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	syms := fs.Args()
	if len(syms) == 0 {
		return fmt.Errorf("code requires at least one symbol name")
	}
	kind, err := parseKind(*kindStr)
	if err != nil {
		return err
	}
	argsMap := map[string]interface{}{
		"symbol":       syms,
		"limit":        *limit,
		"path_scope":   pathScope(*pathFlag),
		"kind":         kind,
		"full":         *full,
		"offset":       *offset,
		"snippet":      *snippet,
		"line_numbers": *lineNumbers,
	}
	if maxLinesSet {
		argsMap["max_lines"] = *maxLines
	}
	return commands.CodeMain(argsMap)
}

func runSearch(argv []string) error {
	fs := newFlagSet("search")
	limit := fs.Int("limit", 10, "max results")
	pathFlag := fs.String("path", "", "limit to file or directory")
	kindStr := fs.String("kind", "", "filter by kind")
	namesOnly := fs.Bool("names-only", false, "print symbol names only")
	pathsOnly := fs.Bool("paths-only", false, "print unique file paths only")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		return fmt.Errorf("search requires at least one pattern")
	}
	kind, err := parseKind(*kindStr)
	if err != nil {
		return err
	}
	return commands.SearchMain(map[string]interface{}{
		"pattern":    patterns,
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
		"kind":       kind,
		"names_only": *namesOnly,
		"paths_only": *pathsOnly,
	})
}

func runSymbols(argv []string) error {
	fs := newFlagSet("symbols")
	limit := fs.Int("limit", 10, "max symbols")
	pathFlag := fs.String("path", "", "limit to file or directory")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("symbols requires a file path")
	}
	return commands.SymbolsMain(map[string]interface{}{
		"file":       rest[0],
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
	})
}

func runRdeps(argv []string) error {
	fs := newFlagSet("rdeps")
	limit := fs.Int("limit", 10, "max importer files")
	pathFlag := fs.String("path", "", "limit to file or directory")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("rdeps requires a file path")
	}
	return commands.RdepsMain(map[string]interface{}{
		"file":       rest[0],
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
	})
}

func runDeps(argv []string) error {
	fs := newFlagSet("deps")
	limit := fs.Int("limit", 10, "max dependencies")
	pathFlag := fs.String("path", "", "limit to file or directory")
	pathsOnly := fs.Bool("paths-only", false, "print unique file paths only")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("deps requires a symbol or file target")
	}
	return commands.DepsMain(map[string]interface{}{
		"target":     rest[0],
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
		"paths_only": *pathsOnly,
	})
}

func runMembers(argv []string) error {
	fs := newFlagSet("members")
	limit := fs.Int("limit", 10, "max members")
	pathFlag := fs.String("path", "", "limit to file or directory")
	namesOnly := fs.Bool("names-only", false, "print member names only")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("members requires a symbol name")
	}
	return commands.MembersMain(map[string]interface{}{
		"symbol":     rest[0],
		"limit":      *limit,
		"path_scope": pathScope(*pathFlag),
		"names_only": *namesOnly,
	})
}

func runSkill(argv []string) error {
	fs := newFlagSet("skill")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	rest := fs.Args()
	target := ""
	if len(rest) > 0 {
		target = rest[0]
	}
	return commands.SkillMain(map[string]interface{}{
		"path": target,
	})
}

func runAnalyze(argv []string) error {
	fs := newFlagSet("analyze")
	limit := fs.Int("limit", 20, "max result rows total")
	pathFlag := fs.String("path", "", "limit to file or directory")
	includeTests := fs.Bool("include-tests", false, "include test paths")
	priority := fs.String("priority", "", "comma-separated check tiers")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	target := ""
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	return commands.AnalyzeMain(map[string]interface{}{
		"target":        target,
		"limit":         *limit,
		"path_scope":    pathScope(*pathFlag),
		"include_tests": *includeTests,
		"priority":      *priority,
	})
}

func runReindex(argv []string) error {
	fs := newFlagSet("reindex")
	var pathArgs []string
	fs.Func("path", "index only tsconfig projects under PATH", func(s string) error {
		pathArgs = append(pathArgs, s)
		return nil
	})
	if err := fs.Parse(argv); err != nil {
		return err
	}
	return commands.ReindexMain(map[string]interface{}{
		"path": pathArgs,
	})
}

func newFlagSet(name string) *flagSet {
	return &flagSet{name: name}
}

type flagSet struct {
	name   string
	bools  map[string]*bool
	ints   map[string]*int
	strs   map[string]*string
	intSet map[string]*bool
	funcs  []funcFlag
	args   []string
	parsed bool
}

func (f *flagSet) trackInt(name string, p *int, set *bool) {
	if f.intSet == nil {
		f.intSet = make(map[string]*bool)
	}
	f.intSet[name] = set
}

type funcFlag struct {
	name string
	fn   func(string) error
}

func (f *flagSet) Bool(name string, def bool, usage string) *bool {
	if f.bools == nil {
		f.bools = make(map[string]*bool)
	}
	v := def
	f.bools[name] = &v
	return &v
}

func (f *flagSet) BoolVar(p *bool, name string, def bool, usage string) {
	*p = def
	if f.bools == nil {
		f.bools = make(map[string]*bool)
	}
	f.bools[name] = p
}

func (f *flagSet) Int(name string, def int, usage string) *int {
	if f.ints == nil {
		f.ints = make(map[string]*int)
	}
	v := def
	f.ints[name] = &v
	return &v
}

func (f *flagSet) String(name string, def string, usage string) *string {
	if f.strs == nil {
		f.strs = make(map[string]*string)
	}
	v := def
	f.strs[name] = &v
	return &v
}

func (f *flagSet) Func(name, usage string, fn func(string) error) {
	f.funcs = append(f.funcs, funcFlag{name: name, fn: fn})
}

func (f *flagSet) Parse(argv []string) error {
	i := 0
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			f.args = append(f.args, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			f.args = append(f.args, arg)
			i++
			continue
		}

		name := strings.TrimLeft(arg, "-")
		eqVal := ""
		if idx := strings.Index(name, "="); idx >= 0 {
			eqVal = name[idx+1:]
			name = name[:idx]
		}

		if name == "h" || name == "help" {
			return fmt.Errorf("use scip-cli help")
		}

		if p, ok := f.bools[name]; ok {
			if eqVal != "" {
				return fmt.Errorf("bool flag --%s cannot take a value", name)
			}
			*p = true
			i++
			continue
		}

		val := eqVal
		if val == "" {
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
				return fmt.Errorf("flag --%s requires a value", name)
			}
			i++
			val = argv[i]
		}

		if p, ok := f.ints[name]; ok {
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
				return fmt.Errorf("flag --%s must be an integer", name)
			}
			if name == "limit" && n < 1 {
				return fmt.Errorf("flag --%s must be an integer >= 1", name)
			}
			if name == "offset" && n < 0 {
				return fmt.Errorf("flag --%s must be >= 0", name)
			}
			*p = n
			if set, ok := f.intSet[name]; ok {
				*set = true
			}
			i++
			continue
		}

		if p, ok := f.strs[name]; ok {
			*p = val
			i++
			continue
		}

		for _, ff := range f.funcs {
			if ff.name == name {
				if err := ff.fn(val); err != nil {
					return err
				}
				i++
				goto next
			}
		}

		return fmt.Errorf("unknown flag --%s", name)
	next:
		i++
	}
	f.parsed = true
	return nil
}

func (f *flagSet) Args() []string {
	return f.args
}

func printUsage() {
	exe := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `scip-cli %s — Fast code intelligence via SCIP indexes

Usage: %s <command> [flags] [args]

Commands:
  refs      Find references to a symbol
  code      Find symbol definition
  search    Search symbols by pattern
  symbols   List symbols in a file
  rdeps     Find reverse dependencies of a file
  deps      Find outbound dependencies
  members   List members of a class/interface
  skill     Install or dump the scip-cli SKILL.md
  analyze   Run SQL-based analysis
  reindex   Force re-indexing of the current project

AI agents: run '%s skill' for quick reference
`, version, exe, exe)
}
