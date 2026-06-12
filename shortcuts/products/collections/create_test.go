package collections

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"shoplazza-cli-v2/shortcuts/common"
)

// newCollExecInput builds an ExecInput via a cobra command for dry-run tests.
func newCollExecInput(t *testing.T, flags map[string]string, values map[string]string, dryRun bool) common.ExecInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "stringslice":
			cmd.Flags().StringSlice(name, nil, "")
		}
	}
	var args []string
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.ExecInput{Flags: common.NewCobraFlagSet(cmd), DryRun: dryRun}
}

func TestCreateShortcut_DeclarativeShape(t *testing.T) {
	if createShortcut.Service != "products collections" {
		t.Errorf("Service: got %q want %q", createShortcut.Service, "products collections")
	}
	if createShortcut.Command != "+create" {
		t.Errorf("Command: got %q want %q", createShortcut.Command, "+create")
	}
	if createShortcut.Execute == nil {
		t.Fatal("+create requires Execute (handles conditional batch association)")
	}
	if err := common.ValidateShortcut(createShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestSortOrderAliasToAPI(t *testing.T) {
	cases := map[string]string{
		"manual":       "manual",
		"best-selling": "sales-desc",
		"price-asc":    "price-asc",
		"price-desc":   "price-desc",
		"newest":       "created-desc",
		"popular":      "views-desc",
		"intelligent":  "intelligent",
	}
	for cli, want := range cases {
		got := sortOrderAliasToAPI(cli)
		if got != want {
			t.Errorf("%q: got %q want %q", cli, got, want)
		}
	}
}

func TestSortOrderAliasToAPI_UnknownPassThrough(t *testing.T) {
	got := sortOrderAliasToAPI("custom-value")
	if got != "custom-value" {
		t.Errorf("unknown values should pass through; got %q", got)
	}
}

// ── createShortcut.Execute (dry-run) ──────────────────────────────────────────

var collCreateFlags = map[string]string{
	"title": "string", "description": "string",
	"image": "string", "sort-order": "string",
	"product-ids": "stringslice",
}

func TestCreateShortcutExecute_DryRunNoProductIDs(t *testing.T) {
	in := newCollExecInput(t, collCreateFlags, map[string]string{"title": "Summer"}, true)
	result, err := createShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(result.Plans))
	}
}

func TestCreateShortcutExecute_DryRunWithProductIDs(t *testing.T) {
	in := newCollExecInput(t, collCreateFlags, map[string]string{
		"title": "Winter", "product-ids": "p1,p2",
	}, true)
	result, err := createShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans (create + batch), got %d", len(result.Plans))
	}
}

func TestCreateShortcutExecute_DryRunWithSortOrder(t *testing.T) {
	in := newCollExecInput(t, collCreateFlags, map[string]string{
		"title": "Spring", "sort-order": "best-selling",
	}, true)
	result, err := createShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(result.Plans))
	}
}
