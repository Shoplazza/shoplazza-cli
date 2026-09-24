# ljs-carousel — slideshows and horizontal sliders

Horizontal scrolling, slideshows, multi-frame heroes, review sliders and the like. lessjs
reference: [spz-carousel](https://lessjs.shoplazza.com/latest/components/spz-carousel/) (write the
tag as `ljs-carousel`).

## Rules

1. Use `ljs-carousel`; no `setInterval` or hand-written transform switching.
2. Dots use `ljs-slide-indicator` with `carousel-id` = the carousel's `id`, and it must have `size`
   (without it no dots render).
3. Each direct child = one slide (a single root element per slide; no `<style>` or HTML comments
   inside a slide).
4. Several slides per screen: set `visible-count`, per breakpoint with the media-query form
   `visible-count="(min-width:960px) 3, 1.1"`. Decimals show part of the next slide (`1.2` on
   mobile tells the shopper there is more).
5. Give the slide root a real px height (CSS variable); never only `height:100%` (black bars
   appear easily).
6. Arrows: add the boolean `controls`; the component brings its own prev/next buttons (preferred).
   No arrows → don't output `controls`. To change the arrow look, put child elements carrying the
   `pre` (previous) and `next` attributes inside the carousel (still with `controls`); don't
   `{% render %}` the theme's own icon snippets.
7. Images: `ljs-img` + `img_url`.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `initial-slide` | First slide shown | No | Default `0` |
| `visible-count` | Slides visible at once | No | Default `1` (full width); media-query form `(min-width:960px) 3, 1.1`; decimals allowed |
| `advance-count` | Slides advanced per step | No | Default `1`; media-query form allowed |
| `effect` | `scroller` (default) / `fade` / `swipe` | No | Value attribute, not boolean |
| `autoplay` / `loop` | Autoplay / loop | No | Boolean, no value; autoplay cycles even without `loop` |
| `controls` | Show arrows | No | Add when arrows are wanted; custom look per rule 6 |
| `delay` | Autoplay interval in ms | No, default `5000` | Schema in seconds → `\| times: 1000`; minimum `1000` |
| `direct` | Direction | No | `horizontal` (default) / `vertical` |
| `scroll-ratio` | Fraction of a slide's width the shopper must drag to switch | No | `0`–`1`, default `0.01` |
| `animate-time` | Transition duration in ms | No, default `300` | |
| `auto-height` | Animate height when slides differ in height | No | Boolean, no value |
| `pause` | Pause autoplay until the attribute is removed | No | Boolean, no value |

## Actions

| Action | Purpose | Parameters |
|---|---|---|
| `goToSlide` | Go to a slide | `index` (0-based) or `path` (the slide's `src`); optional `animate` |
| `goPrev` | Previous slide | none |
| `goNext` | Next slide | none |
| `updateDirect` | Change direction at runtime | `direct` (`horizontal` / `vertical`) |
| `updateCount` | Change `advance-count` at runtime | `count` |
| `calcPosition` | Recompute the arrows' position | none; needs `controls` |

## Events

`slideChange` (a slide change happens — usually enough); `slideEnd` (the transition animation
finished); `mediaChange` (a breakpoint change recomputed `visible-count` / `advance-count`);
`mounted` (the carousel finished mounting, once). Event data includes `index` (0-based current
slide) and `total`.

## Skeleton (single file: root block = carousel shell, inline slide child blocks)

```liquid
{% assign autoplay_delay_ms = block.settings.autoplay_delay | default: 5 | times: 1000 %}
{% capture carousel_id %}carousel-{{ block_id }}{% endcapture %}

<div
  class="{{ root_cls }}"
  style="--slide-height-pc: {{ block.settings.slide_height_pc }}px; --slide-height-mobile: {{ block.settings.slide_height_mobile }}px;"
  {{ block.shoplaza_attributes }}
>
  <ljs-carousel
    id="{{ carousel_id }}"
    layout="container"
    initial-slide="0"
    advance-count="1"
    visible-count="1"
    effect="{{ block.settings.transition_effect | default: 'scroller' }}"
    {% if block.settings.enable_autoplay %}autoplay delay="{{ autoplay_delay_ms }}"{% endif %}
    {% if block.settings.enable_loop %}loop{% endif %}
    {% if block.settings.show_arrows %}controls{% endif %}
  >
    {% for slide in block.blocks %}
      {% assign image = slide.settings.image %}
      <div class="slide-item" {{ slide.shoplaza_attributes }}>
        <div class="slide-media">
          {% if image != blank %}
            <ljs-img src="{{ image | img_url }}" alt="{{ images[image].alt | escape }}" layout="fill" object-fit="cover"></ljs-img>
          {% else %}
            <div class="slide-media-placeholder"></div>
          {% endif %}
        </div>
      </div>
    {% endfor %}
  </ljs-carousel>

  {% if block.settings.show_dots %}
    <div class="{{ root_cls }}_indicator">
      <ljs-slide-indicator layout="container" carousel-id="{{ carousel_id }}" size="5"></ljs-slide-indicator>
    </div>
  {% endif %}
</div>

<style>
  .{{ root_cls }} .slide-item {
    position: relative;
    width: 100%;
    height: var(--slide-height-mobile);
    overflow: hidden;
  }
  .{{ root_cls }} .slide-media { position: absolute; inset: 0; }
  @media screen and (min-width: 960px) {
    .{{ root_cls }} .slide-item { height: var(--slide-height-pc); }
  }
  .{{ root_cls }}_indicator {
    display: flex;
    justify-content: center;
  }
</style>
```

In the schema, `slide` uses a bare `type`, identical in top-level `blocks` and `presets[].blocks`.
Slide fields (image / heading / background color…) go in the slide's own `settings`. Slides have no
`blocks` of their own: a third nesting level isn't possible — writing one breaks the child-item
settings panel and the parent's toggles.

Custom arrow look (still with `controls`) — direct children of `<ljs-carousel>`:

```liquid
{% if block.settings.show_arrows %}
  <svg pre xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" style="width: 24px; height: 24px;">
    <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
  </svg>
  <svg next xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" style="width: 24px; height: 24px;">
    <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
  </svg>
{% endif %}
```

A list-style carousel with one product card per slide: see the carousel-list section of
[product-card.md](../kinds/product-card.md) — slides come from the collection's product loop, not
from inline slide child blocks.

A carousel of a product's main images inside one card: see
[product-card-interaction.md](../kinds/product-card-interaction.md) (in-card main image carousel).
