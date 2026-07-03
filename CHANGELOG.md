# Changelog

## [Unreleased]

### Fixed
- **Windows login/keychain failure** — resource-scoped tokens (`store:<domain>`, `app:<clientID>`) were written to files whose names kept the `:`, which is illegal in Windows filenames; login/store selection failed with `keychain Set: rename: … The parameter is incorrect.`. The `:` is now sanitized to `_`, so the on-disk name is valid on all platforms. (Existing entries on macOS/Linux are re-created on next login.)

## 2.0.4 - 2026-07-03

### Added
- **`app deploy` v1 extension compatibility** — recognizes legacy `extension.config.json` extensions, handles the nested `theme-app/` theme layout, warns when an extension's config names a different app than the deploy target, and migrates v1 configs to `shoplazza.extension.toml` on deploy (marking the old JSON deprecated rather than deleting it).
- `app config link` now auto-activates the linked config — no separate `app config use` step needed.
- Analytics endpoints gain a `filter_crawler_type` param to exclude known bot/crawler traffic from statistics (`no_filter_crawler` default / `official_crawler`).

### Changed
- `theme-extension connect` no longer needs `--partner`: it derives the app's partner from `--client-id` (consistent with `theme-extension release` and `app config link`). The `--partner` flag was removed.
- `auth status` / `auth login` always include `current_store` (empty `""` when no store is selected), consistent with `granted_scopes`.
- Clearer analytics param descriptions — `begin_time`/`end_time` spell out the string Unix-timestamp format, and the `filter`/`filters` params document their operator/value rules and supported keys.

### Fixed
- `auth login --store-domain` now validates the store at login: an invalid or inaccessible store yields a clear warning and is not set as the current store, instead of surfacing later as a confusing `404 store_not_found`. Login itself still succeeds.
- `auth login` no longer prints the store warning twice (stderr summary only, not repeated in the JSON).

## 2.0.3 - 2026-06-26

Products shortcut hardening.

### Added
- **`products +tag`** — add, remove, or replace a product's tags without clobbering the others (`--add` / `--remove` / `--set`).

### Changed
- **`products +set-price` redesigned** — target by `--variant-id` (exact) or `--sku`; a SKU that matches multiple variants is refused with the candidates listed, `--all` updates every matching variant, and giving both cross-checks that they agree. The old `--product-id` was removed. Prevents the previous silent mass price update.
- `app config link` writes the template default scopes (`read_customer write_cart_transform`) when neither the Dashboard nor the config supplies any (matching `app init`).
- Removed the redundant `products collections +create` shortcut — the generated `products collections create` already accepts `product_ids` inline.

### Fixed
- **`products +search` / `+count` filters** — `--published` sends the correct `published`/`unpublished`/`any` enum (`true`/`false` accepted as aliases, invalid values error); `+search --vendor` uses the correct `vendors` param; removed flags the endpoints don't support (`+search --tags`, `+count --vendor`), which were previously ignored silently.

### Notes
- `products +stock`: verified against staging that the inventory API only adds and cannot reduce stock, so `--set` correctly refuses a decrease. Behavior unchanged.

## 2.0.2 - 2026-06-18

### Added
- **Automatic update check** — the CLI checks npm for a newer version in the background and notes it on stderr; `shoplazza update` now checks first, shows live progress, and reports the before/after version. Skipped in CI.
- Version output now includes the build date.

### Changed
- Checkout build toolchain: replaced the deprecated `jscodeshift` dependency with `acorn` + `magic-string` in the HTML-inline step (smaller install, fewer transitive deps); bundle output verified unchanged via golden tests.

### Fixed
- `auth status` always shows `granted_scopes` (`[]` when empty) instead of omitting the field.
- `discounts +rebate` rejects combining `--target order` with product-scope flags locally, with a clear error instead of an opaque server 422.
- Shortcut commands reject stray positional args (catching space-separated-instead-of-comma mistakes) with a helpful hint.

## 2.0.1 - 2026-06-12

Maintenance release (packaging); no functional changes.

## 2.0.0 - 2026-06-12

First public release of the v2 CLI — a full rewrite in Go (the previous v1 was JavaScript).

### Added
- **Full OpenAPI command coverage** — every Shoplazza Open Platform REST endpoint now has a matching CLI command, generated from OpenAPI metadata (products, discounts, orders, customers, billing, shop, themes, webhook, …).
- **Three-layer command system** — shortcuts (`+…`, human/AI-friendly) → API commands (1:1 with endpoints) → raw `api rest` calls for full coverage.
- **20+ shortcut commands** for high-frequency operations — e.g. products `+search`/`+publish`, discounts `+rebate`/`+flashsale`/`+percent-code`/`+bxgy-code`, orders `+ship`/`+refund`/`+update-tracking`.
- **Schema introspection** (`schema`) — inspect any service's methods, parameters, required scopes, and response shape.
- **OAuth login** with OS-native keychain credential storage and automatic token refresh; non-interactive UAT login for CI.
- **`checkout build` / `checkout dev`** — build and hot-reload checkout extensions with a bundled Vite/Node toolchain.

### Upgraded — developer workflow commands
- **App lifecycle** — `app init`, `app dev` (dev server + HMR), `app deploy`, `app extension create` (checkout / theme / function), `app function compile/release/list`, `app versions`, `app config`.
- **Theme development** — `themes init`, `themes serve` (live reload), `pull`, `push`, `package`, `share`.

### Notes
- **Node.js >= 14.18.0** is required for `checkout build` / `checkout dev` (the CLI shells out to Node); both detect Node and fail with a clear hint otherwise.
- The npm package downloads the prebuilt binary for your platform on install. To build from source, use `make install` (Go 1.24+).
