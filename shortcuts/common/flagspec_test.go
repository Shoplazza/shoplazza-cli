package common_test

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestIDFlag_Defaults(t *testing.T) {
	f := common.IDFlag("Resource ID to fetch.")
	if f.Name != "id" {
		t.Errorf("Name: got %q want %q", f.Name, "id")
	}
	if f.Type != common.FlagString {
		t.Errorf("Type: got %v want FlagString", f.Type)
	}
	if !f.Required {
		t.Error("Required: should be true")
	}
	if f.Description == "" {
		t.Error("Description should be set from argument")
	}
}

func TestPageLimitFlag_Defaults(t *testing.T) {
	f := common.PageLimitFlag()
	if f.Name != "page-limit" {
		t.Errorf("Name: got %q want %q", f.Name, "page-limit")
	}
	if f.Type != common.FlagInt {
		t.Errorf("Type: got %v want FlagInt", f.Type)
	}
	if f.Required {
		t.Error("Required: should be false (optional flag)")
	}
}

func TestSinceUntilFlags(t *testing.T) {
	since := common.SinceFlag()
	until := common.UntilFlag()
	if since.Name != "since" || until.Name != "until" {
		t.Errorf("names: got %q/%q want since/until", since.Name, until.Name)
	}
}

func TestFieldsFlag(t *testing.T) {
	f := common.FieldsFlag()
	if f.Name != "fields" {
		t.Errorf("Name: got %q want fields", f.Name)
	}
	if f.Type != common.FlagStringSlice {
		t.Errorf("Type: got %v want FlagStringSlice", f.Type)
	}
}

func TestStartEndTimeFlag_Defaults(t *testing.T) {
	s := common.StartTimeFlag()
	if s.Name != "start" || s.Type != common.FlagString {
		t.Errorf("StartTimeFlag: got name=%q type=%v", s.Name, s.Type)
	}
	e := common.EndTimeFlag()
	if e.Name != "end" || e.Type != common.FlagString {
		t.Errorf("EndTimeFlag: got name=%q type=%v", e.Name, e.Type)
	}
}

func TestValidatePageLimit(t *testing.T) {
	cases := []struct {
		pl      int
		wantErr bool
	}{
		{0, false},
		{1, false},
		{250, false},
		{251, true},
		{-1, true},
	}
	for _, c := range cases {
		err := common.ValidatePageLimit(c.pl)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidatePageLimit(%d) error=%v, wantErr=%v", c.pl, err, c.wantErr)
		}
	}
}

// stubFlags is a minimal FlagSet implementation for testing helpers that
// read only integer flags (like GetValidatedPageLimit).
type stubFlags struct{ pageLimit int }

func (s stubFlags) GetString(_ string) string        { return "" }
func (s stubFlags) GetInt(_ string) int              { return s.pageLimit }
func (s stubFlags) GetFloat(_ string) float64        { return 0 }
func (s stubFlags) GetBool(_ string) bool            { return false }
func (s stubFlags) GetStringSlice(_ string) []string { return nil }
func (s stubFlags) Changed(_ string) bool            { return false }

func TestGetValidatedPageLimit_Zero(t *testing.T) {
	in := common.PlanInput{Flags: stubFlags{pageLimit: 0}}
	got, err := common.GetValidatedPageLimit(in)
	if err != nil || got != 0 {
		t.Errorf("got (%d,%v), want (0,nil)", got, err)
	}
}

func TestGetValidatedPageLimit_Valid(t *testing.T) {
	in := common.PlanInput{Flags: stubFlags{pageLimit: 50}}
	got, err := common.GetValidatedPageLimit(in)
	if err != nil || got != 50 {
		t.Errorf("got (%d,%v), want (50,nil)", got, err)
	}
}

func TestGetValidatedPageLimit_TooLargeErrors(t *testing.T) {
	in := common.PlanInput{Flags: stubFlags{pageLimit: 300}}
	_, err := common.GetValidatedPageLimit(in)
	if err == nil {
		t.Error("expected error for page-limit > 250")
	}
}
