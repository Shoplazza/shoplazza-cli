package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillsync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/updatecheck"
)

// EnvNoNotice disables the agent-facing _notice envelope block when set to "1".
const EnvNoNotice = "SHOPLAZZA_CLI_NO_NOTICE"

// buildNotice assembles the "_notice" block for json success envelopes: an
// "update" entry when a newer CLI is available (from the already-computed
// cache, so it costs nothing), and a "skills" entry when the Agent Skills are
// not installed. Returns nil when there is nothing to say or the user opted out.
func buildNotice(update *updatecheck.Info) map[string]any {
	if os.Getenv(EnvNoNotice) == "1" {
		return nil
	}
	n := map[string]any{}
	if update != nil {
		n["update"] = map[string]any{
			"current": update.Current,
			"latest":  update.Latest,
			"message": update.Message(),
		}
	}
	// A missing skills dir reads as (false, nil); an unreadable one errors — in
	// either error case we simply say nothing rather than guess.
	if installed, err := skillsync.Installed(); err == nil && !installed {
		n["skills"] = map[string]any{
			"installed": false,
			"message":   "Agent Skills not installed — run 'npx skills add Shoplazza/shoplazza-cli -g' for CLI-aware guidance.",
		}
	}
	if len(n) == 0 {
		return nil
	}
	return n
}

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

// stderrIsTTY reports whether stderr is an interactive terminal.
func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
