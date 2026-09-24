# themes block commands — write, read, delete and revert AI cards

All four work inside one edit session: `block +edit` writes a card file and places it on a page,
`block +get` reads a card and where it is placed, `block delete-gen` deletes a card, and
`block revert-gen` undoes one of those writes. Authoritative flags: `themes block <cmd> --help`
and `schema themes.block.<cmd>`.

## Values to gather

Take every value from what is already known — this turn first, then earlier in the conversation,
including earlier command output, carried across turns until the user changes it. Run a command
only when a value is missing; never invent one.

| Value | Where it comes from |
|---|---|
| `theme_id` | SKILL.md → Rules for every operation (1). Pass `-t <theme_id>` on `block +edit` / `block +get`: omitted means the published theme, so it's required whenever the session is on another theme. |
| `template` | The page the card is on; page names in [page-read.md](../page-read.md). A merchant-created page (e.g. a custom collection page) has a suffixed template name — find it with `themes +page -t <theme_id> --list` by `title`. Falling back to the default name writes to every page of that type, and the custom page won't show the card. Don't place or edit cards on `order` / `order_verify` (order detail, order lookup): those pages can't be previewed, so the user can't check the change — say it isn't supported and send nothing. |
| `oseid` | The task's session; else `themes +page -t <theme_id> --template <template>` → `.data.oseid`. |
| `gen_id` | The card's `type` without `blocks/`, always `gen_<id>`. `type` is on `+page` block rows and in `block +get` / `block +edit` output. |
| `target` | The instance's position, like `<sid>.blocks[0]`, copied verbatim from `themes +page` or `block +get` output — never assembled by hand. `<sid>` is a section id, never a `gen_id`. |

## block +edit — create or update a card

`--id` decides what the call does:

| `--id` | Does | `--target` | Without `--template` |
|---|---|---|---|
| omitted | Creates a new card; the server names the file | Container path `<sid>.blocks`. Omitted → the CLI adds a new `_blocks` container at the end of the page and places the card in it | File only, not placed; `instance` is `null` (place it later with `add_section`, [card-add.md](../card-add.md)) |
| given | Rewrites that card's source | Instance path `<sid>.blocks[N]`, required with `--template` | File only; placed instances keep their values and don't pick up the new schema |

### Flags

| Flag | Required | Notes |
|---|---|---|
| `--session` | yes | The edit session (`oseid`). |
| `--content` | yes | The liquid source: a file path, or `-` for stdin. Inline source is not accepted. It must contain `{% schema %}` — the server enforces that (error at `stage:"write"`), not the CLI. |
| `-t, --theme-id` | no | Theme id; defaults to the published theme. |
| `--id` | no | The `gen_<id>` to update (a `blocks/` prefix is accepted). Omit to create. |
| `--template` | no | The page to place on (`index`, `product`, …). Omit to write the file only. |
| `--target` | no | Container path when creating, instance path when updating. Needs `--template`. |
| `--settings` | no | Update only: the instance's current values (JSON object or file), used instead of the values read from `--target`. An object with a `type` key is sent as-is, and that `type` must be `blocks/<gen_id>` of `--id`. Rarely needed. |
| `--ops` | no | Setting values to change on the placed instance — only the keys that change (JSON object or file), merged server-side. Needs `--template` and `--target`. Value formats: [setting-values.md](../setting-values.md). |
| `--section-name` | no | Display name of the `_blocks` container in the editor's structure tree (stored as its `cname`). Needs `--template`. A bilingual object `{"zh-CN":"…","en-US":"…"}`, or a bare name that fills both locales. Without `--target` it names the new container (otherwise merchants see `_blocks`); with `--target` it renames that existing container — don't pass it then unless asked. |

**Checked locally, before any request** (a failure sends nothing): `--target` needs `--template`;
`--ops` needs `--template` and `--target`; `--section-name` needs `--template`; `--id` with
`--template` needs `--target`; `--settings` only with `--id`; only one of `--content` /
`--settings` / `--ops` may be `-`; `--id` must be a `gen_*` id; `--target` must be a container
path when creating and an instance path when updating; `--settings` / `--ops` must be JSON
objects. The CLI does not parse the liquid or the schema — run [self-check.md](self-check.md)
first.

