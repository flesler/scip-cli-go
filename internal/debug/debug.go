package debug

import (
	"fmt"
	"os"
)

// Log prints message to stderr when SCIP_CLI_DEBUG is set.
func Log(message string) {
	if os.Getenv("SCIP_CLI_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, message)
	}
}
