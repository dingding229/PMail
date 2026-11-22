# PMail Pro

![版本 4.0.0-beta.1](https://img.shields.io/badge/PMail%20Pro-4.0.0--beta.1-2563eb)

PMail Pro 是 [Jinnrry/PMail](https://github.com/Jinnrry/PMail) 的增强分支，是一套可自托管的邮件服务。发布镜像位于 Docker Hub：`dingding229/pmail-pro`。

## 功能

- PostgreSQL 17 持久化存储；支持多域名、主地址、别名、发件权限与用户配额。
- 支持 SMTP、IMAP 和 POP3；外发进入队列，支持重试、退信和本地投递。
- 支持自动转发、多目标规则、手动转发、SRS 发件人改写与防循环。
- 支持收取外部 IMAP/POP3 邮件、去重及加密保存凭据。
- React SPA 支持桌面三栏、移动端导航、星标、未读、服务器端草稿和代码/文本附件预览。
- 管理页面提供 DNS 记录向导和 BIND 区域片段下载；界面与后端提示支持中文和英文。

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

### 2. 下载、填写域名并启动

无需克隆源码或生成密码。将 `example.com` 换成你的**主邮件域名**；网页域名自动为 `mail.example.com`。默认镜像是 `dingding229/pmail-pro:4.0.0-beta.1`，请在这个标签发布后使用。以下命令只用于**全新目录**，不要覆盖已有 `.env`。

```bash
mkdir pmail-pro && cd pmail-pro
```

```bash
curl -fsSLO https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.yml
```

```bash
(umask 077; printf '%s\n' 'PMAIL_DOMAIN=example.com' > .env)
```

**B：使用外部反代（默认，推荐）。** PMail 网页只提供 HTTP，上游默认是 `172.17.0.1:8080`；公网 `80/443` 由 NPM 或其他反代负责。NPM 运行在宿主机而非 Docker 时，在 `.env` 增加 `PMAIL_WEB_BIND=127.0.0.1`；网桥不是默认地址时，用 `ip -4 addr show docker0` 查看并填写 NPM 能访问的宿主机地址。不要把内部 `8080` 或预留的 `8443` 暴露到公网，也不要把 `8443` 配作上游。

**A：PMail 直接提供公网 HTTPS（可选）。** 不使用 NPM 时，在启动前只需将下面的覆盖文件另存为 `docker-compose.override.yml`。需要 Docker Compose **2.24.4 或更新版本**。Compose 自动加载它，将网页模式改为 `public` 并绑定 `0.0.0.0:80/443`；不用改 `.env` 里的端口。单独设置 `PMAIL_WEB_MODE=public` 不会切换 Compose 的宿主机端口。确认公网 80、443 可用，且没有被其他服务占用。已经用 `COMPOSE_FILE` 或 `-f` 自定义文件列表时，需要把覆盖文件加进该列表。

```bash
curl -fsSL https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.public.yml -o docker-compose.override.yml
```

两种方式均用同一条命令启动。Compose 会按需拉取镜像，先完成数据库密码初始化，再启动数据库和 PMail。

```bash
docker compose up -d
```

### 3. 获取注册密钥并创建管理员

v4 网页会自动进入首次注册页。已有用户的升级部署直接登录，不会重新注册。

```bash
docker compose logs pmail | grep 注册密钥
```

打开 `https://mail.example.com/`，在注册页输入日志中的注册密钥、自选账号和至少 8 个字符的密码。成功后密钥作废并自动登录。未注册时每次启动以及每 10 分钟会重新打印同一密钥。请限制访问 Docker、日志和备份，不要在公网 HTTP 上输入密钥或密码。如果显式设置了 `PMAIL_ADMIN_PASSWORD`，首次启动会直接创建管理员，使用 `PMAIL_ADMIN_ACCOUNT`（默认 `admin`）与已配置密码登录。

```bash
docker compose ps -a
```

`init-secrets` 应正常退出（退出码 `0`），`postgres` 和 `pmail` 应为 `healthy`。这不代表公网 DNS、邮件端口或证书已就绪。反代路线先完成下一节；直连路线等 PMail 签发网页证书后再访问。登录后在「管理 → 域名管理」核对主邮件域名，在「设置 → 地址与别名」确认实际地址。网页域名不会自动成为收信域名。

Compose 已用命名卷 `pmail_config` 挂载 `/work/config`（配置、DKIM/TLS、注册密钥等），数据库密码另存于 `secrets` 卷。镜像不再自动创建 `/work/config` 匿名卷；若用 `docker run` 而非 Compose，请显式挂载 `--mount type=volume,source=pmail_config,target=/work/config` 或绑定一个持久目录，否则替换容器会丢失该目录的数据。升级时请继续挂载已有配置卷，不要删除未确认归属的旧卷。

多实例需各用不同的 `PMAIL_PROJECT_NAME` 和宿主机端口。接管已有部署时须保留原项目名、`.env` 和数据卷，不要重新初始化。

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

在改镜像标签或更新部署配置**之前**执行。数据库转储和同一时刻的 `.env` 保存在 `backups/`。同时保存当时使用的 Compose 文件，并备份两个命名卷：`pmail_config`（配置、`secret.key`、DKIM/TLS 密钥等）和 `secrets`（`db_password`）。恢复已有 PostgreSQL 数据卷时必须有匹配的数据库密码，不能删除 `secrets` 让它重新生成。外部反代及其证书需单独备份。把机密备份加密后保存到另一台机器。

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
  cp .env "$dump.env"
  mv -- "$tmp" "$dump"
  printf '备份文件：%s\n' "$dump"
)
```

已有使用旧 Compose 的实例可在确认目标镜像支持新配置后更新 Compose 文件，但必须保留旧 `.env` 的 `POSTGRES_PASSWORD` 和 `PMAIL_SECRET_KEY`。首次运行 `init-secrets` 会使用旧密码创建文件，不会修改 PostgreSQL 账号密码；已有文件与显式密码不一致时会报错停止。对只使用新配置的部署，自动生成的密码和密钥不需要再写回 `.env`。

### 升级与回滚

备份完成后，确认 Docker Hub 已发布目标标签，再更新 `.env` 中的 `PMAIL_IMAGE` 并拉取、启动。不要把 `latest` 当作固定版本。以下命令只需输入目标标签，不含仓库名：

```bash
(
  set -eu
  read -r -p '已发布的目标镜像标签：' tag
  [[ "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]] || { echo '镜像标签无效。' >&2; exit 2; }
  if grep -q '^PMAIL_IMAGE=' .env; then
    sed -i "s|^PMAIL_IMAGE=.*|PMAIL_IMAGE=dingding229/pmail-pro:$tag|" .env
  else
    printf '\nPMAIL_IMAGE=dingding229/pmail-pro:%s\n' "$tag" >> .env
  fi
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
  if grep -q '^PMAIL_IMAGE=' .env; then
    sed -i "s|^PMAIL_IMAGE=.*|PMAIL_IMAGE=dingding229/pmail-pro:$tag|" .env
  else
    printf '\nPMAIL_IMAGE=dingding229/pmail-pro:%s\n' "$tag" >> .env
  fi
  docker compose pull pmail init-secrets
)
```

上述回滚命令适用于兼容新 Compose（支持 `init-secrets` 和密码文件）的镜像。若回到 `3.0.0-beta.4` 之前的版本，先恢复该版本的 Compose 文件、配套 `.env` 和凭据；旧版本不能直接使用新 Compose。若旧 `.env` 没有显式数据库密码，取回与数据卷匹配的密码再填入旧配置，不要生成新密码。

如果数据库没有不兼容变更，可执行 `docker compose up -d` 启动旧镜像；否则保持 PMail 停止，先保存当前数据，再按下一节恢复**升级前**的备份并启动。恢复会丢失备份之后的写入。

### 恢复数据库

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

恢复后检查服务状态、登录和邮件数据。从上游 SQLite 或 MySQL 部署迁移，请参阅 [v2 到 v3 迁移与兼容说明](docs/UPGRADE.md)。

停止并移除本项目容器、保留数据卷：

```bash
docker compose down
```

只有确认已备份并且要**永久删除本项目数据库、配置和凭据卷**时，才执行卸载：

```bash
docker compose down -v
```

## 配置项

精简模板见 [`.env.example`](.env.example)。已使用的数据库卷不会因为改了初始化选项而重建。

| 配置项 | 说明 |
| --- | --- |
| `PMAIL_IMAGE` | PMail 镜像标签，默认 `dingding229/pmail-pro:4.0.0-beta.1` |
| `PMAIL_PROJECT_NAME` | Compose 项目名，默认 `pmail`；决定命名卷所属项目 |
| `PMAIL_DOMAIN` / `PMAIL_DOMAINS` / `PMAIL_WEB_DOMAIN` | 主邮件域名（必填）、逗号分隔的附加收信域名、网页域名（默认 `mail.<主域名>`） |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | 数据库、账号默认 `pmail`；密码默认随机生成到 `secrets` 卷，已有部署可保留显式值 |
| `PMAIL_DB_TYPE` / `PMAIL_DB_DSN` | 基础 Compose 固定使用 PostgreSQL；显式连接串优先，默认使用 `PMAIL_DB_PASSWORD_FILE` 指向的凭据文件连接项目内的 `postgres:5432` |
| `PMAIL_ADMIN_ACCOUNT` / `PMAIL_ADMIN_PASSWORD` | 仅当数据库没有用户且显式设置密码时直接创建管理员（账号默认 `admin`）；未设置密码则使用首次注册密钥，已有用户不受影响 |
| `PMAIL_SECRET_KEY` | 外部邮箱凭据的加密密钥、SRS 密钥等；留空时生成 `pmail_config` 卷内的 `secret.key`，必须与数据一并保留 |
| `PMAIL_WEB_MODE` | `proxy` 默认仅提供网页 HTTP；公网直连使用覆盖文件选择 `public`。两种模式都默认 HTTP-01 申请邮件证书 |
| `PMAIL_WEB_ROOT` | 静态前端构建产物目录，默认 `/work/web`；本地可用相对路径（相对进程工作目录） |
| `PMAIL_CORS_ORIGINS` | 可选开发联调白名单，逗号分隔的完整源，例如 `http://localhost:5173`；默认禁用，不改变 Cookie SameSite |
| `PMAIL_DEFAULT_LANG` | 引导接口提供的前端默认语言，默认 `zh-CN` |
| `PMAIL_PUBLIC_IPV4` / `PMAIL_PUBLIC_IPV6` | 可选，DNS 向导使用的邮件服务器公网地址；未设置时分别从 IPv4/IPv6 出站接口探测并缓存一小时（默认 HTTPS 查询 ipify） |
| `PMAIL_DNS_RESOLVER` | 可选，DNS 向导检测所用解析器的 `IP:端口`，IPv6 用方括号；省略时用系统解析器 |
| `PMAIL_WEB_BIND` / `PMAIL_MAIL_BIND` | 宿主机网页和邮件绑定地址，网页默认 `172.17.0.1`（直连 `0.0.0.0`），邮件默认 `0.0.0.0` |
| `PMAIL_HTTP_PORT` / `PMAIL_HTTPS_PORT` | 容器内网页监听端口，默认 `80/443` |
| `PMAIL_HOST_*_PORT` | 宿主机端口映射，网页默认 `8080/8443`，直连覆盖为 `80/443` |
| `PMAIL_SSL_TYPE` | 显式设置优先于网页模式：`0` ACME HTTP-01、`1` 自有证书、`2` ACME DNS-01、`3` 禁用 PMail TLS（包括加密邮件端口） |
| `PMAIL_HTTPS_ENABLED` | 显式设置优先于网页模式：`0/1` 启用 PMail 网页 HTTPS，`2` 仅网页 HTTP；模式 `3` 强制仅网页 HTTP |
| `PMAIL_LOG_LEVEL` | `debug`、`info`、`warn` 或 `error` |
| `PMAIL_SPAM_FILTER_LEVEL` | **只作用于 catch-all（未知收件人）分支**，已知收件人不受此设置影响。`0` 关闭过滤，`1` 在 SPF 与 DKIM 均失败时过滤，`2` 在 SPF 失败时过滤，`3` 在 DKIM 失败时过滤。 |
| `PMAIL_CATCH_ALL` / `PMAIL_ALLOW_USER_ALIAS` | 未知本地收件人是否允许回退到管理员；是否允许普通用户自助创建别名。关闭 catch-all 后，未知收件人不再进入管理员投递分支。 |
| `PMAIL_QUEUE_WORKERS` / `PMAIL_FETCH_WORKERS` | 外发队列和外部邮箱收取并发数；基础 Compose 不透传，需在自定义覆盖文件中传入容器环境 |
| `PMAIL_FETCH_ALLOW_PRIVATE` | 允许外部邮箱连接私网，仅用于受控环境；基础 Compose 不透传 |

### DNS 向导

管理员从域名管理选择记录检测或下载 BIND 记录片段。邮件主机沿用每个域名的 `smtp.<domain>`，无需新增主机名变量；`PMAIL_WEB_DOMAIN` 只指定网页入口，反代模式下网页 A 记录应指向反代。合并导出片段时保留现有 SOA/NS，若已有 SPF 请编辑原记录而非新增。`imap`/`pop` 建议别名用于沿用现有协议和证书主机名；使用 PMail 自动 HTTP-01 邮件证书时，按上文把这些主机也解析到正确的服务。PTR 在公网 IP 服务商处修改。

## 常见问题

- **25 端口不通：** 从外部网络确认入站端口，对出站可用 `nc -vz -w 5 '<收件方的 MX 主机>' 25` 排查。联系云服务商确认是否封锁；本教程不配置外发中继。
- **证书签不出来：** 检查公网 DNS、是否有过时的 AAAA、是否把邮件域名放到了仅支持网页的代理后。A 路线使 PMail 可从公网 `80` 响应 HTTP-01；B 路线检查邮件验证主机及 PMail 日志，不要让 NPM 接管邮件域名的验证路径。
- **网页无法访问：** 查看 `docker compose ps` 与日志。A 路线核对绑定、证书和 `443`；B 路线核对代理是否能访问 `PMAIL_WEB_BIND:PMAIL_HOST_HTTP_PORT`，不要连 `8443`。
- **发信进垃圾箱：** 检查 SPF、DKIM、DMARC 对齐、PTR/rDNS 和公网 IP 声誉，参考收件方邮件头定位失败项。
- **PostgreSQL 无法读取密码文件：** 若手动替换 `secrets` 卷中的 `db_password`，文件属主和属组应为 `70:70`、权限为 `0600`，卷根目录应允许 PostgreSQL 用户遍历。使用自定义 PostgreSQL 镜像时，可在 `init-secrets` 服务环境中设置 `PMAIL_DB_SECRET_UID`、`PMAIL_DB_SECRET_GID`，使文件归属与镜像中的用户和组一致。替换文件不会修改数据库中的密码，文件内容必须与数据库实际密码一致。
- **忘记管理员密码：** 若从未修改且部署时显式配置了密码，可查保管的部署凭据；否则由另一位可以登录的管理员在「用户管理」重置。修改 `.env` 的引导密码再重启不能重置旧账号。当前没有免登录重置入口，不要通过清空数据库卷来重置。
- **外部邮箱收取异常：** 看账户的 `last_error`，检查服务商授权码、TLS 模式及私网访问限制。

## 从源码构建与开发

仅开发和自定义镜像需要完整源码。在**源码根目录**准备一份按安装说明填写的 `.env`，不要提交该文件。基础 Compose 文件没有构建段；加载构建覆盖文件，并给本地镜像使用独立标签：

```bash
# 确保 .env 尚未设置 PMAIL_IMAGE；已有该项时直接修改其值。
printf '\nPMAIL_IMAGE=pmail-dev:local\n' >> .env
docker compose -f docker-compose.yml -f docker-compose.build.yml build pmail
```

CI 的[工作流](.github/workflows/ci.yml)分别检查前后端、执行 Go 的常规与集成测试。在 `server/` 目录使用 `go test -tags integration ./...` 运行集成测试；需要能绑定固定邮件端口的权限和独立、空白的 PostgreSQL 测试库（`PMAIL_TEST_PG_DSN`），该库账号还须能建 schema。可选的 MySQL 主程序集成测试用 `PMAIL_TEST_MYSQL_DSN` 与 `go test -tags integration . -args mysql`，不要使用生产库。

[镜像发布工作流](.github/workflows/docker.yml)构建 `linux/amd64` 和 `linux/arm64` 并推送到 Docker Hub：需配置变量 `DOCKERHUB_USERNAME`、读写权限密钥 `DOCKERHUB_TOKEN`，以及可选变量 `DOCKERHUB_IMAGE`（默认 `<DOCKERHUB_USERNAME>/pmail-pro`）。稳定 `v*` 标签更新 `latest`，预发布标签不更新；也可以手动触发并指定额外标签。

### Web 前端与 API

`web/` 是独立的 React / TypeScript / Vite SPA，`server/` 是 Go API 与邮件服务。生产环境仍是一个镜像：前端产物放在 `PMAIL_WEB_ROOT`（默认 `/work/web`），Go 服务负责 SPA 回退和 API；Go 二进制不内嵌网页。若独立运行 Makefile 打包的二进制，请从解压目录设置 `PMAIL_WEB_ROOT=./web`。网页文件暂不可用时，API 仍可用。

[OpenAPI 契约](docs/openapi.yaml)描述接口，`GET /api/bootstrap` 无需登录。`PMAIL_DEFAULT_LANG` 设置初始语言；`PMAIL_CORS_ORIGINS` 只在真正跨源时填写允许的源。DNS 向导使用 `PMAIL_PUBLIC_IPV4` / `PMAIL_PUBLIC_IPV6` 和 `PMAIL_DNS_RESOLVER`（见上方变量表）。

真实后端联调（将端口替换为本地可访问的 PMail HTTP 地址）：

```bash
cd web
npm ci --ignore-scripts
PMAIL_API_TARGET=http://localhost:8080 npm run dev
```

Vite 在 `http://localhost:5173` 提供页面，将 `/api`（包括附件）代理给后端；该方式无需另配 CORS。仅做前端开发可运行 `npm run dev:mock`（等价于 `VITE_MOCK=1 npm run dev`），由 MSW 提供模拟接口，不必启动 Go 服务。默认生产构建不包含 MSW 或其 service worker。构建验证命令为 `npm run typecheck`、`npm run lint`、`npm run i18n:check` 和 `npm run build`。

### 从 v3 升级到 v4

- 升级前按上文备份数据库和配置卷。已有账号、邮件及域名继续使用，不需再次注册。全新实例不再生成 `initial_admin_password`，使用 [注册密钥](#3-获取注册密钥并创建管理员)；已有旧文件不会被删除，请继续妥善保护。
- 插件设置、可执行插件、微信通知和外部垃圾邮件分类插件已移除。现有插件配置不再读取，不做破坏性数据库迁移；内置 catch-all SPF/DKIM 过滤仍保留。
- 新客户端使用 `/api/attachments/{id}`、`/api/attachments/download/{id}` 和文本预览接口；旧 `/attachments/*` 仍兼容。详见 [更新日志](CHANGELOG.md#400-beta1)。
