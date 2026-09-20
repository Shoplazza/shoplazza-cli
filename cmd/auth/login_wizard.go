package auth

import (
	"errors"
	"slices"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"

	"github.com/charmbracelet/huh"
)

// summaryListWidth wraps the summary card's value column.
const summaryListWidth = 48

// runLoginWizard asks the given steps and returns the flags with the answers
// filled in, as if they had been typed on the command line. A step that is not
// in steps writes nothing back. Sends no request: every option comes from the
// embedded scope map.
func runLoginWizard(steps []loginStep, fl loginFlags) (loginFlags, error) {
	domains := internalauth.TopLevelDomains()
	askStore := slices.Contains(steps, stepStore)
	askDomains := slices.Contains(steps, stepDomains)

	store := fl.storeDomain
	// Default to every domain selected: the common case is full access, so Enter
	// grants everything and the user unchecks to narrow (least privilege stays one
	// keystroke away). Mirrors the "all by default, narrow opt-in" convention.
	selected := append([]string(nil), domains...)

	// One form, one group per planned step: separate forms cannot go back, and
	// huh v1.0.0's WithHideFunc cannot hide the FIRST group.
	build := func() *huh.Form {
		var groups []*huh.Group
		if askStore {
			groups = append(groups, huh.NewGroup(
				huh.NewInput().
					Title("Which store?").
					Description("Leave blank to log in to the account only.").
					Placeholder("my-store.myshoplaza.com").
					Value(&store),
			))
		}
		if askDomains {
			groups = append(groups, huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Which domains do you need access to?").
					Description("All selected by default — press x to remove any you don't need. Each grants the scopes its commands use.").
					Options(domainOptions(domains)...).
					// OnlyOnSubmit: the error appears on enter, and never blocks esc.
					Validate(interact.OnlyOnSubmit(func(v []string) error {
						// store is read live: the user may have gone back and changed it.
						if len(v) == 0 && strings.TrimSpace(store) != "" {
							return errors.New("a store login needs at least one domain — press x to pick one")
						}
						return nil
					})).
					Value(&selected),
			))
		}
		return interact.NewForm(groups...)
	}

	if err := interact.Run(build); err != nil {
		return fl, err
	}

	if askStore {
		fl.storeDomain = strings.TrimSpace(store)
	}
	if askDomains {
		fl.domain = collapseAll(selected, domains)
	}
	return fl, nil
}

// collapseAll returns the "all" sentinel when every domain is selected, and
// selected unchanged otherwise. It is the only way the wizard emits the sentinel.
func collapseAll(selected, domains []string) []string {
	if len(domains) > 0 && len(selected) == len(domains) {
		return []string{internalauth.DomainAll}
	}
	return selected
}

// domainOptions lists the concrete domains, without an "all" row: ctrl+a is
// huh's own select-all.
func domainOptions(domains []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(domains))
	for _, d := range domains {
		label := d
		if desc := domainDescriptions[d]; desc != "" {
			label = d + " — " + desc
		}
		out = append(out, huh.NewOption(label, d))
	}
	return out
}

// domainDescriptions gives each login domain a one-line "what it grants" hint so
// the picker supports informed choice. The label carries it (huh options have no
// separate description column); the authoritative scope list is shown on the
// store's OAuth consent page and afterwards by `auth status`.
var domainDescriptions = map[string]string{
	"orders":    "orders, fulfillments, refunds, transactions",
	"products":  "products, variants, inventory, collections",
	"customers": "customers and addresses",
	"discounts": "discount campaigns and coupon codes",
	"shop":      "shop info, files, metafields, blogs, pages",
	"themes":    "storefront themes",
	"webhook":   "webhook subscriptions",
	"billing":   "application charges (billing)",
	"checkout":  "checkout extensions",
	"app":       "app and extension management",
}

// loginSummaryRows builds the card shown after the wizard, expanding the "all"
// sentinel into the domains it granted.
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
