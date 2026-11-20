# PMail v2.x → v3 升级说明

## 不兼容变更

1. **默认数据库改为 PostgreSQL 17。** SQLite/MySQL 单文件部署需要先执行 `deploy/deploy.sh migrate-from-sqlite` 或 `migrate-from-mysql`。
2. **配置改为环境变量优先。** 复制 `.env.example`，不要把密码和 `PMAIL_SECRET_KEY` 提交到仓库。旧 `config.json` 仍可作为迁移参考。
3. **旧 Token 头格式废弃。** 使用 `/api/token/create` 创建 API Token，通过 `Authorization: Bearer <token>` 使用；兼容的 `Token` 头仅作为过渡，不应继续依赖旧 md5 派生格式。
4. **密码哈希改为 bcrypt。** 旧 32 位 md5 密码首次成功认证后自动升级；明文密码不会写入日志或前端存储。
5. **外发改为持久化队列。** Web/587/465 对外收件人先入队，队列 worker 负责重试；本地收件人仍同步入库。检查 `/api/queue/list` 和父发件记录状态。
6. **前端重写。** UI 改为 Vue 3 + Naive UI + Tiptap，路由使用 hash 模式；旧 Element Plus/wangEditor 页面不再保留。
7. **新增数据表。** `domain`、`mailbox_address`、`api_token`、`outbound_queue`、`forward_setting`、`external_account`、`external_seen`，以及用户配额/认证字段升级。

## 推荐升级流程

```bash
cp .env.example .env
./deploy/deploy.sh init
./deploy/deploy.sh check
./deploy/deploy.sh migrate-from-sqlite /backup/pmail.db
./deploy/deploy.sh up
./deploy/deploy.sh dns
```

MySQL 使用：

```bash
./deploy/deploy.sh migrate-from-mysql 'user:password@tcp(old-host:3306)/pmail?parseTime=True'
```

迁移前备份旧库，迁移后核对管理员登录、域名/DKIM、地址、邮件数量、配额和外发队列。

## 回滚

升级前 `deploy.sh upgrade` 会生成 `backups/pmail-*.dump` 并保留上一镜像 tag。失败时脚本自动尝试回滚；手工恢复：

```bash
./deploy/deploy.sh restore backups/pmail-<timestamp>.dump
# 将 .env 中 PMAIL_IMAGE 改回上一 tag
./deploy/deploy.sh up
```

回滚前不要删除 `pg_data` 或 `pmail_config` 卷。API Token、bcrypt、队列和前端不兼容项回滚后仍应按目标版本的数据库/代码配套处理。
