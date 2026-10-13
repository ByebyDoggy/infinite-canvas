---
title: 接口响应约定
description: 业务接口统一响应结构与前端处理约定
---

# 接口响应约定

后端业务接口统一返回 JSON：

```json
{
  "code": 0,
  "data": {},
  "msg": "ok"
}
```

- `code`: 业务状态码，`0` 表示成功，非 `0` 表示失败。
- `data`: 业务数据。失败时通常为 `null`。
- `msg`: 响应消息。成功默认为 `ok`，失败时放错误原因。

前端请求逻辑以 `code` 判断业务是否成功。当前后端业务失败也会返回 HTTP 200，前端不要只依赖 HTTP 状态码判断结果。

接口连接失败、服务不可达、返回体不是约定 JSON 时，前端按网络或接口异常处理。

## AutoDL 渠道

- `POST /api/ai/autodl/workflows` 使用上述业务响应结构。请求为 `{ baseUrl, workflowId? }`，不传上游 API Key；不带工作流 ID 返回列表，带 ID 返回详情。元数据包含 `uuid`、`name`、`kind` 和详情的 `input_rules`，不返回上游内部节点映射。
- AutoDL 视频继续使用现有 `/api/v1/videos` 创建、查询及任务响应结构。
- `/api/v1/audio/speech` 是 AI 协议代理入口。AutoDL 成功时返回 `{ provider: "autodl", audio_url, mime_type }`；失败仍使用现有 `Fail` 业务结构。其他语音协议的响应保持原样。
- 现有 `/api/v1/canvas/audio-tasks` 消费 AutoDL 的上游音频 URL，返回既有任务结构，不新增结果下载或转存。

## 渠道自定传参转译

- 沿用 `/api/v1/images/generations`、`/api/v1/images/edits`、`/api/v1/audio/speech`、`/api/v1/videos` 及现有画布任务入口。命中自定义的账号请求携带 `{ model, n, _parameterTranslation: { kind, variables } }`；视频另携带面板 `seconds`，沿用现有按秒费用计算。`kind` 为 `image`、`video`、`audio`。
- `_parameterTranslation` 是系统内部负载。后端按账号或公共渠道读取保存的原文，转译后只向供应商发送配置里的方法、地址、请求头、查询参数和请求体。
- 图片 JSON 结果归一为 `{ data: [{ url }] }`；音频 URL 归一为 `{ provider: "parameter-translation", audio_url }`。配置直接返回文件时沿用二进制响应及 MIME。失败仍用现有 `Fail` 业务结构。
- 视频沿用现有任务响应，增加 `parameter_translation: true` 标记；私有查询快照和公共渠道脚本不随响应返回。创建、查询、完成和失败均进入现有任务管理，失败不重发默认请求。

## New API 应用板块

沿用上述业务响应结构，但有一处**例外**：下游 relay 返回 401 时，`/api/session/*` 与模型代理接口返回 **HTTP 401**，因为契约里 401 是唯一的「授权已撤销」信号，前端必须能把它和普通业务失败区分开。

| 接口 | 鉴权 | 说明 |
| --- | --- | --- |
| `GET /api/session/config` | 无 | 返回 `{ appKey, authorizeUrl, platformBase, configured }`，**必须在登录守卫之外** |
| `POST /api/session/exchange` | 无（凭据就是 code） | 一次性启动码换本应用会话 |
| `GET /api/session` | 用户 | 当前平台会话状态，**不含密钥** |
| `POST /api/session/quota` | 用户 | 按用户操作触发的额度刷新，不轮询 |
| `DELETE /api/session/platform` | 用户 | 只解除本应用绑定，**不动**平台侧密钥 |

失败处理约定：

- exchange 失败一律 `{ code: 1, msg }` 并回首页，`msg` 按平台 `code` 翻译；响应没有 `code` 时按「平台没说原因」处理，提示语指向管理员，不得让用户回面板换启动码。
- 模型代理把上游状态码映射成四件不同的事：401 撤销授权并清会话、402 提示充值、429 可退避重试、403 提示模型不在白名单。只有 401 会动会话。
- 应用内「退出登录」只清本应用会话，绝不调平台接口删密钥。
