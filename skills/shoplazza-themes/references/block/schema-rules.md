# themes block schema — rules for a card's `{% schema %}`

Every rule for the `{% schema %}` of a block file: structure decisions, choosing setting types, and
a full field reference (JSONC, each field marked required / optional with its value shape). When a
rule conflicts with the field-reference example, the rule wins. Liquid body rules →
[liquid-rules.md](liquid-rules.md); terms → [concepts.md](concepts.md).

## Structure

### Strict JSON

`{% schema %}` must be strict JSON: no comments, no trailing commas, no single quotes. The
[Full field reference](#full-field-reference-jsonc) is an annotated JSONC version — strip every
comment when copying from it.

### One block file, at most one level of inline children

The unit of output is one block file (`blocks/gen_<id>.liquid`; the server assigns the name). The
section that hosts it is created by the server and holds no settings, so every card-level setting is
declared in the block's own `settings`.

| Need | Where it goes |
|---|---|
| Card-level configuration (title, colors, content width, spacing on four sides, toggles, layout…) | The root block's top-level `settings` |
| A list of items that can be added / removed (FAQ entries, carousel slides, pricing columns…) | Declare **inline child blocks** in top-level `blocks`; the body renders them with `{% for item in block.blocks %}` + `item.settings.*`, and each child root element carries the selection anchor from [liquid-rules.md](liquid-rules.md) → "Card root". Don't use `content_for` (that is the external-file path) |
| Data the platform already has: products, collections, articles | One resource setting (`collection` / `product` / `article`) picks the data source, and the body loops over the object it returns — not inline children |

- Express repeating content as inline children, one child per item — never numbered settings such as
  `title_1` / `title_2`.
- Product lists come from a collection, not from children that each hold a product: `product` is a
  single-pick type and the platform has no multi-product setting, so a list assembled from N children
  leaves count, order and unpublished products to manual upkeep by the merchant. The list-card
  skeleton is in [kinds/product-card.md](kinds/product-card.md).
- One level only: a child is a flat set of settings with no `blocks` / `max_blocks` of its own.
- For cards whose item count is fixed or capped (three fixed selling-point columns, a two-column
  comparison), cap it with top-level `max_blocks`. Without it the editor's default limit applies, and
  the merchant can keep adding items until the layout breaks.

### Inline child `type` is a bare name

Use a bare name (such as `slide`), identical in the top-level `blocks` declaration and in
`presets[].blocks`, without a `blocks/` prefix — the prefix means "reference a separate file", a
multi-file pattern a single-file card doesn't use. A mismatch means the children never show up.

### Defaults live in two places

Every `settings[]` entry that has an `id` must appear in `presets[0].settings`, none omitted —
including entries whose value is an empty string, entries that already have a `default` (such as
`spacing`), and entries whose value is a nested object.

- With inline children, `presets[0].blocks` preloads 2–3 children with `settings`; the same
  full-coverage rule applies to each child.
- Inline children are written in both places: the child's `settings[].default` and
  `presets[0].blocks[].settings`.
- Write non-empty business copy in `default` and in presets, in the card copy language (how it is
  chosen: [generate-block.md](generate-block.md)). If the request dictates an exact phrase, the card
  copy language follows that phrase even when it differs from the storefront's language: never
  translate or rewrite a phrase the user fixed — change the language of the rest instead. Root
  settings, inline children and presets contain only this one language, and
  [self-check.md](self-check.md) checks against it.
- Chinese strings in the field-reference example only show the shape.
- No `| t` and no locale fallbacks (where visible copy lives: [liquid-rules.md](liquid-rules.md) →
  "Copy and i18n").

### Every setting needs a reason

- Upper bound: only the adjustable points the request names get a setting, plus the standard
  `content_width` — declare that one regardless of the request; the only exception is a card that
  bleeds full-width, which gets no `content_width`. Don't add mode switches, style tiers or spare color
  sets "for flexibility": such fields need `visibleOn` wiring to avoid clashing, and a wrong
  `visibleOn` expression silently crashes the whole settings panel (see
  [Setting skeleton](#setting-skeleton)). Hard-code any form the request doesn't mention in the CSS.
- Granularity: fields follow the elements the request names. "Text" is one `textarea`; only
  "title + description" splits into `title` and `text`.
- Lower bound: every visual / behavioral setting must really be used in the HTML / CSS / `style`, and
  every id the Liquid reads must be declared in the schema. [self-check.md](self-check.md) reports
  mismatches in both directions.
- Slots that only add capacity and should hide when empty get `default` `""`; an empty value doesn't
  render.

## Settings

### Setting skeleton

- `type` `id` `desc` `default` `visibleOn` are skeleton keys common to every type; any type may carry
  them, and the panel handles them uniformly. The per-type lines in the field reference list only
  type-specific fields — a missing `desc` / `default` there doesn't mean you shouldn't write it.
- `desc`: one sentence of plain English, never a bilingual object, and it doesn't follow the copy
  language — it isn't shown in the editor panel; it describes the card for programs that read it. The
  top-level card `desc` is required: say what the card is, when to pick it, and which fields it
  exposes. A setting's `desc` is written only when its `label` doesn't make clear what it controls;
  skip it when the `label` and `options` wording already say it.
- `label` / `info` / `options[].label`: bilingual objects `{ "zh-CN": "…", "en-US": "…" }`, never
  plain strings. Exceptions: `header` / `paragraph` carry their text in `content` and have no `id`;
  `image_picker` / `video_picker` have no `label` — put the description in `info`.
- id: lowercase with underscores, `{purpose}_{property}`; no synonyms for the same thing in one file.
  It maps to `block.settings.<id>` in Liquid.
- `visibleOn`: for a boolean parent field, write the parent's id. Only wire boolean parents; no
  compound expressions — the editor evaluates the value as a restricted expression that can only
  reference sibling fields, and a wrong expression either leaves the field always visible or crashes
  the whole settings panel, with no user-facing error.

### Choosing a type

| Need | Use |
|---|---|
| One line of copy / multi-line description / rich text | `text` / `textarea` / `richtext`. When the card controls font size and color itself, use `textarea`: `richtext` stores HTML with inline styles that override the size and color the card's settings set |
| Toggle | `checkbox` |
| Continuous value (height, delay, column count…) | `range` |
| Enumeration | `select` + `options` |
| Card colors (background, title, body, button…) | `color`, one per configurable part |
| Spacing on the card's four sides | `spacing`, id fixed as `spacing`; default in the field reference |
| Card content width | `range`, id fixed as `content_width`; cards that bleed full-width don't declare it |
| Image / store video / external video | `image_picker` / `video_picker` / `video_url` |
| A fixed date or time (campaign end, launch day) | `date_picker`, never a `text` the merchant types into — typed formats are unconstrained, while Liquid and components parse a fixed format. To include hours and minutes also set `props.showTime` |
| Icon slot | `image_picker` with the icon size in `info`; when the merchant doesn't replace it, the card's inline SVG is the fallback ([liquid-rules.md](liquid-rules.md) → "Icons: inline SVG by default, `image_picker` to override") |
| Link / navigation menu | `url` / `link_list` |
| Product / collection | `product` / `collection` — the value must go through `all_products[…]` / `collections[…]`, see [objects.md](objects.md) → "Getting real data from a setting" |
| Group header / explanatory paragraph | `header` / `paragraph` |

### Allowed setting types

- Use these types (each has an example and its fields in the field reference): `text` `textarea`
  `richtext` `html` `checkbox` `range` `select` `custom_select` `header` `paragraph` `color`
  `font_picker` `image_picker` `video_picker` `video_url` `url` `link_list` `page` `article` `blog`
  `product` `collection` `spacing` `date_picker`. Other types appear in some theme schemas; don't use
  them in a card.
- Never use `color_scheme` / `color_scheme_group`. Declare card colors with one `color` per part
  ([Choosing a type](#choosing-a-type)).
  - `color_scheme` stores a scheme id, not a color. All its dropdown options come from the theme
    global settings' `color_scheme_group`; when the theme has no scheme table the options are empty,
    the control won't open, and the card gets no color value.
  - `color_scheme_group` defines the scheme table itself and only works in theme global settings. In
    a card schema it shows no control and can't change the store's colors.
- Shopify-only types are silently dropped: `radio` `inline_richtext` `text_alignment` `number`
  `product_list` `collection_list` `metaobject` `metaobject_list` `color_background` `liquid`. Single
  choice → `select`; number → `range` or `text`.

Each type's valid fields are the ones in the field-reference comments. A missing required field
usually doesn't error, but the control is dead.

## Symptoms → fix

[self-check.md](self-check.md) catches most of these before the block is written.

| Symptom | Cause → fix |
|---|---|
| Editable in the editor, but the storefront doesn't change | A setting is declared but never read → every id needs at least one real reference ([Every setting needs a reason](#every-setting-needs-a-reason)) |
| Clicking "Add" in the editor returns 404 | Top-level `name` and `presets[].name` differ, or `name` is Chinese or a bilingual object → use the same lowercase-underscore semantic name in both. The stored file name isn't compared; the server assigns it when the card is created |
| `<!-- block xxx not support. -->`, or the server rejects adding the card | A child `type` doesn't match its declarations, or carries a `blocks/` prefix → the same bare name everywhere ([Inline child `type` is a bare name](#inline-child-type-is-a-bare-name)) |
| Repeating content ended up as `title_1`, `title_2` | Inline children weren't used → restructure per [One block file, at most one level of inline children](#one-block-file-at-most-one-level-of-inline-children) |
| The added card has the right structure but every field is empty | Declared settings are missing from `presets[0].settings` → fill each in per [Defaults live in two places](#defaults-live-in-two-places); children go in `presets[0].blocks[].settings` |
| A slider / toggle gets a wrong-typed value on the instance | A `presets[0]` value doesn't match its `type` (`range` given a string, `checkbox` given `"true"`) → numbers for `range`, `true` / `false` for `checkbox`, both unquoted |
| Two languages side by side on one card | A copy string isn't in the card copy language → decide per [Defaults live in two places](#defaults-live-in-two-places): if the request dictated that string, change the card copy language and the other strings; otherwise change that string |
| A whole control doesn't render | A Shopify-only type → only [Allowed setting types](#allowed-setting-types) |
| A group header is blank | `header` / `paragraph` written with `label` → use `content` (exception in [Setting skeleton](#setting-skeleton)) |
| Slider range is wrong, with no error | `range` lacks `min` / `max` → write `min` / `max` / `step` / `default` explicitly; `(max-min)` divisible by `step`, `default` within `[min, max]` |
| Odd label in the panel | `image_picker` / `video_picker` given a `label` → exception in [Setting skeleton](#setting-skeleton) |
| Video is black; the `if` passes with no video picked | A `video_picker` value is an object, so `!= blank` is always true → loop over `video.sources` for mp4 / hls; empty check in [ljs/ljs-video.md](ljs/ljs-video.md) |
| A countdown doesn't render, or a picked date shows as empty | The `date_picker` `format` isn't one of the two values in the field reference, or the `presets[0]` value, `default` or `props.min` / `props.max` doesn't follow that format → rewrite per the `date_picker` example |

## Full field reference (JSONC)

Required / optional and value shape for every field. For reference only — a real `{% schema %}`
must be JSON ([Strict JSON](#strict-json)). The example's business copy is written for an English
store; the language `default` and presets actually use follows
[Defaults live in two places](#defaults-live-in-two-places).

The public input-settings reference (shoplazza.dev → Theme → Settings) lists extra options for
some types (e.g. `value_type`, `accept`); the forms below are the ones the Liquid rules in
[liquid-rules.md](liquid-rules.md) expect — keep to them in an AI card.

```jsonc
/* Markers: [required] leaving it out causes problems; [optional] omitted → default applies;
   [conditional] required for certain types / cases */
{
  /* ---------- 1. Top-level fields ---------- */

  // [required] Card identifier. A lowercase-underscore semantic name saying what the card is
  //            (promo_card, faq_list). Must equal presets[].name; only lowercase letters, digits
  //            and underscores — Chinese, a bilingual object, or a mismatch between the two makes
  //            the editor's "Add" return 404. The stored file name is assigned separately by the
  //            server; don't try to match it.
  "name": "promo_card",

  // [required] Card description. Plain English (→ Setting skeleton): what the card is, when to
  //            pick it, which fields it exposes.
  "desc": "What: three-column image-and-text card. When: to showcase three selling points side by side. Key settings: images, text size and color.",

  // [required] Settings array. Write [] even when there are no settings; never omit the key.
  "settings": [ /* see "3. settings" */ ],

  // [required] Presets. Decide whether the card can be added in the editor and what it looks
  //            like once added.
  "presets": [ /* see "2. presets" */ ],

  // [optional] Child block declarations. Without them no children can be attached. See "4. blocks".
  "blocks": [ /* ... */ ],

  // [optional] Panel-wide note (bilingual object).
  "info": { "zh-CN": "用于首页促销位", "en-US": "Homepage promo slot" },

  // [optional] CSS classes appended to the card's outermost wrapper, space-separated.
  "class": "py-5 lg:mt-15",

  // [optional] Per-page instance limit — how many times this card can be added to one page.
  //            Not the child-item limit.
  "limit": 1,

  // [optional] Inline child limit — the most children a merchant can add (→ One block file…).
  //            Omitted → the editor's default limit.
  "max_blocks": 10,

  // [optional] Allowed page types; omitted = all. Usually omitted.
  "templates": ["index"],

  // [optional] Whether the panel starts collapsed.
  "collapse": false,

  /* ---------- 2. presets ---------- */
  // When the merchant clicks "Add", initial values come from presets[0].settings, not from each
  // setting's default.
  "presets": [
    {
      // [required] Identical to the top-level name; a mismatch → the card's settings panel doesn't
      //            show and its settings can't be changed.
      "name": "promo_card",

      // [required] Merchant-facing name (bilingual). Without it the layer list shows the English
      //            type name.
      "cname": { "zh-CN": "促销卡片", "en-US": "Promo card" },

      // [required] Category in the add panel (bilingual).
      "category": { "zh-CN": "内容展示", "en-US": "Content" },

      // [required] Whether it appears in the add panel.
      "display": true,

      // [required] Initial values on add; key = setting id. Every settings[] entry with an id must
      //            appear here: empty strings too, entries that already have a default too, and
      //            nested objects copied in full (→ Defaults live in two places).
      "settings": {
        "title": "Limited-time offer",
        "subtitle": "",
        "show_button": true,
        "columns": 3,
        "spacing": {
          "pc":     { "top": "40", "right": "24", "bottom": "40", "left": "24" },
          "mobile": { "top": "20", "right": "16", "bottom": "20", "left": "16" }
        }
      },

      // [conditional] With inline children, preload 2–3 so the first screen has content to preview.
      "blocks": [
        {
          // [required] The same bare name as top-level blocks[].type (→ Inline child type is a bare name).
          "type": "slide",
          // [required] This child's initial values; every setting with an id in the child
          //            declaration must appear, in the same language as the root settings
          //            (→ Defaults live in two places).
          "settings": { "heading": "Free shipping on all orders" }
        },
        { "type": "slide", "settings": { "heading": "30-day hassle-free returns" } }
      ],

      // [optional] Like the top-level class, applied to instances created from this preset.
      "class": "card-split-spacing"
    }
  ],

  /* ---------- 3. settings[] ---------- */
  // Skeleton: type + id required (header/paragraph excepted); label/info/options[].label are
  // always bilingual objects.
  // Never read an id in Liquid that the schema doesn't declare: it reads nothing, with no error.
  "settings": [
    {
      // [required] Control type; only types from Allowed setting types.
      "type": "text",
      // [required] Value key. Exception: header / paragraph have no id.
      "id": "title",
      // [required] What this setting does.
      "desc": "Sets the card title text",

      // [conditional] Bilingual. Required on every type except header/paragraph/image_picker/video_picker.
      "label": { "zh-CN": "标题", "en-US": "Title" },
      // [required] Source of the initial value when the merchant adds it in the panel; write it both
      //            here and in presets[0].settings (→ Defaults live in two places).
      "default": "Limited-time offer",
      // [optional] Input placeholder hint.
      "placeholder": { "zh-CN": "不超过 20 字", "en-US": "Max 20 chars" },
      // [optional] Note under the control (bilingual).
      "info": { "zh-CN": "留空则不展示", "en-US": "Leave blank to hide" },

      // [optional] Conditional display: for a boolean parent write the parent id, otherwise an expression.
      "visibleOn": "show_button",
      // [conditional] Maximum length for type=text.
      "maxLength": 20
    },

    /* --- Per-type quick reference (pick what you need) --- */

    // Multi-line text: label required; optional default / placeholder / info
    { "type": "textarea", "id": "desc", "label": { "zh-CN": "描述", "en-US": "Desc" } },

    // Rich text: label required; optional default / placeholder / info
    { "type": "richtext", "id": "content", "label": { "zh-CN": "正文", "en-US": "Content" } },

    // Custom HTML: skeleton keys only; no label/info
    { "type": "html", "id": "embed" },

    // Toggle: label required; default falls back to false
    { "type": "checkbox", "id": "show_button", "label": { "zh-CN": "显示按钮", "en-US": "Show button" }, "default": true },

    // Slider: label/min/max required; step defaults to 1 and may be fractional — for a multiplier such
    // as line height write min 1 / max 2.4 / step 0.1 directly instead of converting to an integer
    // percentage and dividing back in Liquid; constraints in Symptoms → fix.
    // unit is appended to the number verbatim and not translated with the panel language, so only
    // use symbols such as px / % / s; for units that need a word (columns, times, slides) omit unit
    // and put it in the label.
    { "type": "range", "id": "columns", "label": { "zh-CN": "列数", "en-US": "Columns" },
      "min": 1, "max": 6, "step": 1, "default": 3 },

    // Card content width: id fixed as content_width, rendered as max-width (see liquid-rules.md →
    // Content width and background come from the block's own CSS)
    { "type": "range", "id": "content_width", "label": { "zh-CN": "内容宽度", "en-US": "Content width" },
      "min": 640, "max": 1600, "step": 20, "unit": "px", "default": 1200 },

    // Dropdown: label/options required; options[].label bilingual too
    { "type": "select", "id": "align", "label": { "zh-CN": "对齐", "en-US": "Align" }, "default": "left",
      "options": [
        { "value": "left",   "label": { "zh-CN": "左对齐", "en-US": "Left" } },
        { "value": "center", "label": { "zh-CN": "居中",   "en-US": "Center" } }
      ] },

    // Styled dropdown: same fields as select
    { "type": "custom_select", "id": "style_preset", "label": { "zh-CN": "样式", "en-US": "Style" }, "options": [] },

    // Group header: content required (not label → Symptoms → fix); no id; optional info
    { "type": "header", "content": { "zh-CN": "内容设置", "en-US": "Content" } },

    // Paragraph: content required; optional style
    { "type": "paragraph", "content": { "zh-CN": "以下为高级配置", "en-US": "Advanced" } },

    // Single color: label required; default a valid #RRGGBB; optional format / disable_alpha.
    // Give each configurable part of the card (background, title, body, button…) its own color;
    // pairing rules in liquid-rules.md → Paired text and background colors.
    { "type": "color", "id": "accent", "label": { "zh-CN": "强调色", "en-US": "Accent" }, "default": "#FF3B30" },

    // Font: label required; optional default
    { "type": "font_picker", "id": "heading_font", "label": { "zh-CN": "字体", "en-US": "Font" } },

    // Image: no label, put the description in info (→ Symptoms → fix); optional product_variant / hiddenMeta
    { "type": "image_picker", "id": "banner", "info": { "zh-CN": "建议 1200×600", "en-US": "1200×600" } },

    // Store video: same shape as image_picker, no label; the value is an object, not a URL (→ Symptoms → fix)
    { "type": "video_picker", "id": "bg_video" },

    // Date picker: label required; format is both the panel display format and the format written
    // back to theme data; props are passed through to the control as-is.
    // format has only two values: with hours/minutes/seconds write "YYYY-MM-DD HH:mm:ss" and also set
    // props.showTime: true; date only, write "YYYY-MM-DD". Any other form (an ISO string with T or a
    // timezone offset) is stored verbatim once the merchant picks a date, and consumers such as a
    // countdown parse dash-separated dates, fail on it, and don't render (→ Symptoms → fix).
    // The presets[0] value for this setting and props.min / props.max follow the same format
    // character for character; range comparison is per day, and the allowed range goes only in
    // props — min / max at the same level as format are not valid fields.
    // No default — with a default, clearing the field makes the input refill it immediately, so it
    // looks impossible to clear.
    { "type": "date_picker", "id": "end_date", "label": { "zh-CN": "结束时间", "en-US": "End date" },
      "format": "YYYY-MM-DD HH:mm:ss", "props": { "showTime": true } },

    // External video link: label required; optional default / placeholder / info
    { "type": "video_url", "id": "yt_url", "label": { "zh-CN": "视频链接", "en-US": "Video URL" } },

    // Link: label required; optional default / urlOnly
    { "type": "url", "id": "button_link", "label": { "zh-CN": "按钮链接", "en-US": "Link" } },

    // Navigation menu: label required; optional default / info
    { "type": "link_list", "id": "menu", "label": { "zh-CN": "菜单", "en-US": "Menu" } },

    // Page / article / blog: label required
    { "type": "page",    "id": "about_page", "label": { "zh-CN": "页面", "en-US": "Page" } },
    { "type": "article", "id": "post",       "label": { "zh-CN": "文章", "en-US": "Article" } },
    { "type": "blog",    "id": "blog",       "label": { "zh-CN": "博客", "en-US": "Blog" } },

    // Product / collection: label required. The value is only an identifier; in Liquid it must go
    // through all_products[...] / collections[...].
    { "type": "product",    "id": "feature_product", "label": { "zh-CN": "商品", "en-US": "Product" } },
    { "type": "collection", "id": "feature_coll",    "label": { "zh-CN": "专辑", "en-US": "Collection" } },

    // Spacing: default is a "four sides × two devices" nested object; values are pixel strings, and
    // empty means unset.
    // Use it for spacing on the card's four sides: id fixed as `spacing`, a value for every side (as
    // below) — the hosting section outputs no CSS, and without left/right values the card touches
    // the screen edges.
    // Having a default doesn't exempt the preset: copy the same object into presets[0].settings.spacing
    // (→ Defaults live in two places).
    { "type": "spacing", "id": "spacing", "label": { "zh-CN": "间距留白", "en-US": "Spacing" },
      "default": {
        "pc":     { "top": "40", "right": "24", "bottom": "40", "left": "24" },
        "mobile": { "top": "20", "right": "16", "bottom": "20", "left": "16" }
      } }
  ],

  /* ---------- 4. blocks[] (inline child declarations; rules in One block file… / Inline child type…) ---------- */
  "blocks": [
    {
      // [required] Bare name, identical to presets[].blocks[].type (→ Symptoms → fix).
      "type": "slide",
      // [required] Layer name in the editor (bilingual).
      "name": { "zh-CN": "轮播项", "en-US": "Slide" },
      // [required] The child's own settings, same fields as "3. settings".
      "settings": [
        { "type": "text", "id": "heading", "label": { "zh-CN": "标题", "en-US": "Heading" }, "default": "Free shipping on all orders" }
      ],
      // [optional] Child panel note.
      "info": { "zh-CN": "最多 8 屏", "en-US": "Up to 8 slides" }
    }
  ]
}
```
