# themes setting values — value format and value domain per control type

A field's value format is set by its control `type`. Read the field schema — `type`, `options`,
`min` / `max` / `step`, `visibleOn`, `info` — and never infer the value type from the field name.
Schema sources: card fields → `themes +page … --section <sid> --include schema`
([page-read.md](page-read.md)); theme settings → `themes session get-config` `schemas`
([global-config.md](global-config.md)). Not every write path checks values
(`session update-config` stores anything), so these rules are the guard.

## Control type → value

| `type` | Value written into `settings` |
|---|---|
| `text` / `textarea` | String |
| `richtext` | HTML string with basic tags only (`<p>` `<br>` `<strong>` `<b>` `<em>` `<i>` `<u>` `<span>`) |
| `html` | HTML string (`<html>`, `<head>`, `<body>` are stripped) |
| `header` / `paragraph` / `page_builder` | No value — editor display items, absent from `settings`; skip |
| `checkbox` | Boolean `true` / `false`, not a string |
| `range` | Number within `min`–`max`, on a `step` (with `unit`, still a bare number) |
| `select` | One of the field's `options[].value` — never the label, never invented |
| `date_picker` | Date string in the field's `format` (no `format` → same format as the current value; a wrong format can't be read by the control). `props.min` / `props.max` bound the range. Clear with `""` |
| `color` | Color string (hex / RGB / RGBA / HSL) |
| `color_scheme` | A scheme id string such as `scheme-1` — a reference, not a color. Schemes are defined in theme settings; changing the reference affects only this card |
| `color_scheme_group` | The value of the theme settings' `color_schemes` key: an object keyed by scheme id, each `{deletable, settings:{…}}`. Writing it changes scheme definitions — every card using them follows |
| `color_group` | `{"color":"<hex>","value":"<option value>"}` |
| `font_picker` | Theme settings: a handle string (`lato_n4`). Card field: a full font object — copy the current value's shape. Handles only from [fonts.md](fonts.md): never concatenate, derive, or change a weight suffix yourself |
| `spacing` | `{"pc":{"top","right","bottom","left"},"mobile":{…}}`; the eight values are numeric **strings** (may be `""`) |
| `tags` | Array of strings |
| `image_picker` | Image URL or path string; unset `""` |
| `video_picker` / `video_url` | Video object — copy the current value's shape |
| `link_list` | Menu object `{id, type:"menu", title, url}`; unset `{}`. A real menu → [resource-binding.md](resource-binding.md) |
| `url` | Resource object `{id, type, url, title}` for the chosen kind of resource; unset `""`. → [resource-binding.md](resource-binding.md) |
| `product` / `collection` / `blog` / `article` / `page` | Full object of a real resource; unset `null`. → [resource-binding.md](resource-binding.md) |
| `source` | An app card's data-source picker: the object of the chosen real record; unset `null`. → [resource-binding.md](resource-binding.md) |
| `app_link` | Object filled by the admin picker — never invent it |

## Value domain: where a value may come from

| Class | Legal values come from | Rule |
|---|---|---|
| Enum / number (`select`, `color_group`, `range`) | The field's `options[].value` / `min`–`max`–`step` | Only from there. Map the wording to an option's `label`, then write that option's `value` — don't translate the wording into a value of your own. A value the user gives that is out of range → state the legal range and don't write it; never clamp silently |
| Reference (`color_scheme`, `font_picker`, image, video, resources) | Real things outside the field schema | Never invent. A `color_scheme` id → check it exists (`get-config`, [global-config.md](global-config.md)); missing → say so and list the existing schemes. Resources → [resource-binding.md](resource-binding.md). Image URLs: only one the user provides, or one already in the store's media library (`shop files list`, shoplazza-shop). Write it whole, domain included. Card defaults are often bare paths like `<hash>.png`; both forms work. Videos must already be uploaded |
| Free (`text`, `richtext`, `checkbox`, `spacing`, `tags`) | The user | Mind the format only. Copy the user gives is written verbatim — no rewriting or polishing (copy language: SKILL.md → Rules for every operation, 7) |

## No concrete value given

Read the field's legal values and current value first, then:

- **A direction is given** (bigger, darker, tighter) → pick a fitting value from the current one
  in that direction and write it; in the summary say "per '<direction>': <old> → <new>" so the
  user can check. Don't stop to ask.
- **No direction** ("adjust it", "make it fit", "your call") → **don't write this field**. Finish
  the other fields that don't depend on the answer, and offer 2–3 fitting candidates (with their
  `label`s) for this one. Not chosen = not done: the summary says it's pending, not done. The same
  for copy: no new copy given → offer candidates or ask; never make it up.

### An icon or graphic with no matching option

The request names an icon or graphic and no entry in the `select` field's `options` matches. Don't
substitute the nearest option, and don't describe a substitute as "similar". Look for an
`image_picker` field at the same level of `settings` (the card's top level or the same block):

- **There is one** → ask for an image URL (or offer images from `shop files list`); once given,
  write it to that `image_picker` and leave the `select` alone. Don't draw an SVG and don't
  generate an image.
- **There is none** → say the presets have no such icon and list the existing options' `label`s
  for the user to choose from.

Both are pending until the user answers: the summary says so.

## Resource-object values

The value is the full object of the chosen real resource (id plus url, title, image…), never a
bare id:

```json
"collection": {"id": "<collection_id>", "type": "collection", "title": "<title>", "url": "/collections/<handle>", "image": []}
```

- Key sets differ by card and theme (`image` an object or `[]`; `url` or `seo_url`): read the
  card's current object (`+page --section <sid>`) and fill the same keys. The link key must be
  present — the card uses it as the click target. Per-type mapping:
  [resource-binding.md](resource-binding.md) → Field mapping.
- No specific resource named → keep the current value.
- An id alone (from earlier output, or given by the user) locates the object but isn't the value:
  look it up and fill url, title and the rest.

## visibleOn (conditional fields)

A field with `visibleOn` (e.g. `"visibleOn": "enable_custom_height"`) takes effect only when its
condition holds; some conditions are written only in `info` — read both. Condition not met → in
the same `replace_props` also set the gating field so the condition holds, or tell the user that
X has to be turned on first. Never write into an inactive field, and never write a similarly named
field instead.

Numbers and booleans are never strings — keep the current value's type (`spacing`'s numeric
strings are the exception). Change values only, never the field definitions.
