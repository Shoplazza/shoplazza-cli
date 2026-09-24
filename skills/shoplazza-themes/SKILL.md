---
name: shoplazza-themes
description: >-
  Store theme operations for a shoplazza store — use when the user works on the store's themes
  through the CLI: page & card editing (装修 / 改页面 / 改卡片 / 模块 / 板块 / 首页 banner /
  轮播图 / 页头页尾 / 加卡片 / 隐藏卡片 / 调整顺序 / sections & blocks / edit the homepage);
  theme settings (主题设置 / 配色 / 字体 / 全局样式 / colors / fonts); app embeds (弹窗 /
  公告栏 / app 开关 / app embed); custom templates (自定义模板 / 商品详情模板 / custom
  template); AI cards (生成卡片 / 自定义卡片 / 改卡片源码 / generate or edit a liquid block);
  installed themes & the market (主题列表 / 当前主题 / 主题市场 / 安装 / 改名 / 复制 / 删除 /
  升级 / 发布主题 / 预览链接 / theme list, install, rename, duplicate, delete, upgrade, publish,
  preview); theme files, version history, page builder. NOT local theme development (themes
  init / serve / push / pull); NOT custom page / blog / menu content (自定义页面内容 / 博客 /
  菜单 → shoplazza-shop); NOT product images (商品图片 → shoplazza-products); NOT creating a
  flash sale (→ shoplazza-discounts).
---

# shoplazza CLI — themes module (store theme operations)

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: the three access tiers, the output envelope (`.data`),
`--dry-run`, `--jq` (incl. "don't pass `-r`"), `schema`, `api rest`, auth / profiles, and the
safety protocol. This file covers only the themes domain and never repeats them.

## Overview

A store can hold many themes; exactly one is **published** (what buyers see). This skill covers
the "Store theme operations" half of `themes --help`: finding and managing installed themes,
the theme market, editing theme pages (cards, blocks, global settings, app embeds), custom
templates, AI cards (generated liquid blocks), theme files and version history, and the page
builder. Local theme development (`init` / `serve` / `push` / `pull` / `package` / `share` /
`env`) is out of scope — see `themes <cmd> --help`.

Page edits go through three layers:

| Layer | What happens | Who sees it |
|---|---|---|
| Edit session (`oseid`) | `+edit`, `session update-config`, `app enable/disable`, `block +edit` write here | Only the preview link |
| Save (`--promote`) | The whole session is written into that theme's draft | Still nobody on the storefront |
| Publish (`--publish` / `themes publish`) | That theme becomes the store's live theme, replacing the old one | Buyers, immediately |

Tiers: shortcuts `+page` (read) / `+edit` (write) / `+card-schema` / `+preview` and
`block +edit` / `block +get` come first; spec leaves (`--params` / `--data`) cover the rest.

## Command map

Intent → command, highest-fit tier first. Authoritative flags live in `themes <cmd> --help` and
`schema themes.<cmd>`, not this table. Every command that acts on a theme takes `-t <theme_id>`
(or `theme_id` in `--params`). On `+page` / `+edit` / `block +edit` / `block +get` an omitted `-t`
means the published theme; `+preview` and `+card-schema` require it.

