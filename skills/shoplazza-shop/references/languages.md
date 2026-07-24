# shop languages — store locales (多语言)

A store language is a locale merchants can enable for storefront visitors and publish to
one or more markets. The standard rollout is a **three-step flow**: add → enable →
publish.

## Flow

```bash
# 0. Find the exact IETF code (e.g. ja-JP) — unsupported codes are SILENTLY DROPPED by add
shop languages supported --jq '.data.languages'

# 1. Add (newly added languages are DISABLED by default)
shop languages add --data '{"codes":["ja-JP"]}'

# 2. Enable (make it visible to storefront visitors)
shop languages enable --params '{"language_code":"ja-JP"}'

# 3. Publish to markets (FULL replacement of the market relations)
shop languages markets                                   # which markets are configurable
shop languages publish --params '{"language_code":"ja-JP"}' --data '{"market_ids":["<mid>",…]}'
```

## All commands

```bash
shop languages list        # languages + enabled state + market relations
shop languages supported   # system-supported codes → .data.languages[] {code,name}
shop languages markets     # configurable markets for publishing
shop languages add     --data '{"codes":["ja-JP","ko-KR"]}'
shop languages enable  --params '{"language_code":"ja-JP"}'
shop languages disable --params '{"language_code":"ja-JP"}'
shop languages publish --params '{"language_code":"ja-JP"}' --data '{"market_ids":[…]}'
shop languages delete  --params '{"language_code":"ja-JP"}'   # --dry-run first
```

## Gotchas

- `add` **silently drops unsupported codes** — verify against `languages supported`
  first, then confirm with `languages list` after adding.
- Newly added languages are **disabled by default** — without `enable`, nothing shows on
  the storefront.
- `publish` is a **full replacement**: send the complete target `market_ids` list every
  time; `"market_ids": []` removes ALL market relations for that language.
- A disabled language can still be published (configured) but takes effect only once
  enabled.
- `delete` removes only the configuration; the translation corpus is retained.

## Boundaries

- Market default language → `shop markets update-default-language` (the language must
  already be published to that market) — see [markets.md](markets.md).
