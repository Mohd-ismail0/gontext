#!/usr/bin/env bash
# Validate deployment profile YAML files against deploy/schema/config.schema.json
# using JSON Schema (draft 2020-12). Requires python3 with PyYAML + jsonschema.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 required for profile validation" >&2
  exit 1
fi

ROOT="$ROOT" python3 <<'PY'
import glob, json, os, sys

try:
    import yaml
except ImportError:
    print("PyYAML required: pip install pyyaml", file=sys.stderr)
    sys.exit(1)
try:
    from jsonschema import Draft202012Validator
except ImportError:
    print("jsonschema required: pip install jsonschema", file=sys.stderr)
    sys.exit(1)

root = os.environ["ROOT"]
schema = json.load(open(os.path.join(root, "deploy/schema/config.schema.json")))
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema)

profiles = sorted(glob.glob(os.path.join(root, "deploy", "profiles", "*.yaml")))
if not profiles:
    print("no deployment profiles found", file=sys.stderr)
    sys.exit(1)

failed = False
for path in profiles:
    doc = yaml.safe_load(open(path))
    errs = sorted(validator.iter_errors(doc), key=lambda e: list(e.path))
    if errs:
        failed = True
        print("%s: %d schema error(s)" % (os.path.basename(path), len(errs)), file=sys.stderr)
        for e in errs:
            loc = "/".join(str(p) for p in e.path) or "<root>"
            print("  - %s: %s" % (loc, e.message), file=sys.stderr)
if failed:
    sys.exit(1)
print("profile schema validation ok (%d files)" % len(profiles))
PY
