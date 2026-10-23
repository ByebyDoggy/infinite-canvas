---
name: new-api-app
description: |
  在 New API「应用」板块下开发、改造、部署 AI 小程序（第三方 Web 应用）的规范。
  用户提到 New API 小程序、应用板块、上架应用、entry_url、启动码、exchange、
  授权项 / grant / 多密钥、分组选择器、把某个项目接入 New API、给应用加平台鉴权/计费、
  让游客能访问首页、只在需要身份时才跳登录、接平台托管授权页（apps/authorize）、
  给应用写用户可见的配置指南（/setup）、让历史记录可删可清空，
  或要让应用通过 App Platform Contract 一致性测试时使用。
---

# New API 小程序开发规范（契约 v3）

New API 应用板块（App Platform）是**宿主登录 + 宿主计费**的第三方 Web 应用平台。
把一个应用接进来，只需遵守四条用户侧接口 + 一条 relay 通路 + 一组 exchange 返回的**授权项**。

v3 的核心变化：**一个应用不再只拿一张密钥，而是拿一组授权项（grant）**。
每条 grant 固定 `(分组 group, 模型集合 models)`，对应平台签发的一张专用 `sk-`。
应用负责把各张密钥的模型列表合并成自己的模型目录，并按模型把请求路由到对应密钥。

## 先厘清它不是什么

不是微信小程序那种「跑在宿主运行时里的沙箱应用」。**没有运行时、没有 SDK、没有宿主渲染。**

```
面板点「打开应用」→ window.open(entry_url?code=…, '_blank')   ← 就是一个新标签页
```

因此：

| 做不到 | 原因 |
|---|---|
| 自己续期凭据 | 平台不提供，续期能力等于永久凭据 |
| 自己撤销授权 | 只能用户在面板「断开连接」 |
| 收到断开通知 | 无 webhook，只能靠 relay 的 401 事后感知 |
| 自己上架 / 审核 / 分发 | 管理员在后台注册 |
| 拿到「合并后的模型列表」 | 平台不提供，由应用自己合并各 grant 的 `/v1/models` |
| 拿到「跨分组万能令牌」 | 令牌只有一个 group 字段，跨组只能靠多张令牌 |

最贴切的类比是**网页授权（OAuth 授权码）**，只是换回来的是「`openid` + 一组真金白银的 API key」，
而且宿主**就是模型提供方**——每次 `chat/completions` 都过它，计费、限流、日志都在它那边。

## 三条链路

```
① 登录链路（浏览器）
面板点「打开应用」
  └─ POST /api/apps/:key/launch        面板会话 + X-Security-Proof(scope=app.launch)
       → entry_url（已追加 ?code=…&state=…），code 一次性、5 分钟

② 兑换链路（应用后端）
POST /api/apps/exchange  { app_key, code }
  → user_id / grant_mode / grants[]（每条含 id/label/group/models/api_key/priority）/ api_base_url / quota

③ 转发链路（应用后端）
按 model 选一条 grant → POST {api_base_url}/v1/…  Authorization: Bearer {grant.api_key}
```

授权项在**授权页**产生，不在应用里产生：

```
GET  /api/apps                  → 应用列表（含 grant_mode / grant_summary / authorized）
GET  /api/apps/:key/options     → 这个应用提供哪些 (分组, 模型)，manual 模式带用户已选
PUT  /api/apps/:key/selection   → manual 模式保存用户勾选（只返回摘要，不含密钥）
DELETE /api/apps/:key/authorization（面板）→ 断开，删掉全部 grant 令牌
```

其余路径都是平台内部实现，**不要依赖**。细节与错误码见 [`reference/contract.md`](reference/contract.md)。

## 三种授权模式

| | `inherit`（默认，v1 行为） | `auto` | `manual` |
|---|---|---|---|
| 谁决定模型面 | 用户自己的分组 | 平台管理员 | 用户自己勾选 |
| 所有用户是否一致 | 否 | **是** | 否 |
| 密钥数量 | 1 | 每个 spec 一张 | 每个勾选分组一张 |
| grant id | `legacy` | `auto:<spec.key>` | `manual:<group>` |
| 用途 | 兼容老应用 | 应用统一能力 | 用户自选 |

