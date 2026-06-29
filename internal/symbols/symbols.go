package symbols

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"
)

type SymbolKind string

const (
	KindFunction SymbolKind = "function"
	KindMethod   SymbolKind = "method"
	KindClass    SymbolKind = "class"
	KindProperty SymbolKind = "property"
	KindUnknown  SymbolKind = "unknown"
)

func FilterableKinds() []SymbolKind {
	return []SymbolKind{KindFunction, KindMethod, KindClass, KindProperty}
}

// ParseFilterableKind validates --kind values (mirrors Python argparse choices).
func ParseFilterableKind(s string) (*SymbolKind, error) {
	if s == "" {
		return nil, nil
	}
	k := SymbolKind(s)
	for _, fk := range FilterableKinds() {
		if k == fk {
			return &k, nil
		}
	}
	return nil, fmt.Errorf("invalid kind %q (choices: function, method, class, property)", s)
}

func KindSQLClause(kind SymbolKind) string {
	switch kind {
	case KindFunction:
		return " AND gs.symbol LIKE '%().' AND gs.symbol NOT LIKE '%#%().'"
	case KindMethod:
		return " AND gs.symbol LIKE '%#%' AND gs.symbol LIKE '%().'"
	case KindClass:
		return " AND gs.symbol LIKE '%#' AND gs.symbol NOT LIKE '%().'"
	case KindProperty:
		return " AND gs.symbol LIKE '%#typeLiteral%'"
	}
	return ""
}

func IsVariableSymbol(s string) bool {
	if strings.HasSuffix(s, "/") || strings.Contains(s, ").(") || strings.Contains(s, "#typeLiteral") {
		return false
	}
	return strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "().")
}

func SQLExcludeVariableSymbols(column string) string {
	if column == "" {
		column = "symbol"
	}
	c := column
	return "NOT (" + c + " LIKE '%.' AND " + c + " NOT LIKE '%().' " +
		"AND " + c + " NOT LIKE '%#typeLiteral%' AND " + c + " NOT LIKE '%).(%' " +
		"AND " + c + " NOT LIKE '%/')"
}

func IsModuleSymbol(s string) bool {
	return strings.HasSuffix(s, "/")
}

func IsTypeOrInterfaceSymbol(s string) bool {
	if strings.HasSuffix(s, "/") || strings.HasSuffix(s, "().") {
		return false
	}
	if strings.Contains(s, "#typeLiteral") {
		return true
	}
	tail := s[strings.LastIndex(s, "/")+1:]
	return strings.Contains(tail, "#")
}

func CycleEdgeTypeSQL(column string) string {
	if column == "" {
		column = "gs.symbol"
	}
	c := column
	return "(" + c + " LIKE '%().' OR " + c + " LIKE '%/' OR (" + c + " NOT LIKE '%#%' AND " + c + " NOT LIKE '%#typeLiteral%'))"
}

func CycleRuntimeEdgeSQL(column string) string {
	if column == "" {
		column = "gs.symbol"
	}
	c := column
	return "(" + c + " LIKE '%().' OR (" + c + " NOT LIKE '%#%' AND " + c + " NOT LIKE '%#typeLiteral%' AND " + c + " NOT LIKE '%/'))"
}

func InferKind(symbol string) SymbolKind {
	if strings.Contains(symbol, "#") && strings.HasSuffix(symbol, "().") {
		return KindMethod
	}
	if strings.HasSuffix(symbol, "().") {
		return KindFunction
	}
	if strings.HasSuffix(symbol, "#") {
		name := symbol[strings.LastIndex(symbol, "/")+1:]
		name = strings.TrimRight(name, "#")
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			return KindClass
		}
	}
	if strings.Contains(symbol, "#typeLiteral") && strings.Contains(symbol, ":") && strings.HasSuffix(symbol, ".") {
		return KindProperty
	}
	return KindUnknown
}

func EscapeLike(s string) string {
	return sqlhelp.EscapeLike(s)
}

