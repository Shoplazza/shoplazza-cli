# themes block edit — change an existing AI card's source

Rewrite the liquid source of an AI card that is already in the theme, check it by hand, and write
it back into the edit session. Saving and publishing are separate steps (SKILL.md → Workflow).
Command details: [block-cli.md](block-cli.md).

## Scope

- **Only AI cards** — the instance `type` starts with `blocks/gen_`. Theme cards and app cards
  have no source you can edit here: change what their settings allow
  ([card-edit.md](../card-edit.md)), or generate a new card ([generate-block.md](generate-block.md)).
- **A value change is not a source edit.** Another product, new text, a different color — even on
  an AI card that is [card-edit.md](../card-edit.md). Edit the source only when no setting can
  express the change (layout, structure, style, adding or removing a setting). Unsure → read the
  card's settings first ([page-read.md](../page-read.md)); no matching setting → edit the source.
- **Only this card's block file** — never a template or any other theme file.

## Reading list

| File | When |
|---|---|
| The fetched source file | Always, in full — every edit point comes from this read |
| [concepts.md](concepts.md) | Always |
| [edit-discipline.md](edit-discipline.md) | Always |
| [liquid-rules.md](liquid-rules.md) | Always |
| [mobile-rules.md](mobile-rules.md) | Always |
| [kind-and-components.md](kind-and-components.md) | Always; then read what it names for the components the change touches |
| [self-check.md](self-check.md) | Always |
| [schema-rules.md](schema-rules.md) | The change touches `{% schema %}` |
| [ljs/writing.md](ljs/writing.md) | The change touches any `ljs-` component |
| [ljs/selection.md](ljs/selection.md) | The change adds a component the source doesn't use yet |

## Flow

1. **Locate the card** → `gen_id` and `target`. Read the page ([page-read.md](../page-read.md)):
   AI-card rows carry `type: blocks/gen_<id>`, `cname` (the merchant-facing name to match the
   user's wording) and a ready-made `target`. `gen_id` is the `type` without `blocks/`.
2. **Fetch the source** into a local file, every time — the server copy is the latest; never reuse
   an old local file:

   ```bash
   themes block +get -t <theme_id> --session <oseid> --id <gen_id> --with-content \
     --jq '.data.doc.content' > <gen_id>.liquid
   ```

   In the same round you can read the placements: `themes block +get … --id <gen_id>` (no
   `--section`) → `ref_count` (see Fork), and `--section <sid>` → the instance's `template`,
   `target`, `settings`.
3. **Read** the source in full and the reading list.
4. **Change** it with exact edits, all at once ([edit-discipline.md](edit-discipline.md)). Every
   `block.settings.<id>` read matches a declared id. New copy is in the card copy language
   ([generate-block.md](generate-block.md) → Before writing); text the user dictated is verbatim.
5. **Self-check** with [self-check.md](self-check.md) and fix every must-fix hit; advisory items
   may stay when deliberate. Fix exactly what each hit names; don't try a different approach at
   random. After 3 fix rounds, stop: don't write back; tell the user which rules the change can't
   satisfy. The original card and the page stay as they were.
6. **Write back**, always with `--template` and `--target` — they carry the instance's current
   values onto the new schema (new settings take their preset value). Without them only the file
   changes and the placed card doesn't pick up the new schema.

   ```bash
   themes block +edit -t <theme_id> --session <oseid> --id <gen_id> --content <gen_id>.liquid \
     --template <template> --target <sid>.blocks[N]
   ```

   When this round also changes a setting's value, add it to the same call:
   `--ops '{"<setting_id>":<value>}'` — only the keys that change; it merges, the rest stay. Value
   formats: [setting-values.md](../setting-values.md). A round that changes only values and no
   source goes to [card-edit.md](../card-edit.md) instead of using `--ops`.
7. **Re-read:** `themes block +get -t <theme_id> --session <oseid> --id <type from the response> --section <instance.target>`
   → confirm, then report.

## Fork (card placed in 2+ places)

When `ref_count` ≥ 2, the server forks the file on update: `branched: true`, a new `type`, and
`previous_type` = the card you passed. The CLI repoints only the `--target` instance to the new
card; every other placement keeps the old card, unchanged. Tell the user the change applies to
this placement only, and use the returned `type` for everything after.

## Errors & recovery

| Situation | Do |
|---|---|
| Self-check still failing after 3 rounds | Don't write back; report which rules can't be satisfied. Nothing on the page changed. |
| `stage:"write"` | The server rejected the file (liquid parse error, invalid `{% schema %}`); nothing was written. Fix and resend. |
| `stage:"place"` with `reverted:true` | Rolled back; send the same command again. |
| `stage:"place"` with `revert_failed:true` | Don't resend; report the failure with `block_type` and `revert_id`. |
| Validation error about `--target` (not on the page, out of range, a different card) | Indexes shift after structural edits; re-read with `themes block +get … --id <gen_id> --section <sid>` and resend with that `target`. |

## Output

- Give the `preview_url` from the response verbatim (or `themes +preview -t <theme_id> --oseid <oseid>`).
- Summarize what changed and which settings were touched, named by their `label` in the user's
  language — never by setting id. Mention a fork if one happened.
- The change sits in the edit-session draft; buyers can't see it yet. Saving and publishing:
  SKILL.md → Workflow.
