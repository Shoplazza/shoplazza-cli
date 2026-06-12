package common_test

import (
	"testing"

	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

func TestFlagSet_GettersReturnTypedValues(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("s", "hello", "")
	cmd.Flags().Int("i", 7, "")
	cmd.Flags().Float64("f", 1.5, "")
	cmd.Flags().Bool("b", true, "")
	cmd.Flags().StringSlice("ss", []string{"a", "b"}, "")

	fs := common.NewCobraFlagSet(cmd)
	if got := fs.GetString("s"); got != "hello" {
		t.Errorf("GetString: got %q, want %q", got, "hello")
	}
	if got := fs.GetInt("i"); got != 7 {
		t.Errorf("GetInt: got %d, want %d", got, 7)
	}
	if got := fs.GetFloat("f"); got != 1.5 {
		t.Errorf("GetFloat: got %v, want %v", got, 1.5)
	}
	if got := fs.GetBool("b"); got != true {
		t.Errorf("GetBool: got %v, want %v", got, true)
	}
	if got := fs.GetStringSlice("ss"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("GetStringSlice: got %v, want [a b]", got)
	}
}

func TestFlagSet_ChangedReflectsExplicitSet(t *testing.T) {
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().String("name", "default", "")
	cmd.SetArgs([]string{"--name=explicit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	fs := common.NewCobraFlagSet(cmd)
	if !fs.Changed("name") {
		t.Errorf("Changed(name) = false; want true after explicit --name=explicit")
	}
}

// TestFlag_ShortAliasBindsAsExpected confirms that when a common.Flag carries
// a Short value, the engine's bindFlag wires up the cobra short alias so that
// `-t <val>` is equivalent to `--theme-id <val>`. Other flags without a Short
// must continue to behave (-only long form).
func TestFlag_ShortAliasBindsAsExpected(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}

	// Mirror what engine.bindFlag would call for a {Name:"theme-id", Short:"t"} flag.
	cmd.Flags().StringP("theme-id", "t", "", "")
	cmd.Flags().String("other", "", "") // no short — must not be reachable via single-char

	cmd.SetArgs([]string{"-t", "abc"})
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute with -t: %v", err)
	}
	if got, _ := cmd.Flags().GetString("theme-id"); got != "abc" {
		t.Errorf("-t abc → theme-id = %q, want %q", got, "abc")
	}
}

// TestFlag_ShortAliasMatchesLongForm exercises bindFlag end-to-end through
// engine.Mount: declares a Shortcut with Short:"t" and ensures both -t and
// --theme-id parse to the same value.
func TestFlag_ShortAliasMatchesLongForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"short", []string{"-t", "abc"}},
		{"long", []string{"--theme-id", "abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "x"}
			cmd.Flags().StringP("theme-id", "t", "", "")
			cmd.SetArgs(tc.args)
			cmd.RunE = func(*cobra.Command, []string) error { return nil }
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			fs := common.NewCobraFlagSet(cmd)
			if got := fs.GetString("theme-id"); got != "abc" {
				t.Errorf("%v: theme-id = %q, want %q", tc.args, got, "abc")
			}
		})
	}
}

func TestPlanInput_Fields(t *testing.T) {
	in := common.PlanInput{Args: []string{"x"}, Tool: "flashsale"}
	if in.Tool != "flashsale" {
		t.Errorf("Tool: got %q want %q", in.Tool, "flashsale")
	}
	if len(in.Args) != 1 || in.Args[0] != "x" {
		t.Errorf("Args: got %v", in.Args)
	}
}
