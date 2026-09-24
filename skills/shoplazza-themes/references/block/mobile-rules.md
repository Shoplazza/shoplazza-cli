# themes block mobile — rules that keep a card usable on phones

This file only locks down the traps that make a phone page unusable. Spacing, font sizes, tap
targets and exact placement are not decided here — they follow the request and the merchant's
settings. Only things that outright break become rules (like the ones in
[Platform traps](#platform-traps)).

## Mobile first; desktop only overrides

- Default CSS is written for phones.
- Desktop differences go only in `@media screen and (min-width: 960px)` overrides.

## No page-level horizontal scroll

- The root container fills the available width (`max-width: 100%` / `min-width: 0`); never hard-code
  a `width` / `min-width` larger than the viewport.
- Long titles and questions may wrap (`overflow-wrap: anywhere` or equivalent); they must never push
  out a horizontal scrollbar.
- Horizontal swiping only happens inside an inner area (tab strip, carousel); the whole page must not
  swipe sideways with it.

## Paired per-device settings

Anything whose value differs between phone and desktop (height, column count, visible slide count)
must be two settings; never one desktop value applied to both.
Rendering: the phone field by default, the desktop field inside `min-width: 960px`.
When the request fixes a dimension ("always three columns", "two-column comparison"), that dimension
is not a setting — write it straight into the CSS, and still drop columns on narrow screens per
[Narrow-screen baseline](#narrow-screen-baseline).

## Interaction and edges

- Key actions must be directly tappable on phones; never make them appear only on hover.
- Sticky top / sticky bottom / fixed bars: give them a solid background, and the body text or main
  button must still scroll into view without being covered (reserve room with padding or equivalent;
  don't leave the shopper to guess).

## Images don't distort or collapse

- Image containers must reserve space (an aspect-ratio box); never let the page grow and jump only
  after the image loads.
- Fetch images per device: give `img_url` a width argument.

## Content decides height

- Card height grows with its content; never cut it with a hard-coded `height`.
- Cards in one row stay the same height: clamp variable-length text such as titles to a set number of
  lines (`-webkit-line-clamp` or equivalent), so one long title can't push the whole row out of line.

## Viewport units

- Full-screen height uses `100svh` / `100dvh`, never bare `100vh` (on phones it includes the address
  bar, and the height jumps while scrolling).
- Widths are always `100%`, never `100vw` (it includes the scrollbar width and pushes out a sideways
  scroll) — this is the concrete form of [No page-level horizontal scroll](#no-page-level-horizontal-scroll).

## Rich text and merchant-entered content

Content typed in the admin is unpredictable, so the container must contain it:

- `img { max-width: 100%; }`
- Tables and other wide content may only scroll sideways inside their own container.
- Long links and unbroken Latin strings must be able to break (`overflow-wrap: anywhere` or
  equivalent).

## Narrow-screen baseline

- Check the layout at 320px wide (iPhone SE / folding-phone cover screen): nothing collapses, nothing
  overflows.
- Multi-column layouts must drop columns on narrow screens; never force several columns there.

## Stacking and sticky elements

- A sticky-top element's `top` must clear the theme header, and its `z-index` must not exceed the
  theme navigation (it must neither cover the navigation nor be hidden by it).
- A sticky-bottom element makes room with `env(safe-area-inset-bottom)`, not just fixed padding (the
  iOS gesture bar covers it).

## Mobile feedback and performance

- Tappable elements give tap feedback with `:active`; `:hover` barely exists on phones (goes with the
  hover-only ban in [Interaction and edges](#interaction-and-edges)).
- Animate only `transform` / `opacity`; never animate `left` / `top` / `width` / `height` (drops frames
  on low-end phones).
- Horizontal swipe areas (tab strips, carousels) all use `scroll-snap` so they stop cleanly, leave
  space at both ends, and hide the scrollbar.

## Platform traps

- Input `font-size` must be ≥ 16px, or iOS zooms the whole page on focus.
- Never use `background-attachment: fixed` (it doesn't work on iOS).