| User intent | Command | Details |
|---|---|---|
| Which themes / which one is live / theme detail & version | `themes list --params '{"published":"1"}'` · `themes get --params '{"theme_id":"<id>"}'` | [query.md](references/query.md) |
| Storefront preview link (optionally with unsaved edits) | `themes +preview -t <id> [--oseid <oseid>] [--path /products/<handle>]` | [query.md](references/query.md) |
| Rename / duplicate / upgrade / delete / publish a theme | `themes rename` · `duplicate` · `upgrade` · `delete` · `publish` | [lifecycle.md](references/lifecycle.md) |
| Browse the theme market / install a theme | `themes market list` · `themes market install --params '{"remote_theme_id":"…"}'` | [market.md](references/market.md) |
| List a theme's page templates | `themes +page -t <id> --list` | [page-read.md](references/page-read.md) |
| Read a page's cards; open an edit session | `themes +page -t <id> --template <name>` → `.data.oseid` | [page-read.md](references/page-read.md) |
| Read one card: current values, field schema, block targets | `themes +page -t <id> --template <name> --session <oseid> --section <sid> --include schema` | [page-read.md](references/page-read.md) |
| Change card settings, add/remove/move blocks, hide/move/delete a card | `themes +edit -t <id> --template <name> --session <oseid> --ops '[…]'` | [card-edit.md](references/card-edit.md) |
| Save the draft / publish it | `themes +edit -t <id> --template <name> --session <oseid> --ops '[]' --promote [--publish]` | [card-edit.md](references/card-edit.md) |
| Add a new card to a page; which cards can be added | `themes section cards` → `themes +card-schema` → `+edit` `add_section` | [card-add.md](references/card-add.md) |
| Theme-wide settings (colors, fonts, layout, buttons, cart…) | `themes session get-config` / `session update-config` | [global-config.md](references/global-config.md) |
| App embeds and injected scripts: list, turn on/off | `themes app list` · `themes app enable` · `themes app disable` · `themes app extensions` | [apps.md](references/apps.md) |
| Custom templates bound to products / collections / pages | `themes template list` · `create` · `update` · `delete` | [page-template.md](references/page-template.md) |
| Generate a new AI card (no existing card fits) | `themes block +edit -t <id> --session <oseid> --content <file> --template <name>` | [block/generate-block.md](references/block/generate-block.md) |
| Change an AI card's source / delete it | `themes block +get` → `block +edit --id <gen_id>` · `themes block delete-gen` | [block/edit-block.md](references/block/edit-block.md) · [block/delete-block.md](references/block/delete-block.md) |
| Theme files and their version history | `themes file tree` · `file get/create/update/rename/delete` · `themes version list/records/get` | [files-versions.md](references/files-versions.md) |
| Page-builder (advanced) cards and templates | `themes pb …` | [page-builder.md](references/page-builder.md) |
| Low-level session / card / block calls the shortcuts don't cover | `themes session …` · `themes section …` · `themes block add/remove/set-props` | [raw-leaves.md](references/raw-leaves.md) |

## Workflow — editing a theme page

Every page edit is one stateful flow. Keep `theme_id` and `oseid` for the whole task.

1. **Resolve `theme_id`** (Rules → 1). Pass `-t <theme_id>` on every read and write below.
2. **Open or reuse the session**: `+page -t <id> --template <name>` → `oseid`, card rows
   (`section_id`, `type`, `area`). Reuse an `oseid` from earlier in the conversation with
   `--session`; a `+page` without `--session` opens a NEW, empty session.
3. **Locate**: read the candidate card with `--session <oseid> --section <sid> --include schema`
   → current `settings`, field `schema` (`label`, `options`, `min`/`max`, `visibleOn`), block rows
   with ready-made `target`s. Copy targets verbatim.
4. **Write** into the same session: `+edit --ops` (cards) · `session update-config` (theme
   settings) · `app enable/disable` · `block +edit` (AI cards) → `applied`, `preview_url`.
5. **Verify**: re-read what you wrote (`+page --session … --section …`, `get-config`,
   `block +get`). Report the re-read state, not what you sent.
6. **Stop at the preview**: summarize the change (field `label`s, old → new) and give the
   `preview_url`. Nothing is live yet.
7. **Save** only when asked: `+edit … --session <oseid> --ops '[]' --promote` → `promoted:true`.
   Then ask whether to publish.
8. **Publish** only with consent (dry-run → restate → wait): `+edit … --ops '[]' --promote
   --publish` → `published:true`. With nothing pending, `themes publish` does the same switch.

## Rules for every operation

