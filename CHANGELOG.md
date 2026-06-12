# Changelog

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
