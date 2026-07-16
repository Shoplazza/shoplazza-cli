package themes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
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
per-op endpoint routing (theme cards and page-builder cards can mix in one
batch), fail-fast application and a ready-to-share preview URL — one call.

Targets are copied verbatim from "themes +page" output. Standard flow passes
the oseid echoed by +page via --session so read and write share one snapshot;
omitting --session creates a fresh session (echoed back for follow-ups).

Ops (JSON array via --ops <file> | - (stdin) | inline JSON):
  update_slot        block target + props        merge props into a block's settings
  replace_props      section target + props      merge props into a section's settings
  remove_array_item  block target                remove a block (same-container batches: descending index)
  append_array_item  container target + value    append {type, settings} (validated against schema/max_blocks)
  add_section        name | pb+template_id       add a section (position: first|last|after:<sid>|before:<sid>)
  remove_section     section target              remove a section (header/footer area auto-resolved)
  move_section       section target + position   reorder a section (position, or numeric to_index)
  set_visibility     section target + visible    show/hide a section
  update_pb          PB section target + ops     apply inner PB operations (body backfilled by the CLI)

Failure semantics: fail-fast; a mid-batch failure returns an api error with
partial=true carrying oseid/applied/failed/remaining — fix the failed op and
retry ONLY the remaining ops with --session. Nothing is rolled back.

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
	resolved, err := preflightOps(ops, oseid, docID, themeID, inner)
	if err != nil {
		if exitErr, ok := err.(*output.ExitError); ok {
			exitErr.WithField("oseid", oseid).WithField("session_created", created)
		}
		return common.ExecResult{}, err
	}

	// Prefetch the store domain (GET /shop, read-only) concurrently with the
	// ops loop; it is only consumed by the preview URL after all ops apply.
	// Buffered so an early error return never blocks the goroutine.
	domainCh := make(chan string, 1)
	go func() { domainCh <- extractStoreDomainBest(ctx, in.Client) }()

	// Sequential fail-fast application.
	applied := make([]map[string]any, 0, len(resolved))
	for i, r := range resolved {
		resp, err := common.Send(ctx, in.Client, r.plan)
		if err != nil {
			// An invalid --session passes through verbatim — never wrapped as
			// partial, never auto-recreated (contract §4.2).
			if !created && isSessionNotFound(err) {
				return common.ExecResult{}, err
			}
			remaining := make([]int, 0, len(resolved)-i-1)
			for j := i + 1; j < len(resolved); j++ {
				remaining = append(remaining, j)
			}
			failed := map[string]any{"index": i, "op": ops[i].Op}
			if ops[i].Target != "" {
				failed["target"] = ops[i].Target
			}
			failed["error"] = err.Error()
			return common.ExecResult{}, partialApplyErr(oseid, created, applied, failed, remaining)
		}
		entry := map[string]any{"op": ops[i].Op}
		if ops[i].Target != "" {
			entry["target"] = ops[i].Target
		}
		if r.newTarget != "" {
			entry["new_target"] = r.newTarget
		}
		if ops[i].Op == "add_section" {
			if sid := extractSectionID(resp); sid != "" {
				entry["new_section_id"] = sid
			}
		}
		applied = append(applied, entry)
	}

	previewURL := buildPreviewURL(<-domainCh, "/", themeID, oseid, "")

	body := map[string]any{
		"oseid": oseid, "session_created": created,
		"applied": applied, "preview_url": previewURL, "promoted": false,
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

// resolvedOp is a preflighted op: the request to send plus the key artifacts
// echoed back in applied[] (docs/theme-page-edit-shortcuts.md §4.5 ①).
type resolvedOp struct {
	plan      common.PlannedRequest
	newTarget string
}

// preflightOps maps every op to its endpoint request, backfilling the body
// fields the model never provides. Checks that need page data (custom_id
// lookup, append type/max_blocks) run here, before the first write.
func preflightOps(ops []editOp, oseid, docID, themeID string, inner map[string]any) ([]resolvedOp, error) {
	out := make([]resolvedOp, 0, len(ops))
	base := func() map[string]any {
		return map[string]any{"doc_id": docID, "theme_id": themeID}
	}
	fail := func(i int, format string, args ...any) error {
		e := output.ErrValidation("op #%d (%s): %s", i, ops[i].Op, fmt.Sprintf(format, args...)).
			WithField("invalid_op", i)
		if ex := opExamples[ops[i].Op]; ex != "" {
			e = e.WithField("example", ex)
		}
		return e
	}

	for i, op := range ops {
		var r resolvedOp
		switch op.Op {
		case "update_slot":
			body := base()
			body["parent_path"] = op.ref.ParentPath
			body["block_index"] = op.ref.BlockIndex
			body["props"] = op.Props
			r.plan = PlanSetSlot(oseid, op.ref.SectionID, body)
		case "replace_props":
			body := base()
			body["props"] = op.Props
			r.plan = PlanSetProps(oseid, op.ref.SectionID, body)
		case "remove_array_item":
			body := base()
			body["parent_path"] = op.ref.ParentPath
			body["block_index"] = op.ref.BlockIndex
			r.plan = PlanRemoveBlock(oseid, op.ref.SectionID, body)
		case "append_array_item":
			section := findSectionByID(inner, op.ref.SectionID)
			if section == nil {
				return nil, fail(i, "section %q not found on this page", op.ref.SectionID)
			}
			container, err := containerAt(section, op.ref.ParentPath)
			if err != nil {
				return nil, fail(i, "%v", err)
			}
			if err := validateAppend(inner, section, op.Value, len(container)); err != nil {
				return nil, fail(i, "%v", err)
			}
			body := base()
			body["parent_path"] = op.ref.ParentPath
			body["index"] = len(container) // tail insert
			body["block"] = op.Value
			r.plan = PlanAddBlock(oseid, op.ref.SectionID, body)
			r.newTarget = fmt.Sprintf("%s[%d]", op.Target, len(container))
		case "add_section":
			body := base()
			if op.Pb {
				body["pb"] = true
				body["template_id"] = op.TemplateID
			} else {
				body["name"] = op.Name
			}
			toIndex, area := resolveSectionPlacement(inner, op)
			body["to_index"] = toIndex
			if area != "" && area != "page" { // area reverse-looked-up, not model-supplied
				body["area"] = area
			}
			r.plan = PlanAddSection(oseid, body)
		case "remove_section":
			// Contract quirk: this endpoint's body has no theme_id.
			body := map[string]any{"doc_id": docID}
			if area := sectionArea(inner, op.ref.SectionID); area != "" && area != "page" {
				body["area"] = area
			}
			r.plan = PlanRemoveSection(oseid, op.ref.SectionID, body)
		case "move_section":
			body := map[string]any{"doc_id": docID}
			toIndex, posArea := resolveSectionPlacement(inner, op)
			area := sectionArea(inner, op.ref.SectionID) // area of the section being moved
			if area == "" {
				area = posArea
			}
			// move "last" = the area's real last index (add's -1 append no-ops here)
			if op.Position == "last" && inner != nil {
				grp := area
				if grp == "" {
					grp = "page"
				}
				if n := len(sectionsByArea(inner)[grp]); n > 0 {
					toIndex = n - 1
				}
			}
			body["to_index"] = toIndex
			if area != "" && area != "page" {
				body["area"] = area
			}
			r.plan = PlanMoveSection(oseid, op.ref.SectionID, body)
		case "set_visibility":
			body := base()
			body["visible"] = *op.Visible
			r.plan = PlanSetVisibility(oseid, op.ref.SectionID, body)
		case "update_pb":
			customID := phCustomID
			if inner != nil {
				section := findSectionByID(inner, op.ref.SectionID)
				if section == nil {
					return nil, fail(i, "section %q not found on this page", op.ref.SectionID)
				}
				id, ok := pbCustomID(getString(section, "type"))
				if !ok {
					return nil, fail(i, "section %q is not a page-builder custom card (type %q)", op.ref.SectionID, getString(section, "type"))
				}
				customID = id
			}
			r.plan = PlanPbBlockSave(map[string]any{
				"event_type": "theme", "action": "save", // fixed values
				"origin_template_id": customID,
				"oseid":              oseid, "doc_id": docID, "section_id": op.ref.SectionID, "theme_id": themeID,
				"ops": op.Ops,
			})
		}
		out = append(out, r)
	}
	return out, nil
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

// sectionAreaIndex returns a section's area and its index within that area.
func sectionAreaIndex(inner map[string]any, sectionID string) (area string, idx int, found bool) {
	if inner == nil {
		return "", 0, false
	}
	ba := sectionsByArea(inner)
	for _, a := range []string{"page", "header", "footer", "global"} {
		for i, m := range ba[a] {
			if anyToString(m["id"]) == sectionID {
				return a, i, true
			}
		}
	}
	return "", 0, false
}

// sectionArea returns a section's area ("" if unknown).
func sectionArea(inner map[string]any, sectionID string) string {
	area, _, _ := sectionAreaIndex(inner, sectionID)
	return area
}

// resolveSectionPlacement turns an op's position (or numeric to_index) into the
// body to_index and, for after/before, the reference section's area.
func resolveSectionPlacement(inner map[string]any, op editOp) (toIndex any, area string) {
	if op.Position == "" {
		if op.ToIndex != nil {
			return *op.ToIndex, ""
		}
		return -1, "" // default: append at tail
	}
	switch op.Position {
	case "first":
		return 0, ""
	case "last":
		return -1, ""
	}
	kind, refID, ok := splitPosition(op.Position)
	if !ok {
		return -1, ""
	}
	a, idx, found := sectionAreaIndex(inner, refID)
	if !found {
		if inner == nil {
			return "<to_index>", "" // dry-run: resolved live
		}
		return -1, "" // ref not on page — let the backend default
	}
	if kind == "after" {
		return idx + 1, a
	}
	return idx, a // before
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
	// Placeholder-backed preflight: append indexes stay unset, custom_id is a
	// placeholder; targets resolve locally so coordinates are real.
	resolved, _ := preflightOpsDryRun(ops, oseidRef, phDocID, themeRef)
	for _, r := range resolved {
		plans = append(plans, r.plan)
	}
	if promote {
		plans = append(plans, PlanPromoteSession(oseidRef, map[string]any{"force": false}))
	}
	return plans
}

// preflightOpsDryRun mirrors preflightOps without page data: append omits the
// index (unknown until live) and update_pb keeps the custom_id placeholder.
func preflightOpsDryRun(ops []editOp, oseid, docID, themeID string) ([]resolvedOp, error) {
	out := make([]resolvedOp, 0, len(ops))
	for i := range ops {
		op := ops[i]
		if op.Op == "append_array_item" {
			body := map[string]any{
				"doc_id": docID, "theme_id": themeID,
				"parent_path": op.ref.ParentPath, "block": op.Value,
			}
			out = append(out, resolvedOp{plan: PlanAddBlock(oseid, op.ref.SectionID, body)})
			continue
		}
		r, err := preflightOps([]editOp{op}, oseid, docID, themeID, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, r...)
	}
	return out, nil
}

// extractSectionID pulls the new section id out of an add-section response.
func extractSectionID(resp map[string]any) string {
	if s := anyToString(resp["section_id"]); s != "" {
		return s
	}
	if d := mapField(resp, "data"); d != nil {
		return anyToString(d["section_id"])
	}
	return ""
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

// partialApplyErr is the mid-batch fail-fast envelope: type "api" with a
// partial=true discriminator, carrying everything an agent needs to recover.
func partialApplyErr(oseid string, created bool, applied []map[string]any, failed map[string]any, remaining []int) *output.ExitError {
	return output.Errorf(output.ExitAPI, output.TypeAPI, "op #%v (%v) failed: %v", failed["index"], failed["op"], failed["error"]).
		WithField("partial", true).
		WithField("oseid", oseid).
		WithField("session_created", created).
		WithField("applied", applied).
		WithField("failed", failed).
		WithField("remaining", remaining).
		WithHint(fmt.Sprintf("fix op #%v, then retry ONLY the remaining ops with --session %s (do NOT resend applied ops)", failed["index"], oseid))
}

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
