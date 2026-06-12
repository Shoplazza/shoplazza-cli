package jsbuild

import (
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestDecodeBuildResult_Success(t *testing.T) {
	res, exitErr := decodeBuildResult([]byte(`{"ok":true,"artifacts":["dist/demo.abc.js"],"durationMs":42}` + "\n"))
	if exitErr != nil {
		t.Fatalf("unexpected error: %v", exitErr)
	}
	if !res.OK || len(res.Artifacts) != 1 || res.Artifacts[0] != "dist/demo.abc.js" {
		t.Fatalf("bad result: %+v", res)
	}
}

func TestDecodeBuildResult_OKFalse(t *testing.T) {
	_, exitErr := decodeBuildResult([]byte(`{"ok":false,"error":{"type":"internal","code":"build_error","message":"boom"}}`))
	if exitErr == nil {
		t.Fatal("expected an ExitError")
	}
	if exitErr.Code != output.ExitInternal || exitErr.Detail.Type != output.TypeInternal {
		t.Errorf("got code=%d type=%q, want internal", exitErr.Code, exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "boom") {
		t.Errorf("message %q should contain the Node error", exitErr.Detail.Message)
	}
}

func TestDecodeBuildResult_UnresolvedImport_HintsNpmInstall(t *testing.T) {
	// Verbatim Rollup failure shape for a scaffolded project that never ran
	// `npm install` (shoplazza-extension-ui is declared but absent).
	stdout := `{"ok":false,"error":{"type":"internal","code":"build_error","message":"[vite]: Rollup failed to resolve import \"shoplazza-extension-ui\" from \"/p/extensions/ccc/src/index.js\".\nThis is most likely unintended because it can break your application at runtime."}}`
	_, exitErr := decodeBuildResult([]byte(stdout))
	if exitErr == nil {
		t.Fatal("expected an ExitError")
	}
	if exitErr.Code != output.ExitValidation || exitErr.Detail.Type != output.TypeValidation {
		t.Errorf("got code=%d type=%q, want validation (missing deps is a user-fixable state)", exitErr.Code, exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "shoplazza-extension-ui") {
		t.Errorf("message %q should keep the unresolved import name", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "npm install") {
		t.Errorf("hint %q should tell the user to run npm install", exitErr.Detail.Hint)
	}
}

func TestDecodeBuildResult_OtherBuildError_StaysInternal(t *testing.T) {
	// Only unresolved-import failures get the npm-install reinterpretation;
	// ordinary build errors must keep the v1-parity internal mapping.
	_, exitErr := decodeBuildResult([]byte(`{"ok":false,"error":{"type":"internal","code":"build_error","message":"Unexpected token (3:7) in /p/extensions/ccc/src/index.js"}}`))
	if exitErr == nil || exitErr.Detail.Type != output.TypeInternal || exitErr.Code != output.ExitInternal {
		t.Fatalf("non-resolve build errors must stay type=internal, got %v", exitErr)
	}
	if exitErr.Detail.Hint != "" {
		t.Errorf("unexpected hint %q on an ordinary build error", exitErr.Detail.Hint)
	}
}

func TestDecodeBuildResult_InvalidJSON(t *testing.T) {
	_, exitErr := decodeBuildResult([]byte("Segmentation fault\n"))
	if exitErr == nil || exitErr.Detail.Type != output.TypeInternal {
		t.Fatalf("invalid JSON must normalize to type=internal, got %v", exitErr)
	}
}
