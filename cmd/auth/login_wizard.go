package auth

import (
	"slices"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"

	"github.com/charmbracelet/huh"
)

// summaryListWidth wraps the summary card's value column, keeping the card
// narrower than a default terminal.
const summaryListWidth = 48

// runLoginWizard asks the screens plan() selected and writes the answers back
// into the flag variables, so the rest of RunE runs as if they had been typed
// on the command line. Nothing here sends a request: every option comes from
// the embedded scope map.
//
// A screen plan() skipped writes nothing back: its flag stays exactly as the
// command line set it.
func runLoginWizard(steps []loginStep, storeDomain *string, domain *[]string) error {
	domains := internalauth.TopLevelDomains()
	askStore := slices.Contains(steps, stepStore)
	askDomains := slices.Contains(steps, stepDomains)

	store := *storeDomain
	var selected []string

	// Every step that runs is a group of ONE form — separate forms cannot go back.
	//
	// Only planned steps get a group. WithHideFunc would read better, but huh
	// v1.0.0 consults it only when moving BETWEEN groups, so a hidden FIRST
	// group still draws (measured ~0.5s, with the -s value in the box) before
	// Init's nextGroup lands. Not building it keeps "given means the step never
	// appears" literally true.
	build := func() *huh.Form {
		var groups []*huh.Group
		// The store, typed straight in. Blank is the skip — it means an
		// account-only login, exactly what omitting -s means. No preceding
		// "account or store?" question: blank already says it.
		if askStore {
			groups = append(groups, huh.NewGroup(
				huh.NewInput().
					Title("Which store?").
					Description("Leave blank to log in to the account only.").
					Placeholder("my-store.myshoplaza.com").
					Value(&store),
			))
		}
		// No Validate on this field, deliberately. MultiSelect.Focus() runs
		// Validate before any keypress, and huh refuses to go back while a group
		// holds an error — so "a store login needs a domain" would greet the user
		// on arrival AND silently swallow esc until they ticked something. The
		// same rule is enforced after the wizard, where it already lived.
		if askDomains {
			groups = append(groups, huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Which domains do you need access to?").
					Description("Each domain grants the scopes its commands need.").
					Options(domainOptions(domains)...).
					Value(&selected),
			))
		}
		return interact.NewForm(groups...)
	}

	if err := interact.Run(build); err != nil {
		return err
	}

	if askStore {
		*storeDomain = strings.TrimSpace(store)
	}
	if askDomains {
		*domain = collapseAll(selected, domains)
	}
	return nil
}

// collapseAll turns a fully-ticked picker into the sentinel the flag and the
// server already speak — the same scope set, only shorter. Ticking every domain
// one by one and passing --domain all must not diverge.
//
// The picker no longer offers an "all" row (ctrl+a is huh's own, and is in the
// help line), so this is the only way the sentinel can come out of the wizard.
func collapseAll(selected, domains []string) []string {
	if len(domains) > 0 && len(selected) == len(domains) {
		return []string{internalauth.DomainAll}
	}
	return selected
}

// domainOptions lists the concrete domains, nothing else. An "all" row used to
// lead the list; it was redundant with ctrl+a, which huh implements and shows in
// the help line, and it put a row in the picker that no other row could be
// combined with.
func domainOptions(domains []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(domains))
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
