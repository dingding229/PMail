# 旧版 PMail 迁移至 PMail Pro 0.0.1

已有 PostgreSQL 版 PMail Pro 的日常镜像升级和回滚，请按 [README 的备份和升级步骤](../README.md#升级备份与恢复) 操作。本页针对旧 SQLite/MySQL 安装。

## 不兼容变更

1. **默认数据库改为 PostgreSQL 17。** SQLite/MySQL 单文件部署需要导入数据库，不能把旧文件直接挂载成 PostgreSQL 数据卷。
2. **配置改为环境变量优先。** 根据 [安装说明](../README.md#安装docker-compose) 准备 `.env`。旧 `config.json` 可作配置参考，不要把密码或 `PMAIL_SECRET_KEY` 提交到仓库。迁移已有 Compose 实例时，保留显式 `POSTGRES_PASSWORD` 和 `PMAIL_SECRET_KEY`；保留并备份 `secrets`、`pmail_config` 卷。回到不支持密码文件的镜像时也需恢复对应的 Compose 文件。
3. **旧 Token 格式废弃。** 使用 `/api/token/create` 创建 API Token，通过 `Authorization: Bearer <token>` 使用。兼容的 `Token` 请求头仅接受新生成的 Token，不再支持旧的 MD5 派生凭据。
4. **密码哈希改为 bcrypt。** 旧 MD5 密码在首次成功认证后自动升级；明文密码不写入日志或前端存储。
5. **外发改用持久化队列。** 网页、587/465 提交的对外邮件先入队，由队列负责重试；本地收件人仍同步入库。可通过 `/api/queue/list` 核对状态。
6. **Web 前端更换。** PMail Pro 使用 `web/` 中的 React SPA；与 Go API 一起部署，同一容器托管网页与 API。
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

保留原先的 SQLite/MySQL 数据和配置备份，直到迁移验收完成。回退时，应使用升级前的旧库备份和与其匹配的旧版本，不要把新版的 PostgreSQL 转储直接用于旧版；密码哈希、API Token、队列和前端均有兼容性变更。已使用 PostgreSQL 的 PMail Pro 版本间回滚，参照 [README 的恢复步骤](../README.md#恢复数据库)。

## catch-all 与入站过滤

`PMAIL_SPAM_FILTER_LEVEL` 只在开启 `PMAIL_CATCH_ALL` 且邮件没有匹配到任何已知本地收件人、准备回退投递给管理员时生效。已知收件人不受此等级影响，即使 SPF/DKIM 未通过也不由此设置过滤。关闭 catch-all 后，未知收件人不会回退投递给管理员。

## 账户、插件与附件兼容

已有数据库用户不受引导机制变化影响。空库不再自动生成 `initial_admin_password`；在 HTTPS 网页输入 `docker compose exec pmail cat /work/config/registration_key` 查到的一次性密钥，或显式设置 `PMAIL_ADMIN_PASSWORD`。升级不会删除旧密码文件。

插件界面、加载框架、微信通知、外部垃圾邮件分类插件均不再可用，已有插件数据不会被迁移或删除。新附件路径为 `/api/attachments/{id}` 和 `/api/attachments/download/{id}`；旧 `/attachments/*` 仍兼容。各项配置与新前端开发入口见 [README](../README.md)，发布变化见 [CHANGELOG](../CHANGELOG.md)。

## 安全边界调整

- `PMAIL_LEGACY_SETUP` 默认关闭。确需遗留初始化时仅显式设 `1`，通过回环监听器提交 `X-PMail-Setup-Token`（令牌读取路径见日志）。不要把该监听器代理到公网；初始化完成会消费令牌。已有配置应优先使用环境变量迁移。
- `PMAIL_PRINT_REGISTRATION_KEY` 默认 `0`；只有显式设 `1` 才把注册密钥打印到日志，迁移自动化应改为读受保护文件。TLS/DKIM/注册密钥必须为服务用户拥有的常规文件，过宽权限会被收紧，不能收紧则拒绝读取。
- POP3 明文 USER/PASS 默认关闭；启用 995 或 110+STLS。`PMAIL_POP3_ALLOW_PLAINTEXT_AUTH=true` 为临时兼容选项。
- Web Cookie 写入现在要求同源 `Origin`/`Referer`；反代需保留 `Host`，并按实际对端填写 `PMAIL_TRUSTED_PROXIES` 以启用可信 HTTPS 与来源识别。JSON 需设置 `Content-Type: application/json`；`/api/logout` 只接受 POST。浏览器开发可继续使用 Vite 同源代理。
- API Token 新增权限范围和默认 90 天过期；显式 `never_expires` 才能签发无期限的新令牌。已有令牌继续工作，但标记 `legacy_scope`，应尽快轮换。`PMAIL_ADMIN_MAIL_ACCESS=1` 才能恢复管理员读取其他人非草稿邮件的能力，并产生额外审计记录。
- `PMAIL_TRUSTED_RELAYS` 记录可信入站中继；私网不会自动判 SPF 通过。出站拒绝私网/保留地址，用 `PMAIL_SMTP_OUTBOUND_ALLOW` 仅放行实际需要的中继；`PMAIL_SMTP_TLS_REQUIRED_DOMAINS` 中的收件域拒绝 TLS 降级。
- 外部邮箱密文为绑定端点的新格式，旧格式读取兼容并在同步时迁移。使用旧二进制回滚之前，备份原数据库；旧二进制不能解密已经迁移的凭据，必要时重新配置密码。
- `PMAIL_WEB_ROOT` 不能包含配置、密钥或数据库文件；如以前指向工作目录，请改为专用前端构建目录。
