# themes block self-check — check a finished AI-card file by hand before writing it

The server rejects only liquid parse failures: a syntax error (unknown or unclosed tag) or a
`{% schema %}` that isn't valid JSON. Everything else is saved silently and breaks on the
storefront or in the editor with no error anywhere, so **this check is the only gate**. Go through
the whole file after writing or editing it, fix every hit, then go through it again (round limit:
Flow in [generate-block.md](generate-block.md) / [edit-block.md](edit-block.md)). Every item is
must-fix unless marked *advisory*. The rules behind the items: [liquid-rules.md](liquid-rules.md),
[schema-rules.md](schema-rules.md), [mobile-rules.md](mobile-rules.md), [objects.md](objects.md),
[ljs/writing.md](ljs/writing.md).

## Syntax

- Each block tag closes with its own `end<tag>` (never a bare `{% end %}`); `else` / `elsif` /
  `when` sit inside their own parent. → Save rejected.
- No `{% liquid %}`, `{% echo %}`, `{% style %}`, `{% doc %}`, `{% sections %}`; no page-level
  `{% section %}` / `{% layout %}` / `{% block %}`; no `{% gassign %}`. → Save rejected, or other
  cards on the page change with this one.
- Every filter is a Shopify filter not listed under liquid-rules → Shopify features that don't
  exist here, or a Shoplazza filter liquid-rules teaches. → An unknown filter silently returns its
  input or last argument (`image_url: width: 800` prints `800`).
- Conditions use only `and` / `or` and `==` `!=` `>` `<` `>=` `<=` `contains`. → `&&` / `||`
  silently drop the right side; `===` breaks the whole page.
- Nothing piped after an HTML-producing filter (`img_tag`, `placeholder_svg_tag`,
  `placeholder_img_tag`, `link_to`, `script_tag`, `stylesheet_tag`); `capture` the argument first.
  → The extra text renders as visible text beside the tag.
- No `-` trim markers (`{{- -}}`, `{%- -%}`). → `{{- x -}}` reads a variable named `-`: blank.
- No quoted-literal index (`settings['key']`) → parse failure. Loop ids use `capture`, not
  `assign … | append` on `forloop.index0` → empty ids; tabs / panels stop linking.

## Schema

- Exactly one `{% schema %}`, strict JSON (no comments, trailing commas, single quotes). → Rejected.
- Top-level `name` equals every `presets[].name`, lowercase letters / digits / underscores only
  → otherwise "Add" in the editor returns 404. Top-level `desc` and `settings` (≥ `[]`) present.
- Every setting `type` is under schema-rules → Allowed setting types; no Shopify-only types, no
  `color_scheme` / `color_scheme_group`. → The control silently vanishes from the panel.
- Setting ids unique per level, lowercase snake_case; repeatable content is inline sub-blocks, not
  `title_1` / `title_2`. → A duplicate id overrides the earlier one.
- `label` / `info` / `options[].label` are bilingual objects; `header` / `paragraph` use `content`
  and no `id`; `image_picker` / `video_picker` put their text in `info`. → Blank or garbled labels.
- `range`: `min`, `max`, `step`, `default`; `step` divides `max − min`; `default` in range. → Wrong slider.
- `date_picker`: `format` is `YYYY-MM-DD` or `YYYY-MM-DD HH:mm:ss` (+ `props.showTime: true`);
  preset value and `props.min` / `props.max` in that format; no `default`. → Dates and countdowns
  don't render.
- `visibleOn` names only a sibling boolean setting. → A bad expression crashes the whole panel.
- Inline sub-blocks: one level; `type` a bare name (no `blocks/`), identical in `blocks[]` and
  `presets[0].blocks[]`; `max_blocks` set when the count is fixed or capped. → Items don't render,
  or adding the card is rejected.

## Presets

- `presets[0]` has `name`, bilingual `cname` and `category`, `display: true`; with sub-blocks, 2–3
  `presets[0].blocks` entries. → Missing from the add panel, raw type name, empty first preview.
- Every declared id, root and per sub-block type, has a value in `presets[0].settings` /
  `presets[0].blocks[].settings` — empty strings, settings with a `default`, and nested objects
  (`spacing`) included. → Those fields start empty: instances copy `presets[0]`, not `default`.
