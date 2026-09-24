# themes card-edit — change cards and blocks, then save or publish

`themes +edit` sends a batch of ops into the edit session in one call. Ops apply independently:
one failure doesn't stop the others and nothing rolls back. Read first
([page-read.md](page-read.md)) — every `target`, field id and value range comes from that read.
Order of steps: SKILL.md → Workflow.

## Command

```bash
themes +edit -t <theme_id> --template index --session <oseid> --ops '[
  {"op":"replace_props","target":"<section_id>","props":{"<field_id>":3}},
  {"op":"update_slot","target":"<section_id>.blocks[0].blocks[0]","props":{"text":"<p>New Arrivals</p>"}},
  {"op":"set_visibility","target":"<other_section_id>","visible":false}
]'
```

- `--ops`: inline JSON array, a file path, or `-` (stdin). Long or quote-heavy text → a file.
- Always pass the task's `--session` and the same `-t`. Without `--session` a new, empty session
  is created (`session_created:true`) and the earlier drafts are not in it.
- Success: `{oseid, session_created, applied:[{op, target, result:"success", new_target? | new_section_id?}], preview_url, promoted, placement_warning?, published?}`.

## Card types

| Row in the read | Kind | How to edit |
|---|---|---|
| `type` is a bare name (`hero_slideshow`) | Theme card | Every op below |
| Block row `type: blocks/gen_<id>` inside a `type: "_blocks"` section | AI card | The card is that block row: its fields → `update_slot` on its `target` (e.g. `<sid>.blocks[0]`); its own blocks are `<target>.blocks[N]`. Layout, structure or new settings → [block/edit-block.md](block/edit-block.md) |
| `type` is `shoplazza://apps/<app>/…`, not `page-builder` | App card | Same ops as a theme card; keys only from current values and schema |
| `type` starts with `shoplazza://apps/page-builder/`, row `kind: "pb"` | Page-builder card | Content → `update_pb`, see [page-builder.md](page-builder.md). Remove / move / hide as for any card |
| `section_id` `header`, `footer`, or a global-area type (`announcement`…) | Global card | The ops here, with that `section_id` as the target. `session update-config` can't reach these cards |

Remove / move / hide a whole card works on every kind by `section_id`.

## Choosing the op

Pick by the shape of `target`, not by the field name:

| `target` | Op | The other op fails with |
|---|---|---|
| Bare `section_id` | `replace_props` | `update_slot` → `target must be a block path (copy it from the +page blocks list)` |
| Contains `.blocks[…]` | `update_slot` | `replace_props` → `target must be a section id` |

A field such as a navigation `menu` often sits on a block row (the header's or footer's menu
block), not on the card — check the schema's block list for where it lives.

## Ops

| Op | Purpose | Required | Target |
|---|---|---|---|
| `replace_props` | Card-level fields (merge; only the keys to change; one op per card) | `target`, `props` | `section_id` |
| `update_slot` | A block's fields (merge) | `target`, `props` | Block row `target`, copied |
| `append_array_item` | Add a block at the end of a container; model `value` on an existing block of that type | `target`, `value{type, settings}` | Container = block `target` + `.blocks` (card level `<sid>.blocks`). `type` must be a key of the card's schema `blocks`; `limit` / `max_blocks` are checked |
| `remove_array_item` | Remove a block | `target` | Block row `target` |
| `move_array_item` | Reorder a block within its container | `target`, `to_index` (0-based) | Block row `target` |
| `add_section` | Add a card | `name` [+ `value`, `position`] | See [card-add.md](card-add.md) |
| `remove_section` | Delete a whole card — name it in the summary | `target` | `section_id` |
| `move_section` | Reorder a card | `target`, `position` | `first` · `last` · `before:<sid>` · `after:<sid>`. Don't use `to_index` (lands one short when moving down) |
| `set_visibility` | Show / hide a card | `target`, `visible` (bool) | `section_id` (blocks have no visibility switch) |
| `update_pb` | Page-builder card content | `target`, `ops` | See [page-builder.md](page-builder.md) |

"Move up one" = `before:<sid of the card above>`. Swapping two cards is one op (move A before B).

## Before writing

1. **Every item of the ask maps to a schema field.** A named store object → its resource field
   ([resource-binding.md](resource-binding.md)); retyping a heading is not binding. A
   presentation ("full width", "sticky", "collapsible") → its switch or option. An item with no
   field is not written — SKILL.md → Rules for every operation (5).
2. **Level.** "The X of block Y in card Z" → the block row; only the card named → card level.
   The same `label` at both levels → write where it takes effect (a block field gated by a
   `visibleOn` switch that is on beats the card field). Unsure → write, re-read, trust the
   effective value.
3. **Label not found** → check theme settings once ([global-config.md](global-config.md));
   not there either → unsupported.
4. **Values** not given verbatim (options, numbers) or composite (color scheme, font, product /
   collection, link, image, spacing) → [setting-values.md](setting-values.md) first.
5. **Promotion facts** written on a card (discount size, threshold, end date) must match a real
   activity in the store — look it up ([resource-binding.md](resource-binding.md)). Figures the
   user states are checked too; no match → say so and ask.
