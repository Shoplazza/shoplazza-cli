package dynamic

import (
	"sort"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/registry"
)

func cmd(path ...string) registry.Command {
	return registry.Command{
		ID:   strings.Join(path, "-"),
		Path: path,
		HTTP: registry.HTTP{Method: "GET", Path: "/x"},
	}
}

func kept(in []registry.Command) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, strings.Join(c.Path, "/"))
	}
	sort.Strings(out)
	return out
}

func TestFilterCommands_DropsDuplicates(t *testing.T) {
	got := kept(filterCommands([]registry.Command{
		cmd("list"),
		cmd("list"),
		cmd("get"),
	}))
	want := []string{"get"}
	if !equalStringSlice(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterCommands_DropsInvalid(t *testing.T) {
	bad := registry.Command{ID: "bad", Path: []string{"NotKebab"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}}
	got := kept(filterCommands([]registry.Command{cmd("list"), bad}))
	want := []string{"list"}
	if !equalStringSlice(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterCommands_AdjacentPrefixConflictDropsBoth(t *testing.T) {
	got := kept(filterCommands([]registry.Command{
		cmd("list"),
		cmd("list", "foo"),
		cmd("get"),
	}))
	want := []string{"get"}
	if !equalStringSlice(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Regression: the shorter path conflicts with a non-adjacent longer path
// (the lex-sort-adjacent approach would miss this; the prefix-set approach
// must still drop both).
func TestFilterCommands_NonAdjacentPrefixConflict(t *testing.T) {
	// Sorted lex order would be: a/b, a/b/c/d, a/b/x. The (a/b ↔ a/b/x)
	// conflict is non-adjacent in that ordering.
	got := kept(filterCommands([]registry.Command{
		cmd("a", "b"),
		cmd("a", "b", "c", "d"),
		cmd("a", "b", "x"),
		cmd("safe"),
	}))
	want := []string{"safe"}
	if !equalStringSlice(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterCommands_KeepsSiblingsWithoutPrefixRelation(t *testing.T) {
	got := kept(filterCommands([]registry.Command{
		cmd("variants", "create"),
		cmd("variants", "delete"),
		cmd("images", "create"),
	}))
	want := []string{"images/create", "variants/create", "variants/delete"}
	if !equalStringSlice(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
