package common_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

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

// Shortcuts take only flags: a stray positional arg (the "spaces not commas"
// mistake) is rejected with a quoting/comma hint, while a single quoted value
// containing a space is one token and passes.
func TestEngine_PositionalArgValidation(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"stray positional arg rejected", []string{"+thing", "--dry-run", "--variants", "a", "b"}, true},
		{"quoted spaced value allowed", []string{"+thing", "--dry-run", "--name", "Summer Sale"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "svc"}
			parent.PersistentFlags().Bool("dry-run", false, "")
			parent.PersistentFlags().String("format", "json", "")
			s := common.Shortcut{
				Service: "svc",
				Command: "+thing",
				Use:     "+thing",
				Short:   "do a thing",
				Flags: []common.Flag{
					{Name: "variants", Type: common.FlagString},
					{Name: "name", Type: common.FlagString},
				},
				Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
					return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
				},
			}
			common.Mount(s, parent, newFakeFactory(t))
			parent.SetArgs(c.args)
			err := parent.Execute()
			if !c.wantErr {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error for stray positional argument")
			}
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "quote") || !strings.Contains(msg, "comma") {
				t.Errorf("error should hint at quoting and comma separation; got %v", err)
			}
		})
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

// TestEngine_DryRunReadsSetsAnnotation: Mount must stamp the DryRunReads
// annotation (consumed by the dynamic module's auth gate).
func TestEngine_DryRunReadsSetsAnnotation(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	s := common.Shortcut{
		Service: "svc", Command: "+reads", Use: "+reads", Short: "reads", DryRunReads: true,
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			return common.ExecResult{}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))
	if got := parent.Commands()[0].Annotations[cmdutil.AnnotationDryRunReads]; got != "true" {
		t.Errorf("+reads must carry the DryRunReads annotation; got %q", got)
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

// Backticks in a flag description render as quotes, not as the value placeholder.
func TestEngine_FlagDescriptionBackticksStayProse(t *testing.T) {
	parent := &cobra.Command{Use: "svc"}
	s := common.Shortcut{
		Service: "svc",
		Command: "thing",
		Use:     "thing",
		Short:   "do a thing",
		Flags: []common.Flag{{
			Name: "theme-id", Short: "t", Type: common.FlagString,
			Description: "Theme ID. Run `shoplazza themes list` to discover.",
		}},
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	common.Mount(s, parent, newFakeFactory(t))

	var sub *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "thing" {
			sub = c
		}
	}
	if sub == nil {
		t.Fatal("expected thing subcommand mounted under svc")
	}
	usage := sub.Flags().FlagUsages()
	if !strings.Contains(usage, "--theme-id string") {
		t.Errorf("placeholder should be the value type:\n%s", usage)
	}
	if strings.Contains(usage, "`") || !strings.Contains(usage, "'shoplazza themes list'") {
		t.Errorf("backticks should render as quotes:\n%s", usage)
	}
}

// A renamed flag keeps its old spelling working: Flag.Aliases normalizes onto
// the canonical name before parsing, so --help shows one entry, Required sees
// the value, and existing callers do not break.
func TestMount_FlagAliasResolvesToCanonicalName(t *testing.T) {
	newShortcut := func(got *string) common.Shortcut {
		return common.Shortcut{
			Service: "svc", Command: "+probe", Use: "+probe",
			Flags: []common.Flag{{
				Name: "theme-id", Short: "t", Aliases: []string{"theme"},
				Type: common.FlagString, Required: true, Description: "Theme ID.",
			}},
			Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
				*got = in.Flags.GetString("theme-id")
				return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
			},
		}
	}
	for _, spelling := range []string{"--theme-id", "--theme", "-t"} {
		var got string
		parent := &cobra.Command{Use: "svc"}
		common.Mount(newShortcut(&got), parent, newFakeFactory(t))
		parent.SetOut(&bytes.Buffer{})
		parent.SetErr(&bytes.Buffer{})
		parent.SetArgs([]string{"+probe", spelling, "t_123", "--dry-run"})
		if err := parent.Execute(); err != nil {
			t.Fatalf("%s: %v", spelling, err)
		}
		if got != "t_123" {
			t.Errorf("%s reached the shortcut as %q, want t_123", spelling, got)
		}
	}
	// The alias must not show up as a second flag.
	var sink string
	parent := &cobra.Command{Use: "svc"}
	common.Mount(newShortcut(&sink), parent, newFakeFactory(t))
	usage := parent.Commands()[0].LocalFlags().FlagUsages()
	if !strings.Contains(usage, "--theme-id") {
		t.Fatalf("the canonical flag is missing from --help:\n%s", usage)
	}
	if strings.Contains(usage, "--theme string") {
		t.Errorf("the alias leaked into --help:\n%s", usage)
	}
}

