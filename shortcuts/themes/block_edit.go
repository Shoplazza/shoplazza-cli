package themes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// themes block +edit — write an AI-generated block file and place it on a page.

var blockEditShortcut = common.Shortcut{
	Service: "themes block",
	Command: "+edit",
	Use:     "+edit --session <oseid> --content <file|-> [--id <gen_id>] [--template <name> [--target <path>]]",
	Short:   "Write a generated block file (create or update) and place it on a template page",
	Long: `Write a generated block's liquid source inside an edit session and, when
--template is given, place it on that page in the same call.

--id decides the mode:
  omitted   create a new block file (the server names it); --target must be a
            container path (<section_id>.blocks) and the instance is appended
            there. Omit --target to wrap it in a new "_blocks" section.
  given     update that block's source; --target must be the instance path
            (<section_id>.blocks[N]) copied from "themes block +get --section".
            The instance's current settings are read from the page and carried
            onto the new schema (new fields take schema defaults). When the block
            is referenced 2+ times the server branches it into a new file
            (branched:true, previous_type); only the targeted instance switches,
            the other references keep the old block.

--settings overrides the current values used for that migration (a JSON
object, or a file). --ops merges extra setting keys into the placed instance
as a second operation (only the keys to change). Omitting --template writes
the file only (instance:null).

--session is required: create one with "themes +page" and reuse its oseid; pass
--theme when the session belongs to a theme other than the published one. Save
or publish with "themes +edit --session <oseid> --ops '[]' --promote [--publish]".

Placement failures after a successful write return an api error with
stage:"place" plus block_type and revert_id, so the write can be re-placed
or reverted (themes block revert-gen) rather than repeated.`,
	Flags: []common.Flag{
		{Name: "theme", Type: common.FlagString, Description: "Theme ID. Defaults to the published theme; required when the session is on another theme."},
		{Name: "session", Type: common.FlagString, Required: true, Description: "Edit session id (oseid) from 'themes +page'."},
		{Name: "id", Type: common.FlagString, Description: "Block id to update (file name without extension, e.g. gen_1a0d523). Omit to create."},
		{Name: "template", Type: common.FlagString, Description: "Template page to place the block on, e.g. index / product. Omit to write the file only."},
		{Name: "target", Type: common.FlagString, Description: "Where to place: a container path (<sid>.blocks) when creating, the instance path (<sid>.blocks[N]) when updating."},
		{Name: "content", Type: common.FlagString, Required: true, Description: "Liquid source: a file path, or '-' for stdin. Must contain a {% schema %} tag."},
		{Name: "settings", Type: common.FlagString, Description: "Update only: the instance's current settings (JSON object or file). Defaults to the values read from --target."},
		{Name: "ops", Type: common.FlagString, Description: "Setting keys to change on the placed instance (JSON object or file), merged server-side. Requires --template and --target."},
	},
	Execute: blockEditExecute,
}

func blockEditExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	themeID := in.Flags.GetString("theme")
	oseid := in.Flags.GetString("session")
	id := in.Flags.GetString("id")
	template := in.Flags.GetString("template")
	target := in.Flags.GetString("target")
	contentArg := in.Flags.GetString("content")
	settingsArg := in.Flags.GetString("settings")
	opsArg := in.Flags.GetString("ops")

	// Network-free validation first.
	if oseid == "" {
		return common.ExecResult{}, output.ErrValidation("--session is required").
			WithHint("create an edit session with `themes +page --template <name>` and pass its oseid")
	}
	if contentArg == "" {
		return common.ExecResult{}, output.ErrValidation("--content is required").
			WithHint("pass the liquid source as a file path, or '-' to read it from stdin")
	}
	if target != "" && template == "" {
		return common.ExecResult{}, output.ErrValidation("--target requires --template")
	}
	if opsArg != "" && (template == "" || target == "") {
		return common.ExecResult{}, output.ErrValidation("--ops requires --template and --target").
			WithHint("--ops changes settings on the placed instance, so the placement must be addressed")
	}
	if id != "" && template != "" && target == "" {
		return common.ExecResult{}, output.ErrValidation("updating with --template requires --target").
			WithHint("pass the instance path to repoint, e.g. --target <section_id>.blocks[N] from `themes block +get --section`; omit --template to only rewrite the file")
	}
	if settingsArg != "" && id == "" {
		return common.ExecResult{}, output.ErrValidation("--settings only applies with --id").
			WithHint("a new block starts from its schema defaults; use --ops to change values on the placed instance")
	}
	if countStdin(contentArg, settingsArg, opsArg) > 1 {
		return common.ExecResult{}, output.ErrValidation("only one of --content / --settings / --ops can read stdin ('-')")
	}
	var cardType string
	if id != "" {
		var err error
		if cardType, _, err = normalizeGenType(id); err != nil {
			return common.ExecResult{}, err
		}
	}
	if template != "" {
		if _, _, err := templateLocation(template, ""); err != nil {
			return common.ExecResult{}, err
		}
	}
	var ref targetRef
	if target != "" {
		target = normalizeBlockTarget(target)
		var err error
		if ref, err = parseTarget(target); err != nil {
			return common.ExecResult{}, err
		}
		switch {
		case id == "" && ref.Kind != targetContainer:
			return common.ExecResult{}, output.ErrValidation("--target %q must be a container path when creating a block", target).
				WithHint("end the target with .blocks (e.g. <section_id>.blocks); to update an existing instance pass --id")
		case id != "" && ref.Kind != targetBlock:
			return common.ExecResult{}, output.ErrValidation("--target %q must be an instance path when updating --id %s", target, id).
				WithHint("copy the target from `themes block +get --id <id> --section <section_id>` (e.g. <section_id>.blocks[0])")
		}
	}
	content, err := readContentInput(contentArg)
	if err != nil {
		return common.ExecResult{}, err
	}
	settings, err := readJSONObjectInput("--settings", settingsArg)
	if err != nil {
		return common.ExecResult{}, err
	}
	if t := getString(settings, "type"); t != "" && t != cardType {
		return common.ExecResult{}, output.ErrValidation("--settings.type %q does not match --id %s", t, id).
			WithHint("the server picks the block to update from settings.type; pass the target instance's own settings")
	}
	ops, err := readJSONObjectInput("--ops", opsArg)
	if err != nil {
		return common.ExecResult{}, err
	}

	if in.DryRun {
		return common.ExecResult{Plans: blockEditDryRunPlans(themeID, oseid, cardType, template, ref, content, settings, ops)}, nil
	}

	// Page context: only when placing.
	var docID string
	var inner map[string]any
	var current map[string]any // the targeted instance's current settings (update)
	var containerLen int       // children in the target container (create)
	if template != "" {
		themeID, docID, err = resolveThemeAndDoc(ctx, in.Client, themeID, template, "")
		if err != nil {
			return common.ExecResult{}, err
		}
		if inner, err = fetchSections(ctx, in.Client, oseid, docID); err != nil {
			return common.ExecResult{}, err
		}
		if target != "" {
			section := findSectionByID(inner, ref.SectionID)
			if section == nil {
				return common.ExecResult{}, output.ErrValidation("section %q not found on template %s", ref.SectionID, template).
					WithHint("run `themes +page --template " + template + " --session " + oseid + "` and copy a target from its output")
			}
			_, children, cerr := containerAt(section, ref.ParentPath)
			if cerr != nil {
				return common.ExecResult{}, output.ErrValidation("invalid --target %q: %v", target, cerr)
			}
			containerLen = len(children)
			if id != "" {
				if ref.BlockIndex >= len(children) {
					return common.ExecResult{}, output.ErrValidation("--target %q is out of range: the container holds %d blocks", target, len(children)).
						WithHint("indexes shift after structural edits; re-read with `themes block +get --id " + id + " --section " + ref.SectionID + "`")
				}
				blk := asMap(children[ref.BlockIndex])
				if got := getString(blk, "type"); got != cardType {
					return common.ExecResult{}, output.ErrValidation("the block at %q is %q, not %s", target, got, cardType).
						WithHint("re-read the instance with `themes block +get --id " + id + " --section " + ref.SectionID + "`")
				}
				current = mapField(blk, "settings")
			}
		}
	}

	// Write the block file.
	var resp map[string]any
	settingsDefaulted := false
	if id == "" {
		resp, err = common.Send(ctx, in.Client, PlanCreateGenBlock(oseid, content))
	} else {
		body := settings
		if body == nil {
			if current == nil {
				settingsDefaulted = true
				body = map[string]any{"type": cardType}
			} else {
				body = map[string]any{"type": cardType, "settings": current}
			}
		} else if _, ok := body["type"]; !ok {
			current = body // a bare values object stands in for the page read
			body = map[string]any{"type": cardType, "settings": body}
		}
		resp, err = common.Send(ctx, in.Client, PlanUpdateGenBlock(oseid, content, body))
	}
	if err != nil {
		return common.ExecResult{}, blockStageErr(err, "write", oseid)
	}
	gen := unwrapData(resp)
	schema := mapField(gen, "settings")
	newType := getString(schema, "type")
	if newType == "" {
		return common.ExecResult{}, output.ErrInternal("gen-blocks response carries no settings.type")
	}
	revertID := getString(gen, "revert_id")
	branched := gen["branched"] == true
	defaults := presetSettings(schema)
	instSettings := defaults
	if id != "" {
		instSettings = migrateSettings(defaults, schemaSettingIDs(schema), current)
	}
	bare := newType[len(genTypePrefix):]
	body := map[string]any{
		"type":      newType,
		"doc":       map[string]any{"id": bare, "location": bare + ".liquid"},
		"settings":  instSettings,
		"ops":       nil,
		"branched":  branched,
		"revert_id": revertID,
		"oseid":     oseid,
		"instance":  nil,
	}
	if ops != nil {
		body["ops"] = ops
	}
	if branched {
		body["previous_type"] = cardType
	}
	if settingsDefaulted {
		body["settings_defaulted"] = true
	}
	if docID == "" {
		return common.ExecResult{Body: body}, nil
	}

	// Place it: one batch, ops apply independently server-side.
	instance := map[string]any{"type": newType, "settings": instSettings}
	var operations []map[string]any
	instTarget := target
	sectionCreated := false
	switch {
	case id == "" && target != "":
		instTarget = fmt.Sprintf("%s[%d]", target, containerLen)
		operations = append(operations, map[string]any{"op": "append_array_item", "target": dotContainerPath(ref), "value": instance})
	case id == "":
		sectionCreated = true
		operations = append(operations, map[string]any{"op": "add_section", "value": map[string]any{
			"type": genSectionType, "name": genSectionType, "settings": map[string]any{}, "blocks": []any{instance},
		}})
	case branched:
		// The instance must repoint to the new (branched) card. The server's
		// in-place type-swap validates fields against the wrong schema, so
		// replace the block: drop it, append the new type with migrated
		// settings (appends last), then move it back to its slot.
		operations = append(operations,
			map[string]any{"op": "remove_array_item", "target": dotBlockPath(ref)},
			map[string]any{"op": "append_array_item", "target": dotContainerPath(ref), "value": instance},
		)
		if containerLen-1 != ref.BlockIndex {
			operations = append(operations, map[string]any{"op": "move_array_item", "target": dotContainerPath(ref),
				"move_target": strconv.Itoa(containerLen - 1), "position": strconv.Itoa(ref.BlockIndex)})
		}
	default:
		operations = append(operations, map[string]any{"op": "replace_props", "target": dotBlockPath(ref), "props": instSettings})
	}
	if ops != nil {
		dot := dotBlockPath(ref)
		if id == "" {
			dot = fmt.Sprintf("%s.%d", dotContainerPath(ref), containerLen)
		}
		operations = append(operations, map[string]any{"op": "replace_props", "target": dot, "props": ops})
	}

	domainCh := make(chan string, 1)
	go func() { domainCh <- extractStoreDomainBest(ctx, in.Client) }()
	pathCh := make(chan string, 1)
	go func() { pathCh <- resolvePreviewPath(ctx, in.Client, template, "") }()

	preIDs := map[string]bool{}
	for _, m := range allSections(inner) {
		preIDs[anyToString(m["id"])] = true
	}
	bresp, err := common.Send(ctx, in.Client, PlanBatchOps(oseid, docID, operations))
	if err != nil {
		e := blockStageErr(err, "place", oseid)
		if exitErr, ok := e.(*output.ExitError); ok {
			exitErr.WithField("block_type", newType).WithField("revert_id", revertID)
		}
		return common.ExecResult{}, e
	}
	results := batchResultStrings(bresp)
	applied := make([]map[string]any, 0, len(operations))
	var failed []int
	for i, op := range operations {
		res := ""
		if i < len(results) {
			res = results[i]
		}
		entry := map[string]any{"op": op["op"], "result": res}
		if t, ok := op["target"]; ok {
			entry["target"] = t
		}
		if res != "success" {
			failed = append(failed, i)
		}
		applied = append(applied, entry)
	}
	if len(failed) > 0 {
		return common.ExecResult{}, blockPlaceFailErr(oseid, newType, revertID, applied, failed)
	}
	if sectionCreated { // the server assigns the new section id; recover it by diff
		if after, rerr := fetchSections(ctx, in.Client, oseid, docID); rerr == nil {
			for _, m := range allSections(after) {
				if sid := anyToString(m["id"]); sid != "" && !preIDs[sid] {
					instTarget = sid + ".blocks[0]"
					break
				}
			}
		}
		if instTarget == "" {
			body["placement_warning"] = "could not recover the new section id; re-read with themes block +get"
		}
	}
	body["applied"] = applied
	body["instance"] = map[string]any{"template": template, "target": instTarget, "section_created": sectionCreated}
	body["preview_url"] = buildPreviewURL(<-domainCh, <-pathCh, themeID, oseid, "")
	return common.ExecResult{Body: body}, nil
}