With `--template`, the CLI also reads the page before writing: a `--target` section that isn't on
the page, an index past the end of the container, or an instance of a different card fails with a
validation error, and nothing is written.

### Examples

```bash
# Create, in a new container at the end of the page
themes block +edit -t <theme_id> --session <oseid> --content ./card.liquid --template index \
  --section-name '{"zh-CN":"品牌故事","en-US":"Brand story"}'

# Create inside an existing section
themes block +edit -t <theme_id> --session <oseid> --content ./card.liquid --template index \
  --target <sid>.blocks

# Write the file only; place it later
themes block +edit -t <theme_id> --session <oseid> --content ./card.liquid

# Update the source; the instance's values follow onto the new schema
themes block +edit -t <theme_id> --session <oseid> --id gen_<id> --content ./gen_<id>.liquid \
  --template index --target <sid>.blocks[0]

# Update and change two setting values in the same call
themes block +edit -t <theme_id> --session <oseid> --id gen_<id> --content ./gen_<id>.liquid \
  --template index --target <sid>.blocks[0] --ops '{"title":"<text>","title_color":"#FF6600"}'
```

### Output

```json
{
  "type": "blocks/gen_<id>",
  "doc": { "id": "gen_<id>", "location": "gen_<id>.liquid" },
  "settings": { "title": "<text>", "title_color": "#222222" },
  "ops": null,
  "branched": false,
  "revert_id": "<revert_id>",
  "oseid": "<oseid>",
  "instance": { "template": "index", "target": "<sid>.blocks[0]", "section_created": true, "section_name": "…" },
  "applied": [
    { "op": "add_section", "result": "success" },
    { "op": "append_array_item", "target": "<sid>.blocks", "result": "success" }
  ],
  "preview_url": "https://…"
}
```

| Field | Meaning |
|---|---|
| `type` | The card type. After an update it may differ from `--id` (Fork) — use this one for everything after. |
| `doc.id` | The `gen_id` for later `--id`. |
| `settings` | The instance values the placement used: the preset values when creating; when updating, the current values carried onto the new schema (keys the new schema no longer declares are dropped, new ones take their preset value). |
| `ops` | The `--ops` object as sent, or `null`. |
| `branched` / `previous_type` | A fork happened (Fork below). |
| `revert_id` | Undo token for this write (`block revert-gen`). |
| `instance` | `template`; `target` — the instance position, index chosen by the server; use it verbatim as a later update's `--target`; `section_created` — the CLI added a container; `section_name` — present when `--section-name` landed. `null` without `--template`. |
| `settings_defaulted` | `true` when an update had no current values to carry (no `--template`, no `--settings`). |
| `applied` | One row per page op: `op`, `result`, `target`. |
| `degraded` | Extras that didn't land (`ops`, `section_name`); the card is placed anyway. |
| `preview_url` | Preview link; quote it verbatim. For a card on a `customers/*` page, the preview opens a login page first: log in as a customer and refresh. |

### Fork

Updating a card that is placed in 2+ places forks it: the server writes a new file
(`branched: true`, a new `type`, `previous_type` = the card you passed), the CLI repoints only the
`--target` instance to it, and every other placement keeps the old card unchanged. Without
`--template` / `--target` nothing is repointed — so always pass both when updating.

### Errors & recovery