1. **Which theme.** In order: the theme named in this turn → the theme established earlier in the
   conversation (opened, previewed, queried) → the published theme
   (`themes list --params '{"published":"1"}'`). A theme named without an id: `themes list`,
   then match `name`, then `merchant_theme_name` yourself; several or no matches → ask.
   Omitting `-t` on a later call silently switches to the published theme.
2. **One session per task.** Every `+page` / `+edit` / `update-config` / `app` / `block` call of
   the task carries the same `oseid`.
3. **Card, block, new card, or theme setting?**
   - A named card or page → edit that card, even if a theme setting has the same name. A whole
     class of elements store-wide → theme setting ([global-config.md](references/global-config.md)).
   - Adding an item inside an existing card is a block op on that card; a new section on the
     page is `add_section` ([card-add.md](references/card-add.md)).
   - Neither clear → cards first; the re-read shows no such field → check theme settings; neither
     has it → the theme doesn't support it.
4. **The schema read is the only truth** about what a theme can do. Themes differ widely; match
   fields by their `label` (merchant-facing name), never by guessing keys. Row count vs per-row
   count, and PC vs mobile fields, are separate fields.
5. **Can't do it → say so, offer the real alternatives, wait.** Never invent a field or block
   type, never replace the ask with a near action (hiding or blanking ≠ deleting), never hand-edit
   theme source files to force it. When no existing card can do it, the AI-card flow can
   ([block/generate-block.md](references/block/generate-block.md)).
6. **A store object named in the request is a binding, not a title.** "Show collection X" means
   filling the card's collection-type field with the real object
   ([resource-binding.md](references/resource-binding.md)), not typing X into a heading.
7. **Card copy language** = the storefront's primary-market language (`shop markets list` →
   the primary market → `shop markets list-language`). Text the user gives is written verbatim.
8. **Safety.** Only these need `--dry-run` → restate → wait for consent: `themes publish`,
   `+edit --publish`, `themes delete`, `themes template delete`, `themes block delete-gen`,
   `themes pb delete-template`, `session promote` with `force:true`, and direct `file` writes on the
   published theme
   ([files-versions.md](references/files-versions.md)). Everything else runs directly.

## Acting on a request

"改一下首页 / 换个主题 / 加一张卡片" is an **action**, not a question:

1. Match the intent (trigger table), resolve the theme (Rules → 1).
2. Read before writing (Workflow 2–3); values come from the user or the schema, never invented.
3. Missing a no-default value → ask once, bundled. Never ask about defaulted values.
4. Run it; destructive/publish operations follow Rules → 8.

### Trigger phrase → command

| User says | Command | How to extract values |
|---|---|---|
| 我有哪些主题 / 现在用的哪个主题 / which theme is live | `themes list` | live → `--params '{"published":"1"}'`; count → `.data.count` |
| 预览一下 / 给我预览链接 / preview | `themes +preview` | page → `--path` (`/`, `/products/<handle>`, …); unsaved edits → `--oseid` |
| 首页有哪些模块 / what's on the homepage | `themes +page --template index` | page name → `--template` (`index`, `product`, `collection`, `cart`, …) |
| 把轮播图标题改成… / 改文案 / 换图 / change the banner text | `+page` → `+edit` `replace_props` / `update_slot` | card from wording → `section_id`; field by `label`; value verbatim |
| 隐藏 / 删掉 / 上移 这个模块 / hide / remove / move a section | `+edit` `set_visibility` / `remove_section` / `move_section` | target = the card's `section_id`; move → `position: before:/after:<sid>` |
| 加一个 xx 卡片 / add a testimonials section | `section cards` → `+edit` `add_section` | name/need → a returned `items[].id`; nothing fits → AI card |
| 改主题配色 / 换字体 / change theme colors or fonts | `session get-config` → `update-config` | field by `label`; fonts only from [fonts.md](references/fonts.md) |
| 开启 / 关闭 xx 弹窗 app / turn on an app embed | `themes app list` → `themes app enable` / `disable` | current state and `block_id` from `themes section list` ([apps.md](references/apps.md)) |
| 给某商品单独做个详情页 / custom product template | `themes template create` | type from wording; bindings from real ids |
| 生成一个 xx 卡片 / 自定义卡片 / generate a card | `block +edit` | only when existing cards can't do it |
| 保存 / 发布 / save / publish the changes | `+edit --ops '[]' --promote [--publish]` | publish needs consent |
| 安装 xx 主题 / install theme X | `market list` → `market install` | preset from wording, else `Default` |
| 主题改名 / 复制 / 升级 / 删除 / rename, duplicate, upgrade, delete | `themes rename` / `duplicate` / `upgrade` / `delete` | only the "which theme" half locates the theme |

