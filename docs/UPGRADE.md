# 旧版 PMail 迁移至 PMail Pro

已有 PMail Pro 的日常升级见 [README](../README.md#升级)，备份与回滚见 [进阶说明](ADVANCED.md)。本页针对旧 SQLite/MySQL 安装。

## 从 v0.0.2 / v0.0.3 升级

- 若 v0.0.2 使用 `PMAIL_PROJECT_NAME` 或 `COMPOSE_PROJECT_NAME`，保留原值；如果已用 v0.0.3，把当前正在使用的项目名设为 `.env` 中的 `COMPOSE_PROJECT_NAME`，避免遗留变量切换到另一套同名逻辑卷。用 `docker compose config` 的 `volumes.*.name` 和 `docker volume ls` 核对现有卷，勿用 `down -v`。
- 旧 `.env` 的发布端口/绑定地址、镜像标签、`POSTGRES_USER` 与 `POSTGRES_DB` 均可继续使用。保留已初始化的 `secrets` 卷，修改 `.env` 中的 `POSTGRES_PASSWORD` 不会轮换现有数据库密码。更新器应使用相同项目名；若使用临时 `-p`、`-f` 或外部环境文件，优先手动更新，避免 sidecar 无法复现宿主机上的配置。
- 从 v0.0.3 的隐式 Docker 私网代理信任升级时，先在 `.env` 明确设置反代实际地址（`PMAIL_TRUSTED_PROXIES`）。若此前依赖浏览器地址自动设置网页主机名，而新部署使用非默认名称，请显式配置 `PMAIL_WEB_DOMAIN`；已有保存值会保留。
- 已有数据库域名表时不需要 `PMAIL_DOMAIN` 才能启动，应用继续使用数据库的默认域名。若迁移的旧库已有用户但没有域名记录，先用 `PMAIL_DOMAIN` 指定原来收信的主域名。`PMAIL_DOMAINS` 仅在域名表为空时用于导入附加域名，不能在线重写现有域名，也不代替主域名。

## 不兼容变更

1. **默认数据库改为 PostgreSQL 17。** SQLite/MySQL 单文件部署需要导入数据库，不能把旧文件直接挂载成 PostgreSQL 数据卷。
2. **不再需要配置文件。** 按 [安装说明](../README.md#安装) 下载 `docker-compose.yml` 即可；导入旧数据库后会自动沿用其中的域名。需要固定域名时，可在可选的 `.env` 中写 `PMAIL_DOMAIN`。不要把密码或 `PMAIL_SECRET_KEY` 提交到仓库。迁移已有 Compose 实例时，保留显式 `POSTGRES_PASSWORD` 和 `PMAIL_SECRET_KEY`；保留并备份 `secrets`、`pmail_config` 卷。回到不支持密码文件的镜像时也需恢复对应的 Compose 文件。
3. **旧 Token 格式废弃。** 使用 `/api/token/create` 创建 API Token，通过 `Authorization: Bearer <token>` 使用。兼容的 `Token` 请求头仅接受新生成的 Token，不再支持旧的 MD5 派生凭据。
4. **密码哈希改为 bcrypt。** 旧 MD5 密码在首次成功认证后自动升级；明文密码不写入日志或前端存储。
5. **外发改用持久化队列。** 网页、587/465 提交的对外邮件先入队，由队列负责重试；本地收件人仍同步入库。可通过 `/api/queue/list` 核对状态。
6. **Web 前端更换。** PMail Pro 使用 `web/` 中的 React SPA；与 Go API 一起部署，同一容器托管网页与 API。
7. **新增数据表。** 包括 `domain`、`mailbox_address`、`api_token`、`outbound_queue`、`forward_setting`、`external_account`、`external_seen`，以及新增的用户配额和认证字段。

## 导入旧数据库

停止旧服务并离机备份数据库及 `config/`。按 README 下载 `docker-compose.yml`，**先不要启动 PMail**：迁移要求目标 PostgreSQL 为空。保留旧配置中的邮件域名、附加域名和网页域名；如果旧默认域名仍使用已经发布的 DKIM 公钥，请在首次启动前保留其私钥。使用下方命令，输入旧私钥 `config/dkim/dkim.priv` 的绝对路径（没有旧私钥则跳过，并在启动后更新 DNS）：

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

保留原先的 SQLite/MySQL 数据和配置备份，直到迁移验收完成。回退时，应使用升级前的旧库备份和与其匹配的旧版本，不要把新版的 PostgreSQL 转储直接用于旧版；密码哈希、API Token、队列和前端均有兼容性变更。已使用 PostgreSQL 的 PMail Pro 版本间回滚，参照 [进阶说明的恢复步骤](ADVANCED.md#恢复数据库)。

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

## 邮件与同步可靠性调整

- 发信、草稿、手动转发的 JSON 默认 32 MiB（`PMAIL_MAIL_JSON_MAX_BYTES`）。超限是 HTTP 413；SMTP DATA 超限返回 552（现有 SMTP 原始消息上限仍为 30 MiB）。解码 MIME 单段/附件总量均不超过 24 MiB，应用邮件总预算 32 MiB，最多 256 段、32 级嵌套和 50 个收件人。外部邮箱超限断开连接并在同步状态显示原因。
- `PMAIL_POP3_MAX_CONNECTIONS`（默认 `128`）和 `PMAIL_POP3_MAX_PER_IP`（默认 `8`）同时作用于 110/995，`PMAIL_POP3_IDLE_SECONDS` 默认 `600`。反代或网关后的共享 IP 可能需要调整每来源连接上限。
- 规则正则改用 Go 语法，不再支持前后向断言或反向引用。保存时检查 1024 字节的正则长度、每条规则最多 32 个条件、每账户最多 100 条规则。已存不兼容规则不会崩溃投递，但应编辑替换。
- 新 IMAP UID 命名空间会改变 `UIDVALIDITY`，升级后客户端会重建邮箱缓存。`\Deleted` 只是邮箱链接标记，`EXPUNGE` 才删除该链接；为避免并发删除其他来源的数据，IMAP 不立即清理全局 MIME 原文。APPEND 仍未实现，明确返回 `NO`。
- 外部 IMAP 重新按邮箱和 UIDVALIDITY 建立游标，已有已导入记录用于去重；删除远端只针对已成功持久化的 UID。无 UIDPLUS 只打删除标记，不做全邮箱 EXPUNGE。
- 未认证 SMTP 连接只能接受本地域收件人，包括受 catch-all 策略允许的本地未知地址，不会替外域收件人兜底。新转发退信地址更长且绑定接收域；旧退信地址只有仍能查到对应发件队列记录时才有效。

## 邮件数据读写优化

- 升级会新增列表投影、附件元数据和分块、共享队列原文，以及配额和 Token 活跃时间的内部列。已有附件在后台按邮件 ID 小批回填；未处理完成的邮件仍可读，升级期间预留额外磁盘空间。原附件 JSON 保留用于原有 MIME 重建路径；不要手动删除。
- 旧队列项在后台迁为共享原文；投递器仍可投递尚未迁移的行。**旧二进制不理解共享原文引用**：生产回滚时应恢复升级前同一时点的数据库和配置备份，而不是对已迁移队列直接启动旧镜像。跨数据库离线导入会复制共享原文，并在新库后台重建附件投影。
- 列表仍接受 `current_page`/`page_size`；新客户端可发送 `cursor: ""` 开始游标模式，随后原样传回 `next_cursor`。游标模式默认不算总页数；`include_total: true` 可求精确值。列表 `desc` 为短摘要，完整正文从详情读取。
- PostgreSQL 不需要 `pg_trgm` 或额外扩展权限。邮件关键字仍使用普通 `LIKE`；如果管理员已有 `pg_trgm` 并给相同表达式配置合适的索引，规划器可以使用它；没有扩展/索引时仍可正常搜索。

## 入站分析（C8）

- `PMAIL_SPAM_FILTER_LEVEL` 仍只控制旧的 catch-all 回退前置筛查，不因 C8 扩大范围。已知收件人不受该变量影响。通过旧检查进入持久化的匿名 SMTP 入站与外部邮箱导入会做新分析，按**各用户**设置决定是否进入广告箱；垃圾判定不拒收、不会替其他收件人移动邮件，也不会自动转发判为广告的邮件。已认证的本机投递也使用分析，但不会使用外部发信 IP 的 DNSBL。
- 个人垃圾过滤与贝叶斯默认启用，默认阈值为管理员设置（初始 `6`）；用户保存设置后优先用自己的阈值。禁用个人垃圾过滤不影响风险和验证码识别。白名单优先于同时匹配的黑名单；允许名单直接放行，不验证原始地址字面值的可信度，请慎用宽泛规则。贝叶斯需每类至少 20 份手动样本才影响评分。
- `PMAIL_SPAM_KEYWORDS` 可设逗号分隔的中英文短语，覆盖内置表；显式空值关闭该特征。DNSBL 和 ClamAV 默认关闭，在管理后台开启后即时用于新投递。只把 clamd 配置到自己控制的服务（`host:port` 或绝对 Unix socket），因为扫描会发送受大小限制的邮件内容。信任中继须自己追加最上层 Received；缺可用上一跳时跳过其 DNSBL 查询。
- 历史邮件不会回溯，未分析字段为 `null`。整体分析预算为 1.5 秒，耗尽或过载会降级而不是丢信；DNSBL/ClamAV 不可用只警告。高危附件必须手工确认下载，不能在线预览，所有 Web 附件别名均执行此规则。管理开关 `risk_block_dangerous_attachments` 仅额外启用审计日志，不取消基础确认规则。此功能不替代协议客户端自身的附件安全策略。

## C9：Telegram、在线更新与富文本

- Telegram 预设关闭。已启用的安装请备份数据库和原 `PMAIL_SECRET_KEY`：更换密钥后旧 Bot 凭据无法解密，需重新填写。更换/清除 Bot 会使旧私聊绑定过期。免打扰期间消息直接丢弃；队列遇断网会重试，在网络中断或进程退出时可能重复发送。
- 更新检查可用 `PMAIL_UPDATE_CHECK=0` 关闭。在线更新器默认不启用，并使用发布镜像 `dingding229/pmail-pro-updater:latest`，不再要求源码或本地构建。刷新部署用的 Compose 文件后，在 `.env` 设置至少 32 字符的随机 `PMAIL_UPDATER_TOKEN`，执行 `docker compose --profile updater up -d`。
- 只下载文件的公网部署需要同一目录的 `docker-compose.yml`、`.env` 和公网覆盖文件；可另存为 `docker-compose.override.yml` 自动加载，也可保留 `docker-compose.public.yml` 并按 README 显式指定两份文件。不要仅使用公网覆盖文件或同时保留内容不一致的覆盖。Sidecar 只读挂载此目录，执行时会保留公网配置；自定义 `-f`、外部环境文件或特殊路径映射仍建议手动更新。
- 启用前核对 [进阶说明的更新器部分](ADVANCED.md#可选在线更新器) 中的 Docker socket 权限风险。保留状态卷追踪任务；更新器中途重启或任务 ID 丢失时返回失败/结果未知，请核对运行版本后再试，不自动回滚数据库。
- 写信内嵌图片只接受 PNG/JPEG/GIF/WebP 文件头，单个 10 MiB，每账户最多 50 MiB 临时图片。未被草稿引用、且 24 小时未使用的临时图片会清理；引用不能跨账号。旧邮件和旧草稿无须重写。签名保留允许的格式，将远程图片替换成文本占位，服务器不会自动重复追加已由编辑器插入的签名。

## C10 / C10R1：出站 TLS 默认值与数据库密码文件

- 出站 SMTP 默认恢复机会性 TLS：对非强制域，先尝试验证证书的 STARTTLS；协商失败重试不验证证书的 STARTTLS；若再次协商失败，才回退明文。未宣告 STARTTLS 的 MX 可直接回退明文。不验证证书仍加密，但不能防主动中间人攻击。
- C10 的 `PMAIL_SMTP_ALLOW_UNVERIFIED_TLS` 已删除，不再读取。若希望保留 C10 默认行为，请更新 Compose 文件并设 `PMAIL_SMTP_REQUIRE_VERIFIED_TLS=1`：只对宣告 STARTTLS 的 MX 强制校验；未宣告时仍为兼容旧 MX 允许明文。这不是全面强制 TLS。`PMAIL_SMTP_TLS_REQUIRED_DOMAINS` 的精确收件域始终要求已验证的 TLS，即使对端未宣告也不降级。外部邮箱的“跳过证书验证”是独立的账户选项，仍默认关闭。
- 数据库密码文件必须是常规文件且不超过 64 KiB。若部署用了跨目录的符号链接，请把实际文件直接挂载到 `PMAIL_DB_PASSWORD_FILE` 指向的位置，或用同目录可解析的相对链接。初始化仍以 0600 保存密码，容器共享目录只授予 PostgreSQL 穿越权限。
