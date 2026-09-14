package scope

import (
	"path/filepath"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/metadata"
)

type IndexScope struct {
	Paths []string
}

func LoadIndexScope(projectRoot string) *IndexScope {
	meta := metadata.LoadMetadata(projectRoot)
	if len(meta.ScopePaths) == 0 {
		return nil
	}
	return &IndexScope{Paths: meta.ScopePaths}
}

func SaveIndexScope(projectRoot string, paths []string) error {
	current := metadata.LoadMetadata(projectRoot)
	return metadata.SaveMetadata(projectRoot, metadata.IndexMetadata{
		ScopePaths:   paths,
		ExcludeGlobs: current.ExcludeGlobs,
	})
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
