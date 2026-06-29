package paths

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flesler/scip-cli-go/internal/sqlhelp"
)

func NormalizePathScope(pathArg, projectRoot string) (string, error) {
	if pathArg == "" {
		return "", nil
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}

	candidate := pathArg
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("--path escapes project root: %s", pathArg)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("--path escapes project root: %s", pathArg)
	}

	if rel == "" || rel == "." {
		return ".", nil
	}
	return filepath.ToSlash(rel), nil
}

func PathInScope(relativePath, scope string) bool {
	if scope == "" {
		return true
	}
	if relativePath == scope {
		return true
	}
	scope = strings.TrimSuffix(scope, "/")
	return strings.HasPrefix(relativePath, scope+"/")
}

func PathFilterSQL(db *sql.DB, scope, docAlias string) (string, []interface{}, error) {
	if scope == "" {
		return "", nil, nil
	}
	if docAlias == "" {
		docAlias = "d"
	}

	col := docAlias + ".relative_path"

	var exists int
	err := sqlhelp.DebugExecuteOne(db, "SELECT 1 FROM documents WHERE relative_path = ? LIMIT 1", scope).Scan(&exists)
	isFile := err == nil

	if isFile {
		return fmt.Sprintf(" AND %s = ?", col), []interface{}{scope}, nil
	}

	escaped := sqlhelp.EscapeLike(strings.TrimSuffix(scope, "/"))
	return fmt.Sprintf(` AND (%s = ? OR %s LIKE ? ESCAPE '\')`, col, col), []interface{}{scope, escaped + "/%"}, nil
}

func PathFilterSQLAny(db *sql.DB, scope string, docAliases ...string) (string, []interface{}, error) {
	if scope == "" {
		return "", nil, nil
	}

	var parts []string
	var params []interface{}
	for _, alias := range docAliases {
		clause, clauseParams, err := PathFilterSQL(db, scope, alias)
		if err != nil {
			return "", nil, err
		}
		if clause == "" {
			continue
		}
		clause = strings.TrimPrefix(clause, " AND ")
		parts = append(parts, clause)
		params = append(params, clauseParams...)
	}

	if len(parts) == 0 {
		return "", nil, nil
	}
	if len(parts) == 1 {
		return " AND " + parts[0], params, nil
	}
	return " AND (" + strings.Join(parts, " OR ") + ")", params, nil
}

func ListIndexedPathsInScope(db *sql.DB, scope string) ([]string, error) {
	clause, params, err := PathFilterSQL(db, scope, "d")
	if err != nil {
		return nil, err
	}
	if clause == "" {
		return nil, nil
	}

	rows, err := sqlhelp.DebugExecute(db, fmt.Sprintf("SELECT d.relative_path FROM documents d WHERE 1=1%s ORDER BY d.relative_path", clause), params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
