# PMail Pro 进阶说明

日常安装与使用只需看 [README](../README.md)。本页收录备份恢复、可选配置、反向代理细节和开发说明。

## 备份

在改镜像标签或更新部署配置**之前**执行。数据库转储保存在 `backups/`。同时保存当时使用的 Compose 文件，并备份两个命名卷：`pmail_config`（配置、`secret.key`、DKIM/TLS 密钥等）和 `secrets`（`db_password`）。恢复已有 PostgreSQL 数据卷时必须有匹配的数据库密码，不能删除 `secrets` 让它重新生成。外部反代及其证书需单独备份。把机密备份加密后保存到另一台机器。

```bash
(
  set -eu
  umask 077
  mkdir -p backups
  dump="backups/pmail-$(date -u +%Y%m%dT%H%M%SZ).dump"
  tmp=$(mktemp backups/.pmail-dump.XXXXXXXX)
  trap 'rm -f -- "$tmp"' EXIT
  docker compose exec -T postgres sh -ec \
    'PGPASSWORD="${POSTGRES_PASSWORD:-$(cat "${POSTGRES_PASSWORD_FILE:?}")}" exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$tmp"
  mv -- "$tmp" "$dump"
  printf '备份文件：%s\n' "$dump"
)
```

已有使用旧 Compose 的实例可在确认目标镜像支持新配置后更新 Compose 文件，但必须保留旧 `.env` 的 `POSTGRES_PASSWORD` 和 `PMAIL_SECRET_KEY`。首次运行 `init-secrets` 会使用旧密码创建文件，不会修改 PostgreSQL 账号密码；已有文件与显式密码不一致时会报错停止。对只使用新配置的部署，自动生成的密码和密钥不需要再写回 `.env`。

## 升级与回滚

升级前先按上一节备份。然后在部署目录执行：

```bash
docker compose pull
docker compose up -d
```

`docker-compose.yml` 默认使用 `latest`。要固定或回滚到某个版本，用 `.env` 中的 `PMAIL_IMAGE` 固定到目标标签（如 `dingding229/pmail-pro:v0.0.2`），再执行上面两条命令。**Compose 不会自动回滚**；若新版本修改过数据库，回滚前先按下一节恢复升级前的备份。

## 恢复数据库

准备与目标镜像匹配的转储文件。若还原到另一台机器，先用原项目名、`.env`、Compose 文件和备份的 `pmail_config`、`secrets` 恢复环境，再执行下面的数据库恢复。此操作会停止 PMail、将备份中的对象恢复到目标数据库。SQL 恢复失败时，下面的命令不会继续启动 PMail。

```bash
(
  set -eu
  read -r -p '备份文件路径：' dump
  [[ -r "$dump" ]] || { echo '无法读取备份文件。' >&2; exit 1; }
  docker compose up -d --wait postgres
  docker compose stop pmail
  docker compose exec -T postgres sh -ec \
    'PGPASSWORD="${POSTGRES_PASSWORD:-$(cat "${POSTGRES_PASSWORD_FILE:?}")}" exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --single-transaction --exit-on-error --no-owner --no-privileges' < "$dump"
  docker compose up -d
)
```

恢复后检查服务状态、登录和邮件数据。从上游 SQLite 或 MySQL 部署迁移，请参阅 [旧版迁移与兼容说明](docs/UPGRADE.md)。

停止并移除本项目容器、保留数据卷：

```bash
docker compose down
```

只有确认已备份并且要**永久删除本项目数据库、配置和凭据卷**时，才执行卸载：

```bash
docker compose down -v
```

## 可选配置（.env）

以下变量都是**可选的**，默认无需设置。需要时写进与 `docker-compose.yml` 同目录的 `.env` 文件（每行 `名称=值`），再执行 `docker compose up -d` 生效。示例见 [`.env.example`](../.env.example)。已使用的数据库卷不会因为改了初始化选项而重建。

