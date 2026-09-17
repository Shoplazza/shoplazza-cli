# Shoplazza CLI 易用性评估结果

评估方法见 [USABILITY_ASSESSMENT.md](./USABILITY_ASSESSMENT.md)。
评估对象：`/usr/local/bin/shoplazza` v2.0.7（npm 安装，2026-07-14）
评估方式：① 我本人结构化扫描（~25 条命令，覆盖正常/异常路径）② 2 个无先验知识的 subagent 盲测真实任务
**环境限制**：本次评估环境未配置商家账号/UAT，所有需要真实网络请求成功的验证（真实数据返回、写操作真正执行）均无法完成，报告中已注明哪些结论受此限制影响。

---

## 总览

| 维度 | 评分 (1-5) | 一句话结论 |
|---|---|---|
| AI Agent 可用性 | **4/5** | 自省能力（`schema`）和分层设计（+shortcut/command/api rest）是亮点，两个盲测 agent 全程未看源码/文档就摸出了高置信度命令；但 `--dry-run` 耦合了身份解析、模块级子命令输入错误被静默吞掉，是两个实打实的短板 |
| 开发者/商家可用性 | **3.5/5** | 命名一致性、README 准确度、错误提示都做得扎实；但 `--format table/pretty` 对订单/商品这类核心嵌套数据基本不起作用，削弱了"面向人类"这个格式化选项的价值 |

---

## 发现列表（按严重程度排序）

### 发现 #1 [HIGH] `--dry-run` 在所有命令层级都要求已配置 profile，无法离线验证请求
- **维度**：AI Agent + 开发者/商家（共同影响）
- **指标编号**：A7（dry-run 可靠性）、B1（首次上手体验）
- **现象**：
  ```
  $ shoplazza orders +search --fulfillment-status waiting --dry-run
  $ shoplazza orders list --dry-run
  $ shoplazza api rest GET /openapi/2026-01/orders --dry-run
  $ shoplazza discounts +percent-code --target product --percent 10 ... --dry-run
  ```
  以上命令（覆盖 shortcut / 自动生成 / raw api 三层）全部返回同样的错误，从未真正打印出"将要发送的请求"：
  ```json
  {"ok": false, "error": {"hint": "run 'shoplazza auth login -s <store> ...", "message": "no profile configured", "type": "validation"}}
  ```
  两个独立的盲测 subagent（订单任务、折扣券任务）**各自独立**跑到了这个结论，且都在报告里主动指出"以为 dry-run 能离线验证，结果发现不行"，是本次唯一被两条完全不同的任务路径同时踩中的问题。
- **影响**：`--dry-run` 的设计初衷（安全地验证命令/参数是否正确，不需要真实凭据也不产生副作用）被"需要 profile 才能拼 base URL"这个前置检查打断。对 agent 而言，意味着"无凭据环境下自主验证参数组合"这条本该存在的安全路径实际不可用；对新手商家而言，意味着"先跑一下 dry-run 看看命令对不对"这个符合直觉的探索方式，必须先完整走完认证流程才能用上，增加了上手门槛。
- **建议**：允许 `--dry-run` 搭配一个显式占位符（如 `--store-domain <domain>` 或允许 `--profile` 缺省时用一个假域名）跳过真实 profile 解析，只要能拼出语法正确的 URL 即可，不需要真实 token。哪怕仍然要求登录态，至少应该在顶层 `--help` 里注明"dry-run 仍需要已配置的 store 上下文"，而不是让用户/agent 自己踩坑才发现。

### 发现 #2 [HIGH] 模块级子命令输入错误被静默吞掉，退出码为 0
- **维度**：AI Agent + 开发者/商家
- **指标编号**：A3（错误结构化）、A4（退出码规范）
- **现象**：
  ```
  $ shoplazza bogus              → "Error: unknown command \"bogus\" for \"shoplazza\"", EXIT=2   ✅ 正确
  $ shoplazza orders bogus-cmd   → 直接打印 orders 的 help 到 stdout，无任何报错，EXIT=0            ❌ 错误
  ```
