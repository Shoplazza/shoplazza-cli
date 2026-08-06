---
name: shoplazza-common
description: >-
  Shared foundation for the shoplazza CLI: authentication (auth login — account-level and
  store-level), auth status/scopes/logout, store switching (auth store use), multi-store
  profiles (add/list/use/rename/remove/info), the three access tiers (+shortcut / spec leaf /
  api rest), the output envelope ({ok,data} / .data), --dry-run / --jq / --fields / --format,
  schema introspection, api rest fallback, update notice, the safety protocol, and how to trust
  a filtered list read (pagination / has_more / cursor, silently-dropped filters, the exact-match
  precision ladder). Read this FIRST on first use of the shoplazza CLI, when logging in /
  authorizing / switching store or profile, on permission errors, when a search or filter looks
  like it returned too much, and before using any domain skill.
---

# shoplazza CLI — shared foundation (shoplazza-common)

This skill is the **foundation** for every domain skill (`shoplazza-discounts`,
`shoplazza-products`, …): it owns all cross-cutting mechanics so domain skills never repeat
them, and every domain skill opens by requiring this file be read first. The authoritative
source for commands/params is `shoplazza <svc> <cmd> --help` and `shoplazza schema` — this
file teaches the mechanics, it does not memorize flags.

**Reply in the user's language.** Skill prose is English, but that never sets your reply
language — mirror the user (中文提问 → 中文回答), including questions you ask back,
write-confirmation restatements, and error explanations. Keep CLI commands, flags, and field
names verbatim (untranslated).

## Access tiers

Start every operation at the **highest tier that fits** and stop as soon as one expresses it:

| Tier | Form | When |
|---|---|---|
| 1. Shortcut | `shoplazza <svc> +<verb> [named flags]` | First choice. Named flags, smart defaults, structured errors. |
| 2. Spec leaf | `shoplazza <svc> <cmd> --params '{…}' [--data '{…}']` | Endpoints with no shortcut; one command per endpoint. |
| 3. api rest | `shoplazza api rest <METHOD> <PATH>` | Raw fallback, full coverage. Only when the first two can't express it. |

In `shoplazza <svc> --help`, entries prefixed with `+` are shortcuts; the rest are spec leaves.

## Output envelope (.data wrapper)

The structured-output contract, uniform across commands:

- Success → stdout: `{"ok":true,"data":<body>}`
- Failure → stderr: `{"ok":false,"error":{"type":…,"message":…}}`, non-zero exit.

**Response bodies nest under `.data`**, so every `jq` path starts at `.data` (e.g.
`.data.products[].id`). Missing the `.data` prefix is the most common extraction mistake.

**Exception — local commands have no envelope.** `auth status` / `auth scopes` /
`profile list` print their object at the top level (no `{ok,data}`, no `--jq` support);
read fields directly (e.g. `logged_in`).

## Common flags

| Flag | Notes |
|---|---|
| `--dry-run` | Print the request that would be sent, **without sending it**. **Always `--dry-run` first** for destructive / batch / money-spending writes. Read commands and shortcuts support it too. |
| `-q, --jq <expr>` | Filter JSON output with a jq expression. **Outputs raw scalars by default** (no surrounding quotes, just a trailing newline) — **do not add `-r`**: cobra parses `-r` as a separate flag and rejects the command. It is a single-string flag. |
| `--fields <f1,f2,…>` | Response field projection on **a few shortcuts only** — verified: `products +search` (comma-separated: `products +search --fields id,title`). **Not universal** — most commands have no `--fields`; check `<shortcut> --help` before using it. The server may still return extra base fields. |
| `--page-limit <n>` | Page size, 1–250, on list shortcuts. **The API default is 10** — see "Reading a filtered list" below. |
| `--format json\|pretty\|table` | Output format, default `json`. Use `json` for scripts/jq, `pretty`/`table` for humans. Global flag. |
| `--profile <name>` | Profile to use for this invocation (see "Profiles"). Global flag. |

Field projection: a few search shortcuts accept `--fields` (verify with `--help`); everywhere else, project/filter with `--jq`.

## Reading a filtered list

A read that returns `{"ok":true,…}` is **not** evidence that the filter or the page you asked
for is the one you got. Two failure modes are silent, and both produce a confident wrong
answer rather than an error:

**1. Truncation — the page default is 10.** List endpoints return 10 records unless
`--page-limit` says otherwise (max 250), with `has_more` / `cursor` marking the rest.
`has_more` may come back `null` rather than `false`, so treat "not `true`" as the end.
Whenever the answer depends on *completeness* — "how many", "which ones", "all of X", any
total you compute yourself — pass `--page-limit 250` and follow the cursor. For a pure count,
prefer the domain's `+count` shortcut over counting rows.

**2. Silently-dropped filters — the API ignores query params it does not recognise.** It
does not 400. A flag whose param name has drifted comes back 200 with the **unfiltered** set,
which reads exactly like "this customer really has that many orders".

So before a filtered read becomes an answer or feeds a write, confirm the filter landed:

```bash
shoplazza <svc> +search --<filter> <value> --dry-run   # 1. which param actually goes on the wire
shoplazza <svc> +count                                  # 2. unfiltered total, for comparison
shoplazza <svc> +count --<filter> <value>               # 3. filtered total — must be < the unfiltered one
```

**A filtered result equal to the unfiltered total is the tell.** Confirm it by spot-checking
that the returned records actually satisfy the filter (`--jq` over the field you filtered on).
If the filter is being dropped, `schema <svc>.<cmd> --view request` lists the params the
endpoint really accepts — reach for the spec leaf or `api rest` with the documented param.

**Precision ladder** — when the endpoint offers more than one way to filter on the same thing,
prefer the narrowest: a dedicated **exact-match** param (often an array, e.g. `customer_emails`,
`skus`, `order_tags`) beats a general **keyword** search, which beats hand-filtering with `--jq`
over a full page. Exact params say what they match; keyword search may match a field you did
not intend.

**Report the basis, not just the number.** When you answer from a list read, say which filter
was applied and what the completeness basis is ("3 of 3, unfiltered total 18"). It makes a
dropped filter visible to the user instead of invisible.

## Schema introspection

Before calling any spec leaf or `api rest`, check the schema to confirm params / body / response
shape — do not guess fields:

```bash
shoplazza schema                              # list all modules
shoplazza schema discounts                    # list a module's commands
shoplazza schema discounts.get                # a command's params / body / response
shoplazza schema orders.create --view request # request side only (--view all|request|response)
```

## api rest fallback

Only when the first two tiers can't express it. Write the **resolved real URL** (not a `{…}`
template):

```bash
shoplazza api rest GET  /openapi/2026-01/products
shoplazza api rest GET  /openapi/2026-01/products/gid_123 --params '{"page_size":10}'
shoplazza api rest POST /openapi/2026-01/discounts/automatic --data @body.json
```

Supports `--params` / `--data` (incl. `-` for stdin, `@file` for a file), `--dry-run`, `-q/--jq`.

**Large integer ids → quote them in JSON.** In `--params` / `--data`, pass a big numeric id as a
**string** (`{"id":"669923000979557666"}`), not a bare number — bare large ints get float-coerced
(→ `6.69…e+17`) and the request is rejected. Applies anywhere an id exceeds ~15 digits
(e.g. `shop redirects`).

## Authentication (auth)

### Login-state preflight

**Before the first store operation in a session, run `shoplazza auth status`** (local, instant,
no envelope). Two independent things must both be true:

- `logged_in: true` — account credentials exist. If `false` → run the login flow below.
- `profiles` non-empty — a store is bound. `logged_in: true` with `profiles: []` still cannot
  reach any store API; ask which store and run `auth login -s <store> --domain <modules>`.

If you skip the preflight, an unauthenticated business command fails fast with a
`validation` error — `"no profile configured"` plus a `hint` naming the fix. Treat that the
same way: don't retry, guide the user through login first.

### Login

Authorization has two layers: **account-level** (`auth login` obtains account credentials) and
**store-level** (exchange a store token for a specific store, stored in a profile).

```bash
# Account-level login (no store bound)
shoplazza auth login --domain products,orders        # or --scope read_product,write_product
# Store-level login (select a store; an interactive login with -s must also pass --scope or --domain)
shoplazza auth login -s my-store.myshoplazza.com --domain discounts
```

- `--domain <module,…>`: authorize by **top-level command name**; the CLI expands each into the
  OAuth scopes that module needs. Options: `billing, checkout, customers, discounts, orders,
  products, shop, themes, webhook, all`. **Preferred** — you don't memorize scope literals.
- `--scope <literal,…>`: fine-grained OAuth scopes (e.g. `read_product,write_product`),
  least-privilege.
