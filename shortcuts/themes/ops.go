package themes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// ops parsing / validation / endpoint mapping for themes +edit.

// editOp is one entry of the +edit --ops array. Fields are a union across the
// nine op kinds; per-kind requirements are enforced by validateOps.
type editOp struct {
	Op     string `json:"op"`
	Target string `json:"target,omitempty"`

	Props map[string]any `json:"props,omitempty"` // update_slot / replace_props
	Value map[string]any `json:"value,omitempty"` // append_array_item: {type, settings}; add_section: {settings, blocks}

	Name       string `json:"name,omitempty"`        // add_section: section type
	ToIndex    *int   `json:"to_index,omitempty"`    // add_section / move_section / move_array_item (numeric)
	Position   string `json:"position,omitempty"`    // add_section / move_section: first|last|after:<sid>|before:<sid>
	Pb         bool   `json:"pb,omitempty"`          // add_section: insert a saved PB card
	TemplateID string `json:"template_id,omitempty"` // add_section pb mode source

	Visible *bool `json:"visible,omitempty"` // set_visibility

	Ops []map[string]any `json:"ops,omitempty"` // update_pb inner PB operations

	ref targetRef // parsed Target, filled by validateOps
}

var opNames = []string{
	"update_slot", "replace_props", "remove_array_item", "append_array_item",
	"move_array_item", "add_section", "remove_section", "move_section",
	"set_visibility", "update_pb",
}

// opExamples gives one minimal valid instance per op, surfaced in validation
// errors (the "example" field) so callers see the correct shape.
var opExamples = map[string]string{
	"update_slot":       `{"op":"update_slot","target":"<section_id>.blocks[0]","props":{"<field>":"<value>"}}`,
	"replace_props":     `{"op":"replace_props","target":"<section_id>","props":{"<field>":"<value>"}}`,
	"remove_array_item": `{"op":"remove_array_item","target":"<section_id>.blocks[1]"}`,
	"append_array_item": `{"op":"append_array_item","target":"<section_id>.blocks","value":{"type":"<block_type>","settings":{}}}`,
	"move_array_item":   `{"op":"move_array_item","target":"<section_id>.blocks[2]","to_index":0}`,
	"add_section":       `{"op":"add_section","name":"<section_type>","position":"last","value":{"settings":{},"blocks":[{"type":"<block_type>","settings":{}}]}}`,
	"remove_section":    `{"op":"remove_section","target":"<section_id>"}`,
	"move_section":      `{"op":"move_section","target":"<section_id>","position":"after:<other_section_id>"}`,
	"set_visibility":    `{"op":"set_visibility","target":"<section_id>","visible":false}`,
	"update_pb":         `{"op":"update_pb","target":"<pb_section_id>","ops":[{"action":"update","targetId":"0.0.1","settings":{}}]}`,
}

// moveItemIndex resolves move_array_item's destination index within the parent
// container. to_index is the CLI spelling (same as move_section); a digits-only
// position is accepted too, since that is what the endpoint itself calls the
// field. first/last stay unsupported here — they need the container length,
// which validation does not have.
func moveItemIndex(op *editOp) (int, error) {
	if op.ToIndex != nil {
		if *op.ToIndex < 0 {
			return 0, fmt.Errorf("to_index must be >= 0, got %d", *op.ToIndex)
		}
		return *op.ToIndex, nil
	}
	if op.Position == "" {
		return 0, fmt.Errorf("to_index is required (0-based position within the parent container)")
	}
	n, err := strconv.Atoi(op.Position)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("position %q is not a 0-based index — move_array_item reorders within one container, so use to_index", op.Position)
	}
	return n, nil
}

// validatePosition checks the add_section/move_section position surface
// (first | last | after:<sid> | before:<sid>).
func validatePosition(pos string) error {
	switch pos {
	case "first", "last":
		return nil
	}
	if strings.HasPrefix(pos, "after:") || strings.HasPrefix(pos, "before:") {
		if strings.TrimSpace(pos[strings.IndexByte(pos, ':')+1:]) == "" {
			return fmt.Errorf("position %q is missing a section id after ':'", pos)
		}
		return nil
	}
	return fmt.Errorf("invalid position %q (use first | last | after:<section_id> | before:<section_id>)", pos)
}

// validateSectionValue checks the optional add_section value: only settings
// (object) and blocks (array of {type, settings}) are accepted, so a misspelt
// key fails here instead of silently dropping out of the request.
func validateSectionValue(value map[string]any) error {
	for k := range value {
		if k != "settings" && k != "blocks" {
			return fmt.Errorf("value.%s is not supported (add_section value takes settings and blocks)", k)
		}
	}
	if v, ok := value["settings"]; ok && asMap(v) == nil {
		return fmt.Errorf("value.settings must be an object")
	}
	if v, ok := value["blocks"]; ok {
		blocks, isList := v.([]any)
		if !isList {
			return fmt.Errorf("value.blocks must be an array of {type, settings}")
		}
		for i, b := range blocks {
			if getString(asMap(b), "type") == "" {
				return fmt.Errorf("value.blocks[%d].type is required", i)
			}
		}
	}
	return nil
}

