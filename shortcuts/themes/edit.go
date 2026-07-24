package themes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// themes +edit — one-shot batch write for agent-driven theme editing
// (docs/theme-page-edit-shortcuts.md §4, docs/plans/theme-page-edit/03-themes-edit.md).
//
// Standard flow: pass the oseid echoed by `themes +page` so read and write
// share one edit-draft snapshot. Omitting --session creates a fresh session —
// legal, but the ops then apply to a snapshot you never read.

var editShortcut = common.Shortcut{
	Service: "themes",
	Command: "+edit",
	Use:     "+edit",
	Short:   "Apply a batch of edit ops to a template page inside one edit session",
	Long: `Apply a batch of edit operations to one template page: session handling,
one batch-operations request for the whole array (theme cards and
page-builder cards can mix) and a ready-to-share preview URL — one call.

Targets are copied verbatim from "themes +page" output. Standard flow passes
the oseid echoed by +page via --session so read and write share one snapshot;
omitting --session creates a fresh session (echoed back for follow-ups).

Ops (JSON array via --ops <file> | - (stdin) | inline JSON):
  update_slot        block target + props        merge props into a block's settings
  replace_props      section target + props      merge props into a section's settings
  remove_array_item  block target                remove a block (same-container batches: descending index)
  append_array_item  container target + value    append {type, settings} (validated against schema/max_blocks)
  add_section        name                        add a section (position: first|last|after:<sid>|before:<sid>)
  remove_section     section target              remove a section
  move_section       section target + position   reorder a section (position, or numeric to_index)
  set_visibility     section target + visible    show/hide a section
  update_pb          PB section target + ops     regenerate the PB card via pb and swap it in place

Failure semantics: ops apply and persist independently server-side — a
failure does not stop or roll back the others. A partial failure returns an
api error carrying per-op results; fix the failed ops and resend ONLY them
with --session.

--promote saves the edit draft back onto the theme draft after all ops apply
(reserve it for explicit user instruction; a conflict returns an api error
with conflict=true and never forces).`,
	Flags: []common.Flag{
		{Name: "template", Type: common.FlagString, Description: "Template name, e.g. index / product. Mutually exclusive with --file."},
		{Name: "file", Type: common.FlagString, Description: "Theme file path, e.g. templates/index.liquid. Mutually exclusive with --template."},
		{Name: "theme", Type: common.FlagString, Description: "Theme ID. Defaults to the published theme."},
		{Name: "session", Type: common.FlagString, Description: "Edit session id (oseid) — pass the one echoed by `themes +page`. Omit to create a fresh session."},
		{Name: "ops", Type: common.FlagString, Required: true, Description: "Edit operations: a file path, '-' for stdin, or an inline JSON array."},
		{Name: "promote", Type: common.FlagBool, Description: "Promote the edit draft onto the theme draft after all ops apply (needs explicit user intent)."},
	},
	Execute: editExecute,
}

func editExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	themeID := in.Flags.GetString("theme")
	template := in.Flags.GetString("template")
	file := in.Flags.GetString("file")
	session := in.Flags.GetString("session")
	promote := in.Flags.GetBool("promote")

	// All network-free checks run before any request (or side effect).
	if _, _, err := templateLocation(template, file); err != nil {
		return common.ExecResult{}, err
	}
	raw, err := readOpsInput(in.Flags.GetString("ops"))
	if err != nil {
		return common.ExecResult{}, err
	}
	ops, err := parseOps(raw)
	if err != nil {
		return common.ExecResult{}, err
	}
	if err := validateOps(ops); err != nil {
		return common.ExecResult{}, err
	}

	if in.DryRun {
		return common.ExecResult{Plans: editDryRunPlans(themeID, session, ops, promote)}, nil
	}

	themeID, docID, err := resolveThemeAndDoc(ctx, in.Client, themeID, template, file)
	if err != nil {
		return common.ExecResult{}, err
	}
	oseid, created := session, false
	if oseid == "" {
		resp, err := common.Send(ctx, in.Client, PlanCreateSession(themeID))
		if err != nil {
			return common.ExecResult{}, err
		}
		if oseid = extractOseid(resp); oseid == "" {
			return common.ExecResult{}, output.ErrInternal("create-session returned no oseid")
		}
		created = true
	}

	// Implicit read only when the batch needs page data (custom_id lookup,
	// append validation, or section placement/area).
	var inner map[string]any
	if opsNeedImplicitRead(ops) {
		if inner, err = fetchSections(ctx, in.Client, oseid, docID); err != nil {
			return common.ExecResult{}, err
		}
	}
	// update_pb pre-flight: pb-block-save generates the replacement theme
	// card that the batch then swaps in via remove_section + add_section.
	cards := map[int]map[string]any{}
	for i := range ops {
		if ops[i].Op == "update_pb" {
			card, cerr := generateThemeCard(ctx, in.Client, ops[i], inner, oseid, docID, themeID)
			if cerr != nil {
				if exitErr, ok := cerr.(*output.ExitError); ok {
					exitErr.WithField("oseid", oseid).WithField("session_created", created)
				}
				return common.ExecResult{}, cerr
			}
			cards[i] = card
		}
	}
	entries, moves, newTargets, err := translateOps(ops, inner, cards)
	if err != nil {
		if exitErr, ok := err.(*output.ExitError); ok {
			exitErr.WithField("oseid", oseid).WithField("session_created", created)
		}
		return common.ExecResult{}, err
	}

	// Prefetch the preview-URL inputs (store domain via GET /shop; storefront
	// path from the edited template, resource pages via one page_size=1 read)
	// concurrently with the batch. Buffered so an early error return never
	// blocks the goroutines.
	domainCh := make(chan string, 1)
	go func() { domainCh <- extractStoreDomainBest(ctx, in.Client) }()
	pathCh := make(chan string, 1)
	go func() { pathCh <- resolvePreviewPath(ctx, in.Client, template, file) }()

	// One request for the whole batch: ops apply and persist independently
	// server-side — no abort, no rollback.
	preIDs := map[string]bool{}
	for _, m := range allSections(inner) {
		preIDs[anyToString(m["id"])] = true
	}
	operations := make([]map[string]any, len(entries))
	for i, e := range entries {
		operations[i] = e.entry
	}
	resp, err := common.Send(ctx, in.Client, PlanBatchOps(oseid, docID, operations))
	if err != nil {
		// An invalid --session passes through verbatim — never auto-recreated
		// (contract §4.2). Any other request-level error applied nothing.
		if !created && isSessionNotFound(err) {
			return common.ExecResult{}, err
		}
		if exitErr, ok := err.(*output.ExitError); ok {
			exitErr.WithField("oseid", oseid).WithField("session_created", created)
		}
		return common.ExecResult{}, err
	}
	perOp := mapBatchResults(len(ops), entries, resp)
	applied := make([]map[string]any, 0, len(ops))
	var failedIdx []int
	for i := range ops {
		entry := map[string]any{"op": ops[i].Op, "result": perOp[i]}
		if ops[i].Target != "" {
			entry["target"] = ops[i].Target
		}
		if nt := newTargets[i]; nt != "" {
			entry["new_target"] = nt
		}
		if perOp[i] != "success" {
			failedIdx = append(failedIdx, i)
		}
		applied = append(applied, entry)
	}
	if len(failedIdx) > 0 {
		return common.ExecResult{}, batchFailErr(oseid, created, applied, failedIdx)
	}

	// Recover server-assigned ids for added sections and restore requested
	// placement (the server always appends) with one follow-up batch.
	var placementWarning string
	if hasAdds(entries) {
		placementWarning = placeSections(ctx, in.Client, oseid, docID, entries, moves, preIDs, applied)
	}

	previewURL := buildPreviewURL(<-domainCh, <-pathCh, themeID, oseid, "")

	body := map[string]any{
		"oseid": oseid, "session_created": created,
		"applied": applied, "preview_url": previewURL, "promoted": false,
	}
	if placementWarning != "" {
		body["placement_warning"] = placementWarning
	}
	if promote {
		resp, err := common.Send(ctx, in.Client, PlanPromoteSession(oseid, map[string]any{"force": false}))
		if err != nil {
			if isPromoteConflict(err) {
				return common.ExecResult{}, promoteConflictErr(oseid, applied, previewURL)
			}
			return common.ExecResult{}, err
		}
		if promoteConflicted(resp) { // registry documents a {promoted, conflict} body; tolerate both shapes
			return common.ExecResult{}, promoteConflictErr(oseid, applied, previewURL)
		}
		body["promoted"] = true
	}
	return common.ExecResult{Body: body}, nil
}

