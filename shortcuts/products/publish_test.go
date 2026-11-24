package products

import (
	"slices"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

func TestPublishShortcut_NoPositionalArgs(t *testing.T) {
	if publishShortcutValue.Args != nil {
		t.Errorf("publish should have Args=nil (no positional); got non-nil")
	}
	if unpublishShortcutValue.Args != nil {
		t.Errorf("unpublish should have Args=nil; got non-nil")
	}
}

func TestPublishShortcut_HasIDFlag(t *testing.T) {
	for _, s := range []common.Shortcut{publishShortcutValue, unpublishShortcutValue, tagShortcut, setVariantsShortcut} {
		var found bool
		for _, f := range s.Flags {
			if f.Name == "product-id" && f.Required && slices.Equal(f.Aliases, []string{"id"}) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s should have a required --product-id flag with --id as its alias", s.Command)
		}
	}
}

func TestPublishShortcut_Validates(t *testing.T) {
	if err := common.ValidateShortcut(publishShortcutValue); err != nil {
		t.Errorf("ValidateShortcut(publish): %v", err)
	}
	if err := common.ValidateShortcut(unpublishShortcutValue); err != nil {
		t.Errorf("ValidateShortcut(unpublish): %v", err)
	}
}

func TestPublishShortcutPlan_Success(t *testing.T) {
	in := shortcuttest.PlanInput(t, "publish", productIDFlags, map[string]string{"product-id": "prod-1"})
	_, err := publishShortcutValue.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
