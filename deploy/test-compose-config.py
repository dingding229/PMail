"""Assert bundled Compose compatibility; needs the CLI, not a running daemon."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
ENV = {k: v for k, v in os.environ.items() if not k.startswith(("PMAIL_", "COMPOSE_", "POSTGRES_"))}


def config(root, env=None, override=False):
    args = ["docker", "compose", "--profile", "updater", "-f", "docker-compose.yml"]
    if override:
        args += ["-f", override]
    return json.loads(subprocess.check_output(args + ["config", "--format", "json"], cwd=root, env=ENV | (env or {})))


def port(app, target):
    item, = [p for p in app["ports"] if p["target"] == target]
    return item["host_ip"], str(item["published"])


with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    for filename in ("docker-compose.yml", "docker-compose.public.yml", "docker-compose.build.yml"):
        shutil.copyfile(ROOT / filename, root / filename)
    fresh = config(root)
    assert fresh["name"] == "pmail"
    assert fresh["volumes"]["pg_data"]["name"] == "pmail_pg_data"
    assert fresh["volumes"]["pmail_config"]["name"] == "pmail_pmail_config"
    assert fresh["volumes"]["secrets"]["name"] == "pmail_secrets"
    app = fresh["services"]["pmail"]
    assert app["environment"]["PMAIL_DOMAIN"] == ""
    assert app["environment"]["PMAIL_WEB_MODE"] == "proxy"
    assert app["environment"]["PMAIL_TRUSTED_PROXIES"] == ""
    assert port(app, 80) == ("172.17.0.1", "8080")
    assert fresh["services"]["updater"]["environment"]["PMAIL_UPDATER_PROJECT"] == "pmail"

    (root / ".env").write_text("""PMAIL_PROJECT_NAME=legacy_mail
PMAIL_IMAGE=example.invalid/pmail:local
PMAIL_UPDATER_IMAGE=example.invalid/updater:local
POSTGRES_USER=legacy_user
POSTGRES_DB=legacy_db
POSTGRES_PASSWORD=placeholder
PMAIL_WEB_BIND=127.0.0.1
PMAIL_HTTP_PORT=9080
PMAIL_HTTPS_PORT=9443
PMAIL_HOST_HTTP_PORT=9081
PMAIL_HOST_HTTPS_PORT=9444
PMAIL_HOST_SMTP_PORT=2525
PMAIL_DOMAIN=example.test
PMAIL_SPAM_KEYWORDS=
PMAIL_UPDATER_TOKEN=example-test-value
PMAIL_QUEUE_WORKERS=2
""", encoding="utf-8")
    legacy = config(root, {"PMAIL_ADMIN_ACCOUNT": "shell_admin"})
    svc = legacy["services"]
    app = svc["pmail"]
    assert legacy["name"] == "legacy_mail"
    assert legacy["volumes"]["pg_data"]["name"] == "legacy_mail_pg_data"
    assert legacy["volumes"]["updater_state"]["name"] == "legacy_mail_updater_state"
    assert svc["updater"]["environment"]["PMAIL_UPDATER_PROJECT"] == "legacy_mail"
    assert svc["updater"]["environment"]["PMAIL_UPDATER_TOKEN"] == "example-test-value"
    assert svc["updater"]["image"] == "example.invalid/updater:local"
    assert app["image"] == svc["init-secrets"]["image"] == "example.invalid/pmail:local"
    assert app["environment"]["PMAIL_DB_USER"] == svc["postgres"]["environment"]["POSTGRES_USER"] == "legacy_user"
    assert app["environment"]["PMAIL_DB_NAME"] == svc["postgres"]["environment"]["POSTGRES_DB"] == "legacy_db"
    assert svc["init-secrets"]["environment"]["POSTGRES_PASSWORD"] == "placeholder"
    assert "POSTGRES_PASSWORD" not in app["environment"]
    assert "POSTGRES_PASSWORD" not in svc["updater"]["environment"]
    assert app["environment"]["PMAIL_SPAM_KEYWORDS"] == ""
    assert app["environment"]["PMAIL_ADMIN_ACCOUNT"] == "shell_admin"
    assert app["environment"]["PMAIL_QUEUE_WORKERS"] == "2"
    assert port(app, 9080) == ("127.0.0.1", "9081")
    assert port(app, 9443) == ("127.0.0.1", "9444")
    assert port(app, 25)[1] == "2525"
    public = config(root, {"COMPOSE_PROJECT_NAME": "public_mail"}, "docker-compose.public.yml")
    assert public["name"] == "public_mail"
    assert public["services"]["updater"]["environment"]["PMAIL_UPDATER_PROJECT"] == "public_mail"
    assert public["services"]["updater"]["environment"]["PMAIL_UPDATER_PUBLIC"] == "1"
    assert public["services"]["pmail"]["environment"]["PMAIL_WEB_MODE"] == "public"
    assert len(public["services"]["pmail"]["ports"]) == 9
    assert port(public["services"]["pmail"], 9080) == ("127.0.0.1", "9081")
    local = config(root, override="docker-compose.build.yml")["services"]
    assert local["pmail"]["image"] == local["init-secrets"]["image"] == "pmail-pro:local"
    assert local["updater"]["image"] == "pmail-pro-updater:local"
print("Compose configuration checks passed")
