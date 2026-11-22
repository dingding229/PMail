# PMail Pro

PMail Pro 是 [PMail](https://github.com/Jinnrry/PMail) 的增强分支。PMail 是一个自托管邮箱平台，默认使用 PostgreSQL，支持多域名、主地址与别名、队列发信、自动/规则/手动转发、外部 IMAP/POP3 收取、API Token，以及响应式 Vue 3 Web 界面。

![桌面界面占位图](docs/images/desktop.png)
![手机界面占位图](docs/images/mobile.png)

## 安装教程

本教程在一台新的 Linux 服务器上用 Docker Hub 镜像 `dingding229/pmail-pro:3.0.0-beta.2` 部署 PMail Pro，不在服务器上编译。`example.com`、`mail.example.com`、其 `smtp`/`imap`/`pop` 子域名和 `203.0.113.10` 都是示例；操作前请替换为自己的域名、公网 IP。`<...>` 表示用实际值替换整个占位符（包括尖括号）。后续命令请在同一个 Bash 终端和部署目录中操作。

### 1. 准备工作

- 64 位 Linux（`amd64` / `arm64`），有可用的公网 IP。小型部署建议至少 **2 GB 内存、20 GB 可用磁盘**，按邮件量和备份增长预留空间；这是建议，不是脚本的硬件门槛。
- 使用能访问 Docker daemon 的账号执行部署命令。如果 `docker version` 提示没有访问权限，可先用 `sudo -i` 进入管理员终端，再从第 2 步开始。安装 **Docker Engine 和 Compose v2**；使用 Caddy 路线需 Compose **2.24.4 或更高**（覆盖文件使用 `!override`）。生产环境优先按 [Docker 官方安装指南](https://docs.docker.com/engine/install/) 为发行版配置软件源并安装 Compose 插件（[合并规则与最低版本](https://docs.docker.com/reference/compose-file/merge/)）。以下是 Ubuntu/Debian 上为全新主机安装所需工具的示例；`jq`/`python3` 可选，只影响 `dns` 的输出格式。

```bash
sudo apt-get update && sudo apt-get install -y ca-certificates curl git dnsutils netcat-openbsd
```

若接受官方便捷脚本的限制，也可从 [Docker 官方便捷安装说明](https://docs.docker.com/engine/install/ubuntu/#install-using-the-convenience-script) 下载脚本，检查内容后执行（Docker 不建议将此方式用于生产环境或升级已有 Docker）。

```bash
curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
```

检查下载的脚本后，执行安装：

```bash
sudo sh /tmp/get-docker.sh
```

```bash
docker version && docker compose version
```

在云安全组和主机防火墙中按下表放行所需的 **TCP** 端口。基础 Compose 默认映射全部列出的邮件端口；不要把映射当成防火墙，未使用的端口应在公网入口禁用。不要对公网发布 PostgreSQL 的 `5432`。

| 端口 | 方向 | 用途 |
| --- | --- | --- |
| `25` | 入站、出站 | **入站必需**，其他邮箱向你投递；直接向外部邮箱投递需要出站 `25` |
| `465` / `587` | 入站，按需 | SMTP 客户端提交：隐式 TLS / STARTTLS |
| `993` / `143` | 入站，按需 | IMAP：隐式 TLS / STARTTLS |
| `995` / `110` | 入站，按需 | POP3：优先使用 `995`（隐式 TLS），不建议公网开放 `110` |
| `80` / `443` | 入站 | HTTP、ACME 验证 / HTTPS；Caddy 路线由 Caddy 占用 |

很多云服务商默认封锁**出站** TCP `25`。本机放行并不能解除云端限制，需向服务商申请解封（通常要提交工单）。还需要一个能修改 DNS 的域名，以及可由 IP 服务商配置 PTR 的公网 IP。**启动前先按[第 6 步](#6-配置-dns)添加 A 记录并确认解析**；证书申请要用到 Web 主机名和 `smtp`、`imap`、`pop` 主机名，即使暂时不用 POP3。

### 2. 获取部署文件

```bash
git clone --branch feature/v3 --depth 1 https://github.com/dingding229/PMail.git pmail-pro && cd pmail-pro
```

浅克隆是为了取得 `deploy/`、`docker-compose.yml`、`docker-compose.caddy.yml` 和 `.env.example`。不需要安装 Go、Node.js 或编译仓库里的源码。不要先把 `.env.example` 复制成 `.env`：`init` 要在新建 `.env` 时生成密码与密钥。

### 3. 初始化

在没有 `.env` 的部署目录中运行：

```bash
export PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.2
./deploy/deploy.sh init
```

向导按下列顺序提问。首次安装用模式 `0`：A 路线由 PMail 提供公网 Web HTTPS；B 路线由 Caddy 终止 Web HTTPS，PMail 仍为邮件协议申请自己的证书。选择内置 Caddy 后，脚本会强制使用证书模式 `0`。

| 实际提示 | 如何填写 |
| --- | --- |
| `主邮件域名 [example.com]:` | 输入自己的邮件域名。示例是 `example.com`，用户地址形如 `admin@example.com`。 |
| `附加域名（逗号分隔，可留空） []:` | 本教程直接回车，只部署一个邮件域名。 |
| `Web 域名 [example.com]:` | 输入自己的网页域名，如 `mail.example.com`；直接回车会用主邮件域名。 |
| `证书模式 0=ACME HTTP-01, 1=自有证书, 2=ACME DNS-01, 3=反代仅 HTTP [3]:` | 本教程输入 `0`。`1` 需要预先配置有效证书；`2` 需要额外配置 DNS-01；`3` 不启用 PMail TLS，不是可同时提供加密邮件客户端服务的选项。 |
| `是否使用内置 Caddy 反代？[y/N]` | B 路线输入 `y`，不再询问是否开放 Web；A 路线直接回车。 |
| `是否对公网开放 Web？[y/N]:` | 仅未选 Caddy、也未传 `--web-public` 时询问。A 路线输入 `y`；不开放时默认绑定 `127.0.0.1`。 |
| `管理员账号 [admin]:` | 直接回车使用 `admin`，或输入自选账号。 |

也可以用以下**非交互命令代替**向导，只选一条。A 路线（PMail 直接提供 HTTPS）：

```bash
PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.2 ./deploy/deploy.sh init --yes --domain example.com --web-domain mail.example.com --ssl-type 0 --web-public --admin-account admin
```

B 路线（推荐，内置 Caddy）：

```bash
PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.2 ./deploy/deploy.sh init --yes --caddy --domain example.com --web-domain mail.example.com --admin-account admin
```

`init` 将镜像、域名、证书模式、Web 绑定和管理员信息写入权限为 `600` 的 `.env`。新建时应看到 `初始化完成。最终项目名：pmail`、`管理员账号：admin` 和 `管理员密码（仅此处显示）：…`，随后有 MX、SPF、DKIM、DMARC、PTR 提示。**立即保存这次打印的管理员密码**，并安全备份 `.env`。尤其不要重新生成已有部署的 `PMAIL_SECRET_KEY`（凭据加密/SRS）和 `POSTGRES_PASSWORD`（已有 PostgreSQL 卷不会随 `.env` 改密码）。初始化密码仍保存在 `.env`，但改密后它不代表当前密码。若 `.env` 已存在且已设置镜像，重新 `init` 不会把镜像改成当前 shell 的值，应检查 `.env` 中的 `PMAIL_IMAGE`。

### 4. 启动与检查

先确认第 6 步中的 A 记录已生效，且第 3 步已选定直接访问或内置 Caddy；然后运行：

```bash
./deploy/deploy.sh check
```

成功时脚本打印 `部署前检查通过。`。端口占用和 `25` 探测失败可能只打印提示；启动前仍要处理实际冲突。`check` 检查 Docker、Compose 配置、端口、磁盘和必要的 `.env` 字段，不验证公网 DNS、证书或邮件投递。

```bash
./deploy/deploy.sh up
```

镜像名含 `/`，所以脚本拉取 PMail 镜像，而不是本地编译。成功时会打印两次校验，首次检查与十秒后检查的 `restart_count` 应一致；下面以首次启动时的 `0` 举例：

```text
PMail 运行校验：status=running health=healthy restart_count=0
PMail 稳定性校验：status=running health=healthy restart_count=0
```

健康检查不代表 HTTPS 证书已经签发，也不代表邮件协议已打开。用以下命令查看容器与日志：

```bash
./deploy/deploy.sh status
```

```bash
./deploy/deploy.sh logs
```

`logs` 会跟踪最后 200 行及后续日志；按 `Ctrl+C` 退出日志查看，不会停止服务。

### 5. 访问网页：选择 A 或 B

#### A. 直接公网访问，由 PMail 提供 HTTPS

若初始化时选择模式 `0` 并开放 Web，脚本已同时启用 PMail Web HTTPS，无需修改 `.env`。公网 `80` 必须到达 PMail，以完成 HTTP-01；`443` 用于浏览器访问。首次签发时 PMail 可能先只有 Web HTTP 和 SMTP `25`；等待签发完成并重载后，再从浏览器打开 `https://mail.example.com/`。证书未就绪时不要用公网 HTTP 登录。

#### B. 推荐：Caddy 反向代理

在第 3 步使用 `--caddy` 或在向导选择内置 Caddy，再按第 4 步启动。脚本会把内置覆盖文件写入 `.env` 的 `COMPOSE_FILE`；不需要自建 Compose 覆盖文件或 Caddyfile。`--caddy` 只应用于新建 `.env`，不会切换现有部署。

公网 `80`/`443` 由 Caddy 使用；PMail 保留邮件端口，仅在容器网络提供 Web HTTP。Caddy 通过 `443` 为 Web 签发证书，并把 `80` 上的 HTTP-01 请求转给 PMail，供邮件证书签发和续期。**如配置了附加邮件域名（`PMAIL_DOMAINS`）**，要在 `deploy/Caddyfile` 的 `http://` 站点列表手动追加它们的 `smtp`、`imap`、`pop` 主机名并配置 DNS；如果 Caddy 已在运行，修改后需重载其配置。公网防火墙仅放行使用的邮件端口，尤其不要无故开放 `110`。

等待 Caddy 签发 Web 证书，从浏览器打开 `https://mail.example.com/`。如需查看 Caddy 日志（自选项目名时替换 `-p` 的值）：

```bash
docker compose --env-file .env -p pmail logs --tail=100 caddy
```

已经有 Nginx 时，可改用下文的 [Nginx 反代示例](#nginx-反代示例)，而不是运行两套反代。

**首次登录（两条路线相同）：**使用 `init` 打印的管理员账号与密码。进入「管理 → 域名管理」，确认 `example.com` 是主邮件域名；如需核对登录地址，在「设置 → 地址与别名」查看实际主地址。Web 主机 `mail.example.com` 不会因为用于网页而自动成为另一个收信域名。

### 6. 配置 DNS

在 DNS 服务商控制台添加以下记录；第 4 步之前完成 A 记录，启动后再补上实际 DKIM 值。表中的主机名是完整名称；有些控制台要求填写 `@` 或相对名称（例如为 `smtp.example.com` 只填 `smtp`）。对邮件主机使用 DNS 解析，不要把它们放到只代理 HTTP 的 CDN 后面。不要留下指向其他主机的 AAAA 记录。

| 类型 | 主机/地址 | 值 | 用途 |
| --- | --- | --- | --- |
| A | `example.com` | `203.0.113.10` | 可选：仅在主域名也应指向本机时添加，不要覆盖已有网站地址 |
| A | `mail.example.com` | `203.0.113.10` | 网页、证书验证 |
| A | `smtp.example.com` | `203.0.113.10` | MX 目标、SMTP 客户端、证书验证 |
| A | `imap.example.com` | `203.0.113.10` | IMAP 客户端、证书验证 |
| A | `pop.example.com` | `203.0.113.10` | POP3 主机、证书验证 |
| MX | `example.com` | 优先级 `10`，目标 `smtp.example.com.` | 收信路由，不填 IP |
| TXT（SPF） | `example.com` | `v=spf1 a mx ~all` | PMail 的默认期望；已有 SPF 时应合并授权来源，不要加第二条 SPF |
| TXT（DKIM） | `default._domainkey.example.com` | `<域名管理中显示的完整 DKIM TXT 值>` | 使用实际选择器和公钥，不要截断 |
| TXT（DMARC） | `_dmarc.example.com` | `v=DMARC1; p=none` | PMail 的初始期望，先观察对齐情况 |
| PTR | `203.0.113.10` | `smtp.example.com.` | 由 IP 服务商配置，目标的 A 记录应解析回同一 IP |

获取 DKIM：在管理员的「域名管理」中打开该域名的 DNS 记录，复制 DKIM 的**期望值**；也可以用 `deploy.sh dns`。后者要求 `curl`，`jq` 或 `python3` 可选（安装后显示格式化记录，否则输出每个域名的原始 DNS JSON）。

用管理员登录，在「设置 → API Token」创建一个 Token（例如命名为 `dns-check`），复制创建时显示的明文。普通用户的 Token 不能读取管理员域名接口。下列命令交互读取，不回显 Token；A、B 两条路线都显式使用 HTTPS，避免脚本的默认本地 HTTP URL 不适合反代或 HTTP 跳转的情况：

```bash
read -rsp 'PMail API Token: ' token; printf '\n'
PMAIL_API_URL=https://mail.example.com PMAIL_API_TOKEN="$token" ./deploy/deploy.sh dns
unset token
```

脚本会查询域名的 MX、SPF、DKIM、DMARC 期望值与匹配结果；A 和 PTR 需单独核对。DNS 变更可能要等缓存过期，从外网查验：

```bash
for host in mail.example.com smtp.example.com imap.example.com pop.example.com; do dig +short A "$host"; done
dig +short MX example.com
dig +short TXT example.com
dig +short TXT default._domainkey.example.com
dig +short TXT _dmarc.example.com
dig +short -x 203.0.113.10
```

### 7. 收发验证与客户端配置

用自己的**外部邮箱**给 `admin@example.com`（或「地址与别名」显示的地址）发一封邮件，在 PMail 网页收件箱确认收到，再从 PMail 网页回复。外发先进入队列；「已排队」不等于投递完成，可在「设置 → 发件队列」检查结果，并在外部邮箱检查是否收到。如还未启用客户端 TLS，也可先完成这步网页收发验证。

证书就绪后，在邮件客户端选中以下任一收件/发件组合。用户名填完整邮箱地址（如 `admin@example.com`）或实际 PMail 账号，密码是该账号的登录密码，不是 API Token。若改过宿主机端口映射，按实际映射填写。不要在证书报错时关闭验证。

| 协议 | 服务器 | 默认端口 | 加密 |
| --- | --- | --- | --- |
| IMAP 收件（推荐） | `imap.example.com` | `993` | SSL/TLS（隐式） |
| IMAP 收件（可选） | `imap.example.com` | `143` | STARTTLS |
| SMTP 发件（推荐） | `smtp.example.com` | `465` | SSL/TLS（隐式） |
| SMTP 发件（可选） | `smtp.example.com` | `587` | STARTTLS |
| POP3 收件（可选） | `pop.example.com` | `995` | SSL/TLS（隐式） |

客户端发件要启用 SMTP 认证。`25` 用于邮件服务器之间投递，不是客户端提交端口。

### 8. 日常运维

**升级镜像：**请先确认目标 tag 已发布。下面以 beta.3 为例。也可设置 `UPGRADE_VERSION` 环境变量；同时指定时 `--version` 优先，两者都不指定才用 `latest`。预发布版本应明确传 tag。

```bash
./deploy/deploy.sh upgrade --version 3.0.0-beta.3
```

脚本先备份 PostgreSQL，再拉取目标镜像并等待健康检查。失败时会尝试恢复旧镜像和升级前的数据库，**即使回滚成功，升级仍返回非零**。数据库恢复失败时，脚本会报错并打印备份路径，不会宣称回滚成功。自编译模式则使用带项目名和时间戳的 `pmail-<project>:<tag>` 镜像，不走仓库镜像拉取分支。

**备份：**脚本会打印 `备份完成：<路径>`，文件在部署目录的 `backups/<project>-<UTC 时间戳>.dump`。这是 PostgreSQL 转储，不包含 `.env` 或 `pmail_config` 配置卷（证书等）；Caddy 路线还应另行备份本地覆盖配置与 Caddy 卷。把备份安全复制到另一处。

```bash
./deploy/deploy.sh backup
```

**恢复：**会暂停 PMail 并用指定转储恢复数据；数据库恢复失败会报错，不会自动重启服务。先备份当前数据库；如需手工回滚版本，先在 `.env` 确认 `PMAIL_IMAGE` 指向与备份相配的旧版本并确保该镜像可用。按提示粘贴 `backup` 打印的完整路径（不带引号或 `备份完成：` 前缀）。

```bash
read -r -p '备份文件的完整路径：' dump && ./deploy/deploy.sh restore "$dump"
```

用[第 4 步](#4-启动与检查)的 `status`/`logs` 查看状态并登录核对恢复结果。不要用 v3 转储恢复到上游 v2；详见 [升级说明](docs/UPGRADE.md)。

**停止：**删除本项目容器和 Compose 网络，但保留命名数据卷。再次运行 `up` 可以启动。

```bash
./deploy/deploy.sh down
```

**卸载但保留数据：**确认提示后删除容器并保留卷。

```bash
./deploy/deploy.sh uninstall
```

**彻底卸载：**先做离机备份。此命令先停止并移除容器，然后在删除卷前询问确认；确认后删除 PostgreSQL 和 PMail 配置卷，Caddy 路线还会删除该项目的 Caddy 卷。`.env` 和 `backups/` 不会被这个命令删除。

```bash
./deploy/deploy.sh uninstall --purge
```

### 9. 常见问题

- **25 端口不通：**入站从外网检查云安全组和防火墙；出站用下列命令连接你可访问的收件域 MX，失败时向云服务商确认是否封锁。`check` 的出站探测只连接 `1.1.1.1:25`，单次失败不能断定云厂商封锁。没有出站 `25` 时，需先解封或另行安排外发中继；本教程不配置中继。

```bash
nc -vz -w 5 '<外部邮箱域名的 MX 主机>' 25
```

- **证书申请失败：**检查相应主机名的公网 DNS、过时的 AAAA、`80`/`443` 的实际入口，以及 PMail/Caddy 日志。A 路线的 PMail 必须收到 `/.well-known/acme-challenge/` 请求；B 路线由 Caddy 转发它，且 Caddy 的 Web 证书验证需要公网 `443`。不要只为给浏览器签证书而改为模式 `3`，否则 PMail 不会开启 TLS 邮件端口。
- **网页打不开：**默认 `PMAIL_WEB_BIND=127.0.0.1`。A 路线核对公网绑定、`PMAIL_HTTPS_ENABLED=0` 与证书；B 路线核对 `COMPOSE_FILE`、Caddy 日志和反代上游。主机本地 Nginx 反代则应保留 PMail 的回环绑定。
- **发信进垃圾箱：**核对 SPF、DKIM、DMARC 对齐和 PTR/rDNS，检查发信 IP 声誉和外部邮箱的邮件头。仅 Web 可访问不代表这些记录正确。
- **忘记管理员密码：**若仍是初始密码，查安全保存的 `init` 输出或 `.env` 的 `PMAIL_ADMIN_PASSWORD`。如果已修改过，可用另一名可登录的管理员在「用户管理」重置。所有管理员都无法登录时，当前没有免登录的重置页面或 `deploy.sh` 重置命令；修改 `.env` 的初始密码再重启不会重置已存在账号。请按已知凭据的备份制定恢复方案，不要通过清空数据卷“重置”。
- **外部邮箱收取卡住：**查看 `last_error`，检查服务商授权码、TLS 模式与 SSRF 限制。

## 功能

- PostgreSQL 17、持久化数据库卷和配置卷。
- 多域名、主地址、别名、发件权限、用户配额和 DNS 记录检查。
- SMTP/IMAP/POP3、外发队列、重试、退信和本地投递。
- 全局自动转发、多目标规则、手动转发、SRS 和防环。
- 外部 IMAP/POP3 收取、UID/UIDL 去重、加密凭据和失败暂停。
- API Token、bcrypt 密码升级、PC/平板/手机响应式 UI、中英文切换。

## 配置项

完整中文注释见 `.env.example`。主要变量如下：

| 变量 | 含义 |
|---|---|
| `PMAIL_PROJECT_NAME` | Compose 项目名；同一主机的每个部署必须使用不同名称。 |
| `PMAIL_IMAGE` | 留空时按项目使用 `pmail-<project>:local` 并本地构建；填写 `dingding229/pmail-pro:3.0.0-beta.2` 等仓库镜像时直接拉取。 |
| `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD` | PostgreSQL 初始化参数。 |
| `PMAIL_DB_TYPE`、`PMAIL_DB_DSN` | 数据库类型和 DSN。 |
| `PMAIL_DOMAIN`、`PMAIL_DOMAINS`、`PMAIL_WEB_DOMAIN` | 邮件域名和 Web 域名。 |
| `PMAIL_HTTP_PORT`、`PMAIL_HTTPS_PORT` | 容器内 Web 监听端口。 |
| `PMAIL_HOST_*_PORT` | SMTP、POP3、IMAP、Web 宿主机端口映射。 |
| `PMAIL_MAIL_BIND`、`PMAIL_WEB_BIND` | 邮件/Web 绑定地址；邮件默认 `0.0.0.0`，Web 默认 `127.0.0.1`。 |
| `PMAIL_SSL_TYPE` | `0` ACME HTTP-01，`1` 自有证书，`2` ACME DNS-01，`3` 仅反代 HTTP。 |
| `PMAIL_HTTPS_ENABLED` | HTTPS 监听模式；SSL 类型 3 时后端只提供 HTTP。 |
| `PMAIL_LOG_LEVEL` | `debug`、`info`、`warn`、`error`。 |
| `PMAIL_SPAM_FILTER_LEVEL` | SPF/DKIM 过滤等级 0–3。 |
| `PMAIL_CATCH_ALL` | 是否将未知本地收件人交给管理员。 |
| `PMAIL_ALLOW_USER_ALIAS` | 是否允许普通用户自助创建别名。 |
| `PMAIL_QUEUE_WORKERS` | 外发队列并发数。 |
| `PMAIL_FETCH_WORKERS` | 外部邮箱收取并发数。 |
| `PMAIL_FETCH_ALLOW_PRIVATE` | 仅受控测试环境允许连接私网目标。 |
| `PMAIL_SECRET_KEY` | 加密和 SRS 密钥，升级时必须保持不变。 |
| `PMAIL_ADMIN_ACCOUNT`、`PMAIL_ADMIN_PASSWORD` | 首次启动管理员引导。 |

脚本还使用 `PMAIL_ENV_FILE`、`PMAIL_API_URL`、`PMAIL_API_TOKEN`。`PMAIL_QUEUE_WORKERS`、`PMAIL_FETCH_WORKERS` 和 `PMAIL_FETCH_ALLOW_PRIVATE` 是 PMail 进程环境变量；基础 Compose 不会透传，须用额外 Compose 覆盖文件把它们传入 `pmail.environment`。PostgreSQL 和可选 MySQL 测试连接分别由 `PMAIL_TEST_PG_DSN`、`PMAIL_TEST_MYSQL_DSN` 指定。

## Nginx 反代示例

适用于已经能管理 Web 证书的同机 Nginx，不要同时加载 Caddy 覆盖文件。从第 3 步用模式 `0`、不开放 Web 初始化，保留 `PMAIL_WEB_BIND=127.0.0.1` 和 `PMAIL_HTTPS_ENABLED=2`，并把 PMail 的宿主机 Web 端口改为回环地址上的 `8080`/`8443`。然后按第 4 步启动。

```bash
sed -i 's/^PMAIL_HOST_HTTP_PORT=.*/PMAIL_HOST_HTTP_PORT=8080/; s/^PMAIL_HOST_HTTPS_PORT=.*/PMAIL_HOST_HTTPS_PORT=8443/' .env
```

在 Nginx 中使用以下配置，替换证书路径占位符。公网 HTTP-01 请求转给 PMail，Web HTTPS 由 Nginx 负责；不用在公网开放 `8080` 或 `8443`。

```nginx
server {
    listen 80;
    server_name mail.example.com smtp.example.com imap.example.com pop.example.com;
    location ^~ /.well-known/acme-challenge/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
    }
    location / {
        return 308 https://mail.example.com$request_uri;
    }
}
server {
    listen 443 ssl;
    server_name mail.example.com;
    ssl_certificate <path-to-web-fullchain>;
    ssl_certificate_key <path-to-web-private-key>;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 多实例与本地镜像

同一主机部署多个实例时必须使用不同项目名，例如 `./deploy/deploy.sh init --project pmail-prod` 和 `./deploy/deploy.sh init --project pmail-staging`。本地镜像按项目命名为 `pmail-<project>:local`，每次 `up` 都会按缓存重新构建，不会跨项目复用。设置 `PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.2` 可切换到预构建镜像模式；该模式会拉取镜像而不本地构建。 所有涉及容器的命令（`up`、`upgrade`、`backup`、`restore`、`dns`、`migrate-*`、`down`、`uninstall`）执行前都会校验 Compose 项目是否归属于当前目录，避免误操作其他目录的容器。

## 预构建镜像（GitHub Actions → Docker Hub）

`.github/workflows/docker.yml` 会构建 `linux/amd64`、`linux/arm64` 镜像并推送到 Docker Hub。首次使用前在 GitHub 仓库 Settings → Secrets and variables → Actions 中配置：

| 类型 | 名称 | 说明 |
| --- | --- | --- |
| Variable | `DOCKERHUB_USERNAME` | Docker Hub 用户名或组织名 |
| Variable | `DOCKERHUB_IMAGE` | 可选，默认 `<DOCKERHUB_USERNAME>/pmail-pro` |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub → Account settings → Personal access tokens 创建，权限 Read & Write |

触发方式：
- 推送 `v*` tag（如 `git tag v3.0.0 && git push origin v3.0.0`）：生成 `3.0.0`、`3.0`、`3`、`latest`、`sha-xxxxxxx`；带 `-` 的预发布 tag（如 `v3.0.0-rc1`）不更新 `latest`。
- Actions 页面手动运行 “PMail Pro Docker image”（可选任意分支）：生成分支名 tag（如 `feature-v3`）、`sha-xxxxxxx`，以及可选的自定义 tag。

部署时在 `.env` 中设置 `PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.2`，`deploy.sh up` 会直接拉取；`./deploy/deploy.sh upgrade --version 3.1.0` 切换到新版本（不填默认 `latest`），启动失败时尝试自动回滚。

## 旧版迁移

停止旧版单文件部署后，将 `<path-to-pmail.db>` 替换为 SQLite 文件路径：

```bash
./deploy/deploy.sh migrate-from-sqlite '<path-to-pmail.db>'
```

或选择 MySQL；将 `<mysql-dsn>` 替换为实际 DSN（格式示例 `user:password@tcp(<mysql-host>:3306)/pmail?parseTime=True`）。

```bash
./deploy/deploy.sh migrate-from-mysql '<mysql-dsn>'
```

迁移完成后检查域名、别名、DKIM、配额和队列，再开放外部邮件流量。

## Go 开发

在 `server/` 中运行 `go test ./...` 执行默认测试；本机或服务器上用 `go test -tags integration ./...` 执行集成测试。完整集成测试需要 root（SMTP 25 等固定邮件端口必须空闲）和通过 `PMAIL_TEST_PG_DSN` 指定的**独立、空白 PostgreSQL 测试库**。IMAP 测试会创建和清理独立 schema，数据库账号须有建 schema 权限；不要使用生产数据库。可选 MySQL 整站测试需设置 `PMAIL_TEST_MYSQL_DSN`，再执行 `go test -tags integration . -args mysql`；CI 使用 PostgreSQL，不依赖 MySQL。

更多内容见 [README.md](README.md)、[docs/API.md](docs/API.md)、[docs/UPGRADE.md](docs/UPGRADE.md)。
