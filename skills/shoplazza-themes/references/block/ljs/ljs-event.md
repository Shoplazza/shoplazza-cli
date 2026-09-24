# ljs-event — declarative event wiring (logic component)

Turns "watch a node / global event → call a target component's method" into a declarative
`ljs-event`. Not a UI component; anything that fits on the source element's own `@tap` /
`@slideChange` shouldn't use it. lessjs reference:
[spz-event](https://lessjs.shoplazza.com/latest/components/spz-event/) (write the tag as
`ljs-event`).

## Rules

1. Use `ljs-event`; no hand-written `addEventListener` that then calls component methods.
2. Prefer the source element's own events: a button that flips slides, the carousel's own callbacks
   and so on go on the source element. Use this component only when whoever listens ≠ whoever is
   clicked, or for global / cross-component events (see [writing.md](writing.md) R6).
3. `layout`: see the single-value table in [writing.md](writing.md) R4.
4. Line up the core trio: `observer-id` (whom to watch) + `event-name` (which event) +
   `target-id` / `target-api` (whom to call, and which method).
5. `observer-id` must be the id of an element that really exists in this card. Leaving it out
   watches `window` (the default), which only suits global events; neither `window` nor `body`
   catches `mouseleave` on a card area.
6. The leave area must be large with a visible boundary: `min-height` ≥ `50vh` plus a dashed
   border. A small padded box doesn't count as a "large area" — during checks you can't move out of
   it or see where it ends.
7. `target-api` must be one of the target component's own methods (`render` / `open`). Never
   `toggleClass`: global actions like it work only from `@tap`-style handlers; through `target-api`
   nothing happens, silently.
8. The tip bar is `position: fixed` at the bottom with a background color. The close button's
   `toggleClass` targets the target component's root (`ljs-render`), and the CSS is written as
   `.host.is-hidden`, not on the inner `.leave-tip`.
9. "Don't show again after closing" → `unListen`. Never `keep-status` (it writes a record to
   browser storage; after a refresh the event never fires again).
10. Boolean attributes carry no value.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `observer-id` | Id of the watched element | No | Value; listening starts once the element can be found. Default `window` |
| `event-name` | Event(s) to listen for | Yes | Value, e.g. `mouseout`; separate several with `;` |
| `target-id` | Target component id | No | Value |
| `target-api` | Target method | No | Value, e.g. `open` / `render`; default `render` |
| `keep-status` | Record the trigger in browser storage | No | Boolean; the event then fires only once, ever — use `unListen` instead |

### Advanced attributes (don't use unprompted)

| Attribute | Purpose | Notes |
|---|---|---|
| `delay` | Delay the call to `target-api` by N ms | Value |
| `event-pattern` | Only trigger when the event data matches | Value: `key=value` / `key!=value`, nested paths (`a.b=1`), regex (`key=/pattern/`) |
| `manual` | Pass the event data to `target-api` | Boolean; without it the argument is null |

- Action: `unListen` (`eventName` = the event to stop listening for).
- Event: `message` — only with `observer-id="window"` + `event-name="message"` (a window message
  arrived); not needed in cards.

## Skeleton (leave a large area of this card → `render` a bottom bar; `unListen` after closing)

```liquid
{% capture event_id %}event-{{ block_id }}{% endcapture %}
{% capture tip_id %}tip-{{ block_id }}{% endcapture %}
{% capture zone_id %}leave-zone-{{ block_id }}{% endcapture %}
{% capture data_id %}leave-data-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <script type="application/json" id="{{ data_id }}">{"tip": {{ block.settings.tip_text | json }}}</script>

  <ljs-event
    id="{{ event_id }}"
    layout="logic"
    observer-id="{{ zone_id }}"
    event-name="mouseleave"
    target-id="{{ tip_id }}"
    target-api="render"
  ></ljs-event>

  <div id="{{ zone_id }}" class="leave-zone">{{ block.settings.zone_text }}</div>

  <ljs-render id="{{ tip_id }}" layout="container" class="leave-bar-host" manual src="script:{{ data_id }}">
    <template>
      <div class="leave-tip">
        <p>${data.tip}</p>
        <button type="button" @tap="{{ tip_id }}.toggleClass(class='is-hidden', force=true);{{ event_id }}.unListen(eventName='mouseleave')">{{ block.settings.close_label }}</button>
      </div>
    </template>
  </ljs-render>
</div>

<style>
  .{{ root_cls }} .leave-zone {
    min-height: 50vh;
    border: 1px dashed #c7d2fe;
    background: #eef2ff;
  }
  .{{ root_cls }} .leave-bar-host.is-hidden { display: none; }
  .{{ root_cls }} .leave-tip {
    position: fixed;
    inset-inline: 0;
    bottom: 0;
    z-index: 999;
    background: #fff;
  }
</style>
```

`target-id` must point at a component actually written in this card, and `target-api` must be a
method listed in that component's doc; write neither from memory. Context outside the card (the
cart, global event buses) is not a dependency.
