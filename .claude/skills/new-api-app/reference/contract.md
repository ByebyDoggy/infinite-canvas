# 接口与错误码速查（契约 v3）

来源：New API App Platform Contract v3（平台侧已实现）与线上实测。
**未写的字段与路径一律视为平台内部实现，应用侧不得依赖。**

## 响应信封

```json
{ "success": true, "message": "", "data": { } }
```

- 失败时 **HTTP 仍是 200**，靠 `success` 判断。例外：鉴权类失败（401/403）与
  `X-Security-Proof` 相关失败（403）。
- `message` 是本地化文案，**不要**用它做分支。
- `code` 由 `ApiErrorI18n` / `writeAppExchangeError` 发出；
  `common.ApiError`（数据库错误、grant spec 细节）**没有 code**。

## 1. 应用列表

```
GET /api/apps          Authorization: Bearer <面板 Access Token>
```

鉴权 `UserAuth`，**仅接受面板会话**，PAT（`nap_`）会被拒（`AUTH_SESSION_REQUIRED`）。

```json
{ "success": true, "data": { "apps": [
  { "key": "canvas", "name": "无限画布", "description": "…", "icon_url": "…",
    "sort_order": 0,
    "model_limits": "gpt-image-2.5",
    "grant_mode": "auto",
    "grant_summary": { "grant_count": 2, "model_count": 3, "groups": ["GPT-Image-2.5", "default"] },
    "authorized": true, "token_id": 42 }
] } }
```

只返回 `status=1` 的应用。`authorized` 表示该用户是否已连接；`token_id` 仅已连接时出现。

`grant_summary` **只用于展示**，不参与任何判断：

| 模式 | `grant_count` | `model_count` | `groups` |
|---|---|---|---|
| `auto` | specs 条数 | 各 spec 模型数之和 | specs 里出现过的分组（去重） |
| `manual` | 允许分组数 | 这些分组当前可用模型数之和 | `grant_allowed_groups`（为空则空数组） |
| `inherit` | 0 | 0 | `[]` |

## 2. 授权选项（新增于 v3）

```
GET /api/apps/:key/options      Authorization: Bearer <面板 Access Token>
```

鉴权 `UserAuth`，仅面板会话。按应用的 `grant_mode` 返回不同结构。

### auto

```json
{ "success": true, "data": {
  "grant_mode": "auto",
  "max_grants": 8,
  "options": [
    { "key": "auto:image", "label": "图像生成", "group": "GPT-Image-2.5",
      "models": ["gpt-image-2.5"], "priority": 0 }
  ]
} }
```

### manual

```json
{ "success": true, "data": {
  "grant_mode": "manual",
  "max_grants": 8,
  "selection": [ { "group": "GPT-Image-2.5", "models": ["gpt-image-2.5"] } ],
  "options": [
    { "key": "manual:GPT-Image-2.5", "label": "GPT-Image-2.5", "group": "GPT-Image-2.5",
      "ratio": 0.2, "models": ["gpt-image-2.5"] }
  ]
} }
```

`options[].models` 是**该分组对当前用户可见**的模型（abilities + 计费配置过滤，
按名字排序）；空分组不会出现在列表里。`selection` 是已保存的选择，没有则 `[]`。

### inherit

```json
{ "grant_mode": "inherit", "max_grants": 8,
  "options": [ { "key": "legacy", "group": "", "models": [] } ] }
```

> **不要**用 `options` 决定模型目录 —— 它只是「能选什么」。真正的目录来自
> exchange 拿到的 grants 各自的 `/v1/models`。

## 3. 保存选择（manual 专用）

```
PUT /api/apps/:key/selection      Authorization: Bearer <面板 Access Token>
{ "selection": [ { "group": "GPT-Image-2.5", "models": ["gpt-image-2.5"] } ] }
```

行为：

1. 校验 group 在 `grant_allowed_groups` 内（若非空）且属于用户可用分组；
2. 校验 `models` ⊆ 该 group 对用户可见的模型；`models` 省略/为空 = 该分组全部可见模型；
3. 每个 group 最多出现一次；总条数 ≤ `max_grants`；
4. 同一事务内增删缺失/多余的 grant 令牌，并更新授权记录的 `selection`。

