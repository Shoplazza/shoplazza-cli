package appcmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

// localFunctionOptions lists the project's local function extensions for a fuzzy
// --name picker (value = directory name, which compile/release resolve against).
// It scans extensions/ under the --path project root — local FS, no network, so
// it fits compile's zero-auth flow. On any error / empty scan the caller
// (ResolveFlags) degrades to manual entry.
func localFunctionOptions(_ context.Context, cmd *cobra.Command, _ *cmdutil.Factory) ([]interact.Option, error) {
	path, _ := cmd.Flags().GetString("path")
	p, err := openProject(path)
	if err != nil {
		return nil, err
	}
	locals, sErr := app.ScanLocalExtensions(p.Root)
	if sErr != nil {
		return nil, sErr
	}
	return localFunctionPickerOptions(locals), nil
}

// localFunctionPickerOptions maps scanned extensions to picker options, keeping
// only function extensions: value = dir (compile/release key on it), label = dir
// (plus the configured name when it differs). Pure, so the mapping is unit-tested.
func localFunctionPickerOptions(locals []app.LocalExt) []interact.Option {
	opts := make([]interact.Option, 0, len(locals))
	for _, l := range locals {
		if l.Type != "function" {
			continue
		}
		label := l.Dir
		if l.Name != "" && l.Name != l.Dir {
			label = l.Dir + " · " + l.Name
		}
		opts = append(opts, interact.Option{Label: label, Value: l.Dir})
	}
	return opts
}
