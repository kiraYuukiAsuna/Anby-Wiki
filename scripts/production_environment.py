"""Read deployment dotenv without evaluating shell code or interpolating secrets."""

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import stat
import sys

PUBLIC_KEYS = {"ANBY_DOMAIN", "CERTBOT_ACCOUNT", "CERTBOT_EMAIL", "WEB_PORT", "WEB_BIND", "RELEASE_ID"}


def load(path: Path) -> dict[str, str]:
    if path.is_symlink() or not path.is_file():
        raise ValueError("Deployment environment must be a regular file.")
    if os.name != "nt" and stat.S_IMODE(path.stat().st_mode) & 0o077:
        raise ValueError("Deployment environment must not be group/world accessible.")
    values = {}
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        match = re.fullmatch(r"(?:export\s+)?([A-Z][A-Z0-9_]*)\s*=\s*(.*)", line)
        if not match:
            raise ValueError(f"Invalid dotenv syntax at line {number}.")
        key, value = match.groups()
        if key in values:
            raise ValueError(f"Duplicate dotenv key: {key}")
        if value.startswith('"'):
            value = json.loads(value)
        elif value.startswith("'"):
            if not value.endswith("'") or len(value) < 2:
                raise ValueError(f"Unterminated dotenv value at line {number}.")
            value = value[1:-1]
        if not isinstance(value, str) or "\n" in value or "\r" in value or "\0" in value:
            raise ValueError(f"Invalid dotenv value at line {number}.")
        values[key] = value
    return values


def update(path: Path, changes: dict[str, str]) -> None:
    lines = path.read_text(encoding="utf-8").splitlines()
    pending = dict(changes)
    for index, line in enumerate(lines):
        match = re.match(r"(?:export\s+)?([A-Z][A-Z0-9_]*)\s*=", line.strip())
        if match and match.group(1) in pending:
            key = match.group(1)
            lines[index] = key + "=" + encode(pending.pop(key))
    lines.extend(key + "=" + encode(value) for key, value in pending.items())
    temporary = path.with_name(path.name + ".new")
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as handle:
            handle.write("\n".join(lines) + "\n")
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def encode(value: str) -> str:
    if re.fullmatch(r"[a-zA-Z0-9_.:/+@%=,?-]*", value):
        return value
    if "'" in value or "\n" in value or "\r" in value:
        raise ValueError("Unsupported deployment value.")
    return "'" + value + "'"


def domain(values: dict[str, str]) -> dict[str, str]:
    hostname = values.get("ANBY_DOMAIN", "")
    if len(hostname) > 253 or not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+", hostname):
        raise ValueError("ANBY_DOMAIN must be a lowercase DNS hostname without scheme, path or port.")
    port = values.get("WEB_PORT", "60019")
    if not re.fullmatch(r"[1-9][0-9]{0,4}", port) or int(port) > 65535:
        raise ValueError("WEB_PORT must be an integer from 1 to 65535.")
    if values.get("WEB_BIND", "127.0.0.1") != "127.0.0.1":
        raise ValueError("Host Nginx deployment requires WEB_BIND=127.0.0.1.")
    account = values.get("CERTBOT_ACCOUNT", "")
    email = values.get("CERTBOT_EMAIL", "")
    if account and not re.fullmatch(r"[a-f0-9]{32}", account):
        raise ValueError("CERTBOT_ACCOUNT must be an existing Certbot account ID.")
    if not account and not re.fullmatch(r"[^\s@-][^\s@]*@[^\s@]+", email):
        raise ValueError("Set CERTBOT_ACCOUNT or CERTBOT_EMAIL before deploying.")
    return dict(ANBY_DOMAIN=hostname, WEB_PORT=port, CERTBOT_ACCOUNT=account, CERTBOT_EMAIL=email)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["exec", "domain", "get"])
    parser.add_argument("environment", type=Path)
    parser.add_argument("arguments", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    values = load(args.environment)
    if args.command == "exec":
        if not args.arguments:
            parser.error("exec requires a command")
        environment = {**os.environ, **values}
        environment["DEPLOY_ENV_FILE"] = str(args.environment.resolve())
        environment["ANBY_DEPLOY_ENV_LOADED"] = str(args.environment.resolve())
        os.execvpe(args.arguments[0], args.arguments, environment)
    elif args.command == "domain":
        for key, value in domain(values).items():
            print(key + "=" + shlex.quote(value))
    else:
        if len(args.arguments) != 1 or args.arguments[0] not in PUBLIC_KEYS:
            parser.error("get accepts one public deployment setting")
        print(values.get(args.arguments[0], ""))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(f"Deployment environment error: {error}")
