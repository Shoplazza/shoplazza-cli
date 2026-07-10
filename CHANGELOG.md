# Changelog

## 2.1.0 - 2026-07-10

### Added
- **Multi-tenant profiles** — persisted per-store execution contexts. One logged-in account can manage many stores and switch between them without re-authenticating. New `shoplazza profile` command group: `add`, `list`, `show`, `use` (with a `--previous` toggle), `update`, `rename`, `remove`. Select a profile per invocation with `--profile` or `SHOPLAZZA_CLI_PROFILE`; resolution order is flag → env → current profile.
- **Per-profile scope subsets** — `profile add --scope` / `auth login --scope` mint a store token limited to a subset of the account's granted scopes (validated server-side).
- `auth status` now reports the current profile, store, scopes, and token status (`valid`/`expired`/`absent`); `doctor` gains config-version and profile-directory checks; shell completion for `--profile` and `profile use --name`.

### Changed
- **Store context is now a profile, not a single global store.** `auth login -s <store>` and `auth store use` create or switch the active profile automatically (re-login keeps existing profiles; switching accounts clears the old account's profiles). `app`, `checkout`, and `theme-extension` commands resolve their store through the current profile; `theme-extension -s <domain>` still works ad-hoc (ephemeral, non-persisted) for a store without a profile.
- Credentials and config moved to a v2 layout (namespaced keychain keys + per-profile metadata). **Existing installs migrate automatically on first run** — v1 files are preserved and a `config.json.v1.bak` backup is written.

### Removed
- The v1 single-store config fields (`store_domain`, `current_account`), superseded by profiles (handled transparently by the auto-migration).

## 2.0.6 - 2026-07-07

### Changed
- **`auth login --domain app`** now expands to the app-extension development scopes — `read_themes write_themes` (plus `read_shop`, which the themes module implies for theme previews) — covering themes, checkout, and theme-extension uploads. Previously it granted the app template's install scopes (`read_customer write_cart_transform`).

### Fixed
- **Partner token no longer dropped by routine logins** — a store-scoped or `--uat` login of the same account carries no partner token, but it used to wipe the stored one, forcing an interactive re-login for every `app` command. It is now preserved for the same account and cleared only on an account switch or explicit logout.

## 2.0.5 - 2026-07-03

### Added
- **`auth login --domain app`** — expands to the app template's default install scopes (`read_customer write_cart_transform`), so you can authorize a test store with exactly what a scaffolded app requests without spelling the scopes out. Complements the existing API-module domains (`products`, `orders`, …).

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
