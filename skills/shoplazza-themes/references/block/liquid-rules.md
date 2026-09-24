# themes block liquid — Liquid rules for an AI card's source

Every Liquid rule for writing or editing a block file (`blocks/gen_<id>.liquid`). `{% schema %}`
rules → [schema-rules.md](schema-rules.md); phone layout → [mobile-rules.md](mobile-rules.md);
terms → [concepts.md](concepts.md).

Shoplazza Liquid is highly compatible with Shopify: all standard tags exist and most filters share
Shopify's names and meaning, so Shopify intuition mostly works. The differences sit in three places:
a few patterns must be replaced mechanically, some Shopify features don't exist here, and some
Shoplazza-only semantics fail silently when you leave them out.

The engine degrades silently on the most common mistakes: an unknown filter returns its input
unchanged, an unknown setting type makes the whole setting vanish from the editor, and a misspelled
attributes variable leaves the block unselectable in the editor. "It renders without errors" does
not mean it is right — run [self-check.md](self-check.md).

## Replace these Shopify patterns

| Shopify habit | Shoplazza form | What goes wrong otherwise |
|---|---|---|
| `{{ img \| image_url: width: 800 }}` | `{{ img \| img_url: '800x' }}` | No error; the input comes back unchanged, so `src` gets an object instead of a URL, and with arguments a stray value such as `800` shows on the page |
| `{{ price \| money_with_currency }}` | `{{ price \| money_with_symbol }}` | Filter doesn't exist; input returned unchanged |
| `{% style %}…{% endstyle %}` | Bare `<style>…</style>` (Liquid interpolation works) or `{% stylesheet %}` (deduplicated) | Tag doesn't exist |
| `{{ block.shopify_attributes }}` | `{{ block.shoplaza_attributes }}` | Outputs nothing; the editor can't select the block |
| `settings['key']` (quoted literal index) | Dot access `settings.key`; a variable index `settings[my_key]` and `arr[forloop.index0]` still work | Quotes aren't allowed inside an index; parsing fails |

## Shoplazza-only semantics

### Card root: `block_id`, `root_cls`, and the selection anchor

Build these two variables at the top of the file. Every id, class and linkage anchor in the card
derives from them — never assemble ids separately:

```liquid
{% capture block_id %}{{ section.id }}-{{ block.id }}{% endcapture %}
{% assign root_cls = "ai-block-" | append: block_id %}
```

