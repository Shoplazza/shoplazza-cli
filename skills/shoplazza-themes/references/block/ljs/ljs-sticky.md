# ljs-sticky — sticky top / bottom bar

Sticky header or footer bar.

## Rules

1. Use `ljs-sticky`. No hand-written fixed bottom bar + JS show/hide.
2. `position="top"` / `"bottom"`.
3. To show/hide it, give it a stable `id` and use the actions `showSticky` / `hideSticky` (often
   driven by [ljs-observer](ljs-observer.md)).
4. Start hidden with the HTML `hidden` attribute. `show` is a display state the component sets; don't
   make it a schema switch.
5. Use only the attributes in the table.
6. The component doesn't position anything itself. It only writes inline `top/bottom: Npx` on an
   element that is already `fixed` / `sticky`, to stack several stuck elements without overlap.
   Write the positioning yourself: `position: sticky` + `top/bottom: 0`, or `fixed` pinned left and
   right. `position="bottom"` alone with no positioning CSS has no visible effect. To override the
   component's base styles, double the class (`.bar.bar`) instead of `!important`.
7. Sticking only shows when the scroll container overflows; the component doesn't add height. If the
   page is too short, the bar looks like a plain div — check the host's height / `overflow` before
   suspecting a missing attribute.
8. With `position: sticky`, no ancestor may have `overflow: hidden`. Put a top bar before the long
   content and a bottom bar after it (or swap the visual order with `column-reverse`).

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `position` | `top` / `bottom` | no | value attribute |
| `show` | Display state | no | set by the component; not a schema switch |
| `target` | Id of a reference element | no | bare id (no `#`); positions are recalculated against that element once it exists |
| `bottom-compensation-container-class` | Extra class for the bottom placeholder container | no | added to the placeholder the component creates to compensate a bottom bar's height; only with `position="bottom"` |

Actions: `showSticky` / `hideSticky`. No custom events.

## Skeleton

```liquid
{% capture sticky_id %}sticky-{{ block_id }}{% endcapture %}
{% assign sticky_position = block.settings.position | default: 'bottom' %}

<div class="{{ root_cls }} is-{{ sticky_position }}" {{ block.shoplaza_attributes }}>
  <ljs-sticky
    id="{{ sticky_id }}"
    class="sticky-bar"
    layout="container"
    position="{{ sticky_position }}"
    {% if block.settings.start_hidden %}hidden{% endif %}
  >
    <div class="sticky-inner">
      {{ block.settings.bar_text | default: 'Sticky bar' }}
      {% if block.settings.button_text != blank %}
        <a href="{{ block.settings.button_link.url | default: '#' }}">{{ block.settings.button_text }}</a>
      {% endif %}
    </div>
  </ljs-sticky>
</div>

<style>
  /* The component only handles sticking; positioning is yours */
  .{{ root_cls }} .sticky-bar {
    position: sticky;
    z-index: 5;
    background: #fff; /* a floating layer needs an explicit background */
  }
  .{{ root_cls }}.is-bottom .sticky-bar { bottom: 0; }
  .{{ root_cls }}.is-top .sticky-bar { top: 0; }
  /* For a top bar, show it before the long content */
  .{{ root_cls }}.is-top { display: flex; flex-direction: column-reverse; }
</style>
```

Show/hide with an observer: `@scrollToVisible="{{ sticky_id }}.hideSticky"` /
`@scrollToInvisible="{{ sticky_id }}.showSticky"`; read [ljs-observer.md](ljs-observer.md) first.