成功只返回摘要，**不含任何密钥**：

```json
{ "success": true, "data": {
  "grant_mode": "manual",
  "grants": [ { "id": "manual:GPT-Image-2.5", "label": "GPT-Image-2.5",
                "group": "GPT-Image-2.5", "models": ["gpt-image-2.5"], "priority": 0 } ]
} }
```

非 manual 模式调用返回 `app.selection_not_allowed`；空选择返回 `app.selection_required`。

## 4. 打开应用

```
POST /api/apps/:key/launch
Authorization: Bearer <面板 Access Token>
X-Security-Proof: <一次性再验证证明，scope=app.launch，context={"app_key":"<key>"}>
```

两道门都必须过：面板会话（PAT 一律拒）+ **首次连接时的**再验证。
已经连接过（授权记录存在且主 grant 令牌仍启用）时**不再要求证明**——
打断被压缩到「每个应用一次」。

请求体可省略：

```json
{ "state": "可选的不透明串", "selection": [ { "group": "…", "models": ["…"] } ] }
```

- `inherit` / `auto`：`selection` 必须**省略**；带上返回 `app.selection_not_allowed`。
- `manual`：带 `selection` → 先保存并签发/回收令牌，再签发启动码；
  不带但已有保存的选择 → 复用；不带且没有任何选择 → `app.selection_required`。
- `state`：原样拼到 `entry_url` 的查询串上，平台**不解释**它。
  长度 ≤ 512 且不含控制字符，否则 `app.launch_state_invalid`。

```json
{ "success": true, "data": {
  "entry_url": "https://canvas.example.com/?code=Kq7…&state=…",
  "expires_at": 1730000300,
  "token_id": 42
} }
```

- **响应里没有密钥明文**，只有 code。密钥只在 exchange 里由应用后端取回。
- `token_id` 是主 grant 的令牌 id，只为兼容 v1 读取方。
- 幂等：同一 `(user, app, grant_key)` 复用原令牌，`token_id` 不变；用户删了该密钥则重建。

`X-Security-Proof` 相关错误都是 **HTTP 403**，`code` 取值为：

| code | 含义 |
|---|---|
| `SECURITY_PROOF_REQUIRED` | 缺证明（面板会弹验证框） |
| `SECURITY_PROOF_INVALID` | 证明状态无效 |
| `SECURITY_PROOF_EXPIRED` | 证明已过期 |
| `SECURITY_PROOF_SCOPE_MISMATCH` | 证明不是给 `app.launch` 的 |
| `SECURITY_PROOF_METHOD_MISMATCH` | 验证方式不被该操作允许 |
| `SECURITY_PROOF_CONTEXT_MISMATCH` | 证明不是发给这个应用的 |
| `SECURITY_PROOF_CONSUMED` | 证明已用过 |

## 5. 兑换启动码（应用服务端调用，公开端点）

```
POST /api/apps/exchange
{ "app_key": "canvas", "code": "Kq7…" }
```

**不需要也不接受**面板会话：凭据就是 code 本身。

```json
{ "success": true, "data": {
  "user_id": 12345, "username": "creator", "role": 1, "language": "zh",
  "app_key": "canvas",
  "grant_mode": "auto",
  "api_base_url": "https://api.example.com",
  "authorize_url": "https://api.example.com/apps/authorize",
  "quota": 500000, "used_quota": 1000, "unlimited_quota": true,

  "grants": [
    { "id": "auto:image", "label": "图像生成", "group": "GPT-Image-2.5",
      "models": ["gpt-image-2.5"], "priority": 0,
      "api_key": "sk-aaaaaaaa", "unlimited_quota": true,
      "remain_quota": 0, "used_quota": 0 },
    { "id": "auto:text", "label": "文本对话", "group": "default",
      "models": ["glm-5.3", "gpt-5.2"], "priority": 1,
      "api_key": "sk-bbbbbbbb", "unlimited_quota": true,
      "remain_quota": 0, "used_quota": 0 }
  ],

  "api_key": "sk-aaaaaaaa"
} }
```

