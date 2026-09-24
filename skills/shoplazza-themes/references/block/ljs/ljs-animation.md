# ljs-animation — one-off CSS entrance animations

Plays a one-off CSS animation on existing nodes: entrance effects, animations that play when the
visitor scrolls to them. lessjs reference:
[spz-animation](https://lessjs.shoplazza.com/latest/components/spz-animation/) (write the tag as
`ljs-animation`).

## Rules

1. Use `ljs-animation`; no hand-written viewport observers or `classList` animation code.
2. `layout`: see the single-value table in [writing.md](writing.md) R4.
3. Required in practice: `trigger` (`visible` / `manual` / `scroll` / `click` / `mousemove`;
   comma-separate several to combine them, e.g. `trigger="visible,scroll"`), `animation-class`, and
   `target` or `new-target` (a selector for the nodes to animate).
4. Use the `run` action only with `trigger="manual"`; `visible` plays on entering the viewport, so
   don't bind a pointless `run`. `trigger="manual"` must use `new-target`; `target` is for the other
   triggers. With only `target`, `run` fails.
5. The animation itself is `@keyframes` + the `animation-class` rule in the block's own `<style>`;
   the component only decides when to add the class.
6. Staggered playback across several nodes: add the boolean `set-order` (no value). Each target
   gets a `--spzanim-order` CSS variable (0-based), so the delay is
   `animation-delay: calc(var(--spzanim-order) * 0.1s)`.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `trigger` | When to play | No | `visible` = on entering the viewport; `manual` = via the `run` action; `scroll` = continuously while scrolling (with CSS variables); `click` = clicking the target; `mousemove` = moving the pointer; comma-separate to combine, e.g. `visible,scroll` |
| `target` / `new-target` | Selector of the animated nodes | No | `trigger="manual"` must use `new-target`; other triggers use `target` |
| `new-target-root` | Mount container for `new-target` | No | Defaults to `document.body` |
| `animation-class` | CSS class to add | No | Value |
| `once` | Play once | No | Boolean; with `trigger="visible"`, plays once and the class is removed afterwards |
| `ignore-initial` | Skip the initial entry | No | Boolean; with `trigger="visible"`, elements already in the viewport on page load don't animate |
| `set-order` | Number the targets | No | Boolean; injects `--spzanim-order` |
| `order-root` | Grouping root for `set-order` | No | Selector; targets inside each root are numbered independently. Unset → all targets numbered together |
| `style-variables` | Initial CSS variables for manual triggering | No | With `trigger="manual"`; a JSON object string whose keys are CSS variable names (starting with `--`) |
| `event-root` | Listening root for click / mousemove | No | With `trigger="click"` / `"mousemove"`; a selector. Unset → listens on `document` |

## Actions & events

| Kind | Name | Notes |
|---|---|---|
| Action | `run` | Only with `trigger="manual"`. Optional arguments: `textContent` (updates the target's text, supports `${key}` placeholders) and any `--name` argument (injected as a CSS variable) |
| Event | `finish` | The animation finished; event data `args` = the arguments passed to `run` |

## Skeleton

```liquid
{% capture anim_id %}anim-{{ block_id }}{% endcapture %}
{% assign anim_trigger = block.settings.trigger | default: 'visible' %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-animation
    id="{{ anim_id }}"
    layout="logic"
    trigger="{{ anim_trigger }}"
    {% if anim_trigger == 'manual' %}
      new-target=".anim-box-{{ block_id }}"
    {% else %}
      target=".anim-box-{{ block_id }}"
    {% endif %}
    animation-class="fade-in-up"
  ></ljs-animation>
  <div class="anim-box-{{ block_id }}">{{ block.settings.label }}</div>
  {% if anim_trigger == 'manual' %}
    <button type="button" @tap="{{ anim_id }}.run">{{ block.settings.run_label }}</button>
  {% endif %}
</div>

<style>
  .{{ root_cls }} .anim-box-{{ block_id }} {
    min-height: 80px;
  }
  @keyframes fade-in-up-{{ block_id }} {
    from { opacity: 0; transform: translateY(16px); }
    to { opacity: 1; transform: translateY(0); }
  }
  .{{ root_cls }} .fade-in-up {
    animation: fade-in-up-{{ block_id }} 0.6s ease both;
  }
</style>
```
