# ljs-accordion — collapsible content sections

Content areas that collapse and expand (FAQ, accordion). lessjs reference:
[spz-accordion](https://lessjs.shoplazza.com/latest/components/spz-accordion/) (write the tag as
`ljs-accordion`).

## Rules

1. Use `ljs-accordion`; no `setInterval`, checkbox hacks or hand-toggled classes.
2. Each direct child is a `<section>` = one item.
3. Each `<section>` must have exactly 2 direct children: the first is the header (the clickable
   area), the second is the content. With one more or one fewer child the component errors and the
   whole accordion doesn't render.
4. Open by default: put the boolean `expanded` attribute on that `section`.
5. `expand-single-section`: only one item open at a time (common for FAQs).
6. `animate`: animation, boolean, no value.
7. Model items as inline child blocks (top-level `blocks` with a bare `type`) and output them with
   `for item in block.blocks`; styles go on the root block's class.
8. Don't nest `ljs-render` inside the accordion. For async rendering pick a component separately
   per [selection.md](selection.md) R2; don't embed it in the accordion.
9. The component owns the height: at runtime it writes an inline `height` + `overflow:hidden` on
   each `<section>` to animate. Therefore:
   - Child `<section>`s get no `padding` / `max-height` / `overflow`; `border-bottom` is fine. The
     component computes the inline height from the content without the section's own padding, so
     padding squashes text when collapsed and cuts content in half when expanded.
   - Spacing goes on the inner header / body: the section keeps only its border; padding is written
     on the `-header` / `-body` elements.
   - Leave show/hide to the component. Don't write `max-height:0` / `[expanded]{max-height:none}`;
     such CSS fights the component's height animation.
10. To style the open state (e.g. rotate an icon), use CSS on `section[expanded]` — the component
    adds and removes `expanded` as sections open and close.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `expand-single-section` | Mutually exclusive expansion | No | Recommended for FAQs |
| `animate` | Animation | No | Boolean, no value |
| `default-expand` | Sections open on load | No | Comma-separated section `aria-label` values or 0-based indexes; or use `expanded` on the section |
| `expanded` (on a `section`) | Section is open | No | Write it for default-open; the component also toggles it |
| `disabled` (on a `section`) | Section can't be opened/closed by clicking | No | Boolean |

## Actions (rarely needed)

| Action | Purpose | Parameters |
|---|---|---|
| `toggle` | Toggle a section open/closed | `section` (the target section's id) |
| `expand` | Open a section | `section` |
| `collapse` | Close a section (only closes, never opens) | `section` |
| `update` | Re-collect sections after the structure changed | none |
| `enable` | Make a section's header clickable again | `section` |
| `disabled` | Disable clicking a section's header (current open state unchanged) | `section` |

Example: `@tap="faq.toggle(section='item-2')"` (the target section needs a stable id).

## Events

`expand` (a section opened) and `collapse` (a section closed); event data `index` = the section's
index. Don't make the card depend on them: `expand` has been seen not to fire in some builds. Style
the accordion's own open state with CSS on `section[expanded]` (rule 10), and use the events only
for optional extras outside it.

## Skeleton (single file: root block + inline item child blocks)

Items are inline child blocks (bare `type: "faq_item"`). Don't name the loop variable `block` (it
collides with the root `block`); use `item`:

```liquid
{% capture acc_id %}faq-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-accordion
    id="{{ acc_id }}"
    layout="container"
    expand-single-section
    animate
  >
    {% for item in block.blocks %}
      {% capture item_id %}faq-item-{{ block_id }}-{{ forloop.index0 }}{% endcapture %}
      <section
        id="{{ item_id }}"
        {% if forloop.first and block.settings.expand_first %}expanded{% endif %}
        {{ item.shoplaza_attributes }}
      >
        <div class="faq-header">{{ item.settings.title }}</div>
        <div class="faq-body">{{ item.settings.content }}</div>
      </section>
    {% endfor %}
  </ljs-accordion>
</div>
```

Ids built from the loop index must use `capture` (see [liquid-rules.md](../liquid-rules.md) →
"Build strings with `capture`"). Item fields live in `item.settings`; items don't have their own
`blocks` (a third level isn't possible).
