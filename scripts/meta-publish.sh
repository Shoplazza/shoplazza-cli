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
# The revision is the spec's own generated_at (canonical UTC "Z" form,
# enforced here because clients order revisions lexically).
set -euo pipefail

SPEC_FILE="${1:?usage: meta-publish.sh <cli_meta.json> [min_cli_version]}"
MIN_CLI_VERSION="${2:-}"
: "${META_OSS_BUCKET:?set META_OSS_BUCKET, e.g. oss://bucket/shoplazza-cli/meta}"
OSSUTIL="${OSSUTIL:-ossutil}"

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }
command -v "$OSSUTIL" >/dev/null || { echo "$OSSUTIL not found" >&2; exit 1; }

if [ -n "$MIN_CLI_VERSION" ] && ! [[ "$MIN_CLI_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "min_cli_version must be X.Y.Z, got: $MIN_CLI_VERSION" >&2
  exit 1
fi

# Validate the spec and extract its revision. Values are passed to python via
# argv/env only — never interpolated into code.
REVISION=$(python3 - "$SPEC_FILE" <<'PY'
import json, re, sys
spec = json.load(open(sys.argv[1]))
rev = spec.get("generated_at", "")
if not re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", rev):
    sys.exit(f"generated_at must be canonical UTC (YYYY-MM-DDTHH:MM:SSZ), got: {rev!r}")
if not spec.get("version") or not spec.get("modules"):
    sys.exit("spec must have version and non-empty modules")
names = [m.get("name") for m in spec["modules"]]
if len(names) != len(set(names)) or not all(names):
    sys.exit("spec has empty or duplicate module names")
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

REVISION="$REVISION" REV_KEY="$REV_KEY" SHA256="$SHA256" MIN_CLI_VERSION="$MIN_CLI_VERSION" \
python3 - "$WORK/manifest.json" <<'PY'
import json, os, sys
manifest = {
    "format_version": 1,
    "revision": os.environ["REVISION"],
    "url": f"specs/{os.environ['REV_KEY']}.json.gz",
    "sha256": os.environ["SHA256"],
}
if os.environ["MIN_CLI_VERSION"]:
    manifest["min_cli_version"] = os.environ["MIN_CLI_VERSION"]
json.dump(manifest, open(sys.argv[1], "w"), indent=2)
PY

SPEC_DEST="$META_OSS_BUCKET/specs/$REV_KEY.json.gz"

# Immutable spec object: refuse to overwrite, and refuse to proceed when
# existence can't be determined (a transient stat error must not become an
# overwrite of an object CDNs already cached).
if STAT_OUT=$("$OSSUTIL" stat "$SPEC_DEST" 2>&1); then
  echo "refusing to overwrite existing $SPEC_DEST — bump generated_at instead" >&2
  exit 1
elif ! grep -qiE "NoSuchKey|not exist|StatusCode=404" <<<"$STAT_OUT"; then
  echo "cannot verify $SPEC_DEST absence, aborting: $STAT_OUT" >&2
  exit 1
fi
"$OSSUTIL" cp "$WORK/spec.json.gz" "$SPEC_DEST" --meta "Cache-Control:max-age=31536000,immutable"
"$OSSUTIL" cp -f "$WORK/manifest.json" "$META_OSS_BUCKET/manifest.json" --meta "Cache-Control:no-cache"

echo "published revision $REVISION (sha256=$SHA256)"
