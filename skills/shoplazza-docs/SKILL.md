---
name: shoplazza-docs
description: >-
  Search and read the Shoplazza developer docs on shoplazza.dev (开发者文档 / 查文档) with
  `shoplazza docs search --query --locale` / `docs get --url`; no login needed. Use when the
  user asks how the Shoplazza platform works or what it allows: app development (应用开发), OAuth and
  access scopes for their own app (应用授权 / 权限范围), verifying webhook HMAC signatures
  (webhook 验签), app proxy (应用代理), checkout / theme / function extensions (结账扩展 /
  主题扩展), theme development and the Liquid reference (主题开发 / Liquid 语法 / 对象 / 过滤器);
  before writing code that calls the Shoplazza API or Liquid; on an unfamiliar API error or field
  (报错 / 字段含义); when upgrading API versions (版本升级 / changelog). Prefer it over WebFetch or
  web search for shoplazza.dev. NOT store data (→ domain skills); NOT a CLI command's params /
  body / response (→ `shoplazza schema` / `--help`); NOT logging the CLI in (→ shoplazza-common);
  NOT webhook subscriptions (→ shoplazza-webhook); NOT editing the store's theme or AI cards
  (→ shoplazza-themes).
allowed-tools: Bash(shoplazza docs *)
---

# shoplazza CLI — developer docs (shoplazza-docs)

**Prerequisite: the shoplazza CLI is installed.** Searching the docs needs no login, store or
profile, so unlike the other skills this one doesn't start by reading
`../shoplazza-common/SKILL.md`. If `shoplazza` is missing (`command not found`), follow only the
"Installation → CLI preflight" part of `../shoplazza-common/SKILL.md` (skip its login), then come
back. Before acting on a store (a domain command or `api rest`), read shoplazza-common as usual.

## Overview

`shoplazza docs` searches and reads the developer docs on www.shoplazza.dev: app and theme
guides, the API reference for every API version, and the changelog, in English and Chinese.
It is online and read-only:

- `docs search --query "<terms>" --locale <zh-CN|en>` → the best-matching passages in that
  language, each with its page's url and title;
- `docs get --url <url>` → one whole page, as Markdown in the `markdown` field.

For anything on shoplazza.dev, use these rather than WebFetch or a web search. Use them when:

- the user asks how the Shoplazza platform works or what it allows: app development, OAuth and
  scopes, webhook signatures, app proxy, checkout / theme / function extensions, theme
  development, Liquid;
- before you write code that calls the Shoplazza API or uses Liquid;
- a call fails with an error, or returns a field, you don't recognize;
- the user upgrades API versions or asks what changed (the changelog).

## Command map

| Intent | Command |
|---|---|
| Find what the docs say about a concept, API, field or error | `docs search --query "<short terms>" --locale <zh-CN\|en>` |
| … in the other language's docs | switch `--locale` |
| … for another API version | add `--api-version 202601` (`2026-01` works too) |
| More or fewer passages | add `--limit <1-20>` (10 when omitted) |
| Read a whole page | `docs get --url <url of a result>` |
| Read a long page in full | `docs get --url <url> --output <new file in the system temp dir>` |

## How to search

When a question is too broad to search well ("商品多规格怎么搞"), search its most likely reading
and say which reading you answered, or ask one clarifying question.

1. **Write a short query, not the user's sentence.** Name the API, field, error code or
   concept: "webhook hmac verify", "app proxy signature", "product-create variants". Measured on
   the real docs: pasting the whole question gets the right API page into the top 5 about half
   the time; a short query, about 9 times in 10.
2. **Always pass `--locale` in the user's language:** `--locale zh-CN` for a user writing
   Chinese, `--locale en` otherwise. Only docs in that language are searched; the query's own
   wording can be in either language.
3. **Stop as soon as the passages answer the question; search 3 times at most.** Rephrase
   between tries: the exact name, fewer words, the other language. After 3 misses, tell the user
   the docs don't seem to cover it and point to the closest pages you did find.
4. **Read `content` first.** Only when the passages aren't enough, `docs get` the top 1–2 pages.
   When a page comes back `truncated: true`, rerun with `--output` set to a new file in the
   system temp dir — never a path in the user's project, since it overwrites without asking —
   and search that file (e.g. `grep`) rather than reading it top to bottom.
5. **Cite and don't invent.** Give the `url` of every page you used. When nothing relevant came
   back, say so — never fill the gap from memory.
6. **Name the API version.** API results are the site's default version unless you pass
   `--api-version`; say which version an answer covers, and search once per version when
   comparing two.

Reading results:

- Results are passages, not pages: one page can appear several times with the same `url`, once
  per matching passage. Read them together before deciding to `docs get` that page, and cite the
  page once.
