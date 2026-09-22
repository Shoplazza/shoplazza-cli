package themes

// Shared test helpers for this package: the golden-file snapshot helpers, the
// command-mounting helpers behind the help and flag tests, and stderr capture.
// The tests themselves live beside the command they cover.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// testdataDir resolves the testdata directory from this test file's source
// location. Anchoring on runtime.Caller (rather than cwd) keeps goldens in the
// package source tree even when a test t.Chdir()s into a tmp theme dir.
func testdataDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

// snapshot serializes got as indented JSON and compares against
// testdata/<name>.golden.json. On first run (file missing) or when
// UPDATE_GOLDEN=1, it writes the current output instead.
func snapshot(t *testing.T, name string, got any) {
	t.Helper()
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := testdataDir()
	golden := filepath.Join(dir, name+".golden.json")
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	want, readErr := os.ReadFile(golden)
	if readErr != nil || update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if update {
			t.Logf("updated golden: %s", golden)
		} else {
			t.Logf("wrote new golden: %s - re-run to verify", golden)
		}
		return
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(b)) {
		t.Errorf("snapshot drift for %s.\nWANT:\n%s\nGOT:\n%s", name, want, b)
	}
}

// plansToMap normalises []PlannedRequest into a stable map shape for
// snapshotting, keeping field order deterministic and letting us strip future
// fields without rewriting every golden.
func plansToMap(plans []common.PlannedRequest) map[string]any {
	arr := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		arr = append(arr, map[string]any{
			"method": p.Method,
			"path":   p.Path,
			"query":  p.Query,
			"body":   p.Body,
		})
	}
	return map[string]any{"plans": arr}
}

// helpFor mounts the themes shortcuts onto a fresh root cobra command (each
// under its own Service path), triggers `--help` for the supplied command
// path, and returns the captured help output. A zero-valued Factory is safe because `--help`
// short-circuits before cobra reaches RunE.
func helpFor(t *testing.T, cmdPath ...string) string {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza"}
	f := &cmdutil.Factory{}
	for _, s := range Shortcuts() {
		// Mount under the shortcut's own Service path (e.g. "themes block").
		parent := root
		for _, seg := range strings.Fields(s.Service) {
			var child *cobra.Command
			for _, c := range parent.Commands() {
				if c.Name() == seg {
					child = c
					break
				}
			}
			if child == nil {
				child = &cobra.Command{Use: seg}
				parent.AddCommand(child)
			}
			parent = child
		}
		common.Mount(s, parent, f)
	}
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	args := append(append([]string{}, cmdPath...), "--help")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("root.Execute(%v) returned error: %v", args, err)
	}
	return buf.String()
}

// shortcutFlags binds a shortcut's own flag declarations the way Mount does —
// same names, same defaults — then sets vals on the result.
func shortcutFlags(t *testing.T, sc common.Shortcut, vals map[string]any) common.FlagSet {
	t.Helper()
	parent := &cobra.Command{Use: "parent"}
	common.Mount(sc, parent, &cmdutil.Factory{})
	cmd := parent.Commands()[0]
	for k, v := range vals {
		if err := cmd.Flags().Set(k, fmt.Sprint(v)); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	return common.NewCobraFlagSet(cmd)
}

// flagsSection returns the "Flags:" part of a help output.
func flagsSection(out string) string {
	if i := strings.Index(out, "Flags:"); i >= 0 {
		return out[i:]
	}
	return ""
}

// captureStderr swaps os.Stderr for a pipe while fn runs, returns everything
// written. Restores os.Stderr on exit so other tests are unaffected.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stderr = old
	}()
	fn()
	_ = w.Close()
	return <-done
}