- **影响**：顶层命令的拼写错误能被正确捕获，但下钻到模块之后（`orders`/`products`/`discounts`/... 任意模块下的二级子命令）输入错误的子命令名不会报错，只是静默回退成 help，且退出码是 0（成功）。对 agent 而言，这意味着脚本里如果拼错了子命令名，程序会"看似成功"地继续跑下去，不会触发任何异常处理逻辑；对人类而言，也容易忽略这其实是一次失败的输入。
- **建议**：让所有层级的"未知子命令"都统一走和顶层一致的报错路径（非零退出码 + 结构化错误），而不是仅在最外层生效。

### 发现 #3 [MEDIUM] `--format table` / `--format pretty` 对嵌套数据基本不起作用
- **维度**：开发者/商家
- **指标编号**：B4（输出可读性）
- **现象**：对 `schema orders.list`（含嵌套 `parameters`/`response` 字段）分别用三种格式输出：
  - `table`：嵌套字段直接原样塞进单元格，超长的直接截断成 `[{"name":"cursor","in":"query","type":"string","descripti...`
  - `pretty`：把整个嵌套 JSON 原样打印成一行超长字符串（上千字符），没有做递归缩进/展开
  - 只有像 `auth status` 这种扁平 key-value 结构，table/pretty 才真正比 JSON 更好读
- **影响**：`orders`/`products`/`customers` 这些最高频模块的 `list`/`get` 类命令，返回的数据天然是嵌套结构（line_items、customer、shipping_address 等），意味着商家在终端里最常用的场景恰恰是 `--format table/pretty` 发挥不出作用的场景，实际上还是得配合 `jq`/`--jq` 才能看清数据，弱化了这个 flag 对非技术商家的价值。
- **建议**：table/pretty 格式化器需要支持嵌套对象/数组的递归展开（至少是分层缩进，而不是单行塞 JSON 字符串），或者针对已知的列表类响应做专门的表格渲染（提取常用字段为列）。

### 发现 #4 [MEDIUM] `schema` 命令不暴露参数的合法枚举值，纯黑盒硬写底层命令会在 enum 字段上抓瞎
- **维度**：AI Agent
- **指标编号**：A6（schema 自省是否够用）
- **现象**：`schema discounts.create-non-automatic --view request` 里 `discount_layer.condition_type` / `obtain_type` 等字段只标注 `"type": "string"`，没有任何 enum 候选值列表；折扣券盲测 agent 明确指出"如果不是有 +percent-code shortcut，这几个字段的合法取值完全是黑箱，只能编造占位符，没有验证依据"。反过来，`orders.list` 的 `status`/`fulfillment_status` 等字段则在 description 里用文字列出了枚举值（如 "`opened`: pending payment - `cancelled`: cancelled..."），订单盲测 agent 正是靠这段文字纠正了自己"unshipped"的错误猜测，改用了正确的 "waiting"。
- **影响**：`schema` 对枚举值的暴露程度在不同命令间不一致（有的写在 description 里，有的完全没有），导致 agent/开发者能否安全绕开 shortcut、直接用底层自动生成命令完全取决于运气。这也意味着"三层架构"里第二层（`<command>`，声称"Full parameter control for scripting"）在有些资源上实际上并不能被独立正确使用。
- **建议**：`schema` 输出统一为每个字段补充结构化的 `enum: [...]` 字段（而不是把枚举值写死在自然语言 description 里），让底层命令在没有对应 shortcut 时也能被 agent 安全构造。

### 发现 #5 [LOW-MEDIUM] 错误信息中泄漏内部实现细节
- **维度**：开发者/商家（信息安全/专业度）
- **指标编号**：A3
- **现象**：无效 UAT 登录时报错：
  ```json
  "message": "login failed: http request failed: POST /api/saiga/cli/auth/me status=401 body={\"code\":\"uat_invalid\",...}"
  ```
  直接透出了内部服务路径 `/api/saiga/cli/auth/me`（"saiga" 明显是内部服务代号）以及原始 HTTP body，而不是把它转换成面向用户的干净错误。同时该字段末尾带有一个残留的字面 `\n`，JSON 格式本身没错但读起来不够干净。
