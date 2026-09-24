# themes market — browse, describe and install market themes

The theme market is the platform's catalog of installable themes — ones not yet in the store.
Market rows are identified by `remote_theme_id`; installing one creates an installed theme with
its own `theme_id`. The two ids are not interchangeable: rename / duplicate / upgrade / delete /
publish ([lifecycle.md](lifecycle.md)) act only on installed themes.

## Commands

| Intent | Command |
|---|---|
| Browse (one row per theme × preset) | `themes market list --jq '…'` (projections in Recipes) |
| One row per theme (Default presets only) | `themes market list --params '{"preset_default":"1"}' --jq '…'` |
| How many | `themes market list --jq '.data.total'` |
| Install | `themes market install --params '{"remote_theme_id":"<remote_theme_id>"}' --data '{"preset":"<preset_name>","name":"<name>"}'` → `.data.data.data.theme_id` |

`limit` defaults to 200 (also the maximum), so one call usually returns the whole market;
`total_pages` > 1 → read the next `page`. Rows are large (full `desc`, `presets`, tens of KB
each) — always project with `--jq`.

## Data model: theme × preset

- Each row is one theme in one style preset; `name` + `preset_name` identify it, and each row is
  a separately installable choice for the merchant (`total` counts rows).
- Rows with the same `name` are presets of one theme and share one `remote_theme_id`; the preset
  to install is chosen by `preset` in the install body.
- `remote_theme_id` is the install key — not `merchant_themes[].id`, and not an installed
  `theme_id`.
- Already installed? An installed theme's `merchant_theme_id` equals the market row's `id`, and
  its `preset` equals `preset_name` (compare with `themes list`).
- `default` on a market row is a catalog flag, not the store's default theme.
- `is_paid` can be missing and `price` empty — call a theme free or paid only when the row says so.

## Filtering

Filter locally on the full list (one call, ≤200 rows). Never pass a filter code you haven't seen
in a response.

| User asks about | Match on |
|---|---|
| Industry | `preset_data.industry[]` |
| Product category | `preset_data.category[]` |
| Features (RTL, COD, quick view, cart drawer, …) | `preset_data.features[]` (a few rows spell it `feature`), plus the `exts` tags |
| Storefront language | `preset_data.language[]` |

- Each taxonomy item is `{name, value}`: `name` is the merchant-facing label (often Chinese),
  `value` a short English code. Match the user's words against either, by meaning — translate
  as needed.
- `exts` is a JSON string; its tags are at `(.exts | fromjson).tags["en-US"]` / `["zh-CN"]`.
- Some rows have no taxonomy in `preset_data`, only `exts` tags; they never match a taxonomy
  filter — mention them when the request is broad.
- The server-side `industry` / `category` / `features` / `language` params need those exact
  codes; filtering locally avoids guessing one.

## Flow

**Browse / describe.** A general or filtered browse → the browse projection. A named theme → the
describe projection with its exact `name` from the list (add `preset_name` when a style was
named). Nothing matches → show the list and ask; never guess.

**Install.**

1. Pick the row: reuse `remote_theme_id` + `preset_name` from a market read earlier in the
   conversation; otherwise read the list.
2. Pick the preset: one row → it; a style named → that `preset_name`; several and none named →
   `Default`, and mention the other styles; no `Default` row → list the preset names and ask;
   nothing matches → back to the list, never guess an id.
3. Say in one line: theme + preset, "installs as an unpublished theme — the live store doesn't
   change". Then install; no consent step (it doesn't touch the live theme).
4. Report the new theme. Install is synchronous: the theme and its files exist at once, with
   `published:"0"`. Going live is a separate `themes publish` with consent
   ([lifecycle.md](lifecycle.md)); later edits pass `-t <new theme_id>`.

## Rules

- `preset` and `name` go in `--data`, never `--params`. Both optional: no `preset` → the
  theme's default preset; no `name` → a server-side name. Don't stop to ask for a name.
- Don't send `default` (going live is `themes publish`) or `version` (ignored — the market's
  current version is installed).
- One theme per call; several themes → install one after another and report each.
- Paid-theme errors (not purchased, plan limit) → relay the message as-is; don't retry or work
  around them.

## Recipes

```bash
# Browse: one row per theme × preset
themes market list --jq '{total: .data.total, rows: [.data.merchant_themes[] | {name, preset_name, remote_theme_id, c_version, is_paid}]}'

# Describe one theme (every preset of it); use desc.zh_CN for a Chinese reply
themes market list --jq '[.data.merchant_themes[] | select(.name == "<name>") | {name, preset_name, remote_theme_id, c_version, preview_url: (.preset_data.preview_url // .preview_url), industry: [.preset_data.industry[]?.name], category: [.preset_data.category[]?.name], features: [(.preset_data.features // .preset_data.feature)[]?.name], tags: ((.exts // "{}") | fromjson | .tags["en-US"] // []), desc: ((.preset_data.desc.en_US // .desc // "") | gsub("<[^>]*>"; " ") | .[0:400])}]'

# Local filter on a taxonomy label or code taken from the data
themes market list --jq '[.data.merchant_themes[] | select(any(.preset_data.category[]?; .name == "<label>" or .value == "<code>")) | "\(.name) · \(.preset_name)"]'

# Install, then confirm
themes market install --params '{"remote_theme_id":"<remote_theme_id>"}' --data '{"preset":"<preset_name>"}' --jq '.data.data.data.theme_id'
themes get --params '{"theme_id":"<new theme_id>"}' --jq '.data.theme | {id, name, published, preset, c_version}'
```

## Errors & recovery

| Symptom | Do |
|---|---|
| Install fails on a paid theme | Relay the message verbatim; no retry |
| The named theme or style matches no row | Show the list (or that theme's presets) and ask |
| A filter finds nothing | Say so and offer the unfiltered list; don't loosen the match silently |
| Install errored and it's unclear whether the theme was created | `themes list` and look for it before any retry |

## Output

- Browse: total first; one line per row — name, preset (always shown when not `Default`, e.g.
  "Nova 2023 · Night"; same-name rows are not merged), version, free/paid only when known, and
  "installed" when a matching installed theme exists. End by offering details or a demo for any
  named theme.
- Describe: industries, categories, features, a one-sentence summary of the description. The
  `preview_url` is a public demo store and can be shared as-is.
- Install: new theme name and `theme_id`, unpublished; the live theme is unchanged.
