#!/usr/bin/env bash
# Publish a cli_meta spec to the static metadata origin.
#
# Usage:   meta-publish.sh <cli_meta.json> [min_cli_version]
# Env:     META_OSS_BUCKET  destination root, e.g. oss://my-bucket/shoplazza-cli/meta  (required)
#          OSSUTIL          ossutil binary (default: ossutil)
#
# Layout produced:
#   ${META_OSS_BUCKET}/specs/<revision>.json.gz   immutable, refuses overwrite
#   ${META_OSS_BUCKET}/manifest.json              overwritten atomically, no-cache
#
# The revision is the spec's own generated_at; the CLI only adopts a download
# whose generated_at matches the manifest revision, so never edit one without
# the other (this script keeps them in sync).
set -euo pipefail

SPEC_FILE="${1:?usage: meta-publish.sh <cli_meta.json> [min_cli_version]}"
MIN_CLI_VERSION="${2:-}"
: "${META_OSS_BUCKET:?set META_OSS_BUCKET, e.g. oss://bucket/shoplazza-cli/meta}"
OSSUTIL="${OSSUTIL:-ossutil}"

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }
command -v "$OSSUTIL" >/dev/null || { echo "$OSSUTIL not found" >&2; exit 1; }

# Validate the spec and extract its revision (generated_at).
REVISION=$(python3 - "$SPEC_FILE" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1]))
rev = spec.get("generated_at", "")
if not rev or not spec.get("modules"):
    sys.exit("spec must have generated_at and non-empty modules")
print(rev)
PY
)

REV_KEY=$(echo "$REVISION" | tr -d ':')  # OSS object name without colons
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

gzip -9 -c "$SPEC_FILE" > "$WORK/spec.json.gz"

# sha256 of the gzipped object (macOS shasum / linux sha256sum).
if command -v sha256sum >/dev/null; then
  SHA256=$(sha256sum "$WORK/spec.json.gz" | awk '{print $1}')
else
  SHA256=$(shasum -a 256 "$WORK/spec.json.gz" | awk '{print $1}')
fi
SIZE=$(wc -c < "$WORK/spec.json.gz" | tr -d ' ')

python3 - "$WORK/manifest.json" <<PY
import json, sys
manifest = {
    "format_version": 1,
    "revision": "$REVISION",
    "url": "specs/$REV_KEY.json.gz",
    "sha256": "$SHA256",
    "size": $SIZE,
}
if "$MIN_CLI_VERSION":
    manifest["min_cli_version"] = "$MIN_CLI_VERSION"
json.dump(manifest, open(sys.argv[1], "w"), indent=2)
PY

SPEC_DEST="$META_OSS_BUCKET/specs/$REV_KEY.json.gz"

# Immutable spec object: refuse to overwrite an existing revision.
if "$OSSUTIL" stat "$SPEC_DEST" >/dev/null 2>&1; then
  echo "refusing to overwrite existing $SPEC_DEST — bump generated_at instead" >&2
  exit 1
fi
"$OSSUTIL" cp "$WORK/spec.json.gz" "$SPEC_DEST" --meta "Cache-Control:max-age=31536000,immutable"
"$OSSUTIL" cp -f "$WORK/manifest.json" "$META_OSS_BUCKET/manifest.json" --meta "Cache-Control:no-cache"

echo "published revision $REVISION (sha256=$SHA256, ${SIZE}B gz)"
