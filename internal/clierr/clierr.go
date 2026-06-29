package clierr

import (
	"fmt"
	"os"
	"strings"
)

// Fatal prints err to stderr (SQLite → "Database error:") and exits 1.
func Fatal(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "SQLITE_"),
		strings.Contains(lower, "database disk image"),
		strings.Contains(lower, "malformed database"):
		fmt.Fprintf(os.Stderr, "Database error: %v\n", err)
	default:
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(1)
}

// FatalMsg prints a fixed message to stderr and exits 1.
func FatalMsg(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
