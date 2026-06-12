package common

import (
	"github.com/spf13/cobra"
)

// FlagType enumerates the CLI flag value kinds the engine knows how to bind.
type FlagType int

const (
	FlagString FlagType = iota
	FlagInt
	FlagFloat
	FlagBool
	FlagStringSlice
)

// Flag declares one CLI flag for a Shortcut.
//
// Default must be of the Go type implied by Type (string for FlagString, int
// for FlagInt, float64 for FlagFloat, bool for FlagBool, []string for
// FlagStringSlice) — or nil, in which case the Go zero value of the type is
// used. The engine panics at startup if Default's runtime type does not match.
type Flag struct {
	Name        string
	Short       string // optional one-char short alias (e.g. "t" for -t)
	Type        FlagType
	Default     any
	Description string
	Required    bool
	Completions []string
}

// FlagSet is the typed accessor over parsed flag values that the engine passes
// to Plan. It deliberately does not expose *cobra.Command or *pflag.FlagSet,
// so Plan stays a pure function from input to PlannedRequest.
type FlagSet interface {
	GetString(name string) string
	GetInt(name string) int
	GetFloat(name string) float64
	GetBool(name string) bool
	GetStringSlice(name string) []string
	Changed(name string) bool
}

// PlanInput is what the engine hands to a Shortcut's Plan function.
// Tool is Command without the leading "+" — for direct use with AutoName.
type PlanInput struct {
	Args  []string
	Flags FlagSet
	Tool  string
}

// cobraFlagSet implements FlagSet over a *cobra.Command's parsed flags.
type cobraFlagSet struct {
	cmd *cobra.Command
}

// NewCobraFlagSet wraps cmd's parsed flags as a FlagSet. Exported for tests.
func NewCobraFlagSet(cmd *cobra.Command) FlagSet { return &cobraFlagSet{cmd: cmd} }

func (f *cobraFlagSet) GetString(name string) string {
	v, _ := f.cmd.Flags().GetString(name)
	return v
}

func (f *cobraFlagSet) GetInt(name string) int {
	v, _ := f.cmd.Flags().GetInt(name)
	return v
}

func (f *cobraFlagSet) GetFloat(name string) float64 {
	v, _ := f.cmd.Flags().GetFloat64(name)
	return v
}

func (f *cobraFlagSet) GetBool(name string) bool {
	v, _ := f.cmd.Flags().GetBool(name)
	return v
}

func (f *cobraFlagSet) GetStringSlice(name string) []string {
	v, _ := f.cmd.Flags().GetStringSlice(name)
	return v
}

func (f *cobraFlagSet) Changed(name string) bool { return f.cmd.Flags().Changed(name) }