- **影响**：不算功能性缺陷，但暴露内部命名不够专业，也让错误信息比同类其它命令（如 `no profile configured`）显得不统一。
- **建议**：认证类错误统一走已有的 `hint/message/type` 清洗逻辑，去掉原始 HTTP 细节，只保留对用户有用的部分（如 "token 无效或已被吊销"）。

### 发现 #6 [LOW] 个别命令帮助文本是占位符水平
- **维度**：开发者/商家
- **指标编号**：B7（跨模块一致性）
- **现象**：`shoplazza shop metafields-resource --help` 的一行描述是 `"metafields-resource operations"`（直接把命令名 + "operations" 拼起来），相比其它模块普遍有一到两句实际语义描述（如 `webhook` 的 "Manage webhook subscriptions"），明显是自动生成时漏填的占位符。
- **影响**：不影响功能，但会让第一次浏览 `--help` 的用户觉得这块功能"不成熟/没写完"，跟整体文档质量的高水准不符。
- **建议**：补上有实际信息量的一句话描述，或在生成脚本里对这类模板做检查。

### 发现 #7 [LOW] `schema` 命令的可用 flag 集合与其它命令不一致
- **维度**：AI Agent
- **指标编号**：A9
- **现象**：订单盲测 agent 尝试对 `schema` 命令使用 `-q/--jq`（在 `list`/`+search` 等命令上可用的 jq 过滤 flag），得到 `unknown shorthand flag: 'q'`，浪费了一次探索尝试。
- **影响**：轻微，但削弱了"全局 flag 到处通用"的预期一致性。
- **建议**：要么让 `--jq` 在所有输出 JSON 的命令上通用生效，要么在顶层 `--help` 里明确哪些是"通用 flag"、哪些是"部分命令专属"。

---

## 值得保留的优点（不是问题，但值得记录，避免后续改动破坏）

1. **三层命令模型的说明写在每个模块的 `--help` 里，而不是只在顶层文档**：agent 和人类下钻到任意模块都能重新看到"这层是给谁用的"，两个盲测任务都因此在第一次 `--help` 就找对了方向，没有在模块选择上走弯路。
2. **`schema <module>.<command> --view request/response` 完全不需要认证**：这是本次两个盲测任务能在零凭据条件下仍然拼出高置信度命令的唯一原因，价值远超预期。
3. **错误统一是结构化 JSON（`ok/error/{type,message,hint}`）,且退出码按错误类型区分（2=validation，3=auth）**：两个 agent 全程没有一次"卡在看不懂的报错里"，`hint` 字段总能给出下一步可执行的命令。
4. **README 的 quickstart 命令与实际 CLI 行为完全一致**（`auth login --store-domain`、`auth status`、`--domain` 按模块申请 scope 等），没有发现文档过期的情况。
5. **`+shortcut` 层的 flag 命名和取舍非常贴近自然语言**（`+ship --tracking --company`、`+percent-code --percent --customer-segments`），两个盲测任务都在这一层几乎零试错地完成了参数构造。

---

## 本次评估未能覆盖的部分（受环境限制）

- 真实认证后的完整端到端体验（真实数据返回的可读性、写操作真正生效后的状态一致性）—— 需要真实商家账号或有效 UAT 才能验证。
- `app`/`theme-extension`/`checkout-extension` 的本地开发/热重载工作流体验（`app init` 在本环境下也被认证前置拦截）。
- 大规模并发/分页场景下的性能与稳定性。

如果后续能提供一个测试店铺的 UAT 或沙箱账号，建议优先补测发现 #1（dry-run 耦合认证）解除后，真实的"零成本试跑"体验是否符合预期，以及 `--format table` 在真实订单列表数据上的实际表现。
