package orders

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// newOrderExecInput builds an ExecInput via a cobra command (for Execute shortcuts).
func newOrderExecInput(t *testing.T, flags map[string]string, values map[string]string, dryRun bool) common.ExecInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "bool":
			cmd.Flags().Bool(name, false, "")
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

// newOrderPlanInput builds a PlanInput via a cobra command.
func newOrderPlanInput(t *testing.T, tool string, flags map[string]string, values map[string]string) common.PlanInput {
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

// ── searchShortcut.Plan ───────────────────────────────────────────────────────

var orderSearchFlags = map[string]string{
	"keyword": "string", "status": "string",
	"financial-status": "string", "fulfillment-status": "string",
	"customer-id": "string", "since": "string", "until": "string",
	"page-limit": "int",
}

func TestOrderSearchPlan_DefaultsSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "search", orderSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrderSearchPlan_WithFiltersSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "search", orderSearchFlags, map[string]string{
		"status": "placed", "page-limit": "5",
	})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── countShortcut.Plan ────────────────────────────────────────────────────────

var orderCountFlags = map[string]string{
	"status": "string", "financial-status": "string",
	"fulfillment-status": "string", "since": "string", "until": "string",
}

func TestOrderCountPlan_DefaultsSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "count", orderCountFlags, nil)
	_, err := countShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrderCountPlan_WithStatusSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "count", orderCountFlags, map[string]string{"status": "placed"})
	_, err := countShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── updateTrackingShortcut.Plan ───────────────────────────────────────────────

var updateTrackingFlags = map[string]string{
	"order-id": "string", "fulfillment-id": "string",
	"tracking": "string", "company": "string",
	"tracking-url": "string", "notify": "bool",
}

func TestUpdateTrackingPlan_BasicSuccess(t *testing.T) {
	in := newOrderPlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123",
	})
	_, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateTrackingPlan_WithNotifySuccess(t *testing.T) {
	in := newOrderPlanInput(t, "update-tracking", updateTrackingFlags, map[string]string{
		"order-id": "ord-1", "fulfillment-id": "ful-1", "tracking": "TRK123", "notify": "true",
	})
	_, err := updateTrackingShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── refundShortcut.Execute (dry-run) ──────────────────────────────────────────

var refundExecFlags = map[string]string{
	"order-id": "string", "amount": "string",
	"payment-line-id": "string", "note": "string", "return-items": "bool",
}

func TestRefundExecute_DryRunReturnsTwoPlans(t *testing.T) {
	in := newOrderExecInput(t, refundExecFlags, map[string]string{
		"order-id": "ord-1", "amount": "10.00",
	}, true)
	result, err := refundShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans (GET + POST), got %d", len(result.Plans))
	}
}

// ── shipShortcut.Execute (dry-run) ────────────────────────────────────────────

var shipExecFlags = map[string]string{
	"order-id": "string", "tracking": "string", "company": "string",
	"company-code": "string", "line-items": "string", "notify": "bool",
}

func TestShipExecute_DryRunReturnsTwoPlans(t *testing.T) {
	in := newOrderExecInput(t, shipExecFlags, map[string]string{
		"order-id": "ord-1", "tracking": "TRK123",
	}, true)
	result, err := shipShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans (GET + POST), got %d", len(result.Plans))
	}
}

func TestShipExecute_DryRunWithNotify(t *testing.T) {
	in := newOrderExecInput(t, shipExecFlags, map[string]string{
		"order-id": "ord-1", "tracking": "TRK123", "notify": "true",
	}, true)
	result, err := shipShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans, got %d", len(result.Plans))
	}
}
