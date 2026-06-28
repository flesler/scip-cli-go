package commands

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed SKILL.md
var skillContent embed.FS

func SkillMain(args map[string]interface{}) error {
	content, err := skillContent.ReadFile("SKILL.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: SKILL.md not found in package")
		os.Exit(1)
	}

	targetPath := args["path"].(string)
	if targetPath != "" {
		target := os.ExpandEnv(targetPath)
		if filepath.Ext(target) == "" {
			target = filepath.Join(target, "SKILL.md")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Installed skill to %s\n", target)
	} else {
		fmt.Print(string(content))
	}

	return nil
}
