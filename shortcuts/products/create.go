package products

import (
	"crypto/rand"
	"fmt"
	"strconv"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

var createShortcut = common.Shortcut{
	Service: "products",
	Command: "+create",
	Use:     "+create --title <name> --price <n> --image <url>",
	Short:   "Quickly create a single-variant product",
	Flags: []common.Flag{
		{Name: "title", Type: common.FlagString, Required: true, Description: "Product title (required)."},
		{Name: "price", Type: common.FlagString, Required: true, Description: "Selling price (required, e.g., '29.99')."},
		{Name: "image", Type: common.FlagString, Required: true, Description: "Image URL (required)."},
		{Name: "compare-price", Type: common.FlagString, Description: "Compare-at price."},
		{Name: "sku", Type: common.FlagString, Description: "SKU."},
		{Name: "stock", Type: common.FlagInt, Description: "Stock quantity (enables inventory tracking)."},
		{Name: "stock-policy", Type: common.FlagString, Default: "deny", Description: "Inventory policy when stock=0.",
			Completions: []string{"continue", "deny", "auto_unpublished"}},
		{Name: "tags", Type: common.FlagStringSlice, Description: "Tags (comma-separated)."},
		{Name: "published", Type: common.FlagBool, Description: "Publish on create (default: draft)."},
		{Name: "collection-ids", Type: common.FlagStringSlice, Description: "Collections to add the product to."},
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		price, err := strconv.ParseFloat(in.Flags.GetString("price"), 64)
		if err != nil {
			return common.PlannedRequest{}, output.ErrValidation("--price must be a number, got %q", in.Flags.GetString("price"))
		}
		variant := map[string]any{
			"price":    price,
			"position": 1,
		}
		if cp := in.Flags.GetString("compare-price"); cp != "" {
			cpf, err := strconv.ParseFloat(cp, 64)
			if err != nil {
				return common.PlannedRequest{}, output.ErrValidation("--compare-price must be a number, got %q", cp)
			}
			variant["compare_at_price"] = cpf
		}
		cmdutil.AddString(variant, "sku", in.Flags.GetString("sku"))

		// inventory_tracking / inventory_policy live on the product; the variant only carries inventory_quantity.
		stockTracked := in.Flags.Changed("stock")
		if stockTracked {
			variant["inventory_quantity"] = in.Flags.GetInt("stock")
		}

		product := map[string]any{
			"title":                    in.Flags.GetString("title"),
			"has_only_default_variant": true,
			"published":                in.Flags.GetBool("published"),
			"inventory_tracking":       stockTracked,
			"images":                   []map[string]any{{"src": in.Flags.GetString("image")}},
			"variants":                 []map[string]any{variant},
			"unique_token":             generateUniqueToken(in.Tool),
		}
		if stockTracked {
			product["inventory_policy"] = in.Flags.GetString("stock-policy")
		}
		if tags := in.Flags.GetStringSlice("tags"); len(tags) > 0 {
			product["tags"] = tags
		}
		if cids := in.Flags.GetStringSlice("collection-ids"); len(cids) > 0 {
			product["collection_ids"] = cids
		}

		return PlanCreate(map[string]any{"product": product}), nil
	},
}

// generateUniqueToken returns a random v4 UUID used as the idempotency token (the API requires a UUID).
func generateUniqueToken(_ string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