// blockEditDryRunPlans lists every intended request without sending any.
func blockEditDryRunPlans(themeID, oseid, cardType, template string, ref targetRef, content string, settings, ops map[string]any) []common.PlannedRequest {
	var plans []common.PlannedRequest
	themeRef := themeID
	if template != "" {
		if themeRef == "" {
			themeRef = phThemeID
			plans = append(plans, PlanThemesList(map[string]any{"published": "1"}))
		}
		plans = append(plans, PlanDocTree(themeRef), PlanSchemasList(oseid, phDocID))
	}
	if cardType == "" {
		plans = append(plans, PlanCreateGenBlock(oseid, content))
	} else {
		body := settings
		if body == nil {
			body = map[string]any{"type": cardType, "settings": phCurrentValues}
		} else if _, ok := body["type"]; !ok {
			body = map[string]any{"type": cardType, "settings": body}
		}
		plans = append(plans, PlanUpdateGenBlock(oseid, content, body))
	}
	if template == "" {
		return plans
	}
	instance := map[string]any{"type": phGenBlockType, "settings": phGenSettings}
	var operations []map[string]any
	var dot string
	switch {
	case cardType == "" && ref.SectionID != "":
		dot = dotContainerPath(ref) + ".<new_index>"
		operations = append(operations, map[string]any{"op": "append_array_item", "target": dotContainerPath(ref), "value": instance})
	case cardType == "":
		operations = append(operations, map[string]any{"op": "add_section", "value": map[string]any{
			"type": genSectionType, "name": genSectionType, "settings": map[string]any{}, "blocks": []any{instance},
		}})
	default:
		dot = dotBlockPath(ref)
		operations = append(operations, map[string]any{"op": "replace_props", "target": dot, "props": phGenSettings})
	}
	if ops != nil && dot != "" {
		operations = append(operations, map[string]any{"op": "replace_props", "target": dot, "props": ops})
	}
	return append(plans, PlanBatchOps(oseid, phDocID, operations))
}