**没有 `grant_mode` 字段 = `inherit`**。老部署不受影响，exchange 仍会返回一条 `legacy` grant，
顶层 `api_key` 也仍然等于 `grants[0].api_key`（v1 兼容）。

## 硬约束

违反任何一条都会退化成「把平台密钥交给浏览器」或「和平台口径漂移」。

### 1. 密钥永远不进浏览器，一张密钥也不进

密钥在 exchange 时由应用**后端**取回，只用于服务端发起的模型调用。所有模型调用经应用转发，
前端**不直连**平台 relay。

v3 之后这条更严：**多张密钥不是把 key 下发给前端的理由**。代码上做三件事：

- 密钥存**独立的表**，按 `(user_id, grant_key)` 唯一，不是 `users` 上的一个字段 ——
  「读用户给前端用」这条路径在类型上就够不到它（本仓库：`model/platform_grant.go`，
  `APIKey string \`json:"-"\``）。
- 状态接口只回 grant 摘要（分组、模型、状态），永不回密钥（`PlatformGrantView`）。
- 日志只记 `grant.id`，不记 key。日志层仍要有 `scrub()` 兜底，但那是兜底不是许可。

### 2. 分支只认 `code`，不认 `message`

契约写明失败响应形如 `{success, code, message, data}`，`code` 是机器可读的错误码，
`message` 是随 `Accept-Language` 变的本地化文案，**不要用它做分支判断**。

> **现状（2026-10 实测）**：`common.ApiErrorI18n` 与 exchange 的 `writeAppExchangeError`
> 已经带 `code`（提交 `57a7abada`，2026-10-14）。但 `common.ApiError`（数据库错误、
> `app.grant_spec_invalid` 的细节）**仍然只有 message**。
>
> 所以兜底规则仍然成立：**响应里没有 `code` 就归为一个独立的「平台没说原因」错误**
> （本仓库：`app.error_unspecified`），如实告诉用户找管理员，把原文放进 `detail`。
> **绝不能**退回到拿 `message` 做分支 —— 那正是把「应用标识配错了」显示成
> 「启动码无效，请回面板重开」的成因：用户回面板拿了新启动码，一模一样地失败。

### 3. 401 按 grant 判定，只有全部失效才等同于断开

平台没有 webhook。用户在面板点「断开连接」时应用不会收到通知，只能在**下一次调 relay
拿到 401** 时才发现。v3 里 401 的含义缩小到「**这一张**密钥被撤销」：

| 上游表现 | 含义 | 应用应做 |
|---|---|---|
| 401 | 这张密钥被删/停用 | 把这条 grant 标 `revoked`；**还有其他 grant 就继续用**；全部失效才清会话 |
| 402 | 用户钱包额度不足（所有 grant 共享） | 提示充值，**不要换 grant 重试** |
| 403 | 模型不在该 grant 白名单，或 app 令牌分组异常 | 提示不支持，不重试 |
| 429 | 限流 | 退避重试（同 grant 一次，再换下一个候选） |
| 5xx / 网络 | 上游抖动 | 同 grant 重试一次，再换下一个候选 |
| 404 / 503 `model_not_found` | 该分组当前没渠道 | 给 `(grant, model)` 做冷却，换下一个候选 |

**不要**把额度查询成功当作「授权仍然有效」：`TokenAuthReadOnly` 有意放行除「被停用」以外
的所有令牌状态，所以「断开后仍能查到额度」是预期行为。

### 4. 模型目录必须应用自己合并

平台**不会**给一个合并后的 `/v1/models`。做法固定：

```
对每条 grant 并行 GET {api_base_url}/v1/models  Authorization: Bearer {grant.api_key}
  成功 → 用它返回的 data[].id
  失败 → 降级用该 grant 声明的 models，并标记 degraded
合并 → 并集去重；同一模型命中多条时按 priority 升序（相同再按 grant id 字典序）
```

- `grants[].models` 是**签发时的声明快照**，不是最终可用性；`/v1/models` 才是。
- 缓存 5 分钟，缓存键必须包含 grant 集合（增删或换分组后要失效，否则用户会继续看到
  已经不存在的模型）。本仓库：`platformModelsCacheKey`。
