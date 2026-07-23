package themes

import (
	"context"
	"fmt"
	"strings"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

// batch-ops translation layer for themes +edit: the --ops vocabulary maps to
// ThemeOperation entries of POST .../edit-sessions/{oseid}/templates/{doc}/operations.
// Whole batch in one request; ops apply and persist independently server-side.
//
// Server grammar (probed live 2026-07-23):
//   - block paths use dot indexes ("sid.blocks.0"), not brackets
//   - block-level props merge via replace_props with a block dot-path (the
//     server's update_slot replaces a block's child slot — a different op)
//   - placement is {position: before|after, move_target: <sid>}; only
//     move_section honors it, add_section always appends (a follow-up move
//     restores the requested position)
//   - add_section value is a full section object; server assigns the id
//     (client-supplied ids are ignored — new ids are recovered by re-reading)

// serverOp is one translated ThemeOperation plus its source op index
// (update_pb expands into two entries sharing one source index).
type serverOp struct {
	entry  map[string]any
	source int
}

// postMove is a placement fix-up sent in a follow-up batch: the section added
// by source op #source must end up before/after moveTarget.
type postMove struct {
	source     int
	position   string // "before" | "after"
	moveTarget string
}

// dotBlockPath renders a parsed block target in the server's dot-index grammar.
func dotBlockPath(ref targetRef) string {
	var b strings.Builder
	b.WriteString(ref.SectionID)
	for _, i := range ref.ParentPath {
		fmt.Fprintf(&b, ".blocks.%d", i)
	}
	fmt.Fprintf(&b, ".blocks.%d", ref.BlockIndex)
	return b.String()
}

// dotContainerPath renders a parsed container target in dot-index grammar.
func dotContainerPath(ref targetRef) string {
	var b strings.Builder
	b.WriteString(ref.SectionID)
	for _, i := range ref.ParentPath {
		fmt.Fprintf(&b, ".blocks.%d", i)
	}
	b.WriteString(".blocks")
	return b.String()
}

// resolveMoveRef translates the position surface (first|last|after:<sid>|
// before:<sid>) or a numeric to_index into the server's {position, move_target}
// pair, resolved against the section's area layout. inner==nil (dry-run)
// yields placeholders.
func resolveMoveRef(inner map[string]any, op editOp) (string, string) {
	if op.Position != "" {
		if kind, sid, ok := splitPosition(op.Position); ok { // after:<sid> / before:<sid>
			return kind, sid
		}
		// first / last resolve against the area layout
		if inner == nil {
			if op.Position == "first" {
				return "before", "<first_section_id>"
			}
			return "after", "<last_section_id>"
		}
		grp := sectionArea(inner, op.ref.SectionID)
		if grp == "" {
			grp = "page"
		}
		list := sectionsByArea(inner)[grp]
		if len(list) == 0 {
			return "", ""
		}
		if op.Position == "first" {
			return "before", anyToString(list[0]["id"])
		}
		return "after", anyToString(list[len(list)-1]["id"])
	}
	// numeric to_index: approximate as "before the section currently at n"
	// (tail indexes clamp to after-last).
	if op.ToIndex == nil {
		return "", ""
	}
	if inner == nil {
		return "before", "<section_id_at_to_index>"
	}
	grp := sectionArea(inner, op.ref.SectionID)
	if grp == "" {
		grp = "page"
	}
	list := sectionsByArea(inner)[grp]
	n := *op.ToIndex
	switch {
	case len(list) == 0:
		return "", ""
	case n <= 0:
		return "before", anyToString(list[0]["id"])
	case n >= len(list)-1:
		return "after", anyToString(list[len(list)-1]["id"])
	default:
		return "before", anyToString(list[n]["id"])
	}
}

// sectionValue builds the add_section value object for a plain (non-pb) add.
func sectionValue(name string) map[string]any {
	return map[string]any{"type": name, "name": name, "settings": map[string]any{}, "blocks": []any{}}
}

// successorOf returns the id of the section right after sid in its area
// layout ("" when sid is last or unknown).
func successorOf(inner map[string]any, sid string) string {
	if inner == nil {
		return ""
	}
	grp := sectionArea(inner, sid)
	if grp == "" {
		grp = "page"
	}
	list := sectionsByArea(inner)[grp]
	for i, m := range list {
		if anyToString(m["id"]) == sid && i+1 < len(list) {
			return anyToString(list[i+1]["id"])
		}
	}
	return ""
}

// translateOps maps the validated +edit ops onto server ThemeOperation
// entries plus the placement fix-ups that need the follow-up batch.
// cards holds the pre-generated theme card per update_pb op index; the
// returned newTargets echo appended block coordinates per op index.
func translateOps(ops []editOp, inner map[string]any, cards map[int]map[string]any) ([]serverOp, []postMove, map[int]string, error) {
	var entries []serverOp
	var moves []postMove
	newTargets := map[int]string{}
	fail := func(i int, format string, args ...any) error {
		e := output.ErrValidation("op #%d (%s): %s", i, ops[i].Op, fmt.Sprintf(format, args...)).
			WithField("invalid_op", i)
		if ex := opExamples[ops[i].Op]; ex != "" {
			e = e.WithField("example", ex)
		}
		return e
	}

	for i, op := range ops {
		switch op.Op {
		case "update_slot":
			// Block-level props merge is the server's replace_props with a
			// block dot-path (the server's own update_slot swaps child slots).
			entries = append(entries, serverOp{map[string]any{
				"op": "replace_props", "target": dotBlockPath(op.ref), "props": op.Props,
			}, i})
		case "replace_props":
			entries = append(entries, serverOp{map[string]any{
				"op": "replace_props", "target": op.ref.SectionID, "props": op.Props,
			}, i})
		case "append_array_item":
			if inner != nil { // schema gate + new_target echo need page data
				section := findSectionByID(inner, op.ref.SectionID)
				if section == nil {
					return nil, nil, nil, fail(i, "section %q not found on this page", op.ref.SectionID)
				}
				container, err := containerAt(section, op.ref.ParentPath)
				if err != nil {
					return nil, nil, nil, fail(i, "%v", err)
				}
				if err := validateAppend(inner, section, op.Value, len(container)); err != nil {
					return nil, nil, nil, fail(i, "%v", err)
				}
				newTargets[i] = fmt.Sprintf("%s[%d]", op.Target, len(container))
			}
			entries = append(entries, serverOp{map[string]any{
				"op": "append_array_item", "target": dotContainerPath(op.ref), "value": op.Value,
			}, i})
		case "remove_array_item":
			entries = append(entries, serverOp{map[string]any{
				"op": "remove_array_item", "target": dotBlockPath(op.ref),
			}, i})
		case "add_section":
			if op.Pb {
				return nil, nil, nil, fail(i, "add_section pb mode is not supported by batch-ops yet (no server-side pb instantiation contract)")
			}
			entries = append(entries, serverOp{map[string]any{
				"op": "add_section", "value": sectionValue(op.Name),
			}, i})
			// The server always appends; a requested position needs a
			// follow-up move once the new id is known.
			if op.Position != "" && op.Position != "last" || op.ToIndex != nil {
				if pos, ref := resolveMoveRef(inner, op); pos != "" && ref != "" {
					moves = append(moves, postMove{source: i, position: pos, moveTarget: ref})
				}
			}
		case "remove_section":
			entries = append(entries, serverOp{map[string]any{
				"op": "remove_section", "target": op.ref.SectionID,
			}, i})
		case "move_section":
			pos, ref := resolveMoveRef(inner, op)
			if pos == "" || ref == "" {
				return nil, nil, nil, fail(i, "cannot resolve position %q on this page", op.Position)
			}
			entries = append(entries, serverOp{map[string]any{
				"op": "move_section", "target": op.ref.SectionID, "position": pos, "move_target": ref,
			}, i})
		case "set_visibility":
			entries = append(entries, serverOp{map[string]any{
				"op": "set_visibility", "target": op.ref.SectionID, "visible": *op.Visible,
			}, i})
		case "update_pb":
			card := cards[i]
			if card == nil {
				return nil, nil, nil, fail(i, "internal: no generated theme card for update_pb")
			}
			// Replace in place: drop the old card, append the generated one,
			// then move it back before the old card's successor.
			entries = append(entries,
				serverOp{map[string]any{"op": "remove_section", "target": op.ref.SectionID}, i},
				serverOp{map[string]any{"op": "add_section", "value": card}, i},
			)
			if succ := successorOf(inner, op.ref.SectionID); succ != "" {
				moves = append(moves, postMove{source: i, position: "before", moveTarget: succ})
			}
		default:
			return nil, nil, nil, fail(i, "unknown op")
		}
	}
	return entries, moves, newTargets, nil
}

// batchResultStrings pulls the ordered result list out of a batch-ops
// response, tolerating the {data:{data:[{op,result}…]}} envelope.
func batchResultStrings(resp map[string]any) []string {
	root := resp
	for range 2 {
		if d := mapField(root, "data"); d != nil {
			root = d
		}
	}
	items := root["data"]
	if items == nil {
		items = root["results"]
	}
	var out []string
	for _, it := range mapSlice(items) {
		out = append(out, getString(it, "result"))
	}
	return out
}

// mapBatchResults folds per-entry results back onto source ops (an update_pb
// expansion fails if either of its two entries failed).
func mapBatchResults(n int, entries []serverOp, resp map[string]any) []string {
	results := batchResultStrings(resp)
	perOp := make([]string, n)
	for j, e := range entries {
		r := "unknown"
		if j < len(results) {
			r = results[j]
		}
		if perOp[e.source] == "" || perOp[e.source] == "success" {
			perOp[e.source] = r
		}
	}
	for i := range perOp {
		if perOp[i] == "" {
			perOp[i] = "success" // ops that expand to zero entries
		}
	}
	return perOp
}

// hasAdds reports whether the batch created sections (new ids to recover).
func hasAdds(entries []serverOp) bool {
	for _, e := range entries {
		if e.entry["op"] == "add_section" {
			return true
		}
	}
	return false
}

// placeSections re-reads the session to recover the server-assigned ids of
// newly added sections (echoed as applied[].new_section_id) and sends the
// follow-up move batch restoring requested positions. Failures degrade to a
// warning: the content ops already applied and are not rolled back.
func placeSections(ctx context.Context, c *client.Client, oseid, docID string, entries []serverOp, moves []postMove, preIDs map[string]bool, applied []map[string]any) string {
	inner, err := fetchSections(ctx, c, oseid, docID)
	if err != nil {
		return "could not re-read the session to recover new section ids: " + err.Error()
	}
	var newIDs []string
	for _, m := range allSections(inner) {
		if id := anyToString(m["id"]); id != "" && !preIDs[id] {
			newIDs = append(newIDs, id)
		}
	}
	// The server appends adds in batch order — assign recovered ids in order.
	bySource := map[int]string{}
	k := 0
	for _, e := range entries {
		if e.entry["op"] == "add_section" && k < len(newIDs) {
			bySource[e.source] = newIDs[k]
			k++
		}
	}
	for src, id := range bySource {
		if src < len(applied) {
			applied[src]["new_section_id"] = id
		}
	}
	if len(moves) == 0 {
		return ""
	}
	var moveOps []map[string]any
	for _, mv := range moves {
		id := bySource[mv.source]
		if id == "" {
			return fmt.Sprintf("op #%d: new section id unknown, requested placement skipped", mv.source)
		}
		moveOps = append(moveOps, map[string]any{
			"op": "move_section", "target": id, "position": mv.position, "move_target": mv.moveTarget,
		})
	}
	resp, err := common.Send(ctx, c, PlanBatchOps(oseid, docID, moveOps))
	if err != nil {
		return "placement moves failed: " + err.Error()
	}
	for _, r := range batchResultStrings(resp) {
		if r != "success" {
			return "placement move result: " + r
		}
	}
	return ""
}

// batchFailErr reports a batch with failed ops: everything else already
// applied and persisted (independent semantics — no abort, no rollback).
func batchFailErr(oseid string, created bool, results []map[string]any, failed []int) *output.ExitError {
	return output.Errorf(output.ExitAPI, output.TypeAPI, "%d of %d ops failed", len(failed), len(results)).
		WithField("results", results).
		WithField("failed", failed).
		WithField("oseid", oseid).
		WithField("session_created", created).
		WithHint("ops apply independently (no rollback) — the other ops are already persisted; fix the failed ops and resend ONLY them with --session " + oseid)
}

// generateThemeCard turns an update_pb op into a theme-card section object by
// calling pb-block-save (the interface owner's flow: pb generates the card,
// the batch then does remove_section + add_section).
func generateThemeCard(ctx context.Context, c *client.Client, op editOp, inner map[string]any, oseid, docID, themeID string) (map[string]any, error) {
	customID := phCustomID
	if inner != nil {
		section := findSectionByID(inner, op.ref.SectionID)
		if section == nil {
			return nil, output.ErrValidation("update_pb: section %q not found on this page", op.ref.SectionID)
		}
		id, ok := pbCustomID(getString(section, "type"))
		if !ok {
			return nil, output.ErrValidation("update_pb: section %q is not a page-builder custom card (type %q)", op.ref.SectionID, getString(section, "type"))
		}
		customID = id
	}
	resp, err := common.Send(ctx, c, PlanPbBlockSave(map[string]any{
		"event_type": "theme", "action": "save", // fixed values
		"origin_template_id": customID,
		"oseid":              oseid, "doc_id": docID, "section_id": op.ref.SectionID, "theme_id": themeID,
		"ops": op.Ops,
	}))
	if err != nil {
		return nil, err
	}
	card := extractThemeCard(resp)
	if card == nil {
		return nil, output.ErrInternal("pb-block-save returned no theme card for update_pb (section %s)", op.ref.SectionID)
	}
	return card, nil
}

// extractThemeCard digs the generated section object out of a pb-block-save
// response, tolerating data wrappers and a few plausible field names.
func extractThemeCard(resp map[string]any) map[string]any {
	root := resp
	for i := 0; i < 2; i++ {
		if d := mapField(root, "data"); d != nil {
			root = d
		}
	}
	for _, key := range []string{"section", "card", "theme_card", "block"} {
		if m := mapField(root, key); m != nil && getString(m, "type") != "" {
			return m
		}
	}
	if getString(root, "type") != "" && (root["settings"] != nil || root["schema"] != nil) {
		return root
	}
	return nil
}
