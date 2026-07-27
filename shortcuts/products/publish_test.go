package products

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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
	var found bool
	for _, f := range publishShortcutValue.Flags {
		if f.Name == "id" && f.Required {
			found = true
		}
	}
	if !found {
		t.Error("publish should have a required --id flag")
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
