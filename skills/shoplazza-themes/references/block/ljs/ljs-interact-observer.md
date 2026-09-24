# ljs-interact-observer — styles that follow scroll position

Linked effects that interpolate continuously with scroll distance, such as parallax and sticky
references. lessjs reference:
[spz-interact-observer](https://lessjs.shoplazza.com/latest/components/spz-interact-observer/)
(write the tag as `ljs-interact-observer`).

## Rules

1. Logic component; bind `target-id` = the `id` of the element it drives. Never hand-compute
   transforms from scroll.
2. Two uses:
   - Parallax offset: `interact="scroll"` + `from` / `to` (e.g. `translateY:0` →
     `translateY:-40`) + optional `smoothness`.
   - Sticky reference: `interact="scroll"` + `observe-id` (the reference) + `target-id` (the sticky
     bar); optionally `<id>.change()` to refresh manually.
3. `from` / `to` / `interact` / `observe-id` are value attributes (e.g. `interact="scroll"`), not
   valueless booleans.
4. The observed / driven nodes need stable `id`s matching the attribute strings; styles go on the
   root block's class.
5. Single-root block; with no child blocks don't force a `for`.
6. With several observed items, generate each item's observer id and target id separately, both
   from `block_id` + `item.id`; write `target-id` as a bare id (no `#`), and the target node must
   really exist.
7. Headings, descriptions, trigger notes and default item copy go straight into `default` and the
   preset (in the card copy language), not `| t`; extra empty items render nothing.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `target-id` | Id of the driven element | Yes | Bare id, no `#` |
| `interact` | Interaction type | No | Use `scroll` (the default) |
| `from` / `to` | Start / end styles | No | For parallax; `prop:value;prop:value` with `translateX/Y/Z`, `scaleX/Y`, `rotateX/Y/Z`. A `to` containing `position:top` / `position:bottom` switches to stick-to-top / bottom mode |
| `smoothness` | Interpolation smoothness | No, default `30` | Number, e.g. `20`; larger = slower |
| `observe-id` | Reference element id | No | Sticky mode: the element watched for being in the viewport |
| `rely-id` | Reference element id | No | Same effect as `observe-id`; use one of the two, normally `observe-id` |
| `rely-sticky` | Recompute only while the reference itself is sticky/fixed | No | Use sparingly, only when the reference may not be sticky and must be filtered |
| `distance` | Scroll distance (px) that triggers the switch | No | Only with `interact="scroll"` and no `from` / `to`; defaults to the driven element's height |
| `entry-show` | Show immediately on first entry | No | Boolean; by default the target stays hidden while initially off-screen. Use only when a sticky bar should show from the start |

The only action is `change` (recompute the position manually); there are no custom events.

Use only the attributes in the table, and don't use `interact="mousemove"`.

## Skeleton

Parallax (enough for a single card):

```liquid
{% capture observer_id %}io-{{ block_id }}{% endcapture %}
{% capture card_id %}io-card-{{ block_id }}{% endcapture %}

<div
  class="{{ root_cls }}"
  {{ block.shoplaza_attributes }}
>
  <ljs-interact-observer
    id="{{ observer_id }}"
    layout="logic"
    target-id="{{ card_id }}"
    interact="scroll"
    from="translateY:0"
    to="translateY:{{ block.settings.parallax_offset | default: -40 }}"
    smoothness="{{ block.settings.smoothness | default: 20 }}"
  ></ljs-interact-observer>

  <div class="io-scene">
    <div id="{{ card_id }}" class="io-card">
      {% if block.settings.image != blank %}
        <ljs-img
          src="{{ block.settings.image | img_url }}"
          alt="{{ block.settings.heading | escape }}"
          layout="fill"
          object-fit="cover"
        ></ljs-img>
      {% endif %}
    </div>
    {% if block.settings.heading != blank %}
      <p class="io-tip">{{ block.settings.heading }}</p>
    {% endif %}
  </div>
</div>
```
