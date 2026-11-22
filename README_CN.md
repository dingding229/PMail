# PMail Pro

PMail Pro 是 [PMail](https://github.com/Jinnrry/PMail) 的增强分支。PMail 是一个自托管邮箱平台，默认使用 PostgreSQL，支持多域名、主地址与别名、队列发信、自动/规则/手动转发、外部 IMAP/POP3 收取、API Token，以及响应式 Vue 3 Web 界面。

![桌面界面占位图](docs/images/desktop.png)
![手机界面占位图](docs/images/mobile.png)

## 三条命令快速开始

```bash
./deploy/deploy.sh init --yes --domain mail.example.com --web-domain mail.example.com
./deploy/deploy.sh check && ./deploy/deploy.sh up
```

首次部署建议不使用 `--yes`，交互向导会询问主域名、附加域名、Web 域名、证书模式、Web 是否公网开放和管理员账号。脚本会生成 PostgreSQL 密码、`PMAIL_SECRET_KEY` 和管理员密码；管理员密码只在初始化结束时打印一次。

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
| `PMAIL_IMAGE` | 留空时按项目使用 `pmail-<project>:local` 并本地构建；填写 `yourname/pmail-pro:3.0.0` 等仓库镜像时直接拉取。 |
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

脚本还使用 `PMAIL_ENV_FILE`、`PMAIL_API_URL`、`PMAIL_API_TOKEN`；CI 使用 `PMAIL_TEST_PG_DSN`、`PMAIL_TEST_MYSQL_DSN`。

## DNS 与证书

每个邮件域名都需要配置 MX、SPF、DKIM、DMARC；公网发信 IP 建议配置 PTR/rDNS。启动后设置 `PMAIL_API_TOKEN`，运行 `./deploy/deploy.sh dns` 查看 DNS 期望值与实际查询结果。脚本必须依赖 `curl`；`jq` 或 `python3` 可选，用于格式化记录；两者都没有时使用 grep/sed 兜底并输出每个域名的原始 DNS JSON。

证书模式选择：模式 0 由 PMail 处理 ACME HTTP-01，模式 2 由 DNS provider 完成 DNS-01，模式 1 使用已有证书，模式 3 仅适用于反代拥有 Web TLS 的场景。邮件协议端口仍必须有对应证书，不要在没有防火墙策略时公开带认证的明文 POP3/SMTP。

## Caddy 反代

```bash
./deploy/deploy.sh up
# 验证覆盖文件：
docker compose --env-file .env -f docker-compose.yml -f docker-compose.caddy.yml config
```

Caddy 覆盖文件负责 Web HTTPS，并反代到 PMail 内部 HTTP。邮件协议证书仍由 PMail 自己的 ACME 模式负责。建议 Web 域名与邮件域名分开，避免两个证书客户端争用同一个 ACME challenge。

## Nginx 反代示例

```nginx
server {
    listen 443 ssl;
    server_name mail.example.com;
    location / {
        proxy_pass http://127.0.0.1:80;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## 多实例与本地镜像

同一主机部署多个实例时必须使用不同项目名，例如 `./deploy/deploy.sh init --project pmail-prod` 和 `./deploy/deploy.sh init --project pmail-staging`。本地镜像按项目命名为 `pmail-<project>:local`，每次 `up` 都会按缓存重新构建，不会跨项目复用。设置 `PMAIL_IMAGE=yourname/pmail-pro:3.0.0` 可切换到预构建镜像模式；该模式会拉取镜像而不本地构建。 所有涉及容器的命令（`up`、`upgrade`、`backup`、`restore`、`dns`、`migrate-*`、`down`、`uninstall`）执行前都会校验 Compose 项目是否归属于当前目录，避免误操作其他目录的容器。

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

部署时在 `.env` 中设置 `PMAIL_IMAGE=yourname/pmail-pro:3.0.0`，`deploy.sh up` 会直接拉取；`UPGRADE_VERSION=3.1.0 ./deploy/deploy.sh upgrade` 切换到新版本（不填默认 `latest`），失败自动回滚。

## 备份、恢复与升级

```bash
./deploy/deploy.sh backup
./deploy/deploy.sh restore backups/<project>-<timestamp>.dump
./deploy/deploy.sh upgrade
```

`upgrade` 会先备份 PostgreSQL，再构建带项目名前缀和时间戳的 `pmail-<project>:<tag>` 镜像，等待健康检查；启动失败时自动尝试恢复旧镜像和备份。新版本确认稳定前请保留备份文件。

## 旧版迁移

停止旧版单文件部署后执行：

```bash
./deploy/deploy.sh migrate-from-sqlite /path/to/pmail.db
# 或
./deploy/deploy.sh migrate-from-mysql 'user:password@tcp(host:3306)/pmail?parseTime=True'
```

迁移完成后检查域名、别名、DKIM、配额和队列，再开放外部邮件流量。

## 常见问题

- **25 端口被封：** 联系云厂商解封 SMTP 出站，或使用中继；同时配置 PTR/rDNS。
- **邮件进垃圾箱：** 检查 PTR、SPF、DKIM 对齐、DMARC 策略和反向 DNS。
- **反代后网页打不开：** 保持 `PMAIL_WEB_BIND=127.0.0.1`，检查反代 upstream 和 `/api/ping`。
- **外部邮箱卡住：** 查看 `last_error`，确认授权码、TLS 模式和 SSRF 配置。
- **回滚：** 保留升级备份，恢复数据库，并将 `PMAIL_IMAGE` 改回上一个 tag。

更多内容见 [README.md](README.md)、[docs/API.md](docs/API.md)、[docs/UPGRADE.md](docs/UPGRADE.md)。