- A result only means a passage scored above the floor. When none of them is about what you
  searched for, treat the topic as not covered instead of stretching an unrelated passage;
  `results: []` (exit 0, a hint on stderr) says so outright.
- A url whose path contains `/api/` (or `/zh-CN/api/`) is API reference, of two kinds. An
  endpoint page's `content` is JSON starting with `operationId`, `method`, `path`, `apiVersion`,
  where `part` says whether it is the `request` or the 200 response (`response200`). A group
  page's `content` is Markdown listing the group's endpoints. Look at `content` before parsing it
  as JSON.
- An endpoint's request and 200 response come back as separate passages, and a large one is
  split further. When the fields you need aren't in the passages you got, `docs get` the page.
- Changelog questions (does an endpoint still exist in a version, a renamed field, breaking
  changes): add "changelog" to the query. All versions share one page (`/changelog`), so check
  the version heading in each passage; when the passages don't settle it, save the page with
  `docs get --url /changelog --output <temp file>` and search the file.

## docs vs schema

`schema` comes first for anything about the CLI's own commands:

| Question | Use |
|---|---|
| Which command does X; its params, request body, response | `shoplazza schema <module>` lists a module's commands; `shoplazza schema <module>.<command>` shows one. Offline, and exactly what the CLI sends |
| Concepts, business rules, flows, Liquid and theme development, the changelog | `docs search` |
| An API the CLI doesn't cover (e.g. loyalty, payment apps) | `docs search`, then call it with `shoplazza api rest <METHOD> <path>` |

- To act on a store, the tier order still holds: shortcut → spec leaf → `api rest` (see
  `../shoplazza-common/SKILL.md`).
- CLI commands call API version 202601, while `docs search` defaults to the site's newer default
  version. When comparing the docs with a CLI command, pass `--api-version 202601`.

## Boundaries

| Sounds like docs | Actually belongs to | Command |
|---|---|---|
| 写 / 改店铺上的 Liquid 卡片 / put a Liquid card on the store | `shoplazza-themes` | `themes block +edit` (the Liquid reference is here) |
| 订阅 webhook / subscribe to an event | `shoplazza-webhook` | `webhook create` (verifying the signature is here) |
| 登录 CLI / 授权店铺 / log the CLI in | `shoplazza-common` | `auth login` (an app's own OAuth flow is here) |
| 查 / 改店铺数据 / read or change store data | the domain skill | e.g. `orders`, `products` |
| A CLI command's params / body / response | `shoplazza-common` | `shoplazza schema <module>.<command>` |
| Running `themes init/serve/push/pull`, `app`, `checkout-extension`, `theme-extension` | those commands | their `--help` (how the platform features work is here) |

## Permissions · Scope

None. `docs` needs no login, store or profile, and ignores `--profile`. It needs network access
to www.shoplazza.dev and the docs search service; `docs get` reads www.shoplazza.dev pages only.

## Gotchas

| Symptom | Cause | Fix |
|---|---|---|
| The right page isn't in the results | Query too long, or phrased as a question | Short terms with the exact API / field name |
| `results: []`, or no result is about your terms | Nothing relevant enough | Rephrase (3 searches at most), try the other `--locale`, then say the docs don't cover it |
| `validation` error listing API versions | `--api-version` names a version the docs don't have | Pick one from the message |
| `api` error, hint "rate limited; retry in N seconds" | Search quota used up | If N ≤ 60, wait and retry once; otherwise stop and tell the user. Never loop |
| `network` error (exit 4) | Offline, sandboxed, or timed out | Retry once, then tell the user the docs are unreachable; don't answer from memory |
| `docs get` → `api` error with `status_code: 404` | The page moved or was removed | `docs search` again for the current page; if that doesn't help, answer from the passages and say the whole page couldn't be read |
| `truncated: true` | Page longer than `--max-chars` (20000 characters) | `--output` to a temp file, then read it; `total_chars` is the whole page's length |

## Recipes

```bash
# A concept question from an English-speaking user
shoplazza docs search --query "webhook hmac verify" --locale en

# The same from a Chinese-speaking user: Chinese docs, whatever the query's wording
shoplazza docs search --query "webhook hmac 验签" --locale zh-CN

# Compare an endpoint's docs with the CLI command: schema first, then the docs at the CLI's version
shoplazza schema products.create --view request
shoplazza docs search --query "product-create" --api-version 202601 --locale en

# Read a page; save a long one to a temp file and read that
shoplazza docs get --url https://www.shoplazza.dev/docs/app/best-practices/security
shoplazza docs get --url /api/202601/product-create --output /tmp/product-create.md
```

## References

- Flags: `shoplazza docs search --help` · `shoplazza docs get --help`
- The CLI's own commands: `shoplazza schema`
- Installing the CLI: `../shoplazza-common/SKILL.md` → Installation
