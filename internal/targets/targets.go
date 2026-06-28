package targets

import (
	"regexp"
	"strings"
)

var fileTargetRe = regexp.MustCompile(`(?i)\.(ts|tsx|js|jsx|mjs|cjs|py)$`)

func LooksLikeFileTarget(target string) bool {
	if strings.Contains(target, "/") || strings.Contains(target, "\\") {
		return true
	}
	return fileTargetRe.MatchString(target)
}
