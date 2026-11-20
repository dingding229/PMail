# PMail HTTP API

除 `/api/ping` 外，API 返回统一 JSON 响应，登录态接口需要 Cookie 或 API Token。管理员接口标记为 **admin**；普通用户只能访问自己的数据。

## 认证

| 方法 | 路径 | 权限 | 请求示例 |
|---|---|---|---|
| POST | `/api/login` | public | `{"account":"alice","password":"..."}` |
| POST | `/api/logout` | user | `{}` |
| GET/POST | `/api/ping` | public | 无请求体，返回 `pong` |
| POST | `/api/token/list` | user | `{}` |
| POST | `/api/token/create` | user | `{"name":"deploy","expires_at":"2030-01-01T00:00:00Z"}` |
| POST | `/api/token/delete` | user/admin | `{"id":1}` |

使用：

```http
Authorization: Bearer pmail_...
```

Token 明文只在创建响应中返回一次。

## 邮件与分组

| 方法 | 路径 | 权限 | 请求字段 |
|---|---|---|---|
| POST | `/api/email/list` | user | `keyword`, `tag`, `current_page`, `page_size` |
| POST | `/api/email/detail` | user | `id` |
| POST | `/api/email/read` | user | `ids` |
| POST | `/api/email/del` | user | `ids`, 可选 `forcedDel` |
| POST | `/api/email/move` | user | `ids`, `group_id` 或 `group_name` |
| POST | `/api/email/send` | user | `from`, `from_address`, `to`, `cc`, `bcc`, `subject`, `text`, `html`, `attrs` |
| POST | `/api/email/forward` | user | `email_id`, `to`, `cc`, `text`, `html`, `from_address`, `include_attachments` |
| POST | `/api/group` | user | 读取默认分组 |
| POST | `/api/group/list` | user | `{}` |
| POST | `/api/group/add` | user | 分组字段 |
| POST | `/api/group/del` | user | `id` |
| GET/POST | `/attachments/{id}` | user | 路径参数 |
| GET | `/attachments/download/{id}` | user | 路径参数 |

发送响应包含逐收件人结果：

```json
{
  "status": "queued",
  "recipients": [
    {"address":"local@example.com","status":"delivered","error":""},
    {"address":"remote@example.net","status":"queued","error":""}
  ]
}
```

## 转发、规则与队列

| 方法 | 路径 | 权限 | 请求字段 |
|---|---|---|---|
| POST | `/api/forward/get` | user | `{}` |
| POST | `/api/forward/set` | user | `enabled`, `targets`, `keep_copy` |
| POST | `/api/rule/get` | user | `{}` |
| POST | `/api/rule/add` | user | `id`, `name`, `rules`, `action`, `params`, `sort` |
| POST | `/api/rule/update` | user | 同 `/api/rule/add` |
| POST | `/api/rule/del` | user | `id` |
| POST | `/api/queue/list` | user/admin | `{}` |
| POST | `/api/queue/retry` | owner/admin | `id` |
| POST | `/api/queue/delete` | owner/admin | `id` |

队列状态包括 `pending`、`sending`、`sent`、`failed`；重试和删除越权/不存在项目返回 404。

## 用户、地址与域名

| 方法 | 路径 | 权限 | 请求字段 |
|---|---|---|---|
| POST | `/api/user/me` | user | 可选 `display_name` |
| POST | `/api/user/change_password` | user | `old_password`, `new_password` |
| POST | `/api/user/info` | user | `{}` |
| POST | `/api/user/list` | admin | `current_page`, `page_size` |
| POST | `/api/user/create` | admin | `account`, `username`/`display_name`, `password`, `quota_mb` |
| POST | `/api/user/edit` | admin | `id`, `account`, `display_name`, `password`, `disabled`, `quota_mb` |
| POST | `/api/user/reset_password` | admin | `id`, `password` |
| POST | `/api/user/set_quota` | admin | `id`, `quota_mb` |
| POST | `/api/user/delete` | admin | `id` |
| POST | `/api/address/list` | user/admin | 可选 `user_id` |
| POST | `/api/address/create` | user/admin | `user_id`, `domain_id`, `local_part`, `can_send` |
| POST | `/api/address/update` | owner/admin | `id`, `can_send` |
| POST | `/api/address/delete` | owner/admin | `id` |
| POST | `/api/address/set_primary` | owner/admin | `id` |
| POST | `/api/domain/list` | admin | `{}` |
| POST | `/api/domain/create` | admin | `name` |
| POST | `/api/domain/delete` | admin | `id` |
| POST | `/api/domain/set_default` | admin | `id` |
| GET/POST | `/api/domain/dns` | admin | `id` |

## 外部邮箱

| 方法 | 路径 | 权限 | 请求字段 |
|---|---|---|---|
| POST | `/api/external/list` | user/admin | `{}` |
| POST | `/api/external/create` | user | ExternalAccount 字段 + `password` |
| POST | `/api/external/update` | owner/admin | `id`、ExternalAccount 字段；空密码不修改 |
| POST | `/api/external/delete` | owner/admin | `id` |
| POST | `/api/external/test` | user | 连接字段 + `password`，不保存 |
| POST | `/api/external/sync_now` | owner/admin | `id` |
| GET | `/api/external/presets` | user | 无请求体 |

ExternalAccount 主要字段：`name`, `protocol`, `host`, `port`, `tls_mode`, `username`, `folder`, `interval_minutes`, `initial_days`, `delete_remote`, `target_group_id`, `enabled`, `insecure_skip_verify`。

## 设置、插件与 ACME

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/settings/modify_password` | user | 与 `/api/user/change_password` 同结构 |
| POST | `/api/plugin/list` | user/admin | 插件列表 |
| POST | `/api/plugin/settings/{name}` | user/admin | 插件设置读写 |
| POST | `/api/setup/*` | setup | 初始化向导步骤 |
| GET | `/.well-known/acme-challenge/{token}` | public | ACME challenge |

所有危险操作由前端二次确认；服务端仍执行权限、归属和参数校验。
