# shoplazza-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/shoplazza-cli.svg)](https://www.npmjs.com/package/shoplazza-cli)

[中文版](./README.zh.md) | [English](./README.md)

The official [Shoplazza Open Platform](https://www.shoplazza.dev/) CLI tool — built for humans and AI Agents. Develop apps and themes, manage products, discounts, orders and customers, all from the terminal with structured output designed for AI Agent integration.

[Install](#installation--quick-start) · [Auth](#authentication) · [Development](#development-workflows) · [Commands](#three-layer-command-system) · [Agent Skills](#agent-skills) · [Advanced](#advanced-usage) · [Contributing](#contributing)

## Why shoplazza-cli?

- **Agent-Native Design** — Structured JSON output out of the box; AI Agents can operate Shoplazza stores with zero extra setup
- **Agent Skills Included** — One command installs [skills](#agent-skills) that teach AI agents this CLI's commands, safety rules, and per-domain gotchas
- **E-Commerce Focused** — Products, Discounts, Orders, Customers with full CRUD and 20+ shortcut commands for high-frequency operations
- **Full Developer Workflow** — App creation, extension scaffolding (checkout / theme / function), local dev server with HMR, one-command deploy; plus theme init, live reload, and packaging
- **Secure & Controllable** — Input injection protection, OS-native keychain credential storage, token auto-refresh
- **Three-Layer Architecture** — Shortcuts (human & AI friendly) → API Commands (OpenAPI-synced) → Raw API (full coverage)
- **Up and Running in 3 Minutes** — Interactive login, from install to first API call in 3 steps

## Features

| Domain | Capabilities |
|--------|-------------|
| 🛍️ Products | CRUD + shortcuts: `+search`, `+count`, `+publish`, `+unpublish`, `+create`, `+set-price`, `+stock`, `+tag` |
| 🏷️ Discounts | CRUD + 8 shortcuts: 7 creators for automatic & code discounts, plus `+search` |
| 📦 Orders | CRUD + shortcuts: `+search`, `+count`, `+ship`, `+refund`, `+update-tracking` |
| 👤 Customers | CRUD + shortcuts: `+search`, `+create` |
| 🏪 Shop | Shop info, blogs & articles, pages, files (`+upload-file`), metafields, markets, languages, redirects, analytics |
| 💳 Billing | Application charges: one-time, recurring, usage-based |
| 🔔 Webhooks | Webhook subscription CRUD |
| 🎨 Themes | `init`, `serve` (live reload), `pull`, `push`, `package`, `share` |
| 🧩 App | Full lifecycle: init → extension create → dev → deploy; extensions: checkout, theme, function |

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

> Run the login command, extract the authorization URL from output, and send it to the user. The command polls until the user completes OAuth in their browser.

```bash
npm install -g shoplazza-cli
shoplazza auth login --store-domain <store-domain> --domain products,orders
shoplazza auth status
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

Access tokens are stored in the OS-native keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service).

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
# 1. Create a new app project (creates a sub-directory)
shoplazza app init --name "My App" --partner <partner-id>

# 2. Add extensions (theme / checkout / function)
cd my-app
shoplazza app extension create --type checkout --name my-checkout
shoplazza app extension create --type theme --name my-theme --theme-type basic
shoplazza app extension create --type function --name my-fn

# 3. Local development (dev server + HMR) — store comes from the active app config
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
shoplazza app config use --config alt.toml      # Switch active app config
shoplazza app config link --client-id <id>      # Link an existing app

# Function extensions (compile/release individually)
shoplazza app function compile --extension my-fn
shoplazza app function release --extension my-fn
shoplazza app function list
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
```

## Three-Layer Command System

The CLI provides three levels of granularity, covering everything from quick operations to fully custom API calls.

### 1. Shortcuts

Prefixed with `+`, designed to be friendly for both humans and AI, with smart defaults and structured output.

```bash
# Products
shoplazza products +search --keyword "shirt"
shoplazza products +publish --id <product-id>

# Discounts — automatic
shoplazza discounts +rebate --title "Summer Sale" --percentage 15 --min-amount 100
shoplazza discounts +flashsale --title "Flash Sale" --percentage 20 --product-ids "123,456"

# Discounts — code-based
shoplazza discounts +percent-code --code "SAVE20" --percentage 20
shoplazza discounts +bxgy-code --code "BUY2GET1" --buy-quantity 2 --get-quantity 1

# Orders
shoplazza orders +ship --order-id <order-id> --tracking <tracking-no>
```

Run `shoplazza <domain> --help` to see all shortcuts for a domain.

### 2. API Commands

Auto-generated from OpenAPI metadata — commands mapped 1:1 to platform endpoints.

```bash
shoplazza products list
shoplazza products get <product-id>
shoplazza products create --data @product.json

shoplazza discounts list
shoplazza discounts create-discount --data @discount.json

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

## Agent Skills

Ready-made [Agent Skills](https://agentskills.io) that teach an AI coding agent how to drive
this CLI properly: which of the three command tiers to reach for, the `{"ok":true,"data":…}`
output envelope, the `--dry-run`-before-writes safety rule, and the per-domain gotchas that
are easy to get wrong. Works with Claude Code, Codex, Cursor, Gemini CLI, Zed and others.

### Install

```bash
npx skills add Shoplazza/shoplazza-cli -g
```

Installs all skills below into `~/.agents/skills/` and links them into your agent's skill
directory, for use across every project.

### Available skills

| Skill | Covers |
|-------|--------|
| `shoplazza-common` | **Base skill — required by all the others.** Auth & profiles, the three command tiers, the output envelope, `--dry-run` safety, `schema` introspection |
| `shoplazza-products` | Products, variants, inventory, collections, comments, gift cards |
| `shoplazza-orders` | Orders, fulfillments, refunds, transactions, draft orders |
| `shoplazza-customers` | Customers and their addresses |
| `shoplazza-discounts` | Automatic and code discounts, coupon campaigns |
| `shoplazza-shop` | Shop info, blogs & articles, pages, files, metafields, markets, languages, redirects, analytics |
| `shoplazza-billing` | Application charges (one-time, recurring, usage-based) |
| `shoplazza-webhook` | Webhook subscriptions |

The sources live in [`skills/`](./skills). Skills run with full agent permissions — read them
before use.

## Advanced Usage

### Common Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--format json\|pretty\|table` | All commands | Output format (default: `json`) |
| `--profile <name>` | All commands | Profile for this invocation (beats `SHOPLAZZA_CLI_PROFILE` and the current profile) |
| `--dry-run` | API & shortcut commands | Preview request without executing |
| `--jq "expr"` / `-q` | API & shortcut commands | Filter JSON output with jq expression |
| `--fields "f1,f2"` | A few search shortcuts | Response field projection (check `--help`; elsewhere use `--jq`) |

### Schema Introspection

Inspect any service's methods, parameters, required scopes, and response shape:

```bash
shoplazza schema                              # List all services
shoplazza schema products                     # Inspect a service
shoplazza schema products.list                # Inspect a method
```

### Updating

```bash
shoplazza update            # update the binary (npm installs) and refresh the API metadata
shoplazza update --check    # report current/latest versions only, no install
```

The CLI notes newer versions in the background, and the command tree itself updates without a
CLI release: newly published API operations arrive via a checksum-verified metadata refresh
(checked at most once per 24h, silently non-fatal). Non-npm installs update the binary via
`brew upgrade` or a re-download; `shoplazza update` still refreshes the metadata.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SHOPLAZZA_UAT` | User Access Token for non-interactive login (equivalent to `--uat`) |
| `SHOPLAZZA_CLI_PROFILE` | Profile to use (overridden by `--profile`) |
| `SHOPLAZZA_CLI_NO_UPDATE_CHECK` | Disable the background new-version check |
| `SHOPLAZZA_CLI_NO_META_UPDATE` | Disable background API-metadata refreshes |
| `SHOPLAZZA_CLI_AUTH_BASE_URL` | Override auth base URL (default: `https://partners.shoplazza.com`) |

## Security & Risk Warnings

> Read Before Use

- **AI Agent Automation Risk** — When AI Agents operate the CLI on your behalf, all API calls carry real consequences (creating products, modifying orders, deleting discounts). Always review the Agent's proposed commands before execution.
- **Credential Safety** — Tokens are stored in the OS-native keychain. Never share your UAT or store tokens. Rotate credentials immediately if you suspect exposure.
- **Scope Control** — Use `--scope` or `--domain` to limit the permissions granted during login. Grant only the scopes your workflow requires.

## Contributing

Contributions are welcome! If you find a bug or have a feature suggestion, please open an Issue or Pull Request on [GitHub](https://github.com/Shoplazza/shoplazza-cli).

For major changes, please open an issue first to discuss the approach.

## License

This project is licensed under the **MIT License**.
When running, it calls the Shoplazza Open Platform APIs. Usage of these APIs is subject to the [Shoplazza Developer Agreement](https://www.shoplazza.dev/).
