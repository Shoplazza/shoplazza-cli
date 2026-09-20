# Changelog

## 2.1.0 - 2026-09-20

### Added
- Interactive prompts for missing required flags, humans only. In a real terminal, a command or shortcut left missing a required flag now asks for it instead of failing — a menu when the flag has fixed choices, a text field otherwise — and fills the answer in. Nothing changes for non-interactive callers: an agent, a pipe, a CI run, or `--format json` still gets the one structured `required flag(s) not set` error it always did, so scripts keep failing fast rather than blocking on a hidden prompt. The prompt appears only when both stdin and stderr are terminals; `CI` or `SHOPLAZZA_CLI_NO_INTERACTIVE` in the environment turns it off. The prompt UI is drawn on stderr, so stdout stays a clean result envelope. This covers every shortcut with required flags, `app extension create` (`--type` and `--theme-type` offered as choices, `--name` typed), and the checkout-extension and function commands.
- Fuzzy resource pickers for id flags, humans only. Leave an id flag unset in a terminal and the CLI fetches the matching resources from the server and opens a filter-as-you-type list — you see a readable label (an order number and status, a product title, a theme name, an extension and its version) and the id behind your choice is filled in, so you never hand-copy an internal id. Wired to `orders +refund` / `+ship` / `+update-tracking` (`--order-id`), `products +publish` / `+unpublish` / `+tag` (`--id`), `themes push` / `pull` (`--theme-id`), and `checkout-extension deploy` / `preview` / `undeploy` (pick the extension, then its version). If the lookup returns nothing or the call fails, it quietly falls back to typing the id. Non-interactive callers are unaffected — the id flags stay required exactly as before.
- Automatic output-format detection (gh/docker/kubectl model). `--format` now defaults to `pretty` at an interactive terminal and to `json` when stdout is piped, redirected, in CI, or run with `--no-input`. `json` stays the stable machine contract, so agents and pipes are unaffected; humans get readable, colored output without passing any flag. Pin the default with `SHOPLAZZA_CLI_FORMAT` (use `=json` to force machine output inside a pty); an explicit `--format` always wins.
- `--no-input` — a global flag that turns off every interactive prompt and fails fast on missing input, for scripts and agents (equivalent to `SHOPLAZZA_CLI_NO_INTERACTIVE=1`).
- Theme multi-environment. A `shoplazza.theme.toml` file records named environments (store, theme id, path, ignore), and `-e/--environment <name>` runs `themes push` / `pull` / `serve` / `share` against one — switching store + profile and filling any unset theme/path/ignore flags from it. Manage them with `themes env list` / `show` / `check` / `add` / `set` / `remove`: `add` is interactive and validates before writing, `check` validates offline, and `pull` / `push -e` record their resolved target back into that environment. A `default` environment is auto-applied when present.
- App per-invocation `--config <name>` on `app dev`, `app deploy` and `app function` — run against a specific app config for this one command without switching the active config; the resolved target (config file + `client_id`) is echoed to stderr.
- `app init` and `app config link` now pre-check for a same-named app before creating one, and point you at `app config link` to reuse the existing app (with its `client_id`) instead of silently creating a duplicate.
- Interactive fill and pickers extended across the CLI (humans only): `te connect` (`--client-id`), `te serve` (`--theme-id`), theme-extension `create` / `build`, `profile use` / `remove` / `rename` / `add`, `app config link` (a guided wizard), `app function compile` / `release` (`--name` picked from local extensions), checkout-extension versions, and missing path params on generated API commands. Optional prompts accept a blank answer.

