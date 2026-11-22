# PMail Pro

PMail Pro is an enhanced fork of [PMail](https://github.com/Jinnrry/PMail). PMail is a self-hosted mail platform with PostgreSQL as its default database, multi-domain mailboxes, aliases, queue-backed outbound delivery, automatic/rule/manual forwarding, external IMAP/POP3 collection, API tokens, and a responsive Vue 3 web UI.

![Desktop UI placeholder](docs/images/desktop.png)
![Mobile UI placeholder](docs/images/mobile.png)

## Quick start

```bash
./deploy/deploy.sh init --yes --domain mail.example.com --web-domain mail.example.com
./deploy/deploy.sh check && ./deploy/deploy.sh up
```

The interactive form is preferred for first-time setup. It generates PostgreSQL credentials, `PMAIL_SECRET_KEY`, and an administrator password. Save the password printed by `init`.

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
| `PMAIL_IMAGE` | Empty/local mode uses the project-scoped `pmail-<project>:local` image; set a registry image such as `yourname/pmail-pro:3.0.0` to pull instead. |
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

The deployment helper also accepts `PMAIL_ENV_FILE`, `PMAIL_API_URL`, and `PMAIL_API_TOKEN` for script operation. CI uses `PMAIL_TEST_PG_DSN` and `PMAIL_TEST_MYSQL_DSN`.

## DNS and certificates

For every mail domain configure MX, SPF, DKIM and DMARC. Set PTR/rDNS for the outbound IP. Run `./deploy/deploy.sh dns` with `PMAIL_API_TOKEN` after startup to compare expected and observed DNS records. `curl` is required; `jq` or `python3` is optional and formats the records, while a grep/sed fallback prints each domain's raw DNS JSON.

Choose SSL mode 0 when PMail owns ACME HTTP-01, mode 2 for a configured DNS provider, mode 1 when certificate files are supplied, or mode 3 when a reverse proxy owns web TLS. Mail protocol TLS must still have a certificate; do not expose authenticated plaintext POP3/SMTP without a deliberate firewall policy.

### Caddy

```bash
./deploy/deploy.sh up
# or validate the override:
docker compose --env-file .env -f docker-compose.yml -f docker-compose.caddy.yml config
```

The Caddy override terminates web HTTPS and proxies to PMail's internal HTTP listener. PMail remains responsible for mail-protocol certificates using its own ACME mode. Use a distinct web domain when the two certificate owners would otherwise compete for the same ACME challenge.

### Nginx

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

Set `PMAIL_IMAGE=yourname/pmail-pro:3.0.0` in `.env` to pull it; `UPGRADE_VERSION=3.1.0 ./deploy/deploy.sh upgrade` switches versions (defaults to `latest`) with automatic rollback.

## Backup, restore and upgrade

```bash
./deploy/deploy.sh backup
./deploy/deploy.sh restore backups/<project>-<timestamp>.dump
./deploy/deploy.sh upgrade
```

`upgrade` creates a PostgreSQL backup, builds a project-scoped timestamped `pmail-<project>:<tag>` image, waits for health, and restores the previous image/database backup when startup fails. Keep the printed backup path until the new release is verified.

## Migrating old installations

Stop the old single-file deployment, place the SQLite file on the host, then run:

```bash
./deploy/deploy.sh migrate-from-sqlite /path/to/pmail.db
# or
./deploy/deploy.sh migrate-from-mysql 'user:password@tcp(host:3306)/pmail?parseTime=True'
```

The migration command imports supported legacy tables into PostgreSQL. Verify domains, aliases, DKIM, quotas and queue state before allowing external traffic.

## Go development

From `server/`, `go test ./...` runs the default suite. Use `go test -tags integration ./...` for integration tests locally or on a server. The full suite requires root (SMTP port 25 and other fixed mail ports must be free) and a **disposable, empty PostgreSQL database** set via `PMAIL_TEST_PG_DSN`. The IMAP fixture creates and drops a private schema, so the database user needs permission to create schemas. Do not point the suite at production. For an optional MySQL full-server run, set `PMAIL_TEST_MYSQL_DSN` and use `go test -tags integration . -args mysql`; CI uses PostgreSQL and does not require MySQL.

## Troubleshooting

- **Port 25 blocked:** ask the cloud provider for SMTP egress or use a relay; configure PTR/rDNS.
- **Messages in spam:** verify PTR, SPF, DKIM alignment, DMARC policy and reverse DNS.
- **Web unavailable behind a proxy:** keep `PMAIL_WEB_BIND=127.0.0.1`, verify proxy upstream and health at `/api/ping`.
- **External mailbox stuck:** inspect `last_error`, account health, TLS mode, app password and SSRF allowlist settings.
- **Rollback:** keep the upgrade backup and use `restore`, then set `PMAIL_IMAGE` back to the previous tag.

See [README_CN.md](README_CN.md), [docs/API.md](docs/API.md), and [docs/UPGRADE.md](docs/UPGRADE.md).