func SymbolLikePatterns(leaf string) []string {
	e := EscapeLike(leaf)
	return []string{
		"%/" + e + "().",
		"%/" + e + "#",
		"%/" + e + ".",
		"%#" + e + "().",
		"%#" + e + ".",
		"%typeLiteral%:" + e + ".",
	}
}

func ParseQualifiedName(name string) ([]string, string) {
	if !strings.Contains(name, ".") {
		return nil, name
	}
	parts := strings.Split(name, ".")
	return parts[:len(parts)-1], parts[len(parts)-1]
}

func IsParameterSymbol(s string) bool {
	return strings.Contains(s, ").(")
}

func SymbolMatchesQualifier(symbolStr string, qualifierParts []string, leaf string) bool {
	if len(qualifierParts) == 0 {
		return true
	}

	idx := strings.LastIndex(symbolStr, "/")
	tail := symbolStr
	if idx >= 0 {
		tail = symbolStr[idx+1:]
	}

	joined := strings.Join(qualifierParts, "#")
	if strings.Contains(tail, joined+"#"+leaf) {
		return true
	}

	container := qualifierParts[len(qualifierParts)-1]
	typeLitRe := regexp.MustCompile(regexp.QuoteMeta(container) + `#typeLiteral\d+:` + regexp.QuoteMeta(leaf) + `\.`)
	if typeLitRe.MatchString(tail) {
		if len(qualifierParts) == 1 {
			return true
		}
		allMatch := true
		for _, part := range qualifierParts[:len(qualifierParts)-1] {
			if !strings.Contains(symbolStr, part) {
				allMatch = false
				break
			}
		}
		return allMatch
	}

	if strings.Contains(tail, container+"#"+leaf) {
		if len(qualifierParts) == 1 {
			return true
		}
		for _, part := range qualifierParts[:len(qualifierParts)-1] {
			if !strings.Contains(symbolStr, part) {
				return false
			}
		}
		return true
	}

	dotted := strings.Join(qualifierParts, ".")
	if strings.Contains(symbolStr, dotted+"."+leaf) || strings.Contains(symbolStr, dotted+"/"+leaf) {
		return true
	}

	if strings.Contains(symbolStr, ".py/") && strings.Contains(symbolStr, container+"#"+leaf) {
		if len(qualifierParts) == 1 {
			return true
		}
		for _, part := range qualifierParts[:len(qualifierParts)-1] {
			if !strings.Contains(symbolStr, part) {
				return false
			}
		}
		return true
	}

	return false
}

func ExtractLeafName(symbolStr string) string {
	idx := strings.LastIndex(symbolStr, "/")
	leaf := symbolStr
	if idx >= 0 {
		leaf = symbolStr[idx+1:]
	}
	leaf = strings.TrimRight(leaf, ".#")
	leaf = strings.TrimSuffix(leaf, "()")
	if strings.Contains(leaf, ":") {
		leaf = leaf[strings.LastIndex(leaf, ":")+1:]
	}
	if strings.Contains(leaf, "#") {
		leaf = leaf[strings.LastIndex(leaf, "#")+1:]
	}
	leaf = strings.ReplaceAll(leaf, "`", "")
	if strings.HasPrefix(leaf, "<get>") || strings.HasPrefix(leaf, "<set>") {
		leaf = leaf[5:]
	}
	return leaf
}

var backtickRe = regexp.MustCompile("`([^`]+)`")
var pyPathRe = regexp.MustCompile(`(\S+\.py)/`)

func ExtractFilePathFromSymbol(symbolStr string) string {
	if loc := backtickRe.FindStringIndex(symbolStr); loc != nil {
		filename := symbolStr[loc[0]+1 : loc[1]-1]
		before := symbolStr[:loc[0]]
		parts := strings.Fields(before)
		if len(parts) >= 5 {
			return strings.Join(parts[4:], " ") + filename
		}
		return filename
	}

	if m := pyPathRe.FindStringSubmatch(symbolStr); m != nil {
		return m[1]
	}
	return ""
}
