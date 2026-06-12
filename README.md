# shoplazza-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/shoplazza-cli.svg)](https://www.npmjs.com/package/shoplazza-cli)

[中文版](./README.zh.md) | [English](./README.md)

The official [Shoplazza Open Platform](https://www.shoplazza.dev/) CLI tool — built for humans and AI Agents. Develop apps and themes, manage products, discounts, orders and customers, all from the terminal with structured output designed for AI Agent integration.

[Install](#installation--quick-start) · [Auth](#authentication) · [Development](#development-workflows) · [Commands](#three-layer-command-system) · [Advanced](#advanced-usage) · [Contributing](#contributing)

## Why shoplazza-cli?

- **Agent-Native Design** — Structured JSON output out of the box; AI Agents can operate Shoplazza stores with zero extra setup
- **E-Commerce Focused** — Products, Discounts, Orders, Customers with full CRUD and 20+ shortcut commands for high-frequency operations
- **Full Developer Workflow** — App creation, extension scaffolding (checkout / theme / function), local dev server with HMR, one-command deploy; plus theme init, live reload, and packaging
- **Secure & Controllable** — Input injection protection, OS-native keychain credential storage, token auto-refresh
- **Three-Layer Architecture** — Shortcuts (human & AI friendly) → API Commands (OpenAPI-synced) → Raw API (full coverage)
- **Up and Running in 3 Minutes** — Interactive login, from install to first API call in 3 steps

## Features

| Domain | Capabilities |
|--------|-------------|
| 🛍️ Products | CRUD + shortcuts: `+search`, `+publish`, `+unpublish`, `+create`, `+set-price`, `+stock` |
| 🏷️ Discounts | CRUD + 8 shortcut creators for automatic & code discounts |
| 📦 Orders | CRUD + shortcuts: `+search`, `+count`, `+ship`, `+refund`, `+update-tracking` |
| 👤 Customers | CRUD + shortcuts: `+search`, `+create` |
| 🎨 Themes | `init`, `serve` (live reload), `pull`, `push`, `package`, `share` |
| 🧩 App | Full lifecycle: init → extension create → dev → deploy; extensions: checkout, theme, function |

## Installation & Quick Start

### Requirements

- Node.js `>=14.18.0` (`npm`/`npx`)
- Go `v1.24`+ (only required for building from source)

### Quick Start (Human Users)

> **Note for AI assistants:** If you are an AI Agent helping the user install, jump directly to [Quick Start (AI Agent)](#quick-start-ai-agent).

```bash
# Install
npm install -g shoplazza-cli

# Log in (replace with your store domain)
shoplazza auth login --store-domain my-store.shoplazza.com --domain products,orders

# Verify
shoplazza auth status

# Start using
shoplazza products list --format table
```

#### Install from source

Requires Go `v1.24`+. `make install` builds the binary and installs it to `~/.local/bin` (user-level — no `sudo`):

```bash
git clone https://github.com/Shoplazza/shoplazza-cli.git
cd shoplazza-cli
make install
```

> If `~/.local/bin` is not on your `PATH`, add it: `export PATH="$HOME/.local/bin:$PATH"`.
> For a system-wide install: `sudo make install PREFIX=/usr/local`.

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

# Switch store
shoplazza auth store use --store-domain another-store.myshoplazza.com

# Check status
shoplazza auth status
```

Access tokens are stored in the OS-native keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service).

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
shoplazza products +publish <product-id>

# Discounts — automatic
shoplazza discounts +rebate --title "Summer Sale" --percentage 15 --min-amount 100
shoplazza discounts +flashsale --title "Flash Sale" --percentage 20 --product-ids "123,456"

# Discounts — code-based
shoplazza discounts +percent-code --code "SAVE20" --percentage 20
shoplazza discounts +bxgy-code --code "BUY2GET1" --buy-quantity 2 --get-quantity 1

# Orders
shoplazza orders +ship <order-id>
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
shoplazza api rest GET /openapi/2022-01/products.json
shoplazza api rest POST /openapi/2022-01/products.json \
  --data '{"product": {"title": "New Product", "status": "active"}}'
```

## Advanced Usage

### Common Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--format json\|pretty\|table` | All commands | Output format (default: `json`) |
| `--fields "f1,f2"` | Shortcut commands | Response field projection |
| `--dry-run` | API & shortcut commands | Preview request without executing |
| `--jq "expr"` / `-q` | API commands | Filter JSON output with jq expression |

### Schema Introspection

Inspect any service's methods, parameters, required scopes, and response shape:

```bash
shoplazza schema                              # List all services
shoplazza schema products                     # Inspect a service
shoplazza schema products.list                # Inspect a method
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SHOPLAZZA_UAT` | User Access Token for non-interactive login (equivalent to `--uat`) |
| `SHOPLAZZA_CLI_AUTH_BASE_URL` | Override auth base URL (default: `https://partners.shoplazza.com`) |

## Security & Risk Warnings

> Read Before Use

- **AI Agent Automation Risk** — When AI Agents operate the CLI on your behalf, all API calls carry real consequences (creating products, modifying orders, deleting discounts). Always review the Agent's proposed commands before execution.
- **Credential Safety** — Tokens are stored in the OS-native keychain. Never share your UAT or store tokens. Rotate credentials immediately if you suspect exposure.
- **Scope Control** — Use `--scope` or `--domain` to limit the permissions granted during login. Grant only the scopes your workflow requires.

## Contributing

Contributions are welcome! If you find a bug or have a feature suggestion, please open an Issue or Pull Request on [GitHub](https://github.com/Shoplazza/shoplazza-cli).

For major changes, please open an issue first to discuss the approach.

### Local Setup

```bash
# Build
make build

# Run tests
make test

# Lint (pre-PR)
go mod tidy
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run --new-from-rev=origin/main

# Install locally
make install
```

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`, `ci:`.

## License

This project is licensed under the **MIT License**.
When running, it calls the Shoplazza Open Platform APIs. Usage of these APIs is subject to the [Shoplazza Developer Agreement](https://www.shoplazza.dev/).