| 字段 | 用途 |
|---|---|
| `user_id` | 应用自己的用户主键，**只用它**做账号关联 |
| `username` / `role` / `language` | 仅展示；`role` **不要**当权限判据 |
| `grant_mode` | `inherit` / `auto` / `manual`，仅供展示与日志 |
| `grants[]` | **权威数据**。按 priority 升序，`grants[0]` 为主 grant |
| `grants[].id` | `auto:<spec.key>` / `manual:<group>` / `legacy`，应用侧的缓存与路由键 |
| `grants[].models` | 签发时的**声明快照**；实际可用性以该密钥的 `/v1/models` 为准 |
| `grants[].api_key` | 该 grant 的专用 `sk-`，只在服务端保存 |
| `grants[].unlimited_quota` / `remain_quota` / `used_quota` | v3 固定 `true` / `0` / 累计消耗（仅展示） |
| `api_base_url` | 所有 grant 共用的 relay 根地址（**不含** `/v1`），必须用它 |
| `authorize_url` | 平台托管授权页地址。首次接入还没调过 exchange，所以配置里要有兜底 |
| `quota` / `used_quota`（顶层） | **用户钱包**总额，不是某张密钥的额度 |
| `api_key`（顶层） | `= grants[0].api_key`，**仅为 v1 客户端兼容** |
| `unlimited_quota`（顶层） | `= grants[0].unlimited_quota`，仅为兼容 |

启动码语义：一次性（重复兑换 → `app.launch_code_invalid`）、5 分钟（过期 →
`app.launch_code_expired`）、绑定应用（`app_key` 不匹配 → `app.launch_code_invalid`）。
v3 的启动码载荷是 `{user_id, app_id}`：grant 集合在 **exchange 时**才读取，
所以「launch 之后、exchange 之前」用户改了选择也不会拿到过期令牌。

`grant_mode=manual` 且用户没有任何选择时 exchange 不可能发生（launch 已拦截）；
若 grants 为空则返回 `app.not_authorized`。

**兑换后立即丢弃 code**，不要把 code 或任何 `api_key` 写进日志、URL、前端存储。

## 6. 断开连接

```
DELETE /api/apps/:key/authorization     Authorization: Bearer <面板 Access Token>
```

删除该用户在该应用下的**全部** grant 令牌、`user_app_grants` 行与授权记录。
删除后任意一张 `api_key` 访问 relay 均返回 401（令牌缓存带失效栅栏）。
未连接时同样返回成功（幂等）。

## 7. relay 通路

```
POST {api_base_url}/v1/chat/completions    Authorization: Bearer {grant.api_key}
```

聊天补全（支持流式）、`/v1/images/generations`、`/v1/responses` 等，都是标准 OpenAI 兼容端点。

| 情况 | 表现 | 应用应做 |
|---|---|---|
| **这一张**密钥被删 / 该 grant 被撤销 | 401 | 只降级这条 grant；还有其他 grant 就继续用；**全部** 401 才清会话 |
| 用户钱包额度不足 | 402 | 提示充值，**不要**换 grant 重试 |
| 触发限流 | 429 | 退避重试（同 grant 一次，再换下一个候选） |
| 模型不在该 grant 白名单 | 403 | 标记 `(grant, model)` 不可用，换下一个候选；全失败才报「未授权该模型」 |
| app 令牌分组配置异常 | 403 `无权访问 X 分组` | 标记该 grant 不可用并告警，不要无限重试 |
| 分组当前没渠道 | 404 / 503 `model_not_found` | 对该 `(grant, model)` 做冷却，换下一个候选 |

计费、限流、白名单、日志、用量归属全部由平台按该密钥完成，**应用不自行记账**。

## 8. 查额度（两个口径，不要混用）

| 接口 | 口径 | 返回 |
|---|---|---|
| `GET {api_base_url}/api/usage/token/` | **这张密钥**（推荐） | 平台自有信封 `{code:true, data:{…}}` |
| `GET {api_base_url}/dashboard/billing/subscription` | 用户钱包总额度 | OpenAI 兼容**裸对象** |
| `GET {api_base_url}/dashboard/billing/usage` | 用户钱包已用 | OpenAI 兼容裸对象 |

三者都用 `Authorization: Bearer <api_key>`，都是只读。**第一个是平台自有信封，
后两个没有 `success` 字段，解析方式不同，不要共用一个解析器。**

