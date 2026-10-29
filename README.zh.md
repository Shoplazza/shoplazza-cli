# shoplazza-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/shoplazza-cli.svg)](https://www.npmjs.com/package/shoplazza-cli)

[中文版](./README.zh.md) | [English](./README.md)

Shoplazza 开放平台官方 CLI 工具 — 让人类和 AI Agent 都能操作 Shoplazza 店铺。开发应用和主题、管理商品、折扣、订单和客户，结构化输出天然适配 AI Agent 集成。

[安装](#安装与快速开始) · [认证](#认证) · [开发工作流](#开发工作流) · [命令](#三层命令调用) · [Agent 插件与 Skills](#ai-agent-插件与-skills) · [进阶用法](#进阶用法) · [贡献](#贡献)

## 为什么选 shoplazza-cli？

- **为 Agent 原生设计** — 结构化 JSON 输出开箱即用，AI Agent 无需额外适配即可操作 Shoplazza 店铺
- **内置 Agent Skills** — 一个插件或一条命令即可安装 [skills](#ai-agent-插件与-skills)，让 AI Agent 掌握本 CLI 的命令体系、安全规则与各业务域的易错点
- **电商全域覆盖** — 商品、折扣、订单、客户完整 CRUD，30+ 快捷命令覆盖高频操作
- **完整开发者工作流** — App 创建、扩展脚手架（checkout / theme / function）、经自动隧道的本地开发服务器、一键部署；主题 init、实时热重载、多环境与打包
- **安全可控** — 凭证本地加密存储、店铺 Token 自动刷新、`--dry-run` 预览请求，破坏性命令执行前需确认
- **按需交互** — 缺少必填参数时直接提问，id 类参数弹出模糊搜索选择器；脚本和 Agent 不会遇到任何提问
- **三层调用架构** — 快捷命令（人机友好）→ API 命令（OpenAPI 同步）→ 通用调用（全 API 覆盖）
- **三分钟上手** — 交互式登录授权，从安装到第一次 API 调用只需三步

## 功能

| 业务域 | 能力 |
|--------|------|
| 🛍️ 商品 | 商品目录、变体、库存、合集、图片、礼品卡、买家评价、仓库、供应商与采购 |
| 🏷️ 折扣 | 自动折扣与代码折扣、限时秒杀、优惠券活动 |
| 📦 订单 | 订单、发货、退款、支付流水、风控、售后、运费区域、物流承运商 |
| 👤 客户 | 客户资料、收货地址、营销订阅 |
| 🏪 店铺 | 店铺信息、博客与文章、自定义页面、菜单、文件、metafields、市场、多语言、URL 重定向、实时运费承运商、数据分析 |
| 💳 计费 | 应用收费：一次性、订阅、按量 |
| 🔔 Webhook | Webhook 订阅 CRUD |
| 🎨 主题 | 本地开发循环（实时热重载、多环境切换）；店铺主题管理（主题市场、发布、文件、版本）；页面、卡片与 AI 卡片编辑 |
| 🧩 应用 | 完整生命周期：init → extension create → dev → deploy；扩展类型：checkout、theme、function |
| 🧱 扩展 | 独立的 `checkout-extension` 与 `theme-extension` 项目：脚手架、本地开发 / 预览、发版、部署 / 发布 |

大部分业务域在生成的命令之外还提供 `+` 快捷命令，运行 `shoplazza <domain> --help` 查看该域有哪些。

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

> **给 AI 助手：** 如果你是 AI Agent 且正在协助用户执行安装，请直接跳转到[快速开始（AI Agent）](#快速开始ai-agent)。

```bash
# 登录授权（替换为你的店铺域名）
shoplazza auth login --store-domain my-store.myshoplazza.com --domain products,orders

# 验证
shoplazza auth status

# 开始使用
shoplazza products list --format table
```

### 快速开始（AI Agent）

先装 [Agent Skills](#ai-agent-插件与-skills)（作为 Agent 的插件安装，或用下面这条命令）—— Agent 会从 skill 里拿到安装、授权引导和命令调用的全部规则：

```bash
npx skills add Shoplazza/shoplazza-cli -g
```

装完后把这句发给 Agent，它会带你完成授权：

```text
/shoplazza-common 帮我登录 <店铺域名>
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

# 补充权限 — 重新登录会替换（而非叠加）已授权 scopes，用 --merge-scopes 保留原有授权
shoplazza auth login --domain discounts --merge-scopes

# 切换店铺
shoplazza auth store use --store-domain another-store.myshoplazza.com

# 查看状态
shoplazza auth status
```

凭证（账户 UAT 与店铺 Token）以 AES-256-GCM 加密，存放在用户配置目录下的 `shoplazza-cli/` 中，文件权限仅限本人读写。

### 多店铺 Profile

Profile 是按店铺划分的执行上下文（店铺 Token + scopes）。一个账户登录后可管理多家店铺、
随时切换而无需重新授权 — `auth login -s` 和 `auth store use` 会自动创建/切换 profile，
也可以直接管理：

```bash
shoplazza profile add --name prod-us -s my-store.myshoplazza.com --use
shoplazza profile list
shoplazza profile use --name prod-us        # 或 --previous 切回上一个
shoplazza products list --profile prod-us   # 单次调用指定 profile，无需切换
```

## 开发工作流

### App 开发

CLI 覆盖完整的 App 生命周期：创建、配置、开发和部署。

```bash
# 1. 创建新 App 项目（在当前目录下创建子目录；账户属于多个 partner 时加 --partner <id>）
shoplazza app init --name "My App"

# 2. 添加扩展（theme / checkout / function）
cd my-app
shoplazza app extension create --type checkout --name my-checkout
shoplazza app extension create --type theme --name my-theme --theme-type basic
shoplazza app extension create --type function --name my-fn

# 3. 本地开发（自动隧道；改动后重跑 app dev 生效）— 店铺由当前活跃的 App 配置决定
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
shoplazza app config use --config alt           # 切换活跃 App 配置为 shoplazza.app.alt.toml（持久）
shoplazza app dev --config staging              # 仅本次针对某个配置运行（不持久化）；dev/deploy/function 同样支持
shoplazza app config link --client-id <id>      # 关联已有 App
shoplazza app config push                       # 把 [dashboard]（name / app_url / redirect_url / embed）推送到 Partner 后台
shoplazza app dev --write-urls                  # 同时把隧道 URL 写入 [dashboard]，随后用 app config push 同步

# Function 扩展（单独编译/发布）
shoplazza app function compile --name my-fn
shoplazza app function release --name my-fn
shoplazza app function list
```

</details>

<details>
<summary>独立扩展项目（checkout / theme）</summary>

```bash
# Checkout 扩展：脚手架、带 HMR 的开发服务器、上传新版本、激活版本
shoplazza checkout-extension init --name my-checkout-ext --extension banner
shoplazza checkout-extension dev --extension-name banner
shoplazza checkout-extension push --name banner
shoplazza checkout-extension deploy --extension-id <id> --version 1.0

# Theme 扩展（别名 te）：脚手架、实时预览、构建新版本、关联 App 并发布
shoplazza theme-extension create --name my-te --type embed
shoplazza te serve --theme-id <theme-id>
shoplazza te build --version 1.0.0 --description "initial release"
shoplazza te connect --client-id <client-id>
shoplazza te release
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

# 5. AI 生成卡片：先读页面（返回编辑会话 id），再一条命令写入 block 文件并落到页面
shoplazza themes +page --template index
shoplazza themes block +edit --session <oseid> --content card.liquid --template index --target <section_id>.blocks
shoplazza themes block +get  --session <oseid> --id gen_1a0d523 --section <section_id>
```

<details>
<summary>主题多环境（多店 / staging）</summary>

在 `shoplazza.theme.toml` 里记录命名环境（店铺、theme id、路径、ignore），用 `-e` 指定其一：

```bash
shoplazza themes env add staging --store <domain>   # 缺的字段交互式补问；写入前校验
shoplazza themes env list                    # 列出已配置环境
shoplazza themes env check                   # 离线校验所有环境
shoplazza themes push -e staging             # 针对该环境运行（店铺 + theme + 路径）
shoplazza themes pull -e staging             # 把解析出的目标回写到 staging
```

存在 `default` 环境时会自动套用；`env set` / `env remove` 用于修改和删除条目。

</details>

## 三层命令调用

CLI 提供三种粒度的调用方式，覆盖从快速操作到完全自定义的全部场景。

### 1. 快捷命令（Shortcuts）

以 `+` 为前缀，对人类与 AI 友好化封装，内置智能默认值和结构化输出。

```bash
shoplazza products +search --keyword "衬衫"
shoplazza discounts +percent-code --target order --percent 20 --code SAVE20
shoplazza orders +ship --order-id <order-id> --tracking <tracking-no>
```

运行 `shoplazza <domain> --help` 查看该域有哪些快捷命令，`shoplazza <domain> +<name> --help` 查看某条快捷命令的参数。

### 2. API 命令

从 OpenAPI 元数据自动生成，命令与平台端点一一对应。

```bash
shoplazza products list
shoplazza products get --params '{"product_id":"<product-id>"}'
shoplazza products create --data @product.json

shoplazza discounts list
shoplazza discounts create-non-automatic --data @discount.json

# 所有业务域：products, discounts, orders, customers, billing, shop, themes, webhook
shoplazza orders list
shoplazza customers list
```

### 3. 通用 API 调用

直接调用任意 Shoplazza 开放平台端点，覆盖全量 API。

```bash
shoplazza api rest GET /openapi/2026-01/products
shoplazza api rest POST /openapi/2026-01/products \
  --data '{"product": {"title": "新商品", "status": "active"}}'
```

## AI Agent 插件与 Skills

开箱即用的 [Agent Skills](https://agentskills.io)，教会 AI 编码 Agent 正确驱动本 CLI：
三层命令该选哪一层、`{"ok":true,"data":…}` 输出信封怎么读、写操作前必须 `--dry-run` 的
安全规则，以及各业务域里容易踩错的坑。适用于 Claude Code、Codex、Cursor 等。

使用前请先安装 CLI 并登录（`shoplazza auth login`）：skill 要在你自己的电脑上调用 CLI，所以在
claude.ai、ChatGPT 网页版这类网页对话里用不了。

### 作为插件安装

插件里打包了下列全部 skill，由 Agent 自己的插件管理负责更新。

| Agent | 安装方式 |
|-------|---------|
| Claude Code | `claude plugin install shoplazza --marketplace Shoplazza/shoplazza-cli` |
| Codex | `codex plugin marketplace add Shoplazza/shoplazza-cli`，再 `codex plugin add shoplazza@shoplazza-cli` |
| GitHub Copilot CLI | `copilot plugin marketplace add Shoplazza/shoplazza-cli`，再 `copilot plugin install shoplazza@shoplazza-cli` |
| VS Code（Copilot） | 打开 `chat.plugins.enabled`，执行 **Chat: Install Plugin From Source**，填 `Shoplazza/shoplazza-cli` |
| Cursor（团队版 / 企业版） | 后台 Dashboard → Plugins & MCPs → Add Marketplace → Import from Repo → `Shoplazza/shoplazza-cli` |
| Antigravity | `agy plugin install https://github.com/Shoplazza/shoplazza-cli` |
| CodeBuddy | `codebuddy plugin marketplace add Shoplazza/shoplazza-cli`，再 `codebuddy plugin install shoplazza@shoplazza-cli` |
| Qoder | `qoder plugins marketplace add Shoplazza/shoplazza-cli`，再 `qoder plugins install shoplazza` |
| Kimi Code CLI | 在会话里输入：`/plugins install https://github.com/Shoplazza/shoplazza-cli` |

Claude Code 2.1.275 以前的版本要分两步：先 `claude plugin marketplace add Shoplazza/shoplazza-cli`，
再 `claude plugin install shoplazza@shoplazza-cli`。

### 用 `npx skills add` 安装

其他 Agent，或者你不想用插件时：

```bash
npx skills add Shoplazza/shoplazza-cli -g
```

将下列全部 skill 装到 `~/.agents/skills/`，并软链到 Agent 的 skill 目录，所有项目均可使用。`shoplazza update` 会顺带刷新已安装的 skill；二进制里也内置了同一套 skill：`shoplazza skills list` / `shoplazza skills read <skill>`。

只想装给某一个 Agent 时加 `-a <名字>`，例如 `-a trae`（TRAE）、`-a trae-cn`（TRAE 国内版）、
`-a hermes-agent`、`-a openclaw`、`-a pi`、`-a gemini-cli`。DeepSeek Harness 会直接读
`~/.agents/skills`，用上面这条命令就够了。

### 二选一

插件和 `npx skills add` 只装一种：两种都装的话，Agent 会看到每个 skill 各两份（在 Claude Code 里是
`shoplazza-orders` 和 `shoplazza:shoplazza-orders`）。`shoplazza doctor` 发现两种都装了会给出警告。
已经两种都装了的，留下插件，删掉另一份：

```bash
npx skills remove --global shoplazza-billing shoplazza-common shoplazza-customers shoplazza-discounts shoplazza-orders shoplazza-products shoplazza-shop shoplazza-themes shoplazza-webhook
```

### Skill 列表

| Skill | 覆盖范围 |
|-------|---------|
| `shoplazza-common` | **基础 skill，其余全部依赖它。** 认证与 profile、三层命令体系、输出信封、`--dry-run` 安全规则、`schema` 自省 |
| `shoplazza-products` | 商品、变体、库存、合集、图片、买家评价、礼品卡、仓库、供应商 |
| `shoplazza-orders` | 订单、发货、退款、交易、风控、售后、运费区域 |
| `shoplazza-customers` | 客户及其地址 |
| `shoplazza-discounts` | 自动折扣与折扣码、优惠券活动 |
| `shoplazza-shop` | 店铺信息、博客与文章、自定义页面、文件、metafields、市场、多语言、URL 重定向、实时运费承运商、数据分析 |
| `shoplazza-themes` | 店铺主题：页面与卡片装修、主题设置、App 嵌入、自定义模板、AI 卡片、主题市场与版本 |
| `shoplazza-billing` | 应用计费（一次性、订阅、按量） |
| `shoplazza-webhook` | Webhook 订阅 |

源文件在 [`skills/`](./skills)。Skill 以 Agent 的完整权限运行，使用前请先阅读其内容。

### 插件清单文件

仓库根目录本身就是插件。各文件给谁读：

| 文件 | 读取方 |
|------|-------|
| `plugin.json` | Codex、Copilot、Antigravity |
| `.claude-plugin/plugin.json`、`.claude-plugin/marketplace.json` | Claude Code、Copilot、CodeBuddy |
| `.agents/plugins/marketplace.json` | Codex |
| `.codebuddy-plugin/marketplace.json` | CodeBuddy |
| `.qoder-plugin/` | Qoder |
| `.kimi-plugin/plugin.json` | Kimi Code CLI |
| `.cursor-plugin/plugin.json` | Cursor |
| `assets/icon.png` | Claude、Codex、Cursor 的插件图标 |
| `assets/logo.png` | 要求带底色 Logo 的商城申请表，比如 Cursor |

它们的 `version` 与 `package.json` 一致；不一致时 `scripts/tag-release.sh` 拒绝打 tag。

## 进阶用法

### 通用 Flag

| Flag | 适用范围 | 说明 |
|------|----------|------|
| `--format json\|pretty\|table\|ndjson\|csv` | 所有命令 | 输出格式。默认自动判定：终端里 `pretty`，管道/CI 下 `json`（或用 `SHOPLAZZA_CLI_FORMAT`）。`json` 是机器契约，`pretty`/`table` 给人看 |
| `--no-input` | 所有命令 | 从不提问；缺输入直接结构化报错（脚本/agent）。等同 `SHOPLAZZA_CLI_NO_INTERACTIVE=1` |
| `--profile <name>` | 所有命令 | 本次调用使用的 profile（优先级高于 `SHOPLAZZA_CLI_PROFILE` 和当前 profile） |
| `--data` / `--params` | API 命令 | 请求体 / 路径与查询参数，JSON 格式：可内联、`@file` 或 `-` 读 stdin |
| `--dry-run` | API 和快捷命令 | 预览请求但不执行 |
| `--jq "expr"` / `-q` | API 和快捷命令 | 使用 jq 表达式过滤 JSON 输出 |

### Schema 自省

查看任意服务的方法列表、参数、所需 scopes 和响应结构：

```bash
shoplazza schema                              # 列出所有服务
shoplazza schema products                     # 查看指定服务
shoplazza schema products.list                # 查看指定方法
```

### 更新

```bash
shoplazza update            # 更新二进制（npm 安装方式），刷新 API 元数据和已安装的 Agent Skills
shoplazza update --check    # 仅报告当前/最新版本，不安装
shoplazza doctor check      # 诊断 CLI 健康状况
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `SHOPLAZZA_UAT` | 用于非交互式登录的 User Access Token（等同 `--uat`） |
| `SHOPLAZZA_ACCESS_TOKEN` | 直接使用该店铺 Access Token，跳过登录（CI）。需要店铺目标：`SHOPLAZZA_CLI_API_BASE_URL` 或一个 profile |
| `SHOPLAZZA_CLI_API_BASE_URL` | 配合 `SHOPLAZZA_ACCESS_TOKEN` 使用的店铺基础 URL（如 `https://my-store.myshoplazza.com`） |
| `SHOPLAZZA_CLI_PROFILE` | 指定使用的 profile（`--profile` 优先） |
| `SHOPLAZZA_CLI_FORMAT` | 固定默认输出格式（`json`/`pretty`/`table`/`ndjson`/`csv`），覆盖 TTY 自动判定；`--format` 仍优先。伪终端里可用 `=json` 强制机器输出 |
| `SHOPLAZZA_CLI_NO_INTERACTIVE` | 关闭交互式提问（会分配伪终端的 Agent harness 需设置；或用 `--no-input`） |
| `SHOPLAZZA_CLI_NO_UPDATE_CHECK` | 关闭后台新版本检测 |
| `SHOPLAZZA_CLI_NO_META_UPDATE` | 关闭后台 API 元数据刷新 |
| `SHOPLAZZA_CLI_NO_NOTICE` | 去掉 json 输出里的 `_notice` 块（CLI / skill 过期提示） |
| `SHOPLAZZA_CLI_SKIP_SKILLS` | `shoplazza update` 后不刷新已安装的 Agent Skills |
| `NO_COLOR` | 关闭彩色输出 |
| `SHOPLAZZA_CLI_AUTH_BASE_URL` | 覆盖认证服务基础 URL（默认：`https://partners.shoplazza.com`） |
| `SHOPLAZZA_CLI_USER_ID` | 覆盖请求头 `cli-user-id`（审计归属）。默认取登录时记录的用户 id；用 `SHOPLAZZA_ACCESS_TOKEN` 注入 token 时没有登录态，需显式设置 |

交互式提问按终端自动判定，且只能关、不能强开 — 不存在对应的 `--interactive` flag。

### 输出与交互：人 vs 脚本/agent

CLI 按 stdout 的去向自动判定使用者（与 `gh`/`docker`/`kubectl` 一致）：

- **在交互式终端 → `pretty`**（可读、带色）。**管道 / 重定向 / CI / 设了 `--no-input`·`SHOPLAZZA_CLI_NO_INTERACTIVE`·`CI` → `json`**（稳定、可解析的机器契约）。所以用管道抓 stdout 的 agent（最常见）或声明了自己的 agent,永远拿到 JSON。
- **人**：在终端里什么都不用传。`pretty`/`table` 只给人看、**不是稳定契约**，别去解析（可能截断/折叠嵌套数据）。任何场景都可 `--format json` 强制 JSON。
- **脚本 / agent**：管道抓 stdout 本身就得到 JSON。若你的 harness 给 stdout 分配了**伪终端**,用 `--format json` 或 `SHOPLAZZA_CLI_FORMAT=json` 强制,并传 `--no-input`（或设 `SHOPLAZZA_CLI_NO_INTERACTIVE=1` / `CI=1`）确保提问不卡住。别把 `SHOPLAZZA_CLI_FORMAT=pretty` 写进共享 shell 配置（子进程会继承）。
- **错误**同样按受众区分：在 `pretty`/`table` 终端下失败会打印可读的 `Error:` / `Hint:` 行（服务端返回时还带失败的 endpoint 和 request id）；管道 / CI / `--no-input` / 任意 `json`·`ndjson`·`csv` 模式下,stderr 仍是 `{"ok":false,"error":{…}}` 信封。可读行需**同时满足**"人类格式 + stderr 是终端"两个条件,所以重定向 stderr 永远是 JSON —— 脚本和 agent 照常解析。

完整的机器契约（信封、错误 subtype、退出码）见 [AGENTS.md](./AGENTS.md)。

## 安全与风险提示

> 使用前请阅读

- **AI Agent 自动化风险** — 当 AI Agent 代你操作 CLI 时，所有 API 调用都会产生真实影响（创建商品、修改订单、删除折扣）。请在执行前审查 Agent 提出的命令。
- **凭证安全** — Token 在本地磁盘加密存储。切勿分享你的 UAT 或店铺 Token。如果怀疑凭证泄露，请立即轮换。
- **权限控制** — 使用 `--scope` 或 `--domain` 限制登录时授予的权限。只授予工作流所需的最小 scopes。

## 贡献

欢迎社区贡献！如果你发现 bug 或有功能建议，请在 [GitHub](https://github.com/Shoplazza/shoplazza-cli) 提交 Issue 或 Pull Request。

对于较大的改动，建议先通过 Issue 与我们讨论。

## 许可证

本项目基于 **MIT 许可证** 开源。
该软件运行时会调用 Shoplazza 开放平台的 API，使用这些 API 需要遵守 [Shoplazza 开发者协议](https://www.shoplazza.dev/)。
