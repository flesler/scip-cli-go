package exclude

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/config"
	"github.com/flesler/scip-cli-go/v2/internal/metadata"
)

func LoadPersistedExcludeGlobs(projectRoot string) []string {
	meta := metadata.LoadMetadata(projectRoot)
	if len(meta.ExcludeGlobs) == 0 {
		return nil
	}
	return meta.ExcludeGlobs
}

func SavePersistedExcludeGlobs(projectRoot string, globs []string) error {
	current := metadata.LoadMetadata(projectRoot)
	return metadata.SaveMetadata(projectRoot, metadata.IndexMetadata{
		ScopePaths:   current.ScopePaths,
		ExcludeGlobs: globs,
	})
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
