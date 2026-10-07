"""Create the initial administrator through the domain API on loopback only."""

import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.request

from production_environment import load, update


def main() -> None:
    os.umask(0o077)
    root = Path(sys.argv[1]).resolve(strict=True)
    environment = Path(sys.argv[2])
    credentials_path = root / "Secret/bootstrap-admin.json"
    if not credentials_path.exists():
        print("Existing deployment has no bootstrap credentials; administrator unchanged.")
        return
    credentials = json.loads(credentials_path.read_text())
    if credentials.get("initialized"):
        print("Initial administrator already configured.")
        return
    values = load(environment)
    base = "http://127.0.0.1:" + values.get("WEB_PORT", "60019")
    cookie = None

    def request(path, payload=None, method="GET"):
        headers = {"Content-Type": "application/json"}
        if cookie:
            headers["Cookie"] = cookie
        data = None if payload is None else json.dumps(payload).encode()
        req = urllib.request.Request(base + path, data=data, method=method, headers=headers)
        try:
            return urllib.request.urlopen(req, timeout=30)
        except urllib.error.HTTPError as error:
            raise ValueError(f"Administrator API {path} returned {error.code}.") from None

    # Login first makes interruption recovery idempotent after successful registration.
    try:
        response = request("/api/v1/auth/login", {"identifier": credentials["username"], "password": credentials["password"]}, "POST")
    except ValueError:
        if values.get("AUTH_REGISTRATION_ENABLED") != "true":
            raise ValueError("Initial administrator requires registration enabled on the private loopback service.") from None
        response = request("/api/v1/auth/register", {key: credentials[key] for key in ["username", "email", "password", "display_name"]}, "POST")
    with response:
        cookie = response.headers.get("Set-Cookie", "").split(";", 1)[0]
        if not cookie:
            raise ValueError("Administrator API did not establish a session.")
    try:
        with request("/api/v1/admin/users") as response:
            if response.status != 200:
                raise ValueError("Initial account did not receive administrator permissions.")
        update(environment, {"AUTH_REGISTRATION_ENABLED": "false"})
        credentials["initialized"] = True
        temporary = credentials_path.with_suffix(".new")
        with temporary.open("x", encoding="utf-8") as handle:
            handle.write(json.dumps(credentials, indent=2) + "\n")
        os.replace(temporary, credentials_path)
    finally:
        with request("/api/v1/auth/logout", method="POST"):
            pass
    print("Initial administrator verified; public registration disabled; credentials in Secret/bootstrap-admin.json.")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(f"Administrator bootstrap failed: {error}")