### Changed
- Destructive commands now ask a human to confirm before they run, and only a human. In a terminal you are asked before the write goes out — `orders +refund` has you retype the order id, `products +unpublish`, `themes push`, every dynamic `delete` / `cancel`, `checkout-extension undeploy`, `auth logout` and `profile remove` ask to proceed — and declining cancels with no change made. This is a safety net for interactive use only: agents, pipes, CI and any `--dry-run` proceed exactly as before, with no new flag and no prompt, so no automation or agent workflow is affected.
- Pretty and table output overhauled: TTY-gated color, nested objects indented and long lists flattened to dot-notation, a field-priority order for common keys, and clearer list-vs-object framing that labels a list envelope with its own key. These formats are for reading only — `json` remains the contract and is unchanged.
- Theme dev commands moved to a dedicated `theme` command group (plain Cobra) that owns its `-e` store client; `push` / `pull` / `serve` / `share` and the whole `env` family now live there, and the old theme shortcuts were removed. Account-level store shortcuts are unchanged. `push` / `pull` with `-e` record their target into that environment, and `push` / `pull` print a one-line human summary on success.
- `auth scopes` / `login` / `logout`, `auth store use` and the remaining `PrintJSON` sites now honor `--format`; `themeext serve` writes its banner and progress to stderr so stdout stays a clean JSON envelope.
- Command help rewritten in plain language (implementation jargon dropped), with tightened root-help wording.
- `app dev` next-steps now lead with "your changes are already pushed — just refresh"; the OAuth install URL is demoted to first-install / app-auth only (skip it if the app is already installed); the app help no longer claims "hot reload" (there is no watcher — re-run to apply changes); and the `app config push` hint is split across two lines so it works in every shell, PowerShell included.
- Errors now follow the same audience split as data output. At a `pretty`/`table` terminal a failure prints a readable `Error:` line — with the hint, and the failing endpoint / request id when the server returned them — instead of a raw JSON blob. Piped, in CI, with `--no-input`, or in any `json`/`ndjson`/`csv` mode, stderr still carries the `{"ok":false,"error":{…}}` envelope, so agents and scripts keep parsing errors as JSON. Both conditions must hold for the human line (a human-oriented format **and** a terminal on stderr), so redirecting stderr always yields the JSON envelope.
- `--jq` now explains an empty result. When a filter selects nothing — a bare `null` or no output, e.g. `.request.path` on a real call (that field exists only under `--dry-run`) — a one-line hint is printed to stderr at a terminal: data lives under `.data`, and `.request` is `--dry-run`-only. stdout still carries the raw jq output verbatim, and a piped/redirected stderr sees no hint, so scripts and agents are unaffected.

### Fixed
- `--no-input` recognizes every truthy pflag form (`--no-input`, `--no-input=true`, and the rest), not just the bare flag.
- Pretty/table truncation is rune-aware, so multi-byte text is no longer cut mid-character; CSV and NDJSON pick the dominant object-list consistently with the other formats.
- `themes env add` / `set` / `remove` reject an empty or whitespace-only name (which previously fell through to `default`), and a malformed `shoplazza.theme.toml` no longer aborts commands run without `-e`.
- The skills drift-lint reads the grouped root help, not only an `Available Commands:` block, so `make skills-lint` no longer fails on the regrouped help surface.
- Interaction deep-audit fixes: dry-run auth handling, destructive-confirm edge cases, and clearer structured errors from `app init`.

## 2.0.13 - 2026-09-11

### Added
- `themes serve --skip-push` — watch against the theme as it already is on the server, without the startup full upload. That upload rebuilds the remote file tree from the zip, so it removes anything the server holds that the local directory does not (files excluded by `.themeignore` included) and overwrites whatever the online Theme Editor wrote, `config/settings_data.json` among it; `--skip-push` leaves all of that in place and sends only the files changed while serve runs. Because nothing reconciles the two trees, serve reports how many files exist on only one side. The flag is refused where it cannot be honored: alongside `--task-id` (the task it waits for is itself a full upload), and in development mode before a development theme exists for the store (creating one is a full upload).

### Changed
- `themes serve` (first run, development mode) and `themes share` print the upload as separate steps — `packaging theme files`, `uploading <zip> (<bytes>)`, `upload task <id>`, `waiting for the server to process the theme` — instead of one `creating development theme` / `uploading and processing theme` line. The task id is now visible while waiting, so an interrupted first run can be resumed with `--task-id`, the same as `push`.

