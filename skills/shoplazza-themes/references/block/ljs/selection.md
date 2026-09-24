# ljs selection — which ljs components a card uses

Which ljs components this card should use. Rules every component follows are in
[writing.md](writing.md); how to write one component is in its own `ljs-<name>.md` in this folder.

## R1 · Components that are easy to confuse

| Pair | How to tell them apart |
|---|---|
| `ljs-animation` vs `ljs-observer` vs `ljs-interact-observer` | Play a predefined animation once when an element scrolls into view, then stop → `ljs-animation`. Only need a one-off event (enter/leave the viewport, scroll direction), the component changes no style itself and event handlers drive the target → `ljs-observer`. Style must change continuously with scroll distance (parallax offset, sticky reference) → `ljs-interact-observer` |
| `ljs-render` vs `ljs-list` | Render once, no paging → `ljs-render`. Paging / load more / infinite scroll → `ljs-list` |
| `ljs-video` vs `ljs-tiktok` / `ljs-vimeo` / `ljs-youtube` | Video file hosted by the store → `ljs-video`. External TikTok / Vimeo / YouTube → the matching `ljs-tiktok` / `ljs-vimeo` / `ljs-youtube`; never mix them with `ljs-video` |
| `ljs-carousel` vs `ljs-slide-indicator` | The carousel is the main component that slides; the slide indicator is its companion dots/progress bar and does nothing on its own |

## R2 · Intent → component

### Content and media

| Need | Use |
|---|---|
| Show an image | [ljs-img](ljs-img.md) |
| Several images / slideshow / marquee / swipe left-right | [ljs-carousel](ljs-carousel.md) + optional [ljs-slide-indicator](ljs-slide-indicator.md), [ljs-img](ljs-img.md) |
| Play the store's own video (mp4 / hls) | [ljs-video](ljs-video.md) |
| Embed a YouTube / Vimeo / TikTok video | [ljs-youtube](ljs-youtube.md) / [ljs-vimeo](ljs-vimeo.md) / [ljs-tiktok](ljs-tiktok.md) |
| 3D model preview | [ljs-model-viewer](ljs-model-viewer.md) |
| Image magnifier / hover to see detail | [ljs-zoom](ljs-zoom.md) |
| Show an amount in the store's currency | [ljs-currency](ljs-currency.md) |
| Show a date / let the shopper pick a date | [ljs-date](ljs-date.md) |
| Relative time ("3 hours ago") | [ljs-timeago](ljs-timeago.md) |
| Number count-up effect | [ljs-odometer](ljs-odometer.md) |
| A number that jumps within a range (**never** to fake view counts or stock) | [ljs-rng](ljs-rng.md) |

### Interaction and overlays

| Need | Use |
|---|---|
| FAQ / collapsible panels / accordion | [ljs-accordion](ljs-accordion.md) |
| Tabs / switch between panels | [ljs-tabs](ljs-tabs.md) |
| Countdown / time left in a limited-time offer | [ljs-countdown](ljs-countdown.md) |
| Dropdown menu | [ljs-dropdown](ljs-dropdown.md) |
| Single- or multi-select option group | [ljs-selector](ljs-selector.md) |
| Small bubble on hover or click | [ljs-tooltip](ljs-tooltip.md) |
| Full-screen content layer opened on click | [ljs-lightbox](ljs-lightbox.md) |
| Brief toast message | [ljs-toast](ljs-toast.md) |
| Sticky bar at top / bottom | [ljs-sticky](ljs-sticky.md) |
| In-page anchor navigation | [ljs-anchor](ljs-anchor.md) |
| Loading state | [ljs-loading](ljs-loading.md) |
| Scroll progress bar / step buttons for a scrollable container / scroll to a given child | [ljs-scrollbar](ljs-scrollbar.md) |
| Play a predefined CSS animation the moment an element scrolls into view (plays once; does not follow scroll position) | [ljs-animation](ljs-animation.md) |

### Products and buying

The product-list card's layouts and settings contract are in
[product-card.md](../kinds/product-card.md), the single-product card in
[product-single.md](../kinds/product-single.md), data access in [objects.md](../objects.md). This
table only answers which ljs component handles an interaction inside the card.

| Need | Use |
|---|---|
| Flip through the main images inside one card | [ljs-carousel](ljs-carousel.md) |
| Add-to-cart area of a product-list card, including switching variants on the card and adding right after choosing | [ljs-variants](ljs-variants.md) + [ljs-product-form](ljs-product-form.md) (restricted, see R3). Select them even when the request doesn't mention variants: write the list card's add-to-cart area per the add-to-cart section of [product-card.md](../kinds/product-card.md); products with several options default to the in-card panel |
| Choose variants, add to cart, buy now on a single-product card | [ljs-product-form](ljs-product-form.md) + [ljs-variants](ljs-variants.md) (restricted, see R3) |
| Choose quantity on the card (− 1 +) | [ljs-quantity](ljs-quantity.md) (restricted, see R3; must sit inside `ljs-product-form > form`) |

### Runtime logic (only when clearly needed)

| Need | Use |
|---|---|
| Fire one action the moment an element enters or leaves the viewport (e.g. add a class when it appears), or detect scroll direction | [ljs-observer](ljs-observer.md) |
| Style must follow scroll position continuously (image moves / scales as you scroll, parallax), or non-scroll interaction observing | [ljs-interact-observer](ljs-interact-observer.md) |
| A toggle that must **survive a refresh** (favorite heart, dismissed announcement bar, "got it" checkbox) | [ljs-state](ljs-state.md): localStorage by default (remembered long-term); add `state-type="sessionStorage"` to remember only for this session (cleared when the tab closes, e.g. a tip already shown this visit). Show/hide that need not survive a refresh → accordion or the main component's `@tap`, not this |
| Fetch data in the browser, then render it through a template (render once, no paging / load more) | [ljs-render](ljs-render.md) |
| Async list from an API / load more / paging | [ljs-list](ljs-list.md). Don't choose it when Liquid already has the data — use `{% for %}`. **Exception**: load-more for a product list uses it, see [product-card-async.md](../kinds/product-card-async.md) |
| One product's data for a purchase form / variant picker | [ljs-data-source](ljs-data-source.md) (`source-type="product"` + `source-id`; full example in the purchase skeleton of [product-single.md](../kinds/product-single.md)). **Only reference the id you declared yourself**; never reference a data source defined by the theme |
| Call a method on **another** component when the pointer leaves / enters an area or some node emits an event (e.g. leave the intro area → show a retention bar); generally, whoever triggers ≠ whoever responds, or events must be wired across nodes | [ljs-event](ljs-event.md) (not needed when it fits on the main component's `@tap`) |
| Custom logic none of the above covers, exported functions that feed data | [ljs-script](ljs-script.md) (static JSON uses the `script:` data carrier, not this) |

## R3 · Buying components are restricted

`ljs-product-form` / `ljs-variants` / `ljs-quantity` may appear only in the buy area of a product
card (list card: the add-to-cart section of [product-card.md](../kinds/product-card.md);
single-product card: the buy-area tiers of [product-single.md](../kinds/product-single.md)). Never
use them on non-product cards — a broken purchase flow directly affects orders. Exceptions are
stated in each component's own restriction notes.
