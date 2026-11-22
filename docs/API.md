# PMail HTTP API

**[OpenAPI 3.1 契约](openapi.yaml) 是前后端唯一接口依据**，包含请求与响应类型、认证和权限、分页、错误码、原始字节响应及兼容路径。

- `GET /api/bootstrap` 无需登录，成功时不套响应包；匿名请求的 `domains` 为 `[]`。`need_register: true` 时，使用 Docker 日志的密钥调用 `POST /api/register`；成功后返回当前用户并设置会话 Cookie。
- 常规 JSON API 返回 `{errorNo, errorMsg, data}`，`errorNo: 0` 表示成功。部分旧接口在 HTTP 200 下报告应用错误。未匹配的 `/api/*` 返回 HTTP 404，JSON `errorNo` 也为 `404`；已匹配接口的资源错误保持原有错误码。登录态使用 Session Cookie、`Authorization: Bearer …` 或兼容的 `Token` 请求头。已登录用户保存的语言优先，否则依次为 `X-PMail-Lang`、`Accept-Language`、`PMAIL_DEFAULT_LANG`。
- `POST /api/domain/dns` 返回本地化 DNS 向导（有 SPF 冲突或未授权时可提供 `suggested_value`）；`GET /api/domain/dns/zone` 下载建议记录。兼容 GET `/api/domain/dns` 仍返回旧数组。
- 附件新地址使用 `<邮件 ID>.<附件 Index>`；下载返回字节，`/api/attachments/preview/{id}` 返回最大 512 KiB 的文本预览。旧 `/attachments/*` 保留。
- 邮件列表与详情新增规范化地址、附件字段，保留旧格式。新增星标、未读、服务器草稿接口；通过发送请求的 `draft_id` 关联已接受的投递与草稿删除。邮件 `text` 与预览 `content` 是纯文本，不应作为 HTML 插入。
- 时间展示和排序请优先使用 RFC3339 `*_iso` 字段（带时区，未设置为 `null`），旧字段仅用于兼容。邮件列表和详情的 `auth_status` 表示收信认证结果；草稿和本服务器发出的邮件为 `not_applicable`，不会标为 `dangerous`。
