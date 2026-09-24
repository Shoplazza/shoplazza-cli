# ljs templates — client-side `<template>` rendering

General rules for components that render DOM in the browser from a `<template>` plus data. Which
components take a template and what their data fields look like are in each component's
`ljs-<name>.md`; this file covers how to write the template itself. Public reference:
<https://lessjs.shoplazza.com/latest/docs/template/>.

## T1 · Where the template lives and how the component finds it

The component looks for its template in this order and stops at the first match:

| Declaration | How | When |
|---|---|---|
| `template="<id>"` attribute | Put `<template id="tpl-{{ block_id }}">` in the same file and reference that id from the component | **Default choice**; `ljs-render` and `ljs-product-snippet` support it |
| Direct child `<template>` | Write `<template>` as a direct child of the component; no id needed | **Required** for `ljs-list` item templates; also the control UI of `ljs-scrollbar` / `ljs-countdown` |
| `template-src` external file | — | **Never**: a block is a self-contained single file, so the template must be inline in the same file |

After rendering, the `<template>` element stays in the DOM and the rendered output is appended
after it — don't write CSS selectors that assume the template disappears.

## T2 · One root element, carrying `data-nosnippet`

Only the first root element of the template's output is rendered: with several sibling roots only
the first renders and the rest are dropped silently. To output several pieces, wrap them in one root
`<div>`.

Put the valueless `data-nosnippet` attribute on that single root. The `<template>` content stays in
the body of the HTML source; `data-nosnippet` tells search engines to keep that text out of search
snippets, so interpolation markers and placeholder copy are not indexed.

## T3 · `${}` expressions: scope and limits

`${}` is evaluated in the browser against the template data and is only for interpolation — putting
one value into text or an attribute. Conditions and loops use directive attributes (T4). It is not
full JS:

- Scope: top-level fields of the data object are variables (`${progress}`); the whole payload is
  `data` (`${data.title}`). Both forms are equivalent — follow the example in the component doc
  you use.
- Allowed: property paths, ternaries, arithmetic and string concatenation, value-level one-line
  array methods (`.map(o => o.value).join('/')` / `.find()` / `.slice()`).
- Forbidden: global-object calls such as `Object.keys(...)` / `JSON.stringify(...)` — the whole
  node is dropped silently (no error, nothing renders).
- The lessjs template docs also list these syntax limits: no comments of any kind inside a
  template; no spaces around `||`, `&&` and `?:` (`${data.name||data.handle}`); `==` / `===` /
  `!=` / `!==` are not supported. Precompute comparisons and flags in `data-function` (T6).
- Interpolation is not limited to text: component attribute values and `@tap` arguments can
  interpolate too (`@tap="{{ comp_id }}.goToSlide(index=${index + 1})"`).
- String values in the data are HTML-escaped before they reach the template (injection
  protection).


## T4 · Loops and conditions use the `spz-for` / `spz-if` directives

Put conditions and loops in attributes, not as large JS inside `${}`. The `<template>` content stays
verbatim in the body of the HTML source, so code strings like `${...map(...) => ...}` get picked up
by search engines as page text and show up in snippets; directives live in attributes and stay out
of the text.

| Directive | How | Note |
|---|---|---|
| `spz-if` | `<div spz-if="${cond}">`; the value **must** be wrapped in `${}` | When false the node is not rendered at all; exception when on the same node as `spz-for`, see below |
| `spz-else` | Valueless `spz-else` on the sibling node right after an `spz-if` node | Only adjacent siblings count |
| `spz-for` | `<li spz-for="item in data.list" key="index">` or `(item, index) in data.list`; the value is **not** wrapped in `${}` | Interpolate with `${item.xxx}` inside the loop body; add `key` |

```html
<template>
  <div class="items">
    <p class="item" spz-for="(item, index) in data.list" key="index">${item.title}</p>
    <span class="badge" spz-if="${data.soldOut}">Sold out</span>
    <span class="badge" spz-else>In stock</span>
  </div>
</template>
```

- The `spz-for` value is only "alias in data path"; the path has no indexes, quotes or method
  calls. Data that needs processing (filtering, joining, sorting) is computed beforehand in the
  `data-function` preprocessor (T6).
- `spz-if` may share a node with `spz-for` to filter item by item; then its value is **not** wrapped
  in `${}` (`spz-if="!!item.image"`). When the loop expands, the component adds the `${}` itself;
  wrapping it yourself produces `${${...}}` and the whole template fails to render. The same holds
  for `key`.
- Short value-level processing may still use a `${}` ternary or a one-line array method
  (`${item.options.map(o => o.value).join('/')}`) — the test is that it produces one value, not a
  chunk of HTML.

## T5 · Liquid vs `${}`

`{% %}` / `{{ }}` are fully evaluated on the server first; `${}` is left for the browser. So:

- Values fixed at render time — settings copy, translations — are written in Liquid directly and
  can be mixed with `${}` in the same template (the Liquid result becomes static template text).
- Runtime data returned by an API or a script always uses `${}`. `{{ item.title }}` is evaluated
  to an empty string at render time, so runtime data never gets in.

## T6 · Data preprocessing: `<template data-function>`

When data must be processed before it reaches the template (derived fields, merging several data
sources), attach a preprocessing function to the `<template>` instead of cramming the computation
into `${}`:

```html
<template data-function="function(data){ data.hasItems = (data.items || []).length > 0; return data; }">
```

- Keep it to a few lines. Arrow functions work too.
- The function body is ordinary JS, not bound by the `${}` limits; the object you `return` becomes
  the template data.
- If it doesn't `return`, or throws, rendering continues with the original data — preprocessing
  errors are silent, so don't count on an error to reveal a problem.
- With several data sources (`src="a;b"`) the `data` argument is an array in `src` order; the
  preprocessor merges it into the one object the template needs.


## Skeleton (covers T1–T6)

```liquid
{% capture render_id %}render-{{ block_id }}{% endcapture %}
{% capture data_id %}data-{{ block_id }}{% endcapture %}
{% capture tpl_id %}tpl-{{ block_id }}{% endcapture %}

<script type="application/json" id="{{ data_id }}">
  { "items": [ { "title": "A", "badge": "New" }, { "title": "B" } ] }
</script>

<ljs-render id="{{ render_id }}" layout="container" src="script:{{ data_id }}" template="{{ tpl_id }}"></ljs-render>

<template
  id="{{ tpl_id }}"
  data-function="function(data){ data.hasItems = (data.items || []).length > 0; return data; }"
>
  <div class="item-list" data-nosnippet>
    <h3>{{ block.settings.heading }}</h3>
    <ul spz-if="${data.hasItems}">
      <li spz-for="(item, index) in data.items" key="index">
        <span>${item.title}</span>
        <em spz-if="!!item.badge">${item.badge}</em>
      </li>
    </ul>
    <p spz-else>{{ block.settings.empty_text }}</p>
  </div>
</template>
```

What it shows: the template is referenced by id (T1); a single root `<div>` with `data-nosnippet`
(T2); runtime data only through `${}` interpolation (T3); conditions and loops via directives —
a standalone `spz-if` wraps `${}`, one sharing a node with `spz-for` doesn't (T4); settings copy
mixed in as Liquid (T5); the derived `hasItems` field computed in `data-function` (T6).
