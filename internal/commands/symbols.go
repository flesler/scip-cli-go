package commands

import (
	"fmt"
	"os"
	"sort"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/output"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/session"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

func SymbolsMain(args map[string]interface{}) error {
	db, _, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	filePattern := args["file"].(string)
	limit := args["limit"].(int)
	freq := false
	if f, ok := args["freq"].(bool); ok {
		freq = f
	}

	filePath, err := session.ResolveOneFile(db, filePattern, pathScope)
	if err != nil {
		return err
	}

	limitPlusOne := limit + 1
	syms, err := queries.GetFileSymbols(db, filePath, &limitPlusOne)
	if err != nil {
		return err
	}

	if len(syms) == 0 {
		fmt.Fprintf(os.Stderr, "No symbols found in '%s'\n", filePath)
		return clierr.Exit(1)
	}

	syms = output.LimitAndWarn(syms, limit, "symbols")

	if freq {
		syms = sortByFrequency(syms)
	}

	for _, sym := range syms {
		if symbols.IsModuleSymbol(sym.Symbol) {
			continue
		}
		kind := symbols.InferKind(sym.Symbol)
		short := symbols.ExtractLeafName(sym.Symbol)
		lineInfo := output.FormatLineRange(&sym.StartLine, &sym.EndLine, "-")
		fmt.Printf("%s %s %s\n", lineInfo, string(kind), short)
	}

	return nil
}

func sortByFrequency(syms []queries.FileSymbol) []queries.FileSymbol {
	// Count occurrences of each symbol name
	counts := make(map[string]int)
	for _, sym := range syms {
		name := symbols.ExtractLeafName(sym.Symbol)
		counts[name]++
	}

	// Create a copy to sort
	sorted := make([]queries.FileSymbol, len(syms))
	copy(sorted, syms)

	// Sort by frequency (descending), then by name (ascending) for ties
	sort.SliceStable(sorted, func(i, j int) bool {
		nameI := symbols.ExtractLeafName(sorted[i].Symbol)
		nameJ := symbols.ExtractLeafName(sorted[j].Symbol)
		countI := counts[nameI]
		countJ := counts[nameJ]

		if countI != countJ {
			return countI > countJ // Higher frequency first
		}
		return nameI < nameJ // Alphabetical for ties
	})

	return sorted
}
