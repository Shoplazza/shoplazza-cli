package common_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// newFakeFactory returns a Factory whose Client is harmless for dry-run
// rendering (BuildRequestSummary only reads method/path/query/body).
func newFakeFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	return &cmdutil.Factory{Client: &client.Client{}}
}

func TestEngine_MountSetsUseShortArgs(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	s := common.Shortcut{
		Service: "svc",
		Command: "+thing",
		Use:     "+thing <id>",
		Short:   "do a thing",
		Args:    cobra.ExactArgs(1),
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var sub *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "+thing" {
			sub = c
		}
	}
	if sub == nil {
		t.Fatal("expected +thing subcommand mounted under svc")
	}
	if sub.Use != "+thing <id>" {
		t.Errorf("Use: got %q want %q", sub.Use, "+thing <id>")
	}
	if sub.Short != "do a thing" {
		t.Errorf("Short: got %q want %q", sub.Short, "do a thing")
	}
	if sub.Args == nil {
		t.Error("Args is nil; expected cobra.ExactArgs(1)")
	}
}

func TestEngine_DryRunRendersPlannedRequestEnvelope(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")
	s := common.Shortcut{
		Service: "svc",
		Command: "+probe",
		Use:     "+probe",
		Short:   "probe",
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{Method: "GET", Path: "/probe"}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var sub *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "+probe" {
			sub = c
		}
	}
	var buf bytes.Buffer
	sub.SetOut(&buf)
	sub.SetErr(&buf)
	parent.SetArgs([]string{"+probe", "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run output: %v\nraw: %s", err, buf.String())
	}
	if env["dry_run"] != true {
		t.Errorf("dry_run flag: got %v want true; envelope=%v", env["dry_run"], env)
	}
}

func TestEngine_ToolNameStripsLeadingPlus(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")

	var seenTool string
	s := common.Shortcut{
		Service: "svc",
		Command: "+thing-x",
		Use:     "+thing-x",
		Short:   "x",
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			seenTool = in.Tool
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))
	parent.SetArgs([]string{"+thing-x", "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if seenTool != "thing-x" {
		t.Errorf("Tool: got %q want %q", seenTool, "thing-x")
	}
}

func TestEngine_FlagDefaultTypeMismatchPanicsAtMount(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	s := common.Shortcut{
		Service: "svc",
		Command: "+bad",
		Use:     "+bad",
		Short:   "bad",
		Flags: []common.Flag{
			{Name: "n", Type: common.FlagInt, Default: "not-an-int"},
		},
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic from incompatible Default; got none")
		}
	}()
	common.Mount(s, parent, newFakeFactory(t))
}

// runMounted mounts s under a fresh parent, executes it with args, and
// returns the command's stdout.
func runMounted(t *testing.T, s common.Shortcut, args ...string) string {
	t.Helper()
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")
	common.Mount(s, parent, newFakeFactory(t))
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetArgs(args)
	if err := parent.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return buf.String()
}

// TestEngine_LocalShortcutPrintsRawBody: Local shortcuts report local
// artifacts, not API responses — live output must be the raw body (PrintBody),
// never the {ok,data} API success envelope.
func TestEngine_LocalShortcutPrintsRawBody(t *testing.T) {
	s := common.Shortcut{
		Service: "svc",
		Command: "+local",
		Use:     "+local",
		Short:   "local",
		Local:   true,
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{Body: map[string]any{"zip_path": "/tmp/x.zip"}}, nil
		},
	}
	out := runMounted(t, s, "+local")
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode output: %v\nraw: %s", err, out)
	}
	if _, hasOK := env["ok"]; hasOK {
		t.Errorf("Local shortcut must not emit the {ok,data} envelope; got %v", env)
	}
	if env["zip_path"] != "/tmp/x.zip" {
		t.Errorf("body fields must be at the top level; got %v", env)
	}
}

// TestEngine_NonLocalExecuteKeepsAPISuccessEnvelope: the default rendering
// for Execute shortcuts stays {ok,data}.
func TestEngine_NonLocalExecuteKeepsAPISuccessEnvelope(t *testing.T) {
	s := common.Shortcut{
		Service: "svc",
		Command: "+api",
		Use:     "+api",
		Short:   "api",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{Body: map[string]any{"theme_id": "t1"}}, nil
		},
	}
	out := runMounted(t, s, "+api")
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode output: %v\nraw: %s", err, out)
	}
	if env["ok"] != true {
		t.Errorf("non-Local Execute must keep the {ok,data} envelope; got %v", env)
	}
}

// TestEngine_AuthFreeSetsAnnotation: Mount must stamp the AuthFree annotation
// (consumed by the dynamic module's auth gate) on AuthFree shortcuts only.
func TestEngine_AuthFreeSetsAnnotation(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	free := common.Shortcut{
		Service: "svc", Command: "+free", Use: "+free", Short: "free", AuthFree: true,
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
	gated := common.Shortcut{
		Service: "svc", Command: "+gated", Use: "+gated", Short: "gated",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
	common.Mount(free, parent, newFakeFactory(t))
	common.Mount(gated, parent, newFakeFactory(t))
	for _, c := range parent.Commands() {
		switch c.Name() {
		case "+free":
			if c.Annotations[cmdutil.AnnotationAuthFree] != "true" {
				t.Errorf("+free must carry the AuthFree annotation; got %v", c.Annotations)
			}
		case "+gated":
			if c.Annotations[cmdutil.AnnotationAuthFree] == "true" {
				t.Errorf("+gated must NOT carry the AuthFree annotation")
			}
		}
	}
}

func TestEngine_RequiredFlagEnforced(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.PersistentFlags().String("format", "json", "")
	s := common.Shortcut{
		Service: "svc",
		Command: "+needs",
		Use:     "+needs",
		Short:   "needs",
		Flags: []common.Flag{
			{Name: "x", Type: common.FlagString, Required: true},
		},
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var out bytes.Buffer
	parent.SetOut(&out)
	parent.SetErr(&out)
	parent.SetArgs([]string{"+needs", "--dry-run"})
	err := parent.Execute()
	if err == nil {
		t.Fatal("expected error for missing required --x; got nil")
	}
	if !strings.Contains(err.Error(), "x") {
		t.Errorf("expected error to mention 'x', got: %v", err)
	}
}
