package customers

import (
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var createShortcut = common.Shortcut{
	Service: "customers",
	Command: "+create",
	Use:     "+create (--email <e> | --phone <p>)",
	Short:   "Create a customer",
	Flags: []common.Flag{
		{Name: "email", Type: common.FlagString, Description: "Email (XOR with --phone)."},
		{Name: "phone", Type: common.FlagString, Description: "Phone (XOR with --email)."},
		{Name: "first-name", Type: common.FlagString, Description: "First name."},
		{Name: "last-name", Type: common.FlagString, Description: "Last name."},
		{Name: "tags", Type: common.FlagStringSlice, Description: "Tags (comma-separated)."},
		{Name: "no-marketing", Type: common.FlagBool, Description: "Do not subscribe to marketing (default: subscribe)."},
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body, err := buildCreateCustomerBody(
			in.Flags.GetString("email"),
			in.Flags.GetString("phone"),
			in.Flags.GetString("first-name"),
			in.Flags.GetString("last-name"),
			in.Flags.GetStringSlice("tags"),
			in.Flags.GetBool("no-marketing"),
		)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		return PlanCreate(body), nil
	},
}

func buildCreateCustomerBody(email, phone, firstName, lastName string, tags []string, noMarketing bool) (map[string]any, error) {
	hasEmail := email != ""
	hasPhone := phone != ""
	if hasEmail == hasPhone {
		return nil, output.ErrValidation("exactly one of --email or --phone is required")
	}
	c := map[string]any{}
	if hasEmail {
		c["contact_type"] = "email"
		c["email"] = email
	}
	if hasPhone {
		c["contact_type"] = "phone"
		c["phone"] = phone
	}
	cmdutil.AddString(c, "first_name", firstName)
	cmdutil.AddString(c, "last_name", lastName)
	if len(tags) > 0 {
		c["tags"] = tags
	}
	c["accepts_marketing"] = !noMarketing
	return map[string]any{"customer": c}, nil
}