`block_id` joins both parts because neither is unique alone: a page can hold several copies of the
same card, and `block.id` is only the block's index inside its section — two hosting sections that
each hold one block both render it as 0. `root_cls` is the card-scoped class prefix; the root element
and every CSS rule in the card start with it (see [Styles and scope](#styles-and-scope)).

The root element's opening tag must include `{{ block.shoplaza_attributes }}`:

```liquid
<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
```

It is the editor's selection anchor: in preview it expands into this block's location data. Without
it the merchant can't click the card in the editor. The root element of each inline child block
writes `{{ item.shoplaza_attributes }}` for the same reason:

```liquid
<div class="{{ root_cls }}__item" {{ item.shoplaza_attributes }}>
```

Ids inside a child block start from `block_id` plus `item.id` (`{{ block_id }}-{{ item.id }}`);
classes start with `root_cls` as in [Styles and scope](#styles-and-scope).

### Inline child blocks and the block root

Render inline children with `{% for item in block.blocks %}` + `item.settings.*`. Don't use
`content_for` (that is the external-file path) — see [schema-rules.md](schema-rules.md) →
"One block file, at most one level of inline children". Don't write Shopify's traditional
`{% for block in section.blocks %}`.

- Put styles on the block root's own class; don't rely on an extra outer wrapper.
- Don't give a container `> * { display: contents }`: the children lose their boxes and the whole row
  layout falls apart.

### Styles and scope

- Use real HTML and standard CSS, in a bare `<style>` (Liquid interpolation works) or
  `{% stylesheet %}` (deduplicated).
- The root class is `{{ root_cls }}`, and every CSS rule in the card starts from it:
  `.{{ root_cls }} .faq-item`. With that in place, child classes can use short plain names.
- No global selectors (`body`, `*`, bare element selectors) — they leak into the theme.
- When several text children, or icon + text pairs, sit in one row, use flex + `space-between` to
  spread them evenly.

### The card's own settings: always `block.settings`

Every setting this card declares in `{% schema %}` is read in the body as `block.settings.<id>`, and
in inline children as `item.settings.<id>`. A setting that is declared but never read does nothing
on the storefront when the merchant changes it; [self-check.md](self-check.md) flags it.

`section.settings` is something else. In Shopify a theme block reading it gets `nil`; in Shoplazza a
block reads the settings of the section that hosts it. But the server decides which section hosts the
card, and you can't know what settings that section has, so never treat it as a source for the card's
own settings — it reads empty on the page, with no error.

## Shopify features that don't exist here

### Tags

These don't exist; writing them throws or isn't recognized:

| Shopify tag | Instead |
|---|---|
| `{% liquid %}` / `{% echo %}` | Ordinary tags, one per line / `{{ }}` |
| `{% style %}` | Bare `<style>` |
| `{% doc %}` | `{% comment %}` |
| `{% sections %}` | No replacement; don't write it |

### Filters

Shopify filters that don't exist here. They don't error — they return the input unchanged. Grouped by
replacement:

| Group | Shopify filter (not available) | Instead |
|---|---|---|
| Renamed | `image_url` `image_tag` `article_img_url` | `img_url` / `img_tag` (see [Replace these Shopify patterns](#replace-these-shopify-patterns)) |
| Renamed | `money_with_currency` `money_amount` | `money_with_symbol` / `money_without_currency` |
| Renamed | `camelize` | `camelcase` |
| Renamed | `translate` | `t` |
| Array | `at_least` `at_most` `compact` `sum` `sort_natural` `sort_by` `find` `find_index` `has` `reject` | Combine `if` / `sort` / `where` / `map` |
| String | `remove_last` `replace_last` | `split` and rejoin, or change the requirement |
| Encoding / hashing | `base64_encode` `base64_decode` `base64_url_safe_encode` `base64_url_safe_decode` `blake3` | No base64; for hashing use `md5` `sha1` `sha256` `hmac_*` |
| Color | `color_contrast` `color_difference` `brightness_difference` `color_to_oklch` | No equivalent; use paired color settings instead of computing contrast at runtime |
| Media tags | `media_tag` `video_tag` `external_video_tag` `external_video_url` `model_viewer_tag` | No media object system; images use `img_url` + an image component, video uses a `video_url` / `video_picker` setting plus hand-written markup |
| Account and checkout | `avatar` `customer_logout_link` `customer_register_link` `login_button` `payment_button` `payment_terms` `payment_type_svg_tag` `item_count_for_variant` `line_items_for` `currency_selector` `standard_event_data` `structured_data` | No equivalent; marketing cards shouldn't build these scenarios |

### Data

- Metaobjects and dynamic sources don't exist: don't write `metaobject` references or dynamic-source
  bindings. Express structured repeating content with inline child blocks
  ([schema-rules.md](schema-rules.md) → "One block file, at most one level of inline children").
- Metafields are supported (`metafield_tag` / `metafield_text`).
- `| t` only works for keys that exist in the locale files; a missing key silently returns an empty
  string.

### Engine behavior

- An unclosed block tag or an unknown tag throws (a loud failure). Unknown filters, unknown setting
  types and a wrong attributes variable fail silently.
- Every block tag closes with `end` + the tag name (`{% endif %}`, `{% endcapture %}`); there is no
  `{% end %}`.

## What you may write

### Tags and filters

1. Standard Shopify tags and filters work with Shopify semantics, except the ones in
   [Replace these Shopify patterns](#replace-these-shopify-patterns) and
   [Shopify features that don't exist here](#shopify-features-that-dont-exist-here). If you aren't sure
   a filter exists here, do without it.
2. Shoplazza-only items this file teaches (`img_url`, `money_with_symbol`, `spz_img`, …) follow the
   form shown in their sections.
3. Never invent a name outside those two groups — an unknown filter returns its input unchanged, so a
   typo can't be spotted from the rendered output.

Syntax differences from Shopify:

- Logic is only `and` / `or`. `&&` / `||` silently drop the right-hand condition.
- Comparison is only `==` `!=` `>` `<` `>=` `<=` `contains`. `===` breaks the whole page.
- `section` / `layout` / `block` are page-level composition tags; never use them in a block. The
  `{% block %}` tag has nothing to do with the `block` object you read settings from.
- Never use `gassign`: it writes the global `settings` (unlike `assign`, which stays in the current
  template scope), so other cards on the same page change too.

### Common Shoplazza-only filters

Shopify has no equivalent for these; use them as shown. Other Shoplazza-only filters exist, but a
self-contained card doesn't need them — don't guess their usage from the name.

| Filter | Form | Does |
|---|---|---|
| `add_root_url` | `{{ '/collections/all' \| add_root_url }}` | Adds the store's URL prefix to a site path you built yourself; without it the link 404s on stores that use a prefix |
| `secondary_image` | `{% assign image2 = product.images \| secondary_image %}` | First image from the second one onward, skipping videos; `nil` if there is none |
| `market_rate_exchange` | `{{ block.settings.amount \| market_rate_exchange \| money_with_symbol }}` | Converts a fixed amount the merchant typed into a setting at the current market's rate. Platform prices such as `product.price` are already in the current market's price — converting them again applies the rate twice |
| `ceil_by` | `{{ 23 \| ceil_by: 10 }}` gives `30+` | Rounds up to the next step and appends `+`, for approximate counts (sales, reviews). Exact multiples also move up (30 gives `40+`); non-positive numbers come back unchanged |
| `hex_to_rgba` | `{{ block.settings.mask_color \| hex_to_rgba: 0.6 }}` | Hex to `rgba()`; the argument is alpha. For overlays and shadows |
| `parse_json` | `{{ json_str \| parse_json }}` | JSON string to array / object; on a parse failure the string comes back unchanged |
| `zerofill` | `{{ 3.5 \| zerofill }}` gives `3.50` | Pads decimals to two places; integers aren't padded. The argument changes the number of places |

For site links prefer the object's own `.url` (`product.url`, `collection.url`, `article.url`) and
`routes.*` — both already carry the URL prefix. Only a path string you build yourself needs
`add_root_url`. The `routes` members are listed under [Root objects](#root-objects).

### Root objects

Object access is allow-listed: reading anything that isn't exposed silently gives null. Use these root
objects in a card:

`block` `section` `template` `shop` `routes` `all_products` `collections` `pages` `articles`
`linklists` `images` `customer` `canonical_url`, plus `default_product` / `default_collection`
([objects.md](objects.md)). The name `settings` also resolves, but a card never reads it (see
[Never read the theme's global settings](#never-read-the-themes-global-settings)).

`template` tells which page is rendering: `template.name` is a template name such as `index`,
`product`, `collection`, `cart`, `search`, `blog`, `article` or `page`; `{{ template }}` outputs it
directly.

Shopify globals such as `localization`, `metaobjects`, `current_page`, `page_image`, `scripts`,
`additional_checkout_buttons`, `all_country_option_tags`, `predictive_search` and `recommendations`
are not available. Loop and tag context variables (`forloop`, `tablerowloop`, `paginate`) work
normally.

Use these `routes` members: `root_url` `cart_url` `search_url` `account_url` `account_login_url`
`account_register_url` `account_addresses_url` `account_order_url` `account_reset_password_url`
`account_reset_success_url`. Products, collections and blogs have no `routes` entry; use the object's
own `.url`.

Resource-reference settings (`product` `collection` `page` `link_list` `blog` `article`
`image_picker` `video_picker`) mostly give you a reference in Liquid, not the data. How to get real
data from each: [objects.md](objects.md) → "Getting real data from a setting".

## Writing discipline

These have nothing to do with Shopify differences, and are just as binding.

### Build strings with `capture`

Build dynamic ids that include `forloop.index0` with `capture`, never with `assign` + `| append` —
`assign i = forloop.index0` followed by `append` yields an empty string, so tab / `data-panel` ids
come out empty:

```liquid
{% capture panel_id %}panel-{{ block_id }}-{{ forloop.index0 }}{% endcapture %}
```

Indexing with `arr[forloop.index0]` is fine. So is `append` at the head of a pipeline that builds a
string (the `root_cls` line in [Card root](#card-root-block_id-root_cls-and-the-selection-anchor)) —
appending a string that came out of `capture` works; the empty string only happens with loop indexes.

Filter arguments take only a variable or a literal; build a string with `capture` first, then pass
the variable. A pipeline inside one `{{ }}` is left-associative: in `{{ a | f: x | g: 's' }}`, `g`
receives the result of `f`, not `x`. The same holds for the argument of `img_url:`, `default:` and
`t:`.

❌ `append` lands after the `<svg>` that `placeholder_svg_tag` produced: the class name renders as
visible text next to the image, and the tag itself never gets that class:

```liquid
{{ 'lifestyle-1' | placeholder_svg_tag: root_cls | append: '__ph' }}
```

✅

```liquid
{% capture ph_cls %}{{ root_cls }}__ph{% endcapture %}
{{ 'lifestyle-1' | placeholder_svg_tag: ph_cls }}
```

Never chain another filter after an HTML-producing filter (`placeholder_svg_tag`,
`placeholder_img_tag`, `img_tag`, `script_tag`, `stylesheet_tag`, `link_to`, and similar): the next
filter operates on the generated markup, as in the ❌ example. [self-check.md](self-check.md) flags it.

### `image_picker` values are keys

An `image_picker` value is a string key, not an image object; reading `alt` / width / height from it
directly gives empty values with no error. Look up the metadata in `images` first:

```liquid
{% assign img = images[block.settings.cover] %}
<ljs-img src="{{ block.settings.cover | img_url: '800x' }}"
     alt="{{ img.alt }}" width="{{ img.width }}" height="{{ img.height }}">
```

### Images use `ljs-img`; media areas always have a placeholder

Output every image on the page (main images, video-cover fallbacks) with the `ljs-img` component,
never a native `<img>`: the native tag has no lazy loading, no container-width sizing and no
empty-image placeholder, so the page jumps as images load. Read [ljs/ljs-img.md](ljs/ljs-img.md)
before writing any image, whatever the card's main component is.

When an image / video is `blank`, never output nothing or an empty `src`; the placeholder needs a
fixed height or aspect-ratio. Skeletons are in the component docs ([ljs/ljs-img.md](ljs/ljs-img.md),
[ljs/ljs-video.md](ljs/ljs-video.md)).

The card's own icons are the exception — see
[Icons](#icons-inline-svg-by-default-image_picker-to-override).

### Placeholder images

When a slot must be filled but no real image is available (the product is a placeholder, content isn't
configured yet), use the platform's built-in placeholder images. Never leave `src` empty and never
make up an external URL.

Pick the key by slot; any string outside these four groups is not recognized:

| Slot | Key | Ratio |
|---|---|---|
| Product image | `product-1`; for several different images on one screen continue up to `product-6` | 1:1 |
| Collection image | `collection-1`; likewise up to `collection-6` | 1:1 |
| Wide slots such as full-width bars and banners | `lifestyle-1`, `lifestyle-2` | about 2.48:1 |
| None of the above fits | `image` | 1:1 |

If the ratio doesn't match the slot, the placeholder is cropped or leaves a large blank area. Don't
extrapolate a pattern such as `product-7` or `banner-1` — a wrong key doesn't error; the page shows a
collapsed blank.

Three forms; choose by how many times the image appears in one card:

| Form | Use for |
|---|---|
| `{{ 'image' \| placeholder_svg_tag: ph_cls }}` | The slot appears once. Inlines an `<svg>` with no extra request; the argument becomes its class as-is, and `fill` follows CSS |
| `{{ 'product-1' \| placeholder_img_tag: ph_cls }}` | The same image repeats in the card, such as one per list row. Outputs an `<img>` served from the image host, same argument. The inline SVG isn't small (about 13 KB for `product-1`, 89 KB for `collection-1`), and repeating it N times costs N times that |
| `{% assign ph = 'image' \| placeholder_svg %}` | Keeps downstream code branch-free: `{% assign img = product.image \| default: ph %}` gives an image object (`src` `width` `height` `alt`) |

Neither `_tag` output has width / height — only the class you pass. Use the card's own class for
`ph_cls`, `capture` it first as in [Build strings with `capture`](#build-strings-with-capture)
(`{% capture ph_cls %}{{ root_cls }}__img{% endcapture %}`), and give it `width:100%; height:100%`.
Don't pass theme utility classes such as `w-full h-full` — the card doesn't have that CSS. The slot
itself needs a definite height or aspect-ratio first, or it collapses to zero height.

### Icons: inline SVG by default, `image_picker` to override

Small icons, such as option icons or icons the merchant may replace, are written as inline `<svg>`.
Don't pull in the theme's own icon snippets or icon components, and don't use icon fonts.

To let the merchant replace an icon, add an `image_picker` ([schema-rules.md](schema-rules.md) →
"Choosing a type", the icon-slot row; put the icon size in `info`). This setting has no `default`,
and `presets[0].settings` gives it an empty string — the inline SVG draws the icon; the
`image_picker` only lets the merchant override it:

```liquid
{% if block.settings.icon != blank %}
  <ljs-img src="{{ block.settings.icon | img_url: '48x' }}" layout="responsive"
    width="1" height="1" object-fit="contain" alt=""></ljs-img>
{% else %}
  <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor"
    stroke-width="2" aria-hidden="true"><path d="M3 12h18"></path></svg>
{% endif %}
```


Without the `{% else %}` branch the slot is empty whenever the merchant hasn't picked an image — never
write only the `{% if %}`.

Functional icons (expand arrow, close ×, plus / minus, play) are inline only, with no setting: their
meaning is fixed by the interaction, and once replaced even the merchant can't recognize the button.

Follow the skeleton, plus four rules:

- The drawing occupies about 20 units of the `viewBox`, with about 2 units of margin on each side, so
  icons placed side by side come out the same size.
- Single color follows the parent `color`: outline icons use the skeleton's `stroke` setup; solid
  icons switch to `fill="currentColor"` and drop the stroke. Use only one style per card, and never
  hard-code a color on the `<svg>` (see [Paired text and background colors](#paired-text-and-background-colors))
  — mixing the two makes icons in one row differ in weight and shade.
- Leave sizing to CSS: `width: 1em; height: 1em` to follow the font size, or explicit px per location.
- When the icon itself is a button, put `aria-label` on the button element.

When the same icon appears several times, store it once with
`{% capture icon %}<svg …></svg>{% endcapture %}` and output `{{ icon }}` wherever needed; the output
is not escaped.

A bare `<svg>` in the `ljs-video` play slot fails the whole card's render (the page outputs
`<!-- card … render error -->`); `ljs-carousel` arrows carry `pre` / `next`; `ljs-quantity`
plus / minus carry `role="decrease"` / `role="increase"`. Write these three per their component docs
([ljs/ljs-video.md](ljs/ljs-video.md), [ljs/ljs-carousel.md](ljs/ljs-carousel.md),
[ljs/ljs-quantity.md](ljs/ljs-quantity.md)).

Don't hand-draw brand logos, multicolor illustrations or gradient graphics; use `image_picker` +
`ljs-img` in that spot
([Images use `ljs-img`](#images-use-ljs-img-media-areas-always-have-a-placeholder)).

### Paired text and background colors

Buttons, active tabs, badges and chips: background and text use a paired set of color settings. Never
write `background: currentColor`, and never use the same variable for background and text — you get
white blocks and invisible text. `currentColor` is only for SVG `fill` / `stroke`.

The same applies to text over the card's background (gradients included): the text color's `default`
must work on the background color's `default` — light text on dark, dark text on light. Two defaults
that each look fine but are unreadable together are the first thing a merchant sees on adding the
card.

### Cards are self-contained

Write the full markup yourself. Never `include` / `render` the theme's own snippets (its product or
product-card snippets, and the like). Snippet names and implementations differ per theme, some themes
don't have the snippet at all, and on another theme the output becomes `not support` or a blank area.
Product cards are no exception — the skeleton is in [kinds/product-card.md](kinds/product-card.md);
follow it instead of calling the theme's implementation.

### Never read the theme's global settings

`settings.*` is the current theme's global configuration, and a card reads none of it. Field ids and
enum values differ completely between themes (`settings.color_primary` is the primary color in one
theme and doesn't exist in another), so after a theme switch the value is empty, the falsy branch
runs, and that whole area disappears with no error.

Declare every color, font size and toggle the card needs as a setting in its own `{% schema %}` for
the merchant to configure (which type: [schema-rules.md](schema-rules.md) → "Choosing a type").

### Web components and JS

| Case | Rule |
|---|---|
| Static, display-only card | No custom elements, no JS, no `customElements` |
| Card with interaction | Only `ljs-*` components whose docs (`ljs/ljs-<name>.md`) you have read this round (see [ljs/writing.md](ljs/writing.md)). Don't write custom elements you haven't read docs for, and don't register `customElements` yourself. When an interaction can be declarative, don't hand-write JS |

### Copy and i18n

A card is a single synchronous `.liquid` file, and visible copy has only two homes — never the
language packs:

- Control copy (`label` / `info` / `options[].label` / `cname` / `category`) lives in
  `{% schema %}` as bilingual objects — see [schema-rules.md](schema-rules.md) → "Setting skeleton"
  and "Full field reference".
- Merchant-editable copy is output as `{{ block.settings.xxx }}`, with a sample value written into
  `default`, never left empty. Which language the samples use: [schema-rules.md](schema-rules.md) →
  "Defaults live in two places".
- Output `textarea` values through `| newline_to_br`: line breaks the merchant typed in a multi-line
  box don't become lines in HTML, and without the filter the text runs together on one line.
- Never use `| t` and never edit `locales/*.json`. Storefront translation is handled by the platform's
  multi-language feature, and the theme outputs values as stored. Theme language packs and locale
  fallbacks for empty settings are outside a card's scope; a request for those is not a card edit.

### Rich-text HTML images: `spz_img`

When outputting a whole chunk of HTML the merchant wrote in a rich-text editor (a `richtext` setting
value, `product.description`, `article.content`), pipe it through `| spz_img`. The template can't
rewrite each native `<img>` in that HTML, so the filter converts them in bulk into `<spz-img>` (a
sibling tag of `ljs-img`), which gives lazy loading and intrinsic-size placeholders. Images on the
platform CDN automatically get `auto-fit` and are cropped to the container's actual width at runtime.

```liquid
<div class="rte-{{ block_id }}">{{ block.settings.content | spz_img }}</div>
```

- To add attributes to every converted image, use the argument:
  `{{ product.description | spz_img: 'class="desc-img"' }}` — the argument is inserted into the
  `<spz-img>` tag as-is.
- Only for whole rich-text HTML; image markup you write yourself still follows
  [Images use `ljs-img`](#images-use-ljs-img-media-areas-always-have-a-placeholder).

### Content width and background come from the block's own CSS

`content_width` is this card's own setting (the fixed-id row in [schema-rules.md](schema-rules.md) →
"Choosing a type"), and the block renders it as `max-width` itself — the hosting section outputs no
CSS at all. Pair `max-width` with `margin-inline: auto` to center it; width without auto margins
leaves the narrowed card stuck to the left. A declared but unrendered `content_width` means the
merchant changes the width in the panel and nothing happens.

The skeleton has two layers with separate jobs: the root container fills the available width and
carries the card background (solid, gradient, background image); the inner wrapper only carries
`max-width` and centering. A background on the inner layer shrinks with the width limit, and the
theme's base color shows on both sides of the card.

A card that must bleed full-width doesn't declare `content_width`; drop the inner wrapper and put the
content directly in the root container — Liquid reading an undeclared id gets an empty value
([schema-rules.md](schema-rules.md) → "Every setting needs a reason").

Write mobile first; override with desktop values inside `min-width: 960px`:

```liquid
{% capture block_id %}{{ section.id }}-{{ block.id }}{% endcapture %}
{% assign block_id = block_id | replace: '.', '_' %}
{% assign root_cls = "ai-block-" | append: block_id %}
<div class="{{ root_cls }}"
  {{ block.shoplaza_attributes }}>
  <div class="block-inner"></div>
</div>

<style>
  .{{ root_cls }} {
    background: {{ block.settings.bg_color | default: 'transparent' }};
  }
  .{{ root_cls }} .block-inner {
    --block-content-width: {{ block.settings.content_width | default: 1200 }}px;
    max-width: var(--block-content-width);
    margin-inline: auto;
  }
</style>
```

### Requirement comment at the top

Start the file, from its first line, with a `{% comment %}` that states what need this card solves.
Whoever edits the card later sees only the source, so this comment is the only record of the
requirement.

### No `-` trim markers

Write `{{ }}` for output and `{% %}` for tags; never `{{- -}}` / `{%- -%}`
([self-check.md](self-check.md) rejects them). Output delimiters are only `{{` and `}}`: in
`{{- title -}}` the leading `-` is read as part of the variable name, which reads empty, and that spot
renders blank with no error.
