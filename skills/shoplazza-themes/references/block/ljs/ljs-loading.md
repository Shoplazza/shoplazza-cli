# ljs-loading — loading state

Full-screen or area loading state, driven by `showLoading` / `close`. lessjs reference:
[spz-loading](https://lessjs.shoplazza.com/latest/components/spz-loading/) (write the tag as
`ljs-loading`).

## Rules

1. Use `ljs-loading`; show / hide through `<id>.showLoading` / `<id>.close` (`@tap`), never by
   toggling `display` yourself.
2. Usually `layout="nodisplay"` + `hidden` + a stable `id`, shown on demand. (`layout="container"`
   shows it inline right away.) `has-mask` (boolean, no value) shows only the dimming mask, without
   the spinner.
3. The component brings its own fixed overlay styling; don't stack a custom full-screen mask on
   top in the card.
4. The only actions are `showLoading` and `close`; there is no `open`. Always trigger it as
   `@tap="<id>.showLoading"`.
5. If the element is still `opacity: 0` after `showLoading`, add
   `#<id>:not([hidden]) { opacity: 1 !important; }` for that instance (minimal style below). Don't
   build your own mask DOM for it; showing and closing still go through the component's actions.
6. When the page also has a close button, lift its container above the component's fixed overlay
   (`position: relative; z-index: 1040`); otherwise once loading shows, `close` can never be
   clicked again.


## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `layout` | Layout | No | See rule 2 |
| `id` | For the actions | No | `capture` it from `block_id` |
| `has-mask` | Mask only, no spinner | No | Boolean, no value |
| `hidden` | Hidden initially | No | Pair with `nodisplay` |
| `animate-in` | Entrance animation | No | Only `fade-in` (default) / `fly-in-bottom` / `fly-in-top` |

The only actions are `showLoading` / `close`; there are no custom events. An `open` action does
not exist.

## Skeleton

```liquid
{% capture loading_id %}loading-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if block.settings.show_demo_triggers %}
    <button type="button" @tap="{{ loading_id }}.showLoading">{{ block.settings.show_label }}</button>
    <button type="button" @tap="{{ loading_id }}.close">{{ block.settings.close_label }}</button>
  {% endif %}

  <ljs-loading
    id="{{ loading_id }}"
    layout="nodisplay"
    hidden
    {% if block.settings.has_mask %}has-mask{% endif %}
  ></ljs-loading>
</div>
```

The more common wiring is another `ljs-*` component calling `showLoading` and `close` around its
async action (via `@tap` / `@finish`); the skeleton above only shows mounting and manual
triggering.

Minimal style when it stays invisible after showing:

```liquid
<style>
  #{{ loading_id }}:not([hidden]) {
    opacity: 1 !important;
  }
</style>
```
