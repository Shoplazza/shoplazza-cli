package shortcuts

import (
	"sort"
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
	"shoplazza-cli-v2/shortcuts/customers"
	"shoplazza-cli-v2/shortcuts/discounts"
	"shoplazza-cli-v2/shortcuts/orders"
	"shoplazza-cli-v2/shortcuts/products"
	"shoplazza-cli-v2/shortcuts/shop"
	"shoplazza-cli-v2/shortcuts/themes"

	"github.com/spf13/cobra"
)

var allShortcuts = concat(
	products.Shortcuts(),
	discounts.Shortcuts(),
	orders.Shortcuts(),
	customers.Shortcuts(),
	shop.Shortcuts(),
	themes.Shortcuts(),
)

func concat(slices ...[]common.Shortcut) []common.Shortcut {
	var total int
	for _, s := range slices {
		total += len(s)
	}
	out := make([]common.Shortcut, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// RegisterShortcuts mounts all shortcuts under their service command groups.
func RegisterShortcuts(program *cobra.Command, f *cmdutil.Factory) {
	byService := map[string][]common.Shortcut{}
	for _, s := range allShortcuts {
		if s.Service == "" {
			continue
		}
		byService[s.Service] = append(byService[s.Service], s)
	}

	services := make([]string, 0, len(byService))
	for svc := range byService {
		services = append(services, svc)
	}
	sort.Strings(services)

	for _, name := range services {
		svc := findOrCreateService(program, name)
		for _, s := range byService[name] {
			common.Mount(s, svc, f)
		}
	}
}

// findOrCreateService walks a space-separated path under program and returns
// the leaf cobra.Command, creating any missing intermediate commands.
func findOrCreateService(program *cobra.Command, service string) *cobra.Command {
	parts := strings.Fields(service)
	current := program
	for _, name := range parts {
		current = findOrCreateChild(current, name)
	}
	return current
}

func findOrCreateChild(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	cmd := &cobra.Command{
		Use:   name,
		Short: name + " commands",
	}
	parent.AddCommand(cmd)
	return cmd
}
