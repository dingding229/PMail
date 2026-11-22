# PMail Pro

PMail Pro 是 [Jinnrry/PMail](https://github.com/Jinnrry/PMail) 的增强分支，是一套可自托管的邮件服务。发布镜像位于 Docker Hub：`dingding229/pmail-pro`。

## 功能

- PostgreSQL 17 持久化存储；支持多域名、主地址、别名、发件权限与用户配额。
- 支持 SMTP、IMAP 和 POP3；外发进入队列，支持重试、退信和本地投递。
- 支持自动转发、多目标规则、手动转发、SRS 发件人改写与防循环。
- 支持收取外部 IMAP/POP3 邮件、去重及加密保存凭据。
- 管理页面支持 DNS 记录核对、API Token 管理和中英文切换；界面适配电脑、平板和手机。

## 安装：Docker Compose

### 1. 准备服务器

准备一台 `amd64` 或 `arm64` 的 Linux 服务器、可用公网 IP 和能修改 DNS 的域名。小型部署建议至少 2 GB 内存、20 GB 可用磁盘。按 [Docker 官方安装指南](https://docs.docker.com/engine/install/) 安装 Docker Engine 与 Compose v2，然后用有权限访问 Docker 的账号执行以下操作。在 Ubuntu/Debian 上，可这样安装本教程所需的工具：

```bash
sudo apt-get update && sudo apt-get install -y ca-certificates curl openssl dnsutils iproute2 netcat-openbsd
```

本文使用 `example.com`（邮件域名）、`mail.example.com`（网页域名）和 `203.0.113.10`（公网 IP）作为示例。执行前替换为自己的值，`<…>` 也是需要替换的占位符。**启动前先配置 [DNS](#dns-配置) 中的 A 记录**，并确认下表所需端口已放行。

| TCP 端口 | 用途 |
| --- | --- |
| `25` | 入站必需，用于其他服务器投递邮件；直接向外投递还需放行出站 `25` |
| `465` / `587` | 按需开放，供 SMTP 客户端提交邮件 |
| `993` / `143` | 按需开放，供 IMAP 客户端收信 |
| `995` / `110` | 按需开放，供 POP3 客户端收信；优先用 `995`，不建议公开 `110` |
| `80` / `443` | A 路线由 PMail 处理 HTTP-01 验证和网页访问；B 路线由 Nginx Proxy Manager（NPM）或你的外部反代使用 |

邮件端口默认绑定 `0.0.0.0`；请在云安全组和主机防火墙上限制未使用的端口，不要对公网发布 PostgreSQL `5432`。云服务商如封锁出站 `25`，需向服务商申请解封，改本机防火墙不能代替解封。

### 2. 下载部署文件

不需要克隆仓库，不需要 Go、Node.js 或源码。新建目录，只下载这两个文件：

```bash
mkdir pmail-pro && cd pmail-pro
```

```bash
curl -fsSLO https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.yml &&
curl -fsSLO https://raw.githubusercontent.com/dingding229/PMail/master/.env.example &&
(umask 077; cp .env.example .env)
```

### 3. 写入 `.env` 并选择网页访问方式

镜像默认是 `dingding229/pmail-pro:3.0.0-beta.3`。首次部署需要设置 `PMAIL_DOMAIN`、`PMAIL_WEB_DOMAIN`、`POSTGRES_PASSWORD`、`PMAIL_ADMIN_PASSWORD` 和 `PMAIL_SECRET_KEY`。管理员账号默认是 `admin`，可按需修改。以下命令使用三个不同的随机值，一次写入 `.env`。先替换两处示例域名；**不要在已部署的实例上重新生成这些值**。

```bash
(
  set -eu
  chmod 600 .env
  pg_password=$(openssl rand -hex 24)
  admin_password=$(openssl rand -hex 24)
  secret_key=$(openssl rand -hex 32)
  sed -i \
    -e 's/^PMAIL_DOMAIN=.*/PMAIL_DOMAIN=example.com/' \
    -e 's/^PMAIL_WEB_DOMAIN=.*/PMAIL_WEB_DOMAIN=mail.example.com/' \
    -e "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$pg_password/" \
    -e "s/^PMAIL_ADMIN_PASSWORD=.*/PMAIL_ADMIN_PASSWORD=$admin_password/" \
    -e "s/^PMAIL_SECRET_KEY=.*/PMAIL_SECRET_KEY=$secret_key/" \
    .env
)
```

把 `.env` 作为机密保管并另存备份。已有数据库卷不会随 `.env` 改密码；更换 `PMAIL_SECRET_KEY` 会影响外部邮箱凭据解密和 SRS。多实例使用不同的 `PMAIL_PROJECT_NAME` 和宿主机端口。**接管已有部署时，先核对其原 Compose 项目名**，设置为同名才能继续使用已有命名卷，不要误用新项目名。

**A：PMail 直接提供公网 HTTPS。** 公网 `80` 必须到达 PMail，以完成 HTTP-01；公网 `443` 用于浏览器访问。宿主机这两个端口不能已被反向代理占用。

```bash
sed -i \
  -e 's/^PMAIL_SSL_TYPE=.*/PMAIL_SSL_TYPE=0/' \
  -e 's/^PMAIL_HTTPS_ENABLED=.*/PMAIL_HTTPS_ENABLED=0/' \
  -e 's/^PMAIL_WEB_BIND=.*/PMAIL_WEB_BIND=0.0.0.0/' \
  -e 's/^PMAIL_HOST_HTTP_PORT=.*/PMAIL_HOST_HTTP_PORT=80/' \
  -e 's/^PMAIL_HOST_HTTPS_PORT=.*/PMAIL_HOST_HTTPS_PORT=443/' \
  .env
```

**B：推荐使用外部 NPM。** 两条路线只选其一。公网 `80/443` 由代理使用，PMail 则通过宿主机 `172.17.0.1:8080` 提供反代上游。若 NPM 直接运行在宿主机，把下方的绑定改为 `127.0.0.1`；若 Docker 网桥不是默认地址，先查看 `ip -4 addr show docker0`，填写 NPM 容器可访问的宿主机地址。

```bash
sed -i \
  -e 's/^PMAIL_SSL_TYPE=.*/PMAIL_SSL_TYPE=0/' \
  -e 's/^PMAIL_HTTPS_ENABLED=.*/PMAIL_HTTPS_ENABLED=2/' \
  -e 's/^PMAIL_WEB_BIND=.*/PMAIL_WEB_BIND=172.17.0.1/' \
  -e 's/^PMAIL_HOST_HTTP_PORT=.*/PMAIL_HOST_HTTP_PORT=8080/' \
  -e 's/^PMAIL_HOST_HTTPS_PORT=.*/PMAIL_HOST_HTTPS_PORT=8443/' \
  .env
```

B 路线的 `8443` 仅保留端口映射，**容器内并不监听网页 HTTPS**。不要将 `8443` 配为代理上游，不要向公网开放上游端口。配置代理主机的方法见下一节。

### 4. 拉取、启动并登录

```bash
docker compose pull && docker compose up -d
```

```bash
docker compose ps
```

```bash
docker compose logs -f pmail
```

等待 `postgres` 和 `pmail` 都显示 `healthy`。这个状态不代表公网 DNS、邮件端口或证书已就绪；首次证书签发时先查看日志，待证书就绪后再用浏览器打开 `https://mail.example.com/`，不要在公网 HTTP 上输入密码。使用 `.env` 的 `PMAIL_ADMIN_ACCOUNT`、`PMAIL_ADMIN_PASSWORD` 登录，在「管理 → 域名管理」核对主邮件域名，并在「设置 → 地址与别名」确认实际邮箱地址。网页域名不会自动成为收信域名。按 `Ctrl+C` 退出日志跟踪不会停止服务。

## 反向代理

### 推荐：Nginx Proxy Manager

NPM 由你另外部署，PMail 不启动或管理它。以下示例使用安装步骤的默认上游 `172.17.0.1:8080`；若修改了绑定地址或宿主机 HTTP 端口，所有代理主机都使用相同的新值。

1. **网页主机。** 在 NPM 的「主机 → 代理主机（Proxy Hosts）」创建 `mail.example.com`。协议（Scheme）选 `http`，转发地址填 `172.17.0.1`，转发端口填 `8080`。开启 WebSocket 支持（Websockets Support）和拦截常见攻击（Block Common Exploits）。在 SSL 页为这个主机申请 **Let's Encrypt** 证书，然后开启强制 SSL（Force SSL）和 HTTP/2 支持。
2. **邮件验证主机。** 为 `smtp.example.com`、`imap.example.com`、`pop.example.com` 各建一个代理主机，使用同一 HTTP 上游。**不要为这三个主机在 NPM 申请或选择证书，不要开启强制 SSL**。让公网 `80` 的 `/.well-known/acme-challenge/` 请求保留原路径转给 PMail。每个附加收信域名也需要这三个主机的 DNS 记录和代理规则。SMTP、IMAP、POP3 本身走各自的邮件端口，不通过 HTTP 代理。

可以在各个**邮件验证主机**的高级设置（Advanced）中加上以下规则，只开放验证路径，避免从这些主机访问明文登录页：

```nginx
if ($uri !~ "^/[.]well-known/acme-challenge/[^/]+$") {
    return 404;
}
```

PMail 日志出现 `TLS certificate is ready or replaced; reloading listeners` 后，等待邮件监听器重载。如果开放了 SMTP `465`，可核对证书链和主机名：

```bash
openssl s_client -connect smtp.example.com:465 -servername smtp.example.com \
  -verify_hostname smtp.example.com -verify_return_error -showcerts </dev/null
```

### 普通 Nginx 示例

若用的是宿主机上的普通 Nginx，将 B 路线的 `PMAIL_WEB_BIND` 改为 `127.0.0.1`，保留 `8080` 上游。以下配置只转发邮件域名的挑战请求，网页证书及续期由你管理。替换证书文件路径；若你的网页证书也使用 HTTP-01，请单独为 `mail.example.com` 配置自己的验证处理。

```nginx
server {
    listen 80;
    server_name smtp.example.com imap.example.com pop.example.com;
    location ^~ /.well-known/acme-challenge/ {
        proxy_pass http://127.0.0.1:8080;
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
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## DNS 配置

在 DNS 服务商处配置下表的记录。名称中的末尾点表示完整域名；有些服务商要求填相对名称，如 `smtp` 或 `@`。邮件主机使用仅 DNS 解析，不要开启只代理网页流量的 CDN；不要保留指向错误机器的 AAAA 记录。

| 类型 | 主机或地址 | 值 |
| --- | --- | --- |
| A | `mail.example.com` | `203.0.113.10` |
| A | `smtp.example.com`、`imap.example.com`、`pop.example.com`（各一条） | `203.0.113.10` |
| MX | `example.com` | 优先级 `10`，目标 `smtp.example.com.` |
| TXT（SPF） | `example.com` | `v=spf1 a mx ~all`；若域名已有 SPF，应合并授权来源，不要添加第二条 SPF |
| TXT（DKIM） | `default._domainkey.example.com` | 在「域名管理」查看该域的 DNS 记录，复制**实际选用的选择器与完整期望值** |
| TXT（DMARC） | `_dmarc.example.com` | 可先用 `v=DMARC1; p=none` 观察 SPF/DKIM 对齐情况 |
| PTR | `203.0.113.10` | 请公网 IP 服务商设置为 `smtp.example.com.`，A 记录应能解析回同一地址 |

主域名 `example.com` 的 A 记录可指向既有网站，不必为了收信覆盖它。网页启动后再从域名管理取得 DKIM；管理员也可在「设置 → API Token」创建凭据，按 [API 文档](docs/API.md) 调用域名 DNS 接口。Token 明文只在创建时显示，请安全保存。DNS 生效后可查询：

```bash
for host in mail.example.com smtp.example.com imap.example.com pop.example.com; do
  dig +short A "$host"
done
dig +short MX example.com
dig +short TXT example.com
dig +short TXT default._domainkey.example.com
dig +short TXT _dmarc.example.com
dig +short -x 203.0.113.10
```

## 邮件客户端设置

先从外部邮箱发信到 `admin@example.com`（或管理页面显示的实际地址），在 PMail 网页收件箱确认收到并回复。「已排队」不等于送达；在「设置 → 发件队列」查看结果，并确认外部邮箱真正收到回复。

邮件证书就绪后，客户端用完整地址（如 `admin@example.com`）或实际账号登录，密码为该账号的登录密码，不是 API Token。发件需开启 SMTP 认证；不要关闭证书校验。自定义端口时填写宿主机映射的端口。

| 协议 | 地址 | 端口 | 加密方式 |
| --- | --- | --- | --- |
| IMAP（推荐） | `imap.example.com` | `993` | 隐式 TLS |
| IMAP | `imap.example.com` | `143` | STARTTLS |
| SMTP（推荐） | `smtp.example.com` | `465` | 隐式 TLS |
| SMTP | `smtp.example.com` | `587` | STARTTLS |
| POP3 | `pop.example.com` | `995` | 隐式 TLS |

## 升级、备份与恢复

所有命令都在原部署目录执行，使用原来的 `.env` 和 Compose 项目名。以下转储命令适用于本项目的 `postgres` 容器；如果 `PMAIL_DB_DSN` 指向外部数据库，应改用外部数据库的备份工具。

### 备份

在改镜像标签或更新部署配置**之前**执行。数据库转储和同一时刻的 `.env` 保存在 `backups/`。另外备份 `pmail_config` 卷中的配置和证书；外部反代及其证书需单独备份。把机密备份加密后保存到另一台机器。

```bash
(
  set -eu
  umask 077
  mkdir -p backups
  dump="backups/pmail-$(date -u +%Y%m%dT%H%M%SZ).dump"
  tmp=$(mktemp backups/.pmail-dump.XXXXXXXX)
  trap 'rm -f -- "$tmp"' EXIT
  docker compose exec -T postgres sh -ec \
    'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$tmp"
  cp .env "$dump.env"
  mv -- "$tmp" "$dump"
  printf '备份文件：%s\n' "$dump"
)
```

### 升级与回滚

备份完成后，确认 Docker Hub 已发布目标标签，再更新 `.env` 中的 `PMAIL_IMAGE` 并拉取、启动。不要把 `latest` 当作固定版本。以下命令只需输入目标标签，不含仓库名：

```bash
(
  set -eu
  read -r -p '已发布的目标镜像标签：' tag
  [[ "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]] || { echo '镜像标签无效。' >&2; exit 2; }
  sed -i "s|^PMAIL_IMAGE=.*|PMAIL_IMAGE=dingding229/pmail-pro:$tag|" .env
  docker compose pull && docker compose up -d
)
```

用 `docker compose ps`、`docker compose logs --tail=100 pmail` 和实际收发验证升级。**Compose 不会自动回滚。** 如需回滚，从与转储配套的 `.dump.env` 确认升级前的镜像标签，再停止 PMail、切回该标签并拉取旧镜像：

```bash
(
  set -eu
  read -r -p '升级前的镜像标签：' tag
  [[ "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]] || { echo '镜像标签无效。' >&2; exit 2; }
  docker compose stop pmail
  sed -i "s|^PMAIL_IMAGE=.*|PMAIL_IMAGE=dingding229/pmail-pro:$tag|" .env
  docker compose pull pmail
)
```

如果数据库没有不兼容变更，可执行 `docker compose up -d` 启动旧镜像；否则保持 PMail 停止，先保存当前数据，再按下一节恢复**升级前**的备份并启动。恢复会丢失备份之后的写入。

### 恢复数据库

准备与目标镜像匹配的转储文件。此操作会停止 PMail、将备份中的对象恢复到目标数据库。SQL 恢复失败时，下面的命令不会继续启动 PMail。

```bash
(
  set -eu
  read -r -p '备份文件路径：' dump
  [[ -r "$dump" ]] || { echo '无法读取备份文件。' >&2; exit 1; }
  docker compose up -d --wait postgres
  docker compose stop pmail
  docker compose exec -T postgres sh -ec \
    'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --single-transaction --exit-on-error --no-owner --no-privileges' < "$dump"
  docker compose up -d
)
```

恢复后检查服务状态、登录和邮件数据。从上游 SQLite 或 MySQL 部署迁移，请参阅 [v2 到 v3 迁移与兼容说明](docs/UPGRADE.md)。

停止并移除本项目容器、保留数据卷：

```bash
docker compose down
```

只有确认已备份并且要**永久删除本项目数据库和配置卷**时，才执行卸载：

```bash
docker compose down -v
```

## 配置项

完整模板见 [`.env.example`](.env.example)。已使用的数据库卷不会因为改了初始化选项而重建。

| 配置项 | 说明 |
| --- | --- |
| `PMAIL_IMAGE` | PMail 镜像标签，默认 `dingding229/pmail-pro:3.0.0-beta.3` |
| `PMAIL_PROJECT_NAME` | Compose 项目名，默认 `pmail`；决定命名卷所属项目 |
| `PMAIL_DOMAIN` / `PMAIL_DOMAINS` / `PMAIL_WEB_DOMAIN` | 主邮件域名、逗号分隔的附加收信域名、网页域名 |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | PostgreSQL 初始化数据库、账号、密码 |
| `PMAIL_DB_TYPE` / `PMAIL_DB_DSN` | 基础 Compose 固定使用 PostgreSQL；可提供自定义连接串，默认连接项目内的 `postgres` |
| `PMAIL_ADMIN_ACCOUNT` / `PMAIL_ADMIN_PASSWORD` | 首次启动的管理员引导凭据；不会重置已有账号的密码 |
| `PMAIL_SECRET_KEY` | 外部邮箱凭据的加密密钥、SRS 密钥等；必须长期保存 |
| `PMAIL_WEB_BIND` / `PMAIL_MAIL_BIND` | 宿主机网页和邮件绑定地址，分别默认 `127.0.0.1` 与 `0.0.0.0` |
| `PMAIL_HTTP_PORT` / `PMAIL_HTTPS_PORT` | 容器内网页监听端口，默认 `80/443` |
| `PMAIL_HOST_*_PORT` | 宿主机端口映射，接入反代时网页通常改用 `8080/8443` |
| `PMAIL_SSL_TYPE` | `0` ACME HTTP-01、`1` 自有证书、`2` ACME DNS-01、`3` 禁用 PMail TLS（包括加密邮件端口） |
| `PMAIL_HTTPS_ENABLED` | `0/1` 启用 PMail 网页 HTTPS，`2` 仅网页 HTTP；模式 `3` 强制仅网页 HTTP |
| `PMAIL_LOG_LEVEL` | `debug`、`info`、`warn` 或 `error` |
| `PMAIL_SPAM_FILTER_LEVEL` | `0` 至 `3`，分别表示关闭、SPF 与 DKIM 均失败、SPF 失败、DKIM 失败时过滤 |
| `PMAIL_CATCH_ALL` / `PMAIL_ALLOW_USER_ALIAS` | 未知本地收件人是否交给管理员；是否允许普通用户自助创建别名 |
| `PMAIL_QUEUE_WORKERS` / `PMAIL_FETCH_WORKERS` | 外发队列和外部邮箱收取并发数；基础 Compose 不透传，需在自定义覆盖文件中传入容器环境 |
| `PMAIL_FETCH_ALLOW_PRIVATE` | 允许外部邮箱连接私网，仅用于受控环境；基础 Compose 不透传 |

## 常见问题

- **25 端口不通：** 从外部网络确认入站端口，对出站可用 `nc -vz -w 5 '<收件方的 MX 主机>' 25` 排查。联系云服务商确认是否封锁；本教程不配置外发中继。
- **证书签不出来：** 检查公网 DNS、是否有过时的 AAAA、是否把邮件域名放到了仅支持网页的代理后。A 路线使 PMail 可从公网 `80` 响应 HTTP-01；B 路线检查邮件验证主机及 PMail 日志，不要让 NPM 接管邮件域名的验证路径。
- **网页无法访问：** 查看 `docker compose ps` 与日志。A 路线核对绑定、证书和 `443`；B 路线核对代理是否能访问 `PMAIL_WEB_BIND:PMAIL_HOST_HTTP_PORT`，不要连 `8443`。
- **发信进垃圾箱：** 检查 SPF、DKIM、DMARC 对齐、PTR/rDNS 和公网 IP 声誉，参考收件方邮件头定位失败项。
- **忘记管理员密码：** 如未修改过，可查 `.env` 中的引导密码；否则由另一位可以登录的管理员在「用户管理」重置。修改 `.env` 的引导密码再重启不能重置旧账号。当前没有免登录重置入口，不要通过清空数据库卷来重置。
- **外部邮箱收取异常：** 看账户的 `last_error`，检查服务商授权码、TLS 模式及私网访问限制。

## 从源码构建与开发

仅开发和自定义镜像需要完整源码。在**源码根目录**准备一份按安装说明填写的 `.env`，不要提交该文件。基础 Compose 文件没有构建段；加载构建覆盖文件，并给本地镜像使用独立标签：

```bash
PMAIL_IMAGE=pmail-dev:local docker compose -f docker-compose.yml -f docker-compose.build.yml build pmail
```

CI 的[工作流](.github/workflows/ci.yml)分别检查前后端、执行 Go 的常规与集成测试。在 `server/` 目录使用 `go test -tags integration ./...` 运行集成测试；需要能绑定固定邮件端口的权限和独立、空白的 PostgreSQL 测试库（`PMAIL_TEST_PG_DSN`），该库账号还须能建 schema。可选的 MySQL 主程序集成测试用 `PMAIL_TEST_MYSQL_DSN` 与 `go test -tags integration . -args mysql`，不要使用生产库。

[镜像发布工作流](.github/workflows/docker.yml)构建 `linux/amd64` 和 `linux/arm64` 并推送到 Docker Hub：需配置变量 `DOCKERHUB_USERNAME`、读写权限密钥 `DOCKERHUB_TOKEN`，以及可选变量 `DOCKERHUB_IMAGE`（默认 `<DOCKERHUB_USERNAME>/pmail-pro`）。稳定 `v*` 标签更新 `latest`，预发布标签不更新；也可以手动触发并指定额外标签。
