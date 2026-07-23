package dynamic

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/registry"
)

func TestBuildLeaf_NoBodyForGET(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"list"},
		HTTP: registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule")
	if leaf.Flags().Lookup("data") != nil {
		t.Fatal("GET must not register --data")
	}
	if leaf.Flags().Lookup("params") == nil {
		t.Fatal("--params is always present")
	}
	// --dry-run is a local leaf flag (not inherited from a module group),
	// so it appears in each spec leaf's own --help "Flags" block and module
	// group --help stays clean.
	if leaf.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run must be registered as a local flag on every leaf")
	}
}

func TestBuildLeaf_NoBodyForBlankBodyMarker(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"x"},
		HTTP: registry.HTTP{Method: "POST", Path: "/x"}, // no Body marker
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule")
	if leaf.Flags().Lookup("data") != nil {
		t.Fatal("POST without http.body=='*' must not register --data")
	}
}

func TestBuildLeaf_BodyForPOST(t *testing.T) {
	cmd := registry.Command{
		Path: []string{"create"},
		HTTP: registry.HTTP{Method: "POST", Path: "/x", Body: "*"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule")
	if leaf.Flags().Lookup("data") == nil {
		t.Fatal("POST with body=='*' must register --data")
	}
}

func TestBuildLeaf_HiddenPropagated(t *testing.T) {
	cmd := registry.Command{
		Path:   []string{"x"},
		Hidden: true,
		HTTP:   registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule")
	if !leaf.Hidden {
		t.Fatal("hidden flag must propagate to cobra leaf")
	}
}

func TestBuildLeaf_SummaryFallsBackToID(t *testing.T) {
	cmd := registry.Command{
		ID:   "some-operation-id",
		Path: []string{"x"},
		HTTP: registry.HTTP{Method: "GET", Path: "/x"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, &cmdutil.Factory{}, "testmodule")
	if leaf.Short != "some-operation-id" {
		t.Fatalf("short = %q, want id fallback", leaf.Short)
	}
}
