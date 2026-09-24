# ljs-lightbox — full-screen overlay layer

A content layer over a full-screen mask (modal dialog). lessjs reference:
[spz-lightbox](https://lessjs.shoplazza.com/latest/components/spz-lightbox/) (write the tag as
`ljs-lightbox`).

## Rules

1. Open and close through the component's actions `<id>.open` / `<id>.close`; don't invent other
   methods.
2. The trigger button lives in the same root block as the lightbox:
   `@tap="{{ lightbox_id }}.open"` / `.close`.
3. Content goes in the lightbox's children (a card / image and text). To keep it open when the
   shopper clicks outside, add `unclose-in-focus` (boolean, no value).
4. Boolean attributes carry no value (`disable-unmount`, `unclose-in-focus`, …).
5. A child with the valueless `close` attribute automatically closes the lightbox on click — an
   alternative to `@tap="<id>.close"`; use one or the other. Only the first `[close]` element takes
   effect; further ones do nothing.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `id` | Instance id | No | For open/close |
| `animate-in` | Entrance animation | No, default `fade-in` | Only `fade-in` / `fly-in-bottom` / `fly-in-top`; any other value makes the component error and the lightbox doesn't render |
| `unclose-in-focus` | Don't close when focus leaves | No | Boolean, no value |
| `disable-unmount` | Don't unmount on close | No | Boolean, no value |
| `target` | Container the lightbox is moved into | No | Rare; normally not written |

- Actions: `open` / `close`.
- Events: `open` / `close`.

Use only the attributes in the table, plus `close` on a child (rule 5).

## Skeleton

```liquid
{% capture lightbox_id %}lightbox-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <button type="button" @tap="{{ lightbox_id }}.open">{{ block.settings.open_label }}</button>

  <ljs-lightbox id="{{ lightbox_id }}" layout="nodisplay" hidden>
    <div class="lightbox-panel">
      <div class="lightbox-head">
        <h3>{{ block.settings.title }}</h3>
        <button type="button" @tap="{{ lightbox_id }}.close">{{ block.settings.close_label | default: 'Close' }}</button>
      </div>
      <div class="lightbox-body">{{ block.settings.content }}</div>
      {% if block.settings.image != blank %}
        <ljs-img
          src="{{ block.settings.image | img_url }}"
          alt="{{ block.settings.title | escape }}"
          layout="responsive"
          width="800"
          height="600"
          object-fit="cover"
        ></ljs-img>
      {% endif %}
    </div>
  </ljs-lightbox>
</div>
```
