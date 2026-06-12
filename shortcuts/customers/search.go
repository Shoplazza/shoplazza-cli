package customers

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "customers",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search customers",
	Flags: []common.Flag{
		{Name: "email", Type: common.FlagString, Description: "Filter by email."},
		{Name: "phone", Type: common.FlagString, Description: "Filter by phone."},
		common.SinceFlag(),
		common.UntilFlag(),
		common.PageLimitFlag(),
		common.FieldsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		pl, err := common.GetValidatedPageLimit(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "email", in.Flags.GetString("email"))
		cmdutil.AddString(q, "phone", in.Flags.GetString("phone"))
		cmdutil.AddString(q, "created_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "created_at_max", in.Flags.GetString("until"))
		if pl > 0 {
			q["page_size"] = pl
		}
		if fields := in.Flags.GetStringSlice("fields"); len(fields) > 0 {
			q["fields"] = fields
		}
		return PlanList(q), nil
	},
}
