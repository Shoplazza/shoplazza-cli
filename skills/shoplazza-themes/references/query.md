# themes query — installed themes, versions, preview links

Read-only lookups on the themes installed in the store: which exist, how many, which one is
live, one theme's status and version, and a storefront preview link. Nothing here opens an edit
session or writes anything.

## Commands

| Intent | Command |
|---|---|
| All installed themes | `themes list --params '{"page_size":250}'` → `.data.themes[]`, `.data.count`, `.data.has_more` / `.data.cursor` |
| How many themes | `themes list --params '{"page_size":1}' --jq '.data.count'` |
| Which theme is live | `themes list --params '{"published":"1"}'` → one row |
| Unpublished themes only | `themes list --params '{"published":"0","page_size":250}'` |
| Specific themes by id | `themes list --params '{"ids":["<id>","<id>"]}'` |
| One theme's detail / status / version | `themes get --params '{"theme_id":"<id>"}'` → `.data.theme` |
| Storefront preview link | `themes +preview -t <id> [--oseid <oseid>] [--path <path>] [--locale <locale>]` → `.data.preview_url` |
| A background task's status | `themes task --params '{"task_id":"<uuid>"}'` → `.data.task` |

Paging: `page_size` 1–250, default 10; `has_more:true` → pass the returned `cursor` (or `page`)
for the next page. `count` is the total across all pages. `published` and `page_size` can be
combined: with `"1"` at most one row comes back and paging is ignored; with `"0"` it pages
through the unpublished themes.

## Reading the fields

| Field | Meaning |
|---|---|
| `published` | `"1"` = the live theme (exactly one per store), `"0"` otherwise — a string |
| `default` | `"1"` on the store's default theme; absent or `"0"` elsewhere |
| `name` | The theme's own display name (what `themes rename` changes) |
| `merchant_theme_name` | The market series it was installed from (Reformia, Hero, Nova 2023, …); `preset` = its style preset |
| `c_version` → `newest_c_version` | Current → newest available version; `has_newest_version:true` = an upgrade exists ([lifecycle.md](lifecycle.md)) |
| `updated_at`, `publish_time` | Unix seconds, as strings |

Themes uploaded from local theme development have no `merchant_theme_name`, `preset`,
`newest_c_version` or `has_newest_version`: no market series, and they can't be upgraded.

Cover images (optional): the top-level `pc_cover_url` / `mobile_cover_url` are often null. The
preset's own covers sit in `merchant_theme_info` (a JSON string), and may be bare paths without a
host:

```bash
themes list --params '{"page_size":250}' --jq '[.data.themes[] | . as $t | {name, cover: ((try ($t.merchant_theme_info | fromjson | .merchant_theme.presets[$t.preset].pc_cover_url) catch null) // $t.pc_cover_url)}]'
```

## Finding a theme by name

The order of precedence is SKILL.md → Rules for every operation (rule 1). To match a name:

1. Fetch every row: `page_size:250`, and follow `cursor` while `has_more` is true — the default
   page of 10 misses themes.
2. Match `name` case-insensitively, exact first, then contains. No hit → match
   `merchant_theme_name` case-insensitively, exact only (a series name is not a substring match).
3. One hit → use its `id`. Several (e.g. two themes named "Nova") or none → list the candidates
   (name, series, live or not, `updated_at`) and ask.

```bash
themes list --params '{"page_size":250}' --jq '[.data.themes[] | {id, name, merchant_theme_name, published, c_version}]'
```

## Preview links

`themes +preview` builds the URL locally: no API call, and no check of `--path` or `--oseid` — a
wrong handle or session id just gives a broken link.

- `-t` is required here (no published-theme default, unlike other shortcuts). Resolve it first;
  for the live theme use `themes list --params '{"published":"1"}'`.
- `--oseid <oseid>`: pass the active edit session when the conversation has unsaved edits on
  that theme, so the preview shows them. Without it the preview shows no session's edits. Never
  invent an oseid.
- `--locale` (e.g. `en_US`, `zh_CN`) previews that storefront language.
- `--path` (default `/`):

| Page | `--path` | Where the handle comes from |
|---|---|---|
| Home | `/` | — |
| Cart / search / not-found | `/cart` · `/search` · `/404` | — |
| Product | `/products/<handle>` | `products +search --keyword <title>` → `.data.products[].handle` (shoplazza-products) |
| Collection | `/collections/<handle>` | `products collections list` → `handle` (shoplazza-products) |
| Custom page | `/pages/<handle>` | `shop pages list` → `url` (shoplazza-shop); the `url` is already the path |
| Blog | `/blogs/<handle>` | `shop blogs list` → `handle` (shoplazza-shop) |
| Article | `/blogs/<blog_handle>/<article_handle>` | `shop blogs list` + `shop articles list` → `handle` (shoplazza-shop) |

A handle always comes from a real record. A page that needs one with no record found, or a page
not in this table → preview the home page and say why.

## Background tasks

`themes task` reads a background theme task (theme upload or upgrade, per its description) →
`.data.task.{type, status, message, info, created_at, updated_at}`. Call it only with a task id
a previous response returned: `task_id` must be a UUID (otherwise 400 `value must be a valid
UUID`), and an unknown UUID returns 500.

## Errors & recovery

| Symptom | Cause | Do |
|---|---|---|
| `themes get` → 404 `Record not found` | Wrong or deleted id | Re-run `themes list` and re-resolve |
| `+preview` → `required flag(s) not set: --theme-id` | `-t` omitted | Resolve the theme (published theme by default) and pass `-t` |
| A named theme isn't in the rows | Only the first page was read | `page_size:250` and follow `cursor` |
| `.data.themes` empty | The store has no themes | Say so |

## Output

- List: lead with the count; one line per theme — name, series when it differs, a live marker,
  version, and "upgrade available (→ newest)" when `has_newest_version` is true.
- Detail: name, series and preset, live or not, `c_version` → `newest_c_version`.
- Preview: the `preview_url`, and whether it includes unsaved edits (`--oseid` passed) or not.
- Relay errors verbatim; never invent a theme, version or link.
