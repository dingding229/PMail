# PMail Pro

![版本 0.0.8](https://img.shields.io/badge/PMail%20Pro-0.0.8-2563eb)

PMail Pro 是一套可以自己部署的邮件服务，基于 [Jinnrry/PMail](https://github.com/Jinnrry/PMail)。一个 Docker Compose 文件就能启动，邮件域名可在首次打开网页时设置。

## 功能

- 收发邮件：SMTP、IMAP、POP3，支持多个域名、别名、转发规则。
- 网页邮箱：电脑和手机都能用，支持暗色模式、富文本写信、草稿、星标。
- 安全：自动识别垃圾邮件、危险附件和可疑代码，自动提取验证码。
- 通知：可以推送到 Telegram。
- 省心：自动申请证书；DNS 向导告诉你每条记录该怎么填；后台可一键更新。

## 安装

### 1. 准备

- 一台 Linux 服务器，装好 Docker（含 Compose 2.24 或更新版本）。
- 一个域名，下文用 `example.com` 举例，网页地址是 `mail.example.com`。
- 服务器的 **25 端口** 能收发。很多云服务商默认封锁出站 25 端口，需要先找服务商解封，否则发不出信。检查方法：

  ```bash
  nc -vz -w 5 gmail-smtp-in.l.google.com 25
  ```

### 2. 下载并启动

在 `docker-compose.yml` 所在目录执行 `docker compose up -d`。

```bash
mkdir pmail && cd pmail
curl -fsSLO https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.yml
docker compose up -d
```

启动容器不需要 `.env`。

### 3. 打开网页完成设置

1. **配置反向代理。** 在 Nginx Proxy Manager（NPM）中新建代理主机 `mail.example.com`，转发到 `http://172.17.0.1:8080`，并申请 SSL 证书。不用 NPM 的话，见 [不用反向代理](#不用反向代理)。
   同机 Docker 网桥反代默认无需填写配置，代理应保留 `Host`、覆盖 `X-Forwarded-Proto` 并正确处理 `X-Forwarded-For`。自定义网页主机名时可设置 `PMAIL_WEB_DOMAIN`；服务不会根据请求 `Host` 自动保存网页主机名。
2. **读取注册密钥：**

   ```bash
   docker compose -p pmail exec pmail cat /work/config/registration_key
   ```

   如果在 `.env` 中修改过 `PMAIL_PROJECT_NAME`，请把 `-p` 后的 `pmail` 换成对应项目名。

3. 打开 `https://mail.example.com`，填写**邮件域名**（`example.com`）、注册密钥、管理员账号和密码。提交后就进入邮箱了。

### 4. 配置 DNS

登录后进入 **系统管理 → 域名与 DNS 向导**，它会列出每条要填的记录，并检查是否已经生效。主要有这几条：

| 类型 | 名称 | 值 |
| --- | --- | --- |
| A | `smtp`、`imap`、`pop`、`mail` | 服务器 IP |
| MX | `@` | `smtp.example.com`，优先级 10 |
| TXT | `@` | `v=spf1 mx a ~all` |
| TXT | `default._domainkey` | 从 DNS 向导复制 |
| TXT | `_dmarc` | `v=DMARC1; p=quarantine` |

注意：

- **`smtp`、`imap`、`pop` 必须直连服务器**，不能开 CDN 代理。用 Cloudflare 的话要设成“仅 DNS”（灰色云朵），否则收不到信，证书也申请不下来。
- 只用 PMail 收信时，删掉其他 MX 记录（比如 Cloudflare Email Routing 的记录）。
- 请服务器服务商把反向解析（PTR）设为 `smtp.example.com`，可以减少发出的信进垃圾箱。

DNS 生效后，PMail 会自动为 `smtp`、`imap`、`pop` 申请证书，不用手动操作。

## 邮件客户端设置

账号填完整邮箱地址（如 `admin@example.com`），密码是网页登录密码。

| 协议 | 服务器 | 端口 | 加密 |
| --- | --- | --- | --- |
| IMAP | `imap.example.com` | 993 | SSL/TLS |
| SMTP | `smtp.example.com` | 465 | SSL/TLS |
| POP3 | `pop.example.com` | 995 | SSL/TLS |

也支持 IMAP 143、SMTP 587 的 STARTTLS 方式。开启两步验证后，请在「邮箱设置 → 个人资料与安全」生成应用专用密码，供 IMAP、SMTP 和 POP3 客户端使用。

多个地址的用户可在邮件侧栏切换主地址、别名和「其他地址」（兜底收件）。同一封邮件可能出现在多个地址视图中，但只保存一份。IMAP 和 POP3 仍显示全部邮件。

## 入站病毒扫描

镜像内置 ClamAV，默认关闭。在「系统管理 → 入站安全」打开开关即可，首次会自动下载约 300 MB 病毒库；运行约需 1–1.5 GB 内存，建议开启前留出至少 2 GB。扫描仅用于入站 SMTP；发现病毒会拒收，扫描器不可用时仍投递并显示未扫描。首次升级至包含本功能的版本时，请拉取新镜像。

## 升级

从旧版升级后，已代收的邮件仍可查看；旧外部账号配置与凭据会在升级时清除。

在 **系统管理 → 系统更新** 查看版本和更新日志。支持程序自更新的官方 Linux 镜像可选 **立即更新**：PMail 验签后暂存并重启，不需要额外容器。首次从旧版本升级、没有签名发布包或需要更新基础镜像时，请手动升级。

升级前检查 `.env`：若 `PMAIL_IMAGE` 固定为旧标签，请删掉该行（默认使用 `latest`）或改为新版本标签，否则 `docker compose pull` 仍会拉取旧镜像。然后在宿主机的 `docker-compose.yml` 所在目录执行：

```bash
docker compose pull
docker compose up -d
```

> ⚠️ 不要执行 `docker compose down -v`。它会删除所有邮件、账号和 DKIM 密钥。

备份、回滚和网页一键更新，见 [进阶说明](docs/ADVANCED.md)。替换旧 Compose 文件前，核对项目名和已有卷，见 [升级说明](docs/UPGRADE.md#从-v002--v003-升级)。

## 不用反向代理

想让 PMail 直接占用服务器的 80 和 443 端口时：

```bash
curl -fsSL https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.public.yml -o docker-compose.override.yml
docker compose up -d
```

公网覆盖文件需要 Compose 2.24.4 或更新版本。直连无需配置可信代理；请不要通过公网明文 HTTP 提交注册密钥或管理员密码。可先在 `.env` 填写 `PMAIL_DOMAIN`，设置 DNS 并等 PMail 的 HTTPS 证书就绪，再访问网页注册。

## 常见问题

**发不出信，队列里显示 “outbound port 25 timed out”**
服务商封锁了出站 25 端口，找服务商解封。

**收不到信**
在 DNS 向导里检查 MX 记录，并确认 `smtp` 没有开 CDN 代理。在服务器外面执行 `nc -vz smtp.example.com 25`，确认端口能连上。

**邮件客户端提示证书错误**
`smtp`、`imap`、`pop` 的 DNS 要直接指向服务器。执行 `docker compose logs pmail | grep -i certificate`，日志会说明哪个名字没有指向本机。

**点退出登录没反应，或者操作时提示“请求来源被拒绝”**
反代需保留 `Host` 并覆盖 `X-Forwarded-Proto`。Cloudflare 的“灵活（Flexible）”和“完全（严格）”均支持，推荐“完全（严格）”；使用“灵活”时，请关闭源站反代的强制 HTTPS 跳转。容器反代模式默认仅信任 `172.16.0.0/12`；其他地址可按日志提示显式设置 `PMAIL_TRUSTED_PROXIES`。默认网页上游只绑定 Docker 网桥，同一 Docker 主机上能访问上游的不可信容器仍可伪造转发头；共享宿主机时请仅信任实际代理地址（`/32` 或 `/128`），且不要公开该上游端口。

**发出的信进了垃圾箱**
用 DNS 向导检查 SPF、DKIM、DMARC 是否都已生效，并设置 PTR。

**忘记管理员密码**
让另一位管理员在 **系统管理 → 用户管理** 里重置。

## 更多

- [进阶说明](docs/ADVANCED.md)：备份恢复、可选配置、Nginx 示例、开发构建
- [从旧版 PMail 迁移](docs/UPGRADE.md)
- [更新日志](CHANGELOG.md)
- [API 文档](docs/API.md)

## 许可证

[GPL-2.0](LICENSE)