| 配置项 | 说明 |
| --- | --- |
| `PMAIL_DOMAIN` / `PMAIL_DOMAINS` / `PMAIL_WEB_DOMAIN` | 可选。主邮件域名（不填则在首次设置网页上填写）、逗号分隔的附加收信域名、网页域名（默认 `mail.<主域名>`） |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | 数据库、账号默认 `pmail`；密码默认随机生成到 `secrets` 卷，已有部署可保留显式值 |
| `PMAIL_DB_TYPE` / `PMAIL_DB_DSN` | 基础 Compose 默认使用 PostgreSQL；显式连接串优先，默认使用 `PMAIL_DB_PASSWORD_FILE` 指向的凭据文件连接项目内的 `postgres:5432` |
| `PMAIL_ADMIN_ACCOUNT` / `PMAIL_ADMIN_PASSWORD` | 仅当数据库没有用户且显式设置密码时直接创建管理员（账号默认 `admin`）；未设置密码则使用首次注册密钥，已有用户不受影响 |
| `PMAIL_SECRET_KEY` | 应用加密与 SRS 密钥等；留空时生成 `pmail_config` 卷内的 `secret.key`，必须与数据一并保留 |
| `PMAIL_WEB_MODE` | `proxy` 默认仅提供网页 HTTP；公网直连使用覆盖文件选择 `public`。两种模式都默认 HTTP-01 申请邮件证书 |
| `PMAIL_WEB_ROOT` | 静态前端构建产物目录，默认 `/work/web`；本地可用相对路径（相对进程工作目录）。与配置、密钥或数据目录重叠时拒绝启动 |
| `PMAIL_CORS_ORIGINS` | 可选开发联调白名单，逗号分隔的完整源，例如 `http://localhost:5173`；默认禁用；跨源白名单不能绕过 Cookie 写入的同源校验 |
| `PMAIL_TRUSTED_RELAYS` | 入站 SMTP 的可信中继 CIDR，逗号分隔；默认不信任私网。仅记录信任，不替代 SPF/DKIM/DMARC。 |
| `PMAIL_POP3_ALLOW_PLAINTEXT_AUTH` | 默认关闭；110 在 STLS 前拒绝 USER/PASS。必要时仅在隔离网络临时开启。 |
| `PMAIL_MAIL_JSON_MAX_BYTES` | 发信、草稿和手动转发 JSON 请求体的字节上限。默认 32 MiB，可设 1024–134217728；解码后邮件、附件和 MIME 段各有独立上限。 |
| `PMAIL_POP3_MAX_CONNECTIONS` | 合计 POP3 110/995 的同时连接数上限（1–4096）。 |
| `PMAIL_POP3_MAX_PER_IP` | 每来源 IP 的 POP3 同时连接数上限（1–4096），IPv4-mapped IPv6 合并计数。 |
| `PMAIL_POP3_IDLE_SECONDS` | POP3 空闲断开秒数（1–3600）；断开不提交待删除标记。 |
| `PMAIL_SMTP_OUTBOUND_ALLOW` | 逗号分隔的显式内网 SMTP 目标 CIDR 或精确 MX 主机名；默认拒绝私有/保留地址。 |
| `PMAIL_SMTP_TLS_REQUIRED_DOMAINS` | 逗号分隔的精确收件域；必须验证 MX 的 STARTTLS 证书，否则不降级投递。 |
| `PMAIL_SMTP_REQUIRE_VERIFIED_TLS` | 默认 `0`：非强制域先尝试带证书验证的 STARTTLS，再尝试不验证证书的 STARTTLS，协商仍失败时可退回明文。设为 `1` 时，已宣告 STARTTLS 的 MX 验证失败不降级；未宣告的 MX 仍可明文。必须使用经验证 TLS 的收件域请配置 `PMAIL_SMTP_TLS_REQUIRED_DOMAINS`。 |
| `PMAIL_TRUSTED_PROXIES` | 逗号分隔的可信 Web 反代 CIDR，仅信任实际连接的代理。让代理覆盖 `X-Forwarded-Proto`、正确追加/清理 `X-Forwarded-For` 并保留 `Host`。容器且非 public 时，未显式设置默认 `172.16.0.0/12`，不含 `192.168.0.0/16`；public 或非容器默认空。显式值（含空值）覆盖默认值。 |
| `PMAIL_ADMIN_MAIL_ACCESS` | 设为 1 才允许管理员读取、预览、下载或转发他人非草稿邮件，访问会审计。 |
| `PMAIL_PRINT_REGISTRATION_KEY` | 仅设为 1 才兼容旧版把密钥打印到 stdout；默认只提示受保护文件路径。 |
| `PMAIL_DEFAULT_LANG` | 引导接口提供的前端默认语言，默认 `zh-CN` |
| `PMAIL_PUBLIC_IPV4` / `PMAIL_PUBLIC_IPV6` | 可选，DNS 向导使用的邮件服务器公网地址；未设置时分别从 IPv4/IPv6 出站接口探测并缓存一小时（默认 HTTPS 查询 ipify） |
| `PMAIL_DNS_RESOLVER` | 可选，DNS 向导及证书名称检测所用解析器的 `IP:端口`，IPv6 用方括号；省略时用系统解析器 |
| `PMAIL_HTTP_PORT` / `PMAIL_HTTPS_PORT` | 容器内网页监听端口，默认 `80/443` |
| `PMAIL_SSL_TYPE` | 显式设置优先于网页模式：`0` ACME HTTP-01、`1` 自有证书、`2` ACME DNS-01、`3` 禁用 PMail TLS（包括加密邮件端口） |
| `PMAIL_HTTPS_ENABLED` | 显式设置优先于网页模式：`0/1` 启用 PMail 网页 HTTPS，`2` 仅网页 HTTP；模式 `3` 强制仅网页 HTTP |
| `PMAIL_LOG_LEVEL` | `debug`、`info`、`warn` 或 `error` |
| `PMAIL_SPAM_FILTER_LEVEL` | **只作用于 catch-all（未知收件人）分支**，已知收件人不受此设置影响。`0` 关闭过滤，`1` 在 SPF 与 DKIM 均失败时过滤，`2` 在 SPF 失败时过滤，`3` 在 DKIM 失败时过滤。 |
| `PMAIL_SPAM_KEYWORDS` | 可选，逗号分隔，覆盖新入站引擎的内置中英文垃圾关键词；显式空值只关闭关键词评分。与 catch-all 前置筛查无关。 |
| `PMAIL_CATCH_ALL` / `PMAIL_ALLOW_USER_ALIAS` | `PMAIL_CATCH_ALL` 显式设置会覆盖后台的兜底开关（默认开启）；`PMAIL_ALLOW_USER_ALIAS` 控制普通用户能否自助创建别名。打开兜底时，仅已配置域名内的未知地址投给一位兜底收件人；关闭时 SMTP 返回 550 5.1.1。 |
| `PMAIL_QUEUE_WORKERS` | 外发队列并发数 |