// readOpsInput loads the --ops value: '-' reads stdin, a leading '[' is
// inline JSON, anything else is a file path.
func readOpsInput(val string) ([]byte, error) {
	switch {
	case val == "":
		return nil, output.ErrValidation("--ops is required")
	case val == "-":
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, output.ErrValidation("reading --ops from stdin: %v", err)
		}
		return raw, nil
	case strings.HasPrefix(strings.TrimSpace(val), "["):
		return []byte(val), nil
	default:
		raw, err := os.ReadFile(val)
		if err != nil {
			return nil, output.ErrValidation("reading --ops file: %v", err)
		}
		return raw, nil
	}
}

// findSectionByID locates a card by stringified id across the page flow and
// the fixed cards group.
func findSectionByID(inner map[string]any, sectionID string) map[string]any {
	for _, m := range allSections(inner) {
		if anyToString(m["id"]) == sectionID {
			return m
		}
	}
	return nil
}

// splitPosition parses "after:<id>" / "before:<id>" into its kind and section id.
func splitPosition(pos string) (kind, id string, ok bool) {
	if i := strings.IndexByte(pos, ':'); i > 0 {
		return pos[:i], pos[i+1:], true
	}
	return "", "", false
}

// sectionArea returns a section's area ("" if unknown).
func sectionArea(inner map[string]any, sectionID string) string {
	if inner == nil {
		return ""
	}
	for area, list := range sectionsByArea(inner) {
		for _, m := range list {
			if anyToString(m["id"]) == sectionID {
				return area
			}
		}
	}
	return ""
}

// containerAt walks parentPath from the section root down to the container
// the append targets and returns its current children.
func containerAt(section map[string]any, parentPath []int) ([]any, error) {
	blocks, _ := section["blocks"].([]any)
	for _, p := range parentPath {
		if p < 0 || p >= len(blocks) {
			return nil, fmt.Errorf("container path index %d out of range (container has %d blocks)", p, len(blocks))
		}
		m := asMap(blocks[p])
		if m == nil {
			return nil, fmt.Errorf("container path index %d is not a block", p)
		}
		blocks, _ = m["blocks"].([]any)
	}
	return blocks, nil
}

