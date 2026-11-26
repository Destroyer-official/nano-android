#!/usr/bin/env bash
set -euo pipefail

LOCK_FILE="tools/toolchain.lock.json"
if [ ! -f "$LOCK_FILE" ]; then
  echo "FAIL: Missing $LOCK_FILE" >&2
  exit 1
fi

python3 - "$LOCK_FILE" <<'PY'
import hashlib, json, os, sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    lock = json.load(f)

failures = 0
for art in lock.get("artifacts", []):
    p = art["path"]
    exp = art["sha256"].upper()
    if not os.path.isfile(p):
        print(f"FAIL: Missing pinned artifact {p} ({art['name']})", file=sys.stderr)
        failures += 1
        continue
    h = hashlib.sha256()
    with open(p, "rb") as af:
        while chunk := af.read(65536):
            h.update(chunk)
    actual = h.hexdigest().upper()
    if actual != exp:
        print(f"FAIL: SHA-256 mismatch for {p}\n  expected: {exp}\n  actual:   {actual}", file=sys.stderr)
        failures += 1
    else:
        print(f"OK: {art['name']} ({exp})")

if failures > 0:
    sys.exit(1)
print(f"SUCCESS: All {len(lock.get('artifacts', []))} pinned toolchain artifacts verified.")
PY
