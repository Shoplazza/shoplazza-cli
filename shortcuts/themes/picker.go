package themes

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// themePicker lets a human who omits --theme-id fuzzy-select a theme by name.
// The theme set is small, so no page size is requested. The id behind the
// chosen name is written onto the flag; non-interactive callers never see it.
var themePicker = &common.ResourcePicker{
	Path: themeBaseV202601,
	Extract: common.ListExtractor("themes", []string{"id"}, func(t map[string]any) string {
		name := common.FirstString(t, "name", "title")
		if role := common.FirstString(t, "theme_type", "role"); role != "" {
			if name == "" {
				return role
			}
			return name + " · " + role
		}
		return name
	}),
}