- 不要在每次生成时都打一遍 `/v1/models`。

### 5. 按模型路由，不跨 grant 绕过分组边界

```
route(model):
  candidates = modelIndex[model]        # 已按 priority 排序
  for g in candidates:                  # 同一模型有多条时才允许换 key
     send to {api_base_url}/v1/... with Bearer g.api_key
```

- 一次请求只用**一张**密钥；禁止把多个密钥拼进同一个请求，也禁止「多 key 轮询重放」。
- 没有任何 grant 覆盖该模型时，直接报「该模型不在本应用的授权范围内」，
  **不要回退到别的密钥**——那会绕过分组边界（本仓库：`errPlatformModelNotGranted`）。
- 声明面为空的 grant（v1 `legacy`）只做兜底，且永远排在声明过模型的 grant 之后。
- 请求体的 `model` 必须是该 grant `/v1/models` 里真实存在的名字，不做别名映射。

### 6. 应用内「退出登录」≠ 撤销平台授权

应用内退出只清**应用自己的会话**，平台密钥与授权记录**保持不动**。

反例（禁止）：退出时顺手调平台接口删密钥。那会让用户下次进入时被静默重新签发一张新密钥，
而用户以为已经收回了。

### 7. 不自建第二套账号

身份只有一个来源：exchange 返回的 `user_id`。不做注册、密码、邮箱验证、找回密码。
`role` 只描述平台内身份，**不要**当权限判据。

### 8. 平台地址来自后端，不硬编码

`api_base_url` 与 `authorize_url` 都从后端拿（本仓库：`GET /api/session/config` 与 exchange）。
exchange 返回的 `api_base_url` 可能和应用能访问到的地址不同（反代、内网域名、CDN）。
首次接入时还没调过 exchange，所以配置里要有一个兜底（`APP_AUTHORIZE_URL`，默认 `${平台}/apps/authorize`）。

### 9. 游客能看首页，登录门槛放在动作上

**这不是可选的体验优化，是契约里写明的应用侧义务**：平台不代理应用的页面，
无法决定它渲染什么。

所以：**没有会话是正常初始状态，不是错误**。启动时 `/api/session` 返回 401（或 403）
不要渲染「请从面板进入」这类死胡同页，而是渲染真正的首页。三步，缺一不可：

**① 首页与功能页不检查会话。** 打开主界面只是本地渲染，不需要会话，不要拦。

**② 需要身份的操作集中到一个 `requireLogin(intent)`**，在动作发生的那一刻调用：

```ts
function requireLogin(intent: PlatformIntent): boolean {
  if (hasSession()) return true;
  const state = encodeBase64(JSON.stringify({
    v: 1,
    returnPath: location.pathname + location.search,
    intent,                                   // 'generate' | 'publish' | …
  }));
  location.href = `${authorizeUrl}?app=${encodeURIComponent(appKey)}&state=${encodeURIComponent(state)}`;
  return false;                               // 已经跳走，调用方必须停手
}
```

调用点放在动作里，例如 `send()` 开头：`if (!requireLogin("generate")) return;`。
**拦截点放在动作处，不放在更早的地方** —— 前面全是本地渲染，本来就不需要会话。

**③ 回跳后按 `state` 还原原页面并继续原动作。** 平台只跳回注册表里登记的 `entry_url`，
**不接受调用方提供的 `redirect_uri`** —— 深链要把目标编进 `state`，回来自己还原。
顺序固定：解 `state` → 抹掉地址栏 `code` → 换会话 → 按 `returnPath` 跳回 → 按 `intent` 继续。
**顺序反了就是「登录回来落到首页」，用户以为白登一次。**

```ts
// returnPath 是自己编进 state 的，会经过用户浏览器 —— 仍要做同源校验，
// 否则 state 就成了开放重定向的入口。
if (target.startsWith("/") && !target.startsWith("//") && !target.startsWith("/\\")) {
  location.replace(target);
}
```

其余必须做对的：

