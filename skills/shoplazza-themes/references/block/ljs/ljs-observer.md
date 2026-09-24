# ljs-observer — scroll and visibility events

Declarative events for scroll direction, element visibility, and scroll boundaries. It renders
nothing; its events drive other elements' actions (`<id>.<action>(...)`).

## Rules

1. Use `ljs-observer`. No hand-written `addEventListener('scroll')` or `IntersectionObserver`.
2. Give it a stable `id`, or the events don't fire. Bind each event to the target's action.
3. Visibility in the viewport: `target` (an element id) + `@scrollToVisible` / `@scrollToInvisible`.
   If you set `threshold`, it must be strictly between 0 and 1 (e.g. `0.5`); `1` or any other
   out-of-range value breaks the component. Leave it out to use the default.
4. Scroll boundary: `value` (a scroll offset, a number or a window property such as
   `window.innerHeight`) + `@scrollExceedBoundary` / `@scrollNotExceedBoundary`.
5. Scroll direction: `@scrollUp` / `@scrollDown`; no extra attributes.
6. Visibility inside a scrolling container: `target` + `root-container` (the container's id;
   default is the window) + `@visible` / `@invisible`.
7. One observer may carry several events. To watch different targets, use one observer per target.
8. `layout`: take the value from the single-value table in [writing.md](writing.md).


## What it can do

| Capability | Events | Key attributes |
|---|---|---|
| Target enters / leaves the viewport | `scrollToVisible` / `scrollToInvisible` | `target` + `threshold` |
| Scroll passes / hasn't passed a boundary | `scrollExceedBoundary` / `scrollNotExceedBoundary` | `value` |
| Scroll direction | `scrollUp` / `scrollDown` | — |
| Target visible inside a scroll container | `visible` / `invisible` | `target` + `root-container` |
| Children added / removed | `childUpdated` | `target` |
| Target visible in the first screen | `visibleInFirstScreen` / `invisibleInFirstScreen` | `target` |

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `id` | Needed for events to fire | yes | unique in the page |
| `target` | Id of the observed element | yes, unless `selector` | when both are set, `target` wins |
| `selector` | CSS selector; first match is observed | only without `target` | |
| `threshold` | Visible ratio | no | strictly between 0 and 1 |
| `value` | Boundary scroll offset | for boundary events | number or window property |
| `root-container` | Id of the scroll container | no | default: the window |
| `media` | Media query; active only when it matches | no | |

Actions: `restart`.

Events: `scrollUp` / `scrollDown` / `scrollExceedBoundary` / `scrollNotExceedBoundary` /
`scrollToVisible` / `scrollToInvisible` / `visible` / `invisible` / `childUpdated` /
`visibleInFirstScreen` / `invisibleInFirstScreen`.

Use only these event names. Don't reach for `selector` or `media` unless needed; write
`root-container` only for a real container case.

## Skeleton

```liquid
{% capture observer_id %}observer-{{ block_id }}{% endcapture %}
{% capture target_id %}observe-target-{{ block_id }}{% endcapture %}
{% capture bar_id %}sticky-bar-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <div id="{{ bar_id }}" class="observe-bar">{{ block.settings.bar_text }}</div>

  <ljs-observer
    id="{{ observer_id }}"
    layout="logic"
    target="{{ target_id }}"
    @scrollToVisible="{{ bar_id }}.toggleClass(class='is-hidden', force=true)"
    @scrollToInvisible="{{ bar_id }}.toggleClass(class='is-hidden', force=false)"
  ></ljs-observer>

  <div id="{{ target_id }}" class="observe-anchor">
    {{ block.settings.anchor_text | default: 'Scroll target' }}
  </div>
</div>
```

The target action (here `toggleClass`) must be a method the target supports; the observer only
fires the event.