// A deprecated spelling stays in --help with a [deprecated] prefix, satisfies
// Required, and reaches Plan under the canonical name; both spellings at once
// is a validation error.
func TestMount_DeprecatedAliasListedAndResolved(t *testing.T) {
	type got struct {
		pct  float64
		tags []string
	}
	newShortcut := func(g *got) common.Shortcut {
		return common.Shortcut{
			Service: "svc", Command: "+probe", Use: "+probe",
			Flags: []common.Flag{
				{Name: "percent-off", DeprecatedAliases: []string{"percent"}, Type: common.FlagFloat, Required: true, Description: "Percent off."},
				{Name: "tags", DeprecatedAliases: []string{"tag"}, Type: common.FlagStringSlice, Description: "Tags."},
			},
			Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
				g.pct = in.Flags.GetFloat("percent-off")
				g.tags = in.Flags.GetStringSlice("tags")
				return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
			},
		}
	}
	run := func(args ...string) (got, error) {
		var g got
		parent := &cobra.Command{Use: "svc", SilenceErrors: true, SilenceUsage: true}
		common.Mount(newShortcut(&g), parent, newFakeFactory(t))
		parent.SetOut(&bytes.Buffer{})
		parent.SetErr(&bytes.Buffer{})
		parent.SetArgs(append([]string{"+probe", "--dry-run"}, args...))
		return g, parent.Execute()
	}

	for _, args := range [][]string{
		{"--percent-off", "20", "--tags", "a,b"},
		{"--percent", "20", "--tag", "a,b"},
	} {
		g, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if g.pct != 20 || strings.Join(g.tags, ",") != "a,b" {
			t.Errorf("%v reached the shortcut as %+v, want pct=20 tags=[a b]", args, g)
		}
	}

	if _, err := run("--percent", "20", "--percent-off", "20"); err == nil || !strings.Contains(err.Error(), "pass only --percent-off") {
		t.Errorf("both spellings: want a 'pass only --percent-off' error, got %v", err)
	}

	var sink got
	parent := &cobra.Command{Use: "svc"}
	common.Mount(newShortcut(&sink), parent, newFakeFactory(t))
	usage := parent.Commands()[0].LocalFlags().FlagUsages()
	for _, want := range []string{"--percent-off float", "--percent float", "[deprecated] Percent off.", "--tag strings", "[deprecated] Tags."} {
		if !strings.Contains(usage, want) {
			t.Errorf("--help is missing %q:\n%s", want, usage)
		}
	}
}

func TestMount_FlagValueChecks(t *testing.T) {
	var gotStatus, gotTags []string
	s := common.Shortcut{
		Service: "svc", Command: "+probe", Use: "+probe",
		Flags: []common.Flag{
			{Name: "target", Type: common.FlagString, Enum: []string{"order", "product"}},
			{Name: "percent-off", DeprecatedAliases: []string{"percent"}, Type: common.FlagFloat},
			{Name: "limit", Type: common.FlagInt},
			{Name: "status", Type: common.FlagStringSlice, Enum: []string{"paid", "refunded"}},
			{Name: "tags", Type: common.FlagStringSlice},
		},
		Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
			gotStatus, gotTags = in.Flags.GetStringSlice("status"), in.Flags.GetStringSlice("tags")
			return common.PlannedRequest{Method: "GET", Path: "/x"}, nil
		},
	}
	cases := []struct {
		name               string
		args               []string
		wantParam, wantMsg string
		wantHint           string
		wantStatus         []string
		wantTags           []string
	}{
		{"single-value flag repeated", []string{"--target", "order", "--target", "product"},
			"--target", `--target was given 2 times ("order", "product")`, "pass --target once", nil, nil},
		{"int flag repeated", []string{"--limit", "1", "--limit", "2"}, "--limit", "given 2 times", "", nil, nil},
		{"deprecated alias repeated", []string{"--percent", "10", "--percent", "20"}, "--percent", "given 2 times", "", nil, nil},
		{"enum string rejected", []string{"--target", "bogus"},
			"--target", `invalid --target value "bogus"`, "accepted: order, product", nil, nil},
		{"enum slice item rejected", []string{"--status", "paid,bogus,nope"},
			"--status", `invalid --status value "bogus", "nope"`, "comma-separate or repeat", nil, nil},
		{"enum slice comma-separated", []string{"--status", "paid,refunded"}, "", "", "", []string{"paid", "refunded"}, nil},
		{"enum slice repeated", []string{"--status", "paid", "--status", "refunded"}, "", "", "", []string{"paid", "refunded"}, nil},
		{"slice items trimmed, empties dropped", []string{"--status", "paid, refunded", "--tags", " a ,, b "},
			"", "", "", []string{"paid", "refunded"}, []string{"a", "b"}},
		{"enum string accepted", []string{"--target", "order"}, "", "", "", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotTags = nil, nil
			parent := &cobra.Command{Use: "svc", SilenceErrors: true, SilenceUsage: true}
			common.Mount(s, parent, newFakeFactory(t))
			parent.SetOut(&bytes.Buffer{})
			parent.SetErr(&bytes.Buffer{})
			parent.SetArgs(append([]string{"+probe", "--dry-run"}, c.args...))
			err := parent.Execute()
			if c.wantParam == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if strings.Join(gotStatus, "|") != strings.Join(c.wantStatus, "|") || strings.Join(gotTags, "|") != strings.Join(c.wantTags, "|") {
					t.Errorf("status/tags = %q/%q, want %q/%q", gotStatus, gotTags, c.wantStatus, c.wantTags)
				}
				return
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Subtype != output.SubtypeInvalidFlagValue || ee.Detail.Param != c.wantParam {
				t.Fatalf("want invalid_flag_value on %s, got %#v", c.wantParam, err)
			}
			if !strings.Contains(ee.Detail.Message, c.wantMsg) || !strings.Contains(ee.Detail.Hint, c.wantHint) {
				t.Errorf("message/hint = %q / %q, want %q / %q", ee.Detail.Message, ee.Detail.Hint, c.wantMsg, c.wantHint)
			}
		})
	}
}
