package products

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

func TestUnpublishShortcutPlan_Success(t *testing.T) {
	in := shortcuttest.PlanInput(t, "unpublish", productIDFlags, map[string]string{"id": "prod-1"})
	_, err := unpublishShortcutValue.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