## 2.0.12 - 2026-09-10

### Added
- `themes push --task-id` / `themes serve --task-id` — resume waiting for an earlier upload task instead of packaging and uploading again. In development mode `serve` reads the new theme id from the task, saves it to `.shoplazza/theme-state.json` and renames the theme, so a run that was cut off no longer leaves an orphan theme and a second run no longer creates another one. The timeout error and a Ctrl-C during the wait both report the `task_id` and the exact resume command; before, Ctrl-C exited silently and the timeout only said to "query status manually".

### Changed
- `products +set-variants --dry-run` reads the product and prints the exact request body it would send, plus the same `{created, inherited, carried_over, deleted, deleted_detail}` summary the live run returns (under `summary` in the dry-run envelope). The preview used to be a placeholder description string, which could not show which variants the full-replace write would delete — the one thing worth reviewing before confirming. The read is the only call it makes; nothing is written.
- The theme upload task wait (`push`, `serve`, `share`) allows 10 minutes instead of 3. The 3-minute cap was copied from v1 as "3 × 60" but v1 polled 180 times at 3s (≈9 min); large themes were reported as failed at the 3-minute mark while the server was still processing them. The `waiting for the server` progress line now shows the task id.
- `themes serve` names the development theme it creates `Development - <theme name>`, as the help text always promised. The upload endpoint names a theme after `theme_info`, so the rename is a separate request; when it fails the theme keeps its uploaded name and a warning is printed.

### Fixed
- `products +set-variants` no longer discards prices and skus when the set of spec dimensions changes. Dropping or adding a dimension used to abandon variant matching entirely, so every combination counted as new: surviving ones were repriced to `--price`, their skus were cleared, and a call that only deletes a dimension was still forced to supply a price it would then apply. Combinations are now matched to the old variants that share their values on the dimensions that stayed, and inherit their price; the sku comes along while that remap is 1:1 (dropping a dimension, or adding one with a single value) and is left unset when a dimension with several values would put one sku on several variants — `--sku-template` covers that case. `--price` is now required only for combinations that inherit nothing. Variant ids still do not survive a dimension change, so stock and images are still reset; `deleted_detail` now reports each old variant's `price` alongside its sku and stock, and the summary counts the rebuilt-but-inherited ones under `carried_over`.
- `themes serve` retries a file sync that hit a transient failure (5xx, 429, a gateway-level 404, a dropped connection or a client-side timeout) twice with a short backoff before marking the file unsynced; previously a single gateway hiccup left the file out of sync until it was edited again. Business 4xx responses are still reported immediately.
- The upload task poll treated a client-side timeout or a gateway-level 404 as fatal and aborted the whole push; both are now tolerated like any other transient error (up to five in a row).
- Progress lines wider than the terminal window used to print a new row on every refresh; the frame is now redrawn in place across the rows it wraps onto.
- Flag help rendered a backticked phrase as the value placeholder (`-t, --theme-id shoplazza themes list`); descriptions now show `--theme-id string` with the phrase in quotes. Affected `themes pull`, `themes push` and `themes serve`.

## 2.0.11 - 2026-09-07

### Added
- `app config push` — sync the `[dashboard]` section of the active `shoplazza.app.toml` (`name`, `app_url`, `redirect_url`, `embed`) to the Partner dashboard, so App URL / Redirect URL changes no longer require clicking through the dashboard. Patch semantics: only fields with a value are sent, and an empty or removed line never clears the dashboard value (`embed = false` is a value; remove the line to leave it alone). Only draft and rejected apps are pushed without `--yes`; any other status (submitted, in review, published, unpublished, or one this build does not recognise) requires it, because the backend write also refreshes the app's review checks. `name` is synced too, so run `app config link` first if the app was edited in the dashboard since. The output echoes the app as stored by the backend.
- `app config link` now also pulls `app_url` / `redirect_url` / `embed` into a `[dashboard]` section, and writes back to the active config file when it already points at the linked app (an `app init` project keeps its base `shoplazza.app.toml` instead of gaining a second file). `app init` records the app name under `[dashboard]`.
- `app dev --write-urls` — write this session's tunnel App URL / Redirect URL into the active config's `[dashboard]` (local file only; default off). Next steps now point at `shoplazza app config push` instead of manual dashboard configuration.
- `app info` shows `app_url`, `redirect_url`, `embed` and `status` when the backend returns them.