### 兜底收件

在 **系统管理 → 入站安全** 设置兜底开关和用户。默认投给最早创建的启用管理员；所选用户被禁用或删除时也按此回退。环境变量只覆盖开关，不覆盖所选用户。邮件保留该收件人的实际信封地址，不会向所有管理员分发。

### 网页按地址分箱

地址视图筛选同一用户的邮件副本，不复制邮件，也不分别维护已读、星标或文件夹。新入站使用 SMTP 信封的收件地址，不用 `To` 猜测；兜底收件按实际兜底地址进入「其他地址」。同一次投递同时命中真实别名及兜底地址时，同一副本在对应别名和「其他地址」均可见。出站和草稿按 `From` 归属。

升级后历史邮件在后台按批回填：收件按旧 `To`／`Cc`，发件及草稿按 `From`，不能匹配的普通邮件仍在「全部」。回填完成前各地址数字可能暂不完整。删除别名只删除关联，不影响邮件；再创建同名别名不会取回旧关联。IMAP 和 POP3 不受地址筛选影响。

### DNS 向导

管理员从域名管理选择记录检测或下载 BIND 记录片段。邮件主机沿用每个域名的 `smtp.<domain>`，无需新增主机名变量；`PMAIL_WEB_DOMAIN` 只指定网页入口，反代模式下网页 A 记录默认建议本机公网 IPv4；若反代在其他服务器，换成其公网 IP，网页可开启 Cloudflare 代理。容器没检测到公网 IPv6 时，SMTP AAAA 为可选；宿主机有公网 IPv6 可手动填或设置 `PMAIL_PUBLIC_IPV6`。合并导出片段时保留现有 SOA/NS，若已有 SPF 请编辑原记录而非新增。`imap`/`pop` 建议别名用于沿用现有协议和证书主机名；使用 PMail 自动 HTTP-01 邮件证书时，按上文把这些主机也解析到正确的服务。PTR 在公网 IP 服务商处修改。

