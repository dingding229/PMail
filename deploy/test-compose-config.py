"""Assert bundled Compose compatibility; needs the CLI, not a running daemon."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
ENV = {k: v for k, v in os.environ.items() if not k.startswith(("PMAIL_", "COMPOSE_", "POSTGRES_"))}


def config(root, env=None, override=None):
    args = ["docker", "compose", "-f", "docker-compose.yml"]
    if override:
        args += ["-f", override]
    return json.loads(subprocess.check_output(args + ["config", "--format", "json"], cwd=root, env=ENV | (env or {})))


def port(app, target):
    item, = [p for p in app["ports"] if p["target"] == target]
    return item.get("host_ip"), str(item["published"])


with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    for filename in ("docker-compose.yml", "docker-compose.public.yml", "docker-compose.build.yml"):
        shutil.copyfile(ROOT / filename, root / filename)
    fresh = config(root)
    assert fresh["name"] == "pmail"
    assert {k: v["name"] for k, v in fresh["volumes"].items()} == {
        "pg_data": "pmail_pg_data", "pmail_config": "pmail_pmail_config", "secrets": "pmail_secrets",
    }
    assert set(fresh["services"]) == {"pmail", "postgres", "init-secrets"}
    app = fresh["services"]["pmail"]
    assert app["restart"] == "unless-stopped"
    assert "PMAIL_DOMAIN" not in app.get("environment", {})
    assert "PMAIL_TRUSTED_PROXIES" not in app.get("environment", {})
    assert port(app, 80) == ("172.17.0.1", "8080")
    assert len(app["ports"]) == 8
    assert not any("docker.sock" in str(mount) for mount in app["volumes"])

    (root / ".env").write_text("""PMAIL_PROJECT_NAME=legacy_mail
PMAIL_IMAGE=example.invalid/pmail:local
POSTGRES_PASSWORD=placeholder
PMAIL_DOMAIN=example.test
PMAIL_SPAM_KEYWORDS=
PMAIL_QUEUE_WORKERS=2
PMAIL_WEB_BIND=127.0.0.1
PMAIL_HOST_HTTP_PORT=9081
PMAIL_HOST_SMTP_PORT=2525
""", encoding="utf-8")
    legacy = config(root)
    assert legacy["name"] == "legacy_mail"
    assert legacy["volumes"]["pg_data"]["name"] == "legacy_mail_pg_data"
    svc = legacy["services"]
    app = svc["pmail"]
    assert app["image"] == svc["init-secrets"]["image"] == "example.invalid/pmail:local"
    assert svc["postgres"]["environment"]["POSTGRES_DB"] == svc["postgres"]["environment"]["POSTGRES_USER"] == "pmail"
    assert svc["init-secrets"]["environment"]["POSTGRES_PASSWORD"] == "placeholder"
    assert not app["environment"].get("POSTGRES_PASSWORD")
    assert app["environment"]["PMAIL_SPAM_KEYWORDS"] == ""
    assert app["environment"]["PMAIL_DOMAIN"] == "example.test"
    assert app["environment"]["PMAIL_QUEUE_WORKERS"] == "2"
    assert port(app, 80) == ("172.17.0.1", "8080")
    assert port(app, 25)[1] == "25"
    public = config(root, {"COMPOSE_PROJECT_NAME": "public_mail"}, "docker-compose.public.yml")
    assert public["name"] == "public_mail"
    app = public["services"]["pmail"]
    assert app["environment"]["PMAIL_WEB_MODE"] == "public"
    assert len(app["ports"]) == 9
    assert port(app, 80)[1] == "80"
    assert port(app, 443)[1] == "443"
    local = config(root, override="docker-compose.build.yml")["services"]
    assert local["pmail"]["image"] == local["init-secrets"]["image"] == "pmail-pro:local"
print("Compose configuration checks passed")