- **Re-login REPLACES the grant** (verified live: a 22-scope grant re-authorized with one
  scope came back with one). Topping up permissions? Pass **`--merge-scopes`** to carry the
  prior grant along (older CLIs lack the flag — build the union yourself via `auth scopes`;
  `--domain` and `--scope` combine in one login). Only omit it when the user explicitly
  wants to narrow permissions.

### Agent-driven login flow

When acting as an AI agent to log the user in: run `auth login` in the **background** (it
**blocks and polls** until the user completes authorization or it times out — see
`--poll-interval` / `--timeout`, defaults 2s / 300s), **extract the authorization URL from its
output and hand it to the user**; the command returns on its own after the user authorizes.

**Re-login for a missing scope? Pass `--merge-scopes`.** Scopes are replaced, not merged
(see Login above); requesting only the missing scope drops everything else mid-task. If the
flag is unknown (older CLI), fall back to building the union: `auth scopes` → granted, then
request granted + the new scopes in one login.

### Non-interactive fast path (UAT)

If you already ran `auth login` on another machine and have an account UAT, skip the browser:

```bash
shoplazza auth login --uat <UAT>     # or set env SHOPLAZZA_UAT=<UAT>
```

### Store switching and status

```bash
shoplazza auth status                                                  # auth state / identity / current store
shoplazza auth scopes                                                  # supported scopes + scopes granted to the account
shoplazza auth store use -s another-store.myshoplazza.com [--scope …]  # exchange a token for that store, set as current profile
shoplazza auth logout                                                  # log out of the current store
```

### Handling insufficient permissions

On 401/403 for a write, the error envelope's `hint` says what's missing. Grant it as directed:
`shoplazza auth login --domain <that module>` (or `--scope <missing scope>`), then retry. Run
`auth scopes` first to see what's already granted.

## Profiles (multi-tenant)

A profile = one store execution context (its store token + scopes). Manage multiple stores by
switching profiles:

```bash
shoplazza profile add --name prod-us -s my-store.myshoplazza.com [--scope …] [--use]
shoplazza profile list
shoplazza profile use --name prod-us          # or --previous to switch back to the last one
shoplazza profile info [--name prod-us]       # defaults to the current profile
shoplazza profile rename --from old --to new
shoplazza profile remove --name prod-us
shoplazza profile update --name prod-us --scope …   # adjust a profile's scopes
```

Per-invocation override: the `--profile <name>` global flag beats the `SHOPLAZZA_CLI_PROFILE`
env var and the current profile.

## Update notice

When the CLI detects a new version after running, the JSON output carries a `_notice` field
(with the latest version). **Do not silently ignore it**: after finishing the user's current
request, tell them the current / latest version and offer to update:

```bash
shoplazza update            # update via npm (--check only reports versions, no install)
# or: npm install -g shoplazza-cli@latest
```

(Set `SHOPLAZZA_CLI_NO_UPDATE_CHECK` to disable the check.)

## Safety protocol

shoplazza-cli has **no** lark-style exit-10 high-risk-write gate and no `--yes` confirmation
flag. **Do not invent a confirmation protocol.** The safety model is **`--dry-run` → restate →
wait for the user's go-ahead**:

1. **`--dry-run` first** for destructive (cancel / delete), batch, or fund-moving (billing
   charges) operations: print the request that would be sent, restate it, **then STOP and end
   your turn**. For entity/campaign **creation via a `+shortcut`** (e.g. creating a discount),
   follow the domain skill — `+shortcuts` are structured and safe to run directly once the
   required, money-affecting values are confirmed (never fabricated); `--dry-run` only if the
   user asks.
2. **Consent must be a user message, not your own judgment.** Showing the dry-run is not
   consent; your own review of it is not consent. The real command may only run in a turn
   that comes **after** the user explicitly agrees (e.g. "确认" / "yes / go ahead"). Running
   dry-run and the real write in the **same turn** is a protocol violation even if the
   dry-run output was perfect.
3. **Restate intent** alongside the dry-run: plain language — which store, what action, which
   key params — so the user knows exactly what they are agreeing to.
4. **Do not fabricate money-spending values** (discount amounts / caps / prices) — omit for the
   default or ask; see each domain skill's required-vs-ask.
5. Rehearse writes against **dev / stg** stores only; never practice on prod.

## Command discovery

```bash
shoplazza --help                 # top-level modules
shoplazza <svc> --help           # a module's shortcuts + spec leaves
shoplazza <svc> <cmd> --help     # a command's flags
shoplazza schema <svc>.<cmd>     # params / body / response shape
```
