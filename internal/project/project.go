package project

import (
	"os"
	"path/filepath"
)

type Language string

const (
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
)

func FindProjectRootAndLanguage(startDir string) (string, Language, bool) {
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	d, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", false
	}

	for {
		if _, err := os.Stat(filepath.Join(d, "package.json")); err == nil {
			return d, LanguageTypeScript, true
		}
		if _, err := os.Stat(filepath.Join(d, "tsconfig.json")); err == nil {
			return d, LanguageTypeScript, true
		}
		if _, err := os.Stat(filepath.Join(d, "pyproject.toml")); err == nil {
			return d, LanguagePython, true
		}
		if _, err := os.Stat(filepath.Join(d, "setup.py")); err == nil {
			return d, LanguagePython, true
		}

		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", "", false
}

func FindProjectRoot(startDir string) (string, bool) {
	root, _, ok := FindProjectRootAndLanguage(startDir)
	return root, ok
}

func DetectLanguage(projectRoot string) (Language, bool) {
	_, lang, ok := FindProjectRootAndLanguage(projectRoot)
	return lang, ok
}