> **不要轮询**。exchange 返回的 `quota` 是进入应用时的快照；需要刷新时按用户操作触发。

## 9. 错误码

### 基础（v1 起）

| `code` | 触发 | 处理 |
|---|---|---|
| `app.not_found` | key 不存在 | **两侧配置不一致**，核对 `key` |
| `app.disabled` | 应用被停用 | 联系平台管理员 |
| `app.key_invalid` | key 形态非法 | 只允许 `[a-z0-9_-]{1,64}` |
| `app.key_duplicate` | key 已被占用 | 换 key |
| `app.name_cannot_be_empty` | 名称为空 | 补名称 |
| `app.entry_url_invalid` | 入口/图标地址协议非法 | 用 `http(s)://` 或站内相对路径 |
| `app.launch_code_invalid` | code 无效 / 已用过 / 应用不匹配 | 让用户回面板重新打开 |
| `app.launch_code_expired` | code 超过 5 分钟 | 同上 |
| `app.launch_state_invalid` | `state` 过长或含控制字符 | 缩短 `state`，只放 returnPath + intent |
| `app.not_authorized` | 授权记录失效（密钥已被删） | 让用户重新打开应用 |
| `auth.user_banned` | 用户被封禁 | 提示用户 |

### v3 新增（授权项 / 选择）

| `code` | 触发 | 处理 |
|---|---|---|
| `app.grant_mode_invalid` | 模式非法，或给非 auto 应用传了 `grant_specs` / 给非 manual 传了 `grant_allowed_groups` | 管理员修正 |
| `app.grant_spec_invalid` | grant 规格校验失败（分组不存在、模型无可用渠道、无计费配置） | 管理员按 `message` 的细节修正 |
| `app.grant_too_many` | 选择条数超过 `max_grants`（上限固定 8） | 减少选择；上限不可调大 |
| `app.selection_required` | manual 模式首次进入未选择 | 打开授权页选择器 |
| `app.selection_invalid` | 选择结构非法（空 group、同一 group 重复、空 models） | 重新选择 |
| `app.selection_group_unusable` | 分组不在允许范围或用户不可用 | 刷新选项后重选 |
| `app.selection_model_unavailable` | 模型不在该分组可见范围 | 刷新选项后重选 |
| `app.selection_not_allowed` | auto/inherit 模式传了 selection | 去掉 selection |

> **`app.grant_spec_invalid` 走 `common.ApiError`，响应里没有 `code`**，只有带细节的
> `message`。这是管理端表单唯一需要读 message 的地方（显示给管理员看，不做分支）。

### 鉴权 / 令牌

| `code` | 触发 | 处理 |
|---|---|---|
| `AUTH_SESSION_REQUIRED` | 用 PAT 调了会话接口 | 用户侧接口只能用面板会话 |
| `ACCESS_TOKEN_SCOPE_DENIED` | PAT 缺少所需 scope | 换一张有 scope 的 PAT |
| `ACCESS_TOKEN_ROUTE_UNDECLARED` | 平台内部路由未声明 | 平台 bug，反馈 |
| `SECURITY_PROOF_*` | 见 §4 | 重新发起或按提示再验证 |

> **`app.not_found` 与 `app.disabled` 是两类不同的问题**：前者是部署配置错了，
> 后者是管理员停用了。都**不该**提示用户「换个启动码再试」。

## 10. 上架（管理端）

```
GET  /api/apps/admin
POST /api/apps/admin
PUT  /api/apps/admin
DELETE /api/apps/admin/:id
Authorization: Bearer <凭据>
```

鉴权 `AdminAuth` + 权限位 `apps:manage`（PAT 需要该 scope，且持有人角色 ≥ 10）。
**`sk-` 一律不行**；**会话 Cookie 不能直接用来调这个接口**——鉴权只读 `Authorization` 头。
若只有 Cookie，先 `POST /api/user/auth/refresh` 换成 access_token。

请求体（`id` 创建时忽略）：

