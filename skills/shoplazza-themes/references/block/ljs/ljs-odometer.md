# ljs-odometer — rolling number counter

Rolls a number up to a target (sales count, milestones). The animation starts when the element
first becomes visible.

## Rules

1. Use it only when the request explicitly asks for a rolling-number effect (see
   [selection.md](selection.md)); don't add it on your own.
2. Set both `start-value` and `end-value`, each with a value — they are not boolean switches.
   Each stat child block has its own `start_value` setting (default `"0"`); `end-value` takes that
   item's target number.
3. `format` is per stat item: each stat child block has a text setting `format`, default
   `"(,ddd).d"`, output on its own component. Putting it on the parent makes every item share one
   format; hard-coding it in the tag leaves the merchant unable to change it.
4. `duration` is in milliseconds; default `2000`, and the schema default is `2000` too.
5. The component is inline; control font size and spacing from the block's root class.
6. One component shell in the root block; each stat item is a child block so start value, target,
   format, title, and prefix/suffix are configurable per item.
7. With several items, each gets its own `ljs-odometer`, with `item.id` appended to its `id`.
   Prefix / suffix go in sibling `<span>`s outside the component, never inside it.
8. Default item labels and other copy go straight into the matching setting's `default` and the
   preset.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `start-value` | Start number | yes | setting default `"0"` |
| `end-value` | Target number | yes | |
| `format` | Number format | no | per-item setting, default `(,ddd).d` |
| `duration` | Animation length | no | ms; default `2000` |

No events, no actions. Use only the attributes in the table; don't style or expose the nodes the
component generates inside itself.

## Skeleton

```liquid
<div
  class="{{ root_cls }}"
  {{ block.shoplaza_attributes }}
>
  {% for item in block.blocks %}
    <div class="odo-item" {{ item.shoplaza_attributes }}>
      <span>{{ item.settings.prefix }}</span>
      <ljs-odometer
        id="odometer-{{ block_id }}-{{ item.id }}"
        layout="container"
        start-value="{{ item.settings.start_value | default: '0' }}"
        end-value="{{ item.settings.target }}"
        format="{{ item.settings.format | default: '(,ddd).d' }}"
        duration="{{ block.settings.duration | default: 2000 }}"
      ></ljs-odometer>
      <span>{{ item.settings.suffix }}</span>
    </div>
  {% endfor %}
</div>
```
