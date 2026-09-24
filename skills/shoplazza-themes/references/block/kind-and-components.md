# themes block kind & components — decide what else to read before writing code

Before writing or changing code, decide which ljs components this card uses and which kind of
card it is. That decides the remaining rules to read; read them all, then start on the code.

**Components** come from the intent → component table in [ljs/selection.md](ljs/selection.md).
A new card picks its full set from the requirement. An edit first identifies which components in
the source the change touches, then decides whether it needs a component the source doesn't have
yet.

**Card kind:**

| Requirement | kind | Rules |
|---|---|---|
| Renders product / collection data, several products in one card | `product` | Forms, tiers and setting contract in [kinds/product-card.md](kinds/product-card.md) |
| Renders product data, one product per card | `product` | Forms, purchase-area tiers and setting contract in [kinds/product-single.md](kinds/product-single.md) |
| All content comes from merchant input; no product / collection data | `generic` | The common rules plus the component docs are the whole rule set |

**Objects:** read [objects.md](objects.md) when the card reads product / collection data, has a
`page` / `link_list` / `blog` / `article` / `image_picker` / `video_picker` setting, or outputs
shop information. Skip it when all content is merchant-typed text and colors.

Once components, kind and objects are settled, read everything that matched, in parallel where
the host allows:

- `ljs/ljs-<name>.md` for each component used (e.g. [ljs/ljs-carousel.md](ljs/ljs-carousel.md),
  [ljs/ljs-countdown.md](ljs/ljs-countdown.md)).
- [ljs/template.md](ljs/template.md) when using `ljs-countdown`, `ljs-list`, `ljs-scrollbar` or
  `ljs-render` — their content structure is written in an inline `<template>`.
- The kind file named by the matching row above (it points on to
  [kinds/product-card-async.md](kinds/product-card-async.md) or
  [kinds/product-card-interaction.md](kinds/product-card-interaction.md) when a tier needs them).
- [objects.md](objects.md) when the objects paragraph applies.

Components the source already uses count as matched too: editing one without its attribute and
event conventions produces markup that fails silently, and [self-check.md](self-check.md) only
catches what it lists.
