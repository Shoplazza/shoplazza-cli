package common_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

	"github.com/spf13/cobra"
)

func TestExecInput_FieldsCompile(t *testing.T) {
	in := common.ExecInput{
		Args:   []string{},
		Flags:  common.NewCobraFlagSet(nil),
		Tool:   "thing",
		Client: &client.Client{},
		DryRun: false,
	}
	if in.Tool != "thing" {
		t.Fatalf("Tool: got %q want %q", in.Tool, "thing")
	}
}

func TestExecResult_FieldsCompile(t *testing.T) {
	r := common.ExecResult{
		Plans: []common.PlannedRequest{{Method: "GET", Path: "/x"}},
		Body:  map[string]any{"ok": true},
	}
	if len(r.Plans) != 1 {
		t.Fatalf("Plans len: got %d want 1", len(r.Plans))
	}
}

func TestShortcut_ExecuteFieldAccepted(t *testing.T) {
	_ = common.Shortcut{
		Service: "svc",
		Command: "+x",
		Use:     "+x",
		Short:   "x",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
}

func TestEngine_ExecutePathWraps_Body_Envelope(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")

	executed := false
	s := common.Shortcut{
		Service: "svc",
		Command: "+go",
		Use:     "+go",
		Short:   "go",
		Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
			executed = true
			if in.Client == nil {
				t.Fatal("ExecInput.Client is nil; engine must inject it")
			}
			if in.Tool != "go" {
				t.Errorf("Tool: got %q want %q", in.Tool, "go")
			}
			if in.DryRun {
				t.Error("DryRun should be false in non-dry-run mode")
			}
			return common.ExecResult{Body: map[string]any{"hello": "world"}}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var sub *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "+go" {
			sub = c
		}
	}
	if sub == nil {
		t.Fatal("expected +go subcommand mounted under svc")
	}
	var buf bytes.Buffer
	sub.SetOut(&buf)
	sub.SetErr(&buf)
	parent.SetArgs([]string{"+go"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !executed {
		t.Fatal("Execute function was not called")
	}
	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\nraw: %s", err, buf.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok || data["hello"] != "world" {
		t.Errorf("expected data.hello=world in output; got: %s", buf.String())
	}
}

func TestValidateShortcut_ExecuteOnlyAccepted(t *testing.T) {
	s := common.Shortcut{
		Service: "svc",
		Command: "+x",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
	if err := common.ValidateShortcut(s); err != nil {
		t.Fatalf("Execute-only shortcut should validate; got: %v", err)
	}
}

func TestValidateShortcut_NeitherPlanNorExecuteRejected(t *testing.T) {
	s := common.Shortcut{Service: "svc", Command: "+x"}
	if err := common.ValidateShortcut(s); err == nil {
		t.Fatal("expected error when both Plan and Execute are nil")
	}
}

func TestValidateShortcut_BothPlanAndExecuteRejected(t *testing.T) {
	s := common.Shortcut{
		Service: "svc",
		Command: "+x",
		Plan: func(_ common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{}, nil
		},
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
	if err := common.ValidateShortcut(s); err == nil {
		t.Fatal("expected error when both Plan and Execute are set")
	}
}

func TestEngine_ExecutePath_DryRunRendersPlans(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")

	s := common.Shortcut{
		Service: "svc",
		Command: "+multi",
		Use:     "+multi",
		Short:   "multi",
		Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
			if !in.DryRun {
				t.Error("DryRun should be true when --dry-run is set")
			}
			return common.ExecResult{Plans: []common.PlannedRequest{
				{Method: "GET", Path: "/a"},
				{Method: "POST", Path: "/b"},
			}}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var sub *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "+multi" {
			sub = c
		}
	}
	var buf bytes.Buffer
	sub.SetOut(&buf)
	sub.SetErr(&buf)
	parent.SetArgs([]string{"+multi", "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\nraw: %s", err, buf.String())
	}
	if env["dry_run"] != true {
		t.Errorf("dry_run: got %v want true", env["dry_run"])
	}
	reqs, ok := env["requests"].([]any)
	if !ok {
		t.Fatalf("envelope.requests not an array; got: %v", env["requests"])
	}
	if len(reqs) != 2 {
		t.Errorf("requests len: got %d want 2", len(reqs))
	}
}