### Required-vs-ask matrix

| Operation | Must ASK if unspecified | Infer if possible | Default silently |
|---|---|---|---|
| `+edit` value change | a value with no direction ("调一下") — ask for that field only | direction ("更大", "深一点") → pick from `min`/`max`/`options` | other fields stay untouched |
| `add_section` | which card, when several candidates fit | the card from name / need; position from wording (`position: before:/after:<sid>`, `first`) | position (end of area) |
| `themes rename` | the new name | the theme | — |
| `themes duplicate` | — | source theme | `copy_theme_name` is required: send `<source name> copy` unless a name was given |
| `themes upgrade` | — | the theme (`has_newest_version` must be true) | new theme name |
| `themes market install` | which theme when the name is ambiguous | preset from wording | preset `Default`, server-side name |
| `themes template create` | bindings the user named but that can't be resolved | `type` from wording | `from: "default"`, no bindings |
| `themes publish` / `delete` / `template delete` | consent (always) | the theme / template | — |

### Never-ask list

`--area` (default `all`) · `--include` · `from` on template create · duplicate / install / upgrade
names · install preset (`Default`) · `limit` / `page` · which session (reuse the current one).

## Boundaries

| Sounds like themes | Actually belongs to | Command |
|---|---|---|
| 自定义页面 content / 关于我们 page text | `shoplazza-shop` | `shop pages …` (a `page` custom template only changes its layout — [page-template.md](references/page-template.md)) |
| 博客文章 / blog posts | `shoplazza-shop` | `shop articles …` |
| 导航菜单 items / menu entries | `shop menu` | `shop menu list` / `update` (binding a menu to a header card is here) |
| 商品图片 / product images | `shoplazza-products` | `products images …` (card image fields are here) |
| 创建限时促销 / create a flash sale | `shoplazza-discounts` | `discounts +flashsale` (binding it to a card is here) |
| 上传素材 / media library files | `shoplazza-shop` | `shop +upload-file` / `shop files …` (theme `assets/` files are here) |
| 应用市场安装 / install an app from the app store | merchant admin | not a CLI action (turning its theme embed on/off is here) |
| 本地开发主题 / theme dev workflow | `themes init/serve/push/pull` | see `--help` (not covered by this skill) |

## Permissions · Scope

| Operation | Needs | Grant |
|---|---|---|
| read (`list`, `get`, `+page`, `get-config`, `app list`, `file tree/get`, `version *`, `market list`) | `themes` read scope | `auth login --domain themes` |
| write (`+edit`, `update-config`, `app enable/disable`, `template *`, `block *`, `file *`, lifecycle, install, publish) | `themes` write scope | `auth login --domain themes` |

`--domain themes` expands to `read_themes`, `write_themes` and `read_shop`; check with
`shoplazza auth scopes`.

## Gotchas

