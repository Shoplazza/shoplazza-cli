# themes page templates — custom templates for chosen products, collections or pages

A custom template gives chosen products, collections or custom pages their own layout instead of
the default one. The template itself holds only a type, a title and its bindings; its content
(cards, copy, layout) is edited like any page ([card-edit.md](card-edit.md)) under the template
name `<type>.<suffix>`. Bound objects use the template; everything else keeps the default.

## Commands

| Intent | Command → result |
|---|---|
| List (optionally one type) | `themes template list --params '{"theme_id":"<theme_id>","type":"<type>"}'` → `.data.theme_templates[]`, `.data.count` |
| Create | `themes template create --params '{"theme_id":"<theme_id>"}' --data '{"type":"<type>","title":"<title>","from":"default","relations":["<id>"]}'` → `.data.theme_template.id` |
| Change bindings | `themes template update --params '{"theme_id":"<theme_id>","template_id":"<template_id>"}' --data '{"type":"<type>","relations":["<id1>","<id2>"]}'` → `data:{}` |
| Delete (consent) | `themes template delete --params '{"theme_id":"<theme_id>","template_id":"<template_id>"}'` → `data:{}` |
| Template name for editing | `themes +page -t <theme_id> --list` → `.data.templates[]` whose `type` is the custom type; `template` = `<type>.<suffix>` |

## The three types

| `type` | Use when | Replaces | `relations` | Result |
|---|---|---|---|---|
| `product` | An existing product's detail page needs a different layout | Product detail page | Product ids | Same URL; product data (price, variants, add to cart) works as usual |
| `collection` | An existing collection page needs a different layout | Collection page | Collection ids | Same URL |
| `page` | The store needs an extra page (landing, campaign, brand page); product and collection pages stay as they are | A custom page's display | Custom page ids | The custom page's `/pages/…` URL renders with this template; products on it come from cards |

A landing page that shows products or links buttons to products is still `page`: products are
picked inside cards, not by binding the template. The API also accepts `product_coll`; this
skill doesn't cover it. Other values (e.g. `blog`) → 400 `type is invalid.`.

## Flow

1. Resolve the theme (SKILL.md → Rules for every operation, 1).
2. `list` first; template ids and current bindings come from it. Unsure between two types → list
   both before choosing.
3. Create, update or delete by the rules below.
4. Edit its content: `+page -t <theme_id> --list` → `<type>.<suffix>` →
   [page-read.md](page-read.md) / [card-edit.md](card-edit.md).

## Create

- Reuse first: a template created earlier in this task, or one with the same title in `list`.
- Required: `type`, `title`. Pass `from:"default"` (starts from a copy of the default template).
- Bindings known → send `relations` in the same call; don't create empty and bind in a second
  call. Nothing named → create without `relations` and bind later with `update`.
- Ids must be real: products → `products +search` (shoplazza-products); collections →
  `products collections list`; custom pages → `shop pages list` (shoplazza-shop). Finding them:
  [resource-binding.md](resource-binding.md).
- "All products of collection X" (`product` only): the API takes `selected_all:1` +
  `front_query_params:{"collection_id":"<collection_id>","search_keyword":""}`, but a test create
  with them returned a template with no bindings (`count` 0). After the call, check the count in
  `template list`; if nothing is bound, tell the user and bind explicit product ids instead.
- An edit session is already open and the next step edits this template → add
  `"oseid":"<oseid>"` to the body. Otherwise the first `+page` on it opens a new session.
- The new template shows up in `+page --list` as `<type>.<suffix>` (suffix = creation timestamp)
  and can be read with `+page --template <type>.<suffix>` at once. It does not appear in
  `themes file tree`.

## Update

`relations` replaces the whole binding set: afterwards the bindings are exactly the ids sent.
`[]` or omitting `relations` unbinds everything. `type` is required (without it → 400
`invalid UpdateThemeTemplateRequest.Type…`).

Adding or removing one binding = read the current set, merge or remove, send the full array. A
`list` row shows `count` (number of bound objects) but only one `obj_id`:

- `count` `"0"` → no bindings; `"1"` → the set is `[obj_id]`.
- `count` above 1 → the full set isn't readable here. Sending a partial list would unbind the
  rest: ask the user for the complete list, or send nothing and say so.

## Delete

Needs consent (SKILL.md → Rules for every operation, 8): `--dry-run` → restate the template
(title, type, bound objects) → wait for the go-ahead. A second delete of the same id → 404
`NOT_FOUND_ERROR`.

## Rules

- The template created or chosen in this task is the working object for later turns — keep
  editing it; don't create another for the same thing. Read and write it with
  `--template <type>.<suffix>`.
- A template has no storefront URL of its own: never build `/pages/<title>`.
- Linking to the page from elsewhere (a homepage button): link the custom page the template is
  bound to, through the card's `page` field ([resource-binding.md](resource-binding.md)). Not
  bound yet → bind it first.
- Custom pages aren't created here. For a `page` template, bind an existing one from
  `shop pages list` — existing title or content doesn't block binding. Several fit → list them
  and ask.
- No custom page to bind → still create and edit the template: content, layout, draft, save and
  publish all work, and the merchant can preview it in the theme editor. Only the storefront
  `/pages/…` URL is missing until a custom page exists and is bound with `update`. Say the URL is
  pending; don't change the plan. Creating the custom page in admin is needed for the URL only.
- Don't use a `product` or `collection` template in place of `page`: those replace a page that
  already exists — the new page the merchant wanted still wouldn't exist, and an existing page
  would get the new layout.

## Output

- `list`: `.data.theme_templates[]` rows `{id (= template_id), title, type, suffix, count,
  obj_id, doc_id, …}`; template name `<type>.<suffix>`. No rows → `data:{}` (no `count`, no
  `theme_templates`).
- `create`: `.data.theme_template.id`.
- `update` / `delete`: `data:{}` — confirm with `list`, then summarize.
- `+page --list`: custom templates carry their own `type`; standard pages have `type:"system"`.
