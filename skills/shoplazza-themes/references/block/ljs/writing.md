# ljs writing — rules every ljs component follows

Rules every ljs component follows. Which components to pick: [selection.md](selection.md). A
component's own attributes and usage: its `ljs-<name>.md` in this folder.

ljs components are the lessjs storefront components (public reference:
<https://lessjs.shoplazza.com/latest/components/overview/>). A card uses the plugin form: every
`spz-` tag name becomes `ljs-` (`spz-carousel` → `ljs-carousel`); all other usage stays the same
(<https://lessjs.shoplazza.com/latest/docs/plugin/>). That includes the `src="spz-script:<id>.<fn>"`
data protocol and the `spz-for` / `spz-if` template directives.

## R4 · How to set `layout`

`layout` decides how a component reserves space before it renders: from `layout` / `width` /
`height` alone the runtime can size and place the element without waiting for JS or data.

The attribute name is the same everywhere; the value is fixed per component. Always follow the two
tables below; don't guess from what "usually" works.

| Value | Meaning |
|---|---|
| `container` | Children define the size, like a plain `div`; no layout of its own, children render immediately |
| `logic` | No style, takes no space by default; for pure logic components (`event` / `observer` kind) |
| `nodisplay` | Not shown, takes no space (like `display:none`); shown by a user action (`lightbox` / `toast` kind) |
| `fill` | Fills the parent's available space; the parent needs `position:relative` / `absolute` |
| `responsive` | Height adapts to the aspect ratio given by `width` / `height`; needs both |
| `fixed` | Fixed width and height, no adapting; needs both |
| `fixed-height` | Fixed height only, width adapts (for horizontally laid-out content such as a carousel) |
| `intrinsic` | Adapts to the aspect ratio until it reaches the `width` / `height` size or a CSS limit (e.g. `max-width`) |
| `flex-item` | When the parent is `display:flex`, shares the remaining space with sibling `flex-item`s |

Which component takes which value (copy as is):

| `layout` value | Components |
|---|---|
| `container` | `accordion` `anchor` `carousel` `countdown` `currency` `date` `list` `model-viewer` `odometer` `product-form` `render` `rng` `scrollbar` `selector` `slide-indicator` `state` `sticky` `tabs` `timeago` `variants` `zoom` |
| `logic` | `animation` `event` `interact-observer` `observer` `script` `tooltip` |
| `nodisplay` | `dropdown` `lightbox` `toast` |
| **See the component doc** | `img` `video` `vimeo` `youtube` `tiktok` `quantity` `loading` — which of `fill` / `responsive` / `fixed` / `fixed-height` / `intrinsic` / `flex-item` (or `nodisplay` for loading) depends on the display need, usually paired with `width` / `height`; each component doc explains it |

Component docs don't repeat `layout` values; this table is the only source.

## R5 · Attributes, ids and CSS

1. Never use a component whose `ljs-<name>.md` you haven't read. Usage is only verifiable from the
   doc; writing from memory fails silently (no error, nothing renders).
2. Never invent attribute names. Use only names that appear in a component doc you have read —
   unknown attributes are ignored silently, so a typo can't be spotted from the rendered result.
3. Make interaction declarative: prefer the main component's `@tap` / `@slideChange` etc.; only
   when that can't cover it, consider a logic component you have read (R6).
4. Boolean attributes carry no value: when off, don't output them. Never `autoplay="false"`.
5. Stable ids and classes: derive linked ids from `block_id` and use `root_cls` for the root class
   (both declared per [liquid-rules.md](../liquid-rules.md) → "Card root"); inside loops, build the
   id with `capture` and append a suffix.
6. Unit conversion: carousel `delay` seconds → ms (`| times: 1000`); countdown days → seconds
   (`| times: 86400`), then pass `timeleft-seconds`.
7. Don't fight the CSS a component controls:

| Component | The component controls | What you write |
|---|---|---|
| `ljs-tabs` | Panel `display` (only `[active]` shows) | Layout goes on `.panel[active]`, media queries included |
| `ljs-accordion` | Inline `height` + `overflow` on each child `<section>` | Child sections carry only borders; padding goes on inner elements; no `max-height` |

## R6 · Actions vs events, event binding

An action is a callable method (`id.goNext()`); an event is a signal a component dispatches when
its state changes (`@atcSuccess="..."`) and can only be listened to, never called. Every component
doc keeps them in two separate tables; don't mix them up.

Every element, lessjs or native, also accepts the lessjs global actions `show`, `hide`,
`toggleClass(class=…, force=…)`, `toggleAttribute(key=…, value=…, force=…)`, `scrollTo` and
`focus`, and fires the global `tap` event
(<https://lessjs.shoplazza.com/latest/docs/actions-and-events/>).

Bind on the main component first:

```liquid
{% capture comp_id %}carousel-{{ block_id }}{% endcapture %}
<ljs-carousel id="{{ comp_id }}" layout="container" ...></ljs-carousel>
<button type="button" @tap="{{ comp_id }}.goNext()">{{ block.settings.next_label }}</button>
```

| Situation | What to do |
|---|---|
| It fits on the main component | Don't use `ljs-event` (e.g. button → `goNext`) |
| When to use `ljs-event` | Whoever triggers ≠ whoever responds, or a global / cross-component event must reach a target's method, and you have read [ljs-event.md](ljs-event.md). `target-api` must be the target component's own method (`render` / `open`), not `toggleClass` |
| When to use `ljs-script` | You need `exportFunction` / `execute`, or a function that feeds data through `src="spz-script:<id>.<fn>"`, and you have read [ljs-script.md](ljs-script.md) |
| Static JSON config | `<script type="application/json" id="…">` + `src="script:<id>"`, **not** `ljs-script` |
| Default | Don't press logic components into service as UI components |

## R7 · Containers and companion components (carousel / accordion / indicator)

1. Direct child = one slide / one item. An inline child item renders a single root element; no
   `<style>` or HTML comments inside an item.
2. Items the merchant can add or remove = inline child blocks at the top level of the root block,
   output directly with `for item in block.blocks` (one level only). Data the platform already has
   (products, collections) is not modeled as child items; use a resource-type setting as the data
   source instead ([schema-rules.md](../schema-rules.md) → "One block file, at most one level of
   inline children").
3. `ljs-slide-indicator`'s `carousel-id` must equal the carousel's `id` (same `capture`), and it
   must have `size` (without it the element is marked `empty` and renders no dots).
4. Give heights in real px (CSS variables); don't rely on `height:100%` alone.

## R8 · Templates

Components that use a `<template>` (`ljs-render` / `ljs-list` / `ljs-scrollbar` / `ljs-countdown` /
the async product-card tier, etc.): how to declare the template, the single-root rule, the limits
of `${}` expressions and the split with Liquid are all in [template.md](template.md). Component
docs only describe their own data fields and don't repeat the template rules.

The template must be written inline as a child of the component (or referenced by id in the same
file), with exactly one root element, and `${}` must not call global objects. `template-src`, the
`shoplaza_asset_url` filter, and the theme's own global data and snippets are all unavailable in
templates.
