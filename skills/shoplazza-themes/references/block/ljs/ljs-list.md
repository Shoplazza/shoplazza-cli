# ljs-list — async lists with paging and load more

Runtime data returned by an API or a script, rendered through a `${}` template, with paging /
load more / infinite scroll. lessjs reference:
[spz-list](https://lessjs.shoplazza.com/latest/components/spz-list/) (write the tag as `ljs-list`).

> Scope: when Liquid already has the data (`block.settings` / blocks / `collection.products`),
> always render it statically with `{% for %}` and don't choose this component. Use it only when
> the request explicitly needs an async list from an API, load more, or paging.

## Rules

1. Data Liquid can reach at render time (blocks / `block.settings` / `collection.products`) is
   rendered with `{% for %}`; wrapping it in this component only adds a request and a blank first
   screen. Only data from a runtime API or an `ljs-script` uses it. Exception: load-more for a
   collection's products, see [product-card-async.md](../kinds/product-card-async.md).
2. The item template is a `<template>` that is a direct child of `ljs-list`; missing or misplaced,
   the page renders an empty shell. Item interpolation uses only `${data.xxx}` — `{{ item.title }}`
   is evaluated to an empty string at Liquid render time, so runtime data never gets in. Template
   rules (`${}` limits, loops, the split with Liquid) follow [template.md](template.md).
3. Data enters only through `src`: an HTTP URL, or `spz-script:<script_id>.<fnName>` pointing at an
   `ljs-script` on the same page (the function exported with `exportFunction`). The protocol stays
   `spz-script:` even though the tag is `ljs-script` — only tag names take the `ljs-` prefix. Never
   hand-write `fetch` + `innerHTML`.
4. `list` / `total` are field paths in the returned JSON, not the data itself: `list="list"`
   `total="count"`; nested: `list="data.list"`.
5. `initial-page` is required: `0` or `1`, whichever page number the data source starts at.
6. Layout via `type`: `row` (horizontal scroll) / `column` / `grid` (with `column-count`) /
   `masonry`. `column-count`, `initial-page`, `list`, `total`, `src`, `page-size` are value
   attributes and must carry a value; switches such as `infinite-scroll` work the other way — when
   off, omit the attribute entirely, never `="false"`.
7. Don't fight the component's CSS: the list element's own `display`, `flex-direction` and
   `overflow` are set by the component for the chosen `type`; a second set of your own clashes
   (horizontal scrolling breaks or double scrollbars appear). Column width, gaps and padding go on
   `.list-item`.
8. Images / amounts inside the template keep using documented components: `ljs-img`
   (`src="${data.image.src}"` + `width` / `height`), `ljs-currency` (`value="${data.price}"`).
9. Derive linked ids from `block_id` with `capture` (the `ljs-script` id too): an empty id gets no
   data, and a hard-coded global id makes two cards on the same page share data.
10. `ljs-pagination` has no doc here, so don't use it ([writing.md](writing.md) R5); page numbers
    depend on the theme's own pagination.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `src` | Data source | Yes | HTTP URL or `spz-script:<id>.<fn>` |
| `initial-page` | Page number the requests start at | Yes | `0` or `1` |
| `list` | Path of the item array in the response | No | Default `data.list`; e.g. `list` |
| `total` | Path of the total count in the response | No | Default `total`; e.g. `count` |
| `type` | `row` / `column` / `grid` / `masonry` | No | Value attribute |
| `column-count` | Grid columns | Required with `type="grid"` | Value attribute |
| `page-size` | Items per page requested | No | A number; default `1000` |
| `page` | Query-parameter name for the page number | No | Default `page` |
| `size` | Query-parameter name for the page size | No | Default `limit` |
| `initial-total` | Total item count known up front | No | Default `0` |
| `infinite-scroll` | Load the next page on scroll | No | Boolean |
| `use-loadmore` | Load-more mode | No | Boolean; accepts media-query syntax to switch per breakpoint |
| `manual` | Don't render / request on mount | No | Boolean |
| `<template>` child | Item template | Yes | `${}` interpolation |

Load more on click: a child element with the valueless `loadmore` attribute becomes the load-more
button and switches the list to load-more mode.

The component sets these attributes on itself — use them in CSS (e.g. hide the button with
`ljs-list[nomore] [loadmore] { display: none; }`): `loading` (request in flight), `finish` (data
arrived), `hasmore` (more pages remain), `nomore` (everything loaded), `data-empty` (no items).

`inherit-url-search`, `maintain-page-state`, `record-params-page-plus-one`,
`same-queries-in-array`, `same-queries-in-string` and `track-event-name` also exist; they serve
full collection pages that sync paging with the URL — a card doesn't need them.

## Actions & events

| Kind | Name | Notes |
|---|---|---|
| Action | `refresh` | Re-request. Optional arguments override request parameters (`refresh(page=1,limit=8,redo=true)`); `redo=true` starts over without keeping earlier pages. `@tap="{{ list_id }}.refresh()"` |
| Action | `listRender` | Re-render from passed data (`data`) |
| Action | `scrollIntoTop` | Scroll the page to the top of the list |
| Action | `resetGridStyle` | Recompute the grid layout of the items |
| Event | `finish` | The first render on mount finished |
| Event | `dom-update` | A re-render finished (including load more / page change) — use this to react to new items |
| Event | `beforeFetchStart` | Right before a request starts |
| Event | `error` | A request failed |

Event data: `data` (the list items shown) and `total`.

## Skeleton

```liquid
{% capture list_id %}list-{{ block_id }}{% endcapture %}
{% capture data_id %}list-data-{{ block_id }}{% endcapture %}

<div id="{{ root_cls }}" class="{{ root_cls }}" {{ block.shoplaza_attributes }}
  style="--list-gap: {{ block.settings.gap | default: 16 }}px;">
  {% if block.settings.title != blank %}
    <h2 class="list-title">{{ block.settings.title }}</h2>
  {% endif %}

  <ljs-list
    id="{{ list_id }}"
    layout="container"
    type="grid"
    column-count="{{ block.settings.column_count | default: 2 }}"
    initial-page="1"
    list="list"
    total="count"
    src="spz-script:{{ data_id }}.getListData"
  >
    <template>
      <div class="list-item">
        <ljs-img
          layout="responsive"
          width="${data.image.width}"
          height="${data.image.height}"
          src="${data.image.src}"
          alt="${data.title}"
        ></ljs-img>
        <div class="list-item-info">
          <h3>${data.title}</h3>
          <ljs-currency layout="container" value="${data.price}"></ljs-currency>
        </div>
      </div>
    </template>
  </ljs-list>
</div>

<ljs-script layout="logic" id="{{ data_id }}" scope="{{ root_cls }}">
  function getListData() {
    return fetch('{{ block.settings.data_url }}')
      .then(function (res) { return res.json(); });
  }
  exportFunction('getListData', getListData);
</ljs-script>

<style>
  .{{ root_cls }} .list-item { padding: var(--list-gap); }
  .{{ root_cls }} .list-item-info { padding: 8px 0 0; }
  .{{ root_cls }} .list-item h3 { margin: 0; font-size: 16px; }
</style>
```

The script's request follows the rules in [ljs-script.md](ljs-script.md) (same-origin only).

Calling the HTTP endpoint directly: set `src` to the endpoint and drop the `ljs-script`:

```liquid
<ljs-list id="{{ list_id }}" layout="container" type="row" initial-page="1"
  list="data.list" total="data.total" src="{{ block.settings.data_url }}">
  <template>…</template>
</ljs-list>
```

`${}` is a restricted expression: property paths, ternaries and array methods work;
`Object.keys(...)` / `JSON.stringify(...)` make the whole node disappear silently (no error). Wrap
every amount in `ljs-currency`; never assemble a currency symbol inside `${}`.

Load-more for a product list: see [product-card-async.md](../kinds/product-card-async.md).
