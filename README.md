# PMail Pro

![版本 0.0.3](https://img.shields.io/badge/PMail%20Pro-0.0.3-2563eb)

PMail Pro 是一套可以自己部署的邮件服务，基于 [Jinnrry/PMail](https://github.com/Jinnrry/PMail)。一个 Docker Compose 文件就能跑起来，不需要填写任何配置。

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

```bash
mkdir pmail && cd pmail
curl -fsSLO https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.yml
docker compose up -d
```

不用改文件，也不用写 `.env`。

### 3. 打开网页完成设置

1. **配置反向代理。** 在 Nginx Proxy Manager（NPM）中新建代理主机 `mail.example.com`，转发到 `http://172.17.0.1:8080`，并申请 SSL 证书。不用 NPM 的话，见 [不用反向代理](#不用反向代理)。
2. **读取注册密钥：**

   ```bash
   docker compose exec pmail cat /work/config/registration_key
   ```

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

也支持 IMAP 143、SMTP 587 的 STARTTLS 方式。

## 升级

在 **系统管理 → 系统更新** 可以查看新版本和更新日志。手动升级：

```bash
docker compose pull
docker compose up -d
```

> ⚠️ 不要执行 `docker compose down -v`。它会删除所有邮件、账号和 DKIM 密钥。

备份、回滚和网页一键更新，见 [进阶说明](docs/ADVANCED.md)。

## 不用反向代理

想让 PMail 直接占用服务器的 80 和 443 端口时：

```bash
curl -fsSL https://raw.githubusercontent.com/dingding229/PMail/master/docker-compose.public.yml -o docker-compose.override.yml
docker compose up -d
```

## 常见问题

**发不出信，队列里显示 “outbound port 25 timed out”**
服务商封锁了出站 25 端口，找服务商解封。

**收不到信**
在 DNS 向导里检查 MX 记录，并确认 `smtp` 没有开 CDN 代理。在服务器外面执行 `nc -vz smtp.example.com 25`，确认端口能连上。

**邮件客户端提示证书错误**
`smtp`、`imap`、`pop` 的 DNS 要直接指向服务器。执行 `docker compose logs pmail | grep -i certificate`，日志会说明哪个名字没有指向本机。

**点退出登录没反应，或者操作时提示“请求来源被拒绝”**
反向代理不在本机 Docker 上时，需要在 `.env` 里写上 `PMAIL_TRUSTED_PROXIES=<反代的 IP>`，再执行 `docker compose up -d`。

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
