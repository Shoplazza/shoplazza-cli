# ljs-slide-indicator — carousel indicator

Dots (or a bar) for an [ljs-carousel](ljs-carousel.md); never hand-write indicator JS.

## Rules

1. Always pair it with an `ljs-carousel`.
2. `carousel-id` must equal the carousel's `id` (build both from one `capture`; never
   `forloop` + `append`).
3. Always write `size`: the number of visible dots, usually `5` (dots beyond it shrink). Without
   `size` the element is marked `empty` and no dots render at all.
4. `layout`: take the value from the single-value table in [writing.md](writing.md).
5. `type` sets the indicator style; default is dots. The public docs show the values `dot` and
   `bar`, including a media-query form such as `(min-width:960px) bar, dot`.
6. Don't hand-write dot switching; clicking a dot jumps to its slide on its own.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `carousel-id` | Linked carousel id | yes | same `capture` as the `ljs-carousel` `id` |
| `size` | Number of visible dots | yes | without it no dots render; usually `5` |
| `type` | Indicator style | no | default dots; `bar` for a bar |

## Skeleton

Style `{{ root_cls }}__dots` so the indicator is centred.

```liquid
{% capture carousel_id %}carousel-{{ block_id }}{% endcapture %}

<ljs-carousel id="{{ carousel_id }}" layout="container" loop autoplay delay="3000">
  {% for slide in block.blocks %}
    <div class="slide-item" {{ slide.shoplaza_attributes }}>…</div>
  {% endfor %}
</ljs-carousel>

{% if block.settings.show_dots %}
<div class="{{ root_cls }}__dots">
  <ljs-slide-indicator
    layout="container"
    carousel-id="{{ carousel_id }}"
    size="{{ block.settings.dot_visible_count | default: 5 }}"
    {% if block.settings.dot_style == 'bar' %}type="bar"{% endif %}
  ></ljs-slide-indicator>
</div>
{% endif %}
```

## Common mistakes

Leaving out `size` is the most common error; on the page the dots simply never appear:

```liquid
{% comment %} Wrong: no size — the element renders with `empty` and no dots inside {% endcomment %}
<ljs-slide-indicator layout="container" carousel-id="{{ carousel_id }}"></ljs-slide-indicator>
```
