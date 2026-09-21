package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func mustQuery(t *testing.T, expr string) *gojq.Query {
	t.Helper()
	q, err := gojq.Parse(expr)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	return q
}

// runJQ reports produced-count and all-null so applyJQ can decide whether to hint.
func TestRunJQ_Detection(t *testing.T) {
	cases := []struct {
		name         string
		expr         string
		input        any
		wantProduced int
		wantAllNull  bool
	}{
		{"value", ".a", map[string]any{"a": 1}, 1, false},
		{"missing key is null", ".missing", map[string]any{}, 1, true},
		{"nested missing is null", ".request.path", map[string]any{"ok": true, "data": map[string]any{}}, 1, true},
		{"empty iteration yields nothing", ".list[]", map[string]any{"list": []any{}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			produced, allNull, err := runJQ(&b, mustQuery(t, tc.expr), tc.input)
			if err != nil {
				t.Fatalf("runJQ: %v", err)
			}
			if produced != tc.wantProduced || allNull != tc.wantAllNull {
				t.Errorf("produced=%d allNull=%v, want %d / %v", produced, allNull, tc.wantProduced, tc.wantAllNull)
			}
		})
	}
}

// The hint names both common causes: data under .data, and .request being dry-run-only.
func TestJQEmptyHintLine(t *testing.T) {
	line := jqEmptyHintLine(".request.path", false)
	for _, want := range []string{"hint:", ".request.path", "matched nothing", ".data", "--dry-run"} {
		if !strings.Contains(line, want) {
			t.Errorf("hint missing %q:\n%s", want, line)
		}
	}
}

// The hint must never reach a non-terminal writer — machine consumers (piped
// stderr, CI) see nothing, preserving the jq output contract.
func TestWriteJQEmptyHint_NonTTYSilent(t *testing.T) {
	var b bytes.Buffer // a buffer is never a terminal
	writeJQEmptyHint(&b, ".request.path")
	if b.Len() != 0 {
		t.Errorf("hint leaked to a non-terminal writer: %q", b.String())
	}
}
