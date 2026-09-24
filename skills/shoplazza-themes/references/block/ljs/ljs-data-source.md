# ljs-data-source — product data for child components

Fetches product-type data in the browser and hands it to the components inside it (the purchase
form and variant picker of [product-single.md](../kinds/product-single.md)). Public reference:
lessjs `spz-data-source` (plugin components use the `ljs-` tag prefix; everything else is the same).

## Rules

1. Only reference a data source you declared in this block, by the `id` you gave it. Never point
   at a data source defined by the theme.
2. Give it an `id` built from `block_id` (and the product id when a card repeats per product), so
   two copies of the card never collide.
3. For one product: `source-type="product"` + `source-id="{{ p.id }}"`. When the product comes
   from an unset setting (placeholder product), it has no real id — render the placeholder card
   instead of the data-source chain.
4. Children read it through their own data-entry attribute — the variant picker uses
   `src="custom:<data-source id>.getData"` (see [ljs-variants](ljs-variants.md)).
5. Don't use it to page through a collection — product lists use
   [ljs-list](ljs-list.md) (see [product-card-async.md](../kinds/product-card-async.md)).

## Attributes

| Attribute | Meaning | Required / default |
|---|---|---|
| `source-type` | `product`, `product-list`, `collection`, `recently-viewed`, `recommendation`, `random` | default `product` |
| `source-id` | Product id, collection id, or comma-separated product ids | depends on type |
| `items` | Path to the array inside the response, e.g. `data.list` | optional |
| `product-data` | Product JSON supplied directly; no request is made | optional |
| `page-size` | Records per request | default `40` |
| `init-page` | First page number requested | default `0` |
| `index` | For a nested data source: take one item of the parent's data by index | optional |

## Skeleton

```liquid
{% capture ds_id %}pds-{{ block_id }}-{{ p.id }}{% endcapture %}
<ljs-data-source id="{{ ds_id }}" layout="container" source-type="product" source-id="{{ p.id }}">
  {% comment %} ljs-product-form / ljs-variants go here and read custom:{{ ds_id }}.getData {% endcomment %}
</ljs-data-source>
```
