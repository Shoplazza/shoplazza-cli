package themes

import (
	"context"
	"fmt"
	"slices"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
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

--id is the whole switch: without it the block is created and its instance
appended, with it that block's source is rewritten. An update carries the
instance's current settings onto the new schema (new fields take their schema
default), and the server forks the file when the block is referenced 2+ times
— only the targeted instance moves to the fork (branched:true,
previous_type), every other reference keeps the old block.

When an op that has to land does not, the write is rolled back and the error
carries stage:"place" with reverted:true — the session is where it was, so
send the same command again. revert_failed:true instead means the block file
stayed behind and a second send would write another one.

Ops the placement can live without (--ops, --section-name) never fail the
call: the block landed, and their names come back in degraded.

--section-name is the display name of the "_blocks" container section the
block sits in, stored as that section's settings.title: it names the
container the CLI adds, and renames the one addressed by --target.

Saving and publishing stay with the shared session:
"themes +edit --session <oseid> --ops '[]' --promote [--publish]".`,
	Flags: []common.Flag{
		{Name: "theme", Type: common.FlagString, Description: "Theme ID. Defaults to the published theme; required when the session is on another theme."},
		{Name: "session", Type: common.FlagString, Required: true, Description: "Edit session id (oseid) from 'themes +page'."},
		{Name: "id", Type: common.FlagString, Description: "Block id to update (file name without extension, e.g. gen_1a0d523). Omit to create."},
		{Name: "template", Type: common.FlagString, Description: "Template page to place the block on, e.g. index / product. Omit to write the file only."},
		{Name: "target", Type: common.FlagString, Description: "Where to place: a container path (<sid>.blocks) when creating, the instance path (<sid>.blocks[N]) when updating. Omit when creating and a \"_blocks\" container is added for it."},
		{Name: "content", Type: common.FlagString, Required: true, Description: "Liquid source: a file path, or '-' for stdin. Must contain a {% schema %} tag."},
		{Name: "settings", Type: common.FlagString, Description: "Update only: the instance's current settings (JSON object or file). Defaults to the values read from --target."},
		{Name: "ops", Type: common.FlagString, Description: "Setting keys to change on the placed instance (JSON object or file), merged server-side. Requires --template and --target."},
		{Name: "section-name", Type: common.FlagString, Description: "Display name of the \"_blocks\" container section the block sits in, stored as its settings.title. Names the container the CLI adds, or renames the one addressed by --target. Requires --template."},
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
	sectionName := in.Flags.GetString("section-name")

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
	if sectionName != "" && template == "" {
		return common.ExecResult{}, output.ErrValidation("--section-name requires --template").
			WithHint("--section-name names the container section the block lands in, so the placement must be addressed")
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
		return common.ExecResult{Plans: blockEditDryRunPlans(themeID, oseid, cardType, template, sectionName, ref, content, settings, ops)}, nil
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

	// Place it in one batch. Each op carries a name: the ones that have to land
	// fail the call and are undone, the rest only report.
	instance := map[string]any{"type": newType, "settings": instSettings}
	var operations []map[string]any
	var names []string
	add := func(name string, op map[string]any) {
		operations, names = append(operations, op), append(names, name)
	}
	degradable := map[string]bool{"ops": true, "section_name": true}
	instTarget, instDot := target, dotBlockPath(ref)
	containerSID := ref.SectionID
	sectionCreated := false
	switch {
	case id == "":
		// Creating always ends in an append — the block is what goes into a
		// container. Without --target that container is a "_blocks" section
		// added first in the same batch, under an id the CLI picks so the
		// append can address it.
		container, at, base := dotContainerPath(ref), containerLen, target
		if target == "" {
			sectionCreated = true
			sid := newSectionID()
			containerSID = sid
			container, at, base = sid+".blocks", 0, sid+".blocks"
			add("add_section", map[string]any{
				"op": "add_section", "section_id": sid,
				"value": map[string]any{"type": genSectionType, "name": genSectionType, "settings": map[string]any{}, "blocks": []any{}},
			})
		}
		instTarget = fmt.Sprintf("%s[%d]", base, at)
		instDot = fmt.Sprintf("%s.%d", container, at)
		add("append", map[string]any{"op": "append_array_item", "target": container, "value": instance})
	case branched:
		// The instance keeps its slot and is repointed at the fork. update_slot
		// is the op that takes a type (replace_props answers invalid_field:type)
		// and it ignores settings passed alongside, so migration follows in its
		// own op.
		add("type", map[string]any{"op": "update_slot", "target": instDot, "props": map[string]any{"type": newType}})
		add("migrate", map[string]any{"op": "replace_props", "target": instDot, "props": instSettings})
	default:
		add("migrate", map[string]any{"op": "replace_props", "target": instDot, "props": instSettings})
	}
	if ops != nil {
		add("ops", map[string]any{"op": "replace_props", "target": instDot, "props": ops})
	}
	// The name always travels as its own op, never inside the add_section
	// value: a container whose schema does not declare the field would fail
	// the add itself, taking the whole placement with it.
	if sectionName != "" {
		add("section_name", map[string]any{"op": "replace_props", "target": containerSID, "props": containerSettings(sectionName)})
	}

	previewURLFor := previewURLLater(ctx, in.Client, themeID, template, "")
	applied := make([]map[string]any, 0, len(operations))

	results, err := runOps(ctx, in.Client, oseid, docID, operations, names, &applied)
	if err != nil {
		e := blockStageErr(err, "place", oseid)
		if exitErr, ok := e.(*output.ExitError); ok {
			exitErr.WithField("block_type", newType).WithField("revert_id", revertID)
		}
		return common.ExecResult{}, e
	}
	var fatal, degraded []string
	for _, n := range names {
		switch {
		case results[n] == opSucceeded:
		case degradable[n]:
			degraded = append(degraded, n)
		default:
			fatal = append(fatal, n)
		}
	}
	if len(fatal) > 0 {
		// Put back only what landed: the container this call added, and — on the
		// fork — the type and settings the instance held before.
		undo := placementUndo{instance: instDot}
		if sectionCreated && results["add_section"] == opSucceeded {
			undo.container = containerSID
		}
		if results["type"] == opSucceeded {
			undo.prevType = cardType
		}
		if results["migrate"] == opSucceeded && branched {
			undo.prevProps = current
		}
		undone := revertPlacement(ctx, in.Client, oseid, docID, revertID, undo)
		e := blockPlaceFailErr(oseid, newType, revertID, applied, fatal, undone)
		if sectionCreated {
			e.WithField("container", containerSID+".blocks")
		} else if id != "" {
			e.WithField("instance_target", instTarget)
		}
		return common.ExecResult{}, e
	}

	body["applied"] = applied
	inst := map[string]any{"template": template, "target": instTarget, "section_created": sectionCreated}
	if sectionName != "" && !slices.Contains(degraded, "section_name") {
		inst["section_name"] = sectionName
	}
	body["instance"] = inst
	if len(degraded) > 0 {
		body["degraded"] = degraded
	}
	body["preview_url"] = previewURLFor(themeID, oseid)
	return common.ExecResult{Body: body}, nil
}

const opSucceeded = "success"

// runOps sends one batch, records a row per op in applied, and returns each
// op's result by name. A request that fails outright leaves every result empty.
func runOps(ctx context.Context, c *client.Client, oseid, docID string, operations []map[string]any, names []string, applied *[]map[string]any) (map[string]string, error) {
	out := map[string]string{}
	if len(operations) == 0 {
		return out, nil
	}
	resp, err := common.Send(ctx, c, PlanBatchOps(oseid, docID, operations))
	if err != nil {
		return out, err
	}
	results := batchResultStrings(resp)
	for i, op := range operations {
		res := ""
		if i < len(results) {
			res = results[i]
		}
		entry := map[string]any{"op": op["op"], "result": res}
		if t, ok := op["target"]; ok {
			entry["target"] = t
		}
		*applied = append(*applied, entry)
		out[names[i]] = res
	}
	return out, nil
}

// placementUndo is what a failed call did land on the page, to be put back:
// the container the CLI added, and what the instance held before a fork
// repointed it.
type placementUndo struct {
	container string
	instance  string
	prevType  string
	prevProps map[string]any
}

// revertPlacement puts the session back to where this call found it after an
// op that had to land failed. The page goes first: reverting the block while an
// instance still points at it would leave that reference dangling, so a page it
// cannot restore stops the rollback. Returns why it could not, or "".
func revertPlacement(ctx context.Context, c *client.Client, oseid, docID, revertID string, undo placementUndo) string {
	var ops []map[string]any
	if undo.prevType != "" {
		ops = append(ops, map[string]any{"op": "update_slot", "target": undo.instance, "props": map[string]any{"type": undo.prevType}})
	}
	if undo.prevProps != nil {
		ops = append(ops, map[string]any{"op": "replace_props", "target": undo.instance, "props": undo.prevProps})
	}
	if undo.container != "" {
		ops = append(ops, map[string]any{"op": "remove_section", "target": undo.container})
	}
	if len(ops) > 0 {
		resp, err := common.Send(ctx, c, PlanBatchOps(oseid, docID, ops))
		if err != nil {
			return "the page changes could not be put back: " + err.Error()
		}
		for i, res := range batchResultStrings(resp) {
			if res != opSucceeded {
				return "the page change " + getString(ops[i], "op") + " could not be put back: " + res
			}
		}
	}
	if _, err := common.Send(ctx, c, PlanRevertGenBlock(oseid, revertID)); err != nil {
		return "the block write could not be reverted: " + err.Error()
	}
	return ""
}

// containerSettings builds the props of the rename op: the "_blocks" container
// section's display name lives in settings.title, declared as a field of the
// container's own schema.
func containerSettings(name string) map[string]any {
	return map[string]any{"title": name}
}

// blockEditDryRunPlans lists every intended request without sending any.
func blockEditDryRunPlans(themeID, oseid, cardType, template, sectionName string, ref targetRef, content string, settings, ops map[string]any) []common.PlannedRequest {
	var plans []common.PlannedRequest
	themeRef := themeID
	if template != "" {
		var lookup []common.PlannedRequest
		themeRef, lookup = dryRunThemeRef(themeID)
		plans = append(plans, lookup...)
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
	containerSID := ref.SectionID
	switch {
	case cardType == "":
		container := dotContainerPath(ref)
		dot = container + ".<new_index>"
		if ref.SectionID == "" { // no --target: the CLI adds the container first
			containerSID = phSectionID
			container, dot = phSectionID+".blocks", phSectionID+".blocks.0"
			operations = append(operations, map[string]any{
				"op": "add_section", "section_id": phSectionID,
				"value": map[string]any{"type": genSectionType, "name": genSectionType, "settings": map[string]any{}, "blocks": []any{}},
			})
		}
		operations = append(operations, map[string]any{"op": "append_array_item", "target": container, "value": instance})
	default:
		dot = dotBlockPath(ref)
		operations = append(operations, map[string]any{"op": "replace_props", "target": dot, "props": phGenSettings})
	}
	// One batch, like a real run. The rollback a failure would trigger is not
	// planned — it only happens on failure.
	if ops != nil && dot != "" {
		operations = append(operations, map[string]any{"op": "replace_props", "target": dot, "props": ops})
	}
	if sectionName != "" {
		operations = append(operations, map[string]any{"op": "replace_props", "target": containerSID,
			"props": containerSettings(sectionName)})
	}
	return append(plans, PlanBatchOps(oseid, phDocID, operations))
}
