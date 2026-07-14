package profile

import (
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

// currentOrNamed returns name, falling back to the current profile, with
// differentiated hints when neither exists. Shared by info/rename/update.
func currentOrNamed(f *cmdutil.Factory, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	if f.Config.CurrentProfile != "" {
		return f.Config.CurrentProfile, nil
	}
	if len(f.Config.Profiles) == 0 {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no profiles configured",
			"run 'shoplazza auth login -s <store-domain>' or 'shoplazza profile add --name <name> --store-domain <domain>' to create one")
	}
	names := make([]string, 0, len(f.Config.Profiles))
	for _, p := range f.Config.Profiles {
		names = append(names, p.Name)
	}
	return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no current profile set",
		"run 'shoplazza profile use <name>' (available: "+strings.Join(names, ", ")+")")
}
