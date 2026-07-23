package customers

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

// newCustomerPlanInput builds a PlanInput backed by a cobra command.
// flags maps flag-name → type; values maps flag-name → string value.
func newCustomerPlanInput(t *testing.T, tool string, flags map[string]string, values map[string]string) common.PlanInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "bool":
			cmd.Flags().Bool(name, false, "")
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
	return common.PlanInput{Tool: tool, Flags: common.NewCobraFlagSet(cmd)}
}

func TestCreateShortcut_DeclarativeShape(t *testing.T) {
	if createShortcut.Service != "customers" || createShortcut.Command != "+create" {
		t.Errorf("identity wrong: %q %q", createShortcut.Service, createShortcut.Command)
	}
	if createShortcut.Plan == nil {
		t.Fatal("+create should be single-step Plan")
	}
	if err := common.ValidateShortcut(createShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestBuildCreateCustomerBody_EmailSetsContactType(t *testing.T) {
	body, err := buildCreateCustomerBody("a@b.com", "", "Alice", "", nil, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, _ := body["customer"].(map[string]any)
	if c["contact_type"] != "email" {
		t.Errorf("contact_type: got %v want email", c["contact_type"])
	}
	if c["email"] != "a@b.com" {
		t.Errorf("email: got %v", c["email"])
	}
}

func TestBuildCreateCustomerBody_PhoneSetsContactType(t *testing.T) {
	body, err := buildCreateCustomerBody("", "+1-555", "", "", nil, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, _ := body["customer"].(map[string]any)
	if c["contact_type"] != "phone" {
		t.Errorf("contact_type: got %v want phone", c["contact_type"])
	}
}

func TestBuildCreateCustomerBody_BothRejected(t *testing.T) {
	_, err := buildCreateCustomerBody("a@b.com", "+1-555", "", "", nil, false)
	if err == nil {
		t.Fatal("expected ExactlyOne validation error")
	}
}

func TestBuildCreateCustomerBody_NeitherRejected(t *testing.T) {
	_, err := buildCreateCustomerBody("", "", "", "", nil, false)
	if err == nil {
		t.Fatal("expected ExactlyOne validation error")
	}
}

func TestBuildCreateCustomerBody_NoMarketingFlipsDefault(t *testing.T) {
	body, _ := buildCreateCustomerBody("a@b.com", "", "", "", nil, true)
	c, _ := body["customer"].(map[string]any)
	if c["accepts_marketing"] != false {
		t.Errorf("accepts_marketing: got %v want false (--no-marketing)", c["accepts_marketing"])
	}
}

func TestBuildCreateCustomerBody_WithTags(t *testing.T) {
	tags := []string{"vip", "wholesale"}
	body, err := buildCreateCustomerBody("a@b.com", "", "Bob", "", tags, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c, _ := body["customer"].(map[string]any)
	got, _ := c["tags"].([]string)
	if len(got) != 2 || got[0] != "vip" {
		t.Errorf("tags: got %v want [vip wholesale]", got)
	}
}

// ── createShortcut.Plan ───────────────────────────────────────────────────────

var createShortcutFlags = map[string]string{
	"email": "string", "phone": "string",
	"first-name": "string", "last-name": "string",
	"tags": "stringslice", "no-marketing": "bool",
}

func TestCreateShortcutPlan_NoEmailOrPhoneErrors(t *testing.T) {
	in := newCustomerPlanInput(t, "create", createShortcutFlags, nil)
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when neither --email nor --phone provided")
	}
}

func TestCreateShortcutPlan_EmailSuccess(t *testing.T) {
	in := newCustomerPlanInput(t, "create", createShortcutFlags, map[string]string{"email": "a@b.com"})
	_, err := createShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── searchShortcut.Plan ───────────────────────────────────────────────────────

var searchShortcutFlags = map[string]string{
	"email": "string", "phone": "string",
	"since": "string", "until": "string",
	"page-limit": "int", "fields": "stringslice",
}

func TestSearchShortcutPlan_DefaultsSuccess(t *testing.T) {
	in := newCustomerPlanInput(t, "search", searchShortcutFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchShortcutPlan_WithEmailSuccess(t *testing.T) {
	in := newCustomerPlanInput(t, "search", searchShortcutFlags, map[string]string{"email": "a@b.com", "page-limit": "10"})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