| 要点 | 为什么 |
|---|---|
| **不要做登录表单** | 应用自己没有账号，唯一的进门方式是平台签发的一次性启动码。表单收集到的用户名密码无处校验 —— 那是骗用户 |
| **`state` 解不出来就当没有** | 它经过用户浏览器，可能被截断或手改。`try/catch` 返回 `null`，正常渲染，不要让整页崩掉 |
| **兑换失败回首页，不回死胡同** | 「这次没登上」不是「这个应用不能用」——带上原因回到首页 |
| **能处理「什么都没带就回来」** | 用户可能在平台确认页点了取消，应用也可能已被停用 |
| **游客态不显示额度/发布等已登录 UI** | 没有账号的人看到「剩余额度」是个谎 |
| **`state` 长度 ≤ 512 且不含控制字符** | 超了平台直接返回 `app.launch_state_invalid`（`state` 会原样拼到 `entry_url` 上） |

> **平台只管登录，不管你的页面。** 授权页 `GET {平台}/apps/authorize?app=<key>&state=<可选>`
> **不需要登录就能打开**，它自己会处理：未登录 → 跳登录页再自动回来；已登录且已连接 →
> 零点击签发跳回；首次连接 → 展示确认页（auto 只读展示规格，manual 给勾选框）过一次再验证。

### 10. 用户要自己动手的配置，说明必须放进应用

平台只负责登录与计费，**其他一切要用户自己配的东西都由应用负责解释**：装浏览器扩展、
填网关地址、签发 Token、连本地服务……这些步骤只写进仓库 README，等于没写。

| 要点 | 为什么 |
|---|---|
| **一个匿名可读的页面**（如 `/setup`） | 正在评估这个应用的人，应该在登录之前就能看到「要准备什么」 |
| **写清两个容易混的地址** | 应用调的端点和扩展连的通道常常只差几个字符（`/mcp` vs `/ws`） |
| **不要托管第三方产物** | 扩展、安装包的官方来源只有一个；应用转发副本等于让用户把刚签发的凭据交给未经追踪的构建 |
| **配置存下来 ≠ 配置能用** | 存完立刻查一次真实状态，并把两种失败说成两句不同的话 |

## 应用侧实现要点（本仓库对应位置）

| 关注点 | 位置 |
|---|---|
| exchange 解析、grant 落库、模型合并、按模型路由、401 降级 | `service/platform.go` |
| 授权项模型（密钥 `json:"-"`） | `model/platform_grant.go` |
| 授权项读写（整体替换、标记 revoked、删除） | `repository/platform_grant.go` |
| 渠道解析（用户自带 → 平台 grant → 系统渠道） | `handler/ai.go` 的 `selectAIRequestChannel` |
| 平台模型注入公开设置 | `handler/settings.go` 的 `PlatformModelChannelSetting` |
| 会话接口（config / exchange / status / quota / unlink） | `handler/platform.go` + `router/router.go` |
| 前端 state 编解码与同源校验 | `web/src/lib/platform-auth.ts` |
| 动作处登录门槛 | `web/src/hooks/use-platform-login.ts` |
| 回调编排 | `web/src/components/layout/client-root-init.tsx` |
| 配置项 | `config/config.go`：`APP_PLATFORM_BASE` / `APP_KEY` / `APP_AUTHORIZE_URL` / `APP_PLATFORM_TIMEOUT` |

**落库语义**：每次 exchange 用平台返回值**整体替换**该用户的授权项。平台侧增删 grant 后，
本地不能残留指向已撤销密钥的记录，否则路由会把请求打到一张死密钥上。

**未知字段容忍**：`grant_mode` 只用于展示，应用侧的判断依据永远是「有没有 grants」。
`grants` 为空（平台还是 v1）时退化为单密钥路径，而不是当成「没有授权」。

## 标准开发步骤

按序做。每一步都是**默认要做**的，跳过要能说出理由。