6. **"Add / remove X in this card"** → map X to a block type via the `label`s in
   `schema.<type>.blocks`; respect `limit`, `static` (fixed block, can't be added or removed)
   and the current count — at the limit, say so. X is a new page-level card →
   [card-add.md](card-add.md).

## One named card, one named field

- Every `target` starts with the `section_id` of the card the user named. Don't touch other
  cards of the same type.
- `props` holds only the fields named this turn. Paired fields stay as they are: the other
  device (PC / mobile), width vs height, a switch vs what it controls, the rest of a color
  group — a new image is not a resize. If a pair should change together, explain why and ask.
- A whole class of elements store-wide → a theme setting, not card by card (SKILL.md → Rules
  for every operation, 3).
- Delete with a delete op; it's done only when the re-read no longer shows the row. Never remove
  the `header` / `footer` card itself — hide it or remove blocks inside.

**When the value is incomplete:**

| Situation | Handling |
|---|---|
| Outside `min`/`max`, off `step`, not in `options` | State the legal range; don't write it, don't clamp it |
| Direction only ("bigger", "darker") | Pick a value from the current one in that direction, within range; write it; summary says old → new |
| Field named, no direction ("adjust it") | Leave it out (the rest of the batch still goes); offer 2–3 candidates by `label` and ask |
| Requested icon / shape not in `options` | Don't substitute the closest; say what exists ([setting-values.md](setting-values.md)) |

An item left pending is not done — say so in the summary.

## Batch rules

1. One task's changes go in one `--ops` array.
2. Several blocks removed from one container → descending index, same batch.
3. After a structural change (add / remove / move) in a container, its old targets are stale →
   re-read with `--session` before editing there again. Before re-ordering in a later turn,
   re-read the current order: a move that already happened returns `success` and changes nothing.
4. New card with content: field names known → put them in the `add_section` `value`; otherwise
   a second `+edit` on `applied[].new_section_id` ([card-add.md](card-add.md)).

## Verify

Re-read after every write: fields → `+page --session <oseid> --section <sid> --include schema`;
added / removed / moved / hidden cards → the whole-page read with `--session`. Report what the
re-read shows (field `label`s, old → new), not what was sent. `success` but the re-read differs
→ report it as not applied. A read without `--session` shows none of the drafts.

## Save and publish

- `--promote` saves the **whole session** — every earlier change in it, including theme
  settings, app switches and AI cards written with the same `oseid`. Say so in the summary.
- Save runs directly when asked. Asked together with the change → add `--promote` to that
  write. Asked after the preview → `+edit … --session <oseid> --ops '[]' --promote`. Then ask
  about publishing. The session stays usable after a save.
- Publish needs consent (SKILL.md → Rules for every operation, 8): write the ops first, then
  dry-run `+edit … --session <oseid> --ops '[]' --promote --publish`, restate, wait. If the
  ops ride in the pending publish call, nothing is written until the user agrees — don't call
  them drafted.
- `promoted:true` → saved (buyers still see nothing); `published:true` → live.

## Errors & recovery

| Signal | Meaning | Do |
|---|---|---|
| Exit 2, `type:"validation"`, `invalid_op:<n>`, e.g. `op #0 (replace_props): target must be a section id, got "<sid>.blocks[0]"` | Op / target shape wrong; nothing sent | Keep the target, use the block-level op with the same meaning (field → `update_slot`, drop the block → `remove_array_item`). No equivalent → report that item as not done |
| `op #0 (<op>): unknown op` | Op name not in the table; nothing sent | Pick by purpose from Ops. Never add a second card to hold an item meant for this card |
| `invalid position "<p>" (use first \| last \| after:<section_id> \| before:<section_id>)` | Bad `position` | Fix it |
| `block type "<t>" is not allowed in "<card>" (schema allows: <types>)` | Block type not in the schema | Use an allowed type; none fits → say so |
| `--ops is empty and nothing else was requested` · `--ops is empty, so --session is required` | Empty batch without `--promote`, or without `--session` | Pass ops, or `--session <oseid> --promote` |
| Exit 1, `N of M ops failed`, `failed:[i…]`, per-op `results[].result` | Those ops rejected; the rest are saved in the session | Fix and resend **only** the failed ops, same `--session` |
| ↳ `invalid_field:<key>` | No such field at this level | Re-read the schema, find the level that has the `label`, change target / op |
| ↳ `invalid_value:<key>` | Out of range, off `step`, or not in `options` | User's value → state the range; your own pick → choose a legal one |
| ↳ `target_not_found` | Stale block index or unknown section | Re-read, copy the target again |
| `placement_warning` on success | Card added but not placed | `move_section` with `before:` / `after:` |
| `promote conflict: the theme draft changed since this edit session was created`, `conflict:true` | Ops applied (previewable); the theme draft changed since the session opened | Ask. Forcing overwrites those draft changes. With consent (dry-run → restate → wait): `themes session promote --params '{"oseid":"<oseid>"}' --data '{"force":true}'`. The error's hint names `themes promote-session`, which doesn't exist |
| `the edit was promoted to the theme draft but publishing failed: …`, `promoted:true, published:false` | Saved, not live | Don't redo ops; resend `+edit … --session <oseid> --ops '[]' --promote --publish` once (same consent) |
| `b_invalid_themeid` (404) | `-t` isn't the theme the session was opened on | Pass that theme's id |
| `b_invalid_request` (403, hint says log in again) or `SESSION_NOT_FOUND` | Session unknown or expired — not an auth problem | Tell the user its drafts are gone; with their OK, open a new session (`+page` without `--session`) and redo the changes |

| 5xx / timeout | Unknown whether it applied | Re-read first; resend only what didn't land, once |

