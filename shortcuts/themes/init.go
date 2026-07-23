package themes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

// initShortcut is the `themes init` workflow: clones the Nova-2023 GitHub
// template into a new directory and prints a cd hint to stderr.
//
// It uses Execute (not Plan) because the operation is a GitHub HTTP fetch plus
// local extraction, not a Shoplazza API call. The "next steps" hint goes
// directly to os.Stderr since common.ExecInput has no Stderr field yet.
var initShortcut = common.Shortcut{
	Service: "themes",
	Command: "init",
	Use:     "init --name <dir>",
	Short:   "Initialize a new theme by cloning the Nova-2023 template",
	// Purely local (GitHub fetch + extraction): runs without login and reports
	// a local artifact rather than an API response (no {ok,data} envelope).
	// Writes the local filesystem, so blind scans skip it.
	AuthFree:     true,
	Local:        true,
	NotScannable: true,
	Flags: []common.Flag{
		{
			Name:        "name",
			Type:        common.FlagString,
			Required:    true,
			Description: "Target directory name for the new theme",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		name := in.Flags.GetString("name")
		if name == "" {
			// Defensive guard so direct Execute callers (tests, programmatic
			// uses) get a clean validation error instead of a silent clone;
			// cobra normally catches this before we reach here.
			return common.ExecResult{}, theme.ErrValidation("missing required flag --name")
		}
		if err := validateInitName(name); err != nil {
			return common.ExecResult{}, err
		}
		stderr := os.Stderr

		if in.DryRun {
			fmt.Fprintf(stderr, "[init] (dry-run) would clone %s into ./%s\n", pack.TemplateTarballURL, name)
			printCdHint(stderr, name, true)
			return common.ExecResult{Body: map[string]any{
				"action":    "clone-template",
				"target":    "./" + name,
				"source":    pack.TemplateTarballURL,
				"dry_run":   true,
				"theme_dir": "./" + name,
			}}, nil
		}

		fmt.Fprintf(stderr, "[init] cloning template Shoplazza/Nova-2023...\n")
		if err := pack.CloneTemplate(ctx, name); err != nil {
			return common.ExecResult{}, classifyCloneErr(err)
		}
		fmt.Fprintf(stderr, "[init] theme initialized at ./%s\n", name)
		printCdHint(stderr, name, false)
		return common.ExecResult{Body: map[string]any{
			"theme_dir": "./" + name,
			"status":    "initialized",
		}}, nil
	},
}

// validateInitName rejects --name values that are not a plain directory
// name: path separators, "..", "." and absolute paths would let the clone
// extract outside the cwd (verified ../../ escape).
func validateInitName(name string) error {
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) || name == ".." || name == "." {
		return theme.ErrValidation(
			"invalid --name %q: must be a plain directory name (no path separators, '..' or absolute paths)", name)
	}
	return nil
}

// classifyCloneErr maps a pack.CloneTemplate failure to the right envelope:
//
//	target dir non-empty           → validation (pick another --name)
//	non-200 from GitHub            → network (typed sentinel)
//	DNS / dial / TLS / timeout     → network
//	everything else (disk, tar)    → internal (local I/O)
func classifyCloneErr(err error) error {
	if errors.Is(err, pack.ErrTargetDirNotEmpty) {
		return theme.ErrValidation("%v; choose another --name or empty the directory first", err)
	}
	if errors.Is(err, pack.ErrTemplateDownload) {
		return theme.ErrCloneNetwork(err)
	}
	var netErr net.Error
	var urlErr *url.Error
	if errors.As(err, &netErr) || errors.As(err, &urlErr) {
		return theme.ErrCloneNetwork(err)
	}
	return theme.ErrLocalIO("clone template", err)
}

// printCdHint writes the multi-line "next steps" cd hint to w. When dryRun is
// true, a parenthetical preamble flags that the directory does not yet exist.
func printCdHint(w io.Writer, name string, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(if executed, will land at ./" + name + ")\n"
	}
	fmt.Fprintf(w, "\n%snext steps:\n   cd %s\n   shoplazza themes serve\n", prefix, name)
}
