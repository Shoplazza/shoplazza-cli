package products

import (
	"testing"
)

func TestUnpublishShortcutPlan_Success(t *testing.T) {
	in := newProductPlanInput(t, "unpublish", productIDFlags, map[string]string{"id": "prod-1"})
	_, err := unpublishShortcutValue.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
