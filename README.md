# shoplazza-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/shoplazza-cli.svg)](https://www.npmjs.com/package/shoplazza-cli)

[中文版](./README.zh.md) | [English](./README.md)

The official [Shoplazza Open Platform](https://www.shoplazza.dev/) CLI tool — built for humans and AI Agents. Develop apps and themes, and manage products, discounts, orders and customers, with structured output designed for AI Agent integration.

[Install](#installation--quick-start) · [Auth](#authentication) · [Development](#development-workflows) · [Commands](#three-layer-command-system) · [Agent Plugin & Skills](#ai-agent-plugin--skills) · [Advanced](#advanced-usage) · [Contributing](#contributing)

## Why shoplazza-cli?

- **Agent-Native Design** — Structured JSON output out of the box; AI Agents can operate Shoplazza stores with zero extra setup
- **Agent Skills Included** — One plugin or one command installs [skills](#ai-agent-plugin--skills) that teach AI agents this CLI's commands, safety rules, and per-domain gotchas
- **E-Commerce Focused** — Products, Discounts, Orders, Customers with full CRUD and 30+ shortcut commands for high-frequency operations
- **Full Developer Workflow** — App creation, extension scaffolding (checkout / theme / function), local dev server over an auto tunnel, one-command deploy; plus theme init, live reload, multi-environment, and packaging
- **Secure & Controllable** — Encrypted local credential storage, automatic store-token refresh, `--dry-run` previews, and a confirmation prompt before destructive commands
- **Interactive When You Need It** — Missing required flags are prompted for and id flags open a fuzzy picker; scripts and agents never see a prompt
- **Three-Layer Architecture** — Shortcuts (human & AI friendly) → API Commands (OpenAPI-synced) → Raw API (full coverage)
- **Up and Running in 3 Minutes** — Interactive login, from install to first API call in 3 steps

## Features

| Domain | Capabilities |
|--------|-------------|
| 🛍️ Products | Catalog, variants, inventory, collections, images, gift cards, reviews, locations, suppliers & procurement |
| 🏷️ Discounts | Automatic and code discounts, flash sales, coupon campaigns |
| 📦 Orders | Orders, fulfillment, refunds, transactions, risk, post-sales, shipping zones, tracking carriers |
| 👤 Customers | Customer profiles, addresses, marketing subscription |
| 🏪 Shop | Shop info, blogs & articles, pages, menus, files, metafields, markets, languages, redirects, carrier services, analytics |
| 💳 Billing | Application charges: one-time, recurring, usage-based |
| 🔔 Webhooks | Webhook subscription CRUD |
| 🎨 Themes | Local dev loop with live reload and multi-environment targeting; store themes (market, publish, files, versions); page, card & AI block editing |
| 🧩 App | Full lifecycle: init → extension create → dev → deploy; extensions: checkout, theme, function |
| 🧱 Extensions | Standalone `checkout-extension` and `theme-extension` projects: scaffold, dev / serve, version, deploy / release |

Most domains also carry `+` shortcuts on top of their generated commands — run
`shoplazza <domain> --help` to see the ones it has.

## Installation & Quick Start

### Install

| Method | Command | Notes |
|--------|---------|-------|
| **npm** | `npm install -g shoplazza-cli` | Recommended. Requires Node.js `>=16`. Self-updates via `shoplazza update`. |
| **Shell script** (macOS / Linux) | `curl -fsSL https://raw.githubusercontent.com/Shoplazza/shoplazza-cli/main/install.sh \| bash` | Auto-detects OS and architecture. |
| **Download binary** | See [GitHub Releases](https://github.com/Shoplazza/shoplazza-cli/releases) | Manual download for any platform. |
| **Build from source** | `git clone ... && cd shoplazza-cli && make install` | Requires Go `v1.24`+. Installs to `~/.local/bin`. |
| **Homebrew** (macOS / Linux) | `brew install Shoplazza/tap/shoplazza-cli` | Auto-updates via `brew upgrade`. |

<details>
<summary>Build from source</summary>

Requires Go `v1.24`+. Installs to `~/.local/bin` (no `sudo` needed):

```bash
git clone https://github.com/Shoplazza/shoplazza-cli.git
cd shoplazza-cli
make install
```

> If `~/.local/bin` is not on your `PATH`, add it: `export PATH="$HOME/.local/bin:$PATH"`.
> For a system-wide install: `sudo make install PREFIX=/usr/local`.

</details>

### Quick Start (Human Users)

> **Note for AI assistants:** If you are an AI Agent helping the user install, jump directly to [Quick Start (AI Agent)](#quick-start-ai-agent).

```bash
# Log in (replace with your store domain)
shoplazza auth login --store-domain my-store.myshoplazza.com --domain products,orders

# Verify
shoplazza auth status

# Start using
shoplazza products list --format table
```

### Quick Start (AI Agent)

Start by installing the [Agent Skills](#ai-agent-plugin--skills), as a plugin for your agent or with the command below — they give the Agent everything it needs: installation, the authorization walkthrough, and how to drive the commands.

```bash
npx skills add Shoplazza/shoplazza-cli -g
```

Then send your Agent this line — it walks you through authorization:

```text
/shoplazza-common log me in to <store-domain>
```

## Authentication

| Command | Description |
|---------|-------------|
| `auth login` | Account-only OAuth — opens browser, obtains UAT |
| `auth login --store-domain <domain>` | OAuth + store token (requires `--scope` or `--domain`) |
| `auth store use --store-domain <domain>` | Switch current store |
| `auth logout` | Sign out and remove credentials |
| `auth status` | Show current auth state |
| `auth scopes` | List available and granted scopes |

```bash
# Interactive login with store
shoplazza auth login --store-domain my-store.myshoplazza.com --domain products

# UAT fast-path (non-interactive, for CI)
shoplazza auth login --uat <user-access-token>

# Top up permissions — re-login REPLACES the grant, so carry the prior scopes along
shoplazza auth login --domain discounts --merge-scopes

# Switch store
shoplazza auth store use --store-domain another-store.myshoplazza.com

# Check status
shoplazza auth status
```

Credentials (the account UAT and store tokens) are encrypted with AES-256-GCM and kept under your user config directory (`shoplazza-cli/`) with owner-only permissions.

### Multi-store profiles

A profile is a per-store execution context (store token + scopes). One logged-in account can
manage several stores and switch between them without re-authenticating — `auth login -s` and
`auth store use` create/switch profiles automatically, or manage them directly:

```bash
shoplazza profile add --name prod-us -s my-store.myshoplazza.com --use
shoplazza profile list
shoplazza profile use --name prod-us        # or --previous to toggle back
shoplazza products list --profile prod-us   # per-invocation override, no switching
```

## Development Workflows

### App Development

The CLI covers the full app lifecycle: create, configure, develop, and deploy.

```bash
# 1. Create a new app project (creates a sub-directory; add --partner <id> if your account has several partners)
shoplazza app init --name "My App"

# 2. Add extensions (theme / checkout / function)
cd my-app
shoplazza app extension create --type checkout --name my-checkout
shoplazza app extension create --type theme --name my-theme --theme-type basic
shoplazza app extension create --type function --name my-fn

# 3. Local development (auto tunnel; re-run to apply changes) — store from the active app config
shoplazza app dev

# 4. Deploy all extensions
shoplazza app deploy

# 5. View deployed versions
shoplazza app versions
```

<details>
<summary>Additional app commands</summary>

```bash
shoplazza app list                              # List apps in your account
shoplazza app info                              # Print app and extension info
shoplazza app config use --config alt           # Switch active app config to shoplazza.app.alt.toml (persistent)
shoplazza app dev --config staging              # Run against one config for this invocation only (not persisted); dev/deploy/function too
shoplazza app config link --client-id <id>      # Link an existing app (pulls its dashboard settings into [dashboard])
shoplazza app config push                       # Push [dashboard] (name / app_url / redirect_url / embed) to the Partner dashboard
shoplazza app dev --write-urls                  # Also record the tunnel URLs in [dashboard], then `app config push` them

# Function extensions (compile/release individually)
shoplazza app function compile --name my-fn
shoplazza app function release --name my-fn
shoplazza app function list
```

</details>

<details>
<summary>Standalone extension projects (checkout / theme)</summary>

```bash
# Checkout extension: scaffold, dev server with HMR, upload a version, then activate it
shoplazza checkout-extension init --name my-checkout-ext --extension banner
shoplazza checkout-extension dev --extension-name banner
shoplazza checkout-extension push --name banner
shoplazza checkout-extension deploy --extension-id <id> --version 1.0

# Theme extension (alias: te): scaffold, live preview, build a version, link to an app and release
shoplazza theme-extension create --name my-te --type embed
shoplazza te serve --theme-id <theme-id>
shoplazza te build --version 1.0.0 --description "initial release"
shoplazza te connect --client-id <client-id>
shoplazza te release
```

</details>

### Theme Development

The CLI provides a complete theme development workflow with live reload.

```bash
# 1. Scaffold a new theme from the Nova-2023 template
shoplazza themes init --name my-theme

# 2. Start the dev server (auto-creates a development theme, live reload)
cd my-theme
shoplazza themes serve

# 3. Pull / push / package
shoplazza themes pull --theme-id <theme-id>
shoplazza themes push --theme-id <theme-id>
shoplazza themes package

# 4. Upload as a preview
shoplazza themes share

# 5. AI-generated blocks: read a page (returns an edit session id), then write a block file and place it in one call
shoplazza themes +page --template index
shoplazza themes block +edit --session <oseid> --content card.liquid --template index --target <section_id>.blocks
shoplazza themes block +get  --session <oseid> --id gen_1a0d523 --section <section_id>
```

<details>
<summary>Theme environments (multi-store / staging)</summary>

Record named environments (store, theme id, path, ignore) in `shoplazza.theme.toml` and target one with `-e`:

```bash
shoplazza themes env add staging --store <domain>   # prompts for what's missing; validates before writing
shoplazza themes env list                    # list configured environments
shoplazza themes env check                   # validate all environments offline
shoplazza themes push -e staging             # run against that environment (store + theme + path)
shoplazza themes pull -e staging             # records its resolved target back into staging
```

A `default` environment is auto-applied when present; `env set` / `env remove` edit and delete entries.

</details>

## Three-Layer Command System

The CLI provides three levels of granularity, covering everything from quick operations to fully custom API calls.

### 1. Shortcuts

Prefixed with `+`, designed to be friendly for both humans and AI, with smart defaults and structured output.

```bash
shoplazza products +search --keyword "shirt"
shoplazza discounts +percent-code --target order --percent 20 --code SAVE20
shoplazza orders +ship --order-id <order-id> --tracking <tracking-no>
```

Run `shoplazza <domain> --help` to see the shortcuts a domain has, and
`shoplazza <domain> +<name> --help` for one shortcut's flags.

### 2. API Commands

Auto-generated from OpenAPI metadata — commands mapped 1:1 to platform endpoints.

```bash
shoplazza products list
shoplazza products get --params '{"product_id":"<product-id>"}'
shoplazza products create --data @product.json

shoplazza discounts list
shoplazza discounts create-non-automatic --data @discount.json

# All domains: products, discounts, orders, customers, billing, shop, themes, webhook
shoplazza orders list
shoplazza customers list
```

### 3. Raw API Calls

Call any Shoplazza Open Platform endpoint directly for full coverage.

```bash
shoplazza api rest GET /openapi/2026-01/products
shoplazza api rest POST /openapi/2026-01/products \
  --data '{"product": {"title": "New Product", "status": "active"}}'
```

## AI Agent Plugin & Skills

Ready-made [Agent Skills](https://agentskills.io) that teach an AI coding agent how to drive
this CLI properly: which of the three command tiers to reach for, the `{"ok":true,"data":…}`
output envelope, the `--dry-run`-before-writes safety rule, and the per-domain gotchas that
are easy to get wrong. Works with Claude Code, Codex, Cursor and others.

Before you start, install the CLI and log in (`shoplazza auth login`): the skills drive the CLI
on your own machine, so they do not work in web chat such as claude.ai or ChatGPT on the web.

### Install as a plugin

The plugin bundles every skill below and is kept up to date by your agent's own plugin manager.

| Agent | Install |
|-------|---------|
| Claude Code | `claude plugin install shoplazza --marketplace Shoplazza/shoplazza-cli` |
| Codex | `codex plugin marketplace add Shoplazza/shoplazza-cli`, then `codex plugin add shoplazza@shoplazza-cli` |
| GitHub Copilot CLI | `copilot plugin marketplace add Shoplazza/shoplazza-cli`, then `copilot plugin install shoplazza@shoplazza-cli` |
| VS Code (Copilot) | Enable `chat.plugins.enabled`, run **Chat: Install Plugin From Source**, enter `Shoplazza/shoplazza-cli` |
| Cursor (Teams / Enterprise) | Dashboard → Plugins & MCPs → Add Marketplace → Import from Repo → `Shoplazza/shoplazza-cli` |
| Antigravity | `agy plugin install https://github.com/Shoplazza/shoplazza-cli` |
| CodeBuddy | `codebuddy plugin marketplace add Shoplazza/shoplazza-cli`, then `codebuddy plugin install shoplazza@shoplazza-cli` |
| Qoder | `qoder plugins marketplace add Shoplazza/shoplazza-cli`, then `qoder plugins install shoplazza` |
| Kimi Code CLI | In a session: `/plugins install https://github.com/Shoplazza/shoplazza-cli` |

Claude Code before 2.1.275 takes two steps: `claude plugin marketplace add Shoplazza/shoplazza-cli`,
then `claude plugin install shoplazza@shoplazza-cli`.

### Install with `npx skills add`

For any other agent, or if you prefer skills without a plugin:

```bash
npx skills add Shoplazza/shoplazza-cli -g
```

Installs all skills below into `~/.agents/skills/` and links them into your agent's skill
directory, for use across every project. `shoplazza update` refreshes installed skills, and the
same skills ship inside the binary: `shoplazza skills list` / `shoplazza skills read <skill>`.

Add `-a <agent>` to install for one agent only, for example `-a trae` (TRAE), `-a trae-cn`
(TRAE CN), `-a hermes-agent`, `-a openclaw`, `-a pi` or `-a gemini-cli`. DeepSeek Harness reads
`~/.agents/skills` directly, so the command above is all it needs.

### Pick one

Install the plugin or `npx skills add`, not both: with both, your agent sees every skill twice
(in Claude Code, `shoplazza-orders` and `shoplazza:shoplazza-orders`). `shoplazza doctor` warns
when it finds both. If you already have both, keep the plugin and remove the other copy:

```bash
npx skills remove --global shoplazza-billing shoplazza-common shoplazza-customers shoplazza-discounts shoplazza-orders shoplazza-products shoplazza-shop shoplazza-themes shoplazza-webhook
```

### Available skills

| Skill | Covers |
|-------|--------|
| `shoplazza-common` | **Base skill — required by all the others.** Auth & profiles, the three command tiers, the output envelope, `--dry-run` safety, `schema` introspection |
| `shoplazza-products` | Products, variants, inventory, collections, images, reviews, gift cards, locations, suppliers |
| `shoplazza-orders` | Orders, fulfillments, refunds, transactions, risk, post-sales, shipping zones |
| `shoplazza-customers` | Customers and their addresses |
| `shoplazza-discounts` | Automatic and code discounts, coupon campaigns |
| `shoplazza-shop` | Shop info, blogs & articles, pages, files, metafields, markets, languages, redirects, carrier services, analytics |
| `shoplazza-themes` | Store themes: page & card editing, theme settings, app embeds, custom templates, AI blocks, market & versions |
| `shoplazza-billing` | Application charges (one-time, recurring, usage-based) |
| `shoplazza-webhook` | Webhook subscriptions |

The sources live in [`skills/`](./skills). Skills run with full agent permissions — read them
before use.

### Plugin manifests

The repository root doubles as the plugin. Each file is read by:

| File | Read by |
|------|---------|
| `plugin.json` | Codex, Copilot, Cursor, Antigravity |
| `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json` | Claude Code, Copilot, CodeBuddy |
| `.agents/plugins/marketplace.json` | Codex |
| `.codebuddy-plugin/marketplace.json` | CodeBuddy |
| `.qoder-plugin/` | Qoder |
| `.kimi-plugin/plugin.json` | Kimi Code CLI |
| `assets/icon.png` | Claude and Codex store listings |
| `assets/logo.png` | Store forms that ask for a logo on a background plate, such as Cursor's |

Their `version` matches `package.json`; `scripts/tag-release.sh` refuses to tag otherwise.

## Advanced Usage

### Common Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--format json\|pretty\|table\|ndjson\|csv` | All commands | Output format. Default auto-detects: `pretty` at a terminal, `json` when piped/CI (or set `SHOPLAZZA_CLI_FORMAT`). `json` is the machine contract; `pretty`/`table` are for humans |
| `--no-input` | All commands | Never prompt; fail fast on missing input (scripts/agents). Same as `SHOPLAZZA_CLI_NO_INTERACTIVE=1` |
| `--profile <name>` | All commands | Profile for this invocation (beats `SHOPLAZZA_CLI_PROFILE` and the current profile) |
| `--data` / `--params` | API commands | Request body / path & query parameters as JSON: inline, `@file`, or `-` for stdin |
| `--dry-run` | API & shortcut commands | Preview request without executing |
| `--jq "expr"` / `-q` | API & shortcut commands | Filter JSON output with jq expression |

### Schema Introspection

Inspect any service's methods, parameters, required scopes, and response shape:

```bash
shoplazza schema                              # List all services
shoplazza schema products                     # Inspect a service
shoplazza schema products.list                # Inspect a method
```

### Updating

```bash
shoplazza update            # update the binary (npm installs), refresh the API metadata and installed Agent Skills
shoplazza update --check    # report current/latest versions only, no install
shoplazza doctor check      # diagnose CLI health
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SHOPLAZZA_UAT` | User Access Token for non-interactive login (equivalent to `--uat`) |
| `SHOPLAZZA_ACCESS_TOKEN` | Use this store access token directly, skipping login (CI). Needs a store target: `SHOPLAZZA_CLI_API_BASE_URL` or a profile |
| `SHOPLAZZA_CLI_API_BASE_URL` | Store base URL used with `SHOPLAZZA_ACCESS_TOKEN` (e.g. `https://my-store.myshoplazza.com`) |
| `SHOPLAZZA_CLI_PROFILE` | Profile to use (overridden by `--profile`) |
| `SHOPLAZZA_CLI_FORMAT` | Pin the default output format (`json`/`pretty`/`table`/`ndjson`/`csv`), overriding TTY auto-detection; `--format` still overrides it. Use `=json` to force machine output in a pty |
| `SHOPLAZZA_CLI_NO_INTERACTIVE` | Disable interactive prompts (set it in Agent harnesses that allocate a pty; or pass `--no-input`) |
| `SHOPLAZZA_CLI_NO_UPDATE_CHECK` | Disable the background new-version check |
| `SHOPLAZZA_CLI_NO_META_UPDATE` | Disable background API-metadata refreshes |
| `SHOPLAZZA_CLI_NO_NOTICE` | Drop the `_notice` block (stale CLI / skills) from json output |
| `SHOPLAZZA_CLI_SKIP_SKILLS` | Skip refreshing installed Agent Skills after `shoplazza update` |
| `NO_COLOR` | Disable colored output |
| `SHOPLAZZA_CLI_AUTH_BASE_URL` | Override auth base URL (default: `https://partners.shoplazza.com`) |
| `SHOPLAZZA_CLI_USER_ID` | Override the `cli-user-id` request header (audit attribution). Defaults to the user id captured at login; set it when `SHOPLAZZA_ACCESS_TOKEN` injects a token, which carries no login state |

Interactive prompts are auto-detected from the terminal and can only be switched **off** — there is no `--interactive` flag to force them on.

### Output & interactivity: humans vs scripts/agents

The CLI auto-detects the audience from where stdout goes (like `gh`/`docker`/`kubectl`):

- **At an interactive terminal → `pretty`** (readable, colored). **Piped, redirected, in CI, or with `--no-input`/`SHOPLAZZA_CLI_NO_INTERACTIVE`/`CI` set → `json`** — the stable, parseable machine contract. So an agent that pipes stdout (the common case), or declares itself, always gets JSON.
- **Humans** need pass nothing at a terminal. `pretty`/`table` are for reading, **not** a stable contract — don't parse them (they may truncate/collapse nested data). Force JSON anywhere with `--format json`.
- **Scripts & agents**: piping stdout already yields JSON. If your harness allocates a **pty** for stdout, force it with `--format json` or `SHOPLAZZA_CLI_FORMAT=json`, and pass `--no-input` (or set `SHOPLAZZA_CLI_NO_INTERACTIVE=1` / `CI=1`) so no prompt can block. Don't set `SHOPLAZZA_CLI_FORMAT=pretty` in a shared shell profile — child processes inherit it.
- **Errors** follow the same split. At a `pretty`/`table` terminal a failure prints a readable `Error:` / `Hint:` line (plus the failing endpoint and request id when the server returned them); piped, in CI, with `--no-input`, or in any `json`/`ndjson`/`csv` mode, stderr carries the `{"ok":false,"error":{…}}` envelope. Both a human format **and** a terminal on stderr are required for the readable line, so a redirected stderr always stays JSON — scripts and agents keep parsing it.

The full machine contract (envelopes, error subtypes, exit codes) is in [AGENTS.md](./AGENTS.md).

## Security & Risk Warnings

> Read Before Use

- **AI Agent Automation Risk** — When AI Agents operate the CLI on your behalf, all API calls carry real consequences (creating products, modifying orders, deleting discounts). Always review the Agent's proposed commands before execution.
- **Credential Safety** — Tokens are encrypted on local disk. Never share your UAT or store tokens. Rotate credentials immediately if you suspect exposure.
- **Scope Control** — Use `--scope` or `--domain` to limit the permissions granted during login. Grant only the scopes your workflow requires.

## Contributing

Contributions are welcome! If you find a bug or have a feature suggestion, please open an Issue or Pull Request on [GitHub](https://github.com/Shoplazza/shoplazza-cli).

For major changes, please open an issue first to discuss the approach.

## License

This project is licensed under the **MIT License**.
When running, it calls the Shoplazza Open Platform APIs. Usage of these APIs is subject to the [Shoplazza Developer Agreement](https://www.shoplazza.dev/).