```json
{
  "key": "canvas",
  "name": "无限画布",
  "description": "…",
  "icon_url": "https://canvas.example.com/icon.svg",
  "entry_url": "https://canvas.example.com/",
  "grant_mode": "auto",
  "grant_specs": [
    { "key": "image", "label": "图像生成", "group": "GPT-Image-2.5",
      "models": ["gpt-image-2.5"], "priority": 0 },
    { "key": "text", "label": "文本对话", "group": "default",
      "models": ["glm-5.3", "gpt-5.2"], "priority": 1 }
  ],
  "grant_allowed_groups": [],
  "max_grants": 8,
  "model_limits": "",
  "unlimited_quota": true,
  "remain_quota": 0,
  "status": 1,
  "sort_order": 0
}
```

| 字段 | 必填 | 规则 |
|---|---|---|
| `key` | ✅ | 自动转小写；`[a-z0-9_-]{1,64}`；全局唯一；**上架后不可改** |
| `name` | ✅ | ≤128 字符 |
| `entry_url` | ✅ | `http(s)://` 或站内相对路径；≤512 |
| `icon_url` | | 可空；非空时同样要过 URL 校验 |
| `description` | | 超 2048 字符会被**静默截断** |
| `grant_mode` | | `""` / `inherit`（等价）/ `auto` / `manual` |
| `grant_specs` | auto 必填 | 1..`max_grants` 条；`key` 匹配 `^[a-z0-9_-]{1,32}$` 且唯一；`label` ≤32 字符（缺省用 key）；`group` 必须存在；`models` 非空去重，且每个模型都在该 group 的 abilities 里且有计费配置；`priority` **按提交值原样保存，缺省 0**（不按数组下标自动赋值） |
| `grant_allowed_groups` | manual 可选 | 每项必须存在；留空 = 用户自己可用的全部分组 |
| `max_grants` | | 1..8，缺省 8；超出按 8 收敛 |
| `model_limits` | | **仅 inherit 模式生效**。逗号分隔，去空白去重；留空 = 不限制 |
| `status` | | 省略 = 1；`0` = 停用 |

> **字段不对称**：`GET /api/apps/admin` 返回的 `grant_specs` / `grant_allowed_groups`
> 是 **JSON 字符串**（库里原样），提交时要 **数组**。前端表单必须解析后回填、序列化后提交。

> **坑**：`unlimited_quota=false` 且 `remain_quota=0` 时，后端会**强制改回
> `unlimited_quota=true`**。这条在 v3 模式下已无意义——应用令牌一律
> `unlimited_quota=true`、`remain_quota=0`，额度只受用户钱包约束。

`POST /api/apps/admin` **不需要** `X-Security-Proof`（只有首次 launch 需要）。

PAT 的创建只能由**面板会话**发起（`POST /api/user/access_tokens`，需
`X-Security-Proof`，scope `access_token.generate`），PAT 自己不能再生 PAT。

## 11. 平台托管授权页（应用发起的登录跳转）

应用在**需要身份的那一刻**把浏览器送过去：

```
GET {平台地址}/apps/authorize?app=<key>&state=<可选，应用自定义>
```

**这个页面不需要登录就能打开**，平台自己处理：

| 进来时的状态 | 平台做什么 |
|---|---|
| 未登录 | 跳平台登录页，登录成功后**自动回到授权页**（应用与 `state` 不丢） |
| 已登录且已连接过 | **零点击**直接签发启动码并跳回 |
| 已登录但首次连接 | 展示确认页（应用名、图标、说明、模型范围、额度策略）；`auto` 只读展示 grant 规格，`manual` 给分组勾选框（受 `max_grants` 限制）；确认 + 再验证后签发 |
| 应用不存在 / 已停用 | 平台页面直接给说明，**不回应用** |

`state` **原样**附加在回跳地址（`entry_url` 的查询串）上，平台不解释它。

**平台只跳回注册表里登记的 `entry_url`，不接受调用方提供的 `redirect_uri`。**
所以深链要这么做：把目标编进 `state`，在 `entry_url` 上自己还原。

应用侧三件事（完整做法见 SKILL 硬约束 §9）：

```ts
// ① 需要身份的动作处调用，不在门口拦
function requireLogin(intent) {
  if (hasSession()) return true;
  const state = btoa(JSON.stringify({
    v: 1,
    returnPath: location.pathname + location.search,
    intent,                                  // 'generate' | 'publish' | …
  }));
  location.href = authorizeUrl +
    '?app=' + encodeURIComponent(appKey) +
    '&state=' + encodeURIComponent(state);
  return false;                              // 已经跳走
}
```

