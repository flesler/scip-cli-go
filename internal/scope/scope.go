package scope

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/scip-cli-go/internal/cache"
)

const ScopeFilename = "index-scope.json"

type IndexScope struct {
	Paths []string `json:"paths"`
}

func scopePath(projectRoot string) string {
	return filepath.Join(cache.GetCacheDir(projectRoot), ScopeFilename)
}

func LoadIndexScope(projectRoot string) *IndexScope {
	path := scopePath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	pathsRaw, ok := raw["paths"].([]interface{})
	if !ok || len(pathsRaw) == 0 {
		return nil
	}

	var paths []string
	for _, p := range pathsRaw {
		if s, ok := p.(string); ok {
			paths = append(paths, s)
		} else {
			return nil
		}
	}

	return &IndexScope{Paths: paths}
}

func SaveIndexScope(projectRoot string, paths []string) error {
	path := scopePath(projectRoot)
	if len(paths) == 0 {
		_ = os.Remove(path)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data := map[string]interface{}{"paths": paths}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(jsonData, '\n'), 0644)
}

func ProjectInScope(project string, scopePaths []string) bool {
	proj := filepath.ToSlash(project)
	for _, prefix := range scopePaths {
		p := strings.TrimSuffix(prefix, "/")
		if proj == p || strings.HasPrefix(proj, p+"/") {
			return true
		}
	}
	return false
}

func ProjectsMatchingScope(projects []string, scopePaths []string) []string {
	var result []string
	for _, p := range projects {
		if ProjectInScope(p, scopePaths) {
			result = append(result, p)
		}
	}
	return result
}