### Changed
- `shoplazza.app.toml` gains a `[dashboard]` table for the fields that sync to the Partner dashboard; top-level keys (`client_id`, `partner_id`, `scopes`) stay local. Writing the file merges one level deep, so updating one `[dashboard]` key keeps the others, and a fixed comment above the section is regenerated on every write. Older CLI versions read and preserve the section untouched.

## 2.0.10 - 2026-08-14

### Added
- `products +set-variants` — edit a product's option matrix (spec dimensions × values) without hand-writing the variants array. `--option "Name:v1,v2"` carries the payload, `--action add|remove|update` the verb; the CLI merges the delta into the current matrix, expands the cartesian product, and submits one request. While dimension names are unchanged, existing variants are matched by option values and keep their id — the API then preserves their sku/stock/image; a dimension change rebuilds every variant and lists each deleted one (id/sku/inventory) in the output. Output is a bounded summary, never the full product, so a 270-variant matrix no longer overflows agent tool-result limits. `--price` applies to newly created variants only; `--sku-template "T-{Color}"` renders per-combo skus. Multi-spec creation is two steps: `+create` (draft), then `+set-variants --action add`.
- `orders refunds finish` — finish a custom-channel refund (new `order-refund-finish` endpoint in the command registry).
- `shoplazza update` now also refreshes installed Agent Skills, and `shoplazza doctor` reports their install state.

### Fixed
- Login polling now treats every 5xx as transient. The session endpoint answers with 200 or a 4xx verdict; a 5xx is only ever infrastructure in between (and the CDN replaces its body), so polling continues until the login deadline instead of aborting on a gateway hiccup.
- `products +create --price -5` was accepted and sent to the API; every price flag now shares the same parser and rejects negative values.
- `products +set-variants` hardening: a dimension that ends up with no values now refuses the update instead of submitting an empty variants array (which would delete every variant); `--sku-template` placeholders match dimension names case-insensitively in rendering, matching validation (a case-mismatched placeholder used to overwrite every sku with the same literal); variant matching follows the server's real option positions, so non-contiguous positions no longer force a spurious rebuild.

## 2.0.9 - 2026-08-06

### Fixed
- **Search filters that never reached the API.** `orders +search --keyword` sent a `query` parameter the orders endpoint does not accept. Unrecognised query params are dropped server-side rather than rejected, so the request came back `200` with **every order in the store** — a wrong answer shaped exactly like a right one (a search for one buyer's email returned all 18 orders of a test store, including other buyers'). It now sends `keyword`. `customers +search --phone` had the same defect, sending `phone` where the endpoint documents `contact`; it now filters instead of returning the first page of the whole customer list.

### Added
- `products +set-price --product-id` and `products +stock --product-id` — resolve a single-variant product's variant automatically; a multi-variant product is refused with its candidates listed, so merchants never need to hunt for internal variant ids.
- `orders +search --email` and `orders +count --email` — exact customer-email match (`customer_emails`), the precise path for "which orders did this buyer place". Prefer it over `--keyword`, which fuzzy-matches order number, customer name and email alike.
- `orders +count` also gains `--keyword` and `--customer-id`, so it accepts the same filters as `orders +search`. The two could previously disagree: `+search` narrowed by customer, `+count` had no way to.
- `auth login --merge-scopes` — also re-requests every previously granted scope. Authorization replaces the account's granted set server-side, so a narrow re-login (say, just `read_inventory`) drops every other scope and trims every profile with it, breaking parallel tasks mid-flight (verified: a 22-scope grant re-authorized with one scope came back with one). With the flag, the requested scopes are merged with the prior grant, the summary notes how many were carried over, and the store profile records the full effective scope set so lazily re-minted tokens keep the merged reach. Default behavior is unchanged.