// parseOps decodes the --ops JSON array.
func parseOps(raw []byte) ([]editOp, error) {
	var ops []editOp
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, output.ErrValidation("--ops is not a valid JSON array: %v", err)
	}
	return ops, nil // an empty batch is syntax-legal; editExecute rules on it
}

// validateOps runs every network-free check (op whitelist, per-kind required
// fields, target kind, descending-index rule) and fills each op's parsed ref.
func validateOps(ops []editOp) error {
	// group key (section + parent path) → last seen remove index, ops order
	lastRemoved := map[string]int{}

	for i := range ops {
		op := &ops[i]
		fail := func(format string, args ...any) *output.ExitError {
			e := output.ErrValidation("op #%d (%s): %s", i, op.Op, fmt.Sprintf(format, args...)).
				WithField("invalid_op", i)
			if ex := opExamples[op.Op]; ex != "" {
				return e.WithField("example", ex) // known op: show the correct shape
			}
			return e.WithHint("valid ops: " + strings.Join(opNames, ", ")) // unknown op
		}

		needTarget := func(kind targetKind, desc string) error {
			if op.Target == "" {
				return fail("target is required")
			}
			ref, err := parseTarget(op.Target)
			if err != nil {
				return fail("%v", err)
			}
			if ref.Kind != kind {
				return fail("target must be %s, got %q", desc, op.Target)
			}
			op.ref = ref
			return nil
		}

		switch op.Op {
		case "update_slot":
			if err := needTarget(targetBlock, "a block path (copy it from the +page blocks list)"); err != nil {
				return err
			}
			if len(op.Props) == 0 {
				return fail("props is required")
			}
		case "replace_props":
			if err := needTarget(targetSection, "a section id"); err != nil {
				return err
			}
			if len(op.Props) == 0 {
				return fail("props is required")
			}
		case "remove_array_item":
			if err := needTarget(targetBlock, "a block path"); err != nil {
				return err
			}
			key := op.ref.SectionID + fmt.Sprint(op.ref.ParentPath)
			if prev, ok := lastRemoved[key]; ok && op.ref.BlockIndex >= prev {
				return fail("positional removals in the same container must be ordered by descending index (previous %d, got %d) — earlier removals shift later indexes", prev, op.ref.BlockIndex)
			}
			lastRemoved[key] = op.ref.BlockIndex
		case "append_array_item":
			if err := needTarget(targetContainer, "a container path (parent target + .blocks)"); err != nil {
				return err
			}
			if getString(op.Value, "type") == "" {
				return fail("value.type is required")
			}
		case "move_array_item":
			if err := needTarget(targetBlock, "a block path (copy it from the +page blocks list)"); err != nil {
				return err
			}
			if _, err := moveItemIndex(op); err != nil {
				return fail("%v", err)
			}
		case "add_section":
			if op.Pb {
				if op.TemplateID == "" {
					return fail("template_id is required when pb=true").
						WithHint(`discover addable pb template ids: themes list-card --params '{"source":"pb,custom"}'`)
				}
				if op.Value != nil {
					return fail("value is not accepted with pb=true (the card content comes from the template)")
				}
			} else {
				if op.Name == "" {
					return fail("name is required (the section type to add)")
				}
				if err := validateSectionValue(op.Value); err != nil {
					return fail("%v", err)
				}
			}
			if op.Position != "" {
				if err := validatePosition(op.Position); err != nil {
					return fail("%v", err)
				}
			}
		case "remove_section", "set_visibility", "move_section":
			if err := needTarget(targetSection, "a section id"); err != nil {
				return err
			}
			if op.Op == "set_visibility" && op.Visible == nil {
				return fail("visible is required")
			}
			if op.Op == "move_section" {
				if op.Position == "" && op.ToIndex == nil {
					return fail("position or to_index is required")
				}
				if op.Position != "" {
					if err := validatePosition(op.Position); err != nil {
						return fail("%v", err)
					}
				}
			}
		case "update_pb":
			if err := needTarget(targetSection, "the PB card's section id"); err != nil {
				return err
			}
			if len(op.Ops) == 0 {
				return fail("ops is required (the inner PB operation list)")
			}
		default:
			return fail("unknown op")
		}
	}
	return nil
}

// opsNeedImplicitRead reports whether applying ops needs the schemas-list read
// (custom_id lookup, append validation, position/area resolution).
func opsNeedImplicitRead(ops []editOp) bool {
	for _, op := range ops {
		switch op.Op {
		case "update_pb", "append_array_item", "move_array_item", "add_section", "remove_section", "move_section":
			return true
		}
	}
	return false
}
