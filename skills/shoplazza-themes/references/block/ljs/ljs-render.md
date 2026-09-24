# ljs-render — client-side render

Fetches data and renders it once into static markup. No paging, no load-more.

## Rules

1. Use `ljs-render`. No hand-built DOM strings, no hand-written fetch + `innerHTML`.
2. Give it a stable `id` when events call it.
3. Data sources for `src`: an API URL; `script:<id>` (a `<script type="application/json">` on the
   page); `spz-script:<script_id>.<function>` (a function exported by an
   [ljs-script](ljs-script.md)); `custom:<id>.<method>` (another component's data, e.g.
   `custom:<state_id>.getState`, see [ljs-state](ljs-state.md)). Join several sources with `;`; they reach the template's
   preprocessing function in that order (see the `data-function` section of
   [template.md](template.md)).
4. The template is an inline `<template>`; follow [template.md](template.md) for single root,
   `${}` expression limits, loops, and conditions.
5. Automatic render: don't write `manual`. Event-driven data: write `manual`, then call `render` /
   `rerender` from outside.
6. `render` takes only `src=` (switch source and refetch) and `redo=`; it does not take `data=`.
   To replace the data use `rerender(data=event)`. Pass `redo=true` only to force a full redo.
7. To drive something after rendering, bind `@finish`; it fires once the DOM is really rendered.


## What it can do

| Capability | How | Check |
|---|---|---|
| Fetch and render automatically | `src` + template | DOM appears once data is ready |
| API source | `src="/api/..."` | the real response reaches the template |
| Page JSON source | `src="script:data-id"` | JSON changes show in the UI |
| Script source | `src="spz-script:script-id.getData"` | exported function's result reaches the template |
| Component source | `src="custom:id.getState"` | reads another component's data |
| Several sources | `src="a;b;c"` | passed in order to the preprocessing function |
| Inline template | child `<template>` | good for small templates |
| Manual first render | `manual` + `render(src=...)` | nothing renders until called |
| Re-render with new data | `rerender(data=...)` | new data replaces the old |
| Render only once | `rerender(once=true)` | later calls don't re-render; `afterOnceFinish` fires instead |
| Chain after render | `@finish="target.action(...)"` | drives the next step after the DOM is done |

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `src` | Data source | yes, unless `manual` and data comes from `render`/`rerender` | URL / `script:` / `spz-script:` / `custom:`; `;` for several |
| `template` | Id of a template elsewhere on the page | no | for a larger shared template |
| `manual` | No automatic first render | no | boolean, no value |
| `id` | Instance id | no | for actions / events |

Actions: `render(src=, redo=)` / `rerender(data=, redo=, once=)`.
Events: `finish` / `afterOnceFinish` (fires instead of re-rendering when `rerender` is called with
`once=true` after the first render) / `error` (the data request failed).

Use only the attributes in the table; don't write `animate`, `duration`, `items`, or `as`.

## Skeleton (automatic, page JSON + inline template)

```liquid
{% capture render_id %}render-{{ block_id }}{% endcapture %}
{% capture data_id %}render-data-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <script type="application/json" id="{{ data_id }}">
    {
      "name": {{ block.settings.title | json }},
      "brief": {{ block.settings.brief | json }},
      "imageSrc": {% if block.settings.image != blank %}{{ block.settings.image | img_url | json }}{% else %}""{% endif %}
    }
  </script>

  <ljs-render id="{{ render_id }}" layout="container" src="script:{{ data_id }}">
    <template>
      <div class="intro">
        <ljs-img layout="fixed" width="80" height="80" src="${data.imageSrc}" alt="${data.name}" object-fit="cover"></ljs-img>
        <p>${data.name}</p>
        <div>${data.brief}</div>
      </div>
    </template>
  </ljs-render>
</div>
```

### Manual re-render and chaining on finish

```liquid
{% capture render_id %}render-{{ block_id }}{% endcapture %}
{% capture status_id %}render-status-{{ block_id }}{% endcapture %}

<button type="button" @tap="{{ render_id }}.rerender(data=event)">{{ block.settings.button_text }}</button>
<ljs-render
  id="{{ render_id }}"
  layout="container"
  manual
  @finish="{{ status_id }}.toggleClass(class='is-done', force=true)"
>
  <template><div>${data.value}</div></template>
</ljs-render>
<div id="{{ status_id }}" class="render-status"></div>
```
