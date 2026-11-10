import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "audit-guest-os-recipes.py"
SPEC = importlib.util.spec_from_file_location("recipe_audit", SCRIPT)
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)


class AuditTests(unittest.TestCase):
    def test_current_image_conventions(self):
        for image, expected in [
            ("local:iso/ubuntu-26.04-base.img", ("ubuntu", "26.04")),
            ("local:iso/ubuntu-24.04-desktop.img", ("ubuntu", "24.04")),
            ("local:vztmpl/debian-13-standard_13.6-1_amd64.tar.zst", ("debian", "13")),
            ("local:iso/rocky-10-base.img", ("rocky", "10")),
            ("local:iso/freebsd-15.1-cloudinit-ufs.img", ("freebsd", "15.1")),
        ]:
            self.assertEqual(AUDIT.recipe_release(image), expected)

    def test_release_comparison(self):
        self.assertTrue(AUDIT.matches(("rocky", "10"), ("rocky", "10.1")))
        self.assertTrue(AUDIT.matches(("debian", "13"), ("debian", "13.6")))
        self.assertTrue(AUDIT.matches(("freebsd", "15.1"), ("freebsd", "15.1-RELEASE")))
        self.assertFalse(AUDIT.matches(("ubuntu", "26.04"), ("ubuntu", "24.04")))
        self.assertFalse(AUDIT.matches(("debian", "13"), ("ubuntu", "26.04")))
        self.assertFalse(AUDIT.matches(("rocky", "10"), ("rocky", "100.1")))

    def test_match_mismatch_and_incomplete(self):
        recipes = {"match": ("debian", "13"), "drift": ("ubuntu", "26.04"),
                   "offline": ("debian", "13"), "opaque": None,
                   "conflict": ("ubuntu", "26.04")}
        observed = {"match": {("debian", "13")}, "drift": {("ubuntu", "24.04")},
                    "conflict": {("ubuntu", "24.04"), ("ubuntu", "26.04")}}
        statuses = {row[0]: row[1] for row in AUDIT.audit(recipes, observed)}
        self.assertEqual(statuses, {"match": "match", "drift": "mismatch", "offline": "unobserved",
                                    "opaque": "unsupported-recipe", "conflict": "ambiguous-observation"})

    def test_unknown_image_does_not_pass(self):
        self.assertIsNone(AUDIT.recipe_release("local:iso/custom.img"))
        self.assertIsNone(AUDIT.recipe_release(None))

    def test_observation_deduplication_and_fqdn(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "observed.json"
            series = {"metric": {"nodename": "demo.home.example", "id": "debian", "version_id": "13"},
                      "value": [0, "1"]}
            path.write_text(json.dumps({"status": "success", "data": {"resultType": "vector", "result": [series, series]}}))
            self.assertEqual(AUDIT.read_observations(path), {"demo": {("debian", "13")}})

    def test_recipe_schema_and_duplicate_hostnames(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "recipes.json"
            record = {"kind": "vm", "node": "node1", "image": "local:iso/ubuntu-26.04-base.img"}
            path.write_text(json.dumps({"demo": record}))
            self.assertEqual(AUDIT.read_recipes([path]), {"demo": ("ubuntu", "26.04")})
            with self.assertRaises(ValueError):
                AUDIT.read_recipes([path, path])
            path.write_text(json.dumps({"demo": dict(record, unexpected="dummy-value")}))
            with self.assertRaises(ValueError):
                AUDIT.read_recipes([path])


    def test_cli_exit_codes_and_input_redaction(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipes = pathlib.Path(tmp) / "recipes.json"
            observations = pathlib.Path(tmp) / "observations.json"
            record = {"kind": "vm", "node": "node1", "image": "local:iso/ubuntu-26.04-base.img"}
            recipes.write_text(json.dumps({"demo": record}))
            series = {"metric": {"nodename": "demo", "id": "ubuntu", "version_id": "26.04"}, "value": [0, "1"]}
            payload = {"status": "success", "data": {"resultType": "vector", "result": [series]}}
            for expected in [0, 1, 3, 2]:
                if expected == 1:
                    series["metric"]["version_id"] = "24.04"
                elif expected == 3:
                    payload["data"]["result"] = []
                elif expected == 2:
                    recipes.write_text(json.dumps({"demo": dict(record, unexpected="redaction-test-sentinel")}))
                observations.write_text(json.dumps(payload))
                result = subprocess.run([sys.executable, str(SCRIPT), "--recipes", str(recipes),
                                         "--observations", str(observations)], capture_output=True, text=True)
                self.assertEqual(result.returncode, expected)
                self.assertNotIn("redaction-test-sentinel", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
