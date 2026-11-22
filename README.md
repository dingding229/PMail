# PMail Pro

PMail Pro is an enhanced fork of [PMail](https://github.com/Jinnrry/PMail). PMail is a self-hosted mail platform with PostgreSQL as its default database, multi-domain mailboxes, aliases, queue-backed outbound delivery, automatic/rule/manual forwarding, external IMAP/POP3 collection, API tokens, and a responsive Vue 3 web UI.

![Desktop UI placeholder](docs/images/desktop.png)
![Mobile UI placeholder](docs/images/mobile.png)

## Installation guide

This guide installs PMail Pro on a fresh Linux server using a prebuilt Docker Hub image. Both options use `dingding229/pmail-pro:3.0.0-beta.3`, which contains the external-proxy certificate fix; do not use the older beta.1/beta.2 images. Replace the example names `example.com`, `mail.example.com`, their `smtp`/`imap`/`pop` hostnames, and `203.0.113.10` with your own domain names and public IP. Replace an entire `<...>` placeholder, including the angle brackets. Run the deployment steps in the same Bash terminal and deployment directory.

### 1. Prerequisites

- 64-bit Linux (`amd64` or `arm64`) and a usable public IP. For a small installation, start with at least **2 GB RAM and 20 GB free disk** and allow room for mail and backups. These are recommendations, not script requirements.
- Run the deployment commands as an account that can access the Docker daemon. If `docker version` reports a permission error, you can open an administrator shell with `sudo -i` and continue from step 2. Install **Docker Engine and Compose v2**. For production, prefer the package repository and Compose plugin in the [official Docker installation guide](https://docs.docker.com/engine/install/). On a fresh Ubuntu/Debian server, install the following host tools. `jq` and `python3` are optional; they format the `dns` output.

```bash
sudo apt-get update && sudo apt-get install -y ca-certificates curl git dnsutils netcat-openbsd iproute2 openssl
```

If you opt for the [official Docker convenience script](https://docs.docker.com/engine/install/ubuntu/#install-using-the-convenience-script), review it before running it. Docker does not recommend this method for production installations or upgrades of an existing Engine.

```bash
curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
```

After reviewing the downloaded script, run the installer:

```bash
sudo sh /tmp/get-docker.sh
```

```bash
docker version && docker compose version
```

Allow the required **TCP** ports in your cloud security group and host firewall. The base Compose file maps all the mail ports below; port mapping is not a firewall. Block unused ports at the public boundary. Do not expose PostgreSQL `5432` publicly.

| Port | Direction | Purpose |
| --- | --- | --- |
| `25` | Inbound and outbound | **Inbound is required** to receive mail from other servers; outbound is required for direct delivery to them |
| `465` / `587` | Inbound, optional | SMTP client submission: implicit TLS / STARTTLS |
| `993` / `143` | Inbound, optional | IMAP: implicit TLS / STARTTLS |
| `995` / `110` | Inbound, optional | POP3: prefer `995` (implicit TLS); avoid publicly exposing `110` |
| `80` / `443` | Inbound | HTTP and ACME validation / HTTPS; PMail uses these ports in option A, NPM in option B |

Many cloud providers block outbound TCP `25` by default. Opening it in your own firewall does not remove the provider's block; ask the provider to lift it, usually through a support ticket. You also need a domain whose DNS you can edit and a public IP for which the IP provider can set PTR. **Publish and check the A records in [step 6](#6-configure-dns) before starting**. Certificate issuance uses the web name and the `smtp`, `imap`, and `pop` names, even if you do not plan to use POP3.

### 2. Get the deployment files

```bash
git clone --branch feature/v3 --depth 1 https://github.com/dingding229/PMail.git pmail-pro && cd pmail-pro
```

The shallow clone provides `deploy/`, `docker-compose.yml`, and `.env.example`. You do not need Go, Node.js, or a source build. Do not copy `.env.example` to `.env` first: `init` generates the passwords and keys when it creates `.env`.

### 3. Initialize

Run the following in a directory that does not yet contain `.env`. For option B, first replace the example image tag with one that includes the external-proxy certificate fix:

```bash
export PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.3
./deploy/deploy.sh init
```

The wizard asks these questions in order. Use certificate mode `0` for this guide. In option A, PMail serves public web HTTPS. In option B, an external Nginx Proxy Manager (NPM) terminates web HTTPS. PMail uses HTTP-01 (mode `0`) for the mail certificate.

| Prompt as printed | What to enter |
| --- | --- |
| `主邮件域名 [example.com]:` | Your mail domain: `example.com` in this guide, with addresses such as `admin@example.com`. |
| `附加域名（逗号分隔，可留空） []:` | Additional comma-separated domains. Press Enter; this guide configures one mail domain. |
| `Web 域名 [example.com]:` | The website hostname, such as `mail.example.com`. If left blank, it defaults to the mail domain. |
| `证书模式 0=ACME HTTP-01, 1=自有证书, 2=ACME DNS-01, 3=反代仅 HTTP [3]:` | Enter `0`. Mode `1` requires an existing certificate, `2` requires additional DNS-01 setup, and `3` disables TLS in PMail, including the TLS mail listeners. |
| `Web 是否由外部反向代理（如 Nginx Proxy Manager）提供？[y/N]` | Enter `y` for option B; the wizard skips the public Web question. Press Enter for option A. |
| `反代上游的宿主机 IPv4 地址 [172.17.0.1]:` | Shown only in option B. Docker-based NPM can usually use the bridge address; for NPM running on the host, enter `127.0.0.1`. |
| `是否对公网开放 Web？[y/N]:` | Shown only without an external proxy and without `--web-public`. Enter `y` for option A; otherwise Web binds to `127.0.0.1` by default. |
| `管理员账号 [admin]:` | Press Enter for `admin`, or choose a different account name. |

Alternatively, **choose one of these non-interactive commands** instead of the wizard. Option A (PMail serves HTTPS directly):

```bash
PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.3 ./deploy/deploy.sh init --yes --domain example.com --web-domain mail.example.com --ssl-type 0 --web-public --admin-account admin
```

Option B (recommended, external NPM). Replace `<tag-containing-this-fix>` before running:

```bash
PMAIL_IMAGE='dingding229/pmail-pro:<tag-containing-this-fix>' \
  ./deploy/deploy.sh init --yes --reverse-proxy --domain example.com --web-domain mail.example.com --admin-account admin
```

`init` writes the image, domains, certificate mode, web bind address, and administrator settings to `.env` with permissions `600`. On a fresh setup, look for `初始化完成。最终项目名：pmail` (project initialized), `管理员账号：admin`, and `管理员密码（仅此处显示）：…` (admin password), followed by DNS guidance. **Save the password shown at this point** and back up `.env` securely. In particular, do not regenerate `PMAIL_SECRET_KEY` (credential encryption/SRS) or `POSTGRES_PASSWORD` for an existing installation (the PostgreSQL volume does not change its password when `.env` changes). The bootstrap password stays in `.env`, but it is not the current password once the account password has been changed. If an existing `.env` already sets an image, rerunning `init` will not replace it with the shell value; check `PMAIL_IMAGE` in `.env`.

### 4. Start and check the services

Confirm the A records in step 6 resolve and that you selected direct public access or an external reverse proxy in step 3. Then run:

```bash
./deploy/deploy.sh check
```

On success the script prints `部署前检查通过。` (preflight checks passed). An occupied port or a failed outbound `25` probe may only produce a warning; resolve real port conflicts before proceeding. In proxy mode, the script checks PMail’s mapped host HTTP port (`8080` by default); it does not report NPM’s normal use of public `80/443` as a PMail port conflict. If the configured `172.17.0.1` address does not exist, `check` fails and points to `--web-bind`. `check` checks Docker, Compose configuration, ports, disk, and the required `.env` fields, not public DNS, TLS issuance, or mail delivery.

```bash
./deploy/deploy.sh up
```

The image name contains `/`, so the script pulls PMail rather than building it. On success the script prints two status lines with the same `restart_count`, ten seconds apart. `0` is shown here as an example:

```text
PMail 运行校验：status=running health=healthy restart_count=0
PMail 稳定性校验：status=running health=healthy restart_count=0
```

A healthy container does not mean that HTTPS certificates have already been issued or that the mail listeners are ready. Check container status and logs with:

```bash
./deploy/deploy.sh status
```

```bash
./deploy/deploy.sh logs
```

`logs` shows the last 200 lines and follows new entries. Press `Ctrl+C` to leave the log viewer without stopping the service.

### 5. Open the website: choose A or B

#### A. Direct public access with HTTPS handled by PMail

If you selected mode `0` and public web access during initialization, the script has already enabled PMail web HTTPS. No `.env` edit is needed. Public TCP `80` must reach PMail for HTTP-01; browsers use `443`. Initially, only web HTTP and SMTP `25` may be available. Wait for issuance and the service reload, then open `https://mail.example.com/`. Do not log in over public, unencrypted HTTP while waiting for the certificate.

#### B. Recommended: Nginx Proxy Manager reverse proxy

Run your own [Nginx Proxy Manager (NPM)](https://nginxproxymanager.com/) on the **same host**, listening on public `80/443`. PMail does not install or manage NPM. By default, `--reverse-proxy` in step 3 publishes PMail’s HTTP port `80` on `172.17.0.1:8080` on the host and sets `PMAIL_SSL_TYPE=0` and `PMAIL_HTTPS_ENABLED=2`. PMail does not listen on its container web HTTPS port `443` in this mode. Host port `8443` is mapped but unused; **do not use it as the NPM upstream or open it to the public**.

If NPM runs directly on the host, add `--web-bind 127.0.0.1` at initialization and use `127.0.0.1` as the upstream below. If NPM runs in Docker and the bridge address differs, look up the host’s bridge IPv4 (omit the mask after `/`), then use `--web-bind <ip>` for an address reachable from the NPM container, not the container’s own IP. If `8080` is taken, pick a free port using `--web-port <port>` and use that same port in NPM.

```bash
ip -4 addr show docker0
```

`init` does not switch an existing `.env`. Back up an existing deployment before adjusting its settings; do not delete `.env` or the data volumes.

1. **Web Proxy Host:** In NPM, create `mail.example.com` under **Hosts → Proxy Hosts**. Set **Scheme** to `http`, **Forward Hostname / IP** to `172.17.0.1`, and **Forward Port** to `8080`. Enable **Websockets Support** and **Block Common Exploits**. In **SSL**, request a **Let's Encrypt** certificate for this web host, and enable **Force SSL** and **HTTP/2 Support**.
2. **Mail-validation Proxy Hosts:** Create separate hosts for `smtp.example.com`, `imap.example.com`, and `pop.example.com`, all forwarding to `http://172.17.0.1:8080`. **Do not request or select a certificate in NPM for these hosts, and do not enable Force SSL.** Do not restrict access to the validation path. Requests for `/.well-known/acme-challenge/` on public `80` must reach PMail unchanged for issuance and renewal of its mail certificate. Set up the same three DNS records and Proxy Hosts for each additional mail domain. SMTP, IMAP, and POP3 traffic itself still goes directly to PMail’s mail ports, not through these HTTP proxy hosts.

To avoid exposing a plaintext web login on the mail-validation hostnames, you can add the following to **Advanced** on each **mail-validation** Proxy Host to allow only HTTP-01 requests:

```nginx
if ($uri !~ "^/[.]well-known/acme-challenge/[^/]+$") {
    return 404;
}
```

Once NPM has issued the web certificate, open `https://mail.example.com/`. The mail certificate is separate. Look for `TLS certificate is ready or replaced; reloading listeners` in the PMail logs. After the reload, if SMTP `465` is open, inspect the certificate actually presented there:

```bash
openssl s_client -connect smtp.example.com:465 -servername smtp.example.com -showcerts </dev/null
```

For plain Nginx instead of NPM, see the [Nginx reverse proxy example](#nginx-reverse-proxy-example).

**First login, for either option:** use the administrator account and password printed by `init`. In **Admin → Domain management**, confirm that `example.com` is the primary mail domain. To confirm the actual mailbox address, check **Settings → Addresses and aliases**. Using `mail.example.com` for the website does not automatically make it a second mail domain.

### 6. Configure DNS

Add the following at your DNS provider. Publish the A records before step 4, then add the generated DKIM value after startup. Hostnames in the table are fully qualified; some DNS dashboards instead expect `@` or a relative name (for example, `smtp` for `smtp.example.com`). Use DNS-only records for mail hosts, not an HTTP-only CDN proxy. Remove stale AAAA records that point elsewhere.

| Type | Host/address | Value | Purpose |
| --- | --- | --- | --- |
| A | `example.com` | `203.0.113.10` | Optional: only if the apex should point here too; do not replace an existing website address |
| A | `mail.example.com` | `203.0.113.10` | Website and certificate validation |
| A | `smtp.example.com` | `203.0.113.10` | MX target, SMTP client, and certificate validation |
| A | `imap.example.com` | `203.0.113.10` | IMAP client and certificate validation |
| A | `pop.example.com` | `203.0.113.10` | POP3 host and certificate validation |
| MX | `example.com` | Priority `10`, target `smtp.example.com.` | Inbound delivery; use a hostname, not an IP |
| TXT (SPF) | `example.com` | `v=spf1 a mx ~all` | PMail's expected default; merge authorized senders into an existing SPF record instead of adding a second one |
| TXT (DKIM) | `default._domainkey.example.com` | `<full DKIM TXT value shown in Domains>` | Use the actual selector and complete public key |
| TXT (DMARC) | `_dmarc.example.com` | `v=DMARC1; p=none` | PMail's initial expected record; monitor alignment before tightening policy |
| PTR | `203.0.113.10` | `smtp.example.com.` | Configure through your IP provider; the target's A record should resolve back to the same IP |

For DKIM, open the domain's DNS records in **Domain management** as an administrator and copy the **expected** DKIM value. Alternatively, use `deploy.sh dns`. This requires `curl`; `jq` or `python3` is optional for formatted records. Without either, the script prints each domain's raw DNS JSON.

Sign in as an administrator, create a token under **Settings → API Token** (for example, name it `dns-check`), and copy the token shown at creation. A regular user's token cannot access the admin domain API. The commands below read the token without echoing it. Both deployment options use the public HTTPS URL explicitly; the script's default local HTTP URL may be unavailable or redirect.

```bash
read -rsp 'PMail API Token: ' token; printf '\n'
PMAIL_API_URL=https://mail.example.com PMAIL_API_TOKEN="$token" ./deploy/deploy.sh dns
unset token
```

The script reports the expected MX, SPF, DKIM, and DMARC records and whether they match. Check A and PTR separately. Allow time for DNS caches to expire, then verify from outside the server:

```bash
for host in mail.example.com smtp.example.com imap.example.com pop.example.com; do dig +short A "$host"; done
dig +short MX example.com
dig +short TXT example.com
dig +short TXT default._domainkey.example.com
dig +short TXT _dmarc.example.com
dig +short -x 203.0.113.10
```

### 7. Verify mail delivery and configure clients

Use an **external mailbox** to send a message to `admin@example.com` or the address shown under **Addresses and aliases**. Check the PMail web inbox and reply from the web app. Outbound messages are queued; a queued status is not proof of delivery. Check **Settings → Outbound queue** and confirm that the external mailbox receives the reply. You can verify web send/receive before configuring a mail client.

After the certificate is ready, choose one incoming and one outgoing option below in your client. Use the full mailbox address (such as `admin@example.com`) or the actual PMail account name as the username, and its login password (not an API token). If you changed host port mappings, use the mapped ports. Do not disable certificate verification to work around an error.

| Protocol | Server | Default port | Encryption |
| --- | --- | --- | --- |
| IMAP incoming (recommended) | `imap.example.com` | `993` | SSL/TLS (implicit) |
| IMAP incoming (optional) | `imap.example.com` | `143` | STARTTLS |
| SMTP outgoing (recommended) | `smtp.example.com` | `465` | SSL/TLS (implicit) |
| SMTP outgoing (optional) | `smtp.example.com` | `587` | STARTTLS |
| POP3 incoming (optional) | `pop.example.com` | `995` | SSL/TLS (implicit) |

Enable SMTP authentication in the client. Port `25` is for server-to-server delivery, not client submission.

### 8. Routine operations

**Upgrade the image:** make sure the target tag has been published. The example uses beta.4. You can also set `UPGRADE_VERSION`; `--version` takes precedence when both are set. The default is `latest` only when neither is set. Specify pre-release tags explicitly.

```bash
./deploy/deploy.sh upgrade --version 3.0.0-beta.4
```

The script backs up PostgreSQL, pulls the target image, and waits for health checks. If the upgrade fails, it attempts to restore the old image and pre-upgrade database. **The upgrade still exits nonzero even if rollback succeeds.** If the database restore fails, the script reports the error and backup path rather than declaring rollback successful. For source builds, `upgrade` instead builds a project-scoped `pmail-<project>:<tag>` image with a timestamped tag.

**Backup:** the script prints `备份完成：<path>` and saves the dump to `backups/<project>-<UTC timestamp>.dump` in the deployment directory. It backs up PostgreSQL, not `.env` or the `pmail_config` volume (certificates, for example). Back up the external proxy’s configuration and certificates separately. Keep copies somewhere other than this server.

```bash
./deploy/deploy.sh backup
```

**Restore:** pauses PMail while restoring the selected database dump. If the database restore fails, the script reports the error and does not restart the service. Back up the current database first. If manually rolling back a version, first make sure `PMAIL_IMAGE` in `.env` points to the matching old image and that the image is available. At the prompt, paste the full path printed by `backup` without quotes or the `备份完成：` prefix.

```bash
read -r -p 'Full path to the backup dump: ' dump && ./deploy/deploy.sh restore "$dump"
```

Use `status` and `logs` from [step 4](#4-start-and-check-the-services) and sign in to verify the restored data. Do not restore a v3 dump to upstream v2; see the [upgrade notes](docs/UPGRADE.md).

**Stop:** removes this project's containers and Compose network but preserves named volumes. Run `up` again to restart.

```bash
./deploy/deploy.sh down
```

**Uninstall but retain data:** confirm the prompt to remove containers and keep the volumes.

```bash
./deploy/deploy.sh uninstall
```

**Uninstall and delete data:** keep an off-server backup first. This command removes the containers before asking whether to delete volumes. Confirming deletes the PostgreSQL and PMail configuration volumes; it does not remove external NPM containers, configuration, or certificates. It does not delete `.env` or `backups/`.

```bash
./deploy/deploy.sh uninstall --purge
```

### 9. Troubleshooting

- **Port 25 is blocked:** check the cloud security group and firewall from an external network for inbound traffic. For outbound traffic, try the command below against a reachable receiving MX and ask your provider about restrictions if it fails. `check` probes only `1.1.1.1:25`; one failed probe does not establish a provider block. If outbound `25` is blocked, request an exemption or arrange a separate relay. This guide does not configure a relay.

```bash
nc -vz -w 5 '<external mailbox provider MX hostname>' 25
```

- **Certificate issuance failed:** check public DNS, stale AAAA records, where public ports `80` and `443` lead, and the PMail and NPM logs. Option A must deliver port `80` challenges to PMail. In option B, check that the three mail-validation Proxy Hosts do not force HTTPS or intercept `/.well-known/acme-challenge/`, and that NPM manages its own web certificate. Do not switch to mode `3` just to get web HTTPS; mode `3` disables PMail's TLS mail listeners.
- **Website unreachable:** `PMAIL_WEB_BIND` defaults to `127.0.0.1`. For option A, check the public bind setting, `PMAIL_HTTPS_ENABLED=0`, and the certificate. For option B, check NPM logs and whether it can reach the upstream `PMAIL_WEB_BIND:PMAIL_HOST_HTTP_PORT` specified in `.env`. If running Nginx on the host instead, keep PMail bound to loopback.
- **Outgoing mail goes to spam:** verify SPF, DKIM and DMARC alignment, PTR/rDNS, and IP reputation. Inspect the message headers at the receiving mailbox. A working website does not validate these records.
- **Forgot the admin password:** if it has never changed, check the saved `init` output or the bootstrap `PMAIL_ADMIN_PASSWORD` in `.env`. Another signed-in administrator can reset it in **User management**. If every admin is locked out, there is no unauthenticated reset page or `deploy.sh` reset command. Editing the bootstrap password in `.env` and restarting does not reset an existing account. Plan recovery from a suitable backup with known credentials; do not erase the data volumes to reset a password.
- **External mailbox collection stalls:** check `last_error`, the provider's app password, TLS mode, and SSRF restrictions.

## Features

- PostgreSQL 17 deployment with persistent `pg_data` and `pmail_config` volumes.
- Multiple local domains, primary addresses, aliases, sender permissions, quotas, and DNS guidance.
- SMTP/IMAP/POP3 services, outbound queue retries, DSN-style failure handling, and local delivery.
- Global forwarding, multi-target rules, manual forwarding, SRS and loop protection.
- External IMAP/POP3 collection with UID/UIDL deduplication and encrypted credentials.
- API tokens, bcrypt password migration, responsive desktop/tablet/mobile UI, and Chinese/English i18n.

## Configuration

All deployment variables are documented in `.env.example`. Important variables:

| Variable | Meaning |
|---|---|
| `PMAIL_PROJECT_NAME` | Docker Compose project name; use a different value for each deployment on one host. |
| `PMAIL_IMAGE` | Empty/local mode uses the project-scoped `pmail-<project>:local` image; set a registry image such as `dingding229/pmail-pro:3.0.0-beta.3` to pull instead. |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` | PostgreSQL bootstrap settings. |
| `PMAIL_DB_TYPE`, `PMAIL_DB_DSN` | Database type/DSN; Compose normally supplies PostgreSQL. |
| `PMAIL_DOMAIN`, `PMAIL_DOMAINS`, `PMAIL_WEB_DOMAIN` | Mail and web domains. |
| `PMAIL_HTTP_PORT`, `PMAIL_HTTPS_PORT` | Container web ports. |
| `PMAIL_HOST_*_PORT` | Host port mappings for SMTP, POP3, IMAP and web. |
| `PMAIL_MAIL_BIND`, `PMAIL_WEB_BIND` | Mail/web bind addresses; mail defaults to `0.0.0.0`, web to `127.0.0.1`. |
| `PMAIL_SSL_TYPE` | `0` ACME HTTP-01, `1` user certificate, `2` ACME DNS-01, `3` reverse-proxy HTTP only. |
| `PMAIL_HTTPS_ENABLED` | HTTPS listener mode; SSL type 3 keeps the backend HTTP-only. |
| `PMAIL_LOG_LEVEL` | `debug`, `info`, `warn` or `error`. |
| `PMAIL_SPAM_FILTER_LEVEL` | SPF/DKIM filtering level 0–3. |
| `PMAIL_CATCH_ALL` | Whether unknown local recipients go to the administrator. |
| `PMAIL_ALLOW_USER_ALIAS` | Allow non-admin users to create aliases. |
| `PMAIL_QUEUE_WORKERS` | Outbound queue concurrency. |
| `PMAIL_FETCH_WORKERS` | External mailbox collection concurrency. |
| `PMAIL_FETCH_ALLOW_PRIVATE` | Allow private destinations for controlled test environments only. |
| `PMAIL_SECRET_KEY` | Encryption/SRS secret; keep stable during upgrades. |
| `PMAIL_ADMIN_ACCOUNT`, `PMAIL_ADMIN_PASSWORD` | First-run administrator bootstrap. |

The deployment helper also accepts `PMAIL_ENV_FILE`, `PMAIL_API_URL`, and `PMAIL_API_TOKEN`. `PMAIL_QUEUE_WORKERS`, `PMAIL_FETCH_WORKERS`, and `PMAIL_FETCH_ALLOW_PRIVATE` are application environment variables. The base Compose file does not pass them through; use an additional Compose override to pass them into `pmail.environment`. PostgreSQL and optional MySQL test connections use `PMAIL_TEST_PG_DSN` and `PMAIL_TEST_MYSQL_DSN`, respectively.

## Nginx reverse proxy example

Use this alternative if you already manage a web certificate with Nginx on the same host. Initialize with `--reverse-proxy --web-bind 127.0.0.1` in step 3, keep the default upstream `127.0.0.1:8080`, and start with step 4.

In Nginx, use the configuration below with your own certificate paths. Nginx handles public web HTTPS and proxies public HTTP-01 challenges to PMail. Do not expose `8080` or `8443` publicly.

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

## Multiple deployments and local images

Run separate deployments with distinct project names, for example `./deploy/deploy.sh init --project pmail-prod` and `./deploy/deploy.sh init --project pmail-staging`. Local images are named `pmail-<project>:local` and are rebuilt by `up`, so projects do not reuse one another's local tag. Set `PMAIL_IMAGE` to a registry image when using a prebuilt image; registry mode pulls instead of building. Container-affecting commands (`up`, `upgrade`, `backup`, `restore`, `dns`, `migrate-*`, `down`, and `uninstall`) verify that the Compose project belongs to the current directory before changing containers.

## Prebuilt images (GitHub Actions → Docker Hub)

`.github/workflows/docker.yml` builds `linux/amd64` and `linux/arm64` images and pushes them to Docker Hub. Configure the repository under Settings → Secrets and variables → Actions:

| Kind | Name | Notes |
| --- | --- | --- |
| Variable | `DOCKERHUB_USERNAME` | Docker Hub user or organization |
| Variable | `DOCKERHUB_IMAGE` | Optional, defaults to `<DOCKERHUB_USERNAME>/pmail-pro` |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub personal access token with Read & Write scope |

Triggers:
- Pushing a `v*` tag (e.g. `v3.0.0`) publishes `3.0.0`, `3.0`, `3`, `latest` and `sha-xxxxxxx`; pre-release tags containing `-` do not move `latest`.
- Running "PMail Pro Docker image" manually from the Actions tab publishes the branch name (e.g. `feature-v3`), `sha-xxxxxxx` and an optional custom tag.

Set `PMAIL_IMAGE=dingding229/pmail-pro:3.0.0-beta.3` in `.env` to pull it; `./deploy/deploy.sh upgrade --version 3.1.0` switches versions (defaults to `latest`); on failure the script attempts to roll back.

## Migrating old installations

Stop the old single-file deployment, place the SQLite file on the host, and replace `<path-to-pmail.db>` with its path:

```bash
./deploy/deploy.sh migrate-from-sqlite '<path-to-pmail.db>'
```

Or, for MySQL, replace `<mysql-dsn>` with the actual DSN (format: `user:password@tcp(<mysql-host>:3306)/pmail?parseTime=True`).

```bash
./deploy/deploy.sh migrate-from-mysql '<mysql-dsn>'
```

The migration command imports supported legacy tables into PostgreSQL. Verify domains, aliases, DKIM, quotas and queue state before allowing external traffic.

## Go development

From `server/`, `go test ./...` runs the default suite. Use `go test -tags integration ./...` for integration tests locally or on a server. The full suite requires root (SMTP port 25 and other fixed mail ports must be free) and a **disposable, empty PostgreSQL database** set via `PMAIL_TEST_PG_DSN`. The IMAP fixture creates and drops a private schema, so the database user needs permission to create schemas. Do not point the suite at production. For an optional MySQL full-server run, set `PMAIL_TEST_MYSQL_DSN` and use `go test -tags integration . -args mysql`; CI uses PostgreSQL and does not require MySQL.

See [README_CN.md](README_CN.md), [docs/API.md](docs/API.md), and [docs/UPGRADE.md](docs/UPGRADE.md).
