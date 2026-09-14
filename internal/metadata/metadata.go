package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/flesler/scip-cli-go/v2/internal/cache"
)

const MetadataFilename = "metadata.json"

type IndexMetadata struct {
	ScopePaths   []string
	ExcludeGlobs []string
}

type OptionalSlice struct {
	Set   bool
	Value []string
}

func MetadataPath(projectRoot string) string {
	return filepath.Join(cache.GetCacheDir(projectRoot), MetadataFilename)
}

func LoadMetadata(projectRoot string) IndexMetadata {
	path := MetadataPath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return IndexMetadata{}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return IndexMetadata{}
	}

	return IndexMetadata{
		ScopePaths:   readStringList(raw["scope"], "paths"),
		ExcludeGlobs: readStringList(raw["exclude"], "globs"),
	}
}

func SaveMetadata(projectRoot string, metadata IndexMetadata) error {
	path := MetadataPath(projectRoot)
	if len(metadata.ScopePaths) == 0 && len(metadata.ExcludeGlobs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	payload := make(map[string]map[string][]string)
	if len(metadata.ScopePaths) > 0 {
		payload["scope"] = map[string][]string{"paths": metadata.ScopePaths}
	}
	if len(metadata.ExcludeGlobs) > 0 {
		payload["exclude"] = map[string][]string{"globs": metadata.ExcludeGlobs}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func readStringList(section interface{}, key string) []string {
	sectionMap, ok := section.(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := sectionMap[key].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	var out []string
	for _, item := range raw {
		s, ok := item.(string)
		if !ok || s == "" {
			return nil
		}
		out = append(out, s)
	}
	return out
}

func ApplyMetadataUpdates(
	projectRoot string,
	fresh bool,
	scopePaths *OptionalSlice,
	excludeGlobs *OptionalSlice,
) (IndexMetadata, error) {
	var scope []string
	var exclude []string

	changed := fresh
	if fresh {
		scope = nil
		exclude = nil
	} else {
		current := LoadMetadata(projectRoot)
		scope = current.ScopePaths
		exclude = current.ExcludeGlobs
	}

	if scopePaths != nil && scopePaths.Set {
		changed = true
		if len(scopePaths.Value) > 0 {
			scope = scopePaths.Value
		} else {
			scope = nil
		}
	}
	if excludeGlobs != nil && excludeGlobs.Set {
		changed = true
		if len(excludeGlobs.Value) > 0 {
			exclude = excludeGlobs.Value
		} else {
			exclude = nil
		}
	}

	result := IndexMetadata{
		ScopePaths:   scope,
		ExcludeGlobs: exclude,
	}
	if changed {
		if err := SaveMetadata(projectRoot, result); err != nil {
			return result, err
		}
	}
	return result, nil
}
