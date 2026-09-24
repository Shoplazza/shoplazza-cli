# themes global config — theme-wide settings (colors, fonts, layout, buttons, cart…)

The theme settings panel: store-wide style and behavior, grouped in categories (colors or color
schemes, typography, layout, buttons, cart, animation switches…). Read with
`themes session get-config`, write with `themes session update-config`, both inside the task's
edit session. Theme settings are shared by every page: `doc_id` is always `"index"` — never ask
which page.

## Commands

| Intent | Command |
|---|---|
| List category names | `themes session get-config --params '{"oseid":"<oseid>","doc_id":"index"}' --jq '[.data.data.schemas[].name]'` |
| One category: fields + current values | `themes session get-config --params '{"oseid":"<oseid>","doc_id":"index","name":"<category>"}'` |
| One current value | `themes session get-config --params '{"oseid":"<oseid>","doc_id":"index"}' --jq '.data.data.settings.<id>'` |
| Write (draft) | `themes session update-config --params '{"oseid":"<oseid>","doc_id":"index"}' --data '{"settings":{"<id>":<value>}}'` |
| Preview | `themes +preview -t <theme_id> --oseid <oseid>` |
| Save / publish | `themes +edit -t <theme_id> --template index --session <oseid> --ops '[]' --promote [--publish]` |

No session yet → `themes +page -t <theme_id> --template index` → `.data.oseid`
(SKILL.md → Workflow — editing a theme page).

## What get-config returns

`.data.data` (double `data`) has two parts, joined by the setting `id`:

- **`schemas[]`** — categories. Each has `name` (multilingual object, e.g.
  `{"en-US":"Colors","zh-CN":"颜色"}`) and `settings[]` with `id` (the key to write), `type`
  (control type), `label` (multilingual), and when present `info`, `default`,
  `options[{value, label}]`, `min` / `max` / `step` / `unit`. Without a `name` filter the first
  item is `theme_info` (theme name and version) — skip it. `header` / `paragraph` items have no
  `id` and no value — skip them.
- **`settings`** — flat `{<id>: <current value>}`. Old values, and whether a key exists, come
  from here.

Categories and fields differ by theme: whether there is an animation category, whether fonts sit
under Typography or Format, whether logo width is a theme setting. Pick fields by the meaning of
`label` / `info` (either language), never by the key's spelling or from memory: list categories,
then read fields.

### Choosing a query

| Known | Query |
|---|---|
| The category (buttons, colors) but not the field | Filter by that category, then pick the field by `label` |
| The field and the target value | Filter by its category; unsure which → list category names first |
| "What can I change in this theme?" | No filter; list category names only and let the user pick |
| One current value, no change intended | `--jq '.data.data.settings.<id>'` |

`name` is a case-insensitive substring match against both languages of the category name
(`col`, `Colors`, `颜色` all hit Colors; `products` hits Products and Products Card), and
`settings` is narrowed to the matched categories. No match → `schemas:[]`, `settings:{}`, not an
error: re-query without `name` and list the category names.

## Where the request lands

Only keys present in `settings` are theme settings:

- A key with the matching meaning is in `settings` → write it here.
- Not there → it is most likely a card field. Global cards (header, footer, announcement bar, cart
  drawer) are shared store-wide but are cards: edit them with `+edit` using their `section_id`
  as the target ([card-edit.md](card-edit.md)). Don't pass `section_id` to `update-config` — it
  fails with `section <id> not found in edit session`.
- Neither has it → this theme has no such setting. Say so; never guess a key.

The request names no card and both layers have the same `label`: the card field is usually a
toggle (show or hide an element), the theme setting a form choice (how many rows, which style,
which ratio). Changing the form → theme setting; toggling an element → card.

A card read that keeps failing (`+page`, retried) doesn't mean the card lacks the field: report
the failed read; don't guess a target, and don't change a theme setting in place of a one-card
request.

## Rules

1. Before saying "the theme settings have / don't have X", this task must have run `get-config`.
2. **`update-config` validates nothing.** Invalid colors, out-of-range numbers, values outside
   `options`, unknown keys — all are saved as sent, and `null` does not remove a key. Check every
   value against its field from `get-config` before writing: formats and value domains in
   [setting-values.md](setting-values.md); font handles only from [fonts.md](fonts.md).
3. Send only the keys that change; put all changes of one task in one call. Other keys stay as
   they are.
4. References and composite values (color scheme, font, image, spacing), or a request that gives a
   direction instead of a value → [setting-values.md](setting-values.md).
5. It is a draft write: run it directly, then stop at the preview.
6. Editing a card's `{% stylesheet %}` source is not supported — say so.

## Colors: two forms

Use whichever form `get-config` shows:

- **Scheme group** (control `color_scheme_group`) — `settings.color_schemes` holds `scheme-1` …
  `scheme-N`, each `{deletable, settings:{<color keys>}}`, referenced by cards' `color_scheme` fields. Changing a scheme's definition changes every
  place that references it. List scheme ids:
  `--params '{"oseid":"<oseid>","doc_id":"index","name":"Color"}' --jq '.data.data.settings.color_schemes | keys'`.
- **Flat colors** — no scheme group; the Colors category is dozens of single `color_*` fields,
  changed one by one.

### Changing the overall tone (flat colors)

When no per-field colors are given and the values must be derived from a direction: sort the
`color_*` fields by `label` into three layers, give every layer a value, and write them in one
call. Changing text and accents while leaving the background produces the opposite of the
intended look (light background with darkened text, or the reverse).

| Layer | Typical `label` words | Rule |
|---|---|---|
| Background | general / header / footer / card background (常规背景 / 页头背景 / 页尾背景 / 卡片背景) | Must get a value — without it the palette doesn't hold |
| Text | general text / headings / product title / header text (常规文字 / 标题 / 商品标题 / 页头文字) | Enough contrast with the new background |
| Accent | button background / badge / price / tag (按钮背景 / 标签 / 价格 / 角标) | Carries the accent color of the direction |

Turning a direction into values: [setting-values.md](setting-values.md) → "No concrete value
given". In the summary list `<layer>: <old> → <new>` per layer, and say these are the store-wide
default colors — cards that use the defaults change with them.

## Style requests

A setting exists → change its value (card field → [card-edit.md](card-edit.md); theme key →
here). For a store-wide hover effect, first look for a native option in the button category.
Effects no setting expresses — hover and focus states, whole-page layout or background, fine
styling of one element — are not supported: say so, and don't write CSS. Describe visible effects
to the merchant, not CSS or selectors.

## Output

- `update-config` returns only `data.section` = the partial you sent. Re-read with `get-config`
  (same category) and report the re-read values: `<label>: <old> → <new>`.
- A shared item changed (a color scheme, a color key, a font) → state the reach: this is the
  store-wide default, and cards without their own value follow it.
- Give the preview link and stop. `update-config` has no `--promote`: theme settings share the
  session draft with card edits and are saved or published together with
  `+edit … --ops '[]' --promote` (SKILL.md → Workflow — editing a theme page, steps 7–8).

## Errors & recovery

| Error | Meaning / fix |
|---|---|
| 400 `settings is required and must not be empty` | Nothing to write — send at least one key |
| 400 `section <id> not found in edit session` | `section_id` was passed — edit the global card with `+edit` instead |
| 404 on `get-config` | Check `oseid` belongs to this theme; `doc_id` must be `"index"` (not `index.liquid`) |
| Session expired, promote conflict | [card-edit.md](card-edit.md) → Errors & recovery |
