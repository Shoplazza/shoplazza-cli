package auth

import (
	"errors"
	"sort"
	"strings"
)

// knownScopes is the complete set of OAuth scopes supported by the platform.
var knownScopes = map[string]struct{}{
	"read_customer": {}, "write_customer": {},
	"read_order": {}, "write_order": {},
	"read_product": {}, "write_product": {},
	"read_collection": {}, "write_collection": {},
	"read_script_tags": {}, "write_script_tags": {},
	"read_content": {}, "write_content": {},
	"read_app_proxy": {}, "write_app_proxy": {},
	"read_data": {}, "write_data": {},
	"read_html_tags": {}, "write_html_tags": {},
	"read_shop": {}, "write_shop": {},
	"read_comments": {}, "write_comments": {},
	"read_price_rules": {}, "write_price_rules": {},
	"read_shop_navigation": {}, "write_shop_navigation": {},
	"read_search_api": {}, "write_search_api": {},
	"read_gift_cards": {}, "write_gift_cards": {},
	"read_themes": {}, "write_themes": {},
	"read_payment_info": {}, "write_payment_info": {},
	"read_inventory": {}, "write_inventory": {},
	"read_finance": {}, "write_finance": {},
	"read_cart_transform": {}, "write_cart_transform": {},
}

// SupportedScopes returns a sorted slice of all known scope names.
func SupportedScopes() []string {
	out := make([]string, 0, len(knownScopes))
	for s := range knownScopes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ValidateScopes returns an error listing any unrecognised scope names.
func ValidateScopes(scopes []string) error {
	var invalid []string
	for _, s := range scopes {
		if _, ok := knownScopes[s]; !ok {
			invalid = append(invalid, s)
		}
	}
	if len(invalid) > 0 {
		return errors.New("unrecognised scope(s): " + strings.Join(invalid, ", "))
	}
	return nil
}
