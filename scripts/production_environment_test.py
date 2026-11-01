import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import production_environment as environment


class TrustedProxyTest(unittest.TestCase):
    def discovery(self, web_ip="172.25.0.9", gateway="172.25.0.1", network_label="app"):
        records = [
            {
                "labels": {"com.docker.compose.project": "wiki", "com.docker.compose.service": service},
                "running": True,
                "networks": {"wiki_app": {"IPAddress": ip, "Gateway": gateway}},
            }
            for service, ip in (("api", "172.25.0.5"), ("web", web_ip))
        ]
        network = {
            "name": "wiki_app", "driver": "bridge",
            "labels": {"com.docker.compose.project": "wiki", "com.docker.compose.network": network_label},
        }
        return [
            subprocess.CompletedProcess([], 0, "api-id\nweb-id\n", ""),
            subprocess.CompletedProcess([], 0, "\n".join(map(json.dumps, records)), ""),
            subprocess.CompletedProcess([], 0, json.dumps(network), ""),
        ]

    def test_refresh_replaces_stale_peers_and_preserves_dotenv(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / ".env"
            path.write_text("TRUSTED_PROXY_IPS=192.0.2.1\nUNRELATED='literal $ value'\n")
            path.chmod(0o600)
            for web_ip, gateway in (("172.25.0.9", "172.25.0.1"), ("172.26.0.8", "172.26.0.1")):
                with patch.object(environment.subprocess, "run", side_effect=self.discovery(web_ip, gateway)) as docker:
                    peers = environment.trusted_proxy_ips(path, Path("compose.yml"), environment.load(path))
                    environment.update(path, {"TRUSTED_PROXY_IPS": peers})
                self.assertEqual(set(peers.split(",")), {gateway, web_ip})
                self.assertEqual(environment.load(path)["UNRELATED"], "literal $ value")
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                self.assertEqual(docker.call_args_list[0].args[0][-4:], ["ps", "-q", "api", "web"])
                self.assertNotIn(".Config.Env", docker.call_args_list[1].args[0][3])

    def test_unknown_topology_and_invalid_addresses_fail_closed(self):
        for outputs in (
            self.discovery(network_label="other"),
            self.discovery(web_ip="172.25.0.0/16"),
            self.discovery(gateway=""),
            [subprocess.CompletedProcess([], 0, "", "")],
            [subprocess.CompletedProcess([], 1, "", "private diagnostic")],
        ):
            with self.subTest(outputs=outputs), patch.object(environment.subprocess, "run", side_effect=outputs):
                with self.assertRaises(ValueError):
                    environment.trusted_proxy_ips(Path(".env"), Path("compose.yml"), {})


if __name__ == "__main__":
    unittest.main()