| Error | Meaning | Do |
|---|---|---|
| `validation` | A local check or the page read failed; nothing was written. | Fix the flags or re-read the `target`, then resend. |
| `stage:"write"` | The server rejected the file (liquid parse error, invalid or missing `{% schema %}`); nothing was written. | Fix the file, resend. |
| `stage:"place"` + `reverted:true` | A required page op failed; the CLI rolled back its page changes and the file write, so the session is where it was. | Send the same command again. |
| `stage:"place"` + `revert_failed:true`, hint gives a `themes block revert-gen` command | The page was put back, but the file stayed in the session (`block_type`, `revert_id`). | Don't resend yet. Run the hint's `revert-gen` command as given (it removes only the leftover file; the page no longer points at it), then retry only after fixing what made the placement fail (`results` / `failed`). |
| `stage:"place"` + `revert_failed:true`, no `revert-gen` in the hint | The page could not be put back either; it may still point at the new file. | Don't resend and don't revert the file (that would leave the page pointing at nothing). Report the failure with `block_type`, `revert_id` and `revert_error`. |
| `stage:"place"`, neither flag | The placement request itself failed; the file was written (`block_type`, `revert_id`), and whether it got placed is unknown. | `block +get --id <block_type>`: placed → carry on; not placed → undo with `block revert-gen` and resend, or place it with `add_section` ([card-add.md](../card-add.md)). Never resend a create without checking. |

## block +get — read a card and its placements

| `--section` | Returns | Use it |
|---|---|---|
| omitted | `ref_count` and `instances[]` (`template` + `target` per placement) | Impact before an edit or a delete |
| given | `instance` (`template`, `target`, `settings`) for that section; an array when the section holds this card twice | The values `+edit` takes: `--template`, `--target`, `--settings` |

- `--section` also accepts a full target; only its section id is used. If that section isn't in the
  card's placement record, add `--template`.
- `instance.settings` holds only the card's own values; its inline sub-block rows aren't there. To
  see how many sub-blocks it has and their values, use
  `themes +page -t <theme_id> --template <template> --session <oseid> --section <sid>`.
- `+page` block rows are the whole tree flattened: the card at `<sid>.blocks[N]`, its sub-blocks
  right after it as `<sid>.blocks[N].blocks[0]`, ….
- `--with-content` adds `doc.content` (the liquid source; can be large) and `name` (the display name
  read from the schema: `presets[0].cname`, else the schema `name`). Use `name` when mentioning the
  card to the user, not `type`.

Flags: `--session` (required), `--id` (required; `blocks/` prefix accepted), `--section`,
`--template`, `--with-content`, `-t, --theme-id`.

```bash
# Where is the card placed — before an edit or delete
themes block +get -t <theme_id> --session <oseid> --id gen_<id>

# The instance values +edit takes
themes block +get -t <theme_id> --session <oseid> --id gen_<id> --section <sid>

# Save the source to a local file
themes block +get -t <theme_id> --session <oseid> --id gen_<id> --with-content --jq '.data.doc.content' > gen_<id>.liquid
```

Output without `--section`:

```json
{
  "type": "blocks/gen_<id>",
  "doc": { "id": "gen_<id>", "location": "gen_<id>.liquid" },
  "saved": true,
  "ref_count": 2,
  "instances": [
    { "template": "index", "target": "<sid>.blocks[0]" },
    { "template": "product", "target": "<sid2>.blocks[1]" }
  ]
}
```

With `--section`, `instances` is replaced by
`"instance": { "template": "index", "target": "<sid>.blocks[0]", "settings": { … } }`.

Errors: "section … holds no recorded instance" → omit `--section` to list placements, or add
`--template`; "section … has no block of type …" → indexes shifted after a structural edit, list
placements again without `--section`.

## block delete-gen — delete a card

Deletes the card file and removes every instance of it from every page of the theme. It ignores
`ref_count` and deletes however many places use the card. Needs consent first
([delete-block.md](delete-block.md)). Parameters go in `--params`, not the `block +edit` / `block +get` flags:

| Param | Required | Notes |
|---|---|---|
| `oseid` | yes | The edit session. |
| `type` | yes | The card type, with the `blocks/` prefix. |

```bash
themes block delete-gen --params '{"oseid":"<oseid>","type":"blocks/gen_<id>"}' --dry-run
```

Returns `revert_id`.

## block revert-gen — undo a write

Restores the card to its state before the write identified by `revert_id` (from any create,
update, delete or revert response). It returns a new `revert_id`, so undo and redo can repeat.

```bash
themes block revert-gen --params '{"oseid":"<oseid>"}' --data '{"revert_id":"<revert_id>"}'
```

Use it when the user asks to undo, or to clean up after a place-stage failure. Undoing a
`delete-gen` brings back the file together with its placements and settings.
