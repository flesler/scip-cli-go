package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFilename = ".scip-cli.json"

type ProjectSettings struct {
	MaxHeapMb      *int     `json:"maxHeapMb,omitempty"`
	IndexRoots     []string `json:"indexRoots,omitempty"`
	OnlyIndexRoots bool     `json:"onlyIndexRoots,omitempty"`
}

func LoadProjectConfig(projectRoot string) (*ProjectSettings, error) {
	path := filepath.Join(projectRoot, ConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectSettings{}, nil
		}
		return nil, fmt.Errorf("invalid %s: %w", ConfigFilename, err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", ConfigFilename, err)
	}

	settings := &ProjectSettings{}

	if v, ok := raw["maxHeapMb"]; ok {
		if v == nil {
			settings.MaxHeapMb = nil
		} else if f, ok := v.(float64); ok {
			n := int(f)
			if n <= 0 {
				return nil, fmt.Errorf("invalid %s: maxHeapMb must be a positive integer", ConfigFilename)
			}
			settings.MaxHeapMb = &n
		} else {
			return nil, fmt.Errorf("invalid %s: maxHeapMb must be a positive integer", ConfigFilename)
		}
	}

	if v, ok := raw["indexRoots"]; ok {
		if v != nil {
			if arr, ok := v.([]interface{}); ok {
				settings.IndexRoots = make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						settings.IndexRoots = append(settings.IndexRoots, s)
					} else {
						return nil, fmt.Errorf("invalid %s: indexRoots must be a string array", ConfigFilename)
					}
				}
			} else {
				return nil, fmt.Errorf("invalid %s: indexRoots must be a string array", ConfigFilename)
			}
		}
	}

	if v, ok := raw["onlyIndexRoots"]; ok {
		if b, ok := v.(bool); ok {
			settings.OnlyIndexRoots = b
		} else {
			return nil, fmt.Errorf("invalid %s: onlyIndexRoots must be a boolean", ConfigFilename)
		}
	}

	return settings, nil
}

func ResolveIndexRoots(projectRoot string, settings *ProjectSettings) ([]string, error) {
	base, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	var roots []string
	for _, entry := range settings.IndexRoots {
		candidate := filepath.Join(base, entry)
		candidate, err = filepath.Abs(candidate)
		if err != nil {
			return nil, err
		}

		rel, err := filepath.Rel(base, candidate)
		if err != nil || (len(rel) >= 2 && rel[:2] == "..") {
			return nil, fmt.Errorf("indexRoots entry escapes project root: %s", entry)
		}

		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("indexRoots path does not exist: %s", entry)
		}

		roots = append(roots, rel)
	}
	return roots, nil
}
