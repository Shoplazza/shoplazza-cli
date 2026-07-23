package dynamic

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

// RegisterCommands attaches every spec module as a top-level cobra group on
// rootCmd. Modules whose name collides with an existing root command are
// skipped (defensive against bad spec). Modules with zero registerable
// commands after filtering are also skipped — no empty groups in --help.
func RegisterCommands(rootCmd *cobra.Command, spec *registry.Spec, factory *cmdutil.Factory) {
	if spec == nil {
		return
	}
	existing := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		existing[c.Name()] = true
	}
	seenModule := map[string]bool{}
	for _, mod := range spec.Modules {
		if mod.Name == "" || !isKebabCase(mod.Name) {
			continue
		}
		if seenModule[mod.Name] {
			continue // duplicate module name in spec — skip all duplicates
		}
		seenModule[mod.Name] = true
		if existing[mod.Name] {
			continue // collides with built-in / +xxx command
		}
		modCmd := buildModuleCommand(mod, spec, factory)
		if modCmd == nil {
			continue // no valid commands → skip whole module
		}
		rootCmd.AddCommand(modCmd)
	}
}

func isKebabCase(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' && i > 0:
		default:
			return false
		}
	}
	return true
}
