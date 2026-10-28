#!/usr/bin/env bash
# Validate contracts: JSON schemas, OpenAPI, OpenFGA model, migration parity.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> JSON Schema files (parse + draft 2020-12 metaschema)"
python3 -c "
import glob, json
from jsonschema import Draft202012Validator
files = sorted(glob.glob('contracts/jsonschema/*.json')) + sorted(glob.glob('deploy/schema/*.schema.json'))
for f in files:
    Draft202012Validator.check_schema(json.load(open(f)))
print('validated %d schema documents' % len(files))
"

echo "==> OpenAPI"
test -s contracts/openapi/openapi.yaml

echo "==> OpenFGA model"
python3 -c "
import json
m=json.load(open('contracts/openfga/model.json'))
assert m.get('schema_version'), 'schema_version required'
assert m.get('type_definitions'), 'type_definitions required'
"

echo "==> AsyncAPI"
test -s contracts/asyncapi/asyncapi.yaml

echo "==> Migration embed parity"
diff -qr migrations internal/migrate/migrations

echo "==> Profiles"
ROOT="$ROOT" bash scripts/validate-profiles.sh

echo "==> contract validation ok"