| # | 步骤 | 要点 | 详见 |
|---|---|---|---|
| 1 | **先定首页** | 没有会话时渲染什么？先写它，再写登录流程 | 硬约束 §9 |
| 2 | **不做登录表单** | 唯一的进门方式是平台签发的一次性启动码 | 硬约束 §9 |
| 3 | **把需要身份的动作列出来** | 通常就是「生成 / 保存 / 发布」。每个动作处调 `requireLogin(intent)`，**门口不拦** | 硬约束 §9 |
| 4 | **授权页地址从后端取** | `GET /api/session/config` 返回 `authorizeUrl` + `appKey`；配置里留 `APP_AUTHORIZE_URL` 兜底 | 硬约束 §8 |
| 5 | **接通回调** | `entry_url?code=…&state=…` → 解 state → 抹 code → 换会话 → 按 returnPath 跳回 → 按 intent 继续 | 硬约束 §9 |
| 6 | **落 grants 而不是单 key** | 按 `(user_id, grant_key)` 建表，exchange 整体替换；密钥列永不序列化 | 硬约束 §1 |
| 7 | **合并模型目录** | 并行拉每条 grant 的 `/v1/models`，失败降级用声明面，5 分钟缓存 | 硬约束 §4 |
| 8 | **按模型路由** | 命中不到就报「不在授权范围」，不回退别的密钥 | 硬约束 §5 |
| 9 | **按 grant 处理 401** | 只降级这一条；全部失效才清会话；402 不换 grant | 硬约束 §3 |
| 10 | **补齐失败路径** | 兑换失败回首页（带原因）、`state` 解不出当没有、只带 `code` 不带 `state` 也要能走 | 硬约束 §9 |
| 11 | **写单测钉住上面每一条** | 见下节 | [`reference/testing.md`](reference/testing.md) |
| 12 | **把用户要用的说明放进应用** | 需要用户自己动手的配置给一个匿名可读的页面 | 硬约束 §10 |

> 第 1 步和第 3 步的顺序是刻意的。先写首页会逼出一个问题：「没登录的人能看到什么？」
> 答案几乎总是「比你想象的多」。反过来先写登录流程，最后一定会得到「全站要求登录」。

## 上架与授权模式配置

管理员在 **管理 → 应用管理** 或 `POST /api/apps/admin` 注册。

| 字段 | 要点 |
|---|---|
| `key` | **上架后不可修改**。它已写进用户授权记录与应用侧配置，改名等于换一个应用 |
| `entry_url` | 根路径即可，平台会追加 `?code=…`。必须 `http(s)://` 或站内相对路径 |
| `icon_url` | 平台塞进 `<img>`，**必须匿名可访问且返回图片类型** |
| `grant_mode` | 省略 = `inherit`；`auto` / `manual` 见下 |
| `max_grants` | 1..8，缺省与上限都是 8，超过按 8 收敛 |
| `model_limits` | **只在 `inherit` 模式生效**。v3 模式下每张令牌的白名单来自各自 grant 的 `models` |

**auto 模式**：`grant_specs` 是数组，每条 `{key,label,group,models,priority}`。
平台在**保存时**就用 `/v1/models` 同一套规则校验（分组存在、模型有可用渠道、有计费配置），
任何一条不满足整单拒绝并返回 `app.grant_spec_invalid`。
改 specs 视为整体替换：key 未变的 grant 复用令牌，移除的删令牌，新增的建令牌。

**manual 模式**：`grant_allowed_groups` 是字符串数组，留空 = 用户自己可用的全部分组。
用户只能选自己可见的分组，且每个分组最多一条选择。
**manual 的 grant 以「分组」为粒度**——同一分组内多个模型合并进同一张令牌，
所以 `grant.id` 只需 `manual:<group>`，不会因同组多选产生重复令牌。

> **两侧 `key` 必须逐字一致，这是最常见的「都配对了但不通」。**
> 症状：面板跳转回来只报「请求失败」，实际是 exchange 返回 `app.not_found`。
> 用契约测试先查这个，别去翻日志。

> **接口字段不对称（管理端）**：`GET /api/apps/admin` 返回的 `grant_specs` /
> `grant_allowed_groups` 是 **JSON 字符串**（数据库原样），而提交时的请求体要
> **数组**（`AppGrantSpec[]` / `string[]`）。前端表单必须解析后再回填、序列化后再提交。

v3 应用的额度语义：应用令牌固定 `unlimited_quota=true`、`remain_quota=0`，
**只受用户钱包约束**；`apps.unlimited_quota` / `apps.remain_quota` 在 v3 模式下不再生效。