- No undeclared preset key or `presets[0].blocks[].type`. → Never read; the add is rejected.
- Value types match: `range` a number, `checkbox` unquoted `true`/`false`, `color` `#RRGGBB`. → Wrong type.
- All copy (`text` / `textarea` / `richtext` defaults and preset values, root and sub-blocks) is
  non-empty, in the one card copy language; dictated lines verbatim. → Two languages side by side;
  setting defaults aren't translated for buyers.

## Settings consistency (declared ⇔ used)

- Every `block.settings.<id>` / `item.settings.<id>` read is declared at that level. → Reads empty;
  its condition is always false; that part never renders.
- Every declared setting is read and changes the output (markup, CSS or an attribute). → The
  merchant changes it and nothing happens.
- No `settings.*` (theme-wide) and no `section.settings.*`. → Empty on other themes or containers.
- Resource settings resolved as objects.md shows (`all_products[…]`, `collections[…]`, `url` →
  `.url`, `image_picker` is a key, `video_picker` an object). → That part renders blank.
- A declared `content_width` renders as `max-width` + `margin-inline: auto` on an inner wrapper; a
  declared `spacing` renders. → Dead control, or the card sticks to the left.
- `textarea` output uses `| newline_to_br`; full rich-text HTML uses `| spz_img`. → Line breaks
  collapse; embedded images jump.

## Objects

- No page-scoped root object read directly (`product`, `collection`, `article`, `blog`, `page`,
  `cart`, `search`, …) — the card can land on any page; use its own resource settings with the
  `default_product` / `default_collection` fallback. → Blank on every other page.
- Only root objects and fields listed in liquid-rules / objects.md; no theme snippets via
  `{% include %}` / `{% render %}`. → Invented names read empty; snippets differ per theme.
- Fallbacks as objects.md writes them (test `p.id`, not `p`; a collection by `products.size`).
  → The fallback never fires.

## Root element & attributes

- One top-level HTML element (not counting `{% schema %}`, `<style>`, comments), whose opening tag
  carries `{{ block.shoplaza_attributes }}` (one "z"); each sub-block root carries
  `{{ item.shoplaza_attributes }}`. → Merchants can't select the card or item in the editor.
- `block_id` (`section.id` + `block.id`) and `root_cls` built at the top; every id and class
  derives from them. → Two copies of the card on one page collide.
- A `{% comment %}` stating the requirement opens the file (the next edit's only record of intent).
- Images use `ljs-img` (no raw `<img>`) in a slot with fixed height or `aspect-ratio`, with a
  placeholder when blank (keys from liquid-rules only). → Page jumps; blank slots collapse.
- *Advisory:* no hard-coded `$` / `￥` beside a price; use `money_with_symbol` or `ljs-currency`.

## CSS scoping & mobile

- Every `<style>` selector starts with the card scope (`.{{ root_cls }} …`, or an id derived from
  `block_id`); `@keyframes` steps aside. → Two copies of the card override each other.
- No global selectors (`body`, `*`, bare tags) and no theme utility classes. → Theme styling
  breaks; utility classes have no CSS in the card.
- Mobile first, desktop in `@media screen and (min-width: 960px)`; no `100vw` or fixed widths past
  the viewport; `svh` / `dvh`, not `100vh`; inputs ≥ 16px; no `background-attachment: fixed`
  (rest in [mobile-rules.md](mobile-rules.md)). → Sideways scroll, zoom jumps, broken phone page.

## JS safety

- Custom logic only in `<ljs-script>`, which has no `type` and has `scope="<card root id>"`;
  static JSON in `<script type="application/json">`. → Unscoped script reaches other cards.
- No `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`, `eval`, `new Function`; use
  `textContent` or an `ljs-render` template. → Merchant input becomes injected script.
- No requests to absolute or external URLs; only same-origin relative paths or `ljs-render` /
  `ljs-list` sources. → Store data leaves the store.
- No hand-registered `customElements`; no JS for what `@tap` or a component action already does.

## ljs usage

- Each `ljs-*` used has its doc read this round (`ljs/ljs-<name>.md`); attributes, actions and
  events exactly as named there, plus that doc's own required items. → Unknown attributes are
  ignored silently.
- `layout` per the table in [ljs/writing.md](ljs/writing.md); boolean attributes bare (never
  `="false"`); linked ids from one `capture`.
- `<template>` content follows [ljs/template.md](ljs/template.md); `ljs-product-form` /
  `ljs-variants` / `ljs-quantity` only in a product card's purchase area ([ljs/selection.md](ljs/selection.md)).