HTTP-01 邮件证书要求公网 80 端口的 `/.well-known/acme-challenge/` 通到 PMail。发布的 A 和 AAAA 都须正确；容器若无法发现宿主机的公网地址，可用 `PMAIL_PUBLIC_IPV4` / `PMAIL_PUBLIC_IPV6` 显式指定。TLS 日志会提示不匹配或暂时无法判定的名称。

## 用宿主机 Nginx 做反向代理

若用的是宿主机上的普通 Nginx，上游填 `172.17.0.1:8080`。以下配置只转发邮件域名的挑战请求，网页证书及续期由你管理。替换证书文件路径；若你的网页证书也使用 HTTP-01，请单独为 `mail.example.com` 配置自己的验证处理。

Cloudflare 的“灵活（Flexible）”和“完全（严格）”均支持，推荐“完全（严格）”。使用“灵活”时，受信代理应在 HTTP 上直接转发网页流量（保留原 `Host`，由代理覆盖 `X-Forwarded-Proto`），关闭 NPM 的 Force SSL，不要使用下方网页虚拟主机的 HTTP → HTTPS 重定向；PMail 应使用反代模式（需显式设置时用 `PMAIL_WEB_MODE=proxy`），不在源站强制跳转。灵活模式回源不加密，且当代理转发 `X-Forwarded-Proto: http` 时 PMail 不会标记 `Secure` Cookie。

```nginx
server {
    listen 80;
    server_name smtp.example.com imap.example.com pop.example.com;
    location ^~ /.well-known/acme-challenge/ {
        proxy_pass http://172.17.0.1:8080;
        proxy_set_header Host $host;
    }
    location / { return 404; }
}
server {
    listen 80;
    server_name mail.example.com;
    location / { return 308 https://mail.example.com$request_uri; }
}
server {
    listen 443 ssl;
    server_name mail.example.com;
    ssl_certificate <网页证书链路径>;
    ssl_certificate_key <网页私钥路径>;
    location / {
        proxy_pass http://172.17.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 从源码构建与开发

仅开发和自定义镜像需要完整源码。在源码根目录加载构建覆盖文件，镜像标记为 `pmail-pro:local`：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

CI 的[工作流](../.github/workflows/ci.yml)分别检查前后端、执行 Go 的常规与集成测试。在 `server/` 目录使用 `go test -tags integration ./...` 运行集成测试；需要能绑定固定邮件端口的权限和独立、空白的 PostgreSQL 测试库（`PMAIL_TEST_PG_DSN`），该库账号还须能建 schema。可选的 MySQL 主程序集成测试用 `PMAIL_TEST_MYSQL_DSN` 与 `go test -tags integration . -args mysql`，不要使用生产库。

[发布工作流](../.github/workflows/docker.yml) 仅在 `vX.Y.Z` 标签匹配源码版本时构建 `linux/amd64` 与 `linux/arm64`。它推送 `dingding229/pmail-pro` 的版本标签及 `latest`（需 `DOCKERHUB_TOKEN`；可选 `DOCKERHUB_USERNAME`），并用 `CHANGELOG.md` 的对应段落创建 GitHub Release。该 Release 上传两种架构的程序/网页压缩包及 `checksums.txt`、`checksums.txt.sig`。发布前须配对设置代码中的 Ed25519 公钥和仓库 Secret `PMAIL_RELEASE_SIGNING_KEY`；缺失或不匹配会阻止发布。

### Web 前端与 API

`web/` 是独立的 React / TypeScript / Vite SPA，`server/` 是 Go API 与邮件服务。生产环境仍是一个镜像：前端产物放在 `PMAIL_WEB_ROOT`（默认 `/work/web`），Go 服务负责 SPA 回退和 API；Go 二进制不内嵌网页。若独立运行 Makefile 打包的二进制，请从解压目录设置 `PMAIL_WEB_ROOT=./web`。网页文件暂不可用时，API 仍可用。

[OpenAPI 契约](openapi.yaml)描述接口，`GET /api/bootstrap` 无需登录。`PMAIL_DEFAULT_LANG` 设置初始语言；`PMAIL_CORS_ORIGINS` 只在真正跨源时填写允许的源。DNS 向导使用 `PMAIL_PUBLIC_IPV4` / `PMAIL_PUBLIC_IPV6` 和 `PMAIL_DNS_RESOLVER`（见上方变量表）。

真实后端联调（将端口替换为本地可访问的 PMail HTTP 地址）：

```bash
cd web
npm ci --ignore-scripts
PMAIL_API_TARGET=http://localhost:8080 npm run dev
```

Vite 在 `http://localhost:5173` 提供页面，将 `/api`（包括附件）代理给后端；该方式无需另配 CORS。仅做前端开发可运行 `npm run dev:mock`（等价于 `VITE_MOCK=1 npm run dev`），由 MSW 提供模拟接口，不必启动 Go 服务。默认生产构建不包含 MSW 或其 service worker。构建验证命令为 `npm run typecheck`、`npm run lint`、`npm run i18n:check` 和 `npm run build`。

