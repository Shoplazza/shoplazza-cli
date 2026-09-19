package cmdutil

import (
	"bytes"
	"testing"
)

// A non-interactive factory (buffer streams, no terminal) must never prompt for
// a destructive confirmation — agents/pipes/CI proceed unchanged. This pins the
// human-only safety property; the interactive branch renders via huh and is
// covered through the callers' injected prompters.
func TestConfirmDestructive_NonInteractive_ReturnsNil(t *testing.T) {
	f := &Factory{IOStreams: IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}}
	if err := ConfirmDestructive(f, "Delete everything? This cannot be undone."); err != nil {
		t.Errorf("non-interactive ConfirmDestructive must return nil (never block), got %v", err)
	}
}
