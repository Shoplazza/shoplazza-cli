# themes card-add — add a new card to a page

Adding a card the page doesn't have yet, and answering "which cards can I add / what fits X".
Two steps: discover candidates with `themes section cards`, then write an `add_section` op with
`themes +edit` into the page's session. Adding an item inside an existing card is a block op
([card-edit.md](card-edit.md)), not this.

## Commands

```bash
# 1. Cards that can be added to a page (theme cards + app extension cards)
themes section cards --params '{"theme_id":"<theme_id>","source":["theme","extension"],"template":"index","limit":100}' \
  --jq '{total: .data.total, items: [.data.items[] | {id, name, source, description, category}]}'

# 2. Check that candidates really have a named feature (up to 10 ids)
themes +card-schema -t <theme_id> --ids <id1>,<id2>,<id3>

# 3. Add, place and fill in one op (oseid from +page, see page-read.md)
themes +edit -t <theme_id> --template index --session <oseid> --ops '[{"op":"add_section","name":"<card_id>","position":"after:<section_id>","value":{"settings":{"<field_id>":"<value>"},"blocks":[{"type":"<block_type>","settings":{"<field_id>":"<value>"}}]}}]'
```

- `section cards` → `.data.items[]` `{id, name, source}`; `description` / `category` may be
  absent (null in the projection). `name` is a string or a multilingual object — match either
  language. `limit` is at most 100; `total` above it → next `page`.
- `+card-schema` → `.data.items[]` `{id, name, source, settings[], blocks{}, max_blocks}` in the
  same projection as `+page --include schema`; ids that resolve nowhere land in `.data.missing[]`.
  Extension cards return empty `settings`.
- `+edit` → `applied[]` with `new_section_id` for each added card.

## Flow

1. Resolve the theme (SKILL.md → Rules for every operation, 1) and the template
   ([page-read.md](page-read.md) → Choosing the page). Open or reuse the session with `+page`.
2. Discover with the same `theme_id` and the `template`. AI cards: add `"gen"` to `source` only
   if the backend accepts it — `InvalidParameter` `invalid source: gen` → drop it. AI cards of
   the session are also listed by `themes session card-list --params '{"oseid":"<oseid>"}'`
   (`gen_list`, when the backend returns it).
3. Pick the card (below).
4. Build `add_section` by source, with `position` and — when field names are known — content.
5. Re-read `+page --session <oseid> --section <new_section_id> --include schema`; align block
   counts and fill what's left in a second `+edit`.
6. Summarize and give `preview_url`; stop (SKILL.md → Workflow).

## Picking the card

| Situation | Do |
|---|---|
| Card named, one `name` match | Use its `id` and `source` |
| Several matches, or unclear | List them (name + source) and ask; stop |
| A need described ("a promo bar", "show buyer photos") | Pick 1–3 candidates from the returned items by `name` (and `description` / `category` when present). Only cards this call returned |
| The need names a feature (countdown, quick add, filters…) | `+card-schema` the candidates; keep only those whose `settings` / `blocks` really have it (match `label`). Empty `settings` (extension) or an id in `missing` → judge by `name` |
| "Which cards can I add?" with no add intended | List names grouped by source, plain text; stop |
| Empty list | Say no cards can be added to this page |
| Nothing fits | Don't force a card or invent a type. A theme setting may cover it (cart / quick-add behavior often does — [global-config.md](global-config.md)); otherwise offer an AI card ([block/generate-block.md](block/generate-block.md)) |

Read the source from `items[].source`, not from how the id looks.

## `add_section` by source

| `source` | `items[].id` looks like | `add_section` | Call it |
|---|---|---|---|
| `theme` | `collection_list` | `"name":"<id>"` (bare) | theme card |
| `extension` | `shoplazza://apps/<app>/blocks/<name>/<id>` | `"name":"<id>"` (full URI) | app card |
| `gen` | `blocks/gen_<id>` | `"name":"_blocks","value":{"blocks":[{"type":"blocks/gen_<id>"}]}` — keep the `blocks/` prefix | AI card |

An AI card is a block held by a `_blocks` container section:

- One `add_section` per AI card; several cards → several ops, never several types in one
  `value.blocks`. Only AI cards go into `_blocks`.
- Don't fill `value.blocks[].settings`: `+card-schema` can't read an AI card's fields, so fill
  them in the second call.

Not addable through this flow: `public` cards — say so. `pb` / `custom` cards are page-builder
cards — see [page-builder.md](page-builder.md).

## Placement

Put `position` in the same `add_section` op: `first` · `last` · `before:<sid>` · `after:<sid>`.
The anchor is an existing card of the area the new card should join, from the whole-page read.
Without `position` the card goes to the end of the page area. `placement_warning` in the
response → the card was added but not placed: `move_section` with `before:` / `after:` on
`applied[].new_section_id`.

## Content

1. **Decide the content.** Marketing copy the user didn't give (title, subtitle, button text)
   may be drafted, in the storefront language (SKILL.md → Rules for every operation, 7). Images,
   video and links come only from the user (a URL) or the store's media library
   (`shop files list`); missing → leave the field empty and name it in the summary.
2. **Field names known from `+card-schema`** → fill in the same op: card fields in
   `value.settings`; blocks in `value.blocks` as `{type, settings}`, where `type` is a key of the
   card's schema `blocks`. Value formats: [setting-values.md](setting-values.md).
3. **Otherwise** (schema not read, extension card, AI card) → add the card only, then one more
   `+edit` in the same `--session`, all content ops in one array. Copy targets from the re-read:
   - Theme / app card: target = `new_section_id`; card fields → `replace_props`; block content →
     `update_slot` on existing block rows, `append_array_item` for new ones.
   - AI card: `new_section_id` is the container; the card is its first block row
     `<new_section_id>.blocks[0]` → `update_slot` there, not `replace_props` on the container.
4. **Align block counts.** The card's preset decides how many blocks it starts with. When the
   user named a count or listed items, re-read, then remove extras (`remove_array_item`,
   descending index) or append the missing ones in the same card. An AI card's own blocks are
   at `<new_section_id>.blocks[0].blocks[N]`. One set of items → one card; never add a second
   card for the overflow.

An op that doesn't fit its target is rejected — [card-edit.md](card-edit.md) → Errors & recovery.

## Summary

"Added card <name> to <page>", then the drafted copy, every carrying field still empty (image,
video, link, body text) and every requested effect the card has no field for. An empty shell is
not "done". Give `preview_url` and stop; saving and publishing:
[card-edit.md](card-edit.md) → Save and publish.
