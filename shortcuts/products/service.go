package products

import "shoplazza-cli-v2/shortcuts/common"

const productsBase = common.APIPrefix + "/products"

// Plan* functions build common.PlannedRequest values without touching the network.

func PlanList(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: productsBase, Query: query}
}

func PlanCount(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: productsBase + "/count", Query: query}
}

func PlanUpdate(productID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PUT", Path: productsBase + "/" + productID, Body: body}
}

func PlanGet(productID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: productsBase + "/" + productID}
}

func PlanCreate(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: productsBase, Body: body}
}

// Variant base path.
const variantsBase = common.APIPrefix + "/variants"

func PlanUpdateVariantBySKU(sku string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PUT", Path: variantsBase + "/sku/" + sku, Body: body}
}

func PlanUpdateVariant(variantID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PUT", Path: variantsBase + "/" + variantID, Body: body}
}

func PlanGetVariant(variantID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: variantsBase + "/" + variantID}
}

func PlanListVariantsBySKU(sku string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: productsBase + "/sku/" + sku + "/variants"}
}

// Inventory base paths.
const (
	inventoryItemsBase  = common.APIPrefix + "/inventory_items"
	inventoryLevelsBase = common.APIPrefix + "/inventory_levels"
	locationsBase       = common.APIPrefix + "/locations"
)

func PlanInventoryItemForVariant(variantID string) common.PlannedRequest {
	return common.PlannedRequest{
		Method: "GET",
		Path:   inventoryItemsBase + "/variant",
		Query:  map[string]any{"variant_ids": []string{variantID}},
	}
}

func PlanDefaultLocation() common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: locationsBase + "/default"}
}

func PlanSetInventoryLevel(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: inventoryLevelsBase + "/set", Body: body}
}

func PlanAdjustInventoryLevel(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PUT", Path: inventoryLevelsBase, Body: body}
}

// PlanGetInventoryLevel reads the current stock for one (inventory_item, location) pair.
func PlanGetInventoryLevel(inventoryItemID, locationID string) common.PlannedRequest {
	return common.PlannedRequest{
		Method: "GET",
		Path:   inventoryLevelsBase,
		Query: map[string]any{
			"inventory_item_ids": []string{inventoryItemID},
			"location_ids":       []string{locationID},
		},
	}
}