### Removed
- `customers +search --fields`. The customers list endpoint documents no `fields` parameter, so the flag was discarded in transit and the full record came back regardless — it only ever looked like it worked. Project with `--jq` instead. `products +search --fields`, where the endpoint does document the parameter, is unaffected. **Breaking**: an invocation that passed `--fields` used to exit 0; it now fails with `unknown flag`.

## 2.0.8 - 2026-07-29

### Added
- **Remote API metadata updates** — the command tree now updates without a CLI release. New API operations reach you as soon as they are published: the CLI checks a small manifest in the background (at most once every 24h), downloads a new spec only when one is on offer, and verifies it (sha256, size caps, canonical revision) before adopting it. Every failure is silent and non-fatal — the CLI keeps whatever spec it already had, and a corrupt cache falls back to the copy embedded in the binary.
- `shoplazza update` now refreshes the metadata as well as the binary. The two halves are independent: a missing or failing `npm` still refreshes metadata, and `--check` stays read-only. The response reports `meta_updated` and `meta_revision`.
- `shoplazza doctor check` reports where the active spec came from: `source=embedded|cached`, its revision, and when the last check completed.
- Two env knobs: `SHOPLAZZA_CLI_NO_META_UPDATE` disables metadata refreshes entirely; `SHOPLAZZA_CLI_META_ORIGIN` points them at a non-default environment.

### Changed
- `doctor check` no longer reports things that are not broken. Directories created on first use are not a finding, and `auth_locks_dirs` now fails only when a write would actually fail — including the case it used to miss, where the config directory itself refuses writes and so the lazy creation cannot succeed either.
- Module path is now `github.com/Shoplazza/shoplazza-cli/v2`. Affects code importing these packages, not CLI usage.
- Embedded API spec regenerated.

### Removed
- The `migration_residue` check in `doctor`. One of its two branches repeated what `config_version` already reports; the other flagged a leftover v1 `auth.json` — a file that stops nothing — and dragged the overall verdict to `ok: false` for it.

## 2.0.7 - 2026-07-10

### Added
- **Multi-tenant profiles** — persisted per-store execution contexts. One logged-in account can manage many stores and switch between them without re-authenticating. New `shoplazza profile` command group: `add`, `list`, `show`, `use` (with a `--previous` toggle), `update`, `rename`, `remove`. Select a profile per invocation with `--profile` or `SHOPLAZZA_CLI_PROFILE`; resolution order is flag → env → current profile.
- **Per-profile scope subsets** — `profile add --scope` / `auth login --scope` mint a store token limited to a subset of the account's granted scopes (validated server-side).
- `auth status` now reports the current profile, store, scopes, and token status (`valid`/`expired`/`absent`); `doctor` gains config-version and profile-directory checks; shell completion for `--profile` and `profile use --name`.

### Changed
- **Store context is now a profile, not a single global store.** `auth login -s <store>` and `auth store use` create or switch the active profile automatically (re-login keeps existing profiles; switching accounts clears the old account's profiles). `app`, `checkout`, and `theme-extension` commands resolve their store through the current profile; `theme-extension -s <domain>` still works ad-hoc (ephemeral, non-persisted) for a store without a profile.
- Credentials and config moved to a v2 layout (namespaced keychain keys + per-profile metadata). **Existing installs migrate automatically on first run** — v1 files are preserved and a `config.json.v1.bak` backup is written.

### Removed
- The v1 single-store config fields (`store_domain`, `current_account`), superseded by profiles (handled transparently by the auto-migration).

### Fixed
- **First store command after a fresh login no longer fails with `no UAT available`** — login now persists the account token under the keychain key the profile execution path reads (as well as the legacy key the older store/app flows read), so `login → any store command` works end-to-end.

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
