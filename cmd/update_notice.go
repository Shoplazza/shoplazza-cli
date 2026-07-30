package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// updateCheckSkippedCommands lists TOP-LEVEL commands that suppress the update
// notice and background metadata refresh (to avoid nagging mid-update and
// avoid corrupting completion output).
var updateCheckSkippedCommands = map[string]bool{
	"update":     true,
	"completion": true,
}

// isUpdateCheckSkippedCommand reports whether the invoked top-level command
// should skip the update checks. It resolves through Cobra's Find, which strips
// flags — so `--format json update` matches, `products update` does not.
func isUpdateCheckSkippedCommand(root *cobra.Command, args []string) bool {
	// Cobra registers __complete only inside Execute, so Find can't see it.
	if len(args) > 0 && (args[0] == "__complete" || args[0] == "__completeNoDesc") {
		return true
	}
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return false
	}
	for cmd.HasParent() && cmd.Parent() != root {
		cmd = cmd.Parent()
	}
	return updateCheckSkippedCommands[cmd.Name()]
}

// stderrIsTTY reports whether stderr is an interactive terminal.
func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
