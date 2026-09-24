# themes block concepts — the terms every AI-card reference uses

Writing, editing and deleting AI cards all use these terms; every rule file under `block/` means
exactly this by them.

- **section** — a unit of page layout, one liquid file. A page is a stack of sections, top to
  bottom.
- **block** — a content unit inside a section, its own liquid file. It can't stand on a page by
  itself; a section must hold it. An AI card is exactly one block file.
- **Inline sub-block** — a repeatable item declared in the block's own `{% schema %}` `blocks`
  array, which the merchant can add and remove. One level only.
- **Quick-add button vs add-to-cart link** — a quick-add button adds to cart from the product card
  itself (no variants → straight into the cart; several options → an in-card panel to pick the
  variant first). An add-to-cart link only goes to the product page. Buttons on a product-list
  card are always quick-add buttons, whether an icon or a full-width bar.
- **schema** — the setting definitions inside `{% schema %}` in a section or block file. They
  decide which settings and presets the card has in the theme editor.
- **ljs component** — a web component available to Shoplazza theme code; its tag starts with
  `ljs-` (`<ljs-xxx></ljs-xxx>`).
- **AI card** — a generated block. Its page instance's `type` starts with `blocks/gen_`; its file
  is `gen_<id>.liquid`.
- **`gen_id` and card type** — `gen_<id>` (the file name without extension) is what `--id`
  takes; the card type `blocks/gen_<id>` is what page instances and `block delete-gen` use.
- **Container (`_blocks` section)** — the section that holds an AI card on a page; its `type` is
  `_blocks` and it has no settings of its own. Call it the AI card when talking to the user; never
  say `_blocks`.
- **A card's three names** — `gen_<id>` is the file identifier, assigned by the server when the
  card is created. The schema's top-level `name` and `presets[].name` are one lowercase snake_case
  semantic name; the two must match each other, not the file name. The name merchants see in the
  editor is `presets[0].cname`.
- **Card copy language** — the language buyers read on the storefront: the default language of
  the store's primary market. Look it up with shoplazza-shop (markets / languages); the recipe is
  in [generate-block.md](generate-block.md) → Before writing.
- **Edit session (`oseid`)** — the editing state of a theme. Creating, editing and deleting cards
  and AI-card sources only take effect inside it. Use one `oseid` for the whole task unless the
  user switches to a theme outside the current session.
