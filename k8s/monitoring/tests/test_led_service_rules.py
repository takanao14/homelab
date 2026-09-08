#!/usr/bin/env python3
"""Usage: python3 test_led_service_rules.py <rendered-chart.yaml> (requires PyYAML and promtool)."""
import pathlib
import subprocess
import sys
import tempfile
import yaml

rendered = list(yaml.safe_load_all(pathlib.Path(sys.argv[1]).read_text()))
resource = next(item for item in rendered if item and item.get("metadata", {}).get("name") == "led-service-external")
with tempfile.TemporaryDirectory(prefix="led-rules-") as directory:
    rules = pathlib.Path(directory) / "rules.yaml"
    rules.write_text(yaml.safe_dump(resource["spec"]))
    tests = yaml.safe_load(pathlib.Path(__file__).with_suffix(".yaml").read_text())
    tests["rule_files"] = [str(rules)]
    test_file = pathlib.Path(directory) / "tests.yaml"
    test_file.write_text(yaml.safe_dump(tests))
    subprocess.run(["promtool", "test", "rules", str(test_file)], check=True)
