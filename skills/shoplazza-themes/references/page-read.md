# themes page-read — find the card, its targets, schema and current values

`themes +page` is the only read command for page editing. It also opens the edit session:
without `--session` it opens a NEW session every time and echoes `oseid` +
`session_created:true`; with `--session` it re-reads that session (drafts included). A task
reads in two steps: a whole-page projection to list the cards, then a single-card read of each
candidate for its current `settings`, field `schema` and block rows with ready-made `target`s.
Theme choice: SKILL.md → Rules for every operation (pass `-t <theme_id>` on every call).

## Commands

```bash
# 1. Whole page: card list + AI-card rows. Keep .data.oseid for the rest of the task.
themes +page -t <theme_id> --template index \
  --jq '{oseid: .data.oseid, areas: .data.areas, sections: [.data.sections[] | {section_id, type, area, name, visible} + ([.blocks[]? | select(.type | startswith("blocks/gen_")) | {type, cname, target}] | if length > 0 then {ai_cards: .} else {} end)]}'

# 2. One card: current settings, field schema, block rows with targets (also the re-read after a write)
themes +page -t <theme_id> --template index --session <oseid> --section <section_id> --include schema

# Global-area cards as a group, with schema
themes +page -t <theme_id> --template index --session <oseid> --area global --include schema

# Templates of the theme (standard + custom); cannot be combined with --template/--section/--include/--area
themes +page -t <theme_id> --list
```

| Flag | Notes |
|---|---|
| `--template <name>` | Page template (see "Choosing the page") |
| `--session <oseid>` | Omit only on the first read of a task; every later read passes it, or earlier drafts are invisible |
| `--area` | `all` (default: every row tagged with `area`, plus `areas` counts) · `page` · `header` · `footer` · `global`. A narrowed read carries no `area` tag on rows |
| `--section <sid>` | One card; `sid` must come from a row this task's whole-page read returned |
| `--include schema` | Field projection with zh-CN `label`s (`label`, `type`, `default`, `min`/`max`/`step`/`unit`, `options`, `visibleOn`, `info`) plus addable block types. Use with `--section` or `--area`, not on the whole-page list |
| `--include pb` | Page-builder canvas — see [page-builder.md](page-builder.md) |

## Choosing the page

| Request | `--template` | Effect |
|---|---|---|
| Names a page type ("商品页 / 商品详情页 / product page", "专辑页 / collection page") | That page's template | Writes the default page of that type — every page of that type changes |
| The product / collection word is part of a card name ("商品列表", "专辑滚动" are whole card names) | Not a page choice — use the other rows | — |
| Header / footer / global-area cards | `index` | Shared store-wide; editing them in any page's session has the same effect. Don't ask which page |
| No page named, change intended | `index`, or follow clues (product detail → `product`, cart → `cart`) | Target not found → switch page and re-read. A pure question with no page → ask which page |
| Page name ambiguous | `--list` first, let the user choose | Pick from the real list, never guess a name |
| A custom template was created or chosen in this task | Its name `<type>.<suffix>` | Only objects bound to it change; falling back to the default name (`product`) writes every default product page |

**Template names:** `index` home · `product` product detail · `collection` collection ·
`cart` · `search` · `blog` blog list · `article` blog post · `page` default custom page · `404`.
Custom templates are `<type>.<suffix>` (`product.<suffix>`, `collection.<suffix>`,
`page.<suffix>`); take the name from `--list` (or `themes template list`), never build it. In
`--list`, `type` is `system` or the custom type, and `title` is the merchant-facing name (often
Chinese) — match the user's wording against `title` to find a custom template.

- `customers/*` templates are customer-account pages (login, register, addresses, orders…). The
  preview opens the login page; changes show only after signing in as a customer — say so in
  the summary.
- `order` and `order_verify` are not supported: changes can't be checked in a preview. Say so
  and send no writes.
- Once a custom template is in play, every read and write of the task uses its name.

## Narrowing to the card

1. **`type` picks candidates; the single-card read decides.** A card is the target only when its
   read really contains the field or block the user named. Read candidates one by one; none has
   it → wrong page, switch and re-read. Never edit a similar-named card instead.
2. **Same `type` several times** (e.g. two `rich_text` cards) → read each in render order;
   several really have the field → list them and ask.
3. **App cards** have a `type` like `shoplazza://apps/<app>/blocks/<name>/<id>`; the row's
   `name` (multilingual object, e.g. `{"en-US":"Product grid","zh-CN":"商品列表"}`) is the
   display name to match. Their settings keys come only from the current values and schema.
4. **AI cards** are block rows with `type: blocks/gen_<id>` — the type carries no meaning. Match
   the user's wording against `cname` (the merchant-facing card name, a multilingual object;
   match either language). Copy the row's `target` for every later read and write.
5. **`_blocks` sections** are the containers that hold AI cards. Call them "AI card" to the
   user; never say `_blocks`. How to edit them: [card-edit.md](card-edit.md) → Card types.
6. **Global-area cards differ per theme** — never assume a list. Read `--area global`; their
   `section_id` equals their `type` string (e.g. `announcement`), copy it as is. Colloquial names
   overlap ("弹窗 / popup" may fit several) → read the group with `--include schema` and pick the
   card whose schema really has the named field. They can be edited, not added.
7. **Header and footer** rows have `section_id` `header` / `footer`.

### Which cards contain a text? (match by current values)

When the ask covers every field on the page holding some text ("replace 'Shop Now' everywhere"),
`type` can't tell you which cards match — match the current values in one read:

```bash
themes +page -t <theme_id> --template index --session <oseid> \
  --jq '[.data.sections[] | ({target: .section_id, op: "replace_props", settings}, (.blocks[]? | {target, op: "update_slot", settings})) | select(.settings | tostring | contains("<old text>")) | {op, target, keys: [.settings | to_entries[] | select(.value | tostring | contains("<old text>")) | .key]}]'
```

Each hit gives the `op`, `target` and field keys to write; build the ops directly from it (no
per-card read needed). Write the current value with only the old text swapped (keep wrappers
such as `<p>…</p>`). Hits on `header`, `footer` or global-area rows change every page — say so.
`contains` is case-sensitive.

## Reading the result

- `sections[]` is in render order. Row: `section_id`, `type`, `area`, `visible`, `settings`
  (current values), `name`, `kind: "pb"` on page-builder cards, and `blocks[]` — the nested
  block tree flattened depth-first, parent before children, each row with `type`, `settings`,
  `target` and (when known) `cname`.
- **`target` is the only coordinate.** Copy it verbatim into ops; never count the tree or build
  a path. The one allowed concatenation: a container for a new block = that block's `target` +
  `.blocks` (card level: `<section_id>.blocks`).
- `schema.<type>.settings` maps intent → field: match the user's words against `label`,
  `options[].label` and `info`. `schema.<type>.blocks` lists the block types the card accepts
  (key = block `type`, with `label`, `limit`, `static`) — the only way to map a merchant's block
  name to a block type. `max_blocks` caps the card's block count.
- `areas` gives per-area counts; target missing from the page group → check the other areas.
- Ready to change something → go to [card-edit.md](card-edit.md) with `oseid`, `target`s, the
  schema and the current values.

## Pure read questions

"What's on the homepage / how many cards" with no change intended → the whole-page read alone is
enough. Answer grouped by `area`: page (numbered in render order), header, footer. **Total =
page + header + footer** (header and footer do render on that page). Global-area cards are not
counted; if there are any, mention them in one separate line. Plain text, no URLs. No page named
→ ask which page first (not needed for header, footer or global cards).
