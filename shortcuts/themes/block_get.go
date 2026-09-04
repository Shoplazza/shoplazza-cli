package themes

import (
	"context"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// themes block +get — read a generated block file and where it is placed.

var blockGetShortcut = common.Shortcut{
	Service: "themes block",
	Command: "+get",
	Use:     "+get --session <oseid> --id <gen_id> [--section <section_id>] [--with-content]",
	Short:   "Read a generated block: file info, reference count, and its placements",
	Long: `Read a generated block inside an edit session.

Without --section the output lists every placement (instances: template +
target) with ref_count — the impact surface before an edit or delete.

With --section the output narrows to the instance in that section
(instance: template, target, settings): the three values "themes block +edit"
takes as --template / --target / --settings. Two placements in one section
yield an array. Pass --template when the section is not among the block's
recorded placements.

--with-content adds the liquid source (doc.content) and the display name
parsed from its schema. --session is required (see "themes +page").`,
	Flags: []common.Flag{
		{Name: "theme", Type: common.FlagString, Description: "Theme ID. Defaults to the published theme; required when the session is on another theme."},
		{Name: "session", Type: common.FlagString, Required: true, Description: "Edit session id (oseid) from 'themes +page'."},
		{Name: "id", Type: common.FlagString, Required: true, Description: "Block id (file name without extension, e.g. gen_1a0d523)."},
		{Name: "section", Type: common.FlagString, Description: "Section id holding the instance to read; returns that instance with its settings."},
		{Name: "template", Type: common.FlagString, Description: "Template page of --section, when it cannot be inferred from the placements."},
		{Name: "with-content", Type: common.FlagBool, Description: "Include the block's liquid source (can be large)."},
	},
	Execute: blockGetExecute,
}

func blockGetExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	themeID := in.Flags.GetString("theme")
	oseid := in.Flags.GetString("session")
	section := in.Flags.GetString("section")
	template := in.Flags.GetString("template")
	withContent := in.Flags.GetBool("with-content")

	if oseid == "" {
		return common.ExecResult{}, output.ErrValidation("--session is required").
			WithHint("create an edit session with `themes +page --template <name>` and pass its oseid")
	}
	cardType, _, err := normalizeGenType(in.Flags.GetString("id"))
	if err != nil {
		return common.ExecResult{}, err
	}
	if section != "" { // a full target is accepted; only its section id matters
		ref, perr := parseTarget(normalizeBlockTarget(section))
		if perr != nil {
			return common.ExecResult{}, perr
		}
		section = ref.SectionID
	}
	if template != "" {
		if _, _, err := templateLocation(template, ""); err != nil {
			return common.ExecResult{}, err
		}
	}

	if in.DryRun {
		plans := []common.PlannedRequest{PlanGetGenBlock(oseid, cardType, withContent)}
		if section != "" {
			themeRef, lookup := dryRunThemeRef(themeID)
			plans = append(plans, lookup...)
			plans = append(plans, PlanDocTree(themeRef), PlanSchemasList(oseid, phDocID))
		}
		return common.ExecResult{Plans: plans}, nil
	}

	resp, err := common.Send(ctx, in.Client, PlanGetGenBlock(oseid, cardType, withContent))
	if err != nil {
		return common.ExecResult{}, err
	}
	gen := unwrapData(resp)
	doc := mapField(gen, "doc")
	if doc == nil {
		return common.ExecResult{}, output.ErrInternal("gen-blocks response carries no doc")
	}
	instances := parseGenInstances(gen["instances"])
	refCount := 0
	if n, ok := numberValue(gen["ref_count"]); ok {
		refCount = int(n)
	}
	body := map[string]any{
		"type":      cardType,
		"doc":       doc,
		"saved":     gen["saved"] == true,
		"ref_count": refCount,
	}
	if c := getString(doc, "content"); c != "" {
		// Best-effort: the endpoint has no display name, so parse it from the
		// source when the caller already asked for the content.
		if schema, serr := extractSchema(c); serr == nil {
			if name := schemaDisplayName(schema); name != nil {
				body["name"] = name
			}
		}
	}
	if section == "" {
		body["instances"] = genInstanceRows(instances)
		return common.ExecResult{Body: body}, nil
	}

	// One section: locate its template, read the page, keep this block's rows.
	if template == "" {
		for _, it := range instances {
			if ref, perr := parseTarget(it.Target); perr == nil && ref.SectionID == section {
				template = it.Template
				break
			}
		}
	}
	if template == "" {
		return common.ExecResult{}, output.ErrValidation("section %q holds no recorded instance of %s", section, cardType).
			WithHint("check the instances list (omit --section), or pass --template <name> to read that page directly")
	}
	_, docID, err := resolveThemeAndDoc(ctx, in.Client, themeID, template, "")
	if err != nil {
		return common.ExecResult{}, err
	}
	inner, err := fetchSections(ctx, in.Client, oseid, docID)
	if err != nil {
		return common.ExecResult{}, err
	}
	sec := findSectionByID(inner, section)
	if sec == nil {
		return common.ExecResult{}, output.ErrValidation("section %q not found on template %s", section, template)
	}
	blocks, _ := sec["blocks"].([]any)
	var rows []map[string]any
	for _, b := range flattenBlocks(section, blocks) {
		if b.Type == cardType {
			rows = append(rows, map[string]any{"template": template, "target": b.Target, "settings": b.Settings})
		}
	}
	switch len(rows) {
	case 0:
		return common.ExecResult{}, output.ErrValidation("section %q on template %s has no block of type %s", section, template, cardType).
			WithHint("indexes shift after structural edits; list placements without --section")
	case 1:
		body["instance"] = rows[0]
	default:
		body["instance"] = rows
	}
	return common.ExecResult{Body: body}, nil
}
