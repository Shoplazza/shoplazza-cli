package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillsync"
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

// wantsVersion reports whether this invocation asks for the version. A false
// positive costs one stray directory read — Cobra still gates the template.
func wantsVersion(args []string) bool {
	for _, a := range args {
		switch a {
		case "--version", "-v":
			return true
		case "--":
			return false // past the terminator these are operands, not flags
		}
	}
	return false
}

// skillLine describes the Agent Skills state for the --version output.
func skillLine() string {
	installed, err := skillsync.Installed()
	switch {
	case err != nil:
		return "skills unreadable"
	case installed:
		return "skills installed"
	default:
		return "skills not installed"
	}
}