// validateAppend enforces the schema gate for append_array_item: value.type
// must be a declared sub-block of the card's schema and the container must
// stay within max_blocks.
func validateAppend(inner, section, value map[string]any, current int) error {
	schemas := mapField(inner, "schemas")
	card := mapField(schemas, getString(section, "type"))
	if card == nil {
		return nil // no schema for this card type — let the server decide
	}
	blockType := getString(value, "type")
	if blocks, ok := card["blocks"].([]any); ok {
		found := false
		var known []string
		for _, b := range blocks {
			bm := asMap(b)
			if bm == nil {
				continue
			}
			known = append(known, getString(bm, "type"))
			if getString(bm, "type") == blockType {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("block type %q is not allowed here (schema allows: %s)", blockType, strings.Join(known, ", "))
		}
	}
	if maxBlocks, ok := numberValue(card["max_blocks"]); ok && current+1 > int(maxBlocks) {
		return fmt.Errorf("container already has %d blocks, max_blocks is %d", current, int(maxBlocks))
	}
	return nil
}

// numberValue normalizes a decoded JSON number (json.Number or float64).
func numberValue(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case interface{ Float64() (float64, error) }: // json.Number
		f, err := t.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// editDryRunPlans lists every intended request without sending any (strict
// zero-call + placeholders; op bodies keep locally-derivable values real).
func editDryRunPlans(themeID, session string, ops []editOp, promote bool) []common.PlannedRequest {
	themeRef := themeID
	var plans []common.PlannedRequest
	if themeRef == "" {
		themeRef = phThemeID
		plans = append(plans, PlanThemesList(map[string]any{"published": "1"}))
	}
	plans = append(plans, PlanDocTree(themeRef))
	oseidRef := session
	if oseidRef == "" {
		oseidRef = phOseid
		plans = append(plans, PlanCreateSession(themeRef)) // will create session
	}
	if opsNeedImplicitRead(ops) {
		plans = append(plans, PlanSchemasList(oseidRef, phDocID))
	}
	// update_pb: one pb-block-save per op (card generation), placeholders for
	// runtime-resolved values; then the whole batch as one request, plus the
	// placement follow-up when adds carry a position.
	cards := map[int]map[string]any{}
	for i := range ops {
		if ops[i].Op == "update_pb" {
			plans = append(plans, PlanPbBlockSave(map[string]any{
				"event_type": "theme", "action": "save",
				"origin_template_id": phCustomID,
				"oseid":              oseidRef, "doc_id": phDocID, "section_id": ops[i].ref.SectionID, "theme_id": themeRef,
				"ops": ops[i].Ops,
			}))
			cards[i] = map[string]any{"type": "<generated_theme_card>"}
		}
	}
	if entries, moves, _, err := translateOps(ops, nil, cards); err == nil {
		operations := make([]map[string]any, len(entries))
		for i, e := range entries {
			operations[i] = e.entry
		}
		plans = append(plans, PlanBatchOps(oseidRef, phDocID, operations))
		if len(moves) > 0 {
			moveOps := make([]map[string]any, 0, len(moves))
			for _, mv := range moves {
				moveOps = append(moveOps, map[string]any{
					"op": "move_section", "target": "<new_section_id>", "position": mv.position, "move_target": mv.moveTarget,
				})
			}
			plans = append(plans, PlanBatchOps(oseidRef, phDocID, moveOps))
		}
	}
	if promote {
		plans = append(plans, PlanPromoteSession(oseidRef, map[string]any{"force": false}))
	}
	return plans
}

// isPromoteConflict classifies a promote failure as a draft conflict: the
// endpoint answers HTTP 409 with "edit session has conflict with draft, retry
// with force=true" — not the {promoted, conflict} body the registry documents.
func isPromoteConflict(err error) bool {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "conflict with draft")
}

// promoteConflicted reads the {promoted, conflict} promote response.
func promoteConflicted(resp map[string]any) bool {
	root := resp
	if d := mapField(resp, "data"); d != nil {
		root = d
	}
	return root["conflict"] == true
}

// ─────────── error envelopes (docs/theme-page-edit-shortcuts.md §4.5 ③④) ───────────

// promoteConflictErr is the --promote conflict envelope: ops applied fine but
// the theme draft moved; forcing is a user decision, never automatic.
func promoteConflictErr(oseid string, applied []map[string]any, previewURL string) *output.ExitError {
	return output.Errorf(output.ExitAPI, output.TypeAPI, "promote conflict: the theme draft changed since this edit session was created").
		WithField("conflict", true).
		WithField("oseid", oseid).
		WithField("applied", applied).
		WithField("preview_url", previewURL).
		WithHint(fmt.Sprintf("review the preview, then promote explicitly after user confirmation: themes promote-session --params '{\"oseid\":\"%s\"}' --data '{\"force\":true}'", oseid))
}
