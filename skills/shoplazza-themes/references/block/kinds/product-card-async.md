# themes product-card-async — async load-more product list (tier L2)

Read this when the list form is L2 ("load more" / load on scroll). The first page is rendered by
Liquid inside `ljs-list`; later pages come from the collection's product API and are rendered by an
inline item template — the same pattern the public Nova theme uses for collection pages.

Form selection, in-card elements, the add-to-cart area and the setting contract are in
[product-card.md](product-card.md). The `${}` template mechanics are in
[ljs/template.md](../ljs/template.md); list attributes are in [ljs-list.md](../ljs/ljs-list.md).

## Skeleton

```liquid
{% comment %} block_id and root_cls come from the file header (liquid-rules.md) {% endcomment %}
{% capture list_id %}pl-{{ block_id }}{% endcapture %}
{% assign list_src = '/api/collections/' | append: col.id | append: '/cps' | add_root_url %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-list id="{{ list_id }}" layout="container" manual
    src="{{ list_src }}?page=0&limit={{ block.settings.page_size }}"
    list="data.products" total="data.count"
    initial-page="0" initial-total="{{ col.products_count }}"
    page-size="{{ block.settings.page_size }}"
    {% if block.settings.load_mode == 'scroll' %}infinite-scroll{% endif %}
  >
    {% comment %} first page: server-rendered with the same markup as the L1 card {% endcomment %}
    {% for p in col.products limit: block.settings.page_size %}
      <div class="{{ root_cls }}__item">…</div>
    {% endfor %}

    {% comment %} later pages: interpolate only ${data.xxx}; Liquid has already run by now {% endcomment %}
    <template>
      <div class="{{ root_cls }}__item">…</div>
    </template>

    {% if block.settings.load_mode == 'click' %}
      <div loadmore class="{{ root_cls }}__loadmore">{{ block.settings.load_more_text }}</div>
    {% endif %}
  </ljs-list>
</div>
```

## Rules

1. `src` is the collection's product API, built as
   `'/api/collections/' | append: col.id | append: '/cps' | add_root_url`, plus
   `?page=0&limit=<page_size>`. Responses carry the items in `data.products` and the total in
   `data.count` — set `list="data.products"` and `total="data.count"`.
2. `manual` keeps the server-rendered first page; the list fetches only the following pages.
   `initial-page="0"` (required, `0` or `1`) and `initial-total` = the collection's product count.
3. `page-size` is a plain number and matches the `limit` in `src` and the Liquid `limit:`.
4. Load mode:
   - `scroll` → `infinite-scroll` on `ljs-list`. It is a boolean attribute: to turn it off, leave
     the whole attribute out; never write `="false"`.
   - `click` → a child element with the `loadmore` attribute. The list uses that element as its
     load button and switches to load-more paging.
5. The item template is a `<template>` that is a direct child of `ljs-list`
   ([ljs/template.md](../ljs/template.md#t1--where-the-template-lives-and-how-the-component-finds-it),
   T1); `ljs-list` has no `template` attribute. No `template-src` pointing at an external file — an
   AI card can't ship assets.
6. The first-page Liquid markup and the `${}` template markup must render the same card.
7. When no collection is picked, output no `ljs-list` at all; render only placeholder cards. A
   collection has no `isMock` field: set a flag in the fallback branch
   (`{% assign col_placeholder = true %}` beside `{% assign col = default_collection %}`) and test
   `col_placeholder`. The placeholder collection has no real id, so the API can't return data.
8. Don't add `ljs-pagination`: page numbers are out of scope for this card (the setting contract in
   [product-card.md](product-card.md) has no setting for them).

## Four things the client template can't do

| Need | Liquid version (L1 / L3) | `${}` client version (L2) |
|---|---|---|
| Money formatting | `ljs-currency`, `value="{{ p.price }}"` | Still `ljs-currency`, `value="${data.price}"`; never build currency symbols inside `${}` |
| Placeholder state | `product.isMock` branch | No `isMock` on the client → when no collection is picked, output no `ljs-list` |
| Swatch images | `option_thumbnails` filter | Not available on the client → only `variants[].image` from the response, de-duplicated yourself |
| Complex expressions | Anything Liquid allows | `${}` is a restricted expression, see below |

## `${}` is a restricted expression

Limits: [ljs/template.md](../ljs/template.md#t3---expressions-scope-and-limits) (T3); branches and
loops use the `spz-if` / `spz-for` directives (T4).

## The cost of L2

The same card's markup exists twice — the Liquid version (first page / placeholder) and the `${}`
version (later pages). Price, discount and sold-out display rules must match in both. Tell the
merchant this in your reply.
