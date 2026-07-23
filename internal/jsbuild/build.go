package jsbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// BuildRequest is the single-line JSON sent to the Node entry over stdin.
type BuildRequest struct {
	Action string `json:"action"` // always "build"
	Name   string `json:"name"`   // local extension directory name (the dir under ./extensions)
	Debug  bool   `json:"debug"`
}

// BuildResult is the single-line JSON the Node entry writes to stdout.
type BuildResult struct {
	OK         bool     `json:"ok"`
	Artifacts  []string `json:"artifacts"`
	DurationMs int64    `json:"durationMs"`
	Error      *struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeBuildResult parses Node's stdout and normalizes failures into an
// *output.ExitError. Build-class failures map to type=internal, except
// unresolved bare imports, which mean the scaffolded project's npm dependencies
// were never installed — a user-fixable state → type=validation.
func decodeBuildResult(stdout []byte) (*BuildResult, *output.ExitError) {
	var r BuildResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &r); err != nil {
		return nil, output.ErrInternal("checkout build: unparseable response from build subprocess: %s", err.Error())
	}
	if !r.OK {
		msg := "checkout build failed"
		if r.Error != nil && r.Error.Message != "" {
			msg = r.Error.Message
		}
		if strings.Contains(msg, "failed to resolve import") {
			return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation, msg,
				"run `npm install` in the project root to install extension dependencies (the project uses npm workspaces)")
		}
		return nil, output.ErrInternal("%s", msg)
	}
	return &r, nil
}

// RunBuild spawns the Node build entry, writes req on its stdin, and returns the
// decoded result. The subprocess inherits the user's cwd (cmd.Dir) so its path
// logic matches process.cwd(). require('vite') resolves from the entry file
// location, independent of cwd.
func RunBuild(ctx context.Context, req BuildRequest, userCwd string) (*BuildResult, *output.ExitError) {
	pkgRoot, err := PkgRoot()
	if err != nil {
		return nil, output.ErrInternal("cannot resolve package root: %s", err.Error())
	}
	nodePath, exitErr := EnsureNode(ctx) // existence + numeric version gate (shared with dev)
	if exitErr != nil {
		return nil, exitErr
	}
	if exitErr := EnsureProjectDeps(ctx, userCwd); exitErr != nil { // first-build auto `npm install`
		return nil, exitErr
	}

	payload, _ := json.Marshal(req)
	cmd := exec.CommandContext(ctx, nodePath, BuildEntryPath(pkgRoot))
	cmd.Dir = userCwd
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	cmd.Stderr = os.Stderr // pass Node's verbose/debug logs straight through
	stdout, runErr := cmd.Output()
	if runErr != nil {
		// Non-zero exit / crash: Node still prints a JSON error line to stdout on
		// handled failures; try to decode it, else normalize the raw stdout.
		if len(bytes.TrimSpace(stdout)) > 0 {
			if _, decErr := decodeBuildResult(stdout); decErr != nil {
				return nil, decErr
			}
		}
		return nil, output.ErrInternal("checkout build subprocess failed: %s", runErr.Error())
	}
	return decodeBuildResult(stdout)
}

// EnsureNode locates `node` and verifies its version is >= 14.18.0. Both
// `checkout build` and `checkout dev` MUST call this before spawning (spec).
func EnsureNode(ctx context.Context) (string, *output.ExitError) {
	nodePath, exitErr := NodePath()
	if exitErr != nil {
		return "", exitErr
	}
	ok, vErr := nodeVersionMeetsFloorFromBinary(ctx, nodePath)
	if vErr != nil {
		return "", vErr
	}
	if !ok {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"installed Node.js is older than the required 14.18.0",
			"upgrade Node.js to >= 14.18.0 (https://nodejs.org)")
	}
	return nodePath, nil
}

// nodeVersionMeetsFloorFromBinary runs `node --version` and applies the numeric gate.
func nodeVersionMeetsFloorFromBinary(ctx context.Context, nodePath string) (bool, *output.ExitError) {
	out, err := exec.CommandContext(ctx, nodePath, "--version").Output()
	if err != nil {
		return false, output.ErrValidation("could not determine Node.js version: %s", err.Error())
	}
	ok, parseErr := nodeVersionMeetsFloor(string(out))
	if parseErr != nil {
		return false, output.ErrValidation("could not parse Node.js version: %s", parseErr.Error())
	}
	return ok, nil
}
