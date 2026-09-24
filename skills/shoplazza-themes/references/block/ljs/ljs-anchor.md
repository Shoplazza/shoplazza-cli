# ljs-anchor — anchor navigation linked to a scroll area

Side-bar anchors linked to a scrolling content area. lessjs reference:
[spz-anchor](https://lessjs.shoplazza.com/latest/components/spz-anchor/) (write the tag as
`ljs-anchor`).

## Rules

1. Use `ljs-anchor`; no hand-written anchor-scroll highlighting JS.
2. Structure:
   - Link area: `role="anchor-links"` → children with `data-anchor-link="key"`.
   - Scroll area: `role="anchor-scrollable-container"` → matching parts with `data-anchor-part="key"`.
   - Link and part keys match one to one. Generate both loops' keys from the same `block_id` +
     `item.id`; never use merchant-typed copy as the key (it can repeat).
3. Prefer a stable `id` (derived from `block_id`).
4. The only actions are `freeze` / `reset`, called from `@tap` when needed; there are no custom
   events.
5. `role="anchor-scrollable-container"` must actually scroll (fixed height or `max-height` +
   `overflow: auto`, with content taller than the container). Otherwise clicks still switch
   `active`, but the scroll linkage can't be verified.
6. Read the nav container's `aria-label` from settings (reuse the heading setting or add a
   dedicated one); write the default copy into `default` / the preset, not `| t`.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `id` | Instance id | No | Needed to call actions |
| `data-anchor-link` | Nav item key | No | On the link children |
| `data-anchor-part` | Content part key | No | Same key as its link |
| `manual` | Don't bind events on init | No | Boolean, no value; normally not written |

The component adds `active` to the current link and part — style the highlight with `[active]`.

Use only the attributes above; any other name is ignored silently and a typo can't be seen in the
rendered result.

## Actions

| Action | Purpose | Parameters |
|---|---|---|
| `freeze` | Remove the component's event listeners | none |
| `reset` | Re-initialize the listeners | `activeValue` (optional: key to activate) |

## Skeleton

```liquid
{% capture anchor_id %}anchor-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-anchor id="{{ anchor_id }}" layout="container" class="anchor-wrap">
    <ul class="anchor-tabs" role="anchor-links">
      {% for item in block.blocks %}
        {% capture anchor_key %}anchor-{{ block_id }}-{{ item.id }}{% endcapture %}
        <li data-anchor-link="{{ anchor_key }}" {{ item.shoplaza_attributes }}>
          {{ item.settings.label }}
        </li>
      {% endfor %}
    </ul>
    <ul class="anchor-scrollable-container" role="anchor-scrollable-container">
      {% for item in block.blocks %}
        {% capture anchor_key %}anchor-{{ block_id }}-{{ item.id }}{% endcapture %}
        <li data-anchor-part="{{ anchor_key }}" {{ item.shoplaza_attributes }}>
          <h3>{{ item.settings.heading }}</h3>
          <div>{{ item.settings.body }}</div>
        </li>
      {% endfor %}
    </ul>
  </ljs-anchor>
</div>

<style>
  .{{ root_cls }} .anchor-scrollable-container {
    max-height: 240px;
    overflow: auto;
  }
</style>
```
