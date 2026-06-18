# shoplazza-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/shoplazza-cli.svg)](https://www.npmjs.com/package/shoplazza-cli)

[中文版](./README.zh.md) | [English](./README.md)

Shoplazza 开放平台官方 CLI 工具 — 让人类和 AI Agent 都能在终端中操作 Shoplazza 店铺。开发应用和主题、管理商品、折扣、订单和客户，结构化输出天然适配 AI Agent 集成。

[安装](#安装与快速开始) · [认证](#认证) · [开发工作流](#开发工作流) · [命令](#三层命令调用) · [进阶用法](#进阶用法) · [贡献](#贡献)

## 为什么选 shoplazza-cli？

- **为 Agent 原生设计** — 结构化 JSON 输出开箱即用，AI Agent 无需额外适配即可操作 Shoplazza 店铺
- **电商全域覆盖** — 商品、折扣、订单、客户完整 CRUD，20+ 快捷命令覆盖高频操作
- **完整开发者工作流** — App 创建、扩展脚手架（checkout / theme / function）、本地开发服务器 + HMR、一键部署；主题 init、实时热重载与打包
- **安全可控** — 输入防注入、OS 原生密钥链存储凭证、Access Token 自动刷新
- **三层调用架构** — 快捷命令（人机友好）→ API 命令（OpenAPI 同步）→ 通用调用（全 API 覆盖）
- **三分钟上手** — 交互式登录授权，从安装到第一次 API 调用只需三步

## 功能

| 业务域 | 能力 |
|--------|------|
| 🛍️ 商品 | CRUD + 快捷命令：`+search`、`+publish`、`+unpublish`、`+create`、`+set-price`、`+stock` |
| 🏷️ 折扣 | CRUD + 8 个快捷创建命令，覆盖自动折扣与代码折扣 |
| 📦 订单 | CRUD + 快捷命令：`+search`、`+count`、`+ship`、`+refund`、`+update-tracking` |
| 👤 客户 | CRUD + 快捷命令：`+search`、`+create` |
| 🎨 主题 | `init`、`serve`（实时热重载）、`pull`、`push`、`package`、`share` |
| 🧩 应用 | 完整生命周期：init → extension create → dev → deploy；扩展类型：checkout、theme、function |

## 安装与快速开始

### 安装

| 方式 | 命令 | 说明 |
|------|------|------|
| **npm** | `npm install -g shoplazza-cli` | 推荐。需要 Node.js `>=16`。可用 `shoplazza update` 自更新。 |
| **一键脚本**（macOS / Linux） | `curl -fsSL https://raw.githubusercontent.com/Shoplazza/shoplazza-cli/main/install.sh \| bash` | 自动检测系统和架构。 |
| **下载二进制** | 见 [GitHub Releases](https://github.com/Shoplazza/shoplazza-cli/releases) | 手动下载，支持所有平台。 |
| **源码构建** | `git clone ... && cd shoplazza-cli && make install` | 需要 Go `v1.24`+。安装到 `~/.local/bin`。 |
| **Homebrew**（macOS / Linux） | `brew install Shoplazza/tap/shoplazza-cli` | 通过 `brew upgrade` 自动更新。 |

<details>
<summary>各平台二进制下载命令</summary>

```bash
# macOS (Apple Silicon)
curl -fsSL https://github.com/Shoplazza/shoplazza-cli/releases/latest/download/shoplazza-cli-darwin-arm64.tar.gz | tar -xz
sudo install -m755 shoplazza /usr/local/bin/

# macOS (Intel)
curl -fsSL https://github.com/Shoplazza/shoplazza-cli/releases/latest/download/shoplazza-cli-darwin-amd64.tar.gz | tar -xz
sudo install -m755 shoplazza /usr/local/bin/

# Linux (x86_64)
curl -fsSL https://github.com/Shoplazza/shoplazza-cli/releases/latest/download/shoplazza-cli-linux-amd64.tar.gz | tar -xz
sudo install -m755 shoplazza /usr/local/bin/

# Linux (ARM64)
curl -fsSL https://github.com/Shoplazza/shoplazza-cli/releases/latest/download/shoplazza-cli-linux-arm64.tar.gz | tar -xz
sudo install -m755 shoplazza /usr/local/bin/
```

</details>

<details>
<summary>源码构建详情</summary>

需要 Go `v1.24`+。安装到 `~/.local/bin`（用户级，无需 `sudo`）：

```bash
git clone https://github.com/Shoplazza/shoplazza-cli.git
cd shoplazza-cli
make install
```

> 若 `~/.local/bin` 不在 `PATH` 中，请加入：`export PATH="$HOME/.local/bin:$PATH"`。
> 需要系统级安装：`sudo make install PREFIX=/usr/local`。

</details>

### 快速开始（人类用户）

> **Note for AI assistants:** 如果你是 AI Agent 且正在协助用户执行安装，请直接跳转到[快速开始（AI Agent）](#快速开始ai-agent)。

```bash
# 登录授权（替换为你的店铺域名）
shoplazza auth login --store-domain my-store.shoplazza.com --domain products,orders

# 验证
shoplazza auth status

# 开始使用
shoplazza products list --format table
```

### 快速开始（AI Agent）

> 运行登录命令，从输出中提取授权链接并发送给用户。命令会持续轮询，直到用户在浏览器中完成 OAuth 授权。

```bash
npm install -g shoplazza-cli
shoplazza auth login --store-domain <店铺域名> --domain products,orders
shoplazza auth status
```

## 认证

| 命令 | 说明 |
|------|------|
| `auth login` | 仅账户 OAuth — 打开浏览器，获取 UAT |
| `auth login --store-domain <域名>` | OAuth + 店铺 Token（需要 `--scope` 或 `--domain`） |
| `auth store use --store-domain <域名>` | 切换当前店铺 |
| `auth logout` | 登出并删除凭证 |
| `auth status` | 查看当前认证状态 |
| `auth scopes` | 列出可用和已授权的 scopes |

```bash
# 交互式登录并选择店铺
shoplazza auth login --store-domain my-store.myshoplazza.com --domain products

# UAT 快速登录（非交互式，适合 CI）
shoplazza auth login --uat <user-access-token>

# 切换店铺
shoplazza auth store use --store-domain another-store.myshoplazza.com

# 查看状态
shoplazza auth status
```

凭证存储在 OS 原生密钥链中（macOS Keychain、Windows Credential Manager、Linux Secret Service）。

## 开发工作流

### App 开发

CLI 覆盖完整的 App 生命周期：创建、配置、开发和部署。

```bash
# 1. 创建新 App 项目（在当前目录下创建子目录）
shoplazza app init --name "My App" --partner <partner-id>

# 2. 添加扩展（theme / checkout / function）
cd my-app
shoplazza app extension create --type checkout --name my-checkout
shoplazza app extension create --type theme --name my-theme --theme-type basic
shoplazza app extension create --type function --name my-fn

# 3. 本地开发（开发服务器 + HMR）— 店铺由当前活跃的 App 配置决定
shoplazza app dev

# 4. 部署所有扩展
shoplazza app deploy

# 5. 查看已部署版本
shoplazza app versions
```

<details>
<summary>更多 App 命令</summary>

```bash
shoplazza app list                              # 列出账户下的 App
shoplazza app info                              # 查看 App 及扩展信息
shoplazza app config use --config alt.toml      # 切换活跃 App 配置
shoplazza app config link --client-id <id>      # 关联已有 App

# Function 扩展（单独编译/发布）
shoplazza app function compile --extension my-fn
shoplazza app function release --extension my-fn
shoplazza app function list
```

</details>

### Theme 主题开发

CLI 提供完整的主题开发工作流，支持实时热重载。

```bash
# 1. 从 Nova-2023 模板创建新主题
shoplazza themes init --name my-theme

# 2. 启动开发服务器（自动创建开发主题，实时热重载）
cd my-theme
shoplazza themes serve

# 3. 拉取 / 推送 / 打包
shoplazza themes pull --theme-id <theme-id>
shoplazza themes push --theme-id <theme-id>
shoplazza themes package

# 4. 上传为预览版
shoplazza themes share
```

## 三层命令调用

CLI 提供三种粒度的调用方式，覆盖从快速操作到完全自定义的全部场景。

### 1. 快捷命令（Shortcuts）

以 `+` 为前缀，对人类与 AI 友好化封装，内置智能默认值和结构化输出。

```bash
# 商品
shoplazza products +search --keyword "衬衫"
shoplazza products +publish <product-id>

# 折扣 — 自动折扣
shoplazza discounts +rebate --title "夏季满减" --percentage 15 --min-amount 100
shoplazza discounts +flashsale --title "限时秒杀" --percentage 20 --product-ids "123,456"

# 折扣 — 代码折扣
shoplazza discounts +percent-code --code "SAVE20" --percentage 20
shoplazza discounts +bxgy-code --code "BUY2GET1" --buy-quantity 2 --get-quantity 1

# 订单
shoplazza orders +ship <order-id>
```

运行 `shoplazza <domain> --help` 查看某个业务域的所有快捷命令。

### 2. API 命令

从 OpenAPI 元数据自动生成，命令与平台端点一一对应。

```bash
shoplazza products list
shoplazza products get <product-id>
shoplazza products create --data @product.json

shoplazza discounts list
shoplazza discounts create-discount --data @discount.json

# 所有业务域：products, discounts, orders, customers, billing, shop, themes, webhook
shoplazza orders list
shoplazza customers list
```

### 3. 通用 API 调用

直接调用任意 Shoplazza 开放平台端点，覆盖全量 API。

```bash
shoplazza api rest GET /openapi/2022-01/products.json
shoplazza api rest POST /openapi/2022-01/products.json \
  --data '{"product": {"title": "新商品", "status": "active"}}'
```

## 进阶用法

### 通用 Flag

| Flag | 适用范围 | 说明 |
|------|----------|------|
| `--format json\|pretty\|table` | 所有命令 | 输出格式（默认：`json`） |
| `--fields "f1,f2"` | 快捷命令 | 响应字段投影 |
| `--dry-run` | API 和快捷命令 | 预览请求但不执行 |
| `--jq "expr"` / `-q` | API 命令 | 使用 jq 表达式过滤 JSON 输出 |

### Schema 自省

查看任意服务的方法列表、参数、所需 scopes 和响应结构：

```bash
shoplazza schema                              # 列出所有服务
shoplazza schema products                     # 查看指定服务
shoplazza schema products.list                # 查看指定方法
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `SHOPLAZZA_UAT` | 用于非交互式登录的 User Access Token（等同 `--uat`） |
| `SHOPLAZZA_CLI_AUTH_BASE_URL` | 覆盖认证服务基础 URL（默认：`https://partners.shoplazza.com`） |

## 安全与风险提示

> 使用前请阅读

- **AI Agent 自动化风险** — 当 AI Agent 代你操作 CLI 时，所有 API 调用都会产生真实影响（创建商品、修改订单、删除折扣）。请在执行前审查 Agent 提出的命令。
- **凭证安全** — Token 存储在 OS 原生密钥链中。切勿分享你的 UAT 或店铺 Token。如果怀疑凭证泄露，请立即轮换。
- **权限控制** — 使用 `--scope` 或 `--domain` 限制登录时授予的权限。只授予工作流所需的最小 scopes。

## 贡献

欢迎社区贡献！如果你发现 bug 或有功能建议，请在 [GitHub](https://github.com/Shoplazza/shoplazza-cli) 提交 Issue 或 Pull Request。

对于较大的改动，建议先通过 Issue 与我们讨论。

## 许可证

本项目基于 **MIT 许可证** 开源。
该软件运行时会调用 Shoplazza 开放平台的 API，使用这些 API 需要遵守 [Shoplazza 开发者协议](https://www.shoplazza.dev/)。
