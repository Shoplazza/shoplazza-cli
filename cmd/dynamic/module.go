package dynamic

import (
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/registry"

	"github.com/spf13/cobra"
)

// moduleShorts gives known top-level modules a custom one-line Short; unlisted
// modules fall back to titleCase(mod.Name).
var moduleShorts = map[string]string{
	"products":  "Manage products, variants, inventory, collections, comments, and gift cards",
	"orders":    "Manage orders, fulfillments, refunds, transactions, and post-sales workflows",
	"customers": "Manage customers and their addresses",
	"discounts": "Manage discount campaigns and coupon codes",
	"billing":   "Manage application charges (one-time, recurring, and usage-based)",
	"shop":      "Manage shop info, articles, blogs, pages, files, and metafields",
	"themes":    "Manage storefront themes, including installation, configuration, and asset editing",
	"webhook":   "Manage webhook subscriptions",
}

func moduleShort(name string) string {
	if s, ok := moduleShorts[name]; ok {
		return s
	}
	return titleCase(name)
}

// accessTierLong is appended to every module's Long to explain the three command tiers.
const accessTierLong = `
Access tiers:
  +<shortcut>   Human and AI-friendly. Named flags, smart defaults, structured errors.
  <command>     Auto-generated from OpenAPI spec. Full parameter control for scripting.
  api rest      Raw HTTP fallback covering the full platform surface.

Run 'shoplazza schema <module>' to list all commands, or 'shoplazza schema <module>.<command>' to view parameters.`

// buildModuleCommand walks mod.Commands and registers each via path[].
// Returns nil if zero commands survive the validity filter.
func buildModuleCommand(mod registry.Module, spec *registry.Spec, factory *cmdutil.Factory) *cobra.Command {
	moduleCmd := &cobra.Command{
		Use:   mod.Name,
		Short: moduleShort(mod.Name),
		Long:  moduleShort(mod.Name) + "\n" + accessTierLong,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Discovery nodes (bare group invocation) skip the auth gate.
			if cmd.Annotations[annotationDiscovery] == "true" {
				return nil
			}
			// Local shortcuts declare AuthFree (stamped by Mount); all other
			// leaves stay gated.
			if cmd.Annotations[cmdutil.AnnotationAuthFree] == "true" {
				return nil
			}
			return cmdutil.RequireAuth(cmd.Context(), factory)
		},
		Annotations: map[string]string{annotationDiscovery: "true"},
	}

	valid := filterCommands(mod.Commands)

	if len(valid) == 0 {
		return nil
	}

	nodes := map[string]*cobra.Command{"": moduleCmd}
	for _, c := range valid {
		parent := moduleCmd
		for i := 0; i < len(c.Path)-1; i++ {
			key := strings.Join(c.Path[:i+1], "/")
			if existing, ok := nodes[key]; ok {
				parent = existing
				continue
			}
			short, long := implicitGroupDocs(mod, c.Path[i])
			grp := &cobra.Command{
				Use:         c.Path[i],
				Short:       short,
				Long:        long,
				Annotations: map[string]string{annotationDiscovery: "true"},
			}
			parent.AddCommand(grp)
			nodes[key] = grp
			parent = grp
		}
		leaf := buildLeafCommand(c, spec, factory, mod.Name)
		parent.AddCommand(leaf)
	}
	return moduleCmd
}

// annotationDiscovery marks a node (module or implicit group) whose job is
// only to list children. The auth gate skips these.
const annotationDiscovery = "shoplazza.discovery"

// filterCommands drops invalid entries: duplicates, prefix conflicts, bad
// HTTP method/path, or non-kebab-case path segments. A prefix conflict is when
// one command's path is a strict prefix of another's (cobra can't host both a
// leaf and a group at the same node); both members of such a pair are dropped.
func filterCommands(cmds []registry.Command) []registry.Command {
	keys := make([]string, len(cmds))
	pathCount := make(map[string]int, len(cmds))
	pathSet := make(map[string]struct{}, len(cmds))
	for i, c := range cmds {
		k := strings.Join(c.Path, "/")
		keys[i] = k
		pathCount[k]++
		pathSet[k] = struct{}{}
	}

	// For each command path k, scan its proper prefixes (split points at '/').
	// If any prefix is itself a path in pathSet, both sides conflict.
	conflicted := make(map[string]struct{})
	for _, k := range keys {
		for j := 0; j < len(k); j++ {
			if k[j] != '/' {
				continue
			}
			prefix := k[:j]
			if _, ok := pathSet[prefix]; ok {
				conflicted[k] = struct{}{}
				conflicted[prefix] = struct{}{}
			}
		}
	}

	keep := make([]registry.Command, 0, len(cmds))
	for i, c := range cmds {
		if !commandIsValid(c) {
			continue
		}
		k := keys[i]
		if pathCount[k] > 1 {
			continue
		}
		if _, bad := conflicted[k]; bad {
			continue
		}
		keep = append(keep, c)
	}
	return keep
}

func commandIsValid(c registry.Command) bool {
	if len(c.Path) == 0 {
		return false
	}
	for _, seg := range c.Path {
		if !isKebabCase(seg) {
			return false
		}
	}
	switch strings.ToUpper(c.HTTP.Method) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return false
	}
	if c.HTTP.Path == "" || c.HTTP.Path[0] != '/' {
		return false
	}
	return true
}

// implicitGroupDocs returns (short, long) for an implicit subgroup, preferring
// metadata declared in the spec's `groups` table and falling back to the
// historical "<name> operations" placeholder when none is provided.
func implicitGroupDocs(mod registry.Module, groupName string) (string, string) {
	if g, ok := mod.Groups[groupName]; ok {
		return firstNonEmpty(g.Summary, groupName+" operations"), g.Description
	}
	return groupName + " operations", ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// titleCase: "gift-cards" → "Gift Cards".
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
