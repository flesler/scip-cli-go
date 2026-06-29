package commands

import (
	"fmt"
	"os"

	"github.com/flesler/scip-cli-go/internal/clierr"
	"github.com/flesler/scip-cli-go/internal/output"
	"github.com/flesler/scip-cli-go/internal/queries"
	"github.com/flesler/scip-cli-go/internal/session"
	"github.com/flesler/scip-cli-go/internal/symbols"
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