要脚本化上架：`sk-` 令牌**不能**调管理端接口，用面板登录返回的 `access_token`
（`POST /api/user/login` → `data.access_token`）或一张 scope 为 `apps:manage` 的 PAT。
字段与坑见 [`reference/contract.md`](reference/contract.md) §10。

## 开发完必须做的事

### 1. 跑契约一致性测试

```bash
npm run test:contract -- --platform <平台> --app <应用> --key <key>
```

检查项由声明驱动（`entryPath` / `iconPath` / `healthPath` / `protectedPaths`），
**不依赖任何应用私有接口**，换一个应用只需换 target。每条失败都带 owner：
`deploy` / `platform` / `app` / `network`。

### 2. 写单测覆盖新增的每个函数，全绿再收工

v3 相关的几条必须有测试钉住（本仓库已有：`service/platform_test.go`、
`handler/model_protocol_flow_test.go`、`web/src/lib/platform-auth.test.ts`）：

- exchange 失败时响应**不含** `sk-`；成功时前端永远拿不到 `sk-`（状态接口只回摘要）；
- `grants` 存在时按模型路由到**对应**密钥；无 grant 覆盖时**不回退**到别的密钥；
- 同一模型命中多条时按 priority 排序，`legacy`（声明面为空）永远排最后；
- **单条** grant 401 只把它标 `revoked`，其他 grant 仍可选；**全部**失效才清会话；
- 402/403/429 不把任何 grant 标记失效、不清会话；
- 模型目录 = 各 grant `/v1/models` 的并集去重；单条失败降级用它声明的 `models`；
- 缓存键包含 grant 集合（增删 grant 后必须重新拉取）；
- v1 回退：只有顶层 `api_key` 时落成一条 `legacy` grant；
- 无 `code` 的失败**不得**被归成某个具体业务错误；
- `app.not_found` / `app.disabled` 的提示语**不含**「回面板重新打开」。

游客路径同样要有测试钉住（读源码或直接测纯函数）：

- 首页在**没有会话时**渲染，`/api/session` 失败走首页而不是错误页；
- 需要身份的动作调用 `requireLogin(intent)`，而不是提前拦；
- `requireLogin` 把 `returnPath` 和 `intent` 编进 `state` 且 base64 编码；
- `returnPath` 只接受**站内路径**（拒绝 `//host`、`/\host`、绝对 URL，且剥掉 Tab/换行），
  否则 `state` 就是开放重定向入口；
- `decodeState` 解不出时返回 `null` 而不是抛异常；
- 页面上**没有** `<form>`，也没有 `/api/session/login` 这类端点；
- 授权页地址来自 `/api/session/config`，页面里**不出现**平台域名。

### 3. 用真浏览器走一遍「登录 → 选分组 → 用得上模型」

**这一条不是可选的。** 实测过的失败：平台会话下模型列表**静默返回空**，
界面上一直显示内置模型，用户点「连接」什么也不发生。单测和 API 级 e2e 全绿。

所以：**平台替换了某个 provider 通路之后，那条通路要端到端走一遍。**
「代码路径存在」「API 返回 200」都不等于「用户点得动」。要断言的是
**用户最终看到什么**（下拉框里真的是平台开放的模型），不是某个函数被调用过。

manual 模式还要额外断言：授权页勾选后，回到应用能看到**自己选的那些分组**的模型；
只勾一个分组时，另一个分组的模型不应出现。

### 4. 部署要点

应用必须能被**公网** `https` 访问（平台把用户浏览器重定向过去）。若走 Cloudflare 隧道：

- 生成是 SSE 流式 → `disableChunkedEncoding: false`，否则正文攒在边缘不吐；
- 会话 Cookie 带 `Secure` → 依赖反代设置 `X-Forwarded-Proto: https`。

> **已知坑**：Cloudflare 会把源站的 **502** 改写成它自己的纯文本 `error code: 502`，
> 应用精心写的错误提示根本到不了浏览器。需要向用户解释的 5xx **用 503**。

### 5. 页面测试读的是文本，别让换行符把它变成空字符串

