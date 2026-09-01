#!/usr/bin/env bash
# Validate contracts: JSON schemas, OpenAPI, OpenFGA model, migration parity.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> JSON Schema files"
for f in contracts/jsonschema/*.json; do
  python3 -c "import json; json.load(open('$f'))"
done

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