```ts
// ② 回跳：解 state → 抹掉 code → 换会话 → 按 returnPath 跳回 → 按 intent 继续
//    顺序反了就是「登录回来落到首页」，用户以为白登一次
const state = decodeState(params.get('state'));
history.replaceState(null, '', location.pathname);   // code 是 5 分钟的凭据
await exchange(params.get('code'));
resume(state);
```

**`authorize_url` 从哪来**：优先用 exchange 返回的，首次接入还没调过 exchange 时用配置兜底
（`APP_AUTHORIZE_URL`，默认 `${平台}/apps/authorize`）。
**不要硬编码进前端页面** —— 前端通过 `GET /api/session/config` 之类的接口拿。

**`state` 会经过用户浏览器，所以：**

- `returnPath` 必须做同源校验（拒绝 `//host`、`/\host`、绝对 URL，并剥掉 Tab/换行），
  否则应用成了开放重定向器；
- 解不出来（被截断、被手改）就当没有，`try/catch` 返回 `null`；
- 用户可能在确认页点取消，**要能处理「什么都没带就回来」**，那不是错误；
- 长度 ≤ 512 且不含控制字符，否则平台 `app.launch_state_invalid` 直接拒绝。

## 12. 明确不在契约内

应用不要期待这些（缺了要在应用侧做，不要等平台）：

- **合并后的 `/v1/models`** —— 平台不提供，由应用合并各 grant 的列表；
- **跨 grant 的自动选路 / 自动重试** —— 由应用路由；
- **跨分组的「万能令牌」** —— 令牌只有一个 group 字段；
- 应用自行增删 grant（只能通过 selection / launch，受 `max_grants` 与允许分组约束）；
- grant 级别的独立额度池（每张令牌仍受用户钱包约束）；
- 平台向应用推送 grant 变更事件（无 webhook，靠 401 / 主动刷新感知）；
- 应用自己的会话机制、配置存储、页面渲染；
- 应用侧主动撤销自己的授权、应用侧用量回传、自行续期 launch 凭据。

## 13. 数据模型速查（应用侧需要知道的）

平台侧：

| 表 | 关键字段 |
|---|---|
| `apps` | `key`, `entry_url`, `grant_mode`, `grant_specs`(JSON text), `grant_allowed_groups`(JSON text), `max_grants`, `model_limits`(仅 inherit) |
| `user_app_grants` | `(user_id, app_id, grant_key)` 唯一；`group`, `models`(JSON), `token_id`, `priority` |
| `user_app_authorizations` | 一行 = 一个用户对一个应用的一次授权；`token_id` 指主 grant；`grant_mode` + `selection` 快照 |
| `tokens` | 新增 `app_id`（0 = 用户自建）；`app_id>0` 的令牌在分组校验里跳过「用户可用分组」检查，只校验分组仍存在 |

应用侧需要自己存：

| 数据 | 说明 |
|---|---|
| 每个 grant 的 `api_key` | 按 `(user_id, grant_key)` 唯一；**永不序列化给前端** |
| `label` / `group` / `models` / `priority` / `status` | 路由与排障用；`status` 有 `active` / `revoked` 两态 |
| 模型目录缓存 | 键必须含 grant 集合；TTL 5 分钟 |

**落库语义**：每次 exchange 用平台返回值**整体替换**该用户的授权项。

## 14. 兼容与迁移

| 场景 | 行为 |
|---|---|
| v1 应用（不传 `grant_mode`） | 平台按 `inherit` 处理，签发一张令牌，exchange 仍带 `grants`（1 条 `legacy`） |
| v1 客户端（只读 `api_key`） | `api_key` = `grants[0].api_key`，行为不变 |
| v3 客户端 | 优先读 `grants`，忽略顶层 `api_key` |
| 已有 v1 授权记录 | 该用户下次 launch 时按 `inherit` 计划重建为一条 `legacy` grant |
| 应用从 inherit 改 auto | 下次 launch 按新 specs 重建，旧 `legacy` 令牌删除 |
