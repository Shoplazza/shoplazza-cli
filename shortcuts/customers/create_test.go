package customers

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

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
	for _, c := range []struct {
		noMarketing bool
		want        map[string]any
	}{
		{false, map[string]any{"accepts_marketing": true, "accepts_sms_marketing": nil}},
		{true, map[string]any{"accepts_marketing": false, "accepts_sms_marketing": false}},
	} {
		body, _ := buildCreateCustomerBody("a@b.com", "", "", "", nil, c.noMarketing)
		customer, _ := body["customer"].(map[string]any)
		for field, want := range c.want {
			if got, sent := customer[field]; got != want || sent != (want != nil) {
				t.Errorf("--no-marketing=%v: %s = %v (sent %v), want %v", c.noMarketing, field, got, sent, want)
			}
		}
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
	in := shortcuttest.PlanInput(t, "create", createShortcutFlags, nil)
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error when neither --email nor --phone provided")
	}
}

func TestCreateShortcutPlan_EmailSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "create", createShortcutFlags, map[string]string{"email": "a@b.com"})
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
	in := shortcuttest.PlanInput(t, "search", searchShortcutFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchShortcutPlan_WithEmailSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", searchShortcutFlags, map[string]string{"email": "a@b.com", "page-limit": "10", "since": "2026-09-01", "until": "2026-09-30 23:59:59"})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Query["created_at_min"] != "2026-09-01T00:00:00Z" || p.Query["created_at_max"] != "2026-09-30T23:59:59Z" {
		t.Errorf("--since/--until must reach created_at_* as RFC3339 UTC, got %v / %v", p.Query["created_at_min"], p.Query["created_at_max"])
	}
}
