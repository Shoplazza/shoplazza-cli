# ljs-tabs — tab panels

Switches between tab panels.

## Rules

1. Use `ljs-tabs`. Don't toggle an `active` class yourself.
2. Structure:
   - Nav: `<ul role="tabs">` → `<li role="tab" data-panel="...">`
   - Panels: `<div role="tabpanel" data-id="...">` (`data-id` equals the matching tab's `data-panel`)
3. Build panel ids with `capture`: `assign` of `forloop.index0` followed by `append` yields an empty
   string, and the tabs don't respond.
4. Write the nav and the panels in the same file, each as its own `for item in block.blocks` loop.
   Both loops use the same inline child block (plain `type: "tab"`) and `forloop.index0` produces
   matching `data-panel` / `data-id`. Moving the panels into a separate file rendered through
   `content_for` loses the shared `forloop`; the ids no longer match and clicking does nothing.
5. `default-tab`: the starting index, a numeric string such as `"0"`; nothing else.
6. Give it a stable `id` (derived from `block_id`).
7. `record-state`: optional, remembers the selected tab (boolean, no value).
8. Images in a panel use `ljs-img` + `img_url`; plain copy is plain Liquid.
9. `interact`: how tabs switch — `click` or `hover` only. Any other value makes the component error.
   Default `click`.
10. The component owns show/hide: it shows the panel that has `[active]`. A bare `display` on
    `.panel` overrides the component's hiding → every panel shows at once and clicking a tab looks
    dead. Put layout rules on `[active]`:

```css
/* Wrong: no [active] — all panels visible, tabs look dead */
.xx__panel { display: flex; }

/* Right */
.xx__panel[active] { display: flex; }
```

The same goes for changing `flex-direction` inside `@media`: include `[active]`. Style the selected
`role="tab"` with `[active]` too; don't toggle a class.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `default-tab` | Initially selected index | no | usually `"0"` |
| `record-state` | Remember the selected tab | no | boolean, no value |
| `interact` | How tabs switch | no | `click` / `hover`; other values make the component error; default `click` |

Events: only `tabChange` (payload: the active panel's `data-id`); rarely needed. `tabChange` is also
an action: `@xxx="<id>.tabChange(dataId='...')"` switches to that panel programmatically.

## Skeleton

Name the loop variable `item` (not `block`, which would clash with the root `block`):

```liquid
{% capture comp_id %}tabs-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-tabs id="{{ comp_id }}" layout="container" record-state default-tab="0">
    <ul role="tabs" class="tabs-nav">
      {% for item in block.blocks %}
        {% capture panel_id %}panel-{{ block_id }}-{{ forloop.index0 }}{% endcapture %}
        <li
          role="tab"
          data-panel="{{ panel_id }}"
          class="tabs-nav-item"
          {% if forloop.first %}active{% endif %}
          {{ item.shoplaza_attributes }}
        >{{ item.settings.tab_label }}</li>
      {% endfor %}
    </ul>

    {% for item in block.blocks %}
      {% capture panel_id %}panel-{{ block_id }}-{{ forloop.index0 }}{% endcapture %}
      <div
        role="tabpanel"
        data-id="{{ panel_id }}"
        class="tabs-panel"
        {% if forloop.first %}active{% endif %}
      >
        <h2>{{ item.settings.heading }}</h2>
        <div>{{ item.settings.description }}</div>
        {% if item.settings.image != blank %}
          <ljs-img
            src="{{ item.settings.image | img_url }}"
            alt="{{ item.settings.heading | escape }}"
            layout="fill"
            object-fit="cover"
          ></ljs-img>
        {% endif %}
      </div>
    {% endfor %}
  </ljs-tabs>
</div>
```

Put `item.shoplaza_attributes` only on the nav `<li>`, not again on the panel `<div>`: one anchor is
enough for the editor, and two would output the same child block's attributes twice.
