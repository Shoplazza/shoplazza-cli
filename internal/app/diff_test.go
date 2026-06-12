package app

import (
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestDiff_MatchesByExtensionID(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "local-name", Type: "theme", ExtensionID: "e1"}}
	remotes := []Extension{{ExtensionID: "e1", ExtensionName: "remote-name", ExtensionType: "theme"}}
	pairs, ex := Diff(locals, remotes)
	if ex != nil {
		t.Fatalf("unexpected error: %v", ex)
	}
	if len(pairs) != 1 || pairs[0].Remote == nil || pairs[0].Remote.ExtensionID != "e1" {
		t.Fatalf("pairs = %+v", pairs)
	}
}

func TestDiff_MatchesByNameType(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "a", Type: "checkout"}}
	remotes := []Extension{{ExtensionID: "r1", ExtensionName: "a", ExtensionType: "checkout"}}
	pairs, ex := Diff(locals, remotes)
	if ex != nil || pairs[0].Remote == nil || pairs[0].Remote.ExtensionID != "r1" {
		t.Fatalf("pairs = %+v, ex=%v", pairs, ex)
	}
}

func TestDiff_SingleSingleSameType(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "x", Type: "function"}}
	remotes := []Extension{{ExtensionID: "r1", ExtensionName: "y", ExtensionType: "function"}}
	pairs, ex := Diff(locals, remotes)
	if ex != nil || pairs[0].Remote == nil || pairs[0].Remote.ExtensionID != "r1" {
		t.Fatalf("pairs = %+v, ex=%v", pairs, ex)
	}
}

func TestDiff_NewExtension_NoRemote(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "brand-new", Type: "theme"}}
	pairs, ex := Diff(locals, nil)
	if ex != nil {
		t.Fatalf("unexpected error: %v", ex)
	}
	if len(pairs) != 1 || pairs[0].Remote != nil {
		t.Fatalf("expected new (nil remote), got %+v", pairs)
	}
}

func TestDiff_AmbiguousSameType_Validation(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "x", Type: "function"}, {Dir: "b", Name: "z", Type: "function"}}
	remotes := []Extension{
		{ExtensionID: "r1", ExtensionName: "p", ExtensionType: "function"},
		{ExtensionID: "r2", ExtensionName: "q", ExtensionType: "function"},
	}
	_, ex := Diff(locals, remotes)
	if ex == nil || ex.Detail == nil || ex.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error, got %v", ex)
	}
}

func TestDiff_OneLocalTwoRemotes_Ambiguous(t *testing.T) {
	locals := []LocalExt{{Dir: "a", Name: "x", Type: "function"}}
	remotes := []Extension{
		{ExtensionID: "r1", ExtensionName: "p", ExtensionType: "function"},
		{ExtensionID: "r2", ExtensionName: "q", ExtensionType: "function"},
	}
	_, ex := Diff(locals, remotes)
	if ex == nil {
		t.Fatal("expected ambiguity error for 1 local vs 2 unmatched remotes")
	}
}
