package themes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// themes +edit — one-shot batch write for agent-driven theme editing.
//
// Standard flow: pass the oseid echoed by `themes +page` so read and write
// share one edit-draft snapshot; omitting --session creates a fresh one.

var editShortcut = common.Shortcut{
	Service:   "themes",
	Command:   "+edit",
	StoreTier: true,
	Use:       "+edit",
	Short:     "Apply a batch of edit ops to a template page inside one edit session",
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
  move_array_item    block target + to_index     reorder a block within its container (0-based, range-checked)
  add_section        name [+ value] | pb+template_id
                                                 add a section (value: {settings, blocks} pre-fills it) or a pb card
                                                 (position: first|last|after:<sid>|before:<sid>)
  remove_section     section target              remove a section
  move_section       section target + position   reorder a section (position, or numeric to_index)
  set_visibility     section target + visible    show/hide a section
  update_pb          PB section target + ops     regenerate the PB card via pb and swap it in place

Failure semantics: ops apply and persist independently server-side — a
failure does not stop or roll back the others. A partial failure returns an
api error carrying per-op results; fix the failed ops and resend ONLY them
with --session.

A promote conflict returns an api error with conflict=true and never forces;
it also stops --publish before anything goes live.

"Already previewed, now ship it" is an empty batch: --ops '[]' with --session
and --promote [--publish] skips the batch request entirely.`,
	Example: `  # Change one block setting, reusing the session +page echoed
  shoplazza themes +edit --template index --session <oseid> --ops '[{"op":"update_slot","target":"<section_id>.blocks[0]","props":{"heading":"Summer sale"}}]'

  # Save a previewed session onto the theme draft
  shoplazza themes +edit --template index --session <oseid> --ops '[]' --promote`,
	Flags: []common.Flag{
		{Name: "template", Type: common.FlagString, Description: "Template name, e.g. index / product. Mutually exclusive with --file."},
		{Name: "file", Type: common.FlagString, Description: "Theme file path, e.g. templates/index.liquid. Mutually exclusive with --template."},
		{Name: "theme-id", Short: "t", Aliases: []string{"theme"}, Type: common.FlagString, Description: "Theme ID. Defaults to the published theme."},
		{Name: "session", Type: common.FlagString, Description: "Edit session id (oseid) — pass the one echoed by `themes +page`. Omit to create a fresh session."},
		{Name: "ops", Type: common.FlagString, Required: true, Description: "Edit operations: a file path, '-' for stdin, or an inline JSON array."},
		{Name: "promote", Type: common.FlagBool, Description: "Promote the edit draft onto the theme draft after all ops apply (needs explicit user intent)."},
		{Name: "publish", Type: common.FlagBool, Description: "Publish the theme live after a clean promote. Requires --promote; only when the user explicitly asked to go live."},
	},
	// Only --promote/--publish leave the edit session, so only they confirm.
	DestructiveIf: func(f common.FlagSet) string {
		target := "live theme"
		if id := f.GetString("theme-id"); id != "" {
			target = "theme " + id
		}
		switch {
		case f.GetBool("publish"):
			return "Publish " + target + "?"
		case f.GetBool("promote"):
			return "Save onto " + target + "'s draft? It ships with the next publish."
		}
		return ""
	},
	Execute: editExecute,
}

// editInput is themes +edit's parsed, network-free-validated input.
type editInput struct {
	themeID  string
	template string
	file     string
	session  string
	promote  bool
	publish  bool
	ops      []editOp
}

// parseEditInput reads the flags and runs every check that needs no request, so
// nothing past it can fail before the first side effect.
func parseEditInput(in common.ExecInput) (editInput, error) {
	e := editInput{
		themeID:  in.Flags.GetString("theme-id"),
		template: in.Flags.GetString("template"),
		file:     in.Flags.GetString("file"),
		session:  in.Flags.GetString("session"),
		promote:  in.Flags.GetBool("promote"),
		publish:  in.Flags.GetBool("publish"),
	}
	if _, _, err := templateLocation(e.template, e.file); err != nil {
		return editInput{}, err
	}
	raw, err := readOpsInput(in.Flags.GetString("ops"))
	if err != nil {
		return editInput{}, err
	}
	if e.ops, err = parseOps(raw); err != nil {
		return editInput{}, err
	}
	if err := validateOps(e.ops); err != nil {
		return editInput{}, err
	}
	// Publishing rides on --promote so the agent-side high-risk gate, which
	// keys off --promote, always covers it.
	if e.publish && !e.promote {
		return editInput{}, output.ErrValidation("--publish requires --promote").
			WithHint("publishing goes live from the theme draft, so the edit has to be promoted first")
	}
	// An empty batch is the "already previewed, now ship it" path: nothing to
	// apply, so it only makes sense against an existing session being promoted.
	if len(e.ops) == 0 {
		switch {
		case !e.promote:
			return editInput{}, output.ErrValidation("--ops is empty and nothing else was requested").
				WithHint("pass ops to apply, or add --promote [--publish] to save the session as it stands")
		case e.session == "":
			return editInput{}, output.ErrValidation("--ops is empty, so --session is required").
				WithHint("an empty batch promotes an existing session; a fresh session would have nothing in it")
		}
	}
	return e, nil
}

// openEditSession reuses the caller's oseid, or creates a fresh edit draft and
// reports that it did so.
func openEditSession(ctx context.Context, c *client.Client, themeID, session string) (string, bool, error) {
	if session != "" {
		return session, false, nil
	}
	resp, err := common.Send(ctx, c, PlanCreateSession(themeID))
	if err != nil {
		return "", false, err
	}
	oseid := extractOseid(resp)
	if oseid == "" {
		return "", false, output.ErrInternal("create-session returned no oseid")
	}
	return oseid, true, nil
}

// preflightCards runs the pb pre-flights the batch needs: update_pb regenerates
// its card via pb-block-save, add_section pb resolves the template via
// pb-single-blocks. Keyed by op index.
func preflightCards(ctx context.Context, c *client.Client, ops []editOp, inner map[string]any,
	oseid, docID, themeID string) (map[int]map[string]any, error) {
	cards := map[int]map[string]any{}
	for i := range ops {
		var card map[string]any
		var err error
		switch {
		case ops[i].Op == "update_pb":
			card, err = generateThemeCard(ctx, c, ops[i], inner, oseid, docID, themeID)
		case ops[i].Op == "add_section" && ops[i].Pb:
			card, err = resolvePbSectionValue(ctx, c, ops[i].TemplateID)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		cards[i] = card
	}
	return cards, nil
}

// foldBatchResults folds the server's per-entry results back onto the source
// ops, returning the applied list and the indexes that did not succeed.
func foldBatchResults(ops []editOp, entries []serverOp, newTargets map[int]string,
	resp map[string]any) ([]map[string]any, []int) {
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
	return applied, failedIdx
}

// promoteAndPublish saves the edit draft onto the theme draft and, when asked,
// takes it live. It records what happened on body.
func promoteAndPublish(ctx context.Context, c *client.Client, e editInput, oseid, themeID string,
	applied []map[string]any, previewURL string, body map[string]any) error {
	if e.promote {
		resp, err := common.Send(ctx, c, PlanPromoteSession(oseid, map[string]any{"force": false}))
		if err != nil {
			if isPromoteConflict(err) {
				return promoteConflictErr(oseid, applied, previewURL)
			}
			return err
		}
		if promoteConflicted(resp) { // registry documents a {promoted, conflict} body; tolerate both shapes
			return promoteConflictErr(oseid, applied, previewURL)
		}
		body["promoted"] = true
	}
	// Publish strictly after a clean promote: a conflict returns above, so it
	// can never go live on top of someone else's draft.
	if e.publish {
		resp, err := common.Send(ctx, c, PlanPublish(themeID))
		if err != nil {
			return publishFailedErr(oseid, themeID, applied, previewURL, err)
		}
		body["published"] = true
		if id := revokePublishID(resp); id != "" {
			body["revoke_publish_id"] = id // the theme this one replaced
		}
	}
	return nil
}

func editExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	e, err := parseEditInput(in)
	if err != nil {
		return common.ExecResult{}, err
	}
	if in.DryRun {
		return common.ExecResult{Plans: editDryRunPlans(e.themeID, e.session, e.ops, e.promote, e.publish)}, nil
	}

	themeID, docID, err := resolveThemeAndDoc(ctx, in.Client, e.themeID, e.template, e.file)
	if err != nil {
		return common.ExecResult{}, err
	}
	oseid, created, err := openEditSession(ctx, in.Client, themeID, e.session)
	if err != nil {
		return common.ExecResult{}, err
	}
	// Past this point the session is the retry handle, so every error carries it.
	fail := func(err error) (common.ExecResult, error) {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			exitErr.WithField("oseid", oseid).WithField("session_created", created)
		}
		return common.ExecResult{}, err
	}

	// Implicit read only when the batch needs page data (custom_id lookup,
	// append validation, or section placement/area).
	var inner map[string]any
	if opsNeedImplicitRead(e.ops) {
		if inner, err = fetchSections(ctx, in.Client, oseid, docID); err != nil {
			return common.ExecResult{}, err
		}
	}
	cards, err := preflightCards(ctx, in.Client, e.ops, inner, oseid, docID, themeID)
	if err != nil {
		return fail(err)
	}
	entries, moves, newTargets, err := translateOps(e.ops, inner, cards)
	if err != nil {
		return fail(err)
	}

	// Prefetch the preview-URL inputs concurrently with the batch.
	previewURLFor := previewURLLater(ctx, in.Client, themeID, e.template, e.file)

	// One request for the whole batch: ops apply and persist independently
	// server-side — no abort, no rollback.
	preIDs := sectionIDSet(inner)
	operations := make([]map[string]any, len(entries))
	for i, en := range entries {
		operations[i] = en.entry
	}
	// An empty batch (the promote/publish-only path) sends no request at all.
	var resp map[string]any
	if len(operations) > 0 {
		resp, err = common.Send(ctx, in.Client, PlanBatchOps(oseid, docID, operations))
	}
	if err != nil {
		// An invalid --session passes through verbatim — never auto-recreated.
		// Any other request-level error applied nothing.
		if !created && isSessionNotFound(err) {
			return common.ExecResult{}, err
		}
		return fail(err)
	}
	applied, failedIdx := foldBatchResults(e.ops, entries, newTargets, resp)
	if len(failedIdx) > 0 {
		return common.ExecResult{}, batchFailErr(oseid, created, applied, failedIdx)
	}

	// Recover server-assigned ids for added sections and restore requested
	// placement (the server always appends) with one follow-up batch.
	var placementWarning string
	if hasAdds(entries) {
		placementWarning = placeSections(ctx, in.Client, oseid, docID, entries, moves, preIDs, applied)
	}

	previewURL := previewURLFor(themeID, oseid)

	body := map[string]any{
		"oseid": oseid, "session_created": created,
		"applied": applied, "preview_url": previewURL, "promoted": false,
	}
	if placementWarning != "" {
		body["placement_warning"] = placementWarning
	}
	if perr := promoteAndPublish(ctx, in.Client, e, oseid, themeID, applied, previewURL, body); perr != nil {
		return common.ExecResult{}, perr
	}
	return common.ExecResult{Body: body}, nil
}

// publishFailedErr covers the promote-succeeded-publish-failed window: the work
// is already on the theme draft, so the caller must retry publish alone rather
// than redo the ops.
func publishFailedErr(oseid, themeID string, applied []map[string]any, previewURL string, cause error) *output.ExitError {
	return output.Errorf(output.ExitAPI, output.TypeAPI, "the edit was promoted to the theme draft but publishing failed: %v", cause).
		WithField("oseid", oseid).
		WithField("applied", applied).
		WithField("promoted", true).
		WithField("published", false).
		WithField("preview_url", previewURL).
		WithHint(fmt.Sprintf(`the changes are saved on the theme draft; retry publish alone: themes publish --params '{"theme_id":"%s"}'`, themeID))
}

// revokePublishID pulls the replaced theme's id out of a publish response,
// tolerating the data wrapper.
func revokePublishID(resp map[string]any) string {
	root := resp
	if d := mapField(root, "data"); d != nil {
		root = d
	}
	if th := mapField(root, "theme"); th != nil {
		root = th
	}
	return getString(root, "revoke_publish_id")
}

// readOpsInput loads the --ops value: '-' reads stdin, a leading '[' is
// inline JSON, anything else is a file path.
func readOpsInput(val string) ([]byte, error) {
	if val == "" {
		return nil, output.ErrValidation("--ops is required")
	}
	return readFlagInput("--ops", val, '[')
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

// containerAt walks parentPath from the section root down to the container the
// append targets, returning the container itself (the section when parentPath
// is empty) and its current children. The container — not the section — owns
// the schema that governs the append.
func containerAt(section map[string]any, parentPath []int) (map[string]any, []any, error) {
	container := section
	blocks, _ := section["blocks"].([]any)
	for _, p := range parentPath {
		if p < 0 || p >= len(blocks) {
			return nil, nil, fmt.Errorf("container path index %d out of range (container has %d blocks)", p, len(blocks))
		}
		m := asMap(blocks[p])
		if m == nil {
			return nil, nil, fmt.Errorf("container path index %d is not a block", p)
		}
		container = m
		blocks, _ = m["blocks"].([]any)
	}
	return container, blocks, nil
}

// appBlockSentinel is the schema entry standing for "app blocks allowed here".
const appBlockSentinel = "@app"

// isAppBlockType reports whether a block type is a URI-addressed app block.
// Schemas declare these with the "@app" sentinel rather than by type.
func isAppBlockType(blockType string) bool {
	return strings.HasPrefix(blockType, "shoplazza://apps/")
}

// validateAppend enforces the schema gate for append_array_item against the
// container's own schema: value.type must be a declared sub-block and the
// container must stay within max_blocks.
//
// App blocks are exempt from the type check. A schema declares them with the
// "@app" sentinel, which never equals the block's own URI type, and cards that
// accept them do not always declare it (product is one) — so the server, not
// the CLI, decides whether an app block fits.
func validateAppend(inner, container, value map[string]any, current int) error {
	containerType := getString(container, "type")
	card := mapField(mapField(inner, "schemas"), containerType)
	if card == nil {
		return nil // no schema for this container — let the server decide
	}
	blockType := getString(value, "type")
	if blocks, ok := card["blocks"].([]any); ok && !isAppBlockType(blockType) {
		found := false
		var known []string
		for _, b := range blocks {
			bm := asMap(b)
			if bm == nil {
				continue
			}
			t := getString(bm, "type")
			if t == appBlockSentinel {
				continue // not a usable value for --ops
			}
			known = append(known, t)
			if t == blockType {
				found = true
			}
		}
		switch {
		case len(known) == 0:
			return fmt.Errorf("%q takes no sub-blocks", containerType)
		case !found:
			return fmt.Errorf("block type %q is not allowed in %q (schema allows: %s)",
				blockType, containerType, strings.Join(known, ", "))
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
func editDryRunPlans(themeID, session string, ops []editOp, promote, publish bool) []common.PlannedRequest {
	themeRef, plans := dryRunThemeRef(themeID)
	plans = append(plans, PlanDocTree(themeRef))
	oseidRef := session
	if oseidRef == "" {
		oseidRef = phOseid
		plans = append(plans, PlanCreateSession(themeRef)) // will create session
	}
	if opsNeedImplicitRead(ops) {
		plans = append(plans, PlanSchemasList(oseidRef, phDocID))
	}
	// pb pre-flight plans, then the whole batch as one request, plus the
	// placement follow-up when adds carry a position.
	cards := map[int]map[string]any{}
	for i := range ops {
		switch {
		case ops[i].Op == "update_pb":
			plans = append(plans, PlanPbBlockSave(map[string]any{
				"event_type": "theme", "action": "save",
				"origin_template_id": phCustomID,
				"origin":             "custom",
				"oseid":              oseidRef, "doc_id": phDocID, "section_id": ops[i].ref.SectionID, "theme_id": themeRef,
				"ops": ops[i].Ops,
			}))
			cards[i] = map[string]any{"type": "<generated_theme_card>"}
		case ops[i].Op == "add_section" && ops[i].Pb:
			plans = append(plans, PlanPbSingleBlocks(ops[i].TemplateID))
			cards[i] = map[string]any{"type": "<pb_template_type_uri>"}
		}
	}
	if entries, moves, _, err := translateOps(ops, nil, cards); err == nil {
		operations := make([]map[string]any, len(entries))
		for i, e := range entries {
			operations[i] = e.entry
		}
		if len(entries) > 0 { // an empty batch sends no request at all
			plans = append(plans, PlanBatchOps(oseidRef, phDocID, operations))
		}
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
	if publish {
		plans = append(plans, PlanPublish(themeRef))
	}
	return plans
}

// isPromoteConflict classifies a promote failure as a draft conflict: the
// endpoint answers HTTP 409, not the documented {promoted, conflict} body.
func isPromoteConflict(err error) bool {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "conflict with draft")
}

// promoteConflicted reads the {promoted, conflict} promote response.
func promoteConflicted(resp map[string]any) bool {
	root := unwrapData(resp)
	return root["conflict"] == true
}

// ─────────── error envelopes ───────────

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