| Symptom | Cause | Fix |
|---|---|---|
| Earlier edits missing from a read | `+page` without `--session` opened a new session | Always pass the task's `--session <oseid>` |
| Edits landed on the live theme instead of the one being discussed | A call omitted `-t` | Pass `-t <theme_id>` on every call of the task |
| `unknown flag: --theme` | The flag is `-t` / `--theme-id` | Use `-t <theme_id>` |
| Saving included changes from earlier turns | `--promote` saves the whole session | Say so in the summary |
| No confirmation prompt before publish | The y/N prompt only appears on an interactive terminal | Get the user's consent yourself (Rules → 8) |
| `update-config` / `app enable` have no `--promote` | They share the page session | Save / publish with `+edit … --ops '[]' --session <oseid> --promote` |
| `update-config` accepted a wrong value | It stores anything: bad colors, out-of-range numbers, unknown keys | Check the value against the `get-config` schema before writing; re-read after |
| Header / footer / announcement change via `update-config` fails | Global cards are cards | Edit them with `+edit` using their `section_id` (e.g. `header`) as the target |
| `app enable` / `disable` returns a server error | Known to fail for ids taken from `app list` | Report it; read the current state from `themes section list` ([apps.md](references/apps.md)); don't retry with guessed ids |
| `invalid source: gen` from `section cards` | Some backends don't accept that source | Retry without `gen` |
| Guessed a global card name (`cart_drawer`…) that isn't there | Global-area cards differ per theme | Read `+page --area global`; the `section_id` equals the `type` |
| A file change went live at once | `file create/update/rename/delete` bypass sessions; on the published theme they are live | Dry-run → restate → consent first |
| New theme id after duplicate / upgrade / install | Those create a new, unpublished theme | Use the new id for everything after; publish separately |
| `themes duplicate` errored, yet a copy appeared | The copy can be created even when the call returns an error | Never retry blindly — check `themes list` for the copy first |
| `themes delete` refused | The published theme can't be deleted | Publish another theme first |
| Template bindings vanished | `template update` `relations` replaces the whole set | Send the complete list; `template list` only shows a count, so ask for the full set when unsure |
| AI card saved but broken on the storefront | The server only rejects liquid parse errors | Run [block/self-check.md](references/block/self-check.md) before writing |
| `order` / `order_verify` template edits can't be checked | Those pages can't be previewed | Say it isn't supported |

## Recipes

```bash
# Which theme is live?
themes list --params '{"published":"1"}' --jq '.data.themes[] | {id, name, merchant_theme_name}'

# Open a session on a theme's homepage and list its cards
themes +page -t <theme_id> --template index --jq '{oseid: .data.oseid, sections: [.data.sections[] | {section_id, type, area}]}'

# Change one card field, re-read it, then save on request
themes +edit -t <theme_id> --template index --session <oseid> --ops '[{"op":"replace_props","target":"<section_id>","props":{"<field_id>":<value>}}]'
themes +page -t <theme_id> --template index --session <oseid> --section <section_id> --include schema
themes +edit -t <theme_id> --template index --session <oseid> --ops '[]' --promote
```

## References

- Themes & market: [query.md](references/query.md) · [lifecycle.md](references/lifecycle.md) · [market.md](references/market.md)
- Page editing: [page-read.md](references/page-read.md) · [card-edit.md](references/card-edit.md) · [card-add.md](references/card-add.md) · [global-config.md](references/global-config.md) · [apps.md](references/apps.md) · [page-template.md](references/page-template.md)
- Values: [setting-values.md](references/setting-values.md) · [resource-binding.md](references/resource-binding.md) · [fonts.md](references/fonts.md)
- AI cards: [block/generate-block.md](references/block/generate-block.md) · [block/edit-block.md](references/block/edit-block.md) · [block/delete-block.md](references/block/delete-block.md) · [block/block-cli.md](references/block/block-cli.md) · [block/self-check.md](references/block/self-check.md)
- Files & builder: [files-versions.md](references/files-versions.md) · [page-builder.md](references/page-builder.md) · [raw-leaves.md](references/raw-leaves.md)
- Per-command flags: `shoplazza themes <cmd> --help` · params/body: `shoplazza schema themes.<cmd>`
- Cross-cutting mechanics: `../shoplazza-common/SKILL.md`
