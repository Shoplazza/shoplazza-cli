package collections

import (
	"context"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

var createShortcut = common.Shortcut{
	Service: "products collections",
	Command: "+create",
	Use:     "+create --title <name> [--product-ids <id,...>]",
	Short:   "Create a collection (and optionally associate products)",
	Flags: []common.Flag{
		{Name: "title", Type: common.FlagString, Required: true, Description: "Collection title (required)."},
		{Name: "description", Type: common.FlagString, Description: "Collection description."},
		{Name: "image", Type: common.FlagString, Description: "Cover image URL."},
		{Name: "sort-order", Type: common.FlagString, Description: "Sort order alias.",
			Completions: []string{"manual", "best-selling", "price-asc", "price-desc", "newest", "popular", "intelligent"}},
		{Name: "product-ids", Type: common.FlagStringSlice, Description: "Products to associate via collects/batch."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		collection := map[string]any{
			"title": in.Flags.GetString("title"),
		}
		cmdutil.AddString(collection, "description", in.Flags.GetString("description"))
		cmdutil.AddString(collection, "image", in.Flags.GetString("image"))
		if so := in.Flags.GetString("sort-order"); so != "" {
			collection["sort_order"] = sortOrderAliasToAPI(so)
		}
		createPlan := PlanCreate(map[string]any{"collection": collection})

		productIDs := in.Flags.GetStringSlice("product-ids")

		if in.DryRun {
			plans := []common.PlannedRequest{createPlan}
			if len(productIDs) > 0 {
				plans = append(plans, PlanBatchAssociate(map[string]any{
					"collection_id": "<resolved-from-step-0>",
					"product_ids":   productIDs,
				}))
			}
			return common.ExecResult{Plans: plans}, nil
		}

		createResp, err := common.Send(ctx, in.Client, createPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		if len(productIDs) == 0 {
			return common.ExecResult{Body: createResp}, nil
		}
		collectionID := extractCollectionID(createResp)
		if collectionID == "" {
			return common.ExecResult{}, output.ErrInternal("collection.id missing from create response")
		}
		_, err = common.Send(ctx, in.Client, PlanBatchAssociate(map[string]any{
			"collection_id": collectionID,
			"product_ids":   productIDs,
		}))
		if err != nil {
			return common.ExecResult{}, err
		}
		return common.ExecResult{Body: createResp}, nil
	},
}

func sortOrderAliasToAPI(cli string) string {
	switch cli {
	case "manual":
		return "manual"
	case "best-selling":
		return "sales-desc"
	case "price-asc":
		return "price-asc"
	case "price-desc":
		return "price-desc"
	case "newest":
		return "created-desc"
	case "popular":
		return "views-desc"
	case "intelligent":
		return "intelligent"
	}
	return cli
}

func extractCollectionID(resp map[string]any) string {
	m, ok := resp["collection"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}
