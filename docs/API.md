# PMail HTTP API

**[OpenAPI 3.1 契约](openapi.yaml) 是前后端唯一接口依据**，包含请求与响应类型、认证和权限、分页、错误码、原始字节响应及兼容路径。

- `GET /api/bootstrap` 无需登录；成功时直接返回引导对象，不套响应包。匿名请求的 `domains` 为 `[]`。`need_register: true` 时，使用 Docker 日志中的一次性注册密钥调用 `POST /api/register`；成功后返回当前用户并设置会话 Cookie。
- 常规 JSON API 返回 `{errorNo, errorMsg, data}`；`errorNo: 0` 表示成功。部分旧接口在 HTTP 200 下报告应用错误，不应只检查 HTTP 状态。登录态使用 Session Cookie、`Authorization: Bearer …` 或兼容的 `Token` 请求头。
- 已登录用户保存的语言优先，否则按 `X-PMail-Lang`、`Accept-Language`、`PMAIL_DEFAULT_LANG` 顺序选择。`/api/user/me` 的 `lang` 设为 `zh-CN`、`en` 或空字符串（清除偏好）。
- 附件接口返回字节；新地址 `/api/attachments/{id}` 与 `/api/attachments/download/{id}` 使用 `<邮件 ID>.<附件 Index>`。已有 `/attachments/*` 链接仍受支持。
- `x-status: planned` 是预留契约，不是当前可用能力。C3 同路径替换的 DNS POST 响应在 `x-planned-replacement` 下；当前仍返回现有记录数组。
