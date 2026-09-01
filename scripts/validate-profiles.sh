#!/usr/bin/env bash
# Validate deployment profile YAML files against deploy/schema/config.schema.json
# Requires: python3 with json and yaml (PyYAML) or go run ./cmd/cf validate-profiles
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCHEMA="$ROOT/deploy/schema/config.schema.json"
PROFILES="$ROOT/deploy/profiles"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 required for profile validation" >&2
  exit 1
fi

python3 <<'PY'
import json, sys, glob, os
try:
    import yaml
except ImportError:
    print("PyYAML required: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

root = os.environ.get("ROOT", ".")
schema_path = os.path.join(root, "deploy/schema/config.schema.json")
profiles_dir = os.path.join(root, "deploy/profiles")

with open(schema_path) as f:
    schema = json.load(f)

required_top = set(schema.get("required", []))
profile_enum = set(schema.get("properties", {}).get("profile", {}).get("enum", []))

errors = []
for path in sorted(glob.glob(os.path.join(profiles_dir, "*.yaml"))):
    with open(path) as f:
        doc = yaml.safe_load(f)
    name = os.path.basename(path)
    for key in required_top:
        if key not in doc:
            errors.append(f"{name}: missing required key {key!r}")
    prof = doc.get("profile")
    if prof and profile_enum and prof not in profile_enum:
        errors.append(f"{name}: profile {prof!r} not in schema enum")
    if doc.get("apiVersion") != "context-fabric.io/v1":
        errors.append(f"{name}: apiVersion must be context-fabric.io/v1")

if errors:
    for e in errors:
        print(e, file=sys.stderr)
    sys.exit(1)
print(f"profile validation ok ({len(list(glob.glob(os.path.join(profiles_dir, '*.yaml'))))} files)")
PY
