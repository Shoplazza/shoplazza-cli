# themes resource binding — fill product / collection / blog / article / page / menu fields with real objects

Resource fields are card fields whose control `type` (in the field schema) is `product`,
`collection`, `blog`, `article`, `page`, `link_list`, `url` or `source`. Their value is the full
data of a real store object — id, title, link, image — not a bare id, and never invented. So:
find the object, then map it onto the keys the card uses. Write the result with `+edit`
([card-edit.md](card-edit.md)): `replace_props` for a section field, `update_slot` for a block
field (menus often sit on header or footer blocks).

## Flow

1. **Find candidates** with the command for the field type (table below), passing the name the
   user gave as the filter.
2. **Branch on the count**: 2 or more → list them (title plus a telling detail such as handle or
   price) and ask which one; exactly 1 → say which object you're using and go on; 0 → say nothing
   matched and ask for another name (`source`: see below).
3. **Get the full object**: take it from the list result by id, or `get` it by id. An id alone
   (from earlier output or the user) only locates the object — look it up.
4. **Map**: read the card's current value
   (`+page -t <theme_id> --template <t> --session <oseid> --section <sid>`) and fill the same
   key set, following the field mapping below.

Check that filtered hits really match the name before choosing (shoplazza-common → Reading a
filtered list).

## Where to find each type

| Field `type` | Object | Command | Filter by name |
|---|---|---|---|
| `product` | Product | `products +search --keyword <name>` (shoplazza-products) | `--keyword` (title); `--collection-id <id>` for one collection's products |
| `collection` | Product collection | `products collections list --params '{"title":"<name>"}'` · `products collections get --params '{"id":"<id>"}'` | `title` (partial match) |
| `blog` | Blog (the blog itself, not its articles) | `shop blogs list --params '{"keyword":"<name>"}'` (shoplazza-shop) | `keyword` |
| `article` | One blog article | `shop articles list --params '{"keyword":"<name>"}'` · `shop articles get --params '{"id":"<id>"}'` | `keyword` (title / content) |
| `page` | Custom page | `shop pages list --params '{"page_size":100}'` (shoplazza-shop) · by URL: `shop pages search --params '{"url":"<url>"}'` | None — match `title` yourself |
| `link_list` | Menu | `shop menu list` → `.data.menus[]` `{id, title, children}` | None — match `title` yourself |
| `source` | Ongoing flash sale | `discounts +search --progress ongoing --discount-type flashsale` (shoplazza-discounts) | `--query <name>` |
| `url` | Depends | Ask which kind of resource to link, then use that row | — |

For products, use the default `+search` output: it carries `id`, `title`, `handle`, `url`,
`price_min`, `price_max`, `published`, `inventory_quantity`, `primary_image`. Don't narrow it with
`--fields` — `primary_image` does not come back through it. Keys that are empty or false are left
out of responses.

## Field mapping

Lookup results and card values don't share one key set. The link key matters most: the card uses
it as the click target for buttons and images, and without it nothing is clickable.

| Field `type` | `type` key in the value | Link key | Other keys |
|---|---|---|---|
| `product` | As the current value has it (`"product"` on some themes, absent on others) | The current value's link key (`url`, or `seo_url` on some themes) ← `+search` `url` (`/products/<handle>`); empty → build `/products/<handle>` | `id`, `title`, `price_min`, `price_max`, `published`, `inventory_quantity`, `spu` ← same-name keys; `image` ← `primary_image` |
| `collection` | `"collection"` | `url` = `/collections/<handle>` (lookups return no link) | `id`, `title`; `image` ← `image`, or `[]`; `products_count` only if the card has it |
| `blog` | `"blog"` | `url` = `/blogs/<handle>` | `id`, `title` |
| `article` | As the current value has it | `url`: lookups return no link — follow the pattern of an article URL already in the card | `id`, `title`; `image` ← `image.src` |
| `page` | `"page"` | `url` ← the page's `url` from `shop pages list` / `get` (e.g. `/pages/<handle>`) | `id`, `title` |
| `link_list` | `"menu"` | `url`: menus have no page URL — keep what the current value has (usually `""`) | `id`, `title` ← `shop menu list` |
| `source` | — | — | See "source fields" below |

- Build paths from the raw `handle`, without URL-encoding it.
- Write only the keys the card's value uses. `handle` and other lookup-only keys stay out —
  pasting the raw lookup result into the field can break the card's rendering.
- A key the lookup doesn't return stays empty; never fill it from another object.

## Empty collections

Before binding a collection to a card that shows its products, check it has products:
`products +search --collection-id <collection_id> --page-limit 1`. No products → tell the user
"this collection has no products; the card will show an empty block", and write only after they
confirm.

## source fields

A `source`-type field binds an ongoing flash-sale activity. Candidates are **only flash sales that
are ongoing** — other discount types don't show on the card, and not-started or finished sales
don't display. Keep both conditions. Id and name: shoplazza-discounts → the ongoing-flash-sale
recipe (`discount_info.id`, `discount_info.discount_name`).

- Fill the field with the same shape as its current value (same keys); a key the lookup can't
  supply (such as a cover image) stays empty.
- No current value to copy the shape from → don't guess a shape: give the field's `label` and ask
  the merchant to pick the activity in the theme editor's card settings.
- 0 results → explain that the card shows only ongoing flash sales; suggest starting one or
  binding a different one. Don't retry with keyword variations.

## `page` field vs page-template binding

Both use custom pages from `shop pages list`, but they are written differently:

| | `page` field (here) | Custom template binding |
|---|---|---|
| Written to | The card's `settings` value | The template's `relations` |
| Value | Full object `{id, type:"page", url, title}` | Array of id strings `["<page_id>"]` |
| Command | `themes +edit` ops | `themes template create` / `update` ([page-template.md](page-template.md)) |

## Rules

1. `id`, `url`, `title`, `image` all come from real lookup results — never guess a URL.
2. A bare id or `{"id":"…"}` in `settings` doesn't render the resource: always write the full
   object.
3. Typing the object's name into a title field is not binding: the card still points elsewhere.