### 从旧版迁移到 PMail Pro

- 升级前按上文备份数据库和配置卷。已有账号、邮件及域名继续使用，不需再次注册。全新实例不再生成 `initial_admin_password`，使用 [注册密钥](../README.md#3-打开网页完成设置)；已有旧文件不会被删除，请继续妥善保护。
- 插件设置、可执行插件、微信通知和外部垃圾邮件分类插件已移除。现有插件配置不再读取，不做破坏性数据库迁移；内置 catch-all SPF/DKIM 过滤仍保留。
- 新客户端使用 `/api/attachments/{id}`、`/api/attachments/download/{id}` 和文本预览接口；旧 `/attachments/*` 仍兼容。详见 [更新日志](../CHANGELOG.md#001)。

### API 与协议的安全默认值

- POP3 优先使用 995；110 必须先 STLS 才能认证，除非临时显式开启明文授权。SMTP 已认证连接在 MAIL 和 DATA 两步复核权限。
- Cookie 为 `HttpOnly`、`SameSite=Lax`；直连 HTTPS 或可信代理声明 HTTPS 时使用 `Secure`。在反向代理保持原始 `Host`，配好 `PMAIL_TRUSTED_PROXIES`，以同源路径调用 API。Cookie 写请求校验浏览器 `Origin`/`Referer`；带 JSON 请求体须使用 `Content-Type: application/json`；注销是 `POST /api/logout`。无浏览器来源的自动化请使用 Bearer token，或在使用 Cookie 时提供正确的 Origin。
- 新 API Token 默认 `read`、90 天过期，可选择 `send`（含读取及发信/草稿）、`admin`（仅管理员）或显式 `never_expires: true`。旧令牌保留权限并有 `legacy_scope` 弃用标识；请轮换。管理员默认不能读取其他人的邮件。
- 入站详情和列表的 `auth_results` 给出 SPF、DKIM、DMARC 原始判定与理由。DMARC 策略作为证据记录，不自动替认证失败邮件新增拒收/隔离动作。已存旧邮件无详细证据时返回 `null`。
- TLS/DKIM 私钥与注册密钥应为服务用户拥有的常规文件，Unix 上应为 `0600`；若用证书挂载，提供实际私钥文件而非最终路径为软链接的文件。

### 内置 ClamAV

镜像包含 Alpine 3.24 的 `clamd` 和 `freshclam`，不需要侧车、外部扫描器或 TCP 端口。默认关闭，从「系统管理 → 入站安全」启用或停止。默认由数据库保存开关；可选 `PMAIL_CLAMAV=true` 或 `PMAIL_CLAMAV=false` 显式覆盖（大小写不敏感；空或无效值不覆盖），界面会锁定开关且不会修改数据库设置。之前配置的远程 `clamd_addr` 不再使用，也不会自动启用内置扫描。

PMail 生成配置，病毒库和私有 Unix socket 放在持久配置卷中的 `/work/config/clamav/`（目录 `0700`、socket `0600`）。首次开启从 `database.clamav.net` 下载，随后约每 6 小时自动检查，管理员可立即检查。关闭不会清除病毒库。约需 300 MB 下载和 1–1.5 GB 内存；低于 2 GB 可用内存时显示提醒。扫描／拦截数字为当前进程最近 24 个小时桶中的扫描结果，不包括跳过的邮件。

未认证 SMTP 的原始入站邮件在 DATA 接受前扫描（最大 32 MiB，超时 30 秒）。检测到恶意软件返回 `554 5.7.1`，信封发件人、收件人与病毒名写入拒收审计日志。下载中、过载、超时或扫描服务异常时仍投递，详情显示“未进行病毒扫描”。SMTP 提交与网页发信不扫描。旧镜像的程序自更新不会安装 APK；首次使用此功能前请升级整个容器镜像。

### Telegram 私聊推送

在管理后台验证并保存自己的 Bot Token（加密保存，不能从 API 读回），再到个人设置生成绑定链接。在 Telegram 私聊中完成 `/start`，发送 `/stop` 可解绑。PMail 通过长轮询接收绑定，不需要入站端口。可设置邮件分组、只推验证码、是否推送垃圾邮件、摘要和免打扰时段。**启用推送即允许将对应邮件标题、发件人及识别出的验证码发送给 Telegram**；摘要默认关闭，高危或未分析邮件不发送摘要。

| 环境变量 | 用途 |
|---|---|
| `PMAIL_TELEGRAM_PROXY` | 可选，优先于管理后台保存的代理，支持 `http`、`https`、`socks5`。 |
| `PMAIL_TELEGRAM_API_BASE` | 可选可信 Bot API 基址，默认 `https://api.telegram.org`；非回环必须用 HTTPS。 |
| `PMAIL_PUBLIC_URL` | 可选，构建 “打开邮箱” 链接的公共 Web 地址。 |
| `PMAIL_UPDATE_CHECK` | 设为 `0` 禁用更新检查；默认每六小时检查。 |
| `PMAIL_UPDATE_RELEASES_URL` | 仅测试用发布信息源。默认用官方 GitHub API；回环 HTTP 只允许检查元数据，不能下载更新。自定义 HTTPS 源的附件及跳转需同源，仍须通过内置公钥验签。 |
| `PMAIL_DISABLE_STAGED_UPDATE` | 设为 `1` 跳过暂存程序并关闭在线安装；改 `.env` 后 `docker compose up -d` 以新环境重建应用容器。 |

### 程序自更新

进入 **系统管理 → 系统更新**，查看日志并确认版本。支持自更新的官方 Linux 镜像须带验签公钥；现有旧镜像先手动升级一次。已检查的版本需提供签名附件，才能在界面点击 **立即更新**。没有可验证的附件时仍可手动升级镜像。

PMail 通过 HTTPS 下载对应架构的程序和网页，使用内置 Ed25519 公钥校验版本绑定的清单并核对 SHA-256。数据暂存于 `/work/config/update/staged`。PMail 排空并退出，由 Compose 的 `restart: unless-stopped` 拉起同一个容器；启动时重新验签并验证程序和网页内容。新程序需要连续 60 秒通过健康探测才标记成功。连续三次启动未达标则回退到镜像程序；若单次启动超过 3 分钟仍未达标，进程会退出以触发下一次尝试。后台与日志会显示结果。更新到等于或高于暂存版本的镜像会清理暂存文件。

- 自更新只替换程序与网页，不更新 Linux 基础镜像，也不回滚数据库、队列或配置。升级前备份；数据库已迁移而需回退时，按兼容性说明恢复整套备份。
- **同机反代安全**：默认 `172.16.0.0/12` 适合只绑定网桥的标准部署。同一 Docker 主机上能够直连上游的不可信容器可能伪造转发头、影响来源 IP 限流；请按需隔离网络或仅允许反代的 `/32`、`/128`。自定义公网绑定应使用 public 模式，或显式将 `PMAIL_TRUSTED_PROXIES` 置空。
- 签名保护下载链，不保护已被攻破的 PMail 进程或能写配置卷的宿主机。发现入侵时不要依赖暂存程序；隔离容器，使用可信镜像并从可信备份恢复。
