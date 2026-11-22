# PMail v2.x → v3 迁移与兼容说明

已有 PostgreSQL 版 PMail Pro 的日常镜像升级和回滚，请按 [README 的备份和升级步骤](../README.md#升级备份与恢复) 操作。本页针对旧 SQLite/MySQL 安装。

## 不兼容变更

1. **默认数据库改为 PostgreSQL 17。** SQLite/MySQL 单文件部署需要导入数据库，不能把旧文件直接挂载成 PostgreSQL 数据卷。
2. **配置改为环境变量优先。** 根据 [安装说明](../README.md#安装docker-compose) 准备 `.env`。旧 `config.json` 可作配置参考，不要把密码或 `PMAIL_SECRET_KEY` 提交到仓库。现有 v3 Compose 实例迁到 `beta.4` 时，保留显式 `POSTGRES_PASSWORD` 和 `PMAIL_SECRET_KEY`；保留并备份 `secrets`、`pmail_config` 卷。回到不支持密码文件的镜像时也需恢复对应的 Compose 文件。
3. **旧 Token 格式废弃。** 使用 `/api/token/create` 创建 API Token，通过 `Authorization: Bearer <token>` 使用。兼容的 `Token` 请求头仅接受新生成的 Token，不再支持旧的 MD5 派生凭据。
4. **密码哈希改为 bcrypt。** 旧 MD5 密码在首次成功认证后自动升级；明文密码不写入日志或前端存储。
5. **外发改用持久化队列。** 网页、587/465 提交的对外邮件先入队，由队列负责重试；本地收件人仍同步入库。可通过 `/api/queue/list` 核对状态。
6. **前端改为 Vue 3 + Naive UI + Tiptap。** 路由采用 hash 模式，不再保留旧 Element Plus/wangEditor 页面。
7. **新增数据表。** 包括 `domain`、`mailbox_address`、`api_token`、`outbound_queue`、`forward_setting`、`external_account`、`external_seen`，以及新增的用户配额和认证字段。

## 导入旧数据库

停止旧服务并离机备份数据库及 `config/`。按 README 下载文件和填写 `.env`，**先不要启动 PMail**：迁移要求目标 PostgreSQL 为空。保留旧配置中的邮件域名、附加域名和网页域名；如果旧默认域名仍使用已经发布的 DKIM 公钥，请在首次启动前保留其私钥。使用下方命令，输入旧私钥 `config/dkim/dkim.priv` 的绝对路径（没有旧私钥则跳过，并在启动后更新 DNS）：

```bash
(
  set -eu
  read -r -p '旧 DKIM 私钥的绝对路径：' key
  [[ -f "$key" && "$key" == /* ]] || { echo '请提供私钥文件的绝对路径。' >&2; exit 1; }
  docker compose pull pmail
  docker compose run --rm --no-deps -T --volume "$key:/migration/dkim.priv:ro" \
    --entrypoint /bin/sh pmail -ec \
    'umask 077; mkdir -p /work/config/dkim; cp /migration/dkim.priv /work/config/dkim/dkim.priv; chmod 600 /work/config/dkim/dkim.priv'
)
```

启动空的 PostgreSQL，然后在下面两种导入方式中选择一种：

```bash
docker compose pull && docker compose up -d --wait postgres
```

**SQLite：** 输入已备份的 SQLite 文件的绝对路径：

```bash
(
  set -eu
  read -r -p 'SQLite 备份文件的绝对路径：' source_db
  [[ -f "$source_db" && "$source_db" == /* ]] || { echo '请提供 SQLite 文件的绝对路径。' >&2; exit 1; }
  docker compose run --rm --no-deps -T --volume "$source_db:/migration/source:ro" \
    pmail /work/pmail migrate --from-type sqlite --from-dsn /migration/source
)
```

**MySQL：** 输入 PMail 容器可达的 DSN，格式为 `user:password@tcp(<主机>:3306)/pmail?parseTime=true`。使用可读取旧库的账号，并在输入后妥善保管连接凭据。

```bash
(
  set -eu
  read -r -s -p '旧 MySQL DSN：' dsn; printf '\n'
  docker compose run --rm --no-deps -T pmail /work/pmail migrate --from-type mysql --from-dsn "$dsn"
)
```

导入成功后执行 `docker compose up -d`。用迁移后的账户登录，核对域名/DKIM、地址、邮件数量、配额和外发队列；旧 Token 需重新创建。`.env` 的引导密码不会覆盖已存在的管理员密码。

## 回滚

保留原先的 SQLite/MySQL 数据和配置备份，直到 v3 验收完成。回退到 v2 时，应使用升级前的旧库备份和与其匹配的旧版本，不要把 v3 的 PostgreSQL 转储直接用于 v2；密码哈希、API Token、队列和前端均有兼容性变更。已使用 PostgreSQL 的 PMail Pro 版本间回滚，参照 [README 的恢复步骤](../README.md#恢复数据库)。
