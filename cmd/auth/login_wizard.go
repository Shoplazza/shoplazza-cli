package auth

import (
	"fmt"
	"os"
	"slices"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/huh"
)

// summaryListWidth wraps the summary card's value column, keeping the card
// narrower than a default terminal.
const summaryListWidth = 48

// loginGateOpen reports whether a human is there to answer prompts. It reuses
// the single gate predicate in internal/output rather than testing terminals
// again here. Streams that are not *os.File — every test factory, and anything
// captured in-process — can be neither read for keystrokes nor drawn on, so
// they close the gate.
func loginGateOpen(f *cmdutil.Factory) bool {
	in, inOK := f.IOStreams.In.(*os.File)
	errOut, errOK := f.IOStreams.ErrOut.(*os.File)
	return inOK && errOK && output.Interactive(in, errOut, os.LookupEnv)
}

// runLoginWizard asks the screens plan() selected and writes the answers back
// into the flag variables, so the rest of RunE runs as if they had been typed
// on the command line. Nothing here sends a request: every option comes from
// the embedded scope map.
//
// storeDomain is the -s value, read only by the validation rule below: a store
// login is the case where zero domains cannot work.
func runLoginWizard(steps []loginStep, storeDomain string, domain *[]string) error {
	domains := internalauth.TopLevelDomains()
	var selected []string

	// Every step is a group of ONE form — separate forms per step cannot go
	// back. A step plan() did not select is hidden, so the hide funcs read that
	// one decision instead of re-deriving it.
	build := func() *huh.Form {
		return interact.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Which domains do you need access to?").
					Description("Each domain grants the scopes its commands need.").
					Options(domainOptions(domains)...).
					Validate(func(v []string) error {
						if len(v) == 0 && strings.TrimSpace(storeDomain) != "" {
							return fmt.Errorf("a store login needs at least one domain")
						}
						return nil
					}).
					Value(&selected),
			).WithHideFunc(func() bool { return !slices.Contains(steps, stepDomains) }),
		)
	}

	if err := interact.Run(build); err != nil {
		return err
	}

	if slices.Contains(steps, stepDomains) {
		// "all" checked, or every domain checked one by one, collapses to the
		// sentinel the flag and the server already speak.
		if slices.Contains(selected, internalauth.DomainAll) || len(selected) == len(domains) {
			selected = []string{internalauth.DomainAll}
		}
		*domain = selected
	}
	return nil
}

// domainOptions lists the domains with the "all" sentinel first, set apart by
// weight alone: bold, but left-aligned with the concrete domains — no indent,
// no grey aside.
func domainOptions(domains []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(domains)+1)
	out = append(out, huh.NewOption(interact.Bold(internalauth.DomainAll), internalauth.DomainAll))
	for _, d := range domains {
		out = append(out, huh.NewOption(d, d))
	}
	return out
}

// loginSummaryRows builds the card shown after the wizard. The flag keeps the
// compact "all" sentinel — scripts and the server want it — while the card
// spells out the domains it actually granted, wrapped to the card's width.
func loginSummaryRows(store string, domain, scope []string) []string {
	if store == "" {
		store = interact.Dim("(account only)")
	}
	rows := []string{interact.Field("Store", store)}
	if len(scope) > 0 {
		return append(rows, interact.FieldList("Scopes", sortedCopy(scope), summaryListWidth))
	}
	granted := domain
	if slices.Contains(granted, internalauth.DomainAll) {
		granted = internalauth.TopLevelDomains()
	}
	return append(rows, interact.FieldList("Domains", sortedCopy(granted), summaryListWidth))
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
