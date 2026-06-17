package cmd

import "os"

// updateCheckSkippedCommands lists subcommands that suppress the update notice
// (to avoid nagging mid-update and avoid corrupting completion output).
var updateCheckSkippedCommands = map[string]bool{
	"update":     true,
	"completion": true,
	"__complete": true,
}

// isUpdateCheckSkippedCommand reports whether the given args match a command that should skip the update notice.
func isUpdateCheckSkippedCommand(args []string) bool {
	for _, a := range args {
		if updateCheckSkippedCommands[a] {
			return true
		}
	}
	return false
}

// stderrIsTTY reports whether stderr is an interactive terminal.
func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
