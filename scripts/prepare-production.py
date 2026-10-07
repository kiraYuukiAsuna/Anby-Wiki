"""Initialize host data directories and private production settings once."""

import base64
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys

from production_environment import domain, load, update


def main() -> None:
    os.umask(0o077)
    root = Path(sys.argv[1]).resolve(strict=True)
    environment = Path(os.environ.get("DEPLOY_ENV_FILE", str(root / ".env"))).resolve()
    for directory in [root / "data", root / "Secret", root / "Secret/deploy-backups"]:
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    fresh = not environment.exists()
    if fresh:
        for directory in [root / "data/postgres", root / "data/minio", root / "data/meilisearch"]:
            if directory.exists() and any(directory.iterdir()):
                raise ValueError("Existing service data requires the original deployment environment.")
        environment.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        with environment.open("x", encoding="utf-8", newline="\n") as handle:
            handle.write((root / "infra/deploy/.env.example").read_text(encoding="utf-8"))
        environment.chmod(0o600)
        update(environment, {
            "ANBY_DOMAIN": os.environ.get("ANBY_DOMAIN", "anbywiki.momiya.cloud"),
            "CERTBOT_ACCOUNT": os.environ.get("CERTBOT_ACCOUNT", ""),
            "CERTBOT_EMAIL": os.environ.get("CERTBOT_EMAIL", ""),
            "POSTGRES_PASSWORD": secrets.token_hex(32),
            "S3_ACCESS_KEY": "anbywiki",
            "S3_SECRET_KEY": secrets.token_hex(32),
            "MEILI_MASTER_KEY": secrets.token_hex(32),
            "AI_CONFIG_MASTER_KEY": base64.b64encode(secrets.token_bytes(32)).decode(),
            "AI_KERNEL_INTERNAL_TOKEN": secrets.token_hex(32),
            "SESSION_COOKIE_SECURE": "true",
            "AUTH_REGISTRATION_ENABLED": "true",
        })
        values = load(environment)
        credentials = {
            "username": "admin", "email": "admin@" + values["ANBY_DOMAIN"],
            "password": "Anby!" + secrets.token_urlsafe(24),
            "display_name": "Anby Wiki Administrator", "initialized": False,
        }
        credential_path = root / "Secret/bootstrap-admin.json"
        with credential_path.open("x", encoding="utf-8") as handle:
            json.dump(credentials, handle, indent=2)
            handle.write("\n")
    values = load(environment)
    if values.get("ENV") != "production":
        raise ValueError("Production deployment requires ENV=production.")
    if values.get("SESSION_COOKIE_SECURE") != "true":
        raise ValueError("HTTPS deployment requires SESSION_COOKIE_SECURE=true.")
    settings = domain(values)
    revision = subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip()
    update(environment, {
        "RELEASE_ID": revision[:12],
        "COLLABORATION_ORIGIN_PATTERNS": "https://" + settings["ANBY_DOMAIN"],
    })
    for name in ["postgres", "minio", "meilisearch", "deploy-backups"]:
        (root / "data" / name).mkdir(mode=0o700, exist_ok=True)
    print("Production settings prepared; existing credentials preserved.")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(f"Production preparation failed: {error}")
