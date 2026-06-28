package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var skipDirNames = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	"coverage":     true,
	".cache":       true,
	"vendor":       true,
	".turbo":       true,
	".nx":          true,
	"tmp":          true,
}

func readJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	return result, err
}

func tsconfigProjectRoot(path string) bool {
	data, err := readJSON(path)
	if err != nil {
		return false
	}
	name := filepath.Base(path)
	if strings.HasPrefix(name, "tsconfig.") && name != "tsconfig.json" {
		_, hasInclude := data["include"]
		_, hasFiles := data["files"]
		_, hasRefs := data["references"]
		if !hasInclude && !hasFiles && !hasRefs {
			return false
		}
	}
	_, hasInclude := data["include"]
	_, hasFiles := data["files"]
	_, hasRefs := data["references"]
	return hasInclude || hasFiles || hasRefs || name == "tsconfig.json"
}

func tsconfigCoversSubdirectories(tsconfigPath string) bool {
	data, err := readJSON(tsconfigPath)
	if err != nil {
		return false
	}
	includeRaw, ok := data["include"]
	if !ok {
		return false
	}
	include, ok := includeRaw.([]interface{})
	if !ok || len(include) == 0 {
		return false
	}
	for _, pattern := range include {
		if s, ok := pattern.(string); ok && (strings.Contains(s, "**") || strings.Contains(s, "/")) {
			return true
		}
	}
	return false
}

func walkTsconfigProjects(root string) []string {
	root, _ = filepath.Abs(root)
	var projects []string

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if skipDirNames[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if !strings.HasPrefix(name, "tsconfig") || !strings.HasSuffix(name, ".json") {
			return nil
		}
		if !tsconfigProjectRoot(path) {
			return nil
		}
		projectDir := filepath.Dir(path)
		projectDir, _ = filepath.Abs(projectDir)
		rel, err := filepath.Rel(root, projectDir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		projects = append(projects, projectDir)
		return nil
	})

	return projects
}

func dedupeNested(projects []string) []string {
	if len(projects) <= 1 {
		return projects
	}

	resolved := make(map[string]bool)
	for _, p := range projects {
		abs, _ := filepath.Abs(p)
		resolved[abs] = true
	}

	var sorted []string
	for p := range resolved {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	var kept []string
	for _, candidate := range sorted {
		isAncestor := false
		for _, other := range sorted {
			if candidate == other {
				continue
			}
			rel, err := filepath.Rel(candidate, other)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(rel, "..") && rel != "." {
				isAncestor = true
				break
			}
		}
		if !isAncestor {
			kept = append(kept, candidate)
		}
	}
	return kept
}

func shouldIndexRootAlongsideProjects(root string, projects []string) bool {
	if len(projects) == 0 || (len(projects) == 1 && projects[0] == ".") {
		return false
	}
	rootTsconfig := filepath.Join(root, "tsconfig.json")
	if _, err := os.Stat(rootTsconfig); os.IsNotExist(err) {
		return false
	}
	return tsconfigCoversSubdirectories(rootTsconfig)
}

// DiscoverProjects finds TypeScript project roots under the given directory.
// scope is currently unused but reserved for future filtering.
func DiscoverProjects(root string, scope string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root: %w", err)
	}

	discovered := dedupeNested(walkTsconfigProjects(root))

	if len(discovered) > 0 {
		var relative []string
		for _, p := range discovered {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				continue
			}
			if rel == "" {
				rel = "."
			}
			relative = append(relative, rel)
		}
		sort.Strings(relative)

		if shouldIndexRootAlongsideProjects(root, relative) {
			hasRoot := false
			for _, r := range relative {
				if r == "." {
					hasRoot = true
					break
				}
			}
			if !hasRoot {
				relative = append([]string{"."}, relative...)
			}
		}
		return relative, nil
	}

	return []string{"."}, nil
}
