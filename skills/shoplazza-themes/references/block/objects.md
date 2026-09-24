# themes block objects — data a card can read

How a card follows what the merchant picked in the editor (product, collection, page, menu, blog,
article, image, video) to data it can render; what to fall back to when nothing is picked; which
fields the resulting product / collection objects have, and how to decide what to show for prices.

Not in this file:
- What a product-list card looks like, which component it uses, list layouts →
  [kinds/product-card.md](kinds/product-card.md); single-product cards →
  [kinds/product-single.md](kinds/product-single.md)
- Which setting type declares each of these → [schema-rules.md](schema-rules.md) → "Settings"

Use only the fields listed in the tables below. Fields outside them aren't necessarily missing, but
they read empty on some stores with no error, and that part of the card is simply blank.

## Getting real data from a setting

Resource settings fall into a few kinds by what they read as. Getting it wrong never errors — that
part of the page is just blank:

| Setting type | What Liquid reads | How to get the data |
|---|---|---|
| `url` | A link object (`.url` `.type` `.title`), not a string | Take the address from `.url`: `href="{{ block.settings.my_link.url }}"`; check `block.settings.my_link.url != blank` before outputting the link |
| `product` | `.id` `.title` `.url` `.image` saved when it was picked | `{% assign p = all_products[block.settings.my_product.id] %}` |
| `collection` | `.id` `.title` `.url` `.image` saved when it was picked | `{% assign col = collections[block.settings.my_collection.id] %}` |
| `page` | `.id` `.title` `.url` saved when it was picked | `{% assign pg = pages[block.settings.my_page.id] %}` → only `.title` `.content` are usable |
| `link_list` | `.id` `.title` `.url` saved when it was picked | `{% for link in linklists[block.settings.my_menu.id].links %}` → each item has `.title` `.url` `.type` |
| `blog` | `.id` `.title` `.url` saved when it was picked | Its article list can't be reached; see below |
| `article` | `.id` `.title` `.url` `.image` saved when it was picked | Excerpt / date / author go through `articles[handle]`; see [Articles](#articles) |
| `image_picker` | A string key, not an image object | URL via `\| img_url`; width, height and alt via `images[key]` ([liquid-rules.md](liquid-rules.md) → "`image_picker` values are keys") |
| `video_picker` | An object — **an empty check is always true** | Loop over `.sources` for mp4 / hls; empty-check pattern in [ljs/ljs-video.md](ljs/ljs-video.md) |

The first six rows are one rule: **the setting stores only a few fields recorded at the moment the
merchant picked, not the object.**

- The saved `.title` `.url` `.image` are the values at that time; when the product / collection is
  later renamed or gets a new image they don't follow. Use them only for empty checks, and always
  display from the real object obtained in the right-hand column.
- Reading `block.settings.xxx.products` / `.price` / `.content` / `.links` directly gives an empty list
  or empty string: something is clearly picked in the editor, yet the storefront area is blank with no
  error.

A card can't list the articles under a `blog`. When asked for a "blog article list" card, say it
can't be done; don't write `blog.articles` (it exists only on the blog listing page, and on any other
page the card is empty).

## Articles

An `article` setting only fills in `.id` `.url` `.title`. Every field other than the title goes
through `articles[handle]`, and the handle isn't in the setting — split it off the end of the url:

```liquid
{% assign picked = block.settings.my_article %}
{% if picked.id and picked.id != 0 %}
  {% assign handle = picked.url | split: '/' | last %}
  {% assign post = articles[handle] %}
{% endif %}
```

Check `picked.id and picked.id != 0`: when nothing is picked `id` may be `0` rather than empty, so a
plain non-empty check enters the fetch branch and `articles['']` returns an empty object.

`articles[handle]` has a performance cost; use it sparingly — at most three to five articles per
card.

Usable fields on the resulting `post`:

| Field | Notes |
|---|---|
| `title` `url` `handle` `id` | Safe |
| `image` | Cover; sub-fields `.src` `.width` `.height` `.alt`, same as `product.image` |
| `excerpt` `content` | Excerpt and body; `excerpt_or_content` means "the excerpt if there is one, otherwise the body" — use it in list cards |
| `author` `published_at` `created_at` `tags` | Safe; format dates with `\| date:` |
| `comments_count` `comments_enabled` `comments` `comment_post_url` | Comment fields; cards usually don't need them |
| `blogs` | The blogs the article belongs to, an array; use `[0].title` as the category name |

Articles have no placeholder object. If one can't be fetched, don't render that block at all; don't
make up sample copy.

## Placeholder objects when nothing is picked

`default_product` / `default_collection` are always available, independent of the theme. They let a
card show a reasonable preview before the merchant configures any data.

`default_product` field by field — render them directly and the preview is complete, with no need to
invent sample data:

| Field | Placeholder value |
|---|---|
| `title` | `Product Title` |
| `price` / `price_min` / `price_max` | `88.88`, `price_varies=false` (the price-range check is always false) |
| `compare_at_price` | `99.99` — higher than `price`, so the placeholder's strike-through price and discount badge **do** show |
| `off_ratio` | `50`, which doesn't match the 11% that 88.88 / 99.99 actually give — this is why the placeholder's discount badge and strike-through price look contradictory; the card isn't miscalculating |
| `image` | A 150×150 svg, `aspect_ratio=1`, empty `alt`; `images.size=5` |
| `url` | **Empty string** — the only field without a usable placeholder value |
| `isMock` / `available` | `true` / `true` |
| `variants.size` / `has_only_default_variant` | `6` / `false` (the placeholder always counts as multi-variant) |
| `brief` | `brief title` |


`default_collection` has `title="Default Collection Name"`, an empty `url`, and `products.size=50`;
the 50 products are copies of the same placeholder product — that's why a preview shows 50 identical
cards, not a data error.

Pages and menus have no placeholder object. If one can't be fetched, don't render that block at all;
don't output an empty shell or placeholder copy.

## Fallback chain (fixed patterns)

Single product:

```liquid
{% assign product = all_products[block.settings.my_product.id] %}
{% unless product.id %}
  {% assign product = default_product %}
{% endunless %}
```

Check `product.id`, not `product`: `all_products[<nonexistent id>]` returns an empty object, not `nil`,
so `{% unless product %}` is always false and the fallback never kicks in.

A collection (list cards) falls back as a whole collection, checked by `products.size`:

```liquid
{% assign col = collections[block.settings.my_collection.id] %}
{% assign real_count = col.products.size %}
{% unless real_count > 0 %}
  {% assign col = default_collection %}
  {% assign real_count = col.products.size %}
{% endunless %}

{% assign show_count = block.settings.product_limit %}
{% if real_count < show_count %}
  {% assign show_count = real_count %}
{% endif %}

{% for p in col.products limit: show_count %}
```

All three are hard requirements:

1. **Fall back the whole collection**; don't fall back to `default_product` item by item inside the
   loop — that needs the loop count computed first and elements picked by index, an extra layer of
   indirection, and you'd have to make up the placeholder count yourself.
2. **Check `products.size`, not `col.id`.** It catches an empty collection and an unfetchable one
   together. More importantly, `col.id` is not a reliable signal: checking it can treat a collection
   that did load as missing, and the merchant sees placeholder data after picking a real collection.
3. **Cap with `limit:`**; don't write `{% for i in (1..n) %}` + `col.products[forloop.index0]`. It runs,
   but adds a pointless index layer.

## Product and collection fields

`product` fields:

| Field | Notes |
|---|---|
| `id` `title` `url` `image` `images` `variants` `selected_or_first_available_variant` `options` `available` `isMock` | Safe, use freely |
| `price` `price_min` `price_max` `compare_at_price` `off_ratio` | Price fields; checks in [Prices](#prices) |
| `description` `brief` `has_only_default_variant` | Safe |
| `need_variant_image` | Whether the product has variant images; one of the preconditions for option thumbnails (`option_thumbnails`). When falsy the whole option area isn't rendered; if it can't be read it simply falls into the no-render branch |
| `handle` `vendor` `seo_url` `type` `sku` `retail_price_max` | **Don't use** — empty on some stores |
| `sold_out` | **Doesn't exist**; see below |

The platform doesn't provide `sold_out`. `product.sold_out` doesn't error and is always falsy. Check
sold-out with `product.available == false`.

Sub-structures:

| Level | Usable fields |
|---|---|
| `product.image` | `.src` `.width` `.height` `.alt` |
| `product.images[i]` | Same as above; get the second image with `{% assign img2 = product.images \| secondary_image %}`, which skips videos |
| `product.variants[i]` | `.id` `.price` `.compare_at_price` `.available` `.available_quantity` `.image` `.url` `.options` `.option1` `.option2` `.option3` |
| `product.selected_or_first_available_variant` | The same kind of object as `variants[i]`, same fields |
| `product.options[i]` | `.name` `.values` `.position` (counts from 1; hidden options are already filtered out by the platform and can't be read; the placeholder product has no `.position`) |
| `product.variants[i].options[j]` | `.name` `.value` |

`variants[0].id` is the default variant id, needed for add-to-cart.

For "which variant should show now" use `selected_or_first_available_variant`. It selects by the
URL's `?variant=`; a card sits on pages like the home page or a collection page that have no such
parameter, so it gets the first `available` variant, and falls back to `variants[0]` when the whole
product is sold out.

In other words, in a card it always equals "the first purchasable SKU" — no need to loop over
`variants` yourself.

For a stock number ("only 3 left", a quantity box's maximum) read the variant-level
`.available_quantity`. It has a performance cost, so read it only when you actually show the number;
check in-stock with `.available`.

`collection` fields are only `id` `title` `url` `description` `image` `products` `products_count`;
`image` has the same sub-fields as `product.image`. Don't write fields outside these tables for either
object.

## Prices

| Case | Check / form |
|---|---|
| Output an amount | `<ljs-currency layout="container" value="{{ product.price }}"></ljs-currency>`: pass the number straight to `value`, without `money_with_symbol`; the component renders its own `class="money"` container (the hook for currency switching), so don't wrap it by hand; in the price row give it `display: inline-block` |
| Show a strike-through price? | `{% if product.price < product.compare_at_price %}` — **compare the values, don't check for non-empty**: without a discount `compare_at_price == price` rather than empty, so a non-empty check puts a strike-through price on every product |
| Price range? | `{% if product.price_min != product.price_max %}` → show `price_min` + "from" |
| Discount percentage | `off_ratio` is already an integer (`30` = 30%); `times: 100` again makes 3000%. When it's empty compute it: `compare_at_price \| minus: price \| times: 100 \| divided_by: compare_at_price \| round` |
| Discount amount | `<ljs-currency layout="container" value="{{ product.compare_at_price \| minus: product.price }}"></ljs-currency>` |

Product-object amounts and amounts the merchant typed both go through `ljs-currency` (form in
[ljs/ljs-currency.md](ljs/ljs-currency.md)).

They differ by one step: a typed amount goes through `| market_rate_exchange` first to convert it to
the current market's currency before it's passed to `value`; product prices are already market
prices and are passed directly.

## `isMock` placeholder state

When the merchant hasn't picked a product, `default_product` comes in and `isMock` is true. When a
list card falls back to `default_collection`, each of its 50 placeholder products is also
`isMock=true` — the table below treats both paths the same.

| Element | Handling |
|---|---|
| Link | `{% unless product.isMock %}href="{{ product.url }}"{% endunless %}`; the placeholder product's `url` is empty, so writing it anyway links to a 404 |
| Add-to-cart button | In the placeholder state **don't output it at all** (not disabled — not rendered) |
| Image | Output a placeholder image: `product-1` for a product slot, `collection-1` for a collection slot; when every list row has one, use `placeholder_img_tag`. Forms in [liquid-rules.md](liquid-rules.md) → "Placeholder images" |
| Title / price / strike-through price / discount badge | Render as usual, with no `isMock` branch — placeholder values in [Placeholder objects when nothing is picked](#placeholder-objects-when-nothing-is-picked) |

Don't use `variants.size` to detect no-variant / single-variant products: the placeholder product
always has 6 variants, so the preview is always treated as multi-option.

## `shop`

Has data on every page; use it directly. Usable fields: `name` `description` `url` `locale`
`currency_code` `finance_symbol` `favicon` `cdn_domain`. `url` is the store home page address, already
carrying the URL prefix; `locale` looks like `en-US`; `currency_code` looks like `USD`;
`finance_symbol` is the current currency symbol.

Other `shop` fields may be empty on some stores; don't use them. Don't build amounts yourself from
`shop.finance_symbol` + a number either — use `ljs-currency`.
