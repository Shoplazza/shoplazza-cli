# themes page builder — page-builder cards and custom card templates

A page-builder ("advanced") card holds a free-form **canvas** — nested layouts, columns and
elements (title, button, image, video, …) — instead of a fixed settings list. On a page it is a
section row with `kind:"pb"` and a `type` like
`shoplazza://apps/page-builder/blocks/<global|custom>-<n>/<hash>`: `global-` cards come from
public templates, `custom-` cards from the merchant's own custom card templates. Change its
content through canvas operations (`update_pb`), not through the row's `settings`.

## Read

| Need | Command |
|---|---|
| Canvas of every pb card on a page | `themes +page -t <id> --template <t> --session <oseid> --include pb` → each `kind:"pb"` row gains `canvas` (text snapshot) or `canvas_error` |
| Canvas of one placed card | `… --section <sid>` — a pb card expands its canvas without `--include pb` |
| Addable pb cards | `themes section cards --params '{"theme_id":"<id>","source":"pb,custom","limit":100}'` → `.data.items[]` = `{id, name, category, second_category, source}` |
| Canvas of a template not on a page | `themes pb summary --data '{"id":"<n>","type":"global"}'` → `.data.text` |
| Name, full `type` and default fields of templates | `themes pb list-blocks --params '{"event_type":"page-builder","source_ids":"global-<n>,custom-<n>"}'` → `.data.blocks.<prefixed id>.{name, type, settings[], source}` |

`pb summary` options: `detail_level` (`minimal` default · `tiered` · `verbose`) and
`detailed_node_ids` (paths to expand, e.g. `["0.1.2"]`).

**Canvas text**: one node per line, indentation = nesting, `#<path> <component> "<name>"` plus
some content, containers shown as `[column]`:

```
#0 layout_001 "Layout_001"
  #0.0 [column] "Inner_container_001"
    #0.0.0 title_001 "Title_001" content="<p>Example title</p>"
    #0.0.1 button_001 "Button_001"
```

The path without `#` (`0.0.1`) is the `targetId` of a canvas operation.

### Id formats

| Where | Form |
|---|---|
| `section cards` `items[].id`, `pb list-blocks` `source_ids`, `+edit` `add_section` `template_id` | Prefixed: `global-<n>` / `custom-<n>` |
| `pb summary` `id` | Bare `<n>`, with `type` = `global` or `custom` (the prefix word) |
| Section `type` on a page | `…/page-builder/blocks/<prefix>-<n>/<hash>` |
| `pb save-template` / `delete-template` `template_id` | Not confirmed — take the id exactly as the call that listed it returned it; on "not found", don't retry with a rebuilt id |

## Edit a placed card — `+edit` op `update_pb`

```json
{"op":"update_pb","target":"<pb_section_id>","ops":[{"action":"update","targetId":"0.0.1","settings":{"<key>":"<value>"}}]}
```

- `target`: the pb card's `section_id` (a block path is rejected). `ops`: the canvas operations,
  at least one. Confirmed form: `action:"update"` + `targetId` (canvas path) + `settings`.
  Only this form is documented here; use a node's existing `settings` keys (from `pb summary`) and
  don't guess other actions.
- The CLI saves the edited canvas as a new card and swaps it into the same position. The card
  gets a **new section id**, returned as `applied[i].new_section_id` — use it for the re-read and
  for later ops. Field values that still exist on the new card carry over.
- One `update_pb` per pb card; it can share an `--ops` array with theme-card ops.
- If generating the new card fails (not a pb card, canvas not loadable, server error), the whole
  command fails and **nothing** in the batch is applied.
- Re-read: `+page --session <oseid> --section <new_section_id>`; paths can shift after structural
  changes, so take `targetId`s from the latest canvas.

Add a pb card to a page: `+edit` op `{"op":"add_section","pb":true,"template_id":"global-<n>","position":"after:<sid>"}`
(no `value` — content comes from the template); see [card-add.md](card-add.md).

## Raw leaves

| Leaf | What it does |
|---|---|
| `pb update --data '{"schema":{…},"ops":[…]}'` | Applies ops to full canvas data you already hold and returns `{schema, text, html, translation, failures}`. **Saves nothing.** None of the reads above return canvas data, so for placed cards use `update_pb` |
| `pb save-block` | Applies `ops` (≥1; an empty list is rejected with 400) to a template, saves the result as a new card and, with `event_type:"theme"`, places it at `section_id` in the session (response adds `type` + `block`); other `event_type`s only save. Required: `event_type`, `origin_template_id`, `action` (`save`/`save_as`/`update`), `oseid`, `doc_id`, `section_id`, `theme_id`; `origin` `custom`/`global` (empty = custom first). `update_pb` fills all of this — prefer it. `doc_id` → [raw-leaves.md](raw-leaves.md) |
| `pb save-template --data '{"action":"save_as","template_id":"<id>"}'` | Saves a custom card template from an existing one → `.data.id` (always a **new** id). `save_as` = new listed copy · `update` = new listed version, and the source template is **taken down** · `save` = saved but not listed. `title` / `category` / `second_category` / `image` / `origin` default to the source's |
| `pb delete-template --params '{"template_id":"<id>"}'` | Deletes a custom card template. **Needs consent**: `--dry-run` → restate → wait |

Before `save-template` with `action:"update"`, tell the user the original template leaves the list.

## Errors & quirks

| Symptom | Meaning → fix |
|---|---|
| `pb summary` → 400 `拉取模版失败（status=400）: invalid template_id` | A prefixed id was sent → strip `global-` / `custom-`, set `type` |
| `pb list-blocks` → `{"blocks":{}}` | A bare id was sent → use `global-<n>` / `custom-<n>` |
| A pb row shows `canvas_error` instead of `canvas` | Its source template can't be loaded; don't guess paths — say the card's canvas can't be read |
| `update_pb: section "<sid>" is not a page-builder card` | Target isn't `kind:"pb"` → use `replace_props` / `update_slot` ([card-edit.md](card-edit.md)) |
| `template_id is required when pb=true` hint names `themes list-card` | That command doesn't exist → `themes section cards` |
| `section cards` → 400 `invalid source: gen` | Drop `gen` from `source` |

## Output

Describe canvas changes by node (`Title_001` text → new text), give the new `section_id` when
`update_pb` replaced the card, and the `preview_url`.
