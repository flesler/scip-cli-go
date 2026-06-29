package clierr

import (
	"fmt"
	"os"
	"strings"
)

// exitCode signals a clean CLI exit (message already on stderr when applicable).
type exitCode int

func (e exitCode) Error() string {
	return fmt.Sprintf("exit status %d", int(e))
}

// Exit returns an error that makes Fatal exit with code without printing again.
func Exit(code int) error {
	return exitCode(code)
}

// Fatal prints err to stderr (SQLite → "Database error:") and exits.
func Fatal(err error) {
	if err == nil {
		return
	}
	if code, ok := err.(exitCode); ok {
		os.Exit(int(code))
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