前端没有构建步骤时，单测往往**把 HTML 当文本读**，用 `'\n  }'` 这类边界去切片。
Windows 上以 CRLF 提交，每处切片都会落空，`toContain` 在空串上照样能过。
`.gitattributes` 钉死 `*.html text eol=lf`，并优先用 `toMatch(/…/)` 而不是对切片结果做断言。

## 排障：先判断平台是不是 v3

三步，都不需要面板登录：

```bash
# ① 路由是否存在：401 = 存在（只是没会话）；404 = 这版二进制还没有 v3
curl -s -o /dev/null -w '%{http_code}\n' https://<平台>/api/apps/<key>/options

# ② 二进制里有没有 v3 符号（注意必须用 grep -a，strings|grep 在部分机器上是假阴性）
docker exec <new-api 容器> sh -lc "grep -qa -- 'grant_mode' /new-api && echo PRESENT || echo ABSENT"

# ③ 库里有表和新列
#    apps.grant_mode / grant_specs / grant_allowed_groups / max_grants
#    user_app_grants 表；tokens.app_id
```

`apps.grant_mode` 为空串表示该应用仍是 `inherit`——**这时候用户拿到的模型面仍然取决于
用户自己的分组**，不是「应用有问题」。要让「所有用户一致」，必须把应用切到 `auto` 并声明
`(分组, 模型)`。

## 常见反模式

| 反模式 | 后果 |
|---|---|
| 拿 `message` 字符串做 `if` 分支 | 换语言就崩；错误归因错，把用户送上无效的往返 |
| 前端直连 relay | 密钥进浏览器 |
| 把多张密钥合并成一个「万能 key」再转发 | 平台没有这个能力，等于绕过分组边界 |
| 假设 `grants[].models` 就是最终可用模型 | 它只是声明快照；实际以 `/v1/models` 为准 |
| 一张密钥 401 就登出整个会话 | 用户只是删了其中一张密钥，其他模型仍然可用 |
| 402 时换下一张密钥重试 | 额度是用户级、所有 grant 共享，换 key 只会再失败一次 |
| 应用内退出时删平台密钥 | 用户以为收回了，实际被静默重签 |
| 把额度查询成功当授权有效 | 撤销是最终一致的，只有 relay 401 可靠 |
| 模型目录不缓存 / 每次生成都拉一遍 | 每次请求多打一轮平台 `/v1/models` |
| 目录缓存键不含 grant 集合 | 换了分组后用户还看到旧模型 |
| 上架后想改 `key` | 不可改，只能换一个 key 重新上架 |
| **全站要求登录** | 用户看到的是一个报错页，只是换了个位置 |
| **做登录表单** | 应用自己没有账号，用户填完也没有任何地方能校验 —— 骗用户 |
| **回跳后直接跳首页** | 用户按下「生成」登录回来却回到首页，以为白登一次 |
| **`returnPath` 不做同源校验** | `state` 经过用户浏览器，应用成了开放重定向器 |
| **把 `authorizeUrl` 硬编码进前端** | 换一个部署就废；地址是平台侧的，应用不控制它 |
| **平台通路只在 API 层测过** | 线路全绿、用户点不动 |
| **配置步骤只写在 README 里** | 需要它的人已经打开了应用，不会回仓库翻 |

## 已知未做（写新功能前先看这里）

- **同一模型多条 grant 时只取第一条**：`SelectPlatformGrant` 只返回一个候选，
  调用方目前不会在 401/5xx 后换下一个候选。契约 §6.3 允许换 key 重试，但尚未实现。
- **没有 `(grant, model)` 冷却**：404/503 之后不会临时降低该候选的优先级。
- **没有手动刷新模型目录的入口**：缓存 5 分钟自然过期，`InvalidatePlatformModels`
  目前只在会话失效/额度刷新时调用。
- **平台侧分组改名不级联 `grant_key`**：`manual:<group>` 内嵌了分组名，
  改名需要同时重写 `grant_key`（平台侧 `model/group_rename.go`，该功能未在本分支）。
  应用侧不受影响，但下次 exchange 会拿到新的 grant id，需要重建目录（已按整体替换处理）。
