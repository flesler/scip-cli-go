package exclude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/cache"
	"github.com/flesler/scip-cli-go/v2/internal/config"
)

const ExcludeFilename = "index-exclude.json"

func excludePath(projectRoot string) string {
	return filepath.Join(cache.GetCacheDir(projectRoot), ExcludeFilename)
}

func LoadPersistedExcludeGlobs(projectRoot string) []string {
	path := excludePath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	globs, ok := raw["globs"].([]interface{})
	if !ok || len(globs) == 0 {
		return nil
	}
	var out []string
	for _, g := range globs {
		if s, ok := g.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func SavePersistedExcludeGlobs(projectRoot string, globs []string) error {
	path := excludePath(projectRoot)
	if len(globs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	payload := map[string][]string{"globs": globs}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func ResolveExcludeGlobs(projectRoot string) ([]string, error) {
	settings, err := config.LoadProjectConfig(projectRoot)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var merged []string
	for _, glob := range append(settings.ExcludeGlobs, LoadPersistedExcludeGlobs(projectRoot)...) {
		if glob == "" || seen[glob] {
			continue
		}
		if err := ValidateGlob(glob); err != nil {
			return nil, err
		}
		seen[glob] = true
		merged = append(merged, glob)
	}
	return merged, nil
}

func globToRegex(pattern string) (*regexp.Regexp, error) {
	normalized := strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "/")
	var parts []string
	index := 0
	for index < len(normalized) {
		char := normalized[index]
		switch char {
		case '*':
			if index+1 < len(normalized) && normalized[index+1] == '*' {
				if index+2 < len(normalized) && normalized[index+2] == '/' {
					parts = append(parts, "(?:.+/)?")
					index += 3
					continue
				}
				parts = append(parts, ".*")
				index += 2
				continue
			}
			parts = append(parts, "[^/]*")
			index++
		case '?':
			parts = append(parts, "[^/]")
			index++
		default:
			if strings.ContainsRune(".^$+{}|()[]", rune(char)) {
				parts = append(parts, regexp.QuoteMeta(string(char)))
			} else {
				parts = append(parts, string(char))
			}
			index++
		}
	}
	return regexp.Compile("^" + strings.Join(parts, "") + "$")
}

func PathMatchesGlob(relativePath, pattern string) bool {
	path := strings.TrimPrefix(strings.ReplaceAll(relativePath, "\\", "/"), "/")
	glob := strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "/")
	if glob == "" {
		return false
	}
	target := path
	if !strings.Contains(glob, "/") {
		target = filepath.Base(path)
	}
	re, err := globToRegex(glob)
	if err != nil {
		return false
	}
	return re.MatchString(target)
}

func PathMatchesAnyGlob(relativePath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if PathMatchesGlob(relativePath, pattern) {
			return true
		}
	}
	return false
}

func FormatExcludeGlobs(globs []string) string {
	return strings.Join(globs, ", ")
}

func ValidateGlob(pattern string) error {
	if _, err := globToRegex(pattern); err != nil {
		return fmt.Errorf("invalid exclude glob %q: %w", pattern, err)
	}
	return nil
}
