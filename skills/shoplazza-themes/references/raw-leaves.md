# themes raw leaves — low-level session, card and block calls

The shortcuts (`+page`, `+edit`, `+card-schema`, `+preview`, `block +edit`, `block +get`) wrap
these leaves and add what the leaves lack: id resolution, schema checks, readable failures,
placement. **Use a raw leaf only for what no shortcut does** — see "When a raw leaf is the right
tool". All take `--params` (path + query) and `--data` (body); exact fields:
`schema themes.<group>.<leaf>`.

## `doc_id` — the template file id

Most leaves below need `doc_id` = the template file's id from `file tree` — **not** `"index"`:

```bash
themes file tree --params '{"theme_id":"<id>"}' --jq '.data.templates[] | select(.location=="index.liquid") | .id'
```

- A custom template (`<type>.<suffix>`, from `+page --list`) is not in the tree: take `doc_id`
  from `themes template list --params '{"theme_id":"<id>"}'` → `.data.theme_templates[]` whose
  `type` + `.` + `suffix` matches.
- `"index"` fails on these leaves (404 `b_record_not_found`, 422 `request_param_error`, or 500).
  Only `session get-config` / `update-config` ignore `doc_id` (`"index"` works there).
- `+page` resolves this itself and does not print it.

## When a raw leaf is the right tool

| Need | Leaf |
|---|---|
| Has the draft changed since this session opened (someone else saved)? | `session check` |
| Which app embeds / script tags are on in this session | `section list` → `.data.data.sections.app_embeds` / `.script_tags` ([apps.md](apps.md)) |
| Which data sources a card field can bind to | `section get` → `data_source_settings` |
| The stored page JSON of the session (section order, hidden flags) | `session get-file` |
| The theme's card library, grouped | `session card-list` |
| Save a stale session over the other change | `session promote` with `force:true` — consent first |

No read-only leaf returns rendered HTML (`section get` returned none in testing, although its
spec says it renders); `set-props` / `block add|remove|set-props` return the card's `html` after a
write. To show a result, give the preview link.

## session

| Leaf | Params / body | Returns · notes |
|---|---|---|
| `session create` | `{"theme_id"}` | `.data.oseid`. `+page` without `--session` does the same and reads the page |
| `session check` | `{"oseid"}` | `data:{}` = no conflict; `data.conflict:true` = the draft changed since the session opened |
| `session get-file` | `{"oseid","doc_id"}` | `.data.template.{id, location, type, content}`; `content` is a JSON string `{sections:{<id>:{name, cname, display, blocks, settings,…}}, content_for_page:[ids], layout}`. Template files only (a config file id → 500) |
| `session update-file` | `{"oseid","doc_id"}` + `{config:[…], global_config?, layout?}` | **Overwrites the whole template**: the `config` list replaces every section and any card missing from it is removed. Never use it for a partial change — use `+edit` |
| `session batch-ops` | `{"oseid","doc_id"}` + `{"operations":[…]}` | `.data.data[]` = `{op, result}`. A failed op still exits 0 (`result:"invalid_field:<key>"`) — check every `result`. Prefer `+edit` |
| `session card-list` | `{"oseid","need_schema"?:1}` | `card_list[]` (+ `block_list`, `gen_list` where supported) of `{group_key, group_name, groups[{type, name, max_blocks, limit, templates}]}` |
| `session promote` | `{"oseid"}` + `{"force":false}` | `.data.promoted:true`. A stale session → 409. For normal saves use `+edit --session <oseid> --ops '[]' --promote` |
| `session get-config` / `update-config` | | Theme settings → [global-config.md](global-config.md) |

`batch-ops` speaks the server's op grammar, not `+edit`'s (block paths use dots,
`<sid>.blocks.0`; positions are `position` + `move_target`). To get a correct body,
`+edit … --dry-run` prints the exact `operations` it would send.

## section

| Leaf | Params / body | Returns · notes |
|---|---|---|
| `section list` | `{"oseid","doc_id","template_name"?}` | `.data.data.{schemas:{<type>:…}, sections:{page_sections[], global_sections[], app_embeds:{<app_key>:[{type, disabled, settings}]}, script_tags:{<app_uid>:[{id, disabled, settings}]}, plugins}}` |
| `section get` | `{"oseid","doc_id","section_id","template_name"?}` | `.data.data.{section:{settings, blocks[{type, settings}],…}, schemas, data_source_settings:{settings:{<key>:{available, data_sources}}, blocks}}`. Works for global cards (`header`) |
| `section set-props` | `{"oseid","section_id"}` + `{"doc_id","theme_id","props":{…}}` | `.data.{html, section}`. **No validation**: out-of-range values and unknown keys are saved. `+edit replace_props` checks range, step, options and field names — prefer it |
| `section remove` | `{"oseid","section_id"}` + `{"doc_id","area"?}` | `content_for_page`. Page cards need the numeric instance id; `area` `header`/`footer` on section-group themes. Prefer `+edit remove_section` |
| `section cards` | | Addable cards → [card-add.md](card-add.md) |

Global card ids (`announcement`, `header`) as `section_id` fail on `set-props` with
`section <id> not found in edit session` on tested themes; `+edit` with `target:"announcement"`
works.

## block

| Leaf | Notes |
|---|---|
| `block add` / `remove` / `set-props` | Params `{"oseid","section_id"}`, body `{"doc_id","theme_id",…}`. Blocks are addressed by `block_index` + `parent_path` (index path from the section root to the parent), not `+page` targets. `add` takes `block:{type, settings}` and `index` — **omitted = 0 = inserted at the front**; `-1` appends. Returns `{html, section}`. These don't run `+edit`'s schema and `max_blocks` checks — prefer `update_slot` / `append_array_item` / `remove_array_item` |
| `block list` | Public cards for a template or theme (`template`, `theme`, `limit`) → `apps[{id, blocks[]}]` |
| `block create-gen` / `get-gen` / `update-gen` / `delete-gen` / `revert-gen` | AI-card files → [block/block-cli.md](block/block-cli.md); prefer `block +edit` / `block +get`. `update-gen` needs the card's full `settings` and may answer `branched:true` (a new card was made; the target instance must be switched to the returned `settings`). `delete-gen` needs consent |

## Rules

1. Same `theme_id` and `oseid` as the rest of the task; raw writes land in the session like `+edit`.
2. Before a raw write that skips validation (`set-props`, `block add|set-props`), check every value
   against `+page --section <sid> --include schema` (`options`, `min`/`max`/`step`, field ids).
3. After any raw write, re-read with `+page -t <id> --template <t> --session <oseid> --section <sid>`
   and report the re-read.
4. `session promote` with `force:true` overwrites someone else's saved changes:
   `--dry-run` → restate → wait for consent. Run `session check` first to show there is a conflict.

## Errors & quirks

| Symptom | Meaning → fix |
|---|---|
| 404 `b_record_not_found` / 422 `request_param_error` / 500 with `doc_id:"index"` | Use the template file id (see `doc_id`) |
| `batch-ops` exit 0 but a `result` isn't `success` | That op failed; the others are saved (no rollback) |
| `session promote` → 409 `edit session has conflict with draft, retry with force=true to overwrite` | The draft changed since the session opened → tell the user; force only with consent |
| `block set-props` → 400 `block_index 9 out of range (parent has 2 blocks)` | Re-read the card's blocks and use a valid index |
